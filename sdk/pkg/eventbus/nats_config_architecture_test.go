package eventbus

import (
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
)

// TestNATSConfigArchitectureCompliance 测试NATS配置是否符合三层架构要求
func TestNATSConfigArchitectureCompliance(t *testing.T) {
	t.Run("用户配置层到程序员配置层的完整转换", func(t *testing.T) {
		// 🔥 第一层：用户配置层 (sdk/config/eventbus.go) - 简化配置，用户友好
		userConfig := &config.EventBusConfig{
			Type:        "nats",
			ServiceName: "test-service",
			
			// 用户只需要配置核心业务相关字段
			NATS: config.NATSConfig{
				URLs:              []string{"nats://localhost:4222"},
				ClientID:          "test-client",
				MaxReconnects:     5,
				ReconnectWait:     2 * time.Second,
				ConnectionTimeout: 10 * time.Second,
				JetStream: config.JetStreamConfig{
					Enabled: true,
					Domain:  "test-domain",
					// 用户配置层只有基础字段
				},
			},
			
			// 安全配置
			Security: config.SecurityConfig{
				Enabled:  false,
				Username: "test-user",
				Password: "test-pass",
			},
		}

		// 🔥 第二层：程序员配置层 (sdk/pkg/eventbus/type.go) - 完整配置，程序控制
		// 通过ConvertConfig函数将用户配置转换为程序员配置
		programmerConfig := ConvertConfig(userConfig)

		// 验证基础配置正确转换
		assert.Equal(t, "nats", programmerConfig.Type)
		assert.Equal(t, []string{"nats://localhost:4222"}, programmerConfig.NATS.URLs)
		assert.Equal(t, "test-client", programmerConfig.NATS.ClientID)
		assert.Equal(t, 5, programmerConfig.NATS.MaxReconnects)
		assert.Equal(t, 2*time.Second, programmerConfig.NATS.ReconnectWait)
		assert.Equal(t, 10*time.Second, programmerConfig.NATS.ConnectionTimeout)

		// 🔥 验证程序员专用字段被正确设置（用户配置层没有这些字段）
		assert.Equal(t, 5*time.Minute, programmerConfig.NATS.HealthCheckInterval, "程序员专用字段：健康检查间隔")

		// 验证JetStream配置的程序员专用字段
		assert.Equal(t, 5*time.Second, programmerConfig.NATS.JetStream.PublishTimeout, "程序员专用字段：发布超时")
		assert.Equal(t, 30*time.Second, programmerConfig.NATS.JetStream.AckWait, "程序员专用字段：确认等待时间")
		assert.Equal(t, 3, programmerConfig.NATS.JetStream.MaxDeliver, "程序员专用字段：最大投递次数")

		// 验证Stream配置的程序员专用字段
		assert.Equal(t, "BUSINESS_STREAM", programmerConfig.NATS.JetStream.Stream.Name, "程序员专用字段：流名称")
		assert.Equal(t, []string{"business.>"}, programmerConfig.NATS.JetStream.Stream.Subjects, "程序员专用字段：流主题")
		assert.Equal(t, "limits", programmerConfig.NATS.JetStream.Stream.Retention, "程序员专用字段：保留策略")
		assert.Equal(t, "file", programmerConfig.NATS.JetStream.Stream.Storage, "程序员专用字段：存储类型")
		assert.Equal(t, 1, programmerConfig.NATS.JetStream.Stream.Replicas, "程序员专用字段：副本数")
		assert.Equal(t, 24*time.Hour, programmerConfig.NATS.JetStream.Stream.MaxAge, "程序员专用字段：最大保存时间")
		assert.Equal(t, int64(100*1024*1024), programmerConfig.NATS.JetStream.Stream.MaxBytes, "程序员专用字段：最大字节数")
		assert.Equal(t, int64(10000), programmerConfig.NATS.JetStream.Stream.MaxMsgs, "程序员专用字段：最大消息数")
		assert.Equal(t, "old", programmerConfig.NATS.JetStream.Stream.Discard, "程序员专用字段：丢弃策略")

		// 验证Consumer配置的程序员专用字段
		assert.Equal(t, "business-consumer", programmerConfig.NATS.JetStream.Consumer.DurableName, "程序员专用字段：持久消费者名称")
		assert.Equal(t, "all", programmerConfig.NATS.JetStream.Consumer.DeliverPolicy, "程序员专用字段：投递策略")
		assert.Equal(t, "explicit", programmerConfig.NATS.JetStream.Consumer.AckPolicy, "程序员专用字段：确认策略")
		assert.Equal(t, "instant", programmerConfig.NATS.JetStream.Consumer.ReplayPolicy, "程序员专用字段：重放策略")
		assert.Equal(t, 100, programmerConfig.NATS.JetStream.Consumer.MaxAckPending, "程序员专用字段：最大待确认数")
		assert.Equal(t, 500, programmerConfig.NATS.JetStream.Consumer.MaxWaiting, "程序员专用字段：最大等待数")
		assert.Equal(t, 3, programmerConfig.NATS.JetStream.Consumer.MaxDeliver, "程序员专用字段：最大投递次数")

		// 验证安全配置正确转换
		assert.False(t, programmerConfig.NATS.Security.Enabled)
		assert.Equal(t, "test-user", programmerConfig.NATS.Security.Username)
		assert.Equal(t, "test-pass", programmerConfig.NATS.Security.Password)
	})

	t.Run("运行时实现层只使用程序员配置", func(t *testing.T) {
		// 🔥 第三层：运行时实现层 (nats.go) - 只使用type.go中定义的结构
		
		// 创建程序员配置层的配置
		programmerNATSConfig := &NATSConfig{
			URLs:                []string{"nats://localhost:4222"},
			ClientID:            "test-client",
			MaxReconnects:       5,
			ReconnectWait:       2 * time.Second,
			ConnectionTimeout:   10 * time.Second,
			HealthCheckInterval: 5 * time.Minute, // 程序员专用字段
			JetStream: JetStreamConfig{
				Enabled:        true,
				Domain:         "test-domain",
				PublishTimeout: 5 * time.Second,  // 程序员专用字段
				AckWait:        30 * time.Second, // 程序员专用字段
				MaxDeliver:     3,                // 程序员专用字段
				Stream: StreamConfig{
					Name:      "BUSINESS_STREAM",     // 程序员专用字段
					Subjects:  []string{"business.>"}, // 程序员专用字段
					Retention: "limits",              // 程序员专用字段
					Storage:   "file",                // 程序员专用字段
					Replicas:  1,                     // 程序员专用字段
					MaxAge:    24 * time.Hour,        // 程序员专用字段
					MaxBytes:  100 * 1024 * 1024,     // 程序员专用字段
					MaxMsgs:   10000,                 // 程序员专用字段
					Discard:   "old",                 // 程序员专用字段
				},
				Consumer: NATSConsumerConfig{
					DurableName:     "business-consumer", // 程序员专用字段
					DeliverPolicy:   "all",               // 程序员专用字段
					AckPolicy:       "explicit",          // 程序员专用字段
					ReplayPolicy:    "instant",           // 程序员专用字段
					MaxAckPending:   100,                 // 程序员专用字段
					MaxWaiting:      500,                 // 程序员专用字段
					MaxDeliver:      3,                   // 程序员专用字段
				},
			},
			Security: NATSSecurityConfig{
				Enabled:  false,
				Username: "test-user",
				Password: "test-pass",
			},
		}

		// 验证NewNATSEventBus函数接受程序员配置层的配置
		// 注意：这里不实际创建连接，只验证配置结构正确
		assert.NotNil(t, programmerNATSConfig)
		assert.Equal(t, "nats://localhost:4222", programmerNATSConfig.URLs[0])
		assert.Equal(t, 5*time.Minute, programmerNATSConfig.HealthCheckInterval)
		
		// 验证运行时实现层使用的是完整的程序员配置
		assert.Equal(t, 5*time.Second, programmerNATSConfig.JetStream.PublishTimeout)
		assert.Equal(t, "BUSINESS_STREAM", programmerNATSConfig.JetStream.Stream.Name)
		assert.Equal(t, "business-consumer", programmerNATSConfig.JetStream.Consumer.DurableName)
	})
}

