# Kafka 分区数量配置建议（单节点场景）

## 测试场景分析

### 当前测试参数

| 压力级别 | 消息数 | 聚合ID数量 | Topic数量 | Worker数量 |
|---------|--------|-----------|----------|-----------|
| 低压 | 500 | 50 | 5 | 256 |
| 中压 | 2000 | 200 | 5 | 256 |
| 高压 | 5000 | 500 | 5 | 256 |
| 极限 | 10000 | 1000 | 5 | 256 |

### 关键约束

1. **单节点 Kafka**：只有 1 个 broker
2. **全局 Keyed-Worker Pool**：256 个 workers
3. **顺序保证需求**：相同聚合ID必须按顺序处理
4. **5 个 Topic**：每个 topic 独立配置分区

---

## 分区数量计算公式

### 理论公式

```
最佳分区数 = max(
    聚合ID数量 / 每个分区的聚合ID数量,
    目标吞吐量 / 单分区吞吐量,
    Worker数量 / Topic数量
)
```

### 实际考虑因素

#### 1. 聚合ID分布

**原则**：分区数应该 ≤ 聚合ID数量

**原因**：
- 如果分区数 > 聚合ID数量，会有空闲分区
- 浪费资源，降低效率

**计算**：
| 压力级别 | 聚合ID数量 | 建议最大分区数 |
|---------|-----------|--------------|
| 低压 | 50 | 50 |
| 中压 | 200 | 200 |
| 高压 | 500 | 500 |
| 极限 | 1000 | 1000 |

#### 2. Worker Pool 并发度

**原则**：分区数应该 ≤ Worker数量 / Topic数量

**原因**：
- 256 个 workers 需要处理 5 个 topic
- 每个 topic 平均分配：256 / 5 ≈ 51 个 workers
- 分区数 > 51 时，部分分区无法充分利用 workers

**计算**：
```
每个 Topic 的最佳分区数 = 256 / 5 = 51
```

#### 3. 单节点 Kafka 的限制

**原则**：单节点 Kafka 的分区数不宜过多

**原因**：
- 每个分区需要独立的文件句柄
- 过多分区会增加元数据开销
- 单节点 Kafka 推荐：总分区数 < 2000

**计算**：
```
5 个 Topic × 每个 Topic 的分区数 < 2000
每个 Topic 的分区数 < 400
```

#### 4. 顺序保证需求

**原则**：相同聚合ID必须路由到同一分区

**实现**：
- 使用 HashPartitioner（已配置 ✅）
- 分区数不影响顺序保证
- 但分区数越少，顺序保证越简单

---

## 推荐配置

### 方案 1：保守配置（推荐用于生产）

**每个 Topic 配置：3 个分区**

**理由**：
1. **平衡性能和顺序**
   - 3 个分区可以充分利用并发
   - 不会过度分散聚合ID
   
2. **Kafka 最佳实践**
   - 3 是 Kafka 社区推荐的最小分区数
   - 提供基本的并发度和容错能力
   
3. **适合所有压力级别**
   - 低压(50 聚合ID)：每分区约 17 个聚合ID
   - 中压(200 聚合ID)：每分区约 67 个聚合ID
   - 高压(500 聚合ID)：每分区约 167 个聚合ID
   - 极限(1000 聚合ID)：每分区约 333 个聚合ID

4. **Worker 利用率**
   - 5 个 Topic × 3 个分区 = 15 个分区
   - 256 个 workers / 15 个分区 ≈ 17 个 workers/分区
   - 充分利用 worker pool

**创建命令**：
```bash
for i in {1..5}; do
  kafka-topics.sh --create \
    --topic kafka.perf.low.topic$i \
    --partitions 3 \
    --replication-factor 1 \
    --bootstrap-server localhost:29094
done
```

**预期效果**：
- ✅ 顺序保证：相同聚合ID路由到同一分区
- ✅ 并发度：3 个分区并发消费
- ✅ 性能：吞吐量提升约 3 倍
- ✅ 资源利用：充分利用 worker pool

---

### 方案 2：高性能配置（推荐用于测试）

**每个 Topic 配置：10 个分区**

**理由**：
1. **更高的并发度**
   - 10 个分区可以更充分利用 256 个 workers
   - 5 个 Topic × 10 个分区 = 50 个分区
   - 256 个 workers / 50 个分区 ≈ 5 个 workers/分区

2. **适合高压和极限场景**
   - 高压(500 聚合ID)：每分区约 50 个聚合ID
   - 极限(1000 聚合ID)：每分区约 100 个聚合ID

3. **更好的负载均衡**
   - 10 个分区可以更均匀地分散聚合ID
   - 减少单个分区的热点问题

**创建命令**：
```bash
for i in {1..5}; do
  kafka-topics.sh --create \
    --topic kafka.perf.medium.topic$i \
    --partitions 10 \
    --replication-factor 1 \
    --bootstrap-server localhost:29094
done
```

