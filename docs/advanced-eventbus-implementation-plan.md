# 高级事件总线实施方案

## 概述

本文档描述了将 evidence-management 中的 `KafkaSubscriberManager` 和 `KafkaPublisherManager` 高级功能迁移到 jxt-core 的具体实施方案。目标是创建一个功能完整的高级事件总线，提供积压检测、恢复模式、聚合处理器、统一健康检查等企业级特性。

## 目标架构

### 分层设计原则

```
┌─────────────────────────────────────────┐
│           业务微服务层                    │
│  ┌─────────────────────────────────────┐ │
│  │     业务消息处理逻辑                 │ │
│  │   - MediaEventHandler              │ │
│  │   - ArchiveEventHandler            │ │
│  │   - 业务事件发布逻辑                │ │
│  │   - 业务特定配置                    │ │
│  │   - Outbox 模式集成                │ │
│  └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│            jxt-core 层                  │
│  ┌─────────────────────────────────────┐ │
│  │      高级事件总线                    │ │
│  │   - 积压检测                        │ │
│  │   - 恢复模式管理                    │ │
│  │   - 聚合处理器框架                  │ │
│  │   - 流量控制                        │ │
│  │   - 统一健康检查                    │ │
│  │   - 重连机制                        │ │
│  │   - 发布端管理                      │ │
│  │   - 订阅端管理                      │ │
│  └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
```

## 实施阶段

### 阶段 1：接口设计和基础框架

#### 1.1 扩展 EventBus 接口

**文件**: `jxt-core/sdk/pkg/eventbus/advanced_interface.go`

```go
package eventbus

import (
    "context"
    "time"
    "golang.org/x/time/rate"
)

// AdvancedEventBus 高级事件总线接口
type AdvancedEventBus interface {
    EventBus // 继承基础接口
    
    // 高级订阅功能
    SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error
    
    // 恢复模式管理
    SetRecoveryMode(enabled bool) error
    IsInRecoveryMode() bool
    RegisterRecoveryModeCallback(callback RecoveryModeCallback) error
    
    // 积压检测
    RegisterBacklogCallback(callback BacklogStateCallback) error
    StartBacklogMonitoring(ctx context.Context) error
    StopBacklogMonitoring() error
    GetBacklogState(ctx context.Context) (*BacklogState, error)
    
    // 聚合处理器管理
    SetMessageRouter(router MessageRouter) error
    SetErrorHandler(handler ErrorHandler) error
    GetProcessorStats() ProcessorStats
    
    // 流量控制
    SetRateLimit(limit rate.Limit, burst int) error
    GetRateLimit() (rate.Limit, int)
}

// 订阅选项
type SubscribeOptions struct {
    UseAggregateProcessor bool          // 是否使用聚合处理器
    ProcessingTimeout     time.Duration // 处理超时时间
    RateLimit            rate.Limit     // 速率限制
    RateBurst            int            // 突发容量
    RetryPolicy          RetryPolicy    // 重试策略
    DeadLetterEnabled    bool           // 是否启用死信队列
}

// 重试策略
type RetryPolicy struct {
    MaxRetries      int           // 最大重试次数
    InitialInterval time.Duration // 初始重试间隔
    MaxInterval     time.Duration // 最大重试间隔
    Multiplier      float64       // 退避倍数
}

// 积压状态
type BacklogState struct {
    HasBacklog       bool              // 是否有积压
    TotalLag         int64             // 总积压数量
    AffectedTopics   []string          // 受影响的主题
    ActiveProcessors int               // 活跃处理器数量
    TotalProcessors  int               // 总处理器数量
    Timestamp        time.Time         // 检测时间
    Details          map[string]int64  // 详细信息
}

// 处理器统计
type ProcessorStats struct {
    TotalProcessors  int64 // 总处理器数量
    ActiveProcessors int64 // 活跃处理器数量
    IdleProcessors   int64 // 空闲处理器数量
    ProcessorDetails map[string]ProcessorDetail // 处理器详情
}

type ProcessorDetail struct {
    AggregateID    string    // 聚合ID
    LastActivity   time.Time // 最后活动时间
    MessageCount   int64     // 处理的消息数量
    IsActive       bool      // 是否活跃
}

// 回调函数类型
type BacklogStateCallback func(ctx context.Context, state BacklogState) error
type RecoveryModeCallback func(ctx context.Context, isRecovery bool) error

// 消息路由器接口
type MessageRouter interface {
    ShouldUseAggregateProcessor(msg MessageContext) bool
    ExtractAggregateID(msg MessageContext) string
    GetProcessingTimeout(msg MessageContext) time.Duration
}

// 错误处理器接口
type ErrorHandler interface {
    IsRetryable(err error) bool
    HandleRetryableError(ctx context.Context, msg MessageContext, err error) error
    HandleNonRetryableError(ctx context.Context, msg MessageContext, err error) error
    SendToDeadLetter(ctx context.Context, msg MessageContext, reason string) error
}

// 消息上下文
type MessageContext struct {
    Message   interface{} // 原始消息对象
    Topic     string      // 主题
    Partition int32       // 分区
    Offset    int64       // 偏移量
    Metadata  map[string]string // 元数据
    Timestamp time.Time   // 时间戳
}
```

