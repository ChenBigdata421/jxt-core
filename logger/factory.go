package logger

import (
	"fmt"
	"sync"
)

// AdapterFactory 适配器工厂
type AdapterFactory func(opts ...Option) Logger

var (
	// 适配器注册表
	adapterRegistry = make(map[string]AdapterFactory)
	registryMu      sync.RWMutex
)

func init() {
	// 注册内置适配器
	RegisterAdapter("default", NewLogger)
	RegisterAdapter("logrus", NewLogrusLogger)
	RegisterAdapter("zap", NewZapLogger)
}

// RegisterAdapter 注册适配器
func RegisterAdapter(name string, factory AdapterFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	adapterRegistry[name] = factory
}

// GetAdapter 获取适配器
func GetAdapter(name string) (AdapterFactory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	
	factory, ok := adapterRegistry[name]
	if !ok {
		return nil, fmt.Errorf("adapter %s not found", name)
	}
	return factory, nil
}

// ListAdapters 列出所有已注册的适配器
func ListAdapters() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	
	names := make([]string, 0, len(adapterRegistry))
	for name := range adapterRegistry {
		names = append(names, name)
	}
	return names
}

// NewFromConfig 从配置创建 Logger
func NewFromConfig(cfg *Config) (Logger, error) {
	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// 获取适配器工厂
	factory, err := GetAdapter(cfg.Adapter.Type)
	if err != nil {
		return nil, err
	}

	// 构建 Options
	opts := []Option{
		WithLevel(parseLevel(cfg.Core.Level)),
		WithName(cfg.Core.Name),
	}

	if cfg.Core.Fields != nil {
		opts = append(opts, WithFields(cfg.Core.Fields))
	}

	// 添加适配器特定的 Options
	opts = append(opts, buildAdapterOptions(cfg)...)

	// 创建基础 Logger
	log := factory(opts...)

	// 应用插件（按顺序）
	log = applyPlugins(log, cfg)

	return log, nil
}

// buildAdapterOptions 构建适配器特定的配置
func buildAdapterOptions(cfg *Config) []Option {
	opts := []Option{}

	// 输出配置
	if cfg.Output.Console != nil && cfg.Output.Console.Enabled {
		opts = append(opts, WithStdout(true))
	}

	if cfg.Output.File != nil && cfg.Output.File.Enabled {
		opts = append(opts,
			WithPath(cfg.Output.File.Path),
			func(o *Options) {
				if cfg.Output.File.MaxSize > 0 {
					o.MaxSize = cfg.Output.File.MaxSize
				}
				if cfg.Output.File.MaxAge > 0 {
					o.MaxAge = cfg.Output.File.MaxAge
				}
				if cfg.Output.File.MaxBackups > 0 {
					o.MaxBackups = cfg.Output.File.MaxBackups
				}
				o.Compress = cfg.Output.File.Compress
				o.LocalTime = cfg.Output.File.LocalTime
			},
		)
	}

	return opts
}

// applyPlugins 应用插件
func applyPlugins(log Logger, cfg *Config) Logger {
	// 采样插件
	if cfg.Plugins.Sampling != nil && cfg.Plugins.Sampling.Enabled {
		log = NewSamplingLogger(log, SamplingConfig{
			Initial:    cfg.Plugins.Sampling.Initial,
			Thereafter: cfg.Plugins.Sampling.Thereafter,
			Tick:       cfg.Plugins.Sampling.Tick,
		})
	}

	// TODO: 脱敏、监控、链路追踪插件

	return log
}

// parseLevel 解析日志级别
func parseLevel(level string) Level {
	switch level {
	case "trace":
		return TraceLevel
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn", "warning":
		return WarnLevel
	case "error":
		return ErrorLevel
	case "fatal", "panic":
		return FatalLevel
	default:
		return InfoLevel
	}
}

// SetupLogger 从配置设置全局 Logger（向后兼容）
func SetupLogger(cfg interface{}) Logger {
	var config *Config

	switch v := cfg.(type) {
	case *Config:
		config = v
	case *LegacyConfig:
		config = FromLegacyConfig(v)
	case Config:
		config = &v
	case LegacyConfig:
		config = FromLegacyConfig(&v)
	default:
		// 使用默认配置
		config = DefaultConfig()
	}

	log, err := NewFromConfig(config)
	if err != nil {
		// 降级到默认 Logger
		log = NewLogger()
		log.Logf(ErrorLevel, "failed to create logger from config: %v", err)
	}

	// 设置为全局 Logger
	DefaultLogger = log
	return log
}

// 便捷构造函数

// NewDevelopmentLogger 创建开发环境 Logger（彩色输出，详细信息）
func NewDevelopmentLogger() Logger {
	cfg := &Config{
		Core: CoreConfig{
			Level:            "debug",
			EnableCaller:     true,
			EnableStacktrace: true,
		},
		Adapter: AdapterConfig{
			Type: "logrus",
		},
		Output: OutputConfig{
			Console: &ConsoleConfig{
				Enabled: true,
				Color:   true,
				Level:   "debug",
			},
			Format: "console",
		},
	}

	log, _ := NewFromConfig(cfg)
	return log
}

// NewProductionLogger 创建生产环境 Logger（JSON 格式，高性能）
func NewProductionLogger(logPath string) Logger {
	cfg := &Config{
		Core: CoreConfig{
			Level:            "info",
			EnableCaller:     false,
			EnableStacktrace: false,
		},
		Adapter: AdapterConfig{
			Type: "logrus",
		},
		Output: OutputConfig{
			Console: &ConsoleConfig{
				Enabled: false,
			},
			File: &FileConfig{
				Enabled:    true,
				Path:       logPath,
				Level:      "info",
				MaxSize:    100,
				MaxAge:     30,
				MaxBackups: 7,
				Compress:   true,
			},
			Format: "json",
		},
		Plugins: PluginsConfig{
			Sampling: &SamplingPluginConfig{
				Enabled:       true,
				Initial:       10,
				Thereafter:    100,
				Tick:          1000000000, // 1s
				ExcludeLevels: []string{"error", "fatal"},
			},
		},
	}

	log, _ := NewFromConfig(cfg)
	return log
}

// NewTestLogger 创建测试环境 Logger（内存输出，不写文件）
func NewTestLogger() Logger {
	return NewLogger(
		WithLevel(DebugLevel),
		WithStdout(true),
	)
}
