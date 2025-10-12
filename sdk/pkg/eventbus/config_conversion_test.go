package eventbus

import (
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
)

func TestConvertUserConfigToInternalKafkaConfig(t *testing.T) {
	// 创建用户配置层的配置
	userConfig := &config.KafkaConfig{
		Brokers: []string{"localhost:9092", "localhost:9093"},
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
	}

	// 转换为程序员配置层
	internalConfig := convertUserConfigToInternalKafkaConfig(userConfig)

	// 验证基础配置
	if len(internalConfig.Brokers) != 2 {
		t.Errorf("Expected 2 brokers, got %d", len(internalConfig.Brokers))
	}
	if internalConfig.Brokers[0] != "localhost:9092" {
		t.Errorf("Expected first broker to be localhost:9092, got %s", internalConfig.Brokers[0])
	}

	// 验证生产者配置 - 用户字段（注意：RequiredAcks被强制设置为-1以支持幂等性）
	if internalConfig.Producer.RequiredAcks != -1 {
		t.Errorf("Expected RequiredAcks to be -1 (WaitForAll for idempotent producer), got %d", internalConfig.Producer.RequiredAcks)
	}
	if internalConfig.Producer.Compression != "snappy" {
		t.Errorf("Expected Compression to be snappy, got %s", internalConfig.Producer.Compression)
	}
	if internalConfig.Producer.FlushFrequency != 100*time.Millisecond {
		t.Errorf("Expected FlushFrequency to be 100ms, got %v", internalConfig.Producer.FlushFrequency)
	}

	// 验证生产者配置 - 程序员设定的默认值
	if internalConfig.Producer.FlushBytes != 1024*1024 {
		t.Errorf("Expected FlushBytes to be 1MB, got %d", internalConfig.Producer.FlushBytes)
	}
	if internalConfig.Producer.RetryMax != 3 {
		t.Errorf("Expected RetryMax to be 3, got %d", internalConfig.Producer.RetryMax)
	}
	if internalConfig.Producer.Idempotent != true {
		t.Errorf("Expected Idempotent to be true, got %v", internalConfig.Producer.Idempotent)
	}
	if internalConfig.Producer.PartitionerType != "hash" {
		t.Errorf("Expected PartitionerType to be hash, got %s", internalConfig.Producer.PartitionerType)
	}

	// 验证消费者配置 - 用户字段
	if internalConfig.Consumer.GroupID != "test-group" {
		t.Errorf("Expected GroupID to be test-group, got %s", internalConfig.Consumer.GroupID)
	}
	if internalConfig.Consumer.AutoOffsetReset != "earliest" {
		t.Errorf("Expected AutoOffsetReset to be earliest, got %s", internalConfig.Consumer.AutoOffsetReset)
	}

	// 验证消费者配置 - 程序员设定的默认值
	if internalConfig.Consumer.MaxProcessingTime != 30*time.Second {
		t.Errorf("Expected MaxProcessingTime to be 30s, got %v", internalConfig.Consumer.MaxProcessingTime)
	}
	if internalConfig.Consumer.FetchMinBytes != 1024 {
		t.Errorf("Expected FetchMinBytes to be 1024, got %d", internalConfig.Consumer.FetchMinBytes)
	}
	if internalConfig.Consumer.EnableAutoCommit != false {
		t.Errorf("Expected EnableAutoCommit to be false, got %v", internalConfig.Consumer.EnableAutoCommit)
	}

	// 验证程序员专用配置
	if internalConfig.HealthCheckInterval != 30*time.Second {
		t.Errorf("Expected HealthCheckInterval to be 30s, got %v", internalConfig.HealthCheckInterval)
	}
	if internalConfig.ClientID != "jxt-eventbus" {
		t.Errorf("Expected ClientID to be jxt-eventbus, got %s", internalConfig.ClientID)
	}

	// 验证网络配置
	if internalConfig.Net.DialTimeout != 30*time.Second {
		t.Errorf("Expected DialTimeout to be 30s, got %v", internalConfig.Net.DialTimeout)
	}
	if internalConfig.Net.MaxOpenConns != 100 {
		t.Errorf("Expected MaxOpenConns to be 100, got %d", internalConfig.Net.MaxOpenConns)
	}

	// 验证安全配置默认值
	if internalConfig.Security.Enabled != false {
		t.Errorf("Expected Security.Enabled to be false, got %v", internalConfig.Security.Enabled)
	}
}

func TestNewKafkaEventBusWithInternalConfig(t *testing.T) {
	// 创建一个完整的程序员配置层配置
	internalConfig := &KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Producer: ProducerConfig{
			RequiredAcks:     -1, // WaitForAll for idempotent producer
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
	}

	// 尝试创建Kafka EventBus（这会失败，因为没有真实的Kafka，但至少验证配置结构正确）
	_, err := NewKafkaEventBus(internalConfig)

	// 我们期望这会失败，因为没有真实的Kafka连接，但错误应该是连接相关的，不是配置相关的
	if err == nil {
		t.Error("Expected error when creating Kafka EventBus without real Kafka server")
	}

	// 检查错误是否与配置无关（即不是nil config或empty brokers错误）
	if err.Error() == "kafka config cannot be nil" || err.Error() == "kafka brokers cannot be empty" {
		t.Errorf("Unexpected config-related error: %v", err)
	}

	t.Logf("Got expected connection-related error: %v", err)
}
