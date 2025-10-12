# Kafka UnifiedConsumer 性能基准测试套件

## 🎯 测试目标

验证Kafka UnifiedConsumer方案在以下场景下的性能和可靠性：
- **单消费者组，单Kafka连接**
- **订阅10个topic主题**
- **Envelope格式消息持久化**
- **基于聚合ID的Keyed-Worker池顺序处理**
- **严格保证同一聚合ID的事件顺序**

## 📋 测试套件组成

### 1. 快速验证测试 (`kafka_unified_quick_test.go`)
**目的**: 验证基本功能正确性
**配置**:
- 10个topic
- 每个topic 100条消息
- 20个聚合ID
- 10个worker

**验证点**:
- ✅ 消息发送/接收成功率 > 95%
- ✅ 零顺序违反
- ✅ 零处理错误
- ✅ 聚合ID分布均匀

### 2. 性能基准测试 (`kafka_unified_performance_benchmark_test.go`)
**目的**: 测量详细性能指标
**配置**:
- 10个topic
- 每个topic 1000条消息
- 100个聚合ID
- 20个worker
- 30秒测试时长

**测量指标**:
- 📊 发送/接收吞吐量 (msg/s)
- ⏱️ 平均/最大延迟 (μs)
- 🔍 消息顺序性
- 📈 聚合ID处理分布

### 3. 压力测试 (`kafka_unified_stress_test.go`)
**目的**: 验证高负载下的稳定性
**配置**:
- 10个topic
- 5个生产者
- 20个消费者
- 5000 msg/s目标吞吐量
- 60秒测试时长
- 1KB消息大小

**压力指标**:
- 🚀 高吞吐量性能
- 💾 系统资源使用
- 🔥 错误率统计
- ⚡ 延迟分布

## 🚀 快速开始

### 前置条件

1. **Kafka服务运行**:
```bash
# 使用Docker启动Kafka
docker run -d --name kafka -p 9092:9092 \
  -e KAFKA_ZOOKEEPER_CONNECT=zookeeper:2181 \
  -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092 \
  -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
  confluentinc/cp-kafka:latest
```

2. **Go环境**: Go 1.19+

### 运行测试

#### 方式1: 使用测试脚本（推荐）
```bash
# 给脚本执行权限
chmod +x run_kafka_performance_tests.sh

# 运行快速验证
./run_kafka_performance_tests.sh quick

# 运行性能基准测试
./run_kafka_performance_tests.sh benchmark

# 运行压力测试
./run_kafka_performance_tests.sh stress

# 运行所有测试
./run_kafka_performance_tests.sh all
```

#### 方式2: 直接使用go test
```bash
# 快速验证测试
go test -v -run TestKafkaUnifiedConsumerQuickValidation -timeout 120s

# 性能基准测试
go test -v -run TestBenchmarkKafkaUnifiedConsumerPerformance -timeout 300s

# 压力测试
go test -v -run TestKafkaUnifiedConsumerStressTest -timeout 600s
```

## 📊 测试结果解读

### 快速验证测试结果示例
```
🚀 Kafka UnifiedConsumer 快速验证结果
============================================================
📊 消息统计:
   发送消息: 1000
   接收消息: 1000
   成功率: 100.00%
   处理错误: 0

🔍 顺序性验证:
   顺序违反: 0
   聚合ID数量: 20
============================================================
✅ 快速验证测试通过！
```

### 性能基准测试结果示例
```
🎯 Kafka UnifiedConsumer 性能基准测试结果
================================================================================
📊 基本统计:
   测试持续时间: 30.45 秒
   发送消息数: 10000
   接收消息数: 10000
   消息成功率: 100.00%

🚀 性能指标:
   发送吞吐量: 3284.52 msg/s
   接收吞吐量: 3284.52 msg/s
   平均发送延迟: 245.67 μs
   平均处理延迟: 1234.56 μs

❌ 错误统计:
   发送错误: 0
   处理错误: 0
   顺序违反: 0

🔍 聚合ID处理分析:
   总聚合数: 100
   消息分布: 98 - 102 (最少-最多)
   总顺序违反: 0
================================================================================
```

### 压力测试结果示例
```
🔥 Kafka UnifiedConsumer 压力测试结果
====================================================================================================
📊 基本统计:
   测试持续时间: 60.12 秒
   发送消息数: 298456
   接收消息数: 298456
   消息成功率: 100.00%
   发送错误: 0
   处理错误: 0

🚀 性能指标:
   发送吞吐量: 4967.23 msg/s
   接收吞吐量: 4967.23 msg/s
   目标吞吐量: 5000 msg/s
   吞吐量达成率: 99.3%

⏱️ 延迟统计:
   平均发送延迟: 156.78 μs
   最大发送延迟: 2345 μs
   平均处理延迟: 2345.67 μs
   最大处理延迟: 15678 μs

🔍 顺序性检查:
   顺序违反次数: 0
   顺序正确率: 100.0000%

💾 系统资源:
   最大内存使用: 245.67 MB
   最大协程数: 156
====================================================================================================
```

## 🎯 性能基准

### 预期性能指标

| 测试类型 | 吞吐量目标 | 延迟目标 | 成功率目标 | 顺序性目标 |
|----------|------------|----------|------------|------------|
| 快速验证 | > 1000 msg/s | < 10ms | > 95% | 100% |
| 性能基准 | > 3000 msg/s | < 5ms | > 99% | 100% |
| 压力测试 | > 4000 msg/s | < 20ms | > 95% | 100% |

### 关键验证点

1. **UnifiedConsumer架构优势**:
   - ✅ 单一ConsumerGroup避免再平衡
   - ✅ 统一消息路由机制
   - ✅ 高效的Keyed-Worker池

2. **消息顺序性保证**:
   - ✅ 同一聚合ID的消息严格顺序处理
   - ✅ 零顺序违反容忍度
   - ✅ 聚合ID hash分布均匀

3. **企业级特性**:
   - ✅ 消息持久化到Kafka
   - ✅ Envelope格式支持
   - ✅ 错误处理和重试机制
   - ✅ 系统资源监控

## 🔧 配置调优

### 高性能配置要点

1. **生产者配置**:
```go
Producer: ProducerConfig{
    RequiredAcks:     1,              // 平衡性能和可靠性
    Compression:      "snappy",       // 高效压缩
    FlushFrequency:   5 * time.Millisecond,  // 快速刷新
    FlushMessages:    200,            // 适中批次大小
    BatchSize:        32 * 1024,      // 32KB批次
    BufferSize:       64 * 1024 * 1024, // 64MB缓冲
    MaxInFlight:      10,             // 高并发
}
```

2. **消费者配置**:
```go
Consumer: ConsumerConfig{
    FetchMaxBytes:     100 * 1024 * 1024, // 100MB大批次
    FetchMaxWait:      100 * time.Millisecond, // 短等待
    MaxPollRecords:    1000,          // 大批次处理
    AutoCommitInterval: 1 * time.Second, // 频繁提交
}
```

## 🐛 故障排除

### 常见问题

1. **Kafka连接失败**:
   - 检查Kafka服务是否运行: `nc -z localhost 9092`
   - 检查防火墙设置
   - 验证Kafka配置

2. **测试超时**:
   - 增加测试超时时间
   - 检查系统资源使用
   - 调整测试参数

3. **顺序违反**:
   - 检查聚合ID提取逻辑
   - 验证Keyed-Worker池配置
   - 分析消息路由机制

4. **性能不达标**:
   - 调整Kafka配置参数
   - 增加worker池大小
   - 优化消息处理逻辑

## 📈 扩展测试

### 自定义测试场景

可以通过修改配置参数来测试不同场景：

```go
// 大规模测试
config := StressTestConfig{
    TopicCount:        50,    // 更多topic
    MessagesPerSecond: 10000, // 更高吞吐量
    TestDuration:      300 * time.Second, // 更长时间
}

// 高并发测试
config := BenchmarkConfig{
    WorkerPoolSize:   100,    // 更多worker
    AggregateCount:   1000,   // 更多聚合ID
}
```

## 🎉 总结

这套测试套件全面验证了Kafka UnifiedConsumer方案的：
- ✅ **功能正确性**: 消息发送接收、顺序处理
- ✅ **性能表现**: 吞吐量、延迟、资源使用
- ✅ **稳定性**: 高负载下的错误率和可靠性
- ✅ **企业特性**: 持久化、监控、故障处理

通过这些测试，可以充分证明UnifiedConsumer方案相比传统方案的优势，为生产环境部署提供可靠的性能基准。
