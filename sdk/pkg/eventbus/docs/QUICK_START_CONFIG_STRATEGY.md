# 主题配置策略 - 快速开始

## 🚀 5分钟快速上手

### 1. 基本使用（默认行为）

```go
package main

import (
    "context"
    "time"
    
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    ctx := context.Background()
    
    // 创建 EventBus
    bus, _ := eventbus.InitializeFromConfig(cfg)
    defer bus.Close()
    
    // 配置主题（幂等操作，可以多次调用）
    bus.ConfigureTopic(ctx, "order.created", eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   24 * time.Hour,
    })
    
    // 使用主题
    bus.Publish(ctx, "order.created", []byte("message"))
}
```

**就这么简单！** 默认行为是 `CreateOrUpdate`，适合大多数场景。

---

### 2. 生产环境使用

```go
func main() {
    bus, _ := eventbus.InitializeFromConfig(cfg)
    defer bus.Close()
    
    // 🔒 生产环境：只创建，不更新
    bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)
    
    // 配置主题
    bus.ConfigureTopic(ctx, "order.created", eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   24 * time.Hour,
        Replicas:        3, // 生产环境建议3副本
    })
}
```

**为什么？** 生产环境配置不应该被意外修改。

---

### 3. 开发环境使用

```go
func main() {
    bus, _ := eventbus.InitializeFromConfig(cfg)
    defer bus.Close()
    
    // 🔧 开发环境：自动创建和更新
    bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
    
    // 第一次配置
    bus.ConfigureTopic(ctx, "dev.test", eventbus.TopicOptions{
        RetentionTime: 1 * time.Hour,
    })
    
    // 修改配置（会自动更新）
    bus.ConfigureTopic(ctx, "dev.test", eventbus.TopicOptions{
        RetentionTime: 2 * time.Hour, // ✅ 会更新
    })
}
```

**为什么？** 开发环境需要快速迭代，自动更新配置很方便。

---

### 4. 配置验证

```go
func main() {
    bus, _ := eventbus.InitializeFromConfig(cfg)
    defer bus.Close()
    
    // 🔍 严格模式：只验证，不修改
    bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
    
    // 验证配置
    err := bus.ConfigureTopic(ctx, "order.created", eventbus.TopicOptions{
        RetentionTime: 24 * time.Hour,
    })
    
    if err != nil {
        log.Fatalf("❌ 配置验证失败: %v", err)
    }
    
    log.Println("✅ 配置验证通过")
}
```

**为什么？** 确保配置符合预期，避免配置漂移。

---

## 📋 配置策略速查表

| 策略 | 使用场景 | 行为 | 代码 |
|------|---------|------|------|
| **CreateOrUpdate** | 开发/测试 | 创建或更新 | `StrategyCreateOrUpdate` |
| **CreateOnly** | 生产环境 | 只创建 | `StrategyCreateOnly` |
| **ValidateOnly** | 配置审计 | 只验证 | `StrategyValidateOnly` |
| **Skip** | 性能优先 | 跳过检查 | `StrategySkip` |

---

## 🎯 常见场景

### 场景1: 应用启动时配置所有主题

```go
func initTopics(bus eventbus.EventBus) error {
    ctx := context.Background()
    
    topics := map[string]eventbus.TopicOptions{
        "order.created": {
            PersistenceMode: eventbus.TopicPersistent,
            RetentionTime:   24 * time.Hour,
            Replicas:        3,
        },
        "notification.sent": {
            PersistenceMode: eventbus.TopicEphemeral,
        },
    }
    
    for topic, opts := range topics {
        if err := bus.ConfigureTopic(ctx, topic, opts); err != nil {
            return fmt.Errorf("配置主题 %s 失败: %w", topic, err)
        }
    }
    
    return nil
}
```

### 场景2: 根据环境选择策略

```go
func setupEventBus(env string) eventbus.EventBus {
    bus, _ := eventbus.InitializeFromConfig(cfg)
    
    switch env {
    case "production":
        bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)
    case "staging":
        bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
    default:
        bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
    }
    
    return bus
}
```

### 场景3: 配置验证失败时的处理

```go
func validateAndConfigure(bus eventbus.EventBus) error {
    // 先验证
    bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
    
    err := bus.ConfigureTopic(ctx, "orders", expectedOptions)
    if err != nil {
        log.Printf("⚠️  配置不一致: %v", err)
        
        // 询问用户是否更新
        if askUserConfirmation("是否更新配置？") {
            bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
            return bus.ConfigureTopic(ctx, "orders", expectedOptions)
        }
        
        return err
    }
    
    return nil
}
```

---

## ⚡ 性能提示

### 提示1: Memory EventBus 使用 Skip 策略

```go
// Memory EventBus 不需要配置检查
if cfg.Type == "memory" {
    bus.SetTopicConfigStrategy(eventbus.StrategySkip)
}
```

### 提示2: 批量配置时使用 Skip

```go
// 批量配置时跳过检查，提升性能
bus.SetTopicConfigStrategy(eventbus.StrategySkip)

for i := 0; i < 1000; i++ {
    bus.ConfigureTopic(ctx, fmt.Sprintf("topic.%d", i), opts)
}

// 恢复正常策略
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
```

---

## 🔍 调试技巧

### 查看当前策略

```go
strategy := bus.GetTopicConfigStrategy()
log.Printf("当前策略: %s", strategy)
```

### 查看主题配置

```go
config, err := bus.GetTopicConfig("order.created")
if err == nil {
    log.Printf("主题配置: %+v", config)
}
```

### 列出所有已配置主题

```go
topics := bus.ListConfiguredTopics()
log.Printf("已配置主题: %v", topics)
```

---

## ❓ 常见问题

### Q: 默认策略是什么？
**A**: `StrategyCreateOrUpdate`（创建或更新）

### Q: 可以动态切换策略吗？
**A**: 可以，随时调用 `SetTopicConfigStrategy()`

### Q: 配置会持久化吗？
**A**: 不需要。代码是配置的唯一来源，Kafka/NATS 会持久化配置

### Q: 重启后配置会丢失吗？
**A**: 不会。每次启动都会重新应用配置（幂等）

### Q: 配置不一致会怎样？
**A**: 根据策略：
- `CreateOrUpdate`: 自动更新
- `CreateOnly`: 使用现有配置
- `ValidateOnly`: 记录警告或返回错误
- `Skip`: 忽略

---

## 📚 更多资源

- [完整使用指南](./TOPIC_CONFIG_STRATEGY.md)
- [实现总结](./IMPLEMENTATION_SUMMARY.md)
- [示例代码](./examples/topic_config_strategy_example.go)
- [单元测试](./topic_config_manager_test.go)

---

## 🎉 开始使用

1. **复制示例代码**
2. **根据环境选择策略**
3. **配置主题**
4. **开始发布/订阅**

就这么简单！🚀


