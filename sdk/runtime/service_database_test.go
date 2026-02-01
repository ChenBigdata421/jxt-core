package runtime

import (
	"testing"

	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
)

func TestApplication_SetTenantServiceDB(t *testing.T) {
	app := NewConfig()
	db1 := &gorm.DB{}
	db2 := &gorm.DB{}

	// Set service database connections
	app.SetTenantServiceDB(1001, "evidence-command", db1)
	app.SetTenantServiceDB(1001, "evidence-query", db2)
	app.SetTenantServiceDB(1002, "evidence-command", db1)

	// Verify the connections are set correctly
	result := app.GetTenantServiceDB(1001, "evidence-command")
	assert.Same(t, db1, result)

	result = app.GetTenantServiceDB(1001, "evidence-query")
	assert.Same(t, db2, result)

	result = app.GetTenantServiceDB(1002, "evidence-command")
	assert.Same(t, db1, result)
}

func TestApplication_GetTenantServiceDB_NotFound(t *testing.T) {
	app := NewConfig()
	db := &gorm.DB{}

	app.SetTenantServiceDB(1001, "evidence-command", db)

	// Try to get non-existent service
	result := app.GetTenantServiceDB(1001, "non-existent")
	assert.Nil(t, result)

	// Try to get non-existent tenant
	result = app.GetTenantServiceDB(9999, "evidence-command")
	assert.Nil(t, result)
}

func TestApplication_GetTenantServiceDBs(t *testing.T) {
	app := NewConfig()
	db1 := &gorm.DB{}
	db2 := &gorm.DB{}

	// Set multiple service database connections
	app.SetTenantServiceDB(1001, "evidence-command", db1)
	app.SetTenantServiceDB(1001, "evidence-query", db2)
	app.SetTenantServiceDB(1002, "evidence-command", db1)

	// Iterate all service database connections
	count := 0
	tenantServices := make(map[string]bool)
	app.GetTenantServiceDBs(func(tenantID int, serviceCode string, db *gorm.DB) bool {
		count++
		key := string(rune(tenantID)) + ":" + serviceCode
		tenantServices[key] = true
		return true
	})

	assert.Equal(t, 3, count)
}

func TestApplication_buildTenantServiceKey(t *testing.T) {
	app := NewConfig()

	key := app.buildTenantServiceKey(1001, "evidence-command")
	assert.Equal(t, "1001:evidence-command", key)

	key = app.buildTenantServiceKey(0, "test")
	assert.Equal(t, "0:test", key)

	key = app.buildTenantServiceKey(1001, "")
	assert.Equal(t, "1001:", key)
}

func TestApplication_parseTenantServiceKey(t *testing.T) {
	app := NewConfig()

	// Valid key
	tenantID, serviceCode := app.parseTenantServiceKey("1001:evidence-command")
	assert.Equal(t, 1001, tenantID)
	assert.Equal(t, "evidence-command", serviceCode)

	// Valid key with empty service code
	tenantID, serviceCode = app.parseTenantServiceKey("1001:")
	assert.Equal(t, 1001, tenantID)
	assert.Equal(t, "", serviceCode)

	// Invalid key - no colon
	tenantID, serviceCode = app.parseTenantServiceKey("invalid")
	assert.Equal(t, 0, tenantID)
	assert.Equal(t, "", serviceCode)

	// Invalid key - empty string
	tenantID, serviceCode = app.parseTenantServiceKey("")
	assert.Equal(t, 0, tenantID)
	assert.Equal(t, "", serviceCode)

	// Invalid key - multiple colons
	tenantID, serviceCode = app.parseTenantServiceKey("1001:evidence:command")
	assert.Equal(t, 1001, tenantID)
	assert.Equal(t, "evidence:command", serviceCode)

	// Invalid key - non-numeric tenant ID
	tenantID, serviceCode = app.parseTenantServiceKey("abc:evidence-command")
	assert.Equal(t, 0, tenantID)
	assert.Equal(t, "evidence-command", serviceCode)
}

func TestApplication_GetTenantDB_BackwardCompatibility(t *testing.T) {
	app := NewConfig()
	db1 := &gorm.DB{}

	// Set service-level security-management
	app.SetTenantServiceDB(1001, "security-management", db1)

	// Should return security-management service DB
	result := app.GetTenantDB(1001)
	assert.Same(t, db1, result)

	// Should return nil when no service-level DB is set
	result = app.GetTenantDB(9999)
	assert.Nil(t, result)
}

func TestApplication_TenantServiceDB_ConcurrentAccess(t *testing.T) {
	app := NewConfig()
	db1 := &gorm.DB{}
	db2 := &gorm.DB{}

	done := make(chan bool, 20)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(tenantID int) {
			if tenantID%2 == 0 {
				app.SetTenantServiceDB(tenantID, "evidence-command", db1)
			} else {
				app.SetTenantServiceDB(tenantID, "evidence-command", db2)
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(tenantID int) {
			app.GetTenantServiceDB(tenantID, "evidence-command")
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify no data races occurred - test passes if we reach here
	assert.True(t, true)
}

func TestApplication_TenantServiceDB_Override(t *testing.T) {
	app := NewConfig()
	db1 := &gorm.DB{}
	db2 := &gorm.DB{}

	// Set initial DB
	app.SetTenantServiceDB(1001, "evidence-command", db1)
	result := app.GetTenantServiceDB(1001, "evidence-command")
	assert.Same(t, db1, result)

	// Override with new DB
	app.SetTenantServiceDB(1001, "evidence-command", db2)
	result = app.GetTenantServiceDB(1001, "evidence-command")
	assert.Same(t, db2, result)
}

func TestApplication_TenantServiceDB_MultipleServices(t *testing.T) {
	app := NewConfig()
	db1 := &gorm.DB{}
	db2 := &gorm.DB{}
	db3 := &gorm.DB{}

	// Set multiple services for same tenant
	app.SetTenantServiceDB(1001, "evidence-command", db1)
	app.SetTenantServiceDB(1001, "evidence-query", db2)
	app.SetTenantServiceDB(1001, "security-management", db3)

	// Verify each service returns correct DB
	result := app.GetTenantServiceDB(1001, "evidence-command")
	assert.Same(t, db1, result)

	result = app.GetTenantServiceDB(1001, "evidence-query")
	assert.Same(t, db2, result)

	result = app.GetTenantServiceDB(1001, "security-management")
	assert.Same(t, db3, result)

	// Verify iteration
	count := 0
	app.GetTenantServiceDBs(func(tenantID int, serviceCode string, db *gorm.DB) bool {
		count++
		assert.Equal(t, 1001, tenantID)
		return true
	})
	assert.Equal(t, 3, count)
}
