package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========== 内部函数测试 ==========
// 这些测试专门针对包内部的私有函数，确保核心逻辑的正确性

// TestExtractAggregateID 测试聚合ID提取逻辑
func TestExtractAggregateID(t *testing.T) {
	tests := []struct {
		name        string
		msgBytes    []byte
		headers     map[string]string
		kafkaKey    []byte
		natsSubject string
		want        string
		wantErr     bool
	}{
		{
			name: "从Envelope提取（优先级1）",
			msgBytes: func() []byte {
				env := NewEnvelope("order-123", "OrderCreated", 1, []byte(`{}`))
				data, _ := env.ToBytes()
				return data
			}(),
			headers:     map[string]string{"X-Aggregate-ID": "header-456"},
			kafkaKey:    []byte("key-789"),
			natsSubject: "events.order.created.subject-999",
			want:        "order-123",
			wantErr:     false,
		},
		{
			name:        "从Header提取（优先级2）",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{"X-Aggregate-ID": "header-456"},
			kafkaKey:    []byte("key-789"),
			natsSubject: "events.order.created.subject-999",
			want:        "header-456",
			wantErr:     false,
		},
		{
			name:        "从Kafka Key提取（优先级3）",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{},
			kafkaKey:    []byte("key-789"),
			natsSubject: "events.order.created.subject-999",
			want:        "key-789",
			wantErr:     false,
		},
		{
			name:        "从NATS Subject提取（优先级4）",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{},
			kafkaKey:    []byte(""),
			natsSubject: "events.order.created.subject-999",
			want:        "subject-999",
			wantErr:     false,
		},
		{
			name:        "未找到聚合ID",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{},
			kafkaKey:    []byte(""),
			natsSubject: "",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Header多种格式支持",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{"aggregate-id": "header-lowercase"},
			kafkaKey:    []byte(""),
			natsSubject: "",
			want:        "header-lowercase",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractAggregateID(tt.msgBytes, tt.headers, tt.kafkaKey, tt.natsSubject)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestValidateAggregateID 测试聚合ID验证逻辑
func TestValidateAggregateID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"有效的简单ID", "order-123", false},
		{"有效的冒号分隔", "order:123", false},
		{"有效的下划线", "order_123", false},
		{"有效的点分隔", "order.123", false},
		{"有效的斜杠", "order/123", false},
		{"有效的复杂ID", "namespace:order-123_v1.0", false},
		{"空字符串", "", true},
		{"只有空格", "   ", true},
		{"超长ID", string(make([]byte, 257)), true},
		{"包含@符号", "order@123", true},
		{"包含#符号", "order#123", true},
		{"包含空格", "order 123", true},
		{"包含中文", "订单-123", true},
		{"包含特殊字符", "order$123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAggregateID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestKeyedWorkerPool_hashToIndex 测试哈希路由算法
func TestKeyedWorkerPool_hashToIndex(t *testing.T) {
	handler := func(ctx context.Context, message []byte) error { return nil }
	pool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
		WorkerCount: 8,
		QueueSize:   10,
		WaitTimeout: 100 * time.Millisecond,
	}, handler)
	defer pool.Stop()

	tests := []struct {
		name string
		key  string
	}{
		{"简单key", "order-123"},
		{"复杂key", "namespace:order-456_v1"},
		{"数字key", "12345"},
		{"空key", ""},
		{"长key", "very-long-aggregate-id-that-should-still-work-correctly"},
	}

	// 测试一致性：相同的key应该总是路由到相同的worker
	for _, tt := range tests {
		t.Run(tt.name+"_一致性", func(t *testing.T) {
			index1 := pool.hashToIndex(tt.key)
			index2 := pool.hashToIndex(tt.key)
			index3 := pool.hashToIndex(tt.key)

			assert.Equal(t, index1, index2, "相同key的哈希结果应该一致")
			assert.Equal(t, index1, index3, "相同key的哈希结果应该一致")
			assert.GreaterOrEqual(t, index1, 0, "索引应该非负")
			assert.Less(t, index1, 8, "索引应该在范围内")
		})
	}

	// 测试分布性：不同的key应该分布到不同的worker
	keySet := []string{
		"order-1", "order-2", "order-3", "order-4",
		"user-1", "user-2", "user-3", "user-4",
		"product-1", "product-2", "product-3", "product-4",
	}

	indexCounts := make(map[int]int)
	for _, key := range keySet {
		index := pool.hashToIndex(key)
		indexCounts[index]++
	}

	// 验证至少有一定的分布性（不是所有key都路由到同一个worker）
	assert.Greater(t, len(indexCounts), 1, "应该有多个worker被使用")
}

// TestKeyedWorkerPool_runWorker 测试Worker运行逻辑
func TestKeyedWorkerPool_runWorker(t *testing.T) {
	var processedMessages []string
	handler := func(ctx context.Context, message []byte) error {
		processedMessages = append(processedMessages, string(message))
		return nil
	}

	pool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
		WorkerCount: 1, // 只用一个worker便于测试
		QueueSize:   10,
		WaitTimeout: 100 * time.Millisecond,
	}, handler)
	defer pool.Stop()

	// 发送测试消息
	ctx := context.Background()
	messages := []*AggregateMessage{
		{
			AggregateID: "test-1",
			Value:       []byte("message-1"),
			Context:     ctx,
			Done:        make(chan error, 1),
		},
		{
			AggregateID: "test-1",
			Value:       []byte("message-2"),
			Context:     ctx,
			Done:        make(chan error, 1),
		},
		{
			AggregateID: "test-1",
			Value:       []byte("message-3"),
			Context:     ctx,
			Done:        make(chan error, 1),
		},
	}

	// 处理消息
	for _, msg := range messages {
		err := pool.ProcessMessage(ctx, msg)
		require.NoError(t, err)

		// 等待处理完成
		select {
		case err := <-msg.Done:
			assert.NoError(t, err)
		case <-time.After(1 * time.Second):
			t.Fatal("消息处理超时")
		}
	}

	// 验证消息按顺序处理
	assert.Equal(t, []string{"message-1", "message-2", "message-3"}, processedMessages)
}

