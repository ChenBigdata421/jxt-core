package logger

import (
	"context"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// zapLogger zap 日志实现（高性能，零分配）
type zapLogger struct {
	logger  *zap.Logger
	sugar   *zap.SugaredLogger
	opts    Options
	logFile *os.File // 持有文件句柄以便关闭
}

// NewZapLogger 创建基于 zap 的高性能 logger
func NewZapLogger(opts ...Option) Logger {
	options := DefaultOptions()
	options.Level = InfoLevel
	options.CallerSkipCount = 2
	options.Name = "go-admin"
	
	for _, o := range opts {
		o(&options)
	}

	// 配置 encoder
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 选择输出格式
	var encoder zapcore.Encoder
	if options.Stdout {
		// 开发环境：彩色输出
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		// 生产环境：JSON 格式
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 配置输出
	var writers []zapcore.WriteSyncer
	var logFile *os.File
	if options.Stdout {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}
	if options.Path != "" {
		var err error
		logFile, err = os.OpenFile(options.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			// 文件打开失败时记录错误并继续（只输出到控制台）
			if options.Stdout {
				// 如果有控制台输出，打印警告
				os.Stderr.WriteString("Warning: failed to open log file: " + err.Error() + "\n")
			}
		} else {
			writers = append(writers, zapcore.AddSync(logFile))
		}
	}
	writer := zapcore.NewMultiWriteSyncer(writers...)

	// 配置日志级别
	zapLevel := toZapLevel(options.Level)
	core := zapcore.NewCore(encoder, writer, zapLevel)

	// 创建 logger
	zapLog := zap.New(core,
		zap.AddCaller(),
		zap.AddCallerSkip(options.CallerSkipCount),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)

	if options.Name != "" {
		zapLog = zapLog.Named(options.Name)
	}

	return &zapLogger{
		logger:  zapLog,
		sugar:   zapLog.Sugar(),
		opts:    options,
		logFile: logFile, // 保存文件句柄
	}
}

func (z *zapLogger) Init(opts ...Option) error {
	for _, o := range opts {
		o(&z.opts)
	}
	
	// P1 FIX: 关闭旧资源避免内存泄漏
	if z.logFile != nil {
		z.logFile.Close()
	}
	if z.logger != nil {
		z.logger.Sync() // 刷新缓冲区
	}
	
	// 重新创建 logger
	newLogger := NewZapLogger(opts...).(*zapLogger)
	z.logger = newLogger.logger
	z.sugar = newLogger.sugar
	z.opts = newLogger.opts
	z.logFile = newLogger.logFile
	
	return nil
}

func (z *zapLogger) Options() Options {
	return z.opts
}

func (z *zapLogger) Fields(fields map[string]interface{}) Logger {
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	return &zapLogger{
		logger: z.logger.With(zapFields...),
		sugar:  z.logger.With(zapFields...).Sugar(),
		opts:   z.opts,
	}
}

func (z *zapLogger) Log(level Level, v ...interface{}) {
	switch level {
	case TraceLevel, DebugLevel:
		z.sugar.Debug(v...)
	case InfoLevel:
		z.sugar.Info(v...)
	case WarnLevel:
		z.sugar.Warn(v...)
	case ErrorLevel:
		z.sugar.Error(v...)
	case FatalLevel:
		z.sugar.Fatal(v...)
	}
}

func (z *zapLogger) Logf(level Level, format string, v ...interface{}) {
	switch level {
	case TraceLevel, DebugLevel:
		z.sugar.Debugf(format, v...)
	case InfoLevel:
		z.sugar.Infof(format, v...)
	case WarnLevel:
		z.sugar.Warnf(format, v...)
	case ErrorLevel:
		z.sugar.Errorf(format, v...)
	case FatalLevel:
		z.sugar.Fatalf(format, v...)
	}
}

func (z *zapLogger) String() string {
	return "zap"
}

// Close 关闭日志文件并刷新缓冲区（P0 FIX: 防止资源泄漏）
func (z *zapLogger) Close() error {
	var err error
	if z.logger != nil {
		if syncErr := z.logger.Sync(); syncErr != nil {
			err = syncErr
		}
	}
	if z.logFile != nil {
		if closeErr := z.logFile.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		z.logFile = nil
	}
	return err
}

// 结构化日志 API（零分配）
func (z *zapLogger) Info(msg string, fields ...Field) {
	if !z.opts.Level.Enabled(InfoLevel) {
		return
	}
	z.logger.Info(msg, toZapFields(fields)...)
}

func (z *zapLogger) Debug(msg string, fields ...Field) {
	if !z.opts.Level.Enabled(DebugLevel) {
		return
	}
	z.logger.Debug(msg, toZapFields(fields)...)
}

func (z *zapLogger) Warn(msg string, fields ...Field) {
	if !z.opts.Level.Enabled(WarnLevel) {
		return
	}
	z.logger.Warn(msg, toZapFields(fields)...)
}

func (z *zapLogger) Error(msg string, fields ...Field) {
	if !z.opts.Level.Enabled(ErrorLevel) {
		return
	}
	z.logger.Error(msg, toZapFields(fields)...)
}

func (z *zapLogger) WithContext(ctx context.Context) Logger {
	// 提取 context 中的字段（trace_id、user_id 等）
	fields := extractContextFields(ctx)
	if len(fields) == 0 {
		return z
	}
	return z.Fields(fields)
}

func (z *zapLogger) With(fields ...Field) Logger {
	return &zapLogger{
		logger: z.logger.With(toZapFields(fields)...),
		sugar:  z.logger.With(toZapFields(fields)...).Sugar(),
		opts:   z.opts,
	}
}

func (z *zapLogger) Sync() error {
	return z.logger.Sync()
}

// 辅助函数

func toZapLevel(level Level) zapcore.Level {
	switch level {
	case TraceLevel, DebugLevel:
		return zapcore.DebugLevel
	case InfoLevel:
		return zapcore.InfoLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	case FatalLevel:
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

func toZapFields(fields []Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, f := range fields {
		switch f.Type {
		case FieldTypeString:
			zapFields[i] = zap.String(f.Key, f.String)
		case FieldTypeInt:
			zapFields[i] = zap.Int64(f.Key, f.Int64)
		case FieldTypeBool:
			zapFields[i] = zap.Bool(f.Key, f.Int64 != 0)
		case FieldTypeDuration:
			zapFields[i] = zap.Duration(f.Key, time.Duration(f.Int64))
		case FieldTypeTime:
			zapFields[i] = zap.Time(f.Key, time.Unix(0, f.Int64))
		case FieldTypeError:
			if f.Any != nil {
				zapFields[i] = zap.NamedError(f.Key, f.Any.(error))
			} else {
				zapFields[i] = zap.Skip()
			}
		default:
			zapFields[i] = zap.Any(f.Key, f.Any)
		}
	}
	return zapFields
}

func extractContextFields(ctx context.Context) map[string]interface{} {
	fields := make(map[string]interface{})
	
	// 提取 trace_id（支持 OpenTelemetry）
	if traceID := ctx.Value("trace_id"); traceID != nil {
		fields["trace_id"] = traceID
	}
	
	// 提取 user_id
	if userID := ctx.Value("user_id"); userID != nil {
		fields["user_id"] = userID
	}
	
	// 提取 request_id
	if requestID := ctx.Value("request_id"); requestID != nil {
		fields["request_id"] = requestID
	}
	
	return fields
}
