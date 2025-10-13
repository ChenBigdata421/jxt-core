# NATS EventBus 重构方案 - 业界最佳实践

## 🎯 重构目标

1. **性能提升 10 倍**：从 327 msg/s 提升到 3000+ msg/s
2. **消除 goroutine 泄漏**：0 泄漏
3. **简化架构**：移除不必要的复杂性
4. **采用业界最佳实践**

---

## 📊 业界最佳实践分析

### 1. NATS 官方推荐

根据 NATS 官方文档和 Stack Overflow 讨论：

#### ✅ **使用批量 ACK 检查**
```go
// ❌ 错误做法：每条消息都启动一个 goroutine 等待 ACK
for i := 0; i < 10000; i++ {
    future, _ := js.PublishAsync(subject, data)
    go func() {
        select {
        case <-future.Ok():
        case <-future.Err():
        }
    }()
}

// ✅ 正确做法：批量检查 ACK（参考 nats bench）
futures := make([]nats.PubAckFuture, 0, batchSize)
for i := 0; i < messageCount; i++ {
    future, _ := js.PublishAsync(subject, data)
    futures = append(futures, future)
    
    // 每 100 条消息检查一次 ACK
    if len(futures) >= 100 {
        checkAcks(futures)
        futures = futures[:0]
    }
}
// 检查剩余的 ACK
checkAcks(futures)

func checkAcks(futures []nats.PubAckFuture) {
    for _, f := range futures {
        select {
        case <-f.Ok():
            // 成功
        case err := <-f.Err():
            // 失败
        case <-time.After(5 * time.Second):
            // 超时
        }
    }
}
```

#### ✅ **使用 PublishAsyncMaxPending 限制未确认消息**
```go
js, _ := nc.JetStream(
    nats.PublishAsyncMaxPending(256),  // 限制未确认消息数量
)
```

#### ✅ **不要为每条消息创建 Header**
```go
// ❌ 错误做法：每条消息都创建 Header
for i := 0; i < 10000; i++ {
    msg := &nats.Msg{
        Subject: topic,
        Data:    data,
        Header: nats.Header{  // 每次都创建新的 Header
            "X-ID": []string{id},
        },
    }
    js.PublishMsgAsync(msg)
}

// ✅ 正确做法：直接发布数据，Header 可选
for i := 0; i < 10000; i++ {
    js.PublishAsync(topic, data)  // 更快
}
```

### 2. Kafka vs NATS 性能对比

根据社区讨论和基准测试：

| 特性 | Kafka | NATS JetStream |
|------|-------|----------------|
| **吞吐量** | 高（批量优化） | 高（需要正确使用） |
| **延迟** | 中等（批量导致延迟） | 低（单条消息快） |
| **复杂度** | 高 | 低 |
| **最佳场景** | 大批量数据处理 | 实时消息传递 |

**关键发现**：
- NATS 的性能瓶颈通常是**使用方式不当**，而非 NATS 本身
- 正确使用 NATS 可以达到与 Kafka 相当甚至更好的性能

---

## 🔧 重构方案

### 方案 1: 批量 ACK 检查（推荐）

**核心思想**：不为每条消息创建 goroutine，而是批量检查 ACK

```go
type natsEventBus struct {
    // ... 现有字段 ...
    
    // ✅ 批量 ACK 管理
    ackBatchSize    int
    ackCheckTimeout time.Duration
}

func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // 序列化
    data, err := envelope.ToBytes()
    if err != nil {
        return err
    }
    
    // ✅ 直接异步发布，不等待 ACK
    _, err = n.js.PublishAsync(topic, data)
    return err
}

// ✅ 新增：批量发布方法
func (n *natsEventBus) PublishEnvelopeBatch(ctx context.Context, topic string, envelopes []*Envelope) error {
    futures := make([]nats.PubAckFuture, 0, len(envelopes))
    
    for _, envelope := range envelopes {
        data, err := envelope.ToBytes()
        if err != nil {
            return err
        }
        
        future, err := n.js.PublishAsync(topic, data)
        if err != nil {
            return err
        }
        futures = append(futures, future)
    }
    
    // ✅ 批量检查 ACK
    return n.checkAcks(futures)
}

func (n *natsEventBus) checkAcks(futures []nats.PubAckFuture) error {
    var errs []error
    timeout := 5 * time.Second
    if n.config.JetStream.PublishTimeout > 0 {
        timeout = n.config.JetStream.PublishTimeout
    }
    
    for _, future := range futures {
        select {
        case <-future.Ok():
            n.publishedMessages.Add(1)
        case err := <-future.Err():
            n.errorCount.Add(1)
            errs = append(errs, err)
        case <-time.After(timeout):
            n.errorCount.Add(1)
            errs = append(errs, fmt.Errorf("ACK timeout"))
        }
    }
    
    if len(errs) > 0 {
        return fmt.Errorf("failed to publish %d messages: %v", len(errs), errs)
    }
    return nil
}
```

### 方案 2: 单个 ACK 处理 Goroutine（备选）

**核心思想**：使用一个共享的 goroutine 处理所有 ACK

