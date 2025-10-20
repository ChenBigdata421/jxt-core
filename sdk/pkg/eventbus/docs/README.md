# EventBus 技术文档索引

本目录包含 EventBus 组件的核心技术文档，涵盖 Kafka、NATS 的最佳实践、性能优化、使用指南等内容。

---

## 📚 文档分类

### 1️⃣ **Kafka 相关文档**

#### **最佳实践与对比**
- **[KAFKA_INDUSTRY_BEST_PRACTICES.md](./KAFKA_INDUSTRY_BEST_PRACTICES.md)**  
  Kafka 业界最佳实践，包括 Confluent、LinkedIn、Uber 等公司的生产环境经验

- **[KAFKA_MULTI_TOPIC_PRE_SUBSCRIPTION_SOLUTION.md](./KAFKA_MULTI_TOPIC_PRE_SUBSCRIPTION_SOLUTION.md)**  
  Kafka 多 Topic 预订阅解决方案，避免 Consumer Group 重平衡问题

- **[KAFKA_NAMING_CONVENTIONS.md](./KAFKA_NAMING_CONVENTIONS.md)**  
  Kafka Topic 命名规范和最佳实践

- **[KAFKA_NATS_ACK_MECHANISM_COMPARISON.md](./KAFKA_NATS_ACK_MECHANISM_COMPARISON.md)**  
  Kafka 与 NATS 的 ACK 确认机制对比分析

- **[KAFKA_VS_NATS_COMPREHENSIVE_COMPARISON.md](./KAFKA_VS_NATS_COMPREHENSIVE_COMPARISON.md)**  
  Kafka 与 NATS 的全面对比（架构、性能、适用场景）

- **[KAFKA_VS_NATS_FINAL_ANALYSIS.md](./KAFKA_VS_NATS_FINAL_ANALYSIS.md)**  
  Kafka 与 NATS 的最终分析和选型建议

#### **性能优化**
- **[KAFKA_PARTITIONS_PERFORMANCE_OPTIMIZATION.md](./KAFKA_PARTITIONS_PERFORMANCE_OPTIMIZATION.md)**  
  Kafka 分区性能优化指南

- **[KAFKA_PARTITIONS_QUICK_START.md](./KAFKA_PARTITIONS_QUICK_START.md)**  
  Kafka 分区快速开始指南

---

### 2️⃣ **Topic 配置与管理**

- **[TOPIC_CONFIG_STRATEGY.md](./TOPIC_CONFIG_STRATEGY.md)**  
  Topic 配置策略详解（StrategySkip、StrategyCreateOnly、StrategyCreateOrUpdate、StrategyValidateOnly）

- **[TOPIC_PERSISTENCE.md](./TOPIC_PERSISTENCE.md)**  
  Topic 持久化模式详解（TopicPersistent、TopicEphemeral、TopicAuto）

- **[TOPIC_BUILDER_IMPLEMENTATION.md](./TOPIC_BUILDER_IMPLEMENTATION.md)**  
  Topic Builder 实现原理和设计思路

- **[TOPIC_BUILDER_QUICK_START.md](./TOPIC_BUILDER_QUICK_START.md)**  
  Topic Builder 快速开始指南

- **[TOPIC_BUILDER_SUMMARY.md](./TOPIC_BUILDER_SUMMARY.md)**  
  Topic Builder 功能总结

- **[TOPIC_BUILDER_COMPRESSION.md](./TOPIC_BUILDER_COMPRESSION.md)**  
  Topic Builder 压缩功能使用指南

---

### 3️⃣ **快速开始**

- **[QUICK_START_CONFIG_STRATEGY.md](./QUICK_START_CONFIG_STRATEGY.md)**  
  配置策略快速开始指南

---

## 🎯 推荐阅读路径

### **新手入门**
1. [QUICK_START_CONFIG_STRATEGY.md](./QUICK_START_CONFIG_STRATEGY.md) - 快速开始
2. [TOPIC_PERSISTENCE.md](./TOPIC_PERSISTENCE.md) - 理解持久化模式
3. [TOPIC_CONFIG_STRATEGY.md](./TOPIC_CONFIG_STRATEGY.md) - 理解配置策略

### **Kafka 用户**
1. [KAFKA_NAMING_CONVENTIONS.md](./KAFKA_NAMING_CONVENTIONS.md) - 命名规范
2. [KAFKA_MULTI_TOPIC_PRE_SUBSCRIPTION_SOLUTION.md](./KAFKA_MULTI_TOPIC_PRE_SUBSCRIPTION_SOLUTION.md) - 预订阅优化
3. [KAFKA_PARTITIONS_QUICK_START.md](./KAFKA_PARTITIONS_QUICK_START.md) - 分区使用
4. [KAFKA_INDUSTRY_BEST_PRACTICES.md](./KAFKA_INDUSTRY_BEST_PRACTICES.md) - 最佳实践

### **NATS 用户**
1. [KAFKA_VS_NATS_COMPREHENSIVE_COMPARISON.md](./KAFKA_VS_NATS_COMPREHENSIVE_COMPARISON.md) - 理解差异
2. [KAFKA_NATS_ACK_MECHANISM_COMPARISON.md](./KAFKA_NATS_ACK_MECHANISM_COMPARISON.md) - ACK 机制对比

### **高级用户**
1. [TOPIC_BUILDER_IMPLEMENTATION.md](./TOPIC_BUILDER_IMPLEMENTATION.md) - 深入理解 Topic Builder
2. [KAFKA_PARTITIONS_PERFORMANCE_OPTIMIZATION.md](./KAFKA_PARTITIONS_PERFORMANCE_OPTIMIZATION.md) - 性能优化
3. [KAFKA_VS_NATS_FINAL_ANALYSIS.md](./KAFKA_VS_NATS_FINAL_ANALYSIS.md) - 选型决策

---

## 📖 相关资源

- **主文档**: [../README.md](../README.md) - EventBus 完整使用文档
- **示例代码**: [../examples/](../examples/) - 各种使用场景的示例代码
- **测试用例**: [../*_test.go](../) - 单元测试和集成测试

---

## 🔄 文档维护

本目录中的文档是 EventBus 组件的核心技术文档，应该：
- ✅ 保持与代码实现的一致性
- ✅ 定期更新以反映最新的最佳实践
- ✅ 补充真实的生产环境案例
- ❌ 不应包含临时的开发过程文档

如有疑问或建议，请联系 EventBus 维护团队。

