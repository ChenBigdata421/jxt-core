# jxt-core EventBus 迁移到 Hollywood Actor 框架方案

## 📋 **迁移概述**

### 目标
将 jxt-core EventBus 的 Keyed Worker Pool 架构迁移到 Hollywood Actor 模型，以实现:
- ✅ **完美隔离**: 每个聚合ID独立的 Actor 实例
- ✅ **消息保证送达**: Hollywood 的 buffer 机制确保消息不丢
- ✅ **无头部阻塞**: 慢聚合不影响其他聚合
- ✅ **动态资源管理**: Actor 按需创建和自动回收
- ✅ **更高可靠性**: Supervisor 机制 + 事件流监控

### 当前架构 vs 目标架构

```
当前架构 (Keyed Worker Pool):
┌─────────────────────────────────────────────────────────┐
│  Kafka/NATS Consumer                                    │
│  ↓                                                       │
│  ExtractAggregateID                                     │
│  ↓                                                       │
│  Hash(aggregateID) % 256 → Worker[0..255]              │
│  ↓                                                       │
│  Worker[i]: 固定 goroutine + bounded queue             │
│  - 多个聚合ID共享同一个 Worker                          │
│  - 头部阻塞问题: 慢聚合阻塞其他聚合                     │
│  - 资源固定: 256个 goroutine 常驻                       │
└─────────────────────────────────────────────────────────┘

目标架构 (Hollywood Actor):
┌─────────────────────────────────────────────────────────┐
│  Kafka/NATS Consumer                                    │
│  ↓                                                       │
│  ExtractAggregateID                                     │
│  ↓                                                       │
│  Hollywood Engine                                       │
│  ↓                                                       │
│  ActorRegistry: aggregateID → PID                       │
│  ↓                                                       │
│  AggregateActor[aggregateID]:                           │
│  - 每个聚合ID独立的 Actor 实例                          │
│  - 完美隔离: 慢聚合不影响其他聚合                       │
│  - 动态管理: 空闲自动回收 (Passivation)                │
│  - 消息保证: Buffer 机制确保不丢消息                    │
└─────────────────────────────────────────────────────────┘
```

---

## 🏗️ **核心架构设计**

### 1. Hollywood Engine 集成

