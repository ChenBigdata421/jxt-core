package provider

// Package provider provides ETCD-based tenant configuration with automatic reconnection.
//
// Reconnection Behavior:
//   - Watch channels that close are automatically reconnected
//   - Exponential backoff: starts at 1s, max 30s, increases by 1.5x each retry
//   - Backoff resets to 1s after successful event processing
//   - All reconnection events are logged for monitoring
//
// Initialization with Retry:
//   - Use NewProviderWithRetry() for automatic retry on LoadAll failures
//   - Retry strategy: 5 attempts, 1s→16s exponential backoff
//   - Total timeout: ~31 seconds
//
// Cache Fallback:
//   - Configure WithCache() to enable persistent file cache
//   - On ETCD failure, automatically loads from local cache
//   - Cache is synced on every ETCD update
//
// Usage:
//
//	// With retry and cache
//	provider, err := provider.NewProviderWithRetry(etcdClient,
//	    provider.WithNamespace("jxt/"),
//	    provider.WithConfigTypes(provider.ConfigTypeDatabase),
//	    provider.WithCache(provider.NewFileCache()),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Start watch with retry
//	if err := provider.StartWatchWithRetry(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer provider.StopWatch()
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time" // For exponential backoff

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger" // Used for watch reconnection logging
	retryv4 "github.com/avast/retry-go/v4"
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
	cache       FileCache    // 持久化缓存（可选）

	mu         sync.RWMutex
	watchCtx   context.Context
	watchCancel context.CancelFunc
	running     atomic.Bool
}

// tenantData 租户数据（新格式）
type tenantData struct {
	Metas     map[int]*TenantMeta                      `json:"metas"`
	Databases map[int]map[string]*ServiceDatabaseConfig `json:"databases"` // tenantID -> serviceCode -> config
	Ftps      map[int][]*FtpConfigDetail                 `json:"ftps"`      // tenantID -> configs[]
	Storages  map[int]*StorageConfig                     `json:"storages"`
	Domains   map[int]*DomainConfig                       `json:"domains"`   // 新增：域名配置
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

// FileCache defines the interface for persistent cache operations.
type FileCache interface {
	Load() (*tenantData, error)
	Save(data *tenantData) error
	IsAvailable() bool
}

// fileCacheAdapter adapts cache.FileCache to provider.FileCache interface
type fileCacheAdapter struct {
	inner *innerFileCache
}

// innerFileCache is the actual cache implementation
type innerFileCache struct {
	filePath string
	mu       sync.RWMutex
}

// newInnerFileCache creates a new inner file cache
func newInnerFileCache() *innerFileCache {
	cachePath := os.Getenv("TENANT_CACHE_PATH")
	if cachePath == "" {
		cachePath = "./cache"
	}
	return &innerFileCache{
		filePath: filepath.Join(cachePath, "tenant_metadata.json"),
	}
}

func (a *fileCacheAdapter) Load() (*tenantData, error) {
	bytes, err := a.inner.loadBytes()
	if err != nil {
		return nil, err
	}

	var data tenantData
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("failed to decode cache: %w", err)
	}
	return &data, nil
}

func (a *fileCacheAdapter) Save(data *tenantData) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode cache: %w", err)
	}
	return a.inner.saveBytes(bytes)
}

func (a *fileCacheAdapter) IsAvailable() bool {
	return a.inner.isAvailable()
}

func (i *innerFileCache) loadBytes() ([]byte, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return os.ReadFile(i.filePath)
}

func (i *innerFileCache) saveBytes(bytes []byte) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(i.filePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	tmpPath := i.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, i.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename cache file: %w", err)
	}
	return nil
}

func (i *innerFileCache) isAvailable() bool {
	_, err := os.ReadFile(i.filePath)
	return err == nil
}

// WithCache configures Provider to use persistent file cache as fallback.
func WithCache(cache FileCache) Option {
	return func(p *Provider) {
		p.cache = cache
	}
}

// NewFileCache creates a new file cache for provider use.
func NewFileCache() FileCache {
	return &fileCacheAdapter{inner: newInnerFileCache()}
}

