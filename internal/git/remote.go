// Package git provides Git repository detection and remote URL parsing.
package git

import (
	"net/url"
	"regexp"
	"strings"
)

// Remote represents a parsed Git remote.
type Remote struct {
	Name string // Remote name (e.g., "origin")
	URL  string // Original URL

	// Normalized components
	Host     string // Normalized host (e.g., "github.com")
	Owner    string // Organization or username
	Repo     string // Repository name (without .git suffix)
	Protocol string // "ssh", "https", or "git"
}

// Common SSH URL patterns:
// git@github.com:owner/repo.git
// ssh://git@github.com/owner/repo.git
// git@github.com:owner/repo (without .git)

// Common HTTPS URL patterns:
// https://github.com/owner/repo.git
// https://github.com/owner/repo
// https://user@github.com/owner/repo.git

var (
	// SSH SCP-style: git@host:owner/repo.git
	sshSCPPattern = regexp.MustCompile(`^(?:([^@]+)@)?([^:]+):(.+?)(?:\.git)?$`)

	// SSH URL-style: ssh://[user@]host/owner/repo.git
	sshURLPattern = regexp.MustCompile(`^ssh://(?:[^@]+@)?([^/]+)/(.+?)(?:\.git)?$`)

	// Git protocol: git://host/owner/repo.git
	gitPattern = regexp.MustCompile(`^git://([^/]+)/(.+?)(?:\.git)?$`)
)

// ParseRemoteURL parses a Git remote URL into its components.
func ParseRemoteURL(rawURL string) (*Remote, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}

	remote := &Remote{URL: rawURL}

	// Try HTTPS/HTTP first
	if strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://") {
		return parseHTTPURL(rawURL, remote)
	}

	// Try SSH URL-style
	if strings.HasPrefix(rawURL, "ssh://") {
		return parseSSHURL(rawURL, remote)
	}

	// Try Git protocol
	if strings.HasPrefix(rawURL, "git://") {
		return parseGitURL(rawURL, remote)
	}

	// Try SSH SCP-style (git@host:path)
	return parseSSHSCP(rawURL, remote)
}

func parseHTTPURL(rawURL string, remote *Remote) (*Remote, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	remote.Protocol = u.Scheme
	remote.Host = normalizeHost(u.Host)

	// Path is /owner/repo or /owner/repo.git
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 1 {
		remote.Owner = parts[0]
	}
	if len(parts) >= 2 {
		remote.Repo = parts[1]
	}

	return remote, nil
}

func parseSSHURL(rawURL string, remote *Remote) (*Remote, error) {
	matches := sshURLPattern.FindStringSubmatch(rawURL)
	if matches == nil {
		// Fall back to generic URL parsing
		return parseHTTPURL(rawURL, remote)
	}

	remote.Protocol = "ssh"
	remote.Host = normalizeHost(matches[1])

	path := matches[2]
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 1 {
		remote.Owner = parts[0]
	}
	if len(parts) >= 2 {
		remote.Repo = parts[1]
	}

	return remote, nil
}

func parseGitURL(rawURL string, remote *Remote) (*Remote, error) {
	matches := gitPattern.FindStringSubmatch(rawURL)
	if matches == nil {
		return remote, nil
	}

	remote.Protocol = "git"
	remote.Host = normalizeHost(matches[1])

	path := matches[2]
	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 1 {
		remote.Owner = parts[0]
	}
	if len(parts) >= 2 {
		remote.Repo = parts[1]
	}

	return remote, nil
}

func parseSSHSCP(rawURL string, remote *Remote) (*Remote, error) {
	matches := sshSCPPattern.FindStringSubmatch(rawURL)
	if matches == nil {
		return remote, nil
	}

	remote.Protocol = "ssh"
	remote.Host = normalizeHost(matches[2])

	path := matches[3]
	path = strings.TrimSuffix(path, ".git")

	parts := strings.SplitN(path, "/", 2)
	if len(parts) >= 1 {
		remote.Owner = parts[0]
	}
	if len(parts) >= 2 {
		remote.Repo = parts[1]
	}

	return remote, nil
}

// normalizeHost removes port numbers and normalizes the host.
func normalizeHost(host string) string {
	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Check if it's actually a port (numeric)
		potentialPort := host[idx+1:]
		isPort := true
		for _, c := range potentialPort {
			if c < '0' || c > '9' {
				isPort = false
				break
			}
		}
		if isPort {
			host = host[:idx]
		}
	}

	return strings.ToLower(host)
}

// CanonicalURL returns a canonical form of the remote URL for comparison.
func (r *Remote) CanonicalURL() string {
	if r.Host == "" {
		return r.URL
	}
	var sb strings.Builder
	sb.WriteString(r.Host)
	if r.Owner != "" {
		sb.WriteString("/")
		sb.WriteString(r.Owner)
		if r.Repo != "" {
			sb.WriteString("/")
			sb.WriteString(r.Repo)
		}
	}
	return sb.String()
}

// MatchesHost checks if the remote matches a host pattern.
func (r *Remote) MatchesHost(pattern string) bool {
	pattern = strings.ToLower(pattern)
	host := strings.ToLower(r.Host)

	// Exact match
	if host == pattern {
		return true
	}

	// Wildcard subdomain match (e.g., "*.github.com")
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // Keep the dot
		return strings.HasSuffix(host, suffix)
	}

	return false
}

// MatchesOrg checks if the remote belongs to a specific organization/owner.
func (r *Remote) MatchesOrg(org string) bool {
	return strings.EqualFold(r.Owner, org)
}

