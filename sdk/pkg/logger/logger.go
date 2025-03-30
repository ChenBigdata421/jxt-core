package logger

import (
	"context"
	"os"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ContextKey string

const (
	TrafficKey ContextKey = "JXT-Request-Id"
	LoggerKey  ContextKey = "_jxt-evidence-zap-logger-request"
)

var (
	Logger        *zap.Logger        //全局ZapLogger打印
	DefaultLogger *zap.SugaredLogger //全局SugarLogger打印，用于简易打印
)

// jiyuanjie add in order to  设置logger中间件
func SetRequestLogger(c *gin.Context) {
	requestId := pkg.GenerateMsgIDFromContext(c) // 创建一个唯一的 TraceID
	ctx := context.WithValue(c.Request.Context(), TrafficKey, requestId)
	// 创建一个带有 TraceID 的 logger
	requestLogger := Logger.With(zap.String(string(TrafficKey), requestId))
	ctx = context.WithValue(ctx, LoggerKey, requestLogger)
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}

// jiyuanjie add in order to 从上下文获得logger
func GetRequestLogger(c *gin.Context) *zap.Logger {
	ctx := c.Request.Context()
	// 从上下文中获取 logger
	requestLogger, ok := ctx.Value(LoggerKey).(*zap.Logger)
	if !ok {
		// 如果没有找到 logger，使用默认 logger
		requestLogger = Logger
	}
	return requestLogger
}

func Info(args ...interface{}) {
	DefaultLogger.Info(args...)
}

func Infof(template string, args ...interface{}) {
	DefaultLogger.Infof(template, args...)
}

func Trace(args ...interface{}) {
	DefaultLogger.Info(args...)
}

func Tracef(template string, args ...interface{}) {
	DefaultLogger.Infof(template, args...)
}

func Debug(args ...interface{}) {
	DefaultLogger.Debug(args...)
}

func Debugf(template string, args ...interface{}) {
	DefaultLogger.Debugf(template, args...)
}

func Warn(args ...interface{}) {
	DefaultLogger.Warn(args...)
}

func Warnf(template string, args ...interface{}) {
	DefaultLogger.Warnf(template, args...)
}

func Error(args ...interface{}) {
	DefaultLogger.Error(args...)
}

func Errorf(template string, args ...interface{}) {
	DefaultLogger.Errorf(template, args...)
}

func Fatal(args ...interface{}) {
	DefaultLogger.Fatal(args...)
	os.Exit(1)
}

func Fatalf(template string, args ...interface{}) {
	DefaultLogger.Fatalf(template, args...)
	os.Exit(1)
}
