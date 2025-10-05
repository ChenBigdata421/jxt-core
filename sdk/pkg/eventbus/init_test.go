package eventbus

import (
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
)

// TestSetDefaults_Memory 测试内存类型默认值
func TestSetDefaults_Memory(t *testing.T) {
	cfg := &EventBusConfig{}

	setDefaults(cfg)

	assert.Equal(t, "memory", cfg.Type)
}

// TestSetDefaults_Kafka 测试Kafka默认值
func TestSetDefaults_Kafka(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "kafka",
	}

	setDefaults(cfg)

	assert.Equal(t, []string{"localhost:9092"}, cfg.Kafka.Brokers)
	assert.Equal(t, 5*time.Minute, cfg.Kafka.HealthCheckInterval)
	assert.Equal(t, int16(1), cfg.Kafka.Producer.RequiredAcks)
	assert.Equal(t, "snappy", cfg.Kafka.Producer.Compression)
	assert.Equal(t, 500*time.Millisecond, cfg.Kafka.Producer.FlushFrequency)
	assert.Equal(t, 100, cfg.Kafka.Producer.FlushMessages)
	assert.Equal(t, 3, cfg.Kafka.Producer.RetryMax)
	assert.Equal(t, 10*time.Second, cfg.Kafka.Producer.Timeout)
	assert.Equal(t, "jxt-eventbus-group", cfg.Kafka.Consumer.GroupID)
	assert.Equal(t, "earliest", cfg.Kafka.Consumer.AutoOffsetReset)
	assert.Equal(t, 30*time.Second, cfg.Kafka.Consumer.SessionTimeout)
	assert.Equal(t, 3*time.Second, cfg.Kafka.Consumer.HeartbeatInterval)
}

// TestSetDefaults_NATS 测试NATS默认值
func TestSetDefaults_NATS(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "nats",
	}

	setDefaults(cfg)

	assert.Equal(t, []string{"nats://localhost:4222"}, cfg.NATS.URLs)
	assert.Equal(t, "jxt-client", cfg.NATS.ClientID)
	assert.Equal(t, 10, cfg.NATS.MaxReconnects)
	assert.Equal(t, 2*time.Second, cfg.NATS.ReconnectWait)
	assert.Equal(t, 10*time.Second, cfg.NATS.ConnectionTimeout)
	assert.Equal(t, 5*time.Minute, cfg.NATS.HealthCheckInterval)
	assert.True(t, cfg.NATS.JetStream.Enabled)
	assert.Equal(t, "JXT_STREAM", cfg.NATS.JetStream.Stream.Name)
	assert.Equal(t, "limits", cfg.NATS.JetStream.Stream.Retention)
	assert.Equal(t, "file", cfg.NATS.JetStream.Stream.Storage)
	assert.Equal(t, "jxt-consumer", cfg.NATS.JetStream.Consumer.DurableName)
	assert.Equal(t, "explicit", cfg.NATS.JetStream.Consumer.AckPolicy)
}

// TestSetDefaults_Metrics 测试Metrics默认值
func TestSetDefaults_Metrics(t *testing.T) {
	cfg := &EventBusConfig{}

	setDefaults(cfg)

	assert.Equal(t, 30*time.Second, cfg.Metrics.CollectInterval)
}

// TestSetDefaults_Tracing 测试Tracing默认值
func TestSetDefaults_Tracing(t *testing.T) {
	cfg := &EventBusConfig{}

	setDefaults(cfg)

	assert.Equal(t, 0.1, cfg.Tracing.SampleRate)
}

// TestSetDefaults_KafkaPartial 测试Kafka部分配置
func TestSetDefaults_KafkaPartial(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "kafka",
		Kafka: KafkaConfig{
			Brokers: []string{"custom:9092"},
			Producer: ProducerConfig{
				RequiredAcks: 2,
			},
		},
	}

	setDefaults(cfg)

	// 自定义值应该保留
	assert.Equal(t, []string{"custom:9092"}, cfg.Kafka.Brokers)
	assert.Equal(t, int16(2), cfg.Kafka.Producer.RequiredAcks)

	// 未设置的值应该使用默认值
	assert.Equal(t, "snappy", cfg.Kafka.Producer.Compression)
	assert.Equal(t, 500*time.Millisecond, cfg.Kafka.Producer.FlushFrequency)
}

// TestSetDefaults_NATSPartial 测试NATS部分配置
func TestSetDefaults_NATSPartial(t *testing.T) {
	cfg := &EventBusConfig{
		Type: "nats",
		NATS: NATSConfig{
			URLs:     []string{"nats://custom:4222"},
			ClientID: "custom-client",
		},
	}

	setDefaults(cfg)

	// 自定义值应该保留
	assert.Equal(t, []string{"nats://custom:4222"}, cfg.NATS.URLs)
	assert.Equal(t, "custom-client", cfg.NATS.ClientID)

	// 未设置的值应该使用默认值
	assert.Equal(t, 10, cfg.NATS.MaxReconnects)
	assert.Equal(t, 2*time.Second, cfg.NATS.ReconnectWait)
}

