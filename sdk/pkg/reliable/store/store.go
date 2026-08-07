package store

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"gorm.io/gorm"
)

// Store 是 event_consumption 的唯一写入者（§2.4 不变量由此保障）。gormshared 实现同一接口，两个薄方言包注入 classifier。
//
// 事务边界（§3.3/M14）：
//   - TryClaim：不接收 *gorm.DB，NewStore 派生独立 session 独立提交（fencing token 签发点，构造期保证）。
//   - Mark*/Schedule/Discard/ClaimForReplay/AdvanceDue/MoveToDeadLetter：显式接收调用方 *gorm.DB。
//
// 租户作用域（本轮评审 S3）：FindEligibleHeads / ObserveExpiredLeases 是 tenant-agnostic（不带 tenant 谓词），
// 服务层必须把它们指向单租户 *gorm.DB（每租户独立库，见 CLAUDE.md 多租户 DB 模型）；List 强制 tenant 作用域
// （TenantID==0 拒绝，不留静默跨租户读）。跨租户隔离由「服务层 DB 绑定 + List 强制」共同保证，
// PR-3 补「第二租户行不被触碰」conformance 用例（见 PR2_SCOPE carry-over）。
//
// 标识符约定（review #19）：TryClaim/MarkSucceeded/MarkFailed/RecordTerminal/RecordAnomaly 用 reliable.Key
// （EventID+Handler+ItemKey）定位行——这些路径在 claim 之前或与 claim 同事务；ClaimForReplay 之后的处置
// （ReleaseClaim/MoveToDeadLetterWithToken）与 pre-claim 的 AdvanceDue/MoveToDeadLetter 用 ClaimForReplay
// 返回的 Row.ID（int64）定位。前者调用方持有 Key，后者持有 claim 返回的 Row。
type Store interface {
	// —— 占位与终结（§3.1）——

	// TryClaim 占位：独立提交。首次写全不可变身份与小审计字段。
	TryClaim(ctx context.Context, in reliable.ClaimInput, lease time.Duration) (reliable.ClaimToken, reliable.Decision, error)

	// MarkSucceeded 终结成功：WHERE status='PROCESSING' AND claim_id=tok；0 行返回 reliable.ErrConflict。
	MarkSucceeded(ctx context.Context, db *gorm.DB, key reliable.Key, tok reliable.ClaimToken) error

	// MarkFailed 终结失败：按 §6.1 矩阵（OutcomeFor）决定 RETRY_SCHEDULED 还是 DEAD_LETTER，一条 UPDATE 完成。
	// WHERE claim_id=tok；attempt>=max 时即使 Retryable 也落 DEAD_LETTER。
	//
	// **签名偏离 spec §3.1/§8.4（本轮评审 C4）**：spec 的 MarkFailed 是 8 参（无 maxAttempts）；本计划加 `maxAttempts`
	// 以便 Store 在同一次 UPDATE 内原子判定「attempt 耗尽→DEAD_LETTER」（避免 SELECT-then-UPDATE 的 attempt TOCTOU）。
	// PR-3 decorator 调用时必须传 maxAttempts（spec §4 Phase C 的 8 参调用需补这一参，否则编译不过）。
	MarkFailed(ctx context.Context, db *gorm.DB, key reliable.Key, tok reliable.ClaimToken,
		class reliable.ErrorClass, safety reliable.ReplaySafety, maxAttempts int,
		cause error, payload []byte) error

	// RecordTerminal 无 token：仅允许「行不存在时插入终态」或「已是同一终态时幂等返回」；
	// 遇现存 PROCESSING/其它终态返回 reliable.ErrConflict（§3.1）。
	RecordTerminal(ctx context.Context, db *gorm.DB, in reliable.ClaimInput,
		class reliable.ErrorClass, cause error, payload []byte) error

	// —— 租约孤儿观测（§3.2；批量，D14；**D20：只观测，不改行状态**）——

	// ObserveExpiredLeases 扫描 status='PROCESSING' AND lease_expires_at<NOW()，批量记
	// consumption_anomalies(kind='LEASE_ORPHAN')，返回扫到的孤儿行数。
	//
	// **D20（本轮评审）**：本方法【不】修改 event_consumption 的 status 与 ownership。
	// 原设计「批量清 ownership 但不改 status」会直接撞死 chk_processing_owner（两方言 100% 报错）；
	// 而租约孤儿行 payload IS NULL，五状态里没有任何一个能表示「无主、可再占位、无 payload」。
	// 重新占位的唯一路径是 TryClaim 发现 lease_expires_at < now 时的内联 CAS 续占（写入新 claim_id，
	// 约束全程成立，旧 token 的 Mark* 得 ErrConflict）。本方法只负责可观测性。
	ObserveExpiredLeases(ctx context.Context, now time.Time) (int, error)

	// —— 重放调度（§6.2 / §6.2.1）——

	// FindEligibleHeads 单条 eligible-head 查询（§6.2.1：NOT EXISTS 更早未解决同聚合行 + FOR UPDATE SKIP LOCKED）。
	FindEligibleHeads(ctx context.Context, now time.Time, limit int) ([]Row, error)

	// ClaimForReplay 把 RETRY_SCHEDULED（或人工授权的 DEAD_LETTER）原子转到 PROCESSING，attempt+1。
	ClaimForReplay(ctx context.Context, db *gorm.DB, id int64) (reliable.ClaimToken, Row, error)

	// ReleaseClaim 归还已占位的行（A3 本轮评审新增）：PROCESSING → RETRY_SCHEDULED，
	// 清 ownership、按退避推进 next_attempt_at，**不改 attempt**（「让路不是失败」，准入 ⑬）。
	//
	// 为什么需要它：ClaimForReplay 之后 handler 才返回 ErrRetryLater 时，行已是 PROCESSING，
	// AdvanceDue（只匹配 RETRY_SCHEDULED）会静默 0 行，行会一直卡在 PROCESSING 直到租约过期。
	// 必须出示 tok（fencing）：旧 token 归还不得覆盖新持有者。不匹配返回 reliable.ErrConflict。
	ReleaseClaim(ctx context.Context, db *gorm.DB, id int64, tok reliable.ClaimToken) error

	// —— scheduler 三分支处置（D8：经 Store 方法，禁止 raw SQL 绕过 §2.4）——

	// AdvanceDue 推进 next_attempt_at（ErrRetryLater 让路用）。不修改 attempt（attempt 只能通过 ClaimForReplay 的 CAS 原子更新）。
	AdvanceDue(ctx context.Context, db *gorm.DB, id int64) error

	// MoveToDeadLetter 把【未占位】的 RETRY_SCHEDULED 行移出自动重放队列（pre-claim 的
	// not-permitted / not-self-replayable 分支）：→ DEAD_LETTER，清 next_attempt_at 与 ownership。
	// payload / error_class 为空时返回 ErrConflict，不撞 CHECK。
	// **本轮评审 A5**：原 A2 把源态放宽到含 PROCESSING 却不要 claim token——任何调用方都能毒掉
	// 别实例在途的 claim（也是 A1 竞态的 exploiting 机制）。现仅认 RETRY_SCHEDULED（pre-claim，无需 token）；
	// 已占位的 PROCESSING 行走 MoveToDeadLetterWithToken（带 fencing），与 MarkSucceeded/MarkFailed 对称。
	MoveToDeadLetter(ctx context.Context, db *gorm.DB, id int64, reason string) error
	// MoveToDeadLetterWithToken 把【已占位】的 PROCESSING 行移到 DEAD_LETTER，须出示 claim token（A5 fencing）。
	// 供 scheduler 在 ClaimForReplay 之后命中 not-permitted/not-self-replayable 时调用；WHERE claim_id=tok
	// 保证不会毒掉别实例在途的 claim。0 行返回 reliable.ErrConflict。
	MoveToDeadLetterWithToken(ctx context.Context, db *gorm.DB, id int64, tok reliable.ClaimToken, errorClass reliable.ErrorClass, reason string) error

	// —— 人工重放授权（§6.2）——

	// ScheduleReplay 仅允许 DEAD_LETTER 以 expected row_version CAS 转到 RETRY_SCHEDULED，
	// replay_generation+1，replay_mode=MANUAL，写入一次性 replay_auth_id 与审批审计。
	// **D12：requester==approver 返回 reliable.ErrConflict**（双人确认由 Store 强制，纵深防御）。
	ScheduleReplay(ctx context.Context, db *gorm.DB, id, expectedVersion int64,
		requester, approver, reason string) error

	// Discard DEAD_LETTER → DISCARDED，CAS expected row_version。
	Discard(ctx context.Context, db *gorm.DB, id, expectedVersion int64, by, reason string) error

	// —— aggregate gate（§6.2.1）——

	// AcquireAggregateGate 获取 (tenant,type,id) 的 DB lease：INSERT 或 ON CONFLICT（若过期）覆盖。
	// **D18#7：返回的唯一 token = holder+uuid**（不是裸 holder），ReleaseAggregateGate 凭它删除，
	// 避免 holder 碰撞时误删他人 lease。被他人有效持有时返回 reliable.ErrRetryLater。
	AcquireAggregateGate(ctx context.Context, db *gorm.DB, key reliable.AggregateGateKey,
		holder string, ttl time.Duration) (token string, err error)

	ReleaseAggregateGate(ctx context.Context, db *gorm.DB, token string) error
	ReclaimExpiredAggregateGates(ctx context.Context, now time.Time) (int, error)

	// —— 异常记录（§2.3；D18#8：带 tenantID，使 anomaly 可按租户过滤/告警）——

	// claimID 参与幂等键 uk_anomaly_once (kind, tenant_id, event_id, handler_id, claim_id)：同一次占位的同类异常
	// 只记一条（本轮评审）——ObserveExpiredLeases 每 tick 都会扫到尚未被续占的同一孤儿行，
	// 若不去重就会把 consumption_anomaly_total{kind="LEASE_ORPHAN"} 的 >10/h 告警用自身刷爆。
	//
	// claimID 语义（review #25）：标识「哪一次占位成了孤儿」。TryClaim 内联续占时传【被顶替的旧 claim_id】
	// （gormshared/store.go tryClaimOnce），ObserveExpiredLeases 观测扫描时传行【当前的 claim_id】——两条路径
	// 用同一孤儿行的同一 claim_id 去重，不会重复计数。传空串会把同 (kind,tenant,event,handler) 的所有异常坍缩成一条。
	RecordAnomaly(ctx context.Context, db *gorm.DB, tenantID int, kind string, key reliable.Key, claimID, detail string) error

	// —— 读（运维 API 用；写权限/审计在服务侧）——
	// review #5：GetByID 强制 tenant 作用域（与 List 的 S3 守卫对齐）——杜绝按主键枚举跨租户裸读
	// payload/headers。运维侧若需全局视图，PR-7 另立 GetByIDGlobal（参考 ListGlobal 的 S3 carry-over）。
	GetByID(ctx context.Context, tenantID int, id int64) (Row, error)
	List(ctx context.Context, filter ListFilter) ([]Row, error)

	// —— §10 ops API 读（PR-2 upper-packages：ListAnomalies / Count / §6.2.1 manual-replay gate）——

	// ListAnomalies 读 consumption_anomalies（§10 /anomalies 视图）。S3 强制 tenant 作用域
	// （TenantID==0 拒绝，与 List 对齐——杜绝跨租户裸读异常明细）。用 store 的 BOUND db（读模式，
	// 与 List/GetByID 一致）。AnomalyFilter.From/To 约束 created_at；ORDER BY created_at DESC。
	//
	// ClaimID 投影不可省（uk_anomaly_once 的幂等键之一，见 RecordAnomaly 注释）：/anomalies 视图
	// 靠它区分「同一次占位的同类异常（应去重显示）」与「同一 orphan/event 的不同占位异常（应分别显示）」。
	ListAnomalies(ctx context.Context, filter AnomalyFilter) ([]AnomalyRow, error)

	// Count 返回 event_consumption 匹配 filter 的行数（§10 dashboard totals）。
	//
	// F6（本轮评审）：用 SQL COUNT(*)，**禁止** list-then-count——后者会让 caller 把 Limit 截断后的
	// 列表长度当总量，静默回退 F6 缺陷。CountFilter 故意是 ListFilter 减去 Limit/Offset，杜绝这条
	// 误用路径。S3 强制 tenant 作用域。From/To 约束 first_seen_at（与 ListFilter 同语义）。
	Count(ctx context.Context, filter CountFilter) (int64, error)

	// HasEarlierUnsolvedSibling 是 §6.2.1 manual-replay 409 门禁（Q1=A）。
	//
	// 自动重放路径（FindEligibleHeads）已通过其 NOT EXISTS 子查询强制「同聚合不得有更早未解决行」；
	// 人工 ScheduleReplay 不走 FindEligibleHeads，本方法为它补这条门禁——否则人工授权可越次序重放，
	// 与自动路径的行为不一致（§6.2.1 gap Q1=A）。B2 opsvc.ReplayOne 在 ScheduleReplay 之前调用本方法，
	// 返回 true → 409 Conflict。
	//
	// **显式接收 *gorm.DB（非 BOUND db）**：让 opsvc 把它与紧随其后的 ScheduleReplay 跑在【同一 db/
	// 同一 tx】上，关闭 check-then-act TOCTOU 窗口（否则两次独立读之间状态可变，门禁可被绕过）。
	// 镜像 EligibleHeadsSQL 的 NOT EXISTS 谓词（replay.go:40-54，三段式 earlier-than）；
	// aggregate-less 行（aggregate_type IS NULL OR aggregate_id = ''）→ false（通知类，自由并行，
	// 与 EligibleHeadsSQL 跳过 NOT EXISTS 同义）。
	HasEarlierUnsolvedSibling(ctx context.Context, db *gorm.DB, id int64) (bool, error)
}

