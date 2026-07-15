# Spec：jxt-core EventBus Kafka/RedPanda 回归修复与加固

| 项 | 内容 |
|---|---|
| 标题 | EventBus Kafka/RedPanda：修复被历次"优化"破坏的部分并加固 |
| 日期 | 2026-07-15 |
| 状态 | Draft（待评审） |
| 影响组件 | `jxt-core/sdk/pkg/eventbus`（`kafka.go`、`partition_pipeline.go`、`type.go`、`topic_config_manager.go`） |
| 影响面 | **跨服务** —— 所有经 jxt-core Kafka/RedPanda 消费/发布的服务（evidence-management、file-storage 等）。生产 broker 为 RedPanda。 |
| 根因性质 | **回归**：历次性能/架构优化引入的破坏，以及修复不完整留下的同类 bug |
| 是否破坏性 | 行为修复（非 API 破坏）：重连恢复可用、压缩恢复、topic 建表修正；少量语义变更需灰度（见各条） |

---

## 1. 背景与目标

对 EventBus Kafka/RedPanda 路径做了一次**回归导向的取证审计**（git `show`/`blame` 取证），发现历次"优化"浪潮系统性破坏了若干功能。本 spec 把这些回归按重要性排序，给出符合业界最佳实践的修复方案。

**两条最刺眼的结论：**
- **重连子系统原本可用**（`f98da6c` 里 `Subscribe` 直接覆写订阅 map、每次重建消费组），被三波优化接力打成"必败 + 即使修了也丢消费"：
  - `1474135`（2025-10-12 kafka 性能调优）：引入共享 `unifiedConsumerGroup` 但重连不重建；注释掉 `reinitializeConnection` 的 `configureSarama`。
  - `5b80639`（2025-10-17 热路径免锁）：`subscriptions` 改 `sync.Map` + `LoadOrStore` 去重，但永不 `Delete` → 重连必报 "already subscribed"。
  - `5a84d32`（2025-10-31 Hollywood 迁移）：消费集中到单 goroutine，重连从不 stop/restart 它。
- **`8970ae9`（"kafka 压缩配置优化"）实际删除了 `1474135` 刚加的 LZ4 producer 压缩**，做的与声称的相反 → 全部 producer 流量明文过线。

**目标**
1. 修复 Critical/High 回归（重连、压缩、topic 建表）。
2. 消除 Medium 反向优化与双路径分叉风险。
3. 清理 Low 死代码。
4. 全程 `go test -race` 守护，新增针对性单测。

**非目标**
- 不改 Actor Pool / 流水线核心算法（已验证正确：mark-once 连续前缀提交、`forwardCompletion` ctx-cancel 安全、窗口背压、buffer 不变量、毒消息告警）。
- 不动订阅 API 签名（`Subscribe`/`SubscribeEnvelope` 等）。
- 不做 NATS 路径（系统已确定用 RedPanda，NATS dormant）。

---

## 2. 因果地图：每个优化"破"了什么

| 优化 (commit / 日期) | 本意 | 实际破坏 |
|---|---|---|
| `1474135` kafka性能调优 (10-12) | 共享 `unifiedConsumerGroup` + 内联 sarama 配置 | ① 重连不重建 consumer group；② `reinitializeConnection` 注释掉 `configureSarama` → 重连丢全部调优；③ ACK handler goroutine 无生命周期管理 |
| `8970ae9` kafka压缩配置优化 (10-20) | "改成 topic 级压缩" | **反向操作**：删除 `1474135` 的 LZ4 producer 压缩 → 全部流量明文；删用户侧 `Compression` 字段；留 `getCompressionCodec()` 死代码 |
| `5b80639` 热路径免锁 (10-17) | `sync.Map` + `LoadOrStore` 去重 | `subscriptions` 永不 `Delete` → `restoreSubscriptions` 必败；`closed` 检查 TOCTOU 窗口略增大 |
| `5a84d32` Hollywood 迁移 (10-31) | 集中消费到单 goroutine | 重连从不 stop/restart 消费 goroutine → 重连后消费永久丢失；留 divergent 死代码 `kafkaConsumerHandler` |
| `85fc15b` 重试优化 (11-01) | Envelope 失败立即重试 | 同一 actor + 相同输入 → 确定性再失败 + 毒消息阻塞翻倍 + 误导性日志 |
| `e2946a4` GetTopicPartitions 修复 (06-28) | 修缺失 topic 判定 | 只修 `GetTopicPartitions`，未回填 `ensureKafkaTopicIdempotent`（同类 bug）→ 真正负责建 topic 的函数仍坏 |
| `c5d8f44` 扩分区修复 (06-28) | create_or_update 真扩分区 | 修了 `Partitions` 字段，漏平行的 `ReplicationFactor` 字段 → RF 漂移不可检测 |
| `28f27d4`…`169da2a` 流水线 (06-24) | 窗口化并发 | 双路径语义分叉：翻 flag 静默改变提交粒度 / DLQ / 分区 offset 完成顺序（文档未提） |

