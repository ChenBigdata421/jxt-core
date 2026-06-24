package eventbus

import (
	"context"
	"errors"
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
