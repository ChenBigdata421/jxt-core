package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewRateLimiter 测试创建速率限制器
func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name   string
		config RateLimitConfig
	}{
		{
			name: "Valid rate limiter",
			config: RateLimitConfig{
				Enabled:       true,
				RatePerSecond: 100.0,
				BurstSize:     10,
			},
		},
		{
			name: "Disabled rate limiter",
			config: RateLimitConfig{
				Enabled:       false,
				RatePerSecond: 100.0,
				BurstSize:     10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewRateLimiter(tt.config)
			assert.NotNil(t, limiter)
		})
	}
}

// TestRateLimiter_Wait 测试等待功能
func TestRateLimiter_Wait(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:       true,
		RatePerSecond: 10.0,
		BurstSize:     1,
	})
	ctx := context.Background()

	// 第一个请求应该立即通过
	start := time.Now()
	err := limiter.Wait(ctx)
	elapsed := time.Since(start)
	
	require.NoError(t, err)
	assert.Less(t, elapsed, 10*time.Millisecond, "第一个请求应该立即通过")
}

// TestRateLimiter_Allow 测试允许功能
func TestRateLimiter_Allow(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:       true,
		RatePerSecond: 10.0,
		BurstSize:     2,
	})

	// 前两个请求应该立即通过（burst=2）
	assert.True(t, limiter.Allow(), "第1个请求应该通过")
	assert.True(t, limiter.Allow(), "第2个请求应该通过")

	// 第三个请求应该被拒绝（超过burst）
	assert.False(t, limiter.Allow(), "第3个请求应该被拒绝")
}

// TestRateLimiter_Reserve 测试预留功能
func TestRateLimiter_Reserve(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:       true,
		RatePerSecond: 10.0,
		BurstSize:     1,
	})

	// 预留一个令牌
	reservation := limiter.Reserve()
	assert.NotNil(t, reservation)

	// 检查延迟
	delay := reservation.Delay()
	assert.GreaterOrEqual(t, delay, time.Duration(0))
}

// TestRateLimiter_SetLimit 测试动态设置速率
func TestRateLimiter_SetLimit(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:       true,
		RatePerSecond: 10.0,
		BurstSize:     1,
	})

	// 设置新的速率
	limiter.SetLimit(100.0)

	// 验证新速率生效
	stats := limiter.GetStats()
	assert.Equal(t, 100.0, stats.RateLimit)
}

// TestRateLimiter_SetBurst 测试动态设置burst
func TestRateLimiter_SetBurst(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:       true,
		RatePerSecond: 10.0,
		BurstSize:     1,
	})

	// 设置新的burst
	limiter.SetBurst(5)

	// 验证新burst生效
	stats := limiter.GetStats()
	assert.Equal(t, 5, stats.BurstSize)
}

// TestRateLimiter_GetStats 测试获取统计信息
func TestRateLimiter_GetStats(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:       true,
		RatePerSecond: 10.0,
		BurstSize:     2,
	})

	// 获取统计信息
	stats := limiter.GetStats()
	assert.NotNil(t, stats)
	assert.True(t, stats.Enabled)
	assert.Equal(t, 10.0, stats.RateLimit)
	assert.Equal(t, 2, stats.BurstSize)
	assert.GreaterOrEqual(t, stats.TokensAvailable, 0.0)
}

// TestRateLimiter_Disabled 测试禁用的速率限制器
func TestRateLimiter_Disabled(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:       false,
		RatePerSecond: 1.0,
		BurstSize:     1,
	})
	ctx := context.Background()

	// 禁用的限制器应该允许所有请求立即通过
	start := time.Now()
	for i := 0; i < 100; i++ {
		err := limiter.Wait(ctx)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)
	
	// 应该非常快（不受速率限制）
	assert.Less(t, elapsed, 100*time.Millisecond)
}

