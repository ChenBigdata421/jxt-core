# Casbin Multi-Tenant Isolation Fix - Stage 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the Casbin multi-tenant isolation issue in jxt-core by removing the `sync.Once` singleton pattern and creating per-tenant independent enforcer instances.

**Architecture:** Remove global state (`enforcer` and `once` variables), create a new `SetupForTenant` function that returns independent enforcers with proper error handling, while keeping `Setup` for backward compatibility.

**Tech Stack:** Go 1.24.0, Casbin v2, GORM, jxt-core v1.1.32, testing with standard Go testing

**Related Documents:**
- [casbin-multi-tenant-isolation-issue.md](../../权限检查的租户隔离问题/casbin-multi-tenant-isolation-issue.md)
- [2026-02-06-fix-casbin-multi-tenant-isolation.md](../../权限检查的租户隔离问题/2026-02-06-fix-casbin-multi-tenant-isolation.md)

---

## Overview

**Problem:** The current `mycasbin.Setup()` uses `sync.Once` singleton pattern, causing all tenants to share the same global enforcer with the first tenant's database connection. This breaks permission isolation.

**Solution (Stage 1):**
- Remove `sync.Once` and global `enforcer` variable
- Create `SetupForTenant(db *gorm.DB, tenantID int) (*casbin.SyncedEnforcer, error)`
- Keep `Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer` for backward compatibility
- Return errors instead of panicking
- Redis Watcher changes deferred to Stage 2

**Files:**
- Modify: [sdk/pkg/casbin/mycasbin.go](../../sdk/pkg/casbin/mycasbin.go)
- Create: [sdk/pkg/casbin/mycasbin_test.go](../../sdk/pkg/casbin/mycasbin_test.go)

---

## Task 1: Add fmt import and remove global variables

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Add fmt import to the import block**

The file currently imports:
```go
import (
	"sync"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	// ...
)
```

Add `"fmt"` to the import block (after `"sync"`):

```go
import (
	"fmt"
	"sync"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/log"
	"github.com/casbin/casbin/v2/model"
	redisWatcher "github.com/go-admin-team/redis-watcher/v2"
	"github.com/go-redis/redis/v9"
	"gorm.io/gorm"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	gormAdapter "github.com/go-admin-team/gorm-adapter/v3"
)
```

**Step 2: Remove the global enforcer and once variables**

Lines 33-36 currently are:
```go
var (
	enforcer *casbin.SyncedEnforcer //策略执行器实例
	once     sync.Once
)
```

Delete these 4 lines (the entire `var` block).

**Step 3: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: FAIL with "undefined: enforcer" or "undefined: once" errors (this confirms we removed dependencies correctly)

**Step 4: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "refactor(casbin): remove global enforcer and once variables in preparation for per-tenant setup"
```

---

## Task 2: Create SetupForTenant function (without Redis Watcher)

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Write the SetupForTenant function**

Add this function immediately after the `var text = ...` block (around line 32, where the global variables were removed):

```go
// SetupForTenant 为指定租户创建独立的 Casbin enforcer
// 每个租户拥有独立的 adapter 和 enforcer 实例
// 参数:
//   - db: 该租户的数据库连接
//   - tenantID: 租户ID（用于日志标识和后续 Redis Watcher 频道隔离）
// 返回:
//   - *casbin.SyncedEnforcer: 该租户专属的 enforcer 实例
//   - error: 错误信息
func SetupForTenant(db *gorm.DB, tenantID int) (*casbin.SyncedEnforcer, error) {
	// 1. 为该租户创建独立的 GORM Adapter
	adapter, err := gormAdapter.NewAdapterByDBUseTableName(db, "sys", "casbin_rule")
	if err != nil && err.Error() != "invalid DDL" {
		return nil, fmt.Errorf("创建 Casbin adapter 失败 (租户 %d): %w", tenantID, err)
	}

	// 2. 加载权限模型
	m, err := model.NewModelFromString(text)
	if err != nil {
		return nil, fmt.Errorf("加载 Casbin 模型失败: %w", err)
	}

	// 3. 创建该租户专属的 SyncedEnforcer
	e, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("创建 Casbin enforcer 失败 (租户 %d): %w", tenantID, err)
	}

	// 4. 从该租户的数据库加载策略
	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("加载 Casbin 策略失败 (租户 %d): %w", tenantID, err)
	}

	// 5. 设置日志
	log.SetLogger(&Logger{})
	e.EnableLog(true)

	// 注意: Redis Watcher 初始化将在 Stage 2 中添加
	// 目前保持与原有 Setup 函数一致的行为，但使用租户隔离的方式

	return e, nil
}
```

**Step 2: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: PASS (the function is defined but not yet used)

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "feat(casbin): add SetupForTenant function for per-tenant enforcer creation"
```

