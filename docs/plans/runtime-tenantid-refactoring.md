# sdk/runtime/ Refactoring Plan: String → Int for TenantID

## Objective

Refactor `sdk/runtime/` package to use `int` for `tenantID` instead of `string`, aligning with the new `sdk/pkg/tenant/` package.

**Key Decision**: Completely remove `TenantResolver` - functionality is superseded by `pkg/tenant/middleware` and `pkg/tenant/provider`.

---

## Current State Analysis

### Files to Modify

| File | Lines | Action |
|------|-------|--------|
| `type.go` | 95 | Update interface, remove `SetTenantMapping/GetTenantID` |
| `application.go` | 370 | Update implementations, remove `tenantResolver` field |
| `tenant_resolver.go` | 100 | **DELETE** - replaced by `pkg/tenant/middleware` |
| `middleware/extract_tenant_id.go` | 224 | Update to store `int` directly |

### Current Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ HTTP Request                                              │
│   ↓                                                        │
│ runtime.TenantResolver.GetTenantID(host) (string)         │  ← DELETE
│   ↓                                                        │
│ context.Set("tenant_id", "123")                          │  ← string
│   ↓                                                        │
│ Application.GetTenantDB("123") (string key)                │
└─────────────────────────────────────────────────────────────┘
```

### Special Value `"*"` to Remove

```go
// Current: "*" as fallback/default
if db, ok := e.tenantDBs.Load("*"); ok {
    return db.(*gorm.DB)  // default DB when tenant not found
}
```

**New Behavior**: No fallback, caller must handle nil explicitly.

---

## Target Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ HTTP Request                                              │
│   ↓                                                        │
│ pkg/tenant/middleware.ExtractTenantID()                   │  ← NEW
│   ├─ host:   subdomain extraction                         │
│   ├─ header: X-Tenant-ID header                            │
│   ├─ query:  query parameter                               │
│   └─ path:   URL path segment                              │
│   ↓                                                        │
│ context.Set("tenant_id", 123)  ← int                       │
│   ↓                                                        │
│ Application.GetTenantDB(123) (int key)                      │
└─────────────────────────────────────────────────────────────┘

Non-HTTP Context (background jobs, CLI):
┌─────────────────────────────────────────────────────────────┐
│ pkg/tenant/provider.GetDatabaseConfig(123)                  │  ← ETCD
│ pkg/tenant/provider.GetFtpConfig(123)                      │  ← ETCD
│ pkg/tenant/provider.GetStorageConfig(123)                  │  ← ETCD
└─────────────────────────────────────────────────────────────┘
```

### TenantID Semantics

| Value | Meaning |
|-------|---------|
| `0` | Invalid/not found (middleware returns error) |
| `1` | Default tenant (system built-in) |
| `2+` | User-created tenants |

---

## Implementation Plan

### Phase 1: Remove TenantResolver

#### 1.1 Delete `tenant_resolver.go`

```bash
# Delete the file entirely
rm sdk/runtime/tenant_resolver.go
```

**Rationale**:
- Functionality superseded by `pkg/tenant/middleware` (4 resolver types vs 1)
- Configuration superseded by `pkg/tenant/provider` (ETCD-based vs manual map)
- No need to maintain dual systems

---

#### 1.2 Update `application.go`

**Remove `tenantResolver` field**:
```go
// Before
type Application struct {
    tenantResolver   *TenantResolver     // ← DELETE
    tenantDBs        sync.Map
    // ...
}

// After
type Application struct {
    // tenantResolver removed - use pkg/tenant/middleware instead
    tenantDBs        sync.Map
    tenantCommandDBs sync.Map
    tenantQueryDBs   sync.Map
    // ...
}
```

**Remove from `NewConfig()`**:
```go
// Before
func NewConfig() *Application {
    return &Application{
        tenantResolver:   NewTenantResolver(),  // ← DELETE
        tenantDBs:        sync.Map{},
        // ...
    }
}

// After
func NewConfig() *Application {
    return &Application{
        tenantDBs:        sync.Map{},
        tenantCommandDBs: sync.Map{},
        tenantQueryDBs:   sync.Map{},
        // ...
    }
}
```

**Remove methods**:
```go
// DELETE these methods:
func (e *Application) SetTenantMapping(host, tenantID string) { ... }
func (e *Application) GetTenantID(host string) string { ... }
```

---

#### 1.3 Update `type.go` - Runtime Interface

**Remove methods**:
```go
// DELETE from interface:
SetTenantMapping(host, tenantID string)
GetTenantID(host string) string
```

