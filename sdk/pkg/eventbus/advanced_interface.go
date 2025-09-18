package eventbus

import (
	"context"
	"time"
)

// AdvancedEventBus 高级事件总线接口
type AdvancedEventBus interface {
	EventBus // 继承基础接口

	// 生命周期管理
	Start(ctx context.Context) error
	Stop() error

	// 订阅端高级功能
	SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error
	SetRecoveryMode(enabled bool) error
	IsInRecoveryMode() bool
	RegisterBacklogCallback(callback BacklogStateCallback) error
	StartBacklogMonitoring(ctx context.Context) error
	StopBacklogMonitoring() error
	SetMessageRouter(router MessageRouter) error
	SetErrorHandler(handler ErrorHandler) error

	// 订阅器管理
	GetAdvancedSubscriber() *AdvancedSubscriber
	RegisterSubscriptionCallback(callback SubscriptionCallback) error

	// 发布端高级功能
	PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error
	SetMessageFormatter(formatter MessageFormatter) error
	RegisterPublishCallback(callback PublishCallback) error

	// 统一功能
	StartHealthCheck(ctx context.Context) error
	StopHealthCheck() error
	GetHealthStatus() HealthCheckStatus
	RegisterHealthCheckCallback(callback HealthCheckCallback) error
	GetConnectionState() ConnectionState
}

// SubscribeOptions 订阅选项
type SubscribeOptions struct {
	UseAggregateProcessor bool          `json:"useAggregateProcessor"` // 是否使用聚合处理器
	ProcessingTimeout     time.Duration `json:"processingTimeout"`     // 处理超时时间
	RateLimit             float64       `json:"rateLimit"`             // 速率限制（每秒）
	RateBurst             int           `json:"rateBurst"`             // 突发大小
	MaxRetries            int           `json:"maxRetries"`            // 最大重试次数
	RetryBackoff          time.Duration `json:"retryBackoff"`          // 重试退避时间
}

// PublishOptions 发布选项
type PublishOptions struct {
	MessageFormatter MessageFormatter  `json:"-"`           // 消息格式化器
	Metadata         map[string]string `json:"metadata"`    // 元数据
	RetryPolicy      RetryPolicy       `json:"retryPolicy"` // 重试策略
	Timeout          time.Duration     `json:"timeout"`     // 超时时间
	AggregateID      interface{}       `json:"aggregateId"` // 聚合ID
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries      int           `json:"maxRetries"`      // 最大重试次数
	InitialInterval time.Duration `json:"initialInterval"` // 初始间隔
	MaxInterval     time.Duration `json:"maxInterval"`     // 最大间隔
	Multiplier      float64       `json:"multiplier"`      // 倍数
}

// ConnectionState 连接状态
type ConnectionState struct {
	IsConnected       bool      `json:"isConnected"`
	LastConnectedTime time.Time `json:"lastConnectedTime"`
	ReconnectCount    int       `json:"reconnectCount"`
	LastError         string    `json:"lastError,omitempty"`
}

// HealthCheckStatus 健康检查状态
type HealthCheckStatus struct {
	IsHealthy           bool      `json:"isHealthy"`
	ConsecutiveFailures int       `json:"consecutiveFailures"`
	LastSuccessTime     time.Time `json:"lastSuccessTime"`
	LastFailureTime     time.Time `json:"lastFailureTime"`
	IsRunning           bool      `json:"isRunning"`
	EventBusType        string    `json:"eventBusType"`
	Source              string    `json:"source"`
}

// 回调函数类型
type BacklogStateCallback func(ctx context.Context, state BacklogState) error
type HealthCheckCallback func(ctx context.Context, result HealthCheckResult) error
type ReconnectCallback func(ctx context.Context) error
type PublishCallback func(ctx context.Context, topic string, message []byte, err error) error

