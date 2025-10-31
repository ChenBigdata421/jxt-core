# Hollywood Actor Pool 迁移快速参考

> **重要**: 本方案使用 **固定 Actor Pool**,而非一个聚合ID一个Actor,适合千万级聚合ID场景。

## 🚀 **快速开始**

### 1. 安装依赖

```bash
cd jxt-core
go get github.com/anthdm/hollywood@latest
go mod tidy
```

### 2. 最小配置

```yaml
# config.yaml
eventbus:
  type: kafka
  kafka:
    brokers: ["localhost:9092"]

  # 启用 Hollywood Actor Pool
  useHollywood: true

  hollywood:
    poolSize: 256        # 固定256个Actor (可选: 512/1024)
    inboxSize: 1000      # 每个Actor的Inbox大小
    maxRestarts: 3       # Actor最大重启次数
    enableEventStream: true
```

### 3. 代码示例

```go
package main

import (
    "context"
    "fmt"

    "jxt-core/sdk/pkg/eventbus"
)

func main() {
    // 创建 EventBus (自动启用 Hollywood Actor Pool)
    config := &eventbus.EventBusConfig{
        Type: "kafka",
        Kafka: &eventbus.KafkaConfig{
            Brokers: []string{"localhost:9092"},
        },
        UseHollywood: true,
        Hollywood: &eventbus.HollywoodActorPoolConfig{
            PoolSize:          256,  // 固定256个Actor
            InboxSize:         1000,
            MaxRestarts:       3,
            EnableEventStream: true,
        },
    }

    bus, err := eventbus.NewEventBus(config)
    if err != nil {
        panic(err)
    }
    defer bus.Close()

    // 订阅主题
    err = bus.Subscribe("order-events", func(ctx context.Context, data []byte) error {
        fmt.Printf("Received: %s\n", string(data))
        return nil
    })

    // 发布消息 (带聚合ID)
    envelope := &eventbus.Envelope{
        AggregateID: "order-123",  // 千万级聚合ID,哈希到256个Actor之一
        EventType:   "OrderCreated",
        EventData:   []byte(`{"orderId":"123","amount":100}`),
    }

    err = bus.Publish("order-events", envelope)
    if err != nil {
        panic(err)
    }
}
```

---

## 📊 **架构对比**

### Keyed Worker Pool (旧)

```
消息 → Hash(aggregateID) % 256 → Worker[i] → 处理
                                   ↓
                          256个固定Worker (goroutine + channel)
                          问题: 无Supervisor,无事件流监控 ❌
```

### Hollywood Actor Pool (新)

```
消息 → Hash(aggregateID) % 256 → Actor[i] → 处理
                                   ↓
                          256个固定Actor (Hollywood管理)
                          优势: Supervisor + 事件流 + 消息保证 ✅
```

**关键**: 两者都是固定Pool,但Hollywood提供了更好的可靠性和可观测性!

---

## 🔑 **核心概念**

### Actor Pool 架构

```
固定256个Actor (启动时创建,运行期间不变)
  ↓
Hash(aggregateID) % 256 → 路由到对应Actor
  ↓
同一聚合ID总是路由到同一Actor (保证顺序)
  ↓
不同聚合ID可能路由到同一Actor (共享,存在头部阻塞可能)
```

### 消息处理流程

```
1. 消息到达 EventBus
2. 提取 AggregateID
3. Hash(aggregateID) % 256 → actorIndex
4. 发送消息到 Actor[actorIndex] 的 Inbox
5. Actor 串行处理消息
6. 返回处理结果
```

### Supervisor 机制 (关键优势)

```
Actor 处理失败 (panic)
        ↓
Hollywood Supervisor 捕获错误
        ↓
自动重启 Actor (最多 maxRestarts 次)
        ↓
Actor 恢复正常,继续处理后续消息
        ↓
其他255个Actor完全不受影响 ✅
```

---

## ⚙️ **配置参数详解**

### poolSize
- **含义**: Actor Pool 的大小 (固定数量)
- **推荐值**: 256 (可选: 512, 1024)
- **说明**:
  - 256: 适合中等规模 (千万级聚合ID)
  - 512: 适合大规模 (亿级聚合ID)
  - 1024: 适合超大规模
  - **注意**: 启动后不可动态调整

