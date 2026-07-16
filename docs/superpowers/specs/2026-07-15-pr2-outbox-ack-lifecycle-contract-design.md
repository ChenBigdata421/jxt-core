# Spec：jxt-core PR2 — Outbox ACK 生命周期与契约加固（v1.1.62）

| 项 | 内容 |
|---|---|
| 标题 | 修复 Outbox ACK 生命周期/registry/错误契约，发布 jxt-core v1.1.62 |
| 日期 | 2026-07-15 |
| 状态 | Draft（待 jxt-core 侧工程评审） |
| 影响模块 | `jxt-core/sdk/pkg/outbox`、`jxt-core/sdk/pkg/eventbus` |
| 目标版本 | **v1.1.62（新发布）**；v1.1.61 为现状基线 |
| 上游契约 | 消费方要求见 `file-storage-service/docs/superpowers/specs/2026-07-14-outbox-ack-batching-design.md` 的 PR2 章节 |
| **权威归属** | **本 spec 是 PR2 的权威实现 spec**；file-storage spec 的 PR2 章节为消费方契约，二者冲突时以本 spec 为准并在两侧同步 |

> **范围说明（F2）**：PR2 分两层交付——**PR2-core（P1，硬）**= ACK 生命周期 + shared registry + 稳定 terminal error，修复已核实的崩溃/丢数据缺陷；**PR2-hardening（P2，非硬）**= checked constructors + default-off（EventID dedupe/账本已移至 PR4，ce-doc-review #6）。core 是 file-storage 安全开攒批的真正承重点；hardening 不阻塞 batching（command 在无 hardening 的 v1.1.61 上已跑生产），可与 core 同批或作为 v1.1.63/补丁跟进。v1.1.62 的发版仅 gate 在 core。

---

## 0. 现状与已核实缺陷（v1.1.61，源码已核实）

PR2 的每一项都对应一个**已核实**的 v1.1.61 缺陷，不是推测。

### 0.1 outbox `EventBusAdapter`（`sdk/pkg/outbox/adapters/eventbus_adapter.go`）

- **`Close()` 非确定性 join（line 197-217）**：`close(stopChan)` → `time.Sleep(100*time.Millisecond)` → `close(outboxResultChan)`。靠固定 sleep 等 goroutine 退出，不是 join；高负载/调度延迟下可能在 goroutine 未退出时关 channel。`Close()` 永远返回 `nil`。**修复（D9，2026-07-16）**：改为 `loopsWg` 确定性 join，并如实返回**adapter 自身** teardown 错误（不再吞成 nil）。**按所有权原则，adapter 不拥有、不关闭注入的 `eventBus`**——bus 的 terminal error 由 bus 所有者（关停编排层）通过直接调用 `bus.Close()` 上抛并汇入进程非零退出（见 §3.3）。adapter 只负责自己创建的转换 goroutine 与 `outboxResultChan`。
- **`tenantResultConversionLoop` 静默丢（line 177-186）**：`select { case ch<-r: case <-stopChan: default: 丢弃 }`——outbox 二级 channel 满时 accepted 结果被静默丢弃。

### 0.2 eventbus ACK registry（NATS `sdk/pkg/eventbus/nats.go` + Kafka `sdk/pkg/eventbus/kafka.go`）

> **生产路径 = Kafka/RedPanda；NATS 消费路径 dormant**（2026-07-15 工程评审 D2 核实，纠正本节早前「NATS 是 file-storage 实跑路径」的误述）。下文 NATS 行号核实的缺陷在 `kafka.go` 为 byte-parallel 结构、同样存在；**Kafka 另有独有缺陷**：其 ACK sender 是两个无 WaitGroup join 的 goroutine（`handleAsyncProducerSuccess/Errors`，`kafka.go:2096-2097`），没有 NATS 那样的 `ackWorkerWg`，因此 `Close()` 关 tenant channel 前必须先 join 这两个 goroutine。PR2 的 Kafka 修复是生产关键路径，优先级高于 NATS。

