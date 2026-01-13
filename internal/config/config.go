// Package config handles configuration parsing and schema definition for auto-git-config.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config represents the top-level configuration structure.
type Config struct {
	// Version of the config schema for future compatibility
	Version string `toml:"version"`

	// Default configuration applied when no rules match (before global git config)
	Default *GitConfig `toml:"default"`

	// Rules define matching conditions and their corresponding git configurations
	Rules []Rule `toml:"rule"`

	// Settings for the tool itself
	Settings Settings `toml:"settings"`
}

// Settings controls tool behavior.
type Settings struct {
	// CacheEnabled enables caching of resolved configurations
	CacheEnabled bool `toml:"cache_enabled"`

	// CacheTTLSeconds sets cache time-to-live in seconds
	CacheTTLSeconds int `toml:"cache_ttl_seconds"`

	// DefaultRemote specifies which remote to match against (default: "origin")
	DefaultRemote string `toml:"default_remote"`

	// IncludeIfDir specifies where to generate includeIf config files
	IncludeIfDir string `toml:"includeif_dir"`

	// Debug enables verbose logging
	Debug bool `toml:"debug"`
}

// Rule defines a matching condition and the git configuration to apply.
type Rule struct {
	// Name is a human-readable identifier for the rule
	Name string `toml:"name"`

	// Priority allows explicit ordering (higher = checked first)
	// If not set, rules are evaluated in definition order within their category
	Priority int `toml:"priority"`

	// Match defines the conditions for this rule to apply
	Match Match `toml:"match"`

	// Config specifies the git configuration to apply when matched
	Config GitConfig `toml:"config"`

	// Enabled allows disabling rules without removing them
	Enabled *bool `toml:"enabled"`
}

// IsEnabled returns whether the rule is enabled (defaults to true).
func (r *Rule) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// Match defines conditions for rule matching.
// Multiple conditions are AND-ed together.
type Match struct {
	// Repository-specific match (highest priority)
	RepoPath string `toml:"repo_path"` // Exact path to repository root

	// Remote-based matching
	RemoteHost    string `toml:"remote_host"`    // Normalized host (e.g., "github.com")
	RemoteURL     string `toml:"remote_url"`     // Regex pattern for full remote URL
	RemoteOrg     string `toml:"remote_org"`     // Organization/username in remote
	RemoteName    string `toml:"remote_name"`    // Which remote to check (default: origin)

	// Path-based matching
	PathPrefix    string `toml:"path_prefix"`    // Directory prefix (supports ~)
	PathContains  string `toml:"path_contains"`  // Path must contain this string
	FolderName    string `toml:"folder_name"`    // Exact folder name match
	FolderPattern string `toml:"folder_pattern"` // Regex pattern for folder name
}

// MatchType returns the primary type of match for precedence calculation.
func (m *Match) MatchType() MatchType {
	if m.RepoPath != "" {
		return MatchTypeRepo
	}
	if m.RemoteHost != "" || m.RemoteURL != "" || m.RemoteOrg != "" {
		return MatchTypeRemote
	}
	if m.PathPrefix != "" || m.PathContains != "" || m.FolderName != "" || m.FolderPattern != "" {
		return MatchTypePath
	}
	return MatchTypeNone
}

// MatchType represents the category of a match for precedence.
type MatchType int

const (
	MatchTypeNone MatchType = iota
	MatchTypePath
	MatchTypeRemote
	MatchTypeRepo
)

func (mt MatchType) String() string {
	switch mt {
	case MatchTypeRepo:
		return "repository"
	case MatchTypeRemote:
		return "remote"
	case MatchTypePath:
		return "path"
	default:
		return "none"
	}
}

// GitConfig represents git configuration values to apply.
type GitConfig struct {
	User   UserConfig   `toml:"user"`
	Commit CommitConfig `toml:"commit"`
	Core   CoreConfig   `toml:"core"`

	// Extra allows setting arbitrary git config keys
	Extra map[string]string `toml:"extra"`
}

