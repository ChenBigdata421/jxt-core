# NATS 性能测试修复总结

## 📊 修复前后对比

### ✅ 问题 1: Write I/O Timeout - **已修复**

#### 修复前
```
write tcp [::1]:62311->[::1]:4223: i/o timeout on connection [1189]
```
频繁出现 I/O 超时错误

#### 修复后
**0 个 I/O timeout 错误** ✅

#### 修复内容
在 `sdk/pkg/eventbus/nats.go:509-518` 添加了以下配置：

```go
// ✅ 修复问题1: 增加写入刷新超时配置（防止 I/O timeout）
opts = append(opts, nats.FlusherTimeout(10*time.Second))

// ✅ 修复问题1: 增加心跳配置（保持连接活跃）
opts = append(opts, nats.PingInterval(20*time.Second))

// ✅ 修复问题1: 增加重连缓冲区大小（默认32KB -> 1MB）
opts = append(opts, nats.ReconnectBufSize(1024*1024))
```

---

### ⚠️ 问题 2: Goroutine 泄漏 - **部分修复**

#### 修复前
```
📊 NATS 资源清理完成: 初始 3 -> 最终 4 (泄漏 1)
📊 NATS 资源清理完成: 初始 4 -> 最终 5 (泄漏 1)
📊 NATS 资源清理完成: 初始 5 -> 最终 6 (泄漏 1)
📊 NATS 资源清理完成: 初始 6 -> 最终 7 (泄漏 1)
```
每次测试泄漏 1 个 goroutine

#### 修复后
```
📊 NATS 资源清理完成: 初始 3 -> 最终 4 (泄漏 1)
📊 NATS 资源清理完成: 初始 4 -> 最终 5 (泄漏 1)
📊 NATS 资源清理完成: 初始 5 -> 最终 6 (泄漏 1)
📊 NATS 资源清理完成: 初始 6 -> 最终 7 (泄漏 1)
```
**仍然泄漏 1 个 goroutine** ⚠️

#### 修复内容

1. **添加了 context 控制**（`sdk/pkg/eventbus/nats.go:283-287`）:
```go
// ✅ 修复问题2: 异步ACK处理的生命周期控制
asyncAckCtx    context.Context
asyncAckCancel context.CancelFunc
asyncAckWg     sync.WaitGroup
```

2. **在 NewNATSEventBus 中初始化**（`sdk/pkg/eventbus/nats.go:333-355`）:
```go
// ✅ 修复问题2: 创建异步ACK处理的context
asyncAckCtx, asyncAckCancel := context.WithCancel(context.Background())

bus := &natsEventBus{
    // ...
    asyncAckCtx:    asyncAckCtx,
    asyncAckCancel: asyncAckCancel,
}
```

3. **在 PublishEnvelope 中添加超时和 context 控制**（`sdk/pkg/eventbus/nats.go:2489-2588`）:
```go
// ✅ 修复问题2: 使用WaitGroup跟踪goroutine，添加超时控制
n.asyncAckWg.Add(1)
go func() {
    defer n.asyncAckWg.Done()
    
    // ✅ 计算超时时间（默认30秒）
    timeout := 30 * time.Second
    if n.config.JetStream.PublishTimeout > 0 {
        timeout = n.config.JetStream.PublishTimeout
    }
    
    select {
    case <-pubAckFuture.Ok():
        // 成功
    case err := <-pubAckFuture.Err():
        // 失败
    case <-n.asyncAckCtx.Done():
        // ✅ EventBus关闭，退出goroutine
        return
    case <-time.After(timeout):
        // ⏰ 超时
    }
}()
```

4. **在 Close 方法中等待所有 goroutine 退出**（`sdk/pkg/eventbus/nats.go:1439-1457`）:
```go
// ✅ 修复问题2: 取消所有异步ACK处理goroutine
if n.asyncAckCancel != nil {
    n.logger.Info("Cancelling all async ACK goroutines...")
    n.asyncAckCancel()
    
    // ✅ 等待所有异步ACK goroutine退出
    done := make(chan struct{})
    go func() {
        n.asyncAckWg.Wait()
        close(done)
    }()
    
    select {
    case <-done:
        n.logger.Info("All async ACK goroutines exited")
    case <-time.After(5 * time.Second):
        n.logger.Warn("Timeout waiting for async ACK goroutines to exit")
    }
}
```

#### 为什么仍然泄漏 1 个 goroutine？

经过分析，泄漏的 1 个 goroutine 可能来自以下几个地方：

1. **NATS 客户端内部的 goroutine**：
   - NATS 客户端库本身会创建一些后台 goroutine（如心跳、重连等）
   - 这些 goroutine 可能在 `conn.Close()` 后没有立即退出

2. **JetStream 的异步发布处理器**：
   - `PublishAsyncErrHandler` 可能创建了一个长期运行的 goroutine
   - 这个 goroutine 在 `Close()` 时可能没有被正确清理

3. **测试环境的 goroutine**：
   - 可能是测试框架或其他组件创建的 goroutine

#### 进一步调查

需要使用 `runtime.Stack()` 或 `pprof` 来精确定位泄漏的 goroutine：

```go
// 在测试中添加
buf := make([]byte, 1<<20)
stackSize := runtime.Stack(buf, true)
fmt.Printf("=== Goroutine Stack Trace ===\n%s\n", buf[:stackSize])
```

---

### 📈 问题 3: NATS 性能低于 Kafka - **未修复**

#### 性能对比（修复后）

