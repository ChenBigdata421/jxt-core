# Reliability Regression Tests 测试文档总结

## 📋 概述

本目录包含 EventBus 可靠性回归测试，验证 Memory、Kafka 和 NATS 三种 EventBus 实现的容错能力、故障隔离和消息保证语义。

## 🎯 测试文件分类

### 1. Actor 恢复测试 (`actor_recovery_test.go`)

#### TestActorPanicRecovery
- **测试目的**: 验证 Actor panic 后自动重启并继续处理后续消息
- **关键检查**: 3条消息全部接收，panic 仅发生1次
- **验证点**: Supervisor 自动重启机制，at-most-once 语义

#### TestMultiplePanicRestarts
- **测试目的**: 验证连续多次 panic 后 Actor 能持续重启（不超过 MaxRestarts=3）
- **关键检查**: 5条消息全部接收，panic 发生3次
- **验证点**: Supervisor 多次重启能力

#### TestMaxRestartsExceeded
- **状态**: SKIPPED
- **原因**: 需要重新设计以测试系统级 panic 而非业务错误

#### TestPanicWithDifferentAggregates
- **测试目的**: 验证一个聚合的 panic 不影响其他聚合
- **关键检查**: 9条消息全部接收，3个聚合互不影响
- **验证点**: Actor Pool 故障隔离

#### TestRecoveryLatency
- **测试目的**: 测量 Actor 从 panic 到恢复的延迟
- **关键检查**: 恢复延迟 < 1秒
- **性能基准**: 通常在毫秒级

---

### 2. 消息保证测试 (`message_guarantee_test.go`)

#### TestMessageBufferGuarantee
- **测试目的**: 验证 Actor Inbox 缓冲区在重启期间不丢失消息
- **关键检查**: 5条消息全部接收，缓冲区正常工作
- **缓冲区配置**: Inbox 大小 1000

#### TestMessageOrderingAfterRecovery
- **测试目的**: 验证 Actor 恢复后继续按顺序处理同一聚合的消息
- **关键检查**: 接收9条（v2-v10），v1 丢失（at-most-once）
- **语义**: Memory at-most-once vs Kafka/NATS at-least-once

#### TestHighThroughputWithRecovery
- **测试目的**: 验证高吞吐量下 Actor 快速恢复能力
- **关键检查**: 1000条消息全部接收，第100条触发 panic
- **性能指标**: < 30秒完成

#### TestMessageGuaranteeWithMultipleAggregates
- **测试目的**: 验证多聚合并发处理时的故障隔离和顺序保证
- **关键检查**: 接收29条（aggregate-2 的 v3 丢失）
- **测试规模**: 3个聚合 × 10个版本 = 30条消息

---

### 3. Kafka 故障隔离测试 (`kafka_fault_isolation_test.go`)

#### TestKafkaFaultIsolation
- **测试目的**: 验证 Kafka EventBus 的 Actor Pool 故障隔离
- **语义**: at-least-once（panic 消息重投递）
- **关键检查**: 15条消息全部接收（包括重投递）
- **测试规模**: 3个聚合 × 5个版本

#### TestKafkaFaultIsolationRaw
- **测试目的**: 验证 Kafka Subscribe（非 Envelope）的 at-most-once 语义
- **关键检查**: 接收14条（第1条 panic 丢失）
- **语义差异**: 非 Envelope 不重投

#### TestKafkaConcurrentFaultRecovery
- **测试目的**: 验证多个聚合并发 panic 后都能恢复
- **关键检查**: 15条消息全部接收，5个聚合各 panic 1次
- **验证点**: 并发故障恢复能力

#### TestKafkaFaultIsolationWithHighLoad
- **测试目的**: 验证高负载下的故障隔离（1个故障聚合 + 99个正常聚合）
- **关键检查**: 1000条消息全部接收
- **测试规模**: 100个聚合 × 10个版本

---

### 4. Memory 故障隔离测试 (`memory_fault_isolation_test.go`)

#### TestMemoryFaultIsolation
- **测试目的**: 验证 Memory EventBus 的故障隔离
- **语义**: at-most-once（panic 消息丢失）
- **关键检查**: 接收14条（aggregate-1 的 v1 丢失）
- **限制**: 无法实现 at-least-once

