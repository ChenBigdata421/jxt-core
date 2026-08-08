package opsvc

import (
	"context"
	"errors"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"gorm.io/gorm"
)

// AccessAuditor 记录一次对门控数据（消费载荷或隔离区原始字节）的特权读。
// 服务侧实现（PR-7）从 ctx 解析已认证调用者身份并丰富事件（M14：内核不从 ctx 读身份，
// 故本接口只携带 TenantID/Kind/RowID 三元组）。opsvc 的义务是：只要 includePayload / includeRaw
// 为 true，就 GUARANTEE 这条调用发生（fail-closed：审计失败 → 不释放敏感数据）。
type AccessAuditor interface {
	RecordPrivilegedAccess(ctx context.Context, e PrivilegedAccessEvent) error
}

// PrivilegedAccessKind 标识被特权读的数据类别。Handler 据此路由到对应审计 sink。
type PrivilegedAccessKind string

const (
	// PrivilegedAccessConsumptionPayload 是对 event_consumption.Payload / Headers / RawKey 的特权读
	// （GetDetail includePayload=true）。
	PrivilegedAccessConsumptionPayload PrivilegedAccessKind = "consumption_payload"
	// PrivilegedAccessQuarantineRaw 是对 raw_message_quarantine.RawValue / RawKey / Headers 的特权读
	// （QuarantineDetail / QuarantineList includeRaw=true）。
	PrivilegedAccessQuarantineRaw PrivilegedAccessKind = "quarantine_raw"
)

// PrivilegedAccessEvent 是一次特权读的审计载荷。RowID=0 表示 bulk 路径（QuarantineList includeRaw=true
// 一次性释放多行原始字节，按 brief「至少一条事件」约定记一条 RowID=0 的 bulk 事件）。
type PrivilegedAccessEvent struct {
	TenantID int
	Kind     PrivilegedAccessKind
	RowID    int64
}

// ConflictError 是 §6.2.1 manual-replay 门禁 / CAS 版本不符 / D12 双人确认冲突的统一标记错误。
// Handler 用 errors.As(*ConflictError) 把它映射到 HTTP 409（brief：Populate Reason 区分三种来源）。
type ConflictError struct {
	Reason string
}

func (e *ConflictError) Error() string { return fmt.Sprintf("opsvc: conflict: %s", e.Reason) }

// 三种 Reason 常量（brief 规定）。
const (
	conflictReasonSibling      = "§6.2.1 earlier-unsolved sibling"
	conflictReasonD12          = "requester==approver (D12)"
	conflictReasonRowVersion   = "row_version mismatch"
)

// ErrMissingTenant 是任何 DTO（或 bare tenantID 参数）的 TenantID==0 时返回的标记错误。
// Handler 把它映射到 HTTP 400（S3 多租户作用域强制）。
var ErrMissingTenant = errors.New("opsvc: TenantID is required (must be > 0); bind a per-tenant request scope")

// Service 是 §10 ops 服务层。每个方法解析请求所属租户的 per-tenant Store / QuarantineStore + *gorm.DB，
// 拒绝 TenantID==0，并把底层 store 错误映射成 opsvc 的标记错误（ConflictError / ErrMissingTenant）。
// 一库一租户：单个 store 只服务一个租户，opsvc 从不枚举租户。
type Service struct {
	resolve store.TenantStoreResolver
	audit   AccessAuditor
	// txRunner 把一段闭包包进单个 DB 事务。默认是 db.Transaction（brief 规定：ReplayOne 的
	// sibling-check + ScheduleReplay 跑在同一 tx 以缩小 check-then-act TOCTOU 窗口）。仅在同包测试里
	// 覆写——避免为「两调用拿到同一 tx」这条断言拉一个真实 DB driver 进测试。生产调用方不设此字段。
	txRunner func(db *gorm.DB, fn func(tx *gorm.DB) error) error
}

