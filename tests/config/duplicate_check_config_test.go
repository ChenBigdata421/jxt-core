package config

import (
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
)

// TestDuplicateCheckConfig_IsEnabled 测试是否启用去重检查
func TestDuplicateCheckConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.DuplicateCheckConfig
		expected bool
	}{
		{
			name:     "nil 配置返回 false",
			config:   nil,
			expected: false,
		},
		{
			name:     "未启用返回 false",
			config:   &config.DuplicateCheckConfig{Enabled: false},
			expected: false,
		},
		{
			name:     "启用返回 true",
			config:   &config.DuplicateCheckConfig{Enabled: true},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsEnabled()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDuplicateCheckConfig_GetStrategy 测试获取去重策略
func TestDuplicateCheckConfig_GetStrategy(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.DuplicateCheckConfig
		expected string
	}{
		{
			name:     "nil 配置返回默认值 filename",
			config:   nil,
			expected: "filename",
		},
		{
			name:     "空字符串返回默认值 filename",
			config:   &config.DuplicateCheckConfig{Strategy: ""},
			expected: "filename",
		},
		{
			name:     "返回配置的 content 策略",
			config:   &config.DuplicateCheckConfig{Strategy: "content"},
			expected: "content",
		},
		{
			name:     "返回配置的 hybrid 策略",
			config:   &config.DuplicateCheckConfig{Strategy: "hybrid"},
			expected: "hybrid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetStrategy()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDuplicateCheckConfig_GetFilenameConfig 测试获取文件名配置
func TestDuplicateCheckConfig_GetFilenameConfig(t *testing.T) {
	t.Run("nil 配置返回默认值", func(t *testing.T) {
		var cfg *config.DuplicateCheckConfig
		result := cfg.GetFilenameConfig()

		assert.NotNil(t, result)
		assert.False(t, result.CaseSensitive)
		assert.Equal(t, "prefix", result.MatchMode)
	})

	t.Run("nil Filename 返回默认值", func(t *testing.T) {
		cfg := &config.DuplicateCheckConfig{Filename: nil}
		result := cfg.GetFilenameConfig()

		assert.NotNil(t, result)
		assert.False(t, result.CaseSensitive)
		assert.Equal(t, "prefix", result.MatchMode)
	})

	t.Run("返回配置的值", func(t *testing.T) {
		cfg := &config.DuplicateCheckConfig{
			Filename: &config.DuplicateFilenameConfig{
				CaseSensitive: true,
				MatchMode:     "exact",
			},
		}
		result := cfg.GetFilenameConfig()

		assert.True(t, result.CaseSensitive)
		assert.Equal(t, "exact", result.MatchMode)
	})
}

// TestDuplicateCheckConfig_GetContentConfig 测试获取内容配置
func TestDuplicateCheckConfig_GetContentConfig(t *testing.T) {
	t.Run("nil 配置返回默认值", func(t *testing.T) {
		var cfg *config.DuplicateCheckConfig
		result := cfg.GetContentConfig()

		assert.NotNil(t, result)
		assert.Equal(t, "sha1", result.Algorithm)
	})

	t.Run("nil Content 返回默认值", func(t *testing.T) {
		cfg := &config.DuplicateCheckConfig{Content: nil}
		result := cfg.GetContentConfig()

		assert.NotNil(t, result)
		assert.Equal(t, "sha1", result.Algorithm)
	})

	t.Run("返回配置的值", func(t *testing.T) {
		cfg := &config.DuplicateCheckConfig{
			Content: &config.DuplicateContentConfig{
				Algorithm: "sha256",
			},
		}
		result := cfg.GetContentConfig()

		assert.Equal(t, "sha256", result.Algorithm)
	})
}

