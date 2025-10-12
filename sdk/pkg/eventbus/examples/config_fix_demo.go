package main

import (
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
	fmt.Println("=== EventBus 配置转换修复演示 ===\n")

	// 1. 演示用户配置层到程序员配置层的转换
	fmt.Println("1. 用户配置层 -> 程序员配置层转换:")
	
	userConfig := &config.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Producer: config.ProducerConfig{
			RequiredAcks:   1, // 用户设置为1
			Compression:    "snappy",
			FlushFrequency: 100 * time.Millisecond,
			FlushMessages:  50,
			Timeout:        10 * time.Second,
		},
		Consumer: config.ConsumerConfig{
			GroupID:           "demo-group",
			AutoOffsetReset:   "earliest",
			SessionTimeout:    30 * time.Second,
			HeartbeatInterval: 3 * time.Second,
		},
	}

	fmt.Printf("用户配置 - RequiredAcks: %d\n", userConfig.Producer.RequiredAcks)
	fmt.Printf("用户配置 - 只有 %d 个生产者字段\n", 5) // RequiredAcks, Compression, FlushFrequency, FlushMessages, Timeout

	// 转换为程序员配置层
	internalConfig := eventbus.ConvertUserConfigToInternalKafkaConfig(userConfig)
	
	fmt.Printf("程序员配置 - RequiredAcks: %d (自动调整为WaitForAll以支持幂等性)\n", internalConfig.Producer.RequiredAcks)
	fmt.Printf("程序员配置 - Idempotent: %v (程序员控制)\n", internalConfig.Producer.Idempotent)
	fmt.Printf("程序员配置 - MaxInFlight: %d (幂等性要求)\n", internalConfig.Producer.MaxInFlight)
	fmt.Printf("程序员配置 - PartitionerType: %s (程序员控制)\n", internalConfig.Producer.PartitionerType)
	fmt.Printf("程序员配置 - 总共有 16+ 个生产者字段 (用户只需要关心5个)\n")

	fmt.Println("\n2. 完整的EventBus配置转换:")
	
	// 创建完整的用户配置
	fullUserConfig := &config.EventBusConfig{
		Type: "kafka",
		Kafka: config.KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Producer: config.ProducerConfig{
				RequiredAcks:   1,
				Compression:    "snappy",
				FlushFrequency: 100 * time.Millisecond,
				FlushMessages:  50,
				Timeout:        10 * time.Second,
			},
			Consumer: config.ConsumerConfig{
				GroupID:           "demo-group",
				AutoOffsetReset:   "earliest",
				SessionTimeout:    30 * time.Second,
				HeartbeatInterval: 3 * time.Second,
			},
		},
		Security: config.SecurityConfig{
			Enabled:  true,
			Protocol: "SASL_SSL",
			Username: "demo-user",
			Password: "demo-pass",
		},
		HealthCheck: config.HealthCheckConfig{
			Enabled: true,
			Publisher: config.HealthCheckPublisherConfig{
				Topic:    "health-check",
				Interval: 30 * time.Second,
			},
		},
		Monitoring: config.MetricsConfig{
			Enabled:         true,
			CollectInterval: 10 * time.Second,
		},
	}

	// 转换配置
	eventBusConfig := eventbus.ConvertConfig(fullUserConfig)
	
	fmt.Printf("转换后的EventBus类型: %s\n", eventBusConfig.Type)
	fmt.Printf("Kafka Brokers: %v\n", eventBusConfig.Kafka.Brokers)
	fmt.Printf("安全配置 - Enabled: %v\n", eventBusConfig.Kafka.Security.Enabled)
	fmt.Printf("安全配置 - Protocol: %s\n", eventBusConfig.Kafka.Security.Protocol)
	fmt.Printf("安全配置 - Username: %s\n", eventBusConfig.Kafka.Security.Username)
	fmt.Printf("健康检查 - Enabled: %v\n", eventBusConfig.Enterprise.HealthCheck.Enabled)
	fmt.Printf("监控 - Enabled: %v\n", eventBusConfig.Metrics.Enabled)

	fmt.Println("\n3. 配置分层设计的优势:")
	fmt.Println("✅ 用户只需要配置核心业务字段")
	fmt.Println("✅ 程序员控制所有技术细节和优化参数")
	fmt.Println("✅ 自动确保配置兼容性（如幂等性生产者的RequiredAcks）")
	fmt.Println("✅ 提供合理的默认值，减少配置错误")
	fmt.Println("✅ 支持企业级特性的透明集成")

	fmt.Println("\n=== 演示完成 ===")
}