- **send-on-closed-channel panic 竞态（崩溃级）**：`sendResultToChannel`（line 3361-3367）在 `RLock` 内取 channel、**RUnlock 后**再 `case tenantChan <- result` 发送；`UnregisterTenant`（line 3448）在 `Lock` 内 `close(ch)`。二者之间无 join/冻结/recover → **RUnlock 之后、send 之前一旦 close(ch) 即 `send on closed channel` panic**。关停时 NATS ack 回调与 UnregisterTenant 并发即可触发。memory impl（`eventbus.go:1396-1418` / `1454-1494`）同构。
- **accepted 结果静默丢 + fallback 到无人监听的 global channel（line 3370 / 3385-3396）**：tenant channel 满 → `default:` 不阻塞 → **回落 `publishResultChan`（global）**；global 也满 → `default:` 直接丢（"Both tenant and global ACK channels full, dropping result"）。file-storage 的 publisher 只听 tenant channel（`StartACKListenerWithChannel`），回落 global 的结果**无人消费 → 静默丢**。
- **无 registry freeze / 无 admission outcome / 无 terminal error**：全包 grep 无 `freeze`/`admission`/`terminal`/`Once` 符号；`UnregisterTenant` 仅 `close+delete`，无 drain/join。

> command 用**同一个** NATS registry + adapter，同样暴露上述缺陷，却已把 ACK 攒批跑在生产（file-storage spec §1 所引 12× 收益）——证明这些缺陷是**关停/背压正确性与健壮性问题，不是 batching 的功能性 gate**。PR2 的正当性来自"把已知崩溃/丢数据路径修干净"，而非"不开 PR2 就不能攒批"。

---

## 1. 背景与目标

file-storage-service 要启用 ACK 攒批（PR5），但其 spec 的 ce-doc-review 把"关停期 accepted ACK 在竞态中丢失/崩溃"评为高风险，叠加 file-storage 自身关停较脆弱（HTTP+FTP 双入口、多 signal owner、FTP 并发 Stop、10s context 被前置耗尽），选择先修 jxt-core 的生命周期/registry 契约再上 batching。PR2 即该 jxt-core 侧修复 + 发版。

**目标**
1. **（PR2-core, P1）ACK 生命周期**：`Close()`/`StopACKListener` 改为 sender-owned close + 确定性 join，消除 `Sleep` 与 send-on-closed panic。
2. **（PR2-core, P1）shared registry + admission**：tenant registry 提供冻结/显式 admission outcome；accepted 结果无损 drain；**移除 fallback-to-global 与静默 `default:` 丢**。
3. **（PR2-core, P1）稳定 terminal error**：重复/并发 `Stop`/`Close`/`UnregisterTenant` 返回同一缓存 terminal error，进程非零退出。
4. **（PR2-hardening, P2）checked constructors + 逆序 rollback**：构造返回 `error` 而非 panic；startup 失败按构造逆序回滚。保留旧 panic ctor 作兼容包装。
5. **（PR2-hardening, P2）default-off**：`DefaultPublisherConfig().ACKBatchSize` 由 50 改 0（fail-safe）。
6. **（移至 PR4）batcher EventID dedupe + 账本/指标**：dedupe 与三类计数账本移至 jxt-benchmark PR4 守恒证明（ce-doc-review #6）；`MarkBatchAsPublished` DB 幂等，dedupe 非 PR2 运行期正确性要求。
7. 发布 **v1.1.62**，保留旧 API 兼容；不修改 Outbox 框架核心算法（batcher/`MarkBatchAsPublished` 已就绪）。

**非目标（明确不做）**
- 不启用 `ConcurrentPublish`（瓶颈在 ACK 标记 DB 调用，不在 broker 提交侧）。
- 不做 durable DB claim/lease（独立 TODO）。
- 不改 topic 分区数。
- 不在 jxt-core 内针对 file-storage 做特化；PR2 是通用契约加固。
- 不重写 Outbox 框架（v1.1.61 的 `ackMarkerBatcher`/`MarkBatchAsPublished`/基础 listener 复用）。

---

## 2. 设计

### 2.1 ACK 生命周期：sender close/join + 稳定 terminal error（PR2-core）