**Update remaining methods** (all `tenantID string` → `tenantID int`):
```go
type Runtime interface {
    // Tenant DB methods (CQRS command side)
    SetTenantDB(tenantID int, db *gorm.DB)
    GetTenantDB(tenantID int) *gorm.DB
    GetTenantDBs(fn func(tenantID int, db *gorm.DB) bool)

    // Tenant Command DB (CQRS)
    SetTenantCommandDB(tenantID int, db *gorm.DB)
    GetTenantCommandDB(tenantID int) *gorm.DB
    GetTenantCommandDBs(fn func(tenantID int, db *gorm.DB) bool)

    // Tenant Query DB (CQRS)
    SetTenantQueryDB(tenantID int, db *gorm.DB)
    GetTenantQueryDB(tenantID int) *gorm.DB
    GetTenantQueryDBs(fn func(tenantID int, db *gorm.DB) bool)

    // Tenant Casbin
    SetTenantCasbin(tenantID int, enforcer *casbin.SyncedEnforcer)
    GetTenantCasbin(tenantID int) *casbin.SyncedEnforcer
    GetCasbins(fn func(tenantID int, enforcer *casbin.SyncedEnforcer) bool)

    // Tenant Crontab
    SetTenantCrontab(tenantID int, crontab *cron.Cron)
    GetTenantCrontab(tenantID int) *cron.Cron
    GetCrontabs(fn func(tenantID int, crontab *cron.Cron) bool)

    // Other methods unchanged...
}
```

---

### Phase 2: Update `application.go` Implementations

#### 2.1 Update all tenantID method signatures

**List of methods to update**:
```go
// SetTenantDB
func (e *Application) SetTenantDB(tenantID int, db *gorm.DB) {
    e.tenantDBs.Store(tenantID, db)
}

// GetTenantDB - No "*" fallback
func (e *Application) GetTenantDB(tenantID int) *gorm.DB {
    if db, ok := e.tenantDBs.Load(tenantID); ok {
        return db.(*gorm.DB)
    }
    return nil
}

// GetTenantDBs - callback signature
func (e *Application) GetTenantDBs(fn func(tenantID int, db *gorm.DB) bool) {
    e.tenantDBs.Range(func(key, value interface{}) bool {
        return fn(key.(int), value.(*gorm.DB))
    })
}

// SetTenantCommandDB
func (e *Application) SetTenantCommandDB(tenantID int, db *gorm.DB) {
    e.tenantCommandDBs.Store(tenantID, db)
}

// GetTenantCommandDB
func (e *Application) GetTenantCommandDB(tenantID int) *gorm.DB {
    if db, ok := e.tenantCommandDBs.Load(tenantID); ok {
        return db.(*gorm.DB)
    }
    return nil
}

// GetTenantCommandDBs
func (e *Application) GetTenantCommandDBs(fn func(tenantID int, db *gorm.DB) bool) {
    e.tenantCommandDBs.Range(func(key, value interface{}) bool {
        return fn(key.(int), value.(*gorm.DB))
    })
}

// SetTenantQueryDB
func (e *Application) SetTenantQueryDB(tenantID int, db *gorm.DB) {
    e.tenantQueryDBs.Store(tenantID, db)
}

// GetTenantQueryDB
func (e *Application) GetTenantQueryDB(tenantID int) *gorm.DB {
    if db, ok := e.tenantQueryDBs.Load(tenantID); ok {
        return db.(*gorm.DB)
    }
    return nil
}

// GetTenantQueryDBs
func (e *Application) GetTenantQueryDBs(fn func(tenantID int, db *gorm.DB) bool) {
    e.tenantQueryDBs.Range(func(key, value interface{}) bool {
        return fn(key.(int), value.(*gorm.DB))
    })
}

// SetTenantCasbin
func (e *Application) SetTenantCasbin(tenantID int, enforcer *casbin.SyncedEnforcer) {
    e.casbins.Store(tenantID, enforcer)
}

// GetTenantCasbin - No "*" fallback
func (e *Application) GetTenantCasbin(tenantID int) *casbin.SyncedEnforcer {
    if value, ok := e.casbins.Load(tenantID); ok {
        return value.(*casbin.SyncedEnforcer)
    }
    return nil
}

// GetCasbins
func (e *Application) GetCasbins(fn func(tenantID int, enforcer *casbin.SyncedEnforcer) bool) {
    e.casbins.Range(func(key, value interface{}) bool {
        return fn(key.(int), value.(*casbin.SyncedEnforcer))
    })
}

// SetTenantCrontab
func (e *Application) SetTenantCrontab(tenantID int, crontab *cron.Cron) {
    e.crontabs.Store(tenantID, crontab)
}

// GetTenantCrontab - No "*" fallback
func (e *Application) GetTenantCrontab(tenantID int) *cron.Cron {
    if value, ok := e.crontabs.Load(tenantID); ok {
        return value.(*cron.Cron)
    }
    return nil
}

// GetCrontabs
func (e *Application) GetCrontabs(fn func(tenantID int, crontab *cron.Cron) bool) {
    e.crontabs.Range(func(key, value interface{}) bool {
        return fn(key.(int), value.(*cron.Cron))
    })
}
```

