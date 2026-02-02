package config

import (
	"fmt"
)

var TenantsConfig = new(Tenants)

// Tenants 统一的多租户配置
type Tenants struct {
	Resolver ResolverConfig        `mapstructure:"resolver" yaml:"resolver"` // 租户识别配置
	Storage  *TenantsStorageConfig `mapstructure:"storage" yaml:"storage"`   // 多租户存储配置
	Default  *DefaultTenantConfig  `mapstructure:"default" yaml:"default"`   // 默认租户配置
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
	Directory string `mapstructure:"directory" yaml:"directory"` // 租户存储目录名（默认: tenants）
}

// DefaultTenantConfig 默认租户配置
type DefaultTenantConfig struct {
	// Legacy: 单个数据库配置（向后兼容）
	Database *TenantDatabaseDetailConfig `mapstructure:"database" yaml:"database"` // 数据库配置

	// NEW: 服务级数据库配置映射（service_code -> config）
	ServiceDatabases map[string]TenantDatabaseDetailConfig `mapstructure:"service_databases" yaml:"service_databases"` // 服务级数据库配置

	Domain     *TenantDomainConfig        `mapstructure:"domain" yaml:"domain"`         // 域名配置
	FTPConfigs []TenantFTPDetailConfig   `mapstructure:"ftp_configs" yaml:"ftp_configs"` // FTP 配置数组
	Storage    *TenantStorageDetailConfig `mapstructure:"storage" yaml:"storage"`        // 存储配置
}

// TenantDatabaseDetailConfig 详细的数据库配置
type TenantDatabaseDetailConfig struct {
	Driver          string `mapstructure:"driver" yaml:"driver"`                         // 数据库驱动: postgres | mysql
	Host            string `mapstructure:"host" yaml:"host"`                             // 数据库主机
	Port            int    `mapstructure:"port" yaml:"port"`                             // 数据库端口
	Database        string `mapstructure:"database" yaml:"database"`                     // 数据库名称
	Username        string `mapstructure:"username" yaml:"username"`                     // 数据库用户
	Password        string `mapstructure:"password" yaml:"password"`                     // 数据库密码
	SSLMode         string `mapstructure:"sslmode" yaml:"sslmode"`                       // SSL 模式: disable | require | verify-ca | verify-full
	MaxOpenConns    int    `mapstructure:"max_open_conns" yaml:"max_open_conns"`         // 最大打开连接数
	MaxIdleConns    int    `mapstructure:"max_idle_conns" yaml:"max_idle_conns"`         // 最大空闲连接数
	ConnMaxIdleTime int    `mapstructure:"conn_max_idle_time" yaml:"conn_max_idle_time"` // 连接最大空闲时间（秒）
	ConnMaxLifeTime int    `mapstructure:"conn_max_life_time" yaml:"conn_max_life_time"` // 连接最大生命周期（秒）
	ConnectTimeout  int    `mapstructure:"connect_timeout" yaml:"connect_timeout"`       // 连接超时（秒）
	ReadTimeout     int    `mapstructure:"read_timeout" yaml:"read_timeout"`             // 读超时（秒）
	WriteTimeout    int    `mapstructure:"write_timeout" yaml:"write_timeout"`           // 写超时（秒）
}

// TenantDomainConfig 域名配置
type TenantDomainConfig struct {
	Primary  string   `mapstructure:"primary" yaml:"primary"`   // 主域名（必填）
	Aliases  []string `mapstructure:"aliases" yaml:"aliases"`   // 备用域名（可选）
	Internal string   `mapstructure:"internal" yaml:"internal"` // 内部调用域名（可选）
}

// TenantFTPDetailConfig 详细的 FTP 配置
type TenantFTPDetailConfig struct {
	Username        string `mapstructure:"username" yaml:"username"`                 // FTP 用户名
	InitialPassword string `mapstructure:"initial_password" yaml:"initial_password"` // 初始密码（首次创建时使用）
	Description     string `mapstructure:"description" yaml:"description"`           // FTP 配置描述（用于标识不同 FTP）
	Status          string `mapstructure:"status" yaml:"status"`                     // 状态: active, inactive
}

