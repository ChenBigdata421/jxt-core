package eventbus

import (
	"context"
	"hash/fnv"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"
)

// UnifiedWorkItem 统一的工作项（支持Kafka和NATS）
type UnifiedWorkItem struct {
	// 通用字段
	Topic       string
	AggregateID string // 如果有聚合ID，则基于哈希路由；否则轮询分配
	Data        []byte
	Handler     MessageHandler
	Context     context.Context

	// Kafka专用字段
	KafkaMessage interface{} // *sarama.ConsumerMessage
	KafkaSession interface{} // sarama.ConsumerGroupSession

	// NATS专用字段
	NATSAckFunc func() error
	NATSBus     interface{} // *natsEventBus，用于更新统计
}

// Process 处理工作项
func (w *UnifiedWorkItem) Process() error {
	// 调用handler处理消息
	return w.Handler(w.Context, w.Data)
}

// UnifiedWorkerPool 统一的全局Keyed-Worker池
// 特点：
// 1. 有聚合ID的消息：基于聚合ID哈希到特定Worker（保证顺序）
// 2. 无聚合ID的消息：轮询分配到任意Worker（高并发）
// 3. 所有topic共享同一个Worker池（减少Goroutine数量）
type UnifiedWorkerPool struct {
	workers     []*UnifiedWorker
	workQueue   chan UnifiedWorkItem
	workerCount int
	queueSize   int
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	logger      *zap.Logger

	// 轮询分配的索引（用于无聚合ID的消息）
	roundRobinIndex int
	roundRobinMu    sync.Mutex
}

// UnifiedWorker 统一的Worker
type UnifiedWorker struct {
	id       int
	pool     *UnifiedWorkerPool
	workChan chan UnifiedWorkItem
	quit     chan bool
}

// NewUnifiedWorkerPool 创建统一的全局Keyed-Worker池
func NewUnifiedWorkerPool(workerCount int, logger *zap.Logger) *UnifiedWorkerPool {
	if workerCount <= 0 {
		// 🔥 优化：增加 Worker 数量（从 CPU×16 → CPU×32）
		// 提升高压下的处理速度，减少消息积压
		workerCount = runtime.NumCPU() * 32
	}

	// 🔥 优化：增加队列大小（从 worker×100 → worker×200）
	// 减少队列满的情况，避免背压
	queueSize := workerCount * 200

	ctx, cancel := context.WithCancel(context.Background())

	pool := &UnifiedWorkerPool{
		workers:         make([]*UnifiedWorker, workerCount),
		workQueue:       make(chan UnifiedWorkItem, queueSize),
		workerCount:     workerCount,
		queueSize:       queueSize,
		ctx:             ctx,
		cancel:          cancel,
		logger:          logger,
		roundRobinIndex: 0,
	}

	// 创建并启动workers
	for i := 0; i < workerCount; i++ {
		worker := &UnifiedWorker{
			id:       i,
			pool:     pool,
			workChan: make(chan UnifiedWorkItem, 50), // 🔥 优化：增加缓冲（从 10 → 50）
			quit:     make(chan bool),
		}
		pool.workers[i] = worker
		pool.wg.Add(1)
		go worker.start()
	}

	// 启动智能分发器
	go pool.smartDispatcher()

	logger.Info("Unified Worker Pool started",
		zap.Int("workerCount", workerCount),
		zap.Int("queueSize", queueSize))

	return pool
}

