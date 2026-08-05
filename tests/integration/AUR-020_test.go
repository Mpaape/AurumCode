// Package integration holds structural integration checks for reconstruction
// board cards. This file belongs to AUR-020: it validates that the memory
// alternatives matrix, the ADR that resolves against it, and the declared
// memory limits are structurally complete, mirroring the checks
// tests/acceptance/AUR-020.sh performs inside the sealed acceptance
// container so the same regression is caught by the normal `go test ./...`
// suite.
package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAUR020Boundary is the AUR-020 integration selector
// (tests/integration/AUR-020.go::TestAUR020Boundary), following the
// Test-prefixed discoverable naming already used by AUR-156/170/190
// (.board/CARD_TEMPLATE.md). Being a `Test`-prefixed function of this shape
// means `go test ./...` discovers and runs it on its own, with no external
// dispatcher: this is what makes the regression it checks for actually run
// in CI instead of silently compiling into dead code.
func TestAUR020Boundary(t *testing.T) {
	t.Helper()

	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("AUR-020 repo root unresolved: %v", err)
	}

	alternatives := readNonEmptyLinesAUR020(t, filepath.Join(root, "tests/specs/AUR-020/alternatives.txt"))
	if len(alternatives) != 5 {
		t.Fatalf("AUR-020: alternatives fixture declares %d names, want exactly 5", len(alternatives))
	}
	allowedDomains := map[string]bool{}
	for _, d := range readNonEmptyLinesAUR020(t, filepath.Join(root, "tests/specs/AUR-020/allowed-source-domains.txt")) {
		allowedDomains[d] = true
	}

	header, rows := parseAlternativesMatrixAUR020(t, filepath.Join(root, ".board/research/memory-design.md"))
	if len(header) < 2 {
		t.Fatalf("AUR-020: alternatives matrix declares %d criteria columns, want at least 2", len(header))
	}
	criteria := map[string]bool{}
	for _, h := range header {
		criteria[strings.ToLower(strings.TrimSpace(h))] = true
	}

	linkRe := regexp.MustCompile(`\((https?://[^)/]+)`)
	versionRe := regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,2}$`)
	postRe := regexp.MustCompile(`^post [0-9]{6,}$`)
	dateRe := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

	seen := map[string]int{}
	for _, row := range rows {
		if len(row) < 4+len(header) {
			continue
		}
		name := strings.TrimSpace(row[0])
		if !containsAUR020(alternatives, name) {
			continue
		}
		seen[name]++
		if seen[name] > 1 {
			t.Fatalf("AUR-020: alternative cited twice in matrix: %s", name)
			continue
		}

		source := row[1]
		version := strings.TrimSpace(row[2])
		date := strings.TrimSpace(row[3])

		m := linkRe.FindStringSubmatch(source)
		if m == nil {
			t.Fatalf("AUR-020: alternative %s source is not a [label](url) link", name)
			continue
		}
		host := strings.TrimPrefix(m[1], "https://")
		host = strings.TrimPrefix(host, "http://")
		if !allowedDomains[host] {
			t.Fatalf("AUR-020: alternative %s source host is not allowlisted: %s", name, host)
		}
		if !versionRe.MatchString(version) && !postRe.MatchString(version) {
			t.Fatalf("AUR-020: alternative %s has no versioned source: %q", name, version)
		}
		if !dateRe.MatchString(date) {
			t.Fatalf("AUR-020: alternative %s source date is not ISO 8601: %q", name, date)
		}
		for i := range header {
			cell := strings.TrimSpace(row[4+i])
			if len(cell) < 4 {
				t.Fatalf("AUR-020: alternative %s has an empty criterion cell for %s", name, header[i])
			}
		}
	}
	for _, name := range alternatives {
		if seen[name] == 0 {
			t.Fatalf("AUR-020: required alternative missing from matrix: %s", name)
		}
	}

	adrPath := filepath.Join(root, ".board/decisions/ADR-0001-optional-local-memory.md")
	adrBytes, err := os.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("AUR-020: ADR unreadable: %v", err)
	}
	adr := string(adrBytes)
	for _, name := range alternatives {
		if !strings.Contains(adr, "### Rejected: "+name) {
			t.Fatalf("AUR-020: ADR does not name %s as a rejected alternative", name)
		}
	}
	if !regexp.MustCompile(`(?m)^### Decision: `).MatchString(adr) {
		t.Fatalf("AUR-020: ADR has no ### Decision: section")
	}
	if !strings.Contains(strings.ToLower(adr), "sqlite fts5") {
		t.Fatalf("AUR-020: ADR decision does not close on SQLite FTS5")
	}

	checkCriteriaCoverageAUR020(t, adr, criteria)

	limitsBytes, err := os.ReadFile(filepath.Join(root, "standards/memory/limits.yaml"))
	if err != nil {
		t.Fatalf("AUR-020: limits file unreadable: %v", err)
	}
	var limits struct {
		IndexSize struct {
			MaxIndexBytes int64 `yaml:"max_index_bytes"`
		} `yaml:"index_size"`
		ContextWindow struct {
			MaxContextWindowTokens int64 `yaml:"max_context_window_tokens"`
		} `yaml:"context_window"`
	}
	if err := yaml.Unmarshal(limitsBytes, &limits); err != nil {
		t.Fatalf("AUR-020: standards/memory/limits.yaml is not valid YAML: %v", err)
	}
	if limits.IndexSize.MaxIndexBytes <= 0 {
		t.Fatalf("AUR-020: standards/memory/limits.yaml has no positive max_index_bytes index-size limit")
	}
	if limits.ContextWindow.MaxContextWindowTokens <= 0 {
		t.Fatalf("AUR-020: standards/memory/limits.yaml has no positive max_context_window_tokens context-window limit")
	}
}

