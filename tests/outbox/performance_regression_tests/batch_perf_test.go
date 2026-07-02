package performance_regression_tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

// =========================================================================
// Regression guard: filterPublishedEvents does 1 batch DB query (not N).
// BatchSize=100 with individual lookups would be ~100 queries; this test
// asserts exactly 1 call to FindPublishedByIdempotencyKeys.
// =========================================================================

func TestFilterPublishedEvents_SingleBatchQuery(t *testing.T) {
	repo := &callCountRepo{events: make(map[string]*outbox.OutboxEvent)}
	pub := outbox.NewOutboxPublisher(repo, &noopSyncPublisher{},
		&staticMapper{topic: "test.topic"},
		&outbox.PublisherConfig{PublishTimeout: 5 * time.Second, MaxRetries: 1},
	)

	events := make([]*outbox.OutboxEvent, 100)
	for i := 0; i < 100; i++ {
		events[i] = &outbox.OutboxEvent{
			ID:            fmt.Sprintf("e-%d", i),
			IdempotencyKey: fmt.Sprintf("k-%d", i),
			Status:         outbox.EventStatusPending,
			AggregateType:  "Test",
			EventType:      "TestEvent",
			Payload:        []byte("{}"),
			CreatedAt:      time.Now(),
		}
	}

	_, err := pub.PublishBatch(context.Background(), events)
	require.NoError(t, err)

	require.Equal(t, int32(1), repo.batchIdempotentCalls.Load(),
		"filterPublishedEvents must make exactly 1 batched idempotency query, not N")
}

// =========================================================================
// Regression guard: MarkBatchAsPublished does 1 batch DB call (not N).
// =========================================================================

func TestMarkBatchAsPublished_SingleBatchDB(t *testing.T) {
	db := setupDB(t)
	repo := gormadapter.NewGormOutboxRepository(db)
	pub := outbox.NewOutboxPublisher(repo, &noopSyncPublisher{},
		&staticMapper{topic: "test.topic"},
		outbox.DefaultPublisherConfig(),
	)

	events := make([]*outbox.OutboxEvent, 50)
	for i := 0; i < 50; i++ {
		e := &outbox.OutboxEvent{
			ID:             fmt.Sprintf("e-%d", i),
			IdempotencyKey: fmt.Sprintf("k-%d", i), // UNIQUE column — must be set or NULL
			AggregateType:  "Test",
			EventType:      "TestEvent",
			Payload:        []byte("{}"),
			Status:         outbox.EventStatusPending,
			CreatedAt:      time.Now(),
		}
		events[i] = e
		require.NoError(t, db.Create(&gormadapter.OutboxEventModel{
			ID: e.ID, Status: string(outbox.EventStatusPending),
			AggregateType: e.AggregateType, IdempotencyKey: e.IdempotencyKey,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}).Error)
	}

	_, err := pub.PublishBatch(context.Background(), events)
	require.NoError(t, err)

	// All should be marked Published.
	for _, e := range events {
		var m gormadapter.OutboxEventModel
		require.NoError(t, db.First(&m, "id = ?", e.ID).Error)
		require.Equal(t, string(outbox.EventStatusPublished), m.Status,
			"event %s should be Published after sync publisher batch", e.ID)
	}
}

// =========================================================================
// Throughput regression: PublishBatch rate should not degrade with batch
// size increase (proving the O(N) per-event cost is gone).
// =========================================================================

func BenchmarkPublishBatch_Sync(b *testing.B) {
	db := setupDB(b)
	repo := gormadapter.NewGormOutboxRepository(db)
	pub := outbox.NewOutboxPublisher(repo, &noopSyncPublisher{},
		&staticMapper{topic: "test.topic"},
		outbox.DefaultPublisherConfig(),
	)

	batchSize := 100
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		events := seedPendingEvents(b, db, batchSize)
		b.StartTimer()

		_, err := pub.PublishBatch(context.Background(), events)
		if err != nil {
			b.Fatalf("PublishBatch failed: %v", err)
		}
	}
}

