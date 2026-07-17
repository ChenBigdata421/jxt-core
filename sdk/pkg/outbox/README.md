# Outbox Pattern 通用框架

## 📋 概述

Outbox 模式是一种用于确保分布式系统中数据一致性的设计模式。本框架提供了一个通用的、可复用的 Outbox 实现，可以被所有微服务使用。

## 🎯 核心特性

- ✅ **数据库无关**：领域模型完全独立于数据库实现
- ✅ **依赖注入**：通过 EventPublisher 接口实现依赖倒置
- ✅ **零外部依赖**：核心包不依赖任何外部库
- ✅ **多租户支持**：内置租户隔离
- ✅ **同步 & 异步双模式**：进程内派发（InProcess）走同步标记；跨服务（Kafka/NATS）走 ACK 监听器
- ✅ **批量幂等检查**：`FindPublishedByIdempotencyKeys` — 一次 `IN` 查询替代 N 次逐条 SELECT
- ✅ **批量标记已发布**：`MarkBatchAsPublished` — 一次 `UPDATE WHERE id IN (…) AND status='pending'` 替代 N 次操作
- ✅ **异步 ACK 标记批量化**：`ackMarkerBatcher` 把异步 ACK 的逐条 `MarkAsPublished` 攒成批量 `MarkBatchAsPublished`（默认满 50 条或每 200ms 一次），commit 数降 ~12×
- ✅ **自动重试**：支持失败事件自动重试
- ✅ **定时发布**：支持延迟发布
- ✅ **自动清理**：自动清理已发布的事件
- ✅ **健康检查**：内置健康检查机制
- ✅ **指标收集**：支持指标收集和监控

## 🏗️ 架构设计

### 核心组件

```
┌─────────────────────────────────────────────────────────┐
│              jxt-core/sdk/pkg/outbox/                   │
├─────────────────────────────────────────────────────────┤
│  ✅ 1. OutboxEvent（领域模型）                           │
│  ✅ 2. OutboxRepository（仓储接口）                      │
│  ✅ 3. EventPublisher（事件发布接口）⭐                  │
│  ✅ 4. SyncSemanticsPublisher（同步语义标记接口）⭐     │
│  ✅ 5. OutboxPublisher（发布器）                         │
│  ✅ 6. OutboxScheduler（调度器）                         │
│  ✅ 7. TopicMapper（Topic 映射）                         │
│                                                         │
│  ✅ 8. GORM 适配器（adapters/gorm/）                    │
│     - OutboxEventModel（数据库模型）                    │
│     - GormOutboxRepository（仓储实现）                  │
└─────────────────────────────────────────────────────────┘
```

### 依赖注入设计

```
业务微服务
    │
    ├─ 创建 EventBus（jxt-core）
    │
    ├─ 创建 EventBusAdapter（实现 EventPublisher 接口）⭐
    │
    └─ 注入到 OutboxScheduler
           │
           └─ OutboxPublisher（依赖 EventPublisher 接口）
```

## ⚡ 同步 vs 异步 Publisher

Outbox 发布分两条路径，由 `SyncSemanticsPublisher` 标记区分。**这是集成时必须做的第一个决策。**

### 同步 Publisher（InProcess / 进程内派发）

适用于事件不跨服务的场景（如 IAM 设备管理、内部审计日志）。事件在进程内同步派发给注册的 handler，`PublishEnvelope` 返回 nil 即表示发布成功。

```
OutboxScheduler
  └─ poll tick
       └─ PublishBatch
            ├─ filterPublishedEvents（1 次批量幂等查询）
            ├─ InProcessEventPublisher.PublishEnvelope（同步跑 handler）
            └─ MarkBatchAsPublished（同步标 published，1 次 UPDATE）
```

**特点**：
- 无需 broker（Kafka/NATS）
- `MarkBatchAsPublished` 在 `PublishEnvelope` 成功后立即执行
- 发布 + 标记全同步，无需额外 goroutine
- **不需要** `StartACKListener`

### 异步 Publisher（Kafka / NATS）

适用于事件需要跨服务消费的场景（如 evidence-management 的 media/archive 事件）。`PublishEnvelope` 返回 nil 只表示提交到 broker，不代表 ACK。

