package outbox

import (
	"context"
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
// NOTE: uses BatchUpdate (not MarkBatchAsPublished) because the interface rename
// happens in Task 4. Task 4 will rename this method and add markBatchCallCount.

type countingRepo struct {
	events map[string]*OutboxEvent
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
func (r *countingRepo) BatchUpdate(_ context.Context, _ []*OutboxEvent) error { return nil }

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
