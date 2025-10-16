package eventbus

import (
	"testing"
	"time"
)

// TestEnvelopeEventID 测试 Envelope EventID 生成
func TestEnvelopeEventID(t *testing.T) {
	tests := []struct {
		name        string
		aggregateID string
		eventType   string
		version     int64
		wantPrefix  string
	}{
		{
			name:        "订单创建事件",
			aggregateID: "order:12345",
			eventType:   "OrderCreated",
			version:     1,
			wantPrefix:  "order:12345:OrderCreated:1:",
		},
		{
			name:        "支付完成事件",
			aggregateID: "payment:67890",
			eventType:   "PaymentCompleted",
			version:     2,
			wantPrefix:  "payment:67890:PaymentCompleted:2:",
		},
		{
			name:        "库存变更事件",
			aggregateID: "inventory:abc123",
			eventType:   "InventoryChanged",
			version:     1,
			wantPrefix:  "inventory:abc123:InventoryChanged:1:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 Envelope
			envelope := NewEnvelope(tt.aggregateID, tt.eventType, tt.version, []byte(`{"test":"data"}`))

			// 测试 GenerateEventID
			eventID := envelope.GenerateEventID()
			if eventID == "" {
				t.Errorf("GenerateEventID() returned empty string")
			}

			// 验证前缀
			if len(eventID) < len(tt.wantPrefix) {
				t.Errorf("GenerateEventID() = %v, want prefix %v", eventID, tt.wantPrefix)
			}
			if eventID[:len(tt.wantPrefix)] != tt.wantPrefix {
				t.Errorf("GenerateEventID() = %v, want prefix %v", eventID, tt.wantPrefix)
			}

			// 验证 EventID 包含时间戳
			if len(eventID) <= len(tt.wantPrefix) {
				t.Errorf("GenerateEventID() should contain timestamp, got %v", eventID)
			}

			t.Logf("Generated EventID: %s", eventID)
		})
	}
}

// TestEnvelopeEnsureEventID 测试 EnsureEventID 方法
func TestEnvelopeEnsureEventID(t *testing.T) {
	t.Run("EventID 为空时自动生成", func(t *testing.T) {
		envelope := NewEnvelope("order:12345", "OrderCreated", 1, []byte(`{"test":"data"}`))

		// 初始 EventID 应该为空
		if envelope.EventID != "" {
			t.Errorf("Initial EventID should be empty, got %v", envelope.EventID)
		}

		// 调用 EnsureEventID
		envelope.EnsureEventID()

		// EventID 应该已生成
		if envelope.EventID == "" {
			t.Errorf("EnsureEventID() should generate EventID")
		}

		t.Logf("Generated EventID: %s", envelope.EventID)
	})

	t.Run("EventID 已存在时不重新生成", func(t *testing.T) {
		envelope := NewEnvelope("order:12345", "OrderCreated", 1, []byte(`{"test":"data"}`))

		// 手动设置 EventID
		originalEventID := "custom:event:id:123"
		envelope.EventID = originalEventID

		// 调用 EnsureEventID
		envelope.EnsureEventID()

		// EventID 应该保持不变
		if envelope.EventID != originalEventID {
			t.Errorf("EnsureEventID() should not change existing EventID, got %v, want %v", envelope.EventID, originalEventID)
		}

		t.Logf("EventID unchanged: %s", envelope.EventID)
	})

	t.Run("使用 NewEnvelopeWithEventID 创建自定义 EventID", func(t *testing.T) {
		customEventID := "my-custom-event-id-12345"
		envelope := NewEnvelopeWithEventID(
			customEventID,
			"order:12345",
			"OrderCreated",
			1,
			[]byte(`{"test":"data"}`),
		)

		// EventID 应该是用户设定的值
		if envelope.EventID != customEventID {
			t.Errorf("NewEnvelopeWithEventID() should set custom EventID, got %v, want %v", envelope.EventID, customEventID)
		}

		// 调用 EnsureEventID 不应该改变用户设定的 EventID
		envelope.EnsureEventID()
		if envelope.EventID != customEventID {
			t.Errorf("EnsureEventID() should not change user-defined EventID, got %v, want %v", envelope.EventID, customEventID)
		}

		t.Logf("Custom EventID preserved: %s", envelope.EventID)
	})

	t.Run("多次调用 EnsureEventID 应该幂等", func(t *testing.T) {
		envelope := NewEnvelope("order:12345", "OrderCreated", 1, []byte(`{"test":"data"}`))

		// 第一次调用
		envelope.EnsureEventID()
		firstEventID := envelope.EventID

		// 第二次调用
		envelope.EnsureEventID()
		secondEventID := envelope.EventID

		// EventID 应该相同
		if firstEventID != secondEventID {
			t.Errorf("Multiple EnsureEventID() calls should be idempotent, got %v and %v", firstEventID, secondEventID)
		}

		t.Logf("EventID consistent: %s", firstEventID)
	})
}

