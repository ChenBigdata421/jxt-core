package config

var TenantsConfig = new(Tenants)

// Tenants 统一的多租户配置
type Tenants struct {
	Enabled  bool                  `mapstructure:"enabled" yaml:"enabled"`   // 是否启用多租户模式
	Resolver ResolverConfig        `mapstructure:"resolver" yaml:"resolver"` // 租户识别配置
	Storage  *TenantsStorageConfig `mapstructure:"storage" yaml:"storage"`   // 多租户存储配置
	Database *TenantsDatabaseConfig `mapstructure:"database" yaml:"database"` // 多租户数据库默认配置
	List     []TenantConfig        `mapstructure:"list" yaml:"list"`         // 租户列表
}

// ResolverConfig 租户识别配置（区分 HTTP 和 FTP）
type ResolverConfig struct {
	HTTP HTTPResolverConfig `mapstructure:"http" yaml:"http"` // HTTP 识别配置
	FTP  FTPResolverConfig  `mapstructure:"ftp" yaml:"ftp"`   // FTP 识别配置
}

// HTTPResolverConfig HTTP 租户识别配置
type HTTPResolverConfig struct {
	Type       string `mapstructure:"type" yaml:"type"`             // 识别方式: host, header, query, path
	HeaderName string `mapstructure:"headerName" yaml:"headerName"` // 当 type 为 header 时使用的 header 名称
	QueryParam string `mapstructure:"queryParam" yaml:"queryParam"` // 当 type 为 query 时使用的查询参数名
	PathIndex  int    `mapstructure:"pathIndex" yaml:"pathIndex"`   // 当 type 为 path 时使用的路径索引
}

// FTPResolverConfig FTP 租户识别配置
type FTPResolverConfig struct {
	Type string `mapstructure:"type" yaml:"type"` // 识别方式: username, password
}

// TenantsStorageConfig 多租户存储配置
type TenantsStorageConfig struct {
	Directory            string                     `mapstructure:"directory" yaml:"directory"`                           // 租户存储目录名（默认: tenants）
	CacheRefreshInterval int                        `mapstructure:"cache_refresh_interval" yaml:"cache_refresh_interval"` // 缓存刷新间隔（秒）
	CreateOnLogin        bool                       `mapstructure:"create_on_login" yaml:"create_on_login"`               // 登录时自动创建目录
	Defaults             StorageMultiTenantDefaults `mapstructure:"defaults" yaml:"defaults"`                             // 存储默认配置
}

// StorageMultiTenantDefaults 多租户存储默认配置
type StorageMultiTenantDefaults struct {
	UploadQuotaGB        int `mapstructure:"upload_quota_gb" yaml:"upload_quota_gb"`               // 上传配额（GB）
	MaxFileSizeMB        int `mapstructure:"max_file_size_mb" yaml:"max_file_size_mb"`             // 单文件最大大小（MB）
	MaxConcurrentUploads int `mapstructure:"max_concurrent_uploads" yaml:"max_concurrent_uploads"` // 最大并发上传数
}

// TenantsDatabaseConfig 多租户数据库默认配置
type TenantsDatabaseConfig struct {
	Defaults DatabaseDefaults `mapstructure:"defaults" yaml:"defaults"` // 数据库连接默认配置
}

// DatabaseDefaults 数据库连接默认值
type DatabaseDefaults struct {
	Driver          string `mapstructure:"driver" yaml:"driver"`
	MaxOpenConns    int    `mapstructure:"maxOpenConns" yaml:"maxOpenConns"`
	MaxIdleConns    int    `mapstructure:"maxIdleConns" yaml:"maxIdleConns"`
	ConnMaxIdleTime int    `mapstructure:"connMaxIdleTime" yaml:"connMaxIdleTime"` // 秒
	ConnMaxLifeTime int    `mapstructure:"connMaxLifeTime" yaml:"connMaxLifeTime"` // 秒
}

// TenantConfig 单个租户配置
type TenantConfig struct {
	ID       string               `mapstructure:"id" yaml:"id"`             // 租户 ID，唯一标识
	Name     string               `mapstructure:"name" yaml:"name"`         // 租户名称
	Active   bool                 `mapstructure:"active" yaml:"active"`     // 是否激活
	HTTP     TenantHTTPConfig     `mapstructure:"http" yaml:"http"`         // HTTP 识别配置
	FTP      TenantFTPConfig      `mapstructure:"ftp" yaml:"ftp"`           // FTP 识别配置
	Database *TenantDatabaseConfig `mapstructure:"database" yaml:"database"` // 数据库配置（可选）
	Storage  *TenantStorageConfig `mapstructure:"storage" yaml:"storage"`   // 存储配置（可选）
}