// TenantStorageDetailConfig 详细的存储配置
type TenantStorageDetailConfig struct {
	UploadQuotaGB        int `mapstructure:"upload_quota_gb" yaml:"upload_quota_gb"`               // 上传配额（GB）
	MaxFileSizeMB        int `mapstructure:"max_file_size_mb" yaml:"max_file_size_mb"`             // 单文件最大大小（MB）
	MaxConcurrentUploads int `mapstructure:"max_concurrent_uploads" yaml:"max_concurrent_uploads"` // 最大并发上传数
}

// ============================================================
// 辅助方法
// ============================================================

// GetResolver 获取租户识别配置
func (tc *Tenants) GetResolver() *ResolverConfig {
	if tc == nil {
		return nil
	}
	return &tc.Resolver
}

// GetDefault 获取默认租户配置
func (tc *Tenants) GetDefault() *DefaultTenantConfig {
	return tc.GetDefaultTenantConfig()
}

// GetHTTP 获取 HTTP 租户识别配置
func (rc *ResolverConfig) GetHTTP() *HTTPResolverConfig {
	if rc == nil {
		return nil
	}
	return &rc.HTTP
}

// GetFTP 获取 FTP 租户识别配置
func (rc *ResolverConfig) GetFTP() *FTPResolverConfig {
	if rc == nil {
		return nil
	}
	return &rc.FTP
}

// GetType 获取 HTTP 租户识别方式
func (hrc *HTTPResolverConfig) GetType() string {
	if hrc == nil {
		return ""
	}
	return hrc.Type
}

// GetHeaderName 获取 Header 名称（当 type 为 header 时使用）
func (hrc *HTTPResolverConfig) GetHeaderName() string {
	if hrc == nil {
		return ""
	}
	return hrc.HeaderName
}

// GetQueryParam 获取查询参数名（当 type 为 query 时使用）
func (hrc *HTTPResolverConfig) GetQueryParam() string {
	if hrc == nil {
		return ""
	}
	return hrc.QueryParam
}

// GetPathIndex 获取路径索引（当 type 为 path 时使用）
func (hrc *HTTPResolverConfig) GetPathIndex() int {
	if hrc == nil {
		return 0
	}
	return hrc.PathIndex
}

// GetType 获取 FTP 租户识别方式
func (frc *FTPResolverConfig) GetType() string {
	if frc == nil {
		return ""
	}
	return frc.Type
}

// GetTenantsStorageConfig 获取多租户存储配置
func (tc *Tenants) GetTenantsStorageConfig() *TenantsStorageConfig {
	if tc == nil || tc.Storage == nil {
		return nil
	}
	return tc.Storage
}

// GetStorageDirectory 获取租户存储目录名，默认 "tenants"
func (tc *TenantsStorageConfig) GetStorageDirectory() string {
	if tc == nil || tc.Directory == "" {
		return "tenants"
	}
	return tc.Directory
}

// GetDefaultTenantConfig 获取默认租户配置
func (tc *Tenants) GetDefaultTenantConfig() *DefaultTenantConfig {
	if tc == nil || tc.Default == nil {
		return nil
	}
	return tc.Default
}

// GetDefaultTenantDatabase 获取默认租户数据库配置
func (dtc *DefaultTenantConfig) GetDefaultTenantDatabase() *TenantDatabaseDetailConfig {
	if dtc == nil || dtc.Database == nil {
		return nil
	}
	return dtc.Database
}

// GetDefaultTenantDomain 获取默认租户域名配置
func (dtc *DefaultTenantConfig) GetDefaultTenantDomain() *TenantDomainConfig {
	if dtc == nil || dtc.Domain == nil {
		return nil
	}
	return dtc.Domain
}