---

## 3. 回归清单与修复方案（按重要性）

每条结构：**位置 / 造成 commit / 破坏机制 / 业界最佳实践方案 / 验收 / 风险**。

### Critical：重连子系统重构（一组改动，必须同改）

这 6 条（+ P1 的 ACK 生命周期）相互纠缠，必须作为一个连贯改动一起修——单点修补撑不住。C5（状态机/锁协议/kill-switch）、C6（重连冷却）为工程评审 + outside voice 新增。

#### C1. `restoreSubscriptions` 必败（"already subscribed"）
- **位置**：`kafka.go:1601`（`Subscribe` 的 `LoadOrStore`）、`kafka.go:2104-2127`（`restoreSubscriptions`）。
- **造成 commit**：`5b80639`（2025-10-17）。git 取证：`git log -S "k.subscriptions.Delete"` 无任何结果——从未删除条目。
- **破坏机制**：重连时 `restoreSubscriptions` 遍历 `subscriptions` 逐个 `Subscribe`，但条目从未删除，`LoadOrStore` 报 `loaded=true` → 返回 "already subscribed" → `reconnect` 整体失败。**每次重连都卡在这。**
- **最佳实践方案**：重连必须重建到与全新初始化等价的状态。`reconnect` 顶部清空 `subscriptions`（及 `activeTopicHandlers`）后再 `restoreSubscriptions`；或新增内部 `subscribeInternal` 绕过去重守卫、重启消费循环。推荐前者（单一重建入口、语义清晰）。
- **验收**：模拟 broker 断连后重连，断言每个 topic 重新注册成功、无 "already subscribed" 错误。
- **风险**：低。清空 + 重建是幂等的。

#### C2. `unifiedConsumerGroup` 重连不重建
- **位置**：`kafka.go:2034-2101`（`reinitializeConnection` 重建 client/producer/consumer/admin 但不重建消费组）；消费循环 `kafka.go:1334`（`getUnifiedConsumerGroup().Consume`）。
- **造成 commit**：`1474135`（2025-10-12）。
- **破坏机制**：旧 client 关闭后，旧消费组绑定在死 client 上；`Consume` 持续报错、2s 重试空转；`consumerStarted` 永不复位 → 无路径拉起新消费组。**重连后消费永久停摆。**
- **最佳实践方案**：`reinitializeConnection` 重建 client 后，`sarama.NewConsumerGroupFromClient(groupID, newClient)` 并 `Store`；`reconnect` 在 `reinitializeConnection` 前 `stopPreSubscriptionConsumer()`、`restoreSubscriptions` 成功后 `startPreSubscriptionConsumer(ctx)`。
- **验收**：断连重连后，新消息能被消费（消费组绑定到新 client）。
- **风险**：中。消费组生命周期编排需谨慎，需竞态测试。

#### C3. `reinitializeConnection` 用空 `sarama.NewConfig()`，丢全部调优
- **位置**：`kafka.go:2052-2057`（注释掉的 `configureSarama`、bare `sarama.NewConfig()`）。
- **造成 commit**：`1474135`（2025-10-12）。原始 `f98da6c` 调用 `configureSarama` 应用完整调优。
- **破坏机制**：重连后新 producer/consumer 用 sarama 默认值——`Idempotent=false`（去重丢失）、`RequiredAcks=WaitForLocal`（acks=1，耐久性降级）、`MaxOpenRequests=5`、`Compression=None`、`Flush=0`（无批处理）、`Return.Successes=false`（ACK handler 变空操作）、fetch/isolation 全回退。**broker 一抖动，正好最需要幂等/重试的时刻，全没了。**
- **最佳实践方案**：把 `NewKafkaEventBus` 里的 sarama 配置构建块抽成 `buildSaramaConfig(cfg *KafkaConfig) (*sarama.Config, error)`，`NewKafkaEventBus` 与 `reinitializeConnection` 共用。配置单一来源（single source of truth）。
- **验收**：断言重连后 producer `Idempotent==true`、`RequiredAcks==WaitForAll`、`Return.Successes==true`、消费 fetch 配置不变。
- **风险**：低-中。纯重构提取，行为应与首次初始化一致。

