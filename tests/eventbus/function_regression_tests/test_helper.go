package function_tests

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/IBM/sarama"
	"github.com/nats-io/nats.go"
)

// TestHelper 测试辅助工具
type TestHelper struct {
	t              *testing.T
	kafkaAdmin     sarama.ClusterAdmin
	natsConn       *nats.Conn
	createdTopics  []string
	createdStreams []string
}

// NewTestHelper 创建测试辅助工具
func NewTestHelper(t *testing.T) *TestHelper {
	// 初始化 logger
	logger.Setup()

	return &TestHelper{
		t:              t,
		createdTopics:  make([]string, 0),
		createdStreams: make([]string, 0),
	}
}

// CreateMemoryEventBus 创建 Memory EventBus 用于测试
func (h *TestHelper) CreateMemoryEventBus() eventbus.EventBus {
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}

	bus, err := eventbus.NewEventBus(cfg)
	h.AssertNoError(err, "Failed to create Memory EventBus")

	return bus
}

// CreateKafkaEventBus 创建 Kafka EventBus 用于测试
func (h *TestHelper) CreateKafkaEventBus(clientID string) eventbus.EventBus {
	cfg := &eventbus.KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: eventbus.ProducerConfig{
			RequiredAcks:    1,
			FlushFrequency:  10 * time.Millisecond,
			FlushMessages:   100,
			Timeout:         5 * time.Second,
			FlushBytes:      1048576,
			RetryMax:        3,
			BatchSize:       16384,
			BufferSize:      8388608,
			MaxMessageBytes: 1048576,
		},
		Consumer: eventbus.ConsumerConfig{
			GroupID:            fmt.Sprintf("test-group-%s", clientID),
			SessionTimeout:     10 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  5 * time.Second,
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			FetchMaxWait:       100 * time.Millisecond,
			MaxPollRecords:     500,
		},
		Net: eventbus.NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		ClientID: clientID,
	}

	bus, err := eventbus.NewKafkaEventBus(cfg)
	if err != nil {
		h.t.Fatalf("Failed to create Kafka EventBus: %v", err)
	}

	return bus
}

// CreateKafkaEventBusWithHealthCheck 创建带自定义健康检查配置的 Kafka EventBus 用于测试
func (h *TestHelper) CreateKafkaEventBusWithHealthCheck(groupID string, healthCheckConfig config.HealthCheckConfig) eventbus.EventBus {
	cfg := &eventbus.KafkaConfig{
		Brokers: []string{"localhost:29094"},
		Producer: eventbus.ProducerConfig{
			RequiredAcks:    1,
			FlushFrequency:  10 * time.Millisecond,
			FlushMessages:   100,
			Timeout:         5 * time.Second,
			FlushBytes:      1048576,
			RetryMax:        3,
			BatchSize:       16384,
			BufferSize:      8388608,
			MaxMessageBytes: 1048576,
		},
		Consumer: eventbus.ConsumerConfig{
			GroupID:            groupID,
			SessionTimeout:     10 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  5 * time.Second,
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			FetchMaxWait:       100 * time.Millisecond,
			MaxPollRecords:     500,
		},
		Net: eventbus.NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		ClientID: groupID,
		Enterprise: eventbus.EnterpriseConfig{
			HealthCheck: eventbus.HealthCheckConfig{
				Enabled:          healthCheckConfig.Enabled,
				Topic:            healthCheckConfig.Publisher.Topic,
				Interval:         healthCheckConfig.Publisher.Interval,
				Timeout:          healthCheckConfig.Publisher.Timeout,
				FailureThreshold: healthCheckConfig.Publisher.FailureThreshold,
				MessageTTL:       healthCheckConfig.Publisher.MessageTTL,
			},
		},
	}

	bus, err := eventbus.NewKafkaEventBus(cfg)
	if err != nil {
		h.t.Fatalf("Failed to create Kafka EventBus: %v", err)
	}

	return bus
}

