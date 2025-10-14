# 快速运行内存持久化性能对比测试

## 🚀 **快速开始**

### 1. 启动服务

```bash
# 启动 Kafka（端口 29094）
docker-compose up -d kafka

# 启动 NATS JetStream（端口 4223）
docker-compose up -d nats
```

### 2. 运行测试

```bash
# 进入项目根目录
cd d:\JXT\jxt-evidence-system\jxt-core

# 运行测试
go test -v ./tests/eventbus/performance_tests/ -run TestMemoryPersistenceComparison -timeout 30m
```

---

## 📊 **测试场景**

测试将按顺序运行以下 4 个场景：

| 场景 | 消息数 | Topic 数 | 聚合ID | 预计时长 |
|------|--------|---------|--------|---------|
| Low | 500 | 5 | 无 | ~30 秒 |
| Medium | 2000 | 5 | 无 | ~1 分钟 |
| High | 5000 | 5 | 无 | ~2 分钟 |
| Extreme | 10000 | 5 | 无 | ~5 分钟 |

**总预计时长**: ~10-15 分钟

---

## 📈 **测试输出**

### 每个场景输出

1. **Kafka 测试**
   - 创建 topics（3 分区）
   - 发送和接收消息
   - 资源监控

2. **NATS 测试**
   - 创建 stream（内存存储）
   - 发送和接收消息
   - 资源监控

3. **对比报告**
   - 消息指标对比
   - 性能指标对比
   - 资源占用对比
   - 连接统计
   - Worker 池统计
   - 性能总结

### 最终汇总报告

- 所有场景的性能指标表格
- 关键发现列表

---

## 🔍 **关键指标**

### 性能指标
- **吞吐量**: msg/s（越高越好）
- **延迟**: ms（越低越好）
- **成功率**: %（应为 100%）

### 资源指标
- **协程泄漏**: 个（应为 0 或接近 0）
- **内存增量**: MB（越低越好）

### 连接指标
- **连接数**: 1（Kafka 和 NATS 各 1 个）
- **消费者组**: 1（每个系统 1 个）
- **分区数**: 3（Kafka 每个 topic）

---

## ⚠️ **注意事项**

### 1. 确保服务运行

```bash
# 检查 Kafka
docker ps | grep kafka

# 检查 NATS
docker ps | grep nats
```

### 2. 端口检查

- Kafka: `localhost:29094`
- NATS: `localhost:4223`

### 3. 资源要求

- **内存**: 至少 4GB 可用
- **CPU**: 至少 4 核
- **磁盘**: 至少 1GB 可用空间

### 4. 测试隔离

测试会自动清理数据，但建议：
- 不要在生产环境运行
- 不要与其他测试同时运行
- 确保 Kafka 和 NATS 没有其他客户端连接

---

## 🐛 **故障排查**

### 问题 1: 连接失败

**错误**: `Failed to connect to Kafka/NATS`

**解决**:
```bash
# 检查服务状态
docker-compose ps

# 重启服务
docker-compose restart kafka nats
```

### 问题 2: 测试超时

**错误**: `Test timeout`

**解决**:
- 增加超时时间: `-timeout 60m`
- 检查系统资源是否充足
- 检查网络连接

### 问题 3: 编译错误

**错误**: `Build failed`

**解决**:
```bash
# 清理缓存
go clean -cache

# 重新编译
go test -c ./tests/eventbus/performance_tests/
```

### 问题 4: 数据清理失败

**错误**: `Failed to delete topic/stream`

**解决**:
```bash
# 手动清理 Kafka
kafka-topics.sh --bootstrap-server localhost:29094 --delete --topic test.memory.kafka.*

# 手动清理 NATS（重启服务）
docker-compose restart nats
```

---

## 📝 **测试日志示例**

```
================================================================================
🎯 Kafka vs NATS JetStream 内存持久化性能对比测试
================================================================================

================================================================================
🚀 开始测试场景: Low 压力 (500 消息, 5 topics)
================================================================================

================================================================================
🔵 测试 Kafka - Low 压力 (500 消息, 5 topics)
================================================================================
🧹 清理 Kafka 测试数据 (topic prefix: test.memory.kafka)...
✅ 成功删除 0 个 Kafka topics
🔧 创建 Kafka Topics (3 分区)...
   ✅ 创建成功: test.memory.kafka.topic1 (3 partitions)
   ✅ 创建成功: test.memory.kafka.topic2 (3 partitions)
   ✅ 创建成功: test.memory.kafka.topic3 (3 partitions)
   ✅ 创建成功: test.memory.kafka.topic4 (3 partitions)
   ✅ 创建成功: test.memory.kafka.topic5 (3 partitions)
📊 验证创建的 Topics:
   test.memory.kafka.topic1: 3 partitions
   test.memory.kafka.topic2: 3 partitions
   test.memory.kafka.topic3: 3 partitions
   test.memory.kafka.topic4: 3 partitions
   test.memory.kafka.topic5: 3 partitions
✅ 成功创建 5 个 Kafka topics
📊 初始资源: Goroutines=45, Memory=12.34 MB
⏳ 等待订阅就绪...
📤 开始发送 500 条消息到 5 个 topics...
✅ 发送完成
⏳ 等待接收完成...
   接收进度: 100/500 (20.0%)
   接收进度: 250/500 (50.0%)
   接收进度: 450/500 (90.0%)
✅ 接收完成: 500/500

================================================================================
🟢 测试 NATS - Low 压力 (500 消息, 5 topics)
================================================================================
[类似的输出...]

================================================================================
📊 Kafka vs NATS JetStream 内存持久化性能对比报告 - Low 压力
================================================================================
[详细的对比报告...]

[重复 Medium, High, Extreme 场景...]

================================================================================
📊 所有场景汇总报告
================================================================================
[汇总表格...]

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

## 📊 **结果分析**

### 查看完整日志

测试日志会输出到控制台，建议重定向到文件：

```bash
go test -v ./tests/eventbus/performance_tests/ -run TestMemoryPersistenceComparison -timeout 30m 2>&1 | tee memory_test_results.log
```

### 关键指标分析

1. **吞吐量对比**
   - 查看 "发送吞吐量" 和 "接收吞吐量"
   - 比较 Kafka 和 NATS 的差异百分比

2. **延迟对比**
   - 查看 "平均发送延迟" 和 "平均处理延迟"
   - 延迟越低越好

3. **资源占用**
   - 查看 "内存增量" 和 "协程泄漏"
   - 确保没有严重的资源泄漏

4. **成功率**
   - 应该都是 100%
   - 如果低于 100%，需要调查原因

---

## 🎯 **下一步**

测试完成后，可以：

1. **分析结果**
   - 比较 Kafka 和 NATS 的性能差异
   - 识别性能瓶颈

2. **优化配置**
   - 调整 Kafka 分区数
   - 调整 NATS 内存限制
   - 调整 Worker 池大小

3. **扩展测试**
   - 增加更多压力场景
   - 测试不同的消息大小
   - 测试不同的 topic 数量

---

**准备好了吗？开始测试吧！** 🚀

```bash
go test -v ./tests/eventbus/performance_tests/ -run TestMemoryPersistenceComparison -timeout 30m
```

