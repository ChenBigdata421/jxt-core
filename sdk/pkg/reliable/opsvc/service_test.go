package opsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/replay"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"gorm.io/gorm"
)

// —— fakes ——
//
// fakeStore 嵌入 nil 的 store.Store 接口以 promotion 满足整张接口；只覆写 opsvc 实际调用的方法，
// 其余方法若被调用会 nil-deref panic（测试只走 opsvc 路径，不会触发）——这是 Go test fake 的惯用法，
// 避免为 22 个方法逐一写 panic 存根。fakeQuarantineStore 同理。

type scheduleCall struct {
	id, ver                     int64
	requester, approver, reason string
}

type fakeStore struct {
	store.Store // nil 嵌入：满足接口；未覆写方法 panic（测试不触发）。
	tenantID    int

	// 调用计数（cross-tenant isolation 断言用）。
	listCalls          int
	getByIDCalls       int
	scheduleCalls      int
	hasSiblingCalls    int
	discardCalls       int
	countCalls         int
	listAnomaliesCalls int

	// QuarantineReplay（PR-7 Task 3）行为控制与捕获。M-2：除 payload+safety 外同样捕获
	// class/maxAttempts/cause——毕业偏离的承重参数（ClassRetryable + DefaultMaxAttempts +
	// errQuarantineGraduated）必须被钉住：未来有人把 class 改成 ClassPoison（行直落死信，
	// 绕过 scheduler 正常驱动）或收紧 maxAttempts，测试必须红。
	tryClaimCalls             int
	markFailedCalls           int
	tryClaimDecision          reliable.Decision
	tryClaimToken             reliable.ClaimToken
	tryClaimErr               error
	markFailedErr             error
	capturedClaimInput        reliable.ClaimInput
	capturedMarkFailedPayload []byte
	capturedMarkFailedSafety  reliable.ReplaySafety
	capturedMarkFailedClass   reliable.ErrorClass
	capturedMarkFailedMaxAtt  int
	capturedMarkFailedCause   error

	// 捕获的入参。
	capturedListFilter    store.ListFilter
	capturedCountFilters  []store.CountFilter
	capturedAnomalyFilter store.AnomalyFilter
	capturedSchedule      scheduleCall
	capturedHasSiblingID  int64
	capturedHasSiblingDB  *gorm.DB
	capturedScheduleDB    *gorm.DB
	capturedDiscardID     int64
	capturedDiscardVer    int64
	capturedDiscardBy     string
	capturedDiscardReason string

	// 返回值。
	listRows      []store.Row
	listErr       error
	getByIDRow    store.Row
	getByIDErr    error
	scheduleErr   error
	hasSiblingRes bool
	hasSiblingErr error
	discardErr    error
	countRes      int64
	countErr      error
	anomalyRows   []store.AnomalyRow
	anomalyErr    error
}

// TryClaim / MarkFailed 覆写（QuarantineReplay 路径）。
func (f *fakeStore) TryClaim(ctx context.Context, in reliable.ClaimInput, lease time.Duration) (reliable.ClaimToken, reliable.Decision, error) {
	f.tryClaimCalls++
	f.capturedClaimInput = in
	if f.tryClaimErr != nil {
		return "", 0, f.tryClaimErr
	}
	return f.tryClaimToken, f.tryClaimDecision, nil
}
func (f *fakeStore) MarkFailed(ctx context.Context, db *gorm.DB, key reliable.Key, tok reliable.ClaimToken,
	class reliable.ErrorClass, safety reliable.ReplaySafety, maxAttempts int, cause error, payload []byte) error {
	f.markFailedCalls++
	f.capturedMarkFailedPayload = payload
	f.capturedMarkFailedSafety = safety
	f.capturedMarkFailedClass = class
	f.capturedMarkFailedMaxAtt = maxAttempts
	f.capturedMarkFailedCause = cause
	return f.markFailedErr
}

func (f *fakeStore) List(ctx context.Context, flt store.ListFilter) ([]store.Row, error) {
	f.listCalls++
	f.capturedListFilter = flt
	return f.listRows, f.listErr
}
func (f *fakeStore) GetByID(ctx context.Context, tenantID int, id int64) (store.Row, error) {
	f.getByIDCalls++
	return f.getByIDRow, f.getByIDErr
}
func (f *fakeStore) ScheduleReplay(ctx context.Context, db *gorm.DB, id, ver int64, req, appr, reason string) error {
	f.scheduleCalls++
	f.capturedSchedule = scheduleCall{id, ver, req, appr, reason}
	f.capturedScheduleDB = db
	return f.scheduleErr
}
func (f *fakeStore) HasEarlierUnsolvedSibling(ctx context.Context, db *gorm.DB, id int64) (bool, error) {
	f.hasSiblingCalls++
	f.capturedHasSiblingID = id
	f.capturedHasSiblingDB = db
	return f.hasSiblingRes, f.hasSiblingErr
}
func (f *fakeStore) Discard(ctx context.Context, db *gorm.DB, id, ver int64, by, reason string) error {
	f.discardCalls++
	f.capturedDiscardID = id
	f.capturedDiscardVer = ver
	f.capturedDiscardBy = by
	f.capturedDiscardReason = reason
	return f.discardErr
}
func (f *fakeStore) Count(ctx context.Context, flt store.CountFilter) (int64, error) {
	f.countCalls++
	f.capturedCountFilters = append(f.capturedCountFilters, flt)
	return f.countRes, f.countErr
}
func (f *fakeStore) ListAnomalies(ctx context.Context, flt store.AnomalyFilter) ([]store.AnomalyRow, error) {
	f.listAnomaliesCalls++
	f.capturedAnomalyFilter = flt
	return f.anomalyRows, f.anomalyErr
}

type fakeQuarantineStore struct {
	store.QuarantineStore
	tenantID int

	listCalls    int
	getByIDCalls int
	resolveCalls int

	capturedListStatus string
	capturedListLimit  int
	capturedGetID      int64
	capturedResolveID  int64
	capturedResolveVer int64
	capturedResolveBy  string
	capturedResolveDB  *gorm.DB

	listRows   []store.QuarantineRow
	listErr    error
	getByIDRow store.QuarantineRow
	getByIDErr error
	resolveErr error
}

func (f *fakeQuarantineStore) List(ctx context.Context, tenantID int, status string, limit int) ([]store.QuarantineRow, error) {
	f.listCalls++
	f.capturedListStatus = status
	f.capturedListLimit = limit
	return f.listRows, f.listErr
}
func (f *fakeQuarantineStore) GetByID(ctx context.Context, tenantID int, id int64) (store.QuarantineRow, error) {
	f.getByIDCalls++
	f.capturedGetID = id
	return f.getByIDRow, f.getByIDErr
}
func (f *fakeQuarantineStore) MarkResolved(ctx context.Context, db *gorm.DB, tenantID int, id, ver int64, by string) error {
	f.resolveCalls++
	f.capturedResolveID = id
	f.capturedResolveVer = ver
	f.capturedResolveBy = by
	f.capturedResolveDB = db
	return f.resolveErr
}

// fakeResolver 是 per-tenant store / qstore / db 的解析器。每个租户一套独立 fake，以便断言跨租户隔离。
// stores/qstores 存接口值，让 dispatchStore（id-driven fake）也能注入。
type fakeResolver struct {
	stores    map[int]store.Store
	qstores   map[int]store.QuarantineStore
	dbs       map[int]*gorm.DB
	storeErr  map[int]error
	qstoreErr map[int]error
}

func newFakeResolver() *fakeResolver {
	return &fakeResolver{
		stores: map[int]store.Store{}, qstores: map[int]store.QuarantineStore{},
		dbs: map[int]*gorm.DB{}, storeErr: map[int]error{}, qstoreErr: map[int]error{},
	}
}

