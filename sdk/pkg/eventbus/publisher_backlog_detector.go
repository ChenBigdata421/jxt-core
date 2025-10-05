package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// PublisherBacklogDetector 发送端积压检测器
type PublisherBacklogDetector struct {
	client            sarama.Client
	admin             sarama.ClusterAdmin
	maxQueueDepth     int64
	maxPublishLatency time.Duration
	rateThreshold     float64
	checkInterval     time.Duration

	// 监控指标
	publishCount   atomic.Int64
	publishLatency atomic.Int64 // 纳秒
	lastCheckTime  time.Time
	publishRate    float64
	queueDepth     atomic.Int64

	// 状态管理
	mu     sync.RWMutex
	logger *zap.Logger

	// 回调和生命周期管理
	callbacks  []PublisherBacklogCallback
	callbackMu sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	isRunning  bool
	ticker     *time.Ticker
}

// NewPublisherBacklogDetector 创建发送端积压检测器
func NewPublisherBacklogDetector(client sarama.Client, admin sarama.ClusterAdmin, cfg config.PublisherBacklogDetectionConfig) *PublisherBacklogDetector {
	return &PublisherBacklogDetector{
		client:            client,
		admin:             admin,
		maxQueueDepth:     cfg.MaxQueueDepth,
		maxPublishLatency: cfg.MaxPublishLatency,
		rateThreshold:     cfg.RateThreshold,
		checkInterval:     cfg.CheckInterval,
		lastCheckTime:     time.Now(),
		logger:            logger.Logger,
	}
}

// RecordPublish 记录发送操作
func (pbd *PublisherBacklogDetector) RecordPublish(latency time.Duration) {
	pbd.publishCount.Add(1)
	pbd.publishLatency.Add(int64(latency))
}

// UpdateQueueDepth 更新队列深度
func (pbd *PublisherBacklogDetector) UpdateQueueDepth(depth int64) {
	pbd.queueDepth.Store(depth)
}

// RegisterCallback 注册积压回调
func (pbd *PublisherBacklogDetector) RegisterCallback(callback PublisherBacklogCallback) error {
	pbd.callbackMu.Lock()
	defer pbd.callbackMu.Unlock()

	pbd.callbacks = append(pbd.callbacks, callback)
	pbd.logger.Info("Publisher backlog callback registered")
	return nil
}

// Start 启动发送端积压检测器
func (pbd *PublisherBacklogDetector) Start(ctx context.Context) error {
	pbd.mu.Lock()
	defer pbd.mu.Unlock()

	if pbd.isRunning {
		return nil
	}

	pbd.ctx, pbd.cancel = context.WithCancel(ctx)
	pbd.ticker = time.NewTicker(pbd.checkInterval)
	pbd.isRunning = true

	pbd.wg.Add(1)
	go pbd.monitoringLoop()

	pbd.logger.Info("Publisher backlog detector started",
		zap.Duration("checkInterval", pbd.checkInterval),
		zap.Int64("maxQueueDepth", pbd.maxQueueDepth),
		zap.Float64("rateThreshold", pbd.rateThreshold))

	return nil
}

// Stop 停止发送端积压检测器
func (pbd *PublisherBacklogDetector) Stop() error {
	pbd.mu.Lock()
	defer pbd.mu.Unlock()

	if !pbd.isRunning {
		return nil
	}

	pbd.isRunning = false
	if pbd.cancel != nil {
		pbd.cancel()
	}
	if pbd.ticker != nil {
		pbd.ticker.Stop()
	}

	pbd.wg.Wait()

	pbd.logger.Info("Publisher backlog detector stopped")
	return nil
}

// monitoringLoop 监控循环
func (pbd *PublisherBacklogDetector) monitoringLoop() {
	defer pbd.wg.Done()

	for {
		select {
		case <-pbd.ctx.Done():
			return
		case <-pbd.ticker.C:
			pbd.performBacklogCheck()
		}
	}
}

// performBacklogCheck 执行积压检查
func (pbd *PublisherBacklogDetector) performBacklogCheck() {
	now := time.Now()

	// 计算发送速率
	publishCount := pbd.publishCount.Load()
	timeDiff := now.Sub(pbd.lastCheckTime).Seconds()
	if timeDiff > 0 {
		pbd.publishRate = float64(publishCount) / timeDiff
	}

	// 计算平均延迟
	totalLatency := pbd.publishLatency.Load()
	var avgLatency time.Duration
	if publishCount > 0 {
		avgLatency = time.Duration(totalLatency / publishCount)
	}

	// 获取当前队列深度
	currentQueueDepth := pbd.queueDepth.Load()

	// 判断是否有积压
	hasBacklog := pbd.isBacklogged(currentQueueDepth, avgLatency, pbd.publishRate)

	// 计算积压比例和严重程度
	backlogRatio := pbd.calculateBacklogRatio(currentQueueDepth, avgLatency, pbd.publishRate)
	severity := pbd.calculateSeverity(backlogRatio)

	// 构造状态
	state := PublisherBacklogState{
		HasBacklog:        hasBacklog,
		QueueDepth:        currentQueueDepth,
		PublishRate:       pbd.publishRate,
		AvgPublishLatency: avgLatency,
		BacklogRatio:      backlogRatio,
		Timestamp:         now,
		Severity:          severity,
	}

	// 通知回调
	pbd.notifyCallbacks(state)

	// 重置计数器
	pbd.publishCount.Store(0)
	pbd.publishLatency.Store(0)
	pbd.lastCheckTime = now
}

