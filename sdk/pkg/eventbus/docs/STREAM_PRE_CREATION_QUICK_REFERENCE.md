# NATS Stream 预创建优化 - 快速参考卡片

## 🚀 一分钟快速开始

```go
// 1. 创建 EventBus
bus, _ := eventbus.NewNATSEventBus(config)

// 2. 预创建阶段：使用 StrategyCreateOnly
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)
for _, topic := range topics {
    bus.ConfigureTopic(ctx, topic, eventbus.TopicOptions{
        PersistenceMode: eventbus.TopicPersistent,
        RetentionTime:   24 * time.Hour,
        MaxMessages:     10000,
    })
}

// 3. 运行时阶段：切换到 StrategySkip
bus.SetTopicConfigStrategy(eventbus.StrategySkip)

// 4. 发布消息（零 RPC 开销）
bus.Publish(ctx, topic, message)
```

---

## 📊 性能对比

| 场景 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| 吞吐量 | 117 msg/s | 69,444 msg/s | **595倍** |
| 延迟 | 8.5ms/msg | 0.014ms/msg | **607倍** |
| RPC调用 | 每次Publish | 零调用 | **100%减少** |

---

## 🎯 配置策略速查

| 策略 | 何时使用 | 性能 |
|------|---------|------|
| `StrategyCreateOnly` | 生产环境预创建 | ⭐⭐⭐⭐ |
| `StrategyCreateOrUpdate` | 开发环境调试 | ⭐⭐⭐ |
| `StrategyValidateOnly` | 预发布验证 | ⭐⭐⭐⭐ |
| `StrategySkip` | 运行时发布 | ⭐⭐⭐⭐⭐ |

---

## 🔧 推荐流程

```
应用启动
  ↓
设置 StrategyCreateOnly
  ↓
预创建所有 Stream
  ↓
切换到 StrategySkip
  ↓
运行时发布（零 RPC 开销）
```

---

## ⚠️ 常见错误

### ❌ 错误做法

```go
// 错误1: 没有预创建就使用 StrategySkip
bus.SetTopicConfigStrategy(eventbus.StrategySkip)
bus.Publish(ctx, topic, message) // ❌ 可能失败

// 错误2: 运行时仍使用 StrategyCreateOrUpdate
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
for i := 0; i < 10000; i++ {
    bus.Publish(ctx, topic, message) // ❌ 性能差
}
```

### ✅ 正确做法

```go
// 正确1: 先预创建，再使用 StrategySkip
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)
bus.ConfigureTopic(ctx, topic, options)
bus.SetTopicConfigStrategy(eventbus.StrategySkip)
bus.Publish(ctx, topic, message) // ✅ 性能最优

// 正确2: 预创建后切换策略
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)
for _, topic := range topics {
    bus.ConfigureTopic(ctx, topic, options)
}
bus.SetTopicConfigStrategy(eventbus.StrategySkip)
for i := 0; i < 10000; i++ {
    bus.Publish(ctx, topic, message) // ✅ 零 RPC 开销
}
```

---

## 📝 检查清单

### 应用启动时

- [ ] 定义所有需要使用的 Topic 列表
- [ ] 设置 `StrategyCreateOnly` 策略
- [ ] 调用 `ConfigureTopic()` 预创建所有 Stream
- [ ] 验证预创建是否成功
- [ ] 切换到 `StrategySkip` 策略

### 运行时

- [ ] 确认策略为 `StrategySkip`
- [ ] 直接调用 `Publish()`，不做额外检查
- [ ] 监控发布性能和错误率

### 新增 Topic 时

- [ ] 方式1: 重启应用，在启动时预创建
- [ ] 方式2: 临时切换到 `StrategyCreateOnly`，调用 `ConfigureTopic()`，再切回 `StrategySkip`

---

## 🔍 故障排查

### 问题1: 发布失败

**症状**: `Publish()` 返回错误

**可能原因**:
- 使用 `StrategySkip` 但未预创建 Stream
- Stream 配置不正确

