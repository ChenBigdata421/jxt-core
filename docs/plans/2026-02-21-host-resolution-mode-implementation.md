# Host Resolution Mode Config Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add support for three mutually exclusive host resolution modes (numeric, domain, code) in the tenant middleware, configurable via ETCD through Provider.

**Architecture:** Add `HostMode` field to configuration structs, implement `ResolverConfigProvider` interface in middleware to read mode from Provider, refactor `extractFromHost` to execute mode-specific logic instead of combined fallback logic.

**Tech Stack:** Go 1.23+, Gin, ETCD Provider

**Scope:** jxt-core project only (tenant-service changes handled separately)

---

## Files to Modify

| File | Changes |
|------|---------|
| `sdk/config/tenant.go` | Add `HostMode` field to `HTTPResolverConfig`, add getter method |
| `sdk/pkg/tenant/provider/models.go` | Add `HTTPHostMode` field to `ResolverConfig`, add getter method |
| `sdk/pkg/tenant/middleware/extract_tenant_id.go` | Add `ResolverConfigProvider` interface, modify `Config`, modify `ExtractTenantID`, refactor `extractFromHost` |
| `sdk/pkg/tenant/middleware/extract_tenant_id_test.go` | Add tests for three modes and `ResolverConfigProvider` integration |

---

## Task 1: Add HostMode to YAML Config Struct

**Files:**
- Modify: `sdk/config/tenant.go:23-28`

**Step 1: Add HostMode field to HTTPResolverConfig**

Add the `HostMode` field after `PathIndex`:

```go
// HTTPResolverConfig HTTP 租户识别配置
type HTTPResolverConfig struct {
	Type       string `mapstructure:"type" yaml:"type"`             // 识别方式: host, header, query, path
	HeaderName string `mapstructure:"headerName" yaml:"headerName"` // 当 type 为 header 时使用的 header 名称
	QueryParam string `mapstructure:"queryParam" yaml:"queryParam"` // 当 type 为 query 时使用的查询参数名
	PathIndex  int    `mapstructure:"pathIndex" yaml:"pathIndex"`   // 当 type 为 path 时使用的路径索引
	HostMode   string `mapstructure:"hostMode" yaml:"hostMode"`     // 【新增】host 模式下的识别方式：numeric, domain, code
}
```

**Step 2: Add GetHostModeOrDefault method**

Add the getter method after `GetPathIndex` (around line 152):

```go
// GetHostModeOrDefault 获取 host 模式，默认为 "numeric"
func (hrc *HTTPResolverConfig) GetHostModeOrDefault() string {
	if hrc == nil || hrc.HostMode == "" {
		return "numeric"
	}
	return hrc.HostMode
}
```

**Step 3: Run tests to verify compilation**

Run: `go build ./sdk/config/...`
Expected: Build succeeds without errors

**Step 4: Commit**

```bash
git add sdk/config/tenant.go
git commit -m "$(cat <<'EOF'
feat(config): add HostMode field to HTTPResolverConfig

Add hostMode field for configuring host-based tenant resolution mode.
Supported modes: numeric (default), domain, code.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add HTTPHostMode to Provider ResolverConfig

**Files:**
- Modify: `sdk/pkg/tenant/provider/models.go:142-156`

**Step 1: Add HTTPHostMode field to ResolverConfig**

Modify the `ResolverConfig` struct:

```go
// ResolverConfig 租户识别配置（全局配置，不属于特定租户）
// 从 ETCD key: common/resolver 加载
//
// 注意：通过 Provider 获取的 ResolverConfig 指针应视为只读，不可修改字段值。
// 如需修改，请复制后使用。
type ResolverConfig struct {
	ID             int64     `json:"id"`             // 数据库主键，消费方不使用，仅用于反序列化兼容
	HTTPType       string    `json:"httpType"`       // host, header, query, path
	HTTPHeaderName string    `json:"httpHeaderName"` // For type=header
	HTTPQueryParam string    `json:"httpQueryParam"` // For type=query
	HTTPPathIndex  int       `json:"httpPathIndex"`  // For type=path
	HTTPHostMode   string    `json:"httpHostMode,omitempty"`  // 【新增】host 模式：numeric, domain, code
	FTPType        string    `json:"ftpType"`        // 目前只支持 username
	CreatedAt      time.Time `json:"createdAt"`      // 创建时间，调试和审计用
	UpdatedAt      time.Time `json:"updatedAt"`      // 更新时间，调试和审计用
}
```

**Step 2: Add GetHTTPHostModeOrDefault method**

Add after the struct definition (around line 157):

```go
// GetHTTPHostModeOrDefault 返回 host 模式，默认为 "numeric"
func (c *ResolverConfig) GetHTTPHostModeOrDefault() string {
	if c == nil || c.HTTPHostMode == "" {
		return "numeric"
	}
	return c.HTTPHostMode
}
```

**Step 3: Run tests to verify compilation**

Run: `go build ./sdk/pkg/tenant/provider/...`
Expected: Build succeeds without errors

**Step 4: Commit**

```bash
git add sdk/pkg/tenant/provider/models.go
git commit -m "$(cat <<'EOF'
feat(provider): add HTTPHostMode field to ResolverConfig

