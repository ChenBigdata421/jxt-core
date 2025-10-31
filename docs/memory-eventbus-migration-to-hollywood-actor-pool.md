# Memory EventBus 迁移到 Hollywood Actor Pool - 技术设计文档

## 📋 文档信息

| 项目 | 内容 |
|------|------|
| **文档标题** | Memory EventBus 迁移到 Hollywood Actor Pool |
| **文档版本** | v1.0 |
| **创建日期** | 2025-10-30 |
| **作者** | Augment Agent |
| **状态** | 待评审 |

---

## 🎯 1. 迁移目标

### 1.1 核心目标

**统一 EventBus 架构**：将 Memory EventBus 迁移到 Hollywood Actor Pool，实现与 Kafka/NATS EventBus 的架构一致性。

### 1.2 具体目标

1. ✅ **架构统一**：Memory EventBus 使用与 Kafka/NATS 相同的 Hollywood Actor Pool
2. ✅ **行为一致**：
   - 有聚合ID：使用一致性哈希路由（保证顺序）
   - 无聚合ID：使用 Round-Robin 轮询路由（最大并发）
3. ✅ **企业级可靠性**：
   - Actor 级别故障隔离
   - 自动重启机制（maxRestarts=3）
   - Dead Letter 处理
4. ✅ **测试环境一致性**：测试环境（Memory）与生产环境（Kafka/NATS）行为一致

### 1.3 非目标

- ❌ **不改变 Memory EventBus 的 API**：保持向后兼容
- ❌ **不添加持久化**：Memory EventBus 仍然是内存实现
- ❌ **不改变性能特征**：保持低延迟特性

---

## 🔍 2. 现状分析

### 2.1 当前 Memory EventBus 架构

<augment_code_snippet path="jxt-core/sdk/pkg/eventbus/memory.go" mode="EXCERPT">
````go
// Lines 92-111
// 异步处理消息，避免阻塞发布者
go func() {
    for _, handler := range handlersCopy {
        go func(h MessageHandler) {  // ⚠️ 每个 handler 一个 goroutine
            defer func() {
                if r := recover(); r != nil {
                    logger.Error("Message handler panicked", "topic", topic, "panic", r)
                    m.metrics.ConsumeErrors++
                }
            }()

            if err := h(ctx, message); err != nil {
                logger.Error("Message handler failed", "topic", topic, "error", err)
                m.metrics.ConsumeErrors++
            } else {
                m.metrics.MessagesConsumed++
            }
        }(handler)
    }
}()
````
</augment_code_snippet>

### 2.2 当前架构的问题

| 问题 | 影响 | 严重性 |
|------|------|--------|
| **无 Actor Pool** | 每个消息直接 spawn goroutine | P0 |
| **无聚合ID路由** | 无法保证同一聚合ID的消息顺序 | P0 |
| **无故障隔离** | panic 导致消息丢失，无重试 | P0 |
| **无自动重启** | 无 Supervisor 机制 | P1 |
| **架构不一致** | 与 Kafka/NATS 行为不同 | P0 |

### 2.3 测试覆盖范围

**使用 Memory EventBus 的测试文件**：

1. **功能回归测试** (function_regression_tests):
   - `integration_test.go` (4 个测试)
   - `json_serialization_test.go` (1 个测试)
   - `prometheus_integration_test.go` (2 个测试)
   - `topic_name_validation_test.go` (6 个测试)

2. **可靠性回归测试** (reliability_regression_tests):
   - `actor_recovery_test.go` (4 个测试)
   - `fault_isolation_test.go` (3 个测试)
   - `message_guarantee_test.go` (4 个测试)

3. **单元测试** (sdk/pkg/eventbus):
   - `memory_regression_test.go` (10+ 个测试)

**总计**: ~34 个测试用例

---

## 🏗️ 3. 技术方案

### 3.1 架构设计

#### 3.1.1 整体架构

```
Memory EventBus (迁移后)
├── memoryEventBus (核心)
│   ├── subscribers map[string][]MessageHandler
│   ├── globalActorPool *HollywoodActorPool  ⭐ 新增
│   ├── roundRobinCounter atomic.Uint64      ⭐ 新增
│   └── mu sync.RWMutex
├── memoryPublisher
└── memorySubscriber
```

