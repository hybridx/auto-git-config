// Package applier handles safe application of git configuration.
package applier

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/denair/auto-git-config/internal/config"
	"github.com/denair/auto-git-config/internal/git"
	"github.com/denair/auto-git-config/internal/resolver"
)

// Mode specifies how configuration should be applied.
type Mode int

const (
	// ModeLocal applies config to the repository's local config.
	ModeLocal Mode = iota

	// ModeEnv sets environment variables for git commands.
	ModeEnv

	// ModeIncludeIf generates includeIf config files.
	ModeIncludeIf

	// ModeDryRun doesn't apply anything, just reports what would happen.
	ModeDryRun
)

// Applier applies resolved configuration to git.
type Applier struct {
	mode     Mode
	settings config.Settings
}

// New creates a new Applier.
func New(mode Mode, settings config.Settings) *Applier {
	return &Applier{
		mode:     mode,
		settings: settings,
	}
}

// Result describes the outcome of applying configuration.
type Result struct {
	// Applied is true if configuration was applied
	Applied bool

	// Mode indicates how the config was applied
	Mode Mode

	// ConfigValues are the values that were applied
	ConfigValues map[string]string

	// EnvVars are the environment variables set (for ModeEnv)
	EnvVars map[string]string

	// IncludeIfPath is the path to generated includeIf file (for ModeIncludeIf)
	IncludeIfPath string

	// Messages are informational messages
	Messages []string

	// Errors encountered during application
	Errors []error
}

// Apply applies the resolved configuration.
func (a *Applier) Apply(resolution *resolver.Resolution) (*Result, error) {
	result := &Result{
		Mode:         a.mode,
		ConfigValues: resolution.FinalConfig,
		EnvVars:      make(map[string]string),
	}

	if len(resolution.FinalConfig) == 0 {
		result.Messages = append(result.Messages, "No configuration to apply")
		return result, nil
	}

	switch a.mode {
	case ModeDryRun:
		return a.applyDryRun(resolution, result)
	case ModeLocal:
		return a.applyLocal(resolution, result)
	case ModeEnv:
		return a.applyEnv(resolution, result)
	case ModeIncludeIf:
		return a.applyIncludeIf(resolution, result)
	default:
		return nil, fmt.Errorf("unknown apply mode: %d", a.mode)
	}
}

func (a *Applier) applyDryRun(resolution *resolver.Resolution, result *Result) (*Result, error) {
	result.Applied = false
	result.Messages = append(result.Messages, "Dry run - no changes made")

	for key, value := range resolution.FinalConfig {
		result.Messages = append(result.Messages, fmt.Sprintf("Would set %s = %s", key, value))
	}

	return result, nil
}

func (a *Applier) applyLocal(resolution *resolver.Resolution, result *Result) (*Result, error) {
	if resolution.Repository == nil {
		return nil, fmt.Errorf("cannot apply local config: not in a git repository")
	}

	for key, value := range resolution.FinalConfig {
		err := git.SetConfigInDir(resolution.Repository.Root, key, value, git.ConfigScopeLocal)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to set %s: %w", key, err))
		} else {
			result.Messages = append(result.Messages, fmt.Sprintf("Set %s = %s", key, value))
		}
	}

	result.Applied = len(result.Errors) == 0
	return result, nil
}

func (a *Applier) applyEnv(resolution *resolver.Resolution, result *Result) (*Result, error) {
	// Convert git config keys to environment variables
	// Git uses GIT_AUTHOR_NAME, GIT_AUTHOR_EMAIL, GIT_COMMITTER_NAME, GIT_COMMITTER_EMAIL
	// and GIT_CONFIG_KEY_N / GIT_CONFIG_VALUE_N for arbitrary config

	envMap := make(map[string]string)

	// Handle common user config specially
	if name, ok := resolution.FinalConfig["user.name"]; ok {
		envMap["GIT_AUTHOR_NAME"] = name
		envMap["GIT_COMMITTER_NAME"] = name
	}
	if email, ok := resolution.FinalConfig["user.email"]; ok {
		envMap["GIT_AUTHOR_EMAIL"] = email
		envMap["GIT_COMMITTER_EMAIL"] = email
	}
	if signingKey, ok := resolution.FinalConfig["user.signingkey"]; ok {
		envMap["GIT_SIGNING_KEY"] = signingKey
	}

	// For other config, use GIT_CONFIG_COUNT pattern (git 2.31+)
	count := 0
	for key, value := range resolution.FinalConfig {
		if key == "user.name" || key == "user.email" || key == "user.signingkey" {
			continue // Already handled
		}
		envMap[fmt.Sprintf("GIT_CONFIG_KEY_%d", count)] = key
		envMap[fmt.Sprintf("GIT_CONFIG_VALUE_%d", count)] = value
		count++
	}
	if count > 0 {
		envMap["GIT_CONFIG_COUNT"] = fmt.Sprintf("%d", count)
	}

	result.EnvVars = envMap
	result.Applied = true
	result.Messages = append(result.Messages, fmt.Sprintf("Set %d environment variables", len(envMap)))

	return result, nil
}

