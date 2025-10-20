package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewPublisherBacklogDetector 测试创建发送端积压检测器
func TestNewPublisherBacklogDetector(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     1 * time.Second,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)

	assert.NotNil(t, detector)
	assert.Equal(t, int64(1000), detector.maxQueueDepth)
	assert.Equal(t, 100*time.Millisecond, detector.maxPublishLatency)
	assert.Equal(t, 1000.0, detector.rateThreshold)
	assert.Equal(t, 1*time.Second, detector.checkInterval)
}

// TestPublisherBacklogDetector_RecordPublish 测试记录发送操作
func TestPublisherBacklogDetector_RecordPublish(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     1 * time.Second,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)

	// 记录一些发送操作
	detector.RecordPublish(10 * time.Millisecond)
	detector.RecordPublish(20 * time.Millisecond)
	detector.RecordPublish(30 * time.Millisecond)

	// 验证计数
	assert.Equal(t, int64(3), detector.publishCount.Load())

	// 验证总延迟
	expectedLatency := int64(10*time.Millisecond + 20*time.Millisecond + 30*time.Millisecond)
	assert.Equal(t, expectedLatency, detector.publishLatency.Load())
}

// TestPublisherBacklogDetector_UpdateQueueDepth 测试更新队列深度
func TestPublisherBacklogDetector_UpdateQueueDepth(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     1 * time.Second,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)

	// 更新队列深度
	detector.UpdateQueueDepth(500)
	assert.Equal(t, int64(500), detector.queueDepth.Load())

	detector.UpdateQueueDepth(1500)
	assert.Equal(t, int64(1500), detector.queueDepth.Load())
}

// TestPublisherBacklogDetector_RegisterCallback 测试注册回调
func TestPublisherBacklogDetector_RegisterCallback(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     1 * time.Second,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)

	// 注册回调
	callback := func(ctx context.Context, state PublisherBacklogState) error {
		return nil
	}

	err := detector.RegisterCallback(callback)
	require.NoError(t, err)
	assert.Len(t, detector.callbacks, 1)
}

// TestPublisherBacklogDetector_StartStop 测试启动和停止
func TestPublisherBacklogDetector_StartStop(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     100 * time.Millisecond,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)
	ctx := context.Background()

	// 启动
	err := detector.Start(ctx)
	require.NoError(t, err)
	assert.True(t, detector.isRunning)

	// 重复启动应该返回 nil
	err = detector.Start(ctx)
	require.NoError(t, err)

	// 停止
	err = detector.Stop()
	require.NoError(t, err)
	assert.False(t, detector.isRunning)

	// 重复停止应该返回 nil
	err = detector.Stop()
	require.NoError(t, err)
}

// TestPublisherBacklogDetector_IsBacklogged 测试积压判断
func TestPublisherBacklogDetector_IsBacklogged(t *testing.T) {
	tests := []struct {
		name          string
		config        config.PublisherBacklogDetectionConfig
		queueDepth    int64
		avgLatency    time.Duration
		publishRate   float64
		expectBacklog bool
	}{
		{
			name: "No backlog - all normal",
			config: config.PublisherBacklogDetectionConfig{
				MaxQueueDepth:     1000,
				MaxPublishLatency: 100 * time.Millisecond,
				RateThreshold:     1000.0,
			},
			queueDepth:    500,
			avgLatency:    50 * time.Millisecond,
			publishRate:   500.0,
			expectBacklog: false,
		},
		{
			name: "Backlog - queue depth exceeded",
			config: config.PublisherBacklogDetectionConfig{
				MaxQueueDepth:     1000,
				MaxPublishLatency: 100 * time.Millisecond,
				RateThreshold:     1000.0,
			},
			queueDepth:    1500,
			avgLatency:    50 * time.Millisecond,
			publishRate:   500.0,
			expectBacklog: true,
		},
		{
			name: "Backlog - latency exceeded",
			config: config.PublisherBacklogDetectionConfig{
				MaxQueueDepth:     1000,
				MaxPublishLatency: 100 * time.Millisecond,
				RateThreshold:     1000.0,
			},
			queueDepth:    500,
			avgLatency:    150 * time.Millisecond,
			publishRate:   500.0,
			expectBacklog: true,
		},
		{
			name: "Backlog - rate exceeded",
			config: config.PublisherBacklogDetectionConfig{
				MaxQueueDepth:     1000,
				MaxPublishLatency: 100 * time.Millisecond,
				RateThreshold:     1000.0,
			},
			queueDepth:    500,
			avgLatency:    50 * time.Millisecond,
			publishRate:   1500.0,
			expectBacklog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewPublisherBacklogDetector(nil, nil, tt.config)
			result := detector.isBacklogged(tt.queueDepth, tt.avgLatency, tt.publishRate)
			assert.Equal(t, tt.expectBacklog, result)
		})
	}
}

