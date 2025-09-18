package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// initTestLogger 初始化测试用的logger
func initTestLogger() {
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}
}

func TestMemoryEventBus(t *testing.T) {
	// 初始化logger
	initTestLogger()

	// 创建内存事件总线配置
	cfg := GetDefaultMemoryConfig()

	// 初始化事件总线
	err := InitializeFromConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize eventbus: %v", err)
	}
	defer CloseGlobal()

	// 获取事件总线实例
	bus := GetGlobal()
	if bus == nil {
		t.Fatal("EventBus not initialized")
	}

	// 测试健康检查
	ctx := context.Background()
	if err := bus.HealthCheck(ctx); err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	// 测试发布和订阅
	topic := "test_topic"
	received := make(chan []byte, 1)

	// 订阅消息
	err = bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
		received <- message
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// 发布消息
	testMessage := []byte("Hello, EventBus!")
	if err := bus.Publish(ctx, topic, testMessage); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// 等待消息接收
	select {
	case msg := <-received:
		if string(msg) != string(testMessage) {
			t.Errorf("Expected message %s, got %s", string(testMessage), string(msg))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Message not received within timeout")
	}
}

func TestEventBusFactory(t *testing.T) {
	// 初始化logger
	initTestLogger()

	// 创建工厂
	cfg := &EventBusConfig{
		Type: "memory",
		Metrics: MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
	}

	factory := NewFactory(cfg)

	// 创建事件总线实例
	bus, err := factory.CreateEventBus()
	if err != nil {
		t.Fatalf("Failed to create eventbus: %v", err)
	}
	defer bus.Close()

	// 测试健康检查
	ctx := context.Background()
	if err := bus.HealthCheck(ctx); err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
}

func TestConfigIntegration(t *testing.T) {
	// 初始化logger
	initTestLogger()

	// 创建配置
	cfg := &config.EventBus{
		Type: "memory",
		Metrics: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 30 * time.Second,
		},
		Tracing: config.TracingConfig{
			Enabled:    false,
			SampleRate: 0.1,
		},
	}

	// 从配置初始化
	if err := InitializeFromConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize from config: %v", err)
	}
	defer CloseGlobal()

	// 检查是否初始化成功
	if !IsInitialized() {
		t.Fatal("EventBus should be initialized")
	}

	// 获取实例
	bus := GetGlobal()
	if bus == nil {
		t.Fatal("EventBus not available")
	}

	// 简单测试
	ctx := context.Background()
	if err := bus.HealthCheck(ctx); err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
}

func TestReconnectCallback(t *testing.T) {
	// 初始化logger
	initTestLogger()

	// 初始化内存事件总线
	cfg := GetDefaultMemoryConfig()
	if err := InitializeFromConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize eventbus: %v", err)
	}
	defer CloseGlobal()

	bus := GetGlobal()

	// 注册重连回调
	callbackCalled := false
	err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
		callbackCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to register reconnect callback: %v", err)
	}

	// 对于内存实现，重连回调可能不会被触发，但注册应该成功
	if callbackCalled {
		t.Log("Reconnect callback was called")
	}
}

func TestMultipleSubscriptions(t *testing.T) {
	// 初始化logger
	initTestLogger()

	// 初始化事件总线
	cfg := GetDefaultMemoryConfig()
	if err := InitializeFromConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize eventbus: %v", err)
	}
	defer CloseGlobal()

	bus := GetGlobal()
	ctx := context.Background()

	// 测试多个主题订阅
	topic1 := "topic1"
	topic2 := "topic2"

	received1 := make(chan []byte, 1)
	received2 := make(chan []byte, 1)

	// 订阅第一个主题
	err := bus.Subscribe(ctx, topic1, func(ctx context.Context, message []byte) error {
		received1 <- message
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to topic1: %v", err)
	}

	// 订阅第二个主题
	err = bus.Subscribe(ctx, topic2, func(ctx context.Context, message []byte) error {
		received2 <- message
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to topic2: %v", err)
	}

	// 发布到第一个主题
	message1 := []byte("Message for topic1")
	if err := bus.Publish(ctx, topic1, message1); err != nil {
		t.Fatalf("Failed to publish to topic1: %v", err)
	}

	// 发布到第二个主题
	message2 := []byte("Message for topic2")
	if err := bus.Publish(ctx, topic2, message2); err != nil {
		t.Fatalf("Failed to publish to topic2: %v", err)
	}

	// 验证消息接收
	select {
	case msg := <-received1:
		if string(msg) != string(message1) {
			t.Errorf("Topic1: Expected %s, got %s", string(message1), string(msg))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Message for topic1 not received")
	}

	select {
	case msg := <-received2:
		if string(msg) != string(message2) {
			t.Errorf("Topic2: Expected %s, got %s", string(message2), string(msg))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Message for topic2 not received")
	}
}

func TestEventBusClose(t *testing.T) {
	// 初始化logger
	initTestLogger()

	// 创建工厂实例
	cfg := &EventBusConfig{
		Type: "memory",
	}

	factory := NewFactory(cfg)
	bus, err := factory.CreateEventBus()
	if err != nil {
		t.Fatalf("Failed to create eventbus: %v", err)
	}

	// 测试关闭
	if err := bus.Close(); err != nil {
		t.Fatalf("Failed to close eventbus: %v", err)
	}

	// 关闭后的操作应该失败
	ctx := context.Background()
	if err := bus.Publish(ctx, "test", []byte("test")); err == nil {
		t.Error("Publish should fail after close")
	}
}

func TestGlobalEventBusManagement(t *testing.T) {
	// 初始化logger
	initTestLogger()

	// 确保开始时没有初始化
	if IsInitialized() {
		CloseGlobal()
	}

	// 测试未初始化状态
	if IsInitialized() {
		t.Error("EventBus should not be initialized")
	}

	if GetGlobal() != nil {
		t.Error("GetGlobal should return nil when not initialized")
	}

	// 初始化
	cfg := GetDefaultMemoryConfig()
	if err := InitializeFromConfig(cfg); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// 测试初始化状态
	if !IsInitialized() {
		t.Error("EventBus should be initialized")
	}

	if GetGlobal() == nil {
		t.Error("GetGlobal should return instance when initialized")
	}

	// 关闭
	if err := CloseGlobal(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// 测试关闭后状态
	if IsInitialized() {
		t.Error("EventBus should not be initialized after close")
	}
}
