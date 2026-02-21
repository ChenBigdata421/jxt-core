# Host 类型租户识别模式配置

## 背景

当前 `extractFromHost` 的行为是**组合式**的：
1. 如果 Provider 可用，先尝试精确域名匹配
2. 回退到子域名提取：
   - 如果子域名是数字，直接用作租户 ID
   - 如果子域名非数字且有 CodeLookuper，尝试租户代码匹配

用户希望支持三种**互斥**模式，只选其一：
- **numeric**: 仅数字子域名（如 `123.example.com`）
- **domain**: 仅精确域名匹配（需 Provider）
- **code**: 仅租户代码匹配（需 Provider）

**配置位置**：ETCD 全局配置（存储在 `provider.ResolverConfig` 中，路径 `common/resolver`）

## 当前代码结构

### 关键结构体

```
sdk/config/tenant.go                    # YAML 配置结构体（启动时读取）
├── Tenants
│   ├── Resolver (config.ResolverConfig)
│   │   ├── HTTP (HTTPResolverConfig)   # type, headerName, queryParam, pathIndex
│   │   └── FTP (FTPResolverConfig)     # type
│   ├── Storage
│   └── Default

sdk/pkg/tenant/provider/models.go       # ETCD 存储结构体
├── ResolverConfig                       # 存储在 ETCD common/resolver
│   ├── HTTPType                         # host, header, query, path
│   ├── HTTPHeaderName
│   ├── HTTPQueryParam
│   ├── HTTPPathIndex
│   └── FTPType
│
├── TenantMeta                           # 租户元数据
├── ServiceDatabaseConfig                # 数据库配置
├── FtpConfigDetail                      # FTP 配置
├── StorageConfig                        # 存储配置
└── DomainConfig                         # 域名配置

sdk/pkg/tenant/provider/provider.go
├── Provider
│   └── data (atomic.Value) -> *tenantData
│       ├── Resolver *ResolverConfig     # 全局识别配置
│       ├── Metas, Databases, Ftps, ...
│       └── domainIndex, codeIndex       # 索引
│
└── Methods
    ├── GetResolverConfig() *ResolverConfig
    ├── GetResolverConfigOrDefault() ResolverConfig
    └── LoadAll(), StartWatch(), ...

sdk/pkg/tenant/middleware/extract_tenant_id.go
├── Config                                # 中间件本地配置
│   ├── resolverType                      # host, header, query, path
│   ├── headerName, queryParam, pathIndex
│   └── domainLookup (DomainLookuper)
│
└── extractFromHost()                     # 当前：组合式逻辑
    ├── 1. 尝试精确域名匹配
    ├── 2. 回退子域名提取
    │   ├── 2a. 数字子域名 -> 直接返回
    │   └── 2b. 非数字 + CodeLookuper -> 租户代码匹配
    └── 3. 都失败 -> 返回 false
```

### 当前配置加载流程

```
settings.yml                    config.Setup()
    │                               │
    │  tenants.resolver.http        ▼
    │  type: "host"          config.TenantsConfig
    │  headerName: ""              │
    │  queryParam: ""              │ tenant-service
    │  pathIndex: 0                │ InitializeResolverConfig()
    │                              │ (启动时初始化到数据库)
    │                              ▼
    │                        tenant_resolver_configs 表
    │                        (数据库)
    │                              │
    │                              │ RefreshCommonConfigs()
    │                              │ PublishTenantResolverConfig()
    │                              ▼
    │                        ETCD common/resolver
    │                        {
    │                          "httpType": "host",
    │                          "httpHeaderName": "",
    │                          ...
    │                        }
    │
    │  消费方微服务
    │  Provider.LoadAll()
    │        │
    │        ▼
    │  tenantData.Resolver -> GetResolverConfig()
    │
    │  中间件当前不读取此配置！
```

**说明**：
- tenant-service 启动时从 `settings.yml` 初始化数据库配置
- 发布时从数据库读取配置发布到 ETCD
- 消费方从 ETCD 读取配置

## 方案设计

### 核心改动

1. **添加 `HTTPHostMode` 字段**到三个 ResolverConfig 结构体（jxt-core 配置、Provider 模型、tenant-service 聚合）
2. **中间件读取 Provider 的 ResolverConfig**，实现互斥模式
3. **tenant-service 数据库和发布逻辑**支持新字段

### 修改文件清单

#### jxt-core 修改

