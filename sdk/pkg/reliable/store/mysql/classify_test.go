package mysql

import (
	"errors"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
)

func mkErr(num uint16) error { return &mysqldriver.MySQLError{Number: num} }

// B2（本轮评审）：原稿用 `c` 同时命名 classifier 与 range 变量，内层 `c` 是结构体字面量、
// 没有 ClassifyDriver 方法 → 编译不过。classifier 改名 clf，range 变量改名 tc。
func TestMySQLClassifierMapping(t *testing.T) {
	clf := MySQLClassifier{}
	cases := []struct {
		num   uint16
		class reliable.ErrorClass
	}{
		{1062, reliable.ClassConflict}, {1213, reliable.ClassRetryable}, {1205, reliable.ClassRetryable},
		{1040, reliable.ClassRetryable}, {1053, reliable.ClassRetryable}, {1452, reliable.ClassRetryable},
		{1146, reliable.ClassUnrecoverable}, {1054, reliable.ClassUnrecoverable}, {1170, reliable.ClassUnrecoverable},
	}
	for _, tc := range cases {
		got, ok := clf.ClassifyDriver(mkErr(tc.num))
		assert.True(t, ok, "err %d", tc.num)
		assert.Equal(t, tc.class, got, "err %d", tc.num)
	}
}

func TestMySQLClassifierUnknownAndNonDriver(t *testing.T) {
	clf := MySQLClassifier{}
	_, ok := clf.ClassifyDriver(mkErr(9999))
	assert.False(t, ok)
	_, ok = clf.ClassifyDriver(errors.New("not mysql"))
	assert.False(t, ok)
}

// ErrorCode 必须返回驱动码数字串（填 error_code 列）。
func TestMySQLErrorCode(t *testing.T) {
	clf := MySQLClassifier{}
	code, ok := clf.ErrorCode(mkErr(1213))
	assert.True(t, ok)
	assert.Equal(t, "1213", code)
	_, ok = clf.ErrorCode(errors.New("not mysql"))
	assert.False(t, ok)
}

// D3：IsDuplicateKey 用 typed errors.As，不字符串匹配。
func TestMySQLIsDuplicateKey(t *testing.T) {
	c := MySQLClassifier{}
	assert.True(t, c.IsDuplicateKey(mkErr(1062)))
	assert.False(t, c.IsDuplicateKey(mkErr(1213)), "deadlock is not dup")
	assert.False(t, c.IsDuplicateKey(errors.New("Duplicate entry ...")), "string match must NOT count")
}

func TestMySQLClassifierPlugsIntoKernel(t *testing.T) {
	var _ reliable.ErrorClassifier = MySQLClassifier{}
	assert.Equal(t, reliable.ClassRetryable, reliable.Classify(mkErr(1213), MySQLClassifier{}))
	assert.Equal(t, reliable.ClassPoison, reliable.Classify(reliable.Permanent(mkErr(1452)), MySQLClassifier{}))
	assert.Equal(t, reliable.ClassConflict, reliable.Classify(mkErr(1062), MySQLClassifier{}))
}
