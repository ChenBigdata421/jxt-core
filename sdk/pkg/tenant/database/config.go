package database

// TenantDatabaseConfig holds the database configuration for a tenant.
// This includes connection parameters, pool settings, and timeout configurations.
type TenantDatabaseConfig struct {
	// Tenant identification
	TenantID int    `json:"tenant_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`

	// Database connection parameters
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	DbName   string `json:"db_name"`
	Username string `json:"username"`
	Password string `json:"password"`
	SSLMode  string `json:"ssl_mode"`

	// Connection pool settings
	MaxOpenConns    int `json:"max_open_conns"`
	MaxIdleConns    int `json:"max_idle_conns"`
	ConnMaxLifeTime int `json:"conn_max_life_time"` // in seconds
	ConnMaxIdleTime int `json:"conn_max_idle_time"` // in seconds

	// Timeout settings (in seconds)
	ConnectTimeout int `json:"connect_timeout"`
	ReadTimeout    int `json:"read_timeout"`
	WriteTimeout   int `json:"write_timeout"`
}
