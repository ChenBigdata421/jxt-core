package outbox

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// DLQHandler 死信队列处理器接口
// 用于处理超过最大重试次数的失败事件
//
// 实现示例：
//
//	type MyDLQHandler struct {
//	    logger *zap.Logger
//	}
//
//	func (h *MyDLQHandler) Handle(ctx context.Context, event *OutboxEvent) error {
//	    // 1. 记录到专门的死信队列表
//	    // 2. 发送到死信队列 Topic
//	    // 3. 记录详细日志
//	    h.logger.Error("Event moved to DLQ",
//	        zap.String("event_id", event.ID),
//	        zap.String("error", event.LastError))
//	    return nil
//	}
type DLQHandler interface {
	// Handle 处理死信事件
	// ctx: 上下文
	// event: 失败的事件
	// 返回：处理失败时返回错误
	Handle(ctx context.Context, event *OutboxEvent) error
}

// DLQAlertHandler 死信队列告警处理器接口
// 用于发送告警通知（邮件、短信、钉钉、企业微信等）
//
// 实现示例：
//
//	type EmailAlertHandler struct {
//	    emailService EmailService
//	}
//
//	func (h *EmailAlertHandler) Alert(ctx context.Context, event *OutboxEvent) error {
//	    subject := fmt.Sprintf("Outbox Event Failed: %s", event.EventType)
//	    body := fmt.Sprintf("Event ID: %s\nError: %s", event.ID, event.LastError)
//	    return h.emailService.Send(subject, body)
//	}
type DLQAlertHandler interface {
	// Alert 发送告警
	// ctx: 上下文
	// event: 失败的事件
	// 返回：发送失败时返回错误
	Alert(ctx context.Context, event *OutboxEvent) error
}

// DLQHandlerFunc 函数式 DLQHandler
// 允许使用函数作为 DLQHandler 实现
type DLQHandlerFunc func(ctx context.Context, event *OutboxEvent) error

// Handle 实现 DLQHandler 接口
func (f DLQHandlerFunc) Handle(ctx context.Context, event *OutboxEvent) error {
	return f(ctx, event)
}

// DLQAlertHandlerFunc 函数式 DLQAlertHandler
// 允许使用函数作为 DLQAlertHandler 实现
type DLQAlertHandlerFunc func(ctx context.Context, event *OutboxEvent) error

// Alert 实现 DLQAlertHandler 接口
func (f DLQAlertHandlerFunc) Alert(ctx context.Context, event *OutboxEvent) error {
	return f(ctx, event)
}

// NoOpDLQAlertHandler 空操作 DLQAlertHandler（默认实现）
type NoOpDLQAlertHandler struct{}

// Alert 实现 DLQAlertHandler 接口（什么都不做）
func (n *NoOpDLQAlertHandler) Alert(ctx context.Context, event *OutboxEvent) error {
	return nil
}

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	// PollInterval 轮询间隔
	PollInterval time.Duration

	// BatchSize 每次处理的事件数量
	BatchSize int

	// TenantID 租户 ID（可选，0 表示处理所有租户）
	TenantID int

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration

	// CleanupRetention 清理保留时间（已发布事件保留多久）
	CleanupRetention time.Duration

	// HealthCheckInterval 健康检查间隔
	HealthCheckInterval time.Duration

	// EnableHealthCheck 是否启用健康检查
	EnableHealthCheck bool

	// EnableCleanup 是否启用自动清理
	EnableCleanup bool

	// EnableMetrics 是否启用指标收集
	EnableMetrics bool

	// EnableRetry 是否启用失败重试
	EnableRetry bool

	// RetryInterval 重试间隔
	RetryInterval time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// EnableDLQ 是否启用死信队列
	EnableDLQ bool

	// DLQInterval 死信队列处理间隔
	DLQInterval time.Duration

	// DLQHandler 死信队列处理器（EnableDLQ=true 时必填；C3 起 Validate 拒绝 nil）
	// 仅作用于 status=dead_lettered 的终态事件（见 processDLQ step2）
	DLQHandler DLQHandler

	// DLQAlertHandler 死信队列告警处理器（可选）
	// 用于发送告警通知
	DLQAlertHandler DLQAlertHandler

	// MetricsCollector 指标收集器（可选）
	// 用于集成 Prometheus、StatsD 等监控系统
	MetricsCollector MetricsCollector

	// ShutdownTimeout 优雅关闭超时时间
	// 调度器停止时等待正在处理的任务完成的最大时间
	// 默认：30 秒
	ShutdownTimeout time.Duration
}

