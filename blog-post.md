# I Built a Tool That Automatically Switches My Git Identity — Here's Why and How

If you've ever pushed a work commit with your personal email, or realized mid-PR that your corporate GitLab commits are signed with `alice@gmail.com`, you know the pain. I've made this mistake more times than I'd like to admit.

Git's built-in solution — `includeIf` with `gitdir` — works, but it forces you to organize repos by directory. Real life isn't that clean. I have GitHub repos in three different folders. I have a work GitLab and a personal GitHub that both live under `~/projects/`. Directory-based matching doesn't cut it.

So I built **auto-git-config** — a CLI tool that automatically applies the right Git identity based on which remote your repo points to. No more wrong-email commits.

## The Problem in 30 Seconds

Most developers work across multiple Git identities:

- **Personal** — `alice@gmail.com` on GitHub
- **Work** — `alice@company.com` on corporate GitLab
- **OSS contributions** — `alice@cool-oss.org` on a specific GitHub org

Git has no built-in way to say "use this email when the remote is `gitlab.company.com`." You either:

1. Manually run `git config user.email` in every new repo (forget once, wrong email forever)
2. Use `includeIf` with `gitdir` (requires all work repos in one folder — what if you clone one elsewhere?)
3. Set up elaborate shell scripts that break on the next macOS update

None of these are great.

## What auto-git-config Does

You write rules in a single TOML config file. Each rule says "when the repo matches *this condition*, use *this identity*." Conditions can be the remote host, organization, path, folder name, or regex patterns — and they AND together.

```toml
# ~/.config/auto-git-config/config.toml

version = "1"

[default.user]
name = "Alice"
email = "alice@gmail.com"

# Personal GitHub
[[rule]]
name = "github-personal"
[rule.match]
remote_host = "github.com"
[rule.config.user]
email = "alice@gmail.com"

# Work GitLab
[[rule]]
name = "work"
priority = 10
[rule.match]
remote_host = "gitlab.company.com"
[rule.config.user]
email = "alice@company.com"
[rule.config.commit]
gpgsign = true
```

Then a two-line shell hook applies the right identity every time your prompt renders:

```bash
# Add to ~/.zshrc
auto_git_config_hook() { eval "$(auto-git-config env 2>/dev/null)"; }
precmd_functions+=(auto_git_config_hook)
```

That's it. When you `cd` into a repo with a `gitlab.company.com` remote, your Git identity silently switches to your work email. `cd` into a GitHub repo, it switches back. You never think about it again.

## How It Actually Works Under the Hood

When the shell hook fires, `auto-git-config env` runs through five stages in under 50ms:

### 1. Detect Context

It runs `git rev-parse --show-toplevel` to find the repo root, then `git remote -v` to get all remotes. Both SSH and HTTPS URLs are normalized into the same components:

```
git@github.com:alice/project.git
https://github.com/alice/project.git
ssh://git@github.com/alice/project.git
```

All three become: `host=github.com, owner=alice, repo=project`.

This means your rules work regardless of whether someone cloned with SSH or HTTPS.

### 2. Match Rules

Each rule has match conditions. When a rule has multiple conditions, they're AND-ed together — all must match.

The matching engine uses a strategy pattern with eight pluggable matchers:

| Matcher | Example | Score |
|---------|---------|-------|
| `repo_path` | Exact path to a specific repo | 1000 |
| `remote_url` | Regex on the full remote URL | 600 |
| `remote_org` | GitHub/GitLab organization | 550 |
| `remote_host` | The git server hostname | 500 |
| `folder_name` | Exact folder name | 200 |
| `folder_pattern` | Regex on folder name | 150 |
| `path_prefix` | Directory tree prefix | 100+ |
| `path_contains` | Substring in the path | 50 |

Scores are used for tie-breaking when multiple rules match at the same priority level.

### 3. Resolve Precedence

When multiple rules match, the winner is picked deterministically:

1. Explicit `priority` field (higher wins)
2. Match type (repo-specific > remote-based > path-based)
3. Match score (more specific wins)
4. Definition order in the config file

### 4. Apply via Environment Variables

The winning rule's config is translated to Git environment variables:

```bash
export GIT_AUTHOR_NAME="Alice"
export GIT_COMMITTER_NAME="Alice"
export GIT_AUTHOR_EMAIL="alice@company.com"
export GIT_COMMITTER_EMAIL="alice@company.com"
```

Git natively reads these variables and they **override** anything in `.gitconfig`. The key insight: we never modify any config files. Everything is environment-variable-based, which means:

- It's instant to apply and remove
- No file permissions issues
- Survives any git config resets
- Easy to debug (`echo $GIT_AUTHOR_EMAIL`)

### 5. Cache

The resolution result is cached to disk (SHA256-keyed by repo root) with a 5-minute TTL. The cache auto-invalidates when:

- The config file changes
- A remote URL changes
- The TTL expires

