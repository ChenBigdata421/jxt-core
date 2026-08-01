# 合并评审报告：dispatch-dropfix（最终版，2026-08-01）

**范围：** 88f9e35（Task 0）..HEAD 的 6 个提交 + 工作区（6 文件 +237/-84）
**来源：** 报告 A（本会话，10 评审者 + 独立验证器，run 20260801-112520）/ 报告 B（另一评审会话，10 评审者，run 20260801-105248）/ 后续修复轮（G4-G10 实施）
**状态：** 所有发现项均已代码核实；仅剩发布流程事项

## 评审范围与意图

- **范围：** 88f9e35..HEAD（Task 0 移除 3s 预热 sleep + Task 1b/2/3 dispatch-dropfix 系列共 6 个提交）+ 工作区未提交改动（README、kafka.go、type.go、两个测试文件）
- **意图：** 消除 Kafka 静默丢消息（spec §5 A）：未激活 handler 的 topic 由 DRAIN（MarkMessage+跳过，静默丢失）改为 HOLD（背压：不读通道、不提交、不推进 frontier）直至激活；legacy ConsumeClaim 与 pipeline consumeWithPipeline 双路径统一；新增 IsActiveTopic 访问器、内部 HoldBackoff 配置、停滞可观测性
- **排除：** TODOS.md（用户未提交改动）、docs/superpowers/plans/2026-07-31-dispatch-dropfix-jxt-core.md（未跟踪计划文档）

## ✅ 已修复（全部经 `git diff`/grep 代码确认）

### 已随提交落地（HEAD 内）

| 项 | 内容 | 确认 |
|----|------|------|
| Task 0 | 3s warmup sleep 删除 + `IsWarmupCompleted`/`GetWarmupInfo` 导出方法移除（提交 88f9e35，纯遥测无调用方） | ✅ |
| Task 1b | `IsActiveTopic(topic string) bool` 访问器（b4f6228，kafka 驱动，供 consumed⊆activated 自检） | ✅ |
| Task 2 | HoldBackoff 内部字段（默认 100ms、<=0 钳位、validate >0）+ legacy hold-on-nil（e6782c2、5e7f212、1e35e66） | ✅ |
| Task 3 | pipeline 在 p.run 之外 hold 至激活（31e13c8，partition_pipeline.go 未动） | ✅ |

### 已随工作区落地（未提交）

| 项 | 内容 | 确认 |
|----|------|------|
| F1 | pipelineCfg hoist（每 claim 解析一次，legacy hold 读 `pipelineCfg.HoldBackoff`，消除每轮 poll churn + 路径不对称） | ✅ kafka.go:1051 |
| F2 | 重复乱码注释修复 | ✅ |
| F3 | `cfg.HoldBackoff = d.HoldBackoff` 单源化（默认值来源唯一，"同源同值"注释由假变真） | ✅ type.go:700 |
| F4 | gofmt 注释对齐（StallWarnInterval 尾注对齐到 HoldBackoff 列） | ✅ gofmt -d 干净 |
| G2 | hold 停滞信号：进入一次性 Warn + monotonic StallEnterReporter 上升沿 + `consumption_partition_stalled_seconds` 实时爬升、退出归零（镜像 p.run 停滞语义，遵守 review 2026-07-26：ClearPartitionStall 仅 topic-unsubscribe 时调用）+ 测试 TestHoldUntilActivated_EmitsStallSignal | ✅ kafka.go:1022-1027 |
| G6 | 死 `if true/else` drain 分支删除（42→34 行，行为等价，gofmt 干净） | ✅ 残留 0 |
| GP1 | legacy envelope 重试失败 `return retryErr` 终止 claim（spec M1：不再继续循环导致后续 MarkMessage 借 sarama MarkOffset MAX 语义越位提交未处理 offset）。TDD 红→绿：测试实证修复前 `marked=[11]`（offset 10 被越位提交 = 静默丢失） | ✅ kafka.go:1098 |
| G3 | D3' 测试注释诚实化（明确 happy-path 不变量守卫定位；确定性接缝列为待办） | ✅ |
| G10 | 测试 helper 合并 `newHoldTestEventBus(t, enabled, holdBackoff)`（7 个调用点更新，删未用 import） | ✅ |
| G4 | hold 循环去重：`holdUntilActivated` 返回 `(*handlerWrapper, error)`，D3' 竞态处理收敛进 helper（Load 命中即捕获；返回后 deactivate 仅移除 map 注册，捕获的 wrapper 仍有效），两调用点各收敛为一行 + err 检查，`resolveWrapper` 删除 | ✅ 签名 1 处、残留 0 |
| G8 | ctx 取消统一返回 nil：helper 保留 ctx.Err() 精确语义，调用层与外层 select（kafka.go:1094）/p.run 的 ctx.Done 分支对齐——正常关停非错误，消除 sarama claim 错误日志噪音；2 处测试 `ErrorIs(Canceled)`→`NoError`（<500ms 迅速返回 + 0 MarkMessage 核心不变量保留） | ✅ 2 处注释 + 2 处断言 |
| G1+G9 | README：v1.8.0 版本条目（行为变更 + ⚠️ 5 服务审计门禁 + 回滚只能降版本 + IsActiveTopic/HoldBackoff/停滞信号/GP1 说明）+ EventBus「Kafka 预订阅语义」小节（hold 语义、停滞可观测性、IsActiveTopic 鸭子类型断言示例、HoldBackoff 内部说明） | ✅ README.md |

