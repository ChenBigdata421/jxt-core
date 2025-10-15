package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// HealthCheckAlertCallback 健康检查告警回调函数
type HealthCheckAlertCallback func(ctx context.Context, alert HealthCheckAlert) error

// HealthCheckAlert 健康检查告警信息
type HealthCheckAlert struct {
	AlertType         string            `json:"alertType"`         // 告警类型：no_messages, connection_lost, message_expired
	Severity          string            `json:"severity"`          // 严重程度：warning, error, critical
	Source            string            `json:"source"`            // 告警来源
	EventBusType      string            `json:"eventBusType"`      // EventBus类型
	Topic             string            `json:"topic"`             // 健康检查主题
	LastMessageTime   time.Time         `json:"lastMessageTime"`   // 最后收到消息的时间
	TimeSinceLastMsg  time.Duration     `json:"timeSinceLastMsg"`  // 距离最后消息的时间
	ExpectedInterval  time.Duration     `json:"expectedInterval"`  // 期望的消息间隔
	ConsecutiveMisses int               `json:"consecutiveMisses"` // 连续错过的消息数
	Timestamp         time.Time         `json:"timestamp"`         // 告警时间
	Metadata          map[string]string `json:"metadata"`          // 额外元数据
}

// HealthCheckSubscriber 健康检查消息订阅监控器
type HealthCheckSubscriber struct {
	config       config.HealthCheckConfig
	eventBus     EventBus
	source       string
	eventBusType string

	// 状态管理
	isRunning             atomic.Bool
	lastMessageTime       atomic.Value // time.Time
	consecutiveMisses     atomic.Int32
	totalMessagesReceived atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 告警回调
	alertCallbacks []HealthCheckAlertCallback
	callbackMu     sync.RWMutex

	// 消息解析器
	parser *HealthCheckMessageParser

	// 监控统计
	stats   HealthCheckSubscriberStats
	statsMu sync.RWMutex
}

// HealthCheckSubscriberStats 订阅监控器统计信息
type HealthCheckSubscriberStats struct {
	StartTime             time.Time `json:"startTime"`
	LastMessageTime       time.Time `json:"lastMessageTime"`
	TotalMessagesReceived int64     `json:"totalMessagesReceived"`
	ConsecutiveMisses     int32     `json:"consecutiveMisses"`
	TotalAlerts           int64     `json:"totalAlerts"`
	LastAlertTime         time.Time `json:"lastAlertTime"`
	IsHealthy             bool      `json:"isHealthy"`
	UptimeSeconds         float64   `json:"uptimeSeconds"`
}

// NewHealthCheckSubscriber 创建健康检查订阅监控器
func NewHealthCheckSubscriber(config config.HealthCheckConfig, eventBus EventBus, source, eventBusType string) *HealthCheckSubscriber {
	hcs := &HealthCheckSubscriber{
		config:       config,
		eventBus:     eventBus,
		source:       source,
		eventBusType: eventBusType,
		parser:       NewHealthCheckMessageParser(),
	}

	// 设置默认配置
	if hcs.config.Subscriber.Topic == "" {
		hcs.config.Subscriber.Topic = GetHealthCheckTopic(eventBusType)
	}
	if hcs.config.Publisher.Interval == 0 {
		hcs.config.Publisher.Interval = 2 * time.Minute
	}
	if hcs.config.Publisher.Timeout == 0 {
		hcs.config.Publisher.Timeout = 10 * time.Second
	}
	if hcs.config.Publisher.FailureThreshold == 0 {
		hcs.config.Publisher.FailureThreshold = 3
	}
	if hcs.config.Publisher.MessageTTL == 0 {
		hcs.config.Publisher.MessageTTL = 5 * time.Minute
	}

	// 订阅监控器默认值
	if hcs.config.Subscriber.MonitorInterval == 0 {
		hcs.config.Subscriber.MonitorInterval = 30 * time.Second
	}
	if hcs.config.Subscriber.WarningThreshold == 0 {
		hcs.config.Subscriber.WarningThreshold = 3
	}
	if hcs.config.Subscriber.ErrorThreshold == 0 {
		hcs.config.Subscriber.ErrorThreshold = 5
	}
	if hcs.config.Subscriber.CriticalThreshold == 0 {
		hcs.config.Subscriber.CriticalThreshold = 10
	}

	// 初始化统计信息
	hcs.stats.StartTime = time.Now()
	hcs.lastMessageTime.Store(time.Time{})

	return hcs
}

// Start 启动健康检查订阅监控
func (hcs *HealthCheckSubscriber) Start(ctx context.Context) error {
	if hcs.isRunning.Load() {
		return nil // 已经在运行
	}

	hcs.ctx, hcs.cancel = context.WithCancel(ctx)
	hcs.isRunning.Store(true)

	// 启动订阅健康检查主题
	if err := hcs.subscribeToHealthCheckTopic(); err != nil {
		hcs.isRunning.Store(false)
		return fmt.Errorf("failed to subscribe to health check topic: %w", err)
	}

	// 启动监控循环
	hcs.wg.Add(1)
	go hcs.monitoringLoop()

	logger.Info("Health check subscriber started",
		"source", hcs.source,
		"eventBusType", hcs.eventBusType,
		"topic", hcs.config.Subscriber.Topic,
		"expectedInterval", hcs.config.Publisher.Interval)

	return nil
}

