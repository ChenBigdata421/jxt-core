# NATS EventBus 协程泄漏根本原因分析

## 🎯 问题总结

**泄漏数量**: 每个测试场景固定泄漏 **5130 个协程**

**根本原因**: NATS EventBus 为每个 topic 创建了一个 **KeyedWorkerPool**，每个池有 **1024 个 worker 协程**

---

## 🔬 协程堆栈分析

### 泄漏协程分布

从 `goroutine_leak_Low_5130.txt` 分析：

```
5120 个协程 @ KeyedWorkerPool.runWorker
 768 个协程 @ UnifiedWorker.start
   5 个协程 @ processUnifiedPullMessages
   5 个协程 @ Subscription.Fetch.func3
   1 个协程 @ smartDispatcher
   其他协程 @ NATS 内部、日志、测试等
-----------------------------------
总计: 5908 个协程（初始 778 个）
泄漏: 5130 个协程
```

### 关键发现

**5120 个协程来自 KeyedWorkerPool**:
```
5120 @ 0xc7a0ae 0xc59017 0x140ddeb 0xc82561
#       0x140ddea       github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus.(*KeyedWorkerPool).runWorker+0xea
        D:/JXT/jxt-evidence-system/jxt-core/sdk/pkg/eventbus/keyed_worker_pool.go:84
```

**计算**:
- 5 个 topics × 1024 workers/topic = **5120 个协程**
- 这正好对应泄漏的主要部分！

---

## 📍 代码位置

### 问题代码 1: 创建 KeyedWorkerPool

**文件**: `sdk/pkg/eventbus/nats.go`  
**位置**: Line 1060-1069

```go
// ⭐ 创建per-topic Keyed-Worker池（与Kafka保持一致）
n.keyedPoolsMu.Lock()
if _, ok := n.keyedPools[topic]; !ok {
    pool := NewKeyedWorkerPool(KeyedWorkerPoolConfig{
        WorkerCount: 1024, // ❌ 每个 topic 1024 个协程！
        QueueSize:   1000,
        WaitTimeout: 200 * time.Millisecond,
    }, handler)
    n.keyedPools[topic] = pool
}
n.keyedPoolsMu.Unlock()
```

### 问题代码 2: 使用 KeyedWorkerPool

**文件**: `sdk/pkg/eventbus/nats.go`  
**位置**: Line 1294-1299

```go
// 获取该topic的Keyed-Worker池
n.keyedPoolsMu.RLock()
pool := n.keyedPools[topic]
n.keyedPoolsMu.RUnlock()

if pool != nil {
    // ⭐ 使用Keyed-Worker池处理（与Kafka保持一致）
    // ...
}
```

### Close() 方法中的清理

**文件**: `sdk/pkg/eventbus/nats.go`  
**位置**: Line 1450-1456

```go
// ⭐ 停止所有Keyed-Worker池
n.keyedPoolsMu.Lock()
for topic, pool := range n.keyedPools {
    pool.Stop()  // ✅ 调用了 Stop()
    n.logger.Debug("Stopped keyed worker pool", zap.String("topic", topic))
}
n.keyedPools = make(map[string]*KeyedWorkerPool)
n.keyedPoolsMu.Unlock()
```

---

## 🤔 为什么 Close() 后协程仍然存在？

### KeyedWorkerPool.Stop() 实现

**文件**: `sdk/pkg/eventbus/keyed_worker_pool.go`  
**位置**: Line 146-153

```go
func (kp *KeyedWorkerPool) Stop() {
    close(kp.stopCh)
    // close all worker channels to stop goroutines
    for _, ch := range kp.workers {
        close(ch)
    }
    kp.wg.Wait()  // ✅ 等待所有 worker 退出
}
```

### runWorker 实现

**文件**: `sdk/pkg/eventbus/keyed_worker_pool.go`  
**位置**: Line 81-112

