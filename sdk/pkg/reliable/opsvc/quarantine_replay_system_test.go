//go:build system
// +build system

// quarantine_replay 系统测试（PR-7 Task 3 review I-2）：QuarantineReplay 的三个隔离区
// CAS 迁移（casToReplaying / casBackToQuarantined / casToResolved）此前只有 in-memory
// fake 覆盖（service_test.go 的 fakeQrState），真实 SQL 谓词零真库覆盖——这是旗舰端点的
// D1 失败类门禁（Error 1054 列缺失 / 方言语法错 / 时间绑定错）缺口。本套用真实 MySQL +
// PostgreSQL（repotest.Setup：env DSN 逃生舱，无则 testcontainers，再无则 skip）钉住：
//
//	a) claim→back 周期（解码失败路径）：QUARANTINED→REPLAYING→QUARANTINED，
//	   row_version +2、replay_attempts +1、error_message = sanitize(cause)、updated_at 被推进。
//	b) claim→resolve 周期（毕业/AlreadySettled 路径）：QUARANTINED→REPLAYING→RESOLVED，
//	   resolved_by = r.By、row_version +2、replay_attempts 不动；
//	   review I-3：RESOLVED 迁移不得覆盖 error_message（原始 quarantine cause 是 incident 记录）。
//
// 走【完整服务路径】而非裸 UPDATE：opsvc.NewService + 真 registry/decoder fake +
// real-DB-backed resolver（repotest.NewStoreFor 构造的 store.Store / store.QuarantineStore），
// svc.qrClaim/qrBack/qrResolve 保持 NewService 绑定的生产默认值（即本文件外的真实 gorm
// 实现）。TryClaim/MarkFailed 也跑真库——周期 (a) 在解码步就折返，TryClaim 用不上；
// 周期 (b) 喂 Claimed 决策太重（MarkFailed 会写 event_consumption 一整套），改用
// AlreadySettled：真库上 TryClaim 先 INSERT 占位再无行可撞 → 把 event_consumption 预置成
// SUCCEEDED 即真实命中 AlreadySettled 分支（claim.go:84-87），从而 resolve 走的也是
// 服务真实编排。casToResolved 对 Claimed/AlreadySettled 两分支本就是同一条语句。
//
// 外部测试包 opsvc_test：需要 repotest.Setup（env DSN 逃生舱），而 repotest 经
// mysql/postgres 薄包 → gormshared，均不 import opsvc（go list -deps 核实无环），但内部
// 测试包直接引 repotest 会把 testcontainers 拉进常规单测的依赖闭包，沿用 gormshared
// 系统测试（recovery_system_test.go / opsprobe_system_test.go）的外部包 + `system` tag
// 惯例——无 tag 时不编译，`go test ./pkg/reliable/opsvc/` 保持零容器依赖。
//
// 运行（专用 scratch 容器；setup.go 会按需自动补 parseTime/multiStatements/loc）：
//
//	RELIABLE_MYSQL_DSN='test:test@tcp(127.0.0.1:3381)/reliable_test?charset=utf8mb4' \
//	RELIABLE_PG_DSN='postgres://test:test@127.0.0.1:5481/reliable_test?sslmode=disable' \
//	go test -count=1 -tags system ./pkg/reliable/opsvc/ -v
package opsvc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/opsvc"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/replay"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/repotest"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var qrDialects = []repotest.Dialect{repotest.DialectMySQL, repotest.DialectPostgres}

// qrFixture 是一个真库方言上的完整服务装配：真 store/quarantineStore/db + 最小 fake
// registry / auditor / decoder。resolver 把三者按 tenant 固定返回（一库一租户原则的测试面）。
type qrFixture struct {
	db      *gorm.DB
	st      store.Store
	qst     store.QuarantineStore
	svc     *opsvc.Service
	decoder *qrDecoderFake
}

// qrTenant / qrHandlerID：装配常量（与 service_test.go 的 harness 取值解耦——真库种子
// 的 uk_raw_delivery 坐标由用例自持，避免跨用例唯一键相撞）。
const (
	qrTenant     = 40
	qrHandlerID  = reliable.HandlerID("qr-sys.v1")
	qrTopic      = "domain.media.qr"
	qrRowVersion = int64(1) // Record 落库的初始版本（DDL DEFAULT 1）。
)

