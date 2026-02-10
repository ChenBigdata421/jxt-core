# TenantId String to Int Refactoring - Phase 1 & 2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Convert `EnterpriseDomainEvent.TenantId` and `Envelope.TenantID` from `string` to `int` in jxt-core library and update all related tests.

**Architecture:** This is a breaking change that affects:
1. Domain event types (`EnterpriseDomainEvent.TenantId`)
2. EventBus types (`Envelope.TenantID`)
3. Event interface methods (`EnterpriseEvent.GetTenantId/SetTenantId`)
4. All serialization/deserialization behavior

**Tech Stack:** Go 1.24+, GORM, jsoniter, testify

**Related Documents:**
- Design: [2026-02-10-tenantid-string-to-int-refactoring.md](./2026-02-10-tenantid-string-to-int-refactoring.md)

---

## Task 1: Update EnterpriseEvent Interface

**Files:**
- Modify: `sdk/pkg/domain/event/event_interface.go`

**Step 1: Read the current interface**

Run: `cat sdk/pkg/domain/event/event_interface.go`
Expected: See `GetTenantId() string` and `SetTenantId(string)` in `EnterpriseEvent` interface

**Step 2: Update the interface methods to use int**

Edit lines 23-24 in `sdk/pkg/domain/event/event_interface.go`:

```go
// BEFORE:
type EnterpriseEvent interface {
	BaseEvent

	// 租户隔离
	GetTenantId() string
	SetTenantId(string)

	// 可观测性方法
	GetCorrelationId() string
	SetCorrelationId(string)
	GetCausationId() string
	SetCausationId(string)
	GetTraceId() string
	SetTraceId(string)
}

// AFTER:
type EnterpriseEvent interface {
	BaseEvent

	// 租户隔离
	GetTenantId() int
	SetTenantId(int)

	// 可观测性方法
	GetCorrelationId() string
	SetCorrelationId(string)
	GetCausationId() string
	SetCausationId(string)
	GetTraceId() string
	SetTraceId(string)
}
```

**Step 3: Verify the change**

Run: `grep -A 5 "type EnterpriseEvent interface" sdk/pkg/domain/event/event_interface.go`
Expected: Should see `GetTenantId() int` and `SetTenantId(int)`

**Step 4: Commit**

```bash
git add sdk/pkg/domain/event/event_interface.go
git commit -m "refactor(event): update EnterpriseEvent interface to use int for TenantId"
```

---

## Task 2: Update EnterpriseDomainEvent Struct and Methods

**Files:**
- Modify: `sdk/pkg/domain/event/enterprise_domain_event.go`

**Step 1: Read the current struct**

Run: `cat sdk/pkg/domain/event/enterprise_domain_event.go`
Expected: See `TenantId string` with gorm tag `type:varchar(255)`

**Step 2: Update TenantId field type and GORM tag**

Edit line 19 in `sdk/pkg/domain/event/enterprise_domain_event.go`:

```go
// BEFORE:
// TenantId string `json:"tenantId" gorm:"type:varchar(255);index;column:tenant_id;comment:租户ID"`

// AFTER:
TenantId int `json:"tenantId" gorm:"type:int;index;column:tenant_id;comment:租户ID"`
```

**Step 3: Update getter/setter method signatures**

Edit lines 48-49 in `sdk/pkg/domain/event/enterprise_domain_event.go`:

```go
// BEFORE:
func (e *EnterpriseDomainEvent) GetTenantId() string   { return e.TenantId }
func (e *EnterpriseDomainEvent) SetTenantId(id string) { e.TenantId = id }

// AFTER:
func (e *EnterpriseDomainEvent) GetTenantId() int   { return e.TenantId }
func (e *EnterpriseDomainEvent) SetTenantId(id int) { e.TenantId = id }
```

**Step 4: Update NewEnterpriseDomainEvent default value**

Edit line 43 in `sdk/pkg/domain/event/enterprise_domain_event.go`:

```go
// BEFORE:
TenantId: "*", // 默认使用通配符，由业务层根据需要设置实际租户ID

// AFTER:
TenantId: 0, // 默认为 0，由业务层根据需要设置实际租户ID
```

**Step 5: Verify the changes**

Run: `grep -n "TenantId" sdk/pkg/domain/event/enterprise_domain_event.go | head -20`
Expected: All occurrences should use `int` type

**Step 6: Commit**

```bash
git add sdk/pkg/domain/event/enterprise_domain_event.go
git commit -m "refactor(event): update EnterpriseDomainEvent.TenantId to int type"
```

---

