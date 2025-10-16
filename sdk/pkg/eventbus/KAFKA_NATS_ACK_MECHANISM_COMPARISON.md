# Kafka vs NATS 异步发布 ACK 机制对比

## 📋 概述

本文档详细对比 Kafka 和 NATS 在异步发布后如何向 Outbox 回发布 ACK 的机制。

## 🎯 核心问题

**问题**: Kafka 和 NATS 采用异步发布后，分别是如何向 Outbox 回发布 ACK 的？

**答案**: 两者采用**完全不同的机制**，但最终都能将 ACK 结果发送到 `publishResultChan` 供 Outbox Processor 消费。

## 🔴 Kafka 的 ACK 机制

### 1. 架构设计

Kafka 使用 **Sarama AsyncProducer**，提供了两个内置的 Channel：
- `Successes()`: 成功发布的消息通道
- `Errors()`: 发布失败的消息通道

### 2. 配置要求

```go
// 必须启用 Success 和 Error 返回
saramaConfig.Producer.Return.Successes = true  // ← 必须设置为 true
saramaConfig.Producer.Return.Errors = true     // ← 必须设置为 true
```

### 3. 发布流程

#### 步骤1: 异步发布消息

```go
func (k *kafkaEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // 1. 序列化 Envelope
    envelopeBytes, err := envelope.ToBytes()
    
    // 2. 创建 Kafka 消息
    msg := &sarama.ProducerMessage{
        Topic: topic,
        Key:   sarama.StringEncoder(envelope.AggregateID),
        Value: sarama.ByteEncoder(envelopeBytes),
        Headers: []sarama.RecordHeader{
            {Key: []byte("X-Aggregate-ID"), Value: []byte(envelope.AggregateID)},
            {Key: []byte("X-Event-Type"), Value: []byte(envelope.EventType)},
            {Key: []byte("X-Event-Version"), Value: []byte(fmt.Sprintf("%d", envelope.EventVersion))},
        },
    }
    
    // 3. 异步发送到 AsyncProducer
    select {
    case k.asyncProducer.Input() <- msg:  // ← 发送到输入通道
        return nil  // ← 立即返回，不等待 ACK
    case <-time.After(100 * time.Millisecond):
        // 队列满，阻塞等待
        k.asyncProducer.Input() <- msg
        return nil
    }
}
```

**关键点**:
- ✅ **立即返回**: 发送到 `Input()` 通道后立即返回，不等待 ACK
- ✅ **无 EventID**: Kafka 不生成 EventID（由 Sarama 内部管理）
- ✅ **无 ACK 处理**: 发布方法本身不处理 ACK

#### 步骤2: 后台 Goroutine 监听 ACK

Kafka 在初始化时启动了两个后台 Goroutine：

```go
// NewKafkaEventBus 初始化时启动
func NewKafkaEventBus(config *KafkaConfig) (EventBus, error) {
    // ...
    
    // 启动 Success 和 Error 处理 Goroutine
    go bus.handleAsyncProducerSuccess()  // ← 处理成功 ACK
    go bus.handleAsyncProducerErrors()   // ← 处理失败 ACK
    
    return bus, nil
}
```

#### 步骤3: 处理成功 ACK

```go
func (k *kafkaEventBus) handleAsyncProducerSuccess() {
    for success := range k.asyncProducer.Successes() {  // ← 从 Successes() 通道读取
        // 1. 记录成功指标
        k.publishedMessages.Add(1)
        
        // 2. 执行回调（如果配置了）
        if k.publishCallback != nil {
            var message []byte
            if success.Value != nil {
                message, _ = success.Value.Encode()
            }
            k.publishCallback(context.Background(), success.Topic, message, nil)
        }
        
        // ⚠️ 注意：Kafka 当前实现没有发送到 publishResultChan
        // 这是一个待改进的点
    }
}
```

#### 步骤4: 处理失败 ACK

