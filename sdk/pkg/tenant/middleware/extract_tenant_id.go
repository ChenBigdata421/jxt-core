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
// Usage:
//
//	// Simple: Use default header-based extraction
//	router.Use(middleware.ExtractTenantID())
//
//	// With Provider: Automatically reads httpType and httpHostMode from ETCD
//	router.Use(middleware.ExtractTenantID(
//	    middleware.WithProviderConfig(provider),
//	))
//
//	// Without Provider: Explicit resolver type
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

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
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

// CodeLookuper defines the tenant code lookup interface for subdomain-based resolution.
// Provider implicitly implements this interface (duck typing).
// Using a separate interface instead of extending DomainLookuper avoids breaking changes.
type CodeLookuper interface {
	GetTenantIDByCode(code string) (int, bool)
}

// ResolverConfigProvider defines the interface for getting resolver configuration.
// Provider implements this interface (GetResolverConfig method already exists).
// This allows middleware to read httpHostMode from ETCD via Provider.
type ResolverConfigProvider interface {
	GetResolverConfig() *provider.ResolverConfig
}

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

// Config holds the middleware configuration
type Config struct {
	resolverType     ResolverType
	headerName       string            // For type=header
	queryParam       string            // For type=query
	pathIndex        int               // For type=path
	onMissingTenant  MissingTenantMode // Behavior when tenant ID is missing
	domainLookup     DomainLookuper    // Domain lookup for host-based resolution
	resolverConfig   *provider.ResolverConfig // Resolver config from Provider (for httpHostMode)
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
