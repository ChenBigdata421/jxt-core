# Casbin 多租户隔离问题分析

**文档版本**: 1.1（已根据源码验证修正）
**创建日期**: 2026-02-06
**分析范围**: security-management 服务 + jxt-core v1.1.32
**严重程度**: 🔴 高危 - 导致所有非首个初始化租户权限检查错误

---

## 问题概述

当前系统中，虽然每个租户有独立的数据库和 `sys_casbin_rule` 表，但由于 `jxt-core` 的 `mycasbin.Setup()` 使用了 `sync.Once` 单例模式，导致所有租户共享同一个全局 `enforcer` 实例，该实例的 GORM Adapter 持有的是首个完成初始化的租户的数据库连接。

**核心问题**：所有租户的权限检查最终都使用的是首个完成初始化的租户的权限数据。

> **注意**：由于 `InitializeAll()` 使用 goroutine 并发初始化（最多10个并发），首个赢得 `sync.Once` 竞争的租户是**不确定的**，取决于 goroutine 调度顺序，不一定是 ID 最小的租户。以下为简化描述，以"租户A"代指首个完成初始化的租户。

---

## 问题场景

### 场景一：启动时租户隔离失败

```
时间线（并发执行，假设租户A先赢得 once.Do 竞争）：

T1: InitializeTenant(A, dbA)          ← goroutine 先到达
    ├── mycasbin.Setup(dbA)
    │   └── once.Do() 执行
    │       ├── adapter = gormAdapter.NewAdapterByDBUseTableName(dbA, "sys", "casbin_rule")
    │       ├── enforcer = casbin.NewSyncedEnforcer(m, adapter)  ← adapter 持有 dbA
    │       └── enforcer.LoadPolicy()  ← 从 dbA.sys_casbin_rule 加载
    │   └── return enforcer@0x100
    └── Runtime.SetTenantCasbin(A, enforcer@0x100)  ← 在 InitializeTenant 中调用

T2: InitializeTenant(B, dbB)          ← goroutine 后到达
    ├── mycasbin.Setup(dbB)
    │   └── once.Do() 不执行（已执行过）
    │   └── return enforcer@0x100  ← 返回 T1 的同一实例
    └── Runtime.SetTenantCasbin(B, enforcer@0x100)  ← 存储的还是同一个实例

结果：
Runtime.casbins = {
    A: enforcer@0x100,  // adapter 指向 dbA ✅
    B: enforcer@0x100,  // 同一个实例！还是指向 dbA ❌
}
```

### 场景二：运行时更新只更新首个租户

```
场景：修改租户B的权限

1. security-management 更新 tenant_B_db.sys_casbin_rule
2. 发布消息到 Redis 频道 "/casbin": "casbin updated"
3. security-management 收到消息
4. 调用 updateCallback()
5. enforcer.LoadPolicy()       ← 全局 enforcer 的 adapter 持有 dbA
6. adapter 执行 SQL: SELECT * FROM sys.casbin_rule
7. 但 adapter 内部的 *gorm.DB 是 dbA，不是 dbB！
8. 从 dbA.sys_casbin_rule 加载数据 ❌

结果：租户B的权限变更没有生效，反而是租户A的权限被重新加载了
```

### 场景三：权限检查使用错误的租户数据

```
用户X（租户B）访问 API:
    │
    ├── global.LoadPolicy(c)
    │   ├── GetTenantCasbin(B) → enforcer@0x100
    │   └── enforcer.LoadPolicy()  ← 每次请求都从 dbA 重新加载策略！
    ├── enforcer.Enforce("user", "/api/v1/user", "GET")
    └── 内部查询的是 dbA.sys_casbin_rule ❌

结果：用户X使用的是租户A的权限规则进行验证
```

---

## 根因分析

### 根因1：sync.Once 导致全局单例

**代码位置**: `jxt-core/sdk/pkg/casbin/mycasbin.go:31-83`

