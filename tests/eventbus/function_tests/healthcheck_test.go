package function_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

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
	bus := helper.CreateNATSEventBus(clientID)
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
	bus := helper.CreateNATSEventBus(clientID)
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

	// 注册健康检查发布器回调
	callbackCalled := false
	err := bus.RegisterHealthCheckPublisherCallback(func(ctx context.Context, result eventbus.HealthCheckResult) error {
		callbackCalled = true
		t.Logf("📞 Health check publisher callback called: %+v", result)
		return nil
	})
	helper.AssertNoError(err, "RegisterHealthCheckPublisherCallback should not return error")

	ctx := context.Background()

	// 启动健康检查发布器
	err = bus.StartHealthCheckPublisher(ctx)
	helper.AssertNoError(err, "StartHealthCheckPublisher should not return error")

	// 等待回调
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

	// 注册健康检查发布器回调
	callbackCalled := false
	err := bus.RegisterHealthCheckPublisherCallback(func(ctx context.Context, result eventbus.HealthCheckResult) error {
		callbackCalled = true
		t.Logf("📞 Health check publisher callback called: %+v", result)
		return nil
	})
	helper.AssertNoError(err, "RegisterHealthCheckPublisherCallback should not return error")

	ctx := context.Background()

	// 启动健康检查发布器
	err = bus.StartHealthCheckPublisher(ctx)
	helper.AssertNoError(err, "StartHealthCheckPublisher should not return error")

	// 等待回调
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

	// 注册健康检查订阅器回调
	callbackCalled := false
	err := bus.RegisterHealthCheckSubscriberCallback(func(ctx context.Context, alert eventbus.HealthCheckAlert) error {
		callbackCalled = true
		t.Logf("📞 Health check subscriber callback called: %+v", alert)
		return nil
	})
	helper.AssertNoError(err, "RegisterHealthCheckSubscriberCallback should not return error")

	ctx := context.Background()

	// 启动健康检查订阅器
	err = bus.StartHealthCheckSubscriber(ctx)
	helper.AssertNoError(err, "StartHealthCheckSubscriber should not return error")

	// 等待回调
	time.Sleep(3 * time.Second)

	// 停止健康检查订阅器
	err = bus.StopHealthCheckSubscriber()
	helper.AssertNoError(err, "StopHealthCheckSubscriber should not return error")

	t.Logf("✅ Kafka health check subscriber callback test passed (callback called: %v)", callbackCalled)
}
