# NATS EventBus 协程泄漏修复分析

## 📊 问题现状

### 测试结果
```
Low:     初始 778  -> 最终 5908  (泄漏 5130)
Medium:  初始 780  -> 最终 5910  (泄漏 5130)
High:    初始 782  -> 最终 5912  (泄漏 5130)
Extreme: 初始 784  -> 最终 5914  (泄漏 5130)
```

**关键发现**：
- 每个测试场景都泄漏 **5130 个协程**
- 泄漏数量固定，不随消息数量变化
- Kafka 只泄漏 134 个（可接受）

---

## 🔍 已实施的修复

### 修复 1: 订阅协程管理

**文件**: `sdk/pkg/eventbus/nats.go`

**问题**: `processUnifiedPullMessages` 协程使用外部 context，Close() 无法停止

**修复**:
```go
// 添加字段（Line 282-285）
subscriptionCtx    context.Context
subscriptionCancel context.CancelFunc
subscriptionWg     sync.WaitGroup

// 初始化（Line 337-339）
subscriptionCtx, subscriptionCancel := context.WithCancel(context.Background())
bus.subscriptionCtx = subscriptionCtx
bus.subscriptionCancel = subscriptionCancel

// 启动协程时跟踪（Line 1151-1155）
n.subscriptionWg.Add(1)
go func() {
    defer n.subscriptionWg.Done()
    n.processUnifiedPullMessages(n.subscriptionCtx, topic, sub)
}()

// Close() 中取消并等待（Line 1424-1440）
if n.subscriptionCancel != nil {
    n.subscriptionCancel()
}
n.subscriptionWg.Wait()
```

**预期效果**: 修复 5 个 topic × 1 个协程 = 5 个协程泄漏

---

## 🐛 发现的新问题

### 问题 1: UnifiedWorkerPool.smartDispatcher() 未被跟踪

**文件**: `sdk/pkg/eventbus/unified_worker_pool.go`

**问题**: Line 104 启动了 `smartDispatcher()` 协程，但没有 `wg.Add(1)`

```go
// Line 104
go pool.smartDispatcher()  // ❌ 没有 wg.Add(1)

// Line 197-209: Close() 方法
func (p *UnifiedWorkerPool) Close() {
    p.cancel()
    for _, worker := range p.workers {
        close(worker.quit)
    }
    p.wg.Wait()  // ❌ 不会等待 smartDispatcher
}
```

**影响**: 每个 EventBus 泄漏 1 个 dispatcher 协程

---

## 📐 协程数量计算

### 理论计算（24 核 CPU）

| 组件 | 数量 | 计算 |
|------|------|------|
| UnifiedWorkerPool workers | 768 | 24 × 32 |
| UnifiedWorkerPool dispatcher | 1 | 固定 |
| processUnifiedPullMessages | 5 | 5 topics |
| backlogDetector.monitorLoop | 1 | 固定 |
| healthCheck goroutine | 1 | 固定（如果启动） |
| **理论总计** | **776** | |
| **实际泄漏** | **5130** | |
| **差距** | **4354** | ❓ |

---

## 🤔 未解之谜

**问题**: 为什么实际泄漏 5130 个，而理论只有 776 个？

**可能原因**:

1. **NATS 客户端内部协程**
   - NATS Conn 可能创建了大量内部协程
   - JetStream 可能有额外的后台协程

2. **Pull Subscription 内部协程**
   - 每个 `sub.Fetch()` 可能创建临时协程
   - 5 个 subscriptions × N 个内部协程

3. **其他未发现的协程创建点**
   - 需要使用 pprof 分析协程堆栈

---

## 🔬 下一步调查

### 方案 1: 使用 pprof 分析协程堆栈

```go
import (
    "runtime/pprof"
    "os"
)

// 在 Close() 之前
f, _ := os.Create("goroutine_before_close.txt")
pprof.Lookup("goroutine").WriteTo(f, 1)
f.Close()

// 在 Close() 之后
f2, _ := os.Create("goroutine_after_close.txt")
pprof.Lookup("goroutine").WriteTo(f2, 1)
f2.Close()
```

### 方案 2: 检查 NATS 客户端文档

查看 NATS Go 客户端是否有已知的协程泄漏问题或需要特殊的关闭步骤。

### 方案 3: 简化测试

创建一个最小化测试，只创建 EventBus 然后立即关闭，看看泄漏多少协程。

---

## 📝 总结

### 已修复
- ✅ `processUnifiedPullMessages` 协程管理（5 个）

### 待修复
- ❌ `smartDispatcher` 协程泄漏（1 个）
- ❌ 未知来源的 4354 个协程泄漏

### 修复效果
- 预期减少: 6 个协程
- 实际减少: 0 个（测试结果仍然是 5130）
- **结论**: 主要泄漏源尚未找到

---

## 🚨 紧急建议

由于主要泄漏源尚未找到，建议：

1. **立即**: 使用 pprof 分析协程堆栈
2. **短期**: 检查 NATS 客户端是否有特殊的关闭要求
3. **长期**: 考虑是否需要重新设计 NATS EventBus 的架构

---

**创建时间**: 2025-10-13
**状态**: 🔴 未解决（修复无效）

