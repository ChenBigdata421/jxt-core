package gormshared

import (
	"encoding/json"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
)

// EventConsumptionModel 是 event_consumption 的 portable GORM model（§2.1）。
// type 用 portable 标签（bytes/datetime/json）由 GORM 按方言翻译；精确列类型/精度/索引由 migration SQL 定。
// 复合索引列序须与两方言 migration DDL 一致（双源原则，沿用 outbox model.go）。
type EventConsumptionModel struct {
	ID int64 `gorm:"primaryKey;autoIncrement"`

	// B5（本轮评审）：uk_event_consumption 列序必须是 (event_id, handler_id, item_key)，与两方言 DDL 逐字一致。
	// 附录 Z（2026-09-01 内核侧落地，Z.4/Z.6）：idx_handler 不再存在（tag 已删）；idx_ops 与
	// idx_aggregate 去掉前导 tenant_id（一库一租户下基数为 1，前导常量列纯属写放大浪费），本 tag
	// 与两方言 migration DDL 同步（双源原则：AutoMigrate 产物须与 SQL 产物同形）。
	EventID       string `gorm:"column:event_id;type:varchar(64);not null;uniqueIndex:uk_event_consumption,priority:1"`
	ItemKey       string `gorm:"column:item_key;type:varchar(100);not null;default:'';uniqueIndex:uk_event_consumption,priority:3"`
	HandlerID     string `gorm:"column:handler_id;type:varchar(100);not null;uniqueIndex:uk_event_consumption,priority:2"`
	TenantID      int    `gorm:"column:tenant_id;not null"`
	EventType     string `gorm:"column:event_type;type:varchar(64)"`
	AggregateType string `gorm:"column:aggregate_type;type:varchar(64);index:idx_aggregate,priority:1"`
	AggregateID   string `gorm:"column:aggregate_id;type:varchar(100);index:idx_aggregate,priority:2"`
	CausalSeq     *int64 `gorm:"column:causal_seq;index:idx_aggregate,priority:4"`
	Topic         string `gorm:"column:topic;type:varchar(100);not null"`

	// §16.12 索引合并后：idx_due → idx_replay（MySQL）/ idx_due（PG partial，同名但不同形）。
	// idx_replay **有意不进 GORM tag**（同 idx_unresolved 的双源原则）：它两方言不同形
	// ——PG 是 partial `(next_attempt_at) WHERE status='RETRY_SCHEDULED'`，MySQL 是普通复合
	// `(status, next_attempt_at, handler_id, first_seen_at)`。GORM tag 只能表达一份固定形态，
	// 加了必与其中一方分叉（AutoMigrate 产物 ≠ migration SQL）。索引只由两方言 migration
	// DDL 承载（postgres/migration.go / mysql/migration.go）。
	Status           string `gorm:"column:status;type:varchar(16);not null;index:idx_lease,priority:1;index:idx_ops,priority:1;index:idx_aggregate,priority:3"`
	Attempt          int    `gorm:"column:attempt;not null;default:1"`
	ReplayGeneration int    `gorm:"column:replay_generation;not null;default:0"`
	RowVersion       int64  `gorm:"column:row_version;not null;default:1"`

	ClaimID        string     `gorm:"column:claim_id;type:char(36)"`
	ClaimedAt      *time.Time `gorm:"column:claimed_at"`
	LeaseExpiresAt *time.Time `gorm:"column:lease_expires_at"`
	LastAttemptAt  *time.Time `gorm:"column:last_attempt_at"`

	ErrorClass       reliable.ErrorClass `gorm:"column:error_class;type:varchar(16)"`
	ErrorCode        string              `gorm:"column:error_code;type:varchar(64)"`
	ErrorFingerprint string              `gorm:"column:error_fingerprint;type:char(64)"`
	ErrorMessage     string              `gorm:"column:error_message;type:text"`
	NextAttemptAt    *time.Time          `gorm:"column:next_attempt_at"`

	ReplayMode           string     `gorm:"column:replay_mode;type:varchar(8)"`
	ReplayRequestedBy    string     `gorm:"column:replay_requested_by;type:varchar(100)"`
	ReplayApprovedBy     string     `gorm:"column:replay_approved_by;type:varchar(100)"`
	ReplayReason         string     `gorm:"column:replay_reason;type:text"`
	ReplayAuthID         string     `gorm:"column:replay_auth_id;type:char(36)"`
	ReplayAuthConsumedAt *time.Time `gorm:"column:replay_auth_consumed_at"`

	Payload      []byte `gorm:"column:payload;type:bytes"` // portable → longblob/bytea
	RawKey       []byte `gorm:"column:raw_key;type:bytes"`
	Headers      []byte `gorm:"column:headers;type:json"` // portable → json/jsonb；[]HeaderPair 的 JSON
	SrcPartition *int32 `gorm:"column:src_partition;index:idx_aggregate,priority:6"`
	SrcOffset    *int64 `gorm:"column:src_offset;index:idx_aggregate,priority:7"`

	RawPayloadHash  string     `gorm:"column:raw_payload_hash;type:char(64)"`
	BrokerTimestamp *time.Time `gorm:"column:broker_timestamp"`

	ResolvedAt    *time.Time `gorm:"column:resolved_at"`
	ResolvedBy    string     `gorm:"column:resolved_by;type:varchar(100)"`
	DiscardReason string     `gorm:"column:discard_reason;type:text"`

	// D22：first_seen_at 进 idx_aggregate 尾部——FindEligibleHeads 的 NOT EXISTS 在事件不带 causal_seq
	// 时按 first_seen_at 比较（准入 ⑩ 支持的场景），无此列会逐行回表。
	FirstSeenAt time.Time `gorm:"column:first_seen_at;not null;index:idx_ops,priority:2;index:idx_aggregate,priority:8"`
	CreatedAt   time.Time `gorm:"column:created_at;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null"`
}

