# NATS JetStream PublishAsync ACK 处理最佳实践

## 问题

**用户提问**：采用 PublishAsync 后，ACK 应该如何处理？

---

## 核心方案

### 方案对比

| 方案 | 优点 | 缺点 | 推荐度 |
|------|------|------|--------|
| **方案 1：后台 Goroutine 处理** | 简单、性能好 | 每条消息一个 Goroutine | ⭐⭐⭐⭐ 推荐 |
| **方案 2：统一 ACK 处理器** | Goroutine 数量少 | 实现复杂 | ⭐⭐⭐⭐⭐ 最推荐 |
| **方案 3：忽略 ACK** | 最简单 | 无法监控错误 | ❌ 不推荐 |
| **方案 4：同步等待 ACK** | 可靠性高 | 失去性能优势 | ❌ 不推荐 |

---

## 方案 1：后台 Goroutine 处理（简单方案）

### 实现代码

```go
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // 检查连接状态
    n.mu.RLock()
    if n.closed {
        n.mu.RUnlock()
        return fmt.Errorf("eventbus is closed")
    }
    n.mu.RUnlock()

    // 获取主题配置
    topicConfig, _ := n.GetTopicConfig(topic)
    shouldUsePersistent := topicConfig.IsPersistent(n.config.JetStream.Enabled)

    if shouldUsePersistent && n.js != nil {
        // 确保主题在 JetStream 中存在
        if err := n.ensureTopicInJetStream(topic, topicConfig); err != nil {
            n.logger.Warn("Failed to ensure topic in JetStream",
                zap.String("topic", topic),
                zap.Error(err))
            return err
        }

        // ✅ 使用 PublishAsync
        pubOpts := []nats.PubOpt{}
        if n.config.JetStream.PublishTimeout > 0 {
            pubOpts = append(pubOpts, nats.AckWait(n.config.JetStream.PublishTimeout))
        }

        pubAckFuture, err := n.js.PublishAsync(topic, message, pubOpts...)
        if err != nil {
            n.errorCount.Add(1)
            n.logger.Error("Failed to publish message to NATS JetStream",
                zap.String("topic", topic),
                zap.Error(err))
            return err
        }

        // ✅ 后台 Goroutine 处理 ACK
        go n.handlePublishAck(topic, pubAckFuture)

        return nil
    }

    // Core NATS 发布（非持久化）
    err := n.conn.Publish(topic, message)
    if err != nil {
        n.errorCount.Add(1)
        n.logger.Error("Failed to publish message to NATS Core",
            zap.String("topic", topic),
            zap.Error(err))
        return err
    }

    n.publishedMessages.Add(1)
    return nil
}

// handlePublishAck 处理异步发布的 ACK
func (n *natsEventBus) handlePublishAck(topic string, pubAckFuture nats.PubAckFuture) {
    select {
    case <-pubAckFuture.Ok():
        // ✅ 发布成功
        n.publishedMessages.Add(1)
        n.logger.Debug("Message published successfully",
            zap.String("topic", topic))

    case err := <-pubAckFuture.Err():
        // ❌ 发布失败
        n.errorCount.Add(1)
        n.logger.Error("Async publish failed",
            zap.String("topic", topic),
            zap.Error(err))

        // 更新指标
        if n.metrics != nil {
            n.metrics.PublishErrors++
        }
    }
}
```

### 优点

- ✅ **实现简单**：每条消息一个 Goroutine 处理 ACK
- ✅ **性能好**：不阻塞发布流程
- ✅ **错误处理**：可以记录每条消息的发布结果

### 缺点

- ⚠️ **Goroutine 数量多**：每条消息一个 Goroutine
- ⚠️ **资源开销**：高并发时可能创建大量 Goroutine

### 适用场景

- ✅ 消息发布频率不高（< 1000 msg/s）
- ✅ 需要简单实现
- ✅ 不关心 Goroutine 数量

---

## 方案 2：统一 ACK 处理器（推荐方案）

### 实现代码

