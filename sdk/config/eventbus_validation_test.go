package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestEventBusConfig_Validate_BasicConfig 测试基础配置验证
func TestEventBusConfig_Validate_BasicConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  EventBusConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid basic config",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
			},
			wantErr: false,
		},
		{
			name: "missing type",
			config: EventBusConfig{
				ServiceName: "test-service",
			},
			wantErr: true,
			errMsg:  "eventbus type is required",
		},
		{
			name: "invalid type",
			config: EventBusConfig{
				Type:        "invalid",
				ServiceName: "test-service",
			},
			wantErr: true,
			errMsg:  "unsupported eventbus type",
		},
		{
			name: "missing service name",
			config: EventBusConfig{
				Type: "memory",
			},
			wantErr: true,
			errMsg:  "service name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetDefaults()
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEventBusConfig_Validate_HealthCheck 测试健康检查配置验证
func TestEventBusConfig_Validate_HealthCheck(t *testing.T) {
	tests := []struct {
		name    string
		config  EventBusConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid health check config",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				Publisher: PublisherConfig{
					PublishTimeout: 10 * time.Second,
				},
				Subscriber: SubscriberConfig{
					MaxConcurrency: 10,
					ProcessTimeout: 30 * time.Second,
				},
				HealthCheck: HealthCheckConfig{
					Enabled: true,
					Publisher: HealthCheckPublisherConfig{
						Interval:         2 * time.Minute,
						Timeout:          10 * time.Second,
						FailureThreshold: 3,
						MessageTTL:       5 * time.Minute,
					},
					Subscriber: HealthCheckSubscriberConfig{
						MonitorInterval:   30 * time.Second,
						WarningThreshold:  3,
						ErrorThreshold:    5,
						CriticalThreshold: 10,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid publisher interval",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				HealthCheck: HealthCheckConfig{
					Enabled: true,
					Publisher: HealthCheckPublisherConfig{
						Interval: 0,
					},
				},
			},
			wantErr: true,
			errMsg:  "health check publisher interval must be positive",
		},
		{
			name: "invalid threshold order",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				HealthCheck: HealthCheckConfig{
					Enabled: true,
					Publisher: HealthCheckPublisherConfig{
						Interval:         2 * time.Minute,
						Timeout:          10 * time.Second,
						FailureThreshold: 3,
						MessageTTL:       5 * time.Minute,
					},
					Subscriber: HealthCheckSubscriberConfig{
						MonitorInterval:   30 * time.Second,
						WarningThreshold:  10,
						ErrorThreshold:    5,
						CriticalThreshold: 3,
					},
				},
			},
			wantErr: true,
			errMsg:  "warning threshold must be less than error threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEventBusConfig_Validate_Publisher 测试发布端配置验证
func TestEventBusConfig_Validate_Publisher(t *testing.T) {
	tests := []struct {
		name    string
		config  EventBusConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid publisher config",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				Publisher: PublisherConfig{
					MaxReconnectAttempts: 5,
					InitialBackoff:       1 * time.Second,
					MaxBackoff:           30 * time.Second,
					PublishTimeout:       10 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid backoff order",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				Publisher: PublisherConfig{
					InitialBackoff: 30 * time.Second,
					MaxBackoff:     1 * time.Second,
					PublishTimeout: 10 * time.Second,
				},
			},
			wantErr: true,
			errMsg:  "publisher initial backoff must be less than or equal to max backoff",
		},
		{
			name: "invalid rate limit config",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				Publisher: PublisherConfig{
					PublishTimeout: 10 * time.Second,
					RateLimit: RateLimitConfig{
						Enabled:       true,
						RatePerSecond: 0,
					},
				},
			},
			wantErr: true,
			errMsg:  "publisher rate limit rate per second must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetDefaults()
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEventBusConfig_Validate_Subscriber 测试订阅端配置验证
func TestEventBusConfig_Validate_Subscriber(t *testing.T) {
	tests := []struct {
		name    string
		config  EventBusConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid subscriber config",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				Subscriber: SubscriberConfig{
					MaxConcurrency: 10,
					ProcessTimeout: 30 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid max concurrency",
			config: EventBusConfig{
				Type:        "memory",
				ServiceName: "test-service",
				Publisher: PublisherConfig{
					PublishTimeout: 10 * time.Second,
				},
				Subscriber: SubscriberConfig{
					MaxConcurrency: -1, // 负数会触发错误
					ProcessTimeout: 30 * time.Second,
				},
			},
			wantErr: true,
			errMsg:  "subscriber max concurrency must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetDefaults()
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEventBusConfig_Validate_Kafka 测试Kafka配置验证
func TestEventBusConfig_Validate_Kafka(t *testing.T) {
	tests := []struct {
		name    string
		config  EventBusConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid kafka config",
			config: EventBusConfig{
				Type:        "kafka",
				ServiceName: "test-service",
				Kafka: KafkaConfig{
					Brokers: []string{"localhost:9092"},
					Producer: ProducerConfig{
						RequiredAcks: 1,
						Compression:  "gzip",
					},
					Consumer: ConsumerConfig{
						GroupID:         "test-group",
						AutoOffsetReset: "earliest",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing brokers",
			config: EventBusConfig{
				Type:        "kafka",
				ServiceName: "test-service",
			},
			wantErr: true,
			errMsg:  "kafka brokers are required",
		},
		{
			name: "invalid compression",
			config: EventBusConfig{
				Type:        "kafka",
				ServiceName: "test-service",
				Kafka: KafkaConfig{
					Brokers: []string{"localhost:9092"},
					Producer: ProducerConfig{
						Compression: "invalid",
					},
					Consumer: ConsumerConfig{
						GroupID: "test-group",
					},
				},
			},
			wantErr: true,
			errMsg:  "unsupported kafka compression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetDefaults()
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEventBusConfig_SetDefaults 测试默认值设置
func TestEventBusConfig_SetDefaults(t *testing.T) {
	t.Run("basic defaults", func(t *testing.T) {
		cfg := &EventBusConfig{
			ServiceName: "test-service",
		}
		cfg.SetDefaults()

		// 验证基础默认值
		assert.Equal(t, "memory", cfg.Type)
		assert.Equal(t, 5, cfg.Publisher.MaxReconnectAttempts)
		assert.Equal(t, 1*time.Second, cfg.Publisher.InitialBackoff)
		assert.Equal(t, 30*time.Second, cfg.Publisher.MaxBackoff)
		assert.Equal(t, 10*time.Second, cfg.Publisher.PublishTimeout)
		assert.Equal(t, 10, cfg.Subscriber.MaxConcurrency)
		assert.Equal(t, 30*time.Second, cfg.Subscriber.ProcessTimeout)
	})

	t.Run("kafka defaults", func(t *testing.T) {
		cfg := &EventBusConfig{
			Type:        "kafka",
			ServiceName: "test-service",
			Kafka: KafkaConfig{
				Brokers: []string{"localhost:9092"},
				Consumer: ConsumerConfig{
					GroupID: "test-group",
				},
			},
		}
		cfg.SetDefaults()

		// 验证Kafka默认值
		assert.Equal(t, 1, cfg.Kafka.Producer.RequiredAcks)
		assert.Equal(t, "snappy", cfg.Kafka.Producer.Compression)
		assert.Equal(t, 500*time.Millisecond, cfg.Kafka.Producer.FlushFrequency)
		assert.Equal(t, 100, cfg.Kafka.Producer.FlushMessages)
		assert.Equal(t, 10*time.Second, cfg.Kafka.Producer.Timeout)
		assert.Equal(t, "latest", cfg.Kafka.Consumer.AutoOffsetReset)
		assert.Equal(t, 30*time.Second, cfg.Kafka.Consumer.SessionTimeout)
		assert.Equal(t, 3*time.Second, cfg.Kafka.Consumer.HeartbeatInterval)
	})

	t.Run("nats defaults", func(t *testing.T) {
		cfg := &EventBusConfig{
			Type:        "nats",
			ServiceName: "test-service",
			NATS: NATSConfig{
				URLs: []string{"nats://localhost:4222"},
				JetStream: JetStreamConfig{
					Enabled: true,
					Stream: StreamConfig{
						Name:     "test-stream",
						Subjects: []string{"test.>"},
					},
				},
			},
		}
		cfg.SetDefaults()

		// 验证NATS默认值
		assert.Equal(t, 10, cfg.NATS.MaxReconnects)
		assert.Equal(t, 2*time.Second, cfg.NATS.ReconnectWait)
		assert.Equal(t, 10*time.Second, cfg.NATS.ConnectionTimeout)
		assert.Equal(t, 10*time.Second, cfg.NATS.JetStream.PublishTimeout)
		assert.Equal(t, 30*time.Second, cfg.NATS.JetStream.AckWait)
		assert.Equal(t, 5, cfg.NATS.JetStream.MaxDeliver)
		assert.Equal(t, "limits", cfg.NATS.JetStream.Stream.Retention)
		assert.Equal(t, "file", cfg.NATS.JetStream.Stream.Storage)
		assert.Equal(t, 1, cfg.NATS.JetStream.Stream.Replicas)
		assert.Equal(t, "all", cfg.NATS.JetStream.Consumer.DeliverPolicy)
		assert.Equal(t, "explicit", cfg.NATS.JetStream.Consumer.AckPolicy)
	})

	t.Run("memory defaults", func(t *testing.T) {
		cfg := &EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
		}
		cfg.SetDefaults()

		// 验证Memory默认值
		assert.Equal(t, 1000, cfg.Memory.MaxChannelSize)
		assert.Equal(t, 100, cfg.Memory.BufferSize)
	})

	t.Run("health check defaults", func(t *testing.T) {
		cfg := &EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			HealthCheck: HealthCheckConfig{
				Enabled: true,
			},
		}
		cfg.SetDefaults()

		// 验证健康检查默认值
		assert.Equal(t, 2*time.Minute, cfg.HealthCheck.Publisher.Interval)
		assert.Equal(t, 10*time.Second, cfg.HealthCheck.Publisher.Timeout)
		assert.Equal(t, 3, cfg.HealthCheck.Publisher.FailureThreshold)
		assert.Equal(t, 5*time.Minute, cfg.HealthCheck.Publisher.MessageTTL)
		assert.Equal(t, 30*time.Second, cfg.HealthCheck.Subscriber.MonitorInterval)
		assert.Equal(t, 3, cfg.HealthCheck.Subscriber.WarningThreshold)
		assert.Equal(t, 5, cfg.HealthCheck.Subscriber.ErrorThreshold)
		assert.Equal(t, 10, cfg.HealthCheck.Subscriber.CriticalThreshold)
	})

	t.Run("publisher enterprise defaults", func(t *testing.T) {
		cfg := &EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			Publisher: PublisherConfig{
				BacklogDetection: PublisherBacklogDetectionConfig{
					Enabled: true,
				},
				RateLimit: RateLimitConfig{
					Enabled: true,
				},
				ErrorHandling: ErrorHandlingConfig{
					DeadLetterTopic: "dlq",
				},
			},
		}
		cfg.SetDefaults()

		// 验证发布端企业特性默认值
		assert.Equal(t, int64(10000), cfg.Publisher.BacklogDetection.MaxQueueDepth)
		assert.Equal(t, 1*time.Second, cfg.Publisher.BacklogDetection.MaxPublishLatency)
		assert.Equal(t, 1000.0, cfg.Publisher.BacklogDetection.RateThreshold)
		assert.Equal(t, 10*time.Second, cfg.Publisher.BacklogDetection.CheckInterval)
		assert.Equal(t, 1000.0, cfg.Publisher.RateLimit.RatePerSecond)
		assert.Equal(t, 100, cfg.Publisher.RateLimit.BurstSize)
		assert.Equal(t, 3, cfg.Publisher.ErrorHandling.MaxRetryAttempts)
		assert.Equal(t, 1*time.Second, cfg.Publisher.ErrorHandling.RetryBackoffBase)
		assert.Equal(t, 30*time.Second, cfg.Publisher.ErrorHandling.RetryBackoffMax)
	})

	t.Run("subscriber enterprise defaults", func(t *testing.T) {
		cfg := &EventBusConfig{
			Type:        "memory",
			ServiceName: "test-service",
			Subscriber: SubscriberConfig{
				BacklogDetection: SubscriberBacklogDetectionConfig{
					Enabled: true,
				},
				RateLimit: RateLimitConfig{
					Enabled: true,
				},
				ErrorHandling: ErrorHandlingConfig{
					DeadLetterTopic: "dlq",
				},
			},
		}
		cfg.SetDefaults()

		// 验证订阅端企业特性默认值
		assert.Equal(t, int64(10000), cfg.Subscriber.BacklogDetection.MaxLagThreshold)
		assert.Equal(t, 5*time.Minute, cfg.Subscriber.BacklogDetection.MaxTimeThreshold)
		assert.Equal(t, 30*time.Second, cfg.Subscriber.BacklogDetection.CheckInterval)
		assert.Equal(t, 1000.0, cfg.Subscriber.RateLimit.RatePerSecond)
		assert.Equal(t, 100, cfg.Subscriber.RateLimit.BurstSize)
		assert.Equal(t, 3, cfg.Subscriber.ErrorHandling.MaxRetryAttempts)
		assert.Equal(t, 1*time.Second, cfg.Subscriber.ErrorHandling.RetryBackoffBase)
		assert.Equal(t, 30*time.Second, cfg.Subscriber.ErrorHandling.RetryBackoffMax)
	})
}