#### 3.1.2 消息处理流程

```
Publish(topic, message)
    ↓
获取 topic 的所有 handlers
    ↓
对每个 handler:
    ├── 提取 aggregateID (如果是 Envelope)
    ├── 确定 routingKey:
    │   ├── 有 aggregateID → routingKey = aggregateID
    │   └── 无 aggregateID → routingKey = "rr-{counter++}"
    ├── 创建 AggregateMessage
    └── 提交到 Hollywood Actor Pool
        ↓
    Actor 处理消息
        ├── 调用 handler(ctx, message)
        ├── panic → Supervisor 自动重启 Actor
        └── 返回结果
```

### 3.2 核心代码变更

#### 3.2.1 新增字段

```go
type memoryEventBus struct {
    subscribers map[string][]MessageHandler
    mu          sync.RWMutex
    closed      bool
    metrics     *Metrics
    
    // ⭐ 新增：Hollywood Actor Pool
    globalActorPool *HollywoodActorPool
    
    // ⭐ 新增：Round-Robin 计数器
    roundRobinCounter atomic.Uint64
}
```

#### 3.2.2 初始化 Actor Pool

```go
func NewMemoryEventBus() EventBus {
    bus := &memoryEventBus{
        subscribers: make(map[string][]MessageHandler),
        metrics: &Metrics{
            LastHealthCheck:   time.Now(),
            HealthCheckStatus: "healthy",
        },
    }

    // ⭐ 初始化 Hollywood Actor Pool
    // ⭐ 修复：使用唯一的 namespace 避免 Prometheus 指标冲突
    namespace := fmt.Sprintf("memory-eventbus-%d", time.Now().UnixNano())

    poolConfig := &HollywoodActorPoolConfig{
        PoolSize:    256,
        InboxSize:   1000,
        MaxRestarts: 3,
        Namespace:   namespace,
    }

    metricsCollector := NewPrometheusActorPoolMetricsCollector(poolConfig.Namespace)
    pool, err := NewHollywoodActorPool(poolConfig, metricsCollector)
    if err != nil {
        // ⭐ 修复：Actor Pool 初始化失败直接返回错误（不使用 Fallback）
        logger.Error("Failed to create Hollywood Actor Pool for Memory EventBus", "error", err)
        panic(fmt.Sprintf("Failed to create Hollywood Actor Pool: %v", err))
    }

    bus.globalActorPool = pool
    logger.Info("Memory EventBus using Hollywood Actor Pool",
        "poolSize", poolConfig.PoolSize,
        "inboxSize", poolConfig.InboxSize,
        "maxRestarts", poolConfig.MaxRestarts)

    return &eventBusManager{...}
}
```

#### 3.2.3 修改 Publish 方法

```go
func (m *memoryEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    m.mu.RLock()
    if m.closed {
        m.mu.RUnlock()
        return fmt.Errorf("memory eventbus is closed")
    }

    handlers, exists := m.subscribers[topic]
    if !exists || len(handlers) == 0 {
        m.mu.RUnlock()
        logger.Debug("No subscribers for topic", "topic", topic)
        return nil
    }

    handlersCopy := make([]MessageHandler, len(handlers))
    copy(handlersCopy, handlers)
    m.mu.RUnlock()

    // ⭐ 使用 Hollywood Actor Pool 处理消息（不再有 Fallback）
    return m.publishWithActorPool(ctx, topic, message, handlersCopy)
}
```

#### 3.2.4 新增 publishWithActorPool 方法

