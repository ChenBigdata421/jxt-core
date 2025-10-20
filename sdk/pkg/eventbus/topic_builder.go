package eventbus

import (
	"context"
	"fmt"
	"time"
)

// TopicBuilder 主题配置构建器
// 提供流畅的链式调用API来配置主题
//
// 使用示例：
//
//	// 使用预设配置
//	err := eventbus.NewTopicBuilder("orders").
//	    ForHighThroughput().
//	    Build(ctx, bus)
//
//	// 预设 + 自定义覆盖
//	err := eventbus.NewTopicBuilder("orders").
//	    ForHighThroughput().
//	    WithPartitions(15).  // 覆盖预设的10个分区
//	    Build(ctx, bus)
//
//	// 完全自定义
//	err := eventbus.NewTopicBuilder("custom-topic").
//	    WithPartitions(20).
//	    WithReplication(3).
//	    WithRetention(30 * 24 * time.Hour).
//	    WithMaxSize(5 * 1024 * 1024 * 1024).
//	    Build(ctx, bus)
type TopicBuilder struct {
	topic   string
	options TopicOptions
	errors  []error
}

// NewTopicBuilder 创建主题配置构建器
func NewTopicBuilder(topic string) *TopicBuilder {
	// 验证主题名称
	if err := ValidateTopicName(topic); err != nil {
		return &TopicBuilder{
			topic:   topic,
			options: DefaultTopicOptions(),
			errors:  []error{err},
		}
	}

	return &TopicBuilder{
		topic:   topic,
		options: DefaultTopicOptions(),
		errors:  []error{},
	}
}

// ========== 预设配置方法 ==========

// ForHighThroughput 使用高吞吐量预设配置
// 适用于高流量场景（>1000 msg/s）
// 配置：10个分区，3个副本，保留7天，1GB存储
func (b *TopicBuilder) ForHighThroughput() *TopicBuilder {
	b.options = HighThroughputTopicOptions()
	return b
}

// ForMediumThroughput 使用中等吞吐量预设配置
// 适用于中流量场景（100-1000 msg/s）
// 配置：5个分区，3个副本，保留3天，500MB存储
func (b *TopicBuilder) ForMediumThroughput() *TopicBuilder {
	b.options = MediumThroughputTopicOptions()
	return b
}

// ForLowThroughput 使用低吞吐量预设配置
// 适用于低流量场景（<100 msg/s）
// 配置：3个分区，3个副本，保留1天，100MB存储
func (b *TopicBuilder) ForLowThroughput() *TopicBuilder {
	b.options = LowThroughputTopicOptions()
	return b
}

// ========== Kafka 专有配置 ==========

// WithPartitions 设置分区数（仅Kafka有效）
// 性能优化：多分区可以提升并行消费能力和吞吐量
//
// 推荐值：
//   - 低流量主题（<100 msg/s）：1-3 个分区
//   - 中流量主题（100-1000 msg/s）：3-10 个分区
//   - 高流量主题（>1000 msg/s）：10-30 个分区
//
// 注意：
//   - 分区数一旦设置，只能增加不能减少
//   - 单个topic建议不超过100个分区
//   - 分区数应该 >= 消费者数量以实现最优并行度
func (b *TopicBuilder) WithPartitions(partitions int) *TopicBuilder {
	if partitions <= 0 {
		b.errors = append(b.errors, fmt.Errorf("partitions must be positive, got %d", partitions))
		return b
	}
	if partitions > 100 {
		b.errors = append(b.errors, fmt.Errorf("partitions should not exceed 100 for performance reasons, got %d", partitions))
		return b
	}
	b.options.Partitions = partitions
	return b
}

// WithReplication 设置副本因子（仅Kafka有效）
// 高可用性配置：副本数越多，可用性越高，但存储成本也越高
//
// 推荐值：
//   - 开发环境：1个副本
//   - 测试环境：1-2个副本
//   - 生产环境：至少3个副本
//
// 注意：
//   - 副本因子一旦设置，不能修改（需要重建topic）
//   - 副本因子不能超过broker数量
func (b *TopicBuilder) WithReplication(replicationFactor int) *TopicBuilder {
	if replicationFactor <= 0 {
		b.errors = append(b.errors, fmt.Errorf("replication factor must be positive, got %d", replicationFactor))
		return b
	}
	if replicationFactor > 5 {
		b.errors = append(b.errors, fmt.Errorf("replication factor should not exceed 5 for performance reasons, got %d", replicationFactor))
		return b
	}
	b.options.ReplicationFactor = replicationFactor
	b.options.Replicas = replicationFactor // 保持同步
	return b
}

// ========== 压缩配置方法（Kafka专有） ==========