```go
type natsEventBus struct {
    conn               *nats.Conn
    js                 nats.JetStreamContext
    
    // ✅ 统一 ACK 处理器
    ackHandlerDone     chan struct{}
    ackHandlerWg       sync.WaitGroup
    
    // 其他字段...
}

// 初始化时启动统一 ACK 处理器
func NewNATSEventBus(config NATSConfig) (*natsEventBus, error) {
    // ... 初始化连接 ...

    bus := &natsEventBus{
        conn:           conn,
        js:             js,
        ackHandlerDone: make(chan struct{}),
        // ...
    }

    // ✅ 启动统一 ACK 处理器
    bus.startAckHandler()

    return bus, nil
}

// startAckHandler 启动统一 ACK 处理器
func (n *natsEventBus) startAckHandler() {
    n.ackHandlerWg.Add(1)
    go func() {
        defer n.ackHandlerWg.Done()

        // ✅ 使用 NATS 内置的 ACK 处理机制
        // PublishAsyncErrHandler 会在后台处理所有 ACK 错误
        // 无需为每条消息创建 Goroutine
        
        <-n.ackHandlerDone
    }()
}

// Publish 方法
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // 检查连接状态
    n.mu.RLock()
    if n.closed {
        n.mu.RUnlock()
        return fmt.Errorf("eventbus is closed")
    }
    n.mu.RUnlock()

    // 获取主题配置
    topicConfig, _ := n.GetTopicConfig(topic)
    shouldUsePersistent := topicConfig.IsPersistent(n.config.JetStream.Enabled)

    if shouldUsePersistent && n.js != nil {
        // 确保主题在 JetStream 中存在
        if err := n.ensureTopicInJetStream(topic, topicConfig); err != nil {
            n.logger.Warn("Failed to ensure topic in JetStream",
                zap.String("topic", topic),
                zap.Error(err))
            return err
        }

        // ✅ 使用 PublishAsync
        pubOpts := []nats.PubOpt{}
        if n.config.JetStream.PublishTimeout > 0 {
            pubOpts = append(pubOpts, nats.AckWait(n.config.JetStream.PublishTimeout))
        }

        // ✅ 直接发布，ACK 由统一处理器处理
        _, err := n.js.PublishAsync(topic, message, pubOpts...)
        if err != nil {
            n.errorCount.Add(1)
            n.logger.Error("Failed to publish message to NATS JetStream",
                zap.String("topic", topic),
                zap.Error(err))
            return err
        }

        // ✅ 立即返回，不等待 ACK
        return nil
    }

    // Core NATS 发布（非持久化）
    err := n.conn.Publish(topic, message)
    if err != nil {
        n.errorCount.Add(1)
        n.logger.Error("Failed to publish message to NATS Core",
            zap.String("topic", topic),
            zap.Error(err))
        return err
    }

    n.publishedMessages.Add(1)
    return nil
}

// Close 方法
func (n *natsEventBus) Close() error {
    n.mu.Lock()
    if n.closed {
        n.mu.Unlock()
        return nil
    }
    n.closed = true
    n.mu.Unlock()

    // ✅ 等待所有异步发布完成
    if n.js != nil {
        select {
        case <-n.js.PublishAsyncComplete():
            n.logger.Info("All async publishes completed")
        case <-time.After(30 * time.Second):
            n.logger.Warn("Timeout waiting for async publishes to complete")
        }
    }

    // ✅ 停止 ACK 处理器
    close(n.ackHandlerDone)
    n.ackHandlerWg.Wait()

    // 关闭连接
    n.conn.Close()

    return nil
}
```

### 使用 NATS 内置的 ACK 错误处理器

```go
// 创建 JetStream Context 时配置 ACK 错误处理器
func NewNATSEventBus(config NATSConfig) (*natsEventBus, error) {
    // ... 连接 NATS ...

    // ✅ 配置 PublishAsync 错误处理器
    jsOpts := []nats.JSOpt{
        // 限制未确认消息数量
        nats.PublishAsyncMaxPending(10000),
        
        // ✅ 配置错误处理器
        nats.PublishAsyncErrHandler(func(js nats.JetStream, originalMsg *nats.Msg, err error) {
            logger.Error("Async publish failed",
                zap.String("subject", originalMsg.Subject),
                zap.Int("size", len(originalMsg.Data)),
                zap.Error(err))
            
            // 更新错误计数
            errorCount.Add(1)
            
            // 可以在这里实现重试逻辑
            // 但要注意避免重复消息
        }),
    }

    js, err := conn.JetStream(jsOpts...)
    if err != nil {
        return nil, fmt.Errorf("failed to create JetStream context: %w", err)
    }

    return &natsEventBus{
        conn: conn,
        js:   js,
        // ...
    }, nil
}
```

