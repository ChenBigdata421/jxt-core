package repotest

import (
	"context"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RunInvariant 遍历 §2.4 四态断言行内字段组合。
func RunInvariant(t *testing.T, d *ConformanceDeps) {
	// C3：原稿只盖了 RETRY_SCHEDULED / DEAD_LETTER / DISCARDED 三态，缺 SUCCEEDED。
	// SUCCEEDED 的「必须清空」列最长（§2.4 表：7 个字段），且这条测试正好能暴露
	// MarkSucceeded 漏清 error_code / error_fingerprint 的问题。
	t.Run("Succeeded_ClearsAllOwnershipAndErrorFields", func(t *testing.T) {
		in := newClaimInput(t, "inv-ok")
		tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
		// 先失败一次把 error_* / payload / next_attempt_at 写满，再重新占位并成功，
		// 确保断言的是「成功时确实清干净了」而不是「本来就空」。
		require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
			reliable.ClassRetryable, reliable.ReplayIdempotent, 5, reliable.Retryable(reliableErr("transient")), []byte("p")))
		dirty := mustGetByEvent(t, d, in.Key)
		require.Equal(t, "RETRY_SCHEDULED", dirty.Status)

		tok2, dec, err := d.Store.ClaimForReplay(context.Background(), d.DB, dirty.ID)
		require.NoError(t, err)
		require.NotEmpty(t, dec.EventID)
		require.NoError(t, d.Store.MarkSucceeded(context.Background(), d.DB, in.Key, tok2))

		r := mustGetByEvent(t, d, in.Key)
		require.Equal(t, "SUCCEEDED", r.Status)
		assert.Nil(t, r.ClaimID, "§2.4: SUCCEEDED clears claim_id")
		assert.Nil(t, r.LeaseExpiresAt, "§2.4: SUCCEEDED clears lease_expires_at")
		assert.Nil(t, r.NextAttemptAt, "§2.4: SUCCEEDED clears next_attempt_at")
		assert.Nil(t, r.ErrorClass, "§2.4: SUCCEEDED clears error_class")
		assert.Nil(t, r.Payload, "§2.4: SUCCEEDED clears payload (热表不承担 BLOB 成本)")

		// §2.4 表未列但 §10 依赖：成功行不得残留上次失败的指纹/码，否则污染
		// 按 error_fingerprint 的聚合定位。
		var fp, code string
		require.NoError(t, d.DB.Raw(`SELECT error_fingerprint FROM event_consumption WHERE event_id=?`, in.Key.EventID).Scan(&fp).Error)
		require.NoError(t, d.DB.Raw(`SELECT error_code FROM event_consumption WHERE event_id=?`, in.Key.EventID).Scan(&code).Error)
		assert.Empty(t, fp, "SUCCEEDED must clear error_fingerprint")
		assert.Empty(t, code, "SUCCEEDED must clear error_code")
	})
	t.Run("RetryScheduled_HasPayloadDueAndErrorClass", func(t *testing.T) {
		in := newClaimInput(t, "inv-retry")
		tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
		require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
			reliable.ClassRetryable, reliable.ReplayIdempotent, 5, reliable.Retryable(reliableErr("x")), []byte("p")))
		r := mustGetByEvent(t, d, in.Key)
		require.Equal(t, "RETRY_SCHEDULED", r.Status)
		assert.NotNil(t, r.Payload, "§2.4: RETRY_SCHEDULED requires payload")
		assert.NotNil(t, r.NextAttemptAt)
		assert.NotNil(t, r.ErrorClass)
		assert.Nil(t, r.ClaimID, "§2.4: clears ownership")
	})
	t.Run("DeadLetter_HasPayloadAndErrorClass_NoNextAttempt", func(t *testing.T) {
		in := newClaimInput(t, "inv-dl")
		tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
		require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
			reliable.ClassPoison, reliable.ReplayUnsafe, 5, reliable.Permanent(reliableErr("bad")), []byte("p")))
		r := mustGetByEvent(t, d, in.Key)
		require.Equal(t, "DEAD_LETTER", r.Status)
		assert.NotNil(t, r.Payload, "§2.4 v2.8: DEAD_LETTER payload is hard condition")
		assert.NotNil(t, r.ErrorClass)
		assert.Nil(t, r.NextAttemptAt)
		assert.Nil(t, r.ClaimID)
	})
	t.Run("Discarded_HasResolvedFields", func(t *testing.T) {
		in := newClaimInput(t, "inv-discard")
		tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
		require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
			reliable.ClassPoison, reliable.ReplayUnsafe, 5, reliable.Permanent(reliableErr("bad")), []byte("p")))
		r := mustGetByEvent(t, d, in.Key)
		require.NoError(t, d.Store.Discard(context.Background(), d.DB, r.ID, r.RowVersion, "ops", "junk"))
		// 读回校验 resolved 字段（raw SQL）
		var resolved *string
		require.NoError(t, d.DB.Raw(`SELECT resolved_by FROM event_consumption WHERE id=?`, r.ID).Scan(&resolved).Error)
		assert.NotNil(t, resolved, "§2.4: DISCARDED requires resolved_at/resolved_by")
	})
	t.Run("Fingerprint_IsSha256_64Hex", func(t *testing.T) {
		in := newClaimInput(t, "inv-fp")
		tok, _, _ := d.Store.TryClaim(context.Background(), in, lease5)
		require.NoError(t, d.Store.MarkFailed(context.Background(), d.DB, in.Key, tok,
			reliable.ClassPoison, reliable.ReplayUnsafe, 5, reliable.Permanent(reliableErr("bad")), []byte("p")))
		var fp string
		require.NoError(t, d.DB.Raw(`SELECT error_fingerprint FROM event_consumption WHERE event_id=?`, in.Key.EventID).Scan(&fp).Error)
		assert.Len(t, fp, 64, "D10: fingerprint is sha256 64 hex")
	})
}