| 文件 | 修改内容 |
|-----|---------|
| `sdk/config/tenant.go` | 在 `HTTPResolverConfig` 添加 `HostMode` 字段 |
| `sdk/pkg/tenant/provider/models.go` | 在 `ResolverConfig` 添加 `HTTPHostMode` 字段 |
| `sdk/pkg/tenant/middleware/extract_tenant_id.go` | 添加 `ResolverConfigProvider` 接口，修改中间件和 `extractFromHost` |

#### tenant-service 修改

| 文件 | 修改内容 |
|-----|---------|
| `tenant/domain/aggregate/tenant_resolver_config.go` | 在 `TenantResolverConfig` 添加 `HTTPHostMode` 字段和辅助方法 |
| `cmd/migrate/migration/models/tenant.go` | 在迁移模型添加 `HTTPHostMode` 字段 |
| `tenant/application/service/config_initializer.go` | 初始化时从 YAML 读取 `HostMode` |
| `distribution/application/service/etcd_publisher_ext.go` | 发布时包含 `httpHostMode` 字段 |

---

## 详细变更

### 1. sdk/config/tenant.go（YAML 配置结构体）

```go
// HTTPResolverConfig HTTP 租户识别配置
type HTTPResolverConfig struct {
    Type       string `mapstructure:"type" yaml:"type"`             // 识别方式: host, header, query, path
    HeaderName string `mapstructure:"headerName" yaml:"headerName"` // 当 type 为 header 时使用的 header 名称
    QueryParam string `mapstructure:"queryParam" yaml:"queryParam"` // 当 type 为 query 时使用的查询参数名
    PathIndex  int    `mapstructure:"pathIndex" yaml:"pathIndex"`   // 当 type 为 path 时使用的路径索引
    HostMode   string `mapstructure:"hostMode" yaml:"hostMode"`     // 【新增】host 模式下的识别方式：numeric, domain, code
}

// GetHostModeOrDefault 返回 host 模式，默认为 "numeric"
func (hrc *HTTPResolverConfig) GetHostModeOrDefault() string {
    if hrc == nil || hrc.HostMode == "" {
        return "numeric"
    }
    return hrc.HostMode
}
```

**配置文件示例**：
```yaml
# config/settings.yml (tenant-service)
tenants:
  resolver:
    http:
      type: "host"
      hostMode: "code"  # 新增：numeric, domain, code
    ftp:
      type: "username"
```

### 2. sdk/pkg/tenant/provider/models.go（ETCD 存储结构体）

```go
// ResolverConfig 租户识别配置（全局配置，不属于特定租户）
// 从 ETCD key: common/resolver 加载
type ResolverConfig struct {
    ID             int64     `json:"id"`
    HTTPType       string    `json:"httpType"`       // host, header, query, path
    HTTPHeaderName string    `json:"httpHeaderName"` // For type=header
    HTTPQueryParam string    `json:"httpQueryParam"` // For type=query
    HTTPPathIndex  int       `json:"httpPathIndex"`  // For type=path
    HTTPHostMode   string    `json:"httpHostMode,omitempty"`  // 【新增】host 模式：numeric, domain, code
    FTPType        string    `json:"ftpType"`
    CreatedAt      time.Time `json:"createdAt"`
    UpdatedAt      time.Time `json:"updatedAt"`
}

// GetHTTPHostModeOrDefault 返回 host 模式，默认为 "numeric"
func (c *ResolverConfig) GetHTTPHostModeOrDefault() string {
    if c == nil || c.HTTPHostMode == "" {
        return "numeric"
    }
    return c.HTTPHostMode
}

// GetHTTPTypeOrDefault 返回识别类型，默认为 "header"
func (c *ResolverConfig) GetHTTPTypeOrDefault() string {
    if c == nil || c.HTTPType == "" {
        return "header"
    }
    return c.HTTPType
}
```

### 3. sdk/pkg/tenant/middleware/extract_tenant_id.go（中间件）

#### 3.1 新增 ResolverConfigProvider 接口

```go
// ResolverConfigProvider 定义获取识别配置的接口
// Provider 已实现此接口（GetResolverConfig 方法已存在）
type ResolverConfigProvider interface {
    GetResolverConfig() *ResolverConfig
}
```

#### 3.2 修改 Config 结构体

```go
// Config holds the middleware configuration
type Config struct {
    resolverType    ResolverType
    headerName      string
    queryParam      string
    pathIndex       int
    onMissingTenant MissingTenantMode
    domainLookup    DomainLookuper
    // 不需要新增字段，通过 domainLookup 断言获取 ResolverConfig
}
```

