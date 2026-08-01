package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

// newHoldTestEventBus 构造最小可用的 kafkaEventBus 用于 ConsumeClaim 单测（两个测试文件共用）：
//   - enabled=false 走 legacy 串行路径（ConsumeClaim）；enabled=true 走 partition pipeline 路径（consumeWithPipeline）
//   - HoldBackoff 由调用方指定（测试用小值以快速、确定性收尾）
//   - SessionTimeout=10s（仅流水线路径消费：newPartitionPipeline 经 consumerConfig().SessionTimeout 读取，
//     默认 FlushTimeout=4s < 5s=SessionTimeout/2 满足 validate 不变量；legacy 路径不读取，无害）
//   - 零值 activeTopicHandlers（sync.Map 直接可用）、零值 roundRobinCounter（atomic 直接可用）
//   - 真实 HollywoodActorPool（processMessageWithKeyedPool / p.run 经此分发；成功后 MarkMessage）
//
// HoldBackoff 通过 pipelineConfig()（=applyPipelineDefaults）读取——非零显式值被保留（不会被默认覆盖），
// 与生产读取路径完全一致（kafka.go ConsumeClaim 的 hold 循环）。
func newHoldTestEventBus(t *testing.T, enabled bool, holdBackoff time.Duration) (*kafkaEventBus, *HollywoodActorPool) {
	t.Helper()
	pool := NewHollywoodActorPool(HollywoodActorPoolConfig{PoolSize: 8, InboxSize: 100, MaxRestarts: 3}, &NoOpActorPoolMetricsCollector{})
	eb := &kafkaEventBus{
		config: &KafkaConfig{
			Consumer: ConsumerConfig{
				SessionTimeout: 10 * time.Second,
				Pipeline:       PipelineConfig{Enabled: enabled, HoldBackoff: holdBackoff},
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
	eb, pool := newHoldTestEventBus(t, false, holdBackoff)
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
// hold 在激活时唤醒，但 topic 在唤醒后、处理前被并发去激活（deactivateTopicHandler / map Delete）。
// 修复语义（review G4 收敛后）：holdUntilActivated 内部 Load 命中即捕获并返回 wrapper——map 删除只
// 移除注册，已捕获的 *handlerWrapper 仍有效，故处理用捕获的 wrapper 继续；绝不跳过 in-hand 消息：
// 跳过 + 后续 MarkMessage 越位提交 = 本次事故的静默丢失类别。
//
// 确定性边界（已向计划报告，review G3 注释诚实化）：hold 唤醒到处理之间是纳秒级 TOCTOU 窗口，
// 外部 goroutine 无法稳定命中，故 D3' 竞态分支无法确定性单测（本测试是 happy-path 不变量守卫 +
// 竞态命中最大化，不是 TOCTOU 的确定性证明——该分支的正确性由代码结构保证并在 code review 层覆盖）。
// 测试走真实 ConsumeClaim + 反复 Store/Delete 抖动以最大化命中机会，并断言铁律不变量：每条 in-hand
// 消息要么被 handler 真正处理（processed+1）+ MarkMessage 恰好一次，要么被 hold 保留（绝不越位跳过）。
// drain 旧码或任何 skip-past bug 都会让 processed != 1 或 marked 含未被处理的 offset → 失败。若要
// 确定性覆盖 D3' 分支，需注入 activationChecker 接缝（本轮未做，列为待办）。
func TestLegacyConsumeClaim_DeactivateRaceDuringHold_ReholdsNotSkips(t *testing.T) {
	const holdBackoff = 5 * time.Millisecond
	const topicCount = 5

	var totalProcessed atomic.Int32
	var totalMarked atomic.Int32
	for i := 0; i < topicCount; i++ {
		topic := fmt.Sprintf("domain.test.race.%d", i)
		eb, pool := newHoldTestEventBus(t, false, holdBackoff)
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
	eb, pool := newHoldTestEventBus(t, false, holdBackoff)
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
		assert.NoError(t, err, "ctx 取消后 ConsumeClaim 应正常返回 nil（G8：正常关停非错误，与外层 select 一致）")
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后 ConsumeClaim 未及时返回（hold 阻塞到 Rebalance.Timeout 60s？）")
	}
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 500*time.Millisecond, "hold 必须在 ctx 取消后迅速返回（不阻塞到 Rebalance.Timeout 60s；P2#9）")
	assert.Empty(t, sess.marked, "取消后仍不得 MarkMessage：in-hand 消息未处理 → 下次会话重投递（at-least-once）")
}

// TestHoldUntilActivated_EmitsStallSignal G2 回归（review 2026-08-01）：hold 是分区停滞态——
// 永不激活的 topic 冻结分区但 group 保持健康（心跳在独立 goroutine），故 hold 必须发信号，
// 绝不静默卡死（lag=0 / group=Stable 面具）：进入时一次性 Warn + monotonic StallEnter 上升沿，
// hold 期间 gauge 按真实时长爬升，结束（激活/取消）归零。
func TestHoldUntilActivated_EmitsStallSignal(t *testing.T) {
	const topic = "domain.test.stall-signal"
	const partition = int32(7)
	const holdBackoff = 5 * time.Millisecond

	type gaugeCall struct {
		topic     string
		partition int32
		seconds   float64
	}
	var mu sync.Mutex
	var enters []int32
	var gauges []gaugeCall
	StallEnterReporter = func(_ string, p int32) {
		mu.Lock()
		defer mu.Unlock()
		enters = append(enters, p)
	}
	StallReporter = func(t string, p int32, s float64) {
		mu.Lock()
		defer mu.Unlock()
		gauges = append(gauges, gaugeCall{t, p, s})
	}
	defer func() { StallReporter = nil; StallEnterReporter = nil }()

	core, logs := observer.New(zap.WarnLevel)
	eb := &kafkaEventBus{logger: zap.New(core)}
	h := &preSubscriptionConsumerHandler{eventBus: eb}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := h.holdUntilActivated(ctx, topic, partition, holdBackoff)
		done <- err
	}()

	// 1. hold 期间：enter 恰好一次、gauge 已上报且秒数 > 0、Warn 恰好一条。
	<-time.After(20 * time.Millisecond) // >> holdBackoff
	mu.Lock()
	require.Len(t, enters, 1, "hold 进入必须触发 StallEnterReporter 恰好一次")
	assert.Equal(t, partition, enters[0])
	require.NotEmpty(t, gauges, "hold 期间必须持续上报 stall gauge")
	last := gauges[len(gauges)-1]
	mu.Unlock()
	assert.Equal(t, topic, last.topic)
	assert.Equal(t, partition, last.partition)
	assert.Greater(t, last.seconds, float64(0), "gauge 必须反映真实 hold 时长（>0）")
	assert.Equal(t, 1, logs.FilterMessage("topic consumed but handler not activated; holding partition (backpressure, no commit) until activation or session end").Len(),
		"Warn 必须恰好一条（一次性，不随 100ms 轮询刷屏）")

	// 2. 激活 → hold 返回 → gauge 归零。
	eb.activeTopicHandlers.Store(topic, &handlerWrapper{})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("hold 未在激活后返回")
	}
	mu.Lock()
	require.NotEmpty(t, gauges)
	assert.Equal(t, float64(0), gauges[len(gauges)-1].seconds, "hold 结束必须归零 gauge（激活路径）")
	mu.Unlock()
}

// 注：GP1（envelope 重试失败终止 claim 防越位）于 2026-08-01 回滚——该方案经集成测试实验证明
// 有害：sarama 单 claim 语义下终止 claim 会阻断分区后续正常消息（reliability
// TestKafkaFaultIsolationWithHighLoad 收到 31/1000，baseline 1008/1000）。legacy 路径的重试失败
// 越位提交是已知限制（见 kafka.go ConsumeClaim 注释），正确解法是 pipeline 路径（DLQ + Strategy A）。
