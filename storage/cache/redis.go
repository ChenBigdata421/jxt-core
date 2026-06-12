package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis cache implement
type Redis struct {
	client *redis.Client
}

func NewRedis(client *redis.Client, options *redis.Options) (*Redis, error) {
	if client == nil {
		client = redis.NewClient(options)
	}
	r := &Redis{client: client}
	if err := r.connect(); err != nil {
		return nil, err
	}
	return r, nil
}

func (*Redis) String() string { return "redis" }

func (r *Redis) connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return r.client.Ping(ctx).Err()
}

func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *Redis) Set(ctx context.Context, key string, val interface{}, expire int) error {
	return r.client.Set(ctx, key, val, time.Duration(expire)*time.Second).Err()
}

func (r *Redis) Del(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *Redis) HashGet(ctx context.Context, hk, key string) (string, error) {
	return r.client.HGet(ctx, hk, key).Result()
}

func (r *Redis) HashDel(ctx context.Context, hk, key string) error {
	return r.client.HDel(ctx, hk, key).Err()
}

func (r *Redis) Increase(ctx context.Context, key string) error {
	return r.client.Incr(ctx, key).Err()
}

func (r *Redis) Decrease(ctx context.Context, key string) error {
	return r.client.Decr(ctx, key).Err()
}

func (r *Redis) Expire(ctx context.Context, key string, dur time.Duration) error {
	return r.client.Expire(ctx, key, dur).Err()
}

// New methods

func (r *Redis) HashSet(ctx context.Context, hk, key string, val interface{}) error {
	return r.client.HSet(ctx, hk, key, val).Err()
}

func (r *Redis) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Redis) SetNX(ctx context.Context, key string, val interface{}, expire int) (bool, error) {
	return r.client.SetNX(ctx, key, val, time.Duration(expire)*time.Second).Result()
}

func (r *Redis) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return r.client.IncrBy(ctx, key, n).Result()
}

func (r *Redis) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, key).Result()
}

func (r *Redis) RunScript(ctx context.Context, script interface{}, keys []string, args ...interface{}) (interface{}, error) {
	s, ok := script.(*redis.Script)
	if !ok {
		return nil, redis.Nil
	}
	return s.Run(ctx, r.client, keys, args...).Result()
}

// GetClient exposes the underlying redis client
func (r *Redis) GetClient() *redis.Client {
	return r.client
}

// Close releases the redis connection
func (r *Redis) Close() error {
	return r.client.Close()
}
