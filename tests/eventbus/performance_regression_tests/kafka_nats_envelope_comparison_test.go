//go:build integration
// +build integration

package performance_tests

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/IBM/sarama"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 🎯 Kafka vs NATS JetStream 全面性能对比测试
//
// 测试要求：
// 1. 创建 EventBus 实例（覆盖 Kafka 和 NATS JetStream 两种实现）
// 2. NATS JetStream 必须持久化到磁盘
// 3. 发布端必须采用 PublishEnvelope 方法
// 4. 订阅端必须采用 SubscribeEnvelope 方法
// 5. 测试覆盖低压500、中压2000、高压5000、极限10000
// 6. Topic 数量为 5
// 7. 输出报告包括性能指标、关键资源占用情况、连接数、消费者组个数对比
// 8. Kafka 的 ClientID 和 topic 名称只使用 ASCII 字符
// 9. 处理延迟测量：端到端延迟（从 Envelope.Timestamp 发送时间到接收时间）

// OrderChecker / NewOrderChecker 已抽取到 ordercheck_helper_test.go（hermetic 的 order_checker_test.go 共用）

// PerfMetrics 性能指标
type PerfMetrics struct {
	// 基本信息
	System       string        // 系统名称 (Kafka/NATS)
	Pressure     string        // 压力级别
	MessageCount int           // 消息总数
	StartTime    time.Time     // 开始时间
	EndTime      time.Time     // 结束时间
	Duration     time.Duration // 持续时间

	// 消息指标
	MessagesSent     int64   // 发送消息数
	MessagesReceived int64   // 接收消息数
	SendErrors       int64   // 发送错误数
	ProcessErrors    int64   // 处理错误数
	SuccessRate      float64 // 成功率

	// 性能指标
	SendThroughput    float64 // 发送吞吐量 (msg/s)
	ReceiveThroughput float64 // 接收吞吐量 (msg/s)
	AvgSendLatency    float64 // 平均发送延迟 (ms) - PublishEnvelope 方法执行时间
	AvgProcessLatency float64 // 平均处理延迟 (ms) - 端到端延迟（Envelope.Timestamp → 接收时间）

	// 资源占用
	InitialGoroutines int     // 初始协程数
	PeakGoroutines    int32   // 峰值协程数 (使用 int32 以便原子操作)
	FinalGoroutines   int     // 最终协程数
	GoroutineLeak     int     // 协程泄漏数
	InitialMemoryMB   float64 // 初始内存 (MB)
	PeakMemoryMB      uint64  // 峰值内存 (MB) - 存储为 uint64 bits 以便原子操作
	FinalMemoryMB     float64 // 最终内存 (MB)
	MemoryDeltaMB     float64 // 内存增量 (MB)

	// 连接和消费者组统计（新增）
	TopicCount         int              // Topic 数量
	ConnectionCount    int              // 连接数
	ConsumerGroupCount int              // 消费者组个数
	TopicList          []string         // Topic 列表
	PartitionCount     map[string]int32 // Kafka Topic 分区数

	// 顺序性指标
	OrderViolations int64 // 顺序违反次数

	// 内部统计
	sendLatencySum   int64 // 发送延迟总和 (微秒)
	sendLatencyCount int64 // 发送延迟计数
	procLatencySum   int64 // 处理延迟总和 (微秒)
	procLatencyCount int64 // 处理延迟计数
}

// createKafkaTopicsWithBuilder 使用 TopicBuilder 创建 Kafka Topics
// 🔥 重构后：压缩配置在 topic 级别，不再使用 Producer 级别的压缩配置
func createKafkaTopicsWithBuilder(ctx context.Context, t *testing.T, bus eventbus.EventBus, topics []string, partitions int, replication int) map[string]int32 {
	t.Logf("🔧 使用 TopicBuilder 创建 Kafka Topics (分区数: %d, 副本数: %d)...", partitions, replication)
	t.Logf("   📦 压缩模式: Topic 级别（每个 topic 独立配置）")

	// 记录实际创建的分区数
	actualPartitions := make(map[string]int32)

	// 使用 TopicBuilder 为每个 topic 创建配置
	for _, topicName := range topics {
		// 🔥 重构后：使用 Builder 模式创建 topic，压缩配置在 topic 级别
		// 配置：指定分区数、副本数、Snappy 压缩、持久化模式
		err := eventbus.NewTopicBuilder(topicName).
			WithPartitions(partitions).    // 分区数
			WithReplication(replication).  // 副本数
			SnappyCompression().           // ✅ Topic 级别压缩（Snappy，平衡性能）
			Persistent().                  // ✅ 持久化模式
			WithRetention(7*24*time.Hour). // 保留 7 天
			WithMaxSize(1*1024*1024*1024). // 1GB
			WithDescription(fmt.Sprintf("Performance test topic with %d partitions and snappy compression", partitions)).
			Build(ctx, bus)

		if err != nil {
			t.Logf("   ❌ 创建失败: %s - %v", topicName, err)
		} else {
			actualPartitions[topicName] = int32(partitions)
			t.Logf("   ✅ 创建成功: %s (%d partitions, snappy compression, persistent)", topicName, partitions)
		}
	}

	// 等待 topic 创建完成
	time.Sleep(500 * time.Millisecond)

	// 验证创建的 topics
	t.Logf("📊 验证创建的 Topics:")
	for _, topicName := range topics {
		config, err := bus.GetTopicConfig(topicName)
		if err != nil {
			t.Logf("   ⚠️  无法获取 topic %s 配置: %v", topicName, err)
		} else {
			actualPartitions[topicName] = int32(config.Partitions)
			t.Logf("   %s: partitions=%d, compression=%s, persistence=%s",
				topicName, config.Partitions, config.Compression, config.PersistenceMode)
		}
	}

	t.Logf("✅ 成功创建 %d 个 Kafka topics (Topic 级别压缩配置)", len(actualPartitions))
	return actualPartitions
}

// createNATSTopicsWithPersistence 已废弃
// ⭐ Stream 预建立方式：在 NATSConfig 中配置统一的 Stream（subject pattern 使用通配符）
// 这样可以避免为每个 topic 创建单独的 Stream，防止 subject 重叠错误
// 参考：jxt-core/sdk/pkg/eventbus/README.md - Stream 预建立优化

