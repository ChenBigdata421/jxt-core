# 域名查找租户功能实现计划

**日期**: 2026-02-18
**状态**: ✅ 已完成 (2026-02-18)

**实现提交**:
- `2c7ba92` feat(tenant/provider): add GetTenantIDByDomain for O(1) domain lookup
- `1b14552` feat(tenant/middleware): add domain lookup support for host-based resolution
- `1974998` test(tenant): add unit tests for domain lookup functionality
- `9dc5587` test(tenant/provider): add performance benchmarks for domain lookup

**性能验证**: BenchmarkGetTenantIDByDomain = 22ns/op (目标 < 50ns/op ✅)

## Context

**问题**: 当前 jxt-core 的 host resolver 只能提取子域名作为租户ID，无法根据完整域名（如 `tenant-alpha.company.com`）查找对应的租户。

**需求**: 实现根据域名查找租户ID的功能，支持：
- Primary 主域名
- Aliases 别名列表
- Internal 内网域名

**影响范围**:
- `jxt-core/sdk/pkg/tenant/provider/provider.go`
- `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go`
- 所有使用 jxt-core 的服务（file-storage-service, security-management 等）

---

## Performance Considerations

### 为什么不用 O(N) 遍历？

遍历所有租户域名配置的方式在高并发场景下会成为瓶颈：

| 租户数量 | 域名检查次数/请求 | 耗时/请求 | 1000 req/s CPU 占用 |
|----------|-------------------|-----------|---------------------|
| 100 | ~400 | ~20μs | ~2% |
| 1000 | ~4000 | ~200μs | ~20% |
| 5000 | ~20000 | ~1ms | ~100% |

### 高性能方案：atomic.Value 反向索引

采用 `atomic.Value` 存储反向索引，实现无锁 O(1) 查找：

| 指标 | 值 |
|------|-----|
| 查找复杂度 | O(1) |
| 查找耗时 | ~10ns（任何规模） |
| 锁竞争 | 无 |
| 内存开销 | ~132KB（1000租户） |

**设计原则**：
- 写入时（低频）：`LoadAll()` 或 ETCD 事件触发时原子替换整个数据（含索引）
- 读取时（高频）：无锁 O(1) 查找

---

## Implementation Plan

### Step 1: Provider 添加域名反向索引

**文件**: `jxt-core/sdk/pkg/tenant/provider/provider.go`

#### 1.1 修改 `tenantData` 结构体内嵌索引

> **设计决策**：将 `domainIndex` 内嵌到 `tenantData` 中，通过一次 `atomic.Value.Store()` 原子替换全部数据，彻底消除一致性窗口。

```go
// tenantData 租户数据
type tenantData struct {
    Metas       map[int]*TenantMeta                      `json:"metas"`
    Databases   map[int]map[string]*ServiceDatabaseConfig `json:"databases"` // tenantID -> serviceCode -> config
    Ftps        map[int][]*FtpConfigDetail                `json:"ftps"`      // tenantID -> configs[]
    Storages    map[int]*StorageConfig                    `json:"storages"`
    Domains     map[int]*DomainConfig                     `json:"domains"`
    domainIndex map[string]int                            // 内嵌：域名反向索引 domain -> tenantID（不导出，不序列化）
}
```

**优势**：
- Provider 无需新增 `domainIndex` 字段
- 一次 `atomic.Store` 替换全部数据，无一致性窗口
- 零额外复杂度

#### 1.2 Provider 结构体无需修改

Provider 保持现有结构，无需新增字段。

#### 1.3 修改 `NewProvider()` 初始化

```go
func NewProvider(client *clientv3.Client, opts ...Option) *Provider {
    p := &Provider{
        client:      client,
        namespace:   "jxt/",
        configTypes: []ConfigType{ConfigTypeDatabase},
        data:        atomic.Value{},
    }
    initialData := &tenantData{
        Metas:     make(map[int]*TenantMeta),
        Databases: make(map[int]map[string]*ServiceDatabaseConfig),
        Ftps:      make(map[int][]*FtpConfigDetail),
        Storages:  make(map[int]*StorageConfig),
        Domains:   make(map[int]*DomainConfig),
    }
    initialData.domainIndex = buildDomainIndex(initialData) // 构建初始索引
    p.data.Store(initialData)
    for _, opt := range opts {
        opt(p)
    }
    return p
}
```

#### 1.4 添加 `buildDomainIndex()` 函数

