package ftp

import (
	"context"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

func TestFtpCache_GetAll(t *testing.T) {
	prov := provider.NewProvider(nil,
		provider.WithConfigTypes(provider.ConfigTypeFtp))

	cache := NewCache(prov)

	// Test with non-existent tenant
	_, err := cache.GetAll(context.Background(), 1)
	if err == nil {
		t.Error("expected error for non-existent tenant")
	}
}

func TestFtpCache_GetByUsername(t *testing.T) {
	prov := provider.NewProvider(nil,
		provider.WithConfigTypes(provider.ConfigTypeFtp))

	cache := NewCache(prov)

	// Test with non-existent username
	_, err := cache.GetByUsername(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent username")
	}
}

func TestFtpCache_GetActive(t *testing.T) {
	prov := provider.NewProvider(nil,
		provider.WithConfigTypes(provider.ConfigTypeFtp))

	cache := NewCache(prov)

	// Test with non-existent tenant
	_, err := cache.GetActive(context.Background(), 1)
	if err == nil {
		t.Error("expected error for non-existent tenant")
	}
}
