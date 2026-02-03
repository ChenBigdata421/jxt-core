package database

// TenantDatabaseConfig 租户数据库配置
type TenantDatabaseConfig struct {
	// 租户信息
	TenantID int    `json:"tenant_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`

	// 服务信息
	ServiceCode string `json:"service_code"`

	// 数据库连接参数
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	DbName   string `json:"db_name"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSLMode  string `json:"ssl_mode"`

	// 连接池设置
	MaxOpenConns int `json:"max_open_conns"`
	MaxIdleConns int `json:"max_idle_conns"`
}
