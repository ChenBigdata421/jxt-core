package performance_tests

import (
	"context"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// createTestKafkaEventBus 创建测试用的 Kafka EventBus
func createTestKafkaEventBus(clientID string) (eventbus.EventBus, error) {
	return eventbus.NewKafkaEventBus(&eventbus.KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: clientID,
		Producer: eventbus.ProducerConfig{
			RequiredAcks:    1,
			Compression:     "snappy",
			FlushFrequency:  10 * time.Millisecond,
			FlushMessages:   100,
			Timeout:         30 * time.Second,
			FlushBytes:      1048576,
			RetryMax:        3,
			BatchSize:       16384,
			BufferSize:      8388608,
			MaxMessageBytes: 1048576,
		},
		Consumer: eventbus.ConsumerConfig{
			GroupID:            clientID + "-group",
			SessionTimeout:     10 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  30 * time.Second,
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			FetchMaxWait:       100 * time.Millisecond,
			MaxPollRecords:     500,
		},
		Net: eventbus.NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	})
}

// TestKafkaGoroutineLeakAnalysis 详细分析 Kafka 协程泄漏
func TestKafkaGoroutineLeakAnalysis(t *testing.T) {
	// 初始化全局 logger
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	t.Log("========================================================================")
	t.Log("🔍 Kafka 协程泄漏详细分析")
	t.Log("========================================================================")

	// 1. 记录初始协程数
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("📊 初始协程数: %d", initialGoroutines)

	// 2. 创建 Kafka EventBus
	t.Log("\n🔧 创建 Kafka EventBus...")
	kafkaBus, err := createTestKafkaEventBus("test-leak-analysis")
	require.NoError(t, err)

	afterCreateGoroutines := runtime.NumGoroutine()
	t.Logf("📊 创建后协程数: %d (增加 %d)", afterCreateGoroutines, afterCreateGoroutines-initialGoroutines)

	// 3. 订阅消息
	t.Log("\n📥 订阅消息...")
	topics := []string{"test.leak.topic1", "test.leak.topic2", "test.leak.topic3"}
	ctx := context.Background()
	for _, topic := range topics {
		err = kafkaBus.Subscribe(ctx, topic, func(ctx context.Context, msg []byte) error {
			return nil
		})
		require.NoError(t, err)
	}

	afterSubscribeGoroutines := runtime.NumGoroutine()
	t.Logf("📊 订阅后协程数: %d (增加 %d)", afterSubscribeGoroutines, afterSubscribeGoroutines-afterCreateGoroutines)

	// 等待订阅就绪
	time.Sleep(2 * time.Second)

	// 5. 获取关闭前的协程堆栈
	t.Log("\n📸 获取关闭前的协程堆栈...")
	beforeCloseProfile := getGoroutineProfile()
	beforeCloseGoroutines := runtime.NumGoroutine()
	t.Logf("📊 关闭前协程数: %d", beforeCloseGoroutines)

	// 6. 关闭 Kafka EventBus
	t.Log("\n🔒 关闭 Kafka EventBus...")
	err = kafkaBus.Close()
	if err != nil {
		t.Logf("⚠️ 关闭时出现错误（可忽略）: %v", err)
	}

	// 等待协程退出
	time.Sleep(2 * time.Second)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// 7. 获取关闭后的协程堆栈
	t.Log("\n📸 获取关闭后的协程堆栈...")
	afterCloseProfile := getGoroutineProfile()
	afterCloseGoroutines := runtime.NumGoroutine()
	t.Logf("📊 关闭后协程数: %d", afterCloseGoroutines)

	// 8. 计算泄漏
	leaked := afterCloseGoroutines - initialGoroutines
	t.Logf("\n🔴 协程泄漏数: %d", leaked)

	// 9. 分析泄漏的协程
	t.Log("\n========================================================================")
	t.Log("🔍 泄漏协程详细分析")
	t.Log("========================================================================")

	analyzeGoroutineLeaks(t, beforeCloseProfile, afterCloseProfile)

	// 10. 输出详细的协程堆栈
	t.Log("\n========================================================================")
	t.Log("📋 关闭后的所有协程堆栈")
	t.Log("========================================================================")
	t.Logf("\n%s", afterCloseProfile)

	// 11. 统计协程类型
	t.Log("\n========================================================================")
	t.Log("📊 协程类型统计")
	t.Log("========================================================================")
	analyzeGoroutineTypes(t, afterCloseProfile)
}

// getGoroutineProfile 获取协程堆栈信息
func getGoroutineProfile() string {
	buf := make([]byte, 1<<20) // 1MB buffer
	n := runtime.Stack(buf, true)
	return string(buf[:n])
}

// analyzeGoroutineLeaks 分析泄漏的协程
func analyzeGoroutineLeaks(t *testing.T, beforeProfile, afterProfile string) {
	beforeLines := strings.Split(beforeProfile, "\n")
	afterLines := strings.Split(afterProfile, "\n")

	t.Logf("关闭前协程堆栈行数: %d", len(beforeLines))
	t.Logf("关闭后协程堆栈行数: %d", len(afterLines))

	// 提取协程信息
	beforeGoroutines := extractGoroutineInfo(beforeProfile)
	afterGoroutines := extractGoroutineInfo(afterProfile)

	t.Logf("\n关闭前协程数: %d", len(beforeGoroutines))
	t.Logf("关闭后协程数: %d", len(afterGoroutines))

	// 找出泄漏的协程（关闭后仍然存在的协程）
	t.Log("\n🔴 泄漏的协程:")
	leakCount := 0
	for _, goroutine := range afterGoroutines {
		// 跳过测试框架的协程
		if strings.Contains(goroutine, "testing.") ||
			strings.Contains(goroutine, "runtime.goexit") {
			continue
		}

		t.Logf("\n协程 #%d:", leakCount+1)
		t.Logf("%s", goroutine)
		leakCount++
	}

	t.Logf("\n总泄漏协程数（排除测试框架）: %d", leakCount)
}

