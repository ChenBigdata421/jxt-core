// Package eventbusdlq is the core DLQ adapter bridging the jxt-core partition-
// pipeline DLQSender contract to the reliable store (spec §7 "handler claim 前 /
// transport 失败" path). It is the promoted, deduplicated home of file-storage's
// hardened copy (file-storage-service/internal/infrastructure/reliable/eventbus_dlq_adapter.go);
// evidence-management's drifted copy is replaced by this package in Task A3.
//
// Behavior is ported VERBATIM from file-storage's copy with these deltas
// (PR-2 Task A2):
//  1. Package eventbusdlq (this file).
//  2. The cache port is store.TenantStoreResolver, defined in sdk/pkg/reliable/store
//     (Q2=A, controller-locked) — NOT here — so opsvc (Task B2) can inject a resolver
//     without importing this package → eventbus → sarama.
//  3. The eventbus→reliable delivery helpers (FromPoisonMessage, FromRawMeta,
//     toHeaderPairs) live here because they reference eventbus types and so CANNOT
//     live at the reliable root (J2: the root must stay free of eventbus/sarama).
//  4. Sanitization uses the core root reliable.SanitizeForLog / SanitizeForStorage
//     (Task A1) — NOT a local copy.
//  5. Logging is INJECTED via LogSink (no global logger): the DLQ path IS the
//     failure path, and file-storage's copy imported jxt-core's global logger and
//     called logger.Warnf/Errorf, which panics in any service that has not
//     initialized that global — i.e. panics on the very path that handles a failed
//     delivery. A nil sink is normalized to a no-op so the DLQ path can never panic
//     on logging (regression-tested in adapter_test.go (h)).
//
// UNCHANGED from the file-storage source (load-bearing, do NOT regress):
//   - the P1 retryable-refusal block (a RETRYABLE cause or ErrRetryLater MUST NOT
//     reach RecordTerminal — RecordTerminal hardcodes DEAD_LETTER+Attempt=1 and
//     ignores the class arg, so terminalizing a retryable cause permanently loses
//     the message after the pipeline ACKs);
//   - the 1 MiB (1<<20) maxQuarantinePayloadBytes cap on the persisted raw payload;
//   - sha256(capped bytes) == raw_payload_hash so the integrity fingerprint is
//     symmetric with the LIVE path;
//   - the adapter holds NO claim token (transport failures pre-claim), so it must
//     NEVER overwrite a PROCESSING row — RecordTerminal's conditional-insert
//     semantics enforce this, and a returned ErrConflict is PROPAGATED (not
//     swallowed into nil/ACK — regression-tested in adapter_test.go (g)).
package eventbusdlq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
)