// TenantHTTPConfig 租户 HTTP 识别配置
type TenantHTTPConfig struct {
	PrimaryDomain  string   `mapstructure:"primary_domain" yaml:"primary_domain"`   // 主域名
	AllowedDomains []string `mapstructure:"allowed_domains" yaml:"allowed_domains"` // 允许的域名列表
}

// TenantFTPConfig 租户 FTP 识别配置
type TenantFTPConfig struct {
	Username     string `mapstructure:"username" yaml:"username"`          // FTP 用户名
	PasswordHash string `mapstructure:"password_hash" yaml:"password_hash"` // FTP 密码哈希
}

// TenantDatabaseConfig 租户特定的数据库配置
type TenantDatabaseConfig struct {
	Driver          string `mapstructure:"driver" yaml:"driver"`
	Source          string `mapstructure:"source" yaml:"source"`
	MaxOpenConns    int    `mapstructure:"maxOpenConns" yaml:"maxOpenConns"`
	MaxIdleConns    int    `mapstructure:"maxIdleConns" yaml:"maxIdleConns"`
	ConnMaxIdleTime int    `mapstructure:"connMaxIdleTime" yaml:"connMaxIdleTime"`
	ConnMaxLifeTime int    `mapstructure:"connMaxLifeTime" yaml:"connMaxLifeTime"`
}

// TenantStorageConfig 租户特定的存储配置
type TenantStorageConfig struct {
	UploadQuotaGB        int `mapstructure:"upload_quota_gb" yaml:"upload_quota_gb"`
	MaxFileSizeMB        int `mapstructure:"max_file_size_mb" yaml:"max_file_size_mb"`
	MaxConcurrentUploads int `mapstructure:"max_concurrent_uploads" yaml:"max_concurrent_uploads"`
}

// ============================================================
// 辅助方法
// ============================================================

// GetTenantsStorageConfig 获取多租户存储配置
func (tc *Tenants) GetTenantsStorageConfig() *TenantsStorageConfig {
	if tc == nil || !tc.Enabled || tc.Storage == nil {
		return nil
	}
	return tc.Storage
}

// GetTenantByID 根据 ID 获取租户配置
func (tc *Tenants) GetTenantByID(id string) *TenantConfig {
	if tc == nil {
		return nil
	}
	for i := range tc.List {
		if tc.List[i].ID == id {
			return &tc.List[i]
		}
	}
	return nil
}

// GetTenantByDomain 根据域名获取租户配置
func (tc *Tenants) GetTenantByDomain(domain string) *TenantConfig {
	if tc == nil {
		return nil
	}
	for i := range tc.List {
		t := &tc.List[i]
		// 检查主域名
		if t.HTTP.PrimaryDomain == domain {
			return t
		}
		// 检查允许的域名列表
		for _, d := range t.HTTP.AllowedDomains {
			if d == domain {
				return t
			}
		}
	}
	return nil
}

// GetTenantByFtpUsername 根据 FTP 用户名获取租户配置
func (tc *Tenants) GetTenantByFtpUsername(username string) *TenantConfig {
	if tc == nil {
		return nil
	}
	for i := range tc.List {
		if tc.List[i].FTP.Username == username {
			return &tc.List[i]
		}
	}
	return nil
}

// GetActiveTenants 获取所有激活的租户
func (tc *Tenants) GetActiveTenants() []TenantConfig {
	if tc == nil {
		return nil
	}
	var active []TenantConfig
	for _, t := range tc.List {
		if t.Active {
			active = append(active, t)
		}
	}
	return active
}

// GetStorageDirectory 获取租户存储目录名，默认 "tenants"
func (tc *TenantsStorageConfig) GetStorageDirectory() string {
	if tc == nil || tc.Directory == "" {
		return "tenants"
	}
	return tc.Directory
}

// GetCacheRefreshInterval 获取缓存刷新间隔，默认 300 秒
func (tc *TenantsStorageConfig) GetCacheRefreshInterval() int {
	if tc == nil || tc.CacheRefreshInterval <= 0 {
		return 300
	}
	return tc.CacheRefreshInterval
}

