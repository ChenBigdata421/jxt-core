package config

import (
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	. "github.com/smartystreets/goconvey/convey"
)

// 创建测试用的多租户配置
func createTestTenantsConfig() *config.Tenants {
	return &config.Tenants{
		Resolver: config.ResolverConfig{
			HTTP: config.HTTPResolverConfig{
				Type:       "host",
				HeaderName: "X-Tenant-ID",
				QueryParam: "tenant",
				PathIndex:  0,
				HostMode:   "numeric",
			},
			FTP: config.FTPResolverConfig{
				Type: "username",
			},
		},
		Storage: &config.TenantsStorageConfig{
			Directory: "tenants",
		},
		Default: &config.DefaultTenantConfig{
			Domain: &config.TenantDomainConfig{
				Primary:  "app.example.com",
				Aliases:  []string{"www.example.com", "api.example.com"},
				Internal: "app.internal",
			},
			FTPConfigs: []config.TenantFTPDetailConfig{
				{
					Username:        "ftp_sales",
					InitialPassword: "Sales@123456",
					Description:     "销售部 FTP",
					Status:          "active",
				},
				{
					Username:        "ftp_hr",
					InitialPassword: "HR@123456",
					Description:     "人事部 FTP",
					Status:          "active",
				},
				{
					Username:        "ftp_finance",
					InitialPassword: "Finance@123456",
					Description:     "财务部 FTP（临时停用）",
					Status:          "inactive",
				},
			},
			Storage: &config.TenantStorageDetailConfig{
				UploadQuotaGB:        1000,
				MaxFileSizeMB:        2048,
				MaxConcurrentUploads: 20,
			},
			ServiceDatabases: map[string]config.TenantDatabaseDetailConfig{
				"evidence-command": {
					Driver:   "mysql",
					Host:     "mysql-command",
					Port:     3306,
					Database: "tenant_command",
					Username: "tenant",
					Password: "password123",
				},
				"evidence-query": {
					Driver:   "postgres",
					Host:     "postgres-query",
					Port:     5432,
					Database: "tenant_query",
					Username: "tenant",
					Password: "password123",
				},
			},
		},
	}
}

func TestTenants_GetResolver(t *testing.T) {
	Convey("测试 Tenants.GetResolver()", t, func() {
		Convey("正常配置应返回 Resolver", func() {
			cfg := createTestTenantsConfig()
			resolver := cfg.GetResolver()
			So(resolver, ShouldNotBeNil)
			So(resolver.HTTP.Type, ShouldEqual, "host")
			So(resolver.FTP.Type, ShouldEqual, "username")
		})

		Convey("nil 配置应返回 nil", func() {
			var nilCfg *config.Tenants
			resolver := nilCfg.GetResolver()
			So(resolver, ShouldBeNil)
		})
	})
}

func TestTenants_GetDefault(t *testing.T) {
	Convey("测试 Tenants.GetDefault()", t, func() {
		Convey("正常配置应返回 DefaultTenantConfig", func() {
			cfg := createTestTenantsConfig()
			defaultCfg := cfg.GetDefault()
			So(defaultCfg, ShouldNotBeNil)
			So(defaultCfg.Domain, ShouldNotBeNil)
			So(defaultCfg.Domain.Primary, ShouldEqual, "app.example.com")
		})

		Convey("nil 配置应返回 nil", func() {
			var nilCfg *config.Tenants
			defaultCfg := nilCfg.GetDefault()
			So(defaultCfg, ShouldBeNil)
		})
	})
}

func TestResolverConfig_GetHTTP(t *testing.T) {
	Convey("测试 ResolverConfig.GetHTTP()", t, func() {
		Convey("正常配置应返回 HTTPResolverConfig", func() {
			cfg := createTestTenantsConfig()
			http := cfg.GetResolver().GetHTTP()
			So(http, ShouldNotBeNil)
			So(http.Type, ShouldEqual, "host")
			So(http.HeaderName, ShouldEqual, "X-Tenant-ID")
		})

		Convey("nil 配置应返回 nil", func() {
			var nilResolver *config.ResolverConfig
			http := nilResolver.GetHTTP()
			So(http, ShouldBeNil)
		})
	})
}