// addTenant 注册一个被本进程服务的租户：独立的 fake store / qstore + sentinel db 指针。
func (r *fakeResolver) addTenant(tid int) (*fakeStore, *fakeQuarantineStore, *gorm.DB) {
	st := &fakeStore{tenantID: tid}
	qs := &fakeQuarantineStore{tenantID: tid}
	db := &gorm.DB{} // sentinel 指针；fake 不使用它的 driver。
	r.stores[tid] = st
	r.qstores[tid] = qs
	r.dbs[tid] = db
	return st, qs, db
}

func (r *fakeResolver) Store(tid int) (store.Store, *gorm.DB, error) {
	if err := r.storeErr[tid]; err != nil {
		return nil, nil, err
	}
	st, ok := r.stores[tid]
	if !ok {
		return nil, nil, errors.New("fake: tenant not served")
	}
	return st, r.dbs[tid], nil
}
func (r *fakeResolver) QuarantineStore(tid int) (store.QuarantineStore, error) {
	if err := r.qstoreErr[tid]; err != nil {
		return nil, err
	}
	qs, ok := r.qstores[tid]
	if !ok {
		return nil, errors.New("fake: tenant quarantine not served")
	}
	return qs, nil
}

// fakeAuditor 记录所有特权读事件；err 非 nil 时模拟审计失败（fail-closed 测试用）。
type fakeAuditor struct {
	events []PrivilegedAccessEvent
	err    error
}

func (a *fakeAuditor) RecordPrivilegedAccess(ctx context.Context, e PrivilegedAccessEvent) error {
	a.events = append(a.events, e)
	return a.err
}

// newSvcWithTxCapture 构造 Service 并把 txRunner 换成「直接调 fn(sentinelTx)」的 fake，
// 返回 svc 与捕获到的 tx 指针（断言「sibling-check 与 ScheduleReplay 拿到同一 tx」用）。
func newSvcWithTxCapture(t *testing.T, r *fakeResolver) (*Service, **gorm.DB, *fakeAuditor) {
	t.Helper()
	aud := &fakeAuditor{}
	svc, err := NewService(r, aud)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	var captured *gorm.DB
	sentinel := &gorm.DB{}
	svc.txRunner = func(db *gorm.DB, fn func(tx *gorm.DB) error) error {
		captured = sentinel
		return fn(sentinel)
	}
	return svc, &captured, aud
}

// —— tests ——

func TestNewService_RequiresAuditor(t *testing.T) {
	r := newFakeResolver()
	if _, err := NewService(r, nil); err == nil {
		t.Fatal("NewService(r, nil) must error (Q4=A: required auditor)")
	}
}

func TestNewService_RequiresResolver(t *testing.T) {
	aud := &fakeAuditor{}
	if _, err := NewService(nil, aud); err == nil {
		t.Fatal("NewService(nil, aud) must error")
	}
}

// crossTenantAssertion：对一次只该命中一个租户的 store 的 helper。
func assertOnlyTouched(t *testing.T, touched, other *fakeStore, msg string) {
	t.Helper()
	if touched.listCalls+touched.getByIDCalls+touched.scheduleCalls+touched.hasSiblingCalls+
		touched.discardCalls+touched.countCalls+touched.listAnomaliesCalls == 0 {
		t.Fatalf("expected tenant's store to be touched: %s", msg)
	}
	if other.listCalls+other.getByIDCalls+other.scheduleCalls+other.hasSiblingCalls+
		other.discardCalls+other.countCalls+other.listAnomaliesCalls != 0 {
		t.Fatalf("cross-tenant leak: other tenant store touched: %s", msg)
	}
}

func TestList_CrossTenantIsolation(t *testing.T) {
	r := newFakeResolver()
	st1, _, _ := r.addTenant(1)
	st2, _, _ := r.addTenant(2)
	st1.listRows = []store.Row{{ID: 10, TenantID: 1, Payload: []byte("secret")}}
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.List(context.Background(), ListQuery{TenantID: 1, Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0].ID != 10 {
		t.Fatalf("unexpected rows: %+v", res.Rows)
	}
	// List 是 payload-free-by-default：即使底层行带 Payload，返回行也必须清零（fail-closed，无审计钩子）。
	if res.Rows[0].Payload != nil || res.Rows[0].Headers != nil || res.Rows[0].RawKey != nil {
		t.Fatalf("List must strip gated fields (no includePayload flag / no audit): %+v", res.Rows[0])
	}
	if st1.capturedListFilter.TenantID != 1 {
		t.Fatalf("filter tenant = %d, want 1", st1.capturedListFilter.TenantID)
	}
	assertOnlyTouched(t, st1, st2, "List")
}

func TestList_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(context.Background(), ListQuery{}); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("List with TenantID=0 must return ErrMissingTenant, got %v", err)
	}
}