```go
var (
    enforcer *casbin.SyncedEnforcer //策略执行器实例
    once     sync.Once
)

func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer {
    once.Do(func() {  // ⚠️ 第一次后不再执行
        Apter, err := gormAdapter.NewAdapterByDBUseTableName(db, "sys", "casbin_rule")
        if err != nil && err.Error() != "invalid DDL" {
            panic(err)
        }
        m, err := model.NewModelFromString(text)
        if err != nil {
            panic(err)
        }
        enforcer, err = casbin.NewSyncedEnforcer(m, Apter)  // ← 使用首个到达的租户的 db
        // ... (Redis watcher 初始化, logger 配置)
        enforcer.LoadPolicy()  // ← 从首个租户的 db 加载
    })
    return enforcer  // ⚠️ 所有租户返回同一个实例
}
```

**影响**：
- 首次调用（赢得 `once.Do` 竞争的 goroutine）：创建 enforcer，使用该租户的 db
- 后续调用：直接返回首次创建的 enforcer，传入的 `db` 参数被忽略

---

### 根因2：GORM Adapter 持有数据库连接引用

**GORM Adapter 内部结构**：

```go
// gorm-adapter/v3 的实现
type Adapter struct {
    driverName string
    dataSourceName string
    ORMER        *gorm.DB  // ← 持有 db 引用！
    filtered     bool
    dbName       string
    // ...
}

func NewAdapterByDBUseTableName(db *gorm.DB, prefix string, table string) (*Adapter, error) {
    return &Adapter{
        ORMER: db,  // ← 保存了 db 的引用
        tablePrefix: prefix,
        tableName:   table,
    }, nil
}
```

**问题**：
- Adapter 在创建时保存了 `*gorm.DB` 的引用
- 后续所有操作（LoadPolicy, AddPolicy, RemovePolicy）都使用这个引用
- 无法切换到其他租户的数据库

---

### 根因3：updateCallback 无法区分租户

**代码位置**: `jxt-core/sdk/pkg/casbin/mycasbin.go:85-91`

```go
func updateCallback(msg string) {
    logger.Infof("casbin updateCallback msg: %v", msg)
    err := enforcer.LoadPolicy()  // ← 使用全局 enforcer 的 adapter 加载
    //                                  而 adapter 持有首个租户的 db
    if err != nil {
        logger.Errorf("casbin LoadPolicy err: %v", err)
    }
}
```

**问题**：
- `updateCallback` 使用全局 `enforcer`
- `enforcer.LoadPolicy()` 调用 `adapter.LoadPolicy()`
- `adapter` 持有的是首个完成初始化的租户的数据库连接
- 无法区分应该更新哪个租户的权限

---

### 根因4：Redis Watcher 无法区分租户

**代码位置**: `jxt-core/sdk/pkg/casbin/mycasbin.go:57-76`

```go
w, err := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
    Options: redis.Options{
        Network:  "tcp",
        Password: config.CacheConfig.Redis.Password,
    },
    Channel:    "/casbin",  // ← 所有租户共享同一个频道
    IgnoreSelf: false,
})
```

**问题**：
- 所有租户共享同一个 Redis 频道: `/casbin`
- 收到消息后，无法知道是哪个租户的权限变更
- 只能更新全局 enforcer（首个租户的权限）

---

### 根因5：global.LoadPolicy 每次请求重加载放大问题

**代码位置**: `security-management/common/global/casbin.go:13-30`

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

**问题**：
- 该函数在**每次权限检查请求**时都会调用 `enforcer.LoadPolicy()`
- 由于所有租户共享同一个 enforcer，`LoadPolicy()` 总是从首个租户的 db 重新加载
- 这意味着问题不仅存在于启动时，而是在**每个请求**中都主动从错误的数据库重新加载策略
- 即使通过其他方式在内存中临时修正了某个租户的策略，下一次请求也会被覆盖回来

---

## 影响范围

> 以下表格中"租户A"代指首个赢得 `sync.Once` 竞争、完成初始化的租户（不确定是哪个租户ID）。

### 启动时影响

| 租户 | enforcer 实例 | adapter 指向 | 权限数据来源 | 结果 |
|------|--------------|-------------|------------|------|
| 租户A（首个初始化） | enforcer@0x100 | dbA | dbA.sys_casbin_rule | ✅ 正确 |
| 租户B | enforcer@0x100 | dbA | dbA.sys_casbin_rule | ❌ 错误 |
| 租户C | enforcer@0x100 | dbA | dbA.sys_casbin_rule | ❌ 错误 |

### 运行时影响

