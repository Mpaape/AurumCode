// Package integration holds structural integration checks for reconstruction
// board cards. This file belongs to AUR-015: it validates that the secure
// code review rule catalog (standards/security-review/rules.md) binds every
// required rule ID to a versioned, dated, allowlisted OWASP/CWE/NIST SSDF
// citation and to exactly one of the nine expected CR-* controls, and that
// every rule's vulnerable/fixed fixture pair actually exists and differs —
// mirroring the checks tests/acceptance/AUR-015.sh performs inside the
// sealed acceptance container so the same regression is caught by the
// normal `go test ./...` suite.
package integration

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAUR015Boundary is the AUR-015 integration selector
// (tests/integration/AUR-015_test.go::TestAUR015Boundary), following the
// Test-prefixed discoverable naming already used by TestAUR020Boundary
// (.board/CARD_TEMPLATE.md). Being a `Test`-prefixed function of this shape
// means `go test ./...` discovers and runs it on its own, with no external
// dispatcher: this is what makes the regression it checks for actually run
// in CI instead of silently compiling into dead code.
func TestAUR015Boundary(t *testing.T) {
	t.Helper()

	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("AUR-015 repo root unresolved: %v", err)
	}

	research := filepath.Join(root, ".board/research/secure-code-review.md")
	rulesPath := filepath.Join(root, "standards/security-review/rules.md")
	specDir := filepath.Join(root, "tests/specs/AUR-015")
	ruleIDsPath := filepath.Join(specDir, "rule-ids.txt")
	domainsPath := filepath.Join(specDir, "allowed-source-domains.txt")
	docsPath := filepath.Join(root, "docs/specs/AUR-015.md")

	for _, f := range []string{research, rulesPath, ruleIDsPath, domainsPath, docsPath} {
		requireArtifactAUR015(t, root, f)
	}

	researchText := readFileAUR015(t, research)
	lowerResearch := strings.ToLower(researchText)
	for _, needle := range []string{"OWASP", "threat", "trust boundary", "fail closed"} {
		if !strings.Contains(lowerResearch, strings.ToLower(needle)) {
			t.Fatalf("AUR-015: research baseline missing required term: %s", needle)
		}
	}

	requiredRules := readNonEmptyLinesAUR015(t, ruleIDsPath)
	if len(requiredRules) != 9 {
		t.Fatalf("AUR-015: rule-ids fixture declares %d rule IDs, want exactly 9", len(requiredRules))
	}
	requiredSet := map[string]bool{}
	for _, id := range requiredRules {
		requiredSet[id] = true
	}

	allowedDomains := map[string]bool{}
	for _, d := range readNonEmptyLinesAUR015(t, domainsPath) {
		allowedDomains[d] = true
	}
	if len(allowedDomains) == 0 {
		t.Fatalf("AUR-015: allowed-source-domains fixture is empty")
	}

	// Fixed expectation of full control coverage, matching this card's own
	// `controls:` frontmatter. Not read from the card file: `.board/cards`
	// is outside this card's owned paths.
	expectedControls := []string{
		"CR-EVD-001", "CR-EVD-002", "CR-EVD-003",
		"CR-REV-001", "CR-REV-002", "CR-REV-003", "CR-REV-004", "CR-REV-005",
		"CR-GATE-001",
	}

	records := parseSecurityReviewRulesAUR015(t, rulesPath)
	if len(records) != 9 {
		t.Fatalf("AUR-015: rules.md declares %d rule blocks, want exactly 9", len(records))
	}

	seenID := map[string]bool{}
	usedControl := map[string]bool{}
	for _, rec := range records {
		id := rec.id
		if !requiredSet[id] {
			t.Fatalf("AUR-015: rule id not in tests/specs/AUR-015/rule-ids.txt: %s", id)
		}
		if seenID[id] {
			t.Fatalf("AUR-015: rule id cited twice: %s", id)
		}
		seenID[id] = true

		checkCitationAUR015(t, id, "OWASP", rec.owasp, allowedDomains)
		checkCitationAUR015(t, id, "CWE", rec.cwe, allowedDomains)
		if !cweNumericRe.MatchString(rec.cwe.label) {
			t.Fatalf("AUR-015: %s: CWE has no numeric CWE-<n> identifier", id)
		}
		checkCitationAUR015(t, id, "NIST SSDF", rec.ssdf, allowedDomains)

		if !controlIDRe.MatchString(rec.control) {
			t.Fatalf("AUR-015: %s: control is missing or not a CR-<FAMILY>-<NNN> id: %s", id, rec.control)
		}
		if usedControl[rec.control] {
			t.Fatalf("AUR-015: %s: control already bound to another rule: %s", id, rec.control)
		}
		usedControl[rec.control] = true

		if strings.TrimSpace(rec.trustBoundary) == "" {
			t.Fatalf("AUR-015: %s: has no Trust boundary statement", id)
		}

		if rec.vulnFixture == "" || rec.fixedFixture == "" {
			t.Fatalf("AUR-015: %s: missing vulnerable or fixed fixture path", id)
		}
		wantPrefix := "tests/specs/AUR-015/" + id + "/"
		if !strings.HasPrefix(rec.vulnFixture, wantPrefix) || !strings.HasPrefix(rec.fixedFixture, wantPrefix) {
			t.Fatalf("AUR-015: %s: fixture path is outside its own %s directory", id, wantPrefix)
		}

		checkFixtureAUR015(t, root, id, rec.vulnFixture, "VULNERABLE")
		checkFixtureAUR015(t, root, id, rec.fixedFixture, "FIXED")

		vulnBytes := readFileAUR015(t, filepath.Join(root, rec.vulnFixture))
		fixedBytes := readFileAUR015(t, filepath.Join(root, rec.fixedFixture))
		if vulnBytes == fixedBytes {
			t.Fatalf("AUR-015: %s: vulnerable and fixed fixtures are byte-identical: %s", id, rec.vulnFixture)
		}
	}

	for id := range requiredSet {
		if !seenID[id] {
			t.Fatalf("AUR-015: required rule missing from rules.md: %s", id)
		}
	}
	for _, ctl := range expectedControls {
		if !usedControl[ctl] {
			t.Fatalf("AUR-015: expected control has no bound rule: %s", ctl)
		}
	}
}

