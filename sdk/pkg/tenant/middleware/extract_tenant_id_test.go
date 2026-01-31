package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

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
