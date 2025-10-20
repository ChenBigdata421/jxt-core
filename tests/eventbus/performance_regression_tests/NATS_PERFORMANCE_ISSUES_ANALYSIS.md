# NATS 性能测试问题分析报告

## 📊 测试结果总结

### ✅ 测试成功完成
- **所有场景通过**：低压(500)、中压(2000)、高压(5000)、极限(10000)
- **成功率**：100%（Kafka 和 NATS 都是 100%）
- **顺序性**：0 违反（完美保序）

### ❌ 发现的问题

## 🔴 问题 1: NATS Write I/O Timeout

### 现象
```
write tcp [::1]:62311->[::1]:4223: i/o timeout on connection [1189]
```

虽然这个错误没有导致测试失败，但在日志中出现，说明存在潜在的性能瓶颈。

### 根本原因

#### 1. 缺少写入超时配置
**位置**：`sdk/pkg/eventbus/nats.go:488-525`

```go
func buildNATSOptionsInternal(config *NATSConfig) []nats.Option {
    var opts []nats.Option
    
    if config.ClientID != "" {
        opts = append(opts, nats.Name(config.ClientID))
    }
    
    if config.MaxReconnects > 0 {
        opts = append(opts, nats.MaxReconnects(config.MaxReconnects))
    }
    
    if config.ReconnectWait > 0 {
        opts = append(opts, nats.ReconnectWait(config.ReconnectWait))
    }
    
    if config.ConnectionTimeout > 0 {
        opts = append(opts, nats.Timeout(config.ConnectionTimeout))  // ❌ 只设置了连接超时
    }
    
    // ❌ 缺少以下关键配置：
    // - nats.FlusherTimeout()  // 写入刷新超时
    // - nats.PingInterval()    // 心跳间隔
    // - nats.MaxPingsOut()     // 最大未响应心跳数
    
    return opts
}
```

**问题**：
- `nats.Timeout()` 只设置连接建立超时，不影响后续的写入操作
- 缺少 `nats.FlusherTimeout()` 导致写入缓冲区刷新超时使用默认值（可能太短）
- 高并发场景下，TCP 写缓冲区满时会触发 I/O timeout

#### 2. 高并发写入压力
**位置**：`tests/eventbus/performance_tests/kafka_nats_comparison_test.go:728-769`

```go
// 为每个聚合ID创建一个 goroutine 串行发送消息
var sendWg sync.WaitGroup
for aggIndex := 0; aggIndex < aggregateCount; aggIndex++ {
    sendWg.Add(1)
    go func(aggregateIndex int) {
        defer sendWg.Done()
        
        // 极限测试：1000个聚合ID，每个发送10条消息
        // = 1000个并发goroutine同时写入
        for version := int64(1); version <= int64(msgCount); version++ {
            envelope := &eventbus.Envelope{...}
            
            // ❌ 1000个goroutine同时调用PublishEnvelope
            if err := eb.PublishEnvelope(ctx, topic, envelope); err != nil {
                atomic.AddInt64(&metrics.SendErrors, 1)
            }
        }
    }(aggIndex)
}
```

**问题**：
- 极限测试时有 **1000 个并发 goroutine** 同时写入
- 每个 goroutine 调用 `PublishEnvelope` 都会触发网络写入
- TCP 连接的写缓冲区有限，高并发时容易满

#### 3. PublishEnvelope 的异步实现问题
**位置**：`sdk/pkg/eventbus/nats.go:2404-2520`

```go
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // ... 序列化 ...
    
    // 🚀 异步发送消息（立即返回，不等待ACK）
    pubAckFuture, err := n.js.PublishMsgAsync(msg)
    if err != nil {
        return fmt.Errorf("failed to submit async publish: %w", err)
    }
    
    // 🚀 后台处理异步ACK（不阻塞主流程）
    go func() {
        select {
        case <-pubAckFuture.Ok():
            // ✅ 发布成功
            n.publishedMessages.Add(1)
            // ...
        case err := <-pubAckFuture.Err():
            // ❌ 发布失败
            n.errorCount.Add(1)
            // ...
        }
    }()
    
    return nil  // ❌ 立即返回，不等待ACK
}
```

