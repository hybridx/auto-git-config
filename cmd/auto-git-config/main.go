package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hybridx/auto-git-config/internal/applier"
	"github.com/hybridx/auto-git-config/internal/config"
	"github.com/hybridx/auto-git-config/internal/resolver"
	"github.com/hybridx/auto-git-config/pkg/cache"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "auto-git-config",
		Short: "Automatic Git configuration based on context",
		Long: `auto-git-config automatically selects and applies the correct Git identity
and settings based on where you're working — matching by repository path,
remote host, organization, folder name, and more.`,
		SilenceUsage: true,
	}

	root.AddCommand(
		applyCmd(),
		explainCmd(),
		envCmd(),
		generateCmd(),
		initCmd(),
		cacheCmd(),
		versionCmd(),
	)

	return root
}

func loadConfig() (*config.Config, error) {
	cfgPath := config.DefaultConfigPath()
	if cfgPath == "" {
		return nil, fmt.Errorf("could not determine config path")
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", cfgPath, err)
	}
	return cfg, nil
}

func resolveConfig(cfg *config.Config) (*resolver.Resolution, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Check cache if enabled
	if cfg.Settings.CacheEnabled {
		configHash, _ := cache.HashFile(config.DefaultConfigPath())
		c := cache.New("", cfg.Settings.CacheTTLSeconds)
		entry, _ := c.Get(workDir, configHash, "")
		if entry != nil {
			return &resolver.Resolution{
				FinalConfig: entry.ResolvedConfig,
			}, nil
		}
	}

	r := resolver.New(cfg)
	resolution, err := r.Resolve(workDir)
	if err != nil {
		return nil, err
	}

	// Store in cache
	if cfg.Settings.CacheEnabled && len(resolution.FinalConfig) > 0 {
		configHash, _ := cache.HashFile(config.DefaultConfigPath())
		remotesHash := cache.HashRemotes(resolution.DebugInfo.Remotes)
		c := cache.New("", cfg.Settings.CacheTTLSeconds)

		matchedRule := ""
		if resolution.SelectedRule != nil {
			matchedRule = resolution.SelectedRule.Name
		}

		_ = c.Set(&cache.Entry{
			RepoRoot:       workDir,
			ConfigHash:     configHash,
			ResolvedConfig: resolution.FinalConfig,
			MatchedRule:    matchedRule,
			RemotesHash:    remotesHash,
		})
	}

	return resolution, nil
}

// --- apply ---

func applyCmd() *cobra.Command {
	var (
		local  bool
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply resolved configuration to the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			resolution, err := resolveConfig(cfg)
			if err != nil {
				return err
			}

			mode := applier.ModeIncludeIf
			if local {
				mode = applier.ModeLocal
			}
			if dryRun {
				mode = applier.ModeDryRun
			}

			a := applier.New(mode, cfg.Settings)
			result, err := a.Apply(resolution)
			if err != nil {
				return err
			}

			for _, msg := range result.Messages {
				fmt.Println(msg)
			}
			for _, e := range result.Errors {
				fmt.Fprintf(os.Stderr, "Error: %s\n", e)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Apply to local .git/config instead of includeIf")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without making changes")

	return cmd
}

// --- explain ---

func explainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain",
		Short: "Show detailed resolution information for debugging",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			workDir, err := os.Getwd()
			if err != nil {
				return err
			}

			r := resolver.New(cfg)
			resolution, err := r.ResolveForExplain(workDir)
			if err != nil {
				return err
			}

			fmt.Println("=== Context ===")
			fmt.Printf("Working directory: %s\n", resolution.DebugInfo.WorkDir)
			if resolution.DebugInfo.RepoRoot != "" {
				fmt.Printf("Repository root:  %s\n", resolution.DebugInfo.RepoRoot)
			}
			if resolution.Repository != nil {
				fmt.Printf("Folder name:      %s\n", resolution.Repository.FolderName())
			}

			if len(resolution.DebugInfo.Remotes) > 0 {
				fmt.Println("\n=== Remotes ===")
				for name, url := range resolution.DebugInfo.Remotes {
					fmt.Printf("  %s: %s\n", name, url)
					if resolution.Repository != nil {
						if remote := resolution.Repository.GetRemote(name); remote != nil {
							fmt.Printf("    → host: %s, owner: %s, repo: %s\n", remote.Host, remote.Owner, remote.Repo)
						}
					}
				}
			}

			fmt.Println("\n=== Matched Rules ===")
			if len(resolution.MatchedRules) == 0 {
				fmt.Println("  (no rules matched)")
			}
			for i, m := range resolution.MatchedRules {
				marker := "  "
				if i == 0 {
					marker = "→ "
				}
				fmt.Printf("%s[%d] %s (type: %s, score: %d)\n",
					marker, i+1, m.Rule.Name, m.MatchType, m.Score)
				fmt.Printf("      %s\n", m.MatchDetails)
			}

			if len(resolution.DebugInfo.RejectedRules) > 0 {
				fmt.Println("\n=== Rejected Rules ===")
				for _, r := range resolution.DebugInfo.RejectedRules {
					fmt.Printf("  ✗ %s: %s\n", r.RuleName, r.Reason)
				}
			}

			fmt.Println("\n=== Final Configuration ===")
			if len(resolution.FinalConfig) == 0 {
				fmt.Println("  (no configuration resolved)")
			}
			for key, value := range resolution.FinalConfig {
				fmt.Printf("  %s = %s\n", key, value)
			}

			if resolution.DefaultApplied {
				fmt.Println("\n  (includes defaults)")
			}

			return nil
		},
	}
}