// Validate 验证配置
func (c *SchedulerConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("scheduler config is nil")
	}

	// 验证 PollInterval
	if c.PollInterval <= 0 {
		return fmt.Errorf("PollInterval must be > 0, got %v", c.PollInterval)
	}
	if c.PollInterval < 1*time.Second {
		return fmt.Errorf("PollInterval is too small (min 1 second), got %v", c.PollInterval)
	}
	if c.PollInterval > 1*time.Hour {
		return fmt.Errorf("PollInterval is too large (max 1 hour), got %v", c.PollInterval)
	}

	// 验证 BatchSize
	if c.BatchSize <= 0 {
		return fmt.Errorf("BatchSize must be > 0, got %d", c.BatchSize)
	}
	if c.BatchSize > 10000 {
		return fmt.Errorf("BatchSize is too large (max 10000), got %d", c.BatchSize)
	}

	// 验证 CleanupInterval
	if c.EnableCleanup {
		if c.CleanupInterval <= 0 {
			return fmt.Errorf("CleanupInterval must be > 0 when cleanup is enabled, got %v", c.CleanupInterval)
		}
		if c.CleanupInterval < 1*time.Minute {
			return fmt.Errorf("CleanupInterval is too small (min 1 minute), got %v", c.CleanupInterval)
		}
	}

	// 验证 CleanupRetention
	if c.EnableCleanup {
		if c.CleanupRetention <= 0 {
			return fmt.Errorf("CleanupRetention must be > 0 when cleanup is enabled, got %v", c.CleanupRetention)
		}
		if c.CleanupRetention < 1*time.Hour {
			return fmt.Errorf("CleanupRetention is too small (min 1 hour), got %v", c.CleanupRetention)
		}
	}

	// 验证 HealthCheckInterval
	if c.EnableHealthCheck {
		if c.HealthCheckInterval <= 0 {
			return fmt.Errorf("HealthCheckInterval must be > 0 when health check is enabled, got %v", c.HealthCheckInterval)
		}
		if c.HealthCheckInterval < 1*time.Second {
			return fmt.Errorf("HealthCheckInterval is too small (min 1 second), got %v", c.HealthCheckInterval)
		}
	}

	// 验证 RetryInterval
	if c.EnableRetry {
		if c.RetryInterval <= 0 {
			return fmt.Errorf("RetryInterval must be > 0 when retry is enabled, got %v", c.RetryInterval)
		}
		if c.RetryInterval < 1*time.Second {
			return fmt.Errorf("RetryInterval is too small (min 1 second), got %v", c.RetryInterval)
		}
	}

	// 验证 MaxRetries
	if c.EnableRetry {
		if c.MaxRetries < 0 {
			return fmt.Errorf("MaxRetries must be >= 0, got %d", c.MaxRetries)
		}
		if c.MaxRetries > 100 {
			return fmt.Errorf("MaxRetries is too large (max 100), got %d", c.MaxRetries)
		}
	}

	// 验证 DLQ 配置（C3：EnableDLQ 必须配 DLQHandler）
	if c.EnableDLQ {
		if c.DLQInterval <= 0 {
			return fmt.Errorf("DLQInterval must be > 0 when DLQ is enabled, got %v", c.DLQInterval)
		}
		if c.DLQInterval < 1*time.Second {
			return fmt.Errorf("DLQInterval is too small (min 1 second), got %v", c.DLQInterval)
		}
		if c.DLQHandler == nil {
			return fmt.Errorf("DLQHandler must not be nil when EnableDLQ is true (NoOpDLQHandler was removed; silent swallow is a bug, not a default)")
		}
	}

	// 验证 ShutdownTimeout
	if c.ShutdownTimeout < 0 {
		return fmt.Errorf("ShutdownTimeout must be >= 0, got %v", c.ShutdownTimeout)
	}
	if c.ShutdownTimeout > 5*time.Minute {
		return fmt.Errorf("ShutdownTimeout is too large (max 5 minutes), got %v", c.ShutdownTimeout)
	}

	return nil
}

