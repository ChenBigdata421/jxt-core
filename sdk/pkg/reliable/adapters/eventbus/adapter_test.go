package eventbusdlq_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/adapters/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"

	"gorm.io/gorm"
)

// —— in-package fakes (port file-storage's fakes, but satisfy the EXPORTED
// store.TenantStoreResolver instead of the unexported cachePort). ——

// fakeQuarantine counts Record calls; the other store.QuarantineStore methods are
// nil-embedded (the adapter only invokes Record).
type fakeQuarantine struct {
	store.QuarantineStore
	recorded int
	lastRow  store.QuarantineRow
}

func (q *fakeQuarantine) Record(_ context.Context, _ *gorm.DB, row store.QuarantineRow) (int64, error) {
	q.recorded++
	q.lastRow = row
	return 1, nil
}

// fakeStore counts RecordTerminal calls and can inject a terminalErr (test (g)).
// The other store.Store methods are nil-embedded.
type fakeStore struct {
	store.Store
	terminalCalls int
	terminalErr   error // injected error returned by RecordTerminal (test g)
	lastClass     reliable.ErrorClass
	lastIn        reliable.ClaimInput
	lastPayload   []byte
}

func (s *fakeStore) RecordTerminal(_ context.Context, _ *gorm.DB, in reliable.ClaimInput, class reliable.ErrorClass, _ error, payload []byte) error {
	s.terminalCalls++
	s.lastClass = class
	s.lastIn = in
	s.lastPayload = payload
	return s.terminalErr
}

// fakeResolver implements store.TenantStoreResolver. *fakeResolver is NOT assignable
// to either service's concrete *StoreCache — which is exactly why the resolver is an
// interface (coherence P2).
type fakeResolver struct {
	st       *fakeStore
	quar     *fakeQuarantine
	storeErr error // injected error for Store()
}

func (f *fakeResolver) Store(_ int) (store.Store, *gorm.DB, error) {
	if f.storeErr != nil {
		return nil, nil, f.storeErr
	}
	return f.st, nil, nil
}

func (f *fakeResolver) QuarantineStore(_ int) (store.QuarantineStore, error) {
	return f.quar, nil
}

// captureSink records log calls so a test can assert the log path ran. Implements
// eventbusdlq.LogSink.
type captureSink struct {
	warns  []string
	errors []string
}

func (c *captureSink) Warnf(format string, args ...any) {
	c.warns = append(c.warns, fmt.Sprintf(format, args...))
}
func (c *captureSink) Errorf(format string, args ...any) {
	c.errors = append(c.errors, fmt.Sprintf(format, args...))
}

// validEnvelopeBytes builds a parseable Envelope with the given tenant (ToBytes validates).
func validEnvelopeBytes(t *testing.T, tenantID int) []byte {
	t.Helper()
	env := &eventbus.Envelope{
		EventID:      "evt-test-001",
		AggregateID:  "file-agg-1",
		EventType:    "file.physical.deleted",
		EventVersion: 1,
		Timestamp:    time.Now().UTC(),
		TenantID:     tenantID,
		Payload:      []byte(`{"file_id":"f-1"}`),
	}
	b, err := env.ToBytes()
	if err != nil {
		t.Fatalf("envelope ToBytes: %v", err)
	}
	return b
}

const (
	testHandlerID    reliable.HandlerID = "file-storage.physical-delete"
	defaultQTenantID                    = 1
)

