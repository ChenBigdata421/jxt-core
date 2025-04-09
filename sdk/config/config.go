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
	Tenants     *Tenants              `mapstructure:"tenants"`
}

var AppConfig = &Config{
	Application: ApplicationConfig,
	Logger:      LoggerConfig,
	HTTP:        HttpConfig,
	RPC:         RpcConfig,
	JWT:         JwtConfig,
	Database:    DatabaseConfig,
	Cache:       CacheConfig,
	Queue:       QueueConfig,
	Locker:      LockerConfig,
	EventBus:    EventBusConfig,
	Tenants:     TenantsConfig,
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

	return nil
}
