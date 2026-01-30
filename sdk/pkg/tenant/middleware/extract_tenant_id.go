package middleware

import (
	"github.com/gin-gonic/gin"
)

// ExtractTenantID is a Gin middleware that extracts the tenant ID from the X-Tenant-ID header
// and stores it in the request context. If the header is missing or empty, it returns a 400 error.
func ExtractTenantID() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			c.JSON(400, gin.H{"error": "X-Tenant-ID header missing"})
			c.Abort()
			return
		}
		c.Set("tenant_id", tenantID)
		c.Next()
	}
}
