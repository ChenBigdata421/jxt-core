# NATS EventBus 免锁优化方案

## 📋 文档信息

- **创建时间**: 2025-10-17
- **版本**: v1.0
- **状态**: 待实施
- **参考文档**: 
  - `lock-free-optimization-design.md` - Kafka 免锁优化设计
  - `lock-free-implementation-guide.md` - Kafka 免锁实施指南
  - `kafka.go` - Kafka EventBus 实现（已完成 P0 优化）

---

## 🎯 优化目标

基于 Kafka EventBus 的免锁优化经验，对 NATS EventBus 进行同等级别的性能优化，目标是：

1. **提升吞吐量**: 从 2861 msg/s 提升到 3500-4000 msg/s（**+22-40%**）
2. **降低延迟**: 从 32 ms 降低到 22-26 ms（**-20-30%**）
3. **减少锁竞争**: 从 30% 降低到 5-10%（**-70%**）
4. **提升 handler 查找性能**: 从 100 万次/秒提升到 300-500 万次/秒（**3-5x**）

---

## 🔥 核心优化原则

**高频路径免锁，低频路径保留锁**

### 高频路径（需要免锁优化）
- `Publish` / `PublishEnvelope` - 每秒数千-数万次
- `handleMessage` (handler查找) - 每条消息
- `processUnifiedPullMessages` (消息路由) - 每条消息
- `GetTopicConfig` - 每次发布/订阅

### 低频路径（保留锁，保持清晰）
- `Subscribe` / `SubscribeEnvelope` - 启动时一次
- `Close` - 关闭时一次
- `ConfigureTopic` - 初始化时

---

## 📊 当前 NATS EventBus 锁使用分析

### 问题点汇总

| 字段 | 当前类型 | 使用频率 | 问题 | 优化方案 | 优先级 |
|------|---------|---------|------|---------|--------|
| **`closed`** | `bool` + `mu.RWMutex` | 🔥 每次发布 | 每次发布都加读锁 | `atomic.Bool` | **P0** |
| **`topicHandlers`** | `map` + `topicHandlersMu.RWMutex` | 🔥 每条消息 | 每条消息都加读锁查找 | `sync.Map` | **P0** |
| **`conn`** | `*nats.Conn` + `mu.RWMutex` | 🔥 每次发布 | 每次发布都加读锁 | `atomic.Value` | **P0** |
| **`js`** | `nats.JetStreamContext` + `mu.RWMutex` | 🔥 每次发布 | 每次发布都加读锁 | `atomic.Value` | **P0** |
| **`topicConfigs`** | `map` + `topicConfigsMu.RWMutex` | 🔥 每次发布 | 每次发布都加读锁 | `sync.Map` | **P1** |
| **`createdStreams`** | `map` + `createdStreamsMu.RWMutex` | 🔥 每次发布 | 每次发布都加读锁 | `sync.Map` | **P1** |
| **`subscriptions`** | `map` + `mu.RWMutex` | ✅ 低频 | 订阅时加锁 | **保留锁** | N/A |
| **`subscriptionHandlers`** | `map` + `subscriptionsMu.RWMutex` | ✅ 低频 | 订阅时加锁 | **保留锁** | N/A |

---

## 🔧 详细优化方案

### 优化 1: `closed` 改为 `atomic.Bool` (🔥 P0)

#### 当前代码 (Line 210-211)
```go
mu     sync.RWMutex
closed bool
```

#### 优化后
```go
// ✅ 低频路径：保留 mu（用于 Subscribe、Close 等低频操作）
mu     sync.Mutex  // 🔥 改为 Mutex（不再需要读写锁）
closed atomic.Bool // 🔥 P0修复：改为 atomic.Bool，热路径无锁读取
```

#### 影响的方法
- `Publish` (Line 923-1035) - 🔥 高频路径
- `PublishEnvelope` - 🔥 高频路径
- `Subscribe` (Line 1060-1126) - ✅ 低频路径
- `Close` (Line 1424-1500) - ✅ 低频路径

#### 修改示例

**高频路径 - Publish**:
```go
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // 🔥 P0修复：无锁检查关闭状态（~5ns vs ~50ns）
    if n.closed.Load() {
        return fmt.Errorf("eventbus is closed")
    }
    
    // ... 其他逻辑
}
```

**低频路径 - Close**:
```go
func (n *natsEventBus) Close() error {
    n.mu.Lock() // 保留锁 - 关闭是低频操作
    defer n.mu.Unlock()
    
    // 🔥 P0修复：使用 atomic.Bool 设置关闭状态
    if n.closed.Load() {
        return nil
    }
    n.closed.Store(true)
    
    // ... 其他关闭逻辑
}
```

#### 性能收益
- **读取性能**: ~5ns (atomic) vs ~50ns (RWMutex)
- **吞吐量提升**: +5-10%
- **锁竞争减少**: -20%

---

### 优化 2: `topicHandlers` 改为 `sync.Map` (🔥 P0)

#### 当前代码 (Line 216-217)
```go
topicHandlers   map[string]MessageHandler // topic到handler的映射
topicHandlersMu sync.RWMutex              // topic handlers锁
```

#### 优化后
```go
// 🔥 P0修复：改为 sync.Map（消息路由时无锁查找）
topicHandlers sync.Map // key: string (topic), value: MessageHandler
```

#### 影响的方法
- `processUnifiedPullMessages` (Line 1204-1258) - 🔥 高频路径
- `handleMessage` (Line 1267-1376) - 🔥 高频路径
- `subscribeJetStream` (Line 1129-1202) - ✅ 低频路径

#### 修改示例

**高频路径 - processUnifiedPullMessages**:
```go
func (n *natsEventBus) processUnifiedPullMessages(ctx context.Context, topic string, sub *nats.Subscription) {
    for {
        msgs, err := sub.Fetch(batchSize, nats.MaxWait(fetchTimeout))
        if err != nil {
            // ... 错误处理
            continue
        }
        
        for _, msg := range msgs {
            // 🔥 P0修复：无锁读取 handler（使用 sync.Map）
            handlerAny, exists := n.topicHandlers.Load(topic)
            if !exists {
                n.logger.Warn("No handler found for topic", zap.String("topic", topic))
                msg.Ack()
                continue
            }
            handler := handlerAny.(MessageHandler)
            
            n.handleMessage(ctx, topic, msg.Data, handler, func() error {
                return msg.Ack()
            })
        }
    }
}
```

**低频路径 - subscribeJetStream**:
```go
func (n *natsEventBus) subscribeJetStream(ctx context.Context, topic string, handler MessageHandler) error {
    // 🔥 P0修复：使用 sync.Map 存储 handler
    n.topicHandlers.Store(topic, handler)
    
    // ... 其他订阅逻辑
}
```

#### 性能收益
- **查找性能**: ~5ns (sync.Map) vs ~20ns (RWMutex)
- **handler 查找吞吐量**: 300-500 万次/秒 vs 100 万次/秒（**3-5x**）
- **消息处理延迟**: -10-15%

---

### 优化 3: `conn` 和 `js` 改为 `atomic.Value` (🔥 P0)

#### 当前代码 (Line 205-206)
```go
conn *nats.Conn
js   nats.JetStreamContext
```

#### 优化后
```go
// 🔥 P0修复：改为 atomic.Value（发布时无锁读取）
conn atomic.Value // stores *nats.Conn
js   atomic.Value // stores nats.JetStreamContext
```

#### 影响的方法
- `Publish` (Line 923-1035) - 🔥 高频路径
- `PublishEnvelope` - 🔥 高频路径
- `NewNATSEventBus` (Line 300-444) - ✅ 低频路径
- `reconnect` - ✅ 低频路径

#### 修改示例

**低频路径 - NewNATSEventBus**:
```go
func NewNATSEventBus(config *NATSConfig) (EventBus, error) {
    // ... 连接初始化
    
    bus := &natsEventBus{
        config: config,
        // ...
    }
    
    // 🔥 P0修复：使用 atomic.Value 存储
    bus.conn.Store(nc)
    bus.js.Store(js)
    bus.closed.Store(false)
    
    // ...
}
```

**高频路径 - Publish**:
```go
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // 🔥 P0修复：无锁检查关闭状态
    if n.closed.Load() {
        return fmt.Errorf("eventbus is closed")
    }
    
    // 🔥 P0修复：无锁读取 JetStream Context
    js, err := n.getJetStreamContext()
    if err != nil {
        return err
    }
    
    // ... 发布逻辑
}
```

**Helper 方法**:
```go
// Helper 方法：无锁读取 JetStream Context
func (n *natsEventBus) getJetStreamContext() (nats.JetStreamContext, error) {
    jsAny := n.js.Load()
    if jsAny == nil {
        return nil, fmt.Errorf("jetstream context not initialized")
    }
    js, ok := jsAny.(nats.JetStreamContext)
    if !ok {
        return nil, fmt.Errorf("invalid jetstream context type")
    }
    return js, nil
}

// Helper 方法：无锁读取 NATS Connection
func (n *natsEventBus) getConn() (*nats.Conn, error) {
    connAny := n.conn.Load()
    if connAny == nil {
        return nil, fmt.Errorf("nats connection not initialized")
    }
    conn, ok := connAny.(*nats.Conn)
    if !ok {
        return nil, fmt.Errorf("invalid nats connection type")
    }
    return conn, nil
}
```

#### 性能收益
- **读取性能**: ~5-10ns (atomic.Value) vs ~50ns (RWMutex)
- **发布吞吐量**: +10-15%
- **CPU 使用率**: -10-15%

---

### 优化 4: `topicConfigs` 改为 `sync.Map` (🔥 P1)

#### 当前代码 (Line 260-261)
```go
topicConfigs   map[string]TopicOptions
topicConfigsMu sync.RWMutex
```

#### 优化后
```go
// 🔥 P1优化：改为 sync.Map（发布时无锁读取配置）
topicConfigs sync.Map // key: string (topic), value: TopicOptions
```

#### 修改示例

**高频路径 - GetTopicConfig**:
```go
func (n *natsEventBus) GetTopicConfig(topic string) (TopicOptions, error) {
    // 🔥 P1优化：无锁读取
    configAny, exists := n.topicConfigs.Load(topic)
    if !exists {
        return TopicOptions{}, fmt.Errorf("topic config not found: %s", topic)
    }
    return configAny.(TopicOptions), nil
}
```

**低频路径 - ConfigureTopic**:
```go
func (n *natsEventBus) ConfigureTopic(ctx context.Context, topic string, options TopicOptions) error {
    n.mu.Lock() // 保留锁 - 配置是低频操作
    defer n.mu.Unlock()
    
    if n.closed.Load() {
        return fmt.Errorf("eventbus is closed")
    }
    
    // 🔥 P1优化：使用 sync.Map 存储配置
    n.topicConfigs.Store(topic, options)
    
    // ... 其他配置逻辑
}
```

---

### 优化 5: `createdStreams` 改为 `sync.Map` + 单飞抑制 (🔥 P1)

#### 当前代码 (Line 266-267)
```go
createdStreams   map[string]bool // streamName -> true
createdStreamsMu sync.RWMutex
```

#### 优化后
```go
// 🔥 P1优化：改为 sync.Map（发布时无锁读取）
createdStreams sync.Map // key: string (streamName), value: bool

// 🔥 P1优化：单飞抑制（避免并发创建风暴）
streamCreateSF singleflight.Group
```

#### 问题分析
- **并发创建风暴**：多个 goroutine 同时发现 stream 不存在，并发调用 `ensureTopicInJetStream`
- **资源浪费**：重复的 RPC 调用和 Stream 创建尝试
- **潜在错误**：并发创建可能导致"already exists"错误

#### 修改示例

**高频路径 - ensureStream（新增方法）**:
```go
// ensureStream 确保 Stream 存在（带单飞抑制）
func (n *natsEventBus) ensureStream(topic string, cfg TopicOptions) error {
    streamName := n.getStreamNameForTopic(topic)

    // 🔥 P1优化：快速路径 - 无锁检查本地缓存
    if _, ok := n.createdStreams.Load(streamName); ok {
        return nil
    }

    // 🔥 P1优化：单飞抑制 - 同一 stream 只创建一次
    _, err, _ := n.streamCreateSF.Do(streamName, func() (any, error) {
        // Double-check：可能其他 goroutine 已经创建成功
        if _, ok := n.createdStreams.Load(streamName); ok {
            return nil, nil
        }

        // 实际创建 Stream
        if err := n.ensureTopicInJetStream(topic, cfg); err != nil {
            return nil, err
        }

        // 🔥 成功后添加到本地缓存
        n.createdStreams.Store(streamName, true)
        return nil, nil
    })

    return err
}
```

**高频路径 - Publish**:
```go
func (n *natsEventBus) Publish(ctx context.Context, topic string, message []byte) error {
    // ...

    // 🔥 P1优化：使用单飞抑制的 ensureStream
    if err := n.ensureStream(topic, topicConfig); err != nil {
        n.logger.Warn("Failed to ensure stream, falling back to Core NATS",
            zap.String("topic", topic),
            zap.Error(err))
        // 降级到 Core NATS
    }

    // ...
}
```

#### 性能收益
- **避免并发创建风暴**：N 个并发请求 → 1 个实际创建
- **减少 RPC 调用**：从 N 次降低到 1 次
- **提升吞吐量**：+5-10%（高并发场景）

---

## 📦 完整的优化后结构体定义

```go
// natsEventBus NATS JetStream事件总线实现
// 🔥 免锁优化版本 - 参考 Kafka EventBus 优化方案
type natsEventBus struct {
    // ✅ 低频路径：保留 mu（用于 Subscribe、Close 等低频操作）
    mu     sync.Mutex  // 🔥 改为 Mutex（不再需要读写锁）
    closed atomic.Bool // 🔥 P0修复：改为 atomic.Bool，热路径无锁读取

    // 🔥 高频路径：改为 atomic.Value（发布时无锁读取）
    conn atomic.Value // stores *nats.Conn
    js   atomic.Value // stores nats.JetStreamContext

    config *NATSConfig // 使用内部配置结构，实现解耦

    // ✅ 低频路径：保留 map + mu（订阅是低频操作）
    subscriptions map[string]*nats.Subscription

    logger             *zap.Logger
    reconnectCallbacks []func(ctx context.Context) error

    // 🔥 统一Consumer管理 - 优化架构
    unifiedConsumer nats.ConsumerInfo // 单一Consumer

    // 🔥 P0修复：改为 sync.Map（消息路由时无锁查找）
    topicHandlers sync.Map // key: string (topic), value: MessageHandler

    // ✅ 低频路径：保留 slice + mu（订阅是低频操作）
    subscribedTopics   []string
    subscribedTopicsMu sync.Mutex

    // 企业级特性（已优化）
    publishedMessages atomic.Int64
    consumedMessages  atomic.Int64
    errorCount        atomic.Int64
    lastHealthCheck   atomic.Value // time.Time
    healthStatus      atomic.Bool

    // 增强的企业级特性
    metricsCollector *time.Ticker
    metrics          *Metrics
    messageFormatter MessageFormatter
    publishCallback  PublishCallback
    errorHandler     ErrorHandler
    messageRouter    MessageRouter

    // 健康检查控制
    healthCheckCancel context.CancelFunc
    healthCheckDone   chan struct{}

    // 自动重连控制
    reconnectConfig   ReconnectConfig
    failureCount      atomic.Int32
    lastReconnectTime atomic.Value // time.Time
    reconnectCallback ReconnectCallback

    // ✅ 低频路径：保留 map + mu（订阅管理是低频操作）
    subscriptionHandlers map[string]MessageHandler // topic -> handler
    subscriptionsMu      sync.Mutex

    // 积压检测器
    backlogDetector          *NATSBacklogDetector      // 订阅端积压检测器
    publisherBacklogDetector *PublisherBacklogDetector // 发送端积压检测器

    // 全局 Keyed-Worker Pool（所有 topic 共享，与 Kafka 保持一致）
    globalKeyedPool *KeyedWorkerPool

    // 🔥 P1优化：改为 sync.Map（发布时无锁读取配置）
    topicConfigs          sync.Map // key: string (topic), value: TopicOptions
    topicConfigStrategy   TopicConfigStrategy       // 配置策略
    topicConfigOnMismatch TopicConfigMismatchAction // 配置不一致时的行为

    // 🔥 P1优化：改为 sync.Map（发布时无锁读取）
    createdStreams sync.Map // key: string (streamName), value: bool

    // 🔥 P1优化：单飞抑制（避免并发创建风暴）
    streamCreateSF singleflight.Group

    // 健康检查订阅监控器
    healthCheckSubscriber *HealthCheckSubscriber
    // 健康检查发布器
    healthChecker *HealthChecker
    // 健康检查配置（从 Enterprise.HealthCheck 转换而来）
    healthCheckConfig config.HealthCheckConfig

    // 异步发布结果通道（用于Outbox模式）
    publishResultChan chan *PublishResult
    // 异步发布结果处理控制
    publishResultWg     sync.WaitGroup
    publishResultCancel context.CancelFunc
    // 是否启用发布结果通道（性能优化：默认禁用）
    enablePublishResult bool

    // ✅ 方案2：共享 ACK 处理器（避免 per-message goroutine）
    ackChan        chan *ackTask  // ACK 任务通道
    ackWorkerWg    sync.WaitGroup // ACK worker 等待组
    ackWorkerStop  chan struct{}  // ACK worker 停止信号
    ackWorkerCount int            // ACK worker 数量（可配置）
}
```

---

## 📈 性能收益预估

| 场景 | 当前性能 | 优化后性能 | 提升幅度 |
|------|---------|-----------|---------|
| **handler查找** (热路径) | 100 万次/秒 | 300-500 万次/秒 | **3-5x** |
| **发布吞吐量** | 2861 msg/s | 3500-4000 msg/s | **+22-40%** |
| **P99 延迟** | 32 ms | 22-26 ms | **-20-30%** |
| **CPU 使用率** | 60% | 45-50% | **-15-25%** |
| **锁竞争时间** | 30% | 5-10% | **-70%** |

---

## 🎯 实施优先级

### P0 修复（立即实施 - 核心热路径）
1. ✅ `closed` 改为 `atomic.Bool`
2. ✅ `topicHandlers` 改为 `sync.Map`
3. ✅ `conn` 和 `js` 改为 `atomic.Value`
4. ✅ 添加 Helper 方法（`getConn`, `getJetStreamContext`）

**预期收益**: 吞吐量 +15-25%，延迟 -15-20%

### P1 优化（高优先级 - 次要热路径）
5. ✅ `topicConfigs` 改为 `sync.Map`
6. ✅ `createdStreams` 改为 `sync.Map` + 单飞抑制（`singleflight.Group`）
7. ✅ 验证 `ackWorkerCount` 配置是否合理（确保 Ack 延迟 < AckWait）

**预期收益**: 吞吐量 +5-10%，延迟 -5-10%，避免并发创建风暴

### P2 优化（可选 - 代码质量）
7. ✅ 更新文档和注释
8. ✅ 添加性能基准测试
9. ✅ 添加并发安全测试

---

## ⚠️ 风险评估

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| **sync.Map 性能退化** | 中 | 低 | 写多场景回退到 RWMutex |
| **atomic.Value 类型断言失败** | 高 | 低 | 严格类型检查 + panic recovery |
| **并发竞态条件** | 高 | 中 | 充分的并发测试 + race detector |
| **协程泄漏** | 中 | 低 | 定期清理 + 监控 |
| **重连时的并发安全** | 中 | 低 | NATS 客户端已有透明重连机制，逐个替换 atomic.Value 即可 |

### 重连一致性说明

**NATS 客户端的透明重连机制**：
- NATS Go 客户端内置了自动重连功能（`SetReconnectHandler`）
- `conn.IsConnected()` 和 `conn.IsReconnecting()` 提供状态检查
- `js` 是从 `conn` 派生的，只要 `conn` 有效，`js` 就有效

**重连流程**：
```go
func (n *natsEventBus) reinitializeConnectionInternal() error {
    n.mu.Lock()
    defer n.mu.Unlock()

    // 1. 创建新连接
    nc, err := nats.Connect(...)
    if err != nil {
        return err
    }

    // 2. 创建新 JetStream Context
    js, err := nc.JetStream()
    if err != nil {
        nc.Close()
        return err
    }

    // 3. 原子替换（顺序无关紧要，因为都是原子操作）
    n.conn.Store(nc)
    n.js.Store(js)

    return nil
}
```

**为什么不需要"统一持锁更新"**：
- `atomic.Value.Store` 是原子操作，任何读取都能看到"旧对象"或"新对象"
- 不会出现"部分新、部分旧"的情况
- 即使读取到旧对象，也是完整可用的（只是会在下次读取时看到新对象）

---

## 🧪 测试验证策略

### 1. 单元测试
- 测试 `atomic.Bool` 的并发读写
- 测试 `sync.Map` 的并发读写
- 测试 `atomic.Value` 的类型安全

### 2. 并发测试
- 使用 `go test -race` 检测数据竞态
- 使用 `go test -count=100` 重复测试
- 使用 `pprof` 检测协程泄漏

### 3. 性能测试
- 运行 `kafka_nats_envelope_comparison_test.go`
- 运行 `memory_persistence_comparison_test.go`
- 对比优化前后的性能指标

### 4. 压力测试
- 极限压力测试（10000 msg/s）
- 长时间运行测试（24 小时）
- 监控协程数、内存使用、CPU 使用

---

## 📝 实施步骤

### 阶段 1: 文档更新
1. 更新 `lock-free-optimization-design.md`（添加 NATS 章节）
2. 更新 `lock-free-implementation-guide.md`（添加 NATS 实施指南）

### 阶段 2: 代码修改
1. 修改 `natsEventBus` 结构体定义
2. 修改 `NewNATSEventBus` 初始化逻辑
3. 修改 `Publish` / `PublishEnvelope` 方法
4. 修改 `processUnifiedPullMessages` 方法
5. 修改 `Close` 方法
6. 添加 Helper 方法

### 阶段 3: 测试验证
1. 运行单元测试
2. 运行并发测试（`go test -race`）
3. 运行性能测试
4. 对比性能指标

### 阶段 4: 代码审查
1. 代码审查
2. 性能审查
3. 安全审查

---

## 🎉 预期成果

优化完成后，NATS EventBus 将达到与 Kafka EventBus 同等的性能水平：

1. ✅ **高吞吐量**: 3500-4000 msg/s
2. ✅ **低延迟**: 22-26 ms (P99)
3. ✅ **低锁竞争**: 5-10%
4. ✅ **高并发安全**: 无数据竞态、无协程泄漏
5. ✅ **企业级质量**: 生产环境可用

---

## 📚 参考资料

1. **Kafka 免锁优化设计**: `lock-free-optimization-design.md`
2. **Kafka 免锁实施指南**: `lock-free-implementation-guide.md`
3. **Kafka EventBus 实现**: `kafka.go`
4. **Go 并发模式**: https://go.dev/blog/pipelines
5. **sync.Map 最佳实践**: https://pkg.go.dev/sync#Map

---

**文档结束**

