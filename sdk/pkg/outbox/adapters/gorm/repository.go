package gorm

import (
	"context"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	"gorm.io/gorm"
)

// GormOutboxRepository GORM 仓储实现
type GormOutboxRepository struct {
	db *gorm.DB
}

// NewGormOutboxRepository 创建 GORM 仓储
func NewGormOutboxRepository(db *gorm.DB) outbox.OutboxRepository {
	return &GormOutboxRepository{db: db}
}

// Save 保存事件
func (r *GormOutboxRepository) Save(ctx context.Context, event *outbox.OutboxEvent) error {
	model := FromEntity(event)
	return r.db.WithContext(ctx).Create(model).Error
}

// SaveBatch 批量保存事件
func (r *GormOutboxRepository) SaveBatch(ctx context.Context, events []*outbox.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}
	models := FromEntities(events)
	return r.db.WithContext(ctx).Create(models).Error
}

// SaveInTx 在事务中保存事件（实现 TransactionalRepository 接口）
func (r *GormOutboxRepository) SaveInTx(ctx context.Context, tx interface{}, event *outbox.OutboxEvent) error {
	gormTx, ok := tx.(*gorm.DB)
	if !ok {
		return fmt.Errorf("invalid transaction type, expected *gorm.DB")
	}

	model := FromEntity(event)
	return gormTx.WithContext(ctx).Create(model).Error
}

// BeginTx 开始事务（实现 TransactionalRepository 接口）
func (r *GormOutboxRepository) BeginTx(ctx context.Context) (interface{}, error) {
	return r.db.WithContext(ctx).Begin(), nil
}

// CommitTx 提交事务（实现 TransactionalRepository 接口）
func (r *GormOutboxRepository) CommitTx(tx interface{}) error {
	gormTx, ok := tx.(*gorm.DB)
	if !ok {
		return fmt.Errorf("invalid transaction type, expected *gorm.DB")
	}
	return gormTx.Commit().Error
}

// RollbackTx 回滚事务（实现 TransactionalRepository 接口）
func (r *GormOutboxRepository) RollbackTx(tx interface{}) error {
	gormTx, ok := tx.(*gorm.DB)
	if !ok {
		return fmt.Errorf("invalid transaction type, expected *gorm.DB")
	}
	return gormTx.Rollback().Error
}

// FindPendingEvents 查找待发布的事件
func (r *GormOutboxRepository) FindPendingEvents(ctx context.Context, limit int, tenantID int) ([]*outbox.OutboxEvent, error) {
	var models []*OutboxEventModel

	query := r.db.WithContext(ctx).
		Where("status = ?", outbox.EventStatusPending).
		Order("created_at ASC").
		Limit(limit)

	// 如果指定了租户 ID，添加租户过滤
	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	// 只查询应该立即发布的事件（scheduled_at 为空或已到时间）
	query = query.Where("scheduled_at IS NULL OR scheduled_at <= ?", time.Now())

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return ToEntities(models), nil
}

// FindByID 根据 ID 查找事件
func (r *GormOutboxRepository) FindByID(ctx context.Context, id string) (*outbox.OutboxEvent, error) {
	var model OutboxEventModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error; err != nil {
		return nil, err
	}
	return model.ToEntity(), nil
}

