package database

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache 数据库配置缓存
type Cache struct {
	provider *provider.Provider
}

// NewCache 创建数据库配置缓存
func NewCache(prov *provider.Provider) *Cache {
	return &Cache{provider: prov}
}

// GetByService 获取指定服务的数据库配置
func (c *Cache) GetByService(ctx context.Context, tenantID int, serviceCode string) (*TenantDatabaseConfig, error) {
	cfg, ok := c.provider.GetServiceDatabaseConfig(tenantID, serviceCode)
	if !ok {
		return nil, fmt.Errorf("database config not found for tenant %d, service %s", tenantID, serviceCode)
	}

	meta, ok := c.provider.GetTenantMeta(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", tenantID)
	}

	return &TenantDatabaseConfig{
		TenantID:     cfg.TenantID,
		Code:         meta.Code,
		Name:         meta.Name,
		ServiceCode:  cfg.ServiceCode,
		Driver:       cfg.Driver,
		Host:         cfg.Host,
		Port:         cfg.Port,
		DbName:       cfg.Database,
		Username:     cfg.Username,
		SSLMode:      cfg.SSLMode,
		MaxOpenConns: cfg.MaxOpenConns,
		MaxIdleConns: cfg.MaxIdleConns,
	}, nil
}

// GetAllServices 获取租户所有服务的数据库配置
func (c *Cache) GetAllServices(ctx context.Context, tenantID int) (map[string]*TenantDatabaseConfig, error) {
	configs, ok := c.provider.GetAllServiceDatabaseConfigs(tenantID)
	if !ok {
		return nil, fmt.Errorf("no database configs found for tenant %d", tenantID)
	}

	meta, ok := c.provider.GetTenantMeta(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", tenantID)
	}

	result := make(map[string]*TenantDatabaseConfig)
	for serviceCode, cfg := range configs {
		result[serviceCode] = &TenantDatabaseConfig{
			TenantID:     cfg.TenantID,
			Code:         meta.Code,
			Name:         meta.Name,
			ServiceCode:  cfg.ServiceCode,
			Driver:       cfg.Driver,
			Host:         cfg.Host,
			Port:         cfg.Port,
			DbName:       cfg.Database,
			Username:     cfg.Username,
			SSLMode:      cfg.SSLMode,
			MaxOpenConns: cfg.MaxOpenConns,
			MaxIdleConns: cfg.MaxIdleConns,
		}
	}

	return result, nil
}