// qrRegistryFake 实现 replay.HandlerRegistry：Lookup 恒命中（system 测试关心 CAS 谓词，
// 不关心 registry 拒绝路径——那条由 service_test.go 覆盖）。Handler 恒 nil 没问题：
// OV⑤ 约定 QuarantineReplay 只读 HandlerInfo.ReplaySafety，绝不调 Handler.Handle。
type qrRegistryFake struct{}

func (qrRegistryFake) Lookup(id reliable.HandlerID) (replay.HandlerInfo, bool) {
	return replay.HandlerInfo{HandlerID: id, ReplaySafety: reliable.ReplayIdempotent}, true
}
func (qrRegistryFake) All() []replay.HandlerInfo { return nil }

// qrDecoderFake 是可控 EnvelopeDecoder：err 非 nil 模拟毒字节解码失败（驱动 claim→back
// 周期）；err==nil 时返回与预置 event_consumption 行同 Key 的身份（驱动 TryClaim 命中
// AlreadySettled → claim→resolve 周期）。
type qrDecoderFake struct {
	err error
}

func (d *qrDecoderFake) decode(raw []byte) (reliable.Key, reliable.Meta, int, error) {
	if d.err != nil {
		return reliable.Key{}, reliable.Meta{}, 0, d.err
	}
	// tenant 与行所在租户一致（跨租户拒绝路径由 service_test.go 覆盖，此处不重复）。
	return reliable.Key{EventID: "qr-sys-evt-1", Handler: qrHandlerID, ItemKey: ""},
		reliable.Meta{EventType: "FileUploaded", AggregateType: "Media", AggregateID: "qr-agg-1"},
		qrTenant, nil
}

// qrAuditorFake 满足 opsvc.AccessAuditor（QuarantineReplay 不发特权读审计——见
// service.go 审计判定注释；auditor 仅是 NewService 的必填构造参数）。
type qrAuditorFake struct{}

func (qrAuditorFake) RecordPrivilegedAccess(ctx context.Context, e opsvc.PrivilegedAccessEvent) error {
	return nil
}

// qrResolver 把真库三件套按固定租户返回（store.TenantStoreResolver 的最小实现）。
type qrResolver struct {
	st  store.Store
	qst store.QuarantineStore
	db  *gorm.DB
}

func (r *qrResolver) Store(tenantID int) (store.Store, *gorm.DB, error) { return r.st, r.db, nil }
func (r *qrResolver) QuarantineStore(tenantID int) (store.QuarantineStore, error) {
	return r.qst, nil
}

// newQrFixture 在干净真库上装配服务。svc.qrClaim/qrBack/qrResolve 不覆写——保留
// NewService 绑定的生产 gorm 实现（这正是本套的被测对象）。
func newQrFixture(t *testing.T, dialect repotest.Dialect) *qrFixture {
	t.Helper()
	db, cleanup := repotest.Setup(t, dialect)
	t.Cleanup(cleanup)
	st, qst := repotest.NewStoreFor(dialect, db)
	dec := &qrDecoderFake{}
	svc, err := opsvc.NewService(&qrResolver{st: st, qst: qst, db: db}, qrAuditorFake{},
		opsvc.WithRegistry(qrRegistryFake{}), opsvc.WithEnvelopeDecoder(dec.decode))
	require.NoError(t, err)
	return &qrFixture{db: db, st: st, qst: qst, svc: svc, decoder: dec}
}

// seedQuarantinedRow 落一条 QUARANTINED 隔离行（真 Record 路径——ON CONFLICT 幂等 +
// GORM autoUpdateTime 写 updated_at（model.go M-1 注释）），返回行 id。
// cause 即 DLQ 落隔离时记的原始 quarantine cause（I-3 断言的「必须被保留」对象）。
func seedQuarantinedRow(t *testing.T, f *qrFixture, srcOffset int64, cause string) int64 {
	t.Helper()
	id, err := f.qst.Record(context.Background(), f.db, store.QuarantineRow{
		TenantID: qrTenant, HandlerID: qrHandlerID, Topic: qrTopic,
		SrcPartition: 3, SrcOffset: srcOffset,
		RawValue: []byte("qr-env-bytes"), RawKey: []byte("rk"),
		Headers:        []reliable.HeaderPair{{Key: "h", Value: []byte("v")}},
		RawPayloadHash: "qr-hash-1", ErrorMessage: cause, Status: "QUARANTINED",
	})
	require.NoError(t, err)
	require.NotZero(t, id)
	return id
}

