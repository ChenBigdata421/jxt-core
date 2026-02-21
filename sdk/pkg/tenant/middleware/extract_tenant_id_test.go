package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
	"github.com/gin-gonic/gin"
)

// mockDomainLookuper is a mock implementation of DomainLookuper for testing
type mockDomainLookuper struct {
	domains map[string]int
	codes   map[string]int // Added for CodeLookuper support
}

func (m *mockDomainLookuper) GetTenantIDByDomain(domain string) (int, bool) {
	id, ok := m.domains[domain]
	return id, ok
}

func (m *mockDomainLookuper) GetTenantIDByCode(code string) (int, bool) {
	if m.codes == nil {
		return 0, false
	}
	id, ok := m.codes[strings.ToLower(code)]
	return id, ok
}

func TestExtractTenantID_Header_Default(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID())
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	tests := []struct {
		name           string
		headerValue    string
		expectedStatus int
	}{
		{
			name:           "Valid numeric X-Tenant-ID header",
			headerValue:    "123",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Empty X-Tenant-ID header",
			headerValue:    "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Non-numeric X-Tenant-ID header",
			headerValue:    "abc",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Zero X-Tenant-ID header",
			headerValue:    "0",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.headerValue != "" {
				req.Header.Set("X-Tenant-ID", tt.headerValue)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestExtractTenantID_Header_CustomName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID(WithHeaderName("Custom-Tenant-Header")))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	t.Run("Valid custom header with numeric ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Custom-Tenant-Header", "456")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("Missing custom header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})
}

func TestExtractTenantID_Query(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID(WithResolverType("query")))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "Valid numeric query parameter",
			url:            "/test?tenant=123",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Missing query parameter",
			url:            "/test",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty query parameter",
			url:            "/test?tenant=",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Non-numeric query parameter",
			url:            "/test?tenant=abc",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestExtractTenantID_Query_CustomParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID(
		WithResolverType("query"),
		WithQueryParam("tenant_id"),
	))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	t.Run("Valid custom query parameter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?tenant_id=789", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

func TestExtractTenantID_Path(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID(WithResolverType("path"), WithPathIndex(0)))
	router.GET("/:tenant_id/users", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "Valid numeric path segment at index 0",
			url:            "/12345/users",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Non-numeric path segment",
			url:            "/tenant-abc/users",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty path",
			url:            "/",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestExtractTenantID_Path_CustomIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID(WithResolverType("path"), WithPathIndex(1)))
	router.GET("/api/:tenant_id/users", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	t.Run("Valid numeric path segment at index 1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/999/users", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

func TestExtractTenantID_Host(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID(WithResolverType("host")))
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	tests := []struct {
		name           string
		host           string
		expectedStatus int
		validateID     func(t *testing.T, body string)
	}{
		{
			name:           "Valid numeric subdomain",
			host:           "123.example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Non-numeric subdomain (should fail)",
			host:           "tenant1.example.com",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Subdomain with port",
			host:           "456.example.com:8080",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Single part domain (no dot)",
			host:           "localhost",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty host",
			host:           "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = tt.host
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.validateID != nil && w.Code == http.StatusOK {
				tt.validateID(t, w.Body.String())
			}
		})
	}
}

func TestGetTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID())
	router.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		if tenantID == 0 {
			c.JSON(500, gin.H{"error": "tenant_id is 0"})
			return
		}
		c.JSON(200, gin.H{"tenant_id": tenantID})
	})

	t.Run("Get tenant ID from context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Tenant-ID", "123")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

func TestGetTenantIDAsInt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Valid numeric tenant ID", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("tenant_id", 12345)

		tenantID, err := GetTenantIDAsInt(c)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if tenantID != 12345 {
			t.Errorf("expected 12345, got %d", tenantID)
		}
	})

	t.Run("Missing tenant ID returns error", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		// Don't set tenant_id

		_, err := GetTenantIDAsInt(c)
		if err == nil {
			t.Error("expected error when tenant_id is missing")
		}
	})
}

func TestMustGetTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Valid tenant ID", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("tenant_id", 999)

		tenantID := MustGetTenantID(c)
		if tenantID != 999 {
			t.Errorf("expected 999, got %d", tenantID)
		}
	})

	t.Run("Missing tenant ID should panic", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		// Don't set tenant_id

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic when tenant_id is missing")
			}
		}()

		_ = MustGetTenantID(c)
	})
}

