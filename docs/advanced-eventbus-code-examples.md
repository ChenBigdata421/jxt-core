# 高级事件总线代码实现示例

## 概述

本文档提供了高级事件总线实施方案中关键组件的详细代码实现示例。

## 核心组件实现

### 1. 聚合处理器管理器

```go
// jxt-core/sdk/pkg/eventbus/aggregate_processor_manager.go
package eventbus

import (
    "context"
    "sync"
    "sync/atomic"
    "time"
    "github.com/hashicorp/golang-lru/v2"
)

// AggregateProcessorManager 聚合处理器管理器
type AggregateProcessorManager struct {
    config      AggregateProcessorConfig
    processors  *lru.Cache[string, *AggregateProcessor]
    wg          sync.WaitGroup
    ctx         context.Context
    cancel      context.CancelFunc
    
    // 统计信息
    totalProcessors  int64
    activeProcessors int64
}

// AggregateProcessor 聚合处理器
type AggregateProcessor struct {
    aggregateID   string
    messages      chan ProcessorMessage
    lastActivity  atomic.Value // time.Time
    done          chan struct{}
    handler       MessageHandler
    options       SubscribeOptions
    isActive      atomic.Bool
    messageCount  int64
}

type ProcessorMessage struct {
    Context MessageContext
    Handler MessageHandler
    Options SubscribeOptions
    Done    chan error
}

func NewAggregateProcessorManager(config AggregateProcessorConfig) (*AggregateProcessorManager, error) {
    cache, err := lru.New[string, *AggregateProcessor](config.CacheSize)
    if err != nil {
        return nil, err
    }
    
    ctx, cancel := context.WithCancel(context.Background())
    
    return &AggregateProcessorManager{
        config:     config,
        processors: cache,
        ctx:        ctx,
        cancel:     cancel,
    }, nil
}

func (apm *AggregateProcessorManager) GetOrCreateProcessor(
    aggregateID string,
    handler MessageHandler,
    options SubscribeOptions,
) (*AggregateProcessor, error) {
    
    // 尝试从缓存获取
    if processor, exists := apm.processors.Get(aggregateID); exists {
        return processor, nil
    }
    
    // 创建新处理器
    processor := &AggregateProcessor{
        aggregateID: aggregateID,
        messages:    make(chan ProcessorMessage, apm.config.ChannelBufferSize),
        done:        make(chan struct{}),
        handler:     handler,
        options:     options,
    }
    processor.lastActivity.Store(time.Now())
    
    // 添加到缓存
    if evicted := apm.processors.Add(aggregateID, processor); evicted {
        // 处理被驱逐的处理器
        apm.handleEvictedProcessor(aggregateID)
    }
    
    // 启动处理器
    atomic.AddInt64(&apm.totalProcessors, 1)
    apm.wg.Add(1)
    go apm.runProcessor(processor)
    
    return processor, nil
}

func (apm *AggregateProcessorManager) runProcessor(processor *AggregateProcessor) {
    defer apm.wg.Done()
    defer apm.releaseProcessor(processor)
    
    atomic.AddInt64(&apm.activeProcessors, 1)
    processor.isActive.Store(true)
    
    defer func() {
        atomic.AddInt64(&apm.activeProcessors, -1)
        processor.isActive.Store(false)
    }()
    
    idleTimer := time.NewTimer(apm.config.IdleTimeout)
    defer idleTimer.Stop()
    
    for {
        select {
        case msg, ok := <-processor.messages:
            if !ok {
                return // 通道已关闭
            }
            
            // 重置空闲计时器
            if !idleTimer.Stop() {
                <-idleTimer.C
            }
            idleTimer.Reset(apm.config.IdleTimeout)
            
            // 处理消息
            apm.processMessage(processor, msg)
            
        case <-idleTimer.C:
            // 检查是否真的空闲
            lastActivity := processor.lastActivity.Load().(time.Time)
            if time.Since(lastActivity) >= apm.config.IdleTimeout {
                return // 空闲超时，退出处理器
            }
            // 重置计时器继续等待
            idleTimer.Reset(apm.config.IdleTimeout)
            
        case <-processor.done:
            return // 收到停止信号
            
        case <-apm.ctx.Done():
            return // 管理器被关闭
        }
    }
}

func (apm *AggregateProcessorManager) processMessage(processor *AggregateProcessor, msg ProcessorMessage) {
    defer func() {
        processor.lastActivity.Store(time.Now())
        atomic.AddInt64(&processor.messageCount, 1)
    }()
    
    // 创建处理上下文
    ctx, cancel := context.WithTimeout(apm.ctx, msg.Options.ProcessingTimeout)
    defer cancel()
    
    // 处理消息
    err := msg.Handler(ctx, msg.Context.Message.([]byte))
    
    // 发送结果
    select {
    case msg.Done <- err:
    case <-ctx.Done():
        // 超时，发送超时错误
        select {
        case msg.Done <- ctx.Err():
        default:
        }
    }
}

func (apm *AggregateProcessorManager) SendMessage(
    aggregateID string,
    msgCtx MessageContext,
    handler MessageHandler,
    options SubscribeOptions,
) error {
    
    processor, err := apm.GetOrCreateProcessor(aggregateID, handler, options)
    if err != nil {
        return err
    }
    
    done := make(chan error, 1)
    msg := ProcessorMessage{
        Context: msgCtx,
        Handler: handler,
        Options: options,
        Done:    done,
    }
    
    // 发送消息到处理器
    select {
    case processor.messages <- msg:
        // 等待处理结果
        select {
        case err := <-done:
            return err
        case <-time.After(options.ProcessingTimeout):
            return context.DeadlineExceeded
        case <-apm.ctx.Done():
            return apm.ctx.Err()
        }
    case <-time.After(options.ProcessingTimeout):
        return context.DeadlineExceeded
    case <-apm.ctx.Done():
        return apm.ctx.Err()
    }
}

func (apm *AggregateProcessorManager) releaseProcessor(processor *AggregateProcessor) {
    apm.processors.Remove(processor.aggregateID)
    close(processor.messages)
    close(processor.done)
    atomic.AddInt64(&apm.totalProcessors, -1)
}

func (apm *AggregateProcessorManager) GetStats() ProcessorStats {
    total := atomic.LoadInt64(&apm.totalProcessors)
    active := atomic.LoadInt64(&apm.activeProcessors)
    
    details := make(map[string]ProcessorDetail)
    
    // 遍历所有处理器获取详细信息
    keys := apm.processors.Keys()
    for _, key := range keys {
        if processor, exists := apm.processors.Get(key); exists {
            details[key] = ProcessorDetail{
                AggregateID:  processor.aggregateID,
                LastActivity: processor.lastActivity.Load().(time.Time),
                MessageCount: atomic.LoadInt64(&processor.messageCount),
                IsActive:     processor.isActive.Load(),
            }
        }
    }
    
    return ProcessorStats{
        TotalProcessors:  total,
        ActiveProcessors: active,
        IdleProcessors:   total - active,
        ProcessorDetails: details,
    }
}

func (apm *AggregateProcessorManager) Stop() error {
    apm.cancel()
    
    // 停止所有处理器
    keys := apm.processors.Keys()
    for _, key := range keys {
        if processor, exists := apm.processors.Get(key); exists {
            close(processor.done)
        }
    }
    
    // 等待所有处理器完成
    done := make(chan struct{})
    go func() {
        apm.wg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        return nil
    case <-time.After(30 * time.Second):
        return fmt.Errorf("timeout waiting for processors to stop")
    }
}
```

