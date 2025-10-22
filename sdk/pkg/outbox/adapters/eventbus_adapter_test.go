package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// MockEventBus 用于测试的 Mock EventBus
type MockEventBus struct {
	publishedEnvelopes []*eventbus.Envelope
	resultChan         chan *eventbus.PublishResult
	shouldFail         bool
	failError          error
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		publishedEnvelopes: make([]*eventbus.Envelope, 0),
		resultChan:         make(chan *eventbus.PublishResult, 100),
		shouldFail:         false,
	}
}

func (m *MockEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *eventbus.Envelope) error {
	if m.shouldFail {
		// 发送失败 ACK
		go func() {
			m.resultChan <- &eventbus.PublishResult{
				EventID:     envelope.EventID,
				Topic:       topic,
				Success:     false,
				Error:       m.failError,
				Timestamp:   time.Now(),
				AggregateID: envelope.AggregateID,
				EventType:   envelope.EventType,
			}
		}()

		if m.failError != nil {
			return m.failError
		}
		return errors.New("mock publish error")
	}

	m.publishedEnvelopes = append(m.publishedEnvelopes, envelope)

	// 发送成功 ACK
	go func() {
		m.resultChan <- &eventbus.PublishResult{
			EventID:     envelope.EventID,
			Topic:       topic,
			Success:     true,
			Timestamp:   time.Now(),
			AggregateID: envelope.AggregateID,
			EventType:   envelope.EventType,
		}
	}()

	return nil
}

func (m *MockEventBus) GetPublishResultChannel() <-chan *eventbus.PublishResult {
	return m.resultChan
}

