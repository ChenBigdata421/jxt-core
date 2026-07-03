package function_regression_tests

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	"github.com/stretchr/testify/require"
)

// handlerCountPublisher is a sync-semantics EventPublisher that counts
// PublishEnvelope calls (simulating InProcessEventPublisher handler
// invocations) so the test can verify handlers actually ran.
type handlerCountPublisher struct {
	mu           sync.Mutex
	handlerCalls atomic.Int32
	resultChan   chan *outbox.PublishResult
}

func newHandlerCountPublisher() *handlerCountPublisher {
	return &handlerCountPublisher{resultChan: make(chan *outbox.PublishResult, 64)}
}

func (p *handlerCountPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (p *handlerCountPublisher) PublishEnvelope(_ context.Context, _ string, envelope *outbox.Envelope) error {
	p.handlerCalls.Add(1)
	return nil
}
func (p *handlerCountPublisher) GetPublishResultChannel() <-chan *outbox.PublishResult {
	return p.resultChan
}
func (p *handlerCountPublisher) IsSyncSemantics() {}

// =========================================================================
// Regression guard: Scheduler poll → PublishPendingEvents → PublishBatch →
// MarkBatchAsPublished. This is the exact path security-management uses
// (sync semantics, no ACK listener, 1s poll interval).
// =========================================================================

func TestSyncScheduler_PollPublishesEvents(t *testing.T) {
	repo := NewMockRepository()
	pub := newHandlerCountPublisher()
	mapper := NewMockTopicMapper()

	cfg := outbox.DefaultSchedulerConfig()
	cfg.PollInterval = 1 * time.Second
	cfg.BatchSize = 100
	cfg.EnableRetry = false
	cfg.EnableCleanup = false
	cfg.EnableHealthCheck = false

	publisherCfg := outbox.DefaultPublisherConfig()
	publisherCfg.PublishTimeout = 5 * time.Second
	publisherCfg.MaxRetries = 1

	// Force immediate poll by using a tiny PollInterval that Validate accepts,
	// then manually tick. Actually the scheduler validates >=1s. We'll use 1s
	// and rely on Eventually with a long enough timeout.
	scheduler := outbox.NewScheduler(
		outbox.WithRepository(repo),
		outbox.WithEventPublisher(pub),
		outbox.WithTopicMapper(mapper),
		outbox.WithSchedulerConfig(cfg),
		outbox.WithPublisherConfig(publisherCfg),
	)

	helper := NewTestHelper(t)
	ctx := context.Background()

	// Save 10 pending events (same pattern as security-management's SaveInTx).
	const eventCount = 10
	eventIDs := make([]string, 0, eventCount)
	for i := 0; i < eventCount; i++ {
		event := helper.CreateTestEvent(1, "Order", "order-sync-"+string(rune('a'+i)), "OrderCreated")
		require.NoError(t, repo.Save(ctx, event))
		eventIDs = append(eventIDs, event.ID)
	}

	// Start scheduler — pollLoop picks up pending events via FindPendingEvents.
	require.NoError(t, scheduler.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = scheduler.Stop(stopCtx)
	}()

	// Wait for the poll cycle to process all events.
	require.Eventually(t, func() bool {
		published := 0
		for _, id := range eventIDs {
			ev, err := repo.FindByID(ctx, id)
			if err != nil || ev == nil {
				continue
			}
			if ev.Status == outbox.EventStatusPublished {
				published++
			}
		}
		return published == eventCount
	}, 4*time.Second, 100*time.Millisecond,
		"scheduler poll must publish all %d events", eventCount)

	// Handler must have been called for each event.
	require.Equal(t, int32(eventCount), pub.handlerCalls.Load(),
		"sync publisher handler must be called exactly %d times", eventCount)
}

// =========================================================================
// Regression guard: Scheduler with BatchSize smaller than pending count
// processes events across multiple poll cycles without dropping any.
// =========================================================================

func TestSyncScheduler_MultiPollBatchCompletes(t *testing.T) {
	repo := NewMockRepository()
	pub := newHandlerCountPublisher()
	mapper := NewMockTopicMapper()

	cfg := outbox.DefaultSchedulerConfig()
	cfg.PollInterval = 1 * time.Second // min allowed by Validate
	cfg.BatchSize = 3 // smaller than total events → multiple polls needed
	cfg.EnableRetry = false
	cfg.EnableCleanup = false
	cfg.EnableHealthCheck = false

	publisherCfg := outbox.DefaultPublisherConfig()
	publisherCfg.PublishTimeout = 5 * time.Second
	publisherCfg.MaxRetries = 1

	scheduler := outbox.NewScheduler(
		outbox.WithRepository(repo),
		outbox.WithEventPublisher(pub),
		outbox.WithTopicMapper(mapper),
		outbox.WithSchedulerConfig(cfg),
		outbox.WithPublisherConfig(publisherCfg),
	)

	helper := NewTestHelper(t)
	ctx := context.Background()

	// Save 10 pending events — BatchSize=3 means 4 poll cycles to clear.
	const eventCount = 10
	for i := 0; i < eventCount; i++ {
		event := helper.CreateTestEvent(1, "Order", "order-multi-"+string(rune('a'+i)), "OrderCreated")
		require.NoError(t, repo.Save(ctx, event))
	}

	require.NoError(t, scheduler.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = scheduler.Stop(stopCtx)
	}()

	// All events must eventually be published across multiple polls.
	require.Eventually(t, func() bool {
		count, _ := repo.Count(ctx, outbox.EventStatusPublished, 0)
		return int(count) >= eventCount
	}, 8*time.Second, 200*time.Millisecond,
		"multi-poll scheduler must publish all %d events (BatchSize=3)", eventCount)

	require.Equal(t, int32(eventCount), pub.handlerCalls.Load(),
		"handler must be called exactly %d times across polls", eventCount)
}
