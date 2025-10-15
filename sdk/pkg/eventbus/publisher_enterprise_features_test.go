package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPublisher_RateLimit_Enabled 测试发布端流量控制启用
func TestPublisher_RateLimit_Enabled(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			RateLimit: config.RateLimitConfig{
				Enabled:       true,
				RatePerSecond: 10.0, // 每秒10条消息
				BurstSize:     5,    // 突发5条
			},
		},
	}

	// 转换配置
	eventBusConfig := ConvertConfig(cfg)
	require.NotNil(t, eventBusConfig)

	// 创建 EventBus
	bus, err := NewEventBus(eventBusConfig)
	require.NoError(t, err)
	defer bus.Close()

	// 验证配置转换
	assert.True(t, eventBusConfig.Enterprise.Publisher.RateLimit.Enabled)
	assert.Equal(t, 10.0, eventBusConfig.Enterprise.Publisher.RateLimit.RatePerSecond)
	assert.Equal(t, 5, eventBusConfig.Enterprise.Publisher.RateLimit.BurstSize)
}

// TestPublisher_RateLimit_Disabled 测试发布端流量控制禁用
func TestPublisher_RateLimit_Disabled(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			RateLimit: config.RateLimitConfig{
				Enabled: false,
			},
		},
	}

	// 转换配置
	eventBusConfig := ConvertConfig(cfg)
	require.NotNil(t, eventBusConfig)

	// 创建 EventBus
	bus, err := NewEventBus(eventBusConfig)
	require.NoError(t, err)
	defer bus.Close()

	// 验证配置
	assert.False(t, eventBusConfig.Enterprise.Publisher.RateLimit.Enabled)
}

// TestPublisher_ErrorHandling_DeadLetterTopic 测试发布端死信队列配置
func TestPublisher_ErrorHandling_DeadLetterTopic(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			ErrorHandling: config.ErrorHandlingConfig{
				DeadLetterTopic:  "publisher-dlq",
				MaxRetryAttempts: 3,
				RetryBackoffBase: 1 * time.Second,
				RetryBackoffMax:  30 * time.Second,
			},
		},
	}

	// 转换配置
	eventBusConfig := ConvertConfig(cfg)
	require.NotNil(t, eventBusConfig)

	// 创建 EventBus
	bus, err := NewEventBus(eventBusConfig)
	require.NoError(t, err)
	defer bus.Close()

	// 验证配置
	assert.Equal(t, "publisher-dlq", eventBusConfig.Enterprise.Publisher.ErrorHandling.DeadLetterTopic)
	assert.Equal(t, 3, eventBusConfig.Enterprise.Publisher.ErrorHandling.MaxRetryAttempts)
	assert.Equal(t, 1*time.Second, eventBusConfig.Enterprise.Publisher.ErrorHandling.RetryBackoffBase)
	assert.Equal(t, 30*time.Second, eventBusConfig.Enterprise.Publisher.ErrorHandling.RetryBackoffMax)
}

// TestPublisher_RetryPolicy_Enabled 测试发布端重试策略启用
func TestPublisher_RetryPolicy_Enabled(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			MaxReconnectAttempts: 5,
			InitialBackoff:       1 * time.Second,
			MaxBackoff:           30 * time.Second,
		},
	}

	// 转换配置
	eventBusConfig := ConvertConfig(cfg)
	require.NotNil(t, eventBusConfig)

	// 创建 EventBus
	bus, err := NewEventBus(eventBusConfig)
	require.NoError(t, err)
	defer bus.Close()

	// 验证配置
	assert.True(t, eventBusConfig.Enterprise.Publisher.RetryPolicy.Enabled)
	assert.Equal(t, 5, eventBusConfig.Enterprise.Publisher.RetryPolicy.MaxRetries)
	assert.Equal(t, 1*time.Second, eventBusConfig.Enterprise.Publisher.RetryPolicy.InitialInterval)
	assert.Equal(t, 30*time.Second, eventBusConfig.Enterprise.Publisher.RetryPolicy.MaxInterval)
}

// TestPublisher_RetryPolicy_Disabled 测试发布端重试策略禁用
func TestPublisher_RetryPolicy_Disabled(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			MaxReconnectAttempts: 0, // 禁用重试
		},
	}

	// 转换配置
	eventBusConfig := ConvertConfig(cfg)
	require.NotNil(t, eventBusConfig)

	// 创建 EventBus
	bus, err := NewEventBus(eventBusConfig)
	require.NoError(t, err)
	defer bus.Close()

	// 验证配置
	assert.False(t, eventBusConfig.Enterprise.Publisher.RetryPolicy.Enabled)
	assert.Equal(t, 0, eventBusConfig.Enterprise.Publisher.RetryPolicy.MaxRetries)
}

