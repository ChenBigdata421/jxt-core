package mycasbin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// --------------------------------------------------------------------------
// TestMain initializes the logger for all tests in this package.
// --------------------------------------------------------------------------

func TestMain(m *testing.M) {
	// Initialize nop logger so watcher_mux.go logging calls don't panic.
	logger.DefaultLogger = zap.NewNop().Sugar()
	logger.Logger = zap.NewNop()

	m.Run()
}

// --------------------------------------------------------------------------
// Test helpers for watcher mux tests
// --------------------------------------------------------------------------

// resetMuxState clears the global mux singleton so tests start clean.
func resetMuxState() {
	ShutdownCasbinWatcherMux()
	watcherMux.Store(nil)
	watcherMuxOnce = sync.Once{} // fresh Once so InitCasbinWatcherMux can run again
	getEnforcerFn = nil
}

// setupMuxTest creates a miniredis server, pub/sub clients, and inits the mux.
// Returns the miniredis server and a cleanup function.
func setupMuxTest(t *testing.T) (mr *miniredis.Miniredis, cleanup func()) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err, "miniredis.Run() should succeed")

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	subClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// Verify both clients work
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, pubClient.Ping(ctx).Err(), "pubClient should connect")
	require.NoError(t, subClient.Ping(ctx).Err(), "subClient should connect")

	resetMuxState()
	InitCasbinWatcherMux(pubClient, subClient)

	cleanup = func() {
		ShutdownCasbinWatcherMux()
		pubClient.Close()
		subClient.Close()
		mr.Close()
		resetMuxState()
	}

	return mr, cleanup
}

// --------------------------------------------------------------------------
// 1. TestParseTenantIDFromChannel
// --------------------------------------------------------------------------

func TestParseTenantIDFromChannel(t *testing.T) {
	tests := []struct {
		name     string
		channel  string
		wantID   int
		wantOK   bool
	}{
		{"standard tenant 5", "/casbin/tenant/5", 5, true},
		{"tenant 0 is valid", "/casbin/tenant/0", 0, true},
		{"tenant 999", "/casbin/tenant/999", 999, true},
		{"empty string", "", 0, false},
		{"missing ID after slash", "/casbin/tenant/", 0, false},
		{"non-numeric ID", "/casbin/tenant/abc", 0, false},
		{"wrong prefix", "/other/prefix/5", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := parseTenantIDFromChannel(tt.channel)
			assert.Equal(t, tt.wantID, gotID)
			assert.Equal(t, tt.wantOK, gotOK)
		})
	}
}

// --------------------------------------------------------------------------
// 2. TestMux_RoutesToCorrectTenant
// --------------------------------------------------------------------------

func TestMux_RoutesToCorrectTenant(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	// Track which tenantIDs the enforcer lookup was called with.
	var lookedUp []int
	var lookupMu sync.Mutex

	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		lookupMu.Lock()
		lookedUp = append(lookedUp, tenantID)
		lookupMu.Unlock()
		return nil // nil enforcer → handleMessage returns after lookup
	})

	// Give the subscription time to become active.
	time.Sleep(150 * time.Millisecond)

	// Publish a message on tenant 5's channel.
	msg, err := json.Marshal(&MSG{Method: Update, ID: "other-instance-id"})
	require.NoError(t, err)

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	err = pubClient.Publish(context.Background(), "/casbin/tenant/5", msg).Err()
	require.NoError(t, err)

	// Wait for delivery.
	time.Sleep(150 * time.Millisecond)

	// Verify only tenant 5 was looked up.
	lookupMu.Lock()
	require.Equal(t, []int{5}, lookedUp, "only tenant 5 should be looked up")
	lookupMu.Unlock()
}

// --------------------------------------------------------------------------
// 3. TestMux_IgnoresSelfMessages
// --------------------------------------------------------------------------

func TestMux_IgnoresSelfMessages(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	// Track enforcer lookups — should NOT be called for self-messages.
	var lookedUp []int
	var lookupMu sync.Mutex

	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		lookupMu.Lock()
		lookedUp = append(lookedUp, tenantID)
		lookupMu.Unlock()
		return nil
	})

	// Give subscription time to activate.
	time.Sleep(150 * time.Millisecond)

	// Publish a message with the mux's own localID (self-message).
	require.NotNil(t, loadWatcherMux(), "mux should be initialized")
	selfID := loadWatcherMux().localID

	msg, err := json.Marshal(&MSG{Method: Update, ID: selfID})
	require.NoError(t, err)

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	err = pubClient.Publish(context.Background(), "/casbin/tenant/5", msg).Err()
	require.NoError(t, err)

	// Wait for potential delivery (should be ignored).
	time.Sleep(150 * time.Millisecond)

	// Verify no enforcer lookup happened.
	lookupMu.Lock()
	assert.Empty(t, lookedUp, "self-messages should be ignored, no enforcer lookup expected")
	lookupMu.Unlock()
}

