# jxt-core EventBus 分离式健康检查实现

## 🎯 问题背景

用户提出了一个重要的架构设计问题：

> "jxt-core项目eventbus组件，对于健康检查的启动，是否也应该是发布端和订阅端分别启动，就像积压检测一样分开启动。应为同一个微服务可能是A业务的发布端，但却是B业务的订阅端。"

这个问题指出了原有健康检查设计的局限性：
- 原有设计采用统一启动方式，无法根据微服务的实际角色灵活配置
- 一个微服务在不同业务场景中可能扮演不同角色（发布端/订阅端）
- 缺乏像积压检测那样的精细化控制能力

## ✅ 解决方案

### 1. 新的分离式接口设计

参考积压检测的成功设计模式，我们重新设计了健康检查接口：

```go
// ========== 健康检查功能（分离发布端和订阅端） ==========
// 发布端健康检查（发送健康检查消息）
StartHealthCheckPublisher(ctx context.Context) error
StopHealthCheckPublisher() error
GetHealthCheckPublisherStatus() HealthCheckStatus
RegisterHealthCheckPublisherCallback(callback HealthCheckCallback) error

// 订阅端健康检查（监控健康检查消息）
StartHealthCheckSubscriber(ctx context.Context) error
StopHealthCheckSubscriber() error
GetHealthCheckSubscriberStats() HealthCheckSubscriberStats
RegisterHealthCheckSubscriberCallback(callback HealthCheckAlertCallback) error

// 根据配置启动所有健康检查（发布端和/或订阅端）
StartAllHealthCheck(ctx context.Context) error
StopAllHealthCheck() error

// 向后兼容的统一接口（已废弃，建议使用分离的接口）
StartHealthCheck(ctx context.Context) error // 已废弃，使用StartHealthCheckPublisher
StopHealthCheck() error                     // 已废弃，使用StopHealthCheckPublisher
GetHealthStatus() HealthCheckStatus         // 已废弃，使用GetHealthCheckPublisherStatus
```

### 2. 实现架构

#### 核心组件分离
- **HealthChecker**: 负责发布健康检查消息（发布端）
- **HealthCheckSubscriber**: 负责监控健康检查消息（订阅端）
- **EventBus**: 提供统一的分离式接口

#### 配置结构优化
```go
// 发布端和订阅端使用独立的主题配置
type HealthCheckSenderConfig struct {
    Topic            string        `mapstructure:"topic"`            // 发布端主题
    Interval         time.Duration `mapstructure:"interval"`         // 发送间隔
    // ... 其他配置
}

type HealthCheckSubscriberConfig struct {
    Topic             string        `mapstructure:"topic"`             // 订阅端主题
    MonitorInterval   time.Duration `mapstructure:"monitorInterval"`   // 监控间隔
    // ... 其他配置
}
```

### 3. 使用场景

#### 场景1：纯发布端服务
```go
// 例如：用户服务只发布自己的健康状态，不监控其他服务
bus.StartHealthCheckPublisher(ctx)
```

#### 场景2：纯订阅端服务
```go
// 例如：监控服务只监控其他服务，不发布自己的健康状态
bus.StartHealthCheckSubscriber(ctx)
```

#### 场景3：混合角色服务
```go
// 例如：订单服务既发布自己的健康状态，又监控用户服务
bus.StartHealthCheckPublisher(ctx)   // 发布自己的健康状态
bus.StartHealthCheckSubscriber(ctx)  // 监控用户服务
// 或者使用便捷方法：
bus.StartAllHealthCheck(ctx)
```

#### 场景4：多重监控
```go
// 监控服务可以创建多个订阅器监控不同服务
userConfig := eventbus.GetDefaultHealthCheckConfig()
userConfig.Subscriber.Topic = "health-check-user-service"
userSubscriber := eventbus.NewHealthCheckSubscriber(userConfig, bus, "monitor", "kafka")

orderConfig := eventbus.GetDefaultHealthCheckConfig()
orderConfig.Subscriber.Topic = "health-check-order-service"
orderSubscriber := eventbus.NewHealthCheckSubscriber(orderConfig, bus, "monitor", "kafka")
```

## 🚀 关键优势

### 1. 角色精确匹配
- 微服务可以根据实际业务角色选择启动发布端或订阅端
- 避免不必要的资源消耗和网络流量
- 支持复杂的微服务监控拓扑

### 2. 配置灵活性
- 发布端和订阅端可以使用不同的主题进行精确配对
- 不同服务可以有不同的健康检查间隔和告警阈值
- 支持跨服务监控和自监控

### 3. 资源优化
- 只启动需要的组件，减少内存和CPU使用
- 避免无意义的消息订阅和处理
- 提高系统整体性能

### 4. 向后兼容
- 保留原有的统一接口，标记为废弃但仍可使用
- 平滑迁移路径，不破坏现有代码
- 提供清晰的迁移指导

### 5. 与积压检测一致
- 采用与积压检测相同的设计模式
- 保持API设计的一致性和可预测性
- 降低学习成本

## 📊 实现效果

通过运行演示程序，我们验证了新设计的有效性：

1. **场景1（纯发布端）**: 成功启动健康检查发布器，发布健康检查消息
2. **场景2（纯订阅端）**: 成功启动健康检查订阅器，监控健康检查消息
3. **场景3（混合角色）**: 成功同时启动发布器和订阅器，实现端到端健康检查
4. **场景4（向后兼容）**: 旧接口仍然工作，但显示废弃警告

## 🔧 迁移指南

### 从旧接口迁移到新接口

```go
// 旧方式（已废弃）
bus.StartHealthCheck(ctx)
status := bus.GetHealthStatus()
bus.StopHealthCheck()

// 新方式（推荐）
bus.StartHealthCheckPublisher(ctx)
status := bus.GetHealthCheckPublisherStatus()
bus.StopHealthCheckPublisher()
```

### 配置文件更新

```yaml
# 旧配置
healthCheck:
  topic: "health-check"  # 统一主题
  
# 新配置
healthCheck:
  sender:
    topic: "health-check-user-service"    # 发布端主题
  subscriber:
    topic: "health-check-order-service"   # 订阅端主题（可以不同）
```

## 🎉 总结

这次实现成功解决了用户提出的架构问题：

1. **实现了发布端和订阅端的分离启动**，与积压检测保持一致
2. **支持微服务在不同业务中扮演不同角色**的需求
3. **提供了更精确的资源控制和监控能力**
4. **保持了向后兼容性**，确保平滑迁移

新的分离式健康检查设计为jxt-core EventBus组件提供了更强大、更灵活的健康监控能力，能够更好地适应复杂的微服务架构需求。
