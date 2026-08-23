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
	"regexp"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	bashextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	"github.com/Mpaape/AurumCode/internal/documentation/normalizer"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// anchorNonWordAUR464 mirrors the Markdown-heading-to-anchor slug rule the
// site's Jekyll/kramdown renderer applies: used here only to verify, at the
// Integration layer, that the anchors this card's renderer produces are
// genuinely unique after that same normalization.
var anchorNonWordAUR464 = regexp.MustCompile(`[^a-z0-9_-]+`)

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

// bashScriptC is the second review round's Blocker 1 fixture at the
// Integration layer: real code ("set -euo pipefail") runs before the one
// documented function, so that function's real doc must survive as ITS
// doc through the full Extract()-writes-to-disk path, not just the
// in-memory scanner the Unit layer already covers.
const bashScriptC = `#!/bin/bash
set -euo pipefail

# Restarts the whole stack.
restart_all() {
  systemctl restart svc
}
`

// bashScriptD is the second review round's Blocker 2 fixture at the
// Integration layer: the exact "foo"/"Foo"/"foo-2" trio that collides twice
// over (once on the base anchor, once on the first disambiguated suffix),
// proved through the real Extract()-writes-to-disk path.
const bashScriptD = `#!/bin/bash
foo() {
  echo "1"
}

Foo() {
  echo "2"
}

foo-2() {
  echo "3"
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
	writeFileAUR464Int(t, filepath.Join(srcDir, "c.sh"), bashScriptC)
	writeFileAUR464Int(t, filepath.Join(srcDir, "d.sh"), bashScriptD)

	ext := bashextractor.NewBashExtractor(site.NewMockRunner())
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageBash,
		SourceDir: srcDir,
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("AUR-464/AC-001/behavior-missing: Extract failed: %v", err)
	}
	if len(result.Files) != 4 {
		t.Fatalf("AUR-464/AC-001/behavior-missing: expected 4 generated pages, got %d (%v)", len(result.Files), result.Files)
	}

	// Second review round's Blocker 1, through the real Extract()-to-disk
	// path: real code before the function's doc comment must not sweep
	// that doc into Notes.
	cPage, err := os.ReadFile(filepath.Join(outDir, "c.sh.md"))
	if err != nil {
		t.Fatalf("AUR-464/AC-001/behavior-missing: c.sh.md missing on disk: %v", err)
	}
	if idx := strings.Index(string(cPage), "### function restart_all"); idx < 0 {
		t.Fatalf("AUR-464/AC-001/behavior-missing: c.sh.md missing restart_all heading:\n%s", cPage)
	} else if fence := strings.Index(string(cPage)[idx:], "```"); fence < 0 ||
		!strings.Contains(string(cPage)[idx:idx+fence], "Restarts the whole stack.") {
		t.Fatalf("AUR-464/AC-001/behavior-missing: restart_all's real doc was lost (code preceded it, so it must NOT be swept into Notes):\n%s", cPage)
	}

	// Second review round's Blocker 2, through the real Extract()-to-disk
	// path: the foo/Foo/foo-2 trio must end up on three distinct anchors,
	// not just three distinct heading TEXTS.
	dPage, err := os.ReadFile(filepath.Join(outDir, "d.sh.md"))
	if err != nil {
		t.Fatalf("AUR-464/AC-002/behavior-missing: d.sh.md missing on disk: %v", err)
	}
	dAnchors := map[string]string{}
	for _, line := range strings.Split(string(dPage), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "###") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		anchor := strings.ToLower(text)
		anchor = strings.ReplaceAll(anchor, " ", "-")
		anchor = anchorNonWordAUR464.ReplaceAllString(anchor, "")
		if other, exists := dAnchors[anchor]; exists && other != text {
			t.Fatalf("AUR-464/AC-002/false-claim: %q and %q both normalize to anchor %q in d.sh.md:\n%s", other, text, anchor, dPage)
		}
		dAnchors[anchor] = text
	}
	if len(dAnchors) != 3 {
		t.Fatalf("AUR-464/AC-002/behavior-missing: expected 3 distinct anchors for the foo/Foo/foo-2 trio in d.sh.md, got %v", dAnchors)
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
	if processed != 4 {
		t.Fatalf("AUR-464/behavior-missing: expected normalizer to process 4 pages, got %d", processed)
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

	t.Logf("AUR-464/AC-001/pass integration bash-pages=4 normalized=%d", processed)
}
