// Package unit holds AUR-474's Unit-layer proof: a Bash function whose body
// opens and closes on the SAME physical line (`greet() { echo "hi"; }`,
// `function greet { ... }`, `function greet() { ... }`) is recognized as a
// real symbol by internal/documentation/extractors/bash, carries its own
// preceding comment as its own Doc (never swept into "## Script Notes"),
// and the four AC-002 non-symbol shapes (my-array=(), x=$(cmd), a one-line
// if/fi, and a quoted string containing "foo() { }") still produce no
// symbol at all.
//
// This file is not named "_test.go" on purpose, mirroring every sibling
// card in this office: tests/acceptance/AUR-474.sh stages a private
// writable copy of the module and writes a tiny bridge "_test.go" file that
// calls TestAUR474, so the assertions below run inside the sandboxed
// acceptance instead of an unrelated top-level `go test ./...`.
package unit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	bashextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	powershellextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/powershell"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// bashOneLinerFixture is the exact "Achado medido" fixture from the card:
// a documented one-line function, plus the three sibling one-line forms
// (function name {...}, function name() {...}) and the AC-002 false-
// positive shapes that must keep producing zero symbols.
const bashOneLinerFixture = `#!/bin/bash
set -euo pipefail

# Greets a name.
greet() { echo "hello $1"; }

# Says goodbye.
function farewell { echo "bye $1"; }

# Restarts a service.
function restart_svc() { systemctl restart "$1"; }

my-array=()
x=$(cmd)
if [ -f "$1" ]; then echo "found"; fi
echo "foo() { }"
`

func writeFileAUR474(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func extractOneAUR474(t *testing.T, script string) string {
	t.Helper()
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeFileAUR474(t, filepath.Join(srcDir, "pipeline.sh"), script)

	ext := bashextractor.NewBashExtractor(site.NewMockRunner())
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageBash,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("AUR-474/AC-001/behavior-missing: Extract failed: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatalf("AUR-474/AC-001/behavior-missing: zero pages generated")
	}
	data, err := os.ReadFile(result.Files[0])
	if err != nil {
		t.Fatalf("AUR-474/AC-001/behavior-missing: read generated page: %v", err)
	}
	return string(data)
}

// ownSection returns the text between headingLine and the next code fence,
// so a check can require text be inside a symbol's OWN section, not merely
// present anywhere on the page (which "## Script Notes" would also satisfy).
func ownSection(t *testing.T, page, headingLine string) string {
	t.Helper()
	idx := strings.Index(page, headingLine)
	if idx < 0 {
		t.Fatalf("AUR-474/AC-001/behavior-missing: heading %q not found:\n%s", headingLine, page)
	}
	rest := page[idx+len(headingLine):]
	fenceIdx := strings.Index(rest, "```")
	if fenceIdx < 0 {
		t.Fatalf("AUR-474/AC-001/behavior-missing: no code fence after %q", headingLine)
	}
	return rest[:fenceIdx]
}

// TestAUR474 is AUR-474's Unit-layer selector.
func TestAUR474(t *testing.T) {
	page := extractOneAUR474(t, bashOneLinerFixture)

	// AC-001: all three one-line forms produce a real symbol heading, each
	// carrying its own preceding comment -- not the file's Script Notes.
	for name, doc := range map[string]string{
		"greet":       "Greets a name.",
		"farewell":    "Says goodbye.",
		"restart_svc": "Restarts a service.",
	} {
		heading := "### function " + name
		section := ownSection(t, page, heading)
		if !strings.Contains(section, doc) {
			t.Fatalf("AUR-474/AC-001/behavior-missing: %q's own section carries no doc %q (likely swept into Script Notes):\n%s", name, doc, page)
		}
	}

	// AC-001 (second half of the defect): the doc text must not ALSO
	// appear loose in Script Notes, un-anchored to any symbol -- the exact
	// "falls mute into Script Notes" failure mode the card measured.
	notesIdx := strings.Index(page, "## Script Notes")
	if notesIdx >= 0 {
		firstSymbolIdx := strings.Index(page, "### function ")
		notesSection := page[notesIdx:]
		if firstSymbolIdx > notesIdx {
			notesSection = page[notesIdx:firstSymbolIdx]
		}
		for _, doc := range []string{"Greets a name.", "Says goodbye.", "Restarts a service."} {
			if strings.Contains(notesSection, doc) {
				t.Fatalf("AUR-474/AC-001/false-claim: doc %q fell into Script Notes even though its function was recognized:\n%s", doc, page)
			}
		}
	}

	// AC-002: none of the four false-positive shapes produced a symbol.
	for _, notWant := range []string{"my-array", "cmd", "found", "foo"} {
		if strings.Contains(page, "### function "+notWant) {
			t.Fatalf("AUR-474/AC-002/false-claim: %q must not become a symbol:\n%s", notWant, page)
		}
	}
	// Exactly the three real functions were recognized -- no more, no less.
	symbolCount := strings.Count(page, "### function ")
	if symbolCount != 3 {
		t.Fatalf("AUR-474/AC-002/false-claim: expected exactly 3 symbols (greet, farewell, restart_svc), got %d:\n%s", symbolCount, page)
	}

	t.Logf("AUR-474/AC-001/pass one-liner-forms=3 AC-002/pass false-positives=0")
}

// TestAUR474NoBodyStillRecognized proves an undocumented one-liner (no
// preceding comment) still becomes a real symbol with its real signature
// and zero synthesized prose -- the same AC-003 guarantee AUR-464
// established for the multi-line form, now also true for the one-line form.
func TestAUR474NoBodyStillRecognized(t *testing.T) {
	const script = "#!/bin/bash\nundocumented_task() { echo \"noop\"; }\n"
	page := extractOneAUR474(t, script)
	section := strings.TrimSpace(ownSection(t, page, "### function undocumented_task"))
	if section != "" {
		t.Fatalf("AUR-474/AC-003/false-claim: undocumented one-liner carries synthesized prose: %q", section)
	}
}

// TestAUR474PowerShellAlreadyHandlesOneLiners is the card's explicit
// instruction ("confirmar o mesmo em PowerShell antes de decidir o
// escopo"): powerShellFunctionPattern has no end-of-line anchor, so a
// PowerShell one-line function declaration already produces a real symbol
// today, with no code change required in that extractor. This locks that
// finding in as a regression guard.
func TestAUR474PowerShellAlreadyHandlesOneLiners(t *testing.T) {
	const script = "Set-StrictMode -Version Latest\n\n# Greets a name.\nfunction Get-Greeting { Write-Output \"hi $Name\" }\n"

	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeFileAUR474(t, filepath.Join(srcDir, "pipeline.ps1"), script)

	ext := powershellextractor.NewPowerShellExtractor(site.NewMockRunner())
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguagePowerShell,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil || len(result.Files) == 0 {
		t.Fatalf("AUR-474/powershell/behavior-missing: Extract failed: %v", err)
	}
	data, err := os.ReadFile(result.Files[0])
	if err != nil {
		t.Fatalf("AUR-474/powershell/behavior-missing: read page: %v", err)
	}
	page := string(data)
	heading := "### function Get-Greeting"
	section := ownSection(t, page, heading)
	if !strings.Contains(section, "Greets a name.") {
		t.Fatalf("AUR-474/powershell/behavior-missing: one-line PowerShell function's doc lost:\n%s", page)
	}
}
