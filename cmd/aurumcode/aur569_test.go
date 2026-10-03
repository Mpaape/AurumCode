package main

// AUR-569 behavior proofs: a finding of the --seguranca pass (deterministic
// evidence) at or above gate.fail_on fails the gate in every
// gate.inconclusive mode, with no provider configured. Driven through the real
// `aurumcode review --base` command.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur569Fixture builds a git repo whose HEAD~1..HEAD diff adds a hardcoded
// credential (security/hardcoded-secret, error) and configures no provider.
// secret=false adds a harmless line instead.
func aur569Fixture(t *testing.T, secret bool) {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(source, parent string) string {
		blob := gitObject(t, dir, "blob", []byte(source))
		tree := gitObject(t, dir, "tree", treeEntry(t, "100644", "app.go", blob))
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nfixture\n"
		return gitObject(t, dir, "commit", []byte(body))
	}
	line := "\tgreeting := \"ola\"\n"
	if secret {
		line = "\tdbPassword := \"hunter2\"\n"
	}
	base := commit("package demo\n\nfunc Run() {\n}\n", "")
	headSrc := "package demo\n\nfunc Run() {\n" + line + "\t_ = " + strings.Fields(line)[0] + "\n}\n"
	head := commit(headSrc, base)
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", []byte(headSrc))
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	t.Cleanup(chdir(t, dir))
}

func aur569Run(t *testing.T, mode string, extra ...string) (int, string) {
	t.Helper()
	policy := policyFixture(t, "gate:\n  fail_on: [high]\n  inconclusive: "+mode+"\n")
	var o, e strings.Builder
	args := append([]string{"--base", "HEAD~1", "--politica", policy}, extra...)
	code := runReview(args, &o, &e, redaction.NewFilter())
	return code, e.String()
}

// AC-002: the security finding fails the gate, warn or block, and the gate line
// names the rule and the origin.
func TestAUR569SecurityFindingFailsGateInEveryInconclusiveMode(t *testing.T) {
	for _, mode := range []string{"warn", "block"} {
		t.Run(mode, func(t *testing.T) {
			aur569Fixture(t, true)
			code, errOut := aur569Run(t, mode, "--seguranca")
			if code != exitFindings {
				t.Fatalf("exit=%d, want exitFindings(%d); stderr=%s", code, exitFindings, errOut)
			}
			if !strings.Contains(errOut, "security/hardcoded-secret") || !strings.Contains(errOut, "origem security") {
				t.Errorf("gate line must name the rule and the origin:\n%s", errOut)
			}
		})
	}
}

// AC-003: without a deterministic finding, warn only warns (exit 0) and block
// still fails on the missing opinion alone (exit 1).
func TestAUR569WithoutFindingWarnStillOnlyWarns(t *testing.T) {
	aur569Fixture(t, false)
	if code, errOut := aur569Run(t, "warn", "--seguranca"); code != 0 {
		t.Fatalf("warn without a finding: exit=%d, want 0; stderr=%s", code, errOut)
	}
	if code, errOut := aur569Run(t, "block", "--seguranca"); code != exitQualityNotReviewed {
		t.Fatalf("block without a finding: exit=%d, want %d; stderr=%s", code, exitQualityNotReviewed, errOut)
	}
}

// The finding counts only when the pass ran: no --seguranca, no security finding.
func TestAUR569NoSecurityPassNoSecurityFinding(t *testing.T) {
	aur569Fixture(t, true)
	if code, errOut := aur569Run(t, "warn"); code != 0 {
		t.Fatalf("warn without --seguranca: exit=%d, want 0; stderr=%s", code, errOut)
	}
}
