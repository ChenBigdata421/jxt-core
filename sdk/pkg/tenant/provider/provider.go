package provider

// Package provider provides ETCD-based tenant configuration with automatic reconnection.
//
// Reconnection Behavior:
//   - Watch channels that close are automatically reconnected
//   - Exponential backoff: starts at 1s, max 30s, increases by 1.5x each retry
//   - Backoff resets to 1s after successful event processing
//   - All reconnection events are logged for monitoring
//
// Usage:
//
//	provider := NewProvider(etcdClient, WithNamespace("jxt/"))
//	provider.LoadAll(ctx)
//	provider.StartWatch(ctx)
//	defer provider.StopWatch()
import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time" // For exponential backoff

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger" // Used for watch reconnection logging
)

// ConfigType defines which configuration types to load
type ConfigType string

const (
	ConfigTypeDatabase ConfigType = "database"
	ConfigTypeFtp      ConfigType = "ftp"
	ConfigTypeStorage  ConfigType = "storage"
)

// Backoff configuration for ETCD watch reconnection
const (
	// InitialBackoff is the starting backoff duration when reconnection is needed
	InitialBackoff = 1 * time.Second
	// MaxBackoff is the maximum backoff duration
	MaxBackoff = 30 * time.Second
	// BackoffMultiplier is the factor by which backoff increases each retry
	BackoffMultiplier = 1.5
)

func (ct ConfigType) String() string {
	return string(ct)
}

// Provider fetches tenant data from ETCD and caches it
type Provider struct {
	client      *clientv3.Client
	namespace   string
	configTypes []ConfigType
	data        atomic.Value // *tenantData

	mu         sync.RWMutex
	watchCtx   context.Context
	watchCancel context.CancelFunc
	running     atomic.Bool
}

type tenantData struct {
	metas    map[int]*TenantMeta
	databases map[int]*DatabaseConfig
	ftps      map[int]*FtpConfig
	storages  map[int]*StorageConfig
}

// Option configures Provider
type Option func(*Provider)

// WithConfigTypes sets which configuration types to load
func WithConfigTypes(types ...ConfigType) Option {
	return func(p *Provider) {
		p.configTypes = types
	}
}

// WithNamespace sets ETCD namespace prefix
func WithNamespace(ns string) Option {
	return func(p *Provider) {
		p.namespace = ns
	}
}

// NewProvider creates a new ETCD tenant provider
func NewProvider(client *clientv3.Client, opts ...Option) *Provider {
	p := &Provider{
		client:      client,
		namespace:   "jxt/",
		configTypes: []ConfigType{ConfigTypeDatabase}, // default
		data:        atomic.Value{},
	}
	p.data.Store(&tenantData{
		metas:    make(map[int]*TenantMeta),
		databases: make(map[int]*DatabaseConfig),
		ftps:      make(map[int]*FtpConfig),
		storages:  make(map[int]*StorageConfig),
	})
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// LoadAll loads all tenant data from ETCD
func (p *Provider) LoadAll(ctx context.Context) error {
	prefix := p.namespace + "tenants/"

	resp, err := p.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	newData := &tenantData{
		metas:    make(map[int]*TenantMeta),
		databases: make(map[int]*DatabaseConfig),
		ftps:      make(map[int]*FtpConfig),
		storages:  make(map[int]*StorageConfig),
	}

	for _, kv := range resp.Kvs {
		p.processKey(string(kv.Key), string(kv.Value), newData)
	}

	p.data.Store(newData)
	return nil
}

// parseTenantID extracts tenant ID from ETCD key
func (p *Provider) parseTenantID(key string) (int, bool) {
	// Remove namespace
	if !strings.HasPrefix(key, p.namespace) {
		return 0, false
	}
	key = strings.TrimPrefix(key, p.namespace)

	// Expected: tenants/{id}/...
	parts := strings.Split(key, "/")
	if len(parts) < 2 || parts[0] != "tenants" {
		return 0, false
	}

	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false
	}

	return id, true
}

func (p *Provider) processKey(key, value string, data *tenantData) {
	tenantID, ok := p.parseTenantID(key)
	if !ok {
		return
	}

	// Parse key path: tenants/{id}/{category} or tenants/{id}/{category}/{field}
	parts := strings.Split(strings.TrimPrefix(key, p.namespace), "/")
	if len(parts) < 3 {
		return
	}

	category := parts[2]

	switch category {
	case "meta":
		p.processMetaKey(tenantID, value, data)
	case "database":
		p.processDatabaseKey(tenantID, value, data)
	case "ftp":
		p.processFtpKey(tenantID, value, data)
	case "storage":
		p.processStorageKey(tenantID, value, data)
	}
}

