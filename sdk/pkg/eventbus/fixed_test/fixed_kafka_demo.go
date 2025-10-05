package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	fmt.Println("🚀 Kafka EventBus 修复后完整测试")
	fmt.Println("=================================")

	// 初始化logger
	zapLogger, _ := zap.NewDevelopment()
	defer zapLogger.Sync()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	// 使用正确的外部端口和修复的配置
	fmt.Println("📡 连接到 Kafka 服务器...")
	config := &eventbus.EventBusConfig{
		Type: "kafka",
		Kafka: eventbus.KafkaConfig{
			Brokers: []string{"localhost:29092"}, // 使用外部端口
			Producer: eventbus.ProducerConfig{
				RequiredAcks:   1,
				Timeout:        5 * time.Second,
				RetryMax:       2,
				Compression:    "none",
				FlushFrequency: 100 * time.Millisecond,
				BatchSize:      1024,
			},
			Consumer: eventbus.ConsumerConfig{
				GroupID:           "kafka-fixed-test",
				SessionTimeout:    15 * time.Second,
				HeartbeatInterval: 3 * time.Second,
				MaxProcessingTime: 2 * time.Minute,
				AutoOffsetReset:   "earliest", // 修复：从最早消息开始读取
				FetchMinBytes:     1,
				FetchMaxBytes:     1024 * 1024,
				FetchMaxWait:      500 * time.Millisecond,
			},
		},
	}

	bus, err := eventbus.NewEventBus(config)
	if err != nil {
		fmt.Printf("❌ 连接 Kafka 失败: %v\n", err)
		return
	}
	defer bus.Close()

	fmt.Println("✅ 成功连接到 Kafka 服务器")

	ctx := context.Background()

	// 测试1: 主题持久化配置
	fmt.Println("\n📋 测试1: 主题持久化配置")
	testKafkaTopicConfiguration(ctx, bus)

	// 测试2: 修复后的发布订阅测试
	fmt.Println("\n🔄 测试2: 修复后的发布订阅测试")
	testKafkaPublishSubscribe(ctx, bus)

	// 测试3: 动态配置管理
	fmt.Println("\n⚙️ 测试3: 动态配置管理")
	testKafkaDynamicConfiguration(ctx, bus)

	// 测试4: 性能测试
	fmt.Println("\n⚡ 测试4: 性能测试")
	testKafkaPerformance(ctx, bus)

	fmt.Println("\n✅ Kafka 修复后完整测试完成！")
}

func testKafkaTopicConfiguration(ctx context.Context, bus eventbus.EventBus) {
	// 配置不同持久化策略的主题
	topics := []struct {
		name    string
		options eventbus.TopicOptions
	}{
		{
			name: "kafka.fixed.orders",
			options: eventbus.TopicOptions{
				PersistenceMode: eventbus.TopicPersistent,
				RetentionTime:   2 * time.Hour,
				MaxSize:         20 * 1024 * 1024,
				Description:     "修复测试-持久化订单",
			},
		},
		{
			name: "kafka.fixed.events",
			options: eventbus.TopicOptions{
				PersistenceMode: eventbus.TopicEphemeral,
				RetentionTime:   30 * time.Minute,
				Description:     "修复测试-临时事件",
			},
		},
		{
			name: "kafka.fixed.metrics",
			options: eventbus.TopicOptions{
				PersistenceMode: eventbus.TopicAuto,
				Description:     "修复测试-自动选择",
			},
		},
	}

	successCount := 0
	for _, topic := range topics {
		err := bus.ConfigureTopic(ctx, topic.name, topic.options)
		if err != nil {
			fmt.Printf("❌ 配置主题 %s 失败: %v\n", topic.name, err)
		} else {
			fmt.Printf("✅ 成功配置主题: %s (%s)\n", topic.name, topic.options.PersistenceMode)
			successCount++
		}
	}

	// 验证配置
	configuredTopics := bus.ListConfiguredTopics()
	fmt.Printf("📋 配置结果: %d/%d 成功, 总计 %d 个主题\n", 
		successCount, len(topics), len(configuredTopics))

	for _, topic := range configuredTopics {
		if len(topic) > 12 && topic[:12] == "kafka.fixed." {
			config, _ := bus.GetTopicConfig(topic)
			fmt.Printf("   - %s: %s (保留: %v)\n", 
				topic, config.PersistenceMode, config.RetentionTime)
		}
	}
}