// TestEnvelopeEventIDUniqueness 测试 EventID 唯一性
func TestEnvelopeEventIDUniqueness(t *testing.T) {
	t.Run("相同参数但不同时间戳应生成不同 EventID", func(t *testing.T) {
		envelope1 := NewEnvelope("order:12345", "OrderCreated", 1, []byte(`{"test":"data"}`))
		time.Sleep(1 * time.Millisecond) // 确保时间戳不同
		envelope2 := NewEnvelope("order:12345", "OrderCreated", 1, []byte(`{"test":"data"}`))

		eventID1 := envelope1.GenerateEventID()
		eventID2 := envelope2.GenerateEventID()

		if eventID1 == eventID2 {
			t.Errorf("EventIDs should be different for different timestamps, got %v and %v", eventID1, eventID2)
		}

		t.Logf("EventID1: %s", eventID1)
		t.Logf("EventID2: %s", eventID2)
	})

	t.Run("不同 AggregateID 应生成不同 EventID", func(t *testing.T) {
		now := time.Now()
		envelope1 := &Envelope{
			AggregateID:  "order:12345",
			EventType:    "OrderCreated",
			EventVersion: 1,
			Timestamp:    now,
			Payload:      []byte(`{"test":"data"}`),
		}
		envelope2 := &Envelope{
			AggregateID:  "order:67890",
			EventType:    "OrderCreated",
			EventVersion: 1,
			Timestamp:    now,
			Payload:      []byte(`{"test":"data"}`),
		}

		eventID1 := envelope1.GenerateEventID()
		eventID2 := envelope2.GenerateEventID()

		if eventID1 == eventID2 {
			t.Errorf("EventIDs should be different for different AggregateIDs, got %v and %v", eventID1, eventID2)
		}

		t.Logf("EventID1: %s", eventID1)
		t.Logf("EventID2: %s", eventID2)
	})

	t.Run("不同 EventType 应生成不同 EventID", func(t *testing.T) {
		now := time.Now()
		envelope1 := &Envelope{
			AggregateID:  "order:12345",
			EventType:    "OrderCreated",
			EventVersion: 1,
			Timestamp:    now,
			Payload:      []byte(`{"test":"data"}`),
		}
		envelope2 := &Envelope{
			AggregateID:  "order:12345",
			EventType:    "OrderUpdated",
			EventVersion: 1,
			Timestamp:    now,
			Payload:      []byte(`{"test":"data"}`),
		}

		eventID1 := envelope1.GenerateEventID()
		eventID2 := envelope2.GenerateEventID()

		if eventID1 == eventID2 {
			t.Errorf("EventIDs should be different for different EventTypes, got %v and %v", eventID1, eventID2)
		}

		t.Logf("EventID1: %s", eventID1)
		t.Logf("EventID2: %s", eventID2)
	})

	t.Run("不同 EventVersion 应生成不同 EventID", func(t *testing.T) {
		now := time.Now()
		envelope1 := &Envelope{
			AggregateID:  "order:12345",
			EventType:    "OrderCreated",
			EventVersion: 1,
			Timestamp:    now,
			Payload:      []byte(`{"test":"data"}`),
		}
		envelope2 := &Envelope{
			AggregateID:  "order:12345",
			EventType:    "OrderCreated",
			EventVersion: 2,
			Timestamp:    now,
			Payload:      []byte(`{"test":"data"}`),
		}

		eventID1 := envelope1.GenerateEventID()
		eventID2 := envelope2.GenerateEventID()

		if eventID1 == eventID2 {
			t.Errorf("EventIDs should be different for different EventVersions, got %v and %v", eventID1, eventID2)
		}

		t.Logf("EventID1: %s", eventID1)
		t.Logf("EventID2: %s", eventID2)
	})
}

// TestEnvelopeEventIDFormat 测试 EventID 格式
func TestEnvelopeEventIDFormat(t *testing.T) {
	envelope := NewEnvelope("order-12345", "OrderCreated", 1, []byte(`{"test":"data"}`))
	eventID := envelope.GenerateEventID()

	// 验证格式: AggregateID:EventType:EventVersion:Timestamp
	// 应该包含 4 个冒号分隔的部分
	parts := splitEventID(eventID)
	if len(parts) != 4 {
		t.Errorf("EventID should have 4 parts separated by ':', got %d parts: %v", len(parts), parts)
	}

	// 验证各部分
	if parts[0] != "order-12345" {
		t.Errorf("EventID part 0 should be 'order-12345', got %v", parts[0])
	}
	if parts[1] != "OrderCreated" {
		t.Errorf("EventID part 1 should be 'OrderCreated', got %v", parts[1])
	}
	if parts[2] != "1" {
		t.Errorf("EventID part 2 should be '1', got %v", parts[2])
	}
	// parts[3] 应该是时间戳（纳秒）

	t.Logf("EventID: %s", eventID)
	t.Logf("Parts: %v", parts)
}

// splitEventID 辅助函数：分割 EventID
// 注意：由于 AggregateID 本身可能包含冒号，所以需要特殊处理
// EventID 格式: AggregateID:EventType:EventVersion:Timestamp
// 从右向左分割，取最后 3 个冒号
func splitEventID(eventID string) []string {
	parts := make([]string, 4)

	// 从右向左找到最后 3 个冒号的位置
	colonPositions := make([]int, 0, 3)
	for i := len(eventID) - 1; i >= 0 && len(colonPositions) < 3; i-- {
		if eventID[i] == ':' {
			colonPositions = append(colonPositions, i)
		}
	}

	if len(colonPositions) < 3 {
		// 没有找到足够的冒号，说明格式不正确
		return []string{eventID}
	}

	// 反转 colonPositions（现在是从右到左的顺序，需要从左到右）
	// colonPositions[2] 是第一个冒号（AggregateID 和 EventType 之间）
	// colonPositions[1] 是第二个冒号（EventType 和 EventVersion 之间）
	// colonPositions[0] 是第三个冒号（EventVersion 和 Timestamp 之间）

	parts[0] = eventID[:colonPositions[2]]                      // AggregateID
	parts[1] = eventID[colonPositions[2]+1 : colonPositions[1]] // EventType
	parts[2] = eventID[colonPositions[1]+1 : colonPositions[0]] // EventVersion
	parts[3] = eventID[colonPositions[0]+1:]                    // Timestamp

	return parts
}
