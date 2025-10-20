# EventVersion 字段用途分析

## 📋 问题

**测试用例中使用 EventVersion 的目的是什么？是 keyed-worker 需要还是谁需要？**

---

## 🔍 分析结果

### 1. EventVersion 的定义

<augment_code_snippet path="sdk/pkg/eventbus/envelope.go" mode="EXCERPT">
````go
// Envelope 统一消息包络结构（方案A）
type Envelope struct {
    AggregateID   string     `json:"aggregate_id"`             // 聚合ID（必填）
    EventType     string     `json:"event_type"`               // 事件类型（必填）
    EventVersion  int64      `json:"event_version"`            // 事件版本（预留，为了将来可能实现事件溯源预留）
    Timestamp     time.Time  `json:"timestamp"`                // 时间戳
    TraceID       string     `json:"trace_id,omitempty"`       // 链路追踪ID（可选）
    CorrelationID string     `json:"correlation_id,omitempty"` // 关联ID（可选）
    Payload       RawMessage `json:"payload"`                  // 业务负载
}
````
</augment_code_snippet>

**关键注释**：`事件版本（预留，为了将来可能实现事件溯源预留）`

---

### 2. EventVersion 的用途

#### ✅ 用途 1: 业务层面 - 事件溯源（Event Sourcing）

**目的**：为将来实现事件溯源（Event Sourcing）预留

**事件溯源的核心概念**：
- 每个聚合根（Aggregate）的状态变化都通过一系列事件来表示
- 每个事件都有一个递增的版本号（EventVersion）
- 通过重放事件序列可以重建聚合根的当前状态

**示例**：
```
聚合ID: order-123
- EventVersion 1: OrderCreated
- EventVersion 2: ItemAdded
- EventVersion 3: ItemRemoved
- EventVersion 4: OrderConfirmed
- EventVersion 5: OrderShipped
```

**用途**：
- 乐观锁控制（Optimistic Concurrency Control）
- 事件重放（Event Replay）
- 状态重建（State Reconstruction）
- 冲突检测（Conflict Detection）

---

#### ✅ 用途 2: 技术层面 - 消息元数据

**Kafka 实现**：

<augment_code_snippet path="sdk/pkg/eventbus/kafka.go" mode="EXCERPT">
````go
Headers: []sarama.RecordHeader{
    {Key: []byte("X-Aggregate-ID"), Value: []byte(envelope.AggregateID)},
    {Key: []byte("X-Event-Version"), Value: []byte(fmt.Sprintf("%d", envelope.EventVersion))},
    {Key: []byte("X-Event-Type"), Value: []byte(envelope.EventType)},
},
````
</augment_code_snippet>

**NATS 实现**：

<augment_code_snippet path="sdk/pkg/eventbus/nats.go" mode="EXCERPT">
````go
Header: nats.Header{
    "X-Aggregate-ID":  []string{envelope.AggregateID},
    "X-Event-Version": []string{fmt.Sprintf("%d", envelope.EventVersion)},
    "X-Event-Type":    []string{envelope.EventType},
    "X-Timestamp":     []string{envelope.Timestamp.Format(time.RFC3339)},
},
````
</augment_code_snippet>

**用途**：
- 作为消息头（Header）传递元数据
- 便于消息追踪和调试
- 支持消息过滤和路由

---

#### ✅ 用途 3: 测试层面 - 顺序验证

**测试代码中的用途**：

<augment_code_snippet path="tests/eventbus/performance_tests/kafka_nats_comparison_test.go" mode="EXCERPT">
````go
// OrderChecker 检查顺序
func (oc *OrderChecker) Check(aggregateID string, version int64) bool {
    shard := oc.shards[shardIndex]
    shard.mu.Lock()
    defer shard.mu.Unlock()

    lastSeq, exists := shard.sequences[aggregateID]
    if exists && version <= lastSeq {
        atomic.AddInt64(&oc.violations, 1)
        return false // 顺序违反
    }

    shard.sequences[aggregateID] = version
    return true // 顺序正确
}
````
</augment_code_snippet>

**用途**：
- 验证同一聚合ID的事件是否按顺序处理
- 检测消息乱序问题
- 验证 Keyed-Worker Pool 的顺序保证

---

### 3. Keyed-Worker Pool 是否需要 EventVersion？

**答案：❌ 不需要！**

**Keyed-Worker Pool 的实现**：

<augment_code_snippet path="sdk/pkg/eventbus/keyed_worker_pool.go" mode="EXCERPT">
````go
func (kp *KeyedWorkerPool) hashToIndex(key string) int {
    h := fnv.New32a()
    _, _ = h.Write([]byte(key))
    return int(h.Sum32() % uint32(len(kp.workers)))
}

