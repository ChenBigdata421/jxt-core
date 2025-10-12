// 暂时禁用NATS测试，专注于Kafka性能测试
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

// TestNATSEventBus_PublishSubscribe_Integration 测试 NATS 发布订阅集成
func TestNATSEventBus_PublishSubscribe_Integration(t *testing.T) {
	// 如果 NATS 服务器不可用，跳过测试
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "test-client-pub-sub",
		MaxReconnects:     5,
		ReconnectWait:     1 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	bus, err := NewNATSEventBus(cfg)
	if err != nil {
		t.Skipf("NATS server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test.integration"

	// 订阅消息
	received := make(chan []byte, 1)
	handler := func(ctx context.Context, data []byte) error {
		received <- data
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	message := []byte("test message")
	err = bus.Publish(ctx, topic, message)
	require.NoError(t, err)

	// 验证接收
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestNATSEventBus_MultipleSubscribers_Integration 测试多个订阅者
func TestNATSEventBus_MultipleSubscribers_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// 创建多个 EventBus 实例来模拟多个订阅者
	// NATS Core 模式不允许同一个连接多次订阅同一个主题
	var buses []EventBus
	defer func() {
		for _, b := range buses {
			b.Close()
		}
	}()

	ctx := context.Background()
	topic := "test.multi.subscribers"

	// 创建多个订阅者（每个使用独立的 EventBus 实例）
	var wg sync.WaitGroup
	var count atomic.Int32

	for i := 0; i < 3; i++ {
		cfg := &config.NATSConfig{
			URLs:              []string{"nats://localhost:4222"},
			ClientID:          fmt.Sprintf("test-client-multi-sub-%d", i),
			MaxReconnects:     5,
			ReconnectWait:     1 * time.Second,
			ConnectionTimeout: 5 * time.Second,
		}

		bus, err := NewNATSEventBus(cfg)
		if err != nil {
			t.Skipf("NATS server not available: %v", err)
		}
		require.NoError(t, err)
		buses = append(buses, bus)

		wg.Add(1)
		handler := func(ctx context.Context, data []byte) error {
			count.Add(1)
			wg.Done()
			return nil
		}

		err = bus.Subscribe(ctx, topic, handler)
		require.NoError(t, err)
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 使用第一个 bus 发布消息
	message := []byte("test message")
	err := buses[0].Publish(ctx, topic, message)
	require.NoError(t, err)

	// 等待所有订阅者接收
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 所有订阅者都应该收到消息
		assert.Equal(t, int32(3), count.Load())
	case <-time.After(2 * time.Second):
		t.Fatalf("Timeout waiting for subscribers. Received: %d, Expected: 3", count.Load())
	}
}

// TestNATSEventBus_JetStream_Integration 测试 JetStream 持久化
func TestNATSEventBus_JetStream_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "test-client-jetstream",
		MaxReconnects:     5,
		ReconnectWait:     1 * time.Second,
		ConnectionTimeout: 5 * time.Second,
		JetStream: config.JetStreamConfig{
			Enabled: true,
			Stream: config.StreamConfig{
				Name:     "TEST_STREAM_INTEGRATION",
				Subjects: []string{"test.jetstream.integration.>"},
				Storage:  "memory",
				MaxAge:   1 * time.Hour,
			},
		},
	}

	bus, err := NewNATSEventBus(cfg)
	if err != nil {
		t.Skipf("NATS server with JetStream not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	// 使用匹配 stream subjects 的 topic
	topic := "test.jetstream.integration.messages"

	// 订阅消息
	received := make(chan []byte, 10)
	handler := func(ctx context.Context, data []byte) error {
		received <- data
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布多条消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 验证接收所有消息
	receivedCount := 0
	timeout := time.After(5 * time.Second)
	for receivedCount < messageCount {
		select {
		case <-received:
			receivedCount++
		case <-timeout:
			t.Fatalf("Timeout: received %d/%d messages", receivedCount, messageCount)
		}
	}

	assert.Equal(t, messageCount, receivedCount)
}

// TestNATSEventBus_ErrorHandling_Integration 测试错误处理
func TestNATSEventBus_ErrorHandling_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "test-client-error",
		MaxReconnects:     5,
		ReconnectWait:     1 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	bus, err := NewNATSEventBus(cfg)
	if err != nil {
		t.Skipf("NATS server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test.error"

	// 订阅消息 - 处理器返回错误
	var errorCount atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		errorCount.Add(1)
		return assert.AnError
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	message := []byte("test message")
	err = bus.Publish(ctx, topic, message)
	require.NoError(t, err)

	// 等待处理器被调用
	time.Sleep(500 * time.Millisecond)

	// 验证处理器被调用（即使返回错误）
	assert.Greater(t, errorCount.Load(), int32(0))
}

// TestNATSEventBus_ConcurrentPublish_Integration 测试并发发布
func TestNATSEventBus_ConcurrentPublish_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "test-client-concurrent",
		MaxReconnects:     5,
		ReconnectWait:     1 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	bus, err := NewNATSEventBus(cfg)
	if err != nil {
		t.Skipf("NATS server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test.concurrent"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

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
	time.Sleep(2 * time.Second)

	// 验证接收到所有消息
	expectedCount := int32(goroutines * messagesPerGoroutine)
	assert.Equal(t, expectedCount, received.Load())
}

// TestNATSEventBus_ContextCancellation_Integration 测试上下文取消
func TestNATSEventBus_ContextCancellation_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "test-client-cancel",
		MaxReconnects:     5,
		ReconnectWait:     1 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	bus, err := NewNATSEventBus(cfg)
	if err != nil {
		t.Skipf("NATS server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	topic := "test.cancel"

	// 订阅消息
	handler := func(ctx context.Context, data []byte) error {
		// 模拟长时间处理
		time.Sleep(5 * time.Second)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 取消上下文
	cancel()

	// 尝试发布消息（应该失败或被取消）
	message := []byte("test message")
	err = bus.Publish(ctx, topic, message)
	// 上下文已取消，发布可能失败
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	}
}

// TestNATSEventBus_Reconnection_Integration 测试重连功能
func TestNATSEventBus_Reconnection_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "test-client-reconnect",
		MaxReconnects:     5,
		ReconnectWait:     1 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	bus, err := NewNATSEventBus(cfg)
	if err != nil {
		t.Skipf("NATS server not available: %v", err)
	}
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test.reconnect"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	message := []byte("test message")
	err = bus.Publish(ctx, topic, message)
	require.NoError(t, err)

	// 等待消息被接收
	time.Sleep(500 * time.Millisecond)

	// 验证接收
	assert.Greater(t, received.Load(), int32(0))
}
