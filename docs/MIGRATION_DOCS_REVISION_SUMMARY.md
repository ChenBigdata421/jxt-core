# EventBus 迁移文档修订总结

**修订日期**: 2025-10-31  
**修订人**: AI Assistant  
**修订原因**: 澄清按 Topic 类型区分的设计原则  

---

## 📋 修订概述

### 🎯 **核心澄清**

之前的理解存在偏差，现已修正为：

| 维度 | 错误理解 | 正确理解 |
|------|---------|---------|
| **区分维度** | 按存储类型区分 | **按 Topic 类型区分** |
| **领域事件** | 可能有/无聚合ID | **必须有聚合ID** |
| **普通消息** | 可能有/无聚合ID | 可能有/无聚合ID，**但总是 Round-Robin** |
| **路由策略** | 按聚合ID是否存在 | **按 Topic 类型** |
| **错误处理** | 统一处理 | **按 Topic 类型区分** |

---

## 🔑 **正确的设计原则**

### 1. **按 Topic 类型区分**

| Topic 类型 | 订阅方法 | 路由策略 | 错误处理 | 语义 |
|-----------|---------|---------|---------|------|
| **领域事件 Topic** | `SubscribeEnvelope` | **聚合ID Hash** | Nak/不 Mark（重投） | **at-least-once** |
| **普通消息 Topic** | `Subscribe` | **Round-Robin** | Ack/Mark（不重投） | **at-most-once** |

### 2. **领域事件 Topic**

**特征**：
- ✅ **必须使用** `SubscribeEnvelope` 订阅
- ✅ **必须有聚合ID**（从 Envelope 中提取）
- ✅ **路由策略**：聚合ID Hash → 保证同一聚合的事件按顺序处理
- ✅ **错误处理**：Nak/不 MarkMessage → at-least-once
- ✅ **存储策略**（NATS）：file storage（磁盘持久化）

**示例 Topic**：
```
domain.order.created
domain.payment.completed
event.user.registered
```

**异常情况**：
- 如果领域事件没有聚合ID → 记录错误日志 + Nak/不 Mark（重投，等待修复）

### 3. **普通消息 Topic**

**特征**：
- ✅ **使用** `Subscribe` 订阅
- ✅ **可能没有聚合ID**
- ✅ **路由策略**：Round-Robin → **即使有聚合ID也忽略**
- ✅ **错误处理**：Ack/MarkMessage → at-most-once
- ✅ **存储策略**（NATS）：memory storage（内存存储）

**示例 Topic**：
```
notification.email
cache.invalidate
metrics.report
```

---

## 📊 **路由策略对比**

### Kafka/NATS 统一实现

```go
func (bus *EventBus) getRoutingKey(aggregateID string, isEnvelope bool) string {
    if isEnvelope {
        // ⭐ 领域事件：必须使用聚合ID路由
        if aggregateID == "" {
            // ⚠️ 异常情况：领域事件没有聚合ID
            logger.Error("Domain event missing aggregate ID")
            return "" // 触发错误处理（Nak/不 Mark）
        }
        return aggregateID
    } else {
        // ⭐ 普通消息：总是使用 Round-Robin（忽略聚合ID）
        index := bus.roundRobinCounter.Add(1)
        return fmt.Sprintf("rr-%d", index)
    }
}
```

### 关键点

| 场景 | 领域事件 Topic | 普通消息 Topic |
|------|---------------|---------------|
| **有聚合ID** | 使用聚合ID Hash | **忽略聚合ID，使用 Round-Robin** |
| **无聚合ID** | **异常**（Nak/不 Mark） | 使用 Round-Robin |

---

## 📝 **已修订的文档**

### 1. **新增文档**

| 文档 | 说明 |
|------|------|
| `EVENTBUS_DUAL_SEMANTICS_DESIGN.md` | EventBus 双语义设计规范（核心文档） |
| `MIGRATION_DOCS_REVISION_SUMMARY.md` | 本文档 |

### 2. **NATS 迁移文档修订**

| 文档 | 修订内容 |
|------|---------|
| `nats-subscribe-to-actor-pool-migration-architecture.md` | ✅ 更新核心变更表<br>✅ 添加存储策略行<br>✅ 更新术语（无聚合ID → 普通消息）<br>✅ 添加 Subscribe vs SubscribeEnvelope 语义差异表<br>✅ 更新架构图<br>✅ 更新路由策略设计 |
| `nats-subscribe-to-actor-pool-migration-implementation.md` | ✅ 更新 handleMessage 实现<br>✅ 更新 handleMessageWithWrapper 实现<br>✅ 添加领域事件无聚合ID的异常处理<br>✅ 添加 Done Channel 等待逻辑 |
| `nats-subscribe-to-actor-pool-migration-summary.md` | ✅ 更新核心变更描述<br>✅ 更新架构图 |

### 3. **Kafka 迁移文档修订**

