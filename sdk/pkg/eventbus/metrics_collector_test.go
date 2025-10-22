package eventbus

import (
	"context"
	"testing"
	"time"
)

// TestNoOpMetricsCollector 测试空操作指标收集器
func TestNoOpMetricsCollector(t *testing.T) {
	collector := &NoOpMetricsCollector{}

	// 所有方法都应该正常执行，不会 panic
	collector.RecordPublish("test", true, 10*time.Millisecond)
	collector.RecordPublishBatch("test", 10, true, 100*time.Millisecond)
	collector.RecordConsume("test", true, 5*time.Millisecond)
	collector.RecordConsumeBatch("test", 10, true, 50*time.Millisecond)
	collector.RecordConnection(true)
	collector.RecordReconnect(true, 1*time.Second)
	collector.RecordBacklog("test", 100)
	collector.RecordBacklogState("test", "normal")
	collector.RecordHealthCheck(true, 10*time.Millisecond)
	collector.RecordError("publish", "test")
}

// TestInMemoryMetricsCollector 测试内存指标收集器
func TestInMemoryMetricsCollector(t *testing.T) {
	collector := NewInMemoryMetricsCollector()

	// 测试发布指标
	collector.RecordPublish("topic1", true, 10*time.Millisecond)
	collector.RecordPublish("topic1", true, 15*time.Millisecond)
	collector.RecordPublish("topic1", false, 20*time.Millisecond)

	if collector.PublishTotal != 3 {
		t.Errorf("Expected PublishTotal=3, got %d", collector.PublishTotal)
	}
	if collector.PublishSuccess != 2 {
		t.Errorf("Expected PublishSuccess=2, got %d", collector.PublishSuccess)
	}
	if collector.PublishFailed != 1 {
		t.Errorf("Expected PublishFailed=1, got %d", collector.PublishFailed)
	}

	// 测试批量发布指标
	collector.RecordPublishBatch("topic1", 10, true, 100*time.Millisecond)

	if collector.PublishTotal != 13 {
		t.Errorf("Expected PublishTotal=13, got %d", collector.PublishTotal)
	}
	if collector.PublishSuccess != 12 {
		t.Errorf("Expected PublishSuccess=12, got %d", collector.PublishSuccess)
	}

	// 测试消费指标
	collector.RecordConsume("topic1", true, 5*time.Millisecond)
	collector.RecordConsume("topic1", false, 10*time.Millisecond)

	if collector.ConsumeTotal != 2 {
		t.Errorf("Expected ConsumeTotal=2, got %d", collector.ConsumeTotal)
	}
	if collector.ConsumeSuccess != 1 {
		t.Errorf("Expected ConsumeSuccess=1, got %d", collector.ConsumeSuccess)
	}
	if collector.ConsumeFailed != 1 {
		t.Errorf("Expected ConsumeFailed=1, got %d", collector.ConsumeFailed)
	}

	// 测试连接指标
	collector.RecordConnection(true)
	if !collector.Connected {
		t.Error("Expected Connected=true")
	}

	collector.RecordConnection(false)
	if collector.Connected {
		t.Error("Expected Connected=false")
	}

	// 测试重连指标
	collector.RecordReconnect(true, 1*time.Second)
	collector.RecordReconnect(false, 2*time.Second)

	if collector.ReconnectTotal != 2 {
		t.Errorf("Expected ReconnectTotal=2, got %d", collector.ReconnectTotal)
	}
	if collector.ReconnectSuccess != 1 {
		t.Errorf("Expected ReconnectSuccess=1, got %d", collector.ReconnectSuccess)
	}
	if collector.ReconnectFailed != 1 {
		t.Errorf("Expected ReconnectFailed=1, got %d", collector.ReconnectFailed)
	}

	// 测试积压指标
	collector.RecordBacklog("topic1", 100)
	collector.RecordBacklog("topic2", 200)

	if collector.BacklogByTopic["topic1"] != 100 {
		t.Errorf("Expected BacklogByTopic[topic1]=100, got %d", collector.BacklogByTopic["topic1"])
	}
	if collector.BacklogByTopic["topic2"] != 200 {
		t.Errorf("Expected BacklogByTopic[topic2]=200, got %d", collector.BacklogByTopic["topic2"])
	}

	// 测试积压状态
	collector.RecordBacklogState("topic1", "normal")
	collector.RecordBacklogState("topic2", "warning")

	if collector.BacklogState["topic1"] != "normal" {
		t.Errorf("Expected BacklogState[topic1]=normal, got %s", collector.BacklogState["topic1"])
	}
	if collector.BacklogState["topic2"] != "warning" {
		t.Errorf("Expected BacklogState[topic2]=warning, got %s", collector.BacklogState["topic2"])
	}

	// 测试健康检查指标
	collector.RecordHealthCheck(true, 10*time.Millisecond)
	collector.RecordHealthCheck(true, 15*time.Millisecond)
	collector.RecordHealthCheck(false, 20*time.Millisecond)

	if collector.HealthCheckTotal != 3 {
		t.Errorf("Expected HealthCheckTotal=3, got %d", collector.HealthCheckTotal)
	}
	if collector.HealthCheckHealthy != 2 {
		t.Errorf("Expected HealthCheckHealthy=2, got %d", collector.HealthCheckHealthy)
	}
	if collector.HealthCheckUnhealthy != 1 {
		t.Errorf("Expected HealthCheckUnhealthy=1, got %d", collector.HealthCheckUnhealthy)
	}

	// 测试错误指标
	collector.RecordError("publish", "topic1")
	collector.RecordError("publish", "topic1")
	collector.RecordError("consume", "topic2")

	if collector.ErrorsByType["publish"] != 2 {
		t.Errorf("Expected ErrorsByType[publish]=2, got %d", collector.ErrorsByType["publish"])
	}
	if collector.ErrorsByType["consume"] != 1 {
		t.Errorf("Expected ErrorsByType[consume]=1, got %d", collector.ErrorsByType["consume"])
	}
	if collector.ErrorsByTopic["topic1"] != 2 {
		t.Errorf("Expected ErrorsByTopic[topic1]=2, got %d", collector.ErrorsByTopic["topic1"])
	}
	if collector.ErrorsByTopic["topic2"] != 1 {
		t.Errorf("Expected ErrorsByTopic[topic2]=1, got %d", collector.ErrorsByTopic["topic2"])
	}
}

