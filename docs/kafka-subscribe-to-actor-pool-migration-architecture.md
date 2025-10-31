# Kafka EventBus Subscribe() 迁移到 Hollywood Actor Pool - 架构设计文档

**文档版本**: v1.0  
**创建日期**: 2025-10-30  
**状态**: 待评审  
**作者**: AI Assistant  

---

## 📋 目录

1. [执行摘要](#执行摘要)
2. [背景和动机](#背景和动机)
3. [当前架构分析](#当前架构分析)
4. [目标架构设计](#目标架构设计)
5. [技术决策](#技术决策)
6. [架构对比](#架构对比)
7. [风险评估](#风险评估)
8. [成功标准](#成功标准)

---

## 执行摘要

### 🎯 **迁移目标**

将 Kafka EventBus 的 `Subscribe()` 方法从**全局 Worker Pool**迁移到 **Hollywood Actor Pool**，并彻底移除全局 Worker Pool 的实现和使用。

### 🔑 **核心变更**

| 项目 | 当前实现 | 目标实现 |
|------|---------|---------|
| **普通消息处理** | 全局 Worker Pool（轮询分发） | Hollywood Actor Pool（Round-Robin 轮询） |
| **领域事件处理** | Hollywood Actor Pool（聚合ID哈希） | Hollywood Actor Pool（聚合ID哈希，保持不变） |
| **并发模型** | 双架构（Actor Pool + Worker Pool） | 单一架构（Actor Pool） |
| **代码复杂度** | 高（两套并发机制） | 低（统一架构） |
| **故障恢复** | Worker Pool无自动重启 | Actor Pool Supervisor机制 |
| **错误处理** | 统一 MarkMessage | 按 Topic 类型：领域事件=不 Mark（重投），普通消息=Mark（不重投） |

### ✅ **预期收益**

1. **架构统一**: 单一并发模型，降低维护成本
2. **企业级可靠性**: Supervisor机制、故障隔离、自动重启
3. **性能优化**: 一致性哈希、Inbox缓冲、减少锁竞争
4. **可观测性**: Actor级别监控、事件流、详细指标
5. **代码简化**: 删除 200+ 行全局 Worker Pool 代码

---

## 背景和动机

### 📊 **当前问题**

#### 1. **架构复杂性**

```
当前架构（双并发模型）:
┌─────────────────────────────────────────────────────────┐
│                    Kafka EventBus                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────────┐         ┌──────────────────┐     │
│  │  有聚合ID消息     │         │  无聚合ID消息     │     │
│  │  (SubscribeEnv)  │         │  (Subscribe)     │     │
│  └────────┬─────────┘         └────────┬─────────┘     │
│           │                            │                │
│           ▼                            ▼                │
│  ┌──────────────────┐         ┌──────────────────┐     │
│  │ Hollywood Actor  │         │ Global Worker    │     │
│  │ Pool (256 actors)│         │ Pool (动态workers)│     │
│  │ - Supervisor     │         │ - 轮询分发        │     │
│  │ - 故障隔离        │         │ - 无故障恢复      │     │
│  │ - 自动重启        │         │ - 简单背压        │     │
│  └──────────────────┘         └──────────────────┘     │
│                                                          │
└─────────────────────────────────────────────────────────┘

问题:
❌ 两套并发机制，维护成本高
❌ Worker Pool 缺少企业级特性
❌ 代码重复，逻辑分散
```

#### 2. **全局 Worker Pool 的局限性**

| 特性 | Worker Pool | Actor Pool |
|------|------------|-----------|
| **故障隔离** | ❌ 无 | ✅ 有（Actor级别） |
| **自动重启** | ❌ 无 | ✅ 有（Supervisor） |
| **监控指标** | ⚠️ 基础 | ✅ 详细（Actor级别） |
| **背压控制** | ⚠️ 简单（100ms超时） | ✅ 完善（Inbox满载阻塞） |
| **事件流** | ❌ 无 | ✅ 有（DeadLetter、Restart） |
| **负载均衡** | ⚠️ 轮询（可能不均） | ✅ 一致性哈希 |

#### 3. **代码维护负担**

```go
// 当前需要维护的代码量
jxt-core/sdk/pkg/eventbus/kafka.go:
  - GlobalWorkerPool 结构体定义: ~20 行
  - NewGlobalWorkerPool: ~25 行
  - start(): ~25 行
  - dispatcher(): ~30 行
  - SubmitWork(): ~15 行
  - Worker.start(): ~15 行
  - processWork(): ~20 行
  - Close(): ~25 行
  - WorkItem 结构体: ~10 行
  - processMessageDirectly(): ~20 行
  
  总计: ~205 行

jxt-core/sdk/pkg/eventbus/kafka.go (使用部分):
  - 初始化 globalWorkerPool: ~5 行
  - processMessageWithKeyedPool 中的路由逻辑: ~20 行
  - Close 中的清理逻辑: ~5 行
  
  总计: ~30 行

总代码量: ~235 行（仅 Kafka）
```

### 🎯 **迁移动机**

1. **统一架构**: 所有消息使用同一套并发模型
2. **提升可靠性**: 利用 Actor Pool 的 Supervisor 机制
3. **简化代码**: 删除全局 Worker Pool 实现
4. **增强监控**: 统一的 Actor 级别监控
5. **未来扩展**: 为 NATS 迁移铺路（NATS 也有类似问题）

---

## 当前架构分析

### 🔍 **消息路由逻辑**

#### **当前实现**（`processMessageWithKeyedPool`）

```go
func (h *preSubscriptionConsumerHandler) processMessageWithKeyedPool(
    ctx context.Context, 
    message *sarama.ConsumerMessage, 
    handler MessageHandler, 
    session sarama.ConsumerGroupSession,
) error {
    // 1️⃣ 提取聚合ID
    aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")
    
    if aggregateID != "" {
        // 2️⃣ 有聚合ID：使用 Hollywood Actor Pool
        pool := h.eventBus.globalActorPool
        if pool != nil {
            aggMsg := &AggregateMessage{
                Topic:       message.Topic,
                AggregateID: aggregateID,
                Handler:     handler,  // ⭐ 每个消息携带自己的 handler
                // ...
            }
            pool.ProcessMessage(ctx, aggMsg)
            // 等待处理完成
            select {
            case err := <-aggMsg.Done:
                session.MarkMessage(message, "")
                return err
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
    
    // 3️⃣ 无聚合ID：使用全局 Worker Pool ⚠️ 问题所在
    workItem := WorkItem{
        Topic:   message.Topic,
        Message: message,
        Handler: handler,  // ⭐ 每个消息携带自己的 handler
        Session: session,
    }
    
    if !h.eventBus.globalWorkerPool.SubmitWork(workItem) {
        // 如果 Worker 池满了，直接处理
        h.processMessageDirectly(ctx, message, handler, session)
    }
    
    return nil
}
```

#### **全局 Worker Pool 工作流程**

```
消息流:
┌─────────────────────────────────────────────────────────┐
│ 1. Kafka Consumer 接收消息                               │
│    ↓                                                     │
│ 2. ExtractAggregateID(message) → ""（无聚合ID）          │
│    ↓                                                     │
│ 3. 创建 WorkItem{Topic, Message, Handler, Session}      │
│    ↓                                                     │
│ 4. globalWorkerPool.SubmitWork(workItem)                │
│    ↓                                                     │
│ 5. workQueue <- workItem（缓冲队列）                     │
│    ↓                                                     │
│ 6. dispatcher() 轮询分发                                 │
│    ├─ Worker 0 ← workItem                               │
│    ├─ Worker 1 ← workItem                               │
│    ├─ Worker 2 ← workItem                               │
│    └─ ...                                                │
│    ↓                                                     │
│ 7. Worker.processWork(workItem)                         │
│    ├─ handler(ctx, message.Value)                       │
│    └─ session.MarkMessage(message, "")                  │
└─────────────────────────────────────────────────────────┘

问题:
❌ 轮询分发：负载可能不均（取决于 Worker 处理速度）
❌ 无故障隔离：Worker panic 会导致整个 Worker 停止
❌ 无自动重启：Worker 停止后不会自动恢复
❌ 监控有限：只有基础的队列满警告
```

### 📊 **ExtractAggregateID 逻辑**

```go
func ExtractAggregateID(
    msgBytes []byte,        // 消息体
    headers map[string]string,  // Headers
    kafkaKey []byte,        // Kafka Key
    natsSubject string,     // NATS Subject（Kafka 为空）
) (string, error) {
    // 优先级 1: 从 Envelope 提取
    if len(msgBytes) > 0 {
        env, err := FromBytes(msgBytes)
        if err == nil && env.AggregateID != "" {
            return env.AggregateID, nil  // ✅ SubscribeEnvelope 走这里
        }
    }
    
    // 优先级 2: 从 Headers 提取
    if aggID := headers["X-Aggregate-ID"]; aggID != "" {
        return aggID, nil
    }
    
    // 优先级 3: 从 Kafka Key 提取
    if len(kafkaKey) > 0 {
        return string(kafkaKey), nil
    }
    
    // 优先级 4: 从 NATS Subject 提取（启发式）
    // Kafka 场景下不适用
    
    return "", nil  // ❌ Subscribe 通常走这里，返回空字符串
}
```

**关键发现**:
- ✅ **SubscribeEnvelope（领域事件 Topic）**: 消息是 Envelope 格式，`env.AggregateID` 总是存在 → Actor Pool（聚合ID Hash）
- ❌ **Subscribe（普通消息 Topic）**: 消息是原始字节，通常无法提取聚合ID → Worker Pool（Round-Robin）

### 📝 **Subscribe vs SubscribeEnvelope 语义差异（按 Topic 类型区分）**

| 特性 | Subscribe（普通消息 Topic） | SubscribeEnvelope（领域事件 Topic） |
|------|---------------------------|--------------------------------|
| **Topic 类型** | 普通消息（notification.*, cache.*） | 领域事件（domain.*, event.*） |
| **消息格式** | 原始字节（[]byte） | Envelope 包装（含 AggregateID） |
| **聚合ID** | 可能无聚合ID | **必须有聚合ID** |
| **路由策略** | **Round-Robin**（忽略聚合ID） | **聚合ID Hash**（保证顺序） |
| **失败处理** | MarkMessage（不重投） | 不 MarkMessage（重投） |
| **交付语义** | **at-most-once** | **at-least-once** |
| **适用场景** | 临时消息、通知、非关键数据 | 领域事件、关键业务数据 |
| **性能** | 更快（无序并发） | 较慢（顺序处理） |
| **可靠性** | 低（错误不重投） | 高（错误重投） |

**设计原则**:
- **Subscribe（普通消息 Topic）**:
  - 失败时 MarkMessage（不重投）→ **at-most-once**
  - 使用 **Round-Robin** 路由（无序并发）
  - 示例：`notification.email`, `cache.invalidate`

- **SubscribeEnvelope（领域事件 Topic）**:
  - 失败时不 MarkMessage（重投）→ **at-least-once**
  - 使用 **聚合ID Hash** 路由（保证顺序）
  - 示例：`domain.order.created`, `event.payment.completed`

**重要**：由开发者保证为正确的 Topic 类型选择正确的订阅方法

---

## 目标架构设计

### 🎯 **核心设计原则**

1. **统一路由**: 所有消息都使用 Hollywood Actor Pool
2. **按 Topic 类型区分路由**:
   - 领域事件 Topic：聚合ID Hash（保证顺序）
   - 普通消息 Topic：Round-Robin（无序并发）
3. **按 Topic 类型区分错误处理**:
   - 领域事件：不 MarkMessage（重投，at-least-once）
   - 普通消息：MarkMessage（不重投，at-most-once）
4. **零配置**: 用户无需修改任何代码或配置
5. **彻底清除**: 完全删除全局 Worker Pool 代码

### 🏗️ **目标架构**

```
目标架构（单一并发模型，按 Topic 类型区分）:
┌─────────────────────────────────────────────────────────┐
│                    Kafka EventBus                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────────┐         ┌──────────────────┐     │
│  │  领域事件 Topic   │         │  普通消息 Topic   │     │
│  │  (SubscribeEnv)  │         │  (Subscribe)     │     │
│  └────────┬─────────┘         └────────┬─────────┘     │
│           │                            │                │
│           │  aggregateID               │  Round-Robin   │
│           │  (必须有)                  │  (忽略聚合ID)  │
│           │                            │                │
│           ▼                            ▼                │
│  ┌──────────────────────────────────────────────────┐  │
│  │        Hollywood Actor Pool (256 actors)         │  │
│  │                                                   │  │
│  │  路由策略:                                         │  │
│  │  - 领域事件: hash(aggregateID) % 256（保证顺序）  │  │
│  │  - 普通消息: Round-Robin 轮询（无序并发）         │  │
│  │                                                   │  │
│  │  错误处理:                                         │  │
│  │  - 领域事件: 不 MarkMessage（重投，at-least-once）│  │
│  │  - 普通消息: MarkMessage（不重投，at-most-once）  │  │
│  │                                                   │  │
│  │  企业级特性:                                       │  │
│  │  ✅ Supervisor 机制（自动重启）                    │  │
│  │  ✅ 故障隔离（Actor 级别）                         │  │
│  │  ✅ Inbox 缓冲（1000 条/Actor）                   │  │
│  │  ✅ 背压控制（Inbox 满载阻塞）                     │  │
│  │  ✅ 事件流（DeadLetter、Restart）                 │  │
│  │  ✅ 详细监控（Actor 级别指标）                     │  │
│  └──────────────────────────────────────────────────┘  │
│                                                          │
└─────────────────────────────────────────────────────────┘

优势:
✅ 单一架构，维护成本低
✅ 统一的企业级特性
✅ 更好的可观测性
✅ 按 Topic 类型区分路由和错误处理
✅ 领域事件保证顺序性和可靠性
✅ 普通消息优化性能
✅ 代码简化（删除 ~235 行）
```

### 🔑 **路由策略设计**

#### **方案选择: 双路由策略（按 Topic 类型区分）**（采用）

```go
// kafkaEventBus 中添加轮询计数器
type kafkaEventBus struct {
    // ... 其他字段
    roundRobinCounter atomic.Uint64  // 轮询计数器（用于普通消息）
}

// 统一的路由键生成逻辑（按 Topic 类型区分）
func (k *kafkaEventBus) getRoutingKey(aggregateID string, isEnvelope bool) string {
    if isEnvelope {
        // ⭐ 领域事件：必须使用聚合ID路由（保证顺序）
        if aggregateID == "" {
            // ⚠️ 异常情况：领域事件没有聚合ID
            // 记录错误日志，不 MarkMessage（重投）
            return "" // 返回空，触发错误处理
        }
        return aggregateID
    } else {
        // ⭐ 普通消息：总是使用 Round-Robin（忽略聚合ID）
        index := k.roundRobinCounter.Add(1)
        return fmt.Sprintf("rr-%d", index)
    }
}

// 使用示例
routingKey := k.getRoutingKey(aggregateID, wrapper.isEnvelope)
// Hollywood Actor Pool 内部会对 routingKey 进行哈希
```

**关键点**:
- 📌 **领域事件（SubscribeEnvelope）**：必须有聚合ID，使用聚合ID Hash，保证顺序
- 📌 **普通消息（Subscribe）**：总是 Round-Robin，即使有聚合ID也忽略

**优势**:
- ✅ **完美负载均衡**: 消息均匀分配到所有 256 个 Actor
- ✅ **最大化并发**: 不同消息并发处理，充分利用 Actor Pool
- ✅ **保持原有特性**: 与全局 Worker Pool 的轮询分发一致
- ✅ **实现简单**: 使用原子计数器，无需哈希计算
- ✅ **无单点瓶颈**: 避免单 Topic 成为性能瓶颈

**劣势**:
- ⚠️ **无缓存局部性**: 相同 topic 的消息可能路由到不同 Actor
- ⚠️ **无顺序保证**: 消息并发处理，无顺序保证（与原 Worker Pool 一致）

**替代方案对比**:

| 方案 | 路由键 | 并发性 | 负载均衡 | 顺序保证 | 缓存局部性 |
|------|-------|-------|---------|---------|-----------|
| **Round-Robin**（采用） | `counter++` | ✅ 最高 | ✅ 完美 | ❌ 无 | ❌ 无 |
| Topic 哈希 | `topic` | ⚠️ 低 | ⚠️ 依赖分布 | ✅ 同Topic有序 | ✅ 高 |
| 随机 | `rand()` | ✅ 高 | ✅ 较好 | ❌ 无 | ❌ 无 |

**最终选择**: **Round-Robin 轮询路由**
- 理由: 与原全局 Worker Pool 行为一致，保持最大并发性能，避免迁移风险

---

## 技术决策

### 决策 1: 路由策略

**问题**: 无聚合ID的消息如何路由到256个Actor？

**决策**: 使用 **Round-Robin 轮询路由**

**理由**:
1. **保持原有并发特性**: 全局 Worker Pool 使用轮询分发，迁移后保持一致
2. **完美负载均衡**: 消息均匀分配到所有 256 个 Actor
3. **最大化并发**: 不同消息可以并发处理，充分利用 Actor Pool
4. **实现简单**: 使用原子计数器实现轮询，无需哈希计算

**实现**:
```go
// kafkaEventBus 中添加轮询计数器
type kafkaEventBus struct {
    // ... 其他字段
    roundRobinCounter atomic.Uint64  // 轮询计数器
}

// 路由逻辑
func (k *kafkaEventBus) getRoutingKey(aggregateID string) string {
    if aggregateID != "" {
        return aggregateID  // 有聚合ID：使用聚合ID（保持有序）
    }
    // 无聚合ID：使用轮询索引（保持并发）
    index := k.roundRobinCounter.Add(1)
    return fmt.Sprintf("rr-%d", index)
}
```

**对比其他方案**:

| 方案 | 并发性 | 负载均衡 | 顺序保证 | 缓存局部性 |
|------|-------|---------|---------|-----------|
| **Round-Robin（采用）** | ✅ 最高 | ✅ 完美 | ❌ 无 | ❌ 无 |
| Topic 哈希 | ⚠️ 低（单Topic瓶颈） | ⚠️ 依赖Topic分布 | ✅ 同Topic有序 | ✅ 高 |
| Topic+Partition 哈希 | ✅ 中等 | ✅ 较好 | ✅ 同Partition有序 | ✅ 中等 |

**选择 Round-Robin 的原因**:
- ✅ 与原全局 Worker Pool 行为一致，降低迁移风险
- ✅ 最大化并发性能，避免单点瓶颈
- ✅ 实现最简单，性能开销最小

---

### 决策 2: 顺序保证语义

**问题**: 迁移后，无聚合ID消息的顺序语义会发生变化吗？

**当前语义**:
- 无聚合ID消息 → 全局 Worker Pool → **Round-Robin 轮询** → **并发处理**（无顺序保证）

**目标语义**:
- 无聚合ID消息 → Actor Pool → **Round-Robin 轮询** → **并发处理**（无顺序保证）

**影响分析**:
- ✅ **无影响**: 顺序语义保持不变（都是无序并发处理）
- ✅ **并发性保持**: 与原 Worker Pool 一样，消息并发处理
- ✅ **性能保持**: 负载均衡特性一致

**决策**: **保持语义不变**

**理由**:
1. Round-Robin 路由保持了原有的并发特性
2. 无聚合ID消息本来就不需要顺序保证
3. 性能特征与原 Worker Pool 一致
4. 用户代码无需任何调整

---

### 决策 3: 代码删除策略

**问题**: 是否彻底删除全局 Worker Pool 代码？

**决策**: **彻底删除**

**理由**:
1. 项目未上生产环境，无需保留兼容代码
2. 删除代码可以降低维护成本
3. 避免代码冗余和混淆
4. 简化架构，降低复杂度

**删除清单**（以符号名为准）:
- `WorkItem` 结构体（~10 行）
- `GlobalWorkerPool` 结构体及所有方法（~160 行）
  - `NewGlobalWorkerPool()` 构造函数
  - `start()` 启动方法
  - `dispatcher()` 分发器
  - `SubmitWork()` 工作提交方法
  - `Close()` 关闭方法
- `Worker` 结构体及所有方法（~45 行）
  - `start()` 启动方法
  - `processWork()` 工作处理方法
- `processMessageDirectly()` 方法（~20 行）
- `globalWorkerPool` 字段及初始化/清理代码（~10 行）

**总计**: 约 200+ 行代码删除

---

### 决策 4: NATS EventBus 处理

**问题**: NATS EventBus 也有类似的全局 Worker Pool，是否一并迁移？

**决策**: **本次不迁移 NATS，单独处理**

**理由**:
1. 降低风险，分阶段迁移
2. Kafka 和 NATS 的实现细节不同，需要分别测试
3. 本次重点验证 Kafka 迁移的可行性
4. NATS 迁移可以复用 Kafka 的经验和代码

**后续计划**:
- 第一阶段: Kafka EventBus 迁移（本次）
- 第二阶段: NATS EventBus 迁移（后续）
- 第三阶段: 统一文档和最佳实践

---

### 决策 5: 订阅互斥规则

**问题**: `Subscribe()` 和 `SubscribeEnvelope()` 能否同时订阅同一个 topic？

**决策**: **不能，同一 topic 只能订阅一次**

**理由**:
1. 当前实现使用 `sync.Map.LoadOrStore()` 检查重复订阅
2. 同一 topic 的第二次订阅会返回错误：`"already subscribed to topic: %s"`
3. 这是 API 级别的约束，与底层并发模型无关

**实现**:
```go
// kafka.go
if _, loaded := k.subscriptions.LoadOrStore(topic, handler); loaded {
    k.mu.Unlock()
    return fmt.Errorf("already subscribed to topic: %s", topic)
}
```

**影响**:
- ✅ 避免重复订阅导致的消息重复处理
- ✅ 简化订阅管理逻辑
- ⚠️ 用户需要明确选择 `Subscribe()` 或 `SubscribeEnvelope()`，不能混用

**最佳实践**:
- 如果消息包含聚合ID（Envelope格式），使用 `SubscribeEnvelope()`
- 如果消息是普通格式（无聚合ID），使用 `Subscribe()`
- 同一 topic 只能选择其中一种订阅方式

---

## 架构对比

### 📊 **性能对比**

| 指标 | 全局 Worker Pool | Hollywood Actor Pool | 变化 |
|------|-----------------|---------------------|------|
| **吞吐量** | ~6000 msg/s（估算） | ~6170 msg/s（实测） | ≈ 持平 |
| **延迟** | 未知 | 185 ms（实测） | 待测试 |
| **内存占用** | 动态（Worker数×队列） | 固定（256×1000） | 更可控 |
| **协程数** | 动态（CPU×2） | 固定（256） | 更可控 |
| **故障恢复** | 无 | 自动重启（3次） | ✅ 提升 |

### 🔍 **代码复杂度对比**

| 项目 | 当前实现 | 目标实现 | 变化 |
|------|---------|---------|------|
| **并发模型** | 2套（Actor + Worker） | 1套（Actor） | -50% |
| **代码行数** | ~3555 行 | ~3310 行 | -245 行 |
| **路由逻辑** | 2处（Actor路由 + Worker分发） | 1处（统一路由） | -50% |
| **清理逻辑** | 2处（Actor Stop + Worker Close） | 1处（Actor Stop） | -50% |

### 🎯 **可观测性对比**

| 指标 | 全局 Worker Pool | Hollywood Actor Pool |
|------|-----------------|---------------------|
| **监控粒度** | Pool 级别 | Actor 级别（256个） |
| **指标数量** | 基础（队列满警告） | 详细（消息数、重启次数、Inbox深度） |
| **事件流** | 无 | DeadLetterEvent、ActorRestartedEvent |
| **调试能力** | 有限 | 强（Actor ID、消息追踪） |

---

## 风险评估

### ⚠️ **高风险**

#### 风险 1: 单 Topic 高负载瓶颈

**描述**: 如果某个 topic 的消息量非常大，所有消息都路由到同一个 Actor，可能成为瓶颈。

**影响**: 吞吐量下降，延迟增加

**缓解措施**:
1. **性能测试**: 在迁移前进行压力测试，验证单 Actor 的吞吐量上限
2. **监控告警**: 监控 Actor Inbox 深度，超过阈值时告警
3. **文档说明**: 在文档中说明，建议用户将高负载 topic 拆分为多个 topic
4. **后备方案**: 如果性能测试不通过，可以回退到轮询路由策略

**概率**: 中等（取决于用户使用场景）

**严重性**: 高（影响吞吐量）

---

### ⚠️ **中风险**

#### 风险 2: 顺序语义变化导致的兼容性问题

**描述**: 无聚合ID消息从并发处理变为串行处理，可能影响依赖并发的用户代码。

**影响**: 用户代码行为变化，可能导致性能下降

**缓解措施**:
1. **文档说明**: 在迁移文档中明确说明语义变化
2. **性能测试**: 验证串行处理的吞吐量是否满足需求
3. **用户指导**: 建议用户使用多个 topic 来实现并发

**概率**: 低（项目未上生产环境）

**严重性**: 中（可能影响性能）

---

### ⚠️ **低风险**

#### 风险 3: Actor Pool Inbox 满载

**描述**: 如果消息生产速度远大于消费速度，Actor Inbox 可能满载，导致消息阻塞。

**影响**: 消息延迟增加，可能导致 Kafka Consumer 超时

**缓解措施**:
1. **Inbox 大小配置**: 保持 Inbox 大小为 1000（与原 Worker Pool 队列大小一致）
2. **监控告警**: 监控 Inbox 深度，超过 80% 时告警
3. **背压机制**: Actor Pool 的 Send 操作会阻塞，自然形成背压

**概率**: 低（Inbox 大小足够）

**严重性**: 中（影响延迟）

---

## 成功标准

### ✅ **功能验证**

1. **基本功能**
   - ✅ Subscribe() 订阅成功
   - ✅ 消息正常接收和处理
   - ✅ Handler 正确执行
   - ✅ Kafka offset 正确提交

2. **路由正确性**
   - ✅ 有聚合ID消息路由到正确的 Actor（基于 aggregateID）
   - ✅ 无聚合ID消息路由到正确的 Actor（基于 topic）
   - ✅ 相同 topic 的消息路由到同一 Actor

3. **故障恢复**
   - ✅ Handler panic 后 Actor 自动重启
   - ✅ Actor 重启后继续处理消息
   - ✅ 重启次数不超过 maxRestarts（3次）

### 📊 **性能验证**

1. **吞吐量**
   - ✅ 单 topic 吞吐量 ≥ 5000 msg/s
   - ✅ 多 topic 吞吐量 ≥ 6000 msg/s
   - ✅ 与当前实现性能持平或更优

2. **延迟**
   - ✅ P50 延迟 ≤ 200 ms
   - ✅ P99 延迟 ≤ 500 ms

3. **资源占用**
   - ✅ 内存占用 ≤ 当前实现的 120%
   - ✅ 协程数稳定（256 个 Actor）
   - ✅ 无协程泄漏

### 🧪 **测试覆盖**

1. **单元测试**
   - ✅ 路由逻辑测试（有/无聚合ID）
   - ✅ Actor Pool 初始化测试
   - ✅ 消息处理测试

2. **集成测试**
   - ✅ Kafka 基本发布订阅测试
   - ✅ 多消息测试
   - ✅ Envelope 发布订阅测试

3. **性能测试**
   - ✅ 单 topic 压力测试
   - ✅ 多 topic 压力测试
   - ✅ 内存持久化性能对比测试

4. **可靠性测试**
   - ✅ Handler panic 恢复测试
   - ✅ Actor 重启测试
   - ✅ Inbox 满载测试

### 📝 **文档完整性**

1. **技术文档**
   - ✅ 架构设计文档
   - ✅ 实施计划文档
   - ✅ 测试计划文档
   - ✅ 影响分析文档

2. **用户文档**
   - ✅ README 更新（说明架构变化）
   - ✅ 迁移指南（如果需要）
   - ✅ 最佳实践文档

---

## 附录

### A. 相关文档

- [Kafka Actor Pool 实施总结](../sdk/pkg/eventbus/docs/kafka-actor-pool-final-summary.md)
- [Hollywood Actor Pool 迁移指南](../sdk/pkg/eventbus/docs/hollywood-actor-pool-migration-guide.md)
- [NATS Actor Pool 迁移总结](../sdk/pkg/eventbus/NATS_ACTOR_POOL_MIGRATION_SUMMARY.md)

### B. 参考资料

- [Hollywood Actor Framework](https://github.com/anthdm/hollywood)
- [Kafka Consumer Group](https://kafka.apache.org/documentation/#consumerapi)
- [Actor Model](https://en.wikipedia.org/wiki/Actor_model)

---

**文档状态**: 待评审  
**下一步**: 等待评审批准后，创建实施计划文档

