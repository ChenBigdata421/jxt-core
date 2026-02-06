# jxt-core 租户组件适配指南

**文档版本**: v2.0
**创建日期**: 2026-02-03
**作者**: 开发团队
**状态**: 适配指南完成
**重要**: 项目未上生产环境，无需考虑向后兼容，可直接删除旧代码

---

## 1. 概述

本文档说明 jxt-core/sdk/pkg/tenant 组件需要进行的修改，以适配 tenant-service 的新 ETCD Key 格式。

**核心变更**：
- 服务数据库配置：从单个配置变为服务级多配置
- FTP 配置：从单个配置变为多配置数组
- 新增域名配置支持
- 删除所有索引缓存（索引后续会删除）

---

## 2. 核心变更

### 2.1 Watch 前缀变更

| 组件 | 旧前缀 | 新前缀 |
|-----|-------|-------|
| 租户配置 | `jxt//tenants/` | `jxt/tenants/` |
| 服务数据库 | `jxt//jxt/config/tenants/` | `jxt/tenants/` |
| FTP用户索引 | `jxt//ftp-users/` | ❌ 删除（不再缓存） |
| Host索引 | `jxt//hosts/` | ❌ 删除（不再缓存） |
| 公共配置 | `jxt//common/` | `jxt/common/` |

### 2.2 数据结构变更

**服务数据库配置**：
- 从单个配置变为服务级多配置
- Key 格式：`jxt/tenants/{id}/database/{serviceCode}`

**FTP 配置**：
- 从单个配置变为多配置数组
- Key 格式：`jxt/tenants/{id}/ftp/{username}`

**域名配置**（新增）：
- Key 格式：`jxt/tenants/{id}/domain/primary`, `jxt/tenants/{id}/domain/aliases`, `jxt/tenants/{id}/domain/internal`

---

## 3. 文件修改清单

| 文件 | 修改类型 | 修改内容 |
|-----|---------|---------|
| `provider/models.go` | 重构 | 删除旧结构，添加新结构，删除索引结构 |
| `provider/provider.go` | 重构 | 重构 Watch 逻辑，删除旧 API，添加新 API |
| `database/cache.go` | 重构 | 删除旧 API，只保留服务级 API |
| `ftp/cache.go` | 重构 | 删除旧 API，返回数组，添加查询方法 |
| `database/config.go` | 修改 | 更新数据结构 |
| `ftp/config.go` | 修改 | 添加新字段 |
| `provider/*_test.go` | 修改 | 更新测试用例 |

---

## 4. 数据结构修改

### 4.1 provider/models.go

```go
package provider

// TenantMeta 租户元数据（保持不变）
type TenantMeta struct {
	TenantID    int    `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Status      string `json:"status"`      // active, inactive, suspended
	BillingPlan string `json:"billingPlan"` // optional
}

// ========== 新增：服务级数据库配置 ==========

