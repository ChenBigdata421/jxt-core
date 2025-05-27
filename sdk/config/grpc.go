package config

import "time"

// GRPCConfig gRPC服务配置
type GRPCConfig struct {
	Server GRPCServerConfig `mapstructure:"server" json:"server"` // gRPC服务配置
	Client GRPCClientConfig `mapstructure:"client" json:"client"` // gRPC客户端配置
}

// GRPCServerConfig gRPC服务端配置
type GRPCServerConfig struct {
	Enabled    bool   `mapstructure:"enabled" json:"enabled,default=true"`    // 是否启用gRPC服务
	ListenOn   string `mapstructure:"listenOn" json:"listenOn,default=:9000"` // 监听地址
	ServiceKey string `mapstructure:"serviceKey" json:"serviceKey"`           // 服务发现键名
	Timeout    int64  `mapstructure:"timeout" json:"timeout,default=2000"`    // 超时时间(毫秒)

	// 认证配置
	Auth bool `mapstructure:"auth" json:"auth,optional"` // 是否启用认证

	// 性能控制
	CpuThreshold   int64 `mapstructure:"cpuThreshold" json:"cpuThreshold,default=900,range=[0:1000]"` // CPU阈值
	StrictControl  bool  `mapstructure:"strictControl" json:"strictControl,optional"`                 // 严格控制模式
	MaxRecvMsgSize int   `mapstructure:"maxRecvMsgSize" json:"maxRecvMsgSize,optional"`               // 最大接收消息大小
	MaxSendMsgSize int   `mapstructure:"maxSendMsgSize" json:"maxSendMsgSize,optional"`               // 最大发送消息大小

	// TLS配置
	TLS TLSConfig `mapstructure:"tls" json:"tls,optional"` // TLS配置

	// KeepAlive配置
	KeepAlive KeepAliveConfig `mapstructure:"keepAlive" json:"keepAlive,optional"` // 保活配置

	// 中间件配置
	Middlewares ServerMiddlewaresConfig `mapstructure:"middlewares" json:"middlewares,optional"` // 中间件配置

	// 自定义服务配置
	Services map[string]ServiceConfig `mapstructure:"services" json:"services,optional"` // 服务配置
}

// GRPCClientConfig gRPC客户端配置
type GRPCClientConfig struct {
	Enabled       bool          `mapstructure:"enabled" json:"enabled,default=true"`            // 是否启用gRPC客户端
	ServiceKey    string        `mapstructure:"serviceKey" json:"serviceKey"`                   // 服务发现键名
	Timeout       int64         `mapstructure:"timeout" json:"timeout,default=2000"`            // 超时时间(毫秒)
	KeepaliveTime time.Duration `mapstructure:"keepaliveTime" json:"keepaliveTime,default=20s"` // Keepalive时间

	// 连接配置
	Endpoints    []string `mapstructure:"endpoints" json:"endpoints,optional"`           // 直连端点列表
	Target       string   `mapstructure:"target" json:"target,optional"`                 // gRPC目标地址
	NonBlock     bool     `mapstructure:"nonBlock" json:"nonBlock,optional"`             // 非阻塞模式
	UseDiscovery bool     `mapstructure:"useDiscovery" json:"useDiscovery,default=true"` // 是否使用服务发现

	// 认证配置
	App   string `mapstructure:"app" json:"app,optional"`     // 应用名称
	Token string `mapstructure:"token" json:"token,optional"` // 认证Token

	// 性能配置
	Retries        int `mapstructure:"retries" json:"retries,default=3"`              // 重试次数
	MaxRecvMsgSize int `mapstructure:"maxRecvMsgSize" json:"maxRecvMsgSize,optional"` // 最大接收消息大小
	MaxSendMsgSize int `mapstructure:"maxSendMsgSize" json:"maxSendMsgSize,optional"` // 最大发送消息大小

	// 中间件配置
	Middlewares ClientMiddlewaresConfig `mapstructure:"middlewares" json:"middlewares,optional"` // 中间件配置
}

