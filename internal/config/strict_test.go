package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAUR575UnknownKeyIsRefused: a misspelled key in any section of the
// repository's config.yml is a configuration error naming the key.
func TestAUR575UnknownKeyIsRefused(t *testing.T) {
	cases := map[string]string{
		"gate.fial_on":           "gate:\n  fial_on: [error]\n",
		"quality_gates.sast":     "quality_gates:\n  sast:\n    enabeld: true\n",
		"top-level":              "gaet:\n  fail_on: [error]\n",
		"review.context":         "review:\n  context:\n    skils: [a.md]\n",
		"exceptions entry field": "exceptions:\n  - rule: x\n    ownr: a\n",
	}
	for name, doc := range cases {
		_, err := Parse([]byte(doc), "repo/.aurumcode/config.yml")
		if err == nil {
			t.Fatalf("%s: Parse accepted an unknown key", name)
		}
		if !strings.Contains(err.Error(), "not found in type") || !strings.Contains(err.Error(), "repo/.aurumcode/config.yml") {
			t.Fatalf("%s: error %q does not name the key and the file", name, err)
		}
	}
	if _, err := Parse([]byte("gate:\n  fial_on: [error]\n"), "c.yml"); err == nil || !strings.Contains(err.Error(), "fial_on") {
		t.Fatalf("error %v must name the unknown key fial_on", err)
	}
	if _, err := Parse([]byte("# only a comment\n"), "c.yml"); err != nil {
		t.Fatalf("comment-only config must stay the zero config: %v", err)
	}
}

// TestAUR575SastSectionValidatedInParse: quality_gates.sast is validated
// with every other section, so an unsupported engine or a flag-like rule
// pack never reaches a run.
func TestAUR575SastSectionValidatedInParse(t *testing.T) {
	cases := map[string]string{
		"quality_gates.sast.engine":     "quality_gates:\n  sast:\n    enabled: true\n    engine: gitleaks\n",
		"quality_gates.sast.rule_packs": "quality_gates:\n  sast:\n    enabled: true\n    rule_packs: [\"--dangerous\"]\n",
	}
	for key, doc := range cases {
		_, err := Parse([]byte(doc), "c.yml")
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("Parse(%s) error = %v, want one naming %s", key, err, key)
		}
	}
}

// TestAUR575InconclusiveDefaultBlocksWhenGated: an absent gate.inconclusive
// resolves to block whenever a gate is declared or a scanner is enabled;
// warn only when written.
func TestAUR575InconclusiveDefaultBlocksWhenGated(t *testing.T) {
	cases := []struct {
		doc  string
		want string
	}{
		{"", ""},
		{"gate:\n  fail_on: [error]\n", InconclusiveBlock},
		{"quality_gates:\n  sast:\n    enabled: true\n", InconclusiveBlock},
		{"quality_gates:\n  sast:\n    enabled: false\n", ""},
		{"analysis_data: {}\n", InconclusiveBlock},
		{"quality_gates:\n  sast:\n    enabled: true\ngate:\n  inconclusive: warn\n", InconclusiveWarn},
	}
	for _, c := range cases {
		cfg, err := Parse([]byte(c.doc), "c.yml")
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.doc, err)
		}
		got, err := cfg.InconclusiveMode()
		if err != nil || got != c.want {
			t.Fatalf("InconclusiveMode(%q) = %q, %v; want %q", c.doc, got, err, c.want)
		}
	}
}

// TestAUR575ExistingConfigsStayValid walks every configuration the repository
// ships (demos, tutorials, fixtures, policies) through the strict Parse, so
// the strict decode never breaks a documented example. knownInvalidConfigs
// are the deliberate failure cases (a tutorial's refused policy) and the
// frozen legacy fixture that never parsed, not even before the strict decode.
func TestAUR575ExistingConfigsStayValid(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var checked, present int
	for _, dir := range []string{"demo", "tests", "docs", ".aurumcode"} {
		base := filepath.Join(root, dir)
		if _, statErr := os.Stat(base); statErr != nil {
			continue
		}
		present++
		walkErr := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || info.Name() != "config.yml" || filepath.Base(filepath.Dir(path)) != ".aurumcode" {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			if knownInvalidConfig(filepath.ToSlash(rel)) {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if _, parseErr := Parse(data, path); parseErr != nil {
				t.Errorf("%v", parseErr)
			}
			checked++
			return nil
		})
		if walkErr != nil {
			t.Fatal(walkErr)
		}
	}
	if present == 0 {
		// A copy of only cmd/internal/pkg (a sealed or staged run) ships no
		// configuration to check: say so, never pass silently with zero files.
		t.Skip("raízes do repositório ausentes nesta cópia (demo, tests, docs, .aurumcode)")
	}
	if checked == 0 {
		t.Fatal("no shipped config.yml was found to check")
	}
	t.Logf("checked %d shipped config.yml files", checked)
}

var knownInvalidConfigs = []string{"/politica-invalida/", "/politica-sem-dono/", "tests/fixtures/repo1/"}

func knownInvalidConfig(rel string) bool {
	for _, marker := range knownInvalidConfigs {
		if strings.Contains("/"+rel, marker) || strings.HasPrefix(rel, marker) {
			return true
		}
	}
	return false
}
