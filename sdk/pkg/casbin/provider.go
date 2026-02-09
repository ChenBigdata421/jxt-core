package mycasbin

import (
	"context"
)

// PolicyRule represents a single Casbin policy rule
type PolicyRule struct {
	PType string // Policy type: "p" (policy) or "g" (role inheritance)
	V0    string // Usually sub (role name)
	V1    string // Usually obj (resource path)
	V2    string // Usually act (HTTP method)
	V3    string // Optional extension field
	V4    string // Optional extension field
	V5    string // Optional extension field
}

// PolicyProvider is the strategy interface for providing policy data
// Microservices implement this interface to supply policies from any source (gRPC, HTTP, DB, etc.)
type PolicyProvider interface {
	// GetPolicies retrieves all policy rules for the specified tenant
	//
	// Parameters:
	//   - ctx: Context (for timeout control, cancellation, etc.)
	//   - tenantID: Tenant identifier
	//
	// Returns:
	//   - []PolicyRule: List of policy rules (including both p and g types)
	//   - error: Error information
	GetPolicies(ctx context.Context, tenantID int) ([]PolicyRule, error)
}