// NewService 构造一个 ops Service。r 与 a 都必须非 nil：
//   - r==nil：无法解析租户 → 构造失败。
//   - a==nil：审计未注入则构造失败（Q4=A「未注入则构造失败」——fail-closed 不能靠运行时 nil 检查兜底，
//     构造期就拒绝，杜绝「上线忘配 auditor」的静默特权泄露）。
//
// 返回值签名偏离 plan 草稿的 `NewService(r) *Service`：Q4 增加了 required auditor + error return。
func NewService(r store.TenantStoreResolver, a AccessAuditor) (*Service, error) {
	if r == nil {
		return nil, errors.New("opsvc: NewService: nil TenantStoreResolver")
	}
	if a == nil {
		return nil, errors.New("opsvc: NewService: nil AccessAuditor (Q4=A: required for fail-closed privileged-access audit)")
	}
	return &Service{resolve: r, audit: a, txRunner: defaultTxRunner}, nil
}

// defaultTxRunner 是 txRunner 的生产默认值：直接委托 gorm.DB.Transaction。
func defaultTxRunner(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return db.Transaction(fn)
}

// requireTenant 是每个公开方法的第一道守卫：TenantID<=0 → ErrMissingTenant（handler → 400）。
// 用 <=0（与 DLQ adapter adapter.go:136 同源）而非 ==0：负租户同样无意义，统一拒绝，不把
// 「负租户是否被 resolver 服务」这一兜底交给运行期（resolver 通常也 fail-closed，但守卫层先拦更稳）。
func requireTenant(tenantID int) error {
	if tenantID <= 0 {
		return ErrMissingTenant
	}
	return nil
}

// mapStoreConflict 把底层 store 的 reliable.ErrConflict 翻译成 *ConflictError。
// ScheduleReplay 的 ErrConflict 只可能是 D12（requester==approver）或 row_version CAS 不符两条路径
// （store.ScheduleReplay 源码 gormshared/replay.go:362-389）：能凭 caller 已知参数区分就区分，
// 否则回落到 row_version mismatch。
func mapStoreConflict(err error, requesterApproverEqual bool) error {
	if !errors.Is(err, reliable.ErrConflict) {
		return err
	}
	reason := conflictReasonRowVersion
	if requesterApproverEqual {
		reason = conflictReasonD12
	}
	return &ConflictError{Reason: reason}
}

// —— 10 个公开方法（1:1 对 §10）——

