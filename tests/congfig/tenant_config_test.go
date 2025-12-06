package congfig

import (
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
)

// 创建测试用的多租户配置
func createTestTenantsConfig() *config.Tenants {
	return &config.Tenants{
		Enabled: true,
		Resolver: config.ResolverConfig{
			HTTP: config.HTTPResolverConfig{
				Type:       "host",
				HeaderName: "X-Tenant-ID",
			},
			FTP: config.FTPResolverConfig{
				Type: "username",
			},
		},
		Storage: &config.TenantsStorageConfig{
			Directory:            "tenants",
			CacheRefreshInterval: 600,
			CreateOnLogin:        true,
			Defaults: config.StorageMultiTenantDefaults{
				UploadQuotaGB:        200,
				MaxFileSizeMB:        1024,
				MaxConcurrentUploads: 20,
			},
		},
		Database: &config.TenantsDatabaseConfig{
			Defaults: config.DatabaseDefaults{
				Driver:       "mysql",
				MaxOpenConns: 50,
				MaxIdleConns: 10,
			},
		},
		List: []config.TenantConfig{
			{
				ID:     "tenant-001",
				Name:   "测试租户1",
				Active: true,
				HTTP: config.TenantHTTPConfig{
					PrimaryDomain:  "tenant1.example.com",
					AllowedDomains: []string{"t1.example.com", "www.tenant1.com"},
				},
				FTP: config.TenantFTPConfig{
					Username:     "ftp_tenant1",
					PasswordHash: "hash123",
				},
			},
			{
				ID:     "tenant-002",
				Name:   "测试租户2",
				Active: true,
				HTTP: config.TenantHTTPConfig{
					PrimaryDomain:  "tenant2.example.com",
					AllowedDomains: []string{},
				},
				FTP: config.TenantFTPConfig{
					Username:     "ftp_tenant2",
					PasswordHash: "hash456",
				},
			},
			{
				ID:     "tenant-003",
				Name:   "禁用租户",
				Active: false,
				HTTP: config.TenantHTTPConfig{
					PrimaryDomain: "disabled.example.com",
				},
				FTP: config.TenantFTPConfig{
					Username: "ftp_disabled",
				},
			},
		},
	}
}

// TestTenants_GetTenantByID 测试根据 ID 获取租户
func TestTenants_GetTenantByID(t *testing.T) {
	cfg := createTestTenantsConfig()

	t.Run("找到存在的租户", func(t *testing.T) {
		tenant := cfg.GetTenantByID("tenant-001")
		assert.NotNil(t, tenant)
		assert.Equal(t, "tenant-001", tenant.ID)
		assert.Equal(t, "测试租户1", tenant.Name)
	})

	t.Run("找不到不存在的租户", func(t *testing.T) {
		tenant := cfg.GetTenantByID("non-existent")
		assert.Nil(t, tenant)
	})

	t.Run("nil 配置返回 nil", func(t *testing.T) {
		var nilCfg *config.Tenants
		tenant := nilCfg.GetTenantByID("tenant-001")
		assert.Nil(t, tenant)
	})
}

// TestTenants_GetTenantByDomain 测试根据域名获取租户
func TestTenants_GetTenantByDomain(t *testing.T) {
	cfg := createTestTenantsConfig()

	t.Run("通过主域名找到租户", func(t *testing.T) {
		tenant := cfg.GetTenantByDomain("tenant1.example.com")
		assert.NotNil(t, tenant)
		assert.Equal(t, "tenant-001", tenant.ID)
	})

	t.Run("通过允许域名找到租户", func(t *testing.T) {
		tenant := cfg.GetTenantByDomain("t1.example.com")
		assert.NotNil(t, tenant)
		assert.Equal(t, "tenant-001", tenant.ID)
	})

	t.Run("找不到不存在的域名", func(t *testing.T) {
		tenant := cfg.GetTenantByDomain("unknown.example.com")
		assert.Nil(t, tenant)
	})

	t.Run("nil 配置返回 nil", func(t *testing.T) {
		var nilCfg *config.Tenants
		tenant := nilCfg.GetTenantByDomain("tenant1.example.com")
		assert.Nil(t, tenant)
	})
}

// TestTenants_GetTenantByFtpUsername 测试根据 FTP 用户名获取租户
func TestTenants_GetTenantByFtpUsername(t *testing.T) {
	cfg := createTestTenantsConfig()

	t.Run("找到存在的用户名", func(t *testing.T) {
		tenant := cfg.GetTenantByFtpUsername("ftp_tenant1")
		assert.NotNil(t, tenant)
		assert.Equal(t, "tenant-001", tenant.ID)
	})

	t.Run("找不到不存在的用户名", func(t *testing.T) {
		tenant := cfg.GetTenantByFtpUsername("unknown_user")
		assert.Nil(t, tenant)
	})

	t.Run("nil 配置返回 nil", func(t *testing.T) {
		var nilCfg *config.Tenants
		tenant := nilCfg.GetTenantByFtpUsername("ftp_tenant1")
		assert.Nil(t, tenant)
	})
}

