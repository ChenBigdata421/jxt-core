package config

type Logger struct {
	//Type            string // zap，logrus，或自研，无用删除by jiyuanjje
	Path   string // 日志文件路径
	Level  string // 日志级别
	Stdout bool   // 是否输出到标准控制台（true：输出，false：不输出）
	//Cap             uint   // 缓存大小，无用，删除by jiyuanjje
	MaxSize         int  // 每个日志文件最大多少MB，一般设置50MB add by jiyuanjje
	ErrorMaxAge     int  // error日志文件保留天数，一般设置14天 add by jiyuanjje
	InfoMaxAge      int  // info日志文件保留天数，一般设置3天 add by jiyuanjje
	MaxBackups      int  // 日志文件保留个数，一般设置20个 add by jiyuanjje
	EnabledDB       bool // 是否启用数据库日志(true：启用，false：不启用)
	GormLoggerLevel int  // 数据库日志打印级别（4：Info，3 Warn，2 Error，1 Silent）add by jiyuanjje
}

var LoggerConfig = new(Logger)
