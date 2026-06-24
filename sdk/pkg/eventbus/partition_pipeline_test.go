package eventbus

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPipelineConfig_Defaults 验证 PipelineConfig 默认值（关闭、合理 windowSize、flush 受 sessionTimeout 约束）
func TestPipelineConfig_Defaults(t *testing.T) {
	t.Run("零值默认关闭且 windowSize 合理", func(t *testing.T) {
		cfg := defaultPipelineConfig()
		assert.False(t, cfg.Enabled, "默认必须关闭，灰度才显式开")
		assert.GreaterOrEqual(t, cfg.WindowSize, 1)
		assert.Less(t, cfg.WindowSize, 1024)
	})

	t.Run("flush 超时必须小于 sessionTimeout/2", func(t *testing.T) {
		sessionTimeout := 10 * time.Second
		cfg := defaultPipelineConfig()
		require.NoError(t, cfg.validate(sessionTimeout))
		assert.Less(t, cfg.FlushTimeout, sessionTimeout/2)
	})

	t.Run("flush 超时过大应报错", func(t *testing.T) {
		sessionTimeout := 10 * time.Second
		cfg := PipelineConfig{Enabled: true, WindowSize: 16, FlushTimeout: 9 * time.Second, DLQTimeout: 30 * time.Second}
		err := cfg.validate(sessionTimeout)
		assert.Error(t, err, "flushTimeout 必须 < sessionTimeout/2")
	})

	t.Run("windowSize 非法应报错", func(t *testing.T) {
		cfg := PipelineConfig{Enabled: true, WindowSize: 0, FlushTimeout: 5 * time.Second, DLQTimeout: 30 * time.Second}
		assert.Error(t, cfg.validate(10*time.Second))
	})
}

// TestDecideCommitable 验证三分支提交判定（无网络，纯逻辑）
func TestDecideCommitable(t *testing.T) {
	t.Run("成功 → commitSuccess", func(t *testing.T) {
		e := &inflightEntry{isEnvelope: true}
		assert.Equal(t, commitSuccess, decideCommitable(e, nil))
	})
	t.Run("普通消息失败 → commitRegularFail（at-most-once，丢弃）", func(t *testing.T) {
		e := &inflightEntry{isEnvelope: false}
		assert.Equal(t, commitRegularFail, decideCommitable(e, errors.New("boom")))
	})
	t.Run("Envelope 失败 → commitEnvelopeFail（走异步 DLQ）", func(t *testing.T) {
		e := &inflightEntry{isEnvelope: true}
		assert.Equal(t, commitEnvelopeFail, decideCommitable(e, errors.New("boom")))
	})
}

// TestAdvanceFrontier_T1 乱序完成无空洞：completion 乱序到达，但提交严格升序、max==连续前沿
func TestAdvanceFrontier_T1(t *testing.T) {
	t.Run("乱序完成只推进连续前缀", func(t *testing.T) {
		inflight := map[int64]*inflightEntry{}
		var frontier int64 = 10
		mk := func(off int64) *sarama.ConsumerMessage { return &sarama.ConsumerMessage{Offset: off} }

		// 乱序：12 先完成，但队头 10 未完成 → 连续前缀被队头阻塞，不应推进
		inflight[12] = &inflightEntry{msg: mk(12), settled: true, commitable: true}
		last := advanceFrontier(inflight, &frontier)
		assert.Nil(t, last, "队头 10 未完成：12 单独完成不应让 frontier 越过 10")
		assert.Equal(t, int64(10), frontier, "队头阻塞，不推进")

		// 10、11 随后完成 → 连续前缀 10/11/12 全部就绪 → 一次性推进到 13，mark-once 最高位 12
		inflight[10] = &inflightEntry{msg: mk(10), settled: true, commitable: true}
		inflight[11] = &inflightEntry{msg: mk(11), settled: true, commitable: true}
		last = advanceFrontier(inflight, &frontier)
		require.NotNil(t, last)
		assert.Equal(t, int64(12), last.Offset, "mark-once：连续 10/11/12 只 Mark 最高位 12")
		assert.Equal(t, int64(13), frontier, "连续前缀推进到 13")
		assert.Empty(t, inflight, "已推进的 entry 全部删除")
	})

	t.Run("DLQ 进行中不越过", func(t *testing.T) {
		inflight := map[int64]*inflightEntry{}
		var frontier int64 = 10
		mk := func(off int64) *sarama.ConsumerMessage { return &sarama.ConsumerMessage{Offset: off} }
		inflight[10] = &inflightEntry{msg: mk(10), settled: true, dlqPending: true} // 10 在 DLQ 中
		inflight[11] = &inflightEntry{msg: mk(11), settled: true, commitable: true}
		last := advanceFrontier(inflight, &frontier)
		assert.Nil(t, last, "队头 10 的 dlqPending 阻挡，不应推进")
		assert.Equal(t, int64(10), frontier)
	})

	t.Run("纯失败 Envelope（commitable=false）阻塞前沿", func(t *testing.T) {
		inflight := map[int64]*inflightEntry{}
		var frontier int64 = 10
		mk := func(off int64) *sarama.ConsumerMessage { return &sarama.ConsumerMessage{Offset: off} }
		inflight[10] = &inflightEntry{msg: mk(10), settled: true, commitable: false}
		last := advanceFrontier(inflight, &frontier)
		assert.Nil(t, last)
		assert.Equal(t, int64(10), frontier, "策略 A：队头不可提交 → 阻塞")
	})
}

