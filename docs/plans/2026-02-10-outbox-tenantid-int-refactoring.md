# Outbox TenantID string→int 重构实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 Outbox 模块中的 TenantID 字段从 string 类型统一重构为 int 类型，与 EnterpriseDomainEvent 和 eventbus.Envelope 保持一致。

**Architecture:** 采用分层重构策略，从底层模型到上层接口，确保类型一致性。TDD 方式，每步修改后运行测试验证。

**Tech Stack:** Go 1.23+, GORM, Ginkgo v2 (测试框架)

---

## 前置分析

### 当前状态

**已完成的修改 (✅):**
- `sdk/pkg/eventbus/envelope.go:24` - `TenantID int` ✅
- `sdk/pkg/domain/event/enterprise_domain_event.go` - `TenantId int` ✅
- `sdk/pkg/domain/event/event_interface.go` - `GetTenantId() int / SetTenantId(int)` ✅

**遗漏需要修改的文件 (❌):**
1. `sdk/pkg/outbox/event.go` - OutboxEvent.TenantID string
2. `sdk/pkg/outbox/repository.go` - 接口方法参数 tenantID string
3. `sdk/pkg/outbox/adapters/gorm/model.go` - OutboxEventModel.TenantID string
4. `sdk/pkg/outbox/event_publisher.go` - Envelope.TenantID string
5. `sdk/pkg/outbox/publisher.go` - PublishPendingEvents tenantID string
6. `sdk/pkg/domain/event/README.md` - 示例代码使用 string
7. 测试文件需要同步更新

### 依赖关系图

```
enterprise_domain_event.go (int) ✅
            ↓
    outbox/event.go (string) ❌ ← 需要修改
            ↓
    outbox/model.go (string) ❌ ← 需要修改
            ↓
    outbox/repository.go (string) ❌ ← 需要修改
            ↓
    outbox/publisher.go (string) ❌ ← 需要修改
            ↓
    event_publisher.go (string) ❌ ← 需要修改
            ↓
    eventbus/envelope.go (int) ✅ ← 目标状态
```

---

## Task 1: 修改 OutboxEvent 核心模型

**Files:**
- Modify: `sdk/pkg/outbox/event.go:33`
- Modify: `sdk/pkg/outbox/event.go:98`
- Modify: `sdk/pkg/outbox/event.go:116` (generateIdempotencyKey)

**Step 1: 修改 OutboxEvent 结构体 TenantID 字段类型**

```go
// 修改前 (Line 28-33)
type OutboxEvent struct {
	ID string
	// TenantID 租户 ID（多租户支持）
	TenantID string
	// ...
}

// 修改后
type OutboxEvent struct {
	ID string
	// TenantID 租户 ID（多租户支持）
	TenantID int
	// ...
}
```

**Step 2: 修改 NewOutboxEvent 函数签名**

```go
// 修改前 (Line 97-103)
func NewOutboxEvent(
	tenantID string,
	aggregateID string,
	aggregateType string,
	eventType string,
	payload jxtevent.BaseEvent,
) (*OutboxEvent, error) {

// 修改后
func NewOutboxEvent(
	tenantID int,
	aggregateID string,
	aggregateType string,
	eventType string,
	payload jxtevent.BaseEvent,
) (*OutboxEvent, error) {
```

**Step 3: 修改 generateIdempotencyKey 函数签名和调用**

```go
// 修改前 (Line 351)
func generateIdempotencyKey(tenantID, aggregateType, aggregateID, eventType, eventID string) string {
	return tenantID + ":" + aggregateType + ":" + aggregateID + ":" + eventType + ":" + eventID
}

// 修改后 (需要将 int 转换为 string)
func generateIdempotencyKey(tenantID int, aggregateType, aggregateID, eventType, eventID string) string {
	tenantIDStr := fmt.Sprintf("%d", tenantID)
	return tenantIDStr + ":" + aggregateType + ":" + aggregateID + ":" + eventType + ":" + eventID
}
```

**Step 4: 运行测试验证**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/outbox/... -v
```

预期: 部分测试失败 (因为调用方仍使用 string)

**Step 5: 提交**

```bash
git add sdk/pkg/outbox/event.go
git commit -m "refactor(outbox): change OutboxEvent.TenantID from string to int"
```

---

## Task 2: 修改 GORM 数据库模型

**Files:**
- Modify: `sdk/pkg/outbox/adapters/gorm/model.go:17`

**Step 1: 修改 OutboxEventModel 结构体**

```go
// 修改前 (Line 16-17)
// TenantID 租户 ID
TenantID string `gorm:"type:varchar(36);not null;index:idx_tenant_status;comment:租户ID"`

