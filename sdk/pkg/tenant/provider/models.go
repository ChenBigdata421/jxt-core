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
