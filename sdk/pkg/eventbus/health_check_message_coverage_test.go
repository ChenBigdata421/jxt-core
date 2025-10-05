package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthCheckMessage_ToBytes_Success 测试转换为字节（成功）
func TestHealthCheckMessage_ToBytes_Success(t *testing.T) {
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	bytes, err := msg.ToBytes()
	assert.NoError(t, err)
	assert.NotNil(t, bytes)
	assert.Greater(t, len(bytes), 0)
}

// TestHealthCheckMessage_ToBytes_EmptyMessage 测试转换为字节（空消息）
func TestHealthCheckMessage_ToBytes_EmptyMessage(t *testing.T) {
	msg := &HealthCheckMessage{}

	bytes, err := msg.ToBytes()
	// 可能返回错误或空字节
	if err != nil {
		assert.Error(t, err)
	} else {
		assert.NotNil(t, bytes)
	}
}

// TestHealthCheckMessageParser_ParseWithoutValidation_Success 测试解析不验证（成功）
func TestHealthCheckMessageParser_ParseWithoutValidation_Success(t *testing.T) {
	// 创建一个有效的消息
	original := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
	}

	// 转换为字节
	bytes, err := original.ToBytes()
	require.NoError(t, err)

	// 创建解析器
	parser := NewHealthCheckMessageParser()

	// 解析不验证
	parsed, err := parser.ParseWithoutValidation(bytes)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, original.MessageID, parsed.MessageID)
	assert.Equal(t, original.Source, parsed.Source)
	assert.Equal(t, original.EventBusType, parsed.EventBusType)
}

// TestHealthCheckMessageParser_ParseWithoutValidation_InvalidJSON 测试解析不验证（无效 JSON）
func TestHealthCheckMessageParser_ParseWithoutValidation_InvalidJSON(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	invalidJSON := []byte("invalid json")

	parsed, err := parser.ParseWithoutValidation(invalidJSON)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessageParser_ParseWithoutValidation_EmptyBytes 测试解析不验证（空字节）
func TestHealthCheckMessageParser_ParseWithoutValidation_EmptyBytes(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	emptyBytes := []byte("")

	parsed, err := parser.ParseWithoutValidation(emptyBytes)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessageParser_ParseWithoutValidation_NilBytes 测试解析不验证（nil 字节）
func TestHealthCheckMessageParser_ParseWithoutValidation_NilBytes(t *testing.T) {
	parser := NewHealthCheckMessageParser()
	var nilBytes []byte

	parsed, err := parser.ParseWithoutValidation(nilBytes)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessageValidator_Validate_AllFields 测试验证（所有字段）
func TestHealthCheckMessageValidator_Validate_AllFields(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	err := validator.Validate(msg)
	assert.NoError(t, err)
}

// TestHealthCheckMessageValidator_Validate_MissingMessageID 测试验证（缺少 MessageID）
func TestHealthCheckMessageValidator_Validate_MissingMessageID(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	msg := &HealthCheckMessage{
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
	}

	err := validator.Validate(msg)
	assert.Error(t, err)
}

// TestHealthCheckMessageValidator_Validate_MissingSource 测试验证（缺少 Source）
func TestHealthCheckMessageValidator_Validate_MissingSource(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		EventBusType: "memory",
		Version:      "1.0",
	}

	err := validator.Validate(msg)
	assert.Error(t, err)
}

// TestHealthCheckMessageValidator_Validate_ZeroTimestamp 测试验证（零时间戳）
func TestHealthCheckMessageValidator_Validate_ZeroTimestamp(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Time{},
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
	}

	err := validator.Validate(msg)
	assert.Error(t, err)
}

// TestHealthCheckMessageParser_Parse_Success 测试解析（成功）
func TestHealthCheckMessageParser_Parse_Success(t *testing.T) {
	// 创建一个有效的消息
	original := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
	}

	// 转换为字节
	bytes, err := original.ToBytes()
	require.NoError(t, err)

	// 创建解析器
	parser := NewHealthCheckMessageParser()

	// 解析并验证
	parsed, err := parser.Parse(bytes)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, original.MessageID, parsed.MessageID)
	assert.Equal(t, original.Source, parsed.Source)
	assert.Equal(t, original.EventBusType, parsed.EventBusType)
}

