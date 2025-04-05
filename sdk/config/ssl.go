package config

type SSL struct {
	KeyStr string `mapstructure:"key_str"` // 私钥内容（字符串形式）
	Pem    string `mapstructure:"pem"`     // 证书内容（PEM格式，字符串形式）
	Enable bool   `mapstructure:"enable"`  // 是否启用安全连接
	Domain string `mapstructure:"domain"`  // 证书域名（用于生成证书）
}

var SslConfig = new(SSL)