// GetDefaultTenantStorage 获取默认租户存储配置
func (dtc *DefaultTenantConfig) GetDefaultTenantStorage() *TenantStorageDetailConfig {
	if dtc == nil || dtc.Storage == nil {
		return nil
	}
	return dtc.Storage
}

// GetFTPConfigs 获取所有 FTP 配置数组
func (dtc *DefaultTenantConfig) GetFTPConfigs() []TenantFTPDetailConfig {
	if dtc == nil || dtc.FTPConfigs == nil {
		return []TenantFTPDetailConfig{}
	}
	return dtc.FTPConfigs
}

// GetActiveFTPConfigs 获取所有状态为 active 的 FTP 配置
func (dtc *DefaultTenantConfig) GetActiveFTPConfigs() []TenantFTPDetailConfig {
	if dtc == nil || dtc.FTPConfigs == nil {
		return []TenantFTPDetailConfig{}
	}

	result := make([]TenantFTPDetailConfig, 0)
	for _, cfg := range dtc.FTPConfigs {
		if cfg.Status == "" || cfg.Status == "active" {
			result = append(result, cfg)
		}
	}
	return result
}

// GetFTPConfigByUsername 根据 Username 查找特定的 FTP 配置
func (dtc *DefaultTenantConfig) GetFTPConfigByUsername(username string) *TenantFTPDetailConfig {
	if dtc == nil || dtc.FTPConfigs == nil {
		return nil
	}

	for i := range dtc.FTPConfigs {
		if dtc.FTPConfigs[i].Username == username {
			return &dtc.FTPConfigs[i]
		}
	}
	return nil
}

// HasFTPConfigs 判断是否配置了多个 FTP
func (dtc *DefaultTenantConfig) HasFTPConfigs() bool {
	return dtc != nil && len(dtc.FTPConfigs) > 0
}

// GetServiceDatabases 获取服务级数据库配置映射
func (dtc *DefaultTenantConfig) GetServiceDatabases() map[string]TenantDatabaseDetailConfig {
	if dtc == nil || dtc.ServiceDatabases == nil {
		return make(map[string]TenantDatabaseDetailConfig)
	}
	return dtc.ServiceDatabases
}

// GetServiceDatabase 获取指定服务的数据库配置
func (dtc *DefaultTenantConfig) GetServiceDatabase(serviceCode string) *TenantDatabaseDetailConfig {
	if dtc == nil || dtc.ServiceDatabases == nil {
		return nil
	}
	if config, ok := dtc.ServiceDatabases[serviceCode]; ok {
		return &config
	}
	return nil
}

// HasServiceDatabases 判断是否配置了服务级数据库
func (dtc *DefaultTenantConfig) HasServiceDatabases() bool {
	return dtc != nil && len(dtc.ServiceDatabases) > 0
}

func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseConnectionString() string {
	if dbConfig == nil {
		return ""
	}

	portStr := fmt.Sprintf("%d", dbConfig.Port)

	if dbConfig.Driver == "postgres" {
		return "postgres://" + dbConfig.Username + ":" + dbConfig.Password + "@" +
			dbConfig.Host + ":" + portStr + "/" + dbConfig.Database +
			"?sslmode=" + dbConfig.SSLMode
	}

	// MySQL 连接字符串
	return dbConfig.Username + ":" + dbConfig.Password + "@tcp(" + dbConfig.Host + ":" +
		portStr + ")/" + dbConfig.Database
}

// GetDatabaseDriver 获取数据库驱动
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseDriver() string {
	if dbConfig == nil {
		return ""
	}
	return dbConfig.Driver
}

// GetDatabaseMaxOpenConns 获取最大连接数
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseMaxOpenConns() int {
	if dbConfig == nil {
		return 0
	}
	return dbConfig.MaxOpenConns
}

// GetDatabaseMaxIdleConns 获取最大空闲连接数
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseMaxIdleConns() int {
	if dbConfig == nil {
		return 0
	}
	return dbConfig.MaxIdleConns
}