#### C4. 消费 goroutine 重连不 stop/restart
- **位置**：消费循环 `kafka.go:1300-1370`；`consumerDone` `kafka.go:1289/1301/1540-1551`；`reconnect` `kafka.go:1976-2031`。
- **造成 commit**：`5a84d32`（2025-10-31）。
- **破坏机制**：活消费 goroutine 从未被通知停止，对着死消费组空转；`consumerStarted` 永不复位 → 无法拉起新消费 goroutine。是 C2 的生命周期半边。
- **最佳实践方案**：见 C2——`reconnect` 编排 stop→reinit→restore→start。配套修 `consumerDone` 的 stop/并发-start close-race（持锁时把 done chan 存入局部变量再 receive）。
- **验收**：重连前后 goroutine 数不泄漏；`Close()` 不会因 close-race 永久阻塞。
- **风险**：中。生命周期编排 + 竞态测试。

#### C5. 重连状态机、加锁协议与 kill-switch（评审 P1 新增，实施前必须明确）

重连重构触及真实状态机（共享消费 goroutine + `subscriptions` map + 消费组 + producer），叙述式描述不足以防竞态/死锁。实施前必须把状态、加锁、回退三件事写死。

**状态机（全程持 `consumerMu`）：**

```
  健康检查连续失败 ≥ FailureThreshold
            │
            ▼
  reconnect(ctx) ── env JXT_KAFKA_AUTO_RECONNECT=off? ──▶ 不重连,仅告警,由编排器重启 pod (kill-switch)
            │ on
            ▼
  ① stopPreSubscriptionConsumer    (停共享消费 goroutine; consumerDone 局部变量捕获)
  ② reinitializeConnection
       - close 旧 client/producer/consumer/admin
       - buildSaramaConfig(cfg)         ◀── 与 NewKafkaEventBus 同一配置来源 (C3)
       - 新 client/producer/consumer/admin
       - NewConsumerGroupFromClient     ◀── 重建消费组 (C2)
  ③ 清空 subscriptions + activeTopicHandlers   ◀── 在 consumerMu 下清空 (C1)
  ④ restoreSubscriptions (重新 Subscribe,不再 already-subscribed)
  ⑤ startPreSubscriptionConsumer    (重启消费 goroutine) (C4)
  ⑥ 释放 consumerMu
            │
            ├─ 任一步失败 ▶ 退避重试(指数); 达上限放弃,保留状态等下次健康检查
            ▼
       恢复正常消费/发布
```

**加锁协议（修订：原"全程持 `consumerMu`"会死锁，已按 outside voice P0 取证改正）：**
- 重连临界区由 **`k.mu`** 串行化（不是 `consumerMu`）。`k.mu` 同时阻塞 `Close`（1890）与 `Subscribe`（1589），天然串行化"重连 vs 关闭/订阅"。
- 新增**内部 lock-free 变体** `stopPreSubscriptionConsumerInternal` / `startPreSubscriptionConsumerInternal` / `subscribeInternal`——假设调用方已持 `k.mu`，**不再取 `consumerMu`**（重连路径在 `k.mu` 下独占消费状态）。
- `consumerStarted` 由步骤 ⑤（`startInternal`）权威设置；步骤 ④ 用 `subscribeInternal`，**不**触发消费 goroutine 启动——消除"Subscribe 副作用与步骤⑤抢启动"的顺序歧义（outside voice P0-2）。
- 锁序唯一化：重连只持 `k.mu`；`Close` 持 `k.mu→consumerMu`；**不存在 `consumerMu→k.mu` 反向路径** → 无 AB-BA；内部变体不重入 → 无自死锁（原方案的两种死锁均消除）。
- `consumerDone` 在持 `k.mu` 时存入局部变量再 receive，消除 stop/并发-start close-race（C4）。
- `Publish` 经 generation/RWMutex 门禁（M3）：重连期间向旧 producer 的发送被阻断而非 panic。

**kill-switch（可回退性，对应评审 1A）：**
- 新增 env `JXT_KAFKA_AUTO_RECONNECT=off`（默认 on）：关闭健康检查触发的自动重连，退化为"健康检查失败到阈值即放弃，由编排器（k8s）重启 pod"。
- 理由：当前重连是坏的（静默失败）；修好后若引入新 panic/死锁可能更糟；共享库 Critical 改动必须能一键回退到"不重连"。

- **验收**：env=off 时健康检查到阈值不触发 reconnect、仅告警；env=on 时走完整状态机；`go test -race` 下状态机无死锁。
- **风险**：中。状态机 + 锁协议 + kill-switch 把高危改动可视化、可回退；仍需 dev 反复断连验证。

