# GetAllTenantIDs API 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 `provider.Provider` 新增 `GetAllTenantIDs() []int` 方法，消除三个微服务对反射和 unsafe 指针的依赖。

**Architecture:** 在 jxt-core 的 Provider 中添加公开方法，从 `tenantData.Metas` map 提取所有租户 ID。三个微服务（security-management、evidence-management、file-storage-service）将删除约 50 行反射代码，直接调用新方法。

**Tech Stack:** Go 1.23+, testify/assert, testify/require

---

## Phase 1: jxt-core 添加 GetAllTenantIDs 方法

### Task 1.1: 编写 GetAllTenantIDs 单元测试

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider_api_test.go` (末尾追加)

**Step 1: 在测试文件末尾添加测试函数**

```go
// TestProvider_GetAllTenantIDs tests the GetAllTenantIDs method
func TestProvider_GetAllTenantIDs(t *testing.T) {
	p := &Provider{namespace: "jxt/"}

	t.Run("empty provider returns empty slice", func(t *testing.T) {
		data := &tenantData{
			Metas:     make(map[int]*TenantMeta),
			Databases: make(map[int]map[string]*ServiceDatabaseConfig),
			Ftps:      make(map[int][]*FtpConfigDetail),
			Storages:  make(map[int]*StorageConfig),
			Domains:   make(map[int]*DomainConfig),
		}
		p.data.Store(data)

		ids := p.GetAllTenantIDs()
		assert.NotNil(t, ids)
		assert.Empty(t, ids)
	})

	t.Run("returns all tenant IDs", func(t *testing.T) {
		data := &tenantData{
			Metas:     make(map[int]*TenantMeta),
			Databases: make(map[int]map[string]*ServiceDatabaseConfig),
			Ftps:      make(map[int][]*FtpConfigDetail),
			Storages:  make(map[int]*StorageConfig),
			Domains:   make(map[int]*DomainConfig),
		}
		data.Metas[1] = &TenantMeta{TenantID: 1, Code: "tenant1", Name: "Tenant 1"}
		data.Metas[2] = &TenantMeta{TenantID: 2, Code: "tenant2", Name: "Tenant 2"}
		data.Metas[3] = &TenantMeta{TenantID: 3, Code: "tenant3", Name: "Tenant 3"}
		p.data.Store(data)

		ids := p.GetAllTenantIDs()
		assert.Len(t, ids, 3)
		assert.ElementsMatch(t, []int{1, 2, 3}, ids)
	})

	t.Run("returns new slice each call", func(t *testing.T) {
		data := &tenantData{
			Metas:     make(map[int]*TenantMeta),
			Databases: make(map[int]map[string]*ServiceDatabaseConfig),
			Ftps:      make(map[int][]*FtpConfigDetail),
			Storages:  make(map[int]*StorageConfig),
			Domains:   make(map[int]*DomainConfig),
		}
		data.Metas[1] = &TenantMeta{TenantID: 1, Code: "tenant1", Name: "Tenant 1"}
		p.data.Store(data)

		ids1 := p.GetAllTenantIDs()
		ids2 := p.GetAllTenantIDs()

		// Modify ids1 should not affect ids2
		ids1[0] = 999
		assert.Equal(t, 1, ids2[0])
	})

	t.Run("concurrent read is safe", func(t *testing.T) {
		data := &tenantData{
			Metas:     make(map[int]*TenantMeta),
			Databases: make(map[int]map[string]*ServiceDatabaseConfig),
			Ftps:      make(map[int][]*FtpConfigDetail),
			Storages:  make(map[int]*StorageConfig),
			Domains:   make(map[int]*DomainConfig),
		}
		for i := 1; i <= 10; i++ {
			data.Metas[i] = &TenantMeta{TenantID: i, Code: fmt.Sprintf("tenant%d", i), Name: fmt.Sprintf("Tenant %d", i)}
		}
		p.data.Store(data)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ids := p.GetAllTenantIDs()
				assert.Len(t, ids, 10)
			}()
		}
		wg.Wait()
	})
}
```

**Step 2: 添加必要的 import**

在文件顶部 import 块中添加 `fmt` 和 `sync`：
```go
import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

**Step 3: 运行测试验证失败**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/tenant/provider/... -run TestProvider_GetAllTenantIDs -v
```

Expected: `FAIL` with "provider.GetAllTenantIDs undefined"

---

### Task 1.2: 实现 GetAllTenantIDs 方法

**Files:**
- Modify: `sdk/pkg/tenant/provider/provider.go` (约 1017 行 GetTenantMeta 之后)

**Step 1: 在 GetTenantMeta 方法之后添加新方法**

在 `GetTenantMeta` 方法（约 1009-1017 行）之后添加：

```go
// GetAllTenantIDs returns all tenant IDs that have configurations loaded.
// The returned slice is newly allocated and safe for modification by the caller.
// Order is not guaranteed; sort the result if deterministic order is needed.
func (p *Provider) GetAllTenantIDs() []int {
	data := p.data.Load().(*tenantData)
	if data == nil {
		return []int{}
	}

	ids := make([]int, 0, len(data.Metas))
	for id := range data.Metas {
		ids = append(ids, id)
	}
	return ids
}
```

**Step 2: 运行测试验证通过**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/tenant/provider/... -run TestProvider_GetAllTenantIDs -v
```

Expected: `PASS`

