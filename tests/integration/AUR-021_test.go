// Package integration holds structural integration checks for reconstruction
// board cards. This file belongs to AUR-021: it validates that the MCP
// conformance/security baseline, the pinned protocol/SDK standard, and the
// tool read-only/mutable classification are structurally complete and
// mutually consistent, mirroring the checks tests/acceptance/AUR-021.sh
// performs inside the sealed acceptance container so the same regression is
// caught by the normal `go test ./...` suite.
package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAUR021Boundary is the AUR-021 integration selector
// (tests/integration/AUR-021_test.go::TestAUR021Boundary), following the
// Test-prefixed discoverable naming already used by AUR-020/156/170/190
// (.board/CARD_TEMPLATE.md). Being a `Test`-prefixed function of this shape
// means `go test ./...` discovers and runs it on its own, with no external
// dispatcher: this is what makes the regression it checks for actually run
// in CI instead of silently compiling into dead code.
func TestAUR021Boundary(t *testing.T) {
	t.Helper()

	root, err := findRepoRootAUR021(t)
	if err != nil {
		t.Fatalf("AUR-021 repo root unresolved: %v", err)
	}

	researchPath := filepath.Join(root, ".board/research/mcp.md")
	research, err := os.ReadFile(researchPath)
	if err != nil {
		t.Fatalf("AUR-021: research document unreadable: %v", err)
	}
	researchText := string(research)

	// -------------------------------------------------------------------
	// Given: .board/research/mcp.md contains, case-insensitive, every term
	// named in the fixture (stdio, read-only, OAuth, injection, capability
	// at time of writing). Absence of any one is the documented invalid
	// case.
	// -------------------------------------------------------------------
	requiredTerms := readNonEmptyLinesAUR021(t, filepath.Join(root, "tests/specs/AUR-021/required-terms.txt"))
	if len(requiredTerms) == 0 {
		t.Fatalf("AUR-021: required-terms fixture declares no terms")
	}
	lowerResearch := strings.ToLower(researchText)
	for _, term := range requiredTerms {
		if !strings.Contains(lowerResearch, strings.ToLower(term)) {
			t.Fatalf("AUR-021: required term missing from %s: %s", researchPath, term)
		}
	}

	// -------------------------------------------------------------------
	// Network-source boundary (MUT-002 guard): parse the "## Sources
	// matrix" pipe table. Every data row must cite a [label](url) source
	// whose host is allowlisted, a parseable version, an ISO-8601 date,
	// and a non-empty "what it fixes" cell.
	// -------------------------------------------------------------------
	allowedDomains := map[string]bool{}
	for _, d := range readNonEmptyLinesAUR021(t, filepath.Join(root, "tests/specs/AUR-021/allowed-source-domains.txt")) {
		allowedDomains[d] = true
	}

	rows := parseSourcesMatrixAUR021(t, researchText)
	if len(rows) < 5 {
		t.Fatalf("AUR-021: ## Sources matrix has %d data rows, want at least 5", len(rows))
	}

	linkRe := regexp.MustCompile(`\((https?://[^)/]+)`)
	versionRe := regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,2}$`)
	dateOrVersionDateRe := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}(\s.*)?$`)
	dateRe := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

	for _, row := range rows {
		if len(row) < 5 {
			t.Fatalf("AUR-021: sources matrix row has %d columns, want at least 5: %v", len(row), row)
		}
		ref := strings.TrimSpace(row[0])
		src := row[1]
		ver := strings.TrimSpace(row[2])
		date := strings.TrimSpace(row[3])
		fixes := strings.TrimSpace(row[4])

		if len(ref) < 4 {
			t.Fatalf("AUR-021: sources matrix row has an empty Reference cell")
		}
		m := linkRe.FindStringSubmatch(src)
		if m == nil {
			t.Fatalf("AUR-021: reference %s source is not a [label](url) link", ref)
		}
		host := strings.TrimPrefix(m[1], "https://")
		host = strings.TrimPrefix(host, "http://")
		if !allowedDomains[host] {
			t.Fatalf("AUR-021: reference %s source host is not allowlisted: %s", ref, host)
		}
		if !versionRe.MatchString(ver) && !dateOrVersionDateRe.MatchString(ver) {
			t.Fatalf("AUR-021: reference %s has no versioned source: %q", ref, ver)
		}
		if !dateRe.MatchString(date) {
			t.Fatalf("AUR-021: reference %s source date is not ISO 8601: %q", ref, date)
		}
		if len(fixes) < 4 {
			t.Fatalf("AUR-021: reference %s has an empty \"What it fixes\" cell", ref)
		}
	}

	// -------------------------------------------------------------------
	// standards/mcp/protocol.yaml: protocol.version and go_sdk.version
	// must both be declared, and both must also appear verbatim in the
	// research document, so the pinned standard cannot silently drift
	// from the source that justifies it.
	// -------------------------------------------------------------------
	protocolPath := filepath.Join(root, "standards/mcp/protocol.yaml")
	protocolBytes, err := os.ReadFile(protocolPath)
	if err != nil {
		t.Fatalf("AUR-021: protocol standard unreadable: %v", err)
	}
	var protocol struct {
		Protocol struct {
			Version    string   `yaml:"version"`
			Transports []string `yaml:"transports"`
		} `yaml:"protocol"`
		GoSDK struct {
			Module  string `yaml:"module"`
			Version string `yaml:"version"`
		} `yaml:"go_sdk"`
	}
	if err := yaml.Unmarshal(protocolBytes, &protocol); err != nil {
		t.Fatalf("AUR-021: %s is not valid YAML: %v", protocolPath, err)
	}
	if !dateRe.MatchString(protocol.Protocol.Version) {
		t.Fatalf("AUR-021: %s has no ISO-8601 protocol.version", protocolPath)
	}
	if protocol.GoSDK.Module == "" || !strings.Contains(protocol.GoSDK.Module, "/") {
		t.Fatalf("AUR-021: %s has no go_sdk.module", protocolPath)
	}
	semverRe := regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	if !semverRe.MatchString(protocol.GoSDK.Version) {
		t.Fatalf("AUR-021: %s has no semantic go_sdk.version", protocolPath)
	}
	if !strings.Contains(researchText, protocol.Protocol.Version) {
		t.Fatalf("AUR-021: %s protocol.version %s is not cited in %s", protocolPath, protocol.Protocol.Version, researchPath)
	}
	if !strings.Contains(researchText, protocol.GoSDK.Version) {
		t.Fatalf("AUR-021: %s go_sdk.version %s is not cited in %s", protocolPath, protocol.GoSDK.Version, researchPath)
	}

	// -------------------------------------------------------------------
	// standards/mcp/tools.yaml (MUT-001 guard): every declared tool has
	// class read_only or mutable; every mutable tool declares
	// requires_explicit_consent: true; no read_only tool sets it; every
	// tool named in the required-mutable-tools fixture (review.publish, at
	// time of writing) is present, classified mutable, and requires
	// explicit consent. Reclassifying that tool read_only, or dropping its
	// consent flag, fails here.
	// -------------------------------------------------------------------
	toolsPath := filepath.Join(root, "standards/mcp/tools.yaml")
	toolsBytes, err := os.ReadFile(toolsPath)
	if err != nil {
		t.Fatalf("AUR-021: tools standard unreadable: %v", err)
	}
	var toolsDoc struct {
		Tools []struct {
			Name                    string `yaml:"name"`
			Class                   string `yaml:"class"`
			RequiresExplicitConsent bool   `yaml:"requires_explicit_consent"`
		} `yaml:"tools"`
	}
	if err := yaml.Unmarshal(toolsBytes, &toolsDoc); err != nil {
		t.Fatalf("AUR-021: %s is not valid YAML: %v", toolsPath, err)
	}
	if len(toolsDoc.Tools) == 0 {
		t.Fatalf("AUR-021: %s declares no tools", toolsPath)
	}

	seen := map[string]struct {
		class   string
		consent bool
	}{}
	for _, tool := range toolsDoc.Tools {
		if tool.Class != "read_only" && tool.Class != "mutable" {
			t.Fatalf("AUR-021: tool %s has an invalid class: %q", tool.Name, tool.Class)
		}
		if tool.Class == "mutable" && !tool.RequiresExplicitConsent {
			t.Fatalf("AUR-021: mutable tool %s does not set requires_explicit_consent: true", tool.Name)
		}
		if tool.Class == "read_only" && tool.RequiresExplicitConsent {
			t.Fatalf("AUR-021: read_only tool %s must not set requires_explicit_consent", tool.Name)
		}
		seen[tool.Name] = struct {
			class   string
			consent bool
		}{tool.Class, tool.RequiresExplicitConsent}
	}

	requiredMutable := readNonEmptyLinesAUR021(t, filepath.Join(root, "tests/specs/AUR-021/required-mutable-tools.txt"))
	if len(requiredMutable) == 0 {
		t.Fatalf("AUR-021: required-mutable-tools fixture declares no tools")
	}
	for _, name := range requiredMutable {
		got, ok := seen[name]
		if !ok {
			t.Fatalf("AUR-021: required mutable tool missing from %s: %s", toolsPath, name)
		}
		if got.class != "mutable" || !got.consent {
			t.Fatalf("AUR-021: required tool %s must be class: mutable with requires_explicit_consent: true, got class=%s requires_explicit_consent=%v", name, got.class, got.consent)
		}
	}
}

