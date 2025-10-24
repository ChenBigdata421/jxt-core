package outbox

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PublisherConfig 发布器配置
type PublisherConfig struct {
	// MaxRetries 最大重试次数
	MaxRetries int

	// RetryDelay 重试延迟
	RetryDelay time.Duration

	// PublishTimeout 发布超时时间
	PublishTimeout time.Duration

	// EnableMetrics 是否启用指标收集
	EnableMetrics bool

	// MetricsCollector 指标收集器（可选）
	// 用于集成 Prometheus、StatsD 等监控系统
	MetricsCollector MetricsCollector

	// ErrorHandler 错误处理器（可选）
	ErrorHandler func(event *OutboxEvent, err error)

	// ConcurrentPublish 是否启用并发发布（默认 false）
	ConcurrentPublish bool

	// PublishConcurrency 并发发布的并发数（默认 10）
	// 只有当 ConcurrentPublish = true 时生效
	PublishConcurrency int
}

// Validate 验证配置
func (c *PublisherConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("publisher config is nil")
	}

	// 验证 MaxRetries
	if c.MaxRetries < 0 {
		return fmt.Errorf("MaxRetries must be >= 0, got %d", c.MaxRetries)
	}
	if c.MaxRetries > 100 {
		return fmt.Errorf("MaxRetries is too large (max 100), got %d", c.MaxRetries)
	}

	// 验证 RetryDelay
	if c.RetryDelay < 0 {
		return fmt.Errorf("RetryDelay must be >= 0, got %v", c.RetryDelay)
	}
	if c.RetryDelay > 1*time.Hour {
		return fmt.Errorf("RetryDelay is too large (max 1 hour), got %v", c.RetryDelay)
	}

	// 验证 PublishTimeout
	if c.PublishTimeout < 0 {
		return fmt.Errorf("PublishTimeout must be >= 0, got %v", c.PublishTimeout)
	}
	if c.PublishTimeout > 5*time.Minute {
		return fmt.Errorf("PublishTimeout is too large (max 5 minutes), got %v", c.PublishTimeout)
	}

	return nil
}

// DefaultPublisherConfig 默认发布器配置
func DefaultPublisherConfig() *PublisherConfig {
	return &PublisherConfig{
		MaxRetries:         3,
		RetryDelay:         time.Second,
		PublishTimeout:     30 * time.Second,
		EnableMetrics:      true,
		ConcurrentPublish:  false, // 默认不启用并发发布（保持向后兼容）
		PublishConcurrency: 10,    // 默认并发数 10
	}
}

// OutboxPublisher Outbox 事件发布器
// 负责将 Outbox 事件发布到 EventBus
type OutboxPublisher struct {
	// repo 仓储
	repo OutboxRepository

	// eventPublisher 事件发布器（接口，依赖注入）
	eventPublisher EventPublisher

	// topicMapper Topic 映射器
	topicMapper TopicMapper

	// config 配置
	config *PublisherConfig

	// metrics 指标收集器（可选）
	metrics *PublisherMetrics

	// metricsCollector 外部指标收集器（可选）
	metricsCollector MetricsCollector

	// ackListenerCtx ACK 监听器上下文
	ackListenerCtx context.Context

	// ackListenerCancel ACK 监听器取消函数
	ackListenerCancel context.CancelFunc

	// ackListenerStarted ACK 监听器是否已启动
	ackListenerStarted bool

	// ackListenerMu ACK 监听器互斥锁
	ackListenerMu sync.Mutex
}

// PublisherMetrics 发布器指标
type PublisherMetrics struct {
	// PublishedCount 已发布事件数量
	PublishedCount int64

	// FailedCount 失败事件数量
	FailedCount int64

	// ErrorCount 错误事件数量（批量发布时使用）
	ErrorCount int64

	// RetryCount 重试次数
	RetryCount int64

	// LastPublishTime 最后发布时间
	LastPublishTime time.Time

	// LastError 最后错误
	LastError error
}

