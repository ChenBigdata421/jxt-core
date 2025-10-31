# Hollywood Actor Pool Mailbox 深度调优指南

> **核心问题**: 如何选择合适的 Mailbox (Inbox) 深度,平衡吞吐量、延迟和内存占用?

---

## 🎯 **Mailbox 深度的本质**

### 什么是 Mailbox?

```
Mailbox (Inbox) = Actor 的消息队列

┌─────────────────────────────────────┐
│         Actor                       │
│  ┌──────────────────────────────┐   │
│  │  Mailbox (Inbox)             │   │
│  │  ┌────┬────┬────┬────┬────┐  │   │
│  │  │Msg1│Msg2│Msg3│... │MsgN│  │   │
│  │  └────┴────┴────┴────┴────┘  │   │
│  │  Capacity: inboxSize         │   │
│  └──────────────────────────────┘   │
│           ↓                         │
│  ┌──────────────────────────────┐   │
│  │  Message Processing          │   │
│  │  (串行处理)                   │   │
│  └──────────────────────────────┘   │
└─────────────────────────────────────┘

特点:
- 消息按 FIFO 顺序处理
- 队列满时触发背压 (阻塞发送方)
- 队列深度直接影响延迟和内存
```

---

## 📊 **Mailbox 深度的三角权衡**

```
         吞吐量
           ↑
           │
           │  inboxSize 增大
           │  ↗
           │
           │
           └──────────────→ 延迟
          ↙
    内存占用

核心矛盾:
- 增大 inboxSize → 吞吐量↑, 延迟↑, 内存↑
- 减小 inboxSize → 吞吐量↓, 延迟↓, 内存↓
```

---

## 🔢 **Mailbox 深度计算公式**

### 理论最小值

```
minInboxSize = msgRate × avgProcessTime × safetyFactor

参数说明:
- msgRate: 每个 Actor 的消息到达速率 (msg/s)
- avgProcessTime: 平均消息处理时间 (s)
- safetyFactor: 安全系数 (1.5-3.0,应对突发)

示例 1: 订单系统
- msgRate = 100 msg/s (每个 Actor)
- avgProcessTime = 10ms = 0.01s
- safetyFactor = 2
→ minInboxSize = 100 × 0.01 × 2 = 2

但考虑突发流量,建议至少 100-500

示例 2: 日志聚合
- msgRate = 10000 msg/s (每个 Actor)
- avgProcessTime = 5ms = 0.005s
- safetyFactor = 3
→ minInboxSize = 10000 × 0.005 × 3 = 150

考虑突发,建议 1000-5000
```

### 内存占用计算

```
totalMemory = poolSize × inboxSize × avgMessageSize

示例:
- poolSize = 256
- inboxSize = 1000
- avgMessageSize = 200B
→ totalMemory = 256 × 1000 × 200B = 51.2MB

- poolSize = 1024
- inboxSize = 10000
- avgMessageSize = 500B
→ totalMemory = 1024 × 10000 × 500B = 5GB ⚠️ 过大!
```

### 延迟估算

```
avgQueuingDelay = (avgInboxDepth / 2) × avgProcessTime

示例:
- avgInboxDepth = 5000 (Mailbox 平均深度)
- avgProcessTime = 10ms
→ avgQueuingDelay = (5000 / 2) × 10ms = 25s ⚠️ 过高!

- avgInboxDepth = 100
- avgProcessTime = 10ms
→ avgQueuingDelay = (100 / 2) × 10ms = 0.5s ✅ 可接受
```

---

## 📈 **性能影响矩阵**

### 不同 inboxSize 的性能对比

| inboxSize | 吞吐量 | P50延迟 | P99延迟 | 内存 (256 pool) | 背压频率 | 推荐场景 |
|-----------|--------|---------|---------|----------------|----------|----------|
| **50** | 30K TPS | 5ms | 10ms | ~2.5MB | 极高 | 极低延迟 |
| **100** | 50K TPS | 8ms | 15ms | ~5MB | 高 | 低延迟优先 |
| **500** | 80K TPS | 15ms | 30ms | ~25MB | 中 | 均衡场景 |
| **1000** | 100K TPS | 25ms | 50ms | ~50MB | 低 | 标准配置 ✅ |
| **2000** | 150K TPS | 40ms | 80ms | ~100MB | 极低 | 高吞吐 |
| **5000** | 200K TPS | 80ms | 150ms | ~250MB | 无 | 极高吞吐 |
| **10000** | 300K TPS | 150ms | 300ms | ~500MB | 无 | 批处理 |