// TestRateLimiter_ConcurrentAccess 测试并发访问
func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewRateLimiter(RateLimitConfig{
		Enabled:       true,
		RatePerSecond: 100.0,
		BurstSize:     10,
	})
	ctx := context.Background()

	var wg sync.WaitGroup
	goroutineCount := 50
	requestsPerGoroutine := 10

	successCount := uint64(0)
	var mu sync.Mutex

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				err := limiter.Wait(ctx)
				if err == nil {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	// 所有请求都应该成功（可能需要等待）
	assert.Equal(t, uint64(goroutineCount*requestsPerGoroutine), successCount)
}

// TestNewAdaptiveRateLimiter 测试创建自适应速率限制器
func TestNewAdaptiveRateLimiter(t *testing.T) {
	limiter := NewAdaptiveRateLimiter(
		RateLimitConfig{
			Enabled:       true,
			RatePerSecond: 100.0,
			BurstSize:     10,
		},
		AdaptiveRateLimitConfig{
			Enabled:          true,
			MaxRatePerSecond: 200.0,
			MinRatePerSecond: 50.0,
			AdaptInterval:    100 * time.Millisecond,
		},
	)
	assert.NotNil(t, limiter)
	assert.NotNil(t, limiter.RateLimiter)
}

// TestAdaptiveRateLimiter_RecordSuccess 测试记录成功
func TestAdaptiveRateLimiter_RecordSuccess(t *testing.T) {
	limiter := NewAdaptiveRateLimiter(
		RateLimitConfig{
			Enabled:       true,
			RatePerSecond: 100.0,
			BurstSize:     10,
		},
		AdaptiveRateLimitConfig{
			Enabled:          true,
			MaxRatePerSecond: 200.0,
			MinRatePerSecond: 50.0,
			AdaptInterval:    100 * time.Millisecond,
		},
	)

	// 记录一些成功
	for i := 0; i < 10; i++ {
		limiter.RecordSuccess()
	}

	stats := limiter.GetAdaptiveStats()
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.SuccessRate, 0.0)
}

// TestAdaptiveRateLimiter_RecordError 测试记录错误
func TestAdaptiveRateLimiter_RecordError(t *testing.T) {
	limiter := NewAdaptiveRateLimiter(
		RateLimitConfig{
			Enabled:       true,
			RatePerSecond: 100.0,
			BurstSize:     10,
		},
		AdaptiveRateLimitConfig{
			Enabled:          true,
			MaxRatePerSecond: 200.0,
			MinRatePerSecond: 50.0,
			AdaptInterval:    100 * time.Millisecond,
		},
	)

	// 记录一些错误
	for i := 0; i < 5; i++ {
		limiter.RecordError()
	}

	stats := limiter.GetAdaptiveStats()
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.ErrorRate, 0.0)
}

// TestAdaptiveRateLimiter_GetAdaptiveStats 测试获取自适应统计信息
func TestAdaptiveRateLimiter_GetAdaptiveStats(t *testing.T) {
	limiter := NewAdaptiveRateLimiter(
		RateLimitConfig{
			Enabled:       true,
			RatePerSecond: 100.0,
			BurstSize:     10,
		},
		AdaptiveRateLimitConfig{
			Enabled:          true,
			MaxRatePerSecond: 200.0,
			MinRatePerSecond: 50.0,
			AdaptInterval:    100 * time.Millisecond,
		},
	)

	limiter.RecordSuccess()
	limiter.RecordError()

	stats := limiter.GetAdaptiveStats()
	assert.NotNil(t, stats)
	assert.True(t, stats.AdaptationEnabled)
	assert.Equal(t, 100.0, stats.BaseRate)
	assert.Equal(t, 200.0, stats.MaxRate)
	assert.Equal(t, 50.0, stats.MinRate)
	assert.GreaterOrEqual(t, stats.ErrorRate, 0.0)
	assert.LessOrEqual(t, stats.ErrorRate, 1.0)
	assert.GreaterOrEqual(t, stats.SuccessRate, 0.0)
	assert.LessOrEqual(t, stats.SuccessRate, 1.0)
}