### 2. 恢复模式管理器

```go
// jxt-core/sdk/pkg/eventbus/recovery_manager.go
package eventbus

import (
    "context"
    "sync"
    "sync/atomic"
    "time"
)

// RecoveryManager 恢复模式管理器
type RecoveryManager struct {
    config              RecoveryModeConfig
    isRecoveryMode      atomic.Bool
    transitionCount     int32
    callbacks           []RecoveryModeCallback
    callbackMu          sync.RWMutex
    
    // 自动检测相关
    isAutoDetecting     atomic.Bool
    detectionCtx        context.Context
    detectionCancel     context.CancelFunc
    wg                  sync.WaitGroup
}

func NewRecoveryManager(config RecoveryModeConfig) *RecoveryManager {
    rm := &RecoveryManager{
        config: config,
    }
    rm.isRecoveryMode.Store(config.Enabled)
    return rm
}

func (rm *RecoveryManager) SetRecoveryMode(enabled bool) error {
    oldMode := rm.isRecoveryMode.Load()
    rm.isRecoveryMode.Store(enabled)
    
    // 重置转换计数
    atomic.StoreInt32(&rm.transitionCount, 0)
    
    // 如果状态发生变化，通知回调
    if oldMode != enabled {
        rm.notifyCallbacks(enabled)
    }
    
    return nil
}

func (rm *RecoveryManager) IsInRecoveryMode() bool {
    return rm.isRecoveryMode.Load()
}

func (rm *RecoveryManager) RegisterCallback(callback RecoveryModeCallback) error {
    rm.callbackMu.Lock()
    defer rm.callbackMu.Unlock()
    rm.callbacks = append(rm.callbacks, callback)
    return nil
}

func (rm *RecoveryManager) StartAutoDetection(ctx context.Context, backlogDetector *BacklogDetector) error {
    if !rm.config.AutoDetection {
        return nil
    }
    
    if rm.isAutoDetecting.Load() {
        return nil // 已经在自动检测中
    }
    
    rm.detectionCtx, rm.detectionCancel = context.WithCancel(ctx)
    rm.isAutoDetecting.Store(true)
    
    // 注册积压状态回调
    if backlogDetector != nil {
        backlogDetector.RegisterCallback(rm.handleBacklogStateChange)
    }
    
    return nil
}

func (rm *RecoveryManager) StopAutoDetection() error {
    if !rm.isAutoDetecting.Load() {
        return nil
    }
    
    rm.detectionCancel()
    rm.wg.Wait()
    rm.isAutoDetecting.Store(false)
    
    return nil
}

func (rm *RecoveryManager) handleBacklogStateChange(ctx context.Context, state BacklogState) error {
    if !rm.config.AutoDetection {
        return nil
    }
    
    if state.HasBacklog {
        // 有积压，进入恢复模式
        if !rm.IsInRecoveryMode() {
            rm.SetRecoveryMode(true)
        }
        // 重置转换计数
        atomic.StoreInt32(&rm.transitionCount, 0)
    } else {
        // 无积压，考虑退出恢复模式
        if rm.IsInRecoveryMode() {
            count := atomic.AddInt32(&rm.transitionCount, 1)
            if count >= int32(rm.config.TransitionThreshold) {
                rm.SetRecoveryMode(false)
            }
        }
    }
    
    return nil
}

func (rm *RecoveryManager) notifyCallbacks(isRecovery bool) {
    rm.callbackMu.RLock()
    callbacks := make([]RecoveryModeCallback, len(rm.callbacks))
    copy(callbacks, rm.callbacks)
    rm.callbackMu.RUnlock()
    
    for _, callback := range callbacks {
        go func(cb RecoveryModeCallback) {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()
            
            if err := cb(ctx, isRecovery); err != nil {
                // 记录回调错误
            }
        }(callback)
    }
}
```

