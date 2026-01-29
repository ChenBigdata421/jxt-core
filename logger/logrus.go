package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// logrusAdapter Logrus 适配器（生态丰富，插件多）
type logrusAdapter struct {
	logger *logrus.Logger
	entry  *logrus.Entry
	opts   Options
}

// NewLogrusLogger 创建基于 Logrus 的 logger
func NewLogrusLogger(opts ...Option) Logger {
	options := DefaultOptions()
	options.Level = InfoLevel
	options.CallerSkipCount = 2
	options.Name = "go-admin"
	
	for _, o := range opts {
		o(&options)
	}

	// 创建 Logrus Logger
	log := logrus.New()

	// 设置日志级别
	log.SetLevel(toLogrusLevel(options.Level))

	// 设置输出
	writers := []io.Writer{}
	
	// 自定义输出（用于测试或特殊场景）
	if options.Out != nil {
		writers = append(writers, options.Out)
	}
	
	// 控制台输出
	if options.Stdout && options.Out == nil {
		writers = append(writers, os.Stdout)
	}
	
	// 文件输出（支持日志轮转）
	if options.Path != "" {
		// 确保目录存在
		dir := filepath.Dir(options.Path)
		if err := os.MkdirAll(dir, 0755); err == nil {
			fileWriter := &lumberjack.Logger{
				Filename:   options.Path,
				MaxSize:    options.MaxSize,
				MaxBackups: options.MaxBackups,
				MaxAge:     options.MaxAge,
				Compress:   options.Compress,
				LocalTime:  options.LocalTime,
			}
			writers = append(writers, fileWriter)
		}
	}
	
	if len(writers) > 0 {
		log.SetOutput(io.MultiWriter(writers...))
	} else {
		// 默认输出到 os.Stderr
		log.SetOutput(os.Stderr)
	}

	// 设置格式化器
	if options.Path != "" && !options.Stdout {
		// 仅文件输出使用 JSON 格式
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "time",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "msg",
				logrus.FieldKeyFunc:  "caller",
			},
		})
	} else {
		// 控制台输出使用友好格式
		log.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			FullTimestamp:   true,
			ForceColors:     true,
			DisableColors:   false,
			PadLevelText:    true,
		})
	}

	// 禁用 Logrus 内置的 ReportCaller（我们在 getEntryWithCaller 中手动处理）
	log.SetReportCaller(false)

	// 创建 Entry（带默认字段）
	entry := log.WithFields(logrus.Fields{})
	if options.Name != "" {
		entry = entry.WithField("logger", options.Name)
	}
	if options.Fields != nil {
		entry = entry.WithFields(logrus.Fields(options.Fields))
	}

	return &logrusAdapter{
		logger: log,
		entry:  entry,
		opts:   options,
	}
}

func (l *logrusAdapter) Init(opts ...Option) error {
	for _, o := range opts {
		o(&l.opts)
	}
	// 重新创建 logger
	newLogger := NewLogrusLogger(opts...)
	if adapter, ok := newLogger.(*logrusAdapter); ok {
		l.logger = adapter.logger
		l.entry = adapter.entry
		l.opts = adapter.opts
	}
	return nil
}

func (l *logrusAdapter) Options() Options {
	return l.opts
}

func (l *logrusAdapter) Fields(fields map[string]interface{}) Logger {
	return &logrusAdapter{
		logger: l.logger,
		entry:  l.entry.WithFields(logrus.Fields(fields)),
		opts:   l.opts,
	}
}

func (l *logrusAdapter) Log(level Level, v ...interface{}) {
	// 获取真实调用者位置（跳过 logger 包装层）
	entry := l.getEntryWithCaller(2)
	
	switch level {
	case TraceLevel, DebugLevel:
		entry.Debug(v...)
	case InfoLevel:
		entry.Info(v...)
	case WarnLevel:
		entry.Warn(v...)
	case ErrorLevel:
		entry.Error(v...)
	case FatalLevel:
		entry.Fatal(v...)
	}
}

func (l *logrusAdapter) Logf(level Level, format string, v ...interface{}) {
	// 获取真实调用者位置（跳过 logger 包装层）
	entry := l.getEntryWithCaller(2)
	
	switch level {
	case TraceLevel, DebugLevel:
		entry.Debugf(format, v...)
	case InfoLevel:
		entry.Infof(format, v...)
	case WarnLevel:
		entry.Warnf(format, v...)
	case ErrorLevel:
		entry.Errorf(format, v...)
	case FatalLevel:
		entry.Fatalf(format, v...)
	}
}

