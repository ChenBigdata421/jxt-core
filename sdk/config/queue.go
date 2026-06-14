package config

import (
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
	RedisConnectOptions `mapstructure:",squash"`
	Producer *queue.ProducerConfig
	Consumer *queue.ConsumerConfig
}

type QueueMemory struct {
	PoolSize uint
}

type QueueNSQ struct {
	NSQOptions // opt: tags — decoded by opt, not mapstructure; defer squash until NSQOptions gains mapstructure tags
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
		rc := e.Redis.RedisConnectOptions
		// producer：db1 → _redisQueueProducer（独立池，幂等）
		producerClient, err := EnsureQueueProducerClient(rc)
		if err != nil {
			return nil, err
		}
		// consumer：db1 → _redisQueue（独立池，避免阻塞 XREADGROUP 饿死非阻塞操作，幂等）
		consumerClient, err := EnsureQueueConsumerClient(rc)
		if err != nil {
			return nil, err
		}
		return queue.NewRedisWithConsumer(producerClient, consumerClient, e.Redis.Producer, e.Redis.Consumer)
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
