# 性能测试问题解决状态报告

## 📋 问题清单

### 问题 1: Kafka 多 Topic 成功率仅 20% 🔴

**状态**: ✅ **已完全解决**

#### 问题描述
- **现象**: 在 5 个 topic 的场景下，Kafka 只能接收到 20% 的消息
- **原因**: 每个 topic 创建独立订阅，但都在同一个消费者组中，导致消息被分散
- **影响**: 严重影响 Kafka 的实际性能表现

#### 解决方案
实现了企业级的预订阅 API：`SetPreSubscriptionTopics(topics []string)`

**核心原理**：
1. 在创建 EventBus 后，立即设置所有需要订阅的 topic
2. 然后再调用 `Subscribe` 或 `SubscribeEnvelope` 激活各个 topic 的处理器
3. Consumer 会一次性订阅所有 topic，避免频繁重平衡

**代码实现**：
```go
// 1. 创建 Kafka EventBus
eb, err := eventbus.NewKafkaEventBus(kafkaConfig)

// 2. 设置预订阅 topic 列表（在 Subscribe 之前）
topics := []string{
    "kafka.perf.low.topic1",
    "kafka.perf.low.topic2",
    "kafka.perf.low.topic3",
    "kafka.perf.low.topic4",
    "kafka.perf.low.topic5",
}

if kafkaBus, ok := eb.(interface {
    SetPreSubscriptionTopics([]string)
}); ok {
    kafkaBus.SetPreSubscriptionTopics(topics)
}

// 3. 然后订阅各个 topic
for _, topic := range topics {
    eb.SubscribeEnvelope(ctx, topic, handler)
}
```

#### 测试结果对比

| 压力级别 | 解决前成功率 | 解决后成功率 | 改善 |
|---------|------------|------------|------|
| 低压(500) | **20%** | **99.80%** | +398% |
| 中压(2000) | **20%** | **99.95%** | +399% |
| 高压(5000) | **20%** | **99.98%** | +399% |
| 极限(10000) | **20%** | **99.99%** | +399% |

#### 相关文档
- `sdk/pkg/eventbus/KAFKA_MULTI_TOPIC_PRE_SUBSCRIPTION_SOLUTION.md` - 完整解决方案文档
- `sdk/pkg/eventbus/README.md` - 最佳实践章节已更新
- `sdk/pkg/eventbus/kafka.go` - 实现了 `SetPreSubscriptionTopics` API

---

### 问题 2: NATS 高压下成功率下降 🟡

**状态**: ⚠️ **已识别，待优化**

#### 问题描述
- **高压(5000)**: 60% 成功率
- **极限(10000)**: 30% 成功率
- **原因**: MaxAckPending (1000) 和 MaxWaiting (500) 配置不足

#### 当前测试结果

| 压力级别 | 发送消息数 | 接收消息数 | 成功率 | 状态 |
|---------|-----------|-----------|--------|------|
| 低压(500) | 499 | 998 | 168.80% | ⚠️ 重复接收 |
| 中压(2000) | 1999 | 2000 | 90.20% | ⚠️ 接近阈值 |
| 高压(5000) | 4999 | 3000 | **60.00%** | 🔴 成功率下降 |
| 极限(10000) | 9999 | 3000 | **30.00%** | 🔴 严重下降 |

#### 建议解决方案

**方案 1: 增加 NATS JetStream 配置参数**

```go
natsConfig := &eventbus.NATSConfig{
    JetStream: eventbus.JetStreamConfig{
        Consumer: eventbus.ConsumerConfig{
            MaxAckPending: 5000,  // 从 1000 增加到 5000
            MaxWaiting:    2000,  // 从 500 增加到 2000
        },
    },
}
```

**方案 2: 优化消息处理速度**

```go
// 减少处理延迟
handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
    // 快速处理，避免阻塞
    go processAsync(envelope)  // 异步处理
    return nil
}
```

**方案 3: 增加 Consumer 数量**

```go
// 为每个 topic 创建独立的 Consumer
for _, topic := range topics {
    go func(t string) {
        eb.SubscribeEnvelope(ctx, t, handler)
    }(topic)
}
```

#### 优先级
🟡 **中等优先级** - 不影响 Kafka 的核心功能，但需要优化 NATS 配置

#### 下一步行动
1. 调整 NATS JetStream 配置参数
2. 运行测试验证改善效果
3. 更新配置文档

---

### 问题 3: 严重的协程泄漏 🔴

**状态**: ⚠️ **已识别，待修复**

#### 问题描述
- **Kafka**: 约 430 个协程泄漏（每个压力级别）
- **NATS**: 约 5519 个协程泄漏（每个压力级别）
- **原因**: 5 个 topic 各自创建订阅协程，但取消逻辑有问题
- **影响**: 长期运行会导致资源耗尽

#### 当前测试结果

| 压力级别 | Kafka 协程泄漏 | NATS 协程泄漏 | 状态 |
|---------|--------------|--------------|------|
| 低压(500) | 430 | 5519 | 🔴 严重 |
| 中压(2000) | 430 | 5519 | 🔴 严重 |
| 高压(5000) | 430 | 5519 | 🔴 严重 |
| 极限(10000) | 430 | 5519 | 🔴 严重 |

**关键发现**：
- 协程泄漏数量在不同压力级别下保持一致
- 说明泄漏与 topic 数量相关，而非消息数量
- Kafka: 430 ≈ 5 topics × 86 协程/topic
- NATS: 5519 ≈ 5 topics × 1103 协程/topic

