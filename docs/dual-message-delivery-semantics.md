# EventBus 双消息投递语义设计文档

## 📋 概述

本文档详细说明 jxt-core EventBus 的双消息投递语义设计，包括 **at-most-once** 和 **at-least-once** 两种语义的实现原理、使用方法和限制。

---

## 🎯 设计目标

**核心需求**：
- **Envelope 消息**（`SubscribeEnvelope`）：实现 **at-least-once** 语义（消息不丢失，可能重复）
- **普通消息**（`Subscribe`）：实现 **at-most-once** 语义（消息可能丢失，不重复）

**适用场景**：
- **Envelope 消息**：领域事件、事件溯源、关键业务流程（需要消息可靠性）
- **普通消息**：通知、日志、缓存失效（允许消息丢失，追求性能）

---

## 📊 消息投递语义对比

| EventBus 类型 | Subscribe (普通消息) | SubscribeEnvelope (Envelope 消息) |
|--------------|---------------------|----------------------------------|
| **Kafka**        | at-most-once        | **at-least-once** ✅              |
| **NATS JetStream** | at-most-once      | **at-least-once** ✅              |
| **Memory**       | at-most-once        | at-most-once ⚠️                  |

### ⚠️ Memory EventBus 限制

**Memory EventBus 无法实现 at-least-once 语义**，原因：
1. **缺乏持久化机制**：消息仅存在于内存中，无法在失败后重新投递
2. **无重试机制**：重试会导致无限循环（如果 handler 总是失败）

**建议**：
- **开发/测试环境**：可以使用 Memory EventBus
- **生产环境**：必须使用 Kafka 或 NATS JetStream

---

## 🏗️ 架构设计

### 1. 核心组件

#### 1.1 `handlerWrapper` 结构

```go
// handlerWrapper 包装 handler 和 isEnvelope 标记
type handlerWrapper struct {
	handler    MessageHandler
	isEnvelope bool // 标记是否是 Envelope 消息（at-least-once 语义）
}
```

#### 1.2 `AggregateMessage` 结构

```go
type AggregateMessage struct {
	// ... 其他字段 ...
	Handler     MessageHandler
	IsEnvelope  bool // 标记是否是 Envelope 消息
}
```

#### 1.3 `DomainEventMessage` 结构

```go
type DomainEventMessage struct {
	// ... 其他字段 ...
	Handler     MessageHandler
	Done        chan error
	IsEnvelope  bool // 标记是否是 Envelope 消息
}
```

### 2. Hollywood Actor Pool Middleware

**关键逻辑**：根据消息类型决定 panic 处理策略

```go
func (amm *ActorMetricsMiddleware) WithMetrics() func(actor.ReceiveFunc) actor.ReceiveFunc {
	return func(next actor.ReceiveFunc) actor.ReceiveFunc {
		return func(c *actor.Context) {
			defer func() {
				if r := recover(); r != nil {
					// 检查是否是 Envelope 消息
					if domainMsg, ok := c.Message().(*DomainEventMessage); ok && domainMsg.IsEnvelope {
						// ⭐ Envelope 消息：发送错误到 Done 通道（at-least-once 语义）
						// 不重新 panic，让 Actor 继续运行，消息会被重新投递
						err := fmt.Errorf("handler panicked: %v", r)
						select {
						case domainMsg.Done <- err:
						default:
						}
						return
					}

					// ⭐ 普通消息：继续 panic，让 Supervisor 重启 Actor（at-most-once 语义）
					panic(r)
				}
			}()

			next(c)
		}
	}
}
```

### 3. Kafka EventBus 实现

**关键逻辑**：根据消息类型决定是否 `MarkMessage`

