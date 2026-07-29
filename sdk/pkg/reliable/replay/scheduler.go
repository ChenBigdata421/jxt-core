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
	store       store.Store
	db          *gorm.DB // Mark*/AdvanceDue/MoveToDeadLetter/aggregate gate 的连接（服务侧注入 pooled db）
	registry    HandlerRegistry
	metrics     reliable.ConsumptionMetrics
	alerter     reliable.Alerter
	gateTTL     time.Duration
	batch       int
	maxAttempts int           // ErrRetryLater 让路终点：attempt≥max 即 DEAD_LETTER + REPLAY_DEFER_EXHAUSTED（与 MarkFailed 对称）
	tickTimeout time.Duration // review #21：可选每 tick 超时（0=不限，沿用调用方 ctx）；兜底 FindEligibleHeads 退化 Seq Scan / 行锁长等待
}

type Option func(*Scheduler)

func WithGateTTL(t time.Duration) Option     { return func(s *Scheduler) { s.gateTTL = t } }
func WithBatchSize(n int) Option             { return func(s *Scheduler) { s.batch = n } }
func WithDB(db *gorm.DB) Option              { return func(s *Scheduler) { s.db = db } }
func WithMaxAttempts(n int) Option           { return func(s *Scheduler) { s.maxAttempts = n } }
func WithTickTimeout(t time.Duration) Option { return func(s *Scheduler) { s.tickTimeout = t } }

