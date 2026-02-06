# 修复 Casbin 多租户隔离问题 — 详细实施计划

**目标**: 修复 `jxt-core` 的 `mycasbin.Setup()` 中 `sync.Once` 单例模式导致的多租户 Casbin enforcer 共享问题，使每个租户拥有独立的 enforcer 实例和数据库连接。

**关联问题文档**: [casbin-multi-tenant-isolation-issue.md](../casbin-multi-tenant-isolation-issue.md)

**技术栈**: Go 1.24.0, Casbin v2, GORM, PostgreSQL, Redis, ETCD, jxt-core v1.1.32

---

## 修复概览

共需修复 **5 个根因**，按依赖关系分为 3 个阶段：

| 阶段 | 根因 | 修改范围 | 风险等级 |
|------|------|---------|---------|
| 阶段一 | 根因1 + 根因2 | `jxt-core` 库 | 🔴 高（共享库变更） |
| 阶段二 | 根因3 + 根因4 | `jxt-core` 库 | 🟡 中（Redis Watcher 重构） |
| 阶段三 | 根因5 | `security-management` 服务 | 🟢 低（本服务内变更） |

### 修改文件清单

| 文件 | 操作 | 阶段 |
|------|------|------|
| `jxt-core/sdk/pkg/casbin/mycasbin.go` | **重写** | 阶段一+二 |
| `jxt-core/sdk/runtime/application.go` | 无需修改 | — |
| `security-management/common/tenantdb/initializer.go` | **修改** | 阶段一 |
| `security-management/common/tenantdb/watcher.go` | **修改** | 阶段一 |
| `security-management/common/global/casbin.go` | **重写** | 阶段三 |
| `security-management/admin/interface/rest/api/sys_role.go` | **修改** | 阶段三 |
| `security-management/common/middleware/permission.go` | 无需修改（已直接使用 `GetTenantCasbin`） | — |

---

## 阶段一：修复根因1 + 根因2 — 移除单例模式，每租户独立 enforcer

### 任务 1.1：重构 `jxt-core/sdk/pkg/casbin/mycasbin.go`

**目标**: 移除 `sync.Once` 单例模式，改为每次调用创建新的 enforcer 实例。

**当前代码** (`mycasbin.go:31-91`):

```go
var (
    enforcer *casbin.SyncedEnforcer //策略执行器实例
    once     sync.Once
)

func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer {
    once.Do(func() {
        Apter, err := gormAdapter.NewAdapterByDBUseTableName(db, "sys", "casbin_rule")
        // ... 只执行一次
        enforcer, err = casbin.NewSyncedEnforcer(m, Apter)
        // ...
    })
    return enforcer  // 所有租户返回同一个实例
}
```

**修改后代码**:

```go
// 移除全局变量 enforcer 和 once
// var (
//     enforcer *casbin.SyncedEnforcer
//     once     sync.Once
// )

// SetupForTenant 为指定租户创建独立的 Casbin enforcer
// 每个租户拥有独立的 adapter 和 enforcer 实例
// 参数:
//   - db: 该租户的数据库连接
//   - tenantID: 租户ID（用于 Redis Watcher 频道隔离）
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

    return e, nil
}

// Setup 保留向后兼容（已废弃，请使用 SetupForTenant）
// Deprecated: 使用 SetupForTenant 替代
func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer {
    e, err := SetupForTenant(db, 0)
    if err != nil {
        panic(err)
    }
    return e
}
```

**关键变更说明**:

1. **移除全局变量**: 删除 `var enforcer` 和 `var once`
2. **新增 `SetupForTenant` 函数**: 每次调用创建新实例，接受 `tenantID` 参数
3. **返回 error**: 不再 panic，改为返回 error（更安全的错误处理）
4. **保留 `Setup` 向后兼容**: 其他使用 jxt-core 的服务不受影响
5. **Redis Watcher 暂不在此处初始化**: 移到阶段二单独处理

**验证方法**:

```go
// 验证不同 db 创建不同 enforcer
e1, _ := mycasbin.SetupForTenant(db1, 1)
e2, _ := mycasbin.SetupForTenant(db2, 2)
assert.NotSame(t, e1, e2)  // 不同实例
```

---

### 任务 1.2：修改 `security-management/common/tenantdb/initializer.go`

