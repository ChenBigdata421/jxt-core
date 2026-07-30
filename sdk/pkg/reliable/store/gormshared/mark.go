package gormshared

import (
	"context"
	"errors"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"gorm.io/gorm"
)

// —— MarkSucceeded（§3.1：WHERE claim_id=tok；0 行→ErrConflict）——

func (s *GormStore) MarkSucceeded(ctx context.Context, db *gorm.DB, key reliable.Key, tok reliable.ClaimToken) error {
	now := nowUTC()
	// 必须用 map[string]any 传 nil：struct-based Updates 会静默跳过 nil 字段，破坏 chk_* CHECK 不变量。
	// review #4：先查 res.Error 再看 RowsAffected——否则 DB 错误/ctx 取消（RowsAffected=0）被伪装成 ErrConflict。
	res := db.WithContext(ctx).Model(&EventConsumptionModel{}).
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
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
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
	// review #9：cause 下方被 cause.Error() 解引用（error_message/error_code/fingerprint），
	// nil 会 panic 并把业务事务打挂、行卡死 PROCESSING。与 payload 守卫同处理——先 fail-fast。
	if cause == nil {
		return fmt.Errorf("reliable: MarkFailed requires non-nil cause")
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
		updates["next_attempt_at"] = now.Add(reliable.Backoff(m.Attempt, reliable.DefaultBackoffBase, reliable.DefaultBackoffCap, s.jitter()))
	}
	// C4/§6.1 TOCTOU 防护：deadLetter 与 Backoff(m.Attempt) 都基于 SELECT 读到的 m.Attempt 计算。
	// 无法把决策整体下推到单条 UPDATE 的 CASE 表达式——next_attempt_at = Backoff(attempt) 的 jitter
	// 在 Go 侧计算（s.jitter() 真随机，不可移植到 SQL）。改用乐观锁：UPDATE 的 WHERE 加 attempt = m.Attempt，
	// 若 attempt 在 SELECT 与 UPDATE 之间被并发改动，则 0 行受影响 → 返回 ErrConflict（fail-fast，不污染状态），
	// 而非用过期的 deadLetter/退避覆盖行。
	// review #4：先查 res.Error（同 MarkSucceeded），DB 错误不得伪装成 CAS conflict。
	res := db.WithContext(ctx).Model(&EventConsumptionModel{}).
		Where("id = ? AND status = ? AND claim_id = ? AND attempt = ?", m.ID, reliable.StatusProcessing, string(tok), m.Attempt).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
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
	// review #9：同 MarkFailed——cause.Error() 不可解引用 nil。
	if cause == nil {
		return fmt.Errorf("reliable: RecordTerminal requires non-nil cause")
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
