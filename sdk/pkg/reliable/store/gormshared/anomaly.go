package gormshared

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
// 「过期即可再占位」由 TryClaim 的内联 CAS 续占独家负责（见 claim.go tryClaimOnce）——那条路径
// 会写入新 claim_id，chk_processing_owner 全程成立，且旧 token 的 Mark* 因 claim_id 不匹配
// 得到 ErrConflict，fencing token 语义完整。本方法只负责可观测性：批量写 LEASE_ORPHAN + 计数。
// 语义与 spec §3.2「payload IS NULL 靠 broker 重投」完全一致：broker 本就会再投，TryClaim 接住。
//
// 命名：接口方法名从 ReclaimExpiredLeases 改为 ObserveExpiredLeases，避免「回收」的误导性暗示
// （T6 store.Store 接口、T10 lease.Runner 同步改名）。
//
// **STUCK_PROCESSING 推迟到 PR-7（本轮评审 R3，原 A2/R2）**：曾按「lease_expires_at 早于 now - 2h 的
// 孤儿升级 kind=STUCK_PROCESSING（§10 P1）」区分瞬态 LEASE_ORPHAN 与真·卡死行——但 STUCK_PROCESSING
// 扩了 spec §2.3 的闭合 enum（CLAIM_TOKEN_MISMATCH|LEASE_ORPHAN），而 spec 不在本仓、无法随 PR-2 一并
// 修订 §2.3（加 enum 值）+ §10（加 P1 告警行 + runbook）。勿让 PR-2 发布 spec 未定义的 anomaly kind（运维
// 在 Grafana 看到无 spec 含义的信号）。整套卡死行闭环——STUCK_PROCESSING kind + PR-7 运维 API
// `RecoverStuckProcessing(id, reconstructedPayload)`（从 broker 重投/审计日志重建 payload 转 RETRY_SCHEDULED）
// + spec §2.3/§10 修订——随 PR-7 同单元交付（见 PR2_SCOPE deviation #5，已改为 carry-over）。
// PR-2 本方法只记 LEASE_ORPHAN；超龄孤儿在 PR-7 前 由 LEASE_ORPHAN 计数 + 人工巡检覆盖。

func (s *GormStore) ObserveExpiredLeases(ctx context.Context, now time.Time) (int, error) {
	var rows []EventConsumptionModel
	// review #12：只投影构造 anomaly 用到的 4 列。PROCESSING 行的 payload 可能很大，全行 Find 会让
	// lease 观测器每 tick 把多达 500 行的 LONGBLOB/BYTEA 拉进堆后立即丢弃。
	if err := s.claimDB.WithContext(ctx).
		Select("event_id", "handler_id", "tenant_id", "claim_id").
		Where("status = ? AND lease_expires_at < ?", reliable.StatusProcessing, now).
		Limit(500).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	// 幂等守卫：同一孤儿行在被 TryClaim 续占前会被每个 tick 反复扫到，不能每次都写一条 anomaly
	// （否则 consumption_anomaly_total{kind="LEASE_ORPHAN"} 的 >10/h 告警会被自身刷爆）。
	// 以 (kind, tenant_id, event_id, handler_id, claim_id) 唯一：同一次占位只记一条。claim_id 变化即新一次孤儿。
	anomalies := make([]AnomalyModel, 0, len(rows))
	for _, r := range rows {
		anomalies = append(anomalies, AnomalyModel{
			Kind: "LEASE_ORPHAN", EventID: r.EventID, HandlerID: r.HandlerID,
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

// —— 异常记录（§2.3；D18#8：带 tenantID）——

// RecordAnomaly 写一条异常。幂等：依赖 uk_anomaly_once (kind, tenant_id, event_id, handler_id, claim_id) +
// ON CONFLICT DO NOTHING——同一次占位的同类异常只记一条，防止 ObserveExpiredLeases 每 tick
// 重复刷同一孤儿行把 consumption_anomaly_total{kind="LEASE_ORPHAN"} 的 >10/h 告警刷爆。
func (s *GormStore) RecordAnomaly(ctx context.Context, db *gorm.DB, tenantID int, kind string, key reliable.Key, claimID, detail string) error {
	return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&AnomalyModel{
		Kind: kind, EventID: key.EventID, HandlerID: string(key.Handler),
		TenantID: tenantID, ClaimID: claimID, Detail: detail, CreatedAt: nowUTC(),
	}).Error
}
