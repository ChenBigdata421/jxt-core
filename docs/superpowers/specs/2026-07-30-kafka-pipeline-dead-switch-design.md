# Kafka 分区消费流水线「死开关」修复设计

- 日期：2026-07-30
- 范围：jxt-core（仅）
- 状态：待评审（已并入评审反馈 v4：v3 的 D2/D3/D4 + v4 2nd eng pass 的 D2=A `reliable.live_enabled` 耦合校正）。**round-2 correction（supersedes v4 coupled-flip framing in §5/§6/§8/VERDICT）**：round-2 review grep-verified R2-6 fail-fast（`subscriber.go:162`）位于 `SubscribeEnvelopeDeliveryWithReliable` 内、**零生产调用方**（command 两个 prod handler 均走 `SubscribeEnvelopeWithDLQ`/PR-0），故 `pipeline.enabled: false`（command + query）**单独**即 v1.1.70 no-op；`reliable.live_enabled: false` 为 command-only Task-8 卫生级（仅当 Task 8 落在同窗口才升为 REQUIRED）；query 无 `reliable/` 包、仅需 `pipeline.enabled: false`；保留 `scheduler_enabled: true`。**以 plan「Release Gate」节为发布决策权威**（supersedes 本文件 §5/§6/§8/VERDICT 中残留的 v4 coupled-flip 表述）。
- 目标 tag：v1.1.70

## 1. 背景

jxt-core 的 Kafka 消费侧实现了分区内消费流水线 `consumeWithPipeline`（N 在飞 + 连续前缀提交 + DLQ seam），作为 legacy 串行路径的性能/可靠性增强通道。该通道由内部 `ConsumerConfig.Pipeline.Enabled`（`sdk/pkg/eventbus/type.go:616`）控制，默认关闭，设计意图是「灰度时显式开启」（release toggle / dark launch）。

但运行期没有任何生产路径能把 `Enabled` 置为 `true`——`pipeline.enabled` 是一个**死开关**。

## 2. 问题（已逐行核实）

死链：

1. 用户配置层 `ConsumerConfig`（`sdk/config/eventbus.go:185`）只有 4 个字段（`GroupID`/`AutoOffsetReset`/`SessionTimeout`/`HeartbeatInterval`），**无 `Pipeline` 字段**。`settings.yml` 的 `eventbus.kafka.consumer.pipeline.enabled` 被 viper 读到后，Unmarshal 时无目标字段 → 丢弃。全 `sdk/config/` grep `Pipeline` → 零命中。
2. `convertUserConfigToInternalKafkaConfig`（`sdk/pkg/eventbus/eventbus.go:1260-1279`）构造内部 `Consumer` literal 时，只 copy 上述 4 字段 + 硬编码程序员默认；`Pipeline` 未赋值 → 零值（`Enabled:false`）。
3. `convertConfig`（`init.go:236-251`）kafka 分支只覆盖 `Security` + `Enterprise`，不补 `Pipeline`。
4. `pipelineConfig()`（`kafka.go:968`）= `applyPipelineDefaults(config.Consumer.Pipeline)`，而 `applyPipelineDefaults`（`type.go:663`）**刻意不默认 `Enabled`**（注释「未显式置 true 即关闭」）→ `Enabled` 保持 `false`。
5. `ConsumeClaim`（函数起于 `kafka.go:990`；`if pipelineCfg.Enabled` 判定在 `:994`）→ 永假 → 走 legacy 串行路径，`consumeWithPipeline` 不可达。
6. 无兜底入口：全包 grep `With*`/`EnablePipeline`/`SetPipeline` → 零；构造函数（`NewEventBus`/`InitializeGlobal`/`NewKafkaEventBus`）只收 config 结构体、无 functional option；全仓生产代码唯一的 `Pipeline:`/`.Pipeline=` 赋值就是字段声明本身，所有 `Enabled:true` 都在绕过 bus 的测试里。
7. evidence-management（`command/server.go:551`、`query/server.go:517`）只调 `InitializeFromConfig`，不手工构造内部 config。

**结论**：内部 `PipelineConfig` 存在，但生产路径没有任何代码能把 `Enabled` 置 `true`。

### 2.1 连带影响（同一根因的下游症状，非独立 bug）

DLQ seam 注入在 `consumeWithPipeline` 内（运行期赋值 `p.dlq = wrapper.dlq` 在 `kafka.go:1214`；`dlq` 字段声明在 `partition_pipeline.go:114`，非 :115-118）。pipeline 不可达 → DLQ 触发逻辑（`sendDLQ`/`forwardCompletion`/`advanceFrontier`）永不执行。