**验证基线**：`go build ./...` 通过；全部改动文件 gofmt 干净（LF 归一化检查）；hold/stall/config 测试套件 PASS（含 `-race`）；D3' churn 测试单独 0.27s 无退化。

## ⏳ 剩余待办（用户 owned / 需决策）

| 项 | 性质 | 状态 |
|----|------|------|
| G5 下游 handler 幂等确认 | 上线清单（TODOS.md F1 未关——hold 的"下次会话重投递"保证仅在 handler 幂等时安全） | ⏳ 非代码 |
| **5 服务 consumed⊆activated 审计** | **发布前置门禁**（plan Task 1）：drain→hold 使「故意预订阅但不激活」的 topic 从静默丢失变为永久停滞；evidence-management command/query、security-management、file-storage-service、tenant-service | ⏳ 跨 4 仓库 |
| D3' 确定性测试接缝 | 注入 `activationChecker` 做确定性覆盖（可选加固，非阻塞） | ⏳ 可选 |
| v1.8.0 tag + 发布 + 5 服务 bump + 回切 | plan Task 5；CHANGELOG 条目已在 README，tag 前复核；回滚只能降模块版本（无 flag 门禁） | ⏳ 用户 owned |

## 遗留残余风险（记录在案）

- 1ns 病态小正数 HoldBackoff 未防御（applyPipelineDefaults 只钳 `<=0`，legacy 绕过 validate；仅能经程序员代码设置，攻击面小）
- 共享 `group.id` 跨服务：另一服务的永不激活 topic 在本进程快照中会冻结整个 group
- hold 期间真实 rebalance 仅经 fake harness 验证，无真实 broker 集成测试（rebalance-while-holding、sarama 1000-msg 背压缓冲、重投递）
- pipeline stale wrapper：claim 中途 deactivate + 不同 handler 重激活会把后续消息路由到已捕获 wrapper（理论性——deactivateTopicHandler 当前零生产调用者）

## 结论

代码层面全部就绪（无未修复的代码发现项）。核心修复经 sarama v1.46.0 源码核实（release() 先取消会话 ctx 再 waitGroup.Wait()，hold 快速返回不会阻塞到 60s Rebalance.Timeout；P2#9 成立）。发布前只剩 2 个非代码事项：G5 幂等确认 + 5 服务审计（承重门禁），之后即可 tag v1.8.0。
