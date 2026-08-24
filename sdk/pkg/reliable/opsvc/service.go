package opsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/replay"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/gormshared"
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
	conflictReasonLiveClaim    = "live claim active"
	conflictReasonMaxReplayAtt = "max replay attempts"
)

// ErrMissingTenant 是任何 DTO（或 bare tenantID 参数）的 TenantID==0 时返回的标记错误。
// Handler 把它映射到 HTTP 400（S3 多租户作用域强制）。
var ErrMissingTenant = errors.New("opsvc: TenantID is required (must be > 0); bind a per-tenant request scope")

// ErrQuarantineReplayUnsupported 是 QuarantineReplay 在 Service 未同时注入 WithRegistry +
// WithEnvelopeDecoder 时返回的标记错误（PR-7 Task 3，C①）。fail-closed：缺任何一个都无法
// 构造 TryClaim 所需的 ClaimInput（registry 校验 HandlerID 合法性；decoder 解出 Key/Meta/TenantID），
// 静默忽略或部分降级都会把毒消息送进一条没有身份校验的执行路径。
var ErrQuarantineReplayUnsupported = errors.New("opsvc: quarantine replay requires WithRegistry + WithEnvelopeDecoder")

// QuarantineReplayMaxAttempts（OV⑤③）：单条隔离行的失败重放上限。达到后 QuarantineReplay
// 返回 *ConflictError{max replay attempts} 且行不再被本端点触碰——操作者必须走
// QuarantineResolve + 外部重排队（re-emit）处置。上限防的是「操作者对一条每次都失败的毒消息
// 无限点击 replay」把 TryClaim/异常路径刷成自噪。
const QuarantineReplayMaxAttempts = 5

// quarantineReplayWatchdog（OV⑤④）：REPLAYING 停留超过该时长视为「上次 replay 崩溃在半路」，
// QuarantineReplay 可重claim（CAS 谓词对 REPLAYING 行加 updated_at < now-watchdog 守卫）。
// 10min >> 任何同步 TryClaim 时长——误判需要 TryClaim 卡死 >10min，那本身就是独立事故。
// 服务侧 Task 14 的 OpsMaintenance sweep（REPLAYING 超时 → QUARANTINED）用同一常量。
const quarantineReplayWatchdog = 10 * time.Minute

// Service 是 §10 ops 服务层。每个方法解析请求所属租户的 per-tenant Store / QuarantineStore + *gorm.DB，
// 拒绝 TenantID==0，并把底层 store 错误映射成 opsvc 的标记错误（ConflictError / ErrMissingTenant）。
// 一库一租户：单个 store 只服务一个租户，opsvc 从不枚举租户。
type Service struct {
	resolve store.TenantStoreResolver
	audit   AccessAuditor
	// registry / envelopeDecoder 只服务 QuarantineReplay（PR-7 Task 3，C①）。nil（未注入）时
	// QuarantineReplay 返回 ErrQuarantineReplayUnsupported（fail-closed），其余 10 个方法不受影响。
	registry        replay.HandlerRegistry
	envelopeDecoder EnvelopeDecoder
	// qrClaim / qrBack / qrResolve 是 QuarantineReplay 的三个隔离区 CAS 迁移（接口冻结决策：
	// 不进 QuarantineStore，直接写在 resolver 提供的 per-tenant db 上）。做成 func 字段与
	// txRunner 同理——仅同包测试覆写，避免为断言 CAS 语义拉真实 DB driver 进单元测试；
	// 生产默认值就是本文件的 gorm 实现（NewService 里绑定）。真实 SQL 语义由 system-tagged
	// 测试（repotest 真库）钉住。
	qrClaim func(ctx context.Context, db *gorm.DB, id int64, tenantID int, expectedVersion int64, watchdogCutoff time.Time) error
	qrBack  func(ctx context.Context, db *gorm.DB, id int64, tenantID int, replayRowVer int64, cause error) error
	// qrResolve 不带 detail 参数（review I-3）：RESOLVED 迁移不写 error_message——原始
	// quarantine cause 必须在隔离行上保留（隔离行 = incident 记录）。见 casToResolved 头注释。
	qrResolve func(ctx context.Context, db *gorm.DB, id int64, tenantID int, replayRowVer int64, by string) error
	// txRunner 把一段闭包包进单个 DB 事务。默认是 db.Transaction（brief 规定：ReplayOne 的
	// sibling-check + ScheduleReplay 跑在同一 tx 以缩小 check-then-act TOCTOU 窗口）。仅在同包测试里
	// 覆写——避免为「两调用拿到同一 tx」这条断言拉一个真实 DB driver 进测试。生产调用方不设此字段。
	txRunner func(db *gorm.DB, fn func(tx *gorm.DB) error) error
}

