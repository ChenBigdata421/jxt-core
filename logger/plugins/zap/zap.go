package zap

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/go-admin-team/go-admin-core/logger"
)

type zaplog struct {
	cfg  zap.Config
	zap  *zap.Logger
	opts logger.Options
	sync.RWMutex
	fields map[string]interface{}
}

func (l *zaplog) Init(opts ...logger.Option) error {
	for _, o := range opts {
		o(&l.opts)
	}

	zapConfig := zap.NewProductionConfig()
	if zconfig, ok := l.opts.Context.Value(configKey{}).(zap.Config); ok {
		zapConfig = zconfig
	}

	if zcconfig, ok := l.opts.Context.Value(encoderConfigKey{}).(zapcore.EncoderConfig); ok {
		zapConfig.EncoderConfig = zcconfig
	}

	writer, ok := l.opts.Context.Value(writerKey{}).(io.Writer)
	if !ok {
		writer = os.Stdout
	}

	skip, ok := l.opts.Context.Value(callerSkipKey{}).(int)
	if !ok || skip < 1 {
		skip = 1
	}

	// Set log Level if not default
	zapConfig.Level = zap.NewAtomicLevel()
	if l.opts.Level != logger.InfoLevel {
		zapConfig.Level.SetLevel(loggerToZapLevel(l.opts.Level))
	}
	zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapConfig.EncoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(writer)),
		zapConfig.Level)

	log := zap.New(logCore, zap.AddCaller(), zap.AddCallerSkip(skip), zap.AddStacktrace(zap.DPanicLevel))

	// Adding seed fields if exist
	if l.opts.Fields != nil {
		fields := make([]zap.Field, 0, len(l.opts.Fields))
		for k, v := range l.opts.Fields {
			fields = append(fields, zap.Any(k, v))
		}
		log = log.With(fields...)
	}

	// Adding namespace
	if namespace, ok := l.opts.Context.Value(namespaceKey{}).(string); ok {
		log = log.With(zap.Namespace(namespace))
	}

	l.cfg = zapConfig
	l.zap = log
	l.fields = make(map[string]interface{})

	return nil
}

func (l *zaplog) Fields(fields map[string]interface{}) logger.Logger {
	l.Lock()
	defer l.Unlock()
	nfields := make(map[string]interface{}, len(l.fields)+len(fields))
	for k, v := range l.fields {
		nfields[k] = v
	}
	for k, v := range fields {
		nfields[k] = v
	}

	zapFields := make([]zap.Field, 0, len(nfields))
	for k, v := range nfields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	return &zaplog{
		cfg:    l.cfg,
		zap:    l.zap.With(zapFields...),
		opts:   l.opts,
		fields: nfields,
	}
}

func (l *zaplog) Error(err error) logger.Logger {
	return l.Fields(map[string]interface{}{"error": err})
}

func (l *zaplog) Log(level logger.Level, args ...interface{}) {
	l.RLock()
	defer l.RUnlock()
	zapFields := make([]zap.Field, 0, len(l.fields))
	for k, v := range l.fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	msg := fmt.Sprint(args...)
	switch loggerToZapLevel(level) {
	case zap.DebugLevel:
		l.zap.Debug(msg, zapFields...)
	case zap.InfoLevel:
		l.zap.Info(msg, zapFields...)
	case zap.WarnLevel:
		l.zap.Warn(msg, zapFields...)
	case zap.ErrorLevel:
		l.zap.Error(msg, zapFields...)
	case zap.FatalLevel:
		l.zap.Fatal(msg, zapFields...)
	}
}

func (l *zaplog) Logf(level logger.Level, format string, args ...interface{}) {
	l.RLock()
	defer l.RUnlock()
	zapFields := make([]zap.Field, 0, len(l.fields))
	for k, v := range l.fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	msg := fmt.Sprintf(format, args...)
	switch loggerToZapLevel(level) {
	case zap.DebugLevel:
		l.zap.Debug(msg, zapFields...)
	case zap.InfoLevel:
		l.zap.Info(msg, zapFields...)
	case zap.WarnLevel:
		l.zap.Warn(msg, zapFields...)
	case zap.ErrorLevel:
		l.zap.Error(msg, zapFields...)
	case zap.FatalLevel:
		l.zap.Fatal(msg, zapFields...)
	}
}

func (l *zaplog) String() string {
	return "zap"
}

func (l *zaplog) Options() logger.Options {
	return l.opts
}

// NewLogger New builds a new logger based on options
func NewLogger(opts ...logger.Option) (logger.Logger, error) {
	options := logger.Options{
		Level:   logger.InfoLevel,
		Fields:  make(map[string]interface{}),
		Out:     os.Stderr,
		Context: context.Background(),
	}

	l := &zaplog{opts: options}
	if err := l.Init(opts...); err != nil {
		return nil, err
	}

	return l, nil
}

func loggerToZapLevel(level logger.Level) zapcore.Level {
	switch level {
	case logger.TraceLevel, logger.DebugLevel:
		return zap.DebugLevel
	case logger.InfoLevel:
		return zap.InfoLevel
	case logger.WarnLevel:
		return zap.WarnLevel
	case logger.ErrorLevel:
		return zap.ErrorLevel
	case logger.FatalLevel:
		return zap.FatalLevel
	default:
		return zap.InfoLevel
	}
}

func zapToLoggerLevel(level zapcore.Level) logger.Level {
	switch level {
	case zap.DebugLevel:
		return logger.DebugLevel
	case zap.InfoLevel:
		return logger.InfoLevel
	case zap.WarnLevel:
		return logger.WarnLevel
	case zap.ErrorLevel:
		return logger.ErrorLevel
	case zap.FatalLevel:
		return logger.FatalLevel
	default:
		return logger.InfoLevel
	}
}
