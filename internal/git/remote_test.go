package git

import (
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantHost string
		wantOwner string
		wantRepo  string
		wantProto string
	}{
		{
			name:      "HTTPS GitHub",
			url:       "https://github.com/owner/repo.git",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantProto: "https",
		},
		{
			name:      "HTTPS GitHub without .git",
			url:       "https://github.com/owner/repo",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantProto: "https",
		},
		{
			name:      "SSH SCP-style GitHub",
			url:       "git@github.com:owner/repo.git",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantProto: "ssh",
		},
		{
			name:      "SSH SCP-style without .git",
			url:       "git@github.com:owner/repo",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantProto: "ssh",
		},
		{
			name:      "SSH URL-style",
			url:       "ssh://git@github.com/owner/repo.git",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantProto: "ssh",
		},
		{
			name:      "GitLab HTTPS",
			url:       "https://gitlab.com/group/subgroup/repo.git",
			wantHost:  "gitlab.com",
			wantOwner: "group",
			wantRepo:  "subgroup/repo",
			wantProto: "https",
		},
		{
			name:      "Bitbucket SSH",
			url:       "git@bitbucket.org:team/repo.git",
			wantHost:  "bitbucket.org",
			wantOwner: "team",
			wantRepo:  "repo",
			wantProto: "ssh",
		},
		{
			name:      "Custom self-hosted",
			url:       "git@git.company.com:org/project.git",
			wantHost:  "git.company.com",
			wantOwner: "org",
			wantRepo:  "project",
			wantProto: "ssh",
		},
		{
			name:      "HTTP with port",
			url:       "http://git.local:8080/owner/repo.git",
			wantHost:  "git.local",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantProto: "http",
		},
		{
			name:      "Git protocol",
			url:       "git://github.com/owner/repo.git",
			wantHost:  "github.com",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantProto: "git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, err := ParseRemoteURL(tt.url)
			if err != nil {
				t.Fatalf("ParseRemoteURL() error = %v", err)
			}
			if remote == nil {
				t.Fatal("ParseRemoteURL() returned nil")
			}

			if remote.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", remote.Host, tt.wantHost)
			}
			if remote.Owner != tt.wantOwner {
				t.Errorf("Owner = %q, want %q", remote.Owner, tt.wantOwner)
			}
			if remote.Repo != tt.wantRepo {
				t.Errorf("Repo = %q, want %q", remote.Repo, tt.wantRepo)
			}
			if remote.Protocol != tt.wantProto {
				t.Errorf("Protocol = %q, want %q", remote.Protocol, tt.wantProto)
			}
		})
	}
}

func TestRemoteMatchesHost(t *testing.T) {
	tests := []struct {
		host    string
		pattern string
		want    bool
	}{
		{"github.com", "github.com", true},
		{"github.com", "GitHub.com", true},
		{"gitlab.com", "github.com", false},
		{"git.company.com", "*.company.com", true},
		{"company.com", "*.company.com", false},
		{"sub.sub.company.com", "*.company.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.host+"_"+tt.pattern, func(t *testing.T) {
			remote := &Remote{Host: tt.host}
			if got := remote.MatchesHost(tt.pattern); got != tt.want {
				t.Errorf("MatchesHost(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestRemoteMatchesOrg(t *testing.T) {
	tests := []struct {
		owner   string
		org     string
		want    bool
	}{
		{"myorg", "myorg", true},
		{"MyOrg", "myorg", true},
		{"myorg", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.owner+"_"+tt.org, func(t *testing.T) {
			remote := &Remote{Owner: tt.owner}
			if got := remote.MatchesOrg(tt.org); got != tt.want {
				t.Errorf("MatchesOrg(%q) = %v, want %v", tt.org, got, tt.want)
			}
		})
	}
}