// DefaultSchedulerConfig 默认调度器配置
func DefaultSchedulerConfig() *SchedulerConfig {
	return &SchedulerConfig{
		PollInterval:        10 * time.Second,
		BatchSize:           100,
		TenantID:            0,
		CleanupInterval:     1 * time.Hour,
		CleanupRetention:    24 * time.Hour,
		HealthCheckInterval: 30 * time.Second,
		EnableHealthCheck:   true,
		EnableCleanup:       true,
		EnableMetrics:       true,
		EnableRetry:         true,
		RetryInterval:       30 * time.Second,
		MaxRetries:          3,
		EnableDLQ:           false, // C3: DLQ 改为 opt-in；启用须显式提供 DLQHandler
		DLQInterval:         5 * time.Minute,
		DLQHandler:          nil, // C3: 不再默认注入 NoOp（静默吞是 bug 不是默认）
		DLQAlertHandler:     &NoOpDLQAlertHandler{},
		ShutdownTimeout:     30 * time.Second,
	}
}

// OutboxScheduler Outbox 调度器
// 负责定时轮询待发布的事件并触发发布
type OutboxScheduler struct {
	// publisher 发布器
	publisher *OutboxPublisher

	// repo 仓储
	repo OutboxRepository

	// config 配置
	config *SchedulerConfig

	// running 是否正在运行（使用原子操作，免锁）
	running atomic.Bool

	// stopCh 停止信号
	stopCh chan struct{}

	// doneCh 完成信号
	doneCh chan struct{}

	// wg 等待组（用于优雅关闭）
	wg sync.WaitGroup

	// metrics 指标
	metrics *SchedulerMetrics
}

// SchedulerMetrics 调度器指标
// 使用 atomic 操作保证并发安全，避免数据竞争
type SchedulerMetrics struct {
	// PollCount 轮询次数（原子操作）
	PollCount atomic.Int64

	// ProcessedCount 处理的事件数量（原子操作）
	ProcessedCount atomic.Int64

	// ErrorCount 错误次数（原子操作）
	ErrorCount atomic.Int64

	// LastPollTime 最后轮询时间（原子操作）
	LastPollTime atomic.Value // time.Time

	// LastCleanupTime 最后清理时间（原子操作）
	LastCleanupTime atomic.Value // time.Time

	// RetryCount 重试次数（原子操作）
	RetryCount atomic.Int64

	// RetriedCount 重试成功的事件数量（原子操作）
	RetriedCount atomic.Int64

	// LastRetryTime 最后重试时间（原子操作）
	LastRetryTime atomic.Value // time.Time

	// LastError 最后错误（原子操作）
	LastError atomic.Value // error
}

// NewSchedulerMetrics 创建调度器指标
func NewSchedulerMetrics() *SchedulerMetrics {
	m := &SchedulerMetrics{}
	// 初始化 atomic.Value
	m.LastPollTime.Store(time.Time{})
	m.LastCleanupTime.Store(time.Time{})
	m.LastRetryTime.Store(time.Time{})
	// 注意：不能存储 error(nil)，因为 atomic.Value 不允许存储 typed nil
	// 如果需要存储错误，应该存储具体的错误值或者不初始化（保持零值）
	// m.LastError 保持零值，使用时需要通过 Load() 检查是否为 nil
	return m
}

