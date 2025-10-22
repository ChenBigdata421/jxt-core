package function_tests

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TestPrometheusIntegration_BasicPublishSubscribe 测试 Prometheus 集成基本发布订阅指标
func TestPrometheusIntegration_BasicPublishSubscribe(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// 使用 InMemoryMetricsCollector 进行测试（更容易验证）
	collector := eventbus.NewInMemoryMetricsCollector()

	// 创建 Memory EventBus 并注入指标收集器
	config := eventbus.GetDefaultConfig("memory")
	config.MetricsCollector = collector

	bus, err := eventbus.NewEventBus(config)
	helper.AssertNoError(err, "Failed to create EventBus")
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	topic := "test.prometheus.basic"

	// 订阅消息
	var received int64
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发布 5 条消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		err = bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待消息接收
	success := helper.WaitForMessages(&received, int64(messageCount), 3*time.Second)
	helper.AssertTrue(success, "Should receive all messages")

	// 验证指标
	helper.AssertEqual(int64(messageCount), collector.PublishTotal, "Publish total should match")
	helper.AssertEqual(int64(messageCount), collector.PublishSuccess, "Publish success should match")
	helper.AssertEqual(int64(0), collector.PublishFailed, "Publish failed should be 0")

	helper.AssertEqual(int64(messageCount), collector.ConsumeTotal, "Consume total should match")
	helper.AssertEqual(int64(messageCount), collector.ConsumeSuccess, "Consume success should match")
	helper.AssertEqual(int64(0), collector.ConsumeFailed, "Consume failed should be 0")

	t.Logf("✅ Prometheus 基本发布订阅指标测试通过")
}

// TestPrometheusIntegration_PublishFailure 测试 Prometheus 发布失败指标
func TestPrometheusIntegration_PublishFailure(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	// 创建 Memory EventBus
	config := eventbus.GetDefaultConfig("memory")
	config.MetricsCollector = collector

	bus, err := eventbus.NewEventBus(config)
	helper.AssertNoError(err, "Failed to create EventBus")
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	topic := "test.prometheus.failure"

	// 订阅消息，模拟处理失败
	var received int64
	var failed int64
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		count := atomic.AddInt64(&received, 1)
		// 奇数消息失败
		if count%2 == 1 {
			atomic.AddInt64(&failed, 1)
			return fmt.Errorf("simulated error")
		}
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发布 10 条消息
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		err = bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待消息接收
	success := helper.WaitForMessages(&received, int64(messageCount), 3*time.Second)
	helper.AssertTrue(success, "Should receive all messages")

	// 验证指标
	helper.AssertEqual(int64(messageCount), collector.PublishTotal, "Publish total should match")
	helper.AssertEqual(int64(messageCount), collector.PublishSuccess, "Publish success should match")

	helper.AssertEqual(int64(messageCount), collector.ConsumeTotal, "Consume total should match")
	helper.AssertEqual(failed, collector.ConsumeFailed, "Consume failed should match")
	expectedSuccess := int64(messageCount) - failed
	helper.AssertEqual(expectedSuccess, collector.ConsumeSuccess, "Consume success should match")

	t.Logf("✅ Prometheus 发布失败指标测试通过 (失败: %d, 成功: %d)", failed, expectedSuccess)
}

