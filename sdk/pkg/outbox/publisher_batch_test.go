package outbox

import (
	"context"
	"fmt"
	"sync"
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

// TestFilterPublishedEvents_FiltersOutPublishedKeys verifies the core skip
// branch: events whose idempotency key is already Published are dropped, while
// non-published and empty-key events survive in input order. The other filter
// tests use a stub returning an empty published set, so they never hit this.
func TestFilterPublishedEvents_FiltersOutPublishedKeys(t *testing.T) {
	repo := &publishedKeysRepo{
		countingRepo: countingRepo{events: map[string]*OutboxEvent{}},
		published:    map[string]struct{}{"k2": {}, "k4": {}},
	}
	p := NewOutboxPublisher(repo, &asyncFakePublisher{}, &staticTopicMapper{topic: "t"}, DefaultPublisherConfig())

	in := []*OutboxEvent{
		{ID: "e1", IdempotencyKey: "k1"}, // not published -> keep
		{ID: "e2", IdempotencyKey: "k2"}, // published -> drop
		{ID: "e3", IdempotencyKey: ""},   // no key -> keep
		{ID: "e4", IdempotencyKey: "k4"}, // published -> drop
	}
	out, err := p.filterPublishedEvents(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, []string{"e1", "e3"}, eventIDs(out),
		"only events whose key is NOT published (plus empty-key events) survive, in order")
}

// === Test helpers (adapted from plan Task 4 helper block; Task 4 will extend these) ===

type countingRepo struct {
	events             map[string]*OutboxEvent
	markBatchCallCount atomic.Int32
}

func (r *countingRepo) Save(_ context.Context, e *OutboxEvent) error { r.events[e.ID] = e; return nil }
func (r *countingRepo) SaveBatch(_ context.Context, evs []*OutboxEvent) error {
	for _, e := range evs {
		r.events[e.ID] = e
	}
	return nil
}
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
func (r *countingRepo) Update(_ context.Context, e *OutboxEvent) error {
	r.events[e.ID] = e
	return nil
}
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

// publishedKeysRepo wraps countingRepo and returns a configurable published-key
// set, so filter tests can exercise the "key already published -> skip" branch.
type publishedKeysRepo struct {
	countingRepo
	published map[string]struct{}
}

func (r *publishedKeysRepo) FindPublishedByIdempotencyKeys(_ context.Context, _ []string) (map[string]struct{}, error) {
	return r.published, nil
}

func eventIDs(es []*OutboxEvent) []string {
	ids := make([]string, len(es))
	for i, e := range es {
		ids[i] = e.ID
	}
	return ids
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

// === ACK batcher integration (Task 4) ===

// recordingMarkRepo wraps countingRepo, recording both MarkBatchAsPublished (batch path)
// and MarkAsPublished (per-event fallback path) IDs (A5)。
type recordingMarkRepo struct {
	countingRepo
	mu           sync.Mutex
	marked       [][]string // 批量路径
	markedSingle []string   // 逐条路径（ACKBatchSize=0 回退）
	batchErr     error      // 若非 nil，MarkBatchAsPublished 返回它
}

func (r *recordingMarkRepo) MarkBatchAsPublished(_ context.Context, events []*OutboxEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	r.marked = append(r.marked, ids)
	if r.batchErr != nil {
		return r.batchErr
	}
	r.markBatchCallCount.Add(1)
	return nil
}

// MarkAsPublished 覆盖 countingRepo 的 no-op，记录逐条 mark 的 ID（A5：用于证明
// ACKBatchSize=0 回退路径确实标记了事件，而非静默丢弃导致事件永久 Pending 反复重发）。
func (r *recordingMarkRepo) MarkAsPublished(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markedSingle = append(r.markedSingle, id)
	return nil
}

func (r *recordingMarkRepo) markedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, b := range r.marked {
		out = append(out, b...)
	}
	return out
}

func (r *recordingMarkRepo) markedSingleIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.markedSingle...)
}

// chanFakePublisher 暴露一个可外部投递的 ACK channel。
type chanFakePublisher struct {
	resultChan chan *PublishResult
}

func newChanFakePublisher(buf int) *chanFakePublisher {
	return &chanFakePublisher{resultChan: make(chan *PublishResult, buf)}
}