| 操作 | 预期行为 | 实际行为 |
|------|---------|---------|
| 修改租户A权限 | 更新租户A的 enforcer | ✅ 正常更新 |
| 修改租户B权限 | 更新租户B的 enforcer | ❌ 更新的还是从 dbA 加载 |
| 修改租户C权限 | 更新租户C的 enforcer | ❌ 更新的还是从 dbA 加载 |

### 权限检查影响

| 用户 | 所属租户 | 使用的权限数据 | 结果 |
|------|---------|--------------|------|
| 用户X | 租户A | 租户A的权限 | ✅ 正确 |
| 用户Y | 租户B | 租户A的权限 | ❌ 错误！可能越权或权限不足 |
| 用户Z | 租户C | 租户A的权限 | ❌ 错误！可能越权或权限不足 |

---

## 安全风险

### 风险1：权限越权

**场景**：租户A拥有 `/*` 全部权限，租户B只有 `/api/v1/user` 的只读权限

```
租户B用户尝试访问 /api/v1/admin（应该被拒绝）
    │
    ├── enforcer.Enforce("user", "/api/v1/admin", "DELETE")
    └── 实际检查的是租户A的权限（允许访问 /api/v1/admin）

结果：租户B用户可以访问 /api/v1/admin ❌ 越权！
```

### 风险2：权限不足

**场景**：租户A没有访问某些资源的权限，租户B有

```
租户B用户尝试访问 /api/v2/special-resource（应该允许）
    │
    ├── enforcer.Enforce("user", "/api/v2/special-resource", "GET")
    └── 实际检查的是租户A的权限（拒绝访问）

结果：租户B用户被拒绝访问 ❌ 权限不足！
```

### 风险3：数据泄露

**场景**：不同租户可能有不同的敏感数据访问策略

```
租户A（政府）- 严格限制
租户B（企业）- 宽松限制

由于权限混淆，可能导致：
- 租户A用户访问了不该访问的资源
- 审计日志无法准确反映实际权限使用情况
```

---

## 相关文件

| 文件 | 问题 |
|------|------|
| `jxt-core/sdk/pkg/casbin/mycasbin.go:31-83` | sync.Once 单例模式，Setup() 只执行一次 |
| `jxt-core/sdk/pkg/casbin/mycasbin.go:85-91` | updateCallback 无租户区分 |
| `jxt-core/sdk/pkg/casbin/mycasbin.go:57-76` | Redis Watcher 频道无租户区分 |
| `security-management/common/tenantdb/initializer.go:196` | 启动时调用 mycasbin.Setup() |
| `security-management/common/tenantdb/watcher.go:163-170` | 运行时新增租户调用 mycasbin.Setup() |
| `security-management/common/global/casbin.go:13-30` | 每次请求调用 LoadPolicy() 放大问题 |

---

## 解决方向

### 方向1：为每个租户创建独立的 enforcer（推荐）

**核心思路**：
- 移除 `sync.Once`，为每个租户创建独立的 `enforcer` 实例
- 每个 `enforcer` 使用自己租户的数据库连接

**需要修改**：
- `jxt-core/sdk/pkg/casbin/mycasbin.go` - 移除单例模式，改为每次创建新实例
- `security-management/common/tenantdb/initializer.go` - 适配新的 API
- `security-management/common/tenantdb/watcher.go` - 适配新的 API
- `security-management/common/global/casbin.go` - 评估是否需要每次请求都 LoadPolicy()

### 方向2：使用独立的 Casbin 服务

**核心思路**：
- 创建独立的 `casbin-service`，维护所有租户的 `enforcer`
- 其他微服务通过 gRPC 调用检查权限

**优点**：
- 权限逻辑集中管理
- 其他微服务不需要 Casbin 依赖

**缺点**：
- 每次权限检查需要网络调用
- `casbin-service` 成为单点故障

---

## 下一步

1. **评估影响范围** - 统计当前生产环境的租户数量
2. **选择解决方案** - 根据业务需求选择方向1或方向2
3. **制定实施计划** - 分阶段修复，避免影响现有业务
4. **测试验证** - 确保修复后所有租户的权限隔离正常

---

**相关文档**:
- [Casbin权限缓存同步问题分析.md](./Casbin权限缓存同步问题分析.md)
- [权限检查机制分析文档.md](./权限检查机制分析文档.md)