// TestPrometheusIntegration_MultipleTopics 测试 Prometheus 多主题指标
func TestPrometheusIntegration_MultipleTopics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	config := eventbus.GetDefaultConfig("memory")
	config.MetricsCollector = collector

	bus, err := eventbus.NewEventBus(config)
	helper.AssertNoError(err, "Failed to create EventBus")
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 定义多个主题
	topics := []struct {
		name  string
		count int
	}{
		{"test.prometheus.topic1", 5},
		{"test.prometheus.topic2", 10},
		{"test.prometheus.topic3", 3},
	}

	totalMessages := 0
	// 为每个主题订阅和发布
	for _, topicInfo := range topics {
		var received int64
		err = bus.Subscribe(ctx, topicInfo.name, func(ctx context.Context, message []byte) error {
			atomic.AddInt64(&received, 1)
			return nil
		})
		helper.AssertNoError(err, "Subscribe should not return error")

		time.Sleep(50 * time.Millisecond)

		// 发布消息
		for i := 0; i < topicInfo.count; i++ {
			err = bus.Publish(ctx, topicInfo.name, []byte(fmt.Sprintf("message-%d", i)))
			helper.AssertNoError(err, "Publish should not return error")
		}

		// 等待消息接收
		success := helper.WaitForMessages(&received, int64(topicInfo.count), 3*time.Second)
		helper.AssertTrue(success, fmt.Sprintf("Should receive all messages for topic %s", topicInfo.name))

		totalMessages += topicInfo.count
	}

	// 验证总体指标（InMemoryMetricsCollector 不区分主题）
	helper.AssertEqual(int64(totalMessages), collector.PublishTotal, "Total publish should match")
	helper.AssertEqual(int64(totalMessages), collector.ConsumeTotal, "Total consume should match")

	t.Logf("✅ Prometheus 多主题指标测试通过 (总消息数: %d)", totalMessages)
}

// TestPrometheusIntegration_Latency 测试 Prometheus 延迟指标
func TestPrometheusIntegration_Latency(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	config := eventbus.GetDefaultConfig("memory")
	config.MetricsCollector = collector

	bus, err := eventbus.NewEventBus(config)
	helper.AssertNoError(err, "Failed to create EventBus")
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	topic := "test.prometheus.latency"

	var received int64
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		// 模拟处理延迟
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发布消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		err = bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
		helper.AssertNoError(err, "Publish should not return error")
	}

	success := helper.WaitForMessages(&received, int64(messageCount), 5*time.Second)
	helper.AssertTrue(success, "Should receive all messages")

	// 验证延迟被记录（InMemoryMetricsCollector 记录最后一次的延迟）
	helper.AssertTrue(collector.PublishLatency > 0, "Publish latency should be recorded")
	helper.AssertTrue(collector.ConsumeLatency >= 10*time.Millisecond, "Consume latency should be at least 10ms")

	t.Logf("✅ Prometheus 延迟指标测试通过 (Publish: %v, Consume: %v)", collector.PublishLatency, collector.ConsumeLatency)
}

// TestPrometheusIntegration_ConnectionMetrics 测试 Prometheus 连接指标
func TestPrometheusIntegration_ConnectionMetrics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	// 手动记录连接状态变化
	collector.RecordConnection(true)
	collector.RecordReconnect(true, 100*time.Millisecond)
	collector.RecordReconnect(false, 200*time.Millisecond)
	collector.RecordReconnect(true, 150*time.Millisecond)

	// 验证指标
	helper.AssertTrue(collector.Connected, "Connected should be true")
	helper.AssertEqual(int64(3), collector.ReconnectTotal, "Reconnect total should be 3")
	helper.AssertEqual(int64(2), collector.ReconnectSuccess, "Reconnect success should be 2")
	helper.AssertEqual(int64(1), collector.ReconnectFailed, "Reconnect failed should be 1")

	t.Logf("✅ Prometheus 连接指标测试通过")
}

// TestPrometheusIntegration_BacklogMetrics 测试 Prometheus 积压指标
func TestPrometheusIntegration_BacklogMetrics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	topic1 := "test.prometheus.backlog1"
	topic2 := "test.prometheus.backlog2"

	// 记录积压指标
	collector.RecordBacklog(topic1, 100)
	collector.RecordBacklogState(topic1, "normal")

	collector.RecordBacklog(topic2, 500)
	collector.RecordBacklogState(topic2, "warning")

	// 验证积压数量
	helper.AssertEqual(int64(100), collector.BacklogByTopic[topic1], "Backlog for topic1 should be 100")
	helper.AssertEqual(int64(500), collector.BacklogByTopic[topic2], "Backlog for topic2 should be 500")

	// 验证积压状态
	helper.AssertEqual("normal", collector.BacklogState[topic1], "Backlog state for topic1 should be normal")
	helper.AssertEqual("warning", collector.BacklogState[topic2], "Backlog state for topic2 should be warning")

	t.Logf("✅ Prometheus 积压指标测试通过")
}