// NewScheduler 创建调度器
//
// 参数：
//
//	options: 函数式选项
//
// 示例：
//
//	scheduler := outbox.NewScheduler(
//	    outbox.WithRepository(repo),
//	    outbox.WithEventPublisher(eventPublisher),
//	    outbox.WithTopicMapper(topicMapper),
//	    outbox.WithSchedulerConfig(config),
//	)
func NewScheduler(options ...SchedulerOption) *OutboxScheduler {
	opts := &schedulerOptions{}
	for _, option := range options {
		option(opts)
	}

	// 验证必需的选项
	if opts.repo == nil {
		panic("repository is required")
	}
	if opts.eventPublisher == nil {
		panic("event publisher is required")
	}
	if opts.topicMapper == nil {
		opts.topicMapper = DefaultTopicMapper
	}
	if opts.schedulerConfig == nil {
		opts.schedulerConfig = DefaultSchedulerConfig()
	}
	if opts.publisherConfig == nil {
		opts.publisherConfig = DefaultPublisherConfig()
	}

	// 验证调度器配置
	if err := opts.schedulerConfig.Validate(); err != nil {
		panic(fmt.Sprintf("invalid scheduler config: %v", err))
	}

	// 创建发布器（发布器配置会在 NewOutboxPublisher 中验证）
	publisher := NewOutboxPublisher(
		opts.repo,
		opts.eventPublisher,
		opts.topicMapper,
		opts.publisherConfig,
	)

	scheduler := &OutboxScheduler{
		publisher: publisher,
		repo:      opts.repo,
		config:    opts.schedulerConfig,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}

	if opts.schedulerConfig.EnableMetrics {
		scheduler.metrics = NewSchedulerMetrics()
	}

	return scheduler
}

// Start 启动调度器
func (s *OutboxScheduler) Start(ctx context.Context) error {
	// 使用 CAS 操作，原子性检查并设置
	// CompareAndSwap(old, new) 如果当前值等于 old，则设置为 new 并返回 true
	if !s.running.CompareAndSwap(false, true) {
		return fmt.Errorf("scheduler is already running")
	}

	// 启动轮询协程
	go s.pollLoop(ctx)

	// 启动清理协程
	if s.config.EnableCleanup {
		go s.cleanupLoop(ctx)
	}

	// 启动健康检查协程
	if s.config.EnableHealthCheck {
		go s.healthCheckLoop(ctx)
	}

	// 启动失败重试协程
	if s.config.EnableRetry {
		go s.retryLoop(ctx)
	}

	// 启动死信队列处理协程
	if s.config.EnableDLQ {
		go s.dlqLoop(ctx)
	}

	return nil
}

// Stop 停止调度器（优雅关闭）
//
// 优雅关闭流程：
// 1. 设置 running = false，阻止新任务启动
// 2. 发送停止信号到所有循环
// 3. 等待所有正在进行的任务完成（使用 WaitGroup）
// 4. 如果超时，强制退出
//
// 参数：
//
//	ctx: 上下文（用于外部取消）
//
// 返回：
//
//	error: 停止失败时返回错误
func (s *OutboxScheduler) Stop(ctx context.Context) error {
	// 使用 CAS 操作，原子性检查并设置
	if !s.running.CompareAndSwap(true, false) {
		return fmt.Errorf("scheduler is not running")
	}

	// 发送停止信号
	close(s.stopCh)

	// 创建超时上下文
	shutdownTimeout := s.config.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 30 * time.Second // 默认 30 秒
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// 等待所有任务完成
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	// 等待完成或超时
	select {
	case <-done:
		// 所有任务已完成
		return nil
	case <-shutdownCtx.Done():
		// 优雅关闭超时
		return fmt.Errorf("graceful shutdown timeout after %v", shutdownTimeout)
	case <-ctx.Done():
		// 外部取消
		return ctx.Err()
	}
}

// IsRunning 判断是否正在运行（免锁版本）
func (s *OutboxScheduler) IsRunning() bool {
	// 使用原子操作读取，完全免锁
	return s.running.Load()
}

