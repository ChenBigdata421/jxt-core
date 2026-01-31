package logger

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// ============================================
// 基础性能测试：不同 Logger 实现对比
// ============================================

// BenchmarkLogrus_Simple 基准测试：Logrus 简单日志
func BenchmarkLogrus_Simple(b *testing.B) {
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Log(InfoLevel, "simple log message")
	}
}

// BenchmarkLogrus_WithFields 基准测试：Logrus 带字段日志
func BenchmarkLogrus_WithFields(b *testing.B) {
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(map[string]interface{}{
			"request_id": "req-12345",
			"user_id":    123,
			"latency":    100,
		}).Log(InfoLevel, "request processed")
	}
}

// BenchmarkLogrus_Logf 基准测试：Logrus 格式化日志
func BenchmarkLogrus_Logf(b *testing.B) {
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Logf(InfoLevel, "user %d logged in from %s", 123, "192.168.1.1")
	}
}

// BenchmarkLogrus_Caller 基准测试：Logrus 获取 Caller 信息
func BenchmarkLogrus_Caller(b *testing.B) {
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
		WithPath("test.log"), // 触发 Caller 逻辑
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Log(InfoLevel, "message with caller")
	}
}

// ============================================
// 异步日志性能测试
// ============================================

// BenchmarkAsync_Baseline 基准测试：同步日志（基线）
func BenchmarkAsync_Baseline(b *testing.B) {
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Log(InfoLevel, "sync log message")
	}
}

// BenchmarkAsync_SmallBuffer 基准测试：异步日志（小缓冲）
func BenchmarkAsync_SmallBuffer(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewAsyncLogger(baseLog, AsyncConfig{
		BufferSize:    100,
		FlushInterval: 10 * time.Millisecond,
		DropPolicy:    "drop",
	})
	defer log.(interface{ Close() error }).Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Log(InfoLevel, "async log message")
	}
}

// BenchmarkAsync_LargeBuffer 基准测试：异步日志（大缓冲）
func BenchmarkAsync_LargeBuffer(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewAsyncLogger(baseLog, AsyncConfig{
		BufferSize:    10000,
		FlushInterval: 100 * time.Millisecond,
		DropPolicy:    "drop",
	})
	defer log.(interface{ Close() error }).Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Log(InfoLevel, "async log message")
	}
}

// BenchmarkAsync_WithFields 基准测试：异步日志带字段
func BenchmarkAsync_WithFields(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewAsyncLogger(baseLog, AsyncConfig{
		BufferSize:    10000,
		FlushInterval: 100 * time.Millisecond,
		DropPolicy:    "drop",
	})
	defer log.(interface{ Close() error }).Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(map[string]interface{}{
			"request_id": "req-12345",
			"user_id":    123,
		}).Log(InfoLevel, "async request processed")
	}
}

// BenchmarkAsync_HighContention 基准测试：异步日志高并发
func BenchmarkAsync_HighContention(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewAsyncLogger(baseLog, AsyncConfig{
		BufferSize:    10000,
		FlushInterval: 100 * time.Millisecond,
		DropPolicy:    "drop",
	})
	defer log.(interface{ Close() error }).Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			log.Log(InfoLevel, "concurrent async log")
		}
	})
}

// ============================================
// 采样日志性能测试
// ============================================

// BenchmarkSampling_First10Then1 基准测试：采样（前10条后每1条）
func BenchmarkSampling_First10Then1(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewSamplingLogger(baseLog, SamplingConfig{
		Initial:    10,
		Thereafter: 1,
	})
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Log(InfoLevel, "sampled log message")
	}
}

// BenchmarkSampling_First100Then10 基准测试：采样（前100条后每10条）
func BenchmarkSampling_First100Then10(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewSamplingLogger(baseLog, SamplingConfig{
		Initial:    100,
		Thereafter: 10,
	})
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Log(InfoLevel, "sampled log message")
	}
}

// BenchmarkSampling_WithFields 基准测试：采样日志带字段
func BenchmarkSampling_WithFields(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewSamplingLogger(baseLog, SamplingConfig{
		Initial:    100,
		Thereafter: 10,
	})
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(map[string]interface{}{
			"endpoint": "/api/users",
			"status":   200,
		}).Log(InfoLevel, "sampled request")
	}
}

// ============================================
// 敏感字段脱敏性能测试
// ============================================

// BenchmarkSanitizer_NoSensitiveFields 基准测试：无敏感字段
func BenchmarkSanitizer_NoSensitiveFields(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewSanitizerLogger(baseLog, DefaultSanitizerConfig)
	
	fields := map[string]interface{}{
		"user_id":   123,
		"request_id": "req-12345",
		"latency":   100,
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(fields).Log(InfoLevel, "no sensitive fields")
	}
}

// BenchmarkSanitizer_WithPassword 基准测试：包含密码字段
func BenchmarkSanitizer_WithPassword(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewSanitizerLogger(baseLog, DefaultSanitizerConfig)
	
	fields := map[string]interface{}{
		"username": "john",
		"password": "secret123",
		"email":    "john@example.com",
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(fields).Log(InfoLevel, "with password")
	}
}

