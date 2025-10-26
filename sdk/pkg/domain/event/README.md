# Domain Event Package

## 概述

`domain/event` 包提供了企业级事件驱动架构的核心领域事件结构和工具函数。这是 jxt-core 的 DDD 共享内核（Shared Kernel）的一部分，为所有微服务提供统一的事件定义和处理标准。

## 核心组件

### 1. BaseDomainEvent（基础领域事件）

包含所有事件驱动系统都需要的核心字段，适用于所有使用事件驱动架构的系统。

**核心字段：**
- `EventID`: 事件唯一标识（UUIDv7，保证时序性）
- `EventType`: 事件类型
- `OccurredAt`: 事件发生时间
- `Version`: 事件版本
- `AggregateID`: 聚合根ID
- `AggregateType`: 聚合根类型
- `Payload`: 事件载荷

**使用示例：**

```go
import (
    jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"
)

// 创建基础事件
event := jxtevent.NewBaseDomainEvent(
    "Archive.Created",
    "archive-123",
    "Archive",
    map[string]interface{}{
        "title": "Test Archive",
    },
)
```

### 2. EnterpriseDomainEvent（企业级领域事件）

在 BaseDomainEvent 基础上增加企业级通用字段，适用于多租户 SaaS 系统和企业级应用。

**额外字段：**
- `TenantId`: 租户ID（默认为 "*"，表示全局租户）
- `CorrelationId`: 业务关联ID（用于业务流程追踪）
- `CausationId`: 因果事件ID（用于事件因果链分析）
- `TraceId`: 分布式追踪ID（集成分布式追踪系统）

**使用示例：**

```go
// 创建企业级事件
event := jxtevent.NewEnterpriseDomainEvent(
    "Archive.Created",
    "archive-123",
    "Archive",
    payload,
)

// 设置租户ID
event.SetTenantId("tenant-001")

// 设置可观测性字段
event.SetCorrelationId("workflow-123")
event.SetCausationId("trigger-event-456")
event.SetTraceId("trace-789")
```

### 3. 序列化/反序列化助手

#### 3.1 DomainEvent 序列化/反序列化

用于序列化和反序列化完整的 DomainEvent 对象。

**使用场景：**
- **Command Side**: 保存到 Outbox 表
- **Query Side**: 从消息队列接收事件

**方法列表：**
- `MarshalDomainEvent(event BaseEvent) ([]byte, error)` - 序列化完整的 DomainEvent
- `UnmarshalDomainEvent[T BaseEvent](data []byte) (T, error)` - 反序列化完整的 DomainEvent
- `MarshalDomainEventToString(event BaseEvent) (string, error)` - 序列化为 JSON 字符串
- `UnmarshalDomainEventFromString[T BaseEvent](jsonString string) (T, error)` - 从 JSON 字符串反序列化

**使用示例：**

```go
// Command Side: 序列化事件保存到 Outbox
event := jxtevent.NewEnterpriseDomainEvent("Archive.Created", "archive-123", "Archive", payload)
eventBytes, err := jxtevent.MarshalDomainEvent(event)
if err != nil {
    return fmt.Errorf("failed to marshal event: %w", err)
}
// 保存 eventBytes 到 Outbox 表

// Query Side: 从消息队列接收并反序列化
receivedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](msg.Payload)
if err != nil {
    return fmt.Errorf("failed to unmarshal event: %w", err)
}
```

#### 3.2 Payload 序列化/反序列化

用于从 DomainEvent 中提取并转换 Payload 为具体的业务结构体。

**使用场景：**
- **Query Side**: 处理具体的业务载荷

**方法列表：**
- `UnmarshalPayload[T any](ev BaseEvent) (T, error)` - 从 DomainEvent 提取并反序列化 Payload
- `MarshalPayload(payload interface{}) ([]byte, error)` - 序列化 Payload（特殊场景）

**优势：**
- ✅ 统一使用 encoding/json
- ✅ 类型安全（泛型）
- ✅ 自动处理 interface{} → map[string]interface{} → 结构体 的转换
- ✅ 支持 []byte 和 RawMessage 类型的 Payload
- ✅ 统一的错误处理
- ✅ 避免各服务重复实现

**使用示例：**

```go
// 定义 Payload 结构
type ArchiveCreatedPayload struct {
    Title     string    `json:"title"`
    CreatedBy string    `json:"createdBy"`
    CreatedAt time.Time `json:"createdAt"`
}

// Query Side: 从 DomainEvent 提取 Payload
payload, err := jxtevent.UnmarshalPayload[ArchiveCreatedPayload](domainEvent)
if err != nil {
    return fmt.Errorf("failed to unmarshal payload: %w", err)
}

// 使用 payload
fmt.Println(payload.Title)
```

**重要说明：**

当 DomainEvent 从 JSON 反序列化时，`Payload` 字段的实际类型是 `map[string]interface{}`（而不是原始的结构体类型）。`UnmarshalPayload` 会自动处理这种情况，将 map 转换为目标结构体。