// ServiceDatabaseConfig 服务级数据库配置
type ServiceDatabaseConfig struct {
	TenantID    int    `json:"tenantId"`
	ServiceCode string `json:"serviceCode"` // evidence-command, evidence-query, file-storage, security-management
	Driver      string `json:"driver"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	SSLMode     string `json:"sslMode"`
	MaxOpenConns int   `json:"maxOpenConns"`
	MaxIdleConns int   `json:"maxIdleConns"`
}

// ========== 新增：FTP配置详情 ==========

// FtpConfigDetail FTP配置详情（支持多配置）
type FtpConfigDetail struct {
	TenantID        int    `json:"tenantId"`
	Username        string `json:"username"`
	PasswordHash    string `json:"passwordHash"`
	Description     string `json:"description"`  // FTP 配置描述
	Status          string `json:"status"`        // active, inactive
	HomeDirectory   string `json:"homeDirectory"`
	WritePermission bool   `json:"writePermission"`
}

// ========== 新增：域名配置 ==========

// DomainConfig 租户域名配置
type DomainConfig struct {
	TenantID int      `json:"tenantId"`
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Primary  string   `json:"primary"`  // 主域名
	Aliases  []string `json:"aliases"`  // 别名列表
	Internal string   `json:"internal"` // 内部域名
}

// ========== 修改：tenantData 支持多配置 ==========

// tenantData 租户数据（新格式）
type tenantData struct {
	Metas     map[int]*TenantMeta                      `json:"metas"`
	Databases map[int]map[string]*ServiceDatabaseConfig `json:"databases"` // tenantID -> serviceCode -> config
	Ftps      map[int][]*FtpConfigDetail                 `json:"ftps"`      // tenantID -> configs[]
	Storages  map[int]*StorageConfig                     `json:"storages"`
	Domains   map[int]*DomainConfig                       `json:"domains"`   // 新增：域名配置
}

// ========== 辅助方法 ==========

// copyData 创建 tenantData 的深拷贝
func (d *tenantData) copyData() *tenantData {
	newData := &tenantData{
		Metas:    make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:     make(map[int][]*FtpConfigDetail),
		Storages: make(map[int]*StorageConfig),
		Domains:  make(map[int]*DomainConfig),
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

// appendOrUpdateFtpConfig 添加或更新FTP配置
func appendOrUpdateFtpConfig(configs []*FtpConfigDetail, newConfig *FtpConfigDetail) []*FtpConfigDetail {
	for i, cfg := range configs {
		if cfg.Username == newConfig.Username {
			configs[i] = newConfig
			return configs
		}
	}
	return append(configs, newConfig)
}

// removeFtpConfigByUsername 从配置列表中移除指定用户名的配置
func removeFtpConfigByUsername(configs []*FtpConfigDetail, username string) []*FtpConfigDetail {
	for i, cfg := range configs {
		if cfg.Username == username {
			return append(configs[:i], configs[i+1:]...)
		}
	}
	return configs
}
```

---

## 5. Provider 核心修改

### 5.1 provider/provider.go - 删除和新增

```go
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

const (
	watchPrefixTenants = "jxt/tenants/"
	watchPrefixCommon  = "jxt/common/"
)

// Provider 租户配置提供者
type Provider struct {
	client      *clientv3.Client
	namespace   string
	data        atomic.Value // *tenantData
	mu          sync.RWMutex
	watchCtx    context.Context
	watchCancel context.CancelFunc
	running     atomic.Bool
}

// ========== 初始化 ==========

// NewProvider 创建 Provider
func NewProvider(client *clientv3.Client, opts ...Option) *Provider {
	p := &Provider{
		client:    client,
		namespace: "jxt/",
		data:      atomic.Value{},
		running:   atomic.Bool{},
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

// ========== 数据加载 ==========

// LoadAll 从 etcd 加载所有配置
func (p *Provider) LoadAll(ctx context.Context) error {
	prefixes := []string{
		watchPrefixTenants,
		watchPrefixCommon,
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
			return fmt.Errorf("failed to load prefix %s: %w", prefix, err)
		}

		for _, kv := range resp.Kvs {
			p.processKeyValue(kv.Key, kv.Value, newData)
		}
	}

	p.data.Store(newData)
	logger.Info("tenant data loaded from etcd",
		zap.Int("tenants", len(newData.Metas)),
		zap.Int("databases", countServiceDatabases(newData.Databases)),
		zap.Int("ftps", countFtpConfigs(newData.Ftps)))
	return nil
}

// processKeyValue 处理单个 Key-Value 对
func (p *Provider) processKeyValue(key, value []byte, data *tenantData) {
	keyStr := string(key)
	keyStr = strings.TrimPrefix(keyStr, p.namespace)

	switch {
	case isTenantMetaKey(keyStr):
		p.parseTenantMeta(keyStr, value, data)

	case isServiceDatabaseKey(keyStr):
		p.parseServiceDatabaseConfig(keyStr, value, data)

	case isFtpConfigKey(keyStr):
		p.parseFtpConfig(keyStr, value, data)

	case isStorageConfigKey(keyStr):
		p.parseStorageConfig(keyStr, value, data)

	case isDomainPrimaryKey(keyStr), isDomainAliasesKey(keyStr), isDomainInternalKey(keyStr):
		p.parseDomainConfig(keyStr, value, data)
	}
}

// ========== Key 判断函数 ==========

func isTenantMetaKey(key string) bool {
	return strings.HasSuffix(key, "/meta") && strings.Contains(key, "/tenants/")
}

func isServiceDatabaseKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 5 && parts[2] == "tenants" && parts[4] == "database"
}

func isFtpConfigKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 5 && parts[2] == "tenants" && parts[4] == "ftp"
}

func isStorageConfigKey(key string) bool {
	return strings.HasSuffix(key, "/storage") && strings.Contains(key, "/tenants/")
}

func isDomainPrimaryKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 5 && parts[2] == "tenants" && parts[4] == "domain" && parts[5] == "primary"
}

func isDomainAliasesKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 5 && parts[2] == "tenants" && parts[4] == "domain" && parts[5] == "aliases"
}

func isDomainInternalKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 5 && parts[2] == "tenants" && parts[4] == "domain" && parts[5] == "internal"
}

// ========== 解析函数 ==========

// parseTenantMeta 解析租户元数据
func (p *Provider) parseTenantMeta(key string, value []byte, data *tenantData) error {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[2])
	if err != nil {
		return err
	}

	var meta TenantMeta
	if err := json.Unmarshal(value, &meta); err != nil {
		return err
	}
	data.Metas[tenantID] = &meta
	return nil
}

// parseServiceDatabaseConfig 解析服务数据库配置
func (p *Provider) parseServiceDatabaseConfig(key string, value []byte, data *tenantData) error {
	// /tenants/{tenantId}/database/{serviceCode}
	parts := strings.Split(key, "/")
	tenantID, _ := strconv.Atoi(parts[2])
	serviceCode := parts[4]

	var config ServiceDatabaseConfig
	if err := json.Unmarshal(value, &config); err != nil {
		return err
	}

	if data.Databases[tenantID] == nil {
		data.Databases[tenantID] = make(map[string]*ServiceDatabaseConfig)
	}
	data.Databases[tenantID][serviceCode] = &config
	return nil
}

// parseFtpConfig 解析FTP配置
func (p *Provider) parseFtpConfig(key string, value []byte, data *tenantData) error {
	// /tenants/{tenantId}/ftp/{username}
	parts := strings.Split(key, "/")
	tenantID, _ := strconv.Atoi(parts[2])

	var config FtpConfigDetail
	if err := json.Unmarshal(value, &config); err != nil {
		return err
	}

	data.Ftps[tenantID] = appendOrUpdateFtpConfig(data.Ftps[tenantID], &config)
	return nil
}

// parseStorageConfig 解析存储配置
func (p *Provider) parseStorageConfig(key string, value []byte, data *tenantData) error {
	parts := strings.Split(key, "/")
	tenantID, _ := strconv.Atoi(parts[2])

	var config StorageConfig
	if err := json.Unmarshal(value, &config); err != nil {
		return err
	}

	data.Storages[tenantID] = &config
	return nil
}

// parseDomainConfig 解析域名配置
func (p *Provider) parseDomainConfig(key string, value []byte, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, _ := strconv.Atoi(parts[2])

	if data.Domains[tenantID] == nil {
		data.Domains[tenantID] = &DomainConfig{}
	}

	domain := data.Domains[tenantID]

	// 根据不同的 key 类型更新对应字段
	if isDomainPrimaryKey(key) {
		var primary string
		json.Unmarshal(value, &primary)
		domain.Primary = primary
	} else if isDomainAliasesKey(key) {
		var aliases []string
		json.Unmarshal(value, &aliases)
		domain.Aliases = aliases
	} else if isDomainInternalKey(key) {
		var internal string
		json.Unmarshal(value, &internal)
		domain.Internal = internal
	}

	// 填充租户信息（如果已有）
	if meta, ok := data.Metas[tenantID]; ok {
		domain.TenantID = tenantID
		domain.Code = meta.Code
		domain.Name = meta.Name
	}
}

// ========== Watch 事件处理 ==========

// StartWatch 启动 Watch
func (p *Provider) StartWatch(ctx context.Context) error {
	p.watchCtx, p.watchCancel = context.WithCancel(ctx)

	watchChan := p.client.Watch(p.watchCtx, watchPrefixTenants, clientv3.WithPrefix())

	p.running.Store(true)
	logger.Info("tenant provider watch started", zap.String("prefix", watchPrefixTenants))

	go p.watchLoop(watchChan)
	return nil
}

// watchLoop Watch 循环（复用现有重连机制）
func (p *Provider) watchLoop(initialWatchChan clientv3.WatchChan) {
	// ... 保持现有实现 ...
}

// handleWatchEvent 处理 Watch 事件
func (p *Provider) handleWatchEvent(event *clientv3.Event) {
	key := string(event.Kv.Key)
	keyStr := strings.TrimPrefix(key, p.namespace)

	newData := p.data.Load().(*tenantData)
	updatedData := newData.copyData()

	switch {
	case isTenantMetaKey(keyStr):
		p.handleTenantMetaChange(event, keyStr, updatedData)

	case isServiceDatabaseKey(keyStr):
		p.handleServiceDatabaseChange(event, keyStr, updatedData)

	case isFtpConfigKey(keyStr):
		p.handleFtpConfigChange(event, keyStr, updatedData)

	case isDomainPrimaryKey(keyStr), isDomainAliasesKey(keyStr), isDomainInternalKey(keyStr):
		p.handleDomainChange(event, keyStr, updatedData)
	}

	p.data.Store(updatedData)
}

// handleServiceDatabaseChange 处理数据库配置变更
func (p *Provider) handleServiceDatabaseChange(event *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, _ := strconv.Atoi(parts[2])
	serviceCode := parts[4]

	if event.Type == clientv3.EventTypeDelete {
		if data.Databases[tenantID] != nil {
			delete(data.Databases[tenantID], serviceCode)
		}
	} else {
		var config ServiceDatabaseConfig
		if err := json.Unmarshal(event.Kv.Value, &config); err != nil {
			logger.Error("failed to unmarshal database config", zap.Error(err))
			return
		}
		if data.Databases[tenantID] == nil {
			data.Databases[tenantID] = make(map[string]*ServiceDatabaseConfig)
		}
		data.Databases[tenantID][serviceCode] = &config
	}
}

// handleFtpConfigChange 处理FTP配置变更
func (p *Provider) handleFtpConfigChange(event *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, _ := strconv.Atoi(parts[2])

	if event.Type == clientv3.EventTypeDelete {
		if len(parts) >= 5 {
			username := parts[4]
			data.Ftps[tenantID] = removeFtpConfigByUsername(data.Ftps[tenantID], username)
		}
	} else {
		var config FtpConfigDetail
		if err := json.Unmarshal(event.Kv.Value, &config); err != nil {
			logger.Error("failed to unmarshal ftp config", zap.Error(err))
			return
		}
		data.Ftps[tenantID] = appendOrUpdateFtpConfig(data.Ftps[tenantID], &config)
	}
}

// handleDomainChange 处理域名配置变更
func (p *Provider) handleDomainChange(event *clientv3.Event, key string, data *tenantData) {
	// 重新解析该租户的所有域名相关 key
	// 注意：这里简化处理，实际可能需要更复杂的逻辑
	p.parseDomainConfig(key, event.Kv.Value, data)
}

// handleTenantMetaChange 处理租户元数据变更（复用现有逻辑）
func (p *Provider) handleTenantMetaChange(event *clientv3.Event, key string, data *tenantData) {
	// ... 保持现有实现 ...
}

// ========== 新 API ==========

// GetServiceDatabaseConfig 获取指定服务的数据库配置
func (p *Provider) GetServiceDatabaseConfig(tenantID int, serviceCode string) (*ServiceDatabaseConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data.Databases[tenantID] == nil {
		return nil, false
	}
	config, ok := data.Databases[tenantID][serviceCode]
	return config, ok
}

// GetAllServiceDatabaseConfigs 获取租户所有服务的数据库配置
func (p *Provider) GetAllServiceDatabaseConfigs(tenantID int) (map[string]*ServiceDatabaseConfig, bool) {
	data := p.data.Load().(*tenantData)
	configs, ok := data.Databases[tenantID]
	return configs, ok
}

// GetFtpConfigs 获取租户所有FTP配置
func (p *Provider) GetFtpConfigs(tenantID int) ([]*FtpConfigDetail, bool) {
	data := p.data.Load().(*tenantData)
	configs, ok := data.Ftps[tenantID]
	return configs, ok
}

// GetFtpConfigByUsername 通过用户名获取FTP配置（遍历所有租户）
func (p *Provider) GetFtpConfigByUsername(username string) (*FtpConfigDetail, bool) {
	data := p.data.Load().(*tenantData)

	// 遍历所有租户的 FTP 配置
	for tenantID, configs := range data.Ftps {
		for _, cfg := range configs {
			if cfg.Username == username {
				return cfg, true
			}
		}
	}
	return nil, false
}

// GetActiveFtpConfigs 获取租户所有活跃的FTP配置
func (p *Provider) GetActiveFtpConfigs(tenantID int) ([]*FtpConfigDetail, bool) {
	data := p.data.Load().(*tenantData)
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

// GetDomainConfig 获取域名配置
func (p *Provider) GetDomainConfig(tenantID int) (*DomainConfig, bool) {
	data := p.data.Load().(*tenantData)
	config, ok := data.Domains[tenantID]
	return config, ok
}

// GetTenantMeta 获取租户元数据（保持现有）
func (p *Provider) GetTenantMeta(tenantID int) (*TenantMeta, bool) {
	data := p.data.Load().(*tenantData)
	meta, ok := data.Metas[tenantID]
	return meta, ok
}

// GetStorageConfig 获取存储配置（保持现有）
func (p *Provider) GetStorageConfig(tenantID int) (*StorageConfig, bool) {
	data := p.data.Load().(*tenantData)
	config, ok := data.Storages[tenantID]
	return config, ok
}

// ========== 辅助函数 ==========

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
```

---

## 6. Cache Layer 修改

### 6.1 database/cache.go

```go
package database

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache 数据库配置缓存
type Cache struct {
	provider *provider.Provider
}

// NewCache 创建数据库配置缓存
func NewCache(prov *provider.Provider) *Cache {
	return &Cache{provider: prov}
}

// GetByService 获取指定服务的数据库配置
func (c *Cache) GetByService(ctx context.Context, tenantID int, serviceCode string) (*TenantDatabaseConfig, error) {
	cfg, ok := c.provider.GetServiceDatabaseConfig(tenantID, serviceCode)
	if !ok {
		return nil, fmt.Errorf("database config not found for tenant %d, service %s", tenantID, serviceCode)
	}

	meta, ok := c.provider.GetTenantMeta(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", tenantID)
	}

	return &TenantDatabaseConfig{
		TenantID:    cfg.TenantID,
		Code:        meta.Code,
		Name:        meta.Name,
		ServiceCode: cfg.ServiceCode,
		Driver:      cfg.Driver,
		Host:        cfg.Host,
		Port:        cfg.Port,
		DbName:      cfg.Database,
		Username:    cfg.Username,
		SSLMode:     cfg.SSLMode,
		MaxOpenConns: cfg.MaxOpenConns,
		MaxIdleConns: cfg.MaxIdleConns,
	}, nil
}

// GetAllServices 获取租户所有服务的数据库配置
func (c *Cache) GetAllServices(ctx context.Context, tenantID int) (map[string]*TenantDatabaseConfig, error) {
	configs, ok := c.provider.GetAllServiceDatabaseConfigs(tenantID)
	if !ok {
		return nil, fmt.Errorf("no database configs found for tenant %d", tenantID)
	}

	meta, ok := c.provider.GetTenantMeta(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", tenantID)
	}

	result := make(map[string]*TenantDatabaseConfig)
	for serviceCode, cfg := range configs {
		result[serviceCode] = &TenantDatabaseConfig{
			TenantID:    cfg.TenantID,
			Code:        meta.Code,
			Name:        meta.Name,
			ServiceCode: cfg.ServiceCode,
			Driver:      cfg.Driver,
			Host:        cfg.Host,
			Port:        cfg.Port,
			DbName:      cfg.Database,
			Username:    cfg.Username,
			SSLMode:     cfg.SSLMode,
			MaxOpenConns: cfg.MaxOpenConns,
			MaxIdleConns: cfg.MaxIdleConns,
		}
	}

	return result, nil
}
```

### 6.2 ftp/cache.go

```go
package ftp

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache FTP配置缓存
type Cache struct {
	provider *provider.Provider
}

// NewCache 创建 FTP 配置缓存
func NewCache(prov *provider.Provider) *Cache {
	return &Cache{provider: prov}
}

// GetAll 获取租户所有FTP配置
func (c *Cache) GetAll(ctx context.Context, tenantID int) ([]*TenantFtpConfigDetail, error) {
	configs, ok := c.provider.GetFtpConfigs(tenantID)
	if !ok {
		return nil, fmt.Errorf("no ftp configs found for tenant %d", tenantID)
	}

	meta, ok := c.provider.GetTenantMeta(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", tenantID)
	}

	result := make([]*TenantFtpConfigDetail, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, &TenantFtpConfigDetail{
			TenantID:     cfg.TenantID,
			Code:         meta.Code,
			Name:         meta.Name,
			Username:     cfg.Username,
			PasswordHash: cfg.PasswordHash,
			Description:  cfg.Description,
			Status:       cfg.Status,
		})
	}

	return result, nil
}

// GetByUsername 通过用户名获取FTP配置
func (c *Cache) GetByUsername(ctx context.Context, username string) (*TenantFtpConfigDetail, error) {
	cfg, ok := c.provider.GetFtpConfigByUsername(username)
	if !ok {
		return nil, fmt.Errorf("ftp config not found for username %s", username)
	}

	meta, ok := c.provider.GetTenantMeta(cfg.TenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", cfg.TenantID)
	}

	return &TenantFtpConfigDetail{
		TenantID:     cfg.TenantID,
		Code:         meta.Code,
		Name:         meta.Name,
		Username:     cfg.Username,
		PasswordHash: cfg.PasswordHash,
		Description:  cfg.Description,
		Status:       cfg.Status,
	}, nil
}

// GetActive 获取租户所有活跃的FTP配置
func (c *Cache) GetActive(ctx context.Context, tenantID int) ([]*TenantFtpConfigDetail, error) {
	configs, ok := c.provider.GetActiveFtpConfigs(tenantID)
	if !ok {
		return nil, fmt.Errorf("no active ftp configs found for tenant %d", tenantID)
	}

	meta, ok := c.provider.GetTenantMeta(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", tenantID)
	}

	result := make([]*TenantFtpConfigDetail, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, &TenantFtpConfigDetail{
			TenantID:     cfg.TenantID,
			Code:         meta.Code,
			Name:         meta.Name,
			Username:     cfg.Username,
			PasswordHash: cfg.PasswordHash,
			Description:  cfg.Description,
			Status:       cfg.Status,
		})
	}

	return result, nil
}
```

### 6.3 database/config.go

```go
package database

// TenantDatabaseConfig 租户数据库配置
type TenantDatabaseConfig struct {
	// 租户信息
	TenantID int    `json:"tenant_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`

	// 服务信息
	ServiceCode string `json:"service_code"`

	// 数据库连接参数
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	DbName   string `json:"db_name"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSLMode  string `json:"ssl_mode"`

	// 连接池设置
	MaxOpenConns int `json:"max_open_conns"`
	MaxIdleConns int `json:"max_idle_conns"`
}

// ========== 删除旧结构 ==========
// 删除旧的 DatabaseConfig（单配置版本）
```

### 6.4 ftp/config.go

```go
package ftp

// TenantFtpConfigDetail FTP配置详情
type TenantFtpConfigDetail struct {
	TenantID     int    `json:"tenant_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Description  string `json:"description"`  // 新增
	Status       string `json:"status"`        // 新增
}

