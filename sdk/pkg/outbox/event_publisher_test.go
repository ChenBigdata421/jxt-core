package outbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNoOpEventPublisher(t *testing.T) {
	publisher := NewNoOpEventPublisher()

	// 测试 PublishEnvelope
	envelope := &Envelope{
		EventID:      "test-event-1",
		AggregateID:  "test-aggregate-1",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      []byte(`{"test":"data"}`),
		Timestamp:    time.Now(),
	}

	err := publisher.PublishEnvelope(context.Background(), "test-topic", envelope)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 测试 GetPublishResultChannel
	resultChan := publisher.GetPublishResultChannel()
	if resultChan == nil {
		t.Error("Expected result channel, got nil")
	}

	// 应该收到成功的 ACK
	select {
	case result := <-resultChan:
		if !result.Success {
			t.Error("Expected success ACK")
		}
		if result.EventID != "test-event-1" {
			t.Errorf("Expected EventID 'test-event-1', got '%s'", result.EventID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive ACK result")
	}
}

// MockEventPublisher 用于测试的 Mock EventPublisher
type MockEventPublisher struct {
	PublishedEnvelopes []*Envelope
	ShouldFail         bool
	FailError          error
	resultChan         chan *PublishResult
}

func NewMockEventPublisher() *MockEventPublisher {
	return &MockEventPublisher{
		PublishedEnvelopes: make([]*Envelope, 0),
		resultChan:         make(chan *PublishResult, 100),
	}
}

func (m *MockEventPublisher) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
	if m.ShouldFail {
		// 发送失败 ACK
		go func() {
			m.resultChan <- &PublishResult{
				EventID:     envelope.EventID,
				Topic:       topic,
				Success:     false,
				Error:       m.FailError,
				Timestamp:   time.Now(),
				AggregateID: envelope.AggregateID,
				EventType:   envelope.EventType,
			}
		}()

		if m.FailError != nil {
			return m.FailError
		}
		return errors.New("mock publish error")
	}

	m.PublishedEnvelopes = append(m.PublishedEnvelopes, envelope)

	// 发送成功 ACK
	go func() {
		m.resultChan <- &PublishResult{
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

func (m *MockEventPublisher) GetPublishResultChannel() <-chan *PublishResult {
	return m.resultChan
}

func (m *MockEventPublisher) Reset() {
	m.PublishedEnvelopes = make([]*Envelope, 0)
	m.ShouldFail = false
	m.FailError = nil
	// 清空通道
	for len(m.resultChan) > 0 {
		<-m.resultChan
	}
}

func TestMockEventPublisher(t *testing.T) {
	mock := NewMockEventPublisher()

	// 测试成功发布
	envelope1 := &Envelope{
		EventID:      "event-1",
		AggregateID:  "agg-1",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      []byte(`{"test":"data1"}`),
		Timestamp:    time.Now(),
	}

	err := mock.PublishEnvelope(context.Background(), "topic1", envelope1)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	envelope2 := &Envelope{
		EventID:      "event-2",
		AggregateID:  "agg-2",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      []byte(`{"test":"data2"}`),
		Timestamp:    time.Now(),
	}

	err = mock.PublishEnvelope(context.Background(), "topic2", envelope2)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(mock.PublishedEnvelopes) != 2 {
		t.Errorf("Expected 2 published envelopes, got %d", len(mock.PublishedEnvelopes))
	}

	if mock.PublishedEnvelopes[0].EventID != "event-1" {
		t.Errorf("Expected EventID 'event-1', got '%s'", mock.PublishedEnvelopes[0].EventID)
	}

	if mock.PublishedEnvelopes[1].EventID != "event-2" {
		t.Errorf("Expected EventID 'event-2', got '%s'", mock.PublishedEnvelopes[1].EventID)
	}

	// 测试 ACK 结果
	resultChan := mock.GetPublishResultChannel()

	// 应该收到 2 个成功的 ACK
	for i := 0; i < 2; i++ {
		select {
		case result := <-resultChan:
			if !result.Success {
				t.Errorf("Expected success ACK, got failure: %v", result.Error)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Expected to receive ACK result")
		}
	}

	// 测试失败发布
	mock.ShouldFail = true
	mock.FailError = errors.New("test error")

	envelope3 := &Envelope{
		EventID:      "event-3",
		AggregateID:  "agg-3",
		EventType:    "TestEvent",
		EventVersion: 1,
		Payload:      []byte(`{"test":"data3"}`),
		Timestamp:    time.Now(),
	}

	err = mock.PublishEnvelope(context.Background(), "topic3", envelope3)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// 应该收到失败的 ACK
	select {
	case result := <-resultChan:
		if result.Success {
			t.Error("Expected failure ACK, got success")
		}
		if result.Error == nil {
			t.Error("Expected error in ACK result")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive ACK result")
	}

	// 测试重置
	mock.Reset()
	if len(mock.PublishedEnvelopes) != 0 {
		t.Errorf("Expected 0 published envelopes after reset, got %d", len(mock.PublishedEnvelopes))
	}

	if mock.ShouldFail {
		t.Error("Expected ShouldFail to be false after reset")
	}
}
