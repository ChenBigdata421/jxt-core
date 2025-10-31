# NATS JetStream 迁移方案修订总结

**修订日期**: 2025-10-31  
**修订人**: AI Assistant  
**状态**: 已完成  

---

## 📋 修订概览

本次修订基于对 GPT-5 评审意见的分析，对 NATS JetStream 迁移到 Actor Pool 的文档和代码进行了全面修正和增强。

---

## ✅ 已完成的修订

### 1. **P0 必须修正的问题**

#### 1.1 API 名称错误修正
- **问题**: 文档中使用了不存在的 `SubmitMessage()` 方法
- **修正**: 全部改为实际的 `ProcessMessage(ctx, msg) error`
- **影响文件**:
  - `nats-subscribe-to-actor-pool-migration-implementation.md`

#### 1.2 术语混淆修正
- **问题**: 文档声称"保持原 Worker Pool 轮询特性"，但实际上 Worker Pool 从未被使用
- **修正**: 改为"保持原 Subscribe 的并发无序特性（无聚合ID消息无顺序保证）"
- **影响文件**:
  - `nats-subscribe-to-actor-pool-migration-summary.md`
  - `nats-subscribe-to-actor-pool-migration-architecture.md`

#### 1.3 监控指标名称修正
- **问题**: 文档中的指标名与实际代码不符
- **修正**: 使用实际的指标名称模式 `{namespace}_actor_pool_{metric_name}`
- **实际指标**:
  - `nats_eventbus_{clientID}_actor_pool_messages_sent_total`
  - `nats_eventbus_{clientID}_actor_pool_messages_processed_total`
  - `nats_eventbus_{clientID}_actor_pool_message_latency_seconds`
  - `nats_eventbus_{clientID}_actor_pool_inbox_depth`
  - `nats_eventbus_{clientID}_actor_pool_inbox_utilization`
  - `nats_eventbus_{clientID}_actor_pool_actor_restarted_total`
  - `nats_eventbus_{clientID}_actor_pool_dead_letters_total`
- **影响文件**:
  - `nats-subscribe-to-actor-pool-migration-impact.md`

---

### 2. **Subscribe/SubscribeEnvelope 语义差异说明**

#### 2.1 文档补充
- **新增内容**: 在架构文档中添加了详细的语义差异对比表
- **关键区别**:

| 特性 | Subscribe | SubscribeEnvelope |
|------|-----------|-------------------|
| **消息格式** | 原始字节（[]byte） | Envelope 包装（含 AggregateID） |
| **JetStream Storage** | **Memory**（内存存储） | **File**（磁盘持久化） |
| **失败处理** | 不 ack，不重投 | 显式 Nak，立即重投 |
| **交付语义** | **at-most-once** | **at-least-once** |
| **适用场景** | 临时消息、通知、非关键数据 | 领域事件、关键业务数据 |
| **性能** | 更快（内存存储） | 较慢（磁盘 I/O） |
| **可靠性** | 低（进程重启丢失） | 高（磁盘持久化） |

- **影响文件**:
  - `nats-subscribe-to-actor-pool-migration-architecture.md`

#### 2.2 代码实现
- **修改**: `nats.go` 中的 `subscribeJetStream()` 方法
- **实现逻辑**:
  ```go
  func (n *natsEventBus) subscribeJetStream(ctx context.Context, topic string, handler MessageHandler, isEnvelope bool) error {
      var storageType nats.StorageType
      if isEnvelope {
          // SubscribeEnvelope: 使用 file storage（at-least-once）
          storageType = nats.FileStorage
      } else {
          // Subscribe: 使用 memory storage（at-most-once）
          storageType = nats.MemoryStorage
      }
      return n.subscribeJetStreamWithStorage(ctx, topic, handler, isEnvelope, storageType)
  }
  ```
- **新增方法**:
  - `subscribeJetStreamWithStorage()`: 使用指定存储类型订阅
  - `ensureTopicStreamExists()`: 为每个 topic 创建专用 Stream，使用指定存储类型
- **影响文件**:
  - `jxt-core/sdk/pkg/eventbus/nats.go`

---

### 3. **增强测试计划**

#### 3.1 新增测试场景

##### TC-F6: Subscribe/SubscribeEnvelope 存储类型验证
- **目标**: 验证 Subscribe 使用 memory storage，SubscribeEnvelope 使用 file storage
- **验证点**:
  - topic-memory 的 Stream 使用 MemoryStorage
  - topic-file 的 Stream 使用 FileStorage
  - Subscribe 消息失败不重投（at-most-once）
  - SubscribeEnvelope 消息失败 Nak 重投（at-least-once）

