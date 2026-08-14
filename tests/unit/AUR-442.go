package unit

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR442 proves the AUR-442 outcome at the package boundary: the
// dogfooding measurement found `--seguranca` matched only 2 of the 8
// catalog rules (security/sql-injection, security/command-injection), both
// requiring the narrow `"..." + var` concatenation shape, so the project's
// own demo fixture -- tests/fixtures/repos/git-demo, whose sole purpose is
// to plant a plaintext secret -- reported "No security findings." AUR-442
// restores the matcher for security/hardcoded-secret (the card's named
// priority, and the rule git-demo's fixture depends on) and leaves the
// remaining five patternless rules (xss, path-traversal, weak-crypto,
// insecure-random, missing-auth) honestly declared as metadata-only rather
// than shipping unvalidated matchers for them. See
// internal/review/rules/security.yml and docs/specs/AUR-442.md.
//
// Selector naming note, same technique tests/unit/AUR-445.go documents: a
// function named TestAUR435 already exists in this package
// (tests/unit/AUR-435.go) and would collide, so this proof is TestAUR442
// even though the card text's TDD proof section names TestAUR435 (a
// copy-paste artifact from the AUR-435 card this one was drafted against).
func TestAUR442(t *testing.T) {
	t.Run("HardcodedSecretNowCarriesAMatcher", testAUR442HardcodedSecretNowCarriesAMatcher)
	t.Run("DeclaredOnlyRulesStayPatternless", testAUR442DeclaredOnlyRulesStayPatternless)
	t.Run("OtherSecurityRulesUnaffected", testAUR442OtherSecurityRulesUnaffected)
	t.Run("ScanFindsTheQuotedCodeShape", testAUR442ScanFindsTheQuotedCodeShape)
	t.Run("ScanFindsTheUnquotedEnvShape", testAUR442ScanFindsTheUnquotedEnvShape)
	t.Run("ScanFindsTheBareUnquotedEnvShape", testAUR442ScanFindsTheBareUnquotedEnvShape)
	t.Run("ScanIgnoresRemovedAndContextLines", testAUR442ScanIgnoresRemovedAndContextLines)
	t.Run("ScanRejectsBenignAssignments", testAUR442ScanRejectsBenignAssignments)
	t.Run("ScanNeverEchoesTheMatchedValue", testAUR442ScanNeverEchoesTheMatchedValue)
	t.Run("FindingsCiteOnlySecurityCategoryRules", testAUR442FindingsCiteOnlySecurityCategoryRules)
	t.Run("ScanIsDeterministic", testAUR442ScanIsDeterministic)
	t.Run("GitDemoSecretIsNowFound", testAUR442GitDemoSecretIsNowFound)
}

// aur442Diff builds a one-file diff whose hunk lines are given verbatim
// (markers included), mirroring tests/unit/AUR-435.go's aur435Diff and what
// analyzer.BuildDiffFile produces.
func aur442Diff(path string, newStart int, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: newStart,
			Lines:    lines,
		}},
	}}}
}

func testAUR442HardcodedSecretNowCarriesAMatcher(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() of the embedded catalog must succeed, got: %v", err)
	}
	re, ok := loader.PatternFor("security/hardcoded-secret")
	if !ok || re == nil {
		t.Fatal("security/hardcoded-secret must now carry a compiled matcher pattern (AUR-442's priority restoration)")
	}
	rule, ok := loader.Get("security/hardcoded-secret")
	if !ok {
		t.Fatal("security/hardcoded-secret must resolve in the catalog")
	}
	if rule.Standard != "SCR-003" {
		t.Fatalf("security/hardcoded-secret must bind the project security standard rule SCR-003 (Cryptographic Failures / hardcoded credential), got %q", rule.Standard)
	}
}

// testAUR442DeclaredOnlyRulesStayPatternless proves the other half of the
// card's honesty rule: a rule this card did NOT give a matcher must still
// resolve (citable by a model finding, AUR-434) but never carry a pattern
// -- so it can never be silently claimed as active by a future change
// without also adding a fixture that proves it.
func testAUR442DeclaredOnlyRulesStayPatternless(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	declaredOnly := []string{
		"security/xss",
		"security/path-traversal",
		"security/weak-crypto",
		"security/insecure-random",
		"security/missing-auth",
	}
	for _, id := range declaredOnly {
		rule, ok := loader.Get(id)
		if !ok {
			t.Fatalf("%s must still resolve in the catalog", id)
		}
		if rule.Category != "security" {
			t.Fatalf("%s must stay category security, got %q", id, rule.Category)
		}
		if _, ok := loader.PatternFor(id); ok {
			t.Fatalf("%s must stay metadata-only (no fixture proves a matcher for it): AUR-442 declares it honestly instead of promising undelivered detection", id)
		}
	}
}