**目标**: 调用新的 `SetupForTenant` 替代 `Setup`。

**当前代码** (`initializer.go:195-203`):

```go
// 7. 初始化 Casbin
enforcer := mycasbin.Setup(db, "")
if enforcer == nil {
    return nil, fmt.Errorf("Casbin 初始化失败: 返回空 enforcer")
}

// 8. 保存到 Runtime
sdk.Runtime.SetTenantDB(tenantID, db)
sdk.Runtime.SetTenantCasbin(tenantID, enforcer)
```

**修改后代码**:

```go
// 7. 初始化 Casbin（每个租户独立的 enforcer）
enforcer, err := mycasbin.SetupForTenant(db, tenantID)
if err != nil {
    return nil, fmt.Errorf("租户 %d Casbin 初始化失败: %w", tenantID, err)
}

// 8. 保存到 Runtime
sdk.Runtime.SetTenantDB(tenantID, db)
sdk.Runtime.SetTenantCasbin(tenantID, enforcer)
```

**变更点**:

1. `mycasbin.Setup(db, "")` → `mycasbin.SetupForTenant(db, tenantID)`
2. 错误处理从 `nil` 检查改为 `error` 检查
3. 传入 `tenantID` 用于日志和后续 Redis Watcher 频道隔离

---

### 任务 1.3：修改 `security-management/common/tenantdb/watcher.go`

**目标**: 运行时新增租户时也使用 `SetupForTenant`。

**当前代码** (`watcher.go:162-171`):

```go
// 连接成功，初始化 Casbin
enforcer := mycasbin.Setup(db, "")
if enforcer == nil {
    slog.Error("新租户 Casbin 初始化失败",
        "tenant_id", tenantID,
        "error", "enforcer is nil")
    // Casbin 失败不影响数据库连接
} else {
    sdk.Runtime.SetTenantCasbin(tenantID, enforcer)
}
```

**修改后代码**:

```go
// 连接成功，初始化 Casbin（每个租户独立的 enforcer）
enforcer, casbinErr := mycasbin.SetupForTenant(db, tenantID)
if casbinErr != nil {
    slog.Error("新租户 Casbin 初始化失败",
        "tenant_id", tenantID,
        "error", casbinErr.Error())
    // Casbin 失败不影响数据库连接
} else {
    sdk.Runtime.SetTenantCasbin(tenantID, enforcer)
}
```

**变更点**:

1. `mycasbin.Setup(db, "")` → `mycasbin.SetupForTenant(db, tenantID)`
2. 错误处理从 `nil` 检查改为 `error` 检查

---

### 阶段一验证清单

- [ ] 每个租户的 enforcer 是不同的实例（指针地址不同）
- [ ] 每个租户的 enforcer 内部 adapter 指向各自的数据库
- [ ] 租户A的权限变更不影响租户B的 enforcer
- [ ] 启动时所有租户都能正确初始化各自的 enforcer
- [ ] 运行时新增租户能正确创建独立的 enforcer
- [ ] 向后兼容：`Setup()` 函数仍可正常工作

---

## 阶段二：修复根因3 + 根因4 — Redis Watcher 租户隔离

### 任务 2.1：重构 `updateCallback` 支持多租户

**目标**: `updateCallback` 不再使用全局 enforcer，改为按租户更新。

**当前代码** (`mycasbin.go:85-91`):

```go
func updateCallback(msg string) {
    logger.Infof("casbin updateCallback msg: %v", msg)
    err := enforcer.LoadPolicy()  // ← 全局 enforcer
    if err != nil {
        logger.Errorf("casbin LoadPolicy err: %v", err)
    }
}
```

**方案**: 在 `SetupForTenant` 中为每个租户创建独立的 callback 闭包。

**修改后代码** (在 `mycasbin.go` 的 `SetupForTenant` 函数中添加):