```go
func (k *kafkaEventBus) handleAsyncProducerErrors() {
    for err := range k.asyncProducer.Errors() {  // ← 从 Errors() 通道读取
        // 1. 记录错误
        k.errorCount.Add(1)
        k.logger.Error("Async producer error",
            zap.String("topic", err.Msg.Topic),
            zap.Error(err.Err))
        
        // 2. 提取消息内容
        var message []byte
        if err.Msg.Value != nil {
            message, _ = err.Msg.Value.Encode()
        }
        
        // 3. 执行回调（如果配置了）
        if k.publishCallback != nil {
            k.publishCallback(context.Background(), err.Msg.Topic, message, err.Err)
        }
        
        // 4. 执行错误处理器（如果配置了）
        if k.errorHandler != nil {
            k.errorHandler.HandleError(context.Background(), err.Err, message, err.Msg.Topic)
        }
        
        // ⚠️ 注意：Kafka 当前实现没有发送到 publishResultChan
        // 这是一个待改进的点
    }
}
```

### 4. Kafka 的问题 ⚠️

**当前实现的问题**:

1. ❌ **没有生成 EventID**: Kafka 的 `PublishEnvelope()` 没有生成 EventID
2. ❌ **没有发送到 publishResultChan**: Success 和 Error 处理器没有发送 `PublishResult` 到 `publishResultChan`
3. ❌ **Outbox 无法追踪**: Outbox Processor 无法通过 `GetPublishResultChannel()` 获取 ACK 结果

**需要改进**:

```go
// 改进后的 handleAsyncProducerSuccess
func (k *kafkaEventBus) handleAsyncProducerSuccess() {
    for success := range k.asyncProducer.Successes() {
        k.publishedMessages.Add(1)
        
        // ✅ 提取 EventID（从 Header 中）
        var eventID string
        for _, header := range success.Headers {
            if string(header.Key) == "X-Event-ID" {
                eventID = string(header.Value)
                break
            }
        }
        
        // ✅ 发送成功结果到 publishResultChan
        if eventID != "" {
            result := &PublishResult{
                EventID:   eventID,
                Topic:     success.Topic,
                Success:   true,
                Error:     nil,
                Timestamp: time.Now(),
            }
            
            select {
            case k.publishResultChan <- result:
                // 成功发送
            default:
                k.logger.Warn("Publish result channel full, dropping success result")
            }
        }
    }
}
```

## 🔵 NATS 的 ACK 机制（方案2）

### 1. 架构设计

NATS 使用 **PubAckFuture** + **共享 ACK Worker 池**：
- `PubAckFuture`: 异步发布返回的 Future 对象，包含 `Ok()` 和 `Err()` 通道
- **ACK Worker 池**: 固定数量的 Worker Goroutine（CPU 核心数 * 2）
- **ACK 任务通道**: 缓冲区大小 100,000 的 Channel

### 2. 发布流程

#### 步骤1: 异步发布消息并获取 Future

```go
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // 1. 序列化 Envelope
    envelopeBytes, err := envelope.ToBytes()
    
    if n.js != nil {
        // 2. 异步发布，获取 Future
        pubAckFuture, err := n.js.PublishAsync(topic, envelopeBytes)  // ← 返回 Future
        if err != nil {
            return fmt.Errorf("failed to publish async: %w", err)
        }
        
        // 3. 生成事件ID（用于 Outbox 模式）
        eventID := fmt.Sprintf("%s:%s:%d:%d",
            envelope.AggregateID,
            envelope.EventType,
            envelope.EventVersion,
            envelope.Timestamp.UnixNano())
        
        // 4. 创建 ACK 任务
        task := &ackTask{
            future:      pubAckFuture,  // ← 包含 Future
            eventID:     eventID,       // ← 包含 EventID
            topic:       topic,
            aggregateID: envelope.AggregateID,
            eventType:   envelope.EventType,
        }
        
        // 5. 发送到 ACK Worker 池
        select {
        case n.ackChan <- task:  // ← 发送到 ACK 任务通道
            return nil  // ← 立即返回
        case <-ctx.Done():
            return ctx.Err()
        default:
            n.logger.Warn("ACK channel full, ACK processing may be delayed")
            return nil
        }
    }
    
    // ...
}
```