---

## Task 3: Refactor Setup function for backward compatibility

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Replace the existing Setup function**

The current `Setup` function (lines 41-91) should be completely replaced with:

```go
// Setup 为指定租户创建 Casbin enforcer（向后兼容函数）
// 注意: 此函数保留用于向后兼容，新代码应使用 SetupForTenant
// Deprecated: 使用 SetupForTenant 替代，以获得更好的错误处理和多租户支持
func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer {
	e, err := SetupForTenant(db, 0)
	if err != nil {
		// 保持原有行为：发生错误时 panic
		panic(err)
	}

	// 兼容旧版：如果配置了 Redis，设置全局 Watcher
	// 注意: 在 Stage 2 中，这将被移到 SetupForTenant 中实现租户隔离
	if config.CacheConfig.Redis != nil {
		w, wErr := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
			Options: redis.Options{
				Network:  "tcp",
				Password: config.CacheConfig.Redis.Password,
			},
			Channel:    "/casbin",
			IgnoreSelf: false,
		})
		if wErr != nil {
			panic(wErr)
		}

		wErr = w.SetUpdateCallback(updateCallback)
		if wErr != nil {
			panic(wErr)
		}
		wErr = e.SetWatcher(w)
		if wErr != nil {
			panic(wErr)
		}
	}

	return e
}
```