// --------------------------------------------------------------------------
// 4. TestMux_NilGuard
// --------------------------------------------------------------------------

func TestMux_NilGuard(t *testing.T) {
	// Ensure global mux is nil.
	resetMuxState()
	defer resetMuxState()

	// Calling setupRedisWatcherForEnforcer with nil mux should not panic.
	// We need a real enforcer to pass — create a minimal one.
	m, err := model.NewModelFromString(text)
	require.NoError(t, err)

	e, err := casbin.NewSyncedEnforcer(m, &SuccessAdapter{})
	require.NoError(t, err)

	// This should be a no-op (no panic, no watcher set).
	assert.NotPanics(t, func() {
		setupRedisWatcherForEnforcer(e, 42)
	}, "setupRedisWatcherForEnforcer should not panic when mux is nil")

	// Enforcer should still function normally (GetPolicy returns empty slice, not nil).
	assert.NotPanics(t, func() { e.GetPolicy() }, "enforcer should be usable after nil mux guard")
}

// --------------------------------------------------------------------------
// 5. TestMux_ReconnectOnDrop
// --------------------------------------------------------------------------

func TestMux_ReconnectOnDrop(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	var lookedUp []int
	var lookupMu sync.Mutex

	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		lookupMu.Lock()
		lookedUp = append(lookedUp, tenantID)
		lookupMu.Unlock()
		return nil
	})

	// Wait for initial subscription.
	time.Sleep(150 * time.Millisecond)

	// Publish first message — should be received.
	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	msg, err := json.Marshal(&MSG{Method: Update, ID: "remote-instance"})
	require.NoError(t, err)

	err = pubClient.Publish(context.Background(), "/casbin/tenant/3", msg).Err()
	require.NoError(t, err)
	time.Sleep(150 * time.Millisecond)

	lookupMu.Lock()
	initialCount := len(lookedUp)
	lookupMu.Unlock()
	require.Equal(t, 1, initialCount, "first message should be delivered")

	// Restart miniredis to simulate a connection drop.
	mr.Restart()

	// Wait for the mux to detect the drop and attempt reconnection.
	// The mux uses exponential backoff starting at 1s, but since subscribeOnce
	// will get an error immediately on the closed channel, it starts reconnecting.
	// We need to wait for the reconnect cycle.
	time.Sleep(200 * time.Millisecond)

	// Publish another message after restart.
	msg2, err := json.Marshal(&MSG{Method: Update, ID: "remote-instance"})
	require.NoError(t, err)

	err = pubClient.Publish(context.Background(), "/casbin/tenant/3", msg2).Err()
	require.NoError(t, err)

	// Wait for delivery after reconnection.
	time.Sleep(200 * time.Millisecond)

	lookupMu.Lock()
	finalCount := len(lookedUp)
	lookupMu.Unlock()

	assert.Equal(t, 2, finalCount, "message after reconnect should be delivered")
}

// --------------------------------------------------------------------------
// Additional coverage: TestMux_DispatchToEnforcer_AllMethods
// --------------------------------------------------------------------------

// fullAdapter implements persist.Adapter + persist.BatchAdapter + persist.UpdatableAdapter
// so all dispatch methods work without panicking.
type fullAdapter struct{}

func (a *fullAdapter) LoadPolicy(model.Model) error          { return nil }
func (a *fullAdapter) SavePolicy(model.Model) error          { return nil }
func (a *fullAdapter) AddPolicy(string, string, []string) error    { return nil }
func (a *fullAdapter) RemovePolicy(string, string, []string) error { return nil }
func (a *fullAdapter) RemoveFilteredPolicy(string, string, int, ...string) error {
	return nil
}
func (a *fullAdapter) AddPolicies(string, string, [][]string) error { return nil }
func (a *fullAdapter) RemovePolicies(string, string, [][]string) error { return nil }
func (a *fullAdapter) UpdatePolicy(string, string, []string, []string) error {
	return nil
}
func (a *fullAdapter) UpdatePolicies(string, string, [][]string, [][]string) error {
	return nil
}
func (a *fullAdapter) UpdateFilteredPolicies(string, string, [][]string, int, ...string) ([][]string, error) {
	return nil, nil
}