// NewOutboxPublisher 创建 Outbox 发布器
//
// 参数：
//
//	repo: 仓储
//	eventPublisher: 事件发布器（接口）
//	topicMapper: Topic 映射器
//	config: 配置（可选，nil 表示使用默认配置）
//
// 返回：
//
//	*OutboxPublisher: 发布器实例
//
// Panics:
//
//	如果配置验证失败，会 panic
func NewOutboxPublisher(
	repo OutboxRepository,
	eventPublisher EventPublisher,
	topicMapper TopicMapper,
	config *PublisherConfig,
) *OutboxPublisher {
	if config == nil {
		config = DefaultPublisherConfig()
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		panic(fmt.Sprintf("invalid publisher config: %v", err))
	}

	publisher := &OutboxPublisher{
		repo:           repo,
		eventPublisher: eventPublisher,
		topicMapper:    topicMapper,
		config:         config,
	}

	if config.EnableMetrics {
		publisher.metrics = &PublisherMetrics{}
	}

	// 设置外部指标收集器
	if config.MetricsCollector != nil {
		publisher.metricsCollector = config.MetricsCollector
	} else {
		publisher.metricsCollector = &NoOpMetricsCollector{}
	}

	return publisher
}

// PublishEvent 发布单个事件
//
// 参数：
//
//	ctx: 上下文
//	event: 要发布的事件
//
// 返回：
//
//	error: 发布失败时返回错误
func (p *OutboxPublisher) PublishEvent(ctx context.Context, event *OutboxEvent) error {
	// 记录开始时间（用于计算耗时）
	startTime := time.Now()

	// 1. 幂等性检查：检查事件是否已经发布过
	if event.IdempotencyKey != "" {
		existingEvent, err := p.repo.FindByIdempotencyKey(ctx, event.IdempotencyKey)
		if err != nil {
			return fmt.Errorf("failed to check idempotency: %w", err)
		}

		// 如果已经存在且已发布，直接返回成功（幂等性保证）
		if existingEvent != nil && existingEvent.IsPublished() {
			return nil
		}

		// 如果已经存在但未发布，使用现有事件继续发布
		if existingEvent != nil {
			event = existingEvent
		}
	}

	// 2. 检查事件是否应该立即发布
	if !event.ShouldPublishNow() {
		return fmt.Errorf("event %s is scheduled for later", event.ID)
	}

	// 3. 获取 Topic
	topic := p.topicMapper.GetTopic(event.AggregateType)
	if topic == "" {
		return fmt.Errorf("no topic found for aggregate type: %s", event.AggregateType)
	}

	// 4. 创建 Envelope
	envelope := p.toEnvelope(event)

	// 5. 创建带超时的上下文
	publishCtx := ctx
	if p.config.PublishTimeout > 0 {
		var cancel context.CancelFunc
		publishCtx, cancel = context.WithTimeout(ctx, p.config.PublishTimeout)
		defer cancel()
	}

	// 6. 发布事件（通过 EventPublisher 接口）
	// ✅ 使用 PublishEnvelope() 支持 Outbox 模式
	// ✅ 异步发布，立即返回，ACK 结果通过 GetPublishResultChannel() 异步通知
	if err := p.eventPublisher.PublishEnvelope(publishCtx, topic, envelope); err != nil {
		// 提交失败（注意：不是 ACK 失败，而是提交到发送队列失败）
		event.MarkAsFailed(err)
		if updateErr := p.repo.Update(ctx, event); updateErr != nil {
			return fmt.Errorf("failed to update event after publish error: %w", updateErr)
		}

		// 调用错误处理器
		if p.config.ErrorHandler != nil {
			p.config.ErrorHandler(event, err)
		}

		// 更新指标
		if p.metrics != nil {
			p.metrics.FailedCount++
			p.metrics.LastError = err
		}

		// 记录失败指标到外部收集器
		p.metricsCollector.RecordFailed(event.TenantID, event.AggregateType, event.EventType, err)

		return fmt.Errorf("failed to publish event: %w", err)
	}

	// ✅ 提交成功（注意：这里不标记为 Published，等待 ACK 监听器处理）
	// ACK 成功后，ACK 监听器会调用 repo.MarkAsPublished()
	// ACK 失败时，事件保持 Pending 状态，等待下次重试
	// 这是乐观更新策略：假设发布会成功，失败时通过 ACK 监听器回滚

	// 计算耗时
	duration := time.Since(startTime)

	// 记录发布耗时到外部收集器
	p.metricsCollector.RecordPublishDuration(event.TenantID, event.AggregateType, event.EventType, duration)

	return nil
}

