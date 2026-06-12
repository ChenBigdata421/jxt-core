package config

import (
	"context"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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
// TestEnsureRedisClient_ConfigConflictDetected
// --------------------------------------------------------------------------

func TestEnsureRedisClient_ConfigConflictDetected(t *testing.T) {
	mr, opts := setupMiniredis(t)
	defer mr.Close()
	defer ResetRedisClientsForTest()

	// First call creates the client.
	client1, err := EnsureRedisClient(opts)
	if err != nil {
		t.Fatalf("first EnsureRedisClient: %v", err)
	}
	if client1 == nil {
		t.Fatal("expected non-nil client")
	}

	// Matching config returns the same instance.
	client2, err := EnsureRedisClient(opts)
	if err != nil {
		t.Fatalf("matching EnsureRedisClient: %v", err)
	}
	if client2 != client1 {
		t.Fatal("expected same client instance for matching config")
	}

	// Conflicting addr returns an error.
	conflictOpts := &redis.Options{
		Addr: "127.0.0.1:1", // different address
		DB:   opts.DB,
	}
	_, err = EnsureRedisClient(conflictOpts)
	if err == nil {
		t.Fatal("expected error for conflicting addr, got nil")
	}

	// Conflicting DB returns an error.
	conflictDB := &redis.Options{
		Addr: opts.Addr,
		DB:   7, // different DB
	}
	_, err = EnsureRedisClient(conflictDB)
	if err == nil {
		t.Fatal("expected error for conflicting DB, got nil")
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
	_, err := EnsureRedisClient(opts)
	if err != nil {
		t.Fatalf("EnsureRedisClient: %v", err)
	}

	// Create Client #2 — same addr/DB should succeed.
	qc, err := EnsureQueueConsumerClient(opts)
	if err != nil {
		t.Fatalf("EnsureQueueConsumerClient: %v", err)
	}
	if qc == nil {
		t.Fatal("expected non-nil queue consumer client")
	}

	// Second call returns the same instance.
	qc2, err := EnsureQueueConsumerClient(opts)
	if err != nil {
		t.Fatalf("second EnsureQueueConsumerClient: %v", err)
	}
	if qc2 != qc {
		t.Fatal("expected same queue consumer client instance")
	}

	// Conflicting addr against Client #1 should fail.
	conflictOpts := &redis.Options{
		Addr: "127.0.0.1:1",
		DB:   opts.DB,
	}
	// Reset and re-create with a fresh state to test the conflict path.
	ResetRedisClientsForTest()
	_, _ = EnsureRedisClient(opts)
	_, err = EnsureQueueConsumerClient(conflictOpts)
	// This succeeds because the queue client doesn't exist yet, and we
	// validate against Client #1's addr/DB. With Client #1 at mr.Addr(),
	// conflictOpts should fail.
	// Actually let's re-check: after reset, we create Client #1 again, then
	// try a conflicting Client #2.
	ResetRedisClientsForTest()
	_, _ = EnsureRedisClient(opts)
	_, err = EnsureQueueConsumerClient(conflictOpts)
	if err == nil {
		t.Fatal("expected error for queue consumer config conflict")
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
	_, err := EnsureRedisClient(opts)
	if err != nil {
		t.Fatalf("EnsureRedisClient: %v", err)
	}

	// Create Client #3.
	sc, err := EnsureSubscriberClient(opts)
	if err != nil {
		t.Fatalf("EnsureSubscriberClient: %v", err)
	}
	if sc == nil {
		t.Fatal("expected non-nil subscriber client")
	}

	// Second call returns the same instance.
	sc2, err := EnsureSubscriberClient(opts)
	if err != nil {
		t.Fatalf("second EnsureSubscriberClient: %v", err)
	}
	if sc2 != sc {
		t.Fatal("expected same subscriber client instance")
	}

	// Conflicting addr against Client #1 should fail.
	ResetRedisClientsForTest()
	_, _ = EnsureRedisClient(opts)
	conflictOpts := &redis.Options{Addr: "127.0.0.1:1", DB: opts.DB}
	_, err = EnsureSubscriberClient(conflictOpts)
	if err == nil {
		t.Fatal("expected error for subscriber config conflict against Client #1")
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
	_, err := EnsureRedisClient(opts)
	if err != nil {
		t.Fatalf("EnsureRedisClient: %v", err)
	}
	_, err = EnsureQueueConsumerClient(opts)
	if err != nil {
		t.Fatalf("EnsureQueueConsumerClient: %v", err)
	}
	_, err = EnsureSubscriberClient(opts)
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
