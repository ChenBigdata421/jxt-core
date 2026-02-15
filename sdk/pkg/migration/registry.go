// pkg/migration/registry.go
package migration

import (
	"fmt"
	"log"
	"sort"
	"sync"

	"gorm.io/gorm"
)

// TenantMigration 单个租户的迁移上下文
type TenantMigration struct {
	tenantID  int
	db        *gorm.DB
	completed map[string]bool // 该租户已完成的迁移版本（内存缓存）
	mutex     sync.Mutex      // 保护该租户的迁移操作
}

// MigrationRegistry 多租户迁移注册表
type MigrationRegistry struct {
	migrations   map[int]*TenantMigration  // 租户ID → 迁移上下文
	versions     map[string]MigrationFunc  // 版本号 → 迁移函数
	registryLock sync.RWMutex              // 保护 migrations map
}

var (
	globalRegistry *MigrationRegistry
	once           sync.Once
)

// GetRegistry 获取全局注册表单例
func GetRegistry() *MigrationRegistry {
	once.Do(func() {
		globalRegistry = &MigrationRegistry{
			migrations: make(map[int]*TenantMigration),
			versions:   make(map[string]MigrationFunc),
		}
	})
	return globalRegistry
}

// RegisterVersion 注册迁移版本（由 init() 调用）
func (r *MigrationRegistry) RegisterVersion(version string, fn MigrationFunc) {
	r.registryLock.Lock()
	defer r.registryLock.Unlock()
	r.versions[version] = fn
}

// GetRegisteredVersions 获取所有已注册版本（排序后）
func (r *MigrationRegistry) GetRegisteredVersions() []string {
	r.registryLock.RLock()
	defer r.registryLock.RUnlock()

	versions := make([]string, 0, len(r.versions))
	for v := range r.versions {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	return versions
}

// SetTenantDb 设置租户数据库连接
func (r *MigrationRegistry) SetTenantDb(tenantID int, db *gorm.DB) {
	r.registryLock.Lock()
	defer r.registryLock.Unlock()

	tm := r.getOrCreateTenantLocked(tenantID)
	tm.db = db
}

// getOrCreateTenantLocked 获取或创建租户迁移上下文（需持有 registryLock）
func (r *MigrationRegistry) getOrCreateTenantLocked(tenantID int) *TenantMigration {
	tm, exists := r.migrations[tenantID]
	if !exists {
		tm = &TenantMigration{
			tenantID:  tenantID,
			completed: make(map[string]bool),
		}
		r.migrations[tenantID] = tm
	}
	return tm
}

// MigrateTenant 执行指定租户的迁移
func (r *MigrationRegistry) MigrateTenant(tenantID int) error {
	r.registryLock.RLock()
	tm, exists := r.migrations[tenantID]
	r.registryLock.RUnlock()

	if !exists || tm.db == nil {
		return fmt.Errorf("租户 %d 的数据库连接未设置", tenantID)
	}

	// 使用租户级别的锁，保证同一租户的迁移串行执行
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 1. 确保 sys_migration 表存在
	if err := tm.db.AutoMigrate(&Migration{}); err != nil {
		return fmt.Errorf("租户 %d 创建 sys_migration 表失败: %w", tenantID, err)
	}

	// 2. 获取已应用的版本（内存 + DB 双重检查）
	applied, err := r.getAppliedVersions(tm)
	if err != nil {
		return err
	}

	// 3. 按版本号排序执行
	versions := r.GetRegisteredVersions()
	for _, version := range versions {
		// 内存缓存检查
		if tm.completed[version] {
			log.Printf("[migration] 租户 %d: 版本 %s 已在内存中标记完成，跳过", tenantID, version)
			continue
		}

		// DB 检查
		if applied[version] {
			tm.completed[version] = true // 同步到内存缓存
			log.Printf("[migration] 租户 %d: 版本 %s 已在 DB 中标记完成，跳过", tenantID, version)
			continue
		}

		// 执行迁移
		fn := r.versions[version]
		if err := r.executeMigration(tm, version, fn); err != nil {
			return fmt.Errorf("租户 %d 版本 %s 迁移失败: %w", tenantID, version, err)
		}

		// 标记完成
		tm.completed[version] = true
		log.Printf("[migration] 租户 %d: 版本 %s 迁移完成", tenantID, version)
	}

	return nil
}

// getAppliedVersions 获取已应用的迁移版本
func (r *MigrationRegistry) getAppliedVersions(tm *TenantMigration) (map[string]bool, error) {
	var records []Migration
	if err := tm.db.Find(&records).Error; err != nil {
		return nil, err
	}

	applied := make(map[string]bool)
	for _, record := range records {
		applied[record.Version] = true
	}
	return applied, nil
}

// executeMigration 执行单个迁移
func (r *MigrationRegistry) executeMigration(tm *TenantMigration, version string, fn MigrationFunc) error {
	return tm.db.Transaction(func(tx *gorm.DB) error {
		// 执行迁移函数
		if err := fn(tx, version); err != nil {
			return err
		}

		// 记录版本到 DB
		return tx.Create(&Migration{Version: version}).Error
	})
}
