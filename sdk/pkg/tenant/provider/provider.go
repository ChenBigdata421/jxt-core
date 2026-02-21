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
	Metas       map[int]*TenantMeta                       `json:"metas"`
	Databases   map[int]map[string]*ServiceDatabaseConfig `json:"databases"` // tenantID -> serviceCode -> config
	Ftps        map[int][]*FtpConfigDetail                `json:"ftps"`      // tenantID -> configs[]
	Storages    map[int]*StorageConfig                    `json:"storages"`
	Domains     map[int]*DomainConfig                     `json:"domains"`     // 域名配置
	Resolver    *ResolverConfig                           `json:"resolver"`    // 全局识别配置
	domainIndex map[string]int                            // 内嵌：域名反向索引 domain -> tenantID（不导出，不序列化）
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
	initialData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}
	initialData.domainIndex = buildDomainIndex(initialData) // 构建初始索引
	p.data.Store(initialData)
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
	prefixes := []string{
		p.namespace + "tenants/",
		p.namespace + "common/",
	}

	newData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	for _, prefix := range prefixes {
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
			return fmt.Errorf("ETCD Get failed for prefix %s: %w", prefix, err)
		}

		for _, kv := range resp.Kvs {
			p.processKey(string(kv.Key), string(kv.Value), newData)
		}
	}

	newData.domainIndex = buildDomainIndex(newData) // 构建索引
	p.data.Store(newData)                           // 一次原子替换
	logger.Infof("tenant provider: loaded %d tenants, %d database configs, %d ftp configs, %d domain mappings from ETCD",
		len(newData.Metas), countServiceDatabases(newData.Databases), countFtpConfigs(newData.Ftps),
		len(newData.domainIndex))

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

// ========== Key 判断函数 ==========

func isTenantMetaKey(key string) bool {
	return strings.HasSuffix(key, "/meta") && strings.HasPrefix(key, "tenants/")
}

func isServiceDatabaseKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 4 && parts[0] == "tenants" && parts[2] == "database"
}

func isFtpConfigKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 4 && parts[0] == "tenants" && parts[2] == "ftp"
}

func isStorageConfigKey(key string) bool {
	return strings.HasSuffix(key, "/storage") && strings.HasPrefix(key, "tenants/")
}

func isDomainPrimaryKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 4 && parts[0] == "tenants" && parts[2] == "domain" && parts[3] == "primary"
}

func isDomainAliasesKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 4 && parts[0] == "tenants" && parts[2] == "domain" && parts[3] == "aliases"
}

func isDomainInternalKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 4 && parts[0] == "tenants" && parts[2] == "domain" && parts[3] == "internal"
}

func isResolverConfigKey(key string) bool {
	return key == "common/resolver"
}

func (p *Provider) processKey(key, value string, data *tenantData) {
	key = strings.TrimPrefix(key, p.namespace)

	switch {
	case isResolverConfigKey(key):
		p.parseResolverConfig(key, value, data)
	case isTenantMetaKey(key):
		p.parseTenantMeta(key, value, data)
	case isServiceDatabaseKey(key):
		p.parseServiceDatabaseConfig(key, value, data)
	case isFtpConfigKey(key):
		p.parseFtpConfig(key, value, data)
	case isStorageConfigKey(key):
		p.parseStorageConfig(key, value, data)
	case isDomainPrimaryKey(key), isDomainAliasesKey(key), isDomainInternalKey(key):
		p.parseDomainConfig(key, value, data)
	}
}

// ========== 解析函数 ==========

// parseTenantMeta 解析租户元数据
func (p *Provider) parseTenantMeta(key string, value string, data *tenantData) error {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}

	var meta TenantMeta
	if err := json.Unmarshal([]byte(value), &meta); err != nil {
		return err
	}
	meta.TenantID = tenantID
	data.Metas[tenantID] = &meta
	return nil
}

