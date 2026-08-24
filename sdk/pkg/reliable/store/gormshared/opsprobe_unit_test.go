package gormshared

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFrozenAggregatesSQL_LockstepWithEarlierUnsolvedSibling（review OV②：不许自证——
// 不把手抄的三段式写进断言，而是**运行时从 EarlierUnsolvedSiblingSQL 切出**三段 OR 臂，
// 方向翻转（< ↔ >，操作数随之换位）后断言逐臂镜像在 FrozenAggregatesSQL。任何一侧改
// earlier-than 谓词，本测试即红——R4-H「禁止服务侧手抄」在 kernel 内部的镜像约束：
// 内核侧这三份同源谓词（EligibleHeadsSQL / EarlierUnsolvedSiblingSQL / FrozenAggregatesSQL）
// 也不许各自漂移。
//
// 镜像按**语义**比较而非逐字节：两个常量的 AND 合取项顺序（e 先还是 c 先）与缩进不同，
// 但每一条列比较 `e.col < c.col` 必须对应 `c.col > e.col`，每一条 IS [NOT] NULL 谓词
// 必须成对出现。逐臂按「规范化比较集 + NULL 谓词集」相等断言——多一条、少一条、改一个
// 操作符或换一列，都红。
func TestFrozenAggregatesSQL_LockstepWithEarlierUnsolvedSibling(t *testing.T) {
	kernelArms := extractThreeTierArms(t, EarlierUnsolvedSiblingSQL)
	frozenArms := extractThreeTierArms(t, FrozenAggregatesSQL)
	require.Len(t, kernelArms, 3, "EarlierUnsolvedSiblingSQL earlier-than predicate must stay three-tier")
	require.Len(t, frozenArms, 3, "FrozenAggregatesSQL earlier-than predicate must stay three-tier")

	for i := range kernelArms {
		kernelArms[i] = normalizeArmIndent(kernelArms[i])
	}
	for i := range frozenArms {
		frozenArms[i] = normalizeArmIndent(frozenArms[i])
	}

	// 臂的锚点语义（改列名/换锚列也算谓词漂移，必须显式过一遍三方评审）。
	assert.Contains(t, kernelArms[0], "causal_seq", "kernel arm 1 anchors on causal_seq")
	assert.Contains(t, kernelArms[1], "src_partition", "kernel arm 2 anchors on src_partition/src_offset")
	assert.Contains(t, kernelArms[2], "first_seen_at", "kernel arm 3 anchors on first_seen_at")

	for i, kernelArm := range kernelArms {
		want := mirroredFragments(t, kernelArm)
		got := mirroredFragments(t, frozenArms[i])
		assert.Equal(t, want, got,
			"arm %d drifted out of lockstep: every e<c comparison must appear as c>e in FrozenAggregatesSQL and vice versa (R4-H)", i+1)
	}

	// 锁步常量三件套共享的「未解决」状态集（改任何一方都要三方同步）。
	assert.Contains(t, FrozenAggregatesSQL, "IN ('RETRY_SCHEDULED','PROCESSING','DEAD_LETTER')")
	assert.Contains(t, EligibleHeadsSQL, "IN ('RETRY_SCHEDULED','PROCESSING','DEAD_LETTER')")

	// aggregate-less 守卫（OV②）：FrozenAggregatesSQL 必须带 `e.aggregate_type IS NOT NULL
	// AND e.aggregate_id <> ''`，与 EligibleHeadsSQL 外层守卫（replay.go，`c.aggregate_type
	// IS NULL OR c.aggregate_id = ''`）互补成对——一个跳过 NOT EXISTS、一个直接排除死信外层。
	// 无此守卫时 ''='' 恒真 ⇒ 任意两条 aggregate-less 行互判冻结（永久误报）。
	assert.Contains(t, FrozenAggregatesSQL, "(e.aggregate_type IS NOT NULL AND e.aggregate_id <> '')")
	assert.Contains(t, EligibleHeadsSQL, "(c.aggregate_type IS NULL OR c.aggregate_id = ''")
}

