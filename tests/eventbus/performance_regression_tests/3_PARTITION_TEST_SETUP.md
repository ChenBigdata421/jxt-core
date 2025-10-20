# Kafka 3 分区测试配置说明

## 📋 修改概述

为了验证 Kafka EventBus 在多分区环境下的顺序保证，我们将 3 分区 topics 的创建作为测试预置条件集成到测试用例中。

## 🔧 主要修改

### 1. 添加创建 3 分区 Topics 的函数

**位置**: `kafka_nats_comparison_test.go` 第 133-199 行

```go
// createKafkaTopicsWithPartitions 创建指定分区数的 Kafka Topics
func createKafkaTopicsWithPartitions(t *testing.T, topics []string, partitions int32) map[string]int32 {
	t.Logf("🔧 创建 Kafka Topics (分区数: %d)...", partitions)

	// 创建 Kafka 配置
	config := sarama.NewConfig()
	config.Version = sarama.V2_6_0_0

	// 创建 Kafka 管理客户端
	admin, err := sarama.NewClusterAdmin([]string{"localhost:29094"}, config)
	if err != nil {
		t.Logf("⚠️  无法创建 Kafka 管理客户端: %v", err)
		return nil
	}
	defer admin.Close()

	// 记录实际创建的分区数
	actualPartitions := make(map[string]int32)

	// 为每个 topic 创建指定分区数
	for _, topicName := range topics {
		// 删除已存在的 topic（如果存在）
		err = admin.DeleteTopic(topicName)
		if err != nil && err != sarama.ErrUnknownTopicOrPartition {
			t.Logf("   ⚠️  删除 topic %s 失败: %v", topicName, err)
		}

		// 等待删除完成
		time.Sleep(100 * time.Millisecond)

		// 创建新的 topic
		topicDetail := &sarama.TopicDetail{
			NumPartitions:     partitions,
			ReplicationFactor: 1, // 单节点，复制因子为 1
		}

		err = admin.CreateTopic(topicName, topicDetail, false)
		if err != nil {
			t.Logf("   ❌ 创建失败: %s - %v", topicName, err)
		} else {
			actualPartitions[topicName] = partitions
			t.Logf("   ✅ 创建成功: %s (%d partitions)", topicName, partitions)
		}
	}

	// 等待 topic 创建完成
	time.Sleep(500 * time.Millisecond)

	// 验证创建的 topics
	t.Logf("📊 验证创建的 Topics:")
	allTopics, err := admin.ListTopics()
	if err != nil {
		t.Logf("⚠️  无法列出 topics: %v", err)
		return actualPartitions
	}

	for _, topicName := range topics {
		if detail, exists := allTopics[topicName]; exists {
			actualPartitions[topicName] = detail.NumPartitions
			t.Logf("   %s: %d partitions", topicName, detail.NumPartitions)
		} else {
			t.Logf("   ⚠️  Topic %s 不存在", topicName)
		}
	}

	t.Logf("✅ 成功创建 %d 个 Kafka topics", len(actualPartitions))
	return actualPartitions
}
```

**功能**：
- 创建指定分区数的 Kafka Topics
- 删除已存在的同名 topic（避免冲突）
- 验证创建结果
- 返回实际创建的分区数映射

---

### 2. 在 PerfMetrics 结构中添加分区数字段

**位置**: `kafka_nats_comparison_test.go` 第 117-122 行

```go
// 连接和消费者组统计（新增）
TopicCount         int              // Topic 数量
ConnectionCount    int              // 连接数
ConsumerGroupCount int              // 消费者组个数
TopicList          []string         // Topic 列表
PartitionCount     map[string]int32 // Kafka Topic 分区数 ← 新增
```

**作用**：记录每个 topic 的实际分区数

---

### 3. 在测试开始前创建 3 分区 Topics

**位置**: `kafka_nats_comparison_test.go` 第 461-474 行

```go
// 🔑 要求：创建 5 个 topic，每个 topic 3 个分区
topicCount := 5
topics := make([]string, topicCount)
for i := 0; i < topicCount; i++ {
	topics[i] = fmt.Sprintf("kafka.perf.%s.topic%d", pressureEn, i+1)
}

// 🔧 预置条件：创建 3 分区的 Kafka Topics
partitionMap := createKafkaTopicsWithPartitions(t, topics, 3)
metrics.PartitionCount = partitionMap

eb, err := eventbus.NewKafkaEventBus(kafkaConfig)
require.NoError(t, err, "Failed to create Kafka EventBus")
defer eb.Close()
```

**改进**：
- 在创建 EventBus 之前先创建 3 分区的 topics
- 将分区信息保存到 metrics 中

---

### 4. 在测试过程中输出分区信息

**位置**: `kafka_nats_comparison_test.go` 第 730-737 行

```go
// 输出分区信息
if len(metrics.PartitionCount) > 0 {
	t.Logf("📊 Kafka Topic 分区配置:")
	for topic, partitions := range metrics.PartitionCount {
		t.Logf("   %s: %d partitions", topic, partitions)
	}
}
```

**作用**：在发送完成后立即输出分区配置，便于验证

---

### 5. 在对比报告中输出分区信息

