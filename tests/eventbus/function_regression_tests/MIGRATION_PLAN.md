# EventBus 测试迁移执行计划

## 📋 执行概览

**目标**: 将 30 个功能测试从 `sdk/pkg/eventbus/` 迁移到 `tests/eventbus/function_tests/`  
**预期时间**: 2-3 天  
**预期覆盖率提升**: 69.8% → 75%+  
**风险等级**: 低 (不影响现有代码)

---

## 🎯 迁移清单

### P0 - 第一批迁移 (15个文件)

#### 核心功能 (5个)
- [ ] `eventbus_core_test.go` - EventBus 核心功能测试
- [ ] `eventbus_types_test.go` - EventBus 类型创建测试
- [ ] `factory_test.go` - 工厂方法测试
- [ ] `envelope_advanced_test.go` - Envelope 高级功能测试
- [ ] `message_formatter_test.go` - 消息格式化测试

#### 积压监控 (2个)
- [ ] `backlog_detector_test.go` - 积压检测器测试
- [ ] `publisher_backlog_detector_test.go` - 发布器积压检测测试

#### 主题配置 (1个)
- [ ] `topic_config_manager_test.go` - 主题配置管理器测试

#### 健康检查 (4个)
- [ ] `health_check_comprehensive_test.go` - 健康检查综合测试
- [ ] `health_check_config_test.go` - 健康检查配置测试
- [ ] `health_check_simple_test.go` - 健康检查简单测试
- [ ] `health_check_deadlock_test.go` - 健康检查死锁测试

#### 集成测试 (3个)
- [ ] `e2e_integration_test.go` - 端到端集成测试
- [ ] `eventbus_integration_test.go` - EventBus 集成测试
- [ ] `health_check_failure_test.go` - 健康检查失败测试

### P1 - 第二批迁移 (15个文件)

#### 配置和工具 (4个)
- [ ] `json_config_test.go` - JSON 配置测试
- [ ] `config_conversion_test.go` - 配置转换测试
- [ ] `extract_aggregate_id_test.go` - 聚合ID提取测试
- [ ] `naming_verification_test.go` - 命名验证测试

#### 健康检查扩展 (4个)
- [ ] `health_checker_advanced_test.go` - 健康检查器高级测试
- [ ] `health_check_message_coverage_test.go` - 健康检查消息覆盖测试
- [ ] `health_check_message_extended_test.go` - 健康检查消息扩展测试
- [ ] `health_check_subscriber_extended_test.go` - 健康检查订阅器扩展测试

#### 覆盖率提升 (3个)
- [ ] `eventbus_health_check_coverage_test.go` - EventBus 健康检查覆盖测试
- [ ] `topic_config_manager_coverage_test.go` - 主题配置覆盖率测试
- [ ] `eventbus_topic_config_coverage_test.go` - EventBus 主题配置覆盖测试

#### 其他功能 (4个)
- [ ] `keyed_worker_pool_test.go` - 键控工作池测试
- [ ] `unified_worker_pool_test.go` - 统一工作池测试
- [ ] `rate_limiter_test.go` - 速率限制器测试
- [ ] `dynamic_subscription_test.go` - 动态订阅测试

---

## 🔧 迁移步骤

### 步骤 1: 环境准备

```bash
# 1. 创建迁移分支
git checkout -b feature/migrate-function-tests

# 2. 确保测试目录存在
mkdir -p tests/eventbus/function_tests

# 3. 备份现有测试
cp -r sdk/pkg/eventbus tests/eventbus/backup_$(date +%Y%m%d)
```

### 步骤 2: 迁移单个测试文件

以 `eventbus_core_test.go` 为例：

```bash
# 1. 复制文件
cp sdk/pkg/eventbus/eventbus_core_test.go tests/eventbus/function_tests/core_test.go

# 2. 编辑文件
# - 修改包名: package eventbus → package function_tests
# - 添加导入: import "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
# - 更新类型引用: EventBus → eventbus.EventBus

# 3. 运行测试
cd tests/eventbus/function_tests
go test -v -run TestEventBusManager_Publish

# 4. 如果测试通过，删除源文件
# rm sdk/pkg/eventbus/eventbus_core_test.go
```