// TLSConfig TLS配置
type TLSConfig struct {
	Enabled bool   `mapstructure:"enabled" json:"enabled,optional"` // 是否启用TLS
	KeyStr  string `mapstructure:"key_str" json:"key_str,optional"` // 私钥内容
	Pem     string `mapstructure:"pem" json:"pem,optional"`         // 证书内容
	Domain  string `mapstructure:"domain" json:"domain,optional"`   // 证书域名
}

// KeepAliveConfig gRPC KeepAlive配置
type KeepAliveConfig struct {
	Time                int  `mapstructure:"time" json:"time,default=7200"`                                // 保活时间(秒)
	Timeout             int  `mapstructure:"timeout" json:"timeout,default=20"`                            // 保活超时(秒)
	PermitWithoutStream bool `mapstructure:"permitWithoutStream" json:"permitWithoutStream,default=false"` // 允许无流保活
}

// ServerMiddlewaresConfig 服务端中间件配置
type ServerMiddlewaresConfig struct {
	Trace      bool `mapstructure:"trace" json:"trace,default=true"`           // 链路跟踪
	Recover    bool `mapstructure:"recover" json:"recover,default=true"`       // 异常恢复
	Stat       bool `mapstructure:"stat" json:"stat,default=true"`             // 统计
	Prometheus bool `mapstructure:"prometheus" json:"prometheus,default=true"` // Prometheus监控
	Breaker    bool `mapstructure:"breaker" json:"breaker,default=true"`       // 熔断器
}

// ClientMiddlewaresConfig 客户端中间件配置
type ClientMiddlewaresConfig struct {
	Trace      bool `mapstructure:"trace" json:"trace,default=true"`           // 链路跟踪
	Duration   bool `mapstructure:"duration" json:"duration,default=true"`     // 持续时间统计
	Prometheus bool `mapstructure:"prometheus" json:"prometheus,default=true"` // Prometheus监控
	Breaker    bool `mapstructure:"breaker" json:"breaker,default=true"`       // 熔断器
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	Enabled      bool     `mapstructure:"enabled" json:"enabled,default=true"`       // 服务启用状态
	Interceptors []string `mapstructure:"interceptors" json:"interceptors,optional"` // 拦截器列表
}

var GrpcConfig = new(GRPCConfig)
var ServiceConfigMap = make(map[string]ServiceConfig)

// ToGoZeroRpcServerConf 转换为go-zero的RpcServerConf
func (c *GRPCServerConfig) ToGoZeroRpcServerConf(etcdConfig *ETCDConfig) map[string]interface{} {
	conf := map[string]interface{}{
		"ListenOn":      c.ListenOn,
		"Timeout":       c.Timeout,
		"CpuThreshold":  c.CpuThreshold,
		"Auth":          c.Auth,
		"StrictControl": c.StrictControl,
	}

	// 添加ETCD配置
	if etcdConfig != nil && etcdConfig.Enabled {
		conf["Etcd"] = map[string]interface{}{
			"Hosts": etcdConfig.Hosts,
			"Key":   c.ServiceKey,
			"User":  etcdConfig.Username,
			"Pass":  etcdConfig.Password,
		}
	}

	return conf
}

// ToGoZeroRpcClientConf 转换为go-zero的RpcClientConf
func (c *GRPCClientConfig) ToGoZeroRpcClientConf(etcdConfig *ETCDConfig) map[string]interface{} {
	conf := map[string]interface{}{
		"Timeout":       c.Timeout,
		"KeepaliveTime": c.KeepaliveTime,
		"NonBlock":      c.NonBlock,
		"App":           c.App,
		"Token":         c.Token,
	}

	// 根据配置选择连接方式
	if c.UseDiscovery && etcdConfig != nil && etcdConfig.Enabled {
		// 使用服务发现
		conf["Etcd"] = map[string]interface{}{
			"Hosts": etcdConfig.Hosts,
			"Key":   c.ServiceKey,
			"User":  etcdConfig.Username,
			"Pass":  etcdConfig.Password,
		}
	} else if len(c.Endpoints) > 0 {
		// 使用直连
		conf["Endpoints"] = c.Endpoints
	} else if c.Target != "" {
		// 使用Target
		conf["Target"] = c.Target
	}

	return conf
}
