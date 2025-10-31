package reliability_regression_tests

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// TestHelper 测试辅助工具
type TestHelper struct {
	t         *testing.T
	timestamp int64
	mu        sync.Mutex
	cleanups  []func()
}

// NewTestHelper 创建测试辅助工具
func NewTestHelper(t *testing.T) *TestHelper {
	// 初始化 logger
	logger.Setup()

	return &TestHelper{
		t:         t,
		timestamp: time.Now().UnixMilli(),
		cleanups:  make([]func(), 0),
	}
}

// GetTimestamp 获取时间戳
func (h *TestHelper) GetTimestamp() int64 {
	return h.timestamp
}

// AddCleanup 添加清理函数
func (h *TestHelper) AddCleanup(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanups = append(h.cleanups, fn)
}

// Cleanup 执行所有清理函数
func (h *TestHelper) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 逆序执行清理函数
	for i := len(h.cleanups) - 1; i >= 0; i-- {
		h.cleanups[i]()
	}
}

// AssertNoError 断言无错误
func (h *TestHelper) AssertNoError(err error, message string) {
	if err != nil {
		h.t.Errorf("❌ %s: %v", message, err)
	}
}

// AssertError 断言有错误
func (h *TestHelper) AssertError(err error, message string) {
	if err == nil {
		h.t.Errorf("❌ %s: expected error but got nil", message)
	}
}

// AssertEqual 断言相等
func (h *TestHelper) AssertEqual(expected, actual interface{}, message string) {
	if expected != actual {
		h.t.Errorf("❌ %s: expected %v, got %v", message, expected, actual)
	}
}

// AssertTrue 断言为真
func (h *TestHelper) AssertTrue(condition bool, message string) {
	if !condition {
		h.t.Errorf("❌ %s: expected true, got false", message)
	}
}

// AssertGreaterThan 断言大于
func (h *TestHelper) AssertGreaterThan(actual, threshold int64, message string) {
	if actual <= threshold {
		h.t.Errorf("❌ %s: expected > %d, got %d", message, threshold, actual)
	}
}

// AssertLessThan 断言小于
func (h *TestHelper) AssertLessThan(actual, threshold int64, message string) {
	if actual >= threshold {
		h.t.Errorf("❌ %s: expected < %d, got %d", message, threshold, actual)
	}
}

// WaitForCondition 等待条件满足
func (h *TestHelper) WaitForCondition(condition func() bool, timeout time.Duration, message string) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if condition() {
			return true
		}

		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				h.t.Logf("⚠️ %s: timeout after %v", message, timeout)
				return false
			}
		}
	}
}

// WaitForMessages 等待消息数量达到预期
func (h *TestHelper) WaitForMessages(received *int64, expected int64, timeout time.Duration) bool {
	return h.WaitForCondition(func() bool {
		return atomic.LoadInt64(received) >= expected
	}, timeout, fmt.Sprintf("Waiting for %d messages", expected))
}

// EventCollector 事件收集器
type EventCollector struct {
	mu                sync.RWMutex
	restartEvents     []interface{}
	deadLetterEvents  []interface{}
	startedEvents     []interface{}
	stoppedEvents     []interface{}
	initializedEvents []interface{}
}

// NewEventCollector 创建事件收集器
func NewEventCollector() *EventCollector {
	return &EventCollector{
		restartEvents:     make([]interface{}, 0),
		deadLetterEvents:  make([]interface{}, 0),
		startedEvents:     make([]interface{}, 0),
		stoppedEvents:     make([]interface{}, 0),
		initializedEvents: make([]interface{}, 0),
	}
}

// AddRestartEvent 添加重启事件
func (ec *EventCollector) AddRestartEvent(event interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.restartEvents = append(ec.restartEvents, event)
}

// AddDeadLetterEvent 添加 DeadLetter 事件
func (ec *EventCollector) AddDeadLetterEvent(event interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.deadLetterEvents = append(ec.deadLetterEvents, event)
}

// AddStartedEvent 添加启动事件
func (ec *EventCollector) AddStartedEvent(event interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.startedEvents = append(ec.startedEvents, event)
}

// AddStoppedEvent 添加停止事件
func (ec *EventCollector) AddStoppedEvent(event interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.stoppedEvents = append(ec.stoppedEvents, event)
}

// AddInitializedEvent 添加初始化事件
func (ec *EventCollector) AddInitializedEvent(event interface{}) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.initializedEvents = append(ec.initializedEvents, event)
}

// GetRestartCount 获取重启次数
func (ec *EventCollector) GetRestartCount() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.restartEvents)
}

// GetDeadLetterCount 获取 DeadLetter 次数
func (ec *EventCollector) GetDeadLetterCount() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.deadLetterEvents)
}

// GetStartedCount 获取启动次数
func (ec *EventCollector) GetStartedCount() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.startedEvents)
}

// GetStoppedCount 获取停止次数
func (ec *EventCollector) GetStoppedCount() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.stoppedEvents)
}

// GetInitializedCount 获取初始化次数
func (ec *EventCollector) GetInitializedCount() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.initializedEvents)
}

// Reset 重置所有计数器
func (ec *EventCollector) Reset() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.restartEvents = make([]interface{}, 0)
	ec.deadLetterEvents = make([]interface{}, 0)
	ec.startedEvents = make([]interface{}, 0)
	ec.stoppedEvents = make([]interface{}, 0)
	ec.initializedEvents = make([]interface{}, 0)
}

// CreateMemoryEventBus 创建 Memory EventBus
func (h *TestHelper) CreateMemoryEventBus() eventbus.EventBus {
	bus := eventbus.NewMemoryEventBus()
	h.AddCleanup(func() {
		if err := bus.Close(); err != nil {
			h.t.Logf("⚠️ Failed to close Memory EventBus: %v", err)
		}
	})
	return bus
}

// CreateKafkaEventBus 创建 Kafka EventBus
func (h *TestHelper) CreateKafkaEventBus(clientID string) eventbus.EventBus {
	cfg := &eventbus.KafkaConfig{
		Brokers:  []string{"localhost:9092"},
		ClientID: clientID,
	}

	bus, err := eventbus.NewKafkaEventBus(cfg)
	h.AssertNoError(err, "NewKafkaEventBus should not return error")

	h.AddCleanup(func() {
		if err := bus.Close(); err != nil {
			h.t.Logf("⚠️ Failed to close Kafka EventBus: %v", err)
		}
	})

	return bus
}

// CloseEventBus 关闭 EventBus
func (h *TestHelper) CloseEventBus(bus eventbus.EventBus) {
	if err := bus.Close(); err != nil {
		h.t.Logf("⚠️ Failed to close EventBus: %v", err)
	}
}

// Sleep 睡眠
func (h *TestHelper) Sleep(duration time.Duration) {
	time.Sleep(duration)
}

// Logf 日志输出
func (h *TestHelper) Logf(format string, args ...interface{}) {
	h.t.Logf(format, args...)
}
