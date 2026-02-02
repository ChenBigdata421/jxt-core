package config

import (
	"testing"
)

// TestTenantFTPDetailConfig_GetFTPDescription tests the GetFTPDescription method
func TestTenantFTPDetailConfig_GetFTPDescription(t *testing.T) {
	tests := []struct {
		name     string
		config   *TenantFTPDetailConfig
		expected string
	}{
		{
			name:     "nil config should return empty string",
			config:   nil,
			expected: "",
		},
		{
			name:     "empty description should return empty string",
			config:   &TenantFTPDetailConfig{Description: ""},
			expected: "",
		},
		{
			name:     "populated description should return description",
			config:   &TenantFTPDetailConfig{Description: "Sales Department FTP"},
			expected: "Sales Department FTP",
		},
		{
			name:     "description with special characters",
			config:   &TenantFTPDetailConfig{Description: "销售部 FTP (临时停用)"},
			expected: "销售部 FTP (临时停用)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetFTPDescription()
			if result != tt.expected {
				t.Errorf("GetFTPDescription() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestTenantFTPDetailConfig_GetFTPStatus tests the GetFTPStatus method
func TestTenantFTPDetailConfig_GetFTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		config   *TenantFTPDetailConfig
		expected string
	}{
		{
			name:     "nil config should return default active",
			config:   nil,
			expected: "active",
		},
		{
			name:     "empty status should return default active",
			config:   &TenantFTPDetailConfig{Status: ""},
			expected: "active",
		},
		{
			name:     "active status should return active",
			config:   &TenantFTPDetailConfig{Status: "active"},
			expected: "active",
		},
		{
			name:     "inactive status should return inactive",
			config:   &TenantFTPDetailConfig{Status: "inactive"},
			expected: "inactive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetFTPStatus()
			if result != tt.expected {
				t.Errorf("GetFTPStatus() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestTenantFTPDetailConfig_IsFTPActive tests the IsFTPActive method
func TestTenantFTPDetailConfig_IsFTPActive(t *testing.T) {
	tests := []struct {
		name     string
		config   *TenantFTPDetailConfig
		expected bool
	}{
		{
			name:     "nil config should be active (default)",
			config:   nil,
			expected: true,
		},
		{
			name:     "empty status should be active (default)",
			config:   &TenantFTPDetailConfig{Status: ""},
			expected: true,
		},
		{
			name:     "active status should be true",
			config:   &TenantFTPDetailConfig{Status: "active"},
			expected: true,
		},
		{
			name:     "inactive status should be false",
			config:   &TenantFTPDetailConfig{Status: "inactive"},
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

// TestDefaultTenantConfig_GetFTPConfigs tests the GetFTPConfigs method
func TestDefaultTenantConfig_GetFTPConfigs(t *testing.T) {
	tests := []struct {
		name     string
		config   *DefaultTenantConfig
		expected int
	}{
		{
			name:     "nil config should return empty slice",
			config:   nil,
			expected: 0,
		},
		{
			name:     "nil FTPConfigs should return empty slice",
			config:   &DefaultTenantConfig{FTPConfigs: nil},
			expected: 0,
		},
		{
			name: "empty FTPConfigs should return empty slice",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{},
			},
			expected: 0,
		},
		{
			name: "populated FTPConfigs should return all configs",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1", Description: "FTP 1"},
					{Username: "ftp2", Description: "FTP 2"},
					{Username: "ftp3", Description: "FTP 3"},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetFTPConfigs()
			if len(result) != tt.expected {
				t.Errorf("GetFTPConfigs() length = %d, want %d", len(result), tt.expected)
			}
		})
	}
}

// TestDefaultTenantConfig_GetActiveFTPConfigs tests the GetActiveFTPConfigs method
func TestDefaultTenantConfig_GetActiveFTPConfigs(t *testing.T) {
	tests := []struct {
		name     string
		config   *DefaultTenantConfig
		expected int
	}{
		{
			name:     "nil config should return empty slice",
			config:   nil,
			expected: 0,
		},
		{
			name:     "nil FTPConfigs should return empty slice",
			config:   &DefaultTenantConfig{FTPConfigs: nil},
			expected: 0,
		},
		{
			name: "empty FTPConfigs should return empty slice",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{},
			},
			expected: 0,
		},
		{
			name: "all active configs should return all",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1", Status: "active"},
					{Username: "ftp2", Status: "active"},
				},
			},
			expected: 2,
		},
		{
			name: "empty status defaults to active",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1", Status: ""},
					{Username: "ftp2", Status: "active"},
				},
			},
			expected: 2,
		},
		{
			name: "mixed statuses should filter only active",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1", Status: "active"},
					{Username: "ftp2", Status: "inactive"},
					{Username: "ftp3", Status: "active"},
					{Username: "ftp4", Status: "inactive"},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetActiveFTPConfigs()
			if len(result) != tt.expected {
				t.Errorf("GetActiveFTPConfigs() length = %d, want %d", len(result), tt.expected)
			}
		})
	}
}

// TestDefaultTenantConfig_GetFTPConfigByUsername tests the GetFTPConfigByUsername method
func TestDefaultTenantConfig_GetFTPConfigByUsername(t *testing.T) {
	active := "active"
	inactive := "inactive"

	tests := []struct {
		name         string
		config       *DefaultTenantConfig
		searchUser   string
		expectedUser string
		expectedDesc string
	}{
		{
			name:         "nil config should return nil",
			config:       nil,
			searchUser:   "ftp1",
			expectedUser: "",
			expectedDesc: "",
		},
		{
			name: "nil FTPConfigs should return nil",
			config: &DefaultTenantConfig{
				FTPConfigs: nil,
			},
			searchUser:   "ftp1",
			expectedUser: "",
			expectedDesc: "",
		},
		{
			name: "empty FTPConfigs should return nil",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{},
			},
			searchUser:   "ftp1",
			expectedUser: "",
			expectedDesc: "",
		},
		{
			name: "existing username should return config",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "sales_ftp", Description: "Sales FTP", Status: active},
					{Username: "hr_ftp", Description: "HR FTP", Status: active},
					{Username: "finance_ftp", Description: "Finance FTP", Status: inactive},
				},
			},
			searchUser:   "hr_ftp",
			expectedUser: "hr_ftp",
			expectedDesc: "HR FTP",
		},
		{
			name: "non-existing username should return nil",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "sales_ftp", Description: "Sales FTP", Status: active},
					{Username: "hr_ftp", Description: "HR FTP", Status: active},
				},
			},
			searchUser:   "it_ftp",
			expectedUser: "",
			expectedDesc: "",
		},
		{
			name: "username with special characters",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "tenant1_sales_ftp", Description: "销售部 FTP", Status: active},
				},
			},
			searchUser:   "tenant1_sales_ftp",
			expectedUser: "tenant1_sales_ftp",
			expectedDesc: "销售部 FTP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetFTPConfigByUsername(tt.searchUser)
			if tt.expectedUser == "" {
				if result != nil {
					t.Errorf("GetFTPConfigByUsername() = %v, want nil", result)
				}
			} else {
				if result == nil {
					t.Errorf("GetFTPConfigByUsername() = nil, want config with username %q", tt.expectedUser)
				} else if result.Username != tt.expectedUser || result.Description != tt.expectedDesc {
					t.Errorf("GetFTPConfigByUsername() = {Username: %q, Description: %q}, want {Username: %q, Description: %q}",
						result.Username, result.Description, tt.expectedUser, tt.expectedDesc)
				}
			}
		})
	}
}

// TestDefaultTenantConfig_HasFTPConfigs tests the HasFTPConfigs method
func TestDefaultTenantConfig_HasFTPConfigs(t *testing.T) {
	tests := []struct {
		name     string
		config   *DefaultTenantConfig
		expected bool
	}{
		{
			name:     "nil config should return false",
			config:   nil,
			expected: false,
		},
		{
			name:     "nil FTPConfigs should return false",
			config:   &DefaultTenantConfig{FTPConfigs: nil},
			expected: false,
		},
		{
			name: "empty FTPConfigs should return false",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{},
			},
			expected: false,
		},
		{
			name: "single FTP config should return true",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1", Description: "FTP 1"},
				},
			},
			expected: true,
		},
		{
			name: "multiple FTP configs should return true",
			config: &DefaultTenantConfig{
				FTPConfigs: []TenantFTPDetailConfig{
					{Username: "ftp1", Description: "FTP 1"},
					{Username: "ftp2", Description: "FTP 2"},
					{Username: "ftp3", Description: "FTP 3"},
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