func TestResolverConfig_GetFTP(t *testing.T) {
	Convey("测试 ResolverConfig.GetFTP()", t, func() {
		Convey("正常配置应返回 FTPResolverConfig", func() {
			cfg := createTestTenantsConfig()
			ftp := cfg.GetResolver().GetFTP()
			So(ftp, ShouldNotBeNil)
			So(ftp.Type, ShouldEqual, "username")
		})

		Convey("nil 配置应返回 nil", func() {
			var nilResolver *config.ResolverConfig
			ftp := nilResolver.GetFTP()
			So(ftp, ShouldBeNil)
		})
	})
}

func TestHTTPResolverConfig_Getters(t *testing.T) {
	Convey("测试 HTTPResolverConfig 的 getter 方法", t, func() {
		http := &config.HTTPResolverConfig{
			Type:       "header",
			HeaderName: "X-Custom-Tenant",
			QueryParam: "tenant_id",
			PathIndex:  2,
			HostMode:   "domain",
		}

		Convey("GetType() 应返回正确值", func() {
			So(http.GetType(), ShouldEqual, "header")
		})

		Convey("GetHeaderName() 应返回正确值", func() {
			So(http.GetHeaderName(), ShouldEqual, "X-Custom-Tenant")
		})

		Convey("GetQueryParam() 应返回正确值", func() {
			So(http.GetQueryParam(), ShouldEqual, "tenant_id")
		})

		Convey("GetPathIndex() 应返回正确值", func() {
			So(http.GetPathIndex(), ShouldEqual, 2)
		})

		Convey("GetHostModeOrDefault() 应返回正确值", func() {
			So(http.GetHostModeOrDefault(), ShouldEqual, "domain")
		})

		Convey("GetHostModeOrDefault() 空值应返回默认值 numeric", func() {
			emptyHTTP := &config.HTTPResolverConfig{}
			So(emptyHTTP.GetHostModeOrDefault(), ShouldEqual, "numeric")
		})

		Convey("nil 配置应返回空值或默认值", func() {
			var nilHTTP *config.HTTPResolverConfig
			So(nilHTTP.GetType(), ShouldEqual, "")
			So(nilHTTP.GetHeaderName(), ShouldEqual, "")
			So(nilHTTP.GetQueryParam(), ShouldEqual, "")
			So(nilHTTP.GetPathIndex(), ShouldEqual, 0)
			So(nilHTTP.GetHostModeOrDefault(), ShouldEqual, "numeric")
		})
	})
}

func TestFTPResolverConfig_GetType(t *testing.T) {
	Convey("测试 FTPResolverConfig.GetType()", t, func() {
		Convey("正常配置应返回类型", func() {
			ftp := &config.FTPResolverConfig{Type: "username"}
			So(ftp.GetType(), ShouldEqual, "username")
		})

		Convey("nil 配置应返回空字符串", func() {
			var nilFTP *config.FTPResolverConfig
			So(nilFTP.GetType(), ShouldEqual, "")
		})
	})
}

func TestTenantsStorageConfig_GetStorageDirectory(t *testing.T) {
	Convey("测试 TenantsStorageConfig.GetStorageDirectory()", t, func() {
		Convey("正常配置应返回目录名", func() {
			storage := &config.TenantsStorageConfig{Directory: "custom_tenants"}
			So(storage.GetStorageDirectory(), ShouldEqual, "custom_tenants")
		})

		Convey("空值应返回默认值 tenants", func() {
			storage := &config.TenantsStorageConfig{}
			So(storage.GetStorageDirectory(), ShouldEqual, "tenants")
		})

		Convey("nil 配置应返回默认值 tenants", func() {
			var nilStorage *config.TenantsStorageConfig
			So(nilStorage.GetStorageDirectory(), ShouldEqual, "tenants")
		})
	})
}

