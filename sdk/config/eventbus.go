package config

// Queue 队列配置
type EventBus struct {
	Kafka KafkaConfig `mapstructure:"kafka"`
}

// 新增 KafkaConfig 结构体
type KafkaConfig struct {
	Brokers             []string `mapstructure:"brokers"`
	HealthCheckInterval int      `mapstructure:"healthCheckInterval"` // 健康检测的时间间隔，整数，单位分钟
}

var EventBusConfig = new(EventBus)
