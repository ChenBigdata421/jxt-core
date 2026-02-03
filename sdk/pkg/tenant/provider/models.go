package provider

// TenantMeta represents tenant metadata from ETCD
type TenantMeta struct {
	TenantID    int    `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Status      string `json:"status"`      // active, inactive, suspended
	BillingPlan string `json:"billingPlan"` // optional
}

// IsEnabled returns true if tenant status is active
func (m *TenantMeta) IsEnabled() bool {
	return m.Status == "active"
}

// DatabaseConfig represents tenant database configuration
type DatabaseConfig struct {
	TenantID  int    `json:"tenantId"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	DbName    string `json:"dbName"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	SSLMode   string `json:"sslMode"`

	MaxOpenConns    int `json:"maxOpenConns"`
	MaxIdleConns    int `json:"maxIdleConns"`
	ConnMaxLifeTime int `json:"connMaxLifeTime"`
	ConnMaxIdleTime int `json:"connMaxIdleTime"`

	ConnectTimeout int `json:"connectTimeout"`
	ReadTimeout    int `json:"readTimeout"`
	WriteTimeout   int `json:"writeTimeout"`
}

// FtpConfig represents tenant FTP configuration
type FtpConfig struct {
	TenantID        int    `json:"tenantId"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	Username        string `json:"username"`
	PasswordHash    string `json:"passwordHash"`
	HomeDirectory   string `json:"homeDirectory"`
	WritePermission bool   `json:"writePermission"`
}

// StorageConfig represents tenant storage configuration
type StorageConfig struct {
	TenantID             int64  `json:"tenantId"`
	Code                 string `json:"code"`
	Name                 string `json:"name"`
	QuotaBytes           int64  `json:"quotaBytes"`
	MaxFileSizeBytes     int64  `json:"maxFileSizeBytes"`
	MaxConcurrentUploads int    `json:"maxConcurrentUploads"`
}

// ========== 新增：服务级数据库配置 ==========

// ServiceDatabaseConfig 服务级数据库配置
type ServiceDatabaseConfig struct {
	TenantID          int    `json:"tenantId"`
	ServiceCode       string `json:"serviceCode"` // evidence-command, evidence-query, file-storage, security-management
	Driver            string `json:"driver"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Database          string `json:"database"`
	Username          string `json:"username"`
	Password          string `json:"password"`           // 加密后的密码（Base64编码）
	SSLMode           string `json:"sslMode"`
	MaxOpenConns      int    `json:"maxOpenConns"`
	MaxIdleConns      int    `json:"maxIdleConns"`
	PasswordEncrypted bool   `json:"passwordEncrypted"` // 密码加密标识
}

// HasEncryptedPassword 检查是否有加密的密码
func (c *ServiceDatabaseConfig) HasEncryptedPassword() bool {
	return c.PasswordEncrypted && c.Password != ""
}

// ========== 新增：FTP配置详情 ==========

// FtpConfigDetail FTP配置详情（支持多配置）
type FtpConfigDetail struct {
	TenantID        int    `json:"tenantId"`
	Username        string `json:"username"`
	PasswordHash    string `json:"passwordHash"`
	Description     string `json:"description"`  // FTP 配置描述
	Status          string `json:"status"`        // active, inactive
	HomeDirectory   string `json:"homeDirectory"`
	WritePermission bool   `json:"writePermission"`
}

// ========== 新增：域名配置 ==========

// DomainConfig 租户域名配置
type DomainConfig struct {
	TenantID int      `json:"tenantId"`
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Primary  string   `json:"primary"`  // 主域名
	Aliases  []string `json:"aliases"`  // 别名列表
	Internal string   `json:"internal"` // 内部域名
}

// ========== FTP Helper Functions ==========

// appendOrUpdateFtpConfig 添加或更新FTP配置
func appendOrUpdateFtpConfig(configs []*FtpConfigDetail, newConfig *FtpConfigDetail) []*FtpConfigDetail {
	for i, cfg := range configs {
		if cfg.Username == newConfig.Username {
			configs[i] = newConfig
			return configs
		}
	}
	return append(configs, newConfig)
}

// removeFtpConfigByUsername 从配置列表中移除指定用户名的配置
func removeFtpConfigByUsername(configs []*FtpConfigDetail, username string) []*FtpConfigDetail {
	for i, cfg := range configs {
		if cfg.Username == username {
			return append(configs[:i], configs[i+1:]...)
		}
	}
	return configs
}
