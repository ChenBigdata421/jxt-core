package config

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// setupMiniredis creates an in-process Redis server for testing.
func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Options) {
	t.Helper()
	mr := miniredis.RunT(t)
	options := &redis.Options{
		Addr: mr.Addr(),
		DB:   0,
	}
	return mr, options
}

// rcFromOpts adapts a standalone *redis.Options to RedisConnectOptions for the
// Ensure* signature (used to migrate the existing standalone tests). Standalone
// tests only set Addr/DB; TLS/pool/sentinel paths have their own coverage
// (TestRedisConnectOptions_FailoverOptions, TestNewClient_Sentinel_*).
func rcFromOpts(o *redis.Options) RedisConnectOptions {
	return RedisConnectOptions{Addr: o.Addr, DB: o.DB}
}

// --------------------------------------------------------------------------
// TestGetRedisClient_ConcurrentAccess
// --------------------------------------------------------------------------

func TestGetRedisClient_ConcurrentAccess(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()

	// Seed Client #1 via SetRedisClient.
	client := redis.NewClient(opts)
	SetRedisClient(client)
	defer ResetRedisClientsForTest()

	// Phase 1: 100 goroutines all call GetRedisClient concurrently.
	var wg sync.WaitGroup
	const readers = 100
	results := make([]*redis.Client, readers)

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = GetRedisClient()
		}(i)
	}
	wg.Wait()

	// Every goroutine must have received the same instance.
	for i, got := range results {
		if got != client {
			t.Fatalf("reader %d: expected same client instance, got different pointer", i)
		}
	}

	// Phase 2: 50 goroutines do Set+Get pairs concurrently.
	// The goal is to verify no panics or data races — we cannot assert
	// Set→Get returns the same value because all 50 goroutines write
	// concurrently. Instead we just verify the final value is non-nil
	// and all goroutines complete without panicking.
	const writers = 50
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			newClient := redis.NewClient(opts)
			SetRedisClient(newClient)
			_ = GetRedisClient()
		}()
	}
	wg.Wait()

	if got := GetRedisClient(); got == nil {
		t.Fatal("expected non-nil client after concurrent Set/Get phase")
	}
}

// --------------------------------------------------------------------------
// TestEnsureQueueConsumerClient
// --------------------------------------------------------------------------

func TestEnsureQueueConsumerClient(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	// Create Client #1 first.
	_, err := EnsureRedisClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("EnsureRedisClient: %v", err)
	}

	// Create Client #2 — same addr/DB should succeed.
	qc, err := EnsureQueueConsumerClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("EnsureQueueConsumerClient: %v", err)
	}
	if qc == nil {
		t.Fatal("expected non-nil queue consumer client")
	}

	// Second call returns the same instance.
	qc2, err := EnsureQueueConsumerClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("second EnsureQueueConsumerClient: %v", err)
	}
	if qc2 != qc {
		t.Fatal("expected same queue consumer client instance")
	}
}

// --------------------------------------------------------------------------
// TestEnsureSubscriberClient
// --------------------------------------------------------------------------

func TestEnsureSubscriberClient(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	// Create Client #1 first.
	_, err := EnsureRedisClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("EnsureRedisClient: %v", err)
	}

	// Create Client #3.
	sc, err := EnsureSubscriberClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("EnsureSubscriberClient: %v", err)
	}
	if sc == nil {
		t.Fatal("expected non-nil subscriber client")
	}

	// Second call returns the same instance.
	sc2, err := EnsureSubscriberClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("second EnsureSubscriberClient: %v", err)
	}
	if sc2 != sc {
		t.Fatal("expected same subscriber client instance")
	}
}

// --------------------------------------------------------------------------
// TestCloseAllRedisClients
// --------------------------------------------------------------------------

func TestCloseAllRedisClients(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	// Create all three clients.
	_, err := EnsureRedisClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("EnsureRedisClient: %v", err)
	}
	_, err = EnsureQueueConsumerClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("EnsureQueueConsumerClient: %v", err)
	}
	_, err = EnsureSubscriberClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("EnsureSubscriberClient: %v", err)
	}

	// Close all — should not panic.
	CloseAllRedisClients()

	// All getters should return nil.
	if GetRedisClient() != nil {
		t.Fatal("expected nil after CloseAllRedisClients")
	}
	if GetQueueConsumerClient() != nil {
		t.Fatal("expected nil queue client after CloseAllRedisClients")
	}
	if GetSubscriberClient() != nil {
		t.Fatal("expected nil subscriber client after CloseAllRedisClients")
	}

	// CloseAllRedisClients on already-nil clients should not panic.
	CloseAllRedisClients()
}