// TestNewPartitionPipeline_BufferInvariant buffer 必须 >= windowSize 且运行期强制
func TestNewPartitionPipeline_BufferInvariant(t *testing.T) {
	t.Run("channel 容量 == windowSize", func(t *testing.T) {
		cfg := PipelineConfig{Enabled: true, WindowSize: 32, FlushTimeout: 4 * time.Second, DLQTimeout: 30 * time.Second}
		p, compCh, dlqDoneCh := newPartitionPipeline(cfg, 10*time.Second)
		assert.Equal(t, 32, cap(compCh))
		assert.Equal(t, 32, cap(dlqDoneCh))
		assert.Equal(t, 32, p.cfg.WindowSize)
	})

	t.Run("flush 超时 >= sessionTimeout/2 应 panic/报错", func(t *testing.T) {
		cfg := PipelineConfig{Enabled: true, WindowSize: 8, FlushTimeout: 9 * time.Second, DLQTimeout: 30 * time.Second}
		assert.Panics(t, func() { _, _, _ = newPartitionPipeline(cfg, 10*time.Second) })
	})
}

// TestForwardCompletion_T17 bridge 非阻塞 drain：done 就绪 + ctx 同时触发，必转发（不随机丢）
func TestForwardCompletion_T17(t *testing.T) {
	t.Run("done 已就绪时 ctx 同时触发仍必转发且 canceled=true", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		done <- nil // handler 已完成（done 就绪）
		compCh := make(chan completion, 4)

		cancel() // ctx 立刻取消，与 done 就绪同时
		forwardCompletion(ctx, 7, done, compCh)

		select {
		case c := <-compCh:
			assert.Equal(t, int64(7), c.offset)
			assert.Nil(t, c.err)
			assert.True(t, c.canceled, "ctx 取消后补捞转发 → canceled=true")
		default:
			t.Fatal("completion 被丢弃：竞态未修复")
		}
	})

	t.Run("done 空 + ctx 取消 → 不转发（留待重投递）", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1) // 空：handler 仍在跑
		compCh := make(chan completion, 4)
		cancel()
		forwardCompletion(ctx, 7, done, compCh)
		select {
		case <-compCh:
			t.Fatal("handler 仍在跑时不应转发")
		default:
		}
	})

	t.Run("正常完成 → canceled=false", func(t *testing.T) {
		ctx := context.Background()
		done := make(chan error, 1)
		done <- errors.New("handler err")
		compCh := make(chan completion, 4)
		forwardCompletion(ctx, 9, done, compCh)
		c := <-compCh
		assert.False(t, c.canceled)
		assert.Error(t, c.err)
	})
}

// fakeDLQ 受控的 DLQSender：可设定延迟、结果、是否被调用。
type fakeDLQ struct {
	delay time.Duration
	ok    bool
	calls int32
}

func (f *fakeDLQ) Send(ctx context.Context, msg *sarama.ConsumerMessage) bool {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return false
		}
	}
	return f.ok
}