// cleanupKafka 清理 Kafka 测试数据
func cleanupKafka(t *testing.T, topicPrefix string) {
	t.Logf("🧹 清理 Kafka 测试数据 (topic prefix: %s)...", topicPrefix)

	// 创建 Kafka 配置
	config := sarama.NewConfig()
	config.Version = sarama.V2_6_0_0

	// 创建 Kafka 管理客户端
	admin, err := sarama.NewClusterAdmin([]string{"localhost:29094"}, config)
	if err != nil {
		t.Logf("⚠️  无法创建 Kafka 管理客户端: %v", err)
		return
	}
	defer admin.Close()

	// 列出所有 topics
	topics, err := admin.ListTopics()
	if err != nil {
		t.Logf("⚠️  无法列出 Kafka topics: %v", err)
		return
	}

	// 收集需要删除的 topics
	var topicsToDelete []string
	for topic := range topics {
		// 只删除以指定前缀开头的 topic
		if len(topicPrefix) == 0 || strings.HasPrefix(topic, topicPrefix) {
			topicsToDelete = append(topicsToDelete, topic)
		}
	}

	// 删除 topics
	if len(topicsToDelete) > 0 {
		err = admin.DeleteTopic(topicsToDelete[0]) // Sarama 一次只能删除一个
		for _, topic := range topicsToDelete {
			err = admin.DeleteTopic(topic)
			if err != nil {
				t.Logf("⚠️  删除 topic %s 失败: %v", topic, err)
			} else {
				t.Logf("   ✅ 已删除 topic: %s", topic)
			}
		}
		t.Logf("✅ 成功删除 %d 个 Kafka topics", len(topicsToDelete))
	} else {
		t.Logf("ℹ️  没有找到需要清理的 Kafka topics")
	}
}