```go
// buildDomainIndex 构建域名反向索引
// 返回 map[domain]tenantID，所有域名已规范化为小写
// 检测并记录域名冲突（不同租户声明相同域名）
func buildDomainIndex(data *tenantData) map[string]int {
    // 预估容量：每个租户约 4 个域名（Primary + 2 Aliases + Internal）
    estimatedSize := len(data.Domains) * 4
    if estimatedSize == 0 {
        estimatedSize = 8 // 最小容量
    }
    index := make(map[string]int, estimatedSize)

    for tenantID, cfg := range data.Domains {
        // Primary 域名
        if cfg.Primary != "" {
            domain := normalizeDomain(cfg.Primary)
            if domain != "" {
                if existingID, exists := index[domain]; exists && existingID != tenantID {
                    logger.Warnf("domain conflict: %s claimed by tenant %d and %d, using %d",
                        domain, existingID, tenantID, tenantID)
                }
                index[domain] = tenantID
            }
        }

        // Aliases 别名列表
        for _, alias := range cfg.Aliases {
            if alias != "" {
                domain := normalizeDomain(alias)
                if domain != "" {
                    if existingID, exists := index[domain]; exists && existingID != tenantID {
                        logger.Warnf("domain conflict: %s claimed by tenant %d and %d, using %d",
                            domain, existingID, tenantID, tenantID)
                    }
                    index[domain] = tenantID
                }
            }
        }

        // Internal 内网域名
        if cfg.Internal != "" {
            domain := normalizeDomain(cfg.Internal)
            if domain != "" {
                if existingID, exists := index[domain]; exists && existingID != tenantID {
                    logger.Warnf("domain conflict: %s claimed by tenant %d and %d, using %d",
                        domain, existingID, tenantID, tenantID)
                }
                index[domain] = tenantID
            }
        }
    }

    return index
}

// normalizeDomain 规范化域名：小写 + 去除空白
func normalizeDomain(domain string) string {
    domain = strings.ToLower(strings.TrimSpace(domain))
    if domain == "" {
        return ""
    }
    return domain
}
```

#### 1.5 添加 `GetTenantIDByDomain()` 公共方法

```go
// GetTenantIDByDomain 根据域名查找租户ID
// 查找复杂度 O(1)，无锁，线程安全
// 支持 Primary、Aliases、Internal 三种域名类型
// 域名匹配不区分大小写（RFC 4343）
// 注意：仅支持精确匹配，不支持通配符域名（如 *.tenant.com）
func (p *Provider) GetTenantIDByDomain(domain string) (int, bool) {
    if domain == "" {
        return 0, false
    }

    normalizedDomain := normalizeDomain(domain)
    if normalizedDomain == "" {
        return 0, false
    }

    // O(1) 查找，无锁
    data := p.data.Load().(*tenantData)
    tenantID, ok := data.domainIndex[normalizedDomain]
    return tenantID, ok
}
```

#### 1.6 修改 `LoadAll()` 构建索引

```go
func (p *Provider) LoadAll(ctx context.Context) error {
    // ... existing code ...

    newData.domainIndex = buildDomainIndex(newData) // 构建索引
    p.data.Store(newData)                           // 一次原子替换

    logger.Infof("tenant provider: loaded %d tenants, %d database configs, %d ftp configs, %d domain mappings from ETCD",
        len(newData.Metas), countServiceDatabases(newData.Databases), countFtpConfigs(newData.Ftps),
        len(newData.domainIndex))

    // ... cache sync ...
    return nil
}
```

> **关于 ConfigType**：域名配置始终加载，不受 `configTypes` 选项过滤。这是因为域名查找是基础设施功能，应始终可用。

#### 1.7 修改 `handleWatchEvent()` 更新索引

```go
func (p *Provider) handleWatchEvent(ev *clientv3.Event) {
    current := p.data.Load().(*tenantData)
    newData := current.copyData()

    // ... existing event handling ...

    newData.domainIndex = buildDomainIndex(newData) // 重建索引
    p.data.Store(newData)                           // 一次原子替换

    // ... cache sync ...
}
```

#### 1.8 修改 `copyData()` 复制索引

```go
func (d *tenantData) copyData() *tenantData {
    newData := &tenantData{
        Metas:     make(map[int]*TenantMeta),
        Databases: make(map[int]map[string]*ServiceDatabaseConfig),
        Ftps:      make(map[int][]*FtpConfigDetail),
        Storages:  make(map[int]*StorageConfig),
        Domains:   make(map[int]*DomainConfig),
    }

    // ... existing copy logic ...

    // 复制 Domains
    for k, v := range d.Domains {
        newData.Domains[k] = v
    }

    // domainIndex 在 copyData 时不复制，由调用方重建
    // 因为索引可以从 Domains 派生

    return newData
}
```