// ServiceOption 是 NewService 的可选注入项（PR-7 Task 3，C①）。additive：现有 2 参调用不变。
type ServiceOption func(*Service)

// EnvelopeDecoder 把隔离区原始字节解回 TryClaim 所需的身份三元组。服务侧实现包住
// eventbus.FromBytes（J2：本包不 import eventbus——那会把 sarama 带进 ops 层）。返回的
// key.ItemKey 恒为 ""（隔离区存的是单条不可解码消息，无批量 item 维度）；tenantID 来自
// envelope 自身声明，QuarantineReplay 会再与请求的租户作用域比对。
type EnvelopeDecoder func(raw []byte) (key reliable.Key, meta reliable.Meta, tenantID int, err error)

// WithRegistry 注入 handler 注册表。**OV⑤ 重设计后的角色**：QuarantineReplay 只用它校验
// 隔离行的 HandlerID 是已注册 handler（unknown → 错误、行不动）——绝不直接调 Handler.Handle。
// 直接调 handler 会造出第三条执行路径，绕过 TryClaim fencing、aggregate gate、attempt 上限
// 与 §10 运维面（OV⑤=8A）。注入的值须与消费侧 scheduler 共用同一注册表实例。
func WithRegistry(reg replay.HandlerRegistry) ServiceOption {
	return func(s *Service) { s.registry = reg }
}

// WithEnvelopeDecoder 注入 envelope 解码器。服务侧实现包住 eventbus.FromBytes（evidence 的
// 双胞胎在 shared/infrastructure/reliable/ops_service.go）——opsvc 自身保持 eventbus-free
// （J2：opsvc 只 import store/reliable/gorm(+replay)，拉进 eventbus 会把 sarama 带入 ops 层）。
// 解出的 tenantID 与 r.TenantID 不一致时 QuarantineReplay 拒绝（防跨租户投递：envelope 自称
// 租户 A 而行存于租户 B 的隔离表，以行所在租户为准 + 显式报错，不静默改写）。
func WithEnvelopeDecoder(fn EnvelopeDecoder) ServiceOption {
	return func(s *Service) { s.envelopeDecoder = fn }
}

