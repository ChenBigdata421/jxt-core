package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// AdvancedSubscriber 高级订阅器
// 提供订阅端的统一管理和监控，但不替代现有的专门管理器
type AdvancedSubscriber struct {
	config           config.SubscriberConfig
	eventBus         EventBus
	
	// 订阅管理
	subscriptions    map[string]*SubscriptionInfo
	subscriptionsMu  sync.RWMutex
	
	// 状态管理
	isStarted        atomic.Bool
	startTime        time.Time
	
	// 统计信息
	totalSubscriptions    int32
	activeSubscriptions   int32
	messagesProcessed     int64
	processingErrors      int64
	
	// 组件引用（不拥有，只是引用）
	backlogDetector      *BacklogDetector
	recoveryManager      *RecoveryManager
	aggregateManager     *AggregateProcessorManager
	
	// 回调
	subscriptionCallbacks []SubscriptionCallback
	callbackMu           sync.RWMutex
}

// SubscriptionInfo 订阅信息
type SubscriptionInfo struct {
	Topic           string                 `json:"topic"`
	Handler         MessageHandler         `json:"-"`
	Options         SubscribeOptions       `json:"options"`
	StartTime       time.Time             `json:"startTime"`
	MessagesCount   int64                 `json:"messagesCount"`
	ErrorsCount     int64                 `json:"errorsCount"`
	LastMessageTime time.Time             `json:"lastMessageTime"`
	IsActive        bool                  `json:"isActive"`
}

// SubscriptionCallback 订阅回调
type SubscriptionCallback func(ctx context.Context, event SubscriptionEvent) error

// SubscriptionEvent 订阅事件
type SubscriptionEvent struct {
	Type         SubscriptionEventType `json:"type"`
	Topic        string               `json:"topic"`
	Timestamp    time.Time            `json:"timestamp"`
	Message      string               `json:"message"`
	Error        error                `json:"error,omitempty"`
}

// SubscriptionEventType 订阅事件类型
type SubscriptionEventType string

const (
	SubscriptionEventStarted SubscriptionEventType = "started"
	SubscriptionEventStopped SubscriptionEventType = "stopped"
	SubscriptionEventError   SubscriptionEventType = "error"
	SubscriptionEventMessage SubscriptionEventType = "message"
)

// NewAdvancedSubscriber 创建高级订阅器
func NewAdvancedSubscriber(config config.SubscriberConfig, eventBus EventBus) *AdvancedSubscriber {
	return &AdvancedSubscriber{
		config:        config,
		eventBus:      eventBus,
		subscriptions: make(map[string]*SubscriptionInfo),
	}
}

// Start 启动高级订阅器
func (as *AdvancedSubscriber) Start(ctx context.Context) error {
	if as.isStarted.Load() {
		return nil
	}
	
	as.startTime = time.Now()
	as.isStarted.Store(true)
	
	logger.Info("Advanced subscriber started")
	return nil
}

// Stop 停止高级订阅器
func (as *AdvancedSubscriber) Stop() error {
	if !as.isStarted.Load() {
		return nil
	}
	
	as.isStarted.Store(false)
	
	// 停止所有订阅
	as.subscriptionsMu.Lock()
	for topic, info := range as.subscriptions {
		info.IsActive = false
		as.notifySubscriptionEvent(SubscriptionEventStopped, topic, "Subscriber stopped", nil)
	}
	as.subscriptionsMu.Unlock()
	
	logger.Info("Advanced subscriber stopped")
	return nil
}