// GetDatabaseConnMaxIdleTime 获取连接最大空闲时间（秒）
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseConnMaxIdleTime() int {
	if dbConfig == nil {
		return 0
	}
	return dbConfig.ConnMaxIdleTime
}

// GetDatabaseConnMaxLifeTime 获取连接最大存活时间（秒）
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseConnMaxLifeTime() int {
	if dbConfig == nil {
		return 0
	}
	return dbConfig.ConnMaxLifeTime
}

// GetDatabaseConnectTimeout 获取连接超时（秒）
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseConnectTimeout() int {
	if dbConfig == nil {
		return 0
	}
	return dbConfig.ConnectTimeout
}

// GetDatabaseReadTimeout 获取读超时（秒）
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseReadTimeout() int {
	if dbConfig == nil {
		return 0
	}
	return dbConfig.ReadTimeout
}

// GetDatabaseWriteTimeout 获取写超时（秒）
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseWriteTimeout() int {
	if dbConfig == nil {
		return 0
	}
	return dbConfig.WriteTimeout
}

// GetDatabaseHost 获取数据库主机
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseHost() string {
	if dbConfig == nil {
		return ""
	}
	return dbConfig.Host
}

// GetDatabasePort 获取数据库端口
func (dbConfig *TenantDatabaseDetailConfig) GetDatabasePort() int {
	if dbConfig == nil {
		return 0
	}
	return dbConfig.Port
}

// GetDatabaseName 获取数据库名称
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseName() string {
	if dbConfig == nil {
		return ""
	}
	return dbConfig.Database
}

// GetDatabaseUsername 获取数据库用户名
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseUsername() string {
	if dbConfig == nil {
		return ""
	}
	return dbConfig.Username
}

// GetDatabasePassword 获取数据库密码
func (dbConfig *TenantDatabaseDetailConfig) GetDatabasePassword() string {
	if dbConfig == nil {
		return ""
	}
	return dbConfig.Password
}

// GetDatabaseSSLMode 获取 SSL 模式
func (dbConfig *TenantDatabaseDetailConfig) GetDatabaseSSLMode() string {
	if dbConfig == nil {
		return ""
	}
	return dbConfig.SSLMode
}

// GetPrimaryDomain 获取主域名
func (domainConfig *TenantDomainConfig) GetPrimaryDomain() string {
	if domainConfig == nil || domainConfig.Primary == "" {
		return ""
	}
	return domainConfig.Primary
}

// GetAliases 获取域名别名列表
func (domainConfig *TenantDomainConfig) GetAliases() []string {
	if domainConfig == nil {
		return nil
	}
	return domainConfig.Aliases
}

// GetAllDomainAliases 获取所有域名别名（包括主域名）
func (domainConfig *TenantDomainConfig) GetAllDomainAliases() []string {
	if domainConfig == nil {
		return nil
	}
	domains := make([]string, 0)
	if domainConfig.Primary != "" {
		domains = append(domains, domainConfig.Primary)
	}
	domains = append(domains, domainConfig.Aliases...)
	return domains
}

// GetInternalDomain 获取内部调用域名
func (domainConfig *TenantDomainConfig) GetInternalDomain() string {
	if domainConfig == nil || domainConfig.Internal == "" {
		return ""
	}
	return domainConfig.Internal
}

// GetFTPUsername 获取 FTP 用户名
func (ftpConfig *TenantFTPDetailConfig) GetFTPUsername() string {
	if ftpConfig == nil || ftpConfig.Username == "" {
		return ""
	}
	return ftpConfig.Username
}

// GetFTPInitialPassword 获取 FTP 初始密码
func (ftpConfig *TenantFTPDetailConfig) GetFTPInitialPassword() string {
	if ftpConfig == nil || ftpConfig.InitialPassword == "" {
		return ""
	}
	return ftpConfig.InitialPassword
}

// GetFTPDescription 获取 FTP 配置描述
func (ftpConfig *TenantFTPDetailConfig) GetFTPDescription() string {
	if ftpConfig == nil {
		return ""
	}
	return ftpConfig.Description
}

