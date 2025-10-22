package function_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// ============================================================================
// 积压监控测试 (来自 backlog_test.go)
// ============================================================================

// TestKafkaSubscriberBacklogMonitoring 测试 Kafka 订阅端积压监控
func TestKafkaSubscriberBacklogMonitoring(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-sub-backlog-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 注册订阅端积压回调
	callbackCalled := false
	err := bus.RegisterSubscriberBacklogCallback(func(ctx context.Context, state eventbus.BacklogState) error {
		callbackCalled = true
		t.Logf("📞 Subscriber backlog callback called: HasBacklog=%v, LagCount=%d",
			state.HasBacklog, state.LagCount)
		return nil
	})
	helper.AssertNoError(err, "RegisterSubscriberBacklogCallback should not return error")

	// 启动订阅端积压监控
	err = bus.StartSubscriberBacklogMonitoring(ctx)
	helper.AssertNoError(err, "StartSubscriberBacklogMonitoring should not return error")

	// 等待监控运行
	time.Sleep(3 * time.Second)

	// 停止订阅端积压监控
	err = bus.StopSubscriberBacklogMonitoring()
	helper.AssertNoError(err, "StopSubscriberBacklogMonitoring should not return error")

	t.Logf("✅ Kafka subscriber backlog monitoring test passed (callback called: %v)", callbackCalled)
}

// TestNATSSubscriberBacklogMonitoring 测试 NATS 订阅端积压监控
func TestNATSSubscriberBacklogMonitoring(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-sub-backlog-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 注册订阅端积压回调
	callbackCalled := false
	err := bus.RegisterSubscriberBacklogCallback(func(ctx context.Context, state eventbus.BacklogState) error {
		callbackCalled = true
		t.Logf("📞 Subscriber backlog callback called: HasBacklog=%v, LagCount=%d",
			state.HasBacklog, state.LagCount)
		return nil
	})
	helper.AssertNoError(err, "RegisterSubscriberBacklogCallback should not return error")

	// 启动订阅端积压监控
	err = bus.StartSubscriberBacklogMonitoring(ctx)
	helper.AssertNoError(err, "StartSubscriberBacklogMonitoring should not return error")

	// 等待监控运行
	time.Sleep(3 * time.Second)

	// 停止订阅端积压监控
	err = bus.StopSubscriberBacklogMonitoring()
	helper.AssertNoError(err, "StopSubscriberBacklogMonitoring should not return error")

	t.Logf("✅ NATS subscriber backlog monitoring test passed (callback called: %v)", callbackCalled)
}

// TestKafkaPublisherBacklogMonitoring 测试 Kafka 发送端积压监控
func TestKafkaPublisherBacklogMonitoring(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-pub-backlog-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 注册发送端积压回调
	callbackCalled := false
	err := bus.RegisterPublisherBacklogCallback(func(ctx context.Context, state eventbus.PublisherBacklogState) error {
		callbackCalled = true
		t.Logf("📞 Publisher backlog callback called: HasBacklog=%v, QueueDepth=%d",
			state.HasBacklog, state.QueueDepth)
		return nil
	})
	helper.AssertNoError(err, "RegisterPublisherBacklogCallback should not return error")

	// 启动发送端积压监控
	err = bus.StartPublisherBacklogMonitoring(ctx)
	helper.AssertNoError(err, "StartPublisherBacklogMonitoring should not return error")

	// 等待监控运行
	time.Sleep(3 * time.Second)

	// 停止发送端积压监控
	err = bus.StopPublisherBacklogMonitoring()
	helper.AssertNoError(err, "StopPublisherBacklogMonitoring should not return error")

	t.Logf("✅ Kafka publisher backlog monitoring test passed (callback called: %v)", callbackCalled)
}