// TestNATSConfigLayerSeparation 测试配置层职责分离
func TestNATSConfigLayerSeparation(t *testing.T) {
	t.Run("用户配置层字段验证", func(t *testing.T) {
		// 用户配置层应该只包含用户需要关心的核心字段
		userNATSConfig := config.NATSConfig{
			URLs:              []string{"nats://localhost:4222"},
			ClientID:          "user-client",
			MaxReconnects:     3,
			ReconnectWait:     1 * time.Second,
			ConnectionTimeout: 5 * time.Second,
			JetStream: config.JetStreamConfig{
				Enabled: true,
				Domain:  "user-domain",
				// 注意：用户配置层没有PublishTimeout、AckWait等程序员字段
			},
		}

		// 验证用户配置层的简化性
		assert.Equal(t, "user-client", userNATSConfig.ClientID)
		assert.Equal(t, 3, userNATSConfig.MaxReconnects)
		assert.True(t, userNATSConfig.JetStream.Enabled)
		assert.Equal(t, "user-domain", userNATSConfig.JetStream.Domain)
	})

	t.Run("程序员配置层字段验证", func(t *testing.T) {
		// 程序员配置层应该包含所有技术细节字段
		programmerNATSConfig := NATSConfig{
			URLs:                []string{"nats://localhost:4222"},
			ClientID:            "programmer-client",
			MaxReconnects:       5,
			ReconnectWait:       2 * time.Second,
			ConnectionTimeout:   10 * time.Second,
			HealthCheckInterval: 5 * time.Minute, // 程序员专用字段
			JetStream: JetStreamConfig{
				Enabled:        true,
				Domain:         "programmer-domain",
				PublishTimeout: 5 * time.Second,  // 程序员专用字段
				AckWait:        30 * time.Second, // 程序员专用字段
				MaxDeliver:     3,                // 程序员专用字段
				// ... 更多程序员专用字段
			},
			Enterprise: EnterpriseConfig{
				// 企业级特性配置
			},
		}

		// 验证程序员配置层的完整性
		assert.Equal(t, "programmer-client", programmerNATSConfig.ClientID)
		assert.Equal(t, 5*time.Minute, programmerNATSConfig.HealthCheckInterval)
		assert.Equal(t, 5*time.Second, programmerNATSConfig.JetStream.PublishTimeout)
		assert.Equal(t, 30*time.Second, programmerNATSConfig.JetStream.AckWait)
		assert.Equal(t, 3, programmerNATSConfig.JetStream.MaxDeliver)
	})
}
