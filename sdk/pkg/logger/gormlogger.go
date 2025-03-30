package logger

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm/logger"
)

type CustomGormLogger struct {
	ZapLogger *zap.Logger
	LogLevel  logger.LogLevel
}

// 创建自定义 GORM 日志器
func NewGormLogger(baseLogger *zap.Logger, gormLogLevel int) logger.Interface {
	return &CustomGormLogger{
		ZapLogger: baseLogger.Named("gorm"),
		LogLevel:  logger.LogLevel(gormLogLevel),
	}
}

// 设置日志级别
func (l *CustomGormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &CustomGormLogger{
		ZapLogger: l.ZapLogger,
		LogLevel:  level,
	}
}

// 实现必要的接口方法
func (l *CustomGormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Info {
		l.ZapLogger.Sugar().Infof(msg, data...)
	}
}

func (l *CustomGormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Warn {
		l.ZapLogger.Sugar().Warnf(msg, data...)
	}
}

func (l *CustomGormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.LogLevel >= logger.Error {
		l.ZapLogger.Sugar().Errorf(msg, data...)
	}
}

// 这是关键方法，我们修改它来记录所有 SQL
func (l *CustomGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	// 关键修改：不管有没有错误，都记录 SQL
	if err != nil && l.LogLevel >= logger.Error {
		l.ZapLogger.Sugar().Errorf("SQL错误: %s, 耗时: %v, 影响行数: %d, SQL: %s", err, elapsed, rows, sql)
	} else if elapsed > 200*time.Millisecond && l.LogLevel >= logger.Warn {
		l.ZapLogger.Sugar().Warnf("慢SQL: 耗时: %v, 影响行数: %d, SQL: %s", elapsed, rows, sql)
	} else if l.LogLevel >= logger.Info {
		// 这是关键：即使是正常 SQL 也记录为 INFO
		l.ZapLogger.Sugar().Infof("SQL: 耗时: %v, 影响行数: %d, SQL: %s", elapsed, rows, sql)
	}
}