// pollLoop 轮询循环
func (s *OutboxScheduler) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()
	defer close(s.doneCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll 执行一次轮询
func (s *OutboxScheduler) poll(ctx context.Context) {
	// 增加 WaitGroup 计数（优雅关闭支持）
	s.wg.Add(1)
	defer s.wg.Done()

	// 更新指标（使用原子操作）
	if s.metrics != nil {
		s.metrics.PollCount.Add(1)
		s.metrics.LastPollTime.Store(time.Now())
	}

	// 发布待发布的事件
	count, err := s.publisher.PublishPendingEvents(ctx, s.config.BatchSize, s.config.TenantID)
	if err != nil {
		if s.metrics != nil {
			s.metrics.ErrorCount.Add(1)
			s.metrics.LastError.Store(err)
		}
		// 记录错误但继续运行
		return
	}

	// 更新指标（使用原子操作）
	if s.metrics != nil {
		s.metrics.ProcessedCount.Add(int64(count))
	}
}

// cleanupLoop 清理循环
func (s *OutboxScheduler) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup(ctx)
		}
	}
}

// cleanup 执行清理
func (s *OutboxScheduler) cleanup(ctx context.Context) {
	// 增加 WaitGroup 计数（优雅关闭支持）
	s.wg.Add(1)
	defer s.wg.Done()

	before := time.Now().Add(-s.config.CleanupRetention)

	// 删除已发布的事件
	_, err := s.repo.DeletePublishedBefore(ctx, before, s.config.TenantID)
	if err != nil {
		if s.metrics != nil {
			s.metrics.LastError.Store(err)
		}
		return
	}

	// 更新指标（使用原子操作）
	if s.metrics != nil {
		s.metrics.LastCleanupTime.Store(time.Now())
	}
}

// retryLoop 失败重试循环
func (s *OutboxScheduler) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.retryFailedEvents(ctx)
		}
	}
}

// retryFailedEvents 重试失败的事件
func (s *OutboxScheduler) retryFailedEvents(ctx context.Context) {
	// 增加 WaitGroup 计数（优雅关闭支持）
	s.wg.Add(1)
	defer s.wg.Done()

	// 更新指标（使用原子操作）
	if s.metrics != nil {
		s.metrics.RetryCount.Add(1)
		s.metrics.LastRetryTime.Store(time.Now())
	}

	// 查找需要重试的失败事件
	events, err := s.repo.FindEventsForRetry(ctx, s.config.MaxRetries, s.config.BatchSize)
	if err != nil {
		if s.metrics != nil {
			s.metrics.LastError.Store(err)
		}
		return
	}

	// 重试每个事件
	for _, event := range events {
		// 检查是否可以重试
		if !event.CanRetry() {
			// 标记为超过最大重试次数
			if err := s.repo.MarkAsMaxRetry(ctx, event.ID, event.LastError); err != nil {
				if s.metrics != nil {
					s.metrics.LastError.Store(err)
				}
			}
			continue
		}

		// 尝试发布事件
		if err := s.publisher.PublishEvent(ctx, event); err != nil {
			// 发布失败，增加重试次数
			if err := s.repo.IncrementRetry(ctx, event.ID, err.Error()); err != nil {
				if s.metrics != nil {
					s.metrics.LastError.Store(err)
				}
			}
		} else {
			// 发布成功，更新指标（使用原子操作）
			if s.metrics != nil {
				s.metrics.RetriedCount.Add(1)
			}
		}
	}
}

// healthCheckLoop 健康检查循环
func (s *OutboxScheduler) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.healthCheck(ctx)
		}
	}
}

// healthCheck 执行健康检查
func (s *OutboxScheduler) healthCheck(ctx context.Context) {
	// 增加 WaitGroup 计数（优雅关闭支持）
	s.wg.Add(1)
	defer s.wg.Done()

	// 检查待发布事件数量
	count, err := s.repo.Count(ctx, EventStatusPending, s.config.TenantID)
	if err != nil {
		if s.metrics != nil {
			s.metrics.LastError.Store(err)
		}
		return
	}

	// 如果待发布事件过多，记录警告
	if count > int64(s.config.BatchSize*10) {
		// TODO: 记录警告日志
		_ = count
	}
}