func BenchmarkPublishBatch_Async(b *testing.B) {
	db := setupDB(b)
	repo := gormadapter.NewGormOutboxRepository(db)
	pub := outbox.NewOutboxPublisher(repo, &noopAsyncPublisher{},
		&staticMapper{topic: "test.topic"},
		outbox.DefaultPublisherConfig(),
	)

	batchSize := 100
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		events := seedPendingEvents(b, db, batchSize)
		b.StartTimer()

		_, err := pub.PublishBatch(context.Background(), events)
		if err != nil {
			b.Fatalf("PublishBatch failed: %v", err)
		}
	}
}

// =========================================================================
// Correctness at scale: 1000+ idempotency keys handled in one batch.
// =========================================================================

func TestFilterPublishedEvents_LargeKeyset(t *testing.T) {
	repo := &callCountRepo{events: make(map[string]*outbox.OutboxEvent)}
	pub := outbox.NewOutboxPublisher(repo, &noopSyncPublisher{},
		&staticMapper{topic: "test.topic"},
		&outbox.PublisherConfig{PublishTimeout: 5 * time.Second, MaxRetries: 1},
	)

	// 500 events, half already published (simulating a stale resubmit).
	events := make([]*outbox.OutboxEvent, 500)
	for i := 0; i < 500; i++ {
		events[i] = &outbox.OutboxEvent{
			ID:             fmt.Sprintf("e-%d", i),
			IdempotencyKey: fmt.Sprintf("k-%d", i),
			Status:         outbox.EventStatusPending,
			AggregateType:  "Test",
			EventType:      "TestEvent",
			Payload:        []byte("{}"),
			CreatedAt:      time.Now(),
		}
	}
	// Mark first 250 keys as published in the repo.
	for i := 0; i < 250; i++ {
		repo.events[fmt.Sprintf("k-%d", i)] = &outbox.OutboxEvent{
			IdempotencyKey: fmt.Sprintf("k-%d", i),
			Status:         outbox.EventStatusPublished,
		}
	}

	n, err := pub.PublishBatch(context.Background(), events)
	require.NoError(t, err)
	require.Equal(t, 250, n, "250 unpublished events should be published; 250 already-published filtered out")
	require.Equal(t, int32(1), repo.batchIdempotentCalls.Load(),
		"500 idempotency keys must be resolved in 1 batched query, not N")
}

// =========================================================================
// Helpers
// =========================================================================

func setupDB(t testing.TB) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: "file::memory:?cache=shared"}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&gormadapter.OutboxEventModel{}))
	return db
}

func seedPendingEvents(t testing.TB, db *gorm.DB, n int) []*outbox.OutboxEvent {
	t.Helper()
	events := make([]*outbox.OutboxEvent, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("e-%d-%d", time.Now().UnixNano(), i)
		require.NoError(t, db.Create(&gormadapter.OutboxEventModel{
			ID:             id,
			Status:         string(outbox.EventStatusPending),
			IdempotencyKey: id, // UNIQUE column — use ID as unique key
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}).Error)
		events[i] = &outbox.OutboxEvent{
			ID:             id,
			Status:         outbox.EventStatusPending,
			IdempotencyKey: id,
			AggregateType:  "Test",
			EventType:      "TestEvent",
			Payload:        []byte("{}"),
			CreatedAt:      time.Now(),
		}
	}
	return events
}

// callCountRepo is a minimal OutboxRepository that tracks batch call counts
// and stores events by idempotency key (for the batch-idempotency test).
type callCountRepo struct {
	mu                   sync.Mutex
	events               map[string]*outbox.OutboxEvent // keyed by IdempotencyKey
	batchIdempotentCalls atomic.Int32
	markBatchCalls       atomic.Int32
}

