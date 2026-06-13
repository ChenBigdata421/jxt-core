package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedis spins up a miniredis instance and returns a connected *Redis
// cache plus the miniredis handle (for error injection) and a cleanup func.
func newTestRedis(t *testing.T) (r *Redis, mr *miniredis.Miniredis, cleanup func()) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
		// Cap dial retries so the dead-connection error-wrapping test fails
		// fast instead of waiting through go-redis's default backoff chain.
		MaxRetries: -1,
	})
	r, err = NewRedis(client, nil)
	require.NoError(t, err)

	cleanup = func() {
		_ = r.Close()
		mr.Close()
	}
	return r, mr, cleanup
}

// ---------------------------------------------------------------------------
// A. Error wrapping (commit 3fa81f0)
// ---------------------------------------------------------------------------

func TestRedis_ErrorWrapping(t *testing.T) {
	// Build a healthy cache first, then kill the server to force errors.
	r, mr, cleanup := newTestRedis(t)
	defer cleanup()

	const key = "mykey"
	ctx := context.Background()

	// Pre-seed a value so that read-path failures are caused by the closed
	// connection rather than a missing key.
	require.NoError(t, r.Set(ctx, key, "v", 60))

	// Tear down the underlying server -> all subsequent commands fail.
	mr.Close()

	script := redis.NewScript("return 1")

	tests := []struct {
		name     string
		opTag    string // substring of the operation tag in the wrapped error
		call     func() error
	}{
		{
			name:  "Get",
			opTag: "cache.Get",
			call: func() error {
				_, err := r.Get(ctx, key)
				return err
			},
		},
		{
			name:  "Set",
			opTag: "cache.Set",
			call: func() error {
				return r.Set(ctx, key, "v", 60)
			},
		},
		{
			name:  "IncrBy",
			opTag: "cache.IncrBy",
			call: func() error {
				_, err := r.IncrBy(ctx, key, 1)
				return err
			},
		},
		{
			name:  "RunScript",
			opTag: "cache.RunScript",
			call: func() error {
				_, err := r.RunScript(ctx, script, []string{key})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.Error(t, err, "%s should return error on dead connection", tt.name)

			msg := err.Error()
			assert.Contains(t, msg, tt.opTag, "error should contain the operation tag")
			assert.Contains(t, msg, key, "error should contain the key")
		})
	}
}

// ---------------------------------------------------------------------------
// B. New context-aware methods (commit 0fbc5b1) - happy path
// ---------------------------------------------------------------------------

func TestRedis_NewMethods_HashSetRoundTrip(t *testing.T) {
	r, _, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, r.HashSet(ctx, "hk", "field", "value"))

	got, err := r.HashGet(ctx, "hk", "field")
	require.NoError(t, err)
	assert.Equal(t, "value", got)
}

func TestRedis_NewMethods_Exists(t *testing.T) {
	r, _, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	ok, err := r.Exists(ctx, "k")
	require.NoError(t, err)
	assert.False(t, ok, "Exists should be false before Set")

	require.NoError(t, r.Set(ctx, "k", "v", 60))

	ok, err = r.Exists(ctx, "k")
	require.NoError(t, err)
	assert.True(t, ok, "Exists should be true after Set")
}

func TestRedis_NewMethods_SetNX(t *testing.T) {
	r, _, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	ok, err := r.SetNX(ctx, "k", "v1", 60)
	require.NoError(t, err)
	assert.True(t, ok, "first SetNX should succeed")

	ok, err = r.SetNX(ctx, "k", "v2", 60)
	require.NoError(t, err)
	assert.False(t, ok, "second SetNX on existing key should fail")

	got, err := r.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v1", got, "original value must be retained")
}

func TestRedis_NewMethods_IncrBy(t *testing.T) {
	r, _, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	v, err := r.IncrBy(ctx, "counter", 5)
	require.NoError(t, err)
	assert.Equal(t, int64(5), v)

	v, err = r.IncrBy(ctx, "counter", 3)
	require.NoError(t, err)
	assert.Equal(t, int64(8), v)

	v, err = r.IncrBy(ctx, "counter", -2)
	require.NoError(t, err)
	assert.Equal(t, int64(6), v)
}

func TestRedis_NewMethods_TTL(t *testing.T) {
	r, _, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, r.Set(ctx, "k", "v", 60))

	dur, err := r.TTL(ctx, "k")
	require.NoError(t, err)
	assert.True(t, dur > 0 && dur <= 60*time.Second, "TTL should be positive and <= 60s, got %v", dur)
}

func TestRedis_NewMethods_RunScript(t *testing.T) {
	r, _, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	// Simple INCR via Lua.
	script := redis.NewScript(`return redis.call("INCR", KEYS[1])`)

	res, err := r.RunScript(ctx, script, []string{"scrkey"})
	require.NoError(t, err)

	got, ok := res.(int64)
	require.True(t, ok, "RunScript result type = %T, want int64", res)
	assert.Equal(t, int64(1), got)
}

// ---------------------------------------------------------------------------
// C. RunScript type guard
// ---------------------------------------------------------------------------

func TestRedis_RunScript_BadType(t *testing.T) {
	r, _, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	// Pass a non-*redis.Script value.
	_, err := r.RunScript(ctx, "not-a-script", []string{"k"})
	require.Error(t, err)

	assert.Contains(t, strings.ToLower(err.Error()), "expected *redis.script",
		"error should describe the expected *redis.Script type; got %q", err.Error())
}
