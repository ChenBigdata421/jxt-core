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