// ListFilter 是运维列表查询参数（§10 API）。
type ListFilter struct {
	TenantID   int
	Status     reliable.Status
	ErrorClass reliable.ErrorClass
	HandlerID  reliable.HandlerID
	// From/To 约束的是 first_seen_at（行首次占位时间，TryClaim 时写一次、之后不变），而非 created_at/updated_at
	// —— 后者随每次状态迁移变化。若要「最近活跃」窗口请按 updated_at 另行过滤（review #26）。
	From, To time.Time
	Limit    int
	Offset   int
}

// AnomalyFilter 是 ListAnomalies 的查询参数（§10 /anomalies 视图）。
// 与 ListFilter 解耦：anomaly 的过滤维度是 kind + created_at（不是 status + first_seen_at），
// 且无 Offset——anomaly 量级远低于 event_consumption，单表 created_at DESC + Limit 已够。
type AnomalyFilter struct {
	TenantID int
	Kind     string
	// From/To 约束 created_at（anomaly 的写入时间，与 ListFilter 约束 first_seen_at 的语义不同）。
	From, To time.Time
	Limit    int
}

// AnomalyRow 是 consumption_anomalies 的领域投影（§10 /anomalies 视图）。
// ClaimID 是 uk_anomaly_once 幂等键的一部分（见 RecordAnomaly 注释）——保留它让视图能区分
// 「同一次占位的同类异常」与「同一 event 的不同占位异常」，前者应去重、后者应分别展示。
type AnomalyRow struct {
	ID        int64
	Kind      string
	EventID   string
	HandlerID reliable.HandlerID
	TenantID  int
	ClaimID   string
	Detail    string
	CreatedAt time.Time
}