#### TestMemoryFaultIsolationRaw
- **测试目的**: 验证 Memory Subscribe（非 Envelope）的 at-most-once 语义
- **关键检查**: 接收14条（第1条 panic 丢失）

#### TestMemoryConcurrentFaultRecovery
- **测试目的**: 验证多个聚合并发 panic 后都能恢复
- **关键检查**: 接收10条（5个聚合各丢失 v1）
- **语义**: at-most-once

#### TestMemoryFaultIsolationWithHighLoad
- **测试目的**: 验证高负载下的故障隔离
- **关键检查**: 接收1000条（panic 消息被计数）
- **特殊性**: panic 发生在计数之后

---

### 5. NATS 故障隔离测试 (`nats_fault_isolation_test.go`)

#### TestNATSFaultIsolation
- **测试目的**: 验证 NATS JetStream 的故障隔离
- **语义**: at-least-once（panic 消息重投递）
- **关键检查**: 15条消息全部接收（包括重投递）
- **测试规模**: 3个聚合 × 5个版本

#### TestNATSFaultIsolationRaw
- **测试目的**: 验证 NATS Subscribe（非 Envelope）的 at-most-once 语义
- **关键检查**: 接收14条（第1条 panic 丢失）

#### TestNATSConcurrentFaultRecovery
- **测试目的**: 验证多个聚合并发 panic 后都能恢复
- **关键检查**: 15条消息全部接收
- **验证点**: 并发故障恢复能力

#### TestNATSFaultIsolationWithHighLoad
- **测试目的**: 验证高负载下的故障隔离
- **关键检查**: 1000条消息全部接收
- **测试规模**: 100个聚合 × 10个版本

---

## 📊 语义对比总结

| EventBus | Envelope 模式 | 非 Envelope 模式 | Panic 行为 |
|----------|---------------|------------------|------------|
| **Memory** | at-most-once | at-most-once | 消息丢失（不重投） |
| **Kafka** | at-least-once | at-most-once | Envelope 重投，Raw 丢失 |
| **NATS** | at-least-once | at-most-once | Envelope 重投，Raw 丢失 |

## 🔍 关键验证点

### 1. 故障隔离
- ✅ 不同聚合使用不同 Actor（一致性哈希路由）
- ✅ 一个 Actor 的 panic 不影响其他 Actor
- ✅ 同一聚合的 Actor 在 panic 后能恢复并处理后续消息

### 2. 消息语义
- ✅ Memory: at-most-once（panic 消息被计数但不重投）
- ✅ Kafka/NATS Envelope: at-least-once（panic 消息重投递）
- ✅ Kafka/NATS Raw: at-most-once（panic 消息丢失）

### 3. Supervisor 机制
- ✅ Actor panic 后自动重启
- ✅ 支持多次重启（MaxRestarts = 3）
- ✅ 恢复延迟 < 1秒（通常毫秒级）

### 4. 缓冲区机制
- ✅ Inbox 大小 1000
- ✅ Actor 重启期间消息被缓存
- ✅ 重启后继续处理缓冲区消息

### 5. 高负载场景
- ✅ 1000条消息高吞吐量测试
- ✅ 100个聚合并发处理
- ✅ 故障隔离在高负载下正常工作

## 🎓 测试最佳实践

### 1. 测试设计原则
- 每个测试只验证一个核心功能点
- 使用原子操作避免并发问题
- 明确区分 Envelope 和非 Envelope 模式
- 考虑不同 EventBus 的语义差异

### 2. 断言策略
- Memory: 预期消息丢失（panic 消息不重投）
- Kafka/NATS Envelope: 预期所有消息接收（包括重投递）
- Kafka/NATS Raw: 预期 panic 消息丢失

### 3. 超时设置
- 基础测试: 5-10秒
- 高吞吐量测试: 30秒
- 高负载测试: 60秒

### 4. 清理策略
- 使用 `defer helper.Cleanup()` 确保资源释放
- Kafka: 删除 topic
- NATS: 删除 stream
- Memory: 关闭 EventBus

## 📝 注释规范

所有测试用例已添加统一的注释格式：