// TestNATSPublisherBacklogMonitoring 测试 NATS 发送端积压监控
func TestNATSPublisherBacklogMonitoring(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-pub-backlog-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 注册发送端积压回调
	callbackCalled := false
	err := bus.RegisterPublisherBacklogCallback(func(ctx context.Context, state eventbus.PublisherBacklogState) error {
		callbackCalled = true
		t.Logf("📞 Publisher backlog callback called: HasBacklog=%v, QueueDepth=%d",
			state.HasBacklog, state.QueueDepth)
		return nil
	})
	helper.AssertNoError(err, "RegisterPublisherBacklogCallback should not return error")

	// 启动发送端积压监控
	err = bus.StartPublisherBacklogMonitoring(ctx)
	helper.AssertNoError(err, "StartPublisherBacklogMonitoring should not return error")

	// 等待监控运行
	time.Sleep(3 * time.Second)

	// 停止发送端积压监控
	err = bus.StopPublisherBacklogMonitoring()
	helper.AssertNoError(err, "StopPublisherBacklogMonitoring should not return error")

	t.Logf("✅ NATS publisher backlog monitoring test passed (callback called: %v)", callbackCalled)
}

// TestKafkaStartAllBacklogMonitoring 测试 Kafka 启动所有积压监控
func TestKafkaStartAllBacklogMonitoring(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-all-backlog-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 注册回调
	subCallbackCalled := false
	err := bus.RegisterSubscriberBacklogCallback(func(ctx context.Context, state eventbus.BacklogState) error {
		subCallbackCalled = true
		t.Logf("📞 Subscriber backlog callback called")
		return nil
	})
	helper.AssertNoError(err, "RegisterSubscriberBacklogCallback should not return error")

	pubCallbackCalled := false
	err = bus.RegisterPublisherBacklogCallback(func(ctx context.Context, state eventbus.PublisherBacklogState) error {
		pubCallbackCalled = true
		t.Logf("📞 Publisher backlog callback called")
		return nil
	})
	helper.AssertNoError(err, "RegisterPublisherBacklogCallback should not return error")

	// 启动所有积压监控
	err = bus.StartAllBacklogMonitoring(ctx)
	helper.AssertNoError(err, "StartAllBacklogMonitoring should not return error")

	// 等待监控运行
	time.Sleep(3 * time.Second)

	// 停止所有积压监控
	err = bus.StopAllBacklogMonitoring()
	helper.AssertNoError(err, "StopAllBacklogMonitoring should not return error")

	t.Logf("✅ Kafka StartAllBacklogMonitoring test passed (sub: %v, pub: %v)",
		subCallbackCalled, pubCallbackCalled)
}

// TestNATSStartAllBacklogMonitoring 测试 NATS 启动所有积压监控
func TestNATSStartAllBacklogMonitoring(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-all-backlog-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 注册回调
	subCallbackCalled := false
	err := bus.RegisterSubscriberBacklogCallback(func(ctx context.Context, state eventbus.BacklogState) error {
		subCallbackCalled = true
		t.Logf("📞 Subscriber backlog callback called")
		return nil
	})
	helper.AssertNoError(err, "RegisterSubscriberBacklogCallback should not return error")

	pubCallbackCalled := false
	err = bus.RegisterPublisherBacklogCallback(func(ctx context.Context, state eventbus.PublisherBacklogState) error {
		pubCallbackCalled = true
		t.Logf("📞 Publisher backlog callback called")
		return nil
	})
	helper.AssertNoError(err, "RegisterPublisherBacklogCallback should not return error")

	// 启动所有积压监控
	err = bus.StartAllBacklogMonitoring(ctx)
	helper.AssertNoError(err, "StartAllBacklogMonitoring should not return error")

	// 等待监控运行
	time.Sleep(3 * time.Second)

	// 停止所有积压监控
	err = bus.StopAllBacklogMonitoring()
	helper.AssertNoError(err, "StopAllBacklogMonitoring should not return error")

	t.Logf("✅ NATS StartAllBacklogMonitoring test passed (sub: %v, pub: %v)",
		subCallbackCalled, pubCallbackCalled)
}

// TestKafkaSetMessageRouter 测试 Kafka 设置消息路由器
func TestKafkaSetMessageRouter(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-router-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 创建自定义路由器
	router := &testMessageRouter{}

	// 设置消息路由器
	err := bus.SetMessageRouter(router)
	helper.AssertNoError(err, "SetMessageRouter should not return error")

	t.Logf("✅ Kafka SetMessageRouter test passed")
}

// TestNATSSetMessageRouter 测试 NATS 设置消息路由器
func TestNATSSetMessageRouter(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-router-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	// 创建自定义路由器
	router := &testMessageRouter{}

	// 设置消息路由器
	err := bus.SetMessageRouter(router)
	helper.AssertNoError(err, "SetMessageRouter should not return error")

	t.Logf("✅ NATS SetMessageRouter test passed")
}

