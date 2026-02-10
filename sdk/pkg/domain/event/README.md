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
- `TenantId`: 租户ID（类型：int，默认为 0 表示全局/无租户）
- `CorrelationId`: 业务关联ID（用于业务流程追踪）
- `CausationId`: 因果事件ID（用于事件因果链分析）
- `TraceId`: 分布式追踪ID（集成分布式追踪系统）

**TenantId 类型说明：**
- `0`: 表示系统级/无租户事件
- `1, 2, 3, ...`: 表示具体租户ID
- 与租户中间件类型一致，避免类型转换

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
event.SetTenantId(1)

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
- ✅ 统一使用 jxtjson（基于 jsoniter，性能比标准库快 2-3 倍）
- ✅ 类型安全（泛型）
- ✅ 自动处理 interface{} → map[string]interface{} → 结构体 的转换
- ✅ 支持 []byte 和 RawMessage 类型的 Payload
- ✅ 统一的错误处理
- ✅ 避免各服务重复实现
- ✅ 与 encoding/json 完全兼容

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
    GetTenantId() int
    SetTenantId(int)
    
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

## 性能特性

### JSON 序列化性能

本包使用统一的 `jxtjson` 包（基于 jsoniter），提供高性能的 JSON 序列化：

| 操作 | 性能指标 | 说明 |
|------|---------|------|
| **序列化** | ~690ns/op | 比 encoding/json 快 2-3 倍 |
| **反序列化** | ~1.2μs/op | 比 encoding/json 快 2-3 倍 |
| **大 Payload** | ~511μs (1000 字段) | 51KB JSON 数据 |
| **并发安全** | ✅ 100 goroutines | 无竞态条件 |

### 性能测试

```bash
# 运行性能基准测试
cd jxt-core/tests/domain/event/function_regression_tests
go test -run TestEnterpriseDomainEvent_PerformanceBenchmark -v

# 运行并发测试
go test -run TestEnterpriseDomainEvent_ConcurrentSerialization -v
```

## 测试

### 运行单元测试

```bash
cd jxt-core/sdk/pkg/domain/event
go test -v
```

### 运行回归测试

```bash
cd jxt-core/tests/domain/event/function_regression_tests
go test -v
```

### 测试覆盖率

- ✅ **基础功能测试**: 14 个测试用例
- ✅ **企业级事件测试**: 15 个测试用例
- ✅ **序列化测试**: 21 个测试用例
- ✅ **集成测试**: 9 个测试用例
- ✅ **Payload 测试**: 13 个测试用例
- ✅ **验证测试**: 16 个测试用例

详细测试报告：
- [序列化测试覆盖率](../../../tests/domain/event/function_regression_tests/ENTERPRISE_SERIALIZATION_TEST_COVERAGE.md)
- [序列化测试说明](../../../tests/domain/event/function_regression_tests/ENTERPRISE_SERIALIZATION_TESTS_README.md)

## 版本历史

- **v1.1.0** (2025-10-26): 序列化增强版本
  - ✅ 新增 21 个 EnterpriseDomainEvent 序列化/反序列化测试
  - ✅ 统一使用 jxtjson 包（基于 jsoniter）
  - ✅ 性能优化：序列化 ~690ns/op，反序列化 ~1.2μs/op
  - ✅ 完整的性能基准测试和并发测试
  - ✅ 与 encoding/json 完全兼容
  - ✅ 支持特殊字符（中文、Emoji、转义字符）
  - ✅ 支持大 Payload（1000+ 字段）
  - ✅ 完整的错误处理测试

- **v1.0.0** (2025-10-25): 初始版本
  - 实现 BaseDomainEvent
  - 实现 EnterpriseDomainEvent
  - 实现 UnmarshalPayload 泛型助手
  - 实现 ValidateConsistency 一致性校验
  - 完整的单元测试覆盖

## 相关文档

### 核心文档
- [序列化指南](./SERIALIZATION_GUIDE.md) - 详细的序列化使用指南
- [实现总结](./IMPLEMENTATION_SUMMARY.md) - 实现细节和设计决策
- [为什么使用 jsoniter](./WHY_JSONITER.md) - 性能分析和选型理由

### 测试文档
- [序列化测试覆盖率](../../../tests/domain/event/function_regression_tests/ENTERPRISE_SERIALIZATION_TEST_COVERAGE.md)
- [序列化测试说明](../../../tests/domain/event/function_regression_tests/ENTERPRISE_SERIALIZATION_TESTS_README.md)
- [测试覆盖率分析](../../../tests/domain/event/function_regression_tests/TEST_COVERAGE_ANALYSIS.md)

### 迁移文档
- [DomainEvent 迁移到 jxt-core 方案](../../../../../evidence-management/docs/domain-event-migration-to-jxt-core.md)
- [统一 JSON 迁移](../../UNIFIED_JSON_MIGRATION.md)

