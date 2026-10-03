package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	// GitHubAPIURLEnv names the one variable that selects the GitHub REST API
	// root for every outbound GitHub client (pull-request client and the
	// analysis_data artifact resolver).
	GitHubAPIURLEnv = "AURUMCODE_GITHUB_API_URL"
	// DefaultGitHubAPIURL is the only place the public API host is declared
	// for the artifact resolver and the shared validation below.
	DefaultGitHubAPIURL = "https://api.github.com"
)

// ValidateGitHubAPIURL accepts only an absolute https:// root, or http://
// for a literal loopback IP (a local test server, never a name that DNS could
// redirect). The returned value has no trailing slash. The error names the
// variable so the operator knows what to fix.
func ValidateGitHubAPIURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("%s: %q is not an absolute URL", GitHubAPIURLEnv, raw)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%s: %q must not carry credentials, a query or a fragment", GitHubAPIURLEnv, raw)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if ip := net.ParseIP(u.Hostname()); ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("%s: %q uses http:// for a host that is not a loopback IP literal; use https://", GitHubAPIURLEnv, raw)
		}
	default:
		return "", fmt.Errorf("%s: %q must use https:// (http:// only for a loopback IP)", GitHubAPIURLEnv, raw)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// GitHubAPIURL resolves the API root from the environment (getenv is
// os.Getenv in production): unset or blank gives the default, anything else
// must pass ValidateGitHubAPIURL.
func GitHubAPIURL(getenv func(string) string) (string, error) {
	v := strings.TrimSpace(getenv(GitHubAPIURLEnv))
	if v == "" {
		return DefaultGitHubAPIURL, nil
	}
	return ValidateGitHubAPIURL(v)
}
