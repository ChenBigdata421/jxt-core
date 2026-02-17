// Package middleware provides tenant ID extraction middleware for Gin framework.
//
// Supported resolver types:
//   - "host":   Extract tenant ID from Host header (domain-based)
//   - "header": Extract tenant ID from a custom header (default: X-Tenant-ID)
//   - "query":  Extract tenant ID from URL query parameter (default: tenant)
//   - "path":   Extract tenant ID from URL path by index
//
// Usage:
//
//	// Simple: Use default header-based extraction
//	router.Use(middleware.ExtractTenantID())
//
//	// Advanced: Configure resolver type and options
//	router.Use(middleware.ExtractTenantID(middleware.WithResolverType("host")))
//	router.Use(middleware.ExtractTenantID(
//	    middleware.WithResolverType("query"),
//	    middleware.WithQueryParam("tenant_id"),
//	))
package middleware

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// contextKey is the type for context keys in this package
type contextKey string

// TenantContextKey is the context key for tenant ID in request context
// Business code can use this key to retrieve tenant ID via ctx.Value()
// Example: tenantID := ctx.Value(middleware.TenantContextKey).(int)
const TenantContextKey contextKey = "JXT-Tenant"

// ResolverType defines the tenant ID extraction strategy
type ResolverType string

const (
	ResolverTypeHost   ResolverType = "host"   // Extract from Host header
	ResolverTypeHeader ResolverType = "header" // Extract from custom header
	ResolverTypeQuery  ResolverType = "query"  // Extract from query parameter
	ResolverTypePath   ResolverType = "path"   // Extract from path segment
)

// MissingTenantMode defines the behavior when tenant ID is missing or invalid
type MissingTenantMode string

const (
	// MissingTenantAbort calls c.Abort() when tenant ID is missing (default)
	// Use this when tenant ID is required
	MissingTenantAbort MissingTenantMode = "Abort"
	// MissingTenantContinue calls c.Next() when tenant ID is missing
	// Use this when tenant ID is optional and you want to continue processing
	MissingTenantContinue MissingTenantMode = "Continue"
)

// DomainLookuper defines the domain lookup interface for tenant ID resolution.
// It is implicitly implemented by provider.Provider (duck typing).
// This interface follows the Go convention of "consumer defines interface".
type DomainLookuper interface {
	GetTenantIDByDomain(domain string) (int, bool)
}

// Config holds the middleware configuration
type Config struct {
	resolverType     ResolverType
	headerName       string            // For type=header
	queryParam       string            // For type=query
	pathIndex        int               // For type=path
	onMissingTenant  MissingTenantMode // Behavior when tenant ID is missing
	domainLookup     DomainLookuper    // Domain lookup for host-based resolution
}

// Option is a function that configures the tenant ID extraction
type Option func(*Config)

// WithResolverType sets the resolver type (host/header/query/path)
// Default: "header"
func WithResolverType(resolverType string) Option {
	return func(c *Config) {
		c.resolverType = ResolverType(resolverType)
	}
}

// WithHeaderName sets the header name for type=header
// Default: "X-Tenant-ID"
func WithHeaderName(name string) Option {
	return func(c *Config) {
		c.headerName = name
	}
}

// WithQueryParam sets the query parameter name for type=query
// Default: "tenant"
func WithQueryParam(param string) Option {
	return func(c *Config) {
		c.queryParam = param
	}
}

// WithPathIndex sets the path segment index for type=path
// Default: 0 (first segment after leading slash)
// Example: /tenant-123/users -> pathIndex=0 extracts "tenant-123"
func WithPathIndex(index int) Option {
	return func(c *Config) {
		c.pathIndex = index
	}
}

// WithOnMissingTenant sets the behavior when tenant ID is missing or invalid
// Default: "Abort" (calls c.Abort())
// Options:
//   - "Abort": calls c.Abort() to stop the middleware chain (tenant required)
//   - "Continue": calls c.Next() to continue to the next middleware/handler (tenant optional)
func WithOnMissingTenant(mode string) Option {
	return func(c *Config) {
		c.onMissingTenant = MissingTenantMode(mode)
	}
}

// WithDomainLookup enables domain lookup for host-based tenant resolution.
// When resolverType is "host", it will first attempt to find tenant ID by
// exact domain match using the provided lookup. If lookup fails, it falls
// back to subdomain extraction.
// The lookup parameter should implement DomainLookuper interface (e.g., provider.Provider).
func WithDomainLookup(lookup DomainLookuper) Option {
	return func(c *Config) {
		c.domainLookup = lookup
	}
}

// defaultConfig returns the default configuration
func defaultConfig() *Config {
	return &Config{
		resolverType:    ResolverTypeHeader,
		headerName:      "X-Tenant-ID",
		queryParam:      "tenant",
		pathIndex:       0,
		onMissingTenant: MissingTenantAbort, // Default: call c.Abort()
	}
}

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

// extractTenantID extracts tenant ID based on the configured resolver type
func extractTenantID(c *gin.Context, cfg *Config) (string, bool) {
	switch cfg.resolverType {
	case ResolverTypeHost:
		return extractFromHost(c, cfg.domainLookup)
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

// extractFromHost extracts tenant ID from Host header
// Priority:
//  1. If domainLookup is configured, attempt exact domain match first
//  2. Fall back to subdomain extraction (e.g., tenant1.example.com -> tenant1)
//
// Exact domain matching does not support wildcards.
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

	// Return the subdomain as tenant ID
	// Example: tenant1.example.com -> tenant1
	subdomain := parts[0]
	if subdomain == "" || subdomain == "www" {
		return "", false
	}

	return subdomain, true
}

// extractFromHeader extracts tenant ID from a custom header
func extractFromHeader(c *gin.Context, headerName string) (string, bool) {
	tenantID := c.GetHeader(headerName)
	return tenantID, tenantID != ""
}

// extractFromQuery extracts tenant ID from URL query parameter
func extractFromQuery(c *gin.Context, queryParam string) (string, bool) {
	tenantID := c.Query(queryParam)
	return tenantID, tenantID != ""
}

// extractFromPath extracts tenant ID from URL path by index
// Path segments are 0-indexed after the leading slash
// Example: /tenant-123/users with pathIndex=0 -> "tenant-123"
func extractFromPath(c *gin.Context, pathIndex int) (string, bool) {
	path := c.Request.URL.Path
	if path == "" || path == "/" {
		return "", false
	}

	// Remove leading slash and split
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")

	if pathIndex >= len(parts) || pathIndex < 0 {
		return "", false
	}

	tenantID := parts[pathIndex]
	return tenantID, tenantID != ""
}

// GetTenantID retrieves the tenant ID from the Gin context as int
// Returns 0 if not found
func GetTenantID(c *gin.Context) int {
	if id, exists := c.Get("tenant_id"); exists {
		return id.(int)
	}
	return 0
}

// GetTenantIDAsInt retrieves tenant ID with error
// Returns error if tenant ID is 0 (not found)
func GetTenantIDAsInt(c *gin.Context) (int, error) {
	id := GetTenantID(c)
	if id == 0 {
		return 0, errors.New("tenant ID not found in context")
	}
	return id, nil
}

// MustGetTenantID retrieves tenant ID or panics
// Panics if not found (useful for handlers that require tenant ID)
func MustGetTenantID(c *gin.Context) int {
	id := GetTenantID(c)
	if id == 0 {
		panic("tenant ID not found in context")
	}
	return id
}
