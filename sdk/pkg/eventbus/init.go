package eventbus

import (
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// InitializeFromConfig 从配置初始化事件总线
func InitializeFromConfig(cfg *config.EventBus) error {
	if cfg == nil {
		return fmt.Errorf("eventbus config is required")
	}

	// 转换配置
	eventBusConfig := convertConfig(cfg)

	// 初始化全局事件总线
	if err := InitializeGlobal(eventBusConfig); err != nil {
		return fmt.Errorf("failed to initialize eventbus from config: %w", err)
	}

	logger.Info("EventBus initialized from config successfully", "type", cfg.Type)
	return nil
}

// convertConfig 转换配置格式
func convertConfig(cfg *config.EventBus) *EventBusConfig {
	eventBusConfig := &EventBusConfig{
		Type: cfg.Type,
		Kafka: KafkaConfig{
			Brokers:             cfg.Kafka.Brokers,
			HealthCheckInterval: cfg.Kafka.HealthCheckInterval,
			Producer: ProducerConfig{
				RequiredAcks:   cfg.Kafka.Producer.RequiredAcks,
				Compression:    cfg.Kafka.Producer.Compression,
				FlushFrequency: cfg.Kafka.Producer.FlushFrequency,
				FlushMessages:  cfg.Kafka.Producer.FlushMessages,
				RetryMax:       cfg.Kafka.Producer.RetryMax,
				Timeout:        cfg.Kafka.Producer.Timeout,
				BatchSize:      cfg.Kafka.Producer.BatchSize,
				BufferSize:     cfg.Kafka.Producer.BufferSize,
			},
			Consumer: ConsumerConfig{
				GroupID:           cfg.Kafka.Consumer.GroupID,
				AutoOffsetReset:   cfg.Kafka.Consumer.AutoOffsetReset,
				SessionTimeout:    cfg.Kafka.Consumer.SessionTimeout,
				HeartbeatInterval: cfg.Kafka.Consumer.HeartbeatInterval,
				MaxProcessingTime: cfg.Kafka.Consumer.MaxProcessingTime,
				FetchMinBytes:     cfg.Kafka.Consumer.FetchMinBytes,
				FetchMaxBytes:     cfg.Kafka.Consumer.FetchMaxBytes,
				FetchMaxWait:      cfg.Kafka.Consumer.FetchMaxWait,
			},
			Security: SecurityConfig{
				Enabled:  cfg.Kafka.Security.Enabled,
				Protocol: cfg.Kafka.Security.Protocol,
				Username: cfg.Kafka.Security.Username,
				Password: cfg.Kafka.Security.Password,
				CertFile: cfg.Kafka.Security.CertFile,
				KeyFile:  cfg.Kafka.Security.KeyFile,
				CAFile:   cfg.Kafka.Security.CAFile,
			},
		},
		NATS: NATSConfig{
			URLs:                cfg.NATS.URLs,
			ClientID:            cfg.NATS.ClientID,
			MaxReconnects:       cfg.NATS.MaxReconnects,
			ReconnectWait:       cfg.NATS.ReconnectWait,
			ConnectionTimeout:   cfg.NATS.ConnectionTimeout,
			HealthCheckInterval: cfg.NATS.HealthCheckInterval,
			JetStream:           convertJetStreamConfig(cfg.NATS.JetStream),
			Security:            convertNATSSecurityConfig(cfg.NATS.Security),
		},
		Metrics: MetricsConfig{
			Enabled:         cfg.Metrics.Enabled,
			CollectInterval: cfg.Metrics.CollectInterval,
			ExportEndpoint:  cfg.Metrics.ExportEndpoint,
		},
		Tracing: TracingConfig{
			Enabled:     cfg.Tracing.Enabled,
			ServiceName: cfg.Tracing.ServiceName,
			Endpoint:    cfg.Tracing.Endpoint,
			SampleRate:  cfg.Tracing.SampleRate,
		},
	}

	// 设置默认值
	setDefaults(eventBusConfig)
	return eventBusConfig
}

// setDefaults 设置默认值
func setDefaults(cfg *EventBusConfig) {
	// 如果没有指定类型，默认使用内存实现
	if cfg.Type == "" {
		cfg.Type = "memory"
	}

	// Kafka默认值
	if cfg.Type == "kafka" {
		if len(cfg.Kafka.Brokers) == 0 {
			cfg.Kafka.Brokers = []string{"localhost:9092"}
		}
		if cfg.Kafka.HealthCheckInterval == 0 {
			cfg.Kafka.HealthCheckInterval = 5 * time.Minute
		}
		if cfg.Kafka.Producer.RequiredAcks == 0 {
			cfg.Kafka.Producer.RequiredAcks = 1
		}
		if cfg.Kafka.Producer.Compression == "" {
			cfg.Kafka.Producer.Compression = "snappy"
		}
		if cfg.Kafka.Producer.FlushFrequency == 0 {
			cfg.Kafka.Producer.FlushFrequency = 500 * time.Millisecond
		}
		if cfg.Kafka.Producer.FlushMessages == 0 {
			cfg.Kafka.Producer.FlushMessages = 100
		}
		if cfg.Kafka.Producer.RetryMax == 0 {
			cfg.Kafka.Producer.RetryMax = 3
		}
		if cfg.Kafka.Producer.Timeout == 0 {
			cfg.Kafka.Producer.Timeout = 10 * time.Second
		}
		if cfg.Kafka.Consumer.GroupID == "" {
			cfg.Kafka.Consumer.GroupID = "jxt-eventbus-group"
		}
		if cfg.Kafka.Consumer.AutoOffsetReset == "" {
			cfg.Kafka.Consumer.AutoOffsetReset = "earliest"
		}
		if cfg.Kafka.Consumer.SessionTimeout == 0 {
			cfg.Kafka.Consumer.SessionTimeout = 30 * time.Second
		}
		if cfg.Kafka.Consumer.HeartbeatInterval == 0 {
			cfg.Kafka.Consumer.HeartbeatInterval = 3 * time.Second
		}
	}

	// NATS默认值
	if cfg.Type == "nats" {
		if len(cfg.NATS.URLs) == 0 {
			cfg.NATS.URLs = []string{"nats://localhost:4222"}
		}

		if cfg.NATS.ClientID == "" {
			cfg.NATS.ClientID = "jxt-client"
		}
		if cfg.NATS.MaxReconnects == 0 {
			cfg.NATS.MaxReconnects = 10
		}
		if cfg.NATS.ReconnectWait == 0 {
			cfg.NATS.ReconnectWait = 2 * time.Second
		}
		if cfg.NATS.ConnectionTimeout == 0 {
			cfg.NATS.ConnectionTimeout = 10 * time.Second
		}
		if cfg.NATS.HealthCheckInterval == 0 {
			cfg.NATS.HealthCheckInterval = 5 * time.Minute
		}
	}

	// Metrics默认值
	if cfg.Metrics.CollectInterval == 0 {
		cfg.Metrics.CollectInterval = 30 * time.Second
	}

	// Tracing默认值
	if cfg.Tracing.SampleRate == 0 {
		cfg.Tracing.SampleRate = 0.1
	}
}

// Setup 设置事件总线（兼容现有代码）
func Setup(cfg *config.EventBus) error {
	return InitializeFromConfig(cfg)
}

// GetDefaultMemoryConfig 获取默认内存配置
func GetDefaultMemoryConfig() *config.EventBus {
	return &config.EventBus{
		Type: "memory",
		Metrics: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
		Tracing: config.TracingConfig{
			Enabled:    false,
			SampleRate: 0.1,
		},
	}
}

// GetDefaultKafkaConfig 获取默认Kafka配置
func GetDefaultKafkaConfig(brokers []string) *config.EventBus {
	if len(brokers) == 0 {
		brokers = []string{"localhost:9092"}
	}

	return &config.EventBus{
		Type: "kafka",
		Kafka: config.KafkaConfig{
			Brokers:             brokers,
			HealthCheckInterval: 5 * time.Minute,
			Producer: config.ProducerConfig{
				RequiredAcks:   1,
				Compression:    "snappy",
				FlushFrequency: 500 * time.Millisecond,
				FlushMessages:  100,
				RetryMax:       3,
				Timeout:        10 * time.Second,
				BatchSize:      16384,
				BufferSize:     32768,
			},
			Consumer: config.ConsumerConfig{
				GroupID:           "jxt-eventbus-group",
				AutoOffsetReset:   "earliest",
				SessionTimeout:    30 * time.Second,
				HeartbeatInterval: 3 * time.Second,
				MaxProcessingTime: 5 * time.Minute,
				FetchMinBytes:     1,
				FetchMaxBytes:     1048576,
				FetchMaxWait:      500 * time.Millisecond,
			},
		},
		Metrics: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
		Tracing: config.TracingConfig{
			Enabled:    false,
			SampleRate: 0.1,
		},
	}
}

// convertJetStreamConfig 转换JetStream配置
func convertJetStreamConfig(cfg config.JetStreamConfig) JetStreamConfig {
	return JetStreamConfig{
		Enabled:        cfg.Enabled,
		Domain:         cfg.Domain,
		APIPrefix:      cfg.APIPrefix,
		PublishTimeout: cfg.PublishTimeout,
		AckWait:        cfg.AckWait,
		MaxDeliver:     cfg.MaxDeliver,
		Stream:         convertStreamConfig(cfg.Stream),
		Consumer:       convertNATSConsumerConfig(cfg.Consumer),
	}
}

// convertStreamConfig 转换流配置
func convertStreamConfig(cfg config.StreamConfig) StreamConfig {
	return StreamConfig{
		Name:      cfg.Name,
		Subjects:  cfg.Subjects,
		Retention: cfg.Retention,
		Storage:   cfg.Storage,
		Replicas:  cfg.Replicas,
		MaxAge:    cfg.MaxAge,
		MaxBytes:  cfg.MaxBytes,
		MaxMsgs:   cfg.MaxMsgs,
		Discard:   cfg.Discard,
	}
}

// convertNATSConsumerConfig 转换NATS消费者配置
func convertNATSConsumerConfig(cfg config.NATSConsumerConfig) NATSConsumerConfig {
	return NATSConsumerConfig{
		DurableName:   cfg.DurableName,
		DeliverPolicy: cfg.DeliverPolicy,
		AckPolicy:     cfg.AckPolicy,
		ReplayPolicy:  cfg.ReplayPolicy,
		MaxAckPending: cfg.MaxAckPending,
		MaxWaiting:    cfg.MaxWaiting,
		MaxDeliver:    cfg.MaxDeliver,
		BackOff:       cfg.BackOff,
	}
}

// convertNATSSecurityConfig 转换NATS安全配置
func convertNATSSecurityConfig(cfg config.NATSSecurityConfig) NATSSecurityConfig {
	return NATSSecurityConfig{
		Enabled:    cfg.Enabled,
		Token:      cfg.Token,
		Username:   cfg.Username,
		Password:   cfg.Password,
		NKeyFile:   cfg.NKeyFile,
		CredFile:   cfg.CredFile,
		CertFile:   cfg.CertFile,
		KeyFile:    cfg.KeyFile,
		CAFile:     cfg.CAFile,
		SkipVerify: cfg.SkipVerify,
	}
}
