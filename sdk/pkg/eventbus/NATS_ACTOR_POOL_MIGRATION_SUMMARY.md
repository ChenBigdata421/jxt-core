# NATS JetStream Hollywood Actor Pool 迁移总结

## ✅ 迁移完成状态

**迁移日期**: 2025-10-29  
**迁移范围**: NATS JetStream EventBus  
**迁移方式**: 直接替换（无配置开关）  
**测试状态**: ✅ 全部通过

---

## 📋 迁移概览

### 迁移目标

将 NATS JetStream EventBus 从 **Keyed Worker Pool** 架构迁移到 **Hollywood Actor Pool** 架构，与 Kafka EventBus 保持一致。

### 迁移原则

1. **直接替换**: 不使用配置开关，迁移后只使用 Hollywood Actor Pool
2. **保持一致**: 与 Kafka EventBus 的 Actor Pool 实现保持一致
3. **向后兼容**: 确保所有现有功能正常工作
4. **测试覆盖**: 编写单元测试和回归测试验证迁移

---

## 🔧 核心修改

### 1. 修改 `natsEventBus` 结构体

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go`

**修改前**:
```go
type natsEventBus struct {
    // ...
    globalKeyedPool *KeyedWorkerPool  // 全局 Keyed-Worker Pool
    // ...
}
```

**修改后**:
```go
type natsEventBus struct {
    // ...
    actorPool *HollywoodActorPool  // Hollywood Actor Pool
    // ...
}
```

### 2. 初始化 Hollywood Actor Pool

**位置**: `NewNATSEventBus` 函数（第 400-416 行）

**修改前**:
```go
// 创建全局 Keyed-Worker Pool
bus.globalKeyedPool = NewKeyedWorkerPool(KeyedWorkerPoolConfig{
    WorkerCount: 256,
    QueueSize:   1000,
    WaitTimeout: 500 * time.Millisecond,
}, nil)
```

**修改后**:
```go
// 创建 Hollywood Actor Pool（所有 topic 共享，与 Kafka 保持一致）
metricsNamespace := fmt.Sprintf("nats_eventbus_%s", strings.ReplaceAll(config.ClientID, "-", "_"))
actorPoolMetrics := NewPrometheusActorPoolMetricsCollector(metricsNamespace)

bus.actorPool = NewHollywoodActorPool(HollywoodActorPoolConfig{
    PoolSize:    256,  // 固定 Actor 数量（与 Kafka 一致）
    InboxSize:   1000, // Inbox 队列大小
    MaxRestarts: 3,    // Supervisor 最大重启次数
}, actorPoolMetrics)

bus.logger.Info("NATS EventBus using Hollywood Actor Pool",
    zap.Int("poolSize", 256),
    zap.Int("inboxSize", 1000),
    zap.Int("maxRestarts", 3))
```

### 3. 更新消息处理逻辑

**位置**: `handleMessage` 函数（第 1383-1429 行）

**修改前**:
```go
if aggregateID != "" {
    pool := n.globalKeyedPool
    if pool != nil {
        aggMsg := &AggregateMessage{
            Topic:       topic,
            Partition:   0,
            Offset:      0,
            Key:         []byte(aggregateID),
            Value:       data,
            Headers:     make(map[string][]byte),
            Timestamp:   time.Now(),
            AggregateID: aggregateID,
            Context:     handlerCtx,
            Done:        make(chan error, 1),
            Handler:     handler,
        }
        
        if err := pool.ProcessMessage(handlerCtx, aggMsg); err != nil {
            // 错误处理
        }
        
        // 等待处理完成
        select {
        case err := <-aggMsg.Done:
            // 处理结果
        case <-handlerCtx.Done():
            // 超时处理
        }
    }
}
```

**修改后**:
```go
if aggregateID != "" {
    if n.actorPool != nil {
        aggMsg := &AggregateMessage{
            Topic:       topic,
            Partition:   0,
            Offset:      0,
            Key:         []byte(aggregateID),
            Value:       data,
            Headers:     make(map[string][]byte),
            Timestamp:   time.Now(),
            AggregateID: aggregateID,
            Context:     handlerCtx,
            Done:        make(chan error, 1),
            Handler:     handler,
        }
        
        if err := n.actorPool.ProcessMessage(handlerCtx, aggMsg); err != nil {
            // 错误处理
        }
        
        // 等待 Actor 处理完成
        select {
        case err := <-aggMsg.Done:
            // 处理结果
        case <-handlerCtx.Done():
            // 超时处理
        }
    }
}
```

### 4. 更新清理逻辑

**位置**: `Close` 函数（第 1549-1553 行）

**修改前**:
```go
// 停止全局 Keyed-Worker 池
if n.globalKeyedPool != nil {
    n.globalKeyedPool.Stop()
    n.logger.Debug("Stopped global keyed worker pool")
}
```

**修改后**:
```go
// 停止 Hollywood Actor Pool
if n.actorPool != nil {
    n.actorPool.Stop()
    n.logger.Debug("Stopped Hollywood Actor Pool")
}
```

---

## 🧪 测试结果

### 单元测试

**文件**: `jxt-core/sdk/pkg/eventbus/nats_actor_pool_test.go`

| 测试名称 | 状态 | 描述 |
|---------|------|------|
| `TestNATSActorPool_BasicProcessing` | ✅ PASS | 基本的消息处理 |
| `TestNATSActorPool_EnvelopeProcessing` | ✅ PASS | Envelope 消息处理 |
| `TestNATSActorPool_OrderGuarantee` | ✅ PASS | 同一聚合ID的消息顺序保证 |
| `TestNATSActorPool_MultipleAggregates` | ✅ PASS | 多个聚合ID的并发处理 |

**总计**: 4/4 通过 (100%)

### 回归测试

**文件**: `jxt-core/tests/eventbus/function_regression_tests/kafka_nats_test.go`

| 测试名称 | 状态 | 描述 |
|---------|------|------|
| `TestNATSBasicPublishSubscribe` | ✅ PASS | NATS 基本发布订阅 |
| `TestNATSMultipleMessages` | ✅ PASS | NATS 多条消息 |
| `TestNATSEnvelopePublishSubscribe` | ✅ PASS | NATS Envelope 发布订阅 |
| `TestNATSEnvelopeOrdering` | ✅ PASS | NATS Envelope 顺序保证 |
| `TestNATSMultipleAggregates` | ✅ PASS | NATS 多聚合并发处理（新增） |
| `TestNATSClose` | ✅ PASS | NATS 关闭测试 |

**总计**: 6/6 通过 (100%)

### 核心回归测试汇总

| 类别 | 测试数量 | 通过数量 | 通过率 |
|------|---------|---------|--------|
| JSON 序列化 | 3 | 3 | 100% |
| Kafka 核心功能 | 6 | 6 | 100% |
| NATS 核心功能 | 6 | 6 | 100% |
| **总计** | **15** | **15** | **100%** |

---

## 📊 架构对比

### Keyed Worker Pool vs Hollywood Actor Pool

| 特性 | Keyed Worker Pool | Hollywood Actor Pool |
|------|------------------|---------------------|
| **Worker/Actor 数量** | 256 | 256 |
| **队列大小** | 1000 | 1000 (Inbox) |
| **消息路由** | 一致性哈希 | 一致性哈希 |
| **顺序保证** | ✅ 同一 Key 顺序处理 | ✅ 同一 AggregateID 顺序处理 |
| **错误处理** | 手动处理 | Supervisor 自动重启 |
| **监控指标** | 无 | ✅ Prometheus 指标 |
| **故障隔离** | ❌ 无 | ✅ 单个 Actor 故障不影响其他 |
| **自恢复能力** | ❌ 无 | ✅ Supervisor 机制 |

### Hollywood Actor Pool 优势

1. **Supervisor 机制**: 自动重启失败的 Actor（最多 3 次）
2. **故障隔离**: 单个 Actor 失败不影响其他 Actor
3. **监控指标**: 内置 Prometheus 指标收集
4. **事件流**: 支持 DeadLetterEvent、ActorRestartedEvent 等事件
5. **一致性**: 与 Kafka EventBus 保持一致的架构

---

## 🎯 关键要点

### 1. 配置参数

- **PoolSize**: 256（固定，不可配置）
- **InboxSize**: 1000（固定，不可配置）
- **MaxRestarts**: 3（固定，不可配置）

### 2. 消息路由

- 使用一致性哈希：`Hash(aggregateID) % poolSize → actorIndex`
- 同一 `aggregateID` 的消息总是路由到同一个 Actor
- 保证同一聚合的消息顺序处理

### 3. 错误处理策略

- **业务错误**: 不触发 Actor 重启，通过 Done channel 返回错误
- **系统错误**: 触发 Actor 重启，由 Supervisor 处理
- **最大重启次数**: 3 次，超过后 Actor 停止

### 4. Prometheus 指标

- **命名空间**: `nats_eventbus_{clientID}`
- **指标类型**:
  - `actor_pool_messages_sent_total`: 发送到 Actor 的消息总数
  - `actor_pool_messages_processed_total`: Actor 处理的消息总数
  - `actor_pool_message_processing_duration_seconds`: 消息处理耗时
  - `actor_pool_inbox_depth`: Actor Inbox 深度
  - `actor_pool_actor_restarts_total`: Actor 重启次数

---

## ✅ 迁移验证清单

- [x] 修改 `natsEventBus` 结构体，替换 `globalKeyedPool` 为 `actorPool`
- [x] 更新 `NewNATSEventBus` 函数，初始化 Hollywood Actor Pool
- [x] 更新 `handleMessage` 函数，使用 Actor Pool 处理消息
- [x] 更新 `Close` 函数，停止 Actor Pool
- [x] 编写单元测试（4 个测试用例）
- [x] 运行单元测试，全部通过
- [x] 添加回归测试（`TestNATSMultipleAggregates`）
- [x] 运行回归测试，全部通过
- [x] 验证消息顺序保证
- [x] 验证多聚合并发处理
- [x] 验证 Prometheus 指标收集

---

## 🚀 后续工作

### 可选优化

1. **性能测试**: 对比 Keyed Worker Pool 和 Hollywood Actor Pool 的性能差异
2. **压力测试**: 验证 Actor Pool 在高负载下的表现
3. **监控仪表板**: 创建 Grafana 仪表板展示 Actor Pool 指标
4. **文档更新**: 更新用户文档，说明 Actor Pool 的使用方式

### 已知问题

1. **Prometheus Metrics 冲突**: 在同一测试进程中运行多个测试时，可能出现 metrics 重复注册错误
   - **影响范围**: 仅影响测试环境
   - **解决方案**: 为每个测试使用唯一的 ClientID

---

## 📝 总结

NATS JetStream EventBus 已成功从 Keyed Worker Pool 迁移到 Hollywood Actor Pool：

- ✅ **迁移完成**: 所有代码修改已完成
- ✅ **测试通过**: 单元测试和回归测试全部通过（100%）
- ✅ **功能验证**: 消息顺序保证、多聚合并发处理等核心功能正常
- ✅ **架构一致**: 与 Kafka EventBus 保持一致的 Actor Pool 架构
- ✅ **监控完善**: 内置 Prometheus 指标收集

**迁移成功！** 🎉

