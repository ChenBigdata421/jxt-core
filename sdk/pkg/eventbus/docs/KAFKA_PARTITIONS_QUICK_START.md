# Kafka 多分区配置快速入门

## 🚀 5分钟快速上手

### 1. 基础使用（推荐）

使用预设配置，一行代码搞定：

```go
import "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"

// 高流量场景（>1000 msg/s）
options := eventbus.HighThroughputTopicOptions()
bus.ConfigureTopic(ctx, "my-topic", options)
// 自动配置：10个分区 + 3个副本
```

### 2. 三种预设配置

```go
// 低流量（<100 msg/s）
eventbus.LowThroughputTopicOptions()    // 3个分区

// 中流量（100-1000 msg/s）
eventbus.MediumThroughputTopicOptions() // 5个分区

// 高流量（>1000 msg/s）
eventbus.HighThroughputTopicOptions()   // 10个分区
```

### 3. 自定义配置

```go
options := eventbus.DefaultTopicOptions()
options.Partitions = 15        // 自定义分区数
options.ReplicationFactor = 3  // 自定义副本数
bus.ConfigureTopic(ctx, "custom-topic", options)
```

## 📊 性能对比

| 配置 | 分区数 | 吞吐量提升 |
|------|--------|------------|
| 默认 | 1      | 基准       |
| 低流量 | 3    | 3倍        |
| 中流量 | 5    | 5倍        |
| 高流量 | 10   | 10倍       |

## ⚡ 完整示例

```go
package main

import (
    "context"
    "time"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    ctx := context.Background()
    
    // 1. 创建 Kafka EventBus
    config := &eventbus.KafkaConfig{
        Brokers: []string{"localhost:9092"},
        Producer: eventbus.ProducerConfig{
            RequiredAcks:   -1,
            Compression:    "lz4",
            FlushFrequency: 10 * time.Millisecond,
        },
        Consumer: eventbus.ConsumerConfig{
            GroupID:         "my-group",
            AutoOffsetReset: "latest",
        },
    }
    
    bus, _ := eventbus.NewKafkaEventBus(config)
    defer bus.Close()
    
    // 2. 配置高性能 topic（10个分区）
    options := eventbus.HighThroughputTopicOptions()
    bus.ConfigureTopic(ctx, "high-perf-topic", options)
    
    // 3. 发布消息（自动分布到10个分区）
    for i := 0; i < 1000; i++ {
        bus.Publish(ctx, "high-perf-topic", []byte("message"))
    }
    
    // 4. 订阅消息（可以启动10个消费者并行处理）
    bus.Subscribe(ctx, "high-perf-topic", func(ctx context.Context, msg []byte) error {
        // 处理消息
        return nil
    })
}
```

## 🎯 选择指南

### 根据流量选择

```
你的消息速率是多少？

< 100 msg/s     → LowThroughputTopicOptions()
100-1000 msg/s  → MediumThroughputTopicOptions()
> 1000 msg/s    → HighThroughputTopicOptions()
> 10000 msg/s   → 自定义（20-30个分区）
```

### 根据消费者数量选择

```
你有多少个消费者？

1-3 个  → 3个分区
3-5 个  → 5个分区
5-10 个 → 10个分区
10+ 个  → 自定义
```

## ⚠️ 重要提示

### ✅ 可以做的

- ✅ 增加分区数（从3个扩容到10个）
- ✅ 使用预设配置
- ✅ 自定义分区数

### ❌ 不能做的

- ❌ 减少分区数（Kafka限制）
- ❌ 修改副本因子（创建后不可变）
- ❌ 分区数超过100（性能考虑）

## 🔧 常见场景

### 场景1：新建高性能 topic

```go
options := eventbus.HighThroughputTopicOptions()
bus.ConfigureTopic(ctx, "new-topic", options)
```

### 场景2：扩容现有 topic

```go
// 从3个分区扩容到10个
options := eventbus.DefaultTopicOptions()
options.Partitions = 10
bus.ConfigureTopic(ctx, "existing-topic", options)
```

### 场景3：测试环境（单副本）

```go
options := eventbus.HighThroughputTopicOptions()
options.ReplicationFactor = 1  // 测试环境只需1个副本
bus.ConfigureTopic(ctx, "test-topic", options)
```

## 📈 性能优化建议

1. **分区数 = 消费者数**（最优并行度）
2. **生产环境至少3个副本**（高可用）
3. **监控消费延迟**（及时扩容）
4. **避免过度分区**（单topic不超过100个分区）

## 🎓 下一步

1. ✅ 阅读完整文档：`KAFKA_PARTITIONS_PERFORMANCE_OPTIMIZATION.md`
2. ✅ 运行示例代码：`examples/kafka_partitions_performance_example.go`
3. ✅ 在项目中应用：根据实际流量选择合适的配置

## 💡 提示

- 不确定选哪个？→ 先用 `MediumThroughputTopicOptions()`
- 流量突然增长？→ 随时可以增加分区数
- 需要严格顺序？→ 使用相同的 key 发送消息

---

**就是这么简单！🎉**