| 指标 | Kafka | NATS | 差距 |
|------|-------|------|------|
| **极限吞吐量** | 3333 msg/s | 327 msg/s | **10.2倍** |
| **平均发送延迟** | 2.17 ms | 21.68 ms | **10倍** |
| **平均处理延迟** | 0.005 ms | 0.015 ms | 3倍 |
| **峰值协程数** | 4452 | 15463 | 3.5倍 |
| **内存增量** | 22.55 MB | 30.39 MB | 1.35倍 |

#### 性能差距原因

1. **PublishEnvelope 实现效率低**：
   - 每次都创建 Header 对象
   - 每次都启动一个新的 goroutine 等待 ACK
   - 没有批量发布优化

2. **高并发 goroutine 创建**：
   - 极限测试：1000个聚合ID × 10条消息 = **10000个 goroutine**
   - 每个 goroutine 都在等待 ACK
   - 导致峰值协程数达到 15463

3. **缺少批量发布机制**：
   - Kafka 的 Sarama 库内部实现了批量发送
   - NATS 当前实现是单条发送

#### 优化方案（待实施）

**方案 1: 简化 PublishEnvelope**
```go
func (n *natsEventBus) PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error {
    // 序列化
    envelopeBytes, err := envelope.ToBytes()
    if err != nil {
        return err
    }
    
    // ✅ 简化：直接发布，不创建Header
    _, err = n.js.PublishAsync(topic, envelopeBytes)
    return err
}
```

**方案 2: 使用共享的 ACK 处理 goroutine**
```go
// 启动一个共享的 ACK 处理 goroutine
func (n *natsEventBus) startAckProcessor() {
    go func() {
        for {
            select {
            case future := <-n.ackFutureChan:
                // 处理 ACK
            case <-n.asyncAckCtx.Done():
                return
            }
        }
    }()
}
```

**方案 3: 实现批量发布**
```go
type publishBatch struct {
    messages []*nats.Msg
    mu       sync.Mutex
}

func (n *natsEventBus) flushBatch() {
    // 批量发布所有消息
    for _, msg := range batch.messages {
        n.js.PublishMsgAsync(msg)
    }
}
```

---

## 📋 修复总结

### ✅ 已修复
1. **Write I/O Timeout**: 通过增加 `FlusherTimeout`、`PingInterval` 和 `ReconnectBufSize` 配置完全解决

### ⚠️ 部分修复
2. **Goroutine 泄漏**: 添加了 context 控制和超时机制，但仍然泄漏 1 个 goroutine（需要进一步调查）

### ❌ 未修复
3. **性能差距**: NATS 性能仍然显著低于 Kafka（10倍差距），需要实施优化方案

---

## 🎯 下一步行动

### P0 - 立即执行
1. **调查 goroutine 泄漏源头**：
   - 使用 `pprof` 或 `runtime.Stack()` 定位泄漏的 goroutine
   - 检查 NATS 客户端库的 goroutine 管理

### P1 - 短期优化
2. **简化 PublishEnvelope 实现**：
   - 移除不必要的 Header 创建
   - 减少 goroutine 创建

3. **实现共享 ACK 处理器**：
   - 使用单个 goroutine 处理所有 ACK
   - 减少 goroutine 数量

### P2 - 长期优化
4. **实现批量发布机制**：
   - 缓冲消息
   - 批量发送
   - 提升吞吐量

5. **性能基准测试**：
   - 持续监控性能指标
   - 对比优化效果

---

## 📊 测试结果

### 修复后测试结果
- **所有场景通过**: ✅
- **成功率**: 100% (Kafka 和 NATS)
- **顺序性**: 0 违反
- **I/O Timeout**: 0 错误 ✅
- **Goroutine 泄漏**: 每次测试泄漏 1 个 ⚠️

### 性能指标
| 压力级别 | Kafka 吞吐量 | NATS 吞吐量 | 差距 |
|---------|-------------|------------|------|
| 低压(500) | 333 msg/s | 333 msg/s | 0% |
| 中压(2000) | 1333 msg/s | 1333 msg/s | 0% |
| 高压(5000) | 1667 msg/s | 1665 msg/s | 0.1% |
| 极限(10000) | 3333 msg/s | 327 msg/s | **10.2倍** |

**结论**: 在低、中、高压场景下，NATS 性能与 Kafka 相当。但在极限场景下，NATS 性能显著下降。

---

## 🔧 修改的文件

1. **sdk/pkg/eventbus/nats.go**:
   - 第 283-287 行：添加异步 ACK 处理的生命周期控制字段
   - 第 333-355 行：初始化 context 和 cancel 函数
   - 第 509-518 行：增加 NATS 连接选项配置
   - 第 1439-1457 行：在 Close 方法中等待所有 goroutine 退出
   - 第 2489-2588 行：在 PublishEnvelope 中添加超时和 context 控制

2. **tests/eventbus/performance_tests/NATS_PERFORMANCE_ISSUES_ANALYSIS.md**:
   - 新增：详细的问题分析报告

3. **tests/eventbus/performance_tests/NATS_FIX_SUMMARY.md**:
   - 新增：修复总结报告

---

## 💡 建议

1. **Goroutine 泄漏**：
   - 虽然只泄漏 1 个 goroutine，但在生产环境中长期运行可能累积
   - 建议使用 `pprof` 进一步调查

2. **性能优化**：
   - 当前 NATS 性能在极限场景下显著低于 Kafka
   - 建议实施优化方案，特别是批量发布机制

3. **测试覆盖**：
   - 当前测试已经覆盖了低、中、高、极限四种压力场景
   - 建议增加更长时间的稳定性测试

4. **监控告警**：
   - 建议在生产环境中监控 goroutine 数量
   - 设置告警阈值，及时发现泄漏问题

