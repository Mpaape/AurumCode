// TestAUR481 proves the AUR-481 outcome at the package boundary for Rust,
// this card's declared priority: the 2026-08-26 measurement planted the
// same four defects (secret, sql, cmd, xss) in eight languages and found
// Rust at zero for all three deterministic rules --
//
//   - security/hardcoded-secret: `const ACCESS_KEY: &str = "...";` never
//     matched, because Rust puts a TYPE between the identifier and `=`
//     and the pattern expected a quote right after `:=`/`:`/`=`.
//   - security/sql-injection: `"...".to_owned() + name` never matched,
//     because Rust's idiomatic concatenation puts a method call between
//     the closing quote and the `+`, and `format!("... {}", name)` --
//     Rust's more common shape -- has no `+` at all.
//
// This file proves the fix (internal/review/rules/security.yml): the
// typed-const and String::from secret shapes, the .to_owned()/.to_string()
// and format! SQL shapes are all found (AC-001); the safe forms named by
// this card -- a numeric constant, a digit-free string constant, and a
// $1-parametrized query -- are not (AC-002); and the pre-existing
// Python/Node/Go regression shapes this catalog already matched keep
// matching, unchanged (AC-003).
//
// Rust command-injection (`Command::new("sh").arg("-c")...`) is a
// declared, documented gap, not an oversight: extending
// security/command-injection's pattern breaks tests/acceptance/AUR-462.sh's
// own MUT-001, which hardcodes that pattern's bytes as a literal mutation
// anchor in a file this card does not own. See docs/specs/AUR-481.md.
package unit

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestAUR481(t *testing.T) {
	t.Run("AC001TypedConstSecretIsFound", testAUR481AC001TypedConstSecretIsFound)
	t.Run("AC001StaticSecretIsFound", testAUR481AC001StaticSecretIsFound)
	t.Run("AC001StringFromSecretIsFound", testAUR481AC001StringFromSecretIsFound)
	t.Run("AC001ToOwnedSQLConcatIsFound", testAUR481AC001ToOwnedSQLConcatIsFound)
	t.Run("AC001ToStringSQLConcatIsFound", testAUR481AC001ToStringSQLConcatIsFound)
	t.Run("AC001FormatMacroSQLIsFound", testAUR481AC001FormatMacroSQLIsFound)
	t.Run("AC002NumericConstIsNotFound", testAUR481AC002NumericConstIsNotFound)
	t.Run("AC002DigitFreeConstIsNotFound", testAUR481AC002DigitFreeConstIsNotFound)
	t.Run("AC002ParametrizedQueryIsNotFound", testAUR481AC002ParametrizedQueryIsNotFound)
	t.Run("AC002FormatMacroWithoutSQLIsNotFound", testAUR481AC002FormatMacroWithoutSQLIsNotFound)
	t.Run("AC002ArgvCommandIsNotFound", testAUR481AC002ArgvCommandIsNotFound)
	t.Run("AC003PythonSQLInjectionUnaffected", testAUR481AC003PythonSQLInjectionUnaffected)
	t.Run("AC003NodeCommandInjectionUnaffected", testAUR481AC003NodeCommandInjectionUnaffected)
	t.Run("AC003GoStyleHardcodedSecretUnaffected", testAUR481AC003GoStyleHardcodedSecretUnaffected)
	t.Run("FixtureEndToEnd", testAUR481FixtureEndToEnd)
	t.Run("ScanIsDeterministic", testAUR481ScanIsDeterministic)
}

// aur481Diff builds a one-file diff whose hunk lines are given verbatim
// (markers included), mirroring tests/unit/AUR-466.go's aur466Diff.
func aur481Diff(path string, newStart int, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: newStart,
			Lines:    lines,
		}},
	}}}
}

func aur481Scan(t *testing.T, diff *types.Diff) []types.ReviewIssue {
	t.Helper()
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	return findings
}

// --- AC-001: the Rust shapes the 2026-08-26 measurement found missed ---