// TestSendDLQ 异步 DLQ：独立 ctx、结果经 dlqDoneCh 回送
func TestSendDLQ(t *testing.T) {
	t.Run("成功 → ok=true（T13 成功路）", func(t *testing.T) {
		p := &partitionPipeline{cfg: PipelineConfig{DLQTimeout: time.Second}, dlq: &fakeDLQ{ok: true}}
		dlqDoneCh := make(chan dlqResult, 4)
		msg := &sarama.ConsumerMessage{Offset: 5}
		sendDLQ(p, 5, msg, dlqDoneCh)
		r := <-dlqDoneCh
		assert.Equal(t, int64(5), r.offset)
		assert.True(t, r.ok)
	})

	t.Run("失败 → ok=false（T13 失败路 → 策略 A 阻塞）", func(t *testing.T) {
		p := &partitionPipeline{cfg: PipelineConfig{DLQTimeout: time.Second}, dlq: &fakeDLQ{ok: false}}
		dlqDoneCh := make(chan dlqResult, 4)
		sendDLQ(p, 5, &sarama.ConsumerMessage{Offset: 5}, dlqDoneCh)
		r := <-dlqDoneCh
		assert.False(t, r.ok)
	})

	t.Run("T15 用独立 ctx：session ctx 取消不打断在飞 DLQ", func(t *testing.T) {
		// dlqTimeout 1s，DLQ 延迟 50ms。即便外层 session ctx 已取消，DLQ 仍应跑完返回 ok。
		p := &partitionPipeline{cfg: PipelineConfig{DLQTimeout: time.Second}, dlq: &fakeDLQ{ok: true, delay: 50 * time.Millisecond}}
		dlqDoneCh := make(chan dlqResult, 4)
		sendDLQ(p, 5, &sarama.ConsumerMessage{Offset: 5}, dlqDoneCh)
		r := <-dlqDoneCh
		assert.True(t, r.ok, "独立 ctx 使 DLQ 不被 session 取消腰斩")
	})
}

// fakeMarker 记录 MarkMessage 的调用序列（验证 mark-once 与升序）。
type fakeMarker struct {
	marked []int64
}

func (m *fakeMarker) MarkMessage(msg *sarama.ConsumerMessage, _ string) {
	m.marked = append(m.marked, msg.Offset)
}

// TestFlush_T4_T11_T14 限时冲刷、canceled 正向信号、不新起 DLQ
func TestFlush_T4_T11_T14(t *testing.T) {
	t.Run("T14 冲刷期 Envelope 业务失败不新起 DLQ，置 commitable=false", func(t *testing.T) {
		inflight := map[int64]*inflightEntry{}
		var frontier int64 = 10
		mk := func(o int64) *sarama.ConsumerMessage { return &sarama.ConsumerMessage{Offset: o} }
		inflight[10] = &inflightEntry{msg: mk(10), isEnvelope: true}
		compCh := make(chan completion, 4)
		dlqDoneCh := make(chan dlqResult, 4)
		compCh <- completion{offset: 10, err: errors.New("envelope fail"), canceled: false}

		marker := &fakeMarker{}
		flush(marker, inflight, &frontier, compCh, dlqDoneCh, noopAlerter{}, 200*time.Millisecond)

		assert.False(t, inflight[10].commitable, "冲刷期 envelope 失败 → commitable=false 留待重投递")
		assert.Equal(t, int64(10), frontier, "不推进")
		assert.Empty(t, marker.marked, "不提交")
	})

	t.Run("ctx 取消（canceled=true）不当业务失败 → commitable=false 留待重投递", func(t *testing.T) {
		inflight := map[int64]*inflightEntry{}
		var frontier int64 = 10
		inflight[10] = &inflightEntry{msg: &sarama.ConsumerMessage{Offset: 10}, isEnvelope: true}
		compCh := make(chan completion, 4)
		compCh <- completion{offset: 10, err: errors.New("grpc canceled"), canceled: true} // 正向信号，非错误类型
		dlqDoneCh := make(chan dlqResult, 4)

		marker := &fakeMarker{}
		flush(marker, inflight, &frontier, compCh, dlqDoneCh, noopAlerter{}, 200*time.Millisecond)

		assert.False(t, inflight[10].commitable)
		assert.Equal(t, int64(10), frontier)
	})

	t.Run("T11 在飞 hang + flush 超时不死等", func(t *testing.T) {
		inflight := map[int64]*inflightEntry{}
		var frontier int64 = 10
		inflight[10] = &inflightEntry{msg: &sarama.ConsumerMessage{Offset: 10}} // 永不 settle
		compCh := make(chan completion, 4)                                      // 空，无 completion 到达
		dlqDoneCh := make(chan dlqResult, 4)

		start := time.Now()
		flush(&fakeMarker{}, inflight, &frontier, compCh, dlqDoneCh, noopAlerter{}, 100*time.Millisecond)
		elapsed := time.Since(start)
		assert.Less(t, elapsed, 500*time.Millisecond, "必须在 timeout 内返回，不被 hang 卡死")
		assert.Equal(t, int64(10), frontier, "未 settle → 不推进 → 留待重投递")
	})

	t.Run("已 settle 的连续前缀在 flush 内提交（mark-once）", func(t *testing.T) {
		inflight := map[int64]*inflightEntry{}
		var frontier int64 = 10
		mk := func(o int64) *sarama.ConsumerMessage { return &sarama.ConsumerMessage{Offset: o} }
		inflight[10] = &inflightEntry{msg: mk(10)}
		inflight[11] = &inflightEntry{msg: mk(11)}
		compCh := make(chan completion, 4)
		compCh <- completion{offset: 11, err: nil, canceled: false}
		compCh <- completion{offset: 10, err: nil, canceled: false}
		dlqDoneCh := make(chan dlqResult, 4)

		marker := &fakeMarker{}
		// ⭐ P2-4：timer 取 1s（远大于两次即时读）；flush 在 unresolved()==0 时退出、timer 不会触发——确定性，无竞态窗。
		flush(marker, inflight, &frontier, compCh, dlqDoneCh, noopAlerter{}, 1*time.Second)
		assert.Equal(t, []int64{11}, marker.marked, "mark-once：连续 10/11 只 Mark 最高位 11")
		assert.Equal(t, int64(12), frontier)
	})
}

