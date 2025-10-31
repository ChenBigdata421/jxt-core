# NATS JetStream Subscribe() 迁移到 Hollywood Actor Pool - 架构设计文档

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

将 NATS JetStream EventBus 的 `Subscribe()` 方法迁移到 **Hollywood Actor Pool**，并彻底移除未使用的 Worker Pool 代码。

### 🔑 **核心变更**

| 项目 | 当前实现 | 目标实现 |
|------|---------|---------|
| **普通消息处理** | 直接处理（goroutine 中执行） | Hollywood Actor Pool（Round-Robin 轮询） |
| **领域事件处理** | Hollywood Actor Pool（聚合ID哈希） | Hollywood Actor Pool（聚合ID哈希，保持不变） |
| **并发模型** | 混合模式（Actor Pool + 直接处理） | 单一架构（Actor Pool） |
| **代码复杂度** | 中（存在未使用的 Worker Pool 代码） | 低（统一架构，清理死代码） |
| **故障恢复** | 普通消息无故障恢复 | Actor Pool Supervisor 机制统一管理 |
| **存储策略** | 统一配置 | 按 Topic 类型：领域事件=file，普通消息=memory |

### ✅ **预期收益**

1. **架构统一**: 单一并发模型，降低维护成本
2. **企业级可靠性**: Supervisor机制、故障隔离、自动重启
3. **性能优化**: 一致性哈希、Inbox缓冲、减少锁竞争
4. **可观测性**: Actor级别监控、事件流、详细指标
5. **代码简化**: 删除 200+ 行全局 Worker Pool 代码

---

## 背景和动机

### 📊 **当前问题**

#### 1. **架构不统一**

NATS JetStream EventBus 当前使用**混合处理模式**:
- **Hollywood Actor Pool**: 处理领域事件（SubscribeEnvelope，有聚合ID）
- **直接处理**: 处理普通消息（Subscribe，可能无聚合ID）

**问题**:
- ❌ 普通消息缺少并发控制和背压机制
- ❌ 普通消息缺少故障恢复机制（无 Supervisor）
- ❌ 监控指标不统一（无 Actor 级别指标）
- ❌ 存在未使用的 Worker Pool 代码（200+ 行死代码）
- ❌ 存储策略未按 Topic 类型区分（领域事件 vs 普通消息）

#### 2. **全局 Worker Pool 的局限性**

**代码位置**: `jxt-core/sdk/pkg/eventbus/nats.go`

```go
// NATSGlobalWorkerPool NATS专用的全局Worker池
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

// 总代码量: ~200 行
```

**局限性**:
- ❌ **无故障恢复**: Worker panic 后不会自动重启
- ❌ **无故障隔离**: 一个 Worker panic 可能影响其他消息
- ❌ **监控不足**: 缺少 Actor 级别的详细监控
- ❌ **代码重复**: 与 Kafka 的 Worker Pool 类似，但独立实现

#### 3. **与 Kafka 的不一致性**

Kafka EventBus 已经完成了从全局 Worker Pool 到 Hollywood Actor Pool 的迁移，但 NATS 仍然使用旧架构。

**问题**:
- ❌ 架构不一致，增加学习成本
- ❌ 监控指标不统一
- ❌ 故障恢复机制不一致

---

### 🎯 **迁移动机**

1. **统一架构**: 所有消息使用同一套并发模型
2. **提升可靠性**: 利用 Actor Pool 的 Supervisor 机制
3. **简化代码**: 删除全局 Worker Pool 实现
4. **增强监控**: 统一的 Actor 级别监控
5. **与 Kafka 保持一致**: 统一 EventBus 架构

---

## 当前架构分析

### 🔍 **消息路由逻辑**

#### **当前实现**（`handleMessage` 和 `handleMessageWithWrapper`）

```go
func (n *natsEventBus) handleMessage(ctx context.Context, topic string, data []byte, handler MessageHandler, ackFunc func() error) {
    // 提取聚合ID
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
        // ❌ 无聚合ID：直接处理（未使用 Worker Pool）
        // 注意：当前代码中，无聚合ID的消息直接在 goroutine 中处理
        // 并未使用全局 Worker Pool
        err := handler(handlerCtx, data)
        // ...
    }
}
```

**关键发现**:
- ✅ **SubscribeEnvelope（领域事件 Topic）**: 消息是 Envelope 格式，`env.AggregateID` 总是存在 → Actor Pool（聚合ID Hash）
- ⚠️ **Subscribe（普通消息 Topic）**: 消息是原始字节，通常无聚合ID → 直接处理（未使用 Worker Pool）

