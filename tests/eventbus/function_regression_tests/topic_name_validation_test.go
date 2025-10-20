package function_tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTopicNameValidation_ValidNames 测试有效的主题名称
func TestTopicNameValidation_ValidNames(t *testing.T) {
	validNames := []string{
		"orders",
		"user.events",
		"system_logs",
		"payment-service",
		"order.created.v2",
		"user_profile_updated",
		"system-health-check",
		"a",                      // 最短有效名称
		strings.Repeat("a", 255), // 最长有效名称
		"MixedCase123",
		"with.dots.and_underscores-and-dashes",
		"0123456789", // 纯数字
		"topic.with.many.segments",
	}

	for _, name := range validNames {
		t.Run("Valid_"+name, func(t *testing.T) {
			err := eventbus.ValidateTopicName(name)
			assert.NoError(t, err, "Topic name '%s' should be valid", name)
			assert.True(t, eventbus.IsValidTopicName(name), "IsValidTopicName should return true for '%s'", name)
		})
	}
}

// TestTopicNameValidation_InvalidNames 测试无效的主题名称
func TestTopicNameValidation_InvalidNames(t *testing.T) {
	testCases := []struct {
		name          string
		topicName     string
		expectedError string
	}{
		{
			name:          "Empty",
			topicName:     "",
			expectedError: "topic name cannot be empty",
		},
		{
			name:          "TooLong",
			topicName:     strings.Repeat("a", 256),
			expectedError: "topic name too long (256 characters, maximum 255)",
		},
		{
			name:          "ContainsSpace",
			topicName:     "order events",
			expectedError: "topic name cannot contain spaces",
		},
		{
			name:          "ContainsSpaceAtStart",
			topicName:     " orders",
			expectedError: "topic name cannot contain spaces",
		},
		{
			name:          "ContainsSpaceAtEnd",
			topicName:     "orders ",
			expectedError: "topic name cannot contain spaces",
		},
		{
			name:          "ChineseCharacters",
			topicName:     "订单",
			expectedError: "topic name contains non-ASCII character '订' at position 0",
		},
		{
			name:          "MixedChineseEnglish",
			topicName:     "order订单",
			expectedError: "topic name contains non-ASCII character '订' at position 5",
		},
		{
			name:          "JapaneseCharacters",
			topicName:     "注文",
			expectedError: "topic name contains non-ASCII character '注' at position 0",
		},
		{
			name:          "KoreanCharacters",
			topicName:     "주문",
			expectedError: "topic name contains non-ASCII character '주' at position 0",
		},
		{
			name:          "EmojiCharacters",
			topicName:     "orders🎉",
			expectedError: "topic name contains non-ASCII character '🎉' at position 6",
		},
		{
			name:          "ControlCharacter",
			topicName:     "order\x00events",
			expectedError: "topic name contains control character at position 5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := eventbus.ValidateTopicName(tc.topicName)
			require.Error(t, err, "Topic name '%s' should be invalid", tc.topicName)
			assert.Contains(t, err.Error(), tc.expectedError, "Error message should contain expected text")
			assert.False(t, eventbus.IsValidTopicName(tc.topicName), "IsValidTopicName should return false for '%s'", tc.topicName)

			// 验证错误类型
			var validationErr *eventbus.TopicNameValidationError
			assert.ErrorAs(t, err, &validationErr, "Error should be TopicNameValidationError")
			if validationErr != nil {
				assert.Equal(t, tc.topicName, validationErr.Topic, "Error should contain the topic name")
			}
		})
	}
}

// TestTopicBuilder_ValidationIntegration 测试 TopicBuilder 集成验证
func TestTopicBuilder_ValidationIntegration(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	t.Run("ValidTopicName", func(t *testing.T) {
		bus := helper.CreateMemoryEventBus()
		defer bus.Close()

		builder := eventbus.NewTopicBuilder("valid.topic.name")
		require.NotNil(t, builder)

		err := builder.
			WithPersistence(eventbus.TopicPersistent).
			WithRetention(24*time.Hour).
			Build(ctx, bus)

		assert.NoError(t, err, "Building with valid topic name should succeed")
	})

	t.Run("InvalidTopicName_Chinese", func(t *testing.T) {
		bus := helper.CreateMemoryEventBus()
		defer bus.Close()

		builder := eventbus.NewTopicBuilder("订单主题")
		require.NotNil(t, builder)

		err := builder.
			WithPersistence(eventbus.TopicPersistent).
			Build(ctx, bus)

		require.Error(t, err, "Building with Chinese topic name should fail")
		assert.Contains(t, err.Error(), "non-ASCII character")
	})

	t.Run("InvalidTopicName_Space", func(t *testing.T) {
		bus := helper.CreateMemoryEventBus()
		defer bus.Close()

		builder := eventbus.NewTopicBuilder("order events")
		require.NotNil(t, builder)

		err := builder.Build(ctx, bus)

		require.Error(t, err, "Building with space in topic name should fail")
		assert.Contains(t, err.Error(), "cannot contain spaces")
	})

	t.Run("InvalidTopicName_Empty", func(t *testing.T) {
		bus := helper.CreateMemoryEventBus()
		defer bus.Close()

		builder := eventbus.NewTopicBuilder("")
		require.NotNil(t, builder)

		err := builder.Build(ctx, bus)

		require.Error(t, err, "Building with empty topic name should fail")
		assert.Contains(t, err.Error(), "cannot be empty")
	})
}

