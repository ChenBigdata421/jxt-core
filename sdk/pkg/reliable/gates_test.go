package reliable_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// repoRoot 从本测试文件位置回溯到仓库根（.../jxt-core）。
// 不依赖 cwd：go test 的 cwd 是包目录，任何相对仓库根的路径都会失效。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// file = <root>/sdk/pkg/reliable/gates_test.go
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "repoRoot must contain go.mod, got %s", root)
	return root
}

// scanReliable 遍历 sdk/pkg/reliable/** 的 .go 文件，返回命中 re 的 "path:line: text"。
// gates_test.go 自身被排除：它承载门禁的词表（pattern 字面量 + SelfCheck 注入样本），
// 把它纳入扫描会让门禁永远自报红——这是「门禁抓门禁自己」的经典反模式。
func scanReliable(t *testing.T, re *regexp.Regexp) []string {
	t.Helper()
	root := filepath.Join(repoRoot(t), "sdk", "pkg", "reliable")
	var hits []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "gates_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(b), "\n") {
			if re.MatchString(line) {
				rel, _ := filepath.Rel(root, path)
				hits = append(hits, rel+":"+itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	}))
	return hits
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// goListDeps 返回 `go list -deps <pkg>` 的输出（该包的传递依赖闭包，含自身）。
// 工具链约定：go list 失败即 FailNow 而非 Skip——工具链坏了不等于「依赖干净」，
// 静默跳过会让门禁在坏环境（缺 Go、GOFLAGS 异常）下静默变绿，形同虚设。
func goListDeps(t *testing.T, pkg string) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Dir = repoRoot(t) // 关键：显式指定仓库根，不靠 cwd
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go list %s failed: %s", pkg, out)
	return string(out)
}

// TestGate_RootKernelZeroDeps 守护 J2：kernel 根包不引 gorm/driver/prometheus/gin/sarama。
func TestGate_RootKernelZeroDeps(t *testing.T) {
	out := goListDeps(t, "./sdk/pkg/reliable")
	for _, banned := range []string{"gorm.io", "github.com/prometheus", "github.com/gin-gonic", "github.com/IBM/sarama"} {
		require.NotContainsf(t, out, banned, "J2 violation: kernel imports %s", banned)
	}
}

// TestGate_RootKernelBansNatsIO 守护 J2 的 nats-io 半边（评审 B11）：kernel 根包不引
// github.com/nats-io。补齐与 scripts/reliable_deps_gate.sh:11 的 Go 侧对等——bash 门禁
// 早已禁 nats-io，但 TestGate_RootKernelZeroDeps 的禁用表一直漏了它，导致没有 bash 的
// CI（Windows / 纯 Go runner）不强制这条。nats-io 属传输层：kernel 只认 Store 抽象，
// 不认任何 broker 客户端。
func TestGate_RootKernelBansNatsIO(t *testing.T) {
	for _, line := range strings.Split(goListDeps(t, "./sdk/pkg/reliable"), "\n") {
		if strings.Contains(line, "github.com/nats-io") {
			t.Fatalf("J2 violation: reliable kernel root pulls nats-io (via %s)", line)
		}
	}
}

// TestGate_ScopedSubPackageDeps 守护 J2 的 scoped 半边（评审 B11）：逐字镜像
// scripts/reliable_deps_gate.sh:15/19/23 的三条 sub-package 矩阵——
//   - gormshared 只许 gorm 本体（两种 driver 都禁，方言归 store/mysql|postgres）；
//   - store/mysql 只许 gorm + mysql driver（禁 postgres driver）；
//   - store/postgres 只许 gorm + postgres driver（禁 mysql driver）；
//   - 三者共禁 prometheus / gin / sarama。
// 之前这些矩阵只活在 bash 门禁里；CI 没有 bash 时无人强制。改脚本矩阵时同步改这里。
func TestGate_ScopedSubPackageDeps(t *testing.T) {
	scoped := []struct {
		pkg    string
		banned []string
	}{
		{"./sdk/pkg/reliable/store/gormshared", []string{
			"github.com/prometheus", "github.com/gin-gonic",
			"gorm.io/driver/mysql", "gorm.io/driver/postgres", "github.com/IBM/sarama"}},
		{"./sdk/pkg/reliable/store/mysql", []string{
			"github.com/prometheus", "github.com/gin-gonic",
			"gorm.io/driver/postgres", "github.com/IBM/sarama"}},
		{"./sdk/pkg/reliable/store/postgres", []string{
			"github.com/prometheus", "github.com/gin-gonic",
			"gorm.io/driver/mysql", "github.com/IBM/sarama"}},
	}
	for _, s := range scoped {
		for _, line := range strings.Split(goListDeps(t, s.pkg), "\n") {
			for _, b := range s.banned {
				if strings.Contains(line, b) {
					t.Fatalf("J2 violation: %s pulls %q (via %s)", s.pkg, b, line)
				}
			}
		}
	}
}

