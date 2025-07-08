package etcd

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
)

// Client etcd客户端接口
type Client interface {
	// Put 存储键值对
	Put(ctx context.Context, key, value string, opts ...interface{}) error
	// Get 获取键值
	Get(ctx context.Context, key string, opts ...interface{}) (string, error)
	// Delete 删除键
	Delete(ctx context.Context, key string, opts ...interface{}) error
	// List 列出指定前缀的所有键值对
	List(ctx context.Context, prefix string, opts ...interface{}) (map[string]string, error)
	// Watch 监听键变化
	Watch(ctx context.Context, key string, opts ...interface{}) (<-chan WatchEvent, error)
	// Register 服务注册
	Register(ctx context.Context, serviceKey, serviceValue string, ttl int64) error
	// Unregister 服务注销
	Unregister(ctx context.Context, serviceKey string) error
	// Discover 服务发现
	Discover(ctx context.Context, servicePrefix string) ([]ServiceEndpoint, error)
	// Close 关闭客户端
	Close() error
}

// WatchEvent 监听事件
type WatchEvent struct {
	Type  WatchEventType
	Key   string
	Value string
}

// WatchEventType 监听事件类型
type WatchEventType int

const (
	WatchEventTypePut WatchEventType = iota
	WatchEventTypeDelete
)

// ServiceEndpoint 服务端点
type ServiceEndpoint struct {
	Address string            `json:"address"`
	Port    int               `json:"port"`
	Meta    map[string]string `json:"meta"`
}

// ClientConfig etcd客户端配置
type ClientConfig struct {
	Endpoints   []string
	Username    string
	Password    string
	Namespace   string
	DialTimeout time.Duration
	Timeout     time.Duration
	TLS         *tls.Config
}

var globalClient Client

// GetClient 获取全局etcd客户端
func GetClient() Client {
	return globalClient
}

// SetClient 设置全局etcd客户端
func SetClient(client Client) {
	if globalClient != nil {
		globalClient.Close()
	}
	globalClient = client
}

// NewClientFromConfig 从配置创建etcd客户端
func NewClientFromConfig(cfg *config.ETCDConfig) (Client, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	clientConfig := &ClientConfig{
		Endpoints:   cfg.Hosts,
		Username:    cfg.Username,
		Password:    cfg.Password,
		Namespace:   cfg.Namespace,
		DialTimeout: time.Duration(cfg.DialTimeout) * time.Second,
		Timeout:     time.Duration(cfg.Timeout) * time.Second,
	}

	if cfg.TLS.Enabled {
		tlsConfig := &tls.Config{}
		// TODO: 根据TLS配置构建tls.Config
		clientConfig.TLS = tlsConfig
	}

	// 使用具体的etcd v3客户端实现
	return NewEtcdV3Client(clientConfig)
}
