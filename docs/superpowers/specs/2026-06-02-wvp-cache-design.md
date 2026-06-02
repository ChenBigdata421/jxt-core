# WVP Configuration Cache — Design Spec

## Goal

Move WVP (Web Video Platform) configuration caching from security-management's PlatformRouter (30s ETCD polling) into jxt-core's Provider (ETCD Watch), following the same pattern as database/ftp/storage cache types.

## Current State

- tenant-service stores WVP config in ETCD at key `jxt/tenants/{tenantId}/platform/wvp`
- security-management's `PlatformRouter` polls ETCD every 30 seconds, maintains its own `map[int]*WvpConfig`
- No other service consumes WVP config today

## Changes

### 1. jxt-core Provider — New Model

**File: `sdk/pkg/tenant/provider/models.go`**

Add struct:

```go
type WvpConfig struct {
    TenantID int    `json:"tenantId"`
    ApiUrl   string `json:"apiUrl"`
    Realm    string `json:"realm"`
}
```

### 2. jxt-core Provider — ConfigType Constant

**File: `sdk/pkg/tenant/provider/provider.go`**

Add constant alongside existing ConfigType values:

```go
ConfigTypeWvp ConfigType = "wvp"
```

### 3. jxt-core Provider — tenantData Extension

**File: `sdk/pkg/tenant/provider/provider.go`**

- Add field to `tenantData`:

```go
Wvps map[int]*WvpConfig `json:"wvps"`
```

- Initialize `Wvps: make(map[int]*WvpConfig)` in `NewProvider`, `LoadAll`, `copyData`
- Add shallow copy in `copyData`:

```go
for k, v := range d.Wvps {
    newData.Wvps[k] = v
}
```

### 4. jxt-core Provider — Key Detection and Parsing

**File: `sdk/pkg/tenant/provider/provider.go`**

Key pattern: `tenants/{tenantId}/platform/wvp`

```go
func isWvpConfigKey(key string) bool {
    parts := strings.Split(key, "/")
    return len(parts) >= 5 && parts[0] == "tenants" && parts[2] == "platform" && parts[3] == "wvp"
}
```

New case in `processKey`:

```go
case isWvpConfigKey(keyStr):
    p.parseWvpConfig(keyStr, value, newData)
```

`parseWvpConfig` extracts tenantID from key path segment (`parts[1]`), unmarshals JSON value into `WvpConfig`, stores in `data.Wvps[tenantID]`.

### 5. jxt-core Provider — Watch Event Handling

**File: `sdk/pkg/tenant/provider/provider.go`**

New case in `handleWatchEvent` → `handleWvpChange(ev, keyStr, newData)`:

- PUT: parse and upsert into `data.Wvps[tenantID]`
- DELETE: `delete(data.Wvps, tenantID)`

Add to `isKnownWatchKey` — keys under `tenants/{id}/platform/` are valid.

### 6. jxt-core Provider — Query Method

**File: `sdk/pkg/tenant/provider/provider.go`**

```go
func (p *Provider) GetWvpConfig(tenantID int) (*WvpConfig, bool)
```

Reads from `p.data.Load().(*tenantData).Wvps[tenantID]`.

### 7. jxt-core Provider — Log Count

Update `LoadAll` log line to include WVP config count.

### 8. jxt-core — New wvp Cache Package

**File: `sdk/pkg/tenant/wvp/config.go`** (new)

```go
type TenantWvpConfig struct {
    TenantID int
    Code     string
    Name     string
    ApiUrl   string
    Realm    string
}
```

**File: `sdk/pkg/tenant/wvp/cache.go`** (new)

```go
type Cache struct {
    provider *provider.Provider
}

func NewCache(prov *provider.Provider) *Cache

func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantWvpConfig, error)
```

`GetByID` calls `provider.GetWvpConfig(tenantID)` + `provider.GetTenantMeta(tenantID)`, combines into `TenantWvpConfig`. Same pattern as `storage.Cache.GetByID`.

**File: `sdk/pkg/tenant/wvp/cache_test.go`** (new)

Unit tests: valid config, unknown tenant error, meta enrichment.

### 9. security-management — Provider Init

**File: `security-management/cmd/api/server.go`**

Add `provider.ConfigTypeWvp` to WithConfigTypes option (for clarity, though current Provider does not filter by ConfigType).

### 10. security-management — tenantdb.Cache Extension

**File: `security-management/common/tenantdb/cache.go`**

Add method:

```go
func (c *Cache) GetWvpConfig(tenantID int) (*wvp.TenantWvpConfig, error)
```

Delegates to `c.provider.GetWvpConfig(tenantID)`, enriches with meta.

### 11. security-management — Replace PlatformRouter