### 3. Kafka 积压检查器

```go
// jxt-core/sdk/pkg/eventbus/kafka_backlog_checker.go
package eventbus

import (
    "context"
    "fmt"
    "time"
    "github.com/Shopify/sarama"
)

// kafkaBacklogChecker Kafka积压检查器
type kafkaBacklogChecker struct {
    client sarama.Client
    admin  sarama.ClusterAdmin
    config BacklogDetectionConfig
}

func (kbc *kafkaBacklogChecker) CheckBacklog(ctx context.Context) (*BacklogState, error) {
    // 获取消费者组信息
    groups, err := kbc.admin.ListConsumerGroups()
    if err != nil {
        return nil, fmt.Errorf("failed to list consumer groups: %w", err)
    }
    
    state := &BacklogState{
        Timestamp:      time.Now(),
        AffectedTopics: make([]string, 0),
        Details:        make(map[string]int64),
    }
    
    var totalLag int64
    
    // 检查每个消费者组
    for groupID := range groups {
        groupLag, topics, err := kbc.checkConsumerGroupLag(ctx, groupID)
        if err != nil {
            continue // 跳过错误的组
        }
        
        totalLag += groupLag
        state.Details[groupID] = groupLag
        
        // 收集受影响的主题
        for _, topic := range topics {
            if !contains(state.AffectedTopics, topic) {
                state.AffectedTopics = append(state.AffectedTopics, topic)
            }
        }
    }
    
    state.TotalLag = totalLag
    state.HasBacklog = totalLag > kbc.config.MaxLagThreshold
    
    return state, nil
}

func (kbc *kafkaBacklogChecker) checkConsumerGroupLag(ctx context.Context, groupID string) (int64, []string, error) {
    // 获取消费者组详情
    groupDesc, err := kbc.admin.DescribeConsumerGroups([]string{groupID})
    if err != nil {
        return 0, nil, err
    }
    
    group, exists := groupDesc[groupID]
    if !exists {
        return 0, nil, fmt.Errorf("group %s not found", groupID)
    }
    
    var totalLag int64
    var topics []string
    
    // 检查每个成员的分区
    for _, member := range group.Members {
        assignment, err := member.GetMemberAssignment()
        if err != nil {
            continue
        }
        
        for topic, partitions := range assignment.Topics {
            topics = append(topics, topic)
            
            for _, partition := range partitions {
                lag, err := kbc.getPartitionLag(topic, partition)
                if err != nil {
                    continue
                }
                totalLag += lag
            }
        }
    }
    
    return totalLag, topics, nil
}

func (kbc *kafkaBacklogChecker) getPartitionLag(topic string, partition int32) (int64, error) {
    // 获取最新偏移量
    latestOffset, err := kbc.client.GetOffset(topic, partition, sarama.OffsetNewest)
    if err != nil {
        return 0, err
    }
    
    // 获取消费者偏移量
    coordinator, err := kbc.client.Coordinator("your-consumer-group") // 需要传入实际的消费者组
    if err != nil {
        return 0, err
    }
    
    request := &sarama.OffsetFetchRequest{
        Version:       1,
        ConsumerGroup: "your-consumer-group",
    }
    request.AddPartition(topic, partition)
    
    response, err := coordinator.FetchOffset(request)
    if err != nil {
        return 0, err
    }
    
    block := response.GetBlock(topic, partition)
    if block == nil {
        return 0, fmt.Errorf("no offset block for topic %s partition %d", topic, partition)
    }
    
    if block.Err != sarama.ErrNoError {
        return 0, block.Err
    }
    
    // 计算滞后
    lag := latestOffset - block.Offset
    if lag < 0 {
        lag = 0
    }
    
    return lag, nil
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}
```