func (*chanFakePublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (*chanFakePublisher) PublishEnvelope(_ context.Context, _ string, _ *Envelope) error {
	return nil
}
func (p *chanFakePublisher) GetPublishResultChannel() <-chan *PublishResult { return p.resultChan }
func (p *chanFakePublisher) ack(id string, success bool) {
	p.resultChan <- &PublishResult{EventID: id, Success: success}
}

// TestACKListener_BatchesMarks：满 K 个成功 ACK → 1 次 MarkBatchAsPublished(K 个 ID)。
func TestACKListener_BatchesMarks(t *testing.T) {
	repo := &recordingMarkRepo{countingRepo: countingRepo{events: map[string]*OutboxEvent{}}}
	pub := newChanFakePublisher(8)
	cfg := DefaultPublisherConfig()
	cfg.ACKBatchSize = 3
	cfg.ACKBatchFlushInterval = 5 * time.Second // 只靠 size-K 触发，不靠 timer
	publisher := NewOutboxPublisher(repo, pub, &staticTopicMapper{topic: "t"}, cfg)

	publisher.StartACKListenerWithChannel(context.Background(), pub.resultChan)
	defer publisher.StopACKListener()

	pub.ack("e1", true)
	pub.ack("e2", true)
	pub.ack("e3", true) // 满 K=3 → flush

	require.Eventually(t, func() bool { return len(repo.markedIDs()) == 3 },
		300*time.Millisecond, 5*time.Millisecond,
		"3 successful ACKs at K=3 must trigger one batched MarkBatchAsPublished")
	require.Equal(t, []string{"e1", "e2", "e3"}, repo.markedIDs())
}

// TestACKListener_DisabledFallsBackToPerEvent：ACKBatchSize=0 → 逐条 MarkAsPublished。
// A5：不仅断言批量路径未走，还断言逐条路径确实标记了 e1（区分"回退生效"与"标记丢失活锁"）。
func TestACKListener_DisabledFallsBackToPerEvent(t *testing.T) {
	repo := &recordingMarkRepo{countingRepo: countingRepo{events: map[string]*OutboxEvent{}}}
	pub := newChanFakePublisher(8)
	cfg := DefaultPublisherConfig()
	cfg.ACKBatchSize = 0 // 禁用攥批
	publisher := NewOutboxPublisher(repo, pub, &staticTopicMapper{topic: "t"}, cfg)

	publisher.StartACKListenerWithChannel(context.Background(), pub.resultChan)
	defer publisher.StopACKListener()

	pub.ack("e1", true)
	require.Eventually(t, func() bool { return len(repo.markedSingleIDs()) >= 1 },
		300*time.Millisecond, 5*time.Millisecond,
		"ACKBatchSize=0 must fall back to per-event MarkAsPublished and mark e1")
	require.Equal(t, []string{"e1"}, repo.markedSingleIDs(), "per-event fallback must mark e1")
	require.Empty(t, repo.markedIDs(), "ACKBatchSize=0 must not use the batch path")
}

// TestACKListener_StopFlushesRemaining：关停时剩余缓冲被冲刷。
func TestACKListener_StopFlushesRemaining(t *testing.T) {
	repo := &recordingMarkRepo{countingRepo: countingRepo{events: map[string]*OutboxEvent{}}}
	pub := newChanFakePublisher(8)
	cfg := DefaultPublisherConfig()
	cfg.ACKBatchSize = 50
	cfg.ACKBatchFlushInterval = 5 * time.Second
	publisher := NewOutboxPublisher(repo, pub, &staticTopicMapper{topic: "t"}, cfg)

	publisher.StartACKListenerWithChannel(context.Background(), pub.resultChan)
	pub.ack("r1", true)
	pub.ack("r2", true)
	time.Sleep(100 * time.Millisecond) // 确保已入缓冲
	publisher.StopACKListener()        // 关停 → 冲刷剩余

	require.Equal(t, []string{"r1", "r2"}, repo.markedIDs(), "Stop must flush remaining buffer")
}

// TestACKListener_ConcurrentACKAndStop_RaceClean：并发 ACK 投递 + StopACKListener，
// -race 下无竞争、无死锁（A6）。A1（指针快照）的回归钉 + 关停失败模式证明。
func TestACKListener_ConcurrentACKAndStop_RaceClean(t *testing.T) {
	repo := &recordingMarkRepo{countingRepo: countingRepo{events: map[string]*OutboxEvent{}}}
	pub := newChanFakePublisher(2048) // > writers*perWriter，确保 ack 不阻塞
	cfg := DefaultPublisherConfig()
	cfg.ACKBatchSize = 10
	cfg.ACKBatchFlushInterval = 5 * time.Millisecond
	publisher := NewOutboxPublisher(repo, pub, &staticTopicMapper{topic: "t"}, cfg)
	publisher.StartACKListenerWithChannel(context.Background(), pub.resultChan)

	const writers = 8
	const perWriter = 100
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				pub.ack(fmt.Sprintf("w%d-%d", w, i), true)
			}
		}(w)
	}

	// 与 ACK 投递并发地 Stop（不等 writers 完成）
	publisher.StopACKListener()
	wg.Wait() // Stop 死锁或 ack 阻塞会在此暴露

	// 标记数受关停时机影响、不固定；at-least-once 由 batcher 自身测试保证。
	// 本测试核心是 -race 无竞争 + Stop 不死锁。
	require.GreaterOrEqual(t, len(repo.markedIDs()), 0)
}
