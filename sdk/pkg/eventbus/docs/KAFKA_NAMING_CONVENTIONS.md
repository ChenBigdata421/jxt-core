# Kafka 命名规范（关键限制）

## 🔴 核心规则

**Kafka 的 ClientID 和 Topic 名称必须只使用 ASCII 字符！**

这是 Kafka 的底层限制，违反此规则会导致消息无法接收（0% 成功率）。

---

## 📋 允许的字符

### ASCII 字符集
- **小写字母**：a-z
- **大写字母**：A-Z
- **数字**：0-9
- **特殊字符**：
  - 连字符：`-`
  - 下划线：`_`
  - 点号：`.`

### 禁止的字符
- ❌ **中文**：业务、订单、用户、事件等
- ❌ **日文**：ビジネス、注文等
- ❌ **韩文**：비즈니스、주문등
- ❌ **阿拉伯文**：أعمال、طلب等
- ❌ **其他 Unicode 字符**：emoji、特殊符号等
- ❌ **空格**：不允许使用空格

---

## ❌ 错误示例

### ClientID 错误示例

```go
// ❌ 错误：使用中文
kafkaConfig := &eventbus.KafkaConfig{
    ClientID: "我的服务",           // 会导致消息无法接收
}

// ❌ 错误：混用中英文
kafkaConfig := &eventbus.KafkaConfig{
    ClientID: "my-服务",            // 会导致消息无法接收
}

// ❌ 错误：使用空格
kafkaConfig := &eventbus.KafkaConfig{
    ClientID: "my service",         // 会导致消息无法接收
}
```

### Topic 名称错误示例

```go
// ❌ 错误：使用中文
topics := []string{
    "业务.订单",                    // 会导致消息无法接收
    "用户.事件",                    // 会导致消息无法接收
    "审计.日志",                    // 会导致消息无法接收
}

// ❌ 错误：混用中英文
topics := []string{
    "business.订单",                // 会导致消息无法接收
    "用户.events",                  // 会导致消息无法接收
    "audit.日志",                   // 会导致消息无法接收
}

// ❌ 错误：使用空格
topics := []string{
    "business orders",              // 会导致消息无法接收
    "user events",                  // 会导致消息无法接收
}

// ❌ 错误：使用 emoji
topics := []string{
    "business.orders.🎉",           // 会导致消息无法接收
}
```

### Consumer Group ID 错误示例

```go
// ❌ 错误：使用中文
kafkaConfig := &eventbus.KafkaConfig{
    Consumer: eventbus.ConsumerConfig{
        GroupID: "订单服务组",       // 会导致消息无法接收
    },
}
```

---

## ✅ 正确示例

### ClientID 正确示例

```go
// ✅ 正确：只使用 ASCII 字符
kafkaConfig := &eventbus.KafkaConfig{
    ClientID: "my-service",              // ✅ 正确
}

kafkaConfig := &eventbus.KafkaConfig{
    ClientID: "order-service-prod",      // ✅ 正确
}

kafkaConfig := &eventbus.KafkaConfig{
    ClientID: "user_service_v2",         // ✅ 正确
}

kafkaConfig := &eventbus.KafkaConfig{
    ClientID: "payment.service.123",     // ✅ 正确
}
```

### Topic 名称正确示例

```go
// ✅ 正确：只使用 ASCII 字符
topics := []string{
    "business.orders",                   // ✅ 正确
    "business.payments",                 // ✅ 正确
    "business.users",                    // ✅ 正确
    "audit.logs",                        // ✅ 正确
    "audit.security",                    // ✅ 正确
    "system.notifications",              // ✅ 正确
    "system.metrics",                    // ✅ 正确
    "temp.cache.invalidation",           // ✅ 正确
}

// ✅ 正确：使用下划线分隔
topics := []string{
    "business_orders",                   // ✅ 正确
    "user_events",                       // ✅ 正确
    "audit_logs",                        // ✅ 正确
}

// ✅ 正确：使用连字符分隔
topics := []string{
    "business-orders",                   // ✅ 正确
    "user-events",                       // ✅ 正确
    "audit-logs",                        // ✅ 正确
}
```

### Consumer Group ID 正确示例

```go
// ✅ 正确：只使用 ASCII 字符
kafkaConfig := &eventbus.KafkaConfig{
    Consumer: eventbus.ConsumerConfig{
        GroupID: "order-service-group",  // ✅ 正确
    },
}

kafkaConfig := &eventbus.KafkaConfig{
    Consumer: eventbus.ConsumerConfig{
        GroupID: "user_service_group",   // ✅ 正确
    },
}
```

---

## 🔍 验证方法

### 验证函数

