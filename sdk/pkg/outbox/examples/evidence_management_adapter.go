package examples

import (
	"context"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// 这是一个示例适配器，展示如何在 evidence-management 项目中使用 jxt-core 的 Outbox

// TransactionInterface 代表 evidence-management 的事务接口
// 实际使用时应该导入：jxt-evidence-system/evidence-management/shared/common/transaction
type TransactionInterface interface {
	Commit() error
	Rollback() error
}

// GetGormTxFunc 从事务中提取 *gorm.DB 的函数类型
// 实际使用时应该使用：persistence.GetTx(tx)
type GetGormTxFunc func(tx TransactionInterface) interface{}

// OutboxRepositoryAdapter 适配器：将 evidence-management 的事务接口适配到 jxt-core
//
// 用途：
//   - 在 evidence-management 项目中使用 jxt-core 的 Outbox 组件
//   - 自动处理事务类型转换
//   - 保持与现有事务管理器的兼容性
//
// 使用示例：
//
//	// 1. 创建 jxt-core 的 Outbox 仓储
//	jxtRepo := gormadapter.NewGormOutboxRepository(db)
//
//	// 2. 创建适配器
//	adapter := NewOutboxRepositoryAdapter(jxtRepo, persistence.GetTx)
//
//	// 3. 在事务中使用
//	transaction.RunInTransaction(ctx, txManager, func(tx transaction.Transaction) error {
//	    // 保存聚合
//	    repo.CreateInTx(ctx, tx, aggregate)
//
//	    // 保存 Outbox 事件（适配器自动处理事务转换）
//	    for _, event := range aggregate.Events() {
//	        outboxEvent := convertToJxtCoreOutboxEvent(event)
//	        adapter.SaveInTx(ctx, tx, outboxEvent)  // ⭐ 传入 transaction.Transaction
//	    }
//	    return nil
//	})
type OutboxRepositoryAdapter struct {
	repo      outbox.OutboxRepository
	getTxFunc GetGormTxFunc
}

// NewOutboxRepositoryAdapter 创建适配器
//
// 参数：
//   - repo: jxt-core 的 Outbox 仓储实现
//   - getTxFunc: 从 transaction.Transaction 中提取 *gorm.DB 的函数
//
// 返回：
//   - *OutboxRepositoryAdapter: 适配器实例
//
// 示例：
//
//	import (
//	    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
//	    "jxt-evidence-system/evidence-management/command/internal/infrastructure/persistence/gorm"
//	)
//
//	jxtRepo := gormadapter.NewGormOutboxRepository(db)
//	adapter := NewOutboxRepositoryAdapter(jxtRepo, persistence.GetTx)
func NewOutboxRepositoryAdapter(repo outbox.OutboxRepository, getTxFunc GetGormTxFunc) *OutboxRepositoryAdapter {
	return &OutboxRepositoryAdapter{
		repo:      repo,
		getTxFunc: getTxFunc,
	}
}

// SaveInTx 在事务中保存事件（适配方法）
//
// 参数：
//   - ctx: 上下文
//   - tx: evidence-management 的 transaction.Transaction 接口
//   - event: jxt-core 的 OutboxEvent
//
// 返回：
//   - error: 保存失败时返回错误
//
// 工作流程：
//  1. 使用 getTxFunc 从 transaction.Transaction 中提取 *gorm.DB
//  2. 调用 jxt-core 仓储的 SaveInTx 方法（传入 *gorm.DB）
//  3. 返回结果
//
// 示例：
//
//	err := adapter.SaveInTx(ctx, tx, outboxEvent)
func (a *OutboxRepositoryAdapter) SaveInTx(ctx context.Context, tx TransactionInterface, event *outbox.OutboxEvent) error {
	// 提取底层 GORM 事务
	gormTx := a.getTxFunc(tx)
	if gormTx == nil {
		return fmt.Errorf("无效的事务对象")
	}

	// 调用 jxt-core 的 SaveInTx（传入 *gorm.DB）
	if transactionalRepo, ok := a.repo.(outbox.TransactionalRepository); ok {
		return transactionalRepo.SaveInTx(ctx, gormTx, event)
	}

	return fmt.Errorf("repository does not support transactions")
}

// Save 保存事件（非事务）
func (a *OutboxRepositoryAdapter) Save(ctx context.Context, event *outbox.OutboxEvent) error {
	return a.repo.Save(ctx, event)
}

// SaveBatch 批量保存事件（非事务）
func (a *OutboxRepositoryAdapter) SaveBatch(ctx context.Context, events []*outbox.OutboxEvent) error {
	return a.repo.SaveBatch(ctx, events)
}

// FindPendingEvents 查找待发布的事件
func (a *OutboxRepositoryAdapter) FindPendingEvents(ctx context.Context, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	return a.repo.FindPendingEvents(ctx, limit, tenantID)
}

// FindByID 根据 ID 查找事件
func (a *OutboxRepositoryAdapter) FindByID(ctx context.Context, id string) (*outbox.OutboxEvent, error) {
	return a.repo.FindByID(ctx, id)
}

// Update 更新事件
func (a *OutboxRepositoryAdapter) Update(ctx context.Context, event *outbox.OutboxEvent) error {
	return a.repo.Update(ctx, event)
}

// Delete 删除事件
func (a *OutboxRepositoryAdapter) Delete(ctx context.Context, id string) error {
	return a.repo.Delete(ctx, id)
}

// DeleteBatch 批量删除事件
func (a *OutboxRepositoryAdapter) DeleteBatch(ctx context.Context, ids []string) error {
	return a.repo.DeleteBatch(ctx, ids)
}

// FindByAggregateID 根据聚合 ID 查找事件
func (a *OutboxRepositoryAdapter) FindByAggregateID(ctx context.Context, aggregateID string, tenantID string) ([]*outbox.OutboxEvent, error) {
	return a.repo.FindByAggregateID(ctx, aggregateID, tenantID)
}

// FindByStatus 根据状态查找事件
func (a *OutboxRepositoryAdapter) FindByStatus(ctx context.Context, status outbox.EventStatus, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	return a.repo.FindByStatus(ctx, status, limit, tenantID)
}

// IncrementRetry 增加重试次数
func (a *OutboxRepositoryAdapter) IncrementRetry(ctx context.Context, eventID string, errorMsg string) error {
	return a.repo.IncrementRetry(ctx, eventID, errorMsg)
}

// MarkAsMaxRetry 标记为超过最大重试次数
func (a *OutboxRepositoryAdapter) MarkAsMaxRetry(ctx context.Context, eventID string, errorMsg string) error {
	return a.repo.MarkAsMaxRetry(ctx, eventID, errorMsg)
}

// FindPendingEventsWithDelay 查找待发布的事件（带延迟）
func (a *OutboxRepositoryAdapter) FindPendingEventsWithDelay(ctx context.Context, limit int, tenantID string, delaySeconds int) ([]*outbox.OutboxEvent, error) {
	return a.repo.FindPendingEventsWithDelay(ctx, limit, tenantID, delaySeconds)
}

// FindEventsForRetry 查找需要重试的事件
func (a *OutboxRepositoryAdapter) FindEventsForRetry(ctx context.Context, maxRetries int, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	return a.repo.FindEventsForRetry(ctx, maxRetries, limit, tenantID)
}

// FindByAggregateType 根据聚合类型查找事件
func (a *OutboxRepositoryAdapter) FindByAggregateType(ctx context.Context, aggregateType string, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	return a.repo.FindByAggregateType(ctx, aggregateType, limit, tenantID)
}

// BatchUpdate 批量更新事件（性能优化）
func (a *OutboxRepositoryAdapter) BatchUpdate(ctx context.Context, events []*outbox.OutboxEvent) error {
	// 降级到逐个更新
	for _, event := range events {
		if err := a.Update(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Count 统计事件数量
func (a *OutboxRepositoryAdapter) Count(ctx context.Context, status outbox.EventStatus, tenantID string) (int64, error) {
	return a.repo.Count(ctx, status, tenantID)
}

// CountByStatus 按状态统计事件数量
func (a *OutboxRepositoryAdapter) CountByStatus(ctx context.Context, tenantID string) (map[outbox.EventStatus]int64, error) {
	return a.repo.CountByStatus(ctx, tenantID)
}

// FindByIdempotencyKey 根据幂等性键查找事件
func (a *OutboxRepositoryAdapter) FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*outbox.OutboxEvent, error) {
	return a.repo.FindByIdempotencyKey(ctx, idempotencyKey)
}

// ExistsByIdempotencyKey 检查幂等性键是否已存在
func (a *OutboxRepositoryAdapter) ExistsByIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error) {
	return a.repo.ExistsByIdempotencyKey(ctx, idempotencyKey)
}

// FindMaxRetryEvents 查找超过最大重试次数的事件
func (a *OutboxRepositoryAdapter) FindMaxRetryEvents(ctx context.Context, limit int, tenantID string) ([]*outbox.OutboxEvent, error) {
	return a.repo.FindMaxRetryEvents(ctx, limit, tenantID)
}

// DeletePublishedBefore 删除指定时间之前已发布的事件
func (a *OutboxRepositoryAdapter) DeletePublishedBefore(ctx context.Context, before time.Time, tenantID string) (int64, error) {
	return a.repo.DeletePublishedBefore(ctx, before, tenantID)
}

// DeleteFailedBefore 删除指定时间之前失败的事件
func (a *OutboxRepositoryAdapter) DeleteFailedBefore(ctx context.Context, before time.Time, tenantID string) (int64, error) {
	return a.repo.DeleteFailedBefore(ctx, before, tenantID)
}

// Ensure OutboxRepositoryAdapter implements OutboxRepository
var _ outbox.OutboxRepository = (*OutboxRepositoryAdapter)(nil)

// -------------------------------------------------------------------
// 完整使用示例
// -------------------------------------------------------------------

/*
package service

import (
    "context"
    "encoding/json"
    "fmt"

    "jxt-evidence-system/evidence-management/shared/common/transaction"
    "jxt-evidence-system/evidence-management/shared/domain/event"
    persistence "jxt-evidence-system/evidence-management/command/internal/infrastructure/persistence/gorm"

    jxtoutbox "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/examples"
)

type ArchiveService struct {
    txManager         transaction.TransactionManager
    repo              ArchiveRepository
    jxtOutboxAdapter  *examples.OutboxRepositoryAdapter  // 使用适配器
}

func NewArchiveService(
    txManager transaction.TransactionManager,
    repo ArchiveRepository,
    db *gorm.DB,
) *ArchiveService {
    // 创建 jxt-core 的 Outbox 仓储
    jxtRepo := gormadapter.NewGormOutboxRepository(db)

    // 创建适配器
    adapter := examples.NewOutboxRepositoryAdapter(jxtRepo, func(tx examples.TransactionInterface) interface{} {
        // 类型断言转换
        if gormTx, ok := tx.(transaction.Transaction); ok {
            return persistence.GetTx(gormTx)
        }
        return nil
    })

    return &ArchiveService{
        txManager:        txManager,
        repo:             repo,
        jxtOutboxAdapter: adapter,
    }
}

func (s *ArchiveService) CreateArchive(ctx context.Context, cmd *CreateArchiveCommand) error {
    // 1. 创建聚合根
    archive := NewArchive(cmd)

    // 2. 执行业务逻辑（生成领域事件）
    if err := archive.Create(); err != nil {
        return err
    }

    // 3. 在事务中持久化
    return transaction.RunInTransaction(ctx, s.txManager, func(tx transaction.Transaction) error {
        // 3.1 保存聚合根
        if err := s.repo.CreateInTx(ctx, tx, archive); err != nil {
            return fmt.Errorf("保存聚合失败: %w", err)
        }

        // 3.2 保存 Outbox 事件（使用适配器）
        for _, domainEvent := range archive.Events() {
            // 转换为 jxt-core 的 OutboxEvent
            outboxEvent, err := convertToJxtCoreOutboxEvent(domainEvent)
            if err != nil {
                return fmt.Errorf("转换事件失败: %w", err)
            }

            // 设置追踪信息
            traceID := extractTraceIDFromContext(ctx)
            correlationID := extractCorrelationIDFromContext(ctx)
            outboxEvent.WithTraceID(traceID).WithCorrelationID(correlationID)

            // 在事务中保存 Outbox 事件（适配器自动处理事务转换）⭐
            if err := s.jxtOutboxAdapter.SaveInTx(ctx, tx, outboxEvent); err != nil {
                return fmt.Errorf("保存 Outbox 事件失败: %w", err)
            }
        }

        // 3.3 清空聚合根中的事件
        archive.ClearEvents()

        return nil
    })
}

// convertToJxtCoreOutboxEvent 转换领域事件为 jxt-core OutboxEvent
func convertToJxtCoreOutboxEvent(domainEvent event.Event) (*jxtoutbox.OutboxEvent, error) {
    // 序列化 Payload
    payload, err := json.Marshal(domainEvent.GetPayload())
    if err != nil {
        return nil, err
    }

    // 创建 OutboxEvent
    return jxtoutbox.NewOutboxEvent(
        domainEvent.GetTenantId(),
        domainEvent.GetAggregateId(),
        domainEvent.GetAggregateType(),
        domainEvent.GetEventType(),
        payload,
    )
}

// extractTraceIDFromContext 从上下文中提取 TraceID
func extractTraceIDFromContext(ctx context.Context) string {
    if traceID, ok := ctx.Value("trace_id").(string); ok {
        return traceID
    }
    return ""
}

// extractCorrelationIDFromContext 从上下文中提取 CorrelationID
func extractCorrelationIDFromContext(ctx context.Context) string {
    if correlationID, ok := ctx.Value("correlation_id").(string); ok {
        return correlationID
    }
    return ""
}
*/
