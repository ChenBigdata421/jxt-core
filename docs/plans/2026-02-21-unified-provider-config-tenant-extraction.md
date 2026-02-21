# Unified Provider Config for Tenant Extraction Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 统一 Provider 层为 `WithProviderConfig`，移除 `WithDomainLookup`。保留所有硬编码选项（`WithResolverType`、`WithHeaderName` 等）供不依赖 Provider 的场景使用。

**Architecture:** API 分为两层：
- **硬编码层**：`WithResolverType`、`WithHeaderName`、`WithQueryParam`、`WithPathIndex`、`WithOnMissingTenant` — 不依赖 Provider，手动指定配置
- **Provider 层**：`WithProviderConfig` — 唯一的 Provider 入口，从 ETCD ResolverConfig 读取全部配置

两层可组合：硬编码选项放在 `WithProviderConfig` 之后可覆盖 ETCD 配置（选项顺序覆盖）。`ProviderConfigurer` 接口确保编译时安全。

**Tech Stack:** Go 1.24, Gin framework, functional options pattern, duck typing interfaces

---

## Background

Current `WithDomainLookup(provider)` has issues:
1. Initialization logic only reads `HTTPType`, not `HTTPHeaderName`/`HTTPQueryParam`/`HTTPPathIndex`
2. Interface doesn't require `CodeLookuper`, causing silent failure in `httpHostMode="code"`
3. Redundant config reading between option and initialization
4. 下游服务 `DomainFallbackMiddleware` 直接调用 `WithDomainLookup`，移除后需同步更新

## Configuration Mapping

```
ETCD ResolverConfig           Middleware Config
─────────────────────────────────────────────────
httpType = "host"    →  resolverType = host
  httpHostMode             →  extractFromHost() mode (numeric/domain/code)

httpType = "header"  →  resolverType = header
  httpHeaderName           →  headerName

httpType = "query"   →  resolverType = query
  httpQueryParam           →  queryParam

httpType = "path"    →  resolverType = path
  httpPathIndex            →  pathIndex
```

---

## Task 1: Add ProviderConfigurer Interface

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go` (near `ResolverConfigProvider` interface)

**Step 1: Write the failing test**

Add to `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id_test.go` (after `mockDomainLookuper` definition):

```go
// TestProviderConfigurerInterface verifies mockResolverConfigProvider implements ProviderConfigurer
func TestProviderConfigurerInterface(t *testing.T) {
	// Compile-time check: mockResolverConfigProvider must implement ProviderConfigurer
	// This ensures the interface includes DomainLookuper, CodeLookuper, and ResolverConfigProvider
	var _ ProviderConfigurer = (*mockResolverConfigProvider)(nil)
}
```

**Step 2: Run test to verify it fails**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/middleware/... -v -run TestProviderConfigurerInterface`
Expected: FAIL with "undefined: ProviderConfigurer"

**Step 3: Write minimal implementation**

Add to `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go` (after `ResolverConfigProvider` interface definition):

```go
// ProviderConfigurer combines all provider interfaces needed for tenant resolution.
// Provider implicitly implements this interface via duck typing.
// Use this with WithProviderConfig() for unified ETCD-based configuration.
//
// Interface composition:
//   - DomainLookuper: for httpHostMode="domain" (exact domain match)
//   - CodeLookuper: for httpHostMode="code" (tenant code from subdomain)
//   - ResolverConfigProvider: for reading httpType and related config from ETCD
type ProviderConfigurer interface {
	DomainLookuper
	CodeLookuper
	ResolverConfigProvider
}
```

**Step 4: Run test to verify it passes**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/middleware/... -v -run TestProviderConfigurerInterface`
Expected: PASS

**Step 5: Commit**

```bash
cd jxt-core
git add sdk/pkg/tenant/middleware/extract_tenant_id.go sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "$(cat <<'EOF'
feat(tenant): add ProviderConfigurer interface for unified config

Add ProviderConfigurer interface that combines DomainLookuper,
CodeLookuper, and ResolverConfigProvider. This ensures compile-time
safety for all tenant resolution methods.

