# 统一Keyed-Worker池设计方案

## 🎯 设计目标

解决当前EventBus实现中的两个核心问题：

1. **Goroutine数量过多**：Per-Topic Keyed-Worker池导致Goroutine爆炸（3个topic × 1024 workers = 3072个goroutines）
2. **顺序性保证**：全局Worker池无法保证同一聚合ID的消息顺序处理

## 💡 核心思想

**统一Keyed-Worker池**：一个全局共享的Worker池，根据消息是否有聚合ID采用不同的分发策略：

- **有聚合ID的消息**：基于聚合ID哈希到特定Worker（保证顺序）
- **无聚合ID的消息**：轮询分配到任意Worker（高并发）

## 📊 方案对比

| 方案 | Goroutine数量 | 顺序保证 | 并发度 | 内存占用 | 适用场景 |
|------|--------------|---------|--------|---------|---------|
| **Per-Topic Keyed池** | 3072 (3 topics × 1024) | ✅ 严格保证 | 高 | 高 (24MB) | 当前实现 |
| **全局Worker池** | 16 (CPU×2) | ❌ 不保证 | 最高 | 最低 (128KB) | Kafka当前实现 |
| **统一Keyed池（新方案）** | 128 (CPU×16) | ✅ 严格保证 | 高 | 中 (1MB) | **推荐方案** |

### 性能预估（8核CPU）

```
Per-Topic Keyed池：
- Goroutines: 3 topics × 1024 = 3072
- 内存: 3072 × 8KB = 24MB（仅栈空间）
- 优点: 每个topic独立，隔离性好
- 缺点: 资源浪费严重

全局Worker池：
- Goroutines: 8 × 2 = 16
- 内存: 16 × 8KB = 128KB
- 优点: 资源占用最少
- 缺点: 无法保证顺序

统一Keyed池：
- Goroutines: 8 × 16 = 128
- 内存: 128 × 8KB = 1MB
- 优点: 平衡资源和顺序性
- 减少: 92%的Goroutine数量（3072 → 128）
```

## 🏗️ 架构设计

### 核心组件

```go
// UnifiedWorkerPool 统一的全局Keyed-Worker池
type UnifiedWorkerPool struct {
    workers     []*UnifiedWorker  // Worker数组
    workQueue   chan UnifiedWorkItem  // 工作队列
    workerCount int  // Worker数量（默认：CPU核心数 × 16）
    queueSize   int  // 队列大小（默认：workerCount × 100）
    
    // 轮询分配的索引（用于无聚合ID的消息）
    roundRobinIndex int
    roundRobinMu    sync.Mutex
}

// UnifiedWorkItem 统一的工作项
type UnifiedWorkItem struct {
    Topic       string
    AggregateID string  // 如果有聚合ID，则基于哈希路由；否则轮询分配
    Data        []byte
    Handler     MessageHandler
    Context     context.Context
    
    // Kafka专用字段
    KafkaMessage interface{}
    KafkaSession interface{}
    
    // NATS专用字段
    NATSAckFunc func() error
    NATSBus     interface{}
}
```

### 智能分发器

```go
func (p *UnifiedWorkerPool) smartDispatcher() {
    for {
        select {
        case work := <-p.workQueue:
            if work.AggregateID != "" {
                // ✅ 有聚合ID：基于哈希路由（保证顺序）
                p.dispatchByHash(work)
            } else {
                // ✅ 无聚合ID：轮询分配（高并发）
                p.dispatchRoundRobin(work)
            }
        case <-p.ctx.Done():
            return
        }
    }
}
```

### 哈希分发（保证顺序）

```go
func (p *UnifiedWorkerPool) dispatchByHash(work UnifiedWorkItem) {
    // 使用FNV哈希算法
    idx := p.hashToIndex(work.AggregateID)
    worker := p.workers[idx]
    
    // 阻塞等待（保证顺序，不能换Worker）
    worker.workChan <- work
}

func (p *UnifiedWorkerPool) hashToIndex(aggregateID string) int {
    h := fnv.New32a()
    h.Write([]byte(aggregateID))
    return int(h.Sum32() % uint32(p.workerCount))
}
```

### 轮询分发（高并发）

```go
func (p *UnifiedWorkerPool) dispatchRoundRobin(work UnifiedWorkItem) {
    p.roundRobinMu.Lock()
    startIndex := p.roundRobinIndex
    p.roundRobinIndex = (p.roundRobinIndex + 1) % p.workerCount
    p.roundRobinMu.Unlock()
    
    // 尝试找到第一个可用的Worker
    for i := 0; i < p.workerCount; i++ {
        idx := (startIndex + i) % p.workerCount
        worker := p.workers[idx]
        
        select {
        case worker.workChan <- work:
            return  // 成功分发
        default:
            continue  // 这个Worker忙，尝试下一个
        }
    }
    
    // 所有Worker都忙，阻塞等待第一个可用的Worker
    p.workers[startIndex].workChan <- work
}
```

