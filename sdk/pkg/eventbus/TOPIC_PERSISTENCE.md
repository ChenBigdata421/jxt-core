# EventBus 主题持久化管理

## 概述

jxt-core EventBus 现在支持按主题设置持久化策略，允许在同一个 EventBus 实例中同时处理持久化和非持久化的主题。这种设计提供了更大的灵活性和更好的资源利用率。

## 主要特性

### 1. 主题级持久化控制
- **动态配置**：运行时可以为不同主题设置不同的持久化策略
- **智能路由**：自动根据主题配置选择合适的发布模式（JetStream 或 Core NATS）
- **统一接口**：现有的 Publish/Subscribe API 保持不变

### 2. 持久化模式
```go
type TopicPersistenceMode string

const (
    TopicPersistent TopicPersistenceMode = "persistent"  // 强制持久化
    TopicEphemeral  TopicPersistenceMode = "ephemeral"   // 强制非持久化  
    TopicAuto       TopicPersistenceMode = "auto"        // 根据全局配置自动选择
)
```

### 3. 主题配置选项
```go
type TopicOptions struct {
    PersistenceMode TopicPersistenceMode // 持久化模式
    RetentionTime   time.Duration        // 消息保留时间
    MaxSize         int64                // 最大存储大小
    MaxMessages     int64                // 最大消息数量
    Replicas        int                  // 副本数量
    Description     string               // 主题描述
}
```

## API 接口

### 主题管理接口
```go
type EventBus interface {
    // 配置主题选项
    ConfigureTopic(ctx context.Context, topic string, options TopicOptions) error
    
    // 简化接口：设置主题持久化
    SetTopicPersistence(ctx context.Context, topic string, persistent bool) error
    
    // 获取主题配置
    GetTopicConfig(topic string) (TopicOptions, error)
    
    // 列出已配置的主题
    ListConfiguredTopics() []string
    
    // 移除主题配置
    RemoveTopicConfig(topic string) error
    
    // 现有接口保持不变
    Publish(ctx context.Context, topic string, message []byte) error
    Subscribe(ctx context.Context, topic string, handler MessageHandler) error
    // ...
}
```

## 使用示例

### 基本用法
```go
// 创建 EventBus
config := &eventbus.EventBusConfig{
    Type: "nats",
    NATS: eventbus.NATSConfig{
        URLs:     []string{"nats://localhost:4222"},
        ClientID: "my-service",
        JetStream: eventbus.JetStreamConfig{
            Enabled: true, // 启用 JetStream 支持
        },
    },
}

bus, err := eventbus.NewEventBus(config)
if err != nil {
    log.Fatal(err)
}
defer bus.Close()

// 配置不同主题的持久化策略
ctx := context.Background()

// 订单事件 - 需要持久化
orderOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicPersistent,
    RetentionTime:   7 * 24 * time.Hour, // 保留7天
    MaxSize:         100 * 1024 * 1024,  // 100MB
    MaxMessages:     10000,              // 1万条消息
    Description:     "订单相关事件，需要持久化存储",
}
bus.ConfigureTopic(ctx, "orders", orderOptions)

// 实时通知 - 不需要持久化
bus.SetTopicPersistence(ctx, "notifications", false)

// 系统监控 - 自动选择
metricsOptions := eventbus.TopicOptions{
    PersistenceMode: eventbus.TopicAuto,
    Description:     "系统监控指标，自动选择持久化策略",
}
bus.ConfigureTopic(ctx, "metrics", metricsOptions)
```

### 发布和订阅
```go
// 发布消息（自动根据配置选择模式）
orderData := `{"order_id": "order-123", "amount": 99.99, "status": "created"}`
err = bus.Publish(ctx, "orders", []byte(orderData))        // 使用 JetStream（持久化）

notificationData := `{"type": "info", "message": "欢迎使用系统"}`
err = bus.Publish(ctx, "notifications", []byte(notificationData)) // 使用 Core NATS（非持久化）

// 订阅消息
err = bus.Subscribe(ctx, "orders", func(ctx context.Context, message []byte) error {
    fmt.Printf("收到订单消息: %s\n", string(message))
    return nil
})
```

### 动态配置管理
```go
// 查看已配置的主题
topics := bus.ListConfiguredTopics()
fmt.Printf("已配置主题: %v\n", topics)

// 获取主题配置
config, err := bus.GetTopicConfig("orders")
if err == nil {
    fmt.Printf("订单主题配置: %+v\n", config)
}

// 动态修改主题配置
err = bus.SetTopicPersistence(ctx, "notifications", true) // 改为持久化

// 移除主题配置
err = bus.RemoveTopicConfig("metrics") // 将使用默认行为
```

## 技术实现

### NATS 实现
- **持久化主题**：使用 NATS JetStream 进行消息发布和订阅
- **非持久化主题**：使用 Core NATS 进行高性能消息传递
- **动态流管理**：根据主题配置自动创建和管理 JetStream 流

### Kafka 实现
- **主题管理**：使用 Kafka Admin API 动态创建和配置主题
- **持久化配置**：通过主题级别的配置参数控制持久化行为

### 内存实现
- **配置存储**：支持主题配置的存储和管理
- **接口兼容**：提供完整的接口实现（实际持久化由具体实现决定）

## 优势

1. **灵活性**：同一个 EventBus 实例可以处理不同持久化需求的主题
2. **资源优化**：只需要一个连接，减少资源消耗
3. **运维简化**：统一的连接管理、健康检查、监控
4. **动态配置**：运行时可以调整主题的持久化策略
5. **向前兼容**：现有的 Publish/Subscribe API 保持不变
6. **渐进式采用**：可以逐步为不同主题配置不同策略

## 最佳实践

1. **主题分类**：
   - 业务关键数据（订单、支付）→ 持久化
   - 实时通知、状态更新 → 非持久化
   - 监控指标、日志 → 根据需求选择

2. **配置管理**：
   - 在应用启动时配置主题策略
   - 使用描述字段记录配置原因
   - 定期审查和优化主题配置

3. **监控和告警**：
   - 监控持久化主题的存储使用情况
   - 设置消息积压告警
   - 跟踪主题配置变更

## 测试

运行主题持久化相关测试：
```bash
cd jxt-core/sdk/pkg/eventbus
go test -run TestTopicPersistence -v
```

运行示例程序：
```bash
go run examples/topic_persistence_example.go
```

## 注意事项

1. **配置持久性**：主题配置存储在内存中，重启后需要重新配置
2. **性能影响**：运行时主题配置查询会有轻微性能开销
3. **兼容性**：确保 NATS 服务器支持 JetStream（NATS 2.2+）
4. **资源管理**：持久化主题会消耗更多存储和内存资源
