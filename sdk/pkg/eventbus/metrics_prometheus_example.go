package eventbus

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusMetricsCollector Prometheus 指标收集器
//
// 使用示例：
//
//	// 1. 创建 Prometheus 收集器
//	collector := eventbus.NewPrometheusMetricsCollector("my_service")
//
//	// 2. 注册到 Prometheus（可选，如果使用 promauto 则自动注册）
//	// prometheus.MustRegister(collector.publishTotal, collector.consumeTotal, ...)
//
//	// 3. 配置 EventBus
//	config := &eventbus.EventBusConfig{
//	    Type: "kafka",
//	    Kafka: eventbus.KafkaConfig{
//	        Brokers: []string{"localhost:9092"},
//	    },
//	    MetricsCollector: collector, // 注入 Prometheus 收集器
//	}
//
//	bus, err := eventbus.NewEventBus(config)
//
//	// 4. 启动 Prometheus HTTP 服务器
//	http.Handle("/metrics", promhttp.Handler())
//	go http.ListenAndServe(":9090", nil)
type PrometheusMetricsCollector struct {
	namespace string
	
	// 发布指标
	publishTotal   *prometheus.CounterVec
	publishSuccess *prometheus.CounterVec
	publishFailed  *prometheus.CounterVec
	publishLatency *prometheus.HistogramVec
	
	// 消费指标
	consumeTotal   *prometheus.CounterVec
	consumeSuccess *prometheus.CounterVec
	consumeFailed  *prometheus.CounterVec
	consumeLatency *prometheus.HistogramVec
	
	// 连接指标
	connected        prometheus.Gauge
	reconnectTotal   prometheus.Counter
	reconnectSuccess prometheus.Counter
	reconnectFailed  prometheus.Counter
	reconnectLatency prometheus.Histogram
	
	// 积压指标
	backlog      *prometheus.GaugeVec
	backlogState *prometheus.GaugeVec
	
	// 健康检查指标
	healthCheckTotal     prometheus.Counter
	healthCheckHealthy   prometheus.Counter
	healthCheckUnhealthy prometheus.Counter
	healthCheckLatency   prometheus.Histogram
	
	// 错误指标
	errorsByType  *prometheus.CounterVec
	errorsByTopic *prometheus.CounterVec
}

// NewPrometheusMetricsCollector 创建 Prometheus 指标收集器
//
// 参数：
//   - namespace: 指标命名空间（例如："my_service"）
//
// 返回：
//   - *PrometheusMetricsCollector: Prometheus 收集器实例
func NewPrometheusMetricsCollector(namespace string) *PrometheusMetricsCollector {
	if namespace == "" {
		namespace = "eventbus"
	}
	
	return &PrometheusMetricsCollector{
		namespace: namespace,
		
		// 发布指标
		publishTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_publish_total",
				Help:      "Total number of published messages",
			},
			[]string{"topic"},
		),
		publishSuccess: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_publish_success_total",
				Help:      "Total number of successfully published messages",
			},
			[]string{"topic"},
		),
		publishFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_publish_failed_total",
				Help:      "Total number of failed published messages",
			},
			[]string{"topic"},
		),
		publishLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "eventbus_publish_latency_seconds",
				Help:      "Publish latency in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"topic"},
		),
		
		// 消费指标
		consumeTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_consume_total",
				Help:      "Total number of consumed messages",
			},
			[]string{"topic"},
		),
		consumeSuccess: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_consume_success_total",
				Help:      "Total number of successfully consumed messages",
			},
			[]string{"topic"},
		),
		consumeFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_consume_failed_total",
				Help:      "Total number of failed consumed messages",
			},
			[]string{"topic"},
		),
		consumeLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "eventbus_consume_latency_seconds",
				Help:      "Consume latency in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"topic"},
		),
		
		// 连接指标
		connected: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "eventbus_connected",
				Help:      "Connection status (1=connected, 0=disconnected)",
			},
		),
		reconnectTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_reconnect_total",
				Help:      "Total number of reconnect attempts",
			},
		),
		reconnectSuccess: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_reconnect_success_total",
				Help:      "Total number of successful reconnects",
			},
		),
		reconnectFailed: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_reconnect_failed_total",
				Help:      "Total number of failed reconnects",
			},
		),
		reconnectLatency: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "eventbus_reconnect_latency_seconds",
				Help:      "Reconnect latency in seconds",
				Buckets:   prometheus.DefBuckets,
			},
		),
		
		// 积压指标
		backlog: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "eventbus_backlog",
				Help:      "Message backlog by topic",
			},
			[]string{"topic"},
		),
		backlogState: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "eventbus_backlog_state",
				Help:      "Backlog state by topic (0=normal, 1=warning, 2=critical)",
			},
			[]string{"topic"},
		),
		
		// 健康检查指标
		healthCheckTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_health_check_total",
				Help:      "Total number of health checks",
			},
		),
		healthCheckHealthy: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_health_check_healthy_total",
				Help:      "Total number of healthy health checks",
			},
		),
		healthCheckUnhealthy: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_health_check_unhealthy_total",
				Help:      "Total number of unhealthy health checks",
			},
		),
		healthCheckLatency: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "eventbus_health_check_latency_seconds",
				Help:      "Health check latency in seconds",
				Buckets:   prometheus.DefBuckets,
			},
		),
		
		// 错误指标
		errorsByType: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_errors_total",
				Help:      "Total number of errors by type",
			},
			[]string{"error_type"},
		),
		errorsByTopic: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "eventbus_errors_by_topic_total",
				Help:      "Total number of errors by topic",
			},
			[]string{"topic"},
		),
	}
}