### maxRestarts
- **含义**: Actor 最大重启次数
- **推荐值**: 3
- **说明**: 超过此次数后,Actor 将被停止,消息发送到 DLQ

### inboxSize (Mailbox 深度) ⭐ 关键参数

- **含义**: 每个 Actor 的 Mailbox (Inbox) 队列深度
- **推荐值**: 1000 (适合 80% 场景)
- **取值范围**: 100 ~ 10000

#### 详细说明

**太小 (< 500)**:
- ❌ 背压频繁,吞吐量受限
- ❌ 无法应对突发流量
- ✅ 延迟低 (消息排队少)
- ✅ 内存占用小
- **适用**: 低延迟优先场景 (实时交易)

**适中 (500-2000)**:
- ✅ 平衡吞吐量和延迟
- ✅ 可应对中等突发流量
- ✅ 内存占用可控
- **适用**: 通用场景 ✅ 推荐

**太大 (> 5000)**:
- ✅ 吞吐量高
- ✅ 可应对大突发流量
- ❌ 延迟高 (消息排队多)
- ❌ 内存占用大
- ❌ 故障恢复慢 (大量消息积压)
- **适用**: 高吞吐场景 (日志聚合)

#### 内存计算公式

```
总内存 = poolSize × inboxSize × avgMessageSize

示例:
- 256 × 1000 × 200B = 51.2MB   (标准配置)
- 256 × 5000 × 200B = 256MB    (高吞吐)
- 512 × 10000 × 500B = 2.5GB   (极端场景)
```

#### 性能影响

| inboxSize | 吞吐量 | P99延迟 | 内存 | 背压频率 |
|-----------|--------|---------|------|----------|
| 100 | 50K TPS | 10-20ms | ~5MB | 高 |
| 500 | 80K TPS | 20-30ms | ~25MB | 中 |
| 1000 | 100K TPS | 40-50ms | ~50MB | 低 ✅ |
| 5000 | 200K TPS | 80-100ms | ~250MB | 极低 |
| 10000 | 300K TPS | 150-200ms | ~500MB | 无 |

#### 调优建议

**如何选择 inboxSize?**

1. **测量平均消息处理时间** (avgProcessTime)
2. **测量消息到达速率** (msgRate)
3. **计算所需缓冲深度**:
   ```
   inboxSize = msgRate × avgProcessTime × safetyFactor

   示例:
   - msgRate = 1000 msg/s (每个 Actor)
   - avgProcessTime = 10ms = 0.01s
   - safetyFactor = 2 (应对突发)
   → inboxSize = 1000 × 0.01 × 2 = 20

   但考虑突发流量,建议至少 100-500
   ```

4. **监控调整**:
   - 如果 Inbox 利用率 > 80% → 增加 inboxSize
   - 如果 P99 延迟过高 + Inbox 深度大 → 减小 inboxSize
   - 如果背压频繁 → 增加 inboxSize 或 poolSize

### enableEventStream
- **含义**: 是否启用事件流监控
- **推荐值**: true
- **说明**:
  - 启用后可监控 DeadLetter, ActorRestarted 事件
  - 生产环境强烈推荐启用

---

## 📊 **监控集成 (混合方案)** ⭐⭐⭐⭐⭐

### 推荐方案: 混合方案 (接口注入 + Middleware)

**核心思想**:
- **接口定义**: 使用接口注入 (与 EventBus 一致,依赖倒置)
- **Middleware 实现**: Middleware 依赖接口,自动记录消息处理
- **手动调用**: 特殊场景手动调用接口

**优势**:
- ✅ 与 EventBus 监控架构保持一致
- ✅ 自动记录消息处理 (Middleware 自动拦截)
- ✅ 灵活性高 (特殊场景可手动调用)

**详细实现**: 参考 `hollywood-actor-pool-prometheus-integration.md`

### 快速集成 (5分钟)

#### 1. 创建 Prometheus 收集器

