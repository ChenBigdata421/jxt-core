// pkg/migration/registry_test.go
package migration

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestGetRegistrySingleton(t *testing.T) {
	r1 := GetRegistry()
	r2 := GetRegistry()

	assert.Same(t, r1, r2, "GetRegistry should return the same instance")
}

func TestRegisterVersion(t *testing.T) {
	r := &MigrationRegistry{
		migrations: make(map[int]*TenantMigration),
		versions:   make(map[string]MigrationFunc),
	}

	called := false
	fn := func(db *gorm.DB, version string) error {
		called = true
		return nil
	}

	r.RegisterVersion("1234567890123", fn)

	assert.Contains(t, r.versions, "1234567890123")
	assert.False(t, called, "Function should not be called during registration")
}

func TestGetRegisteredVersions(t *testing.T) {
	r := &MigrationRegistry{
		migrations: make(map[int]*TenantMigration),
		versions:   make(map[string]MigrationFunc),
	}

	r.RegisterVersion("9999999999999", nil)
	r.RegisterVersion("1111111111111", nil)
	r.RegisterVersion("5555555555555", nil)

	versions := r.GetRegisteredVersions()

	assert.Equal(t, []string{"1111111111111", "5555555555555", "9999999999999"}, versions)
}

func TestSetTenantDb(t *testing.T) {
	r := &MigrationRegistry{
		migrations: make(map[int]*TenantMigration),
		versions:   make(map[string]MigrationFunc),
	}

	r.SetTenantDb(1, nil)

	assert.Contains(t, r.migrations, 1)
}

func TestConcurrentTenantCreation(t *testing.T) {
	r := &MigrationRegistry{
		migrations: make(map[int]*TenantMigration),
		versions:   make(map[string]MigrationFunc),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r.SetTenantDb(id, nil)
		}(i)
	}
	wg.Wait()

	assert.Len(t, r.migrations, 100)
}
