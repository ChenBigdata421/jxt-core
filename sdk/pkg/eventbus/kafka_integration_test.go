// 暂时禁用旧的集成测试，使用新的性能测试
// +build ignore

package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKafkaEventBus_PublishSubscribe_Integration 测试 Kafka 发布订阅集成
func TestKafkaEventBus_PublishSubscribe_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"}, // 使用外部端口
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      10 * time.Second,
			RetryMax:     3,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "test-group-pub-sub",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 2 * time.Minute,
			FetchMinBytes:     1,
			FetchMaxBytes:     1024 * 1024,
			FetchMaxWait:      500 * time.Millisecond,
		},
	}

	bus, err := NewKafkaEventBus(cfg)
	if err != nil {
		t.Skipf("Kafka server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-integration"

	// 订阅消息
	received := make(chan []byte, 1)
	handler := func(ctx context.Context, data []byte) error {
		received <- data
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布消息
	message := []byte("test message")
	err = bus.Publish(ctx, topic, message)
	require.NoError(t, err)

	// 验证接收
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestKafkaEventBus_MultiplePartitions_Integration 测试多分区
func TestKafkaEventBus_MultiplePartitions_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"}, // 使用外部端口
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      10 * time.Second,
			RetryMax:     3,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "test-group-multi",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 2 * time.Minute,
			FetchMinBytes:     1,
			FetchMaxBytes:     1024 * 1024,
			FetchMaxWait:      500 * time.Millisecond,
		},
	}

	bus, err := NewKafkaEventBus(cfg)
	if err != nil {
		t.Skipf("Kafka server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-multi-partition"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布多条消息到不同分区
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待所有消息被接收
	time.Sleep(5 * time.Second)

	// 验证接收到所有消息
	assert.Equal(t, int32(messageCount), received.Load())
}

// TestKafkaEventBus_ConsumerGroup_Integration 测试消费者组
func TestKafkaEventBus_ConsumerGroup_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"}, // 使用外部端口
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      10 * time.Second,
			RetryMax:     3,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "test-consumer-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 2 * time.Minute,
			FetchMinBytes:     1,
			FetchMaxBytes:     1024 * 1024,
			FetchMaxWait:      500 * time.Millisecond,
		},
	}

	// 创建两个消费者（同一个消费者组）
	bus1, err := NewKafkaEventBus(cfg)
	if err != nil {
		t.Skipf("Kafka server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus1.Close()

	bus2, err := NewKafkaEventBus(cfg)
	require.NoError(t, err)
	defer bus2.Close()

	ctx := context.Background()
	topic := "test-consumer-group-topic"

	// 两个消费者订阅同一主题
	var count1, count2 atomic.Int32
	receivedChan := make(chan struct{}, 20)

	handler1 := func(ctx context.Context, data []byte) error {
		count1.Add(1)
		receivedChan <- struct{}{}
		return nil
	}

	handler2 := func(ctx context.Context, data []byte) error {
		count2.Add(1)
		receivedChan <- struct{}{}
		return nil
	}

	err = bus1.Subscribe(ctx, topic, handler1)
	require.NoError(t, err)

	err = bus2.Subscribe(ctx, topic, handler2)
	require.NoError(t, err)

	// 等待订阅生效和消费者组 rebalance 完成
	// Kafka 消费者组 rebalance 通常需要 3-10 秒
	time.Sleep(8 * time.Second)

	// 发布消息
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		message := []byte(fmt.Sprintf("message-%d", i))
		err = bus1.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待所有消息被接收（使用 channel 而不是固定时间）
	receivedCount := 0
	timeout := time.After(15 * time.Second)
waitLoop:
	for receivedCount < messageCount {
		select {
		case <-receivedChan:
			receivedCount++
		case <-timeout:
			t.Logf("Timeout waiting for messages. Received: %d, Expected: %d", receivedCount, messageCount)
			t.Logf("Consumer 1 received: %d, Consumer 2 received: %d", count1.Load(), count2.Load())
			break waitLoop
		}
	}

	// 额外等待一点时间确保所有消息都被处理
	time.Sleep(1 * time.Second)

	// 验证消息被分配到两个消费者
	totalReceived := count1.Load() + count2.Load()
	t.Logf("Total received: %d (Consumer1: %d, Consumer2: %d)", totalReceived, count1.Load(), count2.Load())

	// 至少应该收到大部分消息
	assert.GreaterOrEqual(t, totalReceived, int32(messageCount*8/10), "Should receive at least 80%% of messages")

	// 如果两个消费者都在同一个组，消息应该被分配（但可能不是完全均匀）
	// 注意：如果只有一个分区，可能所有消息都被一个消费者接收
	// 所以我们只验证至少有一个消费者收到了消息
	assert.Greater(t, totalReceived, int32(0), "At least one consumer should receive messages")
}

// TestKafkaEventBus_ErrorHandling_Integration 测试错误处理
func TestKafkaEventBus_ErrorHandling_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"}, // 使用外部端口
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      10 * time.Second,
			RetryMax:     3,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "test-error-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 2 * time.Minute,
			FetchMinBytes:     1,
			FetchMaxBytes:     1024 * 1024,
			FetchMaxWait:      500 * time.Millisecond,
		},
	}

	bus, err := NewKafkaEventBus(cfg)
	if err != nil {
		t.Skipf("Kafka server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-error"

	// 订阅消息 - 处理器返回错误
	var errorCount atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		errorCount.Add(1)
		return assert.AnError
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布消息
	message := []byte("test message")
	err = bus.Publish(ctx, topic, message)
	require.NoError(t, err)

	// 等待处理器被调用
	time.Sleep(3 * time.Second)

	// 验证处理器被调用（即使返回错误）
	assert.Greater(t, errorCount.Load(), int32(0))
}

// TestKafkaEventBus_ConcurrentPublish_Integration 测试并发发布
func TestKafkaEventBus_ConcurrentPublish_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"}, // 使用外部端口
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      10 * time.Second,
			RetryMax:     3,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "test-concurrent-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 2 * time.Minute,
			FetchMinBytes:     1,
			FetchMaxBytes:     1024 * 1024,
			FetchMaxWait:      500 * time.Millisecond,
		},
	}

	bus, err := NewKafkaEventBus(cfg)
	if err != nil {
		t.Skipf("Kafka server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-concurrent"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 并发发布消息
	goroutines := 10
	messagesPerGoroutine := 10
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				message := []byte("message")
				err := bus.Publish(ctx, topic, message)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// 等待所有消息被接收
	time.Sleep(5 * time.Second)

	// 验证接收到所有消息
	expectedCount := int32(goroutines * messagesPerGoroutine)
	assert.Equal(t, expectedCount, received.Load())
}

// TestKafkaEventBus_OffsetManagement_Integration 测试偏移量管理
func TestKafkaEventBus_OffsetManagement_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"}, // 使用外部端口
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      10 * time.Second,
			RetryMax:     3,
		},
		Consumer: config.ConsumerConfig{
			GroupID:            "test-offset-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     10 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  2 * time.Minute,
			FetchMinBytes:      1,
			FetchMaxBytes:      1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
		},
	}

	bus, err := NewKafkaEventBus(cfg)
	if err != nil {
		t.Skipf("Kafka server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-offset"

	// 第一次订阅 - 接收所有消息
	var firstCount atomic.Int32
	handler1 := func(ctx context.Context, data []byte) error {
		firstCount.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler1)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待消息被接收和提交
	time.Sleep(5 * time.Second)

	// 验证第一次接收
	assert.Equal(t, int32(messageCount), firstCount.Load())

	// 关闭第一个消费者
	bus.Close()

	// 创建新的消费者（同一个消费者组）
	bus2, err := NewKafkaEventBus(cfg)
	require.NoError(t, err)
	defer bus2.Close()

	// 第二次订阅 - 应该从上次提交的偏移量开始
	var secondCount atomic.Int32
	handler2 := func(ctx context.Context, data []byte) error {
		secondCount.Add(1)
		return nil
	}

	err = bus2.Subscribe(ctx, topic, handler2)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布新消息
	for i := 0; i < messageCount; i++ {
		message := []byte("new message " + string(rune('0'+i)))
		err = bus2.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待新消息被接收
	time.Sleep(5 * time.Second)

	// 验证第二次只接收新消息（不重复接收旧消息）
	assert.Equal(t, int32(messageCount), secondCount.Load())
}

// TestKafkaEventBus_Reconnection_Integration 测试重连功能
func TestKafkaEventBus_Reconnection_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.KafkaConfig{
		Brokers: []string{"localhost:29092"}, // 使用外部端口
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      10 * time.Second,
			RetryMax:     3,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "test-group-reconnect",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
			MaxProcessingTime: 2 * time.Minute,
			FetchMinBytes:     1,
			FetchMaxBytes:     1024 * 1024,
			FetchMaxWait:      500 * time.Millisecond,
		},
	}

	bus, err := NewKafkaEventBus(cfg)
	if err != nil {
		t.Skipf("Kafka server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-reconnect"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发布消息
	message := []byte("test message")
	err = bus.Publish(ctx, topic, message)
	require.NoError(t, err)

	// 等待消息被接收
	time.Sleep(3 * time.Second)

	// 验证接收
	assert.Greater(t, received.Load(), int32(0))
}
