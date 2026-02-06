# Casbin Multi-Tenant Isolation Fix - Stage 2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix Redis Watcher multi-tenant isolation by creating per-tenant callback closures and using dedicated Redis channels for each tenant.

**Architecture:** In `SetupForTenant`, create tenant-specific callback closures that capture each tenant's enforcer, and use per-tenant Redis channels (`/casbin/tenant/{tenantID}`) instead of the shared `/casbin` channel.

**Tech Stack:** Go 1.24.0, Casbin v2, GORM, Redis Watcher, jxt-core v1.1.32

**Related Documents:**
- [casbin-multi-tenant-isolation-issue.md](../../权限检查的租户隔离问题/casbin-multi-tenant-isolation-issue.md)
- [2026-02-06-fix-casbin-multi-tenant-isolation.md](../../权限检查的租户隔离问题/2026-02-06-fix-casbin-multi-tenant-isolation.md)

---

## Overview

**Problem:** The current Redis Watcher uses a shared channel (`/casbin`) for all tenants, and `updateCallback` relies on the global `enforcer` variable (which was removed in Stage 1).

**Solution (Stage 2):**
- Create tenant-specific callback closures in `SetupForTenant` that capture each tenant's enforcer
- Use per-tenant Redis channels: `/casbin/tenant/{tenantID}`
- Remove the global `updateCallback` function (no longer needed)

**Files:**
- Modify: [sdk/pkg/casbin/mycasbin.go](../../sdk/pkg/casbin/mycasbin.go)

---

## Task 1: Add Redis Watcher setup to SetupForTenant

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Locate the SetupForTenant function and find step 5 (after LoadPolicy)**

The current code has:
```go
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

**Step 2: Replace steps 5-6 with Redis Watcher setup code**

Replace from `// 5. 设置日志` through `return e, nil` with:

```go
// 5. 设置 Redis Watcher（如果 Redis 已配置）
if config.CacheConfig.Redis != nil {
    // 每个租户使用独立的 Redis 频道
    channel := fmt.Sprintf("/casbin/tenant/%d", tenantID)

    w, err := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
        Options: redis.Options{
            Network:  "tcp",
            Password: config.CacheConfig.Redis.Password,
        },
        Channel:    channel, // 租户专属频道
        IgnoreSelf: false,
    })
    if err != nil {
        // Watcher 失败不应阻止 enforcer 创建
        logger.Errorf("租户 %d Redis Watcher 创建失败: %v", tenantID, err)
    } else {
        // 创建租户专属的 callback 闭包
        tenantEnforcer := e // 捕获当前租户的 enforcer
        callback := func(msg string) {
            logger.Infof("casbin updateCallback (租户 %d) msg: %v", tenantID, msg)
            if err := tenantEnforcer.LoadPolicy(); err != nil {
                logger.Errorf("casbin LoadPolicy (租户 %d) err: %v", tenantID, err)
            }
        }

        if err := w.SetUpdateCallback(callback); err != nil {
            logger.Errorf("租户 %d 设置 Watcher callback 失败: %v", tenantID, err)
        }
        if err := e.SetWatcher(w); err != nil {
            logger.Errorf("租户 %d 设置 Watcher 失败: %v", tenantID, err)
        }
    }
}

// 6. 设置日志
log.SetLogger(&Logger{})
e.EnableLog(true)

return e, nil
}
```

**Step 3: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: PASS

**Step 4: Run existing tests**

Run: `go test ./sdk/pkg/casbin/... -v`

Expected: All tests PASS (tests will skip if no Redis available)

