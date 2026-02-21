# Subdomain TenantCode Match Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable non-numeric subdomains to match tenant codes (e.g., `acmeCorp.example.com` → lookup `code="acmecorp"`)

**Architecture:** Add `CodeLookuper` interface with type assertion for backward compatibility, implement `codeIndex` for O(1) lookup (same pattern as `domainIndex`), modify `extractFromHost` to try code matching after numeric check fails.

**Tech Stack:** Go 1.24, Gin framework, ETCD client

---

## Task 1: Add codeIndex to tenantData Structure

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider.go:93-101`

**Step 1: Add codeIndex field to tenantData struct**

Find the `tenantData` struct and add `codeIndex` field after `domainIndex`:

```go
// tenantData 租户数据（新格式）
type tenantData struct {
	Metas       map[int]*TenantMeta                       `json:"metas"`
	Databases   map[int]map[string]*ServiceDatabaseConfig `json:"databases"` // tenantID -> serviceCode -> config
	Ftps        map[int][]*FtpConfigDetail                `json:"ftps"`      // tenantID -> configs[]
	Storages    map[int]*StorageConfig                    `json:"storages"`
	Domains     map[int]*DomainConfig                     `json:"domains"`     // 域名配置
	domainIndex map[string]int                            // 内嵌：域名反向索引 domain -> tenantID（不导出，不序列化）
	codeIndex   map[string]int                            // 内嵌：租户代码索引 code(lowercase) -> tenantID（不导出，不序列化）
}
```

**Step 2: Verify compilation**

Run: `cd jxt-core && go build ./...`
Expected: Success (no errors)

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go
git commit -m "feat(provider): add codeIndex field to tenantData struct"
```

---