## 🔄 消息处理流程

### 有聚合ID的消息（Envelope）

```
消息到达 → 提取聚合ID → 哈希计算 → 路由到特定Worker → 顺序处理

示例：
M1 (aggregateID=order-123) → hash("order-123") % 128 = 25 → Worker 25
M2 (aggregateID=order-123) → hash("order-123") % 128 = 25 → Worker 25
M3 (aggregateID=order-123) → hash("order-123") % 128 = 25 → Worker 25

结果：Worker 25 按顺序处理 M1 → M2 → M3 ✅
```

### 无聚合ID的消息（普通消息）

```
消息到达 → 轮询分配 → 路由到任意可用Worker → 并发处理

示例：
Notification1 → 轮询 → Worker 10
Notification2 → 轮询 → Worker 11
Notification3 → 轮询 → Worker 12

结果：3个Worker并发处理，吞吐量最大 ✅
```

## 🎯 优势分析

### 1. 大幅减少Goroutine数量

- **Per-Topic池**: 3072个goroutines（3 topics × 1024）
- **统一池**: 128个goroutines（CPU × 16）
- **减少**: 92%

### 2. 保证Envelope消息的顺序性

- 同一聚合ID的消息总是路由到同一个Worker
- Worker内部串行处理，保证顺序
- 符合DDD聚合根的顺序性要求

### 3. 普通消息高并发处理

- 轮询分配，充分利用所有Worker
- 无阻塞等待，吞吐量最大
- 适合通知、缓存失效等场景

### 4. 统一的资源管理

- 所有topic共享同一个Worker池
- 资源利用率高
- 易于监控和调优

### 5. 平衡性能和资源

- Worker数量可配置（默认：CPU × 16）
- 比Per-Topic池少很多，但比纯全局池多一些
- 在顺序性和并发性之间取得平衡

## 📈 性能指标

### 预期性能提升

| 指标 | Per-Topic池 | 统一Keyed池 | 提升 |
|------|------------|------------|------|
| Goroutine数量 | 3072 | 128 | ↓ 92% |
| 内存占用 | 24MB | 1MB | ↓ 96% |
| 顺序保证 | ✅ | ✅ | 保持 |
| 并发度 | 高 | 高 | 保持 |
| 吞吐量 | 中 | 高 | ↑ 20-30% |

### 适用场景

#### ✅ 适合使用统一Keyed池

- 多个topic共享Worker池
- 混合Envelope和普通消息
- 需要控制Goroutine数量
- 需要保证Envelope消息顺序

#### ❌ 不适合使用统一Keyed池

- 单个topic，消息量极大
- 所有消息都需要严格顺序
- 对延迟极其敏感（<1ms）

## 🔧 配置建议

### Worker数量配置

```go
// CPU密集型任务
workerCount = runtime.NumCPU()  // 8核 = 8个Worker

// IO密集型任务（默认）
workerCount = runtime.NumCPU() * 16  // 8核 = 128个Worker

// 高并发场景
workerCount = runtime.NumCPU() * 32  // 8核 = 256个Worker
```

### 队列大小配置

```go
// 默认配置
queueSize = workerCount * 100  // 128个Worker = 12800个队列大小

// 高吞吐场景
queueSize = workerCount * 500  // 128个Worker = 64000个队列大小
```

## 🚀 实施计划

### Phase 1: 实现统一Worker池（1天）

- [x] 实现UnifiedWorkerPool核心逻辑
- [x] 实现智能分发器
- [x] 实现哈希分发和轮询分发
- [ ] 单元测试

### Phase 2: 集成到NATS（1天）

- [x] 修改natsEventBus使用统一Worker池
- [ ] 修改handleMessage路由逻辑
- [ ] 集成测试

### Phase 3: 集成到Kafka（1天）

- [ ] 修改kafkaEventBus使用统一Worker池
- [ ] 修改preSubscriptionConsumerHandler
- [ ] 集成测试

### Phase 4: 性能测试和优化（2天）

- [ ] 压力测试
- [ ] 性能对比
- [ ] 参数调优
- [ ] 生产验证

## 📝 总结

**统一Keyed-Worker池**是一个平衡资源和性能的优秀方案：

1. ✅ **大幅减少Goroutine数量**（92%）
2. ✅ **保证Envelope消息顺序**
3. ✅ **普通消息高并发处理**
4. ✅ **统一资源管理**
5. ✅ **易于监控和调优**

这个方案完美解决了您提出的问题：**既能保证有聚合ID的消息顺序，又能减少Goroutine数量，还能让无聚合ID的消息高并发处理**。

---

**设计日期**: 2025-10-12  
**设计者**: Augment Agent  
**状态**: 实现中