#### 3.3 修改 ExtractTenantID 中间件

```go
func ExtractTenantID(opts ...Option) gin.HandlerFunc {
    cfg := defaultConfig()
    for _, opt := range opts {
        opt(cfg)
    }

    // 从 Provider 获取 ResolverConfig（如果可用）
    var resolverCfg *ResolverConfig
    if cfg.domainLookup != nil {
        if rcp, ok := cfg.domainLookup.(ResolverConfigProvider); ok {
            resolverCfg = rcp.GetResolverConfig()
            // 从 ETCD 配置覆盖 resolverType
            if resolverCfg != nil && resolverCfg.HTTPType != "" {
                cfg.resolverType = ResolverType(resolverCfg.HTTPType)
            }
        }
    }

    return func(c *gin.Context) {
        var tenantIDStr string
        var ok bool

        switch cfg.resolverType {
        case ResolverTypeHost:
            tenantIDStr, ok = extractFromHost(c, cfg.domainLookup, resolverCfg)
        case ResolverTypeHeader:
            tenantIDStr, ok = extractFromHeader(c, cfg.headerName)
        case ResolverTypeQuery:
            tenantIDStr, ok = extractFromQuery(c, cfg.queryParam)
        case ResolverTypePath:
            tenantIDStr, ok = extractFromPath(c, cfg.pathIndex)
        default:
            ok = false
        }
        // ... 后续处理不变
    }
}
```

#### 3.4 修改 extractFromHost（实现互斥模式）

```go
// extractFromHost 根据 httpHostMode 执行互斥的租户识别
func extractFromHost(c *gin.Context, lookup DomainLookuper, resolverCfg *ResolverConfig) (string, bool) {
    host := c.Request.Host
    if host == "" {
        return "", false
    }

    // Remove port
    if idx := strings.Index(host, ":"); idx != -1 {
        host = host[:idx]
    }

    // 提取子域名
    parts := strings.Split(host, ".")
    if len(parts) < 2 {
        return "", false
    }
    subdomain := parts[0]
    if subdomain == "" || subdomain == "www" {
        return "", false
    }

    // 根据模式执行互斥逻辑
    mode := resolverCfg.GetHTTPHostModeOrDefault()

    switch mode {
    case "domain":
        // 仅精确域名匹配
        if lookup != nil {
            if tenantID, ok := lookup.GetTenantIDByDomain(host); ok {
                return strconv.Itoa(tenantID), true
            }
        }
        return "", false

    case "code":
        // 仅租户代码匹配
        if lookup != nil {
            if codeLookup, ok := lookup.(CodeLookuper); ok {
                if tenantID, ok := codeLookup.GetTenantIDByCode(strings.ToLower(subdomain)); ok {
                    return strconv.Itoa(tenantID), true
                }
            }
        }
        return "", false

    default: // "numeric"
        // 仅数字子域名
        if _, err := strconv.Atoi(subdomain); err == nil {
            return subdomain, true
        }
        return "", false
    }
}
```

---

## 配置流转流程

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│                           租户识别配置完整流程                                           │
├──────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                      │
│  1. 配置文件                   2. 初始化到数据库              3. 发布到 ETCD           │
│  tenant-service                tenant-service                tenant-service          │
│  config/settings.yml    ──▶    InitializeResolverConfig ──▶  PublishTenantResolver  │
│                                                                Config                │
│  tenants:                      从 config.TenantsConfig        从数据库读取           │
│    resolver:                   读取并保存到数据库              TenantResolverConfig   │
│      http:                     tenant_resolver_configs 表                           │
│        type: "host"                     │                           │               │
│        hostMode: "code"                 │                           │               │
│                                         ▼                           ▼               │
│                              TenantResolverConfig         ETCD common/resolver      │
│                              (数据库记录)                  {                         │
│                              - HTTPType: "host"             "httpType": "host",     │
│                              - HTTPHostMode: "code"         "httpHostMode": "code", │
│                                                             ...                     │
│                                                            }                        │
│                                                             │                       │
│                                                             │ ETCD Watch            │
│                                                             ▼                       │
│  4. 缓存                                                                            │
│  消费方微服务                                                                        │
│  Provider.LoadAll()                                                                 │
│        │                                                                            │
│        ▼                                                                            │
│  tenantData.Resolver                                                                │
│        │                                                                            │
│        ▼                                                                            │
│  Provider.GetResolverConfig()                                                       │
│                                                                                      │
│                                         5. 应用                                      │
│                                         中间件                                        │
│                                         ExtractTenantID()                            │
│                                           │                                         │
│                                           ▼                                         │
│                                         extractFromHost()                            │
│                                         根据 httpHostMode 执行                        │
│                                                                                      │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