// citationAUR015 is one parsed "[label](url) | version <v> | <YYYY-MM-DD>"
// field value.
type citationAUR015 struct {
	label, url, version, date string
}

var (
	citationLineRe = regexp.MustCompile(`^\[(.+)\]\((\S+)\)\s*\|\s*version\s+(\S+)\s*\|\s*(.+)$`)
	cweNumericRe   = regexp.MustCompile(`CWE-([0-9]+)`)
	controlIDRe    = regexp.MustCompile(`^CR-(EVD|REV|GATE)-[0-9]{3}$`)
	versionAUR015  = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
	dateAUR015     = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	ruleHeadingRe  = regexp.MustCompile(`^### Rule: (.+)$`)
)

// parseCitationAUR015 parses a field value of the form
// "[label](url) | version <v> | <date>"; an unparsable value returns a
// zero-value citation whose empty url fails checkCitationAUR015's own
// "has no [label](url) link" check, so a malformed line is never silently
// dropped.
func parseCitationAUR015(value string) citationAUR015 {
	m := citationLineRe.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return citationAUR015{}
	}
	return citationAUR015{label: m[1], url: m[2], version: m[3], date: strings.TrimSpace(m[4])}
}

// ruleRecordAUR015 is one "### Rule: <ID> — <title>" block from rules.md.
type ruleRecordAUR015 struct {
	id                        string
	owasp, cwe, ssdf          citationAUR015
	control                   string
	trustBoundary             string
	vulnFixture, fixedFixture string
}