// ========== 删除旧结构 ==========
// 删除旧的 TenantFtpConfig（单配置版本）
```

---

## 7. 使用示例

### 7.1 数据库配置获取

```go
// 新代码 - 指定服务
dbCache := tenant_database.NewCache(provider)
cfg, err := dbCache.GetByService(ctx, tenantID, "evidence-command")

// 新代码 - 获取所有服务配置
configs, err := dbCache.GetAllServices(ctx, tenantID)
evidenceCommandCfg := configs["evidence-command"]
evidenceQueryCfg := configs["evidence-query"]
```

### 7.2 FTP配置获取

```go
// 新代码 - 获取所有FTP配置
ftpCache := tenant_ftp.NewCache(provider)
configs, err := ftpCache.GetAll(ctx, tenantID)

// 新代码 - 仅获取活跃配置
configs, err := ftpCache.GetActive(ctx, tenantID)

// 新代码 - 通过用户名查找
cfg, err := ftpCache.GetByUsername(ctx, "tenant1_sales_ftp")
```

---

## 8. 测试用例

```go
// provider/provider_test.go

func TestServiceDatabaseConfig(t *testing.T) {
	provider := setupTestProvider()

	// Test GetServiceDatabaseConfig
	cfg, ok := provider.GetServiceDatabaseConfig(1, "evidence-command")
	assert.True(t, ok)
	assert.Equal(t, "evidence-command", cfg.ServiceCode)

	// Test GetAllServiceDatabaseConfigs
	configs, ok := provider.GetAllServiceDatabaseConfigs(1)
	assert.True(t, ok)
	assert.GreaterOrEqual(t, len(configs), 1)
}

