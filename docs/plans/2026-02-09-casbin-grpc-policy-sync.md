# Casbin gRPC Policy Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace direct database connections with gRPC-based policy synchronization, allowing all microservices to fetch Casbin policies from security-management without database coupling.

**Architecture:** Transport-agnostic PolicyProvider interface in jxt-core SDK, with gRPC implementations in each microservice. security-management provides the gRPC service endpoint. Only LoadPolicy() uses gRPC (low-frequency); Enforce() remains in-memory (zero-latency).

**Tech Stack:** Go 1.24.0, gRPC, Casbin v2.54.0, GORM, Redis Watcher, ETCD

---

## Overview

This plan implements the gRPC-based Casbin policy synchronization architecture as described in `docs/sys_casbin_rule 表通过grpc同步到各个微服务/详细设计.md`. The implementation is divided into two phases:

1. **Phase 1: Infrastructure** (1-2 weeks) - jxt-core SDK changes and security-management gRPC server
2. **Phase 2: Migration** (1-2 weeks) - Update all microservices to use the new provider pattern

**Key Design Decision:** Uses PolicyProvider interface pattern (not direct gRPC proto dependency in jxt-core) to keep jxt-core transport-agnostic.

---

## Phase 1: Infrastructure (jxt-core + security-management)

### Task 1: Create PolicyProvider interface and PolicyRule struct

**Files:**
- Create: `sdk/pkg/casbin/provider.go`

**Step 1: Write the failing test**

Create `sdk/pkg/casbin/provider_test.go`:

```go
package mycasbin

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
)

// TestPolicyRule_VerifyFields verifies PolicyRule has all required fields
func TestPolicyRule_VerifyFields(t *testing.T) {
    rule := PolicyRule{
        PType: "p",
        V0:    "admin",
        V1:    "/api/v1/*",
        V2:    "GET",
        V3:    "",
        V4:    "",
        V5:    "",
    }

    assert.Equal(t, "p", rule.PType)
    assert.Equal(t, "admin", rule.V0)
    assert.Equal(t, "/api/v1/*", rule.V1)
    assert.Equal(t, "GET", rule.V2)
}

// TestPolicyProvider_InterfaceExists verifies PolicyProvider interface exists
func TestPolicyProvider_InterfaceExists(t *testing.T) {
    // This test will fail until PolicyProvider interface is defined
    var provider interface{} = &mockProvider{}
    _, ok := provider.(PolicyProvider)
    assert.True(t, ok, "PolicyProvider interface should be defined")
}

// mockProvider is a test implementation of PolicyProvider
type mockProvider struct{}

func (m *mockProvider) GetPolicies(ctx context.Context, tenantID int) ([]PolicyRule, error) {
    return []PolicyRule{
        {PType: "p", V0: "admin", V1: "/api/test", V2: "GET"},
    }, nil
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/casbin/provider_test.go -v`
Expected: FAIL with "undefined: PolicyProvider" and "undefined: PolicyRule"

**Step 3: Write minimal implementation**

Create `sdk/pkg/casbin/provider.go`:

```go
package mycasbin

import (
    "context"
)

// PolicyRule represents a single Casbin policy rule
type PolicyRule struct {
    PType string // Policy type: "p" (policy) or "g" (role inheritance)
    V0    string // Usually sub (role name)
    V1    string // Usually obj (resource path)
    V2    string // Usually act (HTTP method)
    V3    string // Optional extension field
    V4    string // Optional extension field
    V5    string // Optional extension field
}

// PolicyProvider is the strategy interface for providing policy data
// Microservices implement this interface to supply policies from any source (gRPC, HTTP, DB, etc.)
type PolicyProvider interface {
    // GetPolicies retrieves all policy rules for the specified tenant
    //
    // Parameters:
    //   - ctx: Context (for timeout control, cancellation, etc.)
    //   - tenantID: Tenant identifier
    //
    // Returns:
    //   - []PolicyRule: List of policy rules (including both p and g types)
    //   - error: Error information
    GetPolicies(ctx context.Context, tenantID int) ([]PolicyRule, error)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./sdk/pkg/casbin/provider_test.go -v`
Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/casbin/provider.go sdk/pkg/casbin/provider_test.go
git commit -m "feat(casbin): add PolicyProvider interface and PolicyRule struct