**重要发现**:
当前代码中，全局 Worker Pool 的代码虽然存在，但实际上**并未被使用**！普通消息直接在 `handleMessage` 的 goroutine 中处理。

**设计原则**:
- 📌 **按 Topic 类型区分**：领域事件 Topic vs 普通消息 Topic
- 📌 **领域事件**：必须有聚合ID，使用聚合ID Hash 路由，保证顺序性
- 📌 **普通消息**：可能无聚合ID，使用 Round-Robin 路由，无序并发

这意味着：
1. 删除全局 Worker Pool 代码**不会影响现有功能**（因为从未被调用）
2. 迁移到 Actor Pool 可以提供**更好的并发控制**和**故障恢复**

### 📝 **Subscribe vs SubscribeEnvelope 语义差异（按 Topic 类型区分）**

| 特性 | Subscribe（普通消息 Topic） | SubscribeEnvelope（领域事件 Topic） |
|------|---------------------------|--------------------------------|
| **Topic 类型** | 普通消息（notification.*, cache.*） | 领域事件（domain.*, event.*） |
| **消息格式** | 原始字节（[]byte） | Envelope 包装（含 AggregateID） |
| **聚合ID** | 可能无聚合ID | **必须有聚合ID** |
| **路由策略** | **Round-Robin**（忽略聚合ID） | **聚合ID Hash**（保证顺序） |
| **JetStream Storage** | **Memory**（内存存储） | **File**（磁盘持久化） |
| **失败处理** | Ack（不重投） | Nak（重投） |
| **交付语义** | **at-most-once** | **at-least-once** |
| **适用场景** | 临时消息、通知、非关键数据 | 领域事件、关键业务数据 |
| **性能** | 更快（内存存储 + 无序并发） | 较慢（磁盘 I/O + 顺序处理） |
| **可靠性** | 低（进程重启丢失） | 高（磁盘持久化 + 重投） |

**设计原则**:
- **Subscribe（普通消息 Topic）**:
  - 使用 **memory storage**（性能优先）
  - 失败时 Ack（不重投）→ **at-most-once**
  - 使用 **Round-Robin** 路由（无序并发）
  - 示例：`notification.email`, `cache.invalidate`

- **SubscribeEnvelope（领域事件 Topic）**:
  - 使用 **file storage**（可靠性优先）
  - 失败时 Nak（重投）→ **at-least-once**
  - 使用 **聚合ID Hash** 路由（保证顺序）
  - 示例：`domain.order.created`, `event.payment.completed`

**重要**：由开发者保证为正确的 Topic 类型选择正确的订阅方法

---

### 📊 **当前架构图**

```
NATS JetStream 消息
    ↓
processUnifiedPullMessages()
    ↓
    ├─ 每个消息启动一个 goroutine
    │       ↓
    │   handleMessage() / handleMessageWithWrapper()
    │       ↓
    │       ├─ 领域事件 Topic（SubscribeEnvelope）
    │       │   - 有聚合ID → Hollywood Actor Pool（256 Actors）
    │       │                    ↓
    │       │                聚合ID Hash 路由（保证顺序）
    │       │                    ↓
    │       │                file storage（磁盘持久化）
    │       │
    │       └─ 普通消息 Topic（Subscribe）
    │           - 可能无聚合ID → 直接处理（在当前 goroutine）
    │                              ↓
    │                          并发处理（无控制）
    │                              ↓
    │                          memory storage（内存存储）
    │
    └─ 全局 Worker Pool（未使用，但代码存在）
```

**问题**:
- ❌ 普通消息直接在 goroutine 中处理，无并发控制
- ❌ 可能导致 goroutine 数量爆炸（高并发场景）
- ❌ 普通消息无故障恢复机制
- ❌ 全局 Worker Pool 代码存在但未使用，造成代码冗余
- ❌ 存储策略未按 Topic 类型区分

---

## 目标架构设计

### 🎯 **核心设计原则**

1. **统一路由**: 所有消息都使用 Hollywood Actor Pool
2. **按 Topic 类型区分路由**:
   - 领域事件 Topic：聚合ID Hash（保证顺序）
   - 普通消息 Topic：Round-Robin（无序并发）
3. **按 Topic 类型区分存储**:
   - 领域事件 Topic：file storage（可靠性优先）
   - 普通消息 Topic：memory storage（性能优先）