**预期效果**：
- ✅ 顺序保证：相同聚合ID路由到同一分区
- ✅ 高并发度：10 个分区并发消费
- ✅ 高性能：吞吐量提升约 10 倍
- ⚠️ 资源开销：稍高的元数据开销

---

### 方案 3：极限性能配置（仅用于极限测试）

**每个 Topic 配置：50 个分区**

**理由**：
1. **最大化并发度**
   - 50 个分区 = 256 workers / 5 topics
   - 每个分区约 5 个 workers
   - 充分利用所有 workers

2. **适合极限场景**
   - 极限(1000 聚合ID)：每分区约 20 个聚合ID
   - 充分分散负载

**创建命令**：
```bash
for i in {1..5}; do
  kafka-topics.sh --create \
    --topic kafka.perf.extreme.topic$i \
    --partitions 50 \
    --replication-factor 1 \
    --bootstrap-server localhost:29094
done
```

**预期效果**：
- ✅ 顺序保证：相同聚合ID路由到同一分区
- ✅ 最大并发度：50 个分区并发消费
- ✅ 极限性能：吞吐量提升约 50 倍
- ⚠️ 资源开销：较高的元数据开销
- ⚠️ 复杂度：管理复杂度增加

---

### 方案 4：单分区配置（仅用于顺序验证）

**每个 Topic 配置：1 个分区**

**理由**：
1. **100% 顺序保证**
   - 单分区保证全局顺序
   - 适合验证顺序违反问题

2. **简化调试**
   - 消除多分区并发的复杂性
   - 容易定位问题

**创建命令**：
```bash
for i in {1..5}; do
  kafka-topics.sh --create \
    --topic kafka.perf.test.topic$i \
    --partitions 1 \
    --replication-factor 1 \
    --bootstrap-server localhost:29094
done
```

**预期效果**：
- ✅ 100% 顺序保证
- ❌ 低并发度：只能串行消费
- ❌ 低性能：吞吐量受限
- ✅ 简化调试：容易定位问题

---

## 综合推荐

### 按测试目的选择

#### 1. 功能测试（验证顺序保证）

**推荐**：**1 个分区**

**原因**：
- 消除多分区并发的复杂性
- 100% 验证顺序保证
- 容易定位问题

#### 2. 性能测试（平衡性能和顺序）

**推荐**：**3 个分区**

**原因**：
- Kafka 最佳实践
- 平衡性能和复杂度
- 适合所有压力级别

#### 3. 压力测试（最大化性能）

**推荐**：**10 个分区**

**原因**：
- 充分利用 worker pool
- 高并发度
- 适合高压和极限场景

#### 4. 极限测试（测试系统上限）

**推荐**：**50 个分区**

**原因**：
- 最大化并发度
- 测试系统极限
- 发现性能瓶颈

---

## 实施建议

### 步骤 1：验证顺序保证（单分区测试）

```bash
# 创建单分区 topic
for i in {1..5}; do
  kafka-topics.sh --create \
    --topic kafka.perf.test.single.topic$i \
    --partitions 1 \
    --replication-factor 1 \
    --bootstrap-server localhost:29094
done

# 运行测试
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 10m
```

**预期结果**：
- Kafka 顺序违反：**0 次** ✅
- 如果仍有顺序违反，说明问题在 Keyed-Worker Pool 或检测逻辑

### 步骤 2：性能测试（3 分区）

```bash
# 创建 3 分区 topic
for i in {1..5}; do
  kafka-topics.sh --create \
    --topic kafka.perf.test.3part.topic$i \
    --partitions 3 \
    --replication-factor 1 \
    --bootstrap-server localhost:29094
done

# 运行测试
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 10m
```

**预期结果**：
- Kafka 顺序违反：**0 次** ✅（如果检测逻辑正确）
- 吞吐量：提升约 3 倍

### 步骤 3：压力测试（10 分区）

```bash
# 创建 10 分区 topic
for i in {1..5}; do
  kafka-topics.sh --create \
    --topic kafka.perf.test.10part.topic$i \
    --partitions 10 \
    --replication-factor 1 \
    --bootstrap-server localhost:29094
done

# 运行测试
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 10m
```

**预期结果**：
- Kafka 顺序违反：**0 次** ✅（如果检测逻辑正确）
- 吞吐量：提升约 10 倍

---

## 总结

### 最终推荐

| 场景 | 分区数 | 理由 |
|------|--------|------|
| **功能验证** | **1** | 100% 顺序保证，简化调试 |
| **日常测试** | **3** | Kafka 最佳实践，平衡性能和复杂度 |
| **性能测试** | **10** | 充分利用 worker pool，高并发度 |
| **极限测试** | **50** | 最大化并发度，测试系统上限 |

### 当前问题的解决方案

**立即行动**：
1. 创建单分区 topic 测试
2. 验证顺序违反是否为 0
3. 如果为 0，说明问题在多分区并发或检测逻辑
4. 如果仍有顺序违反，说明问题在 Keyed-Worker Pool

**长期方案**：
- 生产环境：使用 **3 个分区**
- 测试环境：使用 **10 个分区**
- 功能验证：使用 **1 个分区**