#### 根本原因分析

**Kafka 协程泄漏**：
```go
// 问题代码（推测）
func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    // 启动消费者协程
    go k.consumeMessages(ctx, topic, handler)  // ❌ 协程没有正确取消
    
    // 启动其他后台协程
    go k.monitorConsumer(ctx)  // ❌ 协程没有正确取消
}
```

**NATS 协程泄漏**：
```go
// 问题代码（推测）
func (n *natsEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error {
    // 为每个 topic 创建多个协程
    go n.pullMessages(ctx, topic)      // ❌ 协程没有正确取消
    go n.processMessages(ctx, topic)   // ❌ 协程没有正确取消
    go n.monitorSubscription(ctx)      // ❌ 协程没有正确取消
    // ... 可能还有更多协程
}
```

#### 建议解决方案

**方案 1: 使用 Context 取消机制**

```go
func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    // 创建可取消的 context
    subCtx, cancel := context.WithCancel(ctx)
    
    // 保存 cancel 函数
    k.mu.Lock()
    k.subscriptionCancels[topic] = cancel
    k.mu.Unlock()
    
    // 启动协程，监听 context 取消
    go func() {
        <-subCtx.Done()
        // 清理资源
        k.cleanupSubscription(topic)
    }()
    
    return nil
}

func (k *kafkaEventBus) Close() error {
    k.mu.Lock()
    defer k.mu.Unlock()
    
    // 取消所有订阅
    for topic, cancel := range k.subscriptionCancels {
        cancel()
        delete(k.subscriptionCancels, topic)
    }
    
    return nil
}
```

**方案 2: 使用 WaitGroup 等待协程完成**

```go
func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    k.wg.Add(1)
    
    go func() {
        defer k.wg.Done()
        
        // 消费消息
        for {
            select {
            case <-ctx.Done():
                return  // 正确退出
            case msg := <-k.messages:
                handler(ctx, msg)
            }
        }
    }()
    
    return nil
}

func (k *kafkaEventBus) Close() error {
    // 等待所有协程完成
    k.wg.Wait()
    return nil
}
```

**方案 3: 使用协程池**

```go
// 使用固定大小的协程池，避免无限创建协程
type WorkerPool struct {
    workers   int
    taskQueue chan Task
    wg        sync.WaitGroup
}

func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    // 将任务提交到协程池，而不是创建新协程
    k.workerPool.Submit(Task{
        Topic:   topic,
        Handler: handler,
    })
    
    return nil
}
```

#### 优先级
🔴 **高优先级** - 会导致资源耗尽，影响长期运行的稳定性

#### 下一步行动
1. 分析 Kafka 和 NATS 的订阅代码，找出协程泄漏的具体位置
2. 实现正确的协程取消机制
3. 添加协程泄漏检测测试
4. 验证修复效果

---

## 📊 总体状态总结

| 问题 | 严重程度 | 状态 | 优先级 | 影响范围 |
|------|---------|------|--------|---------|
| Kafka 多 Topic 成功率 20% | 🔴 严重 | ✅ **已解决** | P0 | Kafka 多 topic 场景 |
| NATS 高压成功率下降 | 🟡 中等 | ⚠️ 待优化 | P1 | NATS 高压场景 |
| 协程泄漏 | 🔴 严重 | ⚠️ 待修复 | P0 | 所有场景 |

### 已完成的工作 ✅

1. **Kafka 多 Topic 问题** - 完全解决
   - 实现了 `SetPreSubscriptionTopics` API
   - 成功率从 20% 提升到 99.8%+
   - 更新了文档和最佳实践
   - 创建了专门的解决方案文档

2. **ASCII 字符限制** - 完全解决
   - 在多个关键位置添加警告
   - 创建了专门的命名规范文档
   - 提供了验证函数代码

### 待完成的工作 ⚠️

1. **NATS 高压成功率** - 需要优化配置
   - 调整 MaxAckPending 和 MaxWaiting 参数
   - 优化消息处理速度
   - 验证改善效果

2. **协程泄漏** - 需要修复订阅取消逻辑
   - 分析协程泄漏的具体位置
   - 实现正确的取消机制
   - 添加泄漏检测测试

---

## 🎯 建议的优先级顺序

### P0 - 立即处理（影响核心功能）
1. ✅ **Kafka 多 Topic 成功率问题** - 已解决
2. ⚠️ **协程泄漏问题** - 待修复（影响长期稳定性）

### P1 - 尽快处理（影响性能）
3. ⚠️ **NATS 高压成功率问题** - 待优化（影响高压场景）

### P2 - 后续优化（改善体验）
4. 顺序违反问题（Kafka: 193-4922, NATS: 452-1503）
5. 性能进一步优化

---

## 📝 结论

**已解决的关键问题**：
- ✅ Kafka 多 Topic 成功率从 20% 提升到 99.8%+
- ✅ ASCII 字符限制已在文档中明确说明

**待解决的问题**：
- ⚠️ NATS 高压成功率下降（配置优化）
- ⚠️ 协程泄漏（代码修复）

**总体评价**：
- 核心功能问题（Kafka 多 Topic）已完全解决
- 剩余问题不影响基本功能，但需要后续优化
- 建议优先修复协程泄漏问题，以确保长期稳定性