func TestFtpConfigByUsername(t *testing.T) {
	provider := setupTestProvider()

	// Test GetFtpConfigByUsername（遍历查找）
	cfg, ok := provider.GetFtpConfigByUsername("tenant1_sales_ftp")
	assert.True(t, ok)
	assert.Equal(t, "tenant1_sales_ftp", cfg.Username)
}

func TestGetActiveFtpConfigs(t *testing.T) {
	provider := setupTestProvider()

	// Test GetActiveFtpConfigs
	configs, ok := provider.GetActiveFtpConfigs(1)
	assert.True(t, ok)

	for _, cfg := range configs {
		assert.Equal(t, "active", cfg.Status)
	}
}

func TestDomainConfig(t *testing.T) {
	provider := setupTestProvider()

	// Test GetDomainConfig
	domain, ok := provider.GetDomainConfig(1)
	assert.True(t, ok)
	assert.NotEmpty(t, domain.Primary)
	assert.NotNil(t, domain.Aliases)
}
```

---

## 9. 迁移清单

### 9.1 内部模块

- [ ] `provider/models.go` - 重构数据结构，删除旧结构，添加新结构
- [ ] `provider/provider.go` - 重构 Watch 和事件处理，删除旧 API，添加新 API
- [ ] `database/cache.go` - 删除旧 API，只保留服务级 API
- [ ] `ftp/cache.go` - 删除旧 API，返回数组，添加查询方法
- [ ] `database/config.go` - 更新数据结构，删除旧结构
- [ ] `ftp/config.go` - 添加新字段，删除旧结构

### 9.2 测试文件

- [ ] `provider/provider_test.go` - 更新测试用例
- [ ] `database/cache_test.go` - 更新测试用例
- [ ] `ftp/cache_test.go` - 更新测试用例

### 9.3 文档

- [ ] `README.md` - 更新使用示例
- [ ] API 文档 - 更新 API 说明

---

## 10. 边界情况处理

### 10.1 数据库配置不存在

当服务未配置数据库时，返回 `(nil, false)`：

```go
cfg, ok := provider.GetServiceDatabaseConfig(tenantID, "evidence-command")
if !ok {
    return errors.New("service database not configured")
}
```

### 10.2 FTP 用户名查找

通过遍历所有租户查找（时间复杂度 O(n)）：

```go
func (p *Provider) GetFtpConfigByUsername(username string) (*FtpConfigDetail, bool) {
    data := p.data.Load().(*tenantData)

    // 遍历所有租户的 FTP 配置
    for tenantID, configs := range data.Ftps {
        for _, cfg := range configs {
            if cfg.Username == username {
                return cfg, true
            }
        }
    }
    return nil, false
}
```

---

## 11. 与 tenant-service 部署协调

### 11.1 部署顺序

```
1. 停止所有消费服务
   └─ evidence-command, evidence-query, file-storage

