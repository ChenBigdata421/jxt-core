package eventbus

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// RateLimiter 流量控制器
// 实现比evidence-management更灵活的流量控制
type RateLimiter struct {
	limiter   *rate.Limiter
	burstSize int
	rateLimit rate.Limit
	enabled   bool
	logger    *zap.Logger
}

// NewRateLimiter 创建流量控制器
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	if !config.Enabled {
		return &RateLimiter{
			enabled: false,
			logger:  logger.Logger,
		}
	}

	rateLimit := rate.Limit(config.RatePerSecond)
	limiter := rate.NewLimiter(rateLimit, config.BurstSize)

	return &RateLimiter{
		limiter:   limiter,
		burstSize: config.BurstSize,
		rateLimit: rateLimit,
		enabled:   true,
		logger:    logger.Logger,
	}
}

// RateLimitConfig 流量控制配置
type RateLimitConfig struct {
	Enabled       bool    // 是否启用流量控制
	RatePerSecond float64 // 每秒允许的请求数
	BurstSize     int     // 突发容量
}

// Wait 等待令牌，实现背压机制
func (rl *RateLimiter) Wait(ctx context.Context) error {
	if !rl.enabled {
		return nil // 未启用流量控制，直接通过
	}

	start := time.Now()
	err := rl.limiter.Wait(ctx)
	if err != nil {
		rl.logger.Error("Rate limiter wait failed", zap.Error(err))
		return err
	}

	waitTime := time.Since(start)
	if waitTime > 100*time.Millisecond {
		rl.logger.Warn("Rate limiter caused significant delay",
			zap.Duration("waitTime", waitTime),
			zap.Float64("rateLimit", float64(rl.rateLimit)),
			zap.Int("burstSize", rl.burstSize))
	}

	return nil
}

// Allow 检查是否允许立即处理（非阻塞）
func (rl *RateLimiter) Allow() bool {
	if !rl.enabled {
		return true
	}
	return rl.limiter.Allow()
}

// Reserve 预留令牌
func (rl *RateLimiter) Reserve() *rate.Reservation {
	if !rl.enabled {
		// 返回一个立即可用的预留
		return &rate.Reservation{}
	}
	return rl.limiter.Reserve()
}

// SetLimit 动态调整限流参数
func (rl *RateLimiter) SetLimit(ratePerSecond float64) {
	if !rl.enabled {
		return
	}

	newLimit := rate.Limit(ratePerSecond)
	rl.limiter.SetLimit(newLimit)
	rl.rateLimit = newLimit

	rl.logger.Info("Rate limit updated",
		zap.Float64("newRateLimit", float64(newLimit)),
		zap.Int("burstSize", rl.burstSize))
}

// SetBurst 动态调整突发容量
func (rl *RateLimiter) SetBurst(burstSize int) {
	if !rl.enabled {
		return
	}

	rl.limiter.SetBurst(burstSize)
	rl.burstSize = burstSize

	rl.logger.Info("Burst size updated",
		zap.Float64("rateLimit", float64(rl.rateLimit)),
		zap.Int("newBurstSize", burstSize))
}

// GetStats 获取流量控制统计信息
func (rl *RateLimiter) GetStats() *RateLimiterStats {
	if !rl.enabled {
		return &RateLimiterStats{
			Enabled: false,
		}
	}

	return &RateLimiterStats{
		Enabled:         true,
		RateLimit:       float64(rl.rateLimit),
		BurstSize:       rl.burstSize,
		TokensAvailable: rl.limiter.Tokens(),
	}
}

// RateLimiterStats 流量控制统计信息
type RateLimiterStats struct {
	Enabled         bool    `json:"enabled"`
	RateLimit       float64 `json:"rateLimit"`
	BurstSize       int     `json:"burstSize"`
	TokensAvailable float64 `json:"tokensAvailable"`
}

// AdaptiveRateLimiter 自适应流量控制器
// 根据系统负载自动调整限流参数
type AdaptiveRateLimiter struct {
	*RateLimiter
	baseRate          rate.Limit
	maxRate           rate.Limit
	minRate           rate.Limit
	adaptInterval     time.Duration
	lastAdaptTime     time.Time
	errorRate         float64
	successRate       float64
	adaptationEnabled bool
}