// List 读 event_consumption（§10 列表）。返回行经 projectDetail 白名单投影到 Detail——不直接透出
// store.Row（它还带 ReplayAuthID 一次性重放 bearer / ClaimID / ReplayRequestedBy 等人员身份与内部令牌，
// 见 store/row.go）；List 无审计钩子，按 fail-closed 只释放与 GetDetail 同构的安全字段集。
// 白名单而非按字段清零：下次 store.Row 新增敏感字段时，denylist 会因遗漏而泄露，白名单天然不泄。
// Detail 的 Payload/Headers/RawKey 恒 nil（门控字段）——需载荷走 GetDetail(includePayload=true，先审计)。
func (s *Service) List(ctx context.Context, q ListQuery) (ListResult, error) {
	if err := requireTenant(q.TenantID); err != nil {
		return ListResult{}, err
	}
	st, _, err := s.resolve.Store(q.TenantID)
	if err != nil {
		return ListResult{}, err
	}
	rows, err := st.List(ctx, store.ListFilter{
		TenantID: q.TenantID, Status: q.Status, ErrorClass: q.ErrorClass,
		HandlerID: q.HandlerID, From: q.From, To: q.To,
		Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		return ListResult{}, err
	}
	out := make([]Detail, len(rows))
	for i := range rows {
		out[i] = projectDetail(rows[i]) // 白名单投影：门控字段与 ReplayAuthID/人员字段均不在 Detail
	}
	return ListResult{Rows: out}, nil
}

// GetDetail 读单行（§10 详情）。includePayload=false → Detail.Payload/Headers/RawKey 恒 nil。
// includePayload=true → 先审计（consumption_payload + 行 id），审计失败则 fail-closed 不释放（brief Q4=A）。
func (s *Service) GetDetail(ctx context.Context, tenantID int, id int64, includePayload bool) (Detail, error) {
	if err := requireTenant(tenantID); err != nil {
		return Detail{}, err
	}
	st, _, err := s.resolve.Store(tenantID)
	if err != nil {
		return Detail{}, err
	}
	row, err := st.GetByID(ctx, tenantID, id)
	if err != nil {
		return Detail{}, err
	}
	d := projectDetail(row)
	if !includePayload {
		return d, nil
	}
	// fail-closed：审计在前，失败即返回、不释放载荷。
	if err := s.audit.RecordPrivilegedAccess(ctx, PrivilegedAccessEvent{
		TenantID: tenantID, Kind: PrivilegedAccessConsumptionPayload, RowID: id,
	}); err != nil {
		return Detail{}, err
	}
	d.Payload = row.Payload
	d.Headers = row.Headers
	d.RawKey = row.RawKey
	return d, nil
}

// ReplayOne 授权单行人工重放（§6.2 / §6.2.1）。把 §6.2.1 门禁（HasEarlierUnsolvedSibling）
// 与 ScheduleReplay 跑在同一 db.Transaction 内以缩小 check-then-act TOCTOU 窗口
// （brief Q1=A：ScheduleReplay 单独不强制 §6.2.1，由本方法补）。两类冲突都映射成 *ConflictError：
//   - HasEarlierUnsolvedSibling==true → §6.2.1 earlier-unsolved sibling（且不调 ScheduleReplay）；
//   - ScheduleReplay 返回 reliable.ErrConflict → D12 或 row_version mismatch。
//
// 残留竞态：sibling 行的 check-then-act 在 tx 外仍可被并行写入翻转（manual replay 低频，可接受，brief 已记）。
func (s *Service) ReplayOne(ctx context.Context, r ReplayRequest) error {
	if err := requireTenant(r.TenantID); err != nil {
		return err
	}
	st, db, err := s.resolve.Store(r.TenantID)
	if err != nil {
		return err
	}
	return s.replayOneInTx(ctx, st, db, BatchReplayItem{
		ID: r.ID, ExpectedRowVersion: r.ExpectedRowVersion,
		Requester: r.Requester, Approver: r.Approver, Reason: r.Reason,
	})
}

// replayOneInTx 是 ReplayOne 与 BatchReplay 共享的单行重放内核：txRunner 包住 sibling-check + ScheduleReplay。
// 调用方负责 tenant 守卫与 resolver 解析；本方法假定 st/db 已是该 tenant 的句柄。
func (s *Service) replayOneInTx(ctx context.Context, st store.Store, db *gorm.DB, item BatchReplayItem) error {
	return s.txRunner(db, func(tx *gorm.DB) error {
		blocked, err := st.HasEarlierUnsolvedSibling(ctx, tx, item.ID)
		if err != nil {
			return err
		}
		if blocked {
			return &ConflictError{Reason: conflictReasonSibling}
		}
		if err := st.ScheduleReplay(ctx, tx, item.ID, item.ExpectedRowVersion,
			item.Requester, item.Approver, item.Reason); err != nil {
			return mapStoreConflict(err, item.Requester == item.Approver)
		}
		return nil
	})
}

// BatchReplay 批量人工重放（§10）。每行独立 CAS + 进度/失败明细：无跨行事务，一行冲突/出错不阻塞其余行。
// 每行内部仍走 replayOneInTx（per-row tx：sibling-check + ScheduleReplay 在同一 tx）。Requester/Approver
// 逐行强制 D12。返回 Results 与入参 Items 同序、一一对应。
func (s *Service) BatchReplay(ctx context.Context, r BatchReplayRequest) (BatchReplayResult, error) {
	if err := requireTenant(r.TenantID); err != nil {
		return BatchReplayResult{}, err
	}
	st, db, err := s.resolve.Store(r.TenantID)
	if err != nil {
		return BatchReplayResult{}, err
	}
	results := make([]BatchReplayRowResult, len(r.Items))
	for i, item := range r.Items {
		err := s.replayOneInTx(ctx, st, db, item)
		res := BatchReplayRowResult{ID: item.ID}
		switch {
		case err == nil:
			res.Ok = true
		default:
			var ce *ConflictError
			if errors.As(err, &ce) {
				// 三态互斥（dto 约定）：冲突行只置 Conflict、不填 Err——否则 handler 若先查 Err!=""
				// 会把常规可重试冲突（§6.2.1 sibling / CAS row_version / D12）误判成硬错误并回 500。
				// ConflictReason 保留具体来源（与单行 ReplayOne 返回的 *ConflictError.Reason 对齐），
				// 供 handler 填充 409 body——否则三种冲突坍缩成一个 opaque flag，操作者无从选择处置。
				res.Conflict = true
				res.ConflictReason = ce.Reason
			} else {
				res.Err = err.Error()
			}
		}
		results[i] = res
	}
	return BatchReplayResult{Results: results}, nil
}

// Discard 把 DEAD_LETTER 行标记为 DISCARDED（§10）。store.Discard 的 reliable.ErrConflict
// （仅 row_version CAS 不符一条路径）映射成 *ConflictError。
func (s *Service) Discard(ctx context.Context, r DiscardRequest) error {
	if err := requireTenant(r.TenantID); err != nil {
		return err
	}
	st, db, err := s.resolve.Store(r.TenantID)
	if err != nil {
		return err
	}
	if err := st.Discard(ctx, db, r.ID, r.ExpectedRowVersion, r.By, r.Reason); err != nil {
		// Discard 无 D12 路径，所有 ErrConflict 都是 row_version mismatch。
		if errors.Is(err, reliable.ErrConflict) {
			return &ConflictError{Reason: conflictReasonRowVersion}
		}
		return err
	}
	return nil
}

// Stats 返回 §10 dashboard totals。全部由 store.Count 填充（F6：禁止 list-then-count）。
// 按五态各跑一次 Count（命中 partial/普通索引），Total = 五态之和。
func (s *Service) Stats(ctx context.Context, q ListQuery) (Stats, error) {
	if err := requireTenant(q.TenantID); err != nil {
		return Stats{}, err
	}
	st, _, err := s.resolve.Store(q.TenantID)
	if err != nil {
		return Stats{}, err
	}
	statuses := []reliable.Status{
		reliable.StatusProcessing, reliable.StatusSucceeded, reliable.StatusRetryScheduled,
		reliable.StatusDeadLetter, reliable.StatusDiscarded,
	}
	byStatus := make(map[reliable.Status]int64, len(statuses))
	var total int64
	for _, stv := range statuses {
		n, err := st.Count(ctx, store.CountFilter{
			TenantID: q.TenantID, Status: stv, ErrorClass: q.ErrorClass,
			HandlerID: q.HandlerID, From: q.From, To: q.To,
		})
		if err != nil {
			return Stats{}, err
		}
		byStatus[stv] = n
		total += n
	}
	return Stats{Total: total, ByStatus: byStatus}, nil
}

// QuarantineList 读 raw_message_quarantine（§10 列表）。includeRaw=false → 每个元素的 RawValue/RawKey/Headers 恒 nil。
// includeRaw=true → 整批记一条 bulk 审计事件（RowID=0），审计失败 fail-closed 不释放任何原始字节。
func (s *Service) QuarantineList(ctx context.Context, tenantID int, status string, limit int, includeRaw bool) ([]QuarantineDetail, error) {
	if err := requireTenant(tenantID); err != nil {
		return nil, err
	}
	qs, err := s.resolve.QuarantineStore(tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := qs.List(ctx, tenantID, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]QuarantineDetail, len(rows))
	for i := range rows {
		out[i] = projectQuarantineDetail(rows[i])
	}
	if !includeRaw {
		return out, nil
	}
	// bulk 路径：至少一条事件（brief 容许 RowID=0 标记整批）。fail-closed 在前。
	if err := s.audit.RecordPrivilegedAccess(ctx, PrivilegedAccessEvent{
		TenantID: tenantID, Kind: PrivilegedAccessQuarantineRaw, RowID: 0,
	}); err != nil {
		return nil, err
	}
	for i := range rows {
		out[i].RawValue = rows[i].RawValue
		out[i].RawKey = rows[i].RawKey
		out[i].Headers = rows[i].Headers
	}
	return out, nil
}

// QuarantineDetail 读单条隔离区行（§10 详情）。includeRaw=false → RawValue/RawKey/Headers 恒 nil。
// includeRaw=true → 先审计（quarantine_raw + 行 id），fail-closed 不释放。
func (s *Service) QuarantineDetail(ctx context.Context, tenantID int, id int64, includeRaw bool) (QuarantineDetail, error) {
	if err := requireTenant(tenantID); err != nil {
		return QuarantineDetail{}, err
	}
	qs, err := s.resolve.QuarantineStore(tenantID)
	if err != nil {
		return QuarantineDetail{}, err
	}
	row, err := qs.GetByID(ctx, tenantID, id)
	if err != nil {
		return QuarantineDetail{}, err
	}
	d := projectQuarantineDetail(row)
	if !includeRaw {
		return d, nil
	}
	if err := s.audit.RecordPrivilegedAccess(ctx, PrivilegedAccessEvent{
		TenantID: tenantID, Kind: PrivilegedAccessQuarantineRaw, RowID: id,
	}); err != nil {
		return QuarantineDetail{}, err
	}
	d.RawValue = row.RawValue
	d.RawKey = row.RawKey
	d.Headers = row.Headers
	return d, nil
}

// QuarantineResolve 把隔离区行标记为 RESOLVED（§10）。qs.MarkResolved 的 reliable.ErrConflict
// （row_version mismatch 或跨租户 0 行命中，后者兼作枚举预言机防护）映射成 *ConflictError。
// db 来自 resolver.Store（per-tenant db，与 QuarantineStore 同库不同表）；QuarantineStore 不返回 db。
func (s *Service) QuarantineResolve(ctx context.Context, r ResolveRequest) error {
	if err := requireTenant(r.TenantID); err != nil {
		return err
	}
	// db 来自 Store()：resolver 只在 Store() 上暴露 *gorm.DB；QuarantineStore() 不带 db。
	_, db, err := s.resolve.Store(r.TenantID)
	if err != nil {
		return err
	}
	qs, err := s.resolve.QuarantineStore(r.TenantID)
	if err != nil {
		return err
	}
	if err := qs.MarkResolved(ctx, db, r.TenantID, r.ID, r.ExpectedRowVersion, r.By); err != nil {
		if errors.Is(err, reliable.ErrConflict) {
			return &ConflictError{Reason: conflictReasonRowVersion}
		}
		return err
	}
	return nil
}

// Anomalies 读 consumption_anomalies（§10 /anomalies 视图）。直接透传 store.ListAnomalies 的结果。
func (s *Service) Anomalies(ctx context.Context, f AnomalyQuery) ([]store.AnomalyRow, error) {
	if err := requireTenant(f.TenantID); err != nil {
		return nil, err
	}
	st, _, err := s.resolve.Store(f.TenantID)
	if err != nil {
		return nil, err
	}
	return st.ListAnomalies(ctx, store.AnomalyFilter{
		TenantID: f.TenantID, Kind: f.Kind, From: f.From, To: f.To, Limit: f.Limit,
	})
}

// —— 投影辅助 ——

// projectDetail 把 store.Row 投影成 Detail（门控字段留空，由 GetDetail 按审计结果填充）。
func projectDetail(r store.Row) Detail {
	return Detail{
		ID: r.ID, EventID: r.EventID, ItemKey: r.ItemKey, HandlerID: r.HandlerID,
		TenantID: r.TenantID, EventType: r.EventType, AggregateType: r.AggregateType,
		AggregateID: r.AggregateID, Topic: r.Topic, Status: r.Status, Attempt: r.Attempt,
		ReplayGeneration: r.ReplayGeneration, RowVersion: r.RowVersion,
		ErrorClass: r.ErrorClass, ErrorCode: r.ErrorCode, ErrorMessage: r.ErrorMessage,
		NextAttemptAt: r.NextAttemptAt, ReplayMode: r.ReplayMode,
		FirstSeenAt: r.FirstSeenAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// projectQuarantineDetail 把 store.QuarantineRow 投影成 QuarantineDetail（门控字段留空）。
func projectQuarantineDetail(r store.QuarantineRow) QuarantineDetail {
	return QuarantineDetail{
		ID: r.ID, TenantID: r.TenantID, HandlerID: r.HandlerID, Topic: r.Topic,
		SrcPartition: r.SrcPartition, SrcOffset: r.SrcOffset, RawPayloadHash: r.RawPayloadHash,
		ErrorMessage: r.ErrorMessage, Status: r.Status, RowVersion: r.RowVersion,
		ResolvedAt: r.ResolvedAt, ResolvedBy: r.ResolvedBy, CreatedAt: r.CreatedAt,
	}
}
