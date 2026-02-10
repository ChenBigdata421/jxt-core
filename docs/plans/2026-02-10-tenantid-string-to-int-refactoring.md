# EnterpriseDomainEvent.TenantId 从 string 重构为 int 方案

**日期**: 2026-02-10
**作者**: Claude Code
**状态**: 设计阶段

## 1. 背景分析

### 1.1 当前问题

当前系统中 `TenantId` 存在类型不一致问题：

| 层级 | 类型 | 说明 |
|------|------|------|
| 租户中间件 (`sdk/pkg/tenant/middleware`) | `int` | 从请求头提取后转换为 int 存储 |
| 租户提供者 (`sdk/pkg/tenant/provider`) | `int` | 所有内部数据结构使用 int |
| 领域事件 (`EnterpriseDomainEvent`) | `string` | **类型不一致** |
| 上下文存储 (`global.TenantIDKey`) | `int` | 使用 jxt-core 中间件存储为 int |

这导致所有服务都需要进行类型转换：

```go
// 当前代码模式 - 到处都是类型转换
tenantID := ctx.Value(global.TenantIDKey)
if tenantID == nil {
    return error
}
domainEvent.SetTenantId(tenantID.(string)) // 类型断言 + 转换

// 实际存储的是 int，但事件期望 string
```

### 1.2 影响范围统计

通过代码扫描，以下文件需要修改：

**jxt-core (基础库):**
- `sdk/pkg/domain/event/enterprise_domain_event.go`
- `sdk/pkg/domain/event/event_interface.go`
- `sdk/pkg/domain/event/enterprise_domain_event_test.go`
- 测试目录下的相关测试文件

**evidence-management/command (写服务):**
~28 处 `SetTenantId(tenantID.(string))` 调用

**evidence-management/query (读服务):**
~10+ 个事件处理器中可能涉及 TenantId 读取

**file-storage-service:**
事件构建和发布逻辑

## 2. 重构目标

1. **类型统一**: `TenantId` 统一使用 `int` 类型
2. **消除转换**: 移除所有不必要的类型转换代码
3. **保持兼容**: 考虑 JSON 序列化的向后兼容性
4. **默认值处理**: 定义合理的通配符/默认值

## 3. 详细设计方案

### 3.1 核心类型修改

#### 3.1.1 EnterpriseEvent 接口修改

**文件**: `sdk/pkg/domain/event/event_interface.go`

```go
// 修改前
type EnterpriseEvent interface {
    BaseEvent
    GetTenantId() string
    SetTenantId(string)
    // ... 其他方法
}

// 修改后
type EnterpriseEvent interface {
    BaseEvent
    GetTenantId() int
    SetTenantId(int)
    // ... 其他方法
}
```

#### 3.1.2 EnterpriseDomainEvent 结构修改

**文件**: `sdk/pkg/domain/event/enterprise_domain_event.go`

```go
// 修改前
type EnterpriseDomainEvent struct {
    BaseDomainEvent
    TenantId string `json:"tenantId" gorm:"type:varchar(255);index;column:tenant_id;comment:租户ID"`
    // ... 其他字段
}

// 修改后
type EnterpriseDomainEvent struct {
    BaseDomainEvent
    TenantId int `json:"tenantId" gorm:"type:int;index;column:tenant_id;comment:租户ID"`
    // ... 其他字段
}
```

#### 3.1.3 方法实现修改

```go
// 修改前
func (e *EnterpriseDomainEvent) GetTenantId() string   { return e.TenantId }
func (e *EnterpriseDomainEvent) SetTenantId(id string) { e.TenantId = id }

// 修改后
func (e *EnterpriseDomainEvent) GetTenantId() int   { return e.TenantId }
func (e *EnterpriseDomainEvent) SetTenantId(id int) { e.TenantId = id }
```

### 3.2 默认值处理策略

| 场景 | 修改前 | 修改后 | 说明 |
|------|--------|--------|------|
| 单租户/系统级事件 | `"*"` | `0` | 0 表示无特定租户 |
| 多租户事件 | `"tenant-001"` | `1` | 直接使用租户 ID |
| 无效租户 | `""` | `0` | 统一使用 0 |

