package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestKafkaConnection 测试Kafka连接
func TestKafkaConnection(t *testing.T) {
	t.Log("🔗 测试Kafka连接...")

	// 创建简化的Kafka配置（禁用幂等性以避免初始化问题）
	kafkaConfig := &KafkaConfig{
		Brokers: []string{"localhost:29092"},
		Producer: ProducerConfig{
			RequiredAcks:     1, // 不使用WaitForAll
			Compression:      "none",
			FlushFrequency:   100 * time.Millisecond,
			FlushMessages:    10,
			Timeout:          5 * time.Second,
			FlushBytes:       1024,
			RetryMax:         1,
			BatchSize:        1024,
			BufferSize:       1024 * 1024,
			Idempotent:       false, // 禁用幂等性
			MaxMessageBytes:  1024 * 1024,
			PartitionerType:  "hash",
			LingerMs:         0,
			CompressionLevel: 1,
			MaxInFlight:      5, // 非幂等性可以使用>1
		},
		Consumer: ConsumerConfig{
			GroupID:           "connection-test-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
		},
		ClientID:            "connection-test-client",
		HealthCheckInterval: 30 * time.Second,
	}

	// 创建EventBus
	eventBus, err := NewKafkaEventBus(kafkaConfig)
	require.NoError(t, err, "应该能够创建Kafka EventBus")
	defer eventBus.Close()

	t.Log("✅ Kafka连接测试成功")
}