**Step 5: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "feat(casbin): add Redis Watcher per-tenant isolation to SetupForTenant"
```

---

## Task 2: Update Setup function to use tenant-specific callback

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Locate the Setup function (backward compatibility function)**

Find the `Setup` function around line 94-130. Note that it currently still references the global `updateCallback` function which we will remove.

**Step 2: Update Setup function to create its own callback closure**

The current Setup function has:
```go
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
```

Update the Setup function's Redis Watcher section to use tenantID=0 and create a local callback:

```go
// 兼容旧版：如果配置了 Redis，设置 Watcher
// 注意: 使用租户0的频道以保持向后兼容
if config.CacheConfig.Redis != nil {
    w, wErr := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
        Options: redis.Options{
            Network:  "tcp",
            Password: config.CacheConfig.Redis.Password,
        },
        Channel:    "/casbin/tenant/0", // 使用租户0的频道
        IgnoreSelf: false,
    })
    if wErr != nil {
        panic(wErr)
    }

    // 创建本地 callback 闭包
    setupEnforcer := e
    setupCallback := func(msg string) {
        logger.Infof("casbin updateCallback (Setup/租户0) msg: %v", msg)
        if err := setupEnforcer.LoadPolicy(); err != nil {
            logger.Errorf("casbin LoadPolicy (Setup/租户0) err: %v", err)
        }
    }

    wErr = w.SetUpdateCallback(setupCallback)
    if wErr != nil {
        panic(wErr)
    }
    wErr = e.SetWatcher(w)
    if wErr != nil {
        panic(wErr)
    }
}

return e
```

**Step 3: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: PASS

**Step 4: Run tests**

Run: `go test ./sdk/pkg/casbin/... -v`

Expected: All tests PASS

**Step 5: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "refactor(casbin): update Setup function to use tenant-specific callback"
```

---

## Task 3: Remove global updateCallback function

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Locate the global updateCallback function**

Find the `updateCallback` function around line 132-139:
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

**Step 2: Delete the entire updateCallback function**

Remove lines 132-139 (the entire function).

**Step 3: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: PASS (Setup function now uses its own callback closure)

**Step 4: Run tests**

Run: `go test ./sdk/pkg/casbin/... -v`

Expected: All tests PASS

**Step 5: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "refactor(casbin): remove global updateCallback function (replaced by per-tenant closures)"
```

---

## Task 4: Update package documentation

**Files:**
- Modify: `sdk/pkg/casbin/mycasbin.go`

**Step 1: Update package-level documentation**

Find the package documentation at the top of the file (lines 3-16):

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
```

Update the documentation to reflect Stage 2 completion:

```go
// Package mycasbin provides Casbin enforcer setup for multi-tenant environments.
//
// The Setup function (deprecated) creates a single global enforcer using sync.Once,
// which causes all tenants to share the same enforcer instance. This is a known
// issue that has been fixed in SetupForTenant.
//
// The SetupForTenant function is the recommended approach, creating independent
// enforcer instances per tenant with proper error handling and per-tenant Redis
// Watcher channels for policy synchronization.
//
// Migration Guide:
//   - Old: enforcer := mycasbin.Setup(db, "")
//   - New: enforcer, err := mycasbin.SetupForTenant(db, tenantID)
//
// Redis Watcher:
//   - Each tenant uses a dedicated Redis channel: /casbin/tenant/{tenantID}
//   - Policy changes are automatically synchronized across instances via Redis pub/sub
//   - Watcher failures do not prevent enforcer creation (graceful degradation)
```

**Step 2: Update SetupForTenant function documentation**

Find the SetupForTenant function documentation (lines 47-54) and update it:

```go
// SetupForTenant 为指定租户创建独立的 Casbin enforcer
// 每个租户拥有独立的 adapter、enforcer 实例和 Redis Watcher 频道
// 参数:
//   - db: 该租户的数据库连接
//   - tenantID: 租户ID（用于日志标识和 Redis Watcher 频道隔离）
// 返回:
//   - *casbin.SyncedEnforcer: 该租户专属的 enforcer 实例
//   - error: 错误信息
//
// Redis Watcher:
//   - 每个租户使用独立的 Redis 频道: /casbin/tenant/{tenantID}
//   - 当租户的权限策略变更时，通过 Redis pub/sub 自动通知所有实例重新加载策略
//   - Redis 不可用时，enforcer 仍能正常创建和使用（优雅降级）
```

**Step 3: Verify the file compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: PASS

**Step 4: Commit**

```bash
git add sdk/pkg/casbin/mycasbin.go
git commit -m "docs(casbin): update package documentation for Stage 2 Redis Watcher support"
```

---

## Task 5: Update README

