# NATS JetStream Subscribe() 迁移到 Hollywood Actor Pool - 文档索引

**文档版本**: v1.0  
**创建日期**: 2025-10-30  
**状态**: 待评审  

---

## 📚 文档导航

### 🎯 快速开始

**如果您是第一次阅读，建议按以下顺序阅读文档**:

1. **[迁移总结](./nats-subscribe-to-actor-pool-migration-summary.md)** ⭐ 推荐首先阅读
   - 快速了解迁移目标、收益、影响
   - 查看架构对比图
   - 了解实施步骤和时间估算

2. **[架构设计文档](./nats-subscribe-to-actor-pool-migration-architecture.md)**
   - 深入了解当前架构的问题
   - 理解目标架构的设计原理
   - 查看技术决策和风险评估

3. **[实施计划文档](./nats-subscribe-to-actor-pool-migration-implementation.md)**
   - 查看详细的代码修改步骤
   - 了解每个步骤的具体操作
   - 查看代码 Diff 预览

4. **[测试计划文档](./nats-subscribe-to-actor-pool-migration-testing.md)**
   - 了解测试策略和验证方案
   - 查看测试用例清单
   - 了解成功标准

5. **[影响分析文档](./nats-subscribe-to-actor-pool-migration-impact.md)**
   - 了解对用户、代码、性能、运维的影响
   - 查看风险缓解措施
   - 了解监控指标变化

---

## 📖 文档清单

### 核心文档

| 文档名称 | 文件路径 | 说明 | 页数 |
|---------|---------|------|------|
| **迁移总结** | [nats-subscribe-to-actor-pool-migration-summary.md](./nats-subscribe-to-actor-pool-migration-summary.md) | 迁移总结和快速参考 | ~300 行 |
| **架构设计** | [nats-subscribe-to-actor-pool-migration-architecture.md](./nats-subscribe-to-actor-pool-migration-architecture.md) | 架构设计和技术决策 | ~300 行 |
| **实施计划** | [nats-subscribe-to-actor-pool-migration-implementation.md](./nats-subscribe-to-actor-pool-migration-implementation.md) | 详细实施步骤 | ~300 行 |
| **测试计划** | [nats-subscribe-to-actor-pool-migration-testing.md](./nats-subscribe-to-actor-pool-migration-testing.md) | 测试策略和用例 | ~300 行 |
| **影响分析** | [nats-subscribe-to-actor-pool-migration-impact.md](./nats-subscribe-to-actor-pool-migration-impact.md) | 影响分析和风险评估 | ~300 行 |
| **文档索引** | [nats-subscribe-to-actor-pool-migration-index.md](./nats-subscribe-to-actor-pool-migration-index.md) | 本文档 | ~200 行 |

---

## 🔍 快速查找

### 架构相关