---

### Phase 3: Update Middleware to Store int

#### 3.1 `sdk/pkg/tenant/middleware/extract_tenant_id.go`

**Update context storage to use int**:
```go
func ExtractTenantID(opts ...Option) gin.HandlerFunc {
    cfg := defaultConfig()
    for _, opt := range opts {
        opt(cfg)
    }

    return func(c *gin.Context) {
        tenantIDStr, ok := extractTenantID(c, cfg)
        if !ok {
            c.JSON(400, gin.H{
                "error":        "tenant ID missing or invalid",
                "resolver_type": string(cfg.resolverType),
            })
            c.Abort()
            return
        }

        // Convert string to int
        tenantID, err := strconv.Atoi(tenantIDStr)
        if err != nil {
            c.JSON(400, gin.H{
                "error":        "tenant ID must be numeric",
                "resolver_type": string(cfg.resolverType),
            })
            c.Abort()
            return
        }

        // Store as int in context
        c.Set("tenant_id", tenantID)
        c.Next()
    }
}

// GetTenantID retrieves tenant ID from context as int
// Returns 0 if not found
func GetTenantID(c *gin.Context) int {
    if id, exists := c.Get("tenant_id"); exists {
        return id.(int)
    }
    return 0
}

// GetTenantIDAsInt retrieves tenant ID with error
// Returns error if tenant ID is 0 (not found)
func GetTenantIDAsInt(c *gin.Context) (int, error) {
    id := GetTenantID(c)
    if id == 0 {
        return 0, errors.New("tenant ID not found in context")
    }
    return id, nil
}

// MustGetTenantID retrieves tenant ID or panics
func MustGetTenantID(c *gin.Context) int {
    id := GetTenantID(c)
    if id == 0 {
        panic("tenant ID not found in context")
    }
    return id
}
```

---

### Phase 4: Testing

#### 4.1 Application Tests

```go
func TestApplication_TenantDB_Int(t *testing.T) {
    app := NewConfig()
    db := &gorm.DB{}

    // Set with int
    app.SetTenantDB(1, db)

    // Get with int
    result := app.GetTenantDB(1)
    assert.Same(t, db, result)

    // Not found returns nil
    result = app.GetTenantDB(999)
    assert.Nil(t, result)

    // Iterate all tenants
    count := 0
    app.GetTenantDBs(func(tenantID int, db *gorm.DB) bool {
        count++
        return true
    })
    assert.Equal(t, 1, count)
}

func TestApplication_Casbin_Int(t *testing.T) {
    app := NewConfig()
    enforcer := &casbin.SyncedEnforcer{}

    app.SetTenantCasbin(1, enforcer)

    result := app.GetTenantCasbin(1)
    assert.Same(t, enforcer, result)

    result = app.GetTenantCasbin(999)
    assert.Nil(t, result)
}
```

#### 4.2 Middleware Tests

```go
func TestExtractTenantID_StoresInt(t *testing.T) {
    gin.SetMode(gin.TestMode)
    router := gin.New()
    router.Use(middleware.ExtractTenantID(
        middleware.WithResolverType("header"),
    ))
    router.GET("/test", func(c *gin.Context) {
        tenantID := middleware.GetTenantID(c)
        c.JSON(200, gin.H{"tenant_id": tenantID})
    })

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Tenant-ID", "123")
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    // Verify it returns int, not string
    assert.Equal(t, 200, w.Code)
    // Response body should show numeric type
}
```

---

## Execution Order

| Step | File | Action | Priority |
|------|------|--------|----------|
| 1 | `tenant_resolver.go` | **DELETE** | High |
| 2 | `type.go` | Update interface, remove 2 methods | High |
| 3 | `application.go` | Update all methods, remove field | High |
| 4 | `middleware/extract_tenant_id.go` | Store int in context | High |
| 5 | Tests | Update/create tests | Medium |
| 6 | Documentation | Update CLAUDE.md, README | Low |

