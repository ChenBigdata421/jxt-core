package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTopicPersistenceConfiguration(t *testing.T) {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	// 创建内存EventBus用于测试
	config := &EventBusConfig{
		Type: "memory",
	}

	bus, err := NewEventBus(config)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	t.Run("ConfigureTopic", func(t *testing.T) {
		// 配置主题选项
		options := TopicOptions{
			PersistenceMode: TopicPersistent,
			RetentionTime:   24 * time.Hour,
			MaxSize:         100 * 1024 * 1024,
			MaxMessages:     10000,
			Description:     "Test topic for persistence",
		}

		err := bus.ConfigureTopic(ctx, "test-topic", options)
		assert.NoError(t, err)

		// 验证配置已保存
		savedOptions, err := bus.GetTopicConfig("test-topic")
		assert.NoError(t, err)
		assert.Equal(t, TopicPersistent, savedOptions.PersistenceMode)
		assert.Equal(t, 24*time.Hour, savedOptions.RetentionTime)
		assert.Equal(t, int64(100*1024*1024), savedOptions.MaxSize)
		assert.Equal(t, int64(10000), savedOptions.MaxMessages)
		assert.Equal(t, "Test topic for persistence", savedOptions.Description)
	})

	t.Run("SetTopicPersistence", func(t *testing.T) {
		// 使用简化接口设置持久化
		err := bus.SetTopicPersistence(ctx, "simple-topic", true)
		assert.NoError(t, err)

		// 验证配置
		options, err := bus.GetTopicConfig("simple-topic")
		assert.NoError(t, err)
		assert.Equal(t, TopicPersistent, options.PersistenceMode)

		// 设置为非持久化
		err = bus.SetTopicPersistence(ctx, "simple-topic", false)
		assert.NoError(t, err)

		options, err = bus.GetTopicConfig("simple-topic")
		assert.NoError(t, err)
		assert.Equal(t, TopicEphemeral, options.PersistenceMode)
	})

	t.Run("ListConfiguredTopics", func(t *testing.T) {
		// 配置几个主题
		topics := []string{"topic1", "topic2", "topic3"}
		for _, topic := range topics {
			err := bus.SetTopicPersistence(ctx, topic, true)
			assert.NoError(t, err)
		}

		// 获取配置的主题列表
		configuredTopics := bus.ListConfiguredTopics()
		
		// 验证所有主题都在列表中
		for _, topic := range topics {
			assert.Contains(t, configuredTopics, topic)
		}
	})

	t.Run("GetTopicConfig_NotConfigured", func(t *testing.T) {
		// 获取未配置主题的配置（应该返回默认配置）
		options, err := bus.GetTopicConfig("non-existent-topic")
		assert.NoError(t, err)
		
		// 应该返回默认配置
		defaultOptions := DefaultTopicOptions()
		assert.Equal(t, defaultOptions.PersistenceMode, options.PersistenceMode)
		assert.Equal(t, defaultOptions.RetentionTime, options.RetentionTime)
	})

	t.Run("RemoveTopicConfig", func(t *testing.T) {
		// 先配置一个主题
		err := bus.SetTopicPersistence(ctx, "removable-topic", true)
		assert.NoError(t, err)

		// 验证主题在列表中
		topics := bus.ListConfiguredTopics()
		assert.Contains(t, topics, "removable-topic")

		// 移除配置
		err = bus.RemoveTopicConfig("removable-topic")
		assert.NoError(t, err)

		// 验证主题不再在列表中
		topics = bus.ListConfiguredTopics()
		assert.NotContains(t, topics, "removable-topic")
	})
}

func TestTopicOptionsIsPersistent(t *testing.T) {
	tests := []struct {
		name                    string
		persistenceMode         TopicPersistenceMode
		globalJetStreamEnabled  bool
		expectedPersistent      bool
	}{
		{
			name:                   "Explicit persistent",
			persistenceMode:        TopicPersistent,
			globalJetStreamEnabled: false,
			expectedPersistent:     true,
		},
		{
			name:                   "Explicit ephemeral",
			persistenceMode:        TopicEphemeral,
			globalJetStreamEnabled: true,
			expectedPersistent:     false,
		},
		{
			name:                   "Auto with JetStream enabled",
			persistenceMode:        TopicAuto,
			globalJetStreamEnabled: true,
			expectedPersistent:     true,
		},
		{
			name:                   "Auto with JetStream disabled",
			persistenceMode:        TopicAuto,
			globalJetStreamEnabled: false,
			expectedPersistent:     false,
		},
		{
			name:                   "Invalid mode defaults to global",
			persistenceMode:        "invalid",
			globalJetStreamEnabled: true,
			expectedPersistent:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := TopicOptions{
				PersistenceMode: tt.persistenceMode,
			}

			result := options.IsPersistent(tt.globalJetStreamEnabled)
			assert.Equal(t, tt.expectedPersistent, result)
		})
	}
}

func TestDefaultTopicOptions(t *testing.T) {
	options := DefaultTopicOptions()

	assert.Equal(t, TopicAuto, options.PersistenceMode)
	assert.Equal(t, 24*time.Hour, options.RetentionTime)
	assert.Equal(t, int64(100*1024*1024), options.MaxSize)
	assert.Equal(t, int64(10000), options.MaxMessages)
	assert.Equal(t, 1, options.Replicas)
}

func TestTopicPersistenceIntegration(t *testing.T) {
	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	// 创建内存EventBus
	config := &EventBusConfig{
		Type: "memory",
	}

	bus, err := NewEventBus(config)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 配置主题
	err = bus.SetTopicPersistence(ctx, "test-integration", true)
	require.NoError(t, err)

	// 设置订阅
	received := make(chan []byte, 1)
	err = bus.Subscribe(ctx, "test-integration", func(ctx context.Context, message []byte) error {
		received <- message
		return nil
	})
	require.NoError(t, err)

	// 发布消息
	testMessage := []byte("test message for integration")
	err = bus.Publish(ctx, "test-integration", testMessage)
	require.NoError(t, err)

	// 验证消息接收
	select {
	case msg := <-received:
		assert.Equal(t, testMessage, msg)
	case <-time.After(1 * time.Second):
		t.Fatal("Message not received within timeout")
	}
}
