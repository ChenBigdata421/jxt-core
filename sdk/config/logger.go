package config

// Logger 日志配置
type Logger struct {
	Type            string `mapstructure:"type"`            // zap，logrus，或自研，无用删除by jiyuanjje
	Path            string `mapstructure:"path"`            // 日志文件路径
	Level           string `mapstructure:"level"`           // 日志级别
	Stdout          bool   `mapstructure:"stdout"`          // 是否输出到标准控制台（true：输出，false：不输出）
	MaxSize         int    `mapstructure:"maxsize"`         // 每个日志文件最大多少MB，一般设置50MB add by jiyuanjje
	ErrorMaxAge     int    `mapstructure:"errormaxage"`     // error日志文件保留天数，一般设置14天 add by jiyuanjje
	InfoMaxAge      int    `mapstructure:"infomaxage"`      // info日志文件保留天数，一般设置3天 add by jiyuanjje
	MaxBackups      int    `mapstructure:"maxbackups"`      // 日志文件保留个数，一般设置20个 add by jiyuanjje
	EnabledDB       bool   `mapstructure:"enableddb"`       // 是否启用数据库日志(true：启用，false：不启用)
	GormLoggerLevel int    `mapstructure:"gormloggerlevel"` // 数据库日志打印级别（4：Info，3 Warn，2 Error，1 Silent）add by jiyuanjie
}

var LoggerConfig = new(Logger)