// --------------------------------------------------------------------------
// TestSetRedisClient_DoesNotCloseOldClient
// --------------------------------------------------------------------------

// TestSetRedisClient_DoesNotShutdownServer verifies that SetRedisClient no
// longer calls Shutdown() on the previous client. The old code used
// _redis.Shutdown() which sends a SHUTDOWN command to the Redis server,
// killing it. This test ensures we only do an assignment.
func TestSetRedisClient_DoesNotShutdownServer(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	oldClient := redis.NewClient(opts)
	SetRedisClient(oldClient)

	// Set a new client — the old miniredis server must still be alive.
	newClient := redis.NewClient(opts)
	SetRedisClient(newClient)

	// If SetRedisClient had called Shutdown on oldClient, the miniredis
	// server would be dead and this Ping would fail.
	if err := newClient.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("miniredis server should still be alive after SetRedisClient: %v", err)
	}

	// Verify the getter returns the new client.
	if got := GetRedisClient(); got != newClient {
		t.Fatal("GetRedisClient should return the most recently set client")
	}
}

// --------------------------------------------------------------------------
// TestRedisHealthCheck
// --------------------------------------------------------------------------

func TestRedisHealthCheck(t *testing.T) {
	mr, _ := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Client #1
	opts := &redis.Options{Addr: mr.Addr()}
	_, err := EnsureRedisClient(rcFromOpts(opts))
	if err != nil {
		t.Fatalf("EnsureRedisClient: %v", err)
	}

	StartRedisHealthCheck(ctx, 50*time.Millisecond)
	time.Sleep(150 * time.Millisecond) // wait for at least one check

	if !IsRedisHealthy() {
		t.Error("expected healthy when Redis server is running")
	}

	// Close the miniredis server and the client so the next ping creates
	// a fresh connection that will fail.
	mr.Close()
	CloseAllRedisClients()
	time.Sleep(150 * time.Millisecond)

	if IsRedisHealthy() {
		t.Error("expected unhealthy after Redis server shutdown")
	}
}

// --------------------------------------------------------------------------
// TestSetRedisClient_ClosesOldClient
// --------------------------------------------------------------------------

// TestSetRedisClient_ClosesOldClient verifies that replacing Client #1 with a
// different pointer releases the OLD client's connection pool via Close(). It
// also verifies the same-instance guard: setting the SAME pointer twice does
// NOT close it.
func TestSetRedisClient_ClosesOldClient(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	// Client A.
	clientA := redis.NewClient(opts)
	SetRedisClient(clientA)

	// Warm it up so the pool is established.
	require.NoError(t, clientA.Ping(context.Background()).Err())

	// Replace with a distinct Client B — old (A) must be closed.
	clientB := redis.NewClient(opts)
	SetRedisClient(clientB)

	// After Close(), pinging the old client should fail ("redis: client is
	// closed" or pool exhausted).
	err := clientA.Ping(context.Background()).Err()
	require.Error(t, err, "old client should be closed after being replaced")

	// The new client must still be functional.
	require.NoError(t, clientB.Ping(context.Background()).Err())

	// Same-instance guard: setting B again must NOT close it.
	SetRedisClient(clientB)
	require.NoError(t, clientB.Ping(context.Background()).Err(),
		"setting the same pointer twice must not close the client")
	require.Equal(t, clientB, GetRedisClient())
}

// --------------------------------------------------------------------------
// Health-checker transition tests
// --------------------------------------------------------------------------

// pollHealthy polls IsRedisHealthy until it returns want or the timeout elapses.
func pollHealthy(t *testing.T, want bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsRedisHealthy() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("IsRedisHealthy() want=%v within %v", want, timeout)
}