// 修改后
// TenantID 租户 ID
TenantID int `gorm:"type:int;not null;index:idx_tenant_status;default:0;comment:租户ID"`
```

**Step 2: 运行测试**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/outbox/adapters/gorm/... -v
```

预期: 部分测试失败

**Step 3: 提交**

```bash
git add sdk/pkg/outbox/adapters/gorm/model.go
git commit -m "refactor(outbox): change OutboxEventModel.TenantID from string to int"
```

---

## Task 3: 修改 OutboxRepository 接口

**Files:**
- Modify: `sdk/pkg/outbox/repository.go`

**Step 1: 修改 FindPendingEvents 方法**

```go
// 修改前 (Line 25)
FindPendingEvents(ctx context.Context, limit int, tenantID string) ([]*OutboxEvent, error)

// 修改后
FindPendingEvents(ctx context.Context, limit int, tenantID int) ([]*OutboxEvent, error)
```

**Step 2: 修改 FindPendingEventsWithDelay 方法**

```go
// 修改前 (Line 33)
FindPendingEventsWithDelay(ctx context.Context, tenantID string, delaySeconds int, limit int) ([]*OutboxEvent, error)

// 修改后
FindPendingEventsWithDelay(ctx context.Context, tenantID int, delaySeconds int, limit int) ([]*OutboxEvent, error)
```

**Step 3: 修改 FindByAggregateID 方法**

```go
// 修改前 (Line 56)
FindByAggregateID(ctx context.Context, aggregateID string, tenantID string) ([]*OutboxEvent, error)

// 修改后
FindByAggregateID(ctx context.Context, aggregateID string, tenantID int) ([]*OutboxEvent, error)
```

**Step 4: 修改 DeletePublishedBefore 方法**

```go
// 修改前 (Line 106)
DeletePublishedBefore(ctx context.Context, before time.Time, tenantID string) (int64, error)

// 修改后
DeletePublishedBefore(ctx context.Context, before time.Time, tenantID int) (int64, error)
```

**Step 5: 修改 DeleteFailedBefore 方法**

```go
// 修改前 (Line 112)
DeleteFailedBefore(ctx context.Context, before time.Time, tenantID string) (int64, error)

// 修改后
DeleteFailedBefore(ctx context.Context, before time.Time, tenantID int) (int64, error)
```

**Step 6: 修改 Count 方法**

```go
// 修改前 (Line 118)
Count(ctx context.Context, status EventStatus, tenantID string) (int64, error)

// 修改后
Count(ctx context.Context, status EventStatus, tenantID int) (int64, error)
```

**Step 7: 修改 CountByStatus 方法**

```go
// 修改前 (Line 124)
CountByStatus(ctx context.Context, tenantID string) (map[EventStatus]int64, error)

// 修改后
CountByStatus(ctx context.Context, tenantID int) (map[EventStatus]int64, error)
```

**Step 8: 修改 FindMaxRetryEvents 方法**

```go
// 修改前 (Line 143)
FindMaxRetryEvents(ctx context.Context, limit int, tenantID string) ([]*OutboxEvent, error)

// 修改后
FindMaxRetryEvents(ctx context.Context, limit int, tenantID int) ([]*OutboxEvent, error)
```

**Step 9: 修改 RepositoryStats 结构体**

```go
// 修改前 (Line 169-170)
// TenantID 租户 ID
TenantID string

// 修改后
// TenantID 租户 ID
TenantID int
```

**Step 10: 修改 GetStats 方法**

```go
// 修改前 (Line 179)
GetStats(ctx context.Context, tenantID string) (*RepositoryStats, error)

// 修改后
GetStats(ctx context.Context, tenantID int) (*RepositoryStats, error)
```

**Step 11: 运行测试**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/outbox/... -v
```

预期: 更多测试失败 (GORM adapter 实现也需要修改)

**Step 12: 提交**

```bash
git add sdk/pkg/outbox/repository.go
git commit -m "refactor(outbox): change Repository interface tenantID params from string to int"
```

---

## Task 4: 修改 GORM Adapter 实现

**Files:**
- Modify: `sdk/pkg/outbox/adapters/gorm/repository.go` (需要确认文件路径)

**Step 1: 搜索 GORM adapter 实现**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
find ./sdk/pkg/outbox/adapters/gorm -name "*.go" -type f
```

