package mycasbin

import (
	"sync/atomic"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// Logger is the implementation for a Logger using zap logger.
type Logger struct {
	enable int32
	logger *zap.Logger
}

// NewLogger creates a new Logger instance
func NewLogger() *Logger {
	//logger, _ := zap.NewProduction()
	//使用sdk/pkg/logger
	if logger.Logger != nil {
		return &Logger{
			enable: 1,
			logger: logger.Logger,
		}
	}

	// 异常处理，当发现logger.Logger为nil时，创建一个新的zapLogger
	logger, _ := zap.NewProduction()
	return &Logger{
		enable: 0,
		logger: logger,
	}
}

// EnableLog controls whether print the message.
func (l *Logger) EnableLog(enable bool) {
	i := 0
	if enable {
		i = 1
	}
	atomic.StoreInt32(&(l.enable), int32(i))
}

// IsEnabled returns if logger is enabled.
func (l *Logger) IsEnabled() bool {
	return atomic.LoadInt32(&(l.enable)) != 0
}

// LogModel log info related to model.
func (l *Logger) LogModel(model [][]string) {
	if !l.IsEnabled() {
		return
	}
	var str string
	for i := range model {
		for j := range model[i] {
			str += " " + model[i][j]
		}
		str += "\n"
	}
	l.logger.Info(str)
}

// LogEnforce log info related to enforce.
func (l *Logger) LogEnforce(matcher string, request []interface{}, result bool, explains [][]string) {
	if !l.IsEnabled() {
		return
	}
	l.logger.Info("casbin enforce",
		zap.String("matcher", matcher),
		zap.Any("request", request),
		zap.Bool("result", result),
		zap.Any("explains", explains))
}

// LogRole log info related to role.
func (l *Logger) LogRole(roles []string) {
	if !l.IsEnabled() {
		return
	}
	l.logger.Info("casbin roles", zap.Strings("roles", roles))
}

// LogPolicy log info related to policy.
func (l *Logger) LogPolicy(policy map[string][][]string) {
	if !l.IsEnabled() {
		return
	}
	l.logger.Info("casbin policy", zap.Any("policy", policy))
}