// PublishBatch 批量发布事件（优化版本）
//
// 参数：
//
//	ctx: 上下文
//	events: 要发布的事件列表
//
// 返回：
//
//	int: 成功发布的事件数量
//	error: 发布失败时返回错误
//
// 性能优化：
//  1. 批量幂等性检查（一次查询）
//  2. 批量发布到 EventBus
//  3. 批量更新数据库状态（一次更新）
func (p *OutboxPublisher) PublishBatch(ctx context.Context, events []*OutboxEvent) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	// 1. 批量幂等性检查（过滤已发布的事件）
	eventsToPublish, err := p.filterPublishedEvents(ctx, events)
	if err != nil {
		return 0, fmt.Errorf("failed to filter published events: %w", err)
	}

	if len(eventsToPublish) == 0 {
		return 0, nil // 所有事件都已发布
	}

	// 2. 批量发布到 EventBus（根据配置选择串行或并发）
	var publishedEvents, failedEvents []*OutboxEvent
	if p.config.ConcurrentPublish {
		publishedEvents, failedEvents = p.batchPublishToEventBusConcurrent(ctx, eventsToPublish)
	} else {
		publishedEvents, failedEvents = p.batchPublishToEventBus(ctx, eventsToPublish)
	}

	// 3. 批量更新成功发布的事件状态
	if len(publishedEvents) > 0 {
		if err := p.batchUpdatePublished(ctx, publishedEvents); err != nil {
			// 更新失败不影响发布结果（事件已经发布到 EventBus）
			// 下次轮询时会重新发布（幂等性保证不会重复）
			if p.metrics != nil {
				p.metrics.LastError = err
			}
		}
	}

	// 4. 批量更新失败事件状态
	if len(failedEvents) > 0 {
		p.batchUpdateFailed(ctx, failedEvents)
	}

	// 5. 更新指标
	if p.metrics != nil {
		p.metrics.PublishedCount += int64(len(publishedEvents))
		p.metrics.ErrorCount += int64(len(failedEvents))
		p.metrics.LastPublishTime = time.Now()
	}

	// 返回成功数量和最后一个错误
	var lastErr error
	if len(failedEvents) > 0 {
		lastErr = fmt.Errorf("failed to publish %d events", len(failedEvents))
	}

	return len(publishedEvents), lastErr
}

// PublishPendingEvents 发布所有待发布的事件
//
// 参数：
//
//	ctx: 上下文
//	limit: 最大发布数量
//	tenantID: 租户 ID（可选）
//
// 返回：
//
//	int: 成功发布的事件数量
//	error: 发布失败时返回错误
func (p *OutboxPublisher) PublishPendingEvents(ctx context.Context, limit int, tenantID string) (int, error) {
	// 查找待发布的事件
	events, err := p.repo.FindPendingEvents(ctx, limit, tenantID)
	if err != nil {
		return 0, fmt.Errorf("failed to find pending events: %w", err)
	}

	if len(events) == 0 {
		return 0, nil
	}

	// 批量发布
	return p.PublishBatch(ctx, events)
}

// filterPublishedEvents 过滤已发布的事件（批量幂等性检查）
func (p *OutboxPublisher) filterPublishedEvents(ctx context.Context, events []*OutboxEvent) ([]*OutboxEvent, error) {
	var eventsToPublish []*OutboxEvent

	for _, event := range events {
		// 跳过没有幂等性键的事件（直接发布）
		if event.IdempotencyKey == "" {
			eventsToPublish = append(eventsToPublish, event)
			continue
		}

		// 检查是否已发布
		existingEvent, err := p.repo.FindByIdempotencyKey(ctx, event.IdempotencyKey)
		if err != nil {
			return nil, err
		}

		// 如果已发布，跳过
		if existingEvent != nil && existingEvent.IsPublished() {
			continue
		}

		eventsToPublish = append(eventsToPublish, event)
	}

	return eventsToPublish, nil
}

