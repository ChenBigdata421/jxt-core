# jxt-core EventBus Keyed-Worker 池优化方案 (Phase 1)

## 🎯 **设计目标**

- ✅ **100% 顺序保证**：同一聚合ID的领域事件严格按顺序处理
- ✅ **架构简化**：固定大小的 Worker 池，避免复杂的动态管理
- ✅ **资源可控**：有界并发，内存和CPU使用稳定可预测
- ✅ **高可用性**：支持背压控制、毒丸处理、数据完整性校验
- ✅ **易于运维**：简单的配置和监控，故障排查容易

## 🏗️ **核心架构设计**

### 1. **Keyed-Worker 池架构**

```go
// 核心架构组件
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Message       │    │  Consistent      │    │  Keyed-Worker   │
│   Router        │───▶│  Hash Router     │───▶│  Pool (M=1024)  │
│                 │    │                  │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │                        │
                                ▼                        ▼
                       ┌──────────────────┐    ┌─────────────────┐
                       │  aggregateId     │    │  Worker[0..M-1] │
                       │  hash % M        │    │  每个Worker:    │
                       │  = workerIndex   │    │  - 有界队列     │
                       └──────────────────┘    │  - 串行处理     │
                                               │  - 背压控制     │
                                               │  - 顺序校验     │
                                               └─────────────────┘
```

### 2. **Worker 内部结构**

```go
type KeyedWorker struct {
    // 基础属性
    id           int
    queue        chan *OrderedMessage
    queueSize    int
    
    // 顺序控制
    lastVersions map[string]int64  // aggregateId -> lastEventVersion
    waitingMsgs  map[string][]*OrderedMessage  // 等待前驱消息的队列
    
    // 背压控制
    pausedKeys   map[string]bool   // 暂停处理的聚合ID
    
    // 毒丸处理
    retryBudgets map[string]*RetryBudget
    dlqSender    DLQSender
    
    // 生命周期
    ctx          context.Context
    cancel       context.CancelFunc
    wg           sync.WaitGroup
    
    // 监控统计
    stats        *WorkerStats
}

type OrderedMessage struct {
    AggregateID    string
    EventVersion   int64     // 事件版本号，确保顺序
    Payload        []byte
    Headers        map[string][]byte
    Timestamp      time.Time
    
    // 处理控制
    Handler        MessageHandler
    Done           chan error
    RetryCount     int
    
    // 元数据
    SourceTopic    string
    Partition      int32     // Kafka分区
    Offset         int64     // Kafka偏移量
    Sequence       uint64    // NATS序列号
}
```

### 3. **一致性哈希路由**

```go
type ConsistentHashRouter struct {
    workerCount  int
    hashFunc     hash.Hash32
    mu           sync.RWMutex
}

func (chr *ConsistentHashRouter) RouteMessage(aggregateID string) int {
    chr.mu.RLock()
    defer chr.mu.RUnlock()
    
    chr.hashFunc.Reset()
    chr.hashFunc.Write([]byte(aggregateID))
    hashValue := chr.hashFunc.Sum32()
    
    return int(hashValue % uint32(chr.workerCount))
}

// 确保相同聚合ID总是路由到同一个Worker
func (chr *ConsistentHashRouter) GetWorkerForAggregate(aggregateID string) int {
    return chr.RouteMessage(aggregateID)
}
```

---

## 🔧 **核心功能实现**

### 1. **Keyed-Worker 池管理器**

