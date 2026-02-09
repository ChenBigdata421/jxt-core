package mycasbin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPolicyRule_VerifyFields verifies PolicyRule has all required fields
func TestPolicyRule_VerifyFields(t *testing.T) {
	rule := PolicyRule{
		PType: "p",
		V0:    "admin",
		V1:    "/api/v1/*",
		V2:    "GET",
		V3:    "",
		V4:    "",
		V5:    "",
	}

	assert.Equal(t, "p", rule.PType)
	assert.Equal(t, "admin", rule.V0)
	assert.Equal(t, "/api/v1/*", rule.V1)
	assert.Equal(t, "GET", rule.V2)
}

// TestPolicyProvider_InterfaceExists verifies PolicyProvider interface is correctly defined
func TestPolicyProvider_InterfaceExists(t *testing.T) {
	var provider interface{} = &mockProvider{}
	_, ok := provider.(PolicyProvider)
	assert.True(t, ok, "PolicyProvider interface should be defined")
}

// mockProvider is a test implementation of PolicyProvider
type mockProvider struct{}

func (m *mockProvider) GetPolicies(ctx context.Context, tenantID int) ([]PolicyRule, error) {
	return []PolicyRule{
		{PType: "p", V0: "admin", V1: "/api/test", V2: "GET"},
	}, nil
}

// TestPolicyRule_FullyPopulated verifies all fields can be set
func TestPolicyRule_FullyPopulated(t *testing.T) {
	rule := PolicyRule{
		PType: "p",
		V0:    "admin",
		V1:    "/api/v1/resource",
		V2:    "GET",
		V3:    "domain.com",
		V4:    "extra1",
		V5:    "extra2",
	}

	assert.Equal(t, "p", rule.PType)
	assert.Equal(t, "admin", rule.V0)
	assert.Equal(t, "/api/v1/resource", rule.V1)
	assert.Equal(t, "GET", rule.V2)
	assert.Equal(t, "domain.com", rule.V3)
	assert.Equal(t, "extra1", rule.V4)
	assert.Equal(t, "extra2", rule.V5)
}
