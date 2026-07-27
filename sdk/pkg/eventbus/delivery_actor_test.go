package eventbus

import (
	"context"
	"testing"
	"time"

	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
	"github.com/IBM/sarama"
)

// deliveryRouting 是 buildAggregateMessage 里「是否走 Delivery 路径」的纯判定，
// 抽出便于单测。plain 订阅返回 nil handler；delivery 订阅返回 handler + 填好的 RawMeta。
func TestDeliveryRouting_PlainSubscriptionReturnsNil(t *testing.T) {
	msg := &sarama.ConsumerMessage{Topic: "t", Value: []byte("v")}
	plain := &handlerWrapper{
		handler: func(ctx context.Context, m []byte) error { return nil },
	}
	raw, dh := deliveryRouting(msg, plain)
	if dh != nil {
		t.Fatalf("plain sub must not route to delivery, got %v", dh)
	}
	if raw.Topic != "" {
		t.Fatalf("plain sub must not populate RawMeta, got %+v", raw)
	}
}

func TestDeliveryRouting_DeliverySubscriptionPopulatesRawAndHandler(t *testing.T) {
	msg := &sarama.ConsumerMessage{
		Topic: "t", Partition: 2, Offset: 5,
		Key: []byte("k"), Value: []byte(`{"x":1}`),
		Headers:   []*sarama.RecordHeader{{Key: []byte("h"), Value: []byte("v")}},
		Timestamp: time.UnixMilli(1_700_000_000_000).UTC(),
	}
	want := EnvelopeDeliveryHandler(func(ctx context.Context, d EnvelopeDelivery) error { return nil })
	deliv := &handlerWrapper{deliveryHandler: want}

	raw, dh := deliveryRouting(msg, deliv)

	if dh == nil {
		t.Fatal("delivery sub must route to delivery handler")
	}
	if raw.Topic != "t" || raw.Partition != 2 || raw.Offset != 5 {
		t.Fatalf("raw coords wrong: %+v", raw)
	}
	if string(raw.RawKey) != "k" || string(raw.RawValue) != `{"x":1}` {
		t.Fatalf("raw kv wrong: key=%q value=%q", raw.RawKey, raw.RawValue)
	}
	if raw.PayloadHash == "" {
		t.Fatal("payload hash must be populated")
	}
	if len(raw.Headers) != 1 || raw.Headers[0].Key != "h" {
		t.Fatalf("raw headers wrong: %+v", raw.Headers)
	}
}

// 兜底：nil wrapper 不 panic。
func TestDeliveryRouting_NilWrapper(t *testing.T) {
	raw, dh := deliveryRouting(&sarama.ConsumerMessage{}, nil)
	if dh != nil || raw.Topic != "" {
		t.Fatalf("nil wrapper must yield zero values: %+v %v", raw, dh)
	}
}

// T-GAP3 回归（REGRESSION RULE，强制）：Receive 改成分支后，plain 路径（DeliveryHandler==nil）
// 仍必须调用 msg.Handler。这是被改动的既有行为，delivery 路径已由 invokeDelivery/deliveryRouting
// 覆盖；本测试钉住「旧订阅不被新分支静默绕过」。直接测 dispatchMessage，无需拉起 Hollywood engine。
func TestDispatchMessage_PlainHandlerStillCalled(t *testing.T) {
	called := false
	msg := &DomainEventMessage{
		Value: []byte("plain"),
		Handler: func(ctx context.Context, m []byte) error {
			called = (string(m) == "plain")
			return nil
		},
		DeliveryHandler: nil,
		Context:         context.Background(),
	}
	if err := dispatchMessage(msg); err != nil {
		t.Fatalf("dispatchMessage plain path: %v", err)
	}
	if !called {
		t.Fatal("plain path must still invoke msg.Handler when DeliveryHandler is nil (T-GAP3 regression)")
	}
}

// D8：delivery 分支必须真的走 invokeDelivery，并把 RawMeta 交到 handler 手上。
// 与 Task 2 的 invokeDelivery 单测互补——那里测解码，这里测「从 DomainEventMessage 进得去」。
func TestDispatchMessage_DeliveryBranchCarriesRawMeta(t *testing.T) {
	env := &Envelope{
		EventID: "e3", AggregateID: "agg-3", EventType: "created",
		EventVersion: 1, Timestamp: time.Now().UTC(), Payload: jxtjson.RawMessage(`{}`),
	}
	data, err := env.ToBytes()
	if err != nil {
		t.Fatalf("ToBytes: %v", err)
	}
	var got EnvelopeDelivery
	msg := &DomainEventMessage{
		Value:   data,
		Handler: nil, // delivery 路径下必须为 nil（D3 互斥不变量）
		Raw:     RawMeta{Topic: "t", Partition: 3, Offset: 9},
		DeliveryHandler: func(ctx context.Context, d EnvelopeDelivery) error {
			got = d
			return nil
		},
		Context: context.Background(),
	}
	if err := dispatchMessage(msg); err != nil {
		t.Fatalf("dispatchMessage delivery path: %v", err)
	}
	if got.Raw.Topic != "t" || got.Raw.Partition != 3 || got.Raw.Offset != 9 {
		t.Fatalf("RawMeta not delivered to handler: %+v", got.Raw)
	}
	if got.Envelope == nil || got.Envelope.EventID != "e3" {
		t.Fatalf("envelope not decoded: %+v", got.Envelope)
	}
}

// D3：两个 handler 都为 nil 时必须返回显式 error，不得 nil-panic。
// Receive 跑在 actor goroutine 且无 recover，nil 调用会触发 supervisor 重启 actor，
// 现场表现为「某个聚合的消息莫名全卡住」——必须换成可诊断的 error。
func TestDispatchMessage_BothHandlersNilReturnsError(t *testing.T) {
	msg := &DomainEventMessage{Value: []byte("x"), Context: context.Background()}
	if err := dispatchMessage(msg); err == nil {
		t.Fatal("both handlers nil must return an error, not panic")
	}
}