// parseServiceDatabaseConfig 解析服务数据库配置
func (p *Provider) parseServiceDatabaseConfig(key string, value string, data *tenantData) error {
	// /tenants/{tenantId}/database/{serviceCode}
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}
	serviceCode := parts[3]

	var config ServiceDatabaseConfig
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return err
	}
	config.TenantID = tenantID

	if data.Databases[tenantID] == nil {
		data.Databases[tenantID] = make(map[string]*ServiceDatabaseConfig)
	}
	data.Databases[tenantID][serviceCode] = &config
	return nil
}

// parseFtpConfig 解析FTP配置
func (p *Provider) parseFtpConfig(key string, value string, data *tenantData) error {
	// /tenants/{tenantId}/ftp/{username}
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}

	var config FtpConfigDetail
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return err
	}
	config.TenantID = tenantID

	data.Ftps[tenantID] = appendOrUpdateFtpConfig(data.Ftps[tenantID], &config)
	return nil
}

// parseStorageConfig 解析存储配置
func (p *Provider) parseStorageConfig(key string, value string, data *tenantData) error {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}

	var config StorageConfig
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return err
	}
	config.TenantID = int64(tenantID)

	if meta, ok := data.Metas[tenantID]; ok {
		config.Code = meta.Code
		config.Name = meta.Name
	}

	data.Storages[tenantID] = &config
	return nil
}

// parseDomainConfig 解析域名配置
func (p *Provider) parseDomainConfig(key string, value string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	if data.Domains[tenantID] == nil {
		data.Domains[tenantID] = &DomainConfig{}
	}

	domain := data.Domains[tenantID]

	// 根据不同的 key 类型更新对应字段
	if isDomainPrimaryKey(key) {
		domain.Primary = parseStringOrJSON(value)
	} else if isDomainAliasesKey(key) {
		var aliases []string
		json.Unmarshal([]byte(value), &aliases)
		domain.Aliases = aliases
	} else if isDomainInternalKey(key) {
		domain.Internal = parseStringOrJSON(value)
	}

	// 填充租户信息（如果已有）
	if meta, ok := data.Metas[tenantID]; ok {
		domain.TenantID = tenantID
		domain.Code = meta.Code
		domain.Name = meta.Name
	}
}

// parseStringOrJSON 解析字符串值，支持纯字符串或 JSON 字符串格式
// ETCD 中可能存储为 "app.jxt.com" 或 "\"app.jxt.com\""
func parseStringOrJSON(value string) string {
	// 先尝试 JSON 解析
	var jsonStr string
	if err := json.Unmarshal([]byte(value), &jsonStr); err == nil {
		return jsonStr
	}
	// JSON 解析失败，直接使用原始字符串
	return strings.TrimSpace(value)
}

// parseResolverConfig 解析租户识别配置（全局配置）
// 返回 error 以与其他解析函数保持风格一致
// 注意：调用方（processKey）忽略返回值，解析失败时仅跳过该配置
func (p *Provider) parseResolverConfig(key string, value string, data *tenantData) error {
	var config ResolverConfig
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return fmt.Errorf("failed to unmarshal resolver config: %w", err)
	}
	data.Resolver = &config
	return nil
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
	current := p.data.Load().(*tenantData)
	newData := current.copyData()

	key := string(ev.Kv.Key)
	keyStr := strings.TrimPrefix(key, p.namespace)

	switch {
	case isTenantMetaKey(keyStr):
		p.handleTenantMetaChange(ev, keyStr, newData)
	case isServiceDatabaseKey(keyStr):
		p.handleServiceDatabaseChange(ev, keyStr, newData)
	case isFtpConfigKey(keyStr):
		p.handleFtpConfigChange(ev, keyStr, newData)
	case isStorageConfigKey(keyStr):
		p.handleStorageChange(ev, keyStr, newData)
	case isDomainPrimaryKey(keyStr), isDomainAliasesKey(keyStr), isDomainInternalKey(keyStr):
		p.handleDomainChange(ev, keyStr, newData)
	}

	newData.domainIndex = buildDomainIndex(newData) // 重建索引
	p.data.Store(newData)                           // 一次原子替换

	// 同步到缓存
	if p.cache != nil {
		go func() {
			if err := p.cache.Save(newData); err != nil {
				logger.Errorf("tenant provider: failed to sync cache: %v", err)
			}
		}()
	}
}