This adds the transport-agnostic PolicyProvider interface that microservices
will implement to fetch Casbin policies from remote sources (gRPC).

Part of Phase 1: Casbin gRPC Policy Sync"
```

---

### Task 2: Create ProviderAdapter (read-only Casbin adapter)

**Files:**
- Create: `sdk/pkg/casbin/provider_adapter.go`
- Create: `sdk/pkg/casbin/provider_adapter_test.go`

**Step 1: Write the failing test**

Create `sdk/pkg/casbin/provider_adapter_test.go`:

```go
package mycasbin

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/casbin/casbin/v2/model"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// mockProviderForAdapter is a mock PolicyProvider for testing
type mockProviderForAdapter struct {
    policies []PolicyRule
    err      error
    delay    time.Duration
}

func (m *mockProviderForAdapter) GetPolicies(ctx context.Context, tenantID int) ([]PolicyRule, error) {
    if m.delay > 0 {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(m.delay):
        }
    }
    if m.err != nil {
        return nil, m.err
    }
    return m.policies, nil
}

// TestNewProviderAdapter verifies adapter creation
func TestNewProviderAdapter(t *testing.T) {
    provider := &mockProviderForAdapter{}
    adapter := NewProviderAdapter(provider, 1)

    assert.NotNil(t, adapter)
}

// TestProviderAdapter_LoadPolicy_Success verifies successful policy loading
func TestProviderAdapter_LoadPolicy_Success(t *testing.T) {
    provider := &mockProviderForAdapter{
        policies: []PolicyRule{
            {PType: "p", V0: "admin", V1: "/api/v1/*", V2: "GET"},
            {PType: "p", V0: "user", V1: "/api/read", V2: "GET"},
        },
    }

    adapter := NewProviderAdapter(provider, 1)

    m, err := model.NewModelFromString(text)
    require.NoError(t, err)

    err = adapter.LoadPolicy(m)
    assert.NoError(t, err)

    // Verify policies were loaded
    policies := m.GetPolicy("p", "p")
    assert.Len(t, policies, 2)
}

// TestProviderAdapter_LoadPolicy_ProviderError verifies error handling
func TestProviderAdapter_LoadPolicy_ProviderError(t *testing.T) {
    provider := &mockProviderForAdapter{
        err: errors.New("provider failed"),
    }

    adapter := NewProviderAdapter(provider, 1)

    m, _ := model.NewModelFromString(text)
    err := adapter.LoadPolicy(m)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "provider GetPolicies failed")
}

// TestPolicyRuleToLine verifies policy rule to line conversion
func TestPolicyRuleToLine(t *testing.T) {
    tests := []struct {
        name     string
        rule     PolicyRule
        expected string
    }{
        {
            name:     "three fields",
            rule:     PolicyRule{PType: "p", V0: "admin", V1: "/api/test", V2: "GET"},
            expected: "p, admin, /api/test, GET",
        },
        {
            name:     "two fields",
            rule:     PolicyRule{PType: "p", V0: "admin", V1: "/api/test"},
            expected: "p, admin, /api/test",
        },
        {
            name:     "g type with two fields",
            rule:     PolicyRule{PType: "g", V0: "alice", V1: "admin"},
            expected: "g, alice, admin",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := policyRuleToLine(tt.rule)
            assert.Equal(t, tt.expected, result)
        })
    }
}

// TestProviderAdapter_LoadPolicy_Timeout verifies timeout handling
func TestProviderAdapter_LoadPolicy_Timeout(t *testing.T) {
    provider := &mockProviderForAdapter{
        delay: 10 * time.Second, // Longer than adapter timeout
    }

    adapter := NewProviderAdapter(provider, 1)

    m, _ := model.NewModelFromString(text)
    err := adapter.LoadPolicy(m)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "context deadline exceeded")
}