```go
type natsEventBus struct {
    // ... 现有字段 ...
    
    // ✅ 共享 ACK 处理
    ackChan    chan nats.PubAckFuture
    ackCtx     context.Context
    ackCancel  context.CancelFunc
    ackWg      sync.WaitGroup
}

func (n *natsEventBus) startAckProcessor() {
    n.ackWg.Add(1)
    go func() {
        defer n.ackWg.Done()
        
        for {
            select {
            case future := <-n.ackChan:
                n.processAck(future)
            case <-n.ackCtx.Done():
                return
            }
        }
    }()
}

func (n *natsEventBus) processAck(future nats.PubAckFuture) {
    timeout := 5 * time.Second
    if n.config.JetStream.PublishTimeout > 0 {
        timeout = n.config.JetStream.PublishTimeout
    }
    
    select {
    case <-future.Ok():
        n.publishedMessages.Add(1)
    case err := <-future.Err():
        n.errorCount.Add(1)
        n.logger.Error("Async publish failed", zap.Error(err))
    case <-time.After(timeout):
        n.errorCount.Add(1)
        n.logger.Error("Async publish ACK timeout")
    }
}

func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    data, err := envelope.ToBytes()
    if err != nil {
        return err
    }
    
    future, err := n.js.PublishAsync(topic, data)
    if err != nil {
        return err
    }
    
    // ✅ 发送到共享的 ACK 处理 goroutine
    select {
    case n.ackChan <- future:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### 方案 3: 完全异步（最快，但需要外部监控）

**核心思想**：完全依赖 NATS 的内部 ACK 处理

```go
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    data, err := envelope.ToBytes()
    if err != nil {
        return err
    }
    
    // ✅ 完全异步，依赖 PublishAsyncErrHandler
    _, err = n.js.PublishAsync(topic, data)
    return err
}

// 在 NewNATSEventBus 中配置全局错误处理器
js, err := nc.JetStream(
    nats.PublishAsyncMaxPending(256),
    nats.PublishAsyncErrHandler(func(js nats.JetStream, msg *nats.Msg, err error) {
        bus.errorCount.Add(1)
        bus.logger.Error("Async publish failed",
            zap.String("subject", msg.Subject),
            zap.Error(err))
    }),
)
```

---

## 🎯 推荐方案：混合方案

结合方案 1 和方案 3 的优点：

1. **默认使用完全异步**（方案 3）- 最快
2. **提供批量发布方法**（方案 1）- 用于需要确认的场景
3. **移除所有 per-message goroutine** - 消除泄漏

```go
// ✅ 快速发布（不等待 ACK）
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    data, err := envelope.ToBytes()
    if err != nil {
        return err
    }
    _, err = n.js.PublishAsync(topic, data)
    return err
}

// ✅ 批量发布（等待 ACK）
func (n *natsEventBus) PublishEnvelopeBatch(ctx context.Context, topic string, envelopes []*Envelope) error {
    // 实现批量 ACK 检查
}

// ✅ 同步发布（等待单个 ACK）
func (n *natsEventBus) PublishEnvelopeSync(ctx context.Context, topic string, envelope *Envelope) error {
    data, err := envelope.ToBytes()
    if err != nil {
        return err
    }
    _, err = n.js.Publish(topic, data)  // 同步发布
    return err
}
```

---

## 📈 预期性能提升

### 修复前
- **吞吐量**: 327 msg/s
- **延迟**: 21.68 ms
- **峰值协程数**: 15463
- **Goroutine 泄漏**: 1 个/测试

### 修复后（预期）
- **吞吐量**: **3000+ msg/s** (9倍提升)
- **延迟**: **< 3 ms** (7倍提升)
- **峰值协程数**: **< 5000** (3倍减少)
- **Goroutine 泄漏**: **0 个** (完全消除)

---

## 🔄 迁移步骤

### 第 1 步：简化 PublishEnvelope
- 移除 per-message goroutine
- 移除 Header 创建（可选）
- 使用完全异步发布

### 第 2 步：优化连接配置
- 增加 FlusherTimeout
- 增加 ReconnectBufSize
- 配置 PublishAsyncMaxPending

### 第 3 步：添加批量发布方法
- 实现 PublishEnvelopeBatch
- 实现批量 ACK 检查

### 第 4 步：测试验证
- 运行性能测试
- 验证 goroutine 泄漏
- 验证消息顺序性

---

## 📝 代码示例

完整的重构代码将在下一步实施。

---

## 🎓 参考资料

1. **NATS 官方文档**:
   - https://docs.nats.io/nats-concepts/jetstream
   - https://docs.nats.io/using-nats/developer/sending/async

2. **Stack Overflow 讨论**:
   - https://stackoverflow.com/questions/70550060/performance-of-nats-jetstream
   - 关键建议：使用批量 ACK 检查，参考 `nats bench` 实现

3. **NATS Bench 源码**:
   - https://github.com/nats-io/natscli/blob/main/cli/bench_command.go
   - 业界标准的性能测试工具

4. **最佳实践**:
   - 不要为每条消息创建 goroutine
   - 使用 PublishAsync 而非 Publish
   - 批量检查 ACK
   - 限制 PublishAsyncMaxPending
   - 使用全局错误处理器

