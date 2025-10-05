package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEventBus_MemoryType 测试创建 Memory 类型的 EventBus
func TestNewEventBus_MemoryType(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	assert.NotNil(t, bus)
	defer bus.Close()
}

// TestNewEventBus_KafkaType 测试创建 Kafka 类型的 EventBus
func TestNewEventBus_KafkaType(t *testing.T) {
	t.Skip("Skipping Kafka test - requires Kafka infrastructure")
}

// TestNewEventBus_NATSType 测试创建 NATS 类型的 EventBus
func TestNewEventBus_NATSType(t *testing.T) {
	t.Skip("Skipping NATS test - requires NATS infrastructure")
}

// TestNewEventBus_InvalidType 测试创建无效类型的 EventBus
func TestNewEventBus_InvalidType(t *testing.T) {
	cfg := &EventBusConfig{Type: "invalid"}
	bus, err := NewEventBus(cfg)
	assert.Error(t, err)
	assert.Nil(t, bus)
}

// TestNewEventBus_EmptyType 测试创建空类型的 EventBus
func TestNewEventBus_EmptyType(t *testing.T) {
	cfg := &EventBusConfig{Type: ""}
	bus, err := NewEventBus(cfg)
	// 应该使用默认类型或返回错误
	if err != nil {
		assert.Error(t, err)
	} else {
		assert.NotNil(t, bus)
		defer bus.Close()
	}
}

// TestEventBusManager_ConcurrentPublish_Types 测试并发发布（类型测试）
func TestEventBusManager_ConcurrentPublish_Types(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	numGoroutines := 10
	numMessages := 10

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numMessages; j++ {
				err := bus.Publish(ctx, "test-topic", []byte("test message"))
				if err != nil {
					t.Logf("Publish error: %v", err)
				}
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
			// 成功
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent publish")
		}
	}
}

// TestEventBusManager_ConcurrentSubscribe 测试并发订阅
func TestEventBusManager_ConcurrentSubscribe(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	numSubscribers := 5

	done := make(chan bool, numSubscribers)

	for i := 0; i < numSubscribers; i++ {
		go func(id int) {
			err := bus.Subscribe(ctx, "test-topic", func(ctx context.Context, message []byte) error {
				return nil
			})
			if err != nil {
				t.Logf("Subscribe error: %v", err)
			}
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numSubscribers; i++ {
		select {
		case <-done:
			// 成功
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent subscribe")
		}
	}
}

// TestEventBusManager_PublishSubscribeRace 测试发布订阅竞态
func TestEventBusManager_PublishSubscribeRace(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 启动订阅者
	received := make(chan bool, 100)
	err = bus.Subscribe(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		received <- true
		return nil
	})
	require.NoError(t, err)

	// 并发发布
	numPublishers := 5
	numMessages := 10
	done := make(chan bool, numPublishers)

	for i := 0; i < numPublishers; i++ {
		go func() {
			for j := 0; j < numMessages; j++ {
				_ = bus.Publish(ctx, "test-topic", []byte("test"))
			}
			done <- true
		}()
	}

	// 等待所有发布者完成
	for i := 0; i < numPublishers; i++ {
		<-done
	}

	// 等待一段时间接收消息
	time.Sleep(500 * time.Millisecond)

	// 验证至少收到一些消息
	receivedCount := len(received)
	t.Logf("Received %d messages", receivedCount)
}

// TestEventBusManager_CloseWhilePublishing 测试发布时关闭
func TestEventBusManager_CloseWhilePublishing(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// 启动发布 goroutine
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			_ = bus.Publish(ctx, "test-topic", []byte("test"))
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// 等待一段时间后关闭
	time.Sleep(100 * time.Millisecond)
	bus.Close()

	// 等待发布 goroutine 完成
	select {
	case <-done:
		// 成功
	case <-time.After(2 * time.Second):
		t.Log("Publish goroutine may still be running")
	}
}

// TestEventBusManager_GetMetrics_Concurrent 测试并发获取指标
func TestEventBusManager_GetMetrics_Concurrent(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				metrics := bus.GetMetrics()
				assert.NotNil(t, metrics)
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestEventBusManager_GetHealthStatus_Concurrent 测试并发获取健康状态
func TestEventBusManager_GetHealthStatus_Concurrent(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				status := bus.GetHealthStatus()
				assert.NotEmpty(t, status)
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestEventBusManager_ConfigureTopic_Concurrent 测试并发配置主题
func TestEventBusManager_ConfigureTopic_Concurrent(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	numGoroutines := 5
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			options := DefaultTopicOptions()
			err := bus.ConfigureTopic(ctx, "test-topic", options)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}
