package performance_tests

import (
	"fmt"
	"hash/fnv"
	"sync"
	"testing"
)

// TestOrderCheckerConcurrency 测试 OrderChecker 的并发安全性
func TestOrderCheckerConcurrency(t *testing.T) {
	orderChecker := NewOrderChecker()

	// 模拟 Keyed-Worker Pool 的行为
	// 256 个 workers，每个 worker 串行处理消息
	workerCount := 256
	messagesPerWorker := 100

	var wg sync.WaitGroup

	// 为每个 worker 创建一个 goroutine
	for workerID := 0; workerID < workerCount; workerID++ {
		wg.Add(1)
		go func(wID int) {
			defer wg.Done()

			// 每个 worker 串行处理消息
			for i := 0; i < messagesPerWorker; i++ {
				aggregateID := fmt.Sprintf("aggregate-%d", wID%10) // 10 个聚合ID
				version := int64(i + 1)

				// 检查顺序
				orderChecker.Check(aggregateID, version)
			}
		}(workerID)
	}

	wg.Wait()

	violations := orderChecker.GetViolations()
	t.Logf("顺序违反次数: %d", violations)

	if violations > 0 {
		t.Errorf("❌ OrderChecker 有并发问题！顺序违反: %d", violations)
	} else {
		t.Logf("✅ OrderChecker 并发安全")
	}
}

// TestOrderCheckerSequential 测试 OrderChecker 的串行正确性
func TestOrderCheckerSequential(t *testing.T) {
	orderChecker := NewOrderChecker()

	aggregateID := "test-aggregate"

	// 按顺序检查
	for i := int64(1); i <= 100; i++ {
		result := orderChecker.Check(aggregateID, i)
		if !result {
			t.Errorf("❌ 顺序检查失败: version %d", i)
		}
	}

	violations := orderChecker.GetViolations()
	if violations > 0 {
		t.Errorf("❌ 串行测试有顺序违反: %d", violations)
	} else {
		t.Logf("✅ 串行测试通过")
	}
}

// TestOrderCheckerOutOfOrder 测试 OrderChecker 能否检测乱序
func TestOrderCheckerOutOfOrder(t *testing.T) {
	orderChecker := NewOrderChecker()

	aggregateID := "test-aggregate"

	// 先发送 version 1
	orderChecker.Check(aggregateID, 1)

	// 再发送 version 3（跳过 2）
	orderChecker.Check(aggregateID, 3)

	// 再发送 version 2（乱序）
	result := orderChecker.Check(aggregateID, 2)

	if result {
		t.Errorf("❌ 应该检测到乱序，但没有检测到")
	}

	violations := orderChecker.GetViolations()
	if violations != 1 {
		t.Errorf("❌ 应该有 1 次顺序违反，实际: %d", violations)
	} else {
		t.Logf("✅ 正确检测到乱序")
	}
}

// TestKeyedWorkerPoolSimulation 模拟 Keyed-Worker Pool 的真实行为
func TestKeyedWorkerPoolSimulation(t *testing.T) {
	orderChecker := NewOrderChecker()

	// 模拟真实场景：
	// - 10 个聚合ID
	// - 每个聚合ID 发送 100 条消息
	// - 使用 Keyed-Worker Pool 路由（256 workers）

	aggregateCount := 10
	messagesPerAggregate := 100
	workerCount := 256

	// 为每个聚合ID创建消息队列
	type Message struct {
		AggregateID string
		Version     int64
	}

	// 生成所有消息
	allMessages := make([]Message, 0, aggregateCount*messagesPerAggregate)
	for aggID := 0; aggID < aggregateCount; aggID++ {
		for version := 1; version <= messagesPerAggregate; version++ {
			allMessages = append(allMessages, Message{
				AggregateID: fmt.Sprintf("aggregate-%d", aggID),
				Version:     int64(version),
			})
		}
	}

	// 模拟 Keyed-Worker Pool 路由
	workerQueues := make([][]Message, workerCount)
	for i := 0; i < workerCount; i++ {
		workerQueues[i] = make([]Message, 0)
	}

	// 使用相同的 hash 算法路由消息
	for _, msg := range allMessages {
		workerIndex := hashToWorkerIndex(msg.AggregateID, workerCount)
		workerQueues[workerIndex] = append(workerQueues[workerIndex], msg)
	}

	// 每个 worker 串行处理自己的队列
	var wg sync.WaitGroup
	for workerID := 0; workerID < workerCount; workerID++ {
		wg.Add(1)
		go func(wID int, queue []Message) {
			defer wg.Done()

			// 串行处理
			for _, msg := range queue {
				orderChecker.Check(msg.AggregateID, msg.Version)
			}
		}(workerID, workerQueues[workerID])
	}

	wg.Wait()

	violations := orderChecker.GetViolations()
	t.Logf("总消息数: %d", len(allMessages))
	t.Logf("顺序违反次数: %d", violations)

	if violations > 0 {
		t.Errorf("❌ Keyed-Worker Pool 模拟测试失败！顺序违反: %d", violations)

		// 打印每个聚合ID的消息分布
		for aggID := 0; aggID < aggregateCount; aggID++ {
			aggregateID := fmt.Sprintf("aggregate-%d", aggID)
			workerIndex := hashToWorkerIndex(aggregateID, workerCount)
			t.Logf("  %s -> Worker %d (队列长度: %d)", aggregateID, workerIndex, len(workerQueues[workerIndex]))
		}
	} else {
		t.Logf("✅ Keyed-Worker Pool 模拟测试通过")
	}
}

// hashToWorkerIndex 使用与 Keyed-Worker Pool 相同的 hash 算法
func hashToWorkerIndex(key string, workerCount int) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() % uint32(workerCount))
}