func TestDefaultTenantConfig_DomainMethods(t *testing.T) {
	Convey("测试 DefaultTenantConfig 域名相关方法", t, func() {
		Convey("GetDefaultTenantDomain() 正常情况", func() {
			cfg := createTestTenantsConfig()
			domain := cfg.GetDefault().GetDefaultTenantDomain()
			So(domain, ShouldNotBeNil)
			So(domain.Primary, ShouldEqual, "app.example.com")
			So(len(domain.Aliases), ShouldEqual, 2)
		})

		Convey("GetDefaultTenantDomain() nil 配置", func() {
			var nilCfg *config.DefaultTenantConfig
			domain := nilCfg.GetDefaultTenantDomain()
			So(domain, ShouldBeNil)
		})
	})
}

func TestDefaultTenantConfig_StorageMethods(t *testing.T) {
	Convey("测试 DefaultTenantConfig 存储相关方法", t, func() {
		Convey("GetDefaultTenantStorage() 正常情况", func() {
			cfg := createTestTenantsConfig()
			storage := cfg.GetDefault().GetDefaultTenantStorage()
			So(storage, ShouldNotBeNil)
			So(storage.UploadQuotaGB, ShouldEqual, 1000)
			So(storage.MaxFileSizeMB, ShouldEqual, 2048)
			So(storage.MaxConcurrentUploads, ShouldEqual, 20)
		})

		Convey("GetDefaultTenantStorage() nil 配置", func() {
			var nilCfg *config.DefaultTenantConfig
			storage := nilCfg.GetDefaultTenantStorage()
			So(storage, ShouldBeNil)
		})
	})
}

func TestDefaultTenantConfig_FTPMethods(t *testing.T) {
	Convey("测试 DefaultTenantConfig FTP 相关方法", t, func() {
		cfg := createTestTenantsConfig()
		defaultCfg := cfg.GetDefault()

		Convey("GetFTPConfigs() 应返回所有配置", func() {
			ftpConfigs := defaultCfg.GetFTPConfigs()
			So(len(ftpConfigs), ShouldEqual, 3)
		})

		Convey("GetActiveFTPConfigs() 应只返回 active 状态的配置", func() {
			activeConfigs := defaultCfg.GetActiveFTPConfigs()
			So(len(activeConfigs), ShouldEqual, 2)
			for _, c := range activeConfigs {
				So(c.Status == "" || c.Status == "active", ShouldBeTrue)
			}
		})

		Convey("GetFTPConfigByUsername() 找到存在的用户名", func() {
			ftp := defaultCfg.GetFTPConfigByUsername("ftp_sales")
			So(ftp, ShouldNotBeNil)
			So(ftp.Username, ShouldEqual, "ftp_sales")
			So(ftp.Description, ShouldEqual, "销售部 FTP")
		})

		Convey("GetFTPConfigByUsername() 找不到不存在的用户名", func() {
			ftp := defaultCfg.GetFTPConfigByUsername("unknown_user")
			So(ftp, ShouldBeNil)
		})

		Convey("HasFTPConfigs() 有配置时应返回 true", func() {
			So(defaultCfg.HasFTPConfigs(), ShouldBeTrue)
		})

		Convey("HasFTPConfigs() 无配置时应返回 false", func() {
			emptyCfg := &config.DefaultTenantConfig{}
			So(emptyCfg.HasFTPConfigs(), ShouldBeFalse)
		})

		Convey("nil 配置的 FTP 方法", func() {
			var nilCfg *config.DefaultTenantConfig
			So(len(nilCfg.GetFTPConfigs()), ShouldEqual, 0)
			So(len(nilCfg.GetActiveFTPConfigs()), ShouldEqual, 0)
			So(nilCfg.GetFTPConfigByUsername("ftp_sales"), ShouldBeNil)
			So(nilCfg.HasFTPConfigs(), ShouldBeFalse)
		})
	})
}

