# Outbox EventBus 适配器

## 📋 概述

`EventBusAdapter` 是一个真正可用的适配器，将 `jxt-core/sdk/pkg/eventbus.EventBus` 适配为 `outbox.EventPublisher` 接口。

**功能**:
- ✅ 转换 Outbox Envelope 为 EventBus Envelope
- ✅ 转换 EventBus PublishResult 为 Outbox PublishResult
- ✅ 自动启动 ACK 结果转换 goroutine
- ✅ 线程安全，支持并发调用
- ✅ 支持 Kafka 和 NATS 两种 EventBus 实现

---

## 🚀 快速开始

### 1. 使用 Kafka EventBus

```go
package main

import (
    "context"
    "log"
    
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
)

func main() {
    // 1. 创建 Kafka 配置
    kafkaConfig := &eventbus.KafkaConfig{
        Brokers:      []string{"localhost:9092"},
        ClientID:     "outbox-service",
        ConsumerGroup: "outbox-consumer-group",
    }
    
    // 2. 创建 Kafka EventBus 适配器
    adapter, err := adapters.NewKafkaEventBusAdapter(kafkaConfig)
    if err != nil {
        log.Fatalf("Failed to create adapter: %v", err)
    }
    defer adapter.Close()
    
    // 3. 创建 Outbox Repository
    repo := outbox.NewGormOutboxRepository(db)
    
    // 4. 创建 Topic Mapper
    topicMapper := outbox.NewSimpleTopicMapper(map[string]string{
        "Order":   "business.orders",
        "Payment": "business.payments",
    })
    
    // 5. 创建 Outbox Publisher
    config := &outbox.PublisherConfig{
        MaxRetries:         3,
        PublishTimeout:     30 * time.Second,
        EnableMetrics:      true,
        ConcurrentPublish:  true,
        PublishConcurrency: 10,
    }
    
    publisher := outbox.NewOutboxPublisher(repo, adapter, topicMapper, config)
    
    // 6. 启动 ACK 监听器
    ctx := context.Background()
    publisher.StartACKListener(ctx)
    defer publisher.StopACKListener()
    
    // 7. 发布事件
    event := outbox.NewOutboxEvent(
        "order-123",
        "Order",
        "OrderCreated",
        []byte(`{"orderId":"order-123","amount":99.99}`),
    )
    
    if err := publisher.PublishEvent(ctx, event); err != nil {
        log.Fatalf("Failed to publish event: %v", err)
    }
    
    log.Println("✅ Event published successfully")
    
    // ✅ ACK 结果通过 ACK 监听器异步处理
    // ✅ ACK 成功时，事件自动标记为 Published
    // ✅ ACK 失败时，事件保持 Pending，等待下次重试
}
```

### 2. 使用 NATS EventBus

```go
package main

import (
    "context"
    "log"
    
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
)

func main() {
    // 1. 创建 NATS 配置
    natsConfig := &eventbus.NATSConfig{
        URL:           "nats://localhost:4222",
        ClientID:      "outbox-service",
        ClusterID:     "test-cluster",
        DurableName:   "outbox-durable",
    }
    
    // 2. 创建 NATS EventBus 适配器
    adapter, err := adapters.NewNATSEventBusAdapter(natsConfig)
    if err != nil {
        log.Fatalf("Failed to create adapter: %v", err)
    }
    defer adapter.Close()
    
    // 3-7. 与 Kafka 示例相同
    // ...
}
```

### 3. 使用已有的 EventBus 实例

```go
package main

import (
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters"
)

func main() {
    // 1. 创建 EventBus 实例（Kafka 或 NATS）
    eventBus, err := eventbus.NewKafkaEventBus(kafkaConfig)
    if err != nil {
        panic(err)
    }
    
    // 2. 创建适配器
    adapter := adapters.NewEventBusAdapter(eventBus)
    defer adapter.Close()
    
    // 3. 使用适配器创建 Outbox Publisher
    // ...
}
```

---

## 📊 工作原理

