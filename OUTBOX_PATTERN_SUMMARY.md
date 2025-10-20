# Outbox 模式方案总结

## 📋 方案概述

本文档总结了 jxt-core Outbox 模式通用框架的完整设计方案。

---

## 🎯 核心决策

### 1. **Outbox 核心逻辑提炼到 jxt-core** ✅

**决策**：将 Outbox 模式的核心逻辑（模型、仓储接口、发布器、调度器）提炼到 jxt-core 作为通用框架。

**理由**：
- ✅ Outbox 模式是**业务无关**的通用模式
- ✅ 所有微服务都需要 Outbox 来保证事件的原子性和最终一致性
- ✅ 避免每个微服务重复实现相同的逻辑
- ✅ 统一维护，统一升级

### 2. **每个微服务有独立的 Outbox 表和调度器实例** ✅

**决策**：每个微服务维护自己的 `outbox_events` 表和调度器实例。

**理由**：
- ✅ 符合微服务的**数据库独立性**原则
- ✅ 避免跨服务依赖
- ✅ 每个服务可以独立扩展和配置
- ✅ 故障隔离，一个服务的问题不影响其他服务

### 3. **领域模型完全数据库无关** ✅

**决策**：`OutboxEvent` 领域模型不包含任何数据库标签，数据库相关逻辑在适配器层。

**理由**：
- ✅ 符合**依赖倒置原则**（DIP）
- ✅ 业务微服务可以自由选择数据库（MySQL/PostgreSQL/MongoDB）
- ✅ 易于测试和维护
- ✅ 支持未来扩展到其他数据库

### 4. **调度器也在 jxt-core** ✅

**决策**：调度器的通用逻辑（轮询、重试、健康检查）在 jxt-core，业务微服务只需配置和启动。

**理由**：
- ✅ 调度逻辑在所有微服务中都是**完全相同**的
- ✅ 避免重复代码
- ✅ 统一升级和 bug 修复
- ✅ 业务微服务只需关注业务特定的部分（Topic 映射）

---

## 🏗️ 架构设计

### 分层架构

```
┌─────────────────────────────────────────────────────────┐
│              jxt-core/sdk/pkg/outbox/                   │
│  (通用框架，所有微服务共享)                              │
├─────────────────────────────────────────────────────────┤
│  1. OutboxEvent 模型（数据库无关）                       │
│  2. OutboxRepository 接口（定义契约）                    │
│  3. EventPublisher 接口（事件发布抽象）⭐ 零外部依赖     │
│  4. OutboxPublisher 发布器（发布逻辑）                   │
│  5. OutboxScheduler 调度器（调度逻辑）                   │
│  6. TopicMapper 接口（业务实现）                         │
│  7. Config 配置                                          │
└─────────────────────────────────────────────────────────┘
                          ▲
                          │ 使用 + 注入 EventPublisher
         ┌────────────────┼────────────────┐
         │                │                │
┌────────▼────────┐ ┌────▼────────┐ ┌────▼────────┐
│ evidence-mgmt   │ │ file-storage│ │ security-   │
│ - 独立 outbox 表│ │ - 独立 outbox│ │ - 独立 outbox│
│ - 独立调度器    │ │ - 独立调度器 │ │ - 独立调度器 │
│ - Topic 映射    │ │ - Topic 映射 │ │ - Topic 映射 │
│ - EventBus 适配器│ │ - EventBus 适配器│ │ - EventBus 适配器│
└─────────────────┘ └─────────────┘ └─────────────┘
         │                │                │
         ▼                ▼                ▼
┌─────────────────────────────────────────────────────────┐
│         jxt-core/sdk/pkg/eventbus/                      │
│  (EventBus 实现：Kafka/NATS/Memory)                     │
└─────────────────────────────────────────────────────────┘
```

### 核心组件

