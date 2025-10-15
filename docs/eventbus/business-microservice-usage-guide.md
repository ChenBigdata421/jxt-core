# 业务微服务使用 jxt-core 统一 EventBus 指南

## 📋 概述

本文档以 **evidence-management** 项目为例，展示如何使用 jxt-core 的**统一 EventBus 接口**。新的设计将基础功能和企业特性合并为一个 EventBus 接口，业务微服务可以根据实际需要灵活启用企业特性。

### 🔄 设计理念

- **统一接口**：基础功能和企业特性合并为一个 EventBus 接口
- **灵活配置**：企业特性通过配置文件灵活启用/禁用
- **渐进式增强**：从基础功能开始，按需启用企业特性
- **向后兼容**：现有代码可以平滑迁移到新接口

## 🏗️ 使用 jxt-core 统一 EventBus 的架构

```
evidence-management 使用 jxt-core 统一 EventBus 架构
├── command 模块：事件发布侧
│   ├── 聚合根：生成领域事件
│   ├── 应用服务：使用 Outbox 模式
│   ├── Domain 层：EventPublisher 接口定义
│   └── Infrastructure 层：EventPublisher 实现 → jxt-core EventBus → Kafka
├── query 模块：事件订阅侧
│   ├── 事件处理器：处理领域事件
│   ├── 读模型更新：更新查询数据库
│   ├── Domain 层：EventSubscriber 接口定义
│   └── Infrastructure 层：EventSubscriber 实现 → jxt-core EventBus → Kafka
└── shared 模块：jxt-core EventBus 初始化和配置 + 领域事件定义
```

## 🚀 快速开始

### 项目结构概览

evidence-management 项目使用 jxt-core 统一 EventBus 的架构：

```
evidence-management/
├── command/                    # 命令侧（写操作）
│   ├── cmd/api/server.go      # 服务启动，初始化 EventBus
│   ├── internal/
│   │   ├── domain/
│   │   │   ├── aggregate/     # 聚合根，生成领域事件
│   │   │   └── event/
│   │   │       └── publisher/ # 发布接口定义（Domain 层）
│   │   ├── application/
│   │   │   └── service/       # 应用服务，使用 Outbox 模式
│   │   └── infrastructure/
│   │       └── eventbus/      # 发布器实现（Infrastructure 层）
├── query/                     # 查询侧（读操作）
│   ├── cmd/api/server.go      # 服务启动，初始化 EventBus
│   ├── internal/
│   │   ├── application/
│   │   │   └── eventhandler/  # 事件处理器 + 订阅接口定义
│   │   └── infrastructure/
│   │       └── eventbus/      # 订阅器实现（Infrastructure 层）
└── shared/                    # 共享模块
    ├── common/eventbus/       # EventBus 初始化和配置
    └── domain/event/          # 领域事件定义（Event 接口和实现）
```

---

## 📤 一、Shared 模块（共享组件）

### 1. EventBus 配置

#### Command 模块配置（仅发布端）

<augment_code_snippet path="evidence-management/command/config/eventbus.yaml" mode="EXCERPT">
````yaml
# Command 模块 EventBus 配置（仅发布端）
eventbus:
  serviceName: "evidence-management-command"
  type: "kafka"
  kafka:
    brokers:
      - "localhost:9092"

  # 发布端配置
  publisher:
    healthCheck:
      enabled: true
      interval: "1m"
    reconnect:
      enabled: true
      maxRetries: 5
      backoffInterval: "5s"

  # 订阅端配置（Command 模块不需要，全部禁用）
  subscriber:
    backlogDetection:
      enabled: false
    aggregateProcessor:
      enabled: false
    rateLimit:
      enabled: false
    recoveryMode:
      enabled: false

  # 健康检查配置
  healthCheck:
    enabled: true
    interval: "1m"
    timeout: "30s"
````
</augment_code_snippet>

#### Query 模块配置（仅订阅端）

<augment_code_snippet path="evidence-management/query/config/eventbus.yaml" mode="EXCERPT">
````yaml
# Query 模块 EventBus 配置（仅订阅端）
eventbus:
  serviceName: "evidence-management-query"
  type: "kafka"
  kafka:
    brokers:
      - "localhost:9092"

  # 发布端配置（Query 模块不需要，全部禁用）
  publisher:
    healthCheck:
      enabled: false
    reconnect:
      enabled: false

  # 订阅端配置
  subscriber:
    backlogDetection:
      enabled: true
      maxLagThreshold: 100
      checkInterval: "30s"
    aggregateProcessor:
      enabled: true
      cacheSize: 500
      maxConcurrency: 10
    rateLimit:
      enabled: true
      limit: 1000
      burst: 1000
    recoveryMode:
      enabled: true
      autoDetection: true

  # 健康检查配置
  healthCheck:
    enabled: true
    interval: "1m"
    timeout: "30s"
