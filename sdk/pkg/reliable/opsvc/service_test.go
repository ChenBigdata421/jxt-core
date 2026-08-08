package opsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"gorm.io/gorm"
)

// —— fakes ——
//
// fakeStore 嵌入 nil 的 store.Store 接口以 promotion 满足整张接口；只覆写 opsvc 实际调用的方法，
// 其余方法若被调用会 nil-deref panic（测试只走 opsvc 路径，不会触发）——这是 Go test fake 的惯用法，
// 避免为 22 个方法逐一写 panic 存根。fakeQuarantineStore 同理。

type scheduleCall struct {
	id, ver                   int64
	requester, approver, reason string
}

type fakeStore struct {
	store.Store // nil 嵌入：满足接口；未覆写方法 panic（测试不触发）。
	tenantID    int

	// 调用计数（cross-tenant isolation 断言用）。
	listCalls            int
	getByIDCalls         int
	scheduleCalls        int
	hasSiblingCalls      int
	discardCalls         int
	countCalls           int
	listAnomaliesCalls   int

	// 捕获的入参。
	capturedListFilter     store.ListFilter
	capturedCountFilters   []store.CountFilter
	capturedAnomalyFilter  store.AnomalyFilter
	capturedSchedule       scheduleCall
	capturedHasSiblingID   int64
	capturedHasSiblingDB   *gorm.DB
	capturedScheduleDB     *gorm.DB
	capturedDiscardID      int64
	capturedDiscardVer     int64
	capturedDiscardBy      string
	capturedDiscardReason  string

	// 返回值。
	listRows       []store.Row
	listErr        error
	getByIDRow     store.Row
	getByIDErr     error
	scheduleErr    error
	hasSiblingRes  bool
	hasSiblingErr  error
	discardErr     error
	countRes       int64
	countErr       error
	anomalyRows    []store.AnomalyRow
	anomalyErr     error
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

	listCalls     int
	getByIDCalls  int
	resolveCalls  int

	capturedListStatus  string
	capturedListLimit   int
	capturedGetID       int64
	capturedResolveID   int64
	capturedResolveVer  int64
	capturedResolveBy   string
	capturedResolveDB   *gorm.DB

	listRows    []store.QuarantineRow
	listErr     error
	getByIDRow  store.QuarantineRow
	getByIDErr  error
	resolveErr  error
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