// TestPublisher_EnterpriseFeatures_ConfigConversion 测试发布端企业特性配置转换
func TestPublisher_EnterpriseFeatures_ConfigConversion(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			MaxReconnectAttempts: 5,
			InitialBackoff:       1 * time.Second,
			MaxBackoff:           30 * time.Second,
			PublishTimeout:       10 * time.Second,
			BacklogDetection: config.PublisherBacklogDetectionConfig{
				Enabled:           true,
				MaxQueueDepth:     1000,
				MaxPublishLatency: 100 * time.Millisecond,
				RateThreshold:     0.8,
				CheckInterval:     5 * time.Second,
			},
			RateLimit: config.RateLimitConfig{
				Enabled:       true,
				RatePerSecond: 100.0,
				BurstSize:     200,
			},
			ErrorHandling: config.ErrorHandlingConfig{
				DeadLetterTopic:  "publisher-dlq",
				MaxRetryAttempts: 3,
				RetryBackoffBase: 1 * time.Second,
				RetryBackoffMax:  30 * time.Second,
			},
		},
	}

	// 转换配置
	eventBusConfig := convertConfig(cfg)

	// 验证发布端企业特性配置
	assert.NotNil(t, eventBusConfig.Enterprise.Publisher)

	// 验证积压检测配置
	assert.True(t, eventBusConfig.Enterprise.Publisher.BacklogDetection.Enabled)
	assert.Equal(t, int64(1000), eventBusConfig.Enterprise.Publisher.BacklogDetection.MaxQueueDepth)

	// 验证重试策略配置
	assert.True(t, eventBusConfig.Enterprise.Publisher.RetryPolicy.Enabled)
	assert.Equal(t, 5, eventBusConfig.Enterprise.Publisher.RetryPolicy.MaxRetries)
	assert.Equal(t, 1*time.Second, eventBusConfig.Enterprise.Publisher.RetryPolicy.InitialInterval)
	assert.Equal(t, 30*time.Second, eventBusConfig.Enterprise.Publisher.RetryPolicy.MaxInterval)
	assert.Equal(t, 2.0, eventBusConfig.Enterprise.Publisher.RetryPolicy.Multiplier)

	// 验证流量控制配置
	assert.True(t, eventBusConfig.Enterprise.Publisher.RateLimit.Enabled)
	assert.Equal(t, 100.0, eventBusConfig.Enterprise.Publisher.RateLimit.RatePerSecond)
	assert.Equal(t, 200, eventBusConfig.Enterprise.Publisher.RateLimit.BurstSize)

	// 验证错误处理配置
	assert.Equal(t, "publisher-dlq", eventBusConfig.Enterprise.Publisher.ErrorHandling.DeadLetterTopic)
	assert.Equal(t, 3, eventBusConfig.Enterprise.Publisher.ErrorHandling.MaxRetryAttempts)
	assert.Equal(t, 1*time.Second, eventBusConfig.Enterprise.Publisher.ErrorHandling.RetryBackoffBase)
	assert.Equal(t, 30*time.Second, eventBusConfig.Enterprise.Publisher.ErrorHandling.RetryBackoffMax)
}

// TestPublisher_RateLimit_Integration 测试发布端流量控制集成
func TestPublisher_RateLimit_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			RateLimit: config.RateLimitConfig{
				Enabled:       true,
				RatePerSecond: 5.0, // 每秒5条消息
				BurstSize:     2,   // 突发2条
			},
		},
	}

	// 转换配置
	eventBusConfig := ConvertConfig(cfg)
	require.NotNil(t, eventBusConfig)

	// 创建 EventBus
	bus, err := NewEventBus(eventBusConfig)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "test-rate-limit"

	// 订阅主题
	receivedCount := 0
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		receivedCount++
		return nil
	})
	require.NoError(t, err)

	// 发布消息（应该受到流量控制）
	start := time.Now()
	messageCount := 10

	for i := 0; i < messageCount; i++ {
		err := bus.Publish(ctx, topic, []byte("test message"))
		require.NoError(t, err)
	}

	elapsed := time.Since(start)

	// 验证：由于流量控制，发布10条消息应该需要一定时间
	// 前2条可以突发，后8条需要按照每秒5条的速率
	// 预期时间：约 8/5 = 1.6秒
	// 注意：Memory EventBus 可能不支持流量控制，这里只是验证配置
	t.Logf("Published %d messages in %v", messageCount, elapsed)

	// 等待消息处理
	time.Sleep(100 * time.Millisecond)

	// 验证消息接收
	assert.Equal(t, messageCount, receivedCount)
}

// TestPublisher_ErrorHandling_Integration 测试发布端错误处理集成
func TestPublisher_ErrorHandling_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			ErrorHandling: config.ErrorHandlingConfig{
				DeadLetterTopic:  "publisher-dlq",
				MaxRetryAttempts: 3,
				RetryBackoffBase: 100 * time.Millisecond,
				RetryBackoffMax:  1 * time.Second,
			},
		},
	}

	// 转换配置
	eventBusConfig := ConvertConfig(cfg)
	require.NotNil(t, eventBusConfig)

	// 创建 EventBus
	bus, err := NewEventBus(eventBusConfig)
	require.NoError(t, err)
	defer bus.Close()

	// 验证配置已正确设置
	assert.Equal(t, "publisher-dlq", eventBusConfig.Enterprise.Publisher.ErrorHandling.DeadLetterTopic)
	assert.Equal(t, 3, eventBusConfig.Enterprise.Publisher.ErrorHandling.MaxRetryAttempts)

	// 注意：Memory EventBus 不会产生发布错误，
	// 实际的错误处理测试需要在 Kafka 集成测试中进行
}

// TestPublisher_RetryPolicy_Integration 测试发布端重试策略集成
func TestPublisher_RetryPolicy_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			MaxReconnectAttempts: 3,
			InitialBackoff:       100 * time.Millisecond,
			MaxBackoff:           1 * time.Second,
		},
	}

	// 转换配置
	eventBusConfig := ConvertConfig(cfg)
	require.NotNil(t, eventBusConfig)

	// 创建 EventBus
	bus, err := NewEventBus(eventBusConfig)
	require.NoError(t, err)
	defer bus.Close()

	// 验证配置已正确设置
	assert.True(t, eventBusConfig.Enterprise.Publisher.RetryPolicy.Enabled)
	assert.Equal(t, 3, eventBusConfig.Enterprise.Publisher.RetryPolicy.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, eventBusConfig.Enterprise.Publisher.RetryPolicy.InitialInterval)
	assert.Equal(t, 1*time.Second, eventBusConfig.Enterprise.Publisher.RetryPolicy.MaxInterval)

	// 注意：实际的重试策略测试需要在 Kafka 集成测试中进行
}