func testAUR481AC001TypedConstSecretIsFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, `+const ACCESS_KEY: &str = "AURUM-FAKE-RUSTKEY-9000-4444";`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/hardcoded-secret" {
		t.Fatalf("expected one security/hardcoded-secret finding for a typed const secret, got %+v", f)
	}
}

func testAUR481AC001StaticSecretIsFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, `+static API_TOKEN: &str = "AURUM-FAKE-RUSTKEY-9000-6666";`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/hardcoded-secret" {
		t.Fatalf("expected one security/hardcoded-secret finding for a typed static secret, got %+v", f)
	}
}

func testAUR481AC001StringFromSecretIsFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, `+    let secret_token = String::from("AURUM-FAKE-RUSTKEY-9000-5555");`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/hardcoded-secret" {
		t.Fatalf("expected one security/hardcoded-secret finding for a String::from-wrapped secret, got %+v", f)
	}
}

func testAUR481AC001ToOwnedSQLConcatIsFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1,
		`+    conn.query(&("SELECT * FROM t WHERE n = '".to_owned() + name));`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/sql-injection" {
		t.Fatalf("expected one security/sql-injection finding for .to_owned() concatenation, got %+v", f)
	}
}

func testAUR481AC001ToStringSQLConcatIsFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1,
		`+    conn.query(&("SELECT * FROM t WHERE n = '".to_string() + name));`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/sql-injection" {
		t.Fatalf("expected one security/sql-injection finding for .to_string() concatenation, got %+v", f)
	}
}

func testAUR481AC001FormatMacroSQLIsFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1,
		`+    let q = format!("SELECT * FROM t WHERE n = '{}'", name);`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/sql-injection" {
		t.Fatalf("expected one security/sql-injection finding for a format! SQL query, got %+v", f)
	}
}

// --- AC-002: the safe forms this card names must NOT produce a finding --

func testAUR481AC002NumericConstIsNotFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, `+const MAX_RETRIES: u32 = 5;`)
	if f := aur481Scan(t, diff); len(f) != 0 {
		t.Fatalf("a numeric constant must not match: got %+v", f)
	}
}

func testAUR481AC002DigitFreeConstIsNotFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, `+const GREETING: &str = "hello world";`)
	if f := aur481Scan(t, diff); len(f) != 0 {
		t.Fatalf("a digit-free string constant must not match: got %+v", f)
	}
}

func testAUR481AC002ParametrizedQueryIsNotFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1,
		`+    conn.query("SELECT * FROM t WHERE n = $1", &[&name]);`)
	if f := aur481Scan(t, diff); len(f) != 0 {
		t.Fatalf("a parametrized query must not match: got %+v", f)
	}
}

func testAUR481AC002FormatMacroWithoutSQLIsNotFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, `+    let s = format!("user {}", name);`)
	if f := aur481Scan(t, diff); len(f) != 0 {
		t.Fatalf("a format! call with no SQL verb must not match: got %+v", f)
	}
}

// testAUR481AC002ArgvCommandIsNotFound proves the argv-form Command this
// card's AC-002 names is unaffected by the AUR-481 secret/sql patches
// (command-injection's pattern itself is unmodified by this card, see the
// package doc comment).
func testAUR481AC002ArgvCommandIsNotFound(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, `+    Command::new("ping").arg(host).spawn().unwrap();`)
	if f := aur481Scan(t, diff); len(f) != 0 {
		t.Fatalf("an argv-form Command with no shell must not match: got %+v", f)
	}
}

// --- AC-003: pre-existing regression shapes stay unchanged --------------

func testAUR481AC003PythonSQLInjectionUnaffected(t *testing.T) {
	diff := aur481Diff("src/db.py", 1,
		`+    query = "SELECT id, name FROM users WHERE name = '" + name + "'"`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/sql-injection" {
		t.Fatalf("expected the pre-existing Python sql-injection shape to keep matching, got %+v", f)
	}
}