// cleanupNATS 清理 NATS JetStream 测试数据
func cleanupNATS(t *testing.T, streamPrefix string) {
	t.Logf("🧹 清理 NATS JetStream 测试数据 (stream prefix: %s)...", streamPrefix)

	// 直接使用 NATS 客户端连接
	nc, err := nats.Connect("nats://localhost:4223", nats.Name("nats-cleanup"))
	if err != nil {
		t.Logf("⚠️  无法连接到 NATS: %v", err)
		return
	}
	defer nc.Close()

	// 创建 JetStream 上下文
	js, err := nc.JetStream()
	if err != nil {
		t.Logf("⚠️  无法创建 JetStream 上下文: %v", err)
		return
	}

	// 列出所有 streams
	streams := js.StreamNames()
	deletedCount := 0

	for streamName := range streams {
		// 只删除以指定前缀开头的 stream
		if len(streamPrefix) == 0 || strings.HasPrefix(streamName, streamPrefix) {
			err := js.DeleteStream(streamName)
			if err != nil {
				t.Logf("⚠️  删除 stream %s 失败: %v", streamName, err)
			} else {
				t.Logf("   ✅ 已删除 stream: %s", streamName)
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		t.Logf("✅ 成功删除 %d 个 NATS streams", deletedCount)
	} else {
		t.Logf("ℹ️  没有找到需要清理的 NATS streams")
	}
}

// cleanupBeforeTest 在每个测试场景前清理环境
func cleanupBeforeTest(t *testing.T, scenarioName string) {
	t.Logf("\n🔄 准备测试环境: %s", scenarioName)

	// 清理 Kafka
	cleanupKafka(t, "kafka.perf")

	// 清理 NATS
	cleanupNATS(t, "PERF_")

	// 强制垃圾回收
	runtime.GC()
	time.Sleep(2 * time.Second)

	t.Logf("✅ 测试环境准备完成\n")
}

// TestKafkaVsNATSPerformanceComparison 主测试函数
func TestKafkaVsNATSPerformanceComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive comparison test in short mode")
	}

	// 初始化 logger（如果还没有初始化）
	if logger.DefaultLogger == nil {
		zapLogger, err := zap.NewDevelopment()
		if err != nil {
			t.Fatalf("Failed to initialize logger: %v", err)
		}
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	t.Log("🚀 ===== Kafka vs NATS JetStream 全面性能对比测试 =====")
	t.Log("📋 测试要求:")
	t.Log("   1. 创建 EventBus 实例（Kafka 和 NATS JetStream）")
	t.Log("   2. NATS JetStream 持久化到磁盘")
	t.Log("   3. 使用 PublishEnvelope 方法发布")
	t.Log("   4. 使用 SubscribeEnvelope 方法订阅")
	t.Log("   5. 测试压力：低压500、中压2000、高压5000、极限10000")
	t.Log("   6. Topic 数量：5")
	t.Log("   7. 统计连接数和消费者组个数")
	t.Log("   8. 仅使用 ASCII 字符命名")
	t.Log("")

	// 测试场景
	scenarios := []struct {
		name     string
		messages int
		timeout  time.Duration
	}{
		{"低压", 500, 60 * time.Second},
		{"中压", 2000, 120 * time.Second},
		{"高压", 5000, 180 * time.Second},
		{"极限", 10000, 300 * time.Second},
	}

	// 存储所有测试结果
	allResults := make(map[string][]*PerfMetrics)
	allResults["Kafka"] = make([]*PerfMetrics, 0)
	allResults["NATS"] = make([]*PerfMetrics, 0)

	// 运行所有场景测试
	for _, scenario := range scenarios {
		t.Log("\n" + strings.Repeat("=", 80))
		t.Logf("🎯 开始 %s 测试 (%d 条消息)", scenario.name, scenario.messages)
		t.Log(strings.Repeat("=", 80))

		// 🔄 清理测试环境
		cleanupBeforeTest(t, scenario.name)

		// 测试 Kafka
		t.Logf("\n🔴 测试 Kafka...")
		kafkaMetrics := runKafkaTest(t, scenario.name, scenario.messages, scenario.timeout)
		allResults["Kafka"] = append(allResults["Kafka"], kafkaMetrics)

		// 清理和等待
		t.Logf("⏳ 等待资源释放...")
		time.Sleep(5 * time.Second)
		runtime.GC()

		// 测试 NATS JetStream
		t.Logf("\n🔵 测试 NATS JetStream (磁盘持久化)...")
		natsMetrics := runNATSTest(t, scenario.name, scenario.messages, scenario.timeout)
		allResults["NATS"] = append(allResults["NATS"], natsMetrics)

		// 对比本轮结果
		compareRoundResults(t, scenario.name, kafkaMetrics, natsMetrics)

		// 清理和等待
		t.Logf("⏳ 等待资源释放...")
		time.Sleep(5 * time.Second)
		runtime.GC()
	}

	// 生成综合对比报告
	generateComparisonReport(t, allResults)
}

// runKafkaTest 运行 Kafka 性能测试
func runKafkaTest(t *testing.T, pressure string, messageCount int, timeout time.Duration) *PerfMetrics {
	metrics := &PerfMetrics{
		System:       "Kafka",
		Pressure:     pressure,
		MessageCount: messageCount,
	}

	// 记录初始资源状态
	metrics.InitialGoroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.InitialMemoryMB = float64(m.Alloc) / 1024 / 1024

	// 创建 Kafka EventBus - 参考成功的测试配置
	// 🔑 关键修复：不能使用中文字符，会导致 Kafka 无法接收消息
	pressureEn := map[string]string{
		"低压": "low",
		"中压": "medium",
		"高压": "high",
		"极限": "extreme",
	}[pressure]
	if pressureEn == "" {
		pressureEn = pressure // 如果不是中文，直接使用
	}

	kafkaConfig := &eventbus.KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: fmt.Sprintf("kafka-perf-%s-%d", pressureEn, time.Now().Unix()),
		Producer: eventbus.ProducerConfig{
			RequiredAcks: -1, // WaitForAll，幂等性生产者要求
			// 🔥 重构后：压缩配置已从 Producer 级别移到 Topic 级别
			// 不再在这里配置 Compression 和 CompressionLevel
			// 压缩配置现在通过 TopicBuilder.SnappyCompression() 在 topic 级别设置
			FlushFrequency:  10 * time.Millisecond,
			FlushMessages:   100,
			Timeout:         30 * time.Second,
			FlushBytes:      1024 * 1024, // 1MB
			RetryMax:        3,
			BatchSize:       16 * 1024,        // 16KB
			BufferSize:      32 * 1024 * 1024, // 32MB
			Idempotent:      true,
			MaxMessageBytes: 1024 * 1024,
			PartitionerType: "hash",
			LingerMs:        5 * time.Millisecond,
			MaxInFlight:     1, // 幂等性生产者要求MaxInFlight=1
		},
		Consumer: eventbus.ConsumerConfig{
			GroupID:            fmt.Sprintf("kafka-perf-%s-group", pressureEn),
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  30 * time.Second,
			FetchMinBytes:      1024,
			FetchMaxBytes:      50 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     500,
			EnableAutoCommit:   false,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_committed",
			RebalanceStrategy:  "range",
		},
		HealthCheckInterval:  30 * time.Second,
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 250 * time.Millisecond,
	}

	// 🔑 要求：创建 5 个 topic，每个 topic 3 个分区
	topicCount := 5
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("kafka.perf.%s.topic%d", pressureEn, i+1)
	}

	// 创建 Kafka EventBus
	eb, err := eventbus.NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err, "Failed to create Kafka EventBus")

	// 🔧 使用 TopicBuilder 创建 3 分区的 Kafka Topics（包含 Snappy 压缩配置）
	ctx := context.Background()
	partitionMap := createKafkaTopicsWithBuilder(ctx, t, eb, topics, 3, 1)
	metrics.PartitionCount = partitionMap
	defer func() {
		// 关闭 EventBus
		eb.Close()

		// 等待 goroutines 完全退出
		t.Logf("⏳ 等待 Kafka EventBus goroutines 退出...")
		time.Sleep(5 * time.Second)

		// 强制 GC
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		// 记录最终资源状态（在 Close() 之后）
		metrics.FinalGoroutines = runtime.NumGoroutine()
		var finalMem runtime.MemStats
		runtime.ReadMemStats(&finalMem)
		metrics.FinalMemoryMB = float64(finalMem.Alloc) / 1024 / 1024
		metrics.MemoryDeltaMB = metrics.FinalMemoryMB - metrics.InitialMemoryMB
		metrics.GoroutineLeak = metrics.FinalGoroutines - metrics.InitialGoroutines

		t.Logf("📊 Kafka 资源清理完成: 初始 %d -> 最终 %d (泄漏 %d)",
			metrics.InitialGoroutines, metrics.FinalGoroutines, metrics.GoroutineLeak)
	}()

	// 🔑 业界最佳实践：在订阅之前设置预订阅 topic 列表
	// 这样可以避免 Kafka Consumer Group 的频繁重平衡，提高性能和稳定性
	// 参考：
	//   - Confluent 官方文档：预订阅模式避免重平衡
	//   - LinkedIn 实践：一次性订阅所有 topic
	//   - Uber 实践：预配置 topic 列表
	//   - sdk/pkg/eventbus/PRE_SUBSCRIPTION_FINAL_REPORT.md
	//   - sdk/pkg/eventbus/pre_subscription_test.go
	if kafkaBus, ok := eb.(interface {
		SetPreSubscriptionTopics([]string)
	}); ok {
		kafkaBus.SetPreSubscriptionTopics(topics)
		t.Logf("✅ 已设置预订阅 topic 列表: %v", topics)
	}

	// 等待连接建立（参考成功测试的等待时间）
	time.Sleep(3 * time.Second)

	// 记录 topic 信息
	metrics.TopicCount = topicCount
	metrics.TopicList = topics
	metrics.ConnectionCount = 1    // Kafka 使用单个连接池
	metrics.ConsumerGroupCount = 1 // 一个消费者组

	// 运行测试 - 在多个 topic 上分发消息
	runPerformanceTestMultiTopic(t, eb, topics, messageCount, timeout, metrics)

	return metrics
}

