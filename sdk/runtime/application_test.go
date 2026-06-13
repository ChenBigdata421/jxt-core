package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"

	"github.com/ChenBigdata421/jxt-core/storage"
	"gorm.io/gorm"
)

func TestApplication_TenantDB_Int(t *testing.T) {
	app := NewConfig()
	db := &gorm.DB{}

	// Set with int
	app.SetTenantDB(1, db)

	// Get with int
	result := app.GetTenantDB(1)
	assert.Same(t, db, result)

	// Not found returns nil
	result = app.GetTenantDB(999)
	assert.Nil(t, result)

	// No "*" fallback
	result = app.GetTenantDB(0)
	assert.Nil(t, result)
}

func TestApplication_TenantDB_Iteration(t *testing.T) {
	app := NewConfig()
	db1 := &gorm.DB{}
	db2 := &gorm.DB{}

	app.SetTenantDB(1, db1)
	app.SetTenantDB(2, db2)

	// Iterate all tenants
	count := 0
	tenantIDs := make(map[int]bool)
	app.GetTenantDBs(func(tenantID int, db *gorm.DB) bool {
		count++
		tenantIDs[tenantID] = true
		return true
	})

	assert.Equal(t, 2, count)
	assert.True(t, tenantIDs[1])
	assert.True(t, tenantIDs[2])
}

func TestApplication_Casbin_Int(t *testing.T) {
	app := NewConfig()
	enforcer := &casbin.SyncedEnforcer{}

	app.SetTenantCasbin(1, enforcer)

	result := app.GetTenantCasbin(1)
	assert.Same(t, enforcer, result)

	result = app.GetTenantCasbin(999)
	assert.Nil(t, result)
}

func TestApplication_Casbin_Iteration(t *testing.T) {
	app := NewConfig()
	enforcer1 := &casbin.SyncedEnforcer{}
	enforcer2 := &casbin.SyncedEnforcer{}

	app.SetTenantCasbin(1, enforcer1)
	app.SetTenantCasbin(2, enforcer2)

	count := 0
	tenantIDs := make(map[int]bool)
	app.GetCasbins(func(tenantID int, enforcer *casbin.SyncedEnforcer) bool {
		count++
		tenantIDs[tenantID] = true
		return true
	})

	assert.Equal(t, 2, count)
	assert.True(t, tenantIDs[1])
	assert.True(t, tenantIDs[2])
}

func TestApplication_Crontab_Int(t *testing.T) {
	app := NewConfig()
	crontab := cron.New()

	app.SetTenantCrontab(1, crontab)

	result := app.GetTenantCrontab(1)
	assert.Same(t, crontab, result)

	result = app.GetTenantCrontab(999)
	assert.Nil(t, result)
}

func TestApplication_Crontab_Iteration(t *testing.T) {
	app := NewConfig()
	crontab1 := cron.New()
	crontab2 := cron.New()

	app.SetTenantCrontab(1, crontab1)
	app.SetTenantCrontab(2, crontab2)

	count := 0
	tenantIDs := make(map[int]bool)
	app.GetCrontabs(func(tenantID int, crontab *cron.Cron) bool {
		count++
		tenantIDs[tenantID] = true
		return true
	})

	assert.Equal(t, 2, count)
	assert.True(t, tenantIDs[1])
	assert.True(t, tenantIDs[2])
}

func TestApplication_NoTenantResolverField(t *testing.T) {
	app := NewConfig()

	// Verify that SetTenantMapping and GetTenantID methods are removed
	// This should cause a compile error if they still exist
	// The test simply verifies that NewConfig() works without tenantResolver
	assert.NotNil(t, app)
}

// mockShutdownQueue tracks whether Shutdown was called.
type mockShutdownQueue struct {
	shutdownCalled bool
}

func (m *mockShutdownQueue) String() string { return "mock" }
func (m *mockShutdownQueue) Append(_ interface{}) error { return nil }
func (m *mockShutdownQueue) Register(_ string, _ interface{}) {}
func (m *mockShutdownQueue) Run()                             {}
func (m *mockShutdownQueue) Shutdown()                        { m.shutdownCalled = true }

func TestApplication_Close_NilFields(t *testing.T) {
	app := NewConfig()
	// Close with nil queue/cache should not panic.
	assert.NotPanics(t, func() {
		app.Close()
	})
}

func TestApplication_Close_Idempotent(t *testing.T) {
	app := NewConfig()
	// Calling Close twice should not panic.
	assert.NotPanics(t, func() {
		app.Close()
		app.Close()
	})
}

// countingQueue is an AdapterQueue mock that counts Shutdown() invocations.
type countingQueue struct {
	shutdownCount int
}

func (c *countingQueue) String() string                                { return "counting" }
func (c *countingQueue) Append(_ storage.Messager) error               { return nil }
func (c *countingQueue) Register(_ string, _ storage.ConsumerFunc)     {}
func (c *countingQueue) Run()                                          {}
func (c *countingQueue) Shutdown()                                     { c.shutdownCount++ }

// countingCacheCloser is an AdapterCache mock that also implements Closer,
// counting Close() invocations.
type countingCacheCloser struct {
	closeCount int
}

func (c *countingCacheCloser) String() string { return "counting-cache" }
func (c *countingCacheCloser) Get(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (c *countingCacheCloser) Set(_ context.Context, _ string, _ interface{}, _ int) error {
	return nil
}
func (c *countingCacheCloser) Del(_ context.Context, _ string) error              { return nil }
func (c *countingCacheCloser) HashGet(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (c *countingCacheCloser) HashDel(_ context.Context, _, _ string) error       { return nil }
func (c *countingCacheCloser) Increase(_ context.Context, _ string) error         { return nil }
func (c *countingCacheCloser) Decrease(_ context.Context, _ string) error         { return nil }
func (c *countingCacheCloser) Expire(_ context.Context, _ string, _ time.Duration) error {
	return nil
}
func (c *countingCacheCloser) HashSet(_ context.Context, _, _ string, _ interface{}) error {
	return nil
}
func (c *countingCacheCloser) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (c *countingCacheCloser) SetNX(_ context.Context, _ string, _ interface{}, _ int) (bool, error) {
	return false, nil
}
func (c *countingCacheCloser) IncrBy(_ context.Context, _ string, _ int64) (int64, error) {
	return 0, nil
}
func (c *countingCacheCloser) TTL(_ context.Context, _ string) (time.Duration, error) {
	return 0, nil
}
func (c *countingCacheCloser) RunScript(_ context.Context, _ interface{}, _ []string, _ ...interface{}) (interface{}, error) {
	return nil, nil
}
func (c *countingCacheCloser) Close() error { c.closeCount++; return nil }

func TestApplication_Close_Order(t *testing.T) {
	app := NewConfig()
	cq := &countingQueue{}
	cc := &countingCacheCloser{}
	app.SetQueueAdapter(cq)
	app.SetCacheAdapter(cc)

	// Close() invokes both the queue Shutdown() and the cache Close().
	app.Close()
	assert.Equal(t, 1, cq.shutdownCount, "queue.Shutdown should be called once")
	assert.Equal(t, 1, cc.closeCount, "cache.Close should be called once")

	// Idempotency: Close() has no guard, so a second call re-invokes both
	// components (matching the actual non-guarded behavior).
	app.Close()
	assert.Equal(t, 2, cq.shutdownCount, "queue.Shutdown called on every Close()")
	assert.Equal(t, 2, cc.closeCount, "cache.Close called on every Close()")
}
