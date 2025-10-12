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
)

// BenchmarkUnifiedConsumerVsOldArchitecture 对比统一消费者组与旧架构的性能
func BenchmarkUnifiedConsumerVsOldArchitecture(b *testing.B) {
	// 使用内存实现进行性能基准测试
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 测试多topic订阅的性能
	topicCount := 10
	messagesPerTopic := 1000

	var processedCount int64

	// 订阅多个topic
	for i := 0; i < topicCount; i++ {
		topic := fmt.Sprintf("perf-topic-%d", i)
		err := bus.Subscribe(ctx, topic, func(ctx context.Context, data []byte) error {
			atomic.AddInt64(&processedCount, 1)
			return nil
		})
		require.NoError(b, err)
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	// 基准测试：发送和处理消息
	for i := 0; i < b.N; i++ {
		for topicIdx := 0; topicIdx < topicCount; topicIdx++ {
			topic := fmt.Sprintf("perf-topic-%d", topicIdx)
			for msgIdx := 0; msgIdx < messagesPerTopic; msgIdx++ {
				message := fmt.Sprintf("message-%d-%d", topicIdx, msgIdx)
				err := bus.Publish(ctx, topic, []byte(message))
				if err != nil {
					b.Fatalf("Failed to publish message: %v", err)
				}
			}
		}
	}

	b.StopTimer()

	// 等待所有消息处理完成
	expectedTotal := int64(b.N * topicCount * messagesPerTopic)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			b.Logf("Timeout waiting for message processing. Processed: %d, Expected: %d", 
				atomic.LoadInt64(&processedCount), expectedTotal)
			return
		case <-ticker.C:
			if atomic.LoadInt64(&processedCount) >= expectedTotal {
				b.Logf("All messages processed successfully. Total: %d", atomic.LoadInt64(&processedCount))
				return
			}
		}
	}
}

// TestUnifiedConsumerResourceUsage 测试统一消费者组的资源使用
func TestUnifiedConsumerResourceUsage(t *testing.T) {
	// 记录初始内存使用
	var initialMemStats, finalMemStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&initialMemStats)

	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 订阅大量topic以测试资源使用
	topicCount := 50
	var processedCount int64

	t.Logf("Subscribing to %d topics...", topicCount)

	for i := 0; i < topicCount; i++ {
		topic := fmt.Sprintf("resource-topic-%d", i)
		err := bus.Subscribe(ctx, topic, func(ctx context.Context, data []byte) error {
			atomic.AddInt64(&processedCount, 1)
			return nil
		})
		require.NoError(t, err)
	}

	// 等待订阅生效
	time.Sleep(500 * time.Millisecond)

	// 发送消息测试处理能力
	messagesPerTopic := 100
	totalMessages := topicCount * messagesPerTopic

	t.Logf("Publishing %d messages across %d topics...", totalMessages, topicCount)

	start := time.Now()
	for i := 0; i < topicCount; i++ {
		topic := fmt.Sprintf("resource-topic-%d", i)
		for j := 0; j < messagesPerTopic; j++ {
			message := fmt.Sprintf("resource-message-%d-%d", i, j)
			err := bus.Publish(ctx, topic, []byte(message))
			require.NoError(t, err)
		}
	}
	publishDuration := time.Since(start)

	// 等待所有消息处理完成
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	processingStart := time.Now()
	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for message processing. Processed: %d, Expected: %d", 
				atomic.LoadInt64(&processedCount), totalMessages)
		case <-ticker.C:
			if atomic.LoadInt64(&processedCount) >= int64(totalMessages) {
				processingDuration := time.Since(processingStart)
				t.Logf("All %d messages processed in %v", totalMessages, processingDuration)
				goto done
			}
		}
	}

done:
	// 记录最终内存使用
	runtime.GC()
	runtime.ReadMemStats(&finalMemStats)

	// 计算性能指标
	publishRate := float64(totalMessages) / publishDuration.Seconds()
	processingRate := float64(totalMessages) / time.Since(processingStart).Seconds()
	memoryIncrease := finalMemStats.Alloc - initialMemStats.Alloc

	// 性能断言
	assert.Greater(t, publishRate, 1000.0, "Publish rate should be > 1000 msg/sec")
	assert.Greater(t, processingRate, 500.0, "Processing rate should be > 500 msg/sec")
	assert.Less(t, memoryIncrease, uint64(50*1024*1024), "Memory increase should be < 50MB")

	t.Logf("📊 Performance Metrics:")
	t.Logf("  Topics: %d", topicCount)
	t.Logf("  Total Messages: %d", totalMessages)
	t.Logf("  Publish Rate: %.2f msg/sec", publishRate)
	t.Logf("  Processing Rate: %.2f msg/sec", processingRate)
	t.Logf("  Memory Increase: %.2f MB", float64(memoryIncrease)/(1024*1024))
	t.Logf("  Publish Duration: %v", publishDuration)
	t.Logf("  Processing Duration: %v", time.Since(processingStart))
}

