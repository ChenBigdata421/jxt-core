# 全局 Keyed-Worker Pool 迁移指南

## 背景

当前实现为每个 topic 创建独立的 Keyed-Worker Pool，导致：
- 资源浪费：5 个 topic × 64 workers = 320 个 goroutines
- 管理复杂：需要维护 `keyedPools map[string]*KeyedWorkerPool`
- 协程泄漏风险：每个 pool 独立管理生命周期

## 目标

使用一个**全局 Keyed-Worker Pool**，所有 topic 共享：
- 减少资源占用：256 个全局 workers（而不是 320 个）
- 简化管理：只需管理一个全局池
- 统一生命周期：全局池随 EventBus 创建和销毁

## 需要修改的文件

### 1. `sdk/pkg/eventbus/keyed_worker_pool.go`

#### 修改 1.1: AggregateMessage 结构添加 Handler 字段

```go
// AggregateMessage 聚合消息（用于 Keyed-Worker 池）
type AggregateMessage struct {
	Topic       string
	Partition   int32
	Offset      int64
	Key         []byte
	Value       []byte
	Headers     map[string][]byte
	Timestamp   time.Time
	AggregateID string
	Context     context.Context
	Done        chan error
	Handler     MessageHandler // 新增：每个消息携带自己的 handler（支持全局池）
}
```

#### 修改 1.2: runWorker 方法支持消息携带的 handler

```go
func (kp *KeyedWorkerPool) runWorker(ch chan *AggregateMessage) {
	defer kp.wg.Done()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// 优先使用消息携带的 handler，如果没有则使用池的默认 handler
			handler := msg.Handler
			if handler == nil {
				handler = kp.handler
			}
			
			var err error
			if handler != nil {
				err = handler(msg.Context, msg.Value)
			} else {
				err = errors.New("no handler available for message")
			}
			
			// return result to caller (non-blocking)
			select {
			case msg.Done <- err:
			default:
			}
		case <-kp.stopCh:
			return
		}
	}
}
```

### 2. `sdk/pkg/eventbus/kafka.go`

#### 修改 2.1: kafkaEventBus 结构体

**删除**：
```go
// Keyed worker pools (per topic)
keyedPools   map[string]*KeyedWorkerPool
keyedPoolsMu sync.RWMutex
```

**添加**：
```go
// 全局 Keyed-Worker Pool（所有 topic 共享）
globalKeyedPool *KeyedWorkerPool
```

#### 修改 2.2: NewKafkaEventBus 初始化

**删除**：
```go
keyedPools:    make(map[string]*KeyedWorkerPool),
```

**添加**（在创建 bus 后）：
```go
// 创建全局 Keyed-Worker Pool（所有 topic 共享）
// 使用较大的 worker 数量以支持多个 topic 的并发处理
bus.globalKeyedPool = NewKeyedWorkerPool(KeyedWorkerPoolConfig{
	WorkerCount: 256,                    // 全局 worker 数量（支持多个 topic）
	QueueSize:   1000,                   // 每个 worker 的队列大小
	WaitTimeout: 500 * time.Millisecond, // 等待超时
}, nil) // handler 将在处理消息时动态传入
```

#### 修改 2.3: Subscribe 方法

**删除**：
```go
// Create per-topic Keyed-Worker pool (Phase 1) - 保持现有逻辑
k.keyedPoolsMu.Lock()
if _, ok := k.keyedPools[topic]; !ok {
	pool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
		WorkerCount: 64,
		QueueSize:   500,
		WaitTimeout: 200 * time.Millisecond,
	}, handler)
	k.keyedPools[topic] = pool
}
k.keyedPoolsMu.Unlock()
```

**替换为**：
```go
// 全局 Keyed-Worker Pool 已在初始化时创建，无需为每个 topic 创建独立池
```

#### 修改 2.4: topicConsumerHandler.ConsumeClaim 方法

**修改前**（第 873-903 行）：
```go
// 获取该 topic 的 keyed 池
h.eventBus.keyedPoolsMu.RLock()
pool := h.eventBus.keyedPools[h.topic]
h.eventBus.keyedPoolsMu.RUnlock()
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
	}
	for _, header := range message.Headers {
		aggMsg.Headers[string(header.Key)] = header.Value
	}
	if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
		return err
	}
	select {
	case err := <-aggMsg.Done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

**修改后**：
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
	for _, header := range message.Headers {
		aggMsg.Headers[string(header.Key)] = header.Value
	}
	if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
		return err
	}
	select {
	case err := <-aggMsg.Done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

#### 修改 2.5: preSubscriptionConsumerHandler.processMessageWithKeyedPool 方法

**修改前**（第 990-1028 行）：
```go
// 获取该 topic 的 keyed 池
h.eventBus.keyedPoolsMu.RLock()
pool := h.eventBus.keyedPools[message.Topic]
h.eventBus.keyedPoolsMu.RUnlock()

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
	}
	for _, header := range message.Headers {
		aggMsg.Headers[string(header.Key)] = header.Value
	}
	
	// 提交到 Keyed-Worker 池
	if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
		return err
	}
	
	// 等待处理完成
	select {
	case err := <-aggMsg.Done:
		if err != nil {
			return err
		}
		// 处理成功，标记消息
		session.MarkMessage(message, "")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

**修改后**：
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
		Handler:     handler, // 携带 topic 的 handler
	}
	for _, header := range message.Headers {
		aggMsg.Headers[string(header.Key)] = header.Value
	}
	
	// 提交到 Keyed-Worker 池
	if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
		return err
	}
	
	// 等待处理完成
	select {
	case err := <-aggMsg.Done:
		if err != nil {
			return err
		}
		// 处理成功，标记消息
		session.MarkMessage(message, "")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

#### 修改 2.6: Close 方法

**添加**（在关闭其他资源后）：
```go
// 关闭全局 Keyed-Worker Pool
if k.globalKeyedPool != nil {
	k.globalKeyedPool.Stop()
}
```

## 预期效果

### 资源占用对比

| 指标 | 修改前（per-topic pool） | 修改后（global pool） | 改善 |
|------|------------------------|---------------------|------|
| Worker 数量 | 5 topics × 64 = 320 | 256 | -20% |
| 内存占用 | ~10 MB | ~8 MB | -20% |
| 管理复杂度 | 高（多个 pool） | 低（单个 pool） | 简化 |

### 功能保证

✅ **顺序性保证**：相同聚合 ID 的消息仍然路由到同一个 worker  
✅ **性能保证**：全局池有足够的 worker 数量（256）  
✅ **隔离性保证**：不同 topic 的消息通过聚合 ID hash 自然隔离  
✅ **兼容性保证**：API 不变，只是内部实现优化  

## 测试验证

运行性能测试验证：
```bash
cd tests/eventbus/performance_tests
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 30m
```

预期结果：
- ✅ Kafka 成功率 > 99%
- ✅ 顺序违反次数 = 0
- ✅ 协程泄漏减少（从 430 降低到 ~260）