```go
func (m *memoryEventBus) publishWithActorPool(ctx context.Context, topic string, message []byte, handlers []MessageHandler) error {
    // 提取 aggregateID（如果是 Envelope）
    // ⭐ 使用与 Kafka/NATS 一致的提取逻辑
    aggregateID, _ := ExtractAggregateID(message, nil, nil, "")

    // ⭐ 确定 routingKey（所有 handler 共享同一个 routingKey）
    // 策略：
    // - 有聚合ID：使用 aggregateID（保证同一聚合的消息顺序）
    // - 无聚合ID：使用 Round-Robin（最大并发，但同一条消息的所有 handler 路由到同一 Actor）
    routingKey := aggregateID
    if routingKey == "" {
        // 无聚合ID：使用 Round-Robin（每条消息递增一次）
        index := m.roundRobinCounter.Add(1)
        routingKey = fmt.Sprintf("rr-%d", index)
    }

    // 对每个 handler 提交到 Actor Pool
    for _, handler := range handlers {
        // 创建 AggregateMessage
        aggMsg := &AggregateMessage{
            Topic:       topic,
            Value:       message,
            AggregateID: routingKey,  // ⭐ 所有 handler 使用相同的 routingKey
            Context:     ctx,
            Done:        make(chan error, 1),
            Handler:     handler,
        }

        // 提交到 Actor Pool
        if err := m.globalActorPool.ProcessMessage(ctx, aggMsg); err != nil {
            logger.Error("Failed to submit message to actor pool", "error", err)
            m.metrics.ConsumeErrors++
            continue
        }

        // ⭐ 修复：使用 time.NewTimer 避免泄漏
        // 异步等待结果（不阻塞发布者）
        go func(msg *AggregateMessage) {
            timer := time.NewTimer(30 * time.Second)
            defer timer.Stop()

            select {
            case err := <-msg.Done:
                if err != nil {
                    logger.Error("Message handler failed", "topic", topic, "error", err)
                    m.metrics.ConsumeErrors++
                } else {
                    m.metrics.MessagesConsumed++
                }
            case <-timer.C:
                logger.Error("Message processing timeout", "topic", topic)
                m.metrics.ConsumeErrors++
            }
        }(aggMsg)
    }

    m.metrics.MessagesPublished++
    return nil
}
```

#### 3.2.5 修改 Close 方法

```go
func (m *memoryEventBus) Close() error {
    m.mu.Lock()
    defer m.mu.Unlock()

    if m.closed {
        return nil
    }

    m.closed = true
    
    // ⭐ 关闭 Hollywood Actor Pool
    if m.globalActorPool != nil {
        m.globalActorPool.Stop()
    }
    
    m.subscribers = make(map[string][]MessageHandler)
    logger.Info("Memory eventbus closed")
    return nil
}
```

---

## 📊 4. 影响分析

### 4.1 性能影响

| 指标 | 迁移前 | 迁移后 | 变化 |
|------|--------|--------|------|
| **延迟** | ~1-5ms | ~2-8ms | +1-3ms (Actor 调度开销) |
| **吞吐量** | ~100k msg/s | 待测试验证 | 待测试验证 (Actor Pool 可能提高吞吐量) |
| **Goroutine 数量** | 不受控 | 256 (固定) | 大幅减少 |
| **内存占用** | 低 | 中 | +10-20MB (Actor Pool) |

**结论**: 延迟略有增加，但 Goroutine 数量大幅减少，内存占用可控。吞吐量需要实际测试验证。

### 4.2 行为变化

| 场景 | 迁移前 | 迁移后 |
|------|--------|--------|
| **无聚合ID消息** | 完全并发 | Round-Robin 轮询（仍然并发） |
| **有聚合ID消息** | 无顺序保证 | ✅ 严格顺序保证 |
| **Handler panic** | 消息丢失 | ✅ Actor 自动重启，消息重试 |
| **多个 handler** | 全部并发执行 | 每个 handler 独立路由 |

**结论**: 行为变化是**改进**，修复了原有的缺陷。

### 4.3 测试影响

**预期通过的测试**:
- ✅ 所有功能回归测试 (13 个)
- ✅ 所有可靠性回归测试 (11 个) ⭐ 之前失败的现在会通过
- ✅ 所有单元测试 (10+ 个)

**可能需要调整的测试**:
- ⚠️ 性能测试：延迟和吞吐量阈值可能需要调整
- ⚠️ 并发测试：Goroutine 数量断言需要更新

---

## 🧪 5. 测试策略

### 5.1 单元测试

**测试文件**: `jxt-core/sdk/pkg/eventbus/memory_regression_test.go`

**测试用例**:
1. ✅ TestMemoryEventBus_ClosedPublish
2. ✅ TestMemoryEventBus_ClosedSubscribe
3. ✅ TestMemoryEventBus_PublishNoSubscribers
4. ✅ TestMemoryEventBus_HandlerError
5. ✅ TestMemoryEventBus_DoubleClose
6. ✅ TestMemoryEventBus_ConcurrentOperations
7. ✅ TestMemoryEventBus_Integration
8. ✅ TestMemoryEventBus_Close

