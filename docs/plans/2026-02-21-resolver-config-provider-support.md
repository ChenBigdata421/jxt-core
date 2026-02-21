# jxt-core Provider 添加租户识别配置（ResolverConfig）设计方案

## 背景

当前架构存在问题：
- **tenant-service** 将租户识别配置发布到 etcd 的 `/common/resolver` 键
- **jxt-core Provider** 虽然加载了 `common/` 前缀，但 `processKey` 函数没有处理该键
- **所有消费方服务** 在中间件中使用硬编码的识别方式

本方案解决第一和第二步：在 Provider 中添加解析逻辑和获取方法。

## 设计方案

### 1. 数据模型定义（models.go）

在 `DomainConfig` 结构体之后添加：

```go
// ResolverConfig 租户识别配置（全局配置，不属于特定租户）
// 从 ETCD key: common/resolver 加载
//
// 注意：通过 Provider 获取的 ResolverConfig 指针应视为只读，不可修改字段值。
// 如需修改，请复制后使用。
type ResolverConfig struct {
    ID             int64     `json:"id"`             // 数据库主键，消费方不使用，仅用于反序列化兼容
    HTTPType       string    `json:"httpType"`       // host, header, query, path
    HTTPHeaderName string    `json:"httpHeaderName"` // For type=header
    HTTPQueryParam string    `json:"httpQueryParam"` // For type=query
    HTTPPathIndex  int       `json:"httpPathIndex"`  // For type=path
    FTPType        string    `json:"ftpType"`        // 目前只支持 username
    CreatedAt      time.Time `json:"createdAt"`      // 创建时间，调试和审计用
    UpdatedAt      time.Time `json:"updatedAt"`      // 更新时间，调试和审计用
}
```

### 2. 修改 tenantData 结构体（provider.go）

```go
type tenantData struct {
    Metas       map[int]*TenantMeta                       `json:"metas"`
    Databases   map[int]map[string]*ServiceDatabaseConfig `json:"databases"`
    Ftps        map[int][]*FtpConfigDetail                `json:"ftps"`
    Storages    map[int]*StorageConfig                    `json:"storages"`
    Domains     map[int]*DomainConfig                     `json:"domains"`
    Resolver    *ResolverConfig                           `json:"resolver"`    // 新增：全局识别配置
    domainIndex map[string]int                            // 内嵌索引
}
```

**注意**：`Resolver` 是指针类型，因为它是全局单例配置。

### 3. 添加键判断函数（provider.go）

```go
func isResolverConfigKey(key string) bool {
    return key == "common/resolver"
}
```

### 4. 添加解析函数（provider.go）

```go
// parseResolverConfig 解析租户识别配置（全局配置）
// 返回 error 以与其他解析函数保持风格一致
// 注意：调用方（processKey）忽略返回值，解析失败时仅跳过该配置
func (p *Provider) parseResolverConfig(key string, value string, data *tenantData) error {
    var config ResolverConfig
    if err := json.Unmarshal([]byte(value), &config); err != nil {
        return fmt.Errorf("failed to unmarshal resolver config: %w", err)
    }
    data.Resolver = &config
    return nil
}
```

### 5. 修改 processKey 分发函数（provider.go）

在 switch 语句中添加 resolver 配置检查：

```go
func (p *Provider) processKey(key, value string, data *tenantData) {
    key = strings.TrimPrefix(key, p.namespace)

    switch {
    case isResolverConfigKey(key):  // 新增
        p.parseResolverConfig(key, value, data)
    case isTenantMetaKey(key):
        p.parseTenantMeta(key, value, data)
    // ... 其他 case
    }
}
```

### 6. 添加获取方法（provider.go）

```go
// GetResolverConfig 获取全局租户识别配置
// 如果未配置返回 nil
func (p *Provider) GetResolverConfig() *ResolverConfig {
    data := p.data.Load().(*tenantData)
    if data == nil {
        return nil
    }
    return data.Resolver
}

// GetResolverConfigOrDefault 获取识别配置，未配置时返回默认值
// 返回值类型（非指针），避免调用方修改污染全局默认值
// 默认：HTTPType=header, HTTPHeaderName=X-Tenant-ID, FTPType=username
func (p *Provider) GetResolverConfigOrDefault() ResolverConfig {
    if cfg := p.GetResolverConfig(); cfg != nil {
        return *cfg
    }
    return ResolverConfig{
        HTTPType:       "header",
        HTTPHeaderName: "X-Tenant-ID",
        FTPType:        "username",
    }
}
```

### 7. 添加 Watch 变更处理（provider.go）

