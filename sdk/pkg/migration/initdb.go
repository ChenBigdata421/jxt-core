// pkg/migration/initdb.go
package migration

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"
)

// InitDbConfig SQL 执行器配置
type InitDbConfig struct {
	Driver      string   // 数据库驱动: mysql, postgres, sqlite3
	SQLFiles    []string // SQL 文件路径（按执行顺序）
	StopOnError bool     // 遇到错误是否停止
}

// InitDb 执行 SQL 初始化脚本
func InitDb(db *gorm.DB, config InitDbConfig) error {
	for _, sqlFile := range config.SQLFiles {
		if _, err := os.Stat(sqlFile); os.IsNotExist(err) {
			continue // 文件不存在则跳过
		}

		if err := executeSQLFile(db, sqlFile, config.StopOnError); err != nil {
			return fmt.Errorf("执行 SQL 文件 %s 失败: %w", sqlFile, err)
		}
	}
	return nil
}

// executeSQLFile 执行单个 SQL 文件
func executeSQLFile(db *gorm.DB, filePath string, stopOnError bool) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var statement strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过注释和空行
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}

		statement.WriteString(line)
		statement.WriteString(" ")

		// 遇到分号则执行语句
		if strings.HasSuffix(line, ";") {
			sql := strings.TrimSpace(statement.String())
			if sql != "" && sql != ";" {
				if err := db.Exec(sql).Error; err != nil && stopOnError {
					return err
				}
			}
			statement.Reset()
		}
	}

	return scanner.Err()
}

// DefaultSQLFiles 根据数据库驱动返回默认 SQL 文件列表
func DefaultSQLFiles(driver, configDir string) []string {
	switch driver {
	case "mysql":
		return []string{
			filepath.Join(configDir, "db-begin-mysql.sql"),
			filepath.Join(configDir, "db.sql"),
			filepath.Join(configDir, "db-end-mysql.sql"),
		}
	case "postgres":
		return []string{
			filepath.Join(configDir, "db.sql"),
			filepath.Join(configDir, "pg.sql"),
		}
	default:
		return []string{
			filepath.Join(configDir, "db.sql"),
		}
	}
}
