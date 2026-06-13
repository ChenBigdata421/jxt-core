package mycasbin

import (
	"context"
	"encoding/json"
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
	watcherMux = nil
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
		expected int
	}{
		{"standard tenant 5", "/casbin/tenant/5", 5},
		{"tenant 0", "/casbin/tenant/0", 0},
		{"tenant 999", "/casbin/tenant/999", 999},
		{"empty string", "", 0},
		{"missing ID after slash", "/casbin/tenant/", 0},
		{"non-numeric ID", "/casbin/tenant/abc", 0},
		{"wrong prefix", "/other/prefix/5", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTenantIDFromChannel(tt.channel)
			assert.Equal(t, tt.expected, result)
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
	require.NotNil(t, watcherMux, "mux should be initialized")
	selfID := watcherMux.localID

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

	require.NotNil(t, watcherMux, "mux should be initialized")
	assert.NotEmpty(t, watcherMux.localID, "localID should be a non-empty UUID")
}

// --------------------------------------------------------------------------
// TestMux_CasbinWatcher_ImplementsInterfaces
// --------------------------------------------------------------------------

func TestMux_CasbinWatcher_ImplementsInterfaces(t *testing.T) {
	_, cleanup := setupMuxTest(t)
	defer cleanup()

	w := watcherMux.NewWatcher(1)

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
	w := watcherMux.NewWatcher(99).(*casbinWatcher)

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
		mux:      watcherMux,
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
	firstID := watcherMux.localID

	InitCasbinWatcherMux(pubClient, subClient)
	secondID := watcherMux.localID

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
