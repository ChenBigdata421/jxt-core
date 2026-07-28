package gormshared

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormStore 实现 store.Store（共享，跨方言）。D17。
type GormStore struct {
	claimDB    *gorm.DB                 // TryClaim 独立提交用（D16：NewStore 派生 NewDB session）
	markDB     *gorm.DB                 // Mark*/Schedule/Discard 等用调用方传入的 db/tx
	classifier reliable.ErrorClassifier // D3：dup 检测 + 第 2 级分类
}

// NewStore 构造共享 GormStore。db 必须是 pooled（非事务）句柄（§3.3，D16）：
// claimDB = db.Session(&gorm.Session{NewDB:true}) 隔离 WithContext 条件；ConnPool 仍是底层池，
// 故只要 db 是 pooled，TryClaim 的 Create 即独立提交。
//
// **构造期 guard（本轮评审 F1/A3）**：已在 gorm v1.24.2 核实 Session{NewDB:true} 只置 clone 标志、
// 不改 ConnPool——若 db 是事务句柄，claimDB 继承 tx ConnPool、TryClaim 静默并入调用方事务，§3.3 三大失效
// 全部复活。故 NewStore 用 ConnPool 类型断言在构造期拒绝 tx 句柄（panic），把「构造期保证」从措辞变成可执行断言。
// 该断言用正向接口（txCommitter），不依赖具体 ConnPool 类型——**R2 修正**：round-1 的 `*sql.Tx` 类型断言在
// `gorm.Config{PrepareStmt:true}` 下漏掉 gorm 的 `*PreparedStmtTX` 包装 → guard 失效、tx 句柄静默过关。
// 正向断言覆盖 `*sql.Tx` / `*PreparedStmtTX` 及任何实现 Commit/Rollback 的 tx 包装；D23 锁版 gorm，升版须重验。
type txCommitter interface {
	Commit() error
	Rollback() error
}

func NewStore(db *gorm.DB, classifier reliable.ErrorClassifier) *GormStore {
	if db == nil {
		panic("reliable: NewStore requires non-nil pooled *gorm.DB")
	}
	if classifier == nil {
		panic("reliable: NewStore requires non-nil classifier")
	}
	if db.Statement != nil {
		// pooled ConnPool（*sql.DB / *sql.Conn / *PreparedStmtDB）不实现 Commit/Rollback；
		// tx ConnPool（*sql.Tx / *PreparedStmtTX）实现 → panic。覆盖 PrepareStmt 包装（R2）。
		if _, ok := db.Statement.ConnPool.(txCommitter); ok {
			panic("reliable: NewStore requires a pooled (non-transaction) *gorm.DB — ConnPool implements Commit/Rollback (tx handle, incl. PrepareStmt-wrapped); TryClaim must independent-commit (§3.3, F1/A3, R2)")
		}
	}
	return &GormStore{claimDB: db.Session(&gorm.Session{NewDB: true}), markDB: db, classifier: classifier}
}

// 编译期保证 GormStore 实现 store.Store。
var _ store.Store = (*GormStore)(nil)

// jitterFraction 产出 [0,1) 的确定性 jitter。
// D4（本轮评审）：种子必须掺入 attempt——原稿只用 rowID，同一行第 1/2/3 次重试的 jitter 完全相同，
// jitter 只在「行之间」去相关、「同一行的历次退避」没有去相关；若上游是共享依赖抖动，
// 所有行会按各自固定偏移形成稳定的周期性冲击（惊群）。
func jitterFraction(rowID int64, attempt int) float64 {
	if rowID <= 0 {
		return 0.5
	}
	seed := rowID*31 + int64(attempt)*2654435761
	if seed < 0 {
		seed = -seed
	}
	return float64(seed%100) / 100.0
}

// —— TryClaim（§3.1/§3.3：独立提交；D3 typed dup；D4 内联回收记 LEASE_ORPHAN）——

// TryClaim 对外入口：有界重试包装。
// A7（本轮评审）：原稿在 dup-key 后直接 `return s.TryClaim(...)` 递归调用自身且无深度上限。
// 病态并发（如另一路反复 insert-then-delete）下可栈溢出。改为有界循环（≤ 3 次），
// 超限返回 ErrConflict 交由上层上抛（不静默 ACK）。
func (s *GormStore) TryClaim(ctx context.Context, in reliable.ClaimInput, lease time.Duration) (reliable.ClaimToken, reliable.Decision, error) {
	const maxDupRetry = 3
	for i := 0; i < maxDupRetry; i++ {
		tok, dec, err, retry := s.tryClaimOnce(ctx, in, lease)
		if !retry {
			return tok, dec, err
		}
	}
	return "", 0, fmt.Errorf("reliable: TryClaim exhausted %d dup-key retries for %s: %w", maxDupRetry, in.Key.EventID, reliable.ErrConflict)
}

