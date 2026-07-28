package eventbus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Step 0 survey conclusions (see task-4-report.md for full detail):
//
// 1. Construction: there is no `newKafkaEventBus` constructor; existing tests
//    build `&kafkaEventBus{...}` directly (see kafka_partitions_selfheal_test.go
//    `newBusWithAdmin`, testing_helpers_test.go `newTestKafkaEventBus`). The
//    registry path under test touches: logger, config (Consumer.GroupID is read
//    by the subscribe-time log line), subscriptions (sync.Map), activeTopicHandlers
//    (sync.Map), closed (atomic.Bool, zero=false is fine), allPossibleTopics +
//    topicsSnapshot (pre-subscription list), and consumerStarted (bool gate on
//    startPreSubscriptionConsumer).
//
// 2. startPreSubscriptionConsumer spawns a goroutine that calls
//    getUnifiedConsumerGroup (returns "not initialized" error when
//    unifiedConsumerGroup is nil — does NOT dial) AND blocks 3s for warmup.
//    The wiring tests verify REGISTRATION side-effects (D2), not consumer
//    startup, so we pre-set consumerStarted=true to skip both the goroutine
//    spawn and the warmup sleep. This is the broker-free seam.
//
// 3. Pre-subscription list read: `k.allPossibleTopics` is a private []string
//    field; same-package test accesses it directly via topicIsPreSubscribed.
//    No new exported method added (brief constraint).
// ---------------------------------------------------------------------------

// newKafkaBusForRegisterTest constructs a *kafkaEventBus sufficient for the
// delivery-wiring regression tests: nop logger, minimal config (GroupID for
// the subscribe-time log line), and consumerStarted pre-set true so
// startPreSubscriptionConsumer returns immediately without spawning the
// broker-dialing goroutine or blocking on the 3s warmup sleep. Tests MUST
// NOT touch the broker connection.
func newKafkaBusForRegisterTest(t *testing.T) *kafkaEventBus {
	t.Helper()
	return &kafkaEventBus{
		logger: zap.NewNop(),
		config: &KafkaConfig{
			Consumer: ConsumerConfig{GroupID: "test-register"},
		},
		// consumerStarted=true short-circuits startPreSubscriptionConsumer:
		// the wiring tests assert registration side-effects, not consumer
		// startup (which requires a real broker). This is the same broker-free
		// intent as newTestKafkaEventBus, scoped to the register path.
		consumerStarted: true,
	}
}

// topicIsPreSubscribed reads the private pre-subscription list directly.
// Same-package test only; not exported.
func topicIsPreSubscribed(k *kafkaEventBus, topic string) bool {
	k.allPossibleTopicsMu.Lock()
	defer k.allPossibleTopicsMu.Unlock()
	for _, t := range k.allPossibleTopics {
		if t == topic {
			return true
		}
	}
	return false
}

// PR-1 验收条目 1（C4）：不支持 Delivery+DLQ 的后端不得实现该能力接口——
// reliable 订阅靠类型断言 fail-fast，不静默降级。
func TestKafkaBusImplementsEnvelopeDeliveryOptionsSubscriber(t *testing.T) {
	var k *kafkaEventBus
	if _, ok := interface{}(k).(EnvelopeDeliveryOptionsSubscriber); !ok {
		t.Fatal("kafkaEventBus must implement EnvelopeDeliveryOptionsSubscriber")
	}
}

func TestMemoryBusDoesNotImplementEnvelopeDelivery(t *testing.T) {
	var m *memoryEventBus
	if _, ok := interface{}(m).(EnvelopeDeliveryOptionsSubscriber); ok {
		t.Fatal("memoryEventBus must NOT implement EnvelopeDeliveryOptionsSubscriber (fail-fast contract)")
	}
}

func TestNatsBusDoesNotImplementEnvelopeDelivery(t *testing.T) {
	var n *natsEventBus
	if _, ok := interface{}(n).(EnvelopeDeliveryOptionsSubscriber); ok {
		t.Fatal("natsEventBus must NOT implement EnvelopeDeliveryOptionsSubscriber (fail-fast contract)")
	}
}

