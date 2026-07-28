package repotest

import (
	"context"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RunQuarantineConformance 覆盖 raw_message_quarantine（§2.3 / 准入 ⑯ store 层）。
func RunQuarantineConformance(t *testing.T, d *ConformanceDeps) {
	t.Run("Record_Idempotent_OnDuplicateDelivery", func(t *testing.T) {
		q := quarantineRow()
		id1, err := d.QStore.Record(context.Background(), d.DB, q)
		require.NoError(t, err)
		id2, err := d.QStore.Record(context.Background(), d.DB, q)
		require.NoError(t, err)
		assert.Equal(t, id1, id2, "duplicate delivery idempotent")
	})
	t.Run("MarkResolved_CAS", func(t *testing.T) {
		q := quarantineRow()
		q.SrcOffset = q.SrcOffset + 100
		id, err := d.QStore.Record(context.Background(), d.DB, q)
		require.NoError(t, err)
		require.NoError(t, d.QStore.MarkResolved(context.Background(), d.DB, id, 1, "ops"))
		assert.Error(t, d.QStore.MarkResolved(context.Background(), d.DB, id, 1, "ops"), "stale version conflicts")
	})
	// B7：headers 列是 NOT NULL，而一条不带 header 的坏消息必须也能落隔离区——
	// 否则按 §4 语义必须上抛不 ACK → 分区阻塞（准入 ⑯ 会踩到）。
	t.Run("Record_EmptyHeaders_NotNullColumn", func(t *testing.T) {
		q := quarantineRow()
		q.SrcOffset = q.SrcOffset + 200
		q.Headers = nil
		id, err := d.QStore.Record(context.Background(), d.DB, q)
		require.NoError(t, err, "headerless bad message must still be quarantinable")
		assert.Greater(t, id, int64(0))
	})
}

// quarantineRow 直接用真实 store.QuarantineRow 类型（helpers.go 已 import store，无占位 alias）。
func quarantineRow() store.QuarantineRow {
	return store.QuarantineRow{
		HandlerID: reliable.HandlerID("h"), Topic: "t", SrcPartition: 1, SrcOffset: 2,
		RawValue: []byte("raw"), RawPayloadHash: "hash1", Status: "QUARANTINED",
		Headers: []reliable.HeaderPair{{Key: "k", Value: []byte("v")}},
	}
}
