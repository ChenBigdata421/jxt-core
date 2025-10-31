# NATS JetStream Subscribe() 迁移到 Hollywood Actor Pool - 测试计划文档

**文档版本**: v1.0  
**创建日期**: 2025-10-30  
**状态**: 待评审  
**作者**: AI Assistant  

---

## 📋 目录

1. [执行摘要](#执行摘要)
2. [测试策略](#测试策略)
3. [功能测试](#功能测试)
4. [性能测试](#性能测试)
5. [可靠性测试](#可靠性测试)
6. [回归测试](#回归测试)
7. [测试执行计划](#测试执行计划)

---

## 执行摘要

### 🎯 **测试目标**

验证 NATS JetStream EventBus 从**全局 Worker Pool**迁移到 **Hollywood Actor Pool** 后的功能正确性、性能达标和可靠性保证。

### 📊 **测试范围**

| 测试类型 | 测试用例数 | 预计时间 |
|---------|-----------|---------|
| **功能测试** | 5 | 1 小时 |
| **性能测试** | 3 | 1 小时 |
| **可靠性测试** | 3 | 0.5 小时 |
| **回归测试** | 全部 | 0.5 小时 |
| **总计** | 11+ | 3 小时 |

---

## 测试策略

### 🔍 **测试原则**

1. **全面覆盖**: 覆盖所有关键功能和场景
2. **性能基准**: 与迁移前性能对比
3. **可靠性验证**: 验证故障恢复机制
4. **回归保护**: 确保现有功能不受影响

### 📊 **成功标准**

#### 1. 功能正确性

- ✅ 所有现有测试通过（4/4 NATS 单元测试）
- ✅ 消息正确路由和处理
- ✅ 无消息丢失
- ✅ 错误处理正确

#### 2. 性能达标

- ✅ 吞吐量 ≥ 当前水平（~162 msg/s）
- ✅ 延迟 ≤ 当前水平（~1010 ms）
- ✅ 内存占用 ≤ 当前水平（~3.14 MB）
- ✅ 协程数稳定（无泄漏）

#### 3. 可靠性保证

- ✅ Actor panic 自动重启
- ✅ 故障隔离正常
- ✅ 监控指标正确

---

## 功能测试

### 📝 **测试用例清单**

#### **TC-F1: 有聚合ID的消息路由**

**目标**: 验证有聚合ID的消息正确路由到 Actor Pool

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化

**测试步骤**:
1. 发布 Envelope 消息（包含聚合ID）
2. 订阅消息并验证接收
3. 验证消息路由到正确的 Actor

**预期结果**:
- ✅ 消息成功接收
- ✅ 同一聚合ID的消息路由到同一个 Actor
- ✅ 消息顺序正确

**测试命令**:
```bash
cd jxt-core/sdk/pkg/eventbus
go test -v -run "TestNATSActorPool" -timeout 60s
```

---

#### **TC-F2: 无聚合ID的消息路由**

**目标**: 验证无聚合ID的消息使用 Round-Robin 路由

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化

**测试步骤**:
1. 发布普通消息（无聚合ID）
2. 订阅消息并验证接收
3. 验证消息使用 Round-Robin 路由

**预期结果**:
- ✅ 消息成功接收
- ✅ 消息均匀分配到所有 Actor
- ✅ 无单点瓶颈

**测试方法**:
```go
// 创建测试用例
func TestNATSRoundRobinRouting(t *testing.T) {
    // 1. 初始化 EventBus
    bus, _ := NewNATSEventBus(config)
    
    // 2. 订阅消息
    receivedCount := atomic.Int64{}
    bus.Subscribe(ctx, "test-topic", func(ctx context.Context, data []byte) error {
        receivedCount.Add(1)
        return nil
    })
    
    // 3. 发布 100 条消息（无聚合ID）
    for i := 0; i < 100; i++ {
        bus.Publish(ctx, "test-topic", []byte(fmt.Sprintf("msg-%d", i)))
    }
    
    // 4. 等待处理完成
    time.Sleep(5 * time.Second)
    
    // 5. 验证所有消息都被接收
    assert.Equal(t, int64(100), receivedCount.Load())
}
```

---

#### **TC-F3: 错误处理**

**目标**: 验证消息处理错误时的行为

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化

**测试步骤**:
1. 订阅消息，handler 返回错误
2. 发布消息
3. 验证错误处理逻辑

**预期结果**:
- ✅ Subscribe 消息：不重投（at-most-once，memory storage）
- ✅ SubscribeEnvelope 消息：Nak 重新投递（at-least-once，file storage）
- ✅ 错误计数器增加
- ✅ Actor 不会 panic（错误被正确处理）

**测试方法**:
```go
func TestNATSErrorHandling(t *testing.T) {
    bus, _ := NewNATSEventBus(config)
    
    // 订阅消息，handler 返回错误
    bus.Subscribe(ctx, "test-topic", func(ctx context.Context, data []byte) error {
        return fmt.Errorf("simulated error")
    })
    
    // 发布消息
    bus.Publish(ctx, "test-topic", []byte("test"))
    
    // 等待处理
    time.Sleep(2 * time.Second)
    
    // 验证错误计数器
    assert.Greater(t, bus.(*natsEventBus).errorCount.Load(), uint64(0))
}
```

---

#### **TC-F4: 并发订阅**

**目标**: 验证多个订阅者并发处理消息

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化

**测试步骤**:
1. 创建多个订阅者
2. 发布消息
3. 验证所有订阅者都接收到消息

**预期结果**:
- ✅ 所有订阅者都接收到消息
- ✅ 无消息丢失
- ✅ 无并发冲突

**测试命令**:
```bash
cd jxt-core/sdk/pkg/eventbus
go test -v -run "TestNATSEventBus_MultipleSubscribers" -timeout 60s
```

---

#### **TC-F5: 关闭和清理**

**目标**: 验证 EventBus 关闭时的清理逻辑

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化

**测试步骤**:
1. 订阅消息
2. 发布消息
3. 关闭 EventBus
4. 验证资源清理

**预期结果**:
- ✅ Actor Pool 正确关闭
- ✅ 连接正确关闭
- ✅ 无资源泄漏

**测试命令**:
```bash
cd jxt-core/sdk/pkg/eventbus
go test -v -run "TestNATSEventBus_Close" -timeout 60s
```

---

#### **TC-F6: Subscribe/SubscribeEnvelope 存储类型验证**

**目标**: 验证 Subscribe 使用 memory storage，SubscribeEnvelope 使用 file storage

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化

**测试步骤**:
1. 使用 Subscribe 订阅 topic-memory
2. 使用 SubscribeEnvelope 订阅 topic-file
3. 检查 JetStream Stream 配置
4. 验证存储类型

**预期结果**:
- ✅ topic-memory 的 Stream 使用 MemoryStorage
- ✅ topic-file 的 Stream 使用 FileStorage
- ✅ Subscribe 消息失败不重投（at-most-once）
- ✅ SubscribeEnvelope 消息失败 Nak 重投（at-least-once）

**测试方法**:
```go
func TestNATSStorageTypeDifference(t *testing.T) {
    bus, _ := NewNATSEventBus(config)

    // 1. Subscribe 订阅（应使用 memory storage）
    bus.Subscribe(ctx, "topic-memory", func(ctx context.Context, data []byte) error {
        return nil
    })

    // 2. SubscribeEnvelope 订阅（应使用 file storage）
    bus.SubscribeEnvelope(ctx, "topic-file", func(ctx context.Context, env *Envelope) error {
        return nil
    })

    // 3. 检查 Stream 配置
    js, _ := bus.getJetStreamContext()

    memoryStream, _ := js.StreamInfo("topic-memory-stream")
    assert.Equal(t, nats.MemoryStorage, memoryStream.Config.Storage)

    fileStream, _ := js.StreamInfo("topic-file-stream")
    assert.Equal(t, nats.FileStorage, fileStream.Config.Storage)
}
```

---

#### **TC-F7: 混合场景测试**

**目标**: 验证同时发送有聚合ID和无聚合ID的消息时的正确性

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化

**测试步骤**:
1. 订阅 topic（同时接收有/无聚合ID消息）
2. 发布 50 条有聚合ID的消息（SubscribeEnvelope）
3. 发布 50 条无聚合ID的消息（Subscribe）
4. 验证所有消息都被正确处理

**预期结果**:
- ✅ 有聚合ID消息按聚合ID路由到同一 Actor
- ✅ 无聚合ID消息使用 Round-Robin 路由
- ✅ 所有 100 条消息都被接收
- ✅ 无消息丢失

**测试方法**:
```go
func TestNATSMixedMessages(t *testing.T) {
    bus, _ := NewNATSEventBus(config)

    receivedWithAgg := atomic.Int64{}
    receivedWithoutAgg := atomic.Int64{}

    // 订阅 Envelope 消息（有聚合ID）
    bus.SubscribeEnvelope(ctx, "mixed-topic", func(ctx context.Context, env *Envelope) error {
        receivedWithAgg.Add(1)
        return nil
    })

    // 订阅普通消息（无聚合ID）
    bus.Subscribe(ctx, "mixed-topic", func(ctx context.Context, data []byte) error {
        receivedWithoutAgg.Add(1)
        return nil
    })

    // 发布混合消息
    for i := 0; i < 50; i++ {
        env := &Envelope{AggregateID: fmt.Sprintf("agg-%d", i%10), Payload: []byte("data")}
        bus.PublishEnvelope(ctx, "mixed-topic", env)
    }
    for i := 0; i < 50; i++ {
        bus.Publish(ctx, "mixed-topic", []byte("data"))
    }

    time.Sleep(5 * time.Second)

    assert.Equal(t, int64(50), receivedWithAgg.Load())
    assert.Equal(t, int64(50), receivedWithoutAgg.Load())
}
```

---

#### **TC-F8: Actor 重启恢复测试**

**目标**: 验证 Actor panic 后自动重启，消息重投

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化
- Actor Pool MaxRestarts = 3

**测试步骤**:
1. 订阅消息，handler 第1次调用 panic
2. 发布消息
3. 验证 Actor 重启
4. 验证消息重投并成功处理

**预期结果**:
- ✅ Actor panic 后自动重启
- ✅ 消息重投并成功处理
- ✅ Actor 重启计数器增加
- ✅ 监控指标正确记录

**测试方法**:
```go
func TestNATSActorRestart(t *testing.T) {
    bus, _ := NewNATSEventBus(config)

    callCount := atomic.Int32{}

    bus.SubscribeEnvelope(ctx, "restart-topic", func(ctx context.Context, env *Envelope) error {
        count := callCount.Add(1)
        if count == 1 {
            panic("simulated panic") // 第1次 panic
        }
        return nil // 第2次成功
    })

    env := &Envelope{AggregateID: "test-agg", Payload: []byte("data")}
    bus.PublishEnvelope(ctx, "restart-topic", env)

    time.Sleep(5 * time.Second)

    // 验证消息被处理了2次（1次 panic + 1次成功）
    assert.Equal(t, int32(2), callCount.Load())
}
```

---

#### **TC-F9: Inbox 满时的背压测试**

**目标**: 验证 Actor Inbox 满时的背压行为

**前置条件**:
- NATS JetStream 已启动
- EventBus 已初始化
- Actor Pool InboxSize = 10（测试用）

**测试步骤**:
1. 订阅消息，handler 阻塞 5 秒
2. 快速发布 100 条消息到同一聚合ID
3. 验证背压行为

**预期结果**:
- ✅ Inbox 满时，ProcessMessage 返回错误或阻塞
- ✅ 消息不会丢失（JetStream 重投）
- ✅ Inbox 满计数器增加
- ✅ 监控指标正确记录

**测试方法**:
```go
func TestNATSInboxBackpressure(t *testing.T) {
    config := &EventBusConfig{
        // ... 配置 InboxSize = 10
    }
    bus, _ := NewNATSEventBus(config)

    processedCount := atomic.Int64{}

    bus.SubscribeEnvelope(ctx, "backpressure-topic", func(ctx context.Context, env *Envelope) error {
        time.Sleep(100 * time.Millisecond) // 模拟慢处理
        processedCount.Add(1)
        return nil
    })

    // 快速发布 100 条消息到同一聚合ID
    for i := 0; i < 100; i++ {
        env := &Envelope{AggregateID: "same-agg", Payload: []byte(fmt.Sprintf("msg-%d", i))}
        bus.PublishEnvelope(ctx, "backpressure-topic", env)
    }

    time.Sleep(15 * time.Second)

    // 验证所有消息最终都被处理
    assert.Equal(t, int64(100), processedCount.Load())
}
```

---

## 性能测试

### 📊 **测试用例清单**

#### **TC-P1: 吞吐量测试**

**目标**: 验证迁移后吞吐量不低于当前水平

**测试场景**:
- 低压: 500 条消息
- 中压: 2000 条消息
- 高压: 5000 条消息
- 极限: 10000 条消息

**成功标准**:
- ✅ 吞吐量 ≥ 162 msg/s（当前水平）

**测试命令**:
```bash
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s
```

**预期结果**:
```
低压(500):   吞吐量 ≥ 33 msg/s
中压(2000):  吞吐量 ≥ 132 msg/s
高压(5000):  吞吐量 ≥ 163 msg/s
极限(10000): 吞吐量 ≥ 321 msg/s
```

---

#### **TC-P2: 延迟测试**

**目标**: 验证迁移后延迟不高于当前水平

**测试场景**:
- 低压: 500 条消息
- 中压: 2000 条消息
- 高压: 5000 条消息
- 极限: 10000 条消息

**成功标准**:
- ✅ 延迟 ≤ 1010 ms（当前水平）

**测试命令**:
```bash
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s
```

**预期结果**:
```
低压(500):   延迟 ≤ 123 ms
中压(2000):  延迟 ≤ 317 ms
高压(5000):  延迟 ≤ 1418 ms
极限(10000): 延迟 ≤ 2181 ms
```

---

#### **TC-P3: 内存占用测试**

**目标**: 验证迁移后内存占用不高于当前水平

**测试场景**:
- 低压: 500 条消息
- 中压: 2000 条消息
- 高压: 5000 条消息
- 极限: 10000 条消息

**成功标准**:
- ✅ 内存增量 ≤ 3.14 MB（当前水平）

**测试命令**:
```bash
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s
```

**预期结果**:
```
低压(500):   内存增量 ≤ 2.20 MB
中压(2000):  内存增量 ≤ 2.71 MB
高压(5000):  内存增量 ≤ 3.34 MB
极限(10000): 内存增量 ≤ 4.32 MB
```

---

## 可靠性测试

### 🔧 **测试用例清单**

#### **TC-R1: Actor Panic 恢复**

**目标**: 验证 Actor panic 后自动重启

**测试步骤**:
1. 订阅消息，handler 触发 panic
2. 发布消息
3. 验证 Actor 自动重启
4. 验证后续消息正常处理

**预期结果**:
- ✅ Actor 自动重启（最多3次）
- ✅ 后续消息正常处理
- ✅ 监控事件正确记录

**测试方法**:
```go
func TestNATSActorPanicRecovery(t *testing.T) {
    bus, _ := NewNATSEventBus(config)
    
    panicCount := atomic.Int32{}
    bus.Subscribe(ctx, "test-topic", func(ctx context.Context, data []byte) error {
        if panicCount.Add(1) <= 2 {
            panic("simulated panic")
        }
        return nil
    })
    
    // 发布 5 条消息
    for i := 0; i < 5; i++ {
        bus.Publish(ctx, "test-topic", []byte(fmt.Sprintf("msg-%d", i)))
    }
    
    time.Sleep(5 * time.Second)
    
    // 验证 panic 次数
    assert.Equal(t, int32(2), panicCount.Load())
}
```

---

#### **TC-R2: 故障隔离**

**目标**: 验证不同聚合ID的消息故障隔离

**测试步骤**:
1. 订阅 Envelope 消息
2. 发布多个聚合ID的消息，其中一个触发错误
3. 验证其他聚合ID的消息正常处理

**预期结果**:
- ✅ 故障聚合ID的消息被隔离
- ✅ 其他聚合ID的消息正常处理
- ✅ 无级联失败

**测试命令**:
```bash
cd jxt-core/tests/eventbus/reliability_regression_tests
go test -v -run "TestFaultIsolation" -timeout 60s
```

---

#### **TC-R3: 监控指标验证**

**目标**: 验证监控指标正确记录

**测试步骤**:
1. 发布和订阅消息
2. 触发错误和 panic
3. 验证监控指标

**预期结果**:
- ✅ `publishedMessages` 计数正确
- ✅ `consumedMessages` 计数正确
- ✅ `errorCount` 计数正确
- ✅ Actor Pool 指标正确

**测试方法**:
```go
func TestNATSMetrics(t *testing.T) {
    bus, _ := NewNATSEventBus(config)
    
    // 订阅消息
    bus.Subscribe(ctx, "test-topic", func(ctx context.Context, data []byte) error {
        return nil
    })
    
    // 发布 10 条消息
    for i := 0; i < 10; i++ {
        bus.Publish(ctx, "test-topic", []byte(fmt.Sprintf("msg-%d", i)))
    }
    
    time.Sleep(5 * time.Second)
    
    // 验证指标
    assert.Equal(t, uint64(10), bus.(*natsEventBus).publishedMessages.Load())
    assert.Equal(t, uint64(10), bus.(*natsEventBus).consumedMessages.Load())
}
```

---

## 回归测试

### 🔄 **测试范围**

运行所有现有测试，确保迁移不影响现有功能。

#### **1. NATS 单元测试**

```bash
cd jxt-core/sdk/pkg/eventbus
go test -v -run "TestNATS" -timeout 60s
```

**预期结果**: 所有测试通过（4/4）

---

#### **2. 可靠性回归测试**

```bash
cd jxt-core/tests/eventbus/reliability_regression_tests
go test -v -timeout 180s
```

**预期结果**: 所有测试通过

---

#### **3. 性能回归测试**

```bash
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s
```

**预期结果**: 性能指标达标

---

## 测试执行计划

### 📅 **执行顺序**

| 阶段 | 测试类型 | 预计时间 | 执行时机 |
|------|---------|---------|---------|
| **阶段 1** | 编译验证 | 15 分钟 | 代码修改完成后 |
| **阶段 2** | 功能测试 | 1 小时 | 编译通过后 |
| **阶段 3** | 性能测试 | 1 小时 | 功能测试通过后 |
| **阶段 4** | 可靠性测试 | 0.5 小时 | 性能测试通过后 |
| **阶段 5** | 回归测试 | 0.5 小时 | 可靠性测试通过后 |
| **总计** | - | 3 小时 | - |

---

### ✅ **测试执行清单**

#### **阶段 1: 编译验证**

- [ ] 编译成功，无错误
- [ ] 无 lint 警告

#### **阶段 2: 功能测试**

- [ ] TC-F1: 有聚合ID的消息路由 ✅
- [ ] TC-F2: 无聚合ID的消息路由 ✅
- [ ] TC-F3: 错误处理 ✅
- [ ] TC-F4: 并发订阅 ✅
- [ ] TC-F5: 关闭和清理 ✅

#### **阶段 3: 性能测试**

- [ ] TC-P1: 吞吐量测试 ✅
- [ ] TC-P2: 延迟测试 ✅
- [ ] TC-P3: 内存占用测试 ✅

#### **阶段 4: 可靠性测试**

- [ ] TC-R1: Actor Panic 恢复 ✅
- [ ] TC-R2: 故障隔离 ✅
- [ ] TC-R3: 监控指标验证 ✅

#### **阶段 5: 回归测试**

- [ ] NATS 单元测试 ✅
- [ ] 可靠性回归测试 ✅
- [ ] 性能回归测试 ✅

---

## 附录

### A. 测试命令汇总

```bash
# 1. 编译验证
cd jxt-core/sdk/pkg/eventbus
go build ./...

# 2. NATS 单元测试
go test -v -run "TestNATS" -timeout 60s

# 3. 性能测试
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s

# 4. 可靠性测试
cd jxt-core/tests/eventbus/reliability_regression_tests
go test -v -timeout 180s
```

---

**文档状态**: 待评审  
**下一步**: 等待评审批准后，执行测试计划

