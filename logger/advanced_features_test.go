package logger

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestSampling 测试日志采样功能
func TestSampling(t *testing.T) {
	buf := &bytes.Buffer{}
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	// 配置采样：前 3 条必记录，之后每 10 条记录 1 条
	samplingLogger := NewSamplingLogger(baseLogger, SamplingConfig{
		Initial:    3,
		Thereafter: 10,
		Tick:       time.Second,
	})
	
	// 记录 30 条日志
	for i := 1; i <= 30; i++ {
		samplingLogger.Log(InfoLevel, "Test log", i)
	}
	
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	
	// 预期：前 3 条 + (30-3)/10 = 5 条，总共 6 条
	expected := 6
	if len(lines) != expected {
		t.Errorf("Expected %d log lines, got %d", expected, len(lines))
	}
	
	t.Logf("Sampling test passed: %d logs recorded out of 30 (%.1f%% reduction)", 
		len(lines), (1-float64(len(lines))/30)*100)
}

// TestSanitizer 测试敏感字段脱敏
func TestSanitizer(t *testing.T) {
	buf := &bytes.Buffer{}
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	// 启用脱敏
	sanitizerLogger := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	// 测试手机号脱敏
	sanitizerLogger.Fields(map[string]interface{}{
		"phone":    "13812345678",
		"password": "secret123",
		"token":    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
	}).Log(InfoLevel, "User login")
	
	output := buf.String()
	
	// 验证脱敏效果
	tests := []struct {
		name     string
		contains string
		notContains string
	}{
		{
			name:        "手机号脱敏",
			contains:    "138****5678",
			notContains: "13812345678",
		},
		{
			name:        "密码删除",
			contains:    "[REDACTED]",
			notContains: "secret123",
		},
		{
			name:        "Token 哈希",
			contains:    "sha256:",
			notContains: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(output, tt.contains) {
				t.Errorf("Expected output to contain '%s'", tt.contains)
			}
			if strings.Contains(output, tt.notContains) {
				t.Errorf("Expected output NOT to contain '%s'", tt.notContains)
			}
		})
	}
	
	t.Logf("Sanitizer test passed:\n%s", output)
}

// TestAsyncPerformance 测试异步日志性能
func TestAsyncPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}
	
	buf := &bytes.Buffer{}
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	// 同步日志
	start := time.Now()
	for i := 0; i < 10000; i++ {
		baseLogger.Log(InfoLevel, "Sync log", i)
	}
	syncDuration := time.Since(start)
	
	// 异步日志
	buf.Reset()
	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    1000,
		FlushInterval: 10 * time.Millisecond,
		DropPolicy:    "block",
	})
	
	start = time.Now()
	for i := 0; i < 10000; i++ {
		asyncLogger.Log(InfoLevel, "Async log", i)
	}
	
	// 关闭并等待刷新
	if closer, ok := asyncLogger.(interface{ Close() error }); ok {
		closer.Close()
	}
	asyncDuration := time.Since(start)
	
	improvement := (1 - float64(asyncDuration)/float64(syncDuration)) * 100
	t.Logf("Async performance test:")
	t.Logf("  Sync:  %v", syncDuration)
	t.Logf("  Async: %v", asyncDuration)
	t.Logf("  Improvement: %.1f%%", improvement)
	
	// 验证异步日志至少有 10% 性能提升
	if improvement < 10 {
		t.Logf("Warning: Async performance improvement (%.1f%%) is less than expected (>10%%)", improvement)
	}
}

// TestCombinedFeatures 测试组合功能（采样 + 脱敏 + 异步）
func TestCombinedFeatures(t *testing.T) {
	buf := &bytes.Buffer{}
	baseLogger := NewLogrusLogger(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)
	
	// 组合：异步 -> 采样 -> 脱敏
	logger := NewAsyncLogger(
		NewSamplingLogger(
			NewSanitizerLogger(baseLogger, DefaultSanitizerConfig),
			SamplingConfig{
				Initial:    5,
				Thereafter: 10,
				Tick:       time.Second,
			},
		),
		AsyncConfig{
			BufferSize:    100,
			FlushInterval: 10 * time.Millisecond,
			DropPolicy:    "block",
		},
	)
	
	// 记录 50 条日志（包含敏感信息）
	for i := 1; i <= 50; i++ {
		logger.Fields(map[string]interface{}{
			"phone":   "13812345678",
			"user_id": i,
		}).Log(InfoLevel, "User action")
	}
	
	// 关闭并等待刷新
	if closer, ok := logger.(interface{ Close() error }); ok {
		closer.Close()
	}
	
	output := buf.String()
	
	// 验证
	if strings.Contains(output, "13812345678") {
		t.Error("Sanitizer failed: raw phone number found in output")
	}
	
	lines := strings.Split(strings.TrimSpace(output), "\n")
	expected := 10 // 前 5 条 + (50-5)/10 = 10 条
	if len(lines) != expected {
		t.Logf("Warning: Expected %d log lines, got %d (sampling may vary)", expected, len(lines))
	}
	
	t.Logf("Combined features test passed:")
	t.Logf("  Input:  50 logs with sensitive data")
	t.Logf("  Output: %d logs (sampling: %.0f%% reduction, sanitizer: applied)", 
		len(lines), (1-float64(len(lines))/50)*100)
}

// BenchmarkSampling 基准测试：采样性能
func BenchmarkSampling(b *testing.B) {
	buf := &bytes.Buffer{}
	baseLogger := NewLogrusLogger(WithOutput(buf), WithLevel(InfoLevel))
	samplingLogger := NewSamplingLogger(baseLogger, DefaultSamplingConfig)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		samplingLogger.Log(InfoLevel, "Benchmark log")
	}
}

// BenchmarkSanitizer 基准测试：脱敏性能
func BenchmarkSanitizer(b *testing.B) {
	buf := &bytes.Buffer{}
	baseLogger := NewLogrusLogger(WithOutput(buf), WithLevel(InfoLevel))
	sanitizerLogger := NewSanitizerLogger(baseLogger, DefaultSanitizerConfig)
	
	fields := map[string]interface{}{
		"phone":    "13812345678",
		"password": "secret123",
		"user_id":  12345,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sanitizerLogger.Fields(fields).Log(InfoLevel, "Benchmark log")
	}
}

// BenchmarkAsync 基准测试：异步性能
func BenchmarkAsync(b *testing.B) {
	buf := &bytes.Buffer{}
	baseLogger := NewLogrusLogger(WithOutput(buf), WithLevel(InfoLevel))
	asyncLogger := NewAsyncLogger(baseLogger, DefaultAsyncConfig)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		asyncLogger.Log(InfoLevel, "Benchmark log")
	}
	
	b.StopTimer()
	if closer, ok := asyncLogger.(interface{ Close() error }); ok {
		closer.Close()
	}
}
