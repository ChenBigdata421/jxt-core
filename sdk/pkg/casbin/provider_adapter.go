package mycasbin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
)

// ProviderAdapter implements Casbin's persist.Adapter interface using a PolicyProvider
// This is a read-only adapter - write operations return errors
type ProviderAdapter struct {
	provider PolicyProvider
	tenantID int
	timeout  time.Duration
}

// NewProviderAdapter creates a new ProviderAdapter
func NewProviderAdapter(provider PolicyProvider, tenantID int) *ProviderAdapter {
	return &ProviderAdapter{
		provider: provider,
		tenantID: tenantID,
		timeout:  5 * time.Second, // Default timeout for Provider calls
	}
}

// LoadPolicy loads all policies from the Provider into the model
// This is called by Casbin SyncedEnforcer.LoadPolicy()
func (a *ProviderAdapter) LoadPolicy(m model.Model) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()

	rules, err := a.provider.GetPolicies(ctx, a.tenantID)
	if err != nil {
		return fmt.Errorf("provider GetPolicies failed: %w", err)
	}

	// Use persist.LoadPolicyLine to load each policy into the model
	// This is Casbin's official API for loading policies from text format
	for _, rule := range rules {
		line := policyRuleToLine(rule)
		persist.LoadPolicyLine(line, m)
	}

	return nil
}

// policyRuleToLine converts PolicyRule to Casbin policy line format
// Format: "p_type, v0, v1, v2, ..." (comma-separated, non-empty fields only)
//
// Precondition: PolicyRule fields must be filled contiguously from left to right,
// without gaps. For example, V0="admin", V1="", V2="/api/test" is invalid.
// This is Casbin's standard data format constraint - data written by gormAdapter
// naturally satisfies this condition.
func policyRuleToLine(r PolicyRule) string {
	parts := []string{r.PType}
	for _, v := range []string{r.V0, r.V1, r.V2, r.V3, r.V4, r.V5} {
		if v == "" {
			break // Casbin policy fields are filled left-to-right, stop at first empty
		}
		parts = append(parts, v)
	}
	return strings.Join(parts, ", ")
}

// SavePolicy is not supported (read-only adapter)
func (a *ProviderAdapter) SavePolicy(m model.Model) error {
	return fmt.Errorf("ProviderAdapter is read-only: SavePolicy not supported")
}

// AddPolicy is not supported (read-only adapter)
func (a *ProviderAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	return fmt.Errorf("ProviderAdapter is read-only: AddPolicy not supported")
}

// RemovePolicy is not supported (read-only adapter)
func (a *ProviderAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	return fmt.Errorf("ProviderAdapter is read-only: RemovePolicy not supported")
}

// RemoveFilteredPolicy is not supported (read-only adapter)
func (a *ProviderAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return fmt.Errorf("ProviderAdapter is read-only: RemoveFilteredPolicy not supported")
}
