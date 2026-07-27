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
