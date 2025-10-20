# Kafka Topic Builder 实现文档

## 📋 概述

本文档介绍了基于 **Builder 模式**的 Kafka Topic 配置方案，提供了优雅、类型安全、易于扩展的 API 来配置 Kafka 主题的分区、副本、保留策略等参数。

## 🎯 设计目标

1. **优雅的 API**：链式调用，代码可读性强
2. **类型安全**：编译时检查，避免运行时错误
3. **灵活配置**：支持预设 + 自定义覆盖
4. **内置验证**：自动检测无效配置
5. **易于扩展**：添加新配置项不影响现有代码

## 🏗️ 架构设计

### 核心组件

```
TopicBuilder (topic_builder.go)
    ├── 预设配置方法
    │   ├── ForHighThroughput()    - 高吞吐量（10分区）
    │   ├── ForMediumThroughput()  - 中等吞吐量（5分区）
    │   └── ForLowThroughput()     - 低吞吐量（3分区）
    │
    ├── Kafka 专有配置
    │   ├── WithPartitions()       - 设置分区数
    │   └── WithReplication()      - 设置副本因子
    │
    ├── 通用配置
    │   ├── WithRetention()        - 设置保留时间
    │   ├── WithMaxSize()          - 设置最大大小
    │   ├── WithMaxMessages()      - 设置最大消息数
    │   ├── WithPersistence()      - 设置持久化模式
    │   └── WithDescription()      - 设置描述
    │
    ├── 环境预设
    │   ├── ForProduction()        - 生产环境（3副本）
    │   ├── ForDevelopment()       - 开发环境（1副本）
    │   └── ForTesting()           - 测试环境（非持久化）
    │
    └── 构建方法
        ├── Validate()             - 验证配置
        ├── Build()                - 构建并配置主题
        └── GetOptions()           - 获取当前配置
```

### 与现有架构的集成

```
TopicBuilder
    ↓ Build()
EventBus.ConfigureTopic()
    ↓
Kafka/NATS 实现
    ↓
实际创建 Topic
```

**设计原则：**
- TopicBuilder 是**上层封装**，提供友好的 API
- ConfigureTopic 是**底层实现**，保持不变
- Builder 最终调用 ConfigureTopic，复用现有逻辑

## 📊 使用示例

### 示例1：使用预设配置（最简单）

```go
// 一行代码搞定
err := eventbus.NewTopicBuilder("orders").
    ForHighThroughput().
    Build(ctx, bus)

// 配置效果：
// - 10个分区
// - 3个副本
// - 保留7天
// - 1GB存储
```

### 示例2：预设 + 自定义覆盖（推荐）

```go
err := eventbus.NewTopicBuilder("orders").
    ForHighThroughput().              // 从高吞吐量预设开始
    WithPartitions(15).               // 覆盖分区数（10 → 15）
    WithRetention(14 * 24 * time.Hour). // 覆盖保留时间（7天 → 14天）
    Build(ctx, bus)

// 配置效果：
// - 15个分区（覆盖）
// - 3个副本（保留）
// - 保留14天（覆盖）
// - 1GB存储（保留）
```

### 示例3：完全自定义配置

```go
err := eventbus.NewTopicBuilder("custom-topic").
    WithPartitions(20).
    WithReplication(3).
    WithRetention(30 * 24 * time.Hour).
    WithMaxSize(5 * 1024 * 1024 * 1024).  // 5GB
    WithDescription("自定义高性能主题").
    Build(ctx, bus)
```

### 示例4：环境特定配置

```go
// 生产环境
err := eventbus.NewTopicBuilder("prod-orders").
    ForProduction().
    WithPartitions(10).
    Build(ctx, bus)

// 开发环境
err := eventbus.NewTopicBuilder("dev-orders").
    ForDevelopment().
    WithPartitions(3).
    Build(ctx, bus)

// 测试环境
err := eventbus.NewTopicBuilder("test-orders").
    ForTesting().
    WithPartitions(1).
    Build(ctx, bus)
```

### 示例5：快捷构建函数

```go
// 一行代码构建高吞吐量主题
err := eventbus.BuildHighThroughputTopic(ctx, bus, "orders")

// 一行代码构建中等吞吐量主题
err := eventbus.BuildMediumThroughputTopic(ctx, bus, "notifications")

// 一行代码构建低吞吐量主题
err := eventbus.BuildLowThroughputTopic(ctx, bus, "logs")
```

## 🔍 配置验证

### 内置验证规则

1. **主题名称**：不能为空
2. **分区数**：
   - 必须 > 0
   - 建议 ≤ 100（性能考虑）
3. **副本因子**：
   - 必须 > 0
   - 建议 ≤ 5（性能考虑）
   - 不应超过分区数
4. **保留时间**：
   - 必须 > 0
   - 最小 1 分钟

### 验证示例

```go
// 有效配置
builder := eventbus.NewTopicBuilder("valid").
    WithPartitions(10).
    WithReplication(3)
err := builder.Validate()  // nil

// 无效配置（分区数为0）
builder := eventbus.NewTopicBuilder("invalid").
    WithPartitions(0)
err := builder.Validate()  // error: partitions must be positive

// 无效配置（副本因子超过分区数）
builder := eventbus.NewTopicBuilder("invalid").
    WithPartitions(3).
    WithReplication(5)
err := builder.Validate()  // error: replication factor should not exceed partitions
```

## 📐 配置推荐

### 基于流量的推荐

| 消息速率 (msg/s) | 推荐配置 | 分区数 | 副本数 | 保留时间 |
|------------------|----------|--------|--------|----------|
| < 100            | `ForLowThroughput()`    | 3  | 3 | 1天  |
| 100-1000         | `ForMediumThroughput()` | 5  | 3 | 3天  |
| > 1000           | `ForHighThroughput()`   | 10 | 3 | 7天  |

### 基于环境的推荐

| 环境 | 推荐配置 | 副本数 | 持久化 | 保留时间 |
|------|----------|--------|--------|----------|
| 生产 | `ForProduction()`   | 3 | 是 | 7天  |
| 开发 | `ForDevelopment()`  | 1 | 是 | 1天  |
| 测试 | `ForTesting()`      | 1 | 否 | 1小时 |

### 基于业务场景的推荐

```go
// 订单系统（高吞吐量 + 长保留）
eventbus.NewTopicBuilder("order-events").
    ForHighThroughput().
    WithRetention(30 * 24 * time.Hour).  // 保留30天用于审计
    Build(ctx, bus)

// 实时通知（中等吞吐量 + 短保留）
eventbus.NewTopicBuilder("notifications").
    ForMediumThroughput().
    WithRetention(24 * time.Hour).  // 只保留1天
    Build(ctx, bus)

// 系统日志（低吞吐量 + 中等保留）
eventbus.NewTopicBuilder("system-logs").
    ForLowThroughput().
    WithRetention(7 * 24 * time.Hour).  // 保留7天
    Build(ctx, bus)
```

## ⚠️ 注意事项

### Kafka 限制

1. **分区数只能增加，不能减少**
   - 设置前需要仔细规划
   - 建议预留一定冗余

2. **副本因子不能修改**
   - 一旦设置，需要重建 topic 才能修改
   - 生产环境建议至少 3 个副本

3. **分区数上限**
   - 单个 topic 建议不超过 100 个分区
   - 集群总分区数建议不超过 10000 个

### 性能考虑

1. **分区数与消费者的关系**
   ```
   消费者数量 > 分区数  → 部分消费者空闲（资源浪费）
   消费者数量 = 分区数  → 最优并行度 ✅
   消费者数量 < 分区数  → 单个消费者处理多个分区
   ```

2. **顺序性保证**
   - 单分区：严格顺序 ✅
   - 多分区：同一 key 的消息保证顺序（使用 HashPartitioner）✅
   - 跨分区：无顺序保证 ❌

## 🎓 最佳实践

### 1. 初期规划

```go
// 根据预期流量选择合适的预设
// 预留一定冗余（分区数可以略大于当前需求）
err := eventbus.NewTopicBuilder("orders").
    ForMediumThroughput().  // 当前流量：500 msg/s
    WithPartitions(8).      // 预留冗余（预设5个，增加到8个）
    Build(ctx, bus)
```

### 2. 渐进式扩容

```go
// 初始配置
eventbus.NewTopicBuilder("scalable-topic").
    WithPartitions(3).
    Build(ctx, bus)

// 流量增长后扩容
eventbus.NewTopicBuilder("scalable-topic").
    WithPartitions(10).  // 增加分区数
    Build(ctx, bus)
```

### 3. 环境隔离

```go
// 根据环境变量选择配置
var builder *eventbus.TopicBuilder
switch os.Getenv("ENV") {
case "production":
    builder = eventbus.NewTopicBuilder("orders").ForProduction()
case "development":
    builder = eventbus.NewTopicBuilder("orders").ForDevelopment()
case "testing":
    builder = eventbus.NewTopicBuilder("orders").ForTesting()
}
builder.WithPartitions(10).Build(ctx, bus)
```

## 📚 API 参考

### 预设配置方法

| 方法 | 分区数 | 副本数 | 保留时间 | 适用场景 |
|------|--------|--------|----------|----------|
| `ForHighThroughput()`   | 10 | 3 | 7天  | >1000 msg/s |
| `ForMediumThroughput()` | 5  | 3 | 3天  | 100-1000 msg/s |
| `ForLowThroughput()`    | 3  | 3 | 1天  | <100 msg/s |
| `ForProduction()`       | -  | 3 | 7天  | 生产环境 |
| `ForDevelopment()`      | -  | 1 | 1天  | 开发环境 |
| `ForTesting()`          | -  | 1 | 1小时 | 测试环境 |

### 配置方法

| 方法 | 参数 | 说明 |
|------|------|------|
| `WithPartitions(n int)` | 分区数 | Kafka 专有，建议 1-100 |
| `WithReplication(n int)` | 副本因子 | Kafka 专有，建议 1-5 |
| `WithRetention(d time.Duration)` | 保留时间 | 最小 1 分钟 |
| `WithMaxSize(bytes int64)` | 最大大小 | 字节数 |
| `WithMaxMessages(count int64)` | 最大消息数 | 消息数量 |
| `WithPersistence(mode TopicPersistenceMode)` | 持久化模式 | persistent/ephemeral/auto |
| `Persistent()` | - | 快捷方法，设置为持久化 |
| `Ephemeral()` | - | 快捷方法，设置为非持久化 |
| `WithDescription(desc string)` | 描述 | 主题描述 |

### 构建方法

| 方法 | 返回值 | 说明 |
|------|--------|------|
| `Validate()` | `error` | 验证配置是否有效 |
| `Build(ctx, bus)` | `error` | 构建并配置主题 |
| `GetOptions()` | `TopicOptions` | 获取当前配置 |
| `GetTopic()` | `string` | 获取主题名称 |

## 🚀 总结

### Builder 模式的优势

✅ **链式调用**：代码可读性强，一目了然  
✅ **预设配置**：覆盖常见场景，开箱即用  
✅ **灵活覆盖**：预设 + 自定义，兼顾简单和灵活  
✅ **类型安全**：编译时检查，避免运行时错误  
✅ **内置验证**：自动检测无效配置，提前发现问题  
✅ **易于扩展**：添加新配置项不影响现有代码  

### 推荐使用方式

1. **简单场景**：使用预设配置（`ForHighThroughput()`）
2. **常见场景**：预设 + 局部覆盖（`ForHighThroughput().WithPartitions(15)`）
3. **复杂场景**：完全自定义配置
4. **快速原型**：使用快捷构建函数（`BuildHighThroughputTopic()`）

### 与之前方案的对比

| 特性 | 方案3（TopicOptions） | 方案2（Builder）✅ |
|------|----------------------|-------------------|
| 可读性 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 易用性 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 灵活性 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 扩展性 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 类型安全 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

**结论：Builder 模式是最优雅、最实用的解决方案！** 🎉