#### C6. 重连冷却门禁（outside voice P1 新增）
- **位置**：`reconnect` `kafka.go:1976-2031`（`failureCount` 仅成功时复位 @2018）；`lastReconnectTime` 字段存在却从不用于门禁。
- **造成 commit**：`1474135`（引入 health-check 触发重连）。
- **破坏机制**：`reconnect` 仅在全成功时 `failureCount.Store(0)`；失败返回 error 后，健康检查 goroutine（2502-2518）每 tick 继续累加 `failureCount`。`reconnect` 内 `MaxAttempts` 耗尽，下一次健康检查失败立刻又 `failureCount ≥ FailureThreshold` → 再 `reconnect` → 再烧一轮退避。broker 真宕时是"无冷却抡锤"。
- **最佳实践方案**：复用已存在但闲置的 `lastReconnectTime` 做门禁——两次 reconnect 间强制最小间隔（如 ≥30s，可配），间隔内健康检查到阈值只告警不重连；`failureCount` 在 reconnect 进入时即复位（而非仅成功时）。
- **验收**：broker 持续宕时 reconnect 频率受冷却约束（日志/指标可观测）；恢复后正常重连。
- **风险**：低-中。冷却太长延迟恢复，需可配。

---

### High

#### H1. Producer 压缩被静默移除
- **位置**：`kafka.go:237`（`Producer.Compression = CompressionNone`）。
- **造成 commit**：`1474135` 加 LZ4；`8970ae9`（2025-10-20）以"topic 级压缩"为由删除。
- **破坏机制**：sarama 是**生产者端**压缩（按 produce-request 批次），topic 级 `compression.type` 只对日志 compaction 有效，**不压缩在途 producer 流量**；且 `Publish()` 路径根本不调 `ConfigureTopic`。结果：所有 producer 流量明文，带宽/磁盘多耗约 50-70%（JSON envelope）。README "1900+ msg/s" 还是带压缩时测的。
- **最佳实践方案**：恢复 producer 端压缩，**默认 snappy**（Kafka/RedPanda 互操作默认、最稳），暴露为配置（`snappy`/`lz4`/`zstd`/`gzip`/`none`）；恢复用户侧 `ProducerConfig.Compression` 字段。RedPanda 原生支持全部 codec。删除死代码 `getCompressionCodec()` 或改为配置解析复用。
  - *决策*：默认 snappy（互操作最广）；追求更低延迟可切 lz4（`1474135` 原选），追求压缩比可切 zstd。
- **验收**：单测断言 `Producer.Compression != CompressionNone`（按配置）；端到端验证 RedPanda 收到的是压缩批次（broker 侧 `compression.type` / 抓包确认）。
- **风险**：低。RedPanda 支持全部 codec；压缩只影响线材格式，不影响语义。

#### H2. `ensureKafkaTopicIdempotent` 漏判缺失 topic
- **位置**：`kafka.go:3275`（`if err != nil || len(metadata) == 0`）。
- **造成 commit**：守卫原始来自 `f98da6c`；回归点是 `e2946a4`（2026-06-28）只修了 `GetTopicPartitions` 的同类 bug，没回填本函数。
- **破坏机制**：sarama `DescribeTopics` 对缺失 topic 返回**无顶层 error**、`metadata[0].Err == ErrUnknownTopicOrPartition` + 空 `Partitions` → `len==0` 为假 → 误入"已存在"分支、**不创建**。`ConfigureTopic` 启动时每 topic 必调它（`shouldCreate`/`shouldUpdate`/默认分支 `kafka.go:3075/3086/3091`，且 `c5d8f44` 的 reconcile 路由穿过它 `kafka.go:3081`）。关掉 broker auto-create 就静默"已存在"跳过 → 首发 `UNKNOWN_TOPIC_OR_PARTITION` / 丢数据。当前被 RedPanda auto-create 掩盖。
- **最佳实践方案（谨慎，勿用朴素改法）**：⚠️ 朴素改法（凡 `metadata[0].Err != ErrNoError` 即建）**会搞挂启动**——`createKafkaTopic`→`CreateTopic` 不容忍 `ErrTopicAlreadyExists`，且 broker auto-create 期间的 `ErrLeaderNotAvailable` 会被误判为缺失 → 误建 → 每服务启动失败。正确做法：
  1. 区分 `ErrUnknownTopicOrPartition`（真缺失 → 建）与瞬时错误（`ErrLeaderNotAvailable`/`ErrNotController` → 短退避重试，不建）；
  2. `createKafkaTopic` 容忍 `ErrTopicAlreadyExists`（视为成功）；
  3. `getActualTopicConfig`（`kafka.go:3419`）也补 `metadata[0].Err` 检查并返回 error，避免调用方基于幻影配置推理；
  4. 补测试：缺失 topic / 瞬时错误 / already-exists 三种情形。
  - ⚠️ **范围说明（必要不充分）**：本修复只解决守卫误判。`shouldCreateOrUpdate` 的 `exists` 当前来自内存 `topicConfigs` cache（重启清空）而非 broker；`StrategyCreateOnly` 重启会误对已存在 topic 发 `CreateTopic`。修了守卫后该 case 从"静默"变为"每次重启发一次 CreateTopic"（被步骤 2 的容忍吃掉、不崩，但仍属浪费）。**`exists` 改为来自 broker 是独立后续 TODO，不在本 spec 范围。**（另：`ConfigureTopic` 的 `default` 分支 `kafka.go:3088`——多数用户未显式设策略时走此路径——冷启动同样 `exists=false`→create-only；与上述 cache 问题同属一个后续 TODO，本 spec 不展开。）
