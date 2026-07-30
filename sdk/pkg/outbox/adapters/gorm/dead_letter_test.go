package gorm

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

func insertStatus(t *testing.T, repo outbox.OutboxRepository, id string, status outbox.EventStatus) {
	t.Helper()
	ev := &outbox.OutboxEvent{
		ID: id, TenantID: 1, AggregateID: "agg", AggregateType: "X", EventType: "Created",
		Payload: []byte(`{}`), Status: status, RetryCount: 3, MaxRetries: 3,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.Save(context.Background(), ev); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// C1 step1：max_retry → dead_lettered CAS 只转一次；重复调用幂等、不报错。
func TestMarkAsDeadLettered_CASOnce(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	insertStatus(t, repo, "ev-dl-1", outbox.EventStatusMaxRetry)

	if err := repo.MarkAsDeadLettered(context.Background(), "ev-dl-1"); err != nil {
		t.Fatalf("first MarkAsDeadLettered: %v", err)
	}
	// 再次调用：已是 dead_lettered，CAS 命中 0 行，但不报错（幂等）
	if err := repo.MarkAsDeadLettered(context.Background(), "ev-dl-1"); err != nil {
		t.Fatalf("idempotent MarkAsDeadLettered: %v", err)
	}
}

// C1 step3：MarkDeadLetterNotified 的 CAS 幂等——重复调用命中 0 行、返回 nil。
// 与 TestMarkAsDeadLettered_CASOnce 对称，钉住 WHERE status=dead_lettered AND dlq_notified_at IS NULL
// 守卫：去掉该守卫的回归会让重复调用报错或误改已通知行。
func TestMarkDeadLetterNotified_CASOnce(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	insertStatus(t, repo, "ev-dl-4", outbox.EventStatusDeadLettered)

	if err := repo.MarkDeadLetterNotified(context.Background(), "ev-dl-4"); err != nil {
		t.Fatalf("first MarkDeadLetterNotified: %v", err)
	}
	// 再次调用：dlq_notified_at 已非 NULL，CAS 命中 0 行，但不报错（幂等）
	if err := repo.MarkDeadLetterNotified(context.Background(), "ev-dl-4"); err != nil {
		t.Fatalf("idempotent MarkDeadLetterNotified: %v", err)
	}
	got, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got) != 0 {
		t.Fatalf("after duplicate MarkDeadLetterNotified, expected 0 unnotified, got %+v", got)
	}
}

// C1 step1 谓词守卫：只有 status=max_retry 才转 dead_lettered。
// 钉住 WHERE status='max_retry'——把它改成 'pending'/'failed' 之类的回归会让非终态行被误转死信
// （静默数据丢失），而现有的 CASOnce 测试只覆盖「已是 dead_lettered」的幂等分支，抓不到这条。
func TestMarkAsDeadLettered_RejectsNonMaxRetry(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	insertStatus(t, repo, "ev-dl-5", outbox.EventStatusFailed) // 非 max_retry

	if err := repo.MarkAsDeadLettered(context.Background(), "ev-dl-5"); err != nil {
		t.Fatalf("MarkAsDeadLettered on non-max_retry must return nil (CAS hits 0 rows): %v", err)
	}
	// 行未被转终态：不出现在死信未通知扫描里
	got, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got) != 0 {
		t.Fatalf("non-max_retry row must not become dead_lettered, got %+v", got)
	}
}

// C1：dead_lettered 但未通知的行可被扫到；标记通知后不再出现。
func TestFindUnnotifiedDeadLettered_AndMarkNotified(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	// 直接造 dead_lettered 行（不经 MarkAsDeadLettered，隔离扫描逻辑）
	insertStatus(t, repo, "ev-dl-2", outbox.EventStatusDeadLettered)

	got, err := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("FindUnnotifiedDeadLettered: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ev-dl-2" {
		t.Fatalf("expected 1 unnotified ev-dl-2, got %+v", got)
	}

	if err := repo.MarkDeadLetterNotified(context.Background(), "ev-dl-2"); err != nil {
		t.Fatalf("MarkDeadLetterNotified: %v", err)
	}

	got2, err := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("second FindUnnotifiedDeadLettered: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("after MarkDeadLetterNotified, expected 0 unnotified, got %+v", got2)
	}
}

// C1：max_retry 不在未通知扫描结果里（终态与通知拆分）。
func TestFindUnnotifiedDeadLettered_ExcludesMaxRetry(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	insertStatus(t, repo, "ev-dl-3", outbox.EventStatusMaxRetry)
	got, _ := repo.FindUnnotifiedDeadLettered(context.Background(), 10, 0)
	if len(got) != 0 {
		t.Fatalf("max_retry must not appear in unnotified scan, got %+v", got)
	}
}
