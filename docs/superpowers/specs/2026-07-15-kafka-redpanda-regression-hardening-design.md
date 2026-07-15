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

这 4 条（+ P1 的 ACK 生命周期）相互纠缠，必须作为一个连贯改动一起修——单点修补撑不住。

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
- **破坏机制**：翻 `PipelineConfig.Enabled` **静默**改变：(a) 提交粒度（逐条 vs 连续前缀 mark-once）；(b) DLQ/告警有无（legacy 无 DLQ/告警，pipeline 有）；(c) **分区 offset 完成顺序**（legacy 严格按 offset，pipeline 允许 N+1..N+15 先于 N 完成）。文档只承诺 per-aggregate 有序（两路径都满足），未提 offset 完成序变了——下游若以"先见 N 再见 N+1"做幂等（如 `lastSeenOffset`），翻 flag 后行为不同。
- **最佳实践方案**：改变正确性语义的 feature flag 必须在 toggle 时显式告知。① README + `type.go:616` 注释明确"启用后分区 offset 完成序不再保证，仅 per-aggregate 有序"；② 首次启用打一次性 `logger.Info` 提示；③ 硬门禁：若有 Envelope 订阅未注入 `DLQSender` 则拒绝启用（或至少强告警），避免启用后静默走 Strategy-A stall + logger 告警。
- **验收**：启用时日志提示；无 DLQ 的 Envelope 订阅在启用 pipeline 时告警/拒绝；文档更新。
- **风险**：低。flag 默认关；本条主要是文档 + 门禁。

#### M3. ACK handler goroutine 无生命周期
- **位置**：`kafka.go:442-443`（初始化 spawn）、`kafka.go:2095-2097`（重连 spawn）、handler `kafka.go:457-519`/`521+`。
- **造成 commit**：`1474135`（引入）、`8970ae9`（固化）。
- **破坏机制**：无 `sync.WaitGroup`/stop chan。重连 handoff 期：`Publish` 可能 load 到正在被 `Close` 的旧 producer → 向已关闭 producer 的 `Input()` 发 → panic；且新 goroutine 调度前 success/error 静默丢弃；`Close()` 从不等 ACK handler drain。
- **最佳实践方案**：goroutine 所有权 + 优雅关闭。加 `sync.WaitGroup`（或 stop chan）；`Close`/`reinitializeConnection` 先 signal stop、关 producer、`wg.Wait()`；`Publish` 经 generation/RWMutex 门禁，不能向正在关闭的 producer 发。（与 Critical 重连重构同改。）
- **验收**：`go test -race` 下重连期间无 panic；`Close` 后 ACK goroutine 全部退出。
- **风险**：中。生命周期 + 竞态。

#### M4. ReplicationFactor 漂移不可检测
- **位置**：`getActualTopicConfig` `kafka.go:3420-3428`（只填 `Replicas`，不填 `ReplicationFactor`）；`compareTopicOptions` `topic_config_manager.go:261`（守卫恒假）。
- **造成 commit**：`c5d8f44`（2026-06-28）修了 `Partitions` 字段，漏平行的 `ReplicationFactor`。
- **破坏机制**：`TopicOptions` 有同义双字段 `Replicas`/`ReplicationFactor`；`createKafkaTopic` 优先 `ReplicationFactor` 回退 `Replicas`；`getActualTopicConfig` 只填 `Replicas` → 校验路径对 RF 漂移全盲（`StrategyValidateOnly`/`shouldUpdate` 永远"all good"）。
- **最佳实践方案**：`getActualTopicConfig` 同时填 `actualConfig.ReplicationFactor`（= `Replicas` 或 `len(Partitions[0].Replicas)`）。长期：合并双字段为单一权威字段。
- **验收**：mock RF 漂移 → `StrategyValidateOnly` 正确上报差异。
- **风险**：低。

---

### Low（清理）

