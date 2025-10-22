package outbox

import (
	"testing"
	"time"
)

// TestPublisherConfig_Validate 测试发布器配置验证
func TestPublisherConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *PublisherConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "publisher config is nil",
		},
		{
			name: "valid config",
			config: &PublisherConfig{
				MaxRetries:     3,
				RetryDelay:     5 * time.Second,
				PublishTimeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "negative MaxRetries",
			config: &PublisherConfig{
				MaxRetries:     -1,
				RetryDelay:     5 * time.Second,
				PublishTimeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "MaxRetries must be >= 0",
		},
		{
			name: "MaxRetries too large",
			config: &PublisherConfig{
				MaxRetries:     101,
				RetryDelay:     5 * time.Second,
				PublishTimeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "MaxRetries is too large",
		},
		{
			name: "negative RetryDelay",
			config: &PublisherConfig{
				MaxRetries:     3,
				RetryDelay:     -1 * time.Second,
				PublishTimeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "RetryDelay must be >= 0",
		},
		{
			name: "RetryDelay too large",
			config: &PublisherConfig{
				MaxRetries:     3,
				RetryDelay:     2 * time.Hour,
				PublishTimeout: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "RetryDelay is too large",
		},
		{
			name: "negative PublishTimeout",
			config: &PublisherConfig{
				MaxRetries:     3,
				RetryDelay:     5 * time.Second,
				PublishTimeout: -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "PublishTimeout must be >= 0",
		},
		{
			name: "PublishTimeout too large",
			config: &PublisherConfig{
				MaxRetries:     3,
				RetryDelay:     5 * time.Second,
				PublishTimeout: 10 * time.Minute,
			},
			wantErr: true,
			errMsg:  "PublishTimeout is too large",
		},
		{
			name: "zero values (valid)",
			config: &PublisherConfig{
				MaxRetries:     0,
				RetryDelay:     0,
				PublishTimeout: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("PublisherConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if err.Error() != tt.errMsg && !contains(err.Error(), tt.errMsg) {
					t.Errorf("PublisherConfig.Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

// TestSchedulerConfig_Validate 测试调度器配置验证
func TestSchedulerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *SchedulerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "scheduler config is nil",
		},
		{
			name:    "valid default config",
			config:  DefaultSchedulerConfig(),
			wantErr: false,
		},
		{
			name: "zero PollInterval",
			config: &SchedulerConfig{
				PollInterval: 0,
				BatchSize:    100,
			},
			wantErr: true,
			errMsg:  "PollInterval must be > 0",
		},
		{
			name: "PollInterval too small",
			config: &SchedulerConfig{
				PollInterval: 500 * time.Millisecond,
				BatchSize:    100,
			},
			wantErr: true,
			errMsg:  "PollInterval is too small",
		},
		{
			name: "PollInterval too large",
			config: &SchedulerConfig{
				PollInterval: 2 * time.Hour,
				BatchSize:    100,
			},
			wantErr: true,
			errMsg:  "PollInterval is too large",
		},
		{
			name: "zero BatchSize",
			config: &SchedulerConfig{
				PollInterval: 10 * time.Second,
				BatchSize:    0,
			},
			wantErr: true,
			errMsg:  "BatchSize must be > 0",
		},
		{
			name: "BatchSize too large",
			config: &SchedulerConfig{
				PollInterval: 10 * time.Second,
				BatchSize:    10001,
			},
			wantErr: true,
			errMsg:  "BatchSize is too large",
		},
		{
			name: "invalid CleanupInterval when cleanup enabled",
			config: &SchedulerConfig{
				PollInterval:    10 * time.Second,
				BatchSize:       100,
				EnableCleanup:   true,
				CleanupInterval: 30 * time.Second,
			},
			wantErr: true,
			errMsg:  "CleanupInterval is too small",
		},
		{
			name: "invalid CleanupRetention when cleanup enabled",
			config: &SchedulerConfig{
				PollInterval:     10 * time.Second,
				BatchSize:        100,
				EnableCleanup:    true,
				CleanupInterval:  1 * time.Hour,
				CleanupRetention: 30 * time.Minute,
			},
			wantErr: true,
			errMsg:  "CleanupRetention is too small",
		},
		{
			name: "invalid HealthCheckInterval when health check enabled",
			config: &SchedulerConfig{
				PollInterval:        10 * time.Second,
				BatchSize:           100,
				EnableHealthCheck:   true,
				HealthCheckInterval: 500 * time.Millisecond,
			},
			wantErr: true,
			errMsg:  "HealthCheckInterval is too small",
		},
		{
			name: "invalid RetryInterval when retry enabled",
			config: &SchedulerConfig{
				PollInterval:  10 * time.Second,
				BatchSize:     100,
				EnableRetry:   true,
				RetryInterval: 500 * time.Millisecond,
			},
			wantErr: true,
			errMsg:  "RetryInterval is too small",
		},
		{
			name: "invalid MaxRetries when retry enabled",
			config: &SchedulerConfig{
				PollInterval:  10 * time.Second,
				BatchSize:     100,
				EnableRetry:   true,
				RetryInterval: 30 * time.Second,
				MaxRetries:    101,
			},
			wantErr: true,
			errMsg:  "MaxRetries is too large",
		},
		{
			name: "invalid DLQInterval when DLQ enabled",
			config: &SchedulerConfig{
				PollInterval: 10 * time.Second,
				BatchSize:    100,
				EnableDLQ:    true,
				DLQInterval:  500 * time.Millisecond,
			},
			wantErr: true,
			errMsg:  "DLQInterval is too small",
		},
		{
			name: "ShutdownTimeout too large",
			config: &SchedulerConfig{
				PollInterval:    10 * time.Second,
				BatchSize:       100,
				ShutdownTimeout: 10 * time.Minute,
			},
			wantErr: true,
			errMsg:  "ShutdownTimeout is too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SchedulerConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("SchedulerConfig.Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