jxt-core 内部 DLQ 接线本身是通的（`activateTopicHandler(...dlq...)` → `wrapper.dlq` → `p.dlq`），evidence-management 侧适配器（`EventBusDLQAdapter`，编译期断言满足 `eventbus.DLQSender`）与注入入口（`SubscribeEnvelopeWithDLQ`，经 `SubscribeEnvelopeWithOptions` 注入非 nil DLQ，且有回归测试锁死）也已就绪——**整条 DLQ 链路构造正确、整体潜伏，唯一闸门就是 pipeline 不可达**。

因此 PR-0 DLQ 运行期触发 + PR-3 可靠投递的 **bus 侧执行** 被同一个死开关卡住——但须厘清当前真实状态（校正先前版本对 R2-6 的误述）：`SubscribeEnvelopeDeliveryWithReliable` **并非**被死开关挡住。evidence-management 经自己的 viper 读 `eventbus.kafka.consumer.pipeline.enabled`（`reliable/config.go:150-151`，绕过 bus 内部 `Consumer.Pipeline`），flag 为 `true` 时 R2-6 fail-fast（`subscriber.go:162`）通过、可靠订阅 **今天即已**带着 DLQ adapter 订阅成功。死开关只屏蔽 bus 侧 `consumeWithPipeline`+DLQ 的 **执行**，不屏蔽订阅。于是真实当前态比「潜伏」更需警惕：evidence-management `command` 正跑着「自以为有 DLQ/隔离兜底」的可靠订阅，而 bus 实跑 legacy 串行（DLQ seam 休眠，K7 非法租户 → 隔离路径「静默不存在」，见 `subscriber.go:147-153`）。修好开关的真正效果是让 bus 终于兑现 evidence-management 早已发布的 DLQ 接线——闭合「订阅侧以为开了 / bus 侧没开」的落差，而非「解锁订阅」。这也正是 §5「激活面」里激活风险是具体的、而非假想的原因。

## 3. 目标 / 非目标

**目标**：把 `pipeline.enabled` 从死开关变成真正的 latent toggle——让 `settings.yml` 的 `pipeline` 配置端到端流到 `kafkaEventBus.config.Consumer.Pipeline`，使 `consumeWithPipeline` 在显式开启时可达。**默认仍关；对未配置 pipeline 的服务默认行为零变化**（已在 `settings.yml` 里设 `enabled:true` 的服务会激活——见 §5「激活面」披露）。

**非目标（明确排除）**：

- evidence-management 的任何改动（adapter / `SubscribeEnvelopeWithDLQ` / reliable config / `dead_letter_queue`）——归各自项目。
- 真正打开 pipeline / 灰度发布——发布决策，受 4 个前置条件（幂等 handler、DLQ 接线、扩分区、eager assignor）约束，本次不做。
- 跨服务 DLQ e2e——需 evidence-management adapter + 库表，jxt-core 单独做不了。
- 不新增 functional option（`WithPipeline`）——测试可直接用内部 `EventBusConfig` 构造，非必需。唯一启用入口是 `settings.yml`；若将来有服务要程序化开关、不经 settings，再加 option（有意识取舍，非缺口，详见 §6）。

## 4. 设计

修复后的配置流（死开关 → latent toggle）：

```
settings.yml                         用户层 sdk/config                内部 sdk/pkg/eventbus
eventbus.kafka.consumer.pipeline:    ConsumerConfig.Pipeline          kafkaEventBus.config.Consumer.Pipeline
  enabled: true      ──viper 解码──▶ PipelineUserConfig{           ──convertUserConfigToInternalKafkaConfig:1260──▶ PipelineConfig{
  windowSize: 32                        Enabled, WindowSize          (§4.1 新增)                                     Enabled, WindowSize,
                                       }                                                                             FlushTimeout/DLQTimeout/StallWarnInterval = 0 }
                                                                                                                      │
                                                                                                         applyPipelineDefaults (type.go:663)
                                                                                                         补 timing 安全默认；Enabled 不默认（保持「未显式置 true 即关闭」）
                                                                                                                      ▼
                                                                                                    ConsumeClaim (kafka.go:994)：pipelineCfg.Enabled
                                                                                                                      │ true —— 修复后可达（死开关前永假）
                                                                                                                      ▼
                                                                                                    consumeWithPipeline + DLQ seam   ✅ 可达
```

### 4.1 用户配置表面（方案 B：Enabled + WindowSize）

在 `sdk/config/eventbus.go` 的 `ConsumerConfig`（:185）新增字段，并定义用户层结构：