## Task 3: Update Envelope TenantID Field

**Files:**
- Modify: `sdk/pkg/eventbus/envelope.go`

**Step 1: Read the current Envelope struct**

Run: `grep -n "TenantID" sdk/pkg/eventbus/envelope.go | head -5`
Expected: Line 24 has `TenantID string` with json tag `tenant_id,omitempty`

**Step 2: Update TenantID field type**

Edit line 24 in `sdk/pkg/eventbus/envelope.go`:

```go
// BEFORE:
TenantID string `json:"tenant_id,omitempty"` // 租户ID（多租户支持，用于Outbox ACK路由）

// AFTER:
TenantID int `json:"tenant_id,omitempty"` // 租户ID（多租户支持，用于Outbox ACK路由）
```

**Step 3: Verify the change**

Run: `grep -n "TenantID" sdk/pkg/eventbus/envelope.go`
Expected: Should only show field definition (no other changes needed)

**Step 4: Commit**

```bash
git add sdk/pkg/eventbus/envelope.go
git commit -m "refactor(eventbus): update Envelope.TenantID to int type"
```

---

## Task 4: Update Test Helper CreateEnvelope Signature

**Files:**
- Modify: `tests/domain/event/function_regression_tests/test_helper.go`

**Step 1: Find the CreateEnvelope function**

Run: `grep -n "CreateEnvelope" tests/domain/event/function_regression_tests/test_helper.go`
Expected: Line 167 has `func (h *TestHelper) CreateEnvelope(eventType, aggregateID, tenantID string, ...)`

**Step 2: Update CreateEnvelope parameter type**

Edit lines 167-174 in `tests/domain/event/function_regression_tests/test_helper.go`:

```go
// BEFORE:
// CreateEnvelope 创建测试用的 Envelope
func (h *TestHelper) CreateEnvelope(eventType, aggregateID, tenantID string, payload []byte) *jxtevent.Envelope {
	return &jxtevent.Envelope{
		EventType:   eventType,
		AggregateID: aggregateID,
		TenantID:    tenantID,
		Payload:     payload,
	}
}

// AFTER:
// CreateEnvelope 创建测试用的 Envelope
func (h *TestHelper) CreateEnvelope(eventType, aggregateID string, tenantID int, payload []byte) *jxtevent.Envelope {
	return &jxtevent.Envelope{
		EventType:   eventType,
		AggregateID: aggregateID,
		TenantID:    tenantID,
		Payload:     payload,
	}
}
```

**Step 3: Commit**

```bash
git add tests/domain/event/function_regression_tests/test_helper.go
git commit -m "refactor(tests): update CreateEnvelope to use int for tenantID"
```

---

## Task 5: Update enterprise_domain_event_test.go

**Files:**
- Modify: `sdk/pkg/domain/event/enterprise_domain_event_test.go`

**Step 1: Read current test file**

Run: `cat sdk/pkg/domain/event/enterprise_domain_event_test.go`
Expected: See tests using string literals like `"*"` and `"tenant-001"`

**Step 2: Update TestNewEnterpriseDomainEvent default value check**

Edit line 32 in `sdk/pkg/domain/event/enterprise_domain_event_test.go`:

```go
// BEFORE:
assert.Equal(t, "*", event.TenantId, "Default TenantId should be '*'")

// AFTER:
assert.Equal(t, 0, event.TenantId, "Default TenantId should be 0")
```

**Step 3: Update TestEnterpriseDomainEvent_TenantId**

Edit lines 38-46 in `sdk/pkg/domain/event/enterprise_domain_event_test.go`:

```go
// BEFORE:
func TestEnterpriseDomainEvent_TenantId(t *testing.T) {
	event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	// 测试默认值
	assert.Equal(t, "*", event.GetTenantId())

	// 测试设置租户ID
	event.SetTenantId("tenant-001")
	assert.Equal(t, "tenant-001", event.GetTenantId())
}

// AFTER:
func TestEnterpriseDomainEvent_TenantId(t *testing.T) {
	event := NewEnterpriseDomainEvent("TestEvent", "test-123", "TestAggregate", nil)

	// 测试默认值
	assert.Equal(t, 0, event.GetTenantId())

	// 测试设置租户ID
	event.SetTenantId(1)
	assert.Equal(t, int(1), event.GetTenantId())
}
```

**Step 4: Update TestEnterpriseDomainEvent_CompleteWorkflow**

Edit line 88 and line 99 in `sdk/pkg/domain/event/enterprise_domain_event_test.go`:

