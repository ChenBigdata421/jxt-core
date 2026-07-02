package outbox

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFilterPublishedEvents_PreservesInputOrder verifies the rewritten
// filterPublishedEvents returns events in the SAME order as the input slice.
// A map-keyed rewrite would randomize order and break per-aggregate publish order.
func TestFilterPublishedEvents_PreservesInputOrder(t *testing.T) {
	repo := &countingRepo{events: map[string]*OutboxEvent{}}
	p := NewOutboxPublisher(repo, &asyncFakePublisher{}, &staticTopicMapper{topic: "t"}, DefaultPublisherConfig())

	in := []*OutboxEvent{
		{ID: "e1", IdempotencyKey: "k1"},
		{ID: "e2", IdempotencyKey: "k2"},
		{ID: "e3", IdempotencyKey: "k3"},
		{ID: "e4", IdempotencyKey: "k4"},
		{ID: "e5", IdempotencyKey: "k5"},
	}
	out, err := p.filterPublishedEvents(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 5)
	for i, e := range out {
		require.Equalf(t, in[i].ID, e.ID, "order must be preserved at index %d", i)
	}
}

// TestFilterPublishedEvents_KeepsIntraBatchDuplicates verifies that when a batch
// contains two events sharing the same non-empty idempotency key (and neither is
// published), BOTH survive. A map keyed by idempotency key would collapse them.
func TestFilterPublishedEvents_KeepsIntraBatchDuplicates(t *testing.T) {
	repo := &countingRepo{events: map[string]*OutboxEvent{}}
	p := NewOutboxPublisher(repo, &asyncFakePublisher{}, &staticTopicMapper{topic: "t"}, DefaultPublisherConfig())

	in := []*OutboxEvent{
		{ID: "e1", IdempotencyKey: "dup"},
		{ID: "e2", IdempotencyKey: "dup"},
	}
	out, err := p.filterPublishedEvents(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 2, "intra-batch duplicate keys must not be collapsed")
}

// TestFilterPublishedEvents_EmptyKeysPassthrough verifies events with no
// idempotency key are all returned (order preserved) without any DB lookup.
func TestFilterPublishedEvents_EmptyKeysPassthrough(t *testing.T) {
	repo := &countingRepo{events: map[string]*OutboxEvent{}}
	p := NewOutboxPublisher(repo, &asyncFakePublisher{}, &staticTopicMapper{topic: "t"}, DefaultPublisherConfig())

	in := []*OutboxEvent{{ID: "e1"}, {ID: "e2"}, {ID: "e3"}}
	out, err := p.filterPublishedEvents(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, "e1", out[0].ID)
}

// === Test helpers (adapted from plan Task 4 helper block; Task 4 will extend these) ===

type countingRepo struct {
	events             map[string]*OutboxEvent
	markBatchCallCount atomic.Int32
}

