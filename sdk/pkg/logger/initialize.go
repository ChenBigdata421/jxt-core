package logger

import (
	"io/ioutil"

	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	toolsConfig "github.com/ChenBigdata421/jxt-core/sdk/config"
)

/*
使用 zap.Logger: 如果你的应用程序对性能要求很高，或者你需要记录结构化日志
（例如，日志需要被机器解析），那么使用 zap.Logger 是更好的选择。
使用 zap.SugaredLogger: 如果你更关注开发的便利性，或者你的日志记录需求相
对简单，zap.SugaredLogger 提供了更友好的接口。
*/

type LogConfig struct {
	Path          string `yaml:"path"`
	ConsoleOutput bool   `yaml:"console_output"`
	Level         string `yaml:"level"`
	FileOutput    bool   `yaml:"file_output"`
	MaxSize       int    `yaml:"max_size"`
	InfoMaxAge    int    `yaml:"info_max_age"`
	ErrorMaxAge   int    `yaml:"error_max_age"`
	MaxBackups    int    `yaml:"max_backups"`
	Compress      bool   `yaml:"compress"`
}

// InitLogger 初始化全局日志记录器，放在程序运行前执行
func Setup() {
	// 配置日志编码器
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

	config := LogConfig{
		Path:          toolsConfig.LoggerConfig.Path,
		ConsoleOutput: toolsConfig.LoggerConfig.Stdout,
		Level:         toolsConfig.LoggerConfig.Level,
		FileOutput:    true,
		MaxSize:       toolsConfig.LoggerConfig.MaxSize,     // 日志文件最大大小，单位MB
		InfoMaxAge:    toolsConfig.LoggerConfig.InfoMaxAge,  // 保留info日志文件的时间，单位天
		ErrorMaxAge:   toolsConfig.LoggerConfig.ErrorMaxAge, // 保留error日志文件的时间，单位天
		MaxBackups:    toolsConfig.LoggerConfig.MaxBackups,  // 保留数量为10个文件
		Compress:      true,                                 // 压缩旧的日志文件
	}

	// 解析日志级别
	var logLevel zapcore.Level
	err := logLevel.UnmarshalText([]byte(config.Level))
	if err != nil {
		// 默认使用info级别
		logLevel = zapcore.InfoLevel
	}

	// 准备写入同步器列表
	var cores []zapcore.Core

	// 创建info日志文件写入器
	infoFileWriteSyncer := getInfoLogWriter(config)

	// 创建error日志文件写入器
	errorFileWriteSyncer := getErrorLogWriter(config)

	// 根据配置的日志级别决定是否添加infoCore
	if logLevel <= zapcore.InfoLevel {
		infoCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			infoFileWriteSyncer,
			zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
				return lvl >= logLevel && lvl < zapcore.ErrorLevel
			}),
		)
		cores = append(cores, infoCore)
	}

	// 始终添加errorCore
	errorCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		errorFileWriteSyncer,
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel
		}),
	)
	cores = append(cores, errorCore)

	// 根据配置决定是否输出到控制台
	if config.ConsoleOutput {
		// 创建控制台编码器配置
		consoleEncoderConfig := encoderConfig
		consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

		// 创建控制台写入同步器
		consoleWriteSyncer := zapcore.AddSync(os.Stdout)

		// 创建控制台core
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderConfig),
			consoleWriteSyncer,
			zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
				return lvl >= logLevel
			}),
		)
		cores = append(cores, consoleCore)
	}

	// 如果没有任何core，添加一个空core防止panic
	if len(cores) == 0 {
		// 创建一个空core，输出到/dev/null
		nullCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(ioutil.Discard),
			zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
				return false // 不记录任何日志
			}),
		)
		cores = append(cores, nullCore)
	}

	// 创建logger
	core := zapcore.NewTee(cores...)

	// 创建 logger 实例
	Logger = zap.New(core, zap.AddCaller())
	// 创建一个简易输出的 SugarLogger 实例
	DefaultLogger = Logger.Sugar()

}

// 创建info日志文件写入器
func getInfoLogWriter(config LogConfig) zapcore.WriteSyncer {
	lumberJackLogger := &lumberjack.Logger{
		Filename:   config.Path + "/info.log",
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.InfoMaxAge,
		Compress:   config.Compress,
	}
	return zapcore.AddSync(lumberJackLogger)
}

// 创建error日志文件写入器
func getErrorLogWriter(config LogConfig) zapcore.WriteSyncer {
	lumberJackLogger := &lumberjack.Logger{
		Filename:   config.Path + "/error.log",
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.ErrorMaxAge,
		Compress:   config.Compress,
	}
	return zapcore.AddSync(lumberJackLogger)
}
