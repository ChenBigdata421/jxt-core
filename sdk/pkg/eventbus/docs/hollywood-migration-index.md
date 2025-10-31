# Hollywood Actor Pool 迁移文档索引

> **重要**: 本方案使用 **固定 Actor Pool**,而非一个聚合ID一个Actor,适合千万级聚合ID场景。

## 📚 **文档导航**

本目录包含 jxt-core EventBus 从 Keyed Worker Pool 迁移到 Hollywood Actor Pool 的完整文档。

---

## 🎯 **快速开始**

### 新手入门 (按顺序阅读)

1. **[Hollywood Actor Pool 迁移指南](./hollywood-actor-pool-migration-guide.md)** ⭐⭐⭐⭐⭐
   - 🏗️ Actor Pool 架构设计 (固定256个Actor)
   - 💻 完整代码实现
   - ⚙️ 配置示例 (含 Mailbox 深度分析)
   - 📊 性能对比
   - 🚀 迁移步骤 (5-8天)
   - **推荐**: 必读!核心方案文档

2. **[Hollywood Mailbox 深度调优指南](./hollywood-mailbox-tuning-guide.md)** ⭐⭐⭐⭐⭐
   - 📏 Mailbox 深度计算公式
   - 📊 性能影响矩阵
   - 🎯 选择决策树
   - 🔍 监控与诊断
   - 🛠️ 实战调优案例
   - **推荐**: 必读!Mailbox 是关键性能参数

3. **[Hollywood Actor Pool Prometheus 监控集成](./hollywood-actor-pool-prometheus-integration.md)** ⭐⭐⭐⭐⭐
   - 📊 **混合方案 (接口注入 + Middleware)** ✅
   - 🔌 与 EventBus 一致的接口注入方式
   - 🤖 Middleware 自动拦截消息处理
   - 💻 完整的实现代码
   - 📈 Grafana 查询示例
   - 🔍 与 EventBus 监控对比
   - **推荐**: 必读!生产环境必备监控

4. **[Hollywood 快速参考](./hollywood-quick-reference.md)** ⭐⭐⭐⭐⭐
   - 🚀 快速开始 (5分钟上手)
   - ⚙️ 配置参数详解 (poolSize, inboxSize, maxRestarts)
   - 📈 性能调优 (3种场景)
   - 🐛 常见问题
   - **推荐**: 必读!快速上手指南

5. **[Hollywood vs Keyed Worker Pool 对比](./hollywood-vs-keyed-worker-pool-comparison.md)** ⭐⭐⭐⭐
   - 📊 架构对比
   - 🔍 核心问题分析
   - 📈 性能对比
   - 💡 迁移建议
   - **推荐**: 了解为什么要迁移

6. **[README: Hollywood 迁移总览](./README-HOLLYWOOD-MIGRATION.md)** ⭐⭐⭐⭐
   - 📝 快速总览
   - 🎯 核心收益
   - ⚙️ 配置参数 (含 Mailbox 深度)
   - 📈 监控指标
   - **推荐**: 快速了解整体方案

6. **[旧版: Hollywood Actor 迁移指南](./hollywood-actor-migration-guide.md)** ⚠️ 已废弃
   - ⚠️ 此文档基于"一个聚合ID一个Actor"设计,不适合千万级聚合ID
   - ⚠️ 请使用 `hollywood-actor-pool-migration-guide.md` 代替

---

## 📖 **文档详情**

### 1. Hollywood Actor Pool 迁移指南 (核心文档)

**文件**: `hollywood-actor-pool-migration-guide.md`

**内容概要**:
- 核心设计理念:
  - 千万级聚合ID约束
  - 固定 Actor Pool (256/512/1024)
  - 单机部署,无分布式
  - 保持顺序性
- Actor Pool 架构设计
- 完整代码实现:
  - `hollywood_actor_pool.go` (Pool管理)
  - `pool_actor.go` (Actor实现)
  - `hollywood_pool_metrics.go` (监控指标)
- 集成到 EventBus
- 配置示例
- 性能对比
- 迁移步骤 (5-8天)

**适合人群**:
- 架构师: 核心设计方案
- 开发者: 完整代码实现
- 项目经理: 工期评估

**阅读时间**: 30-40分钟

---

### 2. Hollywood 快速参考

