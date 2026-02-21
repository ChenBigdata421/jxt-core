package config

import (
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	. "github.com/smartystreets/goconvey/convey"
)

// 注意：以下测试已在 tenant_config_test.go 中覆盖：
// - TestTenantsStorageConfig_GetStorageDirectory
// - TestTenantStorageDetailConfig_Getters
// - TestDefaultTenantConfig_StorageMethods
//
// 本文件保留用于未来添加其他存储相关测试

func TestTenants_GetTenantsStorageConfig_Extra(t *testing.T) {
	Convey("测试 Tenants.GetTenantsStorageConfig() 额外场景", t, func() {
		Convey("完整的 Tenants 配置", func() {
			cfg := &config.Tenants{
				Resolver: config.ResolverConfig{
					HTTP: config.HTTPResolverConfig{
						Type:       "host",
						HeaderName: "X-Tenant-ID",
					},
				},
				Storage: &config.TenantsStorageConfig{
					Directory: "custom_tenants",
				},
				Default: &config.DefaultTenantConfig{
					Storage: &config.TenantStorageDetailConfig{
						UploadQuotaGB:        500,
						MaxFileSizeMB:        1024,
						MaxConcurrentUploads: 15,
					},
				},
			}

			storage := cfg.GetTenantsStorageConfig()
			So(storage, ShouldNotBeNil)
			So(storage.Directory, ShouldEqual, "custom_tenants")

			defaultStorage := cfg.GetDefault().GetDefaultTenantStorage()
			So(defaultStorage, ShouldNotBeNil)
			So(defaultStorage.GetStorageUploadQuotaGB(), ShouldEqual, 500)
		})
	})
}
