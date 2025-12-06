package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config 顶层配置结构
type Config struct {
	Application    *Application          `mapstructure:"application"`
	HTTP           *HTTPConfig           `mapstructure:"http" json:"http"`
	GRPC           *GRPCConfig           `mapstructure:"grpc" json:"grpc"`
	Etcd           *ETCDConfig           `mapstructure:"etcd" json:"etcd"`
	Logger         *Logger               `mapstructure:"logger"`
	SSL            *SSL                  `mapstructure:"ssl"`
	JWT            *JWTConfig            `mapstructure:"jwt"`
	Database       *Database             `mapstructure:"database"`
	Cache          *Cache                `mapstructure:"cache"`
	Queue          *Queue                `mapstructure:"queue"`
	EventBus       *EventBusConfig       `mapstructure:"eventBus"`
	Locker         *Locker               `mapstructure:"locker"`
	Tenants        *Tenants              `mapstructure:"tenants"`
	Storage        *StorageConfig        `mapstructure:"storage"`         // 共享存储配置（HTTP/FTP）
	DuplicateCheck *DuplicateCheckConfig `mapstructure:"duplicate_check"` // 共享去重配置
	FTP            *FTPConfig            `mapstructure:"ftp"`             // FTP 服务器配置
}

var AppConfig = &Config{
	Application:    ApplicationConfig,
	Logger:         LoggerConfig,
	HTTP:           HttpConfig,
	GRPC:           GrpcConfig,
	Etcd:           EtcdConfig,
	JWT:            JwtConfig,
	Database:       DatabaseConfig,
	Cache:          CacheConfig,
	Queue:          QueueConfig,
	Locker:         LockerConfig,
	EventBus:       nil, // 需要在运行时配置
	Tenants:        TenantsConfig,
	Storage:        StorageConfigInstance,
	DuplicateCheck: DuplicateCheckConfigInstance,
	FTP:            FTPConfigInstance,
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