```go
type ConsumerConfig struct {
    GroupID           string             `mapstructure:"groupId"`
    AutoOffsetReset   string             `mapstructure:"autoOffsetReset"`
    SessionTimeout    time.Duration      `mapstructure:"sessionTimeout"`
    HeartbeatInterval time.Duration      `mapstructure:"heartbeatInterval"`
    Pipeline          PipelineUserConfig `mapstructure:"pipeline"` // 新增
}

// PipelineUserConfig 用户层流水线配置。仅暴露开关与并发旋钮；
// timing 类安全不变量（flushTimeout/dlqTimeout/stallWarnInterval）留内部默认，
// 由 applyPipelineDefaults 兜底——避免用户误配违反 FlushTimeout < sessionTimeout/2 而 panic。
type PipelineUserConfig struct {
    Enabled    bool `mapstructure:"enabled"`              // 功能开关，默认 false
    WindowSize int  `mapstructure:"windowSize,omitempty"` // 灰度并发旋钮；0 → 内部默认 16
}
```

理由：

- `Enabled` 是 on/off；`WindowSize` 是灰度真正要拧的旋钮（从 2~4 起步爬坡）。
- `flushTimeout`/`dlqTimeout`/`stallWarnInterval` 与 `sessionTimeout` 绑死（`validate` 不满足会 panic），**不暴露**，由 `applyPipelineDefaults` 用安全默认填充。
- 纯新增字段、零值=关闭，非破坏性。
- settings 路径：`eventbus.kafka.consumer.pipeline.{enabled,windowSize}`（**`consumer.pipeline`**，非 `subscriber.pipeline`；三处一致性详见 §6）。

### 4.2 配置桥接

在 `convertUserConfigToInternalKafkaConfig`（`sdk/pkg/eventbus/eventbus.go:1260` 的 `Consumer` literal）透传：

```go
Consumer: ConsumerConfig{
    // ...既有 4 字段 + 程序员默认...
    Pipeline: PipelineConfig{
        Enabled:    userConfig.Consumer.Pipeline.Enabled,
        WindowSize: userConfig.Consumer.Pipeline.WindowSize,
        // FlushTimeout/DLQTimeout/StallWarnInterval 留零值，运行期由 applyPipelineDefaults 补
    },
},
```

`convertConfig` 的 kafka 分支（`init.go:236-251`）无需改动（已走该函数）。

### 4.3 加载期 fail-fast 校验（必须在建连之前）

现状：`PipelineConfig.validate()` 只在 `newPartitionPipeline`（`partition_pipeline.go:128`）懒触发，非法时 panic → sarama 不 recover → 崩 claim/进程。暴露用户字段后新增失败模式：用户开 pipeline 同时设了较小 `sessionTimeout`（用户可配字段）→ 默认 `FlushTimeout=4s` 违反 `< sessionTimeout/2` → 首条消息 panic。

修法：在 `NewKafkaEventBus`（`kafka.go:238`）**函数入口**加校验——紧随既有的 nil/brokers 守卫（:239-245），**在任何 sarama 建连之前**。`NewKafkaEventBus` 的首次网络触点是 `:372` `sarama.NewClient(cfg.Brokers, ...)`（其后 `:378` producer、`:385` `NewConsumerGroupFromClient`、`:393` admin）。校验必须在 `:372` 之前 return：

```go
// 紧随 nil/brokers 守卫之后、sarama.NewClient(:372) 之前
// effective 单一派生：校验（§4.3）与启动可观测日志（§4.4）共用，勿在 init.go 重算（P3）
effective := applyPipelineDefaults(cfg.Consumer.Pipeline)
logger.Info("kafka consumer pipeline config", // 用 NewKafkaEventBus 既有 logger（同 kafka.go:471）
    "pipelineEnabled", effective.Enabled,
    "pipelineWindowSize", effective.WindowSize) // 生效值（0→16 已补全）—— D3=A
if effective.Enabled {
    if err := effective.validate(cfg.Consumer.SessionTimeout); err != nil {
        return nil, fmt.Errorf("invalid kafka consumer pipeline config: %w", err)
    }
    logger.Warn("kafka consumer pipeline ENABLED — partition-pipeline + DLQ seam is LIVE; " +
        "ensure idempotent handlers, DLQ wiring, partition sizing, and assignor are ready")
}
```

**强制顺序的理由（实现者必须遵守，否则 fail-fast 退化 / 测试跑不了）：**

- **错误路径不触网**：校验在 `sarama.NewClient`（:372）之前 return → 不尝试连 Kafka，不会被连接错误掩盖真正的配置错误，也避免无谓建连。
- **单测零 broker 依赖**：§4.5 的"Enabled=true + 极小 SessionTimeout → 返回 error"用例可直接 `NewKafkaEventBus(&KafkaConfig{...})` 拿到 validation error，无需真 broker、不卡在连接错误上。

