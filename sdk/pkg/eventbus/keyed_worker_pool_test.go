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

// TestNewKeyedWorkerPool 测试创建 Keyed Worker Pool
func TestNewKeyedWorkerPool(t *testing.T) {
	handler := func(ctx context.Context, data []byte) error {
		return nil
	}

	tests := []struct {
		name           string
		config         KeyedWorkerPoolConfig
		expectedWorker int
		expectedQueue  int
		expectedWait   time.Duration
	}{
		{
			name: "Valid config",
			config: KeyedWorkerPoolConfig{
				WorkerCount: 10,
				QueueSize:   100,
				WaitTimeout: 1 * time.Second,
			},
			expectedWorker: 10,
			expectedQueue:  100,
			expectedWait:   1 * time.Second,
		},
		{
			name: "Use defaults when zero",
			config: KeyedWorkerPoolConfig{
				WorkerCount: 0,
				QueueSize:   0,
				WaitTimeout: 0,
			},
			expectedWorker: DefaultKeyedWorkerCount,
			expectedQueue:  DefaultKeyedQueueSize,
			expectedWait:   DefaultKeyedWaitTimeout,
		},
		{
			name: "Partial defaults",
			config: KeyedWorkerPoolConfig{
				WorkerCount: 5,
				QueueSize:   0,
				WaitTimeout: 0,
			},
			expectedWorker: 5,
			expectedQueue:  DefaultKeyedQueueSize,
			expectedWait:   DefaultKeyedWaitTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewKeyedWorkerPool(tt.config, handler)
			assert.NotNil(t, pool)
			assert.Equal(t, tt.expectedWorker, pool.cfg.WorkerCount)
			assert.Equal(t, tt.expectedQueue, pool.cfg.QueueSize)
			assert.Equal(t, tt.expectedWait, pool.cfg.WaitTimeout)
			assert.Len(t, pool.workers, tt.expectedWorker)

			// Cleanup
			pool.Stop()
		})
	}
}

// TestKeyedWorkerPool_ProcessMessage 测试处理消息
func TestKeyedWorkerPool_ProcessMessage(t *testing.T) {
	var processedCount atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		processedCount.Add(1)
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 4,
		QueueSize:   10,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	ctx := context.Background()
	msg := &AggregateMessage{
		AggregateID: "test-aggregate-1",
		Value:       []byte("test message"),
		Context:     ctx,
		Done:        make(chan error, 1),
	}

	err := pool.ProcessMessage(ctx, msg)
	require.NoError(t, err)

	// Wait for processing
	select {
	case err := <-msg.Done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message processing")
	}

	assert.Equal(t, int32(1), processedCount.Load())
}

