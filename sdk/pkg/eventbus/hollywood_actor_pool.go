package eventbus

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/anthdm/hollywood/actor"
	"go.uber.org/atomic"
)

// HollywoodActorPoolConfig configuration for Hollywood Actor Pool
type HollywoodActorPoolConfig struct {
	PoolSize    int // number of actors (e.g., 256/512/1024)
	InboxSize   int // per-actor inbox capacity
	MaxRestarts int // max restarts for supervisor
}

// HollywoodActorPool implements a fixed-size actor pool using Hollywood framework
// - Same aggregateID is routed to the same actor via consistent hashing
// - Each actor processes messages sequentially to guarantee per-aggregate ordering
// - Supervisor mechanism: actor panic auto-restart
// - Event stream monitoring: DeadLetter, ActorRestarted events
type HollywoodActorPool struct {
	config           *HollywoodActorPoolConfig
	engine           *actor.Engine
	actors           []*actor.PID
	metricsCollector ActorPoolMetricsCollector // ⭐ 接口注入

	// Inbox 深度近似计数器 (与每个 Actor 对应)
	inboxDepthCounters []atomic.Int32
}

// NewHollywoodActorPool creates a new Hollywood Actor Pool
func NewHollywoodActorPool(config HollywoodActorPoolConfig, metricsCollector ActorPoolMetricsCollector) *HollywoodActorPool {
	// 默认配置
	if config.PoolSize <= 0 {
		config.PoolSize = 256
	}
	if config.InboxSize <= 0 {
		config.InboxSize = 1000
	}
	if config.MaxRestarts <= 0 {
		config.MaxRestarts = 3
	}

	// 如果没有提供 metricsCollector，使用 NoOp 实现
	if metricsCollector == nil {
		metricsCollector = &NoOpActorPoolMetricsCollector{}
	}

	engine, err := actor.NewEngine(actor.NewEngineConfig())
	if err != nil {
		panic(fmt.Sprintf("failed to create Hollywood engine: %v", err))
	}

	pool := &HollywoodActorPool{
		config:             &config,
		engine:             engine,
		actors:             make([]*actor.PID, config.PoolSize),
		metricsCollector:   metricsCollector,
		inboxDepthCounters: make([]atomic.Int32, config.PoolSize),
	}

	// 初始化 actors
	if err := pool.initActors(); err != nil {
		panic(fmt.Errorf("failed to initialize actors: %w", err))
	}

	// 初始化事件流监控
	pool.initEventStream()

	return pool
}

// initActors initializes all actors in the pool
func (p *HollywoodActorPool) initActors() error {
	for i := 0; i < p.config.PoolSize; i++ {
		actorID := fmt.Sprintf("pool-actor-%d", i)
		middleware := NewActorMetricsMiddleware(actorID, p.metricsCollector)

		pid := p.engine.Spawn(
			func() actor.Receiver {
				// 传入与该 Actor 对应的深度计数器指针
				return NewPoolActor(i, p.config.InboxSize, p.metricsCollector, &p.inboxDepthCounters[i])
			},
			actorID,
			actor.WithInboxSize(p.config.InboxSize),
			actor.WithMaxRestarts(p.config.MaxRestarts),
			actor.WithMiddleware(middleware.WithMetrics()),
		)

		p.actors[i] = pid
	}
	return nil
}

// initEventStream initializes event stream monitoring
func (p *HollywoodActorPool) initEventStream() {
	// TODO: 实现事件流监控
	// Hollywood 的事件流 API 需要进一步研究
	// 暂时通过 Middleware 来捕获 panic 和错误
}

// ProcessMessage routes the message to its actor and enqueues it
func (p *HollywoodActorPool) ProcessMessage(ctx context.Context, msg *AggregateMessage) error {
	// Require AggregateID to route
	if msg.AggregateID == "" {
		return fmt.Errorf("aggregateID required for actor pool")
	}

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
		AggregateID:     msg.AggregateID,
		Value:           msg.Value,
		Context:         ctx,
		Handler:         msg.Handler,
		Done:            msg.Done,
		IsEnvelope:      msg.IsEnvelope,      // ⭐ 传递 Envelope 标记
		Raw:             msg.Raw,             // M15：透传原始 record 指纹
		DeliveryHandler: msg.DeliveryHandler, // M15：透传 delivery handler
	})

	return nil
}

// Stop stops all actors and the engine
func (p *HollywoodActorPool) Stop() {
	for _, pid := range p.actors {
		p.engine.Poison(pid)
	}

	// ⭐ 修复：注销 Prometheus 指标，避免重复注册错误
	if p.metricsCollector != nil {
		if collector, ok := p.metricsCollector.(*PrometheusActorPoolMetricsCollector); ok {
			collector.Unregister()
		}
	}
}

// hashToIndex hashes aggregateID to an actor index
func (p *HollywoodActorPool) hashToIndex(aggregateID string) int {
	h := fnv.New32a()
	h.Write([]byte(aggregateID))
	return int(h.Sum32()) % p.config.PoolSize
}

// DomainEventMessage is the message type sent to actors
type DomainEventMessage struct {
	AggregateID     string
	Value           []byte
	Context         context.Context
	Handler         MessageHandler
	Done            chan error
	IsEnvelope      bool                    // ⭐ 标记是否是 Envelope 消息（at-least-once 语义）
	Raw             RawMeta                 // M15：透传原始 record 指纹
	DeliveryHandler EnvelopeDeliveryHandler // M15：非 nil 时走 Delivery 路径
}

