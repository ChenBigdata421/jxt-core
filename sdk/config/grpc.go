package config

// RPCConfig gRPC服务配置(Go-Zero)
type RPCConfig struct {
	Enabled        bool                     `mapstructure:"enabled" json:"enabled"`               // 是否启用gRPC
	ListenOn       string                   `mapstructure:"listenOn" json:"listenOn"`             // 监听地址
	Timeout        int                      `mapstructure:"timeout" json:"timeout"`               // 超时(毫秒)
	CpuThreshold   int                      `mapstructure:"cpuThreshold" json:"cpuThreshold"`     // CPU阈值
	Etcd           EtcdConfig               `mapstructure:"etcd" json:"etcd"`                     // ETCD配置
	ServerTLS      TLSConfig                `mapstructure:"serverTLS" json:"serverTLS"`           // TLS配置
	MaxRecvMsgSize int                      `mapstructure:"maxRecvMsgSize" json:"maxRecvMsgSize"` // 最大接收消息
	MaxSendMsgSize int                      `mapstructure:"maxSendMsgSize" json:"maxSendMsgSize"` // 最大发送消息
	KeepAlive      KeepAliveConfig          `mapstructure:"keepalive" json:"keepalive"`           // 保活配置
	Services       map[string]ServiceConfig `mapstructure:"services" json:"services"`             // 服务配置
}

// EtcdConfig ETCD服务发现配置
type EtcdConfig struct {
	Hosts []string `mapstructure:"hosts" json:"hosts"` // ETCD主机列表
	Key   string   `mapstructure:"key" json:"key"`     // 服务注册键名
}

// TLSConfig TLS配置
type TLSConfig struct {
	Enabled bool   `mapstructure:"enabled" json:"enabled"` // 是否启用TLS
	KeyStr  string `mapstructure:"key_str"`                // 私钥内容（字符串形式）
	Pem     string `mapstructure:"pem"`                    // 证书内容（PEM格式，字符串形式）
	Domain  string `mapstructure:"domain"`                 // 证书域名（用于生成证书）
}

// KeepAliveConfig gRPC KeepAlive配置
type KeepAliveConfig struct {
	Time                int  `mapstructure:"time" json:"time"`                               // 保活时间(秒)
	Timeout             int  `mapstructure:"timeout" json:"timeout"`                         // 保活超时(秒)
	PermitWithoutStream bool `mapstructure:"permitWithoutStream" json:"permitWithoutStream"` // 允许无流保活
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Enabled      bool     `mapstructure:"enabled" json:"enabled"`           // 服务启用状态
	Interceptors []string `mapstructure:"interceptors" json:"interceptors"` // 拦截器列表
}

var RpcConfig = new(RPCConfig)
var ServiceConfigMap = make(map[string]ServiceConfig)