```go
// BEFORE:
event.SetTenantId("tenant-001")
// ...
assert.Equal(t, "tenant-001", event.GetTenantId())

// AFTER:
event.SetTenantId(1)
// ...
assert.Equal(t, 1, event.GetTenantId())
```

**Step 5: Run tests to verify**

Run: `go test -v ./sdk/pkg/domain/event/...`
Expected: All tests in `enterprise_domain_event_test.go` should PASS

**Step 6: Commit**

```bash
git add sdk/pkg/domain/event/enterprise_domain_event_test.go
git commit -m "refactor(tests): update enterprise_domain_event_test.go for int TenantId"
```

---

## Task 6: Update enterprise_serialization_test.go - Basic Tests

**Files:**
- Modify: `tests/domain/event/function_regression_tests/enterprise_serialization_test.go`

**Step 1: Find all SetTenantId calls with string literals**

Run: `grep -n 'SetTenantId("' tests/domain/event/function_regression_tests/enterprise_serialization_test.go`
Expected: Lines 25, 48, 71, 98, etc.

**Step 2: Update TestEnterpriseDomainEvent_BasicSerialization**

Edit lines 25, 38 in `tests/domain/event/function_regression_tests/enterprise_serialization_test.go`:

```go
// BEFORE:
event.SetTenantId("tenant-001")
// ...
helper.AssertEqual("tenant-001", jsonMap["tenantId"], "TenantId should be serialized")

// AFTER:
event.SetTenantId(1)
// ...
helper.AssertEqual(float64(1), jsonMap["tenantId"], "TenantId should be serialized as number")
// Note: JSON numbers are unmarshaled as float64 in Go
```

**Step 3: Update TestEnterpriseDomainEvent_BasicDeserialization**

Edit lines 48, 62 in `tests/domain/event/function_regression_tests/enterprise_serialization_test.go`:

```go
// BEFORE:
originalEvent.SetTenantId("tenant-001")
// ...
helper.AssertEqual(originalEvent.GetTenantId(), deserializedEvent.GetTenantId(), "TenantId should match")

// AFTER:
originalEvent.SetTenantId(1)
// ...
helper.AssertEqual(originalEvent.GetTenantId(), deserializedEvent.GetTenantId(), "TenantId should match")
```

**Step 4: Update TestEnterpriseDomainEvent_AllEnterpriseFieldsSerialization**

Edit lines 71, 85 in `tests/domain/event/function_regression_tests/enterprise_serialization_test.go`:

```go
// BEFORE:
event.SetTenantId("tenant-acme")
// ...
helper.AssertEqual("tenant-acme", deserializedEvent.GetTenantId(), "TenantId should match")

// AFTER:
event.SetTenantId(1)
// ...
helper.AssertEqual(1, deserializedEvent.GetTenantId(), "TenantId should match")
```

**Step 5: Update TestEnterpriseDomainEvent_PayloadSerialization**

Edit line 98 in `tests/domain/event/function_regression_tests/enterprise_serialization_test.go`:

```go
// BEFORE:
event.SetTenantId("tenant-001")

// AFTER:
event.SetTenantId(1)
```

**Step 6: Commit**

```bash
git add tests/domain/event/function_regression_tests/enterprise_serialization_test.go
git commit -m "refactor(tests): update enterprise_serialization_test.go part 1 - basic tests"
```

---

## Task 7: Update enterprise_serialization_test.go - Advanced Tests

**Files:**
- Modify: `tests/domain/event/function_regression_tests/enterprise_serialization_test.go`

**Step 1: Find remaining SetTenantId calls**

Run: `grep -n 'SetTenantId("' tests/domain/event/function_regression_tests/enterprise_serialization_test.go`
Expected: More occurrences in advanced tests

**Step 2: Update TestEnterpriseDomainEvent_ConcurrentSerialization**

Edit lines 133, 153 in `tests/domain/event/function_regression_tests/enterprise_serialization_test.go`:

```go
// BEFORE:
event.SetTenantId("tenant-concurrent-1")
// ...
helper.AssertEqual("tenant-concurrent-1", deserialized.GetTenantId(), "TenantId should match")

// AFTER:
event.SetTenantId(1)
// ...
helper.AssertEqual(1, deserialized.GetTenantId(), "TenantId should match")
```

**Step 3: Update TestEnterpriseDomainEvent_EmptyTenantId**

This test needs special handling - empty string becomes 0

Edit the entire test (lines 162-179):

