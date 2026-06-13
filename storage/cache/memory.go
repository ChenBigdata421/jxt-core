package cache

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/cast"
)

type item struct {
	Value   string
	Expired time.Time
}

// NewMemory memory模式
func NewMemory() *Memory {
	return &Memory{
		items: new(sync.Map),
	}
}

type Memory struct {
	items *sync.Map
	mutex sync.RWMutex
}

func (*Memory) String() string {
	return "memory"
}

func (m *Memory) connect() {
}

func (m *Memory) Get(_ context.Context, key string) (string, error) {
	item, err := m.getItem(key)
	if err != nil || item == nil {
		return "", err
	}
	return item.Value, nil
}

func (m *Memory) getItem(key string) (*item, error) {
	var err error
	i, ok := m.items.Load(key)
	if !ok {
		return nil, nil
	}
	switch i.(type) {
	case *item:
		item := i.(*item)
		if item.Expired.Before(time.Now()) {
			//过期
			_ = m.del(key)
			//过期后删除
			return nil, nil
		}
		return item, nil
	default:
		err = fmt.Errorf("value of %s type error", key)
		return nil, err
	}
}

func (m *Memory) Set(_ context.Context, key string, val interface{}, expire int) error {
	s, err := cast.ToStringE(val)
	if err != nil {
		return err
	}
	item := &item{
		Value:   s,
		Expired: time.Now().Add(time.Duration(expire) * time.Second),
	}
	return m.setItem(key, item)
}

func (m *Memory) setItem(key string, item *item) error {
	m.items.Store(key, item)
	return nil
}

func (m *Memory) Del(_ context.Context, key string) error {
	return m.del(key)
}

func (m *Memory) del(key string) error {
	m.items.Delete(key)
	return nil
}

func (m *Memory) HashGet(_ context.Context, hk, key string) (string, error) {
	item, err := m.getItem(hk + key)
	if err != nil || item == nil {
		return "", err
	}
	return item.Value, err
}

func (m *Memory) HashDel(_ context.Context, hk, key string) error {
	return m.del(hk + key)
}

func (m *Memory) Increase(_ context.Context, key string) error {
	return m.calculate(key, 1)
}

func (m *Memory) Decrease(_ context.Context, key string) error {
	return m.calculate(key, -1)
}

func (m *Memory) calculate(key string, num int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	item, err := m.getItem(key)
	if err != nil {
		return err
	}

	if item == nil {
		err = fmt.Errorf("%s not exist", key)
		return err
	}
	var n int
	n, err = cast.ToIntE(item.Value)
	if err != nil {
		return err
	}
	n += num
	item.Value = strconv.Itoa(n)
	return m.setItem(key, item)
}

func (m *Memory) Expire(_ context.Context, key string, dur time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	item, err := m.getItem(key)
	if err != nil {
		return err
	}
	if item == nil {
		err = fmt.Errorf("%s not exist", key)
		return err
	}
	item.Expired = time.Now().Add(dur)
	return m.setItem(key, item)
}

// New methods (memory implementations)

func (m *Memory) HashSet(_ context.Context, hk, key string, val interface{}) error {
	s, err := cast.ToStringE(val)
	if err != nil {
		return err
	}
	return m.setItem(hk+key, &item{Value: s, Expired: time.Now().Add(24 * time.Hour)})
}

func (m *Memory) Exists(_ context.Context, key string) (bool, error) {
	item, err := m.getItem(key)
	if err != nil {
		return false, err
	}
	return item != nil, nil
}

// SetNX atomically sets a value only if the key does not exist.
// The entire check-and-set is protected by a write lock to prevent TOCTOU races.
func (m *Memory) SetNX(_ context.Context, key string, val interface{}, expire int) (bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.items.Load(key); ok {
		return false, nil
	}
	s, err := cast.ToStringE(val)
	if err != nil {
		return false, err
	}
	return true, m.setItem(key, &item{
		Value:   s,
		Expired: time.Now().Add(time.Duration(expire) * time.Second),
	})
}

func (m *Memory) IncrBy(_ context.Context, key string, n int64) (int64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	it, err := m.getItem(key)
	if err != nil {
		return 0, err
	}
	if it == nil {
		it = &item{Value: "0", Expired: time.Now().Add(24 * time.Hour)}
	}
	val, err := cast.ToInt64E(it.Value)
	if err != nil {
		return 0, err
	}
	val += n
	it.Value = strconv.FormatInt(val, 10)
	m.setItem(key, it)
	return val, nil
}

func (m *Memory) TTL(_ context.Context, key string) (time.Duration, error) {
	item, err := m.getItem(key)
	if err != nil || item == nil {
		return 0, err
	}
	return time.Until(item.Expired), nil
}

func (m *Memory) RunScript(_ context.Context, script interface{}, keys []string, args ...interface{}) (interface{}, error) {
	// Memory implementation: simulate atomic IncrBy + Expire pattern
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if len(keys) > 0 && len(args) >= 2 {
		key := keys[0]
		incr, ok1 := args[0].(int64)
		expireSeconds, ok2 := args[1].(int64)
		if ok1 && ok2 {
			it, _ := m.getItem(key)
			if it == nil {
				it = &item{Value: "0", Expired: time.Now().Add(24 * time.Hour)}
			}
			val, _ := cast.ToInt64E(it.Value)
			val += incr
			it.Value = strconv.FormatInt(val, 10)
			it.Expired = time.Now().Add(time.Duration(expireSeconds) * time.Second)
			m.setItem(key, it)
			return val, nil
		}
	}
	return nil, fmt.Errorf("RunScript: unsupported script pattern in memory adapter")
}