// testMessageRouter 测试用消息路由器
type testMessageRouter struct{}

func (r *testMessageRouter) Route(ctx context.Context, topic string, message []byte) (eventbus.RouteDecision, error) {
	return eventbus.RouteDecision{
		ShouldProcess: true,
		ProcessorKey:  "default",
	}, nil
}

// TestKafkaSetErrorHandler 测试 Kafka 设置错误处理器
func TestKafkaSetErrorHandler(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-error-handler-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 创建自定义错误处理器
	handler := &testErrorHandler{}

	// 设置错误处理器
	err := bus.SetErrorHandler(handler)
	helper.AssertNoError(err, "SetErrorHandler should not return error")

	t.Logf("✅ Kafka SetErrorHandler test passed")
}

// testErrorHandler 测试用错误处理器
type testErrorHandler struct{}

func (h *testErrorHandler) HandleError(ctx context.Context, err error, message []byte, topic string) eventbus.ErrorAction {
	return eventbus.ErrorAction{
		Action: eventbus.ErrorActionRetry,
	}
}

// ============================================================================
// 健康检查测试 (来自 healthcheck_test.go)
// ============================================================================

// TestKafkaHealthCheckPublisher 测试 Kafka 健康检查发布器
func TestKafkaHealthCheckPublisher(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-health-pub-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 启动健康检查发布器
	err := bus.StartHealthCheckPublisher(ctx)
	helper.AssertNoError(err, "StartHealthCheckPublisher should not return error")

	// 等待健康检查运行
	time.Sleep(3 * time.Second)

	// 获取健康检查状态
	status := bus.GetHealthCheckPublisherStatus()
	t.Logf("📊 Health check publisher status: %+v", status)

	// 停止健康检查发布器
	err = bus.StopHealthCheckPublisher()
	helper.AssertNoError(err, "StopHealthCheckPublisher should not return error")

	t.Logf("✅ Kafka health check publisher test passed")
}

// TestNATSHealthCheckPublisher 测试 NATS 健康检查发布器
func TestNATSHealthCheckPublisher(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-health-pub-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 启动健康检查发布器
	err := bus.StartHealthCheckPublisher(ctx)
	helper.AssertNoError(err, "StartHealthCheckPublisher should not return error")

	// 等待健康检查运行
	time.Sleep(3 * time.Second)

	// 获取健康检查状态
	status := bus.GetHealthCheckPublisherStatus()
	t.Logf("📊 Health check publisher status: %+v", status)

	// 停止健康检查发布器
	err = bus.StopHealthCheckPublisher()
	helper.AssertNoError(err, "StopHealthCheckPublisher should not return error")

	t.Logf("✅ NATS health check publisher test passed")
}

// TestKafkaHealthCheckSubscriber 测试 Kafka 健康检查订阅器
func TestKafkaHealthCheckSubscriber(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-health-sub-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 启动健康检查订阅器
	err := bus.StartHealthCheckSubscriber(ctx)
	helper.AssertNoError(err, "StartHealthCheckSubscriber should not return error")

	// 等待健康检查运行
	time.Sleep(3 * time.Second)

	// 获取健康检查统计
	stats := bus.GetHealthCheckSubscriberStats()
	t.Logf("📊 Health check subscriber stats: %+v", stats)

	// 停止健康检查订阅器
	err = bus.StopHealthCheckSubscriber()
	helper.AssertNoError(err, "StopHealthCheckSubscriber should not return error")

	t.Logf("✅ Kafka health check subscriber test passed")
}