func (r *countingRepo) Save(_ context.Context, e *OutboxEvent) error          { r.events[e.ID] = e; return nil }
func (r *countingRepo) SaveBatch(_ context.Context, evs []*OutboxEvent) error { for _, e := range evs { r.events[e.ID] = e }; return nil }
func (r *countingRepo) FindPendingEvents(_ context.Context, _ int, _ int) ([]*OutboxEvent, error) {
	return nil, nil
}
func (r *countingRepo) FindPendingEventsWithDelay(_ context.Context, _ int, _ int, _ int) ([]*OutboxEvent, error) {
	return nil, nil
}
func (r *countingRepo) FindEventsForRetry(_ context.Context, _ int, _ int) ([]*OutboxEvent, error) {
	return nil, nil
}
func (r *countingRepo) FindByAggregateType(_ context.Context, _ string, _ int) ([]*OutboxEvent, error) {
	return nil, nil
}
func (r *countingRepo) FindByID(_ context.Context, _ string) (*OutboxEvent, error) { return nil, nil }
func (r *countingRepo) FindByAggregateID(_ context.Context, _ string, _ int) ([]*OutboxEvent, error) {
	return nil, nil
}
func (r *countingRepo) Update(_ context.Context, e *OutboxEvent) error          { r.events[e.ID] = e; return nil }
func (r *countingRepo) MarkAsPublished(_ context.Context, _ string) error       { return nil }
func (r *countingRepo) MarkAsFailed(_ context.Context, _ string, _ error) error { return nil }
func (r *countingRepo) IncrementRetry(_ context.Context, _ string, _ string) error {
	return nil
}
func (r *countingRepo) MarkAsMaxRetry(_ context.Context, _ string, _ string) error { return nil }
func (r *countingRepo) IncrementRetryCount(_ context.Context, _ string) error      { return nil }
func (r *countingRepo) Delete(_ context.Context, _ string) error                   { return nil }
func (r *countingRepo) DeleteBatch(_ context.Context, _ []string) error            { return nil }
func (r *countingRepo) DeletePublishedBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (r *countingRepo) DeleteFailedBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}
func (r *countingRepo) Count(_ context.Context, _ EventStatus, _ int) (int64, error) {
	return 0, nil
}
func (r *countingRepo) CountByStatus(_ context.Context, _ int) (map[EventStatus]int64, error) {
	return nil, nil
}
func (r *countingRepo) FindByIdempotencyKey(_ context.Context, _ string) (*OutboxEvent, error) {
	return nil, nil
}
func (r *countingRepo) ExistsByIdempotencyKey(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *countingRepo) FindPublishedByIdempotencyKeys(_ context.Context, _ []string) (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}
func (r *countingRepo) FindMaxRetryEvents(_ context.Context, _ int, _ int) ([]*OutboxEvent, error) {
	return nil, nil
}
func (r *countingRepo) MarkBatchAsPublished(_ context.Context, _ []*OutboxEvent) error {
	r.markBatchCallCount.Add(1)
	return nil
}

type asyncFakePublisher struct{}