// RecordPublish 记录消息发布
func (p *PrometheusMetricsCollector) RecordPublish(topic string, success bool, duration time.Duration) {
	p.publishTotal.WithLabelValues(topic).Inc()
	if success {
		p.publishSuccess.WithLabelValues(topic).Inc()
	} else {
		p.publishFailed.WithLabelValues(topic).Inc()
	}
	p.publishLatency.WithLabelValues(topic).Observe(duration.Seconds())
}

// RecordPublishBatch 记录批量发布
func (p *PrometheusMetricsCollector) RecordPublishBatch(topic string, count int, success bool, duration time.Duration) {
	p.publishTotal.WithLabelValues(topic).Add(float64(count))
	if success {
		p.publishSuccess.WithLabelValues(topic).Add(float64(count))
	} else {
		p.publishFailed.WithLabelValues(topic).Add(float64(count))
	}
	p.publishLatency.WithLabelValues(topic).Observe(duration.Seconds())
}

// RecordConsume 记录消息消费
func (p *PrometheusMetricsCollector) RecordConsume(topic string, success bool, duration time.Duration) {
	p.consumeTotal.WithLabelValues(topic).Inc()
	if success {
		p.consumeSuccess.WithLabelValues(topic).Inc()
	} else {
		p.consumeFailed.WithLabelValues(topic).Inc()
	}
	p.consumeLatency.WithLabelValues(topic).Observe(duration.Seconds())
}

// RecordConsumeBatch 记录批量消费
func (p *PrometheusMetricsCollector) RecordConsumeBatch(topic string, count int, success bool, duration time.Duration) {
	p.consumeTotal.WithLabelValues(topic).Add(float64(count))
	if success {
		p.consumeSuccess.WithLabelValues(topic).Add(float64(count))
	} else {
		p.consumeFailed.WithLabelValues(topic).Add(float64(count))
	}
	p.consumeLatency.WithLabelValues(topic).Observe(duration.Seconds())
}

// RecordConnection 记录连接状态变化
func (p *PrometheusMetricsCollector) RecordConnection(connected bool) {
	if connected {
		p.connected.Set(1)
	} else {
		p.connected.Set(0)
	}
}

// RecordReconnect 记录重连事件
func (p *PrometheusMetricsCollector) RecordReconnect(success bool, duration time.Duration) {
	p.reconnectTotal.Inc()
	if success {
		p.reconnectSuccess.Inc()
	} else {
		p.reconnectFailed.Inc()
	}
	p.reconnectLatency.Observe(duration.Seconds())
}

// RecordBacklog 记录消息积压
func (p *PrometheusMetricsCollector) RecordBacklog(topic string, backlog int64) {
	p.backlog.WithLabelValues(topic).Set(float64(backlog))
}

// RecordBacklogState 记录积压状态变化
func (p *PrometheusMetricsCollector) RecordBacklogState(topic string, state string) {
	var stateValue float64
	switch state {
	case "normal":
		stateValue = 0
	case "warning":
		stateValue = 1
	case "critical":
		stateValue = 2
	default:
		stateValue = -1
	}
	p.backlogState.WithLabelValues(topic).Set(stateValue)
}

// RecordHealthCheck 记录健康检查
func (p *PrometheusMetricsCollector) RecordHealthCheck(healthy bool, duration time.Duration) {
	p.healthCheckTotal.Inc()
	if healthy {
		p.healthCheckHealthy.Inc()
	} else {
		p.healthCheckUnhealthy.Inc()
	}
	p.healthCheckLatency.Observe(duration.Seconds())
}

// RecordError 记录错误
func (p *PrometheusMetricsCollector) RecordError(errorType string, topic string) {
	p.errorsByType.WithLabelValues(errorType).Inc()
	if topic != "" {
		p.errorsByTopic.WithLabelValues(topic).Inc()
	}
}

