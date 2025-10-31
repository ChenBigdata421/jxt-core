# NATS JetStream Subscribe() 迁移到 Hollywood Actor Pool - 影响分析文档

**文档版本**: v1.0  
**创建日期**: 2025-10-30  
**状态**: 待评审  
**作者**: AI Assistant  

---

## 📋 目录

1. [执行摘要](#执行摘要)
2. [影响概览](#影响概览)
3. [用户影响分析](#用户影响分析)
4. [代码影响分析](#代码影响分析)
5. [性能影响分析](#性能影响分析)
6. [运维影响分析](#运维影响分析)
7. [风险缓解措施](#风险缓解措施)

---

## 执行摘要

### 🎯 **迁移目标**

将 NATS JetStream EventBus 的 `Subscribe()` 方法从**全局 Worker Pool**迁移到 **Hollywood Actor Pool**，并彻底移除全局 Worker Pool 的实现和使用。

### 📊 **影响等级**

| 影响类型 | 影响等级 | 说明 |
|---------|---------|------|
| **用户影响** | 🟢 无影响 | API 接口不变，用户代码无需修改 |
| **代码影响** | 🟡 中等 | 删除 200+ 行代码，修改 50 行代码 |
| **性能影响** | 🟢 无影响 | 性能指标预期持平或提升 |
| **运维影响** | 🟡 中等 | 监控指标变化，需要更新告警规则 |

---

## 影响概览

### ✅ **正面影响**

1. **架构统一**: 单一并发模型，降低维护成本
2. **可靠性提升**: Supervisor 机制、故障隔离、自动重启
3. **代码简化**: 删除 200+ 行全局 Worker Pool 代码
4. **可观测性增强**: Actor 级别监控、事件流、详细指标
5. **性能优化**: 一致性哈希、Inbox 缓冲、减少锁竞争

### ⚠️ **潜在风险**

1. **性能回归**: 迁移后性能可能下降（低风险）
2. **行为变更**: 路由策略变更可能影响消息处理顺序（低风险）
3. **监控指标变化**: 需要更新监控和告警规则（中风险）

---

## 用户影响分析

### 👥 **影响范围**

| 用户类型 | 影响程度 | 说明 |
|---------|---------|------|
| **应用开发者** | 🟢 无影响 | API 接口不变，代码无需修改 |
| **运维工程师** | 🟡 中等 | 监控指标变化，需要更新告警规则 |
| **测试工程师** | 🟢 无影响 | 测试用例无需修改 |

---

### 📝 **用户代码兼容性**

#### **API 接口不变**

```go
// ✅ 用户代码无需修改

// 订阅普通消息（at-most-once）
err := bus.Subscribe(ctx, "orders", func(ctx context.Context, data []byte) error {
    // 处理消息
    return nil
})

// 订阅 Envelope 消息（at-least-once）
err := bus.SubscribeEnvelope(ctx, "orders", func(ctx context.Context, envelope *eventbus.Envelope) error {
    // 处理消息
    return nil
})
```

**结论**: 用户代码**完全兼容**，无需任何修改。

---

### 🔄 **行为变更**

#### **无聚合ID的消息处理**

| 项目 | 迁移前 | 迁移后 | 影响 |
|------|-------|-------|------|
| **并发模型** | 直接处理（无限制） | Actor Pool（256 并发） | 🟡 并发受限 |
| **负载均衡** | 无控制 | Round-Robin 轮询 | ✅ 更均衡 |
| **故障恢复** | 无 | Supervisor 自动重启 | ✅ 更可靠 |
| **顺序保证** | 无 | 无 | 🟢 无变化 |

**结论**: 行为变更**对用户透明**，且提升了可靠性。

---

## 代码影响分析

### 📝 **修改文件列表**

| 文件路径 | 修改类型 | 代码行数 | 影响范围 |
|---------|---------|---------|---------|
| `jxt-core/sdk/pkg/eventbus/nats.go` | 删除 + 修改 | ~250 行 | NATS EventBus 内部实现 |

---

### 🔍 **代码变更详情**

#### **1. 删除代码**（~200 行）

**删除内容**:
- `NATSWorkItem` 结构（~30 行）
- `NATSGlobalWorkerPool` 结构（~10 行）
- `NATSWorker` 结构（~5 行）
- `NewNATSGlobalWorkerPool()` 函数（~40 行）
- `SubmitWork()` 方法（~15 行）
- `start()` 方法（~15 行）
- `processWork()` 方法（~20 行）
- `Close()` 方法（~20 行）
- 初始化和清理代码（~10 行）
- 路由逻辑（~35 行）

**影响**: 代码复杂度降低，维护成本降低

---

#### **2. 修改代码**（~50 行）

**修改内容**:
- 添加 `roundRobinCounter` 字段（1 行）
- 修改 `handleMessage()` 方法（~25 行）
- 修改 `handleMessageWithWrapper()` 方法（~25 行）

**影响**: 路由逻辑统一，代码可读性提升

---

### 📊 **代码复杂度对比**

| 指标 | 迁移前 | 迁移后 | 变化 |
|------|-------|-------|------|
| **总代码行数** | ~3589 行 | ~3389 行 | -200 行 |
| **并发模型** | 2 种 | 1 种 | -1 种 |
| **结构体数量** | 3 个 | 0 个 | -3 个 |
| **方法数量** | 8 个 | 0 个 | -8 个 |

**结论**: 代码复杂度**显著降低**，维护成本降低。

---

## 性能影响分析

### 📊 **性能指标对比**

#### **1. 吞吐量**

| 场景 | 迁移前 | 迁移后（预期） | 变化 |
|------|-------|--------------|------|
| **低压(500)** | 33.25 msg/s | ~33 msg/s | ≈ 持平 |
| **中压(2000)** | 132.30 msg/s | ~132 msg/s | ≈ 持平 |
| **高压(5000)** | 163.71 msg/s | ~163 msg/s | ≈ 持平 |
| **极限(10000)** | 321.14 msg/s | ~321 msg/s | ≈ 持平 |

**结论**: 吞吐量预期**持平**。

---

#### **2. 延迟**

| 场景 | 迁移前 | 迁移后（预期） | 变化 |
|------|-------|--------------|------|
| **低压(500)** | 122.959 ms | ~123 ms | ≈ 持平 |
| **中压(2000)** | 316.595 ms | ~317 ms | ≈ 持平 |
| **高压(5000)** | 1418.267 ms | ~1418 ms | ≈ 持平 |
| **极限(10000)** | 2180.783 ms | ~2181 ms | ≈ 持平 |

**结论**: 延迟预期**持平**。

---

#### **3. 内存占用**

| 场景 | 迁移前 | 迁移后（预期） | 变化 |
|------|-------|--------------|------|
| **低压(500)** | 2.20 MB | ~2.20 MB | ≈ 持平 |
| **中压(2000)** | 2.71 MB | ~2.71 MB | ≈ 持平 |
| **高压(5000)** | 3.34 MB | ~3.34 MB | ≈ 持平 |
| **极限(10000)** | 4.32 MB | ~4.32 MB | ≈ 持平 |

**结论**: 内存占用预期**持平**。

---

#### **4. 协程数**

| 场景 | 迁移前 | 迁移后（预期） | 变化 |
|------|-------|--------------|------|
| **峰值协程数** | 动态（无限制） | 固定（256） | ✅ 更可控 |
| **协程泄漏** | 1 个 | 1 个 | ≈ 持平 |

**结论**: 协程数**更可控**，无泄漏风险降低。

---

### 🔍 **性能优化点**

1. **并发控制**: Actor Pool 限制并发数为 256，避免 goroutine 爆炸
2. **负载均衡**: Round-Robin 轮询均匀分配消息
3. **故障恢复**: Supervisor 机制自动重启，减少故障影响
4. **Inbox 缓冲**: 每个 Actor 有 1000 容量的 Inbox，减少阻塞

---

## 运维影响分析

### 📊 **监控指标变化**

#### **1. 删除的监控指标**

| 指标名称 | 说明 | 影响 |
|---------|------|------|
| `nats_worker_pool_queue_size` | Worker Pool 队列大小 | 🔴 删除 |
| `nats_worker_pool_worker_count` | Worker 数量 | 🔴 删除 |
| `nats_worker_pool_active_workers` | 活跃 Worker 数量 | 🔴 删除 |

**影响**: 需要删除相关监控和告警规则

---

#### **2. 新增的监控指标**

**Namespace**: `nats_eventbus_{clientID}` (例如: `nats_eventbus_my_service`)

| 指标名称 | 完整名称示例 | 说明 | 影响 |
|---------|------------|------|------|
| `actor_pool_messages_sent_total` | `nats_eventbus_my_service_actor_pool_messages_sent_total` | 发送到 Actor 的消息总数 | 🟢 新增 |
| `actor_pool_messages_processed_total` | `nats_eventbus_my_service_actor_pool_messages_processed_total` | Actor 处理的消息总数 | 🟢 新增 |
| `actor_pool_message_latency_seconds` | `nats_eventbus_my_service_actor_pool_message_latency_seconds` | 消息处理延迟（直方图） | 🟢 新增 |
| `actor_pool_inbox_depth` | `nats_eventbus_my_service_actor_pool_inbox_depth` | Actor Inbox 当前深度 | 🟢 新增 |
| `actor_pool_inbox_utilization` | `nats_eventbus_my_service_actor_pool_inbox_utilization` | Inbox 利用率（0-1） | 🟢 新增 |
| `actor_pool_actor_restarted_total` | `nats_eventbus_my_service_actor_pool_actor_restarted_total` | Actor 重启次数 | 🟢 新增 |
| `actor_pool_dead_letters_total` | `nats_eventbus_my_service_actor_pool_dead_letters_total` | 死信消息数 | 🟢 新增 |

**Label**: 所有指标都带有 `actor_id` label（例如: `actor_id="actor-0"` 到 `actor_id="actor-255"`）

**影响**: 需要添加新的监控和告警规则

---

#### **3. 保持不变的监控指标**

| 指标名称 | 说明 | 影响 |
|---------|------|------|
| `nats_published_messages` | 发布消息数 | 🟢 保持 |
| `nats_consumed_messages` | 消费消息数 | 🟢 保持 |
| `nats_error_count` | 错误计数 | 🟢 保持 |

**影响**: 无需修改

---

### 🔧 **告警规则更新**

#### **删除的告警规则**

```yaml
# ❌ 删除
- alert: NATSWorkerPoolQueueFull
  expr: nats_worker_pool_queue_size > 20000
  annotations:
    summary: "NATS Worker Pool queue is full"

- alert: NATSWorkerPoolHighUtilization
  expr: nats_worker_pool_active_workers / nats_worker_pool_worker_count > 0.9
  annotations:
    summary: "NATS Worker Pool utilization is high"
```

---

#### **新增的告警规则**

```yaml
# ✅ 新增（使用实际的指标名称）
# 注意：{namespace} 需要替换为实际的 namespace，例如 nats_eventbus_my_service

- alert: NATSActorPoolHighRestartRate
  expr: rate({namespace}_actor_pool_actor_restarted_total[5m]) > 10
  annotations:
    summary: "NATS Actor Pool restart rate is high"
    description: "Actor {{ $labels.actor_id }} restart rate is {{ $value }} restarts/sec"

- alert: NATSActorPoolInboxNearlyFull
  expr: {namespace}_actor_pool_inbox_utilization > 0.9
  annotations:
    summary: "NATS Actor Pool inbox is nearly full"
    description: "Actor {{ $labels.actor_id }} inbox utilization is {{ $value }}"

- alert: NATSActorPoolHighLatency
  expr: histogram_quantile(0.99, rate({namespace}_actor_pool_message_latency_seconds_bucket[5m])) > 1.0
  annotations:
    summary: "NATS Actor Pool message latency is high"
    description: "Actor {{ $labels.actor_id }} P99 latency is {{ $value }}s"

- alert: NATSActorPoolDeadLetters
  expr: rate({namespace}_actor_pool_dead_letters_total[5m]) > 1
  annotations:
    summary: "NATS Actor Pool has dead letters"
    description: "Actor {{ $labels.actor_id }} dead letter rate is {{ $value }} msgs/sec"
```

---

### 📝 **运维操作变更**

| 操作 | 迁移前 | 迁移后 | 影响 |
|------|-------|-------|------|
| **查看并发数** | 查看 Worker Pool 指标 | 查看 Actor Pool 指标 | 🟡 需要更新 |
| **查看队列大小** | 查看 Worker Pool 队列 | 查看 Actor Inbox | 🟡 需要更新 |
| **查看故障恢复** | 无 | 查看 Actor 重启次数 | 🟢 新增 |

---

## 风险缓解措施

### 🔴 **高风险**

无

---

### 🟡 **中风险**

#### **风险 1: 性能回归**

**风险描述**: 迁移后性能可能下降

**缓解措施**:
1. ✅ **性能测试**: 运行性能测试，验证吞吐量和延迟
2. ✅ **压力测试**: 运行高并发场景测试
3. ✅ **监控对比**: 对比迁移前后的监控指标
4. ✅ **回滚方案**: 准备 Git 回滚方案

**责任人**: 开发工程师 + 测试工程师

---

#### **风险 2: 监控指标变化**

**风险描述**: 监控指标变化可能导致告警失效

**缓解措施**:
1. ✅ **更新监控**: 删除旧指标，添加新指标
2. ✅ **更新告警**: 删除旧告警规则，添加新告警规则
3. ✅ **文档更新**: 更新运维文档
4. ✅ **培训**: 对运维团队进行培训

**责任人**: 运维工程师

---

### 🟢 **低风险**

#### **风险 3: 行为变更**

**风险描述**: 路由策略变更可能影响消息处理顺序

**缓解措施**:
1. ✅ **文档说明**: 明确说明无聚合ID的消息无顺序保证
2. ✅ **测试验证**: 运行功能测试，验证行为一致
3. ✅ **用户沟通**: 通知用户行为变更（如有必要）

**责任人**: 开发工程师

---

## 附录

### A. 监控指标映射表

| 迁移前指标 | 迁移后指标 | 说明 |
|-----------|-----------|------|
| `nats_worker_pool_queue_size` | `nats_actor_pool_inbox_size` | 队列大小 → Inbox 大小 |
| `nats_worker_pool_worker_count` | `nats_actor_pool_size` | Worker 数量 → Actor 数量 |
| `nats_worker_pool_active_workers` | `nats_actor_pool_active_actors` | 活跃 Worker → 活跃 Actor |
| 无 | `nats_actor_pool_restart_count` | 新增：Actor 重启次数 |

---

### B. 告警规则映射表

| 迁移前告警 | 迁移后告警 | 说明 |
|-----------|-----------|------|
| `NATSWorkerPoolQueueFull` | `NATSActorPoolInboxFull` | 队列满 → Inbox 满 |
| `NATSWorkerPoolHighUtilization` | 删除 | Actor Pool 固定大小，无需此告警 |
| 无 | `NATSActorPoolHighRestartRate` | 新增：Actor 重启率过高 |

---

### C. 运维文档更新清单

- [ ] 更新监控指标文档
- [ ] 更新告警规则文档
- [ ] 更新故障排查文档
- [ ] 更新性能调优文档
- [ ] 更新运维手册

---

### D. 相关文档

- [架构设计文档](./nats-subscribe-to-actor-pool-migration-architecture.md)
- [实施计划文档](./nats-subscribe-to-actor-pool-migration-implementation.md)
- [测试计划文档](./nats-subscribe-to-actor-pool-migration-testing.md)
- [Kafka Subscribe() 迁移文档](./kafka-subscribe-to-actor-pool-migration-index.md)

---

**文档状态**: 待评审  
**下一步**: 等待评审批准后，开始代码实施