// NewService 构造一个 ops Service。r 与 a 都必须非 nil：
//   - r==nil：无法解析租户 → 构造失败。
//   - a==nil：审计未注入则构造失败（Q4=A「未注入则构造失败」——fail-closed 不能靠运行时 nil 检查兜底，
//     构造期就拒绝，杜绝「上线忘配 auditor」的静默特权泄露）。
//
// opts 是可选注入（PR-7 Task 3）：WithRegistry + WithEnvelopeDecoder 共同启用 QuarantineReplay；
// 只注入其一同样 fail-closed（QuarantineReplay 返回 ErrQuarantineReplayUnsupported，不半启动）。
//
// 返回值签名偏离 plan 草稿的 `NewService(r) *Service`：Q4 增加了 required auditor + error return。
func NewService(r store.TenantStoreResolver, a AccessAuditor, opts ...ServiceOption) (*Service, error) {
	if r == nil {
		return nil, errors.New("opsvc: NewService: nil TenantStoreResolver")
	}
	if a == nil {
		return nil, errors.New("opsvc: NewService: nil AccessAuditor (Q4=A: required for fail-closed privileged-access audit)")
	}
	s := &Service{resolve: r, audit: a, txRunner: defaultTxRunner}
	s.qrClaim = s.casToReplaying
	s.qrBack = s.casBackToQuarantined
	s.qrResolve = s.casToResolved
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s, nil
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

// —— QuarantineReplay（PR-7 Task 3，C①；OV⑤=8A「毕业」流程）——

// quarantine CAS 语句（gorm Updates 形态，与 gormshared/quarantine.go MarkResolved 同纪律：
// 先查 res.Error 再看 RowsAffected——DB 错误/ctx 取消不得伪装成 CAS conflict）。
const (
	quarantineStatusQuarantined = "QUARANTINED"
	quarantineStatusReplaying   = "REPLAYING"
	quarantineStatusResolved    = "RESOLVED"
)

// QuarantineReplay 把一条隔离区毒消息「毕业」回正常消费路径（POST /quarantine/:id/replay）。
// **OV⑤=8A 核心不变量：绝不直接调 Handler.Handle**——直接执行会造出第三条执行路径，绕过
// TryClaim fencing、aggregate gate（§6.2.1）、attempt 上限（§6.2）与 §10 运维面。流程：
//
//  1. 读行（tenant 作用域，GetByID）+ 尝试上限守卫（replay_attempts >= QuarantineReplayMaxAttempts
//     → ConflictError，行不动——OV⑤③）+ registry 校验 HandlerID 已注册（unknown → 错误，行不动）。
//  2. CAS QUARANTINED→REPLAYING（ExpectedRowVersion）；REPLAYING 行仅在 updated_at <
//     now-quarantineReplayWatchdog 时可被重claim（OV⑤④：上次 replay 崩溃在半路的自愈）。
//  3. 解码 envelope（服务注入的 EnvelopeDecoder；解码失败 → CAS 回 QUARANTINED、计数+1、上抛错误）。
//  4. store.TryClaim(Key{EventID 来自解码, Handler 来自【行】而非 envelope, ItemKey:""}, Meta, TenantID,
//     Delivery 由行内 Raw* 重建)——行序与消费侧 LIVE 投递完全相同的仲裁点（M5/M3：与在途消费者
//     的 TryClaim 互斥，无双重处理）。Handler 取行不取 envelope：隔离行的 handler_id 是当初投递
//     绑定的稳定协议标识（§3.1），envelope 只是载荷；以行为准防「毒载荷自称别的 handler」越权。
//     Delivery 的 Topic/Partition/Offset/RawKey/Headers/PayloadHash 用行内存储的 broker 原始值
//     重建（RawValue 是完整 envelope 字节，另作 MarkFailed 的 payload 持久化）。
//  5. TryClaim 三分支：
//     - Claimed → MarkFailed(pay­load=RawValue, class=RETRYABLE, safety=registry 的 ReplaySafety)
//     把行转成 RETRY_SCHEDULED/DEAD_LETTER——这是唯一能同时满足 chk_retry_due（payload +
//     next_attempt_at + error_class）且让 FindEligibleHeads 可扫到的内核路径；随后 CAS
//     REPLAYING→RESOLVED（resolved_by=r.By）。业务执行、attempt 计数、aggregate gate、
//     head 排序全部由既有 replay scheduler 承接（对本方法是黑盒）。attempt 上限由
//     MarkFailed 的 maxAttempts 参数强制（§6.1），与消费侧 LIVE 路径同一条防线。
//     - AlreadySettled → 消息早已落地（幂等成功）：CAS REPLAYING→RESOLVED。与 Claimed 分支
//     写完全相同的迁移（review I-3 后 CAS 不再持久化区分性 detail——见 casToResolved 头注释）；
//     「毕业 vs 早已结算」的区分由 event_consumption 侧承载（Claimed 分支会留下 MarkFailed 的
//     RETRY_SCHEDULED/DEAD_LETTER 行，AlreadySettled 不动消费行）。
//     - AlreadyProcessing → 在途消费者持有租约：CAS REPLAYING→QUARANTINED（row_version+1、
//     replay_attempts+1、error_message="live consumer holds the lease; retry after it settles"），
//     返回 *ConflictError{live claim active}（→ HTTP 409）。等在途方结算后再试。
//     - error → CAS REPLAYING→QUARANTINED（replay_attempts+1、error_message=sanitize 后的 cause），
//     上抛错误（非 ConflictError）。
//
// **接口冻结（Step 3 locked decision）**：REPLAYING→X 的两三次 CAS 由本方法直接在
// resolver 提供的 per-tenant *gorm.DB 上执行（与 ScheduleReplay 写 db 同纪律），【不】给
// store.QuarantineStore 加 MarkReplaying/MarkQuarantinedBack——隔离区全部状态迁移集中在
// 一个文件里（store 侧保持哑 CRUD 端口），且这些迁移是 QuarantineReplay 私有编排，没有
// 第二个调用方需要共享。
//
// **审计判定（controller 指示的 reasoned choice）**：本方法【不】发 quarantine_raw 特权读审计。
// 现有 quarantine_raw 审计（QuarantineDetail/QuarantineList includeRaw=true）的义务是
// 「只要把原始字节释放给【调用方】就保证审计」——本方法读 RawValue 仅作内部解码 + 喂给
// TryClaim/MarkFailed（内核内部数据流），RawValue/RawKey/Headers 从不出现在返回值或错误文本里
// （错误只携带 sanitize 后的 cause 字符串）。特权读审计的威胁模型是「人看到毒载荷内容」；
// 机器内部的字节搬运不产生新的暴露面。此为有意决策，非遗漏。
func (s *Service) QuarantineReplay(ctx context.Context, r QuarantineReplayRequest) error {
	if err := requireTenant(r.TenantID); err != nil {
		return err
	}
	// fail-closed：缺 registry 或 decoder 都无法构造合法 ClaimInput。
	if s.registry == nil || s.envelopeDecoder == nil {
		return ErrQuarantineReplayUnsupported
	}
	qs, err := s.resolve.QuarantineStore(r.TenantID)
	if err != nil {
		return err
	}
	st, db, err := s.resolve.Store(r.TenantID)
	if err != nil {
		return err
	}

	// —— 步骤 1：读行（tenant 作用域）+ 前置守卫（上限 / registry）。行未动。 ——
	row, err := qs.GetByID(ctx, r.TenantID, r.ID)
	if err != nil {
		return err
	}
	if row.ReplayAttempts >= QuarantineReplayMaxAttempts {
		// OV⑤③：达到上限 → 行不动，操作者走 QuarantineResolve + 外部重排队。
		return &ConflictError{Reason: conflictReasonMaxReplayAtt}
	}
	if _, ok := s.registry.Lookup(row.HandlerID); !ok {
		return fmt.Errorf("opsvc: quarantine replay: handler %q not registered (row untouched)", row.HandlerID)
	}

	// —— 步骤 2：CAS →REPLAYING（qrClaim seam；真实谓词见 casToReplaying）。 ——
	now := time.Now().UTC()
	if err := s.qrClaim(ctx, db, r.ID, r.TenantID, r.ExpectedRowVersion, now.Add(-quarantineReplayWatchdog)); err != nil {
		return err // 0 行 → *ConflictError（版本不符 / 非 QUARANTINED 亦非超时 REPLAYING，含已 RESOLVED）。
	}

	// —— 步骤 3：解码。失败 → CAS 回 QUARANTINED、replay_attempts+1、上抛。 ——
	key, meta, envTenant, decErr := s.envelopeDecoder(row.RawValue)
	if decErr != nil {
		return errors.Join(decErr, s.qrBack(ctx, db, r.ID, r.TenantID, row.RowVersion, decErr))
	}
	if envTenant != r.TenantID {
		tenErr := fmt.Errorf("opsvc: quarantine replay: envelope tenant %d != row tenant %d (cross-tenant envelope refused)", envTenant, r.TenantID)
		return errors.Join(tenErr, s.qrBack(ctx, db, r.ID, r.TenantID, row.RowVersion, tenErr))
	}

	// —— 步骤 4：TryClaim（与消费侧 LIVE 投递同一仲裁点）。 ——
	info, _ := s.registry.Lookup(row.HandlerID) // 步骤 1 已确认存在。
	claimKey := reliable.Key{EventID: key.EventID, Handler: row.HandlerID, ItemKey: ""}
	delivery := reliable.DeliveryMeta{
		Topic:           row.Topic,
		Partition:       row.SrcPartition,
		Offset:          row.SrcOffset,
		BrokerTimestamp: derefTime(row.BrokerTimestamp),
		PayloadHash:     row.RawPayloadHash,
		RawKey:          append([]byte(nil), row.RawKey...),
		Headers:         append([]reliable.HeaderPair(nil), row.Headers...),
	}
	tok, dec, err := st.TryClaim(ctx, reliable.ClaimInput{
		Key: claimKey, Meta: meta, TenantID: r.TenantID, Delivery: delivery,
	}, quarantineReplayClaimLease)
	switch {
	case err == nil && dec == reliable.Claimed:
		// 毕业收尾：MarkFailed 把 envelope 字节持久化为 payload 并按 §6.1 矩阵落
		// RETRY_SCHEDULED（safety != ReplayUnsafe）或 DEAD_LETTER——FindEligibleHeads 从此
		// 可扫到（chk_retry_due 的 payload/next_attempt_at/error_class 三件套，在 token 持有
		// 路径内只有 MarkFailed 会写全——review M-5；无 token 的 RecordTerminal 也写全，
		// 但本路径已持 TryClaim token，走 MarkFailed）。类用 RETRYABLE：这是一次人工触发
		// 的重投，非业务失败——RETRYABLE + 非 ReplayUnsafe 正是「让 scheduler 正常驱动」
		// 的入口；unsafe handler 则直接落死信，交给 §6.2 的人工双人路径，
		// 绝不静默重放有进程外副作用的 handler。
		mfErr := st.MarkFailed(ctx, db, claimKey, tok, reliable.ClassRetryable,
			info.ReplaySafety, reliable.DefaultMaxAttempts,
			errQuarantineGraduated, row.RawValue)
		if mfErr != nil {
			// MarkFailed 失败：行留在 PROCESSING（TryClaim 已占位，payload 仍 NULL）。review I-1：
			// 这条残留【不会】被 §3.2 自动回收接管——recoverCandidateSQL 只选 payload IS NOT NULL
			// 的行（gormshared/recovery.go），而 TryClaim-only 行的 payload 恒 NULL（payload 只由
			// MarkFailed/RecordTerminal 写）；broker 重投也不可能：隔离落库时 offset 已被 ACK
			// （adapters/eventbus/adapter.go quarantine write-before-ACK，Record 成功 → nil → ACK）。
			// 真实回收路径 = 操作者重试：下一次 QuarantineReplay 的 TryClaim 命中 claim.go 的
			// 租约过期内联回收分支（PROCESSING + lease_expires_at < now → CAS 续占 → Claimed），
			// 毕业重新走完。⚠ 上限交互（OV⑤③）：QuarantineReplayMaxAttempts 耗尽（5 次失败周期）
			// 后隔离行被本端点永久拒收（conflictReasonMaxReplayAtt），此时 PROCESSING 残留对
			// 本端点不可恢复——只能走 QuarantineResolve + 外部重排队（re-emit）处置。不为它造新机制。
			return errors.Join(mfErr, s.qrBack(ctx, db, r.ID, r.TenantID, row.RowVersion, mfErr))
		}
		// 隔离行 → RESOLVED（毕业完成）。并发迁移（0 行）由 seam 幂等返回 nil——毕业的
		// 事实已由 MarkFailed 的产物保证（TryClaim 的 uk 幂等兜底防双毕业）。不写
		// error_message（review I-3）：原始 quarantine cause 保留在行上。
		return s.qrResolve(ctx, db, r.ID, r.TenantID, row.RowVersion, r.By)
	case err == nil && dec == reliable.AlreadySettled:
		// 幂等成功：消息早已落地。隔离行 → RESOLVED，不计数、同样不写 error_message（I-3）。
		return s.qrResolve(ctx, db, r.ID, r.TenantID, row.RowVersion, r.By)
	case err == nil && dec == reliable.AlreadyProcessing:
		// 在途消费者持有租约：回 QUARANTINED + 409。live 消费者自己的 TryClaim/Mark* 才是
		// 该行的处置者（M5/M3 同一仲裁点），这里绝不能覆盖。
		if backErr := s.qrBack(ctx, db, r.ID, r.TenantID, row.RowVersion, errLiveClaimHolds); backErr != nil {
			return errors.Join(&ConflictError{Reason: conflictReasonLiveClaim}, backErr)
		}
		return &ConflictError{Reason: conflictReasonLiveClaim}
	default: // TryClaim 出错
		return errors.Join(err, s.qrBack(ctx, db, r.ID, r.TenantID, row.RowVersion, err))
	}
}

// errQuarantineGraduated 是毕业路径 MarkFailed 的 cause：一行说明重放的来源，写入
// error_message（sanitize 后）。非业务错误——语义是「这条行是从隔离区毕业的」。
var errQuarantineGraduated = errors.New("quarantine replay: graduated from raw_message_quarantine into replay path")

// errLiveClaimHolds 是 AlreadyProcessing 分支写回 error_message 的固定文案（brief 规定）。
var errLiveClaimHolds = errors.New("live consumer holds the lease; retry after it settles")

// quarantineReplayClaimLease 是毕业 TryClaim 的租约时长。MarkFailed 紧随其后（同请求内），
// 租约只是让 TryClaim 的占位合法（chk_processing_owner 三元组），不承载真实等待。
const quarantineReplayClaimLease = 5 * time.Minute

// casToReplaying 是 qrClaim 的生产实现：CAS QUARANTINED→REPLAYING（或超时 REPLAYING 重claim）。
// OR 谓词写成单个自带括号的字符串——gorm 多 Where 逐条 AND 拼接，把含 OR 的裸串拆出第二个
// Where 再依赖 builder 的 wrapInParentheses 推断（clause/where.go:65-67）是隐式契约；显式括号
// 让 tenant/version 作用域对 OR 短路免疫。0 行 → *ConflictError（版本不符 / 状态谓词未命中）。
func (s *Service) casToReplaying(ctx context.Context, db *gorm.DB, id int64, tenantID int, expectedVersion int64, watchdogCutoff time.Time) error {
	now := time.Now().UTC()
	res := db.WithContext(ctx).Model(&gormshared.QuarantineModel{}).
		Where("id = ? AND tenant_id = ? AND row_version = ? AND (status = ? OR (status = ? AND updated_at < ?))",
			id, tenantID, expectedVersion,
			quarantineStatusQuarantined, quarantineStatusReplaying, watchdogCutoff).
		Updates(map[string]any{
			"status": quarantineStatusReplaying, "updated_at": now,
			"row_version": gorm.Expr("row_version + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return &ConflictError{Reason: conflictReasonRowVersion}
	}
	return nil
}

// casBackToQuarantined 是 qrBack 的生产实现：REPLAYING→QUARANTINED，row_version+1
// （replayRowVer 是 →REPLAYING 之前的版本，+1 即 REPLAYING 态的版本，再 +1 落回）、
// replay_attempts+1（OV⑤③ 的计数——控制器裁定：每次 REPLAYING→QUARANTINED 失败迁移都计，
// 成功毕业不重置，RESOLVED 是终态）、error_message = sanitize 后的 cause（§10 脱敏地板——
// cause 可能携带 DSN/凭证/PII）。
// 0 行（并发迁移：另一操作者 / watchdog sweep 先动了）幂等返回 nil——行已不在我们手里，
// 「离开 REPLAYING」的目的可能已被并发方达成；真 DB 错误照常上抛（滞留 REPLAYING 由
// OV⑤④ watchdog 自愈，调用方以 errors.Join 双抛感知）。
func (s *Service) casBackToQuarantined(ctx context.Context, db *gorm.DB, id int64, tenantID int, replayRowVer int64, cause error) error {
	res := db.WithContext(ctx).Model(&gormshared.QuarantineModel{}).
		Where("id = ? AND tenant_id = ? AND status = ? AND row_version = ?",
			id, tenantID, quarantineStatusReplaying, replayRowVer+1).
		Updates(map[string]any{
			"status":          quarantineStatusQuarantined,
			"error_message":   reliable.SanitizeForStorage(fmt.Sprintf("%v", cause)),
			"replay_attempts": gorm.Expr("replay_attempts + 1"),
			"updated_at":      time.Now().UTC(),
			"row_version":     gorm.Expr("row_version + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	return nil // 0 行 = 并发迁移，幂等 nil（见函数头）。
}

// casToResolved 是 qrResolve 的生产实现：REPLAYING→RESOLVED（毕业 / AlreadySettled 幂等共用）。
// 0 行同样幂等 nil——另一路径（QuarantineResolve / watchdog）已把行移走，目的达成。
//
// review I-3：不再写 error_message——毕业/幂等结算不得覆盖 DLQ 落隔离时记下的原始 quarantine
// cause（隔离行是证据系统的 incident 记录；兄弟路径 gormshared.MarkResolved（quarantine.go:99-102）
// 同纪律：只写 status/resolved_at/resolved_by/row_version）。原先用 "graduated into replay path"
// / "already settled…" 文案 clobber 掉原始 cause，事后取证无从回答「这条毒消息当初为什么进来」。
// 毕业的审计轨迹由 event_consumption 侧承载（MarkFailed 产物 + resolved_by=操作者）。
func (s *Service) casToResolved(ctx context.Context, db *gorm.DB, id int64, tenantID int, replayRowVer int64, by string) error {
	now := time.Now().UTC()
	res := db.WithContext(ctx).Model(&gormshared.QuarantineModel{}).
		Where("id = ? AND tenant_id = ? AND status = ? AND row_version = ?",
			id, tenantID, quarantineStatusReplaying, replayRowVer+1).
		Updates(map[string]any{
			"status": quarantineStatusResolved, "resolved_by": by,
			"resolved_at": now,
			"updated_at":  now,
			"row_version": gorm.Expr("row_version + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	return nil
}

// derefTime 解 *time.Time（nil → 零值），Delivery.BrokerTimestamp 是值类型。
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
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
