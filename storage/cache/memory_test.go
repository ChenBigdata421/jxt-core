package cache

import (
	"context"
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
		{"test02", fields{}, args{key: "test", value: "test1", expire: 1}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMemory()
			ctx := context.Background()
			if err := m.Set(ctx, tt.args.key, tt.args.value, tt.args.expire); err != nil {
				t.Errorf("Set() error = %v", err)
				return
			}
			time.Sleep(2 * time.Second)
			got, err := m.Get(ctx, tt.args.key)
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
	ctx := context.Background()
	if err := m.Set(ctx, "counter", "0", 60); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := m.Increase(ctx, "counter"); err != nil {
				t.Errorf("Increase() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := m.Get(ctx, "counter")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != strconv.Itoa(goroutines) {
		t.Errorf("concurrent increase: got %s, want %d", got, goroutines)
	}
}

func TestMemory_ConcurrentExpire(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if err := m.Set(ctx, "key", "value", 60); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := m.Expire(ctx, "key", 60*time.Second); err != nil {
				t.Errorf("Expire() error = %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := m.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "value" {
		t.Errorf("concurrent expire: got %q, want %q", got, "value")
	}
}

// --------------------------------------------------------------------------
// Tests for new methods (HashSet, Exists, SetNX, IncrBy, TTL, RunScript)
// --------------------------------------------------------------------------

func TestMemory_HashSet(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if err := m.HashSet(ctx, "hk", "field", "value"); err != nil {
		t.Fatalf("HashSet() error = %v", err)
	}
	got, err := m.HashGet(ctx, "hk", "field")
	if err != nil {
		t.Fatalf("HashGet() error = %v", err)
	}
	if got != "value" {
		t.Errorf("HashGet() got %q, want %q", got, "value")
	}
}

func TestMemory_Exists(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// Non-existent key returns false.
	ok, err := m.Exists(ctx, "nokey")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if ok {
		t.Error("Exists() should return false for missing key")
	}

	// Existing key returns true.
	if err := m.Set(ctx, "key", "val", 60); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	ok, err = m.Exists(ctx, "key")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !ok {
		t.Error("Exists() should return true for existing key")
	}
}

func TestMemory_SetNX(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// First SetNX succeeds.
	ok, err := m.SetNX(ctx, "key", "val", 60)
	if err != nil {
		t.Fatalf("SetNX() error = %v", err)
	}
	if !ok {
		t.Error("first SetNX should return true")
	}

	// Second SetNX on same key returns false.
	ok, err = m.SetNX(ctx, "key", "val2", 60)
	if err != nil {
		t.Fatalf("SetNX() error = %v", err)
	}
	if ok {
		t.Error("second SetNX should return false")
	}

	// Value should still be the original.
	got, err := m.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "val" {
		t.Errorf("Get() got %q, want %q", got, "val")
	}
}

func TestMemory_IncrBy(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// IncrBy on missing key starts from 0.
	val, err := m.IncrBy(ctx, "counter", 5)
	if err != nil {
		t.Fatalf("IncrBy() error = %v", err)
	}
	if val != 5 {
		t.Errorf("IncrBy() got %d, want 5", val)
	}

	// Increment existing key.
	val, err = m.IncrBy(ctx, "counter", 3)
	if err != nil {
		t.Fatalf("IncrBy() error = %v", err)
	}
	if val != 8 {
		t.Errorf("IncrBy() got %d, want 8", val)
	}

	// Negative increment.
	val, err = m.IncrBy(ctx, "counter", -2)
	if err != nil {
		t.Fatalf("IncrBy() error = %v", err)
	}
	if val != 6 {
		t.Errorf("IncrBy() got %d, want 6", val)
	}
}

func TestMemory_TTL(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// Missing key returns 0 TTL.
	dur, err := m.TTL(ctx, "nokey")
	if err != nil {
		t.Fatalf("TTL() error = %v", err)
	}
	if dur != 0 {
		t.Errorf("TTL() got %v, want 0 for missing key", dur)
	}

	// Existing key has positive TTL.
	if err := m.Set(ctx, "key", "val", 60); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	dur, err = m.TTL(ctx, "key")
	if err != nil {
		t.Fatalf("TTL() error = %v", err)
	}
	if dur <= 0 || dur > 60*time.Second {
		t.Errorf("TTL() got %v, want between 0 and 60s", dur)
	}
}

func TestMemory_RunScript_Supported(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// Supported pattern: IncrBy + Expire via args.
	res, err := m.RunScript(ctx, "dummy", []string{"counter"}, int64(10), int64(60))
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	got, ok := res.(int64)
	if !ok {
		t.Fatalf("RunScript() result type = %T, want int64", res)
	}
	if got != 10 {
		t.Errorf("RunScript() got %d, want 10", got)
	}
}

func TestMemory_RunScript_Unsupported(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// Empty keys or wrong arg types should return error.
	_, err := m.RunScript(ctx, "bad", []string{}, "not-int64")
	if err == nil {
		t.Error("RunScript() should return error for unsupported pattern")
	}
}

// TestMemory_ConcurrentReadWrite verifies that concurrent readers and writers
// on the same key do not cause data races (detectable with -race).
func TestMemory_ConcurrentReadWrite(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if err := m.Set(ctx, "key", "0", 60); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	const writers = 50
	const readers = 50
	var wg sync.WaitGroup
	wg.Add(writers + readers)

	// Writers: IncrBy
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			_, _ = m.IncrBy(ctx, "key", 1)
		}()
	}

	// Readers: Get, TTL, Exists
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			_, _ = m.Get(ctx, "key")
			_, _ = m.TTL(ctx, "key")
			_, _ = m.Exists(ctx, "key")
		}()
	}

	wg.Wait()
}
