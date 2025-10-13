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

// TestNATSStage2Pressure 第二阶段优化压力测试
// 🎯 目标：验证异步发布 + 全局Worker池的优化效果
// 📊 预期：吞吐量3600-9500 msg/s，Goroutine数量降至100-200
func TestNATSStage2Pressure(t *testing.T) {
	// 🔧 测试配置
	const (
		testDuration    = 30 * time.Second // 测试持续时间
		messageSize     = 1024             // 消息大小（1KB）
		publisherCount  = 10               // 发布者数量
		subscriberCount = 5                // 订阅者数量
		topicCount      = 3                // Topic数量
	)

	// 📊 性能指标
	var (
		publishedCount    int64
		consumedCount     int64
		errorCount        int64
		startTime         time.Time
		endTime           time.Time
		initialGoroutines int
		peakGoroutines    int
	)

	// ✅ 第二阶段优化配置（使用本地NATS服务器）
	timestamp := time.Now().UnixNano()
	config := &NATSConfig{
		URLs:                []string{"nats://localhost:4223"},
		ClientID:            fmt.Sprintf("nats-stage2-pressure-test-%d", timestamp),
		MaxReconnects:       5,
		ReconnectWait:       2 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
		JetStream: JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 100 * time.Millisecond, // ✅ 优化3: 缩短超时
			AckWait:        30 * time.Second,
			MaxDeliver:     3,
			Stream: StreamConfig{
				Name:      fmt.Sprintf("STAGE2_STREAM_%d", timestamp),
				Subjects:  []string{fmt.Sprintf("stage2@%d.>", timestamp)},
				Retention: "limits",
				Storage:   "file",
				Replicas:  1,
				MaxAge:    30 * time.Minute,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   1000000,
				Discard:   "old",
			},
			Consumer: NATSConsumerConfig{
				DurableName:   fmt.Sprintf("stage2_consumer_%d", timestamp),
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 10000, // ✅ 优化8: 增大到10000
				MaxWaiting:    1000,  // ✅ 优化8: 增大到1000
				MaxDeliver:    3,
			},
		},
	}

	// 创建EventBus实例
	bus, err := NewNATSEventBus(config)
	require.NoError(t, err)
	defer bus.Close()

	// 等待连接稳定
	time.Sleep(2 * time.Second)

	// 📊 记录初始Goroutine数量
	initialGoroutines = runtime.NumGoroutine()
	t.Logf("🔍 Initial Goroutines: %d", initialGoroutines)

	// 🎯 创建测试Topic（确保不会被识别为有聚合ID）
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		// 使用包含多个特殊字符的topic名称，确保所有段都包含无效字符
		// 这样ExtractAggregateID就无法从任何段中提取有效的聚合ID
		topics[i] = fmt.Sprintf("stage2@%d.test@%d.msg#%d", timestamp, i, i)
	}

	// 📝 创建测试消息
	testMessage := make([]byte, messageSize)
	for i := range testMessage {
		testMessage[i] = byte(i % 256)
	}

	// 🎯 设置订阅者（每个订阅者订阅一个topic，避免重复订阅）
	var subscriberWg sync.WaitGroup
	for i := 0; i < subscriberCount; i++ {
		subscriberWg.Add(1)
		go func(subscriberID int) {
			defer subscriberWg.Done()

			// 每个订阅者订阅一个topic（轮询分配）
			topicIndex := subscriberID % len(topics)
			topic := topics[topicIndex]

			err := bus.Subscribe(context.Background(), topic, func(ctx context.Context, data []byte) error {
				atomic.AddInt64(&consumedCount, 1)

				// 模拟消息处理（轻量级）
				if len(data) != messageSize {
					atomic.AddInt64(&errorCount, 1)
					return fmt.Errorf("invalid message size: expected %d, got %d", messageSize, len(data))
				}

				return nil
			})

			if err != nil {
				t.Errorf("Subscriber %d failed to subscribe to topic %s: %v", subscriberID, topic, err)
				atomic.AddInt64(&errorCount, 1)
			}
		}(i)
	}

	// 等待订阅者准备就绪
	subscriberWg.Wait()
	time.Sleep(2 * time.Second)

	// 📊 开始性能测试
	t.Logf("🚀 Starting NATS Stage2 pressure test...")
	t.Logf("📊 Test config: %d publishers, %d subscribers, %d topics, %d seconds",
		publisherCount, subscriberCount, topicCount, int(testDuration.Seconds()))

	startTime = time.Now()

	// 🎯 启动发布者
	var publisherWg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	for i := 0; i < publisherCount; i++ {
		publisherWg.Add(1)
		go func(publisherID int) {
			defer publisherWg.Done()

			ticker := time.NewTicker(10 * time.Millisecond) // 每10ms发布一次
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// 轮询发布到不同topic
					topic := topics[int(atomic.LoadInt64(&publishedCount))%topicCount]

					err := bus.Publish(ctx, topic, testMessage)
					if err != nil {
						atomic.AddInt64(&errorCount, 1)
						t.Logf("Publisher %d failed to publish: %v", publisherID, err)
					} else {
						atomic.AddInt64(&publishedCount, 1)
					}
				}
			}
		}(i)
	}

	// 📊 监控Goroutine数量
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := runtime.NumGoroutine()
				if current > peakGoroutines {
					peakGoroutines = current
				}
			}
		}
	}()

	// 等待测试完成
	publisherWg.Wait()
	endTime = time.Now()

	// 等待消息处理完成
	time.Sleep(5 * time.Second)

	// 📊 收集最终统计
	finalPublished := atomic.LoadInt64(&publishedCount)
	finalConsumed := atomic.LoadInt64(&consumedCount)
	finalErrors := atomic.LoadInt64(&errorCount)
	finalGoroutines := runtime.NumGoroutine()

	duration := endTime.Sub(startTime)
	publishThroughput := float64(finalPublished) / duration.Seconds()
	consumeThroughput := float64(finalConsumed) / duration.Seconds()

	// 📊 输出测试结果
	t.Logf("\n"+
		"🎯 ===== NATS Stage2 Pressure Test Results =====\n"+
		"⏱️  Duration: %.2f seconds\n"+
		"📤 Published: %d messages (%.2f msg/s)\n"+
		"📥 Consumed: %d messages (%.2f msg/s)\n"+
		"❌ Errors: %d\n"+
		"🧵 Goroutines: %d → %d (peak: %d)\n"+
		"📊 Success Rate: %.2f%%\n"+
		"🚀 Optimization: Async Publish + Global Worker Pool\n",
		duration.Seconds(),
		finalPublished, publishThroughput,
		finalConsumed, consumeThroughput,
		finalErrors,
		initialGoroutines, finalGoroutines, peakGoroutines,
		float64(finalConsumed)/float64(finalPublished)*100)

	// ✅ 验证第二阶段优化目标
	t.Logf("\n🎯 ===== Stage2 Optimization Goals Verification =====")

	// 目标1: 吞吐量3600-9500 msg/s
	if publishThroughput >= 3600 {
		t.Logf("✅ Throughput Goal: %.2f msg/s (Target: 3600-9500 msg/s) - ACHIEVED", publishThroughput)
	} else {
		t.Logf("❌ Throughput Goal: %.2f msg/s (Target: 3600-9500 msg/s) - NOT ACHIEVED", publishThroughput)
	}

	// 目标2: Goroutine数量降至100-200
	if peakGoroutines <= 200 {
		t.Logf("✅ Goroutine Goal: %d (Target: ≤200) - ACHIEVED", peakGoroutines)
	} else {
		t.Logf("❌ Goroutine Goal: %d (Target: ≤200) - NOT ACHIEVED", peakGoroutines)
	}

	// 目标3: 错误率 < 1%
	errorRate := float64(finalErrors) / float64(finalPublished) * 100
	if errorRate < 1.0 {
		t.Logf("✅ Error Rate Goal: %.2f%% (Target: <1%%) - ACHIEVED", errorRate)
	} else {
		t.Logf("❌ Error Rate Goal: %.2f%% (Target: <1%%) - NOT ACHIEVED", errorRate)
	}

	// 基本断言
	assert.Greater(t, finalPublished, int64(1000), "Should publish at least 1000 messages")
	assert.Greater(t, finalConsumed, int64(500), "Should consume at least 500 messages")
	assert.Less(t, errorRate, 5.0, "Error rate should be less than 5%")
}