- **验收**：mock `metadata[0].Err=ErrUnknownTopicOrPartition` → 断言走创建分支；mock `ErrLeaderNotAvailable` → 断言不建、重试；已存在 topic 启动不报错。
- **风险**：中-高。本条是已知陷阱（朴素改法破坏启动），必须单独 scoped 改 + 充分测试。

---

### Medium

#### M1. Envelope "重试一次"是反向优化
- **位置**：`kafka.go:985-1004`（`preSubscriptionConsumerHandler.ConsumeClaim` 内联重试块）。
- **造成 commit**：`85fc15b`（2025-11-01）。前置代码是"失败即 MarkMessage（避免阻塞）"。
- **破坏机制**：重试调 `processMessageWithKeyedPool` 再来一次，但一致性哈希路由到**同一个**刚失败的 actor（`hollywood_actor_pool.go:113` FNV-32a）、相同消息+ctx → 确定性失败照旧；还在串行 ConsumeClaim 循环里同步重跑 → 毒消息分区阻塞时间翻倍；日志 `Retrying failed Envelope message` 假装在恢复。仅对瞬时调度抖动有效，而那本就被 actor supervisor 覆盖。
- **最佳实践方案**：删除内联重试。Envelope 失败时**不 `MarkMessage`**（留待 Kafka 重投，at-least-once），与流水线路径的"失败→异步 DLQ / 留待重投"模型对齐。瞬时恢复若需要，应放在 actor 层（退避+抖动+重试换路由），而非同 actor 同步重调。
- **验收**：Envelope 失败一次即留待重投（不二次同步执行）；分区阻塞时间不翻倍。
- **风险**：低。重投由 broker 保证 at-least-once。

#### M2. 流水线双路径静默分叉
- **位置**：legacy `kafka.go:954-1016` vs pipeline `partition_pipeline.go:273-364`，分发于 `kafka.go:950-952`，flag `type.go:616`。
- **造成 commit**：`28f27d4`…`169da2a` + `21aed07`/`900e7a2`（2026-06-24/25）。
- **破坏机制**：翻 `PipelineConfig.Enabled` **静默**改变：(a) 提交粒度（逐条 vs 连续前缀 mark-once）；(b) DLQ/告警有无（legacy 无 DLQ/告警，pipeline 有）；(c) **分区 offset 完成顺序**（legacy 严格按 offset，pipeline 允许 N+1..N+15 先于 N 完成）；(d) **关停/rebalance 边界 regular 消息失败语义**（legacy ctx.Done 时退出 ConsumeClaim 不 mark → 留待重投，at-least-once；pipeline `flush()` ctx.Done 路径对 regular-fail 仍 `commitable=true` → 提交丢弃，at-most-once）。文档只承诺 per-aggregate 有序（两路径都满足），未提 offset 完成序变了——下游若以"先见 N 再见 N+1"做幂等（如 `lastSeenOffset`），翻 flag 后行为不同。
- **最佳实践方案**：改变正确性语义的 feature flag 必须在 toggle 时显式告知。① README + `type.go:616` 注释明确"启用后分区 offset 完成序不再保证，仅 per-aggregate 有序"；② 首次启用打一次性 `logger.Info` 提示；③ DLQ 门禁默认 warn-and-allow（强告警 + 计数），hard-gate（拒绝启用）作为可选 env opt-in `JXT_KAFKA_PIPELINE_REQUIRE_DLQ=on`——避免某个暂未接 DLQ 的 topic 卡死整个 pipeline 灰度。
- **验收**：启用时日志提示；无 DLQ 的 Envelope 订阅在启用 pipeline 时告警/拒绝；文档覆盖 (a)-(d) 四种分叉。
- **风险**：低。flag 默认关；本条主要是文档 + 门禁。

