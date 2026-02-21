package provider

import (
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/crypto"
)

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

// DecryptPassword 解密数据库密码
// encryptionKey: 32字节的 AES-256 密钥（由调用者传入）
// 返回: 解密后的明文密码
func (c *ServiceDatabaseConfig) DecryptPassword(encryptionKey string) (string, error) {
	if c.Password == "" {
		return "", fmt.Errorf("password is empty")
	}
	if !c.PasswordEncrypted {
		return "", fmt.Errorf("password is not encrypted")
	}

	cryptoService, err := crypto.NewCryptoService(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create crypto service: %w", err)
	}

	password, err := cryptoService.Decrypt(c.Password)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt password: %w", err)
	}

	return password, nil
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

// ========== 新增：租户识别配置 ==========

// ResolverConfig 租户识别配置（全局配置，不属于特定租户）
// 从 ETCD key: common/resolver 加载
//
// 注意：通过 Provider 获取的 ResolverConfig 指针应视为只读，不可修改字段值。
// 如需修改，请复制后使用。
type ResolverConfig struct {
	ID             int64     `json:"id"`             // 数据库主键，消费方不使用，仅用于反序列化兼容
	HTTPType       string    `json:"httpType"`       // host, header, query, path
	HTTPHeaderName string    `json:"httpHeaderName"` // For type=header
	HTTPQueryParam string    `json:"httpQueryParam"` // For type=query
	HTTPPathIndex  int       `json:"httpPathIndex"`  // For type=path
	HTTPHostMode   string    `json:"httpHostMode,omitempty"` // 【新增】host 模式：numeric, domain, code
	FTPType        string    `json:"ftpType"`        // 目前只支持 username
	CreatedAt      time.Time `json:"createdAt"`      // 创建时间，调试和审计用
	UpdatedAt      time.Time `json:"updatedAt"`      // 更新时间，调试和审计用
}

// GetHTTPHostModeOrDefault 返回 host 模式，默认为 "numeric"
func (c *ResolverConfig) GetHTTPHostModeOrDefault() string {
	if c == nil || c.HTTPHostMode == "" {
		return "numeric"
	}
	return c.HTTPHostMode
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
