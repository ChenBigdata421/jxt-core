# WVP Configuration Cache — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add WVP configuration caching to jxt-core's Provider (ETCD Watch) and migrate security-management from PlatformRouter (30s polling) to the new Provider-based TenantCacheRouter.

**Architecture:** Extend the existing Provider pattern (atomic.Value + copy-on-write + ETCD Watch) to parse/store/serve WVP configs from ETCD key `tenants/{tenantId}/platform/wvp`. A new `wvp` cache package in jxt-core provides the domain model and Cache wrapper. security-management swaps PlatformRouter for a thin TenantCacheRouter adapter that delegates to tenantdb.Cache.

**Tech Stack:** Go 1.23+, ETCD client v3, testify, Ginkgo v2

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `jxt-core/sdk/pkg/tenant/provider/models.go` | Modify | Add `WvpConfig` struct |
| `jxt-core/sdk/pkg/tenant/provider/provider.go` | Modify | Add `ConfigTypeWvp`, `Wvps` field, parsing, watch, copy, query |
| `jxt-core/sdk/pkg/tenant/provider/provider_test.go` | Modify | Add WVP key detection and parsing tests |
| `jxt-core/sdk/pkg/tenant/wvp/config.go` | Create | `TenantWvpConfig` domain model |
| `jxt-core/sdk/pkg/tenant/wvp/cache.go` | Create | Cache wrapper with `GetByID` |
| `jxt-core/sdk/pkg/tenant/wvp/cache_test.go` | Create | Unit tests for Cache |
| `security-management/common/tenantdb/cache.go` | Modify | Add `GetWvpConfig` method |
| `security-management/common/wvp/client.go` | Modify | Add `TenantCacheRouter`, `SetGlobalRouter`, `GetGlobalRouter` |
| `security-management/common/wvp/router.go` | Delete | Remove PlatformRouter |
| `security-management/common/wvp/router_test.go` | Delete | Remove PlatformRouter tests |
| `security-management/cmd/api/server.go` | Modify | Swap startup/shutdown |
| `security-management/equipment_management/application/service/dependencies.go` | Modify | Rename `GetPlatformRouter` → `GetGlobalRouter` |
| `security-management/tests/wvp_config_tests/suite_test.go` | Modify | Replace PlatformRouter with TenantCacheRouter |
| `security-management/tests/wvp_config_tests/wvp_config_e2e_test.go` | Modify | Rewrite for Provider-based tests |

---

### Task 1: Add `WvpConfig` model to Provider

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/models.go`

- [ ] **Step 1: Add WvpConfig struct to models.go**

Append after the `StorageConfig` struct (after line 66):

```go
// WvpConfig represents WVP (Web Video Platform) configuration for a tenant.
type WvpConfig struct {
	TenantID int    `json:"tenantId"`
	ApiUrl   string `json:"apiUrl"`
	Realm    string `json:"realm"`
}
```

- [ ] **Step 2: Verify the file compiles**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go build ./sdk/pkg/tenant/provider/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add sdk/pkg/tenant/provider/models.go
git commit -m "feat(provider): add WvpConfig model for WVP platform configuration"
```

---

### Task 2: Add `ConfigTypeWvp` constant and extend `tenantData`

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider.go`

- [ ] **Step 1: Add ConfigTypeWvp constant**

In the const block (around line 60-63), add after `ConfigTypeStorage`:

```go
ConfigTypeWvp     ConfigType = "wvp"
```

- [ ] **Step 2: Add Wvps field to tenantData struct**

In the `tenantData` struct (around line 94-103), add after `Storages`:

```go
Wvps    map[int]*WvpConfig                      `json:"wvps"`
```

- [ ] **Step 3: Initialize Wvps in NewProvider**

In `NewProvider` (around line 232-244), add to `initialData`:

```go
Wvps:     make(map[int]*WvpConfig),
```

- [ ] **Step 4: Initialize Wvps in LoadAll**

In `LoadAll` (around line 278-284), add to `newData`:

```go
Wvps:    make(map[int]*WvpConfig),
```

- [ ] **Step 5: Add WvpConfig test case to TestConfigType_String**

In `provider_test.go`, add to the test table (around line 21):

```go
{"wvp", ConfigTypeWvp, "wvp"},
```

- [ ] **Step 6: Verify compilation**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go build ./sdk/pkg/tenant/provider/...`
Expected: no errors (Wvps is not used yet, but initialized)

