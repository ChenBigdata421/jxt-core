package eventbus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHollywoodActorPool_Initialization 测试 Actor Pool 初始化
func TestHollywoodActorPool_Initialization(t *testing.T) {
	t.Run("成功初始化", func(t *testing.T) {
		metrics := &NoOpActorPoolMetricsCollector{}
		pool := NewHollywoodActorPool(HollywoodActorPoolConfig{
			PoolSize:    256,
			InboxSize:   1000,
			MaxRestarts: 3,
		}, metrics)

		require.NotNil(t, pool)
		assert.NotNil(t, pool.engine)
		assert.Len(t, pool.actors, 256)
		assert.Len(t, pool.inboxDepthCounters, 256)

		// 清理
		pool.Stop()
	})

	t.Run("默认配置", func(t *testing.T) {
		metrics := &NoOpActorPoolMetricsCollector{}
		pool := NewHollywoodActorPool(HollywoodActorPoolConfig{}, metrics)

		require.NotNil(t, pool)
		assert.Equal(t, 256, pool.config.PoolSize)
		assert.Equal(t, 1000, pool.config.InboxSize)
		assert.Equal(t, 3, pool.config.MaxRestarts)

		// 清理
		pool.Stop()
	})
}

// TestHollywoodActorPool_ProcessMessage 测试消息处理
func TestHollywoodActorPool_ProcessMessage(t *testing.T) {
	t.Run("成功处理消息", func(t *testing.T) {
		metrics := &NoOpActorPoolMetricsCollector{}
		pool := NewHollywoodActorPool(HollywoodActorPoolConfig{
			PoolSize:    256,
			InboxSize:   1000,
			MaxRestarts: 3,
		}, metrics)
		defer pool.Stop()

		ctx := context.Background()
		var processed atomic.Bool

		msg := &AggregateMessage{
			AggregateID: "order-123",
			Context:     ctx,
			Done:        make(chan error, 1),
			Handler: func(ctx context.Context, data []byte) error {
				processed.Store(true)
				return nil
			},
			Value: []byte("test"),
		}

		err := pool.ProcessMessage(ctx, msg)
		require.NoError(t, err)

		// 等待处理完成
		select {
		case err := <-msg.Done:
			assert.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("超时等待消息处理")
		}

		assert.True(t, processed.Load())
	})

	t.Run("缺少AggregateID", func(t *testing.T) {
		metrics := &NoOpActorPoolMetricsCollector{}
		pool := NewHollywoodActorPool(HollywoodActorPoolConfig{
			PoolSize:    256,
			InboxSize:   1000,
			MaxRestarts: 3,
		}, metrics)
		defer pool.Stop()

		ctx := context.Background()
		msg := &AggregateMessage{
			AggregateID: "", // 空的聚合ID
			Context:     ctx,
			Done:        make(chan error, 1),
			Handler: func(ctx context.Context, data []byte) error {
				return nil
			},
			Value: []byte("test"),
		}

		err := pool.ProcessMessage(ctx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "aggregateID required")
	})
}

// TestHollywoodActorPool_OrderGuarantee 测试顺序保证
func TestHollywoodActorPool_OrderGuarantee(t *testing.T) {
	t.Run("同一聚合ID顺序处理", func(t *testing.T) {
		metrics := &NoOpActorPoolMetricsCollector{}
		pool := NewHollywoodActorPool(HollywoodActorPoolConfig{
			PoolSize:    256,
			InboxSize:   1000,
			MaxRestarts: 3,
		}, metrics)
		defer pool.Stop()

		ctx := context.Background()
		aggregateID := "order-123"

		var processedOrder []int
		var mu sync.Mutex

		// 发送10条消息
		for i := 0; i < 10; i++ {
			msgNum := i
			msg := &AggregateMessage{
				AggregateID: aggregateID,
				Context:     ctx,
				Done:        make(chan error, 1),
				Handler: func(ctx context.Context, data []byte) error {
					mu.Lock()
					processedOrder = append(processedOrder, msgNum)
					mu.Unlock()
					time.Sleep(10 * time.Millisecond) // 模拟处理时间
					return nil
				},
				Value: []byte("test"),
			}

			err := pool.ProcessMessage(ctx, msg)
			require.NoError(t, err)
		}

		// 等待所有消息处理完成
		time.Sleep(500 * time.Millisecond)

		// 验证顺序
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, 10, len(processedOrder))
		for i := 0; i < 10; i++ {
			assert.Equal(t, i, processedOrder[i], "消息应该按顺序处理")
		}
	})
}

