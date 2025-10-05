# jxt-core EventBus 健康检查与自动重连功能总结

## 概述

本文档总结了 jxt-core EventBus 组件的健康检查和自动重连功能的完整实现，以及业务层的正确使用方式。

## 功能特性

### ✅ 已实现的核心功能

1. **周期性健康检查**
   - 支持 Kafka、NATS、Memory 三种实现
   - 可配置的检查间隔（默认 30 秒）
   - 同步启动/停止控制
   - 线程安全的生命周期管理

2. **智能自动重连**
   - 基于健康检查失败次数触发
   - 指数退避算法避免频繁重连
   - 自动恢复所有订阅状态
   - 支持 Kafka 和 NATS 双后端

3. **订阅状态恢复**
   - 自动保存订阅信息（topic + handler）
   - 重连后完全恢复订阅状态
   - 线程安全的状态管理
   - 区分不同消息中间件的订阅类型

4. **业务层回调机制**
   - 重连成功后的业务状态处理
   - 支持多个回调函数
   - 错误容忍和隔离
   - 清晰的执行时机保证

## 业务层使用指南

### 🎯 业务层需要做什么？

#### ✅ 需要处理的业务状态

1. **应用级缓存重新加载**
2. **业务状态与其他服务同步**
3. **监控指标和告警通知**
4. **审计日志记录**
5. **外部依赖状态通知**
6. **定时任务重启**

#### ❌ 无需处理的基础设施

1. **重新订阅消息**（EventBus 自动完成）
2. **重新建立连接**（EventBus 自动完成）
3. **恢复消息处理器**（EventBus 自动完成）
4. **健康检查恢复**（EventBus 自动完成）

### 🔧 正确的使用方式

#### 1. 基础设置

```go
// 1. 初始化 EventBus
cfg := eventbus.GetDefaultKafkaConfig([]string{"localhost:9092"})
eventbus.InitializeFromConfig(cfg)
bus := eventbus.GetGlobal()

// 2. 注册重连回调（处理业务状态）
bus.RegisterReconnectCallback(func(ctx context.Context) error {
    // 只处理业务相关状态，基础设施已自动恢复
    return handleBusinessReconnect(ctx)
})

// 3. 启动健康检查（包含自动重连）
ctx, cancel := context.WithCancel(context.Background())
bus.StartHealthCheck(ctx)
```

#### 2. 业务回调实现

```go
func handleBusinessReconnect(ctx context.Context) error {
    // 1. 重新加载缓存
    if err := reloadApplicationCache(); err != nil {
        log.Printf("Failed to reload cache: %v", err)
        // 不返回错误，避免影响重连成功状态
    }
    
    // 2. 同步业务状态
    if err := syncBusinessState(); err != nil {
        log.Printf("Failed to sync business state: %v", err)
    }
    
    // 3. 发送监控指标
    recordReconnectMetrics()
    
    // 4. 通知其他服务
    notifyDependentServices("eventbus_reconnected")
    
    return nil
}
```

#### 3. 优雅关闭

```go
// 推荐的关闭顺序
defer func() {
    // 1. 停止健康检查（同步等待）
    bus.StopHealthCheck()
    
    // 2. 取消应用 context
    cancel()
    
    // 3. 关闭 EventBus
    eventbus.CloseGlobal()
}()
```

## 技术实现细节

### 🔄 自动重连流程

1. **健康检查失败** → 累计失败次数
2. **达到阈值** → 触发自动重连
3. **指数退避** → 避免频繁重连
4. **连接重建** → 重新建立底层连接
5. **订阅恢复** → 自动恢复所有订阅
6. **业务回调** → 执行业务状态处理
7. **状态重置** → 重置失败计数

### 🛡️ 容错机制

#### Kafka 实现
- **严格模式**：任何订阅恢复失败都会导致重连失败
- **快速失败**：确保问题能被及时发现

#### NATS 实现
- **宽松模式**：部分订阅失败不影响整体重连
- **双重保障**：客户端重连 + 应用层重连
- **错误收集**：统一报告所有错误

### 📊 监控和观测

```go
// 获取连接状态
connState := bus.GetConnectionState()

// 获取健康状态
healthStatus := bus.GetHealthStatus()

// 获取重连状态（如果支持）
if reconnectable, ok := bus.(interface{ GetReconnectStatus() ReconnectStatus }); ok {
    status := reconnectable.GetReconnectStatus()
}
```

## 最佳实践

### ✅ 推荐做法

1. **轻量级回调**：回调中只处理必要的业务状态
2. **错误容忍**：回调错误不应影响 EventBus 功能
3. **幂等性**：确保回调可以安全重复执行
4. **快速返回**：避免长时间阻塞操作
5. **详细日志**：记录回调执行情况

### ❌ 避免的做法

1. **重复订阅**：不要在回调中重新订阅
2. **阻塞操作**：避免长时间的网络或文件操作
3. **抛出异常**：回调失败不应影响基础功能
4. **状态依赖**：不要假设回调一定成功

## 配置参数

### 重连配置

```go
type ReconnectConfig struct {
    MaxAttempts      int           // 最大重连次数（默认：10）
    InitialBackoff   time.Duration // 初始退避时间（默认：1s）
    MaxBackoff       time.Duration // 最大退避时间（默认：30s）
    BackoffFactor    float64       // 退避因子（默认：2.0）
    FailureThreshold int           // 失败阈值（默认：3）
}
```

### 健康检查配置

```go
// Kafka 配置
HealthCheckInterval: 30 * time.Second

// NATS 配置
// 使用默认 30 秒间隔
```

## 示例代码

完整的示例代码请参考：
- `examples/health_check_auto_reconnect_best_practice.go` - 最佳实践示例
- `examples/auto_reconnect_example.go` - Kafka 自动重连示例
- `examples/nats_auto_reconnect_example.go` - NATS 自动重连示例

## 总结

jxt-core EventBus 组件提供了完整的健康检查和自动重连功能：

1. **对业务透明**：基础设施层面的问题自动处理
2. **智能可靠**：指数退避和失败阈值控制
3. **状态完整**：自动恢复所有订阅状态
4. **回调机制**：支持业务层状态处理
5. **多后端支持**：Kafka 和 NATS 都有完整实现

业务层只需要关注业务相关的状态处理，无需关心底层连接管理和订阅恢复，大大简化了微服务的可靠性实现。