```go
// handleResolverConfigChange 处理租户识别配置变更
func (p *Provider) handleResolverConfigChange(ev *clientv3.Event, key string, data *tenantData) {
    if ev.Type == clientv3.EventTypeDelete {
        data.Resolver = nil
        logger.Info("tenant provider: resolver config deleted")
    } else {
        var config ResolverConfig
        if err := json.Unmarshal(ev.Kv.Value, &config); err != nil {
            logger.Errorf("failed to unmarshal resolver config: %v", err)
            return
        }
        data.Resolver = &config
        logger.Infof("tenant provider: resolver config updated, id=%d, httpType=%s, ftpType=%s",
            config.ID, config.HTTPType, config.FTPType)
    }
}
```

### 8. 修改 handleWatchEvent 分发（provider.go）

**问题**：Watch 范围扩大后，所有 ETCD 事件都会触发昂贵的 `copyData()` 操作，包括 `_health/sentinel` 等无效事件。

**解决方案**：添加前置过滤，只处理已知 key 类型。

```go
func (p *Provider) handleWatchEvent(ev *clientv3.Event) {
    key := string(ev.Kv.Key)
    keyStr := strings.TrimPrefix(key, p.namespace)

    // 前置过滤：只处理已知 key 类型，避免无效事件触发 copyData()
    if !p.isKnownWatchKey(keyStr) {
        return
    }

    current := p.data.Load().(*tenantData)
    newData := current.copyData()

    switch {
    case isResolverConfigKey(keyStr):  // 新增
        p.handleResolverConfigChange(ev, keyStr, newData)
    case isTenantMetaKey(keyStr):
        p.handleTenantMetaChange(ev, keyStr, newData)
    case isServiceDatabaseKey(keyStr):
        p.handleServiceDatabaseChange(ev, keyStr, newData)
    case isFtpConfigKey(keyStr):
        p.handleFtpConfigChange(ev, keyStr, newData)
    case isStorageConfigKey(keyStr):
        p.handleStorageChange(ev, keyStr, newData)
    case isDomainPrimaryKey(keyStr), isDomainAliasesKey(keyStr), isDomainInternalKey(keyStr):
        p.handleDomainChange(ev, keyStr, newData)
    }

    newData.domainIndex = buildDomainIndex(newData)
    p.data.Store(newData)

    // 同步到缓存
    if p.cache != nil {
        go func() {
            if err := p.cache.Save(newData); err != nil {
                logger.Errorf("tenant provider: failed to sync cache: %v", err)
            }
        }()
    }
}

// isKnownWatchKey 检查是否为需要处理的 key
// 用于前置过滤，避免无效事件（如 _health/sentinel）触发 copyData()
func (p *Provider) isKnownWatchKey(key string) bool {
    // tenants/ 下的配置
    if strings.HasPrefix(key, "tenants/") {
        return true
    }
    // 公共配置
    if key == "common/resolver" || key == "common/storage-directory" {
        return true
    }
    return false
}
```

### 9. 扩展 Watch 范围（provider.go）

**当前问题**：Watch 只监听 `tenants/` 前缀，不监听 `common/` 前缀。

**修改位置**：
1. `StartWatch()` 函数 - 初始 Watch 前缀
2. `watchLoop()` 函数 - 重连时的 Watch 前缀
3. `watchLoop()` 函数 - 日志信息

**修改方案**：

```go
// ========== 1. StartWatch() 函数 ==========
func (p *Provider) StartWatch(ctx context.Context) error {
    // ...
    // 修改前：prefix := p.namespace + "tenants/"
    // 修改后：监听整个 namespace，包括 tenants/ 和 common/
    prefix := p.namespace
    watchChan := p.client.Watch(p.watchCtx, prefix, clientv3.WithPrefix())
    // ...
}

// ========== 2. watchLoop() 函数（重连逻辑）==========
func (p *Provider) watchLoop(initialWatchChan clientv3.WatchChan) {
    backoff := InitialBackoff
    watchChan := initialWatchChan

    // 修改前：logger.Infof("tenant provider: started watching ETCD with prefix %s", p.namespace+"tenants/")
    // 修改后：
    logger.Infof("tenant provider: started watching ETCD with prefix %s", p.namespace)

    for {
        select {
        // ...
        case <-time.After(backoff):
            // Re-create the watch
            // 修改前：prefix := p.namespace + "tenants/"
            // 修改后：保持与 StartWatch 一致
            prefix := p.namespace
            watchChan = p.client.Watch(p.watchCtx, prefix, clientv3.WithPrefix())
            // ...
        }
    }
}
```

**注意**：重连逻辑必须与 `StartWatch()` 使用相同的前缀，否则断连后无法监听 `common/resolver` 变更。

### 10. 修改 copyData 函数（provider.go）

```go
func (d *tenantData) copyData() *tenantData {
    newData := &tenantData{
        // ... 其他字段
        Resolver: d.Resolver,  // 新增：复制指针引用（配置被视为不可变）
    }
    // ...
}
```

