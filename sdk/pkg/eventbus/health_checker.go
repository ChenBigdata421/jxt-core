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

// HealthChecker 统一健康检查器
type HealthChecker struct {
	config       config.HealthCheckConfig
	eventBus     EventBus
	source       string // 微服务名称
	eventBusType string // 事件总线类型
	
	// 状态管理
	isRunning       atomic.Bool
	consecutiveFailures int32
	lastSuccessTime atomic.Value // time.Time
	lastFailureTime atomic.Value // time.Time
	
	// 控制
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	
	// 回调
	callbacks       []HealthCheckCallback
	callbackMu      sync.RWMutex
	
	// 消息解析器
	parser          *HealthCheckMessageParser
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(config config.HealthCheckConfig, eventBus EventBus, source, eventBusType string) *HealthChecker {
	hc := &HealthChecker{
		config:       config,
		eventBus:     eventBus,
		source:       source,
		eventBusType: eventBusType,
		parser:       NewHealthCheckMessageParser(),
	}
	
	// 初始化时间
	hc.lastSuccessTime.Store(time.Now())
	hc.lastFailureTime.Store(time.Time{})
	
	// 设置默认配置
	if hc.config.Topic == "" {
		hc.config.Topic = GetHealthCheckTopic(eventBusType)
	}
	if hc.config.Interval == 0 {
		hc.config.Interval = 2 * time.Minute
	}
	if hc.config.Timeout == 0 {
		hc.config.Timeout = 10 * time.Second
	}
	if hc.config.FailureThreshold == 0 {
		hc.config.FailureThreshold = 3
	}
	if hc.config.MessageTTL == 0 {
		hc.config.MessageTTL = 5 * time.Minute
	}
	
	return hc
}

// Start 启动健康检查
func (hc *HealthChecker) Start(ctx context.Context) error {
	if hc.isRunning.Load() {
		return nil // 已经在运行
	}
	
	if !hc.config.Enabled {
		logger.Info("Health check is disabled", "source", hc.source)
		return nil
	}
	
	hc.ctx, hc.cancel = context.WithCancel(ctx)
	hc.isRunning.Store(true)
	
	hc.wg.Add(1)
	go hc.healthCheckLoop()
	
	logger.Info("Health checker started", 
		"source", hc.source, 
		"eventBusType", hc.eventBusType,
		"topic", hc.config.Topic,
		"interval", hc.config.Interval)
	
	return nil
}

// Stop 停止健康检查
func (hc *HealthChecker) Stop() error {
	if !hc.isRunning.Load() {
		return nil
	}
	
	hc.cancel()
	hc.wg.Wait()
	hc.isRunning.Store(false)
	
	logger.Info("Health checker stopped", "source", hc.source)
	return nil
}

// healthCheckLoop 健康检查循环
func (hc *HealthChecker) healthCheckLoop() {
	defer hc.wg.Done()
	
	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()
	
	// 立即执行一次健康检查
	hc.performHealthCheck()
	
	for {
		select {
		case <-hc.ctx.Done():
			return
		case <-ticker.C:
			hc.performHealthCheck()
		}
	}
}

// performHealthCheck 执行健康检查
func (hc *HealthChecker) performHealthCheck() {
	start := time.Now()
	
	// 创建标准健康检查消息
	healthMsg := NewHealthCheckMessageBuilder(hc.source, hc.eventBusType).
		WithCheckType("periodic").
		WithInstanceID(hc.generateInstanceID()).
		Build()
	
	// 序列化消息
	msgBytes, err := healthMsg.ToBytes()
	if err != nil {
		hc.recordFailure(fmt.Errorf("failed to serialize health check message: %w", err), start)
		return
	}
	
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(hc.ctx, hc.config.Timeout)
	defer cancel()
	
	// 发布健康检查消息到统一主题
	err = hc.eventBus.Publish(ctx, hc.config.Topic, msgBytes)
	
	if err != nil {
		hc.recordFailure(fmt.Errorf("health check publish failed: %w", err), start)
	} else {
		hc.recordSuccess(start)
	}
}

// recordSuccess 记录成功
func (hc *HealthChecker) recordSuccess(startTime time.Time) {
	atomic.StoreInt32(&hc.consecutiveFailures, 0)
	hc.lastSuccessTime.Store(startTime)
	
	result := HealthCheckResult{
		Success:             true,
		Timestamp:           startTime,
		Duration:            time.Since(startTime),
		ConsecutiveFailures: 0,
		EventBusType:        hc.eventBusType,
		Source:              hc.source,
	}
	
	logger.Debug("Health check succeeded", 
		"source", hc.source,
		"duration", result.Duration,
		"topic", hc.config.Topic)
	
	hc.notifyCallbacks(result)
}

// recordFailure 记录失败
func (hc *HealthChecker) recordFailure(err error, startTime time.Time) {
	failures := atomic.AddInt32(&hc.consecutiveFailures, 1)
	hc.lastFailureTime.Store(startTime)
	
	result := HealthCheckResult{
		Success:             false,
		Timestamp:           startTime,
		Duration:            time.Since(startTime),
		Error:               err,
		ConsecutiveFailures: int(failures),
		EventBusType:        hc.eventBusType,
		Source:              hc.source,
	}
	
	logger.Error("Health check failed", 
		"source", hc.source,
		"error", err,
		"consecutiveFailures", failures,
		"duration", result.Duration)
	
	hc.notifyCallbacks(result)
}

// RegisterCallback 注册回调
func (hc *HealthChecker) RegisterCallback(callback HealthCheckCallback) error {
	hc.callbackMu.Lock()
	defer hc.callbackMu.Unlock()
	hc.callbacks = append(hc.callbacks, callback)
	return nil
}

// notifyCallbacks 通知回调
func (hc *HealthChecker) notifyCallbacks(result HealthCheckResult) {
	hc.callbackMu.RLock()
	callbacks := make([]HealthCheckCallback, len(hc.callbacks))
	copy(callbacks, hc.callbacks)
	hc.callbackMu.RUnlock()
	
	for _, callback := range callbacks {
		go func(cb HealthCheckCallback) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			
			if err := cb(ctx, result); err != nil {
				logger.Error("Health check callback failed", 
					"source", hc.source,
					"error", err)
			}
		}(callback)
	}
}