// Stop 停止健康检查订阅监控
func (hcs *HealthCheckSubscriber) Stop() error {
	if !hcs.isRunning.Load() {
		return nil
	}

	hcs.cancel()
	hcs.wg.Wait()
	hcs.isRunning.Store(false)

	logger.Info("Health check subscriber stopped",
		"source", hcs.source,
		"eventBusType", hcs.eventBusType)

	return nil
}

// subscribeToHealthCheckTopic 订阅健康检查主题
func (hcs *HealthCheckSubscriber) subscribeToHealthCheckTopic() error {
	handler := func(ctx context.Context, data []byte) error {
		return hcs.handleHealthCheckMessage(ctx, data)
	}

	return hcs.eventBus.Subscribe(hcs.ctx, hcs.config.Subscriber.Topic, handler)
}

// handleHealthCheckMessage 处理健康检查消息
func (hcs *HealthCheckSubscriber) handleHealthCheckMessage(ctx context.Context, data []byte) error {
	// 解析健康检查消息
	healthMsg, err := hcs.parser.Parse(data)
	if err != nil {
		logger.Warn("Failed to parse health check message",
			"source", hcs.source,
			"error", err,
			"rawData", string(data))
		return nil // 不返回错误，避免影响其他消息处理
	}

	// 更新最后消息时间
	now := time.Now()
	hcs.lastMessageTime.Store(now)
	hcs.totalMessagesReceived.Add(1)

	// 重置连续错过计数
	hcs.consecutiveMisses.Store(0)

	// 更新统计信息
	hcs.updateStats(now, healthMsg)

	logger.Debug("Received health check message",
		"source", hcs.source,
		"messageSource", healthMsg.Source,
		"messageId", healthMsg.MessageID,
		"eventBusType", healthMsg.EventBusType,
		"timestamp", healthMsg.Timestamp)

	return nil
}

// monitoringLoop 监控循环
func (hcs *HealthCheckSubscriber) monitoringLoop() {
	defer hcs.wg.Done()

	// 使用配置的监控间隔，如果未设置则使用发布间隔的一半作为后备
	monitorInterval := hcs.config.Subscriber.MonitorInterval
	if monitorInterval == 0 {
		monitorInterval = hcs.config.Publisher.Interval / 2
	}

	// 最小监控间隔为100ms，避免过于频繁的检查
	if monitorInterval < 100*time.Millisecond {
		monitorInterval = 100 * time.Millisecond
	}

	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()

	logger.Debug("Health check monitoring loop started",
		"source", hcs.source,
		"monitorInterval", monitorInterval,
		"expectedInterval", hcs.config.Publisher.Interval)

	for {
		select {
		case <-hcs.ctx.Done():
			logger.Debug("Health check monitoring loop stopped", "source", hcs.source)
			return
		case <-ticker.C:
			hcs.checkHealthStatus()
		}
	}
}

// checkHealthStatus 检查健康状态
func (hcs *HealthCheckSubscriber) checkHealthStatus() {
	lastMsgTime := hcs.getLastMessageTime()
	now := time.Now()

	// 如果从未收到消息，且启动时间超过期望间隔，则告警
	if lastMsgTime.IsZero() {
		if time.Since(hcs.stats.StartTime) > hcs.config.Publisher.Interval {
			// 增加连续错过计数
			misses := hcs.consecutiveMisses.Add(1)

			// 根据连续错过次数确定告警级别
			severity := "warning"
			if misses >= int32(hcs.config.Publisher.FailureThreshold) {
				severity = "critical"
			} else if misses >= int32(hcs.config.Publisher.FailureThreshold)/2 {
				severity = "error"
			}

			hcs.triggerAlert("no_messages", severity,
				fmt.Sprintf("No health check messages received since startup (consecutive misses: %d)", misses), now, lastMsgTime)
		}
		return
	}

	// 检查消息是否超时
	timeSinceLastMsg := now.Sub(lastMsgTime)
	if timeSinceLastMsg > hcs.config.Publisher.Interval {
		// 增加连续错过计数
		misses := hcs.consecutiveMisses.Add(1)

		// 根据连续错过次数确定告警级别
		severity := "warning"
		if misses >= int32(hcs.config.Publisher.FailureThreshold) {
			severity = "critical"
		} else if misses >= int32(hcs.config.Publisher.FailureThreshold)/2 {
			severity = "error"
		}

		hcs.triggerAlert("no_messages", severity,
			fmt.Sprintf("No health check messages received for %v (consecutive misses: %d)",
				timeSinceLastMsg, misses), now, lastMsgTime)
	}
}

