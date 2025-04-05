package config

type SSL struct {
	KeyStr string `mapstructure:"key_str"`
	Pem    string `mapstructure:"pem"`
	Enable bool   `mapstructure:"enable"`
	Domain string `mapstructure:"domain"`
}

var SslConfig = new(SSL)
