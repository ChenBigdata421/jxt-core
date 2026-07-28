package postgres

import (
	"errors"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/jackc/pgconn"
	"github.com/stretchr/testify/assert"
)

func mkPg(code string) error { return &pgconn.PgError{Code: code} }

// B2（本轮评审）：与 mysql 同款——原稿用 `c` 同时命名 classifier 与 range 变量，内层 `c` 是
// 结构体字面量、没有 ClassifyDriver 方法 → 编译不过。classifier 改名 clf，range 变量改名 tc。
func TestPGClassifierMapping(t *testing.T) {
	clf := PGClassifier{}
	cases := []struct {
		code  string
		class reliable.ErrorClass
	}{
		{"23505", reliable.ClassConflict}, {"23503", reliable.ClassRetryable},
		{"40P01", reliable.ClassRetryable}, {"40001", reliable.ClassRetryable},
		{"55P03", reliable.ClassRetryable}, {"42P01", reliable.ClassUnrecoverable},
		{"42703", reliable.ClassUnrecoverable},
	}
	for _, tc := range cases {
		got, ok := clf.ClassifyDriver(mkPg(tc.code))
		assert.True(t, ok, "SQLSTATE %s", tc.code)
		assert.Equal(t, tc.class, got, "SQLSTATE %s", tc.code)
	}
}

func TestPGClassifierUnknownAndNonDriver(t *testing.T) {
	clf := PGClassifier{}
	_, ok := clf.ClassifyDriver(mkPg("99999"))
	assert.False(t, ok)
	_, ok = clf.ClassifyDriver(errors.New("not pg"))
	assert.False(t, ok)
}

// ErrorCode 必须返回 SQLSTATE 字符串（填 error_code 列）。
func TestPGErrorCode(t *testing.T) {
	clf := PGClassifier{}
	code, ok := clf.ErrorCode(mkPg("40P01"))
	assert.True(t, ok)
	assert.Equal(t, "40P01", code)
	_, ok = clf.ErrorCode(errors.New("not pg"))
	assert.False(t, ok)
}

func TestPGIsDuplicateKey(t *testing.T) {
	c := PGClassifier{}
	assert.True(t, c.IsDuplicateKey(mkPg("23505")))
	assert.False(t, c.IsDuplicateKey(mkPg("40P01")))
	assert.False(t, c.IsDuplicateKey(errors.New("duplicate key value...")), "string match must NOT count")
}

func TestPGClassifierPlugsIntoKernel(t *testing.T) {
	var _ reliable.ErrorClassifier = PGClassifier{}
	assert.Equal(t, reliable.ClassRetryable, reliable.Classify(mkPg("40P01"), PGClassifier{}))
	assert.Equal(t, reliable.ClassPoison, reliable.Classify(reliable.Permanent(mkPg("23503")), PGClassifier{}))
	assert.Equal(t, reliable.ClassConflict, reliable.Classify(mkPg("23505"), PGClassifier{}))
}
