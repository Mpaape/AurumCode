// Package integration holds AUR-464's Integration-layer proof: the Bash and
// PowerShell extractors' real Extract() entrypoint -- walking a source
// directory, writing real files to disk -- feeds pages into
// internal/documentation/normalizer (this card's third owned package)
// without either package reintroducing the fixed "## Documentation"
// placeholder or losing a symbol heading along the way.
//
// This file is not named "_test.go" on purpose, mirroring every sibling
// card in this office: tests/acceptance/AUR-464.sh stages a private
// writable copy of the module and writes a tiny bridge "_test.go" file
// that calls IntegrationAUR464, so the assertions below run inside the
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
	"github.com/Mpaape/AurumCode/internal/documentation/normalizer"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

const bashScriptA = `#!/bin/bash
# Builds the project.
build() {
  make all
}

# Cleans build artifacts.
clean() {
  rm -rf build/
}
`

const bashScriptB = `#!/bin/bash
# Deploys the project.
deploy() {
  ./ship.sh
}

restart_service() {
  systemctl restart svc
}
`

func writeFileAUR464Int(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// IntegrationAUR464 is AUR-464's Integration-layer selector. It proves:
//
//  1. Extract() itself -- not just the internal scanner -- walks a
//     directory of real scripts and writes one real page per script to
//     disk, each with symbol-named headings.
//  2. Across every generated page, "## Documentation" never appears and no
//     heading repeats WITHIN a page (AC-001 holds file-by-file, not just
//     for a single hand-picked fixture).
//  3. Running this card's own normalizer.Normalizer over the generated
//     pages (front matter injection) does not disturb the body headings:
//     the two owned packages compose without the front-matter pass
//     re-introducing a placeholder or corrupting a symbol heading.
func IntegrationAUR464(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeFileAUR464Int(t, filepath.Join(srcDir, "a.sh"), bashScriptA)
	writeFileAUR464Int(t, filepath.Join(srcDir, "b.sh"), bashScriptB)

	ext := bashextractor.NewBashExtractor(site.NewMockRunner())
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageBash,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("AUR-464/AC-001/behavior-missing: Extract failed: %v", err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("AUR-464/AC-001/behavior-missing: expected 2 generated pages, got %d (%v)", len(result.Files), result.Files)
	}

	wantSymbols := map[string][]string{
		filepath.Join(outDir, "a.sh.md"): {"build", "clean"},
		filepath.Join(outDir, "b.sh.md"): {"deploy", "restart_service"},
	}

	for path, symbols := range wantSymbols {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("AUR-464/AC-001/behavior-missing: expected page %s missing on disk: %v", path, err)
		}
		page := string(data)

		if strings.Contains(page, "## Documentation") {
			t.Fatalf("AUR-464/AC-001/behavior-missing: %s still carries the fixed placeholder heading:\n%s", path, page)
		}

		var headings []string
		for _, line := range strings.Split(page, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				if headings != nil {
					for _, prev := range headings {
						if prev == trimmed {
							t.Fatalf("AUR-464/AC-001/behavior-missing: heading %q repeats in %s", trimmed, path)
						}
					}
				}
				headings = append(headings, trimmed)
			}
		}

		for _, sym := range symbols {
			if !strings.Contains(page, "### function "+sym) {
				t.Fatalf("AUR-464/AC-001/behavior-missing: %s missing symbol heading for %q:\n%s", path, sym, page)
			}
		}
	}

	// Compose with this card's own normalizer package: front matter goes
	// on top, the body (and its headings) must survive untouched.
	n := normalizer.NewNormalizer(outDir)
	processed, errs := n.NormalizeDir(outDir)
	if len(errs) > 0 {
		t.Fatalf("AUR-464/behavior-missing: normalizer errors: %v", errs)
	}
	if processed != 2 {
		t.Fatalf("AUR-464/behavior-missing: expected normalizer to process 2 pages, got %d", processed)
	}

	for path, symbols := range wantSymbols {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("AUR-464/behavior-missing: normalized page %s unreadable: %v", path, err)
		}
		page := string(data)
		if !strings.HasPrefix(page, "---\n") {
			t.Fatalf("AUR-464/behavior-missing: %s missing front matter after normalization", path)
		}
		if strings.Contains(page, "## Documentation") {
			t.Fatalf("AUR-464/AC-001/behavior-missing: normalization reintroduced the placeholder heading in %s:\n%s", path, page)
		}
		for _, sym := range symbols {
			if !strings.Contains(page, "### function "+sym) {
				t.Fatalf("AUR-464/AC-001/behavior-missing: normalization lost symbol heading %q in %s:\n%s", sym, path, page)
			}
		}
	}

	t.Logf("AUR-464/AC-001/pass integration bash-pages=2 normalized=%d", processed)
}