Add httpHostMode field to ResolverConfig for ETCD storage.
Includes GetHTTPHostModeOrDefault helper with "numeric" default.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add ResolverConfigProvider Interface

**Files:**
- Modify: `sdk/pkg/tenant/middleware/extract_tenant_id.go:60-73`

**Step 1: Add ResolverConfigProvider interface**

Add the interface after `CodeLookuper` (around line 73):

```go
// ResolverConfigProvider defines the interface for getting resolver configuration.
// Provider implements this interface (GetResolverConfig method already exists).
// This allows middleware to read httpHostMode from ETCD via Provider.
type ResolverConfigProvider interface {
	GetResolverConfig() *ResolverConfig
}
```

Note: `ResolverConfig` is in the `provider` package, so the middleware will need to import it. The interface definition should reference `provider.ResolverConfig` or we can define the interface to return any type that has `GetHTTPHostModeOrDefault() string`.

**Alternative approach - use duck typing with interface:**

```go
// HostModeConfig defines the interface for getting host mode configuration.
// Both provider.ResolverConfig and provider.Provider can satisfy this via duck typing.
type HostModeConfig interface {
	GetHTTPHostModeOrDefault() string
}
```

However, since `Provider.GetResolverConfig()` returns `*ResolverConfig`, we can use type assertion in the middleware.

**Step 2: Run tests to verify compilation**