// runNATSTest 运行 NATS JetStream 性能测试（磁盘持久化）
func runNATSTest(t *testing.T, pressure string, messageCount int, timeout time.Duration) *PerfMetrics {
	metrics := &PerfMetrics{
		System:       "NATS",
		Pressure:     pressure,
		MessageCount: messageCount,
	}

	// 记录初始资源状态
	metrics.InitialGoroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.InitialMemoryMB = float64(m.Alloc) / 1024 / 1024

	// 创建 NATS EventBus（JetStream 磁盘持久化）
	// 🔑 关键修复：不能使用中文字符
	pressureEn := map[string]string{
		"低压": "low",
		"中压": "medium",
		"高压": "high",
		"极限": "extreme",
	}[pressure]
	if pressureEn == "" {
		pressureEn = pressure // 如果不是中文，直接使用
	}

	timestamp := time.Now().Unix()
	streamName := fmt.Sprintf("PERF_%s_%d", pressureEn, timestamp)
	subjectPattern := fmt.Sprintf("nats.perf.%s.%d.>", pressureEn, timestamp) // 使用英文和时间戳确保唯一性

	natsConfig := &eventbus.NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-perf-%s-%d", pressureEn, timestamp),
		MaxReconnects:       10,
		ReconnectWait:       1 * time.Second,
		ConnectionTimeout:   30 * time.Second,
		HealthCheckInterval: 60 * time.Second,
		JetStream: eventbus.JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 10 * time.Second,
			AckWait:        15 * time.Second,
			MaxDeliver:     3,
			Stream: eventbus.StreamConfig{
				Name:      streamName,
				Subjects:  []string{subjectPattern},
				Storage:   "file", // 🔑 关键：磁盘持久化
				Retention: "limits",
				MaxAge:    1 * time.Hour,
				MaxBytes:  1024 * 1024 * 1024, // 1GB
				MaxMsgs:   100000,
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: eventbus.NATSConsumerConfig{
				DurableName:   fmt.Sprintf("perf_%s_%d", pressureEn, timestamp),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 1000,
				MaxWaiting:    500,
				MaxDeliver:    3,
			},
		},
	}

	eb, err := eventbus.NewNATSEventBus(natsConfig)
	require.NoError(t, err, "Failed to create NATS EventBus")
	defer func() {
		// 关闭 EventBus
		eb.Close()

		// 等待 goroutines 完全退出
		t.Logf("⏳ 等待 NATS EventBus goroutines 退出...")
		time.Sleep(5 * time.Second)

		// 强制 GC
		runtime.GC()
		time.Sleep(100 * time.Millisecond)

		// 记录最终资源状态（在 Close() 之后）
		metrics.FinalGoroutines = runtime.NumGoroutine()
		var finalMem runtime.MemStats
		runtime.ReadMemStats(&finalMem)
		metrics.FinalMemoryMB = float64(finalMem.Alloc) / 1024 / 1024
		metrics.MemoryDeltaMB = metrics.FinalMemoryMB - metrics.InitialMemoryMB
		metrics.GoroutineLeak = metrics.FinalGoroutines - metrics.InitialGoroutines

		t.Logf("📊 NATS 资源清理完成: 初始 %d -> 最终 %d (泄漏 %d)",
			metrics.InitialGoroutines, metrics.FinalGoroutines, metrics.GoroutineLeak)
	}()

	// 等待连接建立
	time.Sleep(2 * time.Second)

	// 🔑 要求：创建 5 个 topic (NATS 中称为 subject)
	topicCount := 5
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("nats.perf.%s.%d.topic%d", pressureEn, timestamp, i+1)
	}

	// ⭐ Stream 已在配置中预建立（subject pattern: nats.perf.{pressure}.{timestamp}.>）
	// 无需再调用 ConfigureTopic，避免 subject 重叠错误
	t.Logf("✅ 使用预建立的 Stream: %s (subjects: %v)", streamName, subjectPattern)

	// 记录 topic 信息
	metrics.TopicCount = topicCount
	metrics.TopicList = topics
	metrics.ConnectionCount = 1    // NATS 使用单个连接
	metrics.ConsumerGroupCount = 1 // NATS JetStream 使用一个 durable consumer

	// 运行测试 - 在多个 topic 上分发消息
	runPerformanceTestMultiTopic(t, eb, topics, messageCount, timeout, metrics)

	return metrics
}