// smartDispatcher 智能分发器
// 根据是否有聚合ID，选择不同的分发策略
func (p *UnifiedWorkerPool) smartDispatcher() {
	for {
		select {
		case work := <-p.workQueue:
			if work.AggregateID != "" {
				// ✅ 有聚合ID：基于哈希路由到特定Worker（保证顺序）
				p.dispatchByHash(work)
			} else {
				// ✅ 无聚合ID：轮询分配到任意Worker（高并发）
				p.dispatchRoundRobin(work)
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// dispatchByHash 基于聚合ID哈希分发（保证同一聚合ID的消息顺序）
func (p *UnifiedWorkerPool) dispatchByHash(work UnifiedWorkItem) {
	idx := p.hashToIndex(work.AggregateID)
	worker := p.workers[idx]

	// 尝试快速入队
	select {
	case worker.workChan <- work:
		return
	default:
		// Worker忙，阻塞等待（保证顺序，不能换Worker）
		worker.workChan <- work
	}
}

// dispatchRoundRobin 轮询分发（无聚合ID的消息，追求高并发）
func (p *UnifiedWorkerPool) dispatchRoundRobin(work UnifiedWorkItem) {
	p.roundRobinMu.Lock()
	startIndex := p.roundRobinIndex
	p.roundRobinIndex = (p.roundRobinIndex + 1) % p.workerCount
	p.roundRobinMu.Unlock()

	// 尝试从当前索引开始，找到第一个可用的Worker
	for i := 0; i < p.workerCount; i++ {
		idx := (startIndex + i) % p.workerCount
		worker := p.workers[idx]

		select {
		case worker.workChan <- work:
			return // 成功分发
		default:
			continue // 这个Worker忙，尝试下一个
		}
	}

	// 所有Worker都忙，阻塞等待第一个可用的Worker
	p.workers[startIndex].workChan <- work
}

// hashToIndex 将聚合ID哈希到Worker索引
func (p *UnifiedWorkerPool) hashToIndex(aggregateID string) int {
	h := fnv.New32a()
	h.Write([]byte(aggregateID))
	return int(h.Sum32() % uint32(p.workerCount))
}

// SubmitWork 提交工作到统一Worker池
func (p *UnifiedWorkerPool) SubmitWork(work UnifiedWorkItem) bool {
	select {
	case p.workQueue <- work:
		return true
	case <-time.After(500 * time.Millisecond): // 🔥 优化：增加超时（从 100ms → 500ms）
		// 等待500ms后仍然满，记录警告但仍尝试提交
		p.logger.Warn("Unified worker pool queue full, applying backpressure",
			zap.String("topic", work.Topic),
			zap.String("aggregateID", work.AggregateID),
			zap.Int("queueSize", p.queueSize),
			zap.Int("workerCount", p.workerCount))
		// 阻塞等待，确保消息不丢失
		p.workQueue <- work
		return true
	}
}

// Close 关闭Worker池
func (p *UnifiedWorkerPool) Close() {
	p.cancel()

	// 关闭所有Worker
	for _, worker := range p.workers {
		close(worker.quit)
	}

	// 等待所有Worker完成
	p.wg.Wait()

	p.logger.Info("Unified Worker Pool closed")
}

// start Worker启动
func (w *UnifiedWorker) start() {
	defer w.pool.wg.Done()

	for {
		select {
		case work := <-w.workChan:
			w.processWork(work)
		case <-w.quit:
			return
		case <-w.pool.ctx.Done():
			return
		}
	}
}

// processWork 处理工作
func (w *UnifiedWorker) processWork(work UnifiedWorkItem) {
	defer func() {
		if r := recover(); r != nil {
			w.pool.logger.Error("Unified Worker panic during message processing",
				zap.Int("workerID", w.id),
				zap.String("topic", work.Topic),
				zap.String("aggregateID", work.AggregateID),
				zap.Any("panic", r))
		}
	}()

	// 处理消息
	err := work.Process()
	if err != nil {
		w.pool.logger.Error("Unified Worker message processing failed",
			zap.Int("workerID", w.id),
			zap.String("topic", work.Topic),
			zap.String("aggregateID", work.AggregateID),
			zap.Error(err))
	}

	// 🔥 优化：处理完成后立即 ACK（NATS）
	if work.NATSAckFunc != nil {
		if ackErr := work.NATSAckFunc(); ackErr != nil {
			w.pool.logger.Error("Failed to ACK NATS message",
				zap.Int("workerID", w.id),
				zap.String("topic", work.Topic),
				zap.String("aggregateID", work.AggregateID),
				zap.Error(ackErr))
		}
	}

	// 🔥 优化：处理完成后立即 Mark（Kafka）
	if work.KafkaMessage != nil && work.KafkaSession != nil {
		// Kafka 的 Mark 是异步的，不需要等待
		// 注意：这里不需要显式调用，Kafka 的 ConsumeClaim 会自动处理
	}
}
