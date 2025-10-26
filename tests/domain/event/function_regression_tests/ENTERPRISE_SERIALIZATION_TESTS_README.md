# EnterpriseDomainEvent 序列化/反序列化测试说明

## 📋 概述

本文档说明了 `enterprise_serialization_test.go` 文件中的测试用例，这些测试专门用于验证 `EnterpriseDomainEvent` 的 JSON 序列化和反序列化功能。

## 🎯 测试目标

验证 `EnterpriseDomainEvent` 在以下场景下的序列化/反序列化功能：

1. ✅ **基础功能** - 基本的序列化和反序列化
2. ✅ **企业级字段** - TenantId, CorrelationId, CausationId, TraceId
3. ✅ **Payload 处理** - 各种类型的 Payload（复杂、nil、大型、数组）
4. ✅ **JSON 格式** - 字段命名、嵌套结构
5. ✅ **数据完整性** - 往返序列化、时间戳、特殊字符
6. ✅ **兼容性** - 与标准 JSON 库的互操作
7. ✅ **性能** - 序列化/反序列化性能基准测试
8. ✅ **并发安全** - 多 goroutine 并发序列化
9. ✅ **错误处理** - 边界情况和异常处理
10. ✅ **多租户** - 多租户场景下的序列化

## 📁 文件结构

```
jxt-core/tests/domain/event/function_regression_tests/
├── enterprise_serialization_test.go          # 新增：序列化测试文件
├── ENTERPRISE_SERIALIZATION_TEST_COVERAGE.md # 新增：测试覆盖率报告
├── ENTERPRISE_SERIALIZATION_TESTS_README.md  # 新增：测试说明文档
├── enterprise_domain_event_test.go           # 已有：企业事件基础测试
├── base_domain_event_test.go                 # 已有：基础事件测试
├── json_serialization_test.go                # 已有：JSON 序列化测试
├── payload_helper_test.go                    # 已有：Payload 辅助测试
├── validation_test.go                        # 已有：验证测试
├── integration_test.go                       # 已有：集成测试
└── test_helper.go                            # 测试辅助工具
```

## 🧪 测试用例列表

### 1. 基础序列化/反序列化 (2 个)

#### `TestEnterpriseDomainEvent_BasicSerialization`
- **目的**: 验证基本序列化功能
- **验证点**:
  - 序列化成功
  - 生成有效的 JSON
  - 字段正确映射（eventType, aggregateId, tenantId）

#### `TestEnterpriseDomainEvent_BasicDeserialization`
- **目的**: 验证基本反序列化功能
- **验证点**:
  - 反序列化成功
  - 所有字段值匹配
  - 数据完整性保持

### 2. 企业级字段测试 (3 个)

#### `TestEnterpriseDomainEvent_AllEnterpriseFieldsSerialization`
- **目的**: 验证所有企业级字段的序列化
- **验证点**:
  - TenantId 正确序列化
  - CorrelationId 正确序列化
  - CausationId 正确序列化
  - TraceId 正确序列化

#### `TestEnterpriseDomainEvent_EmptyOptionalFields`
- **目的**: 验证空的可选字段处理
- **验证点**:
  - 空字段可以序列化
  - 反序列化后保持空值

#### `TestEnterpriseDomainEvent_OmitEmptyFields`
- **目的**: 验证 omitempty 标签功能
- **验证点**:
  - 空的可选字段不出现在 JSON 中
  - 减少序列化体积

### 3. Payload 序列化测试 (4 个)

#### `TestEnterpriseDomainEvent_PayloadSerialization`
- **目的**: 验证复杂 Payload 序列化
- **验证点**:
  - 复杂结构正确序列化
  - Payload 可以正确提取

#### `TestEnterpriseDomainEvent_NilPayloadSerialization`
- **目的**: 验证 nil Payload 处理
- **验证点**:
  - nil Payload 不会导致错误
  - 反序列化后 Payload 为 nil

#### `TestEnterpriseDomainEvent_LargePayloadSerialization`
- **目的**: 验证大 Payload 性能
- **验证点**:
  - 1000 个字段可以序列化
  - 性能在可接受范围内

