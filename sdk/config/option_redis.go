package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	_redis              *redis.Client // cache（casbin pub + 健康检查依赖）
	_redisQueue         *redis.Client // queue consumer（adapter 用）
	_redisQueueProducer *redis.Client // queue producer（adapter 用；独立池，避免阻塞 XREADGROUP 饿死非阻塞操作）
	_redisLocker        *redis.Client // locker（adapter 用）
	_redisSub           *redis.Client // subscriber（casbin sub 依赖）
	_redisMu            sync.RWMutex  // RWMutex — reads don't block each other
)

// GetRedisClient returns Client #1 (shared) with a read lock.
// For initialization, use EnsureRedisClient instead.
func GetRedisClient() *redis.Client {
	_redisMu.RLock()
	defer _redisMu.RUnlock()
	return _redis
}

// GetQueueConsumerClient returns Client #2 (queue consumer) with a read lock.
func GetQueueConsumerClient() *redis.Client {
	_redisMu.RLock()
	defer _redisMu.RUnlock()
	return _redisQueue
}

// GetQueueProducerClient returns the queue producer client with a read lock.
func GetQueueProducerClient() *redis.Client {
	_redisMu.RLock()
	defer _redisMu.RUnlock()
	return _redisQueueProducer
}

// GetLockerClient returns the locker client with a read lock.
func GetLockerClient() *redis.Client {
	_redisMu.RLock()
	defer _redisMu.RUnlock()
	return _redisLocker
}

// GetSubscriberClient returns Client #3 (subscriber) with a read lock.
func GetSubscriberClient() *redis.Client {
	_redisMu.RLock()
	defer _redisMu.RUnlock()
	return _redisSub
}

// SetRedisClient sets Client #1 (for external injection).
// If the new client differs from the existing one, the previous client is
// closed (connection pool released) after the lock is released, so the close
// does not block concurrent readers. This uses Close() — not Shutdown() —
// because Shutdown() sends the Redis SERVER a SHUTDOWN command, killing the
// entire Redis process, whereas Close() only releases the local connection pool.
func SetRedisClient(c *redis.Client) {
	_redisMu.Lock()
	old := _redis
	_redis = c
	_redisMu.Unlock()
	if old != nil && old != c {
		_ = old.Close()
	}
}

// EnsureRedisClient returns the cache client, creating it if necessary. MasterName
// non-empty => Sentinel FailoverClient; otherwise standalone. Only cache wires this.
func EnsureRedisClient(rc RedisConnectOptions) (*redis.Client, error) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redis != nil {
		return _redis, nil
	}
	client, err := rc.newClient()
	if err != nil {
		return nil, fmt.Errorf("redis connect: %w", err)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis connect failed: %w", err)
	}
	_redis = client
	return client, nil
}

// EnsureQueueConsumerClient returns the queue consumer client, creating it if
// necessary. Idempotent on its own global slot — no cross-consumer conflict check.
func EnsureQueueConsumerClient(rc RedisConnectOptions) (*redis.Client, error) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redisQueue != nil {
		return _redisQueue, nil
	}
	client, err := rc.newClient()
	if err != nil {
		return nil, fmt.Errorf("redis queue connect: %w", err)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis queue connect failed: %w", err)
	}
	_redisQueue = client
	return client, nil
}

// EnsureSubscriberClient returns the subscriber client, creating it if necessary.
// Best-effort Ping: newClient()/NewFailoverClient() are lazy (only TLS/config
// errors fail); even if the initial Ping fails, the client is stored so
// GetSubscriberClient() is always non-nil and casbin's PSubscribe can self-heal
// via go-redis reconnect + mux backoff.
func EnsureSubscriberClient(rc RedisConnectOptions) (*redis.Client, error) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redisSub != nil {
		return _redisSub, nil
	}
	client, err := rc.newClient() // 惰性，仅 TLS 配置错才失败
	if err != nil {
		return nil, fmt.Errorf("redis subscriber: %w", err)
	}
	// best-effort Ping：短超时 + 失败不 Close、不返回 error，照样存。此处持全局写锁，
	// 故用 1s 超时（而非 context.Background）——否则死端口会触发 MaxRetries 重试，
	// 阻塞写锁 ~1.7s 并卡住所有 Get*/Ensure*（健康检查的 3s 是周期性、非持锁，故不同）。
	// NewClient 惰性，首命令才连网，失败靠 go-redis 重连 + mux 退避自愈。
	pingCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := client.Ping(pingCtx).Err(); err != nil {
		fmt.Printf("EnsureSubscriberClient: initial ping failed (casbin sub self-heals on use): %v\n", err)
	}
	cancel()
	_redisSub = client
	return client, nil
}

// EnsureQueueProducerClient returns the queue producer client (separate pool from
// the consumer so non-blocking XADD isn't starved by blocking XREADGROUP).
func EnsureQueueProducerClient(rc RedisConnectOptions) (*redis.Client, error) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redisQueueProducer != nil {
		return _redisQueueProducer, nil
	}
	client, err := rc.newClient()
	if err != nil {
		return nil, fmt.Errorf("redis queue producer connect: %w", err)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis queue producer connect failed: %w", err)
	}
	_redisQueueProducer = client
	return client, nil
}

