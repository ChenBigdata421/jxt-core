# JXT-Core EventBus 企业级事件总线

## 概述

JXT-Core EventBus 是一个高性能、企业级的事件总线系统，支持多种消息中间件后端，提供统一的API接口和丰富的企业级特性。

## 核心特性

### 🚀 高性能
- **内存模式**: 19,000+ 消息/秒的处理能力
- **低延迟**: 平均延迟 < 100 微秒
- **并发安全**: 支持多协程并发发送和接收

### 🔧 多后端支持
- **Memory**: 内存模式，适用于单机高性能场景
- **Kafka**: 分布式消息队列，适用于大规模集群
- **NATS JetStream**: 云原生消息系统，适用于微服务架构

### 🛡️ 企业级特性
- **健康检查**: 实时监控系统状态
- **重连机制**: 自动重连和故障恢复
- **流量控制**: 自适应限流和背压处理
- **错误处理**: 死信队列和重试机制
- **监控指标**: 详细的性能和业务指标

### 🎯 易用性
- **统一API**: 一套API支持所有后端
- **配置驱动**: 通过配置文件切换后端
- **全局管理**: 支持全局单例模式
- **类型安全**: 完整的Go类型定义

## 快速开始

### 基本使用

```go
package main

import (
    "context"
    "fmt"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 创建事件总线
    config := &eventbus.EventBusConfig{
        Type: "memory", // 或 "kafka", "nats"
    }
    
    bus, err := eventbus.NewEventBus(config)
    if err != nil {
        panic(err)
    }
    defer bus.Close()
    
    // 订阅消息
    err = bus.Subscribe(context.Background(), "user_events", func(ctx context.Context, message []byte) error {
        fmt.Printf("收到消息: %s\n", string(message))
        return nil
    })
    if err != nil {
        panic(err)
    }
    
    // 发布消息
    err = bus.Publish(context.Background(), "user_events", []byte(`{"event": "user_created", "user_id": "123"}`))
    if err != nil {
        panic(err)
    }
}
```

### 使用全局实例

```go
package main

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 初始化全局事件总线
    config := &eventbus.EventBusConfig{Type: "memory"}
    err := eventbus.InitGlobal(config)
    if err != nil {
        panic(err)
    }
    defer eventbus.CloseGlobal()
    
    // 使用全局实例
    bus := eventbus.GetGlobal()
    
    // 订阅和发布...
}
```

### 配置文件驱动

```go
// 从配置文件初始化
err := eventbus.InitFromConfig()
if err != nil {
    panic(err)
}
```

## 配置说明

### Memory 配置
```yaml
eventbus:
  type: memory
```

### Kafka 配置
```yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]
    healthCheckInterval: 30s
    producer:
      requiredAcks: 1
      timeout: 10s
      compression: "snappy"
    consumer:
      groupId: "jxt-consumer-group"
      sessionTimeout: 30s
      heartbeatInterval: 3s
```

### NATS 配置
```yaml
eventbus:
  type: nats
  nats:
    urls: ["nats://localhost:4222"]
    clientId: "jxt-client"
    maxReconnects: 10
    jetstream:
      enabled: true
      stream:
        name: "JXT_STREAM"
        subjects: ["jxt.>"]
```

## 性能测试结果

基于内存模式的性能测试结果：

- **测试场景**: 10,000 条消息，10 个并发协程
- **发送速率**: 19,798 消息/秒
- **处理速率**: 19,402 消息/秒
- **平均延迟**: 51.54 微秒
- **总耗时**: 515 毫秒

## 企业级特性详解

### 健康检查
```go
// 检查系统健康状态
err := bus.HealthCheck(context.Background())
if err != nil {
    log.Printf("系统不健康: %v", err)
}
```

### 重连回调
```go
// 注册重连回调
bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Println("连接已恢复，重新初始化...")
    return nil
})
```

### 流量控制
系统内置自适应流量控制，根据系统负载自动调整处理速率。

### 错误处理
支持死信队列和自动重试机制，确保消息不丢失。

## 最佳实践

1. **生产环境推荐使用 Kafka 或 NATS**
2. **合理设置缓冲区大小**
3. **监控系统健康状态**
4. **使用结构化日志**
5. **定期检查性能指标**

## 示例代码

完整的示例代码请参考：
- `examples/eventbus_example.go` - 基本使用示例
- `examples/performance_demo.go` - 性能测试示例

## 测试

运行所有测试：
```bash
go test ./sdk/pkg/eventbus/... -v
```

运行性能测试：
```bash
go run examples/performance_demo.go
```

## 贡献

欢迎提交 Issue 和 Pull Request 来改进这个项目。

## 许可证

本项目采用 MIT 许可证。
