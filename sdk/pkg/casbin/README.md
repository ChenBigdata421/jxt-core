# Casbin Multi-Tenant Support

This package provides Casbin enforcer setup with multi-tenant support.

## Migration Guide

### Old Usage (Deprecated)

```go
// ❌ Old: Direct database connection
enforcer := mycasbin.Setup(db, "")
```

### For security-management (Local Database)

```go
// ✅ security-management uses local database
enforcer, err := mycasbin.SetupForTenant(db, tenantID)
```

### For Microservices (Remote Provider)

```go
// ✅ Microservices use remote provider
provider := grpc_client.NewGrpcCasbinPolicyProvider(connManager)
enforcer, err := mycasbin.SetupWithProvider(provider, tenantID)
```

## API Reference

### SetupForTenant

```go
func SetupForTenant(db *gorm.DB, tenantID int) (*casbin.SyncedEnforcer, error)
```

Creates an independent Casbin enforcer for the specified tenant.

**Parameters:**
- `db`: GORM database connection for the tenant's database
- `tenantID`: Tenant identifier (used for logging and to pin this tenant's policy-sync messages to a dedicated dispatch shard via the PSUBSCRIBE multiplexer)

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

### SetupWithProvider

```go
func SetupWithProvider(provider PolicyProvider, tenantID int) (*casbin.SyncedEnforcer, error)
```

Creates a Casbin enforcer using a PolicyProvider instead of a database connection.

**Parameters:**
- `provider`: Policy provider implementation (typically gRPC-based)
- `tenantID`: Tenant identifier

**Returns:**
- `*casbin.SyncedEnforcer`: Enforcer with loaded policies
- `error`: Error if setup fails

**Example:**

```go
provider := grpc_client.NewGrpcCasbinPolicyProvider(connManager)
enforcer, err := mycasbin.SetupWithProvider(provider, tenantID)
if err != nil {
    log.Fatalf("Casbin init failed: %v", err)
}

// Use enforcer for permission checks
allowed, err := enforcer.Enforce("user", "/api/v1/resource", "GET")
```

**Usage:**
- For microservices (evidence-management, file-storage-service, tenant-service)
- NOT for security-management (use SetupForTenant instead)

### Setup (Deprecated)

```go
func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer
```

Legacy function for backward compatibility. Creates a single global enforcer.

**Deprecated:** Use `SetupForTenant` instead for proper multi-tenant support.

## Redis Watcher (PSUBSCRIBE Multiplexer)

Policy synchronization uses a **single `PSUBSCRIBE` connection** (the
`CasbinWatcherMux` singleton in `watcher_mux.go`) rather than one subscriber per
tenant. New tenants match the wildcard pattern automatically, with **zero
additional connection cost**.

### Three-client Redis model

The watcher relies on jxt-core's split Redis client architecture
(`sdk/config/option_redis.go`):

| Client | Role | Used by the watcher |
|--------|------|---------------------|
| #1 shared | Non-blocking operations | `PUBLISH` to `/casbin/tenant/{tenantID}` |
| #2 queue consumer | Blocking `XREADGROUP` | — (reserved for the message queue) |
| #3 subscriber | `PSUBSCRIBE` | Listens on `/casbin/tenant/*` for **all** tenants |

### Channel naming

```
PUBLISH target  (per tenant):    /casbin/tenant/{tenantID}
PSUBSCRIBE pattern (all tenants): /casbin/tenant/*
```

### How a policy change propagates

1. A tenant's policies are modified and the watcher publishes an `MSG` to
   `/casbin/tenant/{tenantID}` via Client #1.
2. Every instance's `CasbinWatcherMux` receives the message on the single
   Client #3 `PSUBSCRIBE` connection.
3. Each message is routed to a dedicated dispatch shard via `tenantID % 8`, so
   **all messages for one tenant are processed strictly in publish order** by a
   single worker (8 workers, each with a bounded 256-message queue).
4. The worker applies the update to that tenant's enforcer (`LoadPolicy` or an
   incremental `SelfAdd/Remove/UpdatePolicy`), skipping messages it published
   itself (UUID-based self-ignore).

### Wire format

The `MSG` payload uses a fixed on-wire format. Do not change the field set or
JSON tag names — instances running different jxt-core versions can coexist in
the same Redis during a rolling upgrade only as long as that format stays stable.

### Lifecycle

- `InitCasbinWatcherMux(pubClient, subClient)` — starts the background listener
  and the 8-shard worker pool. Idempotent (`sync.Once`); safe to call from
  concurrent goroutines.
- `ShutdownCasbinWatcherMux()` — drains the workers and listener so Redis
  clients can be closed safely. Wired into `Application.Close()`.
- The subscription reconnects with exponential backoff (1s → 30s cap), reset to
  the minimum after each successfully received message.

### Graceful Degradation

- If Redis is unavailable during `SetupForTenant`, the enforcer is still created
  successfully.
- A warning is logged, but the tenant can still function.
- `PUBLISH` becomes a no-op when Redis is absent, so policy changes won't
  auto-synchronize until Redis is available.

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

None (as of Stage 3). All multi-tenant isolation issues have been fixed.

## Roadmap

- **Stage 1**: Remove singleton, add `SetupForTenant` ✅
- **Stage 2**: Redis Watcher per-tenant isolation ✅
- **Stage 3**: PSUBSCRIBE multiplexer — single-connection all-tenant sync with 8-shard dispatch preserving per-tenant ordering ✅
- **Stage 4**: Update security-management to use new APIs (pending)

## Related Documents

- [Casbin Multi-Tenant Isolation Issue Analysis](../../../docs/权限检查的租户隔离问题/casbin-multi-tenant-isolation-issue.md)
- [Fix Implementation Plan](../../../docs/权限检查的租户隔离问题/2026-02-06-fix-casbin-multi-tenant-isolation.md)
