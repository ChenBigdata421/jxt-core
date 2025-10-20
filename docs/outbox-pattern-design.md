# Outbox 模式设计方案

## 📋 概述

本文档详细说明了 jxt-core 提供的 Outbox 模式通用框架设计，用于在事件驱动架构中保证业务操作与事件发布的原子性和最终一致性。

### 核心目标

- ✅ **原子性保证**：业务操作与事件保存在同一事务中
- ✅ **最终一致性**：通过调度器保证事件最终被发布
- ✅ **高度复用**：所有微服务共享通用框架
- ✅ **数据库无关**：支持多种数据库实现
- ✅ **微服务独立**：每个微服务有独立的 outbox 表和调度器实例

---

## 🏗️ 整体架构

### 架构分层

```
┌─────────────────────────────────────────────────────────┐
│              jxt-core/sdk/pkg/outbox/                   │
├─────────────────────────────────────────────────────────┤
│  核心层（通用，所有微服务共享）                          │
│  ┌───────────────────────────────────────────────────┐  │
│  │ 1. OutboxEvent 模型（领域模型）                   │  │
│  │    - 数据库无关的纯 Go 结构体                     │  │
│  │    - 业务逻辑方法                                 │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ 2. OutboxRepository 接口（仓储契约）              │  │
│  │    - 定义数据访问行为                             │  │
│  │    - 不依赖具体数据库实现                         │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ 3. EventPublisher 接口（事件发布抽象）⭐          │  │
│  │    - 定义事件发布行为                             │  │
│  │    - 不依赖具体 EventBus 实现                     │  │
│  │    - 业务微服务注入实现（依赖倒置）               │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ 4. OutboxPublisher 发布器（发布逻辑）             │  │
│  │    - 使用 EventPublisher 发布事件                 │  │
│  │    - 重试逻辑、错误处理                           │  │
│  │    - 状态更新                                     │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ 5. OutboxScheduler 调度器（调度逻辑）             │  │
│  │    - 定时轮询未发布事件                           │  │
│  │    - 失败事件重试                                 │  │
│  │    - 健康检查和监控                               │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ 6. TopicMapper 接口（Topic 映射）                 │  │
│  │    - 聚合类型 → Topic 的映射                     │  │
│  │    - 业务微服务实现                               │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ 7. Config 配置（配置管理）                        │  │
│  │    - 调度器配置                                   │  │
│  │    - 发布器配置                                   │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│        jxt-core/sdk/pkg/outbox/adapters/                │
├─────────────────────────────────────────────────────────┤
│  适配器层（可选的数据库实现）                            │
│  ┌───────────────────────────────────────────────────┐  │
│  │ GORM 适配器（默认提供）                           │  │
│  │  - GormOutboxRepository 实现                      │  │
│  │  - OutboxEventModel（带 GORM 标签）              │  │
│  │  - 领域模型 ↔ 数据库模型转换                     │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ MongoDB 适配器（未来扩展）                        │  │
│  │ PostgreSQL 适配器（未来扩展）                     │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│              业务微服务层                                │
├─────────────────────────────────────────────────────────┤
│  evidence-management:                                   │
│  ┌───────────────────────────────────────────────────┐  │
│  │ - 独立的 outbox_events 表（MySQL）                │  │
│  │ - 独立的调度器实例                                 │  │
│  │ - 业务特定的 TopicMapper                          │  │
│  │ - EventBus 适配器（注入到 Outbox）⭐              │  │
│  │ - 使用 jxt-core 的 GormOutboxRepository          │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  file-storage-service:                                  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ - 独立的 outbox_events 表（PostgreSQL）           │  │
│  │ - 独立的调度器实例                                 │  │
│  │ - 业务特定的 TopicMapper                          │  │
│  │ - EventBus 适配器（注入到 Outbox）⭐              │  │
│  │ - 使用 jxt-core 的 GormOutboxRepository          │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  security-management:                                   │
│  ┌───────────────────────────────────────────────────┐  │
│  │ - 独立的 outbox_events 表（MySQL）                │  │
│  │ - 独立的调度器实例                                 │  │
│  │ - 业务特定的 TopicMapper                          │  │
│  │ - EventBus 适配器（注入到 Outbox）⭐              │  │
│  │ - 使用 jxt-core 的 GormOutboxRepository          │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 组件交互流程

```
┌──────────────┐
│ 业务服务     │
└──────┬───────┘
       │ 1. 在事务中保存业务数据和事件
       ▼
