# README.md EventID 修订总结

## 📋 修订概述

根据 `envelope.go` 的最新实现，EventID 已从**可选字段**改为**必填字段**（用户必须自己设定），README.md 已进行相应修订。

## 🔄 主要变更

### 1. Envelope 结构定义修订（第 902-962 行）

#### 修改前
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
```

#### 修改后
```go
type Envelope struct {
    // ========== 核心字段（必填） ==========
    EventID       string    `json:"event_id"`       // 事件唯一ID（必填，用户必须提供，用于Outbox模式）
    AggregateID   string    `json:"aggregate_id"`   // 聚合根ID
    EventType     string    `json:"event_type"`     // 事件类型
    EventVersion  int64     `json:"event_version"`  // 事件版本
    Payload       []byte    `json:"payload"`        // 事件负载
    Timestamp     time.Time `json:"timestamp"`      // 事件时间戳

    // ========== 可选字段 ==========
    TraceID       string            `json:"trace_id,omitempty"`       // 链路追踪ID
    CorrelationID string            `json:"correlation_id,omitempty"` // 关联ID
}

// 创建新的Envelope
// eventID: 事件唯一ID（必填，用户必须提供）
// aggregateID: 聚合根ID
// eventType: 事件类型
// eventVersion: 事件版本
// payload: 事件负载
func NewEnvelope(eventID, aggregateID, eventType string, eventVersion int64, payload []byte) *Envelope
```

**关键变化**:
- ✅ EventID 从"可选字段"移到"核心字段（必填）"
- ✅ `NewEnvelope()` 函数签名增加 `eventID` 作为第一个参数
- ✅ EventVersion 类型从 `int` 改为 `int64`
- ✅ 移除了 `Headers` 和 `Source` 字段（简化结构）
- ⚠️ **EventID 只能由发布用户自己填写**（不提供生成策略建议）

### 2. Outbox 模式示例修订（第 362-385 行）

#### 修改前
```go
envelope := &eventbus.Envelope{
    AggregateID:  event.AggregateID,
    EventType:    event.EventType,
    EventVersion: event.EventVersion,
    Timestamp:    event.Timestamp,
    Payload:      event.Payload,
}
```

#### 修改后
```go
// ✅ 创建 Envelope，使用 Outbox 事件的 ID 作为 EventID
envelope := &eventbus.Envelope{
    EventID:      event.ID,  // ⚠️ EventID 是必填字段，由用户提供
    AggregateID:  event.AggregateID,
    EventType:    event.EventType,
    EventVersion: event.EventVersion,
    Timestamp:    event.Timestamp,
    Payload:      event.Payload,
}
```

### 3. 订单服务示例修订（第 1584-1605 行）

#### 修改前
```go
payload, _ := json.Marshal(event)

// 创建 Envelope（包含聚合ID、事件类型、版本等元数据）
envelope := eventbus.NewEnvelope(orderID, "OrderCreated", 1, payload)
envelope.TraceID = "trace-" + orderID

// 发布到持久化主题，EventBus 自动使用 JetStream/Kafka 持久化存储
return s.eventBus.PublishEnvelope(ctx, "business.orders", envelope)
```

#### 修改后
```go
payload, _ := json.Marshal(event)

// ✅ 用户提供 EventID（必填字段）
eventID := fmt.Sprintf("%s:OrderCreated:1:%d", orderID, time.Now().UnixNano())

// 创建 Envelope（包含聚合ID、事件类型、版本等元数据）
envelope := eventbus.NewEnvelope(eventID, orderID, "OrderCreated", 1, payload)
envelope.TraceID = "trace-" + orderID

// 发布到持久化主题，EventBus 自动使用 JetStream/Kafka 持久化存储
return s.eventBus.PublishEnvelope(ctx, "business.orders", envelope)
```

### 4. Keyed-Worker 池示例修订（第 2032-2043 行）

#### 修改前
```go
payload, _ := json.Marshal(event)

// 创建Envelope（包含聚合ID，确保同一订单的事件顺序处理）
envelope := eventbus.NewEnvelope(orderID, "OrderCreated", 1, payload)
envelope.TraceID = "trace-" + orderID
```

#### 修改后
```go
payload, _ := json.Marshal(event)

// ✅ 用户提供 EventID（必填字段）
eventID := fmt.Sprintf("%s:OrderCreated:1:%d", orderID, time.Now().UnixNano())

// 创建Envelope（包含聚合ID，确保同一订单的事件顺序处理）
envelope := eventbus.NewEnvelope(eventID, orderID, "OrderCreated", 1, payload)
envelope.TraceID = "trace-" + orderID
```

### 5. 领域事件发布示例修订（第 2290-2308 行）

#### 修改前
```go
// 订单事件
orderEnv1 := eventbus.NewEnvelope("order-123", "OrderCreated", 1, orderData)
orderEnv2 := eventbus.NewEnvelope("order-123", "OrderPaid", 2, orderData)

// 用户事件
userEnv1 := eventbus.NewEnvelope("user-456", "UserRegistered", 1, userData)
userEnv2 := eventbus.NewEnvelope("user-456", "UserActivated", 2, userData)