// seedSettledConsumption 预置一条 SUCCEEDED 的 event_consumption 行（TryClaim 读到非
// PROCESSING 终态 → AlreadySettled，claim.go:84-87）。列为 chk 约束的最小满足集。
func seedSettledConsumption(t *testing.T, f *qrFixture, eventID string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, f.db.Exec(
		`INSERT INTO event_consumption
		   (event_id,item_key,handler_id,tenant_id,event_type,aggregate_type,aggregate_id,causal_seq,topic,
		    status,attempt,replay_mode,resolved_at,raw_payload_hash,
		    first_seen_at,created_at,updated_at)
		 VALUES (?,'',?,?,'FileUploaded','Media','qr-agg-1',NULL,?,'SUCCEEDED',1,'AUTO',?,?, ?,?,?)`,
		eventID, string(qrHandlerID), qrTenant, qrTopic, now, "qr-hash-1",
		now.Add(-time.Hour), now.Add(-time.Hour), now.Add(-time.Hour),
	).Error)
}

// qrSnap 是隔离行快照（raw SQL 读，方言无关；updated_at/resolved_at 可空列用指针）。
type qrSnap struct {
	Status         string
	RowVersion     int64
	ReplayAttempts int
	ErrorMessage   *string
	UpdatedAt      *time.Time
	ResolvedAt     *time.Time
	ResolvedBy     *string
}

func getQrSnap(t *testing.T, f *qrFixture, id int64) qrSnap {
	t.Helper()
	var s qrSnap
	require.NoError(t, f.db.Raw(
		`SELECT status, row_version, replay_attempts, error_message, updated_at, resolved_at, resolved_by
		 FROM raw_message_quarantine WHERE id = ?`, id).Scan(&s).Error)
	return s
}

// TestQuarantineReplay_System_ClaimBackCycle（I-2 周期 a）：解码失败驱动的完整往返——
// 真库上 casToReplaying（QUARANTINED→REPLAYING，含 OR 谓词 + watchdog 时间绑定）与
// casBackToQuarantined（REPLAYING→QUARANTINED，replayRowVer+1 复合谓词）逐列断言。
// 这两条 UPDATE 的任何方言错（列缺失 1054 / 时间绑定 / gorm.Expr 方言翻译）在这里变红。
func TestQuarantineReplay_System_ClaimBackCycle(t *testing.T) {
	for _, dialect := range qrDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			f := newQrFixture(t, dialect)
			ctx := context.Background()
			const origCause = "decode: envelope: bad crc; cause: handler panic"
			id := seedQuarantinedRow(t, f, 1001, origCause)
			// 毒字节：解码必败 → back 路径。注意 back-CAS 写的 error_message 是【本周期】
			// 的失败 cause（decode 错误），不是落隔离时的原始 cause——这是 casBackToQuarantined
			// 的设计语义（行上的 live incident 状态）；I-3 禁止覆盖的是 RESOLVED 迁移。
			f.decoder.err = errors.New("envelope: bad crc")

			before := getQrSnap(t, f, id)
			require.Equal(t, "QUARANTINED", before.Status)
			require.Equal(t, int64(1), before.RowVersion)
			require.Equal(t, 0, before.ReplayAttempts)
			require.NotNil(t, before.UpdatedAt, "Record must write updated_at (GORM autoUpdateTime, review M-1)")

			err := f.svc.QuarantineReplay(ctx, opsvc.QuarantineReplayRequest{
				TenantID: qrTenant, ID: id, ExpectedRowVersion: qrRowVersion, By: "ops-alice",
			})
			require.Error(t, err, "decode failure must propagate (joined with the back-CAS result)")

			after := getQrSnap(t, f, id)
			// 状态周期：QUARANTINED → REPLAYING → QUARANTINED（两次 CAS 各 +1 row_version）。
			require.Equal(t, "QUARANTINED", after.Status)
			require.Equal(t, int64(3), after.RowVersion, "claim(+1) then back(+1)")
			require.Equal(t, 1, after.ReplayAttempts, "each failed cycle counts (OV⑤③)")
			// back 写 sanitize 后的 cause（此处无敏感模式，原样保留）。
			require.NotNil(t, after.ErrorMessage)
			require.Contains(t, *after.ErrorMessage, "bad crc", "back-CAS must record the sanitized cause")
			// updated_at 由 CAS 显式置 now（UTC）——watchdog 谓词列必须被写入。不与 Record
			// 的 autoUpdateTime 值做单调比较：GORM autoUpdateTime 用连接的 NowFunc（默认
			// time.Now()，随会话时区），CAS 写 time.Now().UTC()，而列是 TIMESTAMP(3) without
			// time zone——跨来源比较耦合会话时区，无生产意义（watchdog 只对 REPLAYING 行读
			// 该列，而 REPLAYING 行的列值恒为 CAS 写入的 UTC）。这里断言「写入了 UTC 合理值」。
			require.NotNil(t, after.UpdatedAt)
			require.WithinDuration(t, time.Now().UTC(), *after.UpdatedAt, 10*time.Minute,
				"back-CAS must stamp updated_at with UTC now (watchdog predicate column)")
			// 失败周期不产生处置标记。resolved_by 列可空，但 Record 的 model 字段是非指针
			// string（落 ''）——断言「无操作者身份」而非 NULL。
			require.Nil(t, after.ResolvedAt)
			if after.ResolvedBy != nil {
				require.Empty(t, *after.ResolvedBy, "failed cycle must not stamp resolved_by")
			}
		})
	}
}