// PoolActor is the actor that processes messages
type PoolActor struct {
	index            int
	actorID          string
	metricsCollector ActorPoolMetricsCollector // ⭐ 持有接口引用
	inboxSize        int

	// Inbox 深度计数器 (与 Pool 持有的对应计数器共享指针)
	inboxDepth *atomic.Int32
}

// NewPoolActor creates a new PoolActor
func NewPoolActor(index int, inboxSize int, metricsCollector ActorPoolMetricsCollector, depthCounter *atomic.Int32) actor.Receiver {
	return &PoolActor{
		index:            index,
		actorID:          fmt.Sprintf("pool-actor-%d", index),
		metricsCollector: metricsCollector,
		inboxSize:        inboxSize,
		inboxDepth:       depthCounter,
	}
}

// dispatchMessage 是 Receive 里「Delivery vs plain」的分发逻辑，抽出便于单测。
// DeliveryHandler != nil → 走 invokeDelivery（携带 RawMeta）；否则走原 msg.Handler 路径。
func dispatchMessage(msg *DomainEventMessage) error {
	if msg.DeliveryHandler != nil {
		return invokeDelivery(msg.Context, msg.Value, msg.Raw, msg.DeliveryHandler)
	}
	// D3：Receive 跑在 actor goroutine 且无 recover，nil Handler 会 panic → supervisor
	// 重启 actor → 该聚合消息静默卡死。返回显式 error 让上层能诊断。
	if msg.Handler == nil {
		return fmt.Errorf("no handler for message: both Handler and DeliveryHandler are nil (aggregate=%s)", msg.AggregateID)
	}
	return msg.Handler(msg.Context, msg.Value)
}

// Receive handles incoming messages
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

		// 记录开始时间（用于计算延迟）
		startTime := time.Now()

		// 处理业务逻辑（M15：经 dispatchMessage 分发——Delivery 路径携带 RawMeta，否则走旧 Handler）
		err := dispatchMessage(msg)

		// 计算处理延迟
		duration := time.Since(startTime)

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

		// ⭐ 错误处理策略：
		// - 业务处理失败：记录错误但不 panic，通过 Done channel 返回错误
		// - 这样可以避免因无效消息（如 payload 为空）导致 Actor 频繁重启
		// - Supervisor 机制应该只用于处理严重的系统错误，而不是业务错误
		if err != nil {
			// 记录处理失败的指标
			pa.metricsCollector.RecordMessageProcessed(pa.actorID, false, duration)

			// 发送错误到 Done channel
			select {
			case msg.Done <- err:
			default:
			}
			// ⚠️ 不再 panic，而是正常返回
			// 这样可以避免因无效消息导致 Actor 重启
			return
		}

		// 记录处理成功的指标
		pa.metricsCollector.RecordMessageProcessed(pa.actorID, true, duration)

		// 成功处理：发送 nil 到 Done channel
		select {
		case msg.Done <- nil:
		default:
		}
	}
}

// ActorMetricsMiddleware is the middleware for recording metrics
type ActorMetricsMiddleware struct {
	actorID   string
	collector ActorPoolMetricsCollector
}

// NewActorMetricsMiddleware creates a new ActorMetricsMiddleware
func NewActorMetricsMiddleware(actorID string, collector ActorPoolMetricsCollector) *ActorMetricsMiddleware {
	return &ActorMetricsMiddleware{
		actorID:   actorID,
		collector: collector,
	}
}

// WithMetrics returns a middleware function that records metrics
func (amm *ActorMetricsMiddleware) WithMetrics() func(actor.ReceiveFunc) actor.ReceiveFunc {
	return func(next actor.ReceiveFunc) actor.ReceiveFunc {
		return func(c *actor.Context) {
			// 跳过系统消息
			switch c.Message().(type) {
			case *actor.Started, *actor.Stopped:
				next(c)
				return
			}

			// ⭐ 捕获 panic（根据消息类型决定处理策略）
			defer func() {
				if r := recover(); r != nil {
					// 检查是否是 DomainEventMessage
					if domainMsg, ok := c.Message().(*DomainEventMessage); ok {
						err := fmt.Errorf("handler panicked: %v", r)

						// ⭐ 关键修复：无论是否Envelope，都发送错误到Done channel
						// 这样可以避免Done channel死锁导致消息处理协程永久阻塞
						select {
						case domainMsg.Done <- err:
						default:
						}

						// 记录 panic 事件
						amm.collector.RecordActorRestarted(amm.actorID)

						if domainMsg.IsEnvelope {
							// ⭐ Envelope 消息：不重新panic，让消息重投递（at-least-once 语义）
							return
						} else {
							// ⭐ 普通消息：也不重新panic，避免Done channel死锁
							// 消息会被ACK并丢失（at-most-once语义）
							// 这是正确的行为：普通消息失败后应该被丢弃，而不是阻塞整个消息流
							return
						}
					}

					// ⭐ 非DomainEventMessage才重新panic（系统消息等）
					panic(r)
				}
			}()

			next(c)
			// 注意: 实际的成功/失败由 PoolActor.Receive 中的错误处理决定
		}
	}
}
