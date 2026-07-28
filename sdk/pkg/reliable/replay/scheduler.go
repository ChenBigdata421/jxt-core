package replay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"gorm.io/gorm"
)

type Scheduler struct {
	store    store.Store
	db       *gorm.DB // Mark*/AdvanceDue/MoveToDeadLetter/aggregate gate 的连接（服务侧注入 pooled db）
	registry HandlerRegistry
	metrics  reliable.ConsumptionMetrics
	alerter  reliable.Alerter
	gateTTL  time.Duration
	batch    int
}

type Option func(*Scheduler)

func WithGateTTL(t time.Duration) Option { return func(s *Scheduler) { s.gateTTL = t } }
func WithBatchSize(n int) Option          { return func(s *Scheduler) { s.batch = n } }
func WithDB(db *gorm.DB) Option           { return func(s *Scheduler) { s.db = db } }

func NewScheduler(s store.Store, db *gorm.DB, reg HandlerRegistry, m reliable.ConsumptionMetrics, a reliable.Alerter, opts ...Option) *Scheduler {
	sch := &Scheduler{store: s, db: db, registry: reg, gateTTL: 5 * time.Minute, batch: 50}
	if m == nil {
		sch.metrics = reliable.NoOpMetrics{}
	} else {
		sch.metrics = m
	}
	if a == nil {
		sch.alerter = reliable.NoOpAlerter{}
	} else {
		sch.alerter = a
	}
	for _, o := range opts {
		o(sch)
	}
	sch.printSafetyInventory() // S6：启动时打印 ReplaySafety 清单（spec §6.1 line 635）
	return sch
}

// printSafetyInventory 遍历注册表，把每个 handler 的 ReplaySafety + RequiresAggregateGate 经 alerter 暴露
// （spec §6.1 line 635「调度器启动时遍历注册表并打印安全类别清单」）。把 ReplayUnsafe 误注册成 Idempotent
// 这类错误在首次重放前即可在告警/指标里发现，而不是等到不可逆副作用发生（S6，本轮评审）。
func (s *Scheduler) printSafetyInventory() {
	if s.registry == nil {
		return
	}
	for _, h := range s.registry.All() {
		s.alerter.AlertAnomaly("REPLAY_SAFETY_INVENTORY", h.HandlerID,
			fmt.Sprintf("ReplaySafety=%v RequiresAggregateGate=%v", h.ReplaySafety, h.RequiresAggregateGate))
	}
}

func (s *Scheduler) Tick(ctx context.Context) error {
	now := time.Now().UTC()
	heads, err := s.store.FindEligibleHeads(ctx, now, s.batch)
	if err != nil {
		return err
	}
	for i := range heads {
		s.processOne(ctx, heads[i])
	}
	return nil
}

func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			_ = s.Tick(ctx)
		}
	}
}