### 4. 默认实现

```go
// jxt-core/sdk/pkg/eventbus/default_implementations.go
package eventbus

import (
    "context"
    "encoding/json"
    "fmt"
    "net"
    "os"
    "strings"
    "time"
)

// DefaultMessageRouter 默认消息路由器
type DefaultMessageRouter struct{}

func (r *DefaultMessageRouter) ShouldUseAggregateProcessor(msg MessageContext) bool {
    // 默认策略：如果消息有聚合ID，则使用聚合处理器
    return r.ExtractAggregateID(msg) != ""
}

func (r *DefaultMessageRouter) ExtractAggregateID(msg MessageContext) string {
    // 尝试从多个可能的字段提取聚合ID
    if aggregateID, exists := msg.Metadata["aggregateID"]; exists {
        return aggregateID
    }
    if aggregateID, exists := msg.Metadata["aggregate_id"]; exists {
        return aggregateID
    }
    if aggregateID, exists := msg.Metadata["AggregateID"]; exists {
        return aggregateID
    }
    return ""
}

func (r *DefaultMessageRouter) GetProcessingTimeout(msg MessageContext) time.Duration {
    // 默认超时时间
    return 30 * time.Second
}

// DefaultErrorHandler 默认错误处理器
type DefaultErrorHandler struct{}

func (h *DefaultErrorHandler) IsRetryable(err error) bool {
    // 默认重试策略
    switch err.(type) {
    case *json.SyntaxError, *json.UnmarshalTypeError:
        return false // JSON错误不重试
    case *net.OpError, *os.SyscallError:
        return true // 网络/系统错误重试
    default:
        // 检查错误消息
        errMsg := strings.ToLower(err.Error())
        if strings.Contains(errMsg, "timeout") ||
           strings.Contains(errMsg, "connection") ||
           strings.Contains(errMsg, "network") {
            return true
        }
        return false
    }
}

func (h *DefaultErrorHandler) HandleRetryableError(ctx context.Context, msg MessageContext, err error) error {
    // 默认重试处理：记录日志
    fmt.Printf("Retryable error for topic %s: %v\n", msg.Topic, err)
    return nil // 使用默认重试机制
}

func (h *DefaultErrorHandler) HandleNonRetryableError(ctx context.Context, msg MessageContext, err error) error {
    // 默认非重试处理：发送到死信队列
    fmt.Printf("Non-retryable error for topic %s: %v\n", msg.Topic, err)
    return h.SendToDeadLetter(ctx, msg, err.Error())
}

func (h *DefaultErrorHandler) SendToDeadLetter(ctx context.Context, msg MessageContext, reason string) error {
    // 默认死信队列处理：记录日志
    fmt.Printf("Sending message to dead letter queue - Topic: %s, Reason: %s\n", msg.Topic, reason)
    
    // 这里可以实现实际的死信队列逻辑
    // 比如发布到 {topic}-dead-letter 主题
    
    return nil
}
```

