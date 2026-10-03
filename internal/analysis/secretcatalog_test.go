package analysis

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Test vectors are assembled from pieces so the repository never carries a
// literal token-shaped string. None of them is a real credential.
var (
	awsKeyVector      = "AKIA" + "Z7Q2MXR4K5TL3WJH"
	awsExampleVector  = "AKIA" + "IOSFODNN7" + "EXAMPLE"
	awsLowEntropyKey  = "AKIA" + strings.Repeat("A", 16)
	githubTokenVector = "ghp_" + "8kQ2zR7vXm4LpT9wN3sB6yH1cJ5dF0gA2eK4"
	passwordVector    = "9fK2xQ7vLp3Zr8Wm"
)

// scannersLockEnv names the scanners lock the acceptance program hands to
// TestSecretCatalogMatchesScannersLock; package tests never read outside
// the module on their own.
const scannersLockEnv = "AURUMCODE_SCANNERS_LOCK"

func TestSecretVendorFormatsReportRuleIDWithoutValue(t *testing.T) {
	diff := singleHunk("app.go", 1, 1,
		`+awsID := "`+awsKeyVector+`"`,
		`+gh := "`+githubTokenVector+`"`)
	f := NewRunner().Analyze(diff)
	if len(f) != 2 {
		t.Fatalf("want 2 findings, got %#v", f)
	}
	for i, wantRule := range []string{"aws-access-token", "github-pat"} {
		if f[i].RuleID != RuleHardcodedSecret || f[i].Line != i+1 || f[i].Side != SideRight {
			t.Fatalf("finding %d = %#v", i, f[i])
		}
		if !strings.Contains(f[i].Message, wantRule) {
			t.Fatalf("finding %d message %q lacks base rule id %s", i, f[i].Message, wantRule)
		}
		for _, v := range []string{awsKeyVector, githubTokenVector} {
			if strings.Contains(f[i].Message, v) {
				t.Fatalf("finding %d message leaks the captured value", i)
			}
		}
	}
}

func TestSecretAllowlistAndEntropy(t *testing.T) {
	r := NewRunner()
	cases := []struct {
		name, path, line string
		want             int
	}{
		{"password with high entropy is a base finding", "config.yml", "password: " + passwordVector, 1},
		{"same password with base stopword is allowlisted", "config.yml", "password: " + passwordVector + "-example", 0},
		{"aws documentation key is allowlisted", "app.go", `id := "` + awsExampleVector + `"`, 0},
		{"low-entropy aws-shaped key is below the minimum", "app.go", `id := "` + awsLowEntropyKey + `"`, 0},
		{"low-entropy password is below the minimum", "config.yml", "password: 1q1q1q1q1q1q1q1q", 0},
		{"global path allowlist excludes lock files", "go.sum", `id := "` + awsKeyVector + `"`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := r.Analyze(singleHunk(tc.path, 1, 1, "+"+tc.line))
			if len(f) != tc.want {
				t.Fatalf("got %#v, want %d findings", f, tc.want)
			}
		})
	}
}

func TestShannonEntropy(t *testing.T) {
	if got := shannonEntropy("aaaa"); got != 0 {
		t.Fatalf("entropy(aaaa) = %v", got)
	}
	if got := shannonEntropy("abcd"); got != 2 {
		t.Fatalf("entropy(abcd) = %v", got)
	}
}

func TestEmbeddedCatalogMatchesRecordedDigest(t *testing.T) {
	want := strings.TrimSpace(string(embeddedSecretCatalogDigest))
	if got := sha256Of(embeddedSecretCatalog); got != want {
		t.Fatalf("embedded secrets.json %s, recorded %s", got, want)
	}
}

func TestSecretCatalogRejectsDivergentDigest(t *testing.T) {
	derived := bytes.Clone(embeddedSecretCatalog)
	derived[len(derived)/2] ^= 0x01
	if _, err := loadSecretCatalog(catalogArtifacts{derived, embeddedSecretCatalogDigest, embeddedLocalSecretCatalog}); err == nil {
		t.Fatal("a derived catalog one byte off its recorded digest was accepted")
	}
}

