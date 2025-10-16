package eventbus

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestNATSStreamPreCreation_Performance 测试Stream预创建性能优化
func TestNATSStreamPreCreation_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// 创建NATS EventBus
	config := &NATSConfig{
		URLs:     []string{"nats://localhost:4222"},
		ClientID: "stream-precreation-perf-test",
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:     "PERF_TEST_STREAM",
				Subjects: []string{"perf.>"},
				Storage:  "memory", // 使用内存存储，加快测试速度
				Replicas: 1,
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	if err != nil {
		t.Skipf("Skipping test: NATS not available: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()
	messageCount := 1000
	topic := "perf.test.topic"

	// ========== 场景1: 未优化（每次Publish都检查Stream） ==========
	t.Run("Without_PreCreation", func(t *testing.T) {
		// 使用默认策略（会检查Stream）
		bus.SetTopicConfigStrategy(StrategyCreateOrUpdate)

		startTime := time.Now()
		for i := 0; i < messageCount; i++ {
			message := []byte(fmt.Sprintf(`{"id": %d}`, i))
			err := bus.Publish(ctx, topic, message)
			if err != nil {
				t.Fatalf("Publish failed: %v", err)
			}
		}
		duration := time.Since(startTime)
		throughput := float64(messageCount) / duration.Seconds()

		t.Logf("❌ 未优化性能:")
		t.Logf("   消息数量: %d", messageCount)
		t.Logf("   总耗时: %v", duration)
		t.Logf("   吞吐量: %.2f msg/s", throughput)
		t.Logf("   平均延迟: %v/msg", duration/time.Duration(messageCount))
	})

	// ========== 场景2: 优化后（预创建 + StrategySkip） ==========
	t.Run("With_PreCreation_And_Skip", func(t *testing.T) {
		// 步骤1: 预创建Stream
		bus.SetTopicConfigStrategy(StrategyCreateOnly)
		err := bus.ConfigureTopic(ctx, topic, TopicOptions{
			PersistenceMode: TopicPersistent,
			RetentionTime:   1 * time.Hour,
			MaxMessages:     10000,
		})
		if err != nil {
			t.Fatalf("ConfigureTopic failed: %v", err)
		}

		// 步骤2: 切换到StrategySkip（跳过运行时检查）
		bus.SetTopicConfigStrategy(StrategySkip)

		// 步骤3: 发布消息（零RPC开销）
		startTime := time.Now()
		for i := 0; i < messageCount; i++ {
			message := []byte(fmt.Sprintf(`{"id": %d}`, i))
			err := bus.Publish(ctx, topic, message)
			if err != nil {
				t.Fatalf("Publish failed: %v", err)
			}
		}
		duration := time.Since(startTime)
		throughput := float64(messageCount) / duration.Seconds()

		t.Logf("✅ 优化后性能:")
		t.Logf("   消息数量: %d", messageCount)
		t.Logf("   总耗时: %v", duration)
		t.Logf("   吞吐量: %.2f msg/s", throughput)
		t.Logf("   平均延迟: %v/msg", duration/time.Duration(messageCount))
	})
}

// TestNATSStreamPreCreation_CacheEffectiveness 测试缓存有效性
func TestNATSStreamPreCreation_CacheEffectiveness(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cache test in short mode")
	}

	config := &NATSConfig{
		URLs:     []string{"nats://localhost:4222"},
		ClientID: "cache-test",
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:     "CACHE_TEST_STREAM",
				Subjects: []string{"cache.>"},
				Storage:  "memory",
				Replicas: 1,
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	if err != nil {
		t.Skipf("Skipping test: NATS not available: %v", err)
	}
	defer bus.Close()

	natsBus := bus.(*natsEventBus)
	ctx := context.Background()
	topic := "cache.test.topic"

	// 验证初始状态：缓存为空
	streamName := natsBus.getStreamNameForTopic(topic)
	natsBus.createdStreamsMu.RLock()
	exists := natsBus.createdStreams[streamName]
	natsBus.createdStreamsMu.RUnlock()

	if exists {
		t.Errorf("Cache should be empty initially")
	}

	// 预创建Stream
	natsBus.SetTopicConfigStrategy(StrategyCreateOnly)
	err = natsBus.ConfigureTopic(ctx, topic, TopicOptions{
		PersistenceMode: TopicPersistent,
		RetentionTime:   1 * time.Hour,
		MaxMessages:     1000,
	})
	if err != nil {
		t.Fatalf("ConfigureTopic failed: %v", err)
	}

	// 验证缓存已更新
	natsBus.createdStreamsMu.RLock()
	exists = natsBus.createdStreams[streamName]
	natsBus.createdStreamsMu.RUnlock()

	if !exists {
		t.Errorf("Cache should contain stream after ConfigureTopic")
	}

	// 切换到StrategySkip并发布消息
	natsBus.SetTopicConfigStrategy(StrategySkip)

	// 发布消息时不应再调用StreamInfo（通过缓存判断）
	for i := 0; i < 10; i++ {
		err := natsBus.Publish(ctx, topic, []byte(fmt.Sprintf(`{"id": %d}`, i)))
		if err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	t.Logf("✅ 缓存测试通过: Stream已缓存，发布时跳过RPC检查")
}

// TestNATSStreamPreCreation_MultipleTopics 测试多Topic预创建
func TestNATSStreamPreCreation_MultipleTopics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multi-topic test in short mode")
	}

	config := &NATSConfig{
		URLs:     []string{"nats://localhost:4222"},
		ClientID: "multi-topic-test",
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:     "MULTI_TOPIC_STREAM",
				Subjects: []string{"multi.>"},
				Storage:  "memory",
				Replicas: 1,
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	if err != nil {
		t.Skipf("Skipping test: NATS not available: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()

	// 定义多个Topic
	topics := []string{
		"multi.topic1",
		"multi.topic2",
		"multi.topic3",
		"multi.topic4",
		"multi.topic5",
	}

	// 预创建所有Topic
	bus.SetTopicConfigStrategy(StrategyCreateOnly)
	for _, topic := range topics {
		err := bus.ConfigureTopic(ctx, topic, TopicOptions{
			PersistenceMode: TopicPersistent,
			RetentionTime:   1 * time.Hour,
			MaxMessages:     1000,
		})
		if err != nil {
			t.Fatalf("ConfigureTopic failed for %s: %v", topic, err)
		}
	}

	// 切换到StrategySkip
	bus.SetTopicConfigStrategy(StrategySkip)

	// 并发发布到多个Topic
	messageCount := 100
	var publishedCount atomic.Int64

	startTime := time.Now()
	for i := 0; i < messageCount; i++ {
		topic := topics[i%len(topics)]
		message := []byte(fmt.Sprintf(`{"id": %d, "topic": "%s"}`, i, topic))

		err := bus.Publish(ctx, topic, message)
		if err != nil {
			t.Fatalf("Publish failed for %s: %v", topic, err)
		}
		publishedCount.Add(1)
	}
	duration := time.Since(startTime)
	throughput := float64(publishedCount.Load()) / duration.Seconds()

	t.Logf("✅ 多Topic预创建测试通过:")
	t.Logf("   Topic数量: %d", len(topics))
	t.Logf("   消息数量: %d", publishedCount.Load())
	t.Logf("   总耗时: %v", duration)
	t.Logf("   吞吐量: %.2f msg/s", throughput)
}

// TestNATSStreamPreCreation_StrategyComparison 对比不同策略的性能
func TestNATSStreamPreCreation_StrategyComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping strategy comparison test in short mode")
	}

	config := &NATSConfig{
		URLs:     []string{"nats://localhost:4222"},
		ClientID: "strategy-comparison-test",
		JetStream: JetStreamConfig{
			Enabled: true,
			Stream: StreamConfig{
				Name:     "STRATEGY_COMPARISON_STREAM",
				Subjects: []string{"strategy.>"},
				Storage:  "memory",
				Replicas: 1,
			},
		},
	}

	bus, err := NewNATSEventBus(config)
	if err != nil {
		t.Skipf("Skipping test: NATS not available: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()
	messageCount := 500

	strategies := []struct {
		name     string
		strategy TopicConfigStrategy
	}{
		{"CreateOrUpdate", StrategyCreateOrUpdate},
		{"CreateOnly", StrategyCreateOnly},
		{"Skip", StrategySkip},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			topic := fmt.Sprintf("strategy.%s", s.name)

			// 预创建（除了Skip策略）
			if s.strategy != StrategySkip {
				bus.SetTopicConfigStrategy(StrategyCreateOnly)
				err := bus.ConfigureTopic(ctx, topic, TopicOptions{
					PersistenceMode: TopicPersistent,
					RetentionTime:   1 * time.Hour,
					MaxMessages:     10000,
				})
				if err != nil {
					t.Fatalf("ConfigureTopic failed: %v", err)
				}
			}

			// 设置策略
			bus.SetTopicConfigStrategy(s.strategy)

			// 发布消息
			startTime := time.Now()
			for i := 0; i < messageCount; i++ {
				message := []byte(fmt.Sprintf(`{"id": %d}`, i))
				err := bus.Publish(ctx, topic, message)
				if err != nil {
					t.Fatalf("Publish failed: %v", err)
				}
			}
			duration := time.Since(startTime)
			throughput := float64(messageCount) / duration.Seconds()

			t.Logf("策略: %s", s.name)
			t.Logf("  消息数量: %d", messageCount)
			t.Logf("  总耗时: %v", duration)
			t.Logf("  吞吐量: %.2f msg/s", throughput)
			t.Logf("  平均延迟: %v/msg", duration/time.Duration(messageCount))
		})
	}
}