```go
type KeyedWorkerPoolManager struct {
    // Worker池
    workers      []*KeyedWorker
    workerCount  int
    
    // 路由器
    router       *ConsistentHashRouter
    
    // 背压控制
    backpressure *BackpressureController
    
    // 配置
    config       *KeyedWorkerConfig
    
    // 生命周期
    ctx          context.Context
    cancel       context.CancelFunc
    wg           sync.WaitGroup
    
    // 监控
    metrics      *PoolMetrics
}

type KeyedWorkerConfig struct {
    // Worker池配置
    WorkerCount    int           `yaml:"workerCount"`    // 固定Worker数量，如1024
    QueueSize      int           `yaml:"queueSize"`      // 每个Worker队列大小，如1000
    
    // 顺序控制配置
    EnableVersionCheck bool      `yaml:"enableVersionCheck"` // 启用版本顺序检查
    WaitTimeout       time.Duration `yaml:"waitTimeout"`     // 等待前驱消息超时
    
    // 背压控制配置
    BackpressureThreshold float64 `yaml:"backpressureThreshold"` // 背压触发阈值，如0.8
    BackpressureStrategy  string  `yaml:"backpressureStrategy"`  // kafka_pause/nats_flow_control
    
    // 毒丸处理配置
    MaxRetries        int           `yaml:"maxRetries"`        // 最大重试次数，如3
    RetryBackoffBase  time.Duration `yaml:"retryBackoffBase"`  // 重试退避基数，如100ms
    RetryBackoffMax   time.Duration `yaml:"retryBackoffMax"`   // 最大退避时间，如30s
    DLQEnabled        bool          `yaml:"dlqEnabled"`        // 启用死信队列
    DLQTopic          string        `yaml:"dlqTopic"`          // 死信队列主题
}

func NewKeyedWorkerPoolManager(config *KeyedWorkerConfig) (*KeyedWorkerPoolManager, error) {
    ctx, cancel := context.WithCancel(context.Background())
    
    // 创建一致性哈希路由器
    router := &ConsistentHashRouter{
        workerCount: config.WorkerCount,
        hashFunc:    fnv.New32a(),
    }
    
    // 创建Worker池
    workers := make([]*KeyedWorker, config.WorkerCount)
    for i := 0; i < config.WorkerCount; i++ {
        worker, err := NewKeyedWorker(i, config, ctx)
        if err != nil {
            cancel()
            return nil, fmt.Errorf("failed to create worker %d: %w", i, err)
        }
        workers[i] = worker
    }
    
    // 创建背压控制器
    backpressure := NewBackpressureController(config, workers)
    
    manager := &KeyedWorkerPoolManager{
        workers:      workers,
        workerCount:  config.WorkerCount,
        router:       router,
        backpressure: backpressure,
        config:       config,
        ctx:          ctx,
        cancel:       cancel,
        metrics:      NewPoolMetrics(),
    }
    
    // 启动所有Worker
    for _, worker := range workers {
        worker.Start()
    }
    
    // 启动背压监控
    manager.startBackpressureMonitoring()
    
    return manager, nil
}

func (kwpm *KeyedWorkerPoolManager) ProcessMessage(msg *OrderedMessage) error {
    // 1. 路由到对应的Worker
    workerIndex := kwpm.router.RouteMessage(msg.AggregateID)
    worker := kwpm.workers[workerIndex]
    
    // 2. 检查Worker是否暂停
    if worker.IsPaused(msg.AggregateID) {
        return ErrWorkerPaused
    }
    
    // 3. 尝试入队
    select {
    case worker.queue <- msg:
        kwpm.metrics.MessagesEnqueued.Inc()
        return nil
    default:
        // 队列满，触发背压
        kwpm.backpressure.HandleQueueFull(workerIndex, msg.AggregateID)
        return ErrQueueFull
    }
}
```

### 2. **Worker 核心处理逻辑**

