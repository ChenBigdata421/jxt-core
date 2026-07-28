package reliable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricNameConstantsMatchSpec(t *testing.T) {
	assert.Equal(t, "consumption_dead_letter_total", MetricDeadLetterTotal)
	assert.Equal(t, "consumption_record_failure_total", MetricRecordFailureTotal)
	assert.Equal(t, "consumption_anomaly_total", MetricAnomalyTotal)
	assert.Equal(t, "consumption_partition_stalled_seconds", MetricPartitionStalledSecs)
	assert.Equal(t, "consumption_pending_count", MetricPendingCount)
	assert.Equal(t, "consumption_replay_blocked_count", MetricReplayBlockedCount)
	assert.Equal(t, "outbox_dead_lettered_total", MetricOutboxDeadLetteredTotal)
}

func TestNoOpMetricsImplementsInterface(t *testing.T) {
	var _ ConsumptionMetrics = NoOpMetrics{}
	var _ Alerter = NoOpAlerter{}
}

func TestNoOpMetricsCallable(t *testing.T) {
	m := NoOpMetrics{}
	assert.NotPanics(t, func() {
		m.IncDeadLetter("h", ClassPoison)
		m.SetPending(StatusRetryScheduled, "h", 1, 5)
	})
}
