package outbox

import (
	"time"
)

// PrometheusMetricsCollector Prometheus 指标收集器（示例实现）
//
// 使用方法：
//
//  1. 添加依赖：
//     go get github.com/prometheus/client_golang/prometheus
//     go get github.com/prometheus/client_golang/prometheus/promauto
//
//  2. 创建收集器：
//     collector := outbox.NewPrometheusMetricsCollector("myapp")
//
//  3. 注册到 Prometheus：
//     prometheus.MustRegister(collector.publishedTotal)
//     prometheus.MustRegister(collector.failedTotal)
//     // ... 注册其他指标
//
//  4. 配置发布器和调度器：
//     publisher := outbox.NewOutboxPublisher(repo, eventPublisher, topicMapper, &outbox.PublisherConfig{
//         MetricsCollector: collector,
//     })
//
//  5. 暴露 metrics 端点：
//     http.Handle("/metrics", promhttp.Handler())
//     http.ListenAndServe(":9090", nil)
//
// 注意：这是一个示例实现，实际使用时需要取消注释并添加 prometheus 依赖
type PrometheusMetricsCollector struct {
	namespace string

	// 以下字段需要使用 prometheus.Counter、prometheus.Histogram 等类型
	// 这里使用 interface{} 是为了避免引入 prometheus 依赖
	// 实际使用时应该替换为具体类型

	// publishedTotal prometheus.Counter
	publishedTotal interface{}

	// failedTotal prometheus.Counter
	failedTotal interface{}

	// retryTotal prometheus.Counter
	retryTotal interface{}

	// dlqTotal prometheus.Counter
	dlqTotal interface{}

	// publishDuration prometheus.Histogram
	publishDuration interface{}

	// pendingGauge prometheus.Gauge
	pendingGauge interface{}

	// failedGauge prometheus.Gauge
	failedGauge interface{}

	// dlqGauge prometheus.Gauge
	dlqGauge interface{}
}

// NewPrometheusMetricsCollector 创建 Prometheus 指标收集器
//
// 实际实现示例（需要取消注释）：
//
// func NewPrometheusMetricsCollector(namespace string) *PrometheusMetricsCollector {
//     return &PrometheusMetricsCollector{
//         namespace: namespace,
//         publishedTotal: promauto.NewCounterVec(
//             prometheus.CounterOpts{
//                 Namespace: namespace,
//                 Subsystem: "outbox",
//                 Name:      "published_total",
//                 Help:      "Total number of published events",
//             },
//             []string{"tenant_id", "aggregate_type", "event_type"},
//         ),
//         failedTotal: promauto.NewCounterVec(
//             prometheus.CounterOpts{
//                 Namespace: namespace,
//                 Subsystem: "outbox",
//                 Name:      "failed_total",
//                 Help:      "Total number of failed events",
//             },
//             []string{"tenant_id", "aggregate_type", "event_type"},
//         ),
//         retryTotal: promauto.NewCounterVec(
//             prometheus.CounterOpts{
//                 Namespace: namespace,
//                 Subsystem: "outbox",
//                 Name:      "retry_total",
//                 Help:      "Total number of retried events",
//             },
//             []string{"tenant_id", "aggregate_type", "event_type"},
//         ),
//         dlqTotal: promauto.NewCounterVec(
//             prometheus.CounterOpts{
//                 Namespace: namespace,
//                 Subsystem: "outbox",
//                 Name:      "dlq_total",
//                 Help:      "Total number of events moved to DLQ",
//             },
//             []string{"tenant_id", "aggregate_type", "event_type"},
//         ),
//         publishDuration: promauto.NewHistogramVec(
//             prometheus.HistogramOpts{
//                 Namespace: namespace,
//                 Subsystem: "outbox",
//                 Name:      "publish_duration_seconds",
//                 Help:      "Event publish duration in seconds",
//                 Buckets:   prometheus.DefBuckets,
//             },
//             []string{"tenant_id", "aggregate_type", "event_type"},
//         ),
//         pendingGauge: promauto.NewGaugeVec(
//             prometheus.GaugeOpts{
//                 Namespace: namespace,
//                 Subsystem: "outbox",
//                 Name:      "pending_events",
//                 Help:      "Number of pending events",
//             },
//             []string{"tenant_id"},
//         ),
//         failedGauge: promauto.NewGaugeVec(
//             prometheus.GaugeOpts{
//                 Namespace: namespace,
//                 Subsystem: "outbox",
//                 Name:      "failed_events",
//                 Help:      "Number of failed events",
//             },
//             []string{"tenant_id"},
//         ),
//         dlqGauge: promauto.NewGaugeVec(
//             prometheus.GaugeOpts{
//                 Namespace: namespace,
//                 Subsystem: "outbox",
//                 Name:      "dlq_events",
//                 Help:      "Number of events in DLQ",
//             },
//             []string{"tenant_id"},
//         ),
//     }
// }
func NewPrometheusMetricsCollector(namespace string) *PrometheusMetricsCollector {
	return &PrometheusMetricsCollector{
		namespace: namespace,
	}
}

