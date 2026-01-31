# 多租户配置管理模块

[![Go Report Card](https://goreportcard.com/badge/github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant)]
[![Coverage](https://codecov.io/gh/ChenBigdata421/jxt-core/pkg/tenant/branch/main/graph/badge.svg)]

> 为所有 JXT 微服务提供共享的租户配置管理，集成 ETCD、自动重连和持久化缓存降级。

## 目录

- [快速开始](#快速开始)
- [核心概念](#核心概念)
- [架构设计](#架构设计)
- [API 参考](#api-参考)
- [使用示例](#使用示例)
- [配置说明](#配置说明)
- [可靠性特性](#可靠性特性)
- [故障排查](#故障排查)
- [扩展指南](#扩展指南)

---

## 快速开始

### 安装

```bash
go get github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant
```

### 基本用法

```go
package main

import (
    "context"
    "log"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/database"
    clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
    // 1. 创建 ETCD 客户端
    etcdClient, err := clientv3.New(clientv3.Config{
        Endpoints:   []string{"localhost:2379"},
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer etcdClient.Close()

    // 2. 创建 Provider（带重试和缓存）
    prov, err := provider.NewProviderWithRetry(etcdClient,
        provider.WithNamespace("jxt/"),
        provider.WithConfigTypes(
            provider.ConfigTypeDatabase,
            provider.ConfigTypeFtp,
            provider.ConfigTypeStorage,
        ),
        provider.WithCache(provider.NewFileCache()),
    )
    if err != nil {
        log.Fatal(err)
    }

    // 3. 启动 Watch（带重试）
    ctx := context.Background()
    if err := prov.StartWatchWithRetry(ctx); err != nil {
        log.Fatal(err)
    }
    defer prov.StopWatch()

    // 4. 使用缓存
    dbCache := database.NewCache(prov)
    cfg, err := dbCache.GetByID(ctx, 1)
    if err != nil {
        log.Fatal(err)
    }

    // 使用配置...
    _ = cfg
}
```

---

## 核心概念

### 配置与状态分离

租户模块遵循 CQRS 原则，实现清晰的配置与状态分离：

```
┌─────────────────────────────────────────────────┐
│  ETCD（由 tenant-service 管理）                   │
│  - 数据库配置                                     │
│  - FTP 配置                                      │
│  - 存储配置                                       │
│  - 元数据: code, name, status                    │
└─────────────────────────────────────────────────┘
                    ↓ Watch
┌─────────────────────────────────────────────────┐
│  jxt-core Provider（内存缓存）                   │
│  - Copy-on-Write 无锁读取                        │
│  - 指数退避自动重连                               │
│  - 持久化文件缓存降级                             │
└─────────────────────────────────────────────────┘
                    ↓ Cache API
┌─────────────────────────────────────────────────┐
│  微服务（file-storage, evidence 等）              │
│  - database.Cache → DatabaseConfig               │
│  - ftp.Cache → FtpConfig                         │
│  - storage.Cache → StorageConfig                 │
└─────────────────────────────────────────────────┘
```

### 租户数据流

1. **配置写入**：tenant-service → ETCD
2. **配置分发**：ETCD Watch → Provider → 微服务
3. **本地降级**：ETCD 不可用 → 本地缓存文件 → 内存

---

## 架构设计

### 分层架构

```
┌─────────────────────────────────────────────────────┐
│  中间件层                                            │
│  - ExtractTenantID（4 种解析方式）                   │
│    * host:   子域名提取                              │
│    * header: 自定义 Header（默认: X-Tenant-ID）       │
│    * query:  查询参数（默认: tenant）                 │
│    * path:   URL 路径片段索引                        │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│  缓存层（按需加载）                                   │
│  - database.Cache  → DatabaseConfig                 │
│  - ftp.Cache       → FtpConfig                      │
│  - storage.Cache   → StorageConfig                  │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│  Provider 层（核心）                                  │
│  - ETCD Watch + 指数退避重连                         │
│  - Copy-on-Write 无锁读取                            │
│  - 持久化文件缓存降级                                 │
│  - 初始化重试机制                                     │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│  持久化层                                            │
│  - ETCD（配置源）                                     │
│  - FileCache（本地降级）                              │
└─────────────────────────────────────────────────────┘
```

### 并发安全性

| 操作 | 机制 | 延迟 |
|------|------|------|
| 读取配置 | `atomic.Value` | ~25ns |
| 更新配置 | Copy-on-Write | ~1μs |
| Watch 重连 | 指数退避 1s→30s | - |

---

## API 参考

### Provider 配置选项

```go
// 命名空间配置
WithNamespace(ns string) Option

// 配置类型过滤（按需加载）
WithConfigTypes(types ...ConfigType) Option

// 本地缓存降级
WithCache(cache FileCache) Option
```

### 核心方法

| 方法 | 描述 | 返回值 |
|------|------|--------|
| `NewProvider(client, opts...)` | 创建基础 Provider | `*Provider` |
| `NewProviderWithRetry(client, opts...)` | 创建并初始化（带重试） | `*Provider, error` |
| `LoadAll(ctx)` | 从 ETCD 加载所有配置 | `error` |
| `StartWatch(ctx)` | 启动 ETCD 监听 | `error` |
| `StartWatchWithRetry(ctx)` | 启动监听（带重试） | `error` |
| `StopWatch()` | 停止监听 | - |
| `GetDatabaseConfig(id)` | 获取数据库配置 | `*DatabaseConfig, bool` |
| `GetFtpConfig(id)` | 获取 FTP 配置 | `*FtpConfig, bool` |
| `GetStorageConfig(id)` | 获取存储配置 | `*StorageConfig, bool` |
| `GetTenantMeta(id)` | 获取租户元数据 | `*TenantMeta, bool` |
| `IsTenantEnabled(id)` | 检查租户是否激活 | `bool` |
| `NewFileCache()` | 创建默认文件缓存 | `FileCache` |
| `NewFileCacheWithPath(path)` | 创建指定路径的文件缓存 | `FileCache` |

### Cache API

#### database.Cache

```go
func NewCache(prov *provider.Provider) *Cache
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantDatabaseConfig, error)
// 注意: GetByCode(ctx, code string) 计划在未来版本中实现
```

#### ftp.Cache

```go
func NewCache(prov *provider.Provider) *Cache
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantFtpConfig, error)
```

#### storage.Cache

```go
func NewCache(prov *provider.Provider) *Cache
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantStorageConfig, error)
```

---

## 使用示例

### 场景 1：数据库连接

```go
dbCache := database.NewCache(prov)

cfg, err := dbCache.GetByID(ctx, tenantID)
if err != nil {
    return err
}

// 创建 GORM 数据库连接
gormDB, err := gorm.Open(mysql.Open(fmt.Sprintf(
    "%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True",
    cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.DbName,
)), &gorm.Config{
    MaxIdleConns: cfg.MaxIdleConns,
    MaxOpenConns: cfg.MaxOpenConns,
})
```

### 场景 2：配额检查

```go
storageCache := storage.NewCache(prov)

cfg, _ := storageCache.GetByID(ctx, tenantID)
if file.Size > cfg.MaxFileSizeBytes {
    return errors.New("文件超过大小限制")
}

// 检查并发上传
if activeUploads >= cfg.MaxConcurrentUploads {
    return errors.New("已达到最大并发上传数")
}
```

### 场景 3：租户状态验证

```go
if !prov.IsTenantEnabled(tenantID) {
    return errors.New("租户未激活")
}
```

### 场景 4：中间件集成

租户模块包含一个 Gin 中间件，用于从 HTTP 请求中提取租户 ID。支持四种解析方式：

```go
import "github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/middleware"

// 默认：从 X-Tenant-ID header 提取
router.Use(middleware.ExtractTenantID())

// 自定义 header 名称
router.Use(middleware.ExtractTenantID(
    middleware.WithHeaderName("X-Tenant-Id"),
))

// 查询参数提取
router.Use(middleware.ExtractTenantID(
    middleware.WithResolverType("query"),
    middleware.WithQueryParam("tenant_id"),
))

// 基于路径提取（提取第一个片段）
router.Use(middleware.ExtractTenantID(
    middleware.WithResolverType("path"),
    middleware.WithPathIndex(0),
))

// 基于域名提取（子域名）
router.Use(middleware.ExtractTenantID(
    middleware.WithResolverType("host"),
))

// 在处理器中获取租户 ID
func handler(c *gin.Context) {
    tenantID := middleware.GetTenantID(c)
    // 或转换为整数
    tenantIDInt, err := middleware.GetTenantIDAsInt(c)
}
```

---

## 配置说明

### 环境变量

| 变量 | 默认值 | 描述 |
|------|--------|------|
| `TENANT_CACHE_PATH` | `./cache/` | 本地缓存文件目录 |
| `ETCD_ENDPOINTS` | `localhost:2379` | ETCD 服务地址 |
| `TENANT_NAMESPACE` | `jxt/` | ETCD 命名空间前缀 |

### ETCD 数据结构

```
jxt/tenants/{id}/meta        → {"id":1,"code":"tenant1","name":"租户1","status":"active"}
jxt/tenants/{id}/database    → {"host":"localhost","port":3306,...}
jxt/tenants/{id}/ftp         → {"username":"ftp1","passwordHash":"...",...}
jxt/tenants/{id}/storage     → {"uploadQuotaGb":100,"maxFileSizeMb":500,...}
```

---

## 可靠性特性

### 1. 自动重连

- **指数退避**：1s → 30s（最大）
- **重置条件**：成功处理事件后退避重置为 1s
- **所有重连事件**：记录日志供监控

### 2. 初始化重试

| 操作 | 重试次数 | 间隔 | 总耗时 |
|------|----------|------|--------|
| LoadAll | 5 | 1s→2s→4s→8s→16s | ~31s |
| StartWatch | 3 | 1s→2s→5s | ~8s |

### 3. 本地缓存降级

```
ETCD 可用:  ETCD Watch → 内存 → 本地文件（同步）
ETCD 不可用: 本地文件 → 内存（只读模式）
```

### 4. 优雅关闭

```go
defer prov.StopWatch()  // 停止监听，刷新缓存
```

---

## 故障排查

### 问题 1：ETCD 连接失败

**症状**：`failed to load tenant data after retries`

**排查步骤**：
```bash
# 检查 ETCD 服务
curl http://localhost:2379/health

# 检查租户数据
etcdctl get "" --prefix --keys-only
```

**解决方案**：
- 启用本地缓存降级（`WithCache()`）
- 检查网络连接
- 查看重连日志

### 问题 2：配置未更新

**症状**：tenant-service 已更新，但微服务未感知

**排查步骤**：
```go
// 检查 watch 是否运行
if !prov.running.Load() {
    log.Error("Provider 未运行")
}
```

**解决方案**：
- 确认 `StartWatch()` 成功
- 检查 ETCD 事件传递
- 查看重连日志

### 问题 3：本地缓存文件损坏

**症状**：`invalid cache file format`

**解决方案**：
```go
// 删除缓存文件，下次启动会重新生成
os.Remove("./cache/tenant_metadata.json")
```

---

## 扩展指南

### 添加新的配置类型

1. 定义 ConfigType：
```go
const ConfigTypeCustom ConfigType = "custom"
```

2. 定义配置模型：
```go
type CustomConfig struct {
    TenantID int    `json:"tenantId"`
    // ...
}
```

3. 扩展 processKey/handleDeleteKey：
```go
case "custom":
    p.processCustomKey(tenantID, parts, value, data)
```

4. 创建 Cache：
```go
// cache/custom/cache.go
type Cache struct { provider *provider.Provider }

func (c *Cache) GetByID(ctx, tenantID int) (*CustomConfig, error) {
    // ...
}
```

---

## 性能指标

| 指标 | 数值 | 描述 |
|------|------|------|
| 读取延迟 | ~25ns | atomic.Value 无锁读取 |
| 更新延迟 | ~1μs | Copy-on-Write |
| 内存占用 | ~1KB/租户 | 包含所有配置类型 |
| Watch 同步 | <1s | ETCD 推送延迟 |

---

## 相关文档

- [ETCD Client v3 文档](https://etcd.io/docs/v3.5/learning/api/)
- [avast/retry-go 文档](https://github.com/avast/retry-go)
- [jxt-core EventBus](../../README_EventBus.md)
- [实现计划](../../../../../docs/plans/2026-01-31-tenant-reliability-enhancement.md)

---

## 许可证

Copyright © 2026 JXT-Evidence-System