// tryClaimOnce 执行一轮占位。第四个返回值 retry=true 表示碰到 dup-key 竞态，应重读重试。
func (s *GormStore) tryClaimOnce(ctx context.Context, in reliable.ClaimInput, lease time.Duration) (reliable.ClaimToken, reliable.Decision, error, bool) {
	if err := in.Key.Validate(); err != nil {
		return "", 0, err, false
	}
	claimID := uuid.NewString()
	now := nowUTC()
	expires := now.Add(lease)

	var existing EventConsumptionModel
	err := s.claimDB.WithContext(ctx).
		Where("event_id = ? AND handler_id = ? AND item_key = ?", in.Key.EventID, string(in.Key.Handler), in.Key.ItemKey).
		First(&existing).Error
	if err == nil {
		switch reliable.Status(existing.Status) {
		case reliable.StatusProcessing:
			if existing.LeaseExpiresAt != nil && existing.LeaseExpiresAt.After(now) {
				return "", reliable.AlreadyProcessing, nil, false // 租约有效，让路
			}
			// 租约过期：内联 CAS 续占（D4：同时记 LEASE_ORPHAN）。
			rows := s.claimDB.WithContext(ctx).Model(&EventConsumptionModel{}).
				Where("id = ? AND status = ? AND lease_expires_at < ?", existing.ID, reliable.StatusProcessing, now).
				Updates(map[string]any{
					"status": reliable.StatusProcessing, "claim_id": claimID,
					"claimed_at": now, "lease_expires_at": expires, "last_attempt_at": now,
					"row_version": gorm.Expr("row_version + 1"),
				}).RowsAffected
			if rows == 1 {
				// D4：内联回收也记 LEASE_ORPHAN，与 lease runner 观测一致。
				// D20（本轮）：这条内联 CAS 是【唯一】的重新占位路径——lease.Runner 只观测不改行。
				// claimID 传【被顶替掉的旧 claim_id】：幂等键按「哪一次占位成了孤儿」去重，
				// 与 ObserveExpiredLeases 记录同一孤儿时用的键一致，两条路径不会重复计数。
				_ = s.RecordAnomaly(ctx, s.claimDB, existing.TenantID, "LEASE_ORPHAN",
					reliable.Key{EventID: existing.EventID, Handler: reliable.HandlerID(existing.HandlerID), ItemKey: existing.ItemKey},
					existing.ClaimID, "lease expired; reclaimed inline by TryClaim")
				return reliable.ClaimToken(claimID), reliable.Claimed, nil, false
			}
			return "", reliable.AlreadyProcessing, nil, false
		default:
			// 终态或 RETRY_SCHEDULED：视为已结算（后者由 scheduler 经 ClaimForReplay 处理）。
			return "", reliable.AlreadySettled, nil, false
		}
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", 0, err, false
	}

	// 行不存在：INSERT 首次占位（claimDB 独立提交）。
	m := &EventConsumptionModel{
		EventID: in.Key.EventID, ItemKey: in.Key.ItemKey, HandlerID: string(in.Key.Handler),
		TenantID: in.TenantID, EventType: in.Meta.EventType, AggregateType: in.Meta.AggregateType,
		AggregateID: in.Meta.AggregateID, CausalSeq: in.Meta.CausalSeq, Topic: in.Delivery.Topic,
		Status: string(reliable.StatusProcessing), Attempt: 1,
		ClaimID: claimID, ClaimedAt: &now, LeaseExpiresAt: &expires, LastAttemptAt: &now,
		SrcPartition:    ptrInt32(in.Delivery.Partition),
		SrcOffset:       ptrInt64(in.Delivery.Offset),
		RawPayloadHash:  in.Delivery.PayloadHash,
		BrokerTimestamp: ptrTime(in.Delivery.BrokerTimestamp),
		RawKey:          append([]byte(nil), in.Delivery.RawKey...),
		Headers:         marshalHeaders(in.Delivery.Headers),
		FirstSeenAt:     now, CreatedAt: now, UpdatedAt: now, ReplayMode: "AUTO",
	}
	if err := s.claimDB.WithContext(ctx).Create(m).Error; err != nil {
		// D3：dup 检测用 typed classifier.IsDuplicateKey，不字符串匹配。
		if s.classifier.IsDuplicateKey(err) {
			return "", 0, nil, true // A7：交由外层有界循环重读，不递归
		}
		return "", 0, err, false
	}
	return reliable.ClaimToken(claimID), reliable.Claimed, nil, false
}

// —— MarkSucceeded（§3.1：WHERE claim_id=tok；0 行→ErrConflict）——

