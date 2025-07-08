package config

// JWT JWT配置
type JWTConfig struct {
	Secret  string `mapstructure:"secret"`
	Timeout int    `mapstructure:"timeout"`
}

var JwtConfig = new(JWTConfig)