func testKafkaPublishSubscribe(ctx context.Context, bus eventbus.EventBus) {
	// 设置消息接收计数器
	var mu sync.Mutex
	messageCount := make(map[string]int)
	receivedMessages := make(map[string][]string)

	// 订阅测试主题
	topics := []string{
		"kafka.fixed.orders",
		"kafka.fixed.events", 
		"kafka.fixed.metrics",
	}

	fmt.Println("📨 设置消息订阅...")
	for _, topic := range topics {
		topicName := topic // 避免闭包问题
		err := bus.Subscribe(ctx, topicName, func(ctx context.Context, message []byte) error {
			mu.Lock()
			messageCount[topicName]++
			count := messageCount[topicName]
			receivedMessages[topicName] = append(receivedMessages[topicName], string(message))
			mu.Unlock()

			fmt.Printf("📨 [%s] 收到消息 #%d: %s\n", topicName, count, string(message))
			return nil
		})
		if err != nil {
			fmt.Printf("❌ 订阅主题 %s 失败: %v\n", topicName, err)
		} else {
			fmt.Printf("✅ 成功订阅主题: %s\n", topicName)
		}
	}

	// 等待订阅建立
	fmt.Println("⏳ 等待 Kafka 消费者组建立...")
	time.Sleep(5 * time.Second)

	// 发布测试消息
	fmt.Println("📤 开始发布测试消息...")
	messages := []struct {
		topic   string
		content string
	}{
		{
			"kafka.fixed.orders",
			`{"order_id": "kafka-fixed-001", "amount": 199.99, "customer": "test-user-1", "timestamp": "2024-01-01T12:00:00Z"}`,
		},
		{
			"kafka.fixed.events",
			`{"event_type": "user_login", "user_id": "user-123", "ip": "192.168.1.100", "timestamp": "2024-01-01T12:01:00Z"}`,
		},
		{
			"kafka.fixed.metrics",
			`{"cpu_usage": 65.4, "memory_usage": 78.2, "disk_usage": 45.1, "timestamp": "2024-01-01T12:02:00Z"}`,
		},
		{
			"kafka.fixed.orders",
			`{"order_id": "kafka-fixed-002", "amount": 299.99, "customer": "test-user-2", "timestamp": "2024-01-01T12:03:00Z"}`,
		},
	}

	publishedCount := 0
	for i, msg := range messages {
		err := bus.Publish(ctx, msg.topic, []byte(msg.content))
		if err != nil {
			fmt.Printf("❌ 发布消息 #%d 到 %s 失败: %v\n", i+1, msg.topic, err)
		} else {
			fmt.Printf("✅ 成功发布消息 #%d 到 %s\n", i+1, msg.topic)
			publishedCount++
		}
		time.Sleep(1 * time.Second) // 给消息处理时间
	}

	// 等待消息处理
	fmt.Println("⏳ 等待消息处理...")
	time.Sleep(8 * time.Second)

	// 统计结果
	fmt.Printf("📊 发布订阅测试结果:\n")
	fmt.Printf("   - 发布成功: %d/%d 消息\n", publishedCount, len(messages))

	mu.Lock()
	totalReceived := 0
	for topic, count := range messageCount {
		fmt.Printf("   - %s: 收到 %d 条消息\n", topic, count)
		totalReceived += count
	}
	mu.Unlock()

	fmt.Printf("   - 总计收到: %d 条消息\n", totalReceived)

	if totalReceived >= publishedCount {
		fmt.Println("✅ 发布订阅测试成功！")
	} else {
		fmt.Printf("⚠️ 发布订阅测试部分成功 (收到 %d/%d)\n", totalReceived, publishedCount)
	}
}

func testKafkaDynamicConfiguration(ctx context.Context, bus eventbus.EventBus) {
	// 动态配置测试
	dynamicTopic := "kafka.fixed.dynamic.test"

	// 1. 创建动态主题
	err := bus.SetTopicPersistence(ctx, dynamicTopic, true)
	if err != nil {
		fmt.Printf("❌ 动态创建主题失败: %v\n", err)
	} else {
		fmt.Printf("✅ 动态创建主题: %s (持久化)\n", dynamicTopic)
	}

	// 2. 验证配置
	config, err := bus.GetTopicConfig(dynamicTopic)
	if err != nil {
		fmt.Printf("❌ 获取动态主题配置失败: %v\n", err)
	} else {
		fmt.Printf("✅ 动态主题配置: %s\n", config.PersistenceMode)
	}

	// 3. 修改配置
	err = bus.SetTopicPersistence(ctx, dynamicTopic, false)
	if err != nil {
		fmt.Printf("❌ 修改动态主题配置失败: %v\n", err)
	} else {
		fmt.Printf("✅ 成功修改主题 %s 为非持久化\n", dynamicTopic)
	}

	// 4. 验证修改
	updatedConfig, err := bus.GetTopicConfig(dynamicTopic)
	if err != nil {
		fmt.Printf("❌ 验证配置修改失败: %v\n", err)
	} else {
		if updatedConfig.PersistenceMode == eventbus.TopicEphemeral {
			fmt.Println("✅ 配置修改验证成功 (已改为非持久化)")
		} else {
			fmt.Printf("⚠️ 配置修改可能有问题 (当前: %s)\n", updatedConfig.PersistenceMode)
		}
	}

	// 5. 清理
	err = bus.RemoveTopicConfig(dynamicTopic)
	if err != nil {
		fmt.Printf("❌ 清理动态主题失败: %v\n", err)
	} else {
		fmt.Println("✅ 成功清理动态主题配置")
	}

	fmt.Println("🔄 动态配置管理测试完成")
}

func testKafkaPerformance(ctx context.Context, bus eventbus.EventBus) {
	// 性能测试主题
	perfTopic := "kafka.fixed.performance.test"
	
	err := bus.SetTopicPersistence(ctx, perfTopic, true)
	if err != nil {
		fmt.Printf("❌ 性能测试主题配置失败: %v\n", err)
		return
	}

	// 批量发布测试
	batchSize := 20
	fmt.Printf("⚡ 开始性能测试 (%d 条消息)...\n", batchSize)

	start := time.Now()
	successCount := 0
	var publishTimes []time.Duration

	for i := 0; i < batchSize; i++ {
		message := fmt.Sprintf(`{"batch_id": %d, "message": "Kafka性能测试消息 #%d", "data": "test_data_%d", "timestamp": "%s"}`,
			i, i+1, i, time.Now().Format(time.RFC3339))

		msgStart := time.Now()
		err = bus.Publish(ctx, perfTopic, []byte(message))
		msgDuration := time.Since(msgStart)

		if err != nil {
			fmt.Printf("❌ 批量发布失败 #%d: %v\n", i+1, err)
		} else {
			successCount++
			publishTimes = append(publishTimes, msgDuration)
		}
	}

	totalDuration := time.Since(start)
	throughput := float64(successCount) / totalDuration.Seconds()

	// 计算平均延迟
	var totalLatency time.Duration
	for _, latency := range publishTimes {
		totalLatency += latency
	}
	avgLatency := totalLatency / time.Duration(len(publishTimes))

	fmt.Printf("⚡ 性能测试结果:\n")
	fmt.Printf("   - 成功发布: %d/%d 消息\n", successCount, batchSize)
	fmt.Printf("   - 总耗时: %v\n", totalDuration)
	fmt.Printf("   - 吞吐量: %.2f 消息/秒\n", throughput)
	fmt.Printf("   - 平均延迟: %v\n", avgLatency)
	fmt.Printf("   - 成功率: %.1f%%\n", float64(successCount)/float64(batchSize)*100)

	// 性能评估
	if throughput > 10 {
		fmt.Println("✅ 性能测试优秀！")
	} else if throughput > 5 {
		fmt.Println("✅ 性能测试良好")
	} else {
		fmt.Println("⚠️ 性能测试一般")
	}
}