```go
// 创建 Prometheus 收集器 (实现 ActorPoolMetricsCollector 接口)
metricsCollector := eventbus.NewPrometheusActorPoolMetricsCollector("my_service")
```

#### 2. 注入到 Actor Pool

```go
config := &eventbus.HollywoodActorPoolConfig{
    PoolSize:          256,
    InboxSize:         1000,
    MaxRestarts:       3,
    EnableEventStream: true,
    MetricsCollector:  metricsCollector, // ⭐ 接口注入 (与 EventBus 一致)
}

pool, _ := eventbus.NewHollywoodActorPool(config)

type ActorMetrics struct {
    msgCounter prometheus.Counter
    msgLatency prometheus.Histogram
}

func NewActorMetrics(actorID string) *ActorMetrics {
    return &ActorMetrics{
        msgCounter: promauto.NewCounter(prometheus.CounterOpts{
            Name: "hollywood_actor_msg_total",
            ConstLabels: prometheus.Labels{"actor_id": actorID},
        }),
        msgLatency: promauto.NewHistogram(prometheus.HistogramOpts{
            Name:    "hollywood_actor_msg_latency_seconds",
            Buckets: []float64{0.001, 0.01, 0.1, 1.0},
            ConstLabels: prometheus.Labels{"actor_id": actorID},
        }),
    }
}

func (am *ActorMetrics) WithMetrics() func(actor.ReceiveFunc) actor.ReceiveFunc {
    return func(next actor.ReceiveFunc) actor.ReceiveFunc {
        return func(c *actor.Context) {
            start := time.Now()
            am.msgCounter.Inc()
            next(c)
            am.msgLatency.Observe(time.Since(start).Seconds())
        }
    }
}
```

#### 3. 注入 Middleware

```go
metrics := NewActorMetrics("pool-actor-0")

pid := engine.Spawn(
    newPoolActor,
    "pool-actor-0",
    actor.WithMiddleware(metrics.WithMetrics()), // ⭐ 注入监控
)
```

#### 4. 暴露 Prometheus Endpoint

```go
import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

go func() {
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(":2112", nil)
}()
```

#### 5. 访问监控指标

```bash
curl http://localhost:2112/metrics

# 输出:
# hollywood_actor_msg_total{actor_id="pool-actor-0"} 12345
# hollywood_actor_msg_latency_seconds_bucket{actor_id="pool-actor-0",le="0.001"} 100
```

### 推荐的监控指标

| 指标名称 | 类型 | 说明 |
|---------|------|------|
| `hollywood_actor_msg_total` | Counter | 消息总数 |
| `hollywood_actor_msg_latency_seconds` | Histogram | 消息延迟 |
| `hollywood_actor_inbox_depth` | Gauge | Mailbox 深度 |
| `hollywood_actor_inbox_utilization` | Gauge | Mailbox 利用率 |

### Grafana 查询示例

```promql
# 吞吐量 (每秒)
rate(hollywood_actor_msg_total[1m])

# P99 延迟
histogram_quantile(0.99, rate(hollywood_actor_msg_latency_seconds_bucket[5m]))

# 平均 Inbox 深度
avg(hollywood_actor_inbox_depth)
```

---

## 📈 **性能调优**

### 场景 1: 高吞吐量 (大规模聚合)

```yaml
hollywood:
  poolSize: 1024          # 更多Actor
  inboxSize: 10000        # 大缓冲区
  maxRestarts: 3
  enableEventStream: true

# 内存占用: 1024 * 10000 * 200B ≈ 2GB
```

### 场景 2: 低延迟 (快速响应)

```yaml
hollywood:
  poolSize: 256           # 标准Actor数量
  inboxSize: 100          # 小缓冲区,减少排队
  maxRestarts: 3
  enableEventStream: true

# 内存占用: 256 * 100 * 200B ≈ 5MB
```

### 场景 3: 内存受限 (资源紧张)

```yaml
hollywood:
  poolSize: 128           # 较少Actor
  inboxSize: 500          # 中等缓冲区
  maxRestarts: 3
  enableEventStream: true

# 内存占用: 128 * 500 * 200B ≈ 13MB
```

---

## 🔍 **监控指标**

### 关键指标

