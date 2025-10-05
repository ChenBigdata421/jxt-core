package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// NATSBacklogDetector NATS JetStream 积压检测器
// 专门针对 JetStream 的积压检测实现
type NATSBacklogDetector struct {
	js               nats.JetStreamContext
	conn             *nats.Conn
	streamName       string
	consumers        map[string]string // consumerName -> durableName
	maxLagThreshold  int64
	maxTimeThreshold time.Duration
	checkInterval    time.Duration
	lastCheckTime    time.Time
	lastCheckResult  bool
	mu               sync.RWMutex
	logger           *zap.Logger

	// 回调和生命周期管理
	callbacks  []BacklogStateCallback
	callbackMu sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	isRunning  bool
	ticker     *time.Ticker
}

// NewNATSBacklogDetector 创建 NATS 积压检测器
func NewNATSBacklogDetector(js nats.JetStreamContext, conn *nats.Conn, streamName string, config BacklogDetectionConfig) *NATSBacklogDetector {
	// 确保 logger 不为 nil
	var log *zap.Logger
	if logger.Logger != nil {
		log = logger.Logger
	} else {
		// 如果全局 logger 未初始化，创建一个简单的 logger
		log, _ = zap.NewDevelopment()
	}

	return &NATSBacklogDetector{
		js:               js,
		conn:             conn,
		streamName:       streamName,
		consumers:        make(map[string]string),
		maxLagThreshold:  config.MaxLagThreshold,
		maxTimeThreshold: config.MaxTimeThreshold,
		checkInterval:    config.CheckInterval,
		logger:           log,
		callbacks:        make([]BacklogStateCallback, 0),
	}
}

// RegisterConsumer 注册消费者
func (nbd *NATSBacklogDetector) RegisterConsumer(consumerName, durableName string) {
	nbd.mu.Lock()
	defer nbd.mu.Unlock()
	nbd.consumers[consumerName] = durableName
	nbd.logger.Debug("NATS consumer registered for backlog detection",
		zap.String("consumer", consumerName),
		zap.String("durable", durableName))
}

// UnregisterConsumer 注销消费者
func (nbd *NATSBacklogDetector) UnregisterConsumer(consumerName string) {
	nbd.mu.Lock()
	defer nbd.mu.Unlock()
	delete(nbd.consumers, consumerName)
	nbd.logger.Debug("NATS consumer unregistered from backlog detection",
		zap.String("consumer", consumerName))
}

// IsNoBacklog 检测是否无积压
func (nbd *NATSBacklogDetector) IsNoBacklog(ctx context.Context) (bool, error) {
	nbd.mu.Lock()
	defer nbd.mu.Unlock()

	// 检查是否需要重新检测
	if time.Since(nbd.lastCheckTime) < nbd.checkInterval {
		return nbd.lastCheckResult, nil
	}

	// 检查 JetStream 连接状态
	if nbd.js == nil {
		return false, fmt.Errorf("JetStream context is not available")
	}

	// 获取流信息
	streamInfo, err := nbd.js.StreamInfo(nbd.streamName)
	if err != nil {
		nbd.logger.Error("Failed to get stream info",
			zap.String("stream", nbd.streamName),
			zap.Error(err))
		return false, fmt.Errorf("failed to get stream info: %w", err)
	}

	// 检查是否有消费者
	if len(nbd.consumers) == 0 {
		nbd.logger.Debug("No consumers registered, considering no backlog")
		nbd.lastCheckTime = time.Now()
		nbd.lastCheckResult = true
		return true, nil
	}

	// 并发检测所有消费者的积压情况
	lagChan := make(chan consumerLag, len(nbd.consumers))
	errChan := make(chan error, len(nbd.consumers))
	var wg sync.WaitGroup

	// 并发检测每个消费者
	for consumerName, durableName := range nbd.consumers {
		wg.Add(1)
		go func(cName, dName string) {
			defer wg.Done()
			if err := nbd.checkConsumerBacklog(ctx, cName, dName, streamInfo, lagChan); err != nil {
				errChan <- err
			}
		}(consumerName, durableName)
	}

	// 等待所有检测完成
	go func() {
		wg.Wait()
		close(lagChan)
		close(errChan)
	}()

	// 收集结果
	var totalLag int64
	var maxLagTime time.Time
	hasBacklog := false

	for lag := range lagChan {
		totalLag += lag.lag
		if lag.lag > nbd.maxLagThreshold {
			hasBacklog = true
		}
		if lag.timestamp.After(maxLagTime) {
			maxLagTime = lag.timestamp
		}
	}

	// 检查是否有错误
	select {
	case err := <-errChan:
		if err != nil {
			nbd.logger.Error("Error during NATS backlog detection", zap.Error(err))
			return false, err
		}
	default:
	}

	// 检查时间维度的积压
	if !maxLagTime.IsZero() && time.Since(maxLagTime) > nbd.maxTimeThreshold {
		hasBacklog = true
	}

	// 更新检测结果
	nbd.lastCheckTime = time.Now()
	nbd.lastCheckResult = !hasBacklog

	if hasBacklog {
		nbd.logger.Warn("NATS message backlog detected",
			zap.Int64("totalLag", totalLag),
			zap.Time("maxLagTime", maxLagTime),
			zap.String("stream", nbd.streamName))
	} else {
		nbd.logger.Debug("No NATS message backlog detected",
			zap.Int64("totalLag", totalLag),
			zap.String("stream", nbd.streamName))
	}

	return !hasBacklog, nil
}

