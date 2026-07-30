# Kafka Pipeline Dead-Switch Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `pipeline.enabled` a real latent toggle in jxt-core — wire `settings.yml`'s `eventbus.kafka.consumer.pipeline.{enabled,windowSize}` end-to-end to `kafkaEventBus.config.Consumer.Pipeline`, and add construction-time fail-fast validation + startup observability. Default stays off; the library ships as a no-op.

**Architecture:** Two-layer config (user `sdk/config` → internal `sdk/pkg/eventbus`). Add a minimal `PipelineUserConfig{Enabled,WindowSize}` to the user-facing `ConsumerConfig`; passthrough inside `convertUserConfigToInternalKafkaConfig`; in `NewKafkaEventBus` (before any sarama dial) derive a single `effective := applyPipelineDefaults(...)`, log it, and validate when enabled. Timing fields stay internal (safe defaults via `applyPipelineDefaults`).

**Tech Stack:** Go 1.23+, sarama, viper/mapstructure, testify (`assert`/`require`).

**Spec:** `docs/superpowers/specs/2026-07-30-kafka-pipeline-dead-switch-design.md` (v4 — v3 eng-review + outside-voice CLEARED, + v4 2nd eng pass folded the D2=A `reliable.live_enabled` coupling).

## Global Constraints

(Copied verbatim from the spec; every task's requirements implicitly include these.)

- **jxt-core only.** No evidence-management / process-management code edits in this plan. The D2/D5 settings-flip + timing-key strip is cross-repo coordination owned by the evidence-management project — captured in the **Release Gate** section below, not as code tasks here. (D2=A's load-bearing flip is `pipeline.enabled: false` (command + query); a command-only `reliable.live_enabled: false` is Task-8 hygiene, not a present-tense coupling — see Release Gate.)
- **Default off, no-op ship.** `pipeline.enabled` absent → `Enabled=false` → legacy serial path, identical to today.
- **Config key is `consumer.pipeline`**, not `subscriber.pipeline`. New field is `Pipeline PipelineUserConfig \`mapstructure:"pipeline"\`` on `ConsumerConfig`.
- **Timing fields are NOT exposed.** Only `{Enabled, WindowSize}`. `flushTimeout`/`dlqTimeout`/`stallWarnInterval` stay internal; `applyPipelineDefaults` fills safe defaults.
- **Branch:** work on `fix/kafka-pipeline-dead-switch` (or a worktree via using-git-worktrees), not `master`. Commit per task.
- **Tests:** scoped run `go test ./sdk/pkg/eventbus/ -run Pipeline -v` (zero broker). Do NOT bare-run the full eventbus suite (it hangs without a broker). Overall compile gate: `go build ./...`.
- **gofmt:** repo uses `core.autocrlf=true`; run gofmt only on the files this plan touches.
- **Target tag:** v1.1.70 (after PR-2 = v1.1.68). Do NOT tag until the Release Gate is cleared.

## File Structure

No new files. Four focused edits; the regression test file is the shared home for both new test cycles.

- `sdk/config/eventbus.go` — MODIFY: add `PipelineUserConfig` struct + `Pipeline` field on user `ConsumerConfig` (around :185).
- `sdk/pkg/eventbus/eventbus.go` — MODIFY: `convertUserConfigToInternalKafkaConfig` `Consumer` literal (around :1260), passthrough `Pipeline`.
- `sdk/pkg/eventbus/kafka.go` — MODIFY: `NewKafkaEventBus` (around :245), insert effective-derive + log + validate + WARN before `sarama.NewClient` (:372).
- `sdk/pkg/eventbus/config_regression_test.go` — MODIFY: add roundtrip + validation tests. (`package eventbus`; already imports `time`, `config`, testify `assert`/`require`.)

---

### Task 1: Wire pipeline config end-to-end (user field → internal config)

**Files:**
- Modify: `sdk/config/eventbus.go` (`ConsumerConfig`, ~:185-192)
- Modify: `sdk/pkg/eventbus/eventbus.go` (`convertUserConfigToInternalKafkaConfig`, `Consumer` literal ~:1260-1279)
- Test: `sdk/pkg/eventbus/config_regression_test.go`

**Interfaces:**
- Consumes: existing internal `PipelineConfig` (`sdk/pkg/eventbus/type.go:621`) and its field on internal `ConsumerConfig` (`type.go:616`). No new internal types.
- Produces: `config.PipelineUserConfig{Enabled bool; WindowSize int}`; user `ConsumerConfig.Pipeline`; and passthrough so `convertUserConfigToInternalKafkaConfig(user).Consumer.Pipeline.{Enabled,WindowSize}` equal the user values (timing fields stay zero — runtime-owned).

- [ ] **Step 1: Write the failing roundtrip tests**

Append to `sdk/pkg/eventbus/config_regression_test.go`:

```go
// TestConvertUserConfig_PipelinePassthrough 钉死 §4.1+§4.2：用户层 pipeline.{enabled,windowSize}
// 经 convertUserConfigToInternalKafkaConfig 端到端贯通到内部 Consumer.Pipeline。
// timing 字段（FlushTimeout 等）转换层不补，留运行期 applyPipelineDefaults（windowSize=0 同理）。
func TestConvertUserConfig_PipelinePassthrough(t *testing.T) {
	user := &config.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		Consumer: config.ConsumerConfig{
			GroupID:        "g1",
			SessionTimeout: 10 * time.Second,
			Pipeline: config.PipelineUserConfig{
				Enabled:    true,
				WindowSize: 4,
			},
		},
	}

	internal := convertUserConfigToInternalKafkaConfig(user)

	assert.True(t, internal.Consumer.Pipeline.Enabled, "Enabled must propagate")
	assert.Equal(t, 4, internal.Consumer.Pipeline.WindowSize, "WindowSize must propagate")
	// 转换层不补 timing——留运行期 applyPipelineDefaults
	assert.Equal(t, time.Duration(0), internal.Consumer.Pipeline.FlushTimeout,
		"FlushTimeout must NOT be defaulted in the convert layer (runtime applyPipelineDefaults owns it)")
}

// TestConvertUserConfig_PipelineDefaultOff 缺省 pipeline 段 → 内部 Enabled=false、WindowSize=0（非破坏）。
func TestConvertUserConfig_PipelineDefaultOff(t *testing.T) {
	user := &config.KafkaConfig{
		Brokers:  []string{"localhost:9092"},
		Consumer: config.ConsumerConfig{GroupID: "g2"}, // 无 Pipeline 段
	}

	internal := convertUserConfigToInternalKafkaConfig(user)

	assert.False(t, internal.Consumer.Pipeline.Enabled, "absent pipeline must stay disabled (no-op default)")
	assert.Equal(t, 0, internal.Consumer.Pipeline.WindowSize, "WindowSize 0 (runtime defaults to 16)")
}
```

- [ ] **Step 2: Run tests to verify they fail (red — compile error)**

Run: `go test ./sdk/pkg/eventbus/ -run TestConvertUserConfig_Pipeline -v`
Expected: FAIL / compile error — `unknown field 'Pipeline' in struct literal of type config.ConsumerConfig` and `undefined: config.PipelineUserConfig`.

- [ ] **Step 3: Add the user config surface (`PipelineUserConfig` + field)**

In `sdk/config/eventbus.go`, replace the existing `ConsumerConfig` block:

```go
// ConsumerConfig 消费者配置 - 用户配置层（简化）
// 只包含用户需要关心的核心配置字段
type ConsumerConfig struct {
	GroupID           string             `mapstructure:"groupId"`           // 消费者组ID
	AutoOffsetReset   string             `mapstructure:"autoOffsetReset"`   // 偏移量重置策略 (earliest, latest, none)
	SessionTimeout    time.Duration      `mapstructure:"sessionTimeout"`    // 会话超时时间
	HeartbeatInterval time.Duration      `mapstructure:"heartbeatInterval"` // 心跳间隔
	Pipeline          PipelineUserConfig `mapstructure:"pipeline"`          // 分区内消费流水线开关（默认关闭，灰度显式开启）
	// 移除了程序员应该控制的字段: MaxProcessingTime, FetchMinBytes, FetchMaxBytes, FetchMaxWait,
	// RebalanceStrategy, IsolationLevel, MaxPollRecords, EnableAutoCommit, AutoCommitInterval
}

// PipelineUserConfig 用户层流水线配置。仅暴露开关与并发旋钮；
// timing 类安全不变量（flushTimeout/dlqTimeout/stallWarnInterval）留内部默认，
// 由 applyPipelineDefaults 兜底——避免用户误配违反 FlushTimeout < sessionTimeout/2 而 panic。
type PipelineUserConfig struct {
	Enabled    bool `mapstructure:"enabled"`              // 功能开关，默认 false
	WindowSize int  `mapstructure:"windowSize,omitempty"` // 灰度并发旋钮；0 → 内部默认 16
}
```

- [ ] **Step 4: Run tests to verify they still fail (now compiles; passthrough missing)**

Run: `go test ./sdk/pkg/eventbus/ -run TestConvertUserConfig_Pipeline -v`
Expected: FAIL — compiles now, but `TestConvertUserConfig_PipelinePassthrough` fails: `Enabled must propagate` (Enabled still false because passthrough not added yet). `TestConvertUserConfig_PipelineDefaultOff` should already PASS.

- [ ] **Step 5: Add the passthrough in the convert layer**

In `sdk/pkg/eventbus/eventbus.go`, in `convertUserConfigToInternalKafkaConfig`'s `Consumer:` literal, add the `Pipeline:` field right after the four mapped user fields:

```go
		// 消费者配置转换
		Consumer: ConsumerConfig{
			// 用户配置字段 (直接映射)
			GroupID:           userConfig.Consumer.GroupID,
			AutoOffsetReset:   userConfig.Consumer.AutoOffsetReset,
			SessionTimeout:    userConfig.Consumer.SessionTimeout,
			HeartbeatInterval: userConfig.Consumer.HeartbeatInterval,
			Pipeline: PipelineConfig{
				Enabled:    userConfig.Consumer.Pipeline.Enabled,
				WindowSize: userConfig.Consumer.Pipeline.WindowSize,
				// FlushTimeout/DLQTimeout/StallWarnInterval 留零值，运行期由 applyPipelineDefaults 补
			},

			// 程序员设定的默认值 (用户不需要关心的技术细节)  ← 以下既有字段保持不变
```

- [ ] **Step 6: Run tests to verify they pass (green)**

Run: `go test ./sdk/pkg/eventbus/ -run TestConvertUserConfig_Pipeline -v`
Expected: PASS — both tests green.

- [ ] **Step 7: Commit**

```bash
git add sdk/config/eventbus.go sdk/pkg/eventbus/eventbus.go sdk/pkg/eventbus/config_regression_test.go
git commit -m "feat(eventbus): wire pipeline.{enabled,windowSize} from user config to internal KafkaConfig

Dead-switch fix (§4.1+§4.2): add PipelineUserConfig to sdk/config ConsumerConfig
and passthrough in convertUserConfigToInternalKafkaConfig. Timing fields stay
internal (runtime applyPipelineDefaults). Default off; no-op for configs without
a pipeline segment.

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 2: Construction-time validation + startup observability

**Files:**
- Modify: `sdk/pkg/eventbus/kafka.go` (`NewKafkaEventBus`, ~:245 — after the brokers guard, before `sarama.NewConfig`)
- Test: `sdk/pkg/eventbus/config_regression_test.go`

**Interfaces:**
- Consumes: Task 1's passthrough (so config-driven `Enabled=true` reaches here); `applyPipelineDefaults` (`type.go:663`); `PipelineConfig.validate(sessionTimeout)` (`type.go:643`); the `logger` package (`sdk/pkg/logger`, used bare at `kafka.go:459/471` — NOT `bus.logger`, which is `zap.NewNop()` at `:407`).
- Produces: `NewKafkaEventBus` returns `fmt.Errorf("invalid kafka consumer pipeline config: %w", err)` BEFORE `sarama.NewClient` (`:372`) when enabled+invalid; emits `logger.Info` (always, effective values) + `logger.Warn` (when LIVE).

- [ ] **Step 1: Write the failing validation test**

Append to `sdk/pkg/eventbus/config_regression_test.go`:

```go
// TestNewKafkaEventBus_PipelineValidationBeforeDial 钉死 §4.3 顺序不变量：
// Enabled=true + 非法 timing（SessionTimeout=8s → /2=4s，默认 FlushTimeout=4s 不满足 <）
// 必须在 sarama.NewClient 之前 return validation error。命中字符串（D4=A）——
// 仅断言 err!=nil 无法区分「校验先跑」与「校验被错排在 NewClient 之后、返回拨号错误」。
// 用例传非空 bogus broker（localhost:1 = 端口拒绝，快速）绕过 :243 守卫；校验先 return，永不到达拨号。
func TestNewKafkaEventBus_PipelineValidationBeforeDial(t *testing.T) {
	cfg := &KafkaConfig{
		Brokers: []string{"localhost:1"},
		Consumer: ConsumerConfig{
			SessionTimeout: 8 * time.Second, // 8/2=4s；默认 FlushTimeout=4s，4<4 不成立 → validate 报错
			Pipeline:       PipelineConfig{Enabled: true},
		},
	}

	bus, err := NewKafkaEventBus(cfg)

	require.Error(t, err, "invalid pipeline config must fail construction")
	assert.Nil(t, bus, "bus must not be returned on validation failure")
	assert.Contains(t, err.Error(), "pipeline",
		"error must come from pipeline validation, not a sarama dial (proves validation runs before NewClient)")
	assert.Contains(t, err.Error(), "invalid kafka consumer pipeline config",
		"error must carry the wrapped prefix")
}
```

- [ ] **Step 2: Run test to verify it fails (red — validation absent → reaches dial)**

Run: `go test ./sdk/pkg/eventbus/ -run TestNewKafkaEventBus_PipelineValidationBeforeDial -v`
Expected: FAIL — `NewKafkaEventBus` does not validate yet, so it proceeds to `sarama.NewClient("localhost:1")` and returns a dial/connection error that does NOT contain `"pipeline"`. The `assert.Contains ... "pipeline"` line fails. (Red may take a few seconds — sarama retries metadata on refused — that is expected and only exists until Step 3.)

- [ ] **Step 3: Add the validation + observability block (single `effective` derivation, P3)**

In `sdk/pkg/eventbus/kafka.go`, in `NewKafkaEventBus`, insert this block immediately after the brokers guard (`if len(cfg.Brokers) == 0 { ... }`, ~:243-245) and before `// 创建Sarama配置` (~:247). This is well before the first network touch, `sarama.NewClient` at :372:

```go
	// ⭐ 分区流水线配置（§4.3/§4.4，P3 单一派生）：校验 + 启动可观测共用同一 effective。
	// 顺序不变量：必须在任何 sarama 建连（:372 sarama.NewClient）之前——错误路径不触网、单测零 broker。
	effective := applyPipelineDefaults(cfg.Consumer.Pipeline)
	logger.Info("kafka consumer pipeline config",
		"pipelineEnabled", effective.Enabled,
		"pipelineWindowSize", effective.WindowSize) // 生效值（0→16 已补全），D3=A——on/off 都打
	if effective.Enabled {
		if err := effective.validate(cfg.Consumer.SessionTimeout); err != nil {
			return nil, fmt.Errorf("invalid kafka consumer pipeline config: %w", err)
		}
		logger.Warn("kafka consumer pipeline ENABLED — partition-pipeline + DLQ seam is LIVE; " +
			"ensure idempotent handlers, DLQ wiring, partition sizing, and assignor are ready")
	}
```

- [ ] **Step 4: Run test to verify it passes (green — validation returns before dial)**

Run: `go test ./sdk/pkg/eventbus/ -run TestNewKafkaEventBus_PipelineValidationBeforeDial -v`
Expected: PASS — returns instantly (validation fires before `NewClient`; no dial). Error contains both `"pipeline"` and `"invalid kafka consumer pipeline config"`.

- [ ] **Step 5: Commit**

```bash
git add sdk/pkg/eventbus/kafka.go sdk/pkg/eventbus/config_regression_test.go
git commit -m "feat(eventbus): fail-fast pipeline config validation + startup observability

In NewKafkaEventBus, before sarama.NewClient: derive single effective pipeline
config (applyPipelineDefaults), log effective enabled/windowSize, and validate
when enabled — returning a wrapped error instead of panicking lazily in
newPartitionPipeline on first message. Error path is network-free (zero-broker
unit test). Adds LIVE WARN when enabled. (§4.3+§4.4, P3 colocated.)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 3: Full build + scoped regression + manual log checkpoint

**Files:** none modified (verification only).

- [ ] **Step 1: Compile the whole module**

Run: `go build ./...`
Expected: exits 0, no errors.

- [ ] **Step 2: Run all Pipeline-scoped regression tests together**

Run: `go test ./sdk/pkg/eventbus/ -run Pipeline -v`
Expected: PASS — `TestConvertUserConfig_PipelinePassthrough`, `TestConvertUserConfig_PipelineDefaultOff`, `TestNewKafkaEventBus_PipelineValidationBeforeDial`, plus the existing `TestPipelineConfig_Defaults` / `TestApplyPipelineDefaults` / `partition_pipeline` tests. Zero broker required.

- [ ] **Step 3: gofmt the touched files (scoped — repo uses core.autocrlf=true)**

Run: `gofmt -w sdk/config/eventbus.go sdk/pkg/eventbus/eventbus.go sdk/pkg/eventbus/kafka.go sdk/pkg/eventbus/config_regression_test.go`
Expected: no output (already formatted) or a clean rewrite; `git diff` shows only formatting if any.

- [ ] **Step 4: Manual log checkpoint (requires a broker — documented, not automated)**

This is spec §7.3. With a running Kafka/RedPanda and a service using `InitializeFromConfig`:
- `settings.yml` without `pipeline:` → startup log shows `pipelineEnabled=false`, behavior unchanged (legacy serial).
- `settings.yml` with `eventbus.kafka.consumer.pipeline.enabled: true` → startup log shows `pipelineEnabled=true` + the LIVE WARN, and `consumeWithPipeline` is reached.

Record the observed log lines in the PR description. (If no broker is available in CI, this remains a manual pre-release check — do not block the automated gate on it.)

- [ ] **Step 5: Commit formatting if any**

```bash
git add sdk/config/eventbus.go sdk/pkg/eventbus/eventbus.go sdk/pkg/eventbus/kafka.go sdk/pkg/eventbus/config_regression_test.go
git commit -m "style(eventbus): gofmt pipeline dead-switch touch points

Co-Authored-By: Claude <noreply@anthropic.com>" || echo "nothing to format-commit"
```

---

## Release Gate (cross-repo — evidence-management owned, NOT jxt-core code)

Per spec §5/§6 (decisions D2=A, D5=A, P2). These are prerequisites for tagging **v1.1.70 as a no-op**; they live in the evidence-management repo and are coordinated in the same release window. The jxt-core release must NOT tag v1.1.70 until these are confirmed (or the activation is explicitly disclosed in release notes).

- **Verification is a blocking artifact, not informal confirmation:** before tagging v1.1.70, record in the jxt-core release notes a linked evidence-management PR URL or commit SHA showing BOTH `command/config/settings.yml` and `query/config/settings.yml` at `eventbus.kafka.consumer.pipeline.enabled: false` (the load-bearing no-op flip), plus `command/config/settings.yml` at `reliable.live_enabled: false` (command-only; query has no `reliable/` package). Also record whether evidence-management's Task 8 PR-3 canary lands in the v1.1.70 window — that determines whether `live_enabled: false` is hygiene or load-bearing (see D2=A). The tag is blocked until that artifact is linked — "confirmed" means the artifact exists, not that someone said the flip was done.
- **D2=A — flip `pipeline.enabled` to false (load-bearing no-op); `reliable.live_enabled: false` is command-only hygiene pending Task 8:** `evidence-management/command/config/settings.yml` and `query/config/settings.yml` currently commit `eventbus.kafka.consumer.pipeline.enabled: true` (windowSize 32/64). The dead switch is the ONLY thing keeping them on legacy serial; without flipping `pipeline.enabled` to false, bumping the jxt-core pin to v1.1.70 silently activates partition-pipeline + DLQ seam. **`pipeline.enabled: false` ALONE is the no-op for v1.1.70** — verified against the evidence-management code (round-2 review): both command prod handlers (`process_task_event_handler.go:55`, `file_storage_event_handler.go:52`) subscribe via `SubscribeEnvelopeWithDLQ` (PR-0), which has no pipeline guard; the R2-6 fail-fast (`subscriber.go:162`) lives inside `SubscribeEnvelopeDeliveryWithReliable`, which has ZERO production callers (the PR-3 canary is future Task 8 work — `startup.go:21-23`); and `reliable.live_enabled` is read only at a startup log line (`server.go:133`), not in any routing branch (`reliable/config.go:29-32` is a struct-field comment, not executable routing). So:
  - `eventbus.kafka.consumer.pipeline.enabled: false` (command AND query) — REQUIRED, load-bearing; keeps the bus on legacy serial (dormant). This is the v1.1.70 no-op.
  - `reliable.live_enabled: false` (command ONLY — query has no `reliable/` package, so no such key) — HYGIENE for v1.1.70, not load-bearing (no handler is on the PR-3 path today). Recommended so that WHEN Task 8 wires the `SubscribeEnvelopeDeliveryWithReliable` canary, command's reliable consumption is already on the PR-0 byte-faithful path rather than fail-refusing on R2-6. **Confirm before tagging whether Task 8 lands in the v1.1.70 window**: if YES, promote this to REQUIRED (load-bearing); if NO, it stays hygiene.
  - KEEP `reliable.scheduler_enabled: true` — RETRY_SCHEDULED rows are already broker-ACKed and will NOT be redelivered; only the scheduler drains them. Flipping it false is the R2-3 data-loss mode (`reliable/config.go:35-37`).
  - Net: jxt-core v1.1.70 ships as a no-op via `pipeline.enabled: false` alone; the command-only `live_enabled: false` + `scheduler_enabled: true` pair is evidence-management's OWN designed rollback shape (`reliable/config.go:23-27`), pre-positioned for the Task 8 canary — not a present-tense runtime coupling.
- **D5=A — strip/re-comment 3 timing keys:** those two settings files also commit `flushTimeout`/`dlqTimeout`/`stallWarnInterval` (query ~:111-112, command ~:122), which have NO decode target and are silently dropped by `sdk/config/config.go:56` bare `v.Unmarshal` (same mechanism as the dead switch). Zero regression today only because the committed values equal `defaultPipelineConfig()`. When editing for D2, also strip or re-comment these 3 keys as `# ignored — jxt-core applies internal defaults; only enabled/windowSize are read`.
- **P2 — coordination scope is THREE kafka consumers:** evidence-management `command`, `query`, AND `process-management` (`type: kafka`). `process-management` has no `pipeline:` segment today (won't activate), but it copied command's `groupId` (`evidence-command-consumer-group`) — that consumer-group conflict is a separate process-management bug, out of scope here.
- **Release notes must state:** jxt-core v1.1.70 ships as a no-op for the library (toggle reachable; every config WITHOUT a `pipeline:` segment — process-management / file-storage / security / tenant — is unaffected and default-off); evidence-management flips `pipeline.enabled=false` (command AND query — the load-bearing no-op) plus a command-only `reliable.live_enabled=false` (keep `scheduler_enabled` true) that is inert today (no handler is on the PR-3 path) but pre-positions command's reliable path for the future Task 8 canary; query needs only `pipeline.enabled=false` (no `reliable/` package). NOTE: `pipeline.enabled=false` ALONE is the real v1.1.70 no-op — the `live_enabled` coupling is forward-looking, not present-tense (round-2 review corrected the v4 rationale, which asserted a PR-3 fail-fast that is currently unwired). Activation is a separate, precondition-gated release (4 preconditions: idempotent handlers, DLQ wiring, partition expansion, eager assignor; F1 idempotency audit still open).

---

## Self-Review (completed)

- **Spec coverage:** §4.1 (user surface) → Task 1 Step 3; §4.2 (passthrough) → Task 1 Step 5; §4.3 (fail-fast, before dial) → Task 2 Step 3; §4.4 (observability, single `effective`) → Task 2 Step 3 (colocated, P3); §4.5 (roundtrip + D4 string assertion) → Task 1 Step 1 + Task 2 Step 1; §5/§6 (D2/D5/P2 release gate) → Release Gate section (D2=A: `pipeline.enabled: false` is the load-bearing no-op; `reliable.live_enabled: false` is command-only Task-8 hygiene, not a present-tense coupling — see Release Gate; round-2 review corrected the v4 rationale); §7 (build/scoped/manual) → Task 3. No spec section unaccounted for.
- **Placeholder scan:** no TBD/TODO/"add validation"/"similar to". Every code step shows full code; every test step shows the test.
- **Type consistency:** `config.PipelineUserConfig` (user, Task 1) vs internal `PipelineConfig` (existing) — passthrough maps `Enabled`/`WindowSize` only; `effective := applyPipelineDefaults(...)` returns internal `PipelineConfig` whose `.validate(sessionTimeout)` and `.Enabled`/`.WindowSize` are used consistently in Task 2. `logger.Info/Warn` matches existing bare usage at `kafka.go:459/471` (imported `sdk/pkg/logger` package). Test identifiers use only already-imported symbols (`config`, `time`, `assert`, `require`) plus same-package internals.