| 组件 | 位置 | 职责 | 数据库相关 |
|------|------|------|-----------|
| **OutboxEvent** | jxt-core | 领域模型、业务逻辑 | ❌ 无关 |
| **OutboxRepository** | jxt-core | 仓储接口、定义契约 | ❌ 无关 |
| **EventPublisher** | jxt-core | 事件发布接口 ⭐ | ❌ 无关 |
| **OutboxPublisher** | jxt-core | 事件发布逻辑 | ❌ 无关 |
| **OutboxScheduler** | jxt-core | 调度逻辑 | ❌ 无关 |
| **TopicMapper** | 业务微服务 | 聚合类型→Topic 映射 | ❌ 无关 |
| **EventBusAdapter** | 业务微服务 | EventBus 适配器 ⭐ | ❌ 无关 |
| **GORM 适配器** | jxt-core/adapters | GORM 仓储实现 | ✅ GORM |
| **MongoDB 适配器** | jxt-core/adapters | MongoDB 仓储实现（未来） | ✅ MongoDB |

---

## 📦 核心组件详解

### 1. OutboxEvent（领域模型）

**文件**：`jxt-core/sdk/pkg/outbox/event.go`

```go
type OutboxEvent struct {
    ID            string
    AggregateID   string
    AggregateType string     // 用于 Topic 映射
    EventType     string
    Payload       []byte
    Status        EventStatus
    RetryCount    int
    TenantID      string     // 多租户支持
    // ...
}
```

**特点**：
- ✅ 纯 Go 结构体，无数据库标签
- ✅ 包含业务逻辑方法（`MarkAsPublished()`, `CanRetry()` 等）
- ✅ 完全数据库无关

### 2. OutboxRepository（仓储接口）

**文件**：`jxt-core/sdk/pkg/outbox/repository.go`

```go
type OutboxRepository interface {
    Save(ctx, event) error
    SaveInTx(ctx, tx, event) error
    FindUnpublishedEvents(ctx, limit) ([]*OutboxEvent, error)
    MarkAsPublished(ctx, eventID) error
    // ...
}
```

**特点**：
- ✅ 定义数据访问契约
- ✅ 不依赖具体数据库实现
- ✅ 支持事务操作

### 3. EventPublisher（事件发布接口）⭐

**文件**：`jxt-core/sdk/pkg/outbox/event_publisher.go`

```go
type EventPublisher interface {
    Publish(ctx context.Context, topic string, data []byte) error
}
```

**特点**：
- ✅ **零外部依赖**：Outbox 包不依赖任何 EventBus 实现
- ✅ **依赖倒置**：依赖接口而非实现
- ✅ **易于测试**：可以轻松 mock
- ✅ **灵活扩展**：业务微服务可以使用任何 EventBus 实现

### 4. OutboxPublisher（发布器）

**文件**：`jxt-core/sdk/pkg/outbox/publisher.go`

```go
type OutboxPublisher struct {
    repo            OutboxRepository
    eventPublisher  EventPublisher    // 事件发布器（接口）⭐
    topicMapper     TopicMapper
    config          *PublisherConfig
}
```

**职责**：
- ✅ 使用 EventPublisher 接口发布事件
- ✅ 处理发布失败和重试逻辑
- ✅ 更新事件状态

### 5. OutboxScheduler（调度器）

**文件**：`jxt-core/sdk/pkg/outbox/scheduler.go`

```go
type Scheduler struct {
    repo      OutboxRepository
    publisher *OutboxPublisher
    config    *SchedulerConfig
    // ...
}
```

**职责**：
- ✅ 定时轮询未发布事件（默认 3 秒）
- ✅ 失败事件重试（默认 30 秒）
- ✅ 健康检查（默认 60 秒）
- ✅ 指标收集（默认 30 秒）

### 5. TopicMapper（Topic 映射）

**文件**：`jxt-core/sdk/pkg/outbox/topic_mapper.go`

```go
type TopicMapper interface {
    GetTopic(aggregateType string) string
}
```

**特点**：
- ✅ 由业务微服务实现
- ✅ 定义聚合类型到 Topic 的映射
- ✅ 业务特定的逻辑

---

## 🔌 数据库适配器