// TestList_AllowlistProjection_DropsSensitiveFields（P2-1）：List 返回 Detail 白名单投影，
// 结构上不含 ReplayAuthID（一次性人工重放 bearer）/ ReplayRequestedBy/ApprovedBy/Reason /
// DiscardReason。即便底层行带这些字段，JSON 序列化（handler 的实际暴露面）也不得出现它们——
// 钉住白名单契约，防未来把敏感字段加回 Detail 或回退到 store.Row 直接透出。
func TestList_AllowlistProjection_DropsSensitiveFields(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(1)
	st.listRows = []store.Row{{
		ID: 99, TenantID: 1,
		ClaimID:           "fencing-token-xyz",
		ReplayAuthID:      "bearer-replay-token",
		ReplayRequestedBy: "alice", ReplayApprovedBy: "bob", ReplayReason: "ops-reason",
		DiscardReason: "sensitive-discard", Payload: []byte("payload-bytes"),
	}}
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.List(context.Background(), ListQuery{TenantID: 1, Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0].ID != 99 {
		t.Fatalf("unexpected rows: %+v", res.Rows)
	}
	d := res.Rows[0]
	// 门控字段恒 nil（List 无 includePayload）。
	if d.Payload != nil || d.Headers != nil || d.RawKey != nil {
		t.Fatalf("List must not release gated fields: %+v", d)
	}
	// 白名单：Detail 的 JSON 不得含敏感字段名或其值。
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, leak := range []string{
		"fencing-token-xyz", "bearer-replay-token", "alice", "bob", "ops-reason", "sensitive-discard", "payload-bytes",
		"ClaimID", "ReplayAuthID", "ReplayRequestedBy", "ReplayApprovedBy", "ReplayReason", "DiscardReason",
	} {
		if strings.Contains(js, leak) {
			t.Fatalf("List leaked %q via Detail JSON: %s", leak, js)
		}
	}
}

func TestGetDetail_PayloadGateAndAudit(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(7)
	st.getByIDRow = store.Row{ID: 50, TenantID: 7, Payload: []byte("p"), RawKey: []byte("k")}
	aud := &fakeAuditor{}
	svc, err := NewService(r, aud)
	if err != nil {
		t.Fatal(err)
	}

	// includePayload=false：payload 字段空，不审计。
	d, err := svc.GetDetail(context.Background(), 7, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Payload != nil || d.Headers != nil || d.RawKey != nil {
		t.Fatalf("includePayload=false must gate payload: %+v", d)
	}
	if len(aud.events) != 0 {
		t.Fatalf("no audit expected when includePayload=false, got %v", aud.events)
	}

	// includePayload=true：审计在前 + 填充载荷。
	d, err = svc.GetDetail(context.Background(), 7, 50, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(d.Payload) != "p" || string(d.RawKey) != "k" {
		t.Fatalf("includePayload=true must fill gated fields: %+v", d)
	}
	if len(aud.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(aud.events))
	}
	ev := aud.events[0]
	if ev.TenantID != 7 || ev.Kind != PrivilegedAccessConsumptionPayload || ev.RowID != 50 {
		t.Fatalf("wrong audit event: %+v", ev)
	}
}

func TestGetDetail_FailClosedOnAuditError(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(7)
	st.getByIDRow = store.Row{ID: 50, TenantID: 7, Payload: []byte("secret")}
	aud := &fakeAuditor{err: errors.New("audit sink down")}
	svc, err := NewService(r, aud)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetDetail(context.Background(), 7, 50, true); err == nil {
		t.Fatal("GetDetail must fail when auditor fails (fail-closed)")
	}
	// includePayload=false 不触发审计，不应受审计失败影响。
	if _, err := svc.GetDetail(context.Background(), 7, 50, false); err != nil {
		t.Fatalf("includePayload=false must not depend on auditor: %v", err)
	}
}

func TestGetDetail_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetDetail(context.Background(), 0, 1, false); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestReplayOne_SiblingBlocked_ConflictAndNoSchedule(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(3)
	st.hasSiblingRes = true // §6.2.1 门禁命中。
	svc, capturedTx, _ := newSvcWithTxCapture(t, r)

	err := svc.ReplayOne(context.Background(), ReplayRequest{
		TenantID: 3, ID: 99, ExpectedRowVersion: 4,
		Requester: "alice", Approver: "bob", Reason: "fix",
	})
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Reason != conflictReasonSibling {
		t.Fatalf("expected ConflictError(%q), got %v", conflictReasonSibling, err)
	}
	if st.scheduleCalls != 0 {
		t.Fatal("ScheduleReplay must NOT be called when §6.2.1 sibling gate blocks")
	}
	if st.hasSiblingCalls != 1 || st.capturedHasSiblingID != 99 {
		t.Fatalf("sibling check not invoked as expected: calls=%d id=%d", st.hasSiblingCalls, st.capturedHasSiblingID)
	}
	// 两调用拿到同一 tx（即使 ScheduleReplay 未被调用，HasEarlierUnsolvedSibling 仍应在 tx 内）。
	if *capturedTx == nil {
		t.Fatal("txRunner not invoked")
	}
	if st.capturedHasSiblingDB != *capturedTx {
		t.Fatal("HasEarlierUnsolvedSibling must run inside the tx from txRunner")
	}
}

func TestReplayOne_StoreConflictD12_MappedToConflictError(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(3)
	st.scheduleErr = reliable.ErrConflict // 模拟 store 的 D12 拒绝（requester==approver 路径）。
	svc, capturedTx, _ := newSvcWithTxCapture(t, r)

	err := svc.ReplayOne(context.Background(), ReplayRequest{
		TenantID: 3, ID: 99, ExpectedRowVersion: 4,
		Requester: "alice", Approver: "alice", Reason: "self",
	})
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConflictError, got %T %v", err, err)
	}
	if ce.Reason != conflictReasonD12 {
		t.Fatalf("reason = %q, want %q (requester==approver)", ce.Reason, conflictReasonD12)
	}
	if st.hasSiblingCalls != 1 || st.scheduleCalls != 1 {
		t.Fatalf("expected sibling-check then schedule, got sibling=%d schedule=%d", st.hasSiblingCalls, st.scheduleCalls)
	}
	// 关键：两调用拿到【同一】 tx（TOCTOU 收口）。
	if st.capturedHasSiblingDB != st.capturedScheduleDB {
		t.Fatal("HasEarlierUnsolvedSibling and ScheduleReplay must share the SAME tx")
	}
	if st.capturedHasSiblingDB != *capturedTx {
		t.Fatal("shared tx must equal the txRunner-provided tx")
	}
}

func TestReplayOne_StoreConflictRowVersion_MappedToConflictError(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(3)
	st.scheduleErr = reliable.ErrConflict // 这次 requester≠approver → 只剩 row_version mismatch 路径。
	svc, _, _ := newSvcWithTxCapture(t, r)

	err := svc.ReplayOne(context.Background(), ReplayRequest{
		TenantID: 3, ID: 99, ExpectedRowVersion: 4,
		Requester: "alice", Approver: "bob", Reason: "fix",
	})
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Reason != conflictReasonRowVersion {
		t.Fatalf("expected ConflictError(%q), got %v", conflictReasonRowVersion, err)
	}
}

func TestReplayOne_HappyPath(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(3)
	svc, _, _ := newSvcWithTxCapture(t, r)

	err := svc.ReplayOne(context.Background(), ReplayRequest{
		TenantID: 3, ID: 99, ExpectedRowVersion: 4,
		Requester: "alice", Approver: "bob", Reason: "fix",
	})
	if err != nil {
		t.Fatalf("happy path: %v", err)
	}
	want := scheduleCall{id: 99, ver: 4, requester: "alice", approver: "bob", reason: "fix"}
	if st.capturedSchedule != want {
		t.Fatalf("schedule args = %+v, want %+v", st.capturedSchedule, want)
	}
}

func TestReplayOne_HasSiblingError_Propagates(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(3)
	dbErr := errors.New("db down")
	st.hasSiblingErr = dbErr
	svc, _, _ := newSvcWithTxCapture(t, r)
	err := svc.ReplayOne(context.Background(), ReplayRequest{TenantID: 3, ID: 9, Requester: "a", Approver: "b"})
	if !errors.Is(err, dbErr) {
		t.Fatalf("underlying db error must propagate, got %v", err)
	}
	if st.scheduleCalls != 0 {
		t.Fatal("ScheduleReplay must not be called after sibling-check error")
	}
}

func TestReplayOne_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReplayOne(context.Background(), ReplayRequest{}); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestBatchReplay_PerRowResults_NoCrossRowTx(t *testing.T) {
	r := newFakeResolver()
	// dispatchStore 按 id 分发不同结果（ok / sibling 冲突 / D12 / row_version mismatch）。
	df := &dispatchStore{tenantID: 5}
	r.stores[5] = df
	r.dbs[5] = &gorm.DB{}
	svc, _, _ := newSvcWithTxCapture(t, r)
	// 覆写 svc.txRunner 让它计数 per-row 调用（验证「无跨行 tx」）。
	perRowTx := 0
	svc.txRunner = func(db *gorm.DB, fn func(tx *gorm.DB) error) error {
		perRowTx++
		tx := &gorm.DB{} // 每行一个新的 sentinel tx（与 db 不同）。
		return fn(tx)
	}

	res, err := svc.BatchReplay(context.Background(), BatchReplayRequest{TenantID: 5, Items: []BatchReplayItem{
		{ID: 11, ExpectedRowVersion: 1, Requester: "a", Approver: "b", Reason: "ok"},
		{ID: 12, ExpectedRowVersion: 1, Requester: "a", Approver: "b"}, // sibling 冲突
		{ID: 13, ExpectedRowVersion: 1, Requester: "x", Approver: "x"}, // D12
		{ID: 14, ExpectedRowVersion: 1, Requester: "a", Approver: "b"}, // row_version mismatch
		{ID: 15, ExpectedRowVersion: 1, Requester: "a", Approver: "b"}, // 硬错误（非冲突）
	}})
	if err != nil {
		t.Fatalf("BatchReplay top-level error: %v", err)
	}
	if len(res.Results) != 5 {
		t.Fatalf("want 5 results, got %d", len(res.Results))
	}
	expect := []struct {
		id       int64
		ok       bool
		conflict bool
		reason   string // 期望 ConflictReason（仅 Conflict 行有意义）
	}{
		{11, true, false, ""},
		{12, false, true, conflictReasonSibling},
		{13, false, true, conflictReasonD12},
		{14, false, true, conflictReasonRowVersion},
		{15, false, false, ""},
	}
	for i, e := range expect {
		got := res.Results[i]
		if got.ID != e.id || got.Ok != e.ok || got.Conflict != e.conflict {
			t.Fatalf("result[%d] = %+v, want %+v", i, got, e)
		}
		if e.conflict && got.ConflictReason != e.reason {
			t.Fatalf("result[%d] ConflictReason = %q, want %q", i, got.ConflictReason, e.reason)
		}
	}
	// P2-2：三态互斥（dto 约定）——Ok/Conflict/Err 互斥。
	// 成功行（Conflict=false, Err=""）；冲突行（不填 Err，handler 凭 Conflict 优先判定）；
	// 硬错误行（Conflict=false 且 Err 非空）。
	for i, got := range res.Results {
		switch {
		case got.Ok:
			if got.Conflict || got.Err != "" {
				t.Fatalf("result[%d] Ok row must have Conflict=false and Err empty: %+v", i, got)
			}
		case got.Conflict:
			if got.Err != "" {
				t.Fatalf("result[%d] Conflict row must NOT populate Err (dto 三态互斥): %+v", i, got)
			}
		default:
			if got.Err == "" {
				t.Fatalf("result[%d] hard-error row must populate Err: %+v", i, got)
			}
		}
	}
	// 无跨行 tx：每行独立一次 txRunner（5 次），每次只包一行。
	if perRowTx != 5 {
		t.Fatalf("txRunner must be invoked once per row (no cross-row tx): got %d", perRowTx)
	}
}

// dispatchStore 是 id-driven 的 BatchReplay 测试 fake：按 id/requester 分发不同结果。
type dispatchStore struct {
	store.Store
	tenantID     int
	lastSchedule scheduleCall
}

func (d *dispatchStore) HasEarlierUnsolvedSibling(ctx context.Context, db *gorm.DB, id int64) (bool, error) {
	return id == 12, nil // id=12 命中 §6.2.1 门禁。
}
func (d *dispatchStore) ScheduleReplay(ctx context.Context, db *gorm.DB, id, ver int64, req, appr, reason string) error {
	d.lastSchedule = scheduleCall{id, ver, req, appr, reason}
	if req == appr {
		return reliable.ErrConflict // D12
	}
	if id == 14 {
		return reliable.ErrConflict // row_version mismatch
	}
	if id == 15 {
		return errors.New("boom: transient db error") // 硬错误（非 ConflictError）→ 走 else 分支填 Err
	}
	return nil
}

func TestBatchReplay_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BatchReplay(context.Background(), BatchReplayRequest{}); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestDiscard_CASConflict_MappedToConflictError(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(8)
	st.discardErr = reliable.ErrConflict // stale row_version.
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Discard(context.Background(), DiscardRequest{
		TenantID: 8, ID: 77, ExpectedRowVersion: 2, By: "ops", Reason: "noise",
	})
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Reason != conflictReasonRowVersion {
		t.Fatalf("expected ConflictError(row_version mismatch), got %v", err)
	}
	if st.capturedDiscardID != 77 || st.capturedDiscardVer != 2 || st.capturedDiscardBy != "ops" || st.capturedDiscardReason != "noise" {
		t.Fatalf("discard args wrong: %+v", st.capturedSchedule)
	}
}

func TestDiscard_HappyPath(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(8)
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Discard(context.Background(), DiscardRequest{TenantID: 8, ID: 77, ExpectedRowVersion: 2, By: "o", Reason: "r"}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if st.discardCalls != 1 {
		t.Fatalf("discard calls = %d", st.discardCalls)
	}
}

func TestDiscard_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Discard(context.Background(), DiscardRequest{}); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestStats_UsesCountNotList(t *testing.T) {
	r := newFakeResolver()
	st, _, _ := r.addTenant(9)
	st.countRes = 3
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := svc.Stats(context.Background(), ListQuery{TenantID: 9})
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	// Count 按五态各一次 → 5 次；List 必须 0 次（F6）。
	if st.countCalls != 5 {
		t.Fatalf("expected Count called once per status (5), got %d", st.countCalls)
	}
	if st.listCalls != 0 {
		t.Fatal("Stats must NOT use List (F6: no list-then-count)")
	}
	// 每个状态 3 → Total = 15；ByStatus 五键各 3。
	if stats.Total != 15 {
		t.Fatalf("Total = %d, want 15", stats.Total)
	}
	if len(stats.ByStatus) != 5 {
		t.Fatalf("ByStatus len = %d, want 5", len(stats.ByStatus))
	}
	for _, v := range stats.ByStatus {
		if v != 3 {
			t.Fatalf("ByStatus value = %d, want 3", v)
		}
	}
	// 每次调用都带 tenant 作用域。
	for _, f := range st.capturedCountFilters {
		if f.TenantID != 9 {
			t.Fatalf("Count filter tenant = %d, want 9", f.TenantID)
		}
	}
}

func TestStats_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Stats(context.Background(), ListQuery{}); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestQuarantineList_RawGateAndBulkAudit(t *testing.T) {
	r := newFakeResolver()
	_, qs, _ := r.addTenant(11)
	qs.listRows = []store.QuarantineRow{
		{ID: 1, TenantID: 11, RawValue: []byte("poison1"), RawKey: []byte("k1")},
		{ID: 2, TenantID: 11, RawValue: []byte("poison2")},
	}
	aud := &fakeAuditor{}
	svc, err := NewService(r, aud)
	if err != nil {
		t.Fatal(err)
	}

	// includeRaw=false：每个元素 raw 字段清零，不审计。
	out, err := svc.QuarantineList(context.Background(), 11, "QUARANTINED", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 rows, got %d", len(out))
	}
	for i, d := range out {
		if d.RawValue != nil || d.RawKey != nil || d.Headers != nil {
			t.Fatalf("row[%d] raw fields must be zero when includeRaw=false: %+v", i, d)
		}
	}
	if len(aud.events) != 0 {
		t.Fatalf("no audit expected when includeRaw=false, got %v", aud.events)
	}

	// includeRaw=true：raw 填充 + 至少一条 bulk 审计事件（RowID=0）。
	out, err = svc.QuarantineList(context.Background(), 11, "QUARANTINED", 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(out[0].RawValue) != "poison1" || string(out[1].RawValue) != "poison2" {
		t.Fatalf("raw not filled: %+v", out)
	}
	if len(aud.events) != 1 {
		t.Fatalf("expected 1 bulk audit event, got %d", len(aud.events))
	}
	ev := aud.events[0]
	if ev.TenantID != 11 || ev.Kind != PrivilegedAccessQuarantineRaw || ev.RowID != 0 {
		t.Fatalf("wrong bulk audit event: %+v", ev)
	}
}

func TestQuarantineList_FailClosedOnAuditError(t *testing.T) {
	r := newFakeResolver()
	_, qs, _ := r.addTenant(11)
	qs.listRows = []store.QuarantineRow{{ID: 1, TenantID: 11, RawValue: []byte("p")}}
	aud := &fakeAuditor{err: errors.New("audit down")}
	svc, err := NewService(r, aud)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QuarantineList(context.Background(), 11, "", 10, true); err == nil {
		t.Fatal("must fail-closed when auditor fails on includeRaw=true")
	}
}

func TestQuarantineList_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QuarantineList(context.Background(), 0, "", 10, false); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestQuarantineDetail_RawGateAndAudit(t *testing.T) {
	r := newFakeResolver()
	_, qs, _ := r.addTenant(13)
	qs.getByIDRow = store.QuarantineRow{ID: 90, TenantID: 13, RawValue: []byte("raw"), RawKey: []byte("k")}
	aud := &fakeAuditor{}
	svc, err := NewService(r, aud)
	if err != nil {
		t.Fatal(err)
	}

	// includeRaw=false：raw 清零。
	d, err := svc.QuarantineDetail(context.Background(), 13, 90, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.RawValue != nil || d.RawKey != nil || d.Headers != nil {
		t.Fatalf("includeRaw=false must gate raw: %+v", d)
	}
	if len(aud.events) != 0 {
		t.Fatalf("no audit expected, got %v", aud.events)
	}

	// includeRaw=true：审计 + 填充。
	d, err = svc.QuarantineDetail(context.Background(), 13, 90, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(d.RawValue) != "raw" || string(d.RawKey) != "k" {
		t.Fatalf("raw not filled: %+v", d)
	}
	if len(aud.events) != 1 || aud.events[0].RowID != 90 || aud.events[0].Kind != PrivilegedAccessQuarantineRaw {
		t.Fatalf("wrong audit: %+v", aud.events)
	}
}

func TestQuarantineDetail_FailClosedOnAuditError(t *testing.T) {
	r := newFakeResolver()
	_, qs, _ := r.addTenant(13)
	qs.getByIDRow = store.QuarantineRow{ID: 90, TenantID: 13, RawValue: []byte("secret")}
	aud := &fakeAuditor{err: errors.New("audit down")}
	svc, err := NewService(r, aud)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QuarantineDetail(context.Background(), 13, 90, true); err == nil {
		t.Fatal("must fail-closed when auditor fails")
	}
}

func TestQuarantineDetail_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.QuarantineDetail(context.Background(), 0, 1, false); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestQuarantineResolve_CASConflict_MappedToConflictError(t *testing.T) {
	r := newFakeResolver()
	_, qs, db := r.addTenant(15)
	qs.resolveErr = reliable.ErrConflict
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.QuarantineResolve(context.Background(), ResolveRequest{
		TenantID: 15, ID: 200, ExpectedRowVersion: 7, By: "ops",
	})
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Reason != conflictReasonRowVersion {
		t.Fatalf("expected ConflictError(row_version mismatch), got %v", err)
	}
	// db 来自 resolver.Store()（QuarantineStore() 不带 db）。
	if qs.capturedResolveDB != db {
		t.Fatalf("MarkResolved must receive the per-tenant db from resolver.Store()")
	}
	if qs.capturedResolveID != 200 || qs.capturedResolveVer != 7 || qs.capturedResolveBy != "ops" {
		t.Fatalf("resolve args wrong: id=%d ver=%d by=%s", qs.capturedResolveID, qs.capturedResolveVer, qs.capturedResolveBy)
	}
}

func TestQuarantineResolve_HappyPath(t *testing.T) {
	r := newFakeResolver()
	_, qs, _ := r.addTenant(15)
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.QuarantineResolve(context.Background(), ResolveRequest{TenantID: 15, ID: 200, ExpectedRowVersion: 7, By: "o"}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if qs.resolveCalls != 1 {
		t.Fatalf("resolve calls = %d", qs.resolveCalls)
	}
}

func TestQuarantineResolve_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.QuarantineResolve(context.Background(), ResolveRequest{}); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestAnomalies_DelegatesAndTenantScoped(t *testing.T) {
	r := newFakeResolver()
	st1, _, _ := r.addTenant(20)
	st2, _, _ := r.addTenant(21)
	st1.anomalyRows = []store.AnomalyRow{{ID: 1, TenantID: 20, Kind: "LEASE_ORPHAN"}}
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := svc.Anomalies(context.Background(), AnomalyQuery{TenantID: 20, Kind: "LEASE_ORPHAN", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TenantID != 20 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if st1.capturedAnomalyFilter.TenantID != 20 || st1.capturedAnomalyFilter.Kind != "LEASE_ORPHAN" {
		t.Fatalf("filter wrong: %+v", st1.capturedAnomalyFilter)
	}
	assertOnlyTouched(t, st1, st2, "Anomalies")
}

func TestAnomalies_RejectsMissingTenant(t *testing.T) {
	svc, err := NewService(newFakeResolver(), &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Anomalies(context.Background(), AnomalyQuery{}); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}

func TestResolverError_Propagates(t *testing.T) {
	r := newFakeResolver()
	r.storeErr[30] = errors.New("tenant 30 not served")
	svc, err := NewService(r, &fakeAuditor{})
	if err != nil {
		t.Fatal(err)
	}
	// 所有走 resolver.Store 的方法都应原样上抛 resolver 错误。
	if _, err := svc.List(context.Background(), ListQuery{TenantID: 30}); err == nil {
		t.Fatal("List must surface resolver error")
	}
	if err := svc.ReplayOne(context.Background(), ReplayRequest{TenantID: 30, ID: 1}); err == nil {
		t.Fatal("ReplayOne must surface resolver error")
	}
}

// —— QuarantineReplay（PR-7 Task 3，C①；OV⑤=8A「毕业」流程）——
//
// fake 策略（ambiguity resolution #3：扩展现有 DB-less fake，不引真实 DB driver）：
// fakeStore 增加 TryClaim/MarkFailed 的行为控制与入参捕获；三个隔离区 CAS（qrClaim/qrBack/
// qrResolve）覆写为 in-memory 状态机（记录迁移序列 + 可注入的失败）。真实 SQL 谓词由
// system-tagged 测试（store/gormshared，repotest 真库）钉住，本套只钉编排语义。

// qrTransition 记录一次 CAS 迁移（断言迁移序列用）。
type qrTransition struct {
	kind       string // "claim" | "back" | "resolve"
	id         int64
	tenantID   int
	rowVer     int64 // 期望的 CAS 版本（claim: ExpectedRowVersion；back/resolve: replayRowVer+1）
	causeOrDtl string
	by         string
}

// qrRowState 是 fakeQrState 持有的隔离行状态。
type qrRowState struct {
	status         string
	rowVersion     int64
	replayAttempts int
	errMsg         string
	updatedAt      time.Time
}

// fakeQrState 是三个 CAS seam 的 in-memory 实现：按 (id, tenant) 持一行状态。
type fakeQrState struct {
	rows map[[2]int64]*qrRowState // key: {id, tenantID}
	log  []qrTransition
	// claimErrs 排队消费（每次 claim 取一条，nil = 走正常谓词）。
	claimErrs []error
}

func newFakeQrState() *fakeQrState {
	return &fakeQrState{rows: map[[2]int64]*qrRowState{}}
}

func (f *fakeQrState) seed(id int64, tenantID int, status string, rowVersion int64, replayAttempts int, updatedAt time.Time) {
	f.rows[[2]int64{id, int64(tenantID)}] = &qrRowState{
		status: status, rowVersion: rowVersion, replayAttempts: replayAttempts, updatedAt: updatedAt,
	}
}

func (f *fakeQrState) row(id int64, tenantID int) (qrRowState, bool) {
	r, ok := f.rows[[2]int64{id, int64(tenantID)}]
	if !ok {
		return qrRowState{}, false
	}
	return *r, true
}

// claim 模拟 casToReplaying 的谓词：(QUARANTINED) OR (REPLAYING AND updatedAt<watchdog)。
func (f *fakeQrState) claim(ctx context.Context, db *gorm.DB, id int64, tenantID int, expectedVersion int64, watchdogCutoff time.Time) error {
	f.log = append(f.log, qrTransition{kind: "claim", id: id, tenantID: tenantID, rowVer: expectedVersion})
	if len(f.claimErrs) > 0 {
		err := f.claimErrs[0]
		f.claimErrs = f.claimErrs[1:]
		if err != nil {
			return err
		}
	}
	r, ok := f.rows[[2]int64{id, int64(tenantID)}]
	if !ok || r.rowVersion != expectedVersion {
		return &ConflictError{Reason: conflictReasonRowVersion}
	}
	if !(r.status == quarantineStatusQuarantined ||
		(r.status == quarantineStatusReplaying && r.updatedAt.Before(watchdogCutoff))) {
		return &ConflictError{Reason: conflictReasonRowVersion}
	}
	r.status = quarantineStatusReplaying
	r.rowVersion++
	r.updatedAt = time.Now().UTC()
	return nil
}

// back 模拟 casBackToQuarantined：replayRowVer+1 命中 REPLAYING 行 → QUARANTINED + 计数。
func (f *fakeQrState) back(ctx context.Context, db *gorm.DB, id int64, tenantID int, replayRowVer int64, cause error) error {
	f.log = append(f.log, qrTransition{kind: "back", id: id, tenantID: tenantID, rowVer: replayRowVer + 1, causeOrDtl: fmt.Sprintf("%v", cause)})
	r, ok := f.rows[[2]int64{id, int64(tenantID)}]
	if !ok || r.status != quarantineStatusReplaying || r.rowVersion != replayRowVer+1 {
		return nil // 并发迁移：幂等 nil（与生产语义一致）。
	}
	r.status = quarantineStatusQuarantined
	r.rowVersion++
	r.replayAttempts++
	r.errMsg = reliable.SanitizeForStorage(fmt.Sprintf("%v", cause))
	r.updatedAt = time.Now().UTC()
	return nil
}

// resolve 模拟 casToResolved。review I-3：不再有 detail 参数——生产实现不写 error_message
// （原始 quarantine cause 保留），fake 同步：errMsg 不被 resolve 触碰。
func (f *fakeQrState) resolve(ctx context.Context, db *gorm.DB, id int64, tenantID int, replayRowVer int64, by string) error {
	f.log = append(f.log, qrTransition{kind: "resolve", id: id, tenantID: tenantID, rowVer: replayRowVer + 1, by: by})
	r, ok := f.rows[[2]int64{id, int64(tenantID)}]
	if !ok || r.status != quarantineStatusReplaying || r.rowVersion != replayRowVer+1 {
		return nil // 并发迁移：幂等 nil。
	}
	r.status = quarantineStatusResolved
	r.rowVersion++
	r.updatedAt = time.Now().UTC()
	return nil
}

// fakeRegistry 实现 replay.HandlerRegistry。handler 用 recordingHandler 包一层——
// 「绝不直接调 Handler.Handle」（OV⑤）的断言钩子。
type fakeRegistry struct {
	handlers  map[reliable.HandlerID]replay.HandlerInfo
	handleCnt map[reliable.HandlerID]int
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{
		handlers:  map[reliable.HandlerID]replay.HandlerInfo{},
		handleCnt: map[reliable.HandlerID]int{},
	}
}

func (fr *fakeRegistry) register(id reliable.HandlerID, safety reliable.ReplaySafety) {
	fr.handlers[id] = replay.HandlerInfo{
		HandlerID: id, ReplaySafety: safety,
		Handler: &recordingHandler{reg: fr, id: id},
	}
}

func (fr *fakeRegistry) Lookup(id reliable.HandlerID) (replay.HandlerInfo, bool) {
	info, ok := fr.handlers[id]
	return info, ok
}

func (fr *fakeRegistry) All() []replay.HandlerInfo {
	out := make([]replay.HandlerInfo, 0, len(fr.handlers))
	for _, v := range fr.handlers {
		out = append(out, v)
	}
	return out
}

type recordingHandler struct {
	reg *fakeRegistry
	id  reliable.HandlerID
}

func (h *recordingHandler) Handle(ctx context.Context, envelopeBytes []byte, delivery reliable.DeliveryMeta) error {
	h.reg.handleCnt[h.id]++
	return nil
}
func (h *recordingHandler) HandlerID() reliable.HandlerID { return h.id }
func (h *recordingHandler) ReplaySafety() reliable.ReplaySafety {
	return h.reg.handlers[h.id].ReplaySafety
}
func (h *recordingHandler) RequiresAggregateGate() bool { return false }

// stubDecoder 是可控的 EnvelopeDecoder fake。
type stubDecoder struct {
	key      reliable.Key
	meta     reliable.Meta
	tenantID int
	err      error
	calls    int
}

func (d *stubDecoder) decode(raw []byte) (reliable.Key, reliable.Meta, int, error) {
	d.calls++
	return d.key, d.meta, d.tenantID, d.err
}

// qrHarness 装配 QuarantineReplay 单测的全套 fake。租户 40 / 行 id 300 / handler
// "media.v1" / row_version 1（QUARANTINED）。
func newQrHarness(t *testing.T) *qrHarness {
	t.Helper()
	r := newFakeResolver()
	st, qs, db := r.addTenant(40)
	qs.getByIDRow = store.QuarantineRow{
		ID: 300, TenantID: 40, HandlerID: "media.v1", Topic: "domain.media",
		SrcPartition: 3, SrcOffset: 77, RawValue: []byte("env-bytes"), RawKey: []byte("rk"),
		Headers: []reliable.HeaderPair{{Key: "h", Value: []byte("v")}}, RawPayloadHash: "hash-x",
		Status: "QUARANTINED", RowVersion: 1,
	}
	reg := newFakeRegistry()
	reg.register("media.v1", reliable.ReplayIdempotent)
	dec := &stubDecoder{
		key:      reliable.Key{EventID: "ev-1", Handler: "media.v1"},
		meta:     reliable.Meta{EventType: "FileUploaded", AggregateType: "Media", AggregateID: "m1"},
		tenantID: 40,
	}
	qst := newFakeQrState()
	qst.seed(300, 40, "QUARANTINED", 1, 0, time.Now().UTC())
	svc, err := NewService(r, &fakeAuditor{}, WithRegistry(reg), WithEnvelopeDecoder(dec.decode))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_ = db
	svc.qrClaim = qst.claim
	svc.qrBack = qst.back
	svc.qrResolve = qst.resolve
	return &qrHarness{svc: svc, res: r, store: st, qstate: qst, reg: reg, dec: dec, handler: "media.v1"}
}

type qrHarness struct {
	svc     *Service
	res     *fakeResolver
	store   *fakeStore
	qstate  *fakeQrState
	reg     *fakeRegistry
	dec     *stubDecoder
	handler reliable.HandlerID
}

func qrReq() QuarantineReplayRequest {
	return QuarantineReplayRequest{TenantID: 40, ID: 300, ExpectedRowVersion: 1, By: "ops-alice"}
}

// TestQuarantineReplay_RequiresRegistry：缺 WithRegistry / 缺 WithEnvelopeDecoder（任一）→
// ErrQuarantineReplayUnsupported，且不触碰任何 store（fail-closed 在解析 store 之前）。
func TestQuarantineReplay_RequiresRegistry(t *testing.T) {
	r := newFakeResolver()
	st, qs, _ := r.addTenant(40)
	qs.getByIDRow = store.QuarantineRow{ID: 300, TenantID: 40, HandlerID: "media.v1", Status: "QUARANTINED"}
	aud := &fakeAuditor{}
	dec := &stubDecoder{key: reliable.Key{EventID: "ev-1", Handler: "media.v1"}, tenantID: 40}

	svcNone, err := NewService(r, aud)
	if err != nil {
		t.Fatal(err)
	}
	svcDec, err := NewService(r, aud, WithEnvelopeDecoder(dec.decode))
	if err != nil {
		t.Fatal(err)
	}
	reg := newFakeRegistry()
	reg.register("media.v1", reliable.ReplayIdempotent)
	svcReg, err := NewService(r, aud, WithRegistry(reg))
	if err != nil {
		t.Fatal(err)
	}
	for i, svc := range []*Service{svcNone, svcDec, svcReg} {
		if err := svc.QuarantineReplay(context.Background(), qrReq()); !errors.Is(err, ErrQuarantineReplayUnsupported) {
			t.Fatalf("case %d: expected ErrQuarantineReplayUnsupported, got %v", i, err)
		}
	}
	if st.getByIDCalls != 0 || qs.getByIDCalls != 0 {
		t.Fatal("fail-closed must reject BEFORE touching any store")
	}
}

// TestQuarantineReplay_HappyPath（毕业）：TryClaim→Claimed，MarkFailed 把 envelope 字节按
// §6.1 矩阵落 RETRY_SCHEDULED；隔离行 CAS REPLAYING→RESOLVED（resolved_by=r.By）。
// 断言 TryClaim 收到解码的 Key/TenantID 与行内重建的 Delivery；且【registry handler 从未被
// 调用】——业务执行属于 scheduler，不属于 opsvc（OV⑤=8A 的核心断言）。
func TestQuarantineReplay_HappyPath(t *testing.T) {
	h := newQrHarness(t)
	h.store.tryClaimDecision = reliable.Claimed
	h.store.tryClaimToken = "tok-1"

	if err := h.svc.QuarantineReplay(context.Background(), qrReq()); err != nil {
		t.Fatalf("happy path: %v", err)
	}

	// TryClaim 入参：Key 的 Handler 来自【行】（row.HandlerID），EventID 来自解码；ItemKey 恒空。
	if h.store.tryClaimCalls != 1 {
		t.Fatalf("TryClaim calls = %d", h.store.tryClaimCalls)
	}
	in := h.store.capturedClaimInput
	if in.Key.Handler != "media.v1" || in.Key.EventID != "ev-1" || in.Key.ItemKey != "" {
		t.Fatalf("TryClaim Key wrong: %+v", in.Key)
	}
	if in.TenantID != 40 {
		t.Fatalf("TryClaim TenantID = %d, want 40", in.TenantID)
	}
	// Delivery 由行内 Raw* 重建（非 envelope）。
	if in.Delivery.Topic != "domain.media" || in.Delivery.Partition != 3 || in.Delivery.Offset != 77 {
		t.Fatalf("Delivery broker identity wrong: %+v", in.Delivery)
	}
	if in.Delivery.PayloadHash != "hash-x" || string(in.Delivery.RawKey) != "rk" {
		t.Fatalf("Delivery integrity fields wrong: %+v", in.Delivery)
	}
	// MarkFailed：payload=RawValue（envelope 字节持久化），safety 来自 registry。
	if h.store.markFailedCalls != 1 {
		t.Fatalf("MarkFailed calls = %d", h.store.markFailedCalls)
	}
	if string(h.store.capturedMarkFailedPayload) != "env-bytes" {
		t.Fatalf("MarkFailed payload must be the envelope bytes, got %q", h.store.capturedMarkFailedPayload)
	}
	if h.store.capturedMarkFailedSafety != reliable.ReplayIdempotent {
		t.Fatalf("MarkFailed safety = %v, want ReplayIdempotent (from registry)", h.store.capturedMarkFailedSafety)
	}
	// M-2：钉住毕业偏离的承重参数——class 必须 ClassRetryable（scheduler 正常驱动的入口；
	// 改成 ClassPoison 会让行直落死信、绕过 §6.2 重试矩阵）+ maxAttempts 必须
	// DefaultMaxAttempts（与消费侧 LIVE 路径同一防线）+ cause 必须是毕业标记 sentinel。
	if h.store.capturedMarkFailedClass != reliable.ClassRetryable {
		t.Fatalf("MarkFailed class = %v, want ClassRetryable (manual re-drive, not a business failure)", h.store.capturedMarkFailedClass)
	}
	if h.store.capturedMarkFailedMaxAtt != reliable.DefaultMaxAttempts {
		t.Fatalf("MarkFailed maxAttempts = %d, want DefaultMaxAttempts (%d)", h.store.capturedMarkFailedMaxAtt, reliable.DefaultMaxAttempts)
	}
	if !errors.Is(h.store.capturedMarkFailedCause, errQuarantineGraduated) {
		t.Fatalf("MarkFailed cause = %v, want errQuarantineGraduated", h.store.capturedMarkFailedCause)
	}
	// 隔离行 → RESOLVED。
	row, ok := h.qstate.row(300, 40)
	if !ok || row.status != "RESOLVED" {
		t.Fatalf("quarantine row must end RESOLVED, got %+v", row)
	}
	// OV⑤ 核心：handler 从未被调用。
	if h.reg.handleCnt[h.handler] != 0 {
		t.Fatalf("Handler.Handle must NEVER be called by QuarantineReplay (got %d calls)", h.reg.handleCnt[h.handler])
	}
	// 成功不计数。
	if row.replayAttempts != 0 {
		t.Fatalf("successful graduation must not increment replay_attempts, got %d", row.replayAttempts)
	}
}

// TestQuarantineReplay_AlreadySettled：TryClaim→AlreadySettled → 行 RESOLVED（幂等成功），
// 不调 MarkFailed、不计数。
func TestQuarantineReplay_AlreadySettled(t *testing.T) {
	h := newQrHarness(t)
	h.store.tryClaimDecision = reliable.AlreadySettled

	if err := h.svc.QuarantineReplay(context.Background(), qrReq()); err != nil {
		t.Fatalf("AlreadySettled must be idempotent success, got %v", err)
	}
	if h.store.markFailedCalls != 0 {
		t.Fatal("MarkFailed must not run on AlreadySettled")
	}
	row, _ := h.qstate.row(300, 40)
	if row.status != "RESOLVED" || row.replayAttempts != 0 {
		t.Fatalf("want RESOLVED/0 attempts, got %+v", row)
	}
	// review I-3：RESOLVED 迁移不再持久化区分性 detail（生产 casToResolved 不写
	// error_message，原始 quarantine cause 保留）。fake 的 resolve 同步不触碰 errMsg——
	// 种子行未经 back 迁移、errMsg 保持初始空值；「毕业 vs 早已结算」无 DB 侧标记。
	// 真实库上的因果保留断言由 quarantine_replay_system_test.go 承载。
	if strings.Contains(row.errMsg, "already settled") {
		t.Fatalf("RESOLVED must NOT persist graduation/settlement detail into error_message (I-3), got %q", row.errMsg)
	}
}

// TestQuarantineReplay_AlreadyProcessing：TryClaim→AlreadyProcessing → 行回 QUARANTINED
// （row_version+1、replay_attempts+1、error_message 固定文案）+ *ConflictError{live claim active}。
func TestQuarantineReplay_AlreadyProcessing(t *testing.T) {
	h := newQrHarness(t)
	h.store.tryClaimDecision = reliable.AlreadyProcessing

	err := h.svc.QuarantineReplay(context.Background(), qrReq())
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Reason != conflictReasonLiveClaim {
		t.Fatalf("expected ConflictError(live claim active), got %v", err)
	}
	row, _ := h.qstate.row(300, 40)
	if row.status != "QUARANTINED" || row.rowVersion != 3 || row.replayAttempts != 1 {
		t.Fatalf("row must be back to QUARANTINED ver=3 attempts=1, got %+v", row)
	}
	if !strings.Contains(row.errMsg, "live consumer holds the lease") {
		t.Fatalf("error_message must carry the lease hint, got %q", row.errMsg)
	}
	if h.store.markFailedCalls != 0 {
		t.Fatal("MarkFailed must not run on AlreadyProcessing")
	}
}

// TestQuarantineReplay_TryClaimError：TryClaim 出错 → 行回 QUARANTINED + 计数，错误上抛
// （非 ConflictError）。
func TestQuarantineReplay_TryClaimError(t *testing.T) {
	h := newQrHarness(t)
	dbErr := errors.New("db conn reset")
	h.store.tryClaimErr = dbErr

	err := h.svc.QuarantineReplay(context.Background(), qrReq())
	if !errors.Is(err, dbErr) {
		t.Fatalf("underlying TryClaim error must propagate, got %v", err)
	}
	var ce *ConflictError
	if errors.As(err, &ce) {
		t.Fatalf("hard error must not be a ConflictError, got %v", err)
	}
	row, _ := h.qstate.row(300, 40)
	if row.status != "QUARANTINED" || row.replayAttempts != 1 {
		t.Fatalf("row must be back QUARANTINED with attempts=1, got %+v", row)
	}
}

// TestQuarantineReplay_VersionConflict：qrClaim CAS 0 行（版本不符 / 非 QUARANTINED 亦非
// 超时 REPLAYING）→ *ConflictError，且不解码、不 TryClaim。
func TestQuarantineReplay_VersionConflict(t *testing.T) {
	h := newQrHarness(t)
	h.qstate.claimErrs = []error{&ConflictError{Reason: conflictReasonRowVersion}}

	err := h.svc.QuarantineReplay(context.Background(), qrReq())
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Reason != conflictReasonRowVersion {
		t.Fatalf("expected ConflictError(row_version mismatch), got %v", err)
	}
	if h.dec.calls != 0 || h.store.tryClaimCalls != 0 {
		t.Fatal("CAS failure must short-circuit before decode/TryClaim")
	}
}

// TestQuarantineReplay_UnknownHandler：行 HandlerID 不在 registry → 错误（行不动），
// 不 CAS、不解码、不 TryClaim。
func TestQuarantineReplay_UnknownHandler(t *testing.T) {
	h := newQrHarness(t)
	h.res.qstores[40].(*fakeQuarantineStore).getByIDRow = store.QuarantineRow{
		ID: 300, TenantID: 40, HandlerID: "ghost.v9", Status: "QUARANTINED", RowVersion: 1,
		RawValue: []byte("env-bytes"),
	}

	err := h.svc.QuarantineReplay(context.Background(), qrReq())
	if err == nil {
		t.Fatal("unknown handler must error")
	}
	var ce *ConflictError
	if errors.As(err, &ce) {
		t.Fatalf("unknown-handler is a 400-class error, not a ConflictError: %v", err)
	}
	if !strings.Contains(err.Error(), "ghost.v9") {
		t.Fatalf("error must name the unknown handler, got %v", err)
	}
	if len(h.qstate.log) != 0 || h.store.tryClaimCalls != 0 || h.dec.calls != 0 {
		t.Fatal("row must be untouched on unknown handler (no CAS, no decode, no TryClaim)")
	}
}

// TestQuarantineReplay_MaxAttempts：replay_attempts 已达 QuarantineReplayMaxAttempts →
// *ConflictError{max replay attempts}，行不动（不 CAS、不 TryClaim）。上限检查在
// QUARANTINED→REPLAYING CAS 之前（controller ambiguity #1）。
func TestQuarantineReplay_MaxAttempts(t *testing.T) {
	h := newQrHarness(t)
	h.res.qstores[40].(*fakeQuarantineStore).getByIDRow.ReplayAttempts = QuarantineReplayMaxAttempts

	err := h.svc.QuarantineReplay(context.Background(), qrReq())
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Reason != conflictReasonMaxReplayAtt {
		t.Fatalf("expected ConflictError(max replay attempts), got %v", err)
	}
	if len(h.qstate.log) != 0 || h.store.tryClaimCalls != 0 {
		t.Fatal("cap check must precede the QUARANTINED→REPLAYING CAS (row untouched)")
	}
}

// TestQuarantineReplay_Watchdog（OV⑤④）：REPLAYING 且 updated_at 早于 watchdog 截止 →
// 可重claim（崩溃残留自愈）；REPLAYING 且 updated_at 新鲜 → 0 行 Conflict。
// 注意：fake 的 qs.getByIDRow.RowVersion 与 qstate 行版本须同步——生产中两者本就同一行。
func TestQuarantineReplay_Watchdog(t *testing.T) {
	h := newQrHarness(t)
	h.store.tryClaimDecision = reliable.AlreadySettled
	stale := h.qstate.rows[[2]int64{300, 40}]
	stale.status = "REPLAYING"
	stale.updatedAt = time.Now().UTC().Add(-11 * time.Minute) // > 10min watchdog
	stale.rowVersion = 2                                      // ExpectedRowVersion 随之。
	h.res.qstores[40].(*fakeQuarantineStore).getByIDRow.RowVersion = 2
	req := qrReq()
	req.ExpectedRowVersion = 2

	if err := h.svc.QuarantineReplay(context.Background(), req); err != nil {
		t.Fatalf("stale REPLAYING row must be re-claimeable: %v", err)
	}
	row, _ := h.qstate.row(300, 40)
	if row.status != "RESOLVED" {
		t.Fatalf("re-claimed row must graduate, got %+v", row)
	}

	// 反例：新鲜 REPLAYING（updated_at=now）→ 0 行 → ConflictError。
	h2 := newQrHarness(t)
	h2.store.tryClaimDecision = reliable.AlreadySettled
	fresh := h2.qstate.rows[[2]int64{300, 40}]
	fresh.status = "REPLAYING"
	fresh.updatedAt = time.Now().UTC()
	fresh.rowVersion = 2
	h2.res.qstores[40].(*fakeQuarantineStore).getByIDRow.RowVersion = 2
	req2 := qrReq()
	req2.ExpectedRowVersion = 2
	err := h2.svc.QuarantineReplay(context.Background(), req2)
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("fresh REPLAYING row must conflict, got %v", err)
	}
}

// TestQuarantineReplay_DecodeError：decoder 报错 → CAS 回 QUARANTINED、计数+1、上抛原错误
// （controller ambiguity #2：非 ConflictError）。
func TestQuarantineReplay_DecodeError(t *testing.T) {
	h := newQrHarness(t)
	decErr := errors.New("envelope: bad crc")
	h.dec.err = decErr

	err := h.svc.QuarantineReplay(context.Background(), qrReq())
	if !errors.Is(err, decErr) {
		t.Fatalf("decode error must propagate (not ConflictError), got %v", err)
	}
	if h.store.tryClaimCalls != 0 {
		t.Fatal("decode failure must not reach TryClaim")
	}
	row, _ := h.qstate.row(300, 40)
	if row.status != "QUARANTINED" || row.replayAttempts != 1 {
		t.Fatalf("row must be back QUARANTINED attempts=1, got %+v", row)
	}
}

// TestQuarantineReplay_TenantMismatch：envelope 自称别的租户 → 拒绝 + 回 QUARANTINED 计数
// （防跨租户投递：行所在租户为准）。
func TestQuarantineReplay_TenantMismatch(t *testing.T) {
	h := newQrHarness(t)
	h.dec.tenantID = 41 // envelope 声明 41，行在 40。

	err := h.svc.QuarantineReplay(context.Background(), qrReq())
	if err == nil || !strings.Contains(err.Error(), "cross-tenant") {
		t.Fatalf("cross-tenant envelope must be refused, got %v", err)
	}
	if h.store.tryClaimCalls != 0 {
		t.Fatal("tenant mismatch must not reach TryClaim")
	}
	row, _ := h.qstate.row(300, 40)
	if row.status != "QUARANTINED" || row.replayAttempts != 1 {
		t.Fatalf("row must be back QUARANTINED attempts=1, got %+v", row)
	}
}

// TestQuarantineReplay_MarkFailedError：MarkFailed 出错 → 错误上抛 + 隔离行回 QUARANTINED
// （可重试毕业；event_consumption 行留 PROCESSING 的残留回收见 service.go I-1 注释——
// §3.2 不接管 payload-NULL 的 TryClaim-only 行，回收靠操作者重试 QuarantineReplay）。
func TestQuarantineReplay_MarkFailedError(t *testing.T) {
	h := newQrHarness(t)
	h.store.tryClaimDecision = reliable.Claimed
	mfErr := errors.New("markfailed: serialization failure")
	h.store.markFailedErr = mfErr

	err := h.svc.QuarantineReplay(context.Background(), qrReq())
	if !errors.Is(err, mfErr) {
		t.Fatalf("MarkFailed error must propagate, got %v", err)
	}
	row, _ := h.qstate.row(300, 40)
	if row.status != "QUARANTINED" || row.replayAttempts != 1 {
		t.Fatalf("row must be back QUARANTINED attempts=1, got %+v", row)
	}
}

// TestQuarantineReplay_RejectsMissingTenant：S3 守卫。
func TestQuarantineReplay_RejectsMissingTenant(t *testing.T) {
	h := newQrHarness(t)
	if err := h.svc.QuarantineReplay(context.Background(), QuarantineReplayRequest{}); !errors.Is(err, ErrMissingTenant) {
		t.Fatalf("got %v", err)
	}
}
