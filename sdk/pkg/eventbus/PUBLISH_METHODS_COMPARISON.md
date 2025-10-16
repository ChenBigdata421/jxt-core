# Publish() vs PublishEnvelope() 方法对比说明

## 📋 概述

EventBus 提供两种消息发布方法，分别适用于不同的场景：

| 方法 | Outbox 支持 | 消息丢失容忍度 | 适用场景 |
|------|-----------|-------------|---------|
| **`PublishEnvelope()`** | ✅ **支持** | ❌ **不容许丢失** | 领域事件、关键业务事件 |
| **`Publish()`** | ❌ **不支持** | ✅ **容许丢失** | 通知、系统事件 |

## 🎯 核心区别

### 1. Outbox 模式支持

#### ✅ `PublishEnvelope()` - 支持 Outbox 模式

```go
// 发布领域事件（支持 Outbox 模式）
envelope := eventbus.NewEnvelope("order-123", "OrderCreated", 1, orderData)
err := bus.PublishEnvelope(ctx, "business.orders", envelope)

// ✅ ACK 结果会发送到 publishResultChan
resultChan := bus.GetPublishResultChannel()
for result := range resultChan {
    if result.Success {
        // 标记为已发布
        outboxRepo.MarkAsPublished(ctx, result.EventID)
    } else {
        // 记录错误，稍后重试
        outboxRepo.RecordError(ctx, result.EventID, result.Error)
    }
}
```

**内部实现**：
```go
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // 1. 序列化 Envelope
    envelopeBytes, _ := json.Marshal(envelope)
    
    // 2. 异步发布，获取 Future
    pubAckFuture, err := n.js.PublishAsync(topic, envelopeBytes)
    
    // 3. 生成 EventID
    eventID := fmt.Sprintf("%s:%s:%d:%d",
        envelope.AggregateID,
        envelope.EventType,
        envelope.EventVersion,
        envelope.Timestamp.UnixNano())
    
    // 4. 发送 ACK 任务到共享 Worker 池
    task := &ackTask{
        future:      pubAckFuture,
        eventID:     eventID,
        topic:       topic,
        aggregateID: envelope.AggregateID,
        eventType:   envelope.EventType,
    }
    
    n.ackChan <- task  // ← ✅ 发送到 ACK Worker 池
    return nil
}
```

#### ❌ `Publish()` - 不支持 Outbox 模式

```go
// 发布普通消息（不支持 Outbox 模式）
message := []byte(`{"user_id": "123", "message": "订单创建成功"}`)
err := bus.Publish(ctx, "system.notifications", message)

// ❌ 不会发送 ACK 结果到 publishResultChan
// ❌ Outbox Processor 无法知道消息是否发布成功
```

**内部实现**：
```go
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // 1. 异步发布
    _, err = n.js.PublishAsync(topic, message)
    if err != nil {
        return err
    }
    
    // 2. 立即返回
    // ❌ 不发送 ACK 任务到 Worker 池
    // ❌ 不发送 PublishResult 到 publishResultChan
    return nil
}
```

### 2. 消息结构

#### `PublishEnvelope()` - Envelope 格式

```go
type Envelope struct {
    AggregateID   string     `json:"aggregate_id"`   // 聚合ID（必填）
    EventType     string     `json:"event_type"`     // 事件类型（必填）
    EventVersion  int64      `json:"event_version"`  // 事件版本（必填）
    Timestamp     time.Time  `json:"timestamp"`      // 时间戳（必填）
    TraceID       string     `json:"trace_id"`       // 追踪ID（可选）
    CorrelationID string     `json:"correlation_id"` // 关联ID（可选）
    Payload       RawMessage `json:"payload"`        // 业务数据（必填）
}

// 示例
envelope := eventbus.NewEnvelope(
    "order-123",           // AggregateID
    "OrderCreated",        // EventType
    1,                     // EventVersion
    orderData,             // Payload
)
```

