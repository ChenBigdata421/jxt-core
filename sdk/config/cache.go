package config

import (
	"fmt"

	"github.com/ChenBigdata421/jxt-core/storage"
	"github.com/ChenBigdata421/jxt-core/storage/cache"
)

type Cache struct {
	Redis  *RedisConnectOptions
	Memory interface{}
}

// Redis Redis配置
type Redis struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

var CacheConfig = new(Cache)

// Setup 构造cache 顺序 redis > 其他 > memory。cache 同处 best-effort 创建
// subscriber (_redisSub)，修复 casbin 跨实例 sub 历来 nil 的隐患：EnsureSubscriberClient
// 用惰性 newClient()、即便初始 Ping 不通也存 client，GetSubscriberClient() 恒非 nil，
// casbin PSubscribe 由 go-redis 自愈 + mux 退避重订阅接管；不影响 cache.Setup 成功。
func (e Cache) Setup() (storage.AdapterCache, error) {
	if e.Redis != nil {
		rc := *e.Redis
		client, err := EnsureRedisClient(rc) // db0 → _redis
		if err != nil {
			return nil, err
		}
		// subscriber: best-effort、非致命。EnsureSubscriberClient 用惰性 newClient()，
		// 即便初始 Ping 不通也存 _redisSub（不 Close/不 error），GetSubscriberClient()
		// 恒非 nil，casbin PSubscribe 由 go-redis 自愈 + mux 退避重订阅接管。
		// 仅 newClient() 构造错（如 TLS）才进 err 分支，casbin 降级（等同今日）。
		if _, err := EnsureSubscriberClient(rc); err != nil {
			fmt.Printf("cache.Setup: subscriber construct failed (casbin cross-instance sync disabled): %v\n", err)
		}
		options, err := rc.GetRedisOptions() // 仅 cache.NewRedis 的 nil-client fallback 用
		if err != nil {
			return nil, err
		}
		return cache.NewRedis(client, options)
	}
	return cache.NewMemory(), nil
}
