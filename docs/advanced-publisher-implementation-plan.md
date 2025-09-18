# 高级事件发布器实施方案

## 概述

本文档描述了将 evidence-management 中的 `KafkaPublisherManager` 高级功能迁移到 jxt-core 的具体实施方案。目标是创建一个功能完整的高级事件发布器，提供统一健康检查、重连机制、发布回调等企业级特性。

## 功能分析总结

### ✅ **可以移到 jxt-core 的功能（约80%）**

#### 1. **连接管理**
- Publisher 初始化和关闭
- 连接池管理
- 原子操作的 Publisher 存储

#### 2. **统一健康检查机制**
- **主题管理**：jxt-core 统一管理健康检查主题
- **消息格式**：标准化的健康检查消息格式
- **响应验证**：统一的健康检查响应验证逻辑
- **调度机制**：定期健康检查循环

#### 3. **重连机制和故障恢复**
- 指数退避算法
- 重连尝试逻辑
- 失败计数管理
- 连接状态跟踪

#### 4. **回调机制框架**
- 重连回调注册和执行
- 发布结果回调
- 健康检查状态回调
- 多回调支持

#### 5. **生命周期管理**
- 启动和关闭流程
- Context 管理
- WaitGroup 协调
- 优雅关闭

### ❌ **必须保留在业务层的功能（约20%）**

#### 1. **业务特定的消息格式化**
- 聚合ID类型转换逻辑
- 业务特定的元数据字段
- 消息序列化策略

#### 2. **业务事件发布逻辑**
- 业务事件的序列化
- 业务特定的错误处理
- 事件接口适配

#### 3. **业务主题映射**
- 主题名称定义
- 主题路由策略

#### 4. **Outbox 模式集成**
- 事件状态管理
- 重试策略
- 业务日志记录

## 目标架构设计

### 核心接口设计

```go
// AdvancedEventBus 高级事件总线接口
type AdvancedEventBus interface {
    EventBus // 继承基础接口
    
    // 高级发布功能
    PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error
    
    // 统一健康检查管理（完全由 jxt-core 负责）
    StartHealthCheck(ctx context.Context) error
    StopHealthCheck() error
    GetHealthStatus() HealthCheckStatus
    RegisterHealthCheckCallback(callback HealthCheckCallback) error
    
    // 重连管理
    RegisterReconnectCallback(callback ReconnectCallback) error
    GetConnectionState() ConnectionState
    
    // 消息格式化
    SetMessageFormatter(formatter MessageFormatter) error
    
    // 发布回调
    RegisterPublishCallback(callback PublishCallback) error
}

// 发布选项
type PublishOptions struct {
    MessageFormatter MessageFormatter
    Metadata        map[string]string
    RetryPolicy     RetryPolicy
    Timeout         time.Duration
    AggregateID     interface{}
}

// 回调函数类型
type HealthCheckCallback func(ctx context.Context, result HealthCheckResult) error
type ReconnectCallback func(ctx context.Context) error
type PublishCallback func(ctx context.Context, topic string, message []byte, err error) error

// 消息格式化器接口
type MessageFormatter interface {
    FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error)
    ExtractAggregateID(aggregateID interface{}) string
    SetMetadata(msg *Message, metadata map[string]string) error
}
```

### 统一健康检查设计

#### 1. **标准化健康检查主题**
```go
const (
    // jxt-core 统一管理的健康检查主题
    DefaultHealthCheckTopic = "jxt-core-health-check"
    KafkaHealthCheckTopic   = "jxt-core-kafka-health-check"
    NATSHealthCheckTopic    = "jxt-core-nats-health-check"
)
```

#### 2. **标准化健康检查消息**
```go
type HealthCheckMessage struct {
    MessageID    string            `json:"messageId"`
    Timestamp    time.Time         `json:"timestamp"`
    Source       string            `json:"source"`       // 微服务名称
    EventBusType string            `json:"eventBusType"` // kafka/nats/memory
    Version      string            `json:"version"`      // jxt-core版本
    Metadata     map[string]string `json:"metadata"`
}
```

