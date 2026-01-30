package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ConfigType defines which configuration types to load
type ConfigType string

const (
	ConfigTypeDatabase ConfigType = "database"
	ConfigTypeFtp      ConfigType = "ftp"
	ConfigTypeStorage  ConfigType = "storage"
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
	case "database":
		p.processDatabaseKey(tenantID, parts, value, data)
	case "ftp":
		p.processFtpKey(tenantID, parts, value, data)
	case "storage":
		p.processStorageKey(tenantID, parts, value, data)
	}
}

// Placeholder methods for Task 8 implementation
func (p *Provider) processDatabaseKey(tenantID int, parts []string, value string, data *tenantData) {
	// TODO: Implement in Task 8
}

func (p *Provider) processFtpKey(tenantID int, parts []string, value string, data *tenantData) {
	// TODO: Implement in Task 8
}

func (p *Provider) processStorageKey(tenantID int, parts []string, value string, data *tenantData) {
	// TODO: Implement in Task 8
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

	go p.watchLoop(watchChan)

	return nil
}

func (p *Provider) watchLoop(watchChan clientv3.WatchChan) {
	for {
		select {
		case <-p.watchCtx.Done():
			return
		case wr, ok := <-watchChan:
			if !ok {
				return
			}
			for _, ev := range wr.Events {
				p.handleWatchEvent(ev)
			}
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
		databases: make(map[int]*DatabaseConfig),
		ftps:      make(map[int]*FtpConfig),
		storages:  make(map[int]*StorageConfig),
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

	p.watchCancel()
	p.running.Store(false)
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