**问题**：
- 虽然使用了 `PublishMsgAsync`，但每次调用都启动一个新的 goroutine
- 没有超时控制，goroutine 可能永久阻塞
- 高并发时会创建大量 goroutine（1000个聚合ID × 每个10条消息 = 10000个goroutine）

### 修复方案

#### 方案 1: 增加 NATS 连接选项配置

```go
func buildNATSOptionsInternal(config *NATSConfig) []nats.Option {
    var opts []nats.Option
    
    // 基础配置
    if config.ClientID != "" {
        opts = append(opts, nats.Name(config.ClientID))
    }
    
    if config.MaxReconnects > 0 {
        opts = append(opts, nats.MaxReconnects(config.MaxReconnects))
    }
    
    if config.ReconnectWait > 0 {
        opts = append(opts, nats.ReconnectWait(config.ReconnectWait))
    }
    
    if config.ConnectionTimeout > 0 {
        opts = append(opts, nats.Timeout(config.ConnectionTimeout))
    }
    
    // ✅ 新增：写入刷新超时（默认2秒，高压场景建议5-10秒）
    opts = append(opts, nats.FlusherTimeout(10*time.Second))
    
    // ✅ 新增：心跳配置（保持连接活跃）
    opts = append(opts, nats.PingInterval(20*time.Second))
    opts = append(opts, nats.MaxPingsOut(3))
    
    // ✅ 新增：发送缓冲区大小（默认32KB，高压场景建议1MB）
    opts = append(opts, nats.ReconnectBufSize(1024*1024))
    
    // 安全配置
    // ...
    
    return opts
}
```

#### 方案 2: 优化 PublishEnvelope 的异步 ACK 处理

```go
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // ... 序列化 ...
    
    // 🚀 异步发送消息
    pubAckFuture, err := n.js.PublishMsgAsync(msg)
    if err != nil {
        return fmt.Errorf("failed to submit async publish: %w", err)
    }
    
    // ✅ 使用带超时的异步ACK处理
    go func() {
        // ✅ 添加超时控制（默认30秒）
        timeout := 30 * time.Second
        if n.config.JetStream.PublishTimeout > 0 {
            timeout = n.config.JetStream.PublishTimeout
        }
        
        select {
        case <-pubAckFuture.Ok():
            // ✅ 发布成功
            n.publishedMessages.Add(1)
            // ...
            
        case err := <-pubAckFuture.Err():
            // ❌ 发布失败
            n.errorCount.Add(1)
            // ...
            
        case <-time.After(timeout):
            // ⏰ 超时
            n.errorCount.Add(1)
            n.logger.Error("Async publish ACK timeout",
                zap.String("subject", topic),
                zap.Duration("timeout", timeout))
        }
    }()
    
    return nil
}
```

#### 方案 3: 使用批量发布优化

```go
// 新增：批量发布方法
func (n *natsEventBus) PublishEnvelopeBatch(ctx context.Context, topic string, envelopes []*Envelope) error {
    // 批量序列化
    messages := make([]*nats.Msg, len(envelopes))
    for i, envelope := range envelopes {
        // ... 序列化 ...
        messages[i] = msg
    }
    
    // 批量异步发布
    futures := make([]nats.PubAckFuture, len(messages))
    for i, msg := range messages {
        future, err := n.js.PublishMsgAsync(msg)
        if err != nil {
            return err
        }
        futures[i] = future
    }
    
    // 统一等待ACK（可选）
    // ...
    
    return nil
}
```

---

## 🔴 问题 2: Goroutine 泄漏

### 现象
```
📊 NATS 资源清理完成: 初始 3 -> 最终 4 (泄漏 1)
📊 NATS 资源清理完成: 初始 4 -> 最终 5 (泄漏 1)
📊 NATS 资源清理完成: 初始 5 -> 最终 6 (泄漏 1)
📊 NATS 资源清理完成: 初始 6 -> 最终 7 (泄漏 1)
```

