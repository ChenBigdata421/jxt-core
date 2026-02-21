# ResolverConfig Provider Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add ResolverConfig support to jxt-core Provider, enabling consumers to read tenant identification configuration from ETCD.

**Architecture:** Extend the existing Provider with ResolverConfig parsing, storage in tenantData, and accessor methods. Add Watch support for real-time updates with proper key filtering.

**Tech Stack:** Go 1.23+, ETCD client v3, atomic.Value for thread-safe data access

---

## Task 1: Add ResolverConfig Struct to models.go

**Files:**
- Modify: `sdk/pkg/tenant/provider/models.go`
- Test: `sdk/pkg/tenant/provider/models_test.go`

**Step 1: Write the failing test**

Create test in `models_test.go`:

```go
func TestResolverConfigJSONRoundTrip(t *testing.T) {
    original := ResolverConfig{
        ID:             1,
        HTTPType:       "header",
        HTTPHeaderName: "X-Tenant-ID",
        HTTPQueryParam: "tenant",
        HTTPPathIndex:  0,
        FTPType:        "username",
        CreatedAt:      time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC),
        UpdatedAt:      time.Date(2026, 2, 21, 11, 0, 0, 0, time.UTC),
    }

    data, err := json.Marshal(original)
    require.NoError(t, err)

    var parsed ResolverConfig
    err = json.Unmarshal(data, &parsed)
    require.NoError(t, err)

    assert.Equal(t, original.ID, parsed.ID)
    assert.Equal(t, original.HTTPType, parsed.HTTPType)
    assert.Equal(t, original.HTTPHeaderName, parsed.HTTPHeaderName)
    assert.Equal(t, original.HTTPQueryParam, parsed.HTTPQueryParam)
    assert.Equal(t, original.HTTPPathIndex, parsed.HTTPPathIndex)
    assert.Equal(t, original.FTPType, parsed.FTPType)
}
```

**Step 2: Run test to verify it fails**

Run: `cd sdk/pkg/tenant/provider && go test -run TestResolverConfigJSONRoundTrip -v`
Expected: FAIL with "undefined: ResolverConfig"

**Step 3: Add ResolverConfig struct to models.go**

Add after `DomainConfig` struct (around line 137):

```go
// ResolverConfig 租户识别配置（全局配置，不属于特定租户）
// 从 ETCD key: common/resolver 加载
//
// 注意：通过 Provider 获取的 ResolverConfig 指针应视为只读，不可修改字段值。
// 如需修改，请复制后使用。
type ResolverConfig struct {
	ID             int64     `json:"id"`             // 数据库主键，消费方不使用，仅用于反序列化兼容
	HTTPType       string    `json:"httpType"`       // host, header, query, path
	HTTPHeaderName string    `json:"httpHeaderName"` // For type=header
	HTTPQueryParam string    `json:"httpQueryParam"` // For type=query
	HTTPPathIndex  int       `json:"httpPathIndex"`  // For type=path
	FTPType        string    `json:"ftpType"`        // 目前只支持 username
	CreatedAt      time.Time `json:"createdAt"`      // 创建时间，调试和审计用
	UpdatedAt      time.Time `json:"updatedAt"`      // 更新时间，调试和审计用
}
```

**Step 4: Run test to verify it passes**

Run: `cd sdk/pkg/tenant/provider && go test -run TestResolverConfigJSONRoundTrip -v`
Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/models.go sdk/pkg/tenant/provider/models_test.go
git commit -m "feat(provider): add ResolverConfig struct for tenant identification config"
```

---

## Task 2: Add Resolver Field to tenantData Struct

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go` (tenantData struct)

**Step 1: Add Resolver field to tenantData**

Modify `tenantData` struct (around line 93-101):

```go
// tenantData 租户数据（新格式）
type tenantData struct {
	Metas       map[int]*TenantMeta                       `json:"metas"`
	Databases   map[int]map[string]*ServiceDatabaseConfig `json:"databases"` // tenantID -> serviceCode -> config
	Ftps        map[int][]*FtpConfigDetail                `json:"ftps"`      // tenantID -> configs[]
	Storages    map[int]*StorageConfig                    `json:"storages"`
	Domains     map[int]*DomainConfig                     `json:"domains"`     // 域名配置
	Resolver    *ResolverConfig                           `json:"resolver"`    // 新增：全局识别配置
	domainIndex map[string]int                            // 内嵌：域名反向索引 domain -> tenantID（不导出，不序列化）
}
```

**Step 2: Verify compilation**

Run: `cd sdk/pkg/tenant/provider && go build`
Expected: Success (no errors)

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "feat(provider): add Resolver field to tenantData struct"
```

---

## Task 3: Add isResolverConfigKey Function

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`
- Test: `sdk/pkg/tenant/provider/provider_test.go`