func NewScheduler(s store.Store, db *gorm.DB, reg HandlerRegistry, m reliable.ConsumptionMetrics, a reliable.Alerter, opts ...Option) *Scheduler {
	sch := &Scheduler{store: s, db: db, registry: reg, gateTTL: 5 * time.Minute, batch: 50, maxAttempts: reliable.DefaultMaxAttempts}
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
			// review #21：可选每 tick 超时兜底——FindEligibleHeads 退化 Seq Scan 或行锁长等待时，无超时会让整个
			// 调度器 stall。默认 0=沿用调用方 ctx（向后兼容）；设了则该 tick 到点即取消（注意：会一并取消在飞
			// handler，故应设为略大于「批量×单 handler 最长耗时」，主要兜底卡死的查询/锁等待）。
			runCtx := ctx
			var cancel context.CancelFunc
			if s.tickTimeout > 0 {
				runCtx, cancel = context.WithTimeout(ctx, s.tickTimeout)
			}
			err := s.Tick(runCtx)
			if cancel != nil {
				cancel()
			}
			// 本轮评审：Tick 返回 FindEligibleHeads 的错误（DB 断连 / schema 漂移 / partial-index 退化为
			// Seq Scan 超时）。原稿 `_ =` 吞掉 → 持续查询失败时调度器「心跳正常却零进度、零告警」，正是
			// 让队列静默 stall 的失败模式。lease.Runner 在自己的 tick 上告警，scheduler 也必须。
			if err != nil {
				s.alerter.AlertAnomaly("REPLAY_TICK_FAILURE", "", err.Error())
			}
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
	// handler 阶段包 recover（本轮评审）：单个毒 payload 的 panic 不得杀掉整个调度器 goroutine。
	// 原稿无 recover——handler.Handle 一旦 panic 就沿 processOne → Tick → Run 上抛，调度器 goroutine
	// 静默死亡、ClaimForReplay 不再运行，整个 RETRY_SCHEDULED 队列冻结且无告警，直到进程重启。
	// panic → 记 REPLAY_HANDLER_PANIC + MoveToDeadLetterWithToken（打破「broker 重投 → 再 panic」死循环）；
	// 正常返回时 recover()=nil，no-op，其余结算路径不受影响。
	defer func() {
		if r := recover(); r != nil {
			s.alerter.AlertAnomaly("REPLAY_HANDLER_PANIC", row.HandlerID, fmt.Sprintf("panic: %v", r))
			if err := s.store.MoveToDeadLetterWithToken(ctx, s.db, claimed.ID, tok, reliable.ClassPoison,
				fmt.Sprintf("handler panic: %v", r)); err != nil {
				s.alerter.AlertAnomaly("REPLAY_DISPOSE_FAILED", row.HandlerID,
					fmt.Sprintf("post-panic dead-letter: %v", err))
			}
		}
	}()

	// 本地调 handler（§6.3）。review #2：用 claimed（ClaimForReplay 竞得后 First(&m,id) 重取的整行，含 payload），
	// 而非 FindEligibleHeads 的窄投影 row（row.Payload 等大列已不再由 FindEligibleHeads 加载）。
	herr := info.Handler.Handle(ctx, claimed.Payload, reliable.DeliveryMeta{
		Topic: claimed.Topic, Partition: derefInt32(claimed.SrcPartition), Offset: derefInt64(claimed.SrcOffset),
		BrokerTimestamp: derefTime(claimed.BrokerTimestamp), PayloadHash: claimed.RawPayloadHash,
		RawKey: claimed.RawKey, Headers: claimed.Headers,
	})
	switch invokeResult(herr) {
	case InvokeOK:
		// 本轮评审：MarkSucceeded 的 ErrConflict 不得吞。它意味着我们的租约在 handler 执行期间过期、
		// 行被别 worker 的 TryClaim 内联续占（claim_id 已换主）——那个 worker 很可能【再次执行了 handler】。
		// at-least-once 语义下 kernel 无法阻止非幂等副作用的双执行，但必须让它【可见】：双应用是运维
		// 需定位的事件，不是静默成功。原稿 `_ =` 把它当成功丢弃。
		if err := s.store.MarkSucceeded(ctx, s.db, row.Key(), tok); err != nil {
			s.alerter.AlertAnomaly("REPLAY_SETTLE_FAILED", row.HandlerID,
				fmt.Sprintf("MarkSucceeded: %v (possible concurrent re-execution — verify non-idempotent side effects)", err))
		}
	case InvokeRetryLater:
		// 让路终点（本轮评审）：ClaimForReplay 每轮 attempt+1，故「claim → handler → RetryLater → Release」
		// 循环会让 attempt 增长。到达 maxAttempts 即升级 DEAD_LETTER + REPLAY_DEFER_EXHAUSTED——与
		// MarkFailed 的 ShouldDeadLetter 对称，避免「handler 永远返回 RetryLater」的行以 ~1h 间隔无限重试、
		// 永不终结。未到上限才走 ReleaseClaim（A3：PROCESSING → RETRY_SCHEDULED，清 ownership，推进 due，不动 attempt）。
		if reliable.ShouldDeadLetter(claimed.Attempt, s.maxAttempts) {
			s.alerter.AlertAnomaly("REPLAY_DEFER_EXHAUSTED", row.HandlerID,
				fmt.Sprintf("attempt %d reached maxAttempts %d via ErrRetryLater", claimed.Attempt, s.maxAttempts))
			if err := s.store.MoveToDeadLetterWithToken(ctx, s.db, claimed.ID, tok, reliable.ClassUnrecoverable,
				"defer exhausted (max attempts reached on retry-later)"); err != nil {
				s.alerter.AlertAnomaly("REPLAY_DISPOSE_FAILED", row.HandlerID, err.Error())
			}
			return
		}
		if err := s.store.ReleaseClaim(ctx, s.db, claimed.ID, tok); err != nil {
			s.alerter.AlertAnomaly("REPLAY_RELEASE_FAILED", row.HandlerID, err.Error())
		}
	case InvokeNotPermitted, InvokeNotSelfReplayable:
		// 移出自动队列：行已是 PROCESSING 且我们持有 tok，走 token-fenced 的 MoveToDeadLetterWithToken（A5）。
		// payload 与 error_class 在 ClaimForReplay 后仍在，DEAD_LETTER 硬条件满足。
		if err := s.store.MoveToDeadLetterWithToken(ctx, s.db, claimed.ID, tok, reliable.ClassPoison, herr.Error()); err != nil {
			s.alerter.AlertAnomaly("REPLAY_DISPOSE_FAILED", row.HandlerID, err.Error())
		}
	case InvokeFailed:
		// review #16：handler 返回裸（非哨兵）错误。按 PR-2 契约它应自行 MarkFailed（PR-3 装饰器统一兜底），
		// 但 scheduler 无法判定它是否真的结算——若没结算，行会卡在 PROCESSING 直到租约过期（at-least-once 下靠
		// 租约过期 + broker 重投恢复）。原稿是静默 no-op；这里发一条信息级 ops 告警（与 REPLAY_HANDLER_PANIC
		// 同属 scheduler 侧告警词汇，不是 consumption_anomalies 的闭合 enum——不触发 spec §2.3 约束），
		// 让「handler 裸错误未自结算」可被运维/agent 关联，而非静默延迟。PR-3 装饰器落地后此分支成为安全网。
		s.alerter.AlertAnomaly("REPLAY_HANDLER_BARE_ERROR", row.HandlerID,
			fmt.Sprintf("handler returned bare error (PR-2 contract: self-MarkFailed expected): %v", herr))
	}
}

