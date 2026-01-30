package ftp

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache provides access to tenant FTP configurations.
// It uses the Provider to fetch tenant data and caches the configurations.
type Cache struct {
	provider *provider.Provider
}

// NewCache creates a new Cache instance with the given Provider.
func NewCache(prov *provider.Provider) *Cache {
	return &Cache{
		provider: prov,
	}
}

// GetByID retrieves the FTP configuration for a tenant by its ID.
// Returns ErrTenantNotFound if the tenant does not exist.
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantFtpConfig, error) {
	cfg, ok := c.provider.GetFtpConfig(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant %d FTP config not found", tenantID)
	}

	return &TenantFtpConfig{
		TenantID:        cfg.TenantID,
		Code:            cfg.Code,
		Name:            cfg.Name,
		Username:        cfg.Username,
		PasswordHash:    cfg.PasswordHash,
		HomeDirectory:   cfg.HomeDirectory,
		WritePermission: cfg.WritePermission,
	}, nil
}
