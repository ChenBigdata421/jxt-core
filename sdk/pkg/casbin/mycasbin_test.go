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

// TestSetupForTenant_ErrorHandling verifies that SetupForTenant returns
// proper errors when given invalid inputs
func TestSetupForTenant_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		db          *gorm.DB
		tenantID    int
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil database should return error",
			db:          nil,
			tenantID:    1,
			wantErr:     true,
			errContains: "adapter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enforcer, err := SetupForTenant(tt.db, tt.tenantID)

			if tt.wantErr {
				assert.Error(t, err, "SetupForTenant 应返回错误")
				assert.Nil(t, enforcer, "出错时 enforcer 应为 nil")
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains,
						"错误信息应包含: "+tt.errContains)
				}
			} else {
				assert.NoError(t, err, "SetupForTenant 不应返回错误")
				assert.NotNil(t, enforcer, "enforcer 不应为 nil")
			}
		})
	}
}

// TestSetup_BackwardCompatibility verifies that the old Setup function
// still works for existing code
func TestSetup_BackwardCompatibility(t *testing.T) {
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

	// Setup 应该仍然工作（不 panic）
	enforcer := Setup(db, "")
	assert.NotNil(t, enforcer, "Setup 应返回非空 enforcer")
}

// TestSetupForTenant_PolicyLoading verifies that policies are loaded
// correctly from the database
func TestSetupForTenant_PolicyLoading(t *testing.T) {
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

	enforcer, err := SetupForTenant(db, 1)
	assert.NoError(t, err, "SetupForTenant 不应返回错误")
	assert.NotNil(t, enforcer, "enforcer 不应为 nil")

	// 验证 enforcer 已正确初始化
	// GetAllSubjects() 返回所有定义的主体（用户）
	subjects := enforcer.GetAllSubjects()
	// 即使没有策略，也应该返回一个空切片而不是 nil
	assert.NotNil(t, subjects, "GetAllSubjects 应返回非空切片")

	// 验证 enforcer 的模型已正确加载
	model := enforcer.GetModel()
	assert.NotNil(t, model, "模型不应为 nil")
}
