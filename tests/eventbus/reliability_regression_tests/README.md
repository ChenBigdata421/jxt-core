# EventBus 可靠性回归测试

## 📋 概述

本目录包含 EventBus 的可靠性和故障恢复测试用例，专门测试 Hollywood Actor Pool 的自恢复能力。

## 🎯 测试目标

### 核心可靠性特性
1. **Actor 自动重启** - Supervisor 机制在 Actor panic 后自动重启
2. **消息保证送达** - Actor 重启后，缓冲区中的消息不丢失
3. **故障隔离** - 单个 Actor 故障不影响其他 Actor
4. **优雅降级** - 达到最大重启次数后的行为
5. **DeadLetter 处理** - 无法送达的消息进入 DeadLetter 队列

## 🧪 测试场景

### 1. Actor Panic 恢复测试
- **场景**: Actor 处理消息时 panic
- **预期**: Supervisor 自动重启 Actor，继续处理后续消息
- **验证**: 消息不丢失，处理顺序正确

### 2. 多次 Panic 重启测试
- **场景**: Actor 连续多次 panic（不超过 MaxRestarts）
- **预期**: 每次 panic 后都能自动重启
- **验证**: 重启次数正确，消息最终都被处理

### 3. 达到最大重启次数测试
- **场景**: Actor panic 次数超过 MaxRestarts（3次）
- **预期**: Actor 停止重启，消息进入 DeadLetter
- **验证**: DeadLetterEvent 被触发，消息可追踪

### 4. 故障隔离测试
- **场景**: 多个 Actor 中只有一个 panic
- **预期**: 只有故障 Actor 重启，其他 Actor 正常工作
- **验证**: 其他 Actor 的消息处理不受影响

### 5. 消息缓冲区保证测试
- **场景**: Actor 重启期间，新消息持续到达
- **预期**: 重启后，缓冲区中的消息按顺序处理
- **验证**: 消息顺序正确，无丢失

### 6. 并发故障恢复测试
- **场景**: 多个 Actor 同时 panic
- **预期**: 所有 Actor 都能独立恢复
- **验证**: 所有消息最终都被处理

### 7. EventStream 监控测试
- **场景**: 订阅 ActorRestartedEvent 和 DeadLetterEvent
- **预期**: 所有故障事件都能被监控到
- **验证**: 事件数量和内容正确

## 📊 测试指标

### 可靠性指标
- **消息送达率**: 应该 100%（除非达到 MaxRestarts）
- **重启成功率**: 应该 100%（在 MaxRestarts 范围内）
- **故障隔离率**: 应该 100%（故障不扩散）
- **恢复时间**: 应该 < 100ms

### 性能指标
- **重启延迟**: Actor 重启到恢复处理的时间
- **消息积压**: 重启期间的消息堆积情况
- **吞吐量影响**: 故障恢复对整体吞吐量的影响

## 🚀 运行测试

### 运行所有可靠性测试
```bash
cd jxt-core/tests/eventbus/reliability_regression_tests
go test -v -timeout 180s
```

### 运行特定测试
```bash
# Actor Panic 恢复测试
go test -v -run TestActorPanicRecovery

# 多次重启测试
go test -v -run TestMultiplePanicRestarts

# 最大重启次数测试
go test -v -run TestMaxRestartsExceeded

# 故障隔离测试
go test -v -run TestFaultIsolation

# 消息缓冲区测试
go test -v -run TestMessageBufferGuarantee

# 并发故障恢复测试
go test -v -run TestConcurrentFaultRecovery

# EventStream 监控测试
go test -v -run TestEventStreamMonitoring
```

## 📝 测试文件

- `actor_recovery_test.go` - Actor 恢复相关测试
- `fault_isolation_test.go` - 故障隔离测试
- `message_guarantee_test.go` - 消息保证送达测试
- `eventstream_monitoring_test.go` - EventStream 监控测试
- `test_helper.go` - 测试辅助函数

## 🔧 Hollywood Supervisor 机制

### 配置参数
```go
actor.WithMaxRestarts(3)  // 最大重启次数（默认 3）
actor.WithInboxSize(1000) // Inbox 缓冲区大小（默认 1000）
```

### 重启策略
- **OneForOne**: 只重启故障的 Actor（Hollywood 默认）
- **自动重启**: panic 后自动重启，无需手动干预
- **消息保证**: Inbox 中的消息在重启后继续处理

### 事件通知
- `actor.ActorRestartedEvent` - Actor 重启事件
- `actor.DeadLetterEvent` - 消息无法送达事件

## ✅ 验收标准

### 必须通过的测试
- [x] Actor Panic 恢复测试
- [x] 多次 Panic 重启测试
- [x] 达到最大重启次数测试
- [x] 故障隔离测试
- [x] 消息缓冲区保证测试
- [x] 并发故障恢复测试
- [x] EventStream 监控测试

### 性能要求
- 重启延迟 < 100ms
- 消息送达率 = 100%（在 MaxRestarts 范围内）
- 故障隔离率 = 100%

## 📚 参考资料

- [Hollywood GitHub](https://github.com/anthdm/hollywood)
- [Actor Model](https://en.wikipedia.org/wiki/Actor_model)
- [Supervisor Pattern](https://www.erlang.org/doc/design_principles/sup_princ.html)