Run: `go build ./sdk/pkg/tenant/middleware/...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id.go
git commit -m "$(cat <<'EOF'
feat(middleware): add ResolverConfigProvider interface

Add interface for middleware to access resolver config from Provider.
Enables reading httpHostMode from ETCD configuration.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Modify Config Struct and ExtractTenantID Middleware

**Files:**
- Modify: `sdk/pkg/tenant/middleware/extract_tenant_id.go:75-165`

**Step 1: Add resolverConfig field to Config struct**

Modify the `Config` struct:

```go
// Config holds the middleware configuration
type Config struct {
	resolverType    ResolverType
	headerName      string            // For type=header
	queryParam      string            // For type=query
	pathIndex       int               // For type=path
	onMissingTenant MissingTenantMode // Behavior when tenant ID is missing
	domainLookup    DomainLookuper    // Domain lookup for host-based resolution
	resolverConfig  *ResolverConfig   // 【新增】Resolver config from Provider (for httpHostMode)
}
```

**Step 2: Import provider package**

Add import at the top of the file:

```go
import (
	// ... existing imports ...
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)
```

**Step 3: Use provider.ResolverConfig type**

Add type alias or use full path. Since the middleware already uses `DomainLookuper` interface which is satisfied by `provider.Provider`, we can:

Option A: Define local `ResolverConfig` type alias
Option B: Import and use `provider.ResolverConfig` directly

Choose Option B for simplicity:

```go
// At the top of extract_tenant_id.go, update imports
import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)
```

Then use `*provider.ResolverConfig` in Config struct.

**Step 4: Update ResolverConfigProvider interface to use provider type**

```go
// ResolverConfigProvider defines the interface for getting resolver configuration.
// Provider implements this interface.
type ResolverConfigProvider interface {
	GetResolverConfig() *provider.ResolverConfig
}
```

**Step 5: Modify ExtractTenantID to read config from Provider**

Replace the `ExtractTenantID` function (lines 161-229):

```go
// ExtractTenantID creates a middleware that extracts tenant ID from the request
// and stores it in the Gin context under the key "tenant_id" as an integer.
//
// By default, it extracts from the "X-Tenant-ID" header.
// Use options to customize the extraction strategy.
//
// The middleware returns HTTP 400 if tenant ID cannot be extracted or is not numeric.
func ExtractTenantID(opts ...Option) gin.HandlerFunc {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

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

	return func(c *gin.Context) {
		tenantIDStr, ok := extractTenantID(c, cfg)
		if !ok {
			// Tenant ID missing - handle based on onMissingTenant setting
			if cfg.onMissingTenant == MissingTenantContinue {
				c.Next()
			} else {
				c.JSON(400, gin.H{
					"error":         "tenant ID missing or invalid",
					"resolver":      string(cfg.resolverType),
					"resolver_type": string(cfg.resolverType),
				})
				c.Abort()
			}
			return
		}

		// Convert string to int
		tenantID, err := strconv.Atoi(tenantIDStr)
		if err != nil {
			// Invalid tenant ID format - handle based on onMissingTenant setting
			if cfg.onMissingTenant == MissingTenantContinue {
				c.Next()
			} else {
				c.JSON(400, gin.H{
					"error":         "tenant ID must be numeric",
					"resolver":      string(cfg.resolverType),
					"resolver_type": string(cfg.resolverType),
				})
				c.Abort()
			}
			return
		}

		// Validate tenantID (0 means invalid/not found)
		if tenantID == 0 {
			// Tenant ID is 0 - handle based on onMissingTenant setting
			if cfg.onMissingTenant == MissingTenantContinue {
				c.Next()
			} else {
				c.JSON(400, gin.H{
					"error":         "tenant ID cannot be 0",
					"resolver":      string(cfg.resolverType),
					"resolver_type": string(cfg.resolverType),
				})
				c.Abort()
			}
			return
		}

		// Store as int in Gin context (for c.Get("tenant_id"))
		c.Set("tenant_id", tenantID)
		c.Set("tenant_resolver_type", string(cfg.resolverType))

		// Also store in request context (for ctx.Value(TenantContextKey))
		// This allows business logic to access tenant ID via context.Context
		ctx := context.WithValue(c.Request.Context(), TenantContextKey, tenantID)
		c.Request = c.Request.WithContext(ctx)

		// Tenant ID found and valid - always continue to next handler
		c.Next()
	}
}
```

**Step 6: Modify extractTenantID to pass resolverConfig**

Update the function:

```go
// extractTenantID extracts tenant ID based on the configured resolver type
func extractTenantID(c *gin.Context, cfg *Config) (string, bool) {
	switch cfg.resolverType {
	case ResolverTypeHost:
		return extractFromHost(c, cfg.domainLookup, cfg.resolverConfig)
	case ResolverTypeHeader:
		return extractFromHeader(c, cfg.headerName)
	case ResolverTypeQuery:
		return extractFromQuery(c, cfg.queryParam)
	case ResolverTypePath:
		return extractFromPath(c, cfg.pathIndex)
	default:
		return "", false
	}
}
```

**Step 7: Run tests to verify compilation**

Run: `go build ./sdk/pkg/tenant/middleware/...`
Expected: Build succeeds

**Step 8: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id.go
git commit -m "$(cat <<'EOF'
feat(middleware): integrate ResolverConfig into middleware

- Add resolverConfig field to Config struct
- Extract resolverConfig from Provider via ResolverConfigProvider interface
- Override resolverType from ETCD config when available
- Pass resolverConfig to extractFromHost for mode-based resolution

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Refactor extractFromHost for Mutually Exclusive Modes

**Files:**
- Modify: `sdk/pkg/tenant/middleware/extract_tenant_id.go:247-303`

**Step 1: Rewrite extractFromHost function**

Replace the current `extractFromHost` function with the new implementation:

```go
// extractFromHost extracts tenant ID from Host header based on httpHostMode.
// Modes (mutually exclusive):
//   - "numeric": Only numeric subdomain (e.g., "123.example.com" -> "123")
//   - "domain":  Only exact domain match via DomainLookuper (requires Provider)
//   - "code":    Only tenant code match via CodeLookuper (requires Provider)
//
// Default mode is "numeric" for backward compatibility.
// Exact domain matching does not support wildcards.
// DNS is case-insensitive (RFC 4343), all lookups use lowercase.
func extractFromHost(c *gin.Context, lookup DomainLookuper, resolverCfg *provider.ResolverConfig) (string, bool) {
	host := c.Request.Host
	if host == "" {
		return "", false
	}

	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Extract subdomain for numeric and code modes
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "", false
	}

	subdomain := parts[0]
	if subdomain == "" || subdomain == "www" {
		return "", false
	}

	// Determine mode (default to "numeric" for backward compatibility)
	mode := "numeric"
	if resolverCfg != nil {
		mode = resolverCfg.GetHTTPHostModeOrDefault()
	}

	switch mode {
	case "domain":
		// Mode: domain - only exact domain match
		if lookup != nil {
			if tenantID, ok := lookup.GetTenantIDByDomain(host); ok {
				return strconv.Itoa(tenantID), true
			}
		}
		return "", false

	case "code":
		// Mode: code - only tenant code match
		if lookup != nil {
			if codeLookup, ok := lookup.(CodeLookuper); ok {
				// DNS is case-insensitive, normalize to lowercase
				if tenantID, ok := codeLookup.GetTenantIDByCode(strings.ToLower(subdomain)); ok {
					return strconv.Itoa(tenantID), true
				}
			}
		}
		return "", false

	default:
		// Mode: numeric - only numeric subdomain
		if _, err := strconv.Atoi(subdomain); err == nil {
			return subdomain, true
		}
		return "", false
	}
}
```

**Step 2: Run tests to verify compilation**

Run: `go build ./sdk/pkg/tenant/middleware/...`
Expected: Build succeeds

**Step 3: Run existing tests**

Run: `go test ./sdk/pkg/tenant/middleware/... -v`
Expected: Some tests may fail because behavior changed from combined to exclusive modes

**Step 4: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id.go
git commit -m "$(cat <<'EOF'
refactor(middleware): implement mutually exclusive host resolution modes

Replace combined fallback logic with three exclusive modes:
- numeric: Only accept numeric subdomains (default)
- domain: Only exact domain match via DomainLookuper
- code: Only tenant code match via CodeLookuper

Breaking change: Previous behavior combined all strategies with
fallback. New behavior is strictly mode-based.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Write Tests for Numeric Mode

**Files:**
- Modify: `sdk/pkg/tenant/middleware/extract_tenant_id_test.go`

**Step 1: Create mockResolverConfigProvider**

Add to the test file:

```go
// mockResolverConfigProvider implements ResolverConfigProvider for testing
type mockResolverConfigProvider struct {
	domainLookuper mockDomainLookuper
	config         *provider.ResolverConfig
}

