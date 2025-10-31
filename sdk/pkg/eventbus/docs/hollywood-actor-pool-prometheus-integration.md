# Hollywood Actor Pool - Prometheus 监控集成方案 (混合方案)

## 📋 **概述**

本文档说明如何为 Hollywood Actor Pool 集成 Prometheus 监控,采用 **混合方案**:
- **接口注入** (与 EventBus 一致,依赖倒置)
- **Hollywood Middleware** (自动拦截,减少手动调用)

---

## 🎯 **监控方案选择**

### 方案对比

| 方案 | 优势 | 劣势 | 推荐度 |
|------|------|------|--------|
| **纯接口注入** | ✅ 与 EventBus 一致<br>✅ 依赖倒置<br>✅ 易于测试 | ⚠️ 需要手动调用接口<br>⚠️ 容易遗漏 | ⭐⭐⭐ |
| **纯 Middleware** | ✅ 框架原生<br>✅ 自动拦截 | ⚠️ 与 EventBus 不一致<br>⚠️ 依赖 Hollywood | ⭐⭐⭐ |
| **混合方案** ⭐ | ✅ 与 EventBus 一致<br>✅ 依赖倒置<br>✅ 自动拦截<br>✅ 灵活性高 | 需要理解两种机制 | ⭐⭐⭐⭐⭐ |

### 推荐方案: 混合方案 ⭐⭐⭐⭐⭐

**核心思想**:
- **接口定义**: 使用接口注入 (与 EventBus 一致,依赖倒置)
- **Middleware 实现**: Middleware 依赖接口,自动记录消息处理
- **手动调用**: 特殊场景手动调用接口 (Middleware 无法处理的场景)

**优势**:
1. ✅ 与 EventBus 监控架构保持一致 (接口注入)
2. ✅ 核心代码不依赖 Prometheus (依赖倒置原则)
3. ✅ 自动记录消息处理 (Middleware 自动拦截)
4. ✅ 灵活性高 (特殊场景可手动调用)
5. ✅ 易于单元测试 (可注入 NoOp 或 InMemory 实现)
6. ✅ 支持多种监控系统 (Prometheus, StatsD, Datadog, etc.)

---

## 🏗️ **混合方案架构**

### 架构图

```
┌─────────────────────────────────────────────────────────────┐
│  HollywoodActorPool                                         │
│                                                              │
│  metricsCollector: ActorPoolMetricsCollector (接口注入) ⭐  │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Actor 0                                               │ │
│  │  ┌──────────────────────────────────────────────────┐ │ │
│  │  │  ActorMetricsMiddleware (依赖接口)              │ │ │
│  │  │  - 自动拦截消息处理                             │ │ │
│  │  │  - 记录延迟、成功率                             │ │ │
│  │  └──────────────────────────────────────────────────┘ │ │
│  │           ↓                                            │ │
│  │  ┌──────────────────────────────────────────────────┐ │ │
│  │  │  PoolActor (持有接口引用)                       │ │ │
│  │  │  - 手动记录 Inbox 深度                          │ │ │
│  │  └──────────────────────────────────────────────────┘ │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  ProcessMessage() - 手动记录消息发送 ⭐                     │
└─────────────────────────────────────────────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  ActorPoolMetrics     │
        │  Collector (接口)     │
        └───────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Prometheus           │
        │  Implementation       │
        └───────────────────────┘
```

### 职责划分

| 组件 | 职责 | 实现方式 |
|------|------|---------|
| **ActorPoolMetricsCollector** | 定义监控接口 | 接口定义 |
| **PrometheusActorPoolMetricsCollector** | Prometheus 实现 | 实现接口 |
| **ActorMetricsMiddleware** | 自动记录消息处理 | Hollywood Middleware (依赖接口) |
| **HollywoodActorPool** | 手动记录消息发送 | 手动调用接口 |
| **PoolActor** | 手动记录 Inbox 深度 | 手动调用接口 |
| **EventStream 订阅** | 记录 Actor 重启、死信 | 手动调用接口 |

---

## 💻 **完整实现**

### 1. 定义监控接口 (与 EventBus 一致)

参考 EventBus 的 `MetricsCollector` 接口设计:

```go
// actor_pool_metrics_collector.go
package eventbus

import "time"

// ActorPoolMetricsCollector Actor Pool 监控指标收集器接口
//
// 设计原则:
// - 依赖倒置: Actor Pool 依赖接口而非具体实现
// - 零依赖: 核心代码不依赖外部监控库 (Prometheus, StatsD, etc.)
// - 可扩展: 支持多种监控系统实现
//
// 参考: EventBus MetricsCollector 接口
type ActorPoolMetricsCollector interface {
    // ========== 消息指标 ==========
    
    // RecordMessageSent 记录消息发送
    RecordMessageSent(actorID string)
    
    // RecordMessageProcessed 记录消息处理
    // success: 是否成功
    // duration: 处理耗时
    RecordMessageProcessed(actorID string, success bool, duration time.Duration)
    
    // ========== Actor 状态指标 ==========
    
    // RecordActorRestarted 记录 Actor 重启
    RecordActorRestarted(actorID string)
    
    // RecordDeadLetter 记录死信
    RecordDeadLetter(actorID string)
    
    // ========== Mailbox 指标 ==========
    
    // RecordInboxDepth 记录 Inbox 深度
    // depth: 当前深度
    // capacity: 容量 (inboxSize)
    RecordInboxDepth(actorID string, depth int, capacity int)
    
    // RecordInboxFull 记录 Inbox 满载
    RecordInboxFull(actorID string)
}
```

---

### 2. NoOp 实现 (默认,零开销)

```go
// NoOpActorPoolMetricsCollector 空操作实现 (默认)
// 用于不需要监控的场景,零性能开销
type NoOpActorPoolMetricsCollector struct{}

func (n *NoOpActorPoolMetricsCollector) RecordMessageSent(actorID string) {}
func (n *NoOpActorPoolMetricsCollector) RecordMessageProcessed(actorID string, success bool, duration time.Duration) {}
func (n *NoOpActorPoolMetricsCollector) RecordActorRestarted(actorID string) {}
func (n *NoOpActorPoolMetricsCollector) RecordDeadLetter(actorID string) {}
func (n *NoOpActorPoolMetricsCollector) RecordInboxDepth(actorID string, depth int, capacity int) {}
func (n *NoOpActorPoolMetricsCollector) RecordInboxFull(actorID string) {}
```

---

### 3. Prometheus 实现 (生产环境)