每次测试都泄漏 **1 个 goroutine**，累积泄漏。

### 根本原因

#### PublishEnvelope 中的异步 ACK 处理 goroutine 泄漏

**位置**：`sdk/pkg/eventbus/nats.go:2468-2520`

```go
// 🚀 后台处理异步ACK（不阻塞主流程）
go func() {
    select {
    case <-pubAckFuture.Ok():
        // ✅ 发布成功
        // ...
    case err := <-pubAckFuture.Err():
        // ❌ 发布失败
        // ...
    }
}()
```

**问题**：
- 没有超时控制，如果 `pubAckFuture` 既不成功也不失败，goroutine 会永久阻塞
- 没有与 EventBus 生命周期绑定，Close() 时无法取消这些 goroutine

### 修复方案

#### 方案 1: 添加超时控制（已在问题1中提出）

#### 方案 2: 使用 context 控制 goroutine 生命周期

```go
type natsEventBus struct {
    // ... 现有字段 ...
    
    // ✅ 新增：异步ACK处理的context
    asyncAckCtx    context.Context
    asyncAckCancel context.CancelFunc
    asyncAckWg     sync.WaitGroup
}

func NewNATSEventBus(config *NATSConfig) (EventBus, error) {
    // ...
    
    // ✅ 创建异步ACK处理的context
    asyncAckCtx, asyncAckCancel := context.WithCancel(context.Background())
    
    bus := &natsEventBus{
        // ...
        asyncAckCtx:    asyncAckCtx,
        asyncAckCancel: asyncAckCancel,
    }
    
    return bus, nil
}

func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // ...
    
    // ✅ 使用WaitGroup跟踪goroutine
    n.asyncAckWg.Add(1)
    go func() {
        defer n.asyncAckWg.Done()
        
        select {
        case <-pubAckFuture.Ok():
            // 成功
        case err := <-pubAckFuture.Err():
            // 失败
        case <-n.asyncAckCtx.Done():
            // ✅ EventBus关闭，退出goroutine
            return
        case <-time.After(30 * time.Second):
            // 超时
        }
    }()
    
    return nil
}

func (n *natsEventBus) Close() error {
    // ...
    
    // ✅ 取消所有异步ACK处理goroutine
    n.asyncAckCancel()
    
    // ✅ 等待所有goroutine退出
    done := make(chan struct{})
    go func() {
        n.asyncAckWg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        n.logger.Info("All async ACK goroutines exited")
    case <-time.After(5 * time.Second):
        n.logger.Warn("Timeout waiting for async ACK goroutines to exit")
    }
    
    // ...
}
```

---

## 🔴 问题 3: NATS 性能显著低于 Kafka

### 性能对比

| 指标 | Kafka | NATS | 差距 |
|------|-------|------|------|
| **极限吞吐量** | 3333 msg/s | 326 msg/s | **10.2倍** |
| **平均发送延迟** | 2.26 ms | 39.04 ms | **17.3倍** |
| **平均处理延迟** | 0.004 ms | 0.016 ms | 4倍 |
| **峰值协程数** | 4452 | 12961 | 2.9倍 |
| **内存增量** | 22.57 MB | 28.26 MB | 1.25倍 |

### 根本原因

#### 1. PublishEnvelope 实现效率低

**Kafka 实现**（高效）：
```go
// Kafka 使用批量异步发送
producer.Input() <- &sarama.ProducerMessage{
    Topic: topic,
    Key:   sarama.StringEncoder(aggregateID),
    Value: sarama.ByteEncoder(data),
}
// 立即返回，批量发送由Sarama内部处理
```

