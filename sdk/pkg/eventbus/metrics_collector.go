package eventbus

import (
	"sync"
	"time"
)

// MetricsCollector 指标收集器接口
// 用于集成 Prometheus、StatsD 等监控系统
//
// 设计原则：
// - 依赖倒置：EventBus 依赖接口而非具体实现
// - 零依赖：核心代码不依赖外部监控库
// - 可扩展：支持多种监控系统实现
type MetricsCollector interface {
	// ========== 发布指标 ==========
	
	// RecordPublish 记录消息发布
	// topic: 主题名称
	// success: 是否成功
	// duration: 发布耗时
	RecordPublish(topic string, success bool, duration time.Duration)
	
	// RecordPublishBatch 记录批量发布
	// topic: 主题名称
	// count: 消息数量
	// success: 是否成功
	// duration: 发布耗时
	RecordPublishBatch(topic string, count int, success bool, duration time.Duration)
	
	// ========== 订阅指标 ==========
	
	// RecordConsume 记录消息消费
	// topic: 主题名称
	// success: 是否成功
	// duration: 处理耗时
	RecordConsume(topic string, success bool, duration time.Duration)
	
	// RecordConsumeBatch 记录批量消费
	// topic: 主题名称
	// count: 消息数量
	// success: 是否成功
	// duration: 处理耗时
	RecordConsumeBatch(topic string, count int, success bool, duration time.Duration)
	
	// ========== 连接指标 ==========
	
	// RecordConnection 记录连接状态变化
	// connected: 是否已连接
	RecordConnection(connected bool)
	
	// RecordReconnect 记录重连事件
	// success: 是否成功
	// duration: 重连耗时
	RecordReconnect(success bool, duration time.Duration)
	
	// ========== 积压指标 ==========
	
	// RecordBacklog 记录消息积压
	// topic: 主题名称
	// backlog: 积压数量
	RecordBacklog(topic string, backlog int64)
	
	// RecordBacklogState 记录积压状态变化
	// topic: 主题名称
	// state: 积压状态（normal, warning, critical）
	RecordBacklogState(topic string, state string)
	
	// ========== 健康检查指标 ==========
	
	// RecordHealthCheck 记录健康检查
	// healthy: 是否健康
	// duration: 检查耗时
	RecordHealthCheck(healthy bool, duration time.Duration)
	
	// ========== 错误指标 ==========
	
	// RecordError 记录错误
	// errorType: 错误类型（publish, consume, connection, etc.）
	// topic: 主题名称（可选）
	RecordError(errorType string, topic string)
}

// NoOpMetricsCollector 空操作指标收集器（默认实现）
type NoOpMetricsCollector struct{}

func (n *NoOpMetricsCollector) RecordPublish(topic string, success bool, duration time.Duration) {}
func (n *NoOpMetricsCollector) RecordPublishBatch(topic string, count int, success bool, duration time.Duration) {
}
func (n *NoOpMetricsCollector) RecordConsume(topic string, success bool, duration time.Duration) {}
func (n *NoOpMetricsCollector) RecordConsumeBatch(topic string, count int, success bool, duration time.Duration) {
}
func (n *NoOpMetricsCollector) RecordConnection(connected bool)                        {}
func (n *NoOpMetricsCollector) RecordReconnect(success bool, duration time.Duration)  {}
func (n *NoOpMetricsCollector) RecordBacklog(topic string, backlog int64)             {}
func (n *NoOpMetricsCollector) RecordBacklogState(topic string, state string)         {}
func (n *NoOpMetricsCollector) RecordHealthCheck(healthy bool, duration time.Duration) {}
func (n *NoOpMetricsCollector) RecordError(errorType string, topic string)            {}