func TestMux_DispatchToEnforcer_AllMethods(t *testing.T) {
	m, err := model.NewModelFromString(text)
	require.NoError(t, err)

	e, err := casbin.NewSyncedEnforcer(m, &fullAdapter{})
	require.NoError(t, err)

	// Add some initial policies so remove/update have something to work with.
	_, err = e.AddPolicy("admin", "/api/test", "GET")
	require.NoError(t, err)

	tests := []struct {
		name string
		msg  *MSG
	}{
		{
			name: "Update triggers LoadPolicy",
			msg:  &MSG{Method: Update},
		},
		{
			name: "UpdateForSavePolicy triggers LoadPolicy",
			msg:  &MSG{Method: UpdateForSavePolicy},
		},
		{
			name: "UpdateForAddPolicy",
			msg: &MSG{Method: UpdateForAddPolicy, Sec: "p", Ptype: "p",
				NewRule: []string{"user", "/api/new", "POST"}},
		},
		{
			name: "UpdateForRemovePolicy",
			msg: &MSG{Method: UpdateForRemovePolicy, Sec: "p", Ptype: "p",
				NewRule: []string{"admin", "/api/test", "GET"}},
		},
		{
			name: "UpdateForAddPolicies",
			msg: &MSG{Method: UpdateForAddPolicies, Sec: "p", Ptype: "p",
				NewRules: [][]string{{"user", "/api/batch1", "GET"}, {"user", "/api/batch2", "POST"}}},
		},
		{
			name: "UpdateForRemoveFilteredPolicy",
			msg: &MSG{Method: UpdateForRemoveFilteredPolicy, Sec: "p", Ptype: "p",
				FieldIndex: 0, FieldValues: []string{"admin"}},
		},
		{
			name: "UpdateForUpdatePolicy",
			msg: &MSG{Method: UpdateForUpdatePolicy, Sec: "p", Ptype: "p",
				OldRule: []string{"admin", "/api/test", "GET"},
				NewRule: []string{"admin", "/api/updated", "PUT"}},
		},
		{
			name: "unknown method is handled gracefully",
			msg:  &MSG{Method: UpdateType("UnknownMethod")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				dispatchToEnforcer(e, tt.msg)
			}, "dispatchToEnforcer should not panic for method %s", tt.msg.Method)
		})
	}
}

// --------------------------------------------------------------------------
// TestMux_HandleMessage_MalformedPayload
// --------------------------------------------------------------------------

func TestMux_HandleMessage_MalformedPayload(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	// No lookup should happen for malformed payloads.
	var lookedUp []int
	var lookupMu sync.Mutex

	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		lookupMu.Lock()
		lookedUp = append(lookedUp, tenantID)
		lookupMu.Unlock()
		return nil
	})

	time.Sleep(150 * time.Millisecond)

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	// Publish invalid JSON.
	err := pubClient.Publish(context.Background(), "/casbin/tenant/5", "not-valid-json{").Err()
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	lookupMu.Lock()
	assert.Empty(t, lookedUp, "malformed payload should not trigger enforcer lookup")
	lookupMu.Unlock()
}

// --------------------------------------------------------------------------
// TestMux_HandleMessage_ZeroTenantID
// --------------------------------------------------------------------------

func TestMux_HandleMessage_ZeroTenantID(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	var lookedUp []int
	var lookupMu sync.Mutex

	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		lookupMu.Lock()
		lookedUp = append(lookedUp, tenantID)
		lookupMu.Unlock()
		return nil
	})

	time.Sleep(150 * time.Millisecond)

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	// Publish to a channel that won't parse a valid tenant ID.
	msg, err := json.Marshal(&MSG{Method: Update, ID: "remote"})
	require.NoError(t, err)

	err = pubClient.Publish(context.Background(), "/casbin/tenant/abc", msg).Err()
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	lookupMu.Lock()
	assert.Empty(t, lookedUp, "unparseable tenant ID should not trigger enforcer lookup")
	lookupMu.Unlock()
}

// --------------------------------------------------------------------------
// TestMux_TenantZeroRoutes — regression for the sentinel-collision fix.
// Tenant 0 is a valid tenant (deprecated Setup() path) and must route,
// not be dropped as if malformed.
// --------------------------------------------------------------------------

func TestMux_TenantZeroRoutes(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	var lookedUp []int
	var lookupMu sync.Mutex

	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		lookupMu.Lock()
		lookedUp = append(lookedUp, tenantID)
		lookupMu.Unlock()
		return nil
	})

	time.Sleep(150 * time.Millisecond)

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	msg, err := json.Marshal(&MSG{Method: Update, ID: "remote"})
	require.NoError(t, err)

	err = pubClient.Publish(context.Background(), "/casbin/tenant/0", msg).Err()
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	lookupMu.Lock()
	assert.Equal(t, []int{0}, lookedUp, "tenant 0 should route to its enforcer, not be dropped")
	lookupMu.Unlock()
}

// --------------------------------------------------------------------------
// TestMux_MultipleTenants
// --------------------------------------------------------------------------

func TestMux_MultipleTenants(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	var lookedUp []int
	var lookupMu sync.Mutex

	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		lookupMu.Lock()
		lookedUp = append(lookedUp, tenantID)
		lookupMu.Unlock()
		return nil
	})

	time.Sleep(150 * time.Millisecond)

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	// Publish to multiple tenants.
	for _, tenantID := range []int{1, 5, 10, 42} {
		msg, err := json.Marshal(&MSG{Method: Update, ID: "remote"})
		require.NoError(t, err)
		ch := casbinChannel(tenantID)
		err = pubClient.Publish(context.Background(), ch, msg).Err()
		require.NoError(t, err)
	}

	time.Sleep(200 * time.Millisecond)

	lookupMu.Lock()
	assert.Len(t, lookedUp, 4, "all 4 tenant messages should trigger lookups")
	assert.Contains(t, lookedUp, 1)
	assert.Contains(t, lookedUp, 5)
	assert.Contains(t, lookedUp, 10)
	assert.Contains(t, lookedUp, 42)
	lookupMu.Unlock()
}