```go
func SetupForTenant(db *gorm.DB, tenantID int) (*casbin.SyncedEnforcer, error) {
    // ... (步骤 1-4 同阶段一)

    // 5. 设置 Redis Watcher（如果 Redis 已配置）
    if config.CacheConfig.Redis != nil {
        // 每个租户使用独立的 Redis 频道
        channel := fmt.Sprintf("/casbin/tenant/%d", tenantID)

        w, err := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
            Options: redis.Options{
                Network:  "tcp",
                Password: config.CacheConfig.Redis.Password,
            },
            Channel:    channel,  // ← 租户专属频道
            IgnoreSelf: false,
        })
        if err != nil {
            // Watcher 失败不应阻止 enforcer 创建
            logger.Errorf("租户 %d Redis Watcher 创建失败: %v", tenantID, err)
        } else {
            // 创建租户专属的 callback 闭包
            tenantEnforcer := e  // 捕获当前租户的 enforcer
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

**关键变更说明**:

1. **独立 Redis 频道**: `/casbin` → `/casbin/tenant/{tenantID}`，每个租户有自己的频道
2. **闭包捕获 enforcer**: `callback` 闭包捕获当前租户的 `tenantEnforcer`，不再依赖全局变量
3. **Watcher 失败不阻塞**: Redis 不可用时仍能正常创建 enforcer
4. **移除全局 `updateCallback` 函数**: 不再需要

---

### 任务 2.2：清理旧的全局 `updateCallback`

**操作**: 删除 `mycasbin.go` 中的全局 `updateCallback` 函数（第 85-91 行）。

该函数已被阶段二中每个租户的闭包 callback 替代。

---

### 阶段二验证清单

- [ ] 每个租户的 Redis Watcher 使用独立频道 `/casbin/tenant/{tenantID}`
- [ ] 租户A的权限变更通知只触发租户A的 enforcer 重新加载
- [ ] 租户B的权限变更通知只触发租户B的 enforcer 重新加载
- [ ] Redis 不可用时，enforcer 仍能正常创建和使用
- [ ] 多租户并发权限变更不会互相干扰

---

## 阶段三：修复根因5 — 优化 `global.LoadPolicy` 每次请求重加载

### 任务 3.1：重构 `common/global/casbin.go`

**目标**: 移除每次请求都调用 `LoadPolicy()` 的逻辑。在阶段二完成后，Redis Watcher 已能自动同步策略变更，不需要每次请求都重新加载。

**当前代码** (`common/global/casbin.go:13-30`):

```go
func LoadPolicy(c *gin.Context) (*casbin.SyncedEnforcer, error) {
    log := logger.GetRequestLogger(c)
    ctx := c.Request.Context()
    tenantID, ok := ctx.Value(TenantIDKey).(int)
    if !ok {
        err := errors.New("tenant id not exist")
        log.Error("casbin rbac_model or policy init error, ", zap.Error(err))
        return nil, err
    }
    if err := sdk.Runtime.GetTenantCasbin(tenantID).LoadPolicy(); err == nil {
        return sdk.Runtime.GetTenantCasbin(tenantID), err
    } else {
        log.Error("casbin rbac_model or policy init error, ", zap.Error(err))
        return nil, err
    }
}
```

**修改后代码**:

```go
// GetEnforcer 获取当前租户的 Casbin enforcer（不再每次请求都重新加载策略）
// 策略同步由 Redis Watcher 自动处理
func GetEnforcer(c *gin.Context) (*casbin.SyncedEnforcer, error) {
    ctx := c.Request.Context()
    tenantID, ok := ctx.Value(TenantIDKey).(int)
    if !ok {
        return nil, errors.New("tenant id not exist")
    }

    e := sdk.Runtime.GetTenantCasbin(tenantID)
    if e == nil {
        return nil, fmt.Errorf("租户 %d 的 Casbin enforcer 未初始化", tenantID)
    }

    return e, nil
}

// LoadPolicy 保留向后兼容，但内部改为仅获取 enforcer
// 策略重新加载由 Redis Watcher 自动处理，此处不再主动调用 LoadPolicy()
// Deprecated: 使用 GetEnforcer 替代
func LoadPolicy(c *gin.Context) (*casbin.SyncedEnforcer, error) {
    return GetEnforcer(c)
}