其余：

- 仅在 `Enabled=true` 时校验（关闭时流水线不跑，timing 不变量无关）。
- 错误经 `NewKafkaEventBus` → `InitializeGlobal` → `InitializeFromConfig` 返回 → 服务启动失败、报错清晰。
- `newPartitionPipeline` 内既有的 `validate` 保留，作为 defense-in-depth（构造期已拦截，此处对 config 驱动的情形事实上不可达）。
- 不变量逻辑只挂在 eventbus 包（`PipelineConfig.validate` + `applyPipelineDefaults` 的权威位置），不在用户层 `config.EventBusConfig.Validate()`（`sdk/config/eventbus.go:542`）重复，避免跨层漂移。

### 4.4 启动可观测

pipeline 死活信号**从 `NewKafkaEventBus` 内发出**（P3：与 §4.3 校验同处、复用同一 `effective`，单一派生源——选择 `NewKafkaEventBus` 而非 `init.go` 作为放置点，避免日后在第二处派生 `applyPipelineDefaults` 造成双真相。注：当前 `init.go:148` 是一句 `logger.Info`、并非 `applyPipelineDefaults` 调用——该函数仅在 `type.go:663` 定义、`kafka.go:969` 调用；P3 是「选位」决策，非「搬移已有调用」）。具体日志行见 §4.3 代码块：

- `pipelineEnabled`（生效 `effective.Enabled`）+ `pipelineWindowSize`（**生效值**，D3=A：`applyPipelineDefaults` 补全后的值，非用户原值 0）——**on/off 都打**，让开关死活在启动日志一眼可见。
- `Enabled=true` 时额外打 WARN（partition-pipeline + DLQ seam is LIVE），把静默激活变成可观测事件（呼应 §5「激活面」）。

用 `NewKafkaEventBus` 既有 `logger`（同 `kafka.go:471`「Kafka EventBus created successfully」所用），**不**依赖 `bus.logger`（构造期为 `zap.NewNop()`，见 `kafka.go:407`）。这是防此类潜伏的最便宜护栏；因 colocated 于校验旁，生效配置只有一处真相，新增字段无需两处同步。

### 4.5 roundtrip 回归测试

在 `sdk/pkg/eventbus/config_regression_test.go`（已有 `convertUserConfigToInternalKafkaConfig` 用例，:322）旁新增：

- `pipeline.enabled=true` → 内部 `config.Consumer.Pipeline.Enabled==true`（断言端到端贯通）。
- `pipeline` 缺省 → 内部 `Enabled==false`（断言默认仍关、非破坏）。
- `windowSize=4` → 内部 `WindowSize==4`；`windowSize=0` → 内部 `WindowSize==0`。
  - `windowSize=0` 这条断言刻意保留：它钉住「`convertUserConfigToInternalKafkaConfig` 自己**不补** WindowSize、留给运行期 `applyPipelineDefaults` 补 16」，防止以后有人在转换层误补。
- 构造期校验（依赖 §4.3 的建连前顺序）：`Enabled=true` + 极小 `SessionTimeout`（如 8s → `sessionTimeout/2=4s`，默认 `FlushTimeout=4s` 不满足 `<`）→ `NewKafkaEventBus` 直接返回 validation error，不触网、无需 broker。**断言必须命中 error 字符串**（D4=A，如 `strings.Contains(err.Error(), "pipeline")` 或前缀 `invalid kafka consumer pipeline config`）——仅断言 `err != nil` 无法区分「校验先于 `sarama.NewClient` 跑」与「校验被错排在 `NewClient` 之后、返回的是拨号错误」（两者皆非 nil）；命中字符串才能钉死 §4.3「错误路径不触网」的顺序不变量。用例需传**非空** brokers 绕过 `:243` 守卫（bogus broker 即可——校验先 return，永不到达拨号）。

现有 `partition_pipeline_test.go`（含 fakeDLQ）不动。

## 5. 影响与风险