// BenchmarkSanitizer_MultipleSensitive 基准测试：多个敏感字段
func BenchmarkSanitizer_MultipleSensitive(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	log := NewSanitizerLogger(baseLog, DefaultSanitizerConfig)
	
	fields := map[string]interface{}{
		"user_id":         123,
		"password":        "secret123",
		"credit_card":     "1234-5678-9012-3456",
		"api_key":         "sk_live_xxxxxxxxxxxxx",
		"access_token":    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
		"social_security": "123-45-6789",
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(fields).Log(InfoLevel, "multiple sensitive fields")
	}
}

// ============================================
// 组合场景性能测试
// ============================================

// BenchmarkCombined_AsyncSamplingSanitizer 基准测试：异步+采样+脱敏
func BenchmarkCombined_AsyncSamplingSanitizer(b *testing.B) {
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	// 层层包装：基础 → 脱敏 → 采样 → 异步
	sanitized := NewSanitizerLogger(baseLog, DefaultSanitizerConfig)
	sampled := NewSamplingLogger(sanitized, SamplingConfig{
		Initial:    100,
		Thereafter: 10,
	})
	log := NewAsyncLogger(sampled, AsyncConfig{
		BufferSize:    10000,
		FlushInterval: 100 * time.Millisecond,
		DropPolicy:    "drop",
	})
	defer log.(interface{ Close() error }).Close()
	
	fields := map[string]interface{}{
		"user_id":  123,
		"password": "secret123",
		"endpoint": "/api/login",
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(fields).Log(InfoLevel, "combined features")
	}
}

// BenchmarkCombined_Production 基准测试：生产环境推荐配置
func BenchmarkCombined_Production(b *testing.B) {
	// 模拟生产环境：文件输出（但使用 discard 避免 I/O 影响基准测试）
	baseLog := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	// 生产配置：脱敏 + 采样 + 异步
	sanitized := NewSanitizerLogger(baseLog, SanitizerConfig{
		Enabled: true,
		Rules: []SanitizerRule{
			{FieldPattern: "password", Strategy: "remove"},
			{FieldPattern: "token", Strategy: "hash"},
			{FieldPattern: "secret", Strategy: "hash"},
			{FieldPattern: "key", Strategy: "hash"},
			{FieldPattern: "credit_card", Strategy: "mask", MaskChar: "*", KeepPrefix: 4, KeepSuffix: 4},
		},
	})
	sampled := NewSamplingLogger(sanitized, SamplingConfig{
		Initial:    100,
		Thereafter: 100,
	})
	log := NewAsyncLogger(sampled, AsyncConfig{
		BufferSize:    10000,
		FlushInterval: 100 * time.Millisecond,
		DropPolicy:    "sample",
	})
	defer log.(interface{ Close() error }).Close()
	
	fields := map[string]interface{}{
		"request_id": "req-12345",
		"user_id":    123,
		"method":     "POST",
		"path":       "/api/users",
		"status":     200,
		"latency_ms": 45,
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			log.Fields(fields).Log(InfoLevel, "production request")
		}
	})
}

// ============================================
// 输出格式性能测试
// ============================================

// BenchmarkFormatter_Console 基准测试：Console 格式化器
func BenchmarkFormatter_Console(b *testing.B) {
	buf := &bytes.Buffer{}
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(buf),
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(map[string]interface{}{
			"user_id": 123,
			"action":  "login",
		}).Log(InfoLevel, "user action")
		buf.Reset() // 避免内存累积
	}
}

// BenchmarkFormatter_JSON 基准测试：JSON 格式化器
func BenchmarkFormatter_JSON(b *testing.B) {
	buf := &bytes.Buffer{}
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(buf),
		WithPath("test.log"), // 触发 JSON formatter
	)
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(map[string]interface{}{
			"user_id": 123,
			"action":  "login",
		}).Log(InfoLevel, "user action")
		buf.Reset()
	}
}

// ============================================
// 压力测试：极限场景
// ============================================

// BenchmarkStress_HighVolume 基准测试：高吞吐量
func BenchmarkStress_HighVolume(b *testing.B) {
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	asyncLog := NewAsyncLogger(log, AsyncConfig{
		BufferSize:    100000, // 10万条缓冲
		FlushInterval: 1 * time.Second,
		DropPolicy:    "drop",
	})
	defer asyncLog.(interface{ Close() error }).Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			asyncLog.Log(InfoLevel, "high volume log")
		}
	})
}

// BenchmarkStress_LargeFields 基准测试：大字段数量
func BenchmarkStress_LargeFields(b *testing.B) {
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	// 模拟包含大量字段的日志
	fields := make(map[string]interface{}, 50)
	for i := 0; i < 50; i++ {
		fields[string(rune('a'+i%26))+string(rune('0'+i%10))] = i
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(fields).Log(InfoLevel, "large fields")
	}
}

// BenchmarkStress_DeepNesting 基准测试：深层嵌套字段
func BenchmarkStress_DeepNesting(b *testing.B) {
	log := NewLogrusLogger(
		WithLevel(InfoLevel),
		WithOutput(io.Discard),
	)
	
	// 模拟深层嵌套的字段
	fields := map[string]interface{}{
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"user-agent": "Mozilla/5.0",
				"auth":       map[string]interface{}{
					"token": "secret",
					"type":  "Bearer",
				},
			},
			"body": map[string]interface{}{
				"user": map[string]interface{}{
					"id":    123,
					"name":  "John",
					"email": "john@example.com",
				},
			},
		},
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		log.Fields(fields).Log(InfoLevel, "deep nesting")
	}
}