// batchPublishToEventBus 批量发布到 EventBus
func (p *OutboxPublisher) batchPublishToEventBus(ctx context.Context, events []*OutboxEvent) (published []*OutboxEvent, failed []*OutboxEvent) {
	for _, event := range events {
		// 检查是否应该立即发布
		if !event.ShouldPublishNow() {
			failed = append(failed, event)
			continue
		}

		// 获取 Topic
		topic := p.topicMapper.GetTopic(event.AggregateType)
		if topic == "" {
			event.MarkAsFailed(fmt.Errorf("no topic found for aggregate type: %s", event.AggregateType))
			failed = append(failed, event)
			continue
		}

		// 创建 Envelope
		envelope := p.toEnvelope(event)

		// 创建带超时的上下文
		publishCtx := ctx
		if p.config.PublishTimeout > 0 {
			var cancel context.CancelFunc
			publishCtx, cancel = context.WithTimeout(ctx, p.config.PublishTimeout)
			defer cancel()
		}

		// 发布事件（使用 PublishEnvelope）
		if err := p.eventPublisher.PublishEnvelope(publishCtx, topic, envelope); err != nil {
			event.MarkAsFailed(fmt.Errorf("failed to publish: %w", err))
			failed = append(failed, event)
			continue
		}

		// ✅ 提交成功（等待 ACK 监听器处理）
		published = append(published, event)
	}

	return published, failed
}

// batchPublishToEventBusConcurrent 并发批量发布到 EventBus（性能优化版本）
func (p *OutboxPublisher) batchPublishToEventBusConcurrent(ctx context.Context, events []*OutboxEvent) (published []*OutboxEvent, failed []*OutboxEvent) {
	if len(events) == 0 {
		return nil, nil
	}

	concurrency := p.config.PublishConcurrency
	if concurrency <= 0 {
		concurrency = 10 // 默认并发数
	}

	var (
		wg            sync.WaitGroup
		publishedChan = make(chan *OutboxEvent, len(events))
		failedChan    = make(chan *OutboxEvent, len(events))
		semaphore     = make(chan struct{}, concurrency) // 限制并发数
	)

	// 并发发布
	for _, event := range events {
		wg.Add(1)
		go func(evt *OutboxEvent) {
			defer wg.Done()

			// 获取信号量（限流）
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 发布单个事件
			if err := p.publishSingleEventToEventBus(ctx, evt); err != nil {
				failedChan <- evt
			} else {
				publishedChan <- evt
			}
		}(event)
	}

	// 等待所有发布完成
	go func() {
		wg.Wait()
		close(publishedChan)
		close(failedChan)
	}()

	// 收集结果（免锁，使用 channel）
	for evt := range publishedChan {
		published = append(published, evt)
	}
	for evt := range failedChan {
		failed = append(failed, evt)
	}

	return published, failed
}

// publishSingleEventToEventBus 发布单个事件到 EventBus（内部方法）
func (p *OutboxPublisher) publishSingleEventToEventBus(ctx context.Context, event *OutboxEvent) error {
	// 检查是否应该立即发布
	if !event.ShouldPublishNow() {
		err := fmt.Errorf("event is scheduled for later")
		event.MarkAsFailed(err)
		return err
	}

	// 获取 Topic
	topic := p.topicMapper.GetTopic(event.AggregateType)
	if topic == "" {
		err := fmt.Errorf("no topic found for aggregate type: %s", event.AggregateType)
		event.MarkAsFailed(err)
		return err
	}

	// 创建 Envelope
	envelope := p.toEnvelope(event)

	// 创建带超时的上下文
	publishCtx := ctx
	if p.config.PublishTimeout > 0 {
		var cancel context.CancelFunc
		publishCtx, cancel = context.WithTimeout(ctx, p.config.PublishTimeout)
		defer cancel()
	}

	// 发布事件（使用 PublishEnvelope）
	if err := p.eventPublisher.PublishEnvelope(publishCtx, topic, envelope); err != nil {
		err = fmt.Errorf("failed to publish: %w", err)
		event.MarkAsFailed(err)
		return err
	}

	// ✅ 提交成功（等待 ACK 监听器处理）
	return nil
}

