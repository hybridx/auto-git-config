// Package matcher provides rule matching logic for auto-git-config.
package matcher

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/denair/auto-git-config/internal/config"
	"github.com/denair/auto-git-config/internal/git"
)

// Context holds all information needed for rule matching.
type Context struct {
	// Repository is the detected git repository (may be nil if not in a repo)
	Repository *git.Repository

	// WorkDir is the current working directory
	WorkDir string

	// Settings from config
	Settings config.Settings
}

// MatchResult represents the outcome of matching a rule.
type MatchResult struct {
	// Rule is the matched rule
	Rule *config.Rule

	// Matched indicates if the rule matched
	Matched bool

	// MatchType indicates which type of match succeeded
	MatchType config.MatchType

	// MatchDetails provides human-readable match information
	MatchDetails string

	// Score is used for tie-breaking when multiple rules match
	// Higher score = more specific match
	Score int
}

// Matcher evaluates rules against a context.
type Matcher struct {
	strategies []Strategy
}

// Strategy is an interface for pluggable matching strategies.
type Strategy interface {
	// Name returns the strategy name for debugging
	Name() string

	// Matches checks if the match condition applies
	Matches(match *config.Match, ctx *Context) (bool, string, int)
}

// NewMatcher creates a new matcher with default strategies.
func NewMatcher() *Matcher {
	return &Matcher{
		strategies: []Strategy{
			&RepoPathStrategy{},
			&RemoteHostStrategy{},
			&RemoteURLStrategy{},
			&RemoteOrgStrategy{},
			&PathPrefixStrategy{},
			&PathContainsStrategy{},
			&FolderNameStrategy{},
			&FolderPatternStrategy{},
		},
	}
}

// Match evaluates a single rule against the context.
func (m *Matcher) Match(rule *config.Rule, ctx *Context) MatchResult {
	if !rule.IsEnabled() {
		return MatchResult{Rule: rule, Matched: false, MatchDetails: "rule disabled"}
	}

	result := MatchResult{
		Rule:      rule,
		Matched:   true,
		MatchType: rule.Match.MatchType(),
		Score:     0,
	}

	var details []string

	// All non-empty conditions must match (AND logic)
	for _, strategy := range m.strategies {
		matched, detail, score := strategy.Matches(&rule.Match, ctx)
		if detail != "" {
			// This strategy had something to check
			if !matched {
				return MatchResult{
					Rule:         rule,
					Matched:      false,
					MatchDetails: detail,
				}
			}
			details = append(details, detail)
			result.Score += score
		}
	}

	// If no conditions were specified, rule doesn't match
	if len(details) == 0 {
		return MatchResult{
			Rule:         rule,
			Matched:      false,
			MatchDetails: "no match conditions specified",
		}
	}

	result.MatchDetails = strings.Join(details, "; ")
	return result
}

// MatchAll evaluates all rules and returns matching results.
func (m *Matcher) MatchAll(rules []config.Rule, ctx *Context) []MatchResult {
	var results []MatchResult
	for i := range rules {
		result := m.Match(&rules[i], ctx)
		if result.Matched {
			results = append(results, result)
		}
	}
	return results
}

// --- Strategy Implementations ---

// RepoPathStrategy matches exact repository paths.
type RepoPathStrategy struct{}

func (s *RepoPathStrategy) Name() string { return "repo_path" }

func (s *RepoPathStrategy) Matches(match *config.Match, ctx *Context) (bool, string, int) {
	if match.RepoPath == "" {
		return false, "", 0
	}

	if ctx.Repository == nil {
		return false, "repo_path: not in a git repository", 0
	}

	expectedPath := config.ExpandPath(match.RepoPath)
	expectedPath, _ = filepath.Abs(expectedPath)

	if ctx.Repository.Root == expectedPath {
		return true, "repo_path: exact match", 1000
	}

	return false, "repo_path: " + ctx.Repository.Root + " != " + expectedPath, 0
}

// RemoteHostStrategy matches remote hosts.
type RemoteHostStrategy struct{}

func (s *RemoteHostStrategy) Name() string { return "remote_host" }

func (s *RemoteHostStrategy) Matches(match *config.Match, ctx *Context) (bool, string, int) {
	if match.RemoteHost == "" {
		return false, "", 0
	}

	if ctx.Repository == nil {
		return false, "remote_host: not in a git repository", 0
	}

	remoteName := match.RemoteName
	if remoteName == "" {
		remoteName = ctx.Settings.DefaultRemote
	}

	remote := ctx.Repository.GetRemote(remoteName)
	if remote == nil {
		// Try primary remote as fallback
		remote = ctx.Repository.GetPrimaryRemote()
	}
	if remote == nil {
		return false, "remote_host: no remotes configured", 0
	}

	if remote.MatchesHost(match.RemoteHost) {
		return true, "remote_host: matched " + remote.Host, 500
	}

	return false, "remote_host: " + remote.Host + " != " + match.RemoteHost, 0
}

// RemoteURLStrategy matches remote URLs with regex.
type RemoteURLStrategy struct{}

func (s *RemoteURLStrategy) Name() string { return "remote_url" }