// TestKeyedWorkerPool_ProcessMessage_ErrorHandling 测试错误处理
func TestKeyedWorkerPool_ProcessMessage_ErrorHandling(t *testing.T) {
	handler := func(ctx context.Context, message []byte) error { return nil }
	pool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
		WorkerCount: 1,
		QueueSize:   1,                     // 小队列便于测试队列满的情况
		WaitTimeout: 10 * time.Millisecond, // 短超时
	}, handler)
	defer pool.Stop()

	ctx := context.Background()

	// 测试缺少AggregateID的情况
	t.Run("缺少AggregateID", func(t *testing.T) {
		msg := &AggregateMessage{
			AggregateID: "", // 空的AggregateID
			Value:       []byte("test"),
			Context:     ctx,
			Done:        make(chan error, 1),
		}

		err := pool.ProcessMessage(ctx, msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "aggregateID required")
	})

	// 测试队列满的情况
	t.Run("队列满", func(t *testing.T) {
		// 创建一个阻塞的handler来模拟队列满的情况
		blockChan := make(chan struct{})
		blockingHandler := func(ctx context.Context, message []byte) error {
			<-blockChan // 阻塞直到收到信号
			return nil
		}

		blockingPool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
			WorkerCount: 1,
			QueueSize:   1,                     // 小队列
			WaitTimeout: 10 * time.Millisecond, // 短超时
		}, blockingHandler)
		defer func() {
			close(blockChan) // 解除阻塞
			blockingPool.Stop()
		}()

		// 先填满队列（这个消息会被worker接收但阻塞在handler中）
		msg1 := &AggregateMessage{
			AggregateID: "test",
			Value:       []byte("blocking-message"),
			Context:     ctx,
			Done:        make(chan error, 1),
		}
		err := blockingPool.ProcessMessage(ctx, msg1)
		assert.NoError(t, err)

		// 再发送一个消息填满队列缓冲区
		msg2 := &AggregateMessage{
			AggregateID: "test",
			Value:       []byte("queue-buffer"),
			Context:     ctx,
			Done:        make(chan error, 1),
		}
		err = blockingPool.ProcessMessage(ctx, msg2)
		assert.NoError(t, err)

		// 第三个消息应该因为队列满而超时
		msg3 := &AggregateMessage{
			AggregateID: "test",
			Value:       []byte("should-timeout"),
			Context:     ctx,
			Done:        make(chan error, 1),
		}
		err = blockingPool.ProcessMessage(ctx, msg3)
		assert.Error(t, err)
		assert.Equal(t, ErrWorkerQueueFull, err)
	})
}
