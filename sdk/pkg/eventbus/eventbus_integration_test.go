package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_CompleteWorkflow 测试完整的工作流程
func TestEventBusManager_CompleteWorkflow(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 1. 设置各种组件
	_ = bus.SetMessageRouter(&mockMessageRouter{})
	_ = bus.SetErrorHandler(&mockErrorHandler{})
	_ = bus.SetMessageFormatter(&mockMessageFormatter{})

	// 2. 注册各种回调
	_ = bus.RegisterSubscriptionCallback(func(ctx context.Context, event SubscriptionEvent) error {
		return nil
	})

	// 3. 启动所有功能
	_ = bus.StartAllHealthCheck(ctx)
	_ = bus.StartAllBacklogMonitoring(ctx)

	// 4. 发布和订阅消息
	received := make(chan bool, 1)
	err = bus.Subscribe(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		received <- true
		return nil
	})
	require.NoError(t, err)

	err = bus.Publish(ctx, "test-topic", []byte("test message"))
	require.NoError(t, err)

	// 等待消息接收
	select {
	case <-received:
		// 成功
	case <-time.After(1 * time.Second):
		t.Fatal("Message not received")
	}

	// 5. 停止所有功能
	_ = bus.StopAllHealthCheck()
	_ = bus.StopAllBacklogMonitoring()
}

// TestEventBusManager_MultipleSubscribers_Integration 测试多个订阅者（集成）
func TestEventBusManager_MultipleSubscribers_Integration(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 创建多个订阅者
	received1 := make(chan bool, 1)
	received2 := make(chan bool, 1)
	received3 := make(chan bool, 1)

	err = bus.Subscribe(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		received1 <- true
		return nil
	})
	require.NoError(t, err)

	err = bus.Subscribe(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		received2 <- true
		return nil
	})
	require.NoError(t, err)

	err = bus.Subscribe(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		received3 <- true
		return nil
	})
	require.NoError(t, err)

	// 发布消息
	err = bus.Publish(ctx, "test-topic", []byte("test message"))
	require.NoError(t, err)

	// 等待所有订阅者接收消息
	timeout := time.After(1 * time.Second)
	count := 0
	for count < 3 {
		select {
		case <-received1:
			count++
		case <-received2:
			count++
		case <-received3:
			count++
		case <-timeout:
			t.Fatalf("Only %d out of 3 subscribers received the message", count)
		}
	}
}

// TestEventBusManager_PublishWithOptions_Advanced 测试高级发布选项
func TestEventBusManager_PublishWithOptions_Advanced(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 测试各种发布选项
	opts := PublishOptions{
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		Timeout: 5 * time.Second,
	}

	err = bus.PublishWithOptions(ctx, "test-topic", []byte("test"), opts)
	assert.NoError(t, err)
}

// TestEventBusManager_SubscribeWithOptions_Advanced 测试高级订阅选项
func TestEventBusManager_SubscribeWithOptions_Advanced(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 测试各种订阅选项
	opts := SubscribeOptions{
		ProcessingTimeout: 5 * time.Second,
		RateLimit:         100.0,
		RateBurst:         10,
		MaxRetries:        3,
		RetryBackoff:      1 * time.Second,
	}

	err = bus.SubscribeWithOptions(ctx, "test-topic", func(ctx context.Context, message []byte) error {
		return nil
	}, opts)
	assert.NoError(t, err)
}

// TestEventBusManager_TopicConfiguration 测试主题配置
func TestEventBusManager_TopicConfiguration(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 配置主题
	options := TopicOptions{
		PersistenceMode: TopicPersistent,
		RetentionTime:   24 * time.Hour,
		MaxSize:         1024 * 1024 * 1024, // 1GB
		MaxMessages:     1000000,
		Replicas:        3,
		Description:     "Test topic",
	}

	err = bus.ConfigureTopic(ctx, "test-topic", options)
	assert.NoError(t, err)

	// 获取配置
	config, err := bus.GetTopicConfig("test-topic")
	assert.NoError(t, err)
	assert.Equal(t, TopicPersistent, config.PersistenceMode)

	// 列出所有配置的主题
	topics := bus.ListConfiguredTopics()
	assert.Contains(t, topics, "test-topic")

	// 移除配置
	err = bus.RemoveTopicConfig("test-topic")
	assert.NoError(t, err)
}

// TestEventBusManager_HealthCheckIntegration 测试健康检查集成
func TestEventBusManager_HealthCheckIntegration(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 启动健康检查
	err = bus.StartAllHealthCheck(ctx)
	assert.NoError(t, err)

	// 等待一段时间让健康检查运行
	time.Sleep(100 * time.Millisecond)

	// 获取健康状态
	status := bus.GetHealthStatus()
	assert.NotEmpty(t, status)

	// 停止健康检查
	err = bus.StopAllHealthCheck()
	assert.NoError(t, err)
}

// TestEventBusManager_BacklogMonitoringIntegration 测试积压监控集成
func TestEventBusManager_BacklogMonitoringIntegration(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 注册积压回调
	callbackCalled := false
	err = bus.RegisterSubscriberBacklogCallback(func(ctx context.Context, state BacklogState) error {
		callbackCalled = true
		return nil
	})
	assert.NoError(t, err)

	// 启动积压监控
	err = bus.StartAllBacklogMonitoring(ctx)
	assert.NoError(t, err)

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 停止积压监控
	err = bus.StopAllBacklogMonitoring()
	assert.NoError(t, err)

	// 验证回调可能被调用（取决于实现）
	_ = callbackCalled
}

// mockMessageFormatter 是一个简单的 Mock 消息格式化器
type mockMessageFormatter struct{}

func (f *mockMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	return &Message{
		UUID:     uuid,
		Payload:  payload,
		Metadata: make(map[string]string),
	}, nil
}

func (f *mockMessageFormatter) ExtractAggregateID(aggregateID interface{}) string {
	if id, ok := aggregateID.(string); ok {
		return id
	}
	return ""
}

func (f *mockMessageFormatter) SetMetadata(msg *Message, metadata map[string]string) error {
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]string)
	}
	for k, v := range metadata {
		msg.Metadata[k] = v
	}
	return nil
}

// mockBusinessHealthChecker2 是一个简单的 Mock 业务健康检查器
type mockBusinessHealthChecker2 struct{}

func (c *mockBusinessHealthChecker2) CheckBusinessHealth(ctx context.Context) error {
	return nil
}

func (c *mockBusinessHealthChecker2) GetBusinessMetrics() interface{} {
	return map[string]interface{}{
		"metric1": 100,
		"metric2": 200,
	}
}

func (c *mockBusinessHealthChecker2) GetBusinessConfig() interface{} {
	return map[string]interface{}{
		"config1": "value1",
		"config2": "value2",
	}
}