// GetStatus 获取健康状态
func (hc *HealthChecker) GetStatus() HealthCheckStatus {
	failures := atomic.LoadInt32(&hc.consecutiveFailures)
	lastSuccess := hc.lastSuccessTime.Load().(time.Time)
	lastFailure := hc.lastFailureTime.Load().(time.Time)
	
	return HealthCheckStatus{
		IsHealthy:           failures < int32(hc.config.FailureThreshold),
		ConsecutiveFailures: int(failures),
		LastSuccessTime:     lastSuccess,
		LastFailureTime:     lastFailure,
		IsRunning:           hc.isRunning.Load(),
		EventBusType:        hc.eventBusType,
		Source:              hc.source,
	}
}

// IsHealthy 检查是否健康
func (hc *HealthChecker) IsHealthy() bool {
	failures := atomic.LoadInt32(&hc.consecutiveFailures)
	return failures < int32(hc.config.FailureThreshold)
}

// GetConsecutiveFailures 获取连续失败次数
func (hc *HealthChecker) GetConsecutiveFailures() int {
	return int(atomic.LoadInt32(&hc.consecutiveFailures))
}

// GetLastSuccessTime 获取最后成功时间
func (hc *HealthChecker) GetLastSuccessTime() time.Time {
	return hc.lastSuccessTime.Load().(time.Time)
}

// GetLastFailureTime 获取最后失败时间
func (hc *HealthChecker) GetLastFailureTime() time.Time {
	return hc.lastFailureTime.Load().(time.Time)
}

// generateInstanceID 生成实例ID
func (hc *HealthChecker) generateInstanceID() string {
	// 这里可以使用更复杂的实例ID生成逻辑
	// 比如结合主机名、进程ID等
	return fmt.Sprintf("%s-%d", hc.source, time.Now().Unix())
}

// GetDefaultHealthCheckConfig 获取默认健康检查配置
func GetDefaultHealthCheckConfig() config.HealthCheckConfig {
	return config.HealthCheckConfig{
		Enabled:          true,
		Topic:            DefaultHealthCheckTopic,
		Interval:         2 * time.Minute,
		Timeout:          10 * time.Second,
		FailureThreshold: 3,
		MessageTTL:       5 * time.Minute,
	}
}
