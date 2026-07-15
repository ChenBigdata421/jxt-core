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

> **范围说明（F2）**：PR2 分两层交付——**PR2-core（P1，硬）**= ACK 生命周期 + shared registry + 稳定 terminal error，修复已核实的崩溃/丢数据缺陷；**PR2-hardening（P2，非硬）**= checked constructors + default-off + EventID dedupe/账本。core 是 file-storage 安全开攒批的真正承重点；hardening 不阻塞 batching（command 在无 hardening 的 v1.1.61 上已跑生产），可与 core 同批或作为 v1.1.63/补丁跟进。v1.1.62 的发版仅 gate 在 core。

---

## 0. 现状与已核实缺陷（v1.1.61，源码已核实）

PR2 的每一项都对应一个**已核实**的 v1.1.61 缺陷，不是推测。

### 0.1 outbox `EventBusAdapter`（`sdk/pkg/outbox/adapters/eventbus_adapter.go`）

- **`Close()` 非确定性 join（line 197-217）**：`close(stopChan)` → `time.Sleep(100*time.Millisecond)` → `close(outboxResultChan)`。靠固定 sleep 等 goroutine 退出，不是 join；高负载/调度延迟下可能在 goroutine 未退出时关 channel。`Close()` 永远返回 `nil`，**无 terminal error 语义**。
- **`tenantResultConversionLoop` 静默丢（line 177-186）**：`select { case ch<-r: case <-stopChan: default: 丢弃 }`——outbox 二级 channel 满时 accepted 结果被静默丢弃。

### 0.2 eventbus NATS registry（`sdk/pkg/eventbus/nats.go`，file-storage 实跑路径）

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
6. **（PR2-hardening, P2）batcher EventID dedupe + 账本/指标**：按 EventID 去重并记录 received / duplicate-collapsed / unique-flush。
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

- `EventBusAdapter.Close()` 与 publisher `StopACKListener()` 统一为：**先冻结 admission（拒绝新发送）→ join 已进入 send 临界区的 sender → drain 已 accepted 结果 → 关闭输出 channel**。移除 `time.Sleep(100ms)`，改为 `<-exited` 等 goroutine 真正退出。
- **消除 send-on-closed panic**：sender 关闭与发送互斥——要么 admission 冻结后 sender 不再发送（channel 不在发送中被关），要么发送路径加 `recover` 并把 panic 转为 terminal error。二选一，推荐前者（冻结 + join）。
- **稳定 terminal error**：首次 `Close`/`Stop` 计算终态错误并缓存；后续并发/重复调用返回同一缓存值（`errors.Join` 聚合多阶段错误）。`Close()` 不再恒返回 nil。

### 2.2 shared registry + admission（PR2-core）

- tenant registry 新增 **admission outcome**：`accepted` / `rejected-frozen` / `rejected-full` / `rejected-unregistered`，调用方据此决定重试或失败，**不再静默丢**。
- **移除 fallback-to-global**：tenant channel 满/未注册时**不得回落 global channel**（global 无人监听 = 静默丢），改为显式 `rejected-full`/`rejected-unregistered` outcome + 上游记账。
- **accepted 后无损 drain**：一旦 admission 返回 `accepted`，结果必须到达 listener（受 deadline 阻塞转发，不得 `default:` 丢）；关停时按 §2.1 冻结→drain→关。
- registry 是唯一允许非阻塞拒绝的 admission 边界；adapter 二级 channel 不得静默 drop。

### 2.3 checked constructors + 逆序 rollback（PR2-hardening）

- 新增 `NewOutboxPublisherChecked(...) (*OutboxPublisher, error)` 等返 error 构造；非法配置返明确错误（含参数名/值），不 panic。
- **保留旧 panic ctor 作兼容包装**（v1.1.61 `NewOutboxPublisher` panic 行为不变，command 等存量代码无感）。
- startup 多构造失败时按**构造逆序** rollback 已成功构造的组件。

### 2.4 default-off + 跨服务审计（PR2-hardening；F3/F7）

- **jxt-core 侧（本 PR2）**：`DefaultPublisherConfig().ACKBatchSize` 由 50 改 0。
- **file-storage 侧（PR5，不在本 spec）**：file-storage loader 内置默认亦为 0；canary 显式设 K=100。两处默认归属不同 PR，不得混淆。
- **跨服务审计（F3）**：default-off 影响所有传 **nil** config 的消费者。已核实 **command 显式设 ACKBatchSize=500**（`outbox_scheduler_manager.go:306`），不走库默认，不受影响。**shared 及其他服务需在 v1.1.62 发版前确认不依赖库默认 50**；若发现依赖，改为在各自服务显式设值（不回退 jxt-core 默认）。

### 2.5 batcher EventID dedupe + 账本/指标（PR2-hardening）