```go
// actor_pool_metrics_prometheus.go
package eventbus

import (
    "time"
    
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusActorPoolMetricsCollector Prometheus 监控实现
//
// 使用示例:
//
//  // 1. 创建 Prometheus 收集器
//  collector := eventbus.NewPrometheusActorPoolMetricsCollector("my_service")
//
//  // 2. 配置 Actor Pool
//  config := &HollywoodActorPoolConfig{
//      PoolSize:  256,
//      InboxSize: 1000,
//      MetricsCollector: collector, // 注入 Prometheus 收集器
//  }
//
//  pool, err := NewHollywoodActorPool(config)
//
//  // 3. 启动 Prometheus HTTP 服务器
//  http.Handle("/metrics", promhttp.Handler())
//  go http.ListenAndServe(":2112", nil)
type PrometheusActorPoolMetricsCollector struct {
    namespace string
    
    // 消息指标
    messagesSent      *prometheus.CounterVec
    messagesProcessed *prometheus.CounterVec
    messagesFailed    *prometheus.CounterVec
    messageLatency    *prometheus.HistogramVec
    
    // Actor 状态指标
    actorsRestarted *prometheus.CounterVec
    deadLetters     *prometheus.CounterVec
    
    // Mailbox 指标
    inboxDepth       *prometheus.GaugeVec
    inboxUtilization *prometheus.GaugeVec
    inboxFull        *prometheus.CounterVec
}

// NewPrometheusActorPoolMetricsCollector 创建 Prometheus 收集器
//
// 参数:
//   - namespace: 指标命名空间 (例如: "my_service")
//
// 返回:
//   - *PrometheusActorPoolMetricsCollector: Prometheus 收集器实例
func NewPrometheusActorPoolMetricsCollector(namespace string) *PrometheusActorPoolMetricsCollector {
    if namespace == "" {
        namespace = "hollywood"
    }
    
    return &PrometheusActorPoolMetricsCollector{
        namespace: namespace,
        
        // 消息指标
        messagesSent: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "actor_pool_messages_sent_total",
                Help:      "Total messages sent to actors",
            },
            []string{"actor_id"},
        ),
        messagesProcessed: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "actor_pool_messages_processed_total",
                Help:      "Total messages processed by actors",
            },
            []string{"actor_id"},
        ),
        messagesFailed: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "actor_pool_messages_failed_total",
                Help:      "Total failed messages",
            },
            []string{"actor_id"},
        ),
        messageLatency: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Name:      "actor_pool_message_latency_seconds",
                Help:      "Message processing latency in seconds",
                Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
            },
            []string{"actor_id"},
        ),
        
        // Actor 状态指标
        actorsRestarted: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "actor_pool_actors_restarted_total",
                Help:      "Total actor restarts",
            },
            []string{"actor_id"},
        ),
        deadLetters: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "actor_pool_dead_letters_total",
                Help:      "Total dead letters",
            },
            []string{"actor_id"},
        ),
        
        // Mailbox 指标
        inboxDepth: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Namespace: namespace,
                Name:      "actor_pool_inbox_depth",
                Help:      "Current inbox depth",
            },
            []string{"actor_id"},
        ),
        inboxUtilization: promauto.NewGaugeVec(
            prometheus.GaugeOpts{
                Namespace: namespace,
                Name:      "actor_pool_inbox_utilization",
                Help:      "Inbox utilization ratio (0-1)",
            },
            []string{"actor_id"},
        ),
        inboxFull: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "actor_pool_inbox_full_total",
                Help:      "Total times inbox was full",
            },
            []string{"actor_id"},
        ),
    }
}

// RecordMessageSent 记录消息发送
func (p *PrometheusActorPoolMetricsCollector) RecordMessageSent(actorID string) {
    p.messagesSent.WithLabelValues(actorID).Inc()
}

// RecordMessageProcessed 记录消息处理
func (p *PrometheusActorPoolMetricsCollector) RecordMessageProcessed(actorID string, success bool, duration time.Duration) {
    p.messagesProcessed.WithLabelValues(actorID).Inc()
    if !success {
        p.messagesFailed.WithLabelValues(actorID).Inc()
    }
    p.messageLatency.WithLabelValues(actorID).Observe(duration.Seconds())
}

// RecordActorRestarted 记录 Actor 重启
func (p *PrometheusActorPoolMetricsCollector) RecordActorRestarted(actorID string) {
    p.actorsRestarted.WithLabelValues(actorID).Inc()
}

// RecordDeadLetter 记录死信
func (p *PrometheusActorPoolMetricsCollector) RecordDeadLetter(actorID string) {
    p.deadLetters.WithLabelValues(actorID).Inc()
}

// RecordInboxDepth 记录 Inbox 深度
//
// ⚠️ 注意: depth 为近似值
// - 计数器在消息发送时 +1，接收时 -1
// - 存在竞态条件，可能与实际队列深度有偏差
// - 仅用于趋势观测和容量规划，不保证精确
func (p *PrometheusActorPoolMetricsCollector) RecordInboxDepth(actorID string, depth int, capacity int) {
    p.inboxDepth.WithLabelValues(actorID).Set(float64(depth))

    utilization := float64(depth) / float64(capacity)
    p.inboxUtilization.WithLabelValues(actorID).Set(utilization)

    if depth >= capacity {
        p.inboxFull.WithLabelValues(actorID).Inc()
    }
}

// RecordInboxFull 记录 Inbox 满载
func (p *PrometheusActorPoolMetricsCollector) RecordInboxFull(actorID string) {
    p.inboxFull.WithLabelValues(actorID).Inc()
}
```

---

### 4. 创建 Middleware (依赖接口) ⭐ 核心

