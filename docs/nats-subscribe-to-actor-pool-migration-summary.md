# NATS JetStream Subscribe() 迁移到 Hollywood Actor Pool - 迁移总结

**文档版本**: v1.0  
**创建日期**: 2025-10-30  
**状态**: 待评审  
**作者**: AI Assistant  

---

## 📋 执行摘要

### 🎯 **迁移目标**

将 NATS JetStream EventBus 的 `Subscribe()` 方法迁移到 **Hollywood Actor Pool**，并彻底移除未使用的 Worker Pool 代码。

### 🔑 **核心变更**

- ✅ **统一架构**: 所有消息（有/无聚合ID）都使用 Hollywood Actor Pool
- ✅ **路由策略**: 有聚合ID用聚合ID哈希，无聚合ID用 Round-Robin 轮询
- ✅ **代码简化**: 删除 200+ 行未使用的 Worker Pool 代码
- ✅ **零配置**: 用户无需修改代码或配置
- ✅ **企业级可靠性**: Supervisor 机制、故障隔离、自动重启
- ✅ **行为一致**: 保持原 Subscribe 的并发无序特性（无聚合ID消息无顺序保证）

### 📊 **预期收益**

| 收益类别 | 具体收益 |
|---------|---------|
| **架构统一** | 单一并发模型，降低维护成本 |
| **可靠性提升** | Supervisor 机制、故障隔离、自动重启 |
| **代码简化** | 删除 200+ 行代码，降低复杂度 |
| **可观测性** | Actor 级别监控、事件流、详细指标 |
| **性能优化** | 一致性哈希、Inbox 缓冲、减少锁竞争 |

---

## 🏗️ 架构对比

### **当前架构**

```
NATS JetStream 消息
    ↓
processUnifiedPullMessages()
    ↓
handleMessage() / handleMessageWithWrapper()
    ↓
    ├─ 有聚合ID → Hollywood Actor Pool（256 Actors）
    │                ↓
    │            顺序处理（按聚合ID哈希）
    │
    └─ 无聚合ID → 直接处理（goroutine 中执行）
                     ↓
                 并发无序处理
```

**问题**:
- ❌ 无聚合ID消息缺少并发控制和背压机制
- ❌ 无聚合ID消息缺少故障恢复机制（无 Supervisor）
- ❌ 监控指标不统一（无 Actor 级别指标）
- ❌ 存在未使用的 Worker Pool 代码（200+ 行）

---

### **目标架构**（单一架构）

```
NATS JetStream 消息
    ↓
processUnifiedPullMessages()
    ↓
handleMessageWithWrapper()
    ↓
    ├─ 有聚合ID → Hollywood Actor Pool（256 Actors）
    │                ↓
    │            顺序处理（按聚合ID哈希）
    │
    └─ 无聚合ID → Hollywood Actor Pool（256 Actors）
                     ↓
                 并发处理（Round-Robin 轮询）
```

**优势**:
- ✅ 单一架构，维护成本低
- ✅ Supervisor 机制，自动故障恢复
- ✅ 统一监控指标
- ✅ 代码简化（删除 200+ 行）

---

## 🔄 路由策略

### **有聚合ID的消息**（保持不变）

```go
// 使用聚合ID作为路由键
routingKey := aggregateID

// Hollywood Actor Pool 内部哈希
actorIndex := Hash(routingKey) % 256

// 同一聚合ID的消息路由到同一个 Actor
// 保证顺序处理
```

**特点**:
- ✅ 顺序保证：同一聚合ID的消息串行处理
- ✅ 故障隔离：不同聚合ID的消息互不影响
- ✅ 自动重启：Actor panic 后自动重启（最多3次）

---

### **无聚合ID的消息**（新增 Round-Robin）

```go
// 使用 Round-Robin 计数器生成路由键
counter := atomic.AddUint64(&n.roundRobinCounter, 1)
routingKey := fmt.Sprintf("rr-%d", counter)

// Hollywood Actor Pool 内部哈希
actorIndex := Hash(routingKey) % 256

// 消息轮询分发到所有 Actor
// 最大化并发处理
```

**特点**:
- ✅ 完美负载均衡：消息均匀分配到所有 256 个 Actor
- ✅ 最大化并发：不同消息并发处理，充分利用 Actor Pool
- ✅ 保持原有特性：保持并发无序处理特性（无顺序保证）
- ✅ 无单点瓶颈：避免单 Topic 成为性能瓶颈
- ✅ 故障恢复：Actor panic 后自动重启，消息重投

---

## 📝 代码修改清单

