package gormshared

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
)

// —— 读 ——
func (s *GormStore) GetByID(ctx context.Context, tenantID int, id int64) (store.Row, error) {
	var m EventConsumptionModel
	// review #5：强制 tenant 作用域——与 List 的 S3 守卫对齐，杜绝按主键枚举跨租户裸读 payload/headers。
	if err := s.markDB.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&m).Error; err != nil {
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
