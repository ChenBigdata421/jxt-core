package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// stubRepo 是 processDLQ 测试用的内存仓储：内嵌 OutboxRepository 接口（未覆盖方法在被调时
// panic——processDLQ 不会调它们），只覆盖 processDLQ 真正触碰的方法。避免 import adapters/gorm
// （会与 outbox 循环）。Task 6 阶段 processDLQ 只调 FindMaxRetryEvents；Task 8 重写后还调
// MarkAsDeadLettered/FindUnnotifiedDeadLettered/MarkDeadLetterNotified——此处一并预定义。
type stubRepo struct {
	OutboxRepository // 嵌入接口：未覆盖方法 panic；processDLQ 只调下面 4 个

	mu       sync.Mutex
	maxRetry map[string]*OutboxEvent
	dead     map[string]*stubDeadRow
}

type stubDeadRow struct {
	ev       *OutboxEvent
	notified bool
}

func newStubRepo(events ...*OutboxEvent) *stubRepo {
	m := make(map[string]*OutboxEvent, len(events))
	for _, e := range events {
		m[e.ID] = e
	}
	return &stubRepo{maxRetry: m, dead: map[string]*stubDeadRow{}}
}

func (s *stubRepo) FindMaxRetryEvents(ctx context.Context, limit int, tenantID int) ([]*OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*OutboxEvent, 0, len(s.maxRetry))
	for _, e := range s.maxRetry {
		out = append(out, e)
	}
	return out, nil
}

func (s *stubRepo) MarkAsDeadLettered(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.maxRetry[id]; ok {
		delete(s.maxRetry, id)
		s.dead[id] = &stubDeadRow{ev: e}
	}
	return nil
}

func (s *stubRepo) FindUnnotifiedDeadLettered(ctx context.Context, limit int, tenantID int) ([]*OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []*OutboxEvent{}
	for _, d := range s.dead {
		if !d.notified {
			out = append(out, d.ev)
		}
	}
	return out, nil
}

func (s *stubRepo) MarkDeadLetterNotified(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.dead[id]; ok {
		d.notified = true
	}
	return nil
}

// newTestScheduler 直接结构体构造一个仅用于 processDLQ 测试的 scheduler——绕过 NewScheduler
// 对 repo/eventPublisher 的 panic；processDLQ 不发布，不需要 publisher。内部包可访问未导出字段。
func newTestScheduler(repo OutboxRepository, cfg *SchedulerConfig) *OutboxScheduler {
	return &OutboxScheduler{repo: repo, config: cfg, wg: sync.WaitGroup{}}
}

// C2 回归：DLQHandler.Handle 返回 error 时，DLQAlertHandler.Alert 仍必须被调用。
// 失效形态是「告警不响」——不会报错，必须有测试兜底。
func TestProcessDLQ_AlertsEvenWhenHandleFails(t *testing.T) {
	repo := newStubRepo(&OutboxEvent{ID: "ev-c2-1", TenantID: 1, Status: EventStatusMaxRetry})

	var alertMu sync.Mutex
	alertCalled := false
	cfg := &SchedulerConfig{
		EnableDLQ: true, DLQInterval: time.Minute, BatchSize: 10,
		DLQHandler: DLQHandlerFunc(func(ctx context.Context, e *OutboxEvent) error {
			return errors.New("handle always fails")
		}),
		DLQAlertHandler: DLQAlertHandlerFunc(func(ctx context.Context, e *OutboxEvent) error {
			alertMu.Lock()
			alertCalled = true
			alertMu.Unlock()
			return nil
		}),
	}

	s := newTestScheduler(repo, cfg)
	s.processDLQ(context.Background())

	if !alertCalled {
		t.Fatal("DLQAlertHandler.Alert must be called even when DLQHandler.Handle fails (C2)")
	}
}

