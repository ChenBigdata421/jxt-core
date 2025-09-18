package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/zap"
)

// AggregateProcessor 聚合处理器
// 确保同一聚合ID的消息按序处理
type AggregateProcessor struct {
	aggregateID  string
	messages     chan *AggregateMessage
	lastActivity atomic.Value // time.Time
	done         chan struct{}
	handler      MessageHandler
	rateLimiter  *RateLimiter
	logger       *zap.Logger
	isRunning    atomic.Bool
}

// AggregateMessage 聚合消息
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

// NewAggregateProcessor 创建聚合处理器
func NewAggregateProcessor(aggregateID string, handler MessageHandler, rateLimiter *RateLimiter) *AggregateProcessor {
	ap := &AggregateProcessor{
		aggregateID: aggregateID,
		messages:    make(chan *AggregateMessage, 100), // 缓冲区大小可配置
		done:        make(chan struct{}),
		handler:     handler,
		rateLimiter: rateLimiter,
		logger:      logger.Logger,
	}

	ap.lastActivity.Store(time.Now())
	return ap
}

// Start 启动聚合处理器
func (ap *AggregateProcessor) Start(ctx context.Context) {
	if !ap.isRunning.CompareAndSwap(false, true) {
		return // 已经在运行
	}

	go ap.processMessages(ctx)
	ap.logger.Debug("Aggregate processor started", zap.String("aggregateID", ap.aggregateID))
}

// Stop 停止聚合处理器
func (ap *AggregateProcessor) Stop() {
	if !ap.isRunning.CompareAndSwap(true, false) {
		return // 已经停止
	}

	close(ap.done)
	ap.logger.Debug("Aggregate processor stopped", zap.String("aggregateID", ap.aggregateID))
}

// ProcessMessage 处理消息
func (ap *AggregateProcessor) ProcessMessage(ctx context.Context, msg *AggregateMessage) error {
	if !ap.isRunning.Load() {
		return fmt.Errorf("aggregate processor is not running")
	}

	select {
	case ap.messages <- msg:
		ap.lastActivity.Store(time.Now())
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-ap.done:
		return fmt.Errorf("aggregate processor is shutting down")
	}
}

// processMessages 处理消息循环
func (ap *AggregateProcessor) processMessages(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			ap.logger.Error("Aggregate processor panic recovered",
				zap.String("aggregateID", ap.aggregateID),
				zap.Any("panic", r))
		}
	}()

	for {
		select {
		case msg := <-ap.messages:
			ap.handleMessage(ctx, msg)
		case <-ap.done:
			// 处理剩余消息
			ap.drainMessages(ctx)
			return
		case <-ctx.Done():
			ap.drainMessages(ctx)
			return
		}
	}
}

// handleMessage 处理单个消息
func (ap *AggregateProcessor) handleMessage(ctx context.Context, msg *AggregateMessage) {
	// 流量控制
	if ap.rateLimiter != nil {
		if err := ap.rateLimiter.Wait(ctx); err != nil {
			ap.sendError(msg, fmt.Errorf("rate limit error: %w", err))
			return
		}
	}

	// 处理消息
	start := time.Now()
	err := ap.handler(msg.Context, msg.Value)
	duration := time.Since(start)

	if err != nil {
		ap.logger.Error("Message processing failed",
			zap.String("aggregateID", ap.aggregateID),
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset),
			zap.Error(err),
			zap.Duration("duration", duration))
	} else {
		ap.logger.Debug("Message processed successfully",
			zap.String("aggregateID", ap.aggregateID),
			zap.String("topic", msg.Topic),
			zap.Int32("partition", msg.Partition),
			zap.Int64("offset", msg.Offset),
			zap.Duration("duration", duration))
	}

	ap.sendError(msg, err)
	ap.lastActivity.Store(time.Now())
}

// sendError 发送处理结果
func (ap *AggregateProcessor) sendError(msg *AggregateMessage, err error) {
	select {
	case msg.Done <- err:
	default:
		// 非阻塞发送，避免死锁
	}
}

// drainMessages 排空剩余消息
func (ap *AggregateProcessor) drainMessages(ctx context.Context) {
	for {
		select {
		case msg := <-ap.messages:
			ap.sendError(msg, fmt.Errorf("processor is shutting down"))
		default:
			return
		}
	}
}

// GetLastActivity 获取最后活动时间
func (ap *AggregateProcessor) GetLastActivity() time.Time {
	return ap.lastActivity.Load().(time.Time)
}

// IsIdle 检查是否空闲
func (ap *AggregateProcessor) IsIdle(idleTimeout time.Duration) bool {
	return time.Since(ap.GetLastActivity()) > idleTimeout
}

// GetStats 获取处理器统计信息
func (ap *AggregateProcessor) GetStats() *AggregateProcessorStats {
	return &AggregateProcessorStats{
		AggregateID:   ap.aggregateID,
		IsRunning:     ap.isRunning.Load(),
		QueueSize:     len(ap.messages),
		QueueCapacity: cap(ap.messages),
		LastActivity:  ap.GetLastActivity(),
	}
}

// AggregateProcessorStats 聚合处理器统计信息
type AggregateProcessorStats struct {
	AggregateID   string    `json:"aggregateId"`
	IsRunning     bool      `json:"isRunning"`
	QueueSize     int       `json:"queueSize"`
	QueueCapacity int       `json:"queueCapacity"`
	LastActivity  time.Time `json:"lastActivity"`
}

