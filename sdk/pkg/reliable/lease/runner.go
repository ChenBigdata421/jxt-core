package lease

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
)

type Runner struct {
	store    store.Store
	metrics  reliable.ConsumptionMetrics
	alerter  reliable.Alerter
	interval time.Duration
	now      func() time.Time
}

func NewRunner(s store.Store, m reliable.ConsumptionMetrics, a reliable.Alerter, interval time.Duration, now func() time.Time) *Runner {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if m == nil {
		m = reliable.NoOpMetrics{}
	}
	if a == nil {
		a = reliable.NoOpAlerter{}
	}
	return &Runner{store: s, metrics: m, alerter: a, interval: interval, now: now}
}

func (r *Runner) Run(ctx context.Context) error {
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			r.tick(ctx)
		}
	}
}

// Tick 执行一轮观测，返回扫到的孤儿行数。服务侧用自己的 scheduler 可直接调。
//
// **D20：不循环**。原稿循环至返回 0（因为回收会清掉行，下一批不同）；现在观测器不改行，
// 同一批孤儿行会被每次 SELECT 反复返回 → 循环永远拿不到 0，会死死转满 maxIter 次。
// 每 tick 只扫一批（LIMIT 500）就够了：观测是为了告警，不是为了清空队列。
func (r *Runner) Tick(ctx context.Context) (observed int, err error) {
	return r.store.ObserveExpiredLeases(ctx, r.now())
}

func (r *Runner) tick(ctx context.Context) {
	n, err := r.Tick(ctx)
	if err != nil {
		// 名字也改了：失败的是「观测」而非「回收」，告警文案不应暗示行被改过。
		r.alerter.AlertAnomaly("LEASE_OBSERVE_FAILURE", "", err.Error())
		return
	}
	// 指标不在这里出：ConsumptionMetrics.IncAnomaly(kind, handlerID) 需要 handlerID，
	// 而 runner 只拿到行数。按 handler 维度计数的职责归 Store（它逐行写 anomaly 时知道 handlerID），
	// 由 PR-3 的 metrics 装饰器在 RecordAnomaly 上挂。runner 这里只保证不 panic、不吞错。
	_ = n
}