#### 3. **健康检查器实现**
```go
type HealthChecker struct {
    config          HealthCheckConfig
    eventBus        EventBus
    source          string // 微服务名称
    eventBusType    string // 事件总线类型
    
    // 状态管理
    isRunning       atomic.Bool
    consecutiveFailures int32
    lastSuccessTime atomic.Value
    lastFailureTime atomic.Value
    
    // 回调
    callbacks       []HealthCheckCallback
}
```

## 实施阶段

### 阶段 1：核心接口和配置（1周）

#### 1.1 扩展配置结构
**文件**: `jxt-core/sdk/config/advanced_eventbus.go`

```go
type AdvancedEventBusConfig struct {
    EventBusConfig `mapstructure:",squash"`
    ServiceName    string `mapstructure:"serviceName"` // 微服务名称
    
    // 统一健康检查配置
    HealthCheck HealthCheckConfig `mapstructure:"healthCheck"`
    
    // 发布端配置
    Publisher PublisherConfig `mapstructure:"publisher"`
    
    // 订阅端配置（已有）
    Subscriber SubscriberConfig `mapstructure:"subscriber"`
}

type HealthCheckConfig struct {
    Enabled          bool          `mapstructure:"enabled"`
    Topic           string        `mapstructure:"topic"`
    Interval        time.Duration `mapstructure:"interval"`
    Timeout         time.Duration `mapstructure:"timeout"`
    FailureThreshold int          `mapstructure:"failureThreshold"`
    MessageTTL      time.Duration `mapstructure:"messageTTL"`
}

type PublisherConfig struct {
    MaxReconnectAttempts int           `mapstructure:"maxReconnectAttempts"`
    MaxBackoff          time.Duration `mapstructure:"maxBackoff"`
    InitialBackoff      time.Duration `mapstructure:"initialBackoff"`
    PublishTimeout      time.Duration `mapstructure:"publishTimeout"`
}
```

#### 1.2 高级接口定义
**文件**: `jxt-core/sdk/pkg/eventbus/advanced_interface.go`

### 阶段 2：健康检查组件（1周）

#### 2.1 健康检查消息
**文件**: `jxt-core/sdk/pkg/eventbus/health_check_message.go`

#### 2.2 健康检查器
**文件**: `jxt-core/sdk/pkg/eventbus/health_checker.go`

### 阶段 3：发布端管理器（2周）

#### 3.1 高级发布管理器
**文件**: `jxt-core/sdk/pkg/eventbus/advanced_publisher.go`

```go
type AdvancedPublisher struct {
    config           PublisherConfig
    eventBus         EventBus
    healthChecker    *HealthChecker
    
    // 连接管理
    publisher        atomic.Value // 存储具体的 Publisher
    isConnected      atomic.Bool
    
    // 重连管理
    backoff          time.Duration
    failureCount     int32
    
    // 回调管理
    reconnectCallbacks []ReconnectCallback
    publishCallbacks   []PublishCallback
    
    // 消息格式化
    messageFormatter MessageFormatter
    
    // 生命周期
    ctx              context.Context
    cancel           context.CancelFunc
    wg               sync.WaitGroup
}
```

#### 3.2 消息格式化器
**文件**: `jxt-core/sdk/pkg/eventbus/message_formatter.go`

```go
// DefaultMessageFormatter 默认消息格式化器
type DefaultMessageFormatter struct{}

func (f *DefaultMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
    msg := NewMessage(uuid, payload)
    
    // 提取聚合ID
    aggID := f.ExtractAggregateID(aggregateID)
    if aggID != "" {
        msg.Metadata["aggregateID"] = aggID
    }
    
    return msg, nil
}

func (f *DefaultMessageFormatter) ExtractAggregateID(aggregateID interface{}) string {
    if aggregateID == nil {
        return ""
    }
    
    switch id := aggregateID.(type) {
    case string:
        return id
    case int64:
        return strconv.FormatInt(id, 10)
    case int:
        return strconv.Itoa(id)
    default:
        return fmt.Sprintf("%v", id)
    }
}

// EvidenceMessageFormatter 业务特定的消息格式化器
type EvidenceMessageFormatter struct {
    DefaultMessageFormatter
}

func (f *EvidenceMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
    msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
    if err != nil {
        return nil, err
    }
    
    // 业务特定的元数据字段名
    if aggID := f.ExtractAggregateID(aggregateID); aggID != "" {
        msg.Metadata["aggregate_id"] = aggID // evidence-management 使用的字段名
    }
    
    return msg, nil
}
```