### 优点

- ✅ **Goroutine 数量少**：使用 NATS 内置的 ACK 处理机制
- ✅ **性能最好**：无需为每条消息创建 Goroutine
- ✅ **资源开销小**：统一处理所有 ACK
- ✅ **优雅关闭**：等待所有异步发布完成

### 缺点

- ⚠️ **实现稍复杂**：需要配置 JetStream Context
- ⚠️ **错误处理粒度粗**：无法精确知道哪条消息失败（除非使用 Msg ID）

### 适用场景

- ✅ **高并发场景**（> 1000 msg/s）
- ✅ **需要最佳性能**
- ✅ **生产环境推荐**

---

## 方案 3：忽略 ACK（不推荐）

### 实现代码

```go
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // ❌ 直接发布，不处理 ACK
    _, err := n.js.PublishAsync(topic, message)
    return err
}
```

### 问题

- ❌ **无法监控错误**：不知道消息是否发布成功
- ❌ **无法调试**：出现问题时难以排查
- ❌ **不符合最佳实践**

### 结论

**❌ 不推荐使用**

---

## 关键配置

### 1. 限制未确认消息数量

```go
// ✅ 防止内存溢出
js, err := conn.JetStream(nats.PublishAsyncMaxPending(10000))
```

**说明**：
- 限制最多 10000 条未确认消息
- 超过限制后，`PublishAsync` 会阻塞等待
- 防止高并发时内存溢出

### 2. 配置 ACK 超时

```go
pubOpts := []nats.PubOpt{
    nats.AckWait(5 * time.Second), // ✅ ACK 超时时间
}

pubAckFuture, err := n.js.PublishAsync(topic, message, pubOpts...)
```

**说明**：
- 5 秒内未收到 ACK，视为超时
- 超时后会触发错误处理器

### 3. 配置消息去重

```go
// ✅ 使用 Msg ID 去重
msgID := generateUniqueID() // 例如：UUID 或 雪花 ID

pubOpts := []nats.PubOpt{
    nats.MsgId(msgID), // ✅ 消息 ID
}

pubAckFuture, err := n.js.PublishAsync(topic, message, pubOpts...)
```

**JetStream 去重机制**：
- ✅ **Duplicates Window**：默认 2 分钟内的重复消息会被去重
- ✅ **Msg ID**：相同 Msg ID 的消息只存储一次

---

## 错误处理策略

### 1. ACK 失败的原因

| 错误类型 | 原因 | 处理方式 |
|---------|------|---------|
| **超时** | 网络延迟、服务器繁忙 | 重试（使用 Msg ID 去重） |
| **连接断开** | 网络故障 | 重连后重试 |
| **权限不足** | 配置错误 | 记录错误，不重试 |
| **Stream 不存在** | 配置错误 | 自动创建 Stream |
| **消息过大** | 消息超过限制 | 记录错误，不重试 |

### 2. 重试策略

```go
func (n *natsEventBus) handlePublishAck(topic string, message []byte, pubAckFuture nats.PubAckFuture) {
    select {
    case <-pubAckFuture.Ok():
        // ✅ 发布成功
        n.publishedMessages.Add(1)

    case err := <-pubAckFuture.Err():
        // ❌ 发布失败
        n.errorCount.Add(1)
        n.logger.Error("Async publish failed",
            zap.String("topic", topic),
            zap.Error(err))

        // ✅ 根据错误类型决定是否重试
        if isRetryableError(err) {
            // 重试（使用 Msg ID 去重）
            go n.retryPublish(topic, message)
        }
    }
}

func isRetryableError(err error) bool {
    // 超时错误可以重试
    if errors.Is(err, nats.ErrTimeout) {
        return true
    }
    
    // 连接错误可以重试
    if errors.Is(err, nats.ErrConnectionClosed) {
        return true
    }
    
    // 其他错误不重试
    return false
}
```

---

## 监控指标

### 1. 关键指标

```go
type PublishMetrics struct {
    // 发布成功数
    PublishedCount atomic.Int64
    
    // 发布失败数
    PublishErrorCount atomic.Int64
    
    // 未确认消息数
    PendingAckCount atomic.Int64
    
    // ACK 超时数
    AckTimeoutCount atomic.Int64
}
```

