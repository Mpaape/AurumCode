// Package integration holds AUR-445's Integration-layer proof: the true
// statements the Unit layer requires the root documentation to carry
// (tests/unit/AUR-445.go) actually have lastro in the source this
// repository ships, read directly -- never by grepping the documentation
// against itself.
//
//  1. LICENSE exists and its first line is a real MIT grant.
//  2. Both cmd/aurumcode and cmd/regenerate-docs are `package main`: the
//     repository really does build two commands, not one.
//  3. internal/documentation/extractors/go/extractor.go names no external
//     tool: it never mentions "gomarkdoc" and never imports "os/exec".
//  4. Every `aurumcode review` flag the corrected documentation names
//     (--base, --fail-on, --modelo, --seguranca, --limite, --pr, --repo,
//     --publicar, --na-linha) is actually registered in
//     cmd/aurumcode/main.go.
//  5. internal/review/rules/security.yml carries exactly 8 rules, of which
//     exactly 2 (security/sql-injection, security/command-injection) carry
//     a `pattern:` the deterministic --seguranca pass matches -- the exact
//     "2 of 8" scope CHANGELOG.md now states.
//
// Scope, same as every sibling card in this office: the sandbox has no
// network and this card never executes `aurumcode` or `regenerate-docs`;
// every check here is a static read of source and rule files already
// materialized by paths/read_paths.
//
// Not named "_test.go" on purpose, same technique as tests/unit/AUR-445.go.
//
// Selector naming note: see tests/unit/AUR-445.go's header. This file uses
// IntegrationAUR445 instead of the card text's IntegrationAUR428, which
// already exists in this package's tests/integration/AUR-428.go and would
// collide.
package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// aur445IntegrationRoot mirrors tests/unit/AUR-445.go's root resolution,
// duplicated rather than imported because these test-support files are not
// wired into a shared internal package (matching every sibling card).
func aur445IntegrationRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-445/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-445/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

func aur445ReadFile(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("AUR-445/AC-001/infrastructure: %s unreadable: %v", rel, err)
	}
	return string(raw)
}

