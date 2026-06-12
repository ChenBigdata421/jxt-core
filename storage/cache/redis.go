package cache

import (
	"context"
	"fmt"
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
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("cache.Get(%q): %w", key, err)
	}
	return val, nil
}

func (r *Redis) Set(ctx context.Context, key string, val interface{}, expire int) error {
	if err := r.client.Set(ctx, key, val, time.Duration(expire)*time.Second).Err(); err != nil {
		return fmt.Errorf("cache.Set(%q): %w", key, err)
	}
	return nil
}

func (r *Redis) Del(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache.Del(%q): %w", key, err)
	}
	return nil
}

func (r *Redis) HashGet(ctx context.Context, hk, key string) (string, error) {
	val, err := r.client.HGet(ctx, hk, key).Result()
	if err != nil {
		return "", fmt.Errorf("cache.HashGet(%q, %q): %w", hk, key, err)
	}
	return val, nil
}

func (r *Redis) HashDel(ctx context.Context, hk, key string) error {
	if err := r.client.HDel(ctx, hk, key).Err(); err != nil {
		return fmt.Errorf("cache.HashDel(%q, %q): %w", hk, key, err)
	}
	return nil
}

func (r *Redis) Increase(ctx context.Context, key string) error {
	if err := r.client.Incr(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache.Increase(%q): %w", key, err)
	}
	return nil
}

func (r *Redis) Decrease(ctx context.Context, key string) error {
	if err := r.client.Decr(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache.Decrease(%q): %w", key, err)
	}
	return nil
}

func (r *Redis) Expire(ctx context.Context, key string, dur time.Duration) error {
	if err := r.client.Expire(ctx, key, dur).Err(); err != nil {
		return fmt.Errorf("cache.Expire(%q): %w", key, err)
	}
	return nil
}

// New methods

func (r *Redis) HashSet(ctx context.Context, hk, key string, val interface{}) error {
	if err := r.client.HSet(ctx, hk, key, val).Err(); err != nil {
		return fmt.Errorf("cache.HashSet(%q, %q): %w", hk, key, err)
	}
	return nil
}

func (r *Redis) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("cache.Exists(%q): %w", key, err)
	}
	return n > 0, nil
}

func (r *Redis) SetNX(ctx context.Context, key string, val interface{}, expire int) (bool, error) {
	ok, err := r.client.SetNX(ctx, key, val, time.Duration(expire)*time.Second).Result()
	if err != nil {
		return false, fmt.Errorf("cache.SetNX(%q): %w", key, err)
	}
	return ok, nil
}

func (r *Redis) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	val, err := r.client.IncrBy(ctx, key, n).Result()
	if err != nil {
		return 0, fmt.Errorf("cache.IncrBy(%q, %d): %w", key, n, err)
	}
	return val, nil
}

func (r *Redis) TTL(ctx context.Context, key string) (time.Duration, error) {
	dur, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("cache.TTL(%q): %w", key, err)
	}
	return dur, nil
}

func (r *Redis) RunScript(ctx context.Context, script interface{}, keys []string, args ...interface{}) (interface{}, error) {
	s, ok := script.(*redis.Script)
	if !ok {
		return nil, fmt.Errorf("cache.RunScript: expected *redis.Script, got %T", script)
	}
	result, err := s.Run(ctx, r.client, keys, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("cache.RunScript(%v): %w", keys, err)
	}
	return result, nil
}

// GetClient exposes the underlying redis client
func (r *Redis) GetClient() *redis.Client {
	return r.client
}

// Close releases the redis connection
func (r *Redis) Close() error {
	return r.client.Close()
}
