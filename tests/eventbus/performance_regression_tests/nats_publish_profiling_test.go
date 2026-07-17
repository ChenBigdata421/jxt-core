package performance_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// TestNATSPublishEnvelopeProfiler 详细分析 PublishEnvelope 的性能瓶颈
func TestNATSPublishEnvelopeProfiler(t *testing.T) {
	// 创建 NATS EventBus
	config := &eventbus.EventBusConfig{
		Type: "nats",
		NATS: eventbus.NATSConfig{
			URLs:     []string{"nats://localhost:4223"},
			ClientID: "profiler-test",
			JetStream: eventbus.JetStreamConfig{
				Enabled: true,
				Stream: eventbus.StreamConfig{
					Name:     "PROFILER_STREAM",
					Subjects: []string{"profiler.*"},
					Storage:  "file",
					Replicas: 1,
				},
				Consumer: eventbus.NATSConsumerConfig{
					DurableName:  "profiler-consumer",
					AckPolicy:    "explicit",
					ReplayPolicy: "instant",
				},
			},
		},
	}

	bus, err := eventbus.NewEventBus(config)
	require.NoError(t, err)
	defer bus.Close()
	// Clean up the JetStream stream so it doesn't accumulate on the shared broker
	// (leftover streams cause "subjects overlap" failures on subsequent runs).
	defer func() {
		if nc, err := nats.Connect(config.NATS.URLs[0]); err == nil {
			defer nc.Close()
			if js, err := nc.JetStream(); err == nil {
				_ = js.DeleteStream("PROFILER_STREAM")
			}
		}
	}()

	ctx := context.Background()
	topic := "profiler.test"

	// 预热（避免首次调用的初始化开销）
	t.Log("🔥 预热阶段...")
	for i := 0; i < 100; i++ {
		envelope := eventbus.NewEnvelopeWithAutoID(
			fmt.Sprintf("warmup-%d", i),
			"WarmupEvent",
			1,
			[]byte("warmup"),
		)
		_ = bus.PublishEnvelope(ctx, topic, envelope)
	}
	time.Sleep(1 * time.Second)

	// 测试阶段：详细计时
	t.Log("📊 开始性能分析...")
	messageCount := 1000

	// 分段计时
	type Timing struct {
		Total     time.Duration
		Validate  time.Duration
		Serialize time.Duration
		Publish   time.Duration
		Other     time.Duration
	}

	timings := make([]Timing, messageCount)

	for i := 0; i < messageCount; i++ {
		envelope := eventbus.NewEnvelopeWithAutoID(
			fmt.Sprintf("agg-%d", i%100),
			"TestEvent",
			int64(i/100+1),
			[]byte(fmt.Sprintf("message %d", i)),
		)

		// 总时间
		totalStart := time.Now()

		// 1. 校验时间
		validateStart := time.Now()
		err := envelope.Validate()
		validateDuration := time.Since(validateStart)
		require.NoError(t, err)

		// 2. 序列化时间
		serializeStart := time.Now()
		_, err = envelope.ToBytes()
		serializeDuration := time.Since(serializeStart)
		require.NoError(t, err)

		// 3. 发布时间（包含所有 PublishEnvelope 的开销）
		publishStart := time.Now()
		err = bus.PublishEnvelope(ctx, topic, envelope)
		publishDuration := time.Since(publishStart)
		require.NoError(t, err)

		totalDuration := time.Since(totalStart)

		timings[i] = Timing{
			Total:     totalDuration,
			Validate:  validateDuration,
			Serialize: serializeDuration,
			Publish:   publishDuration,
			Other:     totalDuration - validateDuration - serializeDuration - publishDuration,
		}
	}

	// 统计分析
	var (
		totalSum     int64
		validateSum  int64
		serializeSum int64
		publishSum   int64
		otherSum     int64
	)

	for _, timing := range timings {
		totalSum += timing.Total.Microseconds()
		validateSum += timing.Validate.Microseconds()
		serializeSum += timing.Serialize.Microseconds()
		publishSum += timing.Publish.Microseconds()
		otherSum += timing.Other.Microseconds()
	}

	avgTotal := float64(totalSum) / float64(messageCount) / 1000.0         // ms
	avgValidate := float64(validateSum) / float64(messageCount) / 1000.0   // ms
	avgSerialize := float64(serializeSum) / float64(messageCount) / 1000.0 // ms
	avgPublish := float64(publishSum) / float64(messageCount) / 1000.0     // ms
	avgOther := float64(otherSum) / float64(messageCount) / 1000.0         // ms

	// 输出结果
	t.Log("================================================================================")
	t.Log("📊 NATS PublishEnvelope 性能分析报告")
	t.Log("================================================================================")
	t.Logf("消息数量: %d", messageCount)
	t.Log("")
	t.Log("⏱️  平均耗时分解:")
	t.Logf("   总耗时:        %.3f ms (100.00%%)", avgTotal)
	t.Logf("   ├─ Validate:   %.3f ms (%.2f%%)", avgValidate, avgValidate/avgTotal*100)
	t.Logf("   ├─ Serialize:  %.3f ms (%.2f%%)", avgSerialize, avgSerialize/avgTotal*100)
	t.Logf("   ├─ Publish:    %.3f ms (%.2f%%)", avgPublish, avgPublish/avgTotal*100)
	t.Logf("   └─ Other:      %.3f ms (%.2f%%)", avgOther, avgOther/avgTotal*100)
	t.Log("")

	// 百分位数分析
	type Percentile struct {
		P50 time.Duration
		P90 time.Duration
		P95 time.Duration
		P99 time.Duration
		Max time.Duration
	}

	calculatePercentile := func(durations []time.Duration) Percentile {
		// 简单排序
		sorted := make([]time.Duration, len(durations))
		copy(sorted, durations)
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[i] > sorted[j] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}

		return Percentile{
			P50: sorted[len(sorted)*50/100],
			P90: sorted[len(sorted)*90/100],
			P95: sorted[len(sorted)*95/100],
			P99: sorted[len(sorted)*99/100],
			Max: sorted[len(sorted)-1],
		}
	}

	// 提取各阶段的耗时
	totalDurations := make([]time.Duration, messageCount)
	publishDurations := make([]time.Duration, messageCount)
	for i := 0; i < messageCount; i++ {
		totalDurations[i] = timings[i].Total
		publishDurations[i] = timings[i].Publish
	}

	totalPercentile := calculatePercentile(totalDurations)
	publishPercentile := calculatePercentile(publishDurations)

	t.Log("📈 百分位数分析 (总耗时):")
	t.Logf("   P50:  %.3f ms", float64(totalPercentile.P50.Microseconds())/1000.0)
	t.Logf("   P90:  %.3f ms", float64(totalPercentile.P90.Microseconds())/1000.0)
	t.Logf("   P95:  %.3f ms", float64(totalPercentile.P95.Microseconds())/1000.0)
	t.Logf("   P99:  %.3f ms", float64(totalPercentile.P99.Microseconds())/1000.0)
	t.Logf("   Max:  %.3f ms", float64(totalPercentile.Max.Microseconds())/1000.0)
	t.Log("")

	t.Log("📈 百分位数分析 (Publish 耗时):")
	t.Logf("   P50:  %.3f ms", float64(publishPercentile.P50.Microseconds())/1000.0)
	t.Logf("   P90:  %.3f ms", float64(publishPercentile.P90.Microseconds())/1000.0)
	t.Logf("   P95:  %.3f ms", float64(publishPercentile.P95.Microseconds())/1000.0)
	t.Logf("   P99:  %.3f ms", float64(publishPercentile.P99.Microseconds())/1000.0)
	t.Logf("   Max:  %.3f ms", float64(publishPercentile.Max.Microseconds())/1000.0)
	t.Log("")

	// 瓶颈分析
	t.Log("🔍 瓶颈分析:")
	maxComponent := "Publish"
	maxValue := avgPublish
	if avgValidate > maxValue {
		maxComponent = "Validate"
		maxValue = avgValidate
	}
	if avgSerialize > maxValue {
		maxComponent = "Serialize"
		maxValue = avgSerialize
	}
	if avgOther > maxValue {
		maxComponent = "Other"
		maxValue = avgOther
	}

	t.Logf("   最大瓶颈: %s (%.3f ms, %.2f%%)", maxComponent, maxValue, maxValue/avgTotal*100)
	t.Log("")

	// 优化建议
	t.Log("💡 优化建议:")
	if avgValidate > 0.1 {
		t.Log("   ⚠️  Validate 耗时较高，考虑缓存校验结果或简化校验逻辑")
	}
	if avgSerialize > 0.5 {
		t.Log("   ⚠️  Serialize 耗时较高，考虑使用更快的序列化库（如 msgpack）")
	}
	if avgPublish > 1.0 {
		t.Log("   ⚠️  Publish 耗时较高，可能是 NATS SDK 的 PublishMsgAsync 开销")
		t.Log("       建议检查:")
		t.Log("       - NATS 服务器是否在本地（减少网络延迟）")
		t.Log("       - PublishAsyncMaxPending 配置是否合理")
		t.Log("       - 是否有网络拥塞或服务器负载过高")
	}
	t.Log("================================================================================")

	// 等待消息处理完成
	time.Sleep(2 * time.Second)
}

