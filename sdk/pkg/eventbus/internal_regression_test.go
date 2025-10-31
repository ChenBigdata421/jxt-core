package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ========== 内部函数测试 ==========
// 这些测试专门针对包内部的私有函数，确保核心逻辑的正确性

// TestExtractAggregateID 测试聚合ID提取逻辑
func TestExtractAggregateID(t *testing.T) {
	tests := []struct {
		name        string
		msgBytes    []byte
		headers     map[string]string
		kafkaKey    []byte
		natsSubject string
		want        string
		wantErr     bool
	}{
		{
			name: "从Envelope提取（优先级1）",
			msgBytes: func() []byte {
				env := NewEnvelopeWithAutoID("order-123", "OrderCreated", 1, []byte(`{}`))
				data, _ := env.ToBytes()
				return data
			}(),
			headers:     map[string]string{"X-Aggregate-ID": "header-456"},
			kafkaKey:    []byte("key-789"),
			natsSubject: "events.order.created.subject-999",
			want:        "order-123",
			wantErr:     false,
		},
		{
			name:        "从Header提取（优先级2）",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{"X-Aggregate-ID": "header-456"},
			kafkaKey:    []byte("key-789"),
			natsSubject: "events.order.created.subject-999",
			want:        "header-456",
			wantErr:     false,
		},
		{
			name:        "从Kafka Key提取（优先级3）",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{},
			kafkaKey:    []byte("key-789"),
			natsSubject: "events.order.created.subject-999",
			want:        "key-789",
			wantErr:     false,
		},
		{
			name:        "从NATS Subject提取（优先级4）",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{},
			kafkaKey:    []byte(""),
			natsSubject: "events.order.created.subject-999",
			want:        "subject-999",
			wantErr:     false,
		},
		{
			name:        "未找到聚合ID",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{},
			kafkaKey:    []byte(""),
			natsSubject: "",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Header多种格式支持",
			msgBytes:    []byte(`{"invalid":"json"}`),
			headers:     map[string]string{"aggregate-id": "header-lowercase"},
			kafkaKey:    []byte(""),
			natsSubject: "",
			want:        "header-lowercase",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractAggregateID(tt.msgBytes, tt.headers, tt.kafkaKey, tt.natsSubject)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestValidateAggregateID 测试聚合ID验证逻辑
func TestValidateAggregateID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"有效的简单ID", "order-123", false},
		{"有效的冒号分隔", "order:123", false},
		{"有效的下划线", "order_123", false},
		{"有效的点分隔", "order.123", false},
		{"有效的斜杠", "order/123", false},
		{"有效的复杂ID", "namespace:order-123_v1.0", false},
		{"空字符串", "", true},
		{"只有空格", "   ", true},
		{"超长ID", string(make([]byte, 257)), true},
		{"包含@符号", "order@123", true},
		{"包含#符号", "order#123", true},
		{"包含空格", "order 123", true},
		{"包含中文", "订单-123", true},
		{"包含特殊字符", "order$123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAggregateID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestExtractAggregateIDFromTopics 测试从不同topic名称中提取聚合ID
func TestExtractAggregateIDFromTopics(t *testing.T) {
	timestamp := time.Now().UnixNano()

	testCases := []struct {
		name            string
		topic           string
		expectedAggID   string
		shouldHaveAggID bool
	}{
		{
			name:            "Simple topic",
			topic:           "debug.simple",
			expectedAggID:   "simple",
			shouldHaveAggID: true,
		},
		{
			name:            "Topic with underscore",
			topic:           "debug.test_1",
			expectedAggID:   "test_1",
			shouldHaveAggID: true,
		},
		{
			name:            "Topic with dash",
			topic:           "debug.test-1",
			expectedAggID:   "test-1",
			shouldHaveAggID: true,
		},
		{
			name:            "Topic with special char @",
			topic:           "debug.test@1",
			expectedAggID:   "debug", // 会从左边的有效部分提取
			shouldHaveAggID: true,
		},
		{
			name:            "Topic with numbers",
			topic:           "debug.123",
			expectedAggID:   "123",
			shouldHaveAggID: true,
		},
		{
			name:            "Stage2 test topic",
			topic:           "stage2.1234567890.test_1",
			expectedAggID:   "test_1",
			shouldHaveAggID: true,
		},
		{
			name:            "Stage2 test topic with timestamp",
			topic:           "stage2." + string(rune(timestamp)) + ".test_1",
			expectedAggID:   "test_1",
			shouldHaveAggID: true,
		},
		{
			name:            "Stage2 topic with special chars",
			topic:           "stage2@1234567890.test@1.msg#1",
			expectedAggID:   "",
			shouldHaveAggID: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 测试原始字节数据（非Envelope）
			testMessage := []byte("test message")

			aggID, err := ExtractAggregateID(testMessage, nil, nil, tc.topic)

			t.Logf("Topic: %s", tc.topic)
			t.Logf("Extracted AggregateID: '%s'", aggID)
			t.Logf("Error: %v", err)
			t.Logf("Has AggregateID: %t", aggID != "")

			if tc.shouldHaveAggID {
				assert.NotEmpty(t, aggID, "Should extract aggregate ID from topic")
				if tc.expectedAggID != "" {
					assert.Equal(t, tc.expectedAggID, aggID, "Should extract expected aggregate ID")
				}
			} else {
				assert.Empty(t, aggID, "Should not extract aggregate ID from topic")
			}
		})
	}
}
