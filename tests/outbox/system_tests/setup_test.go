//go:build system
// +build system

// Package system_tests verifies the outbox DLQ lifecycle and publisher idempotency against a
// REAL MySQL 8 database (docker-compose service `mysql`). Tests are build-tagged `system` and
// t.Skip when MySQL is unreachable, so they never run in the default `go test ./...` gate.
//
// Run: `docker-compose -f docker-compose-nats.yml up -d mysql` then
//
//	`go test -tags=system ./tests/outbox/system_tests/ -v -count=1`
package system_tests

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// mysqlDSN points at the docker-compose MySQL service. The DB `outbox_system_test` is created
// automatically by MYSQL_DATABASE in docker-compose-nats.yml.
const mysqlDSN = "root:test@tcp(127.0.0.1:13306)/outbox_system_test?charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true"

// setupMySQLDB connects to the docker-compose MySQL service (skip-if-down), AutoMigrates the
// outbox model, and clears the table for a clean per-test slate.
func setupMySQLDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(mysqlDSN), &gorm.Config{})
	if err != nil {
		t.Skipf("MySQL unavailable at %s — run `docker-compose -f docker-compose-nats.yml up -d mysql`: %v", mysqlDSN, err)
	}
	require.NoError(t, db.AutoMigrate(&gormadapter.OutboxEventModel{}))
	// OutboxEventModel has no soft-delete column — a hard DELETE is correct and avoids residue.
	require.NoError(t, db.Exec("DELETE FROM outbox_events").Error)
	return db
}

func newRepo(db *gorm.DB) outbox.OutboxRepository {
	return gormadapter.NewGormOutboxRepository(db)
}

// newMaxRetryRow seeds an outbox_events row already at status='max_retry' (pre-exhausted retries),
// which is the input to processDLQ step1 (FindMaxRetryEvents).
func newMaxRetryRow(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Create(&gormadapter.OutboxEventModel{
		ID: id, TenantID: 1, AggregateID: "agg-" + id, AggregateType: "X", EventType: "Created",
		Payload: []byte(`{}`), Status: string(outbox.EventStatusMaxRetry),
		RetryCount: 3, MaxRetries: 3,
		IdempotencyKey: id,
		CreatedAt:      now, UpdatedAt: now,
	}).Error)
}

// newPendingEvent builds a pending outbox event with an explicit idempotency key for the
// idempotency system tests. Used with repo.Save / SaveBatch (the layer that enforces the key).
func newPendingEvent(id, key string) *outbox.OutboxEvent {
	return &outbox.OutboxEvent{
		ID: id, TenantID: 1, AggregateID: "agg-" + id, AggregateType: "X", EventType: "Created",
		Payload: []byte(`{}`), Status: outbox.EventStatusPending,
		RetryCount: 0, MaxRetries: 3,
		IdempotencyKey: key,
		CreatedAt:      time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

// newScheduler builds a REAL OutboxScheduler wired to a real repo + NoOp publisher, with only the
// DLQ loop enabled (retry/cleanup/healthcheck off, pollLoop idled via a long PollInterval). The
// caller calls Start/Stop. DLQInterval must be >= 1s (SchedulerConfig.Validate rejects smaller).
func newScheduler(t *testing.T, repo outbox.OutboxRepository, dlqHandler outbox.DLQHandler, cfgOverride ...func(*outbox.SchedulerConfig)) *outbox.OutboxScheduler {
	t.Helper()
	cfg := &outbox.SchedulerConfig{
		EnableDLQ:         true,
		DLQInterval:       1 * time.Second,
		BatchSize:         50,
		DLQHandler:        dlqHandler,
		DLQAlertHandler:   &outbox.NoOpDLQAlertHandler{},
		EnableRetry:       false,
		EnableCleanup:     false,
		EnableHealthCheck: false,
		PollInterval:      60 * time.Second, // idled — no pending rows to publish during the test window
		ShutdownTimeout:   3 * time.Second,
	}
	for _, fn := range cfgOverride {
		fn(cfg)
	}
	require.NoError(t, cfg.Validate(), "scheduler config must pass Validate")
	return outbox.NewScheduler(
		outbox.WithRepository(repo),
		outbox.WithEventPublisher(outbox.NewNoOpEventPublisher()),
		outbox.WithSchedulerConfig(cfg),
	)
}

// recordingDLQHandler counts Handle invocations. processOneDLQ runs single-threaded in the dlqLoop
// goroutine, so the counter is the only state the test goroutine reads (via atomic).
type recordingDLQHandler struct{ handles int32 }

func (h *recordingDLQHandler) Handle(ctx context.Context, e *outbox.OutboxEvent) error {
	atomic.AddInt32(&h.handles, 1)
	return nil
}
func (h *recordingDLQHandler) handleCount() int32 { return atomic.LoadInt32(&h.handles) }

// failingMarkRepo wraps a real repo and fails MarkDeadLetterNotified for the first `fail` calls,
// then delegates. Used by DLQ-S3 to force a double-Handle on a notify-mark failure.
type failingMarkRepo struct {
	outbox.OutboxRepository
	fail  int32
	calls int32
}

func (r *failingMarkRepo) MarkDeadLetterNotified(ctx context.Context, id string) error {
	if atomic.AddInt32(&r.calls, 1) <= r.fail {
		return errors.New("injected MarkDeadLetterNotified failure")
	}
	return r.OutboxRepository.MarkDeadLetterNotified(ctx, id)
}

// rowNotified reads back a row and reports whether dlq_notified_at is set.
func rowNotified(t *testing.T, db *gorm.DB, id string) bool {
	t.Helper()
	var row gormadapter.OutboxEventModel
	if err := db.First(&row, "id = ?", id).Error; err != nil {
		return false
	}
	return row.DlqNotifiedAt != nil
}
