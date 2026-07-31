package eventbus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// maxInt64 返回切片最大值（pipeline mark-once 不变量校验用：最高提交位 = 最大 offset）。
func maxInt64(xs []int64) int64 {
	m := xs[0]
	for _, v := range xs[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// newPipelineHoldTestEventBus 构造最小可用的 kafkaEventBus 用于 consumeWithPipeline 单测：
//   - Enabled=true（走 partition pipeline 路径，consumeWithPipeline）
//   - HoldBackoff 由调用方指定（测试用小值以快速、确定性收尾）
//   - SessionTimeout=10s（newPartitionPipeline 经 consumerConfig().SessionTimeout 读取；
//     默认 FlushTimeout=4s < 5s=SessionTimeout/2 满足 validate 不变量）
//   - 真实 HollywoodActorPool（pipeline 经 p.pool.ProcessMessage 分发；handler 返回 nil →
//     Done <- nil → commitSuccess → marker.MarkMessage）
//
// HoldBackoff/WindowSize/FlushTimeout/DLQTimeout/StallWarnInterval 通过 pipelineConfig()
// （=applyPipelineDefaults）补默认；Enabled + HoldBackoff 是测试唯一显式设置的项，与生产读取路径一致。
func newPipelineHoldTestEventBus(t *testing.T, holdBackoff time.Duration) (*kafkaEventBus, *HollywoodActorPool) {
	t.Helper()
	pool := NewHollywoodActorPool(HollywoodActorPoolConfig{PoolSize: 8, InboxSize: 100, MaxRestarts: 3}, &NoOpActorPoolMetricsCollector{})
	eb := &kafkaEventBus{
		config: &KafkaConfig{
			Consumer: ConsumerConfig{
				SessionTimeout: 10 * time.Second,
				Pipeline:       PipelineConfig{Enabled: true, HoldBackoff: holdBackoff}, // Enabled=true → pipeline 路径
			},
		},
		logger:          zap.NewNop(),
		globalActorPool: pool,
		// activeTopicHandlers: 零值 sync.Map，直接可用
	}
	return eb, pool
}

// TestConsumeWithPipeline_NoSessionDrain_OnLateActivation_SingleMember 主回归（spec §5 A；review D1/D4/D5a）：
// consumeWithPipeline 的 nil 分支当前 drain 整个 session（read + MarkMessage 循环），对未激活 topic 静默丢失全部消息。
// 修复后必须 **hold 在 p.run 之外**（背压：不读 claim.Messages()、不提交），待激活后解析 wrapper 一次并进 p.run。
//
// 关键 N>=3（不是 1）：drain 错误实现在 N=1 时只丢失 1 条难以区分；用 N>=3 才能逼出「hold 期间不读 claim.Messages()
// 全 N 条被背压」这条 D1 铁律，并确认激活后全 N 条按序提交（0 丢失、0 越位）。
//
// 单成员 / 无 rebalance：fakeClaim 不是真实 consumer group，无 rebalance —— 显式建模生产常态
// （spec §8 P2#8/P1#2：rebalance 重投递会制造 FALSE pass，本测试排除该路径）。
func TestConsumeWithPipeline_NoSessionDrain_OnLateActivation_SingleMember(t *testing.T) {
	const topic = "domain.test.pipeline.late-activation"
	const holdBackoff = 5 * time.Millisecond
	eb, pool := newPipelineHoldTestEventBus(t, holdBackoff)
	defer pool.Stop()
	h := &preSubscriptionConsumerHandler{eventBus: eb}
	cfg := eb.pipelineConfig() // Enabled=true + HoldBackoff=5ms，其余默认

	offsets := []int64{10, 11, 12}
	msgs := make(chan *sarama.ConsumerMessage, len(offsets))
	for _, off := range offsets {
		msgs <- &sarama.ConsumerMessage{Topic: topic, Partition: 0, Offset: off, Value: []byte("payload")}
	}
	close(msgs) // hold 期间不读；激活后 p.run 读完全部 + nil → 排空在飞 → 干净返回

	var processed atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sess := &fakeSession{ctx: ctx}
	claim := &fakeClaim{msgs: msgs, topic: topic}

	done := make(chan error, 1)
	go func() { done <- h.consumeWithPipeline(ctx, sess, claim, cfg) }()

	// 1. hold 期间（topic 未激活）：ZERO MarkMessage。旧 drain 码此刻已读并提交 offset 10（甚至全 N）→ 丢失。
	<-time.After(30 * time.Millisecond) // >> holdBackoff，确认 hold 已轮询多轮
	assert.Empty(t, sess.marked, "hold 期间不得 MarkMessage（drain=静默丢失的根因；review D1/D5a）")

	// 2. 激活 topic → hold 唤醒 → resolveWrapper 一次性返回非 nil → 进 p.run → 按序处理全部 N 条 → mark-once。
	eb.activeTopicHandlers.Store(topic, &handlerWrapper{
		handler: func(_ context.Context, _ []byte) error {
			processed.Add(1)
			return nil
		},
		isEnvelope: false,
	})

	select {
	case err := <-done:
		assert.NoError(t, err, "close(msgs) 排空后应正常返回 nil")
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("consumeWithPipeline 未在激活后返回（hold 未唤醒？）")
	}

	// 3. 全部 N 条经 handler 处理；pipeline mark-once：advanceFrontier 推过连续前缀，标记最高位。
	// 完成顺序决定 marked 是 [10,11,12]（顺序）或仅 [12]（乱序汇合后一次推进）——两者皆正确，
	// 铁律不变量：max(marked) == 最大 offset（全 N 已提交、0 丢失、0 越位）。
	assert.Equal(t, int32(len(offsets)), processed.Load(), "全部 N 条必须经 handler 处理（drain 旧码此处=0）")
	require.NotEmpty(t, sess.marked, "pipeline 必须提交（mark-once 至少标记最高位）")
	assert.Equal(t, offsets[len(offsets)-1], maxInt64(sess.marked),
		"mark-once 最高提交位必须 = 最大 offset %d（全 N 已提交；hold 期间 N 条被背压，未读未丢）",
		offsets[len(offsets)-1])
}

// TestConsumeWithPipeline_ActivatedTopic_RunsImmediately D6' happy-path 回归（round-2 review）：
// topic 在 claim 开始前已激活 → 新增的 hold 分支绝不能增加延迟（resolveWrapper 一次性返回非 nil，for 循环零次）。
// 守护生产最高频路径：既有 pipeline 测试只覆盖 p.run，未覆盖 consumeWithPipeline 入口的 hold 分支。
// 断言：N 条处理 + 提交，且 elapsed ≪ HoldBackoff（hold 分支未被触发的正向证据）。
func TestConsumeWithPipeline_ActivatedTopic_RunsImmediately(t *testing.T) {
	const topic = "domain.test.pipeline.pre-activated"
	const holdBackoff = 200 * time.Millisecond // 故意放大：若 hold 被误触发，elapsed ≈ holdBackoff 一眼可辨
	eb, pool := newPipelineHoldTestEventBus(t, holdBackoff)
	defer pool.Stop()
	h := &preSubscriptionConsumerHandler{eventBus: eb}
	cfg := eb.pipelineConfig()

	offsets := []int64{50, 51, 52}
	msgs := make(chan *sarama.ConsumerMessage, len(offsets))
	for _, off := range offsets {
		msgs <- &sarama.ConsumerMessage{Topic: topic, Partition: 0, Offset: off, Value: []byte("payload")}
	}
	close(msgs)

	var processed atomic.Int32
	// 激活在 claim 开始之前：resolveWrapper 必须首次即返回非 nil → hold 循环零次
	eb.activeTopicHandlers.Store(topic, &handlerWrapper{
		handler: func(_ context.Context, _ []byte) error {
			processed.Add(1)
			return nil
		},
		isEnvelope: false,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sess := &fakeSession{ctx: ctx}
	claim := &fakeClaim{msgs: msgs, topic: topic}

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- h.consumeWithPipeline(ctx, sess, claim, cfg) }()

	select {
	case err := <-done:
		assert.NoError(t, err, "close(msgs) 排空后应正常返回 nil")
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("consumeWithPipeline 未及时返回（p.run 卡住？）")
	}
	elapsed := time.Since(start)

	assert.Equal(t, int32(len(offsets)), processed.Load(), "全部 N 条必须经 handler 处理")
	require.NotEmpty(t, sess.marked, "pipeline 必须提交（mark-once）")
	assert.Equal(t, offsets[len(offsets)-1], maxInt64(sess.marked),
		"mark-once 最高提交位必须 = 最大 offset %d（全 N 已提交）", offsets[len(offsets)-1])
	assert.Less(t, elapsed, holdBackoff, "已激活 topic 不得触发 hold（elapsed ≪ HoldBackoff=%s；got %s）", holdBackoff, elapsed)
}

// TestConsumeWithPipeline_CtxCancelDuringHold_ReturnsPromptly P2#9 session-release 回归：
// hold 期间 ctx 取消（session 结束 / claim 关闭）→ consumeWithPipeline 必须迅速返回 ctx.Err()
// （holdUntilActivated 的 select 有 ctx.Done 分支），绝不阻塞到 Rebalance.Timeout 60s 冻结所有 topic。
// 断言：返回迅速（< 500ms ≪ 60s）、返回 ctx.Canceled、0 MarkMessage（in-hand 未处理 → 下次会话重投递）。
func TestConsumeWithPipeline_CtxCancelDuringHold_ReturnsPromptly(t *testing.T) {
	const topic = "domain.test.pipeline.ctx-cancel"
	const holdBackoff = 5 * time.Millisecond
	eb, pool := newPipelineHoldTestEventBus(t, holdBackoff)
	defer pool.Stop()
	h := &preSubscriptionConsumerHandler{eventBus: eb}
	cfg := eb.pipelineConfig()

	msgs := make(chan *sarama.ConsumerMessage, 3)
	for _, off := range []int64{20, 21, 22} {
		msgs <- &sarama.ConsumerMessage{Topic: topic, Partition: 0, Offset: off, Value: []byte("payload")}
	}
	// 故意不 close(msgs)：只能经 ctx 取消退出，证「hold 返回 ctx.Err，不 drain」

	ctx, cancel := context.WithCancel(context.Background())
	sess := &fakeSession{ctx: ctx}
	claim := &fakeClaim{msgs: msgs, topic: topic}

	done := make(chan error, 1)
	go func() { done <- h.consumeWithPipeline(ctx, sess, claim, cfg) }()

	<-time.After(30 * time.Millisecond) // 确认 hold 已运转、topic 未激活
	assert.Empty(t, sess.marked, "hold 期间不得 MarkMessage")

	start := time.Now()
	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled, "ctx 取消后 hold 应返回 ctx.Err()")
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后 consumeWithPipeline 未及时返回（hold 阻塞到 Rebalance.Timeout 60s？）")
	}
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 500*time.Millisecond, "hold 必须在 ctx 取消后迅速返回（不阻塞到 Rebalance.Timeout 60s；P2#9）")
	assert.Empty(t, sess.marked, "取消后仍不得 MarkMessage：未处理消息下次会话重投递（at-least-once）")
}