```
OutboxScheduler
  └─ poll tick
       └─ PublishBatch
            ├─ filterPublishedEvents（1 次批量幂等查询）
            └─ EventBusAdapter.PublishEnvelope（提交到 broker，立即返回）

ASYNC（独立 goroutine）：
  ACK listener → broker ACK 回来 → ackMarkerBatcher 攒批 → MarkBatchAsPublished
    （DefaultPublisherConfig 默认 ACKBatchSize=0 即攒批关闭；显式设 ACKBatchSize（如 50）后，满批或每 ACKBatchFlushInterval=200ms 一次 bulk UPDATE，
      而非逐条 MarkAsPublished；优雅关停 StopACKListener 会冲刷剩余缓冲）
```

**特点**：
- 依赖 broker（Kafka/RedPanda 或 NATS JetStream）
- 标记清理由 ACK listener 异步处理；默认经 `ackMarkerBatcher` **攒批**标记（`MarkBatchAsPublished`），而非逐条
- **必须** 启动 `StartACKListener`（由业务服务 wiring 层负责）
- `SyncSemanticsPublisher` 标记未实现 → `PublishBatch` 不会走同步标记路径
- ⚠️ **硬崩溃窗口**：非优雅退出会丢失 `ackMarkerBatcher` 内存缓冲（约 ≤`ACKBatchSize` 条）→ 这些事件留 Pending → 下个 tick 重发 → **消费端 handler 必须幂等**（at-least-once 固有重复风险的放大）。优雅关停（`StopACKListener`）会冲刷剩余，不丢。

### 性能特性

同步、异步两条路径都已批量化（`BatchSize=100` 下，幂等检查与标记各从 N 次合成 1 次）：

| 操作 | 同步 publisher（InProcess） | 异步 publisher（Kafka/NATS） |
|---|---|---|
| 幂等检查 `filterPublishedEvents` | `FindPublishedByIdempotencyKeys` — `WHERE idempotency_key IN (…)` → **1 次 SELECT** | 同左 |
| 标记 published | `PublishBatch` 内同步 `MarkBatchAsPublished`（1 次 `UPDATE … WHERE id IN (…) AND status='pending'`） | ACK listener 经 `ackMarkerBatcher` 攒批 → `MarkBatchAsPublished`（`ACKBatchSize` 默认 0 即关闭；显式设 >0 后满批或每 `ACKBatchFlushInterval=200ms` 一次 bulk UPDATE） |

> 异步路径的 ACK 攒批是 v1.1.59 引入：之前每收到 1 个 broker ACK 跑 1 条 `MarkAsPublished`（= 1 次事务/commit），现攥批后 commit 数降 ~12×（实测 run J：95,866 → 8,026）。

## 🚀 快速开始

### 步骤 1：定义 TopicMapper

```go
// internal/outbox/topics.go
package outbox

import (
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

func NewTopicMapper() outbox.TopicMapper {
    return outbox.NewMapBasedTopicMapper(map[string]string{
        "Archive": "archive-events",
        "Media":   "media-events",
    }, "default-events")
}
```

### 步骤 2：选择 EventPublisher

jxt-core 提供两种 EventPublisher：

#### 2a. Kafka / NATS Publisher（异步，跨服务）

使用 jxt-core 内置的 `EventBusAdapter`，无需自己编写：

```go
import (
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    outboxadapters "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
)

// 从框架获取 EventBus 并直接包一层
eventBus := sdk.Runtime.GetEventBus()
eventPublisher := outboxadapters.NewEventBusAdapter(eventBus)
```

`EventBusAdapter` 已实现完整的 `EventPublisher` 接口（`Publish` / `PublishEnvelope` / `GetPublishResultChannel`），并支持多租户 ACK 路由。

#### 2b. InProcess Publisher（同步，进程内派发）

```go
import (
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    outboxadapters "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
)

// InProcessEventPublisher 实现 SyncSemanticsPublisher —
// PublishEnvelope 同步返回，发布后自动标记 Published。
eventPublisher := outboxadapters.NewInProcessEventPublisher()
```

选择对照表：

| 特点 | Kafka/NATS | InProcess |
|---|---|---|
| 跨服务消费 | ✅ | ❌ |
| 需要 broker 基础设施 | ✅ | ❌ |
| 需要 StartACKListener | ✅ 必须 | ❌ 不推荐 |
| 标记策略 | ACK listener 异步标记 | PublishBatch 同步标记 |
| 实现接口 | `EventPublisher` | `SyncSemanticsPublisher`（含 marker） |