// ---------------------------------------------------------------------------
// D2：接线真实性回归。上面三条只验「方法签名存在」，接线完全死掉它们照样绿。
// 下面四条验「订阅真的生效」：不连 broker，只断言注册副作用。
// ---------------------------------------------------------------------------

func TestSubscribeEnvelopeDelivery_RegistersTopicAndHandler(t *testing.T) {
	k := newKafkaBusForRegisterTest(t)
	h := func(ctx context.Context, d EnvelopeDelivery) error { return nil }

	if err := k.subscribeEnvelopeDelivery(context.Background(), "t-delivery", h, EnvelopeSubscribeOptions{}); err != nil {
		t.Fatalf("subscribeEnvelopeDelivery: %v", err)
	}

	// (1) topic 必须进预订阅列表——否则消费者永远不会订阅它（静默不消费）
	if !topicIsPreSubscribed(k, "t-delivery") {
		t.Fatal("topic not added to pre-subscription list: consumer will never receive messages")
	}
	// (2) wrapper 必须挂上 deliveryHandler、且 handler 为 nil（D3 互斥）
	v, ok := k.activeTopicHandlers.Load("t-delivery")
	if !ok {
		t.Fatal("no active handler registered for topic")
	}
	w := v.(*handlerWrapper)
	if w.deliveryHandler == nil {
		t.Fatal("wrapper.deliveryHandler must be set for delivery subscription")
	}
	if w.handler != nil {
		t.Fatal("wrapper.handler must be nil for delivery subscription (D3 mutual exclusion)")
	}
	if !w.isEnvelope {
		t.Fatal("delivery subscription must be marked isEnvelope")
	}
}

// D2：重复订阅必须报错（LoadOrStore 守卫），否则第二次订阅静默覆盖第一次。
func TestSubscribeEnvelopeDelivery_DuplicateTopicRejected(t *testing.T) {
	k := newKafkaBusForRegisterTest(t)
	h := func(ctx context.Context, d EnvelopeDelivery) error { return nil }
	if err := k.subscribeEnvelopeDelivery(context.Background(), "dup", h, EnvelopeSubscribeOptions{}); err != nil {
		t.Fatalf("first subscribe: %v", err)
	}
	if err := k.subscribeEnvelopeDelivery(context.Background(), "dup", h, EnvelopeSubscribeOptions{}); err == nil {
		t.Fatal("duplicate subscription must be rejected, not silently overwrite")
	}
}

// D3：互斥不变量必须在订阅期 fail-fast，而不是等到消息流到 actor 才 nil-panic。
func TestRegisterTopicSubscription_RejectsAmbiguousHandlers(t *testing.T) {
	k := newKafkaBusForRegisterTest(t)
	msgH := func(ctx context.Context, m []byte) error { return nil }
	delH := func(ctx context.Context, d EnvelopeDelivery) error { return nil }
	restorer := func(context.Context) error { return nil } // 值无关紧要——D3 在 LoadOrStore 之前拒绝

	if err := k.registerTopicSubscription(context.Background(), "a", nil, nil, nil, true, EnvelopeSubscribeOptions{}); err == nil {
		t.Fatal("both handlers nil must be rejected at subscribe time")
	}
	if err := k.registerTopicSubscription(context.Background(), "b", msgH, delH, restorer, true, EnvelopeSubscribeOptions{}); err == nil {
		t.Fatal("both handlers non-nil must be rejected at subscribe time")
	}
}

// D8 回归（REGRESSION RULE，强制）：activateTopicHandler 签名变更后，plain 订阅
// 要在 handler 与 isEnvelope 之间补一个 nil。两个形参都是函数类型——塞错位置
// 编译器不报错，只有断言 wrapper 字段能抓到。
func TestActivateTopicHandler_PlainSubscriptionLeavesDeliveryNil(t *testing.T) {
	k := newKafkaBusForRegisterTest(t)
	plain := func(ctx context.Context, m []byte) error { return nil }
	k.activateTopicHandler("t-plain", plain, nil, false, nil, nil)

	v, ok := k.activeTopicHandlers.Load("t-plain")
	if !ok {
		t.Fatal("plain handler not registered")
	}
	w := v.(*handlerWrapper)
	if w.handler == nil {
		t.Fatal("plain subscription must keep handler (arg-position regression)")
	}
	if w.deliveryHandler != nil {
		t.Fatal("plain subscription must leave deliveryHandler nil (arg-position regression)")
	}
}