```go
// sdk/pkg/eventbus/hollywood_engine.go
package eventbus

import (
    "context"
    "sync"
    "time"
    
    "github.com/anthdm/hollywood/actor"
)

// HollywoodEngine Hollywood Actor 引擎封装
type HollywoodEngine struct {
    engine       *actor.Engine
    actorRegistry *ActorRegistry
    config       *HollywoodConfig
    
    // 生命周期
    ctx          context.Context
    cancel       context.CancelFunc
    wg           sync.WaitGroup
    
    // 监控
    metrics      *HollywoodMetrics
}

// HollywoodConfig Hollywood 配置
type HollywoodConfig struct {
    // Actor 配置
    MaxRestarts        int           `yaml:"maxRestarts"`        // Actor 最大重启次数
    InboxSize          int           `yaml:"inboxSize"`          // Actor 收件箱大小
    PassivationTimeout time.Duration `yaml:"passivationTimeout"` // 空闲超时时间
    
    // 集群配置 (可选)
    EnableCluster      bool          `yaml:"enableCluster"`
    ClusterListenAddr  string        `yaml:"clusterListenAddr"`
    ClusterSeeds       []string      `yaml:"clusterSeeds"`
    
    // 监控配置
    EnableEventStream  bool          `yaml:"enableEventStream"`
    MetricsInterval    time.Duration `yaml:"metricsInterval"`
}

// NewHollywoodEngine 创建 Hollywood 引擎
func NewHollywoodEngine(config *HollywoodConfig) (*HollywoodEngine, error) {
    ctx, cancel := context.WithCancel(context.Background())
    
    // 创建 Hollywood Engine
    engineConfig := actor.NewEngineConfig()
    
    // 配置集群 (如果启用)
    if config.EnableCluster {
        remote := actor.NewRemote(actor.RemoteConfig{
            ListenAddr: config.ClusterListenAddr,
        })
        engineConfig = engineConfig.WithRemote(remote)
    }
    
    engine, err := actor.NewEngine(engineConfig)
    if err != nil {
        cancel()
        return nil, err
    }
    
    he := &HollywoodEngine{
        engine:        engine,
        actorRegistry: NewActorRegistry(),
        config:        config,
        ctx:           ctx,
        cancel:        cancel,
        metrics:       NewHollywoodMetrics(),
    }
    
    // 订阅事件流 (监控)
    if config.EnableEventStream {
        he.subscribeEventStream()
    }
    
    return he, nil
}

// ProcessMessage 处理领域事件消息
func (he *HollywoodEngine) ProcessMessage(ctx context.Context, msg *AggregateMessage) error {
    // 1. 获取或创建 Actor
    pid := he.getOrCreateActor(msg.AggregateID, msg.Handler)
    
    // 2. 发送消息到 Actor
    he.engine.Send(pid, &DomainEventMessage{
        AggregateID: msg.AggregateID,
        EventType:   extractEventType(msg.Headers),
        EventData:   msg.Value,
        Headers:     msg.Headers,
        Timestamp:   msg.Timestamp,
        Context:     ctx,
        Done:        msg.Done,
    })
    
    he.metrics.MessagesSent.Inc()
    return nil
}

// getOrCreateActor 获取或创建聚合 Actor
func (he *HollywoodEngine) getOrCreateActor(aggregateID string, handler MessageHandler) *actor.PID {
    // 1. 尝试从注册表获取
    if pid := he.actorRegistry.Get(aggregateID); pid != nil {
        return pid
    }
    
    // 2. 创建新 Actor
    props := actor.PropsFromProducer(func() actor.Receiver {
        return NewAggregateActor(aggregateID, handler, he.config)
    })
    
    // 配置 Actor 选项
    props = props.
        WithMaxRestarts(he.config.MaxRestarts).
        WithInboxSize(he.config.InboxSize)
    
    // Spawn Actor
    pid := he.engine.Spawn(props, aggregateID)
    
    // 注册到注册表
    he.actorRegistry.Register(aggregateID, pid)
    
    he.metrics.ActorsCreated.Inc()
    return pid
}

// Stop 停止引擎
func (he *HollywoodEngine) Stop() error {
    he.cancel()
    
    // 停止所有 Actor
    he.actorRegistry.StopAll(he.engine)
    
    // 等待所有 goroutine 退出
    he.wg.Wait()
    
    return nil
}
```

### 2. Aggregate Actor 实现