### GORM 适配器（默认提供）

**位置**：`jxt-core/sdk/pkg/outbox/adapters/gorm/`

**包含**：
- `model.go`：数据库模型（带 GORM 标签）
- `repository.go`：仓储实现
- 模型转换方法

**示例**：
```go
type OutboxEventModel struct {
    ID            string     `gorm:"type:char(36);primary_key"`
    AggregateID   string     `gorm:"type:varchar(255);not null;index"`
    // ...
}

func (m *OutboxEventModel) ToEntity() *outbox.OutboxEvent
func FromEntity(e *outbox.OutboxEvent) *OutboxEventModel
```

### 未来扩展

- MongoDB 适配器
- PostgreSQL 原生适配器
- 其他数据库适配器

---

## 🚀 业务微服务使用

### 四步集成

#### 步骤 1：定义 TopicMapper

```go
// internal/outbox/topics.go
func NewTopicMapper() outbox.TopicMapper {
    return outbox.NewTopicMapper(map[string]string{
        "Archive": "archive-events",
        "Media":   "media-events",
    })
}
```

#### 步骤 2：创建 EventBus 适配器 ⭐

```go
// internal/outbox/eventbus_adapter.go
package outbox

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

type EventBusAdapter struct {
    eventBus eventbus.EventBus
}

func NewEventBusAdapter(eventBus eventbus.EventBus) outbox.EventPublisher {
    return &EventBusAdapter{eventBus: eventBus}
}

func (a *EventBusAdapter) Publish(ctx context.Context, topic string, data []byte) error {
    return a.eventBus.Publish(ctx, topic, data)
}
```

#### 步骤 3：初始化 Outbox

```go
// cmd/main.go
db := sdk.Runtime.GetDB()
db.AutoMigrate(&gormadapter.OutboxEventModel{})

// 创建 EventBus 适配器 ⭐
eventBus := sdk.Runtime.GetEventBus()
eventPublisher := localoutbox.NewEventBusAdapter(eventBus)

scheduler := outbox.NewScheduler(
    outbox.WithRepository(gormadapter.NewGormOutboxRepository(db)),
    outbox.WithEventPublisher(eventPublisher),  // 注入 EventPublisher ⭐
    outbox.WithTopicMapper(localoutbox.NewTopicMapper()),
)

scheduler.Start(ctx)
defer scheduler.Stop(ctx)
```

#### 步骤 4：业务代码使用

```go
// service/archive_service.go
func (s *Service) CreateArchive(ctx context.Context, cmd *Command) error {
    return transaction.RunInTransaction(ctx, s.txManager, func(tx Transaction) error {
        // 1. 保存业务数据
        s.repo.CreateInTx(ctx, tx, archive)
        
        // 2. 保存事件到 outbox
        for _, event := range archive.Events() {
            s.outboxRepo.SaveInTx(ctx, tx, event)
        }
        
        return nil
        // 调度器会自动发布事件
    })
}
```

---

## 📊 职责划分

| 组件 | 位置 | 业务微服务需要做什么 |
|------|------|---------------------|
| **OutboxEvent** | jxt-core | ❌ 无需实现 |
| **OutboxRepository** | jxt-core | ❌ 无需实现 |
| **EventPublisher 接口** | jxt-core | ❌ 无需实现（只是接口定义）⭐ |
| **OutboxPublisher** | jxt-core | ❌ 无需实现 |
| **OutboxScheduler** | jxt-core | ❌ 无需实现 |
| **GORM 适配器** | jxt-core | ❌ 无需实现（可选使用） |
| **TopicMapper** | 业务微服务 | ✅ **需要实现**（定义映射） |
| **EventBusAdapter** | 业务微服务 | ✅ **需要实现**（5-10 行代码）⭐ |
| **配置** | 业务微服务 | ✅ **可选**（有默认值） |
| **启动调度器** | 业务微服务 | ✅ **需要启动**（一行代码） |

---

## 🎁 核心优势

### 1. 代码复用