// FindByAggregateID 根据聚合根 ID 查找事件
func (r *GormOutboxRepository) FindByAggregateID(ctx context.Context, aggregateID string, tenantID int) ([]*outbox.OutboxEvent, error) {
	var models []*OutboxEventModel

	query := r.db.WithContext(ctx).Where("aggregate_id = ?", aggregateID)

	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	if err := query.Order("created_at ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	return ToEntities(models), nil
}

// FindPendingEventsWithDelay 查找创建时间超过指定延迟的待发布事件
func (r *GormOutboxRepository) FindPendingEventsWithDelay(ctx context.Context, tenantID int, delaySeconds int, limit int) ([]*outbox.OutboxEvent, error) {
	var models []*OutboxEventModel

	// 计算延迟时间点
	delayTime := time.Now().Add(-time.Duration(delaySeconds) * time.Second)

	query := r.db.WithContext(ctx).
		Where("status = ?", outbox.EventStatusPending).
		Where("created_at < ?", delayTime).
		Order("created_at ASC").
		Limit(limit)

	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	// 只查询应该立即发布的事件（scheduled_at 为空或已到时间）
	query = query.Where("scheduled_at IS NULL OR scheduled_at <= ?", time.Now())

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return ToEntities(models), nil
}

// FindEventsForRetry 查找需要重试的失败事件
func (r *GormOutboxRepository) FindEventsForRetry(ctx context.Context, maxRetries int, limit int) ([]*outbox.OutboxEvent, error) {
	var models []*OutboxEventModel

	query := r.db.WithContext(ctx).
		Where("status = ?", outbox.EventStatusFailed).
		Where("retry_count < ?", maxRetries).
		Order("created_at ASC").
		Limit(limit)

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return ToEntities(models), nil
}

// FindByAggregateType 根据聚合类型查找待发布事件
func (r *GormOutboxRepository) FindByAggregateType(ctx context.Context, aggregateType string, limit int) ([]*outbox.OutboxEvent, error) {
	var models []*OutboxEventModel

	query := r.db.WithContext(ctx).
		Where("aggregate_type = ?", aggregateType).
		Where("status = ?", outbox.EventStatusPending).
		Order("created_at ASC").
		Limit(limit)

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return ToEntities(models), nil
}

// Update 更新事件
func (r *GormOutboxRepository) Update(ctx context.Context, event *outbox.OutboxEvent) error {
	model := FromEntity(event)
	return r.db.WithContext(ctx).Save(model).Error
}

// MarkAsPublished 标记事件为已发布
func (r *GormOutboxRepository) MarkAsPublished(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       outbox.EventStatusPublished,
			"published_at": now,
			"updated_at":   now,
		}).Error
}

// MarkAsFailed 标记事件为失败
func (r *GormOutboxRepository) MarkAsFailed(ctx context.Context, id string, err error) error {
	updates := map[string]interface{}{
		"status":     outbox.EventStatusFailed,
		"updated_at": time.Now(),
	}

	if err != nil {
		updates["last_error"] = err.Error()
	}

	return r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// IncrementRetry 增加重试次数（同时更新错误信息和最后重试时间）
func (r *GormOutboxRepository) IncrementRetry(ctx context.Context, id string, errorMsg string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"retry_count":   gorm.Expr("retry_count + 1"),
			"last_error":    errorMsg,
			"last_retry_at": now,
			"status":        outbox.EventStatusFailed,
			"updated_at":    now,
		}).Error
}

// MarkAsMaxRetry 标记事件为超过最大重试次数
func (r *GormOutboxRepository) MarkAsMaxRetry(ctx context.Context, id string, errorMsg string) error {
	return r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     outbox.EventStatusMaxRetry,
			"last_error": errorMsg,
			"updated_at": time.Now(),
		}).Error
}

// IncrementRetryCount 增加重试次数（已废弃，使用 IncrementRetry 代替）
// Deprecated: 使用 IncrementRetry 代替
func (r *GormOutboxRepository) IncrementRetryCount(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"retry_count": gorm.Expr("retry_count + 1"),
			"updated_at":  time.Now(),
		}).Error
}

// Delete 删除事件
func (r *GormOutboxRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&OutboxEventModel{}).Error
}

// DeleteBatch 批量删除事件
func (r *GormOutboxRepository) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&OutboxEventModel{}).Error
}

// DeletePublishedBefore 删除指定时间之前已发布的事件
func (r *GormOutboxRepository) DeletePublishedBefore(ctx context.Context, before time.Time, tenantID int) (int64, error) {
	query := r.db.WithContext(ctx).
		Where("status = ?", outbox.EventStatusPublished).
		Where("published_at < ?", before)

	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	result := query.Delete(&OutboxEventModel{})
	return result.RowsAffected, result.Error
}

// DeleteFailedBefore 删除指定时间之前失败的事件
func (r *GormOutboxRepository) DeleteFailedBefore(ctx context.Context, before time.Time, tenantID int) (int64, error) {
	query := r.db.WithContext(ctx).
		Where("status = ?", outbox.EventStatusFailed).
		Where("updated_at < ?", before)

	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	result := query.Delete(&OutboxEventModel{})
	return result.RowsAffected, result.Error
}

