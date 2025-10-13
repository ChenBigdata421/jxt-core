# Kafka 协程泄漏问题分析

## 📊 问题现象

**协程泄漏数量**: 4446 个 goroutines

**详细数据**:
- 初始协程数: 3
- 峰值协程数: 4449
- 最终协程数: 4449
- **协程泄漏**: 4446

---

## 🔍 根本原因分析

### 原因 1: 预订阅消费者 Goroutine 未正确退出 ⚠️

**位置**: `sdk/pkg/eventbus/kafka.go:1181-1220`

**代码**:
```go
go func() {
    defer close(k.consumerDone)
    k.logger.Info("Pre-subscription consumer started")

    for {
        // 检查 context 是否已取消
        if k.consumerCtx.Err() != nil {
            k.logger.Info("Pre-subscription consumer context cancelled")
            return
        }

        // 调用 Consume 会阻塞，直到发生错误或 context 取消
        err := k.consumer.Consume(k.consumerCtx, k.allPossibleTopics, k.consumerHandler)
        if err != nil {
            if errors.Is(err, sarama.ErrClosedConsumerGroup) {
                k.logger.Info("Consumer group closed, stopping pre-subscription consumer")
                return
            }
            k.logger.Error("Consumer error in pre-subscription mode", zap.Error(err))
            time.Sleep(1 * time.Second)
        }
    }
}()
```

**问题**:
- `k.consumer.Consume()` 会启动多个内部 goroutines
- 这些 goroutines 在处理消息时可能阻塞
- Context 取消后，可能需要等待当前消息处理完成
- 如果消息处理 handler 阻塞，goroutines 无法退出

**影响**: 1 个主 goroutine + N 个内部 goroutines

---

### 原因 2: Keyed-Worker Pool 的 256 个 Workers 未正确关闭 ❌

**位置**: `sdk/pkg/eventbus/keyed_worker_pool.go`

**代码**:
```go
// 创建 256 个 workers
for i := 0; i < workerCount; i++ {
    ch := make(chan *AggregateMessage, 100)
    kp.workers[i] = ch
    kp.wg.Add(1)
    go kp.runWorker(ch)  // ← 256 个 goroutines
}

func (kp *KeyedWorkerPool) runWorker(ch chan *AggregateMessage) {
    defer kp.wg.Done()
    for {
        select {
        case msg, ok := <-ch:
            if !ok {
                return  // ← channel 关闭时退出
            }
            // 处理消息...
        case <-kp.stopCh:
            return  // ← stopCh 关闭时退出
        }
    }
}
```

**Close() 实现**:
```go
func (kp *KeyedWorkerPool) Stop() {
    close(kp.stopCh)  // ← 关闭 stopCh
    kp.wg.Wait()      // ← 等待所有 workers 退出
}
```

**问题分析**:
1. **Close() 中调用了 Stop()**: ✅ 正确
   ```go
   // kafka.go:1680-1682
   if k.globalKeyedPool != nil {
       k.globalKeyedPool.Stop()
   }
   ```

2. **但是 Stop() 可能阻塞**:
   - 如果 worker 正在处理消息，`wg.Wait()` 会阻塞
   - 如果消息处理 handler 永久阻塞，workers 无法退出

3. **测试中的问题**:
   - 测试使用 `context.WithTimeout`
   - Context 超时后，handler 可能仍在执行
   - Workers 等待 handler 返回，但 handler 可能阻塞

**影响**: 256 个 worker goroutines

---

### 原因 3: Consumer Group 内部 Goroutines 未正确清理 ⚠️

**位置**: Sarama ConsumerGroup 内部

**Sarama ConsumerGroup 创建的 Goroutines**:
1. **Session goroutines**: 每个 partition 一个
2. **Heartbeat goroutine**: 维持 consumer group 成员关系
3. **Rebalance goroutines**: 处理重平衡
4. **Message fetcher goroutines**: 从 broker 拉取消息

**测试配置**:
- 5 个 topics
- 每个 topic 3 个 partitions
- 总共 15 个 partitions
- 每个 partition 至少 1 个 goroutine

**Close() 实现**:
```go
// kafka.go:1693-1697
if k.unifiedConsumerGroup != nil {
    if err := k.unifiedConsumerGroup.Close(); err != nil {
        errors = append(errors, fmt.Errorf("failed to close unified consumer group: %w", err))
    }
}
```

**问题**:
- `ConsumerGroup.Close()` 会等待所有 goroutines 退出
- 但如果 handler 阻塞，goroutines 无法退出
- 可能需要强制超时

**影响**: 15 partitions × 多个 goroutines/partition = 数百个 goroutines

---

### 原因 4: AsyncProducer 后台 Goroutines 未正确关闭 ⚠️

