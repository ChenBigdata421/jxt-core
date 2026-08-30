package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/jackc/pgconn"
	v5pgconn "github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func mkPg(code string) error { return &pgconn.PgError{Code: code} }

// mkPgV5 构造 pgx/v5 的 *pgconn.PgError——gorm.io/driver/postgres ≥ v1.5.x 底层是 pgx/v5，
// 其产出的错误是 v5 包类型（与 v4 同名异包）。mkPgV5Wrapped 模拟真实链路：driver 错误
// 总是被 gorm/服务层包一层（%w），errors.As 必须能穿透。
// （gorm.ErrDuplicatedKey 哨兵臂不在本测试：kernel pin gorm v1.24.2 无此哨兵——v1.25.0+
// 才有，且仅当 driver 开 TranslateError 才产出；fs 生产 gorm.Open 未开，错误保持 v5 原生形态。）
func mkPgV5(code string) error        { return &v5pgconn.PgError{Code: code} }
func mkPgV5Wrapped(code string) error { return fmt.Errorf("insert row: %w", &v5pgconn.PgError{Code: code}) }

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

// TestPGIsDuplicateKey_PgxV5 （v1.7.9 修复锚）：gorm.io/driver/postgres ≥ v1.5.x 底层是
// pgx/v5，产出的唯一键冲突是 v5 的 *pgconn.PgError（与 kernel 断言的 v4 同名异包）——
// v1.7.8 及之前只断言 v4 类型，v5 错误恒 false，claim.go 的 dup 检测在 fs（driver v1.5.4）
// 上失明：重复投递撞唯一键被当普通错误上抛 → 分区不 ACK、broker 重投自愈（数据安全，
// 但多一次往返 + 异常类别误标）。另覆盖 gorm.ErrDuplicatedKey 哨兵（driver 开
// TranslateError 时的形态）。实证：evidence（driver v1.4.5 → pgx/v4）不受影响。
func TestPGIsDuplicateKey_PgxV5(t *testing.T) {
	c := PGClassifier{}
	assert.True(t, c.IsDuplicateKey(mkPgV5("23505")), "pgx/v5 PgError 23505 must be recognized")
	assert.True(t, c.IsDuplicateKey(mkPgV5Wrapped("23505")), "wrapped pgx/v5 PgError must be recognized via errors.As")
	assert.False(t, c.IsDuplicateKey(mkPgV5("40P01")))
	assert.False(t, c.IsDuplicateKey(errors.New("duplicate key value violates unique constraint")), "string match must NOT count")
}

// TestPGClassifierAndErrorCode_PgxV5：ClassifyDriver/ErrorCode 与 IsDuplicateKey 同源
// （同一 typed 断言），v5 错误此前同样失明——23505 会掉出 ClassConflict 分支、error_code
// 列拿不到 SQLSTATE。v1.7.9 一并修。
func TestPGClassifierAndErrorCode_PgxV5(t *testing.T) {
	clf := PGClassifier{}
	got, ok := clf.ClassifyDriver(mkPgV5Wrapped("23505"))
	assert.True(t, ok, "pgx/v5 23505 must classify")
	assert.Equal(t, reliable.ClassConflict, got)
	code, ok := clf.ErrorCode(mkPgV5("40P01"))
	assert.True(t, ok)
	assert.Equal(t, "40P01", code)
}

func TestPGClassifierPlugsIntoKernel(t *testing.T) {
	var _ reliable.ErrorClassifier = PGClassifier{}
	assert.Equal(t, reliable.ClassRetryable, reliable.Classify(mkPg("40P01"), PGClassifier{}))
	assert.Equal(t, reliable.ClassPoison, reliable.Classify(reliable.Permanent(mkPg("23503")), PGClassifier{}))
	assert.Equal(t, reliable.ClassConflict, reliable.Classify(mkPg("23505"), PGClassifier{}))
}