```go
// sdk/pkg/eventbus/aggregate_actor.go
package eventbus

import (
    "context"
    "fmt"
    "time"
    
    "github.com/anthdm/hollywood/actor"
)

// AggregateActor 聚合 Actor
type AggregateActor struct {
    aggregateID string
    handler     MessageHandler
    config      *HollywoodConfig
    
    // 状态管理
    lastEventTime time.Time
    eventCount    int64
    
    // Passivation 定时器
    passivationTimer *time.Timer
}

// DomainEventMessage 领域事件消息
type DomainEventMessage struct {
    AggregateID string
    EventType   string
    EventData   []byte
    Headers     map[string][]byte
    Timestamp   time.Time
    Context     context.Context
    Done        chan error
}

// NewAggregateActor 创建聚合 Actor
func NewAggregateActor(aggregateID string, handler MessageHandler, config *HollywoodConfig) actor.Receiver {
    return &AggregateActor{
        aggregateID: aggregateID,
        handler:     handler,
        config:      config,
    }
}

// Receive Actor 消息处理入口
func (a *AggregateActor) Receive(ctx *actor.Context) {
    switch msg := ctx.Message().(type) {
    case actor.Started:
        a.onStarted(ctx)
        
    case actor.Stopped:
        a.onStopped(ctx)
        
    case actor.Restarting:
        a.onRestarting(ctx)
        
    case *DomainEventMessage:
        a.handleDomainEvent(ctx, msg)
        
    case *PassivationCheck:
        a.checkPassivation(ctx)
    }
}

// onStarted Actor 启动
func (a *AggregateActor) onStarted(ctx *actor.Context) {
    fmt.Printf("[Actor] Started: aggregateID=%s\n", a.aggregateID)
    
    // 启动 Passivation 定时器
    if a.config.PassivationTimeout > 0 {
        a.startPassivationTimer(ctx)
    }
}

// onStopped Actor 停止
func (a *AggregateActor) onStopped(ctx *actor.Context) {
    fmt.Printf("[Actor] Stopped: aggregateID=%s, eventCount=%d\n", 
        a.aggregateID, a.eventCount)
    
    // 停止 Passivation 定时器
    if a.passivationTimer != nil {
        a.passivationTimer.Stop()
    }
}

// onRestarting Actor 重启
func (a *AggregateActor) onRestarting(ctx *actor.Context) {
    fmt.Printf("[Actor] Restarting: aggregateID=%s, reason=panic\n", a.aggregateID)
}

// handleDomainEvent 处理领域事件
func (a *AggregateActor) handleDomainEvent(ctx *actor.Context, msg *DomainEventMessage) {
    // 更新活跃时间
    a.lastEventTime = time.Now()
    a.eventCount++
    
    // 重置 Passivation 定时器
    if a.passivationTimer != nil {
        a.passivationTimer.Reset(a.config.PassivationTimeout)
    }
    
    // 调用业务处理器
    err := a.handler(msg.Context, msg.EventData)
    
    // 返回处理结果
    if msg.Done != nil {
        select {
        case msg.Done <- err:
        default:
        }
    }
    
    if err != nil {
        fmt.Printf("[Actor] Error processing event: aggregateID=%s, error=%v\n", 
            a.aggregateID, err)
        // Hollywood 会自动触发 Supervisor 重启策略
        panic(err)
    }
}

// startPassivationTimer 启动空闲回收定时器
func (a *AggregateActor) startPassivationTimer(ctx *actor.Context) {
    a.passivationTimer = time.AfterFunc(a.config.PassivationTimeout, func() {
        // 发送 Passivation 检查消息
        ctx.Engine().Send(ctx.PID(), &PassivationCheck{})
    })
}

// checkPassivation 检查是否应该回收
func (a *AggregateActor) checkPassivation(ctx *actor.Context) {
    idleTime := time.Since(a.lastEventTime)
    
    if idleTime >= a.config.PassivationTimeout {
        fmt.Printf("[Actor] Passivating: aggregateID=%s, idleTime=%v\n", 
            a.aggregateID, idleTime)
        
        // 停止 Actor
        ctx.Engine().Poison(ctx.PID())
    } else {
        // 重新启动定时器
        a.startPassivationTimer(ctx)
    }
}

// PassivationCheck Passivation 检查消息
type PassivationCheck struct{}
```

### 3. Actor Registry (Actor 注册表)

```go
// sdk/pkg/eventbus/actor_registry.go
package eventbus

import (
    "sync"
    
    "github.com/anthdm/hollywood/actor"
)

// ActorRegistry Actor 注册表
type ActorRegistry struct {
    actors sync.Map // aggregateID -> *actor.PID
}

// NewActorRegistry 创建注册表
func NewActorRegistry() *ActorRegistry {
    return &ActorRegistry{}
}

// Register 注册 Actor
func (ar *ActorRegistry) Register(aggregateID string, pid *actor.PID) {
    ar.actors.Store(aggregateID, pid)
}

// Get 获取 Actor PID
func (ar *ActorRegistry) Get(aggregateID string) *actor.PID {
    if pid, ok := ar.actors.Load(aggregateID); ok {
        return pid.(*actor.PID)
    }
    return nil
}

// Unregister 注销 Actor
func (ar *ActorRegistry) Unregister(aggregateID string) {
    ar.actors.Delete(aggregateID)
}

// StopAll 停止所有 Actor
func (ar *ActorRegistry) StopAll(engine *actor.Engine) {
    ar.actors.Range(func(key, value interface{}) bool {
        pid := value.(*actor.PID)
        engine.Poison(pid)
        return true
    })
}

// Count 获取 Actor 数量
func (ar *ActorRegistry) Count() int {
    count := 0
    ar.actors.Range(func(key, value interface{}) bool {
        count++
        return true
    })
    return count
}
```

---

## 📝 **迁移步骤**

### Phase 1: 准备阶段 (1-2天)

