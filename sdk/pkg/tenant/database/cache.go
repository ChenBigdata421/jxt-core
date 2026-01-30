package database

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache provides access to tenant database configurations.
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

// GetByID retrieves the database configuration for a tenant by its ID.
// Returns ErrTenantNotFound if the tenant does not exist.
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantDatabaseConfig, error) {
	cfg, ok := c.provider.GetDatabaseConfig(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant %d database config not found", tenantID)
	}

	// Convert internal DatabaseConfig to TenantDatabaseConfig
	return &TenantDatabaseConfig{
		TenantID:        cfg.TenantID,
		Code:            cfg.Code,
		Name:            cfg.Name,
		Driver:          cfg.Driver,
		Host:            cfg.Host,
		Port:            cfg.Port,
		DbName:          cfg.DbName,
		Username:        cfg.Username,
		Password:        cfg.Password,
		SSLMode:         cfg.SSLMode,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifeTime: cfg.ConnMaxLifeTime,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		ConnectTimeout:  cfg.ConnectTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
	}, nil
}

// GetByCode retrieves the database configuration for a tenant by its code.
// Returns ErrTenantNotFound if the tenant does not exist.
// This is a placeholder implementation that will be completed in Task 8.
func (c *Cache) GetByCode(ctx context.Context, code string) (*TenantDatabaseConfig, error) {
	// Placeholder: will be implemented in Task 8
	// For now, return an error to indicate not yet implemented
	return nil, fmt.Errorf("GetByCode not yet implemented - will be linked to Provider in Task 8")
}
