# Outbox 功能回归测试套件

## 概述

这是 jxt-core Outbox 组件的功能回归测试套件，用于验证 Outbox 模式的核心功能和改进特性。

## 测试文件结构

```
jxt-core/tests/outbox/function_regression_tests/
├── README.md                      # 本文档
├── test_helper.go                 # 测试辅助工具和 Mock 实现
├── uuid_generation_test.go        # UUID 生成功能测试
├── idempotency_test.go            # 幂等性功能测试
└── event_lifecycle_test.go        # 事件生命周期测试
```

## 测试覆盖

### 1. UUID 生成测试 (`uuid_generation_test.go`)

测试 UUIDv7/UUIDv4 生成机制：

- ✅ **基本生成**：验证 UUID 格式和有效性
- ✅ **唯一性**：生成 1000 个 UUID 验证无重复
- ✅ **并发安全**：100 个 goroutine 并发生成 10000 个 UUID
- ✅ **时间排序**：验证 UUIDv7 的时间排序特性
- ✅ **格式验证**：验证 UUID 格式（8-4-4-4-12）
- ✅ **性能测试**：验证 10000 个 UUID 生成性能
- ✅ **稳定性测试**：多次运行确保无 panic
- ✅ **不同聚合根**：验证不同聚合类型的 UUID 生成
- ✅ **降级机制**：验证 UUIDv7 失败时降级到 UUIDv4

**测试用例：**
- `TestUUIDGeneration_Basic`
- `TestUUIDGeneration_Uniqueness`
- `TestUUIDGeneration_Concurrent`
- `TestUUIDGeneration_TimeOrdering`
- `TestUUIDGeneration_Format`
- `TestUUIDGeneration_Performance`
- `TestUUIDGeneration_Stability`
- `TestUUIDGeneration_DifferentAggregates`
- `TestUUIDGeneration_Fallback`

### 2. 幂等性测试 (`idempotency_test.go`)

测试幂等性键生成和检查机制：

- ✅ **自动生成**：验证幂等性键自动生成
- ✅ **自定义键**：验证自定义幂等性键
- ✅ **唯一性**：验证不同事件生成不同幂等性键
- ✅ **相同事件不同 ID**：验证相同参数事件的幂等性键
- ✅ **发布器检查**：验证发布器的幂等性检查逻辑
- ✅ **不同租户**：验证不同租户的幂等性隔离
- ✅ **键格式**：验证幂等性键格式规范
- ✅ **空字段处理**：验证空字段的幂等性键生成
- ✅ **特殊字符**：验证特殊字符的幂等性键处理
- ✅ **并发发布**：验证并发发布的幂等性保证
- ✅ **批量发布**：验证批量发布的幂等性检查

**测试用例：**
- `TestIdempotency_AutoGeneration`
- `TestIdempotency_CustomKey`
- `TestIdempotency_UniqueKeys`
- `TestIdempotency_SameEventDifferentIDs`
- `TestIdempotency_PublisherCheck`
- `TestIdempotency_DifferentTenants`
- `TestIdempotency_KeyFormat`
- `TestIdempotency_EmptyFields`
- `TestIdempotency_SpecialCharacters`
- `TestIdempotency_ConcurrentPublish`
- `TestIdempotency_BatchPublish`

### 3. 事件生命周期测试 (`event_lifecycle_test.go`)

测试事件状态转换和生命周期管理：

- ✅ **初始状态**：验证事件创建时的初始状态
- ✅ **发布成功**：验证发布成功后的状态变化
- ✅ **发布失败**：验证发布失败后的状态变化
- ✅ **重试机制**：验证重试次数和状态转换
- ✅ **状态转换**：验证所有状态转换路径
- ✅ **延迟发布**：验证计划发布时间功能
- ✅ **时间戳**：验证各种时间戳字段
- ✅ **最大重试**：验证超过最大重试次数的处理
- ✅ **错误跟踪**：验证错误信息记录
- ✅ **版本跟踪**：验证事件版本管理
- ✅ **追踪和关联**：验证 TraceID 和 CorrelationID

**测试用例：**
- `TestEventLifecycle_InitialState`
- `TestEventLifecycle_PublishSuccess`
- `TestEventLifecycle_PublishFailure`
- `TestEventLifecycle_RetryMechanism`
- `TestEventLifecycle_StatusTransitions`
- `TestEventLifecycle_ScheduledPublish`
- `TestEventLifecycle_Timestamps`
- `TestEventLifecycle_MaxRetriesExceeded`
- `TestEventLifecycle_ErrorTracking`
- `TestEventLifecycle_VersionTracking`
- `TestEventLifecycle_TraceAndCorrelation`