┌──────────────────────┐
│ OutboxRepository     │
│ (保存到 outbox 表)   │
└──────────────────────┘
       │
       │ 2. 事务提交成功
       ▼
┌──────────────────────┐
│ OutboxScheduler      │
│ (定时轮询)           │
└──────┬───────────────┘
       │ 3. 查询未发布事件
       ▼
┌──────────────────────┐
│ OutboxPublisher      │
│ (发布逻辑)           │
└──────┬───────────────┘
       │ 4. 获取 Topic
       ▼
┌──────────────────────┐
│ TopicMapper          │
│ (业务提供)           │
└──────┬───────────────┘
       │ 5. 发布到 EventPublisher（接口）⭐
       ▼
┌──────────────────────┐
│ EventBusAdapter      │
│ (业务微服务提供)     │
└──────┬───────────────┘
       │ 6. 调用 EventBus
       ▼
┌──────────────────────┐
│ EventBus             │
│ (Kafka/NATS/Memory)  │
└──────────────────────┘
       │ 7. 更新事件状态
       ▼
┌──────────────────────┐
│ OutboxRepository     │
│ (标记为已发布)       │
└──────────────────────┘
```

---

## 📦 核心组件详解

### 1. OutboxEvent 领域模型

**位置**：`jxt-core/sdk/pkg/outbox/event.go`

**职责**：
- 定义 Outbox 事件的领域模型
- 提供业务逻辑方法
- **完全数据库无关**

**核心字段**：
```go
type OutboxEvent struct {
    ID            string     // 事件唯一标识
    AggregateID   string     // 聚合根ID
    AggregateType string     // 聚合类型（用于 Topic 映射）
    EventType     string     // 事件类型
    Payload       []byte     // 事件负载（JSON）
    CreatedAt     time.Time  // 创建时间
    Status        EventStatus // 事件状态
    PublishedAt   *time.Time // 发布时间
    RetryCount    int        // 重试次数
    LastRetryAt   *time.Time // 最后重试时间
    ErrorMessage  string     // 错误信息
    TenantID      string     // 租户ID（多租户支持）
    Version       int        // 版本号
}
```

**事件状态**：
```go
type EventStatus string

const (
    StatusCreated   EventStatus = "CREATED"   // 已创建，未发布
    StatusPublished EventStatus = "PUBLISHED" // 已发布
    StatusFailed    EventStatus = "FAILED"    // 发布失败
    StatusMaxRetry  EventStatus = "MAX_RETRY" // 超过最大重试次数
)
```

**业务方法**：
```go
// MarkAsPublished 标记为已发布
func (e *OutboxEvent) MarkAsPublished()

// MarkAsFailed 标记为失败
func (e *OutboxEvent) MarkAsFailed(errorMessage string)

// MarkAsMaxRetry 标记为超过最大重试次数
func (e *OutboxEvent) MarkAsMaxRetry(errorMessage string)

// IncrementRetry 增加重试次数
func (e *OutboxEvent) IncrementRetry(errorMessage string)

// CanRetry 检查是否可以重试
func (e *OutboxEvent) CanRetry(maxRetries int) bool

// IsPublished 检查是否已发布
func (e *OutboxEvent) IsPublished() bool

