package config

// FTPConfig FTP 服务器配置
var FTPConfigInstance = new(FTPConfig)

// FTPConfig FTP 服务器配置
type FTPConfig struct {
	Enabled           bool            `mapstructure:"enabled" yaml:"enabled"`
	ListenAddr        string          `mapstructure:"listen_addr" yaml:"listen_addr"`
	PublicHost        string          `mapstructure:"public_host" yaml:"public_host"`
	PassivePortRange  PortRange       `mapstructure:"passive_port_range" yaml:"passive_port_range"`
	IdleTimeout       int             `mapstructure:"idle_timeout" yaml:"idle_timeout"`
	ConnectionTimeout int             `mapstructure:"connection_timeout" yaml:"connection_timeout"`
	TLS               FTPTLSConfig    `mapstructure:"tls" yaml:"tls"`
	Users             []FTPUserConfig `mapstructure:"users" yaml:"users"`
}

// PortRange 端口范围配置
type PortRange struct {
	Start int `mapstructure:"start" yaml:"start"`
	End   int `mapstructure:"end" yaml:"end"`
}

// FTPTLSConfig FTP TLS 配置
type FTPTLSConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	CertFile string `mapstructure:"cert_file" yaml:"cert_file"`
	KeyFile  string `mapstructure:"key_file" yaml:"key_file"`
}

// FTPUserConfig FTP 静态用户配置（非多租户模式使用）
type FTPUserConfig struct {
	Username        string `mapstructure:"username" yaml:"username"`
	Password        string `mapstructure:"password" yaml:"password"`
	HomeDirectory   string `mapstructure:"home_directory" yaml:"home_directory"`
	WritePermission bool   `mapstructure:"write_permission" yaml:"write_permission"`
}

// GetListenAddr 获取监听地址，有默认值
func (f *FTPConfig) GetListenAddr() string {
	if f == nil || f.ListenAddr == "" {
		return ":21"
	}
	return f.ListenAddr
}

// GetPublicHost 获取公共主机地址
func (f *FTPConfig) GetPublicHost() string {
	if f == nil || f.PublicHost == "" {
		return "127.0.0.1"
	}
	return f.PublicHost
}

// GetIdleTimeout 获取空闲超时时间（秒），有默认值
func (f *FTPConfig) GetIdleTimeout() int {
	if f == nil || f.IdleTimeout <= 0 {
		return 60
	}
	return f.IdleTimeout
}

// GetConnectionTimeout 获取连接超时时间（秒），有默认值
func (f *FTPConfig) GetConnectionTimeout() int {
	if f == nil || f.ConnectionTimeout <= 0 {
		return 30
	}
	return f.ConnectionTimeout
}

// GetPassivePortStart 获取被动端口范围起始
func (f *FTPConfig) GetPassivePortStart() int {
	if f == nil || f.PassivePortRange.Start <= 0 {
		return 8091
	}
	return f.PassivePortRange.Start
}

// GetPassivePortEnd 获取被动端口范围结束
func (f *FTPConfig) GetPassivePortEnd() int {
	if f == nil || f.PassivePortRange.End <= 0 {
		return 8100
	}
	return f.PassivePortRange.End
}

// IsTLSEnabled 检查是否启用 TLS
func (f *FTPConfig) IsTLSEnabled() bool {
	if f == nil {
		return false
	}
	return f.TLS.Enabled
}

// GetTLSCertFile 获取 TLS 证书文件路径
func (f *FTPConfig) GetTLSCertFile() string {
	if f == nil {
		return ""
	}
	return f.TLS.CertFile
}

// GetTLSKeyFile 获取 TLS 密钥文件路径
func (f *FTPConfig) GetTLSKeyFile() string {
	if f == nil {
		return ""
	}
	return f.TLS.KeyFile
}

// GetUsers 获取静态用户列表
func (f *FTPConfig) GetUsers() []FTPUserConfig {
	if f == nil {
		return nil
	}
	return f.Users
}

// FindUser 根据用户名查找用户
func (f *FTPConfig) FindUser(username string) *FTPUserConfig {
	if f == nil {
		return nil
	}
	for i := range f.Users {
		if f.Users[i].Username == username {
			return &f.Users[i]
		}
	}
	return nil
}