### 步骤 3: 批量迁移脚本

创建 `migrate_tests.sh`:

```bash
#!/bin/bash

# 迁移测试文件的脚本
SOURCE_DIR="sdk/pkg/eventbus"
TARGET_DIR="tests/eventbus/function_tests"

# P0 测试文件列表
P0_FILES=(
    "eventbus_core_test.go"
    "eventbus_types_test.go"
    "factory_test.go"
    "envelope_advanced_test.go"
    "message_formatter_test.go"
    "backlog_detector_test.go"
    "publisher_backlog_detector_test.go"
    "topic_config_manager_test.go"
    "health_check_comprehensive_test.go"
    "health_check_config_test.go"
    "health_check_simple_test.go"
    "health_check_deadlock_test.go"
    "e2e_integration_test.go"
    "eventbus_integration_test.go"
    "health_check_failure_test.go"
)

# 迁移函数
migrate_test() {
    local file=$1
    local source_file="$SOURCE_DIR/$file"
    local target_file="$TARGET_DIR/$file"
    
    echo "迁移: $file"
    
    # 复制文件
    cp "$source_file" "$target_file"
    
    # 修改包名
    sed -i 's/^package eventbus$/package function_tests/' "$target_file"
    
    # 添加导入（如果不存在）
    if ! grep -q 'github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus' "$target_file"; then
        sed -i '/^import (/a\    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"' "$target_file"
    fi
    
    echo "✅ 完成: $file"
}

# 执行迁移
for file in "${P0_FILES[@]}"; do
    migrate_test "$file"
done

echo ""
echo "🎉 迁移完成！"
echo "请运行测试验证: cd $TARGET_DIR && go test -v ./..."
```

### 步骤 4: 验证迁移

```bash
# 1. 运行所有功能测试
cd tests/eventbus/function_tests
go test -v ./...

# 2. 生成覆盖率报告
go test -coverprofile=coverage_migrated.out -covermode=atomic
go tool cover -func=coverage_migrated.out

# 3. 生成 HTML 报告
go tool cover -html=coverage_migrated.out -o coverage_migrated.html

# 4. 对比覆盖率
echo "迁移前覆盖率: 69.8%"
go tool cover -func=coverage_migrated.out | grep total
```

---

## 📝 迁移模板

### 文件头部修改模板

**修改前:**
```go
package eventbus

import (
    "context"
    "testing"
    "time"
)

func TestEventBusManager_Publish(t *testing.T) {
    config := &EventBusConfig{
        Type: EventBusTypeMemory,
    }
    bus := NewEventBus(config)
    // ...
}
```

**修改后:**
```go
package function_tests

import (
    "context"
    "testing"
    "time"
    
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func TestEventBusManager_Publish(t *testing.T) {
    config := &eventbus.EventBusConfig{
        Type: eventbus.EventBusTypeMemory,
    }
    bus := eventbus.NewEventBus(config)
    // ...
}
```

### 类型引用修改规则

| 修改前 | 修改后 |
|--------|--------|
| `EventBus` | `eventbus.EventBus` |
| `EventBusConfig` | `eventbus.EventBusConfig` |
| `EventBusTypeMemory` | `eventbus.EventBusTypeMemory` |
| `EventBusTypeKafka` | `eventbus.EventBusTypeKafka` |
| `EventBusTypeNATS` | `eventbus.EventBusTypeNATS` |
| `Envelope` | `eventbus.Envelope` |
| `MessageHandler` | `eventbus.MessageHandler` |
| `HealthStatus` | `eventbus.HealthStatus` |
| `Metrics` | `eventbus.Metrics` |

---

## ⚠️ 常见问题和解决方案

### 问题 1: 无法访问内部函数

**错误信息:**
```
undefined: someInternalFunction
```

**解决方案:**
- 检查该函数是否为包内部函数（小写开头）
- 如果是内部函数，考虑：
  1. 将该测试保留在源目录
  2. 或者将内部函数改为公开函数
  3. 或者重新设计测试，只测试公开 API

### 问题 2: 测试辅助函数缺失

**错误信息:**
```
undefined: createTestEventBus
```