// ToDomainEvent 转换为领域事件
func (e *OutboxEvent) ToDomainEvent() (*DomainEvent, error)
```

---

### 2. OutboxRepository 仓储接口

**位置**：`jxt-core/sdk/pkg/outbox/repository.go`

**职责**：
- 定义数据访问契约
- **不依赖具体数据库实现**
- 支持事务操作

**核心方法**：
```go
type OutboxRepository interface {
    // 保存事件
    Save(ctx context.Context, event *OutboxEvent) error
    SaveInTx(ctx context.Context, tx Transaction, event *OutboxEvent) error
    
    // 查询事件
    FindUnpublishedEvents(ctx context.Context, limit int) ([]*OutboxEvent, error)
    FindUnpublishedEventsByTenant(ctx context.Context, tenantID string, limit int) ([]*OutboxEvent, error)
    FindEventsForRetry(ctx context.Context, maxRetries int, limit int) ([]*OutboxEvent, error)
    FindByID(ctx context.Context, eventID string) (*OutboxEvent, error)
    
    // 更新状态
    MarkAsPublished(ctx context.Context, eventID string) error
    MarkAsMaxRetry(ctx context.Context, eventID string, errorMsg string) error
    IncrementRetry(ctx context.Context, eventID string, errorMsg string) error
    UpdateStatus(ctx context.Context, eventID string, status EventStatus, errorMsg string) error
    
    // 统计
    CountUnpublishedEvents(ctx context.Context) (int64, error)
    CountUnpublishedEventsByTenant(ctx context.Context, tenantID string) (int64, error)
    
    // 清理
    Delete(ctx context.Context, eventID string) error
    DeleteOldPublishedEvents(ctx context.Context, beforeTime time.Time) error
}
```

---

### 3. EventPublisher 接口 ⭐

**位置**：`jxt-core/sdk/pkg/outbox/event_publisher.go`

**职责**：
- 定义事件发布行为的抽象接口
- **不依赖具体 EventBus 实现**
- 业务微服务注入实现（依赖倒置原则）

**接口定义**：
```go
package outbox

import "context"

// EventPublisher 事件发布器接口
// Outbox 组件通过这个接口发布事件，不依赖具体的 EventBus 实现
type EventPublisher interface {
    // Publish 发布事件到指定 topic
    // ctx: 上下文
    // topic: 目标 topic（由 TopicMapper 提供）
    // data: 事件数据（已序列化为 JSON）
    Publish(ctx context.Context, topic string, data []byte) error
}
```

**设计优势**：
- ✅ **零外部依赖**：Outbox 包不依赖任何 EventBus 实现
- ✅ **依赖倒置**：依赖接口而非实现
- ✅ **易于测试**：可以轻松 mock EventPublisher
- ✅ **灵活扩展**：业务微服务可以使用任何 EventBus 实现

**业务微服务实现示例**：
```go
// evidence-management/internal/outbox/eventbus_adapter.go
package outbox

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// EventBusAdapter 将 jxt-core EventBus 适配为 Outbox EventPublisher
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

---

### 4. OutboxPublisher 发布器

**位置**：`jxt-core/sdk/pkg/outbox/publisher.go`

**职责**：
- 使用 EventPublisher 接口发布事件
- 处理发布失败和重试逻辑
- 更新事件状态

**核心结构**：
```go
type OutboxPublisher struct {
    repo            OutboxRepository  // 仓储
    eventPublisher  EventPublisher    // 事件发布器（接口）⭐
    topicMapper     TopicMapper       // Topic 映射
    config          *PublisherConfig  // 配置
}
```

**配置**：
```go
type PublisherConfig struct {
    MaxRetries int           // 最大重试次数
    BaseDelay  time.Duration // 基础延迟
    MaxDelay   time.Duration // 最大延迟
}
```

**核心方法**：
```go
// PublishEvent 发布单个事件
func (p *OutboxPublisher) PublishEvent(ctx context.Context, event *OutboxEvent) error

// PublishEvents 批量发布事件
func (p *OutboxPublisher) PublishEvents(ctx context.Context, events []*OutboxEvent) error
```

**发布流程**：
1. 检查事件是否可以重试
2. 通过 TopicMapper 获取 Topic
3. 转换为领域事件
4. 通过 EventPublisher 接口发布 ⭐
5. 更新事件状态（成功/失败）

**依赖注入**：
```go
// 创建 OutboxPublisher 时注入 EventPublisher
publisher := outbox.NewOutboxPublisher(
    repo,
    eventPublisher,  // 注入实现（由业务微服务提供）
    topicMapper,
    config,
)
```

---

### 5. OutboxScheduler 调度器

**位置**：`jxt-core/sdk/pkg/outbox/scheduler.go`

**职责**：
- 定时轮询未发布事件
- 触发事件发布
- 失败事件重试
- 健康检查和监控

**核心结构**：
```go
type Scheduler struct {
    repo      OutboxRepository  // 仓储
    publisher *OutboxPublisher  // 发布器
    config    *SchedulerConfig  // 配置
    stopChan  chan struct{}     // 停止信号
    wg        sync.WaitGroup    // 等待组
    running   bool              // 运行状态
    mu        sync.RWMutex      // 锁
}
```