**Step 1: Write the failing test**

Add test in `provider_test.go`:

```go
func TestIsResolverConfigKey(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"common/resolver", true},
		{"tenants/1/meta", false},
		{"tenants/1/database/evidence-command", false},
		{"common/storage-directory", false},
		{"_health/sentinel", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := isResolverConfigKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd sdk/pkg/tenant/provider && go test -run TestIsResolverConfigKey -v`
Expected: FAIL with "undefined: isResolverConfigKey"

**Step 3: Add isResolverConfigKey function**

Add after `isDomainInternalKey()` function (around line 378):

```go
func isResolverConfigKey(key string) bool {
	return key == "common/resolver"
}
```

**Step 4: Run test to verify it passes**

Run: `cd sdk/pkg/tenant/provider && go test -run TestIsResolverConfigKey -v`
Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_test.go
git commit -m "feat(provider): add isResolverConfigKey function"
```

---

## Task 4: Add parseResolverConfig Function

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`
- Test: `sdk/pkg/tenant/provider/provider_test.go`

**Step 1: Write the failing test**

Add test in `provider_test.go`:

```go
func TestParseResolverConfig(t *testing.T) {
	jsonData := `{
		"id": 1,
		"httpType": "header",
		"httpHeaderName": "X-Tenant-ID",
		"httpQueryParam": "",
		"httpPathIndex": 0,
		"ftpType": "username",
		"createdAt": "2026-02-21T10:00:00Z",
		"updatedAt": "2026-02-21T11:00:00Z"
	}`

	p := &Provider{}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	err := p.parseResolverConfig("common/resolver", jsonData, data)
	require.NoError(t, err)
	require.NotNil(t, data.Resolver)
	assert.Equal(t, int64(1), data.Resolver.ID)
	assert.Equal(t, "header", data.Resolver.HTTPType)
	assert.Equal(t, "X-Tenant-ID", data.Resolver.HTTPHeaderName)
	assert.Equal(t, "username", data.Resolver.FTPType)
}
```

**Step 2: Run test to verify it fails**

Run: `cd sdk/pkg/tenant/provider && go test -run TestParseResolverConfig -v`
Expected: FAIL

**Step 3: Add parseResolverConfig function**

Add after `parseDomainConfig()` function:

```go
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
```

**Step 4: Run test to verify it passes**

Run: `cd sdk/pkg/tenant/provider && go test -run TestParseResolverConfig -v`
Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_test.go
git commit -m "feat(provider): add parseResolverConfig function"
```

---

## Task 5: Modify processKey to Handle Resolver Config

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Add resolver case to processKey switch**

Modify `processKey()` function (around line 381):

```go
func (p *Provider) processKey(key, value string, data *tenantData) {
	key = strings.TrimPrefix(key, p.namespace)

	switch {
	case isResolverConfigKey(key): // 新增
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
```

**Step 2: Verify compilation and existing tests pass**

Run: `cd sdk/pkg/tenant/provider && go test -v`
Expected: All tests PASS

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "feat(provider): add resolver case to processKey switch"
```

---

## Task 6: Add GetResolverConfig and GetResolverConfigOrDefault Methods

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`
- Test: `sdk/pkg/tenant/provider/provider_api_test.go`

**Step 1: Write the failing tests**

Add tests in `provider_api_test.go`:

```go
func TestGetResolverConfig_Nil(t *testing.T) {
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  nil,
	})

	cfg := p.GetResolverConfig()
	assert.Nil(t, cfg)
}

func TestGetResolverConfig_WithConfig(t *testing.T) {
	expected := &ResolverConfig{
		ID:             1,
		HTTPType:       "host",
		HTTPHeaderName: "X-Tenant-ID",
		FTPType:        "username",
	}
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  expected,
	})

	cfg := p.GetResolverConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, expected, cfg)
}

func TestGetResolverConfigOrDefault_WithConfig(t *testing.T) {
	expected := &ResolverConfig{
		ID:             1,
		HTTPType:       "query",
		HTTPHeaderName: "X-Custom-Tenant",
		FTPType:        "username",
	}
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  expected,
	})

	cfg := p.GetResolverConfigOrDefault()
	assert.Equal(t, "query", cfg.HTTPType)
	assert.Equal(t, "X-Custom-Tenant", cfg.HTTPHeaderName)
}

func TestGetResolverConfigOrDefault_DefaultValues(t *testing.T) {
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  nil,
	})

	cfg := p.GetResolverConfigOrDefault()
	assert.Equal(t, "header", cfg.HTTPType)
	assert.Equal(t, "X-Tenant-ID", cfg.HTTPHeaderName)
	assert.Equal(t, "username", cfg.FTPType)
}