// dlqLoop 死信队列处理循环
func (s *OutboxScheduler) dlqLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.DLQInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runDLQWithRecover(ctx)
		}
	}
}

// runDLQWithRecover 在 dlqLoop goroutine 顶层兜 panic。processOneDLQ 的 defer-recover 只覆盖
// Handle/Alert/MarkDeadLetterNotified（用户代码 + 通知标记），不覆盖 step1/扫描里的
// FindMaxRetryEvents/MarkAsDeadLettered/FindUnnotifiedDeadLettered（仓储调用）。该 goroutine 无
// supervisor，仓储层（或自定义 OutboxRepository 实现）panic 会杀死整个死信通知循环、wg.Done 不触发。
// 顶层 recover 让本 tick 失败、记 LastError、下一 tick 继续——比 loop 死亡可观测、可恢复。
func (s *OutboxScheduler) runDLQWithRecover(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			if s.metrics != nil {
				s.metrics.LastError.Store(fmt.Errorf("processDLQ top-level panic: %v", r))
			}
		}
	}()
	s.processDLQ(ctx)
}

// processDLQ 处理死信队列（C1 重写：CAS 推进终态 + 独立通知补发）。
//
// 两步拆分（spec §2.2）：
//  1. 先把 max_retry CAS 成 dead_lettered（只转一次，幂等）——「发布已彻底失败」的事实。
//  2. 再扫描 dead_lettered 且 dlq_notified_at IS NULL 的行，调用 Handle + Alert；成功才标记通知。
//
// 不引入 dead_lettering 中间态：原方案的 claim 只匹配 max_retry，一旦 Handle 失败或进程崩溃，
// dead_lettering 永远不会被下一轮重新认领。拆成两个独立事实后，crash 在通知步骤 → 下一轮补发。
//
// ⚠️ 多实例限制（OV#6）：step1 的 MarkAsDeadLettered 是 CAS（多实例安全）；但 step2 的
// FindUnnotifiedDeadLettered 无 claim/lease——两个 scheduler 实例会同时取到同一批未通知行、同时
// 调 Handle（double Handle）。PR-1 不引入 SELECT FOR UPDATE SKIP LOCKED（属 PR-2 durable claim
// 范围，见 TODOS 'file-storage Outbox ACK batching' / 'Outbox durable DB claim/lease'）。
// 故本方法仅保证【单实例】下 Handle 恰一次；多实例场景依赖 Handle/Alert 按 EventID 幂等
// （PR-2 的 OutboxRecordingHandler 落地该保证）。运行多实例前须先确认 Handle 幂等。
func (s *OutboxScheduler) processDLQ(ctx context.Context) {
	s.wg.Add(1)
	defer s.wg.Done()

	// step 1: 终结 max_retry → dead_lettered（CAS，只转一次）
	maxRetryEvents, err := s.repo.FindMaxRetryEvents(ctx, s.config.BatchSize, s.config.TenantID)
	if err != nil {
		if s.metrics != nil {
			s.metrics.LastError.Store(err)
		}
		return
	}
	for _, ev := range maxRetryEvents {
		if ctx.Err() != nil { // P5：优雅关闭不卡满整批 MarkAsDeadLettered
			return
		}
		if err := s.repo.MarkAsDeadLettered(ctx, ev.ID); err != nil {
			if s.metrics != nil {
				s.metrics.LastError.Store(err)
			}
			continue // 单条失败不阻断；下一轮再试
		}
	}

	if ctx.Err() != nil { // P5：进 step2 前再确认未取消
		return
	}

	// step 2: 扫描尚未通知的终态事实（与 step1 解耦：Handle 失败/kill 后下一轮继续补发）
	unnotified, err := s.repo.FindUnnotifiedDeadLettered(ctx, s.config.BatchSize, s.config.TenantID)
	if err != nil {
		if s.metrics != nil {
			s.metrics.LastError.Store(err)
		}
		return
	}
	if len(unnotified) == 0 {
		return
	}

	for _, ev := range unnotified {
		if ctx.Err() != nil { // P5：批次内逐条确认取消，避免慢 Handle 拖住关闭
			return
		}
		s.processOneDLQ(ctx, ev)
	}
}