// BacklogState 积压状态
type BacklogState struct {
	HasBacklog    bool          `json:"hasBacklog"`
	LagCount      int64         `json:"lagCount"`
	LagTime       time.Duration `json:"lagTime"`
	Timestamp     time.Time     `json:"timestamp"`
	Topic         string        `json:"topic"`
	ConsumerGroup string        `json:"consumerGroup"`
}

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	Success             bool          `json:"success"`
	Timestamp           time.Time     `json:"timestamp"`
	Duration            time.Duration `json:"duration"`
	Error               error         `json:"error,omitempty"`
	ConsecutiveFailures int           `json:"consecutiveFailures"`
	EventBusType        string        `json:"eventBusType"`
	Source              string        `json:"source"`
}

// MessageRouter 消息路由器接口
type MessageRouter interface {
	// Route 根据消息内容决定路由策略
	Route(ctx context.Context, topic string, message []byte) (RouteDecision, error)
}

// RouteDecision 路由决策
type RouteDecision struct {
	ShouldProcess bool              `json:"shouldProcess"` // 是否应该处理这个消息
	AggregateID   string            `json:"aggregateId"`   // 聚合ID（用于聚合处理器）
	ProcessorKey  string            `json:"processorKey"`  // 处理器键（用于负载均衡）
	Priority      int               `json:"priority"`      // 优先级
	Metadata      map[string]string `json:"metadata"`      // 额外的元数据
}

// ErrorHandler 错误处理器接口
type ErrorHandler interface {
	// HandleError 处理错误
	HandleError(ctx context.Context, err error, message []byte, topic string) ErrorAction
}

// ErrorAction 错误处理动作
type ErrorAction struct {
	Action      ErrorActionType   `json:"action"`      // 动作类型
	RetryAfter  time.Duration     `json:"retryAfter"`  // 重试延迟
	DeadLetter  bool              `json:"deadLetter"`  // 是否发送到死信队列
	SkipMessage bool              `json:"skipMessage"` // 是否跳过消息
	Metadata    map[string]string `json:"metadata"`    // 额外的元数据
}

// ErrorActionType 错误动作类型
type ErrorActionType string

const (
	ErrorActionRetry      ErrorActionType = "retry"      // 重试
	ErrorActionDeadLetter ErrorActionType = "deadletter" // 死信队列
	ErrorActionSkip       ErrorActionType = "skip"       // 跳过
	ErrorActionFail       ErrorActionType = "fail"       // 失败
)

// MessageFormatter 消息格式化器接口
type MessageFormatter interface {
	// FormatMessage 格式化消息
	FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error)
	// ExtractAggregateID 提取聚合ID
	ExtractAggregateID(aggregateID interface{}) string
	// SetMetadata 设置元数据
	SetMetadata(msg *Message, metadata map[string]string) error
}

// Message 消息结构
type Message struct {
	UUID     string            `json:"uuid"`
	Payload  []byte            `json:"payload"`
	Metadata map[string]string `json:"metadata"`
}

// NewMessage 创建新消息
func NewMessage(uuid string, payload []byte) *Message {
	return &Message{
		UUID:     uuid,
		Payload:  payload,
		Metadata: make(map[string]string),
	}
}

// GetDefaultSubscribeOptions 获取默认订阅选项
func GetDefaultSubscribeOptions() SubscribeOptions {
	return SubscribeOptions{
		UseAggregateProcessor: false,
		ProcessingTimeout:     30 * time.Second,
		RateLimit:             1000,
		RateBurst:             1000,
		MaxRetries:            3,
		RetryBackoff:          1 * time.Second,
	}
}

// GetDefaultPublishOptions 获取默认发布选项
func GetDefaultPublishOptions() PublishOptions {
	return PublishOptions{
		Timeout: 30 * time.Second,
		RetryPolicy: RetryPolicy{
			MaxRetries:      3,
			InitialInterval: 1 * time.Second,
			MaxInterval:     30 * time.Second,
			Multiplier:      2.0,
		},
		Metadata: make(map[string]string),
	}
}