---

### Step 2: Middleware 添加域名查找支持

**文件**: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go`

#### 2.1 定义接口（在 middleware 包内部）

> **设计决策**：遵循 Go 惯例"消费者定义接口"，接口放在 middleware 包内部。Provider 隐式实现该接口（鸭子类型）。

```go
package middleware

// DomainLookuper 定义域名查找接口
// 由 provider.Provider 隐式实现（鸭子类型）
type DomainLookuper interface {
    GetTenantIDByDomain(domain string) (int, bool)
}
```

#### 2.2 添加配置字段

```go
type Config struct {
    resolverType    ResolverType
    headerName      string
    queryParam      string
    pathIndex       int
    onMissingTenant MissingTenantMode
    domainLookup    DomainLookuper // 新增：域名查找器
}
```

#### 2.3 添加配置选项

```go
// WithDomainLookup 启用域名查找功能
// 当 resolverType 为 "host" 时，会先尝试通过完整域名查找租户ID
// 查找失败时回退到子域名提取
func WithDomainLookup(lookup DomainLookuper) Option {
    return func(c *Config) {
        c.domainLookup = lookup
    }
}
```

#### 2.4 修改 `extractTenantID()` 传递 domainLookup

```go
func extractTenantID(c *gin.Context, cfg *Config) (string, bool) {
    switch cfg.resolverType {
    case ResolverTypeHost:
        return extractFromHost(c, cfg.domainLookup) // 只传需要的字段
    case ResolverTypeHeader:
        return extractFromHeader(c, cfg.headerName)
    case ResolverTypeQuery:
        return extractFromQuery(c, cfg.queryParam)
    case ResolverTypePath:
        return extractFromPath(c, cfg.pathIndex)
    default:
        return "", false
    }
}
```

#### 2.5 修改 `extractFromHost()`

```go
// extractFromHost extracts tenant ID from Host header
// 优先级：
// 1. 如果配置了 domainLookup，尝试域名查找（精确匹配，不支持通配符）
// 2. 回退到子域名提取
func extractFromHost(c *gin.Context, lookup DomainLookuper) (string, bool) {
    host := c.Request.Host
    if host == "" {
        return "", false
    }

    // Remove port if present
    if idx := strings.Index(host, ":"); idx != -1 {
        host = host[:idx]
    }

    // 1. 如果配置了域名查找器，先尝试完整域名匹配
    if lookup != nil {
        if tenantID, ok := lookup.GetTenantIDByDomain(host); ok {
            return strconv.Itoa(tenantID), true
        }
    }

    // 2. 回退到子域名提取（现有逻辑）
    parts := strings.Split(host, ".")
    if len(parts) < 2 {
        return "", false
    }

    subdomain := parts[0]
    if subdomain == "" || subdomain == "www" {
        return "", false
    }

    return subdomain, true
}
```

---

### Step 3: 服务集成

**security-management**: `common/middleware/init.go`

```go
import (
    tenantmiddleware "github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/middleware"
    "security-management/common/tenantdb"
)

func SetupMiddleware(r *gin.Engine) error {
    // 获取 tenant cache
    cache, err := tenantdb.GetGlobalCache()
    if err != nil {
        return fmt.Errorf("failed to get tenant cache: %w", err)
    }

    // 配置 middleware
    // cache 通过 GetTenantIDByDomain 方法隐式实现 DomainLookuper 接口
    r.Use(tenantmiddleware.ExtractTenantID(
        tenantmiddleware.WithResolverType("host"),
        tenantmiddleware.WithDomainLookup(cache),
    ))

    return nil
}
```

**security-management/common/tenantdb/cache.go**: 实现 DomainLookuper

```go
// GetTenantIDByDomain 实现 middleware.DomainLookuper 接口
// 直接委托给 provider，无额外逻辑
func (c *Cache) GetTenantIDByDomain(domain string) (int, bool) {
    return c.provider.GetTenantIDByDomain(domain)
}
```

> **设计说明**：Cache 层转发方法的存在是因为 Cache 是 tenantdb 包对外暴露的唯一入口，业务代码不直接访问 Provider。转发方法提供了更好的封装边界。

---

## Critical Files

| 文件 | 修改内容 |
|------|---------|
| `jxt-core/sdk/pkg/tenant/provider/provider.go` | 修改 tenantData 内嵌 domainIndex，添加 buildDomainIndex、GetTenantIDByDomain、normalizeDomain |
| `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go` | 添加 DomainLookuper 接口、WithDomainLookup 选项，修改 extractFromHost |
| `security-management/common/tenantdb/cache.go` | 添加 GetTenantIDByDomain 转发方法 |
| `security-management/common/middleware/init.go` | 配置使用域名查找 |

---

## Verification

### 单元测试

#### 1. `provider_test.go`: 测试 `GetTenantIDByDomain()`

```go
func TestGetTenantIDByDomain(t *testing.T) {
    tests := []struct {
        name     string
        domain   string
        expected int
        found    bool
    }{
        {"Primary 域名匹配", "tenant-alpha.example.com", 1, true},
        {"Primary 大小写不敏感", "Tenant-Alpha.EXAMPLE.COM", 1, true},
        {"Alias 域名匹配", "alias.example.com", 1, true},
        {"Internal 域名匹配", "internal.local", 1, true},
        {"未知域名", "unknown.com", 0, false},
        {"空域名", "", 0, false},
        {"仅空白", "   ", 0, false},
        {"通配符不匹配", "*.tenant.com", 0, false}, // 不支持通配符
    }
    // ...
}
```

#### 2. `provider_test.go`: 测试索引更新

```go
func TestDomainIndexUpdate(t *testing.T) {
    // 1. 初始加载后索引正确
    // 2. ETCD 域名变更后索引更新
    // 3. 租户删除后索引清理
}

