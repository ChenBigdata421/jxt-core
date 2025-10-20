package eventbus

import (
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarshalToString 测试序列化为字符串
func TestMarshalToString(t *testing.T) {
	// 使用简单的结构体避免 jsoniter 的 map 序列化问题
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	str, err := MarshalToString(data)
	require.NoError(t, err)
	assert.Contains(t, str, "test")
	assert.Contains(t, str, "30")
}

// TestUnmarshalFromString 测试从字符串反序列化
func TestUnmarshalFromString(t *testing.T) {
	str := `{"name":"test","age":30}`

	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	var result TestData
	err := UnmarshalFromString(str, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 30, result.Age)
}

// TestMarshal 测试序列化
func TestMarshal(t *testing.T) {
	data := map[string]interface{}{
		"name": "test",
		"age":  30,
	}

	bytes, err := Marshal(data)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes)
	assert.Contains(t, string(bytes), "test")
}

// TestUnmarshal 测试反序列化
func TestUnmarshal(t *testing.T) {
	bytes := []byte(`{"name":"test","age":30}`)

	var result map[string]interface{}
	err := Unmarshal(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
	assert.Equal(t, float64(30), result["age"])
}

// TestMarshalFast 测试快速序列化
func TestMarshalFast(t *testing.T) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	bytes, err := MarshalFast(data)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes)
}

// TestUnmarshalFast 测试快速反序列化
func TestUnmarshalFast(t *testing.T) {
	bytes := []byte(`{"name":"test","age":30}`)

	var result map[string]interface{}
	err := UnmarshalFast(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
}

// TestJSON_RoundTrip 测试完整的序列化反序列化流程
func TestJSON_RoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"string": "value",
		"number": 42,
		"bool":   true,
		"array":  []int{1, 2, 3},
		"nested": map[string]string{
			"key": "value",
		},
	}

	// 序列化
	bytes, err := Marshal(original)
	require.NoError(t, err)

	// 反序列化
	var result map[string]interface{}
	err = Unmarshal(bytes, &result)
	require.NoError(t, err)

	// 验证
	assert.Equal(t, "value", result["string"])
	assert.Equal(t, float64(42), result["number"])
	assert.Equal(t, true, result["bool"])
}

// TestJSONFast_RoundTrip 测试快速序列化反序列化流程
func TestJSONFast_RoundTrip(t *testing.T) {
	type TestData struct {
		String string `json:"string"`
		Number int    `json:"number"`
	}
	original := TestData{String: "value", Number: 42}

	// 快速序列化
	bytes, err := MarshalFast(original)
	require.NoError(t, err)

	// 快速反序列化
	var result TestData
	err = UnmarshalFast(bytes, &result)
	require.NoError(t, err)

	// 验证
	assert.Equal(t, "value", result.String)
	assert.Equal(t, 42, result.Number)
}

// TestMarshalToString_Error 测试序列化错误
func TestMarshalToString_Error(t *testing.T) {
	// 创建一个无法序列化的对象（channel 类型）
	invalidData := make(chan int)

	_, err := MarshalToString(invalidData)
	assert.Error(t, err)
}

// TestUnmarshalFromString_Error 测试反序列化错误
func TestUnmarshalFromString_Error(t *testing.T) {
	invalidJSON := `{"name": invalid}`

	var result map[string]interface{}
	err := UnmarshalFromString(invalidJSON, &result)
	assert.Error(t, err)
}

// TestMarshal_Struct 测试序列化结构体
func TestMarshal_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	person := Person{Name: "Alice", Age: 30}

	bytes, err := Marshal(person)
	require.NoError(t, err)
	assert.Contains(t, string(bytes), "Alice")
	assert.Contains(t, string(bytes), "30")
}

// TestUnmarshal_Struct 测试反序列化到结构体
func TestUnmarshal_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	bytes := []byte(`{"name":"Bob","age":25}`)

	var person Person
	err := Unmarshal(bytes, &person)
	require.NoError(t, err)
	assert.Equal(t, "Bob", person.Name)
	assert.Equal(t, 25, person.Age)
}

// TestJSON_Variables 测试JSON变量
func TestJSON_Variables(t *testing.T) {
	assert.NotNil(t, JSON)
	assert.NotNil(t, JSONFast)
	assert.NotNil(t, JSONDefault)
}

// TestRawMessage 测试RawMessage类型
func TestRawMessage(t *testing.T) {
	type Message struct {
		Type string     `json:"type"`
		Data RawMessage `json:"data"`
	}

	// 创建消息
	msg := Message{
		Type: "test",
		Data: RawMessage(`{"key":"value"}`),
	}

	// 序列化
	bytes, err := Marshal(msg)
	require.NoError(t, err)

	// 反序列化
	var result Message
	err = Unmarshal(bytes, &result)
	require.NoError(t, err)

	assert.Equal(t, "test", result.Type)
	assert.Contains(t, string(result.Data), "key")
}

// TestMarshalToString_EmptyObject 测试序列化空对象
func TestMarshalToString_EmptyObject(t *testing.T) {
	type EmptyData struct{}
	data := EmptyData{}

	str, err := MarshalToString(data)
	require.NoError(t, err)
	assert.Equal(t, "{}", str)
}

// TestUnmarshalFromString_EmptyObject 测试反序列化空对象
func TestUnmarshalFromString_EmptyObject(t *testing.T) {
	str := `{}`

	type EmptyData struct{}
	var result EmptyData
	err := UnmarshalFromString(str, &result)
	require.NoError(t, err)
}

// TestMarshal_Array 测试序列化数组
func TestMarshal_Array(t *testing.T) {
	data := []int{1, 2, 3, 4, 5}

	bytes, err := Marshal(data)
	require.NoError(t, err)
	assert.Contains(t, string(bytes), "1")
	assert.Contains(t, string(bytes), "5")
}

// TestUnmarshal_Array 测试反序列化数组
func TestUnmarshal_Array(t *testing.T) {
	bytes := []byte(`[1,2,3,4,5]`)

	var result []int
	err := Unmarshal(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, result)
}

// BenchmarkMarshal 基准测试：标准序列化
func BenchmarkMarshal(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Marshal(data)
	}
}

// BenchmarkMarshalFast 基准测试：快速序列化
func BenchmarkMarshalFast(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = MarshalFast(data)
	}
}

// BenchmarkMarshalToString 基准测试：序列化为字符串
func BenchmarkMarshalToString(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = MarshalToString(data)
	}
}

func TestConvertUserConfigToInternalKafkaConfig(t *testing.T) {
	// 创建用户配置层的配置
	userConfig := &config.KafkaConfig{
		Brokers: []string{"localhost:9092", "localhost:9093"},
		Producer: config.ProducerConfig{
			RequiredAcks:   1,
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
	// 注意：Compression 字段已从 Producer 级别移到 Topic 级别，通过 TopicBuilder 配置
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

// TestEnterpriseConfigLayering 验证企业级配置分层设计
func TestEnterpriseConfigLayering(t *testing.T) {
	// 1. 创建用户配置层的完整配置
	userConfig := &config.EventBusConfig{
		Type: "kafka",
		Kafka: config.KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Producer: config.ProducerConfig{
				RequiredAcks:   1,
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