// TestPublisherBacklogDetector_CalculateBacklogRatio 测试计算积压比例
func TestPublisherBacklogDetector_CalculateBacklogRatio(t *testing.T) {
	tests := []struct {
		name        string
		config      config.PublisherBacklogDetectionConfig
		queueDepth  int64
		avgLatency  time.Duration
		publishRate float64
		expectRatio float64
	}{
		{
			name: "No backlog",
			config: config.PublisherBacklogDetectionConfig{
				MaxQueueDepth:     1000,
				MaxPublishLatency: 100 * time.Millisecond,
				RateThreshold:     1000.0,
			},
			queueDepth:  500,
			avgLatency:  50 * time.Millisecond,
			publishRate: 500.0,
			expectRatio: 0.5, // max(500/1000, 50/100, 500/1000) = 0.5
		},
		{
			name: "High backlog",
			config: config.PublisherBacklogDetectionConfig{
				MaxQueueDepth:     1000,
				MaxPublishLatency: 100 * time.Millisecond,
				RateThreshold:     1000.0,
			},
			queueDepth:  900,
			avgLatency:  90 * time.Millisecond,
			publishRate: 900.0,
			expectRatio: 0.9, // max(900/1000, 90/100, 900/1000) = 0.9
		},
		{
			name: "Exceeded - capped at 1.0",
			config: config.PublisherBacklogDetectionConfig{
				MaxQueueDepth:     1000,
				MaxPublishLatency: 100 * time.Millisecond,
				RateThreshold:     1000.0,
			},
			queueDepth:  2000,
			avgLatency:  200 * time.Millisecond,
			publishRate: 2000.0,
			expectRatio: 1.0, // capped at 1.0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewPublisherBacklogDetector(nil, nil, tt.config)
			result := detector.calculateBacklogRatio(tt.queueDepth, tt.avgLatency, tt.publishRate)
			assert.InDelta(t, tt.expectRatio, result, 0.01)
		})
	}
}

// TestPublisherBacklogDetector_CalculateSeverity 测试计算严重程度
func TestPublisherBacklogDetector_CalculateSeverity(t *testing.T) {
	tests := []struct {
		name           string
		backlogRatio   float64
		expectSeverity string
	}{
		{"Normal", 0.2, "NORMAL"},
		{"Low", 0.4, "LOW"},
		{"Medium", 0.6, "MEDIUM"},
		{"High", 0.8, "HIGH"},
		{"Critical", 0.95, "CRITICAL"},
	}

	cfg := config.PublisherBacklogDetectionConfig{
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     1 * time.Second,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.calculateSeverity(tt.backlogRatio)
			assert.Equal(t, tt.expectSeverity, result)
		})
	}
}

// TestPublisherBacklogDetector_GetBacklogState 测试获取积压状态
func TestPublisherBacklogDetector_GetBacklogState(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     1 * time.Second,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)

	// 记录一些数据
	detector.RecordPublish(50 * time.Millisecond)
	detector.RecordPublish(60 * time.Millisecond)
	detector.UpdateQueueDepth(500)

	// 等待一小段时间以计算速率
	time.Sleep(100 * time.Millisecond)

	// 获取状态
	state := detector.GetBacklogState()

	assert.False(t, state.HasBacklog)
	assert.Equal(t, int64(500), state.QueueDepth)
	assert.Greater(t, state.PublishRate, 0.0)
	assert.Greater(t, state.AvgPublishLatency, time.Duration(0))
	assert.GreaterOrEqual(t, state.BacklogRatio, 0.0)
	assert.LessOrEqual(t, state.BacklogRatio, 1.0)
	assert.NotEmpty(t, state.Severity)
}

