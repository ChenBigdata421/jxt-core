# Hollywood Actor 实现示例代码

## 📁 **文件结构**

```
jxt-core/sdk/pkg/eventbus/
├── hollywood_engine.go          # Hollywood 引擎封装
├── aggregate_actor.go           # 聚合 Actor 实现
├── actor_registry.go            # Actor 注册表
├── hollywood_metrics.go         # 监控指标
├── hollywood_config.go          # 配置定义
├── hollywood_engine_test.go     # 单元测试
└── examples/
    └── hollywood_example.go     # 使用示例
```

---

## 1️⃣ **hollywood_config.go**

```go
package eventbus

import "time"

// HollywoodConfig Hollywood Actor 配置
type HollywoodConfig struct {
    // Actor 基础配置
    MaxRestarts        int           `yaml:"maxRestarts" json:"maxRestarts"`
    InboxSize          int           `yaml:"inboxSize" json:"inboxSize"`
    PassivationTimeout time.Duration `yaml:"passivationTimeout" json:"passivationTimeout"`
    
    // 集群配置 (可选)
    EnableCluster      bool          `yaml:"enableCluster" json:"enableCluster"`
    ClusterListenAddr  string        `yaml:"clusterListenAddr" json:"clusterListenAddr"`
    ClusterSeeds       []string      `yaml:"clusterSeeds" json:"clusterSeeds"`
    
    // 监控配置
    EnableEventStream  bool          `yaml:"enableEventStream" json:"enableEventStream"`
    MetricsInterval    time.Duration `yaml:"metricsInterval" json:"metricsInterval"`
}

// DefaultHollywoodConfig 默认配置
func DefaultHollywoodConfig() *HollywoodConfig {
    return &HollywoodConfig{
        MaxRestarts:        3,
        InboxSize:          1000,
        PassivationTimeout: 5 * time.Minute,
        EnableCluster:      false,
        EnableEventStream:  true,
        MetricsInterval:    30 * time.Second,
    }
}

// Validate 验证配置
func (c *HollywoodConfig) Validate() error {
    if c.MaxRestarts < 0 {
        return fmt.Errorf("maxRestarts must be >= 0")
    }
    if c.InboxSize <= 0 {
        return fmt.Errorf("inboxSize must be > 0")
    }
    if c.PassivationTimeout < 0 {
        return fmt.Errorf("passivationTimeout must be >= 0")
    }
    return nil
}
```

---

## 2️⃣ **hollywood_metrics.go**

```go
package eventbus

import (
    "sync"
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
    MessagesSent      atomic.Int64
    MessagesProcessed atomic.Int64
    MessagesFailed    atomic.Int64
    
    // 延迟指标
    latencies []time.Duration
    mu        sync.RWMutex
}

// NewHollywoodMetrics 创建监控指标
func NewHollywoodMetrics() *HollywoodMetrics {
    return &HollywoodMetrics{
        latencies: make([]time.Duration, 0, 10000),
    }
}

// RecordLatency 记录延迟
func (hm *HollywoodMetrics) RecordLatency(d time.Duration) {
    hm.mu.Lock()
    defer hm.mu.Unlock()
    
    hm.latencies = append(hm.latencies, d)
    
    // 保留最近 10000 条
    if len(hm.latencies) > 10000 {
        hm.latencies = hm.latencies[1:]
    }
}

// LatencyP99 获取 P99 延迟
func (hm *HollywoodMetrics) LatencyP99() time.Duration {
    hm.mu.RLock()
    defer hm.mu.RUnlock()
    
    if len(hm.latencies) == 0 {
        return 0
    }
    
    // 简单实现: 排序后取 99%
    sorted := make([]time.Duration, len(hm.latencies))
    copy(sorted, hm.latencies)
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i] < sorted[j]
    })
    
    idx := int(float64(len(sorted)) * 0.99)
    return sorted[idx]
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
        "latency_p99_ms":      hm.LatencyP99().Milliseconds(),
    }
}

// Reset 重置指标
func (hm *HollywoodMetrics) Reset() {
    hm.ActorsCreated.Store(0)
    hm.ActorsPassivated.Store(0)
    hm.ActorsRestarted.Store(0)
    hm.MessagesSent.Store(0)
    hm.MessagesProcessed.Store(0)
    hm.MessagesFailed.Store(0)
    
    hm.mu.Lock()
    hm.latencies = hm.latencies[:0]
    hm.mu.Unlock()
}
```

