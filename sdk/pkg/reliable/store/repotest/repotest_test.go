package repotest

import (
	"testing"
)

// TestConformance_AllDialects 是 PR-2 金标测试入口（准入 ⑭）。
// 种子/读全行 helper（seedRetryRow / mustGetFullRow）放 helpers.go（非 _test 文件），
// 因为 conformance.go（非测试）调用它们——若放在 _test 文件，go build 会因非测试代码引用测试符号失败。
func TestConformance_AllDialects(t *testing.T) {
	for _, dialect := range []Dialect{DialectMySQL, DialectPostgres} {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			db, cleanup := Setup(t, dialect)
			defer cleanup()
			st, qs := NewStoreFor(dialect, db)
			d := &ConformanceDeps{DB: db, Store: st, QStore: qs, Dialect: dialect}
			RunConformance(t, d)
			RunInvariant(t, d)
			RunQuarantineConformance(t, d)
			RunErrorPropagationConformance(t, d)
		})
	}
}
