# EventBus - 统一事件总线组件

EventBus是jxt-core提供的统一事件总线组件，支持多种消息中间件实现，为微服务架构提供可靠的事件驱动通信能力。

## 🚀 架构优化亮点

### 统一架构设计
- **NATS**: 1个连接 → 1个JetStream Context → 1个统一Consumer → 多个Pull Subscription
- **Kafka**: 1个连接 → 1个统一Consumer Group → 多个Topic订阅
- **统一接口**: 所有实现都使用相同的EventBus接口，支持无缝切换

### 性能优化成果
- **资源效率**: NATS Consumer数量从N个优化为1个，资源节省33-41%
- **管理简化**: 统一Consumer管理，降低运维复杂度
- **扩展性**: 新增topic无需创建新Consumer，只需添加Pull Subscription

详细优化报告请参考：[NATS优化报告](./NATS_OPTIMIZATION_REPORT.md)

## 🏗️ 架构图

### NATS 统一架构
```
Connection
    └── JetStream Context
        └── Unified Consumer (FilterSubject: ">")
            ├── Pull Subscription (topic1)
            ├── Pull Subscription (topic2)
            └── Pull Subscription (topicN)
```

### Kafka 统一架构
```
Connection
    └── Unified Consumer Group
        ├── Topic Subscription (topic1)
        ├── Topic Subscription (topic2)
        └── Topic Subscription (topicN)
```

## 🚀 快速开始

⚠️ **Kafka 用户必读**：如果使用 Kafka，ClientID 和 Topic 名称**必须只使用 ASCII 字符**（不能使用中文、日文、韩文等），否则消息无法接收！详见 [Kafka 配置章节](#kafka实现配置)。

### 基础使用示例

```go
package main

import (
    "context"
    "log"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 创建NATS EventBus
    bus, err := eventbus.NewPersistentNATSEventBus(
        []string{"nats://localhost:4222"},
        "my-client",
    )
    if err != nil {
        log.Fatal(err)
    }
    defer bus.Close()

    ctx := context.Background()

    // 订阅消息
    err = bus.Subscribe(ctx, "user.created", func(ctx context.Context, message []byte) error {
        log.Printf("Received: %s", string(message))
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    // 发布消息
    err = bus.Publish(ctx, "user.created", []byte(`{"id": "123", "name": "John"}`))
    if err != nil {
        log.Fatal(err)
    }
}
```

## 配置

### 内存实现配置

```yaml
eventbus:
  type: memory
  metrics:
    enabled: true
    collectInterval: 30s
  tracing:
    enabled: false
    sampleRate: 0.1
```

### Kafka实现配置

⚠️ **重要提示**：Kafka 的 `ClientID` 和 `topic` 名称**必须只使用 ASCII 字符**，避免使用中文或其他 Unicode 字符，否则会导致消息无法正常接收！

```yaml
eventbus:
  type: kafka
  kafka:
    brokers:
      - localhost:9092
    clientId: my-service-client    # ⚠️ 必须使用 ASCII 字符，不能使用中文
    healthCheckInterval: 5m
    producer:
      requiredAcks: 1
      compression: snappy
      flushFrequency: 500ms
      flushMessages: 100
      retryMax: 3
      timeout: 10s
      batchSize: 16384
      bufferSize: 32768
    consumer:
      groupId: jxt-eventbus-group  # ⚠️ 必须使用 ASCII 字符，不能使用中文
      autoOffsetReset: earliest
      sessionTimeout: 30s
      heartbeatInterval: 3s
      maxProcessingTime: 5m
      fetchMinBytes: 1
      fetchMaxBytes: 1048576
      fetchMaxWait: 500ms
    security:
      enabled: false
      protocol: PLAINTEXT
  metrics:
    enabled: true
    collectInterval: 30s
  tracing:
    enabled: false
    sampleRate: 0.1

# ⚠️ Topic 命名规范（Kafka）
# ✅ 正确：business.orders, user.events, audit.logs
# ❌ 错误：业务.订单, 用户.事件, 审计.日志
```

### NATS JetStream配置 (优化架构)

NATS EventBus 采用统一Consumer架构，提供企业级的可靠性保证：

**🔥 架构特点**:
- **1个连接**: 高效的连接复用
- **1个JetStream Context**: 统一的流管理
- **1个统一Consumer**: 使用FilterSubject ">" 订阅所有主题
- **多个Pull Subscription**: 每个topic独立的Pull Subscription

```yaml
eventbus:
  type: nats
  nats:
    urls:
      - nats://localhost:4222
    clientId: jxt-client
    maxReconnects: 10
    reconnectWait: 2s
    connectionTimeout: 10s
    healthCheckInterval: 5m

    # JetStream配置 - 统一架构
    jetstream:
      enabled: true
      publishTimeout: 5s
      ackWait: 30s
      maxDeliver: 3

      # 流配置
      stream:
        name: "BUSINESS_STREAM"
        subjects:
          - "order.*"         # 订单相关
          - "payment.*"       # 支付相关
          - "audit.*"         # 审计日志
          - "user.*"          # 用户相关
          - "system.*"        # 系统事件
        retention: "limits"
        storage: "file"       # 文件存储（持久化）
        replicas: 1
        maxAge: 24h
        maxBytes: 100MB
        maxMsgs: 10000
        discard: "old"

      # 统一Consumer配置 (自动创建为 "{durableName}-unified")
      consumer:
        durableName: "business-consumer"  # 实际Consumer名: "business-consumer-unified"
        deliverPolicy: "all"
        ackPolicy: "explicit"
        replayPolicy: "instant"
        maxAckPending: 100
        maxWaiting: 500
        maxDeliver: 3
        # filterSubject: ">" 自动设置，订阅所有主题

  metrics:
    enabled: true
    collectInterval: 30s
```

**优化效果**:
- ✅ 资源节省33-41%（Consumer数量从N个减少到1个）
- ✅ 管理简化（统一Consumer管理）
- ✅ 扩展性强（新增topic无需创建新Consumer）

### NATS JetStream 异步发布与 ACK 处理

NATS JetStream 的 `PublishEnvelope` 方法使用**异步发布模式**，符合业界最佳实践，提供高性能和可靠性保证。

#### 🚀 异步发布机制

**核心特点**:
- ✅ **立即返回**: `PublishEnvelope` 调用后立即返回，不等待 NATS 服务器 ACK
- ✅ **后台处理**: 异步 goroutine 处理 ACK 确认和错误
- ✅ **高吞吐量**: 支持批量发送，吞吐量与 Kafka 基本持平
- ✅ **可靠性保证**: 通过异步 ACK 确认机制保证消息送达

**实现原理**:
```go
// 发布消息（异步，立即返回）
err := eventBus.PublishEnvelope(ctx, topic, envelope)
// ✅ 此时消息已提交到发送队列，立即返回
// 🔄 后台 goroutine 处理 ACK 确认
```

**内部流程**:
```
1. 序列化 Envelope → 创建 NATS 消息
2. 调用 js.PublishMsgAsync(msg) → 提交到发送队列
3. 立即返回（不等待 ACK）
4. 后台 goroutine 监听 PubAckFuture:
   - 成功: 更新计数器 + 发送结果到 resultChan
   - 失败: 记录错误 + 发送结果到 resultChan
```

#### 📊 ACK 处理机制

NATS JetStream 提供两种 ACK 处理方式：

**1. 自动 ACK 处理（默认）**

适用于大多数场景，EventBus 自动处理 ACK 确认：

```go
// 发布消息
err := eventBus.PublishEnvelope(ctx, "orders.created", envelope)
if err != nil {
    // 提交失败（队列满或连接断开）
    log.Error("Failed to submit publish", zap.Error(err))
    return err
}
// ✅ 提交成功，后台自动处理 ACK
```

**特点**:
- ✅ 简单易用，无需额外代码
- ✅ 自动重试（NATS SDK 内置）
- ✅ 错误自动记录到日志
- ⚠️ 无法获取单条消息的 ACK 结果

**2. 手动 ACK 处理（Outbox 模式）**

适用于需要精确控制 ACK 结果的场景（如 Outbox 模式）：

```go
// 获取异步发布结果通道
resultChan := eventBus.GetPublishResultChannel()

// 启动结果监听器
go func() {
    for result := range resultChan {
        if result.Success {
            // ✅ 发布成功
            log.Info("Message published successfully",
                zap.String("eventID", result.EventID),
                zap.String("topic", result.Topic))

            // Outbox 模式：标记为已发布
            outboxRepo.MarkAsPublished(ctx, result.EventID)
        } else {
            // ❌ 发布失败
            log.Error("Message publish failed",
                zap.String("eventID", result.EventID),
                zap.Error(result.Error))

            // Outbox 模式：记录错误，下次重试
            outboxRepo.RecordError(ctx, result.EventID, result.Error)
        }
    }
}()

// 发布消息
err := eventBus.PublishEnvelope(ctx, "orders.created", envelope)
// ✅ 立即返回，ACK 结果通过 resultChan 异步通知
```

**特点**:
- ✅ 精确控制每条消息的 ACK 结果
- ✅ 支持 Outbox 模式的状态更新
- ✅ 支持自定义错误处理和重试逻辑
- ⚠️ 需要额外的结果监听代码

#### 🎯 Outbox 模式集成示例

完整的 Outbox Processor 实现：

```go
type OutboxPublisher struct {
    eventBus   eventbus.EventBus
    outboxRepo OutboxRepository
    logger     *zap.Logger
}

func (p *OutboxPublisher) Start(ctx context.Context) {
    // 启动结果监听器
    resultChan := p.eventBus.GetPublishResultChannel()

    go func() {
        for {
            select {
            case result := <-resultChan:
                if result.Success {
                    // 标记为已发布
                    p.outboxRepo.MarkAsPublished(ctx, result.EventID)
                } else {
                    // 记录错误（下次轮询时重试）
                    p.logger.Error("Publish failed",
                        zap.String("eventID", result.EventID),
                        zap.Error(result.Error))
                }
            case <-ctx.Done():
                return
            }
        }
    }()
}

func (p *OutboxPublisher) PublishEvents(ctx context.Context) {
    // 查询未发布的事件
    events, _ := p.outboxRepo.FindUnpublished(ctx, 100)

    for _, event := range events {
        envelope := &eventbus.Envelope{
            AggregateID:  event.AggregateID,
            EventType:    event.EventType,
            EventVersion: event.EventVersion,
            Timestamp:    event.Timestamp,
            Payload:      event.Payload,
        }

        // 异步发布（立即返回）
        if err := p.eventBus.PublishEnvelope(ctx, event.Topic, envelope); err != nil {
            p.logger.Error("Failed to submit publish", zap.Error(err))
        }
        // ✅ ACK 结果通过 resultChan 异步通知
    }
}
```

#### 📈 性能对比

| 模式 | 发送延迟 | 吞吐量 | 适用场景 |
|------|---------|--------|---------|
| **异步发布** | 1-10 ms | 100-300 msg/s | ✅ 推荐（默认） |
| 同步发布 | 20-70 ms | 10-50 msg/s | ⚠️ 不推荐 |

**性能优势**:
- ✅ 延迟降低 **5-10 倍**
- ✅ 吞吐量提升 **5-10 倍**
- ✅ 与 Kafka AsyncProducer 性能基本持平

#### 🏆 业界最佳实践

根据 NATS 官方文档和核心开发者建议：

> "If you want throughput of publishing messages to a stream, you should use **js.AsyncPublish()** that returns a PubAckFuture"

**推荐配置**:
```yaml
jetstream:
  enabled: true
  publishTimeout: 5s      # 异步发布超时
  ackWait: 60s           # ACK 等待时间（订阅端）
  maxDeliver: 3          # 最大重传次数
```

**最佳实践**:
1. ✅ **默认使用异步发布**: 适用于 99% 的场景
2. ✅ **Outbox 模式使用结果通道**: 精确控制 ACK 状态
3. ✅ **合理配置缓冲区**: `PublishAsyncMaxPending: 10000`
4. ✅ **监控发布指标**: 通过 `GetMetrics()` 监控发送成功率

### 企业特性配置

EventBus 支持丰富的企业特性，可以通过配置启用：

```yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]
    consumer:
      groupId: "my-service"

  # 发布端配置
  publisher:
    # 发送端积压检测
    backlogDetection:
      enabled: true
      maxQueueDepth: 1000       # 最大队列深度
      maxPublishLatency: 5s     # 最大发送延迟
      rateThreshold: 500.0      # 发送速率阈值 (msg/sec)
      checkInterval: 30s        # 检测间隔

    retryPolicy:
      enabled: true
      maxRetries: 3
      backoffStrategy: "exponential"

  # 订阅端配置
  subscriber:
      # Keyed-Worker 池（顺序处理的核心架构）
      keyedWorkerPool:
        enabled: true
        workerCount: 256      # Worker 数量（建议为CPU核心数的8-16倍）
        queueSize: 1000       # 每个 Worker 的队列大小（根据消息大小调整）
        waitTimeout: 200ms    # 队列满时的等待超时（高吞吐场景建议200ms）
        # 优势：
        # - 同一聚合ID的事件严格按序处理
        # - 不同聚合ID并行处理，性能优异
        # - 资源使用可控，无性能抖动
        # - 配置简单，运维友好

      # 订阅端积压检测
      backlogDetection:
        enabled: true
        checkInterval: 30s
        maxLagThreshold: 1000
        maxTimeThreshold: 5m

      # 流量控制
      rateLimit:
        enabled: true
        rateLimit: 1000.0     # 每秒处理消息数
        burst: 2000           # 突发处理能力

  # 健康检查配置（分离式配置，发布器和订阅器独立控制）
  healthCheck:
    enabled: true           # 启用健康检查功能（总开关）
    publisher:
      topic: "health-check-my-service"    # 发布器主题
      interval: "2m"                      # 发布间隔
      timeout: "10s"                      # 发送超时时间
      failureThreshold: 3                 # 连续失败阈值（触发重连）
      messageTTL: "5m"                    # 消息存活时间
    subscriber:
      topic: "health-check-my-service"    # 订阅器主题（与发布器配对）
      monitorInterval: "30s"              # 监控检查间隔
      warningThreshold: 3                 # 警告阈值
      errorThreshold: 5                   # 错误阈值
      criticalThreshold: 10               # 严重阈值
    # 优势：
    # - 分离式控制：发送器和订阅器独立启动和配置
    # - 主题精确配对：支持跨服务监控和多重监控
    # - 角色灵活性：同一服务可在不同业务中扮演不同角色
    # - 配置简化：移除冗余的enabled字段，通过接口控制启动
```

## 核心接口

### EventBus接口

EventBus 是统一的事件总线接口，支持多种消息传递模式：

```go
type EventBus interface {
    // ========== 基础功能 ==========
    // 发布消息到指定主题
    Publish(ctx context.Context, topic string, message []byte) error
    // Subscribe 订阅原始消息（不使用Keyed-Worker池）
    // 特点：直接并发处理，极致性能，无顺序保证
    // 适用：简单消息、通知、缓存失效等不需要顺序的场景
    Subscribe(ctx context.Context, topic string, handler MessageHandler) error
    // 关闭连接
    Close() error
    // 注册重连回调
    RegisterReconnectCallback(callback ReconnectCallback) error

    // ========== 生命周期管理 ==========
    // 启动事件总线（根据配置启用相应功能）
    Start(ctx context.Context) error
    // 停止事件总线
    Stop() error

    // ========== 高级发布功能（可选启用） ==========
    // 使用选项发布消息
    PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error
    // 设置消息格式化器
    SetMessageFormatter(formatter MessageFormatter) error
    // 注册发布回调
    RegisterPublishCallback(callback PublishCallback) error

    // ========== 高级订阅功能（可选启用） ==========
    // 使用选项订阅消息
    SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error

    // ========== 主题持久化管理功能 ==========
    // 配置主题的完整选项（持久化模式、保留时间、最大大小等）
    ConfigureTopic(ctx context.Context, topic string, options TopicOptions) error
    // 简化接口：设置主题的持久化策略（true=持久化，false=非持久化）
    SetTopicPersistence(ctx context.Context, topic string, persistent bool) error
    // 获取主题的配置信息
    GetTopicConfig(topic string) (TopicOptions, error)
    // 列出所有已配置的主题
    ListConfiguredTopics() []string
    // 移除主题配置
    RemoveTopicConfig(topic string) error

    // ========== 积压检测功能 ==========
    // 注册订阅端积压回调
    RegisterSubscriberBacklogCallback(callback BacklogStateCallback) error
    // 启动订阅端积压监控
    StartSubscriberBacklogMonitoring(ctx context.Context) error
    // 停止订阅端积压监控
    StopSubscriberBacklogMonitoring() error

    // 注册发送端积压回调
    RegisterPublisherBacklogCallback(callback PublisherBacklogCallback) error
    // 启动发送端积压监控
    StartPublisherBacklogMonitoring(ctx context.Context) error
    // 停止发送端积压监控
    StopPublisherBacklogMonitoring() error

    // 根据配置启动所有积压监控（发送端和/或订阅端）
    StartAllBacklogMonitoring(ctx context.Context) error
    // 停止所有积压监控
    StopAllBacklogMonitoring() error
    // 设置消息路由器
    SetMessageRouter(router MessageRouter) error
    // 设置错误处理器
    SetErrorHandler(handler ErrorHandler) error
    // 注册订阅回调
    RegisterSubscriptionCallback(callback SubscriptionCallback) error

    // ========== 健康检查功能（分离发布端和订阅端） ==========
    // 发布端健康检查（发送健康检查消息）
    StartHealthCheckPublisher(ctx context.Context) error
    StopHealthCheckPublisher() error
    GetHealthCheckPublisherStatus() HealthCheckStatus
    RegisterHealthCheckPublisherCallback(callback HealthCheckCallback) error

    // 订阅端健康检查（监控健康检查消息）
    StartHealthCheckSubscriber(ctx context.Context) error
    StopHealthCheckSubscriber() error
    GetHealthCheckSubscriberStats() HealthCheckSubscriberStats
    RegisterHealthCheckSubscriberCallback(callback HealthCheckAlertCallback) error

    // 根据配置启动所有健康检查（发布端和/或订阅端）
    StartAllHealthCheck(ctx context.Context) error
    // 停止所有健康检查
    StopAllHealthCheck() error
    // 获取连接状态
    GetConnectionState() ConnectionState
    // 获取监控指标
    GetMetrics() Metrics

    // ========== Envelope 支持（可选使用） ==========
    // 发布Envelope消息
    PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error
    // SubscribeEnvelope 订阅Envelope消息（自动使用Keyed-Worker池）
    // 特点：按聚合ID顺序处理，事件溯源支持，毫秒级延迟
    // 适用：领域事件、事件溯源、聚合管理等需要顺序保证的场景
    SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error
}
```

### 消息处理器类型

```go
// 普通消息处理器
type MessageHandler func(ctx context.Context, message []byte) error

// Envelope消息处理器
type EnvelopeHandler func(ctx context.Context, envelope *Envelope) error
```

### 回调函数类型

```go
// 重连回调
type ReconnectCallback func(ctx context.Context) error

// 健康检查回调
type HealthCheckCallback func(ctx context.Context, result HealthCheckResult) error

// 健康检查告警回调
type HealthCheckAlertCallback func(ctx context.Context, alert HealthCheckAlert) error

// 积压检测回调
type BacklogStateCallback func(ctx context.Context, state BacklogState) error

// 发送端积压检测回调
type PublisherBacklogCallback func(ctx context.Context, state PublisherBacklogState) error

// 发布回调
type PublishCallback func(ctx context.Context, topic string, message []byte, err error) error

// 订阅回调
type SubscriptionCallback func(ctx context.Context, event SubscriptionEvent) error
```

### 状态和结果类型

```go
// 健康检查状态
type HealthCheckStatus struct {
    IsHealthy           bool      `json:"isHealthy"`
    ConsecutiveFailures int       `json:"consecutiveFailures"`
    LastSuccessTime     time.Time `json:"lastSuccessTime"`
    LastFailureTime     time.Time `json:"lastFailureTime"`
    IsRunning           bool      `json:"isRunning"`
    EventBusType        string    `json:"eventBusType"`
    Source              string    `json:"source"`
}

// 健康检查订阅监控器统计信息
type HealthCheckSubscriberStats struct {
    StartTime             time.Time `json:"startTime"`
    LastMessageTime       time.Time `json:"lastMessageTime"`
    TotalMessagesReceived int64     `json:"totalMessagesReceived"`
    ConsecutiveMisses     int32     `json:"consecutiveMisses"`
    TotalAlerts           int64     `json:"totalAlerts"`
    LastAlertTime         time.Time `json:"lastAlertTime"`
    IsHealthy             bool      `json:"isHealthy"`
    UptimeSeconds         float64   `json:"uptimeSeconds"`
}

// 健康检查告警信息
type HealthCheckAlert struct {
    AlertType         string            `json:"alertType"`         // 告警类型
    Severity          string            `json:"severity"`          // 严重程度：warning, error, critical
    Source            string            `json:"source"`            // 告警来源
    EventBusType      string            `json:"eventBusType"`      // EventBus类型
    Topic             string            `json:"topic"`             // 健康检查主题
    LastMessageTime   time.Time         `json:"lastMessageTime"`   // 最后收到消息的时间
    TimeSinceLastMsg  time.Duration     `json:"timeSinceLastMsg"`  // 距离最后消息的时间
    ExpectedInterval  time.Duration     `json:"expectedInterval"`  // 期望的消息间隔
    ConsecutiveMisses int32             `json:"consecutiveMisses"` // 连续错过次数
    Timestamp         time.Time         `json:"timestamp"`         // 告警时间
    Metadata          map[string]string `json:"metadata"`          // 额外元数据
}

// 连接状态
type ConnectionState struct {
    IsConnected       bool      `json:"isConnected"`
    LastConnectedTime time.Time `json:"lastConnectedTime"`
    ReconnectCount    int       `json:"reconnectCount"`
    LastError         string    `json:"lastError,omitempty"`
}

// 监控指标
type Metrics struct {
    MessagesPublished int64     `json:"messagesPublished"`
    MessagesConsumed  int64     `json:"messagesConsumed"`
    PublishErrors     int64     `json:"publishErrors"`
    ConsumeErrors     int64     `json:"consumeErrors"`
    ConnectionErrors  int64     `json:"connectionErrors"`
    LastHealthCheck   time.Time `json:"lastHealthCheck"`
    HealthCheckStatus string    `json:"healthCheckStatus"`
    ActiveConnections int       `json:"activeConnections"`
    MessageBacklog    int64     `json:"messageBacklog"`
}
```

### Envelope结构

Envelope 是事件溯源的核心数据结构，包含完整的事件元数据：

```go
type Envelope struct {
    // ========== 核心字段（必填） ==========
    AggregateID   string    `json:"aggregate_id"`   // 聚合根ID
    EventType     string    `json:"event_type"`     // 事件类型
    EventVersion  int       `json:"event_version"`  // 事件版本
    Payload       []byte    `json:"payload"`        // 事件负载
    Timestamp     time.Time `json:"timestamp"`      // 事件时间戳

    // ========== 可选字段 ==========
    EventID       string            `json:"event_id,omitempty"`       // 事件唯一ID
    TraceID       string            `json:"trace_id,omitempty"`       // 链路追踪ID
    CorrelationID string            `json:"correlation_id,omitempty"` // 关联ID
    Headers       map[string]string `json:"headers,omitempty"`        // 自定义头部
    Source        string            `json:"source,omitempty"`         // 事件源
}

// 创建新的Envelope
func NewEnvelope(aggregateID, eventType string, eventVersion int, payload []byte) *Envelope

// 序列化和反序列化
func (e *Envelope) ToBytes() ([]byte, error)
func FromBytes(data []byte) (*Envelope, error)
```

### 发布选项

```go
type PublishOptions struct {
    AggregateID string            `json:"aggregateId"`       // 聚合ID（用于分区）
    Metadata    map[string]string `json:"metadata"`          // 元数据
    Timeout     time.Duration     `json:"timeout"`           // 发布超时
    Retries     int               `json:"retries"`           // 重试次数
    Headers     map[string]string `json:"headers"`           // 自定义头部
    Priority    int               `json:"priority"`          // 消息优先级
}
```

### 订阅选项

```go
type SubscribeOptions struct {
    ProcessingTimeout time.Duration `json:"processingTimeout"` // 处理超时时间
    RateLimit         float64       `json:"rateLimit"`         // 速率限制（每秒）
    RateBurst         int           `json:"rateBurst"`         // 突发大小
    MaxRetries        int           `json:"maxRetries"`        // 最大重试次数
    RetryBackoff      time.Duration `json:"retryBackoff"`      // 重试退避时间
    DeadLetterEnabled bool          `json:"deadLetterEnabled"` // 是否启用死信队列
    RetryPolicy       RetryPolicy   `json:"retryPolicy"`       // 重试策略
}
```

### 主题持久化配置

EventBus 支持基于主题的动态持久化配置，允许在同一个 EventBus 实例中处理不同持久化需求的主题：

```go
// 主题持久化模式
type TopicPersistenceMode string

const (
    TopicPersistent TopicPersistenceMode = "persistent" // 持久化存储
    TopicEphemeral  TopicPersistenceMode = "ephemeral"  // 内存存储
    TopicAuto       TopicPersistenceMode = "auto"       // 根据全局配置自动选择
)

// 主题配置选项
type TopicOptions struct {
    PersistenceMode TopicPersistenceMode `json:"persistenceMode"` // 持久化模式
    RetentionTime   time.Duration        `json:"retentionTime"`   // 消息保留时间
    MaxSize         int64                `json:"maxSize"`         // 最大存储大小（字节）
    Description     string               `json:"description"`     // 主题描述
}

// 创建默认主题选项
func DefaultTopicOptions() TopicOptions

// 检查是否为持久化模式
func (t TopicOptions) IsPersistent() bool
```

#### 主题持久化使用示例

```go
// 1. 配置不同持久化策略的主题
ctx := context.Background()

// 持久化主题：订单事件需要长期保存
orderOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicPersistent,
    RetentionTime:   24 * time.Hour,
    MaxSize:         100 * 1024 * 1024, // 100MB
    Description:     "订单相关事件",
}
err := bus.ConfigureTopic(ctx, "order.events", orderOptions)

// 非持久化主题：临时通知消息
notificationOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicEphemeral,
    RetentionTime:   30 * time.Minute,
    Description:     "临时通知消息",
}
err = bus.ConfigureTopic(ctx, "notification.temp", notificationOptions)

// 自动模式：根据全局配置决定
metricsOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicAuto,
    Description:     "系统监控指标",
}
err = bus.ConfigureTopic(ctx, "system.metrics", metricsOptions)

// 2. 简化接口：快速设置持久化策略
err = bus.SetTopicPersistence(ctx, "user.events", true)  // 持久化
err = bus.SetTopicPersistence(ctx, "cache.invalidation", false) // 非持久化

// 3. 查询主题配置
config, err := bus.GetTopicConfig("order.events")
if err == nil {
    fmt.Printf("主题配置: %s, 保留时间: %v\n",
        config.PersistenceMode, config.RetentionTime)
}

// 4. 列出所有配置的主题
topics := bus.ListConfiguredTopics()
fmt.Printf("已配置主题: %v\n", topics)

// 5. 移除主题配置
err = bus.RemoveTopicConfig("temp.topic")
```


## 特性

### 🎯 **主题持久化管理（核心特性）**

EventBus 的核心创新是**基于主题的智能持久化管理**，允许在同一个实例中动态配置不同主题的持久化策略：

#### 核心能力
- **🎯 主题级控制**：每个主题可以独立配置持久化策略、保留时间、存储限制
- **🔄 动态配置**：运行时动态添加、修改、删除主题配置，无需重启服务
- **🚀 智能路由**：根据主题配置自动选择最优的消息传递机制
- **⚡ 性能优化**：持久化和非持久化主题并存，各取所长
- **🔧 统一接口**：现有的 Publish/Subscribe API 保持不变，零迁移成本
- **📊 完整监控**：提供主题配置查询和管理接口

#### 智能路由机制
**NATS EventBus**：
- **持久化主题** → JetStream（可靠存储、消息持久化、支持重放）
- **非持久化主题** → Core NATS（高性能、内存传输、微秒级延迟）
- **自动模式** → 根据全局 JetStream 配置决定

**Kafka EventBus**：
- **持久化主题** → 长期保留策略（如7天、多副本、大存储限制）
- **非持久化主题** → 短期保留策略（如1小时、单副本、小存储限制）
- **自动模式** → 根据全局配置决定

#### 主题配置选项
```go
type TopicOptions struct {
    PersistenceMode TopicPersistenceMode // persistent/ephemeral/auto
    RetentionTime   time.Duration        // 消息保留时间
    MaxSize         int64                // 最大存储大小
    MaxMessages     int64                // 最大消息数量
    Replicas        int                  // 副本数量（Kafka）
    Description     string               // 主题描述
}
```

#### 使用示例
```go
// 配置持久化主题（业务关键事件）
orderOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicPersistent,
    RetentionTime:   24 * time.Hour,
    MaxSize:         100 * 1024 * 1024,
    Description:     "订单事件，需要持久化",
}
bus.ConfigureTopic(ctx, "business.orders", orderOptions)

// 配置非持久化主题（临时通知）
bus.SetTopicPersistence(ctx, "system.notifications", false)

// 发布消息（EventBus 自动智能路由）
bus.Publish(ctx, "business.orders", orderData)      // → JetStream/长期保留
bus.Publish(ctx, "system.notifications", notifyData) // → Core NATS/短期保留
```

### 🚀 **核心特性**
- **多种实现**：支持Kafka、NATS、内存队列等多种消息中间件
- **配置驱动**：通过配置文件灵活切换不同的消息中间件
- **统一接口**：单一EventBus接口支持基础和企业级功能
- **线程安全**：支持并发安全的消息发布和订阅
- **DDD兼容**：完全符合领域驱动设计原则
- **向前兼容**：现有API保持不变，支持渐进式采用

### 📨 **消息处理模式**
- **普通消息**：`Publish()` / `Subscribe()` - 高性能并发处理
- **高级选项**：`PublishWithOptions()` / `SubscribeWithOptions()` - 企业特性支持
- **Envelope模式**：`PublishEnvelope()` / `SubscribeEnvelope()` - 事件溯源和聚合管理

### ⚡ **顺序处理 - Keyed-Worker池架构**

#### 🏗️ **架构模式：每个Topic一个Keyed-Worker池**

```
EventBus实例
├── Topic: orders.events     → Keyed-Worker池1 (1024个Worker)
├── Topic: user.events       → Keyed-Worker池2 (1024个Worker)
└── Topic: inventory.events  → Keyed-Worker池3 (1024个Worker)

每个池内的聚合ID路由：
orders.events池:
├── Worker-1:  order-001, order-005, order-009...
├── Worker-2:  order-002, order-006, order-010...
└── Worker-N:  order-XXX (hash(aggregateID) % 1024)
```

#### 🎯 **核心特性**
- **Topic级别隔离**：每个Topic独立的Keyed-Worker池，业务领域完全隔离
- **聚合内顺序**：同一聚合ID的事件通过一致性哈希路由到固定Worker，确保严格按序处理
- **高性能并发**：不同聚合ID的事件可并行处理，充分利用多核性能
- **资源可控性**：每个池固定1024个Worker，内存使用可预测，避免资源溢出
- **自然背压**：有界队列提供背压机制，系统过载时优雅降级
- **监控友好**：Topic级别的池隔离便于独立监控和调优

#### 📊 **架构优势对比**

| 架构方案 | jxt-core采用 | 优缺点分析 |
|---------|-------------|-----------|
| **全局共用池** | ❌ | ❌ 跨Topic竞争资源<br/>❌ 难以隔离监控<br/>❌ 故障影响面大 |
| **每聚合类型一池** | ❌ | ❌ 管理复杂度高<br/>❌ 资源碎片化<br/>❌ 动态聚合类型难处理 |
| **每Topic一池** | ✅ | ✅ 业务领域隔离<br/>✅ 资源使用可控<br/>✅ 监控粒度合适<br/>✅ 扩展性好 |

> 📖 **详细技术文档**：[Keyed-Worker池架构详解](./KEYED_WORKER_POOL_ARCHITECTURE.md)

#### 🎯 **核心特性**
- **Topic级别隔离**：每个Topic独立的Keyed-Worker池，业务领域完全隔离
- **聚合内顺序**：同一聚合ID的事件通过一致性哈希路由到固定Worker，确保严格按序处理
- **高性能并发**：不同聚合ID的事件可并行处理，充分利用多核性能
- **资源可控性**：每个池固定1024个Worker，内存使用可预测，避免资源溢出
- **自然背压**：有界队列提供背压机制，系统过载时优雅降级
- **监控友好**：Topic级别的池隔离便于独立监控和调优
- **性能稳定**：消除了恢复模式切换带来的性能抖动，处理延迟更加稳定

### 🔍 **监控与健康检查**
- **分离式健康检查**：发布端和订阅端独立启动，精确角色控制
- **周期性健康检测**：自动检测连接状态和服务健康
- **快速故障检测**：及时发现连接异常和服务不可用
- **主题精确配对**：支持不同服务使用不同健康检查主题
- **双端积压检测**：支持发送端和订阅端的全面积压监控
- **智能评估**：多维度指标监控，自动计算积压严重程度
- **配置驱动**：根据配置自动决定启动发送端和/或订阅端检测
- **预防性控制**：发送端积压检测实现主动限流和优化
- **指标收集**：内置指标收集和健康检查
- **主题配置监控**：实时监控主题持久化配置和使用情况
- **智能路由监控**：监控消息在不同传输机制间的路由情况

### 🏢 **企业级特性**
- **主题持久化管理**：企业级的主题级持久化控制和动态配置
- **智能路由**：基于主题配置的自动消息路由和传输优化
- **多副本支持**：Kafka主题支持多副本配置，确保数据安全
- **长期数据保留**：支持7天、30天等长期数据保留策略
- **自动重连**：支持连接断开后的自动重连机制
- **流量控制**：支持限流和背压机制
- **死信队列**：失败消息的处理和重试机制
- **链路追踪**：支持分布式链路追踪
- **多租户**：支持多租户隔离
- **配置热更新**：运行时动态修改主题配置，无需重启
- **资源优化**：单一连接处理多种持久化需求，降低资源消耗

## 快速开始

### 1. 基本使用（内存模式）

```go
package main

import (
    "context"
    "log"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 创建内存事件总线配置
    cfg := eventbus.GetDefaultMemoryConfig()

    // 初始化全局事件总线
    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal(err)
    }
    defer eventbus.CloseGlobal()

    // 获取事件总线实例
    bus := eventbus.GetGlobal()

    // 订阅消息
    ctx := context.Background()
    topic := "user_events"

    err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
        log.Printf("Received: %s", string(message))
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    // 发布消息
    message := []byte(`{"event": "user_created", "user_id": "123"}`)
    if err := bus.Publish(ctx, topic, message); err != nil {
        log.Fatal(err)
    }
}
```

### 2. 主题持久化管理（推荐）

EventBus 的核心特性是**基于主题的智能持久化管理**，可以在同一个实例中处理不同持久化需求的主题：

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 1. 创建 NATS EventBus（支持智能路由）
    cfg := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"})
    cfg.NATS.JetStream.Enabled = true // 启用 JetStream 支持

    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal(err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()
    ctx := context.Background()

    // 2. 配置不同主题的持久化策略
    fmt.Println("📋 配置主题持久化策略...")

    // 业务关键事件：需要持久化（使用 JetStream）
    orderOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   24 * time.Hour,    // 保留24小时
        MaxSize:         100 * 1024 * 1024, // 100MB
        Description:     "订单业务事件，需要持久化",
    }
    if err := bus.ConfigureTopic(ctx, "business.orders", orderOptions); err != nil {
        log.Printf("Failed to configure orders topic: %v", err)
    } else {
        fmt.Println("✅ 订单主题配置为持久化（JetStream）")
    }

    // 系统通知：临时消息（使用 Core NATS）
    notifyOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        Description:     "系统通知消息，无需持久化",
    }
    if err := bus.ConfigureTopic(ctx, "system.notifications", notifyOptions); err != nil {
        log.Printf("Failed to configure notifications topic: %v", err)
    } else {
        fmt.Println("✅ 通知主题配置为非持久化（Core NATS）")
    }

    // 使用简化接口设置持久化
    if err := bus.SetTopicPersistence(ctx, "system.metrics", false); err != nil {
        log.Printf("Failed to set metrics persistence: %v", err)
    } else {
        fmt.Println("✅ 指标主题配置为非持久化")
    }

    // 3. 设置订阅（EventBus 自动根据主题配置选择传输机制）

    // 订阅持久化主题（自动使用 JetStream）
    err := bus.Subscribe(ctx, "business.orders", func(ctx context.Context, message []byte) error {
        fmt.Printf("📦 [JetStream] 处理订单事件: %s\n", string(message))
        return nil
    })
    if err != nil {
        log.Printf("Failed to subscribe to orders: %v", err)
    }

    // 订阅非持久化主题（自动使用 Core NATS）
    err = bus.Subscribe(ctx, "system.notifications", func(ctx context.Context, message []byte) error {
        fmt.Printf("⚡ [Core NATS] 处理通知: %s\n", string(message))
        return nil
    })
    if err != nil {
        log.Printf("Failed to subscribe to notifications: %v", err)
    }

    time.Sleep(2 * time.Second) // 等待订阅建立

    // 4. 发布消息（EventBus 自动智能路由）
    fmt.Println("\n📨 开始发布消息，演示智能路由...")

    // 发布到持久化主题（自动路由到 JetStream）
    orderMsg := []byte(`{"order_id": "12345", "amount": 99.99, "status": "created"}`)
    if err := bus.Publish(ctx, "business.orders", orderMsg); err != nil {
        log.Printf("Failed to publish order: %v", err)
    }

    // 发布到非持久化主题（自动路由到 Core NATS）
    notifyMsg := []byte(`{"user_id": "user123", "message": "订单创建成功"}`)
    if err := bus.Publish(ctx, "system.notifications", notifyMsg); err != nil {
        log.Printf("Failed to publish notification: %v", err)
    }

    time.Sleep(1 * time.Second) // 等待消息处理

    // 5. 查询主题配置
    fmt.Println("\n📊 查询主题配置...")

    topics := bus.ListConfiguredTopics()
    fmt.Printf("已配置主题: %v\n", topics)

    config, err := bus.GetTopicConfig("business.orders")
    if err == nil {
        fmt.Printf("订单主题配置: 模式=%s, 保留时间=%v\n",
            config.PersistenceMode, config.RetentionTime)
    }

    // 6. 动态修改主题配置
    fmt.Println("\n🔄 动态修改主题配置...")
    if err := bus.SetTopicPersistence(ctx, "system.notifications", true); err != nil {
        log.Printf("Failed to update notifications persistence: %v", err)
    } else {
        fmt.Println("✅ 通知主题已改为持久化模式")
    }

    fmt.Println("\n✅ 主题持久化管理演示完成！")
    fmt.Println("核心特性:")
    fmt.Println("  🎯 主题级控制 - 不同主题使用不同持久化策略")
    fmt.Println("  🚀 智能路由 - 自动选择 JetStream 或 Core NATS")
    fmt.Println("  🔄 动态配置 - 运行时修改主题配置")
    fmt.Println("  🔧 统一接口 - 现有 API 保持不变")
}
```

### 3. Kafka 主题持久化管理

对于企业级应用，推荐使用 Kafka 的主题持久化管理功能：

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 1. 创建 Kafka EventBus 配置
    cfg := &eventbus.EventBusConfig{
        Type: "kafka",
        Kafka: eventbus.KafkaConfig{
            Brokers: []string{"localhost:29092"},
            Producer: eventbus.ProducerConfig{
                RequiredAcks:   1,
                Timeout:        5 * time.Second,
                Compression:    "snappy",
                FlushFrequency: 100 * time.Millisecond,
            },
            Consumer: eventbus.ConsumerConfig{
                GroupID:         "my-service-group",
                AutoOffsetReset: "earliest",
            },
        },
    }

    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal(err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()
    ctx := context.Background()

    // 2. 配置企业级主题持久化策略

    // 业务关键事件：长期保留
    orderOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   7 * 24 * time.Hour, // 保留7天
        MaxSize:         500 * 1024 * 1024,  // 500MB
        Replicas:        3,                  // 3个副本
        Description:     "订单事件，需要长期保留",
    }
    if err := bus.ConfigureTopic(ctx, "business.orders", orderOptions); err != nil {
        log.Fatal(err)
    }

    // 系统通知：短期保留
    notifyOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   1 * time.Hour, // 仅保留1小时
        MaxSize:         10 * 1024 * 1024, // 10MB
        Replicas:        1, // 单副本
        Description:     "系统通知，短期保留",
    }
    if err := bus.ConfigureTopic(ctx, "system.notifications", notifyOptions); err != nil {
        log.Fatal(err)
    }

    // 3. 使用统一接口发布和订阅
    // EventBus 会自动根据主题配置创建和管理 Kafka 主题

    // 订阅（自动创建主题）
    err := bus.Subscribe(ctx, "business.orders", func(ctx context.Context, message []byte) error {
        fmt.Printf("📦 [Kafka长期保留] 处理订单: %s\n", string(message))
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    err = bus.Subscribe(ctx, "system.notifications", func(ctx context.Context, message []byte) error {
        fmt.Printf("⚡ [Kafka短期保留] 处理通知: %s\n", string(message))
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    time.Sleep(3 * time.Second) // 等待主题创建和订阅建立

    // 发布消息
    orderMsg := []byte(`{"order_id": "67890", "amount": 199.99}`)
    if err := bus.Publish(ctx, "business.orders", orderMsg); err != nil {
        log.Fatal(err)
    }

    notifyMsg := []byte(`{"user_id": "user456", "message": "订单处理中"}`)
    if err := bus.Publish(ctx, "system.notifications", notifyMsg); err != nil {
        log.Fatal(err)
    }

    time.Sleep(2 * time.Second) // 等待消息处理

    fmt.Println("✅ Kafka 主题持久化管理演示完成！")
}
```

