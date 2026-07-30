package reliable_test

import (
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

// TestGate_RootKernelZeroDeps 守护 J2：kernel 根包不引 gorm/driver/prometheus/gin/sarama。
func TestGate_RootKernelZeroDeps(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./sdk/pkg/reliable")
	cmd.Dir = repoRoot(t) // 关键：显式指定仓库根，不靠 cwd
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go list failed: %s", out)
	for _, banned := range []string{"gorm.io", "github.com/prometheus", "github.com/gin-gonic", "github.com/IBM/sarama"} {
		require.NotContainsf(t, string(out), banned, "J2 violation: kernel imports %s", banned)
	}
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
