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

1. **Redis Watcher:** The current implementation uses a shared Redis channel (`/casbin`) for all tenants. This will be fixed in Stage 2 with per-tenant channels (`/casbin/tenant/{tenantID}`).

2. **Global updateCallback:** The `updateCallback` function no longer reloads policies since the global enforcer was removed. A warning is logged instead. Stage 2 will implement per-tenant callback closures.

## Roadmap

- **Stage 1** (Current): Remove singleton, add `SetupForTenant` ✅
- **Stage 2**: Redis Watcher per-tenant isolation
- **Stage 3**: Update security-management to use new APIs

## Related Documents

- [Casbin Multi-Tenant Isolation Issue Analysis](../../../docs/权限检查的租户隔离问题/casbin-multi-tenant-isolation-issue.md)
- [Fix Implementation Plan](../../../docs/权限检查的租户隔离问题/2026-02-06-fix-casbin-multi-tenant-isolation.md)
