# auto-git-config

**Automatic Git configuration based on context** — automatically select and apply the correct Git identity and settings based on where you're working.

## Overview

`auto-git-config` solves the common problem of maintaining multiple Git identities (personal, work, open source contributions) by automatically detecting context and applying the appropriate configuration.

### Key Features

- **Context-aware configuration** — Match by repository path, folder name, Git remote host, organization, or URL patterns
- **Multiple integration options** — Git includeIf, environment variables, local config, or shell integration
- **Safe by default** — Never permanently modifies global config unless explicitly requested
- **Fast execution** — Caching layer ensures minimal overhead on every Git command
- **Transparent debugging** — `explain` command shows exactly how configuration is resolved
- **Escape hatch** — Disable with `AUTO_GIT_CONFIG_DISABLE=1` environment variable

## Installation

### From Source

```bash
go install github.com/denair/auto-git-config/cmd/auto-git-config@latest
```

### Manual Build

```bash
git clone https://github.com/denair/auto-git-config.git
cd auto-git-config
go build -o auto-git-config ./cmd/auto-git-config
```

## Quick Start

1. **Initialize configuration:**

   ```bash
   auto-git-config init
   ```

   This creates `~/.config/auto-git-config/config.toml` with example rules.

2. **Edit the configuration** to match your needs:

   ```bash
   $EDITOR ~/.config/auto-git-config/config.toml
   ```

3. **Verify resolution** in any repository:

   ```bash
   cd ~/work/my-project
   auto-git-config explain
   ```

4. **Apply configuration** using your preferred method (see Integration section).

## Architecture

### Rule Evaluation Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                         auto-git-config                              │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   ┌─────────────┐     ┌─────────────┐     ┌─────────────┐          │
│   │   Detect    │────▶│    Match    │────▶│   Resolve   │          │
│   │   Context   │     │    Rules    │     │   Config    │          │
│   └─────────────┘     └─────────────┘     └─────────────┘          │
│          │                   │                   │                  │
│          ▼                   ▼                   ▼                  │
│   • Working dir       • Path-based        • Merge matched          │
│   • Git repo root     • Remote-based        configs                │
│   • Git remotes       • Repo-specific     • Apply precedence       │
│   • Remote parsing    • AND logic         • Return final config    │
│                                                                      │
├─────────────────────────────────────────────────────────────────────┤
│                        Application Layer                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                │
│   │  IncludeIf  │  │   Local     │  │ Environment │                │
│   │  Generation │  │   Config    │  │  Variables  │                │
│   └─────────────┘  └─────────────┘  └─────────────┘                │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Description |
|-----------|-------------|
| **Config Parser** | Loads and validates TOML configuration with schema enforcement |
| **Git Module** | Detects repository, parses and normalizes remote URLs (SSH/HTTPS) |
| **Matcher** | Pluggable strategy system for rule evaluation with AND logic |
| **Resolver** | Applies precedence rules and merges configurations |
| **Applier** | Safely applies config via multiple methods |
| **Cache** | File-based cache with TTL and invalidation |

### Rule Precedence

Rules are evaluated in a deterministic precedence order:

1. **Explicit priority** — Rules with higher `priority` values are checked first
2. **Match type precedence:**
   - Repository path (`repo_path`) — Highest
   - Remote-based (`remote_host`, `remote_url`, `remote_org`)
   - Path-based (`path_prefix`, `folder_name`, etc.) — Lowest
3. **Match specificity** — More specific matches score higher
4. **Definition order** — Earlier rules win when all else is equal

### Remote URL Normalization

Both SSH and HTTPS URLs are normalized to extract:

| URL Type | Example | Normalized |
|----------|---------|------------|
| HTTPS | `https://github.com/owner/repo.git` | `github.com/owner/repo` |
| SSH (SCP) | `git@github.com:owner/repo.git` | `github.com/owner/repo` |
| SSH (URL) | `ssh://git@github.com/owner/repo.git` | `github.com/owner/repo` |

This allows rules to match regardless of how the remote was configured.

## Configuration

Configuration uses TOML format at `~/.config/auto-git-config/config.toml`.

### Schema

```toml
version = "1"

[settings]
cache_enabled = true
cache_ttl_seconds = 300
default_remote = "origin"
includeif_dir = "~/.config/auto-git-config/gitconfig.d"
debug = false

[default]
# Default git config when no rules match
[default.user]
name = "Default Name"
email = "default@example.com"

[[rule]]
name = "rule-identifier"     # Required: unique name
priority = 0                  # Optional: higher = checked first
enabled = true                # Optional: disable without removing

[rule.match]                  # At least one condition required
repo_path = "~/exact/path"    # Exact repository path
remote_host = "github.com"    # Remote host (supports wildcards: *.company.com)
remote_url = ".*pattern.*"    # Remote URL regex
remote_org = "organization"   # Remote owner/org
remote_name = "upstream"      # Which remote to check (default: origin)
path_prefix = "~/work"        # Path prefix (supports ~)
path_contains = "projects"    # Path must contain string
folder_name = "my-project"    # Exact folder name
folder_pattern = "^oss-.*"    # Folder name regex

[rule.config]                 # Git configuration to apply
[rule.config.user]
name = "User Name"
email = "email@example.com"
signingkey = "KEYID"

[rule.config.commit]
gpgsign = true
template = "~/.gitmessage"
verbose = true

[rule.config.core]
editor = "vim"
autocrlf = "input"
excludesfile = "~/.gitignore"

[rule.config.extra]           # Arbitrary git config keys
"pull.rebase" = "true"
"push.autoSetupRemote" = "true"
```

### Example Configurations

#### Personal vs Work by Remote