// NewFileCacheWithPath creates a cache with a specific file path.
func NewFileCacheWithPath(path string) FileCache {
	return &fileCacheAdapter{inner: &innerFileCache{filePath: path}}
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
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	})
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// NewProviderWithRetry creates a Provider and initializes it with retry.
func NewProviderWithRetry(client *clientv3.Client, opts ...Option) (*Provider, error) {
	p := NewProvider(client, opts...)

	// Retry LoadAll with exponential backoff
	if err := retryv4.Do(
		func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return p.LoadAll(ctx)
		},
		retryv4.Attempts(5),
		retryv4.Delay(time.Second),
		retryv4.MaxDelay(16*time.Second),
		retryv4.DelayType(retryv4.BackOffDelay),
		retryv4.LastErrorOnly(true),
	); err != nil {
		return nil, fmt.Errorf("failed to load tenant data after retries: %w", err)
	}

	logger.Infof("tenant provider: initialized successfully")
	return p, nil
}

// LoadAll loads all tenant data from ETCD, with cache fallback.
func (p *Provider) LoadAll(ctx context.Context) error {
	prefix := p.namespace + "tenants/"

	resp, err := p.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		// 尝试从缓存降级
		if p.cache != nil && p.cache.IsAvailable() {
			logger.Warnf("tenant provider: ETCD unavailable, loading from cache")
			cachedData, cacheErr := p.cache.Load()
			if cacheErr == nil {
				p.data.Store(cachedData)
				logger.Infof("tenant provider: loaded from cache")
				return nil
			}
			logger.Errorf("tenant provider: cache load failed: %v", cacheErr)
		}
		return fmt.Errorf("ETCD Get failed: %w", err)
	}

	// 正常从 ETCD 加载
	newData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	for _, kv := range resp.Kvs {
		p.processKey(string(kv.Key), string(kv.Value), newData)
	}

	p.data.Store(newData)
	logger.Infof("tenant provider: loaded %d tenants from ETCD", len(newData.Databases))

	// 同步到缓存
	if p.cache != nil {
		go func() {
			if err := p.cache.Save(newData); err != nil {
				logger.Errorf("tenant provider: failed to save cache: %v", err)
			}
		}()
	}

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
		// Check if service code is provided in the key path: tenants/{id}/database/{serviceCode}
		serviceCode := ""
		if len(parts) >= 4 {
			serviceCode = parts[3]
		}
		p.processDatabaseKey(tenantID, serviceCode, value, data)
	case "ftp":
		p.processFtpKey(tenantID, value, data)
	case "storage":
		p.processStorageKey(tenantID, value, data)
	case "domain":
		p.processDomainKey(tenantID, value, data)
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
	data.Metas[tenantID] = &meta
}

// processDatabaseKey processes service-level database configuration from ETCD
func (p *Provider) processDatabaseKey(tenantID int, serviceCode string, value string, data *tenantData) {
	var dbConfig ServiceDatabaseConfig
	if err := json.Unmarshal([]byte(value), &dbConfig); err != nil {
		return
	}

	// If serviceCode is not provided in the key path, try to get it from the config
	if serviceCode == "" && dbConfig.ServiceCode != "" {
		serviceCode = dbConfig.ServiceCode
	}
	if serviceCode == "" {
		// Default service code for backward compatibility
		serviceCode = "default"
	}

	dbConfig.TenantID = tenantID
	dbConfig.ServiceCode = serviceCode

	// Initialize nested map if needed
	if data.Databases[tenantID] == nil {
		data.Databases[tenantID] = make(map[string]*ServiceDatabaseConfig)
	}
	data.Databases[tenantID][serviceCode] = &dbConfig
}

// processFtpKey processes FTP configuration from ETCD
func (p *Provider) processFtpKey(tenantID int, value string, data *tenantData) {
	var ftpConfig FtpConfigDetail
	if err := json.Unmarshal([]byte(value), &ftpConfig); err != nil {
		return
	}

	ftpConfig.TenantID = tenantID

	// Initialize array if needed
	if data.Ftps[tenantID] == nil {
		data.Ftps[tenantID] = []*FtpConfigDetail{}
	}

	// Use helper function to append or update
	data.Ftps[tenantID] = appendOrUpdateFtpConfig(data.Ftps[tenantID], &ftpConfig)
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
	if meta, ok := data.Metas[tenantID]; ok {
		storageConfig.Code = meta.Code
		storageConfig.Name = meta.Name
	}

	data.Storages[tenantID] = &storageConfig
}

// processDomainKey processes domain configuration from ETCD
func (p *Provider) processDomainKey(tenantID int, value string, data *tenantData) {
	var domainConfig DomainConfig
	if err := json.Unmarshal([]byte(value), &domainConfig); err != nil {
		return
	}

	domainConfig.TenantID = tenantID
	data.Domains[tenantID] = &domainConfig
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

	// 同步到缓存
	if p.cache != nil {
		go func() {
			if err := p.cache.Save(newData); err != nil {
				logger.Errorf("tenant provider: failed to sync cache: %v", err)
			}
		}()
	}
}