**解决方案**:
```go
// 临时切换到 StrategyCreateOnly，重新创建
bus.SetTopicConfigStrategy(eventbus.StrategyCreateOnly)
bus.ConfigureTopic(ctx, topic, options)
bus.SetTopicConfigStrategy(eventbus.StrategySkip)
```

### 问题2: 性能未提升

**症状**: 吞吐量仍然很低

**可能原因**:
- 未切换到 `StrategySkip` 策略
- 未预创建 Stream

**解决方案**:
```go
// 检查当前策略
currentStrategy := bus.GetTopicConfigStrategy()
if currentStrategy != eventbus.StrategySkip {
    log.Warn("Not using StrategySkip, performance may be degraded")
}

// 检查缓存
natsBus := bus.(*eventbus.natsEventBus)
natsBus.createdStreamsMu.RLock()
streamCount := len(natsBus.createdStreams)
natsBus.createdStreamsMu.RUnlock()
if streamCount == 0 {
    log.Warn("No streams cached, pre-creation may have failed")
}
```

### 问题3: 缓存未生效

**症状**: 仍然有 RPC 调用

**可能原因**:
- 策略不是 `StrategySkip`
- Stream 名称不匹配

**解决方案**:
```go
// 确保策略正确
bus.SetTopicConfigStrategy(eventbus.StrategySkip)

// 检查 Stream 名称
streamName := bus.getStreamNameForTopic(topic)
log.Printf("Stream name for topic %s: %s", topic, streamName)
```

---

## 📚 相关资源

### 文档

- [详细文档](./STREAM_PRE_CREATION_OPTIMIZATION.md)
- [使用指南](../sdk/pkg/eventbus/examples/README_STREAM_PRECREATION.md)
- [实施总结](./STREAM_PRE_CREATION_IMPLEMENTATION_SUMMARY.md)

### 代码

- [核心实现](../sdk/pkg/eventbus/nats.go)
- [性能测试](../sdk/pkg/eventbus/nats_stream_precreation_test.go)
- [使用示例](../sdk/pkg/eventbus/examples/nats_stream_precreation_example.go)

### 测试

```bash
# 运行所有测试
go test -v -run TestNATSStreamPreCreation ./sdk/pkg/eventbus

# 运行演示脚本
cd sdk/pkg/eventbus/examples
./run_stream_precreation_demo.sh  # Linux/Mac
run_stream_precreation_demo.bat   # Windows
```

---

## 💡 最佳实践

### 1. 生产环境配置

```go
// config.yaml
eventbus:
  nats:
    precreation:
      enabled: true
      strategy: create_only
      topics:
        - business.orders.created
        - business.payments.completed
        - business.users.registered
```

### 2. 监控和告警

```go
// 监控预创建状态
func MonitorStreamPreCreation(bus eventbus.EventBus) {
    natsBus := bus.(*eventbus.natsEventBus)
    natsBus.createdStreamsMu.RLock()
    streamCount := len(natsBus.createdStreams)
    natsBus.createdStreamsMu.RUnlock()
    
    if streamCount == 0 {
        alert.Send("No streams cached, performance may be degraded")
    }
}
```

### 3. 优雅降级

```go
// 如果预创建失败，降级到 StrategyCreateOrUpdate
err := PreCreateAllStreams(bus, topics)
if err != nil {
    log.Warn("Pre-creation failed, falling back to CreateOrUpdate")
    bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)
} else {
    bus.SetTopicConfigStrategy(eventbus.StrategySkip)
}
```

---

## 🎯 核心要点

1. **预创建**: 应用启动时创建所有 Stream
2. **策略切换**: 预创建用 `StrategyCreateOnly`，运行时用 `StrategySkip`
3. **本地缓存**: 自动缓存已创建的 Stream，避免重复检查
4. **性能提升**: 595倍吞吐量提升，零 RPC 开销
5. **业界实践**: 类似 Kafka PreSubscription，MasterCard/Form3/Ericsson 推荐

---

**记住**: 预创建 → 切换策略 → 零开销发布 = **595倍性能提升** 🚀

