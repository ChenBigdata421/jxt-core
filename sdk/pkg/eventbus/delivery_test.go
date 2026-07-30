package eventbus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
	"github.com/IBM/sarama"
)

func TestToRawMeta_FillsAllFieldsAndPreservesHeaders(t *testing.T) {
	src := &sarama.ConsumerMessage{
		Topic: "t", Partition: 1, Offset: 9,
		Key:   []byte("k"),
		Value: []byte(`{"event_id":"e"}`),
		Headers: []*sarama.RecordHeader{
			{Key: []byte("x"), Value: []byte("1")},
			{Key: []byte("x"), Value: []byte("2")}, // 重复 key
		},
		Timestamp: time.UnixMilli(1_700_000_000_123).UTC(),
	}

	raw := toRawMeta(src)

	if raw.Topic != "t" || raw.Partition != 1 || raw.Offset != 9 {
		t.Fatalf("coords: %+v", raw)
	}
	if string(raw.RawKey) != "k" {
		t.Fatalf("key: %q", raw.RawKey)
	}
	if string(raw.RawValue) != `{"event_id":"e"}` {
		t.Fatalf("value: %q", raw.RawValue)
	}
	if !raw.Timestamp.Equal(src.Timestamp) {
		t.Fatalf("timestamp: got %v want %v", raw.Timestamp, src.Timestamp)
	}
	if len(raw.Headers) != 2 || raw.Headers[0].Key != "x" || raw.Headers[1].Key != "x" {
		t.Fatalf("headers not preserved: %+v", raw.Headers)
	}
	want := sha256.Sum256(src.Value)
	if raw.PayloadHash != hex.EncodeToString(want[:]) {
		t.Fatalf("payload hash: got %s", raw.PayloadHash)
	}
	// 突变源不应改变已取出的切片（防御性拷贝）
	src.Value[0] = '!'
	if string(raw.RawValue) != `{"event_id":"e"}` {
		t.Fatal("RawValue aliases source slice")
	}
}

func TestInvokeDelivery_DecodesAndDelivers(t *testing.T) {
	env := &Envelope{
		EventID: "e1", AggregateID: "agg-1", EventType: "created",
		EventVersion: 1, Timestamp: time.Now().UTC(), Payload: jxtjson.RawMessage(`{}`),
	}
	data, err := env.ToBytes()
	if err != nil {
		t.Fatalf("ToBytes: %v", err)
	}

	var got EnvelopeDelivery
	h := func(ctx context.Context, d EnvelopeDelivery) error { got = d; return nil }

	raw := RawMeta{Topic: "t", PayloadHash: "deadbeef"}
	if err := invokeDelivery(context.Background(), data, raw, h); err != nil {
		t.Fatalf("invokeDelivery: %v", err)
	}
	if got.Envelope.EventID != "e1" {
		t.Fatalf("envelope not delivered: %+v", got.Envelope)
	}
	if got.Raw.Topic != "t" || got.Raw.PayloadHash != "deadbeef" {
		t.Fatalf("raw not delivered: %+v", got.Raw)
	}
}

func TestInvokeDelivery_DecodeErrorPropagatesAndHandlerNotCalled(t *testing.T) {
	called := false
	h := func(ctx context.Context, d EnvelopeDelivery) error { called = true; return nil }

	err := invokeDelivery(context.Background(), []byte("not-json"), RawMeta{}, h)
	// OV#8：旧条件 `!errors.Is(...) && err == nil` 有死分支——非 nil 但未包装 errInvalidEnvelope
	// 的错误（如裸 FromBytes error）会静默通过。改为：err 为 nil 或未包装 sentinel 都判失败。
	if err == nil || !errors.Is(err, errInvalidEnvelope) {
		t.Fatalf("expected errInvalidEnvelope, got %v", err)
	}
	if called {
		t.Fatal("handler must not be called when envelope fails to decode")
	}
}

// D8：业务 handler 自己返回的 error 必须原样透出，不得被包装成 errInvalidEnvelope。
// 这是 PR-2 reliable kernel 的分岔契约：errInvalidEnvelope = 消息本身坏（进 DLQ），
// 其他 error = 处理失败（可重试）。混淆会导致可重试失败被静默丢进死信。
func TestInvokeDelivery_HandlerErrorPassesThroughUnwrapped(t *testing.T) {
	sentinel := errors.New("business failure")
	env := &Envelope{
		EventID: "e2", AggregateID: "agg-2", EventType: "created",
		EventVersion: 1, Timestamp: time.Now().UTC(), Payload: jxtjson.RawMessage(`{}`),
	}
	data, err := env.ToBytes()
	if err != nil {
		t.Fatalf("ToBytes: %v", err)
	}
	h := func(ctx context.Context, d EnvelopeDelivery) error { return sentinel }

	got := invokeDelivery(context.Background(), data, RawMeta{}, h)
	if !errors.Is(got, sentinel) {
		t.Fatalf("business error must pass through unwrapped, got %v", got)
	}
	if errors.Is(got, errInvalidEnvelope) {
		t.Fatal("business error must NOT be wrapped as errInvalidEnvelope")
	}
}

// D9：PR-2 灰度前的性能基线。当前无任何 delivery 订阅者，故此数字只作基线、
// 不作回归判定（没有对照组）。把 ns/op 与 B/op 记入 jxt-core/PR1_SCOPE.md。
func BenchmarkToRawMeta(b *testing.B) {
	sizes := []struct {
		name string
		n    int
	}{{"1KB", 1 << 10}, {"64KB", 64 << 10}, {"1MB", 1 << 20}}
	for _, s := range sizes {
		b.Run(s.name, func(b *testing.B) {
			msg := &sarama.ConsumerMessage{
				Topic: "t", Partition: 1, Offset: 9,
				Key:   []byte("k"),
				Value: make([]byte, s.n),
				Headers: []*sarama.RecordHeader{
					{Key: []byte("x"), Value: []byte("1")},
				},
				Timestamp: time.UnixMilli(1_700_000_000_123).UTC(),
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = toRawMeta(msg)
			}
		})
	}
}