**优势**：
- ✅ 强制元数据：AggregateID、EventType、EventVersion
- ✅ 支持事件溯源：可按聚合ID重放事件
- ✅ 支持顺序处理：按 AggregateID 路由到固定 Worker
- ✅ 支持 Outbox 模式：生成唯一 EventID

#### `Publish()` - 原始字节

```go
// 任意格式的消息
message := []byte(`{"user_id": "123", "message": "Hello"}`)
bus.Publish(ctx, "notifications", message)
```

**优势**：
- ✅ 灵活：支持任意格式（JSON、Protobuf、Avro 等）
- ✅ 轻量：无需额外元数据
- ✅ 高性能：无序列化开销

### 3. EventID 生成

#### `PublishEnvelope()` - 自动生成 EventID

```go
// EventID 由以下字段组合生成：
eventID := fmt.Sprintf("%s:%s:%d:%d",
    envelope.AggregateID,      // order-123
    envelope.EventType,        // OrderCreated
    envelope.EventVersion,     // 1
    envelope.Timestamp.UnixNano()) // 1697123456789000000

// 结果：order-123:OrderCreated:1:1697123456789000000
```

**用途**：
- ✅ Outbox 表主键
- ✅ 幂等性检查
- ✅ 事件追踪

#### `Publish()` - 无 EventID

```go
// ❌ 普通消息没有 EventID
// ❌ 无法用于 Outbox 模式
// ❌ 无法追踪消息状态
```

## 📊 使用场景对比

### ✅ 使用 `PublishEnvelope()` 的场景

**不容许丢失的领域事件**：

| 场景 | 示例 | 原因 |
|------|------|------|
| **订单事件** | 订单创建、支付完成、订单取消 | 影响业务流程，必须可靠投递 |
| **支付事件** | 支付成功、退款完成 | 涉及资金，绝对不能丢失 |
| **库存事件** | 库存扣减、库存归还 | 影响库存准确性，必须可靠 |
| **用户事件** | 用户注册、账户激活 | 影响用户状态，必须可靠 |
| **审计日志** | 操作记录、合规日志 | 法律要求，必须持久化 |

**代码示例**：
```go
// 订单创建事件（不容许丢失）
func (s *OrderService) CreateOrder(ctx context.Context, order *Order) error {
    // 1. 保存订单到数据库
    if err := s.repo.Save(ctx, order); err != nil {
        return err
    }
    
    // 2. 保存事件到 Outbox 表（同一事务）
    event := &OutboxEvent{
        ID:          uuid.New().String(),
        AggregateID: order.ID,
        EventType:   "OrderCreated",
        Payload:     orderData,
        Status:      "pending",
    }
    if err := s.outboxRepo.Save(ctx, event); err != nil {
        return err
    }
    
    // 3. Outbox Processor 异步发布
    // ✅ 使用 PublishEnvelope() 支持 Outbox 模式
    envelope := eventbus.NewEnvelope(order.ID, "OrderCreated", 1, orderData)
    return s.eventBus.PublishEnvelope(ctx, "business.orders", envelope)
}
```

### ✅ 使用 `Publish()` 的场景

**容许丢失的普通消息**：

| 场景 | 示例 | 原因 |
|------|------|------|
| **通知消息** | 邮件通知、短信通知、推送通知 | 丢失不影响业务，可重发 |
| **缓存失效** | Redis 缓存失效通知 | 丢失只影响性能，不影响正确性 |
| **日志记录** | 应用日志、访问日志 | 丢失不影响业务 |
| **监控指标** | CPU 使用率、内存使用率 | 丢失不影响业务 |
| **实时状态** | 在线状态、心跳消息 | 丢失会被下次更新覆盖 |

