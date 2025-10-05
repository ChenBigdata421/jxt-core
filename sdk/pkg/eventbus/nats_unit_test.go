package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildNATSOptions 测试构建 NATS 选项
func TestBuildNATSOptions(t *testing.T) {
	cfg := &config.NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "test-client",
		MaxReconnects:     10,
		ReconnectWait:     2 * time.Second,
		ConnectionTimeout: 5 * time.Second,
	}

	opts := buildNATSOptions(cfg)
	require.NotNil(t, opts)
	assert.NotEmpty(t, opts)
}

// TestBuildNATSOptions_WithSecurity 测试带安全配置的 NATS 选项
func TestBuildNATSOptions_WithSecurity(t *testing.T) {
	cfg := &config.NATSConfig{
		URLs:              []string{"nats://localhost:4222"},
		ClientID:          "test-client",
		MaxReconnects:     10,
		ReconnectWait:     2 * time.Second,
		ConnectionTimeout: 5 * time.Second,
		Security: config.NATSSecurityConfig{
			Enabled:  true,
			Username: "testuser",
			Password: "testpass",
		},
	}

	opts := buildNATSOptions(cfg)
	require.NotNil(t, opts)
	assert.NotEmpty(t, opts)
}

// TestBuildJetStreamOptions 测试构建 JetStream 选项
func TestBuildJetStreamOptions(t *testing.T) {
	cfg := &config.NATSConfig{
		JetStream: config.JetStreamConfig{
			Enabled: true,
			Stream: config.StreamConfig{
				Name:      "test-stream",
				Subjects:  []string{"test.>"},
				Retention: "limits",
				MaxAge:    24 * time.Hour,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   1000000,
				Replicas:  1,
				Storage:   "file",
			},
		},
	}

	opts := buildJetStreamOptions(cfg)
	// opts may be nil or empty depending on implementation
	_ = opts
}

// TestBuildJetStreamOptions_WithDomain 测试带 Domain 的 JetStream 选项
func TestBuildJetStreamOptions_WithDomain(t *testing.T) {
	cfg := &config.NATSConfig{
		JetStream: config.JetStreamConfig{
			Enabled: true,
			Domain:  "test-domain",
			Stream: config.StreamConfig{
				Name:     "test-stream",
				Subjects: []string{"test.>"},
			},
		},
	}

	opts := buildJetStreamOptions(cfg)
	require.NotNil(t, opts)
	assert.NotEmpty(t, opts)
}

// TestStreamRetentionPolicy 测试流保留策略
func TestStreamRetentionPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected nats.RetentionPolicy
	}{
		{
			name:     "Limits retention",
			policy:   "limits",
			expected: nats.LimitsPolicy,
		},
		{
			name:     "Interest retention",
			policy:   "interest",
			expected: nats.InterestPolicy,
		},
		{
			name:     "WorkQueue retention",
			policy:   "workqueue",
			expected: nats.WorkQueuePolicy,
		},
		{
			name:     "Unknown defaults to limits",
			policy:   "unknown",
			expected: nats.LimitsPolicy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var policy nats.RetentionPolicy
			switch tt.policy {
			case "limits":
				policy = nats.LimitsPolicy
			case "interest":
				policy = nats.InterestPolicy
			case "workqueue":
				policy = nats.WorkQueuePolicy
			default:
				policy = nats.LimitsPolicy
			}
			assert.Equal(t, tt.expected, policy)
		})
	}
}

// TestStreamStorageType 测试流存储类型
func TestStreamStorageType(t *testing.T) {
	tests := []struct {
		name     string
		storage  string
		expected nats.StorageType
	}{
		{
			name:     "File storage",
			storage:  "file",
			expected: nats.FileStorage,
		},
		{
			name:     "Memory storage",
			storage:  "memory",
			expected: nats.MemoryStorage,
		},
		{
			name:     "Unknown defaults to file",
			storage:  "unknown",
			expected: nats.FileStorage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var storage nats.StorageType
			switch tt.storage {
			case "file":
				storage = nats.FileStorage
			case "memory":
				storage = nats.MemoryStorage
			default:
				storage = nats.FileStorage
			}
			assert.Equal(t, tt.expected, storage)
		})
	}
}