// D8 第二条强制回归（Step 5a）：buildAggregateMessage 真的把 deliveryRouting
// 结果挂到 aggMsg 上。这是 Raw/DeliveryHandler 的唯一落位点。
func TestBuildAggregateMessage_PopulatesRawAndDeliveryHandler(t *testing.T) {
	k := newKafkaBusForRegisterTest(t)
	h := &preSubscriptionConsumerHandler{eventBus: k}
	msg := &sarama.ConsumerMessage{
		Topic: "t", Partition: 2, Offset: 7,
		Key: []byte("k"), Value: []byte(`{"event_id":"e"}`),
	}

	// delivery 订阅：Raw 填满、DeliveryHandler 非 nil
	delH := func(ctx context.Context, d EnvelopeDelivery) error { return nil }
	aggD := h.buildAggregateMessage(context.Background(), msg, &handlerWrapper{deliveryHandler: delH, isEnvelope: true})
	if aggD.DeliveryHandler == nil {
		t.Fatal("delivery wrapper must yield non-nil aggMsg.DeliveryHandler")
	}
	if aggD.Raw.Topic != "t" || aggD.Raw.Partition != 2 || aggD.Raw.Offset != 7 || aggD.Raw.PayloadHash == "" {
		t.Fatalf("aggMsg.Raw not populated from deliveryRouting: %+v", aggD.Raw)
	}

	// plain 订阅（回归）：两者保持零值，Handler 不变
	plain := func(ctx context.Context, m []byte) error { return nil }
	aggP := h.buildAggregateMessage(context.Background(), msg, &handlerWrapper{handler: plain})
	if aggP.DeliveryHandler != nil || aggP.Raw.Topic != "" {
		t.Fatalf("plain subscription must leave Raw/DeliveryHandler zero: %+v", aggP.Raw)
	}
	if aggP.Handler == nil {
		t.Fatal("plain subscription must keep aggMsg.Handler")
	}
}

// D10 回归（修复后）：重连后 restoreSubscriptions 必须恢复【全部三种】订阅——
// plain / envelope / delivery。旧实现把 delivery 条目归为 unrestorable 并静默丢弃。
// 「先删后调」还顺带修了历史 F6（无 Delete → Subscribe 返回 already subscribed）。
func TestRestoreSubscriptions_RestoresAllKinds(t *testing.T) {
	k := newKafkaBusForRegisterTest(t)
	ctx := context.Background()

	plainH := func(context.Context, []byte) error { return nil }
	envH := func(context.Context, *Envelope) error { return nil }
	delH := func(context.Context, EnvelopeDelivery) error { return nil }

	if err := k.Subscribe(ctx, "t-plain", plainH); err != nil {
		t.Fatalf("Subscribe plain: %v", err)
	}
	if err := k.subscribeEnvelope(ctx, "t-env", envH, EnvelopeSubscribeOptions{}); err != nil {
		t.Fatalf("subscribeEnvelope: %v", err)
	}
	if err := k.subscribeEnvelopeDelivery(ctx, "t-del", delH, EnvelopeSubscribeOptions{}); err != nil {
		t.Fatalf("subscribeEnvelopeDelivery: %v", err)
	}

	// 模拟重连把 active handlers 撕掉（broker 端消费者已 gone，待 restore 重建）。
	k.activeTopicHandlers.Delete("t-plain")
	k.activeTopicHandlers.Delete("t-env")
	k.activeTopicHandlers.Delete("t-del")

	// 重连恢复：三种订阅都必须重新激活，不能只恢复 plain/envelope 而丢 delivery。
	if err := k.restoreSubscriptions(ctx); err != nil {
		t.Fatalf("restoreSubscriptions must restore all three kinds, got: %v", err)
	}

	for _, topic := range []string{"t-plain", "t-env", "t-del"} {
		v, ok := k.activeTopicHandlers.Load(topic)
		if !ok {
			t.Errorf("topic %s must be re-activated after restore", topic)
			continue
		}
		w, ok := v.(*handlerWrapper)
		if !ok {
			t.Errorf("topic %s active handler has wrong type %T", topic, v)
			continue
		}
		switch topic {
		case "t-del":
			if w.deliveryHandler == nil {
				t.Error("delivery topic must be restored with its deliveryHandler (D10)")
			}
		case "t-env":
			if w.handler == nil {
				t.Error("envelope topic must be restored with its (wrapped) handler (D10)")
			}
		case "t-plain":
			if w.handler == nil {
				t.Error("plain topic must be restored with its handler (D10)")
			}
		}
	}
}

