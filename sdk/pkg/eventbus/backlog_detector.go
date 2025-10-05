package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// partitionLag 分区积压信息
type partitionLag struct {
	topic     string
	partition int32
	lag       int64
	timestamp time.Time
}

// BacklogDetector 消息积压检测器
// 比evidence-management更优雅的实现，复用EventBus的连接
type BacklogDetector struct {
	client           sarama.Client
	admin            sarama.ClusterAdmin
	consumerGroup    string
	maxLagThreshold  int64
	maxTimeThreshold time.Duration
	checkInterval    time.Duration
	lastCheckTime    time.Time
	lastCheckResult  bool
	mu               sync.RWMutex
	logger           *zap.Logger

	// 新增字段支持回调和生命周期管理
	callbacks  []BacklogStateCallback
	callbackMu sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	isRunning  bool
	ticker     *time.Ticker
}

// NewBacklogDetector 创建积压检测器
func NewBacklogDetector(client sarama.Client, admin sarama.ClusterAdmin, consumerGroup string, config BacklogDetectionConfig) *BacklogDetector {
	return &BacklogDetector{
		client:           client,
		admin:            admin,
		consumerGroup:    consumerGroup,
		maxLagThreshold:  config.MaxLagThreshold,
		maxTimeThreshold: config.MaxTimeThreshold,
		checkInterval:    config.CheckInterval,
		logger:           logger.Logger,
	}
}

// BacklogDetectionConfig 积压检测配置
type BacklogDetectionConfig struct {
	MaxLagThreshold  int64         // 最大消息积压数量
	MaxTimeThreshold time.Duration // 最大积压时间
	CheckInterval    time.Duration // 检测间隔
}

// IsNoBacklog 检测是否无积压
// 实现比evidence-management更高效的检测逻辑
func (bd *BacklogDetector) IsNoBacklog(ctx context.Context) (bool, error) {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	// 检查是否需要重新检测
	if time.Since(bd.lastCheckTime) < bd.checkInterval {
		return bd.lastCheckResult, nil
	}

	// 获取消费者组的偏移量信息
	groupOffsets, err := bd.admin.ListConsumerGroupOffsets(bd.consumerGroup, nil)
	if err != nil {
		bd.logger.Error("Failed to get consumer group offsets", zap.Error(err), zap.String("group", bd.consumerGroup))
		return false, fmt.Errorf("failed to get consumer group offsets: %w", err)
	}

	// 获取所有topic的最新偏移量
	topics := make(map[string][]int32)
	for topic, partitions := range groupOffsets.Blocks {
		var partitionList []int32
		for partition := range partitions {
			partitionList = append(partitionList, partition)
		}
		topics[topic] = partitionList
	}

	// 并发检测所有topic的积压情况

	lagChan := make(chan partitionLag, len(topics)*10) // 预估容量
	errChan := make(chan error, len(topics))
	var wg sync.WaitGroup

	// 并发检测每个topic
	for topic, partitions := range topics {
		wg.Add(1)
		go func(topic string, partitions []int32) {
			defer wg.Done()
			if err := bd.checkTopicBacklog(ctx, topic, partitions, groupOffsets.Blocks[topic], lagChan); err != nil {
				errChan <- err
			}
		}(topic, partitions)
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
		if lag.lag > bd.maxLagThreshold {
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
			bd.logger.Error("Error during backlog detection", zap.Error(err))
			return false, err
		}
	default:
	}

	// 检查时间维度的积压
	if !maxLagTime.IsZero() && time.Since(maxLagTime) > bd.maxTimeThreshold {
		hasBacklog = true
	}

	// 更新检测结果
	bd.lastCheckTime = time.Now()
	bd.lastCheckResult = !hasBacklog

	if hasBacklog {
		bd.logger.Warn("Message backlog detected",
			zap.Int64("totalLag", totalLag),
			zap.Time("maxLagTime", maxLagTime),
			zap.String("consumerGroup", bd.consumerGroup))
	} else {
		bd.logger.Debug("No message backlog detected",
			zap.Int64("totalLag", totalLag),
			zap.String("consumerGroup", bd.consumerGroup))
	}

	return !hasBacklog, nil
}

