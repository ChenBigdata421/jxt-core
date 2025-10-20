package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNewTopicBuilder 测试创建 TopicBuilder
func TestNewTopicBuilder(t *testing.T) {
	t.Run("valid topic name", func(t *testing.T) {
		builder := NewTopicBuilder("test-topic")
		assert.NotNil(t, builder)
		assert.Equal(t, "test-topic", builder.GetTopic())
		assert.Equal(t, DefaultTopicOptions(), builder.GetOptions())
	})

	t.Run("empty topic name", func(t *testing.T) {
		builder := NewTopicBuilder("")
		assert.NotNil(t, builder)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "topic name cannot be empty")
	})
}

// TestTopicBuilderPresets 测试预设配置
func TestTopicBuilderPresets(t *testing.T) {
	t.Run("ForHighThroughput", func(t *testing.T) {
		builder := NewTopicBuilder("test").ForHighThroughput()
		opts := builder.GetOptions()
		assert.Equal(t, 10, opts.Partitions)
		assert.Equal(t, 3, opts.ReplicationFactor)
		assert.Equal(t, TopicPersistent, opts.PersistenceMode)
	})

	t.Run("ForMediumThroughput", func(t *testing.T) {
		builder := NewTopicBuilder("test").ForMediumThroughput()
		opts := builder.GetOptions()
		assert.Equal(t, 5, opts.Partitions)
		assert.Equal(t, 3, opts.ReplicationFactor)
		assert.Equal(t, TopicPersistent, opts.PersistenceMode)
	})

	t.Run("ForLowThroughput", func(t *testing.T) {
		builder := NewTopicBuilder("test").ForLowThroughput()
		opts := builder.GetOptions()
		assert.Equal(t, 3, opts.Partitions)
		assert.Equal(t, 3, opts.ReplicationFactor)
		assert.Equal(t, TopicPersistent, opts.PersistenceMode)
	})
}

// TestTopicBuilderKafkaConfig 测试 Kafka 专有配置
func TestTopicBuilderKafkaConfig(t *testing.T) {
	t.Run("WithPartitions valid", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithPartitions(10)
		assert.Equal(t, 10, builder.GetOptions().Partitions)
		assert.NoError(t, builder.Validate())
	})

	t.Run("WithPartitions zero", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithPartitions(0)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partitions must be positive")
	})

	t.Run("WithPartitions negative", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithPartitions(-1)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partitions must be positive")
	})

	t.Run("WithPartitions too large", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithPartitions(150)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "should not exceed 100")
	})

	t.Run("WithReplication valid", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithPartitions(5).WithReplication(3)
		opts := builder.GetOptions()
		assert.Equal(t, 3, opts.ReplicationFactor)
		assert.Equal(t, 3, opts.Replicas) // 应该同步
		assert.NoError(t, builder.Validate())
	})

	t.Run("WithReplication zero", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithReplication(0)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "replication factor must be positive")
	})

	t.Run("WithReplication too large", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithReplication(10)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "should not exceed 5")
	})
}

// TestTopicBuilderCommonConfig 测试通用配置
func TestTopicBuilderCommonConfig(t *testing.T) {
	t.Run("WithRetention valid", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithRetention(24 * time.Hour)
		assert.Equal(t, 24*time.Hour, builder.GetOptions().RetentionTime)
		assert.NoError(t, builder.Validate())
	})

	t.Run("WithRetention zero", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithRetention(0)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retention time must be positive")
	})

	t.Run("WithRetention too short", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithRetention(30 * time.Second)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retention time too short")
	})

	t.Run("WithMaxSize valid", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithMaxSize(1024 * 1024 * 1024)
		assert.Equal(t, int64(1024*1024*1024), builder.GetOptions().MaxSize)
		assert.NoError(t, builder.Validate())
	})

	t.Run("WithMaxSize zero", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithMaxSize(0)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max size must be positive")
	})

	t.Run("WithMaxMessages valid", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithMaxMessages(100000)
		assert.Equal(t, int64(100000), builder.GetOptions().MaxMessages)
		assert.NoError(t, builder.Validate())
	})

	t.Run("WithPersistence", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithPersistence(TopicPersistent)
		assert.Equal(t, TopicPersistent, builder.GetOptions().PersistenceMode)
	})

	t.Run("Persistent shortcut", func(t *testing.T) {
		builder := NewTopicBuilder("test").Persistent()
		assert.Equal(t, TopicPersistent, builder.GetOptions().PersistenceMode)
	})

	t.Run("Ephemeral shortcut", func(t *testing.T) {
		builder := NewTopicBuilder("test").Ephemeral()
		assert.Equal(t, TopicEphemeral, builder.GetOptions().PersistenceMode)
	})

	t.Run("WithDescription", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithDescription("test description")
		assert.Equal(t, "test description", builder.GetOptions().Description)
	})
}

