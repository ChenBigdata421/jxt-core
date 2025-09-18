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

// AdvancedPublisher 高级发布管理器
type AdvancedPublisher struct {
	config           config.PublisherConfig
	eventBus         EventBus
	
	// 连接管理
	isConnected      atomic.Bool
	
	// 重连管理
	backoff          time.Duration
	failureCount     int32
	
	// 回调管理
	reconnectCallbacks []ReconnectCallback
	publishCallbacks   []PublishCallback
	callbackMu         sync.RWMutex
	
	// 消息格式化
	messageFormatter MessageFormatter
	formatterMu      sync.RWMutex
	
	// 生命周期
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	
	// UUID生成器
	uuidGenerator    UUIDGenerator
}

// UUIDGenerator UUID生成器接口
type UUIDGenerator interface {
	Generate() string
}

// DefaultUUIDGenerator 默认UUID生成器
type DefaultUUIDGenerator struct{}

// Generate 生成UUID
func (g *DefaultUUIDGenerator) Generate() string {
	return fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), time.Now().Unix())
}

// NewAdvancedPublisher 创建高级发布管理器
func NewAdvancedPublisher(config config.PublisherConfig, eventBus EventBus) *AdvancedPublisher {
	ap := &AdvancedPublisher{
		config:           config,
		eventBus:         eventBus,
		backoff:          config.InitialBackoff,
		messageFormatter: &DefaultMessageFormatter{},
		uuidGenerator:    &DefaultUUIDGenerator{},
	}
	
	// 设置默认配置
	if ap.config.InitialBackoff == 0 {
		ap.config.InitialBackoff = 1 * time.Second
	}
	if ap.config.MaxBackoff == 0 {
		ap.config.MaxBackoff = 1 * time.Minute
	}
	if ap.config.MaxReconnectAttempts == 0 {
		ap.config.MaxReconnectAttempts = 5
	}
	if ap.config.PublishTimeout == 0 {
		ap.config.PublishTimeout = 30 * time.Second
	}
	
	ap.backoff = ap.config.InitialBackoff
	
	return ap
}

// Start 启动发布管理器
func (ap *AdvancedPublisher) Start(ctx context.Context) error {
	ap.ctx, ap.cancel = context.WithCancel(ctx)
	ap.isConnected.Store(true)
	
	logger.Info("Advanced publisher started", 
		"maxReconnectAttempts", ap.config.MaxReconnectAttempts,
		"publishTimeout", ap.config.PublishTimeout)
	
	return nil
}

// Stop 停止发布管理器
func (ap *AdvancedPublisher) Stop() error {
	if ap.cancel != nil {
		ap.cancel()
	}
	ap.wg.Wait()
	ap.isConnected.Store(false)
	
	logger.Info("Advanced publisher stopped")
	return nil
}

// PublishWithOptions 使用选项发布消息
func (ap *AdvancedPublisher) PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error {
	// 选择消息格式化器
	formatter := opts.MessageFormatter
	if formatter == nil {
		ap.formatterMu.RLock()
		formatter = ap.messageFormatter
		ap.formatterMu.RUnlock()
	}
	
	// 生成消息UUID
	uuid := ap.uuidGenerator.Generate()
	
	// 格式化消息
	msg, err := formatter.FormatMessage(uuid, opts.AggregateID, message)
	if err != nil {
		publishErr := fmt.Errorf("message formatting failed: %w", err)
		ap.notifyPublishCallbacks(ctx, topic, message, publishErr)
		return publishErr
	}
	
	// 设置额外的元数据
	if opts.Metadata != nil {
		if err := formatter.SetMetadata(msg, opts.Metadata); err != nil {
			publishErr := fmt.Errorf("failed to set metadata: %w", err)
			ap.notifyPublishCallbacks(ctx, topic, message, publishErr)
			return publishErr
		}
	}
	
	// 序列化消息
	msgBytes, err := ap.serializeMessage(msg)
	if err != nil {
		publishErr := fmt.Errorf("message serialization failed: %w", err)
		ap.notifyPublishCallbacks(ctx, topic, message, publishErr)
		return publishErr
	}
	
	// 设置超时
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = ap.config.PublishTimeout
	}
	
	publishCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	// 发布消息（带重试）
	err = ap.publishWithRetry(publishCtx, topic, msgBytes, opts.RetryPolicy)
	
	// 通知回调
	ap.notifyPublishCallbacks(ctx, topic, message, err)
	
	if err != nil {
		ap.handlePublishFailure(err)
		return err
	}
	
	ap.handlePublishSuccess()
	return nil
}

// publishWithRetry 带重试的发布
func (ap *AdvancedPublisher) publishWithRetry(ctx context.Context, topic string, message []byte, retryPolicy RetryPolicy) error {
	var lastErr error
	
	maxRetries := retryPolicy.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3 // 默认重试3次
	}
	
	interval := retryPolicy.InitialInterval
	if interval == 0 {
		interval = 1 * time.Second
	}
	
	maxInterval := retryPolicy.MaxInterval
	if maxInterval == 0 {
		maxInterval = 30 * time.Second
	}
	
	multiplier := retryPolicy.Multiplier
	if multiplier == 0 {
		multiplier = 2.0
	}
	
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 尝试发布
		err := ap.eventBus.Publish(ctx, topic, message)
		if err == nil {
			if attempt > 0 {
				logger.Info("Publish succeeded after retry", 
					"topic", topic, 
					"attempt", attempt,
					"totalAttempts", attempt+1)
			}
			return nil
		}
		
		lastErr = err
		
		// 如果是最后一次尝试，直接返回错误
		if attempt == maxRetries {
			break
		}
		
		// 等待重试间隔
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		
		// 增加重试间隔
		interval = time.Duration(float64(interval) * multiplier)
		if interval > maxInterval {
			interval = maxInterval
		}
		
		logger.Warn("Publish failed, retrying", 
			"topic", topic,
			"attempt", attempt+1,
			"error", err,
			"nextRetryIn", interval)
	}
	
	return fmt.Errorf("publish failed after %d attempts: %w", maxRetries+1, lastErr)
}