// --- env ---

func envCmd() *cobra.Command {
	var shell string

	cmd := &cobra.Command{
		Use:   "env",
		Short: "Output shell environment variables for integration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv("AUTO_GIT_CONFIG_DISABLE") == "1" {
				return nil
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			resolution, err := resolveConfig(cfg)
			if err != nil {
				return err
			}

			a := applier.New(applier.ModeEnv, cfg.Settings)
			result, err := a.Apply(resolution)
			if err != nil {
				return err
			}

			for k, v := range result.EnvVars {
				switch shell {
				case "fish":
					fmt.Printf("set -gx %s %q;\n", k, v)
				case "powershell":
					fmt.Printf("$env:%s = %q\n", k, v)
				default:
					fmt.Printf("export %s=%q\n", k, v)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&shell, "shell", "", "Shell format: bash, zsh, fish, powershell (default: bash/zsh)")

	return cmd
}

// --- generate ---

func generateCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate includeIf configuration files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			if global {
				fmt.Print(applier.GenerateGlobalIncludeIfSetup(cfg.Settings, nil))
				return nil
			}

			resolution, err := resolveConfig(cfg)
			if err != nil {
				return err
			}

			a := applier.New(applier.ModeIncludeIf, cfg.Settings)
			result, err := a.Apply(resolution)
			if err != nil {
				return err
			}

			for _, msg := range result.Messages {
				fmt.Println(msg)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "Generate global gitconfig snippet")

	return cmd
}

// --- init ---

func initCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create initial configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := config.DefaultConfigPath()
			if cfgPath == "" {
				return fmt.Errorf("could not determine config path")
			}

			if !force {
				if _, err := os.Stat(cfgPath); err == nil {
					return fmt.Errorf("config already exists at %s (use --force to overwrite)", cfgPath)
				}
			}

			dir := filepath.Dir(cfgPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			if err := os.WriteFile(cfgPath, []byte(defaultConfig()), 0644); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}

			fmt.Printf("Configuration created at %s\n", cfgPath)
			fmt.Println("Edit the file to configure your Git identities.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing configuration")

	return cmd
}

// --- cache ---

func cacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the resolution cache",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "show",
			Short: "Show cache entry for the current repository",
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}

				workDir, err := os.Getwd()
				if err != nil {
					return err
				}

				configHash, _ := cache.HashFile(config.DefaultConfigPath())
				c := cache.New("", cfg.Settings.CacheTTLSeconds)
				entry, err := c.Get(workDir, configHash, "")
				if err != nil {
					return err
				}

				if entry == nil {
					fmt.Println("No cache entry for current directory")
					return nil
				}

				fmt.Printf("Cached at: %s\n", entry.CachedAt.Format(time.RFC3339))
				fmt.Printf("Matched rule: %s\n", entry.MatchedRule)
				fmt.Println("Resolved config:")
				for k, v := range entry.ResolvedConfig {
					fmt.Printf("  %s = %s\n", k, v)
				}

				return nil
			},
		},
		&cobra.Command{
			Use:   "clear",
			Short: "Clear all cache entries",
			RunE: func(cmd *cobra.Command, args []string) error {
				c := cache.New("", 0)
				if err := c.Clear(); err != nil {
					return err
				}
				fmt.Println("Cache cleared")
				return nil
			},
		},
	)

	return cmd
}

// --- version ---

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("auto-git-config %s\n", version)
			fmt.Printf("  commit:  %s\n", commit)
			fmt.Printf("  built:   %s\n", buildDate)
		},
	}
}

func defaultConfig() string {
	var sb strings.Builder
	sb.WriteString(`# auto-git-config configuration
# See: https://github.com/hybridx/auto-git-config

version = "1"

[settings]
cache_enabled = true
cache_ttl_seconds = 300
default_remote = "origin"
debug = false

# Default configuration when no rules match
[default.user]
name = "Your Name"
email = "your@email.com"

# Example: personal GitHub identity
[[rule]]
name = "github-personal"

[rule.match]
remote_host = "github.com"

[rule.config.user]
name = "Your Name"
email = "your-personal@email.com"

# Example: work identity for repos under ~/work
# [[rule]]
# name = "work"
# priority = 10
#
# [rule.match]
# path_prefix = "~/work"
#
# [rule.config.user]
# name = "Your Work Name"
# email = "you@company.com"
#
# [rule.config.commit]
# gpgsign = true
`)
	return sb.String()
}
