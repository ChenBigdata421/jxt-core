# Kafka EventBus Hollywood Actor Pool - 配置设计方案

> **目标**: 设计简洁、向后兼容、易于灰度的配置方案

---

## 🎯 **设计原则**

### 1. **向后兼容**
- 不破坏现有配置
- 默认使用 Keyed Worker Pool
- 新增配置可选

### 2. **简洁优先**
- 最小化用户配置
- 合理的默认值
- 高级配置可选

### 3. **灰度友好**
- 支持环境变量快速切换
- 支持配置文件精细控制
- 支持运行时观测

---

## 📊 **配置需求分析**

### 当前硬编码值

| 参数 | Keyed Pool | Actor Pool | 是否需要配置 |
|------|-----------|-----------|-------------|
| **WorkerCount / PoolSize** | 256 | 256 | ❌ 否 (保持一致) |
| **QueueSize / InboxSize** | 1000 | 1000 | ❌ 否 (保持一致) |
| **WaitTimeout** | 500ms | N/A | ❌ 否 (Actor Pool 不需要) |
| **MaxRestarts** | N/A | 3 | ⚠️ 可选 (默认 3) |
| **UseActorPool** | N/A | false | ✅ 是 (功能开关) |

### 核心结论

**只需要 1 个必需配置: `useActorPool` (功能开关)**

**1 个可选配置: `maxRestarts` (Supervisor 重启次数)**

---

## 🏗️ **配置方案设计**

### 方案 A: 最小化配置 (推荐) ⭐⭐⭐⭐⭐

**理念**: 只暴露功能开关，其他参数使用合理默认值

#### 配置结构

```go
// KafkaConfig (sdk/config/eventbus.go)
type KafkaConfig struct {
    Brokers  []string       `mapstructure:"brokers"`
    Producer ProducerConfig `mapstructure:"producer"`
    Consumer ConsumerConfig `mapstructure:"consumer"`
    
    // ⭐ 新增: Actor Pool 配置 (可选)
    ActorPool ActorPoolConfig `mapstructure:"actorPool"`
}

// ActorPoolConfig Actor Pool 配置
type ActorPoolConfig struct {
    Enabled bool `mapstructure:"enabled"` // 是否启用 Actor Pool (默认: false)
}
```

#### YAML 配置示例

```yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]
    producer:
      requiredAcks: 1
      timeout: 10s
    consumer:
      groupId: "my-group"
    
    # ⭐ Actor Pool 配置 (可选)
    actorPool:
      enabled: false  # 默认使用 Keyed Pool
```

#### 优点
- ✅ 配置极简 (只有 1 个字段)
- ✅ 向后兼容 (不配置 = 使用 Keyed Pool)
- ✅ 易于理解
- ✅ 灰度友好

#### 缺点
- ⚠️ 无法调整 MaxRestarts (固定为 3)
- ⚠️ 无法调整 PoolSize/InboxSize (与 Keyed Pool 一致)

---

### 方案 B: 完整配置

**理念**: 暴露所有可调参数，给高级用户更多控制

#### 配置结构

```go
// ActorPoolConfig Actor Pool 配置
type ActorPoolConfig struct {
    Enabled     bool `mapstructure:"enabled"`     // 是否启用 (默认: false)
    PoolSize    int  `mapstructure:"poolSize"`    // Actor 数量 (默认: 256)
    InboxSize   int  `mapstructure:"inboxSize"`   // Inbox 大小 (默认: 1000)
    MaxRestarts int  `mapstructure:"maxRestarts"` // 最大重启次数 (默认: 3)
}
```

#### YAML 配置示例

```yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]
    
    # ⭐ Actor Pool 配置 (可选)
    actorPool:
      enabled: true      # 启用 Actor Pool
      poolSize: 512      # 自定义 Pool 大小
      inboxSize: 2000    # 自定义 Inbox 大小
      maxRestarts: 5     # 自定义重启次数
```

#### 优点
- ✅ 灵活性高
- ✅ 可调优

#### 缺点
- ❌ 配置复杂
- ❌ 容易配置错误 (PoolSize 和 InboxSize 应该和 Keyed Pool 一致)
- ❌ 增加维护成本

---

### 方案 C: 混合方案 (平衡) ⭐⭐⭐⭐

**理念**: 功能开关 + 少量高级配置

#### 配置结构

```go
// ActorPoolConfig Actor Pool 配置
type ActorPoolConfig struct {
    Enabled     bool `mapstructure:"enabled"`     // 是否启用 (默认: false)
    MaxRestarts int  `mapstructure:"maxRestarts"` // 最大重启次数 (默认: 3)
    // PoolSize 和 InboxSize 自动与 Keyed Pool 保持一致 (256, 1000)
}
```

#### YAML 配置示例

```yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]
    
    # ⭐ Actor Pool 配置 (可选)
    actorPool:
      enabled: false     # 默认使用 Keyed Pool
      maxRestarts: 3     # Supervisor 重启次数 (可选)
```