**预期结果**: 全部通过（无需修改）

### 5.2 功能回归测试

**测试文件**: `jxt-core/tests/eventbus/function_regression_tests/`

**测试用例**:
1. ✅ TestE2E_MemoryEventBus_WithEnvelope
2. ✅ TestE2E_MemoryEventBus_MultipleTopics
3. ✅ TestE2E_MemoryEventBus_ConcurrentPublishSubscribe
4. ✅ TestE2E_MemoryEventBus_Metrics
5. ✅ TestPrometheusIntegration_BasicPublishSubscribe
6. ✅ TestPrometheusIntegration_E2E_Memory
7. ✅ TestTopicNameValidation_Memory_* (6 个测试)

**预期结果**: 全部通过（无需修改）

### 5.3 可靠性回归测试

**测试文件**: `jxt-core/tests/eventbus/reliability_regression_tests/`

**测试用例**:
1. ✅ TestActorRecovery (4 个测试) ⭐ 之前失败，现在会通过
2. ✅ TestFaultIsolation (3 个测试) ⭐ 之前失败，现在会通过
3. ✅ TestMessageOrderingAfterRecovery (4 个测试) ⭐ 之前失败，现在会通过

**预期结果**: 全部通过（修复了之前的失败）

### 5.4 性能测试

**测试目标**: 验证迁移后的性能影响

**测试场景**:

| 场景 | 消息数量 | Topic 数量 | 并发度 | 测试指标 |
|------|----------|-----------|--------|----------|
| **低压** | 1,000 | 1 | 10 | 延迟、吞吐量、Goroutine 数量 |
| **中压** | 10,000 | 5 | 50 | 延迟、吞吐量、Goroutine 数量 |
| **高压** | 100,000 | 10 | 100 | 延迟、吞吐量、Goroutine 数量 |

**性能指标**:
1. **延迟**: P50, P95, P99
2. **吞吐量**: msg/s
3. **Goroutine 数量**: 峰值、平均值
4. **内存占用**: 峰值、平均值

**验收标准**:
- ✅ P99 延迟 < 50ms
- ✅ 吞吐量 > 50k msg/s
- ✅ 峰值 Goroutine 数量 < 500
- ✅ 峰值内存占用 < 100MB

**测试方法**:
```go
func BenchmarkMemoryEventBus_WithActorPool(b *testing.B) {
    bus := NewMemoryEventBus()
    defer bus.Close()

    ctx := context.Background()
    topic := "benchmark.test"

    var received atomic.Int64
    err := bus.Subscribe(ctx, topic, func(ctx context.Context, data []byte) error {
        received.Add(1)
        return nil
    })
    require.NoError(b, err)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        message := []byte(fmt.Sprintf("message-%d", i))
        _ = bus.Publish(ctx, topic, message)
    }
    b.StopTimer()

    // 等待所有消息处理完成
    time.Sleep(1 * time.Second)

    b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "msg/s")
}
```

---

## 🚀 6. 实施计划

### 6.1 实施步骤

| 步骤 | 任务 | 预计时间 |
|------|------|----------|
| 1 | 修改 `memory.go`：新增字段和初始化 | 30 分钟 |
| 2 | 实现 `publishWithActorPool` 方法 | 1 小时 |
| 3 | 实现 `extractAggregateIDFromMessage` 辅助函数 | 30 分钟 |
| 4 | 修改 `Close` 方法 | 15 分钟 |
| 5 | 运行单元测试 | 15 分钟 |
| 6 | 运行功能回归测试 | 30 分钟 |
| 7 | 运行可靠性回归测试 | 30 分钟 |
| 8 | 修复测试失败（如有） | 1 小时 |
| 9 | 代码审查和文档更新 | 30 分钟 |

**总计**: ~5 小时

### 6.2 回滚计划

**触发条件**:
- 超过 20% 的测试失败
- 发现严重的性能退化（>50%）
- 发现数据丢失或顺序错误

**回滚步骤**:
1. 恢复 `memory.go` 到迁移前版本
2. 重新运行所有测试
3. 分析失败原因，修订迁移方案

