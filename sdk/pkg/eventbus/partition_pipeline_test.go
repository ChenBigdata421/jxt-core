package eventbus

import (
	"errors"
	"testing"
	"time"

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