// ReloadPolicy 显式重新加载指定租户的策略（仅在权限变更后调用）
func ReloadPolicy(c *gin.Context) (*casbin.SyncedEnforcer, error) {
    log := logger.GetRequestLogger(c)
    ctx := c.Request.Context()
    tenantID, ok := ctx.Value(TenantIDKey).(int)
    if !ok {
        return nil, errors.New("tenant id not exist")
    }

    e := sdk.Runtime.GetTenantCasbin(tenantID)
    if e == nil {
        return nil, fmt.Errorf("租户 %d 的 Casbin enforcer 未初始化", tenantID)
    }

    if err := e.LoadPolicy(); err != nil {
        log.Error("casbin LoadPolicy error",
            zap.Int("tenant_id", tenantID),
            zap.Error(err))
        return nil, err
    }

    return e, nil
}
```

**关键变更说明**:

1. **`GetEnforcer`**: 新函数，仅获取 enforcer，不重新加载策略
2. **`LoadPolicy`**: 保留向后兼容，但内部不再调用 `enforcer.LoadPolicy()`
3. **`ReloadPolicy`**: 新函数，显式重新加载策略（仅在角色 CRUD 后调用）
4. **性能提升**: 每次请求不再执行数据库查询加载策略

---

### 任务 3.2：更新角色 CRUD 处理器

**目标**: 将 `sys_role.go` 中的 `global.LoadPolicy(c)` 改为 `global.ReloadPolicy(c)`，并在 Delete 操作中补充缺失的策略重载。

**文件**: `admin/interface/rest/api/sys_role.go`

**变更 1 — Insert 方法** (第 139-145 行):

当前代码:
```go
//在运行时更改策略文件后需要重新加载
_, err = global.LoadPolicy(c)
if err != nil {
    log.Error(err.Error())
    e.Error(c, 500, err, "创建失败,"+err.Error())
    return
}
```

修改后:
```go
//在运行时更改策略文件后需要重新加载
_, err = global.ReloadPolicy(c)
if err != nil {
    log.Error(err.Error())
    e.Error(c, 500, err, "创建失败,"+err.Error())
    return
}
```

**变更 2 — Update 方法** (第 189-194 行):

当前代码:
```go
_, err = global.LoadPolicy(c)
if err != nil {
    log.Error(err.Error())
    e.Error(c, 500, err, "更新失败,"+err.Error())
    return
}
```

修改后:
```go
_, err = global.ReloadPolicy(c)
if err != nil {
    log.Error(err.Error())
    e.Error(c, 500, err, "更新失败,"+err.Error())
    return
}
```

**变更 3 — Delete 方法** (第 226-231 行):

当前代码（**缺失策略重载**）:
```go
err = e.sysRoleService.Remove(ctx, &req, cb)
if err != nil {
    log.Error(err.Error())
    e.Error(c, 500, err, "删除失败,"+err.Error())
    return
}

e.OK(c, req.GetId(), fmt.Sprintf("删除角色角色 %v 状态成功！", req.GetId()))
```

修改后（**补充策略重载**）:
```go
err = e.sysRoleService.Remove(ctx, &req, cb)
if err != nil {
    log.Error(err.Error())
    e.Error(c, 500, err, "删除失败,"+err.Error())
    return
}

// 删除角色后重新加载策略（修复：原代码缺失此步骤）
_, err = global.ReloadPolicy(c)
if err != nil {
    log.Error(err.Error())
    e.Error(c, 500, err, "删除角色后刷新权限失败,"+err.Error())
    return
}

e.OK(c, req.GetId(), fmt.Sprintf("删除角色 %v 成功！", req.GetId()))
```

**变更点**:

1. `global.LoadPolicy(c)` → `global.ReloadPolicy(c)` (Insert、Update)
2. Delete 方法补充 `global.ReloadPolicy(c)` 调用（之前缺失）
3. 修正 Delete 成功消息中"角色角色"的重复文案

---

### 阶段三验证清单

- [ ] 普通 API 请求（权限检查）不再调用 `enforcer.LoadPolicy()`，性能提升
- [ ] 角色创建后，当前实例的 enforcer 立即更新
- [ ] 角色修改后，当前实例的 enforcer 立即更新
- [ ] 角色删除后，当前实例的 enforcer 立即更新（新增修复）
- [ ] `LoadPolicy` 旧函数仍可编译通过（向后兼容）
- [ ] Redis Watcher 仍能在多实例间同步策略变更

---

## 测试策略

### 单元测试

| 测试项 | 验证目标 | 测试方法 |
|--------|---------|---------|
| `SetupForTenant` 独立实例 | 不同 db 参数创建不同 enforcer | 比较指针地址 |
| `SetupForTenant` 错误处理 | db 无效时返回 error 而非 panic | 传入 nil db |
| `Setup` 向后兼容 | 旧函数仍可工作 | 调用 Setup 验证返回非 nil |
| `GetEnforcer` 不加载策略 | 仅获取 enforcer，不触发数据库查询 | Mock db 验证无查询 |
| `ReloadPolicy` 显式加载 | 调用后 enforcer 策略更新 | 修改 casbin_rule 后验证 |

### 集成测试

```bash
# 1. 多租户隔离测试
ginkgo -v -focus="多租户权限隔离" tests/admin_tests/api/

