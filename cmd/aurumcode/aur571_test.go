package main

// AUR-571: the analysis_data API address comes from AURUMCODE_GITHUB_API_URL,
// the same variable and validation the PR client uses. Fake GitHub in loopback.

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/artifacts"
	"github.com/Mpaape/AurumCode/internal/config"
)

// AC-001: with the variable pointing at the loopback fake, the review (whose
// gate keeps the default address) resolves the artifact from there.
func TestAUR571ReviewResolvesArtifactFromConfiguredAddress(t *testing.T) {
	f := newAUR533Fake(t, "")
	dir := cleanFixture(t, aur533Config("7", "block"))
	useAUR533Env(t, artifacts.DefaultAPIBase, aur533Gen.Add(24*time.Hour))
	t.Setenv(config.GitHubAPIURLEnv, f.srv.URL)
	audit := filepath.Join(dir, "audit.json")
	code, out, errOut := runAUR533(t, "--auditoria", audit)
	if code != 0 || f.calls.Load() == 0 {
		t.Fatalf("exit=%d calls=%d\n%s%s", code, f.calls.Load(), out, errOut)
	}
	if strings.Contains(out+errOut, "inconclusiva") {
		t.Fatalf("unexpected inconclusive:\n%s%s", out, errOut)
	}
}

// AC-001 (default): without the variable the resolver targets the default.
func TestAUR571UnsetVariableUsesDefaultAddress(t *testing.T) {
	t.Setenv(config.GitHubAPIURLEnv, "")
	got, err := config.GitHubAPIURL(func(k string) string { return "" })
	if err != nil || got != artifacts.DefaultAPIBase {
		t.Fatalf("got %q %v", got, err)
	}
}

// AC-002: a non-loopback http:// address is refused before any request, as a
// blocking inconclusive result that names the variable.
func TestAUR571InsecureAddressIsRefused(t *testing.T) {
	f := newAUR533Fake(t, "")
	cleanFixture(t, aur533Config("7", "block"))
	useAUR533Env(t, artifacts.DefaultAPIBase, aur533Gen.Add(24*time.Hour))
	t.Setenv(config.GitHubAPIURLEnv, "http://ghe.example.com")
	code, out, errOut := runAUR533(t)
	all := out + errOut
	if code == 0 || !strings.Contains(all, config.GitHubAPIURLEnv) || !strings.Contains(all, artifacts.ReasonInvalid) {
		t.Fatalf("exit=%d, want refusal naming the variable:\n%s", code, all)
	}
	if f.calls.Load() != 0 {
		t.Fatalf("no request may be made, got %d", f.calls.Load())
	}
}

// AC-002: the PR client uses the same validation.
func TestAUR571PRClientRefusesInsecureAddress(t *testing.T) {
	t.Setenv(config.GitHubAPIURLEnv, "http://ghe.example.com")
	if _, err := newGitHubClient(); err == nil || !strings.Contains(err.Error(), config.GitHubAPIURLEnv) {
		t.Fatalf("want error naming the variable, got %v", err)
	}
	t.Setenv(config.GitHubAPIURLEnv, "http://127.0.0.1:1")
	if _, err := newGitHubClient(); err != nil {
		t.Fatal(err)
	}
}
