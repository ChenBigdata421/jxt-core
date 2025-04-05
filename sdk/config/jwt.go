package config

// JWT JWT配置
type JWT struct {
	Secret  string `mapstructure:"secret"`
	Timeout int    `mapstructure:"timeout"`
}

var JwtConfig = new(JWT)