#### 优点
- ✅ 配置简洁 (2 个字段)
- ✅ 向后兼容
- ✅ 保证 PoolSize/InboxSize 一致性
- ✅ 允许调整 MaxRestarts

#### 缺点
- ⚠️ 无法调整 PoolSize/InboxSize (但这是优点，确保公平对比)

---

## 🎯 **推荐方案**

### **方案 A (最小化配置)** ⭐⭐⭐⭐⭐

**理由**:
1. ✅ **千万级聚合ID场景**: PoolSize 必须固定 (256/512/1024)，不应该让用户随意调整
2. ✅ **公平对比**: PoolSize 和 InboxSize 必须和 Keyed Pool 一致
3. ✅ **MaxRestarts = 3 足够**: 大多数场景下 3 次重启是合理的
4. ✅ **简化运维**: 配置越少，出错概率越低

### 实施细节

#### 1. 配置结构 (sdk/config/eventbus.go)

```go
// KafkaConfig Kafka配置
type KafkaConfig struct {
    Brokers  []string       `mapstructure:"brokers"`
    Producer ProducerConfig `mapstructure:"producer"`
    Consumer ConsumerConfig `mapstructure:"consumer"`
    
    // ⭐ 新增: Actor Pool 配置 (可选)
    ActorPool ActorPoolConfig `mapstructure:"actorPool"`
}

// ActorPoolConfig Actor Pool 配置
type ActorPoolConfig struct {
    // 功能开关: true = 使用 Actor Pool, false = 使用 Keyed Pool
    Enabled bool `mapstructure:"enabled"` // 默认: false
}
```

#### 2. 默认值设置 (sdk/config/eventbus.go)

```go
func (c *KafkaConfig) SetDefaults() {
    // ... 现有默认值 ...
    
    // ⭐ Actor Pool 默认值
    // Enabled 默认为 false (使用 Keyed Pool)
    // PoolSize 和 InboxSize 自动与 Keyed Pool 一致 (256, 1000)
    // MaxRestarts 固定为 3
}
```

#### 3. 环境变量支持 (向后兼容)

```go
// 优先级: 配置文件 > 环境变量
if os.Getenv("KAFKA_USE_ACTOR_POOL") == "true" {
    bus.useActorPool.Store(true)
} else if cfg.Kafka.ActorPool.Enabled {
    bus.useActorPool.Store(true)
} else {
    bus.useActorPool.Store(false)
}
```

---

## 📝 **配置示例**

### 示例 1: 默认配置 (使用 Keyed Pool)

```yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]
    producer:
      requiredAcks: 1
      timeout: 10s
    consumer:
      groupId: "my-group"
    # 不配置 actorPool = 使用 Keyed Pool
```

### 示例 2: 启用 Actor Pool

```yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]
    producer:
      requiredAcks: 1
      timeout: 10s
    consumer:
      groupId: "my-group"
    
    # ⭐ 启用 Actor Pool
    actorPool:
      enabled: true
```

### 示例 3: 环境变量覆盖

```bash
# 配置文件中 actorPool.enabled = false
# 但环境变量可以覆盖
export KAFKA_USE_ACTOR_POOL=true

# 启动应用
./my-service
```

---

## 🔄 **迁移路径**

### 阶段 1: 开发环境 (配置文件)

```yaml
actorPool:
  enabled: true
```

### 阶段 2: 测试环境 (环境变量灰度)

```bash
# 10% 实例启用 Actor Pool
export KAFKA_USE_ACTOR_POOL=true

# 90% 实例使用 Keyed Pool (默认)
```

### 阶段 3: 生产环境 (配置文件灰度)

```yaml
# 50% 实例
actorPool:
  enabled: true

# 50% 实例
actorPool:
  enabled: false
```

### 阶段 4: 全量上线

```yaml
# 所有实例
actorPool:
  enabled: true
```

---

## ✅ **总结**

### 推荐配置

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `kafka.actorPool.enabled` | bool | `false` | 是否启用 Actor Pool |

### 固定值 (不可配置)

| 参数 | 值 | 说明 |
|------|-----|------|
| `PoolSize` | 256 | 与 Keyed Pool 一致 |
| `InboxSize` | 1000 | 与 Keyed Pool 一致 |
| `MaxRestarts` | 3 | Supervisor 重启次数 |

### 优势

- ✅ **极简配置**: 只有 1 个字段
- ✅ **向后兼容**: 不配置 = 使用 Keyed Pool
- ✅ **灰度友好**: 环境变量 + 配置文件双支持
- ✅ **公平对比**: PoolSize/InboxSize 强制一致
- ✅ **易于维护**: 配置少 = 出错少

---

## 🚀 **下一步**

1. ✅ 在 `sdk/config/eventbus.go` 中添加 `ActorPoolConfig`
2. ✅ 在 `kafka.go` 中读取配置
3. ✅ 更新文档和示例
4. ✅ 编写配置验证测试

**你同意这个方案吗？如果同意，我们开始实施。** 🤔

