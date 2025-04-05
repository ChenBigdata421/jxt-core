package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config 顶层配置结构
type Config struct {
	Application *Application          `mapstructure:"application"`
	HTTP        *HTTPConfig           `mapstructure:"http" json:"http"`
	RPC         *RPCConfig            `mapstructure:"rpc" json:"rpc"`
	Logger      *Logger               `mapstructure:"logger"`
	SSL         *SSL                  `mapstructure:"ssl"`
	JWT         *JWTConfig            `mapstructure:"jwt"`
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
	HTTP:        HttpConfig,
	RPC:         RpcConfig,
	Logger:      LoggerConfig,
	JWT:         JwtConfig,
	Database:    DatabaseConfig,
	Databases:   &DatabasesConfig,
	Cache:       CacheConfig,
	Queue:       QueueConfig,
	Locker:      LockerConfig,
	EventBus:    EventBusConfig,
}

func Setup(configYml string) error {

	v := viper.New()
	v.SetConfigFile(configYml)
	if err := v.ReadInConfig(); err != nil {
		panic(fmt.Sprintf("读取配置文件失败: %v", err))
	}

	// 映射到AppConfig
	if err := v.Unmarshal(AppConfig); err != nil {
		panic(fmt.Sprintf("解析配置文件失败: %v", err))
	}

	AppConfig.multiDatabase()
	return nil
}