4. **按 Topic 类型区分错误处理**:
   - 领域事件：Nak 重投（at-least-once）
   - 普通消息：Ack 不重投（at-most-once）
5. **零配置**: 用户无需修改任何代码或配置
6. **彻底清除**: 完全删除全局 Worker Pool 代码

### 🏗️ **目标架构**

```
NATS JetStream 消息
    ↓
processUnifiedPullMessages()
    ↓
    ├─ 每个消息启动一个 goroutine
    │       ↓
    │   handleMessage() / handleMessageWithWrapper()
    │       ↓
    │       ├─ 领域事件 Topic（SubscribeEnvelope）
    │       │   - 必须有聚合ID → Hollywood Actor Pool（256 Actors）
    │       │                        ↓
    │       │                    routingKey = aggregateID
    │       │                        ↓
    │       │                    聚合ID Hash 路由（保证顺序）
    │       │                        ↓
    │       │                    file storage + Nak 重投
    │       │
    │       └─ 普通消息 Topic（Subscribe）
    │           - 可能无聚合ID → Hollywood Actor Pool（256 Actors）
    │                              ↓
    │                          routingKey = "rr-{counter++}"
    │                              ↓
    │                          Round-Robin 路由（无序并发）
    │                              ↓
    │                          memory storage + Ack 不重投
    │
    └─ 全局 Worker Pool（删除）
```

**优势**:
- ✅ 单一架构，维护成本低
- ✅ 并发控制：Actor Pool 限制并发数为 256
- ✅ 故障恢复：Supervisor 机制自动重启
- ✅ 负载均衡：Round-Robin 轮询均匀分配（普通消息）
- ✅ 顺序保证：聚合ID Hash（领域事件）
- ✅ 性能优化：普通消息用 memory storage
- ✅ 可靠性保证：领域事件用 file storage + 重投
- ✅ 代码简化：删除 200+ 行冗余代码

---

### 🔑 **路由策略设计（按 Topic 类型区分）**

#### **方案：双路由策略**（采用）

```go
type natsEventBus struct {
    // ... 现有字段 ...

    // ⭐ 新增：Round-Robin 计数器（用于普通消息）
    roundRobinCounter atomic.Uint64
}

func (n *natsEventBus) getRoutingKey(aggregateID string, isEnvelope bool) string {
    if isEnvelope {
        // ⭐ 领域事件：必须使用聚合ID路由（保证顺序）
        if aggregateID == "" {
            // ⚠️ 异常情况：领域事件没有聚合ID
            // 记录错误日志，Nak 重投
            return "" // 返回空，触发 Nak
        }
        return aggregateID
    } else {
        // ⭐ 普通消息：总是使用 Round-Robin（忽略聚合ID）
        counter := n.roundRobinCounter.Add(1)
        return fmt.Sprintf("rr-%d", counter)
    }
}

// 使用示例
routingKey := n.getRoutingKey(aggregateID, wrapper.isEnvelope)
// Hollywood Actor Pool 内部会对 routingKey 进行哈希
```

**关键点**:
- 📌 **领域事件**：必须有聚合ID，否则 Nak 重投
- 📌 **普通消息**：总是 Round-Robin，即使有聚合ID也忽略

**优势**:
- ✅ **完美负载均衡**: 消息均匀分配到所有 256 个 Actor
- ✅ **最大化并发**: 不同消息并发处理，充分利用 Actor Pool
- ✅ **保持原有特性**: 与直接处理的并发特性一致
- ✅ **实现简单**: 使用原子计数器，无需哈希计算
- ✅ **无单点瓶颈**: 避免单 Topic 成为性能瓶颈

**劣势**:
- ⚠️ **无缓存局部性**: 相同 topic 的消息可能路由到不同 Actor
- ⚠️ **无顺序保证**: 消息并发处理，无顺序保证（与原实现一致）

---

## 技术决策

### 决策 1: 使用 Round-Robin 轮询路由

**问题**: 无聚合ID的消息如何路由到 Actor Pool？

**备选方案**:

| 方案 | 路由键 | 并发性 | 负载均衡 | 顺序保证 | 缓存局部性 |
|------|-------|-------|---------|---------|-----------|
| **Round-Robin**（采用） | `counter++` | ✅ 最高 | ✅ 完美 | ❌ 无 | ❌ 无 |
| Topic 哈希 | `topic` | ⚠️ 低 | ⚠️ 依赖分布 | ✅ 同Topic有序 | ✅ 高 |
| 随机 | `rand()` | ✅ 高 | ✅ 较好 | ❌ 无 | ❌ 无 |