// processOneDLQ 通知单个死信事件（C1 step2/step3）。
//
// user-supplied DLQHandler.Handle / DLQAlertHandler.Alert 在此运行；defer-recover 防止单条
// 事件 panic（典型：*OutboxEvent 字段 nil 解引用）杀死整个 dlqLoop goroutine——该 goroutine
// 无 supervisor，panic 会让死信通知永久停摆且 wg.Done 不触发（P4 修复）。panic 当作通知失败：
// notifyOK=false → 不标记通知 → 下一轮补发。返回 notifyOK 仅供测试断言。
func (s *OutboxScheduler) processOneDLQ(ctx context.Context, ev *OutboxEvent) (notifyOK bool) {
	notifyOK = true
	defer func() {
		if r := recover(); r != nil {
			notifyOK = false
			if s.metrics != nil {
				s.metrics.LastError.Store(fmt.Errorf("DLQ notify panic for event %s: %v", ev.ID, r))
			}
		}
	}()

	// 通知 = Handle + Alert；任一失败则下轮补发（C2：Handle 失败也 Alert）
	if s.config.DLQHandler != nil {
		if err := s.config.DLQHandler.Handle(ctx, ev); err != nil {
			notifyOK = false
			if s.metrics != nil {
				s.metrics.LastError.Store(err)
			}
		}
	}
	if s.config.DLQAlertHandler != nil {
		if err := s.config.DLQAlertHandler.Alert(ctx, ev); err != nil {
			notifyOK = false
			if s.metrics != nil {
				s.metrics.LastError.Store(err)
			}
		}
	}

	// step 3: 通知成功后条件标记（CAS：只标记未通知的）
	if notifyOK {
		if err := s.repo.MarkDeadLetterNotified(ctx, ev.ID); err != nil {
			if s.metrics != nil {
				s.metrics.LastError.Store(err)
			}
		}
	}
	return notifyOK
}

// SchedulerMetricsSnapshot 调度器指标快照（用于读取）
type SchedulerMetricsSnapshot struct {
	PollCount       int64
	ProcessedCount  int64
	ErrorCount      int64
	LastPollTime    time.Time
	LastCleanupTime time.Time
	RetryCount      int64
	RetriedCount    int64
	LastRetryTime   time.Time
	LastError       error
}

// GetMetrics 获取调度器指标快照
func (s *OutboxScheduler) GetMetrics() *SchedulerMetricsSnapshot {
	if s.metrics == nil {
		return nil
	}

	// 使用原子操作读取所有指标
	snapshot := &SchedulerMetricsSnapshot{
		PollCount:      s.metrics.PollCount.Load(),
		ProcessedCount: s.metrics.ProcessedCount.Load(),
		ErrorCount:     s.metrics.ErrorCount.Load(),
		RetryCount:     s.metrics.RetryCount.Load(),
		RetriedCount:   s.metrics.RetriedCount.Load(),
	}

	// 读取 atomic.Value 类型的字段
	if v := s.metrics.LastPollTime.Load(); v != nil {
		if t, ok := v.(time.Time); ok {
			snapshot.LastPollTime = t
		}
	}
	if v := s.metrics.LastCleanupTime.Load(); v != nil {
		if t, ok := v.(time.Time); ok {
			snapshot.LastCleanupTime = t
		}
	}
	if v := s.metrics.LastRetryTime.Load(); v != nil {
		if t, ok := v.(time.Time); ok {
			snapshot.LastRetryTime = t
		}
	}
	if v := s.metrics.LastError.Load(); v != nil {
		if e, ok := v.(error); ok {
			snapshot.LastError = e
		}
	}

	return snapshot
}
