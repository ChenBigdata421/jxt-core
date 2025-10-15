# EventBus 健康检查破坏性变更说明

## 变更概述

为了简化 jxt-core EventBus 的健康检查接口，我们移除了单次健康检查的对外接口，只保留周期性健康检查和 HTTP 端点。这是一个破坏性变更，但由于 jxt-core 尚未对外发布，因此可以安全地进行此修改。

## 变更详情

### 移除的接口

#### 1. EventBus 基础接口
```go
// 已移除
type EventBus interface {
    HealthCheck(ctx context.Context) error  // ❌ 已移除
    // ... 其他方法保持不变
}
```

#### 2. EventBusHealthChecker 接口
```go
// 已移除的方法
type EventBusHealthChecker interface {
    CheckConnection(ctx context.Context) error              // ❌ 已移除
    CheckMessageTransport(ctx context.Context) error        // ❌ 已移除
    GetEventBusMetrics() EventBusHealthMetrics              // ❌ 已移除
    PerformHealthCheck(ctx context.Context) (*HealthStatus, error) // ❌ 已移除
}
```

### 保留的接口

#### 1. EventBus 基础接口
```go
type EventBus interface {
    // 发布消息到指定主题
    Publish(ctx context.Context, topic string, message []byte) error
    // 订阅指定主题的消息
    Subscribe(ctx context.Context, topic string, handler MessageHandler) error
    // 关闭连接
    Close() error
    // 注册重连回调
    RegisterReconnectCallback(callback ReconnectCallback) error
    
    // 周期性健康检查
    StartHealthCheck(ctx context.Context) error
    StopHealthCheck() error
    
    // ... 其他高级功能保持不变
}
```

#### 2. EventBusHealthChecker 接口（简化版）
```go
type EventBusHealthChecker interface {
    // 注册业务健康检查
    RegisterBusinessHealthCheck(checker BusinessHealthChecker)
    // 获取健康状态（聚合状态）
    GetHealthStatus() HealthCheckStatus
}
```

## 迁移指南

### 旧的使用方式（已废弃）

```go
// ❌ 不再支持
if err := bus.HealthCheck(ctx); err != nil {
    log.Printf("Health check failed: %v", err)
}

// ❌ 不再支持
if hc, ok := bus.(EventBusHealthChecker); ok {
    status, err := hc.PerformHealthCheck(ctx)
    if err != nil {
        log.Printf("Health check failed: %v", err)
    }
}
```

### 新的使用方式（推荐）

#### 1. 启动周期性健康检查
```go
// ✅ 推荐方式
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := bus.StartHealthCheck(ctx); err != nil {
    log.Printf("Failed to start health check: %v", err)
}

// 获取健康状态
if hc, ok := bus.(EventBusHealthChecker); ok {
    status := hc.GetHealthStatus()
    if status.IsHealthy {
        log.Println("EventBus is healthy")
    } else {
        log.Printf("EventBus unhealthy: %d consecutive failures", status.ConsecutiveFailures)
    }
}
```

#### 2. HTTP 健康端点（推荐）
```go
// ✅ 推荐方式
router := mux.NewRouter()
eventbus.SetupHealthCheckRoutes(router, bus)

// 启动 HTTP 服务
log.Fatal(http.ListenAndServe(":8080", router))

// 提供的端点：
// GET /health  - 完整健康检查
// GET /readyz  - 就绪检查
// GET /livez   - 存活检查
```

## 变更原因

1. **简化接口**：移除了业务很少直接使用的单次健康检查方法
2. **统一模式**：推广周期性健康检查 + HTTP 端点的标准模式
3. **降低复杂性**：减少了接口的复杂性，更容易理解和使用
4. **更好的实践**：鼓励使用更符合微服务最佳实践的健康检查方式

## 内部实现变更

虽然对外接口简化了，但内部实现仍然保留了完整的健康检查能力：

- `checkConnection()` - 内部连接检查
- `checkMessageTransport()` - 内部消息传输检查
- `performHealthCheck()` - 内部完整健康检查
- `getEventBusMetrics()` - 内部指标获取

这些方法被 `StartHealthCheck()` 和 HTTP 端点内部使用。

## 影响范围

由于 jxt-core 尚未对外发布，此变更不会影响外部用户。内部测试和示例代码已全部更新。

## 测试验证

所有相关测试已更新并通过：
- `TestEventBusHealthCheck` - 周期性健康检查测试
- `TestHealthCheckHandler` - HTTP 端点测试
- `BenchmarkHealthCheckStatus` - 性能基准测试

## 文档更新

- `README.md` - 更新了健康检查使用说明
- `health-check-example.go` - 更新了示例代码
- 所有测试文件已更新

## 总结

这次变更简化了 EventBus 的健康检查接口，使其更加专注和易用。业务代码应该使用：

1. **周期性健康检查**：`StartHealthCheck(ctx)` + `GetHealthStatus()`
2. **HTTP 健康端点**：`SetupHealthCheckRoutes(router, bus)`

这种方式更符合现代微服务的健康检查最佳实践。