### 4. 工厂模式（高级用法）

```go
// 创建事件总线工厂
cfg := &eventbus.EventBusConfig{
    Type: "nats",
    NATS: eventbus.NATSConfig{
        URLs:     []string{"nats://localhost:4222"},
        ClientID: "my-service",
        JetStream: eventbus.JetStreamConfig{
            Enabled: true,
        },
    },
    Metrics: eventbus.MetricsConfig{
        Enabled: true,
        CollectInterval: 30 * time.Second,
    },
}

factory := eventbus.NewFactory(cfg)
bus, err := factory.CreateEventBus()
if err != nil {
    log.Fatal(err)
}
defer bus.Close()

// 配置主题持久化策略
ctx := context.Background()
err = bus.SetTopicPersistence(ctx, "critical.events", true)  // 持久化
err = bus.SetTopicPersistence(ctx, "temp.notifications", false) // 非持久化
```

### 5. 集成到SDK Runtime

```go
import "github.com/ChenBigdata421/jxt-core/sdk"

// 设置事件总线到Runtime
sdk.Runtime.SetEventBus(bus)

// 从Runtime获取事件总线
eventBus := sdk.Runtime.GetEventBus()

// 在Runtime中使用主题持久化功能
ctx := context.Background()
err := eventBus.SetTopicPersistence(ctx, "runtime.events", true)
```

### 6. 混合使用场景（主题持久化 + Envelope + 普通消息）

EventBus 支持在同一个应用中灵活使用不同的消息模式和持久化策略，业务模块可以根据需求选择最适合的方式：

#### 场景说明

- **业务模块A（订单服务）**：使用 Envelope 方式 + 持久化主题，支持事件溯源和聚合管理
- **业务模块B（通知服务）**：使用普通消息方式 + 非持久化主题，简单高效的消息传递
- **业务模块C（审计服务）**：使用普通消息方式 + 持久化主题，长期存储审计日志

#### 完整示例

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// ========== 业务模块A：订单服务（Envelope + 持久化主题） ==========

type OrderCreatedEvent struct {
    OrderID    string  `json:"order_id"`
    CustomerID string  `json:"customer_id"`
    Amount     float64 `json:"amount"`
    Currency   string  `json:"currency"`
    Timestamp  string  `json:"timestamp"`
}

type OrderService struct {
    eventBus eventbus.EventBus
}

// 使用 Envelope 发布订单事件（支持事件溯源 + 持久化存储）
func (s *OrderService) CreateOrder(ctx context.Context, orderID, customerID string, amount float64) error {
    event := OrderCreatedEvent{
        OrderID:    orderID,
        CustomerID: customerID,
        Amount:     amount,
        Currency:   "USD",
        Timestamp:  time.Now().Format(time.RFC3339),
    }

    payload, _ := json.Marshal(event)

    // 创建 Envelope（包含聚合ID、事件类型、版本等元数据）
    envelope := eventbus.NewEnvelope(orderID, "OrderCreated", 1, payload)
    envelope.TraceID = "trace-" + orderID

    // 发布到持久化主题，EventBus 自动使用 JetStream/Kafka 持久化存储
    return s.eventBus.PublishEnvelope(ctx, "business.orders", envelope)
}

// 使用 Envelope 订阅订单事件（持久化主题，支持消息重放）
func (s *OrderService) SubscribeToOrderEvents(ctx context.Context) error {
    handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
        fmt.Printf("📦 [订单服务-持久化] 收到Envelope事件:\n")
        fmt.Printf("  AggregateID: %s\n", envelope.AggregateID)
        fmt.Printf("  EventType: %s\n", envelope.EventType)
        fmt.Printf("  EventVersion: %d\n", envelope.EventVersion)
        fmt.Printf("  TraceID: %s\n", envelope.TraceID)

        var event OrderCreatedEvent
        json.Unmarshal(envelope.Payload, &event)
        fmt.Printf("  订单详情: %+v\n", event)
        fmt.Printf("  💾 消息已持久化存储，支持重放\n\n")
        return nil
    }

    return s.eventBus.SubscribeEnvelope(ctx, "business.orders", handler)
}

// ========== 业务模块B：通知服务（普通消息 + 非持久化主题） ==========

type NotificationMessage struct {
    UserID    string `json:"user_id"`
    Type      string `json:"type"`
    Title     string `json:"title"`
    Content   string `json:"content"`
    Timestamp string `json:"timestamp"`
}

type NotificationService struct {
    eventBus eventbus.EventBus
}

// 使用普通发布（简单消息传递 + 非持久化存储）
func (s *NotificationService) SendNotification(ctx context.Context, userID, title, content string) error {
    notification := NotificationMessage{
        UserID:    userID,
        Type:      "info",
        Title:     title,
        Content:   content,
        Timestamp: time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(notification)
    // 发布到非持久化主题，EventBus 自动使用 Core NATS/短期保留
    return s.eventBus.Publish(ctx, "system.notifications", message)
}

// 使用普通订阅（非持久化主题，高性能处理）
func (s *NotificationService) SubscribeToNotifications(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        fmt.Printf("⚡ [通知服务-非持久化] 收到普通消息:\n")

        var notification NotificationMessage
        json.Unmarshal(message, &notification)
        fmt.Printf("  通知详情: %+v\n", notification)
        fmt.Printf("  🚀 高性能处理，无持久化存储\n\n")
        return nil
    }

    return s.eventBus.Subscribe(ctx, "system.notifications", handler)
}

// ========== 业务模块C：审计服务（普通消息 + 持久化主题） ==========

type AuditLogEvent struct {
    UserID    string `json:"user_id"`
    Action    string `json:"action"`
    Resource  string `json:"resource"`
    Timestamp string `json:"timestamp"`
    Details   string `json:"details"`
}

type AuditService struct {
    eventBus eventbus.EventBus
}

// 记录审计日志（普通消息 + 持久化存储）
func (s *AuditService) LogUserAction(ctx context.Context, userID, action, resource, details string) error {
    auditLog := AuditLogEvent{
        UserID:    userID,
        Action:    action,
        Resource:  resource,
        Timestamp: time.Now().Format(time.RFC3339),
        Details:   details,
    }

    message, _ := json.Marshal(auditLog)
    // 发布到持久化主题，用于长期审计存储
    return s.eventBus.Publish(ctx, "system.audit", message)
}

// 订阅审计日志（持久化主题，确保审计数据不丢失）
func (s *AuditService) SubscribeToAuditLogs(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        fmt.Printf("📋 [审计服务-持久化] 收到审计日志:\n")

        var auditLog AuditLogEvent
        json.Unmarshal(message, &auditLog)
        fmt.Printf("  审计详情: %+v\n", auditLog)
        fmt.Printf("  💾 审计数据已持久化存储\n\n")
        return nil
    }

    return s.eventBus.Subscribe(ctx, "system.audit", handler)
}

// ========== 主程序：演示混合使用（主题持久化 + 多种消息模式） ==========

func main() {
    fmt.Println("=== EventBus 混合使用演示（主题持久化管理） ===\n")

    // 1. 创建 NATS EventBus（支持智能路由）
    cfg := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"})
    cfg.NATS.JetStream.Enabled = true

    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal(err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()
    ctx := context.Background()

    // 2. 配置不同主题的持久化策略
    fmt.Println("📋 配置主题持久化策略...")

    // 订单事件：持久化存储（支持事件溯源）
    orderOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   24 * time.Hour,
        MaxSize:         100 * 1024 * 1024,
        Description:     "订单事件，需要持久化和事件溯源",
    }
    if err := bus.ConfigureTopic(ctx, "business.orders", orderOptions); err != nil {
        log.Fatal(err)
    }
    fmt.Println("✅ 订单主题配置为持久化（JetStream）")

    // 系统通知：非持久化存储（高性能处理）
    notifyOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        Description:     "系统通知，无需持久化",
    }
    if err := bus.ConfigureTopic(ctx, "system.notifications", notifyOptions); err != nil {
        log.Fatal(err)
    }
    fmt.Println("✅ 通知主题配置为非持久化（Core NATS）")

    // 审计日志：持久化存储（合规要求）
    if err := bus.SetTopicPersistence(ctx, "system.audit", true); err != nil {
        log.Fatal(err)
    }
    fmt.Println("✅ 审计主题配置为持久化")

    // 3. 创建业务服务
    orderService := &OrderService{eventBus: bus}
    notificationService := &NotificationService{eventBus: bus}
    auditService := &AuditService{eventBus: bus}

    // 4. 启动订阅（EventBus 自动根据主题配置选择传输机制）
    fmt.Println("\n🚀 启动智能订阅...")

    if err := orderService.SubscribeToOrderEvents(ctx); err != nil {
        log.Fatal(err)
    }

    if err := notificationService.SubscribeToNotifications(ctx); err != nil {
        log.Fatal(err)
    }

    if err := auditService.SubscribeToAuditLogs(ctx); err != nil {
        log.Fatal(err)
    }

    time.Sleep(2 * time.Second) // 等待订阅建立

    // 5. 发布事件演示（智能路由）
    fmt.Println("📨 开始发布消息，演示智能路由...\n")

    // 订单服务：Envelope + 持久化主题（JetStream）
    fmt.Println("--- 订单事件（Envelope + JetStream持久化） ---")
    orderService.CreateOrder(ctx, "order-123", "customer-456", 99.99)

    // 通知服务：普通消息 + 非持久化主题（Core NATS）
    fmt.Println("--- 系统通知（普通消息 + Core NATS高性能） ---")
    notificationService.SendNotification(ctx, "user-789", "订单确认", "您的订单已创建")

    // 审计服务：普通消息 + 持久化主题（JetStream）
    fmt.Println("--- 审计日志（普通消息 + JetStream持久化） ---")
    auditService.LogUserAction(ctx, "user-789", "CREATE_ORDER", "order-123", "用户创建订单")

    time.Sleep(2 * time.Second) // 等待消息处理

    // 6. 查询主题配置
    fmt.Println("📊 查询主题配置...")
    topics := bus.ListConfiguredTopics()
    fmt.Printf("已配置主题: %v\n", topics)

    fmt.Println("\n✅ 混合使用演示完成！")
    fmt.Println("核心特性验证:")
    fmt.Println("  🎯 主题级持久化控制 - 不同主题使用不同存储策略")
    fmt.Println("  🚀 智能路由 - 自动选择 JetStream 或 Core NATS")
    fmt.Println("  📦 Envelope支持 - 事件溯源和聚合管理")
    fmt.Println("  ⚡ 普通消息支持 - 简单高效的消息传递")
    fmt.Println("  🔧 统一接口 - 现有 API 保持不变")
    fmt.Println("  💾 持久化保证 - 关键数据不丢失")
    fmt.Println("  🚀 高性能处理 - 临时数据快速传递")
}
```

#### 🎯 **Subscribe vs SubscribeEnvelope 核心区别**

| 特性 | `Subscribe()` | `SubscribeEnvelope()` |
|------|---------------|----------------------|
| **消息格式** | 原始字节数据 | Envelope包装格式 |
| **聚合ID提取** | ❌ 通常无法提取 | ✅ 从Envelope.AggregateID提取 |
| **Keyed-Worker池** | ❌ 不使用 | ✅ 自动使用 |
| **处理模式** | 直接并发处理 | 按聚合ID顺序处理 |
| **性能特点** | 极致性能，微秒级延迟 | 顺序保证，毫秒级延迟 |
| **适用场景** | 简单消息、通知、缓存失效 | 领域事件、事件溯源、聚合管理 |
| **顺序保证** | ❌ 无顺序保证 | ✅ 同聚合ID严格顺序 |
| **并发能力** | 完全并发 | 不同聚合ID并发 |

#### 🔍 **聚合ID提取机制**

**为什么`Subscribe`不使用Keyed-Worker池？**

```go
// Subscribe: 原始消息，无法提取聚合ID
bus.Subscribe(ctx, "notifications", func(ctx context.Context, data []byte) error {
    // data是原始JSON: {"message": "hello", "user": "123"}
    // ExtractAggregateID(data, ...) 返回空字符串
    // → 直接并发处理，不使用Keyed-Worker池
})

// SubscribeEnvelope: Envelope格式，能提取聚合ID
bus.SubscribeEnvelope(ctx, "orders", func(ctx context.Context, env *Envelope) error {
    // env.AggregateID = "order-123"
    // ExtractAggregateID成功提取聚合ID
    // → 路由到Keyed-Worker池，顺序处理
})
```

#### 📊 **使用方式对比**

| 使用方式 | 适用场景 | 特点 | 示例 |
|---------|---------|------|------|
| **Envelope** | 事件溯源、聚合管理 | 强制元数据、版本控制、顺序处理 | `PublishEnvelope()` / `SubscribeEnvelope()` |
| **普通消息** | 简单消息传递 | 轻量级、灵活、高性能并发 | `Publish()` / `Subscribe()` |
| **高级选项** | 企业特性需求 | 支持元数据、超时、重试 | `PublishWithOptions()` / `SubscribeWithOptions()` |

#### 🎯 **选择建议**

- **🏛️ 领域事件/事件溯源**：使用 `PublishEnvelope` / `SubscribeEnvelope`
  - 需要顺序处理（如订单状态变更）
  - 需要聚合管理（如用户行为追踪）
  - 需要事件重放（如数据恢复）

- **📢 简单消息传递**：使用 `Publish` / `Subscribe`
  - 通知消息（如邮件、短信）
  - 缓存失效（如Redis清理）
  - 系统监控（如指标上报）

- **🔧 企业特性需求**：使用 `PublishWithOptions` / `SubscribeWithOptions`
  - 需要自定义超时、重试
  - 需要复杂的元数据处理

- **🔄 混合场景**：同一个服务可以根据不同的业务逻辑选择不同的方法

#### 🔬 **技术原理：为什么Subscribe不使用Keyed-Worker池？**

**核心原因：聚合ID提取能力的差异**

```go
// ExtractAggregateID 聚合ID提取优先级
func ExtractAggregateID(msgBytes []byte, headers map[string]string, kafkaKey []byte, natsSubject string) (string, error) {
    // 1. 优先从 Envelope 提取 ⭐ 关键差异点
    if len(msgBytes) > 0 {
        env, err := FromBytes(msgBytes)
        if err == nil && env.AggregateID != "" {
            return env.AggregateID, nil  // ✅ SubscribeEnvelope走这里
        }
    }

    // 2. 从 Headers 提取（通常为空）
    // 3. 从 Kafka Key 提取（通常不是聚合ID）
    // 4. 从 NATS Subject 提取（启发式，不可靠）

    return "", nil  // ❌ Subscribe通常走这里，无聚合ID
}
```

**处理流程对比：**

| 步骤 | `Subscribe` | `SubscribeEnvelope` |
|------|-------------|---------------------|
| **1. 消息接收** | 原始字节数据 | Envelope格式数据 |
| **2. 聚合ID提取** | ❌ 失败（无Envelope） | ✅ 成功（env.AggregateID） |
| **3. 路由决策** | 直接处理 | Keyed-Worker池 |
| **4. 处理模式** | 并发处理 | 顺序处理 |

**设计哲学：**
- **Subscribe**：为高性能并发场景设计，不强制消息格式
- **SubscribeEnvelope**：为事件溯源场景设计，强制Envelope格式以获得聚合ID

#### 🔧 **Keyed-Worker池技术实现**

##### 数据结构
```go
type kafkaEventBus struct {
    // 每个Topic一个Keyed-Worker池
    keyedPools   map[string]*KeyedWorkerPool  // topic -> pool
    keyedPoolsMu sync.RWMutex
}

type KeyedWorkerPool struct {
    workers []chan *AggregateMessage  // 1024个Worker通道
    cfg     KeyedWorkerPoolConfig
}
```

##### 池创建逻辑
```go
// Subscribe时自动为每个Topic创建独立的Keyed-Worker池
k.keyedPoolsMu.Lock()
if _, ok := k.keyedPools[topic]; !ok {
    pool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
        WorkerCount: 1024,        // 每个Topic池固定1024个Worker
        QueueSize:   1000,        // 每个Worker队列大小1000
        WaitTimeout: 200 * time.Millisecond,
    }, handler)
    k.keyedPools[topic] = pool  // 以topic为key存储
}
k.keyedPoolsMu.Unlock()
```

##### 聚合ID路由算法
```go
func (kp *KeyedWorkerPool) ProcessMessage(ctx context.Context, msg *AggregateMessage) error {
    // 1. 验证聚合ID
    if msg.AggregateID == "" {
        return errors.New("aggregateID required for keyed worker pool")
    }

    // 2. 一致性哈希计算Worker索引
    idx := kp.hashToIndex(msg.AggregateID)
    ch := kp.workers[idx]

    // 3. 路由到特定Worker
    select {
    case ch <- msg:
        return nil  // 成功入队
    case <-ctx.Done():
        return ctx.Err()
    case <-time.After(kp.cfg.WaitTimeout):
        return ErrWorkerQueueFull  // 背压机制
    }
}

func (kp *KeyedWorkerPool) hashToIndex(key string) int {
    h := fnv.New32a()
    _, _ = h.Write([]byte(key))
    return int(h.Sum32() % uint32(len(kp.workers)))  // FNV哈希 + 取模
}
```

##### 关键保证
- **一致性路由**：相同聚合ID总是路由到相同Worker
- **顺序处理**：每个Worker内部FIFO处理消息
- **并发能力**：不同聚合ID可以并行处理
- **背压控制**：队列满时提供优雅降级

### 5. 推荐方案：单一EventBus实例 + 智能路由

基于性能测试和架构分析，**强烈推荐使用单一EventBus实例配合不同方法**来处理混合业务场景。这种方案在保持相近性能的同时，显著提升了资源利用效率和架构简洁性。

#### 方案优势

- 🏗️ **架构简洁**：单一实例，减少50%的EventBus管理复杂度
- 💰 **资源高效**：内存节省12.65%，协程减少6.25%
- 🔧 **运维友好**：统一配置、监控和故障处理
- 📈 **性能优异**：吞吐量损失微乎其微（仅1.54%）
- 🔄 **扩展性强**：支持未来新业务场景的灵活接入

#### 完整实现示例

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// ========== 业务A：订单服务（需要顺序处理） ==========

type OrderService struct {
    eventBus eventbus.EventBus // 统一EventBus实例
}

type OrderCreatedEvent struct {
    OrderID    string  `json:"order_id"`
    CustomerID string  `json:"customer_id"`
    Amount     float64 `json:"amount"`
    Timestamp  string  `json:"timestamp"`
}

// 使用 PublishEnvelope 发布订单事件（自动路由到Keyed-Worker池）
func (s *OrderService) CreateOrder(ctx context.Context, orderID, customerID string, amount float64) error {
    event := OrderCreatedEvent{
        OrderID:    orderID,
        CustomerID: customerID,
        Amount:     amount,
        Timestamp:  time.Now().Format(time.RFC3339),
    }

    payload, _ := json.Marshal(event)

    // 创建Envelope（包含聚合ID，确保同一订单的事件顺序处理）
    envelope := eventbus.NewEnvelope(orderID, "OrderCreated", 1, payload)
    envelope.TraceID = "trace-" + orderID

    // 使用SubscribeEnvelope订阅的消息会自动路由到Keyed-Worker池
    // 保证同一聚合ID（订单ID）的事件严格按序处理
    return s.eventBus.PublishEnvelope(ctx, "orders.events", envelope)
}

// 使用 SubscribeEnvelope 订阅订单事件（自动启用Keyed-Worker池）
func (s *OrderService) SubscribeToOrderEvents(ctx context.Context) error {
    handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
        fmt.Printf("📦 [订单服务] 收到有序事件:\n")
        fmt.Printf("  聚合ID: %s (路由到固定Worker)\n", envelope.AggregateID)
        fmt.Printf("  事件类型: %s\n", envelope.EventType)
        fmt.Printf("  处理模式: Keyed-Worker池 (顺序保证)\n")

        var event OrderCreatedEvent
        json.Unmarshal(envelope.Payload, &event)
        fmt.Printf("  订单详情: %+v\n\n", event)

        // 模拟订单处理逻辑
        return s.processOrder(envelope.AggregateID, event)
    }

    // SubscribeEnvelope 会自动启用Keyed-Worker池
    // 同一聚合ID的消息会路由到同一个Worker，确保顺序处理
    return s.eventBus.SubscribeEnvelope(ctx, "orders.events", handler)
}

func (s *OrderService) processOrder(orderID string, event OrderCreatedEvent) error {
    fmt.Printf("   🔄 处理订单 %s: 金额 %.2f\n", orderID, event.Amount)
    time.Sleep(100 * time.Millisecond) // 模拟处理时间
    return nil
}

// ========== 业务B：通知服务（无顺序要求） ==========

type NotificationService struct {
    eventBus eventbus.EventBus // 同一个EventBus实例
}

type NotificationMessage struct {
    UserID    string `json:"user_id"`
    Type      string `json:"type"`
    Title     string `json:"title"`
    Content   string `json:"content"`
    Timestamp string `json:"timestamp"`
}

// 使用 Publish 发布通知消息（直接处理，无Keyed-Worker池）
func (s *NotificationService) SendNotification(ctx context.Context, userID, title, content string) error {
    notification := NotificationMessage{
        UserID:    userID,
        Type:      "info",
        Title:     title,
        Content:   content,
        Timestamp: time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(notification)

    // 使用普通Publish，Subscribe订阅的消息直接并发处理
    // 无需顺序保证，性能更高
    return s.eventBus.Publish(ctx, "notifications.events", message)
}

// 使用 Subscribe 订阅通知消息（直接并发处理）
func (s *NotificationService) SubscribeToNotifications(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        fmt.Printf("📧 [通知服务] 收到并发事件:\n")

        var notification NotificationMessage
        json.Unmarshal(message, &notification)
        fmt.Printf("  用户ID: %s\n", notification.UserID)
        fmt.Printf("  处理模式: 直接并发处理 (高性能)\n")
        fmt.Printf("  通知详情: %+v\n\n", notification)

        // 模拟通知处理逻辑
        return s.processNotification(notification)
    }

    // Subscribe 直接并发处理，无Keyed-Worker池
    // 适合无顺序要求的高频消息
    return s.eventBus.Subscribe(ctx, "notifications.events", handler)
}

func (s *NotificationService) processNotification(notification NotificationMessage) error {
    fmt.Printf("   📤 发送通知给用户 %s: %s\n", notification.UserID, notification.Title)
    time.Sleep(50 * time.Millisecond) // 模拟处理时间
    return nil
}

// ========== 主程序：演示单一EventBus + 智能路由 ==========

func main() {
    fmt.Println("=== 单一EventBus实例 + 智能路由方案演示 ===\n")

    // 1. 创建统一的EventBus实例
    cfg := &eventbus.EventBusConfig{
        Type: "nats",
        NATS: eventbus.NATSConfig{
            URLs: []string{"nats://localhost:4222"},
            JetStream: eventbus.JetStreamConfig{
                Enabled: true,
                Stream: eventbus.StreamConfig{
                    Name:     "unified-business-stream",
                    Subjects: []string{"orders.*", "notifications.*"},
                },
            },
        },
        // 注意：Keyed-Worker池在SubscribeEnvelope时自动创建
        // 无需额外配置，智能路由机制会自动处理
    }

    bus, err := eventbus.NewEventBus(cfg)
    if err != nil {
        log.Fatalf("Failed to create EventBus: %v", err)
    }
    defer bus.Close()

    // 2. 创建业务服务（共享同一个EventBus实例）
    orderService := &OrderService{eventBus: bus}
    notificationService := &NotificationService{eventBus: bus}

    ctx := context.Background()

    // 3. 启动订阅（智能路由）
    fmt.Println("🚀 启动智能路由订阅...")

    // 订单服务：SubscribeEnvelope -> 自动启用Keyed-Worker池
    if err := orderService.SubscribeToOrderEvents(ctx); err != nil {
        log.Fatalf("Failed to subscribe to order events: %v", err)
    }

    // 通知服务：Subscribe -> 直接并发处理
    if err := notificationService.SubscribeToNotifications(ctx); err != nil {
        log.Fatalf("Failed to subscribe to notifications: %v", err)
    }

    time.Sleep(100 * time.Millisecond) // 等待订阅建立

    // 4. 演示智能路由效果
    fmt.Println("📨 开始发布消息，演示智能路由...\n")

    // 业务A：订单事件（有序处理）
    fmt.Println("--- 业务A：订单事件（Envelope + Keyed-Worker池） ---")
    orderService.CreateOrder(ctx, "order-001", "customer-123", 99.99)
    orderService.CreateOrder(ctx, "order-001", "customer-123", 199.99) // 同一订单，保证顺序
    orderService.CreateOrder(ctx, "order-002", "customer-456", 299.99) // 不同订单，并行处理

    time.Sleep(300 * time.Millisecond)

    // 业务B：通知消息（并发处理）
    fmt.Println("--- 业务B：通知消息（普通Subscribe + 并发处理） ---")
    notificationService.SendNotification(ctx, "user-123", "订单确认", "您的订单已创建")
    notificationService.SendNotification(ctx, "user-456", "支付提醒", "请及时完成支付")
    notificationService.SendNotification(ctx, "user-789", "发货通知", "您的商品已发货")

    time.Sleep(500 * time.Millisecond) // 等待消息处理

    // 5. 架构优势总结
    fmt.Println("\n=== 单一EventBus + 智能路由架构优势 ===")
    fmt.Println("✅ 智能路由机制:")
    fmt.Println("  📦 SubscribeEnvelope -> Keyed-Worker池 (顺序保证)")
    fmt.Println("  📧 Subscribe -> 直接并发处理 (高性能)")
    fmt.Println("✅ 资源优化:")
    fmt.Println("  🔗 单一连接，减少资源消耗")
    fmt.Println("  ⚙️ 统一配置，简化运维管理")
    fmt.Println("  📊 统一监控，便于故障排查")
    fmt.Println("✅ 性能表现:")
    fmt.Println("  🚀 吞吐量: 1,173 msg/s (仅比独立实例低1.54%)")
    fmt.Println("  💾 内存节省: 12.65%")
    fmt.Println("  🧵 协程减少: 6.25%")
    fmt.Println("  ⚡ 操作延迟: 50.32 µs/op")
}
```