```go
// NewEnterpriseDomainEvent 修改
func NewEnterpriseDomainEvent(eventType string, aggregateID interface{}, aggregateType string, payload interface{}) *EnterpriseDomainEvent {
    return &EnterpriseDomainEvent{
        BaseDomainEvent: BaseDomainEvent{
            EventID:       uuid.Must(uuid.NewV7()).String(),
            EventType:     eventType,
            OccurredAt:    time.Now(),
            Version:       1,
            AggregateID:   convertAggregateIDToString(aggregateID),
            AggregateType: aggregateType,
            Payload:       payload,
        },
        TenantId: 0, // 默认为 0，由业务层设置实际租户ID
    }
}
```

### 3.3 服务层修改模板

#### 3.3.1 evidence-management/command 修改

所有 `SetTenantId` 调用从类型断言改为直接赋值：

```go
// 修改前 - evidence-management/command/internal/application/service/*.go
tenantID := ctx.Value(global.TenantIDKey)
if tenantID == nil {
    return error
}
domainEvent.SetTenantId(tenantID.(string)) // 类型断言

// 修改后
tenantID := ctx.Value(global.TenantIDKey)
if tenantID == nil {
    return error
}
domainEvent.SetTenantId(tenantID.(int)) // 直接使用 int
```

**影响文件列表** (evidence-management/command):
- `internal/application/service/media.go`
- `internal/application/service/writ_media_relation.go`
- `internal/application/service/writ.go`
- `internal/application/service/case_media_relation.go`
- `internal/application/service/incident_record.go`
- `internal/application/service/incident_record_media_relation.go`
- `internal/application/service/incident_media_info.go`
- `internal/application/service/archive_media_relation.go`
- `internal/application/service/case.go`
- `internal/application/service/archive.go`
- `internal/application/service/evidence_media.go`
- `internal/application/service/evidence_media_source.go`

#### 3.3.2 evidence-management/query 修改

事件处理器中如果使用 `GetTenantId()`，需要确保类型匹配：

```go
// 通常 query 端不需要修改，因为只是消费事件
// 但如果有业务逻辑依赖 TenantId，需要确保逻辑正确
func (h *MediaEventHandler) handleFileUploaded(ctx context.Context, env *eventbus.Envelope) error {
    // 反序列化后，TenantId 已经是 int
    var enterpriseEvent jxtevent.EnterpriseDomainEvent
    if err := json.Unmarshal(env.Message, &enterpriseEvent); err != nil {
        return err
    }

    // 直接使用 int 类型的 TenantId
    tenantID := enterpriseEvent.GetTenantId()
    // ... 业务逻辑
}
```

#### 3.3.3 file-storage-service 修改

```go
// file-storage-service/internal/application/service/*.go
// 如果事件构建时设置 TenantId，需要修改
```

### 3.4 测试修改

**文件**: `sdk/pkg/domain/event/enterprise_domain_event_test.go`

```go
// 修改前
func TestEnterpriseDomainEvent_TenantId(t *testing.T) {
    event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)
    assert.Equal(t, "*", event.GetTenantId()) // 默认值 "*"

    event.SetTenantId("tenant-001")
    assert.Equal(t, "tenant-001", event.GetTenantId())
}

// 修改后
func TestEnterpriseDomainEvent_TenantId(t *testing.T) {
    event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)
    assert.Equal(t, 0, event.GetTenantId()) // 默认值 0

    event.SetTenantId(1)
    assert.Equal(t, int(1), event.GetTenantId())
}
```

## 4. JSON 序列化兼容性分析

### 4.1 JSON 格式变化

```json
// 修改前
{
  "eventId": "...",
  "eventType": "FileUploaded",
  "tenantId": "1",
  ...
}

// 修改后
{
  "eventId": "...",
  "eventType": "FileUploaded",
  "tenantId": 1,
  ...
}
```

### 4.2 兼容性策略

**方案 A: 硬性切换** (推荐)
- 直接修改类型，JSON 格式变更
- 优势: 简单直接，类型安全
- 风险: 已有队列中的旧格式事件会反序列化失败
- 缓解: 确保所有服务同时部署，队列中的旧事件先消费完

**方案 B: 兼容层**
- 保持 `TenantId` 为 `string`，内部使用辅助方法转换
- 优势: 完全向后兼容
- 劣势: 仍然需要类型转换，违背重构初衷