// TestPrometheusIntegration_HealthCheckMetrics 测试 Prometheus 健康检查指标
func TestPrometheusIntegration_HealthCheckMetrics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	// 记录健康检查
	collector.RecordHealthCheck(true, 10*time.Millisecond)
	collector.RecordHealthCheck(true, 15*time.Millisecond)
	collector.RecordHealthCheck(false, 20*time.Millisecond)
	collector.RecordHealthCheck(true, 12*time.Millisecond)

	// 验证健康检查指标
	helper.AssertEqual(int64(4), collector.HealthCheckTotal, "Health check total should be 4")
	helper.AssertEqual(int64(3), collector.HealthCheckHealthy, "Health check healthy should be 3")
	helper.AssertEqual(int64(1), collector.HealthCheckUnhealthy, "Health check unhealthy should be 1")

	t.Logf("✅ Prometheus 健康检查指标测试通过")
}

// TestPrometheusIntegration_ErrorMetrics 测试 Prometheus 错误指标
func TestPrometheusIntegration_ErrorMetrics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	// 记录各种错误
	collector.RecordError("publish", "topic1")
	collector.RecordError("publish", "topic1")
	collector.RecordError("consume", "topic2")
	collector.RecordError("connection", "")
	collector.RecordError("publish", "topic3")

	// 验证按类型分类的错误
	helper.AssertEqual(int64(3), collector.ErrorsByType["publish"], "Publish errors should be 3")
	helper.AssertEqual(int64(1), collector.ErrorsByType["consume"], "Consume errors should be 1")
	helper.AssertEqual(int64(1), collector.ErrorsByType["connection"], "Connection errors should be 1")

	// 验证按主题分类的错误
	helper.AssertEqual(int64(2), collector.ErrorsByTopic["topic1"], "Topic1 errors should be 2")
	helper.AssertEqual(int64(1), collector.ErrorsByTopic["topic2"], "Topic2 errors should be 1")
	helper.AssertEqual(int64(1), collector.ErrorsByTopic["topic3"], "Topic3 errors should be 1")

	t.Logf("✅ Prometheus 错误指标测试通过")
}

// TestPrometheusIntegration_BatchMetrics 测试 Prometheus 批量操作指标
func TestPrometheusIntegration_BatchMetrics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	topic := "test.prometheus.batch"

	// 记录批量发布
	collector.RecordPublishBatch(topic, 10, true, 50*time.Millisecond)
	collector.RecordPublishBatch(topic, 5, true, 30*time.Millisecond)
	collector.RecordPublishBatch(topic, 3, false, 40*time.Millisecond)

	// 记录批量消费
	collector.RecordConsumeBatch(topic, 15, true, 100*time.Millisecond)
	collector.RecordConsumeBatch(topic, 2, false, 80*time.Millisecond)

	// 验证发布指标
	helper.AssertEqual(int64(18), collector.PublishTotal, "Publish total should be 18 (10+5+3)")
	helper.AssertEqual(int64(15), collector.PublishSuccess, "Publish success should be 15 (10+5)")
	helper.AssertEqual(int64(3), collector.PublishFailed, "Publish failed should be 3")

	// 验证消费指标
	helper.AssertEqual(int64(17), collector.ConsumeTotal, "Consume total should be 17 (15+2)")
	helper.AssertEqual(int64(15), collector.ConsumeSuccess, "Consume success should be 15")
	helper.AssertEqual(int64(2), collector.ConsumeFailed, "Consume failed should be 2")

	t.Logf("✅ Prometheus 批量操作指标测试通过")
}