// TestConfigureTopic_ValidationIntegration 测试 ConfigureTopic 集成验证
func TestConfigureTopic_ValidationIntegration(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	// 测试所有 EventBus 类型
	testCases := []struct {
		name      string
		createBus func() eventbus.EventBus
	}{
		{
			name:      "Memory",
			createBus: helper.CreateMemoryEventBus,
		},
		{
			name: "Kafka",
			createBus: func() eventbus.EventBus {
				return helper.CreateKafkaEventBus("test-validation-kafka")
			},
		},
		{
			name: "NATS",
			createBus: func() eventbus.EventBus {
				return helper.CreateNATSEventBus("test-validation-nats")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bus := tc.createBus()
			defer bus.Close()

			t.Run("ValidTopicName", func(t *testing.T) {
				options := eventbus.TopicOptions{
					PersistenceMode: eventbus.TopicPersistent,
					RetentionTime:   24 * time.Hour,
				}

				err := bus.ConfigureTopic(ctx, "valid.topic.name", options)
				assert.NoError(t, err, "ConfigureTopic with valid name should succeed")
			})

			t.Run("InvalidTopicName_Chinese", func(t *testing.T) {
				options := eventbus.TopicOptions{
					PersistenceMode: eventbus.TopicPersistent,
				}

				err := bus.ConfigureTopic(ctx, "订单主题", options)
				require.Error(t, err, "ConfigureTopic with Chinese name should fail")
				assert.Contains(t, err.Error(), "non-ASCII character")
			})

			t.Run("InvalidTopicName_Space", func(t *testing.T) {
				options := eventbus.TopicOptions{
					PersistenceMode: eventbus.TopicPersistent,
				}

				err := bus.ConfigureTopic(ctx, "order events", options)
				require.Error(t, err, "ConfigureTopic with space should fail")
				assert.Contains(t, err.Error(), "cannot contain spaces")
			})

			t.Run("InvalidTopicName_TooLong", func(t *testing.T) {
				options := eventbus.TopicOptions{
					PersistenceMode: eventbus.TopicPersistent,
				}

				longName := strings.Repeat("a", 256)
				err := bus.ConfigureTopic(ctx, longName, options)
				require.Error(t, err, "ConfigureTopic with too long name should fail")
				assert.Contains(t, err.Error(), "too long")
			})

			t.Run("InvalidTopicName_Empty", func(t *testing.T) {
				options := eventbus.TopicOptions{
					PersistenceMode: eventbus.TopicPersistent,
				}

				err := bus.ConfigureTopic(ctx, "", options)
				require.Error(t, err, "ConfigureTopic with empty name should fail")
				assert.Contains(t, err.Error(), "cannot be empty")
			})
		})
	}
}

// TestSetTopicPersistence_ValidationIntegration 测试 SetTopicPersistence 集成验证
func TestSetTopicPersistence_ValidationIntegration(t *testing.T) {
	helper := NewTestHelper(t)
	ctx := context.Background()

	testCases := []struct {
		name      string
		createBus func() eventbus.EventBus
	}{
		{
			name:      "Memory",
			createBus: helper.CreateMemoryEventBus,
		},
		{
			name: "Kafka",
			createBus: func() eventbus.EventBus {
				return helper.CreateKafkaEventBus("test-persistence-kafka")
			},
		},
		{
			name: "NATS",
			createBus: func() eventbus.EventBus {
				return helper.CreateNATSEventBus("test-persistence-nats")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bus := tc.createBus()
			defer bus.Close()

			t.Run("ValidTopicName", func(t *testing.T) {
				err := bus.SetTopicPersistence(ctx, "valid.topic", true)
				assert.NoError(t, err, "SetTopicPersistence with valid name should succeed")
			})

			t.Run("InvalidTopicName_Chinese", func(t *testing.T) {
				err := bus.SetTopicPersistence(ctx, "订单", true)
				require.Error(t, err, "SetTopicPersistence with Chinese name should fail")
				assert.Contains(t, err.Error(), "non-ASCII character")
			})
		})
	}
}

// TestTopicNameValidation_ErrorMessage 测试错误消息的详细程度
func TestTopicNameValidation_ErrorMessage(t *testing.T) {
	t.Run("ChineseCharacter_DetailedMessage", func(t *testing.T) {
		err := eventbus.ValidateTopicName("order订单")
		require.Error(t, err)

		errMsg := err.Error()
		assert.Contains(t, errMsg, "订", "Error should show the problematic character")
		assert.Contains(t, errMsg, "position 5", "Error should show the position")
		assert.Contains(t, errMsg, "ASCII characters only", "Error should explain the requirement")
		assert.Contains(t, errMsg, "Chinese", "Error should mention Chinese characters")
	})

	t.Run("Space_DetailedMessage", func(t *testing.T) {
		err := eventbus.ValidateTopicName("order events")
		require.Error(t, err)

		errMsg := err.Error()
		assert.Contains(t, errMsg, "cannot contain spaces", "Error should explain the issue")
	})

	t.Run("TooLong_DetailedMessage", func(t *testing.T) {
		longName := strings.Repeat("a", 300)
		err := eventbus.ValidateTopicName(longName)
		require.Error(t, err)

		errMsg := err.Error()
		assert.Contains(t, errMsg, "300 characters", "Error should show actual length")
		assert.Contains(t, errMsg, "maximum 255", "Error should show maximum length")
	})
}