```go
// actor_metrics_middleware.go
package eventbus

import (
    "time"

    "github.com/anthdm/hollywood/actor"
)

// ActorMetricsMiddleware Actor 监控 Middleware
//
// 核心设计:
// - 依赖接口 (ActorPoolMetricsCollector),不直接依赖 Prometheus
// - 自动拦截消息处理,记录延迟和成功率
// - 与 EventBus 监控架构保持一致
type ActorMetricsMiddleware struct {
    actorID   string
    collector ActorPoolMetricsCollector // ⭐ 依赖接口,不依赖 Prometheus
}

// NewActorMetricsMiddleware 创建 Actor 监控 Middleware
func NewActorMetricsMiddleware(actorID string, collector ActorPoolMetricsCollector) *ActorMetricsMiddleware {
    return &ActorMetricsMiddleware{
        actorID:   actorID,
        collector: collector,
    }
}

// WithMetrics 返回 Middleware 函数 (Hollywood 标准 Middleware 接口)
func (amm *ActorMetricsMiddleware) WithMetrics() func(actor.ReceiveFunc) actor.ReceiveFunc {
    return func(next actor.ReceiveFunc) actor.ReceiveFunc {
        return func(c *actor.Context) {
            // 只拦截业务消息,忽略系统消息 (Started, Stopped, etc.)
            if _, ok := c.Message().(*DomainEventMessage); !ok {
                next(c)
                return
            }

            start := time.Now()
            success := true

            // ⭐ 捕获 panic,记录失败指标
            defer func() {
                if r := recover(); r != nil {
                    success = false
                    duration := time.Since(start)
                    amm.collector.RecordMessageProcessed(amm.actorID, success, duration)
                    panic(r) // 重新抛出,让 Supervisor 处理
                }
            }()

            // 调用下一个 Middleware 或业务逻辑
            next(c)

            // 记录消息处理指标 (通过接口)
            duration := time.Since(start)
            amm.collector.RecordMessageProcessed(amm.actorID, success, duration) // ⭐ 使用接口
        }
    }
}
```

---

### 5. 集成到 Actor Pool (混合方案)

```go
// hollywood_actor_pool.go
package eventbus

import (
    "context"
    "fmt"

    "github.com/anthdm/hollywood/actor"
    "go.uber.org/atomic"
)

type HollywoodActorPoolConfig struct {
    PoolSize    int
    InboxSize   int
    MaxRestarts int

    EnableEventStream bool

    // MetricsCollector 指标收集器 (可选)
    // 如果不设置,将使用 NoOpActorPoolMetricsCollector (零开销)
    MetricsCollector ActorPoolMetricsCollector `mapstructure:"-"`
}

type HollywoodActorPool struct {
    config             *HollywoodActorPoolConfig
    engine             *actor.Engine
    actors             []*actor.PID
    metricsCollector   ActorPoolMetricsCollector // ⭐ 接口注入

    // Inbox 深度近似计数器 (与每个 Actor 对应)
    inboxDepthCounters []atomic.Int32
}

func NewHollywoodActorPool(config *HollywoodActorPoolConfig) (*HollywoodActorPool, error) {
    pool := &HollywoodActorPool{
        config: config,
        engine: actor.NewEngine(),
        actors: make([]*actor.PID, config.PoolSize),
    }

    // 初始化监控收集器
    if config.MetricsCollector != nil {
        pool.metricsCollector = config.MetricsCollector
    } else {
        pool.metricsCollector = &NoOpActorPoolMetricsCollector{}
    }

    // 初始化 Actors (注入 Middleware)
    pool.initActors()

    // 初始化事件流监听 (记录 Actor 重启、死信)
    pool.initEventStream()

    return pool, nil
}

// initActors 初始化 Actors (注入 Middleware)
func (p *HollywoodActorPool) initActors() error {
    // 初始化 Inbox 深度计数器切片 (与 PoolSize 对应)
    if len(p.inboxDepthCounters) == 0 {
        p.inboxDepthCounters = make([]atomic.Int32, p.config.PoolSize)
    }

    for i := 0; i < p.config.PoolSize; i++ {
        actorID := fmt.Sprintf("pool-actor-%d", i)

        // 创建 Middleware (使用注入的 collector) ⭐
        middleware := NewActorMetricsMiddleware(actorID, p.metricsCollector)

        // Spawn Actor 并注入 Middleware
        pid := p.engine.Spawn(
            func() actor.Receiver {
                // 传入与该 Actor 对应的深度计数器指针
                return NewPoolActor(i, p.config.InboxSize, p.metricsCollector, &p.inboxDepthCounters[i])
            },
            actorID,
            actor.WithInboxSize(p.config.InboxSize),
            actor.WithMaxRestarts(p.config.MaxRestarts),
            actor.WithMiddleware(middleware.WithMetrics()), // ⭐ 注入 Middleware
        )

        p.actors[i] = pid
    }
    return nil
}

// ProcessMessage 处理消息 (手动记录消息发送)
func (p *HollywoodActorPool) ProcessMessage(ctx context.Context, msg *AggregateMessage) error {
    actorIndex := p.hashToIndex(msg.AggregateID)
    actorID := fmt.Sprintf("pool-actor-%d", actorIndex)

    // ⭐ 手动记录消息发送 (Middleware 无法拦截这里)
    p.metricsCollector.RecordMessageSent(actorID)

    // ⭐ 增加 Inbox 深度计数器 (近似值)
    // 注意: 这是近似值，实际入队与计数器增加之间存在时间窗口
    // 如果 Inbox 满载，消息可能被丢弃，但计数器已 +1
    p.inboxDepthCounters[actorIndex].Add(1)

    pid := p.actors[actorIndex]
    p.engine.Send(pid, &DomainEventMessage{
        AggregateID: msg.AggregateID,
        Value:       msg.Value,
        Handler:     msg.Handler,
        Context:     ctx,
    })

    return nil
}

// initEventStream 初始化事件流监听 (记录 Actor 重启、死信)
func (p *HollywoodActorPool) initEventStream() {
    if !p.config.EnableEventStream {
        return
    }

    // ⭐ 订阅 Actor 重启事件 (手动记录)
    p.engine.Subscribe(func(event *actor.ActorRestartedEvent) {
        actorID := event.PID.GetID()
        p.metricsCollector.RecordActorRestarted(actorID)
    })

    // ⭐ 订阅死信事件 (手动记录)
    p.engine.Subscribe(func(event *actor.DeadLetterEvent) {
        actorID := event.Target.GetID()
        p.metricsCollector.RecordDeadLetter(actorID)
    })
}
```