- **默认行为零变化**：不写 pipeline 段 → `Enabled=false` → legacy 串行路径，与现状一致。
- **激活面（非默认路径；发布决策已定 = D2/A）**：「默认行为零变化」仅对 *未写* `pipeline` 段的配置成立。evidence-management `command/config/settings.yml` 与 `query/config/settings.yml` **当前已提交** `eventbus.kafka.consumer.pipeline.enabled: true`（windowSize 32 / 64）——死开关是眼下唯一把它们留在 legacy 串行路径上的闸门。本修复一旦被它们吃到（§6：本地 `replace ../../jxt-core` 让开发态即时生效），partition pipeline + DLQ seam 即在 windowSize 32/64 上 **激活**，等同非目标 #2 推迟的「灰度发布」；§4.3 校验拦不住（两者 `sessionTimeout=10s` → 10/2=5s > 默认 `FlushTimeout=4s`，校验通过 → 静默激活而非 fail-fast）。
  - **决策（D2=A，打 tag 前执行）**：与 evidence-management 协调，在 **v1.1.70 同发布窗口**把 `pipeline.enabled` 置 `false`。**round-2 correction（supersedes v4 coupled-flip）**：`pipeline.enabled: false`（command + query）**单独即 v1.1.70 no-op**——round-2 review grep-verified R2-6 fail-fast（`command/internal/infrastructure/eventbus/subscriber.go:162`）位于 `SubscribeEnvelopeDeliveryWithReliable` 内、**零生产调用方**（command 两个 prod handler `process_task_event_handler.go:55`/`file_storage_event_handler.go:52` 均走 `SubscribeEnvelopeWithDLQ`/PR-0，无 pipeline guard；PR-3 canary 是未来 Task 8——`startup.go:21-23`）；`reliable.live_enabled` 仅在 startup 日志（`server.go:133`）读、非任何路由分支（`reliable/config.go:29-32` 是结构体字段注释、非可执行路由）。故 v4「`pipeline.enabled: false` 单独不是 no-op、必须同翻 `live_enabled`」的前提（command 可靠订阅走 PR-3 Delivery 路径）在当前代码中不成立。具体：
    - `eventbus.kafka.consumer.pipeline.enabled: false`（command AND query）—— REQUIRED、load-bearing；总线留在 legacy 串行（休眠）。**这是 v1.1.70 no-op（单独成立）。**
    - `reliable.live_enabled: false`（command ONLY——query 无 `reliable/` 包，故无此键）—— v1.1.70 卫生级、非 load-bearing（今天无 handler 在 PR-3 路径上）。建议设：当 Task 8 接上 `SubscribeEnvelopeDeliveryWithReliable` canary 时，command 的可靠消费已在 PR-0 byte-faithful 路径、而非在 R2-6 上 fail-refuse。**打 tag 前确认 Task 8 是否落在 v1.1.70 窗口**：是→升为 REQUIRED（load-bearing）；否→维持卫生级。
    - **保留 `reliable.scheduler_enabled: true`**（RETRY_SCHEDULED 行已在 broker 端 ACK、不会重投，只有 scheduler 能排空——`reliable/config.go:35-37`；翻为 false 即 R2-3 数据丢失模式）。
    - 这是 evidence-management **自己设计的回滚模式**（两 flag 拆分的目的即此——`reliable/config.go:23-27`），非新机制。真正的激活作为受 4 前置条件（幂等 handler、DLQ 接线、扩分区、eager assignor）约束的**独立后续发布**——其中幂等 handler 审计即 `TODOS.md` F1，仍 open。**发布说明须载明此翻转动作**（以 plan「Release Gate」节为权威），否则 evidence-management 下次 bump pin 即静默激活。
  - **决策（D5=A，同窗口顺带）**：§4.1 只暴露 `{Enabled, WindowSize}`，`flushTimeout`/`dlqTimeout`/`stallWarnInterval` **无解码目标**——而 evidence-management `query/config/settings.yml:111-112`（`flushTimeout:4s` / `dlqTimeout:30s`）、`command/config/settings.yml:122`（`stallWarnInterval:10s`）**已提交**这些键，经 `sdk/config/config.go:56` 裸 `v.Unmarshal`（无 `ErrorUnused`）**静默丢弃**（与 §2 死开关同机制）。注释随之失真（query:111「jxt-core validate 拒绝 >= 5s」修复后为假——validate 读内部默认 4s，读不到此值）。今日零回归仅因提交值恰等于 `defaultPipelineConfig()`（type.go:636-638）。**这是死开关反模式的窄化重演、且未披露**。D2=A 编辑这两个 settings.yml 时，**顺手剥离或改注**这 3 个 timing 键为 `# ignored — jxt-core applies internal defaults; only enabled/windowSize are read`（零代码、同窗口），闭合陷阱。
  - **连带披露（非本次回归，发布说明载明）**：修开关的真正效果是让 bus 兑现 evidence-management 早已发布的 DLQ 接线。在激活发生**之前**，prod 实跑 legacy 串行、DLQ seam 休眠——与今天完全一致、无回归；但「订阅侧以为有 DLQ/隔离兜底 / bus 侧没跑」的落差仍在，K7 非法租户隔离路径「静默不存在」直至激活。勿把「pipeline 关闭」误读为「DLQ 已接线」。
