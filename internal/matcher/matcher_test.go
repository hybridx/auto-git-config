package matcher

import (
	"testing"

	"github.com/denair/auto-git-config/internal/config"
	"github.com/denair/auto-git-config/internal/git"
)

func TestPathPrefixStrategy(t *testing.T) {
	strategy := &PathPrefixStrategy{}

	tests := []struct {
		name      string
		prefix    string
		workDir   string
		repoRoot  string
		wantMatch bool
	}{
		{
			name:      "exact match",
			prefix:    "/home/user/work",
			workDir:   "/home/user/work",
			wantMatch: true,
		},
		{
			name:      "subdirectory match",
			prefix:    "/home/user/work",
			workDir:   "/home/user/work/project",
			wantMatch: true,
		},
		{
			name:      "no match",
			prefix:    "/home/user/work",
			workDir:   "/home/user/personal",
			wantMatch: false,
		},
		{
			name:      "uses repo root if available",
			prefix:    "/home/user/work",
			workDir:   "/home/user/personal",
			repoRoot:  "/home/user/work/project",
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &config.Match{PathPrefix: tt.prefix}
			ctx := &Context{WorkDir: tt.workDir}
			if tt.repoRoot != "" {
				ctx.Repository = &git.Repository{Root: tt.repoRoot}
			}

			matched, _, _ := strategy.Matches(match, ctx)
			if matched != tt.wantMatch {
				t.Errorf("Matches() = %v, want %v", matched, tt.wantMatch)
			}
		})
	}
}

func TestRemoteHostStrategy(t *testing.T) {
	strategy := &RemoteHostStrategy{}

	tests := []struct {
		name       string
		hostMatch  string
		remoteHost string
		wantMatch  bool
	}{
		{
			name:       "exact match",
			hostMatch:  "github.com",
			remoteHost: "github.com",
			wantMatch:  true,
		},
		{
			name:       "case insensitive",
			hostMatch:  "GitHub.com",
			remoteHost: "github.com",
			wantMatch:  true,
		},
		{
			name:       "no match",
			hostMatch:  "github.com",
			remoteHost: "gitlab.com",
			wantMatch:  false,
		},
		{
			name:       "wildcard match",
			hostMatch:  "*.company.com",
			remoteHost: "git.company.com",
			wantMatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &config.Match{RemoteHost: tt.hostMatch}
			ctx := &Context{
				Repository: &git.Repository{
					Remotes: map[string]*git.Remote{
						"origin": {Host: tt.remoteHost},
					},
				},
				Settings: config.Settings{DefaultRemote: "origin"},
			}

			matched, _, _ := strategy.Matches(match, ctx)
			if matched != tt.wantMatch {
				t.Errorf("Matches() = %v, want %v", matched, tt.wantMatch)
			}
		})
	}
}

func TestFolderNameStrategy(t *testing.T) {
	strategy := &FolderNameStrategy{}

	tests := []struct {
		name       string
		folderName string
		workDir    string
		wantMatch  bool
	}{
		{
			name:       "exact match",
			folderName: "project",
			workDir:    "/home/user/work/project",
			wantMatch:  true,
		},
		{
			name:       "case insensitive",
			folderName: "Project",
			workDir:    "/home/user/work/project",
			wantMatch:  true,
		},
		{
			name:       "no match",
			folderName: "other",
			workDir:    "/home/user/work/project",
			wantMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &config.Match{FolderName: tt.folderName}
			ctx := &Context{WorkDir: tt.workDir}

			matched, _, _ := strategy.Matches(match, ctx)
			if matched != tt.wantMatch {
				t.Errorf("Matches() = %v, want %v", matched, tt.wantMatch)
			}
		})
	}
}

func TestMatcherANDLogic(t *testing.T) {
	m := NewMatcher()

	// Rule with multiple conditions (should AND them)
	rule := &config.Rule{
		Name: "test",
		Match: config.Match{
			RemoteHost: "github.com",
			PathPrefix: "/home/user/work",
		},
		Config: config.GitConfig{
			User: config.UserConfig{
				Email: "test@example.com",
			},
		},
	}

	ctx := &Context{
		WorkDir: "/home/user/work/project",
		Repository: &git.Repository{
			Root: "/home/user/work/project",
			Remotes: map[string]*git.Remote{
				"origin": {Host: "github.com"},
			},
		},
		Settings: config.Settings{DefaultRemote: "origin"},
	}

	result := m.Match(rule, ctx)
	if !result.Matched {
		t.Errorf("Expected rule to match when both conditions are met")
	}

	// Change remote to not match
	ctx.Repository.Remotes["origin"].Host = "gitlab.com"
	result = m.Match(rule, ctx)
	if result.Matched {
		t.Errorf("Expected rule to NOT match when one condition fails")
	}
}

func TestMatcherDisabledRule(t *testing.T) {
	m := NewMatcher()

	enabled := false
	rule := &config.Rule{
		Name:    "disabled-rule",
		Enabled: &enabled,
		Match: config.Match{
			RemoteHost: "github.com",
		},
		Config: config.GitConfig{
			User: config.UserConfig{
				Email: "test@example.com",
			},
		},
	}

	ctx := &Context{
		Repository: &git.Repository{
			Remotes: map[string]*git.Remote{
				"origin": {Host: "github.com"},
			},
		},
		Settings: config.Settings{DefaultRemote: "origin"},
	}

	result := m.Match(rule, ctx)
	if result.Matched {
		t.Errorf("Disabled rule should not match")
	}
}