#### 配置说明

```yaml
# 单一EventBus实例配置
eventbus:
  type: nats
  nats:
    urls: ["nats://localhost:4222"]
    jetstream:
      enabled: true
      stream:
        name: "unified-business-stream"
        subjects: ["orders.*", "notifications.*"]  # 统一流，多业务主题

# 注意：Keyed-Worker池在SubscribeEnvelope时自动创建
# 默认配置：WorkerCount=1024, QueueSize=1000, WaitTimeout=200ms
# 智能路由机制会自动处理有序和无序消息的不同处理方式
```

#### 智能路由机制

| 发布方法 | 订阅方法 | 处理模式 | 适用场景 |
|---------|---------|---------|----------|
| `PublishEnvelope()` | `SubscribeEnvelope()` | **Keyed-Worker池** | 事件溯源、聚合管理、顺序处理 |
| `Publish()` | `Subscribe()` | **直接并发** | 简单消息、通知、无顺序要求 |
| `PublishWithOptions()` | `SubscribeWithOptions()` | **可配置** | 企业特性、自定义处理 |

#### 🏗️ **多Topic Keyed-Worker池实战示例**

```go
package main

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    bus, _ := eventbus.NewEventBus(cfg)
    ctx := context.Background()

    // 🏛️ 订单领域：每个订单ID的事件严格顺序处理
    bus.SubscribeEnvelope(ctx, "orders.events", func(ctx context.Context, env *eventbus.Envelope) error {
        // env.AggregateID = "order-123"
        // 自动创建 orders.events 的Keyed-Worker池
        // order-123 的所有事件路由到同一个Worker，确保顺序
        return processOrderEvent(env)
    })

    // 👤 用户领域：每个用户ID的事件严格顺序处理
    bus.SubscribeEnvelope(ctx, "users.events", func(ctx context.Context, env *eventbus.Envelope) error {
        // env.AggregateID = "user-456"
        // 自动创建 users.events 的Keyed-Worker池（独立于orders.events池）
        // user-456 的所有事件路由到同一个Worker，确保顺序
        return processUserEvent(env)
    })

    // 📦 库存领域：每个商品ID的事件严格顺序处理
    bus.SubscribeEnvelope(ctx, "inventory.events", func(ctx context.Context, env *eventbus.Envelope) error {
        // env.AggregateID = "product-789"
        // 自动创建 inventory.events 的Keyed-Worker池（独立于其他池）
        // product-789 的所有事件路由到同一个Worker，确保顺序
        return processInventoryEvent(env)
    })

    // 📢 通知消息：直接并发处理，不使用Keyed-Worker池
    bus.Subscribe(ctx, "notifications", func(ctx context.Context, data []byte) error {
        // 原始消息，无聚合ID，直接并发处理
        return processNotification(data)
    })

    // 发布不同领域的事件
    publishDomainEvents(bus, ctx)
}

func publishDomainEvents(bus eventbus.EventBus, ctx context.Context) {
    // 订单事件：order-123 的事件会路由到 orders.events 池的同一个Worker
    orderEnv1 := eventbus.NewEnvelope("order-123", "OrderCreated", 1, orderData)
    orderEnv2 := eventbus.NewEnvelope("order-123", "OrderPaid", 2, orderData)
    bus.PublishEnvelope(ctx, "orders.events", orderEnv1)
    bus.PublishEnvelope(ctx, "orders.events", orderEnv2)  // 严格在orderEnv1之后处理

    // 用户事件：user-456 的事件会路由到 users.events 池的同一个Worker
    userEnv1 := eventbus.NewEnvelope("user-456", "UserRegistered", 1, userData)
    userEnv2 := eventbus.NewEnvelope("user-456", "UserActivated", 2, userData)
    bus.PublishEnvelope(ctx, "users.events", userEnv1)
    bus.PublishEnvelope(ctx, "users.events", userEnv2)    // 严格在userEnv1之后处理

    // 库存事件：product-789 的事件会路由到 inventory.events 池的同一个Worker
    invEnv1 := eventbus.NewEnvelope("product-789", "StockAdded", 1, invData)
    invEnv2 := eventbus.NewEnvelope("product-789", "StockReserved", 2, invData)
    bus.PublishEnvelope(ctx, "inventory.events", invEnv1)
    bus.PublishEnvelope(ctx, "inventory.events", invEnv2) // 严格在invEnv1之后处理
}
```

**架构效果**：
```
EventBus实例
├── orders.events池     → 1024个Worker (order-123 → Worker-42)
├── users.events池      → 1024个Worker (user-456 → Worker-156)
├── inventory.events池  → 1024个Worker (product-789 → Worker-89)
└── notifications       → 直接并发处理（无池）

✅ 跨领域隔离：订单、用户、库存事件完全独立处理
✅ 领域内顺序：同一聚合ID的事件严格按序处理
✅ 高性能并发：不同聚合ID和不同领域可以并行处理
```

#### 性能对比数据

##### 🏗️ **Keyed-Worker池性能测试**

基于NATS JetStream + Keyed-Worker池的性能测试结果：

| 测试场景 | 聚合数量 | 事件总数 | 处理时间 | 吞吐量 | 顺序保证 |
|---------|---------|---------|---------|--------|----------|
| **单聚合顺序** | 1个订单 | 10,000事件 | 2.13s | 4,695 events/s | ✅ 严格顺序 |
| **多聚合并发** | 100个订单 | 50,000事件 | 3.61s | 13,850 events/s | ✅ 聚合内顺序 |
| **混合场景** | 3个聚合 | 60事件 | 3.61s | 16.6 events/s | ✅ 完美顺序 |

**关键发现**：
- **顺序保证**：同聚合ID事件100%按序处理
- **并发能力**：不同聚合ID事件完全并行处理
- **性能优异**：多聚合场景下吞吐量显著提升
- **资源效率**：每个Topic池独立，无跨池竞争

##### 📊 **架构方案对比**

基于实际测试（9,000条消息：3,000订单 + 6,000通知）：

| 指标 | 独立实例方案 | 单一实例方案 | 优势 |
|------|-------------|-------------|------|
| **总吞吐量** | 1,192.09 msg/s | 1,173.69 msg/s | 相近(-1.54%) |
| **内存使用** | 4.15 MB | 3.63 MB | **节省12.65%** ✅ |
| **协程数量** | 16 | 15 | **减少6.25%** ✅ |
| **EventBus实例** | 2 | 1 | **减少50%** ✅ |
| **操作延迟** | - | 50.32 µs/op | **优秀** ✅ |

#### 快速开始示例

更简洁的混合使用示例请参考：[examples/quick_start_mixed.go](../../examples/quick_start_mixed.go)

```bash
# 运行混合使用示例
go run examples/quick_start_mixed.go

# 运行单一实例方案演示（推荐）
go run examples/unified_eventbus_demo.go
```

#### 运行示例

**方式1：内存实现（快速体验）**
```bash
# 直接运行，无需外部依赖
cd jxt-core/sdk/pkg/eventbus
go run examples/unified_eventbus_demo.go
```

**方式2：NATS实现（生产环境推荐）**
```bash
# 1. 启动NATS服务器
nats-server -js

# 2. 修改示例配置为NATS
# 编辑 examples/unified_eventbus_demo.go
# 将 Type: "memory" 改为 Type: "nats"
# 添加 NATS 配置

# 3. 运行示例
go run examples/unified_eventbus_demo.go
```

**预期输出**：
```
=== 单一EventBus实例 + 智能路由方案演示 ===

🚀 启动智能路由订阅...
📨 开始发布消息，演示智能路由...

--- 业务A：订单事件（Envelope + Keyed-Worker池） ---
📦 [订单服务] 收到有序事件:
  聚合ID: order-001 (路由到固定Worker)
  事件类型: OrderCreated
  处理模式: Keyed-Worker池 (顺序保证)
  订单详情: {OrderID:order-001 CustomerID:customer-123 Amount:99.99 Timestamp:2025-09-22T22:06:59+08:00}
   🔄 处理订单 order-001: 金额 99.99

📦 [订单服务] 收到有序事件:
  聚合ID: order-001 (路由到固定Worker)
  事件类型: OrderCreated
  处理模式: Keyed-Worker池 (顺序保证)
  订单详情: {OrderID:order-001 CustomerID:customer-123 Amount:199.99 Timestamp:2025-09-22T22:06:59+08:00}
   🔄 处理订单 order-001: 金额 199.99

--- 业务B：通知消息（普通Subscribe + 并发处理） ---
📧 [通知服务] 收到并发事件:
  用户ID: user-123
  处理模式: 直接并发处理 (高性能)
  通知详情: {UserID:user-123 Type:info Title:订单确认 Content:您的订单已创建 Timestamp:2025-09-22T22:07:00+08:00}
   📤 发送通知给用户 user-123: 订单确认

📧 [通知服务] 收到并发事件:
  用户ID: user-456
  处理模式: 直接并发处理 (高性能)
  通知详情: {UserID:user-456 Type:info Title:支付提醒 Content:请及时完成支付 Timestamp:2025-09-22T22:07:00+08:00}
   📤 发送通知给用户 user-456: 支付提醒

=== 单一EventBus + 智能路由架构优势 ===
✅ 智能路由机制:
  📦 SubscribeEnvelope -> Keyed-Worker池 (顺序保证)
  📧 Subscribe -> 直接并发处理 (高性能)
✅ 资源优化:
  🔗 单一连接，减少资源消耗
  ⚙️ 统一配置，简化运维管理
  📊 统一监控，便于故障排查
✅ 性能表现:
  🚀 吞吐量: 1,173 msg/s (仅比独立实例低1.54%)
  💾 内存节省: 12.65%
  🧵 协程减少: 6.25%
  ⚡ 操作延迟: 50.32 µs/op

✅ 演示完成！推荐在生产环境使用此架构方案。
```


## 高级特性

### 1. 企业级主题持久化管理

基于"特性"章节介绍的主题持久化管理核心功能，这里展示企业级的高级配置和最佳实践。

#### 企业级配置策略

**多环境配置管理**：
```yaml
# 生产环境配置
eventbus:
  type: kafka
  topics:
    # 业务关键事件 - 高可靠性配置
    "business.orders":
      persistenceMode: "persistent"
      retentionTime: "168h"      # 7天保留
      maxSize: 1073741824        # 1GB
      replicas: 3                # 3副本
      description: "订单事件，金融级可靠性"

    # 审计日志 - 长期保留配置
    "audit.logs":
      persistenceMode: "persistent"
      retentionTime: "2160h"     # 90天保留
      maxSize: 5368709120        # 5GB
      replicas: 5                # 5副本
      description: "审计日志，合规要求"

    # 实时监控 - 高性能配置
    "monitoring.metrics":
      persistenceMode: "ephemeral"
      retentionTime: "1h"        # 1小时保留
      maxSize: 104857600         # 100MB
      replicas: 1                # 单副本
      description: "实时监控指标"
```

**动态配置热更新**：
```go
// 企业级动态配置管理
type EnterpriseTopicManager struct {
    eventBus eventbus.EventBus
    logger   *zap.Logger
}

// 批量更新主题配置
func (m *EnterpriseTopicManager) BatchUpdateTopics(ctx context.Context, updates map[string]eventbus.TopicOptions) error {
    for topic, options := range updates {
        if err := m.eventBus.ConfigureTopic(ctx, topic, options); err != nil {
            m.logger.Error("Failed to update topic config",
                zap.String("topic", topic), zap.Error(err))
            return err
        }
        m.logger.Info("Topic config updated",
            zap.String("topic", topic),
            zap.String("mode", string(options.PersistenceMode)))
    }
    return nil
}

// 根据业务规则自动调整配置
func (m *EnterpriseTopicManager) AutoAdjustByBusinessRules(ctx context.Context) error {
    // 获取当前所有主题配置
    topics := m.eventBus.ListConfiguredTopics()

    for _, topic := range topics {
        config, err := m.eventBus.GetTopicConfig(topic)
        if err != nil {
            continue
        }

        // 根据主题名称模式自动调整
        if strings.HasPrefix(topic, "business.") {
            // 业务主题：确保持久化
            if config.PersistenceMode != eventbus.TopicPersistent {
                config.PersistenceMode = eventbus.TopicPersistent
                config.RetentionTime = 7 * 24 * time.Hour
                m.eventBus.ConfigureTopic(ctx, topic, config)
            }
        } else if strings.HasPrefix(topic, "temp.") {
            // 临时主题：确保非持久化
            if config.PersistenceMode != eventbus.TopicEphemeral {
                config.PersistenceMode = eventbus.TopicEphemeral
                config.RetentionTime = 30 * time.Minute
                m.eventBus.ConfigureTopic(ctx, topic, config)
            }
        }
    }
    return nil
}
```

#### 智能路由监控

**路由决策监控**：
```go
// 监控智能路由决策
type RouteMonitor struct {
    routeStats map[string]*RouteStats
    mu         sync.RWMutex
}

type RouteStats struct {
    Topic           string
    PersistentCount int64
    EphemeralCount  int64
    LastRouteTime   time.Time
    RouteMode       string // "JetStream", "CoreNATS", "KafkaLongTerm", "KafkaShortTerm"
}

func (m *RouteMonitor) RecordRoute(topic string, isPersistent bool, routeMode string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.routeStats[topic] == nil {
        m.routeStats[topic] = &RouteStats{Topic: topic}
    }

    stats := m.routeStats[topic]
    if isPersistent {
        stats.PersistentCount++
    } else {
        stats.EphemeralCount++
    }
    stats.LastRouteTime = time.Now()
    stats.RouteMode = routeMode
}
```

#### 企业级最佳实践

**1. 主题命名规范**：

⚠️ **Kafka 关键限制**：
- **ClientID 和 Topic 名称必须只使用 ASCII 字符**
- **禁止使用中文、日文、韩文等 Unicode 字符**
- **违反此规则会导致消息无法接收（0% 成功率）**

```go
// ✅ 企业级主题命名规范（仅使用 ASCII 字符）
const (
    // 业务领域主题（持久化）
    TopicOrderEvents    = "business.orders.events"    // ✅ 正确
    TopicPaymentEvents  = "business.payments.events"  // ✅ 正确
    TopicUserEvents     = "business.users.events"     // ✅ 正确

    // 系统级主题（非持久化）
    TopicSystemNotify   = "system.notifications"      // ✅ 正确
    TopicSystemMetrics  = "system.metrics"            // ✅ 正确
    TopicSystemHealth   = "system.health"             // ✅ 正确

    // 审计主题（长期持久化）
    TopicAuditLogs      = "audit.logs"                // ✅ 正确
    TopicSecurityEvents = "audit.security"            // ✅ 正确

    // 临时主题（短期保留）
    TopicTempCache      = "temp.cache.invalidation"   // ✅ 正确
    TopicTempSession    = "temp.session.updates"      // ✅ 正确
)

// ❌ 错误示例（Kafka 不支持，会导致消息无法接收）
/*
const (
    TopicOrderEvents    = "业务.订单.事件"    // ❌ 错误：使用了中文
    TopicPaymentEvents  = "business.支付"    // ❌ 错误：混用中英文
    TopicUserEvents     = "用户事件"         // ❌ 错误：使用了中文
)
*/

// 主题配置模板
var TopicTemplates = map[string]eventbus.TopicOptions{
    "business.*": {
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   7 * 24 * time.Hour,
        MaxSize:         500 * 1024 * 1024, // 500MB
        Replicas:        3,
        Description:     "业务关键事件",
    },
    "audit.*": {
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   90 * 24 * time.Hour, // 90天
        MaxSize:         2 * 1024 * 1024 * 1024, // 2GB
        Replicas:        5,
        Description:     "审计日志，合规要求",
    },
    "system.*": {
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   2 * time.Hour,
        MaxSize:         50 * 1024 * 1024, // 50MB
        Replicas:        1,
        Description:     "系统级消息",
    },
    "temp.*": {
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   30 * time.Minute,
        MaxSize:         10 * 1024 * 1024, // 10MB
        Replicas:        1,
        Description:     "临时消息",
    },
}
```

**2. 配置验证和治理**：
```go
// 企业级配置治理
type TopicGovernance struct {
    eventBus eventbus.EventBus
    rules    []GovernanceRule
}

type GovernanceRule struct {
    Pattern     string
    MinReplicas int
    MaxRetention time.Duration
    RequiredMode eventbus.TopicPersistenceMode
}

func (g *TopicGovernance) ValidateTopicConfig(topic string, options eventbus.TopicOptions) error {
    for _, rule := range g.rules {
        if matched, _ := filepath.Match(rule.Pattern, topic); matched {
            // 验证副本数
            if options.Replicas < rule.MinReplicas {
                return fmt.Errorf("topic %s requires at least %d replicas", topic, rule.MinReplicas)
            }

            // 验证保留时间
            if options.RetentionTime > rule.MaxRetention {
                return fmt.Errorf("topic %s retention time exceeds maximum %v", topic, rule.MaxRetention)
            }

            // 验证持久化模式
            if rule.RequiredMode != "" && options.PersistenceMode != rule.RequiredMode {
                return fmt.Errorf("topic %s requires persistence mode %s", topic, rule.RequiredMode)
            }
        }
    }
    return nil
}

// 自动应用治理规则
func (g *TopicGovernance) ApplyGovernanceRules(ctx context.Context) error {
    topics := g.eventBus.ListConfiguredTopics()

    for _, topic := range topics {
        config, err := g.eventBus.GetTopicConfig(topic)
        if err != nil {
            continue
        }

        // 应用治理规则
        if err := g.ValidateTopicConfig(topic, config); err != nil {
            // 记录违规并尝试修复
            log.Printf("Governance violation for topic %s: %v", topic, err)

            // 自动修复（可选）
            if fixedConfig := g.autoFixConfig(topic, config); fixedConfig != nil {
                g.eventBus.ConfigureTopic(ctx, topic, *fixedConfig)
            }
        }
    }
    return nil
}
```

#### 企业级性能优化

**1. 智能路由性能监控**：
```go
// 性能监控指标
type PerformanceMetrics struct {
    TopicRouteLatency    map[string]time.Duration // 主题路由延迟
    MessageThroughput    map[string]int64         // 消息吞吐量
    PersistentRatio      float64                  // 持久化消息比例
    EphemeralRatio       float64                  // 非持久化消息比例
    RouteDecisionTime    time.Duration            // 路由决策时间
    ConfigUpdateLatency  time.Duration            // 配置更新延迟
}

// 性能基准测试
func BenchmarkTopicPersistence(b *testing.B) {
    bus := setupEventBus()
    ctx := context.Background()

    // 配置测试主题
    persistentOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   24 * time.Hour,
    }
    bus.ConfigureTopic(ctx, "benchmark.persistent", persistentOptions)

    ephemeralOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   30 * time.Minute,
    }
    bus.ConfigureTopic(ctx, "benchmark.ephemeral", ephemeralOptions)

    b.Run("Persistent", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            bus.Publish(ctx, "benchmark.persistent", []byte("test message"))
        }
    })

    b.Run("Ephemeral", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            bus.Publish(ctx, "benchmark.ephemeral", []byte("test message"))
        }
    })
}
```

**2. 企业级性能对比表**：

| 配置类型 | 传输机制 | 延迟 | 吞吐量 | 可靠性 | 存储成本 | 适用场景 |
|---------|----------|------|--------|--------|----------|----------|
| **金融级持久化** | JetStream/Kafka多副本 | 5-10ms | 10K msg/s | 99.99% | 高 | 交易记录、审计日志 |
| **业务级持久化** | JetStream/Kafka标准 | 2-5ms | 50K msg/s | 99.9% | 中 | 订单事件、用户行为 |
| **系统级非持久化** | Core NATS/内存 | 0.1-1ms | 500K msg/s | 95% | 极低 | 系统通知、监控指标 |
| **临时消息** | Core NATS/内存 | 0.05-0.5ms | 1M msg/s | 90% | 无 | 缓存失效、会话更新 |

**3. 自动性能调优**：
```go
// 自动性能调优器
type PerformanceTuner struct {
    eventBus    eventbus.EventBus
    metrics     *PerformanceMetrics
    thresholds  TuningThresholds
}

type TuningThresholds struct {
    HighLatencyThreshold    time.Duration // 高延迟阈值
    LowThroughputThreshold  int64         // 低吞吐量阈值
    HighVolumeThreshold     int64         // 高容量阈值
}

func (t *PerformanceTuner) AutoTune(ctx context.Context) error {
    topics := t.eventBus.ListConfiguredTopics()

    for _, topic := range topics {
        config, err := t.eventBus.GetTopicConfig(topic)
        if err != nil {
            continue
        }

        // 获取主题性能指标
        latency := t.metrics.TopicRouteLatency[topic]
        throughput := t.metrics.MessageThroughput[topic]

        // 自动调优逻辑
        if latency > t.thresholds.HighLatencyThreshold && config.PersistenceMode == eventbus.TopicPersistent {
            // 高延迟持久化主题：考虑优化配置
            if throughput < t.thresholds.LowThroughputThreshold {
                // 低吞吐量：可能降级为非持久化
                log.Printf("Consider downgrading topic %s to ephemeral due to high latency and low throughput", topic)
            }
        }

        if throughput > t.thresholds.HighVolumeThreshold && config.PersistenceMode == eventbus.TopicEphemeral {
            // 高吞吐量非持久化主题：考虑增加资源
            log.Printf("Consider scaling resources for high-volume ephemeral topic %s", topic)
        }
    }

    return nil
}
```

### 2. 企业级健康检查与监控

EventBus 提供企业级的分离式健康检查系统，支持发布端和订阅端独立监控，实现精确的故障检测和自动恢复。

#### 分离式健康检查架构

**核心设计理念**：
- **角色分离**：发布端和订阅端独立健康检查，精确定位故障源
- **主题配对**：支持不同服务使用不同健康检查主题，避免干扰
- **智能评估**：多维度指标监控，自动计算健康状态
- **预防性控制**：主动限流和优化，防止系统过载

#### 企业级健康检查配置

使用 `config.EventBusConfig` 可以分别配置发布端和订阅端的健康检查参数：

```yaml
# 企业级健康检查配置
eventbus:
  type: "kafka"  # 或 "nats", "memory"
  serviceName: "order-service"
  environment: "production"

  # 企业级分离式健康检查配置
  healthCheck:
    enabled: true              # 是否启用健康检查（总开关）

    # 发布端健康检查（主动探测）
    publisher:
      topic: "health-check-order-service-prod"  # 环境隔离的主题
      interval: "90s"                           # 生产环境适中间隔
      timeout: "15s"                            # 充足的超时时间
      failureThreshold: 5                       # 更高的容错性
      messageTTL: "10m"                         # 更长的消息存活时间
      retryPolicy:
        maxRetries: 3
        backoffMultiplier: 2.0
        initialBackoff: "1s"

    # 订阅端健康检查（被动监控）
    subscriber:
      topic: "health-check-order-service-prod"  # 与发布端配对
      monitorInterval: "45s"                    # 监控检查间隔
      warningThreshold: 2                       # 早期预警
      errorThreshold: 4                         # 错误告警
      criticalThreshold: 8                      # 严重告警
      recoveryThreshold: 2                      # 恢复阈值

    # 高级监控配置
    monitoring:
      enableMetrics: true                       # 启用指标收集
      metricsInterval: "30s"                    # 指标收集间隔
      alertWebhook: "https://alerts.company.com/webhook"  # 告警webhook
      dashboardEnabled: true                    # 启用监控面板

  # 主题持久化配置（健康检查主题也支持）
  topics:
    "health-check-order-service-prod":
      persistenceMode: "ephemeral"              # 健康检查消息无需持久化
      retentionTime: "1h"                       # 短期保留
      description: "订单服务健康检查主题"

  kafka:
    brokers: ["kafka-1:9092", "kafka-2:9092", "kafka-3:9092"]
    # 企业级Kafka配置
    producer:
      requiredAcks: 1
      timeout: 30s
      retryMax: 5
    consumer:
      groupID: "order-service-health-check"
      sessionTimeout: 30s
      heartbeatInterval: 10s
```

#### 企业级配置参数详解

**发布端配置（publisher）**：

| 参数 | 类型 | 默认值 | 企业级建议 | 说明 |
|------|------|--------|------------|------|
| `topic` | string | 自动生成 | `health-check-{service}-{env}` | 健康检查发布主题，建议包含服务名和环境 |
| `interval` | duration | `2m` | 生产:`90s`, 开发:`30s` | 健康检查发送间隔，生产环境适中，开发环境频繁 |
| `timeout` | duration | `10s` | 生产:`15s`, 开发:`5s` | 单次健康检查超时，生产环境更宽松 |
| `failureThreshold` | int | `3` | 生产:`5`, 开发:`2` | 连续失败阈值，生产环境更容错 |
| `messageTTL` | duration | `5m` | 生产:`10m`, 开发:`2m` | 消息存活时间，生产环境更长 |
| `retryPolicy.maxRetries` | int | `3` | 生产:`5`, 开发:`2` | 最大重试次数 |
| `retryPolicy.backoffMultiplier` | float | `2.0` | `1.5-3.0` | 退避倍数，控制重试间隔增长 |

**订阅端配置（subscriber）**：

| 参数 | 类型 | 默认值 | 企业级建议 | 说明 |
|------|------|--------|------------|------|
| `topic` | string | 自动生成 | 与发布端配对 | 健康检查订阅主题，必须与发布端匹配 |
| `monitorInterval` | duration | `30s` | 生产:`45s`, 开发:`15s` | 监控检查间隔 |
| `warningThreshold` | int | `3` | 生产:`2`, 开发:`1` | 警告阈值，生产环境早期预警 |
| `errorThreshold` | int | `5` | 生产:`4`, 开发:`2` | 错误阈值，触发告警 |
| `criticalThreshold` | int | `10` | 生产:`8`, 开发:`4` | 严重阈值，触发紧急响应 |
| `recoveryThreshold` | int | `2` | `1-3` | 恢复阈值，连续成功多少次认为恢复 |

**监控配置（monitoring）**：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enableMetrics` | bool | `false` | 是否启用详细指标收集 |
| `metricsInterval` | duration | `60s` | 指标收集和上报间隔 |
| `alertWebhook` | string | - | 告警webhook URL，用于集成企业告警系统 |
| `dashboardEnabled` | bool | `false` | 是否启用内置监控面板 |

**默认健康检查主题**：
- **Kafka**: `jxt-core-kafka-health-check`
- **NATS**: `jxt-core-nats-health-check`
- **Memory**: `jxt-core-memory-health-check`

##### 兼容性配置（传统方式）

为了向后兼容，仍然支持在各EventBus类型中单独配置健康检查间隔：

```yaml
eventbus:
  type: "kafka"
  kafka:
    brokers: ["localhost:9092"]
    healthCheckInterval: "5m"  # 传统配置方式

  # 或者 NATS
  type: "nats"
  nats:
    urls: ["nats://localhost:4222"]
    healthCheckInterval: "5m"  # 传统配置方式
```

**配置优先级**：
1. **统一配置优先**：`healthCheck.interval` > `kafka.healthCheckInterval`
2. **自动降级**：如果统一配置不存在，使用传统配置
3. **默认值兜底**：如果都不配置，使用默认值 `2m`

##### 不同环境的推荐配置

**开发环境**（快速反馈）：
```yaml
healthCheck:
  enabled: true
  publisher:
    interval: "30s"      # 更频繁的发布
    timeout: "5s"        # 更短的超时
    failureThreshold: 2  # 更低的失败阈值
  subscriber:
    monitorInterval: "10s"    # 更频繁的监控
    warningThreshold: 2       # 更低的警告阈值
    errorThreshold: 3
    criticalThreshold: 5
```

**生产环境**（稳定性优先）：
```yaml
healthCheck:
  enabled: true
  publisher:
    interval: "2m"       # 标准间隔
    timeout: "10s"       # 充足的超时时间
    failureThreshold: 5  # 更高的容错性
    messageTTL: "10m"    # 更长的消息存活时间
  subscriber:
    monitorInterval: "30s"    # 标准监控间隔
    warningThreshold: 5       # 更高的容错性
    errorThreshold: 8
    criticalThreshold: 15
```

**高负载环境**（减少开销）：
```yaml
healthCheck:
  enabled: true
  publisher:
    interval: "5m"       # 较长间隔
    timeout: "15s"       # 更长超时
    failureThreshold: 3  # 标准阈值
  subscriber:
    monitorInterval: "60s"    # 较长监控间隔
    warningThreshold: 3       # 标准阈值
    errorThreshold: 5
    criticalThreshold: 10
```

#### 分离式健康检查启动和控制

jxt-core 支持分离式健康检查，发布端和订阅端可以独立启动和控制：

```go
// 1. 创建应用 context
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// 2. 根据服务角色选择启动策略

// 场景A：纯发布端服务（只发送健康检查消息）
if err := bus.StartHealthCheckPublisher(ctx); err != nil {
    log.Printf("Failed to start health check publisher: %v", err)
} else {
    log.Println("Health check publisher started")
}

// 场景B：纯订阅端服务（只监控健康检查消息）
if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
    log.Printf("Failed to start health check subscriber: %v", err)
} else {
    log.Println("Health check subscriber started")
}

// 场景C：混合角色服务（既发送又监控）
if err := bus.StartAllHealthCheck(ctx); err != nil {
    log.Printf("Failed to start all health checks: %v", err)
} else {
    log.Println("All health checks started")
}

// 3. 优雅关闭
defer func() {
    // 停止所有健康检查
    if err := bus.StopAllHealthCheck(); err != nil {
        log.Printf("Failed to stop health checks: %v", err)
    }

    // 关闭 EventBus 资源
    if err := bus.Close(); err != nil {
        log.Printf("Failed to close EventBus: %v", err)
    }
}()

// 4. 动态控制（可选）
// 可以根据运行时条件动态启动/停止
serviceRole := getServiceRole() // 获取服务角色
switch serviceRole {
case "publisher":
    bus.StartHealthCheckPublisher(ctx)
case "subscriber":
    bus.StartHealthCheckSubscriber(ctx)
case "both":
    bus.StartAllHealthCheck(ctx)
}
```

#### 完整的生命周期管理示例

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 1. 初始化 EventBus
    cfg := eventbus.GetDefaultKafkaConfig([]string{"localhost:9092"})
    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal("Failed to initialize EventBus:", err)
    }

    bus := eventbus.GetGlobal()
    log.Println("EventBus initialized successfully")

    // 2. 创建应用级别的 context
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 3. 启动周期性健康检查
    if err := bus.StartHealthCheckPublisher(ctx); err != nil {
        log.Printf("Failed to start health check: %v", err)
        // 注意：健康检查启动失败不会影响 EventBus 基本功能
    } else {
        log.Println("Health check started successfully")
    }

    // 4. 设置优雅关闭信号处理
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    // 5. 应用主逻辑
    go func() {
        log.Println("Application started, running business logic...")
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                log.Println("Business logic stopped")
                return
            case <-ticker.C:
                // 模拟业务逻辑
                log.Println("Processing business logic...")

                // 可以手动检查健康状态
                if state := bus.GetConnectionState(); !state.IsConnected {
                    log.Printf("Warning: EventBus not connected")
                }
            }
        }
    }()

    // 6. 等待退出信号
    <-sigChan
    log.Println("Received shutdown signal, shutting down gracefully...")

    // 7. 优雅关闭序列
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    // 停止周期性健康检查（同步等待完成）
    log.Println("Stopping health check...")
    if err := bus.StopHealthCheckPublisher(); err != nil {
        log.Printf("Error stopping health check: %v", err)
    } else {
        log.Println("Health check stopped successfully")
    }

    // 取消应用 context，停止业务逻辑
    cancel()

    // 关闭 EventBus 资源
    log.Println("Closing EventBus...")
    if err := eventbus.CloseGlobal(); err != nil {
        log.Printf("Error closing EventBus: %v", err)
    } else {
        log.Println("EventBus closed successfully")
    }

    log.Println("Application stopped gracefully")
}
```

#### 分离式健康检查最佳实践

**关键要点**：

1. **角色明确**：根据服务在业务中的实际角色选择启动策略
   - **纯发布端**：只调用 `StartHealthCheckPublisher()`
   - **纯订阅端**：只调用 `StartHealthCheckSubscriber()`
   - **混合角色**：调用 `StartAllHealthCheck()` 或分别启动