// IntegrationAUR445 is AUR-445's Integration-layer selector.
func IntegrationAUR445(t *testing.T) {
	root := aur445IntegrationRoot(t)

	// 1. LICENSE is a real MIT grant, not a placeholder.
	license := aur445ReadFile(t, root, "LICENSE")
	firstLine := strings.SplitN(license, "\n", 2)[0]
	if strings.TrimSpace(firstLine) != "MIT License" {
		t.Fatalf("AUR-445/AC-001/behavior-missing: LICENSE's first line is %q, want %q", firstLine, "MIT License")
	}
	if !strings.Contains(license, "Copyright (c)") {
		t.Fatalf("AUR-445/AC-001/behavior-missing: LICENSE carries no copyright line")
	}

	// 2. Two package main commands really exist.
	for _, rel := range []string{
		filepath.Join("cmd", "aurumcode", "main.go"),
		filepath.Join("cmd", "regenerate-docs", "main.go"),
	} {
		src := aur445ReadFile(t, root, rel)
		if !regexp.MustCompile(`(?m)^package main\s*$`).MatchString(src) {
			t.Fatalf("AUR-445/AC-001/behavior-missing: %s does not declare package main", rel)
		}
	}

	// 3. Go's extractor names no external tool.
	goExtractorDir := filepath.Join(root, "internal", "documentation", "extractors", "go")
	entries, err := os.ReadDir(goExtractorDir)
	if err != nil {
		t.Fatalf("AUR-445/AC-001/infrastructure: reading %s: %v", goExtractorDir, err)
	}
	sawGoFile := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		sawGoFile = true
		rel := filepath.Join("internal", "documentation", "extractors", "go", entry.Name())
		src := aur445ReadFile(t, root, rel)
		lower := strings.ToLower(src)
		// The extractor's own doc comments are allowed to name "gomarkdoc"
		// historically (it explains what AUR-424 removed); what must never
		// appear is an actual dependency on it: an exec.Command/exec.LookPath
		// call or an "os/exec" import.
		if strings.Contains(src, `"os/exec"`) {
			t.Fatalf("AUR-445/AC-001/behavior-missing: %s imports os/exec; Go extraction is claimed to need no external tool", rel)
		}
		if strings.Contains(lower, "exec.command(") || strings.Contains(lower, "exec.lookpath(") {
			t.Fatalf("AUR-445/AC-001/behavior-missing: %s shells out (exec.Command/exec.LookPath); Go extraction is claimed to need no external tool", rel)
		}
	}
	if !sawGoFile {
		t.Fatalf("AUR-445/AC-001/infrastructure: no .go file found under %s", goExtractorDir)
	}

	// 4. Every flag the corrected documentation names is really registered.
	aurumcodeMain := aur445ReadFile(t, root, filepath.Join("cmd", "aurumcode", "main.go"))
	for _, flag := range []string{"base", "fail-on", "modelo", "seguranca", "limite", "pr", "repo", "publicar", "na-linha"} {
		pattern := regexp.MustCompile(`fs\.(String|Bool|Int)\(\s*"` + regexp.QuoteMeta(flag) + `"`)
		if !pattern.MatchString(aurumcodeMain) {
			t.Fatalf("AUR-445/AC-001/behavior-missing: cmd/aurumcode/main.go registers no --%s flag, but the documentation names it", flag)
		}
	}
	// regenerate-docs, by contrast, must register no flag package use at
	// all: it is genuinely all environment variables, which is what makes
	// the corrected "split by binary" documentation honest rather than just
	// differently wrong.
	regenMain := aur445ReadFile(t, root, filepath.Join("cmd", "regenerate-docs", "main.go"))
	if strings.Contains(regenMain, `"flag"`) {
		t.Fatalf("AUR-445/AC-001/behavior-missing: cmd/regenerate-docs/main.go imports \"flag\"; the documentation's env-var-only claim for this binary would be false")
	}

	// 5. The --seguranca scope is exactly 2 of 8 rules with a matcher.
	rulesYAML := aur445ReadFile(t, root, filepath.Join("internal", "review", "rules", "security.yml"))
	ruleIDs := regexp.MustCompile(`(?m)^\s*-\s*id:\s*(\S+)`).FindAllStringSubmatch(rulesYAML, -1)
	if len(ruleIDs) != 8 {
		t.Fatalf("AUR-445/AC-001/behavior-missing: security.yml declares %d rules, want 8 (the count CHANGELOG.md now cites)", len(ruleIDs))
	}
	patternLines := regexp.MustCompile(`(?m)^\s*pattern:\s*`).FindAllString(rulesYAML, -1)
	if len(patternLines) != 2 {
		t.Fatalf("AUR-445/AC-001/behavior-missing: security.yml declares %d `pattern:` matchers, want 2 (the count CHANGELOG.md now cites)", len(patternLines))
	}
	wantMatched := map[string]bool{"security/sql-injection": false, "security/command-injection": false}
	// A rule "has a pattern" when its own id: line is followed by a
	// pattern: line before the next id: line -- walk the id boundaries
	// rather than assume adjacency, since a rule may carry comments or
	// other keys (title, severity, standard) between id and pattern.
	idIdx := regexp.MustCompile(`(?m)^\s*-\s*id:\s*(\S+)`).FindAllStringSubmatchIndex(rulesYAML, -1)
	for i, m := range idIdx {
		id := rulesYAML[m[2]:m[3]]
		end := len(rulesYAML)
		if i+1 < len(idIdx) {
			end = idIdx[i+1][0]
		}
		block := rulesYAML[m[0]:end]
		if regexp.MustCompile(`(?m)^\s*pattern:\s*`).MatchString(block) {
			if _, tracked := wantMatched[id]; tracked {
				wantMatched[id] = true
			}
		}
	}
	for id, matched := range wantMatched {
		if !matched {
			t.Fatalf("AUR-445/AC-001/behavior-missing: rule %s does not carry a pattern: matcher, contradicting the documented 2-of-8 scope", id)
		}
	}

	t.Logf("AUR-445/AC-001/integration pass rules=%d patterns=%d", len(ruleIDs), len(patternLines))
}