// fakePool 记录提交的 AggregateMessage，供测试按需驱动其 Done chan（模拟乱序/失败/成功）。
type fakePool struct {
	submitted []*AggregateMessage
}

func (f *fakePool) ProcessMessage(_ context.Context, msg *AggregateMessage) error {
	f.submitted = append(f.submitted, msg)
	return nil
}

func newPipelineForTest(windowSize int) (*partitionPipeline, chan completion, chan dlqResult) {
	cfg := PipelineConfig{Enabled: true, WindowSize: windowSize, FlushTimeout: 100 * time.Millisecond, DLQTimeout: 200 * time.Millisecond}
	p, compCh, dlqDoneCh := newPartitionPipeline(cfg, 10*time.Second)
	p.pool = &fakePool{}
	p.buildAggMsg = func(m *sarama.ConsumerMessage) *AggregateMessage {
		return &AggregateMessage{Topic: m.Topic, Partition: m.Partition, Offset: m.Offset, Value: m.Value,
			Done: make(chan error, 1), IsEnvelope: true}
	}
	return p, compCh, dlqDoneCh
}

// recordingAlerter 记录是否触发毒消息告警（策略 A）。
type recordingAlerter struct{ called bool }

func (a *recordingAlerter) AlertPoisonMessage(*sarama.ConsumerMessage) { a.called = true }

