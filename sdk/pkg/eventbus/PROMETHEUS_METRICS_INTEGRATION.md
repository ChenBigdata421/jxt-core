# EventBus Prometheus 监控集成

## 概述

为 EventBus 组件添加了 Prometheus 监控能力，参考 Outbox 组件的 MetricsCollector 设计，实现了完整的指标收集和导出功能。

## 设计原则

- ✅ **依赖倒置原则（DIP）**：EventBus 依赖 MetricsCollector 接口而非具体实现
- ✅ **零依赖**：核心代码不依赖外部监控库（Prometheus 库仅在示例代码中使用）
- ✅ **可扩展性**：支持多种监控系统实现（Prometheus、StatsD、DataDog 等）
- ✅ **向后兼容**：不设置 MetricsCollector 时使用 NoOpMetricsCollector，不影响现有功能

## 架构设计

### 1. MetricsCollector 接口

```go
type MetricsCollector interface {
    // 发布指标
    RecordPublish(topic string, success bool, duration time.Duration)
    RecordPublishBatch(topic string, count int, success bool, duration time.Duration)
    
    // 订阅指标
    RecordConsume(topic string, success bool, duration time.Duration)
    RecordConsumeBatch(topic string, count int, success bool, duration time.Duration)
    
    // 连接指标
    RecordConnection(connected bool)
    RecordReconnect(success bool, duration time.Duration)
    
    // 积压指标
    RecordBacklog(topic string, backlog int64)
    RecordBacklogState(topic string, state string)
    
    // 健康检查指标
    RecordHealthCheck(healthy bool, duration time.Duration)
    
    // 错误指标
    RecordError(errorType string, topic string)
}
```

### 2. 实现类

#### NoOpMetricsCollector（默认实现）
- 空操作实现，不收集任何指标
- 用于不需要监控的场景
- 零性能开销

#### InMemoryMetricsCollector（测试/调试实现）
- 内存中收集指标
- 用于测试和调试
- 提供 `GetMetrics()` 方法查看当前指标

#### PrometheusMetricsCollector（生产实现）
- 集成 Prometheus 客户端库
- 自动注册所有指标
- 支持自定义命名空间

## 集成方式

### 1. 在 EventBusConfig 中添加 MetricsCollector 字段

```go
type EventBusConfig struct {
    Type       string
    Kafka      KafkaConfig
    NATS       NATSConfig
    Metrics    MetricsConfig
    Tracing    TracingConfig
    Enterprise EnterpriseConfig
    
    // MetricsCollector 指标收集器（可选）
    MetricsCollector MetricsCollector `mapstructure:"-"`
}
```

### 2. 在 eventBusManager 中集成

```go
type eventBusManager struct {
    config           *EventBusConfig
    publisher        Publisher
    subscriber       Subscriber
    metrics          *Metrics
    metricsCollector MetricsCollector  // 新增字段
    // ... 其他字段
}
```

### 3. 在 Publish 和 Subscribe 方法中记录指标

```go
func (m *eventBusManager) Publish(ctx context.Context, topic string, message []byte) error {
    start := time.Now()
    err := m.publisher.Publish(ctx, topic, message)
    duration := time.Since(start)
    
    // 记录到外部指标收集器
    if m.metricsCollector != nil {
        m.metricsCollector.RecordPublish(topic, err == nil, duration)
        if err != nil {
            m.metricsCollector.RecordError("publish", topic)
        }
    }
    
    return err
}
```

## 使用示例

### 1. 使用 Prometheus 监控

```go
package main

import (
    "context"
    "net/http"
    
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // 1. 创建 Prometheus 指标收集器
    metricsCollector := eventbus.NewPrometheusMetricsCollector("my_service")
    
    // 2. 配置 EventBus
    config := eventbus.GetDefaultConfig("memory")
    config.MetricsCollector = metricsCollector
    
    // 3. 创建 EventBus
    bus, err := eventbus.NewEventBus(config)
    if err != nil {
        panic(err)
    }
    defer bus.Close()
    
    // 4. 启动 Prometheus HTTP 服务器
    go func() {
        http.Handle("/metrics", promhttp.Handler())
        http.ListenAndServe(":9090", nil)
    }()
    
    // 5. 使用 EventBus（所有指标自动记录）
    ctx := context.Background()
    bus.Publish(ctx, "user_events", []byte(`{"event": "user_created"}`))
}
```

### 2. 使用内存指标收集器（测试/调试）

```go
// 创建内存指标收集器
metricsCollector := eventbus.NewInMemoryMetricsCollector()

// 配置 EventBus
config := eventbus.GetDefaultConfig("memory")
config.MetricsCollector = metricsCollector

bus, _ := eventbus.NewEventBus(config)

// 使用 EventBus
bus.Publish(ctx, "test", []byte("message"))

// 查看指标
metrics := metricsCollector.GetMetrics()
fmt.Printf("发布总数: %v\n", metrics["publish_total"])
fmt.Printf("发布成功: %v\n", metrics["publish_success"])
```

### 3. 不使用监控（默认行为）