// InMemoryMetricsCollector 内存指标收集器（用于测试和调试）
type InMemoryMetricsCollector struct {
	mu sync.RWMutex
	
	// 发布指标
	PublishTotal   int64
	PublishSuccess int64
	PublishFailed  int64
	PublishLatency time.Duration
	
	// 消费指标
	ConsumeTotal   int64
	ConsumeSuccess int64
	ConsumeFailed  int64
	ConsumeLatency time.Duration
	
	// 连接指标
	Connected      bool
	ReconnectTotal int64
	ReconnectSuccess int64
	ReconnectFailed  int64
	
	// 积压指标
	BacklogByTopic map[string]int64
	BacklogState   map[string]string
	
	// 健康检查指标
	HealthCheckTotal   int64
	HealthCheckHealthy int64
	HealthCheckUnhealthy int64
	
	// 错误指标
	ErrorsByType map[string]int64
	ErrorsByTopic map[string]int64
}

func NewInMemoryMetricsCollector() *InMemoryMetricsCollector {
	return &InMemoryMetricsCollector{
		BacklogByTopic: make(map[string]int64),
		BacklogState:   make(map[string]string),
		ErrorsByType:   make(map[string]int64),
		ErrorsByTopic:  make(map[string]int64),
	}
}

func (m *InMemoryMetricsCollector) RecordPublish(topic string, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.PublishTotal++
	if success {
		m.PublishSuccess++
	} else {
		m.PublishFailed++
	}
	m.PublishLatency = duration
}

func (m *InMemoryMetricsCollector) RecordPublishBatch(topic string, count int, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.PublishTotal += int64(count)
	if success {
		m.PublishSuccess += int64(count)
	} else {
		m.PublishFailed += int64(count)
	}
	m.PublishLatency = duration
}

func (m *InMemoryMetricsCollector) RecordConsume(topic string, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.ConsumeTotal++
	if success {
		m.ConsumeSuccess++
	} else {
		m.ConsumeFailed++
	}
	m.ConsumeLatency = duration
}

func (m *InMemoryMetricsCollector) RecordConsumeBatch(topic string, count int, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.ConsumeTotal += int64(count)
	if success {
		m.ConsumeSuccess += int64(count)
	} else {
		m.ConsumeFailed += int64(count)
	}
	m.ConsumeLatency = duration
}

func (m *InMemoryMetricsCollector) RecordConnection(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.Connected = connected
}

func (m *InMemoryMetricsCollector) RecordReconnect(success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.ReconnectTotal++
	if success {
		m.ReconnectSuccess++
	} else {
		m.ReconnectFailed++
	}
}

func (m *InMemoryMetricsCollector) RecordBacklog(topic string, backlog int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.BacklogByTopic[topic] = backlog
}

func (m *InMemoryMetricsCollector) RecordBacklogState(topic string, state string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.BacklogState[topic] = state
}

func (m *InMemoryMetricsCollector) RecordHealthCheck(healthy bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.HealthCheckTotal++
	if healthy {
		m.HealthCheckHealthy++
	} else {
		m.HealthCheckUnhealthy++
	}
}

func (m *InMemoryMetricsCollector) RecordError(errorType string, topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.ErrorsByType[errorType]++
	if topic != "" {
		m.ErrorsByTopic[topic]++
	}
}

// GetMetrics 获取当前指标（线程安全）
func (m *InMemoryMetricsCollector) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return map[string]interface{}{
		"publish_total":   m.PublishTotal,
		"publish_success": m.PublishSuccess,
		"publish_failed":  m.PublishFailed,
		"publish_latency": m.PublishLatency,
		
		"consume_total":   m.ConsumeTotal,
		"consume_success": m.ConsumeSuccess,
		"consume_failed":  m.ConsumeFailed,
		"consume_latency": m.ConsumeLatency,
		
		"connected":         m.Connected,
		"reconnect_total":   m.ReconnectTotal,
		"reconnect_success": m.ReconnectSuccess,
		"reconnect_failed":  m.ReconnectFailed,
		
		"backlog_by_topic": m.BacklogByTopic,
		"backlog_state":    m.BacklogState,
		
		"health_check_total":     m.HealthCheckTotal,
		"health_check_healthy":   m.HealthCheckHealthy,
		"health_check_unhealthy": m.HealthCheckUnhealthy,
		
		"errors_by_type":  m.ErrorsByType,
		"errors_by_topic": m.ErrorsByTopic,
	}
}