**关键说明**：
1. **启动时初始化**：tenant-service 从 `settings.yml` 读取配置，通过 `InitializeResolverConfig` 保存到数据库
2. **发布到 ETCD**：从数据库读取 `TenantResolverConfig`，通过 `PublishTenantResolverConfig` 发布到 ETCD
3. **消费方读取**：其他微服务通过 Provider 从 ETCD 读取配置，中间件自动应用

---

## tenant-service 修改详情

### 4. tenant/domain/aggregate/tenant_resolver_config.go（聚合）

```go
// TenantResolverConfig 租户识别配置
type TenantResolverConfig struct {
    Id int64 `json:"id" gorm:"primaryKey;autoIncrement"`

    // HTTP 识别配置
    HTTPType       string `json:"httpType" gorm:"size:16;default:host;not null"`
    HTTPHeaderName string `json:"httpHeaderName" gorm:"size:64"`
    HTTPQueryParam string `json:"httpQueryParam" gorm:"size:64"`
    HTTPPathIndex  int    `json:"httpPathIndex" gorm:"default:0"`
    HTTPHostMode   string `json:"httpHostMode" gorm:"size:16;default:numeric"` // 【新增】

    // FTP 识别配置
    FTPType string `json:"ftpType" gorm:"size:16;default:username;not null"`

    CreatedAt time.Time `json:"createdAt"`
    UpdatedAt time.Time `json:"updatedAt"`
}

// GetHTTPHostModeOrDefault 返回 host 模式，默认为 "numeric"
func (c *TenantResolverConfig) GetHTTPHostModeOrDefault() string {
    if c.HTTPHostMode == "" {
        return "numeric"
    }
    return c.HTTPHostMode
}
```

### 5. cmd/migrate/migration/models/tenant.go（迁移模型）