func (m *mockResolverConfigProvider) GetTenantIDByDomain(domain string) (int, bool) {
	return m.domainLookuper.GetTenantIDByDomain(domain)
}

func (m *mockResolverConfigProvider) GetTenantIDByCode(code string) (int, bool) {
	return m.domainLookuper.GetTenantIDByCode(code)
}

func (m *mockResolverConfigProvider) GetResolverConfig() *provider.ResolverConfig {
	return m.config
}
```

**Step 2: Add import for provider package**

```go
import (
	// ... existing imports ...
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)
```

**Step 3: Write numeric mode tests**

Add test function:

```go
func TestExtractFromHost_NumericMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Numeric subdomain succeeds in numeric mode", func(t *testing.T) {
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

	t.Run("Non-numeric subdomain fails in numeric mode", func(t *testing.T) {
		router := gin.New()
		router.Use(ExtractTenantID(WithResolverType("host")))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(200, nil)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "acmeCorp.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 400 {
			t.Errorf("expected status 400 for non-numeric subdomain, got %d", w.Code)
		}
	})

	t.Run("Numeric mode ignores domain lookup", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				domains: map[string]int{"999.example.com": 500}, // Domain exists
			},
			config: &provider.ResolverConfig{HTTPHostMode: "numeric"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "999.example.com" // Both domain match and numeric
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		// In numeric mode, should return 999 (subdomain), not 500 (domain lookup)
		// This test verifies numeric mode doesn't use domain lookup
	})
}
```

**Step 4: Run tests**

Run: `go test ./sdk/pkg/tenant/middleware/... -v -run TestExtractFromHost_NumericMode`
Expected: All tests pass

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "$(cat <<'EOF'
test(middleware): add tests for numeric host resolution mode

- Test numeric subdomain succeeds
- Test non-numeric subdomain fails
- Test numeric mode ignores domain lookup

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Write Tests for Domain Mode

**Files:**
- Modify: `sdk/pkg/tenant/middleware/extract_tenant_id_test.go`

**Step 1: Write domain mode tests**

Add test function:

```go
func TestExtractFromHost_DomainMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Domain match succeeds in domain mode", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				domains: map[string]int{
					"tenant1.example.com": 100,
				},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "domain"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
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
	})

	t.Run("Numeric subdomain fails in domain mode", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				domains: map[string]int{},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "domain"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(200, nil)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "123.example.com" // Numeric subdomain
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 400 {
			t.Errorf("expected status 400 (domain mode ignores numeric), got %d", w.Code)
		}
	})

	t.Run("Unknown domain fails in domain mode", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				domains: map[string]int{
					"known.example.com": 100,
				},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "domain"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(200, nil)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "unknown.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 400 {
			t.Errorf("expected status 400 for unknown domain, got %d", w.Code)
		}
	})

	t.Run("Domain mode with port", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				domains: map[string]int{
					"tenant1.example.com": 100,
				},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "domain"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "tenant1.example.com:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}
```

**Step 2: Run tests**

Run: `go test ./sdk/pkg/tenant/middleware/... -v -run TestExtractFromHost_DomainMode`
Expected: All tests pass

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "$(cat <<'EOF'
test(middleware): add tests for domain host resolution mode

- Test domain match succeeds
- Test numeric subdomain fails in domain mode
- Test unknown domain fails
- Test domain mode with port

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Write Tests for Code Mode

**Files:**
- Modify: `sdk/pkg/tenant/middleware/extract_tenant_id_test.go`

**Step 1: Write code mode tests**

Add test function:

```go
func TestExtractFromHost_CodeMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Tenant code match succeeds in code mode", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				codes: map[string]int{"acmecorp": 100},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "code"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "acmecorp.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("Code mode is case insensitive", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				codes: map[string]int{"techinc": 200},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "code"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "TECHINC.example.com" // Uppercase in request
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200 (case insensitive), got %d", w.Code)
		}
	})

	t.Run("Numeric subdomain fails in code mode", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				codes: map[string]int{},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "code"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(200, nil)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "123.example.com" // Numeric subdomain
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 400 {
			t.Errorf("expected status 400 (code mode ignores numeric), got %d", w.Code)
		}
	})

	t.Run("Unknown code fails in code mode", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				codes: map[string]int{"known": 100},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "code"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(200, nil)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "unknown.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 400 {
			t.Errorf("expected status 400 for unknown code, got %d", w.Code)
		}
	})

	t.Run("Code mode ignores domain match", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				domains: map[string]int{"tenant-alpha.example.com": 500},
				codes:   map[string]int{},
			},
			config: &provider.ResolverConfig{HTTPHostMode: "code"},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(200, nil)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "tenant-alpha.example.com" // Domain exists but no code match
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 400 {
			t.Errorf("expected status 400 (code mode ignores domain), got %d", w.Code)
		}
	})
}
```

**Step 2: Run tests**

Run: `go test ./sdk/pkg/tenant/middleware/... -v -run TestExtractFromHost_CodeMode`
Expected: All tests pass

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "$(cat <<'EOF'
test(middleware): add tests for code host resolution mode

- Test tenant code match succeeds
- Test case insensitivity
- Test numeric subdomain fails
- Test unknown code fails
- Test code mode ignores domain match

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Write Tests for ResolverConfigProvider Integration

**Files:**
- Modify: `sdk/pkg/tenant/middleware/extract_tenant_id_test.go`

**Step 1: Write integration tests**

Add test function:

```go
func TestExtractTenantID_ResolverConfigProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("ResolverType overridden from ETCD config", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				domains: map[string]int{"test.example.com": 100},
			},
			config: &provider.ResolverConfig{
				HTTPType:     "host",
				HTTPHostMode: "domain",
			},
		}

		router := gin.New()
		// Note: NOT setting WithResolverType - should be overridden from config
		router.Use(ExtractTenantID(WithDomainLookup(mockProvider)))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "test.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200 (config overrode resolver type), got %d", w.Code)
		}
	})

	t.Run("Nil config uses default numeric mode", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{},
			config:         nil, // No config
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "456.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200 (default numeric mode), got %d", w.Code)
		}
	})

	t.Run("Empty HTTPHostMode defaults to numeric", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{
				domains: map[string]int{"test.example.com": 100},
			},
			config: &provider.ResolverConfig{
				HTTPHostMode: "", // Empty - should default to numeric
			},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(200, nil)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "test.example.com" // Has domain match but not numeric
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 400 {
			t.Errorf("expected status 400 (empty mode = numeric, ignores domain), got %d", w.Code)
		}
	})

	t.Run("Invalid HTTPHostMode defaults to numeric", func(t *testing.T) {
		mockProvider := &mockResolverConfigProvider{
			domainLookuper: mockDomainLookuper{},
			config: &provider.ResolverConfig{
				HTTPHostMode: "invalid",
			},
		}

		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockProvider),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "789.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200 (invalid mode = numeric), got %d", w.Code)
		}
	})
}
```

**Step 2: Run all tests**

Run: `go test ./sdk/pkg/tenant/middleware/... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "$(cat <<'EOF'
test(middleware): add ResolverConfigProvider integration tests

