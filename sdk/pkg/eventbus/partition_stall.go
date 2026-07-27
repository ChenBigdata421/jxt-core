package eventbus

// MetricPartitionStalledSecs is the canonical metric NAME (§8.4③: name in core).
// The prometheus Gauge IMPLEMENTATION lives in the service
// (evidence-management/shared/common/metrics), wired into StallReporter.
const MetricPartitionStalledSecs = "consumption_partition_stalled_seconds"

// StallReporter is the injection seam the pipeline stall ticker calls. Nil
// (no-op) by default — core never imports prometheus. The service assigns a
// prometheus Gauge writer at process start. Written ONLY from the stall ticker
// (every StallWarnInterval) to keep the hot path free of per-message writes.
var StallReporter func(topic string, partition int32, seconds float64)

// StallEnterReporter fires once per stall-ENTER transition — monotonic event
// detection that survives rebalance-induced gauge deletes (review 2026-07-26:
// a tick-only Gauge + DeleteLabelValues-on-exit can blink below the 60s P1
// `for:` clause under rebalance churn; a Counter cannot).
var StallEnterReporter func(topic string, partition int32)

// stallClearReporter removes the label set; called on topic-unsubscribe (NOT
// every partition-claim end) so a brief rebalance doesn't wipe visibility.
var stallClearReporter func(topic string, partition int32)

// SetStallClearReporter installs the label-clear callback (used by the service
// to wire a prometheus Gauge DeleteLabelValues). Called on topic-unsubscribe,
// NOT per-claim, to avoid blinking the gauge below the 60s P1 `for:` clause
// under rebalance churn (review 2026-07-26).
func SetStallClearReporter(fn func(topic string, partition int32)) {
	stallClearReporter = fn
}

// ReportPartitionStall forwards (topic, partition, seconds) to the injected
// StallReporter. No-op when the reporter is unset OR when topic is empty
// (empty-topic guard, review 2026-07-26: prevents
// consumption_partition_stalled_seconds{topic="",partition="0"} if a partial
// revert ships the gauge without the adapter).
func ReportPartitionStall(topic string, partition int32, seconds float64) {
	if topic == "" || StallReporter == nil { // empty-topic guard + nil-safe
		return
	}
	StallReporter(topic, partition, seconds)
}

// ReportPartitionStallEnter fires the stall-enter rising-edge hook (monotonic
// Counter source). No-op when unset or empty-topic.
func ReportPartitionStallEnter(topic string, partition int32) {
	if topic == "" || StallEnterReporter == nil {
		return
	}
	StallEnterReporter(topic, partition)
}

// ClearPartitionStall removes the gauge label set for (topic, partition).
// Called on topic-unsubscribe only. No-op when unset or empty-topic.
func ClearPartitionStall(topic string, partition int32) {
	if topic == "" || stallClearReporter == nil {
		return
	}
	stallClearReporter(topic, partition)
}
