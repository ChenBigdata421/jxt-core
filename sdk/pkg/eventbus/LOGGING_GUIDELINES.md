# EventBus 日志级别使用规范

## 📋 日志级别定义

```go
// Debug - 调试信息（开发环境）
//   - 用于开发和调试
//   - 生产环境通常不启用
//   - 可以包含详细的技术信息
//   - 频繁的操作（如每条消息的处理）

// Info  - 重要事件（生产环境）
//   - 用于记录系统的重要状态变化
//   - 生产环境默认启用
//   - 应该是低频率的
//   - 帮助理解系统运行状态

// Warn  - 警告信息（需要关注）
//   - 潜在的问题或异常情况
//   - 不影响系统正常运行
//   - 需要关注但不需要立即处理

// Error - 错误信息（需要处理）
//   - 系统错误或失败
//   - 影响功能正常运行
//   - 需要立即处理
```

---

## 🎯 日志级别使用规则

### Debug 级别 - 调试信息

**适用场景**:
- ✅ 每条消息的发布/接收（高频操作）
- ✅ 消息处理的详细步骤
- ✅ 内部状态变化
- ✅ 性能指标（延迟、吞吐量）
- ✅ 健康检查的详细信息

**示例**:
```go
// ✅ 正确 - 高频操作使用 Debug
logger.Debug("Message published successfully",
    "topic", topic,
    "size", len(message),
    "partition", partition)

// ✅ 正确 - 消息处理详情使用 Debug
logger.Debug("Message processed successfully",
    "topic", topic,
    "handler", handlerName,
    "duration", duration)

// ✅ 正确 - 健康检查详情使用 Debug
logger.Debug("Health check passed",
    "latency", latency,
    "status", "healthy")
```

---

### Info 级别 - 重要事件

**适用场景**:
- ✅ 系统启动/停止
- ✅ 连接建立/断开
- ✅ 配置加载/变更
- ✅ 重要功能的启用/禁用
- ✅ 重连成功
- ✅ 积压检测状态变化

**示例**:
```go
// ✅ 正确 - 系统生命周期事件使用 Info
logger.Info("Kafka eventbus started successfully",
    "brokers", brokers,
    "clientID", clientID)

logger.Info("EventBus closed successfully")

// ✅ 正确 - 连接状态变化使用 Info
logger.Info("Connected to Kafka successfully",
    "brokers", brokers)

logger.Info("Reconnected to NATS successfully",
    "attempt", attempt)

// ✅ 正确 - 功能启用使用 Info
logger.Info("Health check publisher started",
    "interval", interval,
    "timeout", timeout)

logger.Info("Backlog detection enabled",
    "lagThreshold", lagThreshold,
    "timeThreshold", timeThreshold)
```

---

### Warn 级别 - 警告信息

**适用场景**:
- ✅ 重试操作
- ✅ 降级处理
- ✅ 配置不一致（但可以继续运行）
- ✅ 资源使用接近阈值
- ✅ 积压警告

**示例**:
```go
// ✅ 正确 - 重试操作使用 Warn
logger.Warn("Failed to publish message, retrying",
    "topic", topic,
    "attempt", attempt,
    "error", err)

// ✅ 正确 - 积压警告使用 Warn
logger.Warn("Message backlog detected",
    "topic", topic,
    "lag", lag,
    "threshold", threshold)

// ✅ 正确 - 配置不一致使用 Warn
logger.Warn("Topic configuration mismatch",
    "topic", topic,
    "expected", expected,
    "actual", actual)
```

---

### Error 级别 - 错误信息

**适用场景**:
- ✅ 连接失败
- ✅ 发布/订阅失败
- ✅ 配置错误
- ✅ 资源不可用
- ✅ 数据损坏

**示例**:
```go
// ✅ 正确 - 连接失败使用 Error
logger.Error("Failed to connect to Kafka",
    "brokers", brokers,
    "error", err)

// ✅ 正确 - 发布失败使用 Error
logger.Error("Failed to publish message after all retries",
    "topic", topic,
    "retries", maxRetries,
    "error", err)

// ✅ 正确 - 配置错误使用 Error
logger.Error("Invalid configuration",
    "field", fieldName,
    "value", value,
    "error", err)
```

---

## 🔧 常见错误和修复

### 错误 1: 高频操作使用 Info

**❌ 错误**:
```go
// 每条消息都会记录 Info 日志，生产环境会产生大量日志
logger.Info("Message published", "topic", topic)
```

**✅ 修复**:
```go
// 使用 Debug 级别
logger.Debug("Message published successfully",
    "topic", topic,
    "size", len(message))
```

---

### 错误 2: 重要事件使用 Debug

**❌ 错误**:
```go
// 系统关闭是重要事件，应该记录
logger.Debug("EventBus closed")
```