```toml
[[rule]]
name = "github-personal"
[rule.match]
remote_host = "github.com"
[rule.config.user]
name = "Alice Personal"
email = "alice@gmail.com"

[[rule]]
name = "gitlab-work"
[rule.match]
remote_host = "gitlab.company.com"
[rule.config.user]
name = "Alice Professional"
email = "alice@company.com"
[rule.config.commit]
gpgsign = true
```

#### Work Directory Override

```toml
[[rule]]
name = "work-directory"
priority = 10  # Higher than remote-based
[rule.match]
path_prefix = "~/work"
[rule.config.user]
name = "Alice Work"
email = "alice@company.com"
```

#### Specific Organization

```toml
[[rule]]
name = "oss-org"
[rule.match]
remote_host = "github.com"
remote_org = "awesome-oss"
[rule.config.user]
email = "alice@awesome-oss.org"
```

## CLI Commands

### `auto-git-config apply`

Apply resolved configuration to the current repository.

```bash
# Generate includeIf config (recommended)
auto-git-config apply

# Apply to local .git/config
auto-git-config apply --local

# Preview without changes
auto-git-config apply --dry-run
```

### `auto-git-config explain`

Show detailed resolution information for debugging.

```bash
auto-git-config explain

# Example output:
# === Context ===
# Working directory: /home/user/work/project
# Repository root: /home/user/work/project
# Folder name: project
#
# === Remotes ===
#   origin: git@github.com:company/project.git
#     → host: github.com, owner: company, repo: project
#
# === Matched Rules ===
# → [1] work-directory (type: path, score: 118)
#       path_prefix: matched /home/user/work
#   [2] github (type: remote, score: 500)
#       remote_host: matched github.com
#
# === Final Configuration ===
#   user.name = Alice Work
#   user.email = alice@company.com
#   commit.gpgsign = true
```

### `auto-git-config generate`

Generate includeIf configuration files.

```bash
# Generate config for current repo
auto-git-config generate

# Generate global gitconfig snippet
auto-git-config generate --global
```

### `auto-git-config env`

Output shell environment variables for integration.

```bash
# Bash/Zsh
eval "$(auto-git-config env)"

# Fish
auto-git-config env --shell fish | source

# PowerShell
auto-git-config env --shell powershell | Invoke-Expression
```

### `auto-git-config init`

Create initial configuration file.

```bash
auto-git-config init
auto-git-config init --force  # Overwrite existing
```

### `auto-git-config cache`

Manage the resolution cache.

```bash
auto-git-config cache show   # Show cache for current repo
auto-git-config cache clear  # Clear all cache entries
```

## Integration Methods

### 1. Git includeIf (Recommended)

The safest and most Git-native integration. Configuration is loaded conditionally without modifying global config.

**Setup:**

1. Generate includeIf configs for your repositories:

   ```bash
   cd ~/work/project && auto-git-config apply
   ```

2. Add to your `~/.gitconfig`:

   ```ini
   [includeIf "gitdir:~/work/project/"]
       path = ~/.config/auto-git-config/gitconfig.d/repo-xxxxx.gitconfig
   ```

3. Or use directory-based includeIf for all repos in a path:

   ```ini
   [includeIf "gitdir:~/work/"]
       path = ~/.config/auto-git-config/work.gitconfig
   ```

**Pros:** Native Git feature, zero runtime overhead, survives tool uninstall
**Cons:** Requires manual gitconfig editing, doesn't auto-update on config changes

### 2. Shell Integration

Automatically set environment variables when changing directories.

**Bash/Zsh:**

```bash
# Add to ~/.bashrc or ~/.zshrc
auto_git_config_hook() {
    eval "$(auto-git-config env 2>/dev/null)"
}

# For Bash:
PROMPT_COMMAND="auto_git_config_hook; $PROMPT_COMMAND"

# For Zsh:
precmd_functions+=(auto_git_config_hook)
```

**Fish:**

```fish
# Add to ~/.config/fish/config.fish
function __auto_git_config_hook --on-variable PWD
    auto-git-config env --shell fish 2>/dev/null | source
end
```

**Pros:** Automatic, updates immediately on directory change
**Cons:** Slight shell startup overhead, environment-based (not persisted)

### 3. Git Wrapper

Wrap git commands to apply configuration before execution.

```bash
# ~/bin/git
#!/bin/bash
eval "$(auto-git-config env 2>/dev/null)"
exec /usr/bin/git "$@"
```

**Pros:** Works with all Git operations
**Cons:** Adds latency to every Git command

### 4. Git Hooks

Apply configuration on specific Git operations.

```bash
# .git/hooks/pre-commit
#!/bin/bash
auto-git-config apply --local --quiet
```

**Pros:** Ensures correct config before commits
**Cons:** Only applies at hook execution time

## Environment Variables

| Variable | Description |
|----------|-------------|
| `AUTO_GIT_CONFIG_DISABLE=1` | Completely disable auto-git-config |
| `XDG_CONFIG_HOME` | Override config directory (default: `~/.config`) |

## Caching

Resolution results are cached to minimize overhead:

- **Location:** `$TMPDIR/auto-git-config-cache/` or `/tmp/auto-git-config-cache/`
- **TTL:** Configurable, default 5 minutes
- **Invalidation:** Automatic on config file changes or remote URL changes

## Tradeoffs

| Approach | Pros | Cons |
|----------|------|------|
| **includeIf** | Native, zero runtime cost, survives uninstall | Manual setup, doesn't auto-update |
| **Shell hook** | Automatic, immediate updates | Shell-specific, slight overhead |
| **Git wrapper** | Universal, always applies | Latency on every command |
| **Env variables** | No git modification needed | Doesn't affect all scenarios |
| **Local config** | Permanent per-repo | Modifies repo config |

## Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Submit a pull request

## License

MIT License — see LICENSE file for details.