Interface composition:
- DomainLookuper: for httpHostMode="domain"
- CodeLookuper: for httpHostMode="code"
- ResolverConfigProvider: for reading httpType and config

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Replace WithDomainLookup with WithProviderConfig + Update All Tests

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go` (`WithDomainLookup` function)
- Modify: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id_test.go` (update all `WithDomainLookup` references)

**Step 1: Write the failing test**

Add to `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id_test.go`:

```go
// ========== WithProviderConfig Tests ==========

// assertTenantIDInResponse verifies the response body contains expected tenant_id
func assertTenantIDInResponse(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	var resp struct {
		TenantID int `json:"tenant_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v (body: %s)", err, w.Body.String())
	}
	if resp.TenantID != expected {
		t.Errorf("expected tenant_id %d, got %d (body: %s)", expected, resp.TenantID, w.Body.String())
	}
}

// TestWithProviderConfig_HeaderMode tests header mode with custom header name
func TestWithProviderConfig_HeaderMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockProvider := &mockResolverConfigProvider{
		config: &provider.ResolverConfig{
			HTTPType:       "header",
			HTTPHeaderName: "X-Custom-Tenant",
		},
	}

	router := gin.New()
	router.Use(ExtractTenantID(WithProviderConfig(mockProvider)))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Custom-Tenant", "123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	assertTenantIDInResponse(t, w, 123)
}

// TestWithProviderConfig_QueryMode tests query mode with custom param name
func TestWithProviderConfig_QueryMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockProvider := &mockResolverConfigProvider{
		config: &provider.ResolverConfig{
			HTTPType:       "query",
			HTTPQueryParam: "org_id",
		},
	}

	router := gin.New()
	router.Use(ExtractTenantID(WithProviderConfig(mockProvider)))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	req := httptest.NewRequest("GET", "/test?org_id=456", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	assertTenantIDInResponse(t, w, 456)
}

