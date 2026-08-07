# jxt-core - 企业级微服务基础框架

[![Go Version](https://img.shields.io/github/go-mod/go-version/ChenBigdata421/jxt-core)](https://golang.org/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

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
- [x] **Outbox 模式** - 保证业务操作与事件发布的原子性和最终一致性；同步/异步发布器标记分流 + 批量标记（v2.0.0）、异步 ACK 攒批（v1.1.59）⭐
- [x] **可靠消费内核 (reliable)** - 五状态机 + fencing token 幂等去重 + 死信 + 重投调度（一张 event_consumption 表三者合一）；at-least-once + 租约自愈 + aggregate gate 保序；双方言（MySQL/PostgreSQL）Store + 重放调度器 + 租约孤儿观测 ⭐ **核心组件（v1.1.68）**
- [x] **多租户 Provider** - 基于 ETCD 的多租户配置管理，支持实时监听、租户识别配置缓存 ⭐ **核心组件**

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

- Go 1.26+
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
	// 1. 加载配置文件（viper 解析 settings.yml 到 sdk/config.AppConfig）
	config.Setup("config/settings.yml")

	// 2. sdk.Runtime 是框架预初始化的单例（实现 runtime.Runtime 接口），
	//    通过它装配 / 获取各组件，例如：
	//      sdk.Runtime.SetEngine(engine)   // HTTP 路由引擎
	//      sdk.Runtime.SetEventBus(bus)    // 事件总线
	//      sdk.Runtime.SetTenantDB(id, db) // 租户数据库
	_ = sdk.Runtime
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
  path: "./logs"

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
      hostMode: numeric        # type=host 时的模式：numeric（默认）/domain/code
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
  - 可选：Protobuf、Avro、CloudEvents
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

| 编码方式          | 速度 | 体积 | 时延 | 适用场景       |
| --------------- | -- | -- | -- | ---------- |
| **JSON（默认）**   | 基线 | 基线 | 基线 | 通用场景，快速开发  |
| **Protobuf**    | ↑↑ | ↑↑ | ↑↑ | 高吞吐、跨语言    |
| **Avro**        | ↑↑ | ↑↑ | ↑↑ | 模式演进、大数据存储 |
| **CloudEvents** | 基线 | 基线 | 基线 | 跨平台事件标准封装  |

### Kafka 预订阅语义（v1.7.1 起：未激活 topic 由 drain 改 hold）

- **drain → hold**：Kafka 预订阅消费对"handler 尚未激活的 topic"，从「跳过并提交」（drain，静默丢失）改为「背压 hold」——不读通道、不提交、不推进 frontier，待 handler 激活后按序处理；会话结束仍未激活时，已读未处理消息下个会话重投递（at-least-once）。**任何「故意预订阅但不激活」的 topic 会永久停滞分区**（从静默丢失变为静默卡死），升级前必须完成 consumed ⊆ activated 审计
- **停滞可观测性**：hold 进入时记录一次性 Warn 日志 + `consumption_partition_stalled_seconds` 指标（monotonic stall-enter 上升沿 + 实时时长爬升、退出归零），配合 `IsActiveTopic` 启动自检
- **`IsActiveTopic(topic string) bool`**（仅 kafka 驱动 `*kafkaEventBus`）：供服务侧启动时校验 consumed ⊆ activated，鸭子类型断言（NATS/Memory 驱动断言返回 ok=false）：

```go
if it, ok := bus.(interface{ IsActiveTopic(topic string) bool }); ok && it.IsActiveTopic(topic) {
    // kafka 驱动：handler 已激活
}
```

- **`HoldBackoff`**：内部 timing 字段（默认 100ms，<=0 钳位），非用户配置——timing 安全不变量不进 `PipelineUserConfig`

详见 [EventBus 文档](sdk/pkg/eventbus/README.md)

## 项目结构

```
jxt-core/
├── sdk/                       # 核心 SDK
│   ├── runtime/              # 应用运行时（Application 单例：租户/服务级 DB、Casbin、Cron、EventBus 装配）
│   ├── config/               # 配置管理（settings.yml 解析、多租户配置、Redis 多客户端）
│   ├── api/                  # 通用 API 请求/响应绑定类型
│   ├── antd_api/             # Ant Design Pro 风格 API 绑定类型
│   ├── restapi/              # REST API 类型定义
│   ├── middleware/           # HTTP 中间件
│   ├── service/              # 服务层
│   └── pkg/                  # 核心组件包
│       ├── eventbus/         # 事件总线（Kafka/NATS/Memory）⭐
│       ├── outbox/           # Outbox 模式实现 ⭐
│       ├── reliable/         # 可靠消费内核（状态机 + 死信 + 重投 + 隔离区）⭐
│       │   ├── store/        # Store 接口 + Row/QuarantineRow 模型
│       │   │   ├── gormshared/  # MySQL/PostgreSQL 共享 GORM 实现
│       │   │   ├── mysql/       # MySQL 方言（migration + classifier）
│       │   │   ├── postgres/    # PostgreSQL 方言（migration + classifier）
│       │   │   └── repotest/    # 双方言 conformance 测试套件
│       │   ├── replay/       # eligible-head 重放调度器
│       │   └── lease/        # 租约孤儿观测 runner
│       ├── domain/           # 领域事件模型 ⭐
│       ├── tenant/           # 多租户组件（ETCD Provider + 配置缓存）⭐
│       │   ├── provider/     # ETCD 配置 Provider
│       │   ├── cache/        # 本地文件缓存实现
│       │   ├── database/     # 数据库配置缓存
│       │   ├── ftp/          # FTP 配置缓存
│       │   ├── storage/      # 存储配置缓存
│       │   ├── wvp/          # WVP 配置缓存
│       │   └── middleware/   # 租户 ID 提取中间件
│       ├── logger/           # 日志组件（zap）
│       ├── json/             # 统一 JSON 编码（jsoniter）
│       ├── contextpool/      # Context 对象池
│       ├── jwtauth/          # JWT 认证
│       ├── casbin/           # 权限控制（多租户 + PSUBSCRIBE 策略同步）
│       ├── captcha/          # 验证码
│       ├── crypto/           # 加密工具（AES-256-GCM）
│       ├── ws/               # WebSocket
│       ├── cronjob/          # 定时任务
│       ├── migration/        # 数据库迁移框架
│       ├── response/         # 统一响应封装
│       ├── table/            # 表结构分析（代码生成）
│       └── utils/            # 工具函数
├── storage/                   # 存储适配层
│   ├── cache/                # 缓存实现（Memory/Redis）
│   ├── queue/                # 队列实现（Memory/Redis/NSQ）
│   └── locker/               # 分布式锁（Redis）
├── errors/                    # 统一错误码
├── debug/                     # 日志文件 writer
├── tests/                     # 测试套件
│   ├── eventbus/             # EventBus 测试
│   │   ├── performance_regression_tests/  # 性能回归测试
│   │   ├── reliability_regression_tests/  # 可靠性测试
│   │   └── function_tests/                # 功能测试
│   ├── outbox/               # Outbox 测试
│   ├── config/               # 配置测试
│   └── domain/               # 领域事件测试
├── examples/                  # 使用示例
├── tools/                     # 开发工具（database/etcd/poster/search/transfer/language/utils）
└── docs/                      # 文档
```

## 文档

### 核心文档

- [文档中心](docs/README.md) - 完整的文档索引
- [EventBus 文档](sdk/pkg/eventbus/README.md) - 事件总线使用指南
- [Outbox 模式快速开始](docs/outbox-pattern-quick-start.md) ⭐ - 5 分钟快速上手
- [Outbox 模式完整设计](docs/outbox-pattern-design.md) - 完整的架构设计和使用指南
- [可靠消费内核 (reliable)](sdk/pkg/reliable/README.md) ⭐ - 五状态机 + 死信 + 重投调度 + 双方言 Store

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

| 类型       | 说明             | 示例                        |
| -------- | -------------- | ------------------------- |
| `host`   | 从 Host 头识别     | `123.example.com` 或精确域名匹配 |
| `header` | 从自定义 Header 识别 | `X-Tenant-ID: 123`        |
| `query`  | 从 URL 参数识别     | `?tenant=123`             |
| `path`   | 从 URL 路径识别     | `/tenant-123/users`       |

**域名识别（host 类型）支持三种互斥模式**：

通过配置 `hostMode`（YAML）或 `httpHostMode`（ETCD）设置，三种模式**互斥**，不可同时使用：

| 模式        | 说明        | 匹配规则                         | 示例                                                   |
| --------- | --------- | ---------------------------- | ---------------------------------------------------- |
| `numeric` | 数字子域名（默认） | 从子域名提取数字作为租户 ID              | `123.example.com` → `123`                            |
| `domain`  | 精确域名匹配    | 通过 `DomainLookuper` 查询完整域名   | `tenant1.example.com` → 查域名配置 → 租户 ID                |
| `code`    | 租户代码匹配    | 通过 `CodeLookuper` 用子域名匹配租户代码 | `acmecorp.example.com` → 查 `code="acmecorp"` → 租户 ID |

**配置方式**：

```yaml
# YAML 配置方式
tenants:
  resolver:
    http:
      type: host
      hostMode: numeric  # numeric/domain/code，默认 numeric
```

```go
// ========== Host 类型租户识别 - 三种互斥模式 ==========

// 模式1：numeric（默认）- 仅数字子域名（无 Provider）
// - 123.example.com ✅ → 租户 ID 123
// - 456.app.example.com ✅ → 租户 ID 456
// - acmecorp.example.com ❌ → 非数字，识别失败
router.Use(ExtractTenantID(WithResolverType("host")))

// 推荐方式：通过 Provider 自动从 ETCD 读取全部租户识别配置
// 支持四种 httpType：host/header/query/path（由 ETCD httpType 配置决定）
// - host 模式支持三种子模式：numeric/domain/code（由 ETCD httpHostMode 配置决定）
router.Use(ExtractTenantID(
    WithProviderConfig(provider),
))

// ========== 其他租户识别方式（无 Provider） ==========

// Header 识别
router.Use(ExtractTenantID(
    WithResolverType("header"),
    WithHeaderName("X-Tenant-ID"),
))

// Query 参数识别
router.Use(ExtractTenantID(
    WithResolverType("query"),
    WithQueryParam("tenant"),
))

// URL 路径识别
// 例如: /123/users -> 提取第0段 "123" 作为租户 ID
router.Use(ExtractTenantID(
    WithResolverType("path"),
    WithPathIndex(0),
))
```

**重要说明**：

1. **互斥性**：三种模式只能选择一种，不会回退到其他模式
2. **默认值**：`hostMode` 默认为 `numeric`，保持向后兼容
3. **大小写**：`code` 模式下，子域名会转为小写后匹配（DNS 大小写不敏感）
4. **配置来源**：hostMode 优先从 Provider 的 `GetResolverConfig()` 获取，支持 ETCD 动态配置

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

// 获取租户某服务的数据库配置（serviceCode 如 "security-management"）
dbConfig, ok := p.GetServiceDatabaseConfig(tenantID, "security-management")
```

### 性能测试覆盖

- ✅ 高吞吐量场景（1900+ msg/s）
- ✅ 低延迟处理（0.5ms）
- ✅ 消息顺序保证
- ✅ 故障隔离与恢复
- ✅ 协程泄漏检测
- ✅ 内存使用监控

## 版本历史

- v1.7.4 - PR-2 §8.5 上层包补完（adapters/eventbus + opsvc；additive，向后兼容）。【adapters/eventbus】新增 `sdk/pkg/reliable/adapters/eventbus`（包名 `eventbusdlq`）：把 `EventBusDLQAdapter` 提升进 core，源自 file-storage-service 的加固副本（1 MiB 隔离载荷上限 + P1 retryable-refusal 修复——拒绝把 RETRYABLE/`ErrRetryLater` 原因终端化为 DEAD_LETTER，fail-closed）。`TenantStoreResolver` 接口定义在 `sdk/pkg/reliable/store`（非 adapters/eventbus），使 opsvc 复用它时不传递性引入 sarama（保 J2）。`NewEventBusDLQAdapter` 注入极简 `LogSink`（nil→noop，core 不依赖全局 logger，避免 DLQ 失败路径在未初始化全局 logger 的服务里 panic）。reliable 根新增无依赖 `SanitizeForLog`/`SanitizeForStorage`（线性 service 正则，ReDoS 安全，供 adapter 隔离路径脱敏）；**不改动** `gormshared.sanitizeMsg`——内核委托 + Unix 路径正则线性化另立独立 PR（避免本版 fingerprint 路径行为变更）。【opsvc】新增 `sdk/pkg/reliable/opsvc`：§10 运维 service 层 + DTO（List/GetDetail/ReplayOne/BatchReplay/Discard/Stats/QuarantineList/QuarantineDetail/QuarantineResolve/Anomalies），复用 `store.TenantStoreResolver`（每租户独立库）。决策：① §6.2.1 人工重放 409——`ReplayOne` 在 `store.HasEarlierUnsolvedSibling` 为真时返回 `ConflictError`（镜像 `EligibleHeadsSQL` 的 NOT-EXISTS），检查与 `ScheduleReplay` 同事务（TOCTOU 最小化），`BatchReplay` 逐行；② 访问审计——`NewService` 必传非空 `AccessAuditor`，includePayload/includeRaw 为真时先审计后释放（fail-closed），调用者身份由服务侧 PR-7 填充（core 不从 ctx 读身份，M14）；③ Stats 用 `store.Count`（F6，非 list-then-count）。【store 新增】`ListAnomalies` + `Count` + `HasEarlierUnsolvedSibling`（+ `AnomalyFilter`/`AnomalyRow`/`CountFilter`），GormStore 实现于 gormshared（anomaly.go + replay.go）。【M10 去重】evidence-management + file-storage-service 采纳 core 符号、删除各自本地副本（grep 0 本地定义）；evidence-management 的 189 行旧副本缺 P1 修复 + 载荷上限，采纳 core 后一并获得；file-storage 为纯去重（其本地副本即加固源）。【未建】batch/（M11 HandleBatch 装饰器）延后 YAGNI（无生产者发逐 item 子事件信封；触发：有生产者发 `(source_event_id, item_key)` 逐 item 子信封）；adapters/outbox/ 不建（§8.3 J3：outbox DLQHandler 实现是服务基础设施决策，PR-6 已交付各服务 handler）。【破坏性】无；纯增量、向后兼容。J2 门禁 GREEN（reliable 根零依赖，subpackage 引 eventbus 合法），deps gate + repotest 一致性套件（212s）全绿。仅 evidence-management + file-storage-service 升级（二者为 `EventBusDLQAdapter` 消费方）；security-management/process-management/tenant-service 不导入 `sdk/pkg/reliable`，无需升级。
- v1.7.3 - reliable `event_consumption` 唯一约束改名根治（`uk_event_handler` → `uk_event_consumption`），消除 shared library 在消费方 public schema 用通用名导致的跨方案权名冲突。【缺陷】reliable 包把 `event_consumption`（及 `raw_message_quarantine` 等）直接建在消费方的 public schema，其约束名 `uk_event_handler` 与消费方既有同名对象相撞——PG 索引/约束名共享同一 schema 命名空间，`event_processing_records.uk_event_handler`（evidence-management 自 2026-01-01 `4c59591`）/`dead_letter_queue.uk_event_handler` 先由消费方迁移建好占用该名，`CREATE TABLE IF NOT EXISTS event_consumption (... CONSTRAINT uk_event_handler ...)` 在第 1 句即 42P07「relation already exists」失败，整条 13 句 reliable DDL 中止 → `event_consumption`/`raw_message_quarantine` 永远建不出来 → 查询端 `AssertQuarantineReady` fail-closed 把 canary `live_enabled` 置 false → MediaCreated 退回 legacy 分支（PR-4 M9/Task 6 已删 legacy Created arm）→ 全部 MediaCreated 进 DLQ、读模型永不投影（evidence-management 95/97 API 用例失败根因）。【引入版本】PR-4（evidence-management `77d2c29` 2026-08-02，首次在含 `uk_event_handler` 的查询库里强制建 `event_consumption`）起潜伏；累积状态卷下 `CREATE TABLE IF NOT EXISTS` 对已存在表 no-op，掩盖 3 天，pr6 全新卷（`down -v`）首次从零建表才踩响。【根治】给约束加表名命名空间 `uk_event_consumption`（PG `CONSTRAINT` / MySQL `UNIQUE KEY` / GORM `uniqueIndex` tag 三处声明式 + 1 处注释），与消费方通用名 `uk_event_handler` 永不再撞；store 逻辑用 GORM `OnConflict{DoNothing:true}`（按唯一索引推断、不点名约束）+ 列式 `Where(event_id,handler_id,item_key)`，改名零逻辑风险，repotest 一致性套件（250s）全绿。evidence-management 只拥有冲突一方、本地改名只能让路、消不掉这一类 bug，故必须在 jxt-core 根治。【破坏性】仅 schema 对象名（约束名）变更，无 Go 符号/API 变更；消费方已建好的 `event_consumption` 表不受影响（`CREATE TABLE IF NOT EXISTS` 对已存在表 no-op，旧约束名若已落库则保留原样、运行不依赖约束名），全新库直接建为 `uk_event_consumption`；无需消费方做 rename 迁移。消费方 evidence-management query/command `v1.7.2 → v1.7.3`
- v1.7.2 - PR-6 前置（outbox 共享凭据脱敏）+ 外围包测试/bug 健康修复（additive，无 eventbus/outbox 行为变更）。【outbox】新增共享 `SanitizeLastError`（先 2KB 截断后正则脱敏：DSN `:pass@`、`password=/passwd=/pwd=/token=/access_token=/refresh_token=/api_key=/apikey=/secret=/client_secret=`、`Bearer …`、裸 JWT `eyJ…`、单引号字面量；stdlib RE2 线性时间，无 ReDoS）+ 规范死信计数器名常量 `MetricOutboxDeadLetteredTotal = "outbox_dead_lettered_total"`（与 sdk/pkg/reliable 消费侧同名——§8.5 依赖矩阵：security/process 不依赖 reliable，故发布侧在 outbox 包再持一份）；四个发布服务（security/evidence/file-storage/process）统一引用、去除各服务 ~4 份拷贝；纯增量、无既有符号变更。【外围健康修复】全量 `go test` 扫出的既有 latent 缺陷（非回归，均与 eventbus/outbox 无关）：① `sdk/antd_api` `AddError` 误把方法值 `e.Error`（自定义 `Error(int,string,string)` 响应方法）当累积错误字段 `e.Errors`，`go vet` 以「%v 格式化未调用 func」封禁整包、无可用测试门禁——改正为字段并加 `TestAddError_ChainsBothErrors` 回归；② `sdk/api`+`sdk/antd_api` `resolve()` 的 `binding:"dive"`/`validate:"dive"` 递归修复：原 `reflect.ValueOf(ptr).Field(i)`（对指针 Value 非法）并把 `reflect.Value` 回传 `resolve()` 令 `.Elem()` 再 panic，任何 dive 字段即崩（功能完全未触达，零生产影响）；重写为指针安全解包 + 新 `resolveDive` 按类型递归（Ptr/Slice/Array→Struct→`reflect.New`），两份副本同修，antd_api 副本顺带删遗留 `fmt.Println` 调试输出；`TestResolve` 改传 `&d` 对齐生产 `Bind()` 指针契约（gin `ShouldBindWith` 要求指针）；③ `tools/database` `TestDBConfig_Init` 由占位串 `dsn0`+真 `mysql.Open`（DSN 解析即拒「missing the slash」）改为本仓已验证的 CGO-free modernc 内存 sqlite（`sqlite.Dialector{DriverName:"sqlite"}` + 唯一 DSN），免真实 DB/DSN。单模块门禁 GREEN；测试 35/35 broker-free 包全绿、`go build ./...` OK、outbox `-race` 干净。tag 最初打于 outbox 提交 `b722ee8`，后重指向 `80962fc`（outbox 前置 + 其健康修复同属一版）
- v1.7.1 - dispatch 静默丢消息根修复（未激活 topic 由 drain 改 hold + 停滞可观测性）——【行为变更，发布前必须完成 5 服务 consumed⊆activated 审计】Kafka 预订阅消费在 handler 未激活时由「跳过并提交」（drain，静默丢失）改为「背压 hold 至激活」（不读通道、不提交、未处理消息下个会话重投递 at-least-once），legacy ConsumeClaim 与 partition-pipeline consumeWithPipeline 双路径统一（共享 holdUntilActivated，D3' 去激活竞态重入 hold）；移除 3s 预热 sleep（既是竞态窗口又造成 consumerMu 锁 convoy）并随之移除导出方法 IsWarmupCompleted/GetWarmupInfo（无调用方，纯遥测）；新增导出方法 IsActiveTopic(topic) bool（仅 kafka 驱动 *kafkaEventBus，供服务侧启动时 consumed⊆activated 自检；鸭子类型断言 bus.(interface{ IsActiveTopic(topic string) bool })，NATS/Memory 驱动返回 ok=false）；新增内部 HoldBackoff 定时字段（默认 100ms，<=0 钳位，非用户配置——timing 安全不变量不进 PipelineUserConfig）；hold 期间接入停滞可观测性（进入一次性 Warn + monotonic stall-enter + consumption_partition_stalled_seconds 实时爬升、退出归零）。已知限制（GP1 实验回滚 2026-08-01）：legacy 串行路径 envelope 重试失败后的「越位提交」仍保留——后续成功消息的 MarkMessage 会借 sarama MarkOffset MAX 语义越过未处理的毒消息 offset（本 session 静默丢失该毒消息）；曾尝试改为终止 claim 防越位，但集成测试证明 sarama 单 claim 语义下终止 claim 会触发重投循环、阻断整分区正常消息（reliability TestKafkaFaultIsolationWithHighLoad 收到 31/1000 vs baseline 1008/1000），故回滚；正确解法在 pipeline 路径（DLQ + Strategy A poison stall），非 legacy 串行路径。⚠️ 语义变更：任何「故意预订阅但不激活」的 topic 由静默丢失变为永久停滞，发布前必须完成 plan Task 1 的 5 服务 consumed⊆activated 审计（evidence-management command/query、security-management、file-storage-service、tenant-service）；回滚只能降模块版本（无 flag 门禁）。附带清理：legacy ConsumeClaim 死 if true/else drain 分支删除（行为等价）
- v1.7.0 - 版本治理 release（无功能变更，代码内容同 v1.1.71——master 自 v1.1.71 起无功能提交）：本仓库自 go-admin-core fork 继承了一批错误 tag——其根 `go.mod` 声明的是 `matchstalk/utils`、`matchstalk/go-admin-core`、`go-admin-team/go-admin-core`（另含 `sdk/`、`plugins/logger/zap/` 子模块 tag），既非 `github.com/ChenBigdata421/jxt-core` 的合法发布，版本号（v0.1、v1.0.2、v1.2.0–v1.6.5）又高于正规线且 module path 不匹配，导致未 pin 的 `go mod tidy` 误选坏 tag（如 `sdk@v1.5.2`）而硬失败。本 release 删除全部错误 tag（根 `v*` + `sdk/*` + `plugins/*`），`sdk/` 由父模块统一提供、不再作为独立子模块发布；版本号从 v1.1.71 跨过作废的 v1.2.x–v1.6.x 跳至 v1.7.0，以干净 head 使 `@latest` 正确解析为 v1.7.0。**当前有效最新版本 = v1.7.0**，v1.2.x–v1.6.x 版本号作废、永不复用。消费端 `go get github.com/ChenBigdata421/jxt-core@v1.7.0`（从 v1.1.71 升级）；go.mod 中旧的 `exclude github.com/ChenBigdata421/jxt-core/sdk ...` 行可移除
- v1.1.71 - 修复 v1.1.69「主题拓扑收敛 改动5」AUTO_CREATE_TOPICS gate（`kafka.go:3415`）引入的 CI 测试回归（仅测试修复，无库行为变更）：生产路径不再自动建 topic 后，`tests/eventbus/topic_name_validation_test.go` 两个既有 Kafka 子测试 `TestConfigureTopic_ValidationIntegration`、`TestSetTopicPersistence_ValidationIntegration`（/Kafka/ValidTopicName 用例）对不存在的 topic 名调用 `ConfigureTopic`/`SetTopicPersistence` 并期望成功，自 `3e39dd8`（v1.1.69）起在 CI 失败；本地此前因 RedPanda 持久卷跨次运行残留旧 topic 而误绿（CI 每次全新 VM + 卷）。修复：这两个 dev/test 上下文的校验测试 opt-in 设计好的开发逃逸口 `AUTO_CREATE_TOPICS=1`（仅 Kafka 子测试读取；Memory/NATS 不受影响）。已在干净（`docker compose down -v`）broker 上复现 red→green，eventbus/function 全套无回归。生产路径 AUTO_CREATE_TOPICS gate 保持不变
- v1.1.70 - Kafka 消费流水线「死开关」修复 + 启动校验加固（默认关闭，对库为 no-op；v1.1.69 已被主题拓扑收敛方案占用，本组死开关修复顺延为 v1.1.70）：用户层 `ConsumerConfig` 此前无 `pipeline` mapstructure 目标字段，YAML `consumer.pipeline.enabled` 被裸 `v.Unmarshal`（`config.go:56`，无 `ErrorUnused`）静默丢弃 → 除默认关闭外无任何生产路径能开启分区内消费流水线。修复：新增用户层 `PipelineUserConfig{Enabled,WindowSize}` + `convertUserConfigToInternalKafkaConfig` 贯通 + `NewKafkaEventBus` 在首次 `sarama.NewClient` 之前 fail-fast 校验（`FlushTimeout < sessionTimeout/2` 等不变量）+ 启动可观测日志（`Infof` 可 grep）；`windowSize` 加 ≤1024 上界（每分区在飞上限 × 分区数 = chan 缓冲，防误配 OOM）；timing 字段（flushTimeout/dlqTimeout/stallWarnInterval）留内部 `applyPipelineDefaults` 兜底，用户层不暴露。logger 包 `Logger`/`DefaultLogger` 改非空 `zap.NewNop()` 默认（`Setup` 前 nil 裸调用不再 panic）。新增死开关解码回归测试（viper YAML 往返）+ windowSize 上界测试 + logger 非空默认测试，并补进 `test-race.yml` CI。默认关闭 → 对库为 no-op；激活（Task 8 灰度）另有 4 项前置（见 `TODOS.md` F3）
- v1.1.69 - EventBus 主题拓扑收敛方案 改动5/6 收尾：jxt-core 退化为「不创建、不扩容、不断言分区数」——create_or_update 的 reconcile 收敛为仅配置（retention/compression），移除分区扩容（CreatePartitions）；compareTopicOptions 分区差异改为 informational 告警（CanAutoFix 恒 false）；新增 WaitForTopicsExist 启动期 metadata 只读存在性断言（与 WaitForTopologyReady 互补：全局就绪 vs 应用层自检，任一缺失即 fail-fast 指名）；TopicPartitionInfo 注释更新为仅存在性查询；分区正确性交由 infra bootstrap 双遍断言 + redpanda healthcheck (CHECK_ONLY) 独占收敛
- v1.1.68 - 新增可靠消费内核 (reliable)：五状态机（PROCESSING/SUCCEEDED/RETRY_SCHEDULED/DEAD_LETTER/DISCARDED）+ fencing token 幂等去重 + 死信 + 重投调度（一张 event_consumption 表三者合一，M1）；HandlerID/Key/Meta/ClaimInput/ClaimToken/Decision 身份契约；两阶段错误分类（ErrRetryLater vs 终态结算）；attempt/backoff oracle（指数退避 1s→24h，含属性测试）；Store 接口（TryClaim/MarkSucceeded/MarkFailed/AdvanceDue/MoveToDeadLetter）+ QuarantineStore（不可解码坏消息隔离）；双方言 Store（MySQL + PostgreSQL）via gormshared + repotest conformance 套件（含 §2.4 不变量 + quarantine + EXPLAIN gate）；eligible-head 重放调度器（ClaimForReplay + 三种 ReplaySafety）；aggregate gate 保序（非幂等 handler 串行执行）；租约孤儿观测 runner（ObserveExpiredLeases）；CI 门禁（J2 零第三方依赖/M14 显式 *gorm.DB/§3.3 TryClaim 独立 session/D9 claim_id 校验）；contextpool 测试编译修复
- v1.1.67 - 文档 release：补登 v1.1.60–v1.1.66 版本历史（根 README 版本历史自 v1.1.59 后断档 7 个版本未登记）；无代码变更；将 v1.1.60 标记为废弃（sumdb 锁定，被 v1.1.61 替代）
- v1.1.66 - PR-1 投递契约（delivery contract，破坏性变更）：EventBus 新增 `EnvelopeDelivery`/`RawMeta`/`MessageHeader` 契约 + `EnvelopeDeliveryOptionsSubscriber` 能力接口，Kafka `SubscribeEnvelopeDeliveryWithOptions` 填充 raw key/value、保序去重保留 headers、topic/partition/offset/timestamp、sha256 payload hash，actor pool 显式线程化 `Raw`+`DeliveryHandler`（无 context key，M14）；C6 `PoisonMessage.Headers` 由 `map[string]string` 改为 `[]MessageHeader`（保序+重复键）并加 `Timestamp`，Key/Value 防御性拷贝；C4 仅 `kafkaEventBus` 实现 delivery 订阅，memory/nats fail-fast；Outbox 发布侧死信终态（C1）：`EventStatusDeadLettered` 状态 + `dead_lettered_at`/`dlq_notified_at` 列 + 迁移 003，`OutboxRepository` 新增 `MarkAsDeadLettered`(CAS)/`FindUnnotifiedDeadLettered`/`MarkDeadLetterNotified`(CAS)，`processDLQ` 重写为 CAS 终态 + 通知拆分（crash 可下一轮补发、无孤儿中间态）；C2 Alert 不再被 Handle 失败吞；C3 删 `NoOpDLQHandler`、`EnableDLQ=false` 默认、`Validate` 拒绝 `EnableDLQ=true`+nil handler；reconnect 重建 consumer-group + restoreSubscriptions 闭包恢复；新增 Tier-1 系统测试（真 MySQL：DLQ + 幂等端到端）。破坏性：`OutboxRepository` 接口 +3 方法、`PoisonMessage.Headers` 类型变更，消费方需跟随
- v1.1.65 - EventBus 分区流水线 stall 加固：hoist stall-elapsed 局部变量、补 partition-stall metric 测试缺口；新增 partition-stall 注入 seam（core 命名 + service 侧实现）；文档化 stall-counter 灵敏度权衡
- v1.1.64 - EventBus `ensureKafkaTopicIdempotent` 修复：检查 `metadata[0].Err`，防止 topic 不存在时被误判为已存在（与 `GetTopicPartitions` 同一 sarama DescribeTopics 坑；仍被 RedPanda auto-create 掩盖，naive 修复会破坏启动故保留）
- v1.1.63 - PR-2 后续加固：提交 `go.sum`（此前被 gitignore，全新 checkout 无法构建，2026-07-17 修复）；3 处并发修复——adapter tenant-loop spawn vs Close 的 WaitGroup panic、InProcess publisher close-vs-send 数据竞争、三驱动 RegisterTenant-vs-Close TOCTOU；ACK-drop 日志对齐（kafka/nats）；新增 broker-free `-race` 并发测试（kafka/nats/memory byte-parallel）+ 2 个 CI workflow（test-race、test-regression via docker-compose-nats）
- v1.1.62 - PR-2 Outbox ACK 生命周期契约：无损 ACK 准入（admission）+ 稳定终态错误 Close + Kafka producerResultWg（sender WG）+ adapter join + default-off/checked 构造器 + createdStreams 缓存修复
- v1.1.61 - jwtauth 新增 `GetPoliceName` getter（读 policename claim）、弃用 dormant `GetOrgId`（错误 key 'orgid'，0 调用方，v1.3 移除）；作为 v1.1.60 的干净替换发布（2026-07-12）
- v1.1.60 - ⚠️ 废弃：tag 内容含 `GetPoliceName`+弃用 `GetOrgId`，但 sumdb 锁定在 pre-GetPoliceName 内容，消费者拉取不到目标代码，不可用，已被 v1.1.61 替代，请勿引用
- v1.1.59 - Outbox 异步 ACK 批量化（ackMarkerBatcher）：成功 ACK 攒满 50 条或每 200ms flush 一次 MarkBatchAsPublished，解开生产端 commit-bound；配套加固——flushFunc panic recover、防 onError 自死锁（异步+recover）、监听器 Done-ch 快照防竞争、连续失败计数节流、关停冲刷剩余 + 回归测试
- v1.1.58 - Outbox v2.0.0：SyncSemanticsPublisher 同步/异步标记分流；FindPublishedByIdempotencyKeys 批量幂等检查；BatchUpdate 更名为 MarkBatchAsPublished（单条 UPDATE 状态迁移，幂等 WHERE status='pending'）；filterPublishedEvents 批量化（破坏性接口变更）
- v1.1.57 - EventBus 新增 TopicPartitionInfo 可选接口（分区查询）；修复 create_or_update 未扩已有 topic 分区
- v1.1.56 - EventBus Kafka 消费分区流水线优化（partitionPipeline，feature flag 默认关闭）：advanceFrontier 连续前缀提交 + 异步 DLQ + Strategy A poison stall + 同聚合顺序保证；P-1 正确性门全绿，P-2 灰度前置待定
- v1.1.55 - README 刷新对齐代码、移除 go-admin 残留引用、Go toolchain 升级到 1.26.0、示例 main 加 //go:build ignore
- v1.1.53 - Casbin 精简（破坏性变更）：移除 SetupForTenant/Setup、去除 gorm-adapter 依赖（SetupForTenant 下沉至 security-management 自建本地 GormAdapter）；导出 watcher 初始化与 model 文本；新增 Linux-CI Sentinel 故障转移集成测试
- v1.1.52 - Redis Sentinel 支持：新增 Sentinel 字段与 newClient、合并 QueueRedis/QueueNSQ；按 DB 幂等客户端（5 个 Ensure*）；订阅者 ping 收敛到写锁、移除无用的 NSQ squash；同步 casbin/redis README
- v1.1.51 - Redis 基础设施重构：升级 go-redis 到稳定 v9.x（移除 redisqueue）、落地三客户端架构（共享/队列消费者/订阅者）；新增后台健康检查 IsRedisHealthy()、context-aware AdapterCache 接口、优雅 Close()；Casbin 策略同步改为单连接 PSUBSCRIBE 多路复用 + 8 分片有序派发；修复缓存/队列多处数据竞争
- v1.1.50 - StorageConfig 新增 PublicUrl 字段，支持外部代理访问地址
- v1.1.49 - 新增 WVP（视频平台）配置缓存：Provider 支持 ConfigTypeWvp / GetWvpConfig / WVP watch 事件；新增 tenant/wvp 缓存包与 TenantWvpConfig 模型；配套单元测试与设计文档
- v1.1.48 - 多租户默认配置（DefaultTenantConfig）新增 TenantWvpDetailConfig，支持 settings.yml 初始化 WVP 配置
- v1.1.47 - Outbox 新增 InProcessEventPublisher，支持进程内事件分发
- v1.1.46 - Provider 新增 GetAllTenantIDs 方法；jwtauth 将 Dept 字段重命名为 Org（语义清晰）
- v1.1.45 - 新增租户识别配置缓存，支持基于配置缓存进行租户识别
- v1.1.41 - 新增域名查找（DomainLookuper）支持租户 ID 解析
- v1.1.40 - 增强租户 ID 解析能力
- v1.1.39 - 新增域名查找支持
- v1.1.38 - 新增统一迁移框架支持多租户
- v1.1.36 - 增加 *.prof 到 gitignore
- v1.1.35 - Casbin 缓存优化，通过 gRPC 获取策略
- v1.1.31 - 支持每个租户多个 FTP 配置
- v1.1.30 - 支持租户服务数据库配置
- v1.1.29 - 新增多租户组件
- v1.1.28 - 重构 tenants 结构，增加缺省租户
- v1.1.27 - ETCD 增加租户配置
- v1.1.26 - 增加 FTP 配置、存储站点配置
- v1.1.25 - Worker pool 迁移到 Hollywood Actor Pool
- v1.1.20 - 新增 Outbox 组件
- v1.1.19 - 新增 EventBus 组件
- v1.1.18 - 完善 GRPC 配置
- v1.1.16 - 多租户支持，重构 casbin 和 crontab
- v1.1.11 - 增加多租户配置
- v1.1.0 - 移除本地 SDK 模块 replace 指令
- v1.0.0 - 初始版本，提供基础框架功能