- ✅ 避免每个微服务重复实现 Outbox 逻辑
- ✅ 统一的实现，统一的维护
- ✅ 新微服务开箱即用

### 2. 最佳实践

- ✅ jxt-core 提供经过验证的 Outbox 实现
- ✅ 包含重试、监控、健康检查等企业级特性
- ✅ 符合 DDD 和微服务最佳实践

### 3. 简化开发

- ✅ 业务微服务只需 4 步集成
- ✅ 只需关注业务特定的部分（Topic 映射、EventBus 适配器）
- ✅ 大幅降低开发成本

### 4. 统一升级

- ✅ Outbox 功能改进，所有微服务自动受益
- ✅ 集中式的 bug 修复
- ✅ 统一的监控和运维

### 5. 灵活扩展

- ✅ 支持多种数据库（通过适配器）
- ✅ 支持自定义实现
- ✅ 配置灵活，可按需调整

---

## 📚 文档结构

### 1. [快速开始指南](./docs/outbox-pattern-quick-start.md)

**适合**：快速上手，5 分钟集成

**内容**：
- 三步集成
- 完整示例
- 验证集成
- 常见问题

### 2. [完整设计方案](./docs/outbox-pattern-design.md)

**适合**：深入理解，生产环境部署

**内容**：
- 整体架构
- 核心组件详解
- 数据库适配器
- 配置说明
- 事件处理流程
- 多租户支持
- 监控运维
- 最佳实践
- 故障排查

---

## 🔄 实施计划

### 阶段 1：实现 jxt-core Outbox 框架（2 周）

- [ ] 实现 `OutboxEvent` 领域模型
- [ ] 实现 `OutboxRepository` 接口
- [ ] 实现 `OutboxPublisher` 发布器
- [ ] 实现 `OutboxScheduler` 调度器
- [ ] 实现 `TopicMapper` 接口
- [ ] 实现 GORM 适配器
- [ ] 编写单元测试
- [ ] 编写集成测试

### 阶段 2：迁移 evidence-management（1 周）

- [ ] 定义 TopicMapper
- [ ] 初始化 Outbox
- [ ] 迁移业务代码
- [ ] 测试验证
- [ ] 监控配置

### 阶段 3：迁移其他微服务（2 周）

- [ ] file-storage-service
- [ ] security-management
- [ ] 其他微服务

### 阶段 4：完善和优化（1 周）

- [ ] 性能优化
- [ ] 监控告警
- [ ] 文档完善
- [ ] 培训和推广

---

## ✅ 验收标准

### 功能验收

- [ ] 所有核心组件实现完成
- [ ] GORM 适配器实现完成
- [ ] 单元测试覆盖率 > 80%
- [ ] 集成测试通过
- [ ] 至少一个微服务成功集成

### 性能验收

- [ ] 事件发布延迟 < 5 秒（P99）
- [ ] 发布成功率 > 99%
- [ ] 支持 1000+ TPS

### 文档验收

- [ ] 快速开始指南完成
- [ ] 完整设计方案完成
- [ ] API 文档完成
- [ ] 示例代码完成

---

## 📝 总结

### 核心价值

| 价值 | 说明 |
|------|------|
| **原子性保证** | 业务操作与事件发布的原子性 |
| **最终一致性** | 通过调度器保证事件最终被发布 |
| **高度复用** | 所有微服务共享通用框架 |
| **数据库无关** | 支持多种数据库实现 |
| **微服务独立** | 每个微服务独立的 outbox 和调度器 |
| **易于使用** | 三步集成，开箱即用 |
| **企业级特性** | 重试、监控、健康检查 |

### 关键决策

1. ✅ **Outbox 核心逻辑提炼到 jxt-core**
2. ✅ **每个微服务有独立的 Outbox 表和调度器**
3. ✅ **领域模型完全数据库无关**
4. ✅ **调度器也在 jxt-core**
5. ✅ **业务微服务只需提供 TopicMapper**

---

**方案版本**：v1.0  
**最后更新**：2025-10-19  
**维护者**：jxt-core 团队