```go
// BEFORE:
func TestEnterpriseDomainEvent_EmptyTenantId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId("") // 设置空字符串

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证空租户ID
	helper.AssertEqual("", deserializedEvent.GetTenantId(), "Empty TenantId should be preserved")
}

// AFTER:
func TestEnterpriseDomainEvent_ZeroTenantId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId(0) // 设置为 0

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证零值租户ID
	helper.AssertEqual(0, deserializedEvent.GetTenantId(), "Zero TenantId should be preserved")
}
```

**Step 4: Update TestEnterpriseDomainEvent_WildcardTenantId**

This test also needs special handling - wildcard becomes 0

Edit the entire test (lines 182-199):

```go
// BEFORE:
func TestEnterpriseDomainEvent_WildcardTenantId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId("*") // 通配符租户ID

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证通配符租户ID
	helper.AssertEqual("*", deserializedEvent.GetTenantId(), "Wildcard TenantId should be preserved")
}

// AFTER:
func TestEnterpriseDomainEvent_SystemTenantId(t *testing.T) {
	helper := NewTestHelper(t)

	event := helper.CreateEnterpriseDomainEvent("Test.Event", "test-123", "Test", nil)
	event.SetTenantId(0) // 0 表示系统级/无租户

	// 序列化
	bytes, err := jxtevent.MarshalDomainEvent(event)
	helper.AssertNoError(err, "MarshalDomainEvent should succeed")

	// 反序列化
	deserializedEvent, err := jxtevent.UnmarshalDomainEvent[*jxtevent.EnterpriseDomainEvent](bytes)
	helper.AssertNoError(err, "UnmarshalDomainEvent should succeed")

	// 验证系统级租户ID (0)
	helper.AssertEqual(0, deserializedEvent.GetTenantId(), "System TenantId (0) should be preserved")
}
```

**Step 5: Update TestEnterpriseDomainEvent_LargeTenantId**

Edit lines 210-226:

```go
// BEFORE:
event.SetTenantId("tenant-999999")
// ...
helper.AssertEqual("tenant-999999", deserializedEvent.GetTenantId(), "Large TenantId should match")

// AFTER:
event.SetTenantId(999999)
// ...
helper.AssertEqual(999999, deserializedEvent.GetTenantId(), "Large TenantId should match")
```

**Step 6: Commit**

```bash
git add tests/domain/event/function_regression_tests/enterprise_serialization_test.go
git commit -m "refactor(tests): update enterprise_serialization_test.go part 2 - advanced tests"
```

---

## Task 8: Update enterprise_domain_event_test.go in function_regression_tests

**Files:**
- Modify: `tests/domain/event/function_regression_tests/enterprise_domain_event_test.go`

**Step 1: Read the test file**

Run: `cat tests/domain/event/function_regression_tests/enterprise_domain_event_test.go`
Expected: See similar patterns to the sdk test file

**Step 2: Find all SetTenantId calls**

Run: `grep -n 'SetTenantId("' tests/domain/event/function_regression_tests/enterprise_domain_event_test.go`

**Step 3: Update all SetTenantId calls**

Replace all string tenant IDs with int values:

| Line | Before | After |
|------|--------|-------|
| SetTenantId calls | `"tenant-001"`, `"*"` | `1`, `0` |
| AssertEqual checks | string literals | int literals |

**Step 4: Run the specific test file**

