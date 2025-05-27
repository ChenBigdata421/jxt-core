package config

// ETCDConfig ETCD配置
type ETCDConfig struct {
	Enabled     bool      `mapstructure:"enabled" json:"enabled"`         // 是否启用ETCD
	Hosts       []string  `mapstructure:"hosts" json:"hosts"`             // ETCD主机列表
	Username    string    `mapstructure:"username" json:"username"`       // 用户名
	Password    string    `mapstructure:"password" json:"password"`       // 密码
	Namespace   string    `mapstructure:"namespace" json:"namespace"`     // 命名空间前缀
	DialTimeout int       `mapstructure:"dialTimeout" json:"dialTimeout"` // 连接超时(秒)
	Timeout     int       `mapstructure:"timeout" json:"timeout"`         // 操作超时(秒)
	TLS         TLSConfig `mapstructure:"tls" json:"tls"`                 // TLS配置
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