func testAUR481AC003NodeCommandInjectionUnaffected(t *testing.T) {
	diff := aur481Diff("src/app.js", 1, `+  exec("ping -c 1 " + host);`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/command-injection" {
		t.Fatalf("expected the pre-existing Node command-injection shape to keep matching, got %+v", f)
	}
}

func testAUR481AC003GoStyleHardcodedSecretUnaffected(t *testing.T) {
	diff := aur481Diff("config/secrets.env", 1, `+API_TOKEN=AURUM-FAKE-CANARY-VALUE-7777`)
	f := aur481Scan(t, diff)
	if len(f) != 1 || f[0].RuleID != "security/hardcoded-secret" {
		t.Fatalf("expected the pre-existing unquoted UPPER_SNAKE secret shape to keep matching, got %+v", f)
	}
}

// aur481FixtureLines is the exact content of
// tests/fixtures/review/vuln/rust-secret-sql-injection's second commit,
// src/main.rs -- see that directory's history.spec. Embedding it here as
// a synthetic diff keeps this package-boundary proof independent of
// reading the committed bare repository, while still agreeing
// line-for-line with tests/integration/AUR-481.go and
// tests/e2e/AUR-481.sh, which both read the real fixture.
var aur481FixtureLines = []string{
	`+const ACCESS_KEY: &str = "AURUM-FAKE-RUSTKEY-9000-4444";`,
	`+const MAX_RETRIES: u32 = 5;`,
	`+const GREETING: &str = "hello world";`,
	`+`,
	`+fn get_secret() -> String {`,
	`+    let secret_token = String::from("AURUM-FAKE-RUSTKEY-9000-5555");`,
	`+    secret_token`,
	`+}`,
	`+`,
	`+fn query_user(conn: &mut Connection, name: &str) {`,
	`+    conn.query(&("SELECT * FROM t WHERE n = '".to_owned() + name));`,
	`+}`,
	`+`,
	`+fn query_user_fmt(conn: &mut Connection, name: &str) {`,
	`+    let q = format!("SELECT * FROM t WHERE n = '{}'", name);`,
	`+    conn.query(&q);`,
	`+}`,
	`+`,
	`+fn query_user_safe(conn: &mut Connection, name: &str) {`,
	`+    conn.query("SELECT * FROM t WHERE n = $1", &[&name]);`,
	`+}`,
}

func testAUR481FixtureEndToEnd(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, aur481FixtureLines...)
	f := aur481Scan(t, diff)
	if len(f) != 4 {
		t.Fatalf("expected exactly 4 findings (lines 1, 6, 11, 15), got %d: %+v", len(f), f)
	}
	byLine := map[int]string{}
	for _, issue := range f {
		byLine[issue.Line] = issue.RuleID
	}
	want := map[int]string{
		1:  "security/hardcoded-secret",
		6:  "security/hardcoded-secret",
		11: "security/sql-injection",
		15: "security/sql-injection",
	}
	for line, ruleID := range want {
		if got, ok := byLine[line]; !ok || got != ruleID {
			t.Fatalf("expected %s at line %d, got %+v", ruleID, line, f)
		}
	}
	for line := range byLine {
		if _, ok := want[line]; !ok {
			t.Fatalf("unexpected finding at line %d (false positive), full set: %+v", line, f)
		}
	}
}

func testAUR481ScanIsDeterministic(t *testing.T) {
	diff := aur481Diff("src/main.rs", 1, aur481FixtureLines...)
	first := aur481Scan(t, diff)
	second := aur481Scan(t, diff)
	if len(first) != len(second) {
		t.Fatalf("non-deterministic finding count: %d vs %d", len(first), len(second))
	}
	var firstIDs, secondIDs []string
	for _, i := range first {
		firstIDs = append(firstIDs, i.RuleID)
	}
	for _, i := range second {
		secondIDs = append(secondIDs, i.RuleID)
	}
	if strings.Join(firstIDs, ",") != strings.Join(secondIDs, ",") {
		t.Fatalf("non-deterministic rule order: %v vs %v", firstIDs, secondIDs)
	}
}
