# Envelope EventID 自定义支持

## 📋 概述

本文档说明了 Envelope EventID 的自定义支持功能。用户可以选择：
1. ✅ **自动生成 EventID**：由系统自动生成唯一的 EventID
2. ✅ **自定义 EventID**：用户手动指定 EventID（例如：使用 UUID、业务流水号等）

## 🎯 设计原则

### 灵活性优先

- **默认行为**：如果用户不设置 EventID，系统自动生成
- **用户优先**：如果用户设置了 EventID，系统保持不变
- **幂等性**：多次调用 `EnsureEventID()` 不会改变已存在的 EventID

### 适用场景

#### 自动生成 EventID（推荐）

适用于大多数场景：
- ✅ 简单快捷，无需手动管理
- ✅ 自动保证唯一性
- ✅ 包含时间戳信息，便于排序和调试

#### 自定义 EventID

适用于特殊场景：
- ✅ 需要使用外部生成的 UUID
- ✅ 需要与外部系统的 ID 保持一致
- ✅ 需要使用业务流水号作为 EventID
- ✅ 需要特定的 ID 格式

## 📝 实现细节

### 1. Envelope 结构

```go
type Envelope struct {
    EventID       string     `json:"event_id"`                 // 事件ID（可选，用户可自定义或自动生成）
    AggregateID   string     `json:"aggregate_id"`             // 聚合ID（必填）
    EventType     string     `json:"event_type"`               // 事件类型（必填）
    EventVersion  int64      `json:"event_version"`            // 事件版本
    Timestamp     time.Time  `json:"timestamp"`                // 时间戳
    TraceID       string     `json:"trace_id,omitempty"`       // 链路追踪ID（可选）
    CorrelationID string     `json:"correlation_id,omitempty"` // 关联ID（可选）
    Payload       RawMessage `json:"payload"`                  // 业务负载
}
```

### 2. 构造函数

#### NewEnvelope（自动生成 EventID）

```go
// NewEnvelope 创建新的消息包络
// EventID 为空时将在发布时自动生成，也可以手动设置自定义的 EventID
func NewEnvelope(aggregateID, eventType string, eventVersion int64, payload []byte) *Envelope {
    return &Envelope{
        AggregateID:  aggregateID,
        EventType:    eventType,
        EventVersion: eventVersion,
        Timestamp:    time.Now(),
        Payload:      RawMessage(payload),
    }
}
```

#### NewEnvelopeWithEventID（自定义 EventID）

```go
// NewEnvelopeWithEventID 创建新的消息包络并指定 EventID
// 用于需要自定义 EventID 的场景（例如：使用外部生成的 UUID）
func NewEnvelopeWithEventID(eventID, aggregateID, eventType string, eventVersion int64, payload []byte) *Envelope {
    return &Envelope{
        EventID:      eventID,
        AggregateID:  aggregateID,
        EventType:    eventType,
        EventVersion: eventVersion,
        Timestamp:    time.Now(),
        Payload:      RawMessage(payload),
    }
}
```

### 3. EventID 生成和确保方法

#### GenerateEventID

```go
// GenerateEventID 生成事件ID
// 格式: AggregateID:EventType:EventVersion:Timestamp.UnixNano()
// 用于 Outbox 模式的主键
func (e *Envelope) GenerateEventID() string {
    if e.Timestamp.IsZero() {
        e.Timestamp = time.Now()
    }
    return fmt.Sprintf("%s:%s:%d:%d",
        e.AggregateID,
        e.EventType,
        e.EventVersion,
        e.Timestamp.UnixNano())
}
```

#### EnsureEventID

```go
// EnsureEventID 确保 EventID 已生成
// 如果 EventID 为空，则自动生成
// 如果用户已设定 EventID，则保持不变
func (e *Envelope) EnsureEventID() {
    if e.EventID == "" {
        e.EventID = e.GenerateEventID()
    }
}
```

## 💡 使用示例

### 方式1: 自动生成 EventID（推荐）