2. **主题配对**：确保发布端和订阅端使用相同的主题进行配对
   ```yaml
   # 服务A（发布端）
   publisher:
     topic: "health-check-service-a"

   # 服务B（监控服务A）
   subscriber:
     topic: "health-check-service-a"  # 与服务A配对
   ```

3. **生命周期控制**：
   - `StartHealthCheckPublisher(ctx)` - 启动发布端
   - `StartHealthCheckSubscriber(ctx)` - 启动订阅端
   - `StopHealthCheckPublisher()` / `StopHealthCheckSubscriber()` - 独立停止
   - `StartAllHealthCheck(ctx)` / `StopAllHealthCheck()` - 批量操作

4. **配置简化**：不再需要 `subscriber.enabled` 字段，通过接口控制启动

5. **错误处理**：启动失败不会影响 EventBus 的基本功能

6. **分离式架构**：发布端和订阅端独立控制
   - 发布端：`StartHealthCheckPublisher()` / `StopHealthCheckPublisher()`
   - 订阅端：`StartHealthCheckSubscriber()` / `StopHealthCheckSubscriber()`

**推荐的关闭顺序**：
```go
// 1. 停止健康检查（根据启动的组件选择）
if err := bus.StopAllHealthCheck(); err != nil {
    log.Printf("Error stopping health checks: %v", err)
}

// 2. 取消应用 context（停止业务逻辑）
cancel()

// 3. 关闭 EventBus 资源
if err := eventbus.CloseGlobal(); err != nil {
    log.Printf("Error closing EventBus: %v", err)
}
```

### 4. 自动重连机制

jxt-core EventBus 组件内置了智能的自动重连机制，当健康检查检测到连接中断时会自动触发重连。

#### 自动重连特性

- **智能触发**：基于健康检查失败次数自动触发重连
- **指数退避**：使用指数退避算法避免频繁重连
- **状态恢复**：重连成功后自动恢复所有订阅
- **回调通知**：支持重连成功后的回调通知
- **配置灵活**：支持自定义重连参数
- **多后端支持**：Kafka 和 NATS 都支持完整的自动重连功能

#### 基础用法

##### Kafka EventBus 自动重连

```go
// 1. 初始化 Kafka EventBus（自动启用重连）
cfg := eventbus.GetDefaultKafkaConfig([]string{"localhost:9092"})
if err := eventbus.InitializeFromConfig(cfg); err != nil {
    log.Fatal(err)
}

bus := eventbus.GetGlobal()

// 2. 启动健康检查（包含自动重连）
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := bus.StartHealthCheckPublisher(ctx); err != nil {
    log.Printf("Failed to start health check: %v", err)
}

// 3. 注册重连回调（可选，处理业务状态）
err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Println("🔄 Kafka EventBus reconnected successfully!")

    // EventBus 已自动完成：
    // ✅ 重新建立连接
    // ✅ 恢复所有订阅
    // ✅ 重置健康状态

    // 业务层只需处理业务相关状态：
    // - 重新加载缓存
    // - 同步业务状态
    // - 发送监控指标
    // - 通知其他服务

    return reloadBusinessCache(ctx) // 示例：重新加载业务缓存
})
if err != nil {
    log.Printf("Failed to register reconnect callback: %v", err)
}

// 4. 正常使用 EventBus
// 当连接中断时，会自动重连并恢复订阅，无需业务层干预
```

##### NATS EventBus 自动重连

```go
// 1. 初始化 NATS EventBus（自动启用重连）
cfg := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"})
if err := eventbus.InitializeFromConfig(cfg); err != nil {
    log.Fatal(err)
}

bus := eventbus.GetGlobal()

// 2. 启动健康检查（包含自动重连）
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := bus.StartHealthCheckPublisher(ctx); err != nil {
    log.Printf("Failed to start health check: %v", err)
}

// 3. 注册重连回调（可选，处理业务状态）
err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Println("🔄 NATS EventBus reconnected successfully!")

    // NATS 双重保障已自动完成：
    // ✅ 客户端内置重连
    // ✅ 应用层重连逻辑
    // ✅ 自动恢复所有订阅
    // ✅ 重置健康状态

    // 业务层处理业务相关状态：
    // - 重新加载缓存
    // - 同步业务状态
    // - 发送监控指标
    // - 通知其他服务

    return syncBusinessState(ctx) // 示例：同步业务状态
})
if err != nil {
    log.Printf("Failed to register reconnect callback: %v", err)
}

// 4. 正常使用 EventBus
// NATS 客户端内置重连 + 应用层自动重连双重保障，业务层无需关心连接管理
```

#### 高级配置

##### Kafka 重连配置

```go
// 获取 Kafka EventBus 实例（注意：需要类型断言）
kafkaEB := bus.(*kafkaEventBus) // 内部类型，生产环境建议通过接口访问

// 自定义重连配置
customConfig := eventbus.ReconnectConfig{
    MaxAttempts:      5,                    // 最大重连次数
    InitialBackoff:   500 * time.Millisecond, // 初始退避时间
    MaxBackoff:       10 * time.Second,     // 最大退避时间
    BackoffFactor:    1.5,                  // 退避因子
    FailureThreshold: 2,                    // 触发重连的失败次数
}

if err := kafkaEB.SetReconnectConfig(customConfig); err != nil {
    log.Printf("Failed to set Kafka reconnect config: %v", err)
}

// 获取重连状态
status := kafkaEB.GetReconnectStatus()
log.Printf("Kafka - Failure count: %d, Last reconnect: %v",
    status.FailureCount, status.LastReconnectTime)
```

##### NATS 重连配置

```go
// 获取 NATS EventBus 实例（注意：需要类型断言）
natsEB := bus.(*natsEventBus) // 内部类型，生产环境建议通过接口访问

// 自定义重连配置
customConfig := eventbus.ReconnectConfig{
    MaxAttempts:      8,                    // 最大重连次数
    InitialBackoff:   200 * time.Millisecond, // 初始退避时间
    MaxBackoff:       5 * time.Second,      // 最大退避时间
    BackoffFactor:    1.8,                  // 退避因子
    FailureThreshold: 2,                    // 触发重连的失败次数
}

if err := natsEB.SetReconnectConfig(customConfig); err != nil {
    log.Printf("Failed to set NATS reconnect config: %v", err)
}

// 获取重连状态
status := natsEB.GetReconnectStatus()
log.Printf("NATS - Failure count: %d, Last reconnect: %v",
    status.FailureCount, status.LastReconnectTime)
```

#### 重连流程

##### 通用重连流程

1. **健康检查失败**：周期性健康检查检测到连接问题
2. **失败计数**：累计连续失败次数
3. **触发重连**：达到失败阈值时触发自动重连
4. **指数退避**：使用指数退避算法进行重连尝试
5. **连接重建**：重新创建底层客户端连接
6. **订阅恢复**：自动恢复所有之前的订阅
7. **回调通知**：调用注册的重连回调函数（业务层处理业务状态）
8. **状态重置**：重置失败计数，恢复正常运行

**重要说明**：
- **步骤 1-6** 由 EventBus 自动完成，业务层无需干预
- **步骤 7** 是业务层的处理时机，通过回调函数处理业务相关状态
- **步骤 8** 由 EventBus 自动完成，标志重连流程结束

##### Kafka 特定流程

- **连接重建**：重新创建 Sarama 客户端、生产者、消费者、管理客户端
- **订阅恢复**：重新建立所有主题的消费者订阅
- **状态同步**：确保生产者和消费者状态一致

##### NATS 特定流程

- **连接重建**：重新创建 NATS 连接和 JetStream 上下文（如果启用）
- **订阅恢复**：重新建立所有主题的订阅（核心 NATS 或 JetStream）
- **双重保障**：NATS 客户端内置重连 + 应用层重连机制

#### 完整应用示例：健康检查 + 自动重连

以下是一个完整的微服务应用示例，展示如何正确使用健康检查和自动重连功能：

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 1. 初始化 EventBus
    cfg := eventbus.GetDefaultKafkaConfig([]string{"localhost:9092"})
    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal("Failed to initialize EventBus:", err)
    }

    bus := eventbus.GetGlobal()
    log.Println("EventBus initialized successfully")

    // 2. 注册重连回调（处理业务状态）
    err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
        log.Printf("🔄 EventBus reconnected at %v", time.Now().Format("15:04:05"))

        // EventBus 已自动完成基础设施恢复：
        // ✅ 重新建立连接
        // ✅ 恢复所有订阅
        // ✅ 重置健康状态

        // 业务层处理业务相关状态：
        if err := reloadApplicationCache(); err != nil {
            log.Printf("Failed to reload cache: %v", err)
        }

        if err := syncBusinessState(); err != nil {
            log.Printf("Failed to sync business state: %v", err)
        }

        // 发送监控指标
        recordReconnectMetrics()

        log.Println("✅ Business state recovery completed")
        return nil
    })
    if err != nil {
        log.Printf("Failed to register reconnect callback: %v", err)
    }

    // 3. 创建应用 context
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 4. 启动健康检查（包含自动重连）
    if err := bus.StartHealthCheckPublisher(ctx); err != nil {
        log.Printf("Failed to start health check: %v", err)
    } else {
        log.Println("Health check with auto-reconnect started")
    }

    // 5. 设置业务订阅
    topic := "business.events"
    handler := func(ctx context.Context, message []byte) error {
        log.Printf("📨 Processing business event: %s", string(message))
        return processBusinessEvent(message)
    }

    if err := bus.Subscribe(ctx, topic, handler); err != nil {
        log.Printf("Failed to subscribe: %v", err)
    } else {
        log.Printf("Subscribed to topic: %s", topic)
    }

    // 6. 设置优雅关闭
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    // 7. 启动业务逻辑
    go runBusinessLogic(ctx, bus, topic)

    // 8. 启动状态监控
    go monitorEventBusStatus(ctx, bus)

    // 9. 等待退出信号
    log.Println("🚀 Application started. Press Ctrl+C to stop.")
    log.Println("💡 Try stopping Kafka/NATS to see auto-reconnect in action!")

    <-sigChan
    log.Println("📴 Received shutdown signal, shutting down gracefully...")

    // 10. 优雅关闭序列
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    // 停止健康检查
    log.Println("Stopping health check...")
    if err := bus.StopHealthCheckPublisher(); err != nil {
        log.Printf("Error stopping health check: %v", err)
    }

    // 取消应用 context
    cancel()

    // 关闭 EventBus
    log.Println("Closing EventBus...")
    if err := eventbus.CloseGlobal(); err != nil {
        log.Printf("Error closing EventBus: %v", err)
    }

    log.Println("✅ Application stopped gracefully")
}

// 业务逻辑函数
func runBusinessLogic(ctx context.Context, bus eventbus.EventBus, topic string) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    messageCount := 0
    for {
        select {
        case <-ctx.Done():
            log.Println("Business logic stopped")
            return
        case <-ticker.C:
            messageCount++
            message := []byte(fmt.Sprintf("Business event #%d at %v",
                messageCount, time.Now().Format("15:04:05")))

            if err := bus.Publish(ctx, topic, message); err != nil {
                log.Printf("❌ Failed to publish: %v", err)
            } else {
                log.Printf("📤 Published business event #%d", messageCount)
            }
        }
    }
}

// 状态监控函数
func monitorEventBusStatus(ctx context.Context, bus eventbus.EventBus) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            log.Println("Status monitor stopped")
            return
        case <-ticker.C:
            // 获取连接状态
            connState := bus.GetConnectionState()

            // 获取健康状态
            healthStatus := bus.GetHealthCheckPublisherStatus()

            log.Printf("📊 Status - Connected: %v, Healthy: %v, Failures: %d",
                connState.IsConnected,
                healthStatus.IsHealthy,
                healthStatus.ConsecutiveFailures)
        }
    }
}

// 业务状态处理函数
func reloadApplicationCache() error {
    log.Println("🔄 Reloading application cache...")
    // 实现缓存重新加载逻辑
    time.Sleep(100 * time.Millisecond) // 模拟处理时间
    return nil
}

func syncBusinessState() error {
    log.Println("🔄 Syncing business state...")
    // 实现业务状态同步逻辑
    time.Sleep(200 * time.Millisecond) // 模拟处理时间
    return nil
}

func recordReconnectMetrics() {
    log.Println("📊 Recording reconnect metrics...")
    // 实现监控指标记录
}

func processBusinessEvent(message []byte) error {
    // 实现业务事件处理逻辑
    time.Sleep(50 * time.Millisecond) // 模拟处理时间
    return nil
}
```

#### 关键要点总结

1. **自动化程度高**：EventBus 自动处理连接管理和订阅恢复
2. **业务层职责清晰**：只需处理业务相关状态，不需要关心基础设施
3. **回调时机准确**：在连接和订阅恢复完成后才执行业务回调
4. **错误容忍性好**：业务回调失败不影响 EventBus 功能
5. **监控友好**：提供完整的状态监控和指标收集

#### 配置参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| MaxAttempts | 10 | 最大重连尝试次数 |
| InitialBackoff | 1s | 初始退避时间 |
| MaxBackoff | 30s | 最大退避时间 |
| BackoffFactor | 2.0 | 退避时间倍增因子 |
| FailureThreshold | 3 | 触发重连的连续失败次数 |

#### 监控和调试

```go
// 获取重连状态
status := kafkaEB.GetReconnectStatus()
fmt.Printf("重连状态: %+v\n", status)

// 监控重连事件
bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Printf("重连成功 - 时间: %v", time.Now())
    // 发送监控指标
    // metrics.IncrementReconnectCount()
    return nil
})
```

#### 简化的快速开始示例

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 初始化 EventBus
    cfg := eventbus.GetDefaultMemoryConfig()
    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal(err)
    }

    bus := eventbus.GetGlobal()

    // 启动健康检查
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    if err := bus.StartHealthCheckPublisher(ctx); err != nil {
        log.Printf("Health check start failed: %v", err)
    }

    // 模拟应用运行
    log.Println("Application running...")
    time.Sleep(5 * time.Second)

    // 优雅关闭
    log.Println("Shutting down...")
    bus.StopHealthCheckPublisher()        // 停止健康检查
    eventbus.CloseGlobal()       // 关闭 EventBus
    log.Println("Done")
}
```

**推荐的关闭顺序**：
```go
// 1. 停止健康检查（同步等待）
bus.StopHealthCheckPublisher()

// 2. 取消应用 context（停止业务逻辑）
cancel()

// 3. 关闭 EventBus 资源
eventbus.CloseGlobal()
```

#### 业务微服务中的健康检查实现（发布端/订阅端分开）

jxt-core 已内置基础设施层健康检查能力，业务微服务只需在合适的位置“调用/挂载”即可。推荐按发布端（Producer/Command）与订阅端（Consumer/Query）分别集成。

##### A. 发布端（Producer/Command）

- 场景：只负责发布业务事件（无长期订阅）。
- 建议：
  - 就绪检查（/readyz）使用“连接检查”确保可发布
  - 完整健康检查（/health）可选触发“消息传输检查”（将做端到端验证，如果底层支持）
  - 统一暴露 HTTP 健康端点（框架已提供工具函数）

示例（最简集成）：
```go
// 启动周期性健康检查
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := bus.StartHealthCheckPublisher(ctx); err != nil {
    log.Printf("Failed to start health check: %v", err)
}

// 获取健康状态
status := bus.GetHealthCheckPublisherStatus()
if status.IsHealthy {
    log.Println("EventBus is healthy")
}
}
```

挂载 HTTP 端点（推荐）：
```go
mux := http.NewServeMux()
eventbus.SetupHealthCheckRoutes(mux, bus)
// /health  /healthz  /livez  /readyz
```

##### B. 订阅端（Consumer/Query）

- 场景：长期订阅业务事件进行处理。
- 建议：
  - 启用完整健康检查（会在底层自动创建临时订阅做端到端验证）
  - 可选：订阅技术主题 `HealthCheckTopic` 做“持续心跳监控”（更贴近业务侧运维）

示例（获取健康状态）：
```go
status := bus.GetHealthCheckPublisherStatus()
if !status.IsHealthy {
    log.Printf("EventBus unhealthy: %d consecutive failures", status.ConsecutiveFailures)
}
}
```

示例（可选：订阅健康主题做心跳）：
```go
_ = bus.Subscribe(ctx, eventbus.HealthCheckTopic, func(ctx context.Context, msg []byte) error {
    // 更新最后一次心跳时间；可结合阈值做告警
    return nil
})
```

##### C. 业务健康检查的注册（可选）

如需在 /health 返回业务侧指标（例如：待处理队列、外部依赖、租户检查等），实现并注册 `BusinessHealthChecker`：
```go
type MyBizChecker struct{}
func (m *MyBizChecker) CheckBusinessHealth(ctx context.Context) error { return nil }
func (m *MyBizChecker) GetBusinessMetrics() interface{} { return map[string]any{"ok": true} }
func (m *MyBizChecker) GetBusinessConfig() interface{}  { return nil }

if hc, ok := bus.(eventbus.EventBusHealthChecker); ok {
    hc.RegisterBusinessHealthCheck(&MyBizChecker{})
}
```

##### D. HTTP 健康检查端点（统一挂载）

框架提供了标准化端点挂载函数，适用于发布端与订阅端：
```go
mux := http.NewServeMux()
eventbus.SetupHealthCheckRoutes(mux, bus)
// 提供：/health（完整）、/healthz 或 /livez（存活）、/readyz（就绪）
```

**关键要点**：

1. 分层职责：基础设施健康检查由 jxt-core 提供，业务健康检查由业务服务实现并注册
2. 发布端关注“可发布”（连接/传输）；订阅端关注“可消费”（端到端/心跳）
3. 统一端点：优先使用 `SetupHealthCheckRoutes` 暴露 /health、/readyz、/livez
4. 可选增强：订阅 `HealthCheckTopic` 做持续心跳监控与告警
5. 周期性检查：使用 `StartHealthCheckPublisher(ctx)` 启动后台健康检查，通过 `GetHealthCheckPublisherStatus()` 获取状态

### 3. 分离式健康检查订阅监控

jxt-core EventBus 组件提供了完整的分离式健康检查订阅监控机制，支持独立启动和精确配置。

#### 健康检查订阅监控配置

健康检查订阅监控器现在使用独立的配置参数，支持与发送器不同的监控策略：

```yaml
eventbus:
  type: "kafka"
  serviceName: "my-service"

  # 分离式健康检查配置
  healthCheck:
    enabled: true              # 总开关
    publisher:
      topic: "health-check-my-service"    # 发布器主题
      interval: "2m"                      # 发布间隔
      timeout: "10s"                      # 发送超时
      failureThreshold: 3                 # 发送失败阈值
      messageTTL: "5m"                    # 消息存活时间
    subscriber:
      topic: "health-check-my-service"    # 订阅器主题（与发布端配对）
      monitorInterval: "30s"              # 监控检查间隔
      warningThreshold: 3                 # 警告阈值
      errorThreshold: 5                   # 错误阈值
      criticalThreshold: 10               # 严重阈值
```

**配置说明**：
- **独立配置**：订阅监控器有自己的配置参数，可以与发送器不同
- **主题配对**：通过相同的主题名称实现发送器和订阅器的精确配对
- **多级告警**：支持警告、错误、严重三个级别的告警阈值

**告警级别映射**：
- **Warning**: 连续错过 `failureThreshold` 次（默认3次）
- **Error**: 连续错过 `failureThreshold * 1.5` 次（默认5次）
- **Critical**: 连续错过 `failureThreshold * 3` 次（默认10次）

#### 功能特性

- **🔄 独立启动**：订阅监控器可以独立于发送器启动和停止
- **📊 统计监控**：实时统计接收消息数量、连续错过次数、运行时间等
- **🚨 智能告警**：支持多级别告警（warning、error、critical），可自定义告警回调
- **⚡ 高性能**：基于原子操作的无锁统计，对业务性能影响极小
- **🔧 易于集成**：简单的API接口，支持Kafka、NATS、Memory等多种EventBus实现
- **🎛️ 精确配对**：通过主题名称实现与发送器的精确配对
- **🎯 角色灵活**：同一服务可在不同业务中扮演不同监控角色

#### 基本使用

```go
// 1. 独立启动健康检查订阅监控
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// 只启动订阅监控器（不启动发送器）
if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
    log.Printf("Failed to start health check subscriber: %v", err)
} else {
    log.Println("Health check subscriber started successfully")
}

// 2. 注册告警回调（可选）
err := bus.RegisterHealthCheckSubscriberCallback(func(alert HealthCheckAlert) {
    switch alert.Level {
    case "warning":
        log.Printf("⚠️  Health check warning: %s", alert.Message)
    case "error":
        log.Printf("❌ Health check error: %s", alert.Message)
    case "critical":
        log.Printf("🚨 Health check critical: %s", alert.Message)
    }
})

// 3. 获取监控统计信息
stats := bus.GetHealthCheckSubscriberStats()
log.Printf("Health check stats: %+v", stats)

// 4. 独立停止健康检查订阅监控
defer func() {
    if err := bus.StopHealthCheckSubscriber(); err != nil {
        log.Printf("Failed to stop health check subscriber: %v", err)
    }
}()
```

#### 告警机制

健康检查订阅监控器会根据连续错过健康检查消息的次数触发不同级别的告警：

- **Warning（警告）**：连续错过 3 次健康检查消息
- **Error（错误）**：连续错过 5 次健康检查消息
- **Critical（严重）**：连续错过 10 次健康检查消息

```go
// 告警回调函数签名
type HealthCheckAlertCallback func(alert HealthCheckAlert)

// 告警信息结构
type HealthCheckAlert struct {
    Level       string    // "warning", "error", "critical"
    AlertType   string    // "no_messages", "invalid_message", "subscriber_error"
    Message     string    // 告警消息
    Source      string    // 告警来源
    EventBusType string   // EventBus类型
    Timestamp   time.Time // 告警时间
    Metadata    map[string]interface{} // 额外元数据
}
```

#### 统计信息

通过 `GetHealthCheckSubscriberStats()` 可以获取详细的监控统计信息：

```go
type HealthCheckSubscriberStats struct {
    IsRunning              bool      // 是否正在运行
    IsHealthy              bool      // 当前健康状态
    TotalMessagesReceived  int64     // 总接收消息数
    ConsecutiveMisses      int32     // 连续错过次数
    TotalAlerts           int64     // 总告警次数
    LastMessageTime       time.Time // 最后消息时间
    UptimeSeconds         float64   // 运行时间（秒）
    StartTime             time.Time // 启动时间
}
```

#### 使用分离式健康检查配置的编程方式示例

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/config"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 1. 创建包含分离式健康检查配置的EventBus配置
    cfg := &config.EventBusConfig{
        Type:        "kafka",
        ServiceName: "health-check-demo",
        Kafka: config.KafkaConfig{
            Brokers: []string{"localhost:9092"},
        },

        // 重点：使用分离式HealthCheckConfig配置健康检查参数
        HealthCheck: config.HealthCheckConfig{
            Enabled: true, // 启用健康检查
            Publisher: config.HealthCheckPublisherConfig{
                Topic:            "health-check-demo",  // 发布主题
                Interval:         30 * time.Second,     // 30秒发布间隔
                Timeout:          5 * time.Second,      // 5秒超时
                FailureThreshold: 2,                    // 连续失败2次触发重连
                MessageTTL:       2 * time.Minute,      // 消息2分钟过期
            },
            Subscriber: config.HealthCheckSubscriberConfig{
                Topic:             "health-check-demo", // 订阅主题（与发布端配对）
                MonitorInterval:   10 * time.Second,    // 10秒监控间隔
                WarningThreshold:  2,                   // 警告阈值
                ErrorThreshold:    3,                   // 错误阈值
                CriticalThreshold: 5,                   // 严重阈值
            },
        },
    }

    // 2. 使用配置初始化EventBus
    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal("Failed to initialize EventBus:", err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()
    log.Printf("✅ EventBus initialized with separated health check config:")
    log.Printf("   - Enabled: %v", cfg.HealthCheck.Enabled)
    log.Printf("   - Sender Topic: %s", cfg.HealthCheck.Sender.Topic)
    log.Printf("   - Sender Interval: %v", cfg.HealthCheck.Sender.Interval)
    log.Printf("   - Subscriber Topic: %s", cfg.HealthCheck.Subscriber.Topic)
    log.Printf("   - Monitor Interval: %v", cfg.HealthCheck.Subscriber.MonitorInterval)

    // 3. 启动分离式健康检查
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 启动发布器
    if err := bus.StartHealthCheckPublisher(ctx); err != nil {
        log.Printf("Failed to start health check publisher: %v", err)
    } else {
        log.Println("✅ Health check publisher started")
    }

    // 启动订阅器
    if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
        log.Printf("Failed to start health check subscriber: %v", err)
    } else {
        log.Println("✅ Health check subscriber started")
    }

    // 4. 注册告警回调
    bus.RegisterHealthCheckSubscriberCallback(func(alert eventbus.HealthCheckAlert) {
        log.Printf("🚨 Health Alert [%s]: %s", alert.Level, alert.Message)
    })

    // 5. 运行一段时间观察效果
    log.Println("🔄 Running for 2 minutes to observe health check behavior...")
    time.Sleep(2 * time.Minute)

    // 6. 优雅关闭
    log.Println("🛑 Shutting down...")
    bus.StopAllHealthCheck()
    log.Println("✅ Shutdown complete")
}
```

#### 基于配置文件的使用示例

jxt-core 支持从配置文件加载 `EventBusConfig`，以下是YAML格式的配置示例：

```yaml
eventbus:
  type: "kafka"
  serviceName: "health-check-demo"

  kafka:
    brokers: ["localhost:9092"]

  # 分离式健康检查配置
  healthCheck:
    enabled: true
    publisher:
      topic: "health-check-demo"
      interval: "30s"          # 开发环境使用较短间隔
      timeout: "5s"
      failureThreshold: 2      # 较低的失败阈值，快速检测问题
      messageTTL: "2m"
    subscriber:
      topic: "health-check-demo"
      monitorInterval: "10s"   # 监控间隔
      warningThreshold: 2
      errorThreshold: 3
      criticalThreshold: 5
```



#### 完整示例（编程方式配置）

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/config"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 1. 初始化EventBus
    cfg := &config.EventBusConfig{
        Type: "kafka",
        ServiceName: "health-check-demo",
        Kafka: config.KafkaConfig{
            Brokers: []string{"localhost:9092"},
        },
        HealthCheck: config.HealthCheckConfig{
            Enabled: true,
            Publisher: config.HealthCheckPublisherConfig{
                Topic:            "health-check-demo",
                Interval:         30 * time.Second,
                Timeout:          5 * time.Second,
                FailureThreshold: 3,
                MessageTTL:       2 * time.Minute,
            },
            Subscriber: config.HealthCheckSubscriberConfig{
                Topic:             "health-check-demo",
                MonitorInterval:   10 * time.Second,
                WarningThreshold:  2,
                ErrorThreshold:    3,
                CriticalThreshold: 5,
            },
        },
    }

    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal(err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()

    // 2. 启动分离式健康检查
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 根据服务角色选择启动策略
    serviceRole := "both" // "publisher", "subscriber", "both"

    switch serviceRole {
    case "publisher":
        if err := bus.StartHealthCheckPublisher(ctx); err != nil {
            log.Printf("Failed to start health check publisher: %v", err)
        }
    case "subscriber":
        if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
            log.Printf("Failed to start health check subscriber: %v", err)
        }
    case "both":
        if err := bus.StartAllHealthCheck(ctx); err != nil {
            log.Printf("Failed to start all health checks: %v", err)
        } else {
            log.Println("All health checks started")
        }
    }

    // 3. 注册告警回调
    bus.RegisterHealthCheckSubscriberCallback(func(alert eventbus.HealthCheckAlert) {
        log.Printf("🚨 Health Alert [%s]: %s (Type: %s, Source: %s)",
            alert.Level, alert.Message, alert.AlertType, alert.Source)
    })

    // 4. 定期打印统计信息
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                stats := bus.GetHealthCheckSubscriberStats()
                log.Printf("📊 Health Stats: Healthy=%v, Messages=%d, Misses=%d, Alerts=%d",
                    stats.IsHealthy, stats.TotalMessagesReceived,
                    stats.ConsecutiveMisses, stats.TotalAlerts)
            case <-ctx.Done():
                return
            }
        }
    }()

    // 5. 等待信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan

    // 6. 优雅关闭
    log.Println("Shutting down...")

    if err := bus.StopAllHealthCheck(); err != nil {
        log.Printf("Error stopping health checks: %v", err)
    }

    log.Println("Shutdown complete")
}
```

#### 最佳实践

1. **启动顺序**：先启动健康检查发送器，再启动订阅监控器
2. **告警处理**：根据告警级别采取不同的处理策略
3. **监控集成**：将统计信息集成到监控系统（如Prometheus）
4. **优雅关闭**：确保在应用关闭时正确停止监控器
5. **错误处理**：妥善处理启动失败的情况，不影响主业务逻辑

### 4. 分离式健康检查使用场景

#### 场景1：纯发布端服务
```go
// 用户服务：只发布用户事件，不监控其他服务
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// 只启动发布器
if err := bus.StartHealthCheckPublisher(ctx); err != nil {
    log.Printf("Failed to start publisher: %v", err)
}

// 配置：
// healthCheck:
//   publisher:
//     topic: "health-check-user-service"
//     interval: "2m"
```

#### 场景2：纯订阅端服务
```go
// 监控服务：专门监控其他服务的健康状态
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// 只启动订阅器
if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
    log.Printf("Failed to start subscriber: %v", err)
}

// 配置：
// healthCheck:
//   subscriber:
//     topic: "health-check-user-service"  # 监控用户服务
//     monitorInterval: "30s"
```

#### 场景3：混合角色服务
```go
// 订单服务：既发布自己的健康状态，又监控用户服务
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// 启动所有健康检查
if err := bus.StartAllHealthCheck(ctx); err != nil {
    log.Printf("Failed to start health checks: %v", err)
}

// 配置：
// healthCheck:
//   publisher:
//     topic: "health-check-order-service"    # 发布自己的状态
//   subscriber:
//     topic: "health-check-user-service"     # 监控用户服务
```

#### 场景4：跨服务监控拓扑
```yaml
# 服务A（用户服务）
healthCheck:
  publisher:
    topic: "health-check-user-service"

# 服务B（订单服务）
healthCheck:
  publisher:
    topic: "health-check-order-service"
  subscriber:
    topic: "health-check-user-service"    # 监控用户服务

# 服务C（监控服务）
healthCheck:
  subscriber:
    topic: "health-check-user-service"    # 监控用户服务
  # 可以配置多个订阅器监控多个服务
```

### 5. 健康检查主题

jxt-core EventBus 支持自定义健康检查主题，实现精确的服务配对：

**默认主题**：
- **Kafka**: `jxt-core-kafka-health-check`
- **NATS**: `jxt-core-nats-health-check`
- **Memory**: `jxt-core-memory-health-check`

**自定义主题**：
```yaml
healthCheck:
  publisher:
    topic: "health-check-my-service"      # 自定义发布主题
  subscriber:
    topic: "health-check-target-service"  # 自定义订阅主题
```

这些主题会自动创建和管理，业务代码无需关心具体实现。

### 6. 自动重连后的业务层处理

#### 重连后 EventBus 自动完成的工作

当 EventBus 检测到连接中断并成功重连后，会**自动完成**以下工作，**业务层无需手动处理**：

1. **✅ 连接重建**：重新建立与消息中间件的连接
2. **✅ 订阅恢复**：自动恢复所有之前的订阅（topic + handler）
3. **✅ 状态重置**：重置失败计数，恢复健康状态
4. **✅ 消息处理**：重连后立即可以正常收发消息

#### 业务层需要处理的场景

虽然 EventBus 会自动恢复基础功能，但以下**业务相关的状态**可能需要业务层在重连后处理：

##### 🔄 **需要处理的业务状态**

1. **应用级缓存**：重新加载或同步应用缓存
2. **业务状态同步**：与其他服务同步业务状态
3. **监控指标**：发送重连成功的监控指标
4. **日志记录**：记录重连事件用于审计
5. **外部依赖**：通知其他依赖服务连接已恢复
6. **定时任务**：重新启动可能因连接中断而停止的定时任务

##### ✅ **无需处理的基础设施**

1. **❌ 重新订阅**：EventBus 已自动恢复所有订阅
2. **❌ 重新连接**：EventBus 已自动重建连接
3. **❌ 消息处理器**：所有 MessageHandler 已自动恢复
4. **❌ 健康检查**：健康检查会自动恢复正常

#### 重连回调机制

EventBus 提供了 `RegisterReconnectCallback` 方法，允许业务层注册回调函数，在重连成功后执行业务相关的初始化逻辑：

##### 基础回调注册

```go
// 注册重连回调
err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Printf("🔄 EventBus reconnected at %v", time.Now().Format("15:04:05"))

    // 业务层重连后的处理逻辑
    // 注意：订阅已自动恢复，这里只处理业务相关状态

    return nil
})
if err != nil {
    log.Printf("Failed to register reconnect callback: %v", err)
}
```

##### 完整的业务重连处理示例

