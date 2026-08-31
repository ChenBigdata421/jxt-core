package reliability_regression_tests

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TestReadyGate_TripsOnceProbe：ReadyGate 必须 latch 不可逆 + ProbeTrip 恰好一次放行。
func TestReadyGate_TripsOnceProbe(t *testing.T) {
	gate := NewReadyGate()
	if gate.Tripped() {
		t.Fatal("fresh gate must not be tripped")
	}
	gate.Trip()
	if !gate.Tripped() {
		t.Fatal("gate must latch after Trip")
	}
	gate.Trip()
	if !gate.Tripped() {
		t.Fatal("gate must stay tripped")
	}
}

// TestReadyGate_ProbeTripThreadsafe：多 goroutine 并发 ProbeTrip 只放行一次。
func TestReadyGate_ProbeTripThreadsafe(t *testing.T) {
	gate := NewReadyGate()
	var trips atomic.Int64
	start := make(chan struct{})
	done := make(chan struct{})
	for i := 0; i < 16; i++ {
		go func() {
			<-start
			if gate.ProbeTrip() {
				trips.Add(1)
			}
			<-done
		}()
	}
	close(start)
	time.Sleep(50 * time.Millisecond)
	close(done)
	if !gate.Tripped() {
		t.Fatal("concurrent ProbeTrip must trip the gate")
	}
	if got := trips.Load(); got != 1 {
		t.Fatalf("ProbeTrip must return true exactly once across goroutines, got %d", got)
	}
}

// TestAwaitConsumerReady_MemoryBus：就绪屏障端到端语义——Subscribe 后【零 sleep】，
// 业务 handler 按哨兵 AggregateID 接线 gate；AwaitConsumerReady 发布 probe 并等目击。
// 这是 CI flake（欠投递签名）的语义修复锚：probe 被处理过 ⇒ fetch 起点已在 probe 之后。
func TestAwaitConsumerReady_MemoryBus(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := eventbus.NewMemoryEventBus()
	defer bus.Close()

	topic := fmt.Sprintf("test.ready.gate.memory.%d", helper.GetTimestamp())
	var business int64
	gate := NewReadyGate()
	ctx := context.Background()

	err := bus.SubscribeEnvelope(ctx, topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		// 业务 handler 的标准三行接线
		if envelope.AggregateID == ReadyProbeAggregateID {
			gate.Trip()
			return nil
		}
		atomic.AddInt64(&business, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeEnvelope failed: %v", err)
	}

	if !helper.AwaitConsumerReady(ctx, bus, topic, gate, 2*time.Second) {
		t.Fatal("AwaitConsumerReady must return true once the probe envelope is witnessed")
	}
	if atomic.LoadInt64(&business) != 0 {
		t.Fatalf("probe must not be counted as business message, got %d", atomic.LoadInt64(&business))
	}

	// 就绪后发布的业务消息必须全部被目击（memory 总线即时，验证屏障不吞消息）
	for i := 0; i < 5; i++ {
		env := &eventbus.Envelope{
			EventID:      fmt.Sprintf("evt-%d", i),
			AggregateID:  "aggregate-1",
			EventType:    "TestEvent",
			EventVersion: int64(i + 1),
			Timestamp:    time.Now(),
			Payload:      []byte(`{}`),
		}
		if err := bus.PublishEnvelope(ctx, topic, env); err != nil {
			t.Fatalf("PublishEnvelope failed: %v", err)
		}
	}
	if !helper.WaitForMessages(&business, 5, 2*time.Second) {
		t.Fatalf("business messages after gate must all arrive, got %d", atomic.LoadInt64(&business))
	}
}

// TestAwaitConsumerReady_TimeoutFails：probe 无法被目击（handler 不识别哨兵）时
// 必须在 deadline 内返回 false，而不是 hang。
func TestAwaitConsumerReady_TimeoutFails(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := eventbus.NewMemoryEventBus()
	defer bus.Close()

	topic := fmt.Sprintf("test.ready.gate.unwired.%d", helper.GetTimestamp())
	// handler 不接 gate：probe 被消费但永远不放行（模拟未接线/消费路径故障）
	err := bus.SubscribeEnvelope(context.Background(), topic, func(ctx context.Context, envelope *eventbus.Envelope) error {
		return nil
	})
	if err != nil {
		t.Fatalf("SubscribeEnvelope failed: %v", err)
	}

	start := time.Now()
	if helper.AwaitConsumerReady(context.Background(), bus, topic, NewReadyGate(), 300*time.Millisecond) {
		t.Fatal("AwaitConsumerReady must return false when the probe is never witnessed")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("AwaitConsumerReady must respect its deadline, took %v", elapsed)
	}
}