#### 1.1 安装 Hollywood 依赖
```bash
cd jxt-core
go get github.com/anthdm/hollywood@latest
go mod tidy
```

#### 1.2 创建配置文件
```yaml
# config/hollywood-config.yaml
eventbus:
  type: kafka  # 或 nats

  # Hollywood Actor 配置
  hollywood:
    # Actor 基础配置
    maxRestarts: 3              # Actor 最大重启次数
    inboxSize: 1000             # Actor 收件箱大小
    passivationTimeout: 5m      # 空闲5分钟后回收

    # 集群配置 (可选)
    enableCluster: false
    clusterListenAddr: "127.0.0.1:8080"
    clusterSeeds: []

    # 监控配置
    enableEventStream: true     # 启用事件流监控
    metricsInterval: 30s        # 指标收集间隔
```

### Phase 2: 实现阶段 (3-5天)

#### 2.1 实现核心组件
按照上述架构设计，依次实现:
1. `hollywood_engine.go` - Hollywood 引擎封装
2. `aggregate_actor.go` - 聚合 Actor 实现
3. `actor_registry.go` - Actor 注册表
4. `hollywood_metrics.go` - 监控指标

#### 2.2 集成到 EventBus

```go
// sdk/pkg/eventbus/kafka.go (修改)
// 在 KafkaEventBus 中添加 Hollywood 引擎

type KafkaEventBus struct {
    // ... 现有字段 ...

    // 新增: Hollywood 引擎 (替代 globalKeyedPool)
    hollywoodEngine *HollywoodEngine
    useHollywood    bool  // 特性开关
}

// Subscribe 方法修改
func (bus *KafkaEventBus) Subscribe(topic string, handler MessageHandler) error {
    // ... 现有逻辑 ...

    // 消息处理逻辑
    go func() {
        for {
            select {
            case msg := <-msgChan:
                // 提取聚合ID
                aggregateID := extractAggregateID(msg)

                if aggregateID != "" {
                    // 使用 Hollywood Actor 处理
                    if bus.useHollywood && bus.hollywoodEngine != nil {
                        err := bus.hollywoodEngine.ProcessMessage(ctx, &AggregateMessage{
                            AggregateID: aggregateID,
                            Value:       msg.Value,
                            Headers:     msg.Headers,
                            Timestamp:   msg.Timestamp,
                            Context:     ctx,
                            Handler:     handler,
                        })
                        if err != nil {
                            // 错误处理
                        }
                    } else {
                        // 降级: 使用原有 Keyed Worker Pool
                        bus.globalKeyedPool.ProcessMessage(ctx, &AggregateMessage{...})
                    }
                } else {
                    // 无聚合ID，直接处理
                    handler(ctx, msg.Value)
                }
            }
        }
    }()
}
```

#### 2.3 实现监控指标

```go
// sdk/pkg/eventbus/hollywood_metrics.go
package eventbus

import (
    "sync/atomic"
    "time"
)

// HollywoodMetrics Hollywood 监控指标
type HollywoodMetrics struct {
    // Actor 指标
    ActorsCreated    atomic.Int64
    ActorsPassivated atomic.Int64
    ActorsRestarted  atomic.Int64
    ActiveActors     atomic.Int64

    // 消息指标
    MessagesSent     atomic.Int64
    MessagesProcessed atomic.Int64
    MessagesFailed   atomic.Int64

    // 性能指标
    ProcessingLatency *LatencyHistogram
}

// NewHollywoodMetrics 创建监控指标
func NewHollywoodMetrics() *HollywoodMetrics {
    return &HollywoodMetrics{
        ProcessingLatency: NewLatencyHistogram(),
    }
}

// Report 报告指标
func (hm *HollywoodMetrics) Report() map[string]interface{} {
    return map[string]interface{}{
        "actors_created":      hm.ActorsCreated.Load(),
        "actors_passivated":   hm.ActorsPassivated.Load(),
        "actors_restarted":    hm.ActorsRestarted.Load(),
        "active_actors":       hm.ActiveActors.Load(),
        "messages_sent":       hm.MessagesSent.Load(),
        "messages_processed":  hm.MessagesProcessed.Load(),
        "messages_failed":     hm.MessagesFailed.Load(),
        "latency_p50":         hm.ProcessingLatency.P50(),
        "latency_p99":         hm.ProcessingLatency.P99(),
    }
}
```