// TestInMemoryMetricsCollector_GetMetrics 测试获取指标
func TestInMemoryMetricsCollector_GetMetrics(t *testing.T) {
	collector := NewInMemoryMetricsCollector()

	// 记录一些指标
	collector.RecordPublish("topic1", true, 10*time.Millisecond)
	collector.RecordConsume("topic1", true, 5*time.Millisecond)
	collector.RecordBacklog("topic1", 100)
	collector.RecordError("publish", "topic1")

	// 获取指标
	metrics := collector.GetMetrics()

	// 验证指标
	if metrics["publish_total"].(int64) != 1 {
		t.Errorf("Expected publish_total=1, got %v", metrics["publish_total"])
	}
	if metrics["consume_total"].(int64) != 1 {
		t.Errorf("Expected consume_total=1, got %v", metrics["consume_total"])
	}

	backlogByTopic := metrics["backlog_by_topic"].(map[string]int64)
	if backlogByTopic["topic1"] != 100 {
		t.Errorf("Expected backlog_by_topic[topic1]=100, got %d", backlogByTopic["topic1"])
	}

	errorsByType := metrics["errors_by_type"].(map[string]int64)
	if errorsByType["publish"] != 1 {
		t.Errorf("Expected errors_by_type[publish]=1, got %d", errorsByType["publish"])
	}
}

// TestEventBusWithMetricsCollector 测试 EventBus 集成 MetricsCollector
func TestEventBusWithMetricsCollector(t *testing.T) {
	// 创建内存指标收集器
	metricsCollector := NewInMemoryMetricsCollector()

	// 创建 EventBus 配置
	config := GetDefaultConfig("memory")
	config.MetricsCollector = metricsCollector

	// 创建 EventBus
	bus, err := NewEventBus(config)
	if err != nil {
		t.Fatal("Failed to create EventBus:", err)
	}
	defer bus.Close()

	ctx := context.Background()
	topic := "test_topic"

	// 订阅消息
	messageReceived := make(chan bool, 1)
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		messageReceived <- true
		return nil
	})
	if err != nil {
		t.Fatal("Failed to subscribe:", err)
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	err = bus.Publish(ctx, topic, []byte("test message"))
	if err != nil {
		t.Fatal("Failed to publish:", err)
	}

	// 等待消息被消费
	select {
	case <-messageReceived:
		// 消息已接收
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}

	// 等待指标更新
	time.Sleep(100 * time.Millisecond)

	// 验证发布指标
	if metricsCollector.PublishTotal < 1 {
		t.Errorf("Expected PublishTotal >= 1, got %d", metricsCollector.PublishTotal)
	}
	if metricsCollector.PublishSuccess < 1 {
		t.Errorf("Expected PublishSuccess >= 1, got %d", metricsCollector.PublishSuccess)
	}

	// 验证消费指标
	if metricsCollector.ConsumeTotal < 1 {
		t.Errorf("Expected ConsumeTotal >= 1, got %d", metricsCollector.ConsumeTotal)
	}
	if metricsCollector.ConsumeSuccess < 1 {
		t.Errorf("Expected ConsumeSuccess >= 1, got %d", metricsCollector.ConsumeSuccess)
	}
}

// TestEventBusWithoutMetricsCollector 测试 EventBus 不使用 MetricsCollector
func TestEventBusWithoutMetricsCollector(t *testing.T) {
	// 创建 EventBus 配置（不设置 MetricsCollector）
	config := GetDefaultConfig("memory")

	// 创建 EventBus
	bus, err := NewEventBus(config)
	if err != nil {
		t.Fatal("Failed to create EventBus:", err)
	}
	defer bus.Close()

	ctx := context.Background()
	topic := "test_topic"

	// 订阅消息
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		return nil
	})
	if err != nil {
		t.Fatal("Failed to subscribe:", err)
	}

	// 发布消息（应该正常工作，即使没有 MetricsCollector）
	err = bus.Publish(ctx, topic, []byte("test message"))
	if err != nil {
		t.Fatal("Failed to publish:", err)
	}
}