---

## Verification Checklist

### Code Changes
- [x] `tenant_resolver.go` deleted
- [x] `type.go` - `SetTenantMapping` & `GetTenantID` removed from interface
- [x] `type.go` - All tenantID parameters: `string` → `int`
- [x] `application.go` - `tenantResolver` field removed
- [x] `application.go` - `SetTenantMapping()` & `GetTenantID()` deleted
- [x] `application.go` - All `*"` fallback logic removed
- [x] `application.go` - All sync.Map callbacks use `int` type assertion
- [x] `middleware/` - Stores `int` in context
- [x] `middleware/` - `GetTenantID()` returns `int`

### Testing
- [x] All existing tests pass (middleware: 19/19 passed)
- [x] New tests for int type handling (application_test.go: 9/9 passed)
- [x] Tests verify `*` fallback is removed

### Documentation
- [ ] CLAUDE.md updated (TenantResolver removal noted)
- [ ] Migration guide for consumers (if any external users)

---

## Breaking Changes for Consumers

### Removed APIs

```go
// DELETED - No replacement needed
app.SetTenantMapping(host, tenantID string)
app.GetTenantID(host) string
```

### Changed APIs

```go
// Before
app.SetTenantDB("tenant-123", db)
app.SetTenantCasbin("tenant-123", enforcer)
db := app.GetTenantDB("tenant-123")
enforcer := app.GetTenantCasbin("tenant-123")

// After
app.SetTenantDB(123, db)
app.SetTenantCasbin(123, enforcer)
db := app.GetTenantDB(123)
enforcer := app.GetTenantCasbin(123)
```

### Migration Path for Consumers

```go
// 1. Update tenant extraction - use new middleware
import "github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/middleware"

// Old way (deleted)
// app.SetTenantMapping("tenant1.example.com", "123")
// tenantID := app.GetTenantID(r.Host)

// New way
router.Use(middleware.ExtractTenantID(
    middleware.WithResolverType("host"),
))
tenantID := middleware.GetTenantID(c)  // Returns int directly

// 2. Update all string tenantID to int
// Before: tenantID := "123"
// After:  tenantID := 123

// 3. Handle nil explicitly (no "*" fallback)
// Before: GetTenantDB would return "*" DB if not found
// After: Check if nil and handle appropriately
db := app.GetTenantDB(tenantID)
if db == nil {
    return errors.New("tenant DB not found")
}
```

---

## Replacement Guide

### Old Pattern → New Pattern

| Old Pattern | New Pattern |
|-------------|-------------|
| `app.SetTenantMapping(host, "123")` | Configure in ETCD (tenant-service) |
| `app.GetTenantID(host)` | `middleware.ExtractTenantID()` |
| `app.GetTenantDB("*")` for default | `app.GetTenantDB(1)` for default |
| Manual `LoadFromMap()` | Provider `StartWatch()` auto-sync |

---

## Rationale for TenantResolver Removal

### Functional Overlap

| Feature | TenantResolver | pkg/tenant/ |
|---------|---------------|--------------|
| Host-based resolution | ✅ Basic | ✅ Advanced (with port handling) |
| Header-based resolution | ❌ No | ✅ Yes (customizable) |
| Query-based resolution | ❌ No | ✅ Yes (customizable) |
| Path-based resolution | ❌ No | ✅ Yes (index-based) |
| Configuration source | Manual map | ETCD (real-time) |
| Type safety | string (weak) | int (strong) |
| Error handling | Returns "", false | Validation + HTTP error |

### Conclusion

`TenantResolver` provides **no unique value** over `pkg/tenant/`:
1. Fewer features (only host vs 4 resolver types)
2. Manual configuration (vs ETCD auto-sync)
3. Weaker typing (string vs int)
4. No fallback mechanism (no ETCD integration)

**Decision**: Complete removal to avoid architectural confusion.

---

## Estimated Impact

| Component | Methods | Action | Complexity |
|-----------|---------|--------|------------|
| `tenant_resolver.go` | 5 | DELETE | Low |
| `type.go` | 20 | Update + 2 delete | Low |
| `application.go` | 20 | Update + 1 field delete | Medium |
| `middleware/` | 4 | Update context storage | Medium |
| Tests | - | Update + new tests | Medium |
| **Total** | **~49** | **Complete refactor** | **Medium** |
