package config

// HTTPConfig HTTP服务器配置(Gin)
type HTTPConfig struct {
	Enabled        bool      `mapstructure:"enabled" json:"enabled"`               // 是否启用HTTP服务
	Host           string    `mapstructure:"host" json:"host"`                     // 服务器绑定IP
	Port           int       `mapstructure:"port" json:"port"`                     // HTTP端口
	ReadTimeout    int       `mapstructure:"readtimeout" json:"readtimeout"`       // 读取超时(秒)
	WriteTimeout   int       `mapstructure:"writetimeout" json:"writetimeout"`     // 写入超时(秒)
	IdleTimeout    int       `mapstructure:"idletimeout" json:"idletimeout"`       // 空闲超时(秒)
	MaxHeaderBytes int       `mapstructure:"maxheaderbytes" json:"maxheaderbytes"` // 最大请求头(MB)
	SSL            SSLConfig `mapstructure:"ssl" json:"ssl"`                       // SSL配置
}

// SSLConfig SSL/TLS配置
type SSLConfig struct {
	Enabled bool   `mapstructure:"enabled" json:"enabled"` // 是否启用HTTPS
	KeyStr  string `mapstructure:"key_str"`                // 私钥内容（字符串形式）
	Pem     string `mapstructure:"pem"`                    // 证书内容（PEM格式，字符串形式）
	Domain  string `mapstructure:"domain"`                 // 证书域名（用于生成证书）
}

var HttpConfig = new(HTTPConfig)
