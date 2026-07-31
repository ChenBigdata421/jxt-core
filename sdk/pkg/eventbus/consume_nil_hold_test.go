package eventbus

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeClaim 实现 sarama.ConsumerGroupClaim 用到的子集（Messages + 元信息），方法集对齐
// sarama v1.46.0 的 ConsumerGroupClaim 接口（5 个方法）。仅用于 legacy ConsumeClaim 单测。
type fakeClaim struct {
	msgs      chan *sarama.ConsumerMessage
	topic     string
	partition int32
}

func (c *fakeClaim) Topic() string                            { return c.topic }
func (c *fakeClaim) Partition() int32                         { return c.partition }
func (c *fakeClaim) InitialOffset() int64                     { return 0 }
func (c *fakeClaim) HighWaterMarkOffset() int64               { return 0 }
func (c *fakeClaim) Messages() <-chan *sarama.ConsumerMessage { return c.msgs }

// newHoldTestEventBus 构造最小可用的 kafkaEventBus 用于 legacy ConsumeClaim 单测：
//   - Enabled=false（走 legacy 串行路径）
//   - HoldBackoff 由调用方指定（测试用小值以快速、确定性收尾）
//   - 零值 activeTopicHandlers（sync.Map 直接可用）、零值 roundRobinCounter（atomic 直接可用）
//   - 真实 HollywoodActorPool（processMessageWithKeyedPool 经此分发；成功后由调用方 goroutine MarkMessage）
//
// HoldBackoff 通过 pipelineConfig()（=applyPipelineDefaults）读取——非零显式值被保留（不会被默认覆盖），
// 与生产读取路径完全一致（kafka.go ConsumeClaim 的 hold 循环）。
func newHoldTestEventBus(t *testing.T, holdBackoff time.Duration) (*kafkaEventBus, *HollywoodActorPool) {
	t.Helper()
	pool := NewHollywoodActorPool(HollywoodActorPoolConfig{PoolSize: 8, InboxSize: 100, MaxRestarts: 3}, &NoOpActorPoolMetricsCollector{})
	eb := &kafkaEventBus{
		config: &KafkaConfig{
			Consumer: ConsumerConfig{
				Pipeline: PipelineConfig{HoldBackoff: holdBackoff}, // Enabled=false → legacy 路径
			},
		},
		logger:          zap.NewNop(),
		globalActorPool: pool,
		// activeTopicHandlers: 零值 sync.Map，直接可用
	}
	return eb, pool
}

// TestLegacyConsumeClaim_HoldsOnNilThenProcesses_NoLoss 主回归（spec §5 A；review D1/D4）：
// handler 未激活时到达的消息绝不能被 MarkMessage（静默丢失）。必须被 hold，待 handler 激活后处理。
//
// 关键 N>=3（不是 1）：read-and-discard 错误实现（读下一条并丢弃）在 N=1 时也会通过，
// 只有用 N>=3 才能逼出「hold 期间不读 claim.Messages()」这条 D1 铁律。
func TestLegacyConsumeClaim_HoldsOnNilThenProcesses_NoLoss(t *testing.T) {
	const topic = "domain.test.late-activation"
	const holdBackoff = 5 * time.Millisecond
	eb, pool := newHoldTestEventBus(t, holdBackoff)
	defer pool.Stop()
	h := &preSubscriptionConsumerHandler{eventBus: eb}

	offsets := []int64{10, 11, 12}
	msgs := make(chan *sarama.ConsumerMessage, len(offsets))
	for _, off := range offsets {
		msgs <- &sarama.ConsumerMessage{Topic: topic, Partition: 0, Offset: off, Value: []byte("payload")}
	}
	close(msgs) // 第 4 次读返回 nil → ConsumeClaim 处理完 N 条后干净返回

	var processed atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sess := &fakeSession{ctx: ctx}
	claim := &fakeClaim{msgs: msgs, topic: topic}

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	// 1. hold 期间（topic 未激活）：ZERO MarkMessage。旧 drain 码此刻会把 offset 10 提交掉（丢失）。
	<-time.After(30 * time.Millisecond) // >> holdBackoff，确认 hold 已轮询多轮
	assert.Empty(t, sess.marked, "hold 期间不得 MarkMessage（drain=静默丢失的根因；review D1）")

	// 2. 激活 topic → hold 唤醒 → 按序处理全部 N 条 → 每条恰好 MarkMessage 一次。
	eb.activeTopicHandlers.Store(topic, &handlerWrapper{
		handler: func(_ context.Context, _ []byte) error {
			processed.Add(1)
			return nil
		},
		isEnvelope: false,
	})

	select {
	case err := <-done:
		assert.NoError(t, err, "close(msgs) 后应正常返回 nil")
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("ConsumeClaim 未在激活后返回（hold 未唤醒？）")
	}

	// 3. 全部 N 条经 handler 处理 + 恰好 MarkMessage 一次（0 丢失、0 越位）。
	assert.Equal(t, int32(len(offsets)), processed.Load(), "全部 N 条必须经 handler 处理（drain 旧码此处=0）")
	require.Len(t, sess.marked, len(offsets), "全部 N 条必须 MarkMessage 恰好一次（0 丢失）")
	assert.Equal(t, offsets, sess.marked, "mark 序列必须按 offset 升序、无空洞（trailing msgs 在 hold 期间被 sarama 背压，未读未丢）")
}

