# EventBus 免锁优化实施指南

**文档版本**: v1.0  
**创建日期**: 2025-09-30  
**状态**: 实施参考

---

## 📋 目录

1. [实施前准备](#1-实施前准备)
2. [详细代码改动](#2-详细代码改动)
3. [迁移步骤](#3-迁移步骤)
4. [测试验证](#4-测试验证)
5. [性能基准测试](#5-性能基准测试)
6. [常见问题](#6-常见问题)

---

## 1. 实施前准备

### 1.1 依赖检查

```bash
# 确保 Go 版本 >= 1.19（支持 atomic 包的泛型）
go version

# 安装 LRU 缓存库
go get github.com/hashicorp/golang-lru/v2

# 安装限流器
go get golang.org/x/time/rate
```

### 1.2 备份当前代码

```bash
# 创建分支
git checkout -b feature/lock-free-optimization

# 备份关键文件
cp jxt-core/sdk/pkg/eventbus/kafka.go jxt-core/sdk/pkg/eventbus/kafka.go.backup
cp jxt-core/sdk/pkg/eventbus/nats.go jxt-core/sdk/pkg/eventbus/nats.go.backup
```

---

## 2. 详细代码改动

### 2.1 优化原则 ⭐

**核心原则**: **只优化高频路径，低频路径保留锁**

#### 🔥 高频路径（需要优化）
- `Publish` / `PublishEnvelope` - 每秒数千-数万次
- `ConsumeClaim` (handler查找) - 每条消息
- `GetTopicConfig` - 每次发布/订阅

#### ✅ 低频路径（保留锁）
- `Subscribe` / `SubscribeEnvelope` - 启动时一次
- `Close` - 关闭时一次
- `ConfigureTopic` - 初始化时

### 2.2 Kafka EventBus 结构体改动

#### 改动 1: 高频路径 - 订阅映射查找

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**当前代码** (约 L50-L60):
```go
type kafkaEventBus struct {
    config *config.KafkaConfig

    // 🔴 低频路径：保留 mu（用于 Subscribe、Close 等低频操作）
    mu            sync.Mutex

    producer      sarama.SyncProducer
    consumerGroup sarama.ConsumerGroup
    admin         sarama.ClusterAdmin
    client        sarama.Client

    // 🔥 高频路径：需要优化
    subscriptionsMu sync.Mutex
    subscriptions   map[string]MessageHandler

    topicConfigMu sync.RWMutex
    topicConfigs  map[string]TopicOptions

    // ...
}
```

**修改为**:
```go
type kafkaEventBus struct {
    config *config.KafkaConfig

    // ✅ 低频路径：保留 mu（用于 Subscribe、Close 等低频操作）
    mu     sync.Mutex
    closed atomic.Bool // 🔥 P0修复：改为 atomic.Bool，热路径无锁读取

    // 🔥 高频路径：改为 atomic.Value（发布时无锁读取）
    asyncProducer        atomic.Value // stores sarama.AsyncProducer
    consumer             atomic.Value // stores sarama.Consumer
    client               atomic.Value // stores sarama.Client
    admin                atomic.Value // stores sarama.ClusterAdmin
    unifiedConsumerGroup atomic.Value // stores sarama.ConsumerGroup

    // 🔥 高频路径：改为 sync.Map（消息处理时无锁查找）
    subscriptions sync.Map // key: string (topic), value: MessageHandler

    // 🔥 高频路径：改为 sync.Map（发布时无锁读取配置）
    topicConfigs sync.Map // key: string (topic), value: TopicOptions

    // 🔥 P0修复：预订阅模式 - 使用 atomic.Value 存储不可变切片快照
    allPossibleTopicsMu sync.Mutex   // 保护写入
    allPossibleTopics   []string     // 主副本（仅在持有锁时修改）
    topicsSnapshot      atomic.Value // 只读快照（[]string），消费goroutine无锁读取

    // 🔥 高频路径：改为 sync.Map（消息路由时无锁查找）
    activeTopicHandlers sync.Map // key: string (topic), value: MessageHandler

    // ✅ 已优化：原子操作
    isConnected  atomic.Bool
    publishCount atomic.Int64
    consumeCount atomic.Int64
    errorCount   atomic.Int64

    globalWorkerPool *GlobalWorkerPool
    ctx              context.Context
    cancel           context.CancelFunc
    wg               sync.WaitGroup
}
```

**关键变化**:
- 🔥 **P0修复1**: `closed` 改为 `atomic.Bool`，热路径（Publish/PublishEnvelope）无锁读取
- 🔥 **P0修复2**: `allPossibleTopics` 使用 `atomic.Value` 存储不可变快照，消费goroutine无锁读取，消除数据竞态
- 🔥 **P0修复3**: `activeTopicHandlers` 改为 `sync.Map`，消息路由时无锁查找
- 🔥 **高频路径**: `subscriptions`、`topicConfigs`、`asyncProducer` 等改为无锁数据结构
- ✅ **低频路径**: `mu` 保留，用于 `Subscribe`、`Close` 等低频操作
- ✅ **代码清晰**: 低频操作仍使用锁，业务逻辑清晰易懂

---

### 2.3 初始化函数改动

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**当前代码** (约 L150-L200):
```go
func NewKafkaEventBus(cfg *config.KafkaConfig) (EventBus, error) {
    // ...

    bus := &kafkaEventBus{
        config:        cfg,
        subscriptions: make(map[string]MessageHandler),
        keyedPools:    make(map[string]*KeyedWorkerPool),
        topicConfigs:  make(map[string]TopicOptions),
    }

    // 初始化生产者
    bus.producer = producer
    bus.consumerGroup = consumerGroup
    bus.admin = admin
    bus.client = client

    // ...
}
```

**修改为**:
```go
func NewKafkaEventBus(cfg *config.KafkaConfig) (EventBus, error) {
    // ...

    bus := &kafkaEventBus{
        config: cfg,
        // subscriptions, topicConfigs, activeTopicHandlers 不需要初始化（sync.Map 零值可用）

        // 🔥 P0修复：初始化预订阅快照
        allPossibleTopics: make([]string, 0),
    }

    // 🔥 P0修复：初始化 closed 状态
    bus.closed.Store(false)

    // 🔥 P0修复：初始化 topicsSnapshot
    bus.topicsSnapshot.Store([]string{})

    // ✅ 使用 atomic.Value 存储
    bus.asyncProducer.Store(asyncProducer)
    bus.consumer.Store(consumer)
    bus.admin.Store(admin)
    bus.client.Store(client)
    bus.unifiedConsumerGroup.Store(unifiedConsumerGroup)

    // ...
}
```

### 2.4 订阅函数改动（✅ 低频路径 - 保留锁）

**当前代码** (约 L450-L480):
```go
func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    k.subscriptionsMu.Lock()
    defer k.subscriptionsMu.Unlock()

    if _, exists := k.subscriptions[topic]; exists {
        return fmt.Errorf("already subscribed to topic: %s", topic)
    }

    k.subscriptions[topic] = handler

    // ...
}
```

**修改为**:
```go
func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    k.mu.Lock() // ✅ 保留锁 - 订阅是低频操作
    defer k.mu.Unlock()

    // 🔥 P0修复：使用 atomic.Bool 读取关闭状态
    if k.closed.Load() {
        return fmt.Errorf("eventbus is closed")
    }

    // ✅ 使用 LoadOrStore 原子性检查并存储
    if _, loaded := k.subscriptions.LoadOrStore(topic, handler); loaded {
        return fmt.Errorf("already subscribed to topic: %s", topic)
    }

    // 🔥 P0修复：更新预订阅快照（如果使用预订阅模式）
    k.addTopicToPreSubscription(topic)

    // ... 其他订阅逻辑（保持不变）
}

// 🔥 P0修复：添加topic到预订阅列表（内部方法）
func (k *kafkaEventBus) addTopicToPreSubscription(topic string) {
    k.allPossibleTopicsMu.Lock()
    defer k.allPossibleTopicsMu.Unlock()

    // 检查重复
    for _, existingTopic := range k.allPossibleTopics {
        if existingTopic == topic {
            return
        }
    }

    // 创建新的不可变副本
    newTopics := make([]string, len(k.allPossibleTopics)+1)
    copy(newTopics, k.allPossibleTopics)
    newTopics[len(k.allPossibleTopics)] = topic

    // 更新主副本和快照
    k.allPossibleTopics = newTopics
    k.topicsSnapshot.Store(newTopics)
}
```

### 2.5 发布函数改动（🔥 高频路径 - 免锁）

**当前代码** (约 L400-L450):
```go
func (k *kafkaEventBus) Publish(ctx context.Context, topic string, data []byte) error {
    k.mu.Lock()
    defer k.mu.Unlock()
    
    if k.isClosed.Load() {
        return fmt.Errorf("kafka eventbus is closed")
    }
    
    if k.producer == nil {
        return fmt.Errorf("kafka producer not initialized")
    }
    
    // ...
    _, _, err := k.producer.SendMessage(msg)
    // ...
}
```

**修改为**:
```go
func (k *kafkaEventBus) Publish(ctx context.Context, topic string, data []byte) error {
    // 🔥 P0修复：无锁检查关闭状态
    if k.closed.Load() {
        return fmt.Errorf("kafka eventbus is closed")
    }

    // ✅ 无锁读取生产者（使用 helper 方法）
    producer, err := k.getAsyncProducer()
    if err != nil {
        return fmt.Errorf("failed to get async producer: %w", err)
    }

    // ...
    // 发送到 AsyncProducer 的 Input channel
    select {
    case producer.Input() <- msg:
        return nil
    case <-time.After(100 * time.Millisecond):
        // 应用背压
        producer.Input() <- msg
        return nil
    }
}

// Helper 方法：无锁读取 AsyncProducer
func (k *kafkaEventBus) getAsyncProducer() (sarama.AsyncProducer, error) {
    producerAny := k.asyncProducer.Load()
    if producerAny == nil {
        return nil, fmt.Errorf("async producer not initialized")
    }
    producer, ok := producerAny.(sarama.AsyncProducer)
    if !ok {
        return nil, fmt.Errorf("invalid async producer type")
    }
    return producer, nil
}
```

### 2.5 消费函数改动

**当前代码** (约 L550-L600):
```go
func (k *kafkaEventBus) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for message := range claim.Messages() {
        k.subscriptionsMu.Lock()
        handler, exists := k.subscriptions[message.Topic]
        k.subscriptionsMu.Unlock()
        
        if !exists {
            session.MarkMessage(message, "")
            continue
        }
        
        // 处理消息
        if err := handler(session.Context(), message.Value); err != nil {
            logger.Error("Failed to handle message", zap.Error(err))
        }
        
        session.MarkMessage(message, "")
    }
    return nil
}
```

**修改为**:
```go
// 🔥 预订阅模式的消费处理器
type preSubscriptionConsumerHandler struct {
    eventBus *kafkaEventBus
}

func (h *preSubscriptionConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
    for {
        select {
        case message := <-claim.Messages():
            if message == nil {
                return nil
            }

            // 🔥 P0修复：无锁读取 handler（使用 sync.Map）
            handlerAny, exists := h.eventBus.activeTopicHandlers.Load(message.Topic)
            if !exists {
                // 未激活的 topic，跳过
                session.MarkMessage(message, "")
                continue
            }
            handler := handlerAny.(MessageHandler)

            // 处理消息
            if err := h.eventBus.processMessage(session.Context(), message, handler); err != nil {
                h.eventBus.logger.Error("Failed to process message",
                    zap.String("topic", message.Topic),
                    zap.Error(err))
            } else {
                h.eventBus.consumedMessages.Add(1)
                session.MarkMessage(message, "")
            }

        case <-session.Context().Done():
            return nil
        }
    }
}
```

// 🔥 P0修复：预订阅消费者启动（无锁读取 topicsSnapshot）
func (k *kafkaEventBus) startPreSubscriptionConsumer(ctx context.Context) error {
    k.consumerMu.Lock()
    defer k.consumerMu.Unlock()

    if k.consumerStarted {
        return nil
    }

    consumerGroup, err := k.getUnifiedConsumerGroup()
    if err != nil {
        return fmt.Errorf("failed to get unified consumer group: %w", err)
    }

    handler := &preSubscriptionConsumerHandler{eventBus: k}

    go func() {
        defer close(k.consumerDone)

        for {
            if k.consumerCtx.Err() != nil {
                return
            }

            // 🔥 P0修复：无锁读取 topicsSnapshot
            topics := k.topicsSnapshot.Load().([]string)

            if len(topics) == 0 {
                <-k.consumerCtx.Done()
                return
            }

            // ✅ 安全：topics 是不可变副本，不会被修改
            err = consumerGroup.Consume(k.consumerCtx, topics, handler)
            if err != nil {
                if k.consumerCtx.Err() != nil {
                    return
                }
                k.logger.Error("Pre-subscription consumer error, will retry", zap.Error(err))
                time.Sleep(2 * time.Second)
                continue
            }

            k.logger.Info("Pre-subscription consumer session ended, restarting...")
        }
    }()

    k.consumerStarted = true
    return nil
}
    }
    
    select {
    case processor.messages <- aggMsg:
        // Message sent successfully
    case <-time.After(5 * time.Second):
        logger.Warn("Timeout sending message to processor", zap.String("aggregateID", aggregateID))
    case <-ctx.Done():
        logger.Warn("Context cancelled", zap.String("aggregateID", aggregateID))
    }
}
```

### 2.6 恢复模式相关函数

**新增函数**:
```go
// SetRecoveryMode 设置恢复模式
func (k *kafkaEventBus) SetRecoveryMode(isRecovery bool) {
    k.isRecoveryMode.Store(isRecovery)
    if isRecovery {
        logger.Info("Entering recovery mode: messages will be processed in order by aggregate ID")
    } else {
        logger.Info("Exiting recovery mode: gradually transitioning to normal processing")
    }
}

// IsInRecoveryMode 检查是否在恢复模式
func (k *kafkaEventBus) IsInRecoveryMode() bool {
    return k.isRecoveryMode.Load()
}

// getOrCreateAggregateProcessor 获取或创建聚合处理器
func (k *kafkaEventBus) getOrCreateAggregateProcessor(ctx context.Context, aggregateID string, handler MessageHandler) (*aggregateProcessor, error) {
    // 从缓存中获取
    if proc, exists := k.aggregateProcessors.Get(aggregateID); exists {
        return proc, nil
    }
    
    // 只在恢复模式下创建新的处理器
    if !k.IsInRecoveryMode() {
        return nil, fmt.Errorf("not in recovery mode, cannot create new processor")
    }
    
    // 创建新的处理器
    proc := &aggregateProcessor{
        aggregateID: aggregateID,
        messages:    make(chan *AggregateMessage, 100),
        done:        make(chan struct{}),
    }
    proc.lastActivity.Store(time.Now())
    
    // 添加到缓存
    k.aggregateProcessors.Add(aggregateID, proc)
    
    // 启动处理器
    k.wg.Add(1)
    go k.runAggregateProcessor(ctx, proc, handler)
    
    return proc, nil
}

// runAggregateProcessor 运行聚合处理器
func (k *kafkaEventBus) runAggregateProcessor(ctx context.Context, proc *aggregateProcessor, handler MessageHandler) {
    defer k.wg.Done()
    defer k.releaseAggregateProcessor(proc)
    
    idleTimeout := 5 * time.Minute
    
    for {
        select {
        case msg, ok := <-proc.messages:
            if !ok {
                return
            }
            
            // 处理消息
            if err := handler(msg.Context, msg.Value); err != nil {
                logger.Error("Failed to handle message in processor", 
                    zap.String("aggregateID", proc.aggregateID),
                    zap.Error(err))
                k.errorCount.Add(1)
            } else {
                k.consumeCount.Add(1)
            }
            
            proc.lastActivity.Store(time.Now())
            
        case <-time.After(idleTimeout):
            lastActivity := proc.lastActivity.Load().(time.Time)
            if time.Since(lastActivity) > idleTimeout {
                logger.Info("Processor idle, shutting down", 
                    zap.String("aggregateID", proc.aggregateID))
                return
            }
            
        case <-proc.done:
            return
            
        case <-ctx.Done():
            return
        }
    }
}

// releaseAggregateProcessor 释放聚合处理器
func (k *kafkaEventBus) releaseAggregateProcessor(proc *aggregateProcessor) {
    k.aggregateProcessors.Remove(proc.aggregateID)
    close(proc.messages)
    close(proc.done)
}

// extractAggregateID 从消息中提取聚合ID
func extractAggregateID(msg *sarama.ConsumerMessage) string {
    // 从消息头中提取
    for _, header := range msg.Headers {
        if string(header.Key) == "aggregateID" {
            return string(header.Value)
        }
    }
    
    // 或者从消息键中提取
    if len(msg.Key) > 0 {
        return string(msg.Key)
    }
    
    return ""
}
```

---

## 3. 迁移步骤

### 步骤 1: 更新结构体定义

1. 修改 `kafkaEventBus` 结构体
2. 添加 `aggregateProcessor` 结构体
3. 添加 `AggregateMessage` 结构体

### 步骤 2: 更新初始化函数

1. 修改 `NewKafkaEventBus` 函数
2. 使用 `atomic.Value.Store()` 初始化对象
3. 初始化 LRU 缓存和限流器

### 步骤 3: 更新订阅/发布函数

1. 修改 `Subscribe` 函数使用 `sync.Map`
2. 修改 `Publish` 函数使用 `atomic.Value`
3. 修改 `ConsumeClaim` 函数使用 `sync.Map`

### 步骤 4: 添加恢复模式支持

1. 添加 `SetRecoveryMode` 函数
2. 添加 `IsInRecoveryMode` 函数
3. 添加 `processMessage` 函数
4. 添加聚合处理器相关函数

### 步骤 5: 更新其他函数

1. 修改 `Close` 函数
2. 修改 `GetTopicConfig` 函数
3. 修改 `ConfigureTopic` 函数

---

## 4. 测试验证

### 4.1 单元测试

```bash
# 运行所有测试
cd jxt-core/sdk/pkg/eventbus
go test -v -timeout=2m

# 运行并发测试
go test -v -race -timeout=2m

# 运行特定测试
go test -v -run="TestKafka.*" -timeout=2m
```

### 4.2 集成测试

```bash
# 运行集成测试
go test -v -run=".*Integration" -timeout=5m

# 运行 Kafka 集成测试
go test -v -run="TestKafka.*Integration" -timeout=5m
```

### 4.3 并发安全测试

```bash
# 使用 race detector
go test -race -v -timeout=5m

# 压力测试
go test -v -run="TestKafka.*Concurrent" -timeout=10m
```

---

## 5. 性能基准测试

### 5.1 基准测试代码

创建文件 `jxt-core/sdk/pkg/eventbus/kafka_benchmark_test.go`:

```go
package eventbus

import (
    "context"
    "testing"
)

func BenchmarkSubscriptionLookup(b *testing.B) {
    bus := setupKafkaBus(b)
    defer bus.Close()
    
    topic := "test-topic"
    handler := func(ctx context.Context, data []byte) error {
        return nil
    }
    
    bus.Subscribe(context.Background(), topic, handler)
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            // 模拟订阅查找
            handlerAny, _ := bus.(*kafkaEventBus).subscriptions.Load(topic)
            _ = handlerAny.(MessageHandler)
        }
    })
}

func BenchmarkPublish(b *testing.B) {
    bus := setupKafkaBus(b)
    defer bus.Close()
    
    topic := "test-topic"
    data := []byte("test message")
    ctx := context.Background()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            bus.Publish(ctx, topic, data)
        }
    })
}
```

### 5.2 运行基准测试

```bash
# 运行基准测试
go test -bench=. -benchmem -benchtime=10s

# 对比优化前后
go test -bench=BenchmarkSubscriptionLookup -benchmem -count=5 > old.txt
# 应用优化
go test -bench=BenchmarkSubscriptionLookup -benchmem -count=5 > new.txt
benchstat old.txt new.txt
```

---

## 6. 常见问题

### Q1: sync.Map 什么时候性能会退化？

**A**: 当写操作频繁时，`sync.Map` 性能可能不如 `RWMutex + map`。建议：
- 读多写少场景使用 `sync.Map`
- 写多场景回退到 `RWMutex + map`

### Q2: atomic.Value 类型断言失败怎么办？

**A**: 添加类型检查和 panic recovery：

```go
func (k *kafkaEventBus) getProducer() (sarama.SyncProducer, error) {
    producerAny := k.producer.Load()
    if producerAny == nil {
        return nil, fmt.Errorf("producer not initialized")
    }
    
    producer, ok := producerAny.(sarama.SyncProducer)
    if !ok {
        return nil, fmt.Errorf("invalid producer type")
    }
    
    return producer, nil
}
```

### Q3: 如何监控锁竞争？

**A**: 使用 pprof：

```bash
# 启用 mutex profiling
go test -mutexprofile=mutex.out

# 查看锁竞争
go tool pprof mutex.out
```

### Q4: 恢复模式何时切换？

**A**: 建议策略：
- 启动时进入恢复模式
- 检测到无积压后切换到普通模式
- 可配置切换阈值

---

## 5. NATS EventBus 免锁优化实施指南

### 5.1 实施概览

NATS EventBus 的免锁优化遵循与 Kafka 相同的原则和模式，主要优化点包括：

- **P0 优化**: `closed`、`topicHandlers`、`conn/js`
- **P1 优化**: `topicConfigs`、`createdStreams` + 单飞抑制

### 5.2 P0 优化实施步骤

#### 步骤 1: 修改结构体定义

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go`

**修改前** (Line 204-220):
```go
type natsEventBus struct {
    conn               *nats.Conn
    js                 nats.JetStreamContext
    config             *NATSConfig
    subscriptions      map[string]*nats.Subscription
    logger             *zap.Logger
    mu                 sync.RWMutex
    closed             bool
    reconnectCallbacks []func(ctx context.Context) error

    unifiedConsumer    nats.ConsumerInfo
    topicHandlers      map[string]MessageHandler
    topicHandlersMu    sync.RWMutex
    // ...
}
```

**修改后**:
```go
type natsEventBus struct {
    // 🔥 P0修复：改为 atomic.Value（发布时无锁读取）
    conn atomic.Value // stores *nats.Conn
    js   atomic.Value // stores nats.JetStreamContext

    config        *NATSConfig
    subscriptions map[string]*nats.Subscription
    logger        *zap.Logger

    // ✅ 低频路径：保留 mu（用于 Subscribe、Close 等低频操作）
    mu     sync.Mutex  // 🔥 改为 Mutex（不再需要读写锁）
    closed atomic.Bool // 🔥 P0修复：改为 atomic.Bool，热路径无锁读取

    reconnectCallbacks []func(ctx context.Context) error

    unifiedConsumer nats.ConsumerInfo

    // 🔥 P0修复：改为 sync.Map（消息路由时无锁查找）
    topicHandlers sync.Map // key: string (topic), value: MessageHandler
    // ...
}
```

#### 步骤 2: 修改初始化逻辑

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go`

**NewNATSEventBus 函数**:
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
    return bus, nil
}
```

#### 步骤 3: 添加 Helper 方法

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go`

```go
// getConn 无锁读取 NATS Connection
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

// getJetStreamContext 无锁读取 JetStream Context
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
```

#### 步骤 4: 修改高频路径方法

**Publish 方法**:
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

**processUnifiedPullMessages 方法**:
```go
func (n *natsEventBus) processUnifiedPullMessages(ctx context.Context, topic string, sub *nats.Subscription) {
    for {
        msgs, err := sub.Fetch(batchSize, nats.MaxWait(fetchTimeout))
        // ...

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

#### 步骤 5: 修改低频路径方法

**Close 方法**:
```go
func (n *natsEventBus) Close() error {
    n.mu.Lock() // 保留锁 - 关闭是低频操作
    defer n.mu.Unlock()

    // 🔥 P0修复：使用 atomic.Bool 设置关闭状态
    if n.closed.Load() {
        return nil
    }
    n.closed.Store(true)

    // 获取连接对象
    conn, err := n.getConn()
    if err != nil {
        return err
    }

    // ... 其他关闭逻辑
    conn.Drain()
    conn.Close()

    return nil
}
```

**Subscribe 方法**:
```go
func (n *natsEventBus) subscribeJetStream(ctx context.Context, topic string, handler MessageHandler) error {
    // 🔥 P0修复：使用 sync.Map 存储 handler
    n.topicHandlers.Store(topic, handler)

    // ... 其他订阅逻辑
}
```

### 5.3 P1 优化实施步骤

#### 步骤 1: 添加单飞抑制

**修改结构体**:
```go
import "golang.org/x/sync/singleflight"

type natsEventBus struct {
    // ...

    // 🔥 P1优化：改为 sync.Map（发布时无锁读取）
    createdStreams sync.Map // key: string (streamName), value: bool

    // 🔥 P1优化：单飞抑制（避免并发创建风暴）
    streamCreateSF singleflight.Group

    // ...
}
```

#### 步骤 2: 实现 ensureStream 方法

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

### 5.4 测试验证

#### 并发测试
```bash
# 运行并发测试（检测数据竞态）
cd jxt-core/tests/eventbus/performance_tests
go test -race -v -run TestKafkaVsNATSPerformanceComparison -timeout=30m
go test -race -v -run TestMemoryPersistenceComparison -timeout=30m
```

#### 性能测试
```bash
# 运行性能测试
cd jxt-core/tests/eventbus/performance_tests
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout=30m
go test -v -run TestMemoryPersistenceComparison -timeout=30m
```

#### 协程泄漏检测
```bash
# 使用 pprof 检测协程泄漏
go test -v -run TestMemoryPersistenceComparison -timeout=30m -memprofile=mem.prof -cpuprofile=cpu.prof

# 查看协程数
go tool pprof -http=:8080 mem.prof
```

### 5.5 预期结果

优化完成后，NATS EventBus 应达到以下性能指标：

| 指标 | 优化前 | 优化后 | 提升幅度 |
|------|--------|--------|---------|
| **吞吐量** | 2861 msg/s | 3500-4000 msg/s | **+22-40%** |
| **P99 延迟** | 32 ms | 22-26 ms | **-20-30%** |
| **Handler 查找** | 100 万次/秒 | 300-500 万次/秒 | **3-5x** |
| **锁竞争** | 30% | 5-10% | **-70%** |
| **协程泄漏** | 1-12 个 | 0 个 | **-100%** |

---

**文档结束**


