# Multi FTP Config Per Tenant Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Support multiple FTP configurations per tenant in jxt-core config structure, aligning with tenant-service database model changes.

**Architecture:**
- Replace single `FTP` field with `FTPConfigs` array in `DefaultTenantConfig`
- Add `Description` and `Status` fields to `TenantFTPDetailConfig`
- Provide helper methods for querying FTP configs by status and username

**Tech Stack:** Go 1.23+, YAML configuration, unit testing with Go testing

---

### Task 1: Update TenantFTPDetailConfig struct with new fields

**Files:**
- Modify: `sdk/config/tenant.go:78-82`

**Step 1: Read current struct definition**

Run: `code sdk/config/tenant.go:78-82`
Expected: View current `TenantFTPDetailConfig` struct

**Step 2: Add new fields to TenantFTPDetailConfig**

```go
// TenantFTPDetailConfig 详细的 FTP 配置
type TenantFTPDetailConfig struct {
	Username        string `mapstructure:"username" yaml:"username"`                 // FTP 用户名
	InitialPassword string `mapstructure:"initial_password" yaml:"initial_password"` // 初始密码（首次创建时使用）
	Description     string `mapstructure:"description" yaml:"description"`           // FTP 配置描述（用于标识不同 FTP）
	Status          string `mapstructure:"status" yaml:"status"`                     // 状态: active, inactive
}
```

**Step 3: Add getter methods for new fields**

Add after `GetFTPInitialPassword()` method (around line 425):

```go
// GetFTPDescription 获取 FTP 配置描述
func (ftpConfig *TenantFTPDetailConfig) GetFTPDescription() string {
	if ftpConfig == nil {
		return ""
	}
	return ftpConfig.Description
}

// GetFTPStatus 获取 FTP 状态
func (ftpConfig *TenantFTPDetailConfig) GetFTPStatus() string {
	if ftpConfig == nil || ftpConfig.Status == "" {
		return "active" // 默认为 active
	}
	return ftpConfig.Status
}

// IsFTPActive 判断 FTP 配置是否处于 active 状态
func (ftpConfig *TenantFTPDetailConfig) IsFTPActive() bool {
	return ftpConfig.GetFTPStatus() == "active"
}
```

**Step 4: Verify Go syntax**

Run: `go build ./sdk/config/...`
Expected: No errors

**Step 5: Commit**

```bash
git add sdk/config/tenant.go
git commit -m "feat(config): add Description and Status fields to TenantFTPDetailConfig"
```

---

### Task 2: Replace FTP field with FTPConfigs array in DefaultTenantConfig

**Files:**
- Modify: `sdk/config/tenant.go:40-51`

**Step 1: Remove old FTP field and add FTPConfigs array**

Replace the `FTP` field:

```go
// DefaultTenantConfig 默认租户配置
type DefaultTenantConfig struct {
	// Legacy: 单个数据库配置（向后兼容）
	Database *TenantDatabaseDetailConfig `mapstructure:"database" yaml:"database"` // 数据库配置

	// NEW: 服务级数据库配置映射（service_code -> config）
	ServiceDatabases map[string]TenantDatabaseDetailConfig `mapstructure:"service_databases" yaml:"service_databases"` // 服务级数据库配置

	Domain  *TenantDomainConfig        `mapstructure:"domain" yaml:"domain"`   // 域名配置
	FTPConfigs []TenantFTPDetailConfig `mapstructure:"ftp_configs" yaml:"ftp_configs"` // FTP 配置数组
	Storage *TenantStorageDetailConfig `mapstructure:"storage" yaml:"storage"` // 存储配置
}
```

**Step 2: Remove old GetDefaultTenantFTP method**

Delete the `GetDefaultTenantFTP()` method (around lines 204-210):

```go
// DELETE THIS METHOD
// GetDefaultTenantFTP 获取默认租户 FTP 配置
func (dtc *DefaultTenantConfig) GetDefaultTenantFTP() *TenantFTPDetailConfig {
	if dtc == nil || dtc.FTP == nil {
		return nil
	}
	return dtc.FTP
}
```

**Step 3: Verify Go syntax**

Run: `go build ./sdk/config/...`
Expected: No errors

**Step 4: Commit**

```bash
git add sdk/config/tenant.go
git commit -m "refactor(config): replace FTP field with FTPConfigs array"
```

---

### Task 3: Add helper methods for FTPConfigs array

**Files:**
- Modify: `sdk/config/tenant.go` (after line 210, where old method was removed)

**Step 1: Add GetFTPConfigs method**

```go
// GetFTPConfigs 获取所有 FTP 配置数组
func (dtc *DefaultTenantConfig) GetFTPConfigs() []TenantFTPDetailConfig {
	if dtc == nil || dtc.FTPConfigs == nil {
		return []TenantFTPDetailConfig{}
	}
	return dtc.FTPConfigs
}
```

