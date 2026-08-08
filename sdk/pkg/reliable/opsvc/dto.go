// Package opsvc 是 reliable 内核的 §10 ops 服务层（PR-2 Task B2）。它把 store.Store /
// store.QuarantineStore 的底层读写收口成面向运维 API（PR-7 的 gin handler）的方法语义，
// 让「方法语义跨服务不漂移」这条不变量在内核侧保证（J1），而 handler / auth / 序列化留给各服务（J3）。
//
// 依赖卫生（J2，brief Q2=A）：opsvc 只 import store / reliable / gorm + stdlib——
// 不 import gin / prometheus / sarama / adapters-eventbus。tenant→Store+*gorm.DB+QuarantineStore
// 的解析复用 store.TenantStoreResolver（host 在 store 包正是为了让 opsvc 不被 sarama 污染）。
//
// 范围（§10）：本包覆盖 event_consumption 的列表 / 详情 / 人工重放 / 丢弃 / 统计 / 异常视图，
// 以及 raw_message_quarantine 的列表 / 详情 / 处置。§10 之外的端点由各自归属包承载：
//   - POST /api/v1/quarantine/:id/replay 需要服务侧 HandlerRegistry（重放策略是服务特定的）→ PR-7，不在本包。
//   - /outbox/dead-lettered 是 outbox 包自己的 ops 视图（不同表 / 不同生命周期）→ outbox 包。
package opsvc

import (
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
)

// ListQuery 是 List 与 Stats 的查询参数（§10）。List 投影到 store.ListFilter，Stats 投影到
// store.CountFilter（F6：Stats 用 SQL COUNT，禁止 list-then-count）。
type ListQuery struct {
	TenantID   int // 必填（S3）；0 → ErrMissingTenant。
	Status     reliable.Status
	ErrorClass reliable.ErrorClass
	HandlerID  reliable.HandlerID
	// From/To 约束 first_seen_at（与 store.ListFilter / CountFilter 同语义）。
	From, To time.Time
	Limit    int
	Offset   int
}

// ListResult 是 List 的返回。Rows 是 Detail 白名单投影——不直接透出 store.Row：store.Row 还携带
// ReplayAuthID（一次性人工重放 bearer）、ClaimID、ReplayRequestedBy/ApprovedBy/Reason、DiscardReason
// 等人员身份与内部令牌字段（store/row.go），而 List 无审计钩子，按 fail-closed 原则只能释放运维概览
// 所需的安全字段集（与 GetDetail 同构的 projectDetail 白名单）。
//
// 用白名单投影而非按字段清零（denylist）：下次 store.Row 新增敏感字段时，denylist 会因遗漏而泄露，
// 白名单（投影到不含该字段的 Detail）天然不泄。Detail 的 Payload/Headers/RawKey 恒 nil（门控字段，
// List 不开 includePayload）——需载荷走 GetDetail(includePayload=true，先审计)。
type ListResult struct {
	Rows []Detail
}

// Stats 是 Stats 的返回（§10 dashboard totals）。全部由 store.Count 填充（F6），不从 List 派生。
// ByStatus 覆盖五态（PROCESSING/SUCCEEDED/RETRY_SCHEDULED/DEAD_LETTER/DISCARDED）；
// Total 是 ByStatus 各值之和（DB 的 CHECK 约束保证 status 取值闭合，不存在五态之外的行）。
type Stats struct {
	Total    int64
	ByStatus map[reliable.Status]int64
}

// Detail 是 GetDetail 的返回——store.Row 的运维投影。Payload / Headers / RawKey 由
// includePayload 门控：false 时三者恒为 nil；true 时先审计（PrivilegedAccessConsumptionPayload）
// 再填充（fail-closed：审计失败不释放）。
type Detail struct {
	ID               int64
	EventID          string
	ItemKey          string
	HandlerID        reliable.HandlerID
	TenantID         int
	EventType        string
	AggregateType    string
	AggregateID      string
	Topic            string
	Status           reliable.Status
	Attempt          int
	ReplayGeneration int
	RowVersion       int64
	ErrorClass       reliable.ErrorClass
	ErrorCode        string
	ErrorMessage     string
	NextAttemptAt    *time.Time
	ReplayMode       string
	FirstSeenAt      time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// 门控字段（includePayload=false 时恒 nil）。
	Payload []byte
	Headers []reliable.HeaderPair
	RawKey  []byte
}

