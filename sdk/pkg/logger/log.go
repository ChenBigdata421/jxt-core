package logger

import (
	"io"
	"os"

	"github.com/go-admin-team/go-admin-core/logger"
	"github.com/go-admin-team/go-admin-core/logger/plugins/zap"
	"github.com/go-admin-team/go-admin-core/logger/writer"
	"github.com/go-admin-team/go-admin-core/sdk/pkg"

	log "github.com/go-admin-team/go-admin-core/logger"
)

// SetupLogger 日志 cap 单位为kb
func SetupLogger(opts ...Option) logger.Logger {
	op := setDefault()
	for _, o := range opts {
		o(&op)
	}
	if !pkg.PathExist(op.path) {
		err := pkg.PathCreate(op.path)
		if err != nil {
			log.Fatalf("create dir error: %s", err.Error())
		}
	}
	var err error
	var output io.Writer
	switch op.stdout {
	case "file":
		output, err = writer.NewFileWriter(
			writer.WithPath(op.path),
			writer.WithCap(op.cap<<10),
		)
		if err != nil {
			log.Fatal("logger setup error: %s", err.Error())
		}
	default:
		output = os.Stdout
	}
	var level logger.Level
	level, err = logger.GetLevel(op.level)
	if err != nil {
		log.Fatalf("get logger level error, %s", err.Error())
	}

	// 初始化日志实现
	switch op.driver {
	case "zap":
		// 使用 Zap 高性能日志库
		log.DefaultLogger, err = zap.NewLogger(
			logger.WithLevel(level),
			zap.WithOutput(output),
			zap.WithCallerSkip(2),
		)
		if err != nil {
			log.Fatalf("new zap logger error, %s", err.Error())
		}
	default:
		// 使用默认日志实现
		log.DefaultLogger = logger.NewLogger(
			logger.WithLevel(level),
			logger.WithOutput(output),
		)
	}
	return log.DefaultLogger
}
