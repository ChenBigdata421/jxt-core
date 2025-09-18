# EventBus - 统一事件总线组件

EventBus是jxt-core提供的统一事件总线组件，支持多种消息中间件实现，为微服务架构提供可靠的事件驱动通信能力。

## 特性

- 🚀 **多种实现**：支持Kafka、NATS、内存队列等多种消息中间件
- 🔧 **配置驱动**：通过配置文件灵活切换不同的消息中间件
- 📊 **监控友好**：内置指标收集和健康检查
- 🔄 **自动重连**：支持连接断开后的自动重连机制
- 🏗️ **DDD兼容**：完全符合领域驱动设计原则
- 🔒 **线程安全**：支持并发安全的消息发布和订阅
- 📈 **企业级特性**：支持多租户、链路追踪、指标监控

## 快速开始

### 1. 基本使用

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

### 2. 使用工厂模式

```go
// 创建事件总线工厂
cfg := &eventbus.EventBusConfig{
    Type: "memory",
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
```

### 3. 集成到SDK Runtime

```go
import "github.com/ChenBigdata421/jxt-core/sdk"

// 设置事件总线到Runtime
sdk.Runtime.SetEventBus(bus)

// 从Runtime获取事件总线
eventBus := sdk.Runtime.GetEventBus()
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

```yaml
eventbus:
  type: kafka
  kafka:
    brokers:
      - localhost:9092
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
      groupId: jxt-eventbus-group
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
```

### NATS实现配置

```yaml
eventbus:
  type: nats
  nats:
    urls:
      - nats://localhost:4222
    clusterId: jxt-cluster
    clientId: jxt-client
    durableName: jxt-durable
    maxReconnects: 10
    reconnectWait: 2s
    connectionTimeout: 10s
    healthCheckInterval: 5m
  metrics:
    enabled: true
    collectInterval: 30s
```

## 核心接口

### EventBus接口

```go
type EventBus interface {
    // 发布消息到指定主题
    Publish(ctx context.Context, topic string, message []byte) error
    
    // 订阅指定主题的消息
    Subscribe(ctx context.Context, topic string, handler MessageHandler) error
    
    // 健康检查
    HealthCheck(ctx context.Context) error
    
    // 关闭连接
    Close() error
    
    // 注册重连回调
    RegisterReconnectCallback(callback func(ctx context.Context) error) error
}
```

### MessageHandler类型

```go
type MessageHandler func(ctx context.Context, message []byte) error
```

## 高级特性

### 1. 健康检查

#### 基本使用

```go
// 执行健康检查
if err := bus.HealthCheck(ctx); err != nil {
    log.Printf("Health check failed: %v", err)
}
```

#### 业务微服务中的健康检查实现

**重要说明**：jxt-core 中的 `HealthCheck` 方法只提供基础能力，不会自动执行。业务微服务需要根据自己的需求主动调用和实现健康检查机制。

##### 1. 发布端健康检查

在业务微服务中实现定期健康检查：

```go
type PublisherHealthChecker struct {
    eventBus            eventbus.EventBus
    healthCheckInterval time.Duration
    healthCheckTopic    string
    logger              *log.Logger
    ctx                 context.Context
    cancel              context.CancelFunc
    wg                  sync.WaitGroup
}

func NewPublisherHealthChecker(bus eventbus.EventBus, interval time.Duration) *PublisherHealthChecker {
    ctx, cancel := context.WithCancel(context.Background())
    return &PublisherHealthChecker{
        eventBus:            bus,
        healthCheckInterval: interval,
        healthCheckTopic:    "health_check_topic",
        logger:              log.New(os.Stdout, "[HealthChecker] ", log.LstdFlags),
        ctx:                 ctx,
        cancel:              cancel,
    }
}

func (hc *PublisherHealthChecker) Start() {
    hc.wg.Add(1)
    go hc.healthCheckLoop()
}

func (hc *PublisherHealthChecker) healthCheckLoop() {
    defer hc.wg.Done()
    ticker := time.NewTicker(hc.healthCheckInterval)
    defer ticker.Stop()

    for {
        select {
        case <-hc.ctx.Done():
            hc.logger.Println("Health check stopped")
            return
        case <-ticker.C:
            if err := hc.performHealthCheck(); err != nil {
                hc.logger.Printf("Health check failed: %v", err)
                // 实现重连或告警逻辑
                hc.handleHealthCheckFailure(err)
            } else {
                hc.logger.Println("Health check passed")
            }
        }
    }
}

