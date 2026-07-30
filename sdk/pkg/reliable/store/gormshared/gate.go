package gormshared

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
	// 与 ReleaseAggregateGate 对称：必须捕获 *gorm.DB.Error。原稿链式取 .RowsAffected 丢弃了
	// *gorm.DB，DELETE 失败（连接断/锁超时）时返回 (0, nil)——过期的 gate 行静默堆积，最终
	// 楔住相关聚合的重放（AcquireAggregateGate 看到长期“活跃”的死亡持有者）却无任何信号。
	res := s.markDB.WithContext(ctx).Where("expires_at < ?", now).Delete(&AggregateLeaseModel{})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}