**代码示例**：
```go
// 发送通知（容许丢失）
func (s *NotificationService) SendNotification(ctx context.Context, userID, message string) error {
    notification := map[string]string{
        "user_id": userID,
        "message": message,
        "type":    "info",
    }
    
    data, _ := json.Marshal(notification)
    
    // ✅ 使用 Publish() 即可，无需 Outbox 模式
    return s.eventBus.Publish(ctx, "system.notifications", data)
}

// 缓存失效通知（容许丢失）
func (s *CacheService) InvalidateCache(ctx context.Context, key string) error {
    message := map[string]string{
        "action": "invalidate",
        "key":    key,
    }
    
    data, _ := json.Marshal(message)
    
    // ✅ 使用 Publish() 即可，丢失只影响性能
    return s.eventBus.Publish(ctx, "cache.invalidation", data)
}
```

## 🏆 最佳实践

### 1. 选择正确的方法

```go
// ✅ 正确：领域事件使用 PublishEnvelope()
envelope := eventbus.NewEnvelope("order-123", "OrderCreated", 1, orderData)
bus.PublishEnvelope(ctx, "business.orders", envelope)

// ✅ 正确：通知消息使用 Publish()
notification := []byte(`{"user_id": "123", "message": "订单创建成功"}`)
bus.Publish(ctx, "system.notifications", notification)

// ❌ 错误：领域事件使用 Publish()（无法支持 Outbox 模式）
orderData := []byte(`{"order_id": "123", "amount": 99.99}`)
bus.Publish(ctx, "business.orders", orderData)  // ← 不支持 Outbox！
```

### 2. Outbox 模式集成

```go
// Outbox Processor 必须使用 PublishEnvelope()
type OutboxProcessor struct {
    eventBus   eventbus.EventBus
    outboxRepo OutboxRepository
}

func (p *OutboxProcessor) Start(ctx context.Context) {
    // 1. 启动结果监听器
    resultChan := p.eventBus.GetPublishResultChannel()
    go p.listenResults(ctx, resultChan)
    
    // 2. 轮询未发布事件
    ticker := time.NewTicker(5 * time.Second)
    for {
        select {
        case <-ticker.C:
            events, _ := p.outboxRepo.GetPendingEvents(ctx, 100)
            for _, event := range events {
                // ✅ 必须使用 PublishEnvelope()
                envelope := &eventbus.Envelope{
                    AggregateID:  event.AggregateID,
                    EventType:    event.EventType,
                    EventVersion: event.EventVersion,
                    Timestamp:    event.CreatedAt,
                    Payload:      event.Payload,
                }
                p.eventBus.PublishEnvelope(ctx, event.Topic, envelope)
            }
        case <-ctx.Done():
            return
        }
    }
}

func (p *OutboxProcessor) listenResults(ctx context.Context, resultChan <-chan *eventbus.PublishResult) {
    for result := range resultChan {
        if result.Success {
            // 标记为已发布
            p.outboxRepo.MarkAsPublished(ctx, result.EventID)
        } else {
            // 记录错误
            p.outboxRepo.RecordError(ctx, result.EventID, result.Error)
        }
    }
}
```

### 3. 性能测试注意事项

```go
// ⚠️ 性能测试使用 Publish() 是正确的
// 原因：性能测试不需要 Outbox 模式，通过接收端确认消息成功

func BenchmarkNATSPublish(b *testing.B) {
    bus, _ := eventbus.NewPersistentNATSEventBus(...)
    defer bus.Close()
    
    message := []byte("test message")
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // ✅ 性能测试使用 Publish() 是正确的
        bus.Publish(ctx, "benchmark.topic", message)
    }
}
```

## 📁 相关文档

- **EventBus README**: `sdk/pkg/eventbus/README.md`
- **方案2性能分析**: `tests/eventbus/performance_tests/SOLUTION2_PERFORMANCE_ANALYSIS.md`
- **接口定义**: `sdk/pkg/eventbus/type.go`
- **NATS 实现**: `sdk/pkg/eventbus/nats.go`

