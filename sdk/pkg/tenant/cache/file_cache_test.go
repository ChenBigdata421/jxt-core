package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileCache_Load_Save(t *testing.T) {
	// Use temp directory for testing
	tmpDir := t.TempDir()
	cache := NewFileCacheWithPath(filepath.Join(tmpDir, "test_cache.json"))

	// Test: Save and Load
	data := &tenantSnapshot{
		ByID: map[int]*tenantMetadata{
			1: {
				TenantID: 1,
				Code:     "test1",
				Name:     "Test Tenant 1",
				Status:   "active",
			},
		},
		ByDomain:  map[string]int{"example.com": 1},
		ByFtpUser: map[string]int{"ftp1": 1},
	}

	if err := cache.Save(data); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := cache.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.ByID[1].Code != "test1" {
		t.Errorf("expected Code 'test1', got '%s'", loaded.ByID[1].Code)
	}
}

func TestFileCache_NotExist(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewFileCacheWithPath(filepath.Join(tmpDir, "nonexistent.json"))

	_, err := cache.Load()
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected ErrNotExist, got %v", err)
	}
}

func TestFileCache_IsAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewFileCacheWithPath(filepath.Join(tmpDir, "test.json"))

	// Not available initially
	if cache.IsAvailable() {
		t.Error("expected false for non-existent file")
	}

	// Save data
	data := &tenantSnapshot{
		ByID:      make(map[int]*tenantMetadata),
		ByDomain:  make(map[string]int),
		ByFtpUser: make(map[string]int),
	}
	cache.Save(data)

	// Available now
	if !cache.IsAvailable() {
		t.Error("expected true after Save")
	}
}

func TestFileCache_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewFileCacheWithPath(filepath.Join(tmpDir, "test.json"))

	// Save and verify
	data := &tenantSnapshot{
		ByID:      make(map[int]*tenantMetadata),
		ByDomain:  make(map[string]int),
		ByFtpUser: make(map[string]int),
	}
	cache.Save(data)

	if !cache.IsAvailable() {
		t.Error("expected file to exist")
	}

	// Clear and verify
	cache.Clear()

	if cache.IsAvailable() {
		t.Error("expected file to be deleted")
	}
}

func TestFileCache_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "atomic.json")
	cache := NewFileCacheWithPath(cachePath)

	// Save multiple times to test atomic rename
	for i := 0; i < 10; i++ {
		data := &tenantSnapshot{
			ByID: map[int]*tenantMetadata{
				1: {TenantID: 1, Code: "test", Name: "Test", Status: "active"},
			},
			ByDomain:  make(map[string]int),
			ByFtpUser: make(map[string]int),
		}
		if err := cache.Save(data); err != nil {
			t.Fatalf("Save() failed on iteration %d: %v", i, err)
		}
	}

	// Verify file exists and is valid
	if !cache.IsAvailable() {
		t.Error("expected cache file to exist")
	}

	_, err := cache.Load()
	if err != nil {
		t.Errorf("Load() failed after atomic writes: %v", err)
	}
}