// NewAdaptiveRateLimiter 创建自适应流量控制器
func NewAdaptiveRateLimiter(config RateLimitConfig, adaptConfig AdaptiveRateLimitConfig) *AdaptiveRateLimiter {
	baseLimiter := NewRateLimiter(config)

	return &AdaptiveRateLimiter{
		RateLimiter:       baseLimiter,
		baseRate:          rate.Limit(config.RatePerSecond),
		maxRate:           rate.Limit(adaptConfig.MaxRatePerSecond),
		minRate:           rate.Limit(adaptConfig.MinRatePerSecond),
		adaptInterval:     adaptConfig.AdaptInterval,
		adaptationEnabled: adaptConfig.Enabled,
	}
}

// AdaptiveRateLimitConfig 自适应流量控制配置
type AdaptiveRateLimitConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	MaxRatePerSecond float64       `mapstructure:"maxRatePerSecond"`
	MinRatePerSecond float64       `mapstructure:"minRatePerSecond"`
	AdaptInterval    time.Duration `mapstructure:"adaptInterval"`
	ErrorThreshold   float64       `mapstructure:"errorThreshold"`
	SuccessThreshold float64       `mapstructure:"successThreshold"`
}

// RecordSuccess 记录成功处理
func (arl *AdaptiveRateLimiter) RecordSuccess() {
	if !arl.adaptationEnabled {
		return
	}
	arl.successRate = arl.successRate*0.9 + 0.1 // 指数移动平均
	arl.tryAdapt()
}

// RecordError 记录处理错误
func (arl *AdaptiveRateLimiter) RecordError() {
	if !arl.adaptationEnabled {
		return
	}
	arl.errorRate = arl.errorRate*0.9 + 0.1 // 指数移动平均
	arl.tryAdapt()
}

// tryAdapt 尝试自适应调整
func (arl *AdaptiveRateLimiter) tryAdapt() {
	if time.Since(arl.lastAdaptTime) < arl.adaptInterval {
		return
	}

	currentRate := arl.rateLimit
	newRate := currentRate

	// 根据错误率和成功率调整
	if arl.errorRate > 0.1 { // 错误率过高，降低速率
		newRate = rate.Limit(float64(currentRate) * 0.8)
		if newRate < arl.minRate {
			newRate = arl.minRate
		}
	} else if arl.successRate > 0.9 { // 成功率很高，可以提高速率
		newRate = rate.Limit(float64(currentRate) * 1.2)
		if newRate > arl.maxRate {
			newRate = arl.maxRate
		}
	}

	if newRate != currentRate {
		arl.SetLimit(float64(newRate))
		arl.logger.Info("Adaptive rate limit adjusted",
			zap.Float64("oldRate", float64(currentRate)),
			zap.Float64("newRate", float64(newRate)),
			zap.Float64("errorRate", arl.errorRate),
			zap.Float64("successRate", arl.successRate))
	}

	arl.lastAdaptTime = time.Now()
}

// GetAdaptiveStats 获取自适应流量控制统计信息
func (arl *AdaptiveRateLimiter) GetAdaptiveStats() *AdaptiveRateLimiterStats {
	baseStats := arl.GetStats()
	return &AdaptiveRateLimiterStats{
		RateLimiterStats:  *baseStats,
		AdaptationEnabled: arl.adaptationEnabled,
		BaseRate:          float64(arl.baseRate),
		MaxRate:           float64(arl.maxRate),
		MinRate:           float64(arl.minRate),
		ErrorRate:         arl.errorRate,
		SuccessRate:       arl.successRate,
		LastAdaptTime:     arl.lastAdaptTime,
	}
}

// AdaptiveRateLimiterStats 自适应流量控制统计信息
type AdaptiveRateLimiterStats struct {
	RateLimiterStats
	AdaptationEnabled bool      `json:"adaptationEnabled"`
	BaseRate          float64   `json:"baseRate"`
	MaxRate           float64   `json:"maxRate"`
	MinRate           float64   `json:"minRate"`
	ErrorRate         float64   `json:"errorRate"`
	SuccessRate       float64   `json:"successRate"`
	LastAdaptTime     time.Time `json:"lastAdaptTime"`
}
