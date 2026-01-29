package logger

import (
	"fmt"
	"time"
)

// Config 日志配置（完整版，支持所有特性）
type Config struct {
	// Core 核心配置
	Core CoreConfig `json:"core" yaml:"core"`
	
	// Adapter 适配器配置
	Adapter AdapterConfig `json:"adapter" yaml:"adapter"`
	
	// Output 输出配置
	Output OutputConfig `json:"output" yaml:"output"`
	
	// Plugins 插件配置
	Plugins PluginsConfig `json:"plugins" yaml:"plugins"`
}

// CoreConfig 核心配置
type CoreConfig struct {
	// Level 日志级别: trace, debug, info, warn, error, fatal
	Level string `json:"level" yaml:"level"`
	
	// EnableCaller 是否启用调用者信息（文件名:行号）
	EnableCaller bool `json:"enableCaller" yaml:"enableCaller"`
	
	// CallerSkip 跳过的调用栈层数
	CallerSkip int `json:"callerSkip" yaml:"callerSkip"`
	
	// EnableStacktrace 是否启用堆栈跟踪（Error 级别以上）
	EnableStacktrace bool `json:"enableStacktrace" yaml:"enableStacktrace"`
	
	// Name Logger 名称（用于区分不同模块）
	Name string `json:"name" yaml:"name"`
	
	// Fields 默认字段（所有日志都会包含）
	Fields map[string]interface{} `json:"fields" yaml:"fields"`
}

// AdapterConfig 适配器配置
type AdapterConfig struct {
	// Type 适配器类型: zap, zerolog, logrus, slog, default
	Type string `json:"type" yaml:"type"`
	
	// Zap Zap 特定配置
	Zap *ZapConfig `json:"zap,omitempty" yaml:"zap,omitempty"`
	
	// Zerolog Zerolog 特定配置
	Zerolog *ZerologConfig `json:"zerolog,omitempty" yaml:"zerolog,omitempty"`
	
	// Logrus Logrus 特定配置
	Logrus *LogrusConfig `json:"logrus,omitempty" yaml:"logrus,omitempty"`
}

// ZapConfig Zap 适配器配置
type ZapConfig struct {
	// Development 开发模式（更友好的输出格式）
	Development bool `json:"development" yaml:"development"`
	
	// DisableCaller 禁用调用者信息
	DisableCaller bool `json:"disableCaller" yaml:"disableCaller"`
	
	// DisableStacktrace 禁用堆栈跟踪
	DisableStacktrace bool `json:"disableStacktrace" yaml:"disableStacktrace"`
	
	// Sampling 采样配置（Zap 内置）
	Sampling *ZapSamplingConfig `json:"sampling,omitempty" yaml:"sampling,omitempty"`
}

// ZapSamplingConfig Zap 采样配置
type ZapSamplingConfig struct {
	Initial    int `json:"initial" yaml:"initial"`
	Thereafter int `json:"thereafter" yaml:"thereafter"`
}

// ZerologConfig Zerolog 适配器配置
type ZerologConfig struct {
	// TimeFormat 时间格式: unix, unixms, rfc3339
	TimeFormat string `json:"timeFormat" yaml:"timeFormat"`
}

// LogrusConfig Logrus 适配器配置
type LogrusConfig struct {
	// Formatter 格式化器: json, text
	Formatter string `json:"formatter" yaml:"formatter"`
}

// OutputConfig 输出配置
type OutputConfig struct {
	// Console 控制台输出配置
	Console *ConsoleConfig `json:"console,omitempty" yaml:"console,omitempty"`
	
	// File 文件输出配置
	File *FileConfig `json:"file,omitempty" yaml:"file,omitempty"`
	
	// Network 网络输出配置（Kafka、Elasticsearch 等）
	Network *NetworkConfig `json:"network,omitempty" yaml:"network,omitempty"`
	
	// Format 输出格式: json, text, console
	Format string `json:"format" yaml:"format"`
	
	// Encoding 编码格式: json, console, text
	Encoding string `json:"encoding" yaml:"encoding"`
}

// ConsoleConfig 控制台输出配置
type ConsoleConfig struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Color 是否启用彩色输出
	Color bool `json:"color" yaml:"color"`
	
	// Level 控制台日志级别（可与文件不同）
	Level string `json:"level" yaml:"level"`
}

