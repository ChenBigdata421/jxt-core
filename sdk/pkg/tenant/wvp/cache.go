package wvp

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache provides access to tenant WVP configurations.
type Cache struct {
	provider *provider.Provider
}

// NewCache creates a new Cache instance with the given Provider.
func NewCache(prov *provider.Provider) *Cache {
	return &Cache{
		provider: prov,
	}
}

// GetByID retrieves the WVP configuration for a tenant by its ID.
// Returns an error if the tenant does not have a WVP configuration.
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantWvpConfig, error) {
	cfg, ok := c.provider.GetWvpConfig(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant %d WVP config not found", tenantID)
	}

	result := &TenantWvpConfig{
		TenantID: cfg.TenantID,
		ApiUrl:   cfg.ApiUrl,
		Realm:    cfg.Realm,
	}

	if meta, ok := c.provider.GetTenantMeta(tenantID); ok {
		result.Code = meta.Code
		result.Name = meta.Name
	}

	return result, nil
}
