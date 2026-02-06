package mycasbin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// realTestDB connects to a test database (requires TEST_DB_URL environment variable)
// For CI/CD, set up a test PostgreSQL database
func realTestDB(t *testing.T) *gorm.DB {
	dsn := "host=localhost user=postgres password=postgres dbname=postgres_test port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("无法连接到测试数据库: %v (跳过集成测试)", err)
		return nil
	}
	return db
}

// TestSetupForTenant_IndependentInstances verifies that calling SetupForTenant
// with different database connections creates different enforcer instances
func TestSetupForTenant_IndependentInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	db := realTestDB(t)
	if db == nil {
		return
	}
	defer func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	}()

	// 注意: 由于我们使用相同的数据库连接，adapter 会相同
	// 但 enforcer 实例应该是不同的
	enforcer1, err := SetupForTenant(db, 1)
	assert.NoError(t, err, "SetupForTenant(租户1) 不应返回错误")
	assert.NotNil(t, enforcer1, "SetupForTenant(租户1) 应返回非空 enforcer")

	enforcer2, err := SetupForTenant(db, 2)
	assert.NoError(t, err, "SetupForTenant(租户2) 不应返回错误")
	assert.NotNil(t, enforcer2, "SetupForTenant(租户2) 应返回非空 enforcer")

	// 关键验证: 两个 enforcer 应该是不同的实例
	assert.NotSame(t, enforcer1, enforcer2, "不同租户的 enforcer 应该是不同的实例")
}
