package outbox

import (
	"context"
	"time"
)

// OutboxRepository Outbox 仓储接口
// 定义数据访问契约，不依赖具体数据库实现
type OutboxRepository interface {
	// Save 保存事件（在事务中）
	// ctx: 上下文（可能包含事务信息）
	// event: 要保存的事件
	Save(ctx context.Context, event *OutboxEvent) error

	// SaveBatch 批量保存事件（在事务中）
	// ctx: 上下文（可能包含事务信息）
	// events: 要保存的事件列表
	SaveBatch(ctx context.Context, events []*OutboxEvent) error

	// FindPendingEvents 查找待发布的事件
	// ctx: 上下文
	// limit: 最大返回数量
	// tenantID: 租户 ID（可选，0 表示查询所有租户）
	FindPendingEvents(ctx context.Context, limit int, tenantID int) ([]*OutboxEvent, error)

	// FindPendingEventsWithDelay 查找创建时间超过指定延迟的待发布事件
	// 用于调度器避让机制，防止与立即发布产生竞态
	// ctx: 上下文
	// tenantID: 租户 ID（可选）
	// delaySeconds: 延迟秒数（只查询创建时间超过此延迟的事件）
	// limit: 最大返回数量
	FindPendingEventsWithDelay(ctx context.Context, tenantID int, delaySeconds int, limit int) ([]*OutboxEvent, error)

	// FindEventsForRetry 查找需要重试的失败事件
	// ctx: 上下文
	// maxRetries: 最大重试次数（只查询重试次数小于此值的事件）
	// limit: 最大返回数量
	FindEventsForRetry(ctx context.Context, maxRetries int, limit int) ([]*OutboxEvent, error)

	// FindByAggregateType 根据聚合类型查找待发布事件
	// ctx: 上下文
	// aggregateType: 聚合类型
	// limit: 最大返回数量
	FindByAggregateType(ctx context.Context, aggregateType string, limit int) ([]*OutboxEvent, error)

	// FindByID 根据 ID 查找事件
	// ctx: 上下文
	// id: 事件 ID
	FindByID(ctx context.Context, id string) (*OutboxEvent, error)

	// FindByAggregateID 根据聚合根 ID 查找事件
	// ctx: 上下文
	// aggregateID: 聚合根 ID
	// tenantID: 租户 ID（可选）
	FindByAggregateID(ctx context.Context, aggregateID string, tenantID int) ([]*OutboxEvent, error)

	// Update 更新事件
	// ctx: 上下文
	// event: 要更新的事件
	Update(ctx context.Context, event *OutboxEvent) error

	// MarkAsPublished 标记事件为已发布
	// ctx: 上下文
	// id: 事件 ID
	MarkAsPublished(ctx context.Context, id string) error

	// MarkAsFailed 标记事件为失败
	// ctx: 上下文
	// id: 事件 ID
	// err: 错误信息
	MarkAsFailed(ctx context.Context, id string, err error) error

	// IncrementRetry 增加重试次数（同时更新错误信息和最后重试时间）
	// ctx: 上下文
	// id: 事件 ID
	// errorMsg: 错误信息
	IncrementRetry(ctx context.Context, id string, errorMsg string) error

	// MarkAsMaxRetry 标记事件为超过最大重试次数
	// ctx: 上下文
	// id: 事件 ID
	// errorMsg: 错误信息
	MarkAsMaxRetry(ctx context.Context, id string, errorMsg string) error

	// IncrementRetryCount 增加重试次数（已废弃，使用 IncrementRetry 代替）
	// ctx: 上下文
	// id: 事件 ID
	// Deprecated: 使用 IncrementRetry 代替
	IncrementRetryCount(ctx context.Context, id string) error

	// Delete 删除事件
	// ctx: 上下文
	// id: 事件 ID
	Delete(ctx context.Context, id string) error

	// DeleteBatch 批量删除事件
	// ctx: 上下文
	// ids: 事件 ID 列表
	DeleteBatch(ctx context.Context, ids []string) error

	// DeletePublishedBefore 删除指定时间之前已发布的事件
	// ctx: 上下文
	// before: 时间阈值
	// tenantID: 租户 ID（可选）
	DeletePublishedBefore(ctx context.Context, before time.Time, tenantID int) (int64, error)

	// DeleteFailedBefore 删除指定时间之前失败的事件
	// ctx: 上下文
	// before: 时间阈值
	// tenantID: 租户 ID（可选）
	DeleteFailedBefore(ctx context.Context, before time.Time, tenantID int) (int64, error)

	// Count 统计事件数量
	// ctx: 上下文
	// status: 事件状态（可选，空字符串表示所有状态）
	// tenantID: 租户 ID（可选）
	Count(ctx context.Context, status EventStatus, tenantID int) (int64, error)

	// CountByStatus 按状态统计事件数量
	// ctx: 上下文
	// tenantID: 租户 ID（可选）
	// 返回：map[EventStatus]int64
	CountByStatus(ctx context.Context, tenantID int) (map[EventStatus]int64, error)

	// FindByIdempotencyKey 根据幂等性键查找事件
	// ctx: 上下文
	// idempotencyKey: 幂等性键
	// 返回：事件对象（如果不存在返回 nil）
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*OutboxEvent, error)

	// ExistsByIdempotencyKey 检查幂等性键是否已存在
	// ctx: 上下文
	// idempotencyKey: 幂等性键
	// 返回：true 表示已存在，false 表示不存在
	ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error)

	// FindMaxRetryEvents 查找超过最大重试次数的事件
	// ctx: 上下文
	// limit: 最大返回数量
	// tenantID: 租户 ID（可选）
	// 返回：事件列表
	FindMaxRetryEvents(ctx context.Context, limit int, tenantID int) ([]*OutboxEvent, error)

	// BatchUpdate 批量更新事件（可选接口，用于性能优化）
	// ctx: 上下文
	// events: 要更新的事件列表
	// 返回：更新失败时返回错误
	BatchUpdate(ctx context.Context, events []*OutboxEvent) error
}

// RepositoryStats 仓储统计信息
type RepositoryStats struct {
	// PendingCount 待发布事件数量
	PendingCount int64

	// PublishedCount 已发布事件数量
	PublishedCount int64

	// FailedCount 失败事件数量
	FailedCount int64

	// TotalCount 总事件数量
	TotalCount int64

	// OldestPendingAge 最老的待发布事件年龄
	OldestPendingAge time.Duration

	// TenantID 租户 ID
	TenantID int
}

// GetStats 获取仓储统计信息（可选方法）
// 实现此接口的仓储可以提供统计信息
type RepositoryStatsProvider interface {
	// GetStats 获取统计信息
	// ctx: 上下文
	// tenantID: 租户 ID（可选）
	GetStats(ctx context.Context, tenantID int) (*RepositoryStats, error)
}

// TransactionalRepository 支持事务的仓储接口（可选）
// 如果数据库支持事务，可以实现此接口
type TransactionalRepository interface {
	OutboxRepository

	// SaveInTx 在事务中保存事件
	// ctx: 上下文
	// tx: 事务对象（具体类型由实现决定，例如 *gorm.DB）
	// event: 要保存的事件
	SaveInTx(ctx context.Context, tx interface{}, event *OutboxEvent) error

	// BeginTx 开始事务
	// ctx: 上下文
	// 返回：事务对象
	BeginTx(ctx context.Context) (interface{}, error)

	// CommitTx 提交事务
	// tx: 事务对象
	CommitTx(tx interface{}) error

	// RollbackTx 回滚事务
	// tx: 事务对象
	RollbackTx(tx interface{}) error
}