// C1 回归（单实例）：max_retry 事件在【同一 scheduler 实例】的多轮 processDLQ 下不被 Handle 两次。
// ⚠️ 不覆盖多实例并发（OV#6）：两实例同时 step2 会 double Handle，本测试用单实例顺序两轮，
// 无法捕获多实例竞态——那条由 PR-2 durable claim + Handle 幂等保证。
func TestProcessDLQ_SingleInstanceHandlesOnceAcrossLoops(t *testing.T) {
	repo := newStubRepo(&OutboxEvent{ID: "ev-c1-1", TenantID: 1, Status: EventStatusMaxRetry})

	var handleMu sync.Mutex
	handleCalls := 0
	cfg := &SchedulerConfig{
		EnableDLQ: true, DLQInterval: time.Minute, BatchSize: 10,
		DLQHandler: DLQHandlerFunc(func(ctx context.Context, e *OutboxEvent) error {
			handleMu.Lock()
			handleCalls++
			handleMu.Unlock()
			return nil
		}),
		DLQAlertHandler: DLQAlertHandlerFunc(func(ctx context.Context, e *OutboxEvent) error { return nil }),
	}
	s := newTestScheduler(repo, cfg)

	s.processDLQ(context.Background())
	s.processDLQ(context.Background()) // 第二轮不应再 Handle

	handleMu.Lock()
	defer handleMu.Unlock()
	if handleCalls != 1 {
		t.Fatalf("Handle must be called exactly once across two loops, got %d", handleCalls)
	}

	// 行已转终态且已通知：两路扫描都取不到
	got, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got) != 0 {
		t.Fatalf("after two loops, expected 0 unnotified, got %+v", got)
	}
}

// C1 通知补发：Handle 失败 → 不标记通知 → 下一轮重新 Handle（幂等）。
func TestProcessDLQ_NotificationRetriedUntilHandleSucceeds(t *testing.T) {
	repo := newStubRepo(&OutboxEvent{ID: "ev-c1-2", TenantID: 1, Status: EventStatusMaxRetry})

	var handleMu sync.Mutex
	calls := 0
	cfg := &SchedulerConfig{
		EnableDLQ: true, DLQInterval: time.Minute, BatchSize: 10,
		DLQHandler: DLQHandlerFunc(func(ctx context.Context, e *OutboxEvent) error {
			handleMu.Lock()
			calls++
			handleMu.Unlock()
			if calls == 1 {
				return errors.New("transient handle failure") // 首次失败
			}
			return nil // 第二次成功
		}),
		DLQAlertHandler: DLQAlertHandlerFunc(func(ctx context.Context, e *OutboxEvent) error { return nil }),
	}
	s := newTestScheduler(repo, cfg)

	s.processDLQ(context.Background()) // 终态转 dead_lettered；Handle 失败 → 不标记通知
	s.processDLQ(context.Background()) // 重新取到未通知行；Handle 成功 → 标记通知

	handleMu.Lock()
	defer handleMu.Unlock()
	if calls != 2 {
		t.Fatalf("Handle must be retried after failure, expected 2 calls, got %d", calls)
	}
	got, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got) != 0 {
		t.Fatalf("after retry success, expected 0 unnotified, got %+v", got)
	}
}

// C1/C2 回归：Alert 失败 → notifyOK=false → 不标记通知 → 下一轮补发。
// 这是 notifyOK 矩阵的第 3 条分支（Handle-fails、Handle-retry 之外）——钉住 scheduler.go 里
// Alert 错误把 notifyOK 置 false 的那一行，防止回归静默标记已通知。
func TestProcessDLQ_AlertFailureKeepsUnnotifiedAndRetries(t *testing.T) {
	repo := newStubRepo(&OutboxEvent{ID: "ev-alert", TenantID: 1, Status: EventStatusMaxRetry})

	var alertMu sync.Mutex
	alerts := 0
	cfg := &SchedulerConfig{
		EnableDLQ: true, DLQInterval: time.Minute, BatchSize: 10,
		DLQHandler: DLQHandlerFunc(func(ctx context.Context, e *OutboxEvent) error { return nil }),
		DLQAlertHandler: DLQAlertHandlerFunc(func(ctx context.Context, e *OutboxEvent) error {
			alertMu.Lock()
			alerts++
			alertMu.Unlock()
			if alerts == 1 {
				return errors.New("transient alert failure")
			}
			return nil
		}),
	}
	s := newTestScheduler(repo, cfg)

	s.processDLQ(context.Background()) // 终态转 dead_lettered；Alert 失败 → 不标记通知
	got, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got) != 1 {
		t.Fatalf("alert failure must keep row unnotified, got %d unnotified", len(got))
	}

	s.processDLQ(context.Background()) // Alert 成功 → 标记通知
	got2, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got2) != 0 {
		t.Fatalf("after alert success, expected 0 unnotified, got %d", len(got2))
	}
}