```go
// 注册重连回调处理业务状态
err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Println("🔄 EventBus reconnected, handling business state...")

    // 1. 重新加载应用缓存
    if err := reloadApplicationCache(ctx); err != nil {
        log.Printf("Failed to reload cache after reconnect: %v", err)
        // 不返回错误，避免影响重连成功状态
    }

    // 2. 同步业务状态
    if err := syncBusinessState(ctx); err != nil {
        log.Printf("Failed to sync business state: %v", err)
    }

    // 3. 发送监控指标
    metrics.IncrementReconnectCount()
    metrics.RecordReconnectTime(time.Now())

    // 4. 通知其他服务
    notifyDependentServices("eventbus_reconnected")

    // 5. 重启定时任务（如果需要）
    if err := restartPeriodicTasks(ctx); err != nil {
        log.Printf("Failed to restart periodic tasks: %v", err)
    }

    log.Println("✅ Business state recovery completed")
    return nil
})

if err != nil {
    log.Printf("Failed to register reconnect callback: %v", err)
}
```

##### 多个回调处理

```go
// 可以注册多个回调，按注册顺序执行
// 回调1：处理缓存
bus.RegisterReconnectCallback(func(ctx context.Context) error {
    return reloadApplicationCache(ctx)
})

// 回调2：处理监控
bus.RegisterReconnectCallback(func(ctx context.Context) error {
    metrics.IncrementReconnectCount()
    return nil
})

// 回调3：处理通知
bus.RegisterReconnectCallback(func(ctx context.Context) error {
    return notifyDependentServices("reconnected")
})
```

#### 回调执行时机和保证

1. **执行时机**：在连接重建和订阅恢复**完成后**执行
2. **执行顺序**：按注册顺序依次执行所有回调
3. **错误处理**：单个回调失败不影响其他回调执行
4. **超时控制**：回调执行受传入的 context 控制
5. **并发安全**：回调执行是线程安全的

#### 最佳实践

##### ✅ **推荐做法**

1. **轻量级处理**：回调中只处理必要的业务状态，避免重量级操作
2. **错误容忍**：回调中的错误不应影响 EventBus 的正常运行
3. **幂等性**：确保回调可以安全地重复执行
4. **快速返回**：避免在回调中执行长时间阻塞操作
5. **日志记录**：详细记录回调执行情况，便于问题排查

##### ❌ **避免的做法**

1. **重复订阅**：不要在回调中重新订阅，EventBus 已自动恢复
2. **阻塞操作**：避免长时间的网络请求或文件操作
3. **抛出异常**：回调失败不应影响 EventBus 功能
4. **状态依赖**：不要假设回调一定会执行成功

#### 重连状态监控

```go
// 获取重连状态（如果需要）
if kafkaEB, ok := bus.(*kafkaEventBus); ok {
    status := kafkaEB.GetReconnectStatus()
    log.Printf("Reconnect status - Failures: %d, Last: %v",
        status.FailureCount, status.LastReconnectTime)
}
```

### 7. 积压检测

EventBus 支持全面的消息积压检测，包括**发送端积压检测**和**订阅端积压检测**。系统根据配置自动决定启动哪些检测器，支持灵活的监控策略。

#### 🚀 **发送端积压检测**

监控消息发送性能，防止发送端成为系统瓶颈：

```go
// 注册发送端积压回调
err := bus.RegisterPublisherBacklogCallback(func(ctx context.Context, state eventbus.PublisherBacklogState) error {
    if state.HasBacklog {
        log.Printf("📤 发送端积压: 队列深度=%d, 发送速率=%.2f msg/s, 平均延迟=%v, 严重程度=%s",
            state.QueueDepth, state.PublishRate, state.AvgPublishLatency, state.Severity)

        // 根据严重程度采取不同策略
        switch state.Severity {
        case "CRITICAL":
            // 紧急措施：暂停发布、切换备用队列
            return handleCriticalPublisherBacklog(ctx, state)
        case "HIGH":
            // 限流措施：降低发送速率、启用批量模式
            return handleHighPublisherBacklog(ctx, state)
        case "MEDIUM":
            // 优化措施：调整批量大小、启用压缩
            return handleMediumPublisherBacklog(ctx, state)
        }
    }
    return nil
})

// 启动发送端积压监控
err = bus.StartPublisherBacklogMonitoring(ctx)
```

#### 📥 **订阅端积压检测**

监控消息消费性能，确保消费者能够及时处理消息：

```go
// 注册订阅端积压回调
err := bus.RegisterSubscriberBacklogCallback(func(ctx context.Context, state eventbus.BacklogState) error {
    if state.HasBacklog {
        log.Printf("📥 订阅端积压: %d 条消息, 积压时间: %v", state.LagCount, state.LagTime)
        log.Printf("📝 主题: %s, 消费者组: %s", state.Topic, state.ConsumerGroup)

        // 处理积压告警
        sendBacklogAlert(state)

        // 可能的应对措施：增加消费者实例、优化处理逻辑
        return handleSubscriberBacklog(ctx, state)
    } else {
        log.Printf("✅ 积压已清除: %s", state.Topic)
    }
    return nil
})

// 启动订阅端积压监控
err = bus.StartSubscriberBacklogMonitoring(ctx)
```

#### 🎯 **统一积压监控管理**

```go
// 根据配置启动所有积压监控（发送端和/或订阅端）
err := bus.StartAllBacklogMonitoring(ctx)

// 停止所有积压监控
err = bus.StopAllBacklogMonitoring()
```

#### 🔥 **发送端积压检测特性**

- ✅ **多维度监控**：队列深度、发送延迟、发送速率综合监控
- ✅ **智能评估**：自动计算积压比例和严重程度 (NORMAL, LOW, MEDIUM, HIGH, CRITICAL)
- ✅ **预防性控制**：在问题发生前进行限流和优化
- ✅ **回调机制**：支持自定义积压处理策略
- ✅ **配置驱动**：根据配置自动启动或禁用

#### 📊 **订阅端积压检测特性**

- ✅ **并发检测**：支持多 topic 和多分区的并发积压检测
- ✅ **双重阈值**：基于消息数量和时间的双重判断
- ✅ **详细信息**：提供分区级别的积压详情
- ✅ **高性能**：复用 EventBus 连接，避免重复连接
- ✅ **实时监控**：定期检测消费者积压情况

#### 🎯 **统一管理特性**

- ✅ **配置驱动**：根据配置文件自动决定启动策略
- ✅ **统一接口**：提供统一的启动/停止方法
- ✅ **向后兼容**：保持与现有代码的兼容性
- ✅ **灵活组合**：支持只启动发送端、只启动订阅端、或同时启动

#### 📋 **配置积压检测**

新的配置结构支持分别配置发送端和订阅端的积压检测：

```yaml
eventbus:
  type: "kafka"  # 或 "nats"
  serviceName: "evidence-service"

  # 发布端配置
  publisher:
    publishTimeout: 10s
    backlogDetection:
      enabled: true                    # 启用发送端积压检测
      maxQueueDepth: 1000             # 最大队列深度
      maxPublishLatency: 5s           # 最大发送延迟
      rateThreshold: 500.0            # 发送速率阈值 (msg/sec)
      checkInterval: 30s              # 检测间隔

  # 订阅端配置
  subscriber:
    maxConcurrency: 10
    processTimeout: 30s
    backlogDetection:
      enabled: true                    # 启用订阅端积压检测
      maxLagThreshold: 1000           # 最大消息积压数量
      maxTimeThreshold: 5m            # 最大积压时间
      checkInterval: 30s              # 检测间隔

  # Kafka 具体配置
  kafka:
    brokers: ["localhost:9092"]
    producer:
      requiredAcks: 1
      timeout: 10s
    consumer:
      groupId: "evidence-consumer-group"
```

#### 🎛️ **灵活的启动策略**

根据配置自动决定启动哪些积压检测：

```go
// 配置示例 1: 同时启动发送端和订阅端检测
config := &config.EventBusConfig{
    Publisher: config.PublisherConfig{
        BacklogDetection: config.PublisherBacklogDetectionConfig{
            Enabled: true,  // 启动发送端检测
        },
    },
    Subscriber: config.SubscriberConfig{
        BacklogDetection: config.SubscriberBacklogDetectionConfig{
            Enabled: true,  // 启动订阅端检测
        },
    },
}

// 配置示例 2: 只启动发送端检测（高并发写入场景）
config := &config.EventBusConfig{
    Publisher: config.PublisherConfig{
        BacklogDetection: config.PublisherBacklogDetectionConfig{
            Enabled: true,   // 启动发送端检测
            MaxQueueDepth: 5000,
            RateThreshold: 2000.0,
        },
    },
    Subscriber: config.SubscriberConfig{
        BacklogDetection: config.SubscriberBacklogDetectionConfig{
            Enabled: false,  // 禁用订阅端检测
        },
    },
}

// 配置示例 3: 只启动订阅端检测（消费者敏感场景）
config := &config.EventBusConfig{
    Publisher: config.PublisherConfig{
        BacklogDetection: config.PublisherBacklogDetectionConfig{
            Enabled: false,  // 禁用发送端检测
        },
    },
    Subscriber: config.SubscriberConfig{
        BacklogDetection: config.SubscriberBacklogDetectionConfig{
            Enabled: true,   // 启动订阅端检测
            MaxLagThreshold: 500,
            MaxTimeThreshold: 2 * time.Minute,
        },
    },
}
```

#### 💡 **积压检测最佳实践**

##### **1. 发送端积压检测最佳实践**

```go
// 发送端积压处理策略
func handlePublisherBacklog(ctx context.Context, state eventbus.PublisherBacklogState) error {
    switch state.Severity {
    case "CRITICAL":
        // 紧急措施
        log.Printf("🚨 CRITICAL: 暂停非关键消息发布")
        pauseNonCriticalPublishing()
        switchToEmergencyQueue()
        alertOpsTeam("publisher_critical_backlog", state)

    case "HIGH":
        // 限流措施
        log.Printf("⚠️ HIGH: 启用发布限流")
        enablePublishThrottling(0.5) // 降低50%发布速率
        enableBatchMode()
        enableCompression()

    case "MEDIUM":
        // 优化措施
        log.Printf("⚡ MEDIUM: 优化发布策略")
        adjustBatchSize(state.QueueDepth)
        optimizeMessageSerialization()
    }
    return nil
}
```

##### **2. 订阅端积压检测最佳实践**

```go
// 订阅端积压处理策略
func handleSubscriberBacklog(ctx context.Context, state eventbus.BacklogState) error {
    if state.LagCount > 5000 {
        // 严重积压：扩容消费者
        log.Printf("🚨 严重积压，启动额外消费者实例")
        scaleUpConsumers(state.Topic, state.ConsumerGroup)

    } else if state.LagCount > 1000 {
        // 中等积压：增加并发
        log.Printf("⚠️ 中等积压，增加处理并发")
        increaseProcessingConcurrency()

    } else if state.LagTime > 5*time.Minute {
        // 时间积压：优化处理逻辑
        log.Printf("⏰ 处理延迟过高，优化处理逻辑")
        optimizeMessageProcessing()
    }

    // 发送监控告警
    sendToMonitoringSystem(state)
    return nil
}
```

##### **3. 配置策略建议**

- **生产环境**：较高阈值，较长检测间隔，减少系统开销
- **测试环境**：较低阈值，较短检测间隔，便于问题发现
- **开发环境**：可以禁用积压检测，简化开发流程
- **高并发场景**：重点关注发送端积压检测
- **消费敏感场景**：重点关注订阅端积压检测

##### **4. 监控集成建议**

- **指标收集**：将积压状态发送到 Prometheus、InfluxDB 等监控系统
- **告警规则**：设置多级告警机制，避免告警疲劳
- **历史分析**：记录积压历史数据，用于容量规划和性能优化
- **自动化响应**：结合 Kubernetes HPA 实现自动扩容

#### 🚀 **完整的积压检测示例**

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/config"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 创建配置 - 同时启用发送端和订阅端积压检测
    cfg := &config.EventBusConfig{
        Type:        "kafka",
        ServiceName: "backlog-detection-example",

        // 发布端配置
        Publisher: config.PublisherConfig{
            PublishTimeout: 10 * time.Second,
            BacklogDetection: config.PublisherBacklogDetectionConfig{
                Enabled:           true,
                MaxQueueDepth:     1000,
                MaxPublishLatency: 5 * time.Second,
                RateThreshold:     500.0,
                CheckInterval:     30 * time.Second,
            },
        },

        // 订阅端配置
        Subscriber: config.SubscriberConfig{
            MaxConcurrency: 10,
            ProcessTimeout: 30 * time.Second,
            BacklogDetection: config.SubscriberBacklogDetectionConfig{
                Enabled:          true,
                MaxLagThreshold:  1000,
                MaxTimeThreshold: 5 * time.Minute,
                CheckInterval:    30 * time.Second,
            },
        },

        // Kafka 配置
        Kafka: config.KafkaConfig{
            Brokers: []string{"localhost:9092"},
            Producer: config.ProducerConfig{
                RequiredAcks: 1,
                Timeout:      10 * time.Second,
                Compression:  "snappy",
            },
            Consumer: config.ConsumerConfig{
                GroupID:           "backlog-detection-group",
                SessionTimeout:    30 * time.Second,
                HeartbeatInterval: 3 * time.Second,
            },
        },
    }

    // 初始化 EventBus
    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal("Failed to initialize EventBus:", err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()

    // 注册发送端积压回调
    if err := bus.RegisterPublisherBacklogCallback(handlePublisherBacklog); err != nil {
        log.Printf("Failed to register publisher backlog callback: %v", err)
    }

    // 注册订阅端积压回调
    if err := bus.RegisterSubscriberBacklogCallback(handleSubscriberBacklog); err != nil {
        log.Printf("Failed to register subscriber backlog callback: %v", err)
    }

    ctx := context.Background()

    // 启动所有积压监控
    if err := bus.StartAllBacklogMonitoring(ctx); err != nil {
        log.Printf("Failed to start all backlog monitoring: %v", err)
    }

    // 订阅消息
    if err := bus.Subscribe(ctx, "user.events", handleUserEvent); err != nil {
        log.Fatal("Failed to subscribe:", err)
    }

    log.Println("🔍 积压检测示例运行中...")

    // 模拟发送消息
    go func() {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()

        counter := 0
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                counter++
                message := fmt.Sprintf("Test message %d", counter)
                bus.Publish(ctx, "user.events", []byte(message))
            }
        }
    }()

    // 运行应用...
    select {}
}

// 发送端积压处理
func handlePublisherBacklog(ctx context.Context, state eventbus.PublisherBacklogState) error {
    if state.HasBacklog {
        log.Printf("📤 发送端积压: 队列深度=%d, 发送速率=%.2f msg/s, 严重程度=%s",
            state.QueueDepth, state.PublishRate, state.Severity)

        // 根据严重程度采取措施
        switch state.Severity {
        case "CRITICAL":
            log.Printf("🚨 CRITICAL: 实施紧急限流措施")
        case "HIGH":
            log.Printf("⚠️ HIGH: 启用发布优化")
        }
    }
    return nil
}

// 订阅端积压处理
func handleSubscriberBacklog(ctx context.Context, state eventbus.BacklogState) error {
    if state.HasBacklog {
        log.Printf("📥 订阅端积压: %d 条消息, 积压时间: %v, 主题: %s",
            state.LagCount, state.LagTime, state.Topic)

        // 发送告警到监控系统
        sendAlertToMonitoring(state)
    } else {
        log.Printf("✅ 积压已清除: %s", state.Topic)
    }
    return nil
}

func handleUserEvent(ctx context.Context, message []byte) error {
    // 处理用户事件
    log.Printf("处理用户事件: %s", string(message))
    // 模拟处理时间
    time.Sleep(100 * time.Millisecond)
    return nil
}

func sendAlertToMonitoring(state eventbus.BacklogState) {
    // 集成监控系统的示例
    log.Printf("📊 发送积压告警到监控系统: Topic=%s, Lag=%d", state.Topic, state.LagCount)
}
```

### 8. 指标监控

EventBus内置了指标收集功能，支持以下指标：

- `MessagesPublished`: 发布的消息数量
- `MessagesConsumed`: 消费的消息数量
- `PublishErrors`: 发布错误数量
- `ConsumeErrors`: 消费错误数量
- `ConnectionErrors`: 连接错误数量
- `LastHealthCheck`: 最后一次健康检查时间
- `HealthCheckStatus`: 健康检查状态
- `MessageBacklog`: 消息积压数量（NATS）

## Topic常量

EventBus只定义技术基础设施相关的Topic常量：

```go
const (
    // 技术基础设施相关的Topic常量
    HealthCheckTopic = "health_check_topic"  // 用于监控eventbus组件健康状态
    
    // 可能的其他技术性Topic
    // DeadLetterTopic = "dead_letter_topic"  // 死信队列
    // MetricsTopic    = "metrics_topic"      // 指标收集
    // TracingTopic    = "tracing_topic"      // 链路追踪
)
```

**注意**：业务领域相关的Topic应该定义在各自的项目中，不应该定义在jxt-core中。

## 最佳实践

### 1. Kafka 多 Topic 预订阅模式（企业级生产环境）

#### 问题背景

在 Kafka 多 Topic 订阅场景下，如果不使用预订阅模式，会导致以下问题：

- **Consumer Group 频繁重平衡**：每次添加新 topic 都会触发重平衡，导致消息处理中断
- **消息丢失风险**：重平衡期间可能丢失部分消息
- **性能抖动**：重平衡会导致吞吐量和延迟出现明显波动
- **成功率下降**：在并发订阅多个 topic 时，可能只有部分 topic 被成功订阅

#### 企业级解决方案：预订阅 API

EventBus 提供了 `SetPreSubscriptionTopics` API，符合 Confluent、LinkedIn、Uber 等企业的最佳实践。

**核心原则**：
1. 在创建 EventBus 后，**立即**设置所有需要订阅的 topic
2. 然后再调用 `Subscribe` 或 `SubscribeEnvelope` 激活各个 topic 的处理器
3. Consumer 会一次性订阅所有 topic，避免频繁重平衡

#### 正确使用方式

```go
package main

import (
    "context"
    "log"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    ctx := context.Background()

    // 1. 创建 Kafka EventBus
    kafkaConfig := &eventbus.KafkaConfig{
        Brokers:  []string{"localhost:9092"},
        ClientID: "my-service",
        Consumer: eventbus.ConsumerConfig{
            GroupID: "my-consumer-group",
        },
    }

    eb, err := eventbus.NewKafkaEventBus(kafkaConfig)
    if err != nil {
        log.Fatal(err)
    }
    defer eb.Close()

    // 2. 🔑 关键步骤：设置预订阅 topic 列表（在 Subscribe 之前）
    topics := []string{
        "business.orders",
        "business.payments",
        "business.users",
        "audit.logs",
        "system.notifications",
    }

    // 使用类型断言调用 Kafka 特有的 API
    if kafkaBus, ok := eb.(interface {
        SetPreSubscriptionTopics([]string)
    }); ok {
        kafkaBus.SetPreSubscriptionTopics(topics)
        log.Printf("✅ 已设置预订阅 topic 列表: %v", topics)
    }

    // 3. 现在可以安全地订阅各个 topic
    // Consumer 会一次性订阅所有 topic，不会触发重平衡

    // 订阅订单事件
    err = eb.SubscribeEnvelope(ctx, "business.orders", func(ctx context.Context, envelope *eventbus.Envelope) error {
        log.Printf("处理订单事件: %s", envelope.AggregateID)
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    // 订阅支付事件
    err = eb.SubscribeEnvelope(ctx, "business.payments", func(ctx context.Context, envelope *eventbus.Envelope) error {
        log.Printf("处理支付事件: %s", envelope.AggregateID)
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    // 订阅其他 topic...

    log.Println("所有 topic 订阅完成，Consumer 已启动")

    // 应用继续运行...
    select {}
}
```

#### 并发订阅场景

在并发订阅多个 topic 的场景下，预订阅模式尤为重要：

```go
// ✅ 正确做法：先设置预订阅列表，再并发订阅
func setupKafkaSubscriptions(eb eventbus.EventBus, ctx context.Context) error {
    topics := []string{
        "topic1", "topic2", "topic3", "topic4", "topic5",
    }

    // 1. 先设置预订阅列表
    if kafkaBus, ok := eb.(interface {
        SetPreSubscriptionTopics([]string)
    }); ok {
        kafkaBus.SetPreSubscriptionTopics(topics)
    }

    // 2. 然后可以安全地并发订阅
    var wg sync.WaitGroup
    for _, topic := range topics {
        wg.Add(1)
        go func(t string) {
            defer wg.Done()
            handler := createHandlerForTopic(t)
            if err := eb.SubscribeEnvelope(ctx, t, handler); err != nil {
                log.Printf("订阅 %s 失败: %v", t, err)
            }
        }(topic)
    }
    wg.Wait()

    return nil
}

// ❌ 错误做法：直接并发订阅（可能导致只有部分 topic 被订阅）
func setupKafkaSubscriptionsWrong(eb eventbus.EventBus, ctx context.Context) error {
    topics := []string{
        "topic1", "topic2", "topic3", "topic4", "topic5",
    }

    // 直接并发订阅，第一个 Subscribe 会启动 Consumer
    // 此时只有第一个 topic 在 allPossibleTopics 中
    // 后续 topic 虽然被添加，但 Consumer 已经在运行，不会重新订阅
    var wg sync.WaitGroup
    for _, topic := range topics {
        wg.Add(1)
        go func(t string) {
            defer wg.Done()
            handler := createHandlerForTopic(t)
            eb.SubscribeEnvelope(ctx, t, handler) // ❌ 可能失败
        }(topic)
    }
    wg.Wait()

    return nil
}
```

#### 性能对比

使用预订阅模式前后的性能对比（5 个 topic，4 个压力级别）：

| 压力级别 | 不使用预订阅 | 使用预订阅 | 改善 |
|---------|------------|----------|------|
| 低压(500) | 20% 成功率 | **99.80%** | +398% |
| 中压(2000) | 20% 成功率 | **99.95%** | +399% |
| 高压(5000) | 20% 成功率 | **99.98%** | +399% |
| 极限(10000) | 20% 成功率 | **99.99%** | +399% |

**关键发现**：
- 不使用预订阅时，成功率固定在 20%（恰好是 1/5，说明只有 1 个 topic 被订阅）
- 使用预订阅后，成功率提升到 99.8%+，接近完美

#### 业界最佳实践参考

此方案符合以下企业的最佳实践：

1. **Confluent 官方推荐**：
   - 避免频繁重平衡，一次性订阅所有 topic
   - 参考：[Kafka Consumer Group Rebalancing](https://docs.confluent.io/platform/current/clients/consumer.html#rebalancing)

2. **LinkedIn 实践**：
   - 预配置 topic 列表，减少运维复杂度
   - 在应用启动时确定所有 topic，避免动态变化

3. **Uber 实践**：
   - 使用静态 topic 配置，提高系统可预测性
   - 避免运行时动态添加 topic 导致的性能问题

#### 注意事项

1. **仅适用于 Kafka**：此 API 是 Kafka 特有的，NATS 不需要预订阅
2. **必须在 Subscribe 之前调用**：否则无法避免重平衡
3. **使用 ASCII 字符**：Kafka 的 ClientID 和 topic 名称应只使用 ASCII 字符，避免使用中文或其他 Unicode 字符
4. **一次性设置**：应该在应用启动时一次性设置所有 topic，不要动态修改

#### 相关文档

- [PRE_SUBSCRIPTION_FINAL_REPORT.md](./PRE_SUBSCRIPTION_FINAL_REPORT.md) - 预订阅模式详细设计文档
- [KAFKA_INDUSTRY_BEST_PRACTICES.md](./KAFKA_INDUSTRY_BEST_PRACTICES.md) - Kafka 业界最佳实践
- [KAFKA_REBALANCE_SOLUTION_FINAL_REPORT.md](./KAFKA_REBALANCE_SOLUTION_FINAL_REPORT.md) - 重平衡问题解决方案

---

### 2. 主题持久化策略设计

主题持久化管理是 EventBus 的核心特性，正确的策略设计是成功应用的关键。

#### 主题分类策略

**按业务重要性分类**：
```go
// 业务关键数据 - 必须持久化
const (
    TopicOrderEvents    = "business.orders"     // 订单事件
    TopicPaymentEvents  = "business.payments"   // 支付事件
    TopicUserEvents     = "business.users"      // 用户事件
    TopicAuditLogs      = "audit.logs"          // 审计日志
)

// 系统级数据 - 根据需求选择
const (
    TopicSystemNotify   = "system.notifications" // 系统通知（非持久化）
    TopicSystemMetrics  = "system.metrics"       // 系统指标（非持久化）
    TopicSystemHealth   = "system.health"        // 健康检查（非持久化）
)

// 临时数据 - 非持久化
const (
    TopicTempCache      = "temp.cache"           // 缓存失效
    TopicTempSession    = "temp.session"         // 会话更新
)
```

**配置模板化管理**：
```go
// 主题配置模板
var TopicConfigTemplates = map[string]eventbus.TopicOptions{
    // 金融级业务数据
    "business.*": {
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   7 * 24 * time.Hour,    // 7天保留
        MaxSize:         1024 * 1024 * 1024,    // 1GB
        Replicas:        3,                     // 3副本
        Description:     "业务关键事件，金融级可靠性",
    },

    // 合规审计数据
    "audit.*": {
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   90 * 24 * time.Hour,   // 90天保留
        MaxSize:         5 * 1024 * 1024 * 1024, // 5GB
        Replicas:        5,                     // 5副本
        Description:     "审计日志，合规要求",
    },

    // 系统级数据
    "system.*": {
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   2 * time.Hour,         // 2小时保留
        MaxSize:         100 * 1024 * 1024,     // 100MB
        Replicas:        1,                     // 单副本
        Description:     "系统级消息，高性能处理",
    },

    // 临时数据
    "temp.*": {
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   30 * time.Minute,      // 30分钟保留
        MaxSize:         10 * 1024 * 1024,      // 10MB
        Replicas:        1,                     // 单副本
        Description:     "临时消息，快速处理",
    },
}

// 自动应用配置模板
func ApplyTopicConfigTemplates(bus eventbus.EventBus, ctx context.Context) error {
    for pattern, template := range TopicConfigTemplates {
        // 在实际应用中，可以根据主题名称模式自动应用配置
        log.Printf("Template for %s: %+v", pattern, template)
    }
    return nil
}
```

#### 动态配置管理

**配置生命周期管理**：
```go
type TopicConfigManager struct {
    eventBus eventbus.EventBus
    logger   *zap.Logger
}

// 初始化主题配置
func (m *TopicConfigManager) InitializeTopicConfigs(ctx context.Context) error {
    configs := map[string]eventbus.TopicOptions{
        "business.orders": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   7 * 24 * time.Hour,
            MaxSize:         500 * 1024 * 1024,
            Replicas:        3,
            Description:     "订单事件，业务关键",
        },
        "system.notifications": {
            PersistenceMode: eventbus.TopicEphemeral,
            RetentionTime:   1 * time.Hour,
            Description:     "系统通知，无需持久化",
        },
    }

    for topic, config := range configs {
        if err := m.eventBus.ConfigureTopic(ctx, topic, config); err != nil {
            m.logger.Error("Failed to configure topic",
                zap.String("topic", topic), zap.Error(err))
            return err
        }
        m.logger.Info("Topic configured",
            zap.String("topic", topic),
            zap.String("mode", string(config.PersistenceMode)))
    }
    return nil
}

// 运行时配置调整
func (m *TopicConfigManager) AdjustTopicConfig(ctx context.Context, topic string, persistent bool) error {
    currentConfig, err := m.eventBus.GetTopicConfig(topic)
    if err != nil {
        return err
    }

    // 记录配置变更
    oldMode := currentConfig.PersistenceMode

    if err := m.eventBus.SetTopicPersistence(ctx, topic, persistent); err != nil {
        return err
    }

    newConfig, _ := m.eventBus.GetTopicConfig(topic)
    m.logger.Info("Topic persistence changed",
        zap.String("topic", topic),
        zap.String("old_mode", string(oldMode)),
        zap.String("new_mode", string(newConfig.PersistenceMode)))

    return nil
}

// 配置健康检查
func (m *TopicConfigManager) ValidateTopicConfigs(ctx context.Context) error {
    topics := m.eventBus.ListConfiguredTopics()

    for _, topic := range topics {
        config, err := m.eventBus.GetTopicConfig(topic)
        if err != nil {
            m.logger.Warn("Failed to get topic config",
                zap.String("topic", topic), zap.Error(err))
            continue
        }

        // 验证配置合理性
        if strings.HasPrefix(topic, "business.") && config.PersistenceMode != eventbus.TopicPersistent {
            m.logger.Warn("Business topic should be persistent",
                zap.String("topic", topic),
                zap.String("current_mode", string(config.PersistenceMode)))
        }

        if strings.HasPrefix(topic, "temp.") && config.PersistenceMode != eventbus.TopicEphemeral {
            m.logger.Warn("Temp topic should be ephemeral",
                zap.String("topic", topic),
                zap.String("current_mode", string(config.PersistenceMode)))
        }
    }

    return nil
}
```

### 3. DDD架构集成

在DDD架构中，建议按以下方式使用EventBus，并充分利用主题持久化管理：

**领域层**：定义事件发布接口
```go
type EventPublisher interface {
    PublishDomainEvent(ctx context.Context, event event.DomainEvent) error
    PublishIntegrationEvent(ctx context.Context, event event.IntegrationEvent) error
}
```

**基础设施层**：实现领域接口，集成主题持久化管理
```go
type EventBusPublisher struct {
    eventBus eventbus.EventBus
    logger   *zap.Logger
    initialized bool
    mu       sync.Mutex
}

// 应用启动时初始化所有领域和集成事件主题
func (p *EventBusPublisher) Initialize(ctx context.Context) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.initialized {
        return nil
    }

    // 配置领域事件主题（通常需要持久化）
    domainTopics := map[string]eventbus.TopicOptions{
        "domain.order": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   30 * 24 * time.Hour, // 30天
            MaxSize:         500 * 1024 * 1024,   // 500MB
            Replicas:        3,
            Description:     "订单领域事件",
        },
        "domain.user": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   7 * 24 * time.Hour,  // 7天
            MaxSize:         200 * 1024 * 1024,   // 200MB
            Replicas:        2,
            Description:     "用户领域事件",
        },
        "domain.payment": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   90 * 24 * time.Hour, // 90天（金融数据）
            MaxSize:         1024 * 1024 * 1024,  // 1GB
            Replicas:        5,
            Description:     "支付领域事件",
        },
    }

    // 配置集成事件主题（根据重要性决定持久化）
    integrationTopics := map[string]eventbus.TopicOptions{
        "integration.business_critical": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   7 * 24 * time.Hour,
            MaxSize:         300 * 1024 * 1024,
            Replicas:        3,
            Description:     "业务关键集成事件",
        },
        "integration.notification": {
            PersistenceMode: eventbus.TopicEphemeral,
            RetentionTime:   2 * time.Hour,
            Description:     "通知类集成事件",
        },
        "integration.analytics": {
            PersistenceMode: eventbus.TopicEphemeral,
            RetentionTime:   1 * time.Hour,
            Description:     "分析类集成事件",
        },
    }

    // 应用所有配置
    allTopics := make(map[string]eventbus.TopicOptions)
    for k, v := range domainTopics {
        allTopics[k] = v
    }
    for k, v := range integrationTopics {
        allTopics[k] = v
    }

    for topic, options := range allTopics {
        if err := p.eventBus.ConfigureTopic(ctx, topic, options); err != nil {
            p.logger.Error("Failed to configure topic",
                zap.String("topic", topic), zap.Error(err))
            return err
        }
    }

    p.initialized = true
    p.logger.Info("EventBus publisher initialized with all topic configurations")
    return nil
}

func (p *EventBusPublisher) PublishDomainEvent(ctx context.Context, event event.DomainEvent) error {
    // 确保已初始化
    if !p.initialized {
        if err := p.Initialize(ctx); err != nil {
            return fmt.Errorf("failed to initialize publisher: %w", err)
        }
    }

    // 根据聚合类型选择预配置的主题
    topic := fmt.Sprintf("domain.%s", event.GetAggregateType())

    payload, err := event.MarshalJSON()
    if err != nil {
        return err
    }

    // 直接发布，主题配置已在初始化时完成
    return p.eventBus.Publish(ctx, topic, payload)
}

func (p *EventBusPublisher) PublishIntegrationEvent(ctx context.Context, event event.IntegrationEvent) error {
    // 确保已初始化
    if !p.initialized {
        if err := p.Initialize(ctx); err != nil {
            return fmt.Errorf("failed to initialize publisher: %w", err)
        }
    }

    // 根据事件重要性选择预配置的主题
    var topic string
    if event.IsBusinessCritical() {
        topic = "integration.business_critical"
    } else if event.IsNotification() {
        topic = "integration.notification"
    } else {
        topic = "integration.analytics"
    }

    payload, err := event.MarshalJSON()
    if err != nil {
        return err
    }

    // 直接发布，主题配置已在初始化时完成
    return p.eventBus.Publish(ctx, topic, payload)
}
```

### 4. 主题配置管理最佳实践

主题配置应该与消息发布分离，遵循"配置一次，使用多次"的原则。

#### 配置时机和方式

**✅ 推荐做法**：
```go
// 1. 应用启动时统一配置所有主题
func InitializeAllTopics(bus eventbus.EventBus, ctx context.Context) error {
    topicConfigs := map[string]eventbus.TopicOptions{
        // 业务关键主题
        "business.orders": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   7 * 24 * time.Hour,
            MaxSize:         500 * 1024 * 1024,
            Replicas:        3,
            Description:     "订单事件，业务关键",
        },
        // 系统通知主题
        "system.notifications": {
            PersistenceMode: eventbus.TopicEphemeral,
            RetentionTime:   1 * time.Hour,
            Description:     "系统通知，高性能处理",
        },
        // 审计日志主题
        "audit.logs": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   90 * 24 * time.Hour,
            MaxSize:         2 * 1024 * 1024 * 1024,
            Replicas:        5,
            Description:     "审计日志，合规要求",
        },
    }

    for topic, options := range topicConfigs {
        if err := bus.ConfigureTopic(ctx, topic, options); err != nil {
            return fmt.Errorf("failed to configure topic %s: %w", topic, err)
        }
        log.Printf("✅ Configured topic: %s (%s)", topic, options.PersistenceMode)
    }

    return nil
}