// triggerAlert 触发告警
func (hcs *HealthCheckSubscriber) triggerAlert(alertType, severity, message string, timestamp, lastMsgTime time.Time) {
	alert := HealthCheckAlert{
		AlertType:         alertType,
		Severity:          severity,
		Source:            hcs.source,
		EventBusType:      hcs.eventBusType,
		Topic:             hcs.config.Subscriber.Topic,
		LastMessageTime:   lastMsgTime,
		TimeSinceLastMsg:  timestamp.Sub(lastMsgTime),
		ExpectedInterval:  hcs.config.Publisher.Interval,
		ConsecutiveMisses: int(hcs.consecutiveMisses.Load()),
		Timestamp:         timestamp,
		Metadata: map[string]string{
			"message": message,
		},
	}

	// 更新统计信息
	hcs.statsMu.Lock()
	hcs.stats.TotalAlerts++
	hcs.stats.LastAlertTime = timestamp
	hcs.stats.IsHealthy = severity != "critical"
	hcs.statsMu.Unlock()

	// 记录日志
	logLevel := logger.Info
	if severity == "error" {
		logLevel = logger.Warn
	} else if severity == "critical" {
		logLevel = logger.Error
	}

	logLevel("Health check alert triggered",
		"source", hcs.source,
		"alertType", alertType,
		"severity", severity,
		"message", message,
		"timeSinceLastMsg", alert.TimeSinceLastMsg,
		"consecutiveMisses", alert.ConsecutiveMisses)

	// 调用告警回调
	hcs.notifyAlertCallbacks(alert)
}

// notifyAlertCallbacks 通知告警回调
func (hcs *HealthCheckSubscriber) notifyAlertCallbacks(alert HealthCheckAlert) {
	hcs.callbackMu.RLock()
	callbacks := make([]HealthCheckAlertCallback, len(hcs.alertCallbacks))
	copy(callbacks, hcs.alertCallbacks)
	hcs.callbackMu.RUnlock()

	// 从当前 context 派生，而不是使用 Background
	// 如果 ctx 为 nil，则使用 Background 作为后备
	parentCtx := hcs.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	for _, callback := range callbacks {
		go func(cb HealthCheckAlertCallback) {
			// 使用父 context 派生，支持取消传播
			ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
			defer cancel()

			if err := cb(ctx, alert); err != nil {
				logger.Error("Health check alert callback failed",
					"source", hcs.source,
					"alertType", alert.AlertType,
					"error", err)
			}
		}(callback)
	}
}

// RegisterAlertCallback 注册告警回调
func (hcs *HealthCheckSubscriber) RegisterAlertCallback(callback HealthCheckAlertCallback) error {
	hcs.callbackMu.Lock()
	defer hcs.callbackMu.Unlock()
	hcs.alertCallbacks = append(hcs.alertCallbacks, callback)
	return nil
}

// updateStats 更新统计信息
func (hcs *HealthCheckSubscriber) updateStats(now time.Time, healthMsg *HealthCheckMessage) {
	hcs.statsMu.Lock()
	defer hcs.statsMu.Unlock()

	hcs.stats.LastMessageTime = now
	hcs.stats.TotalMessagesReceived = hcs.totalMessagesReceived.Load()
	hcs.stats.ConsecutiveMisses = hcs.consecutiveMisses.Load()
	hcs.stats.IsHealthy = hcs.stats.ConsecutiveMisses < int32(hcs.config.Publisher.FailureThreshold)
	hcs.stats.UptimeSeconds = time.Since(hcs.stats.StartTime).Seconds()
}

// getLastMessageTime 获取最后消息时间
func (hcs *HealthCheckSubscriber) getLastMessageTime() time.Time {
	if t := hcs.lastMessageTime.Load(); t != nil {
		return t.(time.Time)
	}
	return time.Time{}
}

// GetStats 获取统计信息
func (hcs *HealthCheckSubscriber) GetStats() HealthCheckSubscriberStats {
	hcs.statsMu.RLock()
	defer hcs.statsMu.RUnlock()

	stats := hcs.stats
	stats.TotalMessagesReceived = hcs.totalMessagesReceived.Load()
	stats.ConsecutiveMisses = hcs.consecutiveMisses.Load()
	stats.UptimeSeconds = time.Since(hcs.stats.StartTime).Seconds()

	return stats
}

// IsHealthy 检查是否健康
func (hcs *HealthCheckSubscriber) IsHealthy() bool {
	misses := hcs.consecutiveMisses.Load()
	return misses < int32(hcs.config.Publisher.FailureThreshold)
}

// GetLastMessageTime 获取最后消息时间（公开方法）
func (hcs *HealthCheckSubscriber) GetLastMessageTime() time.Time {
	return hcs.getLastMessageTime()
}

// GetConsecutiveMisses 获取连续错过次数
func (hcs *HealthCheckSubscriber) GetConsecutiveMisses() int {
	return int(hcs.consecutiveMisses.Load())
}

// GetTotalMessagesReceived 获取总接收消息数
func (hcs *HealthCheckSubscriber) GetTotalMessagesReceived() int64 {
	return hcs.totalMessagesReceived.Load()
}