// FileConfig 文件输出配置
type FileConfig struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Path 日志文件路径
	Path string `json:"path" yaml:"path"`
	
	// Level 文件日志级别
	Level string `json:"level" yaml:"level"`
	
	// MaxSize 单个文件最大大小（MB）
	MaxSize int `json:"maxSize" yaml:"maxSize"`
	
	// MaxAge 文件最大保留天数
	MaxAge int `json:"maxAge" yaml:"maxAge"`
	
	// MaxBackups 最大备份数量
	MaxBackups int `json:"maxBackups" yaml:"maxBackups"`
	
	// Compress 是否压缩旧日志
	Compress bool `json:"compress" yaml:"compress"`
	
	// LocalTime 使用本地时间（而非 UTC）
	LocalTime bool `json:"localTime" yaml:"localTime"`
}

// NetworkConfig 网络输出配置
type NetworkConfig struct {
	// Type 类型: kafka, elasticsearch, fluentd, syslog
	Type string `json:"type" yaml:"type"`
	
	// Endpoints 端点列表
	Endpoints []string `json:"endpoints" yaml:"endpoints"`
	
	// Topic Kafka Topic
	Topic string `json:"topic" yaml:"topic"`
	
	// Index Elasticsearch Index
	Index string `json:"index" yaml:"index"`
	
	// BufferSize 缓冲区大小
	BufferSize int `json:"bufferSize" yaml:"bufferSize"`
	
	// FlushInterval 刷新间隔
	FlushInterval time.Duration `json:"flushInterval" yaml:"flushInterval"`
}

// PluginsConfig 插件配置
type PluginsConfig struct {
	// Sampling 采样配置
	Sampling *SamplingPluginConfig `json:"sampling,omitempty" yaml:"sampling,omitempty"`
	
	// Metrics 监控配置
	Metrics *MetricsPluginConfig `json:"metrics,omitempty" yaml:"metrics,omitempty"`
	
	// Tracing 链路追踪配置
	Tracing *TracingPluginConfig `json:"tracing,omitempty" yaml:"tracing,omitempty"`
	
	// Sanitize 脱敏配置
	Sanitize *SanitizePluginConfig `json:"sanitize,omitempty" yaml:"sanitize,omitempty"`
	
	// Async 异步写入配置
	Async *AsyncPluginConfig `json:"async,omitempty" yaml:"async,omitempty"`
}

// SamplingPluginConfig 采样插件配置
type SamplingPluginConfig struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Initial 每个周期前 N 条必记录
	Initial int `json:"initial" yaml:"initial"`
	
	// Thereafter 之后每 M 条记录 1 条
	Thereafter int `json:"thereafter" yaml:"thereafter"`
	
	// Tick 采样周期
	Tick time.Duration `json:"tick" yaml:"tick"`
	
	// ExcludeLevels 排除的日志级别（Error 级别不采样）
	ExcludeLevels []string `json:"excludeLevels" yaml:"excludeLevels"`
}

// MetricsPluginConfig 监控插件配置
type MetricsPluginConfig struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Namespace Prometheus Namespace
	Namespace string `json:"namespace" yaml:"namespace"`
	
	// Subsystem Prometheus Subsystem
	Subsystem string `json:"subsystem" yaml:"subsystem"`
}

// TracingPluginConfig 链路追踪插件配置
type TracingPluginConfig struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Provider 追踪提供商: opentelemetry, jaeger, zipkin
	Provider string `json:"provider" yaml:"provider"`
	
	// AutoExtract 自动从 context 提取 trace_id/span_id
	AutoExtract bool `json:"autoExtract" yaml:"autoExtract"`
}

// SanitizePluginConfig 脱敏插件配置
type SanitizePluginConfig struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Rules 脱敏规则
	Rules []SanitizeRule `json:"rules" yaml:"rules"`
}

// SanitizeRule 脱敏规则
type SanitizeRule struct {
	// FieldPattern 字段名正则（如 password|token|secret）
	FieldPattern string `json:"fieldPattern" yaml:"fieldPattern"`
	
	// Strategy 脱敏策略: mask_all, mask_partial, hash
	Strategy string `json:"strategy" yaml:"strategy"`
	
	// MaskChar 掩码字符
	MaskChar string `json:"maskChar" yaml:"maskChar"`
}