// processOne 处理一条 eligible head。
//
// **顺序（A3 本轮评审）**：unknown-handler / not-permitted 前置校验 → **aggregate gate** →
// ClaimForReplay（attempt+1）→ 调 handler → 结算。
// 原稿把 gate 放在 ClaimForReplay **之后**：抢不到 gate 时 attempt 已经 +1（违反准入 ⑬
// 「让路不增 attempt」），而且行已进 PROCESSING、AdvanceDue 又不清 ownership，
// 于是该行永远停在 PROCESSING、FindEligibleHeads 再也取不到——只能等租约到期。
// gate 前置后，抢不到就整行不动、零副作用，下一周期自然重试。
func (s *Scheduler) processOne(ctx context.Context, row store.Row) {
	info, ok := s.registry.Lookup(row.HandlerID)
	if !ok {
		s.metrics.IncReplayBlocked(row.HandlerID)
		s.alerter.AlertAnomaly("UNKNOWN_HANDLER", row.HandlerID, fmt.Sprintf("handler %s not in registry", row.HandlerID))
		// 不动行：handler 可能只是本实例未注册（滚动发布中），别把它推进死信。
		// 若长期未注册，UNKNOWN_HANDLER 告警负责升级。
		return
	}
	if row.ReplayMode == "AUTO" && !reliable.CanAutoReplay(info.ReplaySafety) {
		s.metrics.IncReplayBlocked(row.HandlerID)
		s.alerter.AlertAnomaly("REPLAY_NOT_PERMITTED", row.HandlerID, "CanAutoReplay=false hit")
		// A2（本轮评审）：这条 head 行是 RETRY_SCHEDULED，原稿调的 MoveToDeadLetter 只匹配
		// PROCESSING → 恒 0 行 → 状态与 due 都不动 → 下一周期再取同一行，**违反准入 ⑬**。
		// MoveToDeadLetter 现已接受 RETRY_SCHEDULED（且 chk_retry_due 保证 payload/error_class
		// 非空，DEAD_LETTER 的硬条件天然满足），这里才真正「移出自动队列」。
		if err := s.store.MoveToDeadLetter(ctx, s.db, row.ID, "auto-replay not permitted for safety"); err != nil {
			s.alerter.AlertAnomaly("REPLAY_DISPOSE_FAILED", row.HandlerID, err.Error())
		}
		return
	}
	// A2：原稿这里还有一个 `len(row.Payload) == 0 → MoveToDeadLetter` 分支，已删除。
	// 它是死代码：FindEligibleHeads 只返回 RETRY_SCHEDULED 行，而 chk_retry_due 保证这些行
	// payload 必非空。payload IS NULL 的行只可能是 PROCESSING 租约孤儿，那类行按 D20 靠
	// broker 重投 + TryClaim 内联续占恢复，根本不会进这条路径。留着它只会掩盖真实假设。

	// aggregate gate（§6.2.1）——**先抢 gate，再 claim**（A3）。
	// holder 用 row.ID 派生的稳定串（此时还没有 claim token）；Store 内部会加 uuid 后缀
	// 保证 token 唯一（D18#7），所以不同实例/不同轮次不会互相误删。
	if info.RequiresAggregateGate && !aggregateKeyOf(row).Empty() {
		holder, gerr := s.store.AcquireAggregateGate(ctx, s.db, aggregateKeyOf(row),
			fmt.Sprintf("replay-%d", row.ID), s.gateTTL)
		if gerr != nil {
			// 让路：整行不动，attempt 不增，下一周期重试（准入 ⑬）。
			s.metrics.IncReplayBlocked(row.HandlerID)
			return
		}
		defer func() { _ = s.store.ReleaseAggregateGate(ctx, s.db, holder) }()
	}

	tok, claimed, err := s.store.ClaimForReplay(ctx, s.db, row.ID)
	switch invokeResult(err) {
	case InvokeRetryLater, InvokeNotPermitted, InvokeNotSelfReplayable:
		s.handleNonExecution(ctx, row, err) // D8：经 Store.AdvanceDue/MoveToDeadLetter
		return
	}
	if err != nil {
		return
	}
	_ = claimed

	// 本地调 handler（§6.3）
	herr := info.Handler.Handle(ctx, row.Payload, reliable.DeliveryMeta{
		Topic: row.Topic, Partition: derefInt32(row.SrcPartition), Offset: derefInt64(row.SrcOffset),
		BrokerTimestamp: derefTime(row.BrokerTimestamp), PayloadHash: row.RawPayloadHash,
		RawKey: row.RawKey, Headers: row.Headers,
	})
	switch invokeResult(herr) {
	case InvokeOK:
		_ = s.store.MarkSucceeded(ctx, s.db, row.Key(), tok)
	case InvokeRetryLater:
		// A3（本轮评审）：此时行已是 PROCESSING（我们持有 tok），不能再走 pre-claim 的 AdvanceDue
		// （它只匹配 RETRY_SCHEDULED，会静默 0 行、把行留在 PROCESSING 直到租约过期）。
		// ReleaseClaim 归还占位：PROCESSING → RETRY_SCHEDULED，清 ownership，推进 due，**不动 attempt**。
		if err := s.store.ReleaseClaim(ctx, s.db, claimed.ID, tok); err != nil {
			s.alerter.AlertAnomaly("REPLAY_RELEASE_FAILED", row.HandlerID, err.Error())
		}
	case InvokeNotPermitted, InvokeNotSelfReplayable:
		// 移出自动队列：行已是 PROCESSING 且我们持有 tok，走 token-fenced 的 MoveToDeadLetterWithToken（A5）。
		// payload 与 error_class 在 ClaimForReplay 后仍在，DEAD_LETTER 硬条件满足。
		if err := s.store.MoveToDeadLetterWithToken(ctx, s.db, claimed.ID, tok, herr.Error()); err != nil {
			s.alerter.AlertAnomaly("REPLAY_DISPOSE_FAILED", row.HandlerID, err.Error())
		}
	case InvokeFailed:
		// handler 自己已 MarkFailed（§4 骨架，PR-3），scheduler 不重复改状态。
	}
}

// handleNonExecution 处理 **claim 之前** 就返回的三分支（此时行仍是 RETRY_SCHEDULED）。
// 均推进 due 或移出自动队列，不增 attempt（§6.2 v2.8 架构#4；D8 经 Store）。
// claim **之后** 的让路走 ReleaseClaim（见 processOne），两条路径的源状态不同，不能混用。
func (s *Scheduler) handleNonExecution(ctx context.Context, row store.Row, err error) {
	switch {
	case errors.Is(err, reliable.ErrRetryLater):
		_ = s.store.AdvanceDue(ctx, s.db, row.ID) // 让路：推进 due，不增 attempt
	case errors.Is(err, reliable.ErrNotPermitted), errors.Is(err, reliable.ErrNotSelfReplayable):
		_ = s.store.MoveToDeadLetter(ctx, s.db, row.ID, err.Error()) // 移出自动队列 + 告警
	}
}

func aggregateKeyOf(row store.Row) reliable.AggregateGateKey {
	return reliable.AggregateGateKey{TenantID: row.TenantID, AggregateType: row.AggregateType, AggregateID: row.AggregateID}
}
func derefInt32(p *int32) int32        { if p != nil { return *p }; return 0 }
func derefInt64(p *int64) int64        { if p != nil { return *p }; return 0 }
func derefTime(p *time.Time) time.Time { if p != nil { return *p }; return time.Time{} }
