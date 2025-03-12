package config

type EventBus struct {
	Kafka KafkaConfig // 添加 Kafka 配置
}

// 新增 KafkaConfig 结构体
type KafkaConfig struct {
	Brokers             []string
	HealthCheckInterval int // 健康检测的时间间隔，整数，单位分钟
}

var EventBusConfig = new(EventBus)
