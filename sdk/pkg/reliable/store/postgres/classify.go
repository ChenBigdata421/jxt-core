package postgres

import (
	"errors"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/gormshared"
	"github.com/jackc/pgconn"
	"gorm.io/gorm"
)

// PGClassifier 实现 reliable.ErrorClassifier（§5 第 2 级，PostgreSQL SQLSTATE）。
type PGClassifier struct{}

func (PGClassifier) ClassifyDriver(err error) (reliable.ErrorClass, bool) {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return "", false
	}
	switch pe.Code {
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
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// ErrorCode 提取 PostgreSQL SQLSTATE 码（如 "23505"、"40P01"），用于填充 error_code 列。
// B1（本轮评审）：原稿此处注释行首多一个 `n`、函数体多一层缩进，且文件缺 `store` import（下方
// NewStore 返回 store.Store）——与 mysql/classify.go 同款错误，两处一并修。
func (PGClassifier) ErrorCode(err error) (string, bool) {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code, true
	}
	return "", false
}

// NewStore 注入 PGClassifier 到共享 gormStore。db 必须是 pooled（§3.3，D16）。
func NewStore(db *gorm.DB) store.Store                     { return gormshared.NewStore(db, PGClassifier{}) }
func NewQuarantineStore(db *gorm.DB) store.QuarantineStore { return gormshared.NewQuarantineStore(db) }
