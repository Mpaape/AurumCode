package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// rangeToken is credential-shaped only at run time.
var rangeToken = "ghp" + "_" + strings.Repeat("Z9y8X7w6", 4) + "v5U4"

// gitleaksCommand emulates git and the pinned gitleaks binary for the real
// gitleaks engine: the history of the range it is given holds one leak
// (with the secret in Secret and Match, as gitleaks reports it), and it
// records the --log-opts it received.
func gitleaksCommand(logOpts *string) scanner.Command {
	return func(_ context.Context, _ string, binary string, args ...string) (string, string, error) {
		switch {
		case binary == "git" && len(args) > 0 && args[0] == "rev-parse":
			return "false\n", "", nil
		case binary == "git":
			return "", "", nil
		case len(args) == 1 && args[0] == "version":
			return "v8.30.1\n", "", nil
		}
		for _, a := range args {
			if strings.HasPrefix(a, "--log-opts=") {
				*logOpts = strings.TrimPrefix(a, "--log-opts=")
			}
		}
		leak, _ := json.Marshal([]map[string]any{{
			"RuleID": "github-pat", "Description": "Uncovered a GitHub Personal Access Token",
			"File": "app.go", "StartLine": 4, "Commit": strings.Repeat("ab", 20),
			"Secret": rangeToken, "Match": "token := " + rangeToken,
		}})
		for i, a := range args {
			if a == "--report-path" && i+1 < len(args) {
				return "", "", os.WriteFile(args[i+1], leak, 0o600)
			}
		}
		return "", "", errors.New("no report path")
	}
}

// --base resolves the base ref and HEAD to full commit ids and hands them
// to the engine; the gitleaks finding reaches the gate line and the audit
// with origin gitleaks, and no output carries the secret.
func TestBaseReviewHandsTheResolvedRangeToGitleaks(t *testing.T) {
	aur579Repo(t)
	repo, err := analyzer.OpenRepo(".")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := repo.ResolveRef("HEAD~1")
	head, _ := repo.ResolveRef("HEAD")
	writeRepoConfig(t, "quality_gates:\n  scanners:\n    - engine: gitleaks\n")
	audit := filepath.Join(t.TempDir(), "audit.json")
	var logOpts string
	var out, errOut strings.Builder
	rio := reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter(), deps: reviewDeps{env: &reviewEnv{}, scanners: scanner.Executor{Command: gitleaksCommand(&logOpts)}}}
	code := runReviewWith(rio, []string{"--base", "HEAD~1", "--auditoria", audit})
	if logOpts != base+".."+head || len(base) != 40 {
		t.Fatalf("range %q, want %s..%s", logOpts, base, head)
	}
	if code != exitFindings || !strings.Contains(errOut.String(), "policy gate: gitleaks:github-pat - Uncovered a GitHub Personal Access Token in commit abababababab") || !strings.Contains(errOut.String(), "origem gitleaks, secao repo)") {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	raw, err := os.ReadFile(audit)
	if err != nil || !strings.Contains(string(raw), `"origin": "gitleaks"`) && !strings.Contains(string(raw), `"origin":"gitleaks"`) {
		t.Fatalf("audit lacks origin gitleaks: %v", err)
	}
	if strings.Contains(out.String()+errOut.String()+string(raw), rangeToken[4:]) {
		t.Fatal("an output carries the secret")
	}
}

// An engine's reported identity changes the evidence digest: a verdict
// computed with another binary version or rule base is never reused.
func TestEngineIdentityEntersTheEvidenceDigest(t *testing.T) {
	s := &reviewState{evidence: []prompt.EvidenceItem{{ID: "E1", Origin: "gitleaks", RuleID: "gitleaks:github-pat", File: "app.go", Line: 4}}}
	digest := func() string {
		d, err := cache.DigestOf(s.evidenceIdentity())
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	plain := digest()
	if legacy, _ := cache.DigestOf(s.evidence); plain != legacy {
		t.Fatal("without an engine identity the digest must stay the items' own")
	}
	s.scanVersions = []string{"gitleaks=gitleaks v8.30.1 rulebase a"}
	first := digest()
	s.scanVersions = []string{"gitleaks=gitleaks v8.30.1 rulebase b"}
	second := digest()
	if plain == first || first == second {
		t.Fatalf("identity not in the key: %s %s %s", plain, first, second)
	}
}
