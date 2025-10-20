package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestProductionReadiness 生产就绪综合测试
func TestProductionReadiness(t *testing.T) {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	t.Run("MemoryEventBusStabilityTest", testMemoryEventBusStability)
	t.Run("HealthCheckStabilityTest", testHealthCheckStability)
	t.Run("ConcurrentOperationsTest", testConcurrentOperations)
	t.Run("LongRunningStabilityTest", testLongRunningStability)
	t.Run("ErrorRecoveryTest", testErrorRecovery)
}

// testMemoryEventBusStability 测试内存EventBus的稳定性
func testMemoryEventBusStability(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "stability-test",
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:    "stability-health-check",
				Interval: 2 * time.Second,
				Timeout:  5 * time.Second,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:           "stability-health-check",
				MonitorInterval: 1 * time.Second,
			},
		},
	}

	// 初始化EventBus
	err := InitializeFromConfig(cfg)
	require.NoError(t, err)
	defer CloseGlobal()

	bus := GetGlobal()
	require.NotNil(t, bus)

	// 测试基本发布订阅功能
	topic := "stability-test-topic"
	messageCount := 100
	receivedCount := 0
	var mu sync.Mutex

	// 订阅消息
	err = bus.Subscribe(context.Background(), topic, func(ctx context.Context, message []byte) error {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)

	// 发布消息
	for i := 0; i < messageCount; i++ {
		message := []byte(`{"message": "test"}`)
		err = bus.Publish(context.Background(), topic, message)
		require.NoError(t, err)
	}

	// 等待消息处理
	time.Sleep(2 * time.Second)

	mu.Lock()
	finalCount := receivedCount
	mu.Unlock()

	assert.Equal(t, messageCount, finalCount, "应该接收到所有发布的消息")
}

// testHealthCheckStability 测试健康检查的稳定性（优化版：减少运行时间）
func testHealthCheckStability(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "health-stability-test",
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:    "health-stability-check",
				Interval: 200 * time.Millisecond, // 减少间隔以加快测试
				Timeout:  1 * time.Second,        // 减少超时时间
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:           "health-stability-check",
				MonitorInterval: 100 * time.Millisecond, // 减少监控间隔
			},
		},
	}

	err := InitializeFromConfig(cfg)
	require.NoError(t, err)
	defer CloseGlobal()

	bus := GetGlobal()
	require.NotNil(t, bus)

	// 启动健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = bus.StartHealthCheckPublisher(ctx)
	require.NoError(t, err)

	err = bus.StartHealthCheckSubscriber(ctx)
	require.NoError(t, err)

	// 运行2秒钟（从5秒减少到2秒）
	time.Sleep(2 * time.Second)

	// 检查健康检查状态
	publisherStatus := bus.GetHealthCheckPublisherStatus()
	subscriberStats := bus.GetHealthCheckSubscriberStats()

	assert.True(t, publisherStatus.IsHealthy, "发布器应该是健康的")
	assert.True(t, subscriberStats.IsHealthy, "订阅器应该是健康的")
	assert.Greater(t, subscriberStats.TotalMessagesReceived, int64(3), "应该接收到多条健康检查消息")

	// 停止健康检查
	err = bus.StopHealthCheckPublisher()
	assert.NoError(t, err)

	err = bus.StopHealthCheckSubscriber()
	assert.NoError(t, err)
}

// testConcurrentOperations 测试并发操作
func testConcurrentOperations(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "concurrent-test",
		HealthCheck: config.HealthCheckConfig{
			Enabled: false, // 关闭健康检查以专注于并发测试
		},
	}

	err := InitializeFromConfig(cfg)
	require.NoError(t, err)
	defer CloseGlobal()

	bus := GetGlobal()
	require.NotNil(t, bus)

	topic := "concurrent-test-topic"
	goroutineCount := 10
	messagesPerGoroutine := 50
	totalMessages := goroutineCount * messagesPerGoroutine

	var receivedCount int64
	var mu sync.Mutex

	// 订阅消息
	err = bus.Subscribe(context.Background(), topic, func(ctx context.Context, message []byte) error {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)

	// 并发发布消息
	var wg sync.WaitGroup
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				message := []byte(`{"goroutine": ` + string(rune(goroutineID)) + `, "message": ` + string(rune(j)) + `}`)
				err := bus.Publish(context.Background(), topic, message)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// 等待消息处理
	time.Sleep(2 * time.Second)

	mu.Lock()
	finalCount := receivedCount
	mu.Unlock()

	assert.Equal(t, int64(totalMessages), finalCount, "应该接收到所有并发发布的消息")
}