// --------------------------------------------------------------------------
// TestMux_PublishAndSelfID
// --------------------------------------------------------------------------

func TestMux_PublishSetsLocalID(t *testing.T) {
	_, cleanup := setupMuxTest(t)
	defer cleanup()

	require.NotNil(t, loadWatcherMux(), "mux should be initialized")
	assert.NotEmpty(t, loadWatcherMux().localID, "localID should be a non-empty UUID")
}

// --------------------------------------------------------------------------
// TestMux_CasbinWatcher_ImplementsInterfaces
// --------------------------------------------------------------------------

func TestMux_CasbinWatcher_ImplementsInterfaces(t *testing.T) {
	_, cleanup := setupMuxTest(t)
	defer cleanup()

	w := loadWatcherMux().NewWatcher(1)

	// Verify the watcher implements all required interfaces.
	assert.NotNil(t, w, "NewWatcher should return non-nil")

	// persist.Watcher
	_, ok := w.(interface {
		SetUpdateCallback(func(string)) error
		Update() error
		Close()
	})
	assert.True(t, ok, "should implement persist.Watcher")

	// persist.WatcherEx — use the actual interface from casbin
	type watcherEx interface {
		UpdateForAddPolicy(sec, ptype string, params ...string) error
		UpdateForRemovePolicy(sec, ptype string, params ...string) error
		UpdateForRemoveFilteredPolicy(sec, ptype string, fieldIndex int, fieldValues ...string) error
		UpdateForSavePolicy(model.Model) error
		UpdateForAddPolicies(sec string, ptype string, rules ...[]string) error
		UpdateForRemovePolicies(sec string, ptype string, rules ...[]string) error
	}
	_, ok = w.(watcherEx)
	assert.True(t, ok, "should implement persist.WatcherEx methods")

	// persist.UpdatableWatcher
	_, ok = w.(interface {
		UpdateForUpdatePolicy(sec string, ptype string, oldRule, newRule []string) error
		UpdateForUpdatePolicies(sec string, ptype string, oldRules, newRules [][]string) error
	})
	assert.True(t, ok, "should implement persist.UpdatableWatcher")
}

// --------------------------------------------------------------------------
// TestMux_CasbinWatcher_PublishMethods
// --------------------------------------------------------------------------

func TestMux_CasbinWatcher_PublishMethods(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	// Subscribe separately to capture published messages.
	subClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer subClient.Close()

	sub := subClient.Subscribe(context.Background(), "/casbin/tenant/99")
	defer sub.Close()

	// Use the watcher to publish messages.
	w := loadWatcherMux().NewWatcher(99).(*casbinWatcher)

	// Test Update
	err := w.Update()
	assert.NoError(t, err, "Update should not error")

	// Test UpdateForAddPolicy
	err = w.UpdateForAddPolicy("p", "p", "admin", "/api/test", "GET")
	assert.NoError(t, err)

	// Test UpdateForRemovePolicy
	err = w.UpdateForRemovePolicy("p", "p", "admin", "/api/test", "GET")
	assert.NoError(t, err)

	// Test UpdateForRemoveFilteredPolicy
	err = w.UpdateForRemoveFilteredPolicy("p", "p", 0, "admin")
	assert.NoError(t, err)

	// Test UpdateForSavePolicy
	err = w.UpdateForSavePolicy(nil)
	assert.NoError(t, err)

	// Test UpdateForAddPolicies
	err = w.UpdateForAddPolicies("p", "p", []string{"admin", "/api/a", "GET"}, []string{"admin", "/api/b", "POST"})
	assert.NoError(t, err)

	// Test UpdateForRemovePolicies
	err = w.UpdateForRemovePolicies("p", "p", []string{"admin", "/api/a", "GET"})
	assert.NoError(t, err)

	// Test UpdateForUpdatePolicy
	err = w.UpdateForUpdatePolicy("p", "p", []string{"admin", "/old", "GET"}, []string{"admin", "/new", "PUT"})
	assert.NoError(t, err)

	// Test UpdateForUpdatePolicies
	err = w.UpdateForUpdatePolicies("p", "p",
		[][]string{{"admin", "/old", "GET"}},
		[][]string{{"admin", "/new", "PUT"}})
	assert.NoError(t, err)

	// Test SetUpdateCallback (no-op, should return nil)
	err = w.SetUpdateCallback(func(string) {})
	assert.NoError(t, err)

	// Test Close (no-op, should not panic)
	assert.NotPanics(t, func() { w.Close() })
}

// --------------------------------------------------------------------------
// TestMux_PublishOnClosedMux
// --------------------------------------------------------------------------