**配置**：
```go
type SchedulerConfig struct {
    MainProcessInterval  time.Duration // 主处理间隔（默认 3s）
    RetryProcessInterval time.Duration // 重试间隔（默认 30s）
    HealthCheckInterval  time.Duration // 健康检查间隔（默认 60s）
    MetricsInterval      time.Duration // 指标收集间隔（默认 30s）
    BatchSize            int           // 批处理大小（默认 100）
}
```

**核心方法**：
```go
// Start 启动调度器
func (s *Scheduler) Start(ctx context.Context) error

// Stop 停止调度器
func (s *Scheduler) Stop(ctx context.Context) error

// IsRunning 检查是否运行中
func (s *Scheduler) IsRunning() bool
```

**内部循环**：
- `mainProcessLoop`：主处理循环，定时处理未发布事件
- `retryProcessLoop`：重试循环，处理失败事件
- `healthCheckLoop`：健康检查循环
- `metricsLoop`：指标收集循环

---

### 6. TopicMapper 接口

**位置**：`jxt-core/sdk/pkg/outbox/topic_mapper.go`

**职责**：
- 定义聚合类型到 Topic 的映射
- **由业务微服务实现**

**接口定义**：
```go
type TopicMapper interface {
    GetTopic(aggregateType string) string
}
```

**简单实现**：
```go
type SimpleTopicMapper struct {
    mapping map[string]string
}

func NewTopicMapper(mapping map[string]string) TopicMapper {
    return &SimpleTopicMapper{mapping: mapping}
}

func (m *SimpleTopicMapper) GetTopic(aggregateType string) string {
    if topic, ok := m.mapping[aggregateType]; ok {
        return topic
    }
    return "" // 或返回默认 topic
}
```

---

## 🔌 数据库适配器

### GORM 适配器（默认提供）

**位置**：`jxt-core/sdk/pkg/outbox/adapters/gorm/`

#### 数据库模型

**文件**：`adapters/gorm/model.go`

```go
type OutboxEventModel struct {
    ID            string     `gorm:"type:char(36);primary_key"`
    AggregateID   string     `gorm:"type:varchar(255);not null;index"`
    AggregateType string     `gorm:"type:varchar(100);not null;index"`
    EventType     string     `gorm:"type:varchar(100);not null;index"`
    Payload       []byte     `gorm:"type:json;not null"`
    CreatedAt     time.Time  `gorm:"type:datetime;not null;index"`
    Status        string     `gorm:"type:varchar(20);not null;index"`
    PublishedAt   *time.Time `gorm:"type:datetime"`
    RetryCount    int        `gorm:"type:int;not null;default:0"`
    LastRetryAt   *time.Time `gorm:"type:datetime"`
    ErrorMessage  string     `gorm:"type:text"`
    TenantID      string     `gorm:"type:varchar(100);not null;index"`
    Version       int        `gorm:"type:int;not null;default:1"`
}

func (OutboxEventModel) TableName() string {
    return "outbox_events"
}
```

#### 仓储实现

**文件**：`adapters/gorm/repository.go`

```go
type GormOutboxRepository struct {
    db *gorm.DB
}

func NewGormOutboxRepository(db *gorm.DB) outbox.OutboxRepository {
    return &GormOutboxRepository{db: db}
}

// 实现 OutboxRepository 接口的所有方法
```

#### 模型转换

```go
// ToEntity 数据库模型 → 领域模型
func (m *OutboxEventModel) ToEntity() *outbox.OutboxEvent

// FromEntity 领域模型 → 数据库模型
func FromEntity(e *outbox.OutboxEvent) *OutboxEventModel
```

---

## 🚀 使用指南

### 业务微服务集成步骤

#### 步骤 1：定义 TopicMapper

```go
// evidence-management/internal/outbox/topics.go
package outbox

import "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"

func NewTopicMapper() outbox.TopicMapper {
    return outbox.NewTopicMapper(map[string]string{
        "Archive":                "archive-events",
        "Media":                  "media-events",
        "ArchiveMediaRelation":   "archive-media-relation-events",
        "EnforcementType":        "enforcement-type-events",
        "IncidentRecord":         "incident-record-events",
    })
}
```