**建议**: 采用方案 A，配合以下部署策略：
1. 确保消息队列为空或积压较少
2. 先升级 jxt-core
3. 同时升级所有微服务
4. 监控事件消费错误

## 5. 实施计划

### 5.1 阶段划分

| 阶段 | 任务 | 预估影响 |
|------|------|----------|
| **Phase 1** | jxt-core 基础库修改 | 无运行时影响 |
| **Phase 2** | 单元测试修改 | 确保测试通过 |
| **Phase 3** | evidence-management/command 修改 | 写服务变更 |
| **Phase 4** | evidence-management/query 修改 | 读服务变更 |
| **Phase 5** | file-storage-service 修改 | 文件服务变更 |
| **Phase 6** | 集成测试验证 | 端到端验证 |

### 5.2 详细步骤

#### Phase 1: jxt-core 修改

1. 修改 `sdk/pkg/domain/event/event_interface.go`
   - 更新 `EnterpriseEvent` 接口方法签名

2. 修改 `sdk/pkg/domain/event/enterprise_domain_event.go`
   - 更新 `TenantId` 字段类型
   - 更新 GORM 标签
   - 更新 getter/setter 方法
   - 更新 `NewEnterpriseDomainEvent` 默认值

3. 运行 jxt-core 单元测试

#### Phase 2: 测试修改

1. `sdk/pkg/domain/event/enterprise_domain_event_test.go`
2. `tests/domain/event/function_regression_tests/*.go`

#### Phase 3: evidence-management/command 修改

批量替换所有 `SetTenantId` 调用：

```bash
# 在 evidence-management/command 目录执行
# 搜索模式: SetTenantId\(tenantID\.\(string\)\)
# 替换为: SetTenantId(tenantID.(int))
```

#### Phase 4: evidence-management/query 修改

检查所有事件处理器，确保 TenantId 使用正确。

#### Phase 5: file-storage-service 修改

类似 Phase 3 的模式。

#### Phase 6: 验证

1. 单元测试
2. 集成测试
3. 端到端事件流测试

### 5.3 回滚方案

如果部署后发现问题：
1. 回滚所有微服务到之前版本
2. 回滚 jxt-core 到之前版本
3. 消费掉队列中的新格式事件（作为错误处理）

## 6. 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| JSON 格式不兼容 | 旧事件反序列化失败 | 确保队列清空后再部署 |
| 数据库迁移 | Outbox 表中的旧数据 | 添加数据库迁移脚本 |
| 微服务部署顺序 | 部分服务用旧格式 | 所有服务同步部署 |
| 第三方集成 | 外部系统依赖事件格式 | 通知相关方 |

## 7. 数据库迁移考虑

如果 Outbox 表中有持久化的事件数据：

```sql
-- MySQL 迁移脚本 (如果需要)
-- 注意：varchar 到 int 的迁移需要确保数据都是数字
ALTER TABLE outbox MODIFY COLUMN tenant_id INT NOT NULL DEFAULT 0;

-- 数据清洗 (如果有 "*", "" 等非数字值)
UPDATE outbox SET tenant_id = 0 WHERE tenant_id NOT REGEXP '^[0-9]+$';
```

## 8. 验证清单

- [ ] jxt-core 所有测试通过
- [ ] evidence-management/command 所有测试通过
- [ ] evidence-management/query 所有测试通过
- [ ] file-storage-service 所有测试通过
- [ ] 集成测试验证事件发布和消费
- [ ] 检查 Outbox 表数据兼容性
- [ ] 检查消息队列兼容性
- [ ] 性能测试验证无退化

## 9. 附录

### 9.1 搜索命令

```bash
# 查找所有 SetTenantId 调用
grep -r "SetTenantId" --include="*.go" .

# 查找所有 GetTenantId 调用
grep -r "GetTenantId" --include="*.go" .

# 查找所有 tenantID.(string) 类型断言
grep -r "tenantID\.\(string\)" --include="*.go" .
```

### 9.2 相关文档

- [jxt-core Event 文档](../../sdk/pkg/domain/event/README.md)
- [租户中间件文档](../../sdk/pkg/tenant/middleware/extract_tenant_id.go)
- [租户提供者文档](../../sdk/pkg/tenant/README.md)
