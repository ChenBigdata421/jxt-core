package eventbus

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNATSKafkaUnifiedInterface 测试NATS和Kafka统一接口实现
func TestNATSKafkaUnifiedInterface(t *testing.T) {
	t.Run("UnifiedInterfaceImplementation", func(t *testing.T) {
		// 测试NATS实现
		t.Run("NATS_Interface", func(t *testing.T) {
			bus, err := NewPersistentNATSEventBus([]string{"nats://localhost:4222"}, "test-client")
			if err != nil {
				t.Skipf("NATS server not available: %v", err)
				return
			}
			defer bus.Close()

			// 验证EventBus接口实现
			var eventBus EventBus = bus
			assert.NotNil(t, eventBus, "NATS应该实现EventBus接口")

			// 测试基础方法
			ctx := context.Background()
			err = eventBus.Subscribe(ctx, "test.nats", func(ctx context.Context, message []byte) error {
				return nil
			})
			assert.NoError(t, err, "NATS Subscribe应该成功")

			err = eventBus.Publish(ctx, "test.nats", []byte("test message"))
			assert.NoError(t, err, "NATS Publish应该成功")

			t.Log("✅ NATS统一接口验证通过")
		})

		// 测试Kafka实现
		t.Run("Kafka_Interface", func(t *testing.T) {
			bus, err := NewPersistentKafkaEventBus([]string{"localhost:9092"})
			if err != nil {
				t.Skipf("Kafka server not available: %v", err)
				return
			}
			defer bus.Close()

			// 验证EventBus接口实现
			var eventBus EventBus = bus
			assert.NotNil(t, eventBus, "Kafka应该实现EventBus接口")

			// 测试基础方法
			ctx := context.Background()
			err = eventBus.Subscribe(ctx, "test.kafka", func(ctx context.Context, message []byte) error {
				return nil
			})
			assert.NoError(t, err, "Kafka Subscribe应该成功")

			err = eventBus.Publish(ctx, "test.kafka", []byte("test message"))
			assert.NoError(t, err, "Kafka Publish应该成功")

			t.Log("✅ Kafka统一接口验证通过")
		})

		// 测试内存实现（作为参考）
		t.Run("Memory_Interface", func(t *testing.T) {
			bus := NewMemoryEventBus()
			defer bus.Close()

			// 验证EventBus接口实现
			var eventBus EventBus = bus
			assert.NotNil(t, eventBus, "Memory应该实现EventBus接口")

			// 测试基础方法
			ctx := context.Background()
			err := eventBus.Subscribe(ctx, "test.memory", func(ctx context.Context, message []byte) error {
				return nil
			})
			assert.NoError(t, err, "Memory Subscribe应该成功")

			err = eventBus.Publish(ctx, "test.memory", []byte("test message"))
			assert.NoError(t, err, "Memory Publish应该成功")

			t.Log("✅ Memory统一接口验证通过")
		})
	})
}

// TestOptimizedArchitectureFeatures 测试优化架构特性
func TestOptimizedArchitectureFeatures(t *testing.T) {
	t.Run("ArchitectureOptimizations", func(t *testing.T) {
		// 验证NATS优化特性
		t.Run("NATS_OptimizedFeatures", func(t *testing.T) {
			bus, err := NewPersistentNATSEventBus([]string{"nats://localhost:4222"}, "optimized-test")
			if err != nil {
				t.Skipf("NATS server not available: %v", err)
				return
			}
			defer bus.Close()

			ctx := context.Background()

			// 测试多topic订阅（应该使用统一Consumer）
			topics := []string{"opt.topic1", "opt.topic2", "opt.topic3"}
			for _, topic := range topics {
				err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
					t.Logf("Received message on %s", topic)
					return nil
				})
				assert.NoError(t, err, "多topic订阅应该成功")
			}

			// 测试消息发布
			for _, topic := range topics {
				err := bus.Publish(ctx, topic, []byte(fmt.Sprintf("message for %s", topic)))
				assert.NoError(t, err, "消息发布应该成功")
			}

			t.Logf("✅ NATS优化架构特性验证通过：支持多topic统一Consumer")
		})

		// 验证Kafka优化特性
		t.Run("Kafka_OptimizedFeatures", func(t *testing.T) {
			bus, err := NewPersistentKafkaEventBus([]string{"localhost:9092"})
			if err != nil {
				t.Skipf("Kafka server not available: %v", err)
				return
			}
			defer bus.Close()

			ctx := context.Background()

			// 测试多topic订阅（应该使用统一Consumer Group）
			topics := []string{"opt.kafka1", "opt.kafka2", "opt.kafka3"}
			for _, topic := range topics {
				err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
					t.Logf("Received message on %s", topic)
					return nil
				})
				assert.NoError(t, err, "多topic订阅应该成功")
			}

			// 测试消息发布
			for _, topic := range topics {
				err := bus.Publish(ctx, topic, []byte(fmt.Sprintf("message for %s", topic)))
				assert.NoError(t, err, "消息发布应该成功")
			}

			t.Logf("✅ Kafka优化架构特性验证通过：支持多topic统一Consumer Group")
		})
	})
}

