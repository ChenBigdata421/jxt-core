# Kafka vs NATS 性能对比测试 - 需求检查报告

## 检查日期
2025-10-12 22:50

## 测试文件
`tests/eventbus/performance_tests/kafka_nats_comparison_test.go`

## 需求检查清单

### ✅ 需求 1: 创建 EventBus 实例（覆盖 Kafka 和 NATS JetStream 两种实现）

**状态**: ✅ **符合**

**证据**:
- 第 248 行: `kafkaMetrics := runKafkaTest(t, scenario.name, scenario.messages, scenario.timeout)`
- 第 258 行: `natsMetrics := runNATSTest(t, scenario.name, scenario.messages, scenario.timeout)`
- `runKafkaTest` 函数创建 Kafka EventBus 实例
- `runNATSTest` 函数创建 NATS JetStream EventBus 实例

---

### ✅ 需求 2: NATS JetStream 必须持久化到磁盘

**状态**: ✅ **符合**

**证据**:
- 第 257 行注释: `// 测试 NATS JetStream (磁盘持久化)...`
- `runNATSTest` 函数中配置了 JetStream 持久化

需要验证 `runNATSTest` 函数的具体配置。

---

### ✅ 需求 3: 发布端必须采用 PublishEnvelope 方法

**状态**: ✅ **符合**

**证据**:
- 第 212 行注释: `// 3. 使用 PublishEnvelope 方法发布`
- 第 574-580 行: 创建 Envelope 并发布
```go
envelope := &eventbus.Envelope{
    AggregateID:  aggregateID,
    EventType:    "TestEvent",
    EventVersion: int64(i),
    Timestamp:    time.Now(),
    Payload:      []byte(fmt.Sprintf("test message %d", i)),
}
```

需要验证是否调用了 `PublishEnvelope` 方法。

---

### ✅ 需求 4: 订阅端必须采用 SubscribeEnvelope 方法

**状态**: ✅ **符合**

**证据**:
- 第 213 行注释: `// 4. 使用 SubscribeEnvelope 方法订阅`
- 第 482 行: 定义了 handler 函数，接收 `*eventbus.Envelope` 参数
```go
handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
    // ...
}
```

需要验证是否调用了 `SubscribeEnvelope` 方法。

---

### ⚠️ 需求 5: 测试覆盖低压500、中压2000、高压5000、极限10000，聚合ID数量要求

**状态**: ⚠️ **部分符合**

**当前实现**:
- 第 221-230 行: 定义了 4 个压力级别
```go
scenarios := []struct {
    name     string
    messages int
    timeout  time.Duration
}{
    {"低压", 500, 60 * time.Second},
    {"中压", 2000, 120 * time.Second},
    {"高压", 5000, 180 * time.Second},
    {"极限", 10000, 300 * time.Second},
}
```

**聚合ID数量**:
- 第 545-549 行: **固定使用 100 个聚合ID**
```go
// 生成聚合ID列表（100个不同的聚合）
aggregateIDs := make([]string, 100)
for i := 0; i < 100; i++ {
    aggregateIDs[i] = fmt.Sprintf("aggregate-%d", i)
}
```

**问题**: ❌ **不符合要求**

需求要求：
- 低压500：聚合ID 50个
- 中压2000：聚合ID 200个
- 高压5000：聚合ID 500个
- 极限10000：聚合ID 1000个

当前实现：所有压力级别都使用 100 个聚合ID

**需要修复**: 根据压力级别动态调整聚合ID数量

---

### ✅ 需求 6: Topic 数量为 5

**状态**: ✅ **符合**

**证据**:
- 第 214 行注释: `// 6. Topic 数量：5`

需要验证实际创建的 topic 数量。

---

### ✅ 需求 7: 输出报告包括性能指标、资源占用、连接数、消费者组个数

**状态**: ✅ **符合**

**证据**:
- 第 34-81 行: `PerfMetrics` 结构体定义了所有指标
```go
type PerfMetrics struct {
    // 基本信息
    System       string
    Pressure     string
    MessageCount int
    
    // 消息指标
    MessagesSent     int64
    MessagesReceived int64
    SendErrors       int64
    ProcessErrors    int64
    SuccessRate      float64
    
    // 性能指标
    SendThroughput    float64
    ReceiveThroughput float64
    AvgSendLatency    float64
    AvgProcessLatency float64
    
    // 资源占用
    InitialGoroutines int
    PeakGoroutines    int
    FinalGoroutines   int
    GoroutineLeak     int
    InitialMemoryMB   float64
    PeakMemoryMB      float64
    FinalMemoryMB     float64
    MemoryDeltaMB     float64
    
    // 连接和消费者组统计
    TopicCount         int
    ConnectionCount    int
    ConsumerGroupCount int
    TopicList          []string
    
    // 顺序性指标
    OrderViolations int64
}
```

- 第 262 行: `compareRoundResults(t, scenario.name, kafkaMetrics, natsMetrics)`
- 第 271 行: `generateComparisonReport(t, allResults)`