// TestKafkaPublishEnvelopeProfiler Kafka 对比测试
func TestKafkaPublishEnvelopeProfiler(t *testing.T) {
	// 创建 Kafka EventBus
	config := &eventbus.EventBusConfig{
		Type: "kafka",
		Kafka: eventbus.KafkaConfig{
			Brokers:  []string{"localhost:29092"},
			ClientID: "kafka-profiler-test",
			Producer: eventbus.ProducerConfig{
				RequiredAcks:   1,
				FlushFrequency: 500 * time.Millisecond,
				FlushMessages:  100,
				RetryMax:       3,
				// 注意：压缩配置已从 Producer 级别移到 Topic 级别，通过 TopicBuilder 配置
				Timeout: 10 * time.Second,
			},
			Consumer: eventbus.ConsumerConfig{
				GroupID:         "kafka-profiler-group",
				AutoOffsetReset: "earliest",
			},
		},
	}

	bus, err := eventbus.NewEventBus(config)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "kafka-profiler-test"

	// 预热
	t.Log("🔥 预热阶段...")
	for i := 0; i < 100; i++ {
		envelope := eventbus.NewEnvelopeWithAutoID(
			fmt.Sprintf("warmup-%d", i),
			"WarmupEvent",
			1,
			[]byte("warmup"),
		)
		_ = bus.PublishEnvelope(ctx, topic, envelope)
	}
	time.Sleep(1 * time.Second)

	// 测试阶段
	t.Log("📊 开始性能分析...")
	messageCount := 1000
	publishDurations := make([]time.Duration, messageCount)

	for i := 0; i < messageCount; i++ {
		envelope := eventbus.NewEnvelopeWithAutoID(
			fmt.Sprintf("agg-%d", i%100),
			"TestEvent",
			int64(i/100+1),
			[]byte(fmt.Sprintf("message %d", i)),
		)

		publishStart := time.Now()
		err = bus.PublishEnvelope(ctx, topic, envelope)
		publishDurations[i] = time.Since(publishStart)
		require.NoError(t, err)
	}

	// 统计
	var publishSum int64
	for _, d := range publishDurations {
		publishSum += d.Microseconds()
	}
	avgPublish := float64(publishSum) / float64(messageCount) / 1000.0 // ms

	t.Log("================================================================================")
	t.Log("📊 Kafka PublishEnvelope 性能分析报告")
	t.Log("================================================================================")
	t.Logf("消息数量: %d", messageCount)
	t.Logf("平均 Publish 耗时: %.3f ms", avgPublish)
	t.Log("================================================================================")

	time.Sleep(2 * time.Second)
}
