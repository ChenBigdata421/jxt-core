# reliable 包 README 设计

- **日期**: 2026-07-29
- **分支**: `docs/reliable-readme`
- **状态**: 待用户 review
- **目标产物**: `sdk/pkg/reliable/README.md`（新增，单文件）

## 背景

`reliable` 是 jxt-core 的可靠消费内核（PR-2 已合入 master）。当前文档状况：

- 根 `doc.go` + 8 个子包 `doc.go`（store / gormshared / mysql / postgres / repotest / replay / lease）—— 参考型文档，密度高。
- **无 README**。peer 包 `sdk/pkg/eventbus/README.md`、`sdk/pkg/tenant/README.md` 都有中文 README（TOC + 快速开始 + 章节 + Go 片段）。
- 代码注释大量引用外部 spec（opus5-RCC-v2 §1~§8），读者手头未必有该 spec。
- `PR2_SCOPE.md` 是 PR-2 范围/决策规划文档，不是入门导览。

结论：缺一个入口/导览层。但 doc.go 与 §spec 已是详尽参考，README 若复述它们会引入漂移风险（刚修过 `PR2_SCOPE.md` 的 `PENDING`/`DISCARDED` 枚举漂移）。

## 目标与非目标

**目标**

- 为双受众（消费服务开发者 + jxt-core 贡献者）提供入口/导览层，**接入优先**。
- 自包含讲清核心心智模型：五状态机、一张表三用、fencing token、at-least-once + 租约自愈、aggregate gate。
- 把读者精确导向既有权威文档（`doc.go`、§spec、conformance），不复述。

**非目标（YAGNI，显式裁剪）**

- 不做 replay-safety 决策矩阵（指向 `safety.go`）。
- 不做完整状态转移表（Mermaid 图足够）。
- 不做 Store 逐方法契约参考（`doc.go` 已覆盖）。
- 不做 §spec 章节映射表。

## 受众与深度

- **受众**：双受众，接入优先（快速开始在前，架构/不变量在后）。
- **深度**：精简入口层，目标 ~150-180 行。
- **语言**：中文（对齐 eventbus/tenant README）。

## 结构（对齐 tenant README 风格）

| # | 章节 | 内容 |
|---|------|------|
| 1 | 一句话 | jxt-core 可靠消费内核：一张 `event_consumption` 表 = 幂等 + 死信 + 重投调度；五状态机 + fencing token；at-least-once。根包零第三方依赖。 |
| 2 | 快速开始 | 极简 Go：迁移（`mysql.Migration()`）→ 建 store + quarantineStore → 实现一个 `ReplayableHandler` + `HandlerRegistry` → `replay.NewScheduler(...)` → `scheduler.Run(ctx, interval)`。单一 handler、最小化、illustrative。 |
| 3 | 核心概念 | Mermaid 五状态机图（含合法转移）+ 一张表三用（M1）+ fencing token（让 0 行可判定）+ at-least-once/租约孤儿自愈 + aggregate gate（按聚合串行化）+ replay safety 一句话带过（细节指向 `safety.go`）。 |
| 4 | 包结构导航 | 表格：根(契约/纯函数,零依赖) / store(接口) / gormshared(双方言共享) / {mysql,postgres}(migration+classifier) / repotest(conformance 准入门禁) / replay(scheduler) / lease(租约孤儿 runner)；每行指向对应 `doc.go`。 |
| 5 | 关键不变量 | 白话版列表：TryClaim 独立提交；Mark* 须带 claim_id（fencing）；§2.4 列清空规则；aggregate gate 前置于 claim；tenant 作用域；CAS 写路径传播 `res.Error`。 |
| 6 | 文档地图 | 指针表：8 个 `doc.go`（根 + 7 子包）、`PR2_SCOPE.md`、§spec（opus5-RCC-v2，不复述）、conformance 套件。 |

顶部含 TOC（同 tenant README 的「目录」风格）。

## 关键决策

- **D1 状态机用 Mermaid** + 图注「状态与合法转移以 `state.go` / `legalTransitions` 为准」。不复述枚举为权威，避免漂移。
- **D2 快速开始写真 Go**，基于真实 API（`replay.NewScheduler` / `store.NewStore` / `mysql.NewQuarantineStore` / `HandlerRegistry` / `Run`），**写作时核对签名**。单一 handler、illustrative。
- **D3 单一真相源**：README 是入口/导览层；权威为 `state.go` + 各 `doc.go` + §spec。所有图/示例皆 illustrative，并标注「以代码为准」。
- **D4 篇幅** 目标 ~150-180 行。
- **D5 YAGNI 裁剪**：见上方「非目标」四项，不写入 README。

## 漂移防御

- 不在 README 里新建可与代码冲突的枚举/转移表（`PR2_SCOPE.md` 的 `PENDING`/`DISCARDED` 漂移即此类）。
- Mermaid 图与 `state.go` 的状态常量、`legalTransitions` 一一对应；图注显式声明代码为准。
- quick-start 的 Go 代码写作时与当前 API 对照；不确定的 API 用最小、稳定的构造路径。

## 验收标准

1. 文件落在 `sdk/pkg/reliable/README.md`，~150-180 行，中文，含 TOC + 上述 6 节。
2. 快速开始的 Go 代码与真实 API 一致（签名以代码为准，写作时核对）。
3. Mermaid 状态机与 `state.go` 的状态集 + `legalTransitions` 一致；图注声明以代码为准。
4. 包结构表每个子包指向其 `doc.go`。
5. 不含「非目标」列出的 4 项内容。
6. 不影响 build/test（`.md` 文件；`go build ./...` 维持通过）。

## 风险与对策

| 风险 | 对策 |
|------|------|
| quick-start API 误用/过期 | 写作时核对签名；示例尽量短、走稳定构造路径 |
| Mermaid 与代码漂移 | 图注声明 + review 时与 `state.go` 对照 |
| 篇幅膨胀 | 严格按「非目标」裁剪；每节控制在必要最小 |
| 双语言混杂 | 全中文（代码标识符/路径保留原文），对齐 peer README |