**Step 2: 根据找到的文件，修改所有实现的接口方法**

将所有方法的 `tenantID string` 参数改为 `tenantID int`，包括:
- FindPendingEvents
- FindPendingEventsWithDelay
- FindByAggregateID
- DeletePublishedBefore
- DeleteFailedBefore
- Count
- CountByStatus
- FindMaxRetryEvents
- GetStats

**Step 3: 修改查询条件中的字符串比较**

```go
// 修改前
if tenantID != "" {
    db = db.Where("tenant_id = ?", tenantID)
}

// 修改后
if tenantID != 0 {
    db = db.Where("tenant_id = ?", tenantID)
}
```

**Step 4: 运行测试**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/outbox/adapters/gorm/... -v
```

**Step 5: 提交**

```bash
git add sdk/pkg/outbox/adapters/gorm/
git commit -m "refactor(outbox/gorm): update GORM adapter for int tenantID"
```

---

## Task 5: 修改 EventPublisher Envelope

**Files:**
- Modify: `sdk/pkg/outbox/event_publisher.go:82`
- Modify: `sdk/pkg/outbox/event_publisher.go:102`

**Step 1: 修改 Envelope 结构体 TenantID 字段**

```go
// 修改前 (Line 81-82)
// TenantID 租户ID（多租户支持，用于Outbox ACK路由）
TenantID string `json:"tenant_id,omitempty"`

// 修改后
// TenantID 租户ID（多租户支持，用于Outbox ACK路由）
TenantID int `json:"tenant_id,omitempty"`
```

**Step 2: 修改 PublishResult 结构体 TenantID 字段**

```go
// 修改前 (Line 101-102)
// TenantID 租户ID（多租户支持，用于Outbox ACK路由）
TenantID string

// 修改后
// TenantID 租户ID（多租户支持，用于Outbox ACK路由）
TenantID int
```

**Step 3: 运行测试**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/outbox/... -v
```

**Step 4: 提交**

```bash
git add sdk/pkg/outbox/event_publisher.go
git commit -m "refactor(outbox): change Envelope.TenantID from string to int"
```

---

## Task 6: 修改 Publisher

**Files:**
- Modify: `sdk/pkg/outbox/publisher.go:369`

**Step 1: 修改 PublishPendingEvents 方法签名**

```go
// 修改前 (Line 367-369)
// PublishPendingEvents 发布所有待发布的事件
func (p *OutboxPublisher) PublishPendingEvents(ctx context.Context, limit int, tenantID string) (int, error) {

// 修改后
// PublishPendingEvents 发布所有待发布的事件
func (p *OutboxPublisher) PublishPendingEvents(ctx context.Context, limit int, tenantID int) (int, error) {
```

**Step 2: 运行测试**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/outbox/... -v
```

**Step 3: 提交**

```bash
git add sdk/pkg/outbox/publisher.go
git commit -m "refactor(outbox): change PublishPendingEvents tenantID param from string to int"
```

---

## Task 7: 修改 README 文档

**Files:**
- Modify: `sdk/pkg/domain/event/README.md:67`
- Modify: `sdk/pkg/domain/event/README.md:224-225`

**Step 1: 修改示例代码中的 SetTenantId 调用**

```go
// 修改前 (Line 66-67)
// 设置租户ID
event.SetTenantId("tenant-001")

// 修改后
// 设置租户ID
event.SetTenantId(1)
```

**Step 2: 修改 EnterpriseEvent 接口文档**

```go
// 修改前 (Line 224-225)
// 租户隔离
GetTenantId() string
SetTenantId(string)

