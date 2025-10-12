package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryEventBus_ConcurrentPublishSubscribe 测试Memory EventBus的并发安全性
// 验证修复：handlers切片的并发安全问题
func TestMemoryEventBus_ConcurrentPublishSubscribe(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topic := "concurrent-test"
	var receivedCount int64
	var subscribeCount int64

	// 创建一个处理器
	handler := func(ctx context.Context, message []byte) error {
		atomic.AddInt64(&receivedCount, 1)
		time.Sleep(1 * time.Millisecond) // 模拟处理时间
		return nil
	}

	// 并发订阅和取消订阅（模拟动态订阅场景）
	var wg sync.WaitGroup
	
	// 启动多个goroutine进行订阅
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				err := bus.Subscribe(ctx, topic, handler)
				if err == nil {
					atomic.AddInt64(&subscribeCount, 1)
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// 启动多个goroutine进行发布
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				message := []byte("test-message")
				err := bus.Publish(ctx, topic, message)
				assert.NoError(t, err)
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// 等待所有消息处理完成
	time.Sleep(500 * time.Millisecond)

	// 验证结果
	finalReceivedCount := atomic.LoadInt64(&receivedCount)
	finalSubscribeCount := atomic.LoadInt64(&subscribeCount)
	
	t.Logf("Subscriptions: %d, Messages received: %d", finalSubscribeCount, finalReceivedCount)
	
	// 应该有订阅成功
	assert.Greater(t, finalSubscribeCount, int64(0))
	// 应该有消息被接收（由于并发，可能不是所有消息都被处理）
	assert.Greater(t, finalReceivedCount, int64(0))
}



// TestMemoryEventBus_DynamicSubscribers 测试动态添加/删除订阅者
func TestMemoryEventBus_DynamicSubscribers(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()
	topic := "dynamic-test"

	var messageCount int64

	// 创建多个handler
	createHandler := func(id int) MessageHandler {
		return func(ctx context.Context, message []byte) error {
			atomic.AddInt64(&messageCount, 1)
			t.Logf("Handler %d processed message: %s", id, string(message))
			return nil
		}
	}

	// 动态添加订阅者
	for i := 0; i < 5; i++ {
		err := bus.Subscribe(ctx, topic, createHandler(i))
		require.NoError(t, err)
		
		// 发布消息测试当前订阅者数量
		err = bus.Publish(ctx, topic, []byte("test message"))
		require.NoError(t, err)
		
		time.Sleep(50 * time.Millisecond)
	}

	// 等待所有消息处理完成
	time.Sleep(200 * time.Millisecond)

	// 验证消息计数
	// 第1次发布：1个订阅者 = 1条消息
	// 第2次发布：2个订阅者 = 2条消息
	// ...
	// 第5次发布：5个订阅者 = 5条消息
	// 总计：1+2+3+4+5 = 15条消息
	expectedCount := int64(15)
	actualCount := atomic.LoadInt64(&messageCount)
	
	t.Logf("Expected: %d, Actual: %d", expectedCount, actualCount)
	assert.Equal(t, expectedCount, actualCount)
}

// TestEventBusManager_DoubleClose 测试重复关闭
func TestEventBusManager_DoubleClose(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	// 第一次关闭
	err1 := bus.Close()
	assert.NoError(t, err1)

	// 第二次关闭（应该不报错）
	err2 := bus.Close()
	assert.NoError(t, err2)

	// 关闭后尝试操作（应该报错）
	ctx := context.Background()
	err = bus.Publish(ctx, "test", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}



// TestEventBusManager_NilHandlers 测试nil handler
func TestEventBusManager_NilHandlers(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 测试Subscribe with nil handler
	err = bus.Subscribe(ctx, "test", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler cannot be nil")

	// 测试SubscribeEnvelope with nil handler
	err = bus.SubscribeEnvelope(ctx, "test", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler cannot be nil")
}

// TestEventBusManager_EmptyTopic 测试空topic
func TestEventBusManager_EmptyTopic(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	handler := func(ctx context.Context, message []byte) error {
		return nil
	}

	// 测试空topic订阅
	err = bus.Subscribe(ctx, "", handler)
	// Memory实现可能允许空topic，但应该有一致的行为
	// 这里我们记录当前行为，如果需要可以后续修改
	t.Logf("Empty topic subscribe result: %v", err)

	// 测试空topic发布
	err = bus.Publish(ctx, "", []byte("test"))
	t.Logf("Empty topic publish result: %v", err)
}

// TestEventBusManager_LargeMessage 测试大消息
func TestEventBusManager_LargeMessage(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "large-message-test"

	var receivedSize int64
	handler := func(ctx context.Context, message []byte) error {
		atomic.StoreInt64(&receivedSize, int64(len(message)))
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 创建1MB的消息
	largeMessage := make([]byte, 1024*1024)
	for i := range largeMessage {
		largeMessage[i] = byte(i % 256)
	}

	err = bus.Publish(ctx, topic, largeMessage)
	require.NoError(t, err)

	// 等待处理完成
	time.Sleep(200 * time.Millisecond)

	// 验证消息大小
	assert.Equal(t, int64(len(largeMessage)), atomic.LoadInt64(&receivedSize))
}