```go
// 不设置 MetricsCollector，自动使用 NoOpMetricsCollector
config := eventbus.GetDefaultConfig("memory")
bus, _ := eventbus.NewEventBus(config)

// 正常使用，无性能开销
bus.Publish(ctx, "test", []byte("message"))
```

## Prometheus 指标列表

### 发布指标
- `{namespace}_eventbus_publish_total{topic}` - 发布消息总数
- `{namespace}_eventbus_publish_success_total{topic}` - 发布成功总数
- `{namespace}_eventbus_publish_failed_total{topic}` - 发布失败总数
- `{namespace}_eventbus_publish_latency_seconds{topic}` - 发布延迟（直方图）

### 消费指标
- `{namespace}_eventbus_consume_total{topic}` - 消费消息总数
- `{namespace}_eventbus_consume_success_total{topic}` - 消费成功总数
- `{namespace}_eventbus_consume_failed_total{topic}` - 消费失败总数
- `{namespace}_eventbus_consume_latency_seconds{topic}` - 消费延迟（直方图）

### 连接指标
- `{namespace}_eventbus_connected` - 连接状态（1=已连接，0=未连接）
- `{namespace}_eventbus_reconnect_total` - 重连尝试总数
- `{namespace}_eventbus_reconnect_success_total` - 重连成功总数
- `{namespace}_eventbus_reconnect_failed_total` - 重连失败总数
- `{namespace}_eventbus_reconnect_latency_seconds` - 重连延迟（直方图）

### 积压指标
- `{namespace}_eventbus_backlog{topic}` - 消息积压数量
- `{namespace}_eventbus_backlog_state{topic}` - 积压状态（0=normal, 1=warning, 2=critical）

### 健康检查指标
- `{namespace}_eventbus_health_check_total` - 健康检查总数
- `{namespace}_eventbus_health_check_healthy_total` - 健康检查成功总数
- `{namespace}_eventbus_health_check_unhealthy_total` - 健康检查失败总数
- `{namespace}_eventbus_health_check_latency_seconds` - 健康检查延迟（直方图）

### 错误指标
- `{namespace}_eventbus_errors_total{error_type}` - 错误总数（按类型）
- `{namespace}_eventbus_errors_by_topic_total{topic}` - 错误总数（按主题）

## Prometheus 配置示例

```yaml
scrape_configs:
  - job_name: 'eventbus'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
    scrape_timeout: 10s
```

## Grafana 仪表板示例

### 发布性能面板
```promql
# 发布速率
rate(my_service_eventbus_publish_total[5m])

# 发布成功率
rate(my_service_eventbus_publish_success_total[5m]) / rate(my_service_eventbus_publish_total[5m])

# P95 发布延迟
histogram_quantile(0.95, rate(my_service_eventbus_publish_latency_seconds_bucket[5m]))
```

### 消费性能面板
```promql
# 消费速率
rate(my_service_eventbus_consume_total[5m])

# 消费成功率
rate(my_service_eventbus_consume_success_total[5m]) / rate(my_service_eventbus_consume_total[5m])

# P99 消费延迟
histogram_quantile(0.99, rate(my_service_eventbus_consume_latency_seconds_bucket[5m]))
```

## 测试覆盖

- ✅ `TestNoOpMetricsCollector` - 测试空操作收集器
- ✅ `TestInMemoryMetricsCollector` - 测试内存收集器
- ✅ `TestInMemoryMetricsCollector_GetMetrics` - 测试获取指标
- ✅ `TestEventBusWithMetricsCollector` - 测试 EventBus 集成
- ✅ `TestEventBusWithoutMetricsCollector` - 测试不使用收集器

所有测试通过 ✅

## 相关文件

### 核心实现
- `jxt-core/sdk/pkg/eventbus/metrics_collector.go` - MetricsCollector 接口和实现
- `jxt-core/sdk/pkg/eventbus/metrics_prometheus_example.go` - Prometheus 集成实现
- `jxt-core/sdk/pkg/eventbus/eventbus.go` - EventBus 集成
- `jxt-core/sdk/pkg/eventbus/type.go` - EventBusConfig 扩展

### 测试和示例
- `jxt-core/sdk/pkg/eventbus/metrics_collector_test.go` - 单元测试
- `jxt-core/sdk/pkg/eventbus/examples/prometheus_metrics_example.go` - 完整示例

## 优势

1. **依赖倒置**：核心代码不依赖具体监控系统
2. **零依赖**：不使用监控时无额外依赖
3. **可扩展**：易于添加新的监控系统支持
4. **向后兼容**：不影响现有代码
5. **生产就绪**：完整的指标覆盖和测试

## 与 Outbox 组件的一致性

EventBus 的 MetricsCollector 设计与 Outbox 组件保持一致：
- ✅ 相同的接口设计模式
- ✅ 相同的实现类（NoOp、InMemory、Prometheus）
- ✅ 相同的集成方式
- ✅ 相同的使用体验

这确保了整个 jxt-core SDK 的监控能力具有统一的设计和使用方式。

---

**文档版本**: v1.0  
**更新时间**: 2025-10-21  
**作者**: Augment Agent

