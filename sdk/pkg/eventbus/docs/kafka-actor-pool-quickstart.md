# Kafka EventBus Hollywood Actor Pool - 快速启动指南

> **5 分钟快速上手 Kafka EventBus 的 Hollywood Actor Pool 迁移**

---

## 🚀 **快速启动**

### ⭐ 无需任何配置！

Hollywood Actor Pool 已经**完全替换** Keyed Worker Pool，直接运行即可。

### 步骤 1: 运行应用 (1 分钟)

```bash
# 启动应用
go run main.go

# 查看日志，确认使用 Actor Pool
# 应该看到: "Kafka EventBus using Hollywood Actor Pool"
```

### 步骤 2: 验证监控指标 (2 分钟)

```bash
# 访问 Prometheus 指标端点
curl http://localhost:2112/metrics | grep kafka_eventbus_actor_pool

# 应该看到以下指标:
# kafka_eventbus_actor_pool_messages_sent_total
# kafka_eventbus_actor_pool_messages_processed_total
# kafka_eventbus_actor_pool_message_latency_seconds
# kafka_eventbus_actor_pool_inbox_depth
# kafka_eventbus_actor_pool_actor_restarted_total
```

### 步骤 3: 发送测试消息 (1 分钟)

```go
// 发送带聚合ID的消息
envelope := &Envelope{
    AggregateID: "order-123",
    EventType:   "OrderCreated",
    Payload:     []byte(`{"orderId": "order-123", "amount": 100}`),
}

err := eventBus.PublishEnvelope(ctx, "orders", envelope)
```

---

## 📊 **监控 Dashboard**

### Grafana 查询示例

```promql
# 1. 消息吞吐量 (每秒)
rate(kafka_eventbus_actor_pool_messages_sent_total[1m])

# 2. P99 延迟
histogram_quantile(0.99, 
  rate(kafka_eventbus_actor_pool_message_latency_seconds_bucket[1m])
)

# 3. Actor 重启率 (每 5 分钟)
rate(kafka_eventbus_actor_pool_actor_restarted_total[5m])

# 4. Inbox 利用率 (0-1)
kafka_eventbus_actor_pool_inbox_utilization

# 5. 成功率
sum(rate(kafka_eventbus_actor_pool_messages_processed_total{success="true"}[1m]))
/
sum(rate(kafka_eventbus_actor_pool_messages_processed_total[1m]))
```

---

## 🔄 **回滚方案**

### ⚠️ 注意：已完全替换 Keyed Pool

Hollywood Actor Pool 已经**完全替换** Keyed Worker Pool，无法回滚到 Keyed Pool。

如果需要回滚，请：
1. 回退到之前的代码版本
2. 重新部署应用

---

## 🧪 **快速测试**

### 单元测试

```bash
# 运行 Actor Pool 相关测试
go test -v ./sdk/pkg/eventbus/... -run TestHollywoodActorPool

# 运行所有测试
go test -v ./sdk/pkg/eventbus/...
```

### 集成测试

```bash
# 运行 Kafka 集成测试
go test -v ./tests/eventbus/... -run TestKafkaActorPool

# 性能对比测试
go test -v ./tests/eventbus/... -run TestKafkaActorPoolPerformance
```

---

## 📈 **性能对比**

### 预期结果

| 指标 | Keyed Pool | Actor Pool | 差异 |
|------|-----------|-----------|------|
| **吞吐量** | 100K TPS | ~100K TPS | ~持平 (±5%) |
| **P99 延迟** | 50ms | ~50ms | ~持平 (±5%) |
| **内存占用** | ~50MB | ~50MB | ~持平 |
| **Actor 重启** | N/A | < 1% | 新增监控 |

### 真正优势

- ✅ **Supervisor 机制**: Actor panic 自动重启
- ✅ **事件流监控**: DeadLetter, ActorRestarted 事件
- ✅ **消息保证**: Buffer 机制确保消息不丢失
- ✅ **更好的故障隔离**: Actor 级别隔离

---

## ⚠️ **常见问题**

### Q1: 如何确认 Actor Pool 已启用？