---

## 3️⃣ **actor_registry.go**

```go
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

// List 列出所有 Actor ID
func (ar *ActorRegistry) List() []string {
    var ids []string
    ar.actors.Range(func(key, value interface{}) bool {
        ids = append(ids, key.(string))
        return true
    })
    return ids
}
```

---

## 4️⃣ **aggregate_actor.go**

```go
package eventbus

import (
    "context"
    "fmt"
    "time"
    
    "github.com/anthdm/hollywood/actor"
)

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

// PassivationCheck Passivation 检查消息
type PassivationCheck struct{}

// AggregateActor 聚合 Actor
type AggregateActor struct {
    aggregateID string
    handler     MessageHandler
    config      *HollywoodConfig
    metrics     *HollywoodMetrics
    
    // 状态管理
    lastEventTime time.Time
    eventCount    int64
    
    // Passivation 定时器
    passivationTimer *time.Timer
}

// NewAggregateActor 创建聚合 Actor
func NewAggregateActor(
    aggregateID string,
    handler MessageHandler,
    config *HollywoodConfig,
    metrics *HollywoodMetrics,
) actor.Receiver {
    return &AggregateActor{
        aggregateID: aggregateID,
        handler:     handler,
        config:      config,
        metrics:     metrics,
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
    a.lastEventTime = time.Now()
    a.metrics.ActiveActors.Add(1)
    
    // 启动 Passivation 定时器
    if a.config.PassivationTimeout > 0 {
        a.startPassivationTimer(ctx)
    }
}

// onStopped Actor 停止
func (a *AggregateActor) onStopped(ctx *actor.Context) {
    a.metrics.ActiveActors.Add(-1)
    a.metrics.ActorsPassivated.Add(1)
    
    // 停止 Passivation 定时器
    if a.passivationTimer != nil {
        a.passivationTimer.Stop()
    }
}

// onRestarting Actor 重启
func (a *AggregateActor) onRestarting(ctx *actor.Context) {
    a.metrics.ActorsRestarted.Add(1)
}

// handleDomainEvent 处理领域事件
func (a *AggregateActor) handleDomainEvent(ctx *actor.Context, msg *DomainEventMessage) {
    startTime := time.Now()
    
    // 更新活跃时间
    a.lastEventTime = time.Now()
    a.eventCount++
    
    // 重置 Passivation 定时器
    if a.passivationTimer != nil {
        a.passivationTimer.Reset(a.config.PassivationTimeout)
    }
    
    // 调用业务处理器
    err := a.handler(msg.Context, msg.EventData)
    
    // 记录延迟
    latency := time.Since(startTime)
    a.metrics.RecordLatency(latency)
    
    // 更新指标
    if err != nil {
        a.metrics.MessagesFailed.Add(1)
    } else {
        a.metrics.MessagesProcessed.Add(1)
    }
    
    // 返回处理结果
    if msg.Done != nil {
        select {
        case msg.Done <- err:
        default:
        }
    }
    
    // 如果处理失败,触发 panic (让 Supervisor 处理)
    if err != nil {
        panic(fmt.Errorf("failed to process event: %w", err))
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
        // 停止 Actor (Passivation)
        ctx.Engine().Poison(ctx.PID())
    } else {
        // 重新启动定时器
        a.startPassivationTimer(ctx)
    }
}
```

---

## 5️⃣ **hollywood_engine.go** (核心)