In practice, only the first `cd` into a repo does any real work. Subsequent prompts are a fast cache lookup.

## The `explain` Command — My Favorite Feature

When something seems wrong, `auto-git-config explain` shows the full decision trace:

```
=== Context ===
Working directory: /home/alice/work/api-service
Repository root:  /home/alice/work/api-service
Folder name:      api-service

=== Remotes ===
  origin: git@gitlab.company.com:backend/api-service.git
    → host: gitlab.company.com, owner: backend, repo: api-service

=== Matched Rules ===
→ [1] work (type: remote, score: 500)
      remote_host: matched gitlab.company.com

=== Rejected Rules ===
  ✗ github-personal: remote_host: gitlab.company.com != github.com

=== Final Configuration ===
  user.name = Alice
  user.email = alice@company.com
  commit.gpgsign = true
```

You can see exactly which rules matched, which were rejected and why, and what the final resolved config is. No more guessing.

## Real-World Configuration Examples

### Multiple GitHub Organizations

```toml
# Personal repos on GitHub
[[rule]]
name = "github-personal"
[rule.match]
remote_host = "github.com"
[rule.config.user]
email = "alice@gmail.com"

# Work repos on the same GitHub, different org
[[rule]]
name = "github-work"
priority = 5
[rule.match]
remote_host = "github.com"
remote_org = "my-company"
[rule.config.user]
email = "alice@company.com"
```

The `github-work` rule has higher priority AND requires both conditions (host + org) to match. So company repos get the work email, everything else on GitHub gets personal.

### Path-Based Override

```toml
# Any repo under ~/freelance uses this identity,
# regardless of where it's hosted
[[rule]]
name = "freelance"
priority = 20
[rule.match]
path_prefix = "~/freelance"
[rule.config.user]
name = "Alice Consulting LLC"
email = "alice@freelance.dev"
```

### Secret Project with Anonymous Identity

```toml
[[rule]]
name = "anon-project"
priority = 100
[rule.match]
repo_path = "~/projects/whistleblower-tool"
[rule.config.user]
name = "Anonymous"
email = "anon-42@users.noreply.github.com"
[rule.config.commit]
gpgsign = true
```

## Installation

It's a single Go binary:

```bash
go install github.com/hybridx/auto-git-config/cmd/auto-git-config@latest
```

Initialize a config file:

```bash
auto-git-config init
```

Edit your rules:

```bash
$EDITOR ~/.config/auto-git-config/config.toml
```

Add the shell hook to `~/.zshrc` (or `~/.bashrc`):

```bash
# Go binary path (if not already in PATH)
export PATH="$HOME/go/bin:$PATH"

# Auto-apply Git identity per repo
auto_git_config_hook() { eval "$(auto-git-config env 2>/dev/null)"; }
precmd_functions+=(auto_git_config_hook)
```

Verify:

```bash
cd ~/some-repo
auto-git-config explain
```

## Design Decisions and Tradeoffs

**Why environment variables instead of `includeIf`?**

`includeIf` only supports `gitdir` (path-based matching). It can't match by remote host or organization. Environment variables let us match on anything and apply instantly without touching any files.

**Why a TOML config instead of CLI flags?**

Your identity rules are something you set up once and forget. A config file is the right UX for that — version-controllable, easy to share across machines, and you can see all your rules at a glance.

**Why not just a shell function?**

I started with a shell function. It quickly became 200 lines of fragile bash parsing SSH URLs, handling edge cases, and reimplementing caching. Go gives us proper URL parsing, regex support, clean error handling, and a single binary that works on any OS.

**Why cache to disk instead of memory?**

The tool runs as a subprocess on every prompt — there's no long-lived process to hold memory cache. Disk cache with SHA256 keys and TTL gives us persistence across shell sessions.

## What I Learned Building This

1. **Git's URL formats are a mess.** SCP-style (`git@host:path`), SSH URL-style (`ssh://git@host/path`), HTTPS, HTTP, git protocol — each needs its own parser. And some hosts use non-standard ports.

2. **Map iteration order in Go is random** and will silently break your cache hashing if you forget to sort keys. I shipped this bug and only caught it because cache hit rates were suspiciously low.

3. **The `git config --get` command ignores environment variables.** So `git config user.email` will show your global config even when commits actually use the env var. This confused me for an hour. Use `git var GIT_AUTHOR_IDENT` to see what git will actually use.

4. **TOML doesn't allow redefining table sections.** `[rule.config.user]` appearing twice in the same rule is a parse error, not a merge. This seems obvious in retrospect but it's easy to miss in example configs.

## Try It Out

The project is open source: [github.com/hybridx/auto-git-config](https://github.com/hybridx/auto-git-config)

If you maintain multiple Git identities and have ever pushed a commit with the wrong email, give it a spin. Setup takes about two minutes, and then you never think about it again.

---

*auto-git-config is MIT licensed. Contributions welcome.*
