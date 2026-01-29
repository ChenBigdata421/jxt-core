package logger

import (
	"sync"
	"time"
)

// SamplingConfig 采样配置
type SamplingConfig struct {
	// Initial 每个 Tick 周期内前 N 条日志必须记录
	Initial int
	// Thereafter 超过 Initial 后，每 M 条记录 1 条
	Thereafter int
	// Tick 采样周期（例如：每秒、每分钟重置计数器）
	Tick time.Duration
}

// DefaultSamplingConfig 默认采样配置（生产环境推荐）
var DefaultSamplingConfig = SamplingConfig{
	Initial:    10,              // 每秒前 10 条必记录
	Thereafter: 100,             // 之后每 100 条记录 1 条
	Tick:       time.Second,     // 每秒重置
}

// samplingState 采样状态（共享状态，使用指针避免复制）
type samplingState struct {
	mu      sync.Mutex
	counter uint64    // 当前周期计数器
	tick    uint64    // 周期编号
	start   time.Time // 当前周期起始时间
}

// samplingLogger 采样日志（防止日志风暴）
type samplingLogger struct {
	logger Logger
	config SamplingConfig
	state  *samplingState // P0 FIX: 使用指针共享状态，避免 Fields() 复制导致计数错误
}

// NewSamplingLogger 创建采样 logger
func NewSamplingLogger(logger Logger, config SamplingConfig) Logger {
	// 应用默认值
	if config.Initial <= 0 {
		config.Initial = 10
	}
	if config.Thereafter <= 0 {
		config.Thereafter = 100
	}
	if config.Tick <= 0 {
		config.Tick = time.Second
	}
	
	return &samplingLogger{
		logger: logger,
		config: config,
		state:  &samplingState{start: time.Now()},
	}
}

func (s *samplingLogger) Init(opts ...Option) error {
	return s.logger.Init(opts...)
}

func (s *samplingLogger) Options() Options {
	return s.logger.Options()
}

func (s *samplingLogger) Fields(fields map[string]interface{}) Logger {
	return &samplingLogger{
		logger: s.logger.Fields(fields),
		config: s.config,
		state:  s.state, // P0 FIX: 共享状态指针，不复制
	}
}

func (s *samplingLogger) Log(level Level, v ...interface{}) {
	if s.shouldSample() {
		s.logger.Log(level, v...)
	}
}

func (s *samplingLogger) Logf(level Level, format string, v ...interface{}) {
	if s.shouldSample() {
		s.logger.Logf(level, format, v...)
	}
}

func (s *samplingLogger) String() string {
	return s.logger.String() + "-sampling"
}

// shouldSample 判断是否应该采样（核心算法）
func (s *samplingLogger) shouldSample() bool {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	
	// 检查是否进入新周期
	now := time.Now()
	currentTick := uint64(now.Sub(s.state.start) / s.config.Tick)
	if currentTick > s.state.tick {
		// 新周期，重置计数器
		s.state.tick = currentTick
		s.state.counter = 0
	}
	
	s.state.counter++
	
	// 采样决策
	if s.state.counter <= uint64(s.config.Initial) {
		// 前 N 条必须记录
		return true
	}
	
	// 之后每 M 条记录 1 条
	return (s.state.counter-uint64(s.config.Initial))%uint64(s.config.Thereafter) == 1
}

// 如果底层 logger 实现了扩展接口，透传调用

type extendedLogger interface {
	Info(msg string, fields ...Field)
	Debug(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	WithContext(ctx interface{}) Logger
	With(fields ...Field) Logger
	Sync() error
}

func (s *samplingLogger) Info(msg string, fields ...Field) {
	if !s.shouldSample() {
		return
	}
	if ext, ok := s.logger.(extendedLogger); ok {
		ext.Info(msg, fields...)
	}
}

func (s *samplingLogger) Debug(msg string, fields ...Field) {
	if !s.shouldSample() {
		return
	}
	if ext, ok := s.logger.(extendedLogger); ok {
		ext.Debug(msg, fields...)
	}
}

func (s *samplingLogger) Warn(msg string, fields ...Field) {
	if !s.shouldSample() {
		return
	}
	if ext, ok := s.logger.(extendedLogger); ok {
		ext.Warn(msg, fields...)
	}
}

func (s *samplingLogger) Error(msg string, fields ...Field) {
	// Error 级别日志不采样（保留所有错误）
	if ext, ok := s.logger.(extendedLogger); ok {
		ext.Error(msg, fields...)
	}
}

func (s *samplingLogger) WithContext(ctx interface{}) Logger {
	if ext, ok := s.logger.(extendedLogger); ok {
		return &samplingLogger{
			logger: ext.WithContext(ctx),
			config: s.config,
			state:  s.state, // P0 FIX: 共享状态指针
		}
	}
	return s
}

func (s *samplingLogger) With(fields ...Field) Logger {
	if ext, ok := s.logger.(extendedLogger); ok {
		return &samplingLogger{
			logger: ext.With(fields...),
			config: s.config,
			state:  s.state, // P0 FIX: 共享状态指针
		}
	}
	return s
}

func (s *samplingLogger) Sync() error {
	if ext, ok := s.logger.(extendedLogger); ok {
		return ext.Sync()
	}
	return nil
}
