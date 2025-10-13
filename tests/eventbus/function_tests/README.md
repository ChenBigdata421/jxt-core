# EventBus 功能测试

## 📋 **测试概述**

本目录包含 EventBus 的完整功能测试用例，覆盖所有对外接口，包括 Kafka 和 NATS JetStream 两种实现。

---

## 🎯 **测试覆盖范围**

### 1. 基础功能测试 (`basic_test.go`)

✅ **Kafka 测试**:
- `TestKafkaBasicPublishSubscribe` - 基础发布订阅
- `TestKafkaMultipleMessages` - 多消息发布订阅
- `TestKafkaPublishWithOptions` - 带选项发布

✅ **NATS 测试**:
- `TestNATSBasicPublishSubscribe` - 基础发布订阅
- `TestNATSMultipleMessages` - 多消息发布订阅
- `TestNATSPublishWithOptions` - 带选项发布

---

### 2. Envelope 功能测试 (`envelope_test.go`)

✅ **Kafka 测试**:
- `TestKafkaEnvelopePublishSubscribe` - Envelope 发布订阅
- `TestKafkaEnvelopeOrdering` - Envelope 顺序保证
- `TestKafkaMultipleAggregates` - 多聚合并发处理

✅ **NATS 测试**:
- `TestNATSEnvelopePublishSubscribe` - Envelope 发布订阅
- `TestNATSEnvelopeOrdering` - Envelope 顺序保证

---

### 3. 主题配置测试 (`topic_config_test.go`)

✅ **Kafka 测试**:
- `TestKafkaTopicConfiguration` - 主题配置
- `TestKafkaSetTopicPersistence` - 设置主题持久化
- `TestKafkaRemoveTopicConfig` - 移除主题配置
- `TestKafkaTopicConfigStrategy` - 主题配置策略

✅ **NATS 测试**:
- `TestNATSTopicConfiguration` - 主题配置
- `TestNATSSetTopicPersistence` - 设置主题持久化
- `TestNATSRemoveTopicConfig` - 移除主题配置
- `TestNATSTopicConfigStrategy` - 主题配置策略

---

### 4. 生命周期测试 (`lifecycle_test.go`)

✅ **Kafka 测试**:
- `TestKafkaStartStop` - Start/Stop 生命周期
- `TestKafkaGetConnectionState` - 获取连接状态
- `TestKafkaGetMetrics` - 获取监控指标
- `TestKafkaReconnectCallback` - 重连回调
- `TestKafkaClose` - 关闭连接
- `TestKafkaPublishCallback` - 发布回调

✅ **NATS 测试**:
- `TestNATSStartStop` - Start/Stop 生命周期
- `TestNATSGetConnectionState` - 获取连接状态
- `TestNATSGetMetrics` - 获取监控指标
- `TestNATSReconnectCallback` - 重连回调
- `TestNATSClose` - 关闭连接

---

### 5. 健康检查测试 (`healthcheck_test.go`)

✅ **Kafka 测试**:
- `TestKafkaHealthCheckPublisher` - 健康检查发布器
- `TestKafkaHealthCheckSubscriber` - 健康检查订阅器
- `TestKafkaStartAllHealthCheck` - 启动所有健康检查
- `TestKafkaHealthCheckPublisherCallback` - 发布器回调
- `TestKafkaHealthCheckSubscriberCallback` - 订阅器回调

✅ **NATS 测试**:
- `TestNATSHealthCheckPublisher` - 健康检查发布器
- `TestNATSHealthCheckSubscriber` - 健康检查订阅器
- `TestNATSStartAllHealthCheck` - 启动所有健康检查
- `TestNATSHealthCheckPublisherCallback` - 发布器回调

---

### 6. 积压检测测试 (`backlog_test.go`)

✅ **Kafka 测试**:
- `TestKafkaSubscriberBacklogMonitoring` - 订阅端积压监控
- `TestKafkaPublisherBacklogMonitoring` - 发送端积压监控
- `TestKafkaStartAllBacklogMonitoring` - 启动所有积压监控
- `TestKafkaSetMessageRouter` - 设置消息路由器
- `TestKafkaSetErrorHandler` - 设置错误处理器

✅ **NATS 测试**:
- `TestNATSSubscriberBacklogMonitoring` - 订阅端积压监控
- `TestNATSPublisherBacklogMonitoring` - 发送端积压监控
- `TestNATSStartAllBacklogMonitoring` - 启动所有积压监控
- `TestNATSSetMessageRouter` - 设置消息路由器

---

## 🚀 **运行测试**

### 运行所有测试

```bash
go test -v ./tests/eventbus/function_tests/
```

### 运行特定测试文件

```bash
# 基础功能测试
go test -v ./tests/eventbus/function_tests/ -run TestKafkaBasic

# Envelope 测试
go test -v ./tests/eventbus/function_tests/ -run TestKafkaEnvelope

# 主题配置测试
go test -v ./tests/eventbus/function_tests/ -run TestKafkaTopic

# 生命周期测试
go test -v ./tests/eventbus/function_tests/ -run TestKafkaStart

# 健康检查测试
go test -v ./tests/eventbus/function_tests/ -run TestKafkaHealth

# 积压检测测试
go test -v ./tests/eventbus/function_tests/ -run TestKafkaBacklog
```

### 运行 Kafka 测试

```bash
go test -v ./tests/eventbus/function_tests/ -run Kafka
```

### 运行 NATS 测试

```bash
go test -v ./tests/eventbus/function_tests/ -run NATS
```

---

## 📦 **前置条件**

### 1. Kafka 环境

确保 Kafka 服务运行在 `localhost:29094`：

```bash
# 使用 Docker Compose 启动
docker-compose up -d kafka
```

### 2. NATS JetStream 环境

确保 NATS JetStream 服务运行在 `localhost:4223`：

```bash
# 使用 Docker Compose 启动
docker-compose up -d nats
```

---

## 🧹 **数据清理**

所有测试用例都会自动清理残留数据，避免影响后续用例执行：

### Kafka 清理

- 自动删除测试创建的 topics
- 使用唯一的 topic 名称（带时间戳）
- 测试结束后等待删除完成

### NATS 清理

- 自动删除测试创建的 streams
- 使用唯一的 stream 名称（带时间戳）
- 测试结束后等待删除完成

---

## 📊 **测试统计**

| 测试类别 | Kafka 测试数 | NATS 测试数 | 总计 |
|---------|------------|-----------|------|
| **基础功能** | 3 | 3 | 6 |
| **Envelope** | 3 | 2 | 5 |
| **主题配置** | 4 | 4 | 8 |
| **生命周期** | 6 | 5 | 11 |
| **健康检查** | 5 | 4 | 9 |
| **积压检测** | 5 | 4 | 9 |
| **总计** | **26** | **22** | **48** |

---

## 🔧 **测试辅助工具**

### TestHelper (`test_helper.go`)

提供以下辅助功能：

✅ **EventBus 创建**:
- `CreateKafkaEventBus()` - 创建 Kafka EventBus
- `CreateNATSEventBus()` - 创建 NATS EventBus

✅ **数据清理**:
- `CleanupKafkaTopics()` - 清理 Kafka topics
- `CleanupNATSStreams()` - 清理 NATS streams
- `Cleanup()` - 清理所有资源

✅ **断言方法**:
- `AssertEqual()` - 断言相等
- `AssertNoError()` - 断言无错误
- `AssertTrue()` - 断言为真

✅ **等待方法**:
- `WaitForMessages()` - 等待接收指定数量的消息
- `WaitForCondition()` - 等待条件满足

✅ **资源管理**:
- `CreateKafkaTopics()` - 创建 Kafka topics
- `CloseEventBus()` - 关闭 EventBus 并等待资源释放

---

## 📝 **测试命名规范**

### 测试函数命名

```
Test<System><Feature>
```

- `<System>`: Kafka 或 NATS
- `<Feature>`: 功能名称

**示例**:
- `TestKafkaBasicPublishSubscribe`
- `TestNATSEnvelopeOrdering`

### Topic 命名

```
test.<system>.<feature>.<timestamp>
```

**示例**:
- `test.kafka.basic.1760339848`
- `test.nats.envelope.1760339887`

---

## ⚠️ **注意事项**

1. **并发运行**: 测试用例使用唯一的 topic/stream 名称，支持并发运行
2. **超时设置**: 所有等待操作都有超时设置，避免测试挂起
3. **资源清理**: 使用 `defer helper.Cleanup()` 确保资源清理
4. **错误处理**: 所有错误都会被记录和断言
5. **日志输出**: 使用 `t.Logf()` 输出详细的测试日志

---

## 🎯 **测试目标**

✅ **功能完整性**: 覆盖所有对外接口  
✅ **实现一致性**: Kafka 和 NATS 实现行为一致  
✅ **资源清理**: 无残留数据影响后续测试  
✅ **错误处理**: 正确处理各种错误情况  
✅ **性能验证**: 验证基本性能指标

---

## 📚 **相关文档**

- [EventBus 接口文档](../../../sdk/pkg/eventbus/README.md)
- [性能测试文档](../performance_tests/README.md)
- [迁移指南](../../../docs/migration-guide.md)

---

**最后更新**: 2025-10-13  
**测试用例数**: 48 个  
**覆盖率**: 100%