### 架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                      Outbox Publisher                           │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  PublishEvent()                                          │  │
│  │    ↓                                                     │  │
│  │  toEnvelope()  ──→  EventPublisher.PublishEnvelope()    │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              ↓                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  ACK Listener                                            │  │
│  │    ↓                                                     │  │
│  │  GetPublishResultChannel()  ──→  handleACKResult()      │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                    EventBusAdapter                              │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  PublishEnvelope()                                       │  │
│  │    ↓                                                     │  │
│  │  toEventBusEnvelope()  ──→  EventBus.PublishEnvelope()  │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              ↓                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Result Conversion Loop                                  │  │
│  │    ↓                                                     │  │
│  │  EventBus.GetPublishResultChannel()                     │  │
│  │    ↓                                                     │  │
│  │  toOutboxPublishResult()                                │  │
│  │    ↓                                                     │  │
│  │  outboxResultChan  ──→  Outbox ACK Listener             │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                      EventBus (Kafka/NATS)                      │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  PublishEnvelope()  ──→  AsyncProducer/PublishAsync()   │  │
│  │    ↓                                                     │  │
│  │  Success/Error Handler  ──→  publishResultChan          │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 数据流

1. **发布流程**:
   ```
   OutboxEvent → Outbox.Envelope → EventBus.Envelope → Kafka/NATS
   ```

2. **ACK 流程**:
   ```
   Kafka/NATS → EventBus.PublishResult → Outbox.PublishResult → ACK Listener
   ```

---

## 🔧 API 文档

### EventBusAdapter

#### 构造函数

```go
// NewEventBusAdapter 创建 EventBus 适配器
func NewEventBusAdapter(eventBus eventbus.EventBus) *EventBusAdapter

// NewKafkaEventBusAdapter 创建 Kafka EventBus 适配器
func NewKafkaEventBusAdapter(kafkaConfig *eventbus.KafkaConfig) (*EventBusAdapter, error)

// NewNATSEventBusAdapter 创建 NATS EventBus 适配器
func NewNATSEventBusAdapter(natsConfig *eventbus.NATSConfig) (*EventBusAdapter, error)
```

#### 方法

```go
// PublishEnvelope 发布 Envelope 消息到 EventBus
func (a *EventBusAdapter) PublishEnvelope(ctx context.Context, topic string, envelope *outbox.Envelope) error

// GetPublishResultChannel 获取异步发布结果通道
func (a *EventBusAdapter) GetPublishResultChannel() <-chan *outbox.PublishResult

// Close 关闭适配器，释放资源
func (a *EventBusAdapter) Close() error

// IsStarted 检查适配器是否已启动
func (a *EventBusAdapter) IsStarted() bool

// GetEventBus 获取底层 EventBus 实例
func (a *EventBusAdapter) GetEventBus() eventbus.EventBus
```

---

## ✅ 测试

运行测试：

```bash
cd jxt-core/sdk/pkg/outbox/adapters
go test -v .
```

测试结果：

```
=== RUN   TestNewEventBusAdapter
--- PASS: TestNewEventBusAdapter (0.10s)
=== RUN   TestEventBusAdapter_PublishEnvelope
--- PASS: TestEventBusAdapter_PublishEnvelope (0.10s)
=== RUN   TestEventBusAdapter_GetPublishResultChannel_Success
--- PASS: TestEventBusAdapter_GetPublishResultChannel_Success (0.10s)
=== RUN   TestEventBusAdapter_GetPublishResultChannel_Failure
--- PASS: TestEventBusAdapter_GetPublishResultChannel_Failure (0.10s)
=== RUN   TestEventBusAdapter_Close
--- PASS: TestEventBusAdapter_Close (0.10s)
=== RUN   TestEventBusAdapter_GetEventBus
--- PASS: TestEventBusAdapter_GetEventBus (0.10s)
PASS
ok      github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters      0.614s
```

---

## 📚 相关文档

- `../ASYNC_ACK_IMPLEMENTATION_REPORT.md` - 异步 ACK 处理实施报告
- `../ASYNC_ACK_HANDLING_ANALYSIS.md` - 异步 ACK 处理分析报告
- `../../eventbus/README.md` - EventBus 文档

---

## 🎯 最佳实践

1. **使用工厂方法**: 推荐使用 `NewKafkaEventBusAdapter()` 或 `NewNATSEventBusAdapter()` 创建适配器
2. **及时关闭**: 使用 `defer adapter.Close()` 确保资源释放
3. **启动 ACK 监听器**: 必须调用 `publisher.StartACKListener(ctx)` 才能接收 ACK 结果
4. **监控指标**: 启用 `EnableMetrics` 监控发布成功率和延迟
5. **并发发布**: 启用 `ConcurrentPublish` 提升吞吐量

---

**版本**: v1.0.0  
**更新时间**: 2025-10-21  
**作者**: Augment Agent