func TestMux_PublishOnClosedMux(t *testing.T) {
	_, cleanup := setupMuxTest(t)
	defer cleanup()

	// Close the mux.
	ShutdownCasbinWatcherMux()

	// Publishing on a closed mux should not panic and should return nil (graceful degradation).
	w := &casbinWatcher{
		mux:      loadWatcherMux(),
		tenantID: 1,
		channel:  casbinChannel(1),
	}

	err := w.Update()
	assert.NoError(t, err, "Update on closed mux should return nil (graceful degradation)")
}

// --------------------------------------------------------------------------
// TestMux_ShutdownIdempotent
// --------------------------------------------------------------------------

func TestMux_ShutdownIdempotent(t *testing.T) {
	_, cleanup := setupMuxTest(t)
	defer cleanup()

	// Calling Shutdown multiple times should not panic.
	assert.NotPanics(t, func() {
		ShutdownCasbinWatcherMux()
		ShutdownCasbinWatcherMux()
		ShutdownCasbinWatcherMux()
	})
}

// --------------------------------------------------------------------------
// TestMux_ShutdownWhenNil
// --------------------------------------------------------------------------

func TestMux_ShutdownWhenNil(t *testing.T) {
	resetMuxState()
	defer resetMuxState()

	// Calling Shutdown when mux is nil should be a no-op.
	assert.NotPanics(t, func() {
		ShutdownCasbinWatcherMux()
	})
}

// --------------------------------------------------------------------------
// TestMux_InitIdempotent
// --------------------------------------------------------------------------

func TestMux_InitIdempotent(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	resetMuxState()
	defer resetMuxState()

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	subClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()
	defer subClient.Close()

	// Init twice — second call should be a no-op.
	InitCasbinWatcherMux(pubClient, subClient)
	firstID := loadWatcherMux().localID

	InitCasbinWatcherMux(pubClient, subClient)
	secondID := loadWatcherMux().localID

	assert.Equal(t, firstID, secondID, "second init should be a no-op, localID unchanged")
}

// --------------------------------------------------------------------------
// TestMux_NilEnforcerLookup
// --------------------------------------------------------------------------

func TestMux_NilEnforcerLookup(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	// Do NOT set getEnforcerFn — it should remain nil.
	// Messages should be handled without panic.

	var lookedUp []int
	var lookupMu sync.Mutex

	// Temporarily set a lookup just to verify it's NOT called.
	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		lookupMu.Lock()
		lookedUp = append(lookedUp, tenantID)
		lookupMu.Unlock()
		return nil
	})

	time.Sleep(150 * time.Millisecond)

	// Now clear the lookup to test nil path.
	getEnforcerFn = nil

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	msg, err := json.Marshal(&MSG{Method: Update, ID: "remote"})
	require.NoError(t, err)

	err = pubClient.Publish(context.Background(), "/casbin/tenant/5", msg).Err()
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	// No lookup should have happened since getEnforcerFn is nil.
	lookupMu.Lock()
	assert.Empty(t, lookedUp, "nil getEnforcerFn should not trigger any lookup")
	lookupMu.Unlock()
}

// --------------------------------------------------------------------------
// TestMux_NilEnforcerReturned
// --------------------------------------------------------------------------

func TestMux_NilEnforcerReturned(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	// Set getEnforcerFn to return nil (tenant not registered).
	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		return nil
	})

	time.Sleep(150 * time.Millisecond)

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()

	msg, err := json.Marshal(&MSG{Method: Update, ID: "remote"})
	require.NoError(t, err)

	// Should not panic.
	assert.NotPanics(t, func() {
		_ = pubClient.Publish(context.Background(), "/casbin/tenant/999", msg).Err()
		time.Sleep(150 * time.Millisecond)
	})
}

// --------------------------------------------------------------------------
// TestMux_SetEnforcerLookup
// --------------------------------------------------------------------------

func TestMux_SetEnforcerLookup(t *testing.T) {
	resetMuxState()
	defer resetMuxState()

	called := false
	SetEnforcerLookup(func(tenantID int) *casbin.SyncedEnforcer {
		called = true
		return nil
	})

	require.NotNil(t, getEnforcerFn, "getEnforcerFn should be set")
	getEnforcerFn(1)
	assert.True(t, called, "lookup function should be callable after SetEnforcerLookup")
}

// --------------------------------------------------------------------------
// TestMux_CasbinChannel
// --------------------------------------------------------------------------

func TestMux_CasbinChannel(t *testing.T) {
	assert.Equal(t, "/casbin/tenant/1", casbinChannel(1))
	assert.Equal(t, "/casbin/tenant/0", casbinChannel(0))
	assert.Equal(t, "/casbin/tenant/999", casbinChannel(999))
	assert.Equal(t, "/casbin/tenant/*", casbinChannelPattern())
}

// --------------------------------------------------------------------------
// TestMux_MSGSerialization
// --------------------------------------------------------------------------