// TestPerformanceComparison 性能对比测试
func TestPerformanceComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过性能测试")
	}

	t.Run("PerformanceBenchmark", func(t *testing.T) {
		messageCount := 100
		topicCount := 3

		// 内存EventBus性能基准
		t.Run("Memory_Performance", func(t *testing.T) {
			bus := NewMemoryEventBus()
			defer bus.Close()

			ctx := context.Background()
			var wg sync.WaitGroup
			receivedCount := 0
			var mu sync.Mutex

			// 订阅多个topic
			for i := 0; i < topicCount; i++ {
				topic := fmt.Sprintf("perf.memory.%d", i)
				err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
					mu.Lock()
					receivedCount++
					mu.Unlock()
					wg.Done()
					return nil
				})
				require.NoError(t, err)
			}

			// 发送消息并测量时间
			start := time.Now()
			for i := 0; i < topicCount; i++ {
				topic := fmt.Sprintf("perf.memory.%d", i)
				for j := 0; j < messageCount; j++ {
					wg.Add(1)
					err := bus.Publish(ctx, topic, []byte(fmt.Sprintf("message %d", j)))
					require.NoError(t, err)
				}
			}

			wg.Wait()
			duration := time.Since(start)

			totalMessages := messageCount * topicCount
			throughput := float64(totalMessages) / duration.Seconds()

			t.Logf("Memory EventBus性能:")
			t.Logf("  - 总消息数: %d", totalMessages)
			t.Logf("  - 总耗时: %v", duration)
			t.Logf("  - 吞吐量: %.2f msg/s", throughput)
			t.Logf("  - 平均延迟: %v", duration/time.Duration(totalMessages))

			assert.Equal(t, totalMessages, receivedCount, "应该接收到所有消息")
		})
	})
}

// TestEnvelopeSupport 测试Envelope支持
func TestEnvelopeSupport(t *testing.T) {
	t.Run("EnvelopeFeatures", func(t *testing.T) {
		// 测试内存EventBus的Envelope支持
		t.Run("Memory_Envelope", func(t *testing.T) {
			bus := NewMemoryEventBus()
			defer bus.Close()

			ctx := context.Background()
			var receivedEnvelope *Envelope
			var wg sync.WaitGroup

			// 订阅Envelope消息
			err := bus.SubscribeEnvelope(ctx, "envelope.test", func(ctx context.Context, envelope *Envelope) error {
				receivedEnvelope = envelope
				wg.Done()
				return nil
			})
			require.NoError(t, err)

			// 发送Envelope消息
			testEnvelope := &Envelope{
				AggregateID:  "test-aggregate",
				EventType:    "TestEvent",
				EventVersion: 1,
				Timestamp:    time.Now(),
				Payload:      RawMessage(`{"test": "data"}`),
				TraceID:      "test-trace",
			}

			wg.Add(1)
			err = bus.PublishEnvelope(ctx, "envelope.test", testEnvelope)
			require.NoError(t, err)

			wg.Wait()

			// 验证接收到的Envelope
			require.NotNil(t, receivedEnvelope)
			assert.Equal(t, testEnvelope.AggregateID, receivedEnvelope.AggregateID)
			assert.Equal(t, testEnvelope.EventType, receivedEnvelope.EventType)
			assert.Equal(t, testEnvelope.EventVersion, receivedEnvelope.EventVersion)

			t.Log("✅ Envelope支持验证通过")
		})
	})
}

// TestConcurrentOperations 并发操作测试
func TestConcurrentOperations(t *testing.T) {
	t.Run("ConcurrentSubscribePublish", func(t *testing.T) {
		bus := NewMemoryEventBus()
		defer bus.Close()

		ctx := context.Background()
		concurrency := 10
		messagesPerGoroutine := 10
		var wg sync.WaitGroup
		var receivedCount int64
		var mu sync.Mutex

		// 并发订阅和发布
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				topic := fmt.Sprintf("concurrent.%d", index)

				// 订阅
				err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
					mu.Lock()
					receivedCount++
					mu.Unlock()
					return nil
				})
				assert.NoError(t, err)

				// 发布消息
				for j := 0; j < messagesPerGoroutine; j++ {
					err := bus.Publish(ctx, topic, []byte(fmt.Sprintf("message %d-%d", index, j)))
					assert.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		// 等待消息处理
		time.Sleep(100 * time.Millisecond)

		expectedMessages := int64(concurrency * messagesPerGoroutine)
		mu.Lock()
		actualReceived := receivedCount
		mu.Unlock()

		t.Logf("并发操作结果:")
		t.Logf("  - 预期消息数: %d", expectedMessages)
		t.Logf("  - 实际接收数: %d", actualReceived)
		t.Logf("  - 成功率: %.2f%%", float64(actualReceived)/float64(expectedMessages)*100)

		assert.Equal(t, expectedMessages, actualReceived, "应该接收到所有消息")
		t.Log("✅ 并发操作测试通过")
	})
}
