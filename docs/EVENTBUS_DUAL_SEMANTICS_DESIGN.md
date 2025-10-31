# EventBus 双语义设计规范

**文档版本**: v2.0  
**创建日期**: 2025-10-31  
**状态**: 正式版  
**作者**: AI Assistant  

---

## 📋 目录

1. [核心设计原则](#核心设计原则)
2. [按 Topic 类型区分](#按-topic-类型区分)
3. [路由策略](#路由策略)
4. [错误处理策略](#错误处理策略)
5. [存储策略（NATS 专用）](#存储策略nats-专用)
6. [实现对比](#实现对比)

---

## 核心设计原则

### 🎯 **设计理念**

EventBus 支持**两种消息语义**，通过 **Topic 类型**区分：

| Topic 类型 | 订阅方法 | 语义 | 适用场景 |
|-----------|---------|------|---------|
| **领域事件 Topic** | `SubscribeEnvelope` | **at-least-once** | 领域事件、关键数据 |
| **普通消息 Topic** | `Subscribe` | **at-most-once** | 通知、临时消息 |

### 🔑 **关键原则**

1. ✅ **按 Topic 类型区分**，而非按存储类型或订阅方法
2. ✅ **一个 Topic 只能有一种类型**（领域事件 OR 普通消息）
3. ✅ **由开发者保证**使用正确的订阅方法
4. ✅ **路由策略由 Topic 类型决定**（聚合ID Hash OR Round-Robin）

---

## 按 Topic 类型区分

### 1. **领域事件 Topic**

**特征**：
- ✅ **必须使用** `SubscribeEnvelope` 订阅
- ✅ **必须有聚合ID**（从 Envelope 中提取）
- ✅ **at-least-once 语义**（错误时重投）
- ✅ **保证顺序性**（同一聚合ID的事件按顺序处理）

**示例 Topic**：
```
domain.order.created
domain.payment.completed
domain.inventory.updated
event.user.registered
```

**订阅方式**：
```go
// ✅ 正确
eventBus.SubscribeEnvelope(ctx, "domain.order.created", handler)

// ❌ 错误（由开发者保证不这样做）
eventBus.Subscribe(ctx, "domain.order.created", handler)
```

**消息格式**：
```json
{
  "envelope": {
    "aggregateId": "order-12345",
    "eventType": "OrderCreated",
    "version": 1
  },
  "payload": { ... }
}
```

---

### 2. **普通消息 Topic**

**特征**：
- ✅ **使用** `Subscribe` 订阅
- ✅ **可能没有聚合ID**
- ✅ **at-most-once 语义**（错误时不重投）
- ✅ **无序并发**（性能优先）

**示例 Topic**：
```
notification.email
notification.sms
cache.invalidate
metrics.report
```

**订阅方式**：
```go
// ✅ 正确
eventBus.Subscribe(ctx, "notification.email", handler)

// ❌ 错误（由开发者保证不这样做）
eventBus.SubscribeEnvelope(ctx, "notification.email", handler)
```

**消息格式**：
```json
{
  "to": "user@example.com",
  "subject": "Welcome",
  "body": "..."
}
```

---

## 路由策略

### 📊 **路由决策表**

| Topic 类型 | 聚合ID 存在 | 路由策略 | 目的 |
|-----------|-----------|---------|------|
| **领域事件** | ✅ 有 | **聚合ID Hash** | 保证同一聚合的事件顺序处理 |
| **领域事件** | ❌ 无 | **Nak 重投** + 错误日志 | 异常情况，等待修复 |
| **普通消息** | ✅ 有 | **Round-Robin**（忽略聚合ID） | 无序并发，性能优先 |
| **普通消息** | ❌ 无 | **Round-Robin** | 无序并发，性能优先 |

### 🔧 **实现逻辑**

#### Kafka/NATS 实现

```go
func (bus *EventBus) handleMessage(wrapper *handlerWrapper, data []byte) {
    // 1️⃣ 提取聚合ID
    aggregateID, _ := ExtractAggregateID(data, ...)
    
    // 2️⃣ 根据消息类型确定路由键
    var routingKey string
    if wrapper.isEnvelope {
        // ⭐ 领域事件：必须使用聚合ID路由
        routingKey = aggregateID
        if routingKey == "" {
            // ⚠️ 异常情况：领域事件没有聚合ID
            logger.Error("Domain event missing aggregate ID", 
                zap.String("topic", topic))
            nakFunc() // Nak 重投，等待修复
            return
        }
    } else {
        // ⭐ 普通消息：总是使用 Round-Robin（忽略聚合ID）
        index := bus.roundRobinCounter.Add(1)
        routingKey = fmt.Sprintf("rr-%d", index)
    }
    
    // 3️⃣ 提交到 Actor Pool
    aggMsg := &AggregateMessage{
        AggregateID: routingKey,
        IsEnvelope:  wrapper.isEnvelope,
        // ...
    }
    bus.actorPool.ProcessMessage(ctx, aggMsg)
}
```

---

## 错误处理策略

### 📊 **错误处理决策表**

| Topic 类型 | 错误处理 | 语义 | 说明 |
|-----------|---------|------|------|
| **领域事件** | **Nak / 不 MarkMessage** | at-least-once | 消息会被重新投递 |
| **普通消息** | **Ack / MarkMessage** | at-most-once | 消息不会重投，避免重复 |

### 🔧 **实现逻辑**

#### Kafka 实现

```go
// 等待处理完成
select {
case err := <-aggMsg.Done:
    if err != nil {
        if wrapper.isEnvelope {
            // ⭐ 领域事件：不 MarkMessage，让 Kafka 重新投递
            logger.Warn("Domain event processing failed, will be redelivered",
                zap.String("topic", topic),
                zap.String("aggregateID", aggregateID),
                zap.Error(err))
            return err // 不 MarkMessage
        } else {
            // ⭐ 普通消息：MarkMessage，避免重复投递
            logger.Warn("Regular message processing failed, marking as processed",
                zap.String("topic", topic),
                zap.Error(err))
            session.MarkMessage(message, "")
            return err
        }
    }
    // 成功：MarkMessage
    session.MarkMessage(message, "")
    return nil
}
```

#### NATS 实现

```go
// 等待处理完成
select {
case err := <-aggMsg.Done:
    if err != nil {
        if wrapper.isEnvelope {
            // ⭐ 领域事件：Nak 重新投递
            logger.Warn("Domain event processing failed, will be redelivered",
                zap.String("topic", topic),
                zap.String("aggregateID", aggregateID),
                zap.Error(err))
            nakFunc()
        } else {
            // ⭐ 普通消息：Ack，避免重复投递
            logger.Warn("Regular message processing failed, marking as processed",
                zap.String("topic", topic),
                zap.Error(err))
            ackFunc()
        }
        return
    }
    // 成功：Ack
    ackFunc()
}
```

---

## 存储策略（NATS 专用）

### 📊 **NATS JetStream 存储类型**

| Topic 类型 | 存储类型 | 原因 | 性能 |
|-----------|---------|------|------|
| **领域事件** | **file**（磁盘） | 持久化，支持 at-least-once | 较慢 |
| **普通消息** | **memory**（内存） | 性能优先，at-most-once 可接受 | 快 |

### 🔧 **实现逻辑**

```go
func (n *natsEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    // ⭐ 普通消息 Topic：强制使用 memory storage
    return n.subscribeJetStreamWithStorage(ctx, topic, handler, false, nats.MemoryStorage)
}

func (n *natsEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler MessageHandler) error {
    // ⭐ 领域事件 Topic：强制使用 file storage
    return n.subscribeJetStreamWithStorage(ctx, topic, handler, true, nats.FileStorage)
}
```

**注意**：
- ⚠️ Kafka 没有存储类型的概念（所有消息都持久化到磁盘）
- ⚠️ NATS 的存储类型是性能优化，不影响语义

---

## 实现对比

### 📊 **Kafka vs NATS 实现对比**

| 特性 | Kafka | NATS |
|------|-------|------|
| **路由策略** | 领域事件=聚合ID Hash<br>普通消息=Round-Robin | 领域事件=聚合ID Hash<br>普通消息=Round-Robin |
| **错误处理** | 领域事件=不 MarkMessage<br>普通消息=MarkMessage | 领域事件=Nak<br>普通消息=Ack |
| **存储类型** | 统一磁盘存储 | 领域事件=file<br>普通消息=memory |
| **Actor Pool** | ✅ 统一使用 | ✅ 统一使用 |
| **Round-Robin** | ✅ 已实现 | ✅ 需实现 |

### 🔑 **关键差异**

1. **Kafka**：
   - 所有消息都持久化到磁盘
   - 通过 MarkMessage 控制重投

2. **NATS**：
   - 领域事件用 file storage（磁盘）
   - 普通消息用 memory storage（内存，性能优化）
   - 通过 Ack/Nak 控制重投

---

## 📝 开发者指南

### ✅ **正确使用方式**

```go
// 1️⃣ 领域事件 Topic：使用 SubscribeEnvelope
eventBus.SubscribeEnvelope(ctx, "domain.order.created", func(ctx context.Context, data []byte) error {
    // 处理领域事件
    // - 必须有聚合ID
    // - 错误会导致 Nak/不 MarkMessage（重投）
    // - 同一聚合ID的事件按顺序处理
    return nil
})

// 2️⃣ 普通消息 Topic：使用 Subscribe
eventBus.Subscribe(ctx, "notification.email", func(ctx context.Context, data []byte) error {
    // 处理普通消息
    // - 可能没有聚合ID
    // - 错误会导致 Ack/MarkMessage（不重投）
    // - 无序并发处理
    return nil
})
```

### ❌ **错误使用方式**

```go
// ❌ 错误：领域事件 Topic 使用 Subscribe
eventBus.Subscribe(ctx, "domain.order.created", handler)
// 后果：at-most-once 语义，可能丢失事件

// ❌ 错误：普通消息 Topic 使用 SubscribeEnvelope
eventBus.SubscribeEnvelope(ctx, "notification.email", handler)
// 后果：at-least-once 语义，可能重复发送通知
```

---

## 🎯 总结

1. ✅ **按 Topic 类型区分**：领域事件 vs 普通消息
2. ✅ **路由策略**：领域事件=聚合ID Hash，普通消息=Round-Robin
3. ✅ **错误处理**：领域事件=重投，普通消息=不重投
4. ✅ **存储策略**（NATS）：领域事件=file，普通消息=memory
5. ✅ **由开发者保证**使用正确的订阅方法


