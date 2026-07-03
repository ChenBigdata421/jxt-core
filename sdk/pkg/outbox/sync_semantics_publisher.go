package outbox

// SyncSemanticsPublisher is implemented by event publishers whose PublishEnvelope
// is synchronous: when PublishEnvelope returns, the publish has effectively completed
// (handlers ran, or broker accepted). The InProcessEventPublisher is synchronous;
// Kafka/NATS publishers are NOT (broker ACK arrives later).
//
// OutboxPublisher uses this marker to decide whether to mark events as Published
// synchronously after PublishBatch succeeds. Sync publishers can mark immediately
// (the publish has happened). Async publishers must wait for broker ACK via the
// ACK listener.
//
// CONSTRAINT (eng review Issue 2): InProcessEventPublisher is a HYBRID — its
// PublishEnvelope runs handlers synchronously AND also emits a PublishResult on
// GetPublishResultChannel(). When you use a sync-semantics publisher's synchronous
// marking path (this marker → MarkBatchAsPublished in PublishBatch), do NOT also
// wire StartACKListener on the same publisher: both paths would then mark the same
// event. The MarkBatchAsPublished `WHERE status='pending'` guard makes the second
// write a harmless no-op, but wiring both is redundant and confusing — pick one
// marking path per publisher.
type SyncSemanticsPublisher interface {
	EventPublisher

	// IsSyncSemantics is a marker method with no implementation. Its presence
	// on an EventPublisher type signals sync semantics to the publisher.
	IsSyncSemantics()
}

// ACK batcher (async path): when PublisherConfig.ACKBatchSize>0 (default 50), the ACK
// listener buffers up to ACKBatchSize (or ACKBatchFlushInterval) successful ACKs before
// flushing one MarkBatchAsPublished. Graceful Stop flushes the remainder; a HARD crash
// (panic/OOM/kill -9) loses the in-flight buffer (<=ACKBatchSize events) -> they stay
// Pending -> re-published next tick -> duplicate delivery. Consumer-side handlers MUST
// be idempotent to absorb this (the at-least-once guarantee's inherent duplicate risk,
// widened from 1 to <=ACKBatchSize).
//
// Transient MarkBatchAsPublished failure (brief DB hiccup): the failed batch's IDs are
// dropped from the in-memory buffer (A7). Those events stay Pending and are re-fetched by
// the OutboxScheduler poller on its next tick (seconds, not the batcher's 200ms), causing
// one spurious re-publish + duplicate delivery. This sits inside the <=ACKBatchSize
// duplicate window above; at-least-once is preserved (consumer-idempotency audit tracked
// in jxt-core/TODOS.md, F1).
//
// Steady-state poller race (F-New-1: a 3rd duplicate source, distinct from crash-loss
// above and transient-flush-failure): an ACKed-but-unmarked event sits in the buffer for
// <=ACKBatchFlushInterval with DB status still Pending. If the scheduler's
// FindPendingEvents poller (default ~10s) reaches it in that window, the event is
// re-published once -> duplicate delivery, during NORMAL operation. Mitigated:
// FindPendingEvents orders by created_at ASC (in-buffer events are recent, tail of the
// queue), and T(200ms) << PollInterval(10s) keeps the hit rate low. Falls inside the
// <=ACKBatchSize duplicate window; consumer idempotency (F1) absorbs it.