// findRepoRoot walks up from the current working directory until it finds
// go.mod, so this file works whether `go test` runs from the module root or
// from tests/integration itself.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func readNonEmptyLinesAUR020(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-020: fixture unreadable: %s: %v", path, err)
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

func containsAUR020(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

var tableRowSplitAUR020 = regexp.MustCompile(`\s*\|\s*`)

// parseAlternativesMatrixAUR020 extracts the "## Alternatives matrix" pipe
// table: it returns the criteria column names (everything right of "Date")
// and one []string per data row (Alternative, Source, Version, Date, then
// one cell per criterion column, in header order).
func parseAlternativesMatrixAUR020(t *testing.T, path string) ([]string, [][]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-020: research document unreadable: %s: %v", path, err)
	}

	lines := strings.Split(string(data), "\n")
	inSection := false
	inTable := false
	headerSeen := false
	var header []string
	var rows [][]string

	splitRow := func(line string) []string {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "|")
		trimmed = strings.TrimSuffix(trimmed, "|")
		trimmed = strings.TrimSpace(trimmed)
		return tableRowSplitAUR020.Split(trimmed, -1)
	}

	for _, raw := range lines {
		line := raw
		if inSection && strings.HasPrefix(strings.TrimRight(line, " \t"), "## ") && strings.TrimSpace(line) != "## Alternatives matrix" {
			break
		}
		if !inSection {
			if strings.TrimSpace(line) == "## Alternatives matrix" {
				inSection = true
			}
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			if inTable {
				break
			}
			continue
		}
		inTable = true
		if !headerSeen {
			cols := splitRow(line)
			if len(cols) > 4 {
				header = cols[4:]
			}
			headerSeen = true
			continue
		}
		if regexp.MustCompile(`^\s*\|\s*-+`).MatchString(line) {
			continue
		}
		rows = append(rows, splitRow(line))
	}

	if !headerSeen {
		t.Fatalf("AUR-020: no ## Alternatives matrix table found in %s", path)
	}
	return header, rows
}

// checkCriteriaCoverageAUR020 walks every "### Rejected: <name>" and
// "### Decision: ..." section of the ADR and verifies (a) every
// "- Criterion: <label> — <text>" line names a label present in criteria,
// and (b) every section cites every criterion at least once.
func checkCriteriaCoverageAUR020(t *testing.T, adr string, criteria map[string]bool) {
	t.Helper()
	lines := strings.Split(adr, "\n")

	section := ""
	cited := map[string]bool{}

	finish := func() {
		if section == "" {
			return
		}
		for c := range criteria {
			if !cited[c] {
				t.Fatalf("AUR-020: %q section has no justification for criterion: %s", section, c)
			}
		}
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "### Rejected: "):
			finish()
			section = strings.TrimPrefix(line, "### Rejected: ")
			cited = map[string]bool{}
		case strings.HasPrefix(line, "### Decision: "):
			finish()
			section = "__decision__"
			cited = map[string]bool{}
		case strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### "):
			finish()
			section = ""
			cited = map[string]bool{}
		case section != "" && strings.HasPrefix(line, "- Criterion: "):
			label := extractCriterionLabelAUR020(line)
			if !criteria[label] {
				t.Fatalf("AUR-020: ADR cites a criterion outside the matrix: %s", label)
			}
			cited[label] = true
		}
	}
	finish()
}

func extractCriterionLabelAUR020(line string) string {
	rest := strings.TrimPrefix(line, "- Criterion: ")
	if idx := strings.Index(rest, " — "); idx >= 0 {
		return strings.ToLower(strings.TrimSpace(rest[:idx]))
	}
	if idx := strings.Index(rest, " -- "); idx >= 0 {
		return strings.ToLower(strings.TrimSpace(rest[:idx]))
	}
	return strings.ToLower(strings.TrimSpace(rest))
}