func TestTenantDomainConfig_Getters(t *testing.T) {
	Convey("测试 TenantDomainConfig 的 getter 方法", t, func() {
		domain := &config.TenantDomainConfig{
			Primary:  "app.example.com",
			Aliases:  []string{"www.example.com", "api.example.com"},
			Internal: "app.internal",
		}

		Convey("GetPrimaryDomain() 应返回主域名", func() {
			So(domain.GetPrimaryDomain(), ShouldEqual, "app.example.com")
		})

		Convey("GetAliases() 应返回别名列表", func() {
			aliases := domain.GetAliases()
			So(len(aliases), ShouldEqual, 2)
		})

		Convey("GetAllDomainAliases() 应包含主域名和别名", func() {
			allDomains := domain.GetAllDomainAliases()
			So(len(allDomains), ShouldEqual, 3)
			So(allDomains[0], ShouldEqual, "app.example.com")
		})

		Convey("GetInternalDomain() 应返回内部域名", func() {
			So(domain.GetInternalDomain(), ShouldEqual, "app.internal")
		})

		Convey("nil 配置的 getter 方法", func() {
			var nilDomain *config.TenantDomainConfig
			So(nilDomain.GetPrimaryDomain(), ShouldEqual, "")
			So(nilDomain.GetAliases(), ShouldBeNil)
			So(nilDomain.GetInternalDomain(), ShouldEqual, "")
		})
	})
}

func TestTenantStorageDetailConfig_Getters(t *testing.T) {
	Convey("测试 TenantStorageDetailConfig 的 getter 方法", t, func() {
		storage := &config.TenantStorageDetailConfig{
			UploadQuotaGB:        500,
			MaxFileSizeMB:        1024,
			MaxConcurrentUploads: 10,
		}

		Convey("GetStorageUploadQuotaGB() 应返回配额", func() {
			So(storage.GetStorageUploadQuotaGB(), ShouldEqual, 500)
		})

		Convey("GetStorageMaxFileSizeMB() 应返回最大文件大小", func() {
			So(storage.GetStorageMaxFileSizeMB(), ShouldEqual, 1024)
		})

		Convey("GetStorageMaxConcurrentUploads() 应返回最大并发数", func() {
			So(storage.GetStorageMaxConcurrentUploads(), ShouldEqual, 10)
		})

		Convey("零值应返回默认值", func() {
			emptyStorage := &config.TenantStorageDetailConfig{}
			So(emptyStorage.GetStorageUploadQuotaGB(), ShouldEqual, 1000)
			So(emptyStorage.GetStorageMaxFileSizeMB(), ShouldEqual, 2048)
			So(emptyStorage.GetStorageMaxConcurrentUploads(), ShouldEqual, 20)
		})

		Convey("nil 配置应返回默认值", func() {
			var nilStorage *config.TenantStorageDetailConfig
			So(nilStorage.GetStorageUploadQuotaGB(), ShouldEqual, 1000)
			So(nilStorage.GetStorageMaxFileSizeMB(), ShouldEqual, 2048)
			So(nilStorage.GetStorageMaxConcurrentUploads(), ShouldEqual, 20)
		})
	})
}

func TestTenantFTPDetailConfig_Getters(t *testing.T) {
	Convey("测试 TenantFTPDetailConfig 的 getter 方法", t, func() {
		ftp := &config.TenantFTPDetailConfig{
			Username:        "ftp_test",
			InitialPassword: "Test@123456",
			Description:     "测试 FTP",
			Status:          "active",
		}

		Convey("GetFTPUsername() 应返回用户名", func() {
			So(ftp.GetFTPUsername(), ShouldEqual, "ftp_test")
		})

		Convey("GetFTPInitialPassword() 应返回初始密码", func() {
			So(ftp.GetFTPInitialPassword(), ShouldEqual, "Test@123456")
		})

		Convey("GetFTPDescription() 应返回描述", func() {
			So(ftp.GetFTPDescription(), ShouldEqual, "测试 FTP")
		})

		Convey("GetFTPStatus() 应返回状态", func() {
			So(ftp.GetFTPStatus(), ShouldEqual, "active")
		})

		Convey("IsFTPActive() active 状态应返回 true", func() {
			So(ftp.IsFTPActive(), ShouldBeTrue)
		})

		Convey("IsFTPActive() inactive 状态应返回 false", func() {
			ftp.Status = "inactive"
			So(ftp.IsFTPActive(), ShouldBeFalse)
		})

		Convey("GetFTPStatus() 空值应返回默认值 active", func() {
			emptyFTP := &config.TenantFTPDetailConfig{}
			So(emptyFTP.GetFTPStatus(), ShouldEqual, "active")
		})

		Convey("nil 配置的 getter 方法", func() {
			var nilFTP *config.TenantFTPDetailConfig
			So(nilFTP.GetFTPUsername(), ShouldEqual, "")
			So(nilFTP.GetFTPInitialPassword(), ShouldEqual, "")
			So(nilFTP.GetFTPDescription(), ShouldEqual, "")
			So(nilFTP.GetFTPStatus(), ShouldEqual, "active")
			So(nilFTP.IsFTPActive(), ShouldBeTrue)
		})
	})
}