// TestSecretCatalogMatchesScannersLock binds the embedded catalog to the
// scanners lock: the source digest it records must equal
// secrets_rulebase_sha256. It needs the lock path from the acceptance
// program and is skipped without it.
func TestSecretCatalogMatchesScannersLock(t *testing.T) {
	lockPath := os.Getenv(scannersLockEnv)
	if lockPath == "" {
		t.Skip(scannersLockEnv + " not set")
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^secrets_rulebase_sha256: (\S+)$`).FindSubmatch(data)
	if m == nil {
		t.Fatal("lock has no secrets_rulebase_sha256")
	}
	if string(m[1]) != embeddedSecrets.source.SHA256 {
		t.Fatalf("lock %s, catalog records %s", m[1], embeddedSecrets.source.SHA256)
	}
}

// gcpExampleKey is one of the documentation keys the public base allowlists
// for gcp-api-key; the catalog holds it only as a sha256.
var (
	gcpExampleKey = "AIza" + "SyAnLA7NfeLquW1t" + "JFpx_eQCxoX-oo6YyIs"
	gcpKeyVector  = "AIza" + "Sy8kQ2zR7vXm4LpT9wN3sB6yH1cJ5dF0gA2"
)

// TestHashedAllowlistSuppressesExampleKey proves the hashed allowlist, not
// entropy or another rule, is what suppresses the documentation key: the
// same rule without its allowlists flags it, and a different key of the
// same shape is still flagged with them.
func TestHashedAllowlistSuppressesExampleKey(t *testing.T) {
	r := NewRunner()
	if f := r.Analyze(singleHunk("app.go", 1, 1, `+k := "`+gcpExampleKey+`"`)); len(f) != 0 {
		t.Fatalf("allowlisted example key flagged: %#v", f)
	}
	if f := r.Analyze(singleHunk("app.go", 1, 1, `+k := "`+gcpKeyVector+`"`)); len(f) != 1 || !strings.Contains(f[0].Message, "gcp-api-key") {
		t.Fatalf("gcp key not flagged: %#v", f)
	}
	var gcp *secretRule
	for _, rule := range embeddedSecrets.rules {
		if rule.id == "gcp-api-key" {
			gcp = rule
		}
	}
	if gcp == nil || len(gcp.allowlists) == 0 || len(gcp.allowlists[0].literals) == 0 {
		t.Fatal("gcp-api-key has no hashed allowlist")
	}
	bare := *gcp
	bare.allowlists = nil
	line := `k := "` + gcpExampleKey + `"`
	if !bare.matches(embeddedSecrets.global, "app.go", line, strings.ToLower(line), false) {
		t.Fatal("without its allowlist the rule does not flag the example key: the hash proves nothing")
	}
}

// gitleaksSourceEnv names a local copy of the verified gitleaks TOML (kept by
// scripts/analysis/update-secret-rules.sh with AURUMCODE_GITLEAKS_TOML_OUT).
// The TOML is not committed, so the source-fidelity test only runs with it.
const gitleaksSourceEnv = "AURUMCODE_GITLEAKS_TOML"

// TestDerivedSecretCatalogFaithfulToSource checks, without a TOML parser,
// that the derived JSON carries every rule of the source TOML with the same
// id, regex and entropy in the same order, and that every hashed allowlist
// literal is a literal of the source that the rule's own regex matches.
func TestDerivedSecretCatalogFaithfulToSource(t *testing.T) {
	path := os.Getenv(gitleaksSourceEnv)
	if path == "" {
		t.Skip(gitleaksSourceEnv + " not set: the gitleaks TOML is not committed; run scripts/analysis/update-secret-rules.sh with AURUMCODE_GITLEAKS_TOML_OUT to check fidelity")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256Of(source); got != embeddedSecrets.source.SHA256 {
		t.Fatalf("source TOML %s, catalog records %s", got, embeddedSecrets.source.SHA256)
	}
	var derived secretCatalogFile
	if err := json.Unmarshal(embeddedSecretCatalog, &derived); err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(string(source), "[[rules]]")[1:]
	if len(blocks) != len(derived.Rules) {
		t.Fatalf("toml has %d rules, json %d", len(blocks), len(derived.Rules))
	}
	idRe := regexp.MustCompile(`(?m)^id = "([^"]+)"`)
	regexRe := regexp.MustCompile(`(?ms)^regex = '''(.*?)'''`)
	entropyRe := regexp.MustCompile(`(?m)^entropy = ([0-9.]+)`)
	for i, block := range blocks {
		parts := strings.SplitN(block, "[[rules.allowlists]]", 2)
		head, rule := parts[0], derived.Rules[i]
		if m := idRe.FindStringSubmatch(head); m == nil || m[1] != rule.ID {
			t.Fatalf("rule %d: toml id %v, json %q", i, m, rule.ID)
		}
		if m := regexRe.FindStringSubmatch(head); m != nil && m[1] != rule.Regex {
			t.Fatalf("rule %s: regex differs between toml and json", rule.ID)
		}
		if m := entropyRe.FindStringSubmatch(head); m != nil {
			want, err := strconv.ParseFloat(m[1], 64)
			if err != nil || want != rule.Entropy {
				t.Fatalf("rule %s: toml entropy %s, json %v", rule.ID, m[1], rule.Entropy)
			}
		}
		checkHashedLiterals(t, rule, parts)
	}
}

