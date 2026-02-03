package ftp

import (
	"context"
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

// Cache FTP配置缓存
type Cache struct {
	provider *provider.Provider
}

// NewCache 创建 FTP 配置缓存
func NewCache(prov *provider.Provider) *Cache {
	return &Cache{provider: prov}
}

// GetAll 获取租户所有FTP配置
func (c *Cache) GetAll(ctx context.Context, tenantID int) ([]*TenantFtpConfigDetail, error) {
	configs, ok := c.provider.GetFtpConfigs(tenantID)
	if !ok {
		return nil, fmt.Errorf("no ftp configs found for tenant %d", tenantID)
	}

	meta, ok := c.provider.GetTenantMeta(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", tenantID)
	}

	result := make([]*TenantFtpConfigDetail, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, &TenantFtpConfigDetail{
			TenantID:     cfg.TenantID,
			Code:         meta.Code,
			Name:         meta.Name,
			Username:     cfg.Username,
			PasswordHash: cfg.PasswordHash,
			Description:  cfg.Description,
			Status:       cfg.Status,
		})
	}

	return result, nil
}

// GetByUsername 通过用户名获取FTP配置
func (c *Cache) GetByUsername(ctx context.Context, username string) (*TenantFtpConfigDetail, error) {
	cfg, ok := c.provider.GetFtpConfigByUsername(username)
	if !ok {
		return nil, fmt.Errorf("ftp config not found for username %s", username)
	}

	meta, ok := c.provider.GetTenantMeta(cfg.TenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", cfg.TenantID)
	}

	return &TenantFtpConfigDetail{
		TenantID:     cfg.TenantID,
		Code:         meta.Code,
		Name:         meta.Name,
		Username:     cfg.Username,
		PasswordHash: cfg.PasswordHash,
		Description:  cfg.Description,
		Status:       cfg.Status,
	}, nil
}

// GetActive 获取租户所有活跃的FTP配置
func (c *Cache) GetActive(ctx context.Context, tenantID int) ([]*TenantFtpConfigDetail, error) {
	configs, ok := c.provider.GetActiveFtpConfigs(tenantID)
	if !ok {
		return nil, fmt.Errorf("no active ftp configs found for tenant %d", tenantID)
	}

	meta, ok := c.provider.GetTenantMeta(tenantID)
	if !ok {
		return nil, fmt.Errorf("tenant meta not found for tenant %d", tenantID)
	}

	result := make([]*TenantFtpConfigDetail, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, &TenantFtpConfigDetail{
			TenantID:     cfg.TenantID,
			Code:         meta.Code,
			Name:         meta.Name,
			Username:     cfg.Username,
			PasswordHash: cfg.PasswordHash,
			Description:  cfg.Description,
			Status:       cfg.Status,
		})
	}

	return result, nil
}
