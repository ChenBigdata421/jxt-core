package function_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

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