---

## 📝 7. 风险评估

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| **性能退化** | 中 | 中 | 性能测试，调整 Actor Pool 大小 |
| **测试失败** | 低 | 低 | 充分的单元测试和回归测试 |
| **内存泄漏** | 低 | 高 | 代码审查，压力测试 |
| **向后兼容性** | 低 | 中 | 保留 Fallback 逻辑 |

---

## ✅ 8. 验收标准

1. ✅ **所有单元测试通过** (10+ 个)
2. ✅ **所有功能回归测试通过** (13 个)
3. ✅ **所有可靠性回归测试通过** (11 个) ⭐ 包括之前失败的
4. ✅ **性能退化 < 30%**
5. ✅ **无内存泄漏**
6. ✅ **代码审查通过**

---

## 📚 9. 参考文档

1. [Kafka EventBus 迁移文档](./kafka-global-worker-pool-to-hollywood-actor-pool-migration.md)
2. [Hollywood Actor Pool 设计文档](./hollywood-actor-pool-design.md)
3. [EventBus 架构文档](./eventbus-architecture.md)

---

## 📌 10. 附录

### 10.1 术语表

| 术语 | 定义 |
|------|------|
| **Hollywood Actor Pool** | 基于 Hollywood 框架的 Actor 池，提供故障隔离和自动重启 |
| **Round-Robin** | 轮询路由策略，均匀分配消息到所有 Actor |
| **Consistent Hashing** | 一致性哈希，保证相同 aggregateID 路由到同一 Actor |
| **Supervisor** | 监督者模式，负责监控和重启失败的 Actor |

### 10.2 代码示例

**提取 aggregateID 的辅助函数**:

```go
// ⭐ 复用 Kafka/NATS 的 ExtractAggregateID 函数，保持一致性
// 注意：Memory EventBus 没有 headers 和 key，所以传 nil
func extractAggregateIDFromMessage(message []byte) string {
    aggregateID, _ := ExtractAggregateID(message, nil, nil, "")
    return aggregateID
}
```

**说明**:
- `ExtractAggregateID` 会尝试解析 message 为 Envelope
- 如果解析成功，返回 `envelope.AggregateID`
- 如果解析失败，返回空字符串
- 与 Kafka/NATS 的实现完全一致

### 10.3 多 Handler 路由策略说明

**场景**: 一个 topic 有多个 handler

**策略**: 所有 handler 共享同一个 routingKey

**原因**:
1. **保证顺序**: 同一条消息的所有 handler 在同一个 Actor 中顺序执行
2. **简化逻辑**: 不需要为每个 handler 单独路由
3. **与 Kafka/NATS 一致**: Kafka/NATS 也是这样处理的

**示例**:
```go
// 假设 topic "orders" 有 2 个 handler: handlerA, handlerB
// 发布一条消息（无聚合ID）
bus.Publish(ctx, "orders", message)

// 路由逻辑：
// 1. routingKey = "rr-1" (Round-Robin 计数器递增一次)
// 2. handlerA 和 handlerB 都使用 routingKey = "rr-1"
// 3. 两个 handler 被路由到同一个 Actor (例如 Actor-123)
// 4. Actor-123 顺序执行: handlerA(message) → handlerB(message)
```

### 10.4 SubscribeEnvelope 支持说明

**当前状态**: Memory EventBus 通过 `eventBusManager` 支持 `SubscribeEnvelope`

**实现方式**:
- `SubscribeEnvelope` 在 `eventBusManager` 层实现
- 内部调用 `Subscribe`，handler 自动解析 Envelope
- 无需修改 `memoryEventBus` 的核心逻辑

**代码示例**:
```go
// eventBusManager.SubscribeEnvelope 实现
func (m *eventBusManager) SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error {
    // 包装 handler
    wrappedHandler := func(ctx context.Context, data []byte) error {
        var envelope Envelope
        if err := json.Unmarshal(data, &envelope); err != nil {
            return err
        }
        return handler(ctx, &envelope)
    }

    // 调用底层的 Subscribe
    return m.subscriber.Subscribe(ctx, topic, wrappedHandler)
}
```

**结论**: 无需额外修改，`SubscribeEnvelope` 已经支持。

---

**文档结束**

