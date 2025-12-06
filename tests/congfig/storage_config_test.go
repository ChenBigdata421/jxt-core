package congfig

import (
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
)

// TestStorageConfig_GetStorageSiteNo 测试获取存储站点标识
func TestStorageConfig_GetStorageSiteNo(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.StorageConfig
		expected string
	}{
		{
			name:     "nil 配置返回默认值",
			config:   nil,
			expected: "main",
		},
		{
			name:     "空字符串返回默认值",
			config:   &config.StorageConfig{StorageSiteNo: ""},
			expected: "main",
		},
		{
			name:     "返回配置的值",
			config:   &config.StorageConfig{StorageSiteNo: "site-001"},
			expected: "site-001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetStorageSiteNo()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStorageConfig_GetRootPath 测试获取根存储路径
func TestStorageConfig_GetRootPath(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.StorageConfig
		expected string
	}{
		{
			name:     "nil 配置返回默认值",
			config:   nil,
			expected: "./uploads/ftp",
		},
		{
			name:     "空字符串返回默认值",
			config:   &config.StorageConfig{RootPath: ""},
			expected: "./uploads/ftp",
		},
		{
			name:     "返回配置的值",
			config:   &config.StorageConfig{RootPath: "/custom/path"},
			expected: "/custom/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetRootPath()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStorageConfig_GetTempPath 测试获取临时文件路径
func TestStorageConfig_GetTempPath(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.StorageConfig
		expected string
	}{
		{
			name:     "nil 配置返回默认值",
			config:   nil,
			expected: "../tmp/ftp",
		},
		{
			name:     "空字符串返回默认值",
			config:   &config.StorageConfig{TempPath: ""},
			expected: "../tmp/ftp",
		},
		{
			name:     "返回配置的值",
			config:   &config.StorageConfig{TempPath: "/custom/tmp"},
			expected: "/custom/tmp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetTempPath()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStorageConfig_AllFields 测试配置所有字段
func TestStorageConfig_AllFields(t *testing.T) {
	cfg := &config.StorageConfig{
		StorageSiteNo: "warehouse-a",
		RootPath:      "/data/storage",
		TempPath:      "/data/tmp",
	}

	assert.Equal(t, "warehouse-a", cfg.GetStorageSiteNo())
	assert.Equal(t, "/data/storage", cfg.GetRootPath())
	assert.Equal(t, "/data/tmp", cfg.GetTempPath())
}