// WithCompression 设置压缩算法（Kafka专有）
// 支持的压缩算法：
//   - "none" 或 "" - 不压缩
//   - "gzip" - GZIP压缩（高压缩率，高CPU开销）
//   - "snappy" - Snappy压缩（平衡性能，推荐）
//   - "lz4" - LZ4压缩（最快速度，低压缩率）
//   - "zstd" - Zstandard压缩（最佳平衡，Kafka 2.1+）
//
// 推荐值：
//   - 生产环境：snappy（平衡性能和压缩率）
//   - 高吞吐量：lz4（最快速度）
//   - 存储优先：gzip 或 zstd（最高压缩率）
func (b *TopicBuilder) WithCompression(compression string) *TopicBuilder {
	validCompressions := map[string]bool{
		"":       true,
		"none":   true,
		"gzip":   true,
		"snappy": true,
		"lz4":    true,
		"zstd":   true,
	}
	if !validCompressions[compression] {
		b.errors = append(b.errors, fmt.Errorf("invalid compression algorithm: %s (valid: none, gzip, snappy, lz4, zstd)", compression))
		return b
	}
	b.options.Compression = compression
	return b
}

// WithCompressionLevel 设置压缩级别（Kafka专有）
// 不同压缩算法支持不同的级别范围：
//   - gzip: 1-9（1=最快，9=最高压缩率，默认6）
//   - zstd: 1-22（1=最快，22=最高压缩率，默认3）
//   - snappy/lz4: 忽略此参数（固定压缩级别）
func (b *TopicBuilder) WithCompressionLevel(level int) *TopicBuilder {
	if level < 0 {
		b.errors = append(b.errors, fmt.Errorf("compression level must be non-negative, got %d", level))
		return b
	}
	if level > 22 {
		b.errors = append(b.errors, fmt.Errorf("compression level should not exceed 22, got %d", level))
		return b
	}
	b.options.CompressionLevel = level
	return b
}

// NoCompression 禁用压缩（快捷方法）
func (b *TopicBuilder) NoCompression() *TopicBuilder {
	b.options.Compression = "none"
	b.options.CompressionLevel = 0
	return b
}

// SnappyCompression 使用Snappy压缩（快捷方法，推荐）
// Snappy提供了性能和压缩率的最佳平衡
func (b *TopicBuilder) SnappyCompression() *TopicBuilder {
	b.options.Compression = "snappy"
	b.options.CompressionLevel = 6 // Snappy忽略此参数，但设置为默认值
	return b
}

// GzipCompression 使用GZIP压缩（快捷方法）
// GZIP提供高压缩率，但CPU开销较大
func (b *TopicBuilder) GzipCompression(level int) *TopicBuilder {
	if level < 1 || level > 9 {
		b.errors = append(b.errors, fmt.Errorf("gzip compression level must be 1-9, got %d", level))
		return b
	}
	b.options.Compression = "gzip"
	b.options.CompressionLevel = level
	return b
}

// Lz4Compression 使用LZ4压缩（快捷方法）
// LZ4提供最快的压缩/解压速度
func (b *TopicBuilder) Lz4Compression() *TopicBuilder {
	b.options.Compression = "lz4"
	b.options.CompressionLevel = 0 // LZ4忽略此参数
	return b
}

// ZstdCompression 使用Zstandard压缩（快捷方法，Kafka 2.1+）
// Zstd提供最佳的压缩率和性能平衡
func (b *TopicBuilder) ZstdCompression(level int) *TopicBuilder {
	if level < 1 || level > 22 {
		b.errors = append(b.errors, fmt.Errorf("zstd compression level must be 1-22, got %d", level))
		return b
	}
	b.options.Compression = "zstd"
	b.options.CompressionLevel = level
	return b
}

// ========== 通用配置 ==========

// WithRetention 设置消息保留时间
// 持久化模式下，消息会保留指定时间后自动删除
//
// 推荐值：
//   - 实时消息：1-24小时
//   - 业务事件：3-7天
//   - 审计日志：30-90天
func (b *TopicBuilder) WithRetention(duration time.Duration) *TopicBuilder {
	if duration <= 0 {
		b.errors = append(b.errors, fmt.Errorf("retention time must be positive, got %v", duration))
		return b
	}
	b.options.RetentionTime = duration
	return b
}

// WithMaxSize 设置主题最大存储大小（字节）
// 达到上限后，根据策略删除旧消息
//
// 推荐值：
//   - 小流量：100MB - 500MB
//   - 中流量：500MB - 2GB
//   - 大流量：2GB - 10GB
func (b *TopicBuilder) WithMaxSize(bytes int64) *TopicBuilder {
	if bytes <= 0 {
		b.errors = append(b.errors, fmt.Errorf("max size must be positive, got %d", bytes))
		return b
	}
	b.options.MaxSize = bytes
	return b
}

// WithMaxMessages 设置主题最大消息数量
// 达到上限后，根据策略删除旧消息
func (b *TopicBuilder) WithMaxMessages(count int64) *TopicBuilder {
	if count <= 0 {
		b.errors = append(b.errors, fmt.Errorf("max messages must be positive, got %d", count))
		return b
	}
	b.options.MaxMessages = count
	return b
}