2. 部署新版本 tenant-service
   └─ 启动后自动发布新格式数据到 etcd

3. 清空 etcd 旧数据
   └─ etcdctl del "" --prefix --from-key

4. 验证新格式数据正确
   └─ etcdctl get jxt/tenants/1/meta
   └─ etcdctl get jxt/tenants/1/database/evidence-command

5. 部署新版本 jxt-core
   └─ 更新依赖 jxt-core 的服务

6. 重启消费服务
   └─ 验证服务正常启动
```

### 11.2 依赖关系

| 服务 | 依赖 | 部署顺序 |
|-----|------|----------|
| tenant-service | etcd | 1 |
| jxt-core | etcd | 2 |
| evidence-command | jxt-core | 3 |
| evidence-query | jxt-core | 3 |
| file-storage | jxt-core | 3 |

### 11.3 验证步骤

```bash
# 1. 验证 tenant-service 已部署新格式
etcdctl get jxt/tenants/1/meta
etcdctl get jxt/tenants/1/database/evidence-command
etcdctl get jxt/tenants/1/ftp/tenant1_sales_ftp

# 2. 验证 jxt-core 能正确读取
# 检查服务日志中的数据加载信息

# 3. 验证消费服务正常启动
docker-compose logs evidence-command | grep "database connected"
docker-compose logs file-storage | grep "ftp"
```

---

## 12. 性能影响评估

### 12.1 内存占用

| 项目 | 旧格式 | 新格式 | 变化 |
|------|-------|-------|------|
| 单租户数据 | ~2KB | ~3KB | +50% |
| 100 租户 | ~200KB | ~300KB | +50% |
| 1000 租户 | ~2MB | ~3MB | +50% |

### 12.2 查询性能

| 操作 | 旧格式 | 新格式 | 变化 |
|------|-------|-------|------|
| 获取租户元数据 | O(1) | O(1) | 无变化 |
| 获取服务数据库 | O(1) | O(1) | 无变化 |
| 获取所有 FTP | O(1) | O(1) | 无变化 |
| 通过用户名查 FTP | O(1)（索引） | O(n)（遍历） | **回退** |

**说明**：
- 删除索引缓存简化了代码
- FTP 用户名查找从 O(1) 回退到 O(n)，但 FTP 配置数量通常很少（<10），影响可忽略
- 对于 n=10 的情况，遍历 10 个配置只需几微秒

---

## 13. 常见问题

### Q1: 为什么不缓存索引？

**A**: 索引数据在 tenant-service 中用于快速查找和一致性保证，但 jxt-core 作为消费者只需要读取配置数据。索引由 tenant-service 维护，jxt-core 不需要重复缓存。

### Q2: FTP 用户名查找变慢了吗？

**A**: 理论上从 O(1) 变为 O(n)，但实际影响很小。租户的 FTP 配置数量通常很少（1-5 个），遍历所有租户的 FTP 配置只需要几微秒。如果未来 FTP 配置数量增加到 100+，可以考虑重新引入索引。

### Q3: 为什么需要 copyData()？

**A**: `atomic.Value` 存储的指针不能直接修改。更新数据时需要创建副本，修改后原子替换，保证并发读的安全性。

### Q4: 域名配置为什么是三个 Key？

**A**: 域名配置分为三个独立部分：
- `primary` - 单个字符串，主域名
- `aliases` - JSON 数组，多个备用域名
- `internal` - 单个字符串，内部调用域名

这样设计使得每个部分可以独立更新，不需要每次都传输完整配置。

---

**文档结束**