// ========== Watch 事件处理 ==========

// handleServiceDatabaseChange 处理数据库配置变更
func (p *Provider) handleServiceDatabaseChange(ev *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	serviceCode := parts[3]

	if ev.Type == clientv3.EventTypeDelete {
		if data.Databases[tenantID] != nil {
			delete(data.Databases[tenantID], serviceCode)
		}
	} else {
		var config ServiceDatabaseConfig
		if err := json.Unmarshal(ev.Kv.Value, &config); err != nil {
			logger.Errorf("failed to unmarshal database config: %v", err)
			return
		}
		config.TenantID = tenantID
		if data.Databases[tenantID] == nil {
			data.Databases[tenantID] = make(map[string]*ServiceDatabaseConfig)
		}
		data.Databases[tenantID][serviceCode] = &config
	}
}

// handleFtpConfigChange 处理FTP配置变更
func (p *Provider) handleFtpConfigChange(ev *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	if ev.Type == clientv3.EventTypeDelete {
		username := parts[3]
		data.Ftps[tenantID] = removeFtpConfigByUsername(data.Ftps[tenantID], username)
	} else {
		var config FtpConfigDetail
		if err := json.Unmarshal(ev.Kv.Value, &config); err != nil {
			logger.Errorf("failed to unmarshal ftp config: %v", err)
			return
		}
		config.TenantID = tenantID
		data.Ftps[tenantID] = appendOrUpdateFtpConfig(data.Ftps[tenantID], &config)
	}
}

// handleStorageChange 处理存储配置变更
func (p *Provider) handleStorageChange(ev *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	if ev.Type == clientv3.EventTypeDelete {
		delete(data.Storages, tenantID)
	} else {
		var config StorageConfig
		if err := json.Unmarshal(ev.Kv.Value, &config); err != nil {
			logger.Errorf("failed to unmarshal storage config: %v", err)
			return
		}
		config.TenantID = int64(tenantID)
		if meta, ok := data.Metas[tenantID]; ok {
			config.Code = meta.Code
			config.Name = meta.Name
		}
		data.Storages[tenantID] = &config
	}
}

// handleDomainChange 处理域名配置变更
func (p *Provider) handleDomainChange(ev *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	if data.Domains[tenantID] == nil {
		data.Domains[tenantID] = &DomainConfig{}
	}

	domain := data.Domains[tenantID]

	if ev.Type == clientv3.EventTypeDelete {
		// Handle delete by resetting the specific field
		if isDomainPrimaryKey(key) {
			domain.Primary = ""
		} else if isDomainAliasesKey(key) {
			domain.Aliases = nil
		} else if isDomainInternalKey(key) {
			domain.Internal = ""
		}
	} else {
		// Handle put/update - 使用 parseStringOrJSON 支持 JSON 和纯字符串格式
		value := string(ev.Kv.Value)
		if isDomainPrimaryKey(key) {
			domain.Primary = parseStringOrJSON(value)
		} else if isDomainAliasesKey(key) {
			var aliases []string
			json.Unmarshal(ev.Kv.Value, &aliases)
			domain.Aliases = aliases
		} else if isDomainInternalKey(key) {
			domain.Internal = parseStringOrJSON(value)
		}
	}

	// 填充租户信息
	if meta, ok := data.Metas[tenantID]; ok {
		domain.TenantID = tenantID
		domain.Code = meta.Code
		domain.Name = meta.Name
	}
}

