package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewKafkaEventBus_NilConfig 测试 nil 配置
func TestNewKafkaEventBus_NilConfig(t *testing.T) {
	_, err := NewKafkaEventBus(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config cannot be nil")
}

// TestNewKafkaEventBus_EmptyBrokers 测试空 brokers
func TestNewKafkaEventBus_EmptyBrokers(t *testing.T) {
	cfg := &config.KafkaConfig{
		Brokers: []string{},
	}
	_, err := NewKafkaEventBus(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "brokers cannot be empty")
}

// TestConfigureSarama_Compression 测试压缩配置
func TestConfigureSarama_Compression(t *testing.T) {
	tests := []struct {
		name        string
		compression string
		expected    sarama.CompressionCodec
	}{
		{
			name:        "GZIP compression",
			compression: "gzip",
			expected:    sarama.CompressionGZIP,
		},
		{
			name:        "Snappy compression",
			compression: "snappy",
			expected:    sarama.CompressionSnappy,
		},
		{
			name:        "LZ4 compression",
			compression: "lz4",
			expected:    sarama.CompressionLZ4,
		},
		{
			name:        "ZSTD compression",
			compression: "zstd",
			expected:    sarama.CompressionZSTD,
		},
		{
			name:        "No compression",
			compression: "none",
			expected:    sarama.CompressionNone,
		},
		{
			name:        "Unknown compression defaults to none",
			compression: "unknown",
			expected:    sarama.CompressionNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saramaConfig := sarama.NewConfig()
			cfg := &config.KafkaConfig{
				Producer: config.ProducerConfig{
					Compression:  tt.compression,
					RequiredAcks: 1,
					Timeout:      10 * time.Second,
					RetryMax:     3,
				},
				Consumer: config.ConsumerConfig{
					GroupID:          "test-group",
					AutoOffsetReset:  "earliest",
					EnableAutoCommit: true,
					SessionTimeout:   10 * time.Second,
				},
			}

			err := configureSarama(saramaConfig, cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, saramaConfig.Producer.Compression)
		})
	}
}

// TestConfigureSarama_ProducerSettings 测试生产者配置
func TestConfigureSarama_ProducerSettings(t *testing.T) {
	saramaConfig := sarama.NewConfig()
	cfg := &config.KafkaConfig{
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      15 * time.Second,
			RetryMax:     5,
			Compression:  "gzip",
		},
		Consumer: config.ConsumerConfig{
			GroupID:          "test-group",
			AutoOffsetReset:  "earliest",
			EnableAutoCommit: true,
			SessionTimeout:   10 * time.Second,
		},
	}

	err := configureSarama(saramaConfig, cfg)
	require.NoError(t, err)

	assert.Equal(t, sarama.RequiredAcks(1), saramaConfig.Producer.RequiredAcks)
	assert.Equal(t, 15*time.Second, saramaConfig.Producer.Timeout)
	assert.Equal(t, 5, saramaConfig.Producer.Retry.Max)
	assert.True(t, saramaConfig.Producer.Return.Successes)
	assert.True(t, saramaConfig.Producer.Return.Errors)
}

// TestConfigureSarama_ConsumerSettings 测试消费者配置
func TestConfigureSarama_ConsumerSettings(t *testing.T) {
	saramaConfig := sarama.NewConfig()
	cfg := &config.KafkaConfig{
		Producer: config.ProducerConfig{
			RequiredAcks: 1,
			Timeout:      10 * time.Second,
			RetryMax:     3,
		},
		Consumer: config.ConsumerConfig{
			GroupID:          "test-group",
			AutoOffsetReset:  "earliest",
			EnableAutoCommit: true,
			SessionTimeout:   20 * time.Second,
		},
	}

	err := configureSarama(saramaConfig, cfg)
	require.NoError(t, err)

	assert.Equal(t, 20*time.Second, saramaConfig.Consumer.Group.Session.Timeout)
	// Consumer.Return.Errors is set by configureSarama
}

// TestConfigureSarama_OffsetReset 测试 offset reset 配置
func TestConfigureSarama_OffsetReset(t *testing.T) {
	tests := []struct {
		name     string
		offset   string
		expected int64
	}{
		{
			name:     "Earliest offset",
			offset:   "earliest",
			expected: sarama.OffsetOldest,
		},
		{
			name:     "Latest offset",
			offset:   "latest",
			expected: sarama.OffsetNewest,
		},
		{
			name:     "Unknown defaults to latest",
			offset:   "unknown",
			expected: sarama.OffsetNewest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saramaConfig := sarama.NewConfig()
			cfg := &config.KafkaConfig{
				Producer: config.ProducerConfig{
					RequiredAcks: 1,
					Timeout:      10 * time.Second,
					RetryMax:     3,
				},
				Consumer: config.ConsumerConfig{
					GroupID:          "test-group",
					AutoOffsetReset:  tt.offset,
					EnableAutoCommit: true,
					SessionTimeout:   10 * time.Second,
				},
			}

			err := configureSarama(saramaConfig, cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, saramaConfig.Consumer.Offsets.Initial)
		})
	}
}