// TestGate_RootPackageNoCycleImports 守护 J2/cycle 的精确边：根 reliable 包的**自身**生产 .go 文件
// 不得直接 import sdk/pkg/reliable/store（store 依赖 reliable，会成环）也不得 import gorm.io/gorm（kernel 纯度）。
//
// 与 TestGate_RootKernelZeroDeps 互补：后者用 `go list -deps` 抓 transitive 依赖（广，但粒度粗、
// 慢、需 go 工具链）；本测试用 go/parser 只解析根包目录下的直接 import，精确锁定「会闭合 cycle 的那条边」。
// 这正是 Task-1 抽出 gate 子包要防的回归：一旦有人把 store 或 gorm 加回根包任一文件，
// reliable→store→reliable 的环即刻成型（go build 会拒编，但本测试给出更早、更精确的定位）。
func TestGate_RootPackageNoCycleImports(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "sdk", "pkg", "reliable")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "read reliable root dir")

	// 任何以 reliable/store 结尾的 import 路径都算违规（不硬编码 module 前缀，迁移 module 时仍生效）；
	// gorm.io/gorm 是 J2 明令禁止的 kernel 依赖。
	const bannedStoreSuffix = "sdk/pkg/reliable/store"
	const bannedGorm = "gorm.io/gorm"

	fset := token.NewFileSet()
	var hits []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			continue // 生产不变量：只守非测试文件（_test.go 不进生产 import 图）
		}
		path := filepath.Join(dir, e.Name())
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		require.NoError(t, perr, "parse %s", e.Name())
		for _, imp := range f.Imports {
			impPath := strings.Trim(imp.Path.Value, `"`)
			if impPath == bannedGorm || strings.HasSuffix(impPath, "/"+bannedStoreSuffix) || impPath == bannedStoreSuffix {
				hits = append(hits, e.Name()+": "+impPath)
			}
		}
	}
	require.Empty(t, hits,
		"J2/cycle violation: root reliable package must not import store (cycle) or gorm.io/gorm (purity):\n%s",
		strings.Join(hits, "\n"))
}

// TestGate_NoContextKey 守护 M14：reliable/** 不得 context.WithValue。
func TestGate_NoContextKey(t *testing.T) {
	hits := scanReliable(t, regexp.MustCompile(`context\.WithValue`))
	require.Empty(t, hits, "M14 violation: context.WithValue found:\n%s", strings.Join(hits, "\n"))
}

// placeholderRe 是 D9 无占位符门禁的模式。抽成变量以便下方自测直接喂违规样本。
var placeholderRe = regexp.MustCompile(`实施时|TBD|TODO|FIXME|DATEADD\(|fill in|= interface\{\}|stubDB`)

// TestGate_NoPlaceholders 守护 D9：reliable/** 不得有待补占位标记。
// D21：模式扩到能抓「实施时…」注记与 `= interface{}` / `stubDB` 这类假类型——
// 上一版计划自己留了 4 处这类占位，而旧模式一个都抓不到。
func TestGate_NoPlaceholders(t *testing.T) {
	hits := scanReliable(t, placeholderRe)
	require.Empty(t, hits, "D9 violation: placeholder found:\n%s", strings.Join(hits, "\n"))
}

// TestGate_TryClaimNoExternalTx 守护 §3.3：TryClaim 签名不含 *gorm.DB。
func TestGate_TryClaimNoExternalTx(t *testing.T) {
	hits := scanReliable(t, regexp.MustCompile(`TryClaim\(.*gorm\.DB`))
	require.Empty(t, hits, "§3.3 violation: TryClaim must not take *gorm.DB:\n%s", strings.Join(hits, "\n"))
}

// TestGate_SelfCheck 是门禁的门禁：注入违规样本，断言模式确实命中。
// 没有这条，一个写反了判定 / 路径失效的门禁会永远静默绿（原稿正是如此）。
func TestGate_SelfCheck(t *testing.T) {
	for _, sample := range []string{
		"type reliable_store = interface{} // 占位",
		"type stubDB = struct{}",
		"// 实施时改为真实类型",
		"// TODO: 补齐",
		"next_attempt_at = DATEADD(NOW(), ...)",
	} {
		require.True(t, placeholderRe.MatchString(sample), "placeholder gate must catch: %s", sample)
	}
	// 反向：正常代码不得误报。
	for _, ok := range []string{
		"func (s *GormStore) TryClaim(ctx context.Context, in reliable.ClaimInput, lease time.Duration)",
		"// D20：观测器只记 anomaly，不改行状态",
	} {
		require.False(t, placeholderRe.MatchString(ok), "placeholder gate false positive on: %s", ok)
	}
	// repoRoot 必须真的能定位到 go.mod（否则上面三个门禁全部形同虚设）。
	require.FileExists(t, filepath.Join(repoRoot(t), "go.mod"))
}
