# Kafka EventBus Subscribe() 迁移到 Hollywood Actor Pool - 实施计划文档

**文档版本**: v1.0  
**创建日期**: 2025-10-30  
**状态**: 待评审  
**作者**: AI Assistant  

---

## 📋 目录

1. [实施概览](#实施概览)
2. [代码修改清单](#代码修改清单)
3. [详细实施步骤](#详细实施步骤)
4. [测试验证计划](#测试验证计划)
5. [回滚方案](#回滚方案)

---

## 实施概览

### 🎯 **实施目标**

1. 修改 `processMessageWithKeyedPool` 方法，统一使用 Hollywood Actor Pool
2. 删除全局 Worker Pool 相关代码（约 200+ 行，以符号名为准）
3. 更新路由逻辑，支持 Round-Robin 轮询路由
4. 添加轮询计数器字段到 `kafkaEventBus` 结构
5. 更新测试用例，验证新实现
6. 更新文档，说明架构变化

### 📊 **工作量估算**

| 任务 | 文件数 | 代码行数 | 预计时间 |
|------|-------|---------|---------|
| **代码修改** | 1 | ~50 行修改 + 200+ 行删除 | 2 小时 |
| **测试修改** | 0 | 0 行（现有测试无需修改） | 0 小时 |
| **文档更新** | 2 | ~100 行 | 1 小时 |
| **测试验证** | - | - | 2.5 小时 |
| **总计** | 3 | ~350 行 | 5.5 小时 |

### 🔄 **实施阶段**

```
阶段 1: 代码修改（2 小时）
  ├─ 步骤 1: 修改路由逻辑（30 分钟）
  ├─ 步骤 2: 删除 Worker Pool 代码（30 分钟）
  ├─ 步骤 3: 清理初始化和关闭逻辑（30 分钟）
  └─ 步骤 4: 代码审查和优化（30 分钟）

阶段 2: 测试验证（2 小时）
  ├─ 步骤 5: 运行现有测试（30 分钟）
  ├─ 步骤 6: 性能测试（1 小时）
  └─ 步骤 7: 可靠性测试（30 分钟）

阶段 3: 文档更新（1 小时）
  ├─ 步骤 8: 更新 README（30 分钟）
  └─ 步骤 9: 更新架构文档（30 分钟）
```

---

## 代码修改清单

### 📝 **文件修改清单**

#### 1. `jxt-core/sdk/pkg/eventbus/kafka.go`

**修改类型**: 重构 + 删除

**修改内容**（以符号名为准，行号仅供参考）:

| 符号/方法名 | 修改类型 | 说明 |
|-----------|---------|------|
| `WorkItem` 结构体 | ❌ 删除 | 删除全局 Worker Pool 工作项定义 |
| `GlobalWorkerPool` 结构体 | ❌ 删除 | 删除全局 Worker Pool 及所有方法 |
| `Worker` 结构体 | ❌ 删除 | 删除 Worker 及所有方法 |
| `NewGlobalWorkerPool()` | ❌ 删除 | 删除全局 Worker Pool 构造函数 |
| `SubmitWork()` | ❌ 删除 | 删除工作提交方法 |
| `processMessageDirectly()` | ❌ 删除 | 删除直接处理消息的后备方案 |
| `globalWorkerPool` 字段 | ❌ 删除 | 删除 kafkaEventBus 中的字段 |
| `globalWorkerPool` 初始化 | ❌ 删除 | 删除 NewKafkaEventBus 中的初始化代码 |
| `globalWorkerPool.Close()` | ❌ 删除 | 删除 Close() 中的清理代码 |
| `processMessageWithKeyedPool()` | ✏️ 修改 | 修改路由逻辑，统一使用 Hollywood Actor Pool |

**预计修改行数**: ~50 行修改 + 200+ 行删除

**删除的核心符号**:
- `WorkItem` 结构体（~10 行）
- `GlobalWorkerPool` 结构体及方法（~160 行）
- `Worker` 结构体及方法（~45 行）
- `processMessageDirectly` 方法（~20 行）
- 初始化和清理代码（~10 行）

---

#### 2. `jxt-core/sdk/pkg/eventbus/README.md`

**修改类型**: 更新

**修改内容**:

| 章节 | 修改类型 | 说明 |
|------|---------|------|
| 架构图 | ✏️ 修改 | 更新架构图，移除 Worker Pool |
| Subscribe() 说明 | ✏️ 修改 | 更新说明，说明现在也使用 Actor Pool |
| 性能对比 | ✏️ 修改 | 更新性能数据 |

**预计修改行数**: ~50 行

---

#### 3. `jxt-core/docs/kafka-subscribe-to-actor-pool-migration-summary.md`

**修改类型**: 新增

**修改内容**: 创建迁移总结文档

**预计行数**: ~50 行

---

### 🔍 **结构体字段变更**

#### `kafkaEventBus` 结构体

**删除字段**:
```go
type kafkaEventBus struct {
    // ... 其他字段 ...
    
    // ❌ 删除：全局 Worker Pool
    globalWorkerPool *GlobalWorkerPool
    
    // ... 其他字段 ...
}
```

**保留字段**:
```go
type kafkaEventBus struct {
    // ... 其他字段 ...

    // ✅ 保留：全局 Hollywood Actor Pool
    globalActorPool *HollywoodActorPool

    // ... 其他字段 ...
}
```

**新增字段**:
```go
type kafkaEventBus struct {
    // ... 其他字段 ...

    // ✅ 新增：轮询计数器（用于无聚合ID消息的负载均衡）
    roundRobinCounter atomic.Uint64

    // ... 其他字段 ...
}
```

---

## 详细实施步骤

### 步骤 1: 修改路由逻辑

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**位置**: Line 1154-1224（`processMessageWithKeyedPool` 方法）

**当前代码**:
```go
func (h *preSubscriptionConsumerHandler) processMessageWithKeyedPool(
    ctx context.Context,
    message *sarama.ConsumerMessage,
    handler MessageHandler,
    session sarama.ConsumerGroupSession,
) error {
    // 转换 Headers 为 map
    headersMap := make(map[string]string, len(message.Headers))
    for _, header := range message.Headers {
        headersMap[string(header.Key)] = string(header.Value)
    }

    // 尝试提取聚合ID（优先级：Envelope > Header > Kafka Key）
    aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")

    if aggregateID != "" {
        // 有聚合ID：使用 Hollywood Actor Pool 进行顺序处理
        pool := h.eventBus.globalActorPool
        if pool != nil {
            aggMsg := &AggregateMessage{
                Topic:       message.Topic,
                Partition:   message.Partition,
                Offset:      message.Offset,
                Key:         message.Key,
                Value:       message.Value,
                Headers:     make(map[string][]byte),
                Timestamp:   message.Timestamp,
                AggregateID: aggregateID,
                Context:     ctx,
                Done:        make(chan error, 1),
                Handler:     handler,
            }
            for _, header := range message.Headers {
                aggMsg.Headers[string(header.Key)] = header.Value
            }

            if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
                return err
            }

            select {
            case err := <-aggMsg.Done:
                session.MarkMessage(message, "")
                return err
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }

    // 无聚合ID：使用全局Worker池处理（迁移后将改为使用 Hollywood Actor Pool）
    workItem := WorkItem{
        Topic:   message.Topic,
        Message: message,
        Handler: handler,
        Session: session,
    }

    // 提交到全局Worker池
    if !h.eventBus.globalWorkerPool.SubmitWork(workItem) {
        h.eventBus.logger.Warn("Failed to submit work to global worker pool, using direct processing",
            zap.String("topic", message.Topic),
            zap.Int64("offset", message.Offset))
        // 如果Worker池满了，直接在当前goroutine处理
        h.processMessageDirectly(ctx, message, handler, session)
    }

    return nil
}
```

**修改后代码**:
```go
func (h *preSubscriptionConsumerHandler) processMessageWithKeyedPool(
    ctx context.Context,
    message *sarama.ConsumerMessage,
    handler MessageHandler,
    session sarama.ConsumerGroupSession,
) error {
    // 转换 Headers 为 map
    headersMap := make(map[string]string, len(message.Headers))
    for _, header := range message.Headers {
        headersMap[string(header.Key)] = string(header.Value)
    }

    // 尝试提取聚合ID（优先级：Envelope > Header > Kafka Key）
    aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")

    // ⭐ 核心变更：统一使用 Hollywood Actor Pool
    // 路由策略：
    // - 有聚合ID：使用 aggregateID 作为路由键（保持有序）
    // - 无聚合ID：使用 Round-Robin 轮询（保持并发）
    routingKey := aggregateID
    if routingKey == "" {
        // 使用轮询计数器生成路由键
        index := h.eventBus.roundRobinCounter.Add(1)
        routingKey = fmt.Sprintf("rr-%d", index)
    }

    pool := h.eventBus.globalActorPool
    if pool == nil {
        return fmt.Errorf("hollywood actor pool not initialized")
    }

    aggMsg := &AggregateMessage{
        Topic:       message.Topic,
        Partition:   message.Partition,
        Offset:      message.Offset,
        Key:         message.Key,
        Value:       message.Value,
        Headers:     make(map[string][]byte),
        Timestamp:   message.Timestamp,
        AggregateID: routingKey,  // ⭐ 使用 routingKey（可能是 aggregateID 或 topic）
        Context:     ctx,
        Done:        make(chan error, 1),
        Handler:     handler,
    }
    for _, header := range message.Headers {
        aggMsg.Headers[string(header.Key)] = header.Value
    }

    if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
        return err
    }

    select {
    case err := <-aggMsg.Done:
        session.MarkMessage(message, "")
        return err
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

**关键变更**:
1. ✅ 删除 `if aggregateID != ""` 分支判断
2. ✅ 添加 `routingKey` 逻辑：有聚合ID用聚合ID，无聚合ID用 topic
3. ✅ 统一使用 `globalActorPool.ProcessMessage()`
4. ✅ 删除全局 Worker Pool 相关代码

**预计修改行数**: ~30 行（删除 ~40 行，新增 ~10 行）

---

### 步骤 2: 删除 Worker Pool 代码

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**位置**: Line 33-218

**删除内容**:

1. **GlobalWorkerPool 结构体**（Line 33-44）
2. **Worker 结构体**（Line 46-52）
3. **NewGlobalWorkerPool 函数**（Line 54-76）
4. **start 方法**（Line 78-102）
5. **dispatcher 方法**（Line 104-134）
6. **SubmitWork 方法**（Line 136-150）
7. **Worker.start 方法**（Line 152-166）
8. **Worker.processWork 方法**（Line 168-192）
9. **GlobalWorkerPool.Close 方法**（Line 194-218）

**预计删除行数**: ~185 行

---

### 步骤 3: 删除 WorkItem 结构体和 processMessageDirectly 方法

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

**删除内容**:

1. **WorkItem 结构体**（搜索 `type WorkItem struct`）
   ```go
   // ❌ 删除
   type WorkItem struct {
       Topic   string
       Message *sarama.ConsumerMessage
       Handler MessageHandler
       Session sarama.ConsumerGroupSession
   }
   ```

2. **processMessageDirectly 方法**（Line 1225-1245）
   ```go
   // ❌ 删除
   func (h *preSubscriptionConsumerHandler) processMessageDirectly(
       ctx context.Context,
       message *sarama.ConsumerMessage,
       handler MessageHandler,
       session sarama.ConsumerGroupSession,
   ) {
       // ... 实现代码 ...
   }
   ```

**预计删除行数**: ~30 行

---

### 步骤 4: 清理初始化和关闭逻辑

**文件**: `jxt-core/sdk/pkg/eventbus/kafka.go`

#### 4.1 删除初始化代码

**位置**: Line 577-578

**当前代码**:
```go
// 创建全局Worker池
globalWorkerPool := NewGlobalWorkerPool(0, zap.NewNop()) // 0表示使用默认worker数量
```

**修改**: ❌ 删除这两行

---

**位置**: Line 592

**当前代码**:
```go
bus := &kafkaEventBus{
    config: cfg,
    logger: zap.NewNop(),
    
    // ...
    
    globalWorkerPool:       globalWorkerPool,  // ❌ 删除这一行
    
    // ...
}
```

**修改**: ❌ 删除 `globalWorkerPool` 字段赋值

---

#### 4.2 删除关闭代码

**位置**: Line 1959-1961

**当前代码**:
```go
// 停止全局Worker池
if k.globalWorkerPool != nil {
    k.globalWorkerPool.Close()
}
```

**修改**: ❌ 删除这三行

---

**预计删除行数**: ~10 行

---

### 步骤 5: 代码审查和优化

**检查清单**:

1. ✅ 确认所有 `globalWorkerPool` 引用已删除
2. ✅ 确认所有 `WorkItem` 引用已删除
3. ✅ 确认 `processMessageDirectly` 引用已删除
4. ✅ 确认路由逻辑正确（有/无聚合ID）
5. ✅ 确认错误处理完整
6. ✅ 确认日志记录完整
7. ✅ 运行 `go build` 确认编译通过
8. ✅ 运行 `go vet` 确认无警告
9. ✅ 运行 `golangci-lint` 确认代码质量

**工具命令**:
```bash
# 编译检查
cd jxt-core/sdk/pkg/eventbus
go build

# 静态分析
go vet ./...

# 代码质量检查（如果安装了 golangci-lint）
golangci-lint run
```

---

## 测试验证计划

### 🧪 **测试阶段**

#### 阶段 1: 单元测试（30 分钟）

**目标**: 验证基本功能正常

**测试用例**:
1. ✅ `TestKafkaBasicPublishSubscribe` - 基本发布订阅
2. ✅ `TestKafkaMultipleMessages` - 多消息测试
3. ✅ `TestKafkaEnvelopePublishSubscribe` - Envelope 发布订阅

**执行命令**:
```bash
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafka" -timeout 300s
```

**成功标准**:
- ✅ 所有测试通过
- ✅ 无 panic 或错误
- ✅ 消息正确接收和处理

---

#### 阶段 2: 性能测试（1 小时）

**目标**: 验证性能不低于当前实现

**测试用例**:
1. ✅ `TestMemoryVsPersistenceComparison` - 内存持久化性能对比
2. ✅ `TestKafkaVsNATSPerformanceComparison` - Kafka vs NATS 性能对比

**执行命令**:
```bash
cd jxt-core/tests/eventbus/performance_regression_tests
go test -v -run "TestMemoryVsPersistenceComparison" -timeout 600s
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 600s
```

**成功标准**:
- ✅ 吞吐量 ≥ 5000 msg/s（单 topic）
- ✅ 吞吐量 ≥ 6000 msg/s（多 topic）
- ✅ P50 延迟 ≤ 200 ms
- ✅ P99 延迟 ≤ 500 ms
- ✅ 内存占用 ≤ 当前实现的 120%

---

#### 阶段 3: 可靠性测试（30 分钟）

**目标**: 验证故障恢复机制

**测试用例**:
1. ✅ Handler panic 恢复测试
2. ✅ Actor 重启测试
3. ✅ Inbox 满载测试

**测试代码**（新增）:
```go
// TestKafkaActorPoolReliability 测试 Actor Pool 可靠性
func TestKafkaActorPoolReliability(t *testing.T) {
    helper := NewTestHelper(t)
    defer helper.Cleanup()
    
    topic := fmt.Sprintf("test.kafka.reliability.%d", helper.GetTimestamp())
    helper.CreateKafkaTopics([]string{topic}, 3)
    
    bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-reliability-%d", helper.GetTimestamp()))
    defer helper.CloseEventBus(bus)
    
    ctx := context.Background()
    
    var received int64
    var panicCount int64
    
    // Handler 会在前 3 次调用时 panic
    err := bus.Subscribe(ctx, topic, func(ctx context.Context, message []byte) error {
        count := atomic.AddInt64(&received, 1)
        if count <= 3 {
            atomic.AddInt64(&panicCount, 1)
            panic("simulated panic")
        }
        return nil
    })
    helper.AssertNoError(err, "Subscribe should not return error")
    
    time.Sleep(2 * time.Second)
    
    // 发送 10 条消息
    for i := 0; i < 10; i++ {
        err = bus.Publish(ctx, topic, []byte(fmt.Sprintf("message-%d", i)))
        helper.AssertNoError(err, "Publish should not return error")
    }
    
    time.Sleep(5 * time.Second)
    
    // 验证：前 3 次 panic，后 7 次成功
    helper.AssertEqual(int64(10), atomic.LoadInt64(&received), "Should receive all 10 messages")
    helper.AssertEqual(int64(3), atomic.LoadInt64(&panicCount), "Should panic 3 times")
    
    t.Logf("✅ Actor Pool reliability test passed")
}
```

**成功标准**:
- ✅ Handler panic 后 Actor 自动重启
- ✅ Actor 重启后继续处理消息
- ✅ 重启次数不超过 maxRestarts（3次）
- ✅ Inbox 满载时消息阻塞，不丢失

---

## 回滚方案

### 🔄 **回滚说明**

**项目未上生产环境，回滚风险较低**

如需回滚，使用 Git 回滚即可：

```bash
# 查看提交历史
git log --oneline

# 回滚到迁移前的提交
git revert <commit-hash>

# 或者硬回滚（谨慎使用）
git reset --hard <commit-hash>

# 验证回滚
cd jxt-core/tests/eventbus/function_regression_tests
go test -v -run "TestKafka" -timeout 300s
```

**说明**: 由于项目未上生产环境，无需复杂的回滚流程。如果迁移后发现问题，直接使用 Git 回滚即可

---

## 附录

### A. 代码修改 Diff 预览

```diff
--- a/jxt-core/sdk/pkg/eventbus/kafka.go
+++ b/jxt-core/sdk/pkg/eventbus/kafka.go
@@ -30,188 +30,0 @@
-// GlobalWorkerPool 全局Worker池
-type GlobalWorkerPool struct {
-    workers     []*Worker
-    workQueue   chan WorkItem
-    workerCount int
-    queueSize   int
-    ctx         context.Context
-    cancel      context.CancelFunc
-    wg          sync.WaitGroup
-    logger      *zap.Logger
-    closed      atomic.Bool
-}
-
-// Worker 全局Worker
-type Worker struct {
-    id       int
-    pool     *GlobalWorkerPool
-    workChan chan WorkItem
-    quit     chan bool
-}
-
-// ... (删除所有 Worker Pool 相关代码)
-
@@ -575,3 +387,0 @@
-    // 创建全局Worker池
-    globalWorkerPool := NewGlobalWorkerPool(0, zap.NewNop())
-
@@ -589,1 +388,0 @@
-        globalWorkerPool:       globalWorkerPool,
@@ -1151,74 +1149,45 @@
 func (h *preSubscriptionConsumerHandler) processMessageWithKeyedPool(
     ctx context.Context,
     message *sarama.ConsumerMessage,
     handler MessageHandler,
     session sarama.ConsumerGroupSession,
 ) error {
     // 转换 Headers 为 map
     headersMap := make(map[string]string, len(message.Headers))
     for _, header := range message.Headers {
         headersMap[string(header.Key)] = string(header.Value)
     }
 
     // 尝试提取聚合ID（优先级：Envelope > Header > Kafka Key）
     aggregateID, _ := ExtractAggregateID(message.Value, headersMap, message.Key, "")
 
-    if aggregateID != "" {
-        // 有聚合ID：使用 Hollywood Actor Pool 进行顺序处理
-        pool := h.eventBus.globalActorPool
-        if pool != nil {
-            aggMsg := &AggregateMessage{
-                // ... (省略字段)
-            }
-            // ... (省略处理逻辑)
-        }
+    // ⭐ 核心变更：统一使用 Hollywood Actor Pool
+    routingKey := aggregateID
+    if routingKey == "" {
+        routingKey = message.Topic
     }
 
-    // 无聚合ID：使用全局Worker池处理
-    workItem := WorkItem{
-        Topic:   message.Topic,
-        Message: message,
-        Handler: handler,
-        Session: session,
+    pool := h.eventBus.globalActorPool
+    if pool == nil {
+        return fmt.Errorf("hollywood actor pool not initialized")
     }
 
-    if !h.eventBus.globalWorkerPool.SubmitWork(workItem) {
-        h.eventBus.logger.Warn("Failed to submit work to global worker pool")
-        h.processMessageDirectly(ctx, message, handler, session)
+    aggMsg := &AggregateMessage{
+        Topic:       message.Topic,
+        Partition:   message.Partition,
+        Offset:      message.Offset,
+        Key:         message.Key,
+        Value:       message.Value,
+        Headers:     make(map[string][]byte),
+        Timestamp:   message.Timestamp,
+        AggregateID: routingKey,  // ⭐ 使用 routingKey
+        Context:     ctx,
+        Done:        make(chan error, 1),
+        Handler:     handler,
     }
+    for _, header := range message.Headers {
+        aggMsg.Headers[string(header.Key)] = header.Value
+    }
+
+    if err := pool.ProcessMessage(ctx, aggMsg); err != nil {
+        return err
+    }
+
+    select {
+    case err := <-aggMsg.Done:
+        session.MarkMessage(message, "")
+        return err
+    case <-ctx.Done():
+        return ctx.Err()
+    }
-
-    return nil
 }
@@ -1956,4 +1925,0 @@
-    // 停止全局Worker池
-    if k.globalWorkerPool != nil {
-        k.globalWorkerPool.Close()
-    }
```

---

**文档状态**: 待评审  
**下一步**: 等待评审批准后，开始代码实施