- `EventBusAdapter.Close()` 与 publisher `StopACKListener()` 统一为：**先冻结 admission（拒绝新发送）→ join 已进入 send 临界区的 sender → 关闭输出 channel**。移除 `time.Sleep(100ms)`，改为对 goroutine 真正 join（`WaitGroup`/`<-done`）。
- **关停时不做后台 drain goroutine（D3，2026-07-16 修订）**：sender join 完成后**同步关闭** tenant channel；仍在 `range` 的消费者通过 range 自然排空缓冲，未被消费的 buffered ACK 则被**放弃**（不再靠后台 drain 争抢消费者的读）。放弃的 ACK 对应事件已发到 broker，由 outbox 重扫 at-least-once 重发兜底（见 §3.2 关停豁免与 §5）。早前"冻结→drain→关"表述中的**独立 drain 步骤已删除**。
- **消除 send-on-closed panic（D1）**：采用**锁内发送**——`sendResultToChannel` 全程持 `tenantChannelsMu.RLock()` 完成非阻塞 send；`UnregisterTenant`/`Close` 用写锁 `delete`+`close`，与所有 RLock 互斥，close 时无 sender 处于发送临界区。不加 `recover`。
- **稳定 terminal error（D7，2026-07-16 修订）**：`Close`/`Stop` 用 `sync.Once` + `closeDone` channel。首次调用运行 teardown、`errors.Join` 出终态错误并置 `terminalErr`，随后 `defer close(closeDone)`；**并发/重复调用阻塞在 `<-closeDone`**，返回**字节级相同**的 `terminalErr`。`Close()` 不再恒返回 nil。（放弃早前"pointer 在 teardown 前发布/快速返回"设想——终态错误在 teardown 完成前不可知；见 plan Task 3 Step 3 的 P1-#1 更正。代价：重复 Close 最多阻塞 ≤30s `PublishAsyncComplete` 预算。）

### 2.2 shared registry + admission（PR2-core）

- tenant registry 新增 **admission outcome**：`accepted` / `rejected-frozen` / `rejected-full` / `rejected-unregistered`，调用方据此决定重试或失败，**不再静默丢**。
- **移除 fallback-to-global**：tenant channel 满/未注册时**不得回落 global channel**（global 无人监听 = 静默丢），改为显式 `rejected-full`/`rejected-unregistered` outcome + 上游记账。
- **accepted 后无损转发（稳态）**：稳态运行期，一旦 admission 返回 `accepted`，结果必须到达 listener（受 deadline 阻塞转发，不得 `default:` 丢）。**关停窗口例外**：`Close` 按 §2.1（D3）同步关闭 tenant channel，可放弃缓冲区里 accepted 但未消费的 ACK，由 outbox at-least-once 重发兜底（§3.2 关停豁免）。
- registry 是唯一允许非阻塞拒绝的 admission 边界；adapter 二级 channel 不得在稳态静默 drop（关停时随 §2.1 关闭）。

### 2.3 checked constructors + 逆序 rollback（PR2-hardening）

- 新增 `NewOutboxPublisherChecked(...) (*OutboxPublisher, error)` 等返 error 构造；非法配置返明确错误（含参数名/值），不 panic。
- **保留旧 panic ctor 作兼容包装**（v1.1.61 `NewOutboxPublisher` panic 行为不变，command 等存量代码无感）。
- startup 多构造失败时按**构造逆序** rollback 已成功构造的组件。

### 2.4 default-off + 跨服务审计（PR2-hardening；F3/F7）

- **jxt-core 侧（本 PR2）**：`DefaultPublisherConfig().ACKBatchSize` 由 50 改 0。
- **file-storage 侧（PR5，不在本 spec）**：file-storage loader 内置默认亦为 0；canary 显式设 K=100。两处默认归属不同 PR，不得混淆。
- **跨服务审计（F3）**：default-off 影响所有传 **nil** config 的消费者。已核实 **command 显式设 ACKBatchSize=500**（`outbox_scheduler_manager.go:306`），不走库默认，不受影响。**shared 及其他服务需在 v1.1.62 发版前确认不依赖库默认 50**；若发现依赖，改为在各自服务显式设值（不回退 jxt-core 默认）。

### 2.5 batcher EventID dedupe + 账本/指标（**移至 PR4**；ce-doc-review #6）

> **2026-07-16 修订（ce-doc-review #6）：** dedupe 与三类计数账本（`received` / `duplicate-collapsed` / `unique-flush`）**整体移至 jxt-benchmark PR4**（守恒证明），不再属 PR2-hardening。理由：`MarkBatchAsPublished` 是 `UPDATE ... WHERE id IN (...) AND status=pending`，DB 层幂等，dedupe 非运行期正确性要求；账本唯一消费者是 PR4 守恒门，在 PR4 定义其形状前不应提前耦合进 v1.1.62。若后续发现 v1.1.61 存在重复 ACK 的实际缺陷，再凭该依据把 dedupe 重新引入 PR2（而非凭账本）。

