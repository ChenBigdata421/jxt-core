// pkg/migration/migration.go
package migration

import "time"

// Migration 迁移版本记录表
type Migration struct {
	Version   string    `gorm:"primaryKey;size:64"`
	ApplyTime time.Time `gorm:"autoCreateTime"`
}

func (Migration) TableName() string {
	return "sys_migration"
}