func (EventConsumptionModel) TableName() string { return "event_consumption" }

// ToRow 转 store.Row。
func (m *EventConsumptionModel) ToRow() store.Row {
	return store.Row{
		ID: m.ID, EventID: m.EventID, ItemKey: m.ItemKey,
		HandlerID: reliable.HandlerID(m.HandlerID), TenantID: m.TenantID,
		EventType: m.EventType, AggregateType: m.AggregateType, AggregateID: m.AggregateID,
		CausalSeq: m.CausalSeq, Topic: m.Topic,
		Status: reliable.Status(m.Status), Attempt: m.Attempt,
		ReplayGeneration: m.ReplayGeneration, RowVersion: m.RowVersion,
		ClaimID: m.ClaimID, ClaimedAt: m.ClaimedAt, LeaseExpiresAt: m.LeaseExpiresAt, LastAttemptAt: m.LastAttemptAt,
		ErrorClass: m.ErrorClass, ErrorCode: m.ErrorCode, ErrorFingerprint: m.ErrorFingerprint,
		ErrorMessage: m.ErrorMessage, NextAttemptAt: m.NextAttemptAt,
		ReplayMode: m.ReplayMode, ReplayRequestedBy: m.ReplayRequestedBy, ReplayApprovedBy: m.ReplayApprovedBy,
		ReplayReason: m.ReplayReason, ReplayAuthID: m.ReplayAuthID, ReplayAuthConsumedAt: m.ReplayAuthConsumedAt,
		Payload: m.Payload, RawKey: m.RawKey, Headers: unmarshalHeaders(m.Headers),
		SrcPartition: m.SrcPartition, SrcOffset: m.SrcOffset,
		RawPayloadHash: m.RawPayloadHash, BrokerTimestamp: m.BrokerTimestamp,
		ResolvedAt: m.ResolvedAt, ResolvedBy: m.ResolvedBy, DiscardReason: m.DiscardReason,
		FirstSeenAt: m.FirstSeenAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// marshalHeaders 序列化 []HeaderPair 为 JSON（保留顺序与重复 key；Value []byte → base64，OV#9）。
func marshalHeaders(h []reliable.HeaderPair) []byte {
	if len(h) == 0 {
		return nil
	}
	b, _ := json.Marshal(h)
	return b
}

// marshalHeadersOrEmpty 与 marshalHeaders 同，但空 header 返回 JSON 空数组而非 nil。
// 供 raw_message_quarantine.headers（NOT NULL）使用：B7（本轮评审）——原稿空 header 返回 nil，
// 一条不带 header 的坏消息将无法写入隔离区，按 §4 语义必须上抛不 ACK → 分区阻塞（准入 ⑯ 会踩到）。
func marshalHeadersOrEmpty(h []reliable.HeaderPair) []byte {
	if b := marshalHeaders(h); b != nil {
		return b
	}
	return []byte("[]")
}

func unmarshalHeaders(b []byte) []reliable.HeaderPair {
	var h []reliable.HeaderPair
	if len(b) > 0 {
		_ = json.Unmarshal(b, &h)
	}
	return h
}

// QuarantineModel 是 raw_message_quarantine 的 portable GORM model（§2.3）。
type QuarantineModel struct {
	ID int64 `gorm:"primaryKey;autoIncrement"`
	// review #1：租户隔离（与 event_consumption.tenant_id 对齐）。idx_raw_status 首列为 tenant_id；
	// uk_raw_delivery 也含 tenant_id（priority:1，列序与两方言 DDL 一致）——与 consumption_anomalies.uk_anomaly_once
	// 同理的纵深防御（见 mysql/migration.go uk_raw_delivery 注释）。
	TenantID        int        `gorm:"column:tenant_id;not null;index:idx_raw_status,priority:1;uniqueIndex:uk_raw_delivery,priority:1"`
	HandlerID       string     `gorm:"column:handler_id;type:varchar(100);not null;uniqueIndex:uk_raw_delivery,priority:5"`
	Topic           string     `gorm:"column:topic;type:varchar(100);not null;uniqueIndex:uk_raw_delivery,priority:2"`
	SrcPartition    int32      `gorm:"column:src_partition;not null;uniqueIndex:uk_raw_delivery,priority:3"`
	SrcOffset       int64      `gorm:"column:src_offset;not null;uniqueIndex:uk_raw_delivery,priority:4"`
	RawValue        []byte     `gorm:"column:raw_value;type:bytes;not null"`
	RawKey          []byte     `gorm:"column:raw_key;type:bytes"`
	Headers         []byte     `gorm:"column:headers;type:json;not null"`
	RawPayloadHash  string     `gorm:"column:raw_payload_hash;type:char(64);not null"`
	BrokerTimestamp *time.Time `gorm:"column:broker_timestamp"`
	ErrorMessage    string     `gorm:"column:error_message;type:text"`
	Status          string     `gorm:"column:status;type:varchar(16);not null;index:idx_raw_status,priority:2"`
	RowVersion      int64      `gorm:"column:row_version;not null;default:1"`
	// PR-7 Task 3（C①）：replay_attempts 是 QuarantineReplay 的失败重放计数（见 store.QuarantineRow
	// 同名字段的注释——含为何不沿用 row_version 反推）。INT NOT NULL DEFAULT 0 两方言同形，
	// AutoMigrate 产物与 migration SQL 不分叉，故与 idx_unresolved（刻意不进 tag，D7=6A）不同，
	// 普通 COLUMN 进 GORM tag 是正确做法（双源原则：列在两处声明且同形）。
	ReplayAttempts int        `gorm:"column:replay_attempts;not null;default:0"`
	ResolvedAt     *time.Time `gorm:"column:resolved_at"`
	ResolvedBy     string     `gorm:"column:resolved_by;type:varchar(100)"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null;index:idx_raw_status,priority:3"`
	// UpdatedAt 承载 QuarantineReplay 的 OV⑤④ watchdog 谓词（REPLAYING 超时重claim）。
	// review M-1（纠正原注释）：Record 【会】写 updated_at——GORM 对名为 UpdatedAt 的字段
	// 自动施加 autoUpdateTime 追踪（gorm v1.24.2 schema/field.go:290 + callbacks/create.go，
	// Create 时零值填 curTime），故新隔离行落库即带创建时间。CAS 迁移（opsvc 三个 qr* /
	// Task 14 sweep）则显式置 now（map[string]any Updates 绕过 model 字段追踪，必须显式）。
	// 对 watchdog 是保守正确：QUARANTINED 行的 updated_at=创建时间不影响谓词（谓词只对
	// REPLAYING 行评估），列可空仅为兼容「存量行无此列值」+ 与 DDL（DATETIME(3) 无 NOT NULL）对齐。
	// Task 14 的 sweep（REPLAYING AND updated_at < now-10min → QUARANTINED）同谓词。
	UpdatedAt *time.Time `gorm:"column:updated_at"`
}

func (QuarantineModel) TableName() string { return "raw_message_quarantine" }

func (m *QuarantineModel) ToRow() store.QuarantineRow {
	return store.QuarantineRow{
		ID: m.ID, TenantID: m.TenantID, HandlerID: reliable.HandlerID(m.HandlerID), Topic: m.Topic,
		SrcPartition: m.SrcPartition, SrcOffset: m.SrcOffset,
		RawValue: m.RawValue, RawKey: m.RawKey, Headers: unmarshalHeaders(m.Headers),
		RawPayloadHash: m.RawPayloadHash, BrokerTimestamp: m.BrokerTimestamp,
		ErrorMessage: m.ErrorMessage, Status: m.Status, RowVersion: m.RowVersion,
		ReplayAttempts: m.ReplayAttempts,
		ResolvedAt:     m.ResolvedAt, ResolvedBy: m.ResolvedBy, CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

// AggregateLeaseModel 是 consumption_aggregate_leases 的 portable GORM model（§6.2.1，D17 计划补）。
type AggregateLeaseModel struct {
	TenantID      int       `gorm:"column:tenant_id;primaryKey"`
	AggregateType string    `gorm:"column:aggregate_type;type:varchar(64);primaryKey"`
	AggregateID   string    `gorm:"column:aggregate_id;type:varchar(100);primaryKey"`
	HolderID      string    `gorm:"column:holder_id;type:varchar(100);not null"` // D18#7：存唯一 token（holder+uuid）
	AcquiredAt    time.Time `gorm:"column:acquired_at;not null"`
	ExpiresAt     time.Time `gorm:"column:expires_at;not null"`
}

func (AggregateLeaseModel) TableName() string { return "consumption_aggregate_leases" }

// AnomalyModel 是 consumption_anomalies 的 portable GORM model（§2.3）。
type AnomalyModel struct {
	ID        int64  `gorm:"primaryKey;autoIncrement"`
	Kind      string `gorm:"column:kind;type:varchar(32);not null"`
	EventID   string `gorm:"column:event_id;type:varchar(64)"`
	HandlerID string `gorm:"column:handler_id;type:varchar(100)"`
	// B8（本轮评审）：原稿字段是 *int 却注释「非空」，与 DDL 三处不一致。统一为 int + NOT NULL DEFAULT 0
	// （沿用 outbox model.go 的 tenant_id 写法），两方言 DDL 同步改为 NOT NULL DEFAULT 0。
	TenantID int `gorm:"column:tenant_id;not null;default:0"` // D18#8：RecordAnomaly 必传 tenantID
	// ClaimID 是幂等键的一部分（本轮评审）：uk_anomaly_once (kind, event_id, handler_id, claim_id)。
	// ObserveExpiredLeases 每 tick 都会扫到同一个尚未被续占的孤儿行，若无此唯一键就会把
	// consumption_anomaly_total{kind="LEASE_ORPHAN"} 的「>10/h」告警用自身刷爆（告警自噪）。
	// claim_id 变化即代表「新的一次占位又成了孤儿」，应当记新的一条。
	ClaimID   string    `gorm:"column:claim_id;type:varchar(36);not null;default:''"`
	Detail    string    `gorm:"column:detail;type:text"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (AnomalyModel) TableName() string { return "consumption_anomalies" }