// testAUR442OtherSecurityRulesUnaffected proves this card touched exactly
// one rule's matcher: sql-injection and command-injection (AUR-435) keep
// matching exactly what they matched before.
func testAUR442OtherSecurityRulesUnaffected(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	sqlRe, ok := loader.PatternFor("security/sql-injection")
	if !ok {
		t.Fatal("security/sql-injection must still carry a matcher")
	}
	if !sqlRe.MatchString(`    query = "SELECT id, name FROM users WHERE name = '" + name + "'"`) {
		t.Fatal("security/sql-injection must still match its known planted shape")
	}
	cmdRe, ok := loader.PatternFor("security/command-injection")
	if !ok {
		t.Fatal("security/command-injection must still carry a matcher")
	}
	if !cmdRe.MatchString(`    os.system("rm -rf " + target)`) {
		t.Fatal("security/command-injection must still match its known planted shape")
	}
}

func testAUR442ScanFindsTheQuotedCodeShape(t *testing.T) {
	diff := aur442Diff("src/config.py", 1,
		`+"""Config module."""`,
		"+",
		"+timeout_seconds = 30",
		`+api_key = "AURUM-FAKE-KEY-9000-2222"`,
	)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding (the benign timeout_seconds line must not match), got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.File != "src/config.py" || f.Line != 4 {
		t.Fatalf("expected the finding at src/config.py:4, got %s:%d", f.File, f.Line)
	}
	if f.RuleID != "security/hardcoded-secret" {
		t.Fatalf("expected rule security/hardcoded-secret, got %q", f.RuleID)
	}
	if f.Severity != "error" {
		t.Fatalf("expected severity error, got %q", f.Severity)
	}
	if !strings.Contains(f.Message, "(rule security/hardcoded-secret: Hardcoded Secrets)") {
		t.Fatalf("the finding must cite its sustaining rule (AUR-434 gate), got %q", f.Message)
	}
	if !strings.Contains(f.Message, "standards/security-review SCR-003") {
		t.Fatalf("the finding must cite the project security standard, got %q", f.Message)
	}
}

func testAUR442ScanFindsTheUnquotedEnvShape(t *testing.T) {
	// The exact shape tests/fixtures/repos/git-demo/content/03/config/demo-tokens.txt
	// plants: an unquoted KEY=VALUE env line.
	diff := aur442Diff("config/demo-tokens.txt", 1,
		"+# Synthetic tokens.",
		"+DEMO_API_TOKEN=AURUM-FAKE-TOKEN-0000-0001",
	)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding (the comment line must not match), got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.File != "config/demo-tokens.txt" || f.Line != 2 {
		t.Fatalf("expected the finding at config/demo-tokens.txt:2, got %s:%d", f.File, f.Line)
	}
	if f.RuleID != "security/hardcoded-secret" {
		t.Fatalf("expected rule security/hardcoded-secret, got %q", f.RuleID)
	}
}

// testAUR442ScanFindsTheBareUnquotedEnvShape is a review fix (independent
// reviewer, 2026-08-13): the unquoted branch originally required a
// mandatory leading `[A-Z]` before the keyword alternation, which consumed
// the keyword's own first letter whenever the identifier had no prefix --
// so bare `TOKEN=`, `SECRET=`, `API_KEY=`, `PASSWORD=` (the keyword is the
// entire identifier) silently never matched, despite the rule comment and
// docs/specs/AUR-442.md promising the unquoted shape without that
// exclusion. The leading class is now `[A-Z0-9_]*` (zero-or-more), so the
// keyword itself can start the match; `\b` still requires a real
// identifier boundary, so a keyword glued onto a lowercase prefix
// (`xTOKEN=...`) stays out of scope.
func testAUR442ScanFindsTheBareUnquotedEnvShape(t *testing.T) {
	bare := []string{
		"TOKEN=AURUM-FAKE-TOKEN-0000-0001",
		"SECRET=AURUM-FAKE-SECRET-9000-1111",
		"API_KEY=AURUM-FAKE-KEY-9000-2222",
		"PASSWORD=AURUM-FAKE-PASSWORD-9000-3333",
	}
	for _, line := range bare {
		diff := aur442Diff("config/bare.env", 1, "+"+line)
		findings, err := review.SecurityScan(diff)
		if err != nil {
			t.Fatalf("SecurityScan(%q): %v", line, err)
		}
		if len(findings) != 1 {
			t.Fatalf("bare unquoted keyword must be found: %q, got %d findings: %+v", line, len(findings), findings)
		}
		if findings[0].RuleID != "security/hardcoded-secret" {
			t.Fatalf("expected rule security/hardcoded-secret for %q, got %q", line, findings[0].RuleID)
		}
	}

	// The adversarial case the review fix must keep excluded: a keyword
	// glued onto a lowercase prefix has no `\b` identifier boundary before
	// the keyword's uppercase run, so it is not an env-style variable.
	diff := aur442Diff("config/bare.env", 1, "+xTOKEN=abc123xyz")
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("xTOKEN=... must not match (no identifier boundary before the keyword), got %+v", findings)
	}
}