// isBacklogged 判断是否积压
func (pbd *PublisherBacklogDetector) isBacklogged(queueDepth int64, avgLatency time.Duration, publishRate float64) bool {
	// 队列深度检查
	if pbd.maxQueueDepth > 0 && queueDepth > pbd.maxQueueDepth {
		return true
	}

	// 发送延迟检查
	if pbd.maxPublishLatency > 0 && avgLatency > pbd.maxPublishLatency {
		return true
	}

	// 发送速率检查（速率过高也可能导致积压）
	if pbd.rateThreshold > 0 && publishRate > pbd.rateThreshold {
		return true
	}

	return false
}

// calculateBacklogRatio 计算积压比例
func (pbd *PublisherBacklogDetector) calculateBacklogRatio(queueDepth int64, avgLatency time.Duration, publishRate float64) float64 {
	var ratios []float64

	// 队列深度比例
	if pbd.maxQueueDepth > 0 {
		ratios = append(ratios, float64(queueDepth)/float64(pbd.maxQueueDepth))
	}

	// 延迟比例
	if pbd.maxPublishLatency > 0 {
		ratios = append(ratios, float64(avgLatency)/float64(pbd.maxPublishLatency))
	}

	// 速率比例
	if pbd.rateThreshold > 0 {
		ratios = append(ratios, publishRate/pbd.rateThreshold)
	}

	// 返回最大比例
	maxRatio := 0.0
	for _, ratio := range ratios {
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}

	// 限制在 0.0-1.0 范围内
	if maxRatio > 1.0 {
		maxRatio = 1.0
	}

	return maxRatio
}

// calculateSeverity 计算严重程度
func (pbd *PublisherBacklogDetector) calculateSeverity(backlogRatio float64) string {
	switch {
	case backlogRatio >= 0.9:
		return "CRITICAL"
	case backlogRatio >= 0.7:
		return "HIGH"
	case backlogRatio >= 0.5:
		return "MEDIUM"
	case backlogRatio >= 0.3:
		return "LOW"
	default:
		return "NORMAL"
	}
}

// notifyCallbacks 通知回调
func (pbd *PublisherBacklogDetector) notifyCallbacks(state PublisherBacklogState) {
	pbd.callbackMu.RLock()
	callbacks := make([]PublisherBacklogCallback, len(pbd.callbacks))
	copy(callbacks, pbd.callbacks)
	pbd.callbackMu.RUnlock()

	for _, callback := range callbacks {
		go func(cb PublisherBacklogCallback) {
			defer func() {
				if r := recover(); r != nil {
					pbd.logger.Error("Publisher backlog callback panic", zap.Any("panic", r))
				}
			}()

			if err := cb(pbd.ctx, state); err != nil {
				pbd.logger.Error("Publisher backlog callback error", zap.Error(err))
			}
		}(callback)
	}
}

// GetBacklogState 获取当前积压状态
func (pbd *PublisherBacklogDetector) GetBacklogState() PublisherBacklogState {
	now := time.Now()
	publishCount := pbd.publishCount.Load()
	totalLatency := pbd.publishLatency.Load()
	currentQueueDepth := pbd.queueDepth.Load()

	var avgLatency time.Duration
	if publishCount > 0 {
		avgLatency = time.Duration(totalLatency / publishCount)
	}

	timeDiff := now.Sub(pbd.lastCheckTime).Seconds()
	publishRate := 0.0
	if timeDiff > 0 {
		publishRate = float64(publishCount) / timeDiff
	}

	hasBacklog := pbd.isBacklogged(currentQueueDepth, avgLatency, publishRate)
	backlogRatio := pbd.calculateBacklogRatio(currentQueueDepth, avgLatency, publishRate)
	severity := pbd.calculateSeverity(backlogRatio)

	return PublisherBacklogState{
		HasBacklog:        hasBacklog,
		QueueDepth:        currentQueueDepth,
		PublishRate:       publishRate,
		AvgPublishLatency: avgLatency,
		BacklogRatio:      backlogRatio,
		Timestamp:         now,
		Severity:          severity,
	}
}
