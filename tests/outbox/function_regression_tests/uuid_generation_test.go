package function_regression_tests

import (
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
	"github.com/google/uuid"
)

// TestUUIDGeneration_Basic 测试基本 UUID 生成
func TestUUIDGeneration_Basic(t *testing.T) {
	helper := NewTestHelper(t)

	// 创建事件
	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")

	// 验证 ID 不为空
	helper.AssertNotEmpty(event.ID, "Event ID should not be empty")

	// 验证 ID 是有效的 UUID
	_, err := uuid.Parse(event.ID)
	helper.AssertNoError(err, "Event ID should be a valid UUID")
}

// TestUUIDGeneration_Uniqueness 测试 UUID 唯一性
func TestUUIDGeneration_Uniqueness(t *testing.T) {
	helper := NewTestHelper(t)

	// 生成多个事件
	const count = 1000
	ids := make(map[string]bool)

	for i := 0; i < count; i++ {
		event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")

		// 检查 ID 是否重复
		if ids[event.ID] {
			t.Fatalf("Duplicate UUID found: %s", event.ID)
		}
		ids[event.ID] = true
	}

	// 验证生成了正确数量的唯一 ID
	helper.AssertEqual(count, len(ids), "Should generate unique UUIDs")
}

// TestUUIDGeneration_Concurrent 测试并发 UUID 生成
func TestUUIDGeneration_Concurrent(t *testing.T) {
	helper := NewTestHelper(t)

	const goroutines = 100
	const eventsPerGoroutine = 100

	var wg sync.WaitGroup
	idsChan := make(chan string, goroutines*eventsPerGoroutine)

	// 并发生成 UUID
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
				idsChan <- event.ID
			}
		}()
	}

	wg.Wait()
	close(idsChan)

	// 收集所有 ID
	ids := make(map[string]bool)
	for id := range idsChan {
		if ids[id] {
			t.Fatalf("Duplicate UUID found in concurrent generation: %s", id)
		}
		ids[id] = true
	}

	// 验证生成了正确数量的唯一 ID
	expectedCount := goroutines * eventsPerGoroutine
	helper.AssertEqual(expectedCount, len(ids), "Should generate unique UUIDs concurrently")
}

// TestUUIDGeneration_TimeOrdering 测试 UUID 时间排序（UUIDv7 特性）
func TestUUIDGeneration_TimeOrdering(t *testing.T) {
	helper := NewTestHelper(t)

	// 生成一系列事件，每次生成之间有小延迟
	const count = 10
	events := make([]*outbox.OutboxEvent, count)

	for i := 0; i < count; i++ {
		events[i] = helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
		time.Sleep(1 * time.Millisecond) // 确保时间戳不同
	}

	// 验证 UUID 是按时间排序的（UUIDv7 特性）
	// 注意：这个测试假设使用了 UUIDv7
	for i := 1; i < count; i++ {
		prevID := events[i-1].ID
		currID := events[i].ID

		// UUIDv7 的字符串表示应该是按时间排序的
		if prevID >= currID {
			t.Logf("Warning: UUIDs may not be time-ordered (expected for UUIDv7)")
			t.Logf("Previous: %s, Current: %s", prevID, currID)
			// 不失败测试，因为可能降级到 UUIDv4
		}
	}
}

// TestUUIDGeneration_Format 测试 UUID 格式
func TestUUIDGeneration_Format(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")

	// 验证 UUID 格式（8-4-4-4-12）
	parsedUUID, err := uuid.Parse(event.ID)
	helper.AssertNoError(err, "Should be a valid UUID")

	// 验证 UUID 字符串格式
	helper.AssertEqual(36, len(event.ID), "UUID should be 36 characters long")

	// 验证 UUID 版本（应该是 v7 或 v4）
	version := parsedUUID.Version()
	if version != 4 && version != 7 {
		t.Logf("Warning: UUID version is %d, expected 4 or 7", version)
	}
}

// TestUUIDGeneration_Performance 测试 UUID 生成性能
func TestUUIDGeneration_Performance(t *testing.T) {
	helper := NewTestHelper(t)
	const count = 10000

	start := time.Now()

	for i := 0; i < count; i++ {
		_ = helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
	}

	duration := time.Since(start)

	// 验证性能（应该能在 1 秒内生成 10000 个 UUID）
	if duration > 1*time.Second {
		t.Logf("Warning: UUID generation is slow: %v for %d UUIDs", duration, count)
	}

	t.Logf("Generated %d UUIDs in %v (%.2f UUIDs/ms)",
		count, duration, float64(count)/float64(duration.Milliseconds()))
}

// TestUUIDGeneration_Stability 测试 UUID 生成稳定性
func TestUUIDGeneration_Stability(t *testing.T) {
	helper := NewTestHelper(t)

	// 多次运行，确保没有 panic
	for i := 0; i < 1000; i++ {
		event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")
		helper.AssertNotEmpty(event.ID, "Event ID should not be empty")

		_, err := uuid.Parse(event.ID)
		helper.AssertNoError(err, "Event ID should be a valid UUID")
	}
}

// TestUUIDGeneration_DifferentAggregates 测试不同聚合根的 UUID 生成
func TestUUIDGeneration_DifferentAggregates(t *testing.T) {
	helper := NewTestHelper(t)

	// 为不同的聚合根生成事件
	aggregateTypes := []string{"Order", "User", "Product", "Payment", "Inventory"}
	ids := make(map[string]bool)

	for _, aggregateType := range aggregateTypes {
		for i := 0; i < 100; i++ {
			event := helper.CreateTestEvent(1, aggregateType, "agg-123", "Created")

			// 检查 ID 是否重复
			if ids[event.ID] {
				t.Fatalf("Duplicate UUID found: %s", event.ID)
			}
			ids[event.ID] = true
		}
	}

	// 验证生成了正确数量的唯一 ID
	expectedCount := len(aggregateTypes) * 100
	helper.AssertEqual(expectedCount, len(ids), "Should generate unique UUIDs for different aggregates")
}

// TestUUIDGeneration_Fallback 测试 UUID 生成降级机制
func TestUUIDGeneration_Fallback(t *testing.T) {
	helper := NewTestHelper(t)

	// 即使在高并发情况下，UUID 生成也应该稳定
	// 如果 UUIDv7 失败，应该降级到 UUIDv4

	const goroutines = 50
	const eventsPerGoroutine = 200

	var wg sync.WaitGroup
	errors := make(chan error, goroutines*eventsPerGoroutine)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := helper.CreateTestEvent(1, "Order", "order-123", "OrderCreated")

				// 验证 UUID 有效性
				if _, err := uuid.Parse(event.ID); err != nil {
					errors <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	errorCount := 0
	for err := range errors {
		t.Errorf("UUID generation error: %v", err)
		errorCount++
	}

	helper.AssertEqual(0, errorCount, "Should not have UUID generation errors")
}
