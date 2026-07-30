package eventbus

import (
	"testing"
	"time"

	"github.com/IBM/sarama"
)

// C6 回归：Headers 必须保序、保留重复 key、并携带 broker 时间戳。
// 旧实现 Headers 是 map[string]string：丢序 + 合并重复 key，且无 Timestamp。
func TestToPoisonMessage_PreservesOrderedDuplicateHeadersAndTimestamp(t *testing.T) {
	src := &sarama.ConsumerMessage{
		Topic:     "domain.order.created",
		Partition: 3,
		Offset:    42,
		Key:       []byte("order-7"),
		Value:     []byte(`{"event_id":"e1"}`),
		Headers: []*sarama.RecordHeader{
			{Key: []byte("a"), Value: []byte("1")},
			{Key: []byte("b"), Value: []byte("2")},
			{Key: []byte("a"), Value: []byte("3")}, // 重复 key，必须保留
		},
		Timestamp: time.UnixMilli(1_700_000_000_000).UTC(),
	}

	pm := toPoisonMessage(src)

	if pm.Topic != src.Topic {
		t.Fatalf("topic: got %q want %q", pm.Topic, src.Topic)
	}
	if pm.Partition != 3 || pm.Offset != 42 {
		t.Fatalf("coords: got partition=%d offset=%d", pm.Partition, pm.Offset)
	}
	if string(pm.Key) != "order-7" {
		t.Fatalf("key: got %q", pm.Key)
	}
	if string(pm.Value) != `{"event_id":"e1"}` {
		t.Fatalf("value: got %q", pm.Value)
	}
	if !pm.Timestamp.Equal(src.Timestamp) {
		t.Fatalf("timestamp: got %v want %v", pm.Timestamp, src.Timestamp)
	}
	if len(pm.Headers) != 3 {
		t.Fatalf("headers len: got %d want 3 (duplicate key must be preserved)", len(pm.Headers))
	}
	want := []MessageHeader{
		{Key: "a", Value: []byte("1")},
		{Key: "b", Value: []byte("2")},
		{Key: "a", Value: []byte("3")},
	}
	for i, h := range pm.Headers {
		if h.Key != want[i].Key || string(h.Value) != string(want[i].Value) {
			t.Fatalf("header[%d]: got %+v want %+v", i, h, want[i])
		}
	}
	// OV#1：Key/Value 必须是防御性拷贝——突变 src 不应影响 PoisonMessage 的字段
	src.Key[0] = 'Z'
	src.Value[0] = '!'
	if pm.Key[0] == 'Z' || pm.Value[0] == '!' {
		t.Fatal("PoisonMessage.Key/Value must be defensive copies, not aliases of the source message")
	}
}