### **1. 删除全局 Worker Pool 代码**（~200 行）

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go`

**删除内容**:
- `NATSWorkItem` 结构（~30 行）
- `NATSGlobalWorkerPool` 结构（~10 行）
- `NATSWorker` 结构（~5 行）
- `NewNATSGlobalWorkerPool()` 函数（~40 行）
- `SubmitWork()` 方法（~15 行）
- `start()` 方法（~15 行）
- `processWork()` 方法（~20 行）
- `Close()` 方法（~20 行）
- `NewNATSEventBus()` 中的初始化代码（~5 行）
- `Close()` 中的清理代码（~5 行）
- `handleMessage()` 中的 Worker Pool 路由逻辑（~35 行）

**总计**: ~200 行

---

### **2. 修改路由逻辑**（~50 行）

**文件**: `jxt-core/sdk/pkg/eventbus/nats.go`

**修改内容**:

#### 2.1 添加 Round-Robin 计数器字段

```go
type natsEventBus struct {
    // ... 现有字段 ...
    
    // ⭐ 新增：Round-Robin 计数器（用于无聚合ID消息的轮询路由）
    roundRobinCounter atomic.Uint64
}
```

#### 2.2 修改 `handleMessage()` 方法

```go
func (n *natsEventBus) handleMessage(ctx context.Context, topic string, data []byte, handler MessageHandler, ackFunc func() error) {
    // ... panic recovery ...
    
    // ⭐ 智能路由决策
    aggregateID, _ := ExtractAggregateID(data, nil, nil, "")
    
    if aggregateID != "" {
        // ✅ 有聚合ID：使用聚合ID作为路由键
        routingKey := aggregateID
    } else {
        // ⭐ 无聚合ID：使用 Round-Robin 计数器生成路由键
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
            Done:        make(chan error, 1),
            Handler:     handler,
            IsEnvelope:  false,
        }
        
        n.actorPool.SubmitMessage(aggMsg)
        
        select {
        case err := <-aggMsg.Done:
            if err != nil {
                n.errorCount.Add(1)
                return
            }
        case <-handlerCtx.Done():
            return
        }
        
        // 确认消息
        ackFunc()
        n.consumedMessages.Add(1)
        return
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

#### 2.3 同步修改 `handleMessageWithWrapper()` 方法

```go
func (n *natsEventBus) handleMessageWithWrapper(ctx context.Context, topic string, data []byte, wrapper *handlerWrapper, ackFunc func() error, nakFunc func() error) {
    // ... panic recovery ...
    
    // ⭐ 智能路由决策
    aggregateID, _ := ExtractAggregateID(data, nil, nil, "")
    
    if aggregateID != "" {
        // ✅ 有聚合ID：使用聚合ID作为路由键
        routingKey := aggregateID
    } else {
        // ⭐ 无聚合ID：使用 Round-Robin 计数器生成路由键
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
            Done:        make(chan error, 1),
            Handler:     wrapper.handler,
            IsEnvelope:  wrapper.isEnvelope,
        }
        
        n.actorPool.SubmitMessage(aggMsg)
        
        select {
        case err := <-aggMsg.Done:
            if err != nil {
                if wrapper.isEnvelope {
                    // Envelope 消息：Nak 重新投递
                    if nakFunc != nil {
                        nakFunc()
                    }
                }
                return
            }
        case <-handlerCtx.Done():
            return
        }
        
        // 确认消息
        ackFunc()
        n.consumedMessages.Add(1)
        return
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

---

## 📊 工作量估算

| 任务 | 文件数 | 代码行数 | 预计时间 |
|------|-------|---------|---------|
| **代码修改** | 1 | ~50 行修改 + 200 行删除 | 2 小时 |
| **测试修改** | 0 | 0 行（现有测试无需修改） | 0 小时 |
| **文档更新** | 2 | ~100 行 | 1 小时 |
| **测试验证** | - | - | 2.5 小时 |
| **总计** | 3 | ~350 行 | 5.5 小时 |

---

## ✅ 成功标准

### 1. 功能正确性

- ✅ 所有现有测试通过
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

---

## 🔗 相关文档

1. [架构设计文档](./nats-subscribe-to-actor-pool-migration-architecture.md)
2. [实施计划文档](./nats-subscribe-to-actor-pool-migration-implementation.md)
3. [测试计划文档](./nats-subscribe-to-actor-pool-migration-testing.md)
4. [影响分析文档](./nats-subscribe-to-actor-pool-migration-impact.md)
5. [Kafka Subscribe() 迁移文档](./kafka-subscribe-to-actor-pool-migration-index.md)

---

**文档状态**: ✅ 待评审  
**创建日期**: 2025-10-30  
**下一步**: 等待评审批准后，开始代码实施