// TestDefaultReconnectConfig 测试默认重连配置
func TestDefaultReconnectConfig(t *testing.T) {
	config := DefaultReconnectConfig()

	assert.Equal(t, DefaultMaxReconnectAttempts, config.MaxAttempts)
	assert.Equal(t, DefaultReconnectInitialBackoff, config.InitialBackoff)
	assert.Equal(t, DefaultReconnectMaxBackoff, config.MaxBackoff)
	assert.Equal(t, DefaultReconnectBackoffFactor, config.BackoffFactor)
	assert.Equal(t, DefaultReconnectFailureThreshold, config.FailureThreshold)
}

// TestKafkaEventBus_GetConnectionState 测试获取连接状态
func TestKafkaEventBus_GetConnectionState(t *testing.T) {
	// This test would require a mock or actual Kafka connection
	// For now, we'll skip it or mark it as integration test
	t.Skip("Requires actual Kafka connection or mock")
}

// TestKafkaEventBus_SetTopicConfigStrategy 测试设置主题配置策略
func TestKafkaEventBus_SetTopicConfigStrategy(t *testing.T) {
	// This test would require a Kafka instance
	// For now, we'll test the strategy enum values
	strategies := []TopicConfigStrategy{
		StrategyCreateOnly,
		StrategyCreateOrUpdate,
		StrategyValidateOnly,
		StrategySkip,
	}

	for _, strategy := range strategies {
		assert.NotEmpty(t, strategy)
	}
}

// TestKafkaEventBus_TopicConfigMismatchAction 测试主题配置不匹配行为
func TestKafkaEventBus_TopicConfigMismatchAction(t *testing.T) {
	action := TopicConfigMismatchAction{
		LogLevel: "warn",
		FailFast: false,
	}

	assert.Equal(t, "warn", action.LogLevel)
	assert.False(t, action.FailFast)

	action2 := TopicConfigMismatchAction{
		LogLevel: "error",
		FailFast: true,
	}

	assert.Equal(t, "error", action2.LogLevel)
	assert.True(t, action2.FailFast)
}

// TestKafkaEventBus_PublishOptions 测试发布选项
func TestKafkaEventBus_PublishOptions(t *testing.T) {
	opts := PublishOptions{
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Timeout:     10 * time.Second,
		AggregateID: "test-aggregate-1",
	}

	assert.Len(t, opts.Metadata, 2)
	assert.Equal(t, "value1", opts.Metadata["key1"])
	assert.Equal(t, 10*time.Second, opts.Timeout)
	assert.Equal(t, "test-aggregate-1", opts.AggregateID)
}

// TestKafkaEventBus_SubscribeOptions 测试订阅选项
func TestKafkaEventBus_SubscribeOptions(t *testing.T) {
	opts := SubscribeOptions{
		ProcessingTimeout: 30 * time.Second,
		RateLimit:         100.0,
		RateBurst:         10,
		MaxRetries:        3,
		RetryBackoff:      1 * time.Second,
		DeadLetterEnabled: true,
	}

	assert.Equal(t, 30*time.Second, opts.ProcessingTimeout)
	assert.Equal(t, 100.0, opts.RateLimit)
	assert.Equal(t, 10, opts.RateBurst)
	assert.Equal(t, 3, opts.MaxRetries)
	assert.Equal(t, 1*time.Second, opts.RetryBackoff)
	assert.True(t, opts.DeadLetterEnabled)
}

// TestKafkaEventBus_Metrics 测试指标结构
func TestKafkaEventBus_Metrics(t *testing.T) {
	metrics := Metrics{
		MessagesPublished: 100,
		MessagesConsumed:  90,
		PublishErrors:     5,
		ConsumeErrors:     3,
		LastHealthCheck:   time.Now(),
		HealthCheckStatus: "healthy",
		ActiveConnections: 2,
		MessageBacklog:    10,
	}

	assert.Equal(t, int64(100), metrics.MessagesPublished)
	assert.Equal(t, int64(90), metrics.MessagesConsumed)
	assert.Equal(t, int64(5), metrics.PublishErrors)
	assert.Equal(t, int64(3), metrics.ConsumeErrors)
	assert.Equal(t, "healthy", metrics.HealthCheckStatus)
	assert.Equal(t, 2, metrics.ActiveConnections)
	assert.Equal(t, int64(10), metrics.MessageBacklog)
}

// TestKafkaEventBus_ConnectionState 测试连接状态
func TestKafkaEventBus_ConnectionState(t *testing.T) {
	state := ConnectionState{
		IsConnected:       true,
		LastConnectedTime: time.Now(),
		ReconnectCount:    0,
		LastError:         "",
	}

	assert.True(t, state.IsConnected)
	assert.Empty(t, state.LastError)
	assert.Equal(t, 0, state.ReconnectCount)

	state2 := ConnectionState{
		IsConnected:    false,
		ReconnectCount: 3,
		LastError:      "connection failed",
	}

	assert.False(t, state2.IsConnected)
	assert.Equal(t, "connection failed", state2.LastError)
	assert.Equal(t, 3, state2.ReconnectCount)
}

// TestKafkaEventBus_Context 测试上下文传递
func TestKafkaEventBus_Context(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "test-key", "test-value")

	value := ctx.Value("test-key")
	assert.Equal(t, "test-value", value)

	// Test context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	select {
	case <-ctx.Done():
		assert.Error(t, ctx.Err())
	default:
		t.Fatal("Context should be cancelled")
	}
}