// CreateNATSEventBus 创建 NATS EventBus 用于测试
// 注意：启用 JetStream 以支持持久化功能测试
func (h *TestHelper) CreateNATSEventBus(clientID string) eventbus.EventBus {
	// 使用 clientID 生成唯一的 stream 名称，避免多个测试之间的冲突
	streamName := fmt.Sprintf("TEST_STREAM_%s", clientID)

	// 🔧 修复：为每个测试使用唯一的 subject 前缀，避免 Stream subjects 冲突
	// 使用 clientID 作为前缀，确保每个测试的 subjects 不重叠
	// 例如：nats-envelope-1760865444338.> 只匹配该测试的主题
	subjectPrefix := fmt.Sprintf("%s.>", clientID)

	cfg := &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: clientID,
		// 启用 JetStream 以支持 ConfigureTopic 等持久化功能
		JetStream: eventbus.JetStreamConfig{
			Enabled: true,
			Stream: eventbus.StreamConfig{
				Name:     streamName,
				Subjects: []string{subjectPrefix},
			},
		},
	}

	bus, err := eventbus.NewNATSEventBus(cfg)
	if err != nil {
		h.t.Fatalf("Failed to create NATS EventBus: %v", err)
	}

	return bus
}

// CreateNATSEventBusWithHealthCheck 创建带自定义健康检查配置的 NATS EventBus 用于测试
func (h *TestHelper) CreateNATSEventBusWithHealthCheck(clientID string, healthCheckConfig config.HealthCheckConfig) eventbus.EventBus {
	// 使用 clientID 生成唯一的 stream 名称，避免多个测试之间的冲突
	streamName := fmt.Sprintf("TEST_STREAM_%s", clientID)

	// 🔧 修复：为每个测试使用唯一的 subject 前缀，避免 Stream subjects 冲突
	subjectPrefix := fmt.Sprintf("%s.>", clientID)

	cfg := &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: clientID,
		// 启用 JetStream 以支持健康检查持久化
		JetStream: eventbus.JetStreamConfig{
			Enabled: true,
			Stream: eventbus.StreamConfig{
				Name:     streamName,
				Subjects: []string{subjectPrefix},
			},
		},
		Enterprise: eventbus.EnterpriseConfig{
			HealthCheck: eventbus.HealthCheckConfig{
				Enabled:          healthCheckConfig.Enabled,
				Topic:            healthCheckConfig.Publisher.Topic,
				Interval:         healthCheckConfig.Publisher.Interval,
				Timeout:          healthCheckConfig.Publisher.Timeout,
				FailureThreshold: healthCheckConfig.Publisher.FailureThreshold,
				MessageTTL:       healthCheckConfig.Publisher.MessageTTL,
			},
		},
	}

	bus, err := eventbus.NewNATSEventBus(cfg)
	if err != nil {
		h.t.Fatalf("Failed to create NATS EventBus: %v", err)
	}

	return bus
}

// CleanupKafkaTopics 清理 Kafka topics
func (h *TestHelper) CleanupKafkaTopics(topics []string) {
	if h.kafkaAdmin == nil {
		config := sarama.NewConfig()
		config.Version = sarama.V2_6_0_0
		admin, err := sarama.NewClusterAdmin([]string{"localhost:29094"}, config)
		if err != nil {
			h.t.Logf("⚠️  Failed to create Kafka admin: %v", err)
			return
		}
		h.kafkaAdmin = admin
	}

	for _, topic := range topics {
		err := h.kafkaAdmin.DeleteTopic(topic)
		if err != nil {
			h.t.Logf("⚠️  Failed to delete Kafka topic %s: %v", topic, err)
		} else {
			h.t.Logf("✅ Deleted Kafka topic: %s", topic)
		}
	}

	// 等待删除完成
	time.Sleep(2 * time.Second)
}

// CleanupNATSStreams 清理 NATS streams
func (h *TestHelper) CleanupNATSStreams(streamPrefix string) {
	if h.natsConn == nil {
		nc, err := nats.Connect("nats://localhost:4223")
		if err != nil {
			h.t.Logf("⚠️  Failed to connect to NATS: %v", err)
			return
		}
		h.natsConn = nc
	}

	js, err := h.natsConn.JetStream()
	if err != nil {
		h.t.Logf("⚠️  Failed to get JetStream context: %v", err)
		return
	}

	// 列出所有 streams
	streams := js.StreamNames()
	for stream := range streams {
		// 删除匹配前缀的 stream
		if len(streamPrefix) == 0 || len(stream) >= len(streamPrefix) && stream[:len(streamPrefix)] == streamPrefix {
			err := js.DeleteStream(stream)
			if err != nil {
				h.t.Logf("⚠️  Failed to delete NATS stream %s: %v", stream, err)
			} else {
				h.t.Logf("✅ Deleted NATS stream: %s", stream)
			}
		}
	}

	// 等待删除完成
	time.Sleep(1 * time.Second)
}

