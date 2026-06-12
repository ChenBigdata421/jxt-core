package config

import (
	"github.com/redis/go-redis/v9"

	"github.com/ChenBigdata421/jxt-core/storage"
	"github.com/ChenBigdata421/jxt-core/storage/queue"
)

// Queue 队列配置
type Queue struct {
	Redis  *QueueRedis  `mapstructure:"redis"`
	Memory *QueueMemory `mapstructure:"memory"`
	NSQ    *QueueNSQ    `mapstructure:"nsq" json:"nsq"`
}

type QueueRedis struct {
	RedisConnectOptions
	Producer *queue.ProducerConfig
	Consumer *queue.ConsumerConfig
}

type QueueMemory struct {
	PoolSize uint
}

type QueueNSQ struct {
	NSQOptions
	ChannelPrefix string
}

var QueueConfig = new(Queue)

// Empty 空设置
func (e Queue) Empty() bool {
	return e.Memory == nil && e.Redis == nil && e.NSQ == nil
}

// Setup 启用顺序 redis > 其他 > memory
func (e Queue) Setup() (storage.AdapterQueue, error) {
	if e.Redis != nil {
		client := GetRedisClient()
		if client == nil {
			options, err := e.Redis.RedisConnectOptions.GetRedisOptions()
			if err != nil {
				return nil, err
			}
			client = redis.NewClient(options)
			_redis = client
		}
		return queue.NewRedis(client, e.Redis.Producer, e.Redis.Consumer)
	}
	if e.NSQ != nil {
		cfg, err := e.NSQ.GetNSQOptions()
		if err != nil {
			return nil, err
		}
		return queue.NewNSQ(e.NSQ.Addresses, cfg, e.NSQ.ChannelPrefix)
	}
	return queue.NewMemory(e.Memory.PoolSize), nil
}
