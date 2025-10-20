# NATS EventBus 自动重连功能实现

## 概述

本文档详细说明了为 jxt-core EventBus 组件的 NATS 实现添加完整自动重连功能的过程和实现细节。

## 实现背景

用户要求检查并完善 NATS 部分的自动重连实现，使其与 Kafka 部分保持一致：

> "jxt-core项目的eventbus组件的kafka部分已经实现了周期性健康检查和自动重连，检查一下nats部分实现也同样完整实现了，如果没有，请完整实现"

## 现状分析

### 实现前的状态

NATS EventBus 已有的功能：
- ✅ 基础的健康检查功能
- ✅ NATS 客户端内置的重连机制
- ✅ 重连回调注册和执行
- ❌ 缺少应用层的自动重连逻辑
- ❌ 缺少失败计数和指数退避
- ❌ 缺少订阅状态管理和恢复

### 需要添加的功能

1. **重连配置管理**：与 Kafka 一致的重连配置结构
2. **失败计数机制**：跟踪连续健康检查失败次数
3. **指数退避算法**：避免频繁重连尝试
4. **订阅状态管理**：保存和恢复订阅信息
5. **自动重连触发**：在健康检查循环中集成重连逻辑

## 实现细节

### 1. 数据结构扩展

```go
type natsEventBus struct {
    // ... 现有字段 ...
    
    // 自动重连控制
    reconnectConfig   ReconnectConfig
    failureCount      atomic.Int32
    lastReconnectTime atomic.Value // time.Time
    reconnectCallback ReconnectCallback

    // 订阅管理（用于重连后恢复订阅）
    subscriptionHandlers map[string]MessageHandler // topic -> handler
    subscriptionsMu      sync.RWMutex
}
```

### 2. 核心方法实现

#### A. 重连配置管理

```go
// SetReconnectConfig 设置重连配置
func (n *natsEventBus) SetReconnectConfig(config ReconnectConfig) error

// GetReconnectStatus 获取重连状态
func (n *natsEventBus) GetReconnectStatus() ReconnectStatus
```

#### B. 重连核心逻辑

```go
// reconnect 执行重连逻辑
func (n *natsEventBus) reconnect(ctx context.Context) error {
    // 1. 使用指数退避算法进行重连尝试
    // 2. 重新初始化连接
    // 3. 恢复订阅状态
    // 4. 调用重连回调
}

// reinitializeConnection 重新初始化 NATS 连接
func (n *natsEventBus) reinitializeConnection() error {
    // 1. 关闭现有连接
    // 2. 重新连接到NATS服务器
    // 3. 重新创建JetStream上下文（如果启用）
    // 4. 设置重连处理器
}

// restoreSubscriptions 恢复所有订阅
func (n *natsEventBus) restoreSubscriptions(ctx context.Context) error {
    // 1. 遍历保存的订阅信息
    // 2. 重新建立每个主题的订阅
    // 3. 更新订阅状态
}
```

#### C. 健康检查集成

```go
// 在健康检查循环中添加自动重连逻辑
case <-ticker.C:
    if err := n.healthCheck(healthCtx); err != nil {
        n.logger.Error("Health check failed", zap.Error(err))
        
        // 增加失败计数
        failureCount := n.failureCount.Add(1)
        
        // 检查是否达到重连阈值
        if failureCount >= int32(n.reconnectConfig.FailureThreshold) {
            // 触发自动重连
            if reconnectErr := n.reconnect(healthCtx); reconnectErr != nil {
                n.logger.Error("Auto-reconnect failed", zap.Error(reconnectErr))
            } else {
                n.logger.Info("Auto-reconnect successful")
                n.failureCount.Store(0) // 重置失败计数
            }
        }
    } else {
        // 健康检查成功，重置失败计数
        n.failureCount.Store(0)
    }
```

### 3. 订阅状态管理

在 `Subscribe` 方法中添加状态保存：