// TestWithProviderConfig_PathMode tests path mode with custom index
func TestWithProviderConfig_PathMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockProvider := &mockResolverConfigProvider{
		config: &provider.ResolverConfig{
			HTTPType:      "path",
			HTTPPathIndex: 1,
		},
	}

	router := gin.New()
	router.Use(ExtractTenantID(WithProviderConfig(mockProvider)))
	router.GET("/api/:tenant_id/users", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	// Path: /api/789/users -> index 1 = "789"
	req := httptest.NewRequest("GET", "/api/789/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	assertTenantIDInResponse(t, w, 789)
}

// TestWithProviderConfig_HostMode_Domain tests host mode with domain lookup
func TestWithProviderConfig_HostMode_Domain(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockProvider := &mockResolverConfigProvider{
		domainLookuper: mockDomainLookuper{
			domains: map[string]int{
				"tenant1.example.com": 1,
			},
		},
		config: &provider.ResolverConfig{
			HTTPType:     "host",
			HTTPHostMode: "domain",
		},
	}

	router := gin.New()
	router.Use(ExtractTenantID(WithProviderConfig(mockProvider)))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "tenant1.example.com"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	assertTenantIDInResponse(t, w, 1)
}

// TestWithProviderConfig_HostMode_Code tests host mode with tenant code lookup
func TestWithProviderConfig_HostMode_Code(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockProvider := &mockResolverConfigProvider{
		domainLookuper: mockDomainLookuper{
			codes: map[string]int{
				"acme": 100,
			},
		},
		config: &provider.ResolverConfig{
			HTTPType:     "host",
			HTTPHostMode: "code",
		},
	}

	router := gin.New()
	router.Use(ExtractTenantID(WithProviderConfig(mockProvider)))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "acme.example.com"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	assertTenantIDInResponse(t, w, 100)
}

// TestWithProviderConfig_HostMode_Numeric tests host mode with numeric subdomain
func TestWithProviderConfig_HostMode_Numeric(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockProvider := &mockResolverConfigProvider{
		config: &provider.ResolverConfig{
			HTTPType:     "host",
			HTTPHostMode: "numeric",
		},
	}

	router := gin.New()
	router.Use(ExtractTenantID(WithProviderConfig(mockProvider)))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "999.example.com"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	assertTenantIDInResponse(t, w, 999)
}

// TestWithProviderConfig_NilConfig tests behavior when ResolverConfig is nil
func TestWithProviderConfig_NilConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockProvider := &mockResolverConfigProvider{
		config: nil,
	}

	router := gin.New()
	router.Use(ExtractTenantID(WithProviderConfig(mockProvider)))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	// With nil config, should use default (header mode with X-Tenant-ID)
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-ID", "123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected status 200 (default header mode), got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/middleware/... -v -run TestWithProviderConfig`
Expected: FAIL with "undefined: WithProviderConfig"

**Step 3: Write implementation**

Replace `WithDomainLookup` function with `WithProviderConfig` in `extract_tenant_id.go`:

```go
// WithProviderConfig enables unified provider-based configuration for tenant ID extraction.
// It reads all resolver settings from the Provider's ResolverConfig:
//   - httpType: determines extraction method (host/header/query/path)
//   - httpHeaderName: header name when httpType="header"
//   - httpQueryParam: query param when httpType="query"
//   - httpPathIndex: path index when httpType="path"
//   - httpHostMode: host mode when httpType="host" (numeric/domain/code)
//
// This is the recommended way to configure tenant extraction when using ETCD.
// All existing hardcoded options (WithResolverType, WithHeaderName, etc.) remain available
// and can be placed AFTER WithProviderConfig to override specific ETCD settings.
//
// Note: Configuration is read once at middleware creation time (snapshot semantics).
// Changes to ETCD configuration after middleware creation require re-creating the middleware.
//
// Example:
//
//	router.Use(middleware.ExtractTenantID(
//	    middleware.WithProviderConfig(provider),
//	))
func WithProviderConfig(provider ProviderConfigurer) Option {
	return func(c *Config) {
		c.domainLookup = provider
		if resolverCfg := provider.GetResolverConfig(); resolverCfg != nil {
			c.resolverConfig = resolverCfg

			// Set resolverType from ETCD config
			if resolverCfg.HTTPType != "" {
				c.resolverType = ResolverType(resolverCfg.HTTPType)
			}

			// Set field values based on httpType (only set relevant fields)
			switch resolverCfg.HTTPType {
			case "header":
				if resolverCfg.HTTPHeaderName != "" {
					c.headerName = resolverCfg.HTTPHeaderName
				}
			case "query":
				if resolverCfg.HTTPQueryParam != "" {
					c.queryParam = resolverCfg.HTTPQueryParam
				}
			case "path":
				c.pathIndex = resolverCfg.HTTPPathIndex
			}
		}
	}
}
```

> **Design Note:** `Config.domainLookup` 字段故意保持为 `DomainLookuper` 类型，而非升级为 `ProviderConfigurer`。
> 原因是 `extractFromHost(lookup DomainLookuper, ...)` 函数签名期望 `DomainLookuper` 参数，
> 并在运行时通过 `lookup.(CodeLookuper)` 类型断言来访问 `GetTenantIDByCode`。
> 如果将字段类型改为 `ProviderConfigurer`，则需要同步修改 `extractFromHost` 的函数签名及其内部逻辑，
> 这超出了本次重构的范围。`WithProviderConfig` 通过将 `ProviderConfigurer` 赋值给 `cfg.domainLookup`
> （利用 Go 的接口隐式满足），在运行时仍然能通过类型断言访问所有三个接口的方法。

**Step 4: Run test to verify it passes**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/middleware/... -v -run TestWithProviderConfig`
Expected: PASS

**Step 5: Update existing tests using WithDomainLookup**

Search for all occurrences of `WithDomainLookup` in the test file and replace with `WithProviderConfig`.

The tests that use `WithDomainLookup` should be updated to use `WithProviderConfig`:

```go
// Before:
router.Use(ExtractTenantID(
    WithResolverType("host"),
    WithDomainLookup(mockProvider),
))

// After:
router.Use(ExtractTenantID(WithProviderConfig(mockProvider)))
```

Note: All 24 existing `WithDomainLookup` calls in the test file already use `mockResolverConfigProvider` (which implements all three interfaces: `DomainLookuper`, `CodeLookuper`, `ResolverConfigProvider`), so replacing them with `WithProviderConfig` requires no mock type changes. The `mockDomainLookuper` struct is only used as an embedded field inside `mockResolverConfigProvider`, not passed directly to `WithDomainLookup`.

**Step 6: Run all tests to verify**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/middleware/... -v`
Expected: All tests PASS

**Step 7: Commit**

```bash
cd jxt-core
git add sdk/pkg/tenant/middleware/extract_tenant_id.go sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "$(cat <<'EOF'
feat(tenant): replace WithDomainLookup with WithProviderConfig

Replace WithDomainLookup with WithProviderConfig that reads all resolver
settings from ETCD in a single place:
- httpType determines extraction method (host/header/query/path)
- httpHeaderName for header mode
- httpQueryParam for query mode
- httpPathIndex for path mode
- httpHostMode for host mode (numeric/domain/code)

Update all tests to use WithProviderConfig, ensuring the commit compiles
and all tests pass atomically.

The ProviderConfigurer interface ensures compile-time safety for all
required methods (DomainLookuper, CodeLookuper, ResolverConfigProvider).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Remove Redundant Initialization Logic

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go` (`ExtractTenantID` 函数中 `if cfg.domainLookup != nil` 开头的 Provider 初始化块)

**Step 1: Verify tests still pass after removal**

The redundant initialization logic in `ExtractTenantID` (the `if cfg.domainLookup != nil` block that reads `ResolverConfig` from Provider) is no longer needed since `WithProviderConfig` handles all config reading.

**Step 2: Remove the redundant code**

Remove the following block from `ExtractTenantID` function:

```go
// DELETE THIS BLOCK:
// Try to get ResolverConfig from Provider (if domainLookup implements ResolverConfigProvider)
if cfg.domainLookup != nil {
    if rcp, ok := cfg.domainLookup.(ResolverConfigProvider); ok {
        if resolverCfg := rcp.GetResolverConfig(); resolverCfg != nil {
            cfg.resolverConfig = resolverCfg
            // Override resolverType from ETCD config if set
            if resolverCfg.HTTPType != "" {
                cfg.resolverType = ResolverType(resolverCfg.HTTPType)
            }
        }
    }
}
```

**Step 3: Run tests to verify**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/middleware/... -v`
Expected: All tests PASS

**Step 4: Commit**

```bash
cd jxt-core
git add sdk/pkg/tenant/middleware/extract_tenant_id.go
git commit -m "$(cat <<'EOF'
refactor(tenant): remove redundant config reading from ExtractTenantID

Remove the redundant initialization logic that read config from
ResolverConfigProvider. This is now handled by WithProviderConfig
option, making the code cleaner and avoiding duplicate config reads.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Update Downstream Services

**Files:**
- Modify: `security-management/common/middleware/tenant_fallback.go`
- Modify: `file-storage-service/common/middleware/tenant_fallback.go`
- Modify: `file-storage-service/tests/perf/tenant_resolver_perf_test.go`

**Step 0: Update jxt-core dependency in downstream services**

Before updating the downstream service code, ensure they reference the new jxt-core version that includes `WithProviderConfig`:

- **Option A（本地开发）：** 在各服务的 `go.mod` 中添加或取消注释 `replace` 指令：
  ```
  // file-storage-service/go.mod — 取消注释已有的 replace 行：
  replace github.com/ChenBigdata421/jxt-core => ../jxt-core

  // security-management/go.mod — 添加 replace 指令（当前不存在）：
  replace github.com/ChenBigdata421/jxt-core => ../jxt-core
  ```

- **Option B（发布版本）：** 在 jxt-core 发布新版本后，在各下游服务中运行：
  ```bash
  go get github.com/ChenBigdata421/jxt-core@<new-version-tag>
  go mod tidy
  ```

**Step 1: Update security-management DomainFallbackMiddleware**

Replace `WithDomainLookup(p)` with `WithProviderConfig(p)` + `WithResolverType("host")`:

```go
// security-management/common/middleware/tenant_fallback.go
func DomainFallbackMiddleware(p *provider.Provider) gin.HandlerFunc {
    return func(c *gin.Context) {
        if _, exists := c.Get("tenant_id"); exists {
            c.Next()
            return
        }
        middleware.ExtractTenantID(
            middleware.WithProviderConfig(p),    // 从 ETCD 读取配置 + 设置 domainLookup
            middleware.WithResolverType("host"), // 强制覆盖为 host 模式
        )(c)
    }
}
```

**Step 2: Update file-storage-service DomainFallbackMiddleware**

Same change as Step 1 (identical pattern).

**Step 3: Update file-storage-service perf test mock**

The `mockDomainLookuper` only implements `DomainLookuper`, but `WithProviderConfig` requires `ProviderConfigurer`. Add missing methods:

```go
// file-storage-service/tests/perf/tenant_resolver_perf_test.go

// GetTenantIDByCode implements CodeLookuper interface
func (m *mockDomainLookuper) GetTenantIDByCode(code string) (int, bool) {
    return 0, false
}

// GetResolverConfig implements ResolverConfigProvider interface
func (m *mockDomainLookuper) GetResolverConfig() *provider.ResolverConfig {
    return &provider.ResolverConfig{
        HTTPType:     "host",
        HTTPHostMode: "domain",
    }
}
```

Then replace all 4 occurrences of `WithDomainLookup(lookuper)` with `WithProviderConfig(lookuper)`:

```go
// Before:
middlewareFunc := tenantmiddleware.ExtractTenantID(
    tenantmiddleware.WithResolverType("host"),
    tenantmiddleware.WithDomainLookup(lookuper),
)

// After:
middlewareFunc := tenantmiddleware.ExtractTenantID(
    tenantmiddleware.WithProviderConfig(lookuper),
    tenantmiddleware.WithResolverType("host"), // 覆盖 GetResolverConfig 中的设置
)
```

Note: `import` 需添加 `provider` 包引用 `"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"`

**Step 4: Verify compilation**

```bash
cd security-management && go build ./...
cd file-storage-service && go build ./...
```

**Step 5: Run perf tests**

```bash
cd file-storage-service && go test ./tests/perf/... -v -run TestHTTPTenantResolver
```

**Step 6: Commit**

```bash
git add security-management/common/middleware/tenant_fallback.go
git add file-storage-service/common/middleware/tenant_fallback.go
git add file-storage-service/tests/perf/tenant_resolver_perf_test.go
git commit -m "$(cat <<'EOF'
refactor(tenant): update downstream services to use WithProviderConfig

Update DomainFallbackMiddleware in security-management and
file-storage-service to use WithProviderConfig + WithResolverType("host")
instead of removed WithDomainLookup.

Update perf test mock to satisfy ProviderConfigurer interface by adding
GetTenantIDByCode and GetResolverConfig methods.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Update Package Documentation

**Files:**
- Modify: `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go` (package doc block)
- Modify: `jxt-core/README.md` (`WithDomainLookup` usage example section)

**Step 1: Update package documentation**

Replace the package documentation block with:

```go
// Package middleware provides tenant ID extraction middleware for Gin framework.
//
// Supported resolver types (httpType):
//   - "host":   Extract tenant ID from Host header (domain-based)
//   - "header": Extract tenant ID from a custom header (default: X-Tenant-ID)
//   - "query":  Extract tenant ID from URL query parameter (default: tenant)
//   - "path":   Extract tenant ID from URL path by index
//
// When httpType is "host", three mutually exclusive modes (httpHostMode) are supported:
//   - "numeric": Only numeric subdomain (e.g., "123.example.com" -> "123", default)
//   - "domain":  Only exact domain match via Provider (requires WithProviderConfig)
//   - "code":    Only tenant code match via Provider (requires WithProviderConfig)
//
// # Usage
//
// Recommended: Use WithProviderConfig for unified ETCD-based configuration:
//
//	router.Use(middleware.ExtractTenantID(
//	    middleware.WithProviderConfig(provider),
//	))
//
// Hardcoded: Explicit resolver type without Provider:
//
//	router.Use(middleware.ExtractTenantID(
//	    middleware.WithResolverType("query"),
//	    middleware.WithQueryParam("tenant_id"),
//	))
//
// Combined: Provider with hardcoded override (options applied in order):
//
//	middleware.ExtractTenantID(
//	    middleware.WithProviderConfig(provider),
//	    middleware.WithResolverType("host"), // overrides ETCD httpType
//	)
//
// Simple: Default header-based extraction:
//
//	router.Use(middleware.ExtractTenantID())
package middleware
```

**Step 2: Update README.md**

Replace `WithDomainLookup(provider)` with `WithProviderConfig(provider)` in `jxt-core/README.md` (the recommended usage example section):

```go
// Before (README.md):
middleware.WithDomainLookup(provider)

// After (README.md):
middleware.WithProviderConfig(provider)
```

**Step 3: Run tests to verify**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/middleware/... -v`
Expected: All tests PASS

**Step 4: Commit**

```bash
cd jxt-core
git add sdk/pkg/tenant/middleware/extract_tenant_id.go README.md
git commit -m "$(cat <<'EOF'
docs(tenant): update package docs and README with WithProviderConfig

Update package documentation and README.md to show WithProviderConfig
as the recommended approach for unified ETCD-based configuration.
Remove all references to WithDomainLookup.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Run Full Test Suite

**Files:**
- N/A

**Step 1: Run all middleware tests**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/middleware/... -v`
Expected: All tests PASS

**Step 2: Run all provider tests**

Run: `cd jxt-core && go test ./sdk/pkg/tenant/provider/... -v`
Expected: All tests PASS

**Step 3: Run full jxt-core tests**

Run: `cd jxt-core && go test ./...`
Expected: All tests PASS

**Step 4: Verify downstream services compile**

```bash
cd security-management && go build ./...
cd file-storage-service && go build ./...
cd evidence-management/command && go build ./...
cd evidence-management/query && go build ./...
```

Expected: All services compile without errors

**Step 5: Run linter**

Run: `cd jxt-core && go vet ./sdk/pkg/tenant/...`
Expected: No issues

---

## Summary

| Task | Description | Files Modified |
|------|-------------|----------------|
| 1 | Add ProviderConfigurer interface | `extract_tenant_id.go`, `extract_tenant_id_test.go` |
| 2 | Replace WithDomainLookup with WithProviderConfig + update all tests | `extract_tenant_id.go`, `extract_tenant_id_test.go` |
| 3 | Remove redundant initialization logic | `extract_tenant_id.go` |
| 4 | Update downstream services | `tenant_fallback.go` × 2, `tenant_resolver_perf_test.go` |
| 5 | Update package documentation + README | `extract_tenant_id.go`, `README.md` |
| 6 | Run full test suite + downstream compilation check | N/A |

## Key Files

- `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id.go` - Main implementation
- `jxt-core/sdk/pkg/tenant/middleware/extract_tenant_id_test.go` - Test cases
- `jxt-core/sdk/pkg/tenant/provider/models.go` - ResolverConfig definition (no changes needed)
- `jxt-core/sdk/pkg/tenant/provider/provider.go` - Provider implementation (no changes needed)
- `security-management/common/middleware/tenant_fallback.go` - DomainFallbackMiddleware
- `file-storage-service/common/middleware/tenant_fallback.go` - DomainFallbackMiddleware
- `file-storage-service/tests/perf/tenant_resolver_perf_test.go` - Performance test mock
- `jxt-core/README.md` - Usage documentation

## Final API

```go
// 推荐：统一 ETCD 配置（Provider 层）
router.Use(middleware.ExtractTenantID(
    middleware.WithProviderConfig(provider),
))

// 硬编码选项（无需 Provider）
router.Use(middleware.ExtractTenantID(
    middleware.WithResolverType("query"),
    middleware.WithQueryParam("tenant_id"),
))

// 组合：Provider + 硬编码覆盖（DomainFallbackMiddleware 场景）
// 选项顺序覆盖：WithProviderConfig 先从 ETCD 读取全部配置，
// WithResolverType("host") 再强制覆盖 resolverType 为 host
middleware.ExtractTenantID(
    middleware.WithProviderConfig(p),
    middleware.WithResolverType("host"),
)(c)
```