```go
// 完整的端到端流程示例
// 1. Command Side: 创建事件
originalPayload := ArchiveCreatedPayload{Title: "Test", CreatedBy: "user-001"}
event := jxtevent.NewEnterpriseDomainEvent("Archive.Created", "archive-123", "Archive", originalPayload)

// 2. Command Side: 序列化保存到 Outbox
eventBytes, _ := jxtevent.MarshalDomainEvent(event)

// 3. Query Side: 从消息队列接收并反序列化
receivedEvent, _ := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](eventBytes)
// 此时 receivedEvent.Payload 的类型是 map[string]interface{}

// 4. Query Side: 提取 Payload
payload, _ := jxtevent.UnmarshalPayload[ArchiveCreatedPayload](receivedEvent)
// 现在 payload 是 ArchiveCreatedPayload 结构体
```

### 4. ValidateConsistency（一致性校验）

校验 Envelope 与 DomainEvent 的一致性，确保事件在传输过程中关键信息不被篡改或不一致。

**校验项：**
1. EventType 一致性
2. AggregateID 一致性
3. TenantId 一致性（如果是企业级事件）

**使用示例：**

```go
// 创建 Envelope
envelope := &jxtevent.Envelope{
    EventType:   event.GetEventType(),
    AggregateID: event.GetAggregateID(),
    TenantID:    event.GetTenantId(),
    Payload:     payloadBytes,
}

// 校验一致性
if err := jxtevent.ValidateConsistency(envelope, event); err != nil {
    return fmt.Errorf("consistency validation failed: %w", err)
}
```

## 接口定义

### BaseEvent 接口

所有事件都必须实现此接口：

```go
type BaseEvent interface {
    GetEventID() string
    GetEventType() string
    GetOccurredAt() time.Time
    GetVersion() int
    GetAggregateID() string
    GetAggregateType() string
    GetPayload() interface{}
}
```

### EnterpriseEvent 接口

多租户系统和需要可观测性支持的系统应实现此接口：

```go
type EnterpriseEvent interface {
    BaseEvent
    
    // 租户隔离
    GetTenantId() string
    SetTenantId(string)
    
    // 可观测性方法
    GetCorrelationId() string
    SetCorrelationId(string)
    GetCausationId() string
    SetCausationId(string)
    GetTraceId() string
    SetTraceId(string)
}
```

## 最佳实践

### 1. 直接导入，避免间接层

**✅ 推荐：**
```go
import jxtevent "github.com/ChenBigdata421/jxt-core/sdk/pkg/domain/event"

func Handle(evt *jxtevent.EnterpriseDomainEvent) error {
    // 直接使用，清晰明了
}
```

**❌ 不推荐：**
```go
// 不要创建类型别名，增加间接层
type DomainEvent = jxtevent.EnterpriseDomainEvent
```

### 2. 使用泛型助手简化代码

**✅ 推荐：**
```go
payload, err := jxtevent.UnmarshalPayload[MediaUploadedPayload](domainEvent)
if err != nil {
    return fmt.Errorf("failed to unmarshal payload: %w", err)
}
```

**❌ 不推荐：**
```go
var json = jsoniter.ConfigCompatibleWithStandardLibrary
payloadBytes, _ := json.Marshal(domainEvent.GetPayload())
var payload MediaUploadedPayload
json.Unmarshal(payloadBytes, &payload)
```

### 3. 充分利用可观测性字段

```go
event := jxtevent.NewEnterpriseDomainEvent(
    "Archive.Created",
    archiveID,
    "Archive",
    payload,
)

// 设置租户ID
event.SetTenantId(ctx.TenantID)

// 设置可观测性字段
event.SetCorrelationId(ctx.CorrelationID)  // 业务流程追踪
event.SetCausationId(triggerEventID)       // 因果事件ID
event.SetTraceId(ctx.TraceID)              // 分布式追踪ID
```

### 4. 事件命名规范

**✅ 推荐（使用过去时态）：**
- `Archive.Created`
- `Media.Uploaded`
- `Relation.Deleted`

**❌ 不推荐（命令式）：**
- `CreateArchive`
- `UploadMedia`

### 5. Payload 设计原则

**✅ 好的 Payload 设计：**
```go
type ArchiveCreatedPayload struct {
    ArchiveID   string    `json:"archiveId"`
    Title       string    `json:"title"`
    CreatedBy   string    `json:"createdBy"`
    CreatedAt   time.Time `json:"createdAt"`
}
```

**❌ 避免包含过多信息：**
```go
type ArchiveCreatedPayload struct {
    Archive     *Archive  // 整个聚合根对象，太重
    User        *User     // 关联对象，不必要
    Permissions []string  // 权限信息，不属于事件
}
```

## 测试

运行单元测试：

```bash
cd jxt-core/sdk/pkg/domain/event
go test -v
```

## 版本历史

- **v1.0.0** (2025-10-25): 初始版本
  - 实现 BaseDomainEvent
  - 实现 EnterpriseDomainEvent
  - 实现 UnmarshalPayload 泛型助手
  - 实现 ValidateConsistency 一致性校验
  - 完整的单元测试覆盖

## 相关文档

- [DomainEvent迁移到jxt-core方案](../../../../../evidence-management/docs/domain-event-migration-to-jxt-core.md)