#### `TestEnterpriseDomainEvent_ArrayPayloadSerialization`
- **目的**: 验证数组类型 Payload
- **验证点**:
  - 数组 Payload 正确序列化
  - 元素完整性保持

### 4. JSON 格式测试 (2 个)

#### `TestEnterpriseDomainEvent_JSONFieldNames`
- **目的**: 验证 JSON 字段命名规范
- **验证点**:
  - 使用 camelCase 命名
  - 字段名称符合规范

#### `TestEnterpriseDomainEvent_NestedStructureSerialization`
- **目的**: 验证嵌套结构序列化
- **验证点**:
  - 4 层嵌套结构正确处理
  - 结构完整性保持

### 5. 数据完整性测试 (3 个)

#### `TestEnterpriseDomainEvent_RoundTripSerialization`
- **目的**: 验证往返序列化
- **验证点**:
  - 序列化 → 反序列化 → 序列化
  - 数据不丢失

#### `TestEnterpriseDomainEvent_TimestampSerialization`
- **目的**: 验证时间戳序列化
- **验证点**:
  - 时间精度保持
  - 允许毫秒级误差

#### `TestEnterpriseDomainEvent_SpecialCharactersSerialization`
- **目的**: 验证特殊字符处理
- **验证点**:
  - 中文字符正确处理
  - Emoji 正确处理
  - 转义字符正确处理

### 6. 兼容性测试 (2 个)

#### `TestEnterpriseDomainEvent_JSONCompatibility`
- **目的**: 验证与标准 JSON 库兼容性
- **验证点**:
  - 可以用 encoding/json 反序列化
  - 可以用 encoding/json 序列化
  - 双向兼容

#### `TestEnterpriseDomainEvent_UnifiedJSONUsage`
- **目的**: 验证统一 JSON 包使用
- **验证点**:
  - jxtjson 包正常工作
  - jsoniter 性能优势

### 7. 性能测试 (2 个)

#### `TestEnterpriseDomainEvent_PerformanceBenchmark`
- **目的**: 性能基准测试
- **验证点**:
  - 10,000 次序列化性能
  - 10,000 次反序列化性能
  - 平均耗时 < 10μs

#### `TestEnterpriseDomainEvent_ConcurrentSerialization`
- **目的**: 并发安全测试
- **验证点**:
  - 100 goroutines 并发
  - 每个 100 次迭代
  - 无竞态条件

### 8. 错误处理测试 (1 个)

#### `TestEnterpriseDomainEvent_ErrorHandling`
- **目的**: 验证错误处理
- **验证点**:
  - 空数据反序列化失败
  - 无效 JSON 反序列化失败
  - nil 事件序列化失败

### 9. 多租户测试 (1 个)

#### `TestEnterpriseDomainEvent_MultiTenantSerialization`
- **目的**: 验证多租户场景
- **验证点**:
  - 10 个租户批量处理
  - 租户隔离正确
  - TenantId 正确映射

### 10. 字符串序列化测试 (1 个)

#### `TestEnterpriseDomainEvent_StringSerialization`
- **目的**: 验证字符串序列化
- **验证点**:
  - MarshalToString 正常工作
  - UnmarshalFromString 正常工作

## 🚀 运行测试

### 运行所有序列化测试

```bash
cd jxt-core
go test ./tests/domain/event/function_regression_tests/... -run "TestEnterpriseDomainEvent_.*Serialization" -v
```

### 运行单个测试

```bash
go test ./tests/domain/event/function_regression_tests/... -run TestEnterpriseDomainEvent_BasicSerialization -v
```

### 运行性能测试

```bash
go test ./tests/domain/event/function_regression_tests/... -run TestEnterpriseDomainEvent_PerformanceBenchmark -v
```

### 运行并发测试

```bash
go test ./tests/domain/event/function_regression_tests/... -run TestEnterpriseDomainEvent_ConcurrentSerialization -v
```

## 📊 测试结果示例