## 使用示例

### 业务层使用示例

```go
// evidence-management/internal/infrastructure/eventbus/setup.go
package eventbus

import (
    "context"
    "time"
    "jxt-core/sdk/pkg/eventbus"
    "jxt-core/sdk/config"
)

func SetupAdvancedEventBus() (eventbus.AdvancedEventBus, error) {
    // 加载配置
    cfg := config.GetDefaultAdvancedEventBusConfig()
    
    // 自定义业务配置
    cfg.BacklogDetection.MaxLagThreshold = 100
    cfg.AggregateProcessor.CacheSize = 500
    cfg.RateLimit.Limit = 2000
    
    // 创建高级事件总线
    bus, err := eventbus.NewKafkaAdvancedEventBus(cfg)
    if err != nil {
        return nil, err
    }
    
    // 设置业务特定的组件
    bus.SetMessageRouter(&EvidenceMessageRouter{})
    bus.SetErrorHandler(&EvidenceErrorHandler{})
    
    // 注册回调
    bus.RegisterBacklogCallback(handleBacklogChange)
    bus.RegisterRecoveryModeCallback(handleRecoveryModeChange)
    
    // 启动积压监控
    if err := bus.StartBacklogMonitoring(context.Background()); err != nil {
        return nil, err
    }
    
    return bus, nil
}

func handleBacklogChange(ctx context.Context, state eventbus.BacklogState) error {
    if state.HasBacklog {
        // 发送告警
        sendAlert("Backlog detected", fmt.Sprintf("Total lag: %d", state.TotalLag))
    }
    return nil
}

func handleRecoveryModeChange(ctx context.Context, isRecovery bool) error {
    if isRecovery {
        // 进入恢复模式的业务逻辑
        pauseNonCriticalTasks()
    } else {
        // 退出恢复模式的业务逻辑
        resumeNormalOperations()
    }
    return nil
}

// 订阅示例
func SubscribeToMediaEvents(bus eventbus.AdvancedEventBus, handler MediaEventHandler) error {
    wrappedHandler := func(ctx context.Context, message []byte) error {
        return handler.HandleMediaEvent(ctx, message)
    }
    
    opts := eventbus.SubscribeOptions{
        UseAggregateProcessor: true,
        ProcessingTimeout:     60 * time.Second,
        RateLimit:            1000,
        RateBurst:            1000,
        RetryPolicy: eventbus.RetryPolicy{
            MaxRetries:      3,
            InitialInterval: 1 * time.Second,
            MaxInterval:     30 * time.Second,
            Multiplier:      2.0,
        },
        DeadLetterEnabled: true,
    }
    
    return bus.SubscribeWithOptions(
        context.Background(),
        "media_events",
        wrappedHandler,
        opts,
    )
}
```

这些代码示例展示了高级事件总线的核心实现，包括聚合处理器管理、恢复模式管理、积压检测等关键功能。通过这些组件，可以实现一个功能完整、性能优异的企业级事件总线系统。
