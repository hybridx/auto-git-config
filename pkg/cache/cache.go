// Package cache provides a file-based cache for resolved configurations.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry represents a cached resolution.
type Entry struct {
	// RepoRoot is the repository root path
	RepoRoot string `json:"repo_root"`

	// ConfigHash is a hash of the config file for invalidation
	ConfigHash string `json:"config_hash"`

	// ResolvedConfig is the resolved configuration
	ResolvedConfig map[string]string `json:"resolved_config"`

	// MatchedRule is the name of the matched rule
	MatchedRule string `json:"matched_rule"`

	// CachedAt is when this entry was created
	CachedAt time.Time `json:"cached_at"`

	// RemotesHash is a hash of remotes for invalidation
	RemotesHash string `json:"remotes_hash"`
}

// IsValid checks if the cache entry is still valid.
func (e *Entry) IsValid(ttl time.Duration, currentConfigHash, currentRemotesHash string) bool {
	// Check TTL
	if time.Since(e.CachedAt) > ttl {
		return false
	}

	// Check config hash
	if e.ConfigHash != currentConfigHash {
		return false
	}

	// Check remotes hash (in case remotes changed)
	if e.RemotesHash != currentRemotesHash {
		return false
	}

	return true
}

// Cache manages cached resolutions.
type Cache struct {
	dir string
	ttl time.Duration
}

// New creates a new Cache.
func New(cacheDir string, ttlSeconds int) *Cache {
	if cacheDir == "" {
		// Default cache directory
		cacheDir = filepath.Join(os.TempDir(), "auto-git-config-cache")
	}
	return &Cache{
		dir: cacheDir,
		ttl: time.Duration(ttlSeconds) * time.Second,
	}
}

// Get retrieves a cached entry for a repository.
func (c *Cache) Get(repoRoot, configHash, remotesHash string) (*Entry, error) {
	path := c.entryPath(repoRoot)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Cache miss
		}
		return nil, err
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Corrupted cache, treat as miss
		os.Remove(path)
		return nil, nil
	}

	if !entry.IsValid(c.ttl, configHash, remotesHash) {
		// Stale entry
		os.Remove(path)
		return nil, nil
	}

	return &entry, nil
}

// Set stores a cache entry.
func (c *Cache) Set(entry *Entry) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	entry.CachedAt = time.Now()

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	path := c.entryPath(entry.RepoRoot)
	return os.WriteFile(path, data, 0644)
}

// Invalidate removes a cache entry.
func (c *Cache) Invalidate(repoRoot string) error {
	path := c.entryPath(repoRoot)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Clear removes all cache entries.
func (c *Cache) Clear() error {
	return os.RemoveAll(c.dir)
}

// entryPath returns the file path for a cache entry.
func (c *Cache) entryPath(repoRoot string) string {
	hash := HashString(repoRoot)
	return filepath.Join(c.dir, hash+".json")
}

// HashString creates a SHA256 hash of a string.
func HashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// HashFile creates a SHA256 hash of a file's contents.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// HashRemotes creates a hash of remote URLs.
func HashRemotes(remotes map[string]string) string {
	// Sort keys for deterministic output
	h := sha256.New()
	for name, url := range remotes {
		h.Write([]byte(name))
		h.Write([]byte(url))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