// consumerLag 消费者积压信息
type consumerLag struct {
	consumer  string
	lag       int64
	timestamp time.Time
}

// checkConsumerBacklog 检测单个消费者的积压情况
func (nbd *NATSBacklogDetector) checkConsumerBacklog(ctx context.Context, consumerName, durableName string, streamInfo *nats.StreamInfo, lagChan chan<- consumerLag) error {
	// 获取消费者信息
	consumerInfo, err := nbd.js.ConsumerInfo(nbd.streamName, durableName)
	if err != nil {
		nbd.logger.Error("Failed to get consumer info",
			zap.String("stream", nbd.streamName),
			zap.String("consumer", consumerName),
			zap.String("durable", durableName),
			zap.Error(err))
		return fmt.Errorf("failed to get consumer info for %s: %w", consumerName, err)
	}

	// 计算积压
	// JetStream 中的积压 = 流中的总消息数 - 消费者已确认的消息数
	streamMsgs := int64(streamInfo.State.Msgs)
	consumerAcked := int64(consumerInfo.AckFloor.Consumer)
	lag := streamMsgs - consumerAcked

	if lag < 0 {
		lag = 0 // 避免负数
	}

	// 获取最后消费时间（使用消费者的最后活动时间）
	var timestamp time.Time
	if consumerInfo.Delivered.Last != nil {
		timestamp = *consumerInfo.Delivered.Last
	} else {
		timestamp = time.Now() // 如果没有消费记录，使用当前时间
	}

	lagChan <- consumerLag{
		consumer:  consumerName,
		lag:       lag,
		timestamp: timestamp,
	}

	return nil
}