#### M3. ACK handler goroutine 无生命周期
- **位置**：`kafka.go:442-443`（初始化 spawn）、`kafka.go:2095-2097`（重连 spawn）、handler `kafka.go:457-519`/`521+`。
- **造成 commit**：`1474135`（引入）、`8970ae9`（固化）。
- **破坏机制**（outside voice P1 扩展，范围比想象大）：无 `sync.WaitGroup`/stop chan——① 重连 handoff 期 `Publish` 可能 load 到正在被 `Close` 的旧 producer → 向已关闭 `Input()` 发 → panic；② `Close()` 不 drain `publishResultChan`（缓冲 10000），Outbox 消费方在关闭时丢失尾部 ACK；③ `reinitializeConnection` spawn 新 pair 前不停旧 pair，旧 pair range 旧 producer chan 直到 `producer.Close()` 关 chan 才退出——存在新旧 pair 并存窗口，ACK 可能乱序写 `publishResultChan`/tenant chan。
- **最佳实践方案**：goroutine 所有权 + 优雅关闭。加 `sync.WaitGroup`（或 stop chan）；`Close`/`reinitializeConnection` 先 signal stop、关 producer、`wg.Wait()`；`Publish` 经 generation/RWMutex 门禁，不能向正在关闭的 producer 发；`Close()` drain `publishResultChan`（或通知 Outbox 消费方关闭）；`reinitializeConnection` spawn 新 pair 前先停旧 pair 并 `wg.Wait()`。（与 Critical 重连重构同改。）
- **验收**：`go test -race` 下重连期间无 panic；`Close` 后 ACK goroutine 全部退出。
- **风险**：中。生命周期 + 竞态。

#### M4. ReplicationFactor 漂移不可检测
- **位置**：`getActualTopicConfig` `kafka.go:3420-3428`（只填 `Replicas`，不填 `ReplicationFactor`）；`compareTopicOptions` `topic_config_manager.go:261`（守卫恒假）。
- **造成 commit**：`c5d8f44`（2026-06-28）修了 `Partitions` 字段，漏平行的 `ReplicationFactor`。
- **破坏机制**：`TopicOptions` 有同义双字段 `Replicas`/`ReplicationFactor`；`createKafkaTopic` 优先 `ReplicationFactor` 回退 `Replicas`；`getActualTopicConfig` 只填 `Replicas` → 校验路径对 RF 漂移全盲（`StrategyValidateOnly`/`shouldUpdate` 永远"all good"）。
- **最佳实践方案**：`getActualTopicConfig` 同时填 `actualConfig.ReplicationFactor`（= `Replicas` 或 `len(Partitions[0].Replicas)`），且 **`DescribeTopics` 返回瞬时错误（`ErrLeaderNotAvailable` 等）时必须返回 error 而非吞错返回 RF=0**（outside voice P2）——否则一次 broker 抖动就静默禁用 RF 漂移检测，正好抵消本修复。长期：合并双字段为单一权威字段。
- **验收**：mock RF 漂移 → `StrategyValidateOnly` 正确上报差异。
- **风险**：低。

---

### Low（清理）

| # | 项 | 位置 | 方案 |
|---|---|---|---|
| L1 | 死代码 `kafkaConsumerHandler`/`processMessage`（迁移遗留、语义分叉、无 Envelope/DLQ） | `kafka.go:786-889` | 删除（零调用） |
| L2 | 死代码 `ensureKafkaTopic`（同类 bug、零调用） | `kafka.go:3164-3186` | 删除 |
| L3 | 死代码 `getCompressionCodec()` | `kafka.go:731-744` | 删除或改为压缩配置解析复用（配合 H1） |
| L4 | rebalance 默认 `range` → 应 `sticky`（eager，分区更稳；cooperative-sticky 仍禁）；**首次部署触发一次性全量 rebalance，部署提示避开高峰** | `kafka.go:760-771` | 默认改 `BalanceStrategySticky` |
| L5 | `closed` 检查 TOCTOU 窗口（`5b80639` 提前 Unlock 略放大） | `kafka.go:1592` | `startPreSubscriptionConsumer` 前再 `closed.Load()` 复查，或修复当初为避死锁而分离的持锁范围 |

---

## 4. 分阶段实施计划