# 2. 角色 CRUD 测试（验证策略重载）
ginkgo -v -focus="角色" tests/admin_tests/api/
```

**测试场景**:

1. **租户隔离**: 创建两个租户，各自设置不同权限，验证租户A无法使用租户B的权限
2. **运行时新增租户**: 启动后动态新增租户，验证新租户 enforcer 独立
3. **并发初始化**: 同时初始化多个租户，验证无竞态条件
4. **策略重载**: 修改角色权限后，验证 enforcer 立即反映变更
5. **Redis Watcher 同步**: 在一个实例修改权限，验证其他实例同步更新

### 性能基准测试

```go
// 对比修复前后的权限检查性能
func BenchmarkPermissionCheck(b *testing.B) {
    // 修复前: 每次请求都执行 SELECT * FROM sys.casbin_rule
    // 修复后: 仅从内存读取策略，无数据库查询
    for i := 0; i < b.N; i++ {
        enforcer.Enforce("admin", "/api/v1/user", "GET")
    }
}
```

---

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| jxt-core 变更影响其他服务 | 🔴 高 | 保留 `Setup()` 向后兼容；其他服务无需修改 |
| 多个 enforcer 增加内存开销 | 🟡 中 | 每个 enforcer 约 1-5MB，10个租户约 50MB，可接受 |
| Redis 连接数增加（每租户一个 Watcher） | 🟡 中 | 监控 Redis 连接数；必要时使用连接池 |
| 并发初始化竞态条件 | 🟡 中 | `SetupForTenant` 无全局状态，天然并发安全 |
| 旧版 jxt-core 与新版 security-management 不兼容 | 🔴 高 | 必须先升级 jxt-core 再部署 security-management |

---

## 回滚方案

### 阶段一回滚

如果 `SetupForTenant` 导致问题：

1. **jxt-core**: 恢复 `mycasbin.go` 中 `sync.Once` 和全局 `enforcer` 变量
2. **security-management**: 将 `SetupForTenant(db, tenantID)` 改回 `Setup(db, "")`
3. **影响**: 回到所有租户共享一个 enforcer 的状态（已知问题，但至少可运行）

> ⚠️ 由于 `Setup()` 函数保留了向后兼容，回滚只需修改 `initializer.go` 和 `watcher.go` 中的两处调用即可。

### 阶段二回滚

如果 Redis Watcher 租户隔离导致问题：

1. **jxt-core**: 恢复 `SetupForTenant` 中的 Redis Watcher 部分，使用全局 `/casbin` 频道
2. **影响**: 所有租户共享一个 Redis 频道，权限更新可能互相覆盖

### 阶段三回滚

如果 `GetEnforcer`/`ReloadPolicy` 导致问题：

1. **security-management**: 恢复 `common/global/casbin.go` 为原始 `LoadPolicy` 实现
2. **security-management**: 恢复 `sys_role.go` 中的 `global.LoadPolicy(c)` 调用
3. **影响**: 回到每次请求都重新加载策略的状态（性能差，但功能正确）

---

## 实施时间线

```
阶段一 (2-3天)                    阶段二 (1-2天)              阶段三 (1天)
┌───────────────────────┐      ┌──────────────────┐      ┌──────────────┐
│ 1.1 重构 mycasbin.go   │      │ 2.1 Redis Watcher│      │ 3.1 重构     │
│ 1.2 修改 initializer   │──────│ 2.2 清理旧代码    │──────│ 3.2 更新角色  │
│ 1.3 修改 watcher       │      │                  │      │     处理器    │
│ + 单元测试 + 集成测试    │      │ + 集成测试        │      │ + 集成测试    │
└───────────────────────┘      └──────────────────┘      └──────────────┘
      ↓                              ↓                        ↓
  发布 jxt-core v1.1.33         发布 jxt-core v1.1.34    部署 security-management