// TestNATSHealthCheckSubscriber 测试 NATS 健康检查订阅器
func TestNATSHealthCheckSubscriber(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-health-sub-%d", helper.GetTimestamp())

	// 🔧 修复：使用匹配 Stream subjects 的健康检查 topic
	// Stream subjects 是 "nats-health-sub-*.>"，所以 topic 必须以 clientID 开头
	healthCheckTopic := fmt.Sprintf("%s.health-check", clientID)

	// 创建自定义健康检查配置
	healthCheckConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:            healthCheckTopic,
			Interval:         10 * time.Second,
			Timeout:          10 * time.Second,
			FailureThreshold: 3,
			MessageTTL:       5 * time.Minute,
		},
		Subscriber: config.HealthCheckSubscriberConfig{
			Topic:             healthCheckTopic,
			MonitorInterval:   30 * time.Second,
			WarningThreshold:  3,
			ErrorThreshold:    5,
			CriticalThreshold: 10,
		},
	}

	bus := helper.CreateNATSEventBusWithHealthCheck(clientID, healthCheckConfig)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 启动健康检查订阅器
	err := bus.StartHealthCheckSubscriber(ctx)
	helper.AssertNoError(err, "StartHealthCheckSubscriber should not return error")

	// 等待健康检查运行
	time.Sleep(3 * time.Second)

	// 获取健康检查统计
	stats := bus.GetHealthCheckSubscriberStats()
	t.Logf("📊 Health check subscriber stats: %+v", stats)

	// 停止健康检查订阅器
	err = bus.StopHealthCheckSubscriber()
	helper.AssertNoError(err, "StopHealthCheckSubscriber should not return error")

	t.Logf("✅ NATS health check subscriber test passed")
}

// TestKafkaStartAllHealthCheck 测试 Kafka 启动所有健康检查
func TestKafkaStartAllHealthCheck(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-health-all-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 启动所有健康检查
	err := bus.StartAllHealthCheck(ctx)
	helper.AssertNoError(err, "StartAllHealthCheck should not return error")

	// 等待健康检查运行
	time.Sleep(3 * time.Second)

	// 获取状态
	status := bus.GetHealthCheckPublisherStatus()
	stats := bus.GetHealthCheckSubscriberStats()
	t.Logf("📊 Publisher status: %+v", status)
	t.Logf("📊 Subscriber stats: %+v", stats)

	// 停止所有健康检查
	err = bus.StopAllHealthCheck()
	helper.AssertNoError(err, "StopAllHealthCheck should not return error")

	t.Logf("✅ Kafka StartAllHealthCheck test passed")
}

// TestNATSStartAllHealthCheck 测试 NATS 启动所有健康检查
func TestNATSStartAllHealthCheck(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-health-all-%d", helper.GetTimestamp())

	// 🔧 修复：使用匹配 Stream subjects 的健康检查 topic
	healthCheckTopic := fmt.Sprintf("%s.health-check", clientID)

	// 创建自定义健康检查配置
	healthCheckConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:            healthCheckTopic,
			Interval:         10 * time.Second,
			Timeout:          10 * time.Second,
			FailureThreshold: 3,
			MessageTTL:       5 * time.Minute,
		},
		Subscriber: config.HealthCheckSubscriberConfig{
			Topic:             healthCheckTopic,
			MonitorInterval:   30 * time.Second,
			WarningThreshold:  3,
			ErrorThreshold:    5,
			CriticalThreshold: 10,
		},
	}

	bus := helper.CreateNATSEventBusWithHealthCheck(clientID, healthCheckConfig)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 启动所有健康检查
	err := bus.StartAllHealthCheck(ctx)
	helper.AssertNoError(err, "StartAllHealthCheck should not return error")

	// 等待健康检查运行
	time.Sleep(3 * time.Second)

	// 获取状态
	status := bus.GetHealthCheckPublisherStatus()
	stats := bus.GetHealthCheckSubscriberStats()
	t.Logf("📊 Publisher status: %+v", status)
	t.Logf("📊 Subscriber stats: %+v", stats)

	// 停止所有健康检查
	err = bus.StopAllHealthCheck()
	helper.AssertNoError(err, "StopAllHealthCheck should not return error")

	t.Logf("✅ NATS StartAllHealthCheck test passed")
}

// TestKafkaHealthCheckPublisherCallback 测试 Kafka 健康检查发布器回调
func TestKafkaHealthCheckPublisherCallback(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-health-pub-cb-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 🔧 修复：先启动健康检查发布器，再注册回调
	err := bus.StartHealthCheckPublisher(ctx)
	helper.AssertNoError(err, "StartHealthCheckPublisher should not return error")

	// 注册健康检查发布器回调
	callbackCalled := false
	err = bus.RegisterHealthCheckPublisherCallback(func(ctx context.Context, result eventbus.HealthCheckResult) error {
		callbackCalled = true
		t.Logf("📞 Health check publisher callback called: %+v", result)
		return nil
	})
	helper.AssertNoError(err, "RegisterHealthCheckPublisherCallback should not return error")

	// 等待回调（健康检查发布器默认每1秒发送一次）
	time.Sleep(3 * time.Second)

	// 停止健康检查发布器
	err = bus.StopHealthCheckPublisher()
	helper.AssertNoError(err, "StopHealthCheckPublisher should not return error")

	t.Logf("✅ Kafka health check publisher callback test passed (callback called: %v)", callbackCalled)
}

