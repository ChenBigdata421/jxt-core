package eventbus

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestUnifiedWorkerPool_WithAggregateID 测试有聚合ID的消息（保证顺序）
func TestUnifiedWorkerPool_WithAggregateID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	pool := NewUnifiedWorkerPool(16, logger) // 16个Worker
	defer pool.Close()

	// 记录每个聚合ID的消息处理顺序
	orderMu := sync.Mutex{}
	processOrder := make(map[string][]int)

	// 创建handler
	handler := func(ctx context.Context, data []byte) error {
		msg := string(data)
		var aggregateID string
		var seqNum int
		fmt.Sscanf(msg, "%s:%d", &aggregateID, &seqNum)

		orderMu.Lock()
		processOrder[aggregateID] = append(processOrder[aggregateID], seqNum)
		orderMu.Unlock()

		time.Sleep(1 * time.Millisecond) // 模拟处理时间
		return nil
	}

	// 发送100条消息，分布在10个聚合ID上
	totalMessages := 100
	aggregateCount := 10
	messagesPerAggregate := totalMessages / aggregateCount

	for i := 0; i < totalMessages; i++ {
		aggregateID := fmt.Sprintf("order-%d", i%aggregateCount)
		seqNum := i / aggregateCount

		workItem := UnifiedWorkItem{
			Topic:       "test-topic",
			AggregateID: aggregateID,
			Data:        []byte(fmt.Sprintf("%s:%d", aggregateID, seqNum)),
			Handler:     handler,
			Context:     context.Background(),
		}

		require.True(t, pool.SubmitWork(workItem))
	}

	// 等待所有消息处理完成
	time.Sleep(2 * time.Second)

	// 验证每个聚合ID的消息都是顺序处理的
	orderMu.Lock()
	defer orderMu.Unlock()

	for aggregateID, order := range processOrder {
		assert.Equal(t, messagesPerAggregate, len(order), "聚合ID %s 的消息数量不正确", aggregateID)

		// 验证顺序：应该是 0, 1, 2, 3, ..., 9
		for i, seqNum := range order {
			assert.Equal(t, i, seqNum, "聚合ID %s 的消息顺序错误：期望 %d，实际 %d", aggregateID, i, seqNum)
		}
	}

	t.Logf("✅ 所有聚合ID的消息都按顺序处理")
}

// TestUnifiedWorkerPool_WithoutAggregateID 测试无聚合ID的消息（高并发）
func TestUnifiedWorkerPool_WithoutAggregateID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	pool := NewUnifiedWorkerPool(16, logger) // 16个Worker
	defer pool.Close()

	// 记录处理的消息数量
	var processedCount atomic.Int64
	var workerUsage sync.Map // 记录哪些Worker被使用了

	// 创建handler
	handler := func(ctx context.Context, data []byte) error {
		processedCount.Add(1)

		// 记录当前goroutine ID（模拟Worker ID）
		goroutineID := getGoroutineID()
		workerUsage.Store(goroutineID, true)

		time.Sleep(10 * time.Millisecond) // 模拟处理时间
		return nil
	}

	// 发送100条无聚合ID的消息
	totalMessages := 100

	for i := 0; i < totalMessages; i++ {
		workItem := UnifiedWorkItem{
			Topic:       "test-topic",
			AggregateID: "", // 无聚合ID
			Data:        []byte(fmt.Sprintf("message-%d", i)),
			Handler:     handler,
			Context:     context.Background(),
		}

		require.True(t, pool.SubmitWork(workItem))
	}

	// 等待所有消息处理完成
	time.Sleep(3 * time.Second)

	// 验证所有消息都被处理
	assert.Equal(t, int64(totalMessages), processedCount.Load(), "处理的消息数量不正确")

	// 验证多个Worker被使用（说明是并发处理）
	workerCount := 0
	workerUsage.Range(func(key, value interface{}) bool {
		workerCount++
		return true
	})

	assert.Greater(t, workerCount, 1, "应该有多个Worker被使用")
	t.Logf("✅ 使用了 %d 个Worker进行并发处理", workerCount)
}