**Delete:**
- `security-management/common/wvp/router.go` — entire PlatformRouter implementation
- `security-management/common/wvp/router_test.go` — tests for deleted code

**Modify: `security-management/common/wvp/client.go`**

Add thin adapter implementing the existing `Router` interface:

```go
type TenantCacheRouter struct {
    cache *tenantdb.Cache
}

func NewTenantCacheRouter(cache *tenantdb.Cache) *TenantCacheRouter

func (r *TenantCacheRouter) Get(tenantID int) *WvpConfig {
    cfg, err := r.cache.GetWvpConfig(tenantID)
    if err != nil {
        return nil
    }
    return &WvpConfig{ApiUrl: cfg.ApiUrl, Realm: cfg.Realm}
}
```

All existing WVP consumers (SignClient, etc.) continue to use `Router` interface unchanged.

### 12. security-management — Startup / Shutdown

**File: `security-management/cmd/api/server.go`**

Startup:
```go
// Replace: wvp.SetupPlatformRouter(tenantCache)
wvpRouter := wvp.NewTenantCacheRouter(tenantCache)
wvp.SetGlobalRouter(wvpRouter)
```

Shutdown:
```go
// Remove: wvp.StopPlatformRouter()
// No cleanup needed — Provider handles ETCD watch lifecycle
```

## Benefits

- **Real-time updates**: ETCD Watch replaces 30s polling
- **No lock contention**: atomic.Value replaces sync.RWMutex
- **File cache fallback**: automatic when ETCD unavailable
- **Code reduction**: ~273 lines of PlatformRouter removed from security-management
- **Consistent pattern**: follows existing database/ftp/storage cache convention

## File Summary

| File | Action |
|------|--------|
| `jxt-core/sdk/pkg/tenant/provider/models.go` | Add `WvpConfig` struct |
| `jxt-core/sdk/pkg/tenant/provider/provider.go` | Add parsing, watch, copy, query |
| `jxt-core/sdk/pkg/tenant/wvp/config.go` | New — domain model |
| `jxt-core/sdk/pkg/tenant/wvp/cache.go` | New — Cache wrapper |
| `jxt-core/sdk/pkg/tenant/wvp/cache_test.go` | New — unit tests |
| `security-management/common/tenantdb/cache.go` | Add `GetWvpConfig` method |
| `security-management/common/wvp/client.go` | Add `TenantCacheRouter` |
| `security-management/common/wvp/router.go` | Delete |
| `security-management/common/wvp/router_test.go` | Delete |
| `security-management/cmd/api/server.go` | Update init + shutdown |
| `security-management/tests/wvp_config_tests/` | Rewrite for TenantCacheRouter |
| `jxt-core/sdk/pkg/tenant/provider/provider_test.go` | Add WVP parsing/watch test cases |

## NOT in scope

- ConfigType filtering in Provider (currently a no-op — all key patterns are parsed regardless of `configTypes`)
- Other services adopting WVP cache (only security-management needs it today)
- WVP config validation (ApiUrl/Realm empty checks remain consumer-side)
- Password encryption for WVP config (not applicable)

## What already exists

| Existing code | Plan reuses? | Notes |
|---|---|---|
| `PlatformRouter.parseWvpConfigs()` | No — replaced by Provider parsing | Key pattern and JSON format reused conceptually |
| `PlatformRouter.loadAll()` | No — replaced by Provider ETCD watch | 30s polling eliminated |
| `wvp.Router` interface | Yes — preserved | TenantCacheRouter implements it |
| `wvp.WvpConfig` struct | Yes — preserved | Identical fields to provider.WvpConfig |
| DI registration in `dependencies.go` | Yes — unchanged | Calls `wvp.GetPlatformRouter()` → rename to `GetGlobalRouter()` |
| Provider `copyData()` pattern | Yes — extended | Same shallow-copy approach |
| Provider `processKey()` switch | Yes — extended | New case added |
| Provider `handleWatchEvent()` switch | Yes — extended | New case added |

## Design Clarifications

1. `client.go` needs both `SetGlobalRouter(router Router)` and `GetGlobalRouter() Router` — the DI container at `dependencies.go:180` calls `wvp.GetPlatformRouter()` which must be renamed to `GetGlobalRouter()`.

2. `tenantdb.Cache.GetWvpConfig()` should follow the same error message pattern as `GetDatabaseConfig()` — returning `fmt.Errorf` with tenant ID context.

3. E2E tests in `tests/wvp_config_tests/` must be rewritten to use Provider + TenantCacheRouter instead of PlatformRouter. Polling-specific assertions (wait-for-convergence) are replaced with direct Provider state checks.

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | CLEAR | 2 issues, 0 critical gaps |

- **UNRESOLVED:** 0
- **VERDICT:** ENG CLEARED — ready to implement