func (s *RemoteURLStrategy) Matches(match *config.Match, ctx *Context) (bool, string, int) {
	if match.RemoteURL == "" {
		return false, "", 0
	}

	if ctx.Repository == nil {
		return false, "remote_url: not in a git repository", 0
	}

	pattern, err := regexp.Compile(match.RemoteURL)
	if err != nil {
		return false, "remote_url: invalid pattern: " + err.Error(), 0
	}

	remoteName := match.RemoteName
	if remoteName == "" {
		remoteName = ctx.Settings.DefaultRemote
	}

	remote := ctx.Repository.GetRemote(remoteName)
	if remote == nil {
		remote = ctx.Repository.GetPrimaryRemote()
	}
	if remote == nil {
		return false, "remote_url: no remotes configured", 0
	}

	if pattern.MatchString(remote.URL) {
		return true, "remote_url: matched pattern", 600
	}

	return false, "remote_url: pattern did not match", 0
}

// RemoteOrgStrategy matches organization/owner in remote.
type RemoteOrgStrategy struct{}

func (s *RemoteOrgStrategy) Name() string { return "remote_org" }

func (s *RemoteOrgStrategy) Matches(match *config.Match, ctx *Context) (bool, string, int) {
	if match.RemoteOrg == "" {
		return false, "", 0
	}

	if ctx.Repository == nil {
		return false, "remote_org: not in a git repository", 0
	}

	remoteName := match.RemoteName
	if remoteName == "" {
		remoteName = ctx.Settings.DefaultRemote
	}

	remote := ctx.Repository.GetRemote(remoteName)
	if remote == nil {
		remote = ctx.Repository.GetPrimaryRemote()
	}
	if remote == nil {
		return false, "remote_org: no remotes configured", 0
	}

	if remote.MatchesOrg(match.RemoteOrg) {
		return true, "remote_org: matched " + remote.Owner, 550
	}

	return false, "remote_org: " + remote.Owner + " != " + match.RemoteOrg, 0
}

// PathPrefixStrategy matches directory path prefixes.
type PathPrefixStrategy struct{}

func (s *PathPrefixStrategy) Name() string { return "path_prefix" }

func (s *PathPrefixStrategy) Matches(match *config.Match, ctx *Context) (bool, string, int) {
	if match.PathPrefix == "" {
		return false, "", 0
	}

	prefix := config.ExpandPath(match.PathPrefix)
	prefix, _ = filepath.Abs(prefix)

	checkPath := ctx.WorkDir
	if ctx.Repository != nil {
		checkPath = ctx.Repository.Root
	}

	if strings.HasPrefix(checkPath, prefix) {
		// Score based on prefix length (more specific = higher score)
		score := 100 + len(prefix)
		return true, "path_prefix: matched " + prefix, score
	}

	return false, "path_prefix: " + checkPath + " does not start with " + prefix, 0
}

// PathContainsStrategy matches paths containing a substring.
type PathContainsStrategy struct{}

func (s *PathContainsStrategy) Name() string { return "path_contains" }

func (s *PathContainsStrategy) Matches(match *config.Match, ctx *Context) (bool, string, int) {
	if match.PathContains == "" {
		return false, "", 0
	}

	checkPath := ctx.WorkDir
	if ctx.Repository != nil {
		checkPath = ctx.Repository.Root
	}

	if strings.Contains(checkPath, match.PathContains) {
		return true, "path_contains: found " + match.PathContains, 50
	}

	return false, "path_contains: " + match.PathContains + " not found in path", 0
}

// FolderNameStrategy matches exact folder names.
type FolderNameStrategy struct{}

func (s *FolderNameStrategy) Name() string { return "folder_name" }

func (s *FolderNameStrategy) Matches(match *config.Match, ctx *Context) (bool, string, int) {
	if match.FolderName == "" {
		return false, "", 0
	}

	checkPath := ctx.WorkDir
	if ctx.Repository != nil {
		checkPath = ctx.Repository.Root
	}

	folderName := filepath.Base(checkPath)

	if strings.EqualFold(folderName, match.FolderName) {
		return true, "folder_name: matched " + folderName, 200
	}

	return false, "folder_name: " + folderName + " != " + match.FolderName, 0
}

// FolderPatternStrategy matches folder names with regex.
type FolderPatternStrategy struct{}

func (s *FolderPatternStrategy) Name() string { return "folder_pattern" }

func (s *FolderPatternStrategy) Matches(match *config.Match, ctx *Context) (bool, string, int) {
	if match.FolderPattern == "" {
		return false, "", 0
	}

	pattern, err := regexp.Compile(match.FolderPattern)
	if err != nil {
		return false, "folder_pattern: invalid pattern: " + err.Error(), 0
	}

	checkPath := ctx.WorkDir
	if ctx.Repository != nil {
		checkPath = ctx.Repository.Root
	}

	folderName := filepath.Base(checkPath)

	if pattern.MatchString(folderName) {
		return true, "folder_pattern: matched pattern", 150
	}

	return false, "folder_pattern: pattern did not match " + folderName, 0
}