**关键洞察**:
- ✅ **inboxSize=1000**: 最佳平衡点,适合 80% 场景
- ⚠️ **inboxSize > 5000**: 延迟显著增加,需谨慎评估
- ⚠️ **inboxSize < 500**: 背压频繁,可能影响吞吐量

---

## 🎯 **选择决策树**

```
开始: 确定业务需求
  ↓
优先级是什么?
  ├─ 低延迟 (P99 < 50ms)
  │   ↓
  │   消息处理速度?
  │   ├─ 快 (< 10ms) → inboxSize = 100-500
  │   └─ 慢 (> 10ms) → 优化业务逻辑,然后 inboxSize = 500-1000
  │
  ├─ 高吞吐 (> 200K TPS)
  │   ↓
  │   内存限制?
  │   ├─ 充足 (> 1GB) → inboxSize = 5000-10000
  │   └─ 受限 (< 500MB) → 增加 poolSize,inboxSize = 1000-2000
  │
  └─ 均衡 (通用场景)
      ↓
      inboxSize = 1000 ✅ 推荐
```

---

## 🔍 **监控与诊断**

### 关键监控指标

```go
// 1. Mailbox 深度
inbox_depth{actor_id="pool-actor-0"} = 850

// 2. Mailbox 利用率
inbox_utilization = inbox_depth / inbox_size = 850 / 1000 = 0.85

// 3. Mailbox 满载次数
inbox_full_total = 120

// 4. 背压频率
backpressure_rate = rate(inbox_full_total[5m]) = 10/min
```

### 诊断流程

#### 问题 1: 延迟过高

```
症状: P99 延迟 > 100ms

诊断步骤:
1. 检查 avg(inbox_depth)
   - 如果 > 5000 → Mailbox 积压严重
   - 如果 < 1000 → 业务处理慢

2. 检查 avgProcessTime
   - 如果 > 50ms → 优化业务逻辑
   - 如果 < 10ms → 检查 Mailbox 配置

解决方案:
- 情况 A: inbox_depth 大 + avgProcessTime 小
  → 减小 inboxSize (10000 → 1000)
  → 增加 poolSize (256 → 512)

- 情况 B: inbox_depth 小 + avgProcessTime 大
  → 优化业务逻辑 (数据库查询、网络调用)
  → 添加缓存、异步处理
```

#### 问题 2: 背压频繁

```
症状: rate(inbox_full_total[5m]) > 10

诊断步骤:
1. 检查 inbox_utilization
   - 如果 > 0.9 → Mailbox 太小
   - 如果 < 0.5 → 瞬时突发流量

2. 检查消息到达速率
   - 如果持续高 → 增加 inboxSize 或 poolSize
   - 如果间歇性 → 增加 inboxSize 缓冲突发

解决方案:
- 持续高负载:
  → 增加 poolSize (256 → 512)
  → 保持 inboxSize = 1000

- 突发流量:
  → 增加 inboxSize (1000 → 5000)
  → 接受延迟增加
```

#### 问题 3: 内存占用过高

```
症状: 内存占用 > 1GB

诊断步骤:
1. 计算理论内存
   totalMemory = poolSize × inboxSize × avgMessageSize

2. 检查配置
   - poolSize = ?
   - inboxSize = ?
   - avgMessageSize = ?

解决方案:
- 减小 inboxSize:
  1024 × 10000 × 200B = 2GB
  → 1024 × 1000 × 200B = 200MB ✅

- 减小 poolSize:
  1024 × 5000 × 200B = 1GB
  → 512 × 5000 × 200B = 500MB ✅

- 压缩消息:
  256 × 5000 × 500B = 640MB
  → 256 × 5000 × 200B = 256MB ✅
```

---

## 🛠️ **实战调优案例**

### 案例 1: 订单系统 (低延迟优先)

**初始配置**:
```yaml
hollywood:
  poolSize: 256
  inboxSize: 10000
```

**问题**:
- P99 延迟: 200ms (目标: < 50ms)
- 平均 Inbox 深度: 8000
- 内存占用: 512MB