// TestPublisherBacklogDetector_CallbackExecution 测试回调执行
func TestPublisherBacklogDetector_CallbackExecution(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     50 * time.Millisecond,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)
	ctx := context.Background()

	// 注册回调
	var callbackCount atomic.Int32
	var lastState PublisherBacklogState
	var mu sync.Mutex

	callback := func(ctx context.Context, state PublisherBacklogState) error {
		callbackCount.Add(1)
		mu.Lock()
		lastState = state
		mu.Unlock()
		return nil
	}

	err := detector.RegisterCallback(callback)
	require.NoError(t, err)

	// 启动检测器
	err = detector.Start(ctx)
	require.NoError(t, err)
	defer detector.Stop()

	// 模拟一些发送操作
	detector.RecordPublish(50 * time.Millisecond)
	detector.UpdateQueueDepth(500)

	// 等待回调被调用
	time.Sleep(200 * time.Millisecond)

	// 验证回调被调用
	assert.Greater(t, callbackCount.Load(), int32(0), "回调应该被调用")

	// 验证状态
	mu.Lock()
	assert.Equal(t, int64(500), lastState.QueueDepth)
	mu.Unlock()
}

// TestPublisherBacklogDetector_MultipleCallbacks 测试多个回调
func TestPublisherBacklogDetector_MultipleCallbacks(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     50 * time.Millisecond,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)
	ctx := context.Background()

	// 注册多个回调
	var callback1Count, callback2Count, callback3Count atomic.Int32

	callback1 := func(ctx context.Context, state PublisherBacklogState) error {
		callback1Count.Add(1)
		return nil
	}

	callback2 := func(ctx context.Context, state PublisherBacklogState) error {
		callback2Count.Add(1)
		return nil
	}

	callback3 := func(ctx context.Context, state PublisherBacklogState) error {
		callback3Count.Add(1)
		return nil
	}

	detector.RegisterCallback(callback1)
	detector.RegisterCallback(callback2)
	detector.RegisterCallback(callback3)

	// 启动检测器
	err := detector.Start(ctx)
	require.NoError(t, err)
	defer detector.Stop()

	// 等待回调被调用
	time.Sleep(200 * time.Millisecond)

	// 验证所有回调都被调用
	assert.Greater(t, callback1Count.Load(), int32(0))
	assert.Greater(t, callback2Count.Load(), int32(0))
	assert.Greater(t, callback3Count.Load(), int32(0))
}

// TestPublisherBacklogDetector_ConcurrentRecordPublish 测试并发记录发送
func TestPublisherBacklogDetector_ConcurrentRecordPublish(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     1 * time.Second,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)

	var wg sync.WaitGroup
	goroutineCount := 100
	recordsPerGoroutine := 100

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				detector.RecordPublish(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// 验证计数
	expectedCount := int64(goroutineCount * recordsPerGoroutine)
	assert.Equal(t, expectedCount, detector.publishCount.Load())
}

// TestPublisherBacklogDetector_ConcurrentUpdateQueueDepth 测试并发更新队列深度
func TestPublisherBacklogDetector_ConcurrentUpdateQueueDepth(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     1 * time.Second,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)

	var wg sync.WaitGroup
	goroutineCount := 100

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func(depth int64) {
			defer wg.Done()
			detector.UpdateQueueDepth(depth)
		}(int64(i))
	}

	wg.Wait()

	// 验证队列深度被设置（最后一个值）
	depth := detector.queueDepth.Load()
	assert.GreaterOrEqual(t, depth, int64(0))
	assert.Less(t, depth, int64(goroutineCount))
}

// TestPublisherBacklogDetector_ContextCancellation 测试上下文取消
func TestPublisherBacklogDetector_ContextCancellation(t *testing.T) {
	cfg := config.PublisherBacklogDetectionConfig{
		Enabled:           true,
		MaxQueueDepth:     1000,
		MaxPublishLatency: 100 * time.Millisecond,
		RateThreshold:     1000.0,
		CheckInterval:     50 * time.Millisecond,
	}

	detector := NewPublisherBacklogDetector(nil, nil, cfg)
	ctx, cancel := context.WithCancel(context.Background())

	// 启动检测器
	err := detector.Start(ctx)
	require.NoError(t, err)
	assert.True(t, detector.isRunning)

	// 取消上下文
	cancel()

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 停止检测器
	err = detector.Stop()
	require.NoError(t, err)
	assert.False(t, detector.isRunning)
}
