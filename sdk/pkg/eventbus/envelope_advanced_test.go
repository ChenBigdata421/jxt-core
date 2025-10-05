package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEnvelope 测试创建新的消息包络
func TestNewEnvelope(t *testing.T) {
	payload := []byte(`{"key":"value"}`)
	env := NewEnvelope("agg-123", "test.event", 1, payload)

	assert.NotNil(t, env)
	assert.Equal(t, "agg-123", env.AggregateID)
	assert.Equal(t, "test.event", env.EventType)
	assert.Equal(t, int64(1), env.EventVersion)
	assert.Equal(t, payload, []byte(env.Payload))
	assert.False(t, env.Timestamp.IsZero())
}

// TestEnvelope_Validate 测试包络校验
func TestEnvelope_Validate(t *testing.T) {
	tests := []struct {
		name      string
		envelope  *Envelope
		wantError bool
		errorMsg  string
	}{
		{
			name: "有效的包络",
			envelope: &Envelope{
				AggregateID:  "agg-123",
				EventType:    "test.event",
				EventVersion: 1,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: false,
		},
		{
			name: "缺少AggregateID",
			envelope: &Envelope{
				AggregateID:  "",
				EventType:    "test.event",
				EventVersion: 1,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: true,
			errorMsg:  "aggregate_id is required",
		},
		{
			name: "AggregateID只有空格",
			envelope: &Envelope{
				AggregateID:  "   ",
				EventType:    "test.event",
				EventVersion: 1,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: true,
			errorMsg:  "aggregate_id is required",
		},
		{
			name: "缺少EventType",
			envelope: &Envelope{
				AggregateID:  "agg-123",
				EventType:    "",
				EventVersion: 1,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: true,
			errorMsg:  "event_type is required",
		},
		{
			name: "EventType只有空格",
			envelope: &Envelope{
				AggregateID:  "agg-123",
				EventType:    "   ",
				EventVersion: 1,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: true,
			errorMsg:  "event_type is required",
		},
		{
			name: "EventVersion为0",
			envelope: &Envelope{
				AggregateID:  "agg-123",
				EventType:    "test.event",
				EventVersion: 0,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: true,
			errorMsg:  "event_version must be positive",
		},
		{
			name: "EventVersion为负数",
			envelope: &Envelope{
				AggregateID:  "agg-123",
				EventType:    "test.event",
				EventVersion: -1,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: true,
			errorMsg:  "event_version must be positive",
		},
		{
			name: "缺少Payload",
			envelope: &Envelope{
				AggregateID:  "agg-123",
				EventType:    "test.event",
				EventVersion: 1,
				Payload:      RawMessage{},
			},
			wantError: true,
			errorMsg:  "payload is required",
		},
		{
			name: "AggregateID包含无效字符",
			envelope: &Envelope{
				AggregateID:  "agg@123",
				EventType:    "test.event",
				EventVersion: 1,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: true,
			errorMsg:  "invalid aggregate_id",
		},
		{
			name: "AggregateID太长",
			envelope: &Envelope{
				AggregateID:  string(make([]byte, 257)),
				EventType:    "test.event",
				EventVersion: 1,
				Payload:      RawMessage(`{"key":"value"}`),
			},
			wantError: true,
			errorMsg:  "aggregate_id too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.envelope.Validate()
			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEnvelope_ToBytes 测试序列化
func TestEnvelope_ToBytes(t *testing.T) {
	env := &Envelope{
		AggregateID:  "agg-123",
		EventType:    "test.event",
		EventVersion: 1,
		Timestamp:    time.Now(),
		Payload:      RawMessage(`{"key":"value"}`),
	}

	bytes, err := env.ToBytes()
	require.NoError(t, err)
	assert.NotEmpty(t, bytes)
	assert.Contains(t, string(bytes), "agg-123")
	assert.Contains(t, string(bytes), "test.event")
}

// TestEnvelope_ToBytes_InvalidEnvelope 测试序列化无效包络
func TestEnvelope_ToBytes_InvalidEnvelope(t *testing.T) {
	env := &Envelope{
		AggregateID:  "",
		EventType:    "test.event",
		EventVersion: 1,
		Payload:      RawMessage(`{"key":"value"}`),
	}

	_, err := env.ToBytes()
	assert.Error(t, err)
}

// TestFromBytes 测试反序列化
func TestFromBytes(t *testing.T) {
	original := &Envelope{
		AggregateID:  "agg-123",
		EventType:    "test.event",
		EventVersion: 1,
		Timestamp:    time.Now(),
		Payload:      RawMessage(`{"key":"value"}`),
	}

	bytes, err := original.ToBytes()
	require.NoError(t, err)

	env, err := FromBytes(bytes)
	require.NoError(t, err)
	assert.Equal(t, original.AggregateID, env.AggregateID)
	assert.Equal(t, original.EventType, env.EventType)
	assert.Equal(t, original.EventVersion, env.EventVersion)
}

// TestFromBytes_InvalidJSON 测试反序列化无效JSON
func TestFromBytes_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json}`)

	_, err := FromBytes(invalidJSON)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal envelope")
}

// TestFromBytes_InvalidEnvelope 测试反序列化无效包络
func TestFromBytes_InvalidEnvelope(t *testing.T) {
	invalidEnvelope := []byte(`{"aggregate_id":"","event_type":"test","event_version":1,"payload":"{}"}`)

	_, err := FromBytes(invalidEnvelope)
	assert.Error(t, err)
	// 错误消息可能是 "invalid envelope" 或 "failed to unmarshal envelope"
	assert.True(t, err != nil, "should return an error for invalid envelope")
}

// TestExtractAggregateID_FromEnvelope 测试从Envelope提取
func TestExtractAggregateID_FromEnvelope(t *testing.T) {
	env := &Envelope{
		AggregateID:  "agg-123",
		EventType:    "test.event",
		EventVersion: 1,
		Payload:      RawMessage(`{"key":"value"}`),
	}

	bytes, err := env.ToBytes()
	require.NoError(t, err)

	aggID, err := ExtractAggregateID(bytes, nil, nil, "")
	assert.NoError(t, err)
	assert.Equal(t, "agg-123", aggID)
}

// TestExtractAggregateID_FromHeaders 测试从Headers提取
func TestExtractAggregateID_FromHeaders(t *testing.T) {
	headers := map[string]string{
		"X-Aggregate-ID": "agg-456",
	}

	aggID, err := ExtractAggregateID([]byte("invalid json"), headers, nil, "")
	assert.NoError(t, err)
	assert.Equal(t, "agg-456", aggID)
}

// TestExtractAggregateID_FromHeaders_DifferentKeys 测试从不同的Header键提取
func TestExtractAggregateID_FromHeaders_DifferentKeys(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "X-Aggregate-ID",
			headers: map[string]string{"X-Aggregate-ID": "agg-1"},
			want:    "agg-1",
		},
		{
			name:    "x-aggregate-id",
			headers: map[string]string{"x-aggregate-id": "agg-2"},
			want:    "agg-2",
		},
		{
			name:    "Aggregate-ID",
			headers: map[string]string{"Aggregate-ID": "agg-3"},
			want:    "agg-3",
		},
		{
			name:    "aggregate-id",
			headers: map[string]string{"aggregate-id": "agg-4"},
			want:    "agg-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggID, err := ExtractAggregateID([]byte("invalid"), tt.headers, nil, "")
			assert.NoError(t, err)
			assert.Equal(t, tt.want, aggID)
		})
	}
}

// TestExtractAggregateID_FromKafkaKey 测试从Kafka Key提取
func TestExtractAggregateID_FromKafkaKey(t *testing.T) {
	kafkaKey := []byte("agg-789")

	aggID, err := ExtractAggregateID([]byte("invalid"), nil, kafkaKey, "")
	assert.NoError(t, err)
	assert.Equal(t, "agg-789", aggID)
}

// TestExtractAggregateID_FromNATSSubject 测试从NATS Subject提取
func TestExtractAggregateID_FromNATSSubject(t *testing.T) {
	subject := "events.user.agg-999"

	aggID, err := ExtractAggregateID([]byte("invalid"), nil, nil, subject)
	assert.NoError(t, err)
	assert.Equal(t, "agg-999", aggID)
}

// TestExtractAggregateID_Priority 测试提取优先级
func TestExtractAggregateID_Priority(t *testing.T) {
	// Envelope 优先级最高
	env := &Envelope{
		AggregateID:  "from-envelope",
		EventType:    "test.event",
		EventVersion: 1,
		Payload:      RawMessage(`{"key":"value"}`),
	}
	bytes, _ := env.ToBytes()

	headers := map[string]string{"X-Aggregate-ID": "from-header"}
	kafkaKey := []byte("from-kafka")
	subject := "events.user.from-nats"

	aggID, err := ExtractAggregateID(bytes, headers, kafkaKey, subject)
	assert.NoError(t, err)
	assert.Equal(t, "from-envelope", aggID)
}

// TestExtractAggregateID_NotFound 测试无法提取
func TestExtractAggregateID_NotFound(t *testing.T) {
	aggID, err := ExtractAggregateID([]byte("invalid"), nil, nil, "")
	assert.Error(t, err)
	assert.Equal(t, "", aggID)
	assert.Contains(t, err.Error(), "aggregate_id not found")
}

// TestValidateAggregateID_ValidFormats 测试有效格式
func TestValidateAggregateID_ValidFormats(t *testing.T) {
	validIDs := []string{
		"simple",
		"with-dash",
		"with_underscore",
		"with.dot",
		"with:colon",
		"with/slash",
		"MixedCase123",
		"user:123",
		"order/456",
		"evidence.789",
	}

	for _, id := range validIDs {
		t.Run(id, func(t *testing.T) {
			err := validateAggregateID(id)
			assert.NoError(t, err)
		})
	}
}

// TestValidateAggregateID_InvalidFormats 测试无效格式
func TestValidateAggregateID_InvalidFormats(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		errorMsg string
	}{
		{
			name:     "空字符串",
			id:       "",
			errorMsg: "cannot be empty",
		},
		{
			name:     "只有空格",
			id:       "   ",
			errorMsg: "cannot be empty",
		},
		{
			name:     "包含@符号",
			id:       "user@123",
			errorMsg: "invalid character: @",
		},
		{
			name:     "包含#符号",
			id:       "order#456",
			errorMsg: "invalid character: #",
		},
		{
			name:     "包含空格",
			id:       "user 123",
			errorMsg: "invalid character",
		},
		{
			name:     "包含中文",
			id:       "用户123",
			errorMsg: "invalid character",
		},
		{
			name:     "太长",
			id:       string(make([]byte, 257)),
			errorMsg: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAggregateID(tt.id)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}
