package mycasbin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProviderForSetup is a mock for testing SetupWithProvider
type mockProviderForSetup struct {
	policies []PolicyRule
}

func (m *mockProviderForSetup) GetPolicies(ctx context.Context, tenantID int) ([]PolicyRule, error) {
	return m.policies, nil
}

// TestSetupWithProvider_Success verifies successful enforcer creation
func TestSetupWithProvider_Success(t *testing.T) {
	provider := &mockProviderForSetup{
		policies: []PolicyRule{
			{PType: "p", V0: "admin", V1: "/api/v1/*", V2: "GET"},
		},
	}

	enforcer, err := SetupWithProvider(provider, 1)

	require.NoError(t, err)
	assert.NotNil(t, enforcer)

	// Verify policy was loaded
	policies := enforcer.GetPolicy()
	assert.Len(t, policies, 1)
	assert.Equal(t, []string{"admin", "/api/v1/*", "GET"}, policies[0])
}

// TestSetupWithProvider_NoPolicies verifies enforcer creation with empty policies
func TestSetupWithProvider_NoPolicies(t *testing.T) {
	provider := &mockProviderForSetup{
		policies: []PolicyRule{},
	}

	enforcer, err := SetupWithProvider(provider, 1)

	require.NoError(t, err)
	assert.NotNil(t, enforcer)

	policies := enforcer.GetPolicy()
	assert.Len(t, policies, 0)
}

// TestSetupWithProvider_MultipleTenants verifies tenant isolation
func TestSetupWithProvider_MultipleTenants(t *testing.T) {
	provider1 := &mockProviderForSetup{
		policies: []PolicyRule{{PType: "p", V0: "admin", V1: "/api/v1/*", V2: "GET"}},
	}
	provider2 := &mockProviderForSetup{
		policies: []PolicyRule{{PType: "p", V0: "user", V1: "/api/v2/*", V2: "GET"}},
	}

	enforcer1, err := SetupWithProvider(provider1, 1)
	require.NoError(t, err)

	enforcer2, err := SetupWithProvider(provider2, 2)
	require.NoError(t, err)

	// Verify enforcers are different instances
	assert.NotSame(t, enforcer1, enforcer2)

	// Verify policies are isolated
	policies1 := enforcer1.GetPolicy()
	policies2 := enforcer2.GetPolicy()

	assert.Len(t, policies1, 1)
	assert.Len(t, policies2, 1)
	assert.NotEqual(t, policies1[0], policies2[0])
}
