package eventbus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryEventBus_Integration 测试 Memory EventBus 集成
func TestMemoryEventBus_Integration(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	// Test publish and subscribe
	var receivedCount atomic.Int32
	var receivedData []byte

	handler := func(ctx context.Context, data []byte) error {
		receivedCount.Add(1)
		receivedData = data
		return nil
	}

	// Subscribe
	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// Publish
	testData := []byte("test message")
	err = bus.Publish(ctx, topic, testData)
	require.NoError(t, err)

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify
	assert.Equal(t, int32(1), receivedCount.Load())
	assert.Equal(t, testData, receivedData)
}

// TestMemoryEventBus_MultipleSubscribers 测试多个订阅者
func TestMemoryEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	var count1, count2, count3 atomic.Int32

	handler1 := func(ctx context.Context, data []byte) error {
		count1.Add(1)
		return nil
	}

	handler2 := func(ctx context.Context, data []byte) error {
		count2.Add(1)
		return nil
	}

	handler3 := func(ctx context.Context, data []byte) error {
		count3.Add(1)
		return nil
	}

	// Subscribe multiple handlers
	err := bus.Subscribe(ctx, topic, handler1)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, topic, handler2)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, topic, handler3)
	require.NoError(t, err)

	// Publish
	err = bus.Publish(ctx, topic, []byte("test"))
	require.NoError(t, err)

	// Wait for messages to be processed
	time.Sleep(100 * time.Millisecond)

	// All handlers should receive the message
	assert.Equal(t, int32(1), count1.Load())
	assert.Equal(t, int32(1), count2.Load())
	assert.Equal(t, int32(1), count3.Load())
}

// TestMemoryEventBus_MultipleTopics 测试多个主题
func TestMemoryEventBus_MultipleTopics(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()

	var topic1Count, topic2Count atomic.Int32

	handler1 := func(ctx context.Context, data []byte) error {
		topic1Count.Add(1)
		return nil
	}

	handler2 := func(ctx context.Context, data []byte) error {
		topic2Count.Add(1)
		return nil
	}

	// Subscribe to different topics
	err := bus.Subscribe(ctx, "topic1", handler1)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, "topic2", handler2)
	require.NoError(t, err)

	// Publish to topic1
	err = bus.Publish(ctx, "topic1", []byte("message1"))
	require.NoError(t, err)

	// Publish to topic2
	err = bus.Publish(ctx, "topic2", []byte("message2"))
	require.NoError(t, err)

	// Wait for messages to be processed
	time.Sleep(100 * time.Millisecond)

	// Each handler should only receive messages from its topic
	assert.Equal(t, int32(1), topic1Count.Load())
	assert.Equal(t, int32(1), topic2Count.Load())
}

// TestMemoryEventBus_ConcurrentPublish 测试并发发布
func TestMemoryEventBus_ConcurrentPublish(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	var receivedCount atomic.Int32

	handler := func(ctx context.Context, data []byte) error {
		receivedCount.Add(1)
		return nil
	}

	// Subscribe
	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// Publish concurrently
	messageCount := 100
	done := make(chan bool, messageCount)

	for i := 0; i < messageCount; i++ {
		go func(idx int) {
			err := bus.Publish(ctx, topic, []byte("message"))
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all publishes to complete
	for i := 0; i < messageCount; i++ {
		<-done
	}

	// Wait for messages to be processed
	time.Sleep(500 * time.Millisecond)

	// All messages should be received
	assert.Equal(t, int32(messageCount), receivedCount.Load())
}

// TestMemoryEventBus_Close 测试关闭
func TestMemoryEventBus_Close(t *testing.T) {
	bus := NewMemoryEventBus()

	ctx := context.Background()
	topic := "test-topic"

	handler := func(ctx context.Context, data []byte) error {
		return nil
	}

	// Subscribe
	err := bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// Close
	err = bus.Close()
	require.NoError(t, err)

	// Publishing after close should fail or be ignored
	err = bus.Publish(ctx, topic, []byte("message"))
	// The behavior depends on implementation, but it should not panic
}

// TestEnvelope_Integration 测试 Envelope 集成
func TestEnvelope_Integration(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	var receivedEnvelope *Envelope

	handler := func(ctx context.Context, env *Envelope) error {
		receivedEnvelope = env
		return nil
	}

	// Subscribe with envelope
	err := bus.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err)

	// Create envelope
	testData := []byte("test message")
	envelope := NewEnvelope("test-aggregate-1", "test.event", 1, testData)

	// Publish with envelope
	err = bus.PublishEnvelope(ctx, topic, envelope)
	require.NoError(t, err)

	// Wait for message to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify
	assert.NotNil(t, receivedEnvelope)
	assert.Equal(t, "test-aggregate-1", receivedEnvelope.AggregateID)
	assert.Equal(t, "test.event", receivedEnvelope.EventType)
	assert.Equal(t, testData, []byte(receivedEnvelope.Payload))
}

// TestReconnectConfig_Defaults 测试重连配置默认值
func TestReconnectConfig_Defaults(t *testing.T) {
	config := DefaultReconnectConfig()

	assert.Equal(t, DefaultMaxReconnectAttempts, config.MaxAttempts)
	assert.Equal(t, DefaultReconnectInitialBackoff, config.InitialBackoff)
	assert.Equal(t, DefaultReconnectMaxBackoff, config.MaxBackoff)
	assert.Equal(t, DefaultReconnectBackoffFactor, config.BackoffFactor)
	assert.Equal(t, DefaultReconnectFailureThreshold, config.FailureThreshold)
}