// TestInitializeFromConfig_NilConfig 测试nil配置
func TestInitializeFromConfig_NilConfig(t *testing.T) {
	err := InitializeFromConfig(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus config is required")
}

// TestConvertConfig_Kafka 测试转换Kafka配置
func TestConvertConfig_Kafka(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type:        "kafka",
		ServiceName: "test-service",
		Kafka: config.KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Producer: config.ProducerConfig{
				RequiredAcks:   1,
				Compression:    "snappy",
				FlushFrequency: 500 * time.Millisecond,
			},
			Consumer: config.ConsumerConfig{
				GroupID:         "test-group",
				AutoOffsetReset: "earliest",
			},
		},
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:    "health",
				Interval: 30 * time.Second,
			},
		},
		Monitoring: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
	}

	result := convertConfig(cfg)

	assert.Equal(t, "kafka", result.Type)
	assert.Equal(t, []string{"localhost:9092"}, result.Kafka.Brokers)
	assert.Equal(t, int(1), int(result.Kafka.Producer.RequiredAcks))
	assert.Equal(t, "snappy", result.Kafka.Producer.Compression)
	assert.True(t, result.Enterprise.HealthCheck.Enabled)
	assert.True(t, result.Metrics.Enabled)
}

// TestConvertConfig_NATS 测试转换NATS配置
func TestConvertConfig_NATS(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type:        "nats",
		ServiceName: "test-service",
		NATS: config.NATSConfig{
			URLs:     []string{"nats://localhost:4222"},
			ClientID: "test-client",
			JetStream: config.JetStreamConfig{
				Enabled: true,
				Domain:  "test-domain",
			},
		},
	}

	result := convertConfig(cfg)

	assert.Equal(t, "nats", result.Type)
	assert.Equal(t, []string{"nats://localhost:4222"}, result.NATS.URLs)
	assert.Equal(t, "test-client", result.NATS.ClientID)
	assert.True(t, result.NATS.JetStream.Enabled)
	assert.Equal(t, "test-domain", result.NATS.JetStream.Domain)
}

// TestConvertConfig_EnterpriseFeatures 测试转换企业特性
func TestConvertConfig_EnterpriseFeatures(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type: "memory",
		Publisher: config.PublisherConfig{
			BacklogDetection: config.PublisherBacklogDetectionConfig{
				Enabled:           true,
				MaxQueueDepth:     1000,
				MaxPublishLatency: 5 * time.Minute,
			},
			MaxReconnectAttempts: 3,
			InitialBackoff:       1 * time.Second,
			MaxBackoff:           30 * time.Second,
		},
		Subscriber: config.SubscriberConfig{
			BacklogDetection: config.SubscriberBacklogDetectionConfig{
				Enabled:          true,
				MaxLagThreshold:  2000,
				MaxTimeThreshold: 10 * time.Minute,
			},
			RateLimit: config.RateLimitConfig{
				RatePerSecond: 100,
				BurstSize:     200,
			},
			ErrorHandling: config.ErrorHandlingConfig{
				DeadLetterTopic:  "dlq",
				MaxRetryAttempts: 5,
				RetryBackoffBase: 1 * time.Second,
				RetryBackoffMax:  60 * time.Second,
			},
		},
	}

	result := convertConfig(cfg)

	// 检查发布端企业特性
	assert.True(t, result.Enterprise.Publisher.BacklogDetection.Enabled)
	assert.True(t, result.Enterprise.Publisher.RetryPolicy.Enabled)
	assert.Equal(t, 3, result.Enterprise.Publisher.RetryPolicy.MaxRetries)

	// 检查订阅端企业特性
	assert.True(t, result.Enterprise.Subscriber.BacklogDetection.Enabled)
	assert.Equal(t, float64(100), result.Enterprise.Subscriber.RateLimit.RatePerSecond)
	assert.True(t, result.Enterprise.Subscriber.DeadLetter.Enabled)
	assert.Equal(t, "dlq", result.Enterprise.Subscriber.DeadLetter.Topic)
	assert.Equal(t, 5, result.Enterprise.Subscriber.DeadLetter.MaxRetries)
}

// TestConvertConfig_Security 测试转换安全配置
func TestConvertConfig_Security(t *testing.T) {
	cfg := &config.EventBusConfig{
		Type: "kafka",
		Security: config.SecurityConfig{
			Enabled:  true,
			Protocol: "SASL_SSL",
			Username: "user",
			Password: "pass",
			CertFile: "/path/to/cert",
			KeyFile:  "/path/to/key",
			CAFile:   "/path/to/ca",
		},
	}

	result := convertConfig(cfg)

	assert.True(t, result.Kafka.Security.Enabled)
	assert.Equal(t, "SASL_SSL", result.Kafka.Security.Protocol)
	assert.Equal(t, "user", result.Kafka.Security.Username)
	assert.Equal(t, "pass", result.Kafka.Security.Password)
	assert.Equal(t, "/path/to/cert", result.Kafka.Security.CertFile)
}