// processMetaKey processes tenant metadata from ETCD
func (p *Provider) processMetaKey(tenantID int, value string, data *tenantData) {
	var meta TenantMeta
	if err := json.Unmarshal([]byte(value), &meta); err != nil {
		return
	}
	// Ensure TenantID matches the key
	meta.TenantID = tenantID
	data.metas[tenantID] = &meta
}

// processDatabaseKey processes database configuration from ETCD
func (p *Provider) processDatabaseKey(tenantID int, value string, data *tenantData) {
	// ETCD JSON uses "databaseName", need to map to DbName
	type etcdDatabaseConfig struct {
		TenantID     int    `json:"tenantId"`
		Driver       string `json:"driver"`
		DatabaseName string `json:"databaseName"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		SSLMode      string `json:"sslMode"`
		MaxOpenConns int `json:"maxOpenConns"`
		MaxIdleConns int `json:"maxIdleConns"`
	}

	var etcdDB etcdDatabaseConfig
	if err := json.Unmarshal([]byte(value), &etcdDB); err != nil {
		return
	}

	dbConfig := DatabaseConfig{
		TenantID:     tenantID,
		Driver:       etcdDB.Driver,
		DbName:       etcdDB.DatabaseName,
		Host:         etcdDB.Host,
		Port:         etcdDB.Port,
		Username:     etcdDB.Username,
		Password:     etcdDB.Password,
		SSLMode:      etcdDB.SSLMode,
		MaxOpenConns: etcdDB.MaxOpenConns,
		MaxIdleConns: etcdDB.MaxIdleConns,
	}

	// Merge meta information if available
	if meta, ok := data.metas[tenantID]; ok {
		dbConfig.Code = meta.Code
		dbConfig.Name = meta.Name
	}

	data.databases[tenantID] = &dbConfig
}

// processFtpKey processes FTP configuration from ETCD
func (p *Provider) processFtpKey(tenantID int, value string, data *tenantData) {
	var ftpConfig FtpConfig
	if err := json.Unmarshal([]byte(value), &ftpConfig); err != nil {
		return
	}

	// Merge meta information if available
	if meta, ok := data.metas[tenantID]; ok {
		ftpConfig.Code = meta.Code
		ftpConfig.Name = meta.Name
	}

	ftpConfig.TenantID = tenantID
	data.ftps[tenantID] = &ftpConfig
}

// processStorageKey processes storage configuration from ETCD
func (p *Provider) processStorageKey(tenantID int, value string, data *tenantData) {
	// ETCD storage uses tenantId as int, convert to int64 for our config
	var etcdStorage struct {
		TenantID             int    `json:"tenantId"`
		UploadQuotaGb        int    `json:"uploadQuotaGb"`
		MaxFileSizeMb        int    `json:"maxFileSizeMb"`
		MaxConcurrentUploads int    `json:"maxConcurrentUploads"`
	}

	if err := json.Unmarshal([]byte(value), &etcdStorage); err != nil {
		return
	}

	storageConfig := StorageConfig{
		TenantID:             int64(etcdStorage.TenantID),
		QuotaBytes:           int64(etcdStorage.UploadQuotaGb) * 1024 * 1024 * 1024, // Convert GB to bytes
		MaxFileSizeBytes:     int64(etcdStorage.MaxFileSizeMb) * 1024 * 1024,       // Convert MB to bytes
		MaxConcurrentUploads: etcdStorage.MaxConcurrentUploads,
	}

	// Merge meta information if available
	if meta, ok := data.metas[tenantID]; ok {
		storageConfig.Code = meta.Code
		storageConfig.Name = meta.Name
	}

	data.storages[tenantID] = &storageConfig
}

// StartWatch begins watching ETCD for changes
func (p *Provider) StartWatch(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running.Load() {
		return fmt.Errorf("provider already watching")
	}

	p.watchCtx, p.watchCancel = context.WithCancel(ctx)

	// Build watch prefix
	prefix := p.namespace + "tenants/"

	watchChan := p.client.Watch(p.watchCtx, prefix, clientv3.WithPrefix())

	p.running.Store(true)

	logger.Infof("tenant provider: starting ETCD watch for prefix %s", prefix)

	go p.watchLoop(watchChan)

	return nil
}

func (p *Provider) watchLoop(initialWatchChan clientv3.WatchChan) {
	backoff := InitialBackoff
	watchChan := initialWatchChan

	// Log initial watch start
	logger.Infof("tenant provider: started watching ETCD with prefix %s", p.namespace+"tenants/")

	for {
		select {
		case <-p.watchCtx.Done():
			logger.Info("tenant provider: watch stopped by context")
			return

		case wr, ok := <-watchChan:
			if !ok {
				// Watch channel closed - initiate reconnection with backoff
				logger.Warnf("tenant provider: ETCD watch channel closed, reconnecting in %v", backoff)

				select {
				case <-p.watchCtx.Done():
					logger.Info("tenant provider: reconnection cancelled by context")
					return
				case <-time.After(backoff):
					// Re-create the watch
					prefix := p.namespace + "tenants/"
					watchChan = p.client.Watch(p.watchCtx, prefix, clientv3.WithPrefix())
					logger.Infof("tenant provider: reconnected to ETCD watch, next backoff will be %v",
						calculateBackoff(backoff))
					backoff = calculateBackoff(backoff)
					continue
				}
			}

			// Check for watch errors
			if wr.Err() != nil {
				logger.Errorf("tenant provider: ETCD watch error: %v", wr.Err())
				continue
			}

			// Process events successfully - reset backoff to initial
			for _, ev := range wr.Events {
				p.handleWatchEvent(ev)
			}
			backoff = InitialBackoff
		}
	}
}

func (p *Provider) handleWatchEvent(ev *clientv3.Event) {
	// Get current data snapshot
	current := p.data.Load().(*tenantData)
	newData := p.copyTenantData(current)

	key := string(ev.Kv.Key)
	value := string(ev.Kv.Value)

	switch ev.Type {
	case clientv3.EventTypePut:
		p.processKey(key, value, newData)
	case clientv3.EventTypeDelete:
		p.handleDeleteKey(key, newData)
	}

	p.data.Store(newData)
}

func (p *Provider) copyTenantData(src *tenantData) *tenantData {
	dst := &tenantData{
		metas:    make(map[int]*TenantMeta),
		databases: make(map[int]*DatabaseConfig),
		ftps:      make(map[int]*FtpConfig),
		storages:  make(map[int]*StorageConfig),
	}

	for k, v := range src.metas {
		dst.metas[k] = v
	}
	for k, v := range src.databases {
		dst.databases[k] = v
	}
	for k, v := range src.ftps {
		dst.ftps[k] = v
	}
	for k, v := range src.storages {
		dst.storages[k] = v
	}

	return dst
}

// StopWatch stops watching ETCD for changes
func (p *Provider) StopWatch() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running.Load() {
		return
	}

	logger.Info("tenant provider: stopping ETCD watch")

	p.watchCancel()
	p.running.Store(false)

	logger.Info("tenant provider: ETCD watch stopped")
}

// handleDeleteKey handles delete events
func (p *Provider) handleDeleteKey(key string, data *tenantData) {
	tenantID, ok := p.parseTenantID(key)
	if !ok {
		return
	}

	// Parse key path: tenants/{id}/{category}
	parts := strings.Split(strings.TrimPrefix(key, p.namespace), "/")
	if len(parts) < 3 {
		return
	}

	category := parts[2]

	switch category {
	case "meta":
		delete(data.metas, tenantID)
	case "database":
		delete(data.databases, tenantID)
	case "ftp":
		delete(data.ftps, tenantID)
	case "storage":
		delete(data.storages, tenantID)
	}
}

// GetDatabaseConfig retrieves the database configuration for a tenant
func (p *Provider) GetDatabaseConfig(tenantID int) (*DatabaseConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	cfg, ok := data.databases[tenantID]
	return cfg, ok
}

// GetFtpConfig retrieves the FTP configuration for a tenant
func (p *Provider) GetFtpConfig(tenantID int) (*FtpConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	cfg, ok := data.ftps[tenantID]
	return cfg, ok
}

// GetStorageConfig retrieves the storage configuration for a tenant
func (p *Provider) GetStorageConfig(tenantID int) (*StorageConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	cfg, ok := data.storages[tenantID]
	return cfg, ok
}

// GetTenantMeta retrieves the tenant metadata
func (p *Provider) GetTenantMeta(tenantID int) (*TenantMeta, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	meta, ok := data.metas[tenantID]
	return meta, ok
}

// IsTenantEnabled returns true if the tenant is active
func (p *Provider) IsTenantEnabled(tenantID int) bool {
	meta, ok := p.GetTenantMeta(tenantID)
	return ok && meta.IsEnabled()
}

// calculateBackoff computes the next backoff duration with exponential increase
func calculateBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * BackoffMultiplier)
	if next > MaxBackoff {
		return MaxBackoff
	}
	return next
}

