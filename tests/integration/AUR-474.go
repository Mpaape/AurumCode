// Package integration holds AUR-474's Integration-layer proof: the Bash
// extractor's real Extract() entrypoint -- walking a source directory,
// writing real files to disk -- recognizes a one-line function declaration
// (`name() { ... }`) as a real symbol end to end, and the same AC-002
// false-positive shapes produce zero symbols through that same real path.
//
// This file is not named "_test.go" on purpose, mirroring every sibling
// card in this office: tests/acceptance/AUR-474.sh stages a private
// writable copy of the module and writes a tiny bridge "_test.go" file
// that calls IntegrationAUR474, so the assertions below run inside the
// sandboxed acceptance instead of an unrelated top-level `go test ./...`.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	bashextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// bashOneLinerScript mirrors the card's own "Achado medido" fixture, plus
// the AC-002 false-positive shapes, exercised through the real
// Extract()-writes-to-disk path rather than an in-memory scanner call.
const bashOneLinerScript = `#!/bin/bash
set -euo pipefail

# Greets a name.
greet() { echo "hello $1"; }

my-array=()
x=$(cmd)
if [ -f "$1" ]; then echo "found"; fi
echo "foo() { }"
`

func writeFileAUR474Int(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// IntegrationAUR474 is AUR-474's Integration-layer selector.
func IntegrationAUR474(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeFileAUR474Int(t, filepath.Join(srcDir, "pipeline.sh"), bashOneLinerScript)

	ext := bashextractor.NewBashExtractor(site.NewMockRunner())
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageBash,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("AUR-474/AC-001/behavior-missing: Extract failed: %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("AUR-474/AC-001/behavior-missing: expected 1 generated page, got %d", len(result.Files))
	}

	pagePath := filepath.Join(outDir, "pipeline.sh.md")
	data, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("AUR-474/AC-001/behavior-missing: page missing on disk: %v", err)
	}
	page := string(data)

	heading := "### function greet"
	idx := strings.Index(page, heading)
	if idx < 0 {
		t.Fatalf("AUR-474/AC-001/behavior-missing: one-line function greet never reached disk as a symbol:\n%s", page)
	}
	rest := page[idx+len(heading):]
	fenceIdx := strings.Index(rest, "```")
	if fenceIdx < 0 {
		t.Fatalf("AUR-474/AC-001/behavior-missing: no code fence after greet heading")
	}
	if !strings.Contains(rest[:fenceIdx], "Greets a name.") {
		t.Fatalf("AUR-474/AC-001/behavior-missing: greet's own doc lost on disk (likely swept into Script Notes):\n%s", page)
	}

	// AC-001 second half: the doc must not also leak into Script Notes.
	if notesIdx := strings.Index(page, "## Script Notes"); notesIdx >= 0 {
		notesSection := page[notesIdx:idx]
		if strings.Contains(notesSection, "Greets a name.") {
			t.Fatalf("AUR-474/AC-001/false-claim: greet's doc leaked into Script Notes on disk:\n%s", page)
		}
	}

	// AC-002 through the real disk path: none of the false-positive shapes
	// became a symbol.
	for _, notWant := range []string{"my-array", "cmd", "found", "foo"} {
		if strings.Contains(page, "### function "+notWant) {
			t.Fatalf("AUR-474/AC-002/false-claim: %q must not become a symbol on disk:\n%s", notWant, page)
		}
	}
	if got := strings.Count(page, "### function "); got != 1 {
		t.Fatalf("AUR-474/AC-002/false-claim: expected exactly 1 symbol (greet), got %d:\n%s", got, page)
	}

	t.Logf("AUR-474/AC-001/pass integration one-liner-on-disk=1 AC-002/pass false-positives=0")
}
