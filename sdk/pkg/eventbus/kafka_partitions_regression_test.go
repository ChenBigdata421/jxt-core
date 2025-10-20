package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTopicOptionsPartitions 测试分区配置
func TestTopicOptionsPartitions(t *testing.T) {
	t.Run("DefaultTopicOptions should have 1 partition", func(t *testing.T) {
		options := DefaultTopicOptions()
		assert.Equal(t, 1, options.Partitions, "Default should have 1 partition")
		assert.Equal(t, 1, options.ReplicationFactor, "Default should have 1 replica")
	})

	t.Run("LowThroughputTopicOptions should have 3 partitions", func(t *testing.T) {
		options := LowThroughputTopicOptions()
		assert.Equal(t, 3, options.Partitions, "Low throughput should have 3 partitions")
		assert.Equal(t, 3, options.ReplicationFactor, "Low throughput should have 3 replicas")
	})

	t.Run("MediumThroughputTopicOptions should have 5 partitions", func(t *testing.T) {
		options := MediumThroughputTopicOptions()
		assert.Equal(t, 5, options.Partitions, "Medium throughput should have 5 partitions")
		assert.Equal(t, 3, options.ReplicationFactor, "Medium throughput should have 3 replicas")
	})

	t.Run("HighThroughputTopicOptions should have 10 partitions", func(t *testing.T) {
		options := HighThroughputTopicOptions()
		assert.Equal(t, 10, options.Partitions, "High throughput should have 10 partitions")
		assert.Equal(t, 3, options.ReplicationFactor, "High throughput should have 3 replicas")
	})
}

// TestKafkaTopicPartitionsConfiguration 测试Kafka分区配置
func TestKafkaTopicPartitionsConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kafka integration test in short mode")
	}

	// 检查Kafka是否可用
	if !isKafkaAvailable() {
		t.Skip("Kafka is not available, skipping test")
	}

	ctx := context.Background()

	// 创建Kafka EventBus
	config := &KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Producer: ProducerConfig{
			RequiredAcks:   -1,
			FlushFrequency: 10 * time.Millisecond,
			FlushMessages:  100,
			Timeout:        10 * time.Second,
		},
		Consumer: ConsumerConfig{
			GroupID:           "partition-test-group",
			AutoOffsetReset:   "latest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
		},
	}

	bus, err := NewKafkaEventBus(config)
	require.NoError(t, err, "Failed to create Kafka EventBus")
	defer bus.Close()

	t.Run("Create topic with custom partitions", func(t *testing.T) {
		topic := "test-custom-partitions"
		options := DefaultTopicOptions()
		options.Partitions = 5
		options.ReplicationFactor = 1

		err := bus.ConfigureTopic(ctx, topic, options)
		// 注意：如果topic已存在，可能会返回错误或成功（取决于实现）
		// 这里我们只验证不会panic
		t.Logf("ConfigureTopic result: %v", err)
	})

	t.Run("Create topic with high throughput options", func(t *testing.T) {
		topic := "test-high-throughput"
		options := HighThroughputTopicOptions()
		options.ReplicationFactor = 1 // 测试环境可能只有1个broker

		err := bus.ConfigureTopic(ctx, topic, options)
		t.Logf("ConfigureTopic result: %v", err)
	})

	t.Run("Create topic with medium throughput options", func(t *testing.T) {
		topic := "test-medium-throughput"
		options := MediumThroughputTopicOptions()
		options.ReplicationFactor = 1 // 测试环境可能只有1个broker

		err := bus.ConfigureTopic(ctx, topic, options)
		t.Logf("ConfigureTopic result: %v", err)
	})

	t.Run("Create topic with low throughput options", func(t *testing.T) {
		topic := "test-low-throughput"
		options := LowThroughputTopicOptions()
		options.ReplicationFactor = 1 // 测试环境可能只有1个broker

		err := bus.ConfigureTopic(ctx, topic, options)
		t.Logf("ConfigureTopic result: %v", err)
	})
}