// findRepoRootAUR021 walks up from the current working directory until it
// finds go.mod, so this file works whether `go test` runs from the module
// root or from tests/integration itself.
func findRepoRootAUR021(t *testing.T) (string, error) {
	t.Helper()
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

func readNonEmptyLinesAUR021(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-021: fixture unreadable: %s: %v", path, err)
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

var tableRowSplitAUR021 = regexp.MustCompile(`\s*\|\s*`)

// parseSourcesMatrixAUR021 extracts the "## Sources matrix" pipe table's
// data rows (Reference, Source, Version, Date, What it fixes), skipping the
// header and separator rows.
func parseSourcesMatrixAUR021(t *testing.T, text string) [][]string {
	t.Helper()

	splitRow := func(line string) []string {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "|")
		trimmed = strings.TrimSuffix(trimmed, "|")
		trimmed = strings.TrimSpace(trimmed)
		return tableRowSplitAUR021.Split(trimmed, -1)
	}

	lines := strings.Split(text, "\n")
	inSection := false
	inTable := false
	headerSeen := false
	var rows [][]string

	for _, raw := range lines {
		line := raw
		if inSection && strings.HasPrefix(strings.TrimRight(line, " \t"), "## ") && strings.TrimSpace(line) != "## Sources matrix" {
			break
		}
		if !inSection {
			if strings.TrimSpace(line) == "## Sources matrix" {
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
			headerSeen = true
			continue
		}
		if regexp.MustCompile(`^\s*\|\s*-+`).MatchString(line) {
			continue
		}
		rows = append(rows, splitRow(line))
	}

	if !headerSeen {
		t.Fatalf("AUR-021: no ## Sources matrix table found")
	}
	return rows
}