// LogSink is the minimal logging surface the DLQ adapter uses. It is INJECTED
// (never taken from a global logger): the DLQ path IS the failure path, and a
// service that has not initialized jxt-core's global sdk/logger would otherwise
// panic on the very path that handles a failed delivery. NewEventBusDLQAdapter
// normalizes a nil sink to noopSink, so callers MAY safely pass nil.
//
// The interface is exported so external services can inject their own logger
// (e.g. a zap-SugaredLogger adapter); assignability is structural. Any value whose
// method set includes Warnf(string, ...any) and Errorf(string, ...any) satisfies it.
type LogSink interface {
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// noopSink drops all log output. Used as the default when NewEventBusDLQAdapter is
// called with a nil sink so the DLQ path can never panic on logging.
type noopSink struct{}

func (noopSink) Warnf(string, ...any)  {}
func (noopSink) Errorf(string, ...any) {}

// maxQuarantinePayloadBytes bounds the persisted raw payload in
// raw_message_quarantine (security-lens P2). Reconcile against the broker's
// max.message.bytes (typically 1 MiB).
const maxQuarantinePayloadBytes = 1 << 20

// EventBusDLQAdapter bridges the partition-pipeline DLQSender contract to the
// reliable store. Construct ONE adapter per subscription with a bound stable
// HandlerID (§7); do NOT share a singleton.
//
// The adapter has NO claim token (transport failures happen before claim), so it
// must NEVER overwrite a PROCESSING row — RecordTerminal's conditional-insert
// semantics enforce this (0-row conflict on existing PROCESSING).
type EventBusDLQAdapter struct {
	resolver                store.TenantStoreResolver
	classifier              reliable.ErrorClassifier
	handlerID               reliable.HandlerID
	defaultQuarantineTenant int // config-driven; NEVER a hardcoded literal (multi-tenant safety)
	log                     LogSink
}

// NewEventBusDLQAdapter constructs a per-subscription DLQ bridge.
//   - resolver: the per-tenant TenantStoreResolver (a service's StoreCache
//     satisfies this). Provides Store(tenantID) and QuarantineStore(tenantID);
//     both fail closed via the service's ErrTenantNotServed.
//   - classifier: the dialect classifier (reliablepostgres.PGClassifier{} for
//     file-storage). Used to classify `cause` for RecordTerminal. May be nil —
//     Classify falls back to domain declarations + context/net checks.
//   - handlerID: the stable subscription HandlerID (§7). Bound here, never
//     invented — store/row.go (HandlerID is reliable.HandlerID, NOT string).
//   - defaultQuarantineTenant: the tenant ID whose raw_message_quarantine table
//     holds unparseable / invalid-identity records. Wire a dedicated "global"
//     tenant id from config (A4).
//   - log: the injected log sink. MAY be nil — normalized to a no-op so the DLQ
//     (failure) path can never panic on logging when no sink is configured.
func NewEventBusDLQAdapter(
	resolver store.TenantStoreResolver,
	classifier reliable.ErrorClassifier,
	handlerID reliable.HandlerID,
	defaultQuarantineTenant int,
	log LogSink,
) *EventBusDLQAdapter {
	if log == nil {
		log = noopSink{}
	}
	return &EventBusDLQAdapter{
		resolver:                resolver,
		classifier:              classifier,
		handlerID:               handlerID,
		defaultQuarantineTenant: defaultQuarantineTenant,
		log:                     log,
	}
}

// Send implements eventbus.DLQSender. nil = ACK (advance the partition frontier);
// non-nil = the partition blocks (strategy-A fail-closed).
func (a *EventBusDLQAdapter) Send(ctx context.Context, msg eventbus.PoisonMessage, cause error) error {
	env, err := eventbus.FromBytes(msg.Value)
	if err != nil {
		// Unparseable → quarantine the raw record (write-before-ACK). `err` is the
		// decode reason; `cause` is the downstream context.
		return a.quarantine(ctx, msg, cause, err)
	}
	// Parseable. Validate the identity before touching the main table.
	tenantID := env.TenantID
	if tenantID <= 0 {
		// No trustworthy identity → quarantine raw; cannot shape a main-table row
		// without a tenant (and cannot trust TenantID=0 to scope a write).
		return a.quarantine(ctx, msg, cause, fmt.Errorf("invalid tenantID %d", tenantID))
	}
	// Served check (D1/§7 trust boundary). Unserved → fail closed: the partition
	// blocks rather than silently dropping the record.
	st, gdb, err := a.resolver.Store(tenantID)
	if err != nil {
		return err // fail-closed → partition blocks (strategy-A), no silent loss
	}
	// Parseable + served → no-token terminal insert. RecordTerminal's
	// conditional-insert semantics enforce idempotency; we hold NO claim token
	// (transport failures pre-claim), so we cannot clobber a PROCESSING row.
	dm := FromPoisonMessage(msg)
	in := reliable.ClaimInput{
		Key: reliable.Key{
			EventID: env.EventID,
			Handler: a.handlerID,
		},
		Meta: reliable.Meta{
			EventType:   env.EventType,
			AggregateID: env.AggregateID,
		},
		TenantID: tenantID,
		Delivery: dm,
	}
	class := reliable.Classify(cause, a.classifier)
	if class == reliable.ClassSkip {
		// Defensive: a Skip on the DLQ path is nonsensical (the message already
		// failed). Promote to Poison so RecordTerminal dead-letters it.
		class = reliable.ClassPoison
	}
	// ⭐ P1 silent-loss fix (round-2 R1=A): NEVER terminalize a RETRYABLE cause.
	// `RecordTerminal` HARDCODES `Status: StatusDeadLetter, Attempt: 1` and
	// completely IGNORES the `class` argument (gormshared/mark.go — unlike
	// `MarkFailed`, which routes via `OutcomeFor(class, safety)`). So passing
	// ClassRetryable here writes a DEAD_LETTER row on attempt 1 and returns nil →
	// pipeline advances the frontier → ACK → the message is permanently lost,
	// recoverable only by a human via the PR-7 ops API.
	//
	// This is reachable on a routine transient DB fault: `process()` Phase A
	// returns a bare `TryClaim` error (plan Task 7 Step 3), and PGClassifier maps
	// 40001 (serialization_failure), 40P01 (deadlock_detected), 55P03
	// (lock_not_available) and 23503 (FK) to ClassRetryable. One PG deadlock would
	// silently discard a valid event.
	//
	// Fail closed instead: return the cause → dlqResult.ok=false → Strategy A
	// blocks the partition frontier + alerts, and the message is redelivered after
	// the next rebalance. Same posture as the `a.resolver.Store` failure above. A
	// blocked partition is observable; a lost event is not. Also covers a
	// handler-returned ErrRetryLater reaching the DLQ — Classify maps a bare
	// ErrRetryLater to ClassUnrecoverable (it is NOT a *RetryableError), so the
	// class arm alone would miss it; the errors.Is arm is what catches that path.
	if class == reliable.ClassRetryable || errors.Is(cause, reliable.ErrRetryLater) {
		a.log.Warnf("reliable: retryable cause reached DLQ — refusing to terminalize (partition will block, message redelivered on rebalance) topic=%s partition=%d offset=%d class=%s error=%s",
			msg.Topic, msg.Partition, msg.Offset, class, reliable.SanitizeForLog(fmt.Sprintf("%v", cause)))
		return cause // ← fail closed: dlqResult.ok=false → Strategy A blocks the frontier
	}
	return st.RecordTerminal(ctx, gdb, in, class, cause, msg.Value)
}

// quarantine writes the raw record to raw_message_quarantine for the configured
// default tenant (A4). Write-before-ACK: a failed write surfaces the error so the
// partition blocks (strategy-A); nil return = ACK.
//
// The adapter holds NO claim token — transport failures happen before claim — so
// it cannot clobber a PROCESSING row. QuarantineStore.Record is an INSERT
// (gormshared/quarantine.go ON CONFLICT DO NOTHING) against the tenant's pooled
// *gorm.DB; the caller opens no tx here.
func (a *EventBusDLQAdapter) quarantine(
	ctx context.Context,
	msg eventbus.PoisonMessage,
	cause error,
	decodeErr error,
) error {
	// security-lens P2: cap the persisted raw payload — QuarantineStore.Record
	// writes RawValue verbatim with no bound (raw_message_quarantine.raw_value is
	// an unbounded bytes column); a producer with topic write access could
	// otherwise grow the table until disk exhaustion, after which quarantine
	// writes fail and the partition blocks. Hash/truncate the STORED copy so
	// sha256(payload) == raw_payload_hash still holds for the capped bytes.
	// Reconcile maxQuarantinePayloadBytes against the broker's max.message.bytes.
	value := msg.Value
	if len(value) > maxQuarantinePayloadBytes {
		value = value[:maxQuarantinePayloadBytes]
	}
	qTenant := a.defaultQuarantineTenant // A4: config-driven, never a literal
	qs, err := a.resolver.QuarantineStore(qTenant)
	if err != nil {
		// Unserved quarantine tenant → fail closed (D1). Returning nil would ACK
		// and silently drop the poison message.
		return err
	}
	// Record's signature is (ctx, db *gorm.DB, row); the impl uses the PASSED db
	// (gormshared/quarantine.go), not the store's bound db, so we MUST fetch the
	// tenant pool here. No caller tx — pass the pooled *gorm.DB.
	_, gdb, err := a.resolver.Store(qTenant)
	if err != nil {
		return err
	}

	// K2: BrokerTimestamp is *time.Time (store/row.go). Take a stable address of
	// msg.Timestamp rather than relying on the compiler's temporary address (which
	// would dangle after the call returns if we stored it).
	bts := msg.Timestamp
	// N16: raw_payload_hash is CHAR(64) NOT NULL. Omitting it stores '' and the
	// quarantined record loses the M15 integrity fingerprint — the one field that
	// lets an operator later prove the bytes they are replaying are the bytes that
	// arrived. PoisonMessage carries no hash, so compute it here (sha256 of the
	// raw value — same definition core uses for event_consumption,
	// gormshared/delivery.go toRawMeta). Hash the STORED (capped) copy so the hash
	// matches the persisted RawValue.
	sum := sha256.Sum256(value)
	row := store.QuarantineRow{
		// R2: set TenantID explicitly. The column has no useful default and a zero
		// value made the row invisible to the tenant-scoped ops API (GetByID/List/
		// MarkResolved all scope by tenant_id).
		TenantID: qTenant,
		// N1: the field is reliable.HandlerID (store/row.go), NOT string —
		// `string(a.handlerID)` does not compile and is the wrong direction anyway.
		// Assign the typed value directly.
		HandlerID: a.handlerID,
		Topic:     msg.Topic,
		// K2: SrcPartition is int32 (NOT `Partition int`); SrcOffset is int64.
		SrcPartition: msg.Partition,
		SrcOffset:    msg.Offset,
		RawValue:     value,
		RawKey:       msg.Key,
		// N16: 64-char hex sha256 of the (capped) raw value.
		RawPayloadHash: hex.EncodeToString(sum[:]),
		// C1: shared header helper (preserves order + duplicate keys).
		Headers: toHeaderPairs(msg.Headers),
		// K2: *time.Time pointer.
		BrokerTimestamp: &bts,
		// F4 (§10): scrub BEFORE Record. QuarantineStore.Record stores this
		// verbatim (gormshared/quarantine.go), and a DLQ `cause` can be a GORM/
		// driver error echoing the tenant DSN. The kernel's MarkFailed /
		// RecordTerminal paths scrub inside the kernel (fingerprint.go), but the
		// quarantine path bypasses that — so we apply the canonical root scrubber
		// here (reliable.SanitizeForStorage, Task A1). Truncate-only would silently
		// drop the redaction floor — the exact bug F4 fixes.
		ErrorMessage: reliable.SanitizeForStorage(fmt.Sprintf("decode: %v; cause: %v", decodeErr, cause)),
	}
	if _, err := qs.Record(ctx, gdb, row); err != nil {
		// security-lens P2: the write-failure log line must also route the cause
		// through SanitizeForLog — a GORM/driver error can echo the tenant DSN.
		a.log.Errorf("reliable: quarantine write failed (partition will block) topic=%s partition=%d offset=%d error=%s",
			msg.Topic, msg.Partition, msg.Offset, reliable.SanitizeForLog(fmt.Sprintf("%v", err)))
		return err
	}
	return nil
}

// toHeaderPairs (C1) converts eventbus's ordered, duplicate-permitting header
// slice to the kernel's header-pair slice. Shared by FromRawMeta,
// FromPoisonMessage, and the quarantine-row construction above. Both slices are
// ordered and permit duplicate keys (headers.go); collapsing to a map would merge
// duplicates and is exactly the bug C6 fixed upstream.
func toHeaderPairs(hs []eventbus.MessageHeader) []reliable.HeaderPair {
	if len(hs) == 0 {
		return nil
	}
	hp := make([]reliable.HeaderPair, len(hs))
	for i, h := range hs {
		hp[i] = reliable.HeaderPair{Key: h.Key, Value: h.Value}
	}
	return hp
}

// FromRawMeta converts the eventbus broker-record projection (from
// EnvelopeDelivery.Raw) into the kernel-side reliable.DeliveryMeta. The kernel
// root must not import eventbus (it would drag sarama into the kernel — see
// sdk/pkg/reliable/gates_test.go J2), so this conversion lives here and is
// exported for the consuming service's LIVE delivery path. Scalar fields, RawKey,
// ordered headers, and the broker Timestamp (K2: maps to DeliveryMeta.
// BrokerTimestamp) all round-trip verbatim.
func FromRawMeta(r eventbus.RawMeta) reliable.DeliveryMeta {
	return reliable.DeliveryMeta{
		Topic:           r.Topic,
		Partition:       r.Partition,
		Offset:          r.Offset,
		BrokerTimestamp: r.Timestamp,
		PayloadHash:     r.PayloadHash,
		RawKey:          r.RawKey,
		Headers:         toHeaderPairs(r.Headers), // C1: shared helper
	}
}

// FromPoisonMessage converts a pipeline PoisonMessage (the DLQSender input) to
// DeliveryMeta. PoisonMessage.Headers is []MessageHeader (C6) and carries
// Timestamp (C6) — both set by toPoisonMessage in jxt-core
// (partition_pipeline.go). PoisonMessage carries no PayloadHash, so compute
// sha256(Value) here — the SAME definition the LIVE path (FromRawMeta ←
// RawMeta.PayloadHash) and the quarantine arm (quarantine()) use — so the
// integrity fingerprint is symmetric across LIVE and DLQ-terminaled
// event_consumption rows (N16: raw_payload_hash is CHAR(64) NOT NULL).
func FromPoisonMessage(m eventbus.PoisonMessage) reliable.DeliveryMeta {
	sum := sha256.Sum256(m.Value)
	return reliable.DeliveryMeta{
		Topic:           m.Topic,
		Partition:       m.Partition,
		Offset:          m.Offset,
		BrokerTimestamp: m.Timestamp,
		PayloadHash:     hex.EncodeToString(sum[:]),
		RawKey:          m.Key,
		Headers:         toHeaderPairs(m.Headers), // C1: shared helper
	}
}

// Compile-time: *EventBusDLQAdapter satisfies eventbus.DLQSender
// (partition_pipeline.go — Send(ctx, PoisonMessage, cause) error).
var _ eventbus.DLQSender = (*EventBusDLQAdapter)(nil)
