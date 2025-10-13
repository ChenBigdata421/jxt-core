# NATS JetStream 性能优化方案

## 📋 背景

我们已经对 EventBus + Kafka 进行了全面的性能调优，采用了多个业界优秀实践（AsyncProducer、LZ4压缩、批处理优化、Consumer Fetch优化、Worker Pool优化），取得了显著的性能提升。

但是，EventBus + NATS JetStream 还没有进行任何性能调优。根据对比测试结果，NATS JetStream 在高负载场景下的性能明显低于 Kafka：

| 指标 | Kafka (极限压力) | NATS (极限压力) | 差距 |
|------|-----------------|----------------|------|
| **吞吐量** | 995.65 msg/s | 242.92 msg/s | **Kafka快4.1倍** |
| **内存使用** | 4.56MB | 9.47MB | **NATS高2.1倍** |
| **Goroutine数量** | 83 | 1025 | **NATS高12.3倍** |
| **扩展性** | 1892%增长 | 77%增长 | **Kafka好24倍** |

本文档分析 NATS JetStream 的代码，提出基于业界优秀实践的性能优化方案。

---

## 🔍 当前实现分析

### 1. 发布端 (Publisher)

#### 当前实现 (`sdk/pkg/eventbus/nats.go` 第603-693行)

```go
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // ...
    if shouldUsePersistent && n.js != nil {
        // 使用JetStream发布（持久化）
        var pubOpts []nats.PubOpt
        if n.config.JetStream.PublishTimeout > 0 {
            pubOpts = append(pubOpts, nats.AckWait(n.config.JetStream.PublishTimeout))
        }
        
        // ❌ 同步发布 - 每次调用都等待ACK
        _, err = n.js.Publish(topic, message, pubOpts...)
        // ...
    }
    // ...
}
```

**问题**：
- ❌ **同步发布**：每次 `Publish()` 调用都会阻塞等待 JetStream 服务器的 ACK
- ❌ **无批处理**：每条消息单独发送，无法利用批处理优化
- ❌ **无压缩**：消息未压缩，浪费网络带宽
- ❌ **高延迟**：同步等待导致发布延迟高

### 2. 消费端 (Consumer)

#### 当前实现 (`sdk/pkg/eventbus/nats.go` 第854-894行)

```go
func (n *natsEventBus) processUnifiedPullMessages(ctx context.Context, topic string, sub *nats.Subscription) {
    for {
        // ❌ 每次只拉取10条消息
        msgs, err := sub.Fetch(10, nats.MaxWait(time.Second))
        if err != nil {
            if err == nats.ErrTimeout {
                continue // Timeout is normal, continue pulling
            }
            // ... error handling
        }
        
        // 处理消息
        for _, msg := range msgs {
            // ... handle message
        }
    }
}
```

**问题**：
- ❌ **批量大小太小**：每次只拉取 10 条消息，导致频繁的网络往返
- ❌ **MaxWait 太长**：1秒的等待时间在高负载场景下浪费时间
- ❌ **无并发拉取**：单个 goroutine 串行拉取消息

### 3. 连接管理

#### 当前实现

```go
type natsEventBus struct {
    conn               *nats.Conn           // ✅ 单个连接（符合 NATS 官方最佳实践）
    js                 nats.JetStreamContext
    // ...
}
```

**分析**：
- ✅ **单连接是正确的**：符合 NATS 官方推荐的最佳实践
- ✅ **NATS 协议支持多路复用**：单个连接可以处理多个 topic 的发布和订阅
- ✅ **库内部自动管理**：自动重连、维护订阅

**NATS 官方最佳实践**（来源：NATS 维护者 @mtmk）：
> "**one connection per service is the recommended approach**. you can have as many publishers as you want on the same connection. NATS protocol is asynchronous and can handle multiple publishers without stalling."

**结论**：
- ✅ **保持单连接**：每个 EventBus 实例一个连接（Singleton）
- ❌ **不需要连接池**：NATS 协议已经支持多路复用
- ❌ **不需要每个 Topic 一个连接**：会浪费资源且不符合官方推荐

### 4. Goroutine 管理

#### 当前实现

**Kafka 的优化**（已实施）：
```go
type kafkaEventBus struct {
    globalWorkerPool *GlobalWorkerPool           // ✅ 全局Worker池（无聚合ID消息）
    keyedPools       map[string]*KeyedWorkerPool // ✅ Per-topic Keyed池（有聚合ID消息）
}

// 处理消息
func (h *kafkaConsumerGroupHandler) ConsumeClaim(...) {
    aggregateID := ExtractAggregateID(message.Value, ...)

    if aggregateID != "" {
        // ✅ 有聚合ID：使用Keyed-Worker池（保证顺序）
        pool := h.eventBus.keyedPools[topic]
        pool.ProcessMessage(ctx, aggMsg)
    } else {
        // ✅ 无聚合ID：使用全局Worker池（节省Goroutine）
        h.eventBus.globalWorkerPool.SubmitWork(workItem)
    }
}
```

**NATS 当前实现**（未优化）：
```go
type natsEventBus struct {
    // ❌ 没有全局Worker池
    keyedPools map[string]*KeyedWorkerPool // ✅ Per-topic Keyed池（有聚合ID消息）
}

// 处理消息
func (n *natsEventBus) handleMessage(...) {
    aggregateID := ExtractAggregateID(data, ...)

    if aggregateID != "" {
        // ✅ 有聚合ID：使用Keyed-Worker池（保证顺序）
        pool := n.keyedPools[topic]
        pool.ProcessMessage(ctx, aggMsg)
    } else {
        // ❌ 无聚合ID：直接在Fetcher goroutine中处理（浪费资源）
        handler(ctx, data)
    }
}
```

**问题**：
- ❌ **Goroutine 数量过多**：测试显示 1025 个 Goroutine（是 Kafka 的 12.3 倍）
- ❌ **无全局 Worker 池**：无聚合 ID 的消息直接在 Fetcher goroutine 中处理
- ❌ **资源浪费**：每个消息处理都可能阻塞 Fetcher goroutine

---

## 🎯 优化方案

基于 NATS 官方文档、业界最佳实践和 Kafka 优化经验，提出以下 8 个优化方向：

---

### 优化 1: 异步发布 (Async Publishing) + 统一 ACK 处理器