// TestProviderAdapter_ReadOnlyMethods verifies write operations return errors
func TestProviderAdapter_ReadOnlyMethods(t *testing.T) {
    provider := &mockProviderForAdapter{}
    adapter := NewProviderAdapter(provider, 1)

    m, _ := model.NewModelFromString(text)

    assert.Error(t, adapter.SavePolicy(m))
    assert.Error(t, adapter.AddPolicy("p", "p", []string{"admin", "/api", "GET"}))
    assert.Error(t, adapter.RemovePolicy("p", "p", []string{"admin", "/api", "GET"}))
    assert.Error(t, adapter.RemoveFilteredPolicy("p", "p", 0))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/casbin/provider_adapter_test.go -v`
Expected: FAIL with "undefined: NewProviderAdapter" and "undefined: policyRuleToLine"

**Step 3: Write minimal implementation**

Create `sdk/pkg/casbin/provider_adapter.go`:

```go
package mycasbin

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/casbin/casbin/v2/model"
    "github.com/casbin/casbin/v2/persist"
)

// ProviderAdapter implements Casbin's persist.Adapter interface using a PolicyProvider
// This is a read-only adapter - write operations return errors
type ProviderAdapter struct {
    provider PolicyProvider
    tenantID int
    timeout  time.Duration
}

// NewProviderAdapter creates a new ProviderAdapter
func NewProviderAdapter(provider PolicyProvider, tenantID int) *ProviderAdapter {
    return &ProviderAdapter{
        provider: provider,
        tenantID: tenantID,
        timeout:  5 * time.Second, // Default timeout for Provider calls
    }
}

// LoadPolicy loads all policies from the Provider into the model
// This is called by Casbin SyncedEnforcer.LoadPolicy()
func (a *ProviderAdapter) LoadPolicy(m model.Model) error {
    ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
    defer cancel()

    rules, err := a.provider.GetPolicies(ctx, a.tenantID)
    if err != nil {
        return fmt.Errorf("provider GetPolicies failed: %w", err)
    }

    // Use persist.LoadPolicyLine to load each policy into the model
    // This is Casbin's official API for loading policies from text format
    for _, rule := range rules {
        line := policyRuleToLine(rule)
        persist.LoadPolicyLine(line, m)
    }

    return nil
}

// policyRuleToLine converts PolicyRule to Casbin policy line format
// Format: "p_type, v0, v1, v2, ..." (comma-separated, non-empty fields only)
//
// Precondition: PolicyRule fields must be filled contiguously from left to right,
// without gaps. For example, V0="admin", V1="", V2="/api/test" is invalid.
// This is Casbin's standard data format constraint - data written by gormAdapter
// naturally satisfies this condition.
func policyRuleToLine(r PolicyRule) string {
    parts := []string{r.PType}
    for _, v := range []string{r.V0, r.V1, r.V2, r.V3, r.V4, r.V5} {
        if v == "" {
            break // Casbin policy fields are filled left-to-right, stop at first empty
        }
        parts = append(parts, v)
    }
    return strings.Join(parts, ", ")
}

// SavePolicy is not supported (read-only adapter)
func (a *ProviderAdapter) SavePolicy(m model.Model) error {
    return fmt.Errorf("ProviderAdapter is read-only: SavePolicy not supported")
}

// AddPolicy is not supported (read-only adapter)
func (a *ProviderAdapter) AddPolicy(sec string, ptype string, rule []string) error {
    return fmt.Errorf("ProviderAdapter is read-only: AddPolicy not supported")
}

// RemovePolicy is not supported (read-only adapter)
func (a *ProviderAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
    return fmt.Errorf("ProviderAdapter is read-only: RemovePolicy not supported")
}