| 指标 | 说明 | 告警阈值 |
|------|------|---------|
| `hollywood_actors_active` | 活跃 Actor 数量 | > 100K |
| `hollywood_actors_restarted_total` | Actor 重启次数 | > 10/s |
| `hollywood_messages_processed_total` | 消息处理总数 | - |
| `hollywood_processing_latency_seconds` | 处理延迟 | P99 > 100ms |

### Prometheus 查询示例

```promql
# Actor 重启率
rate(hollywood_actors_restarted_total[5m])

# P99 延迟
histogram_quantile(0.99, hollywood_processing_latency_seconds)

# 活跃 Actor 数量
hollywood_actors_active
```

---

## 🐛 **常见问题**

### Q1: Actor 频繁重启怎么办?

**原因**: 业务处理逻辑 panic

**解决**:
1. 检查业务代码,添加 recover
2. 增加 maxRestarts
3. 实现自定义 Supervisor 策略

```go
// 业务代码添加 recover
func handler(ctx context.Context, data []byte) error {
    defer func() {
        if r := recover(); r != nil {
            log.Errorf("panic recovered: %v", r)
        }
    }()
    
    // 业务逻辑
    return nil
}
```

### Q2: 内存占用过高怎么办?

**原因**: Mailbox 配置过大

**诊断**:
```bash
# 计算理论内存占用
总内存 = poolSize × inboxSize × avgMessageSize

# 示例: 如果内存占用 2GB
# 可能配置: 1024 × 10000 × 200B = 2GB
```

**解决**:
1. **减小 inboxSize** (最直接)
   ```yaml
   hollywood:
     inboxSize: 1000  # 从 10000 减小到 1000
   # 内存: 2GB → 200MB
   ```

2. **减小 poolSize** (如果吞吐量允许)
   ```yaml
   hollywood:
     poolSize: 256    # 从 1024 减小到 256
   # 内存: 2GB → 500MB
   ```

3. **优化消息大小** (压缩、精简字段)
   ```go
   // 使用 Protocol Buffers 或 MessagePack 压缩
   // avgMessageSize: 500B → 200B
   ```

### Q3: 消息处理延迟高怎么办?

**原因**: Mailbox 积压导致排队延迟

**诊断**:
```bash
# 检查 Mailbox 深度
avg_inbox_depth = 8000  # 如果接近 inboxSize,说明积压严重

# 检查延迟来源
总延迟 = 排队延迟 + 处理延迟
排队延迟 ≈ avg_inbox_depth × avgProcessTime
```

**解决**:

**情况 1: Mailbox 深度大 (> 5000)**
```yaml
# 减小 inboxSize,降低排队延迟
hollywood:
  poolSize: 512        # 增加 Actor 数量,分散负载
  inboxSize: 1000      # 减小 Mailbox
# 延迟: 200ms → 40ms
```

**情况 2: 业务处理慢**
```go
// 优化业务逻辑
func handler(ctx context.Context, data []byte) error {
    // 1. 添加超时控制
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    // 2. 异步处理非关键逻辑
    go asyncNotify(data)

    // 3. 批量处理
    return batchProcess(ctx, data)
}
```

**情况 3: 突发流量**
```yaml
# 增加 inboxSize 缓冲突发
hollywood:
  inboxSize: 5000      # 从 1000 增加到 5000
# 但会增加延迟,需权衡
```

### Q4: 如何保证消息顺序?

**答案**: Hollywood 自动保证同一 Actor 的消息顺序

- 同一 aggregateID 的消息 → 同一 Actor → 串行处理 ✅
- 不同 aggregateID 的消息 → 不同 Actor → 并发处理 ✅

### Q5: 如何回滚到 Keyed Worker Pool?

**步骤**:
1. 修改配置: `useHollywood: false`
2. 重启服务
3. 监控指标确认切换成功

```yaml
eventbus:
  useHollywood: false  # 关闭 Hollywood
  
  # 使用原有 Keyed Worker Pool
  keyedWorkerPool:
    workerCount: 256
    queueSize: 1000
```

---

## 🧪 **测试建议**

### 单元测试