// (a) unparseable envelope → quarantine write + nil (ACK) — §7.
func TestEventBusDLQAdapter_UnparseableGoesToQuarantine(t *testing.T) {
	q := &fakeQuarantine{}
	resolver := &fakeResolver{st: &fakeStore{}, quar: q}
	a := eventbusdlq.NewEventBusDLQAdapter(resolver, nil, testHandlerID, defaultQTenantID, nil)

	err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "evidence.file-storage.events", Partition: 1, Offset: 2, Value: []byte("not-an-envelope"),
	}, errors.New("decode boom"))
	if err != nil {
		t.Fatalf("unparseable must quarantine+ACK (nil err), got %v", err)
	}
	if q.recorded != 1 {
		t.Fatalf("quarantine Record called %d times, want 1", q.recorded)
	}
	// HandlerID bound verbatim; SrcPartition is int32 (NOT a string conversion).
	if q.lastRow.HandlerID != testHandlerID {
		t.Fatalf("HandlerID not bound verbatim: %q", q.lastRow.HandlerID)
	}
	if q.lastRow.SrcPartition != 1 || q.lastRow.SrcOffset != 2 {
		t.Fatalf("SrcPartition/SrcOffset not preserved: %+v", q.lastRow)
	}
	if q.lastRow.TenantID != defaultQTenantID {
		t.Fatalf("quarantine row tenant must be the configured default (%d), got %d", defaultQTenantID, q.lastRow.TenantID)
	}
	// raw_payload_hash is CHAR(64) NOT NULL — must be populated.
	if len(q.lastRow.RawPayloadHash) != 64 {
		t.Fatalf("RawPayloadHash must be 64 hex chars (sha256), got len=%d", len(q.lastRow.RawPayloadHash))
	}
	// ErrorMessage must be populated with scrubbed decode+cause context.
	if q.lastRow.ErrorMessage == "" {
		t.Fatalf("ErrorMessage must be populated with decode+cause context")
	}
}

// (b) invalid tenantID → quarantine.
func TestEventBusDLQAdapter_InvalidTenantIDGoesToQuarantine(t *testing.T) {
	st := &fakeStore{}
	q := &fakeQuarantine{}
	resolver := &fakeResolver{st: st, quar: q}
	a := eventbusdlq.NewEventBusDLQAdapter(resolver, nil, testHandlerID, defaultQTenantID, nil)

	err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "evidence.file-storage.events", Partition: 3, Offset: 4,
		Value: validEnvelopeBytes(t, 0), // tenantID 0 → untrustworthy
	}, errors.New("any cause"))
	if err != nil {
		t.Fatalf("invalid-tenant must quarantine+ACK (nil err), got %v", err)
	}
	if q.recorded != 1 {
		t.Fatalf("quarantine Record must be called once for invalid tenant: %d", q.recorded)
	}
	if st.terminalCalls != 0 {
		t.Fatalf("RecordTerminal must NOT be called for invalid tenant: %d", st.terminalCalls)
	}
}

// (c) + (f) RETRYABLE cause → returns the cause (fail-closed, the P1 fix).
// context.DeadlineExceeded exercises the `class == ClassRetryable` arm.
func TestEventBusDLQAdapter_RetryableCauseFailsClosed(t *testing.T) {
	st := &fakeStore{}
	q := &fakeQuarantine{}
	resolver := &fakeResolver{st: st, quar: q}
	sink := &captureSink{}
	a := eventbusdlq.NewEventBusDLQAdapter(resolver, nil, testHandlerID, defaultQTenantID, sink)

	cause := context.DeadlineExceeded // Classify → ClassRetryable
	err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "evidence.file-storage.events", Partition: 5, Offset: 77,
		Value: validEnvelopeBytes(t, 1),
	}, cause)
	if err == nil {
		t.Fatalf("retryable cause must fail closed (return non-nil), got nil")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("retryable cause must be returned verbatim for frontier-blocking, got %v", err)
	}
	if st.terminalCalls != 0 {
		t.Fatalf("RecordTerminal must NOT be called for retryable cause (would DEAD_LETTER on attempt 1): %d calls", st.terminalCalls)
	}
	if q.recorded != 0 {
		t.Fatalf("quarantine must NOT be written for a parseable retryable cause: %d records", q.recorded)
	}
	// The retryable-refusal must route through the log sink (Warnf), not a global logger.
	if len(sink.warns) != 1 {
		t.Fatalf("retryable refusal must log exactly one Warnf via the injected sink, got %d", len(sink.warns))
	}
}