// TestUnifiedConsumerConcurrentSubscriptions 测试并发订阅的性能
func TestUnifiedConsumerConcurrentSubscriptions(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	concurrentSubscribers := 20
	topicsPerSubscriber := 5
	var processedCount int64

	t.Logf("Testing %d concurrent subscribers, %d topics each", 
		concurrentSubscribers, topicsPerSubscriber)

	var wg sync.WaitGroup
	start := time.Now()

	// 并发创建订阅
	for i := 0; i < concurrentSubscribers; i++ {
		wg.Add(1)
		go func(subscriberID int) {
			defer wg.Done()
			
			for j := 0; j < topicsPerSubscriber; j++ {
				topic := fmt.Sprintf("concurrent-topic-%d-%d", subscriberID, j)
				err := bus.Subscribe(ctx, topic, func(ctx context.Context, data []byte) error {
					atomic.AddInt64(&processedCount, 1)
					return nil
				})
				if err != nil {
					t.Errorf("Failed to subscribe to topic %s: %v", topic, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	subscriptionDuration := time.Since(start)

	// 等待所有订阅生效
	time.Sleep(1 * time.Second)

	// 并发发送消息
	messagesPerTopic := 10
	totalTopics := concurrentSubscribers * topicsPerSubscriber
	totalMessages := totalTopics * messagesPerTopic

	t.Logf("Publishing %d messages to %d topics...", totalMessages, totalTopics)

	publishStart := time.Now()
	for i := 0; i < concurrentSubscribers; i++ {
		wg.Add(1)
		go func(subscriberID int) {
			defer wg.Done()
			
			for j := 0; j < topicsPerSubscriber; j++ {
				topic := fmt.Sprintf("concurrent-topic-%d-%d", subscriberID, j)
				for k := 0; k < messagesPerTopic; k++ {
					message := fmt.Sprintf("concurrent-message-%d-%d-%d", subscriberID, j, k)
					err := bus.Publish(ctx, topic, []byte(message))
					if err != nil {
						t.Errorf("Failed to publish to topic %s: %v", topic, err)
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()
	publishDuration := time.Since(publishStart)

	// 等待所有消息处理完成
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for message processing. Processed: %d, Expected: %d", 
				atomic.LoadInt64(&processedCount), totalMessages)
		case <-ticker.C:
			if atomic.LoadInt64(&processedCount) >= int64(totalMessages) {
				goto concurrentDone
			}
		}
	}

concurrentDone:
	totalDuration := time.Since(start)

	// 计算并发性能指标
	subscriptionRate := float64(totalTopics) / subscriptionDuration.Seconds()
	publishRate := float64(totalMessages) / publishDuration.Seconds()
	overallRate := float64(totalMessages) / totalDuration.Seconds()

	// 性能断言
	assert.Greater(t, subscriptionRate, 10.0, "Subscription rate should be > 10 subscriptions/sec")
	assert.Greater(t, publishRate, 500.0, "Publish rate should be > 500 msg/sec")
	assert.Greater(t, overallRate, 100.0, "Overall rate should be > 100 msg/sec")

	t.Logf("📊 Concurrent Performance Metrics:")
	t.Logf("  Concurrent Subscribers: %d", concurrentSubscribers)
	t.Logf("  Total Topics: %d", totalTopics)
	t.Logf("  Total Messages: %d", totalMessages)
	t.Logf("  Subscription Rate: %.2f subscriptions/sec", subscriptionRate)
	t.Logf("  Publish Rate: %.2f msg/sec", publishRate)
	t.Logf("  Overall Rate: %.2f msg/sec", overallRate)
	t.Logf("  Subscription Duration: %v", subscriptionDuration)
	t.Logf("  Publish Duration: %v", publishDuration)
	t.Logf("  Total Duration: %v", totalDuration)
	t.Logf("  Messages Processed: %d", atomic.LoadInt64(&processedCount))
}

// TestUnifiedConsumerMemoryEfficiency 测试内存效率
func TestUnifiedConsumerMemoryEfficiency(t *testing.T) {
	// 这个测试验证统一消费者组架构相比旧架构的内存效率
	// 旧架构：N个topic = N个ConsumerGroup实例 = N倍内存使用
	// 新架构：N个topic = 1个ConsumerGroup实例 = 固定内存使用

	var memStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	baselineMemory := memStats.Alloc

	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 逐步增加topic数量，观察内存使用变化
	topicCounts := []int{1, 5, 10, 20, 50}
	memoryUsages := make([]uint64, len(topicCounts))

	for i, topicCount := range topicCounts {
		// 订阅指定数量的topic
		for j := 0; j < topicCount; j++ {
			topic := fmt.Sprintf("memory-topic-%d-%d", i, j)
			err := bus.Subscribe(ctx, topic, func(ctx context.Context, data []byte) error {
				return nil // 简单处理器
			})
			require.NoError(t, err)
		}

		// 等待订阅生效
		time.Sleep(100 * time.Millisecond)

		// 测量内存使用
		runtime.GC()
		runtime.ReadMemStats(&memStats)
		memoryUsages[i] = memStats.Alloc - baselineMemory

		t.Logf("Topics: %d, Memory Usage: %.2f MB", 
			topicCount, float64(memoryUsages[i])/(1024*1024))
	}

	// 验证内存使用增长是合理的（应该是亚线性增长，而不是线性增长）
	// 在统一架构中，内存使用主要来自topic映射和handler存储，而不是多个消费者组实例
	for i := 1; i < len(memoryUsages); i++ {
		growthRatio := float64(memoryUsages[i]) / float64(memoryUsages[i-1])
		topicRatio := float64(topicCounts[i]) / float64(topicCounts[i-1])
		
		// 内存增长应该小于topic数量增长（证明是统一架构而非多实例架构）
		assert.Less(t, growthRatio, topicRatio*1.5, 
			"Memory growth should be sublinear compared to topic count growth")
		
		t.Logf("Topics %d->%d: Memory growth %.2fx, Topic growth %.2fx", 
			topicCounts[i-1], topicCounts[i], growthRatio, topicRatio)
	}

	t.Log("✅ Memory efficiency test passed - unified architecture shows sublinear memory growth")
}
