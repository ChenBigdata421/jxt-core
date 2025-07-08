package config

// Application 应用程序配置
type Application struct {
	Mode     string `mapstructure:"mode" json:"mode"`         // 环境: dev, test, prod
	Name     string `mapstructure:"name" json:"name"`         // 应用名称
	EnableDP bool   `mapstructure:"enabledp" json:"enabledp"` // 数据权限功能开关

}

var ApplicationConfig = new(Application)