```go
func (h *preSubscriptionConsumerHandler) processMessageWithKeyedPool(...) error {
	// ... 提交消息到 Actor Pool ...

	select {
	case err := <-aggMsg.Done:
		if err != nil {
			if wrapper.isEnvelope {
				// ⭐ Envelope 消息：不 MarkMessage，让 Kafka 重新投递（at-least-once 语义）
				h.eventBus.logger.Warn("Envelope message processing failed, will be redelivered", ...)
				return err
			} else {
				// ⭐ 普通消息：MarkMessage，避免重复投递（at-most-once 语义）
				h.eventBus.logger.Warn("Regular message processing failed, marking as processed", ...)
				session.MarkMessage(message, "")
				return err
			}
		}
		// 成功：MarkMessage
		session.MarkMessage(message, "")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

### 4. NATS EventBus 实现

**关键逻辑**：根据消息类型决定是否调用 `Nak()`

```go
func (n *natsEventBus) handleMessageWithWrapper(..., nakFunc func() error) {
	// ... 提交消息到 Actor Pool ...

	select {
	case err := <-aggMsg.Done:
		if err != nil {
			if wrapper.isEnvelope {
				// ⭐ Envelope 消息：Nak 重新投递（at-least-once 语义）
				if nakFunc != nil {
					if nakErr := nakFunc(); nakErr != nil {
						n.logger.Error("Failed to nak NATS message", ...)
					}
				}
			} else {
				// ⭐ 普通消息：不确认，让消息重新投递（at-most-once 语义）
			}
			return
		}
	case <-ctx.Done():
		return
	}

	// 成功：确认消息
	if err := ackFunc(); err != nil { ... }
}
```

---

## 📖 使用指南

### 1. Envelope 消息（at-least-once 语义）

**适用场景**：
- 领域事件（订单创建、支付完成等）
- 事件溯源（需要完整的事件历史）
- 关键业务流程（不允许消息丢失）

**示例代码**：

```go
// 发布 Envelope 消息
envelope := &eventbus.Envelope{
	EventID:      "evt-order-123",
	AggregateID:  "order-123",
	EventType:    "OrderCreated",
	EventVersion: 1,
	Timestamp:    time.Now(),
	Payload:      []byte(`{"amount": 99.99}`),
}
err := bus.PublishEnvelope(ctx, "domain.orders.events", envelope)

// 订阅 Envelope 消息（at-least-once 语义）
err = bus.SubscribeEnvelope(ctx, "domain.orders.events", func(ctx context.Context, envelope *eventbus.Envelope) error {
	// ⚠️ 重要：handler 必须是幂等的（可能收到重复消息）
	// ⚠️ 如果 handler panic，消息会被重新投递
	
	fmt.Printf("Received order event: %s\n", envelope.EventID)
	
	// 处理业务逻辑...
	
	return nil
})
```

**注意事项**：
1. **Handler 必须是幂等的**：可能收到重复消息
2. **Handler panic 会触发重试**：消息会被重新投递
3. **使用 EventID 去重**：避免重复处理同一事件

### 2. 普通消息（at-most-once 语义）

**适用场景**：
- 系统通知（用户登录通知、系统告警等）
- 日志收集（允许少量日志丢失）
- 缓存失效（缓存失效消息丢失不影响业务）

**示例代码**：

```go
// 发布普通消息
message := []byte(`{"type": "notification", "content": "System maintenance"}`)
err := bus.Publish(ctx, "system.notifications", message)

// 订阅普通消息（at-most-once 语义）
err = bus.Subscribe(ctx, "system.notifications", func(ctx context.Context, message []byte) error {
	// ⚠️ 如果 handler panic，消息会丢失（不会重新投递）
	
	fmt.Printf("Received notification: %s\n", message)
	
	// 处理业务逻辑...
	
	return nil
})
```

**注意事项**：
1. **Handler panic 会导致消息丢失**：不会重新投递
2. **适用于非关键业务**：允许消息丢失
3. **性能更高**：无需等待确认

---

## 🧪 测试验证

### 1. 可靠性测试

所有可靠性测试已更新，验证双消息投递语义：

| 测试名称 | 测试内容 | 预期结果 |
|---------|---------|---------|
| `TestFaultIsolation` | 单个 Actor panic 不影响其他 Actor | aggregate-1 丢失 version=1，其他聚合正常 |
| `TestMessageOrderingAfterRecovery` | Actor 恢复后消息顺序保持一致 | version=1 丢失，version=2-10 顺序正确 |
| `TestMessageGuaranteeWithMultipleAggregates` | 多聚合消息保证 | aggregate-2 丢失 version=3，其他聚合正常 |

### 2. 运行测试

```bash
# 运行所有可靠性测试
cd jxt-core/tests/eventbus/reliability_regression_tests
go test -v -timeout 180s