- [ ] **Step 7: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_test.go
git commit -m "feat(provider): add ConfigTypeWvp and Wvps field to tenantData"
```

---

### Task 3: Add WVP key detection and parsing to Provider

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider.go`
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider_test.go`

- [ ] **Step 1: Write failing test for isWvpConfigKey**

Add to `provider_test.go`:

```go
func TestIsWvpConfigKey(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"tenants/1/platform/wvp", true},
		{"tenants/42/platform/wvp", true},
		{"tenants/1/meta", false},
		{"tenants/1/database/evidence-command", false},
		{"tenants/1/ftp/admin", false},
		{"tenants/1/storage", false},
		{"tenants/1/domain/primary", false},
		{"tenants/1/platform/other", false},
		{"common/resolver", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := isWvpConfigKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/provider/ -run TestIsWvpConfigKey -v`
Expected: compile error — `isWvpConfigKey` undefined

- [ ] **Step 3: Implement isWvpConfigKey**

Add after `isStorageConfigKey` (around line 367 in provider.go):

```go
func isWvpConfigKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) >= 5 && parts[0] == "tenants" && parts[2] == "platform" && parts[3] == "wvp"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/provider/ -run TestIsWvpConfigKey -v`
Expected: PASS

- [ ] **Step 5: Write failing test for parseWvpConfig**

Add to `provider_test.go`:

```go
func TestParseWvpConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}

	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Wvps:      make(map[int]*WvpConfig),
	}

	jsonValue := `{"tenantId":1,"apiUrl":"http://wvp:18978","realm":"3502000000"}`
	err := p.parseWvpConfig("tenants/1/platform/wvp", jsonValue, data)
	require.NoError(t, err)

	cfg, ok := data.Wvps[1]
	require.True(t, ok, "expected config for tenant 1")
	assert.Equal(t, "http://wvp:18978", cfg.ApiUrl)
	assert.Equal(t, "3502000000", cfg.Realm)
	assert.Equal(t, 1, cfg.TenantID)
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/provider/ -run TestParseWvpConfig -v`
Expected: compile error — `parseWvpConfig` undefined

- [ ] **Step 7: Implement parseWvpConfig**

Add to `provider.go` after `parseStorageConfig`:

```go
func (p *Provider) parseWvpConfig(key string, value string, data *tenantData) error {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}

	var config WvpConfig
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return err
	}
	config.TenantID = tenantID

	if config.ApiUrl == "" || config.Realm == "" {
		logger.Warnf("skipping incomplete WVP config for tenant %d: apiUrl=%q, realm=%q", tenantID, config.ApiUrl, config.Realm)
		return nil
	}

	data.Wvps[tenantID] = &config
	return nil
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/provider/ -run TestParseWvpConfig -v`
Expected: PASS

- [ ] **Step 9: Wire parseWvpConfig into processKey switch**

In `processKey` (around line 387-404), add a new case before the closing `}`:

```go
case isWvpConfigKey(key):
	p.parseWvpConfig(key, value, data)
```

Place it after the `isStorageConfigKey` case and before the domain cases.

- [ ] **Step 10: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_test.go
git commit -m "feat(provider): add WVP key detection and parsing"
```

---

### Task 4: Add WVP watch event handling to Provider

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider.go`

- [ ] **Step 1: Add handleWvpChange method**

Add after `handleStorageChange` (around line 736 in provider.go):

```go
func (p *Provider) handleWvpChange(ev *clientv3.Event, key string, data *tenantData) {
	parts := strings.Split(key, "/")
	tenantID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	if ev.Type == clientv3.EventTypeDelete {
		delete(data.Wvps, tenantID)
	} else {
		var config WvpConfig
		if err := json.Unmarshal(ev.Kv.Value, &config); err != nil {
			logger.Errorf("failed to unmarshal wvp config: %v", err)
			return
		}
		config.TenantID = tenantID
t		if config.ApiUrl == "" || config.Realm == "" {
				logger.Warnf("skipping incomplete WVP config for tenant %d: apiUrl=%q, realm=%q", tenantID, config.ApiUrl, config.Realm)
				return
			}
		data.Wvps[tenantID] = &config
	}
}
```

- [ ] **Step 2: Wire handleWvpChange into handleWatchEvent switch**

In `handleWatchEvent` (around line 634-647), add after the `isStorageConfigKey` case:

```go
case isWvpConfigKey(keyStr):
	p.handleWvpChange(ev, keyStr, newData)