```

### 依赖关系

- **阶段二依赖阶段一**: Redis Watcher 重构基于 `SetupForTenant` 函数
- **阶段三依赖阶段二**: `GetEnforcer` 不重新加载策略的前提是 Redis Watcher 能自动同步
- **jxt-core 发版先于 security-management 部署**: 必须先发布新版 jxt-core

### 里程碑

| 里程碑 | 预期日期 | 交付物 |
|--------|---------|--------|
| 阶段一完成 | T+3 | jxt-core v1.1.33 发布，每租户独立 enforcer |
| 阶段二完成 | T+5 | jxt-core v1.1.34 发布，Redis Watcher 租户隔离 |
| 阶段三完成 | T+6 | security-management 部署，性能优化 |
| 全面验证 | T+7 | 所有集成测试通过，生产环境验证 |

---

## 附录：最终 `mycasbin.go` 完整代码

完成阶段一和阶段二后，`jxt-core/sdk/pkg/casbin/mycasbin.go` 的最终形态：

```go
package mycasbin

import (
    "fmt"
    "log"

    "github.com/casbin/casbin/v2"
    "github.com/casbin/casbin/v2/model"
    gormAdapter "github.com/go-admin-team/gorm-adapter/v3"
    redisWatcher "github.com/go-admin-team/redis-watcher/v2"
    "github.com/go-redis/redis/v8"
    "github.com/ChenBigdata421/jxt-core/config"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
    "gorm.io/gorm"
)

var text = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act
`

// SetupForTenant 为指定租户创建独立的 Casbin enforcer
func SetupForTenant(db *gorm.DB, tenantID int) (*casbin.SyncedEnforcer, error) {
    // 1. 创建独立的 GORM Adapter
    adapter, err := gormAdapter.NewAdapterByDBUseTableName(db, "sys", "casbin_rule")
    if err != nil && err.Error() != "invalid DDL" {
        return nil, fmt.Errorf("创建 Casbin adapter 失败 (租户 %d): %w", tenantID, err)
    }

    // 2. 加载权限模型
    m, err := model.NewModelFromString(text)
    if err != nil {
        return nil, fmt.Errorf("加载 Casbin 模型失败: %w", err)
    }

    // 3. 创建 SyncedEnforcer
    e, err := casbin.NewSyncedEnforcer(m, adapter)
    if err != nil {
        return nil, fmt.Errorf("创建 Casbin enforcer 失败 (租户 %d): %w", tenantID, err)
    }

    // 4. 加载策略
    if err := e.LoadPolicy(); err != nil {
        return nil, fmt.Errorf("加载 Casbin 策略失败 (租户 %d): %w", tenantID, err)
    }

    // 5. 设置 Redis Watcher（如果已配置）
    if config.CacheConfig.Redis != nil {
        channel := fmt.Sprintf("/casbin/tenant/%d", tenantID)
        w, wErr := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr,
            redisWatcher.WatcherOptions{
                Options: redis.Options{
                    Network:  "tcp",
                    Password: config.CacheConfig.Redis.Password,
                },
                Channel:    channel,
                IgnoreSelf: false,
            })
        if wErr != nil {
            logger.Errorf("租户 %d Redis Watcher 创建失败: %v", tenantID, wErr)
        } else {
            tenantEnforcer := e
            callback := func(msg string) {
                logger.Infof("casbin updateCallback (租户 %d) msg: %v", tenantID, msg)
                if loadErr := tenantEnforcer.LoadPolicy(); loadErr != nil {
                    logger.Errorf("casbin LoadPolicy (租户 %d) err: %v", tenantID, loadErr)
                }
            }
            _ = w.SetUpdateCallback(callback)
            _ = e.SetWatcher(w)
        }
    }

    // 6. 设置日志
    log.SetLogger(&Logger{})
    e.EnableLog(true)

    return e, nil
}

// Setup 保留向后兼容（已废弃）
// Deprecated: 使用 SetupForTenant 替代
func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer {
    e, err := SetupForTenant(db, 0)
    if err != nil {
        panic(err)
    }
    return e
}
```