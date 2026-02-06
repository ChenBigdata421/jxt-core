# Tenant Provider ETCD Adaptation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Adapt `sdk/pkg/tenant/provider` to support tenant-service's new ETCD key format, removing index caching and adding domain configuration support.

**Architecture:**
- Replace single database config with service-level multi-config
- Replace single FTP config with array of configs
- Add domain configuration support
- Remove all index caching structures (they will be deleted from ETCD)
- No backward compatibility needed (project not in production)

**Tech Stack:** Go 1.23+, ETCD client v3, atomic.Value for thread-safe storage

---

## Task 1: Add New Data Structures to models.go

**Files:**
- Modify: `sdk/pkg/tenant/provider/models.go`

**Step 1: Write the failing test**

Create a new test file `sdk/pkg/tenant/provider/models_test.go`:

```go
package provider

import (
	"testing"
)

func TestServiceDatabaseConfig(t *testing.T) {
	cfg := ServiceDatabaseConfig{
		TenantID:    1,
		ServiceCode: "evidence-command",
		Driver:      "mysql",
		Host:        "localhost",
		Port:        3306,
		Database:    "evidence_cmd",
	}

	if cfg.ServiceCode != "evidence-command" {
		t.Errorf("expected ServiceCode to be evidence-command, got %s", cfg.ServiceCode)
	}
}

func TestFtpConfigDetail(t *testing.T) {
	cfg := FtpConfigDetail{
		TenantID:     1,
		Username:     "tenant1_ftp",
		PasswordHash: "hash",
		Description:  "Main FTP",
		Status:       "active",
	}

	if cfg.Description != "Main FTP" {
		t.Errorf("expected Description to be 'Main FTP', got %s", cfg.Description)
	}
	if cfg.Status != "active" {
		t.Errorf("expected Status to be 'active', got %s", cfg.Status)
	}
}

func TestDomainConfig(t *testing.T) {
	cfg := DomainConfig{
		TenantID: 1,
		Code:     "tenant1",
		Name:     "Tenant 1",
		Primary:  "tenant1.example.com",
		Aliases:  []string{"www.tenant1.example.com"},
		Internal: "tenant1.internal",
	}

	if cfg.Primary != "tenant1.example.com" {
		t.Errorf("expected Primary to be 'tenant1.example.com', got %s", cfg.Primary)
	}
	if len(cfg.Aliases) != 1 {
		t.Errorf("expected 1 alias, got %d", len(cfg.Aliases))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/tenant/provider/ -v -run TestServiceDatabaseConfig`
Expected: FAIL with "undefined: ServiceDatabaseConfig"

**Step 3: Add new structures to models.go**

Add to `sdk/pkg/tenant/provider/models.go`:

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./sdk/pkg/tenant/provider/ -v -run "TestServiceDatabaseConfig|TestFtpConfigDetail|TestDomainConfig"`
Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/models.go sdk/pkg/tenant/provider/models_test.go
git commit -m "feat(tenant): add new data structures for service-level config"
```

---

## Task 2: Refactor tenantData Structure

**Files:**
- Modify: `sdk/pkg/tenant/provider/models.go`
- Test: `sdk/pkg/tenant/provider/models_test.go`

**Step 1: Update tenantData structure**

Modify the `tenantData` struct in `models.go`:

```go
// tenantData 租户数据（新格式）
type tenantData struct {
	Metas     map[int]*TenantMeta                      `json:"metas"`
	Databases map[int]map[string]*ServiceDatabaseConfig `json:"databases"` // tenantID -> serviceCode -> config
	Ftps      map[int][]*FtpConfigDetail                 `json:"ftps"`      // tenantID -> configs[]
	Storages  map[int]*StorageConfig                     `json:"storages"`
	Domains   map[int]*DomainConfig                       `json:"domains"`   // 新增：域名配置
}
```

**Step 2: Add copyData method to tenantData**

Add to `models.go`:

```go
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
```

**Step 3: Add FTP helper functions**

Add to `models.go`:

```go
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

**Step 4: Write tests for helper functions**

Add to `models_test.go`:

```go
func TestAppendOrUpdateFtpConfig(t *testing.T) {
	configs := []*FtpConfigDetail{
		{Username: "user1", Description: "First"},
		{Username: "user2", Description: "Second"},
	}

	// Test update
	newConfig := &FtpConfigDetail{Username: "user1", Description: "Updated"}
	result := appendOrUpdateFtpConfig(configs, newConfig)

	if len(result) != 2 {
		t.Errorf("expected 2 configs, got %d", len(result))
	}
	if result[0].Description != "Updated" {
		t.Errorf("expected first config to be updated")
	}

	// Test append
	newConfig2 := &FtpConfigDetail{Username: "user3", Description: "Third"}
	result = appendOrUpdateFtpConfig(result, newConfig2)

	if len(result) != 3 {
		t.Errorf("expected 3 configs, got %d", len(result))
	}
}

func TestRemoveFtpConfigByUsername(t *testing.T) {
	configs := []*FtpConfigDetail{
		{Username: "user1"},
		{Username: "user2"},
		{Username: "user3"},
	}

	result := removeFtpConfigByUsername(configs, "user2")

	if len(result) != 2 {
		t.Errorf("expected 2 configs, got %d", len(result))
	}
	if result[1].Username != "user3" {
		t.Errorf("expected second config to be user3")
	}
}

func TestTenantDataCopyData(t *testing.T) {
	original := &tenantData{
		Metas: map[int]*TenantMeta{
			1: {TenantID: 1, Code: "t1"},
		},
		Databases: map[int]map[string]*ServiceDatabaseConfig{
			1: {
				"evidence-command": {ServiceCode: "evidence-command"},
			},
		},
		Ftps: map[int][]*FtpConfigDetail{
			1: {{Username: "user1"}},
		},
		Domains: map[int]*DomainConfig{
			1: {Primary: "example.com"},
		},
	}

	copied := original.copyData()

	// Verify copy is independent
	copied.Metas[2] = &TenantMeta{TenantID: 2}
	if _, ok := original.Metas[2]; ok {
		t.Error("copy should be independent")
	}
}
```

**Step 5: Run tests**

Run: `go test ./sdk/pkg/tenant/provider/ -v`
Expected: PASS for all new tests

**Step 6: Commit**

```bash
git add sdk/pkg/tenant/provider/models.go sdk/pkg/tenant/provider/models_test.go
git commit -m "refactor(tenant): update tenantData structure and add helper methods"
```

---

## Task 3: Update Provider Initialization and LoadAll

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Update NewProvider to use new tenantData**

Modify the initialization in `provider.go`:

```go
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
```

**Step 2: Update LoadAll method**

Replace the `LoadAll` method:

```go
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

	p.data.Store(newData)
	logger.Infof("tenant provider: loaded %d tenants, %d database configs, %d ftp configs from ETCD",
		len(newData.Metas), countServiceDatabases(newData.Databases), countFtpConfigs(newData.Ftps))

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
```

**Step 3: Add counting helper functions**

Add to `provider.go`:

```go
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

**Step 4: Update watch prefix constants**

Update the watch prefix in `StartWatch`:

```go
func (p *Provider) StartWatch(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running.Load() {
		return fmt.Errorf("provider already watching")
	}

	p.watchCtx, p.watchCancel = context.WithCancel(ctx)

	// Build watch prefix - watch both tenants and common
	watchChan := p.client.Watch(p.watchCtx, p.namespace+"tenants/", clientv3.WithPrefix())

	p.running.Store(true)

	logger.Infof("tenant provider: starting ETCD watch for prefix %stenants/", p.namespace)

	go p.watchLoop(watchChan)

	return nil
}
```

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "refactor(tenant): update provider initialization and LoadAll for new data structure"
```

---

## Task 4: Rewrite Key Parsing Logic

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Add key detection functions**

Add to `provider.go`:

```go
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
	return len(parts) >= 6 && parts[2] == "tenants" && parts[4] == "domain" && parts[5] == "primary"
}

func isDomainAliasesKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 6 && parts[2] == "tenants" && parts[4] == "domain" && parts[5] == "aliases"
}

func isDomainInternalKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 6 && parts[2] == "tenants" && parts[4] == "domain" && parts[5] == "internal"
}
```

**Step 2: Rewrite processKey method**

Replace the `processKey` method:

```go
func (p *Provider) processKey(key, value string, data *tenantData) {
	key = strings.TrimPrefix(key, p.namespace)

	switch {
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
```

**Step 3: Add new parsing methods**

Add to `provider.go`:

```go
// ========== 解析函数 ==========

// parseTenantMeta 解析租户元数据
func (p *Provider) parseTenantMeta(key string, value string, data *tenantData) error {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[2])
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
	tenantID, err := strconv.Atoi(parts[2])
	if err != nil {
		return err
	}
	serviceCode := parts[4]

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
	tenantID, err := strconv.Atoi(parts[2])
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
	tenantID, err := strconv.Atoi(parts[2])
	if err != nil {
		return err
	}

	var config StorageConfig
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return err
	}
	config.TenantID = tenantID

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
	tenantID, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}

	if data.Domains[tenantID] == nil {
		data.Domains[tenantID] = &DomainConfig{}
	}

	domain := data.Domains[tenantID]

	// 根据不同的 key 类型更新对应字段
	if isDomainPrimaryKey(key) {
		var primary string
		json.Unmarshal([]byte(value), &primary)
		domain.Primary = primary
	} else if isDomainAliasesKey(key) {
		var aliases []string
		json.Unmarshal([]byte(value), &aliases)
		domain.Aliases = aliases
	} else if isDomainInternalKey(key) {
		var internal string
		json.Unmarshal([]byte(value), &internal)
		domain.Internal = internal
	}

	// 填充租户信息（如果已有）
	if meta, ok := data.Metas[tenantID]; ok {
		domain.TenantID = tenantID
		domain.Code = meta.Code
		domain.Name = meta.Name
	}
}
```

**Step 4: Remove old parsing methods**

Delete the following old methods from `provider.go`:
- `processDatabaseKey` (old single database config parser)
- `processFtpKey` (old single ftp config parser)
- `processStorageKey` (old parser, replaced with new version)

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "refactor(tenant): rewrite key parsing logic for new ETCD format"
```

---

## Task 5: Update Watch Event Handling

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Rewrite handleWatchEvent**

Replace the `handleWatchEvent` method:

```go
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
```

**Step 2: Add new event handlers**

Add to `provider.go`:

```go
// ========== Watch 事件处理 ==========

// handleServiceDatabaseChange 处理数据库配置变更
func (p *Provider) handleServiceDatabaseChange(ev *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	serviceCode := parts[4]

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
	tenantID, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}

	if ev.Type == clientv3.EventTypeDelete {
		username := parts[4]
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
	tenantID, err := strconv.Atoi(parts[2])
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
		config.TenantID = tenantID
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
	tenantID, err := strconv.Atoi(parts[2])
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
		// Handle put/update
		if isDomainPrimaryKey(key) {
			var primary string
			json.Unmarshal(ev.Kv.Value, &primary)
			domain.Primary = primary
		} else if isDomainAliasesKey(key) {
			var aliases []string
			json.Unmarshal(ev.Kv.Value, &aliases)
			domain.Aliases = aliases
		} else if isDomainInternalKey(key) {
			var internal string
			json.Unmarshal(ev.Kv.Value, &internal)
			domain.Internal = internal
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
	tenantID, err := strconv.Atoi(parts[2])
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
```

**Step 3: Remove old handler methods**

Delete `handleDeleteKey` method (no longer needed)

**Step 4: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "refactor(tenant): update watch event handling for new data structure"
```

---

## Task 6: Delete Old APIs and Add New APIs

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Delete old APIs**

Delete the following methods from `provider.go`:
- `GetDatabaseConfig(tenantID int)` - old single database config
- `GetFtpConfig(tenantID int)` - old single ftp config

**Step 2: Add new APIs**

Add to `provider.go`:

```go
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
```

**Step 3: Update existing API implementations**

Keep these methods but ensure they use the new data structure:

```go
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