// 实现 EventBus 接口的其他方法（空实现）
func (m *MockEventBus) Publish(ctx context.Context, topic string, message []byte) error {
	return nil
}
func (m *MockEventBus) Subscribe(ctx context.Context, topic string, handler eventbus.MessageHandler) error {
	return nil
}
func (m *MockEventBus) Close() error { return nil }
func (m *MockEventBus) RegisterReconnectCallback(callback eventbus.ReconnectCallback) error {
	return nil
}
func (m *MockEventBus) Start(ctx context.Context) error { return nil }
func (m *MockEventBus) Stop() error                     { return nil }
func (m *MockEventBus) PublishWithOptions(ctx context.Context, topic string, message []byte, opts eventbus.PublishOptions) error {
	return nil
}
func (m *MockEventBus) SetMessageFormatter(formatter eventbus.MessageFormatter) error { return nil }
func (m *MockEventBus) RegisterPublishCallback(callback eventbus.PublishCallback) error {
	return nil
}
func (m *MockEventBus) SubscribeWithOptions(ctx context.Context, topic string, handler eventbus.MessageHandler, opts eventbus.SubscribeOptions) error {
	return nil
}
func (m *MockEventBus) RegisterSubscriberBacklogCallback(callback eventbus.BacklogStateCallback) error {
	return nil
}
func (m *MockEventBus) StartSubscriberBacklogMonitoring(ctx context.Context) error { return nil }
func (m *MockEventBus) StopSubscriberBacklogMonitoring() error                     { return nil }
func (m *MockEventBus) RegisterPublisherBacklogCallback(callback eventbus.PublisherBacklogCallback) error {
	return nil
}
func (m *MockEventBus) StartPublisherBacklogMonitoring(ctx context.Context) error { return nil }
func (m *MockEventBus) StopPublisherBacklogMonitoring() error                     { return nil }
func (m *MockEventBus) StartAllBacklogMonitoring(ctx context.Context) error       { return nil }
func (m *MockEventBus) StopAllBacklogMonitoring() error                           { return nil }
func (m *MockEventBus) SetMessageRouter(router eventbus.MessageRouter) error      { return nil }
func (m *MockEventBus) SetErrorHandler(handler eventbus.ErrorHandler) error       { return nil }
func (m *MockEventBus) RegisterSubscriptionCallback(callback eventbus.SubscriptionCallback) error {
	return nil
}
func (m *MockEventBus) StartHealthCheckPublisher(ctx context.Context) error { return nil }
func (m *MockEventBus) StopHealthCheckPublisher() error                     { return nil }
func (m *MockEventBus) GetHealthCheckPublisherStatus() eventbus.HealthCheckStatus {
	return eventbus.HealthCheckStatus{}
}
func (m *MockEventBus) RegisterHealthCheckPublisherCallback(callback eventbus.HealthCheckCallback) error {
	return nil
}
func (m *MockEventBus) StartHealthCheckSubscriber(ctx context.Context) error { return nil }
func (m *MockEventBus) StopHealthCheckSubscriber() error                     { return nil }
func (m *MockEventBus) GetHealthCheckSubscriberStats() eventbus.HealthCheckSubscriberStats {
	return eventbus.HealthCheckSubscriberStats{}
}
func (m *MockEventBus) RegisterHealthCheckSubscriberCallback(callback eventbus.HealthCheckAlertCallback) error {
	return nil
}
func (m *MockEventBus) StartAllHealthCheck(ctx context.Context) error { return nil }
func (m *MockEventBus) StopAllHealthCheck() error                     { return nil }
func (m *MockEventBus) StartHealthCheck(ctx context.Context) error    { return nil }
func (m *MockEventBus) StopHealthCheck() error                        { return nil }
func (m *MockEventBus) GetHealthStatus() eventbus.HealthCheckStatus {
	return eventbus.HealthCheckStatus{}
}
func (m *MockEventBus) GetConnectionState() eventbus.ConnectionState {
	return eventbus.ConnectionState{}
}
func (m *MockEventBus) GetMetrics() eventbus.Metrics {
	return eventbus.Metrics{}
}
func (m *MockEventBus) ConfigureTopic(ctx context.Context, topic string, options eventbus.TopicOptions) error {
	return nil
}
func (m *MockEventBus) SetTopicPersistence(ctx context.Context, topic string, persistent bool) error {
	return nil
}
func (m *MockEventBus) GetTopicConfig(topic string) (eventbus.TopicOptions, error) {
	return eventbus.TopicOptions{}, nil
}
func (m *MockEventBus) ListConfiguredTopics() []string {
	return nil
}
func (m *MockEventBus) RemoveTopicConfig(topic string) error {
	return nil
}
func (m *MockEventBus) SetTopicConfigStrategy(strategy eventbus.TopicConfigStrategy) {}
func (m *MockEventBus) GetTopicConfigStrategy() eventbus.TopicConfigStrategy {
	return eventbus.StrategySkip
}
func (m *MockEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler eventbus.EnvelopeHandler) error {
	return nil
}

// ========== 测试用例 ==========

func TestNewEventBusAdapter(t *testing.T) {
	mockEventBus := NewMockEventBus()
	adapter := NewEventBusAdapter(mockEventBus)

	if adapter == nil {
		t.Fatal("Expected adapter to be created")
	}

	if !adapter.IsStarted() {
		t.Error("Expected adapter to be started")
	}

	// 清理
	adapter.Close()
}

func TestEventBusAdapter_PublishEnvelope(t *testing.T) {
	mockEventBus := NewMockEventBus()
	adapter := NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	// 创建 Outbox Envelope
	envelope := &outbox.Envelope{
		EventID:       "test-event-1",
		AggregateID:   "test-aggregate-1",
		EventType:     "TestEvent",
		EventVersion:  1,
		Payload:       []byte(`{"test":"data"}`),
		Timestamp:     time.Now(),
		TraceID:       "trace-123",
		CorrelationID: "corr-456",
	}

	// 发布
	err := adapter.PublishEnvelope(context.Background(), "test-topic", envelope)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 验证 EventBus 收到了正确的 Envelope
	if len(mockEventBus.publishedEnvelopes) != 1 {
		t.Fatalf("Expected 1 published envelope, got %d", len(mockEventBus.publishedEnvelopes))
	}

	published := mockEventBus.publishedEnvelopes[0]
	if published.EventID != envelope.EventID {
		t.Errorf("Expected EventID '%s', got '%s'", envelope.EventID, published.EventID)
	}
	if published.AggregateID != envelope.AggregateID {
		t.Errorf("Expected AggregateID '%s', got '%s'", envelope.AggregateID, published.AggregateID)
	}
	if published.EventType != envelope.EventType {
		t.Errorf("Expected EventType '%s', got '%s'", envelope.EventType, published.EventType)
	}
	if published.TraceID != envelope.TraceID {
		t.Errorf("Expected TraceID '%s', got '%s'", envelope.TraceID, published.TraceID)
	}
}

