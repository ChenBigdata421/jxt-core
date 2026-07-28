package store

import (
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
)

// Row 是 event_consumption 表的领域投影（§2.1）。GORM model 标签在 gormshared（portable）。
// 所有时间字段 UTC。
type Row struct {
	ID int64

	// 身份（M5：event_id 恒为 Envelope.EventID）
	EventID       string
	ItemKey       string
	HandlerID     reliable.HandlerID
	TenantID      int
	EventType     string
	AggregateType string
	AggregateID   string
	CausalSeq     *int64
	Topic         string

	// 状态机（§3）
	Status           reliable.Status
	Attempt          int
	ReplayGeneration int
	RowVersion       int64

	// 当前占位所有权；仅 PROCESSING 非空
	ClaimID        string
	ClaimedAt      *time.Time
	LeaseExpiresAt *time.Time
	LastAttemptAt  *time.Time

	// 失败信息（仅失败态填充）
	ErrorClass       reliable.ErrorClass
	ErrorCode        string
	ErrorFingerprint string
	ErrorMessage     string
	NextAttemptAt    *time.Time

	// 人工重放一次性授权
	ReplayMode           string // AUTO | MANUAL
	ReplayRequestedBy    string
	ReplayApprovedBy     string
	ReplayReason         string
	ReplayAuthID         string
	ReplayAuthConsumedAt *time.Time

	// 重放载荷：完整 envelope 字节，仅在需要重放时填充；nil = 不可自助重放
	Payload      []byte
	RawKey       []byte
	Headers      []reliable.HeaderPair
	SrcPartition *int32
	SrcOffset    *int64

	// M15 消费端原始 record 完整性指纹
	RawPayloadHash  string
	BrokerTimestamp *time.Time

	// 人工处置
	ResolvedAt    *time.Time
	ResolvedBy    string
	DiscardReason string

	FirstSeenAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Key 回填 Row 的身份三元组。
func (r *Row) Key() reliable.Key {
	return reliable.Key{EventID: r.EventID, Handler: r.HandlerID, ItemKey: r.ItemKey}
}

// QuarantineRow 是 raw_message_quarantine 的领域投影（§2.3）。
type QuarantineRow struct {
	ID              int64
	HandlerID       reliable.HandlerID
	Topic           string
	SrcPartition    int32
	SrcOffset       int64
	RawValue        []byte
	RawKey          []byte
	Headers         []reliable.HeaderPair
	RawPayloadHash  string
	BrokerTimestamp *time.Time
	ErrorMessage    string
	Status          string // QUARANTINED | REPLAYING | RESOLVED | DISCARDED
	RowVersion      int64
	ResolvedAt      *time.Time
	ResolvedBy      string
	CreatedAt       time.Time
}
