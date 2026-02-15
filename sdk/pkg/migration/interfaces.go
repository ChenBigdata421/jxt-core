// pkg/migration/interfaces.go
package migration

import "gorm.io/gorm"

// MigrationFunc 迁移函数签名
type MigrationFunc func(db *gorm.DB, version string) error

// Registry 迁移注册表接口
type Registry interface {
	// SetTenantDb 设置多租户数据库连接
	SetTenantDb(tenantID int, db *gorm.DB)

	// MigrateTenant 执行指定租户的迁移
	MigrateTenant(tenantID int) error

	// RegisterVersion 注册迁移版本
	RegisterVersion(version string, fn MigrationFunc)

	// GetRegisteredVersions 获取所有已注册版本
	GetRegisteredVersions() []string
}
