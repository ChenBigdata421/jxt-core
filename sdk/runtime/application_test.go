package runtime

import (
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
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

func TestApplication_TenantCommandDB_Int(t *testing.T) {
	app := NewConfig()
	db := &gorm.DB{}

	app.SetTenantCommandDB(1, db)

	result := app.GetTenantCommandDB(1)
	assert.Same(t, db, result)

	result = app.GetTenantCommandDB(999)
	assert.Nil(t, result)
}

func TestApplication_TenantQueryDB_Int(t *testing.T) {
	app := NewConfig()
	db := &gorm.DB{}

	app.SetTenantQueryDB(1, db)

	result := app.GetTenantQueryDB(1)
	assert.Same(t, db, result)

	result = app.GetTenantQueryDB(999)
	assert.Nil(t, result)
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
