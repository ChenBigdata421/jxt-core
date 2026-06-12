package storage

import (
	"context"
	"time"

	"github.com/bsm/redislock"
)

const (
	PrefixKey = "__host"
)

type AdapterCache interface {
	String() string

	// Core operations (context-aware)
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, val interface{}, expire int) error
	Del(ctx context.Context, key string) error
	HashGet(ctx context.Context, hk, key string) (string, error)
	HashDel(ctx context.Context, hk, key string) error
	Increase(ctx context.Context, key string) error
	Decrease(ctx context.Context, key string) error
	Expire(ctx context.Context, key string, dur time.Duration) error

	// New methods
	HashSet(ctx context.Context, hk, key string, val interface{}) error
	Exists(ctx context.Context, key string) (bool, error)
	SetNX(ctx context.Context, key string, val interface{}, expire int) (bool, error)
	IncrBy(ctx context.Context, key string, n int64) (int64, error)
	TTL(ctx context.Context, key string) (time.Duration, error)

	// Script execution (enables atomic multi-key operations like IncrBy+Expire)
	RunScript(ctx context.Context, script interface{}, keys []string, args ...interface{}) (interface{}, error)
}

// Closer is the lifecycle interface for cache adapters that hold resources.
type Closer interface {
	Close() error
}

type AdapterQueue interface {
	String() string
	Append(message Messager) error
	Register(name string, f ConsumerFunc)
	Run()
	Shutdown()
}

type Messager interface {
	SetID(string)
	SetStream(string)
	SetValues(map[string]interface{})
	GetID() string
	GetStream() string
	GetValues() map[string]interface{}
	GetPrefix() string
	SetPrefix(string)
	SetErrorCount(count int)
	GetErrorCount() int
}

type ConsumerFunc func(Messager) error

type AdapterLocker interface {
	String() string
	Lock(key string, ttl int64, options *redislock.Options) (*redislock.Lock, error)
}