// 库存事件
invEnv1 := eventbus.NewEnvelope("product-789", "StockAdded", 1, invData)
invEnv2 := eventbus.NewEnvelope("product-789", "StockReserved", 2, invData)
```

#### 修改后
```go
// 订单事件
orderEnv1 := eventbus.NewEnvelope("order-123:OrderCreated:1:"+fmt.Sprint(time.Now().UnixNano()), "order-123", "OrderCreated", 1, orderData)
orderEnv2 := eventbus.NewEnvelope("order-123:OrderPaid:2:"+fmt.Sprint(time.Now().UnixNano()), "order-123", "OrderPaid", 2, orderData)

// 用户事件
userEnv1 := eventbus.NewEnvelope("user-456:UserRegistered:1:"+fmt.Sprint(time.Now().UnixNano()), "user-456", "UserRegistered", 1, userData)
userEnv2 := eventbus.NewEnvelope("user-456:UserActivated:2:"+fmt.Sprint(time.Now().UnixNano()), "user-456", "UserActivated", 2, userData)

// 库存事件
invEnv1 := eventbus.NewEnvelope("product-789:StockAdded:1:"+fmt.Sprint(time.Now().UnixNano()), "product-789", "StockAdded", 1, invData)
invEnv2 := eventbus.NewEnvelope("product-789:StockReserved:2:"+fmt.Sprint(time.Now().UnixNano()), "product-789", "StockReserved", 2, invData)
```

### 6. 其他示例修订

**PublishOrderEvent 函数**（第 5688-5695 行）:
```go
// ✅ 用户提供 EventID（必填字段）
eventID := fmt.Sprintf("%s:%s:1:%d", orderID, eventType, time.Now().UnixNano())

envelope := eventbus.NewEnvelope(eventID, orderID, eventType, 1, payload)
```

**业务事件发布**（第 5770-5775 行）:
```go
// ✅ 用户提供 EventID（必填字段）
eventID := fmt.Sprintf("business-event:%s:1:%d", messageType, time.Now().UnixNano())
envelope := eventbus.NewEnvelope(eventID, "business-event", messageType, 1, data)
```

**Outbox Publisher 实现**（第 8337-8352 行）:
```go
// ✅ 用户提供 EventID（必填字段，使用 Outbox 事件的 ID）
envelope := &eventbus.Envelope{
    EventID:      event.ID,  // ⚠️ EventID 是必填字段，由用户提供
    AggregateID:  event.AggregateID,
    EventType:    event.EventType,
    EventVersion: event.EventVersion,
    Timestamp:    event.CreatedAt,
    Payload:      event.Payload,
}
```

## 📊 修订统计

| 修订类型 | 修订位置 | 修订数量 |
|---------|---------|---------|
| **Envelope 结构定义** | 第 902-934 行 | 1 处 |
| **Outbox 模式示例** | 第 362-385 行 | 1 处 |
| **订单服务示例** | 第 1584-1605 行 | 1 处 |
| **Keyed-Worker 池示例** | 第 2032-2043 行 | 1 处 |
| **领域事件发布示例** | 第 2290-2308 行 | 6 处 |
| **PublishOrderEvent 函数** | 第 5688-5695 行 | 1 处 |
| **业务事件发布** | 第 5770-5775 行 | 1 处 |
| **Outbox Publisher 实现** | 第 8337-8352 行 | 1 处 |
| **总计** | - | **14 处** |

## ✅ 修订完成检查清单

- [x] Envelope 结构定义已更新（EventID 移到核心字段）
- [x] NewEnvelope 函数签名已更新（增加 eventID 参数）
- [x] ~~新增 EventID 生成策略说明~~（已删除，EventID 只能由用户提供）
- [x] Outbox 模式示例已更新（使用 event.ID 作为 EventID）
- [x] 所有 NewEnvelope() 调用已更新（增加 eventID 参数）
- [x] 所有 Envelope 结构体初始化已更新（增加 EventID 字段）
- [x] 所有示例代码中的 EventID 由用户自己提供

## 🎯 关键要点

### 1. EventID 现在是必填字段

**之前**（可选）:
```go
envelope := eventbus.NewEnvelope(aggregateID, eventType, eventVersion, payload)
// EventID 会自动生成
```

**现在**（必填）:
```go
// ⚠️ 用户必须自己提供 EventID
eventID := "user-provided-unique-id"  // 由用户自己决定如何生成
envelope := eventbus.NewEnvelope(eventID, aggregateID, eventType, eventVersion, payload)
```

### 2. Outbox 模式使用 Outbox 事件的 ID

```go
envelope := &eventbus.Envelope{
    EventID:      event.ID,  // ⚠️ 使用 Outbox 事件的 ID 作为 EventID，由用户提供
    AggregateID:  event.AggregateID,
    EventType:    event.EventType,
    EventVersion: event.EventVersion,
    Timestamp:    event.Timestamp,
    Payload:      event.Payload,
}
```

## 🏆 总结

README.md 已完成全面修订，所有示例代码都已更新为使用**用户自定义 EventID** 的方式。主要变更包括：

1. ✅ **Envelope 结构定义更新**: EventID 从可选字段改为必填字段
2. ✅ **NewEnvelope 函数签名更新**: 增加 eventID 作为第一个参数
3. ✅ **EventID 由用户提供**: 不提供生成策略建议，完全由用户自己决定
4. ✅ **所有示例代码更新**: 14 处示例代码已全部更新
5. ✅ **Outbox 模式说明**: 明确使用 Outbox 事件的 ID 作为 EventID

**文档现在与代码实现完全一致！** 🎉