## Task 2: Add GetTenantIDByCode Method to Provider

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider.go` (after `GetTenantIDByDomain` method, around line 1070)
- Test: `jxt-core/sdk/pkg/tenant/provider/provider_api_test.go`

**Step 1: Write the failing test**

Add to `provider_api_test.go`:

```go
// TestProvider_GetTenantIDByCode tests the GetTenantIDByCode method
func TestProvider_GetTenantIDByCode(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}
	data.Metas[1] = &TenantMeta{TenantID: 1, Code: "acmeCorp", Name: "ACME Corp"}
	data.Metas[2] = &TenantMeta{TenantID: 2, Code: "techInc", Name: "Tech Inc"}

	// Build codeIndex manually for test
	data.codeIndex = map[string]int{
		"acmecorp": 1,
		"techinc":  2,
	}
	p.data.Store(data)

	tests := []struct {
		name      string
		code      string
		wantID    int
		wantFound bool
	}{
		{
			name:      "existing code lowercase",
			code:      "acmecorp",
			wantID:    1,
			wantFound: true,
		},
		{
			name:      "existing code uppercase (case insensitive)",
			code:      "ACMECORP",
			wantID:    1,
			wantFound: true,
		},
		{
			name:      "existing code mixed case",
			code:      "TechInc",
			wantID:    2,
			wantFound: true,
		},
		{
			name:      "non-existing code",
			code:      "unknown",
			wantID:    0,
			wantFound: false,
		},
		{
			name:      "empty code",
			code:      "",
			wantID:    0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, found := p.GetTenantIDByCode(tt.code)
			if found != tt.wantFound {
				t.Errorf("GetTenantIDByCode(%q) found = %v, want %v", tt.code, found, tt.wantFound)
			}
			if id != tt.wantID {
				t.Errorf("GetTenantIDByCode(%q) id = %v, want %v", tt.code, id, tt.wantID)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd jxt-core && go test -v ./sdk/pkg/tenant/provider/... -run TestProvider_GetTenantIDByCode`
Expected: FAIL with "GetTenantIDByCode undefined"

**Step 3: Write minimal implementation**

Add after `GetTenantIDByDomain` method in `provider.go`:

```go
// GetTenantIDByCode 通过租户代码获取租户ID
// 用于子域名匹配场景：subdomain.example.com -> 查找 code=subdomain 的租户
//
// 注意：DNS 子域名不区分大小写（RFC 4343），因此 code 查找也使用小写匹配
// 查找复杂度 O(1)，无锁，线程安全
func (p *Provider) GetTenantIDByCode(code string) (int, bool) {
	if code == "" {
		return 0, false
	}

	data := p.data.Load().(*tenantData)

	// 使用 codeIndex 进行 O(1) 查找，与 domainIndex 模式一致
	tenantID, ok := data.codeIndex[strings.ToLower(code)]
	return tenantID, ok
}
```

**Step 4: Run test to verify it passes**

Run: `cd jxt-core && go test -v ./sdk/pkg/tenant/provider/... -run TestProvider_GetTenantIDByCode`
Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_api_test.go
git commit -m "feat(provider): add GetTenantIDByCode method with O(1) lookup"
```

---

## Task 3: Refactor buildDomainIndex to buildIndexes

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/provider/provider.go:997-1046` (buildDomainIndex function)

**Step 1: Write the failing test**

Add to `provider_api_test.go`:

```go
// TestBuildIndexes tests that both domainIndex and codeIndex are built correctly
func TestBuildIndexes(t *testing.T) {
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add tenant metas
	data.Metas[1] = &TenantMeta{TenantID: 1, Code: "acmeCorp", Name: "ACME"}
	data.Metas[2] = &TenantMeta{TenantID: 2, Code: "techInc", Name: "Tech"}

	// Add domain configs
	data.Domains[1] = &DomainConfig{
		TenantID: 1,
		Primary:  "acme.example.com",
		Aliases:  []string{"www.acme.example.com"},
	}
	data.Domains[2] = &DomainConfig{
		TenantID: 2,
		Primary:  "tech.example.com",
	}

	// Build indexes
	buildIndexes(data)

	// Verify domainIndex
	assert.Equal(t, 1, data.domainIndex["acme.example.com"])
	assert.Equal(t, 1, data.domainIndex["www.acme.example.com"])
	assert.Equal(t, 2, data.domainIndex["tech.example.com"])

	// Verify codeIndex (lowercase)
	assert.Equal(t, 1, data.codeIndex["acmecorp"])
	assert.Equal(t, 2, data.codeIndex["techinc"])

	// Verify case insensitivity in index keys
	assert.NotContains(t, data.codeIndex, "ACMECORP") // Should be lowercase
}
```

**Step 2: Run test to verify it fails**

Run: `cd jxt-core && go test -v ./sdk/pkg/tenant/provider/... -run TestBuildIndexes`
Expected: FAIL with "buildIndexes undefined"

**Step 3: Refactor buildDomainIndex to buildIndexes**

Replace the `buildDomainIndex` function with `buildIndexes`:

```go
// buildIndexes 构建 domainIndex 和 codeIndex
// domainIndex: domain(lowercase) -> tenantID
// codeIndex: code(lowercase) -> tenantID
// 返回 domainIndex 以保持与现有调用的兼容性
// 检测并记录域名冲突和代码冲突
func buildIndexes(data *tenantData) map[string]int {
	// 预估容量：每个租户约 4 个域名（Primary + 2 Aliases + Internal）
	estimatedSize := len(data.Domains) * 4
	if estimatedSize == 0 {
		estimatedSize = 8 // 最小容量
	}
	domainIndex := make(map[string]int, estimatedSize)

	// codeIndex 容量等于租户数量
	codeEstimatedSize := len(data.Metas)
	if codeEstimatedSize == 0 {
		codeEstimatedSize = 8
	}
	codeIndex := make(map[string]int, codeEstimatedSize)

	// 构建域名索引
	for tenantID, cfg := range data.Domains {
		// Primary 域名
		if cfg.Primary != "" {
			domain := normalizeDomain(cfg.Primary)
			if domain != "" {
				if existingID, exists := domainIndex[domain]; exists && existingID != tenantID {
					logger.Warnf("domain conflict: %s claimed by tenant %d and %d, using %d",
						domain, existingID, tenantID, tenantID)
				}
				domainIndex[domain] = tenantID
			}
		}

		// Aliases 域名
		for _, alias := range cfg.Aliases {
			domain := normalizeDomain(alias)
			if domain != "" {
				if existingID, exists := domainIndex[domain]; exists && existingID != tenantID {
					logger.Warnf("domain alias conflict: %s claimed by tenant %d and %d, using %d",
						domain, existingID, tenantID, tenantID)
				}
				domainIndex[domain] = tenantID
			}
		}

		// Internal 域名
		if cfg.Internal != "" {
			domain := normalizeDomain(cfg.Internal)
			if domain != "" {
				if existingID, exists := domainIndex[domain]; exists && existingID != tenantID {
					logger.Warnf("internal domain conflict: %s claimed by tenant %d and %d, using %d",
						domain, existingID, tenantID, tenantID)
				}
				domainIndex[domain] = tenantID
			}
		}
	}

	// 构建代码索引（小写规范化，符合 DNS 不区分大小写特性）
	for tenantID, meta := range data.Metas {
		if meta.Code != "" {
			code := strings.ToLower(strings.TrimSpace(meta.Code))
			if code != "" {
				if existingID, exists := codeIndex[code]; exists && existingID != tenantID {
					logger.Warnf("tenant code conflict: %s claimed by tenant %d and %d, using %d",
						code, existingID, tenantID, tenantID)
				}
				codeIndex[code] = tenantID
			}
		}
	}

	data.domainIndex = domainIndex
	data.codeIndex = codeIndex
	return domainIndex
}

// buildDomainIndex 保留为别名，向后兼容
// Deprecated: Use buildIndexes instead
var buildDomainIndex = buildIndexes
```

**Step 4: Update all callers to use buildIndexes**

Find and replace all `buildDomainIndex` calls with `buildIndexes` in provider.go:
- `LoadAll` function
- `handleWatchEvent` function

Run: `cd jxt-core && grep -n "buildDomainIndex" sdk/pkg/tenant/provider/provider.go`

Update each occurrence.

**Step 5: Run all provider tests**

Run: `cd jxt-core && go test -v ./sdk/pkg/tenant/provider/...`
Expected: All tests PASS

**Step 6: Commit**

```bash
git add sdk/pkg/tenant/provider/provider.go sdk/pkg/tenant/provider/provider_api_test.go
git commit -m "refactor(provider): rename buildDomainIndex to buildIndexes, add codeIndex building"
```

---

## Task 4: Add CodeLookuper Interface

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go:61-66`
- Test: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id_test.go`

**Step 1: Add CodeLookuper interface after DomainLookuper**

In `extract_tenant_id.go`, after the `DomainLookuper` interface definition (around line 66):

```go
// DomainLookuper defines the domain lookup interface for tenant ID resolution.
// It is implicitly implemented by provider.Provider (duck typing).
// This interface follows the Go convention of "consumer defines interface".
type DomainLookuper interface {
	GetTenantIDByDomain(domain string) (int, bool)
}

// CodeLookuper defines the tenant code lookup interface for subdomain-based resolution.
// Provider implicitly implements this interface (duck typing).
// Using a separate interface instead of extending DomainLookuper avoids breaking changes.
type CodeLookuper interface {
	GetTenantIDByCode(code string) (int, bool)
}
```

**Step 2: Verify compilation**

Run: `cd jxt-core && go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id.go
git commit -m "feat(middleware): add CodeLookuper interface for tenant code lookup"
```

---

## Task 5: Modify extractFromHost to Use CodeLookuper

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go:240-278`
- Test: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id_test.go`

**Step 1: Write the failing test for code matching**

Add to `extract_tenant_id_test.go`:

```go
// mockCodeLookuper implements both DomainLookuper and CodeLookuper
type mockCodeLookuper struct {
	domains map[string]int
	codes   map[string]int
}

func (m *mockCodeLookuper) GetTenantIDByDomain(domain string) (int, bool) {
	tenantID, ok := m.domains[domain]
	return tenantID, ok
}

func (m *mockCodeLookuper) GetTenantIDByCode(code string) (int, bool) {
	tenantID, ok := m.codes[strings.ToLower(code)]
	return tenantID, ok
}

func TestExtractFromHost_TenantCodeMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Non-numeric subdomain matches tenant code", func(t *testing.T) {
		router := gin.New()
		mockLookup := &mockCodeLookuper{
			domains: map[string]int{},
			codes:   map[string]int{"acmecorp": 1},
		}
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "acmeCorp.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["tenant_id"].(float64) != 1 {
			t.Errorf("expected tenant_id=1, got %v", resp["tenant_id"])
		}
	})

	t.Run("Non-numeric subdomain case insensitive match", func(t *testing.T) {
		router := gin.New()
		mockLookup := &mockCodeLookuper{
			domains: map[string]int{},
			codes:   map[string]int{"techinc": 2}, // lowercase in index
		}
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "TECHINC.example.com" // uppercase in request
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("Non-numeric subdomain no match returns 400", func(t *testing.T) {
		router := gin.New()
		mockLookup := &mockCodeLookuper{
			domains: map[string]int{},
			codes:   map[string]int{"othercorp": 1},
		}
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(200, nil)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "unknown.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 400 {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("Numeric subdomain still works", func(t *testing.T) {
		router := gin.New()
		router.Use(ExtractTenantID(WithResolverType("host")))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "123.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd jxt-core && go test -v ./sdk/pkg/tenant/middleware/... -run TestExtractFromHost_TenantCodeMatch`
Expected: FAIL with "non-numeric subdomain no match returns 400" - gets 200 instead (subdomain string returned)

**Step 3: Modify extractFromHost function**

Replace the `extractFromHost` function (around line 240-278):

```go
// extractFromHost extracts tenant ID from Host header
// Priority:
//  1. If domainLookup is configured, attempt exact domain match first
//  2. Fall back to subdomain extraction:
//     2a. If subdomain is numeric, use as tenant ID directly
//     2b. If subdomain is non-numeric and CodeLookuper available, match by tenant code
//
// Exact domain matching does not support wildcards.
// DNS is case-insensitive (RFC 4343), all lookups use lowercase.
func extractFromHost(c *gin.Context, lookup DomainLookuper) (string, bool) {
	host := c.Request.Host
	if host == "" {
		return "", false
	}

	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// 1. If domain lookup is configured, try exact domain match first
	if lookup != nil {
		if tenantID, ok := lookup.GetTenantIDByDomain(host); ok {
			return strconv.Itoa(tenantID), true
		}
	}

	// 2. Fall back to subdomain extraction
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "", false
	}

	subdomain := parts[0]
	if subdomain == "" || subdomain == "www" {
		return "", false
	}

	// 2a. If subdomain is numeric, return directly (e.g., "123.example.com" -> "123")
	if _, err := strconv.Atoi(subdomain); err == nil {
		return subdomain, true
	}

	// 2b. Subdomain is non-numeric, try to match by tenant code
	//     e.g., "acmeCorp.example.com" -> lookup by code "acmecorp" (lowercase)
	//     Uses type assertion for backward compatibility - only Provider supports this
	if lookup != nil {
		if codeLookup, ok := lookup.(CodeLookuper); ok {
			// DNS is case-insensitive, normalize to lowercase
			if tenantID, ok := codeLookup.GetTenantIDByCode(strings.ToLower(subdomain)); ok {
				return strconv.Itoa(tenantID), true
			}
		}
	}

	return "", false
}
```

**Step 4: Run test to verify it passes**

Run: `cd jxt-core && go test -v ./sdk/pkg/tenant/middleware/... -run TestExtractFromHost_TenantCodeMatch`
Expected: PASS

**Step 5: Run all middleware tests**

Run: `cd jxt-core && go test -v ./sdk/pkg/tenant/middleware/...`
Expected: All tests PASS

**Step 6: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id.go sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "feat(middleware): add tenant code matching for non-numeric subdomains"
```

---

## Task 6: Update Existing Tests for mockDomainLookuper

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id_test.go`

**Step 1: Find and update mockDomainLookuper**

Locate the existing `mockDomainLookuper` type in the test file and add the `GetTenantIDByCode` method:

```go
// mockDomainLookuper implements DomainLookuper for testing
type mockDomainLookuper struct {
	domains map[string]int
	codes   map[string]int // Added for CodeLookuper support
}

func (m *mockDomainLookuper) GetTenantIDByDomain(domain string) (int, bool) {
	tenantID, ok := m.domains[domain]
	return tenantID, ok
}

func (m *mockDomainLookuper) GetTenantIDByCode(code string) (int, bool) {
	if m.codes == nil {
		return 0, false
	}
	tenantID, ok := m.codes[strings.ToLower(code)]
	return tenantID, ok
}
```

**Step 2: Update test case "Domain lookup fails and subdomain is non-numeric"**

Find the test case that expects 400 for non-numeric subdomain. Update it to either:
- Provide empty `codes` map to maintain 400 behavior, OR
- Split into two tests: one for match, one for no-match

**Step 3: Run all middleware tests**

Run: `cd jxt-core && go test -v ./sdk/pkg/tenant/middleware/...`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "test(middleware): update mockDomainLookuper to implement CodeLookuper"
```

---

## Task 7: Update README Documentation

**Files:**
- Modify: `jxt-core/README.md`

**Step 1: Update the subdomain extraction description**

In the "租户识别方式" section (around line 400), update the description:

```markdown
**域名识别（host 类型）支持三种方式**：

1. **精确域名匹配**（优先）：通过 `DomainLookuper` 接口查询域名对应的租户 ID
2. **数字子域名**：从 Host 中提取数字作为租户 ID（如 `123.example.com` → `123`）
3. **租户代码匹配**（兜底）：非数字子域名通过 `CodeLookuper` 接口匹配租户代码（如 `acmeCorp.example.com` → 查找 `code="acmecorp"`）
```

**Step 2: Update usage examples**

Add example for tenant code matching:

```go
// Provider 自动实现 DomainLookuper 和 CodeLookuper
router.Use(ExtractTenantID(
    WithResolverType("host"),
    WithDomainLookup(provider),  // 支持: 数字子域名 + 精确域名 + 租户代码
))
```

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: update README with tenant code matching for subdomains"
```

---

## Task 8: Run Full Test Suite

**Step 1: Run all jxt-core tests**

Run: `cd jxt-core && go test -v ./...`
Expected: All tests PASS

**Step 2: Run integration check**

Run: `cd jxt-core && go build ./...`
Expected: Success

**Step 3: Final commit (if needed)**

```bash
git status
# If any uncommitted changes:
git add -A
git commit -m "chore: final cleanup for subdomain tenant code matching"
```

---

## Summary

| Task | Description | Files Changed |
|------|-------------|---------------|
| 1 | Add codeIndex to tenantData | `provider/provider.go` |
| 2 | Add GetTenantIDByCode method | `provider/provider.go`, `provider_api_test.go` |
| 3 | Refactor buildDomainIndex to buildIndexes | `provider/provider.go` |
| 4 | Add CodeLookuper interface | `middleware/extract_tenant_id.go` |
| 5 | Modify extractFromHost for code matching | `middleware/extract_tenant_id.go`, `extract_tenant_id_test.go` |
| 6 | Update existing tests | `middleware/extract_tenant_id_test.go` |
| 7 | Update documentation | `README.md` |
| 8 | Full test suite | All files |