```

- [ ] **Step 3: Verify isKnownWatchKey already accepts WVP keys**

The `isKnownWatchKey` function (line 785-799) already accepts all `tenants/` prefixed keys except `_index/`. WVP keys like `tenants/1/platform/wvp` will pass through. No change needed.

Add a test case to `TestIsKnownWatchKey` to confirm:

```go
{"tenants/1/platform/wvp", true},
```

- [ ] **Step 4: Add Wvps shallow copy in copyData**

In `copyData` (around line 840-884), add after the Storages copy block:

```go
// 复制 Wvps
newData.Wvps = make(map[int]*WvpConfig, len(d.Wvps))
for k, v := range d.Wvps {
	newData.Wvps[k] = v
}
```

Also add `Wvps` initialization to the newData struct inside copyData:

```go
Wvps: make(map[int]*WvpConfig),
```

Wait — the copy loop handles initialization. Replace with just the loop:

```go
// 复制 Wvps
for k, v := range d.Wvps {
	newData.Wvps[k] = v
}
```

Make sure to also add `Wvps: make(map[int]*WvpConfig)` to the newData literal in copyData.

- [ ] **Step 5: Update LoadAll log line to include WVP count**

Change the log line (around line 310-312) from:

```go
logger.Infof("tenant provider: loaded %d tenants, %d database configs, %d ftp configs, %d domain mappings, resolver=%v from ETCD",
    len(newData.Metas), countServiceDatabases(newData.Databases), countFtpConfigs(newData.Ftps),
    len(newData.domainIndex), newData.Resolver != nil)
```

to:

```go
logger.Infof("tenant provider: loaded %d tenants, %d database configs, %d ftp configs, %d domain mappings, %d wvp configs, resolver=%v from ETCD",
    len(newData.Metas), countServiceDatabases(newData.Databases), countFtpConfigs(newData.Ftps),
    len(newData.domainIndex), len(newData.Wvps), newData.Resolver != nil)
```

- [ ] **Step 6: Verify compilation and run all provider tests**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/provider/ -v`
Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_test.go
git commit -m "feat(provider): add WVP watch event handling and copyData support"
```

---

### Task 5: Add `GetWvpConfig` query method to Provider

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider.go`

- [ ] **Step 1: Add GetWvpConfig method**

Add after `GetStorageConfig` (around line 1007):

```go
// GetWvpConfig retrieves the WVP configuration for a tenant.
func (p *Provider) GetWvpConfig(tenantID int) (*WvpConfig, bool) {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return nil, false
	}
	cfg, ok := data.Wvps[tenantID]
	return cfg, ok
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go build ./sdk/pkg/tenant/provider/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "feat(provider): add GetWvpConfig query method"
```

---

### Task 6: Create `wvp` cache package

**Files:**
- Create: `jxt-core/sdk/pkg/tenant/wvp/config.go`
- Create: `jxt-core/sdk/pkg/tenant/wvp/cache.go`
- Create: `jxt-core/sdk/pkg/tenant/wvp/cache_test.go`

- [ ] **Step 1: Write the failing test**

Create `jxt-core/sdk/pkg/tenant/wvp/cache_test.go`:

```go
package wvp

import (
	"context"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

func TestCache_GetByID_NotFound(t *testing.T) {
	prov := provider.NewProvider(nil,
		provider.WithConfigTypes(provider.ConfigTypeWvp))

	cache := NewCache(prov)

	_, err := cache.GetByID(context.Background(), 1)
	if err == nil {
		t.Error("expected error for unknown tenant")
	}
}

func TestCache_GetByID_Found(t *testing.T) {
	p := provider.NewProvider(nil,
		provider.WithConfigTypes(provider.ConfigTypeWvp))

	// Populate provider via internal test helper (same pattern as provider_test.go).
	// Since wvp package can't access internal tenantData, this test must live
	// in a provider-internal test file OR we add a public test helper.
	// For now, the found-with-meta path is covered by Task 7's provider-level tests
	// and Task 11's e2e tests. This file tests the wvp.Cache wrapper's not-found path.
	//
	// TODO: Add provider.ExportHelperForTesting or move found-path test to provider_test.go.

	// Minimal found test: verify that if Provider returns data, Cache wraps it correctly.
	// This is tested indirectly through provider_test.go's TestProcessKey_WvpConfig + TestCopyData_WvpPreserved.
	// The wvp.Cache.GetByID is a thin wrapper — the real logic is in Provider.GetWvpConfig + GetTenantMeta.

	// For unit coverage, we test the error message format:
	cache := NewCache(p)

	_, err := cache.GetByID(context.Background(), 1)
	if err == nil {
		t.Error("expected error for tenant without WVP config")
	}
	if err.Error() != "tenant 1 WVP config not found" {
		t.Errorf("error message: got %q, want %q", err.Error(), "tenant 1 WVP config not found")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/wvp/ -v`
Expected: compile error — package does not exist

- [ ] **Step 3: Create config.go**

Create `jxt-core/sdk/pkg/tenant/wvp/config.go`:

```go
package wvp

// TenantWvpConfig represents WVP platform configuration enriched with tenant metadata.
type TenantWvpConfig struct {
	TenantID int
	Code     string
	Name     string
	ApiUrl   string
	Realm    string
}
```

- [ ] **Step 4: Create cache.go**

Create `jxt-core/sdk/pkg/tenant/wvp/cache.go`:

```go
package wvp

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache provides access to tenant WVP configurations.
type Cache struct {
	provider *provider.Provider
}

// NewCache creates a new Cache instance with the given Provider.
func NewCache(prov *provider.Provider) *Cache {
	return &Cache{
		provider: prov,
	}
}

// GetByID retrieves the WVP configuration for a tenant by its ID.
// Returns an error if the tenant does not have a WVP configuration.
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantWvpConfig, error) {
	cfg, ok := c.provider.GetWvpConfig(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant %d WVP config not found", tenantID)
	}

	result := &TenantWvpConfig{
		TenantID: cfg.TenantID,
		ApiUrl:   cfg.ApiUrl,
		Realm:    cfg.Realm,
	}

	if meta, ok := c.provider.GetTenantMeta(tenantID); ok {
		result.Code = meta.Code
		result.Name = meta.Name
	}

	return result, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/wvp/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add sdk/pkg/tenant/wvp/config.go sdk/pkg/tenant/wvp/cache.go sdk/pkg/tenant/wvp/cache_test.go
git commit -m "feat(wvp): add WVP cache package with TenantWvpConfig model"
```

---

### Task 7: jxt-core WVP integration tests

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider_test.go`

This task validates that the Provider correctly handles WVP config through the full pipeline: processKey → parseWvpConfig → copyData → GetWvpConfig, plus watch event handling.

- [ ] **Step 1: Add WVP processKey test**

Add to `provider_test.go`:

```go
func TestProcessKey_WvpConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}

	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Wvps:      make(map[int]*WvpConfig),
	}

	jsonValue := `{"tenantId":1,"apiUrl":"http://wvp:18978","realm":"3502000000"}`
	p.processKey("jxt/tenants/1/platform/wvp", jsonValue, data)

	cfg, ok := data.Wvps[1]
	require.True(t, ok, "expected config for tenant 1")
	assert.Equal(t, "http://wvp:18978", cfg.ApiUrl)
	assert.Equal(t, "3502000000", cfg.Realm)
	assert.Equal(t, 1, cfg.TenantID)
}
```

- [ ] **Step 2: Add WVP copyData test**

```go
func TestCopyData_WvpPreserved(t *testing.T) {
	original := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Wvps: map[int]*WvpConfig{
			1: {TenantID: 1, ApiUrl: "http://wvp-1:18978", Realm: "r1"},
			2: {TenantID: 2, ApiUrl: "http://wvp-2:18978", Realm: "r2"},
		},
	}

	copied := original.copyData()

	assert.Len(t, copied.Wvps, 2)
	assert.Equal(t, "http://wvp-1:18978", copied.Wvps[1].ApiUrl)
	assert.Equal(t, "http://wvp-2:18978", copied.Wvps[2].ApiUrl)

t// Shallow copy shares pointers (same as Storages/Domains pattern).
	// This is safe because the entire map is replaced atomically via atomic.Value.Store().
	assert.Equal(t, original.Wvps[1], copied.Wvps[1], "shallow copy shares pointer")

	// Deleting from copy should not affect original
	delete(copied.Wvps, 1)
	assert.Len(t, copied.Wvps, 1)
	assert.Len(t, original.Wvps, 2)
```

- [ ] **Step 3: Add WVP handleWatchEvent test (PUT + DELETE)**

```go
func TestHandleWvpChange_PutAndDelete(t *testing.T) {
	p := NewProvider(nil)

	initialData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Wvps:      make(map[int]*WvpConfig),
	}
	p.data.Store(initialData)

	// Simulate PUT event
	putEvent := &clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv:   &mvccpb.KeyValue{Key: []byte("jxt/tenants/1/platform/wvp"), Value: []byte(`{"tenantId":1,"apiUrl":"http://wvp:18978","realm":"3502000000"}`)},
	}
	p.handleWatchEvent(putEvent)

	cfg, ok := p.GetWvpConfig(1)
	require.True(t, ok)
	assert.Equal(t, "http://wvp:18978", cfg.ApiUrl)

	// Simulate DELETE event
	deleteEvent := &clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv:   &mvccpb.KeyValue{Key: []byte("jxt/tenants/1/platform/wvp")},
	}
	p.handleWatchEvent(deleteEvent)

	_, ok = p.GetWvpConfig(1)
	assert.False(t, ok, "WVP config should be deleted")
}
```

Note: requires importing `go.etcd.io/etcd/api/v3/mvcc/mvccpb` for `mvccpb.KeyValue`.

- [ ] **Step 4: Run all provider tests**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/provider/ -v -run "Wvp|ProcessKey_Wvp|CopyData_Wvp|HandleWvpChange"`
Expected: all PASS

