package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	// TODO: 取消注释以下import，需要添加etcd v3依赖
	// clientv3 "go.etcd.io/etcd/client/v3"
)

// etcdV3Client 基于etcd v3的客户端实现
type etcdV3Client struct {
	// client   *clientv3.Client
	config    *ClientConfig
	namespace string
}

// NewEtcdV3Client 创建etcd v3客户端
func NewEtcdV3Client(config *ClientConfig) (Client, error) {
	if config == nil {
		return nil, fmt.Errorf("etcd client config is required")
	}

	// TODO: 实现真正的etcd v3客户端创建
	// cfg := clientv3.Config{
	// 	Endpoints:   config.Endpoints,
	// 	DialTimeout: config.DialTimeout,
	// 	Username:    config.Username,
	// 	Password:    config.Password,
	// 	TLS:         config.TLS,
	// }
	//
	// client, err := clientv3.New(cfg)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create etcd client: %w", err)
	// }

	return &etcdV3Client{
		// client:   client,
		config:    config,
		namespace: config.Namespace,
	}, nil
}

// addNamespace 添加命名空间前缀
func (c *etcdV3Client) addNamespace(key string) string {
	if c.namespace == "" {
		return key
	}
	if strings.HasPrefix(key, c.namespace) {
		return key
	}
	return c.namespace + key
}

// removeNamespace 移除命名空间前缀
func (c *etcdV3Client) removeNamespace(key string) string {
	if c.namespace == "" {
		return key
	}
	return strings.TrimPrefix(key, c.namespace)
}

func (c *etcdV3Client) Put(ctx context.Context, key, value string, opts ...interface{}) error {
	// TODO: 实现put操作
	// _, err := c.client.Put(ctx, c.addNamespace(key), value)
	// return err
	return fmt.Errorf("not implemented yet")
}

func (c *etcdV3Client) Get(ctx context.Context, key string, opts ...interface{}) (string, error) {
	// TODO: 实现get操作
	// resp, err := c.client.Get(ctx, c.addNamespace(key))
	// if err != nil {
	// 	return "", err
	// }
	// if len(resp.Kvs) == 0 {
	// 	return "", fmt.Errorf("key not found: %s", key)
	// }
	// return string(resp.Kvs[0].Value), nil
	return "", fmt.Errorf("not implemented yet")
}

func (c *etcdV3Client) Delete(ctx context.Context, key string, opts ...interface{}) error {
	// TODO: 实现delete操作
	// _, err := c.client.Delete(ctx, c.addNamespace(key))
	// return err
	return fmt.Errorf("not implemented yet")
}

func (c *etcdV3Client) List(ctx context.Context, prefix string, opts ...interface{}) (map[string]string, error) {
	// TODO: 实现list操作
	// resp, err := c.client.Get(ctx, c.addNamespace(prefix), clientv3.WithPrefix())
	// if err != nil {
	// 	return nil, err
	// }
	//
	// result := make(map[string]string)
	// for _, kv := range resp.Kvs {
	// 	key := c.removeNamespace(string(kv.Key))
	// 	result[key] = string(kv.Value)
	// }
	// return result, nil
	return nil, fmt.Errorf("not implemented yet")
}

func (c *etcdV3Client) Watch(ctx context.Context, key string, opts ...interface{}) (<-chan WatchEvent, error) {
	// TODO: 实现watch操作
	// watchChan := c.client.Watch(ctx, c.addNamespace(key))
	// eventChan := make(chan WatchEvent)
	//
	// go func() {
	// 	defer close(eventChan)
	// 	for resp := range watchChan {
	// 		for _, event := range resp.Events {
	// 			eventType := WatchEventTypePut
	// 			if event.Type == mvccpb.DELETE {
	// 				eventType = WatchEventTypeDelete
	// 			}
	// 			eventChan <- WatchEvent{
	// 				Type:  eventType,
	// 				Key:   c.removeNamespace(string(event.Kv.Key)),
	// 				Value: string(event.Kv.Value),
	// 			}
	// 		}
	// 	}
	// }()
	//
	// return eventChan, nil
	return nil, fmt.Errorf("not implemented yet")
}

func (c *etcdV3Client) Register(ctx context.Context, serviceKey, serviceValue string, ttl int64) error {
	// TODO: 实现服务注册
	// lease, err := c.client.Grant(ctx, ttl)
	// if err != nil {
	// 	return fmt.Errorf("failed to grant lease: %w", err)
	// }
	//
	// _, err = c.client.Put(ctx, c.addNamespace(serviceKey), serviceValue, clientv3.WithLease(lease.ID))
	// if err != nil {
	// 	return fmt.Errorf("failed to register service: %w", err)
	// }
	//
	// // 续租
	// ch, kaerr := c.client.KeepAlive(ctx, lease.ID)
	// if kaerr != nil {
	// 	return fmt.Errorf("failed to keep alive lease: %w", kaerr)
	// }
	//
	// go func() {
	// 	for ka := range ch {
	// 		// 处理续租响应
	// 		_ = ka
	// 	}
	// }()

	return fmt.Errorf("not implemented yet")
}

func (c *etcdV3Client) Unregister(ctx context.Context, serviceKey string) error {
	return c.Delete(ctx, serviceKey)
}

func (c *etcdV3Client) Discover(ctx context.Context, servicePrefix string) ([]ServiceEndpoint, error) {
	services, err := c.List(ctx, servicePrefix)
	if err != nil {
		return nil, err
	}

	var endpoints []ServiceEndpoint
	for _, value := range services {
		var endpoint ServiceEndpoint
		if err := json.Unmarshal([]byte(value), &endpoint); err == nil {
			endpoints = append(endpoints, endpoint)
		}
	}

	return endpoints, nil
}

func (c *etcdV3Client) Close() error {
	// TODO: 关闭etcd客户端
	// return c.client.Close()
	return nil
}