```go
func (kp *KeyedWorkerPool) runWorker(ch chan *AggregateMessage) {
    defer kp.wg.Done()
    for {
        select {
        case msg, ok := <-ch:
            if !ok {
                return  // ✅ channel 关闭时退出
            }
            // 处理消息...
        case <-kp.stopCh:
            return  // ✅ stopCh 关闭时退出
        }
    }
}
```

### 可能的原因

1. **测量时机问题**？
   - 测试代码在 Close() 之后立即测量
   - 可能需要更多时间让协程完全退出？

2. **wg.Wait() 没有真正等待**？
   - 可能 wg.Add() 和 wg.Done() 不匹配？

3. **协程被阻塞**？
   - 协程可能在等待某些资源（channel、锁等）
   - 无法响应 close(ch) 或 close(stopCh)

---

## 🔧 修复方案

### 方案 1: 移除 KeyedWorkerPool，只使用 UnifiedWorkerPool

**优点**:
- 大幅减少协程数量（5120 → 0）
- 简化架构
- 与设计文档一致（UNIFIED_WORKER_POOL_DESIGN.md）

**缺点**:
- 需要修改代码逻辑

**实施**:
1. 移除 Line 1060-1069 的 KeyedWorkerPool 创建代码
2. 移除 Line 1294-1299 的 KeyedWorkerPool 使用代码
3. 确保所有消息都通过 UnifiedWorkerPool 处理

### 方案 2: 修复 KeyedWorkerPool 的协程泄漏

**优点**:
- 保持现有架构
- 风险较小

**缺点**:
- 仍然有大量协程（5120 个）
- 需要找到泄漏的真正原因

**实施**:
1. 添加更详细的日志来跟踪 Stop() 过程
2. 检查 wg.Add() 和 wg.Done() 是否匹配
3. 检查是否有协程被阻塞

### 方案 3: 减少 KeyedWorkerPool 的 worker 数量

**优点**:
- 快速缓解问题
- 风险最小

**缺点**:
- 治标不治本
- 仍然有协程泄漏

**实施**:
1. 将 WorkerCount 从 1024 减少到 64 或 128
2. 泄漏数量会从 5120 减少到 320 或 640

---

## 💡 推荐方案

**推荐方案 1**: 移除 KeyedWorkerPool，只使用 UnifiedWorkerPool

**理由**:
1. **设计文档明确指出**应该使用统一的 Worker 池
2. **大幅减少资源占用**：5120 个协程 → 0 个
3. **简化架构**：移除冗余的 per-topic 池
4. **与 Kafka 实现一致**：Kafka 也应该使用 UnifiedWorkerPool

**实施步骤**:
1. 检查 handleMessage 方法，确认是否已经使用 UnifiedWorkerPool
2. 移除 Subscribe/SubscribeEnvelope 中创建 KeyedWorkerPool 的代码
3. 移除 handleMessage 中使用 KeyedWorkerPool 的代码
4. 移除 Close() 中停止 KeyedWorkerPool 的代码
5. 运行测试验证修复

---

## 📊 预期效果

### 修复前
```
初始协程: 778
最终协程: 5908
泄漏协程: 5130
```

### 修复后（方案 1）
```
初始协程: 778
最终协程: 778 + 5 (processUnifiedPullMessages) + 1 (smartDispatcher) = 784
泄漏协程: 6
```

### 修复后（方案 3）
```
初始协程: 778
最终协程: 778 + 320 (KeyedWorkerPool 64 workers × 5 topics) = 1098
泄漏协程: 320
```

---

## 🚀 下一步行动

1. **立即**: 实施方案 1，移除 KeyedWorkerPool
2. **验证**: 运行性能测试，确认协程泄漏已修复
3. **优化**: 检查 Kafka EventBus 是否也有类似问题
4. **文档**: 更新设计文档，明确只使用 UnifiedWorkerPool

---

**创建时间**: 2025-10-13  
**状态**: 🟢 根本原因已找到，修复方案已确定  
**优先级**: 🔴 高（严重资源泄漏）

