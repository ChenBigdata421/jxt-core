# Kafka EventBus Subscribe() 迁移到 Hollywood Actor Pool - 测试计划文档

**文档版本**: v1.0  
**创建日期**: 2025-10-30  
**状态**: 待评审  
**作者**: AI Assistant  

---

## 📋 目录

1. [测试概览](#测试概览)
2. [功能测试](#功能测试)
3. [性能测试](#性能测试)
4. [可靠性测试](#可靠性测试)
5. [回归测试](#回归测试)
6. [测试环境](#测试环境)
7. [测试数据](#测试数据)

---

## 测试概览

### 🎯 **测试目标**

1. **功能正确性**: 验证消息路由、处理、提交逻辑正确
2. **性能达标**: 验证吞吐量、延迟、资源占用符合要求
3. **可靠性保证**: 验证故障恢复、自动重启机制正常
4. **回归验证**: 确保现有功能不受影响

### 📊 **测试覆盖率目标**

| 测试类型 | 目标覆盖率 | 当前覆盖率 | 状态 |
|---------|-----------|-----------|------|
| **单元测试** | 80% | 待测试 | 🟡 待执行 |
| **集成测试** | 90% | 待测试 | 🟡 待执行 |
| **性能测试** | 100% | 待测试 | 🟡 待执行 |
| **可靠性测试** | 100% | 待测试 | 🟡 待执行 |

### 🔄 **测试流程**

```
测试流程:
┌─────────────────────────────────────────────────────────┐
│ 1. 功能测试（40 分钟）                                    │
│    ├─ 基本发布订阅                                        │
│    ├─ 多消息测试                                          │
│    ├─ Envelope 测试                                      │
│    ├─ 多聚合测试                                          │
│    ├─ 路由正确性测试（新增）                               │
│    └─ 订阅互斥测试（新增）                                 │
│    ↓                                                     │
│ 2. 性能测试（1 小时）                                     │
│    ├─ 单 topic 压力测试                                   │
│    ├─ 多 topic 压力测试                                   │
│    ├─ 内存持久化性能对比                                   │
│    └─ Kafka vs NATS 性能对比                             │
│    ↓                                                     │
│ 3. 可靠性测试（30 分钟）                                  │
│    ├─ Handler panic 恢复测试                             │
│    ├─ Actor 重启测试                                      │
│    └─ Inbox 满载测试                                      │
│    ↓                                                     │
│ 4. 回归测试（30 分钟）                                    │
│    ├─ 运行所有现有测试                                     │
│    └─ 验证无功能退化                                       │
└─────────────────────────────────────────────────────────┘
```

---

## 功能测试

### 测试用例 1: 基本发布订阅

**测试目标**: 验证 Subscribe() 基本功能正常

**测试文件**: `jxt-core/tests/eventbus/function_regression_tests/kafka_nats_test.go`

**测试方法**: `TestKafkaBasicPublishSubscribe`

**测试步骤**:
1. 创建 Kafka EventBus
2. 订阅 topic
3. 发布 1 条消息
4. 验证消息接收

**预期结果**:
- ✅ 订阅成功
- ✅ 消息正确接收
- ✅ Handler 正确执行
- ✅ 无错误或 panic

**执行命令**:
```bash
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafkaBasicPublishSubscribe" -timeout 60s
```

**成功标准**:
- ✅ 测试通过
- ✅ 接收到 1 条消息
- ✅ 消息内容正确

---

### 测试用例 2: 多消息测试

**测试目标**: 验证批量消息处理正常

**测试文件**: `jxt-core/tests/eventbus/function_regression_tests/kafka_nats_test.go`

**测试方法**: `TestKafkaMultipleMessages`

**测试步骤**:
1. 创建 Kafka EventBus
2. 订阅 topic
3. 发布 10 条消息
4. 验证所有消息接收

**预期结果**:
- ✅ 所有消息正确接收
- ✅ 消息顺序正确（相同 topic）
- ✅ 无消息丢失

**执行命令**:
```bash
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafkaMultipleMessages" -timeout 60s
```

**成功标准**:
- ✅ 测试通过
- ✅ 接收到 10 条消息
- ✅ 消息内容正确

---

### 测试用例 3: Envelope 发布订阅

**测试目标**: 验证 SubscribeEnvelope() 功能不受影响

**测试文件**: `jxt-core/tests/eventbus/function_regression_tests/kafka_nats_test.go`

**测试方法**: `TestKafkaEnvelopePublishSubscribe`

**测试步骤**:
1. 创建 Kafka EventBus
2. 订阅 Envelope
3. 发布 Envelope 消息
4. 验证 Envelope 正确解析

**预期结果**:
- ✅ Envelope 正确解析
- ✅ AggregateID 正确提取
- ✅ Payload 正确解析

**执行命令**:
```bash
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafkaEnvelopePublishSubscribe" -timeout 60s
```

**成功标准**:
- ✅ 测试通过
- ✅ Envelope 字段正确
- ✅ 消息路由到正确的 Actor（基于 aggregateID）

---

### 测试用例 4: 多聚合测试

**测试目标**: 验证多个聚合的消息路由正确

**测试文件**: `jxt-core/tests/eventbus/function_regression_tests/kafka_nats_test.go`

**测试方法**: `TestKafkaMultipleAggregates`

**测试步骤**:
1. 创建 Kafka EventBus
2. 订阅 Envelope
3. 发布 5 个聚合，每个聚合 10 条消息
4. 验证所有消息接收，且每个聚合的消息顺序正确

**预期结果**:
- ✅ 所有消息正确接收（50 条）
- ✅ 每个聚合的消息顺序正确
- ✅ 不同聚合的消息可以并发处理

**执行命令**:
```bash
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafkaMultipleAggregates" -timeout 60s
```

**成功标准**:
- ✅ 测试通过
- ✅ 接收到 50 条消息
- ✅ 0 个顺序违反

---

### 测试用例 5: 负载均衡测试（新增）

**测试目标**: 验证无聚合ID消息使用 Round-Robin 轮询分发到不同 Actor

**测试文件**: `jxt-core/tests/eventbus/function_regression_tests/kafka_routing_test.go`（新增）

**测试方法**: `TestKafkaRoundRobinRouting`（新增）

**测试代码**:
```go
// TestKafkaRoundRobinRouting 测试 Round-Robin 轮询路由
func TestKafkaRoundRobinRouting(t *testing.T) {
    helper := NewTestHelper(t)
    defer helper.Cleanup()

    topic := fmt.Sprintf("test.kafka.roundrobin.%d", helper.GetTimestamp())
    helper.CreateKafkaTopics([]string{topic}, 1)

    bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-roundrobin-%d", helper.GetTimestamp()))
    defer helper.CloseEventBus(bus)

    ctx := context.Background()

    // 订阅消息
    receivedCount := &atomic.Int64{}
    err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
        receivedCount.Add(1)
        return nil
    })
    helper.AssertNoError(err, "Subscribe should not return error")

    time.Sleep(2 * time.Second)

    // 发送 100 条消息（无聚合ID）
    messageCount := 100
    for i := 0; i < messageCount; i++ {
        err := bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
        helper.AssertNoError(err, "Publish should not return error")
    }

    time.Sleep(5 * time.Second)

    // 验证所有消息都被接收
    count := receivedCount.Load()
    helper.AssertEqual(int64(messageCount), count, "Should receive all messages")

    t.Logf("✅ Kafka Round-Robin routing test passed: %d messages processed", count)
}
```

**预期结果**:
- ✅ 所有 100 条消息都被接收
- ✅ 消息通过 Round-Robin 轮询分发到不同 Actor
- ✅ 负载均衡，无单点瓶颈

**执行命令**:
```bash
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafkaRoundRobinRouting" -timeout 60s
```

**成功标准**:
- ✅ 测试通过
- ✅ 接收到所有 100 条消息
- ✅ 无消息丢失

---

### 测试用例 6: 订阅互斥测试（新增）

**测试目标**: 验证同一 topic 不能同时使用 Subscribe() 和 SubscribeEnvelope() 订阅

**测试文件**: `jxt-core/tests/eventbus/function_regression_tests/kafka_subscription_test.go`（新增）

**测试方法**: `TestKafkaSubscriptionMutualExclusion`（新增）

**测试代码**:
```go
// TestKafkaSubscriptionMutualExclusion 测试订阅互斥规则
func TestKafkaSubscriptionMutualExclusion(t *testing.T) {
    helper := NewTestHelper(t)
    defer helper.Cleanup()

    topic := fmt.Sprintf("test.kafka.subscription.mutex.%d", helper.GetTimestamp())
    helper.CreateKafkaTopics([]string{topic}, 1)

    bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-subscription-%d", helper.GetTimestamp()))
    defer helper.CloseEventBus(bus)

    ctx := context.Background()

    // 场景 1: 先 Subscribe，再 Subscribe（应该失败）
    err1 := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
        return nil
    })
    helper.AssertNoError(err1, "First Subscribe should succeed")

    err2 := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
        return nil
    })
    helper.AssertError(err2, "Second Subscribe should fail")
    helper.AssertContains(err2.Error(), "already subscribed", "Error should mention already subscribed")

    // 场景 2: 先 Subscribe，再 SubscribeEnvelope（应该失败）
    topic2 := fmt.Sprintf("test.kafka.subscription.mutex2.%d", helper.GetTimestamp())
    helper.CreateKafkaTopics([]string{topic2}, 1)

    err3 := bus.Subscribe(ctx, topic2, func(ctx context.Context, message []byte) error {
        return nil
    })
    helper.AssertNoError(err3, "Subscribe should succeed")

    err4 := bus.SubscribeEnvelope(ctx, topic2, func(ctx context.Context, envelope *Envelope) error {
        return nil
    })
    helper.AssertError(err4, "SubscribeEnvelope should fail after Subscribe")
    helper.AssertContains(err4.Error(), "already subscribed", "Error should mention already subscribed")

    // 场景 3: 先 SubscribeEnvelope，再 Subscribe（应该失败）
    topic3 := fmt.Sprintf("test.kafka.subscription.mutex3.%d", helper.GetTimestamp())
    helper.CreateKafkaTopics([]string{topic3}, 1)

    err5 := bus.SubscribeEnvelope(ctx, topic3, func(ctx context.Context, envelope *Envelope) error {
        return nil
    })
    helper.AssertNoError(err5, "SubscribeEnvelope should succeed")

    err6 := bus.Subscribe(ctx, topic3, func(ctx context.Context, message []byte) error {
        return nil
    })
    helper.AssertError(err6, "Subscribe should fail after SubscribeEnvelope")
    helper.AssertContains(err6.Error(), "already subscribed", "Error should mention already subscribed")

    t.Logf("✅ Kafka subscription mutual exclusion test passed")
}
```

**预期结果**:
- ✅ 同一 topic 的第二次 Subscribe() 调用失败
- ✅ 同一 topic 的 Subscribe() 和 SubscribeEnvelope() 互斥
- ✅ 错误消息包含 "already subscribed"

**执行命令**:
```bash
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafkaSubscriptionMutualExclusion" -timeout 60s
```

**成功标准**:
- ✅ 测试通过
- ✅ 所有重复订阅都返回错误
- ✅ 错误消息清晰明确

---

## 性能测试

### 测试用例 6: 单 Topic 压力测试

**测试目标**: 验证单 topic 高负载下的性能

**测试场景**: 单个 topic，10,000 条消息

**测试文件**: `jxt-core/tests/eventbus/performance_regression_tests/memory_persistence_comparison_test.go`

**测试方法**: `TestMemoryVsPersistenceComparison`（Extreme 场景）

**测试步骤**:
1. 创建 Kafka EventBus
2. 订阅 topic
3. 发送 10,000 条消息
4. 测量吞吐量、延迟、内存占用

**预期结果**:
- ✅ 吞吐量 ≥ 5000 msg/s
- ✅ P50 延迟 ≤ 200 ms
- ✅ P99 延迟 ≤ 500 ms
- ✅ 内存增量 ≤ 2 MB

**执行命令**:
```bash
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestMemoryVsPersistenceComparison" -timeout 600s
```

**成功标准**:
- ✅ 测试通过
- ✅ 吞吐量 ≥ 5000 msg/s
- ✅ 延迟符合要求

---

### 测试用例 7: 多 Topic 压力测试

**测试目标**: 验证多 topic 并发处理性能

**测试场景**: 10 个 topic，每个 topic 1,000 条消息

**测试文件**: `jxt-core/tests/eventbus/performance_regression_tests/kafka_nats_envelope_comparison_test.go`

**测试方法**: `TestKafkaVsNATSPerformanceComparison`（Extreme 场景）

**测试步骤**:
1. 创建 Kafka EventBus
2. 订阅 10 个 topic
3. 每个 topic 发送 1,000 条消息
4. 测量吞吐量、延迟、内存占用

**预期结果**:
- ✅ 吞吐量 ≥ 6000 msg/s
- ✅ P50 延迟 ≤ 200 ms
- ✅ P99 延迟 ≤ 500 ms
- ✅ 内存增量 ≤ 10 MB

**执行命令**:
```bash
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s
```

**成功标准**:
- ✅ 测试通过
- ✅ 吞吐量 ≥ 6000 msg/s
- ✅ 延迟符合要求

---

### 测试用例 8: 性能对比测试

**测试目标**: 对比迁移前后的性能差异

**测试方法**: 手动对比

**测试步骤**:
1. 记录迁移前的性能数据（如果有）
2. 运行迁移后的性能测试
3. 对比吞吐量、延迟、内存占用

**预期结果**:
- ✅ 吞吐量 ≥ 迁移前的 95%
- ✅ 延迟 ≤ 迁移前的 110%
- ✅ 内存占用 ≤ 迁移前的 120%

**对比指标**:

| 指标 | 迁移前（估算） | 迁移后（实测） | 变化 |
|------|--------------|--------------|------|
| **吞吐量** | ~6000 msg/s | 待测试 | 待测试 |
| **P50 延迟** | 未知 | 待测试 | 待测试 |
| **P99 延迟** | 未知 | 待测试 | 待测试 |
| **内存占用** | 未知 | 待测试 | 待测试 |

---

## 可靠性测试

### 测试用例 9: Handler Panic 恢复测试

**测试目标**: 验证 Handler panic 后 Actor 自动重启

**测试文件**: `jxt-core/tests/eventbus/reliability_tests/kafka_actor_pool_reliability_test.go`（新增）

**测试方法**: `TestKafkaActorPoolPanicRecovery`（新增）

**测试代码**:
```go
// TestKafkaActorPoolPanicRecovery 测试 Actor Pool Panic 恢复
func TestKafkaActorPoolPanicRecovery(t *testing.T) {
    helper := NewTestHelper(t)
    defer helper.Cleanup()
    
    topic := fmt.Sprintf("test.kafka.panic.%d", helper.GetTimestamp())
    helper.CreateKafkaTopics([]string{topic}, 3)
    
    bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-panic-%d", helper.GetTimestamp()))
    defer helper.CloseEventBus(bus)
    
    ctx := context.Background()
    
    var received int64
    var panicCount int64
    
    // Handler 会在前 3 次调用时 panic
    err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
        count := atomic.AddInt64(&received, 1)
        if count <= 3 {
            atomic.AddInt64(&panicCount, 1)
            panic("simulated panic")
        }
        return nil
    })
    helper.AssertNoError(err, "Subscribe should not return error")
    
    time.Sleep(2 * time.Second)
    
    // 发送 10 条消息
    for i := 0; i < 10; i++ {
        err = bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
        helper.AssertNoError(err, "Publish should not return error")
    }
    
    time.Sleep(5 * time.Second)
    
    // 验证：前 3 次 panic，后 7 次成功
    helper.AssertEqual(int64(10), atomic.LoadInt64(&received), "Should receive all 10 messages")
    helper.AssertEqual(int64(3), atomic.LoadInt64(&panicCount), "Should panic 3 times")
    
    t.Logf("✅ Actor Pool panic recovery test passed")
}
```

**预期结果**:
- ✅ Handler panic 后 Actor 自动重启
- ✅ Actor 重启后继续处理消息
- ✅ 所有消息都被处理（包括 panic 的消息）

**执行命令**:
```bash
cd jxt-core/tests/eventbus/reliability_tests
go test -v -run "TestKafkaActorPoolPanicRecovery" -timeout 60s
```

**成功标准**:
- ✅ 测试通过
- ✅ 接收到 10 条消息
- ✅ Panic 3 次

---

### 测试用例 10: Actor 重启次数限制测试

**测试目标**: 验证 Actor 重启次数不超过 maxRestarts（3次）

**测试文件**: `jxt-core/tests/eventbus/reliability_tests/kafka_actor_pool_reliability_test.go`（新增）

**测试方法**: `TestKafkaActorPoolMaxRestarts`（新增）

**测试代码**:
```go
// TestKafkaActorPoolMaxRestarts 测试 Actor Pool 最大重启次数
func TestKafkaActorPoolMaxRestarts(t *testing.T) {
    helper := NewTestHelper(t)
    defer helper.Cleanup()
    
    topic := fmt.Sprintf("test.kafka.maxrestarts.%d", helper.GetTimestamp())
    helper.CreateKafkaTopics([]string{topic}, 3)
    
    bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-maxrestarts-%d", helper.GetTimestamp()))
    defer helper.CloseEventBus(bus)
    
    ctx := context.Background()
    
    var received int64
    
    // Handler 总是 panic
    err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
        atomic.AddInt64(&received, 1)
        panic("always panic")
    })
    helper.AssertNoError(err, "Subscribe should not return error")
    
    time.Sleep(2 * time.Second)
    
    // 发送 10 条消息
    for i := 0; i < 10; i++ {
        err = bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
        helper.AssertNoError(err, "Publish should not return error")
    }
    
    time.Sleep(5 * time.Second)
    
    // 验证：Actor 重启 3 次后停止，所以最多处理 4 条消息（初始 + 3 次重启）
    count := atomic.LoadInt64(&received)
    if count > 4 {
        t.Errorf("Expected at most 4 messages processed, got %d", count)
    }
    
    t.Logf("✅ Actor Pool max restarts test passed (processed %d messages)", count)
}
```

**预期结果**:
- ✅ Actor 重启次数不超过 3 次
- ✅ 重启次数达到上限后，Actor 停止处理消息
- ✅ 消息路由到 Dead Letter Queue（如果配置）

**执行命令**:
```bash
cd jxt-core/tests/eventbus/reliability_tests
go test -v -run "TestKafkaActorPoolMaxRestarts" -timeout 60s
```

**成功标准**:
- ✅ 测试通过
- ✅ 处理的消息数 ≤ 4

---

## 回归测试

### 测试用例 11: 运行所有现有测试

**测试目标**: 确保现有功能不受影响

**测试文件**: 所有测试文件

**测试步骤**:
1. 运行所有单元测试
2. 运行所有集成测试
3. 运行所有性能测试

**执行命令**:
```bash
# 运行所有功能测试
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -timeout 600s

# 运行所有性能测试
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -timeout 1200s
```

**成功标准**:
- ✅ 所有测试通过
- ✅ 无新增失败测试
- ✅ 无性能退化

---

## 测试环境

### 🖥️ **硬件环境**

| 项目 | 配置 |
|------|------|
| **CPU** | 4 核心（最低） |
| **内存** | 8 GB（最低） |
| **磁盘** | 20 GB 可用空间 |

### 🐳 **软件环境**

| 项目 | 版本 |
|------|------|
| **Go** | 1.21+ |
| **Kafka** | 2.8.0+ |
| **Docker** | 20.10+ |
| **Docker Compose** | 1.29+ |

### 🔧 **测试工具**

| 工具 | 用途 |
|------|------|
| **go test** | 运行测试 |
| **pprof** | 性能分析 |
| **Prometheus** | 监控指标 |

---

## 测试数据

### 📊 **测试数据集**

| 数据集 | 消息数 | Topic 数 | 聚合数 | 用途 |
|--------|-------|---------|-------|------|
| **小数据集** | 10 | 1 | 1 | 功能测试 |
| **中数据集** | 100 | 3 | 5 | 集成测试 |
| **大数据集** | 10,000 | 10 | 100 | 性能测试 |

### 📝 **测试消息格式**

#### 原始消息（Subscribe）
```json
{
  "message": "test message",
  "timestamp": "2025-10-30T10:00:00Z"
}
```

#### Envelope 消息（SubscribeEnvelope）
```json
{
  "event_id": "evt-001",
  "aggregate_id": "order-123",
  "event_type": "OrderCreated",
  "event_version": 1,
  "timestamp": "2025-10-30T10:00:00Z",
  "payload": {
    "order_id": "order-123",
    "amount": 100
  }
}
```

---

## 附录

### A. 测试执行清单

```bash
# 1. 功能测试
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafkaBasicPublishSubscribe" -timeout 60s
go test -v -run "TestKafkaMultipleMessages" -timeout 60s
go test -v -run "TestKafkaEnvelopePublishSubscribe" -timeout 60s
go test -v -run "TestKafkaMultipleAggregates" -timeout 60s
go test -v -run "TestKafkaRoutingByTopic" -timeout 60s

# 2. 性能测试
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestMemoryVsPersistenceComparison" -timeout 600s
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s

# 3. 可靠性测试
cd jxt-core/tests/eventbus/reliability_tests
go test -v -run "TestKafkaActorPoolPanicRecovery" -timeout 60s
go test -v -run "TestKafkaActorPoolMaxRestarts" -timeout 60s

# 4. 回归测试
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -timeout 600s
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -timeout 1200s
```

---

**文档状态**: 待评审  
**下一步**: 等待评审批准后，开始测试执行

