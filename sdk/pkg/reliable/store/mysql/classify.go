package mysql

import (
	"errors"
	"strconv"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/gormshared"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// MySQLClassifier 实现 reliable.ErrorClassifier（§5 第 2 级，MySQL 错误码）。
type MySQLClassifier struct{}

// ClassifyDriver 按 §5 错误码映射返回 (class, true)；不认识返回 ("", false) 落 kernel 兜底。
func (MySQLClassifier) ClassifyDriver(err error) (reliable.ErrorClass, bool) {
	var me *mysqldriver.MySQLError
	if !errors.As(err, &me) {
		return "", false
	}
	switch me.Number {
	case 1062:
		return reliable.ClassConflict, true
	case 1213, 1205, 1040, 1053:
		return reliable.ClassRetryable, true
	case 1452:
		return reliable.ClassRetryable, true
	case 1146, 1054, 1170:
		return reliable.ClassUnrecoverable, true
	}
	return "", false
}

// IsDuplicateKey 报告 err 是否是 MySQL 唯一冲突（1062）。D3：typed errors.As，不字符串匹配。
// 供 gormshared.TryClaim 的 Create 竞态重试用。
func (MySQLClassifier) IsDuplicateKey(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// ErrorCode 提取 MySQL 错误码数字（如 "1213"、"1062"），用于填充 error_code 列。
func (MySQLClassifier) ErrorCode(err error) (string, bool) {
	var me *mysqldriver.MySQLError
	if errors.As(err, &me) {
		return strconv.Itoa(int(me.Number)), true
	}
	return "", false
}

// NewStore 注入 MySQLClassifier 到共享 gormStore。db 必须是 pooled（非事务）句柄（§3.3，D16）。
func NewStore(db *gorm.DB) store.Store { return gormshared.NewStore(db, MySQLClassifier{}) }

// NewQuarantineStore 同理。
func NewQuarantineStore(db *gorm.DB) store.QuarantineStore {
	return gormshared.NewQuarantineStore(db)
}