| 阶段 | 内容 | 风险 | 灰度 |
|---|---|---|---|
| **Phase 1（P0）** | 重连子系统重构：`buildSaramaConfig` 抽取 + 重连重建消费组 + `restoreSubscriptions` 清空重建 + stop/restart 消费 goroutine + ACK 生命周期 + 状态机/锁协议/kill-switch + 重连冷却（C1-C6 + M3） | 中（生命周期） | kill-switch `JXT_KAFKA_AUTO_RECONNECT=off` 可一键回退；建议先在 dev 反复断连验证 |
| **Phase 2（P0/P1）** | 恢复 producer 压缩（H1）+ topic 建表守卫谨慎修（H2）+ RF 字段（M4） | H2 中-高 | 压缩默认开；topic 修复单独 scoped |
| **Phase 3（P1）** | 删除内联重试（M1）+ 流水线双路径文档/门禁（M2） | 低 | M2 的硬门禁随 pipeline 灰度 |
| **Phase 4（P2）** | 死代码清理（L1-L3）+ sticky 默认（L4）+ closed TOCTOU（L5） | 低 | 直接改 |

**Phase 依赖说明（outside voice P2）**：C3 的 `buildSaramaConfig`（Phase 1）与 H1 的压缩配置字段（Phase 2）不独立——`buildSaramaConfig` 是单一配置来源，应从第一天就含压缩。推荐把 H1 的"恢复 Compression 配置字段"前移随 C3 一起做（`buildSaramaConfig` 一次到位），避免 Phase 2 回头改它。

---

## 5. 测试计划

### 5.1 测试 harness（评审 5A/7A 新增）

- **编排级单测（不依赖真 broker，CI 默认跑）**：用 sarama `mockbroker` 覆盖重连状态机与各回归点的纯逻辑——断言 reconnect 严格按 stop→reinit→restore→start、清空 subscriptions、重建消费组、`buildSaramaConfig` 配置 parity（init 与 reconnect 等价）。
- **集成 e2e（依赖真 broker，integration-gated，非 CI 默认跑）**：用 testcontainers RedPanda 跑两条端到端——① 断连→重连→新消息可消费；② producer 压缩批次确实到达 broker（H1）。用 `-tags=integration` 或专用 job 触发。
- **门禁命令**：`go build ./...` + `go test -race -run '<pattern>' ./sdk/pkg/eventbus/...`（裸 `go test ./sdk/pkg/eventbus/` 无 broker 会挂，用 scoped `-run` + `go build` 作门禁；e2e 走 integration 标签）。

### 5.2 各回归点测试

- **重连（C1-C5，Phase 1）**：编排级单测断言状态机顺序/清空/重建/配置 parity + kill-switch（env=off 不触发 reconnect）；e2e 断言断连重连后新消息可消费、goroutine 不泄漏、`Close()` 不阻塞；`go test -race`。
- **压缩（H1）**：单测断言 `Producer.Compression` 按配置；e2e（integration）确认 RedPanda 收到压缩批次。
- **topic 守卫（H2）**：mock 三情形——缺失 topic（走创建）、`ErrLeaderNotAvailable`（不建+重试）、`ErrTopicAlreadyExists`（容忍不报错）。
- **重试删除（M1）**：Envelope 失败仅一次执行、留待重投；分区阻塞不翻倍。
- **双路径（M2）**：断言启用时日志提示 + 无 DLQ 告警；文档同步。
- **RF（M4）**：mock RF 漂移 → 校验路径上报。
- **回归守护**：复用现有 `partition_pipeline_test.go`、`kafka_partitions_selfheal_test.go`、`config_regression_test.go`。

### 5.3 回归测试矩阵（评审 6A 新增；IRON RULE：修回归就要有回归测试）

| 回归 | 测试 | 类型 | 依赖 broker? |
|---|---|---|---|
| C1 restoreSubscriptions 必败 | mockbroker：重连后 subs 清空、重新 Subscribe 成功、无 "already subscribed" | 单测 | 否（mockbroker） |
| C2 消费组不重建 | e2e：断连重连后新消息可消费（消费组绑定新 client） | e2e | 是（testcontainers） |
| C3 空 sarama config | 单测：`buildSaramaConfig` init↔reconnect parity（Idempotent/Acks/Successes/Fetch 等价） | 单测 | 否 |
| C4 消费 goroutine 不 restart | e2e：重连前后 goroutine 数不泄漏；`Close()` 不阻塞 | e2e | 是 |
| C5 状态机/锁/kill-switch | mockbroker：状态机顺序 + env=off 不触发 reconnect；`go test -race` 无死锁 | 单测 | 否（mockbroker） |
| C6 重连无冷却 | 单测：reconnect 进入即复位 failureCount + 冷却间隔内不重复 reconnect | 单测 | 否 |
| H1 压缩移除 | 单测：config ≠ None 按配置；e2e：broker 收到压缩批次 | 单测+e2e | e2e 是 |
| H2 topic 漏判 | mock 三情形（缺失/瞬时/already-exists） | 单测 | 否 |
| M1 重试反向优化 | Envelope 失败仅一次执行、留待重投 | 单测 | 否 |
| M2 双路径分叉 | 启用时日志提示 + 无 DLQ 告警 | 单测 | 否 |
| M4 RF 漂移 | mock RF 漂移 → 校验上报 | 单测 | 否 |