// getEntryWithCaller 获取带有正确 Caller 信息的 Entry
func (l *logrusAdapter) getEntryWithCaller(skip int) *logrus.Entry {
	// 查找调用栈中第一个非 logger 包的位置
	pcs := make([]uintptr, 30)
	depth := runtime.Callers(0, pcs)
	frames := runtime.CallersFrames(pcs[:depth])
	
	for {
		frame, more := frames.Next()
		file := frame.File
		fn := frame.Function
		
		// 跳过以下文件：
		// 1. logger 包内部实现文件（logrus.go, zap.go, default.go, factory.go 等）
		// 2. logrus 库内部文件（entry.go 等）
		// 3. gorm logger 适配器（tools/gorm/gormlog/logger.go 等）
		// 4. testing 框架（testing.go, run.go 等）
		// 5. runtime 包
		
		// 只检查文件名，不检查函数名（避免误判测试函数）
		isLoggerImpl := strings.Contains(file, "/logger/logrus.go") ||
		                strings.Contains(file, "/logger/zap.go") ||
		                strings.Contains(file, "/logger/default.go") ||
		                strings.Contains(file, "/logger/factory.go") ||
		                strings.Contains(file, "/logger/sampling.go")
		isGormLogger := strings.Contains(file, "/gorm/gormlog/") || 
		                strings.Contains(file, "/tools/gorm/") ||
		                strings.Contains(file, "/gorm@")
		isLogrus := strings.Contains(file, "/logrus@")
		isTesting := strings.Contains(file, "/testing/") || strings.Contains(fn, "testing.tRunner")
		isRuntime := strings.Contains(file, "/runtime/")
		
		// 找到第一个业务代码（或测试代码）
		if !isLoggerImpl && !isGormLogger && !isLogrus && !isTesting && !isRuntime {
			return l.entry.WithField("caller", formatCaller(frame))
		}
		if !more {
			break
		}
	}
	
	return l.entry
}

// formatCaller 格式化调用者信息为相对路径格式
func formatCaller(frame runtime.Frame) string {
	file := frame.File
	
	// 尝试提取项目相对路径（保留更多上下文信息）
	// 查找常见的项目根目录标识：go-admin-core/, whitelist-api/, app/, common/ 等
	markers := []string{
		"/go-admin-core/",
		"/whitelist-api/",
		"/whitelist-antd/",
		"/app/",
		"/common/",
		"/pkg/",
		"/cmd/",
		"/internal/",
	}
	
	for _, marker := range markers {
		if idx := strings.LastIndex(file, marker); idx >= 0 {
			// 保留从标识符之后的路径
			file = file[idx+len(marker):]
			return fmt.Sprintf("%s:%d", file, frame.Line)
		}
	}
	
	// 如果没找到标识符，至少保留最后两级目录
	parts := strings.Split(file, "/")
	if len(parts) >= 3 {
		file = strings.Join(parts[len(parts)-3:], "/")
	} else if len(parts) >= 2 {
		file = strings.Join(parts[len(parts)-2:], "/")
	}
	
	return fmt.Sprintf("%s:%d", file, frame.Line)
}

func (l *logrusAdapter) String() string {
	return "logrus"
}

// 结构化日志 API（扩展接口）
func (l *logrusAdapter) Info(msg string, fields ...Field) {
	if !l.opts.Level.Enabled(InfoLevel) {
		return
	}
	l.entry.WithFields(toLogrusFields(fields)).Info(msg)
}

func (l *logrusAdapter) Debug(msg string, fields ...Field) {
	if !l.opts.Level.Enabled(DebugLevel) {
		return
	}
	l.entry.WithFields(toLogrusFields(fields)).Debug(msg)
}

func (l *logrusAdapter) Warn(msg string, fields ...Field) {
	if !l.opts.Level.Enabled(WarnLevel) {
		return
	}
	l.entry.WithFields(toLogrusFields(fields)).Warn(msg)
}

func (l *logrusAdapter) Error(msg string, fields ...Field) {
	if !l.opts.Level.Enabled(ErrorLevel) {
		return
	}
	l.entry.WithFields(toLogrusFields(fields)).Error(msg)
}

func (l *logrusAdapter) WithContext(ctx context.Context) Logger {
	// 提取 context 中的字段
	fields := extractContextFields(ctx)
	if len(fields) == 0 {
		return l
	}
	return l.Fields(fields)
}

func (l *logrusAdapter) With(fields ...Field) Logger {
	return &logrusAdapter{
		logger: l.logger,
		entry:  l.entry.WithFields(toLogrusFields(fields)),
		opts:   l.opts,
	}
}

func (l *logrusAdapter) Sync() error {
	// Logrus 不需要显式 Sync
	return nil
}

// 辅助函数

func toLogrusLevel(level Level) logrus.Level {
	switch level {
	case TraceLevel:
		return logrus.TraceLevel
	case DebugLevel:
		return logrus.DebugLevel
	case InfoLevel:
		return logrus.InfoLevel
	case WarnLevel:
		return logrus.WarnLevel
	case ErrorLevel:
		return logrus.ErrorLevel
	case FatalLevel:
		return logrus.FatalLevel
	default:
		return logrus.InfoLevel
	}
}