// AggregateProcessorManager 聚合处理器管理器
type AggregateProcessorManager struct {
	processors    *lru.Cache[string, *AggregateProcessor]
	rateLimiter   *RateLimiter
	idleTimeout   time.Duration
	maxProcessors int
	handler       MessageHandler
	mu            sync.RWMutex
	logger        *zap.Logger
	ctx           context.Context
	cancel        context.CancelFunc
	cleanupTicker *time.Ticker
}

// NewAggregateProcessorManager 创建聚合处理器管理器
func NewAggregateProcessorManager(maxProcessors int, idleTimeout time.Duration, rateLimiter *RateLimiter, handler MessageHandler) (*AggregateProcessorManager, error) {
	cache, err := lru.New[string, *AggregateProcessor](maxProcessors)
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	apm := &AggregateProcessorManager{
		processors:    cache,
		rateLimiter:   rateLimiter,
		idleTimeout:   idleTimeout,
		maxProcessors: maxProcessors,
		handler:       handler,
		logger:        logger.Logger,
		ctx:           ctx,
		cancel:        cancel,
		cleanupTicker: time.NewTicker(idleTimeout / 2), // 清理频率为空闲超时的一半
	}

	// 启动清理协程
	go apm.cleanupLoop()

	return apm, nil
}

// GetOrCreateProcessor 获取或创建聚合处理器
func (apm *AggregateProcessorManager) GetOrCreateProcessor(aggregateID string) (*AggregateProcessor, error) {
	apm.mu.Lock()
	defer apm.mu.Unlock()

	// 尝试从缓存获取
	if processor, exists := apm.processors.Get(aggregateID); exists {
		return processor, nil
	}

	// 创建新的处理器
	processor := NewAggregateProcessor(aggregateID, apm.handler, apm.rateLimiter)
	processor.Start(apm.ctx)

	// 添加到缓存，可能会触发LRU淘汰
	evicted := apm.processors.Add(aggregateID, processor)
	if evicted {
		apm.logger.Debug("Processor cache evicted old entry", zap.String("aggregateID", aggregateID))
	}

	apm.logger.Debug("Created new aggregate processor", zap.String("aggregateID", aggregateID))
	return processor, nil
}

// ProcessMessage 处理消息
func (apm *AggregateProcessorManager) ProcessMessage(ctx context.Context, msg *AggregateMessage) error {
	processor, err := apm.GetOrCreateProcessor(msg.AggregateID)
	if err != nil {
		return fmt.Errorf("failed to get processor: %w", err)
	}

	return processor.ProcessMessage(ctx, msg)
}

// cleanupLoop 清理空闲处理器
func (apm *AggregateProcessorManager) cleanupLoop() {
	defer apm.cleanupTicker.Stop()

	for {
		select {
		case <-apm.cleanupTicker.C:
			apm.cleanupIdleProcessors()
		case <-apm.ctx.Done():
			return
		}
	}
}

// cleanupIdleProcessors 清理空闲处理器
func (apm *AggregateProcessorManager) cleanupIdleProcessors() {
	apm.mu.Lock()
	defer apm.mu.Unlock()

	var toRemove []string

	// 遍历所有处理器，找出空闲的
	for _, aggregateID := range apm.processors.Keys() {
		if processor, exists := apm.processors.Peek(aggregateID); exists {
			if processor.IsIdle(apm.idleTimeout) {
				toRemove = append(toRemove, aggregateID)
			}
		}
	}

	// 移除空闲处理器
	for _, aggregateID := range toRemove {
		if processor, exists := apm.processors.Get(aggregateID); exists {
			processor.Stop()
			apm.processors.Remove(aggregateID)
			apm.logger.Debug("Removed idle processor", zap.String("aggregateID", aggregateID))
		}
	}

	if len(toRemove) > 0 {
		apm.logger.Info("Cleaned up idle processors", zap.Int("count", len(toRemove)))
	}
}

// Stop 停止管理器
func (apm *AggregateProcessorManager) Stop() {
	apm.cancel()

	apm.mu.Lock()
	defer apm.mu.Unlock()

	// 停止所有处理器
	for _, aggregateID := range apm.processors.Keys() {
		if processor, exists := apm.processors.Get(aggregateID); exists {
			processor.Stop()
		}
	}

	apm.processors.Purge()
	apm.logger.Info("Aggregate processor manager stopped")
}

// GetStats 获取管理器统计信息
func (apm *AggregateProcessorManager) GetStats() *AggregateProcessorManagerStats {
	apm.mu.RLock()
	defer apm.mu.RUnlock()

	stats := &AggregateProcessorManagerStats{
		TotalProcessors: apm.processors.Len(),
		MaxProcessors:   apm.maxProcessors,
		IdleTimeout:     apm.idleTimeout,
		Processors:      make([]*AggregateProcessorStats, 0, apm.processors.Len()),
	}

	for _, aggregateID := range apm.processors.Keys() {
		if processor, exists := apm.processors.Peek(aggregateID); exists {
			stats.Processors = append(stats.Processors, processor.GetStats())
			if processor.isRunning.Load() {
				stats.ActiveProcessors++
			} else {
				stats.IdleProcessors++
			}
		}
	}

	return stats
}

// AggregateProcessorManagerStats 聚合处理器管理器统计信息
type AggregateProcessorManagerStats struct {
	TotalProcessors  int                        `json:"totalProcessors"`
	ActiveProcessors int                        `json:"activeProcessors"`
	IdleProcessors   int                        `json:"idleProcessors"`
	MaxProcessors    int                        `json:"maxProcessors"`
	IdleTimeout      time.Duration              `json:"idleTimeout"`
	Processors       []*AggregateProcessorStats `json:"processors"`
}