Run: `go test -v ./tests/domain/event/function_regression_tests/... -run TestEnterpriseDomainEvent`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add tests/domain/event/function_regression_tests/enterprise_domain_event_test.go
git commit -m "refactor(tests): update enterprise_domain_event_test.go in function_regression_tests"
```

---

## Task 9: Update json_serialization_test.go

**Files:**
- Modify: `tests/domain/event/function_regression_tests/json_serialization_test.go`

**Step 1: Find EnterpriseDomainEvent usage**

Run: `grep -n "EnterpriseDomainEvent\|SetTenantId" tests/domain/event/function_regression_tests/json_serialization_test.go`

**Step 2: Update any SetTenantId calls**

Replace string tenant IDs with int values

**Step 3: Run the test**

Run: `go test -v ./tests/domain/event/function_regression_tests/... -run JsonSerialization`
Expected: Tests PASS

**Step 4: Commit**

```bash
git add tests/domain/event/function_regression_tests/json_serialization_test.go
git commit -m "refactor(tests): update json_serialization_test.go for int TenantId"
```

---

## Task 10: Update integration_test.go

**Files:**
- Modify: `tests/domain/event/function_regression_tests/integration_test.go`

**Step 1: Find EnterpriseDomainEvent usage**

Run: `grep -n "EnterpriseDomainEvent\|SetTenantId" tests/domain/event/function_regression_tests/integration_test.go`

**Step 2: Update any SetTenantId calls**

Replace string tenant IDs with int values

**Step 3: Run the test**

Run: `go test -v ./tests/domain/event/function_regression_tests/... -run Integration`
Expected: Tests PASS

**Step 4: Commit**

```bash
git add tests/domain/event/function_regression_tests/integration_test.go
git commit -m "refactor(tests): update integration_test.go for int TenantId"
```

---

## Task 11: Update validation_test.go

**Files:**
- Modify: `tests/domain/event/function_regression_tests/validation_test.go`

**Step 1: Find EnterpriseDomainEvent usage**

Run: `grep -n "EnterpriseDomainEvent\|SetTenantId" tests/domain/event/function_regression_tests/validation_test.go`

**Step 2: Update any SetTenantid calls**

Replace string tenant IDs with int values

**Step 3: Run the test**

Run: `go test -v ./tests/domain/event/function_regression_tests/... -run Validation`
Expected: Tests PASS

**Step 4: Commit**

```bash
git add tests/domain/event/function_regression_tests/validation_test.go
git commit -m "refactor(tests): update validation_test.go for int TenantId"
```

---

## Task 12: Update payload_helper_test.go

**Files:**
- Modify: `tests/domain/event/function_regression_tests/payload_helper_test.go`

**Step 1: Find EnterpriseDomainEvent usage**

Run: `grep -n "EnterpriseDomainEvent\|SetTenantId" tests/domain/event/function_regression_tests/payload_helper_test.go`

**Step 2: Update any SetTenantId calls**

Replace string tenant IDs with int values

**Step 3: Run the test**

Run: `go test -v ./tests/domain/event/function_regression_tests/... -run Payload`
Expected: Tests PASS

**Step 4: Commit**

```bash
git add tests/domain/event/function_regression_tests/payload_helper_test.go
git commit -m "refactor(tests): update payload_helper_test.go for int TenantId"
```

---

## Task 13: Run All jxt-core Tests

**Step 1: Run all tests in sdk/pkg/domain/event**

Run: `go test -v ./sdk/pkg/domain/event/...`
Expected: All tests PASS

**Step 2: Run all tests in tests/domain/event**

Run: `go test -v ./tests/domain/event/...`
Expected: All tests PASS

**Step 3: Run all tests in sdk/pkg/eventbus**

Run: `go test -v ./sdk/pkg/eventbus/...`
Expected: All tests PASS (Envelope changes shouldn't break existing tests)

**Step 4: Run all jxt-core tests**

Run: `go test ./...`
Expected: All tests PASS

**Step 5: Commit final state**

```bash
git add .
git commit -m "refactor: complete Phase 1&2 - TenantId string to int conversion in jxt-core"
```

---

## Task 14: Update Documentation

**Files:**
- Modify: `sdk/pkg/domain/event/README.md`
- Modify: `sdk/pkg/eventbus/README.md`

**Step 1: Update domain event README**

Add a note about the TenantId type change in `sdk/pkg/domain/event/README.md`:

```markdown
## TenantId Type

**Note:** As of 2026-02-10, `TenantId` is of type `int`:
- `0` represents system-level/no tenant
- Positive integers represent specific tenant IDs
- This aligns with the tenant middleware which extracts tenant IDs as integers
```

**Step 2: Update eventbus README**

Add similar note for Envelope.TenantID in `sdk/pkg/eventbus/README.md`

**Step 3: Commit documentation**

```bash
git add sdk/pkg/domain/event/README.md sdk/pkg/eventbus/README.md
git commit -m "docs: update README for TenantId int type change"
```

---

## Verification Checklist

After completing all tasks:

- [ ] All jxt-core tests pass: `go test ./...`
- [ ] No compilation errors in jxt-core
- [ ] Documentation updated
- [ ] Git commits follow conventional commit format
- [ ] Ready for Phase 3 (microservice modifications)

---

## Important Notes

1. **JSON Serialization**: The JSON format of `tenantId` changes from `"1"` to `1` (string to number)
2. **Default Value**: Changed from `"*"` to `0` for system-level/no-tenant events
3. **Breaking Change**: This is a breaking change - all consuming services must be updated together
4. **Next Phases**: Phase 3+ will update the microservices that consume these types

---

## Rollback Plan

If issues arise:

```bash
# Revert all commits
git log --oneline -10  # Identify commits to revert
git revert <commit-range>  # Revert in reverse order

# Or hard reset
git reset --hard <commit-before-changes>
```