// TestUnifiedWorkerPool_Mixed 测试混合场景（有聚合ID + 无聚合ID）
func TestUnifiedWorkerPool_Mixed(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	pool := NewUnifiedWorkerPool(16, logger) // 16个Worker
	defer pool.Close()

	// 记录处理的消息数量
	var withAggregateCount atomic.Int64
	var withoutAggregateCount atomic.Int64

	// 记录每个聚合ID的消息处理顺序
	orderMu := sync.Mutex{}
	processOrder := make(map[string][]int)

	// 创建handler
	handler := func(ctx context.Context, data []byte) error {
		msg := string(data)

		if len(msg) > 6 && msg[:6] == "order-" {
			// 有聚合ID的消息
			var aggregateID string
			var seqNum int
			fmt.Sscanf(msg, "%[^:]:%d", &aggregateID, &seqNum)

			orderMu.Lock()
			processOrder[aggregateID] = append(processOrder[aggregateID], seqNum)
			orderMu.Unlock()

			withAggregateCount.Add(1)
		} else {
			// 无聚合ID的消息
			withoutAggregateCount.Add(1)
		}

		time.Sleep(1 * time.Millisecond) // 模拟处理时间
		return nil
	}

	// 发送50条有聚合ID的消息（5个聚合ID，每个10条）
	for i := 0; i < 50; i++ {
		aggregateID := fmt.Sprintf("order-%d", i%5)
		seqNum := i / 5

		workItem := UnifiedWorkItem{
			Topic:       "test-topic",
			AggregateID: aggregateID,
			Data:        []byte(fmt.Sprintf("%s:%d", aggregateID, seqNum)),
			Handler:     handler,
			Context:     context.Background(),
		}

		require.True(t, pool.SubmitWork(workItem))
	}

	// 发送50条无聚合ID的消息
	for i := 0; i < 50; i++ {
		workItem := UnifiedWorkItem{
			Topic:       "test-topic",
			AggregateID: "", // 无聚合ID
			Data:        []byte(fmt.Sprintf("notification-%d", i)),
			Handler:     handler,
			Context:     context.Background(),
		}

		require.True(t, pool.SubmitWork(workItem))
	}

	// 等待所有消息处理完成
	time.Sleep(2 * time.Second)

	// 验证消息数量
	assert.Equal(t, int64(50), withAggregateCount.Load(), "有聚合ID的消息数量不正确")
	assert.Equal(t, int64(50), withoutAggregateCount.Load(), "无聚合ID的消息数量不正确")

	// 验证有聚合ID的消息顺序
	orderMu.Lock()
	defer orderMu.Unlock()

	for aggregateID, order := range processOrder {
		assert.Equal(t, 10, len(order), "聚合ID %s 的消息数量不正确", aggregateID)

		// 验证顺序
		for i, seqNum := range order {
			assert.Equal(t, i, seqNum, "聚合ID %s 的消息顺序错误", aggregateID)
		}
	}

	t.Logf("✅ 混合场景测试通过：有聚合ID消息保证顺序，无聚合ID消息高并发处理")
}

// TestUnifiedWorkerPool_GoroutineCount 测试Goroutine数量控制
func TestUnifiedWorkerPool_GoroutineCount(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 记录初始Goroutine数量
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// 创建Worker池
	workerCount := 32
	pool := NewUnifiedWorkerPool(workerCount, logger)

	// 等待Worker池启动
	time.Sleep(100 * time.Millisecond)

	// 记录启动后的Goroutine数量
	afterStartGoroutines := runtime.NumGoroutine()
	goroutineDelta := afterStartGoroutines - initialGoroutines

	t.Logf("初始Goroutine数量: %d", initialGoroutines)
	t.Logf("启动后Goroutine数量: %d", afterStartGoroutines)
	t.Logf("增加的Goroutine数量: %d", goroutineDelta)

	// 验证Goroutine数量增长合理
	// 应该是：workerCount个Worker + 1个dispatcher = workerCount + 1
	expectedDelta := workerCount + 1
	tolerance := 5 // 允许5个Goroutine的误差

	assert.InDelta(t, expectedDelta, goroutineDelta, float64(tolerance),
		"Goroutine数量增长不符合预期：期望 %d±%d，实际 %d", expectedDelta, tolerance, goroutineDelta)

	// 关闭Worker池
	pool.Close()

	// 等待Goroutine清理
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// 记录关闭后的Goroutine数量
	afterCloseGoroutines := runtime.NumGoroutine()

	t.Logf("关闭后Goroutine数量: %d", afterCloseGoroutines)

	// 验证Goroutine被正确清理
	assert.InDelta(t, initialGoroutines, afterCloseGoroutines, float64(tolerance),
		"Goroutine未被正确清理")

	t.Logf("✅ Goroutine数量控制正常")
}

// getGoroutineID 获取当前goroutine ID（用于测试）
func getGoroutineID() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	var id int
	fmt.Sscanf(string(buf[:n]), "goroutine %d ", &id)
	return id
}