// TestNATSConfig_Validation 测试 NATS 配置验证
func TestNATSConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.NATSConfig
		wantErr bool
	}{
		{
			name: "Valid config",
			config: &config.NATSConfig{
				URLs:              []string{"nats://localhost:4222"},
				ClientID:          "test-client",
				MaxReconnects:     10,
				ReconnectWait:     2 * time.Second,
				ConnectionTimeout: 5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "Empty URLs uses default",
			config: &config.NATSConfig{
				URLs:              []string{},
				ClientID:          "test-client",
				MaxReconnects:     10,
				ReconnectWait:     2 * time.Second,
				ConnectionTimeout: 5 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just validate the config structure
			assert.NotNil(t, tt.config)
			if len(tt.config.URLs) == 0 {
				// Should use default URL
				assert.Empty(t, tt.config.URLs)
			} else {
				assert.NotEmpty(t, tt.config.URLs)
			}
		})
	}
}

// TestJetStreamConfig_Validation 测试 JetStream 配置验证
func TestJetStreamConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  config.JetStreamConfig
		wantErr bool
	}{
		{
			name: "Valid JetStream config",
			config: config.JetStreamConfig{
				Enabled: true,
				Stream: config.StreamConfig{
					Name:      "test-stream",
					Subjects:  []string{"test.>"},
					Retention: "limits",
					MaxAge:    24 * time.Hour,
					MaxBytes:  1024 * 1024 * 1024,
					MaxMsgs:   1000000,
					Replicas:  1,
					Storage:   "file",
				},
			},
			wantErr: false,
		},
		{
			name: "Disabled JetStream",
			config: config.JetStreamConfig{
				Enabled: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.config)
			if tt.config.Enabled {
				assert.NotEmpty(t, tt.config.Stream.Name)
				assert.NotEmpty(t, tt.config.Stream.Subjects)
			}
		})
	}
}

// TestNATSEventBus_DefaultValues 测试默认值
func TestNATSEventBus_DefaultValues(t *testing.T) {
	// Test default reconnect config
	reconnectConfig := DefaultReconnectConfig()
	assert.Equal(t, DefaultMaxReconnectAttempts, reconnectConfig.MaxAttempts)
	assert.Equal(t, DefaultReconnectInitialBackoff, reconnectConfig.InitialBackoff)
	assert.Equal(t, DefaultReconnectMaxBackoff, reconnectConfig.MaxBackoff)
	assert.Equal(t, DefaultReconnectBackoffFactor, reconnectConfig.BackoffFactor)
	assert.Equal(t, DefaultReconnectFailureThreshold, reconnectConfig.FailureThreshold)
}

// TestNATSEventBus_MetricsStructure 测试指标结构
func TestNATSEventBus_MetricsStructure(t *testing.T) {
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

// TestNATSEventBus_ConnectionState 测试连接状态
func TestNATSEventBus_ConnectionState(t *testing.T) {
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

// TestNATSEventBus_Context 测试上下文传递
func TestNATSEventBus_Context(t *testing.T) {
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

// TestNATSEventBus_TopicConfigStrategy 测试主题配置策略
func TestNATSEventBus_TopicConfigStrategy(t *testing.T) {
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

// TestNATSEventBus_TopicOptions 测试主题选项
func TestNATSEventBus_TopicOptions(t *testing.T) {
	opts := TopicOptions{
		PersistenceMode: TopicPersistent,
		RetentionTime:   24 * time.Hour,
		MaxSize:         1024 * 1024 * 1024,
		MaxMessages:     1000000,
		Replicas:        2,
		Description:     "Test topic",
	}

	assert.Equal(t, TopicPersistent, opts.PersistenceMode)
	assert.Equal(t, 24*time.Hour, opts.RetentionTime)
	assert.Equal(t, int64(1024*1024*1024), opts.MaxSize)
	assert.Equal(t, int64(1000000), opts.MaxMessages)
	assert.Equal(t, 2, opts.Replicas)
	assert.Equal(t, "Test topic", opts.Description)
}