#### 步骤 2：创建 EventBus 适配器 ⭐

```go
// evidence-management/internal/outbox/eventbus_adapter.go
package outbox

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// EventBusAdapter 将 jxt-core EventBus 适配为 Outbox EventPublisher
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
// evidence-management/cmd/main.go
package main

import (
    "context"
    "time"
    "github.com/ChenBigdata421/jxt-core/sdk"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
    localoutbox "jxt-evidence-system/evidence-management/internal/outbox"
)

func main() {
    ctx := context.Background()

    // 1. 获取数据库连接
    db := sdk.Runtime.GetDB()

    // 2. 自动迁移 outbox 表
    if err := db.AutoMigrate(&gormadapter.OutboxEventModel{}); err != nil {
        panic(err)
    }

    // 3. 创建仓储
    outboxRepo := gormadapter.NewGormOutboxRepository(db)

    // 4. 创建 EventBus 适配器 ⭐
    eventBus := sdk.Runtime.GetEventBus()
    eventPublisher := localoutbox.NewEventBusAdapter(eventBus)

    // 5. 创建调度器（注入 EventPublisher）⭐
    scheduler := outbox.NewScheduler(
        outbox.WithRepository(outboxRepo),
        outbox.WithEventPublisher(eventPublisher),  // 注入 EventPublisher 而非 EventBus
        outbox.WithTopicMapper(localoutbox.NewTopicMapper()),
        outbox.WithConfig(&outbox.SchedulerConfig{
            MainProcessInterval:  3 * time.Second,
            RetryProcessInterval: 30 * time.Second,
            HealthCheckInterval:  60 * time.Second,
            MetricsInterval:      30 * time.Second,
            BatchSize:            100,
        }),
        outbox.WithPublisherConfig(&outbox.PublisherConfig{
            MaxRetries: 5,
            BaseDelay:  2 * time.Second,
            MaxDelay:   30 * time.Minute,
        }),
    )

    // 6. 启动调度器
    if err := scheduler.Start(ctx); err != nil {
        panic(err)
    }

    // 应用启动...

    // 优雅关闭
    defer scheduler.Stop(ctx)
}
```

#### 步骤 4：业务代码中使用

```go
// evidence-management/internal/application/service/archive_service.go
package service

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    "jxt-evidence-system/evidence-management/shared/common/transaction"
)

type ArchiveService struct {
    archiveRepo repository.ArchiveRepository
    outboxRepo  outbox.OutboxRepository
    txManager   transaction.TransactionManager
}

// CreateArchive 创建档案
func (s *ArchiveService) CreateArchive(ctx context.Context, cmd *CreateArchiveCommand) error {
    // 使用事务确保业务数据和事件的原子性
    return transaction.RunInTransaction(ctx, s.txManager, func(tx transaction.Transaction) error {
        // 1. 创建档案聚合根
        archive := aggregate.NewArchive(cmd.Code, cmd.Name)

        // 2. 在事务中保存档案
        if err := s.archiveRepo.CreateInTx(ctx, tx, archive); err != nil {
            return err
        }

        // 3. 获取领域事件
        events := archive.Events()

        // 4. 在同一事务中保存事件到 outbox 表
        for _, event := range events {
            if err := s.outboxRepo.SaveInTx(ctx, tx, event); err != nil {
                return err
            }
        }

        // 5. 事务提交后，调度器会自动发布事件
        return nil
    })
}
```

---

## 📊 配置说明

### 调度器配置

```yaml
# config.yaml
outbox:
  scheduler:
    # 主处理间隔：多久轮询一次未发布事件
    mainProcessInterval: 3s

    # 重试间隔：多久处理一次失败事件
    retryProcessInterval: 30s

    # 健康检查间隔
    healthCheckInterval: 60s

    # 指标收集间隔
    metricsInterval: 30s

    # 批处理大小：每次处理多少个事件
    batchSize: 100

  publisher:
    # 最大重试次数
    maxRetries: 5

    # 基础延迟（用于指数退避）
    baseDelay: 2s

    # 最大延迟
    maxDelay: 30m
```

### 默认配置

如果不提供配置，将使用以下默认值：