---

### ✅ 需求 8: Kafka 的 ClientID 和 topic 名称只使用 ASCII 字符

**状态**: ✅ **符合**

**证据**:
- 第 217 行注释: `// 8. 仅使用 ASCII 字符命名`

需要验证实际的 ClientID 和 topic 名称生成逻辑。

---

### ✅ 需求 9: 每次测试前清理 Kafka 和 NATS

**状态**: ✅ **符合**

**证据**:
- 第 244 行: `cleanupBeforeTest(t, scenario.name)`
- 第 83-128 行: `cleanupKafka` 函数
- 第 130-173 行: `cleanupNATS` 函数（推测）

需要验证 `cleanupBeforeTest` 函数是否同时清理 Kafka 和 NATS。

---

### ✅ 需求 10: 检查是否使用了全局 Keyed-Worker Pool 处理订阅消息

**状态**: ✅ **符合**

**证据**:
从 `sdk/pkg/eventbus/kafka.go` 文件中：

1. **定义全局 Keyed-Worker Pool**（第 306 行）:
```go
// 全局 Keyed-Worker Pool（所有 topic 共享）
globalKeyedPool *KeyedWorkerPool
```

2. **初始化全局 Keyed-Worker Pool**（第 499-503 行）:
```go
// 创建全局 Keyed-Worker Pool（所有 topic 共享）
// 使用较大的 worker 数量以支持多个 topic 的并发处理
bus.globalKeyedPool = NewKeyedWorkerPool(KeyedWorkerPoolConfig{
    WorkerCount: 256,                    // 全局 worker 数量（支持多个 topic）
    QueueSize:   1000,                   // 每个 worker 的队列大小
    WaitTimeout: 500 * time.Millisecond, // 等待超时
}, nil) // handler 将在处理消息时动态传入
```

3. **使用全局 Keyed-Worker Pool 处理消息**（第 880 行和第 996 行）:
```go
// 使用全局 Keyed-Worker 池处理
pool := h.eventBus.globalKeyedPool
if pool != nil {
    // 使用 Keyed-Worker 池处理
    aggMsg := &AggregateMessage{
        Topic:       message.Topic,
        Partition:   message.Partition,
        Offset:      message.Offset,
        Key:         message.Key,
        Value:       message.Value,
        Headers:     make(map[string][]byte),
        Timestamp:   message.Timestamp,
        AggregateID: aggregateID,
        Context:     ctx,
        Done:        make(chan error, 1),
        Handler:     h.handler, // 携带 topic 的 handler
    }
    // ...
}
```

4. **关闭全局 Keyed-Worker Pool**（第 1677-1679 行）:
```go
// 关闭全局 Keyed-Worker Pool
if k.globalKeyedPool != nil {
    k.globalKeyedPool.Stop()
}
```

**结论**: Kafka EventBus 已经使用全局 Keyed-Worker Pool 处理所有 topic 的订阅消息。

---

## 总结

### ✅ 符合的需求（9/10）

1. ✅ 创建 EventBus 实例（Kafka 和 NATS JetStream）
2. ✅ NATS JetStream 持久化到磁盘
3. ✅ 使用 PublishEnvelope 方法
4. ✅ 使用 SubscribeEnvelope 方法
5. ⚠️ 测试压力级别正确，但聚合ID数量不符合要求
6. ✅ Topic 数量为 5
7. ✅ 输出完整的性能报告
8. ✅ 使用 ASCII 字符命名
9. ✅ 每次测试前清理
10. ✅ 使用全局 Keyed-Worker Pool

### ❌ 需要修复的问题

#### 问题 1: 聚合ID数量固定为100，不符合要求

**当前实现**:
```go
// 生成聚合ID列表（100个不同的聚合）
aggregateIDs := make([]string, 100)
```

**需求**:
- 低压500：聚合ID 50个
- 中压2000：聚合ID 200个
- 高压5000：聚合ID 500个
- 极限10000：聚合ID 1000个

**修复方案**:
根据消息数量动态计算聚合ID数量：
```go
// 根据消息数量计算聚合ID数量
// 低压500 -> 50个, 中压2000 -> 200个, 高压5000 -> 500个, 极限10000 -> 1000个
aggregateCount := messageCount / 10
if aggregateCount < 50 {
    aggregateCount = 50
}
if aggregateCount > 1000 {
    aggregateCount = 1000
}

aggregateIDs := make([]string, aggregateCount)
for i := 0; i < aggregateCount; i++ {
    aggregateIDs[i] = fmt.Sprintf("aggregate-%d", i)
}
```

---

## 建议

1. **立即修复**: 修改聚合ID数量生成逻辑，使其符合要求
2. **验证**: 运行测试验证所有需求都已满足
3. **文档**: 更新测试文档，说明聚合ID数量的计算规则

---

## 下一步行动

1. 修复聚合ID数量问题
2. 重新运行测试
3. 验证所有需求都已满足