### 步骤 3：初始化 Outbox

```go
// cmd/main.go
package main

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
    localoutbox "your-service/internal/outbox"
)

func main() {
    ctx := context.Background()
    
    // 1. 获取数据库
    db := sdk.Runtime.GetDB()
    
    // 2. 自动迁移（创建表）
    db.AutoMigrate(&gormadapter.OutboxEventModel{})
    
    // 3. 创建仓储
    outboxRepo := gormadapter.NewGormOutboxRepository(db)
    
    // 4. 创建 EventBus 适配器 ⭐
    eventBus := sdk.Runtime.GetEventBus()
    eventPublisher := localoutbox.NewEventBusAdapter(eventBus)
    
    // 5. 创建并启动调度器
    scheduler := outbox.NewScheduler(
        outbox.WithRepository(outboxRepo),
        outbox.WithEventPublisher(eventPublisher),  // 注入 EventPublisher ⭐
        outbox.WithTopicMapper(localoutbox.NewTopicMapper()),
    )
    
    // 6. 启动调度器
    if err := scheduler.Start(ctx); err != nil {
        panic(err)
    }
    defer scheduler.Stop(ctx)

    // 7. 【异步 publisher 必须】启动 ACK listener
    // 如果 EventPublisher 是 Kafka/NATS（未实现 SyncSemanticsPublisher），
    // 必须启动 ACK listener 异步标记事件为 Published。
    //
    // 如果 InProcess（实现 SyncSemanticsPublisher），跳过此步骤：
    // PublishBatch 在发布成功后同步调 MarkBatchAsPublished。
    //
    // 详情见 "⚡ 同步 vs 异步 Publisher" 节。
    if _, isSync := eventPublisher.(outbox.SyncSemanticsPublisher); !isSync {
        ackHost := outbox.NewOutboxPublisher(
            outboxRepo, eventPublisher, localoutbox.NewTopicMapper(), nil)
        ackHost.StartACKListener(ctx)
        defer ackHost.StopACKListener()
    }

    // 8. 启动应用...
}
```

### 步骤 4：业务代码使用

#### 方式 1：使用 DomainEvent（推荐）

```go
// internal/service/archive_service.go
package service

import (
    "context"
    jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

type ArchiveService struct {
    db          *gorm.DB
    outboxRepo  outbox.OutboxRepository
}

func (s *ArchiveService) CreateArchive(ctx context.Context, archive *Archive) error {
    // 开始事务
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 1. 保存业务数据
        if err := tx.Create(archive).Error; err != nil {
            return err
        }

        // 2. 创建 DomainEvent（推荐使用 EnterpriseDomainEvent）
        domainEvent := jxtevent.NewEnterpriseDomainEvent(
            "Archive.Created",
            archive.ID,
            "Archive",
            ArchiveCreatedPayload{
                ArchiveID: archive.ID,
                Title:     archive.Title,
                CreatedBy: archive.CreatedBy,
            },
        )
        domainEvent.SetTenantId(archive.TenantID)

        // 3. 序列化 DomainEvent
        eventBytes, err := jxtevent.MarshalDomainEvent(domainEvent)
        if err != nil {
            return err
        }

        // 4. 创建 Outbox 事件
        outboxEvent, err := outbox.NewOutboxEvent(
            archive.TenantID,
            archive.ID,
            "Archive",
            "Archive.Created",
            eventBytes,  // 传入序列化后的 DomainEvent
        )
        if err != nil {
            return err
        }

        // 5. 在同一事务中保存 Outbox 事件
        if err := s.outboxRepo.Save(ctx, outboxEvent); err != nil {
            return err
        }

        return nil
    })
}
```

#### 方式 2：直接使用 Payload（简单场景）

```go
func (s *ArchiveService) CreateArchive(ctx context.Context, archive *Archive) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 1. 保存业务数据
        if err := tx.Create(archive).Error; err != nil {
            return err
        }

        // 2. 创建 Outbox 事件（直接传入 Payload）
        event, err := outbox.NewOutboxEvent(
            archive.TenantID,
            archive.ID,
            "Archive",
            "ArchiveCreated",
            archive,  // 直接传入业务对象
        )
        if err != nil {
            return err
        }

        // 3. 在同一事务中保存 Outbox 事件
        if err := s.outboxRepo.Save(ctx, event); err != nil {
            return err
        }

        return nil
    })
}
```