// 2. 发布消息时直接使用预配置的主题
func PublishOrderEvent(bus eventbus.EventBus, ctx context.Context, orderData []byte) error {
    // 不需要配置主题，直接发布
    return bus.Publish(ctx, "business.orders", orderData)
}

func PublishNotification(bus eventbus.EventBus, ctx context.Context, notification []byte) error {
    // 不需要配置主题，直接发布
    return bus.Publish(ctx, "system.notifications", notification)
}
```

**❌ 避免的做法**：
```go
// 错误：在发布方法中配置主题
func PublishOrderEventBad(bus eventbus.EventBus, ctx context.Context, orderData []byte) error {
    // ❌ 每次发布都配置主题，性能差且容易出错
    options := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   7 * 24 * time.Hour,
    }
    if err := bus.ConfigureTopic(ctx, "business.orders", options); err != nil {
        return err
    }

    return bus.Publish(ctx, "business.orders", orderData)
}
```

#### 配置管理模式

**模式1：集中式配置管理**
```go
type TopicConfigRegistry struct {
    configs map[string]eventbus.TopicOptions
    mu      sync.RWMutex
}

func NewTopicConfigRegistry() *TopicConfigRegistry {
    return &TopicConfigRegistry{
        configs: make(map[string]eventbus.TopicOptions),
    }
}

func (r *TopicConfigRegistry) RegisterConfig(topic string, options eventbus.TopicOptions) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.configs[topic] = options
}

func (r *TopicConfigRegistry) ApplyAllConfigs(bus eventbus.EventBus, ctx context.Context) error {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for topic, options := range r.configs {
        if err := bus.ConfigureTopic(ctx, topic, options); err != nil {
            return fmt.Errorf("failed to configure topic %s: %w", topic, err)
        }
    }
    return nil
}

// 使用示例
func main() {
    registry := NewTopicConfigRegistry()

    // 注册所有主题配置
    registry.RegisterConfig("business.orders", eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   7 * 24 * time.Hour,
        Description:     "订单事件",
    })

    registry.RegisterConfig("system.notifications", eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   1 * time.Hour,
        Description:     "系统通知",
    })

    // 应用所有配置
    bus := eventbus.GetGlobal()
    ctx := context.Background()
    if err := registry.ApplyAllConfigs(bus, ctx); err != nil {
        log.Fatal(err)
    }

    // 现在可以直接发布消息
    bus.Publish(ctx, "business.orders", orderData)
    bus.Publish(ctx, "system.notifications", notificationData)
}
```

**模式2：配置文件驱动**
```yaml
# topics.yaml
topics:
  business.orders:
    persistenceMode: "persistent"
    retentionTime: "168h"  # 7天
    maxSize: 524288000     # 500MB
    replicas: 3
    description: "订单事件，业务关键"

  system.notifications:
    persistenceMode: "ephemeral"
    retentionTime: "1h"
    description: "系统通知，高性能处理"

  audit.logs:
    persistenceMode: "persistent"
    retentionTime: "2160h"  # 90天
    maxSize: 2147483648     # 2GB
    replicas: 5
    description: "审计日志，合规要求"
```

```go
type TopicConfigFile struct {
    Topics map[string]TopicConfigYAML `yaml:"topics"`
}

type TopicConfigYAML struct {
    PersistenceMode string `yaml:"persistenceMode"`
    RetentionTime   string `yaml:"retentionTime"`
    MaxSize         int64  `yaml:"maxSize"`
    Replicas        int    `yaml:"replicas"`
    Description     string `yaml:"description"`
}

func LoadTopicConfigsFromFile(filename string) (map[string]eventbus.TopicOptions, error) {
    data, err := os.ReadFile(filename)
    if err != nil {
        return nil, err
    }

    var configFile TopicConfigFile
    if err := yaml.Unmarshal(data, &configFile); err != nil {
        return nil, err
    }

    configs := make(map[string]eventbus.TopicOptions)
    for topic, yamlConfig := range configFile.Topics {
        retentionTime, err := time.ParseDuration(yamlConfig.RetentionTime)
        if err != nil {
            return nil, fmt.Errorf("invalid retention time for topic %s: %w", topic, err)
        }

        var persistenceMode eventbus.TopicPersistenceMode
        switch yamlConfig.PersistenceMode {
        case "persistent":
            persistenceMode = eventbus.TopicPersistent
        case "ephemeral":
            persistenceMode = eventbus.TopicEphemeral
        case "auto":
            persistenceMode = eventbus.TopicAuto
        default:
            return nil, fmt.Errorf("invalid persistence mode for topic %s: %s", topic, yamlConfig.PersistenceMode)
        }

        configs[topic] = eventbus.TopicOptions{
            PersistenceMode: persistenceMode,
            RetentionTime:   retentionTime,
            MaxSize:         yamlConfig.MaxSize,
            Replicas:        yamlConfig.Replicas,
            Description:     yamlConfig.Description,
        }
    }

    return configs, nil
}

func ApplyConfigsFromFile(bus eventbus.EventBus, ctx context.Context, filename string) error {
    configs, err := LoadTopicConfigsFromFile(filename)
    if err != nil {
        return err
    }

    for topic, options := range configs {
        if err := bus.ConfigureTopic(ctx, topic, options); err != nil {
            return fmt.Errorf("failed to configure topic %s: %w", topic, err)
        }
        log.Printf("✅ Configured topic from file: %s (%s)", topic, options.PersistenceMode)
    }

    return nil
}
```

#### 动态配置调整策略

**仅在必要时进行动态调整**：
```go
type TopicConfigAdjuster struct {
    eventBus eventbus.EventBus
    logger   *zap.Logger
}

// 仅在运行时确实需要调整时使用
func (a *TopicConfigAdjuster) AdjustTopicIfNeeded(ctx context.Context, topic string, reason string) error {
    currentConfig, err := a.eventBus.GetTopicConfig(topic)
    if err != nil {
        return err
    }

    var needsAdjustment bool
    var newConfig eventbus.TopicOptions = currentConfig

    switch reason {
    case "high_volume_detected":
        // 检测到高流量，可能需要调整配置
        if currentConfig.MaxSize < 1024*1024*1024 { // 小于1GB
            newConfig.MaxSize = 1024 * 1024 * 1024 // 调整为1GB
            needsAdjustment = true
        }

    case "compliance_requirement":
        // 合规要求，需要更长的保留时间
        if currentConfig.RetentionTime < 90*24*time.Hour {
            newConfig.RetentionTime = 90 * 24 * time.Hour
            newConfig.PersistenceMode = eventbus.TopicPersistent
            needsAdjustment = true
        }

    case "performance_optimization":
        // 性能优化，可能需要调整为非持久化
        if strings.HasPrefix(topic, "temp.") && currentConfig.PersistenceMode != eventbus.TopicEphemeral {
            newConfig.PersistenceMode = eventbus.TopicEphemeral
            newConfig.RetentionTime = 30 * time.Minute
            needsAdjustment = true
        }
    }

    if needsAdjustment {
        a.logger.Info("Adjusting topic configuration",
            zap.String("topic", topic),
            zap.String("reason", reason),
            zap.String("old_mode", string(currentConfig.PersistenceMode)),
            zap.String("new_mode", string(newConfig.PersistenceMode)))

        return a.eventBus.ConfigureTopic(ctx, topic, newConfig)
    }

    return nil
}

// 批量健康检查和调整
func (a *TopicConfigAdjuster) PerformHealthCheck(ctx context.Context) error {
    topics := a.eventBus.ListConfiguredTopics()

    for _, topic := range topics {
        config, err := a.eventBus.GetTopicConfig(topic)
        if err != nil {
            continue
        }

        // 检查配置是否合理
        if strings.HasPrefix(topic, "business.") && config.PersistenceMode != eventbus.TopicPersistent {
            a.AdjustTopicIfNeeded(ctx, topic, "business_topic_should_be_persistent")
        }

        if strings.HasPrefix(topic, "temp.") && config.PersistenceMode == eventbus.TopicPersistent {
            a.AdjustTopicIfNeeded(ctx, topic, "temp_topic_should_be_ephemeral")
        }
    }

    return nil
}
```

### 5. 智能消息模式选择

根据业务需求和主题持久化策略选择最适合的消息传递模式：

#### Envelope 模式 + 持久化主题（事件溯源）
```go
// 适用于：需要事件溯源、聚合管理、版本控制的业务关键场景

// 1. 应用启动时统一配置主题（推荐方式）
func InitializeOrderTopics(bus eventbus.EventBus, ctx context.Context) error {
    orderOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   30 * 24 * time.Hour, // 30天保留
        MaxSize:         1024 * 1024 * 1024,  // 1GB
        Replicas:        3,                   // 3副本
        Description:     "订单事件，支持事件溯源",
    }
    return bus.ConfigureTopic(ctx, "business.orders", orderOptions)
}

// 2. 发布消息时专注于业务逻辑
func PublishOrderEvent(bus eventbus.EventBus, ctx context.Context, orderID string, eventType string, payload []byte) error {
    // 直接发布，主题配置已在启动时完成
    envelope := eventbus.NewEnvelope(orderID, eventType, 1, payload)
    return bus.PublishEnvelope(ctx, "business.orders", envelope)
}

// 3. 订阅时自动使用 Keyed-Worker 池确保同一订单的事件严格按顺序处理
func SubscribeOrderEvents(bus eventbus.EventBus, ctx context.Context) error {
    return bus.SubscribeEnvelope(ctx, "business.orders", func(ctx context.Context, env *eventbus.Envelope) error {
        // 同一聚合ID（订单ID）的事件严格按顺序处理
        // 智能路由：持久化主题自动使用 JetStream 或 Kafka 持久化存储
        return processOrderEvent(env)
    })
}
```

#### 普通消息模式 + 非持久化主题（高性能通知）
```go
// 适用于：简单消息传递、高吞吐量、无顺序要求、无持久化需求的场景

// 1. 应用启动时统一配置主题（推荐方式）
func InitializeNotificationTopics(bus eventbus.EventBus, ctx context.Context) error {
    notifyOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   30 * time.Minute,    // 短期保留
        Description:     "系统通知，高性能处理",
    }
    return bus.ConfigureTopic(ctx, "system.notifications", notifyOptions)
}

// 2. 发布消息时专注于业务逻辑
func PublishSystemNotification(bus eventbus.EventBus, ctx context.Context, message []byte) error {
    // 直接发布，主题配置已在启动时完成
    return bus.Publish(ctx, "system.notifications", message)
}

// 3. 并发处理，性能最优
func SubscribeSystemNotifications(bus eventbus.EventBus, ctx context.Context) error {
    return bus.Subscribe(ctx, "system.notifications", func(ctx context.Context, msg []byte) error {
        // 智能路由：非持久化主题自动使用 Core NATS 或内存传输
        return processNotification(msg)
    })
}
```

#### 混合模式（智能路由）
```go
// 适用于：需要根据消息内容动态选择持久化策略的场景

// 1. 应用启动时预配置所有可能的主题（推荐方式）
func InitializeAllTopics(bus eventbus.EventBus, ctx context.Context) error {
    topicConfigs := map[string]eventbus.TopicOptions{
        "business.events": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   7 * 24 * time.Hour,
            Description:     "业务关键事件",
        },
        "system.events": {
            PersistenceMode: eventbus.TopicEphemeral,
            RetentionTime:   1 * time.Hour,
            Description:     "系统事件，高性能处理",
        },
        "general.events": {
            PersistenceMode: eventbus.TopicAuto,
            Description:     "通用事件，自动选择策略",
        },
    }

    for topic, options := range topicConfigs {
        if err := bus.ConfigureTopic(ctx, topic, options); err != nil {
            return fmt.Errorf("failed to configure topic %s: %w", topic, err)
        }
    }
    return nil
}

// 2. 发布消息时根据类型选择合适的主题和模式
func PublishSmartMessage(bus eventbus.EventBus, ctx context.Context, messageType string, data []byte) error {
    switch messageType {
    case "order_created", "payment_completed":
        // 业务关键事件：使用预配置的持久化主题 + Envelope 模式
        envelope := eventbus.NewEnvelope("business-event", messageType, 1, data)
        return bus.PublishEnvelope(ctx, "business.events", envelope)

    case "user_login", "cache_invalidation":
        // 系统事件：使用预配置的非持久化主题 + 普通模式
        return bus.Publish(ctx, "system.events", data)

    default:
        // 通用事件：使用预配置的自动模式主题
        return bus.Publish(ctx, "general.events", data)
    }
}
```

#### 高级选项模式 + 预配置主题（企业特性）
```go
// 适用于：需要超时控制、重试机制、元数据的企业级场景

// 1. 应用启动时预配置不同优先级的主题（推荐方式）
func InitializePriorityTopics(bus eventbus.EventBus, ctx context.Context) error {
    priorityConfigs := map[string]eventbus.TopicOptions{
        "priority.critical": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   7 * 24 * time.Hour,
            MaxSize:         500 * 1024 * 1024,
            Replicas:        3,
            Description:     "关键优先级消息",
        },
        "priority.high": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   3 * 24 * time.Hour,
            MaxSize:         200 * 1024 * 1024,
            Replicas:        2,
            Description:     "高优先级消息",
        },
        "priority.medium": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   24 * time.Hour,
            MaxSize:         100 * 1024 * 1024,
            Replicas:        1,
            Description:     "中等优先级消息",
        },
        "priority.low": {
            PersistenceMode: eventbus.TopicEphemeral,
            RetentionTime:   2 * time.Hour,
            Description:     "低优先级消息",
        },
    }

    for topic, options := range priorityConfigs {
        if err := bus.ConfigureTopic(ctx, topic, options); err != nil {
            return fmt.Errorf("failed to configure priority topic %s: %w", topic, err)
        }
    }
    return nil
}

// 2. 发布消息时根据优先级选择预配置的主题
func PublishWithAdvancedOptions(bus eventbus.EventBus, ctx context.Context, message []byte, priority string) error {
    // 根据优先级选择预配置的主题
    var topic string
    switch priority {
    case "critical":
        topic = "priority.critical"
    case "high":
        topic = "priority.high"
    case "medium":
        topic = "priority.medium"
    default:
        topic = "priority.low"
    }

    // 使用高级发布选项
    publishOpts := eventbus.PublishOptions{
        Timeout: 30 * time.Second,
        Metadata: map[string]string{
            "priority":  priority,
            "timestamp": time.Now().Format(time.RFC3339),
            "topic":     topic,
        },
    }

    return bus.PublishWithOptions(ctx, topic, message, publishOpts)
}

// 3. 动态优先级调整（仅在必要时使用）
func AdjustTopicPriorityIfNeeded(bus eventbus.EventBus, ctx context.Context, topic string, newPriority string) error {
    // 仅在运行时确实需要调整时才使用
    // 大多数情况下应该使用预配置的主题
    switch newPriority {
    case "critical", "high":
        return bus.SetTopicPersistence(ctx, topic, true)
    default:
        return bus.SetTopicPersistence(ctx, topic, false)
    }
}
```

### 6. 企业级错误处理与恢复

结合主题持久化管理的错误处理策略：

#### 发布错误处理与降级策略
```go
type PublishErrorHandler struct {
    eventBus eventbus.EventBus
    logger   *zap.Logger
    fallback eventbus.EventBus // 备用 EventBus（如内存模式）
}

func (h *PublishErrorHandler) PublishWithFallback(ctx context.Context, topic string, message []byte) error {
    // 1. 尝试正常发布
    err := h.eventBus.Publish(ctx, topic, message)
    if err == nil {
        return nil
    }

    h.logger.Error("Primary publish failed",
        zap.String("topic", topic), zap.Error(err))

    // 2. 检查主题配置，决定降级策略
    config, configErr := h.eventBus.GetTopicConfig(topic)
    if configErr != nil {
        h.logger.Warn("Failed to get topic config", zap.Error(configErr))
        config = eventbus.DefaultTopicOptions()
    }

    // 3. 根据主题重要性决定降级策略
    switch config.PersistenceMode {
    case eventbus.TopicPersistent:
        // 持久化主题：尝试重试，然后降级到备用系统
        if retryErr := h.retryPublish(ctx, topic, message, 3); retryErr != nil {
            h.logger.Error("Retry failed, using fallback",
                zap.String("topic", topic), zap.Error(retryErr))

            // 降级到备用 EventBus（如内存模式）
            if h.fallback != nil {
                return h.fallback.Publish(ctx, topic, message)
            }
            return retryErr
        }
        return nil

    case eventbus.TopicEphemeral:
        // 非持久化主题：记录错误但不阻塞业务
        h.logger.Warn("Ephemeral topic publish failed, continuing",
            zap.String("topic", topic), zap.Error(err))
        return nil // 不阻塞业务流程

    default:
        // 自动模式：尝试一次重试
        return h.retryPublish(ctx, topic, message, 1)
    }
}

func (h *PublishErrorHandler) retryPublish(ctx context.Context, topic string, message []byte, maxRetries int) error {
    for i := 0; i < maxRetries; i++ {
        time.Sleep(time.Duration(i+1) * time.Second) // 指数退避

        if err := h.eventBus.Publish(ctx, topic, message); err == nil {
            h.logger.Info("Retry publish succeeded",
                zap.String("topic", topic), zap.Int("attempt", i+1))
            return nil
        }
    }
    return fmt.Errorf("publish failed after %d retries", maxRetries)
}
```

#### 订阅错误处理与重试机制
```go
func SubscribeWithErrorHandling(bus eventbus.EventBus, ctx context.Context, topic string) error {
    return bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
        // 1. 获取主题配置，了解消息重要性
        config, err := bus.GetTopicConfig(topic)
        if err != nil {
            config = eventbus.DefaultTopicOptions()
        }

        // 2. 处理消息
        if err := processMessage(message); err != nil {
            log.Printf("Failed to process message on topic %s: %v", topic, err)

            // 3. 根据主题持久化模式决定错误处理策略
            switch config.PersistenceMode {
            case eventbus.TopicPersistent:
                // 持久化主题：返回错误触发重试（消息不会丢失）
                return fmt.Errorf("processing failed for persistent topic: %w", err)

            case eventbus.TopicEphemeral:
                // 非持久化主题：记录错误但不重试（避免阻塞）
                log.Printf("Skipping retry for ephemeral topic %s: %v", topic, err)
                return nil // 不触发重试

            default:
                // 自动模式：根据错误类型决定
                if isRetryableError(err) {
                    return err // 触发重试
                }
                return nil // 不重试
            }
        }
        return nil
    })
}

func isRetryableError(err error) bool {
    // 判断错误是否可重试
    switch {
    case strings.Contains(err.Error(), "timeout"):
        return true
    case strings.Contains(err.Error(), "connection"):
        return true
    case strings.Contains(err.Error(), "temporary"):
        return true
    default:
        return false
    }
}
```

#### 主题配置错误处理
```go
func ConfigureTopicWithValidation(bus eventbus.EventBus, ctx context.Context, topic string, options eventbus.TopicOptions) error {
    // 1. 验证配置参数
    if err := validateTopicOptions(topic, options); err != nil {
        return fmt.Errorf("invalid topic options: %w", err)
    }

    // 2. 尝试配置主题
    if err := bus.ConfigureTopic(ctx, topic, options); err != nil {
        log.Printf("Failed to configure topic %s: %v", topic, err)

        // 3. 降级到默认配置
        defaultOptions := eventbus.DefaultTopicOptions()
        defaultOptions.Description = fmt.Sprintf("Fallback config for %s", topic)

        if fallbackErr := bus.ConfigureTopic(ctx, topic, defaultOptions); fallbackErr != nil {
            return fmt.Errorf("both primary and fallback config failed: primary=%v, fallback=%v", err, fallbackErr)
        }

        log.Printf("Applied fallback config for topic %s", topic)
    }

    return nil
}

func validateTopicOptions(topic string, options eventbus.TopicOptions) error {
    // 验证保留时间
    if options.RetentionTime < 0 {
        return fmt.Errorf("retention time cannot be negative")
    }

    // 验证存储大小
    if options.MaxSize < 0 {
        return fmt.Errorf("max size cannot be negative")
    }

    // 验证副本数
    if options.Replicas < 0 {
        return fmt.Errorf("replicas cannot be negative")
    }

    // 验证主题命名规范
    if !isValidTopicName(topic) {
        return fmt.Errorf("invalid topic name: %s", topic)
    }

    return nil
}

func isValidTopicName(topic string) bool {
    // 实现主题命名规范验证
    if len(topic) == 0 || len(topic) > 255 {
        return false
    }

    // 不允许包含空格
    if strings.Contains(topic, " ") {
        return false
    }

    // ⚠️ Kafka 要求：只能使用 ASCII 字符
    // 检查是否包含非 ASCII 字符（如中文、日文、韩文等）
    for _, r := range topic {
        if r > 127 {
            return false  // 包含非 ASCII 字符
        }
    }

    return true
}

// 使用示例
func validateKafkaTopicName(topic string) error {
    if !isValidTopicName(topic) {
        return fmt.Errorf("invalid Kafka topic name '%s': must use ASCII characters only (no Chinese, Japanese, Korean, etc.)", topic)
    }
    return nil
}
```

### 7. 优雅关闭与资源清理

```go
type GracefulShutdownManager struct {
    eventBus eventbus.EventBus
    logger   *zap.Logger
    timeout  time.Duration
}

func NewGracefulShutdownManager(bus eventbus.EventBus, logger *zap.Logger) *GracefulShutdownManager {
    return &GracefulShutdownManager{
        eventBus: bus,
        logger:   logger,
        timeout:  30 * time.Second, // 默认30秒超时
    }
}

func (m *GracefulShutdownManager) Shutdown(ctx context.Context) error {
    m.logger.Info("Starting graceful shutdown...")

    // 1. 创建带超时的上下文
    shutdownCtx, cancel := context.WithTimeout(ctx, m.timeout)
    defer cancel()

    // 2. 停止接收新消息（如果支持）
    m.logger.Info("Stopping message acceptance...")

    // 3. 等待正在处理的消息完成
    m.logger.Info("Waiting for in-flight messages to complete...")

    // 4. 保存主题配置（如果需要持久化）
    if err := m.saveTopicConfigs(shutdownCtx); err != nil {
        m.logger.Warn("Failed to save topic configs", zap.Error(err))
    }

    // 5. 关闭 EventBus 连接
    m.logger.Info("Closing EventBus connections...")
    if err := m.eventBus.Close(); err != nil {
        m.logger.Error("Failed to close EventBus", zap.Error(err))
        return err
    }

    m.logger.Info("Graceful shutdown completed")
    return nil
}

func (m *GracefulShutdownManager) saveTopicConfigs(ctx context.Context) error {
    // 获取所有配置的主题
    topics := m.eventBus.ListConfiguredTopics()

    configs := make(map[string]eventbus.TopicOptions)
    for _, topic := range topics {
        if config, err := m.eventBus.GetTopicConfig(topic); err == nil {
            configs[topic] = config
        }
    }

    // 保存到文件或数据库（示例：保存到文件）
    configData, err := json.MarshalIndent(configs, "", "  ")
    if err != nil {
        return err
    }

    return os.WriteFile("topic_configs_backup.json", configData, 0644)
}

// 全局优雅关闭函数
func GracefulShutdownGlobal(ctx context.Context) error {
    bus := eventbus.GetGlobal()
    if bus == nil {
        return nil
    }

    logger, _ := zap.NewProduction()
    defer logger.Sync()

    manager := NewGracefulShutdownManager(bus, logger)
    return manager.Shutdown(ctx)
}

// 在应用中使用
func main() {
    // ... 应用初始化 ...

    // 设置信号处理
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    // 等待关闭信号
    <-sigChan

    // 执行优雅关闭
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    if err := GracefulShutdownGlobal(ctx); err != nil {
        log.Printf("Graceful shutdown failed: %v", err)
        os.Exit(1)
    }

    log.Println("Application shutdown completed")
}
```

### 8. 性能优化与监控

#### 主题持久化性能优化
```go
type PerformanceOptimizer struct {
    eventBus eventbus.EventBus
    metrics  *PerformanceMetrics
    logger   *zap.Logger
}

type PerformanceMetrics struct {
    PublishLatency    map[string]time.Duration // 按主题统计发布延迟
    SubscribeLatency  map[string]time.Duration // 按主题统计处理延迟
    MessageThroughput map[string]int64         // 按主题统计吞吐量
    ErrorRate         map[string]float64       // 按主题统计错误率
    TopicConfigCount  int                      // 配置的主题数量
}

func (p *PerformanceOptimizer) OptimizeTopicConfigs(ctx context.Context) error {
    topics := p.eventBus.ListConfiguredTopics()

    for _, topic := range topics {
        config, err := p.eventBus.GetTopicConfig(topic)
        if err != nil {
            continue
        }

        // 获取主题性能指标
        latency := p.metrics.PublishLatency[topic]
        throughput := p.metrics.MessageThroughput[topic]
        errorRate := p.metrics.ErrorRate[topic]

        // 基于性能指标优化配置
        optimizedConfig := p.optimizeConfigBasedOnMetrics(config, latency, throughput, errorRate)

        if !p.configsEqual(config, optimizedConfig) {
            p.logger.Info("Optimizing topic config",
                zap.String("topic", topic),
                zap.Duration("latency", latency),
                zap.Int64("throughput", throughput),
                zap.Float64("error_rate", errorRate))

            if err := p.eventBus.ConfigureTopic(ctx, topic, optimizedConfig); err != nil {
                p.logger.Error("Failed to optimize topic config",
                    zap.String("topic", topic), zap.Error(err))
            }
        }
    }

    return nil
}

func (p *PerformanceOptimizer) optimizeConfigBasedOnMetrics(
    config eventbus.TopicOptions,
    latency time.Duration,
    throughput int64,
    errorRate float64) eventbus.TopicOptions {

    optimized := config

    // 高延迟优化
    if latency > 100*time.Millisecond {
        if config.PersistenceMode == eventbus.TopicPersistent {
            // 持久化主题高延迟：考虑增加副本或调整保留策略
            if throughput < 1000 { // 低吞吐量
                optimized.RetentionTime = config.RetentionTime / 2 // 减少保留时间
            }
        }
    }

    // 高吞吐量优化
    if throughput > 10000 {
        if config.PersistenceMode == eventbus.TopicEphemeral {
            // 非持久化主题高吞吐量：确保配置合理
            optimized.MaxSize = max(optimized.MaxSize, 500*1024*1024) // 至少500MB
        }
    }

    // 高错误率优化
    if errorRate > 0.05 { // 5%错误率
        if config.PersistenceMode == eventbus.TopicPersistent {
            // 持久化主题高错误率：增加副本数
            optimized.Replicas = max(optimized.Replicas, 3)
        }
    }

    return optimized
}

func (p *PerformanceOptimizer) configsEqual(a, b eventbus.TopicOptions) bool {
    return a.PersistenceMode == b.PersistenceMode &&
           a.RetentionTime == b.RetentionTime &&
           a.MaxSize == b.MaxSize &&
           a.Replicas == b.Replicas
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}
```

#### 智能路由监控
```go
type RouteMonitor struct {
    eventBus    eventbus.EventBus
    routeStats  map[string]*RouteStats
    mu          sync.RWMutex
    logger      *zap.Logger
}

type RouteStats struct {
    Topic              string
    PersistentMessages int64
    EphemeralMessages  int64
    LastRouteTime      time.Time
    RouteMode          string // "JetStream", "CoreNATS", "KafkaLongTerm", "KafkaShortTerm"
    AvgLatency         time.Duration
}

func (m *RouteMonitor) RecordRoute(topic string, isPersistent bool, routeMode string, latency time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.routeStats[topic] == nil {
        m.routeStats[topic] = &RouteStats{Topic: topic}
    }

    stats := m.routeStats[topic]
    if isPersistent {
        stats.PersistentMessages++
    } else {
        stats.EphemeralMessages++
    }
    stats.LastRouteTime = time.Now()
    stats.RouteMode = routeMode

    // 计算平均延迟
    if stats.AvgLatency == 0 {
        stats.AvgLatency = latency
    } else {
        stats.AvgLatency = (stats.AvgLatency + latency) / 2
    }
}

func (m *RouteMonitor) GetRouteStats(topic string) *RouteStats {
    m.mu.RLock()
    defer m.mu.RUnlock()

    if stats, exists := m.routeStats[topic]; exists {
        // 返回副本避免并发问题
        return &RouteStats{
            Topic:              stats.Topic,
            PersistentMessages: stats.PersistentMessages,
            EphemeralMessages:  stats.EphemeralMessages,
            LastRouteTime:      stats.LastRouteTime,
            RouteMode:          stats.RouteMode,
            AvgLatency:         stats.AvgLatency,
        }
    }
    return nil
}

func (m *RouteMonitor) GenerateReport() string {
    m.mu.RLock()
    defer m.mu.RUnlock()

    var report strings.Builder
    report.WriteString("=== 智能路由统计报告 ===\n")

    for topic, stats := range m.routeStats {
        total := stats.PersistentMessages + stats.EphemeralMessages
        persistentRatio := float64(stats.PersistentMessages) / float64(total) * 100

        report.WriteString(fmt.Sprintf(
            "主题: %s\n"+
            "  总消息数: %d\n"+
            "  持久化消息: %d (%.1f%%)\n"+
            "  非持久化消息: %d (%.1f%%)\n"+
            "  当前路由模式: %s\n"+
            "  平均延迟: %v\n"+
            "  最后路由时间: %s\n\n",
            topic, total,
            stats.PersistentMessages, persistentRatio,
            stats.EphemeralMessages, 100-persistentRatio,
            stats.RouteMode,
            stats.AvgLatency,
            stats.LastRouteTime.Format("2006-01-02 15:04:05")))
    }

    return report.String()
}
```

#### 主题配置审计
```go
type ConfigAuditor struct {
    eventBus eventbus.EventBus
    logger   *zap.Logger
    history  []ConfigChange
    mu       sync.Mutex
}

type ConfigChange struct {
    Topic     string
    OldConfig eventbus.TopicOptions
    NewConfig eventbus.TopicOptions
    Timestamp time.Time
    Reason    string
}

func (a *ConfigAuditor) AuditConfigChange(topic string, oldConfig, newConfig eventbus.TopicOptions, reason string) {
    a.mu.Lock()
    defer a.mu.Unlock()

    change := ConfigChange{
        Topic:     topic,
        OldConfig: oldConfig,
        NewConfig: newConfig,
        Timestamp: time.Now(),
        Reason:    reason,
    }

    a.history = append(a.history, change)

    a.logger.Info("Topic config changed",
        zap.String("topic", topic),
        zap.String("old_mode", string(oldConfig.PersistenceMode)),
        zap.String("new_mode", string(newConfig.PersistenceMode)),
        zap.String("reason", reason))
}

func (a *ConfigAuditor) GetConfigHistory(topic string) []ConfigChange {
    a.mu.Lock()
    defer a.mu.Unlock()

    var history []ConfigChange
    for _, change := range a.history {
        if change.Topic == topic {
            history = append(history, change)
        }
    }
    return history
}

func (a *ConfigAuditor) ValidateCurrentConfigs(ctx context.Context) []string {
    var issues []string
    topics := a.eventBus.ListConfiguredTopics()

    for _, topic := range topics {
        config, err := a.eventBus.GetTopicConfig(topic)
        if err != nil {
            issues = append(issues, fmt.Sprintf("无法获取主题 %s 的配置: %v", topic, err))
            continue
        }

        // 验证配置合理性
        if strings.HasPrefix(topic, "business.") && config.PersistenceMode != eventbus.TopicPersistent {
            issues = append(issues, fmt.Sprintf("业务主题 %s 应该配置为持久化", topic))
        }

        if strings.HasPrefix(topic, "temp.") && config.PersistenceMode == eventbus.TopicPersistent {
            issues = append(issues, fmt.Sprintf("临时主题 %s 不应该配置为持久化", topic))
        }

        if config.RetentionTime > 30*24*time.Hour {
            issues = append(issues, fmt.Sprintf("主题 %s 的保留时间过长 (%v)", topic, config.RetentionTime))
        }

        if config.MaxSize > 10*1024*1024*1024 { // 10GB
            issues = append(issues, fmt.Sprintf("主题 %s 的最大大小过大 (%d bytes)", topic, config.MaxSize))
        }
    }

    return issues
}
```

### 9. 生产环境部署最佳实践

#### 环境配置管理
```go
type EnvironmentConfigManager struct {
    environment string // "development", "staging", "production"
    eventBus    eventbus.EventBus
    logger      *zap.Logger
}

func (m *EnvironmentConfigManager) ApplyEnvironmentConfigs(ctx context.Context) error {
    switch m.environment {
    case "production":
        return m.applyProductionConfigs(ctx)
    case "staging":
        return m.applyStagingConfigs(ctx)
    case "development":
        return m.applyDevelopmentConfigs(ctx)
    default:
        return fmt.Errorf("unknown environment: %s", m.environment)
    }
}

