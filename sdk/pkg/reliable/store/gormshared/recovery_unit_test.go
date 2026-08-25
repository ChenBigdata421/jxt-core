package gormshared

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRecoverRowSQL_LeaseRecheckGuard（review OV③，P4 determinism resolution 条款 a）：
// 回收 UPDATE 的 WHERE 必须带 `AND lease_expires_at < ?` 复验——SELECT 与 UPDATE 之间
// broker 重投可触发 tryClaimOnce 的内联续占（claim.go：租约刷新但 status 仍 PROCESSING），
// 不带此守卫会砸掉在途消费者的活租约，其 MarkSucceeded 将 ErrConflict。
//
// 真正的 SELECT-then-UPDATE 竞态交错无法在无钩子下从外部构造（内核函数不可注入），
// WHERE 子句即防线：这里以字符串级断言钉住它必然存在，配合 recovery_system_test.go 的
// 条款 b（P4 先被外部 UPDATE 到新租约再调 RecoverExpiredProcessing，断言行未被触碰）
// 共同覆盖 OV③。若有人删掉该守卫，本测试即红。
func TestRecoverRowSQL_LeaseRecheckGuard(t *testing.T) {
	assert.Contains(t, recoverRowSQL, "AND lease_expires_at < ?",
		"OV③: recovery UPDATE must re-check lease_expires_at < cutoff (concurrent inline re-claim between SELECT and UPDATE must MISS)")
	assert.Contains(t, recoverRowSQL, "AND status = 'PROCESSING'",
		"recovery UPDATE must re-check status (row may have been settled concurrently)")
	// COALESCE 形态（终评 minors）：首轮回收写占位值，重放过的行保留上一轮真实失败元数据
	// （error_class 非空即满足 chk_retry_due）。
	assert.Contains(t, recoverRowSQL, "error_class = COALESCE(error_class, 'RETRYABLE')",
		"chk_retry_due: RETRY_SCHEDULED requires error_class NOT NULL (COALESCE preserves prior forensics)")
	assert.Contains(t, recoverRowSQL, "next_attempt_at = ?",
		"chk_retry_due: RETRY_SCHEDULED requires next_attempt_at NOT NULL")
	// 回收必须清 ownership（§2.4：RETRY_SCHEDULED 必清 claim_id/claimed_at/lease_expires_at），
	// 否则残留的 lease_expires_at 会让下一轮 cutoff 判定读到脏值。
	assert.Contains(t, recoverRowSQL, "claim_id = NULL")
	assert.Contains(t, recoverRowSQL, "lease_expires_at = NULL")
	// 候选 SELECT 只收 payload 非空行（§2.1：payload-NULL 交 broker 重投，只计数）。
	assert.Contains(t, recoverCandidateSQL, "payload IS NOT NULL")
	// 与 candidate SELECT 的 cutoff 谓词形态一致（同一 cutoff 参数复验）。
	assert.Equal(t, strings.Count(recoverRowSQL, "lease_expires_at < ?"), 1)
}