#### 1.2 配置结构扩展

**文件**: `jxt-core/sdk/config/advanced_eventbus.go`

```go
package config

import (
    "time"
    "golang.org/x/time/rate"
)

// AdvancedEventBusConfig 高级事件总线配置
type AdvancedEventBusConfig struct {
    EventBusConfig `mapstructure:",squash"` // 继承基础配置
    
    // 恢复模式配置
    RecoveryMode RecoveryModeConfig `mapstructure:"recoveryMode"`
    
    // 积压检测配置
    BacklogDetection BacklogDetectionConfig `mapstructure:"backlogDetection"`
    
    // 聚合处理器配置
    AggregateProcessor AggregateProcessorConfig `mapstructure:"aggregateProcessor"`
    
    // 流量控制配置
    RateLimit RateLimitConfig `mapstructure:"rateLimit"`
    
    // 重连配置
    Reconnection ReconnectionConfig `mapstructure:"reconnection"`
}

// 恢复模式配置
type RecoveryModeConfig struct {
    Enabled                bool          `mapstructure:"enabled"`                // 是否启用恢复模式
    AutoDetection          bool          `mapstructure:"autoDetection"`          // 是否自动检测
    TransitionThreshold    int           `mapstructure:"transitionThreshold"`    // 切换阈值（连续检测次数）
    ProcessorIdleTimeout   time.Duration `mapstructure:"processorIdleTimeout"`   // 处理器空闲超时
    GradualTransition      bool          `mapstructure:"gradualTransition"`      // 是否渐进式切换
}

// 聚合处理器配置
type AggregateProcessorConfig struct {
    Enabled                bool          `mapstructure:"enabled"`                // 是否启用
    CacheSize              int           `mapstructure:"cacheSize"`              // 缓存大小
    ChannelBufferSize      int           `mapstructure:"channelBufferSize"`      // 通道缓冲大小
    MaxCreateAttempts      int           `mapstructure:"maxCreateAttempts"`      // 最大创建尝试次数
    IdleTimeout            time.Duration `mapstructure:"idleTimeout"`            // 空闲超时
    ActiveThreshold        float64       `mapstructure:"activeThreshold"`        // 活跃阈值
}

// 流量控制配置
type RateLimitConfig struct {
    Enabled bool       `mapstructure:"enabled"` // 是否启用
    Limit   rate.Limit `mapstructure:"limit"`   // 速率限制
    Burst   int        `mapstructure:"burst"`   // 突发容量
}

// 重连配置
type ReconnectionConfig struct {
    MaxAttempts      int           `mapstructure:"maxAttempts"`      // 最大重连次数
    InitialInterval  time.Duration `mapstructure:"initialInterval"`  // 初始重连间隔
    MaxInterval      time.Duration `mapstructure:"maxInterval"`      // 最大重连间隔
    Multiplier       float64       `mapstructure:"multiplier"`       // 退避倍数
    HealthCheckInterval time.Duration `mapstructure:"healthCheckInterval"` // 健康检查间隔
}

// 默认配置
func GetDefaultAdvancedEventBusConfig() AdvancedEventBusConfig {
    return AdvancedEventBusConfig{
        EventBusConfig: GetDefaultEventBusConfig(),
        RecoveryMode: RecoveryModeConfig{
            Enabled:                true,
            AutoDetection:          true,
            TransitionThreshold:    3,
            ProcessorIdleTimeout:   5 * time.Minute,
            GradualTransition:      true,
        },
        BacklogDetection: BacklogDetectionConfig{
            Enabled:          true,
            MaxLagThreshold:  1000,
            MaxTimeThreshold: 5 * time.Minute,
            CheckInterval:    1 * time.Minute,
        },
        AggregateProcessor: AggregateProcessorConfig{
            Enabled:           true,
            CacheSize:         1000,
            ChannelBufferSize: 100,
            MaxCreateAttempts: 3,
            IdleTimeout:       5 * time.Minute,
            ActiveThreshold:   0.1,
        },
        RateLimit: RateLimitConfig{
            Enabled: true,
            Limit:   1000,
            Burst:   1000,
        },
        Reconnection: ReconnectionConfig{
            MaxAttempts:         5,
            InitialInterval:     1 * time.Second,
            MaxInterval:         1 * time.Minute,
            Multiplier:          2.0,
            HealthCheckInterval: 30 * time.Second,
        },
    }
}
```

### 阶段 2：核心组件实现

#### 2.1 积压检测器

**文件**: `jxt-core/sdk/pkg/eventbus/backlog_detector.go`

```go
package eventbus

import (
    "context"
    "sync"
    "time"
    "sync/atomic"
)

// BacklogDetector 积压检测器
type BacklogDetector struct {
    config           BacklogDetectionConfig
    callbacks        []BacklogStateCallback
    currentState     atomic.Value // *BacklogState
    isMonitoring     atomic.Bool
    monitorCtx       context.Context
    monitorCancel    context.CancelFunc
    wg               sync.WaitGroup
    mu               sync.RWMutex
    
    // 检测实现（由具体的EventBus实现提供）
    detector         BacklogChecker
}

// BacklogChecker 积压检查接口
type BacklogChecker interface {
    CheckBacklog(ctx context.Context) (*BacklogState, error)
}

func NewBacklogDetector(config BacklogDetectionConfig, checker BacklogChecker) *BacklogDetector {
    bd := &BacklogDetector{
        config:   config,
        detector: checker,
    }
    bd.currentState.Store(&BacklogState{})
    return bd
}

func (bd *BacklogDetector) RegisterCallback(callback BacklogStateCallback) error {
    bd.mu.Lock()
    defer bd.mu.Unlock()
    bd.callbacks = append(bd.callbacks, callback)
    return nil
}

func (bd *BacklogDetector) Start(ctx context.Context) error {
    if bd.isMonitoring.Load() {
        return nil // 已经在监控中
    }
    
    bd.monitorCtx, bd.monitorCancel = context.WithCancel(ctx)
    bd.isMonitoring.Store(true)
    
    bd.wg.Add(1)
    go bd.monitorLoop()
    
    return nil
}

func (bd *BacklogDetector) Stop() error {
    if !bd.isMonitoring.Load() {
        return nil // 已经停止
    }
    
    bd.monitorCancel()
    bd.wg.Wait()
    bd.isMonitoring.Store(false)
    
    return nil
}

func (bd *BacklogDetector) GetCurrentState() *BacklogState {
    state := bd.currentState.Load().(*BacklogState)
    return state
}

func (bd *BacklogDetector) monitorLoop() {
    defer bd.wg.Done()
    
    ticker := time.NewTicker(bd.config.CheckInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-bd.monitorCtx.Done():
            return
        case <-ticker.C:
            bd.performCheck()
        }
    }
}

func (bd *BacklogDetector) performCheck() {
    ctx, cancel := context.WithTimeout(bd.monitorCtx, 30*time.Second)
    defer cancel()
    
    newState, err := bd.detector.CheckBacklog(ctx)
    if err != nil {
        // 记录错误，但继续监控
        return
    }
    
    oldState := bd.currentState.Load().(*BacklogState)
    if bd.stateChanged(oldState, newState) {
        bd.currentState.Store(newState)
        bd.notifyCallbacks(newState)
    }
}

func (bd *BacklogDetector) stateChanged(old, new *BacklogState) bool {
    return old.HasBacklog != new.HasBacklog ||
           old.TotalLag != new.TotalLag ||
           old.ActiveProcessors != new.ActiveProcessors
}

func (bd *BacklogDetector) notifyCallbacks(state *BacklogState) {
    bd.mu.RLock()
    callbacks := make([]BacklogStateCallback, len(bd.callbacks))
    copy(callbacks, bd.callbacks)
    bd.mu.RUnlock()
    
    for _, callback := range callbacks {
        go func(cb BacklogStateCallback) {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()
            
            if err := cb(ctx, *state); err != nil {
                // 记录回调错误
            }
        }(callback)
    }
}
```

### 阶段 3：Kafka 实现

#### 3.1 高级 Kafka EventBus

**文件**: `jxt-core/sdk/pkg/eventbus/kafka_advanced.go`

```go
package eventbus

import (
    "context"
    "sync"
    "sync/atomic"
    "time"
    "golang.org/x/time/rate"
    "github.com/hashicorp/golang-lru/v2"
)

// kafkaAdvancedEventBus Kafka高级事件总线实现
type kafkaAdvancedEventBus struct {
    *kafkaEventBus // 继承基础实现
    
    // 高级功能组件
    backlogDetector    *BacklogDetector
    recoveryManager    *RecoveryManager
    processorManager   *AggregateProcessorManager
    rateLimiter        *rate.Limiter
    
    // 配置
    advancedConfig AdvancedEventBusConfig
    
    // 状态管理
    isRecoveryMode atomic.Bool
    
    // 回调管理
    recoveryCallbacks []RecoveryModeCallback
    callbackMu        sync.RWMutex
    
    // 自定义处理器
    messageRouter MessageRouter
    errorHandler  ErrorHandler
    routerMu      sync.RWMutex
}

func NewKafkaAdvancedEventBus(config AdvancedEventBusConfig) (AdvancedEventBus, error) {
    // 创建基础 Kafka EventBus
    baseEventBus, err := NewKafkaEventBus(config.EventBusConfig.Kafka)
    if err != nil {
        return nil, err
    }
    
    kafkaBase := baseEventBus.(*kafkaEventBus)
    
    // 创建高级 EventBus
    advanced := &kafkaAdvancedEventBus{
        kafkaEventBus:  kafkaBase,
        advancedConfig: config,
    }
    
    // 初始化组件
    if err := advanced.initializeComponents(); err != nil {
        return nil, err
    }
    
    return advanced, nil
}

func (k *kafkaAdvancedEventBus) initializeComponents() error {
    // 初始化积压检测器
    if k.advancedConfig.BacklogDetection.Enabled {
        checker := &kafkaBacklogChecker{
            client: k.client,
            admin:  k.admin,
            config: k.advancedConfig.BacklogDetection,
        }
        k.backlogDetector = NewBacklogDetector(k.advancedConfig.BacklogDetection, checker)
    }
    
    // 初始化恢复管理器
    k.recoveryManager = NewRecoveryManager(k.advancedConfig.RecoveryMode)
    
    // 初始化聚合处理器管理器
    if k.advancedConfig.AggregateProcessor.Enabled {
        var err error
        k.processorManager, err = NewAggregateProcessorManager(k.advancedConfig.AggregateProcessor)
        if err != nil {
            return err
        }
    }
    
    // 初始化流量控制
    if k.advancedConfig.RateLimit.Enabled {
        k.rateLimiter = rate.NewLimiter(
            k.advancedConfig.RateLimit.Limit,
            k.advancedConfig.RateLimit.Burst,
        )
    }
    
    // 设置默认的消息路由器和错误处理器
    k.messageRouter = &DefaultMessageRouter{}
    k.errorHandler = &DefaultErrorHandler{}
    
    return nil
}

// 实现 AdvancedEventBus 接口
func (k *kafkaAdvancedEventBus) SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error {
    // 包装处理器以支持高级功能
    wrappedHandler := k.wrapHandler(handler, opts)
    
    // 调用基础订阅方法
    return k.kafkaEventBus.Subscribe(ctx, topic, wrappedHandler)
}

func (k *kafkaAdvancedEventBus) wrapHandler(handler MessageHandler, opts SubscribeOptions) MessageHandler {
    return func(ctx context.Context, message []byte) error {
        // 流量控制
        if k.rateLimiter != nil {
            if err := k.rateLimiter.Wait(ctx); err != nil {
                return err
            }
        }
        
        // 创建消息上下文
        msgCtx := k.createMessageContext(ctx, message)
        
        // 决定处理方式
        if k.shouldUseAggregateProcessor(msgCtx, opts) {
            return k.processWithAggregateProcessor(ctx, msgCtx, handler, opts)
        } else {
            return k.processImmediately(ctx, msgCtx, handler, opts)
        }
    }
}

func (k *kafkaAdvancedEventBus) shouldUseAggregateProcessor(msgCtx MessageContext, opts SubscribeOptions) bool {
    if !opts.UseAggregateProcessor {
        return false
    }
    
    if k.processorManager == nil {
        return false
    }
    
    // 检查是否在恢复模式或消息需要聚合处理
    return k.IsInRecoveryMode() || k.messageRouter.ShouldUseAggregateProcessor(msgCtx)
}

func (k *kafkaAdvancedEventBus) SetRecoveryMode(enabled bool) error {
    oldMode := k.isRecoveryMode.Load()
    k.isRecoveryMode.Store(enabled)
    
    // 如果状态发生变化，通知回调
    if oldMode != enabled {
        k.notifyRecoveryModeCallbacks(enabled)
    }
    
    return nil
}

func (k *kafkaAdvancedEventBus) IsInRecoveryMode() bool {
    return k.isRecoveryMode.Load()
}

func (k *kafkaAdvancedEventBus) RegisterBacklogCallback(callback BacklogStateCallback) error {
    if k.backlogDetector == nil {
        return fmt.Errorf("backlog detection is not enabled")
    }
    return k.backlogDetector.RegisterCallback(callback)
}

func (k *kafkaAdvancedEventBus) StartBacklogMonitoring(ctx context.Context) error {
    if k.backlogDetector == nil {
        return fmt.Errorf("backlog detection is not enabled")
    }
    return k.backlogDetector.Start(ctx)
}

func (k *kafkaAdvancedEventBus) StopBacklogMonitoring() error {
    if k.backlogDetector == nil {
        return nil
    }
    return k.backlogDetector.Stop()
}

func (k *kafkaAdvancedEventBus) GetBacklogState(ctx context.Context) (*BacklogState, error) {
    if k.backlogDetector == nil {
        return nil, fmt.Errorf("backlog detection is not enabled")
    }
    return k.backlogDetector.GetCurrentState(), nil
}

// 其他方法实现...
```

### 阶段 4：业务层适配

#### 4.1 业务层接口适配

**文件**: `evidence-management/shared/common/eventbus/advanced_subscriber.go`

```go
package eventbus

import (
    "context"
    "time"
    "github.com/ThreeDotsLabs/watermill/message"
    "jxt-core/sdk/pkg/eventbus"
)

// AdvancedEventSubscriber 高级事件订阅器
type AdvancedEventSubscriber struct {
    eventBus eventbus.AdvancedEventBus
    config   AdvancedSubscriberConfig
}

type AdvancedSubscriberConfig struct {
    DefaultTimeout        time.Duration
    UseAggregateProcessor bool
    RateLimit            rate.Limit
    RateBurst            int
}

func NewAdvancedEventSubscriber(bus eventbus.AdvancedEventBus, config AdvancedSubscriberConfig) *AdvancedEventSubscriber {
    subscriber := &AdvancedEventSubscriber{
        eventBus: bus,
        config:   config,
    }
    
    // 设置业务特定的路由器和错误处理器
    subscriber.setupBusinessLogic()
    
    return subscriber
}

func (s *AdvancedEventSubscriber) setupBusinessLogic() {
    // 设置业务消息路由器
    s.eventBus.SetMessageRouter(&EvidenceMessageRouter{})
    
    // 设置业务错误处理器
    s.eventBus.SetErrorHandler(&EvidenceErrorHandler{})
    
    // 注册积压状态回调
    s.eventBus.RegisterBacklogCallback(s.handleBacklogStateChange)
    
    // 注册恢复模式回调
    s.eventBus.RegisterRecoveryModeCallback(s.handleRecoveryModeChange)
}

// 业务消息路由器
type EvidenceMessageRouter struct{}

func (r *EvidenceMessageRouter) ShouldUseAggregateProcessor(msg eventbus.MessageContext) bool {
    // 检查消息是否有聚合ID
    aggregateID := r.ExtractAggregateID(msg)
    return aggregateID != ""
}

func (r *EvidenceMessageRouter) ExtractAggregateID(msg eventbus.MessageContext) string {
    // 从消息元数据中提取聚合ID
    if aggregateID, exists := msg.Metadata["aggregateID"]; exists {
        return aggregateID
    }
    return ""
}

func (r *EvidenceMessageRouter) GetProcessingTimeout(msg eventbus.MessageContext) time.Duration {
    // 根据消息类型返回不同的超时时间
    if msgType, exists := msg.Metadata["eventType"]; exists {
        switch msgType {
        case "MediaUploaded":
            return 60 * time.Second // 媒体上传需要更长时间
        case "ArchiveCreated":
            return 30 * time.Second
        default:
            return 15 * time.Second
        }
    }
    return 30 * time.Second
}

// 业务错误处理器
type EvidenceErrorHandler struct{}

func (h *EvidenceErrorHandler) IsRetryable(err error) bool {
    // 业务特定的重试逻辑
    switch err.(type) {
    case *json.SyntaxError, *json.UnmarshalTypeError:
        return false // 解析错误不重试
    case *net.OpError, *os.SyscallError:
        return true // 网络错误重试
    default:
        // 检查是否是业务逻辑错误
        if isBusinessLogicError(err) {
            return false
        }
        return true
    }
}

func (h *EvidenceErrorHandler) HandleRetryableError(ctx context.Context, msg eventbus.MessageContext, err error) error {
    // 记录重试错误
    log.Printf("Retryable error for message %s: %v", msg.Topic, err)
    return nil // 返回 nil 表示使用默认重试机制
}

func (h *EvidenceErrorHandler) HandleNonRetryableError(ctx context.Context, msg eventbus.MessageContext, err error) error {
    // 记录不可重试错误
    log.Printf("Non-retryable error for message %s: %v", msg.Topic, err)
    return h.SendToDeadLetter(ctx, msg, err.Error())
}

func (h *EvidenceErrorHandler) SendToDeadLetter(ctx context.Context, msg eventbus.MessageContext, reason string) error {
    // 发送到死信队列的业务逻辑
    deadLetterTopic := fmt.Sprintf("%s-dead-letter", msg.Topic)
    
    // 构造死信消息
    deadLetterMsg := map[string]interface{}{
        "originalTopic":   msg.Topic,
        "originalMessage": msg.Message,
        "reason":         reason,
        "timestamp":      time.Now(),
        "metadata":       msg.Metadata,
    }
    
    // 发布到死信队列
    // 这里需要实现具体的发布逻辑
    log.Printf("Sending message to dead letter queue: %s", deadLetterTopic)
    
    return nil
}

// 积压状态变化处理
func (s *AdvancedEventSubscriber) handleBacklogStateChange(ctx context.Context, state eventbus.BacklogState) error {
    if state.HasBacklog {
        log.Printf("Backlog detected: %d messages, %d active processors", state.TotalLag, state.ActiveProcessors)
        // 可以在这里实现告警逻辑
        return s.sendBacklogAlert(state)
    } else {
        log.Printf("No backlog detected, system is up-to-date")
        return s.clearBacklogAlert()
    }
}

// 恢复模式变化处理
func (s *AdvancedEventSubscriber) handleRecoveryModeChange(ctx context.Context, isRecovery bool) error {
    if isRecovery {
        log.Println("Entering recovery mode: processing messages by aggregate ID")
        // 可以在这里调整业务逻辑，比如暂停某些非关键任务
    } else {
        log.Println("Exiting recovery mode: transitioning to normal processing")
        // 恢复正常业务逻辑
    }
    return nil
}

// 订阅方法
func (s *AdvancedEventSubscriber) Subscribe(topic string, handler func(msg *message.Message) error, timeout time.Duration) error {
    // 包装处理器以适配新接口
    wrappedHandler := func(ctx context.Context, message []byte) error {
        // 将字节数组转换为 watermill 消息
        msg := message.NewMessage(watermill.NewUUID(), message)
        return handler(msg)
    }
    
    // 构造订阅选项
    opts := eventbus.SubscribeOptions{
        UseAggregateProcessor: s.config.UseAggregateProcessor,
        ProcessingTimeout:     timeout,
        RateLimit:            s.config.RateLimit,
        RateBurst:            s.config.RateBurst,
        RetryPolicy: eventbus.RetryPolicy{
            MaxRetries:      3,
            InitialInterval: 1 * time.Second,
            MaxInterval:     30 * time.Second,
            Multiplier:      2.0,
        },
        DeadLetterEnabled: true,
    }
    
    return s.eventBus.SubscribeWithOptions(context.Background(), topic, wrappedHandler, opts)
}
```

## 迁移计划

### 第一阶段：基础框架（2周）
1. 创建高级接口定义
2. 实现配置结构
3. 创建基础组件框架
4. 编写单元测试

### 第二阶段：核心功能（3周）
1. 实现积压检测器
2. 实现恢复模式管理器
3. 实现聚合处理器管理器
4. 实现流量控制

### 第三阶段：Kafka集成（2周）
1. 扩展 Kafka EventBus
2. 实现 Kafka 特定的积压检测
3. 集成所有高级功能
4. 性能优化

### 第四阶段：业务层适配（2周）
1. 创建业务层适配器
2. 实现业务特定的路由器和错误处理器
3. 迁移现有业务代码
4. 集成测试

### 第五阶段：文档和部署（1周）
1. 完善文档
2. 创建迁移指南
3. 部署测试
4. 性能验证

## 风险评估

### 技术风险
- **性能影响**：新的抽象层可能带来性能开销
- **兼容性**：需要确保与现有代码的兼容性
- **复杂性**：增加的功能可能导致系统复杂性上升

### 缓解措施
- 进行充分的性能测试
- 提供向后兼容的接口
- 分阶段实施，逐步验证
- 完善的文档和示例

## 成功指标

1. **功能完整性**：所有现有功能都能正常工作
2. **性能指标**：性能不低于现有实现的 95%
3. **代码复用**：减少 70% 的重复代码
4. **维护性**：新功能的添加和修改更加容易
5. **可扩展性**：支持更多的事件总线实现

## 相关文档

- [代码实现示例](./advanced-eventbus-code-examples.md) - 详细的代码实现示例
- [迁移指南](./migration-guide.md) - 从现有系统迁移的详细步骤
- [API 文档](./advanced-eventbus-api.md) - 完整的 API 参考文档
- [最佳实践](./advanced-eventbus-best-practices.md) - 使用最佳实践和性能优化

## 总结

本实施方案提供了一个完整的路径，将 evidence-management 中的高级功能迁移到 jxt-core，同时保持业务逻辑的灵活性。通过分层设计和接口抽象，我们可以实现技术基础设施的统一，同时允许业务层根据需要进行定制。

### 核心优势

1. **统一技术栈**：所有微服务使用相同的事件总线基础设施
2. **减少重复代码**：预计减少 70% 的重复实现
3. **提高维护性**：集中维护技术组件，统一升级
4. **增强可扩展性**：支持更多事件总线实现（NATS、Redis 等）
5. **改善监控**：统一的监控和告警机制
6. **业务灵活性**：通过接口和回调保持业务逻辑的灵活性

### 实施建议

1. **分阶段实施**：按照文档中的 5 个阶段逐步实施
2. **充分测试**：每个阶段都要进行充分的单元测试和集成测试
3. **性能验证**：确保新实现的性能不低于现有系统
4. **文档完善**：及时更新文档和示例代码
5. **团队培训**：对开发团队进行新架构的培训

这个方案不仅解决了当前的技术债务问题，还为未来的扩展和优化奠定了坚实的基础。