func testAUR442ScanIgnoresRemovedAndContextLines(t *testing.T) {
	const secretLine = `DB_PASSWORD=AURUM-FAKE-PASSWORD-9000-1111`
	diff := aur442Diff("config/secrets.env", 1,
		"-"+secretLine,
		" "+secretLine,
		"+SERVICE_NAME=aurum-fixture",
	)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("removed/context lines must not produce findings, got %+v", findings)
	}
}

// testAUR442ScanRejectsBenignAssignments is the explicit false-positive
// proof the card demands: a credential-named identifier assigned a
// computed value (a function call) or a plain numeric constant must never
// be reported, only a value that actually looks like a hardcoded secret
// (a letter mixed with a digit or hyphen).
func testAUR442ScanRejectsBenignAssignments(t *testing.T) {
	benign := []string{
		"+token := getAuthToken()",
		"+API_TOKEN = get_token_from_vault()",
		"+TOKEN_MAX_AGE_SECONDS = 31536000",
		"+PASSWORD_MIN_LENGTH = 12",
		`+token := "placeholder"`,
		`+secretary := "JaneDoeWorksHere"`,
		`+password_hint := "your favorite pet name"`,
		"+greeting: hello",
		"+  name: demo",
		"+SERVICE_NAME=aurum-fixture",
	}
	for _, line := range benign {
		diff := aur442Diff("src/benign.txt", 1, line)
		findings, err := review.SecurityScan(diff)
		if err != nil {
			t.Fatalf("SecurityScan(%q): %v", line, err)
		}
		if len(findings) != 0 {
			t.Fatalf("benign line must not produce a hardcoded-secret finding: %q, got %+v", line, findings)
		}
	}
}

// testAUR442ScanNeverEchoesTheMatchedValue is the card's redaction
// requirement: the finding must cite file:line and the rule, with the
// value redacted. securitypass.go builds every message from
// rule.Description (trusted catalog text), never from the matched diff
// body, so the secret value structurally cannot reach the finding; this
// pins that invariant against regression.
func testAUR442ScanNeverEchoesTheMatchedValue(t *testing.T) {
	const secretValue = "AURUM-FAKE-CANARY-VALUE-7777"
	diff := aur442Diff("config/secrets.env", 1, "+API_TOKEN="+secretValue)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding, got %+v", findings)
	}
	if strings.Contains(findings[0].Message, secretValue) {
		t.Fatalf("the finding message must never echo the matched secret value, got %q", findings[0].Message)
	}
}

func testAUR442FindingsCiteOnlySecurityCategoryRules(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	diff := aur442Diff("config/secrets.env", 1, "+API_TOKEN=AURUM-FAKE-TOKEN-1234567")
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding for the planted line")
	}
	for _, f := range findings {
		rule, ok := loader.Get(f.RuleID)
		if !ok {
			t.Fatalf("finding cites unknown rule %q", f.RuleID)
		}
		if rule.Category != "security" {
			t.Fatalf("the security pass cited a %s-category rule (%s): the rubric separation is broken", rule.Category, rule.ID)
		}
	}
}

func testAUR442ScanIsDeterministic(t *testing.T) {
	diff := aur442Diff("config/secrets.env", 1,
		"+API_TOKEN=AURUM-FAKE-TOKEN-1111111",
		"+DB_PASSWORD=AURUM-FAKE-PASSWORD-2222222",
	)
	first, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	second, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("expected two findings both runs, got first=%+v second=%+v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("SecurityScan is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
		}
	}
	if first[0].Line >= first[1].Line {
		t.Fatalf("expected findings in ascending line order, got %+v", first)
	}
}

// testAUR442GitDemoSecretIsNowFound is the card's core Outcome, proved at
// the package boundary without touching the filesystem: the exact three
// lines tests/fixtures/repos/git-demo/content/03/config/demo-tokens.txt
// plants (read-only input to this card; not re-read from disk here so
// this test has no dependency on that fixture's path) now each produce a
// security/hardcoded-secret finding, where before AUR-442 none did.
func testAUR442GitDemoSecretIsNowFound(t *testing.T) {
	diff := aur442Diff("config/demo-tokens.txt", 4,
		"+DEMO_API_TOKEN=AURUM-FAKE-TOKEN-0000-0001",
		"+DEMO_DB_PASSWORD=AURUM-FAKE-PASSWORD-0000-0002",
		"+DEMO_WEBHOOK_SECRET=AURUM-FAKE-WEBHOOK-0000-0003",
	)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 3 {
		t.Fatalf("expected all three planted git-demo secrets to be found, got %d: %+v", len(findings), findings)
	}
	for i, f := range findings {
		if f.RuleID != "security/hardcoded-secret" {
			t.Fatalf("finding %d: expected rule security/hardcoded-secret, got %q", i, f.RuleID)
		}
		if f.Line != 4+i {
			t.Fatalf("finding %d: expected line %d, got %d", i, 4+i, f.Line)
		}
	}
}