func (a *Applier) applyIncludeIf(resolution *resolver.Resolution, result *Result) (*Result, error) {
	if resolution.Repository == nil {
		return nil, fmt.Errorf("cannot generate includeIf: not in a git repository")
	}

	// Generate a unique filename based on repo path
	repoHash := hashPath(resolution.Repository.Root)
	filename := fmt.Sprintf("repo-%s.gitconfig", repoHash)
	includeIfDir := config.ExpandPath(a.settings.IncludeIfDir)

	// Ensure directory exists
	if err := os.MkdirAll(includeIfDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create includeIf directory: %w", err)
	}

	filePath := filepath.Join(includeIfDir, filename)

	// Generate git config content
	var content strings.Builder
	content.WriteString("# Auto-generated by auto-git-config\n")
	content.WriteString(fmt.Sprintf("# For repository: %s\n", resolution.Repository.Root))
	if resolution.SelectedRule != nil {
		content.WriteString(fmt.Sprintf("# Matched rule: %s\n", resolution.SelectedRule.Name))
	}
	content.WriteString("\n")

	for key, value := range resolution.FinalConfig {
		section, subkey := splitConfigKey(key)
		content.WriteString(fmt.Sprintf("[%s]\n", section))
		content.WriteString(fmt.Sprintf("    %s = %s\n", subkey, value))
	}

	// Write the file
	if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
		return nil, fmt.Errorf("failed to write includeIf file: %w", err)
	}

	result.IncludeIfPath = filePath
	result.Applied = true
	result.Messages = append(result.Messages, fmt.Sprintf("Generated includeIf config: %s", filePath))
	result.Messages = append(result.Messages, fmt.Sprintf("Add to your global gitconfig: [includeIf \"gitdir:%s/\"]\n    path = %s", resolution.Repository.Root, filePath))

	return result, nil
}

// GenerateIncludeIfConfig generates a git config snippet for includeIf.
func GenerateIncludeIfConfig(repoPath, configPath string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[includeIf \"gitdir:%s/\"]\n", repoPath))
	sb.WriteString(fmt.Sprintf("    path = %s\n", configPath))
	return sb.String()
}

// GenerateGlobalIncludeIfSetup generates the global gitconfig entries needed.
func GenerateGlobalIncludeIfSetup(settings config.Settings, resolutions []*resolver.Resolution) string {
	var sb strings.Builder
	sb.WriteString("# Add the following to your ~/.gitconfig\n\n")

	for _, res := range resolutions {
		if res.Repository == nil {
			continue
		}
		repoHash := hashPath(res.Repository.Root)
		filename := fmt.Sprintf("repo-%s.gitconfig", repoHash)
		filePath := filepath.Join(config.ExpandPath(settings.IncludeIfDir), filename)

		sb.WriteString(GenerateIncludeIfConfig(res.Repository.Root, filePath))
		sb.WriteString("\n")
	}

	return sb.String()
}

// splitConfigKey splits "section.key" into ("section", "key").
func splitConfigKey(key string) (string, string) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return key, ""
}

// hashPath creates a short hash of a path for filenames.
func hashPath(path string) string {
	// Simple hash: use first 8 chars of a deterministic string
	// Replace non-alphanumeric chars and truncate
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, path)

	if len(cleaned) > 32 {
		cleaned = cleaned[len(cleaned)-32:]
	}

	return cleaned
}