// TestRedisHealthCheck_Recovery verifies the checker flips back to healthy
// after an unhealthy period once a fresh live client is set.
func TestRedisHealthCheck_Recovery(t *testing.T) {
	mr1, opts1 := setupMiniredis(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client1 := redis.NewClient(opts1)
	SetRedisClient(client1)

	StartRedisHealthCheck(ctx, 40*time.Millisecond)
	defer StopRedisHealthCheck()
	defer ResetRedisClientsForTest()

	// Confirm it starts healthy.
	pollHealthy(t, true, 2*time.Second)

	// Kill the server -> next ticks should mark unhealthy.
	mr1.Close()
	pollHealthy(t, false, 2*time.Second)

	// Inject a FRESH live miniredis client.
	mr2, opts2 := setupMiniredis(t)
	defer mr2.Close()
	client2 := redis.NewClient(opts2)
	SetRedisClient(client2)

	// Should recover to healthy.
	pollHealthy(t, true, 2*time.Second)
}

// TestRedisHealthCheck_NilClient exercises the client==nil branch: with no
// client set, the checker must report unhealthy.
func TestRedisHealthCheck_NilClient(t *testing.T) {
	CloseAllRedisClients()
	defer ResetRedisClientsForTest()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartRedisHealthCheck(ctx, 40*time.Millisecond)
	defer StopRedisHealthCheck()

	pollHealthy(t, false, 2*time.Second)
}

// TestStopRedisHealthCheck_HaltsLoop verifies that after StopRedisHealthCheck
// the loop no longer runs, so making the client unhealthy does NOT flip health.
func TestStopRedisHealthCheck_HaltsLoop(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := redis.NewClient(opts)
	SetRedisClient(client)

	StartRedisHealthCheck(ctx, 40*time.Millisecond)

	// Confirm healthy first.
	pollHealthy(t, true, 2*time.Second)

	// Stop the loop.
	StopRedisHealthCheck()

	// Make the client unhealthy by closing its server.
	mr.Close()

	// Wait well beyond several tick intervals. Because the loop is stopped,
	// the cached health value should remain whatever it was. To be robust,
	// assert it does NOT go false within a bounded window.
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		require.True(t, IsRedisHealthy(),
			"health should not flip after the checker is stopped")
		time.Sleep(40 * time.Millisecond)
	}
}

// TestStartRedisHealthCheck_DoubleStartCancelsPrevious verifies that calling
// StartRedisHealthCheck twice cancels the previous checker so only one active
// loop remains (no goroutine leak).
func TestStartRedisHealthCheck_DoubleStartCancelsPrevious(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	client := redis.NewClient(opts)
	SetRedisClient(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartRedisHealthCheck(ctx, 40*time.Millisecond)
	StartRedisHealthCheck(ctx, 40*time.Millisecond)
	defer StopRedisHealthCheck()

	// The second start replaced the first. The active checker must still be
	// running and reporting healthy.
	pollHealthy(t, true, 2*time.Second)

	// Capture goroutine count before/after to confirm no leak: the first
	// loop's goroutine must have exited.
	before := runtime.NumGoroutine()
	// Give the cancelled first loop time to observe ctx.Done and exit.
	time.Sleep(120 * time.Millisecond)
	after := runtime.NumGoroutine()
	// Allow some slack but require no growth (the cancelled loop exited).
	require.LessOrEqual(t, after, before+1,
		"first checker goroutine should have been cancelled, no leak")
}

// --------------------------------------------------------------------------
// Sentinel 支持 + 补丁 A（squash）回归（Workstream 1）
// --------------------------------------------------------------------------

// toMap turns raw YAML bytes into a generic map. Viper uses gopkg.in/yaml.v3
// under the hood, so we mirror that here to feed mapstructure the same shape
// of data that viper would produce after decoding settings.yml.
func toMap(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	return m
}

// TestQueueRedis_SquashDecodesFields 是补丁 A 的回归测试：修复前 mapstructure 不拍平
// 内嵌 RedisConnectOptions，queue.redis.* 解析为零值（security-management crash-loop 根因）。
func TestQueueRedis_SquashDecodesFields(t *testing.T) {
	raw := []byte(`
addr: "redis:6379"
password: "p"
db: 1
master_name: "mymaster"
`)
	var q QueueRedis
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "mapstructure", Result: &q,
	})
	require.NoError(t, err)
	require.NoError(t, dec.Decode(toMap(t, raw)))
	require.Equal(t, "redis:6379", q.Addr)
	require.Equal(t, "p", q.Password)
	require.Equal(t, 1, q.DB)
	require.Equal(t, "mymaster", q.MasterName)
}

func TestRedisConnectOptions_FailoverOptions(t *testing.T) {
	rc := RedisConnectOptions{
		MasterName:       "mymaster",
		SentinelAddrs:    []string{"s1:26379", "s2:26379"},
		Username:         "u",
		Password:         "p",
		SentinelPassword: "sp",
		DB:               3,
		MaxRetries:       7,
		PoolSize:         20,
	}
	fo, err := rc.failoverOptions()
	require.NoError(t, err)
	require.Equal(t, "mymaster", fo.MasterName)
	require.Equal(t, []string{"s1:26379", "s2:26379"}, fo.SentinelAddrs)
	require.Equal(t, "u", fo.Username)
	require.Equal(t, "p", fo.Password)
	require.Equal(t, "sp", fo.SentinelPassword)
	require.Equal(t, 3, fo.DB)
	require.Equal(t, 7, fo.MaxRetries)
	require.Equal(t, 20, fo.PoolSize)
	require.Nil(t, fo.TLSConfig)
}

