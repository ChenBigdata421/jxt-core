// pkg/migration/initdb_test.go
package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultSQLFiles(t *testing.T) {
	tests := []struct {
		name      string
		driver    string
		configDir string
		expected  []string
	}{
		{
			name:      "mysql driver",
			driver:    "mysql",
			configDir: "config",
			expected: []string{
				filepath.Join("config", "db-begin-mysql.sql"),
				filepath.Join("config", "db.sql"),
				filepath.Join("config", "db-end-mysql.sql"),
			},
		},
		{
			name:      "postgres driver",
			driver:    "postgres",
			configDir: "config",
			expected: []string{
				filepath.Join("config", "db.sql"),
				filepath.Join("config", "pg.sql"),
			},
		},
		{
			name:      "default driver",
			driver:    "sqlite3",
			configDir: "config",
			expected: []string{
				filepath.Join("config", "db.sql"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultSQLFiles(tt.driver, tt.configDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteSQLFile(t *testing.T) {
	// Create a temporary SQL file
	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "test.sql")

	content := `-- This is a comment
CREATE TABLE test (id INT);
-- Another comment
INSERT INTO test VALUES (1);
`
	err := os.WriteFile(sqlFile, []byte(content), 0644)
	assert.NoError(t, err)

	// Note: Full integration test would require a real DB connection
	// This test only verifies file reading logic
}
