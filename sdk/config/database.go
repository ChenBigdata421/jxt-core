package config

type Database struct { //jiyuanjie 添加 为了支持CQRS（主从库可以是完全不同类型数据库）
	CommandDB DBConfig
	QueryDB   DBConfig
}

type DBConfig struct {
	Driver          string
	Source          string
	ConnMaxIdleTime int
	ConnMaxLifeTime int
	MaxIdleConns    int
	MaxOpenConns    int
	Registers       []DBResolverConfig
}

type DBResolverConfig struct {
	Sources  []string
	Replicas []string
	Policy   string
	Tables   []string
}

var (
	DatabaseConfig  = new(Database)
	DatabasesConfig = make(map[string]*Database)
)
