// Package resolver handles configuration resolution with precedence rules.
package resolver

import (
	"sort"

	"github.com/hybridx/auto-git-config/internal/config"
	"github.com/hybridx/auto-git-config/internal/git"
	"github.com/hybridx/auto-git-config/internal/matcher"
)

// Resolution represents the outcome of configuration resolution.
type Resolution struct {
	// FinalConfig is the merged configuration to apply
	FinalConfig map[string]string

	// MatchedRules are the rules that matched, in precedence order
	MatchedRules []matcher.MatchResult

	// SelectedRule is the highest-precedence matched rule (nil if none)
	SelectedRule *config.Rule

	// DefaultApplied indicates if default config was used
	DefaultApplied bool

	// Repository is the detected repository
	Repository *git.Repository

	// Debug information
	DebugInfo DebugInfo
}

// DebugInfo contains debugging information about the resolution.
type DebugInfo struct {
	WorkDir       string
	RepoRoot      string
	Remotes       map[string]string
	AllMatches    []matcher.MatchResult
	RejectedRules []RejectedRule
}

// RejectedRule represents a rule that didn't match with the reason.
type RejectedRule struct {
	RuleName string
	Reason   string
}

// Resolver resolves configuration based on context.
type Resolver struct {
	config  *config.Config
	matcher *matcher.Matcher
}

// New creates a new Resolver.
func New(cfg *config.Config) *Resolver {
	return &Resolver{
		config:  cfg,
		matcher: matcher.NewMatcher(),
	}
}

// Resolve determines the configuration for the given directory.
func (r *Resolver) Resolve(workDir string) (*Resolution, error) {
	resolution := &Resolution{
		FinalConfig: make(map[string]string),
		DebugInfo: DebugInfo{
			WorkDir: workDir,
			Remotes: make(map[string]string),
		},
	}

	// Detect repository
	repo, err := git.DetectRepository(workDir)
	if err != nil {
		return nil, err
	}
	resolution.Repository = repo

	if repo != nil {
		resolution.DebugInfo.RepoRoot = repo.Root
		for name, remote := range repo.Remotes {
			resolution.DebugInfo.Remotes[name] = remote.URL
		}
	}

	// Create matching context
	ctx := &matcher.Context{
		Repository: repo,
		WorkDir:    workDir,
		Settings:   r.config.Settings,
	}

	// Match all rules
	allMatches := make([]matcher.MatchResult, 0, len(r.config.Rules))
	for i := range r.config.Rules {
		result := r.matcher.Match(&r.config.Rules[i], ctx)
		allMatches = append(allMatches, result)

		if !result.Matched {
			resolution.DebugInfo.RejectedRules = append(resolution.DebugInfo.RejectedRules, RejectedRule{
				RuleName: r.config.Rules[i].Name,
				Reason:   result.MatchDetails,
			})
		}
	}

	resolution.DebugInfo.AllMatches = allMatches

	// Filter to only matched rules
	var matchedRules []matcher.MatchResult
	for _, result := range allMatches {
		if result.Matched {
			matchedRules = append(matchedRules, result)
		}
	}

	// Sort by precedence
	sortByPrecedence(matchedRules)
	resolution.MatchedRules = matchedRules

	// Apply default config first (lowest priority)
	if r.config.Default != nil {
		for k, v := range r.config.Default.ToGitConfigMap() {
			resolution.FinalConfig[k] = v
		}
		if len(resolution.FinalConfig) > 0 {
			resolution.DefaultApplied = true
		}
	}

	// Apply matched rules in reverse precedence order (lowest first)
	// so higher precedence rules override
	for i := len(matchedRules) - 1; i >= 0; i-- {
		result := matchedRules[i]
		for k, v := range result.Rule.Config.ToGitConfigMap() {
			resolution.FinalConfig[k] = v
		}
	}

	// Set selected rule to highest precedence match
	if len(matchedRules) > 0 {
		resolution.SelectedRule = matchedRules[0].Rule
	}

	return resolution, nil
}

// sortByPrecedence sorts match results by precedence (highest first).
// Precedence order:
// 1. Explicit priority (higher = first)
// 2. Match type (repo > remote > path)
// 3. Match score (higher = first)
// 4. Definition order (earlier = first)
func sortByPrecedence(results []matcher.MatchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]

		// 1. Explicit priority (higher = first)
		if a.Rule.Priority != b.Rule.Priority {
			return a.Rule.Priority > b.Rule.Priority
		}

		// 2. Match type precedence (repo > remote > path)
		if a.MatchType != b.MatchType {
			return a.MatchType > b.MatchType
		}

		// 3. Match score (higher = more specific = first)
		if a.Score != b.Score {
			return a.Score > b.Score
		}

		// 4. Stable sort maintains definition order
		return false
	})
}

// ResolveForExplain is like Resolve but includes more debug information.
func (r *Resolver) ResolveForExplain(workDir string) (*Resolution, error) {
	return r.Resolve(workDir)
}

