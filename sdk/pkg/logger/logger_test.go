package logger

import "testing"

// TestDefaultLoggerNonNilByDefault 钉死 P2 修复：Logger / DefaultLogger 必须有非空 nop 默认。
// 否则任何在 initialize.Setup 之前的裸 logger.* 调用（如 NewKafkaEventBus 里的启动日志 :250）
// 会 nil deref panic——此前仅靠 eventbus 包测试的 init() 兜底才不炸，依赖极隐式。
func TestDefaultLoggerNonNilByDefault(t *testing.T) {
	if Logger == nil {
		t.Fatal("Logger must be non-nil (nop default) before Setup to avoid nil panic")
	}
	if DefaultLogger == nil {
		t.Fatal("DefaultLogger must be non-nil (nop default) before Setup to avoid nil panic")
	}
	// 裸调用各入口在 nop 上不 panic 即通过（initialize.Setup 仍会覆盖为真实实例）。
	Info("pre-setup info", "k", 1)
	Infof("pre-setup infof: k=%v", 1)
	Warn("pre-setup warn")
	Warnf("pre-setup warnf: %d", 1)
}
