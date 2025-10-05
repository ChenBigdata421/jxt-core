package main

import (
	"log"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// UnifiedConfigExample 展示如何使用新的UnifiedEventBusConfig
func main() {
	log.Println("=== jxt-core EventBus UnifiedConfig 使用示例 ===")

	// 示例1：Kafka配置
	kafkaExample()

	// 示例2：NATS配置
	natsExample()

	// 示例3：配置迁移示例
	migrationExample()
}

// kafkaExample Kafka统一配置示例
func kafkaExample() {
	log.Println("\n--- Kafka 统一配置示例 ---")

	// 创建统一配置
	cfg := &config.EventBusConfig{
		// 基础配置
		Type:        "kafka",
		ServiceName: "kafka-demo-service",

		// 统一健康检查配置
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:            "health-check",   // 可选，默认自动生成
				Interval:         2 * time.Minute,  // 每2分钟发布一次健康检查
				Timeout:          10 * time.Second, // 10秒超时
				FailureThreshold: 3,                // 连续失败3次触发重连
				MessageTTL:       5 * time.Minute,  // 消息5分钟过期
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				MonitorInterval:   30 * time.Second, // 30秒检查一次
				WarningThreshold:  3,                // 3次未收到消息发出警告
				ErrorThreshold:    5,                // 5次未收到消息报错
				CriticalThreshold: 10,               // 10次未收到消息严重告警
			},
		},

		// 安全配置
		Security: config.SecurityConfig{
			Enabled:  false, // 开发环境关闭安全认证
			Protocol: "PLAINTEXT",
		},

		// 发布端配置
		Publisher: config.PublisherConfig{
			MaxReconnectAttempts: 5,                // 最大重连5次
			InitialBackoff:       1 * time.Second,  // 初始退避1秒
			MaxBackoff:           30 * time.Second, // 最大退避30秒
			PublishTimeout:       10 * time.Second, // 发布超时10秒

			// 企业特性：积压检测
			BacklogDetection: config.BacklogDetectionConfig{
				Enabled:          true,
				MaxLagThreshold:  1000,             // 最大积压1000条消息
				MaxTimeThreshold: 5 * time.Minute,  // 最大积压5分钟
				CheckInterval:    30 * time.Second, // 30秒检查一次
			},

			// 企业特性：流量控制
			RateLimit: config.RateLimitConfig{
				Enabled:       true,
				RatePerSecond: 100.0, // 每秒最多100条消息
				BurstSize:     200,   // 突发最多200条
			},

			// 企业特性：错误处理
			ErrorHandling: config.ErrorHandlingConfig{
				DeadLetterTopic:  "dead-letter-queue",
				MaxRetryAttempts: 3,
				RetryBackoffBase: 1 * time.Second,
				RetryBackoffMax:  10 * time.Second,
			},
		},

		// 订阅端配置
		Subscriber: config.SubscriberConfig{
			MaxConcurrency: 10,               // 最大并发10个goroutine
			ProcessTimeout: 30 * time.Second, // 处理超时30秒

			// 企业特性：积压检测
			BacklogDetection: config.BacklogDetectionConfig{
				Enabled:          true,
				MaxLagThreshold:  500,              // 订阅端积压阈值更低
				MaxTimeThreshold: 2 * time.Minute,  // 订阅端时间阈值更短
				CheckInterval:    15 * time.Second, // 更频繁的检查
			},

			// 企业特性：流量控制
			RateLimit: config.RateLimitConfig{
				Enabled:       true,
				RatePerSecond: 50.0, // 订阅端处理速度
				BurstSize:     100,
			},

			// 企业特性：错误处理
			ErrorHandling: config.ErrorHandlingConfig{
				DeadLetterTopic:  "subscriber-dead-letter",
				MaxRetryAttempts: 2, // 订阅端重试次数更少
				RetryBackoffBase: 500 * time.Millisecond,
				RetryBackoffMax:  5 * time.Second,
			},
		},

		// Kafka特定配置
		Kafka: config.KafkaSpecificConfig{
			Brokers: []string{"localhost:9092"},
			Producer: config.ProducerConfig{
				RequiredAcks:   1,        // 等待leader确认
				Compression:    "snappy", // 使用snappy压缩
				FlushFrequency: 100 * time.Millisecond,
				FlushMessages:  100,
				RetryMax:       3,
				Timeout:        10 * time.Second,
				BatchSize:      16384,
				BufferSize:     33554432,
			},
			Consumer: config.ConsumerConfig{
				GroupID:           "kafka-demo-group",
				AutoOffsetReset:   "latest",
				SessionTimeout:    30 * time.Second,
				HeartbeatInterval: 3 * time.Second,
				MaxProcessingTime: 5 * time.Minute,
				FetchMinBytes:     1,
				FetchMaxBytes:     1048576,
				FetchMaxWait:      500 * time.Millisecond,
			},
			Net: config.NetConfig{
				DialTimeout:  30 * time.Second,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
		},

		// 监控配置
		Monitoring: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
			ExportEndpoint:  "http://localhost:8080/metrics",
		},
	}

	// 设置默认值
	cfg.SetDefaults()

	// 验证配置
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Kafka配置验证失败: %v", err)
	}

	// 初始化EventBus
	if err := eventbus.InitializeFromUnifiedConfig(cfg); err != nil {
		log.Fatalf("Kafka EventBus初始化失败: %v", err)
	}

	log.Println("✅ Kafka EventBus 初始化成功！")
}