```go
// 调度器默认配置
DefaultSchedulerConfig = &SchedulerConfig{
    MainProcessInterval:  3 * time.Second,
    RetryProcessInterval: 30 * time.Second,
    HealthCheckInterval:  60 * time.Second,
    MetricsInterval:      30 * time.Second,
    BatchSize:            100,
}

// 发布器默认配置
DefaultPublisherConfig = &PublisherConfig{
    MaxRetries: 5,
    BaseDelay:  2 * time.Second,
    MaxDelay:   30 * time.Minute,
}
```

---

## 🔄 事件处理流程

### 完整流程图

```
┌─────────────────────────────────────────────────────────┐
│                   业务操作                               │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              开始数据库事务                              │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│          保存业务数据（如：Archive）                      │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│    保存领域事件到 outbox_events 表（状态：CREATED）      │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│                  提交事务                                │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              业务操作完成，返回成功                       │
└─────────────────────────────────────────────────────────┘

        （异步，由调度器处理）
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│    OutboxScheduler 定时轮询（每 3 秒）                   │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│    查询状态为 CREATED 的事件（批量 100 个）              │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              OutboxPublisher 处理事件                    │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│    通过 TopicMapper 获取 Topic                          │
│    (如：Archive → "archive-events")                     │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│         转换为领域事件（DomainEvent）                    │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│    发布到 EventBus (Kafka/NATS/Memory)                  │
└────────────────────┬────────────────────────────────────┘
                     │
                     ├─────────────┐
                     │             │
                 成功 │             │ 失败
                     │             │
                     ▼             ▼
        ┌────────────────┐  ┌──────────────────┐
        │ 标记为 PUBLISHED│  │ 增加重试次数      │
        │ 设置发布时间    │  │ 记录错误信息      │
        └────────────────┘  │ 状态保持 CREATED  │
                            └──────────────────┘
                                    │
                                    ▼
                            ┌──────────────────┐
                            │ 等待下次重试      │
                            │ (指数退避)       │
                            └──────────────────┘
```

### 重试策略

#### 指数退避算法

```go
delay := baseDelay * (2 ^ retryCount)
if delay > maxDelay {
    delay = maxDelay
}
```

#### 重试示例

| 重试次数 | 延迟时间 | 说明 |
|---------|---------|------|
| 0 | 立即 | 首次发布 |
| 1 | 2s | 第一次重试 |
| 2 | 4s | 第二次重试 |
| 3 | 8s | 第三次重试 |
| 4 | 16s | 第四次重试 |
| 5 | 32s | 第五次重试 |
| 6+ | MAX_RETRY | 超过最大重试次数 |

---

## 🏢 多租户支持

### 租户隔离

每个租户的事件在 `outbox_events` 表中通过 `tenant_id` 字段隔离：

```sql
CREATE TABLE outbox_events (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(100) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    -- ... 其他字段
    INDEX idx_tenant_status (tenant_id, status),
    INDEX idx_tenant_created (tenant_id, created_at)
);
```

### 多租户查询

```go
// 查询指定租户的未发布事件
events, err := repo.FindUnpublishedEventsByTenant(ctx, tenantID, limit)

// 统计指定租户的未发布事件数量
count, err := repo.CountUnpublishedEventsByTenant(ctx, tenantID)
```

---

## 📈 监控和运维

### 健康检查

调度器会定期执行健康检查：

```go
func (s *Scheduler) performHealthCheck(ctx context.Context) {
    // 检查未发布事件数量
    count, err := s.repo.CountUnpublishedEvents(ctx)
    if err != nil {
        logger.Error("健康检查失败", "error", err)
        return
    }

    // 如果积压超过阈值，发出警告
    if count > 1000 {
        logger.Warn("未发布事件积压", "count", count)
    }
}
```

### 指标收集

调度器会定期收集以下指标：

- **未发布事件数量**：`outbox.unpublished.count`
- **失败事件数量**：`outbox.failed.count`
- **超过最大重试事件数量**：`outbox.max_retry.count`
- **已处理事件数量**：`outbox.processed.count`
- **发布成功率**：`outbox.publish.success_rate`

### 日志记录