// fakeConsumerGroup 是消费循环重连测试用的假 sarama.ConsumerGroup。只实现 Consume/Close
// （nil-embed 接口保其余方法签名；测试路径只调 Consume，生产修复路径只调 Close）。不接触 broker。
type fakeConsumerGroup struct {
	sarama.ConsumerGroup // nil-embedded：仅满足接口签名，未覆盖方法不会在测试路径被调
	id           string
	consumeCalls int32
	closed       int32
}

func (f *fakeConsumerGroup) Consume(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error {
	atomic.AddInt32(&f.consumeCalls, 1)
	// 模拟「Consume 返回（重平衡/会话结束）→ 循环重新迭代」；短暂 sleep 避免忙循环吃满 CPU。
	time.Sleep(2 * time.Millisecond)
	return nil
}

func (f *fakeConsumerGroup) Close() error {
	atomic.StoreInt32(&f.closed, 1)
	return nil
}

// D10 完整修复回归：消费循环每轮从 k.unifiedConsumerGroup 取 group 调 Consume。
// 模拟重连——把 unifiedConsumerGroup 替换成新 group——循环必须在下一次迭代读到新 group。
// 这条【循环侧】不变量 + reinitializeConnection 重建 group，共同保证「重连后真实消费恢复」。
// （reinitializeConnection 自身调 sarama.NewClient 会拨号，无法 broker-free 测试；本测试钉住
// 它依赖的循环侧不变量——若循环不重读 group，reinitializeConnection 的重建也无济于事。）
func TestConsumeLoop_ReadsReplacedConsumerGroup(t *testing.T) {
	k := newKafkaBusForRegisterTest(t)
	k.consumerStarted = false                  // 默认 true 是为短路消费者启动；这里要真起循环
	k.topicsSnapshot.Store([]string{"t-fake"}) // 非空，否则循环会等 ctx.Done 直接退出

	g1 := &fakeConsumerGroup{id: "g1"}
	k.unifiedConsumerGroup.Store(g1)

	if err := k.startPreSubscriptionConsumer(context.Background()); err != nil {
		t.Fatalf("startPreSubscriptionConsumer: %v", err)
	}
	defer func() {
		if k.consumerCancel != nil {
			k.consumerCancel()
		}
		<-k.consumerDone
	}()

	waitConsumed := func(g *fakeConsumerGroup, what string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && atomic.LoadInt32(&g.consumeCalls) == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		if atomic.LoadInt32(&g.consumeCalls) == 0 {
			t.Fatalf("loop never Consume'd on %s", what)
		}
	}

	// 循环先在 g1 上 Consume。
	waitConsumed(g1, "g1 (before swap)")

	// 模拟重连修复：用新 group 替换 unifiedConsumerGroup（reinitializeConnection 现在就是这么做的）。
	g2 := &fakeConsumerGroup{id: "g2"}
	k.unifiedConsumerGroup.Store(g2)

	// 循环下一次迭代必须读到 g2 并 Consume。
	waitConsumed(g2, "g2 (after swap)")

	// 切换后 g1 不应再被调用。
	g1CallsAtSwap := atomic.LoadInt32(&g1.consumeCalls)
	time.Sleep(30 * time.Millisecond)
	if got := atomic.LoadInt32(&g1.consumeCalls); got > g1CallsAtSwap {
		t.Errorf("g1 must not be invoked after swap; calls grew %d -> %d", g1CallsAtSwap, got)
	}
}