func (hc *PublisherHealthChecker) performHealthCheck() error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // 1. 检查 EventBus 连接状态
    if err := hc.eventBus.HealthCheck(ctx); err != nil {
        return fmt.Errorf("eventbus health check failed: %w", err)
    }

    // 2. 发送测试消息验证发布功能
    testMessage := []byte(fmt.Sprintf("health_check_%d", time.Now().Unix()))
    if err := hc.eventBus.Publish(ctx, hc.healthCheckTopic, testMessage); err != nil {
        return fmt.Errorf("failed to publish health check message: %w", err)
    }

    return nil
}

func (hc *PublisherHealthChecker) handleHealthCheckFailure(err error) {
    // 实现重连逻辑
    hc.logger.Printf("Attempting to recover from health check failure: %v", err)

    // 可以在这里实现：
    // 1. 重新初始化连接
    // 2. 发送告警通知
    // 3. 记录指标
    // 4. 触发熔断机制
}

func (hc *PublisherHealthChecker) Stop() {
    hc.cancel()
    hc.wg.Wait()
}
```

##### 2. 订阅端健康检查

订阅端通过监听健康检查消息来验证订阅功能：

```go
type SubscriberHealthChecker struct {
    eventBus         eventbus.EventBus
    lastMessageTime  atomic.Value // time.Time
    maxMessageAge    time.Duration
    healthCheckTopic string
    logger           *log.Logger
    ctx              context.Context
    cancel           context.CancelFunc
    wg               sync.WaitGroup
}

func NewSubscriberHealthChecker(bus eventbus.EventBus, maxAge time.Duration) *SubscriberHealthChecker {
    ctx, cancel := context.WithCancel(context.Background())
    hc := &SubscriberHealthChecker{
        eventBus:         bus,
        maxMessageAge:    maxAge,
        healthCheckTopic: "health_check_topic",
        logger:           log.New(os.Stdout, "[SubHealthChecker] ", log.LstdFlags),
        ctx:              ctx,
        cancel:           cancel,
    }
    hc.lastMessageTime.Store(time.Now())
    return hc
}

func (hc *SubscriberHealthChecker) Start() error {
    // 订阅健康检查主题
    err := hc.eventBus.Subscribe(hc.ctx, hc.healthCheckTopic, hc.handleHealthCheckMessage)
    if err != nil {
        return fmt.Errorf("failed to subscribe to health check topic: %w", err)
    }

    hc.wg.Add(1)
    go hc.monitorLoop()

    hc.logger.Printf("Started health check monitoring for topic: %s", hc.healthCheckTopic)
    return nil
}

func (hc *SubscriberHealthChecker) handleHealthCheckMessage(ctx context.Context, message []byte) error {
    hc.lastMessageTime.Store(time.Now())
    hc.logger.Printf("Received health check message: %s", string(message))
    return nil
}

func (hc *SubscriberHealthChecker) monitorLoop() {
    defer hc.wg.Done()
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-hc.ctx.Done():
            return
        case <-ticker.C:
            if err := hc.checkHealth(); err != nil {
                hc.logger.Printf("Subscriber health check failed: %v", err)
                // 实现重连或告警逻辑
            } else {
                hc.logger.Println("Subscriber health check passed")
            }
        }
    }
}

func (hc *SubscriberHealthChecker) checkHealth() error {
    // 1. 检查 EventBus 连接状态
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := hc.eventBus.HealthCheck(ctx); err != nil {
        return fmt.Errorf("eventbus connection check failed: %w", err)
    }

    // 2. 检查是否及时收到健康检查消息
    lastMsg := hc.lastMessageTime.Load().(time.Time)
    if time.Since(lastMsg) > hc.maxMessageAge {
        return fmt.Errorf("no health check message received in the last %v", hc.maxMessageAge)
    }

    return nil
}