```go
// 发布成功
logger.Info("事件发布成功",
    "eventID", event.ID,
    "topic", topic,
    "aggregateType", event.AggregateType)

// 发布失败
logger.Error("事件发布失败",
    "eventID", event.ID,
    "topic", topic,
    "error", err,
    "retryCount", event.RetryCount)

// 超过最大重试
logger.Warn("事件超过最大重试次数",
    "eventID", event.ID,
    "retryCount", event.RetryCount,
    "errorMessage", event.ErrorMessage)
```

---

## 🧹 事件清理

### 自动清理

定期清理已发布的旧事件，避免表过大：

```go
// 清理 7 天前已发布的事件
beforeTime := time.Now().AddDate(0, 0, -7)
err := repo.DeleteOldPublishedEvents(ctx, beforeTime)
```

### 清理策略

建议配置：
- **清理间隔**：每天一次
- **保留时间**：7-30 天
- **批量删除**：每次删除 1000 条

---

## 🔒 事务保证

### ACID 保证

Outbox 模式通过数据库事务保证：

1. **原子性（Atomicity）**：业务数据和事件要么都保存，要么都不保存
2. **一致性（Consistency）**：事务提交后，数据和事件状态一致
3. **隔离性（Isolation）**：并发事务互不干扰
4. **持久性（Durability）**：事务提交后，数据持久化

### 示例

```go
// 错误示例：不在事务中
func (s *Service) CreateArchive(ctx context.Context, cmd *Command) error {
    // ❌ 业务数据保存
    s.repo.Create(ctx, archive)

    // ❌ 事件保存（不在同一事务中，可能失败）
    s.outboxRepo.Save(ctx, event)

    // 如果事件保存失败，业务数据已经保存，数据不一致！
}

// 正确示例：在事务中
func (s *Service) CreateArchive(ctx context.Context, cmd *Command) error {
    return transaction.RunInTransaction(ctx, s.txManager, func(tx Transaction) error {
        // ✅ 业务数据保存
        if err := s.repo.CreateInTx(ctx, tx, archive); err != nil {
            return err // 事务回滚
        }

        // ✅ 事件保存（同一事务）
        if err := s.outboxRepo.SaveInTx(ctx, tx, event); err != nil {
            return err // 事务回滚
        }

        // 两者要么都成功，要么都失败
        return nil
    })
}
```

---

## 🎯 最佳实践

### 1. 事件设计

**DO**：
- ✅ 事件应该是**不可变的**
- ✅ 事件应该包含**完整的业务信息**
- ✅ 使用**过去时态**命名事件（如：`ArchiveCreated`）
- ✅ 事件 Payload 使用 **JSON 格式**

**DON'T**：
- ❌ 不要在事件中包含敏感信息（如密码）
- ❌ 不要在事件中包含大量数据（考虑使用引用）
- ❌ 不要修改已发布的事件结构（考虑版本化）

### 2. 性能优化

**批量处理**：
```go
// 配置合适的批处理大小
config := &outbox.SchedulerConfig{
    BatchSize: 100, // 根据实际情况调整
}
```

**索引优化**：
```sql
-- 确保有合适的索引
CREATE INDEX idx_status_created ON outbox_events(status, created_at);
CREATE INDEX idx_tenant_status ON outbox_events(tenant_id, status);
```

**定期清理**：
```go
// 定期清理已发布的旧事件
go func() {
    ticker := time.NewTicker(24 * time.Hour)
    for range ticker.C {
        beforeTime := time.Now().AddDate(0, 0, -7)
        repo.DeleteOldPublishedEvents(ctx, beforeTime)
    }
}()
```

### 3. 错误处理

**幂等性**：
```go
// 消费端应该实现幂等性处理
func (h *ArchiveEventHandler) Handle(ctx context.Context, event *Event) error {
    // 检查事件是否已处理
    if h.isProcessed(event.ID) {
        return nil // 幂等，直接返回
    }

    // 处理事件
    if err := h.process(event); err != nil {
        return err
    }

    // 标记为已处理
    h.markAsProcessed(event.ID)
    return nil
}
```

**重试策略**：
```go
// 配置合理的重试策略
config := &outbox.PublisherConfig{
    MaxRetries: 5,           // 最大重试 5 次
    BaseDelay:  2 * time.Second,  // 基础延迟 2 秒
    MaxDelay:   30 * time.Minute, // 最大延迟 30 分钟
}
```

### 4. 监控告警

