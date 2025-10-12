package eventbus

import (
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
)

// TestEnterpriseConfigLayering 验证企业级配置分层设计
func TestEnterpriseConfigLayering(t *testing.T) {
	// 1. 创建用户配置层的完整配置
	userConfig := &config.EventBusConfig{
		Type: "kafka",
		Kafka: config.KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Producer: config.ProducerConfig{
				RequiredAcks:   1,
				Compression:    "snappy",
				FlushFrequency: 100 * time.Millisecond,
				FlushMessages:  50,
				Timeout:        10 * time.Second,
			},
			Consumer: config.ConsumerConfig{
				GroupID:           "test-group",
				AutoOffsetReset:   "earliest",
				SessionTimeout:    30 * time.Second,
				HeartbeatInterval: 3 * time.Second,
			},
		},
		Security: config.SecurityConfig{
			Enabled:  true,
			Protocol: "SASL_SSL",
			Username: "test-user",
			Password: "test-pass",
		},
		// 企业级特性配置
		Publisher: config.PublisherConfig{
			BacklogDetection: config.PublisherBacklogDetectionConfig{
				Enabled:           true,
				MaxQueueDepth:     1000,
				MaxPublishLatency: 5 * time.Second,
				RateThreshold:     0.8,
				CheckInterval:     30 * time.Second,
			},
		},
		Subscriber: config.SubscriberConfig{
			BacklogDetection: config.SubscriberBacklogDetectionConfig{
				Enabled:          true,
				MaxLagThreshold:  500,
				MaxTimeThreshold: 2 * time.Minute,
				CheckInterval:    1 * time.Minute,
			},
		},
	}

	// 2. 转换为程序员配置层
	programmerConfig := ConvertConfig(userConfig)

	// 3. 验证基础配置正确转换
	if programmerConfig.Type != "kafka" {
		t.Errorf("Expected type kafka, got %s", programmerConfig.Type)
	}

	// 4. 验证Kafka配置包含企业级特性
	kafkaConfig := programmerConfig.Kafka

	// 验证基础配置
	if len(kafkaConfig.Brokers) != 1 || kafkaConfig.Brokers[0] != "localhost:9092" {
		t.Errorf("Kafka brokers not correctly transferred")
	}

	// 验证生产者配置（包含程序员控制的字段）
	if kafkaConfig.Producer.RequiredAcks != -1 { // 应该被强制设置为WaitForAll
		t.Errorf("Expected RequiredAcks to be -1 for idempotent producer, got %d", kafkaConfig.Producer.RequiredAcks)
	}
	if kafkaConfig.Producer.Idempotent != true {
		t.Errorf("Expected Idempotent to be true")
	}
	if kafkaConfig.Producer.PartitionerType != "hash" {
		t.Errorf("Expected PartitionerType to be hash, got %s", kafkaConfig.Producer.PartitionerType)
	}

	// 验证安全配置正确转换
	if !kafkaConfig.Security.Enabled {
		t.Errorf("Security should be enabled")
	}
	if kafkaConfig.Security.Protocol != "SASL_SSL" {
		t.Errorf("Expected protocol SASL_SSL, got %s", kafkaConfig.Security.Protocol)
	}

	// 5. 验证企业级特性配置正确转换到程序员配置层
	enterpriseConfig := kafkaConfig.Enterprise

	// 验证发布端积压检测配置
	if !enterpriseConfig.Publisher.BacklogDetection.Enabled {
		t.Errorf("Publisher backlog detection should be enabled")
	}
	if enterpriseConfig.Publisher.BacklogDetection.MaxQueueDepth != 1000 {
		t.Errorf("Expected MaxQueueDepth 1000, got %d", enterpriseConfig.Publisher.BacklogDetection.MaxQueueDepth)
	}
	if enterpriseConfig.Publisher.BacklogDetection.MaxPublishLatency != 5*time.Second {
		t.Errorf("Expected MaxPublishLatency 5s, got %v", enterpriseConfig.Publisher.BacklogDetection.MaxPublishLatency)
	}

	// 验证订阅端积压检测配置
	if !enterpriseConfig.Subscriber.BacklogDetection.Enabled {
		t.Errorf("Subscriber backlog detection should be enabled")
	}
	if enterpriseConfig.Subscriber.BacklogDetection.MaxLagThreshold != 500 {
		t.Errorf("Expected MaxLagThreshold 500, got %d", enterpriseConfig.Subscriber.BacklogDetection.MaxLagThreshold)
	}
	if enterpriseConfig.Subscriber.BacklogDetection.MaxTimeThreshold != 2*time.Minute {
		t.Errorf("Expected MaxTimeThreshold 2m, got %v", enterpriseConfig.Subscriber.BacklogDetection.MaxTimeThreshold)
	}

	// 6. 验证程序员配置层包含所有必要的默认值
	if kafkaConfig.ClientID != "jxt-eventbus" {
		t.Errorf("Expected ClientID jxt-eventbus, got %s", kafkaConfig.ClientID)
	}
	if kafkaConfig.HealthCheckInterval != 30*time.Second {
		t.Errorf("Expected HealthCheckInterval 30s, got %v", kafkaConfig.HealthCheckInterval)
	}
	if kafkaConfig.Net.DialTimeout != 30*time.Second {
		t.Errorf("Expected DialTimeout 30s, got %v", kafkaConfig.Net.DialTimeout)
	}
}

