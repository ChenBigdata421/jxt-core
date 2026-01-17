package config

// ETCDConfig ETCD配置
type ETCDConfig struct {
	Enabled     bool              `mapstructure:"enabled" json:"enabled"`         // 是否启用ETCD
	Hosts       []string          `mapstructure:"hosts" json:"hosts"`             // ETCD主机列表
	Username    string            `mapstructure:"username" json:"username"`       // 用户名
	Password    string            `mapstructure:"password" json:"password"`       // 密码
	Namespace   string            `mapstructure:"namespace" json:"namespace"`     // 命名空间前缀
	DialTimeout int               `mapstructure:"dialTimeout" json:"dialTimeout"` // 连接超时(秒)
	Timeout     int               `mapstructure:"timeout" json:"timeout"`         // 操作超时(秒)
	TLS         TLSConfig         `mapstructure:"tls" json:"tls"`                 // TLS配置
	Tenant      *ETCDTenantConfig `mapstructure:"tenant" json:"tenant"`           // 租户相关配置
}

var EtcdConfig = new(ETCDConfig)

// IsEnabled 检查etcd是否启用
func (e *ETCDConfig) IsEnabled() bool {
	return e != nil && e.Enabled && len(e.Hosts) > 0
}

// GetNamespacedKey 获取带命名空间的键名
func (e *ETCDConfig) GetNamespacedKey(key string) string {
	if e.Namespace == "" {
		return key
	}
	return e.Namespace + key
}

// ETCDTenantConfig ETCD 租户数据提供者配置
type ETCDTenantConfig struct {
	WatchEnabled      bool   `mapstructure:"watchEnabled" json:"watchEnabled"`             // 启用 Watch 监听
	WatchRetryInterval int   `mapstructure:"watchRetryInterval" json:"watchRetryInterval"` // Watch 重试间隔(秒)
	CacheFile         string `mapstructure:"cacheFile" json:"cacheFile"`                   // 本地缓存文件路径
}

// GetCacheFile 获取缓存文件路径
func (c *ETCDTenantConfig) GetCacheFile() string {
	if c == nil || c.CacheFile == "" {
		return "./cache/tenant_metadata.json"
	}
	return c.CacheFile
}

// GetWatchRetryInterval 获取 Watch 重试间隔
func (c *ETCDTenantConfig) GetWatchRetryInterval() int {
	if c == nil || c.WatchRetryInterval <= 0 {
		return 30
	}
	return c.WatchRetryInterval
}
