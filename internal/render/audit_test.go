package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

func writePolicyFixture(t *testing.T, dir, configYAML, skillBody string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".aurumcode", "skills"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".aurumcode", "config.yml"), []byte(configYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".aurumcode", "skills", "security.md"), []byte(skillBody), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestAUR521PolicyDigestChangesWhenPolicyChanges covers AC-001: the digest
// must change when the policy's config or a skill file it references
// changes, and must be empty when no policy is active at all.
func TestAUR521PolicyDigestChangesWhenPolicyChanges(t *testing.T) {
	if got := PolicyDigest("/does/not/matter", nil); got != "" {
		t.Fatalf("digest with no policy must be empty, got %q", got)
	}

	dirA := t.TempDir()
	configYAML := "review:\n  context:\n    skills:\n      - .aurumcode/skills/security.md\n"
	writePolicyFixture(t, dirA, configYAML, "## No Hardcoded Secrets\n\nNever commit a literal credential.\n")
	cfgA, err := config.Parse([]byte(configYAML), filepath.Join(dirA, ".aurumcode", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	digestA := PolicyDigest(dirA, cfgA)
	if digestA == "" {
		t.Fatal("digest with an active policy must not be empty")
	}

	// Same config, different skill content -> digest must change.
	dirB := t.TempDir()
	writePolicyFixture(t, dirB, configYAML, "## No Hardcoded Secrets\n\nNever commit a literal credential, ever.\n")
	cfgB, err := config.Parse([]byte(configYAML), filepath.Join(dirB, ".aurumcode", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	digestB := PolicyDigest(dirB, cfgB)
	if digestB == digestA {
		t.Fatal("digest must change when the policy's skill content changes")
	}

	// Different rules in config.yml itself -> digest must change too.
	dirC := t.TempDir()
	configYAMLC := "rules:\n  no-hardcoded-secrets:\n    severity: error\nreview:\n  context:\n    skills:\n      - .aurumcode/skills/security.md\n"
	writePolicyFixture(t, dirC, configYAMLC, "## No Hardcoded Secrets\n\nNever commit a literal credential.\n")
	cfgC, err := config.Parse([]byte(configYAMLC), filepath.Join(dirC, ".aurumcode", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	digestC := PolicyDigest(dirC, cfgC)
	if digestC == digestA {
		t.Fatal("digest must change when the policy's own config.yml content changes")
	}

	// Re-running over the same fixture reproduces the same digest.
	if again := PolicyDigest(dirA, cfgA); again != digestA {
		t.Fatal("digest must be deterministic across repeated computation of the same policy")
	}
}

// TestAUR521AuditRecordInconclusiveMarksOmittedFiles covers AC-004: an
// inconclusive run's audit record names the gate's own inconclusive
// decision and lists the files left out of coverage.
func TestAUR521AuditRecordInconclusiveMarksOmittedFiles(t *testing.T) {
	rec := BuildAuditRecord(
		"digest123", "workflowsha", "octo/repo", "deadbeef", "gpt-test", "comment",
		AuditGate{Decision: "inconclusive", Reason: "review inconclusive (partial_coverage)"},
		nil, nil,
		false, []string{"b.go", "a.go"},
	)
	if rec.Gate.Decision != "inconclusive" {
		t.Fatalf("decision=%q, want inconclusive", rec.Gate.Decision)
	}
	if rec.Coverage.Complete {
		t.Fatal("coverage must not claim complete on an inconclusive/partial run")
	}
	if got := rec.Coverage.Omitted; len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Fatalf("omitted files=%v, want sorted [a.go b.go]", got)
	}
	if rec.ExceptionsApplied == nil {
		t.Fatal("exceptions_applied must be an empty list, never nil/omitted")
	}
	if rec.BlockingFindings == nil {
		t.Fatal("blocking_findings must be an empty list, never nil/omitted")
	}
}

// TestAUR521WriteAuditRecordRedactsSecretCanary covers AC-005: a secret
// canary present in a finding's own path/severity text must not survive
// into the written audit file.
func TestAUR521WriteAuditRecordRedactsSecretCanary(t *testing.T) {
	const canary = "AURUM-CANARY-f00dfeed"
	t.Setenv(redaction.CanaryEnv, canary)
	filter := redaction.FromEnv()

	rec := BuildAuditRecord(
		"digest", "wfsha", "octo/repo", "sha123", "gpt-test", "comment",
		AuditGate{Decision: "fail", Reason: "token leaked: " + canary},
		[]AuditFinding{{RuleID: "r1", Path: "app.go", Line: 1, Severity: "error"}},
		nil, true, nil,
	)

	path := filepath.Join(t.TempDir(), "audit.json")
	if err := WriteAuditRecord(path, rec, filter); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), canary) {
		t.Fatalf("canary leaked into audit record: %s", data)
	}
	if !strings.Contains(string(data), redaction.Marker) {
		t.Fatalf("redaction marker missing from audit record: %s", data)
	}
	// The file must still be valid JSON after redaction rewrote the reason.
	var roundtrip map[string]any
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("audit record is not valid JSON after redaction: %v\n%s", err, data)
	}
}

// TestAUR521WriteAuditRecordRedactsEscapedSecrets is B3: a secret
// containing a `"`, a `\` or an embedded newline survives
// json.MarshalIndent as a DIFFERENT byte sequence (the special character
// escaped) than the one a post-marshal-only redaction pass would still
// match against the raw secret value -- so redacting only the final JSON
// text misses it. WriteAuditRecord must redact each field BEFORE
// marshaling (and still run the post-marshal pass as defense in depth).
// Each secret below carries a stable prefix that is byte-identical whether
// or not JSON escaped the special character that follows it, so the
// leak check below is correct regardless of escaping.
func TestAUR521WriteAuditRecordRedactsEscapedSecrets(t *testing.T) {
	quoteSecret := `AURUMQUOTE-abc"xyz`
	backslashSecret := "AURUMBACKSLASH-abc\\xyz"
	newlineSecret := "AURUMNEWLINE-abc\nxyz"
	filter := redaction.NewFilter(quoteSecret, backslashSecret, newlineSecret)

	rec := BuildAuditRecord(
		"digest", "wfsha", "octo/repo", "sha123", "gpt-test", "comment",
		AuditGate{Decision: "fail", Reason: "a: " + quoteSecret},
		[]AuditFinding{{RuleID: "r1", Path: "app.go", Line: 1, Severity: "error"}},
		[]AuditException{{RuleID: "r2", Path: "app.go", Justification: "b: " + backslashSecret}},
		true, []string{"c: " + newlineSecret},
	)

	path := filepath.Join(t.TempDir(), "audit.json")
	if err := WriteAuditRecord(path, rec, filter); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"AURUMQUOTE-abc", "AURUMBACKSLASH-abc", "AURUMNEWLINE-abc"} {
		if strings.Contains(string(data), prefix) {
			t.Fatalf("secret with prefix %q leaked into the audit record (escaped or not):\n%s", prefix, data)
		}
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("audit record is not valid JSON after redaction: %v\n%s", err, data)
	}
}