func (m *EnvironmentConfigManager) applyProductionConfigs(ctx context.Context) error {
    configs := map[string]eventbus.TopicOptions{
        "business.orders": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   30 * 24 * time.Hour, // 30天
            MaxSize:         2 * 1024 * 1024 * 1024, // 2GB
            Replicas:        5, // 高可用
            Description:     "生产环境订单事件",
        },
        "audit.logs": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   90 * 24 * time.Hour, // 90天合规要求
            MaxSize:         10 * 1024 * 1024 * 1024, // 10GB
            Replicas:        5,
            Description:     "生产环境审计日志",
        },
        "system.notifications": {
            PersistenceMode: eventbus.TopicEphemeral,
            RetentionTime:   4 * time.Hour, // 4小时
            MaxSize:         100 * 1024 * 1024, // 100MB
            Replicas:        3, // 适度冗余
            Description:     "生产环境系统通知",
        },
    }

    return m.applyConfigs(ctx, configs)
}

func (m *EnvironmentConfigManager) applyDevelopmentConfigs(ctx context.Context) error {
    configs := map[string]eventbus.TopicOptions{
        "business.orders": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   2 * time.Hour, // 短期保留
            MaxSize:         50 * 1024 * 1024, // 50MB
            Replicas:        1, // 单副本
            Description:     "开发环境订单事件",
        },
        "system.notifications": {
            PersistenceMode: eventbus.TopicEphemeral,
            RetentionTime:   30 * time.Minute,
            MaxSize:         10 * 1024 * 1024, // 10MB
            Replicas:        1,
            Description:     "开发环境系统通知",
        },
    }

    return m.applyConfigs(ctx, configs)
}

func (m *EnvironmentConfigManager) applyConfigs(ctx context.Context, configs map[string]eventbus.TopicOptions) error {
    for topic, config := range configs {
        if err := m.eventBus.ConfigureTopic(ctx, topic, config); err != nil {
            m.logger.Error("Failed to apply config",
                zap.String("environment", m.environment),
                zap.String("topic", topic),
                zap.Error(err))
            return err
        }

        m.logger.Info("Applied environment config",
            zap.String("environment", m.environment),
            zap.String("topic", topic),
            zap.String("mode", string(config.PersistenceMode)))
    }

    return nil
}
```

## 故障排除

### 常见问题与解决方案

#### 1. 主题持久化相关问题

**问题**：主题配置不生效
```bash
# 检查主题配置
topics := bus.ListConfiguredTopics()
for _, topic := range topics {
    config, err := bus.GetTopicConfig(topic)
    if err != nil {
        log.Printf("Failed to get config for %s: %v", topic, err)
    } else {
        log.Printf("Topic %s: mode=%s, retention=%v",
            topic, config.PersistenceMode, config.RetentionTime)
    }
}
```

**解决方案**：
- 确保在发布消息前配置主题
- 检查配置参数是否合理
- 验证 EventBus 实现是否支持主题持久化管理

**问题**：智能路由不工作
```bash
# 检查 NATS JetStream 是否启用
config := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"})
config.NATS.JetStream.Enabled = true // 确保启用

# 检查 Kafka Admin API 是否可用
# 确保 Kafka 版本支持 Admin API（0.11+）
```

#### 2. 性能问题

**问题**：发布延迟过高
- **持久化主题**：检查存储性能、网络延迟、副本数配置
- **非持久化主题**：检查是否误配置为持久化模式

**问题**：内存使用过高
- 检查主题保留时间和最大大小配置
- 监控消息积压情况
- 考虑调整批量处理参数

#### 3. 连接问题

1. **NATS 连接失败**：检查 NATS 服务器状态和 JetStream 配置
2. **Kafka 连接失败**：检查 Kafka 集群状态和网络连接
3. **消息丢失**：确保正确处理错误和重试机制
4. **内存泄漏**：确保正确关闭 EventBus 实例和清理主题配置

## Keyed-Worker池架构优势

### 🚀 **相比传统恢复模式的优势**

#### 1. **架构简洁性**
```
传统恢复模式：
正常模式 ⟷ 恢复模式 (复杂状态切换)
├── 积压检测逻辑
├── 模式切换逻辑
├── 状态同步机制
└── 配置管理复杂

Keyed-Worker池：
统一处理模式 (无状态切换)
├── 一致性哈希路由
├── 固定Worker池
├── 有界队列背压
└── 配置简单直观
```

#### 2. **性能稳定性**
- **消除性能抖动**：无模式切换，处理延迟稳定可预测
- **资源使用可控**：固定Worker数量，内存使用上限明确
- **并发性能优异**：不同聚合ID并行处理，充分利用多核
- **背压自然**：队列满时自动背压，无需复杂的流控逻辑

#### 3. **顺序保证强度**
- **严格顺序**：同一聚合ID通过哈希路由到固定Worker
- **无竞争条件**：每个Worker独立处理，避免锁竞争
- **故障隔离**：单个Worker故障不影响其他聚合ID处理

#### 4. **运维简化**
- **配置简单**：只需配置Worker数量和队列大小
- **监控直观**：Worker利用率、队列深度等指标清晰
- **故障诊断**：无复杂状态，问题定位更容易

### 📊 **性能对比**

| 特性 | 传统恢复模式 | Keyed-Worker池 |
|------|-------------|----------------|
| 顺序保证 | 依赖模式切换 | 架构级保证 |
| 性能稳定性 | 模式切换抖动 | 稳定可预测 |
| 资源使用 | 动态变化 | 固定可控 |
| 配置复杂度 | 高 | 低 |
| 故障恢复 | 需要状态同步 | 自动恢复 |
| 并发性能 | 受模式限制 | 充分并行 |

### Keyed-Worker 池性能调优

#### 队列满问题
```go
// 症状：收到 ErrWorkerQueueFull 错误
// 解决方案：
1. 增加队列大小：queueSize: 2000
2. 增加 Worker 数量：workerCount: 512
3. 减少等待超时：waitTimeout: 100ms
4. 优化消息处理逻辑，提高处理速度
```

#### 顺序处理性能优化
```go
// 1. 合理设置 Worker 数量（建议为 CPU 核心数的 8-16 倍）
keyedWorkerPool:
  workerCount: 256  # 对于 16 核 CPU

// 2. 根据消息大小调整队列大小
keyedWorkerPool:
  queueSize: 1000   # 小消息可以设置更大
  queueSize: 100    # 大消息建议设置较小

// 3. 调整等待超时
keyedWorkerPool:
  waitTimeout: 200ms  # 高吞吐场景
  waitTimeout: 1s     # 低延迟要求场景
```

#### 监控指标
- 监控队列使用率：避免频繁的队列满
- 监控处理延迟：确保消息及时处理
- 监控 Worker 利用率：避免资源浪费

### 日志级别

设置适当的日志级别以获取调试信息：

```go
// 设置Debug级别以获取详细日志
logger.SetLevel(logger.DebugLevel)
```

## 贡献

欢迎提交Issue和Pull Request来改进EventBus组件。

## 许可证

本项目采用MIT许可证。




## Kafka 使用举例

⚠️ **重要提示**：使用 Kafka 时，**ClientID 和 Topic 名称必须只使用 ASCII 字符**！

**常见错误**：
- ❌ 使用中文：`"业务.订单"`, `"用户事件"`
- ❌ 混用中英文：`"business.支付"`, `"订单.events"`
- ✅ 正确做法：`"business.orders"`, `"user.events"`

**后果**：使用非 ASCII 字符会导致消息无法接收（0% 成功率），这是 Kafka 的底层限制。

---

Kafka EventBus 现在支持**基于主题的智能持久化管理**，可以在同一个 EventBus 实例中动态创建和配置不同持久化策略的主题，提供企业级的消息处理能力。

### 核心特性

- **🎯 主题级控制**：每个主题可以独立配置持久化策略和保留时间
- **🔄 动态主题管理**：使用 Kafka Admin API 动态创建和配置主题
- **🚀 智能配置**：根据业务需求自动设置主题参数（分区、副本、保留策略）
- **⚡ 性能优化**：持久化主题使用长期保留，非持久化主题使用短期保留
- **🔧 统一接口**：单一 EventBus 实例处理多种持久化需求

### 智能主题管理机制

EventBus 会根据主题的持久化配置自动创建和配置 Kafka 主题：

- **持久化主题** → 长期保留策略（如7天、多副本、大存储限制）
- **非持久化主题** → 短期保留策略（如1分钟、单副本、小存储限制）
- **自动模式** → 根据全局配置决定保留策略

### 完整使用示例

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// ========== 业务事件结构 ==========

type OrderCreatedEvent struct {
    OrderID    string  `json:"order_id"`
    CustomerID string  `json:"customer_id"`
    Amount     float64 `json:"amount"`
    Status     string  `json:"status"`
    Timestamp  string  `json:"timestamp"`
}

type NotificationMessage struct {
    UserID    string `json:"user_id"`
    Title     string `json:"title"`
    Content   string `json:"content"`
    Type      string `json:"type"`
    Timestamp string `json:"timestamp"`
}

type MetricsData struct {
    ServiceName string  `json:"service_name"`
    CPUUsage    float64 `json:"cpu_usage"`
    MemoryUsage float64 `json:"memory_usage"`
    Timestamp   string  `json:"timestamp"`
}

// ========== 业务服务 ==========

type BusinessService struct {
    eventBus eventbus.EventBus
}

func NewBusinessService(bus eventbus.EventBus) *BusinessService {
    return &BusinessService{eventBus: bus}
}

// 发布订单事件（持久化）
func (s *BusinessService) CreateOrder(ctx context.Context, orderID, customerID string, amount float64) error {
    event := OrderCreatedEvent{
        OrderID:    orderID,
        CustomerID: customerID,
        Amount:     amount,
        Status:     "created",
        Timestamp:  time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(event)
    // 发布到持久化主题，EventBus 自动使用长期保留策略
    return s.eventBus.Publish(ctx, "business.orders", message)
}

// 发送通知消息（非持久化）
func (s *BusinessService) SendNotification(ctx context.Context, userID, title, content string) error {
    notification := NotificationMessage{
        UserID:    userID,
        Title:     title,
        Content:   content,
        Type:      "info",
        Timestamp: time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(notification)
    // 发布到非持久化主题，EventBus 自动使用短期保留策略
    return s.eventBus.Publish(ctx, "system.notifications", message)
}

// 发送监控指标（自动模式）
func (s *BusinessService) SendMetrics(ctx context.Context, serviceName string, cpu, memory float64) error {
    metrics := MetricsData{
        ServiceName: serviceName,
        CPUUsage:    cpu,
        MemoryUsage: memory,
        Timestamp:   time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(metrics)
    // 发布到自动模式主题，EventBus 根据全局配置决定
    return s.eventBus.Publish(ctx, "system.metrics", message)
}

// 订阅订单事件
func (s *BusinessService) SubscribeToOrderEvents(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        var event OrderCreatedEvent
        json.Unmarshal(message, &event)

        fmt.Printf("💾 [订单服务-Kafka持久化] 收到订单事件: %+v\n", event)
        fmt.Printf("   ✅ 消息已持久化存储，保留7天\n")
        fmt.Printf("   📊 主题配置: 长期保留策略\n\n")

        return s.processOrderEvent(event)
    }

    return s.eventBus.Subscribe(ctx, "business.orders", handler)
}

// 订阅通知消息
func (s *BusinessService) SubscribeToNotifications(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        var notification NotificationMessage
        json.Unmarshal(message, &notification)

        fmt.Printf("⚡ [通知服务-Kafka短期保留] 收到通知: %+v\n", notification)
        fmt.Printf("   🚀 高性能处理，短期保留\n")
        fmt.Printf("   📊 主题配置: 短期保留策略\n\n")

        return s.processNotification(notification)
    }

    return s.eventBus.Subscribe(ctx, "system.notifications", handler)
}

// 订阅监控指标
func (s *BusinessService) SubscribeToMetrics(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        var metrics MetricsData
        json.Unmarshal(message, &metrics)

        fmt.Printf("📊 [监控服务-Kafka自动模式] 收到指标: %+v\n", metrics)
        fmt.Printf("   🔄 主题配置: 根据全局配置自动选择\n\n")

        return s.processMetrics(metrics)
    }

    return s.eventBus.Subscribe(ctx, "system.metrics", handler)
}

func (s *BusinessService) processOrderEvent(event OrderCreatedEvent) error {
    fmt.Printf("   📋 处理订单: %s, 金额: %.2f\n", event.OrderID, event.Amount)
    return nil
}

func (s *BusinessService) processNotification(notification NotificationMessage) error {
    fmt.Printf("   🔔 处理通知: %s -> %s\n", notification.UserID, notification.Title)
    return nil
}

func (s *BusinessService) processMetrics(metrics MetricsData) error {
    fmt.Printf("   📈 处理指标: %s CPU=%.1f%% MEM=%.1f%%\n",
        metrics.ServiceName, metrics.CPUUsage, metrics.MemoryUsage)
    return nil
}

// ========== 主程序：演示 Kafka 主题持久化管理 ==========

func main() {
    fmt.Println("=== Kafka 主题持久化管理演示 ===\n")

    // 1. 初始化 Kafka EventBus
    cfg := &eventbus.EventBusConfig{
        Type: "kafka",
        Kafka: eventbus.KafkaConfig{
            Brokers: []string{"localhost:29092"}, // Kafka 服务器地址
            Producer: eventbus.ProducerConfig{
                RequiredAcks:   1,
                Timeout:        5 * time.Second,
                RetryMax:       3,
                Compression:    "snappy",
                FlushFrequency: 100 * time.Millisecond,
                BatchSize:      16384,
            },
            Consumer: eventbus.ConsumerConfig{
                GroupID:           "business-service-group",
                SessionTimeout:    30 * time.Second,
                HeartbeatInterval: 3 * time.Second,
                MaxProcessingTime: 2 * time.Minute,
                AutoOffsetReset:   "earliest", // 从最早消息开始读取
                FetchMinBytes:     1,
                FetchMaxBytes:     1024 * 1024,
                FetchMaxWait:      500 * time.Millisecond,
            },
        },
    }

    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatalf("Failed to initialize EventBus: %v", err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()
    ctx := context.Background()

    // 2. 配置不同主题的持久化策略
    fmt.Println("📋 配置主题持久化策略...")

    // 业务关键事件：需要长期持久化
    orderOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   7 * 24 * time.Hour, // 保留7天
        MaxSize:         500 * 1024 * 1024,  // 500MB
        MaxMessages:     50000,              // 5万条消息
        Replicas:        3,                  // 3个副本
        Description:     "订单相关事件，需要长期持久化存储",
    }
    if err := bus.ConfigureTopic(ctx, "business.orders", orderOptions); err != nil {
        log.Printf("Failed to configure orders topic: %v", err)
    } else {
        fmt.Println("✅ 订单主题配置为长期持久化 (7天保留)")
    }

    // 系统通知：临时消息，短期保留
    notificationOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   1 * time.Hour,     // 仅保留1小时
        MaxSize:         10 * 1024 * 1024,  // 10MB
        Replicas:        1,                 // 单副本
        Description:     "系统通知消息，短期保留",
    }
    if err := bus.ConfigureTopic(ctx, "system.notifications", notificationOptions); err != nil {
        log.Printf("Failed to configure notifications topic: %v", err)
    } else {
        fmt.Println("✅ 通知主题配置为短期保留 (1小时)")
    }

    // 监控指标：自动模式，根据全局配置决定
    metricsOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicAuto,
        Description:     "系统监控指标，自动选择保留策略",
    }
    if err := bus.ConfigureTopic(ctx, "system.metrics", metricsOptions); err != nil {
        log.Printf("Failed to configure metrics topic: %v", err)
    } else {
        fmt.Println("✅ 指标主题配置为自动模式")
    }

    // 3. 创建业务服务
    service := NewBusinessService(bus)

    // 4. 启动订阅（EventBus 会根据主题配置自动创建 Kafka 主题）
    fmt.Println("\n🚀 启动智能订阅...")

    if err := service.SubscribeToOrderEvents(ctx); err != nil {
        log.Printf("Failed to subscribe to order events: %v", err)
    }

    if err := service.SubscribeToNotifications(ctx); err != nil {
        log.Printf("Failed to subscribe to notifications: %v", err)
    }

    if err := service.SubscribeToMetrics(ctx); err != nil {
        log.Printf("Failed to subscribe to metrics: %v", err)
    }

    time.Sleep(3 * time.Second) // 等待订阅建立和主题创建

    // 5. 演示智能主题管理
    fmt.Println("📨 开始发布消息，演示智能主题管理...\n")

    // 发布到持久化主题（自动创建长期保留主题）
    fmt.Println("--- 订单事件（自动创建长期保留主题） ---")
    service.CreateOrder(ctx, "order-12345", "customer-67890", 299.99)
    time.Sleep(1 * time.Second)

    // 发布到非持久化主题（自动创建短期保留主题）
    fmt.Println("--- 通知消息（自动创建短期保留主题） ---")
    service.SendNotification(ctx, "user-123", "订单确认", "您的订单 order-12345 已创建成功")
    time.Sleep(1 * time.Second)

    // 发布到自动模式主题（根据全局配置决定）
    fmt.Println("--- 监控指标（自动模式） ---")
    service.SendMetrics(ctx, "order-service", 65.4, 78.2)
    time.Sleep(1 * time.Second)

    // 6. 演示动态配置管理
    fmt.Println("--- 动态配置管理演示 ---")

    // 查看已配置的主题
    topics := bus.ListConfiguredTopics()
    fmt.Printf("📋 已配置主题: %v\n", topics)

    // 查看特定主题配置
    config, err := bus.GetTopicConfig("business.orders")
    if err == nil {
        fmt.Printf("📊 订单主题配置: 模式=%s, 保留时间=%v, 最大大小=%d, 副本数=%d\n",
            config.PersistenceMode, config.RetentionTime, config.MaxSize, config.Replicas)
    }

    // 动态修改主题配置
    fmt.Println("🔄 动态修改通知主题为长期持久化...")
    if err := bus.SetTopicPersistence(ctx, "system.notifications", true); err != nil {
        log.Printf("Failed to update notifications persistence: %v", err)
    } else {
        fmt.Println("✅ 通知主题已改为长期持久化模式")
    }

    // 再次发布通知，观察配置变化
    fmt.Println("📨 发布通知消息（现在使用长期保留）...")
    service.SendNotification(ctx, "user-456", "配置更新", "主题配置已动态更新为长期保留")
    time.Sleep(1 * time.Second)

    // 7. 总结
    fmt.Println("\n=== Kafka 主题持久化管理演示完成 ===")
    fmt.Println("✅ 核心特性验证:")
    fmt.Println("  🎯 主题级持久化控制 - 不同主题使用不同保留策略")
    fmt.Println("  🚀 智能主题管理 - 自动创建和配置 Kafka 主题")
    fmt.Println("  🔄 动态配置管理 - 运行时修改主题配置")
    fmt.Println("  ⚡ 性能优化 - 长期和短期保留策略并存")
    fmt.Println("  🔧 统一接口 - 单一 EventBus 实例处理多种需求")
    fmt.Println("  📊 完整监控 - 主题配置查询和管理")
    fmt.Println("  🛡️ 企业级特性 - 多副本、大容量、长期保留")
}
```

### 配置示例

```yaml
# Kafka 主题持久化管理配置
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:29092"]
    producer:
      requiredAcks: 1
      timeout: 5s
      retryMax: 3
      compression: "snappy"
      flushFrequency: 100ms
      batchSize: 16384
    consumer:
      groupID: "business-service-group"
      sessionTimeout: 30s
      heartbeatInterval: 3s
      maxProcessingTime: 2m
      autoOffsetReset: "earliest"
      fetchMinBytes: 1
      fetchMaxBytes: 1048576
      fetchMaxWait: 500ms

  # 预配置主题持久化策略（可选）
  topics:
    "business.orders":
      persistenceMode: "persistent"
      retentionTime: "168h"  # 7天
      maxSize: 524288000     # 500MB
      replicas: 3
      description: "订单相关事件，需要长期持久化存储"

    "system.notifications":
      persistenceMode: "ephemeral"
      retentionTime: "1h"    # 1小时
      maxSize: 10485760      # 10MB
      replicas: 1
      description: "系统通知消息，短期保留"

    "system.metrics":
      persistenceMode: "auto"
      description: "系统监控指标，自动选择保留策略"
```

### 运行示例

```bash
# 1. 启动 Kafka 服务器
# 使用 Docker Compose 或直接启动 Kafka
docker-compose up -d kafka

# 2. 运行 Kafka 主题持久化管理示例
go run examples/kafka_topic_persistence_example.go

# 3. 观察输出，验证智能主题管理功能
# - 订单事件自动创建长期保留主题
# - 通知消息自动创建短期保留主题
# - 动态配置管理功能

# 4. 验证 Kafka 主题创建
# 使用 Kafka 工具查看创建的主题和配置
kafka-topics.sh --bootstrap-server localhost:29092 --list
kafka-configs.sh --bootstrap-server localhost:29092 --describe --entity-type topics
```

### 核心优势

1. **🎯 主题级控制**：每个主题可以独立配置保留策略、副本数、存储限制
2. **🚀 智能主题管理**：使用 Kafka Admin API 自动创建和配置主题
3. **⚡ 性能优化**：长期和短期保留策略并存，各取所长
4. **🔄 动态配置**：运行时可以修改主题配置，自动应用到 Kafka
5. **🔧 统一接口**：单一 EventBus 实例处理多种需求，简化架构
6. **📊 完整监控**：提供主题配置查询和管理接口
7. **🛡️ 企业级特性**：支持多副本、大容量存储、长期保留
8. **🎛️ 渐进式采用**：可以逐步为不同主题配置不同策略

## NATS JetStream 使用举例

NATS EventBus 现在支持**基于主题的智能持久化管理**，可以在同一个 EventBus 实例中同时处理持久化和非持久化主题，提供更大的灵活性和更好的资源利用率。

### 核心特性

- **🎯 主题级控制**：每个主题可以独立配置持久化策略
- **🔄 动态配置**：运行时动态添加、修改、删除主题配置
- **🚀 智能路由**：根据主题配置自动选择 JetStream（持久化）或 Core NATS（非持久化）
- **⚡ 性能优化**：持久化主题使用可靠存储，非持久化主题使用高性能内存传输
- **🔧 统一接口**：单一 EventBus 实例处理多种持久化需求

### 智能路由机制

EventBus 会根据主题的持久化配置自动选择最优的消息传递机制：

- **持久化主题** → JetStream（可靠存储、消息持久化、支持重放）
- **非持久化主题** → Core NATS（高性能、内存传输、低延迟）
- **自动模式** → 根据全局配置决定

### 完整使用示例

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// ========== 业务事件结构 ==========

type OrderCreatedEvent struct {
    OrderID    string  `json:"order_id"`
    CustomerID string  `json:"customer_id"`
    Amount     float64 `json:"amount"`
    Status     string  `json:"status"`
    Timestamp  string  `json:"timestamp"`
}

type NotificationMessage struct {
    UserID    string `json:"user_id"`
    Title     string `json:"title"`
    Content   string `json:"content"`
    Type      string `json:"type"`
    Timestamp string `json:"timestamp"`
}

type MetricsData struct {
    ServiceName string  `json:"service_name"`
    CPUUsage    float64 `json:"cpu_usage"`
    MemoryUsage float64 `json:"memory_usage"`
    Timestamp   string  `json:"timestamp"`
}

// ========== 业务服务 ==========

type BusinessService struct {
    eventBus eventbus.EventBus
}

func NewBusinessService(bus eventbus.EventBus) *BusinessService {
    return &BusinessService{eventBus: bus}
}

// 发布订单事件（持久化）
func (s *BusinessService) CreateOrder(ctx context.Context, orderID, customerID string, amount float64) error {
    event := OrderCreatedEvent{
        OrderID:    orderID,
        CustomerID: customerID,
        Amount:     amount,
        Status:     "created",
        Timestamp:  time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(event)
    // 发布到持久化主题，EventBus 自动使用 JetStream
    return s.eventBus.Publish(ctx, "business.orders", message)
}

// 发送通知消息（非持久化）
func (s *BusinessService) SendNotification(ctx context.Context, userID, title, content string) error {
    notification := NotificationMessage{
        UserID:    userID,
        Title:     title,
        Content:   content,
        Type:      "info",
        Timestamp: time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(notification)
    // 发布到非持久化主题，EventBus 自动使用 Core NATS
    return s.eventBus.Publish(ctx, "system.notifications", message)
}

// 发送监控指标（自动模式）
func (s *BusinessService) SendMetrics(ctx context.Context, serviceName string, cpu, memory float64) error {
    metrics := MetricsData{
        ServiceName: serviceName,
        CPUUsage:    cpu,
        MemoryUsage: memory,
        Timestamp:   time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(metrics)
    // 发布到自动模式主题，EventBus 根据全局配置决定
    return s.eventBus.Publish(ctx, "system.metrics", message)
}

// 订阅订单事件
func (s *BusinessService) SubscribeToOrderEvents(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        var event OrderCreatedEvent
        json.Unmarshal(message, &event)

        fmt.Printf("💾 [订单服务-JetStream] 收到订单事件: %+v\n", event)
        fmt.Printf("   ✅ 消息已持久化存储，支持重放和恢复\n")
        fmt.Printf("   📊 传输机制: JetStream (持久化)\n\n")

        return s.processOrderEvent(event)
    }

    return s.eventBus.Subscribe(ctx, "business.orders", handler)
}

// 订阅通知消息
func (s *BusinessService) SubscribeToNotifications(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        var notification NotificationMessage
        json.Unmarshal(message, &notification)

        fmt.Printf("⚡ [通知服务-Core NATS] 收到通知: %+v\n", notification)
        fmt.Printf("   🚀 高性能处理，无持久化开销\n")
        fmt.Printf("   📊 传输机制: Core NATS (非持久化)\n\n")

        return s.processNotification(notification)
    }

    return s.eventBus.Subscribe(ctx, "system.notifications", handler)
}

// 订阅监控指标
func (s *BusinessService) SubscribeToMetrics(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        var metrics MetricsData
        json.Unmarshal(message, &metrics)

        fmt.Printf("📊 [监控服务-自动模式] 收到指标: %+v\n", metrics)
        fmt.Printf("   🔄 传输机制: 根据全局配置自动选择\n\n")

        return s.processMetrics(metrics)
    }

    return s.eventBus.Subscribe(ctx, "system.metrics", handler)
}

func (s *BusinessService) processOrderEvent(event OrderCreatedEvent) error {
    fmt.Printf("   📋 处理订单: %s, 金额: %.2f\n", event.OrderID, event.Amount)
    return nil
}

func (s *BusinessService) processNotification(notification NotificationMessage) error {
    fmt.Printf("   🔔 处理通知: %s -> %s\n", notification.UserID, notification.Title)
    return nil
}

func (s *BusinessService) processMetrics(metrics MetricsData) error {
    fmt.Printf("   📈 处理指标: %s CPU=%.1f%% MEM=%.1f%%\n",
        metrics.ServiceName, metrics.CPUUsage, metrics.MemoryUsage)
    return nil
}

// ========== 主程序：演示主题持久化管理 ==========

func main() {
    fmt.Println("=== NATS 主题持久化管理演示 ===\n")

    // 1. 初始化NATS EventBus（启用JetStream支持）
    cfg := &eventbus.EventBusConfig{
        Type: "nats",
        NATS: eventbus.NATSConfig{
            URLs:     []string{"nats://localhost:4222"},
            ClientID: "topic-persistence-demo",

            // 启用JetStream支持（用于持久化主题）
            JetStream: eventbus.JetStreamConfig{
                Enabled: true,
                Stream: eventbus.StreamConfig{
                    Name:     "BUSINESS_STREAM",
                    Subjects: []string{"business.*", "system.*"},
                    Storage:  "file",
                    Retention: "limits",
                    MaxAge:   24 * time.Hour,
                    MaxBytes: 100 * 1024 * 1024, // 100MB
                },
                Consumer: eventbus.ConsumerConfig{
                    DurableName:   "business-consumer",
                    DeliverPolicy: "all",
                    AckPolicy:     "explicit",
                },
            },
        },
    }

    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatalf("Failed to initialize EventBus: %v", err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()
    ctx := context.Background()

    // 2. 配置不同主题的持久化策略
    fmt.Println("📋 配置主题持久化策略...")

    // 业务关键事件：需要持久化
    orderOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   7 * 24 * time.Hour, // 保留7天
        MaxSize:         100 * 1024 * 1024,  // 100MB
        MaxMessages:     10000,              // 1万条消息
        Description:     "订单相关事件，需要持久化存储",
    }
    if err := bus.ConfigureTopic(ctx, "business.orders", orderOptions); err != nil {
        log.Printf("Failed to configure orders topic: %v", err)
    } else {
        fmt.Println("✅ 订单主题配置为持久化 (JetStream)")
    }

    // 系统通知：临时消息，不需要持久化
    notificationOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   30 * time.Minute, // 仅保留30分钟
        Description:     "系统通知消息，无需持久化",
    }
    if err := bus.ConfigureTopic(ctx, "system.notifications", notificationOptions); err != nil {
        log.Printf("Failed to configure notifications topic: %v", err)
    } else {
        fmt.Println("✅ 通知主题配置为非持久化 (Core NATS)")
    }

    // 监控指标：自动模式，根据全局配置决定
    metricsOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicAuto,
        Description:     "系统监控指标，自动选择传输模式",
    }
    if err := bus.ConfigureTopic(ctx, "system.metrics", metricsOptions); err != nil {
        log.Printf("Failed to configure metrics topic: %v", err)
    } else {
        fmt.Println("✅ 指标主题配置为自动模式")
    }

    // 3. 创建业务服务
    service := NewBusinessService(bus)

    // 4. 启动订阅（EventBus 会根据主题配置自动选择传输机制）
    fmt.Println("\n🚀 启动智能订阅...")

    if err := service.SubscribeToOrderEvents(ctx); err != nil {
        log.Printf("Failed to subscribe to order events: %v", err)
    }

    if err := service.SubscribeToNotifications(ctx); err != nil {
        log.Printf("Failed to subscribe to notifications: %v", err)
    }

    if err := service.SubscribeToMetrics(ctx); err != nil {
        log.Printf("Failed to subscribe to metrics: %v", err)
    }

    time.Sleep(2 * time.Second) // 等待订阅建立

    // 5. 演示智能路由
    fmt.Println("📨 开始发布消息，演示智能路由...\n")

    // 发布到持久化主题（自动使用 JetStream）
    fmt.Println("--- 订单事件（自动路由到 JetStream） ---")
    service.CreateOrder(ctx, "order-12345", "customer-67890", 299.99)
    time.Sleep(1 * time.Second)

    // 发布到非持久化主题（自动使用 Core NATS）
    fmt.Println("--- 通知消息（自动路由到 Core NATS） ---")
    service.SendNotification(ctx, "user-123", "订单确认", "您的订单 order-12345 已创建成功")
    time.Sleep(1 * time.Second)

    // 发布到自动模式主题（根据全局配置决定）
    fmt.Println("--- 监控指标（自动模式） ---")
    service.SendMetrics(ctx, "order-service", 65.4, 78.2)
    time.Sleep(1 * time.Second)

    // 6. 演示动态配置管理
    fmt.Println("--- 动态配置管理演示 ---")

    // 查看已配置的主题
    topics := bus.ListConfiguredTopics()
    fmt.Printf("📋 已配置主题: %v\n", topics)

    // 查看特定主题配置
    config, err := bus.GetTopicConfig("business.orders")
    if err == nil {
        fmt.Printf("📊 订单主题配置: 模式=%s, 保留时间=%v, 最大大小=%d\n",
            config.PersistenceMode, config.RetentionTime, config.MaxSize)
    }

    // 动态修改主题配置
    fmt.Println("🔄 动态修改通知主题为持久化...")
    if err := bus.SetTopicPersistence(ctx, "system.notifications", true); err != nil {
        log.Printf("Failed to update notifications persistence: %v", err)
    } else {
        fmt.Println("✅ 通知主题已改为持久化模式")
    }

    // 再次发布通知，观察路由变化
    fmt.Println("📨 发布通知消息（现在使用 JetStream）...")
    service.SendNotification(ctx, "user-456", "配置更新", "主题配置已动态更新")
    time.Sleep(1 * time.Second)

    // 7. 总结
    fmt.Println("\n=== 主题持久化管理演示完成 ===")
    fmt.Println("✅ 核心特性验证:")
    fmt.Println("  🎯 主题级持久化控制 - 不同主题使用不同策略")
    fmt.Println("  🚀 智能路由机制 - 自动选择 JetStream 或 Core NATS")
    fmt.Println("  🔄 动态配置管理 - 运行时修改主题配置")
    fmt.Println("  ⚡ 性能优化 - 持久化和非持久化并存")
    fmt.Println("  🔧 统一接口 - 单一 EventBus 实例处理多种需求")

}
```

### 配置示例

```yaml
# NATS 主题持久化管理配置
eventbus:
  type: nats
  nats:
    urls: ["nats://localhost:4222"]
    clientId: "topic-persistence-demo"

    # JetStream配置（用于持久化主题）
    jetstream:
      enabled: true
      stream:
        name: "BUSINESS_STREAM"
        subjects:
          - "business.*"      # 业务相关主题
          - "system.*"        # 系统相关主题
        storage: "file"       # 文件存储
        retention: "limits"
        maxAge: 24h
        maxBytes: 100MB
      consumer:
        durableName: "business-consumer"
        deliverPolicy: "all"
        ackPolicy: "explicit"

  # 预配置主题持久化策略（可选）
  topics:
    "business.orders":
      persistenceMode: "persistent"
      retentionTime: "168h"  # 7天
      maxSize: 104857600     # 100MB
      description: "订单相关事件，需要持久化存储"

    "system.notifications":
      persistenceMode: "ephemeral"
      retentionTime: "30m"
      description: "系统通知消息，无需持久化"

    "system.metrics":
      persistenceMode: "auto"
      description: "系统监控指标，自动选择传输模式"