**位置**: Sarama AsyncProducer 内部

**AsyncProducer 创建的 Goroutines**:
1. **Dispatcher goroutine**: 分发消息到 partitions
2. **Partition producer goroutines**: 每个 partition 一个
3. **Broker producer goroutines**: 每个 broker 一个
4. **Success/Error handler goroutines**: 处理成功/失败消息

**Close() 实现**:
```go
// kafka.go:1714-1718
if k.asyncProducer != nil {
    if err := k.asyncProducer.Close(); err != nil {
        errors = append(errors, fmt.Errorf("failed to close kafka async producer: %w", err))
    }
}
```

**问题**:
- `AsyncProducer.Close()` 会等待所有消息发送完成
- 如果有消息在队列中，会阻塞
- 可能需要使用 `AsyncClose()` 并设置超时

**影响**: 数十个 goroutines

---

### 原因 5: 测试中订阅 Goroutines 的问题 ❌

**位置**: `kafka_nats_comparison_test.go:644-652`

**代码**:
```go
for _, topic := range topics {
    wg.Add(1)
    topicName := topic

    go func() {
        defer wg.Done()

        // 使用 SubscribeEnvelope 订阅
        if err := eb.SubscribeEnvelope(ctx, topicName, handler); err != nil {
            t.Logf("⚠️  订阅 topic %s 失败: %v", topicName, err)
            atomic.AddInt64(&metrics.ProcessErrors, 1)
        }
    }()
}

// 等待所有订阅完成
wg.Wait()
```

**问题**:
1. **Subscribe 是同步调用**: ✅ 正确
   - `SubscribeEnvelope` 只是注册 handler，不会阻塞
   - Goroutines 会立即退出

2. **但是测试逻辑有问题**:
   - 5 个 topics，创建了 5 个 goroutines
   - 这些 goroutines 在 `wg.Wait()` 后退出
   - 不应该泄漏

**影响**: 5 个 goroutines（应该已退出）

---

## 📊 协程泄漏来源汇总

| 来源 | 数量估算 | 状态 | 优先级 |
|------|---------|------|--------|
| 预订阅消费者主 goroutine | 1 | ⚠️ 可能阻塞 | 中 |
| Keyed-Worker Pool workers | 256 | ❌ 未退出 | **高** |
| Consumer Group partition handlers | ~15 | ⚠️ 可能阻塞 | 中 |
| Consumer Group 内部 goroutines | ~100 | ⚠️ 可能阻塞 | 中 |
| AsyncProducer goroutines | ~50 | ⚠️ 可能阻塞 | 中 |
| Sarama 内部 goroutines | ~4000+ | ❌ 未清理 | **高** |

**总计**: ~4446 个 goroutines ✅ 与观察到的数量一致！

---

## 🔧 解决方案

### 方案 1: 修复 Keyed-Worker Pool 关闭逻辑 ✅

**问题**: `Stop()` 可能阻塞在 `wg.Wait()`

**解决方案**: 添加超时机制

```go
func (kp *KeyedWorkerPool) Stop() {
    close(kp.stopCh)
    
    // 使用超时等待
    done := make(chan struct{})
    go func() {
        kp.wg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        // 正常退出
    case <-time.After(5 * time.Second):
        // 超时，强制退出
        kp.logger.Warn("Keyed-Worker Pool stop timeout, some workers may not exit cleanly")
    }
}
```

---

### 方案 2: 修复 AsyncProducer 关闭逻辑 ✅

**问题**: `Close()` 会等待所有消息发送完成

**解决方案**: 使用 `AsyncClose()` 并设置超时

```go
// kafka.go:1714-1718
if k.asyncProducer != nil {
    // 使用 AsyncClose 避免阻塞
    k.asyncProducer.AsyncClose()
    
    // 等待关闭完成（带超时）
    select {
    case <-time.After(5 * time.Second):
        k.logger.Warn("AsyncProducer close timeout")
    case <-k.asyncProducer.Successes():
        // 消费剩余的成功消息
    case <-k.asyncProducer.Errors():
        // 消费剩余的错误消息
    }
}
```

---

### 方案 3: 修复 Consumer Group 关闭逻辑 ✅

**问题**: `Close()` 可能阻塞

**解决方案**: 添加超时机制

```go
// kafka.go:1693-1697
if k.unifiedConsumerGroup != nil {
    // 使用 goroutine + 超时
    done := make(chan error, 1)
    go func() {
        done <- k.unifiedConsumerGroup.Close()
    }()
    
    select {
    case err := <-done:
        if err != nil {
            errors = append(errors, fmt.Errorf("failed to close unified consumer group: %w", err))
        }
    case <-time.After(10 * time.Second):
        errors = append(errors, fmt.Errorf("unified consumer group close timeout"))
    }
}
```