- **当前架构问题**: [架构设计文档 - 背景和动机](./nats-subscribe-to-actor-pool-migration-architecture.md#背景和动机)
- **目标架构设计**: [架构设计文档 - 目标架构设计](./nats-subscribe-to-actor-pool-migration-architecture.md#目标架构设计)
- **路由策略**: [架构设计文档 - 技术决策](./nats-subscribe-to-actor-pool-migration-architecture.md#技术决策)
- **架构对比**: [架构设计文档 - 架构对比](./nats-subscribe-to-actor-pool-migration-architecture.md#架构对比)

### 实施相关

- **代码修改清单**: [实施计划文档 - 代码修改清单](./nats-subscribe-to-actor-pool-migration-implementation.md#代码修改清单)
- **详细实施步骤**: [实施计划文档 - 详细实施步骤](./nats-subscribe-to-actor-pool-migration-implementation.md#详细实施步骤)
- **代码 Diff 预览**: [实施计划文档 - 附录 A](./nats-subscribe-to-actor-pool-migration-implementation.md#附录)

### 测试相关

- **功能测试**: [测试计划文档 - 功能测试](./nats-subscribe-to-actor-pool-migration-testing.md#功能测试)
- **性能测试**: [测试计划文档 - 性能测试](./nats-subscribe-to-actor-pool-migration-testing.md#性能测试)
- **可靠性测试**: [测试计划文档 - 可靠性测试](./nats-subscribe-to-actor-pool-migration-testing.md#可靠性测试)
- **测试执行清单**: [测试计划文档 - 附录 A](./nats-subscribe-to-actor-pool-migration-testing.md#附录)

### 影响相关

- **影响概览**: [影响分析文档 - 影响概览](./nats-subscribe-to-actor-pool-migration-impact.md#影响概览)
- **用户影响**: [影响分析文档 - 用户影响分析](./nats-subscribe-to-actor-pool-migration-impact.md#用户影响分析)
- **性能影响**: [影响分析文档 - 性能影响分析](./nats-subscribe-to-actor-pool-migration-impact.md#性能影响分析)
- **风险缓解**: [影响分析文档 - 风险缓解措施](./nats-subscribe-to-actor-pool-migration-impact.md#风险缓解措施)

---

## 👥 按角色阅读指南

### 项目经理

**推荐阅读**:
1. [迁移总结](./nats-subscribe-to-actor-pool-migration-summary.md) - 了解整体情况
2. [影响分析 - 影响概览](./nats-subscribe-to-actor-pool-migration-impact.md#影响概览) - 了解影响范围
3. [架构设计 - 风险评估](./nats-subscribe-to-actor-pool-migration-architecture.md#风险评估) - 了解风险

**关注重点**:
- 实施时间: 5.5 小时
- 影响范围: 用户无感知，内部重构
- 风险等级: 中等风险
- 成功标准: 功能、性能、测试覆盖

---

### 架构师

**推荐阅读**:
1. [架构设计文档](./nats-subscribe-to-actor-pool-migration-architecture.md) - 完整阅读
2. [技术决策](./nats-subscribe-to-actor-pool-migration-architecture.md#技术决策) - 重点关注
3. [架构对比](./nats-subscribe-to-actor-pool-migration-architecture.md#架构对比) - 重点关注

**关注重点**:
- 路由策略: Round-Robin 轮询路由
- 顺序语义: 从并发变为串行（相同 topic）
- 性能影响: 单 Topic 可能成为瓶颈
- 可观测性: Actor 级别监控

---

### 开发工程师

**推荐阅读**:
1. [实施计划文档](./nats-subscribe-to-actor-pool-migration-implementation.md) - 完整阅读
2. [代码修改清单](./nats-subscribe-to-actor-pool-migration-implementation.md#代码修改清单) - 重点关注
3. [详细实施步骤](./nats-subscribe-to-actor-pool-migration-implementation.md#详细实施步骤) - 重点关注

**关注重点**:
- 修改文件: `nats.go`
- 删除代码: ~200 行（全局 Worker Pool）
- 修改代码: ~50 行（路由逻辑）
- 代码 Diff: 查看附录 A

---

### 测试工程师

**推荐阅读**:
1. [测试计划文档](./nats-subscribe-to-actor-pool-migration-testing.md) - 完整阅读
2. [测试执行清单](./nats-subscribe-to-actor-pool-migration-testing.md#附录) - 重点关注
3. [成功标准](./nats-subscribe-to-actor-pool-migration-architecture.md#成功标准) - 重点关注

**关注重点**:
- 测试用例: 10+ 个
- 测试时间: 2.5 小时
- 成功标准: 功能、性能、可靠性
- 测试命令: 查看附录 A

---

### 运维工程师

**推荐阅读**:
1. [影响分析 - 运维影响](./nats-subscribe-to-actor-pool-migration-impact.md#运维影响分析) - 重点关注
2. [监控指标变化](./nats-subscribe-to-actor-pool-migration-impact.md#监控指标变化) - 重点关注
3. [回滚方案](./nats-subscribe-to-actor-pool-migration-implementation.md#回滚方案) - 重点关注

**关注重点**:
- 新增监控指标: Actor Pool 指标
- 删除监控指标: Worker Pool 指标
- 告警规则: 需要更新
- 回滚方案: Git 回滚或手动恢复

---

## 📊 文档统计

### 文档规模

| 指标 | 数量 |
|------|------|
| **文档总数** | 6 个（含索引） |
| **总行数** | ~1800 行 |
| **总字数** | ~50,000 字 |
| **创建时间** | 2025-10-30 |

### 内容分布

| 文档类型 | 文档数 | 占比 |
|---------|-------|------|
| **架构设计** | 1 | 17% |
| **实施计划** | 1 | 17% |
| **测试计划** | 1 | 17% |
| **影响分析** | 1 | 17% |
| **总结文档** | 1 | 17% |
| **索引文档** | 1 | 17% |

---

## 🔄 文档更新记录

| 日期 | 版本 | 更新内容 | 作者 |
|------|------|---------|------|
| 2025-10-30 | v1.0 | 初始版本创建 | AI Assistant |

---

## 📚 参考文档

### 相关迁移文档

- [Kafka Subscribe() 迁移文档索引](./kafka-subscribe-to-actor-pool-migration-index.md)
- [Memory EventBus 迁移文档](./memory-eventbus-migration-to-hollywood-actor-pool.md)
- [双消息投递语义设计文档](./dual-message-delivery-semantics.md)

### 技术参考

- [Hollywood Actor Framework](https://github.com/anthdm/hollywood)
- [NATS JetStream Documentation](https://docs.nats.io/nats-concepts/jetstream)
- [Actor Model](https://en.wikipedia.org/wiki/Actor_model)

---

## 💡 使用建议

1. **首次阅读**: 从迁移总结开始，快速了解整体情况
2. **深入研究**: 按角色阅读指南选择相关文档
3. **实施前**: 完整阅读实施计划和测试计划
4. **实施中**: 参考代码修改清单和详细步骤
5. **实施后**: 执行测试计划，验证成功标准

---

## 📞 联系方式

如有疑问，请联系:
- **技术负责人**: [待定]
- **项目经理**: [待定]
- **文档维护**: AI Assistant

---

**文档状态**: ✅ 待评审  
**创建日期**: 2025-10-30  
**下一步**: 等待评审批准后，开始代码实施

