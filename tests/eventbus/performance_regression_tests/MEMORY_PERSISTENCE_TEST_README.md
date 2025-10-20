# Kafka vs NATS JetStream 内存持久化性能对比测试

## 📋 **测试概述**

本测试文件 `memory_persistence_comparison_test.go` 提供了 Kafka 和 NATS JetStream 在内存持久化模式下的完整性能对比测试。

---

## 🎯 **测试要求**

### 1. **EventBus 实例创建**
✅ 创建 EventBus 实例，覆盖 Kafka 和 NATS JetStream 两种实现

### 2. **持久化模式**
✅ Kafka 和 NATS JetStream 都使用**内存持久化**
- Kafka: 默认内存持久化
- NATS: `Storage: "memory"`

### 3. **发布订阅方法**
✅ 发布端必须采用 `Publish` 方法
✅ 订阅端必须采用 `Subscribe` 方法
❌ **不使用** `PublishEnvelope` 和 `SubscribeEnvelope`
❌ **无聚合ID**

### 4. **测试场景**
✅ 测试覆盖 4 个压力级别：
- **低压**: 500 消息（无聚合ID）
- **中压**: 2000 消息（无聚合ID）
- **高压**: 5000 消息（无聚合ID）
- **极限**: 10000 消息（无聚合ID）

### 5. **Topic 配置**
✅ Topic 数量: **5 个**
✅ Kafka 分区数: **3 个分区/topic**

### 6. **性能报告**
✅ 输出报告包括：
- Kafka 和 NATS JetStream 的性能指标对比
- 关键资源占用情况（协程、内存）
- 连接数统计
- 消费者组个数统计
- Kafka 分区数详情

### 7. **命名规范**
✅ Kafka 的 ClientID 和 topic 名称只使用 **ASCII 字符**
- Kafka ClientID: `memory-kafka-{pressure}`
- Kafka Topics: `test.memory.kafka.topic1`, `test.memory.kafka.topic2`, ...
- NATS ClientID: `memory-nats-{pressure}`
- NATS Topics: `test.memory.nats.topic1`, `test.memory.nats.topic2`, ...

### 8. **数据清理**
✅ 每次测试前清理 Kafka 和 NATS 数据
- Kafka: 删除所有 `test.memory.kafka` 前缀的 topics
- NATS: 删除所有 `TEST_MEMORY_NATS` 前缀的 streams

### 9. **Worker 池检查**
✅ 检查是否使用了全局 Worker 池处理订阅消息
- Kafka: 使用全局 Worker 池
- NATS: 使用全局 Worker 池
- Worker 池大小: `runtime.NumCPU() * 2`

### 10. **Kafka 分区配置**
✅ Kafka 采用 **3 分区**
✅ 在报告中体现分区数

---

## 📊 **测试指标**

### 消息指标
- 发送消息数
- 接收消息数
- 发送错误数
- 处理错误数
- 成功率

### 性能指标
- 发送吞吐量 (msg/s)
- 接收吞吐量 (msg/s)
- 平均发送延迟 (ms)
- 平均处理延迟 (ms)
- 测试时长 (s)

### 资源占用
- 初始协程数
- 峰值协程数
- 最终协程数
- 协程泄漏数
- 初始内存 (MB)
- 峰值内存 (MB)
- 最终内存 (MB)
- 内存增量 (MB)

### 连接统计
- 连接数
- 消费者组个数
- Topic 列表
- Kafka 分区数（每个 topic）

### Worker 池统计
- 是否使用全局 Worker 池
- Worker 池大小

---

## 🚀 **运行测试**

### 前置条件

1. **启动 Kafka**（端口 29094）:
```bash
docker-compose up -d kafka
```

2. **启动 NATS JetStream**（端口 4223）:
```bash
docker-compose up -d nats
```

### 运行完整测试

```bash
go test -v ./tests/eventbus/performance_tests/ -run TestMemoryPersistenceComparison -timeout 30m
```

### 运行单个场景测试

测试文件不支持单独运行场景，但会按顺序运行所有 4 个场景：
1. Low (500 消息)
2. Medium (2000 消息)
3. High (5000 消息)
4. Extreme (10000 消息)

---

## 📈 **测试报告示例**

### 单场景报告

```
================================================================================
📊 Kafka vs NATS JetStream 内存持久化性能对比报告 - Low 压力
================================================================================

📋 基本信息:
  压力级别: Low
  消息总数: 500
  Topic 数量: 5
  Kafka 分区数: 3 (每个 topic)

📊 Kafka 分区详情:
  test.memory.kafka.topic1: 3 partitions
  test.memory.kafka.topic2: 3 partitions
  test.memory.kafka.topic3: 3 partitions
  test.memory.kafka.topic4: 3 partitions
  test.memory.kafka.topic5: 3 partitions

📨 消息指标对比:
  指标                 | Kafka           | NATS            | 差异
  ----------------------------------------------------------------------
  发送消息数           | 500             | 500             | +0.0%
  接收消息数           | 500             | 500             | +0.0%
  发送错误数           | 0               | 0               | -
  成功率               | 100.00%         | 100.00%         | +0.0%

⚡ 性能指标对比:
  指标                 | Kafka           | NATS            | 差异
  ----------------------------------------------------------------------
  发送吞吐量 (msg/s)   | 333.33          | 333.33          | +0.0%
  接收吞吐量 (msg/s)   | 333.33          | 333.33          | +0.0%
  平均发送延迟 (ms)    | 0.123           | 0.156           | +26.8%
  平均处理延迟 (ms)    | 1.234           | 1.567           | +27.0%
  测试时长 (s)         | 1.50            | 1.50            | -

💾 资源占用对比:
  指标                 | Kafka           | NATS            | 差异
  ----------------------------------------------------------------------
  初始协程数           | 45              | 42              | -3
  峰值协程数           | 67              | 65              | -2
  最终协程数           | 46              | 43              | -3
  协程泄漏数           | 1               | 1               | -
  初始内存 (MB)        | 12.34           | 11.23           | -1.11
  峰值内存 (MB)        | 15.67           | 14.56           | -1.11
  最终内存 (MB)        | 13.45           | 12.34           | -1.11
  内存增量 (MB)        | 1.11            | 1.11            | -

🔗 连接和消费者组统计:
  指标                 | Kafka           | NATS
  -------------------------------------------------------
  连接数               | 1               | 1
  消费者组个数         | 1               | 1

⚙️  Worker 池统计:
  指标                 | Kafka           | NATS
  -------------------------------------------------------
  使用全局 Worker 池   | true            | true
  Worker 池大小        | 16              | 16

🎯 性能总结:
  ✅ Kafka 吞吐量领先 NATS 0.0%
  ✅ Kafka 延迟优于 NATS 27.0%
  ✅ Kafka 内存占用少于 NATS 0.00 MB
================================================================================
```

### 汇总报告

```
================================================================================
📊 所有场景汇总报告
================================================================================

场景       | 系统            | 吞吐量(msg/s)   | 延迟(ms)        | 成功率(%)       | 内存增量(MB)
----------------------------------------------------------------------------------------------------
Low        | Kafka           | 333.33          | 1.234           | 100.00          | 1.11
           | NATS            | 333.33          | 1.567           | 100.00          | 1.11
----------------------------------------------------------------------------------------------------
Medium     | Kafka           | 1333.33         | 1.345           | 100.00          | 2.34
           | NATS            | 1333.33         | 1.678           | 100.00          | 2.34
----------------------------------------------------------------------------------------------------
High       | Kafka           | 1666.67         | 1.456           | 100.00          | 3.45
           | NATS            | 1666.67         | 1.789           | 100.00          | 3.45
----------------------------------------------------------------------------------------------------
Extreme    | Kafka           | 3333.33         | 1.567           | 100.00          | 4.56
           | NATS            | 333.33          | 15.678          | 100.00          | 4.56
----------------------------------------------------------------------------------------------------

🔍 关键发现:
  1. ✅ 所有测试使用 Publish/Subscribe 方法（无聚合ID）
  2. ✅ Kafka 和 NATS 都使用内存持久化
  3. ✅ Kafka 使用 3 分区配置
  4. ✅ 两个系统都使用全局 Worker 池处理订阅消息
  5. ✅ Topic 数量: 5
  6. ✅ 测试前已清理 Kafka 和 NATS 数据

✅ 所有测试完成！
================================================================================
```

---

## 🔧 **配置详情**

### Kafka 配置

```go
cfg := &eventbus.KafkaConfig{
    Brokers:  []string{"localhost:29094"},
    ClientID: "memory-kafka-{pressure}",
    Producer: eventbus.ProducerConfig{
        RequiredAcks:    1,
        Compression:     "snappy",
        FlushFrequency:  10 * time.Millisecond,
        FlushMessages:   100,
        Timeout:         30 * time.Second,
        MaxMessageBytes: 1048576,
    },
    Consumer: eventbus.ConsumerConfig{
        GroupID:            "memory-kafka-{pressure}-group",
        SessionTimeout:     10 * time.Second,
        HeartbeatInterval:  3 * time.Second,
        MaxProcessingTime:  30 * time.Second,
        AutoOffsetReset:    "earliest",
        EnableAutoCommit:   true,
        AutoCommitInterval: 1 * time.Second,
    },
}
```

### NATS 配置

```go
cfg := &eventbus.NATSConfig{
    URLs:     []string{"nats://localhost:4223"},
    ClientID: "memory-nats-{pressure}",
    JetStream: eventbus.JetStreamConfig{
        Enabled:        true,
        PublishTimeout: 30 * time.Second,
        AckWait:        30 * time.Second,
        MaxDeliver:     3,
        Stream: eventbus.StreamConfig{
            Name:      "TEST_MEMORY_NATS",
            Subjects:  []string{"test.memory.nats.>"},
            Storage:   "memory", // 内存持久化
            Retention: "limits",
            MaxAge:    24 * time.Hour,
            MaxBytes:  1024 * 1024 * 1024, // 1GB
            MaxMsgs:   1000000,
            Replicas:  1,
        },
    },
}
```

---

## ✅ **测试验证清单**

- [x] 创建 EventBus 实例（Kafka 和 NATS）
- [x] Kafka 和 NATS 都使用内存持久化
- [x] 使用 Publish 方法发布消息
- [x] 使用 Subscribe 方法订阅消息
- [x] 测试覆盖低压、中压、高压、极限 4 个场景
- [x] 无聚合ID
- [x] Topic 数量为 5
- [x] 输出性能指标和资源占用对比报告
- [x] 只使用 ASCII 字符命名
- [x] 每次测试前清理 Kafka 和 NATS
- [x] 检查全局 Worker 池使用情况
- [x] Kafka 采用 3 分区
- [x] 报告中体现 Kafka 分区数

---

**创建时间**: 2025-10-13  
**测试文件**: `memory_persistence_comparison_test.go`  
**测试函数**: `TestMemoryPersistenceComparison`