**Step 3: 运行所有 provider 测试确保无回归**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
go test ./sdk/pkg/tenant/provider/... -v
```

Expected: All tests `PASS`

---

### Task 1.3: 提交 jxt-core 更改

**Step 1: 提交代码**

```bash
cd d:/JXT/jxt-evidence-system/jxt-core
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_api_test.go
git commit -m "feat(tenant): add GetAllTenantIDs method to Provider

- Add GetAllTenantIDs() []int method to enumerate all tenant IDs
- Add comprehensive unit tests (empty, multiple tenants, copy safety, concurrency)
- Eliminates need for reflection in consuming services"
```

---

## Phase 2: 更新 security-management

### Task 2.1: 删除反射代码，使用新方法

**Files:**
- Modify: `security-management/common/tenantdb/cache.go`

**Step 1: 找到 GetTenantIDs 方法（约 138-176 行）**

删除整个 `GetTenantIDs` 方法及其内部的 `getProviderTenantIDs` 辅助函数。

**Step 2: 替换为简化版本**

将原来的 40+ 行反射代码替换为：

```go
// GetTenantIDs 获取所有租户 ID 列表
func (c *Cache) GetTenantIDs() []int {
	if c == nil || c.provider == nil {
		return []int{}
	}
	return c.provider.GetAllTenantIDs()
}
```

**Step 3: 删除不再需要的 import**

删除：
- `reflect`
- `sync/atomic`（如果仅用于此功能）

**Step 4: 运行测试验证**

```bash
cd d:/JXT/jxt-evidence-system/security-management
go test ./common/tenantdb/... -v
```

Expected: All tests `PASS`

---

### Task 2.2: 提交 security-management 更改

```bash
cd d:/JXT/jxt-evidence-system/security-management
git add common/tenantdb/cache.go
git commit -m "refactor(tenant): use provider.GetAllTenantIDs instead of reflection

- Replace ~50 lines of reflection/unsafe code with single method call
- Remove unused reflect and sync/atomic imports"
```

---

## Phase 3: 更新 evidence-management

### Task 3.1: 删除反射代码，使用新方法

**Files:**
- Modify: `evidence-management/shared/common/tenantdb/cache.go`

**Step 1: 找到 getProviderTenantIDs 辅助函数和 GetTenantIDs 方法（约 130-180 行）**

删除整个 `getProviderTenantIDs` 辅助函数，简化 `GetTenantIDs` 方法。

**Step 2: 替换为简化版本**

```go
// GetTenantIDs 获取所有租户 ID 列表
func (c *Cache) GetTenantIDs() []int {
	if c == nil || c.provider == nil {
		return []int{}
	}
	return c.provider.GetAllTenantIDs()
}
```

**Step 3: 删除不再需要的 import**

删除：
- `reflect`
- `sync/atomic`（如果仅用于此功能）

**Step 4: 运行测试验证**

```bash
cd d:/JXT/jxt-evidence-system/evidence-management/tests
ginkgo -v
```

Expected: All tests `PASS`

---

### Task 3.2: 提交 evidence-management 更改

```bash
cd d:/JXT/jxt-evidence-system/evidence-management
git add shared/common/tenantdb/cache.go
git commit -m "refactor(tenant): use provider.GetAllTenantIDs instead of reflection

- Replace ~50 lines of reflection/unsafe code with single method call
- Remove unused reflect and sync/atomic imports"
```

---

## Phase 4: 更新 file-storage-service

### Task 4.1: 删除反射代码，使用新方法

**Files:**
- Modify: `file-storage-service/common/tenantdb/cache.go`

**Step 1: 找到 GetTenantIDs 方法（约 138-176 行）**

删除整个反射实现。

**Step 2: 替换为简化版本**

```go
// GetTenantIDs 获取所有租户 ID 列表
func (c *Cache) GetTenantIDs() []int {
	if c == nil || c.provider == nil {
		return []int{}
	}
	return c.provider.GetAllTenantIDs()
}
```

**Step 3: 删除不再需要的 import**

删除：
- `reflect`
- `sync/atomic`（如果仅用于此功能）

**Step 4: 运行测试验证**

```bash
cd d:/JXT/jxt-evidence-system/file-storage-service
go test ./common/tenantdb/... -v
```

Expected: All tests `PASS`

---

### Task 4.2: 提交 file-storage-service 更改

```bash
cd d:/JXT/jxt-evidence-system/file-storage-service
git add common/tenantdb/cache.go
git commit -m "refactor(tenant): use provider.GetAllTenantIDs instead of reflection

- Replace ~50 lines of reflection/unsafe code with single method call
- Remove unused reflect and sync/atomic imports"
```

---

## 验证清单

### jxt-core
- [ ] `GetAllTenantIDs()` 方法实现正确
- [ ] 单元测试覆盖：空数据、多租户、返回副本、并发安全
- [ ] `go test ./sdk/pkg/tenant/provider/... -v` 全部通过
- [ ] 已提交

### security-management
- [ ] cache.go 删除反射代码
- [ ] 直接调用 `provider.GetAllTenantIDs()`
- [ ] `go test ./common/tenantdb/... -v` 通过
- [ ] 已提交

### evidence-management
- [ ] cache.go 删除反射代码
- [ ] 直接调用 `provider.GetAllTenantIDs()`
- [ ] `ginkgo -v` 通过
- [ ] 已提交

### file-storage-service
- [ ] cache.go 删除反射代码
- [ ] 直接调用 `provider.GetAllTenantIDs()`
- [ ] `go test ./common/tenantdb/... -v` 通过
- [ ] 已提交

---

## 回滚计划

如果发现问题：
1. 各服务 `git revert` 相应 commit 即可
2. jxt-core 的新方法向后兼容，不影响其他服务