// P4 回归：DLQHandler.Handle panic 时，processOneDLQ 的 defer-recover 兜住——
// 不抛杀 dlqLoop goroutine，行保持未通知、下一轮补发。
func TestProcessDLQ_HandlePanicDoesNotKillLoop(t *testing.T) {
	repo := newStubRepo(&OutboxEvent{ID: "ev-panic", TenantID: 1, Status: EventStatusMaxRetry})

	cfg := &SchedulerConfig{
		EnableDLQ: true, DLQInterval: time.Minute, BatchSize: 10,
		DLQHandler: DLQHandlerFunc(func(ctx context.Context, e *OutboxEvent) error {
			panic("simulated handler panic")
		}),
		DLQAlertHandler: DLQAlertHandlerFunc(func(ctx context.Context, e *OutboxEvent) error { return nil }),
	}
	s := newTestScheduler(repo, cfg)

	// 第一轮：Handle panic 被 Recover 兜住 → 不标记通知。processDLQ 必须正常返回（不向 dlqLoop 抛）。
	s.processDLQ(context.Background())
	got, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got) != 1 {
		t.Fatalf("Handle panic must keep row unnotified for retry, got %d unnotified", len(got))
	}

	// 第二轮：换一个不 panic 的 handler，应正常标记通知（证明 goroutine/循环没死）。
	s.config.DLQHandler = DLQHandlerFunc(func(ctx context.Context, e *OutboxEvent) error { return nil })
	s.processDLQ(context.Background())
	got2, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got2) != 0 {
		t.Fatalf("after non-panic retry, expected 0 unnotified, got %d", len(got2))
	}
}

// P5 回归：ctx 已取消时，processDLQ 在 step1 批次内逐条 ctx.Err() 检查命中、提前返回，
// 不会把整批 max_retry 跑完。钉住 scheduler.go processDLQ step1 循环里的 ctx.Err() 守卫——
// 去掉它的回归会让优雅关闭在慢仓储/大批量下拖到 ShutdownTimeout。
func TestProcessDLQ_RespectsCtxCancellation(t *testing.T) {
	repo := newStubRepo(
		&OutboxEvent{ID: "ev-ctx-1", TenantID: 1, Status: EventStatusMaxRetry},
		&OutboxEvent{ID: "ev-ctx-2", TenantID: 1, Status: EventStatusMaxRetry},
	)
	cfg := &SchedulerConfig{
		EnableDLQ: true, DLQInterval: time.Minute, BatchSize: 10,
		DLQHandler:      DLQHandlerFunc(func(ctx context.Context, e *OutboxEvent) error { return nil }),
		DLQAlertHandler: DLQAlertHandlerFunc(func(ctx context.Context, e *OutboxEvent) error { return nil }),
	}
	s := newTestScheduler(repo, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 预先取消
	s.processDLQ(ctx)

	// ctx 取消 → step1 首个 ctx.Err() 命中、立即返回 → 没有行被转终态
	got, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got) != 0 {
		t.Fatalf("cancelled ctx must abort before terminalizing, got %d dead_lettered", len(got))
	}
	// max_retry 行原样还在
	mr, _ := repo.FindMaxRetryEvents(context.Background(), 10, 0)
	if len(mr) != 2 {
		t.Fatalf("max_retry rows must remain after cancelled processDLQ, got %d", len(mr))
	}
}