func TestMux_MSGSerialization(t *testing.T) {
	original := &MSG{
		Method:      UpdateForAddPolicy,
		ID:          "test-id-123",
		Sec:         "p",
		Ptype:       "p",
		OldRule:     []string{"a", "b"},
		NewRule:     []string{"c", "d"},
		FieldIndex:  2,
		FieldValues: []string{"x", "y"},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	parsed := &MSG{}
	err = json.Unmarshal(data, parsed)
	require.NoError(t, err)

	assert.Equal(t, original.Method, parsed.Method)
	assert.Equal(t, original.ID, parsed.ID)
	assert.Equal(t, original.Sec, parsed.Sec)
	assert.Equal(t, original.Ptype, parsed.Ptype)
	assert.Equal(t, original.FieldIndex, parsed.FieldIndex)
	assert.Equal(t, original.OldRule, parsed.OldRule)
	assert.Equal(t, original.NewRule, parsed.NewRule)
	assert.Equal(t, original.FieldValues, parsed.FieldValues)
}

// --------------------------------------------------------------------------
// TestMux_NilPublishClient
// --------------------------------------------------------------------------

func TestMux_NilPublishClient(t *testing.T) {
	// Test the publish method's nil guard.
	mux := &CasbinWatcherMux{
		pubClient: nil,
		localID:   "test",
		ctx:       context.Background(),
	}

	err := mux.publish(1, &MSG{Method: Update})
	assert.NoError(t, err, "publish with nil pubClient should return nil (graceful degradation)")
}

// --------------------------------------------------------------------------
// TestMux_NilMuxPublish
// --------------------------------------------------------------------------

func TestMux_NilMuxPublish(t *testing.T) {
	var mux *CasbinWatcherMux = nil

	err := mux.publish(1, &MSG{Method: Update})
	assert.NoError(t, err, "publish on nil mux should return nil (graceful degradation)")
}

// --------------------------------------------------------------------------
// TestShardForTenant — per-tenant routing is deterministic and in range
// --------------------------------------------------------------------------

func TestShardForTenant(t *testing.T) {
	// Same tenant always maps to the same shard (the ordering guarantee depends
	// on this determinism).
	for _, id := range []int{0, 1, 7, 8, 99, 1000, -1, -8, -15} {
		s1 := shardForTenant(id)
		s2 := shardForTenant(id)
		assert.Equal(t, s1, s2, "shardForTenant must be deterministic for tenant %d", id)
		assert.GreaterOrEqual(t, s1, 0, "shard index must be non-negative for tenant %d", id)
		assert.Less(t, s1, dispatchWorkers, "shard index must be < dispatchWorkers for tenant %d", id)
	}
}

// --------------------------------------------------------------------------
// TestMux_PreservesPerTenantOrder — regression for the worker-pool ordering
// hazard. Messages for one tenant must be applied in publish order, so an
// Add-then-Remove of the same rule ends with the rule absent (not present).
// --------------------------------------------------------------------------

func TestMux_PreservesPerTenantOrder(t *testing.T) {
	mr, cleanup := setupMuxTest(t)
	defer cleanup()

	const tenantID = 7

	m, err := model.NewModelFromString(text)
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m, &fullAdapter{})
	require.NoError(t, err)

	SetEnforcerLookup(func(id int) *casbin.SyncedEnforcer {
		if id == tenantID {
			return enforcer
		}
		return nil
	})

	// Let the subscription become active.
	time.Sleep(150 * time.Millisecond)

	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer pubClient.Close()
	channel := casbinChannel(tenantID)

	rule := []string{"alice", "/api/order", "GET"}
	// Publish an ordered sequence on the SAME tenant channel: Add rule, then
	// Remove the same rule. Single-worker-per-tenant must apply them in order.
	publish := func(method UpdateType, newRule []string) {
		payload, mErr := json.Marshal(&MSG{
			Method: method, ID: "remote-instance", Sec: "p", Ptype: "p", NewRule: newRule,
		})
		require.NoError(t, mErr)
		require.NoError(t, pubClient.Publish(context.Background(), channel, payload).Err())
	}
	publish(UpdateForAddPolicy, rule)
	publish(UpdateForRemovePolicy, rule)

	// Wait for both messages to drain.
	time.Sleep(300 * time.Millisecond)

	has := enforcer.HasPolicy(rule)
	assert.False(t, has,
		"Add-then-Remove applied in order must leave the rule absent; presence indicates out-of-order dispatch")
}

// --------------------------------------------------------------------------
// TestEnsureWatcherMux — lazy initialisation from configured Redis clients (F2)
// --------------------------------------------------------------------------

func TestEnsureWatcherMux_NoRedis(t *testing.T) {
	resetMuxState()
	defer resetMuxState()
	config.ResetRedisClientsForTest()
	defer config.ResetRedisClientsForTest()

	// No Redis configured → graceful degradation, no mux.
	assert.Nil(t, ensureWatcherMux(), "ensureWatcherMux should return nil when Redis is not configured")
	assert.Nil(t, loadWatcherMux(), "mux should not be initialised without Redis")
}

