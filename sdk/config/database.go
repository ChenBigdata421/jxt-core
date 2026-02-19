package config

// Database 数据库配置
/*
type Database struct {
	MasterDB  DBConfig `mapstructure:"masterDB"`
	CommandDB DBConfig `mapstructure:"commandDB"`
	QueryDB   DBConfig `mapstructure:"queryDB"`
	ProcessDB DBConfig `mapstructure:"processDB"`
}

// DBConfig 数据库具体配置
*/

type Database struct {
	Driver          string     `mapstructure:"driver"`
	Source          string     `mapstructure:"source"`
	ConnMaxIdleTime int        `mapstructure:"connmaxidletime"`
	ConnMaxLifeTime int        `mapstructure:"connmaxlifetime"`
	MaxIdleConns    int        `mapstructure:"maxidleconns"`
	MaxOpenConns    int        `mapstructure:"maxopenconns"`
	Registers       []Register `mapstructure:"registers"`
}

type Register struct {
	Sources  []string `mapstructure:"sources"`
	Replicas []string `mapstructure:"replicas"`
	Policy   string   `mapstructure:"policy"`
	Tables   []string `mapstructure:"tables"`
}

var (
	DatabaseConfig = new(Database)
)
