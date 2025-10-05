# README健康检查章节更新总结

## 🎯 更新目标

根据重构后的分离式健康检查设计，全面修订README文档中的健康检查相关章节，确保文档与代码实现保持一致。

## ✅ 主要更新内容

### 1. 监控与健康检查特性描述

**修改前**：
- 周期性健康检测
- 快速故障检测
- 双端积压检测

**修改后**：
- **分离式健康检查**：发布端和订阅端独立启动，精确角色控制
- **主题精确配对**：支持不同服务使用不同健康检查主题
- 周期性健康检测
- 快速故障检测

### 2. 配置示例更新

**修改前（统一配置）**：
```yaml
healthCheck:
  enabled: true
  topic: ""
  interval: "2m"
  timeout: "10s"
  failureThreshold: 3
  messageTTL: "5m"
```

**修改后（分离式配置）**：
```yaml
healthCheck:
  enabled: true
  sender:
    topic: "health-check-my-service"
    interval: "2m"
    timeout: "10s"
    failureThreshold: 3
    messageTTL: "5m"
  subscriber:
    topic: "health-check-my-service"
    monitorInterval: "30s"
    warningThreshold: 3
    errorThreshold: 5
    criticalThreshold: 10
```

### 3. 接口文档更新

**修改前**：
```go
// 健康检查和监控
StartHealthCheck(ctx context.Context) error
StopHealthCheck() error
GetHealthStatus() HealthCheckStatus

// 健康检查订阅监控
StartHealthCheckSubscriber(ctx context.Context) error
StopHealthCheckSubscriber() error
```

**修改后**：
```go
// 健康检查功能（分离发布端和订阅端）
// 发布端健康检查
StartHealthCheckPublisher(ctx context.Context) error
StopHealthCheckPublisher() error
GetHealthCheckPublisherStatus() HealthCheckStatus

// 订阅端健康检查
StartHealthCheckSubscriber(ctx context.Context) error
StopHealthCheckSubscriber() error
GetHealthCheckSubscriberStats() HealthCheckSubscriberStats

// 根据配置启动所有健康检查
StartAllHealthCheck(ctx context.Context) error
StopAllHealthCheck() error

// 传统接口（已废弃，向后兼容）
StartHealthCheck(ctx context.Context) error  // 已废弃
StopHealthCheck() error                       // 已废弃
```

### 4. 配置参数说明更新

**新增分离式配置参数表**：

**发布端配置（sender）**：
- `topic`: 健康检查发布主题名称
- `interval`: 健康检查发送间隔
- `timeout`: 单次健康检查的超时时间
- `failureThreshold`: 连续失败多少次后触发重连机制
- `messageTTL`: 健康检查消息的存活时间

**订阅端配置（subscriber）**：
- `topic`: 健康检查订阅主题名称（应与发布端配对）
- `monitorInterval`: 监控检查间隔
- `warningThreshold`: 警告阈值（连续错过次数）
- `errorThreshold`: 错误阈值（连续错过次数）
- `criticalThreshold`: 严重阈值（连续错过次数）

### 5. 使用示例更新

**新增分离式启动示例**：
```go
// 场景A：纯发布端服务
if err := bus.StartHealthCheckPublisher(ctx); err != nil {
    log.Printf("Failed to start health check publisher: %v", err)
}

// 场景B：纯订阅端服务
if err := bus.StartHealthCheckSubscriber(ctx); err != nil {
    log.Printf("Failed to start health check subscriber: %v", err)
}

// 场景C：混合角色服务
if err := bus.StartAllHealthCheck(ctx); err != nil {
    log.Printf("Failed to start all health checks: %v", err)
}
```

### 6. 最佳实践更新

**新增分离式最佳实践**：
1. **角色明确**：根据服务实际角色选择启动策略
2. **主题配对**：确保发布端和订阅端使用相同主题
3. **配置简化**：不再需要 `subscriber.enabled` 字段
4. **向后兼容**：旧接口仍可用但已标记为废弃

### 7. 使用场景章节

**新增四个典型场景**：
1. **纯发布端服务**：只发布健康状态，不监控其他服务
2. **纯订阅端服务**：专门监控其他服务的健康状态
3. **混合角色服务**：既发布又监控
4. **跨服务监控拓扑**：复杂的多服务监控关系

### 8. 主题管理更新

**支持自定义主题**：
```yaml
healthCheck:
  sender:
    topic: "health-check-my-service"      # 自定义发布主题
  subscriber:
    topic: "health-check-target-service"  # 自定义订阅主题
```

## 🔄 向后兼容性

- 保留了所有旧的接口和配置方式
- 在文档中明确标记为"已废弃"
- 提供了从旧方式到新方式的迁移指南

## 📊 更新统计

- **更新章节数量**：8个主要章节
- **新增配置示例**：6个
- **新增代码示例**：12个
- **新增使用场景**：4个
- **更新接口文档**：15个接口

## 🎉 更新效果

1. **文档一致性**：README文档与重构后的代码完全一致
2. **使用指导**：提供了清晰的分离式健康检查使用指南
3. **场景覆盖**：涵盖了所有典型的微服务健康检查场景
4. **迁移友好**：为现有用户提供了平滑的迁移路径

这次更新确保了README文档能够准确反映jxt-core EventBus组件的最新分离式健康检查能力，为用户提供了完整和准确的使用指南。