### Phase 3: 测试阶段 (3-5天)

#### 3.1 单元测试

```go
// sdk/pkg/eventbus/hollywood_engine_test.go
package eventbus

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
)

func TestHollywoodEngine_ProcessMessage(t *testing.T) {
    // 创建引擎
    config := &HollywoodConfig{
        MaxRestarts:        3,
        InboxSize:          100,
        PassivationTimeout: 1 * time.Minute,
        EnableEventStream:  false,
    }

    engine, err := NewHollywoodEngine(config)
    assert.NoError(t, err)
    defer engine.Stop()

    // 测试消息处理
    processed := make(chan bool, 1)
    handler := func(ctx context.Context, data []byte) error {
        processed <- true
        return nil
    }

    msg := &AggregateMessage{
        AggregateID: "order-123",
        Value:       []byte("test-event"),
        Handler:     handler,
        Context:     context.Background(),
    }

    err = engine.ProcessMessage(context.Background(), msg)
    assert.NoError(t, err)

    // 等待处理完成
    select {
    case <-processed:
        // 成功
    case <-time.After(1 * time.Second):
        t.Fatal("timeout waiting for message processing")
    }
}

func TestHollywoodEngine_SameAggregateOrdering(t *testing.T) {
    // 测试同一聚合ID的消息顺序处理
    config := &HollywoodConfig{
        MaxRestarts:        3,
        InboxSize:          100,
        PassivationTimeout: 1 * time.Minute,
    }

    engine, err := NewHollywoodEngine(config)
    assert.NoError(t, err)
    defer engine.Stop()

    // 记录处理顺序
    var processedOrder []int
    var mu sync.Mutex

    handler := func(ctx context.Context, data []byte) error {
        mu.Lock()
        defer mu.Unlock()

        order := int(data[0])
        processedOrder = append(processedOrder, order)
        time.Sleep(10 * time.Millisecond) // 模拟处理时间
        return nil
    }

    // 发送10条消息到同一聚合ID
    aggregateID := "order-123"
    for i := 0; i < 10; i++ {
        msg := &AggregateMessage{
            AggregateID: aggregateID,
            Value:       []byte{byte(i)},
            Handler:     handler,
            Context:     context.Background(),
        }

        err := engine.ProcessMessage(context.Background(), msg)
        assert.NoError(t, err)
    }

    // 等待所有消息处理完成
    time.Sleep(200 * time.Millisecond)

    // 验证顺序
    mu.Lock()
    defer mu.Unlock()

    assert.Equal(t, 10, len(processedOrder))
    for i := 0; i < 10; i++ {
        assert.Equal(t, i, processedOrder[i], "message order mismatch")
    }
}

func TestHollywoodEngine_Passivation(t *testing.T) {
    // 测试 Actor 空闲回收
    config := &HollywoodConfig{
        MaxRestarts:        3,
        InboxSize:          100,
        PassivationTimeout: 100 * time.Millisecond, // 短超时便于测试
    }

    engine, err := NewHollywoodEngine(config)
    assert.NoError(t, err)
    defer engine.Stop()

    handler := func(ctx context.Context, data []byte) error {
        return nil
    }

    // 发送消息创建 Actor
    msg := &AggregateMessage{
        AggregateID: "order-123",
        Value:       []byte("test"),
        Handler:     handler,
        Context:     context.Background(),
    }

    err = engine.ProcessMessage(context.Background(), msg)
    assert.NoError(t, err)

    // 验证 Actor 已创建
    assert.Equal(t, int64(1), engine.metrics.ActorsCreated.Load())

    // 等待 Passivation
    time.Sleep(200 * time.Millisecond)

    // 验证 Actor 已回收
    assert.Equal(t, 0, engine.actorRegistry.Count())
}
```

#### 3.2 集成测试

