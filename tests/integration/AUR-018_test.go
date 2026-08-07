// Package integration holds structural integration checks for reconstruction
// board cards. This file belongs to AUR-018 and is the file its own card
// declares in paths (tests/integration/AUR-018_test.go) and in its TDD
// proof's "Integration:" line (tests/integration/AUR-018_test.go::TestAUR018Boundary).
// It proves that standards/scm/capabilities.yaml (the machine-readable
// limits) and .board/research/scm-ci.md (the dated, versioned research)
// never diverge: every forge named in the YAML matrix must also be named
// in the research matrix table, no forge may declare a write-capable
// credential for a hostile head in either role, and every cited research
// source is a versioned/dated vendor document from an allowlisted host.
package integration

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// scmForgeCapabilities mirrors the subset of standards/scm/capabilities.yaml
// this check needs. It intentionally does not model every field: widening
// the schema there must not silently widen what this integration check
// accepts.
type scmForgeCapabilities struct {
	Rule struct {
		LeastPrivilege      string `yaml:"least_privilege"`
		HostileHeadReadOnly string `yaml:"hostile_head_read_only"`
	} `yaml:"rule"`
	Forges map[string]struct {
		Source   string `yaml:"source"`
		Analyzer struct {
			Credential              string `yaml:"credential"`
			AllowWriteOnHostileHead bool   `yaml:"allow_write_on_hostile_head"`
		} `yaml:"analyzer"`
		Publisher struct {
			Credential              string `yaml:"credential"`
			AllowWriteOnHostileHead bool   `yaml:"allow_write_on_hostile_head"`
		} `yaml:"publisher"`
	} `yaml:"forges"`
}

// aur018RepoRoot locates the repository root by walking up from this source
// file's own location to the nearest ancestor containing go.mod. Using
// runtime.Caller instead of the process working directory keeps this check
// correct whether it is invoked via `go test ./...` from the repo root or
// from within its own package directory.
func aur018RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("AUR-018: could not resolve caller for repo root")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("AUR-018: go.mod not found above tests/integration")
	return ""
}

var aur018AllowlistedDocHosts = map[string]bool{
	"docs.github.com": true,
	"docs.gitlab.com": true,
	"docs.gitea.com":  true,
}

var aur018MarkdownLinkRE = regexp.MustCompile(`\[[^\]]+\]\((https?://[^)]+)\)`)
var aur018VersionOrDateRE = regexp.MustCompile(`\d{4}-\d{2}-\d{2}|v?\d+\.\d+(\.\d+)?`)