---

### 6. 在 PoolActor 中记录 Inbox 深度 (手动)

```go
// pool_actor.go
package eventbus

import (
    "fmt"

    "github.com/anthdm/hollywood/actor"
    "go.uber.org/atomic"
)

type PoolActor struct {
    index            int
    actorID          string
    metricsCollector ActorPoolMetricsCollector // ⭐ 持有接口引用
    inboxSize        int

    // Inbox 深度计数器 (与 Pool 持有的对应计数器共享指针)
    inboxDepth *atomic.Int32
}

func NewPoolActor(index int, inboxSize int, metricsCollector ActorPoolMetricsCollector, depthCounter *atomic.Int32) actor.Receiver {
    return &PoolActor{
        index:            index,
        actorID:          fmt.Sprintf("pool-actor-%d", index),
        metricsCollector: metricsCollector,
        inboxSize:        inboxSize,
        inboxDepth:       depthCounter,
    }
}

func (pa *PoolActor) Receive(ctx *actor.Context) {
    switch msg := ctx.Message().(type) {
    case actor.Started:
        // Actor 启动

    case actor.Stopped:
        // Actor 停止

    case *DomainEventMessage:
        // ⭐ 每次收到消息,减少计数器 (近似值)
        // 注意: 这是近似值，与实际 Inbox 深度可能有偏差
        pa.inboxDepth.Add(-1)

        // ✅ 消息处理由 Middleware 自动记录 (延迟、成功率)
        // 这里只需要处理业务逻辑
        err := msg.Handler(msg.Context, msg.Value)

        // ⭐ 手动记录 Inbox 深度 (Middleware 无法获取)
        // ⚠️ 重要说明:
        // - 这是一个近似值，基于原子计数器 (+1 发送时, -1 接收时)
        // - 实际深度由 Hollywood 内部管理，此值仅用于趋势观测
        // - 存在竞态条件，可能与真实队列深度有偏差
        // - 不应用于精确的容量判断或限流决策
        depth := int(pa.inboxDepth.Load())
        if depth < 0 {
            depth = 0 // 防止负数 (竞态条件可能导致)
        }
        pa.metricsCollector.RecordInboxDepth(pa.actorID, depth, pa.inboxSize)

        if err != nil {
            // 错误处理
        }
    }
}
```

