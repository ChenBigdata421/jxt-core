package outbox

import (
	"context"
	"testing"
)

// C3 回归：EnableDLQ=true 且 DLQHandler=nil 时 Validate 必须报错。
// 旧实现默认注入 NoOpDLQHandler 静默吞事件，processDLQ 对每条 max_retry 静默 no-op。
func TestValidate_RejectsEnableDLQWithoutHandler(t *testing.T) {
	c := DefaultSchedulerConfig()
	c.EnableDLQ = true
	c.DLQHandler = nil
	if err := c.Validate(); err == nil {
		t.Fatal("Validate must reject EnableDLQ=true with nil DLQHandler")
	}

	// 配上 handler 即通过（DLQHandlerFunc 签名以实际为准：Handle(ctx, *OutboxEvent) error）
	c.DLQHandler = DLQHandlerFunc(func(ctx context.Context, e *OutboxEvent) error { return nil })
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate should pass with handler set: %v", err)
	}
}

// C3：默认配置必须 EnableDLQ=false（不再默认注入 NoOpDLQHandler）。
func TestDefaultSchedulerConfig_HasDLQDisabledByDefault(t *testing.T) {
	c := DefaultSchedulerConfig()
	if c.EnableDLQ {
		t.Fatal("default config must have EnableDLQ=false (C3: NoOp default removed)")
	}
	if c.DLQHandler != nil {
		t.Fatal("default config must have nil DLQHandler (C3: NoOpDLQHandler removed)")
	}
}