// ReplayRequest 是 ReplayOne 单行人工重放授权（§6.2 / §6.2.1）。ExpectedRowVersion 是 CAS 版本；
// Requester/Approver 必须不同（D12 双人确认，由 store.ScheduleReplay 在事务内纵深防御）。
type ReplayRequest struct {
	TenantID           int
	ID                 int64
	ExpectedRowVersion int64
	Requester          string
	Approver           string
	Reason             string
}

// BatchReplayItem 是 BatchReplay 的单行条目。D12 逐行强制，故 Requester/Approver/Reason 在每个
// item 上独立声明（调用方可在一批里重复填同一对 requester/approver）。
type BatchReplayItem struct {
	ID                 int64
	ExpectedRowVersion int64
	Requester          string
	Approver           string
	Reason             string
}

// BatchReplayRequest 是 BatchReplay 的入参。每行独立 CAS（§10「每行独立 CAS + 进度/失败明细」），
// 无跨行事务——一行冲突/出错不阻塞其余行。
type BatchReplayRequest struct {
	TenantID int
	Items    []BatchReplayItem
}

// BatchReplayRowResult 是 BatchReplay 单行结果。Ok / Conflict / Err 三态互斥：Ok=true 表示成功；
// Conflict=true 表示该行命中 §6.2.1 门禁或 CAS 版本/D12 冲突（业务可重试）；其余错误进 Err。
type BatchReplayRowResult struct {
	ID       int64
	Ok       bool
	Conflict bool
	Err      string
}

// BatchReplayResult 是 BatchReplay 的返回。Results 与入参 Items 一一对应、同序。
type BatchReplayResult struct {
	Results []BatchReplayRowResult
}

// DiscardRequest 是 Discard（DEAD_LETTER → DISCARDED）的入参（§10）。CAS ExpectedRowVersion。
type DiscardRequest struct {
	TenantID           int
	ID                 int64
	ExpectedRowVersion int64
	By                 string
	Reason             string
}

// ResolveRequest 是 QuarantineResolve（raw_message_quarantine → RESOLVED）的入参（§10）。
// CAS ExpectedRowVersion；跨租户命中（0 行）由 store 返回 ErrConflict（兼作枚举预言机防护）。
type ResolveRequest struct {
	TenantID           int
	ID                 int64
	ExpectedRowVersion int64
	By                 string
}

// AnomalyQuery 是 Anomalies（§10 /anomalies 视图）的查询参数。Kind 约束 anomaly 类型；
// From/To 约束 created_at（与 ListQuery 约束 first_seen_at 的语义不同）。
type AnomalyQuery struct {
	TenantID int // 必填（S3）。
	Kind     string
	From, To time.Time
	Limit    int
}

// QuarantineDetail 是 QuarantineList / QuarantineDetail 的返回——store.QuarantineRow 的运维投影。
// RawValue / RawKey / Headers 是隔离区最高敏感字段（不可解码的毒消息，最可能带 PII / 攻击者可控内容，
// kernel 注释 store.go:~152），由 includeRaw 强制门控：false 时三者恒 nil；true 时先审计
// （PrivilegedAccessQuarantineRaw）再填充（fail-closed）。
type QuarantineDetail struct {
	ID             int64
	TenantID       int
	HandlerID      reliable.HandlerID
	Topic          string
	SrcPartition  int32
	SrcOffset      int64
	RawPayloadHash string
	ErrorMessage   string
	Status         string
	RowVersion     int64
	ResolvedAt     *time.Time
	ResolvedBy     string
	CreatedAt      time.Time

	// 门控字段（includeRaw=false 时恒 nil）。
	RawValue []byte
	RawKey   []byte
	Headers  []reliable.HeaderPair
}
