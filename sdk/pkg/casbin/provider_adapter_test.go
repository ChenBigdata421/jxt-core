package mycasbin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/casbin/casbin/v2/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProviderForAdapter is a mock PolicyProvider for testing
type mockProviderForAdapter struct {
	policies []PolicyRule
	err      error
	delay    time.Duration
}

func (m *mockProviderForAdapter) GetPolicies(ctx context.Context, tenantID int) ([]PolicyRule, error) {
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.delay):
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.policies, nil
}

// TestNewProviderAdapter verifies adapter creation
func TestNewProviderAdapter(t *testing.T) {
	provider := &mockProviderForAdapter{}
	adapter := NewProviderAdapter(provider, 1)

	assert.NotNil(t, adapter)
}

// TestProviderAdapter_LoadPolicy_Success verifies successful policy loading
func TestProviderAdapter_LoadPolicy_Success(t *testing.T) {
	provider := &mockProviderForAdapter{
		policies: []PolicyRule{
			{PType: "p", V0: "admin", V1: "/api/v1/*", V2: "GET"},
			{PType: "p", V0: "user", V1: "/api/read", V2: "GET"},
		},
	}

	adapter := NewProviderAdapter(provider, 1)

	m, err := model.NewModelFromString(text)
	require.NoError(t, err)

	err = adapter.LoadPolicy(m)
	assert.NoError(t, err)

	// Verify policies were loaded
	policies := m.GetPolicy("p", "p")
	assert.Len(t, policies, 2)
}

// TestProviderAdapter_LoadPolicy_ProviderError verifies error handling
func TestProviderAdapter_LoadPolicy_ProviderError(t *testing.T) {
	provider := &mockProviderForAdapter{
		err: errors.New("provider failed"),
	}

	adapter := NewProviderAdapter(provider, 1)

	m, _ := model.NewModelFromString(text)
	err := adapter.LoadPolicy(m)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider GetPolicies failed")
}

// TestPolicyRuleToLine verifies policy rule to line conversion
func TestPolicyRuleToLine(t *testing.T) {
	tests := []struct {
		name     string
		rule     PolicyRule
		expected string
	}{
		{
			name:     "three fields",
			rule:     PolicyRule{PType: "p", V0: "admin", V1: "/api/test", V2: "GET"},
			expected: "p, admin, /api/test, GET",
		},
		{
			name:     "two fields",
			rule:     PolicyRule{PType: "p", V0: "admin", V1: "/api/test"},
			expected: "p, admin, /api/test",
		},
		{
			name:     "g type with two fields",
			rule:     PolicyRule{PType: "g", V0: "alice", V1: "admin"},
			expected: "g, alice, admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := policyRuleToLine(tt.rule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProviderAdapter_LoadPolicy_Timeout verifies timeout handling
func TestProviderAdapter_LoadPolicy_Timeout(t *testing.T) {
	provider := &mockProviderForAdapter{
		delay: 10 * time.Second, // Longer than adapter timeout
	}

	adapter := NewProviderAdapter(provider, 1)

	m, _ := model.NewModelFromString(text)
	err := adapter.LoadPolicy(m)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

// TestProviderAdapter_ReadOnlyMethods verifies write operations return errors
func TestProviderAdapter_ReadOnlyMethods(t *testing.T) {
	provider := &mockProviderForAdapter{}
	adapter := NewProviderAdapter(provider, 1)

	m, _ := model.NewModelFromString(text)

	assert.Error(t, adapter.SavePolicy(m))
	assert.Error(t, adapter.AddPolicy("p", "p", []string{"admin", "/api", "GET"}))
	assert.Error(t, adapter.RemovePolicy("p", "p", []string{"admin", "/api", "GET"}))
	assert.Error(t, adapter.RemoveFilteredPolicy("p", "p", 0))
}
