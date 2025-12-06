package congfig

import (
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
)

// TestTenants_GetActiveTenants 测试获取所有激活的租户
func TestTenants_GetActiveTenants(t *testing.T) {
	cfg := createTestTenantsConfig()

	t.Run("获取激活的租户", func(t *testing.T) {
		active := cfg.GetActiveTenants()
		assert.Len(t, active, 2)

		ids := make([]string, len(active))
		for i, tenant := range active {
			ids[i] = tenant.ID
		}
		assert.Contains(t, ids, "tenant-001")
		assert.Contains(t, ids, "tenant-002")
		assert.NotContains(t, ids, "tenant-003")
	})

	t.Run("nil 配置返回 nil", func(t *testing.T) {
		var nilCfg *config.Tenants
		active := nilCfg.GetActiveTenants()
		assert.Nil(t, active)
	})
}

// TestTenants_GetTenantsStorageConfig 测试获取多租户存储配置
func TestTenants_GetTenantsStorageConfig(t *testing.T) {
	t.Run("启用多租户时返回存储配置", func(t *testing.T) {
		cfg := createTestTenantsConfig()
		storage := cfg.GetTenantsStorageConfig()
		assert.NotNil(t, storage)
		assert.Equal(t, "tenants", storage.Directory)
	})

	t.Run("未启用多租户时返回 nil", func(t *testing.T) {
		cfg := &config.Tenants{Enabled: false}
		storage := cfg.GetTenantsStorageConfig()
		assert.Nil(t, storage)
	})

	t.Run("Storage 为 nil 时返回 nil", func(t *testing.T) {
		cfg := &config.Tenants{Enabled: true, Storage: nil}
		storage := cfg.GetTenantsStorageConfig()
		assert.Nil(t, storage)
	})

	t.Run("nil 配置返回 nil", func(t *testing.T) {
		var nilCfg *config.Tenants
		storage := nilCfg.GetTenantsStorageConfig()
		assert.Nil(t, storage)
	})
}

// TestTenantsStorageConfig_GetStorageDirectory 测试获取存储目录
func TestTenantsStorageConfig_GetStorageDirectory(t *testing.T) {
	t.Run("nil 配置返回默认值", func(t *testing.T) {
		var cfg *config.TenantsStorageConfig
		assert.Equal(t, "tenants", cfg.GetStorageDirectory())
	})

	t.Run("空字符串返回默认值", func(t *testing.T) {
		cfg := &config.TenantsStorageConfig{Directory: ""}
		assert.Equal(t, "tenants", cfg.GetStorageDirectory())
	})

	t.Run("返回配置的值", func(t *testing.T) {
		cfg := &config.TenantsStorageConfig{Directory: "custom_dir"}
		assert.Equal(t, "custom_dir", cfg.GetStorageDirectory())
	})
}

// TestTenantsStorageConfig_GetCacheRefreshInterval 测试获取缓存刷新间隔
func TestTenantsStorageConfig_GetCacheRefreshInterval(t *testing.T) {
	t.Run("nil 配置返回默认值 300", func(t *testing.T) {
		var cfg *config.TenantsStorageConfig
		assert.Equal(t, 300, cfg.GetCacheRefreshInterval())
	})

	t.Run("0 值返回默认值 300", func(t *testing.T) {
		cfg := &config.TenantsStorageConfig{CacheRefreshInterval: 0}
		assert.Equal(t, 300, cfg.GetCacheRefreshInterval())
	})

	t.Run("负值返回默认值 300", func(t *testing.T) {
		cfg := &config.TenantsStorageConfig{CacheRefreshInterval: -1}
		assert.Equal(t, 300, cfg.GetCacheRefreshInterval())
	})

	t.Run("返回配置的值", func(t *testing.T) {
		cfg := &config.TenantsStorageConfig{CacheRefreshInterval: 600}
		assert.Equal(t, 600, cfg.GetCacheRefreshInterval())
	})
}

// TestTenantsStorageConfig_Defaults 测试存储默认值
func TestTenantsStorageConfig_Defaults(t *testing.T) {
	t.Run("GetDefaultUploadQuotaGB", func(t *testing.T) {
		var nilCfg *config.TenantsStorageConfig
		assert.Equal(t, 100, nilCfg.GetDefaultUploadQuotaGB())

		cfg := &config.TenantsStorageConfig{Defaults: config.StorageMultiTenantDefaults{UploadQuotaGB: 500}}
		assert.Equal(t, 500, cfg.GetDefaultUploadQuotaGB())
	})

	t.Run("GetDefaultMaxFileSizeMB", func(t *testing.T) {
		var nilCfg *config.TenantsStorageConfig
		assert.Equal(t, 500, nilCfg.GetDefaultMaxFileSizeMB())

		cfg := &config.TenantsStorageConfig{Defaults: config.StorageMultiTenantDefaults{MaxFileSizeMB: 2048}}
		assert.Equal(t, 2048, cfg.GetDefaultMaxFileSizeMB())
	})

	t.Run("GetDefaultMaxConcurrentUploads", func(t *testing.T) {
		var nilCfg *config.TenantsStorageConfig
		assert.Equal(t, 10, nilCfg.GetDefaultMaxConcurrentUploads())

		cfg := &config.TenantsStorageConfig{Defaults: config.StorageMultiTenantDefaults{MaxConcurrentUploads: 50}}
		assert.Equal(t, 50, cfg.GetDefaultMaxConcurrentUploads())
	})
}

// TestTenantConfig_GetAllDomains 测试获取所有域名
func TestTenantConfig_GetAllDomains(t *testing.T) {
	t.Run("只有主域名", func(t *testing.T) {
		cfg := &config.TenantConfig{
			HTTP: config.TenantHTTPConfig{
				PrimaryDomain: "example.com",
			},
		}
		domains := cfg.GetAllDomains()
		assert.Equal(t, []string{"example.com"}, domains)
	})

	t.Run("主域名和允许域名", func(t *testing.T) {
		cfg := &config.TenantConfig{
			HTTP: config.TenantHTTPConfig{
				PrimaryDomain:  "example.com",
				AllowedDomains: []string{"www.example.com", "app.example.com"},
			},
		}
		domains := cfg.GetAllDomains()
		assert.Len(t, domains, 3)
		assert.Contains(t, domains, "example.com")
		assert.Contains(t, domains, "www.example.com")
		assert.Contains(t, domains, "app.example.com")
	})
}

