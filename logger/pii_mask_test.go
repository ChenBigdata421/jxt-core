package logger

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestMaskingCore_DisabledByDefault 未 SetMasker 时 0 开销委托。
func TestMaskingCore_DisabledByDefault(t *testing.T) {
	SetMasker(nil) // 清除
	defer SetMasker(nil)

	var buf bytes.Buffer
	core := newMaskingCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&buf), zapcore.DebugLevel,
	))
	log := zap.New(core)
	log.Info("手机 13812345678")
	_ = log.Sync()
	if !strings.Contains(buf.String(), "13812345678") {
		t.Fatalf("未注册 Masker 时应原样透传，got: %s", buf.String())
	}
}

// TestMaskingCore_Enabled 注册 Masker 后 Entry.Message 和 string Field 都被脱敏。
func TestMaskingCore_Enabled(t *testing.T) {
	SetMasker(func(s string) string {
		// 简化版 mask：直接把 138xxxxxxxx 替换
		return strings.ReplaceAll(s, "13812345678", "138****5678")
	})
	defer SetMasker(nil)

	var buf bytes.Buffer
	core := newMaskingCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&buf), zapcore.DebugLevel,
	))
	log := zap.New(core)
	log.Info("Message 手机 13812345678", zap.String("field_phone", "13812345678"))
	_ = log.Sync()

	out := buf.String()
	if strings.Contains(out, "13812345678") {
		t.Fatalf("期望手机号被脱敏，实际 output 含原值: %s", out)
	}
	if !strings.Contains(out, "138****5678") {
		t.Fatalf("期望包含 mask 后的值 138****5678, got: %s", out)
	}
}

// TestMaskingCore_WithFields 子 logger With(fields) 也要脱敏预填字段。
func TestMaskingCore_WithFields(t *testing.T) {
	SetMasker(func(s string) string { return strings.ReplaceAll(s, "SECRET", "***") })
	defer SetMasker(nil)

	var buf bytes.Buffer
	core := newMaskingCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&buf), zapcore.DebugLevel,
	))
	log := zap.New(core).With(zap.String("token", "SECRET"))
	log.Info("hello")
	_ = log.Sync()
	if strings.Contains(buf.String(), "SECRET") {
		t.Fatalf("With-prefilled string field 应被脱敏, got: %s", buf.String())
	}
}

// TestMaskingCore_NonStringFields 数字/bool 等非字符串字段不受影响。
func TestMaskingCore_NonStringFields(t *testing.T) {
	SetMasker(func(s string) string { return "MASKED" })
	defer SetMasker(nil)

	var buf bytes.Buffer
	core := newMaskingCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&buf), zapcore.DebugLevel,
	))
	log := zap.New(core)
	log.Info("n", zap.Int("count", 42), zap.Bool("ok", true))
	_ = log.Sync()
	out := buf.String()
	if !strings.Contains(out, "42") || !strings.Contains(out, "true") {
		t.Fatalf("数字/bool 应原样透传, got: %s", out)
	}
}

// BenchmarkMaskingCore_Disabled 未注册时开销基线。
func BenchmarkMaskingCore_Disabled(b *testing.B) {
	SetMasker(nil)
	core := newMaskingCore(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard), zapcore.InfoLevel,
	))
	log := zap.New(core)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Info("hello", zap.String("k", "v"))
	}
}

// BenchmarkMaskingCore_Enabled 注册后开销。
func BenchmarkMaskingCore_Enabled(b *testing.B) {
	SetMasker(func(s string) string { return s })
	defer SetMasker(nil)
	core := newMaskingCore(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard), zapcore.InfoLevel,
	))
	log := zap.New(core)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Info("hello", zap.String("k", "v"))
	}
}