**Step 2: Add GetActiveFTPConfigs method**

```go
// GetActiveFTPConfigs 获取所有状态为 active 的 FTP 配置
func (dtc *DefaultTenantConfig) GetActiveFTPConfigs() []TenantFTPDetailConfig {
	if dtc == nil || dtc.FTPConfigs == nil {
		return []TenantFTPDetailConfig{}
	}

	result := make([]TenantFTPDetailConfig, 0)
	for _, cfg := range dtc.FTPConfigs {
		if cfg.Status == "" || cfg.Status == "active" {
			result = append(result, cfg)
		}
	}
	return result
}
```

**Step 3: Add GetFTPConfigByUsername method**

```go
// GetFTPConfigByUsername 根据 Username 查找特定的 FTP 配置
func (dtc *DefaultTenantConfig) GetFTPConfigByUsername(username string) *TenantFTPDetailConfig {
	if dtc == nil || dtc.FTPConfigs == nil {
		return nil
	}

	for i := range dtc.FTPConfigs {
		if dtc.FTPConfigs[i].Username == username {
			return &dtc.FTPConfigs[i]
		}
	}
	return nil
}
```

**Step 4: Add HasFTPConfigs method**

```go
// HasFTPConfigs 判断是否配置了多个 FTP
func (dtc *DefaultTenantConfig) HasFTPConfigs() bool {
	return dtc != nil && len(dtc.FTPConfigs) > 0
}
```

**Step 5: Verify Go syntax**

Run: `go build ./sdk/config/...`
Expected: No errors

**Step 6: Commit**

```bash
git add sdk/config/tenant.go
git commit -m "feat(config): add helper methods for FTPConfigs array"
```

---

### Task 4: Update YAML configuration example comment

**Files:**
- Modify: `sdk/config/tenant.go:452-548` (comment section at end of file)

**Step 1: Update the FTP configuration example**

Replace the old `ftp:` section with new `ftp_configs:` array:

```yaml
    # FTP 配置数组（支持多个 FTP）
    ftp_configs:
      - username: "tenant1_sales_ftp"
        initial_password: "Sales@123456"
        description: "销售部 FTP"
        status: "active"

      - username: "tenant1_hr_ftp"
        initial_password: "HR@123456"
        description: "人事部 FTP"
        status: "active"

      - username: "tenant1_finance_ftp"
        initial_password: "Finance@123456"
        description: "财务部 FTP（临时停用）"
        status: "inactive"
```

**Step 2: Verify file is valid Go**

Run: `go build ./sdk/config/...`
Expected: No errors

**Step 3: Commit**

```bash
git add sdk/config/tenant.go
git commit -m "docs(config): update YAML example with ftp_configs array"
```

---

### Task 5: Write unit tests for new FTP config methods

**Files:**
- Create: `sdk/config/tenant_ftp_test.go`

**Step 1: Create test file with test structure**