// UserConfig represents user.* git configuration.
type UserConfig struct {
	Name       string `toml:"name"`
	Email      string `toml:"email"`
	SigningKey string `toml:"signingkey"`
}

// CommitConfig represents commit.* git configuration.
type CommitConfig struct {
	GPGSign   *bool  `toml:"gpgsign"`
	Template  string `toml:"template"`
	Verbose   *bool  `toml:"verbose"`
}

// CoreConfig represents core.* git configuration.
type CoreConfig struct {
	Editor      string `toml:"editor"`
	Autocrlf    string `toml:"autocrlf"`
	Excludesfile string `toml:"excludesfile"`
}

// ToGitConfigMap converts GitConfig to a flat map of git config keys to values.
func (gc *GitConfig) ToGitConfigMap() map[string]string {
	result := make(map[string]string)

	// User config
	if gc.User.Name != "" {
		result["user.name"] = gc.User.Name
	}
	if gc.User.Email != "" {
		result["user.email"] = gc.User.Email
	}
	if gc.User.SigningKey != "" {
		result["user.signingkey"] = gc.User.SigningKey
	}

	// Commit config
	if gc.Commit.GPGSign != nil {
		result["commit.gpgsign"] = fmt.Sprintf("%t", *gc.Commit.GPGSign)
	}
	if gc.Commit.Template != "" {
		result["commit.template"] = gc.Commit.Template
	}
	if gc.Commit.Verbose != nil {
		result["commit.verbose"] = fmt.Sprintf("%t", *gc.Commit.Verbose)
	}

	// Core config
	if gc.Core.Editor != "" {
		result["core.editor"] = gc.Core.Editor
	}
	if gc.Core.Autocrlf != "" {
		result["core.autocrlf"] = gc.Core.Autocrlf
	}
	if gc.Core.Excludesfile != "" {
		result["core.excludesfile"] = gc.Core.Excludesfile
	}

	// Extra config
	for k, v := range gc.Extra {
		result[k] = v
	}

	return result
}

// IsEmpty returns true if no configuration values are set.
func (gc *GitConfig) IsEmpty() bool {
	return len(gc.ToGitConfigMap()) == 0
}

// DefaultSettings returns the default tool settings.
func DefaultSettings() Settings {
	homeDir, _ := os.UserHomeDir()
	return Settings{
		CacheEnabled:    true,
		CacheTTLSeconds: 300, // 5 minutes
		DefaultRemote:   "origin",
		IncludeIfDir:    filepath.Join(homeDir, ".config", "auto-git-config", "gitconfig.d"),
		Debug:           false,
	}
}

// Load reads and parses a configuration file.
func Load(path string) (*Config, error) {
	// Expand ~ in path
	path = expandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return Parse(string(data))
}

// Parse parses configuration from a TOML string.
func Parse(data string) (*Config, error) {
	var cfg Config
	if _, err := toml.Decode(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	if cfg.Settings.DefaultRemote == "" {
		cfg.Settings.DefaultRemote = "origin"
	}
	if cfg.Settings.IncludeIfDir == "" {
		cfg.Settings.IncludeIfDir = DefaultSettings().IncludeIfDir
	}

	// Validate rules
	for i, rule := range cfg.Rules {
		if rule.Name == "" {
			return nil, fmt.Errorf("rule %d: name is required", i)
		}
		if rule.Config.IsEmpty() {
			return nil, fmt.Errorf("rule %q: config cannot be empty", rule.Name)
		}
	}

	return &cfg, nil
}

// DefaultConfigPath returns the default configuration file path.
func DefaultConfigPath() string {
	// Check XDG_CONFIG_HOME first
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "auto-git-config", "config.toml")
	}

	// Fall back to ~/.config
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "auto-git-config", "config.toml")
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// ExpandPath is exported for use by other packages.
func ExpandPath(path string) string {
	return expandPath(path)
}