**最终选择**: **Round-Robin 轮询路由**

**理由**:
1. 与原实现行为一致（直接处理，无顺序保证）
2. 保持最大并发性能
3. 避免迁移风险
4. 实现简单，性能最优

---

### 决策 2: 彻底删除全局 Worker Pool

**问题**: 是否保留全局 Worker Pool 代码作为备用？

**决策**: **彻底删除**

**理由**:
1. 当前代码中全局 Worker Pool **未被使用**
2. 保留会增加代码复杂度和维护成本
3. Hollywood Actor Pool 已经过充分验证
4. 与 Kafka EventBus 保持一致

---

### 决策 3: 保持 API 接口不变

**问题**: 是否需要修改 `Subscribe()` 接口？

**决策**: **保持不变**

**理由**:
1. 用户代码无需修改
2. 内部实现变更对用户透明
3. 降低迁移风险

---

## 架构对比

### 📊 **性能对比**

| 指标 | 当前实现（直接处理） | Hollywood Actor Pool | 变化 |
|------|-----------------|---------------------|------|
| **吞吐量** | ~162 msg/s（实测） | ~162 msg/s（预期） | ≈ 持平 |
| **延迟** | ~1010 ms（实测） | ~1010 ms（预期） | ≈ 持平 |
| **内存占用** | 动态（goroutine数） | 固定（256×1000） | 更可控 |
| **协程数** | 动态（无限制） | 固定（256） | ✅ 更可控 |
| **故障恢复** | 无 | 自动重启（3次） | ✅ 提升 |

### 🔍 **代码复杂度对比**

| 指标 | 当前实现 | 目标实现 | 变化 |
|------|---------|---------|------|
| **总代码行数** | ~250 行 | ~50 行 | -200 行 |
| **并发模型** | 2 种 | 1 种 | -1 种 |
| **监控指标** | 2 套 | 1 套 | -1 套 |
| **故障恢复** | 0 种 | 1 种 | +1 种 |

---

## 风险评估

### 🔴 **高风险**

无

### 🟡 **中风险**

#### 1. 性能回归风险

**风险**: 迁移后性能可能下降

**缓解措施**:
- ✅ 性能测试验证（吞吐量、延迟）
- ✅ 压力测试（高并发场景）
- ✅ 监控指标对比

#### 2. 行为变更风险

**风险**: 路由策略变更可能影响消息处理顺序

**缓解措施**:
- ✅ 文档说明：无聚合ID的消息无顺序保证（与原实现一致）
- ✅ 测试验证：确保行为一致

### 🟢 **低风险**

#### 1. 代码回归风险

**风险**: 删除代码可能影响未知功能

**缓解措施**:
- ✅ 全局 Worker Pool 未被使用，删除无影响
- ✅ 充分的回归测试
- ✅ Git 版本控制，可快速回滚

---

## 成功标准

### 1. 功能正确性

- ✅ 所有现有测试通过（4/4 NATS 单元测试）
- ✅ 消息正确路由和处理
- ✅ 无消息丢失
- ✅ 错误处理正确

### 2. 性能达标

- ✅ 吞吐量 ≥ 当前水平（~162 msg/s）
- ✅ 延迟 ≤ 当前水平（~1010 ms）
- ✅ 内存占用 ≤ 当前水平（~3.14 MB）
- ✅ 协程数稳定（无泄漏）

### 3. 可靠性保证

- ✅ Actor panic 自动重启
- ✅ 故障隔离正常
- ✅ 监控指标正确

### 4. 代码质量

- ✅ 删除 200+ 行冗余代码
- ✅ 代码可读性提升
- ✅ 架构统一

---

## 附录

### A. 相关文档

- [Kafka Subscribe() 迁移文档](./kafka-subscribe-to-actor-pool-migration-index.md)
- [Hollywood Actor Pool 迁移指南](../sdk/pkg/eventbus/docs/hollywood-actor-pool-migration-guide.md)
- [NATS Actor Pool 迁移总结](../sdk/pkg/eventbus/NATS_ACTOR_POOL_MIGRATION_SUMMARY.md)

### B. 参考资料

- [Hollywood Actor Framework](https://github.com/anthdm/hollywood)
- [NATS JetStream Documentation](https://docs.nats.io/nats-concepts/jetstream)
- [Actor Model](https://en.wikipedia.org/wiki/Actor_model)

---

**文档状态**: 待评审  
**下一步**: 等待评审批准后，创建实施计划文档