// RemoveFilteredPolicy is not supported (read-only adapter)
func (a *ProviderAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
    return fmt.Errorf("ProviderAdapter is read-only: RemoveFilteredPolicy not supported")
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./sdk/pkg/casbin/provider_adapter_test.go -v`
Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/casbin/provider_adapter.go sdk/pkg/casbin/provider_adapter_test.go
git commit -m "feat(casbin): add ProviderAdapter for read-only policy loading

Implements persist.Adapter interface using PolicyProvider.
Only LoadPolicy is supported; write operations return errors.

Includes tests for:
- Successful policy loading
- Provider error handling
- Timeout handling
- Read-only method enforcement

Part of Phase 1: Casbin gRPC Policy Sync"
```

---

### Task 3: Extract setupRedisWatcherForEnforcer helper function

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`
- Create: `sdk/pkg/casbin/redis_watcher_test.go`

**Step 1: Write the test first**

Create `sdk/pkg/casbin/redis_watcher_test.go`:

```go
package mycasbin

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// TestSetupRedisWatcherForEnforcer_NoRedis verifies behavior when Redis is not configured
func TestSetupRedisWatcherForEnforcer_NoRedis(t *testing.T) {
    // This test verifies that when Redis is not configured,
    // setupRedisWatcherForEnforcer does not panic
    // Note: This requires config.CacheConfig.Redis to be nil

    // Since we can't easily mock config.CacheConfig in the current setup,
    // this test documents the expected behavior
    t.Skip("Requires config mock or integration test setup")
}

// TestSetupRedisWatcherForEnforcer_CreatesWatcher verifies Redis Watcher creation
func TestSetupRedisWatcherForEnforcer_CreatesWatcher(t *testing.T) {
    t.Skip("Requires actual Redis or integration test setup")
}
```

**Step 2: Run existing tests to ensure current behavior**

Run: `go test ./sdk/pkg/casbin/mycasbin_test.go -v`
Expected: Current tests should pass (baseline)

**Step 3: Refactor SetupForTenant to extract helper**

Modify `sdk/pkg/casbin/mycasbin.go` - add helper function after SetupForTenant:

```go
// setupRedisWatcherForEnforcer sets up Redis Watcher for an enforcer
// Called by both SetupForTenant and SetupWithProvider to eliminate code duplication
//
// Behavior:
//   - If Redis is configured, creates Watcher and sets callback
//   - If Redis is not configured or creation fails, logs only (does not prevent enforcer creation)
//   - Each tenant uses a dedicated Redis channel: /casbin/tenant/{tenantID}
func setupRedisWatcherForEnforcer(e *casbin.SyncedEnforcer, tenantID int) {
    if config.CacheConfig.Redis == nil {
        return
    }

    channel := fmt.Sprintf("/casbin/tenant/%d", tenantID)

    w, err := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
        Options: redis.Options{
            Network:  "tcp",
            Password: config.CacheConfig.Redis.Password,
        },
        Channel:    channel,
        IgnoreSelf: false,
    })
    if err != nil {
        logger.Errorf("租户 %d Redis Watcher 创建失败: %v", tenantID, err)
        return
    }

    // Capture enforcer in closure for this tenant
    tenantEnforcer := e
    _ = w.SetUpdateCallback(func(msg string) {
        logger.Infof("casbin updateCallback (租户 %d) msg: %v", tenantID, msg)
        if err := tenantEnforcer.LoadPolicy(); err != nil {
            logger.Errorf("casbin LoadPolicy (租户 %d) err: %v", tenantID, err)
        }
    })
    _ = e.SetWatcher(w)
}
```

**Step 4: Update SetupForTenant to use the helper**

Modify `sdk/pkg/casbin/mycasbin.go` - replace the Redis Watcher section (lines 93-126):

```go
    // 5. Set up Redis Watcher using shared helper
    setupRedisWatcherForEnforcer(e, tenantID)
```

**Step 5: Run tests to verify no regression**

Run: `go test ./sdk/pkg/casbin/... -v`
Expected: All existing tests pass

**Step 6: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go sdk/pkg/casbin/redis_watcher_test.go
git commit -m "refactor(casbin): extract setupRedisWatcherForEnforcer helper

Extracts Redis Watcher setup code into a shared helper function
to eliminate duplication between SetupForTenant and SetupWithProvider.

Part of Phase 1: Casbin gRPC Policy Sync"
```

---

### Task 4: Implement SetupWithProvider function

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`
- Create: `sdk/pkg/casbin/setup_with_provider_test.go`

**Step 1: Write the failing test**

Create `sdk/pkg/casbin/setup_with_provider_test.go`:

```go
package mycasbin

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// mockProviderForSetup is a mock for testing SetupWithProvider
type mockProviderForSetup struct {
    policies []PolicyRule
}

func (m *mockProviderForSetup) GetPolicies(ctx context.Context, tenantID int) ([]PolicyRule, error) {
    return m.policies, nil
}

