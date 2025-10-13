package function_tests

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TestKafkaBasicPublishSubscribe 测试 Kafka 基础发布订阅功能
func TestKafkaBasicPublishSubscribe(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// 创建唯一的 topic
	topic := fmt.Sprintf("test.kafka.basic.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	// 创建 EventBus
	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-basic-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 订阅消息
	var received int64
	var lastMessage []byte
	ctx := context.Background()

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		lastMessage = message
		t.Logf("📨 Received message: %s", string(message))
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	// 等待订阅就绪
	time.Sleep(2 * time.Second)

	// 发布消息
	testMessage := []byte("Hello Kafka!")
	err = bus.Publish(ctx, topic, testMessage)
	helper.AssertNoError(err, "Publish should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 10*time.Second)
	helper.AssertTrue(success, "Should receive message within timeout")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&received), "Should receive exactly 1 message")
	helper.AssertEqual(string(testMessage), string(lastMessage), "Message content should match")

	t.Logf("✅ Kafka basic publish/subscribe test passed")
}

// TestNATSBasicPublishSubscribe 测试 NATS 基础发布订阅功能
func TestNATSBasicPublishSubscribe(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// 创建唯一的 topic
	topic := fmt.Sprintf("test.nats.basic.%d", helper.GetTimestamp())

	// 创建 EventBus
	clientID := fmt.Sprintf("nats-basic-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	// 清理 NATS stream
	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_BASIC_%d", helper.GetTimestamp()/1000))

	// 订阅消息
	var received int64
	var lastMessage []byte
	ctx := context.Background()

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		lastMessage = message
		t.Logf("📨 Received message: %s", string(message))
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	// 等待订阅就绪
	time.Sleep(2 * time.Second)

	// 发布消息
	testMessage := []byte("Hello NATS!")
	err = bus.Publish(ctx, topic, testMessage)
	helper.AssertNoError(err, "Publish should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 10*time.Second)
	helper.AssertTrue(success, "Should receive message within timeout")
	helper.AssertEqual(int64(1), atomic.LoadInt64(&received), "Should receive exactly 1 message")
	helper.AssertEqual(string(testMessage), string(lastMessage), "Message content should match")

	t.Logf("✅ NATS basic publish/subscribe test passed")
}

// TestKafkaMultipleMessages 测试 Kafka 多消息发布订阅
func TestKafkaMultipleMessages(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.multiple.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-multiple-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	var received int64
	ctx := context.Background()

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	// 发布多条消息
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		message := []byte(fmt.Sprintf("Message %d", i))
		err = bus.Publish(ctx, topic, message)
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待所有消息接收
	success := helper.WaitForMessages(&received, int64(messageCount), 15*time.Second)
	helper.AssertTrue(success, "Should receive all messages within timeout")
	helper.AssertEqual(int64(messageCount), atomic.LoadInt64(&received), "Should receive all messages")

	t.Logf("✅ Kafka multiple messages test passed")
}

// TestNATSMultipleMessages 测试 NATS 多消息发布订阅
func TestNATSMultipleMessages(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.multiple.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-multiple-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_MULTIPLE_%d", helper.GetTimestamp()/1000))

	var received int64
	ctx := context.Background()

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	// 发布多条消息
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		message := []byte(fmt.Sprintf("Message %d", i))
		err = bus.Publish(ctx, topic, message)
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待所有消息接收
	success := helper.WaitForMessages(&received, int64(messageCount), 15*time.Second)
	helper.AssertTrue(success, "Should receive all messages within timeout")
	helper.AssertEqual(int64(messageCount), atomic.LoadInt64(&received), "Should receive all messages")

	t.Logf("✅ NATS multiple messages test passed")
}

// TestKafkaPublishWithOptions 测试 Kafka PublishWithOptions
func TestKafkaPublishWithOptions(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.options.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-options-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	var received int64
	ctx := context.Background()

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	// 使用选项发布消息
	testMessage := []byte("Message with options")
	opts := eventbus.PublishOptions{
		Metadata: map[string]string{
			"key": "test-key",
		},
	}
	err = bus.PublishWithOptions(ctx, topic, testMessage, opts)
	helper.AssertNoError(err, "PublishWithOptions should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 10*time.Second)
	helper.AssertTrue(success, "Should receive message within timeout")

	t.Logf("✅ Kafka PublishWithOptions test passed")
}

// TestNATSPublishWithOptions 测试 NATS PublishWithOptions
func TestNATSPublishWithOptions(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.options.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-options-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_OPTIONS_%d", helper.GetTimestamp()/1000))

	var received int64
	ctx := context.Background()

	err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&received, 1)
		return nil
	})
	helper.AssertNoError(err, "Subscribe should not return error")

	time.Sleep(2 * time.Second)

	// 使用选项发布消息
	testMessage := []byte("Message with options")
	opts := eventbus.PublishOptions{
		Metadata: map[string]string{
			"key": "test-key",
		},
	}
	err = bus.PublishWithOptions(ctx, topic, testMessage, opts)
	helper.AssertNoError(err, "PublishWithOptions should not return error")

	// 等待消息接收
	success := helper.WaitForMessages(&received, 1, 10*time.Second)
	helper.AssertTrue(success, "Should receive message within timeout")

	t.Logf("✅ NATS PublishWithOptions test passed")
}
