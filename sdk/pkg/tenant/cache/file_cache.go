// Package cache provides persistent cache layer for tenant data.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileCache provides persistent file-based caching for tenant data.
type FileCache struct {
	mu       sync.RWMutex
	filePath string
	codec    Codec
}

// Codec defines the encoding/decoding interface for cache data.
type Codec interface {
	Encode(data interface{}) ([]byte, error)
	Decode(bytes []byte, dest interface{}) error
}

// JSONCodec implements Codec using JSON encoding.
type JSONCodec struct{}

func (JSONCodec) Encode(data interface{}) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

func (JSONCodec) Decode(bytes []byte, dest interface{}) error {
	return json.Unmarshal(bytes, dest)
}

// NewFileCache creates a new file-based cache.
// The cache path can be configured via TENANT_CACHE_PATH env variable.
// Default path: ./cache/tenant_metadata.json
func NewFileCache() *FileCache {
	cachePath := os.Getenv("TENANT_CACHE_PATH")
	if cachePath == "" {
		cachePath = "./cache"
	}
	return &FileCache{
		filePath: filepath.Join(cachePath, "tenant_metadata.json"),
		codec:    JSONCodec{},
	}
}

// NewFileCacheWithPath creates a cache with a specific file path.
func NewFileCacheWithPath(path string) *FileCache {
	return &FileCache{
		filePath: path,
		codec:    JSONCodec{},
	}
}

// Load reads cached tenant data from file.
// Returns os.ErrNotExist if cache file doesn't exist.
func (f *FileCache) Load() (*tenantSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	bytes, err := os.ReadFile(f.filePath)
	if err != nil {
		return nil, err
	}

	var snapshot tenantSnapshot
	if err := f.codec.Decode(bytes, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to decode cache: %w", err)
	}

	return &snapshot, nil
}

// Save writes tenant data to cache file atomically.
// Uses temporary file + rename to ensure consistency.
func (f *FileCache) Save(data *tenantSnapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Create directory if not exists
	if err := os.MkdirAll(filepath.Dir(f.filePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Encode data
	bytes, err := f.codec.Encode(data)
	if err != nil {
		return fmt.Errorf("failed to encode cache: %w", err)
	}

	// Write to temporary file
	tmpPath := f.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, f.filePath); err != nil {
		os.Remove(tmpPath) // Clean up
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	return nil
}

// IsAvailable checks if the cache file exists and is readable.
func (f *FileCache) IsAvailable() bool {
	_, err := os.ReadFile(f.filePath)
	return err == nil
}

// Clear removes the cache file.
func (f *FileCache) Clear() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return os.Remove(f.filePath)
}

// tenantSnapshot represents the cacheable data structure.
// This must match provider.tenantData structure.
type tenantSnapshot struct {
	ByID      map[int]*tenantMetadata `json:"by_id"`
	ByDomain  map[string]int          `json:"by_domain"`
	ByFtpUser map[string]int          `json:"by_ftp_user"`
}

type tenantMetadata struct {
	TenantID int    `json:"tenant_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	// ... other fields to match provider models
}