**分析**:
```
avgQueuingDelay = (8000 / 2) × 10ms = 40s ⚠️
总延迟 = 40s (排队) + 10ms (处理) ≈ 40s
```

**解决方案**:
```yaml
hollywood:
  poolSize: 512        # 增加 Actor,分散负载
  inboxSize: 500       # 减小 Mailbox,降低排队
```

**结果**:
- P99 延迟: 200ms → 30ms ✅
- 平均 Inbox 深度: 8000 → 200
- 内存占用: 512MB → 50MB ✅

---

### 案例 2: 日志聚合 (高吞吐优先)

**初始配置**:
```yaml
hollywood:
  poolSize: 256
  inboxSize: 1000
```

**问题**:
- 背压告警: 50/min
- Inbox 利用率: 95%
- 吞吐量: 100K TPS (目标: 300K TPS)

**分析**:
```
Mailbox 太小,无法缓冲突发流量
背压导致上游阻塞,吞吐量受限
```

**解决方案**:
```yaml
hollywood:
  poolSize: 512        # 增加 Actor
  inboxSize: 10000     # 增加 Mailbox 缓冲
```

**结果**:
- 背压告警: 50/min → 0 ✅
- Inbox 利用率: 95% → 60% ✅
- 吞吐量: 100K TPS → 350K TPS ✅
- 延迟: P99 50ms → 120ms (可接受)

---

### 案例 3: 物联网数据 (内存受限)

**初始配置**:
```yaml
hollywood:
  poolSize: 1024
  inboxSize: 10000
```

**问题**:
- 内存占用: 2GB (限制: 500MB)
- 消息大小: 200B

**分析**:
```
totalMemory = 1024 × 10000 × 200B = 2GB
需要减少到 500MB
```

**解决方案 1: 减小 inboxSize**
```yaml
hollywood:
  poolSize: 1024
  inboxSize: 2500      # 2GB → 500MB
```

**解决方案 2: 减小 poolSize**
```yaml
hollywood:
  poolSize: 256
  inboxSize: 10000     # 2GB → 500MB
```

**选择**: 方案 1 (保持 poolSize,减小 inboxSize)
- 吞吐量: 300K TPS → 200K TPS (仍满足需求)
- 内存: 2GB → 500MB ✅

---

## ✅ **最佳实践总结**

### 推荐配置

| 场景 | poolSize | inboxSize | 预期性能 |
|------|----------|-----------|----------|
| **低延迟** (实时交易) | 256-512 | 100-500 | P99 < 30ms, 50-80K TPS |
| **标准均衡** (通用) | 256 | 1000 | P99 < 50ms, 100K TPS ✅ |
| **高吞吐** (日志聚合) | 512-1024 | 5000-10000 | P99 < 150ms, 200-300K TPS |
| **内存受限** (边缘计算) | 128-256 | 200-500 | P99 < 30ms, 30-50K TPS |

### 调优原则

1. **从标准配置开始**: poolSize=256, inboxSize=1000
2. **监控关键指标**: inbox_utilization, latency_p99, backpressure_rate
3. **根据监控调整**:
   - inbox_utilization > 0.8 → 增加 inboxSize 或 poolSize
   - latency_p99 过高 + inbox_depth 大 → 减小 inboxSize
   - backpressure_rate 高 → 增加 inboxSize 或 poolSize
4. **迭代优化**: 小步调整,观察效果,避免大幅变动

### 避免的陷阱

- ❌ **盲目增大 inboxSize**: 延迟会显著增加
- ❌ **过小 inboxSize**: 背压频繁,吞吐量受限
- ❌ **忽略内存占用**: 可能导致 OOM
- ❌ **不监控 Mailbox 深度**: 无法发现问题根源

---

## 📚 **参考资料**

- [Hollywood Actor Pool 迁移指南](./hollywood-actor-pool-migration-guide.md)
- [Hollywood 快速参考](./hollywood-quick-reference.md)
- [Akka Mailbox Configuration](https://doc.akka.io/docs/akka/current/mailboxes.html)
- [Erlang Process Mailbox](https://www.erlang.org/doc/efficiency_guide/processes.html)

---

**最后更新**: 2025-10-29
**维护者**: jxt-core Team

