package gormshared

import (
	"context"
	"errors"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormQuarantineStore struct{ db *gorm.DB }

var _ store.QuarantineStore = (*GormQuarantineStore)(nil)

func NewQuarantineStore(db *gorm.DB) *GormQuarantineStore { return &GormQuarantineStore{db: db} }

func (q *GormQuarantineStore) Record(ctx context.Context, db *gorm.DB, row store.QuarantineRow) (int64, error) {
	now := nowUTC()
	if row.Status == "" {
		row.Status = "QUARANTINED"
	}
	m := &QuarantineModel{
		TenantID: row.TenantID, HandlerID: string(row.HandlerID), Topic: row.Topic,
		SrcPartition: row.SrcPartition, SrcOffset: row.SrcOffset,
		RawValue: row.RawValue, RawKey: row.RawKey,
		// B7：headers 列是 NOT NULL，空 header 必须落 JSON 空数组而非 NULL。
		Headers: marshalHeadersOrEmpty(row.Headers),
		// row.BrokerTimestamp is already *time.Time (store.QuarantineRow) — assign directly,
		// do NOT wrap with ptrTime (which expects a time.Time value). Model field is *time.Time.
		RawPayloadHash: row.RawPayloadHash, BrokerTimestamp: row.BrokerTimestamp,
		ErrorMessage: row.ErrorMessage, Status: row.Status, CreatedAt: now,
	}
	// A6（本轮评审）：原稿是「Create 报错 → 同一 db 上再 SELECT」，在 PostgreSQL 调用方事务里
	// 会因第一个 INSERT 失败而整个 tx aborted（25P02），后续 SELECT 全废。改为
	// ON CONFLICT DO NOTHING：不报错、不污染事务，冲突时 RowsAffected==0 再读回已有 id。
	res := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(m)
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 1 {
		return m.ID, nil
	}
	var exist QuarantineModel
	// review #1：回读必须带 tenant_id——否则共享库下 ON CONFLICT 命中别租户行时，会读回第一租户的 id（id 污染）。
	if err := db.WithContext(ctx).Where("tenant_id = ? AND topic = ? AND src_partition = ? AND src_offset = ? AND handler_id = ?",
		row.TenantID, row.Topic, row.SrcPartition, row.SrcOffset, string(row.HandlerID)).First(&exist).Error; err != nil {
		return 0, err
	}
	return exist.ID, nil
}

func (q *GormQuarantineStore) GetByID(ctx context.Context, tenantID int, id int64) (store.QuarantineRow, error) {
	if tenantID <= 0 {
		return store.QuarantineRow{}, fmt.Errorf("reliable: QuarantineStore.GetByID tenantID must be > 0 for multi-tenant isolation (S3)")
	}
	var m QuarantineModel
	// review #1：强制 tenant 作用域——隔离区存的是不可解码的毒消息（最可能带 PII），绝不能跨租户裸读。
	if err := q.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&m).Error; err != nil {
		return store.QuarantineRow{}, err
	}
	return m.ToRow(), nil
}

func (q *GormQuarantineStore) List(ctx context.Context, tenantID int, status string, limit int) ([]store.QuarantineRow, error) {
	if tenantID <= 0 {
		return nil, fmt.Errorf("reliable: QuarantineStore.List tenantID must be > 0 for multi-tenant isolation (S3)")
	}
	if limit <= 0 {
		limit = 100
	}
	var ms []QuarantineModel
	qry := q.db.WithContext(ctx).Model(&QuarantineModel{}).Where("tenant_id = ?", tenantID)
	if status != "" {
		qry = qry.Where("status = ?", status)
	}
	if err := qry.Order("id DESC").Limit(limit).Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]store.QuarantineRow, len(ms))
	for i := range ms {
		out[i] = ms[i].ToRow()
	}
	return out, nil
}

func (q *GormQuarantineStore) MarkResolved(ctx context.Context, db *gorm.DB, tenantID int, id, expectedVersion int64, by string) error {
	if tenantID <= 0 {
		return fmt.Errorf("reliable: QuarantineStore.MarkResolved tenantID must be > 0 for multi-tenant isolation (S3)")
	}
	now := nowUTC()
	// review #2：WHERE 含 tenant_id——跨租户凭泄露的 (id, version) 处置命中 0 行 → ErrConflict。
	// 与 GetByID/List 同等级隔离；0 行同时覆盖「版本不匹配/已处置/租户不符」，不泄露 id 归属。
	// review #4：先查 res.Error——DB 错误/ctx 取消（RowsAffected=0）不得伪装成 ErrConflict。
	res := db.WithContext(ctx).Model(&QuarantineModel{}).
		Where("id = ? AND tenant_id = ? AND row_version = ?", id, tenantID, expectedVersion).
		Updates(map[string]any{
			"status": "RESOLVED", "resolved_at": now, "resolved_by": by,
			"row_version": gorm.Expr("row_version + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.Join(reliable.ErrConflict, errors.New("quarantine row version mismatch"))
	}
	return nil
}