**解决方案:**
- 将辅助函数复制到 `test_helper.go`
- 或者使用已有的 `NewTestHelper()` 函数

### 问题 3: Mock 对象不可用

**错误信息:**
```
undefined: mockEventBus
```

**解决方案:**
- 重新设计 Mock 策略
- 使用接口和依赖注入
- 或者保留该测试在源目录

### 问题 4: 测试超时

**错误信息:**
```
test timed out after 10m0s
```

**解决方案:**
- 检查测试是否有死锁
- 增加超时时间
- 优化测试逻辑

---

## 📊 迁移进度跟踪

### 第一批 (P0) - 15个文件

| 文件 | 状态 | 测试结果 | 覆盖率 | 备注 |
|------|------|---------|--------|------|
| eventbus_core_test.go | ⏳ 待迁移 | - | - | - |
| eventbus_types_test.go | ⏳ 待迁移 | - | - | - |
| factory_test.go | ⏳ 待迁移 | - | - | - |
| envelope_advanced_test.go | ⏳ 待迁移 | - | - | - |
| message_formatter_test.go | ⏳ 待迁移 | - | - | - |
| backlog_detector_test.go | ⏳ 待迁移 | - | - | - |
| publisher_backlog_detector_test.go | ⏳ 待迁移 | - | - | - |
| topic_config_manager_test.go | ⏳ 待迁移 | - | - | - |
| health_check_comprehensive_test.go | ⏳ 待迁移 | - | - | - |
| health_check_config_test.go | ⏳ 待迁移 | - | - | - |
| health_check_simple_test.go | ⏳ 待迁移 | - | - | - |
| health_check_deadlock_test.go | ⏳ 待迁移 | - | - | - |
| e2e_integration_test.go | ⏳ 待迁移 | - | - | - |
| eventbus_integration_test.go | ⏳ 待迁移 | - | - | - |
| health_check_failure_test.go | ⏳ 待迁移 | - | - | - |

**进度**: 0/15 (0%)

### 第二批 (P1) - 15个文件

**进度**: 0/15 (0%)

---

## ✅ 验收标准

### 1. 测试通过率

- [ ] 所有迁移的测试都能通过
- [ ] 通过率 ≥ 95%

### 2. 覆盖率

- [ ] 功能测试覆盖率 ≥ 75%
- [ ] 相比迁移前提升 ≥ 5%

### 3. 代码质量

- [ ] 所有测试文件遵循命名规范
- [ ] 测试代码清晰易读
- [ ] 没有重复代码

### 4. 文档完整

- [ ] 更新 README.md
- [ ] 更新覆盖率报告
- [ ] 记录迁移过程中的问题和解决方案

---

## 📅 时间计划

### Day 1: P0 迁移 (15个文件)

- **上午** (4小时)
  - 环境准备
  - 迁移核心功能测试 (5个)
  - 迁移积压监控测试 (2个)

- **下午** (4小时)
  - 迁移主题配置测试 (1个)
  - 迁移健康检查测试 (4个)
  - 迁移集成测试 (3个)
  - 运行测试验证

### Day 2: P1 迁移 (15个文件)

- **上午** (4小时)
  - 迁移配置和工具测试 (4个)
  - 迁移健康检查扩展测试 (4个)

- **下午** (4小时)
  - 迁移覆盖率提升测试 (3个)
  - 迁移其他功能测试 (4个)
  - 运行测试验证

### Day 3: 验证和文档

- **上午** (3小时)
  - 生成覆盖率报告
  - 对比迁移前后差异
  - 修复发现的问题

- **下午** (3小时)
  - 更新文档
  - 代码审查
  - 提交 PR

---

## 🎉 完成标准

- [x] 所有 P0 测试迁移完成
- [x] 所有 P1 测试迁移完成
- [x] 测试通过率 ≥ 95%
- [x] 覆盖率 ≥ 75%
- [x] 文档更新完成
- [x] 代码审查通过
- [x] PR 合并

---

**计划创建时间**: 2025-10-14  
**预计完成时间**: 2025-10-17  
**负责人**: 开发团队  
**审核人**: Tech Lead

