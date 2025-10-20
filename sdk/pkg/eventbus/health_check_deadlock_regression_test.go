package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHealthCheck_NoDeadlock 测试健康检查不会产生死锁
func TestHealthCheck_NoDeadlock(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 并发执行健康检查和其他操作，确保不会死锁
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 启动多个goroutine执行健康检查
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				status, err := manager.performFullHealthCheck(ctx)
				if err == nil {
					assert.NotNil(t, status)
					assert.NotEmpty(t, status.Overall)
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// 启动多个goroutine执行其他操作
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				// 发布消息
				err := bus.Publish(ctx, "test-topic", []byte("test"))
				assert.NoError(t, err)

				// 获取指标
				metrics := bus.GetMetrics()
				assert.NotNil(t, metrics)

				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	// 等待所有操作完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All operations completed without deadlock")
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out - possible deadlock detected")
	}
}

// TestHealthCheck_ConcurrentAccess 测试健康检查的并发访问
func TestHealthCheck_ConcurrentAccess(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	var wg sync.WaitGroup
	var healthCheckCount int64
	var operationCount int64

	// 并发健康检查
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, err := manager.performFullHealthCheck(ctx)
				if err == nil {
					healthCheckCount++
				}
			}
		}()
	}

	// 并发业务操作
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				err := bus.Publish(ctx, "concurrent-topic", []byte("test"))
				if err == nil {
					operationCount++
				}
			}
		}()
	}

	wg.Wait()

	t.Logf("Health checks completed: %d", healthCheckCount)
	t.Logf("Operations completed: %d", operationCount)

	// 验证操作都能正常完成
	assert.Greater(t, healthCheckCount, int64(0))
	assert.Greater(t, operationCount, int64(0))
}

// TestHealthCheck_AfterClose 测试关闭后的健康检查
func TestHealthCheck_AfterClose(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 关闭前的健康检查应该成功
	status, err := manager.performFullHealthCheck(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", status.Overall)

	// 关闭EventBus
	bus.Close()

	// 关闭后的健康检查应该返回错误
	status, err = manager.performFullHealthCheck(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eventbus is closed")
	assert.Equal(t, "unhealthy", status.Overall)
}

// TestHealthCheck_WithBusinessChecker 测试带业务健康检查器的情况
func TestHealthCheck_WithBusinessChecker(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 创建模拟的业务健康检查器
	mockChecker := &mockBusinessHealthChecker{}

	// 注册业务健康检查器
	manager.RegisterBusinessHealthCheck(mockChecker)

	// 执行健康检查
	status, err := manager.performFullHealthCheck(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", status.Overall)
	assert.NotNil(t, status.Business)

	// 设置业务检查器为不健康（这里我们创建一个新的会失败的检查器）
	failingChecker := &failingBusinessHealthChecker{}
	manager.RegisterBusinessHealthCheck(failingChecker)

	// 再次执行健康检查
	status, err = manager.performFullHealthCheck(ctx)
	assert.Error(t, err)
	assert.Equal(t, "unhealthy", status.Overall)
}

// TestHealthCheck_Timeout 测试健康检查超时
func TestHealthCheck_Timeout(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)

	// 创建会超时的context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 创建会延迟的业务健康检查器
	slowChecker := &slowBusinessHealthChecker{
		delay: 200 * time.Millisecond,
	}

	manager.RegisterBusinessHealthCheck(slowChecker)

	// 执行健康检查（应该超时）
	start := time.Now()
	_, err = manager.performFullHealthCheck(ctx)
	duration := time.Since(start)

	// 验证在合理时间内返回（不会无限等待）
	assert.True(t, duration < 500*time.Millisecond, "Health check should timeout quickly")

	// 可能会因为context超时而返回错误
	t.Logf("Health check completed in %v with error: %v", duration, err)
}

// mockBusinessHealthChecker 模拟业务健康检查器
type mockBusinessHealthChecker struct{}

func (m *mockBusinessHealthChecker) CheckBusinessHealth(ctx context.Context) error {
	return nil
}

func (m *mockBusinessHealthChecker) GetBusinessMetrics() interface{} {
	return map[string]interface{}{"status": "healthy"}
}

func (m *mockBusinessHealthChecker) GetBusinessConfig() interface{} {
	return map[string]interface{}{"config": "test"}
}

// slowBusinessHealthChecker 慢速业务健康检查器
type slowBusinessHealthChecker struct {
	delay time.Duration
}

func (s *slowBusinessHealthChecker) CheckBusinessHealth(ctx context.Context) error {
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *slowBusinessHealthChecker) GetBusinessMetrics() interface{} {
	return map[string]interface{}{"slow": true}
}

func (s *slowBusinessHealthChecker) GetBusinessConfig() interface{} {
	return map[string]interface{}{"delay": s.delay.String()}
}

// failingBusinessHealthChecker 会失败的业务健康检查器
type failingBusinessHealthChecker struct{}

func (f *failingBusinessHealthChecker) CheckBusinessHealth(ctx context.Context) error {
	return assert.AnError
}

func (f *failingBusinessHealthChecker) GetBusinessMetrics() interface{} {
	return map[string]interface{}{"failing": true}
}

func (f *failingBusinessHealthChecker) GetBusinessConfig() interface{} {
	return map[string]interface{}{"type": "failing"}
}

// TestHealthCheck_MetricsUpdate 测试健康检查指标更新
func TestHealthCheck_MetricsUpdate(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 获取初始指标
	initialMetrics := bus.GetMetrics()
	initialTime := initialMetrics.LastHealthCheck

	// 等待一小段时间确保时间戳不同
	time.Sleep(10 * time.Millisecond)

	// 执行健康检查
	_, err = manager.performFullHealthCheck(ctx)
	assert.NoError(t, err)

	// 获取更新后的指标
	updatedMetrics := bus.GetMetrics()

	// 验证指标被更新
	assert.True(t, updatedMetrics.LastHealthCheck.After(initialTime))
	assert.Equal(t, "healthy", updatedMetrics.HealthCheckStatus)
}

// TestHealthCheck_StateConsistency 测试健康检查状态一致性
func TestHealthCheck_StateConsistency(t *testing.T) {
	cfg := &EventBusConfig{Type: "memory"}
	bus, err := NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	manager := bus.(*eventBusManager)
	ctx := context.Background()

	// 并发执行健康检查和状态读取
	var wg sync.WaitGroup
	var inconsistencyCount int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// 执行健康检查
				status, err := manager.performFullHealthCheck(ctx)
				if err == nil {
					// 立即读取指标
					metrics := bus.GetMetrics()

					// 检查状态一致性
					if status.Overall == "healthy" && metrics.HealthCheckStatus != "healthy" {
						inconsistencyCount++
					}
				}
			}
		}()
	}

	wg.Wait()

	// 验证状态一致性
	assert.Equal(t, int64(0), inconsistencyCount, "Health check state should be consistent")
}
