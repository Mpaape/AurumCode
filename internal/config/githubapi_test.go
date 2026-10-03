package config

import (
	"strings"
	"testing"
)

func envOf(v string) func(string) string {
	return func(k string) string {
		if k == GitHubAPIURLEnv {
			return v
		}
		return ""
	}
}

// AUR-571 AC-001: unset (or blank) selects the public default.
func TestAUR571GitHubAPIURLDefault(t *testing.T) {
	for _, v := range []string{"", "   "} {
		got, err := GitHubAPIURL(envOf(v))
		if err != nil || got != DefaultGitHubAPIURL {
			t.Fatalf("env %q: got %q, %v", v, got, err)
		}
	}
	if DefaultGitHubAPIURL != "https://api.github.com" {
		t.Fatalf("default changed: %s", DefaultGitHubAPIURL)
	}
}

// AUR-571 AC-001/AC-002: accepted and refused addresses, error names the variable.
func TestAUR571GitHubAPIURLValidation(t *testing.T) {
	ok := map[string]string{
		"https://ghe.example.com/api/v3/": "https://ghe.example.com/api/v3",
		"https://api.github.com":          "https://api.github.com",
		"http://127.0.0.1:8080":           "http://127.0.0.1:8080",
		"http://[::1]:9":                  "http://[::1]:9",
	}
	for in, want := range ok {
		got, err := GitHubAPIURL(envOf(in))
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"http://ghe.example.com", "http://localhost:8080", "http://10.0.0.5", "ftp://127.0.0.1",
		"api.github.com", "https://", "https://u:p@ghe.example.com", "https://ghe.example.com/?x=1",
	} {
		_, err := GitHubAPIURL(envOf(in))
		if err == nil || !strings.Contains(err.Error(), GitHubAPIURLEnv) {
			t.Errorf("%q must be refused naming %s, got %v", in, GitHubAPIURLEnv, err)
		}
	}
}
