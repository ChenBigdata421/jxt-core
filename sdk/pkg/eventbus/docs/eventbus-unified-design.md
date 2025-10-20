# jxt-core EventBus 统一设计方案

## 📋 概述

本文档描述了 jxt-core EventBus 的统一设计方案，将原来的 EventBus 和 AdvancedEventBus 合并为一个统一的 EventBus 接口，通过配置来灵活启用企业特性。

## 🎯 设计目标

### 1. 简化接口设计
- **统一接口**：只有一个 EventBus 接口，减少学习成本
- **渐进式增强**：从基础功能开始，按需启用高级特性
- **配置驱动**：通过配置文件控制功能启用，而非代码硬编码

### 2. 提高易用性
- **向后兼容**：现有代码可以平滑迁移
- **灵活配置**：业务微服务根据实际需要选择功能
- **开箱即用**：默认配置提供基础功能，高级功能可选

### 3. 降低维护成本
- **单一接口**：减少接口维护复杂度
- **统一实现**：避免代码重复和不一致
- **清晰职责**：基础功能和企业特性职责分明

## 🏗️ 架构设计

### 统一 EventBus 接口

```go
type EventBus interface {
    // ========== 基础功能 ==========
    Publish(ctx context.Context, topic string, message []byte) error
    Subscribe(ctx context.Context, topic string, handler MessageHandler) error
    HealthCheck(ctx context.Context) error
    Close() error
    RegisterReconnectCallback(callback ReconnectCallback) error

    // ========== 生命周期管理 ==========
    Start(ctx context.Context) error
    Stop() error

    // ========== 高级发布功能（可选启用） ==========
    PublishWithOptions(ctx context.Context, topic string, message []byte, opts PublishOptions) error
    SetMessageFormatter(formatter MessageFormatter) error
    RegisterPublishCallback(callback PublishCallback) error

    // ========== 高级订阅功能（可选启用） ==========
    SubscribeWithOptions(ctx context.Context, topic string, handler MessageHandler, opts SubscribeOptions) error
    SetRecoveryMode(enabled bool) error
    IsInRecoveryMode() bool
    RegisterBacklogCallback(callback BacklogStateCallback) error
    StartBacklogMonitoring(ctx context.Context) error
    StopBacklogMonitoring() error
    SetMessageRouter(router MessageRouter) error
    SetErrorHandler(handler ErrorHandler) error
    RegisterSubscriptionCallback(callback SubscriptionCallback) error

    // ========== 统一健康检查和监控 ==========
    StartHealthCheck(ctx context.Context) error
    StopHealthCheck() error
    GetHealthStatus() HealthCheckStatus
    RegisterHealthCheckCallback(callback HealthCheckCallback) error
    GetConnectionState() ConnectionState
    GetMetrics() Metrics
}
```

### 配置驱动的企业特性

```yaml
# 企业特性配置
enterprise:
  # 发布端企业特性
  publisher:
    messageFormatter:
      enabled: true
      type: "json"
    publishCallback:
      enabled: true
    retryPolicy:
      enabled: true
      maxRetries: 3

  # 订阅端企业特性
  subscriber:
    backlogDetection:
      enabled: true
      checkInterval: 30s
    aggregateProcessor:
      enabled: true
      maxWorkers: 10
    rateLimit:
      enabled: true
      rateLimit: 1000.0
    recoveryMode:
      enabled: true
      autoEnable: true
    deadLetter:
      enabled: true
      topic: "dead-letter-queue"

  # 统一企业特性
  healthCheck:
    enabled: true
    interval: 30s
  monitoring:
    enabled: true
    metricsInterval: 60s
```

## 🔧 实现方案

### 1. 接口合并
- 将 AdvancedEventBus 的所有方法合并到 EventBus 接口
- 保持方法签名不变，确保向后兼容
- 添加 `AdvancedEventBus = EventBus` 类型别名用于过渡

### 2. 配置结构
- 新增 `EnterpriseConfig` 配置结构
- 分为发布端、订阅端、统一三类企业特性
- 每个特性都有独立的启用/禁用开关

### 3. 实现策略
- 基础实现提供所有方法的默认实现
- 高级实现根据配置启用相应功能
- 通过组合模式复用基础功能

## 📊 使用场景

### 场景 1：基础使用
```go
// 最简配置
config := &EventBusConfig{
    Type: "kafka",
    Kafka: KafkaConfig{
        Brokers: []string{"localhost:9092"},
    },
    // 企业特性默认全部禁用
}

bus, _ := NewEventBus(config)
bus.Publish(ctx, "topic", message)
bus.Subscribe(ctx, "topic", handler)
```

### 场景 2：高性能场景
```yaml
enterprise:
  subscriber:
    aggregateProcessor:
      enabled: true
      maxWorkers: 20
    rateLimit:
      enabled: true
      rateLimit: 5000.0
    backlogDetection:
      enabled: true
```

### 场景 3：高可靠性场景
```yaml
enterprise:
  publisher:
    retryPolicy:
      enabled: true
      maxRetries: 5
  subscriber:
    deadLetter:
      enabled: true
      topic: "dlq"
    errorHandler:
      enabled: true
      type: "deadletter"
  healthCheck:
    enabled: true
```

## 🚀 迁移指南

### 从 EventBus 迁移
```go
// 原代码
bus, _ := NewEventBus(config)

// 新代码（无需修改）
bus, _ := NewEventBus(config)
// 所有原有方法继续可用
```

### 从 AdvancedEventBus 迁移
```go
// 原代码
advancedBus, _ := NewAdvancedEventBus(config)

// 新代码
config.Enterprise = EnterpriseConfig{
    // 启用需要的企业特性
}
bus, _ := NewEventBus(config)
// 所有高级方法继续可用
```

## 📈 优势总结

### 1. 开发体验
- **学习成本降低**：只需学习一个接口
- **配置简单**：通过 YAML 配置控制功能
- **渐进式采用**：可以从基础功能开始，逐步启用高级特性

### 2. 运维友好
- **配置驱动**：无需修改代码即可调整功能
- **监控统一**：所有实现提供统一的监控接口
- **故障排查**：统一的健康检查和连接状态

### 3. 架构优势
- **接口简化**：减少接口数量和复杂度
- **实现统一**：避免代码重复和不一致
- **扩展性好**：新增企业特性只需扩展配置

## 🔮 未来规划

### 短期目标
- 完善所有企业特性的配置化实现
- 提供更多使用场景的配置模板
- 完善文档和示例代码

### 长期目标
- 支持运行时动态配置调整
- 提供可视化配置管理界面
- 集成更多企业级特性（如分布式追踪、审计日志等）

## 📚 相关文档

- [业务微服务使用指南](./business-microservice-usage-guide.md)
- [配置参考](../sdk/pkg/eventbus/example_unified_config.yaml)
- [使用示例](../sdk/pkg/eventbus/example_unified_usage.go)
- [API 文档](../sdk/pkg/eventbus/type.go)
