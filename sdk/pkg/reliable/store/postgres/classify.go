package postgres

import (
	"errors"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/gormshared"
	"github.com/jackc/pgconn"
	v5pgconn "github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// PGClassifier 实现 reliable.ErrorClassifier（§5 第 2 级，PostgreSQL SQLSTATE）。
type PGClassifier struct{}

// pgErrorCode 提取 SQLSTATE（v1.7.9）：同时断言 pgx/v4 与 pgx/v5 两代 *pgconn.PgError。
// 为什么有两代：gorm.io/driver/postgres ≤ v1.4.x 底层是 pgx/v4，≥ v1.5.x 是 pgx/v5——
// 两代的 PgError 同名异包（github.com/jackc/pgconn vs github.com/jackc/pgx/v5/pgconn），
// typed 断言跨包不命中。v1.7.8 及之前只断言 v4：driver ≥ v1.5 的使用方（file-storage，
// v1.5.4）拿到的唯一键/死锁/语法错误在 ClassifyDriver/IsDuplicateKey/ErrorCode 三处全部
// 失明——23505 不再进 ClassConflict、dup 检测（claim.go D3）恒 false。后果实证：数据
// 安全（fail-closed + broker 重投自愈），但多一次投递往返 + 异常类别误标。evidence
// （driver v1.4.5 → pgx/v4）不受影响。v5 走 build tag 之外的双断言而非类型别名——两包
// 结构体字段兼容（Code/Message/Detail...），各断各的即可；包都已在模块图内（v4 direct、
// v5 因 driver≥1.5 的使用方提升为 direct require）。
func pgErrorCode(err error) (string, bool) {
	var v4 *pgconn.PgError
	if errors.As(err, &v4) {
		return v4.Code, true
	}
	var v5 *v5pgconn.PgError
	if errors.As(err, &v5) {
		return v5.Code, true
	}
	return "", false
}

func (PGClassifier) ClassifyDriver(err error) (reliable.ErrorClass, bool) {
	code, ok := pgErrorCode(err)
	if !ok {
		return "", false
	}
	switch code {
	case "23505":
		return reliable.ClassConflict, true
	case "23503":
		return reliable.ClassRetryable, true
	case "40P01", "40001", "55P03":
		return reliable.ClassRetryable, true
	case "42P01", "42703":
		return reliable.ClassUnrecoverable, true
	}
	return "", false
}

// IsDuplicateKey 报告 err 是否是 PG unique_violation（23505）。D3：typed errors.As。
func (PGClassifier) IsDuplicateKey(err error) bool {
	code, ok := pgErrorCode(err)
	return ok && code == "23505"
}

// ErrorCode 提取 PostgreSQL SQLSTATE 码（如 "23505"、"40P01"），用于填充 error_code 列。
// B1（本轮评审）：原稿此处注释行首多一个 `n`、函数体多一层缩进，且文件缺 `store` import（下方
// NewStore 返回 store.Store）——与 mysql/classify.go 同款错误，两处一并修。
func (PGClassifier) ErrorCode(err error) (string, bool) {
	return pgErrorCode(err)
}

// NewStore 注入 PGClassifier 到共享 gormStore。db 必须是 pooled（§3.3，D16）。
func NewStore(db *gorm.DB) store.Store                     { return gormshared.NewStore(db, PGClassifier{}) }
func NewQuarantineStore(db *gorm.DB) store.QuarantineStore { return gormshared.NewQuarantineStore(db) }