**A**: 查看应用日志，应该看到：

```
INFO  Kafka EventBus using Hollywood Actor Pool poolSize=256 inboxSize=1000 maxRestarts=3
```

### Q2: 可以调整 PoolSize 或 InboxSize 吗？

**A**: 不可以。这些参数已经固定为最优值 (PoolSize=256, InboxSize=1000)。

### Q3: 可以回滚到 Keyed Pool 吗？

**A**: 不可以。Hollywood Actor Pool 已经完全替换 Keyed Pool。如需回滚，请回退代码版本。

### Q4: Inbox 深度监控准确吗？

**A**: Inbox 深度是**近似值**，基于原子计数器。仅用于趋势观测，不保证精确。

### Q5: Actor 重启会丢失消息吗？

**A**: 不会。Supervisor 重启 Actor 时，Inbox 中的消息会保留。

---

## 🎯 **下一步**

### 开发环境

1. ✅ 启用 Actor Pool
2. ✅ 运行单元测试
3. ✅ 运行集成测试
4. ✅ 观测监控指标

### 测试环境

1. ✅ 10% 流量灰度
2. ✅ 观测 24 小时
3. ✅ 对比 Keyed Pool 基线
4. ✅ 确认无异常后扩大灰度

### 生产环境

1. ✅ 50% 流量灰度
2. ✅ 观测 3-5 天
3. ✅ 全量上线
4. ✅ 持续观测 7 天

---

## 📚 **相关文档**

- [实施总结](./kafka-actor-pool-implementation-summary.md)
- [详细实施方案](./hollywood-migration-kafka-implementation.md)
- [架构对比](./hollywood-vs-keyed-worker-pool-comparison.md)
- [监控集成](./hollywood-actor-pool-prometheus-integration.md)

---

## ✅ **检查清单**

### 启用 Actor Pool 前

- [ ] 确认已部署最新代码
- [ ] 确认 Prometheus 监控已配置
- [ ] 确认 Grafana Dashboard 已创建
- [ ] 确认回滚方案已准备

### 启用 Actor Pool 后

- [ ] 确认日志显示 "using Hollywood Actor Pool"
- [ ] 确认 Prometheus 指标正常上报
- [ ] 确认消息正常处理
- [ ] 确认无 Actor 重启异常

### 观测期间

- [ ] 监控吞吐量 (应与 Keyed Pool 持平)
- [ ] 监控 P99 延迟 (应与 Keyed Pool 持平)
- [ ] 监控 Actor 重启率 (应 < 1%)
- [ ] 监控内存占用 (应与 Keyed Pool 持平)

---

## 🆘 **紧急回滚**

### 触发条件

| 指标 | 阈值 | 动作 |
|------|------|------|
| P99 延迟 | > 基线 + 20% | 立即回滚 |
| Actor 重启率 | > 5% | 立即回滚 |
| 错误率 | > 1% | 立即回滚 |
| 内存增长 | > 50% | 观察 30 分钟后回滚 |

### 回滚步骤

```bash
# 1. 禁用 Actor Pool
export KAFKA_USE_ACTOR_POOL=false

# 2. 重启应用
systemctl restart your-service

# 3. 确认回滚成功
curl http://localhost:2112/metrics | grep keyed_worker_pool

# 4. 通知团队
echo "Rolled back to Keyed Worker Pool at $(date)" | mail -s "Actor Pool Rollback" team@example.com
```

---

## 💡 **最佳实践**

### 1. 渐进式灰度

```
开发环境 (100%) → 测试环境 (10%) → 生产环境 (50%) → 生产环境 (100%)
```

### 2. 充分观测

- 每个阶段至少观测 24 小时
- 对比 Keyed Pool 基线数据
- 关注异常指标

### 3. 快速回滚

- 准备好回滚脚本
- 设置告警阈值
- 团队随时待命

### 4. 文档记录

- 记录每次灰度的数据
- 记录遇到的问题和解决方案
- 更新文档和最佳实践

---

**祝你迁移顺利！** 🎉

如有问题，请参考 [详细实施方案](./hollywood-migration-kafka-implementation.md) 或联系团队。

