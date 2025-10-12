package eventbus

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// NATSMetrics NATS EventBus详细指标
type NATSMetrics struct {
	// 消息指标
	MessagesPublished   *prometheus.CounterVec
	MessagesConsumed    *prometheus.CounterVec
	MessageProcessingTime *prometheus.HistogramVec
	MessageSize         *prometheus.HistogramVec
	
	// JetStream特定指标
	StreamMessages      *prometheus.GaugeVec
	StreamBytes         *prometheus.GaugeVec
	ConsumerPending     *prometheus.GaugeVec
	ConsumerDelivered   *prometheus.CounterVec
	ConsumerAckPending  *prometheus.GaugeVec
	
	// 连接指标
	ConnectionStatus    prometheus.Gauge
	ReconnectCount      prometheus.Counter
	HealthCheckDuration *prometheus.HistogramVec
	
	// 错误指标
	PublishErrors       *prometheus.CounterVec
	ConsumeErrors       *prometheus.CounterVec
	ConnectionErrors    prometheus.Counter
	JetStreamErrors     *prometheus.CounterVec
	
	// 性能指标
	PublishDuration     *prometheus.HistogramVec
	SubscribeDuration   *prometheus.HistogramVec
	
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewNATSMetrics 创建NATS指标收集器
func NewNATSMetrics(namespace, subsystem string) *NATSMetrics {
	return &NATSMetrics{
		// 消息指标
		MessagesPublished: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "messages_published_total",
				Help:      "Total number of messages published to NATS",
			},
			[]string{"topic", "jetstream"},
		),
		
		MessagesConsumed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "messages_consumed_total",
				Help:      "Total number of messages consumed from NATS",
			},
			[]string{"topic", "jetstream", "consumer"},
		),
		
		MessageProcessingTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "message_processing_duration_seconds",
				Help:      "Time spent processing messages",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"topic", "status"},
		),
		
		MessageSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "message_size_bytes",
				Help:      "Size of messages in bytes",
				Buckets:   prometheus.ExponentialBuckets(64, 2, 10),
			},
			[]string{"topic", "direction"},
		),
		
		// JetStream特定指标
		StreamMessages: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "jetstream_stream_messages",
				Help:      "Number of messages in JetStream stream",
			},
			[]string{"stream"},
		),
		
		StreamBytes: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "jetstream_stream_bytes",
				Help:      "Number of bytes in JetStream stream",
			},
			[]string{"stream"},
		),
		
		ConsumerPending: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "jetstream_consumer_pending",
				Help:      "Number of pending messages for JetStream consumer",
			},
			[]string{"stream", "consumer"},
		),
		
		ConsumerDelivered: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "jetstream_consumer_delivered_total",
				Help:      "Total number of messages delivered to JetStream consumer",
			},
			[]string{"stream", "consumer"},
		),
		
		ConsumerAckPending: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "jetstream_consumer_ack_pending",
				Help:      "Number of messages pending acknowledgment for JetStream consumer",
			},
			[]string{"stream", "consumer"},
		),
		
		// 连接指标
		ConnectionStatus: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "connection_status",
				Help:      "NATS connection status (1=connected, 0=disconnected)",
			},
		),
		
		ReconnectCount: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconnect_total",
				Help:      "Total number of NATS reconnections",
			},
		),
		
		HealthCheckDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "health_check_duration_seconds",
				Help:      "Time spent on health checks",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"status"},
		),
		
		// 错误指标
		PublishErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "publish_errors_total",
				Help:      "Total number of publish errors",
			},
			[]string{"topic", "error_type"},
		),
		
		ConsumeErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "consume_errors_total",
				Help:      "Total number of consume errors",
			},
			[]string{"topic", "error_type"},
		),
		
		ConnectionErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "connection_errors_total",
				Help:      "Total number of connection errors",
			},
		),
		
		JetStreamErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "jetstream_errors_total",
				Help:      "Total number of JetStream errors",
			},
			[]string{"operation", "error_type"},
		),
		
		// 性能指标
		PublishDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "publish_duration_seconds",
				Help:      "Time spent publishing messages",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"topic", "jetstream"},
		),
		
		SubscribeDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "subscribe_duration_seconds",
				Help:      "Time spent on subscription operations",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"topic", "operation"},
		),
		
		logger: nil, // 将在初始化时设置
	}
}

