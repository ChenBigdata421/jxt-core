package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
			expectedAggID:   "",
			shouldHaveAggID: false,
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
