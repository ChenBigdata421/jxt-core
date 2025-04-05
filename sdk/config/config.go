package config

// Config 顶层配置结构
type Config struct {
	Application *Application          `mapstructure:"application"`
	Logger      *Logger               `mapstructure:"logger"`
	SSL         *SSL                  `mapstructure:"ssl"`
	JWT         *JWT                  `mapstructure:"jwt"`
	Database    *Database             `mapstructure:"database"`
	Databases   *map[string]*Database `mapstructure:"databases"`
	Cache       *Cache                `mapstructure:"cache"`
	Queue       *Queue                `mapstructure:"queue"`
	EventBus    *EventBus             `mapstructure:"eventBus"`
	Locker      *Locker               `mapstructure:"locker"`
}

// 多db改造，如果多db配置不存在，则默认使用单个db配置，并以*为key
func (e *Config) multiDatabase() {
	if len(*e.Databases) == 0 {
		*e.Databases = map[string]*Database{
			"*": e.Database,
		}

	}
}

var AppConfig = &Config{
	Application: ApplicationConfig,
	SSL:         SslConfig,
	Logger:      LoggerConfig,
	JWT:         JwtConfig,
	Database:    DatabaseConfig,
	Databases:   &DatabasesConfig,
	Cache:       CacheConfig,
	Queue:       QueueConfig,
	Locker:      LockerConfig,
	EventBus:    EventBusConfig,
}