---

---

## 📊 **使用示例**

### 完整示例代码 (混合方案)

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/prometheus/client_golang/prometheus/promhttp"
    "your-project/eventbus"
)

func main() {
    // ========== 1. 创建 Prometheus 指标收集器 (接口实现) ==========

    metricsCollector := eventbus.NewPrometheusActorPoolMetricsCollector("my_service")

    fmt.Println("✅ Prometheus 指标收集器已创建")

    // ========== 2. 配置 Actor Pool (注入接口) ==========

    config := &eventbus.HollywoodActorPoolConfig{
        PoolSize:          256,
        InboxSize:         1000,
        MaxRestarts:       3,
        EnableEventStream: true,
        MetricsCollector:  metricsCollector, // ⭐ 注入接口 (与 EventBus 一致)
    }

    // ========== 3. 创建 Actor Pool ==========
    // Pool 内部会:
    // - 为每个 Actor 创建 Middleware (依赖注入的接口)
    // - Middleware 自动拦截消息处理
    // - 订阅事件流 (Actor 重启、死信)

    pool, err := eventbus.NewHollywoodActorPool(config)
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    fmt.Println("✅ Actor Pool 已创建 (256 actors, Middleware 已注入)")

    // ========== 4. 启动 Prometheus HTTP 服务器 ==========

    go func() {
        http.Handle("/metrics", promhttp.Handler())
        fmt.Println("✅ Prometheus 服务器启动: http://localhost:2112/metrics")
        http.ListenAndServe(":2112", nil)
    }()

    // ========== 5. 使用 Actor Pool ==========
    // 指标记录:
    // - 消息发送: 手动记录 (ProcessMessage 中)
    // - 消息处理: Middleware 自动记录 ✅
    // - Inbox 深度: PoolActor 手动记录
    // - Actor 重启/死信: EventStream 订阅自动记录 ✅

    ctx := context.Background()

    // 发送消息
    for i := 0; i < 1000; i++ {
        aggregateID := fmt.Sprintf("order-%d", i%100)

        err := pool.ProcessMessage(ctx, &eventbus.AggregateMessage{
            AggregateID: aggregateID,
            Value:       []byte(`{"event": "order_created"}`),
            Handler: func(ctx context.Context, data []byte) error {
                // 处理业务逻辑
                time.Sleep(10 * time.Millisecond)
                return nil
            },
        })

        if err != nil {
            fmt.Printf("❌ 发送失败: %v\n", err)
        }
    }

    fmt.Println("✅ 已发送 1000 条消息")
    fmt.Println("✅ Middleware 已自动记录所有消息处理指标")

    // ========== 6. 访问 Prometheus 指标 ==========

    fmt.Println("\n访问 http://localhost:2112/metrics 查看指标")
    fmt.Println("按 Ctrl+C 退出")

    select {}
}
```

### 指标记录方式说明

| 指标 | 记录方式 | 位置 |
|------|---------|------|
| **消息发送** | 手动调用接口 | `HollywoodActorPool.ProcessMessage()` |
| **消息处理 (延迟、成功率)** | ✅ Middleware 自动拦截 | `ActorMetricsMiddleware` |
| **Inbox 深度** | 手动调用接口 | `PoolActor.Receive()` |
| **Actor 重启** | ✅ EventStream 自动订阅 | `HollywoodActorPool.initEventStream()` |
| **死信** | ✅ EventStream 自动订阅 | `HollywoodActorPool.initEventStream()` |

### Prometheus 指标输出示例

访问 `http://localhost:2112/metrics`:

```
# HELP my_service_actor_pool_messages_sent_total Total messages sent to actors
# TYPE my_service_actor_pool_messages_sent_total counter
my_service_actor_pool_messages_sent_total{actor_id="pool-actor-0"} 50
my_service_actor_pool_messages_sent_total{actor_id="pool-actor-1"} 48

# HELP my_service_actor_pool_message_latency_seconds Message processing latency in seconds
# TYPE my_service_actor_pool_message_latency_seconds histogram
my_service_actor_pool_message_latency_seconds_bucket{actor_id="pool-actor-0",le="0.01"} 50
my_service_actor_pool_message_latency_seconds_sum{actor_id="pool-actor-0"} 0.5

# HELP my_service_actor_pool_inbox_depth Current inbox depth (approximate)
# TYPE my_service_actor_pool_inbox_depth gauge
# ⚠️ 注意: 此值为近似值，基于原子计数器，仅用于趋势观测
my_service_actor_pool_inbox_depth{actor_id="pool-actor-0"} 0

# HELP my_service_actor_pool_inbox_utilization Inbox utilization ratio (0-1, approximate)
# TYPE my_service_actor_pool_inbox_utilization gauge
# ⚠️ 注意: 基于近似深度计算，仅用于容量规划参考
my_service_actor_pool_inbox_utilization{actor_id="pool-actor-0"} 0.0
```

---

## 📈 **Grafana 集成**

### Grafana 查询示例

```promql
# 1. 消息吞吐量 (每秒)
rate(my_service_actor_pool_messages_processed_total[1m])

# 2. P99 延迟
histogram_quantile(0.99, rate(my_service_actor_pool_message_latency_seconds_bucket[5m]))

# 3. 平均 Inbox 深度
avg(my_service_actor_pool_inbox_depth)

# 4. Actor 重启频率
rate(my_service_actor_pool_actors_restarted_total[1m])
```

---

## 🔍 **与 EventBus 监控对比**

| 特性 | EventBus | Actor Pool | 一致性 |
|------|----------|-----------|--------|
| **接口设计** | `MetricsCollector` | `ActorPoolMetricsCollector` | ✅ 相同模式 |
| **Prometheus 实现** | `PrometheusMetricsCollector` | `PrometheusActorPoolMetricsCollector` | ✅ 相同模式 |
| **注入方式** | `config.MetricsCollector` | `config.MetricsCollector` | ✅ 相同方式 |

**结论**: 使用方式完全一致! ✅

---

## ✅ **总结**

### 混合方案核心优势

1. ✅ **与 EventBus 一致**: 相同的接口注入方式
2. ✅ **依赖倒置**: Middleware 依赖接口,核心代码不依赖 Prometheus
3. ✅ **自动化**: Middleware 自动拦截消息处理,减少手动调用
4. ✅ **灵活性**: 特殊场景可手动调用接口 (Inbox 深度、消息发送)
5. ✅ **易于测试**: 可注入 NoOp 或 InMemory 实现
6. ✅ **支持多种监控系统**: Prometheus, StatsD, Datadog, etc.
7. ✅ **完整监控**: 消息、Actor 状态、Mailbox 深度
8. ✅ **Hollywood 原生**: 利用 Hollywood 的 Middleware 机制

### 混合方案架构总结

```
接口定义 (ActorPoolMetricsCollector)
    ↓
    ├─→ Prometheus 实现 (PrometheusActorPoolMetricsCollector)
    ├─→ NoOp 实现 (NoOpActorPoolMetricsCollector)
    └─→ 其他实现 (StatsD, Datadog, etc.)

注入方式:
    ├─→ HollywoodActorPool.metricsCollector (接口注入)
    ├─→ ActorMetricsMiddleware (依赖接口,自动拦截)
    └─→ PoolActor.metricsCollector (持有接口引用)

记录方式:
    ├─→ 自动: Middleware 拦截消息处理 ✅
    ├─→ 自动: EventStream 订阅 Actor 重启/死信 ✅
    └─→ 手动: 消息发送、Inbox 深度
```

### 与纯接口注入/纯 Middleware 对比

| 特性 | 纯接口注入 | 纯 Middleware | 混合方案 ⭐ |
|------|-----------|--------------|------------|
| **与 EventBus 一致** | ✅ | ❌ | ✅ |
| **依赖倒置** | ✅ | ⚠️ | ✅ |
| **自动拦截** | ❌ | ✅ | ✅ |
| **灵活性** | ✅ | ⚠️ | ✅ |
| **易于测试** | ✅ | ⚠️ | ✅ |
| **代码侵入性** | ⚠️ 高 | ✅ 低 | ✅ 低 |
| **Hollywood 原生** | ❌ | ✅ | ✅ |

---