### 2.6 Go toolchain（F4，已核实）

- jxt-core `go.mod` **当前已是 `go 1.26.0` / `toolchain go1.26.4`**（v1.1.61 HEAD 即如此）。**v1.1.62 不新增抬高 Go floor。**
- 跨服务影响：**command 已在 `go 1.26.0`**；唯一未升级的是 **file-storage（当前 `go 1.24.0`）**，其 Go 1.24→1.26 升级属 **PR5（file-storage）** 职责，不属 PR2。
- 结论：PR2 发 v1.1.62 **对 command/shared 无新 toolchain 影响**。

---

## 3. 验收标准（行为门，非"测试通过"流程门；F6）

1. **send-on-closed panic = 0**：`UnregisterTenant` 与 N 个并发在途 ACK 回调（N≥1000）竞态，`go test -race -count=N` 下 0 panic、0 data race。
2. **accepted 丢失 = 0（稳态口径）+ 关停窗口豁免**：
   - **稳态**：tenant channel 满载/未注册场景下 admission 返回显式 outcome；**无任何 accepted 结果静默丢或 fallback global**；稳态账本守恒 `accepted = forwarded = listener received`。
   - **关停窗口豁免（D3，2026-07-16 修订）**：`Close`/`UnregisterTenant` 可放弃缓冲区里 accepted 但未被 listener 消费的 ACK。这**不是数据丢失**——对应事件已发到 broker，outbox 重扫 at-least-once 重发，per-aggregate 顺序经 `SubscribeEnvelope` 保留，最终一致。故关停窗口**允许 duplicate publish、不允许 data loss**；该口径**依赖消费端幂等（evidence-management PR3）**，PR5 启用攒批必须在 PR3 部署之后（见 §6）。验收断言据此拆分：稳态守恒 + 关停后经重发的最终一致，而非"关停窗口零 accepted 放弃"。
3. **terminal error 稳定（D7）+ 所有权归属（D9）**：并发/重复 `Close`/`Stop`/`UnregisterTenant` 阻塞至 teardown 完成（`closeDone`）后返回**同一 terminal error**（字节级相等）。**归属（D9）**：bus 的 terminal error 由 bus 所有者（关停编排层，file-storage PR5 / command）**直接调 `bus.Close()`** 取得，与 `adapter.Close()`、`publisher.StopACKListener()` 一起 `errors.Join` 聚合，任一非 nil → 进程非零退出。`EventBusAdapter.Close()` 只返回自身错误、**不代关 bus**（"don't close what you don't own"）。§3.3 的"非零退出"在编排层强制，不在 adapter 边界。
4. **确定性 join**：`Close`/`Stop` 在 goroutine 真正退出后返回（无 `time.Sleep`）；drain 预算内完成。
5. **checked constructor**：非法配置返含参数名/值的 error（不 panic）；startup 多构造失败按逆序 rollback（验证残留=0）。
6. **default-off**：`DefaultPublisherConfig().ACKBatchSize == 0`；nil-config 消费者不开启 batching。
7. **EventID dedupe + 账本**：**移至 PR4**（ce-doc-review #6），不再作为 v1.1.62 验收门。
8. **兼容**：旧 panic ctor 行为不变；command 在 v1.1.62 上编译 + 既有测试全绿（无行为回归）。
9. **NATS + Kafka 双覆盖**：上述门禁在 NATS 与 Kafka 两条 registry 实现上均通过。

---

## 4. 测试计划

- **race/stress**：close-vs-send 竞态、full-buffer、repeated/concurrent Stop、flush panic/error/timeout——均 `-race` + 高 `-count`。
- **契约**：NATS 与 Kafka 双 broker 的 admission outcome、accepted 无损转发、terminal error 稳定性。
- **纯逻辑**：可控 ticker/barrier/fake clock，禁固定 `Sleep` 猜异步。
- **release gate**：不接受"80% ACK 即可"的旧容忍口径；每个行为门必须 0 panic / 稳态 0 drop（关停窗口 D3 放弃由重发兜底，见 §3.2）。
- **跨服务回归**：v1.1.62 在 command（已 Go 1.26）上编译 + 既有 outbox/eventbus/tenant 测试全绿。

---

## 5. 风险（F1：crash 与 loss 分级）