---

## 6. 风险

| 风险 | 等级 | 缓解 |
|---|---|---|
| 重连重构触及生命周期，引入新竞态 | 中 | `go test -race` + dev 反复断连验证；分步提交可回滚；kill-switch（`JXT_KAFKA_AUTO_RECONNECT=off`）可一键退化为不重连 |
| H2 topic 守卫朴素改法搞挂启动 | 中-高 | 严格按"区分瞬时 vs 缺失 + 容忍 already-exists + 测试"；单独 scoped 改 |
| 恢复压缩改变线材格式 | 低 | RedPanda 原生支持全部 codec；不影响语义 |
| 删除内联重试改变失败行为 | 低 | broker 重投保证 at-least-once；可观测性不降 |
| 重连修复"暴露"了原本被掩盖的问题（重连现在真会触发） | 低-中 | 这是预期收益；上线前在 dev 充分压测断连场景 |

---

## 7. 非回归（已澄清，不在本 spec 范围）

- **幂等 + `MaxInFlight<=0` → `MaxOpenRequests=100`**：潜在 footgun，但默认配置安全、且启动期大声报错，非"先好后破"。属预存缺口（可选在 Phase 2 顺带加防御性 clamp）。
- **`processMessageWithKeyedPool` 名字漂移**：纯命名（实际用 actor pool），活路径正确。
- **`globalKeyedPool`**：`.go` 源码已彻底清除（仅 `.md` 残留）。
- **`UnregisterTenant` close(ch) 竞态（outside voice，独立后续）**：`kafka.go:3562` `close(ch)` 与 `sendResultToChannel`（3472）的 Load→select 间存在 TOCTOU，可能向已关 channel 发 → panic 崩 ACK handler。属 multi-tenant ACK 路径预存 bug（非本 spec"优化回归"主线），**单独立项处理**。

---

## 8. 参考（git 取证锚点）

- `1474135`（2025-10-12）kafka 性能调优 — C2/C3/M3 根因；引入 LZ4 压缩（后被 `8970ae9` 删）。
- `5b80639`（2025-10-17）热路径免锁 — C1 根因（`sync.Map` + `LoadOrStore`）。
- `5a84d32`（2025-10-31）Hollywood 迁移 — C4 根因 + L1 死代码。
- `8970ae9`（2025-10-20）压缩配置"优化" — H1 根因（删 LZ4）。
- `85fc15b`（2025-11-01）重试优化 — M1 根因。
- `e2946a4`（2026-06-28）GetTopicPartitions 修复 — H2（同类 bug 未回填）。
- `c5d8f44`（2026-06-28）扩分区修复 — M4（RF 字段漏修）。
- `28f27d4`…`169da2a` + `21aed07`/`900e7a2`（2026-06-24/25）流水线 — M2 双路径。
- 关键文件：`kafka.go`、`partition_pipeline.go`、`type.go`、`topic_config_manager.go`、`hollywood_actor_pool.go`。

---

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| Eng Review | `/plan-eng-review` | 架构/代码/测试/性能 | 1 | CLEAR | Step 0 scope 接受 as-is；Arch 4 条（1A/2A/3A/4A 全采纳）；Code Quality 0；Tests 3 条（5A/6A/7A 全采纳）；Performance 0 |
| Outside Voice | Claude subagent（独立二次评审） | 抓评审盲点 | 1 | folded | 1×P0（C5 锁协议死锁，已改正）+ 1×P0（consumerStarted 顺序歧义，并入 C5）+ 4×P1（C6 重连冷却 / M3 扩展 Close drain+pair 重叠 / M2(d) 关停语义分叉 / tenant close 竞态→独立立项）+ 3×P2（M4 传播 DescribeTopics 错误 / H2 默认策略分支 / Phase C3↔H1 依赖），全部并入 |

- **VERDICT**：ENG CLEARED —— 所有评审 findings（含 outside voice）已采纳并入 spec；outside voice 抓出的 C5 死锁（P0，自死锁 + AB-BA）已按 `k.mu` 串行化 + 内部 lock-free 变体重写锁协议。
- **交叉模型一致性**：outside voice 独立确认了 spec 的若干诊断准确（legacy 严格 offset 有序、H1 压缩诊断、C1 never-Delete），并补出了评审漏掉的死锁与 7 项扩展——全部处理，无悬而未决的分歧。

NO UNRESOLVED DECISIONS