func TestDomainConflict(t *testing.T) {
    // 测试域名冲突时后者覆盖前者，并记录警告日志
}
```

#### 3. `extract_tenant_id_test.go`: 测试带 domainLookup 的 host resolver

```go
func TestExtractFromHost_WithDomainLookup(t *testing.T) {
    // 1. 配置 domainLookup 时域名查找成功
    // 2. 未配置 domainLookup 时回退到子域名
    // 3. 域名查找失败时回退到子域名
}
```

### 性能基准测试

```go
func BenchmarkGetTenantIDByDomain(b *testing.B) {
    // 构造测试数据
    provider := &Provider{}
    data := &tenantData{
        Domains: make(map[int]*DomainConfig),
    }
    for i := 0; i < 1000; i++ {
        data.Domains[i] = &DomainConfig{
            Primary:  fmt.Sprintf("tenant-%d.example.com", i),
            Aliases:  []string{fmt.Sprintf("alias-%d.example.com", i)},
            Internal: fmt.Sprintf("internal-%d.local", i),
        }
    }
    data.domainIndex = buildDomainIndex(data)
    provider.data.Store(data)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        provider.GetTenantIDByDomain("tenant-500.example.com")
    }
}

// 目标：< 50ns/op
```

### 集成测试

1. 启动 tenant-service，配置租户域名
2. 启动 security-management，配置 host resolver + domainLookup
3. 发送请求验证：
   ```bash
   curl -H "Host: tenant-alpha.example.com" http://localhost:8000/api/...
   ```
4. 验证返回正确的租户ID

### 验证场景

- [x] Primary 域名匹配
- [x] Alias 域名匹配
- [x] Internal 域名匹配
- [x] 大小写不敏感匹配
- [x] 未知域名回退到子域名提取
- [x] 域名配置变更后索引更新
- [x] 租户删除后域名索引清理
- [x] 空 domain 字符串处理
- [x] domain 包含端口（如 `tenant.com:8080`）
- [x] 并发查找（多个 goroutine 同时调用）- BenchmarkGetTenantIDByDomain_Parallel
- [x] 性能基准 < 50ns/op - 实测 22ns/op ✅
- [x] 通配符域名不匹配（仅精确匹配）
- [x] 域名冲突时后者覆盖并记录警告

---

## Dependencies

- 无外部依赖
- 现有 Provider 和 Middleware 结构保持兼容
- 向后兼容：不配置 `WithDomainLookup()` 时行为不变

---

## Summary

| 设计决策 | 理由 |
|----------|------|
| `domainIndex` 内嵌到 `tenantData` | 一次原子替换，彻底消除一致性窗口，零额外复杂度 |
| 域名规范化（小写） | RFC 4343 域名不区分大小写 |
| 接口定义在 middleware 包 | Go 惯例"消费者定义接口"，Provider 隐式实现，middleware 零依赖 |
| 索引整体重建 | 比增量更新更简单，写入频率低，性能无影响 |
| `buildDomainIndex` 独立函数 | 不依赖 Provider 实例状态，更清晰的函数式设计 |
| 域名冲突检测并记录警告 | 避免静默覆盖，便于排查配置问题 |
| 仅精确匹配，不支持通配符 | 实现简单，满足当前需求 |
| 域名配置始终加载 | 基础设施功能，不受 configTypes 过滤 |