## 测试辅助工具 (`test_helper.go`)

### TestHelper

提供常用的测试断言方法：

```go
helper := NewTestHelper(t)
helper.AssertNoError(err, "Should not error")
helper.AssertEqual(expected, actual, "Should be equal")
helper.AssertNotEmpty(value, "Should not be empty")
```

### MockRepository

模拟 Outbox 仓储，实现完整的 `OutboxRepository` 接口：

```go
repo := NewMockRepository()
repo.Save(ctx, event)
repo.FindPendingEvents(ctx, 100, "tenant1")
repo.FindByIdempotencyKey(ctx, "key")
```

**支持的方法：**
- Save, SaveBatch
- Update, BatchUpdate
- FindPendingEvents, FindByID, FindByAggregateID
- FindByIdempotencyKey, ExistsByIdempotencyKey
- MarkAsPublished, MarkAsFailed
- IncrementRetry, MarkAsMaxRetry
- Delete, DeleteBatch
- FindFailedEvents, FindMaxRetryEvents
- FindScheduledEvents
- CountPendingEvents, CountFailedEvents

### MockEventPublisher

模拟事件发布器，实现 `EventPublisher` 接口：

```go
publisher := NewMockEventPublisher()
publisher.Publish(ctx, "topic", data)
publisher.SetPublishError(err)  // 模拟发布错误
publisher.GetPublishedCount()   // 获取发布次数
```

### MockTopicMapper

模拟 Topic 映射器，实现 `TopicMapper` 接口：

```go
mapper := NewMockTopicMapper()
mapper.SetTopicMapping("Order", "order-events")
topic := mapper.GetTopic("Order")  // 返回 "order-events"
```

## 运行测试

### 运行所有测试

```bash
cd jxt-core/tests/outbox/function_regression_tests
go test -v .
```

### 运行特定测试

```bash
# UUID 生成测试
go test -v -run "TestUUIDGeneration"

# 幂等性测试
go test -v -run "TestIdempotency"

# 事件生命周期测试
go test -v -run "TestEventLifecycle"
```

### 运行单个测试用例

```bash
go test -v -run "TestUUIDGeneration_Concurrent"
```

### 性能测试

```bash
go test -v -run "TestUUIDGeneration_Performance"
```

## 测试原则

1. **独立性**：每个测试用例独立运行，不依赖其他测试
2. **可重复性**：测试结果可重复，不受外部环境影响
3. **清晰性**：测试名称清晰，测试逻辑简单
4. **完整性**：覆盖正常流程和异常流程
5. **性能**：包含性能和并发测试

## 测试数据

测试使用的示例数据：

- **租户 ID**：`tenant1`, `tenant2`
- **聚合类型**：`Order`, `User`, `Product`, `Payment`, `Inventory`
- **聚合 ID**：`order-123`, `user-456`, etc.
- **事件类型**：`OrderCreated`, `UserRegistered`, etc.

## 扩展测试

要添加新的测试用例：

1. 在相应的测试文件中添加测试函数
2. 使用 `TestHelper` 进行断言
3. 使用 Mock 对象模拟依赖
4. 遵循现有的命名规范

示例：

```go
func TestNewFeature_BasicUsage(t *testing.T) {
    helper := NewTestHelper(t)
    
    // 准备测试数据
    event := helper.CreateTestEvent("tenant1", "Order", "order-123", "OrderCreated")
    
    // 执行测试
    // ...
    
    // 验证结果
    helper.AssertNoError(err, "Should not error")
    helper.AssertEqual(expected, actual, "Should match")
}
```

## 持续集成

这些测试应该在 CI/CD 流程中自动运行：

```yaml
# .github/workflows/test.yml
- name: Run Outbox Function Tests
  run: |
    cd jxt-core/tests/outbox/function_regression_tests
    go test -v -race -coverprofile=coverage.out .
    go tool cover -html=coverage.out -o coverage.html
```

## 测试覆盖率

目标覆盖率：**>= 80%**

查看覆盖率：

```bash
go test -coverprofile=coverage.out .
go tool cover -html=coverage.out
```

## 相关文档

- [Outbox 模式设计文档](../../../sdk/pkg/outbox/outbox-pattern-design.md)
- [Outbox 改进文档](../../../sdk/pkg/outbox/IMPROVEMENTS.md)
- [Outbox API 文档](../../../sdk/pkg/outbox/)

---

**版本**: v1.0  
**更新时间**: 2025-10-21  
**作者**: Augment Agent