// EnsureLockerClient returns the locker client (independent pool from cache).
func EnsureLockerClient(rc RedisConnectOptions) (*redis.Client, error) {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if _redisLocker != nil {
		return _redisLocker, nil
	}
	client, err := rc.newClient()
	if err != nil {
		return nil, fmt.Errorf("redis locker connect: %w", err)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis locker connect failed: %w", err)
	}
	_redisLocker = client
	return client, nil
}

// CloseAllRedisClients closes all five Redis clients and resets them to nil.
// This is the primary teardown path for all clients. SetRedisClient also
// closes a replaced cache client to prevent connection pool leaks.
func CloseAllRedisClients() {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	for _, c := range []*redis.Client{_redis, _redisQueue, _redisQueueProducer, _redisLocker, _redisSub} {
		if c != nil {
			c.Close()
		}
	}
	_redis = nil
	_redisQueue = nil
	_redisQueueProducer = nil
	_redisLocker = nil
	_redisSub = nil
}

// ResetRedisClientsForTest clears all global Redis clients WITHOUT closing
// them. For use in tests only — allows resetting global state between test
// cases without triggering actual network operations on mock clients.
func ResetRedisClientsForTest() {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	_redis = nil
	_redisQueue = nil
	_redisQueueProducer = nil
	_redisLocker = nil
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

	// Sentinel（可选）。MasterName 非空 => NewFailoverClient；空 => 保持单点行为。
	MasterName       string   `mapstructure:"master_name" json:"master_name"`
	SentinelAddrs    []string `mapstructure:"sentinel_addrs" json:"sentinel_addrs"`
	SentinelPassword string   `mapstructure:"sentinel_password" json:"sentinel_password"`
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

// failoverOptions 构造 Sentinel 模式的 *redis.FailoverOptions，逐字段带过
// 认证/连接池/重试配置。FailoverOptions（go-redis sentinel.go）无 Network 字段，
// 连接走 SentinelAddrs，默认 tcp，故不设置。
func (e RedisConnectOptions) failoverOptions() (*redis.FailoverOptions, error) {
	tlsCfg, err := getTLS(e.Tls)
	if err != nil {
		return nil, err
	}
	return &redis.FailoverOptions{
		MasterName:       e.MasterName,
		SentinelAddrs:    e.SentinelAddrs,
		Username:         e.Username,
		Password:         e.Password,
		SentinelPassword: e.SentinelPassword,
		DB:               e.DB,
		MaxRetries:       e.MaxRetries,
		PoolSize:         e.PoolSize, // 0 => go-redis 内部默认值，与单点一致
		TLSConfig:        tlsCfg,
	}, nil
}

// newClient 构造 *redis.Client：MasterName 非空 => NewFailoverClient（Sentinel），
// 否则 => NewClient（单点）。两者都返回 *redis.Client，调用方与适配器零感知。
func (e RedisConnectOptions) newClient() (*redis.Client, error) {
	if e.MasterName != "" {
		if len(e.SentinelAddrs) == 0 {
			return nil, fmt.Errorf("redis sentinel: master_name %q set but sentinel_addrs is empty", e.MasterName)
		}
		fo, err := e.failoverOptions()
		if err != nil {
			return nil, err
		}
		return redis.NewFailoverClient(fo), nil
	}
	opts, err := e.GetRedisOptions() // 复用现有单点构造，零分叉
	if err != nil {
		return nil, err
	}
	return redis.NewClient(opts), nil
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

// --------------------------------------------------------------------------
// Redis Health Check
// --------------------------------------------------------------------------

// RedisHealth tracks the health status of the Redis connection.
type RedisHealth struct {
	mu        sync.RWMutex
	healthy   bool
	lastCheck time.Time
	lastErr   error
}

var redisHealth = &RedisHealth{healthy: true} // assume healthy until first check

var healthCancel context.CancelFunc

func (h *RedisHealth) setHealth(healthy bool, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.healthy = healthy
	h.lastCheck = time.Now()
	h.lastErr = err
}

// StartRedisHealthCheck starts a background goroutine that periodically pings
// the Redis server and updates the health status. It stops when ctx is cancelled
// or StopRedisHealthCheck is called.
// Calling StartRedisHealthCheck twice cancels the previous checker first.
func StartRedisHealthCheck(ctx context.Context, interval time.Duration) {
	if interval == 0 {
		interval = 10 * time.Second
	}
	_redisMu.Lock()
	if healthCancel != nil {
		healthCancel() // cancel previous checker to prevent goroutine leak
	}
	ctx, healthCancel = context.WithCancel(ctx)
	_redisMu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				client := GetRedisClient()
				if client == nil {
					redisHealth.setHealth(false, fmt.Errorf("redis client is nil"))
					continue
				}
				pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				err := client.Ping(pingCtx).Err()
				cancel()
				redisHealth.setHealth(err == nil, err)
			}
		}
	}()
}

// StopRedisHealthCheck stops the background health checker goroutine.
func StopRedisHealthCheck() {
	_redisMu.Lock()
	defer _redisMu.Unlock()
	if healthCancel != nil {
		healthCancel()
		healthCancel = nil
	}
}

// IsRedisHealthy returns whether the Redis connection is healthy based on the
// most recent health check.
func IsRedisHealthy() bool {
	redisHealth.mu.RLock()
	defer redisHealth.mu.RUnlock()
	return redisHealth.healthy
}
