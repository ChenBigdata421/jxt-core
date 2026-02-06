# Casbin Multi-Tenant Support

This package provides Casbin enforcer setup with multi-tenant support.

## Migration Guide

### Old Usage (Deprecated)

The old `Setup` function uses a singleton pattern that breaks multi-tenant isolation:

```go
// ❌ 所有租户共享同一个 enforcer
enforcer := mycasbin.Setup(db, "")
```

### New Usage (Recommended)

Use `SetupForTenant` for proper multi-tenant isolation:

```go
// ✅ 每个租户独立的 enforcer
enforcer, err := mycasbin.SetupForTenant(db, tenantID)
if err != nil {
    log.Fatalf("初始化 Casbin 失败: %v", err)
}
```

## API Reference

### SetupForTenant

```go
func SetupForTenant(db *gorm.DB, tenantID int) (*casbin.SyncedEnforcer, error)
```

Creates an independent Casbin enforcer for the specified tenant.

**Parameters:**
- `db`: GORM database connection for the tenant's database
- `tenantID`: Tenant identifier (used for logging and future Redis Watcher channel isolation)

**Returns:**
- `*casbin.SyncedEnforcer`: Tenant-specific enforcer instance
- `error`: Error if setup fails

**Example:**

```go
db := getTenantDatabase(tenantID)
enforcer, err := mycasbin.SetupForTenant(db, tenantID)
if err != nil {
    return fmt.Errorf("租户 %d Casbin 初始化失败: %w", tenantID, err)
}

// Use enforcer for permission checks
allowed, err := enforcer.Enforce("user", "/api/v1/resource", "GET")
```

### Setup (Deprecated)

```go
func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer
```

Legacy function for backward compatibility. Creates a single global enforcer.

**Deprecated:** Use `SetupForTenant` instead for proper multi-tenant support.

## Redis Watcher

Each tenant gets a dedicated Redis channel for policy synchronization:

```go
// Tenant 1 uses channel: /casbin/tenant/1
// Tenant 2 uses channel: /casbin/tenant/2
// etc.
```

When a tenant's policies are modified in the database:
1. The modification publishes a message to `/casbin/tenant/{tenantID}`
2. All instances running that tenant subscribe to the message
3. Each instance's callback reloads policies from the tenant's database
4. The enforcer is updated with the latest policies

**Graceful Degradation:**
- If Redis is unavailable during SetupForTenant, the enforcer is still created successfully
- A warning is logged, but the tenant can still function
- Policy changes won't be auto-synchronized until Redis is available

## Testing

Run tests:

```bash
# Unit tests (mock-based)
go test ./sdk/pkg/casbin/... -short

# Integration tests (requires test database)
go test ./sdk/pkg/casbin/... -v

# With coverage
go test ./sdk/pkg/casbin/... -cover -coverprofile=coverage.out
```

## Known Issues

None (as of Stage 2). All multi-tenant isolation issues have been fixed.

## Roadmap

- **Stage 1**: Remove singleton, add `SetupForTenant` ✅
- **Stage 2** (Current): Redis Watcher per-tenant isolation ✅
- **Stage 3**: Update security-management to use new APIs (pending)

## Related Documents

- [Casbin Multi-Tenant Isolation Issue Analysis](../../../docs/权限检查的租户隔离问题/casbin-multi-tenant-isolation-issue.md)
- [Fix Implementation Plan](../../../docs/权限检查的租户隔离问题/2026-02-06-fix-casbin-multi-tenant-isolation.md)