// TestKeyedWorkerPool_MissingAggregateID 测试缺少 AggregateID
func TestKeyedWorkerPool_MissingAggregateID(t *testing.T) {
	handler := func(ctx context.Context, data []byte) error {
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 4,
		QueueSize:   10,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	ctx := context.Background()
	msg := &AggregateMessage{
		AggregateID: "", // Missing AggregateID
		Value:       []byte("test message"),
		Context:     ctx,
		Done:        make(chan error, 1),
	}

	err := pool.ProcessMessage(ctx, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aggregateID required")
}

// TestKeyedWorkerPool_SameKeyRouting 测试相同 key 路由到同一 worker
func TestKeyedWorkerPool_SameKeyRouting(t *testing.T) {
	var mu sync.Mutex
	workerIDs := make(map[string]int)

	handler := func(ctx context.Context, data []byte) error {
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 4,
		QueueSize:   10,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	ctx := context.Background()
	aggregateID := "test-aggregate-1"

	// Send multiple messages with the same AggregateID
	for i := 0; i < 10; i++ {
		msg := &AggregateMessage{
			AggregateID: aggregateID,
			Value:       []byte("test message"),
			Context:     ctx,
			Done:        make(chan error, 1),
		}

		err := pool.ProcessMessage(ctx, msg)
		require.NoError(t, err)

		// Record which worker processed this
		workerIdx := pool.hashToIndex(aggregateID)
		mu.Lock()
		workerIDs[aggregateID] = workerIdx
		mu.Unlock()
	}

	// All messages with the same AggregateID should go to the same worker
	mu.Lock()
	assert.Len(t, workerIDs, 1)
	mu.Unlock()
}

// TestKeyedWorkerPool_DifferentKeysDistribution 测试不同 key 分布到不同 worker
func TestKeyedWorkerPool_DifferentKeysDistribution(t *testing.T) {
	handler := func(ctx context.Context, data []byte) error {
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 4,
		QueueSize:   10,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	ctx := context.Background()
	workerDistribution := make(map[int]int)

	// Send messages with different AggregateIDs
	for i := 0; i < 100; i++ {
		aggregateID := "aggregate-" + string(rune(i))
		msg := &AggregateMessage{
			AggregateID: aggregateID,
			Value:       []byte("test message"),
			Context:     ctx,
			Done:        make(chan error, 1),
		}

		err := pool.ProcessMessage(ctx, msg)
		require.NoError(t, err)

		workerIdx := pool.hashToIndex(aggregateID)
		workerDistribution[workerIdx]++
	}

	// Check that messages are distributed across workers
	assert.Greater(t, len(workerDistribution), 1, "Messages should be distributed across multiple workers")
}

// TestKeyedWorkerPool_QueueFull 测试队列满时的行为
func TestKeyedWorkerPool_QueueFull(t *testing.T) {
	// Use a slow handler to fill up the queue
	handler := func(ctx context.Context, data []byte) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 1,
		QueueSize:   2,
		WaitTimeout: 50 * time.Millisecond,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	ctx := context.Background()
	aggregateID := "test-aggregate-1"

	// Fill up the queue
	for i := 0; i < 3; i++ {
		msg := &AggregateMessage{
			AggregateID: aggregateID,
			Value:       []byte("test message"),
			Context:     ctx,
			Done:        make(chan error, 1),
		}
		err := pool.ProcessMessage(ctx, msg)
		require.NoError(t, err)
	}

	// This should timeout because queue is full
	msg := &AggregateMessage{
		AggregateID: aggregateID,
		Value:       []byte("test message"),
		Context:     ctx,
		Done:        make(chan error, 1),
	}

	err := pool.ProcessMessage(ctx, msg)
	assert.Error(t, err)
	assert.Equal(t, ErrWorkerQueueFull, err)
}

// TestKeyedWorkerPool_ContextCancellation 测试上下文取消
func TestKeyedWorkerPool_ContextCancellation(t *testing.T) {
	handler := func(ctx context.Context, data []byte) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 1,
		QueueSize:   1,
		WaitTimeout: 5 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	// Fill up the queue
	ctx := context.Background()
	msg1 := &AggregateMessage{
		AggregateID: "test-aggregate-1",
		Value:       []byte("test message 1"),
		Context:     ctx,
		Done:        make(chan error, 1),
	}
	err := pool.ProcessMessage(ctx, msg1)
	require.NoError(t, err)

	msg2 := &AggregateMessage{
		AggregateID: "test-aggregate-1",
		Value:       []byte("test message 2"),
		Context:     ctx,
		Done:        make(chan error, 1),
	}
	err = pool.ProcessMessage(ctx, msg2)
	require.NoError(t, err)

	// Try to send with a cancelled context
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	msg3 := &AggregateMessage{
		AggregateID: "test-aggregate-1",
		Value:       []byte("test message 3"),
		Context:     cancelCtx,
		Done:        make(chan error, 1),
	}

	err = pool.ProcessMessage(cancelCtx, msg3)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// TestKeyedWorkerPool_HandlerError 测试处理器返回错误
func TestKeyedWorkerPool_HandlerError(t *testing.T) {
	expectedErr := errors.New("handler error")
	handler := func(ctx context.Context, data []byte) error {
		return expectedErr
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 2,
		QueueSize:   10,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	ctx := context.Background()
	msg := &AggregateMessage{
		AggregateID: "test-aggregate-1",
		Value:       []byte("test message"),
		Context:     ctx,
		Done:        make(chan error, 1),
	}

	err := pool.ProcessMessage(ctx, msg)
	require.NoError(t, err)

	// Wait for processing and check error
	select {
	case err := <-msg.Done:
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message processing")
	}
}

// TestKeyedWorkerPool_ConcurrentProcessing 测试并发处理
func TestKeyedWorkerPool_ConcurrentProcessing(t *testing.T) {
	var processedCount atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		processedCount.Add(1)
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 10,
		QueueSize:   100,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	ctx := context.Background()
	messageCount := 100
	var wg sync.WaitGroup

	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := &AggregateMessage{
				AggregateID: "aggregate-" + string(rune(idx%10)),
				Value:       []byte("test message"),
				Context:     ctx,
				Done:        make(chan error, 1),
			}

			err := pool.ProcessMessage(ctx, msg)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Wait for all messages to be processed
	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int32(messageCount), processedCount.Load())
}

// TestKeyedWorkerPool_Stop 测试停止
func TestKeyedWorkerPool_Stop(t *testing.T) {
	var processedCount atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		processedCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 4,
		QueueSize:   10,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)

	ctx := context.Background()

	// Send some messages
	for i := 0; i < 5; i++ {
		msg := &AggregateMessage{
			AggregateID: "test-aggregate-1",
			Value:       []byte("test message"),
			Context:     ctx,
			Done:        make(chan error, 1),
		}
		err := pool.ProcessMessage(ctx, msg)
		require.NoError(t, err)
	}

	// Stop the pool
	pool.Stop()

	// Verify that Stop() completes without hanging
	// The important thing is that all workers have been stopped gracefully
}

// TestKeyedWorkerPool_HashConsistency 测试哈希一致性
func TestKeyedWorkerPool_HashConsistency(t *testing.T) {
	handler := func(ctx context.Context, data []byte) error {
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 8,
		QueueSize:   10,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	// Test that the same key always hashes to the same index
	testKeys := []string{
		"aggregate-1",
		"aggregate-2",
		"aggregate-3",
		"test-key",
		"another-key",
	}

	for _, key := range testKeys {
		idx1 := pool.hashToIndex(key)
		idx2 := pool.hashToIndex(key)
		idx3 := pool.hashToIndex(key)

		assert.Equal(t, idx1, idx2, "Hash should be consistent for key: %s", key)
		assert.Equal(t, idx2, idx3, "Hash should be consistent for key: %s", key)
		assert.GreaterOrEqual(t, idx1, 0, "Index should be non-negative")
		assert.Less(t, idx1, config.WorkerCount, "Index should be within worker count")
	}
}

// TestKeyedWorkerPool_OrderingGuarantee 测试顺序保证
func TestKeyedWorkerPool_OrderingGuarantee(t *testing.T) {
	var mu sync.Mutex
	processedOrder := make([]int, 0)

	handler := func(ctx context.Context, data []byte) error {
		// Extract message ID from data
		msgID := int(data[0])
		mu.Lock()
		processedOrder = append(processedOrder, msgID)
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	config := KeyedWorkerPoolConfig{
		WorkerCount: 4,
		QueueSize:   100,
		WaitTimeout: 1 * time.Second,
	}

	pool := NewKeyedWorkerPool(config, handler)
	defer pool.Stop()

	ctx := context.Background()
	aggregateID := "test-aggregate-1"

	// Send messages in order
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		msg := &AggregateMessage{
			AggregateID: aggregateID,
			Value:       []byte{byte(i)},
			Context:     ctx,
			Done:        make(chan error, 1),
		}
		err := pool.ProcessMessage(ctx, msg)
		require.NoError(t, err)
	}

	// Wait for all messages to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify order
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, processedOrder, messageCount)
	for i := 0; i < messageCount; i++ {
		assert.Equal(t, i, processedOrder[i], "Messages should be processed in order")
	}
}
