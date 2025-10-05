package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_PublishSubscribe_LargeMessage 测试发布订阅（大消息）
func TestEventBusManager_PublishSubscribe_LargeMessage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	// 订阅
	received := make(chan []byte, 1)
	handler := func(ctx context.Context, message []byte) error {
		received <- message
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 创建大消息（1MB）
	largeMessage := make([]byte, 1024*1024)
	for i := range largeMessage {
		largeMessage[i] = byte(i % 256)
	}

	// 发布
	err = bus.Publish(ctx, topic, largeMessage)
	require.NoError(t, err)

	// 等待接收
	select {
	case msg := <-received:
		assert.Equal(t, len(largeMessage), len(msg))
		assert.Equal(t, largeMessage, msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestEventBusManager_PublishEnvelope_LargePayload 测试发布 Envelope（大负载）
func TestEventBusManager_PublishEnvelope_LargePayload(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	// 订阅
	received := make(chan *Envelope, 1)
	handler := func(ctx context.Context, envelope *Envelope) error {
		received <- envelope
		return nil
	}

	err = bus.SubscribeEnvelope(ctx, topic, handler)
	require.NoError(t, err)

	// 创建大负载（1MB）
	largePayload := make([]byte, 1024*1024)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	// 创建 Envelope
	envelope := NewEnvelope("test-aggregate", "test-event", 1, largePayload)

	// 发布
	err = bus.PublishEnvelope(ctx, topic, envelope)
	require.NoError(t, err)

	// 等待接收
	select {
	case env := <-received:
		assert.Equal(t, "test-aggregate", env.AggregateID)
		assert.Equal(t, "test-event", env.EventType)
		assert.Equal(t, len(largePayload), len(env.Payload))
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for envelope")
	}
}

// TestEventBusManager_MultipleTopics 测试多个主题
func TestEventBusManager_MultipleTopics(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅多个主题
	topics := []string{"topic1", "topic2", "topic3"}
	received := make(map[string]chan []byte)

	for _, topic := range topics {
		received[topic] = make(chan []byte, 1)
		topicCopy := topic // 捕获变量
		handler := func(ctx context.Context, message []byte) error {
			received[topicCopy] <- message
			return nil
		}
		err = bus.Subscribe(ctx, topic, handler)
		require.NoError(t, err)
	}

	// 发布到每个主题
	for _, topic := range topics {
		message := []byte("message for " + topic)
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 验证每个主题都收到了消息
	for _, topic := range topics {
		select {
		case msg := <-received[topic]:
			assert.Equal(t, []byte("message for "+topic), msg)
		case <-time.After(1 * time.Second):
			t.Fatalf("Timeout waiting for message on topic %s", topic)
		}
	}
}

// TestEventBusManager_SubscribeMultipleHandlers 测试订阅多个处理器
func TestEventBusManager_SubscribeMultipleHandlers(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	// 订阅多个处理器
	received1 := make(chan []byte, 1)
	handler1 := func(ctx context.Context, message []byte) error {
		received1 <- message
		return nil
	}

	received2 := make(chan []byte, 1)
	handler2 := func(ctx context.Context, message []byte) error {
		received2 <- message
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler1)
	require.NoError(t, err)

	err = bus.Subscribe(ctx, topic, handler2)
	require.NoError(t, err)

	// 发布消息
	message := []byte("test message")
	err = bus.Publish(ctx, topic, message)
	require.NoError(t, err)

	// 验证两个处理器都收到了消息
	select {
	case msg := <-received1:
		assert.Equal(t, message, msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message in handler1")
	}

	select {
	case msg := <-received2:
		assert.Equal(t, message, msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message in handler2")
	}
}

// TestEventBusManager_EmptyMessage 测试空消息
func TestEventBusManager_EmptyMessage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-topic"

	// 订阅
	received := make(chan []byte, 1)
	handler := func(ctx context.Context, message []byte) error {
		received <- message
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 发布空消息
	err = bus.Publish(ctx, topic, []byte{})
	require.NoError(t, err)

	// 等待接收
	select {
	case msg := <-received:
		assert.Equal(t, []byte{}, msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}