// TestRun_T2 envelope 失败处理：DLQ 成功路 + 【回归·强制】队头阻塞时后续成功不得泄漏提交
func TestRun_T2(t *testing.T) {
	t.Run("DLQ 成功 → 失败 offset 经 DLQ 后推进", func(t *testing.T) {
		p, compCh, dlqDoneCh := newPipelineForTest(8)
		pool := p.pool.(*fakePool)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		msgs := make(chan *sarama.ConsumerMessage, 4)
		msgs <- &sarama.ConsumerMessage{Offset: 10}
		close(msgs)

		marker := &fakeMarker{}
		p.dlq = &fakeDLQ{ok: true} // offset 10 失败但 DLQ 成功

		done := make(chan struct{})
		go func() { _ = p.run(ctx, msgs, marker, compCh, dlqDoneCh); close(done) }()

		<-time.After(50 * time.Millisecond)
		require.NotEmpty(t, pool.submitted)
		pool.submitted[0].Done <- errors.New("handler fail") // 10 失败
		<-time.After(150 * time.Millisecond)                 // 等 DLQ 回送 + 推进

		cancel()
		<-done
		// 10 经 DLQ 成功 → commitable 翻 true → 推进并提交：marker 必含 10（证明 DLQ 成功路把失败 offset 救回并提交，非静默跳过）
		require.Contains(t, marker.marked, int64(10), "DLQ 成功后 offset 10 必须被提交")
	})

	// ⭐ 回归守护（IRON RULE）：队头失败被阻塞时，后续成功不得越过队头提交——这是本改动要修的静默丢数据 bug。
	t.Run("队头阻塞·后续成功不泄漏提交（无跳过守护）", func(t *testing.T) {
		p, compCh, dlqDoneCh := newPipelineForTest(8)
		pool := p.pool.(*fakePool)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		msgs := make(chan *sarama.ConsumerMessage, 4)
		msgs <- &sarama.ConsumerMessage{Offset: 10} // 队头 envelope
		msgs <- &sarama.ConsumerMessage{Offset: 11} // 后续 envelope
		close(msgs)

		marker := &fakeMarker{}
		p.dlq = &fakeDLQ{ok: false} // 队头 10 失败 + DLQ 失败 → 策略 A 永久阻塞

		done := make(chan struct{})
		go func() { _ = p.run(ctx, msgs, marker, compCh, dlqDoneCh); close(done) }()

		<-time.After(50 * time.Millisecond)
		require.Len(t, pool.submitted, 2)
		pool.submitted[0].Done <- errors.New("head fail") // 10 失败 → DLQ 失败 → 阻塞前沿
		<-time.After(150 * time.Millisecond)
		pool.submitted[1].Done <- nil // 11 成功
		<-time.After(150 * time.Millisecond)

		cancel()
		<-done
		// 关键断言：11 虽成功，但队头 10 阻塞 → 11 不得被提交（连续前缀提交）。对照旧路径会静默跳过 10。
		assert.Empty(t, marker.marked, "队头阻塞时，后续成功不得泄漏一次越过队头的提交（无静默跳过）")
	})
}

// TestRun_T3 envelope 失败 + DLQ 失败 → 策略 A：前沿永久阻塞、强告警、不 re-submit
func TestRun_T3(t *testing.T) {
	p, compCh, dlqDoneCh := newPipelineForTest(8)
	pool := p.pool.(*fakePool)
	alerter := &recordingAlerter{}
	p.alert = alerter
	p.dlq = &fakeDLQ{ok: false} // DLQ 失败

	ctx, cancel := context.WithCancel(context.Background())
	msgs := make(chan *sarama.ConsumerMessage, 4)
	msgs <- &sarama.ConsumerMessage{Offset: 10}
	close(msgs)
	marker := &fakeMarker{}

	done := make(chan struct{})
	go func() { _ = p.run(ctx, msgs, marker, compCh, dlqDoneCh); close(done) }()

	<-time.After(50 * time.Millisecond)
	pool.submitted[0].Done <- errors.New("handler fail")
	<-time.After(200 * time.Millisecond) // 等 DLQ 失败回送 + 告警

	cancel()
	<-done

	assert.True(t, alerter.called, "策略 A：DLQ 失败必须强告警")
	assert.Empty(t, marker.marked, "前沿阻塞，不提交毒消息")
}

// TestRun_Backpressure 窗口满则停读，腾位后才提交新消息（P3·次要，非阻塞）
func TestRun_Backpressure(t *testing.T) {
	p, compCh, dlqDoneCh := newPipelineForTest(2) // windowSize=2
	pool := p.pool.(*fakePool)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msgs := make(chan *sarama.ConsumerMessage, 4)
	msgs <- &sarama.ConsumerMessage{Offset: 0}
	msgs <- &sarama.ConsumerMessage{Offset: 1}
	msgs <- &sarama.ConsumerMessage{Offset: 2} // 第 3 条：窗口满时应未被提交
	close(msgs)
	marker := &fakeMarker{}

	done := make(chan struct{})
	go func() { _ = p.run(ctx, msgs, marker, compCh, dlqDoneCh); close(done) }()

	<-time.After(50 * time.Millisecond)
	assert.Len(t, pool.submitted, 2, "窗口满（2）：第 3 条不得被 ProcessMessage")
	pool.submitted[0].Done <- nil // 腾一个位
	<-time.After(50 * time.Millisecond)
	assert.Len(t, pool.submitted, 3, "腾位后才读第 3 条")
	pool.submitted[1].Done <- nil
	pool.submitted[2].Done <- nil
	<-time.After(100 * time.Millisecond)
	cancel()
	<-done
}