func TestEnsureWatcherMux_NoSubscriber(t *testing.T) {
	resetMuxState()
	defer resetMuxState()
	config.ResetRedisClientsForTest()
	defer config.ResetRedisClientsForTest()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Client #1 set, but Client #3 (subscriber) missing → degrade with warning.
	config.SetRedisClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	assert.Nil(t, ensureWatcherMux(), "ensureWatcherMux should return nil when subscriber client is missing")
	assert.Nil(t, loadWatcherMux(), "mux should not be initialised without a subscriber client")
}

func TestEnsureWatcherMux_LazyInit(t *testing.T) {
	resetMuxState()
	defer resetMuxState()
	config.ResetRedisClientsForTest()
	defer config.ResetRedisClientsForTest()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// Both Client #1 and Client #3 configured → mux is created on first use.
	config.SetRedisClient(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	_, err = config.EnsureSubscriberClient(config.RedisConnectOptions{Addr: mr.Addr()})
	require.NoError(t, err)

	m := ensureWatcherMux()
	require.NotNil(t, m, "ensureWatcherMux should initialise the mux when both clients exist")
	assert.Same(t, m, loadWatcherMux(), "ensureWatcherMux must return the stored singleton")

	// Second call is idempotent — same instance, no re-init.
	assert.Same(t, m, ensureWatcherMux(), "ensureWatcherMux must be idempotent")

	ShutdownCasbinWatcherMux()
}

// --------------------------------------------------------------------------
// Worker-pool ordering, concurrency, and backpressure coverage (F3)
//
// These tests bypass Redis and inject *redis.Message values straight into the
// per-shard dispatch queues, which deterministically exercises the worker pool
// — the path that a2c5490 introduced to preserve per-tenant ordering while
// isolating slow tenants. miniredis pub/sub is timing-dependent and cannot
// reliably stress the worker pool; direct injection can.
// --------------------------------------------------------------------------

// recordingAdapter wraps fullAdapter and records every AddPolicy call in
// order. The mutex makes the recording safe for the multi-worker concurrent test.
type recordingAdapter struct {
	fullAdapter
	mu    sync.Mutex
	calls []string
}

func (a *recordingAdapter) AddPolicy(sec, ptype string, rule []string) error {
	a.mu.Lock()
	a.calls = append(a.calls, "add:"+strings.Join(rule, ","))
	a.mu.Unlock()
	return a.fullAdapter.AddPolicy(sec, ptype, rule)
}

// Count returns the number of recorded AddPolicy calls (thread-safe).
func (a *recordingAdapter) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

// Calls returns a copy of the recorded calls in order (thread-safe).
func (a *recordingAdapter) Calls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.calls))
	copy(out, a.calls)
	return out
}

// tryInject enqueues a message for tenantID directly into its shard's dispatch
// queue (shardForTenant), bypassing Redis. It returns a bool instead of calling
// t.Fatal so it stays safe to invoke from worker goroutines in the concurrent
// test. A false result means the enqueue blocked past the timeout — a worker is
// not draining (backpressure deadlock) rather than the intended transient block
// on a full buffer.
func tryInject(tenantID int, method UpdateType, rule []string) bool {
	mux := loadWatcherMux()
	if mux == nil {
		return false
	}
	payload, err := json.Marshal(&MSG{
		Method: method, ID: "remote-test", Sec: "p", Ptype: "p", NewRule: rule,
	})
	if err != nil {
		return false
	}
	msg := &redis.Message{
		Channel: casbinChannel(tenantID),
		Payload: string(payload),
	}
	shard := shardForTenant(tenantID)
	select {
	case mux.dispatchChs[shard] <- msg:
		return true
	case <-time.After(3 * time.Second):
		return false
	}
}

// TestMux_DispatchWorker_PerShardOrdering asserts the core guarantee of the
// sharded worker pool: all messages for one tenant (one shard -> one worker)
// are applied strictly in publish order. This is the hazard a2c5490 introduced
// and must not regress — out-of-order Add/Remove on the same rule would leave
// policy state divergent across instances.
func TestMux_DispatchWorker_PerShardOrdering(t *testing.T) {
	_, cleanup := setupMuxTest(t)
	defer cleanup()

	const tenantID = 7
	m, err := model.NewModelFromString(text)
	require.NoError(t, err)
	adapter := &recordingAdapter{}
	enforcer, err := casbin.NewSyncedEnforcer(m, adapter)
	require.NoError(t, err)
	enforcer.EnableAutoSave(true)

	SetEnforcerLookup(func(id int) *casbin.SyncedEnforcer {
		if id == tenantID {
			return enforcer
		}
		return nil
	})

	const n = 30
	for i := 0; i < n; i++ {
		require.True(t, tryInject(tenantID, UpdateForAddPolicy, []string{"alice", "/seq", fmt.Sprintf("%d", i)}),
			"ordered message %d should enqueue", i)
	}

	require.Eventually(t, func() bool { return adapter.Count() == n },
		2*time.Second, 5*time.Millisecond,
		"all %d ordered messages should be processed", n)

	calls := adapter.Calls()
	require.Len(t, calls, n)
	for i, c := range calls {
		assert.Equal(t, fmt.Sprintf("add:alice,/seq,%d", i), c,
			"message %d processed out of order — single-worker-per-shard must preserve publish order", i)
	}
}