##### TC-F7: 混合场景测试
- **目标**: 验证同时发送有聚合ID和无聚合ID的消息时的正确性
- **验证点**:
  - 有聚合ID消息按聚合ID路由到同一 Actor
  - 无聚合ID消息使用 Round-Robin 路由
  - 所有消息都被接收，无消息丢失

##### TC-F8: Actor 重启恢复测试
- **目标**: 验证 Actor panic 后自动重启，消息重投
- **验证点**:
  - Actor panic 后自动重启
  - 消息重投并成功处理
  - Actor 重启计数器增加
  - 监控指标正确记录

##### TC-F9: Inbox 满时的背压测试
- **目标**: 验证 Actor Inbox 满时的背压行为
- **验证点**:
  - Inbox 满时，ProcessMessage 返回错误或阻塞
  - 消息不会丢失（JetStream 重投）
  - Inbox 满计数器增加
  - 监控指标正确记录

- **影响文件**:
  - `nats-subscribe-to-actor-pool-migration-testing.md`

---

### 4. **实施方案更新**

#### 4.1 工作量估算更新
- **修改前**: 5.5 小时
- **修改后**: 8 小时（增加了存储类型区分的实现）

#### 4.2 新增实施步骤
- **步骤 0**: 区分 Subscribe/SubscribeEnvelope 的存储类型
  - 修改 `Subscribe()` 方法，强制使用 memory storage
  - 修改 `SubscribeEnvelope()` 方法，强制使用 file storage
  - 新增 `subscribeJetStreamWithStorage()` 方法
  - 新增 `ensureTopicStreamExists()` 方法

- **影响文件**:
  - `nats-subscribe-to-actor-pool-migration-implementation.md`

---

## 🔍 关键技术决策

### 1. **为每个 topic 创建专用 Stream**
- **原因**: 不同 topic 需要不同的存储类型（memory vs file）
- **实现**: Stream 名称格式为 `{base_stream_name}_{topic_suffix}`
- **优点**:
  - 灵活控制每个 topic 的存储类型
  - 隔离不同 topic 的数据
  - 便于监控和管理
- **缺点**:
  - 增加 Stream 数量
  - 需要更多资源

### 2. **存储类型不可变**
- **NATS 限制**: 已存在的 Stream 无法修改存储类型
- **处理策略**: 如果 Stream 已存在但存储类型不匹配，记录警告但继续使用
- **建议**: 在生产环境部署前，确保删除旧的 Stream 或使用新的 Stream 名称

### 3. **直接删除 Worker Pool 代码**
- **原因**: Worker Pool 代码从未被使用（死代码）
- **策略**: 无需两阶段删除，直接删除即可
- **风险**: 无（因为从未被调用）

---

## 📊 修改统计

### 文档修改
| 文件 | 修改类型 | 修改行数 |
|------|---------|---------|
| `nats-subscribe-to-actor-pool-migration-summary.md` | 术语修正 | ~20 行 |
| `nats-subscribe-to-actor-pool-migration-architecture.md` | 术语修正 + 新增语义说明 | ~50 行 |
| `nats-subscribe-to-actor-pool-migration-implementation.md` | API修正 + 新增实施步骤 | ~80 行 |
| `nats-subscribe-to-actor-pool-migration-testing.md` | 新增测试场景 | ~210 行 |
| `nats-subscribe-to-actor-pool-migration-impact.md` | 监控指标修正 | ~30 行 |
| **总计** | | **~390 行** |

### 代码修改
| 文件 | 修改类型 | 修改行数 |
|------|---------|---------|
| `jxt-core/sdk/pkg/eventbus/nats.go` | 新增方法 + 逻辑修改 | ~70 行 |
| **总计** | | **~70 行** |

---

## ✅ 验证清单

- [x] P0 问题全部修正
- [x] Subscribe/SubscribeEnvelope 语义差异已说明
- [x] 代码实现支持存储类型区分
- [x] 测试计划已增强
- [x] 监控指标名称已修正
- [x] 实施方案已更新
- [x] 所有文档一致性检查通过

---

## 🚀 下一步建议

1. **代码审查**: 请团队成员审查 `nats.go` 的修改
2. **测试验证**: 运行新增的测试场景，验证存储类型区分
3. **性能测试**: 验证 memory storage 的性能提升
4. **文档审查**: 请技术写作团队审查文档修订
5. **部署计划**: 制定生产环境部署计划（注意 Stream 存储类型迁移）

---

## 📝 备注

- 本次修订完全基于用户需求和 GPT-5 的评审意见
- 所有修改都经过代码编译验证，无语法错误
- 文档和代码保持一致性
- 测试计划覆盖了所有关键场景