// RecordPublished 记录事件发布成功
//
// 实际实现示例（需要取消注释）：
//
// func (c *PrometheusMetricsCollector) RecordPublished(tenantID, aggregateType, eventType string) {
//     c.publishedTotal.(*prometheus.CounterVec).With(prometheus.Labels{
//         "tenant_id":      tenantID,
//         "aggregate_type": aggregateType,
//         "event_type":     eventType,
//     }).Inc()
// }
func (c *PrometheusMetricsCollector) RecordPublished(tenantID, aggregateType, eventType string) {
	// 实际实现需要调用 prometheus counter
}

// RecordFailed 记录事件发布失败
func (c *PrometheusMetricsCollector) RecordFailed(tenantID, aggregateType, eventType string, err error) {
	// 实际实现需要调用 prometheus counter
}

// RecordRetry 记录事件重试
func (c *PrometheusMetricsCollector) RecordRetry(tenantID, aggregateType, eventType string) {
	// 实际实现需要调用 prometheus counter
}

// RecordDLQ 记录事件进入死信队列
func (c *PrometheusMetricsCollector) RecordDLQ(tenantID, aggregateType, eventType string) {
	// 实际实现需要调用 prometheus counter
}

// RecordPublishDuration 记录发布耗时
//
// 实际实现示例（需要取消注释）：
//
// func (c *PrometheusMetricsCollector) RecordPublishDuration(tenantID, aggregateType, eventType string, duration time.Duration) {
//     c.publishDuration.(*prometheus.HistogramVec).With(prometheus.Labels{
//         "tenant_id":      tenantID,
//         "aggregate_type": aggregateType,
//         "event_type":     eventType,
//     }).Observe(duration.Seconds())
// }
func (c *PrometheusMetricsCollector) RecordPublishDuration(tenantID, aggregateType, eventType string, duration time.Duration) {
	// 实际实现需要调用 prometheus histogram
}

// SetPendingCount 设置待发布事件数量
func (c *PrometheusMetricsCollector) SetPendingCount(tenantID string, count int64) {
	// 实际实现需要调用 prometheus gauge
}

// SetFailedCount 设置失败事件数量
func (c *PrometheusMetricsCollector) SetFailedCount(tenantID string, count int64) {
	// 实际实现需要调用 prometheus gauge
}

// SetDLQCount 设置死信队列事件数量
func (c *PrometheusMetricsCollector) SetDLQCount(tenantID string, count int64) {
	// 实际实现需要调用 prometheus gauge
}

// 完整使用示例（需要取消注释并添加 prometheus 依赖）：
//
// package main
//
// import (
//     "net/http"
//     "time"
//
//     "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
//     "github.com/prometheus/client_golang/prometheus/promhttp"
// )
//
// func main() {
//     // 1. 创建 Prometheus 指标收集器
//     metricsCollector := outbox.NewPrometheusMetricsCollector("myapp")
//
//     // 2. 创建发布器配置
//     publisherConfig := &outbox.PublisherConfig{
//         EnableMetrics:    true,
//         MetricsCollector: metricsCollector,
//     }
//
//     // 3. 创建发布器
//     publisher := outbox.NewOutboxPublisher(
//         repo,
//         eventPublisher,
//         topicMapper,
//         publisherConfig,
//     )
//
//     // 4. 创建调度器配置
//     schedulerConfig := &outbox.SchedulerConfig{
//         EnableMetrics:    true,
//         MetricsCollector: metricsCollector,
//     }
//
//     // 5. 创建调度器
//     scheduler := outbox.NewScheduler(
//         outbox.WithRepository(repo),
//         outbox.WithEventPublisher(eventPublisher),
//         outbox.WithTopicMapper(topicMapper),
//         outbox.WithSchedulerConfig(schedulerConfig),
//     )
//
//     // 6. 启动调度器
//     ctx := context.Background()
//     if err := scheduler.Start(ctx); err != nil {
//         log.Fatal(err)
//     }
//
//     // 7. 暴露 Prometheus metrics 端点
//     http.Handle("/metrics", promhttp.Handler())
//     log.Println("Prometheus metrics available at :9090/metrics")
//     http.ListenAndServe(":9090", nil)
// }
//
// Prometheus 查询示例：
//
// # 每秒发布事件数
// rate(myapp_outbox_published_total[5m])
//
// # 按事件类型分组的发布数
// sum by (event_type) (myapp_outbox_published_total)
//
// # 发布耗时 P99
// histogram_quantile(0.99, rate(myapp_outbox_publish_duration_seconds_bucket[5m]))
//
// # 待发布事件数
// myapp_outbox_pending_events
//
// # 失败率
// rate(myapp_outbox_failed_total[5m]) / rate(myapp_outbox_published_total[5m])