// GetFTPStatus 获取 FTP 状态
func (ftpConfig *TenantFTPDetailConfig) GetFTPStatus() string {
	if ftpConfig == nil || ftpConfig.Status == "" {
		return "active" // 默认为 active
	}
	return ftpConfig.Status
}

// IsFTPActive 判断 FTP 配置是否处于 active 状态
func (ftpConfig *TenantFTPDetailConfig) IsFTPActive() bool {
	return ftpConfig.GetFTPStatus() == "active"
}

// GetStorageUploadQuotaGB 获取上传配额（GB）
func (storageConfig *TenantStorageDetailConfig) GetStorageUploadQuotaGB() int {
	if storageConfig == nil || storageConfig.UploadQuotaGB <= 0 {
		return 1000
	}
	return storageConfig.UploadQuotaGB
}

// GetStorageMaxFileSizeMB 获取单文件最大大小（MB）
func (storageConfig *TenantStorageDetailConfig) GetStorageMaxFileSizeMB() int {
	if storageConfig == nil || storageConfig.MaxFileSizeMB <= 0 {
		return 2048
	}
	return storageConfig.MaxFileSizeMB
}

// GetStorageMaxConcurrentUploads 获取最大并发上传数
func (storageConfig *TenantStorageDetailConfig) GetStorageMaxConcurrentUploads() int {
	if storageConfig == nil || storageConfig.MaxConcurrentUploads <= 0 {
		return 20
	}
	return storageConfig.MaxConcurrentUploads
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

  # 默认租户配置
  default:
    # 数据库配置 - 默认租户的专属数据库（向后兼容）
    database:
      driver: "postgres"
      host: "postgres-tenant-service"
      port: 5432
      database: "tenant-servicedb"
      username: "tenant"
      password: "password123"
      sslmode: "disable"
      max_open_conns: 50
      max_idle_conns: 10
      conn_max_idle_time: 300  # 秒
      conn_max_life_time: 3600  # 秒
      connect_timeout: 10  # 秒
      read_timeout: 30  # 秒
      write_timeout: 30  # 秒

    # NEW: 服务级数据库配置
    service_databases:
      evidence-command:
        driver: "mysql"
        host: "mysql-command"
        port: 3306
        database: "tenant_default_command"
        username: "tenant_default_cmd"
        password: "password123"
        sslmode: "disable"
        max_open_conns: 100
        max_idle_conns: 20

      evidence-query:
        driver: "postgres"
        host: "postgres-query"
        port: 5432
        database: "tenant_default_query"
        username: "tenant_default_query"
        password: "password123"
        sslmode: "disable"

      file-storage:
        driver: "postgres"
        host: "postgres-storage"
        port: 5432
        database: "tenant_default_storage"
        username: "tenant_default_storage"
        password: "password123"

      security-management:
        driver: "postgres"
        host: "postgres-security"
        port: 5432
        database: "securitydb"
        username: "tenant"
        password: "password123"

    # 域名配置 - 统一域名方案
    domain:
      primary: "app.example.com"  # 主域名（必填）
      aliases:  # 备用域名（可选）
        - "www.example.com"
      internal: "app.internal"  # 内部调用域名（可选）

    # FTP 配置数组（支持多个 FTP）
    ftp_configs:
      - username: "tenant1_sales_ftp"
        initial_password: "Sales@123456"
        description: "销售部 FTP"
        status: "active"

      - username: "tenant1_hr_ftp"
        initial_password: "HR@123456"
        description: "人事部 FTP"
        status: "active"

      - username: "tenant1_finance_ftp"
        initial_password: "Finance@123456"
        description: "财务部 FTP（临时停用）"
        status: "inactive"

    # 存储配置 - 默认租户的存储限制
    storage:
      upload_quota_gb: 1000  # 上传配额（GB）
      max_file_size_mb: 2048  # 单文件最大大小（MB）
      max_concurrent_uploads: 20  # 最大并发上传数

*/