### 11. 更新 LoadAll 日志（provider.go）

在 `LoadAll` 函数末尾的日志中添加 resolver 加载状态：

```go
logger.Infof("tenant provider: loaded %d tenants, %d database configs, %d ftp configs, %d domain mappings, resolver=%v from ETCD",
    len(newData.Metas), countServiceDatabases(newData.Databases), countFtpConfigs(newData.Ftps),
    len(newData.domainIndex), newData.Resolver != nil)
```

## 修改文件清单

| 文件 | 修改位置 | 修改类型 | 说明 |
|------|----------|----------|------|
| `models.go` | `DomainConfig` 结构体之后 | 新增 | `ResolverConfig` 结构体 |
| `provider.go` | `tenantData` 结构体定义 | 修改 | 添加 `Resolver` 字段 |
| `provider.go` | `isDomainInternalKey()` 函数之后 | 新增 | `isResolverConfigKey()` 函数 |
| `provider.go` | `parseDomainConfig()` 函数之后 | 新增 | `parseResolverConfig()` 函数 |
| `provider.go` | `processKey()` 函数的 switch 语句 | 修改 | 添加 resolver case |
| `provider.go` | `GetTenantIDByDomain()` 方法附近 | 新增 | `GetResolverConfig()` 方法 |
| `provider.go` | `GetResolverConfig()` 方法之后 | 新增 | `GetResolverConfigOrDefault()` 方法（返回值类型，非指针） |
| `provider.go` | `handleDomainChange()` 函数之后 | 新增 | `handleResolverConfigChange()` 函数 |
| `provider.go` | `handleWatchEvent()` 函数开头 | 修改 | 添加前置过滤 `isKnownWatchKey()` |
| `provider.go` | `handleWatchEvent()` 函数附近 | 新增 | `isKnownWatchKey()` 函数 |
| `provider.go` | `StartWatch()` 函数 | 修改 | 扩展 Watch 范围到整个 namespace |
| `provider.go` | `watchLoop()` 函数（重连逻辑和日志） | 修改 | 使用与 StartWatch 相同的前缀 |
| `provider.go` | `copyData()` 函数 | 修改 | 添加 `Resolver` 字段复制 |
| `provider.go` | `NewProvider()` 和 `LoadAll()` | 修改 | 初始化 `Resolver: nil`，更新日志添加 resolver 加载状态 |

## 缓存兼容性

- `tenantData` 使用 JSON 序列化，添加 `json:"resolver"` 标签后自动兼容
- 旧缓存文件没有 `resolver` 字段时会默认为 `nil`
- `GetResolverConfigOrDefault()` 提供默认值兜底

## 验证方案

1. **单元测试**：
   - `TestResolverConfigParsing` - 验证 JSON 解析
   - `TestIsResolverConfigKey` - 验证键匹配
   - `TestGetResolverConfig` - 验证获取方法
   - `TestGetResolverConfigOrDefault` - 验证默认值

2. **集成测试**：
   - `TestLoadAllWithResolver` - 验证 LoadAll 加载 resolver 配置
   - `TestWatchResolverChanges` - 验证 Watch 实时更新

3. **补充测试场景**：
   - **并发安全测试**：多个 goroutine 同时调用 `GetResolverConfig()`
   - **Watch 重连测试**：模拟 ETCD 断开重连后，resolver 配置是否仍能正常更新
   - **缓存降级测试**：ETCD 不可用时，从缓存加载 resolver 配置
   - **边界值测试**：
     - `HTTPPathIndex` 为负数时的行为
     - `HTTPType` 为空字符串或无效值时的处理
     - JSON 中缺少某些字段时的反序列化行为

4. **手动验证**：
   - 启动 tenant-service，确认发布 `/common/resolver`
   - 启动消费方服务，调用 `GetResolverConfig()` 确认读取成功
   - 修改 tenant-service 中的识别配置，确认消费方实时更新

## 依赖关系

### tenant-service → ETCD

- **发布路径**：`/jxt/common/resolver`
- **发布时机**：
  1. 服务启动时（`RefreshCommonConfigs`）
  2. 配置更新时（`PublishTenantResolverConfig`）
  3. ETCD 重连后（`RefreshCommonConfigs`）

### jxt-core Provider → ETCD

- **订阅路径**：`/jxt/common/resolver`
- **加载时机**：
  1. `LoadAll()` 初始化加载
  2. `Watch` 实时监听变更

### 消费方服务 → jxt-core Provider

- **调用方法**：`GetResolverConfig()` 或 `GetResolverConfigOrDefault()`
- **使用场景**：中间件初始化、租户识别逻辑

## 后续工作

本方案完成后，后续工作：
- **步骤 3**：更新消费方服务（evidence-management、file-storage-service、security-management），从 Provider 读取配置而非硬编码