// (f) ErrRetryLaterFailsClosed — R1 errors.Is arm: a bare ErrRetryLater classifies as
// ClassUnrecoverable (not *RetryableError), so the class arm alone misses it; the
// errors.Is(cause, ErrRetryLater) arm is what catches it.
func TestEventBusDLQAdapter_ErrRetryLaterFailsClosed(t *testing.T) {
	st := &fakeStore{}
	q := &fakeQuarantine{}
	resolver := &fakeResolver{st: st, quar: q}
	a := eventbusdlq.NewEventBusDLQAdapter(resolver, nil, testHandlerID, defaultQTenantID, nil)

	cause := fmt.Errorf("handler signaled retry: %w", reliable.ErrRetryLater)
	err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "evidence.file-storage.events", Partition: 5, Offset: 78,
		Value: validEnvelopeBytes(t, 1),
	}, cause)
	if err == nil {
		t.Fatalf("ErrRetryLater cause must fail closed (return non-nil), got nil")
	}
	if !errors.Is(err, reliable.ErrRetryLater) {
		t.Fatalf("returned error must wrap ErrRetryLater for frontier-blocking, got %v", err)
	}
	if st.terminalCalls != 0 || q.recorded != 0 {
		t.Fatalf("neither path may touch storage for ErrRetryLater: terminal=%d quarantine=%d", st.terminalCalls, q.recorded)
	}
}

// (d) parseable+served+poison → RecordTerminal called with ClassPoison.
func TestEventBusDLQAdapter_PoisonCauseRecordsTerminal(t *testing.T) {
	st := &fakeStore{}
	q := &fakeQuarantine{}
	resolver := &fakeResolver{st: st, quar: q}
	a := eventbusdlq.NewEventBusDLQAdapter(resolver, nil, testHandlerID, defaultQTenantID, nil)

	payload := validEnvelopeBytes(t, 1)
	cause := reliable.Permanent(errors.New("payload schema mismatch"))
	err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "evidence.file-storage.events", Partition: 9, Offset: 100, Value: payload,
	}, cause)
	if err != nil {
		t.Fatalf("poison cause must RecordTerminal+ACK (nil err), got %v", err)
	}
	if st.terminalCalls != 1 {
		t.Fatalf("RecordTerminal must be called once for poison cause: %d", st.terminalCalls)
	}
	if st.lastClass != reliable.ClassPoison {
		t.Fatalf("RecordTerminal class must be ClassPoison, got %s", st.lastClass)
	}
	if q.recorded != 0 {
		t.Fatalf("quarantine must NOT be written for a parseable poison cause: %d", q.recorded)
	}
	if string(st.lastPayload) != string(payload) {
		t.Fatalf("RecordTerminal payload must be the raw envelope bytes")
	}
	if st.lastIn.TenantID != 1 {
		t.Fatalf("ClaimInput.TenantID must be the envelope tenant: %d", st.lastIn.TenantID)
	}
}