func (hc *SubscriberHealthChecker) Stop() {
    hc.cancel()
    hc.wg.Wait()
}
```

##### 3. 集成到微服务应用

```go
func main() {
    // 初始化 EventBus
    cfg := eventbus.GetDefaultKafkaConfig([]string{"localhost:9092"})
    if err := eventbus.InitializeFromConfig(cfg); err != nil {
        log.Fatal(err)
    }
    defer eventbus.CloseGlobal()

    bus := eventbus.GetGlobal()

    // 启动发布端健康检查
    pubHealthChecker := NewPublisherHealthChecker(bus, 2*time.Minute)
    pubHealthChecker.Start()
    defer pubHealthChecker.Stop()

    // 启动订阅端健康检查
    subHealthChecker := NewSubscriberHealthChecker(bus, 5*time.Minute)
    if err := subHealthChecker.Start(); err != nil {
        log.Fatal(err)
    }
    defer subHealthChecker.Stop()

    // 应用主逻辑
    // ...

    // 优雅关闭
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    <-c
    log.Println("Shutting down gracefully...")
}
```

##### 4. HTTP 健康检查端点

为微服务提供 HTTP 健康检查端点：

```go
type HealthHandler struct {
    eventBus eventbus.EventBus
}

func (h *HealthHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()

    // 检查 EventBus 健康状态
    if err := h.eventBus.HealthCheck(ctx); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status": "unhealthy",
            "error":  err.Error(),
            "timestamp": time.Now(),
        })
        return
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "healthy",
        "timestamp": time.Now(),
    })
}

// 注册路由
func setupHealthRoutes(bus eventbus.EventBus) {
    handler := &HealthHandler{eventBus: bus}
    http.HandleFunc("/health", handler.HealthCheck)
    http.HandleFunc("/health/eventbus", handler.HealthCheck)
}
```

**关键要点**：

1. **主动调用**：jxt-core 不会自动执行健康检查，业务微服务必须主动调用
2. **分层设计**：发布端和订阅端有不同的健康检查策略
3. **故障恢复**：健康检查失败时应实现适当的恢复机制
4. **监控集成**：可以将健康检查结果暴露给监控系统
5. **配置灵活**：健康检查间隔和超时时间应该可配置

### 2. 重连回调

```go
// 注册重连回调
err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Println("EventBus reconnected, reinitializing subscriptions...")
    // 重新初始化订阅等操作
    return nil
})
```

### 3. 指标监控

EventBus内置了指标收集功能，支持以下指标：

- `MessagesPublished`: 发布的消息数量
- `MessagesConsumed`: 消费的消息数量
- `PublishErrors`: 发布错误数量
- `ConsumeErrors`: 消费错误数量
- `ConnectionErrors`: 连接错误数量
- `LastHealthCheck`: 最后一次健康检查时间
- `HealthCheckStatus`: 健康检查状态

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

### 1. DDD架构集成

在DDD架构中，建议按以下方式使用EventBus：

**领域层**：定义事件发布接口
```go
type EventPublisher interface {
    Publish(ctx context.Context, topic string, event event.Event) error
}
```

**基础设施层**：实现领域接口
```go
type EventBusPublisher struct {
    eventBus eventbus.EventBus
}

func (p *EventBusPublisher) Publish(ctx context.Context, topic string, event event.Event) error {
    payload, err := event.MarshalJSON()
    if err != nil {
        return err
    }
    return p.eventBus.Publish(ctx, topic, payload)
}
```

### 2. 错误处理

```go
// 发布时的错误处理
if err := bus.Publish(ctx, topic, message); err != nil {
    log.Printf("Failed to publish message: %v", err)
    // 实现重试逻辑或降级处理
}

// 订阅时的错误处理
err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
    if err := processMessage(message); err != nil {
        log.Printf("Failed to process message: %v", err)
        return err // 返回错误以触发重试
    }
    return nil
})
```

### 3. 优雅关闭

```go
// 在应用关闭时优雅关闭EventBus
func gracefulShutdown() {
    if err := eventbus.CloseGlobal(); err != nil {
        log.Printf("Failed to close EventBus: %v", err)
    }
}
```

## 故障排除

### 常见问题

1. **连接失败**：检查消息中间件服务是否正常运行
2. **消息丢失**：确保正确处理错误和重试机制
3. **性能问题**：调整批量大小和缓冲区配置
4. **内存泄漏**：确保正确关闭EventBus实例

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