// parseSecurityReviewRulesAUR015 extracts every "### Rule: <ID> — <title>"
// block inside the "## Rules" section of rules.md. Content outside that
// section — including the "## Field format" example block, which itself
// contains lines shaped like "- OWASP: ..." — is never scanned, matching
// tests/acceptance/AUR-015.sh's own in_section state machine.
func parseSecurityReviewRulesAUR015(t *testing.T, path string) []ruleRecordAUR015 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-015: rules.md unreadable: %v", err)
	}

	inSection := false
	var records []ruleRecordAUR015
	var cur *ruleRecordAUR015

	flush := func() {
		if cur != nil {
			records = append(records, *cur)
			cur = nil
		}
	}

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \t")

		if line == "## Rules" {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") && line != "## Rules" {
			inSection = false
		}
		if !inSection {
			continue
		}

		if m := ruleHeadingRe.FindStringSubmatch(line); m != nil {
			flush()
			title := m[1]
			id := title
			if idx := strings.Index(title, " — "); idx >= 0 {
				id = title[:idx]
			}
			cur = &ruleRecordAUR015{id: strings.TrimSpace(id)}
			continue
		}
		if cur == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "- OWASP: "):
			cur.owasp = parseCitationAUR015(strings.TrimPrefix(line, "- OWASP: "))
		case strings.HasPrefix(line, "- CWE: "):
			cur.cwe = parseCitationAUR015(strings.TrimPrefix(line, "- CWE: "))
		case strings.HasPrefix(line, "- NIST SSDF: "):
			cur.ssdf = parseCitationAUR015(strings.TrimPrefix(line, "- NIST SSDF: "))
		case strings.HasPrefix(line, "- Control: "):
			cur.control = strings.TrimSpace(strings.TrimPrefix(line, "- Control: "))
		case strings.HasPrefix(line, "- Trust boundary: "):
			cur.trustBoundary = strings.TrimSpace(strings.TrimPrefix(line, "- Trust boundary: "))
		case strings.HasPrefix(line, "- Vulnerable fixture: "):
			cur.vulnFixture = strings.TrimSpace(strings.TrimPrefix(line, "- Vulnerable fixture: "))
		case strings.HasPrefix(line, "- Fixed fixture: "):
			cur.fixedFixture = strings.TrimSpace(strings.TrimPrefix(line, "- Fixed fixture: "))
		}
	}
	flush()
	return records
}

// checkCitationAUR015 mirrors tests/acceptance/AUR-015.sh's checkurl /
// checkver / checkdate: a missing link, a non-http(s) or non-allowlisted
// host, an unversioned source, or a non-ISO-8601 date all fail closed.
func checkCitationAUR015(t *testing.T, id, label string, cit citationAUR015, allowedDomains map[string]bool) {
	t.Helper()
	if cit.url == "" {
		t.Fatalf("AUR-015: %s: %s has no [label](url) link", id, label)
	}
	u, err := url.Parse(cit.url)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		t.Fatalf("AUR-015: %s: %s url is not http(s): %s", id, label, cit.url)
	}
	if !allowedDomains[u.Host] {
		t.Fatalf("AUR-015: %s: %s source host is not allowlisted: %s", id, label, u.Host)
	}
	if !versionAUR015.MatchString(cit.version) {
		t.Fatalf("AUR-015: %s: %s has no versioned source: %q", id, label, cit.version)
	}
	if !dateAUR015.MatchString(cit.date) {
		t.Fatalf("AUR-015: %s: %s source date is not ISO 8601: %q", id, label, cit.date)
	}
}

// checkFixtureAUR015 mirrors tests/acceptance/AUR-015.sh's fixture-pair
// loop: absent, symlinked, empty, or unmarked fixtures all fail closed.
func checkFixtureAUR015(t *testing.T, root, id, relPath, marker string) {
	t.Helper()
	full := filepath.Join(root, relPath)
	info, err := os.Lstat(full)
	if err != nil {
		t.Fatalf("AUR-015: %s: fixture absent: %s", id, relPath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("AUR-015: %s: fixture is a symlink: %s", id, relPath)
	}
	if info.Size() == 0 {
		t.Fatalf("AUR-015: %s: fixture is empty: %s", id, relPath)
	}
	data := readFileAUR015(t, full)
	if !strings.Contains(data, "Fixture: "+marker) {
		t.Fatalf("AUR-015: %s: fixture is missing its 'Fixture: %s' marker: %s", id, marker, relPath)
	}
}

// requireArtifactAUR015 mirrors tests/acceptance/AUR-015.sh's card-owned
// artifact loop: absent, symlinked, or empty artifacts fail closed before
// any other check runs.
func requireArtifactAUR015(t *testing.T, root, path string) {
	t.Helper()
	rel, relErr := filepath.Rel(root, path)
	if relErr != nil {
		rel = path
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("AUR-015: required artifact absent: %s", rel)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("AUR-015: required artifact is a symlink: %s", rel)
	}
	if info.Size() == 0 {
		t.Fatalf("AUR-015: required artifact is empty: %s", rel)
	}
}

// readFileAUR015 reads a file whose absence is already an infrastructure
// failure for this test (not a behavioral one, since existence was checked
// by requireArtifactAUR015/checkFixtureAUR015 first).
func readFileAUR015(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-015: file unreadable: %s: %v", path, err)
	}
	return string(data)
}

// readNonEmptyLinesAUR015 reads a newline-delimited fixture, trimming
// whitespace and dropping empty lines, mirroring the shell script's
// `grep -Ev '^[[:space:]]*$'`.
func readNonEmptyLinesAUR015(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-015: fixture unreadable: %s: %v", path, err)
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