// batchUpdatePublished 批量更新已发布事件的状态
func (p *OutboxPublisher) batchUpdatePublished(ctx context.Context, events []*OutboxEvent) error {
	// 使用批量更新接口（如果仓储支持）
	if batchRepo, ok := p.repo.(interface {
		BatchUpdate(ctx context.Context, events []*OutboxEvent) error
	}); ok {
		return batchRepo.BatchUpdate(ctx, events)
	}

	// 降级到逐个更新
	for _, event := range events {
		if err := p.repo.Update(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

// batchUpdateFailed 批量更新失败事件的状态
func (p *OutboxPublisher) batchUpdateFailed(ctx context.Context, events []*OutboxEvent) {
	for _, event := range events {
		// 忽略更新错误，下次轮询时会重试
		_ = p.repo.Update(ctx, event)
	}
}

// RetryFailedEvent 重试失败的事件
//
// 参数：
//
//	ctx: 上下文
//	event: 要重试的事件
//
// 返回：
//
//	error: 重试失败时返回错误
func (p *OutboxPublisher) RetryFailedEvent(ctx context.Context, event *OutboxEvent) error {
	// 检查是否可以重试
	if !event.CanRetry() {
		return fmt.Errorf("event %s has reached max retries", event.ID)
	}

	// 重置为待发布状态
	event.ResetForRetry()
	if err := p.repo.Update(ctx, event); err != nil {
		return fmt.Errorf("failed to reset event for retry: %w", err)
	}

	// 更新指标
	if p.metrics != nil {
		p.metrics.RetryCount++
	}

	// 延迟后重试
	if p.config.RetryDelay > 0 {
		time.Sleep(p.config.RetryDelay)
	}

	// 发布事件
	return p.PublishEvent(ctx, event)
}

// GetMetrics 获取发布器指标
func (p *OutboxPublisher) GetMetrics() *PublisherMetrics {
	if p.metrics == nil {
		return nil
	}
	// 返回副本，避免并发问题
	return &PublisherMetrics{
		PublishedCount:  p.metrics.PublishedCount,
		FailedCount:     p.metrics.FailedCount,
		RetryCount:      p.metrics.RetryCount,
		LastPublishTime: p.metrics.LastPublishTime,
		LastError:       p.metrics.LastError,
	}
}

// StartACKListener 启动 ACK 监听器（使用全局 ACK Channel）
// 监听 EventBus 的异步发布结果，并更新 Outbox 事件状态
//
// 参数：
//
//	ctx: 上下文（用于控制监听器生命周期）
//
// 注意：
//   - 此方法应该在应用启动时调用一次
//   - 监听器会在后台运行，直到 ctx 被取消或调用 StopACKListener()
//   - 重复调用此方法是安全的（会忽略后续调用）
//   - 多租户场景推荐使用 StartACKListenerWithChannel()
func (p *OutboxPublisher) StartACKListener(ctx context.Context) {
	// 使用全局 ACK Channel
	resultChan := p.eventPublisher.GetPublishResultChannel()
	p.StartACKListenerWithChannel(ctx, resultChan)
}

// StartACKListenerWithChannel 启动 ACK 监听器（使用自定义 ACK Channel）
// 监听指定的 ACK Channel，并更新 Outbox 事件状态
//
// 参数：
//
//	ctx: 上下文（用于控制监听器生命周期）
//	resultChan: 自定义的 ACK 结果通道（如租户专属通道）
//
// 注意：
//   - 此方法用于多租户场景，每个租户使用独立的 ACK Channel
//   - 重复调用此方法是安全的（会忽略后续调用）
func (p *OutboxPublisher) StartACKListenerWithChannel(ctx context.Context, resultChan <-chan *PublishResult) {
	p.ackListenerMu.Lock()
	defer p.ackListenerMu.Unlock()

	// 如果已经启动，直接返回
	if p.ackListenerStarted {
		return
	}

	// 创建监听器上下文
	p.ackListenerCtx, p.ackListenerCancel = context.WithCancel(ctx)
	p.ackListenerStarted = true

	// 启动 ACK 监听器 goroutine（使用自定义 Channel）
	go p.ackListenerLoopWithChannel(resultChan)
}

// StopACKListener 停止 ACK 监听器
func (p *OutboxPublisher) StopACKListener() {
	p.ackListenerMu.Lock()
	defer p.ackListenerMu.Unlock()

	if !p.ackListenerStarted {
		return
	}

	// 取消监听器上下文
	if p.ackListenerCancel != nil {
		p.ackListenerCancel()
	}

	p.ackListenerStarted = false
}

// ackListenerLoop ACK 监听器循环（使用全局 ACK Channel）
// 监听 EventBus 的异步发布结果，并更新 Outbox 事件状态
func (p *OutboxPublisher) ackListenerLoop() {
	// 获取发布结果通道
	resultChan := p.eventPublisher.GetPublishResultChannel()
	p.ackListenerLoopWithChannel(resultChan)
}

// ackListenerLoopWithChannel ACK 监听器循环（使用自定义 ACK Channel）
// 监听指定的 ACK Channel，并更新 Outbox 事件状态
func (p *OutboxPublisher) ackListenerLoopWithChannel(resultChan <-chan *PublishResult) {
	for {
		select {
		case result := <-resultChan:
			// 处理 ACK 结果
			p.handleACKResult(result)

		case <-p.ackListenerCtx.Done():
			// 监听器被停止
			return
		}
	}
}

// handleACKResult 处理 ACK 结果
func (p *OutboxPublisher) handleACKResult(result *PublishResult) {
	if result == nil {
		return
	}

	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if result.Success {
		// ACK 成功，标记为已发布
		if err := p.repo.MarkAsPublished(ctx, result.EventID); err != nil {
			// 记录错误（不影响后续处理）
			if p.config.ErrorHandler != nil {
				p.config.ErrorHandler(nil, fmt.Errorf("failed to mark event %s as published: %w", result.EventID, err))
			}
		}

		// 更新指标
		if p.metrics != nil {
			p.metrics.PublishedCount++
			p.metrics.LastPublishTime = time.Now()
		}

		// 更新外部指标收集器
		p.metricsCollector.RecordPublished("", result.AggregateID, result.EventType)

	} else {
		// ACK 失败，记录错误（保持 Pending 状态，等待下次重试）
		if p.config.ErrorHandler != nil {
			p.config.ErrorHandler(nil, fmt.Errorf("publish failed for event %s: %w", result.EventID, result.Error))
		}

		// 更新指标
		if p.metrics != nil {
			p.metrics.FailedCount++
			p.metrics.LastError = result.Error
		}

		// 更新外部指标收集器
		p.metricsCollector.RecordFailed("", result.AggregateID, result.EventType, result.Error)
	}
}

// toEnvelope 将 OutboxEvent 转换为 Envelope
func (p *OutboxPublisher) toEnvelope(event *OutboxEvent) *Envelope {
	return &Envelope{
		EventID:       event.ID,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		EventVersion:  1, // 默认版本 1
		Payload:       event.Payload,
		Timestamp:     event.CreatedAt,
		TraceID:       event.TraceID,
		CorrelationID: event.CorrelationID,
		TenantID:      event.TenantID, // ← 租户ID（多租户支持，用于Outbox ACK路由）
	}
}

// ResetMetrics 重置指标
func (p *OutboxPublisher) ResetMetrics() {
	if p.metrics != nil {
		p.metrics.PublishedCount = 0
		p.metrics.FailedCount = 0
		p.metrics.RetryCount = 0
		p.metrics.LastPublishTime = time.Time{}
		p.metrics.LastError = nil
	}
}