// TestPrometheusIntegration_E2E_Kafka 测试 Kafka EventBus 的 Prometheus 集成
// 注意：由于 Kafka EventBus 的架构限制，MetricsCollector 目前仅在 Memory EventBus 中完全支持
// Kafka EventBus 需要在内部实现 MetricsCollector 集成，这是一个待实现的功能
func TestPrometheusIntegration_E2E_Kafka(t *testing.T) {
	t.Skip("Skipping Kafka Prometheus integration test: MetricsCollector not yet implemented in KafkaEventBus")

	if testing.Short() {
		t.Skip("Skipping Kafka Prometheus integration test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	// 创建 Kafka EventBus 并注入指标收集器
	clientID := fmt.Sprintf("kafka-prometheus-%d", helper.GetTimestamp())

	// 使用与其他 Kafka 测试相同的配置，并注入指标收集器
	cfg := &eventbus.KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: eventbus.ProducerConfig{
			RequiredAcks:    1,
			FlushFrequency:  10 * time.Millisecond,
			FlushMessages:   100,
			Timeout:         5 * time.Second,
			FlushBytes:      1048576,
			RetryMax:        3,
			BatchSize:       16384,
			BufferSize:      8388608,
			MaxMessageBytes: 1048576,
		},
		Consumer: eventbus.ConsumerConfig{
			GroupID:            fmt.Sprintf("test-group-%s", clientID),
			SessionTimeout:     10 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  5 * time.Second,
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			FetchMaxWait:       100 * time.Millisecond,
			MaxPollRecords:     500,
		},
		Net: eventbus.NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		ClientID: clientID,
	}

	busConfig := &eventbus.EventBusConfig{
		Type:             "kafka",
		Kafka:            *cfg,
		MetricsCollector: collector,
	}

	kafkaBusWithMetrics, err := eventbus.NewEventBus(busConfig)
	if err != nil {
		t.Skipf("Skipping Kafka test: %v", err)
		return
	}
	defer helper.CloseEventBus(kafkaBusWithMetrics)

	ctx := context.Background()
	topic := fmt.Sprintf("test.prometheus.kafka.%d", helper.GetTimestamp())

	// 订阅消息
	var received int64
	err = kafkaBusWithMetrics.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	// 发布消息
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		err = kafkaBusWithMetrics.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待消息接收
	success := helper.WaitForMessages(&received, int64(messageCount), 10*time.Second)
	helper.AssertTrue(success, "Should receive all messages")

	// 验证指标
	helper.AssertTrue(collector.PublishTotal >= int64(messageCount), "Publish total should be at least message count")
	helper.AssertTrue(collector.ConsumeTotal >= int64(messageCount), "Consume total should be at least message count")

	t.Logf("✅ Kafka Prometheus 集成测试通过 (Publish: %d, Consume: %d)", collector.PublishTotal, collector.ConsumeTotal)
}

// TestPrometheusIntegration_E2E_Memory 测试 Memory EventBus 的 Prometheus 集成
func TestPrometheusIntegration_E2E_Memory(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	collector := eventbus.NewInMemoryMetricsCollector()

	// 创建 Memory EventBus 并注入指标收集器
	config := eventbus.GetDefaultConfig("memory")
	config.MetricsCollector = collector

	bus, err := eventbus.NewEventBus(config)
	helper.AssertNoError(err, "Failed to create Memory EventBus")
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	topic := "test.prometheus.memory"

	// 订阅消息
	var received int64
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(100 * time.Millisecond)

	// 发布消息
	messageCount := 20
	for i := 0; i < messageCount; i++ {
		err = bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待消息接收
	success := helper.WaitForMessages(&received, int64(messageCount), 5*time.Second)
	helper.AssertTrue(success, "Should receive all messages")

	// 验证指标
	helper.AssertEqual(int64(messageCount), collector.PublishTotal, "Publish total should match")
	helper.AssertEqual(int64(messageCount), collector.PublishSuccess, "Publish success should match")
	helper.AssertEqual(int64(messageCount), collector.ConsumeTotal, "Consume total should match")
	helper.AssertEqual(int64(messageCount), collector.ConsumeSuccess, "Consume success should match")

	t.Logf("✅ Memory Prometheus 集成测试通过 (Publish: %d, Consume: %d)", collector.PublishTotal, collector.ConsumeTotal)
}