// TestSetupWithProvider_Success verifies successful enforcer creation
func TestSetupWithProvider_Success(t *testing.T) {
    provider := &mockProviderForSetup{
        policies: []PolicyRule{
            {PType: "p", V0: "admin", V1: "/api/v1/*", V2: "GET"},
        },
    }

    enforcer, err := SetupWithProvider(provider, 1)

    require.NoError(t, err)
    assert.NotNil(t, enforcer)

    // Verify policy was loaded
    policies := enforcer.GetPolicy()
    assert.Len(t, policies, 1)
    assert.Equal(t, []string{"admin", "/api/v1/*", "GET"}, policies[0])
}

// TestSetupWithProvider_NoPolicies verifies enforcer creation with empty policies
func TestSetupWithProvider_NoPolicies(t *testing.T) {
    provider := &mockProviderForSetup{
        policies: []PolicyRule{},
    }

    enforcer, err := SetupWithProvider(provider, 1)

    require.NoError(t, err)
    assert.NotNil(t, enforcer)

    policies := enforcer.GetPolicy()
    assert.Len(t, policies, 0)
}

// TestSetupWithProvider_MultipleTenments verifies tenant isolation
func TestSetupWithProvider_MultipleTenants(t *testing.T) {
    provider1 := &mockProviderForSetup{
        policies: []PolicyRule{{PType: "p", V0: "admin", V1: "/api/v1/*", V2: "GET"}},
    }
    provider2 := &mockProviderForSetup{
        policies: []PolicyRule{{PType: "p", V0: "user", V1: "/api/v2/*", V2: "GET"}},
    }

    enforcer1, err := SetupWithProvider(provider1, 1)
    require.NoError(t, err)

    enforcer2, err := SetupWithProvider(provider2, 2)
    require.NoError(t, err)

    // Verify enforcers are different instances
    assert.NotSame(t, enforcer1, enforcer2)

    // Verify policies are isolated
    policies1 := enforcer1.GetPolicy()
    policies2 := enforcer2.GetPolicy()

    assert.Len(t, policies1, 1)
    assert.Len(t, policies2, 1)
    assert.NotEqual(t, policies1[0], policies2[0])
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./sdk/pkg/casbin/setup_with_provider_test.go -v`
Expected: FAIL with "undefined: SetupWithProvider"

**Step 3: Write minimal implementation**

Add to `sdk/pkg/casbin/mycasbin.go` after SetupForTenant:

```go
// SetupWithProvider creates a Casbin enforcer using a PolicyProvider (for microservices)
// Unlike SetupForTenant, this does not require a local database connection
//
// ⚠️  For non-security-management microservices only (remote fetch mode)
// security-management should continue using SetupForTenant
//
// Parameters:
//   - provider: Policy provider (implemented by microservice, typically a gRPC adapter)
//   - tenantID: Tenant identifier
//
// Returns:
//   - *casbin.SyncedEnforcer: Enforcer with loaded policies
//   - error: Initialization error
func SetupWithProvider(provider PolicyProvider, tenantID int) (*casbin.SyncedEnforcer, error) {
    // 1. Create ProviderAdapter
    adapter := NewProviderAdapter(provider, tenantID)

    // 2. Load the same RBAC model as SetupForTenant
    m, err := model.NewModelFromString(text) // text variable is already defined in mycasbin.go
    if err != nil {
        return nil, fmt.Errorf("failed to create casbin model: %w", err)
    }

    // 3. Create SyncedEnforcer
    e, err := casbin.NewSyncedEnforcer(m, adapter)
    if err != nil {
        return nil, fmt.Errorf("failed to create casbin enforcer: %w", err)
    }

    // 4. Load policies (via ProviderAdapter → Provider → remote service)
    if err := e.LoadPolicy(); err != nil {
        return nil, fmt.Errorf("failed to load policy via provider: %w", err)
    }

    // 5. Set up Redis Watcher using shared helper
    setupRedisWatcherForEnforcer(e, tenantID)

    // 6. Enable logging
    log.SetLogger(&Logger{})
    e.EnableLog(true)

    return e, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./sdk/pkg/casbin/setup_with_provider_test.go -v`
Expected: PASS

**Step 5: Update function documentation comments**

Update `SetupForTenant` comment in `sdk/pkg/casbin/mycasbin.go` to add warning:

```go
// SetupForTenant creates an independent Casbin enforcer for the specified tenant
// Each tenant has its own adapter, enforcer instance, and Redis Watcher channel
//
// ⚠️  For security-management internal use only (local database mode)
// Other microservices should use SetupWithProvider
//
// Parameters:
```

**Step 6: Run all tests to verify no regression**

Run: `go test ./sdk/pkg/casbin/... -v`
Expected: All tests pass

**Step 7: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go sdk/pkg/casbin/setup_with_provider_test.go
git commit -m "feat(casbin): add SetupWithProvider for microservices

New function for microservices to fetch Casbin policies via PolicyProvider
instead of direct database connections. Uses ProviderAdapter internally.

Includes tests for:
- Successful enforcer creation
- Empty policy handling
- Multi-tenant isolation

Part of Phase 1: Casbin gRPC Policy Sync"
```

---

### Task 5: Update README.md

**Files:**
- Modify: `sdk/pkg/casbin/README.md`

**Step 1: Add SetupWithProvider documentation**

Add new section after "SetupForTenant" section:

```markdown
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
```

**Step 2: Update Migration Guide section**

```markdown
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
```

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/README.md
git commit -m "docs(casbin): update README with SetupWithProvider documentation

Documents the new SetupWithProvider API and updated migration guide.

Part of Phase 1: Casbin gRPC Policy Sync"
```

---

## Phase 1 Completion Checklist

- [ ] All unit tests pass: `go test ./sdk/pkg/casbin/... -v`
- [ ] No compilation errors
- [ ] README updated
- [ ] Code follows existing patterns

**After Phase 1 completion**, the jxt-core SDK is ready for microservices to implement gRPC providers. Phase 2 (security-management gRPC server and microservice migrations) is done in separate repositories.

---

## Phase 2: Migration (Separate Repositories)

The following tasks are for other repositories and should be executed using `superpowers:executing-plans` in separate sessions:

### Task 6: security-management gRPC Server (security-management repository)

**Files to create in security-management:**
- `admin/domain/aggregate/repository/casbin_rule.go` - Repository interface
- `admin/infrastructure/persistence/gorm/casbin_rule_repository.go` - GORM implementation
- `admin/domain/aggregate/casbin_rule.go` - CasbinRule aggregate
- `admin/interface/grpc/casbin/proto/casbin_policy.proto` - Proto definition
- `admin/interface/grpc/casbin/casbin_policy_server.go` - gRPC server
- `admin/interface/grpc/casbin/dependencies.go` - DI registration

**Validation:** `grpcurl -d '{"tenant_id": 1}' localhost:9000 casbin.CasbinPolicyService/GetPolicies`

### Task 7: evidence-migration Migration (evidence-management repository)

**Files to modify in evidence-management:**
- `shared/infrastructure/grpc/proto/casbin/` - Add proto files
- `shared/infrastructure/grpc/client/casbin_policy_provider.go` - gRPC provider
- `shared/common/database/initialize.go` - Replace Setup() with SetupWithProvider()
- Remove `masterDB` configuration

### Task 8: file-storage-service Migration (file-storage-service repository)

**Files to modify in file-storage-service:**
- Add `infrastructure/grpc/` directory if not exists
- Add proto files and provider
- Update initialize.go to use SetupWithProvider()

### Task 9: tenant-service Migration (tenant-service repository)

**Files to modify in tenant-service:**
- Add gRPC client infrastructure if not exists
- Add provider implementation
- Update initialize.go

---

## Notes for Execution

1. **Test Coverage:** Each new function should have unit tests covering success, error, and edge cases
2. **No Regressions:** Run full test suite after each task
3. **Documentation:** Update comments and README as code changes
4. **Commit Messages:** Use conventional commits with "(casbin)" scope

---

## Rollback Plan

If issues arise during Phase 2 migration:

1. Revert microservice changes: `git revert <commit-hash>`
2. Restore old `Setup()` calls temporarily
3. Keep Phase 1 changes (they don't affect existing services)
4. Fix issues and retry migration

Phase 1 changes are backward compatible and can be merged without affecting existing services.