**Files:**
- Modify: `sdk/pkg/casbin/README.md`

**Step 1: Update the "Known Issues" section**

Find the "Known Issues" section in README.md and remove Stage 2 items (now fixed):

Current section:
```markdown
## Known Issues

1. **Redis Watcher:** The current implementation uses a shared Redis channel (`/casbin`) for all tenants. This will be fixed in Stage 2 with per-tenant channels (`/casbin/tenant/{tenantID}`).

2. **Global updateCallback:** The `updateCallback` function no longer reloads policies since the global enforcer was removed. A warning is logged instead. Stage 2 will implement per-tenant callback closures.
```

Replace with:
```markdown
## Known Issues

None (as of Stage 2). All multi-tenant isolation issues have been fixed.
```

**Step 2: Update the "Roadmap" section**

Find the "Roadmap" section and update it:

Current section:
```markdown
## Roadmap

- **Stage 1** (Current): Remove singleton, add `SetupForTenant` ✅
- **Stage 2**: Redis Watcher per-tenant isolation
- **Stage 3**: Update security-management to use new APIs
```

Update to:
```markdown
## Roadmap

- **Stage 1**: Remove singleton, add `SetupForTenant` ✅
- **Stage 2** (Current): Redis Watcher per-tenant isolation ✅
- **Stage 3**: Update security-management to use new APIs (pending)
```

**Step 3: Add Redis Watcher section**

After the "API Reference" section, add a new section:

```markdown
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
```

**Step 4: Commit**

```bash
git add sdk/pkg/casbin/README.md
git commit -m "docs(casbin): update README for Stage 2 completion"
```

---

## Task 6: Run final verification tests

**Files:**
- Test: `sdk/pkg/casbin/mycasbin_test.go`

**Step 1: Run all tests**

Run: `go test ./sdk/pkg/casbin/... -v`

Expected: All tests PASS

**Step 2: Verify package compiles**

Run: `go build ./sdk/pkg/casbin/...`

Expected: PASS

**Step 3: Check for any remaining references to global updateCallback**

Run: `grep -n "updateCallback" sdk/pkg/casbin/mycasbin.go`

Expected: No results (function has been removed)

**Step 4: Verify no global variables remain**

Run: `grep -n "var.*enforcer\|var.*once" sdk/pkg/casbin/mycasbin.go`

Expected: Only the `var text` model definition should appear

**Step 5: Commit**

```bash
git add -A
git commit -m "chore(casbin): final verification - Stage 2 complete"
```

---

## Summary

This implementation plan covers Stage 2 of the Casbin multi-tenant isolation fix:

1. ✅ Added Redis Watcher setup to `SetupForTenant` with per-tenant channels
2. ✅ Updated `Setup` function to use tenant-specific callback closure
3. ✅ Removed global `updateCallback` function
4. ✅ Updated package documentation
5. ✅ Updated README with Redis Watcher information

### What Changed

| Component | Before (Stage 1) | After (Stage 2) |
|-----------|------------------|-----------------|
| Redis Channel | Shared `/casbin` for all | Per-tenant `/casbin/tenant/{tenantID}` |
| Callback | Global `updateCallback` (no-op) | Per-tenant callback closures |
| Policy Sync | Not working (removed in Stage 1) | Working via Redis pub/sub |

### Key Improvements

1. **Tenant Isolation**: Each tenant has its own Redis channel and callback
2. **Graceful Degradation**: Redis failures don't prevent enforcer creation
3. **Automatic Sync**: Policy changes are synchronized across all instances
4. **No Global State**: All callbacks are local closures capturing tenant-specific enforcers

### Testing Checklist

- [ ] All unit tests pass
- [ ] Package compiles without errors
- [ ] No references to global `updateCallback` remain
- [ ] No global `enforcer` or `once` variables
- [ ] Documentation updated for Stage 2

### Next Steps (Stage 3)

Stage 3 will be implemented in the `security-management` service:
- Update `common/global/casbin.go` to remove `LoadPolicy()` on every request
- Update role CRUD handlers to use `ReloadPolicy()` instead
- This stage is not part of jxt-core and will be documented separately