// AsyncPluginConfig 异步写入插件配置
type AsyncPluginConfig struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// BufferSize 缓冲区大小
	BufferSize int `json:"bufferSize" yaml:"bufferSize"`
	
	// FlushInterval 刷新间隔
	FlushInterval time.Duration `json:"flushInterval" yaml:"flushInterval"`
	
	// DropOnFull 缓冲区满时丢弃日志（否则阻塞）
	DropOnFull bool `json:"dropOnFull" yaml:"dropOnFull"`
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Core: CoreConfig{
			Level:            "info",
			EnableCaller:     true,
			CallerSkip:       2,
			EnableStacktrace: true,
			Name:             "go-admin",
		},
		Adapter: AdapterConfig{
			Type: "logrus", // 默认使用 Logrus（生态丰富）
			Logrus: &LogrusConfig{
				Formatter: "json",
			},
		},
		Output: OutputConfig{
			Console: &ConsoleConfig{
				Enabled: true,
				Color:   true,
				Level:   "debug",
			},
			File: &FileConfig{
				Enabled:    true,
				Path:       "logs/app.log",
				Level:      "info",
				MaxSize:    100,
				MaxAge:     30,
				MaxBackups: 7,
				Compress:   true,
				LocalTime:  true,
			},
			Format:   "json",
			Encoding: "json",
		},
		Plugins: PluginsConfig{
			Sampling: &SamplingPluginConfig{
				Enabled:       false,
				Initial:       10,
				Thereafter:    100,
				Tick:          time.Second,
				ExcludeLevels: []string{"error", "fatal"},
			},
			Metrics: &MetricsPluginConfig{
				Enabled:   false,
				Namespace: "go_admin",
				Subsystem: "logger",
			},
			Tracing: &TracingPluginConfig{
				Enabled:     false,
				Provider:    "opentelemetry",
				AutoExtract: true,
			},
			Sanitize: &SanitizePluginConfig{
				Enabled: false,
				Rules: []SanitizeRule{
					{FieldPattern: "password|passwd|pwd", Strategy: "mask_all", MaskChar: "*"},
					{FieldPattern: "token|secret|key", Strategy: "mask_all", MaskChar: "*"},
					{FieldPattern: "phone|mobile", Strategy: "mask_partial", MaskChar: "*"},
					{FieldPattern: "email", Strategy: "mask_partial", MaskChar: "*"},
				},
			},
			Async: &AsyncPluginConfig{
				Enabled:       false,
				BufferSize:    1000,
				FlushInterval: time.Second,
				DropOnFull:    false,
			},
		},
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证日志级别
	if !isValidLevel(c.Core.Level) {
		return fmt.Errorf("invalid log level: %s", c.Core.Level)
	}
	
	// 验证适配器类型
	validAdapters := map[string]bool{
		"zap": true, "zerolog": true, "logrus": true, "slog": true, "default": true,
	}
	if !validAdapters[c.Adapter.Type] {
		return fmt.Errorf("invalid adapter type: %s", c.Adapter.Type)
	}
	
	// 验证输出格式
	validFormats := map[string]bool{
		"json": true, "text": true, "console": true,
	}
	if !validFormats[c.Output.Format] {
		return fmt.Errorf("invalid output format: %s", c.Output.Format)
	}
	
	// 验证至少有一个输出
	hasOutput := false
	if c.Output.Console != nil && c.Output.Console.Enabled {
		hasOutput = true
	}
	if c.Output.File != nil && c.Output.File.Enabled {
		hasOutput = true
	}
	if c.Output.Network != nil {
		hasOutput = true
	}
	if !hasOutput {
		return fmt.Errorf("at least one output must be enabled")
	}
	
	return nil
}

func isValidLevel(level string) bool {
	validLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warn": true, "error": true, "fatal": true, "panic": true,
	}
	return validLevels[level]
}

// FromLegacyConfig 从旧配置格式转换（兼容性）
func FromLegacyConfig(legacy *LegacyConfig) *Config {
	cfg := DefaultConfig()
	
	// 转换核心配置
	cfg.Core.Level = legacy.Level
	
	// 转换输出配置
	if legacy.Stdout == "" || legacy.Stdout == "default" {
		// 空或 default：输出到控制台
		cfg.Output.Console.Enabled = true
		cfg.Output.File.Enabled = false
	} else if legacy.Stdout == "file" {
		// file：只输出到文件
		cfg.Output.Console.Enabled = false
		cfg.Output.File.Enabled = true
	} else {
		// 其他：控制台 + 文件
		cfg.Output.Console.Enabled = true
		cfg.Output.File.Enabled = true
	}
	
	// 设置文件路径
	if legacy.Path != "" {
		cfg.Output.File.Path = legacy.Path + "/app.log"
	}
	
	return cfg
}

// LegacyConfig 旧配置格式（向后兼容）
type LegacyConfig struct {
	Path      string `json:"path" yaml:"path"`
	Stdout    string `json:"stdout" yaml:"stdout"`
	Level     string `json:"level" yaml:"level"`
	EnabledDB bool   `json:"enableddb" yaml:"enableddb"`
}