// TestMux_WorkerPool_ConcurrentPerTenant runs all 8 workers concurrently, each
// dispatching to its OWN tenant enforcer (one enforcer per shard). This mirrors
// production: shardForTenant deterministically maps each tenant to one shard →
// one worker → one enforcer, so every enforcer has exactly one writer. Run
// under `go test -race` to confirm the shared dispatch path (getEnforcerFn
// lookup, logging, channel routing) is race-free under concurrent operation.
//
// Why one enforcer per tenant, not one shared: casbin's Self* methods are NOT
// individually locked by SyncedEnforcer — addPolicyWithoutNotify mutates the
// model map without a lock, so two workers sharing one enforcer crash with
// "concurrent map read and map write". The single-writer-per-enforcer property
// shardForTenant guarantees is what makes those calls safe; this test respects
// it by giving each tenant its own enforcer.
func TestMux_WorkerPool_ConcurrentPerTenant(t *testing.T) {
	_, cleanup := setupMuxTest(t)
	defer cleanup()

	const tenants = 8 // tenantID % 8 yields all 8 distinct shards
	const perTenant = 40

	enforcers := make([]*casbin.SyncedEnforcer, tenants)
	adapters := make([]*recordingAdapter, tenants)
	for tID := 0; tID < tenants; tID++ {
		m, err := model.NewModelFromString(text)
		require.NoError(t, err)
		ad := &recordingAdapter{}
		e, err := casbin.NewSyncedEnforcer(m, ad)
		require.NoError(t, err)
		e.EnableAutoSave(true)
		enforcers[tID] = e
		adapters[tID] = ad
	}

	SetEnforcerLookup(func(id int) *casbin.SyncedEnforcer {
		if id >= 0 && id < tenants {
			return enforcers[id]
		}
		return nil
	})

	var (
		wg         sync.WaitGroup
		errMu      sync.Mutex
		injectErrs []string
	)
	for tID := 0; tID < tenants; tID++ {
		wg.Add(1)
		go func(tenantID int) {
			defer wg.Done()
			for i := 0; i < perTenant; i++ {
				if !tryInject(tenantID, UpdateForAddPolicy,
					[]string{"u", "/x", fmt.Sprintf("%d-%d", tenantID, i)}) {
					errMu.Lock()
					injectErrs = append(injectErrs, fmt.Sprintf("tenant %d msg %d", tenantID, i))
					errMu.Unlock()
				}
			}
		}(tID)
	}
	wg.Wait()
	require.Empty(t, injectErrs, "some injections timed out (worker not draining?): %v", injectErrs)

	// No-loss assertion per tenant via the recording adapter (mutex-safe — unlike
	// a direct GetPolicy, which races casbin's unlocked model write). Unique rules
	// mean each message records exactly one AddPolicy, so Count==perTenant proves
	// the worker applied every message.
	for tID := 0; tID < tenants; tID++ {
		tid := tID
		require.Eventually(t, func() bool { return adapters[tid].Count() == perTenant },
			3*time.Second, 5*time.Millisecond,
			"tenant %d should process all %d messages (no loss under concurrent dispatch)", tid, perTenant)
	}
}

// TestMux_DispatchWorker_BurstBeyondBuffer injects more messages than a shard's
// dispatchBufferSize. The worker drains concurrently, so the sender must block
// (backpressure) and resume — never drop. This guards the head-of-line
// message-loss hazard the bounded pool was added to prevent.
func TestMux_DispatchWorker_BurstBeyondBuffer(t *testing.T) {
	_, cleanup := setupMuxTest(t)
	defer cleanup()

	const tenantID = 7
	m, err := model.NewModelFromString(text)
	require.NoError(t, err)
	adapter := &recordingAdapter{}
	enforcer, err := casbin.NewSyncedEnforcer(m, adapter)
	require.NoError(t, err)
	enforcer.EnableAutoSave(true)

	SetEnforcerLookup(func(id int) *casbin.SyncedEnforcer {
		if id == tenantID {
			return enforcer
		}
		return nil
	})

	total := dispatchBufferSize + 100 // 356 > 256 → exercises backpressure, not drop
	for i := 0; i < total; i++ {
		require.True(t, tryInject(tenantID, UpdateForAddPolicy, []string{"burst", "/x", fmt.Sprintf("%d", i)}),
			"burst message %d must enqueue (backpressure, not drop)", i)
	}

	require.Eventually(t, func() bool { return adapter.Count() == total },
		3*time.Second, 5*time.Millisecond,
		"all %d burst messages must be processed — no loss past the buffer", total)
}