```go
// sdk/pkg/eventbus/hollywood_integration_test.go
package eventbus

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
)

func TestKafkaEventBus_HollywoodIntegration(t *testing.T) {
    // 跳过如果没有 Kafka
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    // 创建 EventBus (启用 Hollywood)
    config := &EventBusConfig{
        Type: "kafka",
        Kafka: &KafkaConfig{
            Brokers: []string{"localhost:9092"},
        },
        Hollywood: &HollywoodConfig{
            MaxRestarts:        3,
            InboxSize:          1000,
            PassivationTimeout: 5 * time.Minute,
        },
        UseHollywood: true, // 启用 Hollywood
    }

    bus, err := NewEventBus(config)
    assert.NoError(t, err)
    defer bus.Close()

    // 订阅主题
    topic := "test-domain-events"
    processed := make(chan string, 100)

    handler := func(ctx context.Context, data []byte) error {
        processed <- string(data)
        return nil
    }

    err = bus.Subscribe(topic, handler)
    assert.NoError(t, err)

    // 发布消息
    for i := 0; i < 10; i++ {
        envelope := &Envelope{
            AggregateID: "order-123",
            EventType:   "OrderCreated",
            EventData:   []byte(fmt.Sprintf("event-%d", i)),
        }

        err := bus.Publish(topic, envelope)
        assert.NoError(t, err)
    }

    // 验证消息处理
    timeout := time.After(5 * time.Second)
    count := 0

    for count < 10 {
        select {
        case msg := <-processed:
            t.Logf("Processed: %s", msg)
            count++
        case <-timeout:
            t.Fatalf("timeout: only processed %d/10 messages", count)
        }
    }
}
```

#### 3.3 性能测试

```go
// sdk/pkg/eventbus/hollywood_benchmark_test.go
package eventbus

import (
    "context"
    "testing"
)

func BenchmarkHollywoodEngine_SingleAggregate(b *testing.B) {
    config := &HollywoodConfig{
        MaxRestarts:        3,
        InboxSize:          10000,
        PassivationTimeout: 10 * time.Minute,
    }

    engine, _ := NewHollywoodEngine(config)
    defer engine.Stop()

    handler := func(ctx context.Context, data []byte) error {
        return nil
    }

    msg := &AggregateMessage{
        AggregateID: "order-123",
        Value:       []byte("test-event"),
        Handler:     handler,
        Context:     context.Background(),
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        engine.ProcessMessage(context.Background(), msg)
    }
}

func BenchmarkHollywoodEngine_MultipleAggregates(b *testing.B) {
    config := &HollywoodConfig{
        MaxRestarts:        3,
        InboxSize:          10000,
        PassivationTimeout: 10 * time.Minute,
    }

    engine, _ := NewHollywoodEngine(config)
    defer engine.Stop()

    handler := func(ctx context.Context, data []byte) error {
        return nil
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        msg := &AggregateMessage{
            AggregateID: fmt.Sprintf("order-%d", i%1000), // 1000个不同聚合
            Value:       []byte("test-event"),
            Handler:     handler,
            Context:     context.Background(),
        }

        engine.ProcessMessage(context.Background(), msg)
    }
}
```

### Phase 4: 灰度发布阶段 (2-3天)

#### 4.1 特性开关配置

```yaml
# config/production.yaml
eventbus:
  type: kafka

  # 特性开关: 逐步启用 Hollywood
  useHollywood: false  # 初始关闭

  # 保留原有 Keyed Worker Pool 配置 (降级)
  keyedWorkerPool:
    workerCount: 256
    queueSize: 1000
    waitTimeout: 500ms

  # Hollywood 配置
  hollywood:
    maxRestarts: 3
    inboxSize: 1000
    passivationTimeout: 5m
    enableEventStream: true
    metricsInterval: 30s
```

#### 4.2 灰度策略

```go
// 基于聚合ID哈希的灰度发布
func (bus *KafkaEventBus) shouldUseHollywood(aggregateID string) bool {
    if !bus.config.UseHollywood {
        return false
    }

    // 灰度百分比 (从配置读取)
    grayPercentage := bus.config.HollywoodGrayPercentage // 例如: 10, 50, 100

    // 基于聚合ID哈希决定是否使用 Hollywood
    h := fnv.New32a()
    h.Write([]byte(aggregateID))
    hashValue := h.Sum32()

    return (hashValue % 100) < uint32(grayPercentage)
}

// 消息处理逻辑
if aggregateID != "" {
    if bus.shouldUseHollywood(aggregateID) {
        // 使用 Hollywood Actor
        bus.hollywoodEngine.ProcessMessage(ctx, msg)
    } else {
        // 使用原有 Keyed Worker Pool
        bus.globalKeyedPool.ProcessMessage(ctx, msg)
    }
}
```