| # | 项 | 位置 | 方案 |
|---|---|---|---|
| L1 | 死代码 `kafkaConsumerHandler`/`processMessage`（迁移遗留、语义分叉、无 Envelope/DLQ） | `kafka.go:786-889` | 删除（零调用） |
| L2 | 死代码 `ensureKafkaTopic`（同类 bug、零调用） | `kafka.go:3164-3186` | 删除 |
| L3 | 死代码 `getCompressionCodec()` | `kafka.go:731-744` | 删除或改为压缩配置解析复用（配合 H1） |
| L4 | rebalance 默认 `range` → 应 `sticky`（eager，分区更稳；cooperative-sticky 仍禁） | `kafka.go:760-771` | 默认改 `BalanceStrategySticky` |
| L5 | `closed` 检查 TOCTOU 窗口（`5b80639` 提前 Unlock 略放大） | `kafka.go:1592` | `startPreSubscriptionConsumer` 前再 `closed.Load()` 复查，或修复当初为避死锁而分离的持锁范围 |

---

## 4. 分阶段实施计划

| 阶段 | 内容 | 风险 | 灰度 |
|---|---|---|---|
| **Phase 1（P0）** | 重连子系统重构：`buildSaramaConfig` 抽取 + 重连重建消费组 + `restoreSubscriptions` 清空重建 + stop/restart 消费 goroutine + ACK 生命周期（C1-C4 + M3） | 中（生命周期） | 改完即生效，无需 flag；建议先在 dev 反复断连验证 |
| **Phase 2（P0/P1）** | 恢复 producer 压缩（H1）+ topic 建表守卫谨慎修（H2）+ RF 字段（M4） | H2 中-高 | 压缩默认开；topic 修复单独 scoped |
| **Phase 3（P1）** | 删除内联重试（M1）+ 流水线双路径文档/门禁（M2） | 低 | M2 的硬门禁随 pipeline 灰度 |
| **Phase 4（P2）** | 死代码清理（L1-L3）+ sticky 默认（L4）+ closed TOCTOU（L5） | 低 | 直接改 |

---

## 5. 测试计划

- **重连（Phase 1）**：模拟 broker 断连重连 → 断言订阅重建成功、消费组绑定新 client、新消息可消费、goroutine 不泄漏、`Close()` 不阻塞；`go test -race`。
- **压缩（H1）**：单测断言 `Producer.Compression` 按配置；端到端确认 RedPanda 收到压缩批次。
- **topic 守卫（H2）**：mock 三情形——缺失 topic（走创建）、`ErrLeaderNotAvailable`（不建+重试）、`ErrTopicAlreadyExists`（容忍不报错）。
- **重试删除（M1）**：Envelope 失败仅一次执行、留待重投；分区阻塞不翻倍。
- **双路径（M2）**：断言启用时日志提示 + 无 DLQ 告警；文档同步。
- **RF（M4）**：mock RF 漂移 → 校验路径上报。
- **回归守护**：复用现有 `partition_pipeline_test.go`、`kafka_partitions_selfheal_test.go`、`config_regression_test.go`。
- **门禁命令**：`go build ./...` + `go test -race ./sdk/pkg/eventbus/...`（注意：裸 `go test ./sdk/pkg/eventbus/` 无 broker 会挂，用 scoped `-run` + `go build` 作门禁）。

---

## 6. 风险

| 风险 | 等级 | 缓解 |
|---|---|---|
| 重连重构触及生命周期，引入新竞态 | 中 | `go test -race` + dev 反复断连验证；分步提交可回滚 |
| H2 topic 守卫朴素改法搞挂启动 | 中-高 | 严格按"区分瞬时 vs 缺失 + 容忍 already-exists + 测试"；单独 scoped 改 |
| 恢复压缩改变线材格式 | 低 | RedPanda 原生支持全部 codec；不影响语义 |
| 删除内联重试改变失败行为 | 低 | broker 重投保证 at-least-once；可观测性不降 |
| 重连修复"暴露"了原本被掩盖的问题（重连现在真会触发） | 低-中 | 这是预期收益；上线前在 dev 充分压测断连场景 |

---

## 7. 非回归（已澄清，不在本 spec 范围）

- **幂等 + `MaxInFlight<=0` → `MaxOpenRequests=100`**：潜在 footgun，但默认配置安全、且启动期大声报错，非"先好后破"。属预存缺口（可选在 Phase 2 顺带加防御性 clamp）。
- **`processMessageWithKeyedPool` 名字漂移**：纯命名（实际用 actor pool），活路径正确。
- **`globalKeyedPool`**：`.go` 源码已彻底清除（仅 `.md` 残留）。

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