// TestQuarantineReplay_System_ClaimResolveCycle（I-2 周期 b）：AlreadySettled 驱动的毕业
// 收尾——真库上 casToResolved（REPLAYING→RESOLVED，resolved_by 落操作者）。review I-3 的
// 核心断言在此：RESOLVED 迁移【不得】覆盖 error_message——原始 quarantine cause（隔离行
// 的 incident 记录）必须原样保留（与 gormshared.MarkResolved 同纪律）。
func TestQuarantineReplay_System_ClaimResolveCycle(t *testing.T) {
	for _, dialect := range qrDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			f := newQrFixture(t, dialect)
			ctx := context.Background()
			const origCause = "decode: envelope: bad crc; cause: handler panic"
			id := seedQuarantinedRow(t, f, 2001, origCause)
			// 预置 SUCCEEDED 消费行 → TryClaim 命中 AlreadySettled（真实分支，非 fake）。
			seedSettledConsumption(t, f, "qr-sys-evt-1")

			require.NoError(t, f.svc.QuarantineReplay(ctx, opsvc.QuarantineReplayRequest{
				TenantID: qrTenant, ID: id, ExpectedRowVersion: qrRowVersion, By: "ops-bob",
			}))

			after := getQrSnap(t, f, id)
			require.Equal(t, "RESOLVED", after.Status)
			require.Equal(t, int64(3), after.RowVersion, "claim(+1) then resolve(+1)")
			require.Equal(t, 0, after.ReplayAttempts, "success must not count (OV⑤③)")
			require.NotNil(t, after.ResolvedAt)
			require.NotNil(t, after.ResolvedBy)
			require.Equal(t, "ops-bob", *after.ResolvedBy)
			// I-3：原始 cause 保留——不得出现 "graduated"/"already settled" 之类的覆盖文案。
			require.NotNil(t, after.ErrorMessage)
			require.Equal(t, origCause, *after.ErrorMessage,
				"casToResolved must NOT clobber error_message (review I-3: quarantine row is the incident record)")
			require.NotContains(t, *after.ErrorMessage, "graduated")
			require.NotContains(t, *after.ErrorMessage, "already settled")
		})
	}
}