| 风险 | 等级 | 缓解 |
|---|---|---|
| **关停期 send-on-closed panic（崩溃级）** | 高 | sender-owned close + join + admission 冻结；§3.1 race/stress 证 0 panic |
| **满载/竞态 accepted ACK 静默丢或 fallback global（数据级）** | 高 | 显式 admission outcome；稳态 accepted 无损转发；禁 fallback-global；稳态账本守恒。关停窗口按 D3 放弃 buffered ACK，由 outbox at-least-once 重发 + PR3 消费端幂等兜底（§3.2 豁免） |
| **重复/并发 Stop 状态不一致** | 中 | 稳定 terminal error 缓存 + `errors.Join` + 非零退出 |
| **default-off 击穿依赖库默认的消费者** | 中 | F3 审计：command 已核实显式设值；shared/其他发版前确认 |
| **v1.1.61→v1.1.62 行为回归** | 中 | 旧 panic ctor 兼容包装；command 既有测试全绿 |
| **hardening 与 core 同批发版拖延 v1.1.62** | 低 | F2：发版仅 gate 在 core；hardening 可拆 v1.1.63/补丁 |

---

## 6. 发布与回滚（F2）

- **v1.1.62 发版 gate 在 PR2-core + Task 5 default-off**（ce-doc-review #4：§2.1/§2.2/§2.3-terminal error + §3 行为门 1-4 + §2.4 default-off）。core + default-off 完成即可打 tag。
- **PR2-hardening**（checked ctor / default-off；dedupe 已移至 PR4，ce-doc-review #6）可与 core 同批进 v1.1.62，或拆为 v1.1.63/补丁；**不阻塞 file-storage PR5**（PR5 只消费 core 的生命周期/registry 契约 + 显式 ACKBatchSize，不依赖 hardening）。
- **PR3 前置门（D3 依赖）**：v1.1.62 tag 本身安全（default-off，`ACKBatchSize=0` 不放弃 ACK）。但 **PR5 启用攒批（`ACKBatchSize>0`）必须在 evidence-management PR3 消费端幂等部署之后**——否则关停窗口 D3 放弃触发的 at-least-once 重发会命中非幂等消费者，造成 duplicate Media。发版顺序：PR2(v1.1.62) → PR3(消费端幂等) → PR5(启用攒批)。
- **回滚**：保留旧 panic ctor + 旧行为路径；若 v1.1.62 在 command 回归，command 可回退 v1.1.61（其 lifecycle 缺陷本就存在，回退不引入新风险）。file-storage 侧回滚走 `OUTBOX_ACK_BATCH_SIZE=0`（PR5 职责）。

---

## 7. 参考

**本 spec 核实的源码**
- `jxt-core/sdk/pkg/outbox/adapters/eventbus_adapter.go`（`Close` Sleep-join L197-217；`tenantResultConversionLoop` `default:` 丢 L177-186）
- `jxt-core/sdk/pkg/eventbus/nats.go`（`sendResultToChannel` discard+fallback L3358-3397；`UnregisterTenant` close 无 join L3433-3455；send-on-closed panic 竞态 RUnlock L3363→send L3367 vs close L3448）
- `jxt-core/sdk/pkg/eventbus/eventbus.go`（memory impl 同构，L1396-1418 / L1454-1494）
- `jxt-core/sdk/pkg/outbox/publisher.go`（`StopACKListener` drain L775-798；`Validate` panic L218-220；`DefaultPublisherConfig` 默认 50 L126）
- `jxt-core/sdk/pkg/outbox/ack_marker_batcher.go`（`Close` flush L73-94）

**关联 spec**
- 消费方契约：`file-storage-service/docs/superpowers/specs/2026-07-14-outbox-ack-batching-design.md`（PR2 章节 / §4.3 lifecycle 论证 / §6.2 验收）
- jxt-core 侧相邻：`jxt-core/docs/superpowers/specs/2026-07-15-kafka-redpanda-regression-hardening-design.md`（亦碰 v1.1.62/lifecycle/registry——PR2 实施前须确认两份 spec 的 registry/lifecycle 工作不重复、不冲突，必要时合并）

**Go floor 核实（F4）**
- `jxt-core/go.mod` = `go 1.26.0` / `toolchain go1.26.4`（v1.1.61 HEAD 即如此）
- `evidence-management/command/go.mod` = `go 1.26.0`；`file-storage-service/go.mod` = `go 1.24.0`（PR5 升级）