// GetBacklogInfo 获取详细积压信息
func (nbd *NATSBacklogDetector) GetBacklogInfo(ctx context.Context) (*NATSBacklogInfo, error) {
	nbd.mu.RLock()
	defer nbd.mu.RUnlock()

	// 检查 JetStream 连接状态
	if nbd.js == nil {
		return nil, fmt.Errorf("JetStream context is not available")
	}

	// 获取流信息
	streamInfo, err := nbd.js.StreamInfo(nbd.streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	info := &NATSBacklogInfo{
		StreamName: nbd.streamName,
		CheckTime:  time.Now(),
		TotalLag:   0,
		Consumers:  make(map[string]*NATSConsumerBacklogInfo),
		StreamInfo: NATSStreamInfo{
			Messages: int64(streamInfo.State.Msgs),
			Bytes:    int64(streamInfo.State.Bytes),
			FirstSeq: int64(streamInfo.State.FirstSeq),
			LastSeq:  int64(streamInfo.State.LastSeq),
		},
	}

	// 获取每个消费者的详细信息
	for consumerName, durableName := range nbd.consumers {
		consumerInfo, err := nbd.js.ConsumerInfo(nbd.streamName, durableName)
		if err != nil {
			nbd.logger.Error("Failed to get consumer info for backlog info",
				zap.String("consumer", consumerName),
				zap.Error(err))
			continue
		}

		lag := int64(streamInfo.State.Msgs) - int64(consumerInfo.AckFloor.Consumer)
		if lag < 0 {
			lag = 0
		}

		var lastDelivered time.Time
		if consumerInfo.Delivered.Last != nil {
			lastDelivered = *consumerInfo.Delivered.Last
		}

		info.Consumers[consumerName] = &NATSConsumerBacklogInfo{
			ConsumerName:    consumerName,
			DurableName:     durableName,
			AckedMessages:   int64(consumerInfo.AckFloor.Consumer),
			PendingMessages: int64(consumerInfo.NumPending),
			Lag:             lag,
			LastDelivered:   lastDelivered,
			NumRedelivered:  int64(consumerInfo.NumRedelivered),
		}

		info.TotalLag += lag
	}

	return info, nil
}

// NATSBacklogInfo NATS 积压信息
type NATSBacklogInfo struct {
	StreamName string                              `json:"streamName"`
	CheckTime  time.Time                           `json:"checkTime"`
	TotalLag   int64                               `json:"totalLag"`
	Consumers  map[string]*NATSConsumerBacklogInfo `json:"consumers"`
	StreamInfo NATSStreamInfo                      `json:"streamInfo"`
}

// NATSConsumerBacklogInfo NATS 消费者积压信息
type NATSConsumerBacklogInfo struct {
	ConsumerName    string    `json:"consumerName"`
	DurableName     string    `json:"durableName"`
	AckedMessages   int64     `json:"ackedMessages"`
	PendingMessages int64     `json:"pendingMessages"`
	Lag             int64     `json:"lag"`
	LastDelivered   time.Time `json:"lastDelivered"`
	NumRedelivered  int64     `json:"numRedelivered"`
}

// NATSStreamInfo NATS 流信息
type NATSStreamInfo struct {
	Messages int64 `json:"messages"`
	Bytes    int64 `json:"bytes"`
	FirstSeq int64 `json:"firstSeq"`
	LastSeq  int64 `json:"lastSeq"`
}

// RegisterCallback 注册积压状态回调
func (nbd *NATSBacklogDetector) RegisterCallback(callback BacklogStateCallback) error {
	nbd.callbackMu.Lock()
	defer nbd.callbackMu.Unlock()
	nbd.callbacks = append(nbd.callbacks, callback)
	nbd.logger.Debug("NATS backlog callback registered")
	return nil
}

// Start 启动积压监控
func (nbd *NATSBacklogDetector) Start(ctx context.Context) error {
	nbd.mu.Lock()
	defer nbd.mu.Unlock()

	if nbd.isRunning {
		nbd.logger.Warn("NATS backlog detector is already running")
		return nil
	}

	// 创建子 context
	nbd.ctx, nbd.cancel = context.WithCancel(ctx)
	nbd.ticker = time.NewTicker(nbd.checkInterval)
	nbd.isRunning = true

	// 启动监控 goroutine
	nbd.wg.Add(1)
	go nbd.monitorLoop()

	nbd.logger.Info("NATS backlog monitoring started",
		zap.String("stream", nbd.streamName),
		zap.Duration("interval", nbd.checkInterval))
	return nil
}

// Stop 停止积压监控
func (nbd *NATSBacklogDetector) Stop() error {
	nbd.mu.Lock()
	defer nbd.mu.Unlock()

	if !nbd.isRunning {
		nbd.logger.Debug("NATS backlog detector is not running")
		return nil
	}

	// 停止监控
	if nbd.cancel != nil {
		nbd.cancel()
	}

	if nbd.ticker != nil {
		nbd.ticker.Stop()
	}

	nbd.isRunning = false

	// 等待 goroutine 结束
	nbd.mu.Unlock()
	nbd.wg.Wait()
	nbd.mu.Lock()

	nbd.logger.Info("NATS backlog monitoring stopped",
		zap.String("stream", nbd.streamName))
	return nil
}

// monitorLoop 监控循环
func (nbd *NATSBacklogDetector) monitorLoop() {
	defer nbd.wg.Done()
	defer nbd.ticker.Stop()

	nbd.logger.Debug("NATS backlog monitor loop started")

	for {
		select {
		case <-nbd.ctx.Done():
			nbd.logger.Debug("NATS backlog monitor loop stopped")
			return
		case <-nbd.ticker.C:
			nbd.performBacklogCheck()
		}
	}
}

// performBacklogCheck 执行积压检查
func (nbd *NATSBacklogDetector) performBacklogCheck() {
	hasBacklog, err := nbd.IsNoBacklog(nbd.ctx)
	if err != nil {
		nbd.logger.Error("NATS backlog check failed", zap.Error(err))
		return
	}

	// 获取详细积压信息
	backlogInfo, err := nbd.GetBacklogInfo(nbd.ctx)
	if err != nil {
		nbd.logger.Error("Failed to get NATS backlog info", zap.Error(err))
		return
	}

	// 构造状态
	state := BacklogState{
		HasBacklog:    !hasBacklog,
		LagCount:      backlogInfo.TotalLag,
		Timestamp:     time.Now(),
		Topic:         nbd.streamName, // 对于 NATS，使用 stream 名称作为 topic
		ConsumerGroup: nbd.streamName, // 对于 NATS，使用 stream 名称作为消费者组
	}

	// 计算最大积压时间（简化实现）
	if state.HasBacklog {
		state.LagTime = time.Since(nbd.lastCheckTime)
	}

	// 通知回调
	nbd.notifyCallbacks(state)
}

// notifyCallbacks 通知回调
func (nbd *NATSBacklogDetector) notifyCallbacks(state BacklogState) {
	nbd.callbackMu.RLock()
	callbacks := make([]BacklogStateCallback, len(nbd.callbacks))
	copy(callbacks, nbd.callbacks)
	nbd.callbackMu.RUnlock()

	// 从当前 context 派生，而不是使用 Background
	// 如果 ctx 为 nil，则使用 Background 作为后备
	nbd.mu.RLock()
	parentCtx := nbd.ctx
	nbd.mu.RUnlock()
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	for _, callback := range callbacks {
		go func(cb BacklogStateCallback) {
			// 使用父 context 派生，支持取消传播
			ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
			defer cancel()

			if err := cb(ctx, state); err != nil {
				nbd.logger.Error("NATS backlog callback failed",
					zap.Error(err),
					zap.Bool("hasBacklog", state.HasBacklog))
			}
		}(callback)
	}
}
