package mycasbin

import (
	"testing"

	_ "github.com/casbin/casbin/v2" // Imported for type use in tests
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