````
</augment_code_snippet>

### 2. 初始化 jxt-core EventBus

<augment_code_snippet path="evidence-management/shared/common/eventbus/eventbus_setup.go" mode="EXCERPT">
````go
package eventbus

import (
    "context"
    "log"
    "time"
    "github.com/ChenBigdata421/jxt-core/sdk/config"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// SetupEventBusForCommand 为 Command 模块初始化 jxt-core 事件总线（仅发布端）
func SetupEventBusForCommand() (eventbus.EventBus, error) {
    // 创建配置
    cfg := &eventbus.EventBusConfig{
        Type: "kafka",
        Kafka: eventbus.KafkaConfig{
            Brokers: []string{"localhost:9092"},
        },
        Enterprise: eventbus.EnterpriseConfig{
            // 启用发布端企业特性
            Publisher: eventbus.PublisherEnterpriseConfig{
                RetryPolicy: eventbus.RetryPolicyConfig{
                    Enabled: true,
                    MaxRetries: 3,
                },
                PublishCallback: eventbus.PublishCallbackConfig{
                    Enabled: true,
                },
                MessageFormatter: eventbus.MessageFormatterConfig{
                    Enabled: true,
                    Type: "json",
                },
            },
            // Command 模块不需要订阅功能，禁用订阅端配置
            Subscriber: eventbus.SubscriberEnterpriseConfig{
                BacklogDetection: eventbus.BacklogDetectionConfig{
                    Enabled: false,
                },
                AggregateProcessor: eventbus.AggregateProcessorConfig{
                    Enabled: false,
                },
                RateLimit: eventbus.RateLimitConfig{
                    Enabled: false,
                },
            },
            // 统一企业特性
            HealthCheck: eventbus.HealthCheckConfig{
                Enabled: true,
                Interval: 30 * time.Second,
            },
        },
    }

    // 创建事件总线
    bus, err := eventbus.NewEventBus(cfg)
    if err != nil {
        return nil, err
    }

    // 设置业务特定的组件（仅发布端相关）
    bus.SetErrorHandler(&EvidenceErrorHandler{})
    bus.SetMessageFormatter(&EvidenceMessageFormatter{})

    // 注册发布端相关回调
    bus.RegisterHealthCheckCallback(handlePublisherHealthCheck)
    bus.RegisterReconnectCallback(handlePublisherReconnect)

    return bus, nil
}

// SetupEventBusForQuery 为 Query 模块初始化 jxt-core 事件总线（仅订阅端）
func SetupEventBusForQuery() (eventbus.EventBus, error) {
    // 创建配置
    cfg := &eventbus.EventBusConfig{
        Type: "kafka",
        Kafka: eventbus.KafkaConfig{
            Brokers: []string{"localhost:9092"},
        },
        Enterprise: eventbus.EnterpriseConfig{
            // Query 模块不需要发布功能，禁用发布端配置
            Publisher: eventbus.PublisherEnterpriseConfig{
                RetryPolicy: eventbus.RetryPolicyConfig{
                    Enabled: false,
                },
                PublishCallback: eventbus.PublishCallbackConfig{
                    Enabled: false,
                },
            },
            // 启用订阅端企业特性
            Subscriber: eventbus.SubscriberEnterpriseConfig{
                BacklogDetection: eventbus.BacklogDetectionConfig{
                    Enabled: true,
                    MaxLagThreshold: 100,
                    CheckInterval: 30 * time.Second,
                },
                AggregateProcessor: eventbus.AggregateProcessorConfig{
                    Enabled: true,
                    MaxWorkers: 10,
                    BufferSize: 500,
                },
                RateLimit: eventbus.RateLimitConfig{
                    Enabled: true,
                    RateLimit: 1000,
                    BurstSize: 1000,
                },
                RecoveryMode: eventbus.RecoveryModeConfig{
                    Enabled: true,
                    AutoEnable: true,
                },
            },
            // 统一企业特性
            HealthCheck: eventbus.HealthCheckConfig{
                Enabled: true,
                Interval: 30 * time.Second,
            },
        },
    }

    // 创建事件总线
    bus, err := eventbus.NewEventBus(cfg)
    if err != nil {
        return nil, err
    }

    // 设置业务特定的组件（仅订阅端相关）
    bus.SetErrorHandler(&EvidenceErrorHandler{})

    // 注册订阅端相关回调
    bus.RegisterBacklogCallback(handleBacklogChange)
    bus.RegisterHealthCheckCallback(handleSubscriberHealthCheck)

    return bus, nil
}

// 业务特定的消息路由器
type EvidenceMessageRouter struct{}

func (r *EvidenceMessageRouter) Route(ctx context.Context, topic string, message []byte) (string, error) {
    // 根据业务逻辑路由消息到不同的处理器
    return topic, nil
}

// 业务特定的错误处理器
type EvidenceErrorHandler struct{}

func (h *EvidenceErrorHandler) HandleError(ctx context.Context, err error, topic string, message []byte) error {
    log.Printf("Evidence management error in topic %s: %v", topic, err)
    // 业务特定的错误处理逻辑
    return nil
}
````
</augment_code_snippet>

### 3. 领域事件定义

<augment_code_snippet path="evidence-management/shared/domain/event/domain_event.go" mode="EXCERPT">
````go
// DomainEvent 实现了Event接口
type DomainEvent struct {
    TenantId      string    `json:"tenantId"`
    EventID       string    `json:"eventId"`
    EventType     string    `json:"eventType"`
    OccurredAt    time.Time `json:"occurredAt"`
    Version       int       `json:"version"`
    AggregateID   string    `json:"aggregateId"`
    AggregateType string    `json:"aggregateType"`
    Payload       []byte    `json:"payload"`
}

func NewDomainEvent(eventType string, aggregateID interface{}, aggregateType string, payload []byte) *DomainEvent {
    eventID := uuid.Must(uuid.NewV7()).String()
    return &DomainEvent{
        EventID:       eventID,
        EventType:     eventType,
        OccurredAt:    time.Now(),
        Version:       1,
        AggregateID:   convertAggregateIDToString(aggregateID),
        AggregateType: aggregateType,
        Payload:       payload,
    }
}
````
</augment_code_snippet>

### 4. 事件主题定义

<augment_code_snippet path="evidence-management/shared/common/eventbus/type.go" mode="EXCERPT">
````go
const (
    MediaEventTopic                = "evidence_media_topic"
    EnforcementTypeEventTopic      = "evidence_enforcement_type_topic"
    ArchiveEventTopic              = "evidence_archive_topic"
    ArchiveMediaRelationEventTopic = "evidence_archive_media_relation_topic"
    RecordEventTopic               = "evidence_record_topic"
    HealthCheckTopic               = "health_check_topic"
)
````
</augment_code_snippet>

---

## 📤 二、Command 模块（发布端）

### 1. 服务启动和初始化

<augment_code_snippet path="evidence-management/command/cmd/api/server.go" mode="EXCERPT">
````go
func init() {
    // 初始化基础组件
    logger.Setup()            // 初始化zaplogger
    database.MasterDbSetup()  // 初始化主数据库
    database.CommandDbSetup() // 初始化命令数据库
    storage.Setup()           // 添加内存队列的创建

    // 初始化 jxt-core EventBus（仅发布端）
    bus, err := eventbus.SetupEventBusForCommand()
    if err != nil {
        log.Fatal("Failed to setup EventBus for Command:", err)
    }

    // 启动事件总线
    ctx := context.Background()
    if err := bus.Start(ctx); err != nil {
        log.Fatal("Failed to start EventBus:", err)
    }

    // 启动健康检查
    if err := bus.StartHealthCheck(ctx); err != nil {
        log.Fatal("Failed to start health check:", err)
    }

    // 注册到依赖注入容器
    di.Provide(func() eventbus.EventBus {
        return bus
    })
}
````
</augment_code_snippet>

### 2. Domain 层接口定义

#### 2.1 发布接口定义（Domain 层）

<augment_code_snippet path="evidence-management/command/internal/domain/event/publisher/publisher.go" mode="EXCERPT">
````go
package publisher

import (
    "context"
    "jxt-evidence-system/evidence-management/shared/domain/event"
)

// EventPublisher 定义领域事件发布的接口（Domain 层）
type EventPublisher interface {
    Publish(ctx context.Context, topic string, event event.Event) error
    RegisterReconnectCallback(callback func(ctx context.Context) error) error
}
````
</augment_code_snippet>

### 3. Infrastructure 层实现

#### 3.1 发布器实现（Infrastructure 层）

<augment_code_snippet path="evidence-management/command/internal/infrastructure/eventbus/eventbus_publisher.go" mode="EXCERPT">
````go
package eventbus

import (
    "context"
    "fmt"
    "time"
    "jxt-evidence-system/evidence-management/command/internal/domain/event/publisher"
    "jxt-evidence-system/evidence-management/shared/common/di"
    "jxt-evidence-system/evidence-management/shared/domain/event"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// 依赖注入注册
func init() {
    registrations = append(registrations, registerEventPublisherDependencies)
}

func registerEventPublisherDependencies() {
    if err := di.Provide(func(bus eventbus.EventBus) publisher.EventPublisher {
        return NewEventPublisher(bus)
    }); err != nil {
        logger.Fatalf("failed to provide EventPublisher: %v", err)
    }
}

// EventPublisher Infrastructure 层实现，使用 jxt-core EventBus
type EventPublisher struct {
    eventBus eventbus.EventBus
}

func NewEventPublisher(bus eventbus.EventBus) *EventPublisher {
    return &EventPublisher{
        eventBus: bus,
    }
}

// Publish 实现 Domain 接口：发布事件到 EventBus
func (p *EventPublisher) Publish(ctx context.Context, topic string, event event.Event) error {
    if event == nil {
        return fmt.Errorf("event cannot be nil")
    }

    payload, err := event.MarshalJSON()
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }

    // 使用 jxt-core EventBus 的企业发布功能
    opts := eventbus.PublishOptions{
        AggregateID: event.GetAggregateID(),
        Metadata: map[string]string{
            "eventType":   event.GetEventType(),
            "tenantId":    event.GetTenantId(),
            "contentType": "application/json",
            "version":     event.GetVersion(),
        },
        Timeout: 30 * time.Second,
    }

    return p.eventBus.PublishWithOptions(ctx, topic, payload, opts)
}

// RegisterReconnectCallback 实现 Domain 接口：注册重连回调
func (p *EventPublisher) RegisterReconnectCallback(callback func(ctx context.Context) error) error {
    return p.eventBus.RegisterReconnectCallback(callback)
}
````
</augment_code_snippet>





### 4. 聚合根中生成事件

<augment_code_snippet path="evidence-management/command/internal/domain/aggregate/media/media.go" mode="EXCERPT">
````go
// Media 媒体聚合根
type Media struct {
    ID                    valueobject.MediaID
    MediaName             string
    MediaCate             int
    IsNonEnforcementMedia int
    IsLocked              int
    ControlBy             models.ControlBy
    ModelTime             models.ModelTime
    events                []event.Event // 聚合根中的事件列表
}

// CreateMedia 创建媒体业务方法
func (e *Media) CreateMedia() error {
    domainEvent, err := e.createMediaUploadedEvent()
    if err != nil {
        return fmt.Errorf("生成媒体创建事件失败: %w", err)
    }
    e.AddEvent(domainEvent)
    return nil
}

// AddEvent 添加事件到聚合根
func (e *Media) AddEvent(event event.Event) {
    e.events = append(e.events, event)
}

// Events 获取聚合根中的所有事件
func (e *Media) Events() []event.Event {
    return e.events
}

// ClearEvents 清空聚合根中的事件
func (e *Media) ClearEvents() {
    e.events = []event.Event{}
}
````
</augment_code_snippet>

### 5. 具体事件载荷定义

<augment_code_snippet path="evidence-management/shared/domain/event/media_events.go" mode="EXCERPT">
````go
// media事件类型定义
const (
    EventTypeMediaUploaded         = "MediaUploaded"
    EventTypeMediaBasicInfoUpdated = "MediaBasicInfoUpdated"
    EventTypeMediaCommentsUpdated  = "MediaCommentsUpdated"
    EventTypeMediaDeleted          = "MediaDeleted"
)

// MediaUploadedPayload 媒体上传事件载荷
type MediaUploadedPayload struct {
    MediaID              string           `json:"mediaId"`
    MediaName            string           `json:"mediaName"`
    MediaCate            int              `json:"mediaCate"`
    IsNonEnforcementMedia int             `json:"isNonEnforcementMedia"`
    IsLocked             int              `json:"isLocked"`
    ControlBy            models.ControlBy `json:"controlBy"`
    ModelTime            models.ModelTime `json:"modelTime"`
}
````
</augment_code_snippet>

### 6. 应用服务中使用 Outbox 模式

<augment_code_snippet path="evidence-management/command/internal/application/service/media.go" mode="EXCERPT">
````go
// CreateMedia 创建媒体应用服务方法
func (e *MediaApplicationService) CreateMedia(ctx context.Context, cmd *command.CreateMediaCommand) (valueobject.MediaID, error) {
    // 1. 创建聚合根并生成事件
    media, err := aggregate.NewMedia(cmd.MediaName, cmd.MediaCate, cmd.CreateBy)
    if err != nil {
        return valueobject.MediaID{}, err
    }

    if err := media.CreateMedia(); err != nil {
        return valueobject.MediaID{}, err
    }

    // 2. 使用事务确保数据一致性
    err = transaction.RunInTransaction(ctx, e.txManager, func(tx transaction.Transaction) error {
        // 保存媒体数据
        if err := e.repo.CreateInTx(ctx, tx, media); err != nil {
            return fmt.Errorf("创建媒体的数据库持久化失败: %w", err)
        }

        // 在同一事务中保存事件到outbox表
        for _, event := range media.Events() {
            // 设置租户ID
            tenantID := ctx.Value(global.TenantIDKey)
            if tenantID != nil {
                event.SetTenantId(tenantID.(string))
            }

            if err := e.outboxRepo.SaveInTx(ctx, tx, event); err != nil {
                return fmt.Errorf("保存创建媒体的事件到outbox失败: %w", err)
            }
        }

        media.ClearEvents()
        return nil
    })

    // 3. 事务外立即发布事件（快速最终一致性）
    go e.publishOutboxEventsImmediately(ctx, eventIDs...)

    return media.ID, nil
}
````
</augment_code_snippet>

---

## 📥 三、Query 模块（订阅端）

### 1. 服务启动和初始化

<augment_code_snippet path="evidence-management/query/cmd/api/server.go" mode="EXCERPT">
````go
func init() {
    // 初始化基础组件
    logger.Setup()             // 初始化zaplogger
    database.MasterDbSetup()   // 初始化主数据库
    database.CommandDbSetup()  // 初始化命令数据库
    database.QueryDbSetup()    // 初始化查询数据库
    storage.Setup()            // 内存队列的创建

    // 初始化 jxt-core EventBus（仅订阅端）
    bus, err := eventbus.SetupEventBusForQuery()
    if err != nil {
        log.Fatal("Failed to setup EventBus for Query:", err)
    }

    // 启动事件总线
    ctx := context.Background()
    if err := bus.Start(ctx); err != nil {
        log.Fatal("Failed to start EventBus:", err)
    }

    // 启动积压监控
    if err := bus.StartBacklogMonitoring(ctx); err != nil {
        log.Fatal("Failed to start backlog monitoring:", err)
    }

    // 注册到依赖注入容器
    di.Provide(func() eventbus.EventBus {
        return bus
    })

    // 设置事件订阅
    setupEventSubscriptions()
}

func setupEventSubscriptions() error {
    // 订阅 Media 领域事件
    err := di.Invoke(func(mediaHandler *eventhandler.MediaEventHandler, bus eventbus.EventBus) {
        if mediaHandler != nil {
            // 使用 EventBus 的企业订阅功能
            if err := subscribeMediaEventsWithEventBus(bus, mediaHandler); err != nil {
                log.Fatal("Failed to subscribe media events:", err)
            }
        }
    })

    // 订阅 EnforcementType 领域事件
    err = di.Invoke(func(enforcementHandler *eventhandler.EnforcementTypeEventHandler, bus eventbus.EventBus) {
        if enforcementHandler != nil {
            // 使用 EventBus 的企业订阅功能
            if err := subscribeEnforcementTypeEventsWithEventBus(bus, enforcementHandler); err != nil {
                log.Fatal("Failed to subscribe enforcement type events:", err)
            }
        }
    })

    // 订阅 Archive 领域事件
    err = di.Invoke(func(archiveHandler *eventhandler.ArchiveMediaRelationEventHandler, bus eventbus.EventBus) {
        if archiveHandler != nil {
            // 使用 EventBus 的企业订阅功能
            if err := subscribeArchiveEventsWithEventBus(bus, archiveHandler); err != nil {
                log.Fatal("Failed to subscribe archive events:", err)
            }
        }
    })

    return nil
}
````
</augment_code_snippet>

### 2. Domain 层接口定义

#### 2.1 订阅接口定义（Domain 层）

<augment_code_snippet path="evidence-management/query/internal/application/eventhandler/subscriber.go" mode="EXCERPT">
````go
package eventhandler

import (
    "time"
    "github.com/ThreeDotsLabs/watermill/message"
)

// EventSubscriber 定义领域事件订阅的接口（Domain 层）
type EventSubscriber interface {
    Subscribe(topic string, handler func(msg *message.Message) error, timeout time.Duration) error
}
````
</augment_code_snippet>

### 3. Infrastructure 层实现

#### 3.1 订阅器实现（Infrastructure 层）

<augment_code_snippet path="evidence-management/query/internal/infrastructure/eventbus/eventbus_subscriber.go" mode="EXCERPT">
````go
package eventbus

import (
    "context"
    "time"
    "jxt-evidence-system/evidence-management/query/internal/application/eventhandler"
    "jxt-evidence-system/evidence-management/shared/common/di"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
    "github.com/ThreeDotsLabs/watermill/message"
)

// 依赖注入注册
func init() {
    registrations = append(registrations, registerEventSubscriberDependencies)
}

func registerEventSubscriberDependencies() {
    if err := di.Provide(func(bus eventbus.EventBus) eventhandler.EventSubscriber {
        return NewEventSubscriber(bus)
    }); err != nil {
        logger.Fatalf("failed to provide EventSubscriber: %v", err)
    }
}

// EventSubscriber 使用 jxt-core EventBus 的事件订阅器
type EventSubscriber struct {
    eventBus eventbus.EventBus
}

func NewEventSubscriber(bus eventbus.EventBus) *EventSubscriber {
    return &EventSubscriber{
        eventBus: bus,
    }
}

// Subscribe 订阅事件（基础接口兼容）
func (s *EventSubscriber) Subscribe(topic string, handler func(msg *message.Message) error, timeout time.Duration) error {
    // 包装处理器以适配 EventBus
    wrappedHandler := func(ctx context.Context, message []byte) error {
        msg := message.NewMessage("", message)
        return handler(msg)
    }

    // 使用基础订阅功能
    return s.eventBus.Subscribe(context.Background(), topic, wrappedHandler)
}

// SubscribeWithOptions 使用企业特性选项订阅
func (s *EventSubscriber) SubscribeWithOptions(ctx context.Context, topic string, handler func(ctx context.Context, message []byte) error, opts eventbus.SubscribeOptions) error {
    return s.eventBus.SubscribeWithOptions(ctx, topic, handler, opts)
}
````
</augment_code_snippet>

### 4. 事件处理器与 EventBus 集成

#### 4.1 Media 事件订阅集成

<augment_code_snippet path="evidence-management/query/internal/infrastructure/eventbus/media_subscription.go" mode="EXCERPT">
````go
// subscribeMediaEventsWithEventBus 使用 EventBus 订阅媒体事件
func subscribeMediaEventsWithEventBus(bus eventbus.EventBus, mediaHandler *eventhandler.MediaEventHandler) error {
    // 配置媒体事件的企业订阅选项
    opts := eventbus.SubscribeOptions{
        UseAggregateProcessor: true,  // 启用聚合处理器，确保同一媒体的事件顺序处理
        ProcessingTimeout:     60 * time.Second,
        RateLimit:            500,    // 每秒处理500条消息
        RateBurst:            1000,   // 突发处理1000条消息
        RetryPolicy: eventbus.RetryPolicy{
            MaxRetries:      3,
            InitialInterval: 1 * time.Second,
            MaxInterval:     30 * time.Second,
            Multiplier:      2.0,
        },
        DeadLetterEnabled: true,      // 启用死信队列
    }

    // 包装现有的 MediaEventHandler.handleMediaEvent 方法
    handler := func(ctx context.Context, message []byte) error {
        // 将 []byte 转换为 watermill.Message 以兼容现有处理器
        msg := message.NewMessage("", message)
        return mediaHandler.handleMediaEvent(msg)
    }

    // 订阅媒体事件主题
    return bus.SubscribeWithOptions(context.Background(), eventbus.MediaEventTopic, handler, opts)
}
````
</augment_code_snippet>

#### 4.2 EnforcementType 事件订阅集成

<augment_code_snippet path="evidence-management/query/internal/infrastructure/eventbus/enforcement_subscription.go" mode="EXCERPT">
````go
// subscribeEnforcementTypeEventsWithEventBus 使用 EventBus 订阅执法类型事件
func subscribeEnforcementTypeEventsWithEventBus(bus eventbus.EventBus, enforcementHandler *eventhandler.EnforcementTypeEventHandler) error {
    // 配置执法类型事件的企业订阅选项
    opts := eventbus.SubscribeOptions{
        UseAggregateProcessor: true,  // 启用聚合处理器
        ProcessingTimeout:     30 * time.Second,
        RateLimit:            200,    // 执法类型事件相对较少
        RateBurst:            500,
        RetryPolicy: eventbus.RetryPolicy{
            MaxRetries:      3,
            InitialInterval: 1 * time.Second,
            MaxInterval:     30 * time.Second,
            Multiplier:      2.0,
        },
        DeadLetterEnabled: true,
    }

    // 包装现有的 EnforcementTypeEventHandler.handleEnforcementTypeEvent 方法
    handler := func(ctx context.Context, message []byte) error {
        // 将 []byte 转换为 watermill.Message 以兼容现有处理器
        msg := message.NewMessage("", message)
        return enforcementHandler.handleEnforcementTypeEvent(msg)
    }

    // 订阅执法类型事件主题
    return bus.SubscribeWithOptions(context.Background(), eventbus.EnforcementTypeEventTopic, handler, opts)
}
````
</augment_code_snippet>

#### 4.3 Archive 事件订阅集成

<augment_code_snippet path="evidence-management/query/internal/infrastructure/eventbus/archive_subscription.go" mode="EXCERPT">
````go
// subscribeArchiveEventsWithEventBus 使用 EventBus 订阅档案事件
func subscribeArchiveEventsWithEventBus(bus eventbus.EventBus, archiveHandler *eventhandler.ArchiveMediaRelationEventHandler) error {
    // 配置档案事件的企业订阅选项
    opts := eventbus.SubscribeOptions{
        UseAggregateProcessor: true,  // 启用聚合处理器
        ProcessingTimeout:     45 * time.Second,
        RateLimit:            300,    // 档案事件处理需要更多资源
        RateBurst:            600,
        RetryPolicy: eventbus.RetryPolicy{
            MaxRetries:      3,
            InitialInterval: 1 * time.Second,
            MaxInterval:     30 * time.Second,
            Multiplier:      2.0,
        },
        DeadLetterEnabled: true,
    }

    // 包装现有的 ArchiveMediaRelationEventHandler.handleArchiveMediaRelationEvent 方法
    handler := func(ctx context.Context, message []byte) error {
        // 将 []byte 转换为 watermill.Message 以兼容现有处理器
        msg := message.NewMessage("", message)
        return archiveHandler.handleArchiveMediaRelationEvent(msg)
    }

    // 订阅档案事件主题
    return bus.SubscribeWithOptions(context.Background(), eventbus.ArchiveEventTopic, handler, opts)
}
````
</augment_code_snippet>

### 5. 现有事件处理器结构

evidence-management 项目已经定义了完整的事件处理器，我们只需要将它们与 EventBus 集成：

#### 5.1 MediaEventHandler 结构

<augment_code_snippet path="evidence-management/query/internal/application/eventhandler/media_event_handler.go" mode="EXCERPT">
````go
// MediaEventHandler 媒体事件处理器（已存在）
type MediaEventHandler struct {
    Subscriber           EventSubscriber
    repo                 repository.MediaReadModelRepository
    userInfoService      infrastructure_service.UserInfoService
    orgInfoService       infrastructure_service.OrganizationInfoService
    lawcameraInfoService infrastructure_service.LawcameraInfoService
}

// handleMediaEvent 处理媒体事件（已存在的方法）
func (h *MediaEventHandler) handleMediaEvent(msg *message.Message) error {
    // Step 1: 反序列化为领域事件结构体
    domainEvent := &event.DomainEvent{}
    err := domainEvent.UnmarshalJSON(msg.Payload)
    if err != nil {
        return fmt.Errorf("failed to unmarshal media event: %w", err)
    }

    // Step 2: 取出领域事件的租户id
    tenantID := domainEvent.GetTenantId()
    if tenantID == "" {
        return fmt.Errorf("租户ID不能为空")
    }

    // Step 3: 把租户id记录到context，传给repo
    ctx := context.WithValue(context.Background(), global.TenantIDKey, tenantID)

    // Step 4: 根据事件类型进行处理
    eventType := domainEvent.GetEventType()
    switch eventType {
    // 基础事件
    case event.EventTypeMediaUploaded:
        return h.handleMediaUploadedEvent(ctx, domainEvent)
    case event.EventTypeMediaBasicInfoUpdated:
        return h.handleMediaBasicInfoUpdatedEvent(ctx, domainEvent)
    case event.EventTypeMediaCommentsUpdated:
        return h.handleMediaCommentsUpdatedEvent(ctx, domainEvent)
    case event.EventTypeMediaDeleted:
        return h.handleMediaDeletedEvent(ctx, domainEvent)
    // ... 其他事件类型
    default:
        logger.Error("未知的媒体事件类型", "eventType", eventType)
        return fmt.Errorf("unknown media event type: %s", eventType)
    }
}
````
</augment_code_snippet>

#### 5.2 EnforcementTypeEventHandler 结构

<augment_code_snippet path="evidence-management/query/internal/application/eventhandler/enforcement_type_event_handler.go" mode="EXCERPT">
````go
// EnforcementTypeEventHandler 执法类型事件处理器（已存在）
type EnforcementTypeEventHandler struct {
    Subscriber EventSubscriber
    repo       repository.EnforcementTypeReadModelRepository
}

// handleEnforcementTypeEvent 处理执法类型事件（已存在的方法）
func (h *EnforcementTypeEventHandler) handleEnforcementTypeEvent(msg *message.Message) error {
    // Step 1: 反序列化为领域事件结构体
    domainEvent := &event.DomainEvent{}
    err := domainEvent.UnmarshalJSON(msg.Payload)
    if err != nil {
        return fmt.Errorf("failed to unmarshal enforcement type event: %w", err)
    }

    // Step 2: 取出领域事件的租户id
    tenantID := domainEvent.GetTenantId()
    if tenantID == "" {
        return fmt.Errorf("租户ID不能为空")
    }

    // Step 3: 把租户id记录到context，传给repo
    ctx := context.WithValue(context.Background(), global.TenantIDKey, tenantID)

    // Step 4: 根据事件类型进行处理
    eventType := domainEvent.GetEventType()
    switch eventType {
    case event.EventTypeEnforcementTypeCreated:
        return h.handleEnforcementTypeCreatedEvent(ctx, domainEvent)
    case event.EventTypeEnforcementTypeUpdated:
        return h.handleEnforcementTypeUpdatedEvent(ctx, domainEvent)
    case event.EventTypeEnforcementTypeDeleted:
        return h.handleEnforcementTypeDeletedEvent(ctx, domainEvent)
    default:
        logger.Error("未知的执法类型事件类型", "eventType", eventType)
        return fmt.Errorf("unknown enforcement type event type: %s", eventType)
    }
}
````
</augment_code_snippet>

### 6. 集成优势

通过将现有的事件处理器与 jxt-core EventBus 集成，evidence-management 项目获得以下优势：

#### 6.1 保持业务逻辑不变
- **现有处理器复用**：MediaEventHandler、EnforcementTypeEventHandler 等处理器的业务逻辑完全保持不变
- **接口兼容性**：通过适配器模式，现有的 `handleMediaEvent` 等方法可以直接使用
- **零业务代码修改**：只需要修改基础设施层的订阅方式

#### 6.2 获得企业级功能
- **聚合处理器**：确保同一媒体/执法类型的事件按顺序处理，避免并发冲突
- **积压检测**：自动检测消息积压，及时发现性能问题
- **流量控制**：智能限流，防止系统过载
- **死信队列**：自动处理失败消息，提高系统可靠性

#### 6.3 差异化配置
- **Media 事件**：高并发处理（500 msg/s），长超时（60s）
- **EnforcementType 事件**：中等并发（200 msg/s），标准超时（30s）
- **Archive 事件**：资源密集型处理（300 msg/s），中等超时（45s）

#### 6.4 监控和可观测性
- **统一监控**：所有事件处理的统计信息集中管理
- **健康检查**：自动监控订阅器状态
- **回调机制**：支持积压告警、恢复模式通知等


## 🎯 使用 jxt-core 统一 EventBus 的核心优势

### 1. 架构对比

**传统自定义实现**：
- 需要自己实现 KafkaPublisherManager/KafkaSubscriberManager
- 手动处理连接管理、健康检查、重连逻辑
- 缺乏统一的监控和管理能力

**jxt-core EventBus**：
- 开箱即用的企业特性
- 统一的配置和管理接口
- 内置的监控、积压检测、恢复模式等企业级功能

### 2. 核心功能优势

#### 发布端优势
- **统一健康检查**：自动监控发布器状态
- **智能重连机制**：自动处理连接断开和恢复
- **发布统计监控**：详细的发布成功率、延迟等指标
- **回调机制**：支持发布结果回调、重连回调等

#### 订阅端优势
- **积压检测**：自动检测消息积压并触发告警
- **恢复模式**：在积压情况下自动切换处理策略
- **聚合处理器**：确保同一聚合的事件按顺序处理
- **流量控制**：内置限流和背压机制
- **死信队列**：自动处理失败消息

#### 运维优势
- **统一配置**：通过配置文件统一管理所有参数
- **可观测性**：丰富的指标和日志输出
- **故障恢复**：自动故障检测和恢复机制
- **性能优化**：内置的性能优化策略

### 3. 业务价值

1. **开发效率提升**：减少基础设施代码编写，专注业务逻辑
2. **运维成本降低**：统一的监控和管理，减少运维复杂度
3. **系统可靠性**：经过验证的企业级功能，提高系统稳定性
4. **扩展性增强**：支持多种企业特性，满足复杂业务需求

## 📚 总结

通过 evidence-management 项目的示例，本文档展示了如何在业务微服务中使用 jxt-core 统一 EventBus：

### 核心实现要点

- **Command 模块**：使用 EventBus 发布事件，支持高级发布选项和监控
- **Query 模块**：使用 EventBus 订阅事件，支持聚合处理、流量控制等企业特性
- **配置管理**：通过统一配置文件管理所有 EventBus 参数
- **适配器模式**：保持业务接口不变，底层使用 EventBus

### 技术优势

- **企业级功能**：积压检测、恢复模式、健康检查等
- **高可用性**：自动重连、故障恢复、死信队列
- **可观测性**：丰富的监控指标和回调机制
- **性能优化**：聚合处理、流量控制、批量处理

这种架构为微服务提供了可靠、高效的事件驱动基础设施，支持高并发、高可用的分布式系统。