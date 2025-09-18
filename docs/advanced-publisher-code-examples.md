# 高级事件发布器代码实现示例

## 概述

本文档提供了高级事件发布器实施方案中关键组件的详细代码实现示例，包括统一健康检查、发布端管理、消息格式化等核心功能。

## 核心组件实现

### 1. 统一健康检查消息

```go
// jxt-core/sdk/pkg/eventbus/health_check_message.go
package eventbus

import (
    "encoding/json"
    "fmt"
    "time"
    "crypto/rand"
    "encoding/hex"
)

// HealthCheckMessage 标准化的健康检查消息
type HealthCheckMessage struct {
    MessageID    string            `json:"messageId"`
    Timestamp    time.Time         `json:"timestamp"`
    Source       string            `json:"source"`       // 微服务名称
    EventBusType string            `json:"eventBusType"` // kafka/nats/memory
    Version      string            `json:"version"`      // jxt-core版本
    Metadata     map[string]string `json:"metadata"`
}

// CreateHealthCheckMessage 创建标准健康检查消息
func CreateHealthCheckMessage(source, eventBusType string) *HealthCheckMessage {
    return &HealthCheckMessage{
        MessageID:    generateMessageID(),
        Timestamp:    time.Now(),
        Source:       source,
        EventBusType: eventBusType,
        Version:      GetJXTCoreVersion(),
        Metadata:     make(map[string]string),
    }
}

// ToBytes 序列化为字节数组
func (h *HealthCheckMessage) ToBytes() ([]byte, error) {
    return json.Marshal(h)
}

// FromBytes 从字节数组反序列化
func (h *HealthCheckMessage) FromBytes(data []byte) error {
    return json.Unmarshal(data, h)
}

// IsValid 验证消息是否有效
func (h *HealthCheckMessage) IsValid() bool {
    if h.MessageID == "" || h.Source == "" || h.EventBusType == "" {
        return false
    }
    
    // 检查时间戳是否在合理范围内（避免时钟偏移问题）
    now := time.Now()
    if h.Timestamp.After(now.Add(1*time.Minute)) || 
       h.Timestamp.Before(now.Add(-5*time.Minute)) {
        return false
    }
    
    return true
}

// IsExpired 检查消息是否过期
func (h *HealthCheckMessage) IsExpired(ttl time.Duration) bool {
    return time.Since(h.Timestamp) > ttl
}

func generateMessageID() string {
    bytes := make([]byte, 8)
    rand.Read(bytes)
    return fmt.Sprintf("hc-%d-%s", time.Now().UnixNano(), hex.EncodeToString(bytes))
}

func GetJXTCoreVersion() string {
    return "2.0.0" // 从构建信息获取
}
```

### 2. 统一健康检查器

```go
// jxt-core/sdk/pkg/eventbus/health_checker.go
package eventbus

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
    "time"
)

// HealthChecker 健康检查器
type HealthChecker struct {
    config          HealthCheckConfig
    eventBus        EventBus
    source          string // 微服务名称
    eventBusType    string // 事件总线类型
    
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
}

type HealthCheckCallback func(ctx context.Context, result HealthCheckResult) error

type HealthCheckResult struct {
    Success             bool          `json:"success"`
    Timestamp           time.Time     `json:"timestamp"`
    Duration            time.Duration `json:"duration"`
    Error               error         `json:"error,omitempty"`
    ConsecutiveFailures int           `json:"consecutiveFailures"`
    EventBusType        string        `json:"eventBusType"`
    Source              string        `json:"source"`
}

type HealthCheckStatus struct {
    IsHealthy           bool      `json:"isHealthy"`
    ConsecutiveFailures int       `json:"consecutiveFailures"`
    LastSuccessTime     time.Time `json:"lastSuccessTime"`
    LastFailureTime     time.Time `json:"lastFailureTime"`
    IsRunning           bool      `json:"isRunning"`
}

func NewHealthChecker(config HealthCheckConfig, eventBus EventBus, source, eventBusType string) *HealthChecker {
    hc := &HealthChecker{
        config:       config,
        eventBus:     eventBus,
        source:       source,
        eventBusType: eventBusType,
    }
    hc.lastSuccessTime.Store(time.Now())
    hc.lastFailureTime.Store(time.Time{})
    return hc
}

func (hc *HealthChecker) Start(ctx context.Context) error {
    if hc.isRunning.Load() {
        return nil // 已经在运行
    }
    
    hc.ctx, hc.cancel = context.WithCancel(ctx)
    hc.isRunning.Store(true)
    
    hc.wg.Add(1)
    go hc.healthCheckLoop()
    
    return nil
}

func (hc *HealthChecker) Stop() error {
    if !hc.isRunning.Load() {
        return nil
    }
    
    hc.cancel()
    hc.wg.Wait()
    hc.isRunning.Store(false)
    
    return nil
}

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

func (hc *HealthChecker) performHealthCheck() {
    start := time.Now()
    
    // 创建标准健康检查消息
    healthMsg := CreateHealthCheckMessage(hc.source, hc.eventBusType)
    healthMsg.Metadata["checkType"] = "periodic"
    
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
    
    hc.notifyCallbacks(result)
}

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
    
    hc.notifyCallbacks(result)
}

func (hc *HealthChecker) RegisterCallback(callback HealthCheckCallback) error {
    hc.callbackMu.Lock()
    defer hc.callbackMu.Unlock()
    hc.callbacks = append(hc.callbacks, callback)
    return nil
}

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
                // 记录回调错误，但不影响健康检查
            }
        }(callback)
    }
}

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
    }
}
```

### 3. 消息格式化器

```go
// jxt-core/sdk/pkg/eventbus/message_formatter.go
package eventbus

import (
    "fmt"
    "strconv"
)

// MessageFormatter 消息格式化器接口
type MessageFormatter interface {
    FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error)
    ExtractAggregateID(aggregateID interface{}) string
    SetMetadata(msg *Message, metadata map[string]string) error
}

// Message 消息结构
type Message struct {
    UUID     string            `json:"uuid"`
    Payload  []byte            `json:"payload"`
    Metadata map[string]string `json:"metadata"`
}

func NewMessage(uuid string, payload []byte) *Message {
    return &Message{
        UUID:     uuid,
        Payload:  payload,
        Metadata: make(map[string]string),
    }
}

// DefaultMessageFormatter 默认消息格式化器
type DefaultMessageFormatter struct{}

func (f *DefaultMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
    msg := NewMessage(uuid, payload)
    
    // 提取聚合ID
    aggID := f.ExtractAggregateID(aggregateID)
    if aggID != "" {
        msg.Metadata["aggregateID"] = aggID
    }
    
    return msg, nil
}

func (f *DefaultMessageFormatter) ExtractAggregateID(aggregateID interface{}) string {
    if aggregateID == nil {
        return ""
    }
    
    switch id := aggregateID.(type) {
    case string:
        return id
    case int64:
        return strconv.FormatInt(id, 10)
    case int:
        return strconv.Itoa(id)
    case int32:
        return strconv.FormatInt(int64(id), 10)
    default:
        return fmt.Sprintf("%v", id)
    }
}

func (f *DefaultMessageFormatter) SetMetadata(msg *Message, metadata map[string]string) error {
    if msg.Metadata == nil {
        msg.Metadata = make(map[string]string)
    }
    
    for k, v := range metadata {
        msg.Metadata[k] = v
    }
    
    return nil
}

// EvidenceMessageFormatter 业务特定的消息格式化器
type EvidenceMessageFormatter struct {
    DefaultMessageFormatter
}

func (f *EvidenceMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
    msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
    if err != nil {
        return nil, err
    }
    
    // 业务特定的元数据字段名（evidence-management 使用 aggregate_id）
    if aggID := f.ExtractAggregateID(aggregateID); aggID != "" {
        msg.Metadata["aggregate_id"] = aggID
        // 同时保留标准字段名以便兼容
        msg.Metadata["aggregateID"] = aggID
    }
    
    return msg, nil
}
```

### 4. 高级发布管理器

```go
// jxt-core/sdk/pkg/eventbus/advanced_publisher.go
package eventbus

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
    "time"
)

// AdvancedPublisher 高级发布管理器
type AdvancedPublisher struct {
    config           PublisherConfig
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
}

type PublisherConfig struct {
    MaxReconnectAttempts int           `mapstructure:"maxReconnectAttempts"`
    MaxBackoff          time.Duration `mapstructure:"maxBackoff"`
    InitialBackoff      time.Duration `mapstructure:"initialBackoff"`
    PublishTimeout      time.Duration `mapstructure:"publishTimeout"`
}

type PublishOptions struct {
    MessageFormatter MessageFormatter
    Metadata        map[string]string
    RetryPolicy     RetryPolicy
    Timeout         time.Duration
    AggregateID     interface{}
}

type RetryPolicy struct {
    MaxRetries      int           `json:"maxRetries"`
    InitialInterval time.Duration `json:"initialInterval"`
    MaxInterval     time.Duration `json:"maxInterval"`
    Multiplier      float64       `json:"multiplier"`
}

type ReconnectCallback func(ctx context.Context) error
type PublishCallback func(ctx context.Context, topic string, message []byte, err error) error

func NewAdvancedPublisher(config PublisherConfig, eventBus EventBus) *AdvancedPublisher {
    return &AdvancedPublisher{
        config:           config,
        eventBus:         eventBus,
        backoff:          config.InitialBackoff,
        messageFormatter: &DefaultMessageFormatter{},
    }
}

func (ap *AdvancedPublisher) Start(ctx context.Context) error {
    ap.ctx, ap.cancel = context.WithCancel(ctx)
    ap.isConnected.Store(true)
    return nil
}

func (ap *AdvancedPublisher) Stop() error {
    if ap.cancel != nil {
        ap.cancel()
    }
    ap.wg.Wait()
    ap.isConnected.Store(false)
    return nil
}

func (ap *AdvancedPublisher) PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error {
    // 选择消息格式化器
    formatter := opts.MessageFormatter
    if formatter == nil {
        ap.formatterMu.RLock()
        formatter = ap.messageFormatter
        ap.formatterMu.RUnlock()
    }
    
    // 生成消息UUID
    uuid := generateUUID()
    
    // 格式化消息
    msg, err := formatter.FormatMessage(uuid, opts.AggregateID, message)
    if err != nil {
        ap.notifyPublishCallbacks(ctx, topic, message, fmt.Errorf("message formatting failed: %w", err))
        return err
    }
    
    // 设置额外的元数据
    if opts.Metadata != nil {
        formatter.SetMetadata(msg, opts.Metadata)
    }
    
    // 序列化消息
    msgBytes, err := ap.serializeMessage(msg)
    if err != nil {
        ap.notifyPublishCallbacks(ctx, topic, message, fmt.Errorf("message serialization failed: %w", err))
        return err
    }
    
    // 设置超时
    timeout := opts.Timeout
    if timeout == 0 {
        timeout = ap.config.PublishTimeout
    }
    
    publishCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    // 发布消息
    err = ap.eventBus.Publish(publishCtx, topic, msgBytes)
    
    // 通知回调
    ap.notifyPublishCallbacks(ctx, topic, message, err)
    
    if err != nil {
        ap.handlePublishFailure(err)
        return err
    }
    
    ap.handlePublishSuccess()
    return nil
}

func (ap *AdvancedPublisher) SetMessageFormatter(formatter MessageFormatter) error {
    ap.formatterMu.Lock()
    defer ap.formatterMu.Unlock()
    ap.messageFormatter = formatter
    return nil
}

func (ap *AdvancedPublisher) RegisterReconnectCallback(callback ReconnectCallback) error {
    ap.callbackMu.Lock()
    defer ap.callbackMu.Unlock()
    ap.reconnectCallbacks = append(ap.reconnectCallbacks, callback)
    return nil
}

func (ap *AdvancedPublisher) RegisterPublishCallback(callback PublishCallback) error {
    ap.callbackMu.Lock()
    defer ap.callbackMu.Unlock()
    ap.publishCallbacks = append(ap.publishCallbacks, callback)
    return nil
}

func (ap *AdvancedPublisher) handlePublishSuccess() {
    atomic.StoreInt32(&ap.failureCount, 0)
    ap.backoff = ap.config.InitialBackoff
}

func (ap *AdvancedPublisher) handlePublishFailure(err error) {
    failures := atomic.AddInt32(&ap.failureCount, 1)
    
    // 如果连续失败次数达到阈值，触发重连
    if failures >= 3 {
        go ap.attemptReconnect()
    }
}

func (ap *AdvancedPublisher) attemptReconnect() {
    // 重连逻辑（简化版）
    for i := 0; i < ap.config.MaxReconnectAttempts; i++ {
        time.Sleep(ap.backoff)
        
        // 这里应该调用具体的重连逻辑
        // 成功后通知回调
        ap.notifyReconnectCallbacks()
        
        // 更新退避时间
        ap.backoff *= 2
        if ap.backoff > ap.config.MaxBackoff {
            ap.backoff = ap.config.MaxBackoff
        }
    }
}

func (ap *AdvancedPublisher) notifyReconnectCallbacks() {
    ap.callbackMu.RLock()
    callbacks := make([]ReconnectCallback, len(ap.reconnectCallbacks))
    copy(callbacks, ap.reconnectCallbacks)
    ap.callbackMu.RUnlock()
    
    for _, callback := range callbacks {
        go func(cb ReconnectCallback) {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()
            cb(ctx)
        }(callback)
    }
}

func (ap *AdvancedPublisher) notifyPublishCallbacks(ctx context.Context, topic string, message []byte, err error) {
    ap.callbackMu.RLock()
    callbacks := make([]PublishCallback, len(ap.publishCallbacks))
    copy(callbacks, ap.publishCallbacks)
    ap.callbackMu.RUnlock()
    
    for _, callback := range callbacks {
        go func(cb PublishCallback) {
            callbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            cb(callbackCtx, topic, message, err)
        }(callback)
    }
}

func (ap *AdvancedPublisher) serializeMessage(msg *Message) ([]byte, error) {
    // 这里应该根据具体的事件总线实现来序列化消息
    // 对于 Kafka，可能需要转换为 watermill 的 Message 格式
    return msg.Payload, nil
}

func generateUUID() string {
    // 生成UUID的逻辑
    return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}
```

### 5. 业务层适配器示例

```go
// evidence-management/shared/common/eventbus/advanced_adapter.go
package eventbus

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "jxt-core/sdk/pkg/eventbus"
    "jxt-evidence-system/evidence-management/shared/domain/event"
)

// EventPublisherAdapter 事件发布器适配器
type EventPublisherAdapter struct {
    advancedBus eventbus.AdvancedEventBus
}

func NewEventPublisherAdapter(bus eventbus.AdvancedEventBus) *EventPublisherAdapter {
    adapter := &EventPublisherAdapter{
        advancedBus: bus,
    }
    
    // 设置业务特定的消息格式化器
    bus.SetMessageFormatter(&EvidenceMessageFormatter{})
    
    // 注册业务回调
    bus.RegisterReconnectCallback(adapter.handleReconnect)
    bus.RegisterPublishCallback(adapter.handlePublishResult)
    bus.RegisterHealthCheckCallback(adapter.handleHealthCheck)
    
    return adapter
}

// PublishMessage 保持与原有接口兼容
func (a *EventPublisherAdapter) PublishMessage(topic string, uuid string, aggregateID interface{}, payload []byte) error {
    opts := eventbus.PublishOptions{
        AggregateID: aggregateID,
        Timeout:     30 * time.Second,
        Metadata: map[string]string{
            "messageUUID": uuid,
        },
    }
    
    return a.advancedBus.PublishWithOptions(context.Background(), topic, payload, opts)
}

// Publish 实现 EventPublisher 接口
func (a *EventPublisherAdapter) Publish(ctx context.Context, topic string, event event.Event) error {
    if event == nil {
        return fmt.Errorf("event cannot be nil")
    }
    
    payload, err := event.MarshalJSON()
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }
    
    opts := eventbus.PublishOptions{
        AggregateID: event.GetAggregateID(),
        Metadata: map[string]string{
            "eventType": event.GetEventType(),
            "eventID":   event.GetEventID(),
        },
        Timeout: 30 * time.Second,
    }
    
    return a.advancedBus.PublishWithOptions(ctx, topic, payload, opts)
}

func (a *EventPublisherAdapter) RegisterReconnectCallback(callback func(ctx context.Context) error) error {
    return a.advancedBus.RegisterReconnectCallback(callback)
}

func (a *EventPublisherAdapter) handleReconnect(ctx context.Context) error {
    log.Println("Kafka publisher reconnected successfully")
    // 这里可以添加业务特定的重连处理逻辑
    // 比如重新发布失败的消息、更新状态等
    return nil
}

func (a *EventPublisherAdapter) handlePublishResult(ctx context.Context, topic string, message []byte, err error) error {
    if err != nil {
        log.Printf("Publish failed for topic %s: %v", topic, err)
        // 这里可以添加业务特定的失败处理逻辑
        // 比如记录到失败队列、发送告警等
    } else {
        log.Printf("Publish succeeded for topic %s", topic)
    }
    return nil
}

func (a *EventPublisherAdapter) handleHealthCheck(ctx context.Context, result eventbus.HealthCheckResult) error {
    if !result.Success {
        log.Printf("Health check failed: %v", result.Error)
        // 业务特定的健康检查失败处理
        // 比如发送告警、更新监控指标等
    } else {
        log.Printf("Health check succeeded for %s", result.Source)
    }
    return nil
}

// EvidenceMessageFormatter 业务特定的消息格式化器
type EvidenceMessageFormatter struct {
    eventbus.DefaultMessageFormatter
}

func (f *EvidenceMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*eventbus.Message, error) {
    msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
    if err != nil {
        return nil, err
    }
    
    // evidence-management 特定的字段名
    if aggID := f.ExtractAggregateID(aggregateID); aggID != "" {
        msg.Metadata["aggregate_id"] = aggID // 保持与现有代码兼容
    }
    
    return msg, nil
}
```

这些代码示例展示了高级事件发布器的核心实现，包括统一健康检查、消息格式化、发布管理等关键功能。通过这些组件，可以实现一个功能完整、易于维护的企业级事件发布系统。