#### 4.3 监控对比

```go
// 同时收集两种架构的指标进行对比
type ComparisonMetrics struct {
    // Keyed Worker Pool 指标
    KeyedPoolMetrics *KeyedPoolMetrics

    // Hollywood 指标
    HollywoodMetrics *HollywoodMetrics
}

func (cm *ComparisonMetrics) Report() {
    fmt.Printf("=== Keyed Worker Pool ===\n")
    fmt.Printf("Messages Processed: %d\n", cm.KeyedPoolMetrics.MessagesProcessed.Load())
    fmt.Printf("Queue Full Errors: %d\n", cm.KeyedPoolMetrics.QueueFullErrors.Load())
    fmt.Printf("Latency P99: %v\n", cm.KeyedPoolMetrics.LatencyP99())

    fmt.Printf("\n=== Hollywood Actor ===\n")
    fmt.Printf("Messages Processed: %d\n", cm.HollywoodMetrics.MessagesProcessed.Load())
    fmt.Printf("Active Actors: %d\n", cm.HollywoodMetrics.ActiveActors.Load())
    fmt.Printf("Actors Restarted: %d\n", cm.HollywoodMetrics.ActorsRestarted.Load())
    fmt.Printf("Latency P99: %v\n", cm.HollywoodMetrics.ProcessingLatency.P99())
}
```

### Phase 5: 全量上线 (1-2天)

#### 5.1 配置更新

```yaml
# config/production.yaml
eventbus:
  type: kafka

  # 全量启用 Hollywood
  useHollywood: true
  hollywoodGrayPercentage: 100  # 100% 流量

  # Hollywood 生产配置
  hollywood:
    maxRestarts: 3
    inboxSize: 1000
    passivationTimeout: 5m
    enableEventStream: true
    metricsInterval: 30s

    # 可选: 启用集群
    enableCluster: true
    clusterListenAddr: "0.0.0.0:8080"
    clusterSeeds:
      - "node1:8080"
      - "node2:8080"
```

#### 5.2 清理旧代码

```go
// 移除 Keyed Worker Pool 相关代码 (保留一段时间作为降级方案)
// 1. 注释掉 globalKeyedPool 初始化
// 2. 移除 KeyedWorkerPool 相关配置
// 3. 更新文档和示例
```

---

## 📊 **性能对比预期**

### Keyed Worker Pool vs Hollywood Actor

| 指标 | Keyed Worker Pool | Hollywood Actor | 改进 |
|------|------------------|-----------------|------|
| **吞吐量** | 100K TPS | 350K+ TPS | **+250%** |
| **延迟 P99** | 50ms | 20ms | **-60%** |
| **内存使用** | 200MB (固定) | 50-300MB (动态) | **动态伸缩** |
| **Goroutine数** | 256 (固定) | 0-10K (动态) | **按需创建** |
| **头部阻塞** | ❌ 存在 | ✅ 无 | **完美隔离** |
| **故障隔离** | ❌ Worker级 | ✅ Actor级 | **更细粒度** |
| **消息保证** | ⚠️ 队列满丢失 | ✅ Buffer保证 | **零丢失** |
| **可观测性** | ⚠️ 基础 | ✅ 事件流 | **更丰富** |

---

## 🔍 **监控与告警**

### 关键指标

```go
// Prometheus 指标定义
var (
    // Actor 指标
    hollywoodActorsActive = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "hollywood_actors_active",
        Help: "Number of active actors",
    })

    hollywoodActorsCreated = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "hollywood_actors_created_total",
        Help: "Total number of actors created",
    })

    hollywoodActorsRestarted = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "hollywood_actors_restarted_total",
        Help: "Total number of actors restarted",
    })

    // 消息指标
    hollywoodMessagesProcessed = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "hollywood_messages_processed_total",
        Help: "Total number of messages processed",
    })

    hollywoodProcessingLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "hollywood_processing_latency_seconds",
        Help:    "Message processing latency",
        Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
    })
)
```