```go
func (kw *KeyedWorker) Start() {
    kw.wg.Add(1)
    go kw.processLoop()
}

func (kw *KeyedWorker) processLoop() {
    defer kw.wg.Done()
    
    for {
        select {
        case msg := <-kw.queue:
            kw.processMessage(msg)
            
        case <-kw.ctx.Done():
            return
        }
    }
}

func (kw *KeyedWorker) processMessage(msg *OrderedMessage) {
    defer func() {
        if r := recover(); r != nil {
            kw.handlePanic(msg, r)
        }
    }()
    
    // 1. 检查是否暂停处理该聚合ID
    if kw.pausedKeys[msg.AggregateID] {
        kw.requeueMessage(msg)
        return
    }
    
    // 2. 顺序校验
    if kw.config.EnableVersionCheck {
        if !kw.checkMessageOrder(msg) {
            return // 消息被缓存等待前驱
        }
    }
    
    // 3. 处理消息
    err := kw.handleMessage(msg)
    
    // 4. 处理结果
    if err != nil {
        kw.handleError(msg, err)
    } else {
        kw.handleSuccess(msg)
    }
    
    // 5. 处理等待队列中的后续消息
    if kw.config.EnableVersionCheck {
        kw.processWaitingMessages(msg.AggregateID)
    }
}

func (kw *KeyedWorker) checkMessageOrder(msg *OrderedMessage) bool {
    lastVersion, exists := kw.lastVersions[msg.AggregateID]
    
    if !exists {
        // 第一条消息，直接处理
        return true
    }
    
    expectedVersion := lastVersion + 1
    
    if msg.EventVersion == expectedVersion {
        // 顺序正确，可以处理
        return true
    } else if msg.EventVersion > expectedVersion {
        // 消息超前，需要等待前驱消息
        kw.addToWaitingQueue(msg)
        return false
    } else {
        // 消息重复或过期，发送到DLQ
        kw.sendToDLQ(msg, fmt.Errorf("duplicate or outdated message: expected %d, got %d", 
            expectedVersion, msg.EventVersion))
        return false
    }
}

func (kw *KeyedWorker) handleMessage(msg *OrderedMessage) error {
    // 执行业务处理逻辑
    return msg.Handler(msg.Payload, msg.Headers)
}

func (kw *KeyedWorker) handleError(msg *OrderedMessage, err error) {
    // 获取重试预算
    budget := kw.getRetryBudget(msg.AggregateID)
    
    if budget.CanRetry() {
        // 指数退避重试
        backoffDuration := kw.calculateBackoff(msg.RetryCount)
        
        time.AfterFunc(backoffDuration, func() {
            msg.RetryCount++
            budget.UseRetry()
            
            select {
            case kw.queue <- msg:
                // 重新入队成功
            default:
                // 队列满，直接发送到DLQ
                kw.sendToDLQ(msg, fmt.Errorf("retry queue full: %w", err))
            }
        })
    } else {
        // 重试预算耗尽，发送到DLQ
        kw.sendToDLQ(msg, fmt.Errorf("retry budget exhausted: %w", err))
    }
}

func (kw *KeyedWorker) handleSuccess(msg *OrderedMessage) {
    // 更新版本号
    if kw.config.EnableVersionCheck {
        kw.lastVersions[msg.AggregateID] = msg.EventVersion
    }
    
    // 重置重试预算
    kw.resetRetryBudget(msg.AggregateID)
    
    // 通知处理完成
    if msg.Done != nil {
        msg.Done <- nil
    }
    
    // 更新统计
    kw.stats.MessagesProcessed.Inc()
}
```

### 3. **背压控制机制**

```go
type BackpressureController struct {
    config      *KeyedWorkerConfig
    workers     []*KeyedWorker
    
    // Kafka背压控制
    kafkaConsumer KafkaConsumerController
    
    // NATS背压控制
    natsConsumer  NATSConsumerController
    
    // 监控
    metrics       *BackpressureMetrics
}

func (bc *BackpressureController) HandleQueueFull(workerIndex int, aggregateID string) {
    worker := bc.workers[workerIndex]
    
    // 计算队列使用率
    queueUsage := float64(len(worker.queue)) / float64(cap(worker.queue))
    
    if queueUsage >= bc.config.BackpressureThreshold {
        switch bc.config.BackpressureStrategy {
        case "kafka_pause":
            bc.handleKafkaBackpressure(workerIndex, aggregateID)
        case "nats_flow_control":
            bc.handleNATSBackpressure(workerIndex, aggregateID)
        }
    }
}

func (bc *BackpressureController) handleKafkaBackpressure(workerIndex int, aggregateID string) {
    // 暂停相关分区的消费
    partitions := bc.kafkaConsumer.GetPartitionsForAggregate(aggregateID)
    
    for _, partition := range partitions {
        bc.kafkaConsumer.PausePartition(partition)
        bc.metrics.PausedPartitions.Inc()
    }
    
    // 启动恢复监控
    go bc.monitorKafkaRecovery(workerIndex, aggregateID, partitions)
}

func (bc *BackpressureController) handleNATSBackpressure(workerIndex int, aggregateID string) {
    // 降低拉取速率
    bc.natsConsumer.ReduceFetchRate(aggregateID)
    
    // 收紧MaxAckPending
    bc.natsConsumer.SetMaxAckPending(aggregateID, 10) // 降低到10
    
    // 启用Flow Control
    bc.natsConsumer.EnableFlowControl(aggregateID)
    
    bc.metrics.FlowControlActivated.Inc()
    
    // 启动恢复监控
    go bc.monitorNATSRecovery(workerIndex, aggregateID)
}

func (bc *BackpressureController) monitorKafkaRecovery(workerIndex int, aggregateID string, partitions []int32) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        worker := bc.workers[workerIndex]
        queueUsage := float64(len(worker.queue)) / float64(cap(worker.queue))
        
        if queueUsage < 0.5 { // 队列使用率降到50%以下
            // 恢复分区消费
            for _, partition := range partitions {
                bc.kafkaConsumer.ResumePartition(partition)
                bc.metrics.ResumedPartitions.Inc()
            }
            return
        }
    }
}
```