// TestHollywoodActorPool_ErrorHandling 测试错误处理
func TestHollywoodActorPool_ErrorHandling(t *testing.T) {
	t.Run("处理器返回错误", func(t *testing.T) {
		metrics := &NoOpActorPoolMetricsCollector{}
		pool := NewHollywoodActorPool(HollywoodActorPoolConfig{
			PoolSize:    256,
			InboxSize:   1000,
			MaxRestarts: 3,
		}, metrics)
		defer pool.Stop()

		ctx := context.Background()
		expectedErr := errors.New("处理失败")

		msg := &AggregateMessage{
			AggregateID: "order-123",
			Context:     ctx,
			Done:        make(chan error, 1),
			Handler: func(ctx context.Context, data []byte) error {
				return expectedErr
			},
			Value: []byte("test"),
		}

		err := pool.ProcessMessage(ctx, msg)
		require.NoError(t, err)

		// 等待处理完成
		select {
		case err := <-msg.Done:
			assert.Equal(t, expectedErr, err)
		case <-time.After(2 * time.Second):
			t.Fatal("超时等待错误")
		}
	})
}

// TestHollywoodActorPool_ConcurrentProcessing 测试并发处理
func TestHollywoodActorPool_ConcurrentProcessing(t *testing.T) {
	t.Run("不同聚合ID并发处理", func(t *testing.T) {
		metrics := &NoOpActorPoolMetricsCollector{}
		pool := NewHollywoodActorPool(HollywoodActorPoolConfig{
			PoolSize:    256,
			InboxSize:   1000,
			MaxRestarts: 3,
		}, metrics)
		defer pool.Stop()

		ctx := context.Background()
		var processedCount atomic.Int32

		// 发送100条不同聚合ID的消息
		for i := 0; i < 100; i++ {
			aggregateID := "order-" + string(rune(i))
			msg := &AggregateMessage{
				AggregateID: aggregateID,
				Context:     ctx,
				Done:        make(chan error, 1),
				Handler: func(ctx context.Context, data []byte) error {
					processedCount.Add(1)
					time.Sleep(10 * time.Millisecond)
					return nil
				},
				Value: []byte("test"),
			}

			err := pool.ProcessMessage(ctx, msg)
			require.NoError(t, err)
		}

		// 等待所有消息处理完成
		time.Sleep(2 * time.Second)

		// 验证所有消息都被处理
		assert.Equal(t, int32(100), processedCount.Load())
	})
}

// TestHollywoodActorPool_Stop 测试停止
func TestHollywoodActorPool_Stop(t *testing.T) {
	t.Run("正常停止", func(t *testing.T) {
		metrics := &NoOpActorPoolMetricsCollector{}
		pool := NewHollywoodActorPool(HollywoodActorPoolConfig{
			PoolSize:    256,
			InboxSize:   1000,
			MaxRestarts: 3,
		}, metrics)

		// 发送一些消息
		ctx := context.Background()
		for i := 0; i < 10; i++ {
			msg := &AggregateMessage{
				AggregateID: "order-" + string(rune(i)),
				Context:     ctx,
				Done:        make(chan error, 1),
				Handler: func(ctx context.Context, data []byte) error {
					return nil
				},
				Value: []byte("test"),
			}
			_ = pool.ProcessMessage(ctx, msg)
		}

		// 停止Pool
		pool.Stop()

		// 等待停止完成
		time.Sleep(100 * time.Millisecond)
	})
}