### 2. 监控 ACK 状态

```go
// 定期检查未确认消息数量
func (n *natsEventBus) monitorPendingAcks() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            // ✅ 获取未确认消息数量
            pending := n.js.PublishAsyncPending()
            
            n.logger.Info("Pending ACKs",
                zap.Int("pending", pending))
            
            // ⚠️ 如果未确认消息过多，发出告警
            if pending > 5000 {
                n.logger.Warn("Too many pending ACKs",
                    zap.Int("pending", pending))
            }
            
        case <-n.ackHandlerDone:
            return
        }
    }
}
```

---

## 最佳实践总结

### ✅ 推荐做法

1. **使用方案 2（统一 ACK 处理器）**：
   - 配置 `PublishAsyncErrHandler`
   - 限制 `PublishAsyncMaxPending`
   - 等待 `PublishAsyncComplete()`

2. **使用 Msg ID 去重**：
   - 防止重复消息
   - 支持安全重试

3. **监控未确认消息数量**：
   - 定期检查 `PublishAsyncPending()`
   - 设置告警阈值

4. **优雅关闭**：
   - 等待所有异步发布完成
   - 超时后强制关闭

### ❌ 不推荐做法

1. **忽略 ACK**：
   - 无法监控错误
   - 不符合最佳实践

2. **同步等待每个 ACK**：
   - 失去性能优势
   - 不如直接使用 `Publish()`

3. **无限重试**：
   - 可能导致重复消息
   - 应该使用 Msg ID 去重

---

## Kafka EventBus 的 ACK 处理实现

### ✅ **Kafka 使用了业界最佳实践**

让我们看看 Kafka EventBus 是如何处理 AsyncProducer 的 ACK 的：

#### 1. 配置 AsyncProducer

```go
// 创建 Sarama 配置
saramaConfig := sarama.NewConfig()

// ✅ 配置 AsyncProducer 返回成功和错误
saramaConfig.Producer.Return.Successes = true
saramaConfig.Producer.Return.Errors = true
```

**关键点**：
- ✅ `Return.Successes = true`：启用成功 ACK 返回
- ✅ `Return.Errors = true`：启用错误 ACK 返回

#### 2. 启动统一 ACK 处理器

```go
// 创建 AsyncProducer
asyncProducer, err := sarama.NewAsyncProducerFromClient(client)

// ✅ 启动统一 ACK 处理器（两个 Goroutine）
go bus.handleAsyncProducerSuccess()
go bus.handleAsyncProducerErrors()
```

**关键点**：
- ✅ **只有 2 个 Goroutine**：一个处理成功，一个处理错误
- ✅ **统一处理所有 Topic 的 ACK**：不是每条消息一个 Goroutine

#### 3. 处理成功 ACK

```go
// 🚀 优化1：处理AsyncProducer成功消息
func (k *kafkaEventBus) handleAsyncProducerSuccess() {
    for success := range k.asyncProducer.Successes() {
        // 记录成功指标
        k.publishedMessages.Add(1)

        // 如果配置了回调，执行回调
        if k.publishCallback != nil {
            // 提取消息内容
            var message []byte
            if success.Value != nil {
                message, _ = success.Value.Encode()
            }
            k.publishCallback(context.Background(), success.Topic, message, nil)
        }
    }
}
```

**关键点**：
- ✅ **单个 Goroutine 处理所有成功 ACK**
- ✅ **更新指标**：`publishedMessages.Add(1)`
- ✅ **执行回调**：如果配置了回调函数

#### 4. 处理错误 ACK

```go
// 🚀 优化1：处理AsyncProducer错误
func (k *kafkaEventBus) handleAsyncProducerErrors() {
    for err := range k.asyncProducer.Errors() {
        // 记录错误
        k.errorCount.Add(1)
        k.logger.Error("Async producer error",
            zap.String("topic", err.Msg.Topic),
            zap.Error(err.Err))

        // 如果配置了回调，执行回调
        if k.publishCallback != nil {
            // 提取消息内容
            var message []byte
            if err.Msg.Value != nil {
                message, _ = err.Msg.Value.Encode()
            }
            k.publishCallback(context.Background(), err.Msg.Topic, message, err.Err)
        }

        // 如果配置了错误处理器，执行错误处理
        if k.errorHandler != nil {
            // 提取消息内容
            var message []byte
            if err.Msg.Value != nil {
                message, _ = err.Msg.Value.Encode()
            }
            k.errorHandler.HandleError(context.Background(), err.Err, message, err.Msg.Topic)
        }
    }
}
```