```go
// 保存订阅处理器（用于重连后恢复）
n.subscriptionsMu.Lock()
n.subscriptionHandlers[topic] = handler
n.subscriptionsMu.Unlock()
```

### 4. NATS 客户端重连处理器增强

```go
nc.SetReconnectHandler(func(nc *nats.Conn) {
    n.logger.Info("NATS reconnected", zap.String("url", nc.ConnectedUrl()))
    // 重置失败计数
    n.failureCount.Store(0)
    // 更新重连时间
    n.lastReconnectTime.Store(time.Now())
    // 恢复订阅
    n.restoreSubscriptions(context.Background())
    // 执行重连回调
    n.executeReconnectCallbacks()
})
```

## 关键特性

### 1. 双重保障机制

NATS EventBus 提供了双重重连保障：

1. **NATS 客户端内置重连**：
   - 自动处理网络中断
   - 透明的连接恢复
   - 内置的重连策略

2. **应用层自动重连**：
   - 基于健康检查的主动重连
   - 指数退避算法
   - 完整的订阅状态恢复

### 2. 指数退避算法

```go
func (n *natsEventBus) calculateBackoff(attempt int) time.Duration {
    backoff := float64(n.reconnectConfig.InitialBackoff)
    
    // 指数退避算法
    for i := 1; i < attempt; i++ {
        backoff *= n.reconnectConfig.BackoffFactor
    }
    
    // 限制最大退避时间
    if backoff > float64(n.reconnectConfig.MaxBackoff) {
        backoff = float64(n.reconnectConfig.MaxBackoff)
    }
    
    return time.Duration(backoff)
}
```

### 3. 订阅恢复机制

- **状态保存**：自动保存所有订阅的主题和处理器
- **智能恢复**：区分核心 NATS 和 JetStream 订阅
- **错误处理**：部分订阅失败不影响整体重连成功

## 测试验证

### 1. 单元测试

创建了 `nats_reconnect_test.go` 文件，包含：

- ✅ `TestNATSEventBusReconnect`：完整的重连功能测试
- ✅ `TestNATSDefaultReconnectConfig`：默认配置验证
- ✅ 重连配置管理测试
- ✅ 重连回调注册测试
- ✅ 订阅信息记录测试
- ✅ 重连状态跟踪测试
- ✅ 指数退避计算测试

### 2. 集成测试

- ✅ 所有现有测试通过
- ✅ 与 Kafka 和 Memory 实现兼容
- ✅ 健康检查功能正常

## 使用示例

### 基础用法

```go
// 1. 初始化 NATS EventBus
cfg := eventbus.GetDefaultNATSConfig([]string{"nats://localhost:4222"})
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

// 3. 注册重连回调
err := bus.RegisterReconnectCallback(func(ctx context.Context) error {
    log.Println("NATS EventBus reconnected successfully!")
    return nil
})
```

### 高级配置

```go
// 获取 NATS EventBus 实例
natsEB := bus.(*natsEventBus)

// 自定义重连配置
customConfig := eventbus.ReconnectConfig{
    MaxAttempts:      8,
    InitialBackoff:   200 * time.Millisecond,
    MaxBackoff:       5 * time.Second,
    BackoffFactor:    1.8,
    FailureThreshold: 2,
}

if err := natsEB.SetReconnectConfig(customConfig); err != nil {
    log.Printf("Failed to set NATS reconnect config: %v", err)
}
```

## 总结

✅ **完成目标**：
- NATS EventBus 现在具备与 Kafka EventBus 相同的自动重连能力
- 实现了完整的失败检测、重连逻辑和状态恢复机制
- 保持了与现有 API 的完全兼容性

✅ **核心优势**：
- **双重保障**：客户端内置重连 + 应用层重连机制
- **智能重连**：指数退避算法避免频繁重连
- **状态完整**：自动恢复所有订阅状态
- **高可靠性**：适用于生产环境的企业级可靠性

这个实现确保了 NATS EventBus 在网络不稳定环境中的稳定性和可靠性，为业务应用提供了透明的重连保障。