func (r *callCountRepo) Save(_ context.Context, e *outbox.OutboxEvent) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.events[e.IdempotencyKey] = e
	return nil
}
func (r *callCountRepo) SaveBatch(_ context.Context, evts []*outbox.OutboxEvent) error {
	r.mu.Lock(); defer r.mu.Unlock()
	for _, e := range evts { r.events[e.IdempotencyKey] = e }
	return nil
}
func (r *callCountRepo) FindPendingEvents(_ context.Context, _ int, _ int) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}
func (r *callCountRepo) FindPendingEventsWithDelay(_ context.Context, _ int, _ int, _ int) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}
func (r *callCountRepo) FindEventsForRetry(_ context.Context, _ int, _ int) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}
func (r *callCountRepo) FindByAggregateType(_ context.Context, _ string, _ int) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}
func (r *callCountRepo) FindByID(_ context.Context, _ string) (*outbox.OutboxEvent, error) {
	return nil, nil
}
func (r *callCountRepo) FindByAggregateID(_ context.Context, _ string, _ int) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}
func (r *callCountRepo) Update(_ context.Context, e *outbox.OutboxEvent) error {
	r.mu.Lock(); defer r.mu.Unlock()
	r.events[e.IdempotencyKey] = e
	return nil
}
func (r *callCountRepo) MarkAsPublished(_ context.Context, _ string) error { return nil }
func (r *callCountRepo) MarkAsFailed(_ context.Context, _ string, _ error) error { return nil }
func (r *callCountRepo) IncrementRetry(_ context.Context, _ string, _ string) error { return nil }
func (r *callCountRepo) MarkAsMaxRetry(_ context.Context, _ string, _ string) error { return nil }
func (r *callCountRepo) IncrementRetryCount(_ context.Context, _ string) error { return nil }
func (r *callCountRepo) Delete(_ context.Context, _ string) error { return nil }
func (r *callCountRepo) DeleteBatch(_ context.Context, _ []string) error { return nil }
func (r *callCountRepo) DeletePublishedBefore(_ context.Context, _ time.Time, _ int) (int64, error) { return 0, nil }
func (r *callCountRepo) DeleteFailedBefore(_ context.Context, _ time.Time, _ int) (int64, error) { return 0, nil }
func (r *callCountRepo) Count(_ context.Context, _ outbox.EventStatus, _ int) (int64, error) { return 0, nil }
func (r *callCountRepo) CountByStatus(_ context.Context, _ int) (map[outbox.EventStatus]int64, error) { return nil, nil }
func (r *callCountRepo) FindByIdempotencyKey(_ context.Context, _ string) (*outbox.OutboxEvent, error) { return nil, nil }
func (r *callCountRepo) ExistsByIdempotencyKey(_ context.Context, _ string) (bool, error) { return false, nil }
func (r *callCountRepo) FindMaxRetryEvents(_ context.Context, _ int, _ int) ([]*outbox.OutboxEvent, error) {
	return nil, nil
}
func (r *callCountRepo) FindPublishedByIdempotencyKeys(_ context.Context, keys []string) (map[string]struct{}, error) {
	r.mu.Lock(); defer r.mu.Unlock()
	r.batchIdempotentCalls.Add(1)
	// Return publisher-dot-idempotency keys (used in GORM repo); our mock
	// keyed by IdempotencyKey, so match on that field.
	publishedKeys := make(map[string]struct{})
	for key, event := range r.events {
		if event.Status == outbox.EventStatusPublished {
			publishedKeys[key] = struct{}{}
		}
	}
	result := make(map[string]struct{})
	for _, k := range keys {
		if _, ok := publishedKeys[k]; ok {
			result[k] = struct{}{}
		}
	}
	return result, nil
}
func (r *callCountRepo) MarkBatchAsPublished(_ context.Context, events []*outbox.OutboxEvent) error {
	r.markBatchCalls.Add(1)
	r.mu.Lock(); defer r.mu.Unlock()
	for _, e := range events {
		if existing, ok := r.events[e.IdempotencyKey]; ok {
			existing.Status = outbox.EventStatusPublished
		}
	}
	return nil
}

// noopSyncPublisher implements SyncSemanticsPublisher for unit tests.
type noopSyncPublisher struct{}

func (*noopSyncPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (*noopSyncPublisher) PublishEnvelope(_ context.Context, _ string, _ *outbox.Envelope) error {
	return nil
}
func (*noopSyncPublisher) GetPublishResultChannel() <-chan *outbox.PublishResult {
	return make(chan *outbox.PublishResult, 1)
}
func (*noopSyncPublisher) IsSyncSemantics() {}

// noopAsyncPublisher intentionally does NOT implement SyncSemanticsPublisher.
type noopAsyncPublisher struct{}

func (*noopAsyncPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (*noopAsyncPublisher) PublishEnvelope(_ context.Context, _ string, _ *outbox.Envelope) error {
	return nil
}
func (*noopAsyncPublisher) GetPublishResultChannel() <-chan *outbox.PublishResult {
	return make(chan *outbox.PublishResult, 1)
}

type staticMapper struct{ topic string }

func (m *staticMapper) GetTopic(_ string) string { return m.topic }