**关键指标**：
- 未发布事件数量 > 1000：警告
- 失败事件数量 > 100：警告
- 超过最大重试事件数量 > 10：告警
- 发布成功率 < 95%：告警

**告警配置**：
```yaml
alerts:
  - name: outbox_backlog
    condition: outbox.unpublished.count > 1000
    severity: warning

  - name: outbox_failed
    condition: outbox.failed.count > 100
    severity: warning

  - name: outbox_max_retry
    condition: outbox.max_retry.count > 10
    severity: critical
```

---

## 🔧 故障排查

### 常见问题

#### 1. 事件未发布

**症状**：事件保存到 outbox 表，但长时间未发布

**排查步骤**：
1. 检查调度器是否启动：`scheduler.IsRunning()`
2. 检查 EventBus 连接：查看 EventBus 健康状态
3. 检查 TopicMapper：确认聚合类型有对应的 Topic
4. 查看日志：查找错误信息

**解决方案**：
```bash
# 查看未发布事件
SELECT * FROM outbox_events WHERE status = 'CREATED' ORDER BY created_at;

# 手动触发处理（如果提供了 HTTP 端点）
curl -X POST http://localhost:8080/api/outbox/trigger/process
```

#### 2. 事件重复发布

**症状**：同一事件被发布多次

**原因**：
- 调度器重启时，事件状态未及时更新
- 网络问题导致状态更新失败

**解决方案**：
- 消费端实现幂等性处理
- 使用事件 ID 去重

#### 3. 事件积压

**症状**：未发布事件数量持续增长

**排查步骤**：
1. 检查 EventBus 是否正常
2. 检查发布速率是否跟得上生产速率
3. 检查是否有大量失败事件

**解决方案**：
```go
// 增加批处理大小
config.BatchSize = 200

// 减少处理间隔
config.MainProcessInterval = 1 * time.Second

// 增加并发（如果支持）
```

---

## 📚 参考资料

### 相关文档

- [EventBus 基础文档](../sdk/pkg/eventbus/README.md)
- [事务管理文档](../sdk/pkg/transaction/README.md)
- [多租户支持文档](../sdk/pkg/multitenancy/README.md)

### 设计模式

- [Transactional Outbox Pattern](https://microservices.io/patterns/data/transactional-outbox.html)
- [Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html)
- [CQRS Pattern](https://martinfowler.com/bliki/CQRS.html)

### 最佳实践

- [Domain-Driven Design](https://www.domainlanguage.com/ddd/)
- [Microservices Patterns](https://microservices.io/patterns/index.html)

---

## 📝 总结

### 核心优势

| 优势 | 说明 |
|------|------|
| **原子性保证** | 业务操作与事件发布的原子性 |
| **最终一致性** | 通过调度器保证事件最终被发布 |
| **高度复用** | 所有微服务共享通用框架 |
| **数据库无关** | 支持多种数据库实现 |
| **微服务独立** | 每个微服务独立的 outbox 和调度器 |
| **易于使用** | 简单的 API，开箱即用 |
| **可扩展** | 支持自定义实现和扩展 |

### 职责划分

| 组件 | 位置 | 职责 | 业务微服务需要做什么 |
|------|------|------|---------------------|
| **OutboxEvent** | jxt-core | 领域模型 | ❌ 无需实现 |
| **OutboxRepository** | jxt-core | 仓储接口 | ❌ 无需实现 |
| **OutboxPublisher** | jxt-core | 发布逻辑 | ❌ 无需实现 |
| **OutboxScheduler** | jxt-core | 调度逻辑 | ❌ 无需实现 |
| **GORM 适配器** | jxt-core | GORM 实现 | ❌ 无需实现（可选使用） |
| **TopicMapper** | 业务微服务 | Topic 映射 | ✅ **需要实现** |
| **配置** | 业务微服务 | 调度器配置 | ✅ **需要提供**（可选） |
| **启动** | 业务微服务 | 组装和启动 | ✅ **需要启动** |

### 下一步

1. **实施 jxt-core Outbox 框架**
2. **迁移 evidence-management 微服务**
3. **迁移其他微服务**
4. **完善监控和告警**
5. **编写单元测试和集成测试**

---

**文档版本**：v1.0
**最后更新**：2025-10-19
**维护者**：jxt-core 团队