// natsExample NATS统一配置示例
func natsExample() {
	log.Println("\n--- NATS 统一配置示例 ---")

	cfg := &config.UnifiedEventBusConfig{
		Type:        "nats",
		ServiceName: "nats-demo-service",

		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Sender: config.HealthCheckSenderConfig{
				Interval:         1 * time.Minute, // NATS检查更频繁
				Timeout:          5 * time.Second,
				FailureThreshold: 2,
				MessageTTL:       3 * time.Minute,
			},
			Subscriber: config.HealthCheckSubscriberConfig{
				Enabled:         true,
				MonitorInterval: 15 * time.Second,
			},
		},

		Security: config.SecurityConfig{
			Enabled: false,
		},

		Publisher: config.PublisherConfig{
			MaxReconnectAttempts: 10, // NATS重连次数更多
			InitialBackoff:       500 * time.Millisecond,
			MaxBackoff:           10 * time.Second,
			PublishTimeout:       5 * time.Second,
		},

		Subscriber: config.SubscriberConfig{
			MaxConcurrency: 20, // NATS并发更高
			ProcessTimeout: 15 * time.Second,
		},

		NATS: config.NATSSpecificConfig{
			URLs:              []string{"nats://localhost:4222"},
			ClientID:          "nats-demo-client",
			MaxReconnects:     10,
			ReconnectWait:     2 * time.Second,
			ConnectionTimeout: 10 * time.Second,
			JetStream: config.JetStreamConfig{
				Enabled:        true,
				Domain:         "",
				APIPrefix:      "",
				PublishTimeout: 5 * time.Second,
				AckWait:        30 * time.Second,
				MaxDeliver:     3,
				Stream: config.StreamConfig{
					Name:      "demo-stream",
					Subjects:  []string{"demo.>"},
					Retention: "limits",
					Storage:   "file",
					Replicas:  1,
					MaxAge:    24 * time.Hour,
					MaxBytes:  1024 * 1024 * 1024, // 1GB
					MaxMsgs:   1000000,
					Discard:   "old",
				},
				Consumer: config.NATSConsumerConfig{
					DurableName:   "demo-consumer",
					DeliverPolicy: "all",
					AckPolicy:     "explicit",
					ReplayPolicy:  "instant",
					MaxAckPending: 1000,
					MaxWaiting:    512,
					MaxDeliver:    3,
					BackOff:       []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
				},
			},
			Security: config.NATSSecurityConfig{
				Enabled: false,
			},
		},

		Monitoring: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 15 * time.Second,
			ExportEndpoint:  "http://localhost:8081/metrics",
		},
	}

	cfg.SetDefaults()

	if err := cfg.Validate(); err != nil {
		log.Fatalf("NATS配置验证失败: %v", err)
	}

	log.Println("✅ NATS 统一配置创建成功！")
	log.Printf("配置详情: Type=%s, ServiceName=%s, HealthCheck=%t",
		cfg.Type, cfg.ServiceName, cfg.HealthCheck.Enabled)
}

// migrationExample 配置迁移示例
func migrationExample() {
	log.Println("\n--- 配置迁移示例 ---")

	// 旧的高级配置
	oldCfg := &config.AdvancedEventBusConfig{
		EventBus: config.EventBus{
			Type: "kafka",
			Kafka: config.KafkaConfig{
				Brokers:             []string{"localhost:9092"},
				HealthCheckInterval: 2 * time.Minute,
			},
		},
		ServiceName: "migration-demo",
		HealthCheck: config.HealthCheckConfig{
			Enabled:          true,
			Interval:         2 * time.Minute,
			Timeout:          10 * time.Second,
			FailureThreshold: 3,
			MessageTTL:       5 * time.Minute,
		},
	}

	log.Println("旧配置创建完成")

	// 转换为新配置
	newCfg := config.ConvertToUnified(oldCfg)
	if newCfg == nil {
		log.Fatal("配置转换失败")
	}

	log.Println("✅ 配置迁移成功！")
	log.Printf("新配置: Type=%s, ServiceName=%s", newCfg.Type, newCfg.ServiceName)
	log.Printf("健康检查: Enabled=%t, Sender.Interval=%v",
		newCfg.HealthCheck.Enabled, newCfg.HealthCheck.Sender.Interval)

	// 验证新配置
	if err := newCfg.Validate(); err != nil {
		log.Fatalf("迁移后配置验证失败: %v", err)
	}

	log.Println("✅ 迁移后配置验证通过！")
}