```go
// 验证是否只包含 ASCII 字符
func isValidKafkaName(name string) bool {
    if len(name) == 0 || len(name) > 255 {
        return false
    }
    
    for _, r := range name {
        if r > 127 {
            return false  // 包含非 ASCII 字符
        }
    }
    
    return true
}

// 验证 ClientID
func validateClientID(clientID string) error {
    if !isValidKafkaName(clientID) {
        return fmt.Errorf("invalid ClientID '%s': must use ASCII characters only (a-z, A-Z, 0-9, -, _, .)", clientID)
    }
    return nil
}

// 验证 Topic 名称
func validateTopicName(topic string) error {
    if !isValidKafkaName(topic) {
        return fmt.Errorf("invalid topic name '%s': must use ASCII characters only (a-z, A-Z, 0-9, -, _, .)", topic)
    }
    return nil
}

// 验证 Consumer Group ID
func validateGroupID(groupID string) error {
    if !isValidKafkaName(groupID) {
        return fmt.Errorf("invalid GroupID '%s': must use ASCII characters only (a-z, A-Z, 0-9, -, _, .)", groupID)
    }
    return nil
}
```

### 使用示例

```go
func main() {
    clientID := "my-service"
    if err := validateClientID(clientID); err != nil {
        log.Fatal(err)
    }
    
    topics := []string{
        "business.orders",
        "user.events",
    }
    
    for _, topic := range topics {
        if err := validateTopicName(topic); err != nil {
            log.Fatal(err)
        }
    }
    
    groupID := "my-consumer-group"
    if err := validateGroupID(groupID); err != nil {
        log.Fatal(err)
    }
    
    // 创建 Kafka EventBus
    kafkaConfig := &eventbus.KafkaConfig{
        Brokers:  []string{"localhost:9092"},
        ClientID: clientID,
        Consumer: eventbus.ConsumerConfig{
            GroupID: groupID,
        },
    }
    
    eb, err := eventbus.NewKafkaEventBus(kafkaConfig)
    if err != nil {
        log.Fatal(err)
    }
    defer eb.Close()
}
```

---

## 🚨 违反规则的后果

### 1. 消息无法接收（0% 成功率）

**测试结果**：
- 使用中文 ClientID 或 Topic 名称
- 发送 500 条消息
- **接收 0 条消息**（0% 成功率）

### 2. 无明显错误提示

Kafka 不会抛出明显的错误，只是静默失败：
- 发送端显示成功
- 接收端收不到消息
- 日志中没有明显错误

### 3. 调试困难

问题不易发现：
- 配置看起来正常
- 连接正常
- 发送成功
- 但就是收不到消息

### 4. 生产环境风险

如果在生产环境使用非 ASCII 字符：
- 消息丢失
- 业务中断
- 数据不一致
- 难以排查

---

## 📚 命名建议

### 1. 使用英文单词

```go
// ✅ 推荐：使用英文单词
const (
    TopicOrderEvents    = "business.orders.events"
    TopicPaymentEvents  = "business.payments.events"
    TopicUserEvents     = "business.users.events"
    TopicAuditLogs      = "audit.logs"
)
```

### 2. 使用点号分隔层级

```go
// ✅ 推荐：使用点号分隔
"business.orders.created"
"business.orders.updated"
"business.orders.deleted"
```

### 3. 使用下划线或连字符分隔单词

```go
// ✅ 推荐：使用下划线
"order_service_events"
"user_profile_updates"

// ✅ 推荐：使用连字符
"order-service-events"
"user-profile-updates"
```

### 4. 使用小写字母

```go
// ✅ 推荐：使用小写字母
"business.orders"
"user.events"

// ⚠️ 可以但不推荐：使用大写字母
"BUSINESS.ORDERS"
"USER.EVENTS"
```

### 5. 使用有意义的名称

```go
// ✅ 推荐：有意义的名称
"business.orders.created"
"audit.security.login.failed"
"system.health.check"

// ❌ 不推荐：无意义的名称
"topic1"
"test"
"abc"
```

---

## 🔗 相关文档

- [README.md](./README.md) - EventBus 组件完整文档
- [KAFKA_MULTI_TOPIC_PRE_SUBSCRIPTION_SOLUTION.md](./KAFKA_MULTI_TOPIC_PRE_SUBSCRIPTION_SOLUTION.md) - Kafka 多 Topic 预订阅解决方案
- [KAFKA_INDUSTRY_BEST_PRACTICES.md](./KAFKA_INDUSTRY_BEST_PRACTICES.md) - Kafka 业界最佳实践

---

## ✅ 总结

1. **核心规则**：Kafka 的 ClientID 和 Topic 名称必须只使用 ASCII 字符
2. **允许字符**：a-z, A-Z, 0-9, -, _, .
3. **禁止字符**：中文、日文、韩文等任何 Unicode 字符
4. **后果**：违反规则会导致消息无法接收（0% 成功率）
5. **验证**：使用验证函数在创建 EventBus 前检查
6. **建议**：使用英文单词、点号分隔层级、小写字母、有意义的名称

**记住**：这不是建议，而是 Kafka 的硬性要求！