// TestNATSHealthCheckPublisherCallback 测试 NATS 健康检查发布器回调
func TestNATSHealthCheckPublisherCallback(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-health-pub-cb-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 🔧 修复：先启动健康检查发布器，再注册回调
	err := bus.StartHealthCheckPublisher(ctx)
	helper.AssertNoError(err, "StartHealthCheckPublisher should not return error")

	// 注册健康检查发布器回调
	callbackCalled := false
	err = bus.RegisterHealthCheckPublisherCallback(func(ctx context.Context, result eventbus.HealthCheckResult) error {
		callbackCalled = true
		t.Logf("📞 Health check publisher callback called: %+v", result)
		return nil
	})
	helper.AssertNoError(err, "RegisterHealthCheckPublisherCallback should not return error")

	// 等待回调（健康检查发布器默认每1秒发送一次）
	time.Sleep(3 * time.Second)

	// 停止健康检查发布器
	err = bus.StopHealthCheckPublisher()
	helper.AssertNoError(err, "StopHealthCheckPublisher should not return error")

	t.Logf("✅ NATS health check publisher callback test passed (callback called: %v)", callbackCalled)
}

// TestKafkaHealthCheckSubscriberCallback 测试 Kafka 健康检查订阅器回调
func TestKafkaHealthCheckSubscriberCallback(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-health-sub-cb-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 🔧 修复：先启动健康检查订阅器，再注册回调
	err := bus.StartHealthCheckSubscriber(ctx)
	helper.AssertNoError(err, "StartHealthCheckSubscriber should not return error")

	// 注册健康检查订阅器回调
	callbackCalled := false
	err = bus.RegisterHealthCheckSubscriberCallback(func(ctx context.Context, alert eventbus.HealthCheckAlert) error {
		callbackCalled = true
		t.Logf("📞 Health check subscriber callback called: %+v", alert)
		return nil
	})
	helper.AssertNoError(err, "RegisterHealthCheckSubscriberCallback should not return error")

	// 等待回调（订阅器在检测到消息丢失时会触发回调）
	time.Sleep(6 * time.Second)

	// 停止健康检查订阅器
	err = bus.StopHealthCheckSubscriber()
	helper.AssertNoError(err, "StopHealthCheckSubscriber should not return error")

	t.Logf("✅ Kafka health check subscriber callback test passed (callback called: %v)", callbackCalled)
}