// TestAUR018Boundary is the AUR-018 acceptance boundary test, discoverable
// by `go test ./tests/integration/...`. It covers what the shell acceptance
// gate (tests/acceptance/AUR-018.sh) does not, by itself, prove: that the
// machine-readable standard and the research file cross-reference each
// other, that every cited source is a versioned/dated vendor document from
// an allowlisted host (MUT-002's concern), and that the six-term
// nominal/invalid boundary the acceptance script enforces is not an
// accident of one specific wording.
func TestAUR018Boundary(t *testing.T) {
	root := aur018RepoRoot(t)

	yamlPath := filepath.Join(root, "standards", "scm", "capabilities.yaml")
	researchPath := filepath.Join(root, ".board", "research", "scm-ci.md")

	var caps scmForgeCapabilities
	var research string

	t.Run("IntegrationCrossCheck", func(t *testing.T) {
		yamlBytes, err := os.ReadFile(yamlPath)
		if err != nil {
			t.Fatalf("cannot read %s: %v", yamlPath, err)
		}
		if err := yaml.Unmarshal(yamlBytes, &caps); err != nil {
			t.Fatalf("cannot parse %s: %v", yamlPath, err)
		}
		if len(caps.Forges) == 0 {
			t.Fatalf("%s declares no forges", yamlPath)
		}

		requiredForges := []string{"github", "gitea", "gitlab"}
		for _, name := range requiredForges {
			forge, ok := caps.Forges[name]
			if !ok {
				t.Fatalf("%s is missing forge %q", yamlPath, name)
			}
			if forge.Analyzer.AllowWriteOnHostileHead {
				t.Fatalf("forge %q analyzer allows write on a hostile head", name)
			}
			if forge.Publisher.AllowWriteOnHostileHead {
				t.Fatalf("forge %q publisher allows write on a hostile head", name)
			}
			if strings.TrimSpace(forge.Analyzer.Credential) == "" {
				t.Fatalf("forge %q analyzer declares no credential", name)
			}
			if strings.TrimSpace(forge.Publisher.Credential) == "" {
				t.Fatalf("forge %q publisher declares no credential", name)
			}
			if strings.TrimSpace(forge.Source) == "" {
				t.Fatalf("forge %q has no source citation", name)
			}
		}

		if strings.TrimSpace(caps.Rule.HostileHeadReadOnly) == "" {
			t.Fatalf("%s is missing rule.hostile_head_read_only", yamlPath)
		}
		if strings.TrimSpace(caps.Rule.LeastPrivilege) == "" {
			t.Fatalf("%s is missing rule.least_privilege", yamlPath)
		}

		researchBytes, err := os.ReadFile(researchPath)
		if err != nil {
			t.Fatalf("cannot read %s: %v", researchPath, err)
		}
		research = string(researchBytes)

		for _, name := range requiredForges {
			display := map[string]string{"github": "GitHub", "gitea": "Gitea", "gitlab": "GitLab"}[name]
			if !strings.Contains(research, display) {
				t.Fatalf("%s does not mention forge %q present in %s", researchPath, display, yamlPath)
			}
		}

		for _, needle := range []string{"analyzer", "publisher", "least privilege"} {
			if !strings.Contains(strings.ToLower(research), needle) {
				t.Fatalf("%s is missing required term %q", researchPath, needle)
			}
		}
	})

	t.Run("SourceCitationsAreVersionedAndAllowlisted", func(t *testing.T) {
		if research == "" {
			researchBytes, err := os.ReadFile(researchPath)
			if err != nil {
				t.Fatalf("cannot read %s: %v", researchPath, err)
			}
			research = string(researchBytes)
		}
		matches := aur018MarkdownLinkRE.FindAllStringSubmatch(research, -1)
		if len(matches) == 0 {
			t.Fatalf("%s cites no markdown-linked sources", researchPath)
		}
		checked := 0
		for _, m := range matches {
			rawURL := m[1]
			u, err := url.Parse(rawURL)
			if err != nil {
				t.Fatalf("source URL %q does not parse: %v", rawURL, err)
			}
			if u.Scheme != "https" {
				t.Fatalf("source URL %q is not https", rawURL)
			}
			if !aur018AllowlistedDocHosts[u.Host] {
				// Fail-closed, no carve-out: every markdown-linked URL in
				// this file must resolve to the exact host of an
				// allowlisted forge's own documentation. The prior version
				// of this check only rejected a non-allowlisted host if it
				// happened to contain the literal substring "github.com",
				// "gitlab.com", or "gitea.com"; a host like
				// raw.githubusercontent.com contains "github" but never the
				// substring "github.com" (the character right after
				// "github" is "u", from "usercontent", not "."), so that
				// substring check silently `continue`d past it instead of
				// failing. There is no legitimate non-vendor citation in
				// this file today, so an unrecognized host is rejected
				// outright rather than treated as an unrelated, ignorable
				// link.
				t.Fatalf("source URL %q uses a non-allowlisted host: %s", rawURL, u.Host)
			}
			checked++
			// The citation must carry a version or a date somewhere in the
			// same table row as the link, not merely anywhere in the file.
			row := aur018TableRowContaining(research, rawURL)
			if row == "" {
				t.Fatalf("source URL %q is not inside a table row", rawURL)
			}
			if !aur018VersionOrDateRE.MatchString(row) {
				t.Fatalf("source row for %q has no version or date: %q", rawURL, row)
			}
		}
		if checked < 3 {
			t.Fatalf("expected at least 3 allowlisted vendor-doc sources (one per forge), found %d", checked)
		}
	})

	t.Run("SixTermBoundary", func(t *testing.T) {
		if research == "" {
			researchBytes, err := os.ReadFile(researchPath)
			if err != nil {
				t.Fatalf("cannot read %s: %v", researchPath, err)
			}
			research = string(researchBytes)
		}
		requiredTerms := []string{"GitHub", "Gitea", "GitLab", "analyzer", "publisher", "least privilege"}
		lower := strings.ToLower(research)

		t.Run("nominal/all_terms_present", func(t *testing.T) {
			for _, term := range requiredTerms {
				if !strings.Contains(lower, strings.ToLower(term)) {
					t.Fatalf("nominal vector: required term %q is absent from %s", term, researchPath)
				}
			}
		})

		for _, term := range requiredTerms {
			term := term
			t.Run("invalid/missing_"+aur018Sanitize(term), func(t *testing.T) {
				// The invalid vector: content with exactly this one
				// required term excised (case-insensitively) must fail the
				// same containment check the acceptance script performs.
				excised := aur018Excise(research, term)
				if strings.Contains(strings.ToLower(excised), strings.ToLower(term)) {
					t.Fatalf("could not excise term %q from research content for the invalid vector", term)
				}
			})
		}
	})
}

func aur018TableRowContaining(content, needle string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func aur018Excise(content, term string) string {
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(term))
	return re.ReplaceAllString(content, "")
}

func aur018Sanitize(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "_")
}