func (*asyncFakePublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (*asyncFakePublisher) PublishEnvelope(_ context.Context, _ string, _ *Envelope) error {
	return nil
}
func (*asyncFakePublisher) GetPublishResultChannel() <-chan *PublishResult {
	return make(chan *PublishResult, 1)
}

// NOTE: no IsSyncSemantics method — async publisher

type staticTopicMapper struct{ topic string }

func (m *staticTopicMapper) GetTopic(_ string) string { return m.topic }

// TestPublishBatch_SyncPublisher_MarksSynchronously verifies that when the
// EventPublisher implements SyncSemanticsPublisher, PublishBatch calls
// MarkBatchAsPublished synchronously after publish success.
func TestPublishBatch_SyncPublisher_MarksSynchronously(t *testing.T) {
	repo := &countingRepo{events: map[string]*OutboxEvent{}}
	publisher := NewOutboxPublisher(
		repo,
		&syncFakePublisher{}, // implements IsSyncSemantics
		&staticTopicMapper{topic: "test"},
		DefaultPublisherConfig(),
	)

	event := &OutboxEvent{
		ID: "e1", Status: EventStatusPending, AggregateType: "Test", EventType: "Test",
		Payload: []byte("{}"), IdempotencyKey: "",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	_, err := publisher.PublishBatch(context.Background(), []*OutboxEvent{event})
	require.NoError(t, err)
	require.Equal(t, int32(1), repo.markBatchCallCount.Load(),
		"sync publisher should trigger MarkBatchAsPublished synchronously")
}

// TestPublishBatch_AsyncPublisher_DoesNotMark verifies that when the
// EventPublisher does NOT implement SyncSemanticsPublisher, PublishBatch
// skips the synchronous MarkBatchAsPublished call (deferred to ACK listener).
func TestPublishBatch_AsyncPublisher_DoesNotMark(t *testing.T) {
	repo := &countingRepo{events: map[string]*OutboxEvent{}}
	publisher := NewOutboxPublisher(
		repo,
		&asyncFakePublisher{}, // does NOT implement IsSyncSemantics
		&staticTopicMapper{topic: "test"},
		DefaultPublisherConfig(),
	)

	event := &OutboxEvent{
		ID: "e1", Status: EventStatusPending, AggregateType: "Test", EventType: "Test",
		Payload: []byte("{}"), IdempotencyKey: "",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	_, err := publisher.PublishBatch(context.Background(), []*OutboxEvent{event})
	require.NoError(t, err)
	require.Equal(t, int32(0), repo.markBatchCallCount.Load(),
		"async publisher must NOT trigger synchronous MarkBatchAsPublished")
}

type syncFakePublisher struct{}

func (*syncFakePublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (*syncFakePublisher) PublishEnvelope(_ context.Context, _ string, _ *Envelope) error {
	return nil
}
func (*syncFakePublisher) GetPublishResultChannel() <-chan *PublishResult {
	return make(chan *PublishResult, 1)
}
func (*syncFakePublisher) IsSyncSemantics() {} // marker — qualifies as SyncSemanticsPublisher

// failingRepo wraps countingRepo and returns configurable errors for testing error paths.
type failingRepo struct {
	countingRepo
	findPublishedErr error
	markBatchErr     error
	errorHandlerSeen atomic.Int32
}

func (r *failingRepo) FindPublishedByIdempotencyKeys(_ context.Context, _ []string) (map[string]struct{}, error) {
	return nil, r.findPublishedErr
}

func (r *failingRepo) MarkBatchAsPublished(_ context.Context, _ []*OutboxEvent) error {
	if r.markBatchErr != nil {
		return r.markBatchErr
	}
	r.markBatchCallCount.Add(1)
	return nil
}

func TestPublishBatch_FindPublishedError_Propagates(t *testing.T) {
	repo := &failingRepo{
		countingRepo:     countingRepo{events: map[string]*OutboxEvent{}},
		findPublishedErr: fmt.Errorf("simulated DB timeout"),
	}
	publisher := NewOutboxPublisher(
		repo,
		&syncFakePublisher{},
		&staticTopicMapper{topic: "test"},
		DefaultPublisherConfig(),
	)

	event := &OutboxEvent{
		ID: "e1", Status: EventStatusPending, AggregateType: "Test", EventType: "Test",
		Payload: []byte("{}"), IdempotencyKey: "k1",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	_, err := publisher.PublishBatch(context.Background(), []*OutboxEvent{event})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to filter published events")
	require.Contains(t, err.Error(), "simulated DB timeout")
	require.Equal(t, int32(0), repo.markBatchCallCount.Load(),
		"MarkBatchAsPublished must not be called when FindPublished fails")
}

func TestPublishBatch_MarkBatchError_ContinuesAndInvokesErrorHandler(t *testing.T) {
	repo := &failingRepo{
		countingRepo: countingRepo{events: map[string]*OutboxEvent{}},
		markBatchErr: fmt.Errorf("simulated UPDATE failure"),
	}
	handlerCalled := atomic.Int32{}
	cfg := DefaultPublisherConfig()
	cfg.ErrorHandler = func(_ *OutboxEvent, err error) {
		handlerCalled.Add(1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "MarkBatchAsPublished failed")
	}
	publisher := NewOutboxPublisher(repo, &syncFakePublisher{}, &staticTopicMapper{topic: "test"}, cfg)

	event := &OutboxEvent{
		ID: "e1", Status: EventStatusPending, AggregateType: "Test", EventType: "Test",
		Payload: []byte("{}"), IdempotencyKey: "",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	published, err := publisher.PublishBatch(context.Background(), []*OutboxEvent{event})
	require.NoError(t, err, "PublishBatch must not fail just because MarkBatchAsPublished did")
	require.Equal(t, 1, published, "the event was still published to the bus")
	require.Equal(t, int32(1), handlerCalled.Load(),
		"ErrorHandler must be invoked exactly once when MarkBatchAsPublished fails")
}