// TestLegacyConsumeClaim_DeactivateRaceDuringHold_ReholdsNotSkips D3' 回归（round-2 review，CRITICAL）：
// hold 在激活时唤醒，但 topic 在 outer re-check（kafka.go ConsumeClaim 的 `for !exists` 重 Load）前被并发去激活
// （deactivateTopicHandler / map Delete）。此时 `for !exists` 必须回到 hold——绝不跳过 in-hand 消息：
// 跳过 + 后续 MarkMessage 越位提交 = 本次事故的静默丢失类别。
//
// 确定性边界（已向计划报告）：holdUntilActivated 仅在 Load 命中激活时返回 nil；其返回到 ConsumeClaim 的
// outer re-Load 之间是纳秒级 TOCTOU 窗口，外部 goroutine 无法稳定命中。故本测试走真实 ConsumeClaim +
// 反复 Store/Delete 抖动以最大化命中机会，并断言铁律不变量：每条 in-hand 消息要么被 handler 真正处理
// （processed+1）+ MarkMessage 恰好一次，要么被 hold 保留（绝不越位跳过）。drain 旧码或任何 skip-past
// bug 都会让 processed != 1 或 marked 含未被处理的 offset → 失败。
func TestLegacyConsumeClaim_DeactivateRaceDuringHold_ReholdsNotSkips(t *testing.T) {
	const holdBackoff = 5 * time.Millisecond
	const topicCount = 5

	var totalProcessed atomic.Int32
	var totalMarked atomic.Int32
	for i := 0; i < topicCount; i++ {
		topic := fmt.Sprintf("domain.test.race.%d", i)
		eb, pool := newHoldTestEventBus(t, holdBackoff)
		h := &preSubscriptionConsumerHandler{eventBus: eb}

		const off int64 = 100
		msgs := make(chan *sarama.ConsumerMessage, 1)
		msgs <- &sarama.ConsumerMessage{Topic: topic, Partition: 0, Offset: off, Value: []byte("payload")}
		close(msgs)

		var processed atomic.Int32
		noopHandler := func(_ context.Context, _ []byte) error {
			processed.Add(1)
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		sess := &fakeSession{ctx: ctx}
		claim := &fakeClaim{msgs: msgs, topic: topic}

		done := make(chan error, 1)
		go func() { done <- h.ConsumeClaim(sess, claim) }()
		<-time.After(15 * time.Millisecond) // 确认 hold 已运转、未激活

		// 抖动：短暂激活后立即去激活，制造 inner-wake / outer-recheck 之间的竞态窗口；
		// 收尾稳定激活让 hold 必然处理 in-hand 消息（避免悬挂）。
		churnDone := make(chan struct{})
		go func() {
			defer close(churnDone)
			for j := 0; j < 15; j++ {
				eb.activeTopicHandlers.Store(topic, &handlerWrapper{handler: noopHandler, isEnvelope: false})
				time.Sleep(1 * time.Millisecond)
				eb.deactivateTopicHandler(topic)
				time.Sleep(1 * time.Millisecond)
			}
			eb.activeTopicHandlers.Store(topic, &handlerWrapper{handler: noopHandler, isEnvelope: false})
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cancel()
			pool.Stop()
			<-churnDone
			t.Fatalf("topic %s: ConsumeClaim 未在稳定激活后返回（hold 未唤醒？）", topic)
		}
		<-churnDone // 让抖动 goroutine 收尾，避免访问已 Stop 的 pool
		cancel()    // 释放可能仍持有的 ctx（done 已收到，ConsumeClaim 已返回）
		pool.Stop()

		// 铁律：恰好处理一次、恰好 MarkMessage 一次（in-hand offset）。不得越位跳过、不得丢失。
		assert.Equal(t, int32(1), processed.Load(), "topic %s: in-hand 消息必须被 handler 处理恰好一次（不得 skip-past）", topic)
		require.Len(t, sess.marked, 1, "topic %s: 恰好一次 MarkMessage（in-hand offset %d）", topic, off)
		assert.Equal(t, off, sess.marked[0], "topic %s: 被提交的必须是被处理的同一 offset（不得越位）", topic)
		totalProcessed.Add(processed.Load())
		totalMarked.Add(int32(len(sess.marked)))
	}
	assert.Equal(t, int32(topicCount), totalProcessed.Load(), "全部 topic：无丢失处理")
	assert.Equal(t, int32(topicCount), totalMarked.Load(), "全部 topic：无丢失 MarkMessage")
}

// TestLegacyConsumeClaim_NeverActivated_StallsNotDrains D4 负向不变量：
// handler 永不激活的 topic 必须 STALL（分区背压、0 MarkMessage、0 丢失），不得 drain。
// ctx 取消后 hold 迅速返回 ctx.Err()（sarama session release 不阻塞到 Rebalance.Timeout 60s，review P2#9）。
func TestLegacyConsumeClaim_NeverActivated_StallsNotDrains(t *testing.T) {
	const topic = "domain.test.never-activated"
	const holdBackoff = 5 * time.Millisecond
	eb, pool := newHoldTestEventBus(t, holdBackoff)
	defer pool.Stop()
	h := &preSubscriptionConsumerHandler{eventBus: eb}

	msgs := make(chan *sarama.ConsumerMessage, 3)
	for _, off := range []int64{20, 21, 22} {
		msgs <- &sarama.ConsumerMessage{Topic: topic, Partition: 0, Offset: off, Value: []byte("payload")}
	}
	// 故意不 close(msgs)：claim 只能经 ctx 取消退出，证「stall 而非 drain」。

	ctx, cancel := context.WithCancel(context.Background())
	sess := &fakeSession{ctx: ctx}
	claim := &fakeClaim{msgs: msgs, topic: topic}

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()

	// handler 永不激活 → stall（背压）：0 MarkMessage、0 丢失。旧 drain 码此刻已把 20/21/22 全部提交（丢失）。
	<-time.After(30 * time.Millisecond) // >> holdBackoff
	assert.Empty(t, sess.marked, "handler 永不激活：必须 stall（0 MarkMessage），不得 drain")

	// ctx 取消 → hold 经 select 的 ctx.Done 分支迅速返回 ctx.Err()。
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled, "ctx 取消后 hold 应返回 ctx.Err()")
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后 ConsumeClaim 未及时返回（hold 阻塞到 Rebalance.Timeout 60s？）")
	}
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 500*time.Millisecond, "hold 必须在 ctx 取消后迅速返回（不阻塞到 Rebalance.Timeout 60s；P2#9）")
	assert.Empty(t, sess.marked, "取消后仍不得 MarkMessage：in-hand 消息未处理 → 下次会话重投递（at-least-once）")
}
