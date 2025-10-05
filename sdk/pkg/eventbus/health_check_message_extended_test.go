package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthCheckMessage_FromBytes 测试从字节反序列化
func TestHealthCheckMessage_FromBytes(t *testing.T) {
	original := CreateHealthCheckMessage("test-source", "memory")
	
	// 序列化
	data, err := original.ToBytes()
	require.NoError(t, err)
	
	// 反序列化
	restored := &HealthCheckMessage{}
	err = restored.FromBytes(data)
	assert.NoError(t, err)
	assert.Equal(t, original.MessageID, restored.MessageID)
	assert.Equal(t, original.Source, restored.Source)
	assert.Equal(t, original.EventBusType, restored.EventBusType)
}

// TestHealthCheckMessage_FromBytes_InvalidData 测试无效数据反序列化
func TestHealthCheckMessage_FromBytes_InvalidData(t *testing.T) {
	msg := &HealthCheckMessage{}
	err := msg.FromBytes([]byte("invalid json"))
	assert.Error(t, err)
}

// TestHealthCheckMessage_IsValid 测试消息有效性验证
func TestHealthCheckMessage_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		msg      *HealthCheckMessage
		expected bool
	}{
		{
			name: "valid message",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expected: true,
		},
		{
			name: "missing message id",
			msg: &HealthCheckMessage{
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expected: false,
		},
		{
			name: "missing source",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expected: false,
		},
		{
			name: "missing event bus type",
			msg: &HealthCheckMessage{
				MessageID: "test-id",
				Source:    "test-source",
				Timestamp: time.Now(),
			},
			expected: false,
		},
		{
			name: "timestamp too far in future",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now().Add(2 * time.Minute),
			},
			expected: false,
		},
		{
			name: "timestamp too far in past",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now().Add(-10 * time.Minute),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHealthCheckMessage_SetMetadata_NilMap 测试在 nil map 上设置元数据
func TestHealthCheckMessage_SetMetadata_NilMap(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: nil,
	}

	msg.SetMetadata("key1", "value1")
	assert.NotNil(t, msg.Metadata)
	assert.Equal(t, "value1", msg.Metadata["key1"])
}

// TestHealthCheckMessage_SetMetadata_ExistingMap 测试在已有 map 上设置元数据
func TestHealthCheckMessage_SetMetadata_ExistingMap(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: map[string]string{
			"existing": "value",
		},
	}

	msg.SetMetadata("key1", "value1")
	assert.Equal(t, "value1", msg.Metadata["key1"])
	assert.Equal(t, "value", msg.Metadata["existing"])
}

// TestHealthCheckMessage_GetMetadata_NilMap 测试从 nil map 获取元数据
func TestHealthCheckMessage_GetMetadata_NilMap(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: nil,
	}

	value, exists := msg.GetMetadata("key1")
	assert.False(t, exists)
	assert.Equal(t, "", value)
}

// TestHealthCheckMessage_GetMetadata_ExistingKey 测试获取存在的元数据
func TestHealthCheckMessage_GetMetadata_ExistingKey(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: map[string]string{
			"key1": "value1",
		},
	}

	value, exists := msg.GetMetadata("key1")
	assert.True(t, exists)
	assert.Equal(t, "value1", value)
}

// TestHealthCheckMessage_GetMetadata_NonExistingKey 测试获取不存在的元数据
func TestHealthCheckMessage_GetMetadata_NonExistingKey(t *testing.T) {
	msg := &HealthCheckMessage{
		Metadata: map[string]string{
			"key1": "value1",
		},
	}

	value, exists := msg.GetMetadata("key2")
	assert.False(t, exists)
	assert.Equal(t, "", value)
}

// TestGetHealthCheckTopic_AllTypes 测试所有 EventBus 类型的健康检查主题
func TestGetHealthCheckTopic_AllTypes(t *testing.T) {
	tests := []struct {
		eventBusType string
		expected     string
	}{
		{"memory", "jxt-core-memory-health-check"},
		{"kafka", "jxt-core-kafka-health-check"},
		{"nats", "jxt-core-nats-health-check"},
		{"unknown", "jxt-core-unknown-health-check"},
	}

	for _, tt := range tests {
		t.Run(tt.eventBusType, func(t *testing.T) {
			topic := GetHealthCheckTopic(tt.eventBusType)
			assert.Equal(t, tt.expected, topic)
		})
	}
}

// TestHealthCheckMessageValidator_Validate_AllScenarios 测试所有验证场景
func TestHealthCheckMessageValidator_Validate_AllScenarios(t *testing.T) {
	validator := NewHealthCheckMessageValidator()

	tests := []struct {
		name      string
		msg       *HealthCheckMessage
		expectErr bool
	}{
		{
			name: "valid message",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now(),
				Version:      "1.0.0",
			},
			expectErr: false,
		},
		{
			name:      "nil message",
			msg:       nil,
			expectErr: true,
		},
		{
			name: "empty message id",
			msg: &HealthCheckMessage{
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expectErr: true,
		},
		{
			name: "empty source",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				EventBusType: "memory",
				Timestamp:    time.Now(),
			},
			expectErr: true,
		},
		{
			name: "empty event bus type",
			msg: &HealthCheckMessage{
				MessageID: "test-id",
				Source:    "test-source",
				Timestamp: time.Now(),
			},
			expectErr: true,
		},
		{
			name: "zero timestamp",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Time{},
			},
			expectErr: true,
		},
		{
			name: "timestamp too far in future",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now().Add(2 * time.Minute),
			},
			expectErr: true,
		},
		{
			name: "timestamp too far in past",
			msg: &HealthCheckMessage{
				MessageID:    "test-id",
				Source:       "test-source",
				EventBusType: "memory",
				Timestamp:    time.Now().Add(-10 * time.Minute),
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.msg)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestHealthCheckMessageParser_Parse 测试消息解析
func TestHealthCheckMessageParser_Parse(t *testing.T) {
	parser := NewHealthCheckMessageParser()

	// 创建一个有效的消息
	original := CreateHealthCheckMessage("test-source", "memory")
	data, err := original.ToBytes()
	require.NoError(t, err)

	// 解析消息
	parsed, err := parser.Parse(data)
	assert.NoError(t, err)
	assert.NotNil(t, parsed)
	assert.Equal(t, original.MessageID, parsed.MessageID)
	assert.Equal(t, original.Source, parsed.Source)
}

// TestHealthCheckMessageParser_Parse_InvalidData 测试解析无效数据
func TestHealthCheckMessageParser_Parse_InvalidData(t *testing.T) {
	parser := NewHealthCheckMessageParser()

	parsed, err := parser.Parse([]byte("invalid json"))
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

// TestHealthCheckMessageParser_Parse_EmptyData 测试解析空数据
func TestHealthCheckMessageParser_Parse_EmptyData(t *testing.T) {
	parser := NewHealthCheckMessageParser()

	parsed, err := parser.Parse([]byte{})
	assert.Error(t, err)
	assert.Nil(t, parsed)
}