### 阶段 4：Kafka 集成（1周）

#### 4.1 Kafka 高级事件总线
**文件**: `jxt-core/sdk/pkg/eventbus/kafka_advanced.go`

```go
type kafkaAdvancedEventBus struct {
    *kafkaEventBus // 继承基础实现
    
    // 高级组件
    advancedPublisher *AdvancedPublisher
    healthChecker     *HealthChecker
    
    // 配置
    advancedConfig AdvancedEventBusConfig
}

func NewKafkaAdvancedEventBus(config AdvancedEventBusConfig) (AdvancedEventBus, error) {
    // 创建基础 EventBus
    baseEventBus, err := NewKafkaEventBus(config.EventBusConfig.Kafka)
    if err != nil {
        return nil, err
    }
    
    kafkaBase := baseEventBus.(*kafkaEventBus)
    
    advanced := &kafkaAdvancedEventBus{
        kafkaEventBus:  kafkaBase,
        advancedConfig: config,
    }
    
    // 初始化高级发布器
    advanced.advancedPublisher = NewAdvancedPublisher(
        config.Publisher,
        advanced, // 使用自身作为 EventBus
    )
    
    // 初始化健康检查器
    if config.HealthCheck.Enabled {
        advanced.healthChecker = NewHealthChecker(
            config.HealthCheck,
            advanced,
            config.ServiceName,
            "kafka",
        )
    }
    
    return advanced, nil
}

func (k *kafkaAdvancedEventBus) PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error {
    return k.advancedPublisher.PublishWithOptions(ctx, topic, message, opts)
}

func (k *kafkaAdvancedEventBus) StartHealthCheck(ctx context.Context) error {
    if k.healthChecker == nil {
        return fmt.Errorf("health check is not enabled")
    }
    return k.healthChecker.Start(ctx)
}

func (k *kafkaAdvancedEventBus) SetMessageFormatter(formatter MessageFormatter) error {
    return k.advancedPublisher.SetMessageFormatter(formatter)
}
```

### 阶段 5：业务层适配（1周）

#### 5.1 业务层适配器
**文件**: `evidence-management/shared/common/eventbus/advanced_adapter.go`

```go
// EventPublisherAdapter 事件发布器适配器
type EventPublisherAdapter struct {
    advancedBus eventbus.AdvancedEventBus
}

func NewEventPublisherAdapter(bus eventbus.AdvancedEventBus) *EventPublisherAdapter {
    adapter := &EventPublisherAdapter{
        advancedBus: bus,
    }
    
    // 设置业务特定的消息格式化器
    bus.SetMessageFormatter(&EvidenceMessageFormatter{})
    
    // 注册业务回调
    bus.RegisterReconnectCallback(adapter.handleReconnect)
    bus.RegisterPublishCallback(adapter.handlePublishResult)
    bus.RegisterHealthCheckCallback(adapter.handleHealthCheck)
    
    return adapter
}

// PublishMessage 保持与原有接口兼容
func (a *EventPublisherAdapter) PublishMessage(topic string, uuid string, aggregateID interface{}, payload []byte) error {
    opts := eventbus.PublishOptions{
        AggregateID: aggregateID,
        Timeout:     30 * time.Second,
    }
    
    return a.advancedBus.PublishWithOptions(context.Background(), topic, payload, opts)
}

// Publish 实现 EventPublisher 接口
func (a *EventPublisherAdapter) Publish(ctx context.Context, topic string, event event.Event) error {
    if event == nil {
        return fmt.Errorf("event cannot be nil")
    }
    
    payload, err := event.MarshalJSON()
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }
    
    opts := eventbus.PublishOptions{
        AggregateID: event.GetAggregateID(),
        Metadata: map[string]string{
            "eventType": event.GetEventType(),
            "eventID":   event.GetEventID(),
        },
        Timeout: 30 * time.Second,
    }
    
    return a.advancedBus.PublishWithOptions(ctx, topic, payload, opts)
}

func (a *EventPublisherAdapter) handleReconnect(ctx context.Context) error {
    // 业务特定的重连处理逻辑
    log.Println("Kafka publisher reconnected successfully")
    return nil
}

func (a *EventPublisherAdapter) handlePublishResult(ctx context.Context, topic string, message []byte, err error) error {
    if err != nil {
        // 业务特定的发布失败处理
        log.Printf("Publish failed for topic %s: %v", topic, err)
    }
    return nil
}

func (a *EventPublisherAdapter) handleHealthCheck(ctx context.Context, result eventbus.HealthCheckResult) error {
    if !result.Success {
        // 业务特定的健康检查失败处理
        log.Printf("Health check failed: %v", result.Error)
        // 可以发送告警、更新监控指标等
    }
    return nil
}
```

