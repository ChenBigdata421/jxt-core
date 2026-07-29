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
		require.NoError(t, d.QStore.MarkResolved(context.Background(), d.DB, 0, id, 1, "ops"))
		assert.Error(t, d.QStore.MarkResolved(context.Background(), d.DB, 0, id, 1, "ops"), "stale version conflicts")
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

	// review #1（纵深防御 / 内部一致性）：uk_raw_delivery 必须含 tenant_id。当前每租户独立库不会触发，
	// 但共享库下两租户撞上相同 (topic,partition,offset,handler) 时，第二租户的毒消息会被
	// ON CONFLICT DO NOTHING 静默吞掉，回读还拿到第一租户的 id（丢消息 + id 污染 + ACK 语义错误）。
	// consumption_anomalies.uk_anomaly_once 已含 tenant_id（同理由，见 mysql/migration.go:82 注释），
	// 隔离区须对齐。用单测试库即可建模——两租户同坐标必须各得一行。
	t.Run("Record_TwoTenants_SameCoordinates_DistinctRows", func(t *testing.T) {
		const off int64 = 302
		qid1, err := d.QStore.Record(context.Background(), d.DB, store.QuarantineRow{
			TenantID: 1, HandlerID: reliable.HandlerID("h"), Topic: "t", SrcPartition: 1, SrcOffset: off,
			RawValue: []byte("v1"), RawPayloadHash: "h1", Status: "QUARANTINED",
		})
		require.NoError(t, err)
		qid2, err := d.QStore.Record(context.Background(), d.DB, store.QuarantineRow{
			TenantID: 2, HandlerID: reliable.HandlerID("h"), Topic: "t", SrcPartition: 1, SrcOffset: off,
			RawValue: []byte("v2"), RawPayloadHash: "h2", Status: "QUARANTINED",
		})
		require.NoError(t, err)
		assert.NotEqual(t, qid1, qid2, "two tenants with identical broker coordinates must get distinct quarantine rows")

		// 各自按 tenant 读回自己的行，且 raw payload 不串租户。
		got1, err := d.QStore.GetByID(context.Background(), 1, qid1)
		require.NoError(t, err)
		assert.Equal(t, []byte("v1"), got1.RawValue, "tenant-1 row must hold tenant-1 payload")
		got2, err := d.QStore.GetByID(context.Background(), 2, qid2)
		require.NoError(t, err)
		assert.Equal(t, []byte("v2"), got2.RawValue, "tenant-2 row must hold tenant-2 payload")

		// 这组坐标恰有 2 行（每租户一条），证明没有被跨租户静默去重。
		var n int64
		require.NoError(t, d.DB.WithContext(context.Background()).Raw(
			`SELECT COUNT(*) FROM raw_message_quarantine WHERE topic=? AND src_partition=? AND src_offset=? AND handler_id=?`,
			"t", 1, off, "h").Scan(&n).Error)
		assert.Equal(t, int64(2), n, "identical coordinates must yield one row per tenant (no silent cross-tenant dedup)")
	})

	// review #2（纵深防御）：MarkResolved 必须按 tenant 作用域——隔离区读已 PII 加固（GetByID/List），
	// 处置写同理不可跨租户。证据系统里跨租户 resolve = 静默处置别租户的毒消息证据（完整性问题）。
	// tenant-1 凭泄露的 (id, version) 处置 tenant-2 的行：必须 ErrConflict，且 tenant-2 行不被动。
	t.Run("MarkResolved_RejectsCrossTenant", func(t *testing.T) {
		qid2, err := d.QStore.Record(context.Background(), d.DB, store.QuarantineRow{
			TenantID: 2, HandlerID: reliable.HandlerID("h"), Topic: "tx", SrcPartition: 1, SrcOffset: 303,
			RawValue: []byte("victim"), RawPayloadHash: "hx2", Status: "QUARANTINED",
		})
		require.NoError(t, err)

		// tenant-1（运维）拿到 tenant-2 的 (id, version=1) 试图处置。
		err = d.QStore.MarkResolved(context.Background(), d.DB, 1, qid2, 1, "ops-tenant-1")
		assert.ErrorIs(t, err, reliable.ErrConflict, "tenant-1 must not resolve tenant-2 row")

		// tenant-2 行未被触碰：仍 QUARANTINED、version 仍 1。
		got, err := d.QStore.GetByID(context.Background(), 2, qid2)
		require.NoError(t, err)
		assert.Equal(t, "QUARANTINED", got.Status, "victim row status untouched by cross-tenant resolve")
		assert.Equal(t, int64(1), got.RowVersion, "victim row version untouched by cross-tenant resolve")
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