```go
package main

import (
    "context"
    "fmt"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 创建 EventBus
    bus, _ := eventbus.NewNATSEventBus(/* ... */)
    defer bus.Close()

    // 创建 Envelope（不设置 EventID）
    envelope := eventbus.NewEnvelope(
        "order:12345",
        "OrderCreated",
        1,
        []byte(`{"amount": 100.00, "customer": "Alice"}`),
    )

    // 发布消息（EventID 会自动生成）
    err := bus.PublishEnvelope(context.Background(), "orders.events", envelope)
    if err != nil {
        panic(err)
    }

    // EventID 已自动生成
    fmt.Printf("✅ Published with auto-generated EventID: %s\n", envelope.EventID)
    // 输出: ✅ Published with auto-generated EventID: order:12345:OrderCreated:1:1760606512366291400
}
```

### 方式2: 使用 NewEnvelopeWithEventID 自定义 EventID

```go
package main

import (
    "context"
    "fmt"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/google/uuid"
)

func main() {
    // 创建 EventBus
    bus, _ := eventbus.NewNATSEventBus(/* ... */)
    defer bus.Close()

    // 生成自定义 EventID（例如：UUID）
    customEventID := uuid.New().String()

    // 使用 NewEnvelopeWithEventID 创建 Envelope
    envelope := eventbus.NewEnvelopeWithEventID(
        customEventID,  // 自定义 EventID
        "order:12345",
        "OrderCreated",
        1,
        []byte(`{"amount": 100.00, "customer": "Alice"}`),
    )

    // 发布消息（使用自定义 EventID）
    err := bus.PublishEnvelope(context.Background(), "orders.events", envelope)
    if err != nil {
        panic(err)
    }

    // EventID 保持用户设定的值
    fmt.Printf("✅ Published with custom EventID: %s\n", envelope.EventID)
    // 输出: ✅ Published with custom EventID: 550e8400-e29b-41d4-a716-446655440000
}
```

### 方式3: 手动设置 EventID

```go
package main

import (
    "context"
    "fmt"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 创建 EventBus
    bus, _ := eventbus.NewNATSEventBus(/* ... */)
    defer bus.Close()

    // 创建 Envelope
    envelope := eventbus.NewEnvelope(
        "order:12345",
        "OrderCreated",
        1,
        []byte(`{"amount": 100.00, "customer": "Alice"}`),
    )

    // 手动设置 EventID（例如：业务流水号）
    envelope.EventID = "ORDER-2025-10-16-000001"

    // 发布消息（使用手动设置的 EventID）
    err := bus.PublishEnvelope(context.Background(), "orders.events", envelope)
    if err != nil {
        panic(err)
    }

    // EventID 保持手动设置的值
    fmt.Printf("✅ Published with manual EventID: %s\n", envelope.EventID)
    // 输出: ✅ Published with manual EventID: ORDER-2025-10-16-000001
}
```

### 方式4: 混合使用（部分自动，部分自定义）

```go
package main

import (
    "context"
    "fmt"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/google/uuid"
)

func main() {
    bus, _ := eventbus.NewNATSEventBus(/* ... */)
    defer bus.Close()

    // 场景1: 普通订单事件，使用自动生成的 EventID
    envelope1 := eventbus.NewEnvelope(
        "order:12345",
        "OrderCreated",
        1,
        []byte(`{"amount": 100.00}`),
    )
    bus.PublishEnvelope(context.Background(), "orders.events", envelope1)
    fmt.Printf("Order event: %s\n", envelope1.EventID)

    // 场景2: 支付事件，需要与外部支付系统的 ID 保持一致
    externalPaymentID := uuid.New().String()
    envelope2 := eventbus.NewEnvelopeWithEventID(
        externalPaymentID,
        "payment:67890",
        "PaymentCompleted",
        1,
        []byte(`{"amount": 100.00, "paymentId": "` + externalPaymentID + `"}`),
    )
    bus.PublishEnvelope(context.Background(), "payments.events", envelope2)
    fmt.Printf("Payment event: %s\n", envelope2.EventID)
}
```

## 🔍 EventID 格式对比

### 自动生成格式

**格式**: `AggregateID:EventType:EventVersion:Timestamp.UnixNano()`

**示例**:
```
order:12345:OrderCreated:1:1760606512366291400
└─────┬────┘└────┬─────┘└┬┘└────────┬──────────┘
  AggregateID  EventType  │      Timestamp
                      EventVersion  (纳秒)
```

**优点**:
- ✅ 自动保证唯一性
- ✅ 包含时间戳，便于排序
- ✅ 包含业务信息，便于调试
- ✅ 无需额外依赖

**缺点**:
- ⚠️ 格式固定，不够灵活
- ⚠️ 长度较长