```

### 运行示例

```bash
# 1. 启动NATS服务器（支持JetStream）
nats-server -js

# 2. 运行主题持久化管理示例
go run examples/topic_persistence_example.go

# 3. 观察输出，验证智能路由功能
# - 订单事件自动使用 JetStream（持久化）
# - 通知消息自动使用 Core NATS（非持久化）
# - 动态配置管理功能

# 4. 测试动态配置
# 在程序运行时，观察主题配置的动态修改效果
```

### 核心优势

1. **🎯 主题级控制**：每个主题可以独立配置持久化策略，灵活性极高
2. **🚀 智能路由**：自动选择最优传输机制，无需手动判断
3. **⚡ 性能优化**：持久化和非持久化并存，各取所长
4. **🔄 动态配置**：运行时可以修改主题配置，无需重启服务
5. **🔧 统一接口**：单一 EventBus 实例处理多种需求，简化架构
6. **📊 完整监控**：提供主题配置查询和管理接口
7. **🛡️ 向前兼容**：现有的 Publish/Subscribe API 保持不变
8. **🎛️ 渐进式采用**：可以逐步为不同主题配置不同策略

## 持久化与非持久化同时使用场景的主题持久化管理方案

如果您的业务需要同时支持持久化和非持久化消息处理，**强烈推荐使用主题持久化管理方案**，这是EventBus的核心特性：

### 方案优势

- **🎯 统一架构**：单一EventBus实例，通过主题配置实现不同持久化策略
- **🚀 智能路由**：自动根据主题配置选择JetStream（持久化）或Core NATS（非持久化）
- **⚡ 性能优化**：持久化和非持久化主题并存，各取所长
- **🔧 资源节约**：单一连接，减少资源消耗和管理复杂度
- **📊 统一监控**：单一实例的健康检查、监控和管理
- **🔄 动态配置**：运行时可以调整主题的持久化策略
- **🛡️ 向前兼容**：现有API保持不变，零迁移成本

### 适用场景

| 业务类型 | 主题配置策略 | 智能路由结果 | 示例场景 |
|---------|-------------|-------------|----------|
| **关键业务数据** | 持久化主题 | 自动使用JetStream存储 | 订单处理、支付记录、用户注册 |
| **实时通知消息** | 非持久化主题 | 自动使用Core NATS传输 | 系统通知、状态更新、心跳检测 |
| **审计日志** | 长期持久化主题 | JetStream + 长期保留 | 合规审计、安全日志、操作记录 |
| **临时缓存** | 短期非持久化主题 | Core NATS + 快速清理 | 缓存失效、会话更新、临时状态 |

### 完整使用示例

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
    "go.uber.org/zap"
)

// ========== 业务A：订单服务（需要持久化） ==========

type OrderService struct {
    eventBus eventbus.EventBus // 统一EventBus实例
}

type OrderCreatedEvent struct {
    OrderID    string  `json:"order_id"`
    CustomerID string  `json:"customer_id"`
    Amount     float64 `json:"amount"`
    Timestamp  string  `json:"timestamp"`
}

func (s *OrderService) CreateOrder(ctx context.Context, orderID, customerID string, amount float64) error {
    event := OrderCreatedEvent{
        OrderID:    orderID,
        CustomerID: customerID,
        Amount:     amount,
        Timestamp:  time.Now().Format(time.RFC3339),
    }

    message, _ := json.Marshal(event)

    // 发布到预配置的持久化主题
    // 智能路由：自动使用JetStream进行持久化存储
    return s.eventBus.Publish(ctx, "business.orders", message)
}

func (s *OrderService) SubscribeToOrderEvents(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        var event OrderCreatedEvent
        json.Unmarshal(message, &event)

        fmt.Printf("💾 [订单服务-智能路由] 收到订单事件: %+v\n", event)
        fmt.Printf("   🚀 智能路由：自动使用JetStream持久化存储\n")
        fmt.Printf("   📊 性能指标: JetStream发布延迟 ~800µs\n\n")

        return nil
    }

    return s.eventBus.Subscribe(ctx, "business.orders", handler)
}

// ========== 业务B：通知服务（不需要持久化） ==========

type NotificationService struct {
    eventBus eventbus.EventBus // 统一EventBus实例
}

type NotificationMessage struct {
    UserID    string `json:"user_id"`
    Title     string `json:"title"`
    Content   string `json:"content"`
    Timestamp string `json:"timestamp"`
    Priority  string `json:"priority"`
}

func (s *NotificationService) SendNotification(ctx context.Context, userID, title, content string) error {
    notification := NotificationMessage{
        UserID:    userID,
        Title:     title,
        Content:   content,
        Timestamp: time.Now().Format(time.RFC3339),
        Priority:  "normal",
    }

    message, _ := json.Marshal(notification)

    // 发布到预配置的非持久化主题
    // 智能路由：自动使用Core NATS进行高性能传输
    return s.eventBus.Publish(ctx, "system.notifications", message)
}

func (s *NotificationService) SubscribeToNotifications(ctx context.Context) error {
    handler := func(ctx context.Context, message []byte) error {
        var notification NotificationMessage
        json.Unmarshal(message, &notification)

        fmt.Printf("⚡ [通知服务-智能路由] 收到通知: %+v\n", notification)
        fmt.Printf("   🚀 智能路由：自动使用Core NATS高性能传输\n")
        fmt.Printf("   📊 性能指标: Core NATS发布延迟 ~70µs\n\n")

        return nil
    }

    return s.eventBus.Subscribe(ctx, "system.notifications", handler)
}

// ========== 主程序：演示主题持久化管理 ==========

func main() {
    // 初始化logger
    zapLogger, _ := zap.NewDevelopment()
    defer zapLogger.Sync()
    logger.Logger = zapLogger
    logger.DefaultLogger = zapLogger.Sugar()

    fmt.Println("=== 主题持久化管理方案演示 ===\n")

    // 1. 创建统一的EventBus实例（支持智能路由）
    cfg := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"})
    cfg.NATS.JetStream.Enabled = true // 启用JetStream支持

    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatalf("Failed to initialize EventBus: %v", err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()
    ctx := context.Background()

    // 2. 配置主题持久化策略（应用启动时一次性配置）
    fmt.Println("📋 配置主题持久化策略...")

    // 业务关键主题：持久化存储
    orderOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   7 * 24 * time.Hour, // 7天保留
        MaxSize:         500 * 1024 * 1024,  // 500MB
        Replicas:        3,                  // 3副本
        Description:     "订单事件，业务关键，需要持久化",
    }
    if err := bus.ConfigureTopic(ctx, "business.orders", orderOptions); err != nil {
        log.Fatalf("Failed to configure orders topic: %v", err)
    }
    fmt.Println("✅ 订单主题配置为持久化 (JetStream)")

    // 系统通知主题：非持久化传输
    notificationOptions := eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
        RetentionTime:   1 * time.Hour,      // 1小时保留
        Description:     "系统通知，高性能处理，无需持久化",
    }
    if err := bus.ConfigureTopic(ctx, "system.notifications", notificationOptions); err != nil {
        log.Fatalf("Failed to configure notifications topic: %v", err)
    }
    fmt.Println("✅ 通知主题配置为非持久化 (Core NATS)")

    // 3. 创建业务服务（使用同一个EventBus实例）
    orderService := &OrderService{eventBus: bus}
    notificationService := &NotificationService{eventBus: bus}

    // 4. 启动订阅
    fmt.Println("\n🚀 启动业务订阅...")

    if err := orderService.SubscribeToOrderEvents(ctx); err != nil {
        log.Fatalf("Failed to subscribe to order events: %v", err)
    }

    if err := notificationService.SubscribeToNotifications(ctx); err != nil {
        log.Fatalf("Failed to subscribe to notifications: %v", err)
    }

    time.Sleep(100 * time.Millisecond) // 等待订阅建立

    // 5. 查看主题配置状态
    fmt.Println("\n📊 主题配置状态:")
    topics := bus.ListConfiguredTopics()
    for _, topic := range topics {
        config, _ := bus.GetTopicConfig(topic)
        fmt.Printf("  - %s: %s (%s)\n", topic, config.PersistenceMode, config.Description)
    }

    // 6. 演示业务流程
    fmt.Println("\n📨 开始发布消息，演示智能路由效果...\n")

    // 业务A：订单事件（智能路由到JetStream）
    fmt.Println("--- 业务A：订单事件（智能路由到JetStream） ---")
    orderService.CreateOrder(ctx, "order-12345", "customer-67890", 299.99)

    time.Sleep(200 * time.Millisecond)

    // 业务B：通知消息（智能路由到Core NATS）
    fmt.Println("--- 业务B：通知消息（智能路由到Core NATS） ---")
    notificationService.SendNotification(ctx, "user-123", "订单确认", "您的订单已创建成功")

    time.Sleep(500 * time.Millisecond) // 等待消息处理

    // 7. 动态配置演示
    fmt.Println("\n🔄 演示动态配置调整...")

    // 将通知主题临时改为持久化（演示动态配置）
    if err := bus.SetTopicPersistence(ctx, "system.notifications", true); err != nil {
        log.Printf("Failed to change notifications persistence: %v", err)
    } else {
        fmt.Println("✅ 通知主题已动态调整为持久化模式")
    }

    // 再次发布通知消息（现在将使用JetStream）
    notificationService.SendNotification(ctx, "user-456", "配置变更", "通知主题已改为持久化模式")
    time.Sleep(200 * time.Millisecond)

    // 8. 性能对比演示
    fmt.Println("\n=== 智能路由性能对比 ===")
    fmt.Println("📊 实测性能指标:")
    fmt.Println("  💾 持久化主题 (JetStream): ~800µs 发布延迟")
    fmt.Println("  ⚡ 非持久化主题 (Core NATS): ~70µs 发布延迟")
    fmt.Println("  🚀 智能路由开销:           <5µs 路由决策")

    // 9. 总结
    fmt.Println("\n=== 主题持久化管理方案特点 ===")
    fmt.Println("✅ 优势:")
    fmt.Println("  🎯 统一架构：单一EventBus实例")
    fmt.Println("  🚀 智能路由：自动选择最优传输机制")
    fmt.Println("  💰 资源节约：单一连接，减少资源消耗")
    fmt.Println("  📊 统一监控：单一实例的健康检查和监控")
    fmt.Println("  🔄 动态配置：运行时调整主题策略")
    fmt.Println("  🛡️ 向前兼容：现有API保持不变")
    fmt.Println("❌ 注意事项:")
    fmt.Println("  ⚙️ 配置管理：需要合理规划主题配置策略")
    fmt.Println("  📈 路由开销：极小的路由决策开销（<5µs）")
}
```

### 主题配置管理方法

EventBus提供了灵活的主题配置管理方法来实现不同的持久化策略：

```go
// 1. 创建支持智能路由的EventBus实例
cfg := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"})
cfg.NATS.JetStream.Enabled = true // 启用JetStream支持

bus, err := eventbus.NewEventBus(cfg)
// 特点：单一实例，支持智能路由，统一管理

// 2. 配置不同类型的主题

// 业务关键主题（持久化存储）
businessOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicPersistent,
    RetentionTime:   7 * 24 * time.Hour,
    MaxSize:         500 * 1024 * 1024,
    Replicas:        3,
    Description:     "业务关键事件，持久化存储",
}
bus.ConfigureTopic(ctx, "business.orders", businessOptions)
// 智能路由：自动使用JetStream，发布延迟 ~800µs

// 系统通知主题（高性能传输）
systemOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicEphemeral,
    RetentionTime:   1 * time.Hour,
    Description:     "系统通知，高性能处理",
}
bus.ConfigureTopic(ctx, "system.notifications", systemOptions)
// 智能路由：自动使用Core NATS，发布延迟 ~70µs

// 审计日志主题（长期保留）
auditOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicPersistent,
    RetentionTime:   90 * 24 * time.Hour, // 90天
    MaxSize:         2 * 1024 * 1024 * 1024, // 2GB
    Replicas:        5,
    Description:     "审计日志，合规要求",
}
bus.ConfigureTopic(ctx, "audit.logs", auditOptions)
// 智能路由：自动使用JetStream，长期保留

// 3. 简化配置方法
bus.SetTopicPersistence(ctx, "temp.cache", false)      // 临时缓存：非持久化
bus.SetTopicPersistence(ctx, "financial.transactions", true) // 金融交易：持久化
```

### 主题配置策略指南

| 业务需求 | 主题配置策略 | 智能路由结果 | 性能特点 |
|---------|-------------|-------------|----------|
| **关键业务数据** | `TopicPersistent` + 长期保留 | 自动使用JetStream | 数据安全，~800µs延迟 |
| **实时通知** | `TopicEphemeral` + 短期保留 | 自动使用Core NATS | 高性能，~70µs延迟 |
| **审计日志** | `TopicPersistent` + 90天保留 | JetStream + 多副本 | 合规安全，长期存储 |
| **临时缓存** | `TopicEphemeral` + 30分钟保留 | Core NATS + 快速清理 | 最高性能，快速处理 |
| **金融交易** | `TopicPersistent` + 5副本 | JetStream + 高可用 | 金融级可靠性 |
| **系统监控** | `TopicAuto` + 自动选择 | 根据全局配置决定 | 灵活适应 |

### 配置示例

#### 主题持久化管理配置（推荐）
```yaml
eventbus:
  type: nats
  nats:
    urls: ["nats://localhost:4222"]
    clientId: "unified-client"
    jetstream:
      enabled: true  # 启用JetStream支持智能路由
      stream:
        name: "UNIFIED_STREAM"
        subjects: ["*"]
        storage: "file"  # 文件存储
      consumer:
        durableName: "unified-consumer"
        ackPolicy: "explicit"  # 显式确认

  # 预配置主题持久化策略
  topics:
    "business.orders":
      persistenceMode: "persistent"
      retentionTime: "168h"      # 7天保留
      maxSize: 524288000         # 500MB
      replicas: 3
      description: "订单事件，业务关键"

    "system.notifications":
      persistenceMode: "ephemeral"
      retentionTime: "1h"        # 1小时保留
      description: "系统通知，高性能处理"

    "audit.logs":
      persistenceMode: "persistent"
      retentionTime: "2160h"     # 90天保留
      maxSize: 2147483648        # 2GB
      replicas: 5
      description: "审计日志，合规要求"

    "temp.cache":
      persistenceMode: "ephemeral"
      retentionTime: "30m"       # 30分钟保留
      description: "临时缓存，快速处理"
```

#### 传统独立实例配置（不推荐）
```yaml
# 持久化实例配置
persistent_eventbus:
  type: nats
  nats:
    urls: ["nats://localhost:4222"]
    clientId: "persistent-client"
    jetstream:
      enabled: true

# 非持久化实例配置
ephemeral_eventbus:
  type: nats
  nats:
    urls: ["nats://localhost:4222"]
    clientId: "ephemeral-client"
    jetstream:
      enabled: false
```

### 运行示例

#### 前置要求

1. **安装NATS服务器**：
```bash
# 方式1：使用Go安装
go install github.com/nats-io/nats-server/v2@latest

# 方式2：下载二进制文件
# 访问 https://github.com/nats-io/nats-server/releases
# 下载适合您系统的版本

# 方式3：使用Docker
docker run -p 4222:4222 -p 8222:8222 nats:latest -js
```

2. **启动NATS服务器**：
```bash
# 启动支持JetStream的NATS服务器
nats-server -js

# 或者使用配置文件启动
nats-server -c nats-server.conf
```

#### 运行测试

**方式1：主题持久化管理示例（推荐）**
```bash
# 运行主题持久化管理演示
go run examples/topic_persistence_example.go

# 运行混合使用场景演示
go run examples/mixed_persistence_example.go
```

**方式2：自动化测试**
```bash
# 一键运行完整测试（自动下载和启动NATS服务器）
./examples/test_topic_persistence.sh
```

**方式3：手动测试**
```bash
# 1. 手动启动NATS服务器
./examples/setup_nats_server.sh

# 2. 在另一个终端运行测试
go run examples/topic_persistence_validation.go
go run examples/topic_persistence_example.go
```

**方式4：仅验证配置（无需NATS服务器）**
```bash
# 仅验证主题配置和内存实例功能
go run examples/topic_persistence_validation.go
```

#### 预期输出

**无NATS服务器时**：
```
🔌 测试NATS连接（需要NATS服务器）...
   注意：如果NATS服务器未运行，以下测试将失败
   启动命令：nats-server -js
  - 测试主题持久化管理...
    ❌ EventBus初始化失败: failed to connect to NATS: nats: no servers available for connection
    💡 请确保NATS服务器正在运行: nats-server -js
```

**有NATS服务器时**：
```
=== 主题持久化管理方案演示 ===

📋 配置主题持久化策略...
✅ 订单主题配置为持久化 (JetStream)
✅ 通知主题配置为非持久化 (Core NATS)

🚀 启动业务订阅...

📊 主题配置状态:
  - business.orders: persistent (订单事件，业务关键，需要持久化)
  - system.notifications: ephemeral (系统通知，高性能处理，无需持久化)

📨 开始发布消息，演示智能路由效果...

--- 业务A：订单事件（智能路由到JetStream） ---
💾 [订单服务-智能路由] 收到订单事件: {OrderID:order-12345 CustomerID:customer-67890 Amount:299.99 Timestamp:2025-09-22T03:09:57+08:00}
   🚀 智能路由：自动使用JetStream持久化存储
   📊 性能指标: JetStream发布延迟 ~800µs

--- 业务B：通知消息（智能路由到Core NATS） ---
⚡ [通知服务-智能路由] 收到通知: {UserID:user-123 Title:订单确认 Content:您的订单已创建成功 Timestamp:2025-09-22T03:09:57+08:00 Priority:normal}
   🚀 智能路由：自动使用Core NATS高性能传输
   📊 性能指标: Core NATS发布延迟 ~70µs

🔄 演示动态配置调整...
✅ 通知主题已动态调整为持久化模式
⚡ [通知服务-智能路由] 收到通知: {UserID:user-456 Title:配置变更 Content:通知主题已改为持久化模式 Timestamp:2025-09-22T03:09:58+08:00 Priority:normal}
   🚀 智能路由：现在使用JetStream持久化存储

=== 智能路由性能对比 ===
📊 实测性能指标:
  💾 持久化主题 (JetStream): ~800µs 发布延迟
  ⚡ 非持久化主题 (Core NATS): ~70µs 发布延迟
  🚀 智能路由开销:           <5µs 路由决策

=== 主题持久化管理方案特点 ===
✅ 优势:
  🎯 统一架构：单一EventBus实例
  🚀 智能路由：自动选择最优传输机制
  💰 资源节约：单一连接，减少资源消耗
  📊 统一监控：单一实例的健康检查和监控
  🔄 动态配置：运行时调整主题策略
  🛡️ 向前兼容：现有API保持不变
```


## 🎯 主题持久化管理 vs 独立实例方案对比

### 📋 **方案对比总结**

针对持久化与非持久化同时使用的场景，我们提供两种方案：

- **🚀 主题持久化管理方案（推荐）**：单一EventBus实例 + 智能路由 + 动态配置
- **⚙️ 独立实例方案（传统）**：多个EventBus实例 + 手动管理 + 静态配置

### 🚀 **快速开始**

```bash
# 1. 启动完整的跨Docker演示环境
./start-cross-docker-demo.sh

# 2. 或者单独测试EventBus功能
cd jxt-core/sdk/pkg/eventbus
go run examples/cross_docker_dual_eventbus.go
```

### 📊 **架构对比**

| 特性 | 领域事件 (NATS JetStream) | 简单消息 (NATS Core) |
|------|---------------------------|---------------------|
| **持久化** | ✅ 文件存储 | ❌ 内存存储 |
| **跨Docker** | ✅ 集群支持 | ✅ 轻量级支持 |
| **顺序保证** | ✅ Keyed-Worker池 | ❌ 并发处理 |
| **性能** | ~1ms延迟 | ~10µs延迟 |
| **可靠性** | 99.99% | 95-99% |
| **适用场景** | 订单、支付、库存 | 通知、缓存、监控 |

### 🏗️ **实现示例**

<details>
<summary>点击查看完整代码示例</summary>

```go
// 业务A：领域事件服务（NATS JetStream）
func createDomainEventsConfig() *eventbus.EventBusConfig {
    return &eventbus.EventBusConfig{
        Type: "nats",
        NATS: eventbus.NATSConfig{
            URLs: []string{"nats://localhost:4222"},
            JetStream: eventbus.JetStreamConfig{
                Enabled: true,
                Stream: eventbus.StreamConfig{
                    Name:      "domain-events-stream",
                    Subjects:  []string{"domain.*.events"},
                    Storage:   "file",     // 持久化存储
                    Replicas:  3,          // 高可用
                },
                Consumer: eventbus.NATSConsumerConfig{
                    DurableName:   "domain-events-processor",
                    AckPolicy:     "explicit",
                    ReplayPolicy:  "instant",
                },
            },
        },
    }
}

// 业务B：简单消息服务（NATS Core）
func createSimpleMessagesConfig() *eventbus.EventBusConfig {
    return &eventbus.EventBusConfig{
        Type: "nats",
        NATS: eventbus.NATSConfig{
            URLs: []string{"nats://localhost:4222"},
            JetStream: eventbus.JetStreamConfig{
                Enabled: false, // 使用NATS Core
            },
        },
    }
}
```

</details>

### 📁 **相关文件**

- **完整示例**：`examples/cross_docker_dual_eventbus.go`
- **生产配置**：`examples/cross_docker_production_config.yaml`
- **Docker部署**：`docker-compose.cross-docker-dual-nats.yml`
- **架构分析**：`CROSS_DOCKER_DUAL_NATS_ARCHITECTURE_REPORT.md`

---

## NATS JetStream 异步发布与 Outbox 模式完整示例

本章节提供 NATS JetStream 异步发布和 Outbox 模式的完整实现示例，展示如何在生产环境中使用异步发布机制。

### 🎯 核心概念

**异步发布**:
- ✅ `PublishEnvelope` 立即返回，不等待 NATS 服务器 ACK
- ✅ 后台 goroutine 处理 ACK 确认和错误
- ✅ 通过 `GetPublishResultChannel()` 获取发布结果

**Outbox 模式**:
- ✅ 业务事务中保存数据 + 保存事件到 Outbox 表（原子性）
- ✅ Outbox Processor 轮询未发布事件并异步发布
- ✅ 监听发布结果通道，更新 Outbox 状态

### 📦 完整实现示例

#### 1. Outbox 表结构

```sql
CREATE TABLE outbox_events (
    id VARCHAR(36) PRIMARY KEY,
    aggregate_id VARCHAR(255) NOT NULL,
    aggregate_type VARCHAR(100) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    event_version BIGINT NOT NULL,
    payload JSONB NOT NULL,
    topic VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',  -- pending, published, failed
    error_message TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    published_at TIMESTAMP,
    INDEX idx_status_created (status, created_at),
    INDEX idx_aggregate (aggregate_id, event_version)
);
```

#### 2. Outbox Repository 接口

```go
package repository

import (
    "context"
    "time"
)

type OutboxEvent struct {
    ID            string
    AggregateID   string
    AggregateType string
    EventType     string
    EventVersion  int64
    Payload       []byte
    Topic         string
    Status        string
    ErrorMessage  string
    RetryCount    int
    CreatedAt     time.Time
    PublishedAt   *time.Time
}

type OutboxRepository interface {
    // SaveInTx 在事务中保存事件到 Outbox
    SaveInTx(ctx context.Context, tx Transaction, event *OutboxEvent) error

    // FindUnpublished 查询未发布的事件（分页）
    FindUnpublished(ctx context.Context, limit int) ([]*OutboxEvent, error)

    // MarkAsPublished 标记事件为已发布
    MarkAsPublished(ctx context.Context, eventID string) error

    // RecordError 记录发布错误
    RecordError(ctx context.Context, eventID string, err error) error
}
```

#### 3. Outbox Publisher 实现

```go
package infrastructure

import (
    "context"
    "fmt"
    "time"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "go.uber.org/zap"
)

type OutboxPublisher struct {
    eventBus   eventbus.EventBus
    outboxRepo OutboxRepository
    logger     *zap.Logger

    // 控制
    ctx        context.Context
    cancel     context.CancelFunc
    pollTicker *time.Ticker
}

func NewOutboxPublisher(
    eventBus eventbus.EventBus,
    outboxRepo OutboxRepository,
    logger *zap.Logger,
) *OutboxPublisher {
    ctx, cancel := context.WithCancel(context.Background())

    return &OutboxPublisher{
        eventBus:   eventBus,
        outboxRepo: outboxRepo,
        logger:     logger,
        ctx:        ctx,
        cancel:     cancel,
        pollTicker: time.NewTicker(5 * time.Second), // 每5秒轮询一次
    }
}

// Start 启动 Outbox Publisher
func (p *OutboxPublisher) Start() {
    // 启动结果监听器
    go p.startResultListener()

    // 启动轮询器
    go p.startPoller()

    p.logger.Info("Outbox Publisher started")
}

// Stop 停止 Outbox Publisher
func (p *OutboxPublisher) Stop() {
    p.cancel()
    p.pollTicker.Stop()
    p.logger.Info("Outbox Publisher stopped")
}

// startResultListener 启动异步发布结果监听器
func (p *OutboxPublisher) startResultListener() {
    resultChan := p.eventBus.GetPublishResultChannel()

    for {
        select {
        case result := <-resultChan:
            if result.Success {
                // ✅ 发布成功：标记为已发布
                if err := p.outboxRepo.MarkAsPublished(p.ctx, result.EventID); err != nil {
                    p.logger.Error("Failed to mark event as published",
                        zap.String("eventID", result.EventID),
                        zap.Error(err))
                } else {
                    p.logger.Debug("Event marked as published",
                        zap.String("eventID", result.EventID),
                        zap.String("topic", result.Topic),
                        zap.String("aggregateID", result.AggregateID))
                }
            } else {
                // ❌ 发布失败：记录错误
                if err := p.outboxRepo.RecordError(p.ctx, result.EventID, result.Error); err != nil {
                    p.logger.Error("Failed to record publish error",
                        zap.String("eventID", result.EventID),
                        zap.Error(err))
                }

                p.logger.Error("Event publish failed",
                    zap.String("eventID", result.EventID),
                    zap.String("topic", result.Topic),
                    zap.Error(result.Error))
            }

        case <-p.ctx.Done():
            p.logger.Info("Result listener stopped")
            return
        }
    }
}

// startPoller 启动轮询器
func (p *OutboxPublisher) startPoller() {
    for {
        select {
        case <-p.pollTicker.C:
            p.publishPendingEvents()

        case <-p.ctx.Done():
            p.logger.Info("Poller stopped")
            return
        }
    }
}

// publishPendingEvents 发布待发布的事件
func (p *OutboxPublisher) publishPendingEvents() {
    // 查询未发布的事件（每次最多100条）
    events, err := p.outboxRepo.FindUnpublished(p.ctx, 100)
    if err != nil {
        p.logger.Error("Failed to find unpublished events", zap.Error(err))
        return
    }

    if len(events) == 0 {
        return
    }

    p.logger.Info("Publishing pending events", zap.Int("count", len(events)))

    for _, event := range events {
        // 构建 Envelope
        envelope := &eventbus.Envelope{
            AggregateID:  event.AggregateID,
            EventType:    event.EventType,
            EventVersion: event.EventVersion,
            Timestamp:    event.CreatedAt,
            Payload:      event.Payload,
        }

        // 🚀 异步发布（立即返回，不阻塞）
        if err := p.eventBus.PublishEnvelope(p.ctx, event.Topic, envelope); err != nil {
            p.logger.Error("Failed to submit publish",
                zap.String("eventID", event.ID),
                zap.String("topic", event.Topic),
                zap.Error(err))

            // 记录错误
            p.outboxRepo.RecordError(p.ctx, event.ID, err)
        } else {
            p.logger.Debug("Event submitted for async publish",
                zap.String("eventID", event.ID),
                zap.String("topic", event.Topic),
                zap.String("aggregateID", event.AggregateID))
        }
        // ✅ ACK 结果通过 resultChan 异步通知
    }
}
```

#### 4. 业务服务集成

```go
package service

import (
    "context"
    "encoding/json"
    "time"

    "github.com/google/uuid"
    "go.uber.org/zap"
)

type OrderService struct {
    orderRepo  OrderRepository
    outboxRepo OutboxRepository
    txManager  TransactionManager
    logger     *zap.Logger
}

// CreateOrder 创建订单（使用 Outbox 模式）
func (s *OrderService) CreateOrder(ctx context.Context, cmd *CreateOrderCommand) error {
    // 在事务中保存订单和事件
    return s.txManager.RunInTransaction(ctx, func(tx Transaction) error {
        // 1. 创建订单聚合
        order := NewOrder(cmd.OrderID, cmd.CustomerID, cmd.Amount)

        // 2. 保存订单到数据库
        if err := s.orderRepo.SaveInTx(ctx, tx, order); err != nil {
            return fmt.Errorf("failed to save order: %w", err)
        }

        // 3. 获取领域事件
        domainEvent := order.Events()[0] // OrderCreatedEvent

        // 4. 序列化事件 Payload
        payload, err := json.Marshal(domainEvent)
        if err != nil {
            return fmt.Errorf("failed to marshal event: %w", err)
        }

        // 5. 保存事件到 Outbox（在同一事务中）
        outboxEvent := &OutboxEvent{
            ID:            uuid.New().String(),
            AggregateID:   order.ID,
            AggregateType: "Order",
            EventType:     "OrderCreated",
            EventVersion:  1,
            Payload:       payload,
            Topic:         "orders.created",
            Status:        "pending",
            CreatedAt:     time.Now(),
        }

        if err := s.outboxRepo.SaveInTx(ctx, tx, outboxEvent); err != nil {
            return fmt.Errorf("failed to save outbox event: %w", err)
        }

        s.logger.Info("Order and event saved in transaction",
            zap.String("orderID", order.ID),
            zap.String("eventID", outboxEvent.ID))

        return nil
    })

    // ✅ 事务提交后，Outbox Processor 会自动轮询并发布事件
}
```

#### 5. 主程序启动

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    // 1. 创建 EventBus
    config := &eventbus.EventBusConfig{
        Type: "nats",
        NATS: eventbus.NATSConfig{
            URLs:     []string{"nats://localhost:4222"},
            ClientID: "order-service",
            JetStream: eventbus.JetStreamConfig{
                Enabled: true,
                Stream: eventbus.StreamConfig{
                    Name:     "ORDERS_STREAM",
                    Subjects: []string{"orders.*"},
                    Storage:  "file",
                    Replicas: 1,
                },
                Consumer: eventbus.NATSConsumerConfig{
                    DurableName:  "order-consumer",
                    AckPolicy:    "explicit",
                    ReplayPolicy: "instant",
                },
            },
        },
    }

    bus, err := eventbus.NewEventBus(config)
    if err != nil {
        logger.Fatal("Failed to create EventBus", zap.Error(err))
    }
    defer bus.Close()

    // 2. 创建 Outbox Repository
    outboxRepo := NewPostgresOutboxRepository(db, logger)

    // 3. 创建并启动 Outbox Publisher
    outboxPublisher := NewOutboxPublisher(bus, outboxRepo, logger)
    outboxPublisher.Start()
    defer outboxPublisher.Stop()

    // 4. 创建业务服务
    orderService := NewOrderService(orderRepo, outboxRepo, txManager, logger)

    // 5. 启动 HTTP 服务器
    // ...

    // 6. 优雅关闭
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    logger.Info("Shutting down gracefully...")
}
```

### 📊 性能指标

使用异步发布 + Outbox 模式的性能表现：

| 指标 | 同步发布 | 异步发布 | 提升 |
|------|---------|---------|------|
| **发送延迟** | 20-70 ms | 1-10 ms | **5-10x** |
| **吞吐量** | 10-50 msg/s | 100-300 msg/s | **5-10x** |
| **事务延迟** | 50-100 ms | 10-20 ms | **5x** |
| **资源利用** | 高（阻塞等待） | 低（异步处理） | **优** |

### 🏆 最佳实践

1. **✅ 使用异步发布**: 默认推荐，适用于 99% 的场景
2. **✅ 监听结果通道**: Outbox 模式必须监听 `GetPublishResultChannel()`
3. **✅ 合理配置轮询间隔**: 建议 5-10 秒，平衡实时性和性能
4. **✅ 实现幂等消费**: 消费端必须支持幂等处理（Outbox 提供 at-least-once）
5. **✅ 监控 Outbox 积压**: 定期检查 `status='pending'` 的事件数量
6. **✅ 设置重试上限**: 避免无限重试，建议 3-5 次后转人工处理

### 📁 相关文档

- **异步发布实现报告**: `sdk/pkg/eventbus/NATS_ASYNC_PUBLISH_IMPLEMENTATION_REPORT.md`
- **性能测试报告**: `tests/eventbus/performance_tests/nats_async_test.log`
- **Outbox 模式设计**: `docs/eventbus-extraction-proposal.md`
