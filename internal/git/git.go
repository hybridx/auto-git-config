package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repository represents a Git repository context.
type Repository struct {
	// Root is the absolute path to the repository root (.git parent)
	Root string

	// WorkDir is the current working directory within the repo
	WorkDir string

	// Remotes are the configured remotes
	Remotes map[string]*Remote
}

// DetectRepository detects the Git repository for the given path.
// Returns nil if not in a git repository.
func DetectRepository(path string) (*Repository, error) {
	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if path exists
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("path does not exist: %w", err)
	}

	// If it's a file, use its directory
	if !info.IsDir() {
		absPath = filepath.Dir(absPath)
	}

	// Find repository root using git rev-parse
	root, err := findRepoRoot(absPath)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, nil // Not in a git repository
	}

	repo := &Repository{
		Root:    root,
		WorkDir: absPath,
		Remotes: make(map[string]*Remote),
	}

	// Load remotes
	if err := repo.loadRemotes(); err != nil {
		// Non-fatal: repo might not have remotes configured
		// Just log in debug mode
	}

	return repo, nil
}

// findRepoRoot finds the repository root directory.
func findRepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path

	output, err := cmd.Output()
	if err != nil {
		// Check if it's just "not a git repository"
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "not a git repository") {
				return "", nil
			}
		}
		return "", fmt.Errorf("failed to detect git root: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// loadRemotes loads all configured remotes.
func (r *Repository) loadRemotes() error {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = r.Root

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		url := parts[1]

		// Skip if we already have this remote (remotes appear twice: fetch/push)
		if _, exists := r.Remotes[name]; exists {
			continue
		}

		remote, err := ParseRemoteURL(url)
		if err != nil {
			continue // Skip malformed URLs
		}
		if remote != nil {
			remote.Name = name
			r.Remotes[name] = remote
		}
	}

	return scanner.Err()
}

// GetRemote returns a specific remote by name.
func (r *Repository) GetRemote(name string) *Remote {
	return r.Remotes[name]
}

// GetOrigin returns the origin remote, or nil if not configured.
func (r *Repository) GetOrigin() *Remote {
	return r.Remotes["origin"]
}

// GetPrimaryRemote returns the most likely "primary" remote.
// Prefers origin, then falls back to the first remote found.
func (r *Repository) GetPrimaryRemote() *Remote {
	if origin := r.GetOrigin(); origin != nil {
		return origin
	}
	for _, remote := range r.Remotes {
		return remote
	}
	return nil
}

// FolderName returns the repository's folder name.
func (r *Repository) FolderName() string {
	return filepath.Base(r.Root)
}

// GetConfig reads a git config value.
func GetConfig(key string, scope ConfigScope) (string, error) {
	args := []string{"config"}
	switch scope {
	case ConfigScopeGlobal:
		args = append(args, "--global")
	case ConfigScopeLocal:
		args = append(args, "--local")
	case ConfigScopeSystem:
		args = append(args, "--system")
	}
	args = append(args, "--get", key)

	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// SetConfig sets a git config value.
func SetConfig(key, value string, scope ConfigScope) error {
	args := []string{"config"}
	switch scope {
	case ConfigScopeGlobal:
		args = append(args, "--global")
	case ConfigScopeLocal:
		args = append(args, "--local")
	case ConfigScopeSystem:
		args = append(args, "--system")
	}
	args = append(args, key, value)

	cmd := exec.Command("git", args...)
	return cmd.Run()
}

// SetConfigInDir sets a git config value in a specific directory.
func SetConfigInDir(dir, key, value string, scope ConfigScope) error {
	args := []string{"config"}
	switch scope {
	case ConfigScopeGlobal:
		args = append(args, "--global")
	case ConfigScopeLocal:
		args = append(args, "--local")
	case ConfigScopeSystem:
		args = append(args, "--system")
	}
	args = append(args, key, value)

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}

// ConfigScope represents the scope for git config operations.
type ConfigScope int

const (
	ConfigScopeDefault ConfigScope = iota // Let git decide
	ConfigScopeLocal                      // Repository-local config
	ConfigScopeGlobal                     // User global config
	ConfigScopeSystem                     // System-wide config
)

// GetEffectiveConfig reads a config value from the effective configuration.
func GetEffectiveConfig(key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", key)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetAllConfig returns all git config values as a map.
func GetAllConfig() (map[string]string, error) {
	cmd := exec.Command("git", "config", "--list")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	config := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "="); idx != -1 {
			key := line[:idx]
			value := line[idx+1:]
			config[key] = value
		}
	}

	return config, scanner.Err()
}