// Subscribe 订阅主题（包装基础订阅）
func (as *AdvancedSubscriber) Subscribe(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error {
	// 包装处理器以添加统计和回调
	wrappedHandler := as.wrapHandler(topic, handler)
	
	// 调用基础订阅
	err := as.eventBus.Subscribe(ctx, topic, wrappedHandler)
	if err != nil {
		as.notifySubscriptionEvent(SubscriptionEventError, topic, "Subscribe failed", err)
		return err
	}
	
	// 记录订阅信息
	as.subscriptionsMu.Lock()
	as.subscriptions[topic] = &SubscriptionInfo{
		Topic:       topic,
		Handler:     handler,
		Options:     opts,
		StartTime:   time.Now(),
		IsActive:    true,
	}
	as.subscriptionsMu.Unlock()
	
	atomic.AddInt32(&as.totalSubscriptions, 1)
	atomic.AddInt32(&as.activeSubscriptions, 1)
	
	as.notifySubscriptionEvent(SubscriptionEventStarted, topic, "Subscription started", nil)
	
	logger.Info("Subscription created", "topic", topic)
	return nil
}

// wrapHandler 包装处理器以添加统计和监控
func (as *AdvancedSubscriber) wrapHandler(topic string, handler MessageHandler) MessageHandler {
	return func(ctx context.Context, message []byte) error {
		start := time.Now()
		
		// 更新统计
		atomic.AddInt64(&as.messagesProcessed, 1)
		
		// 更新订阅信息
		as.updateSubscriptionStats(topic, true, time.Now())
		
		// 调用原始处理器
		err := handler(ctx, message)
		
		// 处理错误
		if err != nil {
			atomic.AddInt64(&as.processingErrors, 1)
			as.updateSubscriptionStats(topic, false, time.Now())
			as.notifySubscriptionEvent(SubscriptionEventError, topic, "Message processing failed", err)
		} else {
			as.notifySubscriptionEvent(SubscriptionEventMessage, topic, "Message processed", nil)
		}
		
		// 记录处理时间（可以添加到指标中）
		duration := time.Since(start)
		logger.Debug("Message processed", 
			"topic", topic,
			"duration", duration,
			"success", err == nil)
		
		return err
	}
}

// updateSubscriptionStats 更新订阅统计
func (as *AdvancedSubscriber) updateSubscriptionStats(topic string, success bool, timestamp time.Time) {
	as.subscriptionsMu.Lock()
	defer as.subscriptionsMu.Unlock()
	
	if info, exists := as.subscriptions[topic]; exists {
		info.LastMessageTime = timestamp
		if success {
			info.MessagesCount++
		} else {
			info.ErrorsCount++
		}
	}
}

// GetSubscriptionInfo 获取订阅信息
func (as *AdvancedSubscriber) GetSubscriptionInfo(topic string) (*SubscriptionInfo, bool) {
	as.subscriptionsMu.RLock()
	defer as.subscriptionsMu.RUnlock()
	
	info, exists := as.subscriptions[topic]
	if !exists {
		return nil, false
	}
	
	// 返回副本
	infoCopy := *info
	return &infoCopy, true
}

// GetAllSubscriptions 获取所有订阅信息
func (as *AdvancedSubscriber) GetAllSubscriptions() map[string]*SubscriptionInfo {
	as.subscriptionsMu.RLock()
	defer as.subscriptionsMu.RUnlock()
	
	result := make(map[string]*SubscriptionInfo)
	for topic, info := range as.subscriptions {
		infoCopy := *info
		result[topic] = &infoCopy
	}
	
	return result
}

// GetStats 获取订阅器统计信息
func (as *AdvancedSubscriber) GetStats() SubscriberStats {
	return SubscriberStats{
		IsStarted:           as.isStarted.Load(),
		StartTime:           as.startTime,
		TotalSubscriptions:  atomic.LoadInt32(&as.totalSubscriptions),
		ActiveSubscriptions: atomic.LoadInt32(&as.activeSubscriptions),
		MessagesProcessed:   atomic.LoadInt64(&as.messagesProcessed),
		ProcessingErrors:    atomic.LoadInt64(&as.processingErrors),
		Uptime:             time.Since(as.startTime),
	}
}

// SubscriberStats 订阅器统计信息
type SubscriberStats struct {
	IsStarted           bool          `json:"isStarted"`
	StartTime           time.Time     `json:"startTime"`
	TotalSubscriptions  int32         `json:"totalSubscriptions"`
	ActiveSubscriptions int32         `json:"activeSubscriptions"`
	MessagesProcessed   int64         `json:"messagesProcessed"`
	ProcessingErrors    int64         `json:"processingErrors"`
	Uptime             time.Duration `json:"uptime"`
}

// RegisterSubscriptionCallback 注册订阅回调
func (as *AdvancedSubscriber) RegisterSubscriptionCallback(callback SubscriptionCallback) error {
	as.callbackMu.Lock()
	defer as.callbackMu.Unlock()
	as.subscriptionCallbacks = append(as.subscriptionCallbacks, callback)
	return nil
}

// notifySubscriptionEvent 通知订阅事件
func (as *AdvancedSubscriber) notifySubscriptionEvent(eventType SubscriptionEventType, topic, message string, err error) {
	as.callbackMu.RLock()
	callbacks := make([]SubscriptionCallback, len(as.subscriptionCallbacks))
	copy(callbacks, as.subscriptionCallbacks)
	as.callbackMu.RUnlock()
	
	event := SubscriptionEvent{
		Type:      eventType,
		Topic:     topic,
		Timestamp: time.Now(),
		Message:   message,
		Error:     err,
	}
	
	for _, callback := range callbacks {
		go func(cb SubscriptionCallback) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			
			if err := cb(ctx, event); err != nil {
				logger.Error("Subscription callback failed", 
					"eventType", eventType,
					"topic", topic,
					"error", err)
			}
		}(callback)
	}
}

// SetBacklogDetector 设置积压检测器引用
func (as *AdvancedSubscriber) SetBacklogDetector(detector *BacklogDetector) {
	as.backlogDetector = detector
}

// SetRecoveryManager 设置恢复管理器引用
func (as *AdvancedSubscriber) SetRecoveryManager(manager *RecoveryManager) {
	as.recoveryManager = manager
}

// SetAggregateManager 设置聚合管理器引用
func (as *AdvancedSubscriber) SetAggregateManager(manager *AggregateProcessorManager) {
	as.aggregateManager = manager
}

// GetBacklogDetector 获取积压检测器
func (as *AdvancedSubscriber) GetBacklogDetector() *BacklogDetector {
	return as.backlogDetector
}

// GetRecoveryManager 获取恢复管理器
func (as *AdvancedSubscriber) GetRecoveryManager() *RecoveryManager {
	return as.recoveryManager
}

// GetAggregateManager 获取聚合管理器
func (as *AdvancedSubscriber) GetAggregateManager() *AggregateProcessorManager {
	return as.aggregateManager
}

// IsStarted 检查是否已启动
func (as *AdvancedSubscriber) IsStarted() bool {
	return as.isStarted.Load()
}
