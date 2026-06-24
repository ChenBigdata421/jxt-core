package eventbus

import (
	"errors"
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
