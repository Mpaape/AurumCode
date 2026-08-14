// Package unit holds AUR-424's Unit-layer proof: the standard-library Go
// documentation extractor (internal/documentation/extractors/go) parses real
// Go source, renders real symbols and their real doc comments, and never
// invokes an external tool to do it.
//
// This file is not named "_test.go" on purpose, mirroring every sibling card
// in this office (AUR-402..AUR-411, AUR-422): tests/acceptance/AUR-424.sh
// stages a private writable copy of the module and writes a tiny bridge
// "_test.go" file that calls TestAUR424, so the assertions below run inside
// the sandboxed acceptance instead of being swept into an unrelated top-level
// `go test ./...`.
package unit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	goextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/go"
)

// countingRunnerAUR424 satisfies whatever minimal "has a Run method" interface
// the extractor constructor declares (Go interface satisfaction is
// structural, so this package never needs to import the exact type the
// constructor names). It always fails, loudly, so a call that reached it would
// turn into a visible test failure rather than a quiet no-op.
type countingRunnerAUR424 struct {
	calls int
}

func (r *countingRunnerAUR424) Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
	r.calls++
	return "", fmt.Errorf("AUR-424/AC-001/MUT-001: unexpected external tool invocation: %s %v", cmd, args)
}

// fixtureSourceAUR424 is a small, self-contained Go source file: one exported
// type with an exported field, and one exported function, each with a real Go
// doc comment. TestAUR424 also exercises the checked-in fixture at
// tests/fixtures/docs/goproject (see IntegrationAUR424); this in-memory
// fixture keeps the Unit layer independent of that file so a change to one
// cannot silently blind the other.
const fixtureSourceAUR424 = `package sample

// Widget documents a small exported type used only by this test.
type Widget struct {
	// Label is the widget's human-readable name.
	Label string
}

// MakeWidget builds a Widget with the given label.
func MakeWidget(label string) Widget {
	return Widget{Label: label}
}
`

func writeFixtureAUR424(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "widget.go"), []byte(fixtureSourceAUR424), 0o644); err != nil {
		t.Fatalf("write fixture source: %v", err)
	}
}

func readFileAUR424(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated page %s: %v", path, err)
	}
	return string(b)
}

// TestAUR424 is AUR-424's Unit-layer selector. It proves the two halves of the
// card's outcome at the extractor package's own public API:
//
//  1. Feeding the extractor a real Go source file produces a real Markdown
//     page containing the real exported symbols and their real doc comments
//     (not an empty file, not a placeholder).
//  2. No call ever reaches the caller-supplied runner: the standard-library
//     rewrite has nothing to shell out to.
//
// Before the fix, step 1 failed: the old gomarkdoc-backed extractor called
// countingRunnerAUR424.Run("gomarkdoc", ...), which errors here exactly the
// way a genuinely missing gomarkdoc binary errors in the sandbox, and the
// package was skipped with zero generated docs -- the AUR-424/AC-001/
// behavior-missing label below is what a RED run before the fix (or a
// MUT-001 run that reintroduces the gomarkdoc call) prints.
func TestAUR424(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeFixtureAUR424(t, srcDir)

	runner := &countingRunnerAUR424{}
	extractor := goextractor.NewGoExtractor(runner)

	result, err := extractor.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("AUR-424/AC-001/behavior-missing: Extract failed: %v", err)
	}
	if result.Stats.DocsGenerated == 0 || len(result.Files) == 0 {
		t.Fatalf("AUR-424/AC-001/behavior-missing: zero Go pages generated (processed=%d generated=%d errors=%v)",
			result.Stats.FilesProcessed, result.Stats.DocsGenerated, result.Errors)
	}

	page := readFileAUR424(t, result.Files[0])
	for _, want := range []string{
		"sample", "Widget", "Widget documents a small exported type",
		"Label", "func MakeWidget", "MakeWidget builds a Widget with the given label",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("AUR-424/AC-001/behavior-missing: generated page missing %q:\n%s", want, page)
		}
	}

	if runner.calls != 0 {
		t.Fatalf("AUR-424/AC-001/MUT-001: extraction invoked an external tool %d time(s)", runner.calls)
	}

	// Validate must not depend on anything external either: a runner that
	// always errors must not change the verdict. This is the exact assertion
	// that would have caught the original defect, where Validate looked up
	// the gomarkdoc binary and the pipeline silently skipped Go on failure.
	if err := extractor.Validate(context.Background()); err != nil {
		t.Fatalf("AUR-424/AC-001/behavior-missing: Validate depends on something external: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("AUR-424/AC-001/MUT-001: Validate invoked an external tool %d time(s)", runner.calls)
	}

	t.Logf("AUR-424/AC-001/pass docs_generated=%d files=%d external_calls=%d",
		result.Stats.DocsGenerated, len(result.Files), runner.calls)
}