// handleNonExecution 处理 **claim 之前** 就返回的三分支（此时行仍是 RETRY_SCHEDULED）。
// 均推进 due 或移出自动队列，不增 attempt（§6.2 v2.8 架构#4；D8 经 Store）。
// claim **之后** 的让路走 ReleaseClaim（见 processOne），两条路径的源状态不同，不能混用。
func (s *Scheduler) handleNonExecution(ctx context.Context, row store.Row, err error) {
	switch {
	case errors.Is(err, reliable.ErrRetryLater):
		// 让路：推进 due，不增 attempt。AdvanceDue 在行已被别实例抢占（已离开 RETRY_SCHEDULED）时 0 行
		// 返回 ErrConflict——良性，不告警；其余错误（真 DB 失败 / guard 未命中）须可见（本轮评审）：
		// 原稿一律 `_ =`，等于把 MoveToDeadLetter 精心区分的「良性/真失败」机制从调用侧废掉。
		if advErr := s.store.AdvanceDue(ctx, s.db, row.ID); advErr != nil && !errors.Is(advErr, reliable.ErrConflict) {
			s.alerter.AlertAnomaly("REPLAY_DISPOSE_FAILED", row.HandlerID, fmt.Sprintf("AdvanceDue: %v", advErr))
		}
	case errors.Is(err, reliable.ErrNotPermitted), errors.Is(err, reliable.ErrNotSelfReplayable):
		// 移出自动队列。MoveToDeadLetter 内部已按 re-SELECT 区分良性（状态已迁移→nil）与真失败（guard 未命中→ErrConflict），
		// 故任何 non-nil 都是真失败，须告警（原稿 `_ =` 吞掉）。
		if dlqErr := s.store.MoveToDeadLetter(ctx, s.db, row.ID, err.Error()); dlqErr != nil {
			s.alerter.AlertAnomaly("REPLAY_DISPOSE_FAILED", row.HandlerID, fmt.Sprintf("MoveToDeadLetter: %v", dlqErr))
		}
	}
}

func aggregateKeyOf(row store.Row) reliable.AggregateGateKey {
	return reliable.AggregateGateKey{TenantID: row.TenantID, AggregateType: row.AggregateType, AggregateID: row.AggregateID}
}
func derefInt32(p *int32) int32 {
	if p != nil {
		return *p
	}
	return 0
}
func derefInt64(p *int64) int64 {
	if p != nil {
		return *p
	}
	return 0
}
func derefTime(p *time.Time) time.Time {
	if p != nil {
		return *p
	}
	return time.Time{}
}