**推荐使用方式 1**，因为：
- ✅ 使用标准的 DomainEvent 结构
- ✅ 包含完整的事件元数据（EventID、OccurredAt、Version 等）
- ✅ 支持企业级字段（TenantId、CorrelationId、TraceId 等）
- ✅ 便于事件溯源和审计
- ✅ 与 Query Side 的事件处理保持一致

## 🔄 与 DomainEvent 集成

Outbox 组件与 `domain/event` 组件深度集成，支持标准的 DomainEvent 序列化：

### Command Side（发布端）

```go
import jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"

// 1. 创建 EnterpriseDomainEvent
domainEvent := jxtevent.NewEnterpriseDomainEvent(
    "Archive.Created",
    archiveID,
    "Archive",
    payload,
)
domainEvent.SetTenantId(tenantID)

// 2. 序列化 DomainEvent
eventBytes, err := jxtevent.MarshalDomainEvent(domainEvent)

// 3. 保存到 Outbox
outboxEvent, err := outbox.NewOutboxEvent(
    tenantID,
    archiveID,
    "Archive",
    "Archive.Created",
    eventBytes,  // 传入序列化后的字节数组
)
```

### Query Side（订阅端）

```go
// 1. 从消息队列接收 Envelope
envelope, err := eventbus.FromBytes(msg.Data)

// 2. 反序列化 DomainEvent
domainEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](envelope.Payload)

// 3. 提取 Payload
payload, err := jxtevent.UnmarshalPayload[ArchiveCreatedPayload](domainEvent)
```

### 性能特性

- ✅ **高性能序列化**: 使用 jxtjson（基于 jsoniter），比标准库快 2-3 倍
- ✅ **序列化性能**: ~690ns/op
- ✅ **反序列化性能**: ~1.2μs/op
- ✅ **并发安全**: 支持 100+ goroutines 并发序列化

详细信息请参考：[DomainEvent 序列化指南](../domain/event/SERIALIZATION_GUIDE.md)

## 📚 详细文档

### 核心文档
- [完整设计方案](../../../docs/outbox-pattern-design.md)
- [快速开始指南](../../../docs/outbox-pattern-quick-start.md)
- [依赖注入设计](../../../docs/outbox-pattern-dependency-injection-design.md)

### 集成文档
- [DomainEvent 序列化指南](../domain/event/SERIALIZATION_GUIDE.md)
- [DomainEvent 实现总结](../domain/event/IMPLEMENTATION_SUMMARY.md)
- [统一 JSON 迁移](../../UNIFIED_JSON_MIGRATION.md)

## 🎁 核心优势

### 1. 符合 SOLID 原则

- ✅ **依赖倒置原则（DIP）**：Outbox 依赖 EventPublisher 接口，不依赖具体实现
- ✅ **开闭原则（OCP）**：可以扩展新的 EventPublisher 实现，无需修改 Outbox 代码
- ✅ **单一职责原则（SRP）**：每个组件职责明确

### 2. 零外部依赖

```go
// jxt-core/sdk/pkg/outbox/ 的依赖
import (
    "context"
    "time"
    "encoding/json"
    // 没有任何外部包依赖！
)
```

### 3. 易于测试

```go
// 可以轻松 mock EventPublisher
type MockEventPublisher struct {
    publishedEvents []PublishedEvent
}

func (m *MockEventPublisher) Publish(ctx context.Context, topic string, data []byte) error {
    m.publishedEvents = append(m.publishedEvents, PublishedEvent{topic, data})
    return nil
}
```

### 4. 灵活扩展

- ✅ 支持任何 EventBus 实现（Kafka/NATS/Memory/自定义）
- ✅ 支持任何数据库（GORM/MongoDB/自定义）
- ✅ 支持自定义 TopicMapper

## 📊 组件职责

