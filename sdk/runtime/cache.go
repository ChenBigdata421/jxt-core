package runtime

import (
	"context"
	"time"

	"github.com/ChenBigdata421/jxt-core/storage"
)

const (
	intervalTenant = ""
)

func NewCache(prefix string, store storage.AdapterCache) storage.AdapterCache {
	return &Cache{
		prefix: prefix,
		store:  store,
	}
}

type Cache struct {
	prefix string
	store  storage.AdapterCache
}

func (e *Cache) String() string {
	if e.store == nil {
		return ""
	}
	return e.store.String()
}

func (e *Cache) SetPrefix(prefix string) {
	e.prefix = prefix
}

func (e Cache) Connect() error {
	return nil
}

func (e Cache) Get(ctx context.Context, key string) (string, error) {
	return e.store.Get(ctx, e.prefix+intervalTenant+key)
}

func (e Cache) Set(ctx context.Context, key string, val interface{}, expire int) error {
	return e.store.Set(ctx, e.prefix+intervalTenant+key, val, expire)
}

func (e Cache) Del(ctx context.Context, key string) error {
	return e.store.Del(ctx, e.prefix+intervalTenant+key)
}

func (e Cache) HashGet(ctx context.Context, hk, key string) (string, error) {
	return e.store.HashGet(ctx, hk, e.prefix+intervalTenant+key)
}

func (e Cache) HashDel(ctx context.Context, hk, key string) error {
	return e.store.HashDel(ctx, hk, e.prefix+intervalTenant+key)
}

func (e Cache) Increase(ctx context.Context, key string) error {
	return e.store.Increase(ctx, e.prefix+intervalTenant+key)
}

func (e Cache) Decrease(ctx context.Context, key string) error {
	return e.store.Decrease(ctx, e.prefix+intervalTenant+key)
}

func (e Cache) Expire(ctx context.Context, key string, dur time.Duration) error {
	return e.store.Expire(ctx, e.prefix+intervalTenant+key, dur)
}

func (e Cache) HashSet(ctx context.Context, hk, key string, val interface{}) error {
	return e.store.HashSet(ctx, hk, e.prefix+intervalTenant+key, val)
}

func (e Cache) Exists(ctx context.Context, key string) (bool, error) {
	return e.store.Exists(ctx, e.prefix+intervalTenant+key)
}

func (e Cache) SetNX(ctx context.Context, key string, val interface{}, expire int) (bool, error) {
	return e.store.SetNX(ctx, e.prefix+intervalTenant+key, val, expire)
}

func (e Cache) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return e.store.IncrBy(ctx, e.prefix+intervalTenant+key, n)
}

func (e Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	return e.store.TTL(ctx, e.prefix+intervalTenant+key)
}

func (e Cache) RunScript(ctx context.Context, script interface{}, keys []string, args ...interface{}) (interface{}, error) {
	// Prepend prefix to the first key (Lua scripts typically operate on one key)
	if e.prefix != "" && len(keys) > 0 {
		keys[0] = e.prefix + intervalTenant + keys[0]
	}
	return e.store.RunScript(ctx, script, keys, args...)
}

func (e Cache) Close() error {
	if c, ok := e.store.(storage.Closer); ok {
		return c.Close()
	}
	return nil
}