---

## 📋 **配置示例**

### 1. **生产环境配置**

```yaml
# config/keyed-worker-pool-production.yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["kafka-1:9092", "kafka-2:9092", "kafka-3:9092"]
    consumerGroup: "order-service"
  
  # Keyed-Worker池配置
  keyedWorkerPool:
    # Worker池基础配置
    workerCount: 1024              # 固定1024个Worker
    queueSize: 1000                # 每个Worker队列1000条消息
    
    # 顺序控制配置
    enableVersionCheck: true       # 启用版本顺序检查
    waitTimeout: 30s               # 等待前驱消息30秒超时
    
    # 背压控制配置
    backpressureThreshold: 0.8     # 队列80%满时触发背压
    backpressureStrategy: "kafka_pause"  # Kafka暂停分区策略
    
    # 毒丸处理配置
    maxRetries: 3                  # 最大重试3次
    retryBackoffBase: 100ms        # 基础退避100ms
    retryBackoffMax: 30s           # 最大退避30秒
    dlqEnabled: true               # 启用死信队列
    dlqTopic: "order-events-dlq"   # 死信队列主题
    
    # 监控配置
    metricsEnabled: true           # 启用指标收集
    metricsInterval: 30s           # 指标收集间隔
```

### 2. **开发环境配置**

```yaml
# config/keyed-worker-pool-development.yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]
    consumerGroup: "dev-order-service"
  
  keyedWorkerPool:
    workerCount: 64                # 开发环境较少Worker
    queueSize: 100                 # 较小队列便于测试
    
    enableVersionCheck: true       # 保持顺序检查
    waitTimeout: 10s               # 较短超时便于调试
    
    backpressureThreshold: 0.7     # 更早触发背压便于测试
    backpressureStrategy: "kafka_pause"
    
    maxRetries: 2                  # 较少重试便于调试
    retryBackoffBase: 50ms
    retryBackoffMax: 5s
    dlqEnabled: true
    dlqTopic: "dev-order-events-dlq"
    
    metricsEnabled: true
    metricsInterval: 10s           # 更频繁的指标收集
```

---

## 📊 **性能预期**

### 1. **并发处理能力**

```go
// 基于固定Worker池的并发能力
Worker数量: 1024个
每Worker队列: 1000条消息
理论并发消息: 1,024,000条

// 实际聚合ID并发处理能力
同时处理的聚合ID数量: 无限制 (通过哈希分散)
每个聚合ID: 严格串行处理
不同聚合ID: 完全并发处理

// 资源使用预测
内存占用: ~200MB (1024 * 1000 * 200字节/消息)
Goroutine数量: 1024个 (每Worker一个)
CPU使用: 可控且稳定
```

### 2. **性能基准**

```go
// 预期性能指标
吞吐量: 100,000+ TPS
延迟: P99 < 50ms
内存使用: 稳定在200MB左右
CPU使用: 60-80%
顺序准确性: 100%
可用性: 99.9%+
```

这个 Phase 1 方案通过固定大小的 Keyed-Worker 池，既保证了同一聚合ID的严格顺序处理，又大大简化了架构复杂度，资源使用稳定可控，是一个非常实用的落地方案。
