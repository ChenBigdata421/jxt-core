# Kafka vs NATS JetStream 性能对比测试 - 快速入门

## 🚀 5分钟快速开始

### 步骤 1: 验证测试文件

**Linux/Mac:**
```bash
cd sdk/pkg/eventbus/performance_tests
chmod +x verify_test.sh
./verify_test.sh
```

**Windows:**
```cmd
cd sdk\pkg\eventbus\performance_tests
verify_test.bat
```

如果看到 "✅ 测试文件编译成功！"，说明一切正常。

### 步骤 2: 启动服务

确保 Kafka 和 NATS 服务正在运行：

```bash
# 启动 Kafka (RedPanda)
docker-compose up -d kafka

# 启动 NATS JetStream
docker-compose up -d nats

# 验证服务状态
docker ps | grep -E "kafka|nats"
```

### 步骤 3: 运行测试

**Linux/Mac:**
```bash
chmod +x run_comparison_test.sh
./run_comparison_test.sh
```

**Windows:**
```cmd
run_comparison_test.bat
```

**或者直接运行:**
```bash
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 30m
```

### 步骤 4: 查看结果

测试完成后，你会看到：

1. **实时输出** - 控制台显示详细的测试进度和结果
2. **日志文件** - 保存在 `test_results/comparison_test_YYYYMMDD_HHMMSS.log`

## 📊 预期输出示例

```
================================================================================
📊 低压 测试结果对比
================================================================================

📨 消息统计:
   发送消息数           | Kafka:      500 | NATS:      500
   接收消息数           | Kafka:      500 | NATS:      500
   成功率               | Kafka:  100.00% | NATS:  100.00%

🚀 性能指标:
   发送吞吐量           | Kafka:    1234.56 msg/s | NATS:    2345.67 msg/s
   接收吞吐量           | Kafka:    1234.56 msg/s | NATS:    2345.67 msg/s
   平均发送延迟         | Kafka:      0.123 ms | NATS:      0.056 ms
   平均处理延迟         | Kafka:      0.234 ms | NATS:      0.123 ms

💾 资源占用:
   峰值协程数           | Kafka:       67 | NATS:       52
   内存增量             | Kafka:      11.11 MB | NATS:       7.11 MB

🏆 性能优势:
   NATS 吞吐量优势: +90.12%
   NATS 延迟优势: -47.44%
   NATS 内存优势: -36.00%
================================================================================
```

## 🎯 测试内容

### 测试场景
- ✅ **低压**: 500 条消息 (60秒超时)
- ✅ **中压**: 2000 条消息 (120秒超时)
- ✅ **高压**: 5000 条消息 (180秒超时)
- ✅ **极限**: 10000 条消息 (300秒超时)

### 测试配置
- ✅ **Kafka**: 持久化 (RequiredAcks=-1), Snappy 压缩, 幂等性
- ✅ **NATS**: 磁盘持久化 (Storage=file), 1GB 存储限制
- ✅ **发布**: 使用 `PublishEnvelope()` 方法
- ✅ **订阅**: 使用 `SubscribeEnvelope()` 方法

### 测试指标
- 📊 **性能**: 吞吐量、延迟、成功率
- 💾 **资源**: 协程数、内存占用
- 🔄 **可靠性**: 顺序性、错误率

## ⏱️ 测试时间

- **完整测试**: 约 15-20 分钟 (4个场景)
- **单场景**: 约 1-5 分钟

## 🔧 自定义测试

### 只运行低压测试

编辑 `kafka_nats_comparison_test.go`，修改 `scenarios`:

```go
scenarios := []struct {
    name     string
    messages int
    timeout  time.Duration
}{
    {"低压", 500, 60 * time.Second}, // 只保留这一行
}
```

### 修改消息数量

```go
scenarios := []struct {
    name     string
    messages int
    timeout  time.Duration
}{
    {"自定义", 1000, 90 * time.Second},
}
```

## 📚 更多文档

- **README.md** - 完整的使用说明
- **EXAMPLE_REPORT.md** - 详细的测试报告示例
- **SUMMARY.md** - 测试套件总结
- **COMPLETION_REPORT.md** - 任务完成报告

## ❓ 常见问题

### Q: 测试失败，提示连接不上 Kafka
**A:** 检查 Kafka 是否运行：
```bash
docker ps | grep kafka
netstat -an | grep 29092  # Linux/Mac
netstat -an | findstr 29092  # Windows
```

### Q: 测试失败，提示连接不上 NATS
**A:** 检查 NATS 是否运行：
```bash
docker ps | grep nats
netstat -an | grep 4223  # Linux/Mac
netstat -an | findstr 4223  # Windows
```

### Q: 测试超时
**A:** 可能原因：
- 系统资源不足
- 网络延迟
- 消息数量太多

解决方案：
- 增加 timeout 参数
- 减少消息数量
- 关闭其他应用释放资源

### Q: 如何只测试 Kafka 或 NATS？
**A:** 注释掉不需要的测试：
```go
// 只测试 Kafka
kafkaMetrics := runKafkaTest(t, scenario.name, scenario.messages, scenario.timeout)
// natsMetrics := runNATSTest(t, scenario.name, scenario.messages, scenario.timeout)
```

### Q: 如何保存测试结果？
**A:** 测试结果会自动保存到 `test_results/` 目录，文件名包含时间戳。

## 🎉 下一步

1. ✅ 运行测试，查看结果
2. 📊 分析性能数据
3. 💡 根据结果做技术选型
4. 🔧 根据需求调整配置

## 📞 获取帮助

如果遇到问题：

1. 查看 **README.md** 的故障排除部分
2. 检查测试日志文件
3. 查看 **EXAMPLE_REPORT.md** 了解预期输出
4. 阅读 **SUMMARY.md** 了解测试详情

---

*快速入门指南*  
*版本: 1.0.0*  
*更新日期: 2025-10-12*