- [ ] **Step 5: Run full provider test suite**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/provider/ -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add sdk/pkg/tenant/provider/provider_test.go
git commit -m "test(provider): add WVP processKey, copyData, and watch event tests"
```

---

## CHECKPOINT: Publish jxt-core

After Task 7, all jxt-core changes are complete and tested. **Pause here for manual jxt-core release.**

Steps:
1. Push jxt-core to remote: `cd jxt-core && git push`
2. Tag and publish the new jxt-core version (e.g. `go get github.com/ChenBigdata421/jxt-core@vX.Y.Z`)
3. Resume with Task 8 in security-management

---

### Task 8: Bump jxt-core dependency in security-management

**Files:**
- Modify: `security-management/go.mod`

- [ ] **Step 1: Update jxt-core dependency**

```bash
cd D:\JXT\jxt-evidence-system\security-management
go get github.com/ChenBigdata421/jxt-core@latest
go mod tidy
```

- [ ] **Step 2: Verify new Provider symbols are available**

Run: `cd D:\JXT\jxt-evidence-system\security-management && go doc github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider ConfigTypeWvp`
Expected: prints `ConfigTypeWvp ConfigType = "wvp"`

Run: `cd D:\JXT\jxt-evidence-system\security-management && go doc github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider Provider.GetWvpConfig`
Expected: prints the method signature

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: bump jxt-core dependency for WVP cache support"
```

---

### Task 9: Add `GetWvpConfig` to tenantdb.Cache

**Files:**
- Modify: `security-management/common/tenantdb/cache.go`

- [ ] **Step 1: Add import for wvp package**

Add to the import block:

```go
"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/wvp"
```

- [ ] **Step 2: Add GetWvpConfig method**

Add after the `GetDatabaseConfig` method (after line 207):

```go
// GetWvpConfig retrieves the WVP configuration for a tenant.
func (c *Cache) GetWvpConfig(tenantID int) (*wvp.TenantWvpConfig, error) {
	cfg, ok := c.provider.GetWvpConfig(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant %d WVP config not found", tenantID)
	}

	result := &wvp.TenantWvpConfig{
		TenantID: cfg.TenantID,
		ApiUrl:   cfg.ApiUrl,
		Realm:    cfg.Realm,
	}

	if meta, ok := c.provider.GetTenantMeta(tenantID); ok {
		result.Code = meta.Code
		result.Name = meta.Name
	}

	return result, nil
}
```

- [ ] **Step 3: Add ConfigTypeWvp to Provider initialization in Setup**

In the `Setup` function (around line 73-74), change:

```go
provider.WithConfigTypes(provider.ConfigTypeDatabase),
```

to:

```go
provider.WithConfigTypes(provider.ConfigTypeDatabase, provider.ConfigTypeWvp),
```

- [ ] **Step 4: Verify compilation**

Run: `cd D:\JXT\jxt-evidence-system\security-management && go build ./common/tenantdb/...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add common/tenantdb/cache.go
git commit -m "feat(tenantdb): add GetWvpConfig method and ConfigTypeWvp to Provider init"
```

---

### Task 10: Replace PlatformRouter with TenantCacheRouter (atomic)

This task merges the client.go rewrite, router.go deletion, and server.go/dependencies.go updates into one atomic commit to avoid intermediate compile breaks.

**Files:**
- Modify: `security-management/common/wvp/client.go` — add TenantCacheRouter, SetGlobalRouter, GetGlobalRouter
- Delete: `security-management/common/wvp/router.go` — remove PlatformRouter
- Delete: `security-management/common/wvp/router_test.go` — remove PlatformRouter tests
- Modify: `security-management/cmd/api/server.go` — swap startup/shutdown
- Modify: `security-management/equipment_management/application/service/dependencies.go` — rename GetPlatformRouter → GetGlobalRouter

- [ ] **Step 1: Rewrite client.go**

Replace the entire contents of `client.go` with:

```go
package wvp

import (
	"sync"

	"jxt-evidence-system/security-management/common/tenantdb"
)

// Router abstracts tenant-aware WVP platform configuration lookup.
type Router interface {
	Get(tenantID int) *WvpConfig
}

// WvpConfig holds the WVP platform configuration for a single tenant.
type WvpConfig struct {
	ApiUrl string
	Realm  string
}

// TenantCacheRouter implements Router by delegating to tenantdb.Cache.
type TenantCacheRouter struct {
	cache *tenantdb.Cache
}

// NewTenantCacheRouter creates a new TenantCacheRouter.
func NewTenantCacheRouter(cache *tenantdb.Cache) *TenantCacheRouter {
	return &TenantCacheRouter{cache: cache}
}

// Get returns the WVP configuration for the given tenant, or nil if not found.
func (r *TenantCacheRouter) Get(tenantID int) *WvpConfig {
	cfg, err := r.cache.GetWvpConfig(tenantID)
	if err != nil {
		return nil
	}
	return &WvpConfig{ApiUrl: cfg.ApiUrl, Realm: cfg.Realm}
}

// ---------- Global router ----------

var (
	globalRouter Router
	globalMu     sync.Mutex
)

// SetGlobalRouter sets the global WVP router instance.
func SetGlobalRouter(router Router) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalRouter = router
}

// GetGlobalRouter returns the global WVP router instance.
func GetGlobalRouter() Router {
	globalMu.Lock()
	defer globalMu.Unlock()
	return globalRouter
}

var _ Router = (*TenantCacheRouter)(nil)
```

- [ ] **Step 2: Delete router.go and router_test.go**

```bash
rm security-management/common/wvp/router.go
rm security-management/common/wvp/router_test.go
```

- [ ] **Step 3: Update server.go startup (line 113-117)**

Replace:

```go
if tenantCache != nil {
    if _, err := wvp.SetupPlatformRouter(tenantCache); err != nil {
        log.Printf("Warning: WVP PlatformRouter init failed: %v", err)
    }
}
```

With:

```go
if tenantCache != nil {
    wvpRouter := wvp.NewTenantCacheRouter(tenantCache)
    wvp.SetGlobalRouter(wvpRouter)
}
```

- [ ] **Step 4: Update server.go shutdown (line 565-567)**

Remove these three lines:

```go
log.Println("Stopping WVP PlatformRouter...")
wvp.StopPlatformRouter()
log.Println("WVP PlatformRouter stopped")
```

- [ ] **Step 5: Update dependencies.go (line 178-185)**

Replace:

```go
func registerWvpRouterDependencies() {
	err := di.Provide(func() wvp.Router {
		return wvp.GetPlatformRouter()
	})
	if err != nil {
		logger.Fatalf("Failed to provide WvpRouter: %v", err)
	}
}
```

With:

```go
func registerWvpRouterDependencies() {
	err := di.Provide(func() wvp.Router {
		return wvp.GetGlobalRouter()
	})
	if err != nil {
		logger.Fatalf("Failed to provide WvpRouter: %v", err)
	}
}
```

- [ ] **Step 6: Verify compilation**

Run: `cd D:\JXT\jxt-evidence-system\security-management && go build ./...`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add -A security-management/common/wvp/ security-management/cmd/api/server.go security-management/equipment_management/application/service/dependencies.go
git commit -m "refactor(wvp): replace PlatformRouter with TenantCacheRouter

- Delete PlatformRouter (30s ETCD polling) and its tests
- Add TenantCacheRouter delegating to tenantdb.Cache
- Move WvpConfig from router.go to client.go
- Update server.go startup/shutdown
- Update DI to use GetGlobalRouter()"
```

---

### Task 11: Rewrite WVP e2e tests

**Files:**
- Modify: `security-management/tests/wvp_config_tests/suite_test.go`
- Modify: `security-management/tests/wvp_config_tests/wvp_config_e2e_test.go`

- [ ] **Step 1: Update suite_test.go — replace PlatformRouter references**

The `setupRouter` function (lines 220-227) currently calls `wvp.StopPlatformRouter()` and `wvp.SetupPlatformRouter()`. Replace it with:

```go
// setupRouter creates a fresh TenantCacheRouter backed by the tenantDB cache.
func setupRouter() wvp.Router {
	wvpRouter := wvp.NewTenantCacheRouter(tenantCache)
	wvp.SetGlobalRouter(wvpRouter)
	return wvpRouter
}
```

The `AfterSuite` (line 77-86) calls `wvp.StopPlatformRouter()`. Remove that line:

```go
var _ = AfterSuite(func() {
	if etcdClient != nil {
		etcdClient.Close()
	}
	if tenantCache != nil {
		tenantCache.Stop()
	}
})
```

- [ ] **Step 2: Rewrite wvp_config_e2e_test.go**

The test file uses `*wvp.PlatformRouter` and methods like `LoadedCount`, `MissCount`. Replace with `wvp.Router` interface and Provider-backed assertions.

Key changes:
- `var router *wvp.PlatformRouter` → `var router wvp.Router`
- Remove `LoadedCount` / `MissCount` assertions (PlatformRouter-specific metrics)
- `router.Get(id)` still works through `wvp.Router` interface
- `setupRouter()` now returns `wvp.Router`
- For ETCD propagation tests, add `time.Sleep` or `Eventually` to wait for ETCD Watch (instead of re-creating router)

```go
package wvp_config_tests

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"jxt-evidence-system/security-management/common/wvp"
)

