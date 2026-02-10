package outbox

import (
	"sync"
	"time"
)

// MetricsCollector Outbox 指标收集器接口
// 用于集成 Prometheus、StatsD 等监控系统
//
// 实现示例（Prometheus）：
//
//	type PrometheusMetricsCollector struct {
//	    publishedTotal   prometheus.Counter
//	    failedTotal      prometheus.Counter
//	    retryTotal       prometheus.Counter
//	    publishDuration  prometheus.Histogram
//	    pendingGauge     prometheus.Gauge
//	}
//
//	func (c *PrometheusMetricsCollector) RecordPublished(tenantID int, aggregateType, eventType string) {
//	    c.publishedTotal.With(prometheus.Labels{
//	        "tenant_id":      fmt.Sprint(tenantID),
//	        "aggregate_type": aggregateType,
//	        "event_type":     eventType,
//	    }).Inc()
//	}
type MetricsCollector interface {
	// RecordPublished 记录事件发布成功
	RecordPublished(tenantID int, aggregateType, eventType string)

	// RecordFailed 记录事件发布失败
	RecordFailed(tenantID int, aggregateType, eventType string, err error)

	// RecordRetry 记录事件重试
	RecordRetry(tenantID int, aggregateType, eventType string)

	// RecordDLQ 记录事件进入死信队列
	RecordDLQ(tenantID int, aggregateType, eventType string)

	// RecordPublishDuration 记录发布耗时
	RecordPublishDuration(tenantID int, aggregateType, eventType string, duration time.Duration)

	// SetPendingCount 设置待发布事件数量
	SetPendingCount(tenantID int, count int64)

	// SetFailedCount 设置失败事件数量
	SetFailedCount(tenantID int, count int64)

	// SetDLQCount 设置死信队列事件数量
	SetDLQCount(tenantID int, count int64)
}

// NoOpMetricsCollector 空操作指标收集器（默认实现）
type NoOpMetricsCollector struct{}

// RecordPublished 实现 MetricsCollector 接口
func (n *NoOpMetricsCollector) RecordPublished(tenantID int, aggregateType, eventType string) {}

// RecordFailed 实现 MetricsCollector 接口
func (n *NoOpMetricsCollector) RecordFailed(tenantID int, aggregateType, eventType string, err error) {}

// RecordRetry 实现 MetricsCollector 接口
func (n *NoOpMetricsCollector) RecordRetry(tenantID int, aggregateType, eventType string) {}

// RecordDLQ 实现 MetricsCollector 接口
func (n *NoOpMetricsCollector) RecordDLQ(tenantID int, aggregateType, eventType string) {}

// RecordPublishDuration 实现 MetricsCollector 接口
func (n *NoOpMetricsCollector) RecordPublishDuration(tenantID int, aggregateType, eventType string, duration time.Duration) {
}

// SetPendingCount 实现 MetricsCollector 接口
func (n *NoOpMetricsCollector) SetPendingCount(tenantID int, count int64) {}

// SetFailedCount 实现 MetricsCollector 接口
func (n *NoOpMetricsCollector) SetFailedCount(tenantID int, count int64) {}

// SetDLQCount 实现 MetricsCollector 接口
func (n *NoOpMetricsCollector) SetDLQCount(tenantID int, count int64) {}

// InMemoryMetricsCollector 内存指标收集器（用于测试和简单场景）
type InMemoryMetricsCollector struct {
	mu sync.RWMutex

	// 计数器
	PublishedCount int64
	FailedCount    int64
	RetryCount     int64
	DLQCount       int64

	// 按租户统计
	PendingByTenant map[int]int64
	FailedByTenant  map[int]int64
	DLQByTenant     map[int]int64

	// 按事件类型统计
	PublishedByType map[string]int64
	FailedByType    map[string]int64

	// 耗时统计
	TotalDuration time.Duration
	AvgDuration   time.Duration
	MaxDuration   time.Duration
	MinDuration   time.Duration
	DurationCount int64
}

// NewInMemoryMetricsCollector 创建内存指标收集器
func NewInMemoryMetricsCollector() *InMemoryMetricsCollector {
	return &InMemoryMetricsCollector{
		PendingByTenant: make(map[int]int64),
		FailedByTenant:  make(map[int]int64),
		DLQByTenant:     make(map[int]int64),
		PublishedByType: make(map[string]int64),
		FailedByType:    make(map[string]int64),
		MinDuration:     time.Hour, // 初始化为一个大值
	}
}

// RecordPublished 记录事件发布成功
func (c *InMemoryMetricsCollector) RecordPublished(tenantID int, aggregateType, eventType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.PublishedCount++
	c.PublishedByType[eventType]++
}

// RecordFailed 记录事件发布失败
func (c *InMemoryMetricsCollector) RecordFailed(tenantID int, aggregateType, eventType string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.FailedCount++
	c.FailedByType[eventType]++
}

// RecordRetry 记录事件重试
func (c *InMemoryMetricsCollector) RecordRetry(tenantID int, aggregateType, eventType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.RetryCount++
}

// RecordDLQ 记录事件进入死信队列
func (c *InMemoryMetricsCollector) RecordDLQ(tenantID int, aggregateType, eventType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DLQCount++
}

// RecordPublishDuration 记录发布耗时
func (c *InMemoryMetricsCollector) RecordPublishDuration(tenantID int, aggregateType, eventType string, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.TotalDuration += duration
	c.DurationCount++
	c.AvgDuration = c.TotalDuration / time.Duration(c.DurationCount)

	if duration > c.MaxDuration {
		c.MaxDuration = duration
	}
	if duration < c.MinDuration {
		c.MinDuration = duration
	}
}

// SetPendingCount 设置待发布事件数量
func (c *InMemoryMetricsCollector) SetPendingCount(tenantID int, count int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.PendingByTenant[tenantID] = count
}

// SetFailedCount 设置失败事件数量
func (c *InMemoryMetricsCollector) SetFailedCount(tenantID int, count int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.FailedByTenant[tenantID] = count
}

// SetDLQCount 设置死信队列事件数量
func (c *InMemoryMetricsCollector) SetDLQCount(tenantID int, count int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DLQByTenant[tenantID] = count
}

// GetSnapshot 获取指标快照（用于测试和调试）
func (c *InMemoryMetricsCollector) GetSnapshot() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"published_count": c.PublishedCount,
		"failed_count":    c.FailedCount,
		"retry_count":     c.RetryCount,
		"dlq_count":       c.DLQCount,
		"avg_duration":    c.AvgDuration,
		"max_duration":    c.MaxDuration,
		"min_duration":    c.MinDuration,
	}
}

// Reset 重置所有指标（用于测试）
func (c *InMemoryMetricsCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.PublishedCount = 0
	c.FailedCount = 0
	c.RetryCount = 0
	c.DLQCount = 0
	c.TotalDuration = 0
	c.AvgDuration = 0
	c.MaxDuration = 0
	c.MinDuration = time.Hour
	c.DurationCount = 0

	c.PendingByTenant = make(map[int]int64)
	c.FailedByTenant = make(map[int]int64)
	c.DLQByTenant = make(map[int]int64)
	c.PublishedByType = make(map[string]int64)
	c.FailedByType = make(map[string]int64)
}

