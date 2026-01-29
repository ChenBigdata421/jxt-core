package config

import (
	"github.com/go-admin-team/go-admin-core/logger"
	"github.com/go-admin-team/go-admin-core/sdk/pkg"
	log "github.com/go-admin-team/go-admin-core/logger"
)

type Logger struct {
	Type      string
	Path      string
	Level     string
	Stdout    string
	EnabledDB bool
	Cap       uint
}

// Setup 设置logger（使用新的 logger 架构）
func (e Logger) Setup() {
	// 确保目录存在
	if e.Path != "" && !pkg.PathExist(e.Path) {
		if err := pkg.PathCreate(e.Path); err != nil {
			log.Fatalf("create log dir error: %s", err.Error())
		}
	}

	// 构建选项
	opts := []logger.Option{
		logger.WithName("go-admin"),
		logger.WithCallerSkipCount(2),
	}

	// 设置日志级别
	if level, err := logger.GetLevel(e.Level); err == nil {
		opts = append(opts, logger.WithLevel(level))
	}

	// 设置输出
	if e.Stdout == "file" || e.Stdout == "" {
		opts = append(opts, logger.WithStdout(false))
	} else {
		opts = append(opts, logger.WithStdout(true))
	}

	// 设置文件路径
	if e.Path != "" {
		opts = append(opts, logger.WithPath(e.Path))
	}

	// 选择适配器
	switch e.Type {
	case "zap":
		log.DefaultLogger = logger.NewZapLogger(opts...)
	default:
		log.DefaultLogger = logger.NewLogrusLogger(opts...)
	}
}

var LoggerConfig = new(Logger)