```go
package config

import (
	"testing"
)

func TestTenantFTPDetailConfig_GetFTPDescription(t *testing.T) {
	tests := []struct {
		name     string
		config   *TenantFTPDetailConfig
		expected string
	}{
		{
			name:     "nil config returns empty",
			config:   nil,
			expected: "",
		},
		{
			name: "config with description returns description",
			config: &TenantFTPDetailConfig{
				Description: "销售部 FTP",
			},
			expected: "销售部 FTP",
		},
		{
			name:     "config without description returns empty",
			config:   &TenantFTPDetailConfig{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetFTPDescription()
			if result != tt.expected {
				t.Errorf("GetFTPDescription() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTenantFTPDetailConfig_GetFTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		config   *TenantFTPDetailConfig
		expected string
	}{
		{
			name:     "nil config returns active default",
			config:   nil,
			expected: "active",
		},
		{
			name:     "empty status returns active default",
			config:   &TenantFTPDetailConfig{},
			expected: "active",
		},
		{
			name: "config with active status",
			config: &TenantFTPDetailConfig{
				Status: "active",
			},
			expected: "active",
		},
		{
			name: "config with inactive status",
			config: &TenantFTPDetailConfig{
				Status: "inactive",
			},
			expected: "inactive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetFTPStatus()
			if result != tt.expected {
				t.Errorf("GetFTPStatus() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTenantFTPDetailConfig_IsFTPActive(t *testing.T) {
	tests := []struct {
		name     string
		config   *TenantFTPDetailConfig
		expected bool
	}{
		{
			name:     "nil config returns true (default active)",
			config:   nil,
			expected: true,
		},
		{
			name:     "empty status returns true (default active)",
			config:   &TenantFTPDetailConfig{},
			expected: true,
		},
		{
			name: "active status returns true",
			config: &TenantFTPDetailConfig{
				Status: "active",
			},
			expected: true,
		},
		{
			name: "inactive status returns false",
			config: &TenantFTPDetailConfig{
				Status: "inactive",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsFTPActive()
			if result != tt.expected {
				t.Errorf("IsFTPActive() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDefaultTenantConfig_GetFTPConfigs(t *testing.T) {
	tests := []struct {
		name     string
		config   *DefaultTenantConfig
		expected int
	}{
		{
			name:     "nil config returns empty slice",
			config:   nil,
			expected: 0,
		},
		{
			name:     "config without FTPConfigs returns empty slice",
			config:   &DefaultTenantConfig{},
			expected: 0,
		},
		{
			name: "config with FTPConfigs returns all",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1"},
					{Username: "ftp2"},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetFTPConfigs()
			if len(result) != tt.expected {
				t.Errorf("GetFTPConfigs() len = %v, want %v", len(result), tt.expected)
			}
		})
	}
}

func TestDefaultTenantConfig_GetActiveFTPConfigs(t *testing.T) {
	tests := []struct {
		name     string
		config   *DefaultTenantConfig
		expected int
	}{
		{
			name:     "nil config returns empty slice",
			config:   nil,
			expected: 0,
		},
		{
			name:     "config without FTPConfigs returns empty slice",
			config:   &DefaultTenantConfig{},
			expected: 0,
		},
		{
			name: "filters only active configs",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1", Status: "active"},
					{Username: "ftp2", Status: "inactive"},
					{Username: "ftp3"}, // empty status = active
				},
			},
			expected: 2, // ftp1 and ftp3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetActiveFTPConfigs()
			if len(result) != tt.expected {
				t.Errorf("GetActiveFTPConfigs() len = %v, want %v", len(result), tt.expected)
			}
		})
	}
}

func TestDefaultTenantConfig_GetFTPConfigByUsername(t *testing.T) {
	config := &DefaultTenantConfig{
		FTPConfigs: []TenantFTPDetailConfig{
			{Username: "ftp1"},
			{Username: "ftp2"},
		},
	}

	t.Run("finds existing username", func(t *testing.T) {
		result := config.GetFTPConfigByUsername("ftp1")
		if result == nil {
			t.Errorf("GetFTPConfigByUsername() returned nil for existing username")
		}
		if result.Username != "ftp1" {
			t.Errorf("GetFTPConfigByUsername() = %v, want ftp1", result.Username)
		}
	})

	t.Run("returns nil for non-existent username", func(t *testing.T) {
		result := config.GetFTPConfigByUsername("nonexistent")
		if result != nil {
			t.Errorf("GetFTPConfigByUsername() returned non-nil for non-existent username")
		}
	})

	t.Run("returns nil for nil config", func(t *testing.T) {
		var config *DefaultTenantConfig
		result := config.GetFTPConfigByUsername("ftp1")
		if result != nil {
			t.Errorf("GetFTPConfigByUsername() returned non-nil for nil config")
		}
	})
}

func TestDefaultTenantConfig_HasFTPConfigs(t *testing.T) {
	tests := []struct {
		name     string
		config   *DefaultTenantConfig
		expected bool
	}{
		{
			name:     "nil config returns false",
			config:   nil,
			expected: false,
		},
		{
			name:     "empty FTPConfigs returns false",
			config:   &DefaultTenantConfig{},
			expected: false,
		},
		{
			name: "non-empty FTPConfigs returns true",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.HasFTPConfigs()
			if result != tt.expected {
				t.Errorf("HasFTPConfigs() = %v, want %v", result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run tests to verify they pass**

Run: `go test -v ./sdk/config/... -run "TestTenant.*FTP"`
Expected: All tests PASS

**Step 3: Commit**

```bash
git add sdk/config/tenant_ftp_test.go
git commit -m "test(config): add unit tests for FTP config methods"
```

---

### Task 6: Run full test suite to ensure no regressions

**Files:**
- Test: `sdk/config/...`

**Step 1: Run all config tests**

Run: `go test -v ./sdk/config/...`
Expected: All tests PASS

**Step 2: Run all sdk tests**

Run: `go test -v ./sdk/...`
Expected: All tests PASS

**Step 3: Verify build**

Run: `go build ./sdk/...`
Expected: No errors

**Step 4: Commit (if any additional fixes needed)**

```bash
git add -A
git commit -m "fix(config): address any issues found in testing"
```

---

## Summary

This plan refactors the tenant config structure to support multiple FTP configurations per tenant:

1. Added `Description` and `Status` fields to `TenantFTPDetailConfig`
2. Replaced single `FTP` field with `FTPConfigs` array
3. Added helper methods: `GetFTPConfigs()`, `GetActiveFTPConfigs()`, `GetFTPConfigByUsername()`, `HasFTPConfigs()`
4. Added getter methods: `GetFTPDescription()`, `GetFTPStatus()`, `IsFTPActive()`
5. Updated YAML configuration example
6. Added comprehensive unit tests

**No backward compatibility needed** - project is not yet in production.
