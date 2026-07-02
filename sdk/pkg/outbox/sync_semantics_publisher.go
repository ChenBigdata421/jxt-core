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