func (s *GormStore) MarkSucceeded(ctx context.Context, db *gorm.DB, key reliable.Key, tok reliable.ClaimToken) error {
	now := nowUTC()
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("event_id = ? AND handler_id = ? AND item_key = ? AND status = ? AND claim_id = ?",
			key.EventID, string(key.Handler), key.ItemKey, reliable.StatusProcessing, string(tok)).
		Updates(map[string]any{
			"status": reliable.StatusSucceeded, "claim_id": nil, "claimed_at": nil,
			"lease_expires_at": nil, "last_attempt_at": nil,
			"error_class": nil, "error_message": "", "next_attempt_at": nil, "payload": nil,
			// C3（本轮评审）：也必须清 error_code / error_fingerprint。§2.4 不变量表没列这两列，
			// 但「成功了却留着上次失败的指纹」会污染 §10 按 error_fingerprint 的聚合定位。
			"error_code": "", "error_fingerprint": "",
			"updated_at": now, "row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// —— MarkFailed（§6.1 矩阵 + attempt 耗尽）——

func (s *GormStore) MarkFailed(ctx context.Context, db *gorm.DB, key reliable.Key, tok reliable.ClaimToken,
	class reliable.ErrorClass, safety reliable.ReplaySafety, maxAttempts int, cause error, payload []byte) error {

	if payload == nil {
		return fmt.Errorf("reliable: MarkFailed requires non-nil payload")
	}

	var m EventConsumptionModel
	if err := db.WithContext(ctx).
		Where("event_id = ? AND handler_id = ? AND item_key = ? AND status = ? AND claim_id = ?",
			key.EventID, string(key.Handler), key.ItemKey, reliable.StatusProcessing, string(tok)).
		First(&m).Error; err != nil {
		return reliable.ErrConflict
	}

	now := nowUTC()
	deadLetter := reliable.OutcomeFor(class, safety).DeadLetter
	if reliable.ShouldDeadLetter(m.Attempt, maxAttempts) {
		deadLetter = true // attempt 耗尽，即使 Retryable 也 DEAD_LETTER
	}
	updates := map[string]any{
		"claim_id": nil, "claimed_at": nil, "lease_expires_at": nil,
		"error_class": class, "error_code": stableErrorCode(cause, s.classifier), "error_message": sanitizeMsg(cause.Error()),
		"error_fingerprint": fingerprint(class, cause.Error()), // D10：sha256 64 hex
		"payload":           payload, "updated_at": now, "last_attempt_at": now,
		"row_version": gorm.Expr("row_version + 1"),
	}
	if deadLetter {
		updates["status"] = reliable.StatusDeadLetter
		updates["next_attempt_at"] = nil
	} else {
		updates["status"] = reliable.StatusRetryScheduled
		updates["next_attempt_at"] = now.Add(reliable.Backoff(m.Attempt, reliable.DefaultBackoffBase, reliable.DefaultBackoffCap, jitterFraction(m.ID, m.Attempt)))
	}
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND claim_id = ?", m.ID, reliable.StatusProcessing, string(tok)).
		Updates(updates).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// —— RecordTerminal（无 token；payload 必须非空，§4 v2.8）——

func (s *GormStore) RecordTerminal(ctx context.Context, db *gorm.DB, in reliable.ClaimInput,
	class reliable.ErrorClass, cause error, payload []byte) error {
	if payload == nil {
		return fmt.Errorf("reliable: RecordTerminal requires non-nil payload")
	}
	now := nowUTC()
	var existing EventConsumptionModel
	err := db.WithContext(ctx).
		Where("event_id = ? AND handler_id = ? AND item_key = ?", in.Key.EventID, string(in.Key.Handler), in.Key.ItemKey).
		First(&existing).Error
	if err == nil {
		if reliable.IsTerminal(reliable.Status(existing.Status)) && existing.ErrorClass == class {
			return nil // 幂等
		}
		return reliable.ErrConflict
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	m := &EventConsumptionModel{
		EventID: in.Key.EventID, ItemKey: in.Key.ItemKey, HandlerID: string(in.Key.Handler),
		TenantID: in.TenantID, EventType: in.Meta.EventType, AggregateType: in.Meta.AggregateType,
		AggregateID: in.Meta.AggregateID, CausalSeq: in.Meta.CausalSeq, Topic: in.Delivery.Topic,
		Status: string(reliable.StatusDeadLetter), Attempt: 1,
		ErrorClass: class, ErrorCode: stableErrorCode(cause, s.classifier), ErrorMessage: sanitizeMsg(cause.Error()), ErrorFingerprint: fingerprint(class, cause.Error()),
		Payload: payload, RawPayloadHash: in.Delivery.PayloadHash, BrokerTimestamp: ptrTime(in.Delivery.BrokerTimestamp),
		RawKey: append([]byte(nil), in.Delivery.RawKey...), Headers: marshalHeaders(in.Delivery.Headers),
		SrcPartition: ptrInt32(in.Delivery.Partition), SrcOffset: ptrInt64(in.Delivery.Offset),
		FirstSeenAt: now, CreatedAt: now, UpdatedAt: now, ReplayMode: "AUTO",
	}
	return db.WithContext(ctx).Create(m).Error
}

// —— ObserveExpiredLeases（§3.2；D14 批量；**D20 本轮评审：只观测，不改行状态**）——
//
// D20 决策背景：原稿在此批量清 claim_id/claimed_at/lease_expires_at 却不改 status，
// 直接撞死自己写的 chk_processing_owner（`status <> 'PROCESSING' OR (claim_id IS NOT NULL AND ...)`），
// 两方言上 100% 返回约束错误 → lease.Runner 每 tick 告警、崩溃残留事件永久卡死。
//
// 根因是状态模型缺一个「无主、可再占位、payload 可空」的空闲态：租约孤儿行 payload IS NULL
// （§2.1：payload 只在失败时写），于是 RETRY_SCHEDULED 被 chk_retry_due 挡、DEAD_LETTER 被
// chk_dead_payload 挡、PROCESSING 被 chk_processing_owner 挡——五状态里无一可落。
//
// D20 解法：**回收器不改行状态**。行保持 PROCESSING 但 lease_expires_at 已过期，
// 「过期即可再占位」由 TryClaim 的内联 CAS 续占独家负责（见上方 tryClaimOnce）——那条路径
// 会写入新 claim_id，chk_processing_owner 全程成立，且旧 token 的 Mark* 因 claim_id 不匹配
// 得到 ErrConflict，fencing token 语义完整。本方法只负责可观测性：批量写 LEASE_ORPHAN + 计数。
// 语义与 spec §3.2「payload IS NULL 靠 broker 重投」完全一致：broker 本就会再投，TryClaim 接住。
//
// 命名：接口方法名从 ReclaimExpiredLeases 改为 ObserveExpiredLeases，避免「回收」的误导性暗示
// （T6 store.Store 接口、T10 lease.Runner 同步改名）。
//
// **卡死行恢复（本轮评审 A2）**：D20 的「broker 重投 + TryClaim 内联续占」是【通常】路径，非【必然】——
// 若 broker 在 rebalance 前已过期该消息（spec §10 自承 retention 会过期），PROCESSING、payload-NULL 行
// 既不被 FindEligibleHeads 取到（只查 RETRY_SCHEDULED）、也不被 MoveToDeadLetter 接收（payload-NULL 守卫），
// 会永久卡死。本方法对 lease_expires_at 早于 now - stuckProcessingThreshold 的孤儿另记 kind=`STUCK_PROCESSING`
// （§10 P1，与瞬态 LEASE_ORPHAN P2 区分），让运维从噪声中挑出真正需介入的行。确定性救活（运维 runbook）：
// 从 broker 重投/审计日志重建 payload → PR-7 运维 API `RecoverStuckProcessing(id, reconstructedPayload)`
// （Store 级方法，PR-7 随运维闭环交付，见 PR2_SCOPE carry-over）补 payload 转 RETRY_SCHEDULED。
//
// **Spec 修订前置（R2）**：STUCK_PROCESSING 扩了 spec §2.3 的闭合 enum（CLAIM_TOKEN_MISMATCH|LEASE_ORPHAN）——
// 须同步修订 spec §2.3（加 enum 值）+ §10（加 P1 告警行 + runbook 引用），或把 STUCK_PROCESSING 整体推到 PR-7
// 与 RecoverStuckProcessing 同单元交付；勿让 PR-2 发布 spec 未定义的 anomaly kind（运维在 Grafana 看到无 spec 含义的信号）。
const stuckProcessingThreshold = 2 * time.Hour // 超此年龄的孤儿升级为 STUCK_PROCESSING（P1）

func (s *GormStore) ObserveExpiredLeases(ctx context.Context, now time.Time) (int, error) {
	var rows []EventConsumptionModel
	if err := s.claimDB.WithContext(ctx).
		Where("status = ? AND lease_expires_at < ?", reliable.StatusProcessing, now).
		Limit(500).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	// 幂等守卫：同一孤儿行在被 TryClaim 续占前会被每个 tick 反复扫到，不能每次都写一条 anomaly
	// （否则 consumption_anomaly_total{kind="LEASE_ORPHAN"} 的 >10/h 告警会被自身刷爆）。
	// 以 (kind, event_id, handler_id, claim_id) 唯一：同一次占位只记一条。claim_id 变化即新一次孤儿。
	anomalies := make([]AnomalyModel, 0, len(rows))
	for _, r := range rows {
		// A2：超龄孤儿升级 STUCK_PROCESSING（P1），与瞬态 LEASE_ORPHAN（P2）区分；kind 不同 → uk_anomaly_once 独立去重。
		kind := "LEASE_ORPHAN"
		if r.LeaseExpiresAt != nil && now.Sub(*r.LeaseExpiresAt) > stuckProcessingThreshold {
			kind = "STUCK_PROCESSING"
		}
		anomalies = append(anomalies, AnomalyModel{
			Kind: kind, EventID: r.EventID, HandlerID: r.HandlerID,
			TenantID:  r.TenantID, // B8：值类型，非指针
			ClaimID:   r.ClaimID,  // 幂等键的一部分：标识「哪一次占位成了孤儿」
			Detail:    "lease expired during processing; awaiting broker redelivery + TryClaim inline reclaim",
			CreatedAt: nowUTC(),
		})
	}
	// ON CONFLICT DO NOTHING：依赖 consumption_anomalies 的 uk_anomaly_once 唯一索引（两方言 DDL 同步加）。
	if err := s.claimDB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&anomalies).Error; err != nil {
		return 0, err
	}
	return len(rows), nil
}

// —— FindEligibleHeads（§6.2.1；D5：删死分支 + 无聚合行跳过分组）——

// EligibleHeadsSQL 是 eligible-head 查询（§6.2.1）。无聚合行（通知类）跳过 NOT EXISTS，自由并行；
// 有聚合行才排除「同聚合存在更早未解决行」。
//
// **包级导出常量（D22 本轮评审）**：repotest 的 EXPLAIN 门禁直接引用同一个字符串，零复制、零漂移
// ——若把 SQL 复制一份进测试，门禁很快就会变成在测一个没人跑的查询。
//
// D22（本轮评审）：status 写**字面量**而非参数。PG 的 idx_due 是 partial index
// （`WHERE status = 'RETRY_SCHEDULED'`），参数化 `status = $1` 在 generic plan 下无法蕴含该谓词
// → 索引失效退化 Seq Scan，且是「执行 5 次后才切 generic plan」的间歇性退化。本查询只查这一个
// 状态，写死字面量既消除歧义又让两方言都稳定命中索引（EXPLAIN 门禁才可能是确定性的）。
//
// A4（本轮评审）：`FOR UPDATE SKIP LOCKED` 在 autocommit 的 claimDB 上，行锁只存活于本语句期间，
// 语句一结束即释放。**它不是跨 claim 的持有**，只是并发扫描者之间的 best-effort 去重
// （同一瞬间两个 scheduler 各取不同批次）。真正防止重复重放的是 ClaimForReplay 的 CAS
// （WHERE status='RETRY_SCHEDULED' AND ...）——不要误以为这里的锁提供了正确性保证。
const EligibleHeadsSQL = `
SELECT c.* FROM event_consumption c
WHERE c.status = 'RETRY_SCHEDULED' AND c.next_attempt_at <= ?
  AND (c.aggregate_type IS NULL OR c.aggregate_id = ''
       OR NOT EXISTS (
         SELECT 1 FROM event_consumption e
         WHERE e.tenant_id = c.tenant_id
           AND e.aggregate_type = c.aggregate_type AND e.aggregate_id = c.aggregate_id
           AND e.status IN ('RETRY_SCHEDULED','PROCESSING','DEAD_LETTER')
           AND (
             (e.causal_seq IS NOT NULL AND c.causal_seq IS NOT NULL AND e.causal_seq < c.causal_seq)
             OR (e.causal_seq IS NULL AND c.causal_seq IS NULL
                 AND e.src_partition IS NOT NULL AND c.src_partition IS NOT NULL
                 AND (e.src_partition < c.src_partition OR (e.src_partition = c.src_partition AND e.src_offset < c.src_offset)))
             OR (e.causal_seq IS NULL AND c.causal_seq IS NULL
                 AND (e.src_partition IS NULL OR c.src_partition IS NULL)
                 AND e.first_seen_at < c.first_seen_at)
           )
       ))
ORDER BY c.next_attempt_at ASC
LIMIT ?
FOR UPDATE SKIP LOCKED`

func (s *GormStore) FindEligibleHeads(ctx context.Context, now time.Time, limit int) ([]store.Row, error) {
	if limit <= 0 {
		limit = 50
	}
	var models []EventConsumptionModel
	// D22：status 已写字面量，参数只剩 now 与 limit。
	if err := s.claimDB.WithContext(ctx).Raw(EligibleHeadsSQL, now, limit).Scan(&models).Error; err != nil {
		return nil, err
	}
	out := make([]store.Row, len(models))
	for i := range models {
		out[i] = models[i].ToRow()
	}
	return out, nil
}

// —— ClaimForReplay（RETRY_SCHEDULED→PROCESSING，attempt+1；含 MANUAL 授权消费）——

func (s *GormStore) ClaimForReplay(ctx context.Context, db *gorm.DB, id int64) (reliable.ClaimToken, store.Row, error) {
	claimID := uuid.NewString()
	now := nowUTC()
	lease := 5 * time.Minute
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND replay_mode = ?", id, reliable.StatusRetryScheduled, "AUTO").
		Updates(map[string]any{
			"status": reliable.StatusProcessing, "claim_id": claimID,
			"claimed_at": now, "lease_expires_at": now.Add(lease),
			"last_attempt_at": now, "attempt": gorm.Expr("attempt + 1"),
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 1 {
		var m EventConsumptionModel
		if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
			return "", store.Row{}, err
		}
		return reliable.ClaimToken(claimID), m.ToRow(), nil
	}
	// AUTO CAS 失败：区分「该行是 MANUAL（走人工授权 claim）」与「AUTO 行被别实例抢占/已变状态」。
	// **本轮评审 A1（P0）**：原稿无条件 fall-through 到 claimManualReplay——AUTO 行被别实例 claim 后，
	// 在 claimManualReplay 里因 ReplayMode!=MANUAL 返回 ErrNotPermitted，scheduler 把它送进 MoveToDeadLetter，
	// 毒掉别实例在途的 claim → DEAD_LETTER → 后续重投命中 AlreadySettled 静默永久丢数据（spec §4 v2.7 警告）。
	// 现仅 MANUAL 行走人工路径；其余一律 ErrRetryLater（让路 AdvanceDue，不增 attempt、绝不进死信）。
	var cur EventConsumptionModel
	if err := db.WithContext(ctx).First(&cur, id).Error; err != nil {
		return "", store.Row{}, err
	}
	if cur.ReplayMode == "MANUAL" {
		return s.claimManualReplay(ctx, db, id, claimID, now, lease)
	}
	return "", store.Row{}, reliable.ErrRetryLater
}

func (s *GormStore) claimManualReplay(ctx context.Context, db *gorm.DB, id int64, claimID string, now time.Time, lease time.Duration) (reliable.ClaimToken, store.Row, error) {
	var m EventConsumptionModel
	if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
		return "", store.Row{}, err
	}
	if m.ReplayMode != "MANUAL" || m.ReplayAuthID == "" || m.ReplayAuthConsumedAt != nil {
		return "", store.Row{}, reliable.ErrNotPermitted
	}
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND replay_auth_id = ? AND replay_auth_consumed_at IS NULL",
			id, reliable.StatusRetryScheduled, m.ReplayAuthID).
		Updates(map[string]any{
			"status": reliable.StatusProcessing, "claim_id": claimID,
			"claimed_at": now, "lease_expires_at": now.Add(lease),
			"last_attempt_at": now, "attempt": gorm.Expr("attempt + 1"),
			"replay_auth_consumed_at": now, "row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return "", store.Row{}, reliable.ErrNotPermitted
	}
	if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
		return "", store.Row{}, err
	}
	return reliable.ClaimToken(claimID), m.ToRow(), nil
}

// —— D8：scheduler 三分支处置经 Store 方法（不绕过 §2.4）——

// AdvanceDue 推进 next_attempt_at，**不增 attempt**（§6.2 「让路不是失败」）。
// A3（本轮评审）：加 status 守卫。本方法只对 RETRY_SCHEDULED 行有意义——原稿无守卫，
// scheduler 在 gate 抢不到时对已进 PROCESSING 的行调 AdvanceDue，结果行永远停在 PROCESSING
// （FindEligibleHeads 再也取不到）。现在 scheduler 已把 gate 前置于 ClaimForReplay（见 T11），
// 本方法只会作用于 RETRY_SCHEDULED 行；守卫把这一前提变成可执行的断言。
func (s *GormStore) AdvanceDue(ctx context.Context, db *gorm.DB, id int64) error {
	var m EventConsumptionModel
	if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
		return err
	}
	next := nowUTC().Add(reliable.Backoff(m.Attempt, reliable.DefaultBackoffBase, reliable.DefaultBackoffCap, jitterFraction(m.ID, m.Attempt)))
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ?", id, reliable.StatusRetryScheduled).
		Updates(map[string]any{
			"next_attempt_at": next, "updated_at": nowUTC(),
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// ReleaseClaim 归还已占位的行（A3 本轮评审新增）：PROCESSING → RETRY_SCHEDULED。
// 清 ownership + 按退避推进 due，**不改 attempt**（让路不是失败）。必须出示 tok（fencing）。
//
// chk_retry_due 要求 RETRY_SCHEDULED 必有 payload / next_attempt_at / error_class：
// 前两者由本方法保证（payload 在 claim 期间未被清，due 在这里写）；error_class 因为能被
// ClaimForReplay 拿到的行必定曾是 RETRY_SCHEDULED，所以也非空。WHERE 里仍显式守卫，
// 宁可 ErrConflict 也不招约束错误。
func (s *GormStore) ReleaseClaim(ctx context.Context, db *gorm.DB, id int64, tok reliable.ClaimToken) error {
	var m EventConsumptionModel
	if err := db.WithContext(ctx).First(&m, id).Error; err != nil {
		return err
	}
	now := nowUTC()
	next := now.Add(reliable.Backoff(m.Attempt, reliable.DefaultBackoffBase, reliable.DefaultBackoffCap, jitterFraction(m.ID, m.Attempt)))
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND claim_id = ? AND payload IS NOT NULL AND error_class IS NOT NULL",
			id, reliable.StatusProcessing, string(tok)).
		Updates(map[string]any{
			"status":   reliable.StatusRetryScheduled,
			"claim_id": nil, "claimed_at": nil, "lease_expires_at": nil,
			"next_attempt_at": next, "updated_at": now,
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// MoveToDeadLetter 把【未占位】的 RETRY_SCHEDULED 行移出自动重放队列（§6.2 表：pre-claim 的
// ErrNotPermitted / ErrNotSelfReplayable）。
//
// **本轮评审 A5**：原 A2 把源态放宽到含 PROCESSING 却不校验 claim token——任何调用方都能毒掉别实例
// 在途的 claim（也是 A1 竞态的 exploiting 机制，且独立违反 spec §3.1「Mark* 须带 claim_id」的 fencing
// 对称性——MarkSucceeded/MarkFailed 都要 token，唯独原 MoveToDeadLetter 不要）。现仅认 RETRY_SCHEDULED
// （pre-claim，无需 token）；已占位的 PROCESSING 行走 MoveToDeadLetterWithToken。
//
// chk_dead_payload 要求 DEAD_LETTER 必有 payload 与 error_class，WHERE 里显式加两个 IS NOT NULL 守卫：
// 宁可返回 ErrConflict 让调用方告警，也不要招来约束错误。离开 RETRY_SCHEDULED 时统一清 ownership
// （§2.4 不变量表：DEAD_LETTER 必清 claim_id/claimed_at/lease_expires_at/next_attempt_at）。
func (s *GormStore) MoveToDeadLetter(ctx context.Context, db *gorm.DB, id int64, reason string) error {
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND payload IS NOT NULL AND error_class IS NOT NULL",
			id, reliable.StatusRetryScheduled).
		Updates(map[string]any{
			"status": reliable.StatusDeadLetter, "next_attempt_at": nil,
			"claim_id": nil, "claimed_at": nil, "lease_expires_at": nil,
			"error_message": sanitizeMsg(reason), "updated_at": nowUTC(),
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 1 {
		return nil
	}
	// **R2**：rows==0 区分「状态被别实例抢走（预期，不报错）」与「guard 未命中（真失败）」。原稿一律 ErrConflict
	// → scheduler 对良性竞态也发 REPLAY_DISPOSE_FAILED，训炼运维忽视该通道（boy-who-cried-wolf）。
	// re-SELECT：状态已离开 RETRY_SCHEDULED（被并行 ClaimForReplay 或别实例 MoveToDeadLetter 处理）→ nil（良性）；
	// 仍 RETRY_SCHEDULED 却 0 行 = payload/error_class NULL（违反 chk_retry_due，真异常）→ ErrConflict 告警。
	var cur EventConsumptionModel
	if err := db.WithContext(ctx).First(&cur, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 行消失：无现存 Store 路径删除 RETRY_SCHEDULED 行（Discard/ScheduleReplay 均要求 DEAD_LETTER 源）——属 out-of-band DELETE；返回 nil 视为良性，若未来加 purge API 须重审
		}
		return err // 真 DB 错误，上抛
	}
	if cur.Status != string(reliable.StatusRetryScheduled) {
		return nil // 状态已迁移，别实例在处理/已处置，不告警
	}
	return reliable.ErrConflict // 仍 RETRY_SCHEDULED 却未更新 = guard 未命中，真失败
}

// MoveToDeadLetterWithToken 把【已占位】的 PROCESSING 行移到 DEAD_LETTER，须出示 claim token（A5 fencing）。
// 供 scheduler 在 ClaimForReplay 之后命中 ErrNotPermitted/ErrNotSelfReplayable 时调用——此时行是 PROCESSING、
// 我们持有 tok，payload 与 error_class 在 ClaimForReplay 后仍在（DEAD_LETTER 硬条件满足）。
// WHERE claim_id=tok 保证不会毒掉别实例在途的 claim（与 MarkSucceeded/MarkFailed 对称）；0 行返回 ErrConflict。
func (s *GormStore) MoveToDeadLetterWithToken(ctx context.Context, db *gorm.DB, id int64, tok reliable.ClaimToken, reason string) error {
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND claim_id = ? AND payload IS NOT NULL AND error_class IS NOT NULL",
			id, reliable.StatusProcessing, string(tok)).
		Updates(map[string]any{
			"status": reliable.StatusDeadLetter, "next_attempt_at": nil,
			"claim_id": nil, "claimed_at": nil, "lease_expires_at": nil,
			"error_message": sanitizeMsg(reason), "updated_at": nowUTC(),
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// —— ScheduleReplay（DEAD_LETTER→RETRY_SCHEDULED，CAS；D12：requester≠approver 强制）——

func (s *GormStore) ScheduleReplay(ctx context.Context, db *gorm.DB, id, expectedVersion int64,
	requester, approver, reason string) error {
	if requester == approver {
		// D12：双人确认由 Store 强制（纵深防御），不只靠服务运维层。
		return reliable.ErrConflict
	}
	now := nowUTC()
	authID := uuid.NewString()
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND row_version = ?", id, reliable.StatusDeadLetter, expectedVersion).
		Updates(map[string]any{
			"status":              reliable.StatusRetryScheduled,
			"replay_generation":   gorm.Expr("replay_generation + 1"),
			"replay_mode":         "MANUAL",
			"replay_requested_by": requester, "replay_approved_by": approver, "replay_reason": reason,
			"replay_auth_id": authID, "replay_auth_consumed_at": nil,
			"next_attempt_at": now, "updated_at": now,
			"row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

func (s *GormStore) Discard(ctx context.Context, db *gorm.DB, id, expectedVersion int64, by, reason string) error {
	now := nowUTC()
	rows := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND row_version = ?", id, reliable.StatusDeadLetter, expectedVersion).
		Updates(map[string]any{
			"status": reliable.StatusDiscarded, "resolved_at": now, "resolved_by": by,
			"discard_reason": reason, "claim_id": nil, "claimed_at": nil,
			"lease_expires_at": nil, "next_attempt_at": nil,
			"updated_at": now, "row_version": gorm.Expr("row_version + 1"),
		}).RowsAffected
	if rows == 0 {
		return reliable.ErrConflict
	}
	return nil
}

// —— aggregate gate（§6.2.1；D18#7：token=holder+uuid 唯一）——

func (s *GormStore) AcquireAggregateGate(ctx context.Context, db *gorm.DB, key reliable.AggregateGateKey,
	holder string, ttl time.Duration) (string, error) {
	if key.Empty() {
		return "", nil
	}
	now := nowUTC()
	expires := now.Add(ttl)
	token := holder + ":" + uuid.NewString() // D18#7：唯一 token

	// A6（本轮评审）：原稿是「Create 失败 → 在同一个 db 上继续 UPDATE」。若调用方传的是事务句柄
	// （签名收 *gorm.DB 就是为了能加入业务事务），PostgreSQL 在第一个 INSERT 失败后即进入 aborted
	// 状态，后续语句全部 `25P02: current transaction is aborted`——MySQL 上却侥幸能过，
	// 是典型的「MySQL 绿 / PG 红」双方言陷阱。
	//
	// 改为两步均不可能报错的写法（两方言行为一致）：
	//   1) 先 CAS 覆盖【已过期】的 gate（纯 UPDATE，无冲突风险）；
	//   2) 再 INSERT ... ON CONFLICT DO NOTHING（PG）/ ON DUPLICATE KEY UPDATE no-op（MySQL），
	//      冲突时 RowsAffected==0 而不报错，不会污染调用方事务。
	// 两步都未得手 → 有人持有活跃 gate → ErrRetryLater。
	if rows := db.WithContext(ctx).Model(&AggregateLeaseModel{}).
		Where("tenant_id = ? AND aggregate_type = ? AND aggregate_id = ? AND expires_at < ?",
			key.TenantID, key.AggregateType, key.AggregateID, now).
		Updates(map[string]any{"holder_id": token, "acquired_at": now, "expires_at": expires}).RowsAffected; rows == 1 {
		return token, nil
	}
	m := &AggregateLeaseModel{
		TenantID: key.TenantID, AggregateType: key.AggregateType, AggregateID: key.AggregateID,
		HolderID: token, AcquiredAt: now, ExpiresAt: expires,
	}
	res := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(m)
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 1 {
		return token, nil
	}
	return "", reliable.ErrRetryLater
}

func (s *GormStore) ReleaseAggregateGate(ctx context.Context, db *gorm.DB, token string) error {
	// 依赖 idx_holder（D3）；无索引时这里是全表扇 + 行锁，而 gate 在重放热路径上。
	return db.WithContext(ctx).Where("holder_id = ?", token).Delete(&AggregateLeaseModel{}).Error
}

func (s *GormStore) ReclaimExpiredAggregateGates(ctx context.Context, now time.Time) (int, error) {
	res := s.markDB.WithContext(ctx).Where("expires_at < ?", now).Delete(&AggregateLeaseModel{}).RowsAffected
	return int(res), nil
}

// —— 异常记录（§2.3；D18#8：带 tenantID）——

// RecordAnomaly 写一条异常。幂等：依赖 uk_anomaly_once (kind, event_id, handler_id, claim_id) +
// ON CONFLICT DO NOTHING——同一次占位的同类异常只记一条，防止 ObserveExpiredLeases 每 tick
// 重复刷同一孤儿行把 consumption_anomaly_total{kind="LEASE_ORPHAN"} 的 >10/h 告警刷爆。
func (s *GormStore) RecordAnomaly(ctx context.Context, db *gorm.DB, tenantID int, kind string, key reliable.Key, claimID, detail string) error {
	return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&AnomalyModel{
		Kind: kind, EventID: key.EventID, HandlerID: string(key.Handler),
		TenantID: tenantID, ClaimID: claimID, Detail: detail, CreatedAt: nowUTC(),
	}).Error
}

// —— 读 ——
func (s *GormStore) GetByID(ctx context.Context, id int64) (store.Row, error) {
	var m EventConsumptionModel
	if err := s.markDB.WithContext(ctx).First(&m, id).Error; err != nil {
		return store.Row{}, err
	}
	return m.ToRow(), nil
}

func (s *GormStore) List(ctx context.Context, f store.ListFilter) ([]store.Row, error) {
	// S3（本轮评审）：多租户隔离——List 必须显式 tenant 作用域（PR-2 无全局/admin 消费者）。
	// TenantID==0 视为「忘记限定租户」，拒绝而非静默跨租户读；全局视图（PR-7 运维）另立 ListGlobal。
	if f.TenantID == 0 {
		return nil, fmt.Errorf("reliable: ListFilter.TenantID is required for multi-tenant isolation (S3); bind a per-tenant *gorm.DB and set TenantID")
	}
	q := s.markDB.WithContext(ctx).Model(&EventConsumptionModel{}).Where("tenant_id = ?", f.TenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.ErrorClass != "" {
		q = q.Where("error_class = ?", f.ErrorClass)
	}
	if f.HandlerID != "" {
		q = q.Where("handler_id = ?", f.HandlerID)
	}
	if !f.From.IsZero() {
		q = q.Where("first_seen_at >= ?", f.From)
	}
	if !f.To.IsZero() {
		q = q.Where("first_seen_at < ?", f.To)
	}
	if f.Limit <= 0 {
		f.Limit = 100
	}
	var ms []EventConsumptionModel
	if err := q.Order("id DESC").Limit(f.Limit).Offset(f.Offset).Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]store.Row, len(ms))
	for i := range ms {
		out[i] = ms[i].ToRow()
	}
	return out, nil
}

// —— ptr helpers ——
func ptrInt32(v int32) *int32 { return &v }
func ptrInt64(v int64) *int64 { return &v }
func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