**位置**: `kafka_nats_comparison_test.go` 第 1001-1020 行

```go
// 连接和消费者组统计（新增）
t.Logf("\n🔗 连接和消费者组:")
t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "Topic 数量", kafka.TopicCount, nats.TopicCount)
t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "连接数", kafka.ConnectionCount, nats.ConnectionCount)
t.Logf("   %-20s | Kafka: %8d | NATS: %8d", "消费者组个数", kafka.ConsumerGroupCount, nats.ConsumerGroupCount)
if len(kafka.TopicList) > 0 {
	t.Logf("   Kafka Topics: %v", kafka.TopicList)
}

// 输出 Kafka 分区信息 ← 新增
if len(kafka.PartitionCount) > 0 {
	t.Logf("   Kafka Partitions:")
	for topic, partitions := range kafka.PartitionCount {
		t.Logf("      %s: %d partitions", topic, partitions)
	}
}

if len(nats.TopicList) > 0 {
	t.Logf("   NATS Topics: %v", nats.TopicList)
}
```

**作用**：在每轮测试结果对比中显示分区配置

---

## 📊 测试输出示例

### 创建 Topics 阶段

```
🔧 创建 Kafka Topics (分区数: 3)...
   ✅ 创建成功: kafka.perf.low.topic1 (3 partitions)
   ✅ 创建成功: kafka.perf.low.topic2 (3 partitions)
   ✅ 创建成功: kafka.perf.low.topic3 (3 partitions)
   ✅ 创建成功: kafka.perf.low.topic4 (3 partitions)
   ✅ 创建成功: kafka.perf.low.topic5 (3 partitions)
📊 验证创建的 Topics:
   kafka.perf.low.topic1: 3 partitions
   kafka.perf.low.topic2: 3 partitions
   kafka.perf.low.topic3: 3 partitions
   kafka.perf.low.topic4: 3 partitions
   kafka.perf.low.topic5: 3 partitions
✅ 成功创建 5 个 Kafka topics
```

### 发送完成阶段

```
✅ 发送完成: 500/500 条消息
📊 Kafka Topic 分区配置:
   kafka.perf.low.topic1: 3 partitions
   kafka.perf.low.topic2: 3 partitions
   kafka.perf.low.topic3: 3 partitions
   kafka.perf.low.topic4: 3 partitions
   kafka.perf.low.topic5: 3 partitions
```

### 对比报告阶段

```
🔗 连接和消费者组:
   Topic 数量             | Kafka:        5 | NATS:        5
   连接数                  | Kafka:        1 | NATS:        1
   消费者组个数               | Kafka:        1 | NATS:        1
   Kafka Topics: [kafka.perf.low.topic1 kafka.perf.low.topic2 kafka.perf.low.topic3 kafka.perf.low.topic4 kafka.perf.low.topic5]
   Kafka Partitions:
      kafka.perf.low.topic1: 3 partitions
      kafka.perf.low.topic2: 3 partitions
      kafka.perf.low.topic3: 3 partitions
      kafka.perf.low.topic4: 3 partitions
      kafka.perf.low.topic5: 3 partitions
   NATS Topics: [nats.perf.low.1760287946.topic1 nats.perf.low.1760287946.topic2 ...]
```

---

## 🎯 验证目标

通过这些修改，我们可以验证：

1. ✅ **3 分区 Topics 成功创建**
2. ✅ **分区数在测试过程中正确记录**
3. ✅ **分区信息在报告中清晰展示**
4. ✅ **同一聚合ID的消息在多分区环境下仍然严格按顺序处理**

---

## 🚀 运行测试

### Windows

```batch
cd tests\eventbus\performance_tests
run_test.bat
```

### Linux/Mac

```bash
cd tests/eventbus/performance_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 20m
```

---

## 📝 预期结果

**Kafka 顺序违反统计（3 分区）**：

| 压力级别 | 顺序违反 | 成功率 |
|---------|---------|--------|
| 低压(500) | **0** | **100.00%** ✅ |
| 中压(2000) | **0** | **100.00%** ✅ |
| 高压(5000) | **0** | **100.00%** ✅ |
| 极限(10000) | **0** | **100.00%** ✅ |

**关键技术保证**：
1. ✅ **HashPartitioner**：相同 AggregateID 路由到同一 partition
2. ✅ **Keyed-Worker Pool**：同一 partition 的消息按顺序处理
3. ✅ **串行发送**：同一聚合ID的消息按顺序发送

---

## 📚 相关文件

- `kafka_nats_comparison_test.go` - 主测试文件
- `run_test.bat` - Windows 测试脚本
- `create_3partition_topics.sh` - Linux 创建 topics 脚本（独立工具）
- `create_3partition_topics.bat` - Windows 创建 topics 脚本（独立工具）

---

## ✅ 总结

通过将 3 分区 topics 的创建集成到测试用例中，我们实现了：

1. **自动化测试环境准备** - 无需手动创建 topics
2. **分区信息透明化** - 测试过程中清晰展示分区配置
3. **可重复性** - 每次测试都使用相同的分区配置
4. **可验证性** - 通过日志验证分区数和顺序保证

这为验证 Kafka EventBus 在多分区环境下的顺序保证提供了完整的测试基础设施。

