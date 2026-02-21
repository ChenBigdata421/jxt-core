# jxt-core - 企业级微服务基础框架

[![Go Version](https://img.shields.io/github/go-mod/go-version/ChenBigdata421/jxt-core)](https://golang.org/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

本仓库基于 [go-admin-core](https://github.com/ChenBigdata421/jxt-core) 修改，原项目遵循 Apache 2.0 协议

## 项目简介

jxt-core 是一个基于 Go 语言的企业级微服务基础框架，提供了构建现代化分布式应用所需的核心组件和工具。框架采用模块化设计，支持多种部署模式，适用于大型企业级应用开发。

## 核心特性

### 🚀 基础设施组件
- [x] **日志组件** - 基于 zap 的高性能结构化日志，支持多种输出格式和日志分级
- [x] **配置管理** - 基于 viper 的多格式配置支持（YAML、JSON、TOML等）
- [x] **缓存系统** - 支持 Memory、Redis 多种缓存后端
- [x] **消息队列** - 支持 Memory、Redis、NSQ 多种队列实现
- [x] **分布式锁** - 基于 Redis 的分布式锁实现
- [x] **EventBus 事件总线** - 支持 Kafka、NATS JetStream、Memory 三种实现，统一 API ⭐ **核心组件**
- [x] **Outbox 模式** - 保证业务操作与事件发布的原子性和最终一致性 ⭐ **新增**
- [x] **多租户 Provider** - 基于 ETCD 的多租户配置管理，支持实时监听 ⭐ **新增**

### 🔧 服务治理
- [x] **服务发现** - 基于 ETCD 的服务注册与发现
- [x] **gRPC 支持** - 完整的 gRPC 服务端和客户端实现
- [x] **HTTP 服务** - 基于 Gin 的 RESTful API 支持
- [x] **负载均衡** - 支持多种负载均衡策略
- [x] **监控指标** - 集成 Prometheus 监控指标收集

### 🔐 安全认证
- [x] **JWT 认证** - 完整的 JWT token 生成和验证
- [x] **权限控制** - 基于 Casbin 的 RBAC 权限管理
- [x] **验证码** - 图形验证码生成和验证
- [x] **加密工具** - 常用的加密解密工具集

### 📡 通信协议
- [x] **WebSocket** - 实时双向通信支持
- [x] **gRPC** - 高性能 RPC 通信
- [x] **HTTP/HTTPS** - 标准 Web API 支持

### 💾 数据存储
- [x] **多数据库支持** - MySQL、PostgreSQL、SQLite、SQL Server
- [x] **ORM 集成** - 基于 GORM 的数据库操作
- [x] **读写分离** - 支持主从数据库配置
- [x] **事务管理** - 完整的数据库事务支持
- [x] **服务级数据库配置** - 支持为每个租户的每个微服务配置独立数据库 ⭐ **新增**

### ⏰ 任务调度
- [x] **定时任务** - 基于 cron 的任务调度
- [x] **异步任务** - 基于队列的异步任务处理

### 🛠 开发工具
- [x] **代码生成** - 自动生成常用代码模板
- [x] **工具函数** - 丰富的工具函数库
- [x] **中间件** - 常用的 HTTP 中间件
- [x] **响应封装** - 统一的 API 响应格式

## 快速开始

### 环境要求

- Go 1.23+ 
- Redis 6.0+ (可选)
- MySQL 8.0+ / PostgreSQL 12+ (可选)
- ETCD 3.5+ (用于服务发现和多租户配置，可选)
- Ginkgo/Gomega (测试框架，可选)

### 安装

```bash
go get github.com/ChenBigdata421/jxt-core
```

### 基本使用

```go
package main

import (
    "github.com/ChenBigdata421/jxt-core/sdk"
    "github.com/ChenBigdata421/jxt-core/sdk/config"
)

func main() {
    // 初始化配置
    cfg := config.NewConfig()
    
    // 创建应用实例
    app := sdk.NewApplication(cfg)
    
    // 启动应用
    app.Run()
}
```

### 配置示例

```yaml
# 应用配置
application:
  name: "my-service"
  mode: "dev"
  version: "1.0.0"

# HTTP 服务配置
http:
  host: "0.0.0.0"
  port: 8080

# 数据库配置
database:
  masterDB:
    driver: "mysql"
    source: "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"

# 缓存配置
cache:
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0

# 日志配置
logger:
  level: "info"
  format: "json"

### 多租户配置（Tenants）

框架内置统一的多租户配置，结构定义见 `sdk/config/tenant.go`。核心能力包括：

- `resolver`：支持 HTTP（host/header/query/path 四种模式）与 FTP（username/password）双通道识别；
- `storage`：配置多租户租户目录名（默认 `tenants`，最终路径为 `./uploads/<directory>/<tenant_id>`）；
- `default`：提供默认租户的数据库、域名、FTP 与存储限额等完整初始化信息，首次创建租户时可直接写入数据库。

示例配置：

```yaml
tenants:
  resolver:
    http:
      type: host               # host/header/query/path
      headerName: X-Tenant-ID   # type=header 时的 Header 名
      queryParam: tenant        # type=query 时的 Query Key
      pathIndex: 0              # type=path 时的路径索引
    ftp:
      type: username            # username/password

  storage:
    directory: tenants

  default:
    database:
      driver: postgres
      host: postgres-tenant
      port: 5432
      database: tenant-servicedb
      username: tenant
      password: password123
      sslmode: disable
      max_open_conns: 50
      max_idle_conns: 10
      conn_max_idle_time: 300
      conn_max_life_time: 3600
      connect_timeout: 10
      read_timeout: 30
      write_timeout: 30

    domain:
      primary: app.example.com
      aliases:
        - www.example.com
      internal: app.internal

    ftp:
      username: default_ftp
      initial_password: Default@123456

    storage:
      upload_quota_gb: 1000
      max_file_size_mb: 2048
      max_concurrent_uploads: 20
```

> 更详细的字段含义与环境变量映射，请参考 `docs/tenant.yml`。

### 服务级数据库配置 ⭐

框架支持为每个租户的每个微服务配置独立的数据库连接，实现更细粒度的数据隔离和性能优化。

#### 配置示例

```yaml
tenants:
  default:
    service_databases:
      evidence-command:
        driver: mysql
        host: mysql-command
        port: 3306
        database: tenant_command

      evidence-query:
        driver: postgres
        host: postgres-query
        port: 5432
        database: tenant_query

      file-storage:
        driver: postgres
        host: postgres-storage
        port: 5432
        database: tenant_storage

      security-management:
        driver: postgres
        host: postgres-security
        port: 5432
        database: securitydb
```

#### 使用方法

```go
// 设置服务数据库连接
app.SetTenantServiceDB(tenantID, "evidence-command", db)

// 获取服务数据库连接
db := app.GetTenantServiceDB(tenantID, "evidence-command")

// 遍历所有服务数据库连接
app.GetTenantServiceDBs(func(tenantID int, serviceCode string, db *gorm.DB) bool {
    fmt.Printf("租户 %d 的 %s 服务数据库\n", tenantID, serviceCode)
    return true
})
```

#### 支持的服务代码

- `evidence-command` - 证据管理写服务（CQRS Command 端）
- `evidence-query` - 证据管理读服务（CQRS Query 端）
- `file-storage` - 文件存储服务
- `security-management` - 安全管理服务

#### 向后兼容

旧的 API 仍然可用，并自动映射到服务级配置：

```go
// 这些方法仍然可用，内部映射到服务级配置
db := app.GetTenantDB(tenantID)              // 映射到 security-management
```

详细文档参见: [服务级数据库配置指南](sdk/config/SERVICE_DATABASE_CONFIG.md)

## EventBus 事件总线 ⭐

### 核心特性

- **统一 API**：Kafka、NATS JetStream、Memory 三种实现共享同一套接口
- **Hollywood Actor Pool**：基于 Actor 模型的消息处理，256 个 Actor 并发处理
- **双模式支持**：
  - `Subscribe`：高性能无序并发处理（Round-Robin 路由）
  - `SubscribeEnvelope`：聚合ID 顺序保证（一致性哈希路由）
- **多语义保证**：
  - At-Most-Once（Memory、普通消息）
  - At-Least-Once（Kafka/NATS Envelope）
- **高性能编码**：
  - 默认：JSON（jsoniter，比标准库快 2-3 倍）
  - 可选：Protobuf、Avro、MessagePack、CloudEvents
  - 零拷贝：Cap'n Proto、FlatBuffers（适合极致性能场景）
- **Outbox 集成**：与 Outbox 模式无缝集成，保证事件发布可靠性
- **故障隔离**：Supervisor 自动重启机制，单个聚合故障不影响其他聚合
- **性能监控**：集成 Prometheus 指标，实时监控吞吐量、延迟、错误率

### 性能指标

- **吞吐量**：1900+ msg/s（单实例）
- **延迟**：0.5ms（P99）
- **并发处理**：256 Actor 并发
- **内存占用**：3.4 MB（1000 条消息）
- **监控开销**：Kafka ~3.9%，NATS ~24.5%

### 编码方式选型

| 编码方式 | 速度 | 体积 | 时延 | 适用场景 |
|---------|------|------|------|----------|
| **JSON（默认）** | 基线 | 基线 | 基线 | 通用场景，快速开发 |
| **Protobuf** | ↑↑ | ↑↑ | ↑↑ | 高吞吐、跨语言 |
| **Cap'n Proto** | ↑↑↑ | ↑↑↑ | ↑↑↑ | 极致性能、零拷贝 |
| **FlatBuffers** | ↑↑↑ | ↑↑↑ | ↑↑↑ | 游戏、移动端、IoT |

详见 [EventBus 文档](sdk/pkg/eventbus/README.md)

## 项目结构

```
jxt-core/
├── sdk/                    # 核心 SDK
│   ├── config/            # 配置管理
│   │   └── config.go     # 配置管理逻辑
│   ├── pkg/               # 核心组件包
│   │   ├── eventbus/      # 事件总线（Kafka/NATS/Memory）⭐
│   │   ├── outbox/        # Outbox 模式实现 ⭐
│   │   ├── domain/        # 领域事件模型
│   │   ├── json/          # 统一 JSON 编码（jsoniter）
│   │   ├── logger/        # 日志组件
│   │   ├── jwtauth/       # JWT 认证
│   │   ├── casbin/        # 权限控制
│   │   ├── captcha/       # 验证码
│   │   ├── ws/            # WebSocket
│   │   ├── cronjob/       # 定时任务
│   │   ├── tenant/        # 多租户组件 ⭐
│   │   │   ├── cache/     # 本地文件缓存实现
│   │   │   ├── provider/  # ETCD 配置 Provider
│   │   │   ├── database/  # 数据库配置缓存
│   │   │   ├── ftp/       # FTP 配置缓存
│   │   │   ├── storage/   # 存储配置缓存
│   │   │   └── middleware/ # 租户 ID 提取中间件
│   ├── middleware/        # 中间件
│   └── service/           # 服务层
├── storage/               # 存储层
│   ├── cache/             # 缓存实现
│   ├── queue/             # 队列实现
│   └── locker/            # 分布式锁
├── tests/                 # 测试套件
│   ├── eventbus/          # EventBus 测试
│   │   ├── performance_regression_tests/  # 性能回归测试
│   │   ├── reliability_regression_tests/  # 可靠性测试
│   │   └── function_tests/                # 功能测试
│   └── outbox/            # Outbox 测试
├── examples/              # 使用示例
├── tools/                 # 开发工具
└── docs/                  # 文档
```

## 文档

### 核心文档
- [文档中心](docs/README.md) - 完整的文档索引
- [EventBus 文档](sdk/pkg/eventbus/README.md) - 事件总线使用指南
- [Outbox 模式快速开始](docs/outbox-pattern-quick-start.md) ⭐ - 5 分钟快速上手
- [Outbox 模式完整设计](docs/outbox-pattern-design.md) - 完整的架构设计和使用指南

### 其他文档
- [快速开始指南](docs/quickstart.md)
- [配置文档](docs/config.md)
- [API 文档](docs/api.md)
- [部署指南](docs/deployment.md)

## 贡献

欢迎提交 Issue 和 Pull Request 来帮助改进项目。

## 许可证

本项目采用 [Apache 2.0](LICENSE) 许可证。

## 架构优化

### Hollywood Actor Pool 架构

- **统一消息处理**：Kafka、NATS、Memory 三种实现都使用同一个 Actor Pool
- **智能路由**：
  - 有聚合ID：一致性哈希路由到固定 Actor，保证顺序
  - 无聚合ID：Round-Robin 轮询路由，最大化并发
- **故障隔离**：每个聚合独立 Actor，单个故障不影响全局
- **自动恢复**：Supervisor 机制自动重启失败的 Actor
- **背压控制**：Inbox 队列（1000 容量）提供背压机制

### NATS ACK Worker Pool

- **异步 ACK 处理**：固定大小的 worker 池（默认 2×CPU 核心数）
- **CSP 实现**：基于 Go 原生 channel + goroutine
- **超时保护**：30 秒 ACK 超时，避免永久阻塞
- **多租户支持**：支持租户专属 ACK 通道

### 多租户 Provider 架构

- **ETCD 配置中心**：基于 ETCD 存储租户配置，支持实时监听变更
- **三类配置支持**：Database、FTP、Storage 配置独立管理
- **原子更新**：使用 `atomic.Value` 保证配置读取的线程安全
- **Watch 机制**：自动监听 ETCD 变更，实时同步配置
- **Gin 中间件**：`ExtractTenantID` 中间件自动提取租户 ID

#### 租户识别方式

框架支持四种 HTTP 租户识别方式：

| 类型 | 说明 | 示例 |
|------|------|------|
| `host` | 从 Host 头识别 | `tenant1.example.com` 或精确域名匹配 |
| `header` | 从自定义 Header 识别 | `X-Tenant-ID: 123` |
| `query` | 从 URL 参数识别 | `?tenant=123` |
| `path` | 从 URL 路径识别 | `/tenant-123/users` |

**域名识别（host 类型）支持两种方式**：

1. **精确域名匹配**（优先）：通过 `DomainLookuper` 接口查询域名对应的租户 ID
2. **子域名提取**（兜底）：从 Host 中提取第一部分作为租户 ID（如 `123.example.com` → `123`）

```go
// 方式1：仅子域名提取（子域名必须是数字）
router.Use(ExtractTenantID(WithResolverType("host")))

// 方式2：精确域名匹配 + 子域名 fallback
router.Use(ExtractTenantID(
    WithResolverType("host"),
    WithDomainLookup(provider),  // provider 实现了 DomainLookuper 接口
))

// 方式3：Header 识别
router.Use(ExtractTenantID(
    WithResolverType("header"),
    WithHeaderName("X-Tenant-ID"),
))

// 方式4：Query 参数识别
router.Use(ExtractTenantID(
    WithResolverType("query"),
    WithQueryParam("tenant"),
))
```

#### 使用示例

```go
// 创建 ETCD 客户端
client, _ := clientv3.New(clientv3.Config{
    Endpoints: []string{"localhost:2379"},
})

// 创建 Provider
p := provider.NewProvider(client,
    provider.WithNamespace("jxt/"),
    provider.WithConfigTypes(
        provider.ConfigTypeDatabase,
        provider.ConfigTypeFtp,
        provider.ConfigTypeStorage,
    ),
)

// 加载所有租户配置
p.LoadAll(ctx)

// 启动 Watch 监听变更
p.StartWatch(ctx)

// 获取租户数据库配置
dbConfig := p.GetDatabaseConfig(tenantID)
```

### 性能测试覆盖

- ✅ 高吞吐量场景（1900+ msg/s）
- ✅ 低延迟处理（0.5ms）
- ✅ 消息顺序保证
- ✅ 故障隔离与恢复
- ✅ 协程泄漏检测
- ✅ 内存使用监控

## 版本历史

- v1.0.0 - 初始版本，提供基础框架功能
- v1.1.0 - 新增 EventBus 组件（Kafka/NATS/Memory）
- v1.2.0 - 新增 Outbox 模式，集成 EventBus
- v1.3.0 - Hollywood Actor Pool 架构优化，性能提升 3 倍
- v1.4.0 - 新增 ETCD 多租户 Provider，支持 Database/FTP/Storage 配置实时同步