**NATS 实现**（低效）：
```go
// NATS 每次都创建Header、序列化、启动goroutine
msg := &nats.Msg{
    Subject: topic,
    Data:    envelopeBytes,
    Header: nats.Header{  // ❌ 每次都创建Header
        "X-Aggregate-ID":  []string{envelope.AggregateID},
        "X-Event-Version": []string{fmt.Sprintf("%d", envelope.EventVersion)},
        "X-Event-Type":    []string{envelope.EventType},
        "X-Timestamp":     []string{envelope.Timestamp.Format(time.RFC3339)},
    },
}

pubAckFuture, err := n.js.PublishMsgAsync(msg)
// ❌ 每次都启动一个新的goroutine等待ACK
go func() {
    select {
    case <-pubAckFuture.Ok():
    case err := <-pubAckFuture.Err():
    }
}()
```

#### 2. 缺少批量发布优化

Kafka 的 Sarama 库内部实现了批量发送优化：
- 消息在内存中缓冲
- 达到批量大小或超时后统一发送
- 减少网络往返次数

NATS 当前实现：
- 每条消息单独发送
- 没有批量缓冲机制
- 网络往返次数多

### 修复方案

#### 方案 1: 简化 PublishEnvelope 实现

```go
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    n.mu.RLock()
    defer n.mu.RUnlock()
    
    if n.closed {
        return fmt.Errorf("nats eventbus is closed")
    }
    
    // 序列化Envelope
    envelopeBytes, err := envelope.ToBytes()
    if err != nil {
        n.errorCount.Add(1)
        return fmt.Errorf("failed to serialize envelope: %w", err)
    }
    
    // ✅ 简化：直接发布，不创建Header（Header可选）
    _, err = n.js.PublishAsync(topic, envelopeBytes)
    if err != nil {
        n.errorCount.Add(1)
        return fmt.Errorf("failed to publish: %w", err)
    }
    
    // ✅ 不启动goroutine，由统一的ACK错误处理器处理
    return nil
}
```

#### 方案 2: 实现批量发布

```go
type natsEventBus struct {
    // ...
    
    // ✅ 批量发布缓冲区
    publishBuffer     []*nats.Msg
    publishBufferMu   sync.Mutex
    publishBufferSize int
    publishTicker     *time.Ticker
}

func (n *natsEventBus) startBatchPublisher() {
    n.publishTicker = time.NewTicker(10 * time.Millisecond)
    go func() {
        for range n.publishTicker.C {
            n.flushPublishBuffer()
        }
    }()
}

func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // ... 序列化 ...
    
    msg := &nats.Msg{
        Subject: topic,
        Data:    envelopeBytes,
    }
    
    // ✅ 添加到缓冲区
    n.publishBufferMu.Lock()
    n.publishBuffer = append(n.publishBuffer, msg)
    shouldFlush := len(n.publishBuffer) >= n.publishBufferSize
    n.publishBufferMu.Unlock()
    
    // ✅ 达到批量大小，立即刷新
    if shouldFlush {
        n.flushPublishBuffer()
    }
    
    return nil
}

func (n *natsEventBus) flushPublishBuffer() {
    n.publishBufferMu.Lock()
    if len(n.publishBuffer) == 0 {
        n.publishBufferMu.Unlock()
        return
    }
    
    messages := n.publishBuffer
    n.publishBuffer = make([]*nats.Msg, 0, n.publishBufferSize)
    n.publishBufferMu.Unlock()
    
    // ✅ 批量发布
    for _, msg := range messages {
        n.js.PublishMsgAsync(msg)
    }
}
```

---

## 📋 修复优先级

### P0 - 立即修复
1. **Goroutine 泄漏**：添加 context 控制和超时机制
2. **Write I/O Timeout**：增加 FlusherTimeout 配置

### P1 - 短期优化
3. **简化 PublishEnvelope**：移除不必要的 Header 创建
4. **优化异步 ACK 处理**：减少 goroutine 创建

### P2 - 长期优化
5. **实现批量发布**：提升吞吐量
6. **性能基准测试**：持续监控性能指标

---

## 🎯 预期效果

修复后预期性能提升：
- **吞吐量**：从 326 msg/s 提升到 **2000+ msg/s**（6倍提升）
- **延迟**：从 39ms 降低到 **5ms 以内**（8倍提升）
- **Goroutine 泄漏**：**0 泄漏**
- **I/O Timeout**：**0 错误**