**文件**: `hollywood-quick-reference.md`

**内容概要**:
- 快速开始 (安装、配置、代码示例)
- Actor Pool 架构对比
- 核心概念:
  - Actor Pool 架构
  - 消息处理流程
  - Supervisor 机制
- 配置参数详解:
  - poolSize (256/512/1024)
  - inboxSize
  - maxRestarts
  - enableEventStream
- 性能调优 (3种场景)
- 监控指标
- 常见问题 (Q&A)

**适合人群**:
- 开发者: 快速上手
- 运维人员: 配置和监控

**阅读时间**: 10-15分钟

---

### 3. Hollywood vs Keyed Worker Pool 对比

**文件**: `hollywood-vs-keyed-worker-pool-comparison.md`

**内容概要**:
- 架构对比 (图示)
- 核心问题分析:
  - 头部阻塞问题
  - 资源利用效率
  - 故障隔离
  - 消息可靠性
- 性能对比
- 迁移收益总结
- 迁移路径

**适合人群**:
- 决策者: 了解迁移价值
- 架构师: 理解架构差异

**阅读时间**: 15-20分钟

---



---

## 🗺️ **学习路径**

### 路径 1: 决策者 (20分钟)

```
1. 阅读 "Hollywood vs Keyed Worker Pool 对比" (10分钟)
   ↓
2. 阅读 "Hollywood Actor Pool 迁移指南" 的前两章 (10分钟)
   ↓
决策: 是否迁移
```

### 路径 2: 架构师 (1小时)

```
1. 阅读 "Hollywood Actor Pool 迁移指南" 完整版 (40分钟)
   ↓
2. 阅读 "Hollywood 快速参考" (20分钟)
   ↓
输出: 详细设计方案
```

### 路径 3: 开发者 (1.5小时)

```
1. 阅读 "Hollywood 快速参考" (15分钟)
   ↓
2. 阅读 "Hollywood Actor Pool 迁移指南" 的代码实现部分 (45分钟)
   ↓
3. 动手实践: 实现核心组件 (30分钟)
   ↓
输出: 可运行的代码
```

### 路径 4: 运维人员 (30分钟)

```
1. 阅读 "Hollywood 快速参考" 的 "配置参数详解" (15分钟)
   ↓
2. 阅读 "Hollywood 快速参考" 的 "性能调优" (15分钟)
   ↓
输出: 配置方案
```

---

## 🎯 **关键决策点**

### 决策 1: 是否迁移?

**阅读**:
- `hollywood-vs-keyed-worker-pool-comparison.md`
- `hollywood-actor-pool-migration-guide.md` 的 "核心优势" 章节

**决策依据**:
- ✅ 需要 Supervisor 自动重启机制 → 迁移
- ✅ 需要事件流监控 (DeadLetter, ActorRestarted) → 迁移
- ✅ 需要消息保证送达 → 迁移
- ✅ 可靠性要求高 → 迁移
- ❌ 对可靠性要求不高 → 不迁移
- ❌ 团队不熟悉 Actor 模型 → 谨慎评估

### 决策 2: Pool 大小?

**阅读**:
- `hollywood-quick-reference.md` 的 "配置参数详解 - poolSize"

**参数选择**:
- **千万级聚合**: poolSize=256 ✅ 推荐
- **亿级聚合**: poolSize=512
- **超大规模**: poolSize=1024
- **小规模**: poolSize=128

### 决策 3: 配置参数?

**阅读**:
- `hollywood-quick-reference.md` 的 "性能调优"

**参数选择**:
- **高吞吐**: poolSize=1024, inboxSize=10000
- **低延迟**: poolSize=256, inboxSize=100
- **内存受限**: poolSize=128, inboxSize=500

---

## 📊 **迁移时间表**

### 标准时间表 (5-8天) ✅ 推荐

```
Week 1:
├─ Day 1-2: Phase 1 实现阶段
│  ├─ 实现 hollywood_actor_pool.go
│  ├─ 实现 pool_actor.go
│  ├─ 实现 hollywood_pool_metrics.go
│  └─ 集成到 kafka.go/nats.go
│
├─ Day 3-5: Phase 2 测试阶段
│  ├─ 单元测试
│  ├─ 集成测试
│  └─ 性能测试
│
├─ Day 6-8: Phase 3 灰度上线
│  ├─ 特性开关: useHollywood=true
│  ├─ 监控对比
│  └─ 全量上线
```