| 文档 | 修订内容 |
|------|---------|
| `kafka-subscribe-to-actor-pool-migration-architecture.md` | ✅ 更新核心变更表<br>✅ 添加错误处理行<br>✅ 更新术语（无聚合ID → 普通消息）<br>✅ 添加 Subscribe vs SubscribeEnvelope 语义差异表<br>✅ 更新架构图<br>✅ 更新路由策略设计 |

---

## 🔧 **实现要点**

### 1. **Kafka 实现**

```go
// 等待处理完成
select {
case err := <-aggMsg.Done:
    if err != nil {
        if wrapper.isEnvelope {
            // ⭐ 领域事件：不 MarkMessage，让 Kafka 重新投递
            logger.Warn("Domain event processing failed, will be redelivered", ...)
            return err // 不 MarkMessage
        } else {
            // ⭐ 普通消息：MarkMessage，避免重复投递
            logger.Warn("Regular message processing failed, marking as processed", ...)
            session.MarkMessage(message, "")
            return err
        }
    }
    // 成功：MarkMessage
    session.MarkMessage(message, "")
    return nil
}
```

### 2. **NATS 实现**

```go
// 等待处理完成
select {
case err := <-aggMsg.Done:
    if err != nil {
        if wrapper.isEnvelope {
            // ⭐ 领域事件：Nak 重新投递
            logger.Warn("Domain event processing failed, will be redelivered", ...)
            nakFunc()
        } else {
            // ⭐ 普通消息：Ack，避免重复投递
            logger.Warn("Regular message processing failed, marking as processed", ...)
            ackFunc()
        }
        return
    }
    // 成功：Ack
    ackFunc()
}
```

### 3. **存储策略（NATS 专用）**

```go
func (n *natsEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
    // ⭐ 普通消息 Topic：强制使用 memory storage
    return n.subscribeJetStreamWithStorage(ctx, topic, handler, false, nats.MemoryStorage)
}

func (n *natsEventBus) SubscribeEnvelope(ctx context.Context, topic string, handler MessageHandler) error {
    // ⭐ 领域事件 Topic：强制使用 file storage
    return n.subscribeJetStreamWithStorage(ctx, topic, handler, true, nats.FileStorage)
}
```

---

## ✅ **修订清单**

### 已完成

- [x] 创建 `EVENTBUS_DUAL_SEMANTICS_DESIGN.md`（核心设计规范）
- [x] 更新 NATS 架构文档
- [x] 更新 NATS 实施文档
- [x] 更新 Kafka 架构文档
- [x] 创建修订总结文档

### 待完成（代码实现）

- [ ] 修改 `nats.go` 的 `handleMessageWithWrapper` 方法
- [ ] 添加 `roundRobinCounter atomic.Uint64` 字段
- [ ] 实现按 Topic 类型区分的路由逻辑
- [ ] 实现 Done Channel 等待逻辑
- [ ] 实现按 Topic 类型区分的错误处理
- [ ] 实现存储类型区分（Subscribe=memory, SubscribeEnvelope=file）
- [ ] 运行测试验证

---

## 🎯 **关键差异总结**

### Kafka vs NATS

| 特性 | Kafka | NATS |
|------|-------|------|
| **路由策略** | 领域事件=聚合ID Hash<br>普通消息=Round-Robin | 领域事件=聚合ID Hash<br>普通消息=Round-Robin |
| **错误处理** | 领域事件=不 MarkMessage<br>普通消息=MarkMessage | 领域事件=Nak<br>普通消息=Ack |
| **存储类型** | 统一磁盘存储 | 领域事件=file<br>普通消息=memory |
| **Actor Pool** | ✅ 统一使用 | ✅ 统一使用 |
| **Round-Robin** | ✅ 已实现 | ⚠️ 需实现 |

---

## 📚 **参考文档**

1. **核心设计**：`EVENTBUS_DUAL_SEMANTICS_DESIGN.md`
2. **NATS 迁移**：
   - `nats-subscribe-to-actor-pool-migration-architecture.md`
   - `nats-subscribe-to-actor-pool-migration-implementation.md`
   - `nats-subscribe-to-actor-pool-migration-testing.md`
3. **Kafka 迁移**：
   - `kafka-subscribe-to-actor-pool-migration-architecture.md`
   - `kafka-subscribe-to-actor-pool-migration-implementation.md`
   - `kafka-subscribe-to-actor-pool-migration-testing.md`
4. **Kafka 实现参考**：
   - `jxt-core/sdk/pkg/eventbus/kafka.go` (Line 960-1030)
   - `jxt-core/sdk/pkg/eventbus/hollywood_actor_pool.go`

---

## 🚀 **下一步行动**

1. **代码实现**：
   - 实现 NATS 的 Round-Robin 路由
   - 实现 Done Channel 等待
   - 实现按 Topic 类型区分的错误处理
   - 实现存储类型区分

2. **测试验证**：
   - 验证领域事件的聚合ID Hash 路由
   - 验证普通消息的 Round-Robin 路由
   - 验证领域事件无聚合ID的异常处理
   - 验证错误处理的区分（Nak vs Ack）
   - 验证存储类型区分（file vs memory）

3. **文档完善**：
   - 更新测试文档
   - 更新影响分析文档
   - 创建开发者指南