// TestRun_T5 同聚合多在飞仍按 offset 升序执行（扩展现有 OrderGuarantee 到多在飞场景）
func TestRun_T5(t *testing.T) {
	// 用真实 HollywoodActorPool：同聚合哈希到同一 actor，邮箱 FIFO
	metrics := &NoOpActorPoolMetricsCollector{}
	pool := NewHollywoodActorPool(HollywoodActorPoolConfig{PoolSize: 8, InboxSize: 100, MaxRestarts: 3}, metrics)
	defer pool.Stop()

	var mu sync.Mutex
	var order []int64
	handler := func(_ context.Context, value []byte) error {
		off := int64(0)
		fmt.Sscanf(string(value), "%d", &off)
		mu.Lock()
		order = append(order, off)
		mu.Unlock()
		return nil
	}

	cfg := PipelineConfig{Enabled: true, WindowSize: 8, FlushTimeout: 100 * time.Millisecond, DLQTimeout: 200 * time.Millisecond}
	p, compCh, dlqDoneCh := newPartitionPipeline(cfg, 10*time.Second)
	p.pool = pool
	p.buildAggMsg = func(m *sarama.ConsumerMessage) *AggregateMessage {
		return &AggregateMessage{
			Offset: m.Offset, Value: m.Value, AggregateID: "same-agg", // 同聚合 → 同 actor
			Done: make(chan error, 1), Handler: handler, IsEnvelope: false,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	msgs := make(chan *sarama.ConsumerMessage, 8)
	for _, off := range []int64{10, 11, 12, 13, 14} {
		msgs <- &sarama.ConsumerMessage{Offset: off, Value: []byte(fmt.Sprintf("%d", off))}
	}
	close(msgs)

	done := make(chan struct{})
	go func() { _ = p.run(ctx, msgs, &fakeMarker{}, compCh, dlqDoneCh); close(done) }()
	<-time.After(300 * time.Millisecond) // 等全部处理
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, order, 5)
	assert.Equal(t, []int64{10, 11, 12, 13, 14}, order, "同聚合多在飞仍按 offset 升序执行")
}

// TestRun_T6 队头阻塞期间不对旧 offset 做 re-submit（回归守护）
func TestRun_T6(t *testing.T) {
	p, compCh, dlqDoneCh := newPipelineForTest(8)
	pool := p.pool.(*fakePool)
	p.dlq = &fakeDLQ{ok: false} // 让 offset 10 永久阻塞

	// ⭐ P2-2：数 buildAggMsg 调用（真正的 re-submit 面），而非 pool.submitted（后者可能被 flush 空窗掩盖）
	var buildCount int32
	origBuild := p.buildAggMsg
	p.buildAggMsg = func(m *sarama.ConsumerMessage) *AggregateMessage {
		if m.Offset == 10 {
			atomic.AddInt32(&buildCount, 1)
		}
		return origBuild(m)
	}

	ctx, cancel := context.WithCancel(context.Background())
	msgs := make(chan *sarama.ConsumerMessage, 4)
	msgs <- &sarama.ConsumerMessage{Offset: 10}
	close(msgs)

	go func() { _ = p.run(ctx, msgs, &fakeMarker{}, compCh, dlqDoneCh) }()
	<-time.After(50 * time.Millisecond)
	pool.submitted[0].Done <- errors.New("fail") // 触发 DLQ → 失败 → 阻塞
	<-time.After(200 * time.Millisecond)

	cancel()
	<-time.After(100 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&buildCount), "严禁对旧 offset re-submit：buildAggMsg 对 offset 10 只能调用一次")
}