// TestTopicBuilderEnvironmentPresets 测试环境预设
func TestTopicBuilderEnvironmentPresets(t *testing.T) {
	t.Run("ForProduction", func(t *testing.T) {
		builder := NewTopicBuilder("test").ForProduction()
		opts := builder.GetOptions()
		assert.Equal(t, 3, opts.ReplicationFactor)
		assert.Equal(t, TopicPersistent, opts.PersistenceMode)
		assert.Equal(t, 7*24*time.Hour, opts.RetentionTime)
	})

	t.Run("ForDevelopment", func(t *testing.T) {
		builder := NewTopicBuilder("test").ForDevelopment()
		opts := builder.GetOptions()
		assert.Equal(t, 1, opts.ReplicationFactor)
		assert.Equal(t, TopicPersistent, opts.PersistenceMode)
		assert.Equal(t, 24*time.Hour, opts.RetentionTime)
	})

	t.Run("ForTesting", func(t *testing.T) {
		builder := NewTopicBuilder("test").ForTesting()
		opts := builder.GetOptions()
		assert.Equal(t, 1, opts.ReplicationFactor)
		assert.Equal(t, TopicEphemeral, opts.PersistenceMode)
		assert.Equal(t, 1*time.Hour, opts.RetentionTime)
	})
}

// TestTopicBuilderChaining 测试链式调用
func TestTopicBuilderChaining(t *testing.T) {
	t.Run("preset + override", func(t *testing.T) {
		builder := NewTopicBuilder("test").
			ForHighThroughput().
			WithPartitions(15).
			WithRetention(14 * 24 * time.Hour)

		opts := builder.GetOptions()
		assert.Equal(t, 15, opts.Partitions)                 // 覆盖了预设的10
		assert.Equal(t, 14*24*time.Hour, opts.RetentionTime) // 覆盖了预设的7天
		assert.Equal(t, 3, opts.ReplicationFactor)           // 保留预设的3
	})

	t.Run("complete custom", func(t *testing.T) {
		builder := NewTopicBuilder("test").
			WithPartitions(20).
			WithReplication(3).
			WithRetention(30 * 24 * time.Hour).
			WithMaxSize(5 * 1024 * 1024 * 1024).
			WithDescription("custom topic")

		opts := builder.GetOptions()
		assert.Equal(t, 20, opts.Partitions)
		assert.Equal(t, 3, opts.ReplicationFactor)
		assert.Equal(t, 30*24*time.Hour, opts.RetentionTime)
		assert.Equal(t, int64(5*1024*1024*1024), opts.MaxSize)
		assert.Equal(t, "custom topic", opts.Description)
	})
}

// TestTopicBuilderValidation 测试配置验证
func TestTopicBuilderValidation(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		builder := NewTopicBuilder("test").
			WithPartitions(10).
			WithReplication(3)
		assert.NoError(t, builder.Validate())
	})

	t.Run("replication factor exceeds partitions", func(t *testing.T) {
		builder := NewTopicBuilder("test").
			WithPartitions(3).
			WithReplication(5)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "replication factor")
		assert.Contains(t, err.Error(), "should not exceed partitions")
	})

	t.Run("multiple validation errors", func(t *testing.T) {
		builder := NewTopicBuilder("test").
			WithPartitions(0).
			WithReplication(0)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validation errors")
	})
}

// TestTopicBuilderBuild 测试构建方法
func TestTopicBuilderBuild(t *testing.T) {
	t.Run("build with memory eventbus", func(t *testing.T) {
		// 创建 Memory EventBus 用于测试
		bus := NewMemoryEventBus()
		defer bus.Close()

		ctx := context.Background()

		// 使用 Builder 构建
		err := NewTopicBuilder("test-topic").
			ForHighThroughput().
			Build(ctx, bus)

		assert.NoError(t, err)

		// 验证配置已保存
		config, err := bus.GetTopicConfig("test-topic")
		assert.NoError(t, err)
		assert.Equal(t, 10, config.Partitions)
	})

	t.Run("build with invalid config", func(t *testing.T) {
		bus := NewMemoryEventBus()
		defer bus.Close()

		ctx := context.Background()

		// 使用无效配置
		err := NewTopicBuilder("test-topic").
			WithPartitions(0).
			Build(ctx, bus)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})
}