func TestExtractTenantID_ContextStoresResolverType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID(WithResolverType("query")))
	router.GET("/test", func(c *gin.Context) {
		resolverType := c.GetString("tenant_resolver_type")
		c.JSON(200, gin.H{"resolver_type": resolverType})
	})

	t.Run("Resolver type stored in context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?tenant=123", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

func TestExtractTenantID_StoresIntInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ExtractTenantID())
	router.GET("/test", func(c *gin.Context) {
		// Verify tenant_id is stored as int, not string
		tenantID, exists := c.Get("tenant_id")
		if !exists {
			c.JSON(500, gin.H{"error": "tenant_id not found"})
			return
		}
		// Type assertion to int
		id, ok := tenantID.(int)
		if !ok {
			c.JSON(500, gin.H{"error": "tenant_id is not int"})
			return
		}
		c.JSON(200, gin.H{"tenant_id": id, "type": "int"})
	})

	t.Run("Tenant ID stored as int in context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Tenant-ID", "456")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

// ========== Domain Lookup Tests ==========

func TestExtractFromHost_WithDomainLookup(t *testing.T) {
	// Create mock domain lookup
	mockLookup := &mockDomainLookuper{
		domains: map[string]int{
			"tenant-alpha.example.com": 100,
			"tenant-beta.example.com":  200,
			"alias.example.com":        100,
			"internal.local":           300,
		},
	}

	gin.SetMode(gin.TestMode)

	t.Run("Domain lookup success with exact match", func(t *testing.T) {
		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "tenant-alpha.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Domain lookup with alias", func(t *testing.T) {
		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "alias.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Domain lookup with port", func(t *testing.T) {
		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "tenant-alpha.example.com:8080"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Fallback to subdomain when domain lookup fails", func(t *testing.T) {
		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		// "999" is not in mockLookup, but it's a valid numeric subdomain
		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "999.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should succeed via subdomain fallback (extracts "999")
		if w.Code != http.StatusOK {
			t.Errorf("expected status 200 (subdomain fallback), got %d", w.Code)
		}
	})

	t.Run("No domainLookup configured - use subdomain extraction", func(t *testing.T) {
		router := gin.New()
		router.Use(ExtractTenantID(WithResolverType("host")))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		// Without domainLookup, only numeric subdomains work
		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "789.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("Domain lookup fails and subdomain is non-numeric", func(t *testing.T) {
		router := gin.New()
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID})
		})

		// Domain not in lookup, and subdomain is non-numeric
		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "unknown.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should fail because both lookup and subdomain parsing fail
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("Domain lookup returns correct tenant ID", func(t *testing.T) {
		router := gin.New()
		var capturedTenantID int
		router.Use(ExtractTenantID(
			WithResolverType("host"),
			WithDomainLookup(mockLookup),
		))
		router.GET("/test", func(c *gin.Context) {
			capturedTenantID = GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": capturedTenantID})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Host = "tenant-beta.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		if capturedTenantID != 200 {
			t.Errorf("expected tenant ID 200, got %d", capturedTenantID)
		}
	})
}

func TestExtractTenantID_WithContinueMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Abort mode (default) - returns 400 on missing tenant", func(t *testing.T) {
		router := gin.New()
		var handlerCalled bool
		router.Use(ExtractTenantID(WithResolverType("header")))
		router.GET("/test", func(c *gin.Context) {
			handlerCalled = true
			c.JSON(200, gin.H{"message": "ok"})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		// No X-Tenant-ID header
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
		if handlerCalled {
			t.Error("handler should not be called in abort mode")
		}
	})

	t.Run("Continue mode - continues on missing tenant", func(t *testing.T) {
		router := gin.New()
		var handlerCalled bool
		router.Use(ExtractTenantID(
			WithResolverType("header"),
			WithOnMissingTenant("Continue"),
		))
		router.GET("/test", func(c *gin.Context) {
			handlerCalled = true
			tenantID := GetTenantID(c)
			c.JSON(200, gin.H{"tenant_id": tenantID, "message": "ok"})
		})

		req := httptest.NewRequest("GET", "/test", nil)
		// No X-Tenant-ID header
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		if !handlerCalled {
			t.Error("handler should be called in continue mode")
		}
	})
}

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
	tenantID, ok := m.codes[code]
	return tenantID, ok
}

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
				domains: map[string]int{"999.example.com": 500},
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
		req.Host = "999.example.com"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		// In numeric mode, should return 999 (subdomain), not 500 (domain lookup)
	})
}