```go
func TestAggregateActor_Ordering(t *testing.T) {
    // 测试同一聚合的消息顺序
    // 验证: 消息按发送顺序处理
}

func TestAggregateActor_Passivation(t *testing.T) {
    // 测试 Actor 空闲回收
    // 验证: 超时后 Actor 被回收
}

func TestAggregateActor_Restart(t *testing.T) {
    // 测试 Actor 重启机制
    // 验证: panic 后 Actor 自动重启
}
```

### 集成测试

```go
func TestKafkaEventBus_Hollywood(t *testing.T) {
    // 测试 Kafka + Hollywood 集成
    // 验证: 端到端消息处理
}
```

### 性能测试

```go
func BenchmarkHollywood_Throughput(b *testing.B) {
    // 测试吞吐量
    // 目标: > 350K TPS
}

func BenchmarkHollywood_Latency(b *testing.B) {
    // 测试延迟
    // 目标: P99 < 20ms
}
```

---

## 📚 **代码示例**

### 示例 1: 订单聚合

```go
// 订单事件处理
func handleOrderEvent(ctx context.Context, data []byte) error {
    var event OrderEvent
    if err := json.Unmarshal(data, &event); err != nil {
        return err
    }
    
    switch event.Type {
    case "OrderCreated":
        return handleOrderCreated(ctx, event)
    case "OrderPaid":
        return handleOrderPaid(ctx, event)
    case "OrderShipped":
        return handleOrderShipped(ctx, event)
    default:
        return fmt.Errorf("unknown event type: %s", event.Type)
    }
}

// 订阅订单事件
bus.Subscribe("order-events", handleOrderEvent)
```

### 示例 2: 用户聚合

```go
// 用户事件处理
func handleUserEvent(ctx context.Context, data []byte) error {
    var event UserEvent
    if err := json.Unmarshal(data, &event); err != nil {
        return err
    }
    
    // 更新用户状态
    return userService.UpdateState(ctx, event)
}

// 订阅用户事件
bus.Subscribe("user-events", handleUserEvent)
```

### 示例 3: 自定义监控

```go
// 自定义事件流监听
engine.Subscribe(actor.DeadLetterEvent{}, monitorPID)
engine.Subscribe(actor.ActorRestartedEvent{}, monitorPID)

// 监控 Actor
type MonitorActor struct{}

func (m *MonitorActor) Receive(ctx *actor.Context) {
    switch msg := ctx.Message().(type) {
    case actor.DeadLetterEvent:
        log.Warnf("Dead letter: %+v", msg)
        
    case actor.ActorRestartedEvent:
        log.Warnf("Actor restarted: %+v", msg)
    }
}
```

---

## ✅ **迁移检查清单**

### 开发阶段
- [ ] 安装 Hollywood 依赖
- [ ] 实现核心组件 (Engine, Actor, Registry)
- [ ] 编写单元测试 (覆盖率 > 80%)
- [ ] 编写集成测试
- [ ] 性能测试达标 (吞吐量 > 350K TPS)

### 测试阶段
- [ ] 功能测试通过
- [ ] 性能测试通过
- [ ] 压力测试通过
- [ ] 故障注入测试通过

### 上线阶段
- [ ] 配置特性开关
- [ ] 10% 灰度发布
- [ ] 50% 灰度发布
- [ ] 100% 全量发布
- [ ] 监控告警配置完成

### 运维阶段
- [ ] 文档更新完成
- [ ] 团队培训完成
- [ ] 应急预案准备
- [ ] 回滚方案验证

---

## 🎯 **关键收益**

| 维度 | 改进 |
|------|------|
| **吞吐量** | +250% (100K → 350K TPS) |
| **延迟** | -60% (50ms → 20ms P99) |
| **可靠性** | 更好的故障隔离 (Actor 级, 非完美) + 消息保证 |
| **可观测性** | 事件流监控 + 丰富指标 |
| **资源利用** | 固定池,资源可控 |

---

## 📞 **获取帮助**

- **Hollywood 官方文档**: https://github.com/anthdm/hollywood
- **jxt-core 详细迁移指南**: `docs/hollywood-actor-migration-guide.md`
- **示例代码**: `examples/hollywood_example.go`