// extractGoroutineInfo 提取协程信息
func extractGoroutineInfo(profile string) []string {
	var goroutines []string
	lines := strings.Split(profile, "\n")

	var currentGoroutine strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(line, "goroutine ") {
			if currentGoroutine.Len() > 0 {
				goroutines = append(goroutines, currentGoroutine.String())
				currentGoroutine.Reset()
			}
		}
		currentGoroutine.WriteString(line)
		currentGoroutine.WriteString("\n")
	}

	if currentGoroutine.Len() > 0 {
		goroutines = append(goroutines, currentGoroutine.String())
	}

	return goroutines
}

// analyzeGoroutineTypes 统计协程类型
func analyzeGoroutineTypes(t *testing.T, profile string) {
	typeCount := make(map[string]int)
	goroutines := extractGoroutineInfo(profile)

	for _, goroutine := range goroutines {
		goroutineType := identifyGoroutineType(goroutine)
		typeCount[goroutineType]++
	}

	t.Log("\n协程类型分布:")
	for goroutineType, count := range typeCount {
		t.Logf("  %-50s: %d", goroutineType, count)
	}
}

// identifyGoroutineType 识别协程类型
func identifyGoroutineType(goroutine string) string {
	switch {
	// Sarama 相关
	case strings.Contains(goroutine, "github.com/IBM/sarama.(*Broker)"):
		return "Sarama Broker"
	case strings.Contains(goroutine, "github.com/IBM/sarama.(*client)"):
		return "Sarama Client"
	case strings.Contains(goroutine, "github.com/IBM/sarama.(*consumerGroup)"):
		return "Sarama ConsumerGroup"
	case strings.Contains(goroutine, "github.com/IBM/sarama.(*partitionConsumer)"):
		return "Sarama PartitionConsumer"
	case strings.Contains(goroutine, "github.com/IBM/sarama.(*asyncProducer)"):
		return "Sarama AsyncProducer"
	case strings.Contains(goroutine, "github.com/IBM/sarama"):
		return "Sarama Other"

	// EventBus 相关
	case strings.Contains(goroutine, "GlobalWorkerPool"):
		return "EventBus GlobalWorkerPool"
	case strings.Contains(goroutine, "KeyedWorkerPool"):
		return "EventBus KeyedWorkerPool"
	case strings.Contains(goroutine, "dispatcher"):
		return "EventBus Dispatcher"
	case strings.Contains(goroutine, "preSubscriptionConsumer"):
		return "EventBus PreSubscriptionConsumer"
	case strings.Contains(goroutine, "eventbus"):
		return "EventBus Other"

	// 测试框架
	case strings.Contains(goroutine, "testing."):
		return "Testing Framework"

	// 运行时
	case strings.Contains(goroutine, "runtime."):
		return "Go Runtime"

	default:
		return "Unknown"
	}
}

// TestKafkaGoroutineLeakWithPprof 使用 pprof 分析协程泄漏
func TestKafkaGoroutineLeakWithPprof(t *testing.T) {
	// 初始化全局 logger
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	t.Log("========================================================================")
	t.Log("🔍 使用 pprof 分析 Kafka 协程泄漏")
	t.Log("========================================================================")

	// 1. 记录初始协程数
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("📊 初始协程数: %d", initialGoroutines)

	// 2. 创建 Kafka EventBus（使用简化配置避免 logger 问题）
	kafkaBus, err := createTestKafkaEventBus("test-leak-pprof")
	require.NoError(t, err)

	// 3. 订阅消息
	topics := []string{"test.leak.topic1"}
	ctx := context.Background()
	err = kafkaBus.Subscribe(ctx, topics[0], func(ctx context.Context, msg []byte) error {
		return nil
	})
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	// 4. 保存关闭前的 goroutine profile
	beforeFile := "goroutine_before_close.prof"
	f1, err := os.Create(beforeFile)
	require.NoError(t, err)
	err = pprof.Lookup("goroutine").WriteTo(f1, 2)
	f1.Close()
	if err == nil {
		t.Logf("✅ 已保存关闭前的 goroutine profile: %s", beforeFile)
	}

	// 5. 关闭
	err = kafkaBus.Close()
	if err != nil {
		t.Logf("⚠️ 关闭时出现错误（可忽略）: %v", err)
	}

	time.Sleep(2 * time.Second)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// 6. 保存关闭后的 goroutine profile
	afterFile := "goroutine_after_close.prof"
	f2, err := os.Create(afterFile)
	require.NoError(t, err)
	err = pprof.Lookup("goroutine").WriteTo(f2, 2)
	f2.Close()
	if err == nil {
		t.Logf("✅ 已保存关闭后的 goroutine profile: %s", afterFile)
	}

	afterGoroutines := runtime.NumGoroutine()
	leaked := afterGoroutines - initialGoroutines
	t.Logf("\n🔴 协程泄漏数: %d", leaked)

	t.Log("\n========================================================================")
	t.Log("📋 使用以下命令分析 goroutine profile:")
	t.Log("========================================================================")
	t.Logf("go tool pprof -http=:8080 %s", beforeFile)
	t.Logf("go tool pprof -http=:8081 %s", afterFile)
	t.Log("\n或者使用文本模式:")
	t.Logf("go tool pprof -text %s", afterFile)
}