### 自定义格式

**示例**:
```
550e8400-e29b-41d4-a716-446655440000  (UUID)
ORDER-2025-10-16-000001               (业务流水号)
evt_1234567890                        (自定义前缀)
```

**优点**:
- ✅ 格式灵活，可以使用任意字符串
- ✅ 可以与外部系统保持一致
- ✅ 可以使用标准 UUID
- ✅ 可以使用业务流水号

**缺点**:
- ⚠️ 需要用户保证唯一性
- ⚠️ 可能需要额外依赖（例如：UUID 库）

## 🧪 测试验证

### 测试用例

```go
// 测试1: 自动生成 EventID
func TestAutoGenerateEventID(t *testing.T) {
    envelope := NewEnvelope("order:12345", "OrderCreated", 1, []byte(`{}`))
    envelope.EnsureEventID()
    
    if envelope.EventID == "" {
        t.Error("EventID should be auto-generated")
    }
}

// 测试2: 用户自定义 EventID（使用 NewEnvelopeWithEventID）
func TestCustomEventIDWithConstructor(t *testing.T) {
    customID := "my-custom-id"
    envelope := NewEnvelopeWithEventID(customID, "order:12345", "OrderCreated", 1, []byte(`{}`))
    
    if envelope.EventID != customID {
        t.Errorf("EventID should be %s, got %s", customID, envelope.EventID)
    }
    
    // EnsureEventID 不应该改变用户设定的 EventID
    envelope.EnsureEventID()
    if envelope.EventID != customID {
        t.Error("EnsureEventID should not change user-defined EventID")
    }
}

// 测试3: 用户自定义 EventID（手动设置）
func TestCustomEventIDManual(t *testing.T) {
    envelope := NewEnvelope("order:12345", "OrderCreated", 1, []byte(`{}`))
    customID := "my-custom-id"
    envelope.EventID = customID
    
    envelope.EnsureEventID()
    if envelope.EventID != customID {
        t.Error("EnsureEventID should not change existing EventID")
    }
}
```

### 测试结果

```bash
$ go test -v -run "^TestEnvelopeEnsureEventID" ./sdk/pkg/eventbus

=== RUN   TestEnvelopeEnsureEventID
=== RUN   TestEnvelopeEnsureEventID/EventID_为空时自动生成
    envelope_eventid_test.go:87: Generated EventID: order:12345:OrderCreated:1:1760606512366291400
=== RUN   TestEnvelopeEnsureEventID/EventID_已存在时不重新生成
    envelope_eventid_test.go:105: EventID unchanged: custom:event:id:123
=== RUN   TestEnvelopeEnsureEventID/使用_NewEnvelopeWithEventID_创建自定义_EventID
    envelope_eventid_test.go:129: Custom EventID preserved: my-custom-event-id-12345
=== RUN   TestEnvelopeEnsureEventID/多次调用_EnsureEventID_应该幂等
    envelope_eventid_test.go:148: EventID consistent: order:12345:OrderCreated:1:1760606512366291400
--- PASS: TestEnvelopeEnsureEventID (0.00s)
PASS
```

## 🏆 总结

### 实现成果

1. ✅ **灵活的 EventID 管理**: 支持自动生成和用户自定义
2. ✅ **新增构造函数**: `NewEnvelopeWithEventID()` 用于自定义 EventID
3. ✅ **幂等性保证**: `EnsureEventID()` 不会改变已存在的 EventID
4. ✅ **向后兼容**: 现有代码无需修改，默认行为保持不变
5. ✅ **测试覆盖**: 所有场景都有测试覆盖

### 使用建议

1. **默认使用自动生成**: 对于大多数场景，使用 `NewEnvelope()` 并让系统自动生成 EventID
2. **特殊场景使用自定义**: 只在需要与外部系统集成或有特殊要求时使用自定义 EventID
3. **保证唯一性**: 如果使用自定义 EventID，务必保证其唯一性（例如：使用 UUID）
4. **一致性**: 在同一个项目中，尽量保持 EventID 格式的一致性

### 相关文档

- **Envelope 实现**: `sdk/pkg/eventbus/envelope.go`
- **EventID 测试**: `sdk/pkg/eventbus/envelope_eventid_test.go`
- **EventID 实现总结**: `sdk/pkg/eventbus/ENVELOPE_EVENTID_IMPLEMENTATION.md`