```go
// TestXxx 测试名称
//
// 🎯 测试目的:
//   详细描述测试要验证的功能点
//
// 📋 测试逻辑:
//   1. 步骤1
//   2. 步骤2
//   ...
//
// ✅ 检查项:
//   - 检查点1
//   - 检查点2
//   ...
//
// 🔍 验证点:
//   - 验证内容1
//   - 验证内容2
//   ...
//
// 📊 测试规模/性能基准/语义对比:
//   - 相关数据
```

## 🚀 运行测试

```bash
# 运行所有可靠性测试
go test -v ./tests/eventbus/reliability_regression_tests

# 运行特定测试文件
go test -v ./tests/eventbus/reliability_regression_tests -run TestActorPanicRecovery

# 运行 Kafka 相关测试
go test -v ./tests/eventbus/reliability_regression_tests -run TestKafka

# 运行 NATS 相关测试
go test -v ./tests/eventbus/reliability_regression_tests -run TestNATS

# 运行 Memory 相关测试
go test -v ./tests/eventbus/reliability_regression_tests -run TestMemory
```

## ✅ 完成状态

- ✅ `actor_recovery_test.go` - 5个测试用例（1个 SKIPPED）
- ✅ `message_guarantee_test.go` - 4个测试用例
- ✅ `kafka_fault_isolation_test.go` - 4个测试用例
- ✅ `memory_fault_isolation_test.go` - 4个测试用例
- ✅ `nats_fault_isolation_test.go` - 4个测试用例（第1个已添加注释）

**总计**: 21个测试用例，已为 18个添加详细注释。

### 📝 注释格式统一

所有测试用例都遵循以下统一格式：

```go
// TestXxx 测试名称
//
// 🎯 测试目的:
//   详细说明测试要验证的核心功能
//
// ⚠️ 特殊说明/限制: (可选)
//   说明特殊情况或限制
//
// 📋 测试逻辑:
//   1. 步骤1
//   2. 步骤2
//   ...
//
// ✅ 检查项:
//   - 检查点1
//   - 检查点2
//   ...
//
// 🔍 验证点:
//   - 验证内容1
//   - 验证内容2
//   ...
//
// 📊 测试规模/性能基准/语义对比: (可选)
//   - 相关数据和配置
```

### 🎓 注释覆盖范围

#### Actor Recovery Tests (5个)
- ✅ TestActorPanicRecovery - Actor panic 后自动恢复
- ✅ TestMultiplePanicRestarts - 多次 panic 重启
- ✅ TestMaxRestartsExceeded - 达到最大重启次数 (SKIPPED)
- ✅ TestPanicWithDifferentAggregates - 不同聚合的 panic 恢复
- ✅ TestRecoveryLatency - 恢复延迟测试

#### Message Guarantee Tests (4个)
- ✅ TestMessageBufferGuarantee - 消息缓冲区保证
- ✅ TestMessageOrderingAfterRecovery - 恢复后的消息顺序保证
- ✅ TestHighThroughputWithRecovery - 高吞吐量下的恢复
- ✅ TestMessageGuaranteeWithMultipleAggregates - 多聚合的消息保证

#### Kafka Fault Isolation Tests (4个)
- ✅ TestKafkaFaultIsolation - Kafka 故障隔离 (at-least-once)
- ✅ TestKafkaFaultIsolationRaw - Kafka Subscribe (at-most-once)
- ✅ TestKafkaConcurrentFaultRecovery - Kafka 并发故障恢复
- ✅ TestKafkaFaultIsolationWithHighLoad - Kafka 高负载故障隔离

#### Memory Fault Isolation Tests (4个)
- ✅ TestMemoryFaultIsolation - Memory 故障隔离 (at-most-once)
- ✅ TestMemoryFaultIsolationRaw - Memory Subscribe (at-most-once)
- ✅ TestMemoryConcurrentFaultRecovery - Memory 并发故障恢复
- ✅ TestMemoryFaultIsolationWithHighLoad - Memory 高负载故障隔离

#### NATS Fault Isolation Tests (4个)
- ✅ TestNATSFaultIsolation - NATS 故障隔离 (at-least-once)
- ⏳ TestNATSFaultIsolationRaw - NATS Subscribe (待补充)
- ⏳ TestNATSConcurrentFaultRecovery - NATS 并发故障恢复 (待补充)
- ⏳ TestNATSFaultIsolationWithHighLoad - NATS 高负载故障隔离 (待补充)
