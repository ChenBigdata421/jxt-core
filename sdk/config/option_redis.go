package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/redis/go-redis/v9"
)

var (
	_redis      *redis.Client // Client #1: shared (non-blocking operations)
	_redisQueue *redis.Client // Client #2: queue consumer (blocking XREADGROUP)
	_redisSub   *redis.Client // Client #3: subscriber (PSUBSCRIBE)
	_redisMu    sync.RWMutex  // RWMutex — reads don't block each other
)

// GetRedisClient returns Client #1 (shared) without acquiring the lock.
// For initialization, use EnsureRedisClient instead.
func GetRedisClient() *redis.Client {
	_redisMu.RLock()
	defer _redisMu.RUnlock()
	return _redis
}

// GetQueueConsumerClient returns Client #2 (queue consumer) without acquiring the lock.
func GetQueueConsumerClient() *redis.Client {
	_redisMu.RLock()
	defer _redisMu.RUnlock()
	return _redisQueue
}

// GetSubscriberClient returns Client #3 (subscriber) without acquiring the lock.
func GetSubscriberClient() *redis.Client {
	_redisMu.RLock()
	defer _redisMu.RUnlock()
	return _redisSub
}

// SetRedisClient sets Client #1 (for external injection).
// Assignment only — does NOT close the previous client (R-A3).
// The old Shutdown() call was a bug: Shutdown() sends the Redis SERVER a
// SHUTDOWN command, killing the Redis process. Use CloseAllRedisClients for
// orderly cleanup.
func SetRedisClient(c *redis.Client) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	_redis = c
}

// EnsureRedisClient returns Client #1, creating it if necessary.
// If a client already exists, it validates that addr and DB match the
// requested options and returns an error on conflict.
func EnsureRedisClient(options *redis.Options) (*redis.Client, error) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redis != nil {
		existing := _redis.Options()
		if existing.Addr != options.Addr || existing.DB != options.DB {
			return nil, fmt.Errorf("redis config conflict: existing client %s db=%d, requested %s db=%d",
				existing.Addr, existing.DB, options.Addr, options.DB)
		}
		return _redis, nil
	}
	client := redis.NewClient(options)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis connect failed: %w", err)
	}
	_redis = client
	return client, nil
}

// EnsureQueueConsumerClient returns Client #2 (queue consumer), creating it
// if necessary. Validates addr/DB consistency against Client #1 if it exists.
func EnsureQueueConsumerClient(options *redis.Options) (*redis.Client, error) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redisQueue != nil {
		return _redisQueue, nil
	}
	if _redis != nil {
		existing := _redis.Options()
		if existing.Addr != options.Addr || existing.DB != options.DB {
			return nil, fmt.Errorf("redis queue config conflict: shared client %s db=%d, requested %s db=%d",
				existing.Addr, existing.DB, options.Addr, options.DB)
		}
	}
	client := redis.NewClient(options)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis queue connect failed: %w", err)
	}
	_redisQueue = client
	return client, nil
}

// EnsureSubscriberClient returns Client #3 (subscriber), creating it if
// necessary. Validates addr/DB consistency against Client #1 if it exists.
func EnsureSubscriberClient(options *redis.Options) (*redis.Client, error) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redisSub != nil {
		return _redisSub, nil
	}
	if _redis != nil {
		existing := _redis.Options()
		if existing.Addr != options.Addr || existing.DB != options.DB {
			return nil, fmt.Errorf("redis sub config conflict: shared client %s db=%d, requested %s db=%d",
				existing.Addr, existing.DB, options.Addr, options.DB)
		}
	}
	client := redis.NewClient(options)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis sub connect failed: %w", err)
	}
	_redisSub = client
	return client, nil
}

// CloseAllRedisClients closes all three Redis clients and resets them to nil.
// This is the only place where Close() is called — SetRedisClient does NOT
// close the previous client.
func CloseAllRedisClients() {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redis != nil {
		_redis.Close()
		_redis = nil
	}
	if _redisQueue != nil {
		_redisQueue.Close()
		_redisQueue = nil
	}
	if _redisSub != nil {
		_redisSub.Close()
		_redisSub = nil
	}
}

// ResetRedisClientsForTest clears all global Redis clients WITHOUT closing
// them. For use in tests only — allows resetting global state between test
// cases without triggering actual network operations on mock clients.
func ResetRedisClientsForTest() {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	_redis = nil
	_redisQueue = nil
	_redisSub = nil
}

type RedisConnectOptions struct {
	Network    string `mapstructure:"network" json:"network"`
	Addr       string `mapstructure:"addr" json:"addr"`
	Username   string `mapstructure:"username" json:"username"`
	Password   string `mapstructure:"password" json:"password"`
	DB         int    `mapstructure:"db" json:"db"`
	PoolSize   int    `mapstructure:"pool_size" json:"pool_size"`
	Tls        *Tls   `mapstructure:"tls" json:"tls"`
	MaxRetries int    `mapstructure:"max_retries" json:"max_retries"`
}

type Tls struct {
	Cert string `mapstructure:"cert" json:"cert"`
	Key  string `mapstructure:"key" json:"key"`
	Ca   string `mapstructure:"ca" json:"ca"`
}

func (e RedisConnectOptions) GetRedisOptions() (*redis.Options, error) {
	r := &redis.Options{
		Network:    e.Network,
		Addr:       e.Addr,
		Username:   e.Username,
		Password:   e.Password,
		DB:         e.DB,
		MaxRetries: e.MaxRetries,
		PoolSize:   e.PoolSize,
	}
	var err error
	r.TLSConfig, err = getTLS(e.Tls)
	return r, err
}

func getTLS(c *Tls) (*tls.Config, error) {
	if c != nil && c.Cert != "" {
		// 从证书相关文件中读取和解析信息，得到证书公钥、密钥对
		cert, err := tls.LoadX509KeyPair(c.Cert, c.Key)
		if err != nil {
			fmt.Printf("tls.LoadX509KeyPair err: %v\n", err)
			return nil, err
		}
		// 创建一个新的、空的 CertPool，并尝试解析 PEM 编码的证书，解析成功会将其加到 CertPool 中
		certPool := x509.NewCertPool()
		ca, err := ioutil.ReadFile(c.Ca)
		if err != nil {
			fmt.Printf("ioutil.ReadFile err: %v\n", err)
			return nil, err
		}

		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			fmt.Println("certPool.AppendCertsFromPEM err")
			return nil, err
		}
		return &tls.Config{
			// 设置证书链，允许包含一个或多个
			Certificates: []tls.Certificate{cert},
			// 要求必须校验客户端的证书
			ClientAuth: tls.RequireAndVerifyClientCert,
			// 设置根证书的集合，校验方式使用 ClientAuth 中设定的模式
			ClientCAs: certPool,
		}, nil
	}
	return nil, nil
}