func TestGetResolverConfigOrDefault_ReturnsValueNotPointer(t *testing.T) {
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  nil,
	})

	cfg1 := p.GetResolverConfigOrDefault()
	cfg2 := p.GetResolverConfigOrDefault()

	// Modify cfg1 should not affect cfg2
	cfg1.HTTPType = "modified"

	assert.Equal(t, "header", cfg2.HTTPType)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd sdk/pkg/tenant/provider && go test -run TestGetResolverConfig -v`
Expected: FAIL

**Step 3: Add GetResolverConfig and GetResolverConfigOrDefault methods**

Add after `GetTenantIDByDomain()` method (around line 851):

```go
// GetResolverConfig 获取全局租户识别配置
// 如果未配置返回 nil
func (p *Provider) GetResolverConfig() *ResolverConfig {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil
	}
	return data.Resolver
}

// GetResolverConfigOrDefault 获取识别配置，未配置时返回默认值
// 返回值类型（非指针），避免调用方修改污染全局默认值
// 默认：HTTPType=header, HTTPHeaderName=X-Tenant-ID, FTPType=username
func (p *Provider) GetResolverConfigOrDefault() ResolverConfig {
	if cfg := p.GetResolverConfig(); cfg != nil {
		return *cfg
	}
	return ResolverConfig{
		HTTPType:       "header",
		HTTPHeaderName: "X-Tenant-ID",
		FTPType:        "username",
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd sdk/pkg/tenant/provider && go test -run TestGetResolverConfig -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_api_test.go
git commit -m "feat(provider): add GetResolverConfig and GetResolverConfigOrDefault methods"
```

---

## Task 7: Add handleResolverConfigChange Function

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Add handleResolverConfigChange function**

Add after `handleDomainChange()` function:

```go
// handleResolverConfigChange 处理租户识别配置变更
func (p *Provider) handleResolverConfigChange(ev *clientv3.Event, key string, data *tenantData) {
	if ev.Type == clientv3.EventTypeDelete {
		data.Resolver = nil
		logger.Info("tenant provider: resolver config deleted")
	} else {
		var config ResolverConfig
		if err := json.Unmarshal(ev.Kv.Value, &config); err != nil {
			logger.Errorf("failed to unmarshal resolver config: %v", err)
			return
		}
		data.Resolver = &config
		logger.Infof("tenant provider: resolver config updated, id=%d, httpType=%s, ftpType=%s",
			config.ID, config.HTTPType, config.FTPType)
	}
}
```

**Step 2: Verify compilation**

Run: `cd sdk/pkg/tenant/provider && go build`
Expected: Success

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "feat(provider): add handleResolverConfigChange function for Watch updates"
```

---

## Task 8: Add isKnownWatchKey and Modify handleWatchEvent

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`
- Test: `sdk/pkg/tenant/provider/provider_test.go`

**Step 1: Write the failing test**

Add test in `provider_test.go`:

```go
func TestIsKnownWatchKey(t *testing.T) {
	p := &Provider{}

	tests := []struct {
		key      string
		expected bool
	}{
		// tenants/ 下的配置
		{"tenants/1/meta", true},
		{"tenants/1/database/evidence-command", true},
		{"tenants/1/ftp/admin", true},
		{"tenants/1/storage", true},
		{"tenants/1/domain/primary", true},
		// 公共配置
		{"common/resolver", true},
		{"common/storage-directory", true},
		// 应该被过滤的 key
		{"_health/sentinel", false},
		{"platform/configs/some-key", false},
		{"tenants/_index/by-code/abc", false},
		{"tenants/_index/ftp-user/admin", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := p.isKnownWatchKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd sdk/pkg/tenant/provider && go test -run TestIsKnownWatchKey -v`
Expected: FAIL

**Step 3: Add isKnownWatchKey function and modify handleWatchEvent**

Add `isKnownWatchKey` function after `handleResolverConfigChange`:

```go
// isKnownWatchKey 检查是否为需要处理的 key
// 用于前置过滤，避免无效事件（如 _health/sentinel）触发 copyData()
func (p *Provider) isKnownWatchKey(key string) bool {
	// tenants/ 下的配置
	if strings.HasPrefix(key, "tenants/") {
		return true
	}
	// 公共配置
	if key == "common/resolver" || key == "common/storage-directory" {
		return true
	}
	return false
}
```

Modify `handleWatchEvent()` function (around line 600) to add prefix filter and resolver case:

```go
func (p *Provider) handleWatchEvent(ev *clientv3.Event) {
	key := string(ev.Kv.Key)
	keyStr := strings.TrimPrefix(key, p.namespace)

	// 前置过滤：只处理已知 key 类型，避免无效事件触发 copyData()
	if !p.isKnownWatchKey(keyStr) {
		return
	}

	current := p.data.Load().(*tenantData)
	newData := current.copyData()

	switch {
	case isResolverConfigKey(keyStr): // 新增
		p.handleResolverConfigChange(ev, keyStr, newData)
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

	newData.domainIndex = buildDomainIndex(newData)
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

**Step 4: Run tests to verify they pass**

Run: `cd sdk/pkg/tenant/provider && go test -run TestIsKnownWatchKey -v`
Expected: PASS

Run: `cd sdk/pkg/tenant/provider && go test -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_test.go
git commit -m "feat(provider): add isKnownWatchKey filter and resolver case to handleWatchEvent"
```

---

## Task 9: Expand Watch Range in StartWatch and watchLoop

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Modify StartWatch function**

Find `StartWatch()` function (around line 527) and change the prefix:

```go
func (p *Provider) StartWatch(ctx context.Context) error {
	// ... existing code ...

	// Build watch prefix
	// 修改前：prefix := p.namespace + "tenants/"
	// 修改后：监听整个 namespace，包括 tenants/ 和 common/
	prefix := p.namespace
	watchChan := p.client.Watch(p.watchCtx, prefix, clientv3.WithPrefix())

	// ... rest of function ...
}
```

**Step 2: Modify watchLoop function**

Find `watchLoop()` function (around line 552) and update:

1. Change the log message (around line 557):
```go
// 修改前：logger.Infof("tenant provider: started watching ETCD with prefix %s", p.namespace+"tenants/")
// 修改后：
logger.Infof("tenant provider: started watching ETCD with prefix %s", p.namespace)
```

2. Change the reconnection prefix (around line 576):
```go
case <-time.After(backoff):
	// Re-create the watch
	// 修改前：prefix := p.namespace + "tenants/"
	// 修改后：保持与 StartWatch 一致
	prefix := p.namespace
	watchChan = p.client.Watch(p.watchCtx, prefix, clientv3.WithPrefix())
```

**Step 3: Verify compilation and tests**

Run: `cd sdk/pkg/tenant/provider && go test -v`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "feat(provider): expand Watch range to entire namespace for common/resolver support"
```

---

## Task 10: Modify copyData to Include Resolver Field

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Add Resolver field copy to copyData**

Modify `copyData()` function (around line 776):

```go
func (d *tenantData) copyData() *tenantData {
	newData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  d.Resolver, // 新增：复制指针引用（配置被视为不可变）
	}

	// ... rest of function ...
}
```

**Step 2: Verify compilation**

Run: `cd sdk/pkg/tenant/provider && go build`
Expected: Success

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "feat(provider): add Resolver field to copyData function"
```

---

## Task 11: Update LoadAll Log to Include Resolver Status

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go`

**Step 1: Find and update LoadAll log**

Find the log statement in `LoadAll()` function and update:

```go
// 修改前：
logger.Infof("tenant provider: loaded %d tenants, %d database configs, %d ftp configs, %d domain mappings from ETCD",
    len(newData.Metas), countServiceDatabases(newData.Databases), countFtpConfigs(newData.Ftps),
    len(newData.domainIndex))

// 修改后：
logger.Infof("tenant provider: loaded %d tenants, %d database configs, %d ftp configs, %d domain mappings, resolver=%v from ETCD",
    len(newData.Metas), countServiceDatabases(newData.Databases), countFtpConfigs(newData.Ftps),
    len(newData.domainIndex), newData.Resolver != nil)
```

**Step 2: Verify compilation**

Run: `cd sdk/pkg/tenant/provider && go build`
Expected: Success

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "feat(provider): add resolver status to LoadAll log output"
```

---

## Task 12: Final Verification

**Step 1: Run all tests**

Run: `cd sdk/pkg/tenant/provider && go test -v ./...`
Expected: All tests PASS

**Step 2: Run go vet**

Run: `cd sdk/pkg/tenant/provider && go vet ./...`
Expected: No issues

**Step 3: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix(provider): resolve any remaining issues in ResolverConfig implementation"
```

---

## Summary

| Task | Description | Files Modified |
|------|-------------|----------------|
| 1 | Add ResolverConfig struct | models.go, models_test.go |
| 2 | Add Resolver field to tenantData | provider.go |
| 3 | Add isResolverConfigKey function | provider.go, provider_test.go |
| 4 | Add parseResolverConfig function | provider.go, provider_test.go |
| 5 | Modify processKey switch | provider.go |
| 6 | Add GetResolverConfig methods | provider.go, provider_api_test.go |
| 7 | Add handleResolverConfigChange | provider.go |
| 8 | Add isKnownWatchKey + modify handleWatchEvent | provider.go, provider_test.go |
| 9 | Expand Watch range | provider.go |
| 10 | Modify copyData | provider.go |
| 11 | Update LoadAll log | provider.go |
| 12 | Final verification | - |
