package logger

import (
	"context"
	"io"
	"os"
	"path/filepath"

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
	
	// 控制台输出
	if options.Stdout {
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

	// 启用调用者信息
	log.SetReportCaller(true)

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
	switch level {
	case TraceLevel, DebugLevel:
		l.entry.Debug(v...)
	case InfoLevel:
		l.entry.Info(v...)
	case WarnLevel:
		l.entry.Warn(v...)
	case ErrorLevel:
		l.entry.Error(v...)
	case FatalLevel:
		l.entry.Fatal(v...)
	}
}

func (l *logrusAdapter) Logf(level Level, format string, v ...interface{}) {
	switch level {
	case TraceLevel, DebugLevel:
		l.entry.Debugf(format, v...)
	case InfoLevel:
		l.entry.Infof(format, v...)
	case WarnLevel:
		l.entry.Warnf(format, v...)
	case ErrorLevel:
		l.entry.Errorf(format, v...)
	case FatalLevel:
		l.entry.Fatalf(format, v...)
	}
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