// checkTopicBacklog 检测单个topic的积压情况
func (bd *BacklogDetector) checkTopicBacklog(ctx context.Context, topic string, partitions []int32, groupPartitions map[int32]*sarama.OffsetFetchResponseBlock, lagChan chan<- partitionLag) error {
	// 暂时忽略 ctx 参数，未来可用于取消操作
	_ = ctx

	// 获取topic的最新偏移量（这里只是为了验证topic存在）
	_, err := bd.client.GetOffset(topic, partitions[0], sarama.OffsetNewest)
	if err != nil {
		return fmt.Errorf("failed to get latest offset for topic %s: %w", topic, err)
	}

	// 检测每个分区的积压
	for _, partition := range partitions {
		groupPartition, exists := groupPartitions[partition]
		if !exists {
			continue
		}

		// 获取分区的最新偏移量
		partitionLatestOffset, err := bd.client.GetOffset(topic, partition, sarama.OffsetNewest)
		if err != nil {
			bd.logger.Error("Failed to get partition latest offset",
				zap.String("topic", topic), zap.Int32("partition", partition), zap.Error(err))
			continue
		}

		// 计算积压
		lag := partitionLatestOffset - groupPartition.Offset
		if lag < 0 {
			lag = 0 // 避免负数
		}

		// 获取消费时间戳（如果可用）
		timestamp := time.Now() // sarama.OffsetFetchResponseBlock 没有时间戳字段

		lagChan <- partitionLag{
			topic:     topic,
			partition: partition,
			lag:       lag,
			timestamp: timestamp,
		}
	}

	return nil
}

// GetBacklogInfo 获取详细的积压信息
func (bd *BacklogDetector) GetBacklogInfo(ctx context.Context) (*BacklogInfo, error) {
	bd.mu.RLock()
	defer bd.mu.RUnlock()

	groupOffsets, err := bd.admin.ListConsumerGroupOffsets(bd.consumerGroup, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer group offsets: %w", err)
	}

	info := &BacklogInfo{
		ConsumerGroup: bd.consumerGroup,
		CheckTime:     time.Now(),
		Topics:        make(map[string]*TopicBacklogInfo),
	}

	for topic, partitions := range groupOffsets.Blocks {
		topicInfo := &TopicBacklogInfo{
			Topic:      topic,
			Partitions: make(map[int32]*PartitionBacklogInfo),
		}

		for partition, groupPartition := range partitions {
			latestOffset, err := bd.client.GetOffset(topic, partition, sarama.OffsetNewest)
			if err != nil {
				continue
			}

			lag := latestOffset - groupPartition.Offset
			if lag < 0 {
				lag = 0
			}

			topicInfo.Partitions[partition] = &PartitionBacklogInfo{
				Partition:      partition,
				ConsumerOffset: groupPartition.Offset,
				LatestOffset:   latestOffset,
				Lag:            lag,
				Timestamp:      time.Now(), // sarama.OffsetFetchResponseBlock 没有时间戳字段
			}

			topicInfo.TotalLag += lag
		}

		info.Topics[topic] = topicInfo
		info.TotalLag += topicInfo.TotalLag
	}

	return info, nil
}

// BacklogInfo 积压信息
type BacklogInfo struct {
	ConsumerGroup string                       `json:"consumerGroup"`
	CheckTime     time.Time                    `json:"checkTime"`
	TotalLag      int64                        `json:"totalLag"`
	Topics        map[string]*TopicBacklogInfo `json:"topics"`
}

// TopicBacklogInfo topic积压信息
type TopicBacklogInfo struct {
	Topic      string                          `json:"topic"`
	TotalLag   int64                           `json:"totalLag"`
	Partitions map[int32]*PartitionBacklogInfo `json:"partitions"`
}