// 修改后
// 租户隔离
GetTenantId() int
SetTenantId(int)
```

**Step 3: 提交**

```bash
git add sdk/pkg/domain/event/README.md
git commit -m "docs(outbox): fix TenantId type documentation (string → int)"
```

---

## Task 8: 修复单元测试

**Files:**
- Modify: `sdk/pkg/outbox/event_test.go` (如果存在)
- Modify: `tests/outbox/function_regression_tests/*.go`

**Step 1: 搜索所有使用 string tenantID 的测试**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
grep -r "NewOutboxEvent.*tenantID" --include="*_test.go" ./tests/outbox/
grep -r "SetTenantId\(" --include="*_test.go" ./tests/outbox/
```

**Step 2: 修改测试代码中的字符串字面量为整数**

```go
// 修改前
event := outbox.NewOutboxEvent("tenant-001", "agg-1", "Test", "test", payload)

// 修改后
event := outbox.NewOutboxEvent(1, "agg-1", "Test", "test", payload)
```

**Step 3: 运行所有测试**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./tests/outbox/... -v
go test ./sdk/pkg/outbox/... -v
```

**Step 4: 提交**

```bash
git add tests/outbox/ sdk/pkg/outbox/
git commit -m "test(outbox): update tests for int tenantID"
```

---

## Task 9: 运行完整测试套件

**Step 1: 运行 jxt-core 所有测试**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./... -v
```

预期: 所有测试通过

**Step 2: 运行 outbox 回归测试**

```bash
cd tests/outbox/function_regression_tests
ginkgo -v
```

预期: 所有测试通过

**Step 3: 提交（如果通过）**

```bash
git add .
git commit -m "test(outbox): all tests pass after TenantID int refactoring"
```

---

## Task 10: 更新重构计划文档

**Files:**
- Modify: `docs/plans/2026-02-10-tenantid-string-to-int-refactoring.md`

**Step 1: 在原计划文档中添加 Outbox 模块部分**

在 "3. 详细设计方案" 中添加新的子章节：

```markdown
### 3.5 Outbox 模块修改

#### 3.5.1 OutboxEvent 修改

**文件**: `sdk/pkg/outbox/event.go`

```go
// 修改前
type OutboxEvent struct {
    TenantID string
}

// 修改后
type OutboxEvent struct {
    TenantID int
}
```

#### 3.5.2 OutboxRepository 接口修改

所有方法的 `tenantID string` 参数改为 `tenantID int`:
- FindPendingEvents
- FindPendingEventsWithDelay
- FindByAggregateID
- DeletePublishedBefore
- DeleteFailedBefore
- Count
- CountByStatus
- FindMaxRetryEvents
- GetStats

#### 3.5.3 EventPublisher Envelope 修改

**文件**: `sdk/pkg/outbox/event_publisher.go`

```go
// 修改前
type Envelope struct {
    TenantID string `json:"tenant_id,omitempty"`
}

// 修改后
type Envelope struct {
    TenantID int `json:"tenant_id,omitempty"`
}
```
```

**Step 2: 更新状态为已完成**

```markdown
**状态**: 已完成 (2026-02-10)
```

**Step 3: 提交**

```bash
git add docs/plans/2026-02-10-tenantid-string-to-int-refactoring.md
git commit -m "docs(plans): update TenantId refactoring plan with Outbox module changes"
```

---

## 验证清单

完成后验证以下项目：

- [ ] `sdk/pkg/outbox/event.go` - OutboxEvent.TenantID 是 int
- [ ] `sdk/pkg/outbox/repository.go` - 所有接口方法 tenantID 参数是 int
- [ ] `sdk/pkg/outbox/adapters/gorm/model.go` - OutboxEventModel.TenantID 是 int
- [ ] `sdk/pkg/outbox/adapters/gorm/repository.go` - 实现方法 tenantID 参数是 int
- [ ] `sdk/pkg/outbox/event_publisher.go` - Envelope.TenantID 是 int
- [ ] `sdk/pkg/outbox/publisher.go` - PublishPendingEvents tenantID 参数是 int
- [ ] `sdk/pkg/domain/event/README.md` - 示例代码使用 int 类型
- [ ] 所有单元测试通过
- [ ] 所有回归测试通过
- [ ] eventbus.Envelope.TenantID (int) 与 outbox.Envelope.TenantID (int) 类型一致

---

## 回滚方案

如果出现问题：

```bash
# 回滚到重构前的状态
git revert <commit-hash>...

# 或者重置到指定提交
git reset --hard <pre-refactoring-commit>
```

---

## 相关文档

- [原重构计划](./2026-02-10-tenantid-string-to-int-refactoring.md)
- [Outbox Pattern 文档](../outbox/README.md)
- [Domain Event 文档](../../sdk/pkg/domain/event/README.md)
