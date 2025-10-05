package eventbus

import (
	"encoding/json"
	"testing"
	"time"
)

// TestMessage 测试消息结构
type TestMessage struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	OrderID   string    `json:"order_id"`
	Amount    float64   `json:"amount"`
	Payload   []byte    `json:"payload"`
}

// 创建测试数据
func createTestMessage() *TestMessage {
	return &TestMessage{
		ID:        "test-message-12345",
		Type:      "order.created",
		Timestamp: time.Now(),
		OrderID:   "order-67890",
		Amount:    99.99,
		Payload:   []byte(`{"order_id":"order-67890","customer_id":"customer-54321","amount":99.99,"items":[{"id":"item-1","name":"Product A","price":29.99},{"id":"item-2","name":"Product B","price":69.99}],"shipping":{"address":"123 Main St","city":"New York","country":"USA"}}`),
	}
}

// 创建复杂测试数据
func createComplexTestMessage() *TestMessage {
	msg := createTestMessage()
	// 增加数据复杂度 - 创建更大的 Payload
	largePayload := make([]byte, 10000) // 10KB 的数据
	for i := range largePayload {
		largePayload[i] = byte('a' + (i % 26))
	}
	msg.Payload = largePayload
	return msg
}

// BenchmarkStandardJSON_Marshal 标准库 JSON 序列化性能测试
func BenchmarkStandardJSON_Marshal(b *testing.B) {
	msg := createTestMessage()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJsoniter_Marshal jsoniter 序列化性能测试
func BenchmarkJsoniter_Marshal(b *testing.B) {
	msg := createTestMessage()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJsoniterFast_Marshal jsoniter 最快配置序列化性能测试
func BenchmarkJsoniterFast_Marshal(b *testing.B) {
	msg := createTestMessage()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := MarshalFast(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStandardJSON_Unmarshal 标准库 JSON 反序列化性能测试
func BenchmarkStandardJSON_Unmarshal(b *testing.B) {
	msg := createTestMessage()
	data, _ := json.Marshal(msg)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var result TestMessage
		err := json.Unmarshal(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJsoniter_Unmarshal jsoniter 反序列化性能测试
func BenchmarkJsoniter_Unmarshal(b *testing.B) {
	msg := createTestMessage()
	data, _ := Marshal(msg)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var result TestMessage
		err := Unmarshal(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJsoniterFast_Unmarshal jsoniter 最快配置反序列化性能测试
func BenchmarkJsoniterFast_Unmarshal(b *testing.B) {
	msg := createTestMessage()
	data, _ := MarshalFast(msg)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var result TestMessage
		err := UnmarshalFast(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStandardJSON_MarshalString 标准库 JSON 字符串序列化性能测试
func BenchmarkStandardJSON_MarshalString(b *testing.B) {
	msg := createTestMessage()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
		_ = string(data) // 转换为字符串
	}
}

// BenchmarkJsoniter_MarshalToString jsoniter 直接序列化为字符串性能测试
func BenchmarkJsoniter_MarshalToString(b *testing.B) {
	msg := createTestMessage()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := MarshalToString(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComplexData_StandardJSON 复杂数据标准库性能测试
func BenchmarkComplexData_StandardJSON(b *testing.B) {
	msg := createComplexTestMessage()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
		var result TestMessage
		err = json.Unmarshal(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComplexData_Jsoniter 复杂数据 jsoniter 性能测试
func BenchmarkComplexData_Jsoniter(b *testing.B) {
	msg := createComplexTestMessage()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
		var result TestMessage
		err = Unmarshal(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkComplexData_JsoniterFast 复杂数据 jsoniter 最快配置性能测试
func BenchmarkComplexData_JsoniterFast(b *testing.B) {
	msg := createComplexTestMessage()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := MarshalFast(msg)
		if err != nil {
			b.Fatal(err)
		}
		var result TestMessage
		err = UnmarshalFast(data, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestJSONCompatibility 测试 jsoniter 与标准库的兼容性
func TestJSONCompatibility(t *testing.T) {
	msg := createTestMessage()

	// 使用标准库序列化
	stdData, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	// 使用 jsoniter 序列化
	jsoniterData, err := Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	// 验证两者可以互相反序列化
	var stdResult, jsoniterResult TestMessage

	// 标准库数据用 jsoniter 反序列化
	err = Unmarshal(stdData, &stdResult)
	if err != nil {
		t.Fatal(err)
	}

	// jsoniter 数据用标准库反序列化
	err = json.Unmarshal(jsoniterData, &jsoniterResult)
	if err != nil {
		t.Fatal(err)
	}

	// 验证结果一致性
	if stdResult.ID != msg.ID || jsoniterResult.ID != msg.ID {
		t.Error("ID mismatch")
	}
	if stdResult.Type != msg.Type || jsoniterResult.Type != msg.Type {
		t.Error("Type mismatch")
	}
}

// TestEnvelopeJSONPerformance 测试 Envelope 的 JSON 性能
func TestEnvelopeJSONPerformance(t *testing.T) {
	payload := []byte(`{"order_id":"order-123","amount":99.99}`)
	envelope := NewEnvelope("order-123", "OrderCreated", 1, payload)

	// 测试序列化
	data, err := envelope.ToBytes()
	if err != nil {
		t.Fatal(err)
	}

	// 测试反序列化
	env2, err := FromBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	// 验证数据一致性
	if env2.AggregateID != envelope.AggregateID {
		t.Error("AggregateID mismatch")
	}
	if env2.EventType != envelope.EventType {
		t.Error("EventType mismatch")
	}
	if env2.EventVersion != envelope.EventVersion {
		t.Error("EventVersion mismatch")
	}
}
