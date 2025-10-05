package eventbus

import (
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

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

	// NATS默认值（智能双模式）
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

		// 默认启用JetStream（可配置关闭）
		if !cfg.NATS.JetStream.Enabled {
			// 如果未明确配置，默认启用JetStream
			cfg.NATS.JetStream.Enabled = true
		}

		// JetStream默认配置（仅在启用时设置）
		if cfg.NATS.JetStream.Enabled {
			if cfg.NATS.JetStream.Stream.Name == "" {
				cfg.NATS.JetStream.Stream.Name = "JXT_STREAM"
			}
			if len(cfg.NATS.JetStream.Stream.Subjects) == 0 {
				cfg.NATS.JetStream.Stream.Subjects = []string{"persistent.>", "order.>", "payment.>", "audit.>", "critical.>", "durable.>"}
			}
			if cfg.NATS.JetStream.Stream.Retention == "" {
				cfg.NATS.JetStream.Stream.Retention = "limits"
			}
			if cfg.NATS.JetStream.Stream.Storage == "" {
				cfg.NATS.JetStream.Stream.Storage = "file"
			}
			if cfg.NATS.JetStream.Consumer.DurableName == "" {
				cfg.NATS.JetStream.Consumer.DurableName = "jxt-consumer"
			}
			if cfg.NATS.JetStream.Consumer.AckPolicy == "" {
				cfg.NATS.JetStream.Consumer.AckPolicy = "explicit"
			}
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

// ==========================================================================
// 新配置结构支持
// ==========================================================================

// InitializeFromConfig 从配置初始化事件总线
func InitializeFromConfig(cfg *config.EventBusConfig) error {
	if cfg == nil {
		return fmt.Errorf("eventbus config is required")
	}

	// 设置默认值（必须在验证之前）
	cfg.SetDefaults()

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid unified config: %w", err)
	}

	// 设置全局配置
	SetGlobalConfig(cfg)

	// 转换配置
	eventBusConfig := convertConfig(cfg)

	// 初始化全局事件总线
	if err := InitializeGlobal(eventBusConfig); err != nil {
		return fmt.Errorf("failed to initialize eventbus from unified config: %w", err)
	}

	logger.Info("EventBus initialized from unified config successfully",
		"type", cfg.Type,
		"serviceName", cfg.ServiceName,
		"healthCheckEnabled", cfg.HealthCheck.Enabled,
		"publisherConfigured", cfg.Publisher.PublishTimeout > 0,
		"subscriberConfigured", cfg.Subscriber.MaxConcurrency > 0)
	return nil
}

// convertConfig 转换配置格式
func convertConfig(cfg *config.EventBusConfig) *EventBusConfig {
	eventBusConfig := &EventBusConfig{
		Type: cfg.Type,
	}

	// 转换健康检查配置到企业特性
	eventBusConfig.Enterprise.HealthCheck = HealthCheckConfig{
		Enabled:          cfg.HealthCheck.Enabled,
		Topic:            cfg.HealthCheck.Publisher.Topic,
		Interval:         cfg.HealthCheck.Publisher.Interval,
		Timeout:          cfg.HealthCheck.Publisher.Timeout,
		FailureThreshold: cfg.HealthCheck.Publisher.FailureThreshold,
		MessageTTL:       cfg.HealthCheck.Publisher.MessageTTL,
	}

	// 转换发布端企业特性（根据现有结构）
	eventBusConfig.Enterprise.Publisher = PublisherEnterpriseConfig{
		BacklogDetection: cfg.Publisher.BacklogDetection,
		MessageFormatter: MessageFormatterConfig{
			Enabled: true,
			Type:    "json", // 默认使用JSON格式
		},
		PublishCallback: PublishCallbackConfig{
			Enabled: false, // 默认关闭
		},
		RetryPolicy: RetryPolicyConfig{
			Enabled:         cfg.Publisher.MaxReconnectAttempts > 0,
			MaxRetries:      cfg.Publisher.MaxReconnectAttempts,
			InitialInterval: cfg.Publisher.InitialBackoff,
			MaxInterval:     cfg.Publisher.MaxBackoff,
			Multiplier:      2.0, // 默认倍数
		},
	}

	// 转换订阅端企业特性（根据现有结构）
	eventBusConfig.Enterprise.Subscriber = SubscriberEnterpriseConfig{
		BacklogDetection: cfg.Subscriber.BacklogDetection,
		RateLimit: RateLimitConfig{
			RatePerSecond: cfg.Subscriber.RateLimit.RatePerSecond,
			BurstSize:     cfg.Subscriber.RateLimit.BurstSize,
		},
		DeadLetter: DeadLetterConfig{
			Enabled:    cfg.Subscriber.ErrorHandling.DeadLetterTopic != "",
			Topic:      cfg.Subscriber.ErrorHandling.DeadLetterTopic,
			MaxRetries: cfg.Subscriber.ErrorHandling.MaxRetryAttempts,
		},
		MessageRouter: MessageRouterConfig{
			Enabled: false,  // 默认关闭
			Type:    "hash", // 默认哈希路由
		},
		ErrorHandler: ErrorHandlerConfig{
			Enabled: cfg.Subscriber.ErrorHandling.MaxRetryAttempts > 0,
			Type:    "retry", // 默认重试策略
		},
	}

	// 转换监控配置
	eventBusConfig.Metrics = MetricsConfig{
		Enabled:         cfg.Monitoring.Enabled,
		CollectInterval: cfg.Monitoring.CollectInterval,
		ExportEndpoint:  cfg.Monitoring.ExportEndpoint,
	}

	// 根据类型转换特定配置
	switch cfg.Type {
	case "kafka":
		eventBusConfig.Kafka = KafkaConfig{
			Brokers: cfg.Kafka.Brokers,
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
				Enabled:  cfg.Security.Enabled,
				Protocol: cfg.Security.Protocol,
				Username: cfg.Security.Username,
				Password: cfg.Security.Password,
				CertFile: cfg.Security.CertFile,
				KeyFile:  cfg.Security.KeyFile,
				CAFile:   cfg.Security.CAFile,
			},
		}
	case "nats":
		eventBusConfig.NATS = NATSConfig{
			URLs:              cfg.NATS.URLs,
			ClientID:          cfg.NATS.ClientID,
			MaxReconnects:     cfg.NATS.MaxReconnects,
			ReconnectWait:     cfg.NATS.ReconnectWait,
			ConnectionTimeout: cfg.NATS.ConnectionTimeout,
			// JetStream配置需要单独转换，这里先使用基本配置
			JetStream: JetStreamConfig{
				Enabled: cfg.NATS.JetStream.Enabled,
				Domain:  cfg.NATS.JetStream.Domain,
			},
			Security: NATSSecurityConfig{
				Enabled:  cfg.Security.Enabled,
				Username: cfg.Security.Username,
				Password: cfg.Security.Password,
				CertFile: cfg.Security.CertFile,
				KeyFile:  cfg.Security.KeyFile,
				CAFile:   cfg.Security.CAFile,
			},
		}
	}

	return eventBusConfig
}