- **非破坏性**：用户 config 纯新增可选字段；内部 config 透传零值等价于现状。
- **解锁**：PR-0 DLQ 运行期 + PR-3 可靠投递从 dead 变 latent（能否真正生效交各自项目）。
- **风险点（改善而非回退）**：用户开 pipeline + 小 `sessionTimeout` 现在会启动失败而非运行期 panic——这是 fail-fast 改善，但需在升级说明里提示。

## 6. 集成与发布备注

- **配置键路径（务必三处一致）**：用户层字段 `ConsumerConfig.Pipeline`（`mapstructure:"pipeline"`）决定的 settings 路径是 `eventbus.kafka.consumer.pipeline.{enabled,windowSize}`——是 **`consumer.pipeline`**，不是 `subscriber.pipeline`（PR-3 计划/R2-6 文本里偶尔的 `subscriber.pipeline.enabled` 是笔误，GSTACK review 已校正为 `consumer`）。修完后 `settings.yml`、jxt-core、evidence-management fail-fast 三处都用 `consumer.pipeline`，一致。
- **evidence-management R2-6 fail-fast 自动消解（round-2 correction: re-coupling 非当前态运行期耦合，见 §5 D2=A）**：evidence-management 的 `LoadReliableConfig` 直接读 `settings` 的 `eventbus.kafka.consumer.pipeline.enabled` 做 fail-fast 判定（R2-6）。修完 jxt-core 后，运行期总线也吃同一个 key——「fail-fast 读的值」与「运行期实际值」第一次一致，Task 6 review 报的那个 Critical 会随这一处修复自动消解。**但须厘清当前真实状态（round-2 correction，supersedes v4）**：v4 把 `pipeline.enabled` 同时视为 (a) 总线 `consumeWithPipeline` 闸门 + (b) evidence-management R2-6 订阅 fail-fast 闸门，断言 D2=A（key=false）会让 reliable 在 Delivery 路径上拒绝订阅、故必须同翻 `live_enabled=false`。round-2 review grep-verified：R2-6 fail-fast（`subscriber.go:162`）**位于 `SubscribeEnvelopeDeliveryWithReliable` 内、零生产调用方**（command 两个 prod handler 均走 `SubscribeEnvelopeWithDLQ`/PR-0；PR-3 canary 是未来 Task 8——`startup.go:21-23`），且 `reliable.live_enabled` 仅在 startup 日志读、非路由分支；故「re-coupling 强制同翻 `live_enabled`」的当前态前提不成立。**`pipeline.enabled: false`（command + query）单独即让总线休眠**；`live_enabled: false` 为 command-only Task-8 卫生级（query 无 `reliable/` 包，仅需 `pipeline.enabled: false`）；保留 `scheduler_enabled: true`。进阶（非必需）：可让 evidence-management 改读已解析结构体 `config.AppConfig.EventBus.Kafka.Consumer.Pipeline.Enabled`，更稳。发布决策以 plan「Release Gate」节为权威。
- **版本与 replace**：tag = **v1.1.70**（jxt-core v1.1.x 顺序，PR-2 = v1.1.68）。evidence-management 用本地 `replace ../../jxt-core`，jxt-core 一提交、evidence-management 开发态立即吃到；release 时再把 pin 改 v1.1.70。**发布协调（D2=A + D5=A）**：evidence-management 在 pin 到 v1.1.70 的**同一发布**里，把 `command`/`query` settings.yml 的 `pipeline.enabled` 置 `false`，并**剥离/改注 3 个 timing 键**（`flushTimeout`/`dlqTimeout`/`stallWarnInterval`——无解码目标、静默丢弃，见 §5 D5），确保 v1.1.70 以 no-op 上线；激活另作受 4 前置条件约束的独立发布。
- **协调范围（P2）—— 3 个 Kafka 消费服务**：除 evidence-management `command`/`query` 外，`process-management`（`type: kafka`，`config/settings.yml`）是第 3 个 jxt-core Kafka 消费方。它**当前无 `pipeline:` 段** → v1.1.70 下不激活；但它**照搬了 command 的 `groupId`**（`evidence-command-consumer-group`，`:102`），未来若再 copy 一份 `pipeline:` 段是现实激活隐患。发布协调须追踪 3 个服务，而非 2 个。（范围外披露：process-management 与 evidence-management command 共用 `evidence-command-consumer-group` 是 consumer-group 冲突——process-management 侧的 bug，非本 PR，须另行路由修复。）
- **启用入口取舍**：唯一启用入口是 `settings.yml`，不设 functional option（呼应非目标 #4）。对 evidence-management（走 settings）够用；将来若有服务要程序化开关、不经 settings，再加 option——有意识取舍，非缺口。