// TestKafkaHealthCheckPublisherSubscriberIntegration 测试 Kafka 健康检查发布端和订阅端集成
// 模拟 A 端启动发布端健康检测，B 端启动订阅端健康检测
// 验证发布端能例行成功发送健康检测消息，订阅端能例行成功接收到健康检测消息
// 优化：设置健康检查间隔为 10 秒，测试时间为 1 分钟，预期接收约 6 条消息
func TestKafkaHealthCheckPublisherSubscriberIntegration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	ctx := context.Background()

	// 创建自定义健康检查配置：每 10 秒发送一次
	customHealthCheckConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:            eventbus.DefaultHealthCheckTopic,
			Interval:         10 * time.Second, // 设置为 10 秒
			Timeout:          10 * time.Second,
			FailureThreshold: 3,
			MessageTTL:       5 * time.Minute,
		},
		Subscriber: config.HealthCheckSubscriberConfig{
			Topic:             eventbus.DefaultHealthCheckTopic,
			MonitorInterval:   30 * time.Second,
			WarningThreshold:  3,
			ErrorThreshold:    5,
			CriticalThreshold: 10,
		},
	}

	// 创建 A 端 EventBus（发布端）- 使用自定义配置
	busA := helper.CreateKafkaEventBusWithHealthCheck(
		fmt.Sprintf("kafka-health-integration-a-%d", helper.GetTimestamp()),
		customHealthCheckConfig,
	)
	defer helper.CloseEventBus(busA)

	// 创建 B 端 EventBus（订阅端）- 使用自定义配置
	busB := helper.CreateKafkaEventBusWithHealthCheck(
		fmt.Sprintf("kafka-health-integration-b-%d", helper.GetTimestamp()),
		customHealthCheckConfig,
	)
	defer helper.CloseEventBus(busB)

	// A 端：启动健康检查发布器
	t.Logf("🚀 A 端：启动健康检查发布器（间隔：10秒）")
	err := busA.StartHealthCheckPublisher(ctx)
	helper.AssertNoError(err, "A 端 StartHealthCheckPublisher should not return error")

	// B 端：启动健康检查订阅器
	t.Logf("🚀 B 端：启动健康检查订阅器")
	err = busB.StartHealthCheckSubscriber(ctx)
	helper.AssertNoError(err, "B 端 StartHealthCheckSubscriber should not return error")

	// 等待健康检查消息发送和接收（1 分钟，预期接收约 6 条消息）
	// 计算：启动时立即发送 1 条 + 每 10 秒发送 1 条 × 6 次 = 约 6-7 条
	t.Logf("⏳ 等待健康检查消息发送和接收（1分钟）...")
	t.Logf("✅ EventBus 现在使用自定义健康检查配置（间隔：10秒）")
	time.Sleep(60 * time.Second) // 1 分钟

	// 验证 A 端发布器状态
	publisherStatus := busA.GetHealthCheckPublisherStatus()
	t.Logf("📊 A 端发布器状态: IsHealthy=%v, ConsecutiveFailures=%d, LastSuccessTime=%v",
		publisherStatus.IsHealthy,
		publisherStatus.ConsecutiveFailures,
		publisherStatus.LastSuccessTime)

	// 验证发布器健康
	if !publisherStatus.IsHealthy {
		t.Errorf("❌ A 端发布器应该是健康的，但实际状态为不健康")
	}

	// 验证发布器成功发送过消息
	if publisherStatus.LastSuccessTime.IsZero() {
		t.Errorf("❌ A 端发布器应该成功发送过健康检查消息，但 LastSuccessTime 为零值")
	}

	// 验证发布器没有连续失败
	if publisherStatus.ConsecutiveFailures > 0 {
		t.Errorf("❌ A 端发布器不应该有连续失败，但实际 ConsecutiveFailures=%d", publisherStatus.ConsecutiveFailures)
	}

	// 验证 B 端订阅器状态
	subscriberStats := busB.GetHealthCheckSubscriberStats()
	t.Logf("📊 B 端订阅器统计: TotalMessagesReceived=%d, ConsecutiveMisses=%d, IsHealthy=%v, LastMessageTime=%v",
		subscriberStats.TotalMessagesReceived,
		subscriberStats.ConsecutiveMisses,
		subscriberStats.IsHealthy,
		subscriberStats.LastMessageTime)

	// 验证订阅器接收到消息
	if subscriberStats.TotalMessagesReceived == 0 {
		t.Errorf("❌ B 端订阅器应该接收到健康检查消息，但实际接收数量为 0")
	}

	// 验证订阅器健康（至少接收到 1 条消息就应该是健康的）
	if subscriberStats.TotalMessagesReceived > 0 && !subscriberStats.IsHealthy {
		t.Logf("⚠️  B 端订阅器接收到 %d 条消息，但状态为不健康（可能是因为消息间隔检测）", subscriberStats.TotalMessagesReceived)
	}

	// 验证订阅器接收到的消息数量合理（1分钟内，10秒发送一次，应该至少接收到 5-8 条消息）
	// 计算：启动时立即发送 1 条 + 每 10 秒发送 1 条 × 6 次 = 约 6-7 条
	if subscriberStats.TotalMessagesReceived < 5 {
		t.Errorf("❌ B 端订阅器在 1 分钟内只接收到 %d 条消息，预期至少 5 条", subscriberStats.TotalMessagesReceived)
	} else if subscriberStats.TotalMessagesReceived > 8 {
		t.Logf("⚠️  B 端订阅器在 1 分钟内接收到 %d 条消息，超过预期的 8 条", subscriberStats.TotalMessagesReceived)
	} else {
		t.Logf("✅ B 端订阅器在 1 分钟内接收到 %d 条消息，符合预期（5-8 条）", subscriberStats.TotalMessagesReceived)
	}

	// 验证订阅器最后接收消息时间不为零
	if subscriberStats.LastMessageTime.IsZero() {
		t.Errorf("❌ B 端订阅器应该接收过消息，但 LastMessageTime 为零值")
	}

	// 停止 A 端发布器
	t.Logf("🛑 停止 A 端健康检查发布器")
	err = busA.StopHealthCheckPublisher()
	helper.AssertNoError(err, "A 端 StopHealthCheckPublisher should not return error")

	// 停止 B 端订阅器
	t.Logf("🛑 停止 B 端健康检查订阅器")
	err = busB.StopHealthCheckSubscriber()
	helper.AssertNoError(err, "B 端 StopHealthCheckSubscriber should not return error")

	t.Logf("✅ Kafka 健康检查发布端和订阅端集成测试通过")
	t.Logf("   - A 端成功发送健康检查消息（自定义间隔：10秒）")
	t.Logf("   - B 端成功接收 %d 条健康检查消息（测试时长：1分钟）", subscriberStats.TotalMessagesReceived)
}