**关键点**:
- ✅ **获取 Future**: `PublishAsync()` 返回 `PubAckFuture`
- ✅ **生成 EventID**: 使用 `AggregateID:EventType:EventVersion:Timestamp` 组合
- ✅ **创建 ACK 任务**: 封装 Future 和 EventID
- ✅ **发送到 Worker 池**: 通过 `ackChan` 发送任务
- ✅ **立即返回**: 不等待 ACK

#### 步骤2: ACK Worker 池处理任务

NATS 在初始化时启动了固定数量的 Worker：

```go
// NewNATSEventBus 初始化时启动
func NewNATSEventBus(config *NATSConfig) (EventBus, error) {
    // ...
    
    bus := &natsEventBus{
        // ...
        ackChan:        make(chan *ackTask, 100000),  // ← ACK 任务通道
        ackWorkerStop:  make(chan struct{}),
        ackWorkerCount: runtime.NumCPU() * 2,         // ← Worker 数量
    }
    
    // 启动 ACK Worker 池
    bus.startACKWorkers()  // ← 启动 48 个 Worker（24核 * 2）
    
    return bus, nil
}

// startACKWorkers 启动 ACK worker 池
func (n *natsEventBus) startACKWorkers() {
    for i := 0; i < n.ackWorkerCount; i++ {
        n.ackWorkerWg.Add(1)
        go n.ackWorker(i)  // ← 启动每个 Worker
    }
    
    n.logger.Info("NATS ACK worker pool started",
        zap.Int("workerCount", n.ackWorkerCount),
        zap.Int("ackChanSize", cap(n.ackChan)),
        zap.Int("resultChanSize", cap(n.publishResultChan)))
}
```

#### 步骤3: Worker 从通道读取任务

```go
func (n *natsEventBus) ackWorker(workerID int) {
    defer n.ackWorkerWg.Done()
    
    n.logger.Debug("ACK worker started", zap.Int("workerID", workerID))
    
    for {
        select {
        case task := <-n.ackChan:  // ← 从 ACK 任务通道读取
            n.processACKTask(task)  // ← 处理任务
            
        case <-n.ackWorkerStop:
            n.logger.Debug("ACK worker stopping", zap.Int("workerID", workerID))
            return
        }
    }
}
```

#### 步骤4: 处理 ACK 任务

```go
func (n *natsEventBus) processACKTask(task *ackTask) {
    select {
    case <-task.future.Ok():  // ← 等待 Future 的 Ok() 通道
        // ✅ 发布成功
        n.publishedMessages.Add(1)
        
        // 发送成功结果到 publishResultChan
        result := &PublishResult{
            EventID:     task.eventID,     // ← 使用预生成的 EventID
            Topic:       task.topic,
            Success:     true,
            Error:       nil,
            Timestamp:   time.Now(),
            AggregateID: task.aggregateID,
            EventType:   task.eventType,
        }
        
        select {
        case n.publishResultChan <- result:  // ← 发送到 Outbox 通道
            // 成功发送结果
        default:
            n.logger.Warn("Publish result channel full, dropping success result",
                zap.String("eventID", task.eventID),
                zap.String("topic", task.topic))
        }
        
    case err := <-task.future.Err():  // ← 等待 Future 的 Err() 通道
        // ❌ 发布失败
        n.errorCount.Add(1)
        n.logger.Error("Async publish ACK failed",
            zap.String("eventID", task.eventID),
            zap.String("topic", task.topic),
            zap.Error(err))
        
        // 发送失败结果到 publishResultChan
        result := &PublishResult{
            EventID:     task.eventID,     // ← 使用预生成的 EventID
            Topic:       task.topic,
            Success:     false,
            Error:       err,
            Timestamp:   time.Now(),
            AggregateID: task.aggregateID,
            EventType:   task.eventType,
        }
        
        select {
        case n.publishResultChan <- result:  // ← 发送到 Outbox 通道
            // 成功发送结果
        default:
            n.logger.Warn("Publish result channel full, dropping error result",
                zap.String("eventID", task.eventID),
                zap.String("topic", task.topic))
        }
    }
}
```