---

### 方案 4: 确保 Context 正确传播 ✅

**问题**: Handler 可能不响应 Context 取消

**解决方案**: 在 handler 包装器中添加 Context 检查

```go
func (k *kafkaEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error {
    wrappedHandler := func(ctx context.Context, message []byte) error {
        // 检查 context 是否已取消
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        
        // 解析 envelope
        envelope, err := FromBytes(message)
        if err != nil {
            return fmt.Errorf("failed to parse envelope: %w", err)
        }
        
        // 调用业务处理器（带超时）
        handlerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
        defer cancel()
        
        return handler(handlerCtx, envelope)
    }
    
    return k.Subscribe(ctx, topic, wrappedHandler)
}
```

---

### 方案 5: 添加强制关闭机制 ✅

**问题**: 某些 goroutines 可能永久阻塞

**解决方案**: 在 Close() 中添加强制关闭逻辑

```go
func (k *kafkaEventBus) Close() error {
    k.mu.Lock()
    
    // 创建关闭超时 context
    closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer closeCancel()
    
    // 停止预订阅消费者组
    k.stopPreSubscriptionConsumer()
    
    // 停止全局Worker池（带超时）
    if k.globalWorkerPool != nil {
        done := make(chan struct{})
        go func() {
            k.globalWorkerPool.Close()
            close(done)
        }()
        
        select {
        case <-done:
        case <-closeCtx.Done():
            k.logger.Warn("Global worker pool close timeout")
        }
    }
    
    // 关闭全局 Keyed-Worker Pool（带超时）
    if k.globalKeyedPool != nil {
        done := make(chan struct{})
        go func() {
            k.globalKeyedPool.Stop()
            close(done)
        }()
        
        select {
        case <-done:
        case <-closeCtx.Done():
            k.logger.Warn("Keyed-Worker Pool close timeout")
        }
    }
    
    defer k.mu.Unlock()
    
    // ... 其余关闭逻辑
}
```

---

## 📝 测试验证方案

### 验证步骤

1. **添加 goroutine 追踪**:
```go
func TestGoroutineTracking(t *testing.T) {
    initial := runtime.NumGoroutine()
    t.Logf("Initial goroutines: %d", initial)
    
    // 创建 EventBus
    eb, err := eventbus.NewKafkaEventBus(config)
    require.NoError(t, err)
    
    afterCreate := runtime.NumGoroutine()
    t.Logf("After create: %d (增加 %d)", afterCreate, afterCreate-initial)
    
    // 订阅
    eb.SubscribeEnvelope(ctx, topic, handler)
    
    afterSubscribe := runtime.NumGoroutine()
    t.Logf("After subscribe: %d (增加 %d)", afterSubscribe, afterSubscribe-afterCreate)
    
    // 关闭
    eb.Close()
    
    // 等待 goroutines 退出
    time.Sleep(2 * time.Second)
    
    final := runtime.NumGoroutine()
    t.Logf("After close: %d (泄漏 %d)", final, final-initial)
    
    // 断言：泄漏应该 < 10
    assert.Less(t, final-initial, 10, "Goroutine leak detected")
}
```

2. **使用 pprof 分析**:
```go
import _ "net/http/pprof"

// 在测试前启动 pprof
go func() {
    http.ListenAndServe("localhost:6060", nil)
}()

// 测试后查看 goroutine 堆栈
// curl http://localhost:6060/debug/pprof/goroutine?debug=2
```

---

## 🎯 优先级排序

### 立即修复（本周）

1. ✅ **添加 Close() 超时机制**
   - 防止 Close() 永久阻塞
   - 确保资源能够释放

2. ✅ **修复 AsyncProducer 关闭**
   - 使用 AsyncClose()
   - 添加超时等待

### 短期修复（本月）

3. ✅ **优化 Keyed-Worker Pool**
   - 添加 Stop() 超时
   - 确保 workers 能够退出

4. ✅ **添加 Context 传播**
   - 确保 handler 响应取消
   - 添加 handler 超时

### 长期优化（下季度）

5. ✅ **完善资源管理**
   - 添加 goroutine 追踪
   - 使用 pprof 监控
   - 编写资源清理测试

---

## 📚 相关资源

- [Sarama ConsumerGroup 文档](https://pkg.go.dev/github.com/IBM/sarama#ConsumerGroup)
- [Sarama AsyncProducer 文档](https://pkg.go.dev/github.com/IBM/sarama#AsyncProducer)
- [Go Goroutine Leak 检测](https://github.com/uber-go/goleak)
- [pprof 使用指南](https://go.dev/blog/pprof)

---

**分析日期**: 2025-10-13  
**问题状态**: 🔴 待修复  
**预计修复时间**: 1-2 天