**Step 2: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: FAIL with "undefined: updateCallback" (we'll fix this in Task 4)

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "refactor(casbin): refactor Setup to use SetupForTenant with backward compatibility"
```

---

## Task 4: Modify updateCallback to handle error condition

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Modify updateCallback function**

The current `updateCallback` function (lines 93-100) references the global `enforcer` which no longer exists. Since we're keeping `Setup` for backward compatibility and it creates a single enforcer, we need to handle the callback differently.

For Stage 1, the simplest approach is to make `updateCallback` a no-op with a warning, since the global enforcer no longer exists:

Replace the entire `updateCallback` function (lines 93-100) with:

```go
// updateCallback is kept for backward compatibility but is now a no-op
// In Stage 2, each tenant will have its own callback closure
// Deprecated: This function no longer reloads policy since the global enforcer was removed
func updateCallback(msg string) {
	logger.Infof("casbin updateCallback msg: %v (警告: 全局 enforcer 已移除，此回调不再生效，请使用 SetupForTenant)", msg)
	// 不再执行 LoadPolicy，因为全局 enforcer 已不存在
	// 在 Stage 2 中，每个租户将有自己的 callback 闭包
}
```

**Step 4: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: PASS

**Step 5: Run existing tests (if any)**

Run: `go test ./sdk/pkg/casbin/... -v`

Expected: PASS or no tests (currently no tests exist)

**Step 6: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "refactor(casbin): update updateCallback to no-op with deprecation warning"
```

---

## Task 5: Create unit test file - basic test structure

**Files:**
- Create: `sdk/pkg/casbin/mycasbin_test.go`

**Step 1: Create the test file with basic imports and setup**

Create the file with this content:

```go
package mycasbin

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/casbin/casbin/v2"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mockDB creates a mock GORM DB using sqlmock for testing
func mockDB() (*gorm.DB, *sql.DB, error) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		return nil, nil, err
	}

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB,
	}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	return gormDB, sqlDB, err
}

// realTestDB connects to a test database (requires TEST_DB_URL environment variable)
// For CI/CD, set up a test PostgreSQL database
func realTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost user=postgres password=postgres dbname=postgres_test port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("无法连接到测试数据库: %v (跳过集成测试)", err)
		return nil
	}
	return db
}
```

**Step 2: Verify the test file compiles**

Run: `go test -c ./sdk/pkg/casbin/...`

Expected: PASS (compiles successfully)

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/mycasbin_test.go
git commit -m "test(casbin): add test file with mock and real DB helpers"
```

---

## Task 6: Write test for SetupForTenant creating independent instances

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin_test.go`

**Step 1: Write the test for independent enforcer instances**

Add this test function to `mycasbin_test.go`:

```go
// TestSetupForTenant_IndependentInstances verifies that calling SetupForTenant
// with different database connections creates different enforcer instances
func TestSetupForTenant_IndependentInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	db := realTestDB(t)
	if db == nil {
		return
	}
	defer func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	}()

	// 注意: 由于我们使用相同的数据库连接，adapter 会相同
	// 但 enforcer 实例应该是不同的
	enforcer1, err := SetupForTenant(db, 1)
	assert.NoError(t, err, "SetupForTenant(租户1) 不应返回错误")
	assert.NotNil(t, enforcer1, "SetupForTenant(租户1) 应返回非空 enforcer")

	enforcer2, err := SetupForTenant(db, 2)
	assert.NoError(t, err, "SetupForTenant(租户2) 不应返回错误")
	assert.NotNil(t, enforcer2, "SetupForTenant(租户2) 应返回非空 enforcer")

	// 关键验证: 两个 enforcer 应该是不同的实例
	assert.NotSame(t, enforcer1, enforcer2, "不同租户的 enforcer 应该是不同的实例")
}
```

**Step 2: Run the test**

Run: `go test ./sdk/pkg/casbin/... -v -run TestSetupForTenant_IndependentInstances`

Expected: SKIP (if no test database) or PASS

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/mycasbin_test.go
git commit -m "test(casbin): add test for SetupForTenant independent instances"
```

---

## Task 7: Write test for SetupForTenant error handling

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin_test.go`

**Step 1: Write the error handling test**

Add this test function to `mycasbin_test.go`:

```go
// TestSetupForTenant_ErrorHandling verifies that SetupForTenant returns
// proper errors when given invalid inputs
func TestSetupForTenant_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		db          *gorm.DB
		tenantID    int
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil database should return error",
			db:          nil,
			tenantID:    1,
			wantErr:     true,
			errContains: "adapter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enforcer, err := SetupForTenant(tt.db, tt.tenantID)

			if tt.wantErr {
				assert.Error(t, err, "SetupForTenant 应返回错误")
				assert.Nil(t, enforcer, "出错时 enforcer 应为 nil")
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains,
						"错误信息应包含: "+tt.errContains)
				}
			} else {
				assert.NoError(t, err, "SetupForTenant 不应返回错误")
				assert.NotNil(t, enforcer, "enforcer 不应为 nil")
			}
		})
	}
}
```

**Step 2: Run the test**

Run: `go test ./sdk/pkg/casbin/... -v -run TestSetupForTenant_ErrorHandling`

Expected: PASS

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/mycasbin_test.go
git commit -m "test(casbin): add error handling tests for SetupForTenant"
```

---

## Task 8: Write test for Setup backward compatibility

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin_test.go`

**Step 1: Write the backward compatibility test**

Add this test function to `mycasbin_test.go`:

```go
// TestSetup_BackwardCompatibility verifies that the old Setup function
// still works for existing code
func TestSetup_BackwardCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	db := realTestDB(t)
	if db == nil {
		return
	}
	defer func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	}()

	// Setup 应该仍然工作（不 panic）
	enforcer := Setup(db, "")
	assert.NotNil(t, enforcer, "Setup 应返回非空 enforcer")

	// 验证 enforcer 是有效的 SyncedEnforcer
	_, ok := enforcer.(*casbin.SyncedEnforcer)
	assert.True(t, ok, "enforcer 应该是 *casbin.SyncedEnforcer 类型")
}
```

**Step 2: Run the test**

Run: `go test ./sdk/pkg/casbin/... -v -run TestSetup_BackwardCompatibility`

Expected: SKIP (if no test database) or PASS

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/mycasbin_test.go
git commit -m "test(casbin): add backward compatibility test for Setup function"
```

---

## Task 9: Write test for policy loading verification

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin_test.go`

**Step 1: Write the policy loading test**

Add this test function to `mycasbin_test.go`:

```go
// TestSetupForTenant_PolicyLoading verifies that policies are loaded
// correctly from the database
func TestSetupForTenant_PolicyLoading(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	db := realTestDB(t)
	if db == nil {
		return
	}
	defer func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	}()

	enforcer, err := SetupForTenant(db, 1)
	assert.NoError(t, err, "SetupForTenant 不应返回错误")
	assert.NotNil(t, enforcer, "enforcer 不应为 nil")

	// 验证 enforcer 已正确初始化
	// GetAllSubjects() 返回所有定义的主体（用户）
	subjects := enforcer.GetAllSubjects()
	// 即使没有策略，也应该返回一个空切片而不是 nil
	assert.NotNil(t, subjects, "GetAllSubjects 应返回非空切片")

	// 验证 enforcer 的模型已正确加载
	model := enforcer.GetModel()
	assert.NotNil(t, model, "模型不应为 nil")
}
```

**Step 2: Run the test**

Run: `go test ./sdk/pkg/casbin/... -v -run TestSetupForTenant_PolicyLoading`

Expected: SKIP (if no test database) or PASS

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/mycasbin_test.go
git commit -m "test(casbin): add policy loading verification test"
```

---

## Task 10: Run all tests and verify coverage

**Files:**
- Test: `sdk/pkg/casbin/mycasbin_test.go`

**Step 1: Run all tests in the casbin package**

Run: `go test ./sdk/pkg/casbin/... -v`

Expected: All tests PASS (or SKIP if no test database)

**Step 2: Check test coverage**

Run: `go test ./sdk/pkg/casbin/... -cover -coverprofile=coverage.out`

Expected: Coverage report should show >50% for mycasbin.go

**Step 3: Display coverage details**

Run: `go tool cover -html=coverage.out -o coverage.html`

Expected: HTML coverage report generated

**Step 4: Verify code compiles for entire jxt-core**

Run: `go build ./...`

Expected: PASS (entire jxt-core builds successfully)

**Step 5: Commit**

```bash
git add sdk/pkg/casbin/coverage.out
git commit -m "test(casbin): add coverage report and verify all tests pass"
```

---

## Task 11: Update documentation and add deprecation notices

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Add package-level documentation**

Add this documentation comment at the top of the file (after the package declaration):

```go
// Package mycasbin provides Casbin enforcer setup for multi-tenant environments.
//
// The Setup function (deprecated) creates a single global enforcer using sync.Once,
// which causes all tenants to share the same enforcer instance. This is a known
// issue and will be fixed in future stages.
//
// The SetupForTenant function is the new recommended approach, creating independent
// enforcer instances per tenant with proper error handling.
//
// Migration Guide:
//   - Old: enforcer := mycasbin.Setup(db, "")
//   - New: enforcer, err := mycasbin.SetupForTenant(db, tenantID)
//
// Stage 2 will add Redis Watcher support for per-tenant policy synchronization.
package mycasbin
```

**Step 2: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: PASS

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "docs(casbin): add package documentation and migration guide"
```

---

## Task 12: Create README for the casbin package

**Files:**
- Create: `sdk/pkg/casbin/README.md`

**Step 1: Create README with usage examples**

Create the file with this content:

```markdown
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
```

**Step 2: Verify README exists**

Run: `ls sdk/pkg/casbin/README.md`

Expected: File exists

**Step 3: Commit**

```bash
git add sdk/pkg/casbin/README.md
git commit -m "docs(casbin): add package README with migration guide"
```

---

## Task 13: Final verification and cleanup

**Files:**
- All modified files

**Step 1: Run full test suite for jxt-core**

Run: `go test ./... -short`

Expected: All tests PASS

**Step 2: Verify jxt-core builds**

Run: `go build ./...`

Expected: PASS

**Step 3: Check for any remaining references to global variables**

Run: `grep -r "var enforcer\|var once" sdk/pkg/casbin/`

Expected: No results (global variables removed)

**Step 4: Verify backward compatibility - create a simple test**

Create a temporary file to verify old code still works:

```bash
cat > /tmp/test_backward_compat.go << 'EOF'
package main

import (
	"fmt"
	"gorm.io/gorm"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/casbin"
)

func main() {
	// This simulates how existing code uses the old Setup API
	// It should still compile (though it won't run without a real DB)
	var db *gorm.DB
	enforcer := mycasbin.Setup(db, "")
	fmt.Println("Setup returned:", enforcer != nil)
}
EOF
```

Run: `go run /tmp/test_backward_compat.go 2>&1 | head -5`

Expected: Compiles successfully (may panic at runtime due to nil DB, but that's expected)

**Step 5: Clean up test file**

Run: `rm /tmp/test_backward_compat.go`

**Step 6: Final commit**

```bash
git add -A
git commit -m "chore(casbin): final verification - Stage 1 complete"
```

---

## Summary

This implementation plan covers Stage 1 of the Casbin multi-tenant isolation fix:

1. ✅ Removed global `enforcer` and `once` variables
2. ✅ Created `SetupForTenant(db, tenantID)` function with proper error handling
3. ✅ Maintained backward compatibility with `Setup()` function
4. ✅ Added comprehensive unit tests
5. ✅ Added documentation and migration guide

### What Changed

| Component | Before | After |
|-----------|--------|-------|
| Global state | `enforcer`, `once` variables | Removed |
| Error handling | `panic()` on errors | Returns `error` |
| Per-tenant | Single shared enforcer | Independent enforcers per call |
| API | `Setup(db, "")` | `SetupForTenant(db, tenantID)` + deprecated `Setup()` |

### Next Steps (Stage 2)

- Add Redis Watcher per-tenant channel isolation
- Implement per-tenant `updateCallback` closures
- Remove deprecated `updateCallback` function

### Migration for Consumers

Services using jxt-core will need to update from:
```go
enforcer := mycasbin.Setup(db, "")
```

To:
```go
enforcer, err := mycasbin.SetupForTenant(db, tenantID)
```

This change will be required in `security-management` service (covered in the full implementation plan).

---

**Testing Checklist:**

- [ ] All unit tests pass
- [ ] Integration tests pass (with test database)
- [ ] Code coverage > 50%
- [ ] Backward compatibility maintained (Setup still works)
- [ ] No compilation errors in jxt-core
- [ ] Documentation complete (README + package docs)