```go
package eventbus

import (
    "context"
    "fmt"
    "sync"
    "time"
    
    "github.com/anthdm/hollywood/actor"
)

// HollywoodEngine Hollywood Actor 引擎封装
type HollywoodEngine struct {
    engine        *actor.Engine
    actorRegistry *ActorRegistry
    config        *HollywoodConfig
    metrics       *HollywoodMetrics
    
    // 生命周期
    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
}

// NewHollywoodEngine 创建 Hollywood 引擎
func NewHollywoodEngine(config *HollywoodConfig) (*HollywoodEngine, error) {
    if config == nil {
        config = DefaultHollywoodConfig()
    }
    
    if err := config.Validate(); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }
    
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
        return nil, fmt.Errorf("failed to create engine: %w", err)
    }
    
    he := &HollywoodEngine{
        engine:        engine,
        actorRegistry: NewActorRegistry(),
        config:        config,
        metrics:       NewHollywoodMetrics(),
        ctx:           ctx,
        cancel:        cancel,
    }
    
    // 订阅事件流 (监控)
    if config.EnableEventStream {
        he.subscribeEventStream()
    }
    
    // 启动指标收集
    if config.MetricsInterval > 0 {
        he.startMetricsCollection()
    }
    
    return he, nil
}

// ProcessMessage 处理领域事件消息
func (he *HollywoodEngine) ProcessMessage(ctx context.Context, msg *AggregateMessage) error {
    if msg.AggregateID == "" {
        return fmt.Errorf("aggregateID is required")
    }
    
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
    
    he.metrics.MessagesSent.Add(1)
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
        return NewAggregateActor(aggregateID, handler, he.config, he.metrics)
    })
    
    // 配置 Actor 选项
    props = props.
        WithMaxRestarts(he.config.MaxRestarts).
        WithInboxSize(he.config.InboxSize)
    
    // Spawn Actor
    pid := he.engine.Spawn(props, aggregateID)
    
    // 注册到注册表
    he.actorRegistry.Register(aggregateID, pid)
    
    he.metrics.ActorsCreated.Add(1)
    return pid
}

// subscribeEventStream 订阅事件流
func (he *HollywoodEngine) subscribeEventStream() {
    // 订阅 DeadLetter 事件
    he.engine.Subscribe(actor.DeadLetterEvent{}, he.engine.SpawnFunc(
        func(ctx *actor.Context) {
            if msg, ok := ctx.Message().(actor.DeadLetterEvent); ok {
                fmt.Printf("[Hollywood] DeadLetter: %+v\n", msg)
            }
        },
        "deadletter-monitor",
    ))
    
    // 订阅 ActorRestarted 事件
    he.engine.Subscribe(actor.ActorRestartedEvent{}, he.engine.SpawnFunc(
        func(ctx *actor.Context) {
            if msg, ok := ctx.Message().(actor.ActorRestartedEvent); ok {
                fmt.Printf("[Hollywood] ActorRestarted: %+v\n", msg)
            }
        },
        "restart-monitor",
    ))
}

// startMetricsCollection 启动指标收集
func (he *HollywoodEngine) startMetricsCollection() {
    he.wg.Add(1)
    go func() {
        defer he.wg.Done()
        
        ticker := time.NewTicker(he.config.MetricsInterval)
        defer ticker.Stop()
        
        for {
            select {
            case <-ticker.C:
                metrics := he.metrics.Report()
                fmt.Printf("[Hollywood Metrics] %+v\n", metrics)
                
            case <-he.ctx.Done():
                return
            }
        }
    }()
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

// GetMetrics 获取监控指标
func (he *HollywoodEngine) GetMetrics() map[string]interface{} {
    return he.metrics.Report()
}

// extractEventType 从 Headers 提取事件类型
func extractEventType(headers map[string][]byte) string {
    if eventType, ok := headers["event_type"]; ok {
        return string(eventType)
    }
    return "unknown"
}
```

---

## 📝 **使用说明**

### 1. 创建引擎

```go
config := &HollywoodConfig{
    MaxRestarts:        3,
    InboxSize:          1000,
    PassivationTimeout: 5 * time.Minute,
}

engine, err := NewHollywoodEngine(config)
if err != nil {
    panic(err)
}
defer engine.Stop()
```

### 2. 处理消息

```go
handler := func(ctx context.Context, data []byte) error {
    // 业务逻辑
    return nil
}

msg := &AggregateMessage{
    AggregateID: "order-123",
    Value:       []byte("event-data"),
    Handler:     handler,
}

err := engine.ProcessMessage(context.Background(), msg)
```

### 3. 获取指标

```go
metrics := engine.GetMetrics()
fmt.Printf("Metrics: %+v\n", metrics)
```

---

## ✅ **下一步**

1. 将以上代码保存到对应文件
2. 运行单元测试验证功能
3. 集成到 EventBus (kafka.go, nats.go)
4. 编写集成测试
5. 性能测试和调优

