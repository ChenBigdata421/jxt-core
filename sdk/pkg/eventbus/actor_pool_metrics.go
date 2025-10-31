package eventbus

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ActorPoolMetricsCollector defines the interface for collecting Actor Pool metrics
// 使用接口注入模式，与 EventBus 的 MetricsCollector 保持一致
type ActorPoolMetricsCollector interface {
	// RecordMessageSent records a message sent to an actor
	RecordMessageSent(actorID string)

	// RecordMessageProcessed records a message processed by an actor
	RecordMessageProcessed(actorID string, success bool, duration time.Duration)

	// RecordInboxDepth records the inbox depth of an actor
	// Note: depth is an approximate value based on atomic counter
	RecordInboxDepth(actorID string, depth int, capacity int)

	// RecordActorRestarted records an actor restart event
	RecordActorRestarted(actorID string)

	// RecordDeadLetter records a dead letter event
	RecordDeadLetter(actorID string)
}

// NoOpActorPoolMetricsCollector is a no-op implementation of ActorPoolMetricsCollector
type NoOpActorPoolMetricsCollector struct{}

func (n *NoOpActorPoolMetricsCollector) RecordMessageSent(actorID string) {}
func (n *NoOpActorPoolMetricsCollector) RecordMessageProcessed(actorID string, success bool, duration time.Duration) {
}
func (n *NoOpActorPoolMetricsCollector) RecordInboxDepth(actorID string, depth int, capacity int) {}
func (n *NoOpActorPoolMetricsCollector) RecordActorRestarted(actorID string)                      {}
func (n *NoOpActorPoolMetricsCollector) RecordDeadLetter(actorID string)                          {}

// PrometheusActorPoolMetricsCollector implements ActorPoolMetricsCollector using Prometheus
type PrometheusActorPoolMetricsCollector struct {
	namespace string

	// Prometheus metrics
	messagesSent      *prometheus.CounterVec
	messagesProcessed *prometheus.CounterVec
	messageLatency    *prometheus.HistogramVec
	inboxDepth        *prometheus.GaugeVec
	inboxUtilization  *prometheus.GaugeVec
	actorRestarted    *prometheus.CounterVec
	deadLetters       *prometheus.CounterVec
}

// NewPrometheusActorPoolMetricsCollector creates a new Prometheus metrics collector
func NewPrometheusActorPoolMetricsCollector(namespace string) *PrometheusActorPoolMetricsCollector {
	collector := &PrometheusActorPoolMetricsCollector{
		namespace: namespace,
	}

	// 初始化 Prometheus 指标
	collector.messagesSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "actor_pool_messages_sent_total",
			Help:      "Total messages sent to actors",
		},
		[]string{"actor_id"},
	)

	collector.messagesProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "actor_pool_messages_processed_total",
			Help:      "Total messages processed by actors",
		},
		[]string{"actor_id", "success"},
	)

	collector.messageLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "actor_pool_message_latency_seconds",
			Help:      "Message processing latency in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"actor_id"},
	)

	collector.inboxDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "actor_pool_inbox_depth",
			Help:      "Current inbox depth (approximate)",
		},
		[]string{"actor_id"},
	)

	collector.inboxUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "actor_pool_inbox_utilization",
			Help:      "Inbox utilization ratio (0-1, approximate)",
		},
		[]string{"actor_id"},
	)

	collector.actorRestarted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "actor_pool_actor_restarted_total",
			Help:      "Total actor restarts",
		},
		[]string{"actor_id"},
	)

	collector.deadLetters = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "actor_pool_dead_letters_total",
			Help:      "Total dead letters",
		},
		[]string{"actor_id"},
	)

	// 注册 Prometheus 指标
	prometheus.MustRegister(
		collector.messagesSent,
		collector.messagesProcessed,
		collector.messageLatency,
		collector.inboxDepth,
		collector.inboxUtilization,
		collector.actorRestarted,
		collector.deadLetters,
	)

	return collector
}

// RecordMessageSent records a message sent to an actor
func (p *PrometheusActorPoolMetricsCollector) RecordMessageSent(actorID string) {
	p.messagesSent.WithLabelValues(actorID).Inc()
}

// RecordMessageProcessed records a message processed by an actor
func (p *PrometheusActorPoolMetricsCollector) RecordMessageProcessed(actorID string, success bool, duration time.Duration) {
	successStr := "true"
	if !success {
		successStr = "false"
	}
	p.messagesProcessed.WithLabelValues(actorID, successStr).Inc()
	p.messageLatency.WithLabelValues(actorID).Observe(duration.Seconds())
}

// RecordInboxDepth records the inbox depth of an actor
//
// ⚠️ 注意: depth 为近似值
// - 计数器在消息发送时 +1，接收时 -1
// - 存在竞态条件，可能与实际队列深度有偏差
// - 仅用于趋势观测和容量规划，不保证精确
func (p *PrometheusActorPoolMetricsCollector) RecordInboxDepth(actorID string, depth int, capacity int) {
	p.inboxDepth.WithLabelValues(actorID).Set(float64(depth))

	// 避免除以零
	var utilization float64
	if capacity > 0 {
		utilization = float64(depth) / float64(capacity)
	} else {
		utilization = 0
	}
	p.inboxUtilization.WithLabelValues(actorID).Set(utilization)
}

// RecordActorRestarted records an actor restart event
func (p *PrometheusActorPoolMetricsCollector) RecordActorRestarted(actorID string) {
	p.actorRestarted.WithLabelValues(actorID).Inc()
}

// RecordDeadLetter records a dead letter event
func (p *PrometheusActorPoolMetricsCollector) RecordDeadLetter(actorID string) {
	p.deadLetters.WithLabelValues(actorID).Inc()
}

// Unregister unregisters all Prometheus metrics
func (p *PrometheusActorPoolMetricsCollector) Unregister() {
	prometheus.Unregister(p.messagesSent)
	prometheus.Unregister(p.messagesProcessed)
	prometheus.Unregister(p.messageLatency)
	prometheus.Unregister(p.inboxDepth)
	prometheus.Unregister(p.inboxUtilization)
	prometheus.Unregister(p.actorRestarted)
	prometheus.Unregister(p.deadLetters)
}