// TestHealthCheckMessageParser_Parse_InvalidMessage 测试解析（无效消息）
func TestHealthCheckMessageParser_Parse_InvalidMessage(t *testing.T) {
	parser := NewHealthCheckMessageParser()

	// 创建一个无效的消息（缺少必填字段）
	invalidMsg := &HealthCheckMessage{
		MessageID: "test-id",
		// 缺少其他必填字段
	}

	// 转换为字节
	bytes, err := invalidMsg.ToBytes()
	require.NoError(t, err)

	// 解析应该失败（验证失败）
	parsed, err := parser.Parse(bytes)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessage_RoundTrip 测试往返转换
func TestHealthCheckMessage_RoundTrip(t *testing.T) {
	original := &HealthCheckMessage{
		MessageID:    "test-id-123",
		Timestamp:    time.Now().Truncate(time.Second), // 截断以避免精度问题
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
		Metadata: map[string]string{
			"status": "healthy",
			"count":  "42",
		},
	}

	// 转换为字节
	bytes, err := original.ToBytes()
	require.NoError(t, err)

	// 创建解析器
	parser := NewHealthCheckMessageParser()

	// 解析回来
	parsed, err := parser.Parse(bytes)
	require.NoError(t, err)

	// 验证字段
	assert.Equal(t, original.MessageID, parsed.MessageID)
	assert.Equal(t, original.Source, parsed.Source)
	assert.Equal(t, original.EventBusType, parsed.EventBusType)
	assert.Equal(t, original.Version, parsed.Version)
	assert.NotNil(t, parsed.Metadata)
}

// TestHealthCheckMessage_DifferentEventBusTypes 测试不同的 EventBus 类型
func TestHealthCheckMessage_DifferentEventBusTypes(t *testing.T) {
	validator := NewHealthCheckMessageValidator()
	eventBusTypes := []string{"memory", "kafka", "nats"}

	for _, busType := range eventBusTypes {
		msg := &HealthCheckMessage{
			MessageID:    "test-id",
			Timestamp:    time.Now(),
			Source:       "test-source",
			EventBusType: busType,
			Version:      "1.0",
		}

		err := validator.Validate(msg)
		assert.NoError(t, err, "EventBus type %s should be valid", busType)

		bytes, err := msg.ToBytes()
		assert.NoError(t, err, "EventBus type %s should convert to bytes", busType)
		assert.NotNil(t, bytes)
	}
}

// TestHealthCheckMessage_WithMetadata 测试带 Metadata
func TestHealthCheckMessage_WithMetadata(t *testing.T) {
	msg := &HealthCheckMessage{
		MessageID:    "test-id",
		Timestamp:    time.Now(),
		Source:       "test-source",
		EventBusType: "memory",
		Version:      "1.0",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		},
	}

	// 验证
	validator := NewHealthCheckMessageValidator()
	err := validator.Validate(msg)
	assert.NoError(t, err)

	// 转换为字节
	bytes, err := msg.ToBytes()
	assert.NoError(t, err)
	assert.NotNil(t, bytes)

	// 创建解析器
	parser := NewHealthCheckMessageParser()

	// 解析回来
	parsed, err := parser.Parse(bytes)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.NotNil(t, parsed.Metadata)
	assert.Equal(t, len(msg.Metadata), len(parsed.Metadata))
}

// TestCreateHealthCheckMessage 测试创建健康检查消息
func TestCreateHealthCheckMessage(t *testing.T) {
	msg := CreateHealthCheckMessage("test-source", "memory")

	assert.NotNil(t, msg)
	assert.NotEmpty(t, msg.MessageID)
	assert.Equal(t, "test-source", msg.Source)
	assert.Equal(t, "memory", msg.EventBusType)
	assert.False(t, msg.Timestamp.IsZero())

	// 验证消息
	validator := NewHealthCheckMessageValidator()
	err := validator.Validate(msg)
	assert.NoError(t, err)
}
