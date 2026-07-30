//go:build system
// +build system

package system_tests

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
	"github.com/stretchr/testify/require"
)

// DLQ-S1: the real Start/dlqLoop drives the full lifecycle on MySQL.
// Seed a max_retry row, start the scheduler, and verify it transitions to dead_lettered, the
// DLQHandler fires, dlq_notified_at is set, and (single-instance) Handle is called exactly once.
//
// Covers what the unit suite cannot: the real dlqLoop ticker -> runDLQWithRecover -> processDLQ ->
// processOneDLQ path against real MySQL CAS predicates, all glued together.
func TestSystem_DLQ_FullLifecycle(t *testing.T) {
	db := setupMySQLDB(t)
	repo := newRepo(db)
	handler := &recordingDLQHandler{}
	s := newScheduler(t, repo, handler)
	newMaxRetryRow(t, db, "ev-dlq-s1")

	require.NoError(t, s.Start(context.Background()))
	defer s.Stop(context.Background())

	// Wait up to ~4s for terminalize + notify (DLQInterval=1s -> a couple of ticks).
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if handler.handleCount() >= 1 && rowNotified(t, db, "ev-dlq-s1") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	require.GreaterOrEqualf(t, handler.handleCount(), int32(1), "DLQHandler must be invoked for the max_retry row")
	// Single-instance Handle-once: once notified, FindUnnotifiedDeadLettered returns 0, so later
	// ticks do not re-Handle. This is the OV#6 SINGLE-instance guarantee (multi-instance is OV#6's gap).
	require.Equalf(t, int32(1), handler.handleCount(), "expected Handle-once within the window; got %d (re-Handle would mean the notify mark did not stick)", handler.handleCount())

	var row gormadapter.OutboxEventModel
	require.NoError(t, db.First(&row, "id = ?", "ev-dlq-s1").Error)
	require.Equal(t, string(outbox.EventStatusDeadLettered), row.Status, "row must transition to dead_lettered")
	require.NotNil(t, row.DeadLetteredAt, "terminal timestamp dead_lettered_at must be set")
	require.NotNil(t, row.DlqNotifiedAt, "dlq_notified_at must be set after successful Handle+Alert")
}

// DLQ-S2: crash-between-steps recovery. A scheduler that died AFTER step1 (terminalize) but BEFORE
// step2 (notify) leaves an orphaned dead_lettered + dlq_notified_at IS NULL row. Seed that state
// directly, start a FRESH scheduler, and verify the next loop recovers it (notifies the orphan).
func TestSystem_DLQ_CrashBetweenStepsRecovers(t *testing.T) {
	db := setupMySQLDB(t)
	repo := newRepo(db)
	handler := &recordingDLQHandler{}

	// Simulate the post-crash state: terminal but unnotified.
	now := time.Now().UTC()
	require.NoError(t, db.Create(&gormadapter.OutboxEventModel{
		ID: "ev-dlq-s2", TenantID: 1, AggregateID: "agg", AggregateType: "X", EventType: "Created",
		Payload: []byte(`{}`), Status: string(outbox.EventStatusDeadLettered),
		DeadLetteredAt: &now, IdempotencyKey: "ev-dlq-s2",
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	s := newScheduler(t, repo, handler)
	require.NoError(t, s.Start(context.Background()))
	defer s.Stop(context.Background())

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && handler.handleCount() == 0 {
		time.Sleep(100 * time.Millisecond)
	}

	require.GreaterOrEqualf(t, handler.handleCount(), int32(1), "restarted scheduler must notify the orphaned dead_lettered row")
	require.Truef(t, rowNotified(t, db, "ev-dlq-s2"), "orphaned row must be marked notified after restart")
}

// DLQ-S3: a failed MarkDeadLetterNotified forces a second Handle (double-Handle characterization).
// Inject one failing mark; the first tick Handles + fails the mark (row stays unnotified); the
// second tick re-Handles + succeeds. Verifies the recovery contract: a notify-mark failure is not
// terminal, and the row is re-handled on the next tick.
func TestSystem_DLQ_NotifyMarkFailureCausesDoubleHandle(t *testing.T) {
	db := setupMySQLDB(t)
	repo := newRepo(db)
	handler := &recordingDLQHandler{}
	failingRepo := &failingMarkRepo{OutboxRepository: repo, fail: 1} // first mark call fails
	s := newScheduler(t, failingRepo, handler)
	newMaxRetryRow(t, db, "ev-dlq-s3")

	require.NoError(t, s.Start(context.Background()))
	defer s.Stop(context.Background())

	// Need >= 2 ticks: tick1 Handles + fails mark; tick2 Handles + succeeds. Wait up to ~5s.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if handler.handleCount() >= 2 && rowNotified(t, db, "ev-dlq-s3") {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}

	require.GreaterOrEqualf(t, handler.handleCount(), int32(2), "a failed notify-mark must cause a second Handle; got %d", handler.handleCount())
	require.Truef(t, rowNotified(t, db, "ev-dlq-s3"), "row must eventually be notified after the mark retry succeeds")
}

// DLQ-S4: Stop() does NOT cooperatively cancel an in-flight Handle (R1 characterization).
// Stop closes stopCh + wg.Wait(ShutdownTimeout) but never cancels the Start ctx, so the ctx.Err()
// guards inside processDLQ do not fire on Stop and a blocked Handle is bounded only by ShutdownTimeout.
// This test PINS that behavior: when R1 is fixed (derive cancellable ctx in Start, cancel in Stop),
// the elapsed assertion flips to "much less than ShutdownTimeout" and this test must be updated.
func TestSystem_DLQ_StopDoesNotInterruptInflightHandle(t *testing.T) {
	db := setupMySQLDB(t)
	repo := newRepo(db)
	newMaxRetryRow(t, db, "ev-dlq-s4")

	started := make(chan struct{})
	release := make(chan struct{})
	blockedHandler := outbox.DLQHandlerFunc(func(ctx context.Context, e *outbox.OutboxEvent) error {
		close(started) // signal: Handle is now in-flight inside the dlqLoop goroutine
		select {
		case <-ctx.Done(): // R1 gap: Start ctx is NOT cancelled by Stop, so this does not fire on Stop
		case <-release: // test release (cleanup)
		}
		return nil
	})
	s := newScheduler(t, repo, blockedHandler, func(cfg *outbox.SchedulerConfig) {
		cfg.ShutdownTimeout = 600 * time.Millisecond
	})
	require.NoError(t, s.Start(context.Background()))
	defer close(release) // always unblock the handler so the goroutine exits

	<-started // wait until the handler is in-flight

	stopStart := time.Now()
	err := s.Stop(context.Background())
	elapsed := time.Since(stopStart)

	require.Errorf(t, err, "Stop should return the graceful-shutdown timeout error because the in-flight Handle blocks wg.Done")
	// R1 characterization: no cooperative cancellation -> Stop waits the full ShutdownTimeout.
	require.GreaterOrEqualf(t, elapsed, 450*time.Millisecond, "Stop must wait ~ShutdownTimeout (600ms); got %v — if this drops, R1 was fixed and this test must flip", elapsed)
}
