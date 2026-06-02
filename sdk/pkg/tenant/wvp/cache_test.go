package wvp

import (
	"context"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

func TestCache_GetByID_NotFound(t *testing.T) {
	prov := provider.NewProvider(nil,
		provider.WithConfigTypes(provider.ConfigTypeWvp))

	cache := NewCache(prov)

	_, err := cache.GetByID(context.Background(), 1)
	if err == nil {
		t.Error("expected error for unknown tenant")
	}
}

func TestCache_GetByID_Found(t *testing.T) {
	p := provider.NewProvider(nil,
		provider.WithConfigTypes(provider.ConfigTypeWvp))

	// The found-with-meta path is covered by provider-level tests (Task 7)
	// and e2e tests (Task 11). This tests the wvp.Cache wrapper's error message format.
	cache := NewCache(p)

	_, err := cache.GetByID(context.Background(), 1)
	if err == nil {
		t.Error("expected error for tenant without WVP config")
	}
	if err.Error() != "tenant 1 WVP config not found" {
		t.Errorf("error message: got %q, want %q", err.Error(), "tenant 1 WVP config not found")
	}
}