```go
// TenantResolverConfig 租户识别配置
type TenantResolverConfig struct {
    Id int64 `gorm:"primaryKey;autoIncrement"`
    // HTTP 识别配置
    HTTPType       string `gorm:"size:16;default:host;not null"`
    HTTPHeaderName string `gorm:"size:64"`
    HTTPQueryParam string `gorm:"size:64"`
    HTTPPathIndex  int    `gorm:"default:0"`
    HTTPHostMode   string `gorm:"size:16;default:numeric"` // 【新增】
    // FTP 识别配置
    FTPType   string `gorm:"size:16;default:username;not null"`
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 6. tenant/application/service/config_initializer.go（初始化）

```go
// InitializeResolverConfig 初始化租户识别配置
func (s *ConfigInitializerService) InitializeResolverConfig(ctx context.Context, resolverCfg *config.ResolverConfig) error {
    // 检查是否已存在
    existing, err := s.resolverConfigRepo.Get(ctx)
    if err != nil {
        return fmt.Errorf("failed to check existing resolver config: %w", err)
    }

    if existing != nil {
        logger.Info("resolver config already exists, skipping initialization")
        return nil
    }

    // 创建新配置
    newConfig := &aggregate.TenantResolverConfig{
        HTTPType:       resolverCfg.HTTP.Type,
        HTTPHeaderName: resolverCfg.HTTP.HeaderName,
        HTTPQueryParam: resolverCfg.HTTP.QueryParam,
        HTTPPathIndex:  resolverCfg.HTTP.PathIndex,
        HTTPHostMode:   resolverCfg.HTTP.HostMode,  // 【新增】从 YAML 读取
        FTPType:        resolverCfg.FTP.Type,
        CreatedAt:      time.Now(),
        UpdatedAt:      time.Now(),
    }

    if err := s.resolverConfigRepo.Create(ctx, newConfig); err != nil {
        return fmt.Errorf("failed to create resolver config: %w", err)
    }

    logger.Info("resolver config initialized from settings.yml")
    return nil
}
```

### 7. distribution/application/service/etcd_publisher_ext.go（发布）

**修改现有的 `PublishTenantResolverConfig` 方法**：

```go
// PublishTenantResolverConfig 发布租户识别配置（公共配置，全局唯一）
func (p *EtcdPublisher) PublishTenantResolverConfig(ctx context.Context, config *tenantAggregate.TenantResolverConfig) error {
    if p.client == nil {
        logger.Warn("etcd client not available, skip publishing resolver config")
        return nil
    }

    key := "/common/resolver"
    data := map[string]interface{}{
        "id":             config.Id,
        "httpType":       config.HTTPType,
        "httpHeaderName": config.HTTPHeaderName,
        "httpQueryParam": config.HTTPQueryParam,
        "httpPathIndex":  config.HTTPPathIndex,
        "httpHostMode":   config.HTTPHostMode,  // 【新增】
        "ftpType":        config.FTPType,
        "createdAt":      config.CreatedAt,
        "updatedAt":      config.UpdatedAt,
    }

    value, err := json.Marshal(data)
    if err != nil {
        return fmt.Errorf("failed to marshal resolver config: %w", err)
    }

    if err := p.client.Put(ctx, key, string(value)); err != nil {
        return fmt.Errorf("failed to publish resolver config to etcd: %w", err)
    }

    logger.Info("resolver config published to etcd")
    return nil
}
```

**说明**：
- tenant-service 从**数据库**读取 `TenantResolverConfig`（不是从 `config.TenantsConfig`）
- 启动时通过 `InitializeResolverConfig` 从 `settings.yml` **初始化**数据库配置
- 发布时通过 `RefreshCommonConfigs` 调用 `PublishTenantResolverConfig` 发布到 ETCD

---

## 支持的模式

| httpHostMode | 说明 | 示例 | 需要 Provider |
|--------------|------|------|---------------|
| `numeric`（默认） | 仅数字子域名 | `123.example.com` → `123` | 否 |
| `domain` | 仅精确域名 | `tenant1.example.com` → 查域名配置 | 是 |
| `code` | 仅租户代码 | `acmeCorp.example.com` → 查租户代码 | 是 |

### 向后兼容

- `httpHostMode` 为空或不存在时，默认为 `numeric`
- `httpType` 为空或不存在时，默认为 `header`（与 Provider.GetResolverConfigOrDefault() 一致）
- 现有 ETCD 配置无需修改，行为与现在一致

---

## 消费方微服务（无需修改代码）

现有代码：
```go
// cmd/main.go
p := provider.NewProvider(etcdClient, ...)
p.LoadAll(ctx)
go p.StartWatch(ctx)

// router.go
router.Use(middleware.ExtractTenantID(
    middleware.WithDomainLookup(p),  // 自动从 Provider 读取 httpType 和 httpHostMode
))
```

### 配置热更新

1. 修改 tenant-service 的 `settings.yml`
2. 重启 tenant-service，发布新配置到 ETCD
3. 消费方微服务的 Provider 通过 Watch 自动更新
4. 下次请求时，中间件使用新配置

---

## 测试用例

| 测试场景 | 输入条件 | 期望结果 |
|---------|---------|---------|
| **无 Provider** | 不传 `WithDomainLookup`，Host: `123.example.com` | 识别为租户 `123`（默认 numeric） |
| **numeric 模式** | `httpHostMode: "numeric"`，Host: `456.example.com` | 识别为租户 `456` |
| **numeric 模式，非数字** | `httpHostMode: "numeric"`，Host: `acme.example.com` | 识别失败 |
| **domain 模式，匹配成功** | `httpHostMode: "domain"`，Host: `tenant1.example.com`，Provider 有该域名 | 识别成功 |
| **domain 模式，匹配失败** | `httpHostMode: "domain"`，Host: `unknown.example.com` | 识别失败 |
| **code 模式，匹配成功** | `httpHostMode: "code"`，Host: `acmecorp.example.com`，Provider 有该代码 | 识别成功 |
| **code 模式，大小写** | `httpHostMode: "code"`，Host: `ACMECorp.example.com` | 识别成功（忽略大小写） |
| **无效 httpHostMode** | `httpHostMode: "invalid"` | 回退到 numeric 模式 |
| **httpType=header** | `httpType: "header"`，Header: `X-Tenant-ID: 789` | 识别为租户 `789` |

---

## 验证方式

```bash
cd jxt-core

# 运行中间件测试
go test ./sdk/pkg/tenant/middleware/... -v -run TestExtractFromHost

# 运行 Provider 测试
go test ./sdk/pkg/tenant/provider/... -v

# 构建
go build ./sdk/pkg/tenant/...
```