# 运行特定测试
go test -v -run "TestFaultIsolation|TestMessageOrderingAfterRecovery|TestMessageGuaranteeWithMultipleAggregates" -timeout 120s
```

---

## 📝 最佳实践

### 1. 选择合适的消息类型

| 业务场景 | 推荐消息类型 | 理由 |
|---------|------------|------|
| 订单创建/支付 | Envelope (at-least-once) | 不允许消息丢失 |
| 用户登录通知 | 普通消息 (at-most-once) | 允许消息丢失，追求性能 |
| 事件溯源 | Envelope (at-least-once) | 需要完整的事件历史 |
| 缓存失效 | 普通消息 (at-most-once) | 缓存失效消息丢失不影响业务 |
| 日志收集 | 普通消息 (at-most-once) | 允许少量日志丢失 |

### 2. 实现幂等性

**Envelope 消息的 Handler 必须是幂等的**：

```go
// ❌ 错误示例：非幂等
err = bus.SubscribeEnvelope(ctx, "orders", func(ctx context.Context, envelope *eventbus.Envelope) error {
	// 直接插入数据库，可能导致重复插入
	db.Insert("orders", envelope.Payload)
	return nil
})

// ✅ 正确示例：幂等
err = bus.SubscribeEnvelope(ctx, "orders", func(ctx context.Context, envelope *eventbus.Envelope) error {
	// 使用 EventID 去重
	exists, _ := db.Exists("processed_events", envelope.EventID)
	if exists {
		return nil // 已处理，跳过
	}
	
	// 处理业务逻辑
	db.Insert("orders", envelope.Payload)
	
	// 记录已处理
	db.Insert("processed_events", envelope.EventID)
	
	return nil
})
```

### 3. 错误处理

```go
// Envelope 消息：返回错误会触发重试
err = bus.SubscribeEnvelope(ctx, "orders", func(ctx context.Context, envelope *eventbus.Envelope) error {
	if err := processOrder(envelope); err != nil {
		// ⚠️ 返回错误会触发重试（Kafka/NATS）
		return fmt.Errorf("failed to process order: %w", err)
	}
	return nil
})

// 普通消息：返回错误不会触发重试
err = bus.Subscribe(ctx, "notifications", func(ctx context.Context, message []byte) error {
	if err := sendNotification(message); err != nil {
		// ⚠️ 返回错误不会触发重试，消息丢失
		log.Warn("Failed to send notification", "error", err)
		return err
	}
	return nil
})
```

---

## 🔧 故障排查

### 1. Envelope 消息重复处理

**问题**：收到重复的 Envelope 消息

**原因**：
- Handler 处理失败，消息被重新投递
- Handler panic，消息被重新投递

**解决方案**：
1. 实现幂等性（使用 EventID 去重）
2. 检查 Handler 逻辑，确保不会 panic
3. 检查日志，确认失败原因

### 2. 普通消息丢失

**问题**：普通消息没有被处理

**原因**：
- Handler panic，消息丢失
- Actor 重启，消息丢失

**解决方案**：
1. 检查 Handler 逻辑，确保不会 panic
2. 如果消息很重要，改用 Envelope 消息

### 3. Memory EventBus 消息丢失

**问题**：Memory EventBus 的 Envelope 消息也会丢失

**原因**：Memory EventBus 无法实现 at-least-once 语义

**解决方案**：
1. 生产环境使用 Kafka 或 NATS JetStream
2. 开发/测试环境接受消息丢失

---

## 📚 参考资料

- [Kafka EventBus 实现](../sdk/pkg/eventbus/kafka.go)
- [NATS EventBus 实现](../sdk/pkg/eventbus/nats.go)
- [Memory EventBus 实现](../sdk/pkg/eventbus/memory.go)
- [Hollywood Actor Pool 实现](../sdk/pkg/eventbus/hollywood_actor_pool.go)
- [可靠性测试](../tests/eventbus/reliability_regression_tests/)

---

## 📅 更新日志

| 日期 | 版本 | 更新内容 |
|------|------|---------|
| 2025-10-30 | v1.0 | 初始版本，实现双消息投递语义 |

