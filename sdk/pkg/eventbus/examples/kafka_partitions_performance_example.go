package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// KafkaPartitionsPerformanceExample 演示Kafka多分区性能优化
//
// 本示例展示：
// 1. 如何为不同流量级别的topic配置合适的分区数
// 2. 多分区如何提升并行消费能力和吞吐量
// 3. 分区数对性能的实际影响
//
// 性能提升原理：
// - 单分区：所有消息串行处理，吞吐量受限于单个消费者
// - 多分区：消息并行处理，吞吐量线性扩展（最多到分区数）
//
// 最佳实践：
// - 低流量（<100 msg/s）：1-3个分区
// - 中流量（100-1000 msg/s）：3-10个分区
// - 高流量（>1000 msg/s）：10-30个分区
func main() {
	fmt.Println("=== Kafka 多分区性能优化示例 ===\n")

	// 场景1：单分区 vs 多分区性能对比
	fmt.Println("场景1：单分区 vs 多分区性能对比")
	demonstrateSingleVsMultiPartition()

	// 场景2：不同流量级别的分区配置
	fmt.Println("\n场景2：不同流量级别的分区配置")
	demonstrateThroughputBasedPartitioning()

	// 场景3：动态增加分区数
	fmt.Println("\n场景3：动态增加分区数（扩容）")
	demonstratePartitionScaling()

	// 场景4：分区与消费者组的关系
	fmt.Println("\n场景4：分区与消费者组的关系")
	demonstratePartitionConsumerMapping()
}

// demonstrateSingleVsMultiPartition 演示单分区 vs 多分区性能对比
func demonstrateSingleVsMultiPartition() {
	ctx := context.Background()

	// 创建Kafka EventBus
	bus, err := createKafkaEventBus()
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	// 测试1：单分区topic
	fmt.Println("\n测试1：单分区topic（baseline）")
	singlePartitionTopic := "performance-test-single-partition"
	singlePartitionOptions := eventbus.DefaultTopicOptions()
	singlePartitionOptions.Partitions = 1
	singlePartitionOptions.ReplicationFactor = 1

	if err := bus.ConfigureTopic(ctx, singlePartitionTopic, singlePartitionOptions); err != nil {
		log.Printf("Warning: Failed to configure single partition topic: %v", err)
	}

	singlePartitionThroughput := measureThroughput(ctx, bus, singlePartitionTopic, 1000)
	fmt.Printf("单分区吞吐量: %.2f msg/s\n", singlePartitionThroughput)

	// 测试2：10分区topic
	fmt.Println("\n测试2：10分区topic（优化后）")
	multiPartitionTopic := "performance-test-multi-partition"
	multiPartitionOptions := eventbus.HighThroughputTopicOptions()
	multiPartitionOptions.Partitions = 10
	multiPartitionOptions.ReplicationFactor = 1

	if err := bus.ConfigureTopic(ctx, multiPartitionTopic, multiPartitionOptions); err != nil {
		log.Printf("Warning: Failed to configure multi partition topic: %v", err)
	}

	multiPartitionThroughput := measureThroughput(ctx, bus, multiPartitionTopic, 1000)
	fmt.Printf("10分区吞吐量: %.2f msg/s\n", multiPartitionThroughput)

	// 性能提升比例
	improvement := (multiPartitionThroughput - singlePartitionThroughput) / singlePartitionThroughput * 100
	fmt.Printf("\n性能提升: %.2f%%\n", improvement)
	fmt.Printf("理论最大提升: 10倍（10个分区）\n")
}

// demonstrateThroughputBasedPartitioning 演示基于流量级别的分区配置
func demonstrateThroughputBasedPartitioning() {
	ctx := context.Background()

	bus, err := createKafkaEventBus()
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	// 低流量topic（<100 msg/s）
	fmt.Println("\n配置低流量topic（3分区）")
	lowThroughputTopic := "low-throughput-topic"
	lowOptions := eventbus.LowThroughputTopicOptions()
	if err := bus.ConfigureTopic(ctx, lowThroughputTopic, lowOptions); err != nil {
		log.Printf("Warning: %v", err)
	}
	fmt.Printf("Topic: %s, Partitions: %d, 适用场景: <100 msg/s\n",
		lowThroughputTopic, lowOptions.Partitions)

	// 中流量topic（100-1000 msg/s）
	fmt.Println("\n配置中流量topic（5分区）")
	mediumThroughputTopic := "medium-throughput-topic"
	mediumOptions := eventbus.MediumThroughputTopicOptions()
	if err := bus.ConfigureTopic(ctx, mediumThroughputTopic, mediumOptions); err != nil {
		log.Printf("Warning: %v", err)
	}
	fmt.Printf("Topic: %s, Partitions: %d, 适用场景: 100-1000 msg/s\n",
		mediumThroughputTopic, mediumOptions.Partitions)

	// 高流量topic（>1000 msg/s）
	fmt.Println("\n配置高流量topic（10分区）")
	highThroughputTopic := "high-throughput-topic"
	highOptions := eventbus.HighThroughputTopicOptions()
	if err := bus.ConfigureTopic(ctx, highThroughputTopic, highOptions); err != nil {
		log.Printf("Warning: %v", err)
	}
	fmt.Printf("Topic: %s, Partitions: %d, 适用场景: >1000 msg/s\n",
		highThroughputTopic, highOptions.Partitions)
}

// demonstratePartitionScaling 演示分区扩容
func demonstratePartitionScaling() {
	ctx := context.Background()

	bus, err := createKafkaEventBus()
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	topic := "scalable-topic"

	// 初始配置：3个分区
	fmt.Println("\n步骤1：创建topic，初始3个分区")
	initialOptions := eventbus.DefaultTopicOptions()
	initialOptions.Partitions = 3
	initialOptions.ReplicationFactor = 1

	if err := bus.ConfigureTopic(ctx, topic, initialOptions); err != nil {
		log.Printf("Warning: %v", err)
	}
	fmt.Printf("Topic创建成功: %s, Partitions: %d\n", topic, initialOptions.Partitions)

	// 模拟流量增长，需要扩容
	fmt.Println("\n步骤2：流量增长，扩容到10个分区")
	scaledOptions := eventbus.DefaultTopicOptions()
	scaledOptions.Partitions = 10
	scaledOptions.ReplicationFactor = 1

	// 注意：Kafka只允许增加分区，不允许减少
	if err := bus.ConfigureTopic(ctx, topic, scaledOptions); err != nil {
		log.Printf("扩容结果: %v", err)
	} else {
		fmt.Printf("扩容成功: %s, 新分区数: %d\n", topic, scaledOptions.Partitions)
	}

	fmt.Println("\n重要提示：")
	fmt.Println("- Kafka分区数只能增加，不能减少")
	fmt.Println("- 增加分区后，新消息会分布到所有分区")
	fmt.Println("- 已有消息的分区不会改变")
}

// demonstratePartitionConsumerMapping 演示分区与消费者的映射关系
func demonstratePartitionConsumerMapping() {
	ctx := context.Background()

	bus, err := createKafkaEventBus()
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	topic := "partition-consumer-mapping"

	// 创建6个分区的topic
	options := eventbus.DefaultTopicOptions()
	options.Partitions = 6
	options.ReplicationFactor = 1

	if err := bus.ConfigureTopic(ctx, topic, options); err != nil {
		log.Printf("Warning: %v", err)
	}

	fmt.Printf("\nTopic: %s, Partitions: %d\n", topic, options.Partitions)
	fmt.Println("\n消费者与分区的映射关系：")
	fmt.Println("场景1：1个消费者 -> 处理所有6个分区（串行）")
	fmt.Println("场景2：2个消费者 -> 每个处理3个分区（并行度2）")
	fmt.Println("场景3：3个消费者 -> 每个处理2个分区（并行度3）")
	fmt.Println("场景4：6个消费者 -> 每个处理1个分区（并行度6，最优）")
	fmt.Println("场景5：10个消费者 -> 6个消费者工作，4个空闲（浪费资源）")

	fmt.Println("\n最佳实践：")
	fmt.Println("- 消费者数量 ≤ 分区数（避免资源浪费）")
	fmt.Println("- 消费者数量 = 分区数（最大并行度）")
	fmt.Println("- 分区数应该是消费者数的倍数（均衡负载）")
}

// measureThroughput 测量吞吐量
func measureThroughput(ctx context.Context, bus eventbus.EventBus, topic string, messageCount int) float64 {
	var receivedCount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)

	// 订阅消息
	handler := func(ctx context.Context, message []byte) error {
		receivedCount.Add(1)
		if receivedCount.Load() >= int64(messageCount) {
			wg.Done()
		}
		return nil
	}

	if err := bus.Subscribe(ctx, topic, handler); err != nil {
		log.Printf("Failed to subscribe: %v", err)
		return 0
	}

	// 等待订阅生效
	time.Sleep(2 * time.Second)

	// 发送消息并计时
	start := time.Now()
	for i := 0; i < messageCount; i++ {
		message := fmt.Sprintf("message-%d", i)
		if err := bus.Publish(ctx, topic, []byte(message)); err != nil {
			log.Printf("Failed to publish: %v", err)
		}
	}

	// 等待所有消息被接收（最多等待30秒）
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		duration := time.Since(start)
		throughput := float64(messageCount) / duration.Seconds()
		return throughput
	case <-time.After(30 * time.Second):
		log.Printf("Timeout waiting for messages")
		return 0
	}
}

// createKafkaEventBus 创建Kafka EventBus
func createKafkaEventBus() (eventbus.EventBus, error) {
	config := &eventbus.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Producer: eventbus.ProducerConfig{
			RequiredAcks:   -1,
			Compression:    "lz4",
			FlushFrequency: 10 * time.Millisecond,
			FlushMessages:  100,
			Timeout:        10 * time.Second,
		},
		Consumer: eventbus.ConsumerConfig{
			GroupID:           "partition-performance-test",
			AutoOffsetReset:   "latest",
			SessionTimeout:    10 * time.Second,
			HeartbeatInterval: 3 * time.Second,
		},
	}

	return eventbus.NewKafkaEventBus(config)
}

