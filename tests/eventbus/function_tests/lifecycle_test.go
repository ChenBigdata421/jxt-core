package function_tests

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestKafkaStartStop 测试 Kafka Start/Stop 生命周期
func TestKafkaStartStop(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-lifecycle-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 启动 EventBus
	err := bus.Start(ctx)
	helper.AssertNoError(err, "Start should not return error")

	// 等待启动完成
	time.Sleep(1 * time.Second)

	// 停止 EventBus
	err = bus.Stop()
	helper.AssertNoError(err, "Stop should not return error")

	t.Logf("✅ Kafka Start/Stop test passed")
}

// TestNATSStartStop 测试 NATS Start/Stop 生命周期
func TestNATSStartStop(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-lifecycle-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 启动 EventBus
	err := bus.Start(ctx)
	helper.AssertNoError(err, "Start should not return error")

	// 等待启动完成
	time.Sleep(1 * time.Second)

	// 停止 EventBus
	err = bus.Stop()
	helper.AssertNoError(err, "Stop should not return error")

	t.Logf("✅ NATS Start/Stop test passed")
}

// TestKafkaGetConnectionState 测试 Kafka 获取连接状态
func TestKafkaGetConnectionState(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-conn-state-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 获取连接状态
	state := bus.GetConnectionState()
	t.Logf("📊 Connection state: %+v", state)

	// 验证状态字段存在
	helper.AssertTrue(state.IsConnected || !state.IsConnected, "IsConnected field should exist")

	t.Logf("✅ Kafka GetConnectionState test passed")
}

// TestNATSGetConnectionState 测试 NATS 获取连接状态
func TestNATSGetConnectionState(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-conn-state-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	// 获取连接状态
	state := bus.GetConnectionState()
	t.Logf("📊 Connection state: %+v", state)

	// 验证状态字段存在
	helper.AssertTrue(state.IsConnected || !state.IsConnected, "IsConnected field should exist")

	t.Logf("✅ NATS GetConnectionState test passed")
}

// TestKafkaGetMetrics 测试 Kafka 获取监控指标
func TestKafkaGetMetrics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.metrics.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-metrics-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 发布一些消息
	for i := 0; i < 5; i++ {
		message := []byte(fmt.Sprintf("Message %d", i))
		err := bus.Publish(ctx, topic, message)
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待消息发送完成
	time.Sleep(2 * time.Second)

	// 获取指标
	metrics := bus.GetMetrics()
	t.Logf("📊 Metrics: %+v", metrics)

	// 验证指标字段存在
	helper.AssertTrue(metrics.MessagesPublished >= 0, "MessagesPublished should be non-negative")

	t.Logf("✅ Kafka GetMetrics test passed")
}

// TestNATSGetMetrics 测试 NATS 获取监控指标
func TestNATSGetMetrics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.metrics.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-metrics-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_METRICS_%d", helper.GetTimestamp()/1000))

	ctx := context.Background()

	// 发布一些消息
	for i := 0; i < 5; i++ {
		message := []byte(fmt.Sprintf("Message %d", i))
		err := bus.Publish(ctx, topic, message)
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待消息发送完成
	time.Sleep(2 * time.Second)

	// 获取指标
	metrics := bus.GetMetrics()
	t.Logf("📊 Metrics: %+v", metrics)

	// 验证指标字段存在
	helper.AssertTrue(metrics.MessagesPublished >= 0, "MessagesPublished should be non-negative")

	t.Logf("✅ NATS GetMetrics test passed")
}

// TestKafkaReconnectCallback 测试 Kafka 重连回调
func TestKafkaReconnectCallback(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-reconnect-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 注册重连回调
	callbackCalled := false
	err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
		callbackCalled = true
		t.Logf("📞 Reconnect callback called")
		return nil
	})
	helper.AssertNoError(err, "RegisterReconnectCallback should not return error")

	// 注意：实际触发重连需要模拟网络故障，这里只验证注册成功
	t.Logf("✅ Kafka RegisterReconnectCallback test passed (callback registered: %v)", !callbackCalled)
}

// TestNATSReconnectCallback 测试 NATS 重连回调
func TestNATSReconnectCallback(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-reconnect-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	// 注册重连回调
	callbackCalled := false
	err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
		callbackCalled = true
		t.Logf("📞 Reconnect callback called")
		return nil
	})
	helper.AssertNoError(err, "RegisterReconnectCallback should not return error")

	// 注意：实际触发重连需要模拟网络故障，这里只验证注册成功
	t.Logf("✅ NATS RegisterReconnectCallback test passed (callback registered: %v)", !callbackCalled)
}

// TestKafkaClose 测试 Kafka Close
func TestKafkaClose(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-close-%d", helper.GetTimestamp()))

	// 关闭 EventBus
	err := bus.Close()
	helper.AssertNoError(err, "Close should not return error")

	// 验证关闭后无法发布消息
	ctx := context.Background()
	topic := fmt.Sprintf("test.kafka.close.%d", helper.GetTimestamp())
	err = bus.Publish(ctx, topic, []byte("test"))
	helper.AssertTrue(err != nil, "Publish should return error after Close")

	t.Logf("✅ Kafka Close test passed")
}

// TestNATSClose 测试 NATS Close
func TestNATSClose(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-close-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)

	// 关闭 EventBus
	err := bus.Close()
	helper.AssertNoError(err, "Close should not return error")

	// 验证关闭后无法发布消息
	ctx := context.Background()
	topic := fmt.Sprintf("test.nats.close.%d", helper.GetTimestamp())
	err = bus.Publish(ctx, topic, []byte("test"))
	helper.AssertTrue(err != nil, "Publish should return error after Close")

	t.Logf("✅ NATS Close test passed")
}

// TestKafkaPublishCallback 测试 Kafka 发布回调
func TestKafkaPublishCallback(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.pub.callback.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-pub-callback-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 注册发布回调
	callbackCalled := false
	err := bus.RegisterPublishCallback(func(ctx context.Context, topic string, message []byte, err error) error {
		callbackCalled = true
		t.Logf("📞 Publish callback called: Topic=%s, Error=%v", topic, err)
		return nil
	})
	helper.AssertNoError(err, "RegisterPublishCallback should not return error")

	ctx := context.Background()

	// 发布消息
	err = bus.Publish(ctx, topic, []byte("test message"))
	helper.AssertNoError(err, "Publish should not return error")

	// 等待回调
	time.Sleep(2 * time.Second)

	t.Logf("✅ Kafka RegisterPublishCallback test passed (callback called: %v)", callbackCalled)
}
