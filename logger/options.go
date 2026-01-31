package logger

import (
	"context"
	"io"
	"os"
)

// Option is a function that configures the logger options
// Option 是一个配置日志记录器选项的函数
type Option func(*Options)

// Options are logger options
// Options 是日志记录器选项
type Options struct {
	// Level is the logging level the logger should log at. default is `InfoLevel`
	// Level 是日志记录器的日志级别，默认是 `InfoLevel`
	Level Level
	// Fields are fields to always be logged
	// Fields 是始终要记录的字段
	Fields map[string]interface{}
	// Out is the output writer for the logger. default is `os.Stderr`
	// Out 是日志记录器的输出写入器，默认是 `os.Stderr`
	Out io.Writer
	// CallerSkipCount is the frame count to skip for file:line info
	// CallerSkipCount 是跳过的帧数，用于文件:行信息
	CallerSkipCount int
	// Context is alternative options
	// Context 是替代选项
	Context context.Context
	// Name is the logger name
	// Name 是日志记录器的名称
	Name string
	// Path is the log file path (optional)
	// Path 是日志文件路径（可选）
	Path string
	// Stdout enables console output
	// Stdout 启用控制台输出
	Stdout bool
	
	// --- 日志轮转配置 ---
	// MaxSize 单个文件最大大小（MB），默认 100
	MaxSize int
	// MaxAge 文件最大保留天数，默认 30
	MaxAge int
	// MaxBackups 最大备份数量，默认 7
	MaxBackups int
	// Compress 是否压缩旧日志，默认 true
	Compress bool
	// LocalTime 使用本地时间（而非 UTC），默认 true
	LocalTime bool
}

// WithFields sets default fields for the logger
// WithFields 设置日志记录器的默认字段
func WithFields(fields map[string]interface{}) Option {
	return func(args *Options) {
		args.Fields = fields
	}
}

// WithLevel sets default level for the logger
// WithLevel 设置日志记录器的默认级别
func WithLevel(level Level) Option {
	return func(args *Options) {
		args.Level = level
	}
}

// WithOutput sets default output writer for the logger
// WithOutput 设置日志记录器的默认输出写入器
func WithOutput(out io.Writer) Option {
	return func(args *Options) {
		args.Out = out
	}
}

// WithCallerSkipCount sets frame count to skip
// WithCallerSkipCount 设置要跳过的帧数
func WithCallerSkipCount(c int) Option {
	return func(args *Options) {
		args.CallerSkipCount = c
	}
}

// WithName sets name for logger
// WithName 设置日志记录器的名称
func WithName(name string) Option {
	return func(args *Options) {
		args.Name = name
	}
}

// SetOption sets a custom option
// SetOption 设置自定义选项
func SetOption(k, v interface{}) Option {
	return func(o *Options) {
		if o.Context == nil {
			o.Context = context.Background()
		}
		o.Context = context.WithValue(o.Context, k, v)
	}
}

// WithStdout sets whether to enable console output
// WithStdout 设置是否启用控制台输出
func WithStdout(enabled bool) Option {
	return func(args *Options) {
		args.Stdout = enabled
	}
}

// WithPath sets the log file path
// WithPath 设置日志文件路径
func WithPath(path string) Option {
	return func(args *Options) {
		args.Path = path
	}
}

// WithSampling sets sampling configuration for the logger
// WithSampling 设置日志采样配置
func WithSampling(config SamplingConfig) Option {
	return func(o *Options) {
		SetOption("sampling", config)(o)
	}
}

// WithAsync sets async configuration for the logger
// WithAsync 设置异步日志配置
func WithAsync(config AsyncConfig) Option {
	return func(o *Options) {
		SetOption("async", config)(o)
	}
}

// WithSanitizer sets sanitizer configuration for the logger
// WithSanitizer 设置脱敏配置
func WithSanitizer(config SanitizerConfig) Option {
	return func(o *Options) {
		SetOption("sanitizer", config)(o)
	}
}

// DefaultOptions returns default options
// DefaultOptions 返回默认选项
func DefaultOptions() Options {
	return Options{
		Level:           InfoLevel,
		Fields:          make(map[string]interface{}),
		Out:             os.Stderr,
		CallerSkipCount: 3,
		Context:         context.Background(),
		Name:            "",
		MaxSize:         100,
		MaxAge:          30,
		MaxBackups:      7,
		Compress:        true,
		LocalTime:       true,
	}
}