// testLongRunningStability 测试长时间运行的稳定性（优化版：减少运行时间）
func testLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过长时间运行测试")
	}

	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "long-running-test",
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:    "long-running-health-check",
				Interval: 500 * time.Millisecond, // 减少间隔以加快测试
				Timeout:  2 * time.Second,        // 减少超时时间
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Topic:           "long-running-health-check",
				MonitorInterval: 200 * time.Millisecond, // 减少监控间隔
			},
		},
	}

	err := InitializeFromConfig(cfg)
	require.NoError(t, err)
	defer CloseGlobal()

	bus := GetGlobal()
	require.NotNil(t, bus)

	// 启动健康检查
	ctx := context.Background()
	err = bus.StartHealthCheckPublisher(ctx)
	require.NoError(t, err)

	err = bus.StartHealthCheckSubscriber(ctx)
	require.NoError(t, err)

	// 运行5秒（从30秒减少到5秒）
	duration := 5 * time.Second
	startTime := time.Now()

	// 等待第一条消息到达
	time.Sleep(1 * time.Second)

	for time.Since(startTime) < duration {
		// 定期检查状态
		publisherStatus := bus.GetHealthCheckPublisherStatus()
		subscriberStats := bus.GetHealthCheckSubscriberStats()

		// 只在接收到消息后才检查健康状态
		if subscriberStats.TotalMessagesReceived > 0 {
			assert.True(t, publisherStatus.IsHealthy, "发布器应该保持健康")
			assert.True(t, subscriberStats.IsHealthy, "订阅器应该保持健康")
		}

		time.Sleep(1 * time.Second) // 减少检查间隔
	}

	// 最终检查
	subscriberStats := bus.GetHealthCheckSubscriberStats()
	assert.Greater(t, subscriberStats.TotalMessagesReceived, int64(5), "应该接收到足够多的健康检查消息")
	assert.Equal(t, int32(0), subscriberStats.ConsecutiveMisses, "不应该有连续错过的消息")

	// 停止健康检查
	err = bus.StopHealthCheckPublisher()
	assert.NoError(t, err)

	err = bus.StopHealthCheckSubscriber()
	assert.NoError(t, err)
}

// testErrorRecovery 测试错误恢复能力
func testErrorRecovery(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type:        "memory",
		ServiceName: "error-recovery-test",
		HealthCheck: config.HealthCheckConfig{
			Enabled: false,
		},
	}

	err := InitializeFromConfig(cfg)
	require.NoError(t, err)
	defer CloseGlobal()

	bus := GetGlobal()
	require.NotNil(t, bus)

	topic := "error-recovery-topic"
	errorCount := 0
	successCount := 0
	var mu sync.Mutex

	// 订阅消息，模拟间歇性错误
	err = bus.Subscribe(context.Background(), topic, func(ctx context.Context, message []byte) error {
		mu.Lock()
		defer mu.Unlock()

		if errorCount < 5 {
			errorCount++
			return assert.AnError // 模拟错误
		}
		successCount++
		return nil
	})
	require.NoError(t, err)

	// 发布消息
	for i := 0; i < 10; i++ {
		message := []byte(`{"message": "test"}`)
		err = bus.Publish(context.Background(), topic, message)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	// 等待处理
	time.Sleep(2 * time.Second)

	mu.Lock()
	finalErrorCount := errorCount
	finalSuccessCount := successCount
	mu.Unlock()

	assert.Equal(t, 5, finalErrorCount, "应该有5个错误")
	assert.Equal(t, 5, finalSuccessCount, "应该有5个成功")
}