- batcher 按 EventID 去重，记录 `received` / `duplicate-collapsed` / `unique-flush` 三类计数。
- 账本用于 PR4 守恒证明（`accepted = forwarded = received = collapsed + unique`），非运行期正确性要求——`MarkBatchAsPublished` 是 `UPDATE ... WHERE id IN (...) AND status=pending`，DB 层幂等，不去重也不产生重复副作用。

### 2.6 Go toolchain（F4，已核实）

- jxt-core `go.mod` **当前已是 `go 1.26.0` / `toolchain go1.26.4`**（v1.1.61 HEAD 即如此）。**v1.1.62 不新增抬高 Go floor。**
- 跨服务影响：**command 已在 `go 1.26.0`**；唯一未升级的是 **file-storage（当前 `go 1.24.0`）**，其 Go 1.24→1.26 升级属 **PR5（file-storage）** 职责，不属 PR2。
- 结论：PR2 发 v1.1.62 **对 command/shared 无新 toolchain 影响**。

---

## 3. 验收标准（行为门，非"测试通过"流程门；F6）

1. **send-on-closed panic = 0**：`UnregisterTenant` 与 N 个并发在途 ACK 回调（N≥1000）竞态，`go test -race -count=N` 下 0 panic、0 data race。
2. **accepted 丢失 = 0**：tenant channel 满载/关停/未注册场景下，admission 返回显式 outcome；**无任何 accepted 结果静默丢或 fallback global**；账本守恒 `accepted = forwarded = listener received`。
3. **terminal error 稳定**：并发/重复 `Close`/`Stop`/`UnregisterTenant` 返回**同一缓存 terminal error**（字节级相等）；任一阶段错误使进程非零退出。
4. **确定性 join**：`Close`/`Stop` 在 goroutine 真正退出后返回（无 `time.Sleep`）；drain 预算内完成。
5. **checked constructor**：非法配置返含参数名/值的 error（不 panic）；startup 多构造失败按逆序 rollback（验证残留=0）。
6. **default-off**：`DefaultPublisherConfig().ACKBatchSize == 0`；nil-config 消费者不开启 batching。
7. **EventID dedupe**：重复 EventID 在 batcher 内 collapsed，账本三类计数守恒。
8. **兼容**：旧 panic ctor 行为不变；command 在 v1.1.62 上编译 + 既有测试全绿（无行为回归）。
9. **NATS + Kafka 双覆盖**：上述门禁在 NATS 与 Kafka 两条 registry 实现上均通过。

---

## 4. 测试计划

- **race/stress**：close-vs-send 竞态、full-buffer、repeated/concurrent Stop、flush panic/error/timeout——均 `-race` + 高 `-count`。
- **契约**：NATS 与 Kafka 双 broker 的 admission outcome、accepted 无损转发、terminal error 稳定性。
- **纯逻辑**：可控 ticker/barrier/fake clock，禁固定 `Sleep` 猜异步。
- **release gate**：不接受"80% ACK 即可"的旧容忍口径；每个行为门必须 0 panic / 0 drop。
- **跨服务回归**：v1.1.62 在 command（已 Go 1.26）上编译 + 既有 outbox/eventbus/tenant 测试全绿。

---

## 5. 风险（F1：crash 与 loss 分级）

| 风险 | 等级 | 缓解 |
|---|---|---|
| **关停期 send-on-closed panic（崩溃级）** | 高 | sender-owned close + join + admission 冻结；§3.1 race/stress 证 0 panic |
| **满载/竞态 accepted ACK 静默丢或 fallback global（数据级）** | 高 | 显式 admission outcome；accepted 后无损 drain；禁 fallback-global；账本守恒 |
| **重复/并发 Stop 状态不一致** | 中 | 稳定 terminal error 缓存 + `errors.Join` + 非零退出 |
| **default-off 击穿依赖库默认的消费者** | 中 | F3 审计：command 已核实显式设值；shared/其他发版前确认 |
| **v1.1.61→v1.1.62 行为回归** | 中 | 旧 panic ctor 兼容包装；command 既有测试全绿 |
| **hardening 与 core 同批发版拖延 v1.1.62** | 低 | F2：发版仅 gate 在 core；hardening 可拆 v1.1.63/补丁 |

---

## 6. 发布与回滚（F2）

- **v1.1.62 发版仅 gate 在 PR2-core**（§2.1/§2.2/§2.3-terminal error + §3 行为门 1-4）。core 完成即可打 tag。
- **PR2-hardening**（checked ctor / default-off / dedupe）可与 core 同批进 v1.1.62，或拆为 v1.1.63/补丁；**不阻塞 file-storage PR5**（PR5 只消费 core 的生命周期/registry 契约 + 显式 ACKBatchSize，不依赖 hardening）。
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