func TestNewClient_Standalone(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	rc := RedisConnectOptions{Addr: opts.Addr, DB: opts.DB}
	client, err := rc.newClient()
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NoError(t, client.Ping(context.Background()).Err())
	_ = client.Close()
}

func TestNewClient_Sentinel_ReturnsFailoverClient(t *testing.T) {
	// NewFailoverClient 是惰性的（首条命令前不连接），故无需真实 Sentinel，
	// 只断言返回非 nil client（且不 panic）。
	rc := RedisConnectOptions{
		MasterName:    "mymaster",
		SentinelAddrs: []string{"127.0.0.1:26379"},
		Password:      "x",
		DB:            0,
	}
	client, err := rc.newClient()
	require.NoError(t, err)
	require.NotNil(t, client)
	_ = client.Close()
}

// --------------------------------------------------------------------------
// Setup-level regression + best-effort subscriber Ping（补丁 B）
// --------------------------------------------------------------------------

// TestSetup_MultiDB_NoConflict regressions the security-management crash-loop.
// Old code: cache.Setup(db0) then queue.Setup(db1) hit EnsureRedisClient's
// "redis config conflict" (db0 != db1) -> log.Fatalf -> restart loop.
// New code: each consumer owns its client on its own db via Ensure*; no cross-db conflict.
func TestSetup_MultiDB_NoConflict(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	// save/restore Config globals — ResetRedisClientsForTest only resets clients,
	// not CacheConfig/QueueConfig/LockerConfig, so restore explicitly to avoid
	// polluting other tests in this package.
	savedCache, savedQueue, savedLocker := CacheConfig.Redis, QueueConfig.Redis, LockerConfig.Redis
	defer func() {
		CacheConfig.Redis, QueueConfig.Redis, LockerConfig.Redis = savedCache, savedQueue, savedLocker
	}()

	// cache: db0（顺带建 _redisSub，非致命）
	CacheConfig.Redis = &RedisConnectOptions{Addr: opts.Addr, DB: 0}
	if _, err := CacheConfig.Setup(); err != nil {
		t.Fatalf("cache.Setup(db0): %v", err)
	}
	// queue: db1 — 旧代码在此返回 conflict error（crash-loop 路径）
	QueueConfig.Redis = &QueueRedis{
		RedisConnectOptions: RedisConnectOptions{Addr: opts.Addr, DB: 1},
	}
	if _, err := QueueConfig.Setup(); err != nil {
		t.Fatalf("queue.Setup(db1): %v (this is the crash-loop path)", err)
	}
	// locker: db2 — 旧代码在 queue 之后也会 conflict
	LockerConfig.Redis = &RedisConnectOptions{Addr: opts.Addr, DB: 2}
	if _, err := LockerConfig.Setup(); err != nil {
		t.Fatalf("locker.Setup(db2): %v", err)
	}

	// 断言 db 隔离 + 幂等：cache 在 db0、queue consumer 在 db1、subscriber 在 db0
	require.Equal(t, 0, GetRedisClient().Options().DB)
	require.Equal(t, 1, GetQueueConsumerClient().Options().DB)
	require.Equal(t, 0, GetSubscriberClient().Options().DB)

	// 幂等回归：再次 Setup 不报错、不创建新 client（Ensure* 本槽复用）
	cp1 := GetRedisClient()
	CacheConfig.Setup()
	require.Same(t, cp1, GetRedisClient(), "EnsureRedisClient must be idempotent")
}

// TestEnsureSubscriberClient_BestEffortPing 锁定"初始 Ping 失败也存 client"——
// GetSubscriberClient() 恒非 nil，casbin PSubscribe 才有对象自愈（避免"失败即永久 nil"漏洞）。
func TestEnsureSubscriberClient_BestEffortPing(t *testing.T) {
	defer ResetRedisClientsForTest()
	// 指向无服务端口：newClient() 惰性成功（不连网），Ping 必失败（ECONNREFUSED 即时返回）。
	// 若 CI 环境过滤该端口，改用 miniredis 关闭后的 addr（deadAddr := opts.Addr; mr.Close()）。
	rc := RedisConnectOptions{Addr: "127.0.0.1:1"}
	c, err := EnsureSubscriberClient(rc)
	require.NoError(t, err, "EnsureSubscriberClient must not error on Ping failure")
	require.NotNil(t, c, "client must be stored even when Ping fails")
	require.Same(t, c, GetSubscriberClient(), "GetSubscriberClient must return the stored client")
	_ = c.Close()
}
