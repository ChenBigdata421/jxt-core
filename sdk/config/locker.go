package config

import (
	"github.com/ChenBigdata421/jxt-core/storage"
	"github.com/ChenBigdata421/jxt-core/storage/locker"
)

// Locker 锁配置
type Locker struct {
	Redis *RedisConnectOptions `mapstructure:"redis"`
}

var LockerConfig = new(Locker)

// Empty 空设置
func (e Locker) Empty() bool {
	return e.Redis == nil
}

// Setup 启用顺序 redis > 其他 > memory
func (e Locker) Setup() (storage.AdapterLocker, error) {
	if e.Redis != nil {
		options, err := e.Redis.GetRedisOptions()
		if err != nil {
			return nil, err
		}
		client, err := EnsureRedisClient(options)
		if err != nil {
			return nil, err
		}
		return locker.NewRedis(client), nil
	}
	return nil, nil
}