```bash
=== RUN   TestEnterpriseDomainEvent_BasicSerialization
--- PASS: TestEnterpriseDomainEvent_BasicSerialization (0.01s)
=== RUN   TestEnterpriseDomainEvent_AllEnterpriseFieldsSerialization
--- PASS: TestEnterpriseDomainEvent_AllEnterpriseFieldsSerialization (0.00s)
=== RUN   TestEnterpriseDomainEvent_PerformanceBenchmark
    enterprise_serialization_test.go:339: Serialization: 10000 iterations in 6.99ms (avg: 690ns per operation)
    enterprise_serialization_test.go:355: Deserialization: 10000 iterations in 112.06ms (avg: 1.205µs per operation)
--- PASS: TestEnterpriseDomainEvent_PerformanceBenchmark (0.02s)
=== RUN   TestEnterpriseDomainEvent_LargePayloadSerialization
    enterprise_serialization_test.go:388: Large payload serialization time: 511.1µs (size: 51629 bytes)
    enterprise_serialization_test.go:398: Large payload deserialization time: 0s
--- PASS: TestEnterpriseDomainEvent_LargePayloadSerialization (0.00s)

PASS
ok      github.com/ChenBigdata421/jxt-core/tests/domain/event/function_regression_tests 0.253s
```

## 🔧 使用的辅助函数

测试使用 `test_helper.go` 中的辅助函数：

- `CreateEnterpriseDomainEvent()` - 创建测试事件
- `CreateTestPayload()` - 创建简单 Payload
- `CreateComplexPayload()` - 创建复杂 Payload
- `AssertNoError()` - 断言无错误
- `AssertEqual()` - 断言相等
- `AssertNotEmpty()` - 断言非空
- `AssertContains()` - 断言包含

## 📚 相关 API

### 序列化 API

```go
// 序列化 DomainEvent 为字节数组
func MarshalDomainEvent(event BaseEvent) ([]byte, error)

// 序列化 DomainEvent 为字符串
func MarshalDomainEventToString(event BaseEvent) (string, error)
```

### 反序列化 API

```go
// 从字节数组反序列化 DomainEvent
func UnmarshalDomainEvent[T BaseEvent](data []byte) (T, error)

// 从字符串反序列化 DomainEvent
func UnmarshalDomainEventFromString[T BaseEvent](jsonString string) (T, error)

// 提取并反序列化 Payload
func UnmarshalPayload[T any](event BaseEvent) (T, error)
```

## 🎯 测试覆盖的场景

- ✅ 正常场景：标准的序列化/反序列化
- ✅ 边界场景：nil Payload、空字段、大 Payload
- ✅ 异常场景：无效 JSON、空数据、nil 事件
- ✅ 性能场景：10,000 次迭代、大 Payload
- ✅ 并发场景：100 goroutines × 100 iterations
- ✅ 兼容场景：encoding/json 互操作
- ✅ 多租户场景：10 个租户批量处理
- ✅ 特殊字符场景：中文、Emoji、转义字符

## ✅ 验收标准

所有测试必须满足以下标准：

1. ✅ **功能正确性** - 所有测试用例通过
2. ✅ **性能要求** - 序列化/反序列化 < 10μs
3. ✅ **并发安全** - 无竞态条件
4. ✅ **数据完整性** - 往返序列化数据不丢失
5. ✅ **兼容性** - 与 encoding/json 兼容
6. ✅ **错误处理** - 异常情况正确处理

## 📖 参考文档

- [序列化指南](../../../sdk/pkg/domain/event/SERIALIZATION_GUIDE.md)
- [实现总结](../../../sdk/pkg/domain/event/IMPLEMENTATION_SUMMARY.md)
- [测试覆盖率报告](./ENTERPRISE_SERIALIZATION_TEST_COVERAGE.md)
- [统一 JSON 迁移](../../../sdk/pkg/UNIFIED_JSON_MIGRATION.md)

---

**创建时间**: 2025-10-26  
**维护者**: JXT Team  
**版本**: v1.0