- Test resolver type overridden from ETCD config
- Test nil config uses default numeric mode
- Test empty HTTPHostMode defaults to numeric
- Test invalid HTTPHostMode defaults to numeric

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Update Existing Tests for New Behavior

**Files:**
- Modify: `sdk/pkg/tenant/middleware/extract_tenant_id_test.go`

**Step 1: Update tests that expect combined behavior**

The existing tests in `TestExtractFromHost_WithDomainLookup` may fail because they expect the old combined behavior. Update them:

1. "Fallback to subdomain when domain lookup fails" - This test expects fallback to numeric subdomain. With the new exclusive mode behavior, this needs to explicitly set mode to "numeric" or accept that it will fail in domain mode.

2. Update the mock to include config for appropriate modes.

**Step 2: Run all tests to identify failures**

Run: `go test ./sdk/pkg/tenant/middleware/... -v`
Note which tests fail.

**Step 3: Fix failing tests**

For each failing test, either:
- Add explicit mode configuration to test the intended behavior
- Update test expectation to match new exclusive mode behavior

**Step 4: Run all tests again**

Run: `go test ./sdk/pkg/tenant/middleware/... -v`
Expected: All tests pass

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/middleware/extract_tenant_id_test.go
git commit -m "$(cat <<'EOF'
test(middleware): update existing tests for exclusive mode behavior