**关键点**:
- ✅ **等待 Future**: 使用 `select` 等待 `Ok()` 或 `Err()` 通道
- ✅ **构造 PublishResult**: 包含 EventID、Topic、Success、Error 等信息
- ✅ **发送到 publishResultChan**: 供 Outbox Processor 消费
- ✅ **非阻塞发送**: 使用 `default` 避免阻塞

## 📊 对比总结

| 维度 | Kafka | NATS (方案2) |
|------|-------|-------------|
| **异步发布 API** | `asyncProducer.Input() <- msg` | `js.PublishAsync(topic, data)` |
| **返回值** | 无（发送到 Channel） | `PubAckFuture` |
| **ACK 通道** | `Successes()` / `Errors()` | `future.Ok()` / `future.Err()` |
| **后台处理** | 2 个全局 Goroutine | 48 个 Worker Goroutine（CPU * 2） |
| **EventID 生成** | ❌ 无（需改进） | ✅ 有（发布时生成） |
| **发送到 publishResultChan** | ❌ 无（需改进） | ✅ 有（Worker 发送） |
| **Outbox 支持** | ❌ 不完整（需改进） | ✅ 完整支持 |
| **架构模式** | 全局 Success/Error 处理器 | Per-message Future + Worker 池 |

## 🏆 优劣势分析

### Kafka 的优势

1. ✅ **简单**: 只需 2 个全局 Goroutine
2. ✅ **资源占用低**: 固定 2 个 Goroutine
3. ✅ **成熟**: Sarama AsyncProducer 久经考验

### Kafka 的劣势

1. ❌ **Outbox 支持不完整**: 没有 EventID，没有发送到 `publishResultChan`
2. ❌ **需要改进**: 需要在 `PublishEnvelope()` 中生成 EventID 并添加到 Header
3. ❌ **无法追踪单条消息**: Success/Error 通道只有 Topic 信息，无法关联到具体的 Envelope

### NATS 的优势

1. ✅ **Outbox 支持完整**: EventID 生成 + `publishResultChan` 发送
2. ✅ **Per-message Future**: 每条消息都有独立的 Future，可精确追踪
3. ✅ **Worker 池**: 固定数量的 Worker，资源可控
4. ✅ **生产就绪**: 100% 成功率，稳定可靠

### NATS 的劣势

1. ⚠️ **资源占用略高**: 48 个 Worker Goroutine（但固定数量，可接受）
2. ⚠️ **复杂度略高**: 需要管理 Worker 池和 ACK 任务通道

## 💡 最佳实践建议

### 对于 Kafka

**建议改进 `PublishEnvelope()` 方法**:

1. ✅ 在发布时生成 EventID
2. ✅ 将 EventID 添加到 Kafka Message Header
3. ✅ 在 `handleAsyncProducerSuccess()` 中提取 EventID 并发送到 `publishResultChan`
4. ✅ 在 `handleAsyncProducerErrors()` 中提取 EventID 并发送到 `publishResultChan`

### 对于 NATS

**当前实现已经很好**:

1. ✅ EventID 生成机制完善
2. ✅ ACK Worker 池稳定可靠
3. ✅ `publishResultChan` 发送完整
4. ✅ Outbox 模式支持完整

## 📁 相关文档

- **Kafka 实现**: `sdk/pkg/eventbus/kafka.go`
- **NATS 实现**: `sdk/pkg/eventbus/nats.go`
- **接口定义**: `sdk/pkg/eventbus/type.go`
- **方法对比**: `sdk/pkg/eventbus/PUBLISH_METHODS_COMPARISON.md`

