# JXT-Core EventBus 自动重连功能实现总结

## 概述

本文档总结了 jxt-core EventBus 组件自动重连功能的完整实现，包括设计理念、核心功能、使用方式和最佳实践。

## 实现背景

### 问题分析

用户询问：
> "当前jxt-core项目的eventbus组件，是否已经实现当周期性健康检查发现到kafka的连接中断，会自动重连吗？"

经过分析发现：
- ✅ 已实现周期性健康检查
- ✅ 能够检测到 Kafka 连接中断
- ❌ 但健康检查失败时只记录错误日志，没有触发重连

用户进一步要求：
> "jxt-core项目的eventbus组件的kafka部分已经实现了周期性健康检查和自动重连，检查一下nats部分实现也同样完整实现了，如果没有，请完整实现"

经过检查发现 NATS 部分缺少完整的自动重连机制。

### 设计决策

**在 jxt-core 项目的 eventbus 组件内部实现自动重连**，理由：
1. **封装性**：基础设施层面的重连逻辑应该对业务层透明
2. **一致性**：所有使用 EventBus 的业务服务都能获得相同的可靠性保障
3. **简化性**：业务层无需关心底层连接管理细节
4. **可维护性**：集中管理重连逻辑，便于统一优化和调试
5. **多后端统一**：Kafka 和 NATS 都提供相同的自动重连能力

## 核心功能实现

### 1. 重连配置结构

```go
// ReconnectConfig 重连配置
type ReconnectConfig struct {
    MaxAttempts      int           // 最大重连尝试次数
    InitialBackoff   time.Duration // 初始退避时间
    MaxBackoff       time.Duration // 最大退避时间
    BackoffFactor    float64       // 退避时间倍增因子
    FailureThreshold int           // 触发重连的连续失败次数
}

// ReconnectStatus 重连状态
type ReconnectStatus struct {
    FailureCount      int32     // 当前连续失败次数
    LastReconnectTime time.Time // 最后一次重连时间
}
```

### 2. 核心重连方法

#### A. Kafka 重连方法
```go
func (k *kafkaEventBus) reconnect(ctx context.Context) error {
    // 1. 检查重连次数限制
    // 2. 计算退避时间（指数退避算法）
    // 3. 执行重连逻辑
    // 4. 恢复订阅状态
    // 5. 调用重连回调
}

func (k *kafkaEventBus) reinitializeConnection() error {
    // 1. 关闭现有连接
    // 2. 重新创建 Sarama 客户端
    // 3. 重新创建生产者和消费者
    // 4. 重新创建管理客户端
}

func (k *kafkaEventBus) restoreSubscriptions(ctx context.Context) error {
    // 1. 遍历保存的订阅信息
    // 2. 重新建立每个主题的订阅
    // 3. 更新订阅状态
}
```

#### B. NATS 重连方法
```go
func (n *natsEventBus) reconnect(ctx context.Context) error {
    // 1. 检查重连次数限制
    // 2. 计算退避时间（指数退避算法）
    // 3. 执行重连逻辑
    // 4. 恢复订阅状态
    // 5. 调用重连回调
}

func (n *natsEventBus) reinitializeConnection() error {
    // 1. 关闭现有连接
    // 2. 重新创建 NATS 连接
    // 3. 重新创建 JetStream 上下文（如果启用）
    // 4. 确保流存在
}

func (n *natsEventBus) restoreSubscriptions(ctx context.Context) error {
    // 1. 遍历保存的订阅信息
    // 2. 重新建立每个主题的订阅（核心 NATS 或 JetStream）
    // 3. 更新订阅状态
}
```

### 3. 健康检查集成

```go
// 在健康检查循环中集成自动重连
case <-ticker.C:
    if err := k.healthCheck(healthCtx); err != nil {
        k.logger.Error("Health check failed", zap.Error(err))
        
        // 增加失败计数
        failureCount := k.failureCount.Add(1)
        
        // 检查是否达到重连阈值
        if failureCount >= int32(k.reconnectConfig.FailureThreshold) {
            k.logger.Warn("Health check failure threshold reached, attempting reconnect",
                zap.Int32("failureCount", failureCount),
                zap.Int("threshold", k.reconnectConfig.FailureThreshold))
            
            // 触发自动重连
            if reconnectErr := k.reconnect(healthCtx); reconnectErr != nil {
                k.logger.Error("Auto-reconnect failed", zap.Error(reconnectErr))
            } else {
                k.logger.Info("Auto-reconnect successful")
                k.failureCount.Store(0) // 重置失败计数
            }
        }
    } else {
        // 健康检查成功，重置失败计数
        k.failureCount.Store(0)
    }
```

## 关键特性