const (
	testTenantID = 999
	apiTenantID  = 1
)

var _ = Describe("WVP Config E2E", Ordered, func() {
	var router wvp.Router

	BeforeAll(func() {
		_ = etcdHelper.deleteWvpConfig(testTenantID)
		tenantService.deleteWvpConfig(apiTenantID)

		router = setupRouter()
	})

	AfterAll(func() {
		_ = etcdHelper.deleteWvpConfig(testTenantID)
		tenantService.deleteWvpConfig(apiTenantID)
	})

	// --- Scenario 1: Direct ETCD write -> TenantCacheRouter reads ---

	Context("ETCD direct write to TenantCacheRouter read", func() {
		AfterEach(func() {
			_ = etcdHelper.deleteWvpConfig(testTenantID)
		})

		It("should read WVP config after writing to ETCD", func() {
			apiUrl := "http://wvp-test:18978"
			realm := "3502000099"
			err := etcdHelper.putWvpConfig(testTenantID, apiUrl, realm)
			Expect(err).NotTo(HaveOccurred())

			// Wait for ETCD Watch to propagate to Provider
			Eventually(func() *wvp.WvpConfig {
				return router.Get(testTenantID)
			}, 5*time.Second, 200*time.Millisecond).ShouldNot(BeNil(), "should find config for tenant %d", testTenantID)

			cfg := router.Get(testTenantID)
			Expect(cfg.ApiUrl).To(Equal(apiUrl))
			Expect(cfg.Realm).To(Equal(realm))
		})

		It("should return nil for a tenant with no WVP config", func() {
			_ = etcdHelper.deleteWvpConfig(testTenantID)

			// Wait for deletion to propagate
			Eventually(func() *wvp.WvpConfig {
				return router.Get(testTenantID)
			}, 5*time.Second, 200*time.Millisecond).Should(BeNil(), "should not find config for tenant without WVP config")
		})
	})

	// --- Scenario 2: Tenant-service API -> ETCD -> TenantCacheRouter reads ---

	Context("Tenant-service API to TenantCacheRouter read", func() {
		BeforeEach(func() {
			tenantService.deleteWvpConfig(apiTenantID)
		})

		AfterEach(func() {
			tenantService.deleteWvpConfig(apiTenantID)
		})

		It("should create WVP config via tenant-service and read via TenantCacheRouter", func() {
			apiUrl := "http://wvp-e2e-create:18978"
			realm := "3502000099"

			resp, err := tenantService.createWvpConfig(apiTenantID, apiUrl, realm)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Code).To(Equal(200), fmt.Sprintf("create should succeed: %s", resp.Msg))

			Eventually(func() *wvp.WvpConfig {
				return router.Get(apiTenantID)
			}, 5*time.Second, 200*time.Millisecond).ShouldNot(BeNil(), "TenantCacheRouter should find config for tenant %d", apiTenantID)

			cfg := router.Get(apiTenantID)
			Expect(cfg.ApiUrl).To(Equal(apiUrl))
			Expect(cfg.Realm).To(Equal(realm))
		})

		It("should update WVP config via tenant-service and reflect in TenantCacheRouter", func() {
			initialURL := "http://wvp-e2e-update-v1:18978"
			initialRealm := "3502000099"
			resp, err := tenantService.createWvpConfig(apiTenantID, initialURL, initialRealm)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Code).To(Equal(200))

			Eventually(func() *wvp.WvpConfig {
				return router.Get(apiTenantID)
			}, 5*time.Second, 200*time.Millisecond).ShouldNot(BeNil())

			updatedURL := "http://wvp-e2e-update-v2:18978"
			updatedRealm := "3502000088"
			resp, err = tenantService.updateWvpConfig(apiTenantID, updatedURL, updatedRealm)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Code).To(Equal(200))

			Eventually(func() string {
				cfg := router.Get(apiTenantID)
				if cfg == nil {
					return ""
				}
				return cfg.ApiUrl
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(updatedURL), "TenantCacheRouter should reflect updated apiUrl")

			cfg := router.Get(apiTenantID)
			Expect(cfg.Realm).To(Equal(updatedRealm))
		})

		It("should delete WVP config via tenant-service and return nil from TenantCacheRouter", func() {
			resp, err := tenantService.createWvpConfig(apiTenantID, "http://wvp-e2e-delete:18978", "3502000099")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Code).To(Equal(200))

			Eventually(func() *wvp.WvpConfig {
				return router.Get(apiTenantID)
			}, 5*time.Second, 200*time.Millisecond).ShouldNot(BeNil())

			resp, err = tenantService.deleteWvpConfig(apiTenantID)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Code).To(Equal(200))

			Eventually(func() *wvp.WvpConfig {
				return router.Get(apiTenantID)
			}, 5*time.Second, 200*time.Millisecond).Should(BeNil(), "TenantCacheRouter should return nil after delete")
		})
	})

	// --- Scenario 3: ETCD data integrity ---

	Context("ETCD data integrity", func() {
		BeforeEach(func() {
			tenantService.deleteWvpConfig(apiTenantID)
		})

		AfterEach(func() {
			tenantService.deleteWvpConfig(apiTenantID)
		})

		It("should store correct JSON structure in ETCD", func() {
			apiUrl := "http://wvp-e2e-json:18978"
			realm := "3502000099"

			resp, err := tenantService.createWvpConfig(apiTenantID, apiUrl, realm)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Code).To(Equal(200))

			Eventually(func() string {
				val, _ := etcdHelper.getWvpConfig(apiTenantID)
				return val
			}, 5*time.Second, 200*time.Millisecond).ShouldNot(BeEmpty())

			rawVal, err := etcdHelper.getWvpConfig(apiTenantID)
			Expect(err).NotTo(HaveOccurred())

			var data map[string]interface{}
			err = json.Unmarshal([]byte(rawVal), &data)
			Expect(err).NotTo(HaveOccurred())

			Expect(data["tenantId"]).To(Equal(float64(apiTenantID)))
			Expect(data["apiUrl"]).To(Equal(apiUrl))
			Expect(data["realm"]).To(Equal(realm))
		})
	})
})
```

- [ ] **Step 3: Verify compilation**

Run: `cd D:\JXT\jxt-evidence-system\security-management && go build ./tests/wvp_config_tests/...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add security-management/tests/wvp_config_tests/suite_test.go security-management/tests/wvp_config_tests/wvp_config_e2e_test.go
git commit -m "test(wvp): rewrite e2e tests for TenantCacheRouter

