# 业务微服务使用 jxt-core AdvancedEventBus 指南

## 📋 概述

本文档以 **evidence-management** 项目为例，详细说明业务微服务如何使用 jxt-core 的 AdvancedEventBus 进行事件发布和订阅。通过具体的代码示例，展示如何在 evidence-management 的 command 模块和 query 模块中集成和使用 jxt-core 的高级事件总线功能。

## 🏗️ 使用 jxt-core AdvancedEventBus 的架构

```
evidence-management 使用 jxt-core AdvancedEventBus 架构
├── command 模块：事件发布侧
│   ├── 聚合根：生成领域事件
│   ├── 应用服务：使用 Outbox 模式
│   └── EventPublisher 适配器 → jxt-core AdvancedEventBus → Kafka
├── query 模块：事件订阅侧
│   ├── 事件处理器：处理领域事件
│   ├── 读模型更新：更新查询数据库
│   └── EventSubscriber 适配器 → jxt-core AdvancedEventBus → Kafka
└── shared 模块：jxt-core AdvancedEventBus 初始化和配置
```

## 🚀 快速开始

### 1. 项目结构

evidence-management 项目使用 jxt-core AdvancedEventBus 的架构：

```
evidence-management/
├── command/                    # 命令侧（写操作）
│   ├── cmd/api/server.go      # 服务启动，初始化 AdvancedEventBus
│   ├── internal/
│   │   ├── domain/
│   │   │   ├── aggregate/     # 聚合根，生成领域事件
│   │   │   └── event/         # 事件定义
│   │   ├── application/
│   │   │   └── service/       # 应用服务，使用 Outbox 模式
│   │   └── infrastructure/
│   │       └── eventbus/      # AdvancedEventBus 适配器
├── query/                     # 查询侧（读操作）
│   ├── cmd/api/server.go      # 服务启动，初始化 AdvancedEventBus
│   ├── internal/
│   │   ├── application/
│   │   │   └── eventhandler/  # 事件处理器
│   │   └── infrastructure/
│   │       └── eventbus/      # AdvancedEventBus 适配器
└── shared/                    # 共享模块
    ├── common/eventbus/       # AdvancedEventBus 初始化和配置
    └── domain/event/          # 领域事件定义
```

### 2. AdvancedEventBus 配置

#### Command 模块配置（仅发布端）

<augment_code_snippet path="evidence-management/command/config/eventbus.yaml" mode="EXCERPT">
````yaml
# Command 模块 AdvancedEventBus 配置（仅发布端）
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
# Query 模块 AdvancedEventBus 配置（仅订阅端）
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

## 📤 发布侧使用示例（Command 模块）

### 1. 初始化 jxt-core AdvancedEventBus

<augment_code_snippet path="evidence-management/shared/common/eventbus/advanced_setup.go" mode="EXCERPT">
````go
package eventbus

