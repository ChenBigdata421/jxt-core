# Kafka EventBus 迁移到 Hollywood Actor Pool - 实施方案

> **目标**: 将 Kafka EventBus 从 Keyed Worker Pool 迁移到 Hollywood Actor Pool
> 
> **范围**: 仅 Kafka EventBus (`kafka.go`)，NATS EventBus 后续单独迁移
> 
> **策略**: 功能开关 + 灰度发布 + 快速回滚

---

## 🎯 **迁移目标**

### 当前架构 (Keyed Worker Pool)

```go
// kafka.go:336
type kafkaEventBus struct {
    // ...
    globalKeyedPool *KeyedWorkerPool  // ⭐ 当前使用
}

// kafka.go:616 - 初始化
bus.globalKeyedPool = NewKeyedWorkerPool(KeyedWorkerPoolConfig{
    WorkerCount: 256,                    // 256 workers
    QueueSize:   1000,                   // 每个 worker 队列 1000
    WaitTimeout: 500 * time.Millisecond,
}, nil)

// kafka.go:1038 - 消息处理
pool := h.eventBus.globalKeyedPool
pool.ProcessMessage(ctx, aggMsg)
```

### 目标架构 (Hollywood Actor Pool)

```go
// kafka.go:336
type kafkaEventBus struct {
    // ...
    globalActorPool *HollywoodActorPool  // ⭐ 迁移目标
    
    // 功能开关
    useActorPool atomic.Bool  // true: Actor Pool, false: Keyed Pool
}

// kafka.go:616 - 初始化
bus.globalActorPool = NewHollywoodActorPool(HollywoodActorPoolConfig{
    PoolSize:    256,      // 256 actors (与 Keyed Pool 一致)
    InboxSize:   1000,     // Inbox 1000 (与 QueueSize 一致)
    MaxRestarts: 3,        // Supervisor 最大重启次数
}, metricsCollector)

// kafka.go:1038 - 消息处理
pool := h.eventBus.globalActorPool
pool.ProcessMessage(ctx, aggMsg)
```

---

## 📋 **实施步骤**

### 阶段 1: 代码实现 (2-3 天)

#### 步骤 1.1: 添加 Hollywood Actor Pool 字段

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**位置**: Line 336 附近

```go
type kafkaEventBus struct {
    // ... 现有字段 ...
    
    // 全局 Keyed-Worker Pool（所有 topic 共享）
    globalKeyedPool *KeyedWorkerPool  // ⭐ 保留，用于回滚
    
    // 全局 Hollywood Actor Pool（所有 topic 共享）
    globalActorPool *HollywoodActorPool  // ⭐ 新增
    
    // 功能开关：true = Actor Pool, false = Keyed Pool
    useActorPool atomic.Bool  // ⭐ 新增
}
```

#### 步骤 1.2: 初始化 Hollywood Actor Pool

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**位置**: Line 616 附近 (在 `globalKeyedPool` 初始化之后)

```go
// 创建全局 Keyed-Worker Pool（所有 topic 共享）
bus.globalKeyedPool = NewKeyedWorkerPool(KeyedWorkerPoolConfig{
    WorkerCount: 256,
    QueueSize:   1000,
    WaitTimeout: 500 * time.Millisecond,
}, nil)

// ⭐ 创建全局 Hollywood Actor Pool（所有 topic 共享）
// 创建 Prometheus 监控收集器
metricsCollector := NewPrometheusActorPoolMetricsCollector("kafka_eventbus")

bus.globalActorPool = NewHollywoodActorPool(HollywoodActorPoolConfig{
    PoolSize:    256,      // 与 Keyed Pool 一致
    InboxSize:   1000,     // 与 QueueSize 一致
    MaxRestarts: 3,        // Supervisor 配置
}, metricsCollector)

// ⭐ 初始化功能开关（默认使用 Keyed Pool，确保安全）
bus.useActorPool.Store(false)

// ⭐ 从环境变量或配置读取功能开关
if os.Getenv("KAFKA_USE_ACTOR_POOL") == "true" {
    bus.useActorPool.Store(true)
    bus.logger.Info("Kafka EventBus using Hollywood Actor Pool")
} else {
    bus.logger.Info("Kafka EventBus using Keyed Worker Pool")
}
```

#### 步骤 1.3: 修改消息处理逻辑 (预订阅模式)

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**位置**: Line 1038 附近 (`ConsumeClaim` 方法中)

```go
// 尝试提取聚合ID
aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")
if aggregateID != "" {
    // ⭐ 根据功能开关选择 Pool
    if h.eventBus.useActorPool.Load() {
        // ✅ 使用 Hollywood Actor Pool
        pool := h.eventBus.globalActorPool
        if pool != nil {
            aggMsg := &AggregateMessage{
                Topic:       message.Topic,
                Partition:   message.Partition,
                Offset:      message.Offset,
                Key:         message.Key,
                Value:       message.Value,
                Headers:     make(map[string][]byte),
                Timestamp:   message.Timestamp,
                AggregateID: aggregateID,
                Context:     ctx,
                Done:        make(chan error, 1),
                Handler:     h.handler,
            }
            for _, header := range message.Headers {
                aggMsg.Headers[string(header.Key)] = header.Value
            }
            if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
                return err
            }
            select {
            case err := <-aggMsg.Done:
                return err
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    } else {
        // ✅ 使用 Keyed Worker Pool (原有逻辑)
        pool := h.eventBus.globalKeyedPool
        if pool != nil {
            // ... 原有代码 ...
        }
    }
}
```

#### 步骤 1.4: 修改消息处理逻辑 (普通模式)

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**位置**: Line 1158 附近 (`processMessageWithKeyedPool` 方法)

```go
func (h *preSubscriptionConsumerHandler) processMessageWithKeyedPool(ctx context.Context, message *sarama.ConsumerMessage, handler MessageHandler, session sarama.ConsumerGroupSession) error {
    // ... 提取聚合ID ...
    
    if aggregateID != "" {
        // ⭐ 根据功能开关选择 Pool
        if h.eventBus.useActorPool.Load() {
            // ✅ 使用 Hollywood Actor Pool
            pool := h.eventBus.globalActorPool
            if pool != nil {
                // ... Actor Pool 处理逻辑 ...
            }
        } else {
            // ✅ 使用 Keyed Worker Pool (原有逻辑)
            pool := h.eventBus.globalKeyedPool
            if pool != nil {
                // ... 原有代码 ...
            }
        }
    }
    
    // 无聚合ID：使用全局Worker池处理 (不变)
    // ...
}
```

#### 步骤 1.5: 添加 Close 方法支持

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**位置**: `Close()` 方法中

```go
func (k *kafkaEventBus) Close() error {
    // ... 现有关闭逻辑 ...
    
    // ⭐ 关闭 Keyed Worker Pool
    if k.globalKeyedPool != nil {
        k.globalKeyedPool.Stop()
    }
    
    // ⭐ 关闭 Hollywood Actor Pool
    if k.globalActorPool != nil {
        k.globalActorPool.Stop()
    }
    
    // ... 其他关闭逻辑 ...
}
```

---

### 阶段 2: 监控集成 (1 天)

#### 步骤 2.1: 实现 Prometheus 监控收集器

**文件**: `jxt-core/sdk/pkg/eventbus/kafka_actor_pool_metrics.go` (新建)

参考 `hollywood-actor-pool-prometheus-integration.md` 文档中的实现。

#### 步骤 2.2: 注册 Prometheus 指标

在 Kafka EventBus 初始化时注册指标：

```go
// 注册 Prometheus 指标
prometheus.MustRegister(
    metricsCollector.messagesSent,
    metricsCollector.messagesProcessed,
    metricsCollector.messageLatency,
    // ... 其他指标
)
```

---

### 阶段 3: 配置管理 (0.5 天)

#### 步骤 3.1: 添加配置字段

**文件**: `jxt-core/sdk/pkg/eventbus/config.go`

```go
type KafkaConfig struct {
    // ... 现有字段 ...
    
    // Hollywood Actor Pool 配置
    ActorPool ActorPoolConfig `json:"actor_pool" yaml:"actor_pool"`
}

type ActorPoolConfig struct {
    Enabled     bool `json:"enabled" yaml:"enabled"`           // 是否启用 Actor Pool
    PoolSize    int  `json:"pool_size" yaml:"pool_size"`       // Actor 数量 (默认 256)
    InboxSize   int  `json:"inbox_size" yaml:"inbox_size"`     // Inbox 大小 (默认 1000)
    MaxRestarts int  `json:"max_restarts" yaml:"max_restarts"` // 最大重启次数 (默认 3)
}
```

#### 步骤 3.2: 环境变量支持

```bash
# 功能开关
KAFKA_USE_ACTOR_POOL=true|false

# Actor Pool 配置
KAFKA_ACTOR_POOL_SIZE=256
KAFKA_ACTOR_INBOX_SIZE=1000
KAFKA_ACTOR_MAX_RESTARTS=3
```

---

### 阶段 4: 测试 (2-3 天)

#### 步骤 4.1: 单元测试

**文件**: `jxt-core/sdk/pkg/eventbus/kafka_actor_pool_test.go` (新建)

测试内容：
- ✅ Actor Pool 初始化
- ✅ 消息路由到正确的 Actor
- ✅ 聚合ID 顺序保证
- ✅ Supervisor 重启机制
- ✅ 功能开关切换

#### 步骤 4.2: 集成测试

**文件**: `jxt-core/tests/eventbus/kafka_actor_pool_integration_test.go` (新建)

测试场景：
- ✅ 端到端消息处理
- ✅ 高并发场景 (10K TPS)
- ✅ Actor panic 恢复
- ✅ Inbox 满载处理
- ✅ 性能对比 (Keyed Pool vs Actor Pool)

---

## 🚀 **灰度发布计划**

### 第 1 天: 开发环境验证

```bash
# 启用 Actor Pool
export KAFKA_USE_ACTOR_POOL=true

# 运行测试
go test -v ./sdk/pkg/eventbus/... -run TestKafkaActorPool
```

### 第 2-3 天: 测试环境灰度 (10%)

```yaml
# 配置 10% 流量使用 Actor Pool
kafka:
  actor_pool:
    enabled: true
    pool_size: 256
    inbox_size: 1000
```

**观测指标**:
- `kafka_eventbus_actor_pool_messages_sent_total`
- `kafka_eventbus_actor_pool_message_latency_seconds`
- `kafka_eventbus_actor_pool_actor_restarted_total`

### 第 4-5 天: 生产环境灰度 (50%)

**观测重点**:
- P99 延迟是否持平
- Actor 重启频率
- Inbox 利用率

### 第 6-7 天: 生产环境全量 (100%)

**成功标准**:
- ✅ P99 延迟 ≤ Keyed Pool 基线 + 5%
- ✅ Actor 重启率 < 1%
- ✅ 无消息丢失
- ✅ 无内存泄漏

---

## 🔙 **回滚方案**

### 快速回滚 (< 1 分钟)

```bash
# 方式 1: 环境变量
export KAFKA_USE_ACTOR_POOL=false

# 方式 2: 配置文件
kafka:
  actor_pool:
    enabled: false

# 重启服务
systemctl restart jxt-eventbus
```

### 回滚触发条件

| 指标 | 阈值 | 动作 |
|------|------|------|
| P99 延迟 | > 基线 + 20% | 立即回滚 |
| Actor 重启率 | > 5% | 立即回滚 |
| 错误率 | > 1% | 立即回滚 |
| 内存增长 | > 50% | 观察 30 分钟后回滚 |

---

## 📊 **监控 Dashboard**

### Grafana 面板配置

```promql
# 消息吞吐量
rate(kafka_eventbus_actor_pool_messages_sent_total[1m])

# P99 延迟
histogram_quantile(0.99, rate(kafka_eventbus_actor_pool_message_latency_seconds_bucket[1m]))

# Actor 重启率
rate(kafka_eventbus_actor_pool_actor_restarted_total[5m])

# Inbox 利用率
kafka_eventbus_actor_pool_inbox_utilization
```

---

## ✅ **验收标准**

### 功能验收

- ✅ 所有单元测试通过
- ✅ 所有集成测试通过
- ✅ 聚合ID 顺序保证正确
- ✅ Supervisor 重启机制正常
- ✅ 功能开关切换无问题

### 性能验收

- ✅ 吞吐量 ≥ Keyed Pool 基线 - 5%
- ✅ P99 延迟 ≤ Keyed Pool 基线 + 5%
- ✅ 内存占用 ≤ Keyed Pool 基线 + 10%

### 可靠性验收

- ✅ 7 天无 Actor 重启异常
- ✅ 7 天无消息丢失
- ✅ 7 天无内存泄漏

---

## 📝 **Kafka 特定注意事项**

### 1. Partition 和 Offset

Kafka 有 Partition 和 Offset 概念，需要在 `AggregateMessage` 中正确设置：

```go
aggMsg := &AggregateMessage{
    Topic:       message.Topic,
    Partition:   message.Partition,  // ⭐ Kafka 特有
    Offset:      message.Offset,     // ⭐ Kafka 特有
    // ...
}
```

### 2. ACK 机制

Kafka 使用 `session.MarkMessage()` 确认消息：

```go
// 处理成功后标记消息
session.MarkMessage(message, "")
```

### 3. 聚合ID 提取

Kafka 从多个来源提取聚合ID（优先级）：
1. Envelope 消息体
2. Kafka Headers
3. Kafka Key

```go
aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")
```

---

## 🎯 **下一步**

完成 Kafka EventBus 迁移后：
1. ✅ 收集性能数据和经验教训
2. ✅ 更新文档（实际性能数据）
3. ✅ 准备 NATS EventBus 迁移方案
4. ✅ 复用 Kafka 迁移经验

**预计总时间**: 8-13 天

**风险等级**: 🟡 中等 (有功能开关和回滚方案)

**成功概率**: 95% (架构相似，风险可控)