// TestQuarantineReplay_System_WatchdogPredicate（I-2 补充）：casToReplaying 的 OR 谓词
// 时间绑定在真库上的两面——(a) 陈旧 REPLAYING 行（updated_at 早于 watchdog 截止）可重claim；
// (b) 新鲜 REPLAYING 行 0 行命中 → *ConflictError。直接驱动生产 seam（绕过 GetByID 的
// 读完-写偏移：行状态由 SQL 预置，谓词只看 WHERE），这是 fake 无法证明的方言时间比较。
func TestQuarantineReplay_System_WatchdogPredicate(t *testing.T) {
	for _, dialect := range qrDialects {
		dialect := dialect
		t.Run(string(dialect), func(t *testing.T) {
			f := newQrFixture(t, dialect)
			ctx := context.Background()
			staleID := seedQuarantinedRow(t, f, 3001, "cause-stale")
			freshID := seedQuarantinedRow(t, f, 3002, "cause-fresh")
			// 直接把两行预置成 REPLAYING：stale 的 updated_at 拨回 11min 前（> 10min watchdog），
			// fresh 置 now。row_version 拨成 5（复验版本谓词不受 status 分支影响）。
			staleAt := time.Now().UTC().Add(-11 * time.Minute).Truncate(time.Millisecond)
			freshAt := time.Now().UTC().Truncate(time.Millisecond)
			require.NoError(t, f.db.Exec(
				`UPDATE raw_message_quarantine SET status='REPLAYING', row_version=5, updated_at=? WHERE id=?`,
				staleAt, staleID).Error)
			require.NoError(t, f.db.Exec(
				`UPDATE raw_message_quarantine SET status='REPLAYING', row_version=5, updated_at=? WHERE id=?`,
				freshAt, freshID).Error)

			// (a) 陈旧行：CAS 命中 → 重claim 成功，随后全链走完（decode→TryClaim INSERT 新
			// 消费行→Claimed→MarkFailed(payload=RawValue)→resolve）——额外覆盖了毕业主分支
			// 在真库上的 MarkFailed 落地（chk_retry_due 三件套 + event_consumption 侧产物）。
			require.NoError(t, f.svc.QuarantineReplay(ctx, opsvc.QuarantineReplayRequest{
				TenantID: qrTenant, ID: staleID, ExpectedRowVersion: 5, By: "ops",
			}))
			staleSnap := getQrSnap(t, f, staleID)
			require.Equal(t, "RESOLVED", staleSnap.Status, "re-claimed stale row must graduate end-to-end")
			require.Equal(t, int64(7), staleSnap.RowVersion, "preset 5, claim(+1), resolve(+1)")
			// 毕业主分支同样不覆盖 cause（I-3 对 Claimed 分支的镜像断言）。
			require.NotNil(t, staleSnap.ErrorMessage)
			require.Equal(t, "cause-stale", *staleSnap.ErrorMessage)
			// MarkFailed 真库产物：RETRY_SCHEDULED（RETRYABLE + ReplayIdempotent）+
			// payload 已持久化 + chk_retry_due 三件套齐备（否则 CHECK 直接拒写）。
			var cons struct {
				Status     string
				PayloadLen *int64
				ErrorClass *string
			}
			require.NoError(t, f.db.Raw(
				`SELECT status, OCTET_LENGTH(payload) AS payload_len, error_class FROM event_consumption WHERE event_id = ?`,
				"qr-sys-evt-1").Scan(&cons).Error)
			require.Equal(t, "RETRY_SCHEDULED", cons.Status)
			require.NotNil(t, cons.PayloadLen)
			require.Equal(t, int64(len("qr-env-bytes")), *cons.PayloadLen)
			require.NotNil(t, cons.ErrorClass)
			require.Equal(t, "RETRYABLE", *cons.ErrorClass)

			// (b) 新鲜行：CAS 0 行 → *ConflictError。
			err := f.svc.QuarantineReplay(ctx, opsvc.QuarantineReplayRequest{
				TenantID: qrTenant, ID: freshID, ExpectedRowVersion: 5, By: "ops",
			})
			require.Error(t, err)
			var ce *opsvc.ConflictError
			require.ErrorAs(t, err, &ce, "fresh REPLAYING row must conflict (watchdog guard)")
		})
	}
}