import (
    "context"
    "log"
    "time"
    "github.com/ChenBigdata421/jxt-core/sdk/config"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// SetupAdvancedEventBusForCommand 为 Command 模块初始化 jxt-core 高级事件总线（仅发布端）
func SetupAdvancedEventBusForCommand() (eventbus.AdvancedEventBus, error) {
    // 加载配置
    cfg := config.GetDefaultAdvancedEventBusConfig()
    cfg.ServiceName = "evidence-management-command"
    cfg.Type = "kafka"
    cfg.Kafka.Brokers = []string{"localhost:9092"}

    // 启用发布端高级功能
    cfg.Publisher.HealthCheck.Enabled = true
    cfg.Publisher.HealthCheck.Interval = 1 * time.Minute
    cfg.Publisher.Reconnect.Enabled = true
    cfg.Publisher.Reconnect.MaxRetries = 5
    cfg.Publisher.Reconnect.BackoffInterval = 5 * time.Second

    // Command 模块不需要订阅功能，禁用订阅端配置
    cfg.Subscriber.BacklogDetection.Enabled = false
    cfg.Subscriber.AggregateProcessor.Enabled = false
    cfg.Subscriber.RateLimit.Enabled = false

    // 创建高级事件总线
    bus, err := eventbus.NewKafkaAdvancedEventBus(cfg)
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

// SetupAdvancedEventBusForQuery 为 Query 模块初始化 jxt-core 高级事件总线（仅订阅端）
func SetupAdvancedEventBusForQuery() (eventbus.AdvancedEventBus, error) {
    // 加载配置
    cfg := config.GetDefaultAdvancedEventBusConfig()
    cfg.ServiceName = "evidence-management-query"
    cfg.Type = "kafka"
    cfg.Kafka.Brokers = []string{"localhost:9092"}

    // Query 模块不需要发布功能，禁用发布端配置
    cfg.Publisher.HealthCheck.Enabled = false
    cfg.Publisher.Reconnect.Enabled = false

    // 启用订阅端高级功能
    cfg.Subscriber.BacklogDetection.Enabled = true
    cfg.Subscriber.BacklogDetection.MaxLagThreshold = 100
    cfg.Subscriber.BacklogDetection.CheckInterval = 30 * time.Second
    cfg.Subscriber.AggregateProcessor.Enabled = true
    cfg.Subscriber.AggregateProcessor.CacheSize = 500
    cfg.Subscriber.AggregateProcessor.MaxConcurrency = 10
    cfg.Subscriber.RateLimit.Enabled = true
    cfg.Subscriber.RateLimit.Limit = 1000
    cfg.Subscriber.RateLimit.Burst = 1000
    cfg.Subscriber.RecoveryMode.Enabled = true
    cfg.Subscriber.RecoveryMode.AutoDetection = true

    // 创建高级事件总线
    bus, err := eventbus.NewKafkaAdvancedEventBus(cfg)
    if err != nil {
        return nil, err
    }

    // 设置业务特定的组件（仅订阅端相关）
    bus.SetErrorHandler(&EvidenceErrorHandler{})

    // 注册订阅端相关回调
    bus.RegisterBacklogCallback(handleBacklogChange)
    bus.RegisterRecoveryModeCallback(handleRecoveryModeChange)
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

### 2. Command 服务启动

<augment_code_snippet path="evidence-management/command/cmd/api/server.go" mode="EXCERPT">
````go
func init() {
    // 初始化基础组件
    logger.Setup()            // 初始化zaplogger
    database.MasterDbSetup()  // 初始化主数据库
    database.CommandDbSetup() // 初始化命令数据库
    storage.Setup()           // 添加内存队列的创建

    // 初始化 jxt-core AdvancedEventBus（仅发布端）
    bus, err := eventbus.SetupAdvancedEventBusForCommand()
    if err != nil {
        log.Fatal("Failed to setup AdvancedEventBus for Command:", err)
    }

    // 启动事件总线
    ctx := context.Background()
    if err := bus.Start(ctx); err != nil {
        log.Fatal("Failed to start AdvancedEventBus:", err)
    }

    // 启动健康检查
    if err := bus.StartHealthCheck(ctx); err != nil {
        log.Fatal("Failed to start health check:", err)
    }

    // 注册到依赖注入容器
    di.Provide(func() eventbus.AdvancedEventBus {
        return bus
    })
}
````
</augment_code_snippet>

### 3. AdvancedEventBus 适配器

<augment_code_snippet path="evidence-management/command/internal/infrastructure/eventbus/advanced_publisher.go" mode="EXCERPT">
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
    registrations = append(registrations, registerAdvancedEventPublisherDependencies)
}

func registerAdvancedEventPublisherDependencies() {
    if err := di.Provide(func(bus eventbus.AdvancedEventBus) publisher.EventPublisher {
        return NewAdvancedEventPublisher(bus)
    }); err != nil {
        logger.Fatalf("failed to provide AdvancedEventPublisher: %v", err)
    }
}

// AdvancedEventPublisher 使用 jxt-core AdvancedEventBus 的事件发布器
type AdvancedEventPublisher struct {
    advancedBus eventbus.AdvancedEventBus
}

func NewAdvancedEventPublisher(bus eventbus.AdvancedEventBus) *AdvancedEventPublisher {
    return &AdvancedEventPublisher{
        advancedBus: bus,
    }
}

// Publish 发布事件到 Kafka
func (p *AdvancedEventPublisher) Publish(ctx context.Context, topic string, event event.Event) error {
    if event == nil {
        return fmt.Errorf("event cannot be nil")
    }

    payload, err := event.MarshalJSON()
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }

    // 使用 AdvancedEventBus 的高级发布功能
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

    return p.advancedBus.PublishWithOptions(ctx, topic, payload, opts)
}

// RegisterReconnectCallback 注册重连回调
func (p *AdvancedEventPublisher) RegisterReconnectCallback(callback func(ctx context.Context) error) error {
    return p.advancedBus.RegisterReconnectCallback(callback)
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

### 5. 聚合根中生成事件

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

### 6. 具体事件载荷定义

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

### 7. 应用服务中使用 Outbox 模式

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

## 📥 订阅侧使用示例（Query 模块）

### 1. Query 服务启动

<augment_code_snippet path="evidence-management/query/cmd/api/server.go" mode="EXCERPT">
````go
func init() {
    // 初始化基础组件
    logger.Setup()             // 初始化zaplogger
    database.MasterDbSetup()   // 初始化主数据库
    database.CommandDbSetup()  // 初始化命令数据库
    database.QueryDbSetup()    // 初始化查询数据库
    storage.Setup()            // 内存队列的创建

    // 初始化 jxt-core AdvancedEventBus（仅订阅端）
    bus, err := eventbus.SetupAdvancedEventBusForQuery()
    if err != nil {
        log.Fatal("Failed to setup AdvancedEventBus for Query:", err)
    }

    // 启动事件总线
    ctx := context.Background()
    if err := bus.Start(ctx); err != nil {
        log.Fatal("Failed to start AdvancedEventBus:", err)
    }

    // 启动积压监控
    if err := bus.StartBacklogMonitoring(ctx); err != nil {
        log.Fatal("Failed to start backlog monitoring:", err)
    }

    // 注册到依赖注入容器
    di.Provide(func() eventbus.AdvancedEventBus {
        return bus
    })

    // 设置事件订阅
    setupEventSubscriptions()
}

func setupEventSubscriptions() error {
    // 订阅 Media 领域事件
    err := di.Invoke(func(mediaHandler *eventhandler.MediaEventHandler, bus eventbus.AdvancedEventBus) {
        if mediaHandler != nil {
            // 使用 AdvancedEventBus 的高级订阅功能
            if err := subscribeMediaEventsWithAdvancedBus(bus, mediaHandler); err != nil {
                log.Fatal("Failed to subscribe media events:", err)
            }
        }
    })

    // 订阅 EnforcementType 领域事件
    err = di.Invoke(func(enforcementHandler *eventhandler.EnforcementTypeEventHandler, bus eventbus.AdvancedEventBus) {
        if enforcementHandler != nil {
            // 使用 AdvancedEventBus 的高级订阅功能
            if err := subscribeEnforcementTypeEventsWithAdvancedBus(bus, enforcementHandler); err != nil {
                log.Fatal("Failed to subscribe enforcement type events:", err)
            }
        }
    })

    // 订阅 Archive 领域事件
    err = di.Invoke(func(archiveHandler *eventhandler.ArchiveMediaRelationEventHandler, bus eventbus.AdvancedEventBus) {
        if archiveHandler != nil {
            // 使用 AdvancedEventBus 的高级订阅功能
            if err := subscribeArchiveEventsWithAdvancedBus(bus, archiveHandler); err != nil {
                log.Fatal("Failed to subscribe archive events:", err)
            }
        }
    })

    return nil
}
````
</augment_code_snippet>

### 2. AdvancedEventBus 订阅适配器

<augment_code_snippet path="evidence-management/query/internal/infrastructure/eventbus/advanced_subscriber.go" mode="EXCERPT">
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
    registrations = append(registrations, registerAdvancedEventSubscriberDependencies)
}

func registerAdvancedEventSubscriberDependencies() {
    if err := di.Provide(func(bus eventbus.AdvancedEventBus) eventhandler.EventSubscriber {
        return NewAdvancedEventSubscriber(bus)
    }); err != nil {
        logger.Fatalf("failed to provide AdvancedEventSubscriber: %v", err)
    }
}

// AdvancedEventSubscriber 使用 jxt-core AdvancedEventBus 的事件订阅器
type AdvancedEventSubscriber struct {
    advancedBus eventbus.AdvancedEventBus
}

func NewAdvancedEventSubscriber(bus eventbus.AdvancedEventBus) *AdvancedEventSubscriber {
    return &AdvancedEventSubscriber{
        advancedBus: bus,
    }
}

// Subscribe 订阅事件（基础接口兼容）
func (s *AdvancedEventSubscriber) Subscribe(topic string, handler func(msg *message.Message) error, timeout time.Duration) error {
    // 包装处理器以适配 AdvancedEventBus
    wrappedHandler := func(ctx context.Context, message []byte) error {
        msg := message.NewMessage("", message)
        return handler(msg)
    }

    // 使用基础订阅功能
    return s.advancedBus.Subscribe(context.Background(), topic, wrappedHandler)
}

// SubscribeWithAdvancedOptions 使用高级选项订阅
func (s *AdvancedEventSubscriber) SubscribeWithAdvancedOptions(ctx context.Context, topic string, handler func(ctx context.Context, message []byte) error, opts eventbus.SubscribeOptions) error {
    return s.advancedBus.SubscribeWithOptions(ctx, topic, handler, opts)
}
````
</augment_code_snippet>

### 3. 利用现有事件处理器与 AdvancedEventBus 集成

#### 3.1 Media 事件订阅集成

<augment_code_snippet path="evidence-management/query/internal/infrastructure/eventbus/media_subscription.go" mode="EXCERPT">
````go
// subscribeMediaEventsWithAdvancedBus 使用 AdvancedEventBus 订阅媒体事件
func subscribeMediaEventsWithAdvancedBus(bus eventbus.AdvancedEventBus, mediaHandler *eventhandler.MediaEventHandler) error {
    // 配置媒体事件的高级订阅选项
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

#### 3.2 EnforcementType 事件订阅集成

<augment_code_snippet path="evidence-management/query/internal/infrastructure/eventbus/enforcement_subscription.go" mode="EXCERPT">
````go
// subscribeEnforcementTypeEventsWithAdvancedBus 使用 AdvancedEventBus 订阅执法类型事件
func subscribeEnforcementTypeEventsWithAdvancedBus(bus eventbus.AdvancedEventBus, enforcementHandler *eventhandler.EnforcementTypeEventHandler) error {
    // 配置执法类型事件的高级订阅选项
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

#### 3.3 Archive 事件订阅集成

<augment_code_snippet path="evidence-management/query/internal/infrastructure/eventbus/archive_subscription.go" mode="EXCERPT">
````go
// subscribeArchiveEventsWithAdvancedBus 使用 AdvancedEventBus 订阅档案事件
func subscribeArchiveEventsWithAdvancedBus(bus eventbus.AdvancedEventBus, archiveHandler *eventhandler.ArchiveMediaRelationEventHandler) error {
    // 配置档案事件的高级订阅选项
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

### 4. 现有事件处理器结构

evidence-management 项目已经定义了完整的事件处理器，我们只需要将它们与 AdvancedEventBus 集成：

#### 4.1 MediaEventHandler 结构

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

#### 4.2 EnforcementTypeEventHandler 结构

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

### 5. 集成优势

通过将现有的事件处理器与 jxt-core AdvancedEventBus 集成，evidence-management 项目获得以下优势：

#### 5.1 保持业务逻辑不变
- **现有处理器复用**：MediaEventHandler、EnforcementTypeEventHandler 等处理器的业务逻辑完全保持不变
- **接口兼容性**：通过适配器模式，现有的 `handleMediaEvent` 等方法可以直接使用
- **零业务代码修改**：只需要修改基础设施层的订阅方式

#### 5.2 获得企业级功能
- **聚合处理器**：确保同一媒体/执法类型的事件按顺序处理，避免并发冲突
- **积压检测**：自动检测消息积压，及时发现性能问题
- **流量控制**：智能限流，防止系统过载
- **死信队列**：自动处理失败消息，提高系统可靠性

#### 5.3 差异化配置
- **Media 事件**：高并发处理（500 msg/s），长超时（60s）
- **EnforcementType 事件**：中等并发（200 msg/s），标准超时（30s）
- **Archive 事件**：资源密集型处理（300 msg/s），中等超时（45s）

#### 5.4 监控和可观测性
- **统一监控**：所有事件处理的统计信息集中管理
- **健康检查**：自动监控订阅器状态
- **回调机制**：支持积压告警、恢复模式通知等

### 4. 具体事件处理逻辑

<augment_code_snippet path="evidence-management/query/internal/application/eventhandler/media_event_handler.go" mode="EXCERPT">
````go
// handleMediaUploadedEvent 处理媒体上传事件
func (h *MediaEventHandler) handleMediaUploadedEvent(ctx context.Context, domainEvent *event.DomainEvent) error {
    var payload event.MediaUploadedPayload
    var json = jsoniter.ConfigCompatibleWithStandardLibrary

    if err := json.Unmarshal(domainEvent.Payload, &payload); err != nil {
        logger.Error("Error unmarshalling MediaUploadedPayload", "error", err)
        return fmt.Errorf("error unmarshalling MediaUploadedPayload: %w", err)
    }

    // 转换为读模型
    mediaReadModel := &models.MediaReadModel{
        ID:                    uuid.MustParse(payload.MediaID),
        MediaName:             payload.MediaName,
        MediaCate:             payload.MediaCate,
        IsNonEnforcementMedia: payload.IsNonEnforcementMedia,
        IsLocked:              payload.IsLocked,
        CreateBy:              payload.ControlBy.CreateBy,
        UpdateBy:              payload.ControlBy.UpdateBy,
        CreatedAt:             payload.ModelTime.CreatedAt,
        UpdatedAt:             payload.ModelTime.UpdatedAt,
    }

    // 检查媒体是否已存在，避免覆盖已有的状态字段
    existingMedia, err := h.repo.FindByID(ctx, mediaUUID)
    if err != nil && err.Error() != "查看对象不存在或无权查看" {
        logger.Error("查询现有媒体失败", "error", err)
        return err
    }

    if existingMedia != nil {
        // 媒体已存在，只更新非状态相关的字段
        updates := map[string]interface{}{
            "MediaName":  mediaReadModel.MediaName,
            "MediaCate":  mediaReadModel.MediaCate,
            "UpdateBy":   mediaReadModel.UpdateBy,
            "UpdatedAt":  mediaReadModel.UpdatedAt,
        }
        err = h.repo.UpdateByID(ctx, mediaUUID, updates)
    } else {
        // 媒体不存在，创建新的读模型
        err = h.repo.Create(ctx, mediaReadModel)
    }

    return err
}
````
</augment_code_snippet>

### 5. 执法类型事件处理器

<augment_code_snippet path="evidence-management/query/internal/application/eventhandler/enforcement_type_event_handler.go" mode="EXCERPT">
````go
// EnforcementTypeEventHandler 执法类型事件处理器
type EnforcementTypeEventHandler struct {
    Subscriber EventSubscriber
    repo       repository.EnforcementTypeReadModelRepository
}

func (h *EnforcementTypeEventHandler) ConsumeEvent(topic string) error {
    if err := h.Subscriber.Subscribe(topic, h.handleEnforcementTypeEvent, time.Second*30); err != nil {
        logger.Error("Failed to subscribe to topic", "topic", topic, "error", err)
        return err
    }
    return nil
}

func (h *EnforcementTypeEventHandler) handleEnforcementTypeEvent(msg *message.Message) error {
    // Step 1: 反序列化为领域事件结构体
    domainEvent := &event.DomainEvent{}
    err := domainEvent.UnmarshalJSON(msg.Payload)
    if err != nil {
        logger.Error("执法类型事件反序列化失败", "error", err, "messageID", msg.UUID)
        return fmt.Errorf("failed to unmarshal enforcement type event: %w", err)
    }

    // Step 2: 取出领域事件的租户id
    tenantID := domainEvent.GetTenantId()
    if tenantID == "" {
        logger.Error("执法类型事件租户ID为空", "eventID", domainEvent.GetEventID())
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

### 6. 基础设施层初始化配置

<augment_code_snippet path="evidence-management/shared/common/eventbus/initialize.go" mode="EXCERPT">
````go
// SetupPublisher 初始化发布器管理器
func SetupPublisher() {
    log.Printf("Kafka Brokers => %s \n", pkg.Green(strings.Join(toolsConfig.EventBusConfig.Kafka.Brokers, ", ")))

    // 配置 watermill kafka 发布器
    kafkaConfig := kafka.PublisherConfig{
        Brokers:               toolsConfig.EventBusConfig.Kafka.Brokers,
        Marshaler:             kafka.DefaultMarshaler{},
        OverwriteSaramaConfig: kafka.DefaultSaramaSyncPublisherConfig(),
    }

    // 创建 KafkaPublisherManager 配置
    config := DefaultKafkaPublisherManagerConfig()
    config.KafkaPublisherConfig = kafkaConfig
    config.HealthCheckInterval = time.Duration(healthCheckInterval) * time.Minute

    // 创建并启动 KafkaPublisherManager
    manager, err := NewKafkaPublisherManager(config)
    if err != nil {
        log.Fatal(pkg.Red("new kafka manager error: %v\n"), err)
    }

    if err := manager.Start(); err != nil {
        log.Fatal(pkg.Red("Kafka publisher setup error: %v\n"), err)
    } else {
        log.Println(pkg.Green("Kafka publisher setup success!"))
    }

    // 设置全局默认管理器，供适配器使用
    DefaultKafkaPublisherManager = manager
}

// SetupSubscriber 初始化订阅器管理器
func SetupSubscriber() {
    log.Printf("Kafka Subscriber Brokers => %s\n", pkg.Green(strings.Join(toolsConfig.EventBusConfig.Kafka.Brokers, ", ")))

    // 配置 watermill kafka 订阅器
    kafkaConfig := kafka.SubscriberConfig{
        Brokers:               toolsConfig.EventBusConfig.Kafka.Brokers,
        Unmarshaler:           kafka.DefaultMarshaler{},
        OverwriteSaramaConfig: kafka.DefaultSaramaSyncPublisherConfig(),
        ConsumerGroup:         "evidence-query-group", // 消费者组
    }

    // 创建 KafkaSubscriberManager 配置
    config := DefaultKafkaSubscriberManagerConfig()
    config.KafkaConfig = kafkaConfig
    config.HealthCheckConfig.Interval = time.Duration(healthCheckInterval) * time.Minute
    config.HealthCheckConfig.MaxMessageAge = time.Duration(healthCheckInterval) * 3 * time.Minute

    // 创建并启动 KafkaSubscriberManager
    manager, err := NewKafkaSubscriberManager(config)
    if err != nil {
        log.Fatal(pkg.Red("new kafka manager error: %v\n"), err)
    }

    if err := manager.Start(); err != nil {
        log.Fatal(pkg.Red("Kafka subscriber setup error: %v\n"), err)
    } else {
        log.Printf(pkg.Green("Kafka subscriber setup success!"))
    }

    // 设置全局默认管理器，供适配器使用
    DefaultKafkaSubscriberManager = manager
}
````
</augment_code_snippet>

### 7. 与 AdvancedEventBus 的关系

evidence-management 项目目前使用的是**简化版的事件总线架构**，主要特点：

1. **当前架构**：
   - 使用 `KafkaPublisherManager` 和 `KafkaSubscriberManager` 作为底层基础设施
   - 通过适配器模式提供业务层接口
   - 具备基本的健康检查、重连、错误处理功能

2. **与 AdvancedEventBus 的对比**：
   - **相同点**：都基于 Kafka，都有健康检查和重连机制
   - **不同点**：AdvancedEventBus 提供更多高级功能（积压检测、恢复模式、聚合处理器等）

3. **升级路径**：
   - 可以逐步迁移到 AdvancedEventBus
   - 保持现有业务接口不变
   - 获得更强大的监控和管理能力

## 🔧 完整的业务服务示例

### 1. Command 服务启动流程

<augment_code_snippet path="evidence-management/command/cmd/api/server.go" mode="EXCERPT">
````go
func main() {
    // 初始化基础组件
    logger.Setup()            // 初始化zaplogger
    database.MasterDbSetup()  // 初始化主数据库
    database.CommandDbSetup() // 初始化命令数据库
    storage.Setup()           // 内存队列的创建
    eventbus.SetupPublisher() // 初始化KafkaPublisherManager

    usageStr := `starting evidence management command api server...`
    log.Println(usageStr)

    // 启动HTTP服务器
    // ...
}
````
</augment_code_snippet>

### 2. Query 服务启动流程

<augment_code_snippet path="evidence-management/query/cmd/api/server.go" mode="EXCERPT">
````go
func main() {
    // 初始化基础组件
    logger.Setup()             // 初始化zaplogger
    database.MasterDbSetup()   // 初始化主数据库
    database.CommandDbSetup()  // 初始化命令数据库
    database.QueryDbSetup()    // 初始化查询数据库
    storage.Setup()            // 内存队列的创建
    eventbus.SetupSubscriber() // 初始化eventbus

    // 设置事件订阅
    if err := setupEventSubscriptions(); err != nil {
        log.Fatalf("Failed to setup event subscriptions: %v", err)
    }

    usageStr := `starting evidence management query api server...`
    log.Println(usageStr)

    // 启动HTTP服务器
    // ...
}
````
</augment_code_snippet>

## � 升级到 AdvancedEventBus

### 1. 当前架构 vs AdvancedEventBus

**当前 evidence-management 架构**：
```
业务层 → 适配器 → KafkaPublisherManager/KafkaSubscriberManager → Watermill → Kafka
```

**AdvancedEventBus 架构**：
```
业务层 → AdvancedEventBus → 高级组件 → 基础EventBus → Kafka
```

### 2. 升级步骤

#### 步骤1：创建 AdvancedEventBus 实例

```go
// evidence-management/shared/common/eventbus/advanced_setup.go
func SetupAdvancedEventBus() (eventbus.AdvancedEventBus, error) {
    // 加载配置
    cfg := config.GetDefaultAdvancedEventBusConfig()
    cfg.ServiceName = "evidence-management"
    cfg.Type = "kafka"

    // 自定义业务配置
    cfg.Subscriber.BacklogDetection.Enabled = true
    cfg.Subscriber.BacklogDetection.MaxLagThreshold = 100
    cfg.Subscriber.AggregateProcessor.Enabled = true
    cfg.Subscriber.AggregateProcessor.CacheSize = 500
    cfg.Subscriber.RateLimit.Enabled = true
    cfg.Subscriber.RateLimit.Limit = 1000

    // 创建高级事件总线
    bus, err := eventbus.NewKafkaAdvancedEventBus(cfg)
    if err != nil {
        return nil, err
    }

    // 设置业务特定的组件
    bus.SetMessageRouter(&EvidenceMessageRouter{})
    bus.SetErrorHandler(&EvidenceErrorHandler{})

    // 注册回调
    bus.RegisterBacklogCallback(handleBacklogChange)
    bus.RegisterRecoveryModeCallback(handleRecoveryModeChange)

    return bus, nil
}
```

#### 步骤2：创建业务适配器

```go
// evidence-management/command/internal/infrastructure/eventbus/advanced_publisher.go
type AdvancedEventPublisher struct {
    advancedBus eventbus.AdvancedEventBus
}

func NewAdvancedEventPublisher(bus eventbus.AdvancedEventBus) *AdvancedEventPublisher {
    return &AdvancedEventPublisher{advancedBus: bus}
}

func (p *AdvancedEventPublisher) Publish(ctx context.Context, topic string, event event.Event) error {
    payload, err := event.MarshalJSON()
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }

    // 使用 AdvancedEventBus 的高级发布功能
    opts := eventbus.PublishOptions{
        AggregateID: event.GetAggregateID(),
        Metadata: map[string]string{
            "eventType":   event.GetEventType(),
            "tenantId":    event.GetTenantId(),
            "contentType": "application/json",
        },
        Timeout: 30 * time.Second,
    }

    return p.advancedBus.PublishWithOptions(ctx, topic, payload, opts)
}
```

#### 步骤3：渐进式迁移

```go
// 在依赖注入中同时提供两种实现
func init() {
    // 注册当前实现（向后兼容）
    di.Provide(func() eventhandler.EventSubscriber {
        return &KafkaEventSubscriber{
            kafkaManager: eventbus.DefaultKafkaSubscriberManager,
        }
    })

    // 注册新的 AdvancedEventBus 实现
    di.Provide(func() eventbus.AdvancedEventBus {
        bus, err := SetupAdvancedEventBus()
        if err != nil {
            log.Fatal("Failed to setup AdvancedEventBus:", err)
        }
        return bus
    })

    // 可选：提供适配器以逐步迁移
    di.Provide(func(bus eventbus.AdvancedEventBus) publisher.EventPublisher {
        return NewAdvancedEventPublisher(bus)
    })
}
```

### 3. 获得的高级功能

升级到 AdvancedEventBus 后，evidence-management 项目将获得：

1. **积压检测**：自动检测消息积压并触发告警
2. **恢复模式**：在积压情况下自动切换到恢复模式
3. **聚合处理器**：按聚合ID顺序处理消息
4. **高级监控**：详细的性能指标和健康状态
5. **流量控制**：智能的限流和背压机制
6. **错误处理**：更灵活的错误处理策略

## �📊 核心特性和最佳实践

### 1. Outbox 模式确保事务一致性

evidence-management 项目使用 Outbox 模式确保业务操作和事件发布的事务一致性：

1. **事务内操作**：在同一数据库事务中完成业务数据写入和事件保存到 outbox 表
2. **事务外发布**：事务提交后立即尝试发布事件，失败时由调度器重试
3. **幂等性保证**：通过事件ID确保重复发布的幂等性

### 2. 多租户支持

通过 `TenantID` 实现多租户隔离：

- **发布侧**：从上下文获取租户ID并设置到事件中
- **订阅侧**：从事件中提取租户ID并设置到处理上下文中
- **数据隔离**：Repository 层根据租户ID选择对应的数据库连接

### 3. 事件版本管理

通过事件版本字段支持事件结构演进：

- **向后兼容**：新版本事件处理器能处理旧版本事件
- **渐进升级**：支持不同版本的生产者和消费者并存

### 4. 错误处理和重试机制

- **分层错误处理**：聚合根、应用服务、基础设施层各自处理相应错误
- **重试策略**：支持指数退避、最大重试次数等配置
- **死信队列**：无法处理的消息进入死信队列，避免阻塞正常流程

### 5. 性能优化

- **批量处理**：支持批量更新读模型，提高处理效率
- **异步发布**：事务外异步发布事件，不阻塞业务操作
- **连接池**：复用 Kafka 连接，减少连接开销

## 🚀 总结

通过 evidence-management 项目的实际示例，我们可以看到：

### **发布侧（Command 模块）**：
1. **聚合根生成事件**：在业务操作中生成领域事件
2. **Outbox 模式**：确保事务一致性
3. **事件发布器**：将事件发布到 Kafka
4. **立即发布 + 调度重试**：保证最终一致性

### **订阅侧（Query 模块）**：
1. **事件订阅器**：从 Kafka 订阅事件
2. **事件处理器**：根据事件类型分发处理
3. **读模型更新**：更新查询数据库
4. **幂等性处理**：避免重复处理

### **核心优势**：
- **事务一致性**：通过 Outbox 模式保证
- **多租户支持**：完整的租户隔离机制
- **高可用性**：自动重连、健康检查、重试机制
- **可扩展性**：支持水平扩展和负载均衡
- **可观测性**：详细的日志和监控

这种架构为业务开发提供了稳定可靠的事件驱动基础设施，开发者只需关注业务逻辑实现。

## 🎯 扩展阅读

### 相关文档

1. **[AdvancedEventBus 核心功能文档](advanced-eventbus-code-examples.md)** - 详细的 API 和配置说明
2. **[EventBus 提取方案](eventbus-extraction-proposal.md)** - 架构设计和接口定义
3. **[jxt-core SDK 文档](../sdk/README.md)** - 完整的 SDK 使用指南

### 项目结构参考

```
evidence-management/
├── command/                    # 命令侧
│   ├── cmd/api/               # 服务入口
│   ├── internal/
│   │   ├── domain/            # 领域层
│   │   │   ├── aggregate/     # 聚合根
│   │   │   ├── event/         # 事件定义
│   │   │   └── valueobject/   # 值对象
│   │   ├── application/       # 应用层
│   │   │   ├── command/       # 命令定义
│   │   │   └── service/       # 应用服务
│   │   └── infrastructure/    # 基础设施层
│   │       ├── eventbus/      # 事件发布
│   │       ├── persistence/   # 数据持久化
│   │       └── transaction/   # 事务管理
├── query/                     # 查询侧
│   ├── cmd/api/               # 服务入口
│   ├── internal/
│   │   ├── application/       # 应用层
│   │   │   ├── eventhandler/  # 事件处理器
│   │   │   ├── query/         # 查询定义
│   │   │   └── service/       # 查询服务
│   │   ├── infrastructure/    # 基础设施层
│   │   │   ├── eventbus/      # 事件订阅
│   │   │   └── persistence/   # 读模型存储
│   │   └── models/            # 读模型定义
└── shared/                    # 共享模块
    ├── common/                # 通用组件
    │   ├── di/                # 依赖注入
    │   ├── eventbus/          # EventBus 初始化
    │   └── global/            # 全局常量
    └── domain/                # 领域共享
        └── event/             # 事件定义
```

### 开发建议

1. **事件设计原则**
   - 事件应该表达业务意图，而不是技术实现
   - 保持事件的向后兼容性
   - 使用明确的事件命名约定

2. **性能考虑**
   - 合理设置批处理大小
   - 监控消息积压情况
   - 优化序列化性能

3. **运维监控**
   - 设置关键指标告警
   - 定期检查死信队列
   - 监控事件处理延迟

## 🎯 使用 jxt-core AdvancedEventBus 的核心优势

### 1. 架构对比

**传统自定义实现**：
- 需要自己实现 KafkaPublisherManager/KafkaSubscriberManager
- 手动处理连接管理、健康检查、重连逻辑
- 缺乏统一的监控和管理能力

**jxt-core AdvancedEventBus**：
- 开箱即用的高级功能
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
4. **扩展性增强**：支持多种高级功能，满足复杂业务需求

## 📚 总结

通过 evidence-management 项目的示例，本文档展示了如何在业务微服务中使用 jxt-core AdvancedEventBus：

### 核心实现要点

- **Command 模块**：使用 AdvancedEventBus 发布事件，支持高级发布选项和监控
- **Query 模块**：使用 AdvancedEventBus 订阅事件，支持聚合处理、流量控制等高级功能
- **配置管理**：通过统一配置文件管理所有 EventBus 参数
- **适配器模式**：保持业务接口不变，底层使用 AdvancedEventBus

### 技术优势

- **企业级功能**：积压检测、恢复模式、健康检查等
- **高可用性**：自动重连、故障恢复、死信队列
- **可观测性**：丰富的监控指标和回调机制
- **性能优化**：聚合处理、流量控制、批量处理

这种架构为微服务提供了可靠、高效的事件驱动基础设施，支持高并发、高可用的分布式系统。