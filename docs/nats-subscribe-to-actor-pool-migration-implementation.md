# NATS JetStream Subscribe() 迁移到 Hollywood Actor Pool - 实施计划文档

**文档版本**: v1.0  
**创建日期**: 2025-10-30  
**状态**: 待评审  
**作者**: AI Assistant  

---

## 📋 目录

1. [执行摘要](#执行摘要)
2. [代码修改清单](#代码修改清单)
3. [详细实施步骤](#详细实施步骤)
4. [回滚方案](#回滚方案)
5. [验证清单](#验证清单)

---

## 执行摘要

### 🎯 **实施目标**

将 NATS JetStream EventBus 的 `Subscribe()` 方法迁移到 **Hollywood Actor Pool**，并实现 Subscribe/SubscribeEnvelope 的存储类型区分，彻底移除未使用的 Worker Pool 代码。

### 🔑 **核心变更**

1. **Subscribe 使用 memory storage** → at-most-once 语义
2. **SubscribeEnvelope 使用 file storage** → at-least-once 语义
3. **无聚合ID消息使用 Round-Robin 路由** → 统一到 Actor Pool
4. **删除未使用的 Worker Pool 代码** → 清理死代码
5. **⭐ 完全复制 Kafka 的实现模式** → 确保一致性

### 🎯 **参考实现**

本迁移方案**完全参考 Kafka EventBus 的 Actor Pool 实现**（已验证、已上线）：
- 路由逻辑：`kafka.go` Line 962-974
- 错误处理：`kafka.go` Line 1004-1027
- AggregateMessage 构建：`kafka.go` Line 981-994
- Done Channel 等待：`kafka.go` Line 1004-1030

详细分析见：`KAFKA_ACTOR_POOL_INSIGHTS_FOR_NATS.md`

### 📊 **工作量估算**

| 任务 | 文件数 | 代码行数 | 预计时间 |
|------|-------|---------|---------|
| **代码修改** | 1 | ~80 行修改 + 200 行删除 | 3 小时 |
| **测试修改** | 1 | ~50 行（新增语义测试） | 1 小时 |
| **文档更新** | 2 | ~150 行 | 1 小时 |
| **测试验证** | - | - | 3 小时 |
| **总计** | 4 | ~480 行 | 8 小时 |

---

## 代码修改清单

### 📝 **修改文件列表**

| 文件路径 | 修改类型 | 代码行数 | 说明 |
|---------|---------|---------|------|
| `jxt-core/sdk/pkg/eventbus/nats.go` | 删除 + 修改 | ~280 行 | 删除 Worker Pool，修改路由逻辑，区分存储类型 |

---

### 🔍 **详细修改内容**

#### **0. 区分 Subscribe/SubscribeEnvelope 的存储类型**（新增 ~30 行）

**目标**: 实现 Subscribe = memory storage (at-most-once), SubscribeEnvelope = file storage (at-least-once)

##### 0.1 修改 `Subscribe()` 方法，强制使用 memory storage

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go` (Line ~920-980)

```go
func (n *natsEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    // ... 现有代码 ...

    if n.config.JetStream.Enabled && jsAvailable {
        // ⭐ Subscribe 使用 memory storage（at-most-once）
        err = n.subscribeJetStreamWithStorage(ctx, topic, handler, false, nats.MemoryStorage)
    } else {
        // Core NATS 订阅
        // ...
    }
}
```

##### 0.2 修改 `SubscribeEnvelope()` 方法，强制使用 file storage

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go` (Line ~2730-2790)

```go
func (n *natsEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error {
    // ... 现有代码 ...

    if n.config.JetStream.Enabled && jsAvailable {
        // ⭐ SubscribeEnvelope 使用 file storage（at-least-once）
        err = n.subscribeJetStreamWithStorage(ctx, topic, wrappedHandler, true, nats.FileStorage)
    } else {
        // Core NATS 订阅
        // ...
    }
}
```

##### 0.3 新增 `subscribeJetStreamWithStorage()` 方法

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go` (新增 ~20 行)

```go
// subscribeJetStreamWithStorage 使用指定的存储类型订阅 JetStream
func (n *natsEventBus) subscribeJetStreamWithStorage(
    ctx context.Context,
    topic string,
    handler MessageHandler,
    isEnvelope bool,
    storageType nats.StorageType, // ⭐ 新增参数
) error {
    // 创建 Stream 时使用指定的 storageType
    streamConfig := &nats.StreamConfig{
        Name:      n.getStreamNameForTopic(topic),
        Subjects:  []string{topic},
        Storage:   storageType, // ⭐ 使用传入的存储类型
        Retention: nats.LimitsPolicy,
        Replicas:  1,
    }

    // ... 其余逻辑与 subscribeJetStream 相同 ...
}
```

---

#### **1. 删除全局 Worker Pool 代码**（~200 行）

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go`

**删除内容**:

##### 1.1 删除 `NATSWorkItem` 结构（Lines 67-96）

```go
// ❌ 删除
type NATSWorkItem struct {
    ctx     context.Context
    topic   string
    data    []byte
    handler MessageHandler
    ackFunc func() error
}
```

##### 1.2 删除 `NATSGlobalWorkerPool` 结构（Lines 98-108）

```go
// ❌ 删除
type NATSGlobalWorkerPool struct {
    workers     []*NATSWorker
    workQueue   chan NATSWorkItem
    workerCount int
    queueSize   int
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
    logger      *zap.Logger
}
```

##### 1.3 删除 `NATSWorker` 结构（Lines 110-114）

```go
// ❌ 删除
type NATSWorker struct {
    id   int
    pool *NATSGlobalWorkerPool
}
```

##### 1.4 删除 `NewNATSGlobalWorkerPool()` 函数（Lines 116-155）

```go
// ❌ 删除整个函数（~40 行）
func NewNATSGlobalWorkerPool(workerCount int, logger *zap.Logger) *NATSGlobalWorkerPool {
    // ...
}
```

##### 1.5 删除 `SubmitWork()` 方法（Lines 157-171）

```go
// ❌ 删除整个方法（~15 行）
func (p *NATSGlobalWorkerPool) SubmitWork(item NATSWorkItem) error {
    // ...
}
```

##### 1.6 删除 `start()` 方法（Lines 173-182）

```go
// ❌ 删除整个方法（~10 行）
func (p *NATSGlobalWorkerPool) start() {
    // ...
}
```

##### 1.7 删除 `processWork()` 方法（Lines 184-193）

```go
// ❌ 删除整个方法（~10 行）
func (w *NATSWorker) processWork() {
    // ...
}
```

##### 1.8 删除 `Close()` 方法（Lines 195-198）

```go
// ❌ 删除整个方法（~4 行）
func (p *NATSGlobalWorkerPool) Close() {
    // ...
}
```

##### 1.9 删除 `natsEventBus` 结构中的 `globalWorkerPool` 字段（Line 268）

```go
type natsEventBus struct {
    // ... 其他字段 ...
    
    // ❌ 删除
    globalWorkerPool *NATSGlobalWorkerPool
    
    // ... 其他字段 ...
}
```

##### 1.10 删除 `NewNATSEventBus()` 中的初始化代码（Lines 360-365）

```go
// ❌ 删除
// 创建全局Worker池
globalWorkerPool := NewNATSGlobalWorkerPool(256, logger)
globalWorkerPool.start()
```

##### 1.11 删除 `Close()` 中的清理代码（Lines 2600-2605）

```go
// ❌ 删除
// 关闭全局Worker池
if n.globalWorkerPool != nil {
    n.globalWorkerPool.Close()
}
```

**总计删除**: ~200 行

---

#### **2. 修改路由逻辑**（~50 行）

##### 2.1 添加 Round-Robin 计数器字段（Line 268）

```go
type natsEventBus struct {
    // ... 现有字段 ...
    
    // ⭐ 新增：Round-Robin 计数器（用于无聚合ID消息的轮询路由）
    roundRobinCounter atomic.Uint64
}
```

##### 2.2 修改 `handleMessage()` 方法（Lines 1274-1400）

**当前代码**:
```go
func (n *natsEventBus) handleMessage(ctx context.Context, topic string, data []byte, handler MessageHandler, ackFunc func() error) {
    // ... panic recovery ...
    
    aggregateID, _ := ExtractAggregateID(data, nil, nil, "")
    
    if aggregateID != "" {
        // ✅ 有聚合ID：使用 Hollywood Actor Pool
        if n.actorPool != nil {
            aggMsg := &AggregateMessage{
                AggregateID: aggregateID,
                // ...
            }
            n.actorPool.SubmitMessage(aggMsg)
            // ...
        }
    } else {
        // ❌ 无聚合ID：直接处理
        err := handler(handlerCtx, data)
        // ...
    }
}
```

**修改后**:
```go
func (n *natsEventBus) handleMessage(ctx context.Context, topic string, data []byte, handler MessageHandler, ackFunc func() error) {
    // ... panic recovery ...

    aggregateID, _ := ExtractAggregateID(data, nil, nil, topic)

    // ⭐ 普通消息（Subscribe）：总是使用 Round-Robin
    // 注意：handleMessage 只用于 Subscribe（普通消息 Topic）
    counter := n.roundRobinCounter.Add(1)
    routingKey := fmt.Sprintf("rr-%d", counter)
    
    // ⭐ 统一使用 Hollywood Actor Pool 处理
    if n.actorPool != nil {
        aggMsg := &AggregateMessage{
            Topic:       topic,
            Value:       data,
            AggregateID: routingKey, // ⭐ 使用 routingKey
            Context:     handlerCtx,
            Done:        make(chan error, 1), // ⭐ buffered channel
            Handler:     handler,
            IsEnvelope:  false, // ⭐ Subscribe 消息
        }

        if err := n.actorPool.ProcessMessage(handlerCtx, aggMsg); err != nil {
            n.logger.Error("Failed to submit message to actor pool", zap.Error(err))
            ackFunc() // ⭐ 普通消息失败也 Ack（at-most-once）
            n.errorCount.Add(1)
            return
        }

        // ⭐ 等待处理完成（参考 Kafka 实现）
        select {
        case err := <-aggMsg.Done:
            if err != nil {
                // ⭐ 普通消息：Ack，避免重复投递（at-most-once）
                n.logger.Warn("Regular message processing failed, marking as processed",
                    zap.String("topic", topic),
                    zap.Error(err))
                ackFunc()
                n.errorCount.Add(1)
                return
            }
            // 成功：Ack
            ackFunc()
            n.consumedMessages.Add(1)
            return
        case <-handlerCtx.Done():
            return
        }
    }
    
    // 降级：直接处理
    err := handler(handlerCtx, data)
    if err != nil {
        n.errorCount.Add(1)
        return
    }
    
    ackFunc()
    n.consumedMessages.Add(1)
}
```

##### 2.3 同步修改 `handleMessageWithWrapper()` 方法（Lines 1135-1270）

**修改内容**: 与 `handleMessage()` 类似，添加 Round-Robin 路由逻辑

```go
func (n *natsEventBus) handleMessageWithWrapper(ctx context.Context, topic string, data []byte, wrapper *handlerWrapper, ackFunc func() error, nakFunc func() error) {
    // ... panic recovery ...

    aggregateID, _ := ExtractAggregateID(data, nil, nil, topic)

    // ⭐ 根据消息类型确定路由键（按 Topic 类型区分）
    var routingKey string
    if wrapper.isEnvelope {
        // ⭐ 领域事件：必须使用聚合ID路由（保证顺序）
        routingKey = aggregateID
        if routingKey == "" {
            // ⚠️ 异常情况：领域事件没有聚合ID
            n.logger.Error("Domain event missing aggregate ID",
                zap.String("topic", topic))
            nakFunc() // Nak 重投，等待修复
            return
        }
    } else {
        // ⭐ 普通消息：总是使用 Round-Robin（忽略聚合ID）
        counter := n.roundRobinCounter.Add(1)
        routingKey = fmt.Sprintf("rr-%d", counter)
    }
    
    // ⭐ 统一使用 Hollywood Actor Pool 处理
    if n.actorPool != nil {
        aggMsg := &AggregateMessage{
            Topic:       topic,
            Value:       data,
            AggregateID: routingKey, // ⭐ 使用 routingKey
            Context:     handlerCtx,
            Done:        make(chan error, 1), // ⭐ buffered channel
            Handler:     wrapper.handler,
            IsEnvelope:  wrapper.isEnvelope,
        }

        if err := n.actorPool.ProcessMessage(handlerCtx, aggMsg); err != nil {
            n.logger.Error("Failed to submit message to actor pool", zap.Error(err))
            if wrapper.isEnvelope {
                nakFunc()
            } else {
                ackFunc()
            }
            return
        }

        // ⭐ 等待处理完成（参考 Kafka 实现）
        select {
        case err := <-aggMsg.Done:
            if err != nil {
                if wrapper.isEnvelope {
                    // ⭐ Envelope 消息：Nak 重新投递（at-least-once）
                    n.logger.Warn("Envelope message processing failed, will be redelivered",
                        zap.String("topic", topic),
                        zap.Error(err))
                    nakFunc()
                } else {
                    // ⭐ 普通消息：Ack，避免重复投递（at-most-once）
                    n.logger.Warn("Regular message processing failed, marking as processed",
                        zap.String("topic", topic),
                        zap.Error(err))
                    ackFunc()
                }
                return
            }
            // 成功：Ack
            ackFunc()
            n.consumedMessages.Add(1)
            return
        case <-handlerCtx.Done():
            return
        }
    }
    
    // 降级：直接处理
    err := wrapper.handler(handlerCtx, data)
    if err != nil {
        if wrapper.isEnvelope {
            if nakFunc != nil {
                nakFunc()
            }
        }
        return
    }
    
    ackFunc()
    n.consumedMessages.Add(1)
}
```

**总计修改**: ~50 行

---

## 详细实施步骤

### 🔧 **步骤 1: 删除全局 Worker Pool 代码**（1 小时）

#### 1.1 删除结构定义和方法

```bash
# 文件: jxt-core/sdk/pkg/eventbus/nats.go

# 删除以下内容:
# - Lines 67-96: NATSWorkItem 结构
# - Lines 98-108: NATSGlobalWorkerPool 结构
# - Lines 110-114: NATSWorker 结构
# - Lines 116-155: NewNATSGlobalWorkerPool() 函数
# - Lines 157-171: SubmitWork() 方法
# - Lines 173-182: start() 方法
# - Lines 184-193: processWork() 方法
# - Lines 195-198: Close() 方法
```

#### 1.2 删除 `natsEventBus` 结构中的字段

```go
// 删除 Line 268
globalWorkerPool *NATSGlobalWorkerPool
```

#### 1.3 删除初始化和清理代码

```go
// 删除 NewNATSEventBus() 中的初始化代码（Lines 360-365）
// 删除 Close() 中的清理代码（Lines 2600-2605）
```

---

### 🔧 **步骤 2: 添加 Round-Robin 计数器**（15 分钟）

```go
// 文件: jxt-core/sdk/pkg/eventbus/nats.go
// 位置: Line 268（natsEventBus 结构）

type natsEventBus struct {
    // ... 现有字段 ...
    
    // ⭐ 新增：Round-Robin 计数器
    roundRobinCounter atomic.Uint64
}
```

---

### 🔧 **步骤 3: 修改 `handleMessage()` 方法**（30 分钟）

参考 [2.2 修改 `handleMessage()` 方法](#22-修改-handlemessage-方法lines-1274-1400)

---

### 🔧 **步骤 4: 修改 `handleMessageWithWrapper()` 方法**（30 分钟）

参考 [2.3 同步修改 `handleMessageWithWrapper()` 方法](#23-同步修改-handlemessagewithwrapper-方法lines-1135-1270)

---

### 🔧 **步骤 5: 编译验证**（15 分钟）

```bash
cd jxt-core/sdk/pkg/eventbus
go build ./...
```

**预期结果**: 编译成功，无错误

---

### 🔧 **步骤 6: 运行单元测试**（1 小时）

```bash
cd jxt-core/sdk/pkg/eventbus
go test -v -run "TestNATS" -timeout 60s
```

**预期结果**: 所有测试通过

---

### 🔧 **步骤 7: 运行性能测试**（1 小时）

```bash
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s
```

**预期结果**: 性能指标达标

---

### 🔧 **步骤 8: 运行可靠性测试**（30 分钟）

```bash
cd jxt-core/tests/eventbus/reliability_regression_tests
go test -v -timeout 180s
```

**预期结果**: 所有测试通过

---

## 回滚方案

### 🔄 **方案 1: Git 回滚**（推荐）

```bash
# 查看提交历史
git log --oneline

# 回滚到迁移前的提交
git revert <commit-hash>

# 或者硬回滚
git reset --hard <commit-hash>
```

---

### 🔄 **方案 2: 手动恢复**

如果 Git 回滚不可行，可以手动恢复代码:

1. 从备份或 Git 历史中恢复 `nats.go` 文件
2. 重新编译和测试
3. 重新部署

---

## 验证清单

### ✅ **功能验证**

- [ ] 所有 NATS 单元测试通过（4/4）
- [ ] 消息正确路由到 Actor Pool
- [ ] 有聚合ID的消息按聚合ID路由
- [ ] 无聚合ID的消息使用 Round-Robin 路由
- [ ] 错误处理正确
- [ ] 无消息丢失

### ✅ **性能验证**

- [ ] 吞吐量 ≥ 162 msg/s
- [ ] 延迟 ≤ 1010 ms
- [ ] 内存占用 ≤ 3.14 MB
- [ ] 协程数稳定（无泄漏）

### ✅ **可靠性验证**

- [ ] Actor panic 自动重启
- [ ] 故障隔离正常
- [ ] 监控指标正确

### ✅ **代码质量验证**

- [ ] 删除 200+ 行冗余代码
- [ ] 代码可读性提升
- [ ] 无编译错误
- [ ] 无 lint 警告

---

## 附录

### A. 测试命令清单

```bash
# 1. 编译验证
cd jxt-core/sdk/pkg/eventbus
go build ./...

# 2. 单元测试
go test -v -run "TestNATS" -timeout 60s

# 3. 性能测试
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s

# 4. 可靠性测试
cd jxt-core/tests/eventbus/reliability_regression_tests
go test -v -timeout 180s
```

---

**文档状态**: 待评审  
**下一步**: 等待评审批准后，开始代码实施