// SetMessageFormatter 设置消息格式化器
func (ap *AdvancedPublisher) SetMessageFormatter(formatter MessageFormatter) error {
	ap.formatterMu.Lock()
	defer ap.formatterMu.Unlock()
	ap.messageFormatter = formatter
	
	logger.Info("Message formatter updated", "formatterType", fmt.Sprintf("%T", formatter))
	return nil
}

// RegisterReconnectCallback 注册重连回调
func (ap *AdvancedPublisher) RegisterReconnectCallback(callback ReconnectCallback) error {
	ap.callbackMu.Lock()
	defer ap.callbackMu.Unlock()
	ap.reconnectCallbacks = append(ap.reconnectCallbacks, callback)
	return nil
}

// RegisterPublishCallback 注册发布回调
func (ap *AdvancedPublisher) RegisterPublishCallback(callback PublishCallback) error {
	ap.callbackMu.Lock()
	defer ap.callbackMu.Unlock()
	ap.publishCallbacks = append(ap.publishCallbacks, callback)
	return nil
}

// handlePublishSuccess 处理发布成功
func (ap *AdvancedPublisher) handlePublishSuccess() {
	atomic.StoreInt32(&ap.failureCount, 0)
	ap.backoff = ap.config.InitialBackoff
}

// handlePublishFailure 处理发布失败
func (ap *AdvancedPublisher) handlePublishFailure(err error) {
	failures := atomic.AddInt32(&ap.failureCount, 1)
	
	// 如果连续失败次数达到阈值，触发重连
	if failures >= 3 {
		go ap.attemptReconnect()
	}
}

// attemptReconnect 尝试重连
func (ap *AdvancedPublisher) attemptReconnect() {
	logger.Info("Starting reconnection attempt", "failureCount", atomic.LoadInt32(&ap.failureCount))
	
	for i := 0; i < ap.config.MaxReconnectAttempts; i++ {
		select {
		case <-ap.ctx.Done():
			return
		case <-time.After(ap.backoff):
		}
		
		logger.Info("Attempting to reconnect", "attempt", i+1, "backoff", ap.backoff)
		
		// 这里应该调用具体的重连逻辑
		// 对于当前的设计，我们假设重连成功
		if ap.isReconnectSuccessful() {
			logger.Info("Reconnection successful", "attempt", i+1)
			ap.notifyReconnectCallbacks()
			atomic.StoreInt32(&ap.failureCount, 0)
			ap.backoff = ap.config.InitialBackoff
			return
		}
		
		// 更新退避时间
		ap.backoff *= 2
		if ap.backoff > ap.config.MaxBackoff {
			ap.backoff = ap.config.MaxBackoff
		}
		
		logger.Warn("Reconnection failed", "attempt", i+1, "nextBackoff", ap.backoff)
	}
	
	logger.Error("All reconnection attempts failed", "maxAttempts", ap.config.MaxReconnectAttempts)
}

// isReconnectSuccessful 检查重连是否成功
func (ap *AdvancedPublisher) isReconnectSuccessful() bool {
	// 这里应该实现具体的重连检查逻辑
	// 比如发送一个测试消息
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	testMessage := []byte("reconnect-test")
	err := ap.eventBus.Publish(ctx, "test-topic", testMessage)
	return err == nil
}

// notifyReconnectCallbacks 通知重连回调
func (ap *AdvancedPublisher) notifyReconnectCallbacks() {
	ap.callbackMu.RLock()
	callbacks := make([]ReconnectCallback, len(ap.reconnectCallbacks))
	copy(callbacks, ap.reconnectCallbacks)
	ap.callbackMu.RUnlock()
	
	for _, callback := range callbacks {
		go func(cb ReconnectCallback) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			
			if err := cb(ctx); err != nil {
				logger.Error("Reconnect callback failed", "error", err)
			}
		}(callback)
	}
}

// notifyPublishCallbacks 通知发布回调
func (ap *AdvancedPublisher) notifyPublishCallbacks(ctx context.Context, topic string, message []byte, err error) {
	ap.callbackMu.RLock()
	callbacks := make([]PublishCallback, len(ap.publishCallbacks))
	copy(callbacks, ap.publishCallbacks)
	ap.callbackMu.RUnlock()
	
	for _, callback := range callbacks {
		go func(cb PublishCallback) {
			callbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			
			if callbackErr := cb(callbackCtx, topic, message, err); callbackErr != nil {
				logger.Error("Publish callback failed", "error", callbackErr)
			}
		}(callback)
	}
}

// serializeMessage 序列化消息
func (ap *AdvancedPublisher) serializeMessage(msg *Message) ([]byte, error) {
	// 这里应该根据具体的事件总线实现来序列化消息
	// 对于简单实现，直接返回payload
	return msg.Payload, nil
}

// GetConnectionState 获取连接状态
func (ap *AdvancedPublisher) GetConnectionState() ConnectionState {
	return ConnectionState{
		IsConnected:    ap.isConnected.Load(),
		ReconnectCount: int(atomic.LoadInt32(&ap.failureCount)),
	}
}

// SetUUIDGenerator 设置UUID生成器
func (ap *AdvancedPublisher) SetUUIDGenerator(generator UUIDGenerator) {
	ap.uuidGenerator = generator
}