func toLogrusFields(fields []Field) logrus.Fields {
	result := make(logrus.Fields, len(fields))
	for _, f := range fields {
		switch f.Type {
		case FieldTypeString:
			result[f.Key] = f.String
		case FieldTypeInt:
			result[f.Key] = f.Int64
		case FieldTypeBool:
			result[f.Key] = f.Int64 != 0
		case FieldTypeDuration:
			result[f.Key] = f.Int64
		case FieldTypeTime:
			result[f.Key] = f.Int64
		case FieldTypeError:
			if f.Any != nil {
				result[f.Key] = f.Any
			}
		default:
			result[f.Key] = f.Any
		}
	}
	return result
}

// Logrus Hook 支持

// AddHook 添加 Logrus Hook（Logrus 生态优势）
func (l *logrusAdapter) AddHook(hook logrus.Hook) {
	l.logger.AddHook(hook)
}

// GetLogrusLogger 获取底层 Logrus Logger（用于高级定制）
func (l *logrusAdapter) GetLogrusLogger() *logrus.Logger {
	return l.logger
}

// SetFormatter 设置格式化器
func (l *logrusAdapter) SetFormatter(formatter logrus.Formatter) {
	l.logger.SetFormatter(formatter)
}

// 常用 Hook 示例

// SentryHook Sentry 错误上报 Hook
type SentryHook struct {
	levels []logrus.Level
}

func NewSentryHook() *SentryHook {
	return &SentryHook{
		levels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
		},
	}
}

func (h *SentryHook) Levels() []logrus.Level {
	return h.levels
}

func (h *SentryHook) Fire(entry *logrus.Entry) error {
	// TODO: 集成 Sentry SDK
	// sentry.CaptureException(entry)
	return nil
}

// ElasticsearchHook Elasticsearch 日志存储 Hook
type ElasticsearchHook struct {
	client interface{} // Elasticsearch client
	index  string
	levels []logrus.Level
}

func NewElasticsearchHook(client interface{}, index string) *ElasticsearchHook {
	return &ElasticsearchHook{
		client: client,
		index:  index,
		levels: logrus.AllLevels,
	}
}

func (h *ElasticsearchHook) Levels() []logrus.Level {
	return h.levels
}

func (h *ElasticsearchHook) Fire(entry *logrus.Entry) error {
	// TODO: 将日志发送到 Elasticsearch
	// h.client.Index(h.index, entry)
	return nil
}

// CallerHook 修正调用者信息的 Hook（跳过 logger 包装层）
type CallerHook struct {
	skip int
}

func NewCallerHook(skip int) *CallerHook {
	return &CallerHook{skip: skip}
}

func (h *CallerHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *CallerHook) Fire(entry *logrus.Entry) error {
	// 获取真实的调用者信息
	// 从调用栈中查找第一个非 logger 包的调用者
	pcs := make([]uintptr, 30)
	depth := runtime.Callers(0, pcs)
	frames := runtime.CallersFrames(pcs[:depth])
	
	for {
		frame, more := frames.Next()
		// 跳过以下文件：
		// 1. logger 包内部文件（logrus.go, zap.go, default.go 等）
		// 2. logrus 库内部文件（entry.go, logger.go 等）
		// 3. runtime 包文件
		// 4. testing 包文件（仅在 Go test 运行时）
		if !strings.Contains(frame.File, "/logger/") &&
		   !strings.Contains(frame.File, "/logrus@") &&
		   !strings.Contains(frame.File, "/runtime/") &&
		   !strings.Contains(frame.File, "/testing/") &&
		   !strings.Contains(frame.Function, "github.com/go-admin-team/go-admin-core/logger") &&
		   !strings.Contains(frame.Function, "github.com/sirupsen/logrus") &&
		   !strings.Contains(frame.Function, "runtime.") &&
		   !strings.Contains(frame.Function, "testing.") {
			// 找到第一个业务代码调用位置
			entry.Caller = &runtime.Frame{
				PC:       frame.PC,
				File:     frame.File,
				Line:     frame.Line,
				Function: frame.Function,
			}
			break
		}
		if !more {
			break
		}
	}
	return nil
}

// MetricsHook Prometheus 监控 Hook
type MetricsHook struct {
	// TODO: Prometheus metrics
	levels []logrus.Level
}

func NewMetricsHook() *MetricsHook {
	return &MetricsHook{
		levels: logrus.AllLevels,
	}
}

func (h *MetricsHook) Levels() []logrus.Level {
	return h.levels
}

func (h *MetricsHook) Fire(entry *logrus.Entry) error {
	// TODO: 记录 Prometheus 指标
	// logCounter.WithLabelValues(entry.Level.String()).Inc()
	return nil
}