func TestTenantDatabaseDetailConfig_Getters(t *testing.T) {
	Convey("测试 TenantDatabaseDetailConfig 的 getter 方法", t, func() {
		db := &config.TenantDatabaseDetailConfig{
			Driver:          "postgres",
			Host:            "localhost",
			Port:            5432,
			Database:        "testdb",
			Username:        "testuser",
			Password:        "testpass",
			SSLMode:         "disable",
			MaxOpenConns:    100,
			MaxIdleConns:    20,
			ConnMaxIdleTime: 300,
			ConnMaxLifeTime: 3600,
			ConnectTimeout:  10,
			ReadTimeout:     30,
			WriteTimeout:    30,
		}

		Convey("基本 getter 方法", func() {
			So(db.GetDatabaseDriver(), ShouldEqual, "postgres")
			So(db.GetDatabaseHost(), ShouldEqual, "localhost")
			So(db.GetDatabasePort(), ShouldEqual, 5432)
			So(db.GetDatabaseName(), ShouldEqual, "testdb")
			So(db.GetDatabaseUsername(), ShouldEqual, "testuser")
			So(db.GetDatabasePassword(), ShouldEqual, "testpass")
			So(db.GetDatabaseSSLMode(), ShouldEqual, "disable")
		})

		Convey("连接池配置 getter 方法", func() {
			So(db.GetDatabaseMaxOpenConns(), ShouldEqual, 100)
			So(db.GetDatabaseMaxIdleConns(), ShouldEqual, 20)
			So(db.GetDatabaseConnMaxIdleTime(), ShouldEqual, 300)
			So(db.GetDatabaseConnMaxLifeTime(), ShouldEqual, 3600)
		})

		Convey("超时配置 getter 方法", func() {
			So(db.GetDatabaseConnectTimeout(), ShouldEqual, 10)
			So(db.GetDatabaseReadTimeout(), ShouldEqual, 30)
			So(db.GetDatabaseWriteTimeout(), ShouldEqual, 30)
		})

		Convey("GetDatabaseConnectionString() PostgreSQL", func() {
			connStr := db.GetDatabaseConnectionString()
			So(connStr, ShouldContainSubstring, "postgres://")
			So(connStr, ShouldContainSubstring, "sslmode=disable")
		})

		Convey("GetDatabaseConnectionString() MySQL", func() {
			mysqlDB := &config.TenantDatabaseDetailConfig{
				Driver:   "mysql",
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "testuser",
				Password: "testpass",
			}
			connStr := mysqlDB.GetDatabaseConnectionString()
			So(connStr, ShouldContainSubstring, "testuser:testpass@tcp")
		})

		Convey("nil 配置应返回空值或零值", func() {
			var nilDB *config.TenantDatabaseDetailConfig
			So(nilDB.GetDatabaseDriver(), ShouldEqual, "")
			So(nilDB.GetDatabaseHost(), ShouldEqual, "")
			So(nilDB.GetDatabasePort(), ShouldEqual, 0)
			So(nilDB.GetDatabaseName(), ShouldEqual, "")
			So(nilDB.GetDatabaseConnectionString(), ShouldEqual, "")
		})
	})
}
