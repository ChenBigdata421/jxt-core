package eventbus

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"time"
)

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
}

// KeyedWorkerPool implements Phase 1: a fixed-size keyed worker pool.
// - Same aggregateID is routed to the same worker via consistent hashing
// - Each worker processes messages sequentially to guarantee per-aggregate ordering
// - Each worker has a bounded queue; enqueue will block up to WaitTimeout
// - If enqueue times out, ProcessMessage returns ErrWorkerQueueFull (caller can apply backpressure)

var ErrWorkerQueueFull = errors.New("keyed worker queue full")

// KeyedWorkerPoolConfig configuration for the pool.
type KeyedWorkerPoolConfig struct {
	WorkerCount int           // number of workers (e.g., 256/1024)
	QueueSize   int           // per-worker queue capacity (bounded)
	WaitTimeout time.Duration // max time to wait when queue is full on enqueue
}

// KeyedWorkerPool routes AggregateMessage by AggregateID to workers.
// The pool is created per subscription/topic so it can call the topic's handler.
type KeyedWorkerPool struct {
	cfg     KeyedWorkerPoolConfig
	handler MessageHandler

	workers []chan *AggregateMessage
	wg      sync.WaitGroup
	stopCh  chan struct{}
}

func NewKeyedWorkerPool(cfg KeyedWorkerPoolConfig, handler MessageHandler) *KeyedWorkerPool {
	// 使用默认值（如果未配置）
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = DefaultKeyedWorkerCount
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = DefaultKeyedQueueSize
	}
	if cfg.WaitTimeout <= 0 {
		cfg.WaitTimeout = DefaultKeyedWaitTimeout
	}

	kp := &KeyedWorkerPool{
		cfg:     cfg,
		handler: handler,
		workers: make([]chan *AggregateMessage, cfg.WorkerCount),
		stopCh:  make(chan struct{}),
	}

	for i := 0; i < cfg.WorkerCount; i++ {
		ch := make(chan *AggregateMessage, cfg.QueueSize)
		kp.workers[i] = ch
		kp.wg.Add(1)
		go kp.runWorker(ch)
	}

	return kp
}

func (kp *KeyedWorkerPool) runWorker(ch chan *AggregateMessage) {
	defer kp.wg.Done()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// Process sequentially; guarantee per-key ordering because routing is stable.
			err := kp.handler(msg.Context, msg.Value)
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

// ProcessMessage routes the message to its worker and enqueues it.
func (kp *KeyedWorkerPool) ProcessMessage(ctx context.Context, msg *AggregateMessage) error {
	// Require AggregateID to route; if missing, reject so caller can fallback
	if msg.AggregateID == "" {
		return errors.New("aggregateID required for keyed worker pool")
	}

	idx := kp.hashToIndex(msg.AggregateID)
	ch := kp.workers[idx]

	// Try fast-path enqueue.
	select {
	case ch <- msg:
		return nil
	default:
	}

	// Bounded wait to avoid busy-loop; caller can apply backpressure if needed.
	timer := time.NewTimer(kp.cfg.WaitTimeout)
	defer timer.Stop()

	select {
	case ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrWorkerQueueFull
	}
}

// Stop stops all workers and drains queues.
func (kp *KeyedWorkerPool) Stop() {
	close(kp.stopCh)
	// close all worker channels to stop goroutines
	for _, ch := range kp.workers {
		close(ch)
	}
	kp.wg.Wait()
}

func (kp *KeyedWorkerPool) hashToIndex(key string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(len(kp.workers)))
}