### 快速时间表 (3-4天,风险较高)

```
Week 1:
├─ Day 1-2: 实现 + 单元测试
├─ Day 3: 集成测试 + 性能测试
├─ Day 4: 灰度上线
```

---

## ✅ **检查清单**

### 开发前检查

- [ ] 阅读 `hollywood-actor-pool-migration-guide.md`
- [ ] 阅读 `hollywood-quick-reference.md`
- [ ] 理解 Actor Pool 架构
- [ ] 准备开发环境
- [ ] 安装 Hollywood 依赖: `go get github.com/anthdm/hollywood@latest`

### 开发中检查

- [ ] 实现 `hollywood_actor_pool.go`
- [ ] 实现 `pool_actor.go`
- [ ] 实现 `hollywood_pool_metrics.go`
- [ ] 集成到 `kafka.go` 和 `nats.go`
- [ ] 添加配置项: `useHollywood`, `hollywood.poolSize`, etc.
- [ ] 单元测试覆盖率 > 80%
- [ ] 集成测试通过
- [ ] 性能测试达标

### 上线前检查

- [ ] 功能测试通过
- [ ] 性能测试通过 (吞吐量、延迟)
- [ ] 压力测试通过
- [ ] 监控指标配置完成
- [ ] 特性开关准备: `useHollywood: false` (默认关闭)
- [ ] 回滚方案准备

### 上线后检查

- [ ] 监控指标正常 (messages_sent, messages_processed, actors_restarted)
- [ ] 无 DeadLetter 告警
- [ ] 性能达到预期
- [ ] 特性开关切换: `useHollywood: true`
- [ ] 旧代码清理 (可选,保留 Keyed Worker Pool 作为降级方案)

---

## 🆘 **获取帮助**

### 文档问题

- 查看本索引文档
- 阅读对应的详细文档
- 查看代码示例

### 技术问题

- 查看 "Hollywood 快速参考" 的 "常见问题"
- 查看 Hollywood 官方文档: https://github.com/anthdm/hollywood
- 查看 Hollywood 示例: https://github.com/anthdm/hollywood/tree/master/examples

### 实施问题

- 查看 "Hollywood 迁移指南" 的详细步骤
- 查看 "Hollywood 实现示例" 的代码
- 参考 jxt-core 现有代码结构

---

## 📈 **预期收益**

### 可靠性提升 (核心收益)

- ✅ **Supervisor 机制**: Actor panic 自动重启,不影响其他消息
- ✅ **事件流监控**: DeadLetter, ActorRestarted 事件
- ✅ **消息保证**: Buffer 机制确保消息不丢失
- ✅ **故障隔离**: Actor 级别,一个 Actor 崩溃不影响其他255个

### 可观测性提升

- ✅ **更丰富的监控指标**: messages_sent, messages_processed, actors_restarted, dead_letters
- ✅ **事件流**: 实时监控 Actor 状态变化
- ✅ **延迟统计**: P99 延迟监控

### 性能提升 (次要收益)

- ✅ 吞吐量: 100K → 100-120K TPS (+0-20%)
- ✅ 延迟: P99 50ms → 40-50ms (略有改善)
- ⚠️ 内存占用: 相同 (都是固定 Pool)

---

## 🎓 **学习资源**

### Actor 模型

- **The Actor Model**: https://www.brianstorti.com/the-actor-model/
- **Akka Documentation**: https://doc.akka.io/docs/akka/current/typed/guide/introduction.html

### Hollywood 框架

- **Hollywood GitHub**: https://github.com/anthdm/hollywood
- **Hollywood Examples**: https://github.com/anthdm/hollywood/tree/master/examples

### DDD 聚合设计

- **DDD Aggregate**: https://martinfowler.com/bliki/DDD_Aggregate.html
- **Event Sourcing**: https://microservices.io/patterns/data/event-sourcing.html

---

## 📞 **联系方式**

如有问题,请联系:
- 项目负责人: [待填写]
- 技术支持: [待填写]
- 文档维护: [待填写]

---

**最后更新**: 2025-10-29
**文档版本**: v1.0
**维护者**: jxt-core Team

