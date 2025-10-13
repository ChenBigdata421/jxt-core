# Kafka vs NATS JetStream 性能对比测试

## 📋 测试概述

这是一个完整的 Kafka 和 NATS JetStream 性能对比测试套件，专门设计用于评估两种消息系统在不同压力级别下的表现。

### 测试特点

✅ **创建 EventBus 实例** - 覆盖 Kafka 和 NATS JetStream 两种不同实现  
✅ **NATS JetStream 磁盘持久化** - 确保数据持久化到磁盘  
✅ **使用 PublishEnvelope 方法** - 发布端采用标准 Envelope 格式  
✅ **使用 SubscribeEnvelope 方法** - 订阅端采用标准 Envelope 处理  
✅ **多压力级别测试** - 覆盖低压500、中压2000、高压5000、极限10000  
✅ **详细性能报告** - 包括性能指标和关键资源占用情况对比  

## 🎯 测试场景

| 压力级别 | 消息数量 | 超时时间 | 说明 |
|---------|---------|---------|------|
| 低压 | 500 | 60秒 | 基础功能验证 |
| 中压 | 2000 | 120秒 | 常规负载测试 |
| 高压 | 5000 | 180秒 | 高负载压力测试 |
| 极限 | 10000 | 300秒 | 极限性能测试 |

## 📊 测试指标

### 消息指标
- 发送消息数
- 接收消息数
- 发送错误数
- 处理错误数
- 成功率
- 顺序违反次数

### 性能指标
- 发送吞吐量 (msg/s)
- 接收吞吐量 (msg/s)
- 平均发送延迟 (ms)
- 平均处理延迟 (ms)
- 总耗时 (s)

### 资源占用
- 初始协程数
- 峰值协程数
- 最终协程数
- 协程泄漏数
- 初始内存 (MB)
- 峰值内存 (MB)
- 最终内存 (MB)
- 内存增量 (MB)

## 🚀 运行测试

### 前置条件

1. **启动 Kafka (RedPanda)**
```bash
# 确保 Kafka 在 localhost:29092 运行
docker-compose up -d kafka
```

2. **启动 NATS JetStream**
```bash
# 确保 NATS 在 localhost:4223 运行
docker-compose up -d nats
```

### 运行测试

```bash
# 进入测试目录
cd sdk/pkg/eventbus/performance_tests

# 运行完整测试
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 30m

# 运行单个压力级别（修改测试代码中的 scenarios）
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 10m
```

### 快速测试（仅低压）

如果只想快速验证，可以修改测试代码，只保留低压场景：

```go
scenarios := []struct {
    name     string
    messages int
    timeout  time.Duration
}{
    {"低压", 500, 60 * time.Second},
}
```

## 📈 测试报告示例

测试完成后会生成详细的对比报告：

```
================================================================================
📊 低压 测试结果对比
================================================================================

📨 消息统计:
   发送消息数           | Kafka:      500 | NATS:      500
   接收消息数           | Kafka:      500 | NATS:      500
   成功率               | Kafka:  100.00% | NATS:  100.00%
   发送错误             | Kafka:        0 | NATS:        0
   处理错误             | Kafka:        0 | NATS:        0
   顺序违反             | Kafka:        0 | NATS:        0

🚀 性能指标:
   发送吞吐量           | Kafka:    1234.56 msg/s | NATS:    2345.67 msg/s
   接收吞吐量           | Kafka:    1234.56 msg/s | NATS:    2345.67 msg/s
   平均发送延迟         | Kafka:      0.123 ms | NATS:      0.056 ms
   平均处理延迟         | Kafka:      0.234 ms | NATS:      0.123 ms
   总耗时               | Kafka:       0.41 s | NATS:       0.21 s

💾 资源占用:
   初始协程数           | Kafka:       45 | NATS:       38
   峰值协程数           | Kafka:       67 | NATS:       52
   最终协程数           | Kafka:       46 | NATS:       39
   协程泄漏             | Kafka:        1 | NATS:        1
   初始内存             | Kafka:      12.34 MB | NATS:       8.56 MB
   峰值内存             | Kafka:      23.45 MB | NATS:      15.67 MB
   最终内存             | Kafka:      13.45 MB | NATS:       9.23 MB
   内存增量             | Kafka:      11.11 MB | NATS:       7.11 MB

🏆 性能优势:
   NATS 吞吐量优势: +90.12%
   NATS 延迟优势: -47.44%
   NATS 内存优势: -36.00%
================================================================================
```

## 🏆 综合报告

测试结束后会生成综合对比报告，包括：

1. **性能指标汇总表** - 所有场景的详细数据
2. **综合评分** - 平均吞吐量、延迟、内存占用
3. **最终结论** - 各项指标的获胜者
4. **使用建议** - 针对不同场景的选择建议

## 🔧 配置说明

### Kafka 配置

- **持久化**: RequiredAcks = -1 (WaitForAll)
- **压缩**: Snappy
- **幂等性**: 启用
- **批量大小**: 16KB
- **缓冲区**: 32MB

### NATS JetStream 配置

- **存储**: file (磁盘持久化) 🔑
- **保留策略**: limits
- **最大存储**: 1GB
- **副本数**: 1
- **ACK 策略**: explicit

## 📝 注意事项

1. **测试时间**: 完整测试（4个场景）预计需要 15-20 分钟
2. **资源要求**: 建议至少 4GB 可用内存
3. **网络要求**: 确保 Kafka 和 NATS 服务可访问
4. **并发控制**: 测试使用 100 个并发批次发送消息
5. **聚合ID**: 使用 100 个不同的聚合ID测试顺序性

## 🐛 故障排除

### Kafka 连接失败
```bash
# 检查 Kafka 是否运行
docker ps | grep kafka
# 检查端口
netstat -an | grep 29092
```

### NATS 连接失败
```bash
# 检查 NATS 是否运行
docker ps | grep nats
# 检查端口
netstat -an | grep 4223
```

### 测试超时
- 增加 timeout 参数
- 减少消息数量
- 检查系统资源

### 顺序违反
- 检查 Keyed-Worker 池配置
- 验证聚合ID提取逻辑
- 查看日志中的错误信息

## 📚 相关文档

- [EventBus README](../README.md)
- [Kafka 性能测试文档](../KAFKA_PERFORMANCE_TESTS_README.md)
- [NATS 性能基准报告](../NATS_PERFORMANCE_BENCHMARK_FINAL_REPORT.md)
- [Kafka vs NATS 对比分析](../KAFKA_VS_NATS_FINAL_ANALYSIS.md)

## 🎉 总结

这个测试套件提供了全面的 Kafka 和 NATS JetStream 性能对比，帮助你：

- ✅ 了解两种系统在不同负载下的表现
- ✅ 评估资源占用和效率
- ✅ 验证消息顺序性保证
- ✅ 做出明智的技术选型决策

---

*测试文件: `kafka_nats_comparison_test.go`*  
*创建日期: 2025-10-12*  
*版本: 1.0.0*