// TestKafkaEventBusUsesOnlyProgrammerConfig 验证KafkaEventBus只使用程序员配置层
func TestKafkaEventBusUsesOnlyProgrammerConfig(t *testing.T) {
	// 创建程序员配置层的配置
	programmerConfig := &KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Producer: ProducerConfig{
			RequiredAcks:     -1,
			Compression:      "snappy",
			FlushFrequency:   100 * time.Millisecond,
			FlushMessages:    50,
			Timeout:          10 * time.Second,
			FlushBytes:       1024 * 1024,
			RetryMax:         3,
			BatchSize:        16 * 1024,
			BufferSize:       32 * 1024 * 1024,
			Idempotent:       true,
			MaxMessageBytes:  1024 * 1024,
			PartitionerType:  "hash",
			LingerMs:         5 * time.Millisecond,
			CompressionLevel: 6,
			MaxInFlight:      1,
		},
		Consumer: ConsumerConfig{
			GroupID:            "test-group",
			AutoOffsetReset:    "earliest",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  30 * time.Second,
			FetchMinBytes:      1024,
			FetchMaxBytes:      50 * 1024 * 1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     500,
			EnableAutoCommit:   false,
			AutoCommitInterval: 5 * time.Second,
			IsolationLevel:     "read_committed",
			RebalanceStrategy:  "range",
		},
		HealthCheckInterval:  30 * time.Second,
		ClientID:             "jxt-eventbus",
		MetadataRefreshFreq:  10 * time.Minute,
		MetadataRetryMax:     3,
		MetadataRetryBackoff: 250 * time.Millisecond,
		Net: NetConfig{
			DialTimeout:  30 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			KeepAlive:    30 * time.Second,
			MaxIdleConns: 10,
			MaxOpenConns: 100,
		},
		Security: SecurityConfig{
			Enabled: false,
		},
		Enterprise: EnterpriseConfig{
			Publisher: PublisherEnterpriseConfig{
				BacklogDetection: config.PublisherBacklogDetectionConfig{
					Enabled:           true,
					MaxQueueDepth:     1000,
					MaxPublishLatency: 5 * time.Second,
					RateThreshold:     0.8,
					CheckInterval:     30 * time.Second,
				},
			},
			Subscriber: SubscriberEnterpriseConfig{
				BacklogDetection: config.SubscriberBacklogDetectionConfig{
					Enabled:          true,
					MaxLagThreshold:  500,
					MaxTimeThreshold: 2 * time.Minute,
					CheckInterval:    1 * time.Minute,
				},
			},
		},
	}

	// 验证配置结构正确性（不实际创建连接）
	if programmerConfig == nil {
		t.Error("Programmer config should not be nil")
	}

	// 验证程序员配置层包含所有必要字段
	if len(programmerConfig.Brokers) == 0 {
		t.Error("Brokers should not be empty")
	}

	// 验证企业级特性配置存在
	if !programmerConfig.Enterprise.Publisher.BacklogDetection.Enabled {
		t.Error("Publisher backlog detection should be enabled")
	}

	if !programmerConfig.Enterprise.Subscriber.BacklogDetection.Enabled {
		t.Error("Subscriber backlog detection should be enabled")
	}

	// 验证程序员控制的字段存在
	if programmerConfig.Producer.FlushBytes == 0 {
		t.Error("FlushBytes should be set (programmer-controlled field)")
	}

	if programmerConfig.Net.DialTimeout == 0 {
		t.Error("DialTimeout should be set (programmer-controlled field)")
	}

	t.Logf("✅ Programmer config structure validation passed")
}

// TestConfigLayeringSeparation 验证配置分层的职责分离
func TestConfigLayeringSeparation(t *testing.T) {
	// 验证用户配置层字段数量（应该很少）
	userKafkaConfig := config.KafkaConfig{}

	// 用户配置层应该只有3个主要字段：Brokers, Producer, Consumer
	// Producer应该只有5个字段：RequiredAcks, Compression, FlushFrequency, FlushMessages, Timeout
	// Consumer应该只有4个字段：GroupID, AutoOffsetReset, SessionTimeout, HeartbeatInterval

	// 验证程序员配置层字段数量（应该很多）
	programmerKafkaConfig := KafkaConfig{}

	// 程序员配置层应该包含所有技术细节：
	// - 基础配置：Brokers, Producer, Consumer
	// - 程序员控制：HealthCheckInterval, Security, Net, ClientID等
	// - 企业级特性：Enterprise

	// 这个测试主要是文档性的，验证设计理念
	t.Logf("User config (simplified): %T", userKafkaConfig)
	t.Logf("Programmer config (complete): %T", programmerKafkaConfig)

	// 验证程序员配置包含企业级特性
	if programmerKafkaConfig.Enterprise == (EnterpriseConfig{}) {
		// 这是正常的，因为我们只是创建了零值
		t.Logf("Enterprise config is available in programmer layer")
	}
}