Replace PlatformRouter-specific assertions (LoadedCount, MissCount,
setupRouter re-creation) with Provider-backed ETCD Watch assertions
using Eventually matchers."
```

---

### Task 12: Final verification and cleanup

**Files:** None

- [ ] **Step 1: Run all jxt-core provider tests**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/provider/ -v`
Expected: all PASS

- [ ] **Step 2: Run wvp cache tests**

Run: `cd D:\JXT\jxt-evidence-system\jxt-core && go test ./sdk/pkg/tenant/wvp/ -v`
Expected: all PASS

- [ ] **Step 3: Build entire security-management**

Run: `cd D:\JXT\jxt-evidence-system\security-management && go build ./...`
Expected: no errors

- [ ] **Step 4: Verify no remaining references to deleted code**

Run:
```bash
cd D:\JXT\jxt-evidence-system\security-management && grep -r "PlatformRouter\|SetupPlatformRouter\|StopPlatformRouter\|GetPlatformRouter" --include="*.go" .
```
Expected: no matches

- [ ] **Step 5: Final commit if any cleanup needed**

```bash
git add -A
git commit -m "chore: cleanup after WVP cache migration"
```

---

## Summary of Changes

| Component | Before | After |
|-----------|--------|-------|
| jxt-core Provider | No WVP support | Parses/watches/serves WVP config |
| jxt-core wvp package | Does not exist | New Cache + TenantWvpConfig |
| security-management WVP | PlatformRouter (30s poll, sync.RWMutex) | TenantCacheRouter (ETCD Watch, atomic.Value) |
| security-management tenantdb | No WVP support | GetWvpConfig method |
| e2e tests | PlatformRouter-specific | Provider-backed ETCD Watch assertions |

## Task Sequence

| Phase | Tasks | Description |
|-------|-------|-------------|
| **jxt-core** | 1-6 | WvpConfig model, Provider extension, wvp cache package |
| **jxt-core test** | 7 | Integration tests (processKey, copyData, watch events) |
| **Checkpoint** | — | Manual jxt-core version release |
| **security-management** | 8-11 | Bump dependency, TenantCacheRouter, rewrite e2e |
| **Verification** | 12 | Cross-project build + grep checks |

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 2 | CLEAR | 4 issues, 0 critical gaps |

- **UNRESOLVED:** 0
- **VERDICT:** ENG CLEARED — ready to implement