// (e) + (f) oversize raw value (>1 MiB) → quarantined value is capped and
// sha256(capped) == RawPayloadHash (integrity fingerprint symmetric with the LIVE path).
func TestEventBusDLQAdapter_QuarantineWritesCapPayloads(t *testing.T) {
	q := &fakeQuarantine{}
	resolver := &fakeResolver{st: &fakeStore{}, quar: q}
	a := eventbusdlq.NewEventBusDLQAdapter(resolver, nil, testHandlerID, defaultQTenantID, nil)

	const cap = 1 << 20 // 1 MiB — the documented public cap
	huge := make([]byte, cap+4096)
	for i := range huge {
		huge[i] = byte('A' + (i % 26))
	}
	err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "evidence.file-storage.events", Partition: 2, Offset: 3, Value: huge,
	}, errors.New("unparseable"))
	if err != nil {
		t.Fatalf("oversized unparseable must quarantine+ACK, got %v", err)
	}
	if q.recorded != 1 {
		t.Fatalf("quarantine Record called %d times, want 1", q.recorded)
	}
	if len(q.lastRow.RawValue) != cap {
		t.Fatalf("RawValue must be capped to %d bytes, got %d", cap, len(q.lastRow.RawValue))
	}
	// sha256 of the STORED (capped) bytes must equal RawPayloadHash.
	sum := sha256.Sum256(q.lastRow.RawValue)
	wantHash := hex.EncodeToString(sum[:])
	if q.lastRow.RawPayloadHash != wantHash {
		t.Fatalf("RawPayloadHash must be sha256(capped RawValue): got %q want %q", q.lastRow.RawPayloadHash, wantHash)
	}
}

// (g) fail-closed on conflict (F5): RecordTerminal returns ErrConflict (existing
// PROCESSING row, no token) → the adapter must PROPAGATE the error, NOT swallow it
// into nil/ACK. The adapter holds NO claim token (transport failures pre-claim), so
// RecordTerminal's conditional-insert semantics must NOT be worked around.
func TestEventBusDLQAdapter_PropagatesRecordTerminalConflict(t *testing.T) {
	st := &fakeStore{terminalErr: reliable.ErrConflict}
	q := &fakeQuarantine{}
	resolver := &fakeResolver{st: st, quar: q}
	a := eventbusdlq.NewEventBusDLQAdapter(resolver, nil, testHandlerID, defaultQTenantID, nil)

	payload := validEnvelopeBytes(t, 1)
	cause := reliable.Permanent(errors.New("poison"))
	err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "evidence.file-storage.events", Partition: 7, Offset: 9, Value: payload,
	}, cause)
	if err == nil {
		t.Fatalf("RecordTerminal ErrConflict must be PROPAGATED (non-nil), got nil — silent ACK would lose the poison")
	}
	if !errors.Is(err, reliable.ErrConflict) {
		t.Fatalf("propagated error must wrap reliable.ErrConflict, got %v", err)
	}
	if st.terminalCalls != 1 {
		t.Fatalf("RecordTerminal must be called once: %d", st.terminalCalls)
	}
}

// (h) nil log sink does not panic on the quarantine path — the DLQ path IS the
// failure path; a service that has not initialized a global logger must not panic
// here. The adapter takes an injected sink and is nil-safe (constructor normalizes
// nil to a no-op).
func TestEventBusDLQAdapter_NilLogSinkDoesNotPanic(t *testing.T) {
	q := &fakeQuarantine{}
	resolver := &fakeResolver{st: &fakeStore{}, quar: q}
	a := eventbusdlq.NewEventBusDLQAdapter(resolver, nil, testHandlerID, defaultQTenantID, nil) // nil sink

	// Exercise BOTH log sites: quarantine write (Errorf on failure) and the
	// retryable-refusal (Warnf). Neither must panic with a nil sink.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil sink must not panic on quarantine path, recovered: %v", r)
		}
	}()

	// 1. Unparseable → quarantine path (the quarantine write succeeds, so only the
	//    decode-side bookkeeping runs; still exercises the path with a nil sink).
	if err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "t", Partition: 1, Offset: 1, Value: []byte("not-an-envelope"),
	}, errors.New("decode")); err != nil {
		t.Fatalf("unparseable must quarantine+ACK, got %v", err)
	}

	// 2. Retryable-refusal path: this is the Warnf site — the failure path of the
	//    failure path, where a missing logger is most likely.
	if err := a.Send(context.Background(), eventbus.PoisonMessage{
		Topic: "t", Partition: 1, Offset: 2, Value: validEnvelopeBytes(t, 1),
	}, context.DeadlineExceeded); err == nil {
		t.Fatalf("retryable must fail closed")
	}
}