// GetDefaultUploadQuotaGB 获取默认上传配额，默认 100 GB
func (tc *TenantsStorageConfig) GetDefaultUploadQuotaGB() int {
	if tc == nil || tc.Defaults.UploadQuotaGB <= 0 {
		return 100
	}
	return tc.Defaults.UploadQuotaGB
}

// GetDefaultMaxFileSizeMB 获取默认单文件最大大小，默认 500 MB
func (tc *TenantsStorageConfig) GetDefaultMaxFileSizeMB() int {
	if tc == nil || tc.Defaults.MaxFileSizeMB <= 0 {
		return 500
	}
	return tc.Defaults.MaxFileSizeMB
}

// GetDefaultMaxConcurrentUploads 获取默认最大并发上传数，默认 10
func (tc *TenantsStorageConfig) GetDefaultMaxConcurrentUploads() int {
	if tc == nil || tc.Defaults.MaxConcurrentUploads <= 0 {
		return 10
	}
	return tc.Defaults.MaxConcurrentUploads
}

// GetAllDomains 获取租户的所有域名（主域名 + 允许的域名）
func (t *TenantConfig) GetAllDomains() []string {
	domains := make([]string, 0)
	if t.HTTP.PrimaryDomain != "" {
		domains = append(domains, t.HTTP.PrimaryDomain)
	}
	domains = append(domains, t.HTTP.AllowedDomains...)
	return domains
}

// 配置举例
/*
# 多租户系统配置
tenants:
  # 是否启用多租户功能
  enabled: true

  # 租户识别配置（区分 HTTP 和 FTP）
  resolver:
    http:
      type: "host"  # 可选: host, header, query, path
      headerName: "X-Tenant-ID"  # 当 type 为 header 时使用
      queryParam: "tenant"  # 当 type 为 query 时使用
      pathIndex: 0  # 当 type 为 path 时使用
    ftp:
      type: "username"  # 可选: username, password

  # 多租户存储配置
  storage:
    directory: "tenants"  # 租户存储目录名（实际路径：./uploads/tenants/<tenant_id>）
    cache_refresh_interval: 300  # 缓存刷新间隔（秒）
    create_on_login: true  # 登录时自动创建租户目录
    defaults:
      upload_quota_gb: 100  # 默认上传配额（GB）
      max_file_size_mb: 500  # 默认单文件最大大小（MB）
      max_concurrent_uploads: 10  # 默认最大并发上传数

  # 多租户数据库默认配置
  database:
    defaults:
      driver: "mysql"
      maxOpenConns: 10
      maxIdleConns: 5
      connMaxIdleTime: 60  # 秒
      connMaxLifeTime: 3600  # 秒

  # 租户列表
  list:
    # 租户 1：主租户
    - id: "tenant_primary"
      name: "Primary Organization"
      active: true
      http:
        primary_domain: "app.example.com"
        allowed_domains:
          - "www.example.com"
          - "api.example.com"
      ftp:
        username: "ftp_primary"
        password_hash: "$2b$12$..."
      database:
        driver: "mysql"
        source: "user:password@tcp(db.example.com:3306)/primary_db"
        maxOpenConns: 15
      storage:
        upload_quota_gb: 200
        max_file_size_mb: 1000
        max_concurrent_uploads: 20

    # 租户 2：次租户
    - id: "tenant_secondary"
      name: "Secondary Business Unit"
      active: true
      http:
        primary_domain: "secondary.example.com"
        allowed_domains:
          - "secondary-api.example.com"
      ftp:
        username: "ftp_secondary"
        password_hash: "$2b$12$..."
      database:
        driver: "mysql"
        source: "user:password@tcp(db.example.com:3306)/secondary_db"
      storage:
        upload_quota_gb: 150

    # 租户 3：特殊租户（使用 PostgreSQL）
    - id: "tenant_special"
      name: "Special Division"
      active: true
      http:
        primary_domain: "special.example.com"
      ftp:
        username: "ftp_special"
        password_hash: "$2b$12$..."
      database:
        driver: "postgres"
        source: "postgres://user:password@db.example.com:5432/special_db"

    # 租户 4：禁用的租户
    - id: "tenant_inactive"
      name: "Inactive Customer"
      active: false
      http:
        primary_domain: "inactive.example.com"
      ftp:
        username: "ftp_inactive"
        password_hash: "$2b$12$..."
      database:
        source: "user:password@tcp(db.example.com:3306)/inactive_db"
*/
