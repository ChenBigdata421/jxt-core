package config

// Application 应用程序配置
type Application struct {
	Mode          string `mapstructure:"mode" json:"mode"`
	Host          string `mapstructure:"host" json:"host"`
	Name          string `mapstructure:"name" json:"name"`
	Port          int    `mapstructure:"port" json:"port"`
	ReadTimeout   int    `mapstructure:"readtimeout" json:"readtimeout"`
	WriterTimeout int    `mapstructure:"writertimeout" json:"writetimeout"`
	EnableDP      bool   `mapstructure:"enabledp" json:"enabledp"`
	DemoMsg       string `mapstructure:"demomsg" json:"demomsg"`
}

var ApplicationConfig = new(Application)