// sourceLiteralRe matches a TOML literal or basic string on one line.
var sourceLiteralRe = regexp.MustCompile(`'''(.*?)'''|"([^"\n]*)"`)

// checkHashedLiterals asserts each literal_sha256 of rule is the digest of a
// literal in the rule's source allowlists that the rule's regex matches.
func checkHashedLiterals(t *testing.T, rule secretRuleSpec, parts []string) {
	t.Helper()
	known := map[string]string{}
	if len(parts) == 2 {
		for _, m := range sourceLiteralRe.FindAllStringSubmatch(parts[1], -1) {
			lit := m[1] + m[2]
			known[strings.TrimPrefix(sha256Of([]byte(lit)), sha256Prefix)] = lit
		}
	}
	re := regexp.MustCompile(rule.Regex)
	for _, a := range rule.Allowlists {
		for _, h := range a.LiteralSHA {
			lit, ok := known[h]
			if !ok {
				t.Fatalf("rule %s: hashed literal %s is not a source allowlist literal", rule.ID, h)
			}
			if !re.MatchString(lit) {
				t.Fatalf("rule %s: hashed literal %s does not match the rule regex", rule.ID, h)
			}
		}
	}
}

// TestSkippedSecretRulesAreDeclared pins the public-base rules the evaluator
// does not apply, so a new skip (for instance a regex Go's RE2 refuses) is a
// visible test change instead of a silent loss of coverage.
func TestSkippedSecretRulesAreDeclared(t *testing.T) {
	want := []skippedRule{{ID: "pkcs12-file", Reason: skipPathOnly}}
	got := embeddedSecrets.skipped
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("skipped rules = %#v, want %#v", got, want)
	}
}

// TestNoSecretRegexCompiledInGo walks the production Go files of the package
// and fails when a regexp.MustCompile/Compile call has a string literal that
// looks like a secret rule: secret patterns must come from the data catalog.
func TestNoSecretRegexCompiledInGo(t *testing.T) {
	secretish := regexp.MustCompile(`(?i)passw|secret|token|api[_-]?key|credential|akia|ghp_`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "MustCompile" && sel.Sel.Name != "Compile") {
				return true
			}
			for _, arg := range call.Args {
				if lit, ok := arg.(*ast.BasicLit); ok && secretish.MatchString(lit.Value) {
					t.Errorf("%s: secret-shaped regex compiled in Go", fset.Position(lit.Pos()))
				}
			}
			return true
		})
	}
}