// TestRun_T9 反复 rebalance 不泄漏 goroutine（bridge 有界、compCh buffer 足够）
func TestRun_T9(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		p, compCh, dlqDoneCh := newPipelineForTest(8)
		ctx, cancel := context.WithCancel(context.Background())
		msgs := make(chan *sarama.ConsumerMessage, 8)
		for off := int64(0); off < 8; off++ {
			msgs <- &sarama.ConsumerMessage{Offset: off}
		}
		close(msgs)
		done := make(chan struct{})
		go func() { _ = p.run(ctx, msgs, &fakeMarker{}, compCh, dlqDoneCh); close(done) }()
		<-time.After(30 * time.Millisecond)
		cancel() // bridge 经 ctx.Done 分支退出
		<-done
	}
	// ⭐ P2-3：轮询等 bridge goroutine 退出后再计数（避免调度抖动假阳/假阴），阈值收紧到 +2
	deadline := time.Now().Add(500 * time.Millisecond)
	var after int
	for time.Now().Before(deadline) {
		runtime.GC()
		after = runtime.NumGoroutine()
		if after <= before+2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.LessOrEqual(t, after, before+2, "反复 rebalance 后 goroutine 不应净增长（bridge 经 ctx.Done 退出、不泄漏）")
}

// TestRun_T10 compCh nil 守卫：构造 offset 不在 inflight 的 completion，不 panic
func TestRun_T10(t *testing.T) {
	p, compCh, dlqDoneCh := newPipelineForTest(8)
	ctx, cancel := context.WithCancel(context.Background())
	msgs := make(chan *sarama.ConsumerMessage, 1)
	close(msgs) // 无消息
	done := make(chan struct{})
	go func() { _ = p.run(ctx, msgs, &fakeMarker{}, compCh, dlqDoneCh); close(done) }()
	compCh <- completion{offset: 9999, err: nil} // 不在 inflight（compCh 即 run 在用的同一 chan，A1）
	cancel()
	<-done // 不 panic 即通过
}

// TestRun_T16 windowSize=1 与现状串行行为等价（提交升序、无并发放大）
func TestRun_T16(t *testing.T) {
	p, compCh, dlqDoneCh := newPipelineForTest(1) // windowSize=1
	pool := p.pool.(*fakePool)
	p.dlq = &fakeDLQ{ok: true}
	ctx, cancel := context.WithCancel(context.Background())
	msgs := make(chan *sarama.ConsumerMessage, 5)
	for off := int64(0); off < 5; off++ {
		msgs <- &sarama.ConsumerMessage{Offset: off}
	}
	close(msgs)
	marker := &fakeMarker{}
	go func() { _ = p.run(ctx, msgs, marker, compCh, dlqDoneCh) }()
	<-time.After(50 * time.Millisecond)
	// windowSize=1：同一时刻最多一条在飞
	require.Len(t, pool.submitted, 1)
	pool.submitted[0].Done <- nil // 完成第一条
	<-time.After(50 * time.Millisecond)
	cancel()
	// 每完成一条才读下一条 → 提交严格升序、无空洞
	for i := 1; i < len(marker.marked); i++ {
		assert.Greater(t, marker.marked[i], marker.marked[i-1])
	}
}

// fakeSession 实现 sarama.ConsumerGroupSession 用到的子集（MarkMessage + Context 功能性，其余 no-op）。
// 方法集对齐 sarama v1.46.0 的 ConsumerGroupSession 接口（8 个方法）。
type fakeSession struct {
	marked []int64
	ctx    context.Context
}

func (s *fakeSession) Claims() map[string][]int32               { return nil }
func (s *fakeSession) MemberID() string                         { return "" }
func (s *fakeSession) GenerationID() int32                      { return 0 }
func (s *fakeSession) MarkOffset(string, int32, int64, string)  {}
func (s *fakeSession) Commit()                                  {}
func (s *fakeSession) ResetOffset(string, int32, int64, string) {}
func (s *fakeSession) MarkMessage(msg *sarama.ConsumerMessage, _ string) {
	s.marked = append(s.marked, msg.Offset)
}
func (s *fakeSession) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

// TestSaramaSessionMarker 验证适配器把 MarkMessage 调用透传到底层 sarama session。
func TestSaramaSessionMarker(t *testing.T) {
	s := &fakeSession{}
	m := saramaSessionMarker{s: s}
	m.MarkMessage(&sarama.ConsumerMessage{Offset: 3}, "")
	assert.Equal(t, []int64{3}, s.marked)
}