**关键点**：
- ✅ **单个 Goroutine 处理所有错误 ACK**
- ✅ **更新指标**：`errorCount.Add(1)`
- ✅ **记录日志**：记录错误详情
- ✅ **执行回调**：如果配置了回调函数
- ✅ **执行错误处理器**：如果配置了错误处理器

#### 5. 优雅关闭

```go
func (k *kafkaEventBus) Close() error {
    // ...

    // 🚀 优化1：关闭AsyncProducer
    if k.asyncProducer != nil {
        // ✅ Close() 会等待所有未发送的消息发送完成
        // ✅ Close() 会关闭 Successes 和 Errors channel
        // ✅ handleAsyncProducerSuccess() 和 handleAsyncProducerErrors() 会自动退出
        if err := k.asyncProducer.Close(); err != nil {
            errors = append(errors, fmt.Errorf("failed to close kafka async producer: %w", err))
        }
    }

    // ...
}
```

**关键点**：
- ✅ **等待所有消息发送完成**：`asyncProducer.Close()` 会阻塞等待
- ✅ **自动关闭 channel**：Successes 和 Errors channel 会被关闭
- ✅ **Goroutine 自动退出**：`for range` 循环会自动退出

---

### ✅ **Kafka 实现符合业界最佳实践**

| 最佳实践 | Kafka 实现 | 符合度 |
|---------|-----------|--------|
| **统一 ACK 处理器** | ✅ 2 个 Goroutine 处理所有 ACK | ✅ 完全符合 |
| **限制 Goroutine 数量** | ✅ 不是每条消息一个 Goroutine | ✅ 完全符合 |
| **监控指标** | ✅ 更新 publishedMessages 和 errorCount | ✅ 完全符合 |
| **错误处理** | ✅ 记录日志 + 回调 + 错误处理器 | ✅ 完全符合 |
| **优雅关闭** | ✅ 等待所有消息发送完成 | ✅ 完全符合 |

---

### 📊 **Kafka vs NATS ACK 处理对比**

| 特性 | Kafka (Sarama) | NATS (推荐方案 2) |
|------|---------------|------------------|
| **ACK 处理器** | 2 个 Goroutine（Success + Error） | 1 个 Goroutine + 内置错误处理器 |
| **配置方式** | `Return.Successes = true`<br>`Return.Errors = true` | `PublishAsyncErrHandler(...)`<br>`PublishAsyncMaxPending(10000)` |
| **成功 ACK** | `for success := range Successes()` | 自动处理（无需手动处理） |
| **错误 ACK** | `for err := range Errors()` | `PublishAsyncErrHandler` 回调 |
| **优雅关闭** | `asyncProducer.Close()` | `js.PublishAsyncComplete()` |
| **Goroutine 数量** | 2 个（固定） | 1 个（固定） |

---

### ✅ **结论**

**Kafka EventBus 的 ACK 处理完全符合业界最佳实践**：

1. ✅ **使用统一 ACK 处理器**：2 个 Goroutine 处理所有 Topic 的 ACK
2. ✅ **不是每条消息一个 Goroutine**：避免 Goroutine 数量爆炸
3. ✅ **完善的监控指标**：更新 publishedMessages 和 errorCount
4. ✅ **完善的错误处理**：记录日志 + 回调 + 错误处理器
5. ✅ **优雅关闭**：等待所有消息发送完成

**NATS EventBus 应该采用相同的方案**（推荐方案 2）：

1. ✅ **使用 NATS 内置的 ACK 处理机制**：`PublishAsyncErrHandler`
2. ✅ **限制未确认消息数量**：`PublishAsyncMaxPending(10000)`
3. ✅ **优雅关闭**：`js.PublishAsyncComplete()`
4. ✅ **与 Kafka 保持一致**：使用相同的设计理念

---

**创建时间**：2025-10-12
**更新时间**：2025-10-12
**作者**：Augment Agent
**版本**：v2.0（添加 Kafka 实现分析）