## 7. 验证

1. `go build ./...` 通过。
2. 新 roundtrip 测试 + 既有 `partition_pipeline_test.go` 通过（scoped 跑：`go test ./sdk/pkg/eventbus/ -run Pipeline`；裸跑全套需 broker，见既有约定）。构造期校验用例因 §4.3 顺序保证，零 broker 依赖。
3. 手动：`settings.yml` 设 `eventbus.kafka.consumer.pipeline.enabled: true` → 启动日志出现 `pipelineEnabled=true`；不设 → `pipelineEnabled=false` 且行为不变。

## 8. 待决 / 已决

- 已决：用户层暴露 `Enabled` + `WindowSize`（方案 B）；timing 字段留内部默认。
- 已决：fail-fast 校验挂在 `NewKafkaEventBus` **函数入口、`sarama.NewClient`（:372）之前**（单一权威位置、错误路径不触网）。
- 已决：唯一启用入口 `settings.yml`，不加 functional option。
- 已决（v3 评审 D2=A；v4 校正；**round-2 correction supersedes v4 coupled-flip**）：激活闸门为**发布协调**而非代码守卫。v3 原写「翻 `pipeline.enabled=false`（一行）令 jxt-core 以纯 no-op 发布」——v4 eng re-review 发现此为假：`pipeline.enabled=false` 会触发 evidence-management command 的 R2-6 订阅 fail-fast（`subscriber.go:162`），而 `live_enabled` 默认 true；故 v4 校正为**耦合翻转**：同窗口 `pipeline.enabled=false` + `reliable.live_enabled=false`（保留 `scheduler_enabled=true`）。**round-2 correction（grep-verified，supersedes v4）**：R2-6 fail-fast（`subscriber.go:162`）位于 `SubscribeEnvelopeDeliveryWithReliable` 内、**零生产调用方**（command 两个 prod handler 均走 `SubscribeEnvelopeWithDLQ`/PR-0；PR-3 canary 是未来 Task 8——`startup.go:21-23`），故 v4「必须耦合翻转」的前提（command 可靠订阅走 PR-3 Delivery）不成立。**正确表述**：`pipeline.enabled: false`（command + query）**单独即 v1.1.70 no-op**；`reliable.live_enabled: false` 为 command-only Task-8 卫生级（仅当 Task 8 落在同窗口才升为 REQUIRED；query 无 `reliable/` 包、仅需 `pipeline.enabled: false`）；保留 `scheduler_enabled: true`。激活另作受 4 前置条件约束的独立发布。发布决策以 plan「Release Gate」节为权威；发布说明载明翻转动作（非 v4 的「耦合翻转」）。
- 已决（v3 评审 D3=A）：§4.4 启动日志记 `applyPipelineDefaults` **生效** windowSize（非用户原值 0），保证 LIVE 信号诚实。
- 已决（v3 评审 D4=A）：§4.5 T4 断言 validation error **字符串**（非仅 `err != nil`），钉死「校验先于 `sarama.NewClient`」顺序不变量。
- 已决（v3 评审 D5=A，outside-voice P1）：`PipelineUserConfig` 只暴露 `{Enabled,WindowSize}`；evidence-management 已提交的 3 个 timing 键（`flushTimeout`/`dlqTimeout`/`stallWarnInterval`）无解码目标、经 `config.go:56` 静默丢弃——D2=A 编辑 settings.yml 时顺带剥离/改注，避免死开关反模式窄化重演（见 §5 D5）。
- 已决（v3 评审 P2，outside-voice）：§6 协调范围扩到 3 个 Kafka 消费服务（含 process-management，当前不激活）；其与 command 共用 `evidence-command-consumer-group` 的 consumer-group 冲突属范围外，另行路由。
- 已决（v3 评审 P3，outside-voice）：§4.4 启动日志移入 `NewKafkaEventBus`、复用 §4.3 的单一 `effective` 派生，不在 `init.go:148` 重算；用既有 `logger`（非构造期 `zap.NewNop()` 的 `bus.logger`）。
- 无悬而未决项。

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Codex Review | `/codex review` | Independent 2nd opinion | 1 | absorbed (Claude subagent — Codex CLI unavailable) | 3 findings (D5/P2/P3), all verified + folded into v3 |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 2 | clean (2nd pass folded D2=A coupling) | 7 issues decided (6 prior + D2=A `live_enabled` coupling), 0 critical gaps |
| Design Review | `/plan-design-review` | UI/UX gaps | 0 | — | — |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |

**VERDICT:** ENG + OUTSIDE-VOICE CLEARED — ready to implement. jxt-core v1.1.70 ships as a no-op for the library (toggle reachable; every config WITHOUT a `pipeline:` segment — process-management / file-storage / security / tenant — is unaffected, default-off). **Round-2 correction (supersedes the v4 "coupled flip" wording below):** evidence-management flips `pipeline.enabled→false` (command AND query — the load-bearing v1.1.70 no-op, ALONE sufficient) plus a command-only `reliable.live_enabled→false` (keep `scheduler_enabled` true) that is inert today (no handler is on the PR-3 path — the R2-6 fail-fast at `subscriber.go:162` is inside `SubscribeEnvelopeDeliveryWithReliable`, which has ZERO prod callers; both command handlers use `SubscribeEnvelopeWithDLQ`/PR-0; PR-3 canary is future Task 8) but pre-positions command's reliable path for the future Task 8 canary; query needs only `pipeline.enabled→false` (no `reliable/` package). Plus the D5=A timing-key cleanup. Activation is a separate, precondition-gated release (F1 idempotency audit still open). **The plan's "Release Gate" section is authoritative for the release/no-op decision.** *(Prior v4 VERDICT wording — "COUPLED flip ... `pipeline.enabled→false` AND `reliable.live_enabled→false` ... so command's reliable path falls back to PR-0 rather than fail-refusing on R2-6" — is preserved here for audit; it overstated a present-tense coupling that round-2 grep-verified as currently unwired.)*

**2nd ENG PASS (2026-07-30):** An independent re-review verified every load-bearing code claim against source at confidence 10/10 — dead-switch root cause (no `Pipeline` field on user `ConsumerConfig`), single conversion path (no second dead switch; `convertConfig` routes through `convertUserConfigToInternalKafkaConfig`), network-free validation insertion (kafka.go:239-371 are `saramaConfig` assignments; first dial is `sarama.NewClient` at :372), `fmt`+`logger` already imported, validate arithmetic (8s/2=4s vs default FlushTimeout=4s → error). The jxt-core fix is correct and minimal. It surfaced ONE issue the v3 eng + outside-voice passes missed: **D2=A as written is NOT a no-op for evidence-management** — `pipeline.enabled=false` triggers command's R2-6 fail-fast (`subscriber.go:162`) and `reliable.live_enabled` defaults true with no committed key. Resolution accepted (Option A): co-flip `live_enabled=false` to route onto the PR-0 fallback (verified `reliable/config.go:29-32`). Folded into §5 D2=A + the plan's Release Gate. **Round-2 correction (supersedes the co-flip framing above):** the R2-6 fail-fast at `subscriber.go:162` is inside `SubscribeEnvelopeDeliveryWithReliable`, which round-2 grep-verified has ZERO production callers (both command prod handlers — `process_task_event_handler.go:55`, `file_storage_event_handler.go:52` — subscribe via `SubscribeEnvelopeWithDLQ`/PR-0; the PR-3 canary is future Task 8 work — `startup.go:21-23`); and `reliable.live_enabled` is read only at a startup log line (`server.go:133`), not in any routing branch. Therefore the "co-flip is load-bearing" premise above is forward-looking, not present-tense: `pipeline.enabled: false` (command + query) ALONE is the v1.1.70 no-op; `reliable.live_enabled: false` is command-only Task-8 hygiene (promotes to REQUIRED only if Task 8 lands in the v1.1.70 window); query needs only `pipeline.enabled: false` (no `reliable/` package). Keep `scheduler_enabled: true`. The plan's Release Gate records this correction as authoritative. Minor doc fixes folded in: ConsumeClaim starts kafka.go:990 (the `if` is :994); partition_pipeline.go:115-118 are struct field decls (DLQ injection is kafka.go:1214); P3/init.go:148 restated as a placement choice (init.go:148 is a `logger.Info`, not an `applyPipelineDefaults` call).

**CROSS-MODEL:** Outside voice (Claude subagent, fresh context — Codex CLI not installed on this host) surfaced 3 issues the eng review missed — D5 (timing-key dead config, the narrower dead-switch antipattern), P2 (process-management = 3rd kafka consumer), P3 (§4.4 duplicate `applyPipelineDefaults` derivation) — all verified against code at confidence 8-9/10 and absorbed into the v3 body. Activation surface independently re-confirmed (only evidence-management command/query ship `enabled:true`; file-storage/security/tenant do not). No cross-model disagreement remained.

NO UNRESOLVED DECISIONS
