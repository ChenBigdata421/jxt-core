# Topic Builder 快速入门

## 🚀 3分钟上手

### 1. 最简单的用法

```go
import "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"

// 一行代码搞定
err := eventbus.NewTopicBuilder("orders").
    ForHighThroughput().
    Build(ctx, bus)
```

**效果：**
- ✅ 10个分区（高并发）
- ✅ 3个副本（高可用）
- ✅ 保留7天
- ✅ 1GB存储

---

## 📊 三种预设配置

### 高吞吐量（>1000 msg/s）

```go
eventbus.NewTopicBuilder("orders").
    ForHighThroughput().
    Build(ctx, bus)
```

### 中等吞吐量（100-1000 msg/s）

```go
eventbus.NewTopicBuilder("notifications").
    ForMediumThroughput().
    Build(ctx, bus)
```

### 低吞吐量（<100 msg/s）

```go
eventbus.NewTopicBuilder("logs").
    ForLowThroughput().
    Build(ctx, bus)
```

---

## 🎨 预设 + 自定义（推荐）

```go
// 从高吞吐量预设开始，然后调整分区数
err := eventbus.NewTopicBuilder("orders").
    ForHighThroughput().    // 10个分区
    WithPartitions(15).     // 覆盖为15个分区
    Build(ctx, bus)
```

---

## 🔧 完全自定义

```go
err := eventbus.NewTopicBuilder("custom-topic").
    WithPartitions(20).
    WithReplication(3).
    WithRetention(30 * 24 * time.Hour).
    WithMaxSize(5 * 1024 * 1024 * 1024).  // 5GB
    Build(ctx, bus)
```

---

## 🌍 环境配置

### 生产环境

```go
eventbus.NewTopicBuilder("prod-orders").
    ForProduction().
    WithPartitions(10).
    Build(ctx, bus)
```

### 开发环境

```go
eventbus.NewTopicBuilder("dev-orders").
    ForDevelopment().
    WithPartitions(3).
    Build(ctx, bus)
```

### 测试环境

```go
eventbus.NewTopicBuilder("test-orders").
    ForTesting().
    WithPartitions(1).
    Build(ctx, bus)
```

---

## ⚡ 快捷函数

```go
// 一行代码
eventbus.BuildHighThroughputTopic(ctx, bus, "orders")
eventbus.BuildMediumThroughputTopic(ctx, bus, "notifications")
eventbus.BuildLowThroughputTopic(ctx, bus, "logs")
```

---

## 🎯 选择指南

### 根据流量选择

```
你的消息速率是多少？

< 100 msg/s     → ForLowThroughput()
100-1000 msg/s  → ForMediumThroughput()
> 1000 msg/s    → ForHighThroughput()
```

### 根据环境选择

```
你在什么环境？

生产环境 → ForProduction()
开发环境 → ForDevelopment()
测试环境 → ForTesting()
```

---

## ⚠️ 重要提示

### ✅ 可以做的

- ✅ 增加分区数（从3个扩容到10个）
- ✅ 使用预设配置
- ✅ 覆盖预设配置

### ❌ 不能做的

- ❌ 减少分区数（Kafka限制）
- ❌ 修改副本因子（创建后不可变）
- ❌ 分区数超过100（性能考虑）

---

## 📝 完整示例

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
        // ... 其他配置
    }
    bus, _ := eventbus.NewKafkaEventBus(config)
    defer bus.Close()
    
    // 2. 使用 Builder 配置主题
    err := eventbus.NewTopicBuilder("orders").
        ForHighThroughput().
        WithPartitions(15).
        Build(ctx, bus)
    
    if err != nil {
        panic(err)
    }
    
    // 3. 发布消息
    bus.Publish(ctx, "orders", []byte("order data"))
    
    // 4. 订阅消息
    bus.Subscribe(ctx, "orders", func(ctx context.Context, msg []byte) error {
        // 处理消息
        return nil
    })
}
```

---

## 🎓 常见场景

### 场景1：订单系统

```go
eventbus.NewTopicBuilder("order-events").
    ForHighThroughput().
    WithRetention(30 * 24 * time.Hour).  // 保留30天用于审计
    Build(ctx, bus)
```

### 场景2：实时通知

```go
eventbus.NewTopicBuilder("notifications").
    ForMediumThroughput().
    WithRetention(24 * time.Hour).  // 只保留1天
    Build(ctx, bus)
```

### 场景3：系统日志

```go
eventbus.NewTopicBuilder("system-logs").
    ForLowThroughput().
    WithRetention(7 * 24 * time.Hour).  // 保留7天
    Build(ctx, bus)
```

---

## 💡 提示

### 不确定选哪个？

→ 先用 `ForMediumThroughput()`

### 流量突然增长？

→ 随时可以增加分区数

### 需要严格顺序？

→ 使用相同的 key 发送消息

---

## 📚 下一步

1. ✅ 阅读完整文档：`TOPIC_BUILDER_IMPLEMENTATION.md`
2. ✅ 运行示例代码：`examples/topic_builder_example.go`
3. ✅ 在项目中应用：根据实际流量选择合适的配置

---

**就是这么简单！🎉**

## 🔗 相关文档

- [完整实现文档](TOPIC_BUILDER_IMPLEMENTATION.md)
- [Kafka 分区性能优化](KAFKA_PARTITIONS_PERFORMANCE_OPTIMIZATION.md)
- [示例代码](examples/topic_builder_example.go)