**参考**：
- NATS 官方文档：[JetStream Async Publishing](https://docs.nats.io/nats-concepts/jetstream)
- GitHub Issue: [nats-server#4799](https://github.com/nats-io/nats-server/issues/4799) - "use PublishAsync in reasonable batches"
- Kafka EventBus 实现：使用统一 ACK 处理器（2 个 Goroutine）

**当前问题**：
- 同步发布每次都等待 ACK，延迟高
- 无法利用批处理优化

---

#### 方案 A：每条消息一个 Goroutine（不推荐）❌

```go
// ❌ 不推荐：每条消息创建一个 Goroutine
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    pubAckFuture, err := n.js.PublishAsync(topic, message)
    if err != nil {
        return err
    }

    // ❌ 每条消息一个 Goroutine
    go func() {
        select {
        case <-pubAckFuture.Ok():
            n.publishedMessages.Add(1)
        case err := <-pubAckFuture.Err():
            n.errorCount.Add(1)
            n.logger.Error("Async publish failed", zap.Error(err))
        }
    }()

    return nil
}
```

**缺点**：
- ❌ **Goroutine 数量爆炸**：每条消息一个 Goroutine，高负载下可能创建数千个 Goroutine
- ❌ **资源浪费**：每个 Goroutine 占用内存（约 2-8KB）
- ❌ **调度开销**：大量 Goroutine 增加调度器负担

---

#### 方案 B：统一 ACK 处理器（推荐）✅

**参考 Kafka EventBus 的实现**：

Kafka EventBus 使用 **2 个 Goroutine** 统一处理所有 Topic 的 ACK：

```go
// Kafka EventBus 的实现
type kafkaEventBus struct {
    asyncProducer sarama.AsyncProducer
    // ...
}

// 初始化时启动 2 个 Goroutine
func NewKafkaEventBus(...) {
    // ...
    go bus.handleAsyncProducerSuccess()  // ✅ 处理成功 ACK
    go bus.handleAsyncProducerErrors()   // ✅ 处理错误 ACK
}

// 处理成功 ACK
func (k *kafkaEventBus) handleAsyncProducerSuccess() {
    for success := range k.asyncProducer.Successes() {
        k.publishedMessages.Add(1)
        if k.publishCallback != nil {
            k.publishCallback(context.Background(), success.Topic, message, nil)
        }
    }
}

// 处理错误 ACK
func (k *kafkaEventBus) handleAsyncProducerErrors() {
    for err := range k.asyncProducer.Errors() {
        k.errorCount.Add(1)
        k.logger.Error("Async producer error", zap.Error(err.Err))
        if k.publishCallback != nil {
            k.publishCallback(context.Background(), err.Msg.Topic, message, err.Err)
        }
    }
}
```

**NATS 的推荐实现**（使用 NATS 内置机制）：

```go
type natsEventBus struct {
    conn *nats.Conn
    js   nats.JetStreamContext
    // ...
}

// 初始化时配置统一 ACK 处理器
func NewNATSEventBus(...) {
    // ✅ 使用 NATS 内置的 ACK 错误处理器
    jsOpts := []nats.JSOpt{
        // 限制未确认消息数量（防止内存溢出）
        nats.PublishAsyncMaxPending(10000),

        // ✅ 统一错误处理器（只处理错误，成功自动处理）
        nats.PublishAsyncErrHandler(func(js nats.JetStream, originalMsg *nats.Msg, err error) {
            bus.errorCount.Add(1)
            bus.logger.Error("Async publish failed",
                zap.String("topic", originalMsg.Subject),
                zap.Error(err))

            // 如果配置了回调，执行回调
            if bus.publishCallback != nil {
                bus.publishCallback(context.Background(), originalMsg.Subject, originalMsg.Data, err)
            }
        }),
    }

    js, err := conn.JetStream(jsOpts...)
    // ...
}

// 发布消息
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // ✅ 异步发布（不等待 ACK）
    pubAckFuture, err := n.js.PublishAsync(topic, message)
    if err != nil {
        return err
    }

    // ✅ 成功的 ACK 由 NATS 内部自动处理
    // ✅ 错误的 ACK 由 PublishAsyncErrHandler 统一处理

    return nil
}

// 优雅关闭
func (n *natsEventBus) Close() error {
    // ✅ 等待所有异步发布完成
    select {
    case <-n.js.PublishAsyncComplete():
        n.logger.Info("All async publishes completed")
    case <-time.After(30 * time.Second):
        n.logger.Warn("Timeout waiting for async publishes to complete")
    }

    // ...
}
```

**优点**：
- ✅ **Goroutine 数量固定**：NATS 内部使用固定数量的 Goroutine 处理 ACK
- ✅ **资源高效**：不会因为消息数量增加而创建更多 Goroutine
- ✅ **与 Kafka 保持一致**：使用相同的设计理念（统一 ACK 处理器）
- ✅ **实现简单**：使用 NATS 内置机制，无需手动管理 Goroutine
- ✅ **优雅关闭**：`PublishAsyncComplete()` 等待所有消息发送完成

---

#### Kafka vs NATS ACK 处理对比

| 特性 | Kafka (Sarama) | NATS (推荐方案) |
|------|---------------|-----------------|
| **ACK 处理器** | 2 个 Goroutine（Success + Error） | 1 个 Goroutine + 内置错误处理器 |
| **配置方式** | `Return.Successes = true`<br>`Return.Errors = true` | `PublishAsyncErrHandler(...)`<br>`PublishAsyncMaxPending(10000)` |
| **成功 ACK** | `for success := range Successes()` | 自动处理（无需手动处理） |
| **错误 ACK** | `for err := range Errors()` | `PublishAsyncErrHandler` 回调 |
| **优雅关闭** | `asyncProducer.Close()` | `js.PublishAsyncComplete()` |
| **Goroutine 数量** | 2 个（固定） | 1 个（固定） |

---

**预期收益**：
- ✅ **吞吐量提升 3-5倍**：异步发布消除阻塞等待
- ✅ **延迟降低 50-70%**：不需要等待每条消息的 ACK
- ✅ **批处理优化**：NATS 内部会自动批处理异步发布的消息
- ✅ **Goroutine 数量固定**：不会因为消息数量增加而创建更多 Goroutine
- ✅ **与 Kafka 保持一致**：使用相同的设计理念

**实现复杂度**：⭐⭐ (中等)

**风险**：
- ⚠️ 需要配置 `PublishAsyncMaxPending` 限制未确认消息数量（防止内存溢出）
- ⚠️ 需要在 Close 时等待所有异步发布完成（使用 `PublishAsyncComplete()`）

---

#### 🎯 **Outbox 模式集成：如何通知 Outbox 发送成功/失败**

在 Outbox 模式中，需要知道消息是否发送成功，以便更新 Outbox 表的状态。异步发布模式下有以下几种方案：

---

##### 方案 1：使用回调函数（推荐）✅

**核心思路**：通过回调函数通知 Outbox 发送结果

```go
// 1. 定义 EventBus 的发布回调接口
type PublishCallback func(ctx context.Context, topic string, message []byte, err error)

type natsEventBus struct {
    conn            *nats.Conn
    js              nats.JetStreamContext
    publishCallback PublishCallback  // ✅ 发布回调
    // ...
}

// 2. 配置统一 ACK 处理器（带回调）
func NewNATSEventBus(config NATSConfig, callback PublishCallback) (*natsEventBus, error) {
    // ...

    // ✅ 配置 PublishAsyncErrHandler（处理错误 ACK）
    jsOpts := []nats.JSOpt{
        nats.PublishAsyncMaxPending(10000),
        nats.PublishAsyncErrHandler(func(js nats.JetStream, originalMsg *nats.Msg, err error) {
            bus.errorCount.Add(1)
            bus.logger.Error("Async publish failed",
                zap.String("topic", originalMsg.Subject),
                zap.Error(err))

            // ✅ 通知 Outbox 发送失败
            if bus.publishCallback != nil {
                bus.publishCallback(context.Background(), originalMsg.Subject, originalMsg.Data, err)
            }
        }),
    }

    js, err := conn.JetStream(jsOpts...)

    bus := &natsEventBus{
        conn:            conn,
        js:              js,
        publishCallback: callback,
        // ...
    }

    // ✅ 启动成功 ACK 处理器
    go bus.handleSuccessAcks()

    return bus, nil
}

// 3. 处理成功 ACK（需要手动实现）
func (n *natsEventBus) handleSuccessAcks() {
    // NATS 没有内置的成功 ACK 回调，需要手动实现
    // 方案：使用 PubAckFuture 跟踪每条消息

    // 这里需要维护一个 map 来跟踪消息
    // 详见方案 2
}

// 4. 发布 Envelope 消息（带 Event ID）
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // 校验 Envelope
    if err := envelope.Validate(); err != nil {
        return fmt.Errorf("invalid envelope: %w", err)
    }

    // 序列化 Envelope
    envelopeBytes, err := envelope.ToBytes()
    if err != nil {
        n.errorCount.Add(1)
        return fmt.Errorf("failed to serialize envelope: %w", err)
    }

    // ✅ 创建 NATS 消息（带 Event ID 作为 Msg ID）
    msg := &nats.Msg{
        Subject: topic,
        Data:    envelopeBytes,
        Header: nats.Header{
            "X-Aggregate-ID":  []string{envelope.AggregateID},
            "X-Event-Type":    []string{envelope.EventType},
            "X-Event-Version": []string{fmt.Sprintf("%d", envelope.EventVersion)},
            "X-Timestamp":     []string{envelope.Timestamp.Format(time.RFC3339)},
        },
    }

    // ✅ 添加可选字段到 Header
    if envelope.EventID != "" {
        msg.Header.Set("X-Event-ID", envelope.EventID)
        msg.Header.Set("Nats-Msg-Id", envelope.EventID)  // ✅ 使用 Event ID 作为 Msg ID（幂等性保证）
    }
    if envelope.TraceID != "" {
        msg.Header.Set("X-Trace-ID", envelope.TraceID)
    }
    if envelope.CorrelationID != "" {
        msg.Header.Set("X-Correlation-ID", envelope.CorrelationID)
    }

    // ✅ 使用 PublishMsgAsync 异步发布
    pubAckFuture, err := n.js.PublishMsgAsync(msg)
    if err != nil {
        n.errorCount.Add(1)
        n.logger.Error("Failed to publish envelope message",
            zap.String("topic", topic),
            zap.String("eventID", envelope.EventID),
            zap.String("aggregateID", envelope.AggregateID),
            zap.Error(err))

        // ✅ 立即通知 Outbox 发送失败
        if n.publishCallback != nil {
            n.publishCallback(ctx, topic, envelopeBytes, err)
        }
        return err
    }

    // ✅ 后台处理成功 ACK
    go func() {
        select {
        case <-pubAckFuture.Ok():
            n.publishedMessages.Add(1)
            n.logger.Debug("Envelope message published successfully",
                zap.String("topic", topic),
                zap.String("eventID", envelope.EventID),
                zap.String("aggregateID", envelope.AggregateID))

            // ✅ 通知 Outbox 发送成功
            if n.publishCallback != nil {
                n.publishCallback(context.Background(), topic, envelopeBytes, nil)
            }
        case err := <-pubAckFuture.Err():
            // 错误已经由 PublishAsyncErrHandler 处理
        }
    }()

    return nil
}
```

**Outbox 服务使用示例**：

```go
// evidence-management/command/internal/application/service/media.go
type mediaService struct {
    repo           repository.MediaRepository
    eventPublisher publisher.EventPublisher
    outboxRepo     event_repository.OutboxRepository
    txManager      transaction.TransactionManager
}

// 初始化 EventBus（带回调）
func NewMediaService(...) *mediaService {
    // ✅ 创建 EventBus 时传入回调函数
    eventBus, _ := eventbus.NewNATSEventBus(config, func(ctx context.Context, topic string, message []byte, err error) {
        // ✅ 从消息中提取 Event ID
        var envelope eventbus.Envelope
        if unmarshalErr := json.Unmarshal(message, &envelope); unmarshalErr != nil {
            logger.Error("Failed to unmarshal envelope", zap.Error(unmarshalErr))
            return
        }

        eventID := envelope.EventID
        if eventID == "" {
            logger.Warn("Envelope missing EventID")
            return
        }

        if err != nil {
            // ❌ 发送失败：记录错误，等待重试
            logger.Error("Event publish failed",
                zap.String("eventID", eventID),
                zap.String("aggregateID", envelope.AggregateID),
                zap.String("eventType", envelope.EventType),
                zap.Error(err))
            // 可选：增加重试计数
            // outboxRepo.IncrementRetryCount(ctx, eventID)
        } else {
            // ✅ 发送成功：标记为已发布
            if markErr := outboxRepo.MarkAsPublished(ctx, eventID); markErr != nil {
                logger.Error("Failed to mark event as published",
                    zap.String("eventID", eventID),
                    zap.Error(markErr))
            } else {
                logger.Info("Event published successfully",
                    zap.String("eventID", eventID),
                    zap.String("aggregateID", envelope.AggregateID),
                    zap.String("eventType", envelope.EventType))
            }
        }
    })

    return &mediaService{
        eventPublisher: eventBus,
        outboxRepo:     outboxRepo,
        // ...
    }
}

// 立即发布事件（异步，不影响主流程）
func (s *mediaService) publishEventsImmediately(ctx context.Context, eventIDs []string) {
    go func() {
        events, _ := s.outboxRepo.FindUnpublishedEventsByEventIDs(ctx, eventIDs)
        for _, outboxEvent := range events {
            topic := s.getTopicByAggregateType(outboxEvent.AggregateType)
            domainEvent, _ := outboxEvent.ToDomainEvent()

            // ✅ 创建 Envelope（包含 Event ID）
            payload, _ := domainEvent.MarshalJSON()
            envelope := eventbus.NewEnvelope(
                outboxEvent.AggregateID,
                outboxEvent.EventType,
                outboxEvent.EventVersion,
                payload,
            )

            // ✅ 设置 Event ID（用于幂等性保证和回调识别）
            envelope.EventID = outboxEvent.ID

            // ✅ 设置追踪信息（可选）
            envelope.TraceID = fmt.Sprintf("outbox-%s", outboxEvent.ID)
            envelope.CorrelationID = outboxEvent.AggregateID

            // ✅ 发布 Envelope 消息（成功/失败由回调处理）
            if err := s.eventPublisher.PublishEnvelope(ctx, topic, envelope); err != nil {
                s.logger.Error("Failed to publish envelope",
                    zap.String("eventID", outboxEvent.ID),
                    zap.Error(err))
                // 发布失败会立即触发回调，无需在这里处理
            }

            // ⚠️ 注意：不在这里调用 MarkAsPublished
            // 由回调函数在收到 ACK 后调用
        }
    }()
}
```

**优点**：
- ✅ **准确性高**：只有收到 JetStream ACK 才标记为已发布
- ✅ **与 Kafka 保持一致**：Kafka 也使用回调函数
- ✅ **支持重试**：失败的消息可以由定时任务重试
- ✅ **幂等性保证**：使用 Envelope.EventID 作为 NATS Msg ID 防止重复发布
- ✅ **使用 Envelope 标准格式**：符合当前 EventBus 设计，包含完整元数据

**缺点**：
- ⚠️ **需要每条消息一个 Goroutine**：处理成功 ACK（可以优化为批量处理）
- ⚠️ **实现复杂度中等**：需要维护消息 ID 和回调的映射

**关键点**：
- ✅ **Envelope 需要添加 EventID 字段**：用于幂等性保证和回调识别
- ✅ **EventID 作为 NATS Msg ID**：通过 `Nats-Msg-Id` Header 设置
- ✅ **回调中解析 Envelope**：从消息字节中提取 EventID

---

##### 方案 2：使用 Channel 通知（高性能）✅

**核心思路**：使用 Channel 批量通知 Outbox 发送结果

```go
// 1. 定义发布结果
type PublishResult struct {
    EventID   string
    Topic     string
    Message   []byte
    Success   bool
    Error     error
    Timestamp time.Time
}

type natsEventBus struct {
    conn          *nats.Conn
    js            nats.JetStreamContext
    resultChan    chan *PublishResult  // ✅ 结果通知 Channel
    // ...
}

// 2. 初始化 EventBus
func NewNATSEventBus(config NATSConfig) (*natsEventBus, error) {
    // ...

    bus := &natsEventBus{
        conn:       conn,
        js:         js,
        resultChan: make(chan *PublishResult, 10000),  // ✅ 缓冲 Channel
        // ...
    }

    // ✅ 配置错误处理器
    jsOpts := []nats.JSOpt{
        nats.PublishAsyncMaxPending(10000),
        nats.PublishAsyncErrHandler(func(js nats.JetStream, originalMsg *nats.Msg, err error) {
            // ✅ 从消息头中提取 Event ID
            eventID := originalMsg.Header.Get("Event-ID")

            // ✅ 发送失败结果到 Channel
            bus.resultChan <- &PublishResult{
                EventID:   eventID,
                Topic:     originalMsg.Subject,
                Message:   originalMsg.Data,
                Success:   false,
                Error:     err,
                Timestamp: time.Now(),
            }
        }),
    }

    return bus, nil
}

// 3. 发布 Envelope 消息（带 Event ID）
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // 校验 Envelope
    if err := envelope.Validate(); err != nil {
        return fmt.Errorf("invalid envelope: %w", err)
    }

    // 序列化 Envelope
    envelopeBytes, err := envelope.ToBytes()
    if err != nil {
        n.errorCount.Add(1)
        return fmt.Errorf("failed to serialize envelope: %w", err)
    }

    // ✅ 创建 NATS 消息（带 Event ID 头）
    msg := nats.NewMsg(topic)
    msg.Data = envelopeBytes
    msg.Header.Set("X-Event-ID", envelope.EventID)
    msg.Header.Set("X-Aggregate-ID", envelope.AggregateID)
    msg.Header.Set("X-Event-Type", envelope.EventType)
    msg.Header.Set("Nats-Msg-Id", envelope.EventID)  // ✅ 幂等性保证

    // 添加可选字段
    if envelope.TraceID != "" {
        msg.Header.Set("X-Trace-ID", envelope.TraceID)
    }
    if envelope.CorrelationID != "" {
        msg.Header.Set("X-Correlation-ID", envelope.CorrelationID)
    }

    pubAckFuture, err := n.js.PublishMsgAsync(msg)
    if err != nil {
        n.errorCount.Add(1)
        // ✅ 立即发送失败结果
        n.resultChan <- &PublishResult{
            EventID:   envelope.EventID,
            Topic:     topic,
            Message:   envelopeBytes,
            Success:   false,
            Error:     err,
            Timestamp: time.Now(),
        }
        return err
    }

    // ✅ 后台处理成功 ACK
    go func() {
        select {
        case <-pubAckFuture.Ok():
            n.publishedMessages.Add(1)
            // ✅ 发送成功结果
            n.resultChan <- &PublishResult{
                EventID:   envelope.EventID,
                Topic:     topic,
                Message:   envelopeBytes,
                Success:   true,
                Error:     nil,
                Timestamp: time.Now(),
            }
        case err := <-pubAckFuture.Err():
            // 错误已经由 PublishAsyncErrHandler 处理
        }
    }()

    return nil
}

// 4. 获取结果 Channel
func (n *natsEventBus) GetResultChannel() <-chan *PublishResult {
    return n.resultChan
}
```

**Outbox 服务使用示例**：

```go
// evidence-management/command/internal/application/service/outbox_publisher.go
type OutboxPublisher struct {
    eventBus   eventbus.EventBus
    outboxRepo event_repository.OutboxRepository
    logger     *zap.Logger
}

// 启动结果处理器
func (p *OutboxPublisher) Start(ctx context.Context) {
    // ✅ 监听发布结果 Channel
    resultChan := p.eventBus.GetResultChannel()

    go func() {
        for {
            select {
            case result := <-resultChan:
                if result.Success {
                    // ✅ 发送成功：标记为已发布
                    if err := p.outboxRepo.MarkAsPublished(ctx, result.EventID); err != nil {
                        p.logger.Error("Failed to mark event as published",
                            zap.String("eventID", result.EventID),
                            zap.Error(err))
                    }
                } else {
                    // ❌ 发送失败：记录错误
                    p.logger.Error("Event publish failed",
                        zap.String("eventID", result.EventID),
                        zap.Error(result.Error))
                    // 可选：增加重试计数
                    // p.outboxRepo.IncrementRetryCount(ctx, result.EventID)
                }

            case <-ctx.Done():
                return
            }
        }
    }()
}

// 发布事件
func (p *OutboxPublisher) PublishEvents(ctx context.Context, eventIDs []string) {
    events, _ := p.outboxRepo.FindUnpublishedEventsByEventIDs(ctx, eventIDs)
    for _, outboxEvent := range events {
        topic := p.getTopicByAggregateType(outboxEvent.AggregateType)
        domainEvent, _ := outboxEvent.ToDomainEvent()

        // ✅ 创建 Envelope（包含 Event ID）
        payload, _ := domainEvent.MarshalJSON()
        envelope := eventbus.NewEnvelope(
            outboxEvent.AggregateID,
            outboxEvent.EventType,
            outboxEvent.EventVersion,
            payload,
        )

        // ✅ 设置 Event ID（用于幂等性保证和结果识别）
        envelope.EventID = outboxEvent.ID
        envelope.TraceID = fmt.Sprintf("outbox-%s", outboxEvent.ID)
        envelope.CorrelationID = outboxEvent.AggregateID

        // ✅ 发布 Envelope 消息
        if err := p.eventBus.PublishEnvelope(ctx, topic, envelope); err != nil {
            p.logger.Error("Failed to publish envelope",
                zap.String("eventID", outboxEvent.ID),
                zap.Error(err))
        }

        // ⚠️ 不在这里调用 MarkAsPublished
        // 由结果处理器在收到 ACK 后调用
    }
}
```

**优点**：
- ✅ **高性能**：批量处理结果，减少数据库操作
- ✅ **解耦**：发布和状态更新完全解耦
- ✅ **可扩展**：可以添加更多的结果处理逻辑（监控、告警等）
- ✅ **与 Kafka 保持一致**：Kafka 也使用 Channel 处理结果
- ✅ **使用 Envelope 标准格式**：符合当前 EventBus 设计

**缺点**：
- ⚠️ **需要每条消息一个 Goroutine**：处理成功 ACK
- ⚠️ **需要维护 Channel**：需要正确关闭和清理

**关键点**：
- ✅ **Envelope 需要添加 EventID 字段**：用于幂等性保证和结果识别
- ✅ **EventID 作为 NATS Msg ID**：通过 `Nats-Msg-Id` Header 设置
- ✅ **结果处理器解耦**：独立的 Goroutine 处理发布结果

---

##### 方案 3：乐观更新 + 定时校验（简化方案）⚠️

**核心思路**：发布后立即标记为已发布，定时任务校验并重试失败的消息

```go
// 立即发布事件（乐观更新）
func (s *mediaService) publishEventsImmediately(ctx context.Context, eventIDs []string) {
    go func() {
        events, _ := s.outboxRepo.FindUnpublishedEventsByEventIDs(ctx, eventIDs)
        for _, outboxEvent := range events {
            topic := s.getTopicByAggregateType(outboxEvent.AggregateType)
            domainEvent, _ := outboxEvent.ToDomainEvent()

            // ✅ 发布消息
            if err := s.eventPublisher.Publish(ctx, topic, domainEvent); err != nil {
                // ❌ 发布失败：记录错误，不标记为已发布
                s.logger.Error("Event publish failed",
                    zap.String("eventID", outboxEvent.ID),
                    zap.Error(err))
                continue
            }

            // ✅ 发布成功（PublishAsync 返回 nil）：立即标记为已发布
            // ⚠️ 注意：这里是乐观更新，实际 ACK 可能还没收到
            s.outboxRepo.MarkAsPublished(ctx, outboxEvent.ID)
        }
    }()
}

// 定时任务：校验并重试
func (s *mediaService) retryFailedEvents(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            // ✅ 查找超过 5 分钟仍未发布的事件
            events, _ := s.outboxRepo.FindUnpublishedEventsOlderThan(ctx, 5*time.Minute)

            for _, event := range events {
                // ✅ 重试发布
                topic := s.getTopicByAggregateType(event.AggregateType)
                domainEvent, _ := event.ToDomainEvent()

                if err := s.eventPublisher.Publish(ctx, topic, domainEvent); err == nil {
                    s.outboxRepo.MarkAsPublished(ctx, event.ID)
                }
            }

        case <-ctx.Done():
            return
        }
    }
}
```

**优点**：
- ✅ **实现简单**：无需回调或 Channel
- ✅ **性能好**：无需等待 ACK
- ✅ **最终一致性**：定时任务保证最终发布成功

**缺点**：
- ❌ **不准确**：可能标记为已发布但实际 ACK 失败
- ❌ **延迟高**：失败的消息需要等待定时任务重试
- ❌ **不推荐用于异步发布**：失去了异步发布的准确性优势

---

##### 📊 **方案对比**

| 方案 | 准确性 | 性能 | 实现复杂度 | 推荐度 |
|------|--------|------|-----------|--------|
| **方案 1：回调函数** | ✅ 高 | ⭐⭐⭐⭐ 好 | ⭐⭐⭐ 中等 | ⭐⭐⭐⭐⭐ 最推荐 |
| **方案 2：Channel 通知** | ✅ 高 | ⭐⭐⭐⭐⭐ 最好 | ⭐⭐⭐⭐ 复杂 | ⭐⭐⭐⭐ 推荐 |
| **方案 3：乐观更新** | ❌ 低 | ⭐⭐⭐⭐⭐ 最好 | ⭐ 简单 | ⭐⭐ 不推荐 |

---

##### ✅ **推荐方案**

**对于 Outbox 模式 + NATS JetStream 异步发布**：

1. **首选方案 1（回调函数）**：
   - ✅ 准确性高：只有收到 ACK 才标记为已发布
   - ✅ 与 Kafka 保持一致
   - ✅ 实现复杂度适中
   - ✅ **使用 PublishEnvelope**：符合当前 EventBus 设计
   - ✅ **Envelope.EventID 作为 Msg ID**：保证幂等性

2. **高性能场景选方案 2（Channel 通知）**：
   - ✅ 批量处理结果，性能最好
   - ✅ 解耦发布和状态更新
   - ✅ **使用 PublishEnvelope**：符合当前 EventBus 设计
   - ⚠️ 实现复杂度较高

3. **不推荐方案 3（乐观更新）**：
   - ❌ 失去了异步发布的准确性优势
   - ❌ 可能导致消息丢失（标记为已发布但实际 ACK 失败）

---

##### 📋 **Envelope 结构需要添加 EventID 字段**

为了支持 Outbox 模式，需要在 `Envelope` 结构中添加 `EventID` 字段：

```go
// sdk/pkg/eventbus/envelope.go
type Envelope struct {
    EventID       string     `json:"event_id,omitempty"`       // ✅ 新增：事件ID（用于Outbox模式）
    AggregateID   string     `json:"aggregate_id"`             // 聚合ID（必填）
    EventType     string     `json:"event_type"`               // 事件类型（必填）
    EventVersion  int64      `json:"event_version"`            // 事件版本
    Timestamp     time.Time  `json:"timestamp"`                // 时间戳
    TraceID       string     `json:"trace_id,omitempty"`       // 链路追踪ID（可选）
    CorrelationID string     `json:"correlation_id,omitempty"` // 关联ID（可选）
    Payload       RawMessage `json:"payload"`                  // 业务负载
}
```

**EventID 字段的用途**：
1. ✅ **幂等性保证**：作为 NATS Msg ID，防止重复发布
2. ✅ **回调识别**：在回调函数中识别是哪个 Outbox 事件
3. ✅ **结果通知**：在 Channel 通知中识别事件
4. ✅ **追踪和调试**：完整的事件生命周期追踪

**修改建议**：
- ✅ `EventID` 字段为可选（`omitempty`），不影响现有代码
- ✅ Outbox 模式下必须设置 `EventID`
- ✅ 非 Outbox 模式下可以不设置 `EventID`

---

### 优化 2: 增大批量拉取大小 (Larger Fetch Batch Size)

**参考**：
- Byron Ruth 博客：[Grokking NATS Consumers: Pull-based](https://www.byronruth.com/grokking-nats-consumers-part-3/)
  - "The batch size is the maximum number of messages you want to handle in a single call"
  - "Ensure you can ack all of them within the AckWait window (default is 30 seconds)"

**当前问题**：
- 每次只拉取 10 条消息，频繁的网络往返
- 高负载场景下无法充分利用网络带宽

**优化方案**：
```go
// 根据压力级别动态调整批量大小
batchSize := 100  // 低压：100
// batchSize := 250  // 中压：250
// batchSize := 500  // 高压：500
// batchSize := 1000 // 极限：1000

msgs, err := sub.Fetch(batchSize, nats.MaxWait(100*time.Millisecond))
```

**批量大小选择原则**：
1. **确保在 AckWait 窗口内处理完**：默认 30 秒
2. **根据消息处理速度调整**：如果每条消息处理 10ms，30 秒可以处理 3000 条
3. **考虑内存限制**：批量大小 × 消息大小 < 可用内存

**预期收益**：
- ✅ **吞吐量提升 5-10倍**：减少网络往返次数
- ✅ **CPU 使用率降低**：减少系统调用次数
- ✅ **延迟降低**：批量处理更高效

**实现复杂度**：⭐ (简单)

**风险**：
- ⚠️ 批量太大可能导致 AckWait 超时
- ⚠️ 需要根据实际场景调优

---

### 优化 3: 缩短 MaxWait 时间 (Shorter MaxWait)

**参考**：
- NATS 官方文档：默认 MaxWait 是 3 秒，但可以根据场景调整

**当前问题**：
- MaxWait 设置为 1 秒，在高负载场景下浪费时间
- 如果消息到达速度快，不需要等待这么久

**优化方案**：
```go
// 高负载场景：缩短 MaxWait
msgs, err := sub.Fetch(batchSize, nats.MaxWait(100*time.Millisecond))

// 低负载场景：保持较长 MaxWait
msgs, err := sub.Fetch(batchSize, nats.MaxWait(500*time.Millisecond))
```

**预期收益**：
- ✅ **延迟降低 50-90%**：减少不必要的等待时间
- ✅ **吞吐量提升 10-20%**：更快地拉取下一批消息

**实现复杂度**：⭐ (简单)

**风险**：
- ⚠️ MaxWait 太短可能导致频繁的空拉取（浪费 CPU）

---

### 优化 4: 并发拉取消息 (Concurrent Fetch) - ⚠️ **需要保证顺序性**

**参考**：
- Byron Ruth 博客：多个订阅者可以绑定到同一个 Pull Consumer 并发拉取消息

**当前问题**：
- 单个 goroutine 串行拉取消息，无法充分利用多核 CPU

**⚠️ 重要约束**：
- **必须保证同一个聚合 ID 的消息严格按顺序处理**
- 当前实现已经使用 `KeyedWorkerPool` 保证了顺序性

**优化方案（保证顺序性）**：

#### 方案 A：单 Fetcher + KeyedWorkerPool（当前实现）✅

```go
// 当前实现：单个 goroutine 拉取，KeyedWorkerPool 保证顺序
func (n *natsEventBus) processUnifiedPullMessages(ctx context.Context, topic string, sub *nats.Subscription) {
    for {
        // ✅ 单个 goroutine 拉取消息（保证拉取顺序）
        msgs, err := sub.Fetch(batchSize, nats.MaxWait(100*time.Millisecond))

        // ✅ KeyedWorkerPool 保证同一个聚合 ID 的消息顺序处理
        for _, msg := range msgs {
            aggregateID := ExtractAggregateID(msg.Data)
            pool.ProcessMessage(ctx, &AggregateMessage{
                AggregateID: aggregateID,
                Value:       msg.Data,
                // ...
            })
        }
    }
}
```

**优点**：
- ✅ **顺序性保证**：拉取顺序 + KeyedWorkerPool 双重保证
- ✅ **实现简单**：当前已实现
- ✅ **无风险**：不会破坏顺序性

**缺点**：
- ❌ **拉取吞吐量受限**：单 goroutine 拉取可能成为瓶颈

#### 方案 B：多 Fetcher + 全局顺序队列（复杂）❌

```go
// ❌ 不推荐：实现复杂，收益有限
// 多个 goroutine 并发拉取，但需要全局队列保证顺序
type OrderedMessageQueue struct {
    queue    chan *nats.Msg
    sequence uint64
    mu       sync.Mutex
}

// 多个 Fetcher 并发拉取
for i := 0; i < numFetchers; i++ {
    go func() {
        msgs, _ := sub.Fetch(batchSize, nats.MaxWait(100*time.Millisecond))
        // 需要按序号排序后再处理...
    }()
}
```

**缺点**：
- ❌ **实现复杂**：需要全局排序逻辑
- ❌ **性能开销**：排序和同步开销大
- ❌ **收益有限**：KeyedWorkerPool 已经并发处理

#### 方案 C：增大批量大小（推荐）✅

```go
// ✅ 推荐：增大批量大小，单 Fetcher 也能高吞吐
func (n *natsEventBus) processUnifiedPullMessages(ctx context.Context, topic string, sub *nats.Subscription) {
    // ✅ 增大批量大小到 500-1000
    batchSize := 500

    for {
        // 单次拉取更多消息，减少网络往返
        msgs, err := sub.Fetch(batchSize, nats.MaxWait(100*time.Millisecond))

        // KeyedWorkerPool 并发处理不同聚合 ID 的消息
        for _, msg := range msgs {
            // ... 处理消息
        }
    }
}
```

**优点**：
- ✅ **顺序性保证**：单 Fetcher 保证拉取顺序
- ✅ **高吞吐量**：批量大小 500-1000，减少网络往返
- ✅ **并发处理**：KeyedWorkerPool 并发处理不同聚合 ID
- ✅ **实现简单**：只需修改批量大小

**预期收益**：
- ✅ **吞吐量提升 5-10倍**：批量大小从 10 增加到 500-1000
- ✅ **保证顺序性**：单 Fetcher + KeyedWorkerPool
- ✅ **充分利用多核**：KeyedWorkerPool 内部并发处理

**实现复杂度**：⭐ (简单)

**风险**：
- ⚠️ 批量太大可能导致 AckWait 超时（需要调优）

---

**最终推荐**：
- ✅ **采用方案 C**：增大批量大小（10 → 500-1000）
- ✅ **保持单 Fetcher**：保证拉取顺序
- ✅ **依赖 KeyedWorkerPool**：保证同一聚合 ID 的消息顺序处理
- ❌ **不使用多 Fetcher**：避免破坏顺序性

---

### 优化 5: 连接管理优化 - ❌ **不推荐修改（保持单连接）**

**NATS 官方最佳实践**：
- ✅ **每个服务一个连接（Singleton）**
- ✅ **每个 EventBus 实例一个连接**

**参考**：
- NATS 官方文档：docs.nats.io/using-nats/developer/connecting
- NATS 维护者 @mtmk（GitHub Discussion #654）：
  > "**one connection per service is the recommended approach**. you can have as many publishers as you want on the same connection. NATS protocol is asynchronous and can handle multiple publishers without stalling."

**当前实现分析**：
```go
type natsEventBus struct {
    conn *nats.Conn           // ✅ 单个连接（正确）
    js   nats.JetStreamContext
    // ...
}
```

**为什么单连接是正确的**：
1. ✅ **NATS 协议支持多路复用**：单个连接可以处理多个 topic
2. ✅ **减少资源开销**：每个连接都有 TCP 握手、心跳、内存开销
3. ✅ **简化管理**：单连接更容易监控和调试
4. ✅ **自动重连**：库内部会自动处理重连和订阅维护
5. ✅ **顺序性保证**：单连接天然保证同一发布者的消息顺序

**❌ 不推荐的方案**：

#### 方案 A：每个 Topic 一个连接（不推荐）

```go
type natsEventBus struct {
    // ❌ 不推荐：每个 topic 一个连接
    topicConns map[string]*nats.Conn
    topicJS    map[string]nats.JetStreamContext
    connMu     sync.RWMutex
}
```

**为什么不推荐**：
- ❌ **不符合 NATS 官方最佳实践**：官方推荐每个服务一个连接
- ❌ **浪费资源**：每个连接都有 TCP 握手、心跳、内存、Goroutine 开销
- ❌ **增加复杂度**：需要管理多个连接的生命周期、重连逻辑
- ❌ **没有性能优势**：NATS 协议已经支持多路复用，单连接足够高效

#### 方案 B：连接池（轮询）（不推荐）

```go
type natsEventBus struct {
    // ❌ 不推荐：连接池（轮询）
    connPool []*nats.Conn
    jsPool   []nats.JetStreamContext
    index    atomic.Uint32
}

func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // ❌ 轮询获取连接
    idx := n.index.Add(1) % uint32(len(n.connPool))
    js := n.jsPool[idx]

    _, err := js.PublishAsync(topic, message)
    return err
}
```

**为什么不推荐**：
- ❌ **破坏顺序性**：同一个 topic 的消息可能通过不同连接发布，到达顺序不确定
- ❌ **不符合官方推荐**：NATS 官方推荐单连接
- ❌ **增加复杂度**：需要管理连接池

**✅ 推荐方案：保持单连接（当前实现）**

```go
type natsEventBus struct {
    conn *nats.Conn           // ✅ 单个连接（Singleton）
    js   nats.JetStreamContext
    // ...
}

func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // ✅ 使用同一个 JetStream Context
    _, err := n.js.PublishAsync(topic, message)
    return err
}
```

**优点**：
- ✅ **符合 NATS 官方最佳实践**
- ✅ **简单高效**：单连接，易于管理
- ✅ **顺序性保证**：单连接天然保证发布顺序
- ✅ **资源节约**：最少的连接、内存、Goroutine
- ✅ **自动重连**：库内部自动处理

**预期收益**：
- ✅ **无需修改**：当前实现已经是最佳实践
- ✅ **性能优化重点**：异步发布、批量大小、MaxWait、配置优化

**实现复杂度**：⭐ (无需修改)

**结论**：
- ✅ **保持单连接**：每个 EventBus 实例一个连接
- ❌ **不需要每个 Topic 一个连接**
- ❌ **不需要连接池**

---

### 优化 6: 消息压缩 (Message Compression)

**参考**：
- Kafka 优化经验：LZ4 压缩可以减少 50-70% 的网络带宽

**当前问题**：
- NATS JetStream 不支持内置压缩
- 消息未压缩，浪费网络带宽

**优化方案**：
```go
// 在应用层进行压缩
import "github.com/pierrec/lz4/v4"

func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // 压缩消息
    compressed := make([]byte, lz4.CompressBlockBound(len(message)))
    compressedSize, err := lz4.CompressBlock(message, compressed, nil)
    if err != nil {
        return err
    }
    compressed = compressed[:compressedSize]
    
    // 发布压缩后的消息
    _, err = n.js.PublishAsync(topic, compressed)
    return err
}

// 消费端解压缩
func (n *natsEventBus) handleMessage(msg *nats.Msg) {
    // 解压缩消息
    decompressed := make([]byte, maxMessageSize)
    decompressedSize, err := lz4.UncompressBlock(msg.Data, decompressed)
    if err != nil {
        // 处理错误
        return
    }
    decompressed = decompressed[:decompressedSize]
    
    // 处理解压缩后的消息
    // ...
}
```

**预期收益**：
- ✅ **网络带宽减少 50-70%**：压缩后消息更小
- ✅ **吞吐量提升 30-50%**：网络传输更快

**实现复杂度**：⭐⭐ (中等)

**风险**：
- ⚠️ 增加 CPU 开销（压缩/解压缩）
- ⚠️ 需要在发布端和消费端都实现
- ⚠️ 需要处理不可压缩的消息（如已压缩的图片）

---

### 优化 7: 使用全局 Worker 池减少 Goroutine 数量 (Global Worker Pool) - 🔴 **高优先级**

**参考**：
- Kafka 优化经验：使用全局 Worker 池处理无聚合 ID 的消息
- 测试结果：NATS 使用 1025 个 Goroutine，是 Kafka 的 12.3 倍（83 个）

**当前问题**：
- ❌ **没有全局 Worker 池**：无聚合 ID 的消息直接在 Fetcher goroutine 中处理
- ❌ **Goroutine 数量过多**：1025 个（Kafka 只有 83 个）
- ❌ **资源浪费**：每个消息处理都可能阻塞 Fetcher goroutine

**Kafka 的优化方案**（已实施）：
```go
type kafkaEventBus struct {
    globalWorkerPool *GlobalWorkerPool           // ✅ 全局Worker池
    keyedPools       map[string]*KeyedWorkerPool // ✅ Per-topic Keyed池
}

// GlobalWorkerPool 全局Worker池
type GlobalWorkerPool struct {
    workers     []*Worker
    workQueue   chan WorkItem
    workerCount int  // 默认：CPU核心数 × 2
    queueSize   int  // 默认：workerCount × 100
}

// 处理消息
func (h *kafkaConsumerGroupHandler) ConsumeClaim(...) {
    aggregateID := ExtractAggregateID(message.Value, ...)

    if aggregateID != "" {
        // ✅ 有聚合ID：使用Keyed-Worker池（保证顺序）
        pool := h.eventBus.keyedPools[topic]
        pool.ProcessMessage(ctx, aggMsg)
    } else {
        // ✅ 无聚合ID：使用全局Worker池（节省Goroutine）
        workItem := WorkItem{
            Message: message,
            Handler: handler,
            Session: session,
        }
        h.eventBus.globalWorkerPool.SubmitWork(workItem)
    }
}
```

**NATS 优化方案**（推荐实施）：
```go
type natsEventBus struct {
    conn               *nats.Conn
    js                 nats.JetStreamContext

    // ✅ 添加全局Worker池（与Kafka保持一致）
    globalWorkerPool   *GlobalWorkerPool
    keyedPools         map[string]*KeyedWorkerPool
}

// 初始化时创建全局Worker池
func NewNATSEventBus(config NATSConfig) (*natsEventBus, error) {
    // ...

    // ✅ 创建全局Worker池
    globalWorkerPool := NewGlobalWorkerPool(0, logger) // 0表示使用默认worker数量

    return &natsEventBus{
        conn:             conn,
        js:               js,
        globalWorkerPool: globalWorkerPool,
        keyedPools:       make(map[string]*KeyedWorkerPool),
        // ...
    }, nil
}

// 处理消息
func (n *natsEventBus) handleMessage(ctx context.Context, topic string, data []byte, handler MessageHandler, ackFunc func() error) {
    aggregateID := ExtractAggregateID(data, ...)

    if aggregateID != "" {
        // ✅ 有聚合ID：使用Keyed-Worker池（保证顺序）
        pool := n.keyedPools[topic]
        pool.ProcessMessage(ctx, aggMsg)
    } else {
        // ✅ 无聚合ID：使用全局Worker池（节省Goroutine）
        workItem := WorkItem{
            Topic:   topic,
            Data:    data,
            Handler: handler,
            AckFunc: ackFunc,
        }
        n.globalWorkerPool.SubmitWork(workItem)
    }
}

// 关闭时停止全局Worker池
func (n *natsEventBus) Close() error {
    // ...

    // ✅ 停止全局Worker池
    if n.globalWorkerPool != nil {
        n.globalWorkerPool.Close()
    }

    // ...
}
```

**预期收益**：
- ✅ **Goroutine 数量降低 80-90%**：从 1025 降低到 100-200（与 Kafka 相当）
- ✅ **内存使用降低 50-70%**：减少 Goroutine 栈内存（从 9.47MB → 3-5MB）
- ✅ **调度开销降低**：减少 Goroutine 切换
- ✅ **与 Kafka 保持一致**：使用相同的全局 Worker 池设计

**实现复杂度**：⭐⭐ (中等)

**风险**：
- ⚠️ 需要复用 Kafka 的 GlobalWorkerPool 代码
- ⚠️ 需要适配 NATS 的消息确认机制（AckFunc）

---

### 优化 8: 配置优化 (Configuration Tuning)

**参考**：
- NATS 官方文档：[JetStream Configuration](https://docs.nats.io/nats-concepts/jetstream)

**当前配置问题**：
```go
Consumer: NATSConsumerConfig{
    MaxAckPending: 500,   // ❌ 可能太小
    MaxWaiting:    200,   // ❌ 可能太小
}
```

**优化方案**：
```go
Consumer: NATSConsumerConfig{
    MaxAckPending: 10000,  // ✅ 增大到 10000（允许更多未确认消息）
    MaxWaiting:    1000,   // ✅ 增大到 1000（允许更多并发拉取请求）
    AckWait:       30 * time.Second,  // ✅ 确保足够的处理时间
}

Stream: StreamConfig{
    MaxBytes: 1024 * 1024 * 1024,  // ✅ 增大到 1GB（更大的缓冲）
    MaxMsgs:  1000000,              // ✅ 增大到 100万条消息
}
```

**预期收益**：
- ✅ **吞吐量提升 20-30%**：减少配置限制
- ✅ **稳定性提升**：避免因配置不足导致的错误

**实现复杂度**：⭐ (简单)

**风险**：
- ⚠️ 增大配置可能增加内存使用

---

## 📊 优化优先级和预期收益（更新：符合 NATS 官方最佳实践 + 全局 Worker 池）

| 优化项 | 优先级 | 实现复杂度 | 预期吞吐量提升 | 预期延迟降低 | 预期内存降低 | Goroutine降低 | 顺序性影响 | 符合官方推荐 |
|--------|--------|-----------|---------------|-------------|-------------|--------------|-----------|-------------|
| **1. 异步发布 + 统一ACK处理器** | 🔴 高 | ⭐⭐ 中等 | **3-5倍** | **50-70%** | - | ✅ 固定数量 | ✅ 无影响 | ✅ **是（Kafka经验）** |
| **2. 增大批量大小** | 🔴 高 | ⭐ 简单 | **5-10倍** | **30-50%** | - | - | ✅ 无影响 | ✅ 是 |
| **3. 缩短 MaxWait** | 🟡 中 | ⭐ 简单 | **10-20%** | **50-90%** | - | - | ✅ 无影响 | ✅ 是 |
| **4. 并发拉取** | ❌ 不推荐 | ⭐⭐ 中等 | ~~2-4倍~~ | - | - | - | ❌ **破坏顺序** | ❌ 否 |
| **5. 连接管理** | ✅ 保持单连接 | ⭐ 无需修改 | - | - | - | - | ✅ 无影响 | ✅ **官方推荐** |
| **6. 消息压缩** | 🟢 低 | ⭐⭐ 中等 | **30-50%** | - | - | - | ✅ 无影响 | ✅ 是 |
| **7. 全局Worker池** | 🔴 **高** | ⭐⭐ 中等 | - | - | **50-70%** | **80-90%** | ✅ 无影响 | ✅ **是（Kafka经验）** |
| **8. 配置优化** | 🔴 高 | ⭐ 简单 | **20-30%** | - | - | - | ✅ 无影响 | ✅ 是 |

**重要说明**：
- ❌ **优化 4（并发拉取）不推荐**：会破坏同一聚合 ID 的消息顺序
- ❌ **优化 5（每 Topic 一个连接）不推荐**：不符合 NATS 官方最佳实践
- ✅ **优化 5（保持单连接）推荐**：符合 NATS 官方推荐，每个 EventBus 实例一个连接
- ✅ **优化 7（全局 Worker 池）强烈推荐**：与 Kafka 保持一致，大幅降低 Goroutine 数量
- ✅ **优化 2（增大批量大小）替代优化 4**：单 Fetcher + 大批量 + KeyedWorkerPool 并发处理

**综合预期收益**（优化 1、2、3、7、8，保持单连接）：
- ✅ **吞吐量提升 8-15倍**：从 242.92 msg/s 提升到 1900-3600 msg/s
- ✅ **延迟降低 60-80%**：从 18.82ms 降低到 4-8ms
- ✅ **内存使用降低 50-70%**：从 9.47MB 降低到 3-5MB（优化 7）
- ✅ **Goroutine 数量降低 80-90%**：从 1025 降低到 100-200（优化 7）
- ✅ **与 Kafka 性能相当**：Goroutine 数量接近 Kafka（83 个）
- ✅ **顺序性保证**：同一聚合 ID 的消息严格按顺序处理
- ✅ **符合 NATS 官方最佳实践**：每个 EventBus 实例一个连接

---

## 🎯 实施建议（更新：符合 NATS 官方最佳实践 + 全局 Worker 池）

### 第一阶段（快速见效，符合官方推荐）✅
1. ✅ **优化 2**：增大批量拉取大小（10 → 500-1000）
2. ✅ **优化 3**：缩短 MaxWait 时间（1s → 100ms）
3. ✅ **优化 8**：配置优化（MaxAckPending: 500→10000, MaxWaiting: 200→1000）
4. ✅ **优化 5**：确认单连接（无需修改，当前实现已符合官方推荐）

**预期收益**：
- 吞吐量提升 **5-8倍**（从 242.92 → 1200-1900 msg/s）
- 延迟降低 **50-70%**（从 18.82ms → 6-9ms）
- ✅ **顺序性保证**：单 Fetcher + KeyedWorkerPool
- ✅ **符合 NATS 官方最佳实践**：每个 EventBus 实例一个连接

**实施时间**：1-2 小时

### 第二阶段（核心优化，符合官方推荐）✅
5. ✅ **优化 1**：异步发布（PublishAsync）+ 统一 ACK 处理器
   - 使用 `PublishAsync` 替代同步 `Publish`
   - 使用 NATS 内置的 `PublishAsyncErrHandler` 统一处理错误 ACK
   - 配置 `PublishAsyncMaxPending(10000)` 限制未确认消息数量
   - 在 Close 时使用 `PublishAsyncComplete()` 等待所有消息发送完成
   - **与 Kafka 保持一致**：使用统一 ACK 处理器，固定 Goroutine 数量
6. ✅ **优化 7**：全局 Worker 池（与 Kafka 保持一致）
7. ❌ ~~**优化 4**：并发拉取消息~~（会破坏顺序性，不实施）
8. ❌ ~~**优化 5**：每 Topic 一个连接~~（不符合官方推荐，不实施）

**预期收益**：
- 吞吐量再提升 **3-5倍**（从 1200-1900 → 3600-9500 msg/s）
- 延迟降低 **50-70%**（从 6-9ms → 2-3ms）
- **Goroutine 数量降低 80-90%**（从 1025 → 100-200）
- **内存使用降低 50-70%**（从 9.47MB → 3-5MB）
- ✅ **顺序性保证**：异步发布和全局 Worker 池都不影响消费顺序
- ✅ **符合 NATS 官方最佳实践**：单连接 + 异步发布 + 统一 ACK 处理器
- ✅ **与 Kafka 保持一致**：使用相同的全局 Worker 池设计和统一 ACK 处理器理念

**实施时间**：2-4 小时

### 第三阶段（进阶优化，可选）🟡
9. ✅ **优化 6**：消息压缩（如果网络是瓶颈）

**预期收益**：
- 吞吐量再提升 **30-50%**（如果网络是瓶颈）
- ✅ **顺序性保证**：所有优化都不影响顺序
- ✅ **符合 NATS 官方最佳实践**

**实施时间**：2-4 小时

---

### 🎯 **推荐实施路径**

#### 路径 1：保守路径（推荐）
1. **第一阶段**：优化 2、3、8（快速见效，低风险）
2. **测试验证**：运行压力测试，验证吞吐量提升 5-8倍
3. **第二阶段**：优化 1（异步发布）
4. **测试验证**：验证吞吐量提升到 3600-9500 msg/s
5. **评估是否需要第三阶段**

#### 路径 2：激进路径
1. **一次性实施**：优化 1、2、3、8
2. **测试验证**：验证吞吐量提升 8-15倍
3. **根据需要实施第三阶段**

---

### ⚠️ **关键注意事项**

1. **连接管理（NATS 官方最佳实践）**：
   - ✅ **保持单连接**：每个 EventBus 实例一个连接（Singleton）
   - ✅ **符合官方推荐**：NATS 维护者明确建议"one connection per service"
   - ❌ **不使用每 Topic 一个连接**：不符合官方推荐，浪费资源
   - ❌ **不使用连接池（轮询）**：会破坏发布顺序

2. **统一 ACK 处理器（与 Kafka 保持一致）**：
   - ✅ **使用 NATS 内置机制**：`PublishAsyncErrHandler` 统一处理错误 ACK
   - ✅ **固定 Goroutine 数量**：NATS 内部使用固定数量的 Goroutine 处理 ACK
   - ❌ **不使用每条消息一个 Goroutine**：避免 Goroutine 数量爆炸
   - ✅ **限制未确认消息数量**：`PublishAsyncMaxPending(10000)` 防止内存溢出
   - ✅ **优雅关闭**：使用 `PublishAsyncComplete()` 等待所有消息发送完成
   - ✅ **与 Kafka 保持一致**：Kafka 使用 2 个 Goroutine 统一处理 ACK，NATS 使用内置机制

3. **全局 Worker 池（与 Kafka 保持一致）**：
   - ✅ **复用 Kafka 的 GlobalWorkerPool**：使用相同的代码和设计
   - ✅ **有聚合 ID**：使用 Keyed-Worker 池（保证顺序）
   - ✅ **无聚合 ID**：使用全局 Worker 池（节省 Goroutine）
   - ✅ **Worker 数量**：默认 CPU 核心数 × 2
   - ✅ **队列大小**：默认 Worker 数量 × 100

4. **顺序性保证**：
   - ✅ **保持单 Fetcher**：每个 topic 只有一个 goroutine 拉取消息
   - ✅ **依赖 KeyedWorkerPool**：保证同一聚合 ID 的消息顺序处理
   - ✅ **全局 Worker 池不影响顺序**：只处理无聚合 ID 的消息（无顺序要求）
   - ✅ **异步发布不影响顺序**：单连接保证发布顺序，JetStream 按接收顺序存储
   - ❌ **不使用多 Fetcher**：避免破坏拉取顺序

5. **批量大小调优**：
   - 起始值：500
   - 根据 AckWait 时间调整（默认 30 秒）
   - 公式：`批量大小 ≤ (AckWait / 单条消息处理时间)`
   - 示例：如果单条消息处理 10ms，30 秒可以处理 3000 条

6. **异步发布限流**：
   - 需要限制未确认消息的数量（避免内存溢出）
   - 建议：`PublishAsyncMaxPending(10000)`
   - NATS 内部会自动处理背压（backpressure）

7. **测试验证**：
   - 使用相同的压力测试场景（低压、中压、高压、极限）
   - 验证顺序性：检查同一聚合 ID 的消息是否按顺序处理
   - 验证性能：吞吐量、延迟、内存、Goroutine 数量
   - **重点验证 Goroutine 数量**：应该从 1025 降低到 100-200（与 Kafka 相当）
   - **验证 ACK 处理**：确认错误 ACK 被正确处理和记录

---

## 📚 参考资料

1. **NATS 官方文档**
   - [Connecting to NATS](https://docs.nats.io/using-nats/developer/connecting)
   - [JetStream Model Deep Dive](https://docs.nats.io/using-nats/developer/develop_jetstream/model_deep_dive)
   - [JetStream Concepts](https://docs.nats.io/nats-concepts/jetstream)

2. **NATS 官方最佳实践**
   - [GitHub Discussion #654: Best way to handle connection object in NATS?](https://github.com/nats-io/nats.net/discussions/654)
   - NATS 维护者 @mtmk 的官方回答：
     > "**one connection per service is the recommended approach**. you can have as many publishers as you want on the same connection. NATS protocol is asynchronous and can handle multiple publishers without stalling."

3. **社区最佳实践**
   - [Grokking NATS Consumers: Pull-based](https://www.byronruth.com/grokking-nats-consumers-part-3/)
   - [NATS Server Issue #4799](https://github.com/nats-io/nats-server/issues/4799)

4. **Kafka 优化经验**
   - Confluent 官方文档
   - LinkedIn、Uber、Airbnb 的 Kafka 优化实践

---

## ✅ 下一步

1. **评审方案**：与团队讨论优化方案的可行性和优先级
2. **确认连接管理**：确认当前单连接实现符合 NATS 官方最佳实践
3. **实施第一阶段**：快速见效的优化（优化 2、3、8）
4. **实施第二阶段**：核心优化（优化 1、7 - 全局 Worker 池）
5. **性能测试**：使用相同的压力测试场景验证优化效果
6. **验证 Goroutine 数量**：确认从 1025 降低到 100-200（与 Kafka 相当）
7. **迭代优化**：根据测试结果调整参数和实施后续优化

---

**创建时间**：2025-10-12
**更新时间**：2025-10-12
**作者**：Augment Agent
**版本**：v3.0（基于 NATS 官方最佳实践 + 全局 Worker 池优化）