// CountFilter 是 Count 的查询参数（§10 dashboard totals）。
//
// **刻意不复用 ListFilter（F6）**：ListFilter 带 Limit/Offset，COUNT(*) 无 paging 概念；
// 若复用，调用方传一个带 Limit 的 ListFilter 进 Count 会让人误以为总量被 Limit 截断——
// 即使实现忽略 Limit，类型层面也保留了那条误用路径。CountFilter 删掉 Limit/Offset，从类型
// 层面切断 F6 复发的可能。字段语义与 ListFilter 同（From/To 约束 first_seen_at）。
type CountFilter struct {
	TenantID   int
	Status     reliable.Status
	ErrorClass reliable.ErrorClass
	HandlerID  reliable.HandlerID
	From, To   time.Time
}

// QuarantineStore 是 raw_message_quarantine 的写入/读取接口（§2.3）。落库成功才 ACK。
type QuarantineStore interface {
	Record(ctx context.Context, db *gorm.DB, row QuarantineRow) (int64, error)
	// review #1：GetByID/List/MarkResolved 强制 tenant 作用域（与 event_consumption.List 的 S3 对齐）——
	// 隔离区存的是不可解码的毒消息，最可能带 PII/攻击者可控内容，绝不能跨租户裸读或裸改。
	// review #2：MarkResolved 是 ops 处置路径（凭 id+version），凭 id 泄露即可跨租户 dispose 别租户证据——
	// 故与读路径同等级地强制 tenant；跨租户命中 0 行 → ErrConflict（不泄露 id 归属，兼作枚举预言机防护）。
	GetByID(ctx context.Context, tenantID int, id int64) (QuarantineRow, error)
	List(ctx context.Context, tenantID int, status string, limit int) ([]QuarantineRow, error)
	MarkResolved(ctx context.Context, db *gorm.DB, tenantID int, id, expectedVersion int64, by string) error
}