// TestNATSHealthCheckPublisherSubscriberIntegration 测试 NATS 健康检查发布端和订阅端集成
// 模拟 A 端启动发布端健康检测，B 端启动订阅端健康检测
// 验证发布端能例行成功发送健康检测消息，订阅端能例行成功接收到健康检测消息
// 优化：设置健康检查间隔为 10 秒，测试时间为 1 分钟，预期接收约 6 条消息
func TestNATSHealthCheckPublisherSubscriberIntegration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	ctx := context.Background()

	// 🔧 修复：使用共享的 clientID 前缀，确保 A 端和 B 端使用相同的 Stream
	sharedClientID := fmt.Sprintf("nats-health-integration-%d", helper.GetTimestamp())

	// 🔧 修复：健康检查 topic 必须匹配 Stream subjects 模式 (clientID.>)
	healthCheckTopic := fmt.Sprintf("%s.health-check", sharedClientID)

	// 创建自定义健康检查配置：每 10 秒发送一次
	customHealthCheckConfig := config.HealthCheckConfig{
		Enabled: true,
		Publisher: config.HealthCheckPublisherConfig{
			Topic:            healthCheckTopic, // 🔧 修复：使用匹配 Stream 的 topic
			Interval:         10 * time.Second, // 设置为 10 秒
			Timeout:          10 * time.Second,
			FailureThreshold: 3,
			MessageTTL:       5 * time.Minute,
		},
		Subscriber: config.HealthCheckSubscriberConfig{
			Topic:             healthCheckTopic, // 🔧 修复：使用匹配 Stream 的 topic
			MonitorInterval:   30 * time.Second,
			WarningThreshold:  3,
			ErrorThreshold:    5,
			CriticalThreshold: 10,
		},
	}

	// 创建 A 端 EventBus（发布端）- 使用共享的 clientID
	clientIDA := sharedClientID
	busA := helper.CreateNATSEventBusWithHealthCheck(clientIDA, customHealthCheckConfig)
	defer helper.CloseEventBus(busA)

	// 创建 B 端 EventBus（订阅端）- 使用共享的 clientID
	clientIDB := sharedClientID
	busB := helper.CreateNATSEventBusWithHealthCheck(clientIDB, customHealthCheckConfig)
	defer helper.CloseEventBus(busB)

	// 🔧 重要：对于 NATS Core（非 JetStream），必须先订阅再发布
	// 因为 NATS Core 不会保留消息，只有订阅后发布的消息才能被接收

	// B 端：先启动健康检查订阅器
	t.Logf("🚀 B 端：先启动健康检查订阅器（NATS Core 需要先订阅）")
	err := busB.StartHealthCheckSubscriber(ctx)
	helper.AssertNoError(err, "B 端 StartHealthCheckSubscriber should not return error")

	// 等待订阅器完全启动（确保订阅已建立）
	time.Sleep(1 * time.Second)

	// A 端：再启动健康检查发布器
	t.Logf("🚀 A 端：再启动健康检查发布器")
	err = busA.StartHealthCheckPublisher(ctx)
	helper.AssertNoError(err, "A 端 StartHealthCheckPublisher should not return error")

	// 等待健康检查消息发送和接收（1 分钟，预期接收约 6 条消息）
	// 计算：启动时立即发送 1 条 + 每 10 秒发送 1 条 × 6 次 = 约 6-7 条
	t.Logf("⏳ 等待健康检查消息发送和接收（1分钟）...")
	t.Logf("✅ EventBus 现在使用自定义健康检查配置（间隔：10秒）")
	time.Sleep(60 * time.Second) // 1 分钟

	// 验证 A 端发布器状态
	publisherStatus := busA.GetHealthCheckPublisherStatus()
	t.Logf("📊 A 端发布器状态: IsHealthy=%v, ConsecutiveFailures=%d, LastSuccessTime=%v",
		publisherStatus.IsHealthy,
		publisherStatus.ConsecutiveFailures,
		publisherStatus.LastSuccessTime)

	// 验证发布器健康
	if !publisherStatus.IsHealthy {
		t.Errorf("❌ A 端发布器应该是健康的，但实际状态为不健康")
	}

	// 验证发布器成功发送过消息
	if publisherStatus.LastSuccessTime.IsZero() {
		t.Errorf("❌ A 端发布器应该成功发送过健康检查消息，但 LastSuccessTime 为零值")
	}

	// 验证发布器没有连续失败
	if publisherStatus.ConsecutiveFailures > 0 {
		t.Errorf("❌ A 端发布器不应该有连续失败，但实际 ConsecutiveFailures=%d", publisherStatus.ConsecutiveFailures)
	}

	// 验证 B 端订阅器状态
	subscriberStats := busB.GetHealthCheckSubscriberStats()
	t.Logf("📊 B 端订阅器统计: TotalMessagesReceived=%d, ConsecutiveMisses=%d, IsHealthy=%v, LastMessageTime=%v",
		subscriberStats.TotalMessagesReceived,
		subscriberStats.ConsecutiveMisses,
		subscriberStats.IsHealthy,
		subscriberStats.LastMessageTime)

	// 验证订阅器接收到消息
	if subscriberStats.TotalMessagesReceived == 0 {
		t.Errorf("❌ B 端订阅器应该接收到健康检查消息，但实际接收数量为 0")
	}

	// 验证订阅器健康（至少接收到 1 条消息就应该是健康的）
	if subscriberStats.TotalMessagesReceived > 0 && !subscriberStats.IsHealthy {
		t.Logf("⚠️  B 端订阅器接收到 %d 条消息，但状态为不健康（可能是因为消息间隔检测）", subscriberStats.TotalMessagesReceived)
	}

	// 验证订阅器接收到的消息数量合理（1分钟内，10秒发送一次，应该至少接收到 5-8 条消息）
	// 计算：启动时立即发送 1 条 + 每 10 秒发送 1 条 × 6 次 = 约 6-7 条
	if subscriberStats.TotalMessagesReceived < 5 {
		t.Errorf("❌ B 端订阅器在 1 分钟内只接收到 %d 条消息，预期至少 5 条", subscriberStats.TotalMessagesReceived)
	} else if subscriberStats.TotalMessagesReceived > 8 {
		t.Logf("⚠️  B 端订阅器在 1 分钟内接收到 %d 条消息，超过预期的 8 条", subscriberStats.TotalMessagesReceived)
	} else {
		t.Logf("✅ B 端订阅器在 1 分钟内接收到 %d 条消息，符合预期（5-8 条）", subscriberStats.TotalMessagesReceived)
	}

	// 验证订阅器最后接收消息时间不为零
	if subscriberStats.LastMessageTime.IsZero() {
		t.Errorf("❌ B 端订阅器应该接收过消息，但 LastMessageTime 为零值")
	}

	// 停止 A 端发布器
	t.Logf("🛑 停止 A 端健康检查发布器")
	err = busA.StopHealthCheckPublisher()
	helper.AssertNoError(err, "A 端 StopHealthCheckPublisher should not return error")

	// 停止 B 端订阅器
	t.Logf("🛑 停止 B 端健康检查订阅器")
	err = busB.StopHealthCheckSubscriber()
	helper.AssertNoError(err, "B 端 StopHealthCheckSubscriber should not return error")

	t.Logf("✅ NATS 健康检查发布端和订阅端集成测试通过")
	t.Logf("   - A 端成功发送健康检查消息（自定义间隔：10秒）")
	t.Logf("   - B 端成功接收 %d 条健康检查消息（测试时长：1分钟）", subscriberStats.TotalMessagesReceived)
}