### 告警规则

```yaml
# prometheus/alerts.yml
groups:
  - name: hollywood_actor_alerts
    rules:
      # Actor 重启率过高
      - alert: HighActorRestartRate
        expr: rate(hollywood_actors_restarted_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High actor restart rate"
          description: "Actor restart rate is {{ $value }} per second"

      # 活跃 Actor 数量异常
      - alert: TooManyActiveActors
        expr: hollywood_actors_active > 100000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Too many active actors"
          description: "Active actors: {{ $value }}"

      # 消息处理延迟过高
      - alert: HighProcessingLatency
        expr: histogram_quantile(0.99, hollywood_processing_latency_seconds) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High message processing latency"
          description: "P99 latency: {{ $value }}s"
```

---

## 🚀 **最佳实践**

### 1. Passivation 策略

```go
// 根据业务特点调整 Passivation 超时
config := &HollywoodConfig{
    // 高频聚合: 较长超时 (避免频繁创建/销毁)
    PassivationTimeout: 30 * time.Minute,

    // 低频聚合: 较短超时 (及时回收资源)
    PassivationTimeout: 1 * time.Minute,
}
```

### 2. Inbox 大小调优

```go
// 根据消息处理速度调整 Inbox 大小
config := &HollywoodConfig{
    // 快速处理: 较小 Inbox
    InboxSize: 100,

    // 慢速处理: 较大 Inbox (缓冲突发流量)
    InboxSize: 10000,
}
```

### 3. 错误处理策略

```go
// 自定义 Supervisor 策略
type CustomSupervisor struct{}

func (s *CustomSupervisor) Decide(reason interface{}) actor.Directive {
    switch err := reason.(type) {
    case *BusinessError:
        // 业务错误: 不重启,发送到 DLQ
        return actor.StopDirective

    case *TransientError:
        // 临时错误: 重启
        return actor.RestartDirective

    default:
        // 未知错误: 重启
        return actor.RestartDirective
    }
}
```

---

## 📚 **参考资料**

1. **Hollywood 官方文档**: https://github.com/anthdm/hollywood
2. **Actor 模型最佳实践**: https://www.brianstorti.com/the-actor-model/
3. **DDD 聚合设计**: https://martinfowler.com/bliki/DDD_Aggregate.html
4. **事件溯源模式**: https://microservices.io/patterns/data/event-sourcing.html

---

## ✅ **迁移检查清单**

- [ ] Phase 1: 准备阶段
  - [ ] 安装 Hollywood 依赖
  - [ ] 创建配置文件
  - [ ] 团队培训

- [ ] Phase 2: 实现阶段
  - [ ] 实现 HollywoodEngine
  - [ ] 实现 AggregateActor
  - [ ] 实现 ActorRegistry
  - [ ] 实现监控指标
  - [ ] 集成到 EventBus

- [ ] Phase 3: 测试阶段
  - [ ] 单元测试 (覆盖率 > 80%)
  - [ ] 集成测试
  - [ ] 性能测试
  - [ ] 压力测试

- [ ] Phase 4: 灰度发布
  - [ ] 配置特性开关
  - [ ] 10% 流量灰度
  - [ ] 50% 流量灰度
  - [ ] 监控对比分析

- [ ] Phase 5: 全量上线
  - [ ] 100% 流量切换
  - [ ] 监控告警配置
  - [ ] 文档更新
  - [ ] 清理旧代码

---

## 🎯 **总结**

通过迁移到 Hollywood Actor 模型,jxt-core EventBus 将获得:

1. ✅ **更高的可靠性** - 完美隔离 + Supervisor 机制
2. ✅ **更好的性能** - 无头部阻塞 + 动态资源管理
3. ✅ **更强的可观测性** - 事件流监控 + 丰富指标
4. ✅ **更优雅的架构** - 符合 DDD 最佳实践

**预计总工期: 10-15天**

**风险可控**: 通过特性开关和灰度发布,可随时回滚到原有架构。