func (kp *KeyedWorkerPool) runWorker(ch chan *AggregateMessage) {
    defer kp.wg.Done()
    for {
        select {
        case msg, ok := <-ch:
            if !ok {
                return
            }
            // Process sequentially; guarantee per-key ordering
            handler := msg.Handler
            if handler == nil {
                handler = kp.handler
            }

            var err error
            if handler != nil {
                err = handler(msg.Context, msg.Value)
            }
            
            select {
            case msg.Done <- err:
            default:
            }
        case <-kp.stopCh:
            return
        }
    }
}
````
</augment_code_snippet>

**Keyed-Worker Pool 只需要 AggregateID**：
- 使用 `AggregateID` 计算 hash
- 根据 hash 路由到固定的 worker
- Worker 串行处理消息，保证顺序

**不需要 EventVersion 的原因**：
- Keyed-Worker Pool 依赖消息到达的顺序，而不是 EventVersion
- 只要消息按顺序到达同一个 worker，就能保证按顺序处理
- EventVersion 是业务层面的概念，Keyed-Worker Pool 是技术层面的实现

---

### 4. EventVersion 的校验

**Envelope 的校验逻辑**：

<augment_code_snippet path="sdk/pkg/eventbus/envelope.go" mode="EXCERPT">
````go
func (e *Envelope) Validate() error {
    if strings.TrimSpace(e.AggregateID) == "" {
        return errors.New("aggregate_id is required")
    }
    if strings.TrimSpace(e.EventType) == "" {
        return errors.New("event_type is required")
    }
    if e.EventVersion <= 0 {
        return errors.New("event_version must be positive")
    }
    if len(e.Payload) == 0 {
        return errors.New("payload is required")
    }

    // 校验 aggregateID 格式
    if err := validateAggregateID(e.AggregateID); err != nil {
        return fmt.Errorf("invalid aggregate_id: %w", err)
    }

    return nil
}
````
</augment_code_snippet>

**校验要求**：
- EventVersion 必须 > 0
- 这是为了确保事件版本的有效性

---

## 📊 总结

### EventVersion 的使用者

| 使用者 | 是否需要 | 用途 |
|--------|---------|------|
| **业务层（Event Sourcing）** | ✅ 需要 | 事件溯源、乐观锁、状态重建 |
| **EventBus（Kafka/NATS）** | ✅ 需要 | 消息元数据、追踪、调试 |
| **测试代码（OrderChecker）** | ✅ 需要 | 顺序验证、乱序检测 |
| **Keyed-Worker Pool** | ❌ 不需要 | 只需要 AggregateID 进行路由 |

---

### EventVersion 的设计意图

1. **主要目的**：为将来实现事件溯源（Event Sourcing）预留
2. **次要目的**：作为消息元数据，便于追踪和调试
3. **测试目的**：验证消息顺序的正确性

---

### 测试代码中使用 EventVersion 的原因

**测试代码使用 EventVersion 的目的**：
- ✅ 验证同一聚合ID的事件是否按顺序处理
- ✅ 检测 Keyed-Worker Pool 是否正确工作
- ✅ 确保消息不会乱序

**为什么需要严格递增的 EventVersion**：
- 如果 EventVersion 不是严格递增的（如使用全局索引），会导致误判
- 只有每个聚合ID的 EventVersion 严格递增（1, 2, 3, ...），才能准确检测顺序违反

**示例**：
```go
// ❌ 错误：使用全局索引
EventVersion: int64(i)  // 0, 1, 2, 3, 4, ...

// 结果：
// aggregate-0: version 0, 50, 100, ...  ← 不连续，误判为顺序违反
// aggregate-1: version 1, 51, 101, ...  ← 不连续，误判为顺序违反

// ✅ 正确：使用每个聚合ID的递增版本号
version := atomic.AddInt64(&aggregateVersions[aggregateIndex], 1)
EventVersion: version  // 每个聚合ID: 1, 2, 3, 4, ...

// 结果：
// aggregate-0: version 1, 2, 3, 4, ...  ← 连续递增，正确检测
// aggregate-1: version 1, 2, 3, 4, ...  ← 连续递增，正确检测
```

---

## 🎯 结论

1. **EventVersion 不是 Keyed-Worker Pool 需要的**
   - Keyed-Worker Pool 只需要 AggregateID 进行路由
   - 顺序保证是通过消息到达顺序实现的，而不是 EventVersion

2. **EventVersion 是业务层面的需求**
   - 主要用于事件溯源（Event Sourcing）
   - 作为消息元数据传递

3. **测试代码使用 EventVersion 是为了验证顺序**
   - 通过检查 EventVersion 是否严格递增来验证顺序
   - 这是测试 Keyed-Worker Pool 是否正确工作的手段
   - 不是 Keyed-Worker Pool 的功能需求

4. **修复顺序违反问题的关键**
   - 确保每个聚合ID的 EventVersion 严格递增
   - 确保同一聚合ID的消息由一个 goroutine 串行发送
   - 这样才能准确验证 Keyed-Worker Pool 的顺序保证