func TestEventBusAdapter_GetPublishResultChannel_Success(t *testing.T) {
	mockEventBus := NewMockEventBus()
	adapter := NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	// 发布事件
	envelope := &outbox.Envelope{
		EventID:      "test-event-1",
		AggregateID:  "test-aggregate-1",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      []byte(`{"test":"data"}`),
		Timestamp:    time.Now(),
	}

	err := adapter.PublishEnvelope(context.Background(), "test-topic", envelope)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// 获取结果通道
	resultChan := adapter.GetPublishResultChannel()

	// 应该收到成功的 ACK
	select {
	case result := <-resultChan:
		if !result.Success {
			t.Error("Expected success ACK")
		}
		if result.EventID != "test-event-1" {
			t.Errorf("Expected EventID 'test-event-1', got '%s'", result.EventID)
		}
		if result.Topic != "test-topic" {
			t.Errorf("Expected Topic 'test-topic', got '%s'", result.Topic)
		}
		if result.AggregateID != "test-aggregate-1" {
			t.Errorf("Expected AggregateID 'test-aggregate-1', got '%s'", result.AggregateID)
		}
		if result.EventType != "TestEvent" {
			t.Errorf("Expected EventType 'TestEvent', got '%s'", result.EventType)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Expected to receive ACK result")
	}
}

func TestEventBusAdapter_GetPublishResultChannel_Failure(t *testing.T) {
	mockEventBus := NewMockEventBus()
	mockEventBus.shouldFail = true
	mockEventBus.failError = errors.New("test error")

	adapter := NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	// 发布事件
	envelope := &outbox.Envelope{
		EventID:      "test-event-1",
		AggregateID:  "test-aggregate-1",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      []byte(`{"test":"data"}`),
		Timestamp:    time.Now(),
	}

	err := adapter.PublishEnvelope(context.Background(), "test-topic", envelope)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// 获取结果通道
	resultChan := adapter.GetPublishResultChannel()

	// 应该收到失败的 ACK
	select {
	case result := <-resultChan:
		if result.Success {
			t.Error("Expected failure ACK")
		}
		if result.Error == nil {
			t.Error("Expected error in ACK result")
		}
		if result.EventID != "test-event-1" {
			t.Errorf("Expected EventID 'test-event-1', got '%s'", result.EventID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Expected to receive ACK result")
	}
}

func TestEventBusAdapter_Close(t *testing.T) {
	mockEventBus := NewMockEventBus()
	adapter := NewEventBusAdapter(mockEventBus)

	if !adapter.IsStarted() {
		t.Error("Expected adapter to be started")
	}

	// 关闭适配器
	err := adapter.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if adapter.IsStarted() {
		t.Error("Expected adapter to be stopped")
	}

	// 重复关闭应该是安全的
	err = adapter.Close()
	if err != nil {
		t.Errorf("Expected no error on second close, got %v", err)
	}
}

func TestEventBusAdapter_GetEventBus(t *testing.T) {
	mockEventBus := NewMockEventBus()
	adapter := NewEventBusAdapter(mockEventBus)
	defer adapter.Close()

	eventBus := adapter.GetEventBus()
	if eventBus == nil {
		t.Error("Expected to get EventBus instance")
	}
}
