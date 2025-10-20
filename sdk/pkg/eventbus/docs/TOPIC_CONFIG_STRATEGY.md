# 主题配置策略 (Topic Configuration Strategy)

## 📋 概述

主题配置策略功能提供了**幂等的主题配置管理**，确保 EventBus 代码配置与消息中间件（Kafka/NATS）配置的一致性。

### 核心特性

✅ **幂等配置**: `ConfigureTopic()` 可以安全地多次调用  
✅ **智能策略**: 支持4种配置策略，适应不同环境  
✅ **配置验证**: 自动检测配置漂移和不一致  
✅ **灵活控制**: 支持创建、更新、验证、跳过等行为  
✅ **无需持久化**: 配置由代码管理，无需额外存储  

---

## 🎯 设计理念

### 问题背景

在之前的实现中：
- 主题配置只存储在内存中
- 每次重启需要重新配置
- 配置变更可能导致不一致

### 解决方案

**核心观点**: 
1. **代码是配置的唯一来源** (Single Source of Truth)
2. **消息中间件已经持久化配置** (Kafka Topics, NATS Streams)
3. **需要的是配置同步机制**，而不是持久化存储

**实现方式**:
- 幂等配置：确保配置可以重复应用
- 智能策略：根据环境选择不同的配置行为
- 配置验证：启动时验证配置一致性

---

## 📚 配置策略

### 1. StrategyCreateOnly (只创建)

**适用场景**: 生产环境  
**行为**: 只创建新主题，不更新现有配置  
**优点**: 安全，避免意外修改生产配置  

```go
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)

// 第一次调用：创建主题
bus.ConfigureTopic(ctx, "orders", eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicPersistent,
    RetentionTime:   24 * time.Hour,
})

// 第二次调用：不会更新，使用现有配置
bus.ConfigureTopic(ctx, "orders", eventbus.TopicOptions{
    RetentionTime: 48 * time.Hour, // 不会生效
})
```

### 2. StrategyCreateOrUpdate (创建或更新)

**适用场景**: 开发环境、测试环境  
**行为**: 创建新主题或更新现有配置  
**优点**: 灵活，支持配置迭代  

```go
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)

// 第一次调用：创建主题
bus.ConfigureTopic(ctx, "orders", eventbus.TopicOptions{
    RetentionTime: 24 * time.Hour,
})

// 第二次调用：更新配置
bus.ConfigureTopic(ctx, "orders", eventbus.TopicOptions{
    RetentionTime: 48 * time.Hour, // 会更新
})
```

### 3. StrategyValidateOnly (只验证)

**适用场景**: 严格模式、配置审计  
**行为**: 只验证配置一致性，不修改  
**优点**: 确保配置符合预期  

```go
bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)

// 验证配置是否一致
err := bus.ConfigureTopic(ctx, "orders", eventbus.TopicOptions{
    RetentionTime: 24 * time.Hour,
})

if err != nil {
    // 配置不一致，需要人工介入
    log.Fatalf("配置验证失败: %v", err)
}
```

### 4. StrategySkip (跳过检查)

**适用场景**: 性能优先、Memory EventBus  
**行为**: 跳过所有配置检查  
**优点**: 最快，无额外开销  

```go
bus.SetTopicConfigStrategy(eventbus.StrategySkip)

// 跳过所有检查，直接返回
bus.ConfigureTopic(ctx, "orders", eventbus.TopicOptions{
    RetentionTime: 24 * time.Hour,
})
```

---

## 🔧 使用方法

### 基本用法

```go
package main

import (
    "context"
    "time"
    
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    ctx := context.Background()
    
    // 1. 创建 EventBus
    bus, err := eventbus.InitializeFromConfig(cfg)
    if err != nil {
        panic(err)
    }
    defer bus.Close()
    
    // 2. 设置配置策略（可选，默认为 CreateOrUpdate）
    bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
    
    // 3. 配置主题（幂等操作）
    err = bus.ConfigureTopic(ctx, "order.created", eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   24 * time.Hour,
        MaxMessages:     10000,
        Replicas:        3,
    })
    if err != nil {
        panic(err)
    }
    
    // 4. 使用主题
    bus.Publish(ctx, "order.created", []byte("message"))
}
```

### 环境特定配置

```go
// 根据环境设置不同的策略
func initEventBus(env string) eventbus.EventBus {
    bus, _ := eventbus.InitializeFromConfig(cfg)
    
    switch env {
    case "production":
        // 生产环境：只创建，不更新
        bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)
        
    case "staging":
        // 预发布环境：验证配置一致性
        bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
        
    case "development":
        // 开发环境：自动创建和更新
        bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
        
    default:
        // 默认：创建或更新
        bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
    }
    
    return bus
}
```

### 配置不一致处理