func (p *Provider) copyTenantData(src *tenantData) *tenantData {
	dst := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	for k, v := range src.Metas {
		dst.Metas[k] = v
	}
	for k, v := range src.Databases {
		dst.Databases[k] = make(map[string]*ServiceDatabaseConfig)
		for kk, vv := range v {
			dst.Databases[k][kk] = vv
		}
	}
	for k, v := range src.Ftps {
		newFtps := make([]*FtpConfigDetail, len(v))
		copy(newFtps, v)
		dst.Ftps[k] = newFtps
	}
	for k, v := range src.Storages {
		dst.Storages[k] = v
	}
	for k, v := range src.Domains {
		dst.Domains[k] = v
	}

	return dst
}

// copyData 创建 tenantData 的深拷贝
func (d *tenantData) copyData() *tenantData {
	newData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// 复制 Metas
	for k, v := range d.Metas {
		newData.Metas[k] = v
	}

	// 复制 Databases
	for k, v := range d.Databases {
		newData.Databases[k] = make(map[string]*ServiceDatabaseConfig)
		for kk, vv := range v {
			newData.Databases[k][kk] = vv
		}
	}

	// 复制 Ftps
	for k, v := range d.Ftps {
		newFtps := make([]*FtpConfigDetail, len(v))
		copy(newFtps, v)
		newData.Ftps[k] = newFtps
	}

	// 复制 Storages
	for k, v := range d.Storages {
		newData.Storages[k] = v
	}

	// 复制 Domains
	for k, v := range d.Domains {
		newData.Domains[k] = v
	}

	return newData
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

// StartWatchWithRetry starts ETCD watch with retry on failure.
func (p *Provider) StartWatchWithRetry(ctx context.Context) error {
	return retryv4.Do(
		func() error {
			return p.StartWatch(ctx)
		},
		retryv4.Attempts(3),
		retryv4.Delay(time.Second),
		retryv4.MaxDelay(5*time.Second),
		retryv4.DelayType(retryv4.BackOffDelay),
	)
}

// handleDeleteKey handles delete events
func (p *Provider) handleDeleteKey(key string, data *tenantData) {
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
		delete(data.Metas, tenantID)
	case "database":
		// Check if service code is provided: tenants/{id}/database/{serviceCode}
		if len(parts) >= 4 {
			serviceCode := parts[3]
			if dbMap, ok := data.Databases[tenantID]; ok {
				delete(dbMap, serviceCode)
				// Clean up empty map
				if len(dbMap) == 0 {
					delete(data.Databases, tenantID)
				}
			}
		} else {
			delete(data.Databases, tenantID)
		}
	case "ftp":
		delete(data.Ftps, tenantID)
	case "storage":
		delete(data.Storages, tenantID)
	case "domain":
		delete(data.Domains, tenantID)
	}
}

// GetServiceDatabaseConfig retrieves the service-level database configuration for a tenant
func (p *Provider) GetServiceDatabaseConfig(tenantID int, serviceCode string) (*ServiceDatabaseConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	if dbMap, ok := data.Databases[tenantID]; ok {
		cfg, ok := dbMap[serviceCode]
		return cfg, ok
	}
	return nil, false
}

// GetAllServiceDatabases retrieves all service database configurations for a tenant
func (p *Provider) GetAllServiceDatabases(tenantID int) (map[string]*ServiceDatabaseConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	cfg, ok := data.Databases[tenantID]
	return cfg, ok
}

// GetFtpConfigs retrieves all FTP configurations for a tenant
func (p *Provider) GetFtpConfigs(tenantID int) ([]*FtpConfigDetail, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	cfg, ok := data.Ftps[tenantID]
	return cfg, ok
}

// GetDomainConfig retrieves the domain configuration for a tenant
func (p *Provider) GetDomainConfig(tenantID int) (*DomainConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	cfg, ok := data.Domains[tenantID]
	return cfg, ok
}

// GetStorageConfig retrieves the storage configuration for a tenant
func (p *Provider) GetStorageConfig(tenantID int) (*StorageConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	cfg, ok := data.Storages[tenantID]
	return cfg, ok
}

// GetTenantMeta retrieves the tenant metadata
func (p *Provider) GetTenantMeta(tenantID int) (*TenantMeta, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	meta, ok := data.Metas[tenantID]
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