// RecordPublish 记录消息发布指标
func (m *NATSMetrics) RecordPublish(topic string, jetstream bool, size int, duration time.Duration, err error) {
	jetstreamStr := "false"
	if jetstream {
		jetstreamStr = "true"
	}
	
	if err == nil {
		m.MessagesPublished.WithLabelValues(topic, jetstreamStr).Inc()
		m.MessageSize.WithLabelValues(topic, "publish").Observe(float64(size))
		m.PublishDuration.WithLabelValues(topic, jetstreamStr).Observe(duration.Seconds())
	} else {
		errorType := classifyError(err)
		m.PublishErrors.WithLabelValues(topic, errorType).Inc()
		if jetstream {
			m.JetStreamErrors.WithLabelValues("publish", errorType).Inc()
		}
	}
}

// RecordConsume 记录消息消费指标
func (m *NATSMetrics) RecordConsume(topic, consumer string, jetstream bool, size int, processingTime time.Duration, err error) {
	jetstreamStr := "false"
	if jetstream {
		jetstreamStr = "true"
	}
	
	if err == nil {
		m.MessagesConsumed.WithLabelValues(topic, jetstreamStr, consumer).Inc()
		m.MessageSize.WithLabelValues(topic, "consume").Observe(float64(size))
		m.MessageProcessingTime.WithLabelValues(topic, "success").Observe(processingTime.Seconds())
	} else {
		errorType := classifyError(err)
		m.ConsumeErrors.WithLabelValues(topic, errorType).Inc()
		m.MessageProcessingTime.WithLabelValues(topic, "error").Observe(processingTime.Seconds())
		if jetstream {
			m.JetStreamErrors.WithLabelValues("consume", errorType).Inc()
		}
	}
}

// RecordHealthCheck 记录健康检查指标
func (m *NATSMetrics) RecordHealthCheck(duration time.Duration, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	m.HealthCheckDuration.WithLabelValues(status).Observe(duration.Seconds())
}

// UpdateConnectionStatus 更新连接状态
func (m *NATSMetrics) UpdateConnectionStatus(connected bool) {
	if connected {
		m.ConnectionStatus.Set(1)
	} else {
		m.ConnectionStatus.Set(0)
	}
}

// RecordReconnect 记录重连事件
func (m *NATSMetrics) RecordReconnect() {
	m.ReconnectCount.Inc()
}

// UpdateJetStreamMetrics 更新JetStream指标
func (m *NATSMetrics) UpdateJetStreamMetrics(streamName string, messages, bytes uint64) {
	m.StreamMessages.WithLabelValues(streamName).Set(float64(messages))
	m.StreamBytes.WithLabelValues(streamName).Set(float64(bytes))
}

// UpdateConsumerMetrics 更新消费者指标
func (m *NATSMetrics) UpdateConsumerMetrics(streamName, consumerName string, pending, delivered, ackPending uint64) {
	m.ConsumerPending.WithLabelValues(streamName, consumerName).Set(float64(pending))
	m.ConsumerDelivered.WithLabelValues(streamName, consumerName).Add(float64(delivered))
	m.ConsumerAckPending.WithLabelValues(streamName, consumerName).Set(float64(ackPending))
}

// classifyError 分类错误类型
func classifyError(err error) string {
	if err == nil {
		return "none"
	}
	
	errStr := err.Error()
	switch {
	case containsSubstring(errStr, "timeout"):
		return "timeout"
	case containsSubstring(errStr, "connection"):
		return "connection"
	case containsSubstring(errStr, "permission"):
		return "permission"
	case containsSubstring(errStr, "not found"):
		return "not_found"
	case containsSubstring(errStr, "invalid"):
		return "invalid"
	default:
		return "unknown"
	}
}

// containsSubstring 检查字符串是否包含子字符串
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
		 indexOf(s, substr) >= 0)))
}

// indexOf 查找子字符串位置
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