```go
// 配置主题
err := bus.ConfigureTopic(ctx, "orders", eventbus.TopicOptions{
    RetentionTime: 24 * time.Hour,
})

if err != nil {
    // 处理配置错误
    log.Printf("配置失败: %v", err)
    
    // 可以选择：
    // 1. 忽略错误，使用现有配置
    // 2. 重试配置
    // 3. 终止启动
}

// 查看当前配置
config, _ := bus.GetTopicConfig("orders")
log.Printf("当前配置: %+v", config)
```

---

## 📊 配置策略对比

| 策略 | 创建新主题 | 更新现有配置 | 验证配置 | 性能 | 适用环境 |
|------|-----------|-------------|---------|------|---------|
| **CreateOnly** | ✅ | ❌ | ❌ | 快 | 生产环境 |
| **CreateOrUpdate** | ✅ | ✅ | ✅ | 中 | 开发/测试 |
| **ValidateOnly** | ❌ | ❌ | ✅ | 快 | 严格模式 |
| **Skip** | ❌ | ❌ | ❌ | 最快 | 性能优先 |

---

## 🎨 最佳实践

### 1. 生产环境配置

```go
// 生产环境：使用只创建策略
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)

// 配置所有主题
topics := []struct{
    name string
    opts eventbus.TopicOptions
}{
    {"order.created", eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   24 * time.Hour,
        Replicas:        3,
    }},
    {"notification.sent", eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicEphemeral,
    }},
}

for _, topic := range topics {
    if err := bus.ConfigureTopic(ctx, topic.name, topic.opts); err != nil {
        log.Printf("配置主题 %s 失败: %v", topic.name, err)
    }
}
```

### 2. 开发环境配置

```go
// 开发环境：使用创建或更新策略
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)

// 可以随时修改配置
bus.ConfigureTopic(ctx, "dev.test", eventbus.TopicOptions{
    RetentionTime: 1 * time.Hour,
})

// 后续修改会自动更新
bus.ConfigureTopic(ctx, "dev.test", eventbus.TopicOptions{
    RetentionTime: 2 * time.Hour, // 会更新
})
```

### 3. 配置验证

```go
// 启动时验证所有主题配置
func validateTopicConfigs(bus eventbus.EventBus) error {
    // 临时切换到验证模式
    originalStrategy := bus.GetTopicConfigStrategy()
    bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
    defer bus.SetTopicConfigStrategy(originalStrategy)
    
    // 验证所有主题
    for _, topic := range expectedTopics {
        if err := bus.ConfigureTopic(ctx, topic.name, topic.opts); err != nil {
            return fmt.Errorf("主题 %s 配置不一致: %w", topic.name, err)
        }
    }
    
    return nil
}
```

---

## 🔍 配置不一致检测

### 自动检测

系统会自动检测以下配置不一致：

- ✅ 持久化模式 (PersistenceMode)
- ✅ 保留时间 (RetentionTime)
- ✅ 最大大小 (MaxSize)
- ✅ 最大消息数 (MaxMessages)
- ✅ 副本数 (Replicas)

### 不一致处理

```go
// 配置不一致时的行为
type TopicConfigMismatchAction struct {
    LogLevel string // debug, info, warn, error
    FailFast bool   // 是否立即失败
}

// 示例：严格模式
action := TopicConfigMismatchAction{
    LogLevel: "error",
    FailFast: true, // 发现不一致立即失败
}

// 示例：宽松模式
action := TopicConfigMismatchAction{
    LogLevel: "warn",
    FailFast: false, // 只记录警告，继续运行
}
```

---

## 📝 示例代码

完整示例请参考：
- `examples/topic_config_strategy_example.go` - 完整使用示例
- `topic_config_manager_test.go` - 单元测试

---

## 🚀 迁移指南

### 从旧版本迁移

**旧代码**:
```go
// 每次启动都需要配置
bus.ConfigureTopic(ctx, "orders", options)
```

**新代码**:
```go
// 设置策略（可选）
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)

// 幂等配置（可以多次调用）
bus.ConfigureTopic(ctx, "orders", options)
```

**无需修改**: 现有代码无需修改，默认行为与之前相同（CreateOrUpdate）

---

## ❓ FAQ

### Q1: 为什么不需要持久化存储？

**A**: 因为：
1. 代码是配置的唯一来源
2. Kafka/NATS 已经持久化了配置
3. 每次启动都会重新应用配置

### Q2: 配置策略什么时候生效？

**A**: 调用 `ConfigureTopic()` 时立即生效

### Q3: 可以动态切换策略吗？

**A**: 可以，调用 `SetTopicConfigStrategy()` 即可

### Q4: 配置不一致会怎样？

**A**: 根据策略和配置：
- `CreateOrUpdate`: 自动更新
- `CreateOnly`: 使用现有配置
- `ValidateOnly`: 记录警告或返回错误
- `Skip`: 忽略

---

## 📚 相关文档

- [EventBus 使用指南](./README.md)
- [主题持久化管理](./TOPIC_PERSISTENCE.md)
- [代码检视报告](./CODE_REVIEW_REPORT.md)