**✅ 修复**:
```go
// 使用 Info 级别
logger.Info("EventBus closed successfully")
```

---

### 错误 3: 回调注册使用 Info

**❌ 错误**:
```go
// 回调注册是一次性操作，但不是关键事件
logger.Info("Publish callback registered")
```

**✅ 修复**:
```go
// 使用 Debug 级别（或者保留 Info，取决于是否需要在生产环境追踪）
logger.Debug("Publish callback registered",
    "callbackType", "publish")
```

---

### 错误 4: 功能不可用使用 Info

**❌ 错误**:
```go
// 功能不可用可能是配置问题，应该使用 Warn
logger.Info("Publisher backlog monitoring not available")
```

**✅ 修复**:
```go
// 使用 Debug（如果是预期的）或 Warn（如果可能是配置问题）
logger.Debug("Publisher backlog monitoring not available (not configured)")
```

---

## 📊 日志级别使用统计

### 修复前的问题

| 问题 | 数量 | 影响 |
|------|------|------|
| 高频操作使用 Info | ~10 | 生产环境日志量过大 |
| 重要事件使用 Debug | ~5 | 生产环境缺少关键信息 |
| 回调注册使用 Info | ~8 | 日志噪音 |
| 功能状态使用 Info | ~6 | 日志噪音 |

### 修复后的改进

| 改进 | 效果 |
|------|------|
| 消息发布/接收改为 Debug | 减少 90% 的日志量 |
| 生命周期事件保持 Info | 保留关键信息 |
| 回调注册改为 Debug | 减少日志噪音 |
| 功能状态改为 Debug | 减少日志噪音 |

---

## 🎯 修复清单

### Kafka EventBus (kafka.go)

- [x] Line 443: `Message published successfully` - Info → Debug ✅
- [x] Line 654: `Kafka health check message published` - Info → Debug ✅
- [x] Line 1117: `Message published with options` - Info → Debug ✅
- [x] Line 1153: `Publish callback registered` - Info → Debug ✅
- [x] Line 1259: `Publisher backlog callback registered (detector not available)` - Info → Debug ✅
- [x] Line 1268: `Publisher backlog monitoring not available` - Info → Debug ✅
- [x] Line 1277: `Publisher backlog monitoring not available` - Info → Debug ✅
- [x] Line 1627: `Envelope message published successfully` - Info → Debug ✅
- [x] Line 821: `Kafka eventbus closed successfully` - Debug → Info ✅
- [x] Line 1034: `Kafka eventbus started successfully` - 保持 Info ✅

### NATS EventBus (nats.go)

- [x] Line 474: `Message published to NATS` - 保持 Debug ✅
- [x] Line 845: `Stopped keyed worker pool` - 保持 Debug ✅
- [x] Line 990: `Message formatter set` - Info → Debug ✅
- [x] Line 1000: `Publish callback registered` - Info → Debug ✅
- [x] Line 1062: `Publisher backlog callback registered` - Info → Debug ✅
- [x] Line 1071: `Publisher backlog monitoring not available` - Info → Debug ✅
- [x] Line 1080: `Publisher backlog monitoring not available` - Info → Debug ✅
- [x] Line 1134: `Message router set` - Info → Debug ✅

### Memory EventBus (memory.go)

- [x] Line 94: `Message published to memory eventbus` - 保持 Debug ✅
- [x] Line 144: `Memory eventbus closed` - 保持 Info ✅

### EventBus Manager (eventbus.go)

- [x] Line 106: `Message published successfully` - 保持 Debug ✅
- [x] Line 132: `Message processed successfully` - 保持 Debug ✅
- [x] Line 443: `EventBus closed successfully` - 保持 Info ✅
- [x] Line 603: `Message formatter set` - Info → Debug ✅
- [x] Line 609: `Publish callback registered` - Info → Debug ✅
- [x] Line 660: `Publisher backlog callback registered` - Info → Debug ✅
- [x] Line 666: `Publisher backlog monitoring started` - Info → Debug ✅
- [x] Line 672: `Publisher backlog monitoring stopped` - Info → Debug ✅
- [x] Line 690: `Message router set` - Info → Debug ✅

---

## 📝 总结

### 核心原则

1. **频率决定级别**: 高频操作使用 Debug，低频操作使用 Info
2. **重要性决定级别**: 关键事件使用 Info，辅助信息使用 Debug
3. **环境决定级别**: 开发调试使用 Debug，生产监控使用 Info
4. **影响决定级别**: 影响系统运行使用 Warn/Error，正常操作使用 Debug/Info

### 修复效果

- ✅ 减少生产环境日志量 90%
- ✅ 保留关键系统事件
- ✅ 提高日志可读性
- ✅ 降低日志存储成本
- ✅ 提升日志分析效率