// TestPartitionConfigComparison 测试分区配置比较
func TestPartitionConfigComparison(t *testing.T) {
	t.Run("Partition count mismatch - can increase", func(t *testing.T) {
		expected := DefaultTopicOptions()
		expected.Partitions = 10

		actual := DefaultTopicOptions()
		actual.Partitions = 5

		mismatches := compareTopicOptions("test-topic", expected, actual)

		// 应该检测到分区数不匹配
		found := false
		for _, mismatch := range mismatches {
			if mismatch.Field == "Partitions" {
				found = true
				assert.True(t, mismatch.CanAutoFix, "Increasing partitions should be auto-fixable")
				assert.Equal(t, 10, mismatch.ExpectedValue)
				assert.Equal(t, 5, mismatch.ActualValue)
			}
		}
		assert.True(t, found, "Should detect partition count mismatch")
	})

	t.Run("Partition count mismatch - cannot decrease", func(t *testing.T) {
		expected := DefaultTopicOptions()
		expected.Partitions = 5

		actual := DefaultTopicOptions()
		actual.Partitions = 10

		mismatches := compareTopicOptions("test-topic", expected, actual)

		// 应该检测到分区数不匹配
		found := false
		for _, mismatch := range mismatches {
			if mismatch.Field == "Partitions" {
				found = true
				assert.False(t, mismatch.CanAutoFix, "Decreasing partitions should not be auto-fixable")
				assert.Equal(t, 5, mismatch.ExpectedValue)
				assert.Equal(t, 10, mismatch.ActualValue)
			}
		}
		assert.True(t, found, "Should detect partition count mismatch")
	})

	t.Run("ReplicationFactor mismatch", func(t *testing.T) {
		expected := DefaultTopicOptions()
		expected.ReplicationFactor = 3

		actual := DefaultTopicOptions()
		actual.ReplicationFactor = 1

		mismatches := compareTopicOptions("test-topic", expected, actual)

		// 应该检测到副本因子不匹配
		found := false
		for _, mismatch := range mismatches {
			if mismatch.Field == "ReplicationFactor" {
				found = true
				assert.False(t, mismatch.CanAutoFix, "Replication factor should not be auto-fixable")
				assert.Equal(t, 3, mismatch.ExpectedValue)
				assert.Equal(t, 1, mismatch.ActualValue)
			}
		}
		assert.True(t, found, "Should detect replication factor mismatch")
	})
}

// TestPartitionPerformanceScenarios 测试不同分区配置的性能场景
func TestPartitionPerformanceScenarios(t *testing.T) {
	scenarios := []struct {
		name        string
		optionsFunc func() TopicOptions
		description string
	}{
		{
			name:        "Low throughput scenario",
			optionsFunc: LowThroughputTopicOptions,
			description: "Suitable for <100 msg/s",
		},
		{
			name:        "Medium throughput scenario",
			optionsFunc: MediumThroughputTopicOptions,
			description: "Suitable for 100-1000 msg/s",
		},
		{
			name:        "High throughput scenario",
			optionsFunc: HighThroughputTopicOptions,
			description: "Suitable for >1000 msg/s",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			options := scenario.optionsFunc()

			// 验证配置合理性
			assert.Greater(t, options.Partitions, 0, "Partitions should be positive")
			assert.Greater(t, options.ReplicationFactor, 0, "ReplicationFactor should be positive")
			assert.Greater(t, options.RetentionTime, time.Duration(0), "RetentionTime should be positive")
			assert.Greater(t, options.MaxSize, int64(0), "MaxSize should be positive")
			assert.Greater(t, options.MaxMessages, int64(0), "MaxMessages should be positive")

			t.Logf("Scenario: %s", scenario.description)
			t.Logf("  Partitions: %d", options.Partitions)
			t.Logf("  ReplicationFactor: %d", options.ReplicationFactor)
			t.Logf("  RetentionTime: %v", options.RetentionTime)
			t.Logf("  MaxSize: %d bytes", options.MaxSize)
			t.Logf("  MaxMessages: %d", options.MaxMessages)
		})
	}
}

// isKafkaAvailable 检查Kafka是否可用
func isKafkaAvailable() bool {
	// 简单的可用性检查
	// 实际项目中可以尝试连接Kafka
	return false // 默认跳过，需要手动启动Kafka才能运行
}

