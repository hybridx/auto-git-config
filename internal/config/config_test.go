package config

import (
	"testing"
)

func TestParse(t *testing.T) {
	toml := `
version = "1"

[settings]
cache_enabled = true
cache_ttl_seconds = 300
default_remote = "origin"

[default.user]
name = "Default User"
email = "default@example.com"

[[rule]]
name = "github"
priority = 0

[rule.match]
remote_host = "github.com"

[rule.config.user]
name = "GitHub User"
email = "github@example.com"

[[rule]]
name = "work"
priority = 10

[rule.match]
path_prefix = "~/work"

[rule.config.user]
name = "Work User"
email = "work@company.com"

[rule.config.commit]
gpgsign = true
`

	cfg, err := Parse(toml)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.Version != "1" {
		t.Errorf("Version = %q, want %q", cfg.Version, "1")
	}

	if cfg.Settings.CacheEnabled != true {
		t.Errorf("CacheEnabled = %v, want true", cfg.Settings.CacheEnabled)
	}

	if cfg.Settings.CacheTTLSeconds != 300 {
		t.Errorf("CacheTTLSeconds = %d, want 300", cfg.Settings.CacheTTLSeconds)
	}

	if cfg.Default == nil {
		t.Fatal("Default is nil")
	}
	if cfg.Default.User.Name != "Default User" {
		t.Errorf("Default.User.Name = %q, want %q", cfg.Default.User.Name, "Default User")
	}

	if len(cfg.Rules) != 2 {
		t.Fatalf("len(Rules) = %d, want 2", len(cfg.Rules))
	}

	// Check github rule
	githubRule := cfg.Rules[0]
	if githubRule.Name != "github" {
		t.Errorf("Rules[0].Name = %q, want %q", githubRule.Name, "github")
	}
	if githubRule.Match.RemoteHost != "github.com" {
		t.Errorf("Rules[0].Match.RemoteHost = %q, want %q", githubRule.Match.RemoteHost, "github.com")
	}
	if githubRule.Config.User.Email != "github@example.com" {
		t.Errorf("Rules[0].Config.User.Email = %q, want %q", githubRule.Config.User.Email, "github@example.com")
	}

	// Check work rule
	workRule := cfg.Rules[1]
	if workRule.Name != "work" {
		t.Errorf("Rules[1].Name = %q, want %q", workRule.Name, "work")
	}
	if workRule.Priority != 10 {
		t.Errorf("Rules[1].Priority = %d, want 10", workRule.Priority)
	}
	if workRule.Match.PathPrefix != "~/work" {
		t.Errorf("Rules[1].Match.PathPrefix = %q, want %q", workRule.Match.PathPrefix, "~/work")
	}
	if workRule.Config.Commit.GPGSign == nil || *workRule.Config.Commit.GPGSign != true {
		t.Error("Rules[1].Config.Commit.GPGSign should be true")
	}
}

func TestGitConfigToMap(t *testing.T) {
	gpgSign := true
	gc := GitConfig{
		User: UserConfig{
			Name:       "Test User",
			Email:      "test@example.com",
			SigningKey: "ABC123",
		},
		Commit: CommitConfig{
			GPGSign: &gpgSign,
		},
		Extra: map[string]string{
			"custom.key": "custom-value",
		},
	}

	m := gc.ToGitConfigMap()

	expected := map[string]string{
		"user.name":       "Test User",
		"user.email":      "test@example.com",
		"user.signingkey": "ABC123",
		"commit.gpgsign":  "true",
		"custom.key":      "custom-value",
	}

	for k, v := range expected {
		if m[k] != v {
			t.Errorf("ToGitConfigMap()[%q] = %q, want %q", k, m[k], v)
		}
	}
}

func TestMatchType(t *testing.T) {
	tests := []struct {
		match Match
		want  MatchType
	}{
		{Match{RepoPath: "/path"}, MatchTypeRepo},
		{Match{RemoteHost: "github.com"}, MatchTypeRemote},
		{Match{RemoteURL: ".*github.*"}, MatchTypeRemote},
		{Match{RemoteOrg: "myorg"}, MatchTypeRemote},
		{Match{PathPrefix: "~/work"}, MatchTypePath},
		{Match{FolderName: "project"}, MatchTypePath},
		{Match{}, MatchTypeNone},
	}

	for _, tt := range tests {
		if got := tt.match.MatchType(); got != tt.want {
			t.Errorf("MatchType() = %v, want %v for %+v", got, tt.want, tt.match)
		}
	}
}

func TestParseValidation(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr bool
	}{
		{
			name: "rule without name",
			toml: `
[[rule]]
[rule.match]
remote_host = "github.com"
[rule.config.user]
name = "Test"
`,
			wantErr: true,
		},
		{
			name: "rule with empty config",
			toml: `
[[rule]]
name = "test"
[rule.match]
remote_host = "github.com"
`,
			wantErr: true,
		},
		{
			name: "valid minimal rule",
			toml: `
[[rule]]
name = "test"
[rule.match]
remote_host = "github.com"
[rule.config.user]
email = "test@example.com"
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.toml)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