// Cleanup 清理所有资源
func (h *TestHelper) Cleanup() {
	if len(h.createdTopics) > 0 {
		h.CleanupKafkaTopics(h.createdTopics)
	}

	if len(h.createdStreams) > 0 {
		for _, stream := range h.createdStreams {
			h.CleanupNATSStreams(stream)
		}
	}

	if h.kafkaAdmin != nil {
		h.kafkaAdmin.Close()
	}

	if h.natsConn != nil {
		h.natsConn.Close()
	}
}

// WaitForMessages 等待接收指定数量的消息
func (h *TestHelper) WaitForMessages(received *int64, expected int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(received) >= expected {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// AssertEqual 断言相等
func (h *TestHelper) AssertEqual(expected, actual interface{}, message string) {
	if expected != actual {
		h.t.Errorf("%s: expected %v, got %v", message, expected, actual)
	}
}

// AssertNoError 断言无错误
func (h *TestHelper) AssertNoError(err error, message string) {
	if err != nil {
		h.t.Errorf("%s: %v", message, err)
	}
}

// AssertTrue 断言为真
func (h *TestHelper) AssertTrue(condition bool, message string) {
	if !condition {
		h.t.Errorf("%s: expected true, got false", message)
	}
}

// AssertNotEmpty 断言非空
func (h *TestHelper) AssertNotEmpty(obj interface{}, message string) {
	if obj == nil {
		h.t.Errorf("%s: expected non-empty, got nil", message)
		return
	}

	// 检查字符串、切片、map 等
	switch v := obj.(type) {
	case string:
		if v == "" {
			h.t.Errorf("%s: expected non-empty string", message)
		}
	case []byte:
		if len(v) == 0 {
			h.t.Errorf("%s: expected non-empty byte slice", message)
		}
	}
}

// AssertNotNil 断言不为 nil
func (h *TestHelper) AssertNotNil(obj interface{}, message string) {
	if obj == nil {
		h.t.Errorf("%s: expected non-nil, got nil", message)
	}
}

// AssertError 断言有错误
func (h *TestHelper) AssertError(err error, message string) {
	if err == nil {
		h.t.Errorf("%s: expected error, got nil", message)
	}
}

// CreateKafkaTopics 创建 Kafka topics
func (h *TestHelper) CreateKafkaTopics(topics []string, numPartitions int32) {
	if h.kafkaAdmin == nil {
		config := sarama.NewConfig()
		config.Version = sarama.V2_6_0_0
		admin, err := sarama.NewClusterAdmin([]string{"localhost:29094"}, config)
		if err != nil {
			h.t.Fatalf("Failed to create Kafka admin: %v", err)
		}
		h.kafkaAdmin = admin
	}

	for _, topic := range topics {
		err := h.kafkaAdmin.CreateTopic(topic, &sarama.TopicDetail{
			NumPartitions:     numPartitions,
			ReplicationFactor: 1,
		}, false)
		if err != nil {
			h.t.Logf("⚠️  Failed to create Kafka topic %s: %v", topic, err)
		} else {
			h.t.Logf("✅ Created Kafka topic: %s", topic)
			h.createdTopics = append(h.createdTopics, topic)
		}
	}

	// 等待创建完成
	time.Sleep(2 * time.Second)
}

// WaitForCondition 等待条件满足
func (h *TestHelper) WaitForCondition(condition func() bool, timeout time.Duration, message string) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	h.t.Logf("⚠️  Timeout waiting for condition: %s", message)
	return false
}

// GetTimestamp 获取当前时间戳（用于生成唯一ID）
func (h *TestHelper) GetTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// CloseEventBus 关闭 EventBus 并等待资源释放
func (h *TestHelper) CloseEventBus(bus eventbus.EventBus) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 停止所有监控
	_ = bus.StopAllBacklogMonitoring()
	_ = bus.StopAllHealthCheck()

	// 关闭 EventBus
	err := bus.Close()
	if err != nil {
		h.t.Logf("⚠️  Failed to close EventBus: %v", err)
	}

	// 等待资源释放
	select {
	case <-ctx.Done():
		h.t.Logf("⚠️  Timeout waiting for EventBus to close")
	case <-time.After(1 * time.Second):
		// 正常关闭
	}
}