#### 5.2 依赖注入更新
**文件**: `evidence-management/command/internal/infrastructure/eventbus/publisher.go`

```go
func registerKafkaEventPublisherDependencies() {
    // 注册高级事件总线
    err := di.Provide(func() (eventbus.AdvancedEventBus, error) {
        config := GetEventBusConfig()
        return eventbus.NewKafkaAdvancedEventBus(config)
    })
    if err != nil {
        logger.Fatalf("failed to provide AdvancedEventBus: %v", err)
    }
    
    // 注册适配器
    err = di.Provide(func(bus eventbus.AdvancedEventBus) publisher.EventPublisher {
        return NewEventPublisherAdapter(bus)
    })
    if err != nil {
        logger.Fatalf("failed to provide EventPublisher: %v", err)
    }
}
```

## 配置示例

### jxt-core 配置
```yaml
eventbus:
  type: "kafka"
  serviceName: "evidence-management"
  
  kafka:
    brokers: ["localhost:9092"]
    consumer_group: "evidence-management"
  
  healthCheck:
    enabled: true
    topic: "jxt-core-kafka-health-check"  # jxt-core 统一管理
    interval: "2m"
    timeout: "10s"
    failureThreshold: 3
    messageTTL: "5m"
  
  publisher:
    maxReconnectAttempts: 5
    maxBackoff: "1m"
    initialBackoff: "1s"
    publishTimeout: "30s"
```

### 业务层配置
```go
// evidence-management 只需要指定服务名和基础配置
func GetEventBusConfig() config.AdvancedEventBusConfig {
    return config.AdvancedEventBusConfig{
        ServiceName: "evidence-management",
        EventBusConfig: config.EventBusConfig{
            Type: "kafka",
            Kafka: config.KafkaConfig{
                Brokers:       []string{"localhost:9092"},
                ConsumerGroup: "evidence-management",
            },
        },
        HealthCheck: config.HealthCheckConfig{
            Enabled: true,
            // 其他配置使用 jxt-core 默认值
        },
    }
}
```

## 迁移步骤

### 第一步：更新配置
1. 移除业务层的健康检查主题配置
2. 添加 `serviceName` 配置
3. 使用 jxt-core 的统一健康检查配置

### 第二步：创建适配器
1. 创建 `EventPublisherAdapter`
2. 实现业务特定的消息格式化器
3. 注册业务回调函数

### 第三步：更新依赖注入
1. 注册 `AdvancedEventBus`
2. 注册适配器作为 `EventPublisher`
3. 移除旧的 `KafkaPublisherManager` 注册

### 第四步：测试验证
1. 验证发布功能正常
2. 验证健康检查使用统一主题和格式
3. 验证重连机制正常
4. 验证回调函数正常执行

## 预期收益

1. **减少重复代码 80%**：发布端技术基础设施统一实现
2. **统一健康检查**：所有微服务使用相同的健康检查机制
3. **提高可维护性**：技术组件集中维护
4. **增强监控能力**：标准化的健康检查数据
5. **保持业务灵活性**：通过适配器和回调机制

这个方案将发布端的技术基础设施完全下沉到 jxt-core，同时通过适配器模式保持业务层的灵活性，实现了技术统一和业务定制的平衡。
