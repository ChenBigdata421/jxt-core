package cache

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestMemory_Get(t *testing.T) {
	type fields struct {
		items   *sync.Map
		queue   *sync.Map
		wait    sync.WaitGroup
		mutex   sync.RWMutex
		PoolNum uint
	}
	type args struct {
		key    string
		value  string
		expire int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{"test01", fields{}, args{key: "test", value: "test", expire: 10}, "test", false},
		{"test02", fields{}, args{key: "test", value: "test1", expire: 1}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMemory()
			if err := m.Set(tt.args.key, tt.args.value, tt.args.expire); err != nil {
				t.Errorf("Set() error = %v", err)
				return
			}
			time.Sleep(2 * time.Second)
			got, err := m.Get(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Get() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemory_ConcurrentIncrease(t *testing.T) {
	m := NewMemory()
	if err := m.Set("counter", "0", 60); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := m.Increase("counter"); err != nil {
				t.Errorf("Increase() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := m.Get("counter")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != strconv.Itoa(goroutines) {
		t.Errorf("concurrent increase: got %s, want %d", got, goroutines)
	}
}

func TestMemory_ConcurrentExpire(t *testing.T) {
	m := NewMemory()
	if err := m.Set("key", "value", 60); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := m.Expire("key", 60*time.Second); err != nil {
				t.Errorf("Expire() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := m.Get("key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "value" {
		t.Errorf("concurrent expire: got %q, want %q", got, "value")
	}
}