### 1. 指数退避算法
- **初始退避时间**：1秒
- **最大退避时间**：30秒
- **退避因子**：2.0
- **算法**：`backoff = min(initialBackoff * (factor ^ attempt), maxBackoff)`

### 2. 失败阈值控制
- **默认阈值**：连续失败3次触发重连
- **可配置**：支持自定义失败阈值
- **智能重置**：健康检查成功时自动重置计数

### 3. 订阅状态管理
- **状态保存**：自动保存所有订阅的主题和处理器
- **状态恢复**：重连成功后自动恢复所有订阅
- **线程安全**：使用读写锁保护订阅状态

### 4. 回调通知机制
- **重连回调**：支持注册重连成功后的回调函数
- **异步执行**：回调函数异步执行，不阻塞重连流程
- **错误处理**：回调执行失败不影响重连结果

## 使用方式

### 1. 基础用法（自动启用）

```go
// 1. 初始化 EventBus（自动启用重连）
cfg := eventbus.GetDefaultKafkaConfig([]string{"localhost:9092"})
if err := eventbus.InitializeFromConfig(cfg); err != nil {
    log.Fatal(err)
}

bus := eventbus.GetGlobal()

// 2. 启动健康检查（包含自动重连）
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := bus.StartHealthCheck(ctx); err != nil {
    log.Printf("Failed to start health check: %v", err)
}

// 3. 注册重连回调（可选）
err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Println("EventBus reconnected successfully!")
    return nil
})
```

### 2. 监控和调试

```go
// 获取连接状态
connState := bus.GetConnectionState()
fmt.Printf("Connected: %v, Reconnect Count: %d\n", 
    connState.IsConnected, connState.ReconnectCount)

// 获取健康状态
healthStatus := bus.GetHealthStatus()
fmt.Printf("Healthy: %v, Consecutive Failures: %d\n",
    healthStatus.IsHealthy, healthStatus.ConsecutiveFailures)
```

## 测试验证

### 1. 单元测试
- ✅ `TestDefaultReconnectConfig`：验证默认重连配置
- ✅ `TestKafkaEventBusReconnect`：验证 Kafka 重连功能（需要 Kafka 环境）
- ✅ `TestNATSEventBusReconnect`：验证 NATS 重连功能（需要 NATS 环境）
- ✅ `TestHealthCheckFailureHandling`：验证失败处理逻辑

### 2. 集成测试
- ✅ 所有现有测试通过，确保没有破坏现有功能
- ✅ Kafka 和 NATS 健康检查和重连功能正常工作
- ✅ 内存实现保持兼容

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| MaxAttempts | 10 | 最大重连尝试次数 |
| InitialBackoff | 1s | 初始退避时间 |
| MaxBackoff | 30s | 最大退避时间 |
| BackoffFactor | 2.0 | 退避时间倍增因子 |
| FailureThreshold | 3 | 触发重连的连续失败次数 |

## 最佳实践

### 1. 生产环境建议
- 使用 `StopHealthCheck()` 方法停止健康检查（比 context 取消更可靠）
- 注册重连回调进行监控和告警
- 合理设置重连参数，避免过于频繁的重连尝试

### 2. 优雅关闭序列
```go
// 1. 停止健康检查和自动重连
if err := bus.StopHealthCheck(); err != nil {
    log.Printf("Error stopping health check: %v", err)
}

// 2. 取消应用 context
cancel()

// 3. 关闭 EventBus 资源
if err := eventbus.CloseGlobal(); err != nil {
    log.Printf("Error closing EventBus: %v", err)
}
```

## 总结

✅ **完成目标**：
- 在 jxt-core EventBus 组件内部实现了完整的自动重连机制
- **Kafka 和 NATS 都支持**完整的自动重连功能
- 参考 evidence-management 项目的可靠性模式
- 提供了智能的失败检测和恢复能力
- 保持了 API 的简洁性和向后兼容性

✅ **核心价值**：
- **透明性**：业务层无需关心重连逻辑
- **可靠性**：自动处理网络中断和连接故障
- **智能性**：指数退避算法避免频繁重连
- **可观测性**：提供完整的状态监控和回调机制
- **多后端统一**：Kafka、NATS、Memory 都提供一致的接口

✅ **特殊优势**：
- **Kafka**：完全自主的重连机制，不依赖客户端内置功能
- **NATS**：双重保障 - 客户端内置重连 + 应用层重连机制
- **Memory**：开发测试友好，生产环境可无缝切换

这个实现为 jxt-core EventBus 组件提供了企业级的可靠性保障，确保在网络不稳定的环境中也能保持稳定的消息传递能力。无论使用 Kafka 还是 NATS 作为底层消息中间件，都能获得相同的可靠性体验。
