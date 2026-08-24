package gorm

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	"github.com/stretchr/testify/require"
)

// PR-7 C②（§10）：ops 死信列表 FindDeadLettered/CountDeadLettered。
// 与 C1 的 FindUnnotifiedDeadLettered（通知补发扫描，dlq_notified_at IS NULL）分野：
// ops 视图展示全部 dead_lettered 行——已 dlq_notified 的死信仍要出现在运维列表里。

// insertDeadLettered 直接以 dead_lettered 终态 + 显式 dead_lettered_at 落一行。
// 绕过 MarkAsDeadLettered 的 now()：排序断言（dead_lettered_at DESC）需要可控的时间戳。
func insertDeadLettered(t *testing.T, repo outbox.OutboxRepository, id string, tenantID int, at time.Time) {
	t.Helper()
	ev := &outbox.OutboxEvent{
		ID: id, TenantID: tenantID, AggregateID: "agg-" + id, AggregateType: "X", EventType: "Created",
		Payload: []byte(`{}`), Status: outbox.EventStatusDeadLettered, RetryCount: 3, MaxRetries: 3,
		// idempotency_key 上有 uniqueIndex：多条死信共存于同一测试库时必须每行唯一
		// （现有 insertStatus 每库只插一行，空串不会撞；本文件会插多行）。
		IdempotencyKey: id,
		DeadLetteredAt: &at,
		CreatedAt:      at, UpdatedAt: at,
	}
	require.NoError(t, repo.Save(context.Background(), ev))
}

func idsOf(events []*outbox.OutboxEvent) []string {
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	return ids
}

// C② 核心契约：max_retry 行经 MarkAsDeadLettered 转终态后必须可列出；
// pending/published/failed/max_retry 噪声行不得出现；
// 已 dlq_notified 的死信行仍出现——这是 ops 视图与 C1 通知扫描的本质区别。
func TestFindDeadLettered_OpsViewIncludesNotified(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	ctx := context.Background()

	// 噪声：非 dead_lettered 状态的行一律不得进入 ops 死信列表
	insertStatus(t, repo, "ops-pending", outbox.EventStatusPending)
	insertStatus(t, repo, "ops-published", outbox.EventStatusPublished)
	insertStatus(t, repo, "ops-failed", outbox.EventStatusFailed)
	insertStatus(t, repo, "ops-maxretry", outbox.EventStatusMaxRetry)

	// 主角①：完整链路 max_retry → MarkAsDeadLettered（CAS 转终态）
	insertStatus(t, repo, "ops-dl-via-cas", outbox.EventStatusMaxRetry)
	require.NoError(t, repo.MarkAsDeadLettered(ctx, "ops-dl-via-cas"))

	// 主角②：已 dlq_notified 的死信（ops 视图必须仍展示）
	insertDeadLettered(t, repo, "ops-dl-notified", 1, time.Now().UTC().Add(-time.Minute))
	require.NoError(t, repo.MarkDeadLetterNotified(ctx, "ops-dl-notified"))

	got, err := repo.FindDeadLettered(ctx, 10, 0, 0)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ops-dl-via-cas", "ops-dl-notified"}, idsOf(got),
		"ops listing must contain exactly the dead_lettered rows, notified or not")

	count, err := repo.CountDeadLettered(ctx, 0)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
}

// C② 租户过滤：tenantID>0 只看该租户；tenantID<=0（含 0 与负数）为 ops 全租户视图。
func TestFindDeadLettered_TenantScoping(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()
	insertDeadLettered(t, repo, "t1-old", 1, now.Add(-2*time.Minute))
	insertDeadLettered(t, repo, "t1-new", 1, now.Add(-time.Minute))
	insertDeadLettered(t, repo, "t2-only", 2, now)

	// tenantID=1：只看租户 1，最新在前
	got, err := repo.FindDeadLettered(ctx, 10, 0, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"t1-new", "t1-old"}, idsOf(got))

	// tenantID=0：全租户（ops 视图）
	gotAll, err := repo.FindDeadLettered(ctx, 10, 0, 0)
	require.NoError(t, err)
	require.Len(t, gotAll, 3)

	// tenantID<0：同样视为全租户
	gotNeg, err := repo.FindDeadLettered(ctx, 10, 0, -1)
	require.NoError(t, err)
	require.Len(t, gotNeg, 3)

	countT1, err := repo.CountDeadLettered(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, countT1)

	countAll, err := repo.CountDeadLettered(ctx, 0)
	require.NoError(t, err)
	require.EqualValues(t, 3, countAll)
}

// C② 排序契约：dead_lettered_at DESC 为首键，id DESC 为并列时间戳的确定性次键。
// 钉住 ORDER BY——丢掉 id 次键会让同一秒内死信的行顺序不稳定（分页翻页错乱）。
func TestFindDeadLettered_OrdersByDeadLetteredAtDescThenIDDesc(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	ctx := context.Background()

	t0 := time.Now().UTC().Add(-3 * time.Minute)
	t1 := t0.Add(time.Minute) // a/b 共用：钉 id DESC 次键
	t2 := t1.Add(time.Minute)

	insertDeadLettered(t, repo, "zz-old", 1, t0)
	insertDeadLettered(t, repo, "a", 1, t1)
	insertDeadLettered(t, repo, "b", 1, t1)
	insertDeadLettered(t, repo, "top", 1, t2)

	got, err := repo.FindDeadLettered(ctx, 10, 0, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"top", "b", "a", "zz-old"}, idsOf(got),
		"expected dead_lettered_at DESC with id DESC tiebreak")
}

// C② 分页契约：LIMIT/OFFSET 与 CountDeadLettered 配合支撑 REST 分页。
func TestFindDeadLettered_LimitOffsetPaging(t *testing.T) {
	repo := NewGormOutboxRepository(setupTestDB(t))
	ctx := context.Background()

	t0 := time.Now().UTC()
	insertDeadLettered(t, repo, "oldest", 1, t0)
	insertDeadLettered(t, repo, "mid", 1, t0.Add(time.Minute))
	insertDeadLettered(t, repo, "newest", 1, t0.Add(2*time.Minute))

	total, err := repo.CountDeadLettered(ctx, 0)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)

	page1, err := repo.FindDeadLettered(ctx, 2, 0, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"newest", "mid"}, idsOf(page1))

	page2, err := repo.FindDeadLettered(ctx, 2, 2, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"oldest"}, idsOf(page2))
}
