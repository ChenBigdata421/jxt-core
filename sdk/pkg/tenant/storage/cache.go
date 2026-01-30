package storage

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache provides access to tenant storage configurations.
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

// GetByID retrieves the storage configuration for a tenant by its ID.
// Returns an error if the tenant does not exist.
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantStorageConfig, error) {
	cfg, ok := c.provider.GetStorageConfig(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant %d storage config not found", tenantID)
	}

	return &TenantStorageConfig{
		TenantID:             int(cfg.TenantID),
		Code:                 cfg.Code,
		Name:                 cfg.Name,
		QuotaBytes:           cfg.QuotaBytes,
		MaxFileSizeBytes:     cfg.MaxFileSizeBytes,
		MaxConcurrentUploads: cfg.MaxConcurrentUploads,
	}, nil
}