// Count 统计事件数量
func (r *GormOutboxRepository) Count(ctx context.Context, status outbox.EventStatus, tenantID int) (int64, error) {
	var count int64

	query := r.db.WithContext(ctx).Model(&OutboxEventModel{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

// CountByStatus 按状态统计事件数量
func (r *GormOutboxRepository) CountByStatus(ctx context.Context, tenantID int) (map[outbox.EventStatus]int64, error) {
	type StatusCount struct {
		Status string
		Count  int64
	}

	var results []StatusCount

	query := r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Select("status, COUNT(*) as count").
		Group("status")

	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	if err := query.Scan(&results).Error; err != nil {
		return nil, err
	}

	counts := make(map[outbox.EventStatus]int64)
	for _, result := range results {
		counts[outbox.EventStatus(result.Status)] = result.Count
	}

	return counts, nil
}

// GetStats 获取统计信息（实现 RepositoryStatsProvider 接口）
func (r *GormOutboxRepository) GetStats(ctx context.Context, tenantID int) (*outbox.RepositoryStats, error) {
	counts, err := r.CountByStatus(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	stats := &outbox.RepositoryStats{
		PendingCount:   counts[outbox.EventStatusPending],
		PublishedCount: counts[outbox.EventStatusPublished],
		FailedCount:    counts[outbox.EventStatusFailed],
		TenantID:       tenantID,
	}

	stats.TotalCount = stats.PendingCount + stats.PublishedCount + stats.FailedCount

	// 查找最老的待发布事件
	var oldestPending OutboxEventModel
	query := r.db.WithContext(ctx).
		Where("status = ?", outbox.EventStatusPending).
		Order("created_at ASC").
		Limit(1)

	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	if err := query.First(&oldestPending).Error; err == nil {
		stats.OldestPendingAge = time.Since(oldestPending.CreatedAt)
	}

	return stats, nil
}

// FindByIdempotencyKey 根据幂等性键查找事件
func (r *GormOutboxRepository) FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*outbox.OutboxEvent, error) {
	if idempotencyKey == "" {
		return nil, fmt.Errorf("idempotency key cannot be empty")
	}

	var model OutboxEventModel
	err := r.db.WithContext(ctx).
		Where("idempotency_key = ?", idempotencyKey).
		First(&model).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 不存在返回 nil，不是错误
		}
		return nil, err
	}

	return model.ToEntity(), nil
}

// ExistsByIdempotencyKey 检查幂等性键是否已存在
func (r *GormOutboxRepository) ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error) {
	if idempotencyKey == "" {
		return false, fmt.Errorf("idempotency key cannot be empty")
	}

	var count int64
	err := r.db.WithContext(ctx).
		Model(&OutboxEventModel{}).
		Where("idempotency_key = ?", idempotencyKey).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// FindMaxRetryEvents 查找超过最大重试次数的事件
func (r *GormOutboxRepository) FindMaxRetryEvents(ctx context.Context, limit int, tenantID int) ([]*outbox.OutboxEvent, error) {
	var models []*OutboxEventModel

	query := r.db.WithContext(ctx).
		Where("status = ?", outbox.EventStatusMaxRetry).
		Order("created_at ASC").
		Limit(limit)

	if tenantID != 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return ToEntities(models), nil
}

// BatchUpdate 批量更新事件（性能优化）
func (r *GormOutboxRepository) BatchUpdate(ctx context.Context, events []*outbox.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}

	// 使用事务批量更新
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, event := range events {
			model := FromEntity(event)

			// 使用 Updates 方法更新所有字段
			if err := tx.Model(&OutboxEventModel{}).
				Where("id = ?", model.ID).
				Updates(model).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Ensure GormOutboxRepository implements the interfaces
var (
	_ outbox.OutboxRepository        = (*GormOutboxRepository)(nil)
	_ outbox.RepositoryStatsProvider = (*GormOutboxRepository)(nil)
	_ outbox.TransactionalRepository = (*GormOutboxRepository)(nil)
)