// handleTenantMetaChange 处理租户元数据变更
func (p *Provider) handleTenantMetaChange(ev *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	if ev.Type == clientv3.EventTypeDelete {
		delete(data.Metas, tenantID)
	} else {
		var meta TenantMeta
		if err := json.Unmarshal(ev.Kv.Value, &meta); err != nil {
			logger.Errorf("failed to unmarshal tenant meta: %v", err)
			return
		}
		meta.TenantID = tenantID
		data.Metas[tenantID] = &meta
	}
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

	// domainIndex 在 copyData 时不复制，由调用方重建
	// 因为索引可以从 Domains 派生

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

// GetAllServiceDatabaseConfigs retrieves all service database configurations for a tenant
func (p *Provider) GetAllServiceDatabaseConfigs(tenantID int) (map[string]*ServiceDatabaseConfig, bool) {
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

// GetFtpConfigByUsername retrieves FTP configuration by username across all tenants
func (p *Provider) GetFtpConfigByUsername(username string) (*FtpConfigDetail, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}

	// Iterate through all tenants' FTP configurations
	for _, configs := range data.Ftps {
		for _, cfg := range configs {
			if cfg.Username == username {
				return cfg, true
			}
		}
	}
	return nil, false
}

// GetActiveFtpConfigs retrieves all active FTP configurations for a tenant
func (p *Provider) GetActiveFtpConfigs(tenantID int) ([]*FtpConfigDetail, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}

	configs, ok := data.Ftps[tenantID]
	if !ok {
		return nil, false
	}

	var activeConfigs []*FtpConfigDetail
	for _, cfg := range configs {
		if cfg.Status == "" || cfg.Status == "active" {
			activeConfigs = append(activeConfigs, cfg)
		}
	}

	return activeConfigs, len(activeConfigs) > 0
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

func countServiceDatabases(databases map[int]map[string]*ServiceDatabaseConfig) int {
	count := 0
	for _, services := range databases {
		count += len(services)
	}
	return count
}

func countFtpConfigs(ftps map[int][]*FtpConfigDetail) int {
	count := 0
	for _, configs := range ftps {
		count += len(configs)
	}
	return count
}

// normalizeDomain 规范化域名：小写 + 去除空白
func normalizeDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return ""
	}
	return domain
}

// buildDomainIndex 构建域名反向索引
// 返回 map[domain]tenantID，所有域名已规范化为小写
// 检测并记录域名冲突（不同租户声明相同域名）
func buildDomainIndex(data *tenantData) map[string]int {
	// 预估容量：每个租户约 4 个域名（Primary + 2 Aliases + Internal）
	estimatedSize := len(data.Domains) * 4
	if estimatedSize == 0 {
		estimatedSize = 8 // 最小容量
	}
	index := make(map[string]int, estimatedSize)

	for tenantID, cfg := range data.Domains {
		// Primary 域名
		if cfg.Primary != "" {
			domain := normalizeDomain(cfg.Primary)
			if domain != "" {
				if existingID, exists := index[domain]; exists && existingID != tenantID {
					logger.Warnf("domain conflict: %s claimed by tenant %d and %d, using %d",
						domain, existingID, tenantID, tenantID)
				}
				index[domain] = tenantID
			}
		}

		// Aliases 别名列表
		for _, alias := range cfg.Aliases {
			if alias != "" {
				domain := normalizeDomain(alias)
				if domain != "" {
					if existingID, exists := index[domain]; exists && existingID != tenantID {
						logger.Warnf("domain conflict: %s claimed by tenant %d and %d, using %d",
							domain, existingID, tenantID, tenantID)
					}
					index[domain] = tenantID
				}
			}
		}

		// Internal 内网域名
		if cfg.Internal != "" {
			domain := normalizeDomain(cfg.Internal)
			if domain != "" {
				if existingID, exists := index[domain]; exists && existingID != tenantID {
					logger.Warnf("domain conflict: %s claimed by tenant %d and %d, using %d",
						domain, existingID, tenantID, tenantID)
				}
				index[domain] = tenantID
			}
		}
	}

	return index
}

// GetTenantIDByDomain 根据域名查找租户ID
// 查找复杂度 O(1)，无锁，线程安全
// 支持 Primary、Aliases、Internal 三种域名类型
// 域名匹配不区分大小写（RFC 4343）
// 注意：仅支持精确匹配，不支持通配符域名（如 *.tenant.com）
func (p *Provider) GetTenantIDByDomain(domain string) (int, bool) {
	if domain == "" {
		return 0, false
	}

	normalizedDomain := normalizeDomain(domain)
	if normalizedDomain == "" {
		return 0, false
	}

	// O(1) 查找，无锁
	data := p.data.Load().(*tenantData)
	tenantID, ok := data.domainIndex[normalizedDomain]
	return tenantID, ok
}