// WithPersistence 设置持久化模式
//
// 模式说明：
//   - TopicPersistent: 持久化存储（使用JetStream/Kafka等）
//   - TopicEphemeral: 非持久化存储（使用Core NATS/内存等）
//   - TopicAuto: 自动选择（根据EventBus全局配置）
func (b *TopicBuilder) WithPersistence(mode TopicPersistenceMode) *TopicBuilder {
	if mode != TopicPersistent && mode != TopicEphemeral && mode != TopicAuto {
		b.errors = append(b.errors, fmt.Errorf("invalid persistence mode: %s", mode))
		return b
	}
	b.options.PersistenceMode = mode
	return b
}

// Persistent 设置为持久化模式（快捷方法）
func (b *TopicBuilder) Persistent() *TopicBuilder {
	b.options.PersistenceMode = TopicPersistent
	return b
}

// Ephemeral 设置为非持久化模式（快捷方法）
func (b *TopicBuilder) Ephemeral() *TopicBuilder {
	b.options.PersistenceMode = TopicEphemeral
	return b
}

// WithDescription 设置主题描述
func (b *TopicBuilder) WithDescription(description string) *TopicBuilder {
	b.options.Description = description
	return b
}

// ========== 便捷配置方法 ==========

// ForProduction 生产环境配置（高可用 + 持久化）
// 配置：3个副本，持久化存储，保留7天
func (b *TopicBuilder) ForProduction() *TopicBuilder {
	b.options.ReplicationFactor = 3
	b.options.Replicas = 3
	b.options.PersistenceMode = TopicPersistent
	b.options.RetentionTime = 7 * 24 * time.Hour
	return b
}

// ForDevelopment 开发环境配置（单副本 + 短保留）
// 配置：1个副本，持久化存储，保留1天
func (b *TopicBuilder) ForDevelopment() *TopicBuilder {
	b.options.ReplicationFactor = 1
	b.options.Replicas = 1
	b.options.PersistenceMode = TopicPersistent
	b.options.RetentionTime = 24 * time.Hour
	return b
}

// ForTesting 测试环境配置（非持久化 + 短保留）
// 配置：1个副本，非持久化存储，保留1小时
func (b *TopicBuilder) ForTesting() *TopicBuilder {
	b.options.ReplicationFactor = 1
	b.options.Replicas = 1
	b.options.PersistenceMode = TopicEphemeral
	b.options.RetentionTime = 1 * time.Hour
	return b
}

// ========== 构建和验证 ==========

// Validate 验证配置是否有效
func (b *TopicBuilder) Validate() error {
	// 检查构建过程中的错误
	if len(b.errors) > 0 {
		return fmt.Errorf("builder has %d validation errors: %v", len(b.errors), b.errors)
	}

	// 检查主题名称
	if b.topic == "" {
		return fmt.Errorf("topic name cannot be empty")
	}

	// 检查分区数和副本因子的关系
	if b.options.Partitions > 0 && b.options.ReplicationFactor > 0 {
		// 副本因子不应该超过分区数（虽然Kafka允许，但不推荐）
		if b.options.ReplicationFactor > b.options.Partitions {
			return fmt.Errorf("replication factor (%d) should not exceed partitions (%d)",
				b.options.ReplicationFactor, b.options.Partitions)
		}
	}

	// 检查保留时间和大小的合理性
	if b.options.RetentionTime > 0 && b.options.RetentionTime < time.Minute {
		return fmt.Errorf("retention time too short: %v (minimum 1 minute)", b.options.RetentionTime)
	}

	return nil
}

// Build 构建并配置主题
// 这是最终的构建方法，会验证所有配置并调用 EventBus.ConfigureTopic
func (b *TopicBuilder) Build(ctx context.Context, bus EventBus) error {
	// 验证配置
	if err := b.Validate(); err != nil {
		return fmt.Errorf("topic builder validation failed: %w", err)
	}

	// 调用 EventBus 的 ConfigureTopic 方法
	if err := bus.ConfigureTopic(ctx, b.topic, b.options); err != nil {
		return fmt.Errorf("failed to configure topic %s: %w", b.topic, err)
	}

	return nil
}

// GetOptions 获取当前配置（用于调试或预览）
func (b *TopicBuilder) GetOptions() TopicOptions {
	return b.options
}

// GetTopic 获取主题名称
func (b *TopicBuilder) GetTopic() string {
	return b.topic
}

// ========== 便捷构建函数 ==========

// BuildHighThroughputTopic 快速构建高吞吐量主题
func BuildHighThroughputTopic(ctx context.Context, bus EventBus, topic string) error {
	return NewTopicBuilder(topic).ForHighThroughput().Build(ctx, bus)
}

// BuildMediumThroughputTopic 快速构建中等吞吐量主题
func BuildMediumThroughputTopic(ctx context.Context, bus EventBus, topic string) error {
	return NewTopicBuilder(topic).ForMediumThroughput().Build(ctx, bus)
}

// BuildLowThroughputTopic 快速构建低吞吐量主题
func BuildLowThroughputTopic(ctx context.Context, bus EventBus, topic string) error {
	return NewTopicBuilder(topic).ForLowThroughput().Build(ctx, bus)
}