// runPerformanceTestMultiTopic 运行多 topic 性能测试核心逻辑
func runPerformanceTestMultiTopic(t *testing.T, eb eventbus.EventBus, topics []string, messageCount int, timeout time.Duration, metrics *PerfMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 聚合ID顺序跟踪（使用高性能分片锁检查器）
	orderChecker := NewOrderChecker()

	// 创建统一的消息处理器
	handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
		receiveTime := time.Now()

		// 更新接收计数
		atomic.AddInt64(&metrics.MessagesReceived, 1)

		// 检查顺序性（线程安全，使用分片锁）
		orderChecker.Check(envelope.AggregateID, envelope.EventVersion)

		// 记录端到端处理延迟（从发送时间到接收时间）
		if !envelope.Timestamp.IsZero() {
			latency := receiveTime.Sub(envelope.Timestamp).Microseconds()
			atomic.AddInt64(&metrics.procLatencySum, latency)
			atomic.AddInt64(&metrics.procLatencyCount, 1)
		}

		// 更新峰值协程数（原子操作）
		currentGoroutines := int32(runtime.NumGoroutine())
		for {
			oldPeak := atomic.LoadInt32(&metrics.PeakGoroutines)
			if currentGoroutines <= oldPeak {
				break
			}
			if atomic.CompareAndSwapInt32(&metrics.PeakGoroutines, oldPeak, currentGoroutines) {
				break
			}
		}

		// 更新峰值内存（原子操作）
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		currentMemoryMB := float64(m.Alloc) / 1024 / 1024
		currentMemoryBits := math.Float64bits(currentMemoryMB)
		for {
			oldPeakBits := atomic.LoadUint64(&metrics.PeakMemoryMB)
			oldPeakMB := math.Float64frombits(oldPeakBits)
			if currentMemoryMB <= oldPeakMB {
				break
			}
			if atomic.CompareAndSwapUint64(&metrics.PeakMemoryMB, oldPeakBits, currentMemoryBits) {
				break
			}
		}

		return nil
	}

	// 🔑 业界最佳实践：并发订阅所有 topic
	// Kafka 的预订阅机制现在支持动态订阅，会自动触发重平衡
	var wg sync.WaitGroup
	for _, topic := range topics {
		wg.Add(1)
		topicName := topic // 避免闭包问题

		go func() {
			defer wg.Done()

			// 使用 SubscribeEnvelope 订阅
			if err := eb.SubscribeEnvelope(ctx, topicName, handler); err != nil {
				t.Logf("⚠️  订阅 topic %s 失败: %v", topicName, err)
				atomic.AddInt64(&metrics.ProcessErrors, 1)
			}
		}()
	}

	// 等待所有订阅完成
	wg.Wait()

	// 等待订阅建立和重平衡完成（参考成功测试的等待时间）
	time.Sleep(5 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()

	// 根据消息数量动态计算聚合ID数量
	// 低压500 -> 50个, 中压2000 -> 200个, 高压5000 -> 500个, 极限10000 -> 1000个
	aggregateCount := messageCount / 10
	if aggregateCount < 50 {
		aggregateCount = 50
	}
	if aggregateCount > 1000 {
		aggregateCount = 1000
	}

	// 生成聚合ID列表
	aggregateIDs := make([]string, aggregateCount)
	for i := 0; i < aggregateCount; i++ {
		aggregateIDs[i] = fmt.Sprintf("aggregate-%d", i)
	}

	// 计算每个聚合ID需要发送的消息数量
	messagesPerAggregate := messageCount / aggregateCount
	remainingMessages := messageCount % aggregateCount

	// 为每个聚合ID创建一个 goroutine 串行发送消息
	// 这样保证同一个聚合ID的消息严格按顺序发送
	var sendWg sync.WaitGroup
	for aggIndex := 0; aggIndex < aggregateCount; aggIndex++ {
		sendWg.Add(1)
		go func(aggregateIndex int) {
			defer sendWg.Done()

			aggregateID := aggregateIDs[aggregateIndex]

			// 计算该聚合ID需要发送的消息数量
			msgCount := messagesPerAggregate
			if aggregateIndex < remainingMessages {
				msgCount++ // 前面的聚合ID多发送一条消息
			}

			// 串行发送该聚合ID的所有消息
			for version := int64(1); version <= int64(msgCount); version++ {
				// 轮询选择 topic
				topic := topics[aggregateIndex%len(topics)]

				// 生成 EventID（格式：AggregateID:EventType:EventVersion:Timestamp）
				eventID := fmt.Sprintf("%s:TestEvent:%d:%d", aggregateID, version, time.Now().UnixNano())

				envelope := &eventbus.Envelope{
					EventID:      eventID,
					AggregateID:  aggregateID,
					EventType:    "TestEvent",
					EventVersion: version, // 严格递增的版本号
					Timestamp:    time.Now(),
					Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"aggregate":"%s","version":%d}`, aggregateID, version)),
				}

				sendStart := time.Now()
				if err := eb.PublishEnvelope(ctx, topic, envelope); err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
				} else {
					atomic.AddInt64(&metrics.MessagesSent, 1)
					// 记录发送延迟
					latency := time.Since(sendStart).Microseconds()
					atomic.AddInt64(&metrics.sendLatencySum, latency)
					atomic.AddInt64(&metrics.sendLatencyCount, 1)
				}
			}
		}(aggIndex)
	}

	// 等待所有发送完成
	sendWg.Wait()
	t.Logf("✅ 发送完成: %d/%d 条消息", metrics.MessagesSent, messageCount)

	// 输出分区信息
	if len(metrics.PartitionCount) > 0 {
		t.Logf("📊 Kafka Topic 分区配置:")
		for topic, partitions := range metrics.PartitionCount {
			t.Logf("   %s: %d partitions", topic, partitions)
		}
	}

	// 等待接收完成或超时（参考成功测试的等待策略）
	t.Logf("⏳ 等待消息处理完成...")
	waitTime := 15 * time.Second
	if messageCount > 2000 {
		waitTime = 30 * time.Second
	}
	time.Sleep(waitTime)

	metrics.EndTime = time.Now()
	metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)

	// 计算性能指标
	durationSeconds := metrics.Duration.Seconds()
	if durationSeconds > 0 {
		metrics.SendThroughput = float64(atomic.LoadInt64(&metrics.MessagesSent)) / durationSeconds
		metrics.ReceiveThroughput = float64(atomic.LoadInt64(&metrics.MessagesReceived)) / durationSeconds
	}

	sendLatencyCount := atomic.LoadInt64(&metrics.sendLatencyCount)
	if sendLatencyCount > 0 {
		sendLatencySum := atomic.LoadInt64(&metrics.sendLatencySum)
		metrics.AvgSendLatency = float64(sendLatencySum) / float64(sendLatencyCount) / 1000.0 // 转换为毫秒
	}

	procLatencyCount := atomic.LoadInt64(&metrics.procLatencyCount)
	if procLatencyCount > 0 {
		procLatencySum := atomic.LoadInt64(&metrics.procLatencySum)
		metrics.AvgProcessLatency = float64(procLatencySum) / float64(procLatencyCount) / 1000.0 // 转换为毫秒
	}

	metrics.SuccessRate = float64(metrics.MessagesReceived) / float64(messageCount) * 100

	// 获取顺序违反次数
	metrics.OrderViolations = orderChecker.GetViolations()

	// 取消订阅
	cancel()

	// 注意：最终资源状态将在 EventBus Close() 之后记录
	// 这样可以准确统计协程泄漏
}

// runPerformanceTest 运行单 topic 性能测试核心逻辑（保留用于兼容）
func runPerformanceTest(t *testing.T, eb eventbus.EventBus, topic string, messageCount int, timeout time.Duration, metrics *PerfMetrics) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 聚合ID顺序跟踪（使用高性能分片锁检查器）
	orderChecker := NewOrderChecker()

	// 使用 SubscribeEnvelope 订阅
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		handler := func(ctx context.Context, envelope *eventbus.Envelope) error {
			receiveTime := time.Now()

			// 更新接收计数
			atomic.AddInt64(&metrics.MessagesReceived, 1)

			// 检查顺序性（线程安全，使用分片锁）
			orderChecker.Check(envelope.AggregateID, envelope.EventVersion)

			// 记录端到端处理延迟（从发送时间到接收时间）
			if !envelope.Timestamp.IsZero() {
				latency := receiveTime.Sub(envelope.Timestamp).Microseconds()
				atomic.AddInt64(&metrics.procLatencySum, latency)
				atomic.AddInt64(&metrics.procLatencyCount, 1)
			}

			return nil
		}

		// 🔑 关键：使用 SubscribeEnvelope 方法
		err := eb.(eventbus.EnvelopeSubscriber).SubscribeEnvelope(ctx, topic, handler)
		if err != nil {
			t.Logf("❌ Subscribe error: %v", err)
			atomic.AddInt64(&metrics.ProcessErrors, 1)
		}
	}()

	// 等待订阅建立（参考成功测试的等待时间）
	time.Sleep(3 * time.Second)

	// 开始发送消息
	metrics.StartTime = time.Now()

	// 根据消息数量动态计算聚合ID数量
	// 低压500 -> 50个, 中压2000 -> 200个, 高压5000 -> 500个, 极限10000 -> 1000个
	aggregateCount := messageCount / 10
	if aggregateCount < 50 {
		aggregateCount = 50
	}
	if aggregateCount > 1000 {
		aggregateCount = 1000
	}

	// 生成聚合ID列表
	aggregateIDs := make([]string, aggregateCount)
	for i := 0; i < aggregateCount; i++ {
		aggregateIDs[i] = fmt.Sprintf("aggregate-%d", i)
	}

	// 计算每个聚合ID需要发送的消息数量
	messagesPerAggregate := messageCount / aggregateCount
	remainingMessages := messageCount % aggregateCount

	// 为每个聚合ID创建一个 goroutine 串行发送消息
	// 这样保证同一个聚合ID的消息严格按顺序发送
	var sendWg sync.WaitGroup
	for aggIndex := 0; aggIndex < aggregateCount; aggIndex++ {
		sendWg.Add(1)
		go func(aggregateIndex int) {
			defer sendWg.Done()

			aggregateID := aggregateIDs[aggregateIndex]

			// 计算该聚合ID需要发送的消息数量
			msgCount := messagesPerAggregate
			if aggregateIndex < remainingMessages {
				msgCount++ // 前面的聚合ID多发送一条消息
			}

			// 串行发送该聚合ID的所有消息
			for version := int64(1); version <= int64(msgCount); version++ {
				// 生成 EventID（格式：AggregateID:EventType:EventVersion:Timestamp）
				eventID := fmt.Sprintf("%s:PerformanceTestEvent:%d:%d", aggregateID, version, time.Now().UnixNano())

				// 创建 Envelope
				envelope := &eventbus.Envelope{
					EventID:      eventID,
					AggregateID:  aggregateID,
					EventType:    "PerformanceTestEvent",
					EventVersion: version, // 严格递增的版本号
					Timestamp:    time.Now(),
					Payload:      jxtjson.RawMessage(fmt.Sprintf(`{"aggregate":"%s","version":%d}`, aggregateID, version)),
				}

				// 🔑 关键：使用 PublishEnvelope 方法
				sendStart := time.Now()
				err := eb.(eventbus.EnvelopePublisher).PublishEnvelope(ctx, topic, envelope)
				sendLatency := time.Since(sendStart).Microseconds()

				if err != nil {
					atomic.AddInt64(&metrics.SendErrors, 1)
				} else {
					atomic.AddInt64(&metrics.MessagesSent, 1)
					atomic.AddInt64(&metrics.sendLatencySum, sendLatency)
					atomic.AddInt64(&metrics.sendLatencyCount, 1)
				}

				// 更新峰值资源
				updatePeakResources(metrics)
			}
		}(aggIndex)
	}

	// 等待发送完成
	sendWg.Wait()
	t.Logf("✅ 发送完成: %d/%d 条消息", atomic.LoadInt64(&metrics.MessagesSent), messageCount)

	// 等待接收完成或超时（参考成功测试的等待策略）
	t.Logf("⏳ 等待消息处理完成...")
	waitTime := 15 * time.Second
	if messageCount > 2000 {
		waitTime = 30 * time.Second
	}
	time.Sleep(waitTime)

	metrics.EndTime = time.Now()
	metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)

	// 计算最终指标
	calculateFinalMetrics(metrics)

	// 记录最终资源状态
	metrics.FinalGoroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.FinalMemoryMB = float64(m.Alloc) / 1024 / 1024
	metrics.GoroutineLeak = metrics.FinalGoroutines - metrics.InitialGoroutines

	// 获取顺序违反次数
	metrics.OrderViolations = orderChecker.GetViolations()

	// 取消上下文，停止订阅
	cancel()
	time.Sleep(1 * time.Second)
}

// updatePeakResources 更新峰值资源占用（原子操作）
func updatePeakResources(metrics *PerfMetrics) {
	// 更新峰值协程数
	currentGoroutines := int32(runtime.NumGoroutine())
	for {
		oldPeak := atomic.LoadInt32(&metrics.PeakGoroutines)
		if currentGoroutines <= oldPeak {
			break
		}
		if atomic.CompareAndSwapInt32(&metrics.PeakGoroutines, oldPeak, currentGoroutines) {
			break
		}
	}

	// 更新峰值内存
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memoryMB := float64(m.Alloc) / 1024 / 1024
	memoryBits := math.Float64bits(memoryMB)
	for {
		oldPeakBits := atomic.LoadUint64(&metrics.PeakMemoryMB)
		oldPeakMB := math.Float64frombits(oldPeakBits)
		if memoryMB <= oldPeakMB {
			break
		}
		if atomic.CompareAndSwapUint64(&metrics.PeakMemoryMB, oldPeakBits, memoryBits) {
			break
		}
	}
}

// calculateFinalMetrics 计算最终性能指标
func calculateFinalMetrics(metrics *PerfMetrics) {
	// 成功率
	messagesSent := atomic.LoadInt64(&metrics.MessagesSent)
	messagesReceived := atomic.LoadInt64(&metrics.MessagesReceived)
	if messagesSent > 0 {
		metrics.SuccessRate = float64(messagesReceived) / float64(messagesSent) * 100
	}

	// 吞吐量
	if metrics.Duration.Seconds() > 0 {
		metrics.SendThroughput = float64(messagesSent) / metrics.Duration.Seconds()
		metrics.ReceiveThroughput = float64(messagesReceived) / metrics.Duration.Seconds()
	}

	// 平均延迟
	sendLatencyCount := atomic.LoadInt64(&metrics.sendLatencyCount)
	if sendLatencyCount > 0 {
		sendLatencySum := atomic.LoadInt64(&metrics.sendLatencySum)
		metrics.AvgSendLatency = float64(sendLatencySum) / float64(sendLatencyCount) / 1000.0 // ms
	}
	procLatencyCount := atomic.LoadInt64(&metrics.procLatencyCount)
	if procLatencyCount > 0 {
		procLatencySum := atomic.LoadInt64(&metrics.procLatencySum)
		metrics.AvgProcessLatency = float64(procLatencySum) / float64(procLatencyCount) / 1000.0 // ms
	}

	// 资源增量
	peakMemoryMB := math.Float64frombits(atomic.LoadUint64(&metrics.PeakMemoryMB))
	metrics.MemoryDeltaMB = peakMemoryMB - metrics.InitialMemoryMB
}

// compareRoundResults 对比单轮测试结果
func compareRoundResults(t *testing.T, pressure string, kafka, nats *PerfMetrics) {
	t.Log("\n" + strings.Repeat("=", 80))
	t.Logf("📊 %s 测试结果对比", pressure)
	t.Log(strings.Repeat("=", 80))

	// 消息统计对比
	t.Logf("\n📨 消息统计:")
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "发送消息数", kafka.MessagesSent, nats.MessagesSent)
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "接收消息数", kafka.MessagesReceived, nats.MessagesReceived)
	t.Logf("   %-20s | Kafka: %7.2f%% | NATS: %7.2f%%", "成功率", kafka.SuccessRate, nats.SuccessRate)
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "发送错误", kafka.SendErrors, nats.SendErrors)
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "处理错误", kafka.ProcessErrors, nats.ProcessErrors)
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "顺序违反", kafka.OrderViolations, nats.OrderViolations)

	// 性能指标对比
	t.Logf("\n🚀 性能指标:")
	t.Logf("   %-20s | Kafka: %10.2f msg/s | NATS: %10.2f msg/s", "发送吞吐量", kafka.SendThroughput, nats.SendThroughput)
	t.Logf("   %-20s | Kafka: %10.2f msg/s | NATS: %10.2f msg/s", "接收吞吐量", kafka.ReceiveThroughput, nats.ReceiveThroughput)
	t.Logf("   %-20s | Kafka: %10.3f ms | NATS: %10.3f ms", "平均发送延迟", kafka.AvgSendLatency, nats.AvgSendLatency)
	t.Logf("   %-20s | Kafka: %10.3f ms | NATS: %10.3f ms", "平均处理延迟", kafka.AvgProcessLatency, nats.AvgProcessLatency)
	t.Logf("   %-20s | Kafka: %10.2f s | NATS: %10.2f s", "总耗时", kafka.Duration.Seconds(), nats.Duration.Seconds())

	// 资源占用对比
	t.Logf("\n💾 资源占用:")
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "初始协程数", kafka.InitialGoroutines, nats.InitialGoroutines)
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "峰值协程数", atomic.LoadInt32(&kafka.PeakGoroutines), atomic.LoadInt32(&nats.PeakGoroutines))
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "最终协程数", kafka.FinalGoroutines, nats.FinalGoroutines)
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "协程泄漏", kafka.GoroutineLeak, nats.GoroutineLeak)
	t.Logf("   %-20s | Kafka: %10.2f MB | NATS: %10.2f MB", "初始内存", kafka.InitialMemoryMB, nats.InitialMemoryMB)
	t.Logf("   %-20s | Kafka: %10.2f MB | NATS: %10.2f MB", "峰值内存", math.Float64frombits(atomic.LoadUint64(&kafka.PeakMemoryMB)), math.Float64frombits(atomic.LoadUint64(&nats.PeakMemoryMB)))
	t.Logf("   %-20s | Kafka: %10.2f MB | NATS: %10.2f MB", "最终内存", kafka.FinalMemoryMB, nats.FinalMemoryMB)
	t.Logf("   %-20s | Kafka: %10.2f MB | NATS: %10.2f MB", "内存增量", kafka.MemoryDeltaMB, nats.MemoryDeltaMB)

	// 连接和消费者组统计（新增）
	t.Logf("\n🔗 连接和消费者组:")
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "Topic 数量", kafka.TopicCount, nats.TopicCount)
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "连接数", kafka.ConnectionCount, nats.ConnectionCount)
	t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "消费者组个数", kafka.ConsumerGroupCount, nats.ConsumerGroupCount)
	if len(kafka.TopicList) > 0 {
		t.Logf("   Kafka Topics: %v", kafka.TopicList)
	}

	// 输出 Kafka 分区信息
	if len(kafka.PartitionCount) > 0 {
		t.Logf("   Kafka Partitions:")
		for topic, partitions := range kafka.PartitionCount {
			t.Logf("      %s: %d partitions", topic, partitions)
		}
	}

	if len(nats.TopicList) > 0 {
		t.Logf("   NATS Topics: %v", nats.TopicList)
	}

	// 性能优势分析
	t.Logf("\n🏆 性能优势:")
	if kafka.ReceiveThroughput > nats.ReceiveThroughput {
		improvement := (kafka.ReceiveThroughput - nats.ReceiveThroughput) / nats.ReceiveThroughput * 100
		t.Logf("   Kafka 吞吐量优势: +%.2f%%", improvement)
	} else {
		improvement := (nats.ReceiveThroughput - kafka.ReceiveThroughput) / kafka.ReceiveThroughput * 100
		t.Logf("   NATS 吞吐量优势: +%.2f%%", improvement)
	}

	if kafka.AvgProcessLatency < nats.AvgProcessLatency {
		improvement := (nats.AvgProcessLatency - kafka.AvgProcessLatency) / nats.AvgProcessLatency * 100
		t.Logf("   Kafka 延迟优势: -%.2f%%", improvement)
	} else {
		improvement := (kafka.AvgProcessLatency - nats.AvgProcessLatency) / kafka.AvgProcessLatency * 100
		t.Logf("   NATS 延迟优势: -%.2f%%", improvement)
	}

	if kafka.MemoryDeltaMB < nats.MemoryDeltaMB {
		improvement := (nats.MemoryDeltaMB - kafka.MemoryDeltaMB) / nats.MemoryDeltaMB * 100
		t.Logf("   Kafka 内存优势: -%.2f%%", improvement)
	} else {
		improvement := (kafka.MemoryDeltaMB - nats.MemoryDeltaMB) / kafka.MemoryDeltaMB * 100
		t.Logf("   NATS 内存优势: -%.2f%%", improvement)
	}

	t.Log(strings.Repeat("=", 80))

	// 基本断言
	assert.Greater(t, kafka.SuccessRate, 90.0, "Kafka 成功率应该 > 90%")
	assert.Greater(t, nats.SuccessRate, 90.0, "NATS 成功率应该 > 90%")
	assert.Equal(t, int64(0), kafka.OrderViolations, "Kafka 不应该有顺序违反")
	assert.Equal(t, int64(0), nats.OrderViolations, "NATS 不应该有顺序违反")
}

// generateComparisonReport 生成综合对比报告
func generateComparisonReport(t *testing.T, allResults map[string][]*PerfMetrics) {
	t.Log("\n" + strings.Repeat("=", 100))
	t.Log("🏆 Kafka vs NATS JetStream 综合性能对比报告")
	t.Log(strings.Repeat("=", 100))

	kafkaResults := allResults["Kafka"]
	natsResults := allResults["NATS"]

	if len(kafkaResults) == 0 || len(natsResults) == 0 {
		t.Logf("❌ 测试数据不足，无法生成报告")
		return
	}

	// 汇总表格
	t.Logf("\n📊 性能指标汇总表:")
	t.Logf("%-10s | %-15s | %-15s | %-15s | %-15s | %-15s | %-15s",
		"压力级别", "系统", "吞吐量(msg/s)", "延迟(ms)", "成功率(%)", "内存增量(MB)", "协程泄漏")
	t.Log(strings.Repeat("-", 100))

	for i := 0; i < len(kafkaResults); i++ {
		kafka := kafkaResults[i]
		nats := natsResults[i]

		t.Logf("%-10s | %-15s | %15.2f | %15.3f | %15.2f | %15.2f | %15d",
			kafka.Pressure, "Kafka", kafka.ReceiveThroughput, kafka.AvgProcessLatency,
			kafka.SuccessRate, kafka.MemoryDeltaMB, kafka.GoroutineLeak)
		t.Logf("%-10s | %-15s | %15.2f | %15.3f | %15.2f | %15.2f | %15d",
			nats.Pressure, "NATS", nats.ReceiveThroughput, nats.AvgProcessLatency,
			nats.SuccessRate, nats.MemoryDeltaMB, nats.GoroutineLeak)
		t.Log(strings.Repeat("-", 100))
	}

	// 计算平均值
	var kafkaAvgThroughput, kafkaAvgLatency, kafkaAvgMemory float64
	var natsAvgThroughput, natsAvgLatency, natsAvgMemory float64

	for i := 0; i < len(kafkaResults); i++ {
		kafkaAvgThroughput += kafkaResults[i].ReceiveThroughput
		kafkaAvgLatency += kafkaResults[i].AvgProcessLatency
		kafkaAvgMemory += kafkaResults[i].MemoryDeltaMB

		natsAvgThroughput += natsResults[i].ReceiveThroughput
		natsAvgLatency += natsResults[i].AvgProcessLatency
		natsAvgMemory += natsResults[i].MemoryDeltaMB
	}

	count := float64(len(kafkaResults))
	kafkaAvgThroughput /= count
	kafkaAvgLatency /= count
	kafkaAvgMemory /= count
	natsAvgThroughput /= count
	natsAvgLatency /= count
	natsAvgMemory /= count

	// 综合评分
	t.Logf("\n🎯 综合评分:")
	t.Logf("%-20s | Kafka: %10.2f msg/s | NATS: %10.2f msg/s", "平均吞吐量", kafkaAvgThroughput, natsAvgThroughput)
	t.Logf("%-20s | Kafka: %10.3f ms | NATS: %10.3f ms", "平均延迟", kafkaAvgLatency, natsAvgLatency)
	t.Logf("%-20s | Kafka: %10.2f MB | NATS: %10.2f MB", "平均内存增量", kafkaAvgMemory, natsAvgMemory)

	// 连接和消费者组统计（新增）
	if len(kafkaResults) > 0 && len(natsResults) > 0 {
		t.Logf("\n🔗 连接和消费者组统计:")
		t.Logf("%-20s | Kafka: %8d | NATS: %8d", "Topic 数量", kafkaResults[0].TopicCount, natsResults[0].TopicCount)
		t.Logf("%-20s | Kafka: %8d | NATS: %8d", "连接数", kafkaResults[0].ConnectionCount, natsResults[0].ConnectionCount)
		t.Logf("%-20s | Kafka: %8d | NATS: %8d", "消费者组个数", kafkaResults[0].ConsumerGroupCount, natsResults[0].ConsumerGroupCount)
	}

	// 获胜者判定
	t.Logf("\n🏆 最终结论:")

	kafkaWins := 0
	natsWins := 0

	if kafkaAvgThroughput > natsAvgThroughput {
		kafkaWins++
		t.Logf("   ✅ 吞吐量: Kafka 胜出 (+%.2f%%)",
			(kafkaAvgThroughput-natsAvgThroughput)/natsAvgThroughput*100)
	} else {
		natsWins++
		t.Logf("   ✅ 吞吐量: NATS 胜出 (+%.2f%%)",
			(natsAvgThroughput-kafkaAvgThroughput)/kafkaAvgThroughput*100)
	}

	if kafkaAvgLatency < natsAvgLatency {
		kafkaWins++
		t.Logf("   ✅ 延迟: Kafka 胜出 (-%.2f%%)",
			(natsAvgLatency-kafkaAvgLatency)/natsAvgLatency*100)
	} else {
		natsWins++
		t.Logf("   ✅ 延迟: NATS 胜出 (-%.2f%%)",
			(kafkaAvgLatency-natsAvgLatency)/kafkaAvgLatency*100)
	}

	if kafkaAvgMemory < natsAvgMemory {
		kafkaWins++
		t.Logf("   ✅ 内存效率: Kafka 胜出 (-%.2f%%)",
			(natsAvgMemory-kafkaAvgMemory)/natsAvgMemory*100)
	} else {
		natsWins++
		t.Logf("   ✅ 内存效率: NATS 胜出 (-%.2f%%)",
			(kafkaAvgMemory-natsAvgMemory)/kafkaAvgMemory*100)
	}

	t.Logf("\n🎉 总体获胜者:")
	if kafkaWins > natsWins {
		t.Logf("   🥇 Kafka 以 %d:%d 获胜！", kafkaWins, natsWins)
	} else if natsWins > kafkaWins {
		t.Logf("   🥇 NATS 以 %d:%d 获胜！", natsWins, kafkaWins)
	} else {
		t.Logf("   🤝 平局 %d:%d", kafkaWins, natsWins)
	}

	// 使用建议
	t.Logf("\n💡 使用建议:")
	t.Logf("🔴 选择 Kafka 当:")
	t.Logf("   • 需要企业级可靠性和持久化保证")
	t.Logf("   • 需要复杂的数据处理管道")
	t.Logf("   • 需要成熟的生态系统和工具链")
	t.Logf("   • 数据量大且需要长期保存")

	t.Logf("\n🔵 选择 NATS JetStream 当:")
	t.Logf("   • 需要极致的性能和低延迟")
	t.Logf("   • 需要简单的部署和运维")
	t.Logf("   • 需要轻量级的消息传递")
	t.Logf("   • 构建实时应用和微服务")

	t.Log("\n" + strings.Repeat("=", 100))
	t.Logf("✅ 测试完成！共执行 %d 个场景，%d 次测试", len(kafkaResults), len(kafkaResults)*2)
	t.Log(strings.Repeat("=", 100))
}