// extractThreeTierArms 从 earlier-than 谓词常量里切出 `AND ( … )` 块的顶层 OR 臂。
// 用括号深度配对找块边界（锚子串/行号都会随注释与缩进漂移），再在深度 0 处按 OR 切臂。
func extractThreeTierArms(t *testing.T, sql string) []string {
	t.Helper()
	const openAnchor = "AND ("
	start := strings.Index(sql, openAnchor+"\n")
	require.GreaterOrEqual(t, start, 0, "open anchor %q not found in source const", openAnchor)
	open := strings.Index(sql[start:], "(") + start
	depth := 0
	end := -1
	for i := open; i < len(sql); i++ {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	require.GreaterOrEqual(t, end, 0, "unbalanced parens after anchor %q", openAnchor)
	return splitTopLevelOR(sql[open+1 : end])
}

// splitTopLevelOR 在括号深度 0 处按 OR 切分（臂内嵌套括号里的 OR 不受影响）。
func splitTopLevelOR(body string) []string {
	var arms []string
	var cur strings.Builder
	depth := 0
	i := 0
	for i < len(body) {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 {
			trimmed := strings.TrimSpace(body[i:])
			if strings.HasPrefix(trimmed, "OR ") && (body[i] == ' ' || body[i] == '\n') {
				if s := strings.TrimSpace(cur.String()); s != "" {
					arms = append(arms, s)
				}
				cur.Reset()
				// 跳过本分隔符（空白 + "OR"）。
				for i < len(body) && (body[i] == ' ' || body[i] == '\n') {
					i++
				}
				i += len("OR")
				for i < len(body) && (body[i] == ' ' || body[i] == '\n') {
					i++
				}
				continue
			}
		}
		cur.WriteByte(body[i])
		i++
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		arms = append(arms, s)
	}
	return arms
}

// normalizeArmIndent 把臂内所有空白折叠成单空格（两常量缩进不同：EarlierUnsolvedSiblingSQL
// 臂体 4 空格，FrozenAggregatesSQL 8 空格）。
func normalizeArmIndent(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var (
	// cmpRe 匹配 `e.col <op> c.col` 形态的列比较（op ∈ {<,>,=}；`<>` 不在谓词子集内，
	// 两字符序列不匹配该单符正则，天然排除）。
	cmpRe = regexp.MustCompile(`\b([ec])\.(\w+)\s*([<>=])\s*([ec])\.(\w+)`)
	// nullRe 匹配 `e.col IS NULL` / `e.col IS NOT NULL`（方向无关谓词）。
	nullRe = regexp.MustCompile(`\b[ec]\.\w+ IS (?:NOT )?NULL`)
)

// flipComp 翻转比较符（镜像方向：< ↔ >；= 自反）。
func flipComp(op string) string {
	switch op {
	case "<":
		return ">"
	case ">":
		return "<"
	default:
		return op
	}
}

// mirroredFragments 把一条臂规范化成排序后的「镜像片段」集合：
//   - 每条比较 `e.col < c.col` 规范化为 c-侧在前 + 翻转操作符（`c.col > e.col`）；
//   - 每条 IS [NOT] NULL 谓词原样保留（无方向）。
//
// kernel 臂与 frozen 臂各跑一遍：kernel 侧得到「应出现的镜像」，frozen 侧得到「实际写出的
// 镜像」，两者集合相等即锁步成立。
func mirroredFragments(t *testing.T, arm string) []string {
	t.Helper()
	var frags []string
	for _, m := range cmpRe.FindAllStringSubmatch(arm, -1) {
		left, lcol, op, right, rcol := m[1], m[2], m[3], m[4], m[5]
		if left == "e" {
			// kernel 侧 e.col < c.col → 规范化（换位 + 翻转）为 c.col > e.col
			frags = append(frags, fmt.Sprintf("%s.%s %s %s.%s", right, rcol, flipComp(op), left, lcol))
		} else {
			// frozen 侧已是 c 侧在前：c.col > e.col 即规范化形态，原样保留
			frags = append(frags, fmt.Sprintf("%s.%s %s %s.%s", left, lcol, op, right, rcol))
		}
	}
	frags = append(frags, nullRe.FindAllString(arm, -1)...)
	sort.Strings(frags)
	return frags
}
