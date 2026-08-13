package unit

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR435 proves the security pass at the package boundary: the embedded
// catalog's security rules now carry the matcher the c12d7ab restoration
// measured as missing ("as regras sao metadados sem padrao"), the scan finds
// a planted synthetic vulnerability on the exact added line, it never
// matches removed or context lines, and its findings cite only rules of
// category security -- the separation from the quality rubric that MUT-001
// attacks.
func TestAUR435(t *testing.T) {
	t.Run("SecurityRulesCarryTheRestoredMatcher", testAUR435SecurityRulesCarryTheRestoredMatcher)
	t.Run("QualityRulesStayMetadataOnly", testAUR435QualityRulesStayMetadataOnly)
	t.Run("ScanFindsThePlantedVulnerability", testAUR435ScanFindsThePlantedVulnerability)
	t.Run("ScanIgnoresRemovedAndContextLines", testAUR435ScanIgnoresRemovedAndContextLines)
	t.Run("ScanTracksHunkLineNumbers", testAUR435ScanTracksHunkLineNumbers)
	t.Run("FindingsCiteOnlySecurityCategoryRules", testAUR435FindingsCiteOnlySecurityCategoryRules)
	t.Run("ScanIsDeterministic", testAUR435ScanIsDeterministic)
	t.Run("CleanDiffYieldsNoFindings", testAUR435CleanDiffYieldsNoFindings)
}

// aur435SQLInjectionLine is the same synthetic shape the committed fixture
// plants in tests/fixtures/review/vuln/content/02/src/db.py: a SQL query
// assembled by string concatenation. Purely synthetic; accepted by nothing.
const aur435SQLInjectionLine = `    query = "SELECT id, name FROM users WHERE name = '" + name + "'"`

// aur435Diff builds a one-file diff whose hunk lines are given verbatim
// (markers included), mirroring what analyzer.BuildDiffFile produces.
func aur435Diff(path string, newStart int, lines ...string) *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: newStart,
			Lines:    lines,
		}},
	}}}
}

func testAUR435SecurityRulesCarryTheRestoredMatcher(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load() of the embedded catalog must succeed, got: %v", err)
	}
	re, ok := loader.PatternFor("security/sql-injection")
	if !ok || re == nil {
		t.Fatal("security/sql-injection must carry a compiled matcher pattern")
	}
	if !re.MatchString(aur435SQLInjectionLine) {
		t.Fatalf("the sql-injection matcher must match the planted line %q", aur435SQLInjectionLine)
	}
	if re.MatchString(`    return db.execute(query)`) {
		t.Fatal("the sql-injection matcher must not match an ordinary line")
	}
	rule, ok := loader.Get("security/sql-injection")
	if !ok {
		t.Fatal("security/sql-injection must resolve in the catalog")
	}
	if rule.Standard != "SCR-001" {
		t.Fatalf("security/sql-injection must bind the project security standard rule SCR-001, got %q", rule.Standard)
	}
}

func testAUR435QualityRulesStayMetadataOnly(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	quality := loader.GetByCategory("quality")
	if len(quality) == 0 {
		t.Fatal("expected quality rules in the embedded catalog")
	}
	for _, rule := range quality {
		if _, ok := loader.PatternFor(rule.ID); ok {
			t.Fatalf("quality rule %s must not carry a security matcher pattern", rule.ID)
		}
	}
}

func testAUR435ScanFindsThePlantedVulnerability(t *testing.T) {
	diff := aur435Diff("src/db.py", 1,
		"+\"\"\"Data access helpers.\"\"\"",
		"+",
		"+",
		"+def find_user(db, name):",
		"+"+aur435SQLInjectionLine,
		"+    return db.execute(query)",
	)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one security finding, got %d: %+v", len(findings), findings)
	}
	f := findings[0]
	if f.File != "src/db.py" || f.Line != 5 {
		t.Fatalf("expected the finding at src/db.py:5, got %s:%d", f.File, f.Line)
	}
	if f.RuleID != "security/sql-injection" {
		t.Fatalf("expected rule security/sql-injection, got %q", f.RuleID)
	}
	if f.Severity != "error" {
		t.Fatalf("expected severity error from the rule, got %q", f.Severity)
	}
	if !strings.Contains(f.Message, "(rule security/sql-injection: SQL Injection Vulnerability)") {
		t.Fatalf("the finding must cite its sustaining rule (AUR-434 gate), got %q", f.Message)
	}
	if !strings.Contains(f.Message, "standards/security-review SCR-001") {
		t.Fatalf("the finding must cite the project security standard, got %q", f.Message)
	}
}

func testAUR435ScanIgnoresRemovedAndContextLines(t *testing.T) {
	// The same vulnerable body as a removed line and as a context line:
	// neither is new code under review, so neither may produce a finding.
	diff := aur435Diff("src/old.py", 1,
		"-"+aur435SQLInjectionLine,
		" "+aur435SQLInjectionLine,
		"+    return None",
	)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("removed/context lines must not produce findings, got %+v", findings)
	}
}

func testAUR435ScanTracksHunkLineNumbers(t *testing.T) {
	// NewStart=10; a context line (10), a removed line (does not advance
	// the new-file counter), a context line (11), then the added
	// vulnerable line at new-file line 12.
	diff := aur435Diff("src/db.py", 10,
		" def find_user(db, name):",
		"-    return None",
		"     # rewritten below",
		"+"+aur435SQLInjectionLine,
	)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected exactly one finding, got %+v", findings)
	}
	if findings[0].Line != 12 {
		t.Fatalf("expected the finding at new-file line 12, got %d", findings[0].Line)
	}
}

func testAUR435FindingsCiteOnlySecurityCategoryRules(t *testing.T) {
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	diff := aur435Diff("src/db.py", 1, "+"+aur435SQLInjectionLine)
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

func testAUR435ScanIsDeterministic(t *testing.T) {
	diff := aur435Diff("src/db.py", 1,
		"+def a():",
		"+"+aur435SQLInjectionLine,
		"+def b():",
		"+"+aur435SQLInjectionLine,
	)
	first, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	second, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("SecurityScan is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if len(first) != 2 || first[0].Line >= first[1].Line {
		t.Fatalf("expected two findings in ascending line order, got %+v", first)
	}
}

func testAUR435CleanDiffYieldsNoFindings(t *testing.T) {
	diff := aur435Diff("config/app.yaml", 1,
		"+app:",
		"+  name: demo",
		"+  greeting: hello",
	)
	findings, err := review.SecurityScan(diff)
	if err != nil {
		t.Fatalf("SecurityScan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a clean diff must yield no security findings, got %+v", findings)
	}
}