// TestQuickBuildFunctions 测试快捷构建函数
func TestQuickBuildFunctions(t *testing.T) {
	bus := NewMemoryEventBus()
	defer bus.Close()

	ctx := context.Background()

	t.Run("BuildHighThroughputTopic", func(t *testing.T) {
		err := BuildHighThroughputTopic(ctx, bus, "high-topic")
		assert.NoError(t, err)

		config, _ := bus.GetTopicConfig("high-topic")
		assert.Equal(t, 10, config.Partitions)
	})

	t.Run("BuildMediumThroughputTopic", func(t *testing.T) {
		err := BuildMediumThroughputTopic(ctx, bus, "medium-topic")
		assert.NoError(t, err)

		config, _ := bus.GetTopicConfig("medium-topic")
		assert.Equal(t, 5, config.Partitions)
	})

	t.Run("BuildLowThroughputTopic", func(t *testing.T) {
		err := BuildLowThroughputTopic(ctx, bus, "low-topic")
		assert.NoError(t, err)

		config, _ := bus.GetTopicConfig("low-topic")
		assert.Equal(t, 3, config.Partitions)
	})
}

// TestTopicBuilderCompression 测试压缩配置
func TestTopicBuilderCompression(t *testing.T) {
	t.Run("WithCompression - valid algorithms", func(t *testing.T) {
		validAlgorithms := []string{"", "none", "gzip", "snappy", "lz4", "zstd"}
		for _, algo := range validAlgorithms {
			builder := NewTopicBuilder("test").WithCompression(algo)
			_ = builder.GetOptions()
			assert.NoError(t, builder.Validate())
		}
	})

	t.Run("WithCompression - invalid algorithm", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithCompression("invalid")
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid compression algorithm")
	})

	t.Run("WithCompressionLevel - valid levels", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithCompressionLevel(6)
		_ = builder.GetOptions()
		assert.NoError(t, builder.Validate())
	})

	t.Run("WithCompressionLevel - invalid level (negative)", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithCompressionLevel(-1)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "compression level must be non-negative")
	})

	t.Run("WithCompressionLevel - invalid level (too high)", func(t *testing.T) {
		builder := NewTopicBuilder("test").WithCompressionLevel(23)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "compression level should not exceed 22")
	})

	t.Run("NoCompression", func(t *testing.T) {
		builder := NewTopicBuilder("test").
			ForHighThroughput(). // 预设包含 snappy 压缩
			NoCompression()      // 禁用压缩
		_ = builder.GetOptions()
	})

	t.Run("SnappyCompression", func(t *testing.T) {
		builder := NewTopicBuilder("test").SnappyCompression()
		_ = builder.GetOptions()
	})

	t.Run("GzipCompression - valid level", func(t *testing.T) {
		builder := NewTopicBuilder("test").GzipCompression(9)
		_ = builder.GetOptions()
		assert.NoError(t, builder.Validate())
	})

	t.Run("GzipCompression - invalid level", func(t *testing.T) {
		builder := NewTopicBuilder("test").GzipCompression(10)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "gzip compression level must be 1-9")
	})

	t.Run("Lz4Compression", func(t *testing.T) {
		builder := NewTopicBuilder("test").Lz4Compression()
		_ = builder.GetOptions()
	})

	t.Run("ZstdCompression - valid level", func(t *testing.T) {
		builder := NewTopicBuilder("test").ZstdCompression(3)
		_ = builder.GetOptions()
		assert.NoError(t, builder.Validate())
	})

	t.Run("ZstdCompression - invalid level", func(t *testing.T) {
		builder := NewTopicBuilder("test").ZstdCompression(23)
		err := builder.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "zstd compression level must be 1-22")
	})

	t.Run("Compression override", func(t *testing.T) {
		builder := NewTopicBuilder("test").
			ForHighThroughput(). // 预设 snappy 压缩
			GzipCompression(6)   // 覆盖为 gzip 压缩
		opts := builder.GetOptions()
		assert.Equal(t, 10, opts.Partitions) // 保留预设的分区数
	})
}