// PartitionBacklogInfo 分区积压信息
type PartitionBacklogInfo struct {
	Partition      int32     `json:"partition"`
	ConsumerOffset int64     `json:"consumerOffset"`
	LatestOffset   int64     `json:"latestOffset"`
	Lag            int64     `json:"lag"`
	Timestamp      time.Time `json:"timestamp"`
}

// RegisterCallback 注册积压状态回调
func (bd *BacklogDetector) RegisterCallback(callback BacklogStateCallback) error {
	bd.callbackMu.Lock()
	defer bd.callbackMu.Unlock()
	bd.callbacks = append(bd.callbacks, callback)
	return nil
}

// Start 启动积压检测器
func (bd *BacklogDetector) Start(ctx context.Context) error {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	if bd.isRunning {
		return nil
	}

	bd.ctx, bd.cancel = context.WithCancel(ctx)
	bd.ticker = time.NewTicker(bd.checkInterval)
	bd.isRunning = true

	bd.wg.Add(1)
	go bd.monitoringLoop()

	bd.logger.Info("Backlog detector started",
		zap.String("consumerGroup", bd.consumerGroup),
		zap.Duration("checkInterval", bd.checkInterval))

	return nil
}

// Stop 停止积压检测器
func (bd *BacklogDetector) Stop() error {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	if !bd.isRunning {
		return nil
	}

	bd.isRunning = false
	if bd.cancel != nil {
		bd.cancel()
	}
	if bd.ticker != nil {
		bd.ticker.Stop()
	}

	bd.wg.Wait()

	bd.logger.Info("Backlog detector stopped")
	return nil
}

// monitoringLoop 监控循环
func (bd *BacklogDetector) monitoringLoop() {
	defer bd.wg.Done()

	for {
		select {
		case <-bd.ticker.C:
			bd.performBacklogCheck()
		case <-bd.ctx.Done():
			return
		}
	}
}

// performBacklogCheck 执行积压检查
func (bd *BacklogDetector) performBacklogCheck() {
	hasBacklog, err := bd.IsNoBacklog(bd.ctx)
	if err != nil {
		bd.logger.Error("Backlog check failed", zap.Error(err))
		return
	}

	// 获取详细积压信息
	backlogInfo, err := bd.GetBacklogInfo(bd.ctx)
	if err != nil {
		bd.logger.Error("Failed to get backlog info", zap.Error(err))
		return
	}

	// 构造状态
	state := BacklogState{
		HasBacklog:    !hasBacklog,
		ConsumerGroup: bd.consumerGroup,
		LagCount:      backlogInfo.TotalLag,
		Timestamp:     time.Now(),
	}

	// 计算最大积压时间（简化实现）
	if state.HasBacklog {
		state.LagTime = time.Since(bd.lastCheckTime)
	}

	// 通知回调
	bd.notifyCallbacks(state)
}

// notifyCallbacks 通知回调
func (bd *BacklogDetector) notifyCallbacks(state BacklogState) {
	bd.callbackMu.RLock()
	callbacks := make([]BacklogStateCallback, len(bd.callbacks))
	copy(callbacks, bd.callbacks)
	bd.callbackMu.RUnlock()

	// 从当前 context 派生，而不是使用 Background
	// 如果 ctx 为 nil，则使用 Background 作为后备
	bd.mu.RLock()
	parentCtx := bd.ctx
	bd.mu.RUnlock()
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	for _, callback := range callbacks {
		go func(cb BacklogStateCallback) {
			// 使用父 context 派生，支持取消传播
			ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
			defer cancel()

			if err := cb(ctx, state); err != nil {
				bd.logger.Error("Backlog callback failed",
					zap.Error(err),
					zap.Bool("hasBacklog", state.HasBacklog))
			}
		}(callback)
	}
}