// IsTenantEnabled returns true if the tenant is active
func (p *Provider) IsTenantEnabled(tenantID int) bool {
	meta, ok := p.GetTenantMeta(tenantID)
	return ok && meta.IsEnabled()
}
```

**Step 4: Remove old copyTenantData method**

Delete the old `copyTenantData` method (replaced by `copyData` on `tenantData`)

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "refactor(tenant): delete old APIs and add new APIs for service-level config"
```

---

## Task 7: Update Cache Layer - database/cache.go

**Files:**
- Modify: `sdk/pkg/tenant/database/cache.go`
- Modify: `sdk/pkg/tenant/database/config.go`

**Step 1: Update config.go with new structure**

Replace `sdk/pkg/tenant/database/config.go`:

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
```

**Step 2: Rewrite cache.go**

Replace `sdk/pkg/tenant/database/cache.go`:

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

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/database/cache.go sdk/pkg/tenant/database/config.go
git commit -m "refactor(tenant/database): update cache layer for service-level database config"
```

---

## Task 8: Update Cache Layer - ftp/cache.go

**Files:**
- Modify: `sdk/pkg/tenant/ftp/cache.go`
- Modify: `sdk/pkg/tenant/ftp/config.go`

**Step 1: Update config.go with new fields**

Replace `sdk/pkg/tenant/ftp/config.go`:

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
```

**Step 2: Rewrite cache.go**

Replace `sdk/pkg/tenant/ftp/cache.go`:

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

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/ftp/cache.go sdk/pkg/tenant/ftp/config.go
git commit -m "refactor(tenant/ftp): update cache layer for multi-FTP config support"
```

---

## Task 9: Update Tests

**Files:**
- Modify: `sdk/pkg/tenant/provider/*_test.go`
- Modify: `sdk/pkg/tenant/database/*_test.go`
- Modify: `sdk/pkg/tenant/ftp/*_test.go`

**Step 1: Run all tests to identify failures**

Run: `go test ./sdk/pkg/tenant/... -v`

**Step 2: Fix failing tests one by one**

For each failing test:
1. Update test to use new API
2. Update expected data structures
3. Run test again to verify fix

**Step 3: Add new tests for new APIs**

Add tests for:
- `GetServiceDatabaseConfig`
- `GetAllServiceDatabaseConfigs`
- `GetFtpConfigs`
- `GetFtpConfigByUsername`
- `GetActiveFtpConfigs`
- `GetDomainConfig`

**Step 4: Run full test suite**

Run: `go test ./sdk/pkg/tenant/... -v`

Expected: All tests pass

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/
git commit -m "test(tenant): update tests for new ETCD format support"
```

---

## Task 10: Final Verification

**Step 1: Build verification**

Run: `go build ./sdk/...`

Expected: No build errors

**Step 2: Run full test suite**

Run: `go test ./sdk/pkg/tenant/... -v -cover`

Expected: All tests pass with good coverage

**Step 3: Integration test (if ETCD available)**

Run: `go test ./sdk/pkg/tenant/... -tags=integration -v`

Expected: Integration tests pass

**Step 4: Documentation check**

Verify the implementation matches the adaptation guide:
- [ ] All index structures removed
- [ ] Domain configuration support added
- [ ] Service-level database config working
- [ ] Multi-FTP config support working
- [ ] All old APIs deleted
- [ ] All new APIs implemented

**Step 5: Commit if any final fixes**

```bash
git add .
git commit -m "chore(tenant): final verification and cleanup"
```

---

## Migration Checklist

- [ ] All old APIs deleted (`GetDatabaseConfig`, `GetFtpConfig`)
- [ ] All index-related structures removed
- [ ] Service-level database config working
- [ ] Multi-FTP config array support working
- [ ] Domain configuration support added
- [ ] All tests passing
- [ ] No backward compatibility code remaining

## Deployment Coordination

1. Deploy new tenant-service first (publishes new ETCD format)
2. Clear old ETCD data
3. Deploy new jxt-core
4. Update dependent services to use new APIs

---

**Plan complete and saved to `docs/plans/2026-02-03-tenant-provider-etcd-adaptation.md`.**

**Execution Options:**

1. **Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

2. **Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