| 组件 | 位置 | 业务微服务需要做什么 |
|------|------|---------------------|
| **OutboxEvent** | jxt-core | ❌ 无需实现 |
| **OutboxRepository** | jxt-core | ❌ 无需实现 |
| **EventPublisher 接口** | jxt-core | ❌ 无需实现（只是接口定义）⭐ |
| **SyncSemanticsPublisher** | jxt-core | ❌ 无需实现（InProcess 已内置 marker）⭐ |
| **OutboxPublisher** | jxt-core | ❌ 无需实现 |
| **OutboxScheduler** | jxt-core | ❌ 无需实现 |
| **GORM 适配器** | jxt-core | ❌ 无需实现（可选使用） |
| **TopicMapper** | 业务微服务 | ✅ **需要实现**（定义映射） |
| **EventBusAdapter** | jxt-core | ❌ 开箱即用（`outbox/adapters/`） |
| **配置** | 业务微服务 | ✅ **可选**（有默认值） |
| **启动调度器** | 业务微服务 | ✅ **需要启动**（一行代码） |
| **启动 ACK listener** | 业务微服务 | ✅ **仅异步 publisher 需要** ⭐ |

## 🔧 配置选项

### 调度器配置

```go
config := &outbox.SchedulerConfig{
    PollInterval:        10 * time.Second,  // 轮询间隔
    BatchSize:           100,                // 批量大小
    TenantID:            "",                 // 租户 ID（可选）
    CleanupInterval:     1 * time.Hour,      // 清理间隔
    CleanupRetention:    24 * time.Hour,     // 清理保留时间
    HealthCheckInterval: 30 * time.Second,   // 健康检查间隔
    EnableHealthCheck:   true,               // 启用健康检查
    EnableCleanup:       true,               // 启用自动清理
    EnableMetrics:       true,               // 启用指标收集
}

scheduler := outbox.NewScheduler(
    outbox.WithRepository(repo),
    outbox.WithEventPublisher(eventPublisher),
    outbox.WithTopicMapper(topicMapper),
    outbox.WithSchedulerConfig(config),
)
```

### 发布器配置

```go
config := &outbox.PublisherConfig{
    MaxRetries:     3,                    // 最大重试次数
    RetryDelay:     time.Second,          // 重试延迟
    PublishTimeout: 30 * time.Second,     // 发布超时
    EnableMetrics:  true,                 // 启用指标收集
    ErrorHandler: func(event *outbox.OutboxEvent, err error) {
        // 自定义错误处理
    },

    // —— 异步 ACK 标记攒批（仅异步 publisher 的 ACK listener 路径生效；v1.1.59+）——
    ACKBatchSize:             50,                       // 0 = 禁用攥批（回退逐条 MarkAsPublished）；DefaultPublisherConfig 默认 0（关闭/fail-safe）；此处显式设 50 启用；max 10000
    ACKBatchFlushInterval:    200 * time.Millisecond,   // 攒批 flush 间隔；默认 200ms；max 5min
    ACKBatchFailureThreshold: 5,                        // 连续 flush 失败的告警阈值（达阈值及倍数经 ErrorHandler 告警）；0 = 每次失败都告警；默认 5
}

scheduler := outbox.NewScheduler(
    outbox.WithRepository(repo),
    outbox.WithEventPublisher(eventPublisher),
    outbox.WithTopicMapper(topicMapper),
    outbox.WithPublisherConfig(config),
)
```

> `DefaultPublisherConfig()` 默认 `ACKBatchSize=0`（攥批**关闭**，fail-safe：硬崩溃不丢内存缓冲，逐条 `MarkAsPublished`）。需要攒批时显式设置 `ACKBatchSize`（如 50）；`ACKBatchSize=0` 回退 v2.0.0 的逐条 `MarkAsPublished` 行为。⚠️ 攒批开启时，硬崩溃会丢失内存缓冲（约 ≤`ACKBatchSize` 条）→ **消费端 handler 必须幂等**。

## 📈 监控指标

```go
// 获取调度器指标
metrics := scheduler.GetMetrics()
fmt.Printf("Poll Count: %d\n", metrics.PollCount)
fmt.Printf("Processed Count: %d\n", metrics.ProcessedCount)
fmt.Printf("Error Count: %d\n", metrics.ErrorCount)

// 获取发布器指标
publisherMetrics := publisher.GetMetrics()
fmt.Printf("Published Count: %d\n", publisherMetrics.PublishedCount)
fmt.Printf("Failed Count: %d\n", publisherMetrics.FailedCount)
```

## 📝 许可证

MIT License