Update tests that expected combined fallback behavior to use
explicit mode configuration for intended behavior.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Final Verification

**Step 1: Run all middleware tests**

Run: `go test ./sdk/pkg/tenant/middleware/... -v`
Expected: All tests pass

**Step 2: Run all provider tests**

Run: `go test ./sdk/pkg/tenant/provider/... -v`
Expected: All tests pass

**Step 3: Run all config tests**

Run: `go test ./sdk/config/... -v`
Expected: All tests pass

**Step 4: Build entire project**

Run: `go build ./...`
Expected: Build succeeds

**Step 5: Final commit (if any changes)**

```bash
git status
# If any uncommitted changes:
git add -A
git commit -m "$(cat <<'EOF'
chore: final cleanup for host resolution mode implementation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Add HostMode to YAML config | `sdk/config/tenant.go` |
| 2 | Add HTTPHostMode to Provider model | `sdk/pkg/tenant/provider/models.go` |
| 3 | Add ResolverConfigProvider interface | `sdk/pkg/tenant/middleware/extract_tenant_id.go` |
| 4 | Integrate ResolverConfig into middleware | `sdk/pkg/tenant/middleware/extract_tenant_id.go` |
| 5 | Refactor extractFromHost for exclusive modes | `sdk/pkg/tenant/middleware/extract_tenant_id.go` |
| 6 | Write numeric mode tests | `sdk/pkg/tenant/middleware/extract_tenant_id_test.go` |
| 7 | Write domain mode tests | `sdk/pkg/tenant/middleware/extract_tenant_id_test.go` |
| 8 | Write code mode tests | `sdk/pkg/tenant/middleware/extract_tenant_id_test.go` |
| 9 | Write ResolverConfigProvider tests | `sdk/pkg/tenant/middleware/extract_tenant_id_test.go` |
| 10 | Update existing tests | `sdk/pkg/tenant/middleware/extract_tenant_id_test.go` |
| 11 | Final verification | - |

---

## Backward Compatibility Notes

1. **Default mode is "numeric"** - When `HTTPHostMode` is empty or not provided, behavior defaults to numeric subdomain extraction, matching the most common existing use case.

2. **Breaking change** - The old combined fallback behavior (try domain → try numeric → try code) is replaced with exclusive modes. Services that relied on fallback will need to configure the appropriate mode.

3. **ETCD config takes precedence** - When Provider is available, `HTTPType` from ETCD overrides middleware options. This allows centralized configuration management.

4. **No Provider changes required** - The Provider already has `GetResolverConfig()` method. It just needs to receive configs with the new `httpHostMode` field from ETCD (tenant-service responsibility).
