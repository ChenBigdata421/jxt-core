package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventBusManager_PerformHealthCheck 测试执行健康检查
func TestEventBusManager_PerformHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	err = manager.performHealthCheck(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_PerformHealthCheck_Closed 测试关闭状态的健康检查
func TestEventBusManager_PerformHealthCheck_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	bus.Close()

	ctx := context.Background()
	err = manager.performHealthCheck(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed")
}

// TestEventBusManager_PerformFullHealthCheck 测试完整健康检查
func TestEventBusManager_PerformFullHealthCheck(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	status, err := manager.performFullHealthCheck(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "healthy", status.Overall)
	assert.NotZero(t, status.Timestamp)
	assert.NotZero(t, status.CheckDuration)
}

// TestEventBusManager_PerformFullHealthCheck_Closed 测试关闭状态的完整健康检查
func TestEventBusManager_PerformFullHealthCheck_Closed(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	bus.Close()

	ctx := context.Background()
	status, err := manager.performFullHealthCheck(ctx)
	assert.Error(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "unhealthy", status.Overall)
	assert.Equal(t, "closed", status.Infrastructure.EventBus.ConnectionStatus)
}

// TestEventBusManager_PerformFullHealthCheck_WithBusinessChecker 测试带业务健康检查器的完整检查
func TestEventBusManager_PerformFullHealthCheck_WithBusinessChecker(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 注册业务健康检查器
	checker := &mockBusinessHealthChecker{}
	manager.RegisterBusinessHealthCheck(checker)

	ctx := context.Background()
	status, err := manager.performFullHealthCheck(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "healthy", status.Overall)
	assert.NotNil(t, status.Business)
}

// TestEventBusManager_PerformFullHealthCheck_BusinessCheckerFails 测试业务健康检查失败
func TestEventBusManager_PerformFullHealthCheck_BusinessCheckerFails(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 注册会失败的业务健康检查器
	checker := &mockFailingBusinessHealthChecker{}
	manager.RegisterBusinessHealthCheck(checker)

	ctx := context.Background()
	status, err := manager.performFullHealthCheck(ctx)
	assert.Error(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "unhealthy", status.Overall)
}

// TestEventBusManager_CheckInfrastructureHealth 测试基础设施健康检查
func TestEventBusManager_CheckInfrastructureHealth(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	infraHealth, err := manager.checkInfrastructureHealth(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "connected", infraHealth.EventBus.ConnectionStatus)
}

// TestEventBusManager_CheckConnection 测试连接检查
func TestEventBusManager_CheckConnection(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	err = manager.checkConnection(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_CheckConnection_NoPublisher 测试无发布器的连接检查
func TestEventBusManager_CheckConnection_NoPublisher(t *testing.T) {
	manager := &eventBusManager{
		publisher:  nil,
		subscriber: nil,
	}

	ctx := context.Background()
	err := manager.checkConnection(ctx)
	assert.NoError(t, err) // 没有发布器和订阅器时应该返回 nil
}

// TestEventBusManager_CheckMessageTransport 测试消息传输检查
func TestEventBusManager_CheckMessageTransport(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	err = manager.checkMessageTransport(ctx)
	assert.NoError(t, err)
}

// TestEventBusManager_CheckMessageTransport_NoPublisher 测试无发布器的消息传输检查
func TestEventBusManager_CheckMessageTransport_NoPublisher(t *testing.T) {
	manager := &eventBusManager{
		publisher: nil,
	}

	ctx := context.Background()
	err := manager.checkMessageTransport(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publisher not initialized")
}

// TestEventBusManager_CheckMessageTransport_WithTimeout 测试带超时的消息传输检查
func TestEventBusManager_CheckMessageTransport_WithTimeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 创建一个会很快超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = manager.checkMessageTransport(ctx)
	// Memory EventBus 应该能快速完成，不会超时
	assert.NoError(t, err)
}

// TestEventBusManager_PerformEndToEndTest 测试端到端测试
func TestEventBusManager_PerformEndToEndTest(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	testTopic := "test-e2e-topic"
	healthMsg := "health-check-test"

	err = manager.performEndToEndTest(ctx, testTopic, healthMsg)
	assert.NoError(t, err)
}

// TestEventBusManager_PerformEndToEndTest_Timeout 测试端到端测试超时
func TestEventBusManager_PerformEndToEndTest_Timeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 创建一个会立即超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // 确保上下文已超时

	testTopic := "test-e2e-topic"
	healthMsg := "health-check-test"

	err = manager.performEndToEndTest(ctx, testTopic, healthMsg)
	// 由于上下文已超时，应该返回错误
	assert.Error(t, err)
}

// mockFailingBusinessHealthChecker 模拟失败的业务健康检查器
type mockFailingBusinessHealthChecker struct{}

func (m *mockFailingBusinessHealthChecker) CheckBusinessHealth(ctx context.Context) error {
	return assert.AnError
}

func (m *mockFailingBusinessHealthChecker) GetBusinessMetrics() interface{} {
	return map[string]interface{}{"status": "unhealthy"}
}

func (m *mockFailingBusinessHealthChecker) GetBusinessConfig() interface{} {
	return map[string]interface{}{"config": "test"}
}
