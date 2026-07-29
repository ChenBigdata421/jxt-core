package gormshared

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

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
		// review #14：dup-key 竞态重试前加 jitter 退避。热点 key 上 N 个消费者同时连试会把
		// uk_event_handler 叶子上的 INSERT 争用放大 maxDupRetry 倍；退避把并发 claimant 错开。
		// 与 PR-7 的 single-RTT 优化正交——这里只是别让重试在争用最烈时硬撞。重试期间尊重 ctx 取消。
		delay := reliable.Backoff(i+1, 2*time.Millisecond, 50*time.Millisecond, s.jitter())
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", 0, ctx.Err()
		case <-timer.C:
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
			// review #4：先查 res.Error——DB 错误/ctx 取消不得伪装成 AlreadyProcessing（否则调用方零感知）。
			res := s.claimDB.WithContext(ctx).Model(&EventConsumptionModel{}).
				Where("id = ? AND status = ? AND lease_expires_at < ?", existing.ID, reliable.StatusProcessing, now).
				Updates(map[string]any{
					"status": reliable.StatusProcessing, "claim_id": claimID,
					"claimed_at": now, "lease_expires_at": expires, "last_attempt_at": now,
					"row_version": gorm.Expr("row_version + 1"),
				})
			if res.Error != nil {
				return "", 0, res.Error, false
			}
			if res.RowsAffected == 1 {
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
