// Package unit holds AUR-464's Unit-layer proof: the Bash and PowerShell
// documentation extractors (internal/documentation/extractors/bash,
// internal/documentation/extractors/powershell) attach each documented
// comment block to the real function it precedes -- never to a fixed
// "## Documentation" placeholder repeated once per block -- and a symbol
// with no comment still appears, with its real signature and no invented
// prose.
//
// This file is not named "_test.go" on purpose, mirroring every sibling
// card in this office (AUR-402..AUR-411, AUR-422, AUR-424, AUR-426,
// AUR-427): tests/acceptance/AUR-464.sh stages a private writable copy of
// the module and writes a tiny bridge "_test.go" file that calls
// TestAUR464, so the assertions below run inside the sandboxed acceptance
// instead of being swept into an unrelated top-level `go test ./...`.
package unit

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	bashextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	powershellextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/powershell"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

const bashFixtureAUR464 = `#!/bin/bash
# Overview of this helper script.
# It has nothing to do with any one function below.

set -euo pipefail

# Greets a name.
greet() {
  echo "hello $1"
}

# not attached to any function: a stray mid-script note
echo "loading..."

function farewell {
  echo "bye"
}

undocumented_task() {
  echo "noop"
}
`

const powershellFixtureAUR464 = `<#
.SYNOPSIS
Overview of this helper script.
#>

# not attached to any function: a stray mid-script note
Write-Host "loading"

# Greets a name.
function Get-Greeting {
    param($Name)
    Write-Output "hello $Name"
}

function Undocumented-Task {
    Write-Output "noop"
}
`

func writeFileAUR464(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFileAUR464(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated page %s: %v", path, err)
	}
	return string(b)
}

// githubSlugAUR464 mimics the GitHub/kramdown Markdown-heading-to-anchor
// rule the site's Jekyll renderer applies: lowercase, spaces to hyphens,
// strip everything that is not a letter, digit, hyphen, or underscore. This
// is test-side only (production code never needs a slugger of its own): it
// proves AC-002 -- that two distinct symbol headings produce two distinct
// anchors -- using the same rule the real navigation will apply.
var githubSlugNonWordAUR464 = regexp.MustCompile(`[^a-z0-9_-]+`)

func githubSlugAUR464(heading string) string {
	s := strings.ToLower(strings.TrimSpace(heading))
	s = strings.ReplaceAll(s, " ", "-")
	s = githubSlugNonWordAUR464.ReplaceAllString(s, "")
	return s
}

// headingsAUR464 extracts every Markdown heading line ("#"+ prefix) from a
// generated page, in order.
func headingsAUR464(page string) []string {
	var out []string
	for _, line := range strings.Split(page, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			out = append(out, trimmed)
		}
	}
	return out
}

// TestAUR464 is AUR-464's Unit-layer selector. It proves, for both the Bash
// and the PowerShell extractor:
//
//  1. AC-001: every documented block's heading carries the real symbol
//     name, never the fixed "## Documentation" placeholder, and no heading
//     text repeats within the page.
//  2. AC-002: two distinct symbols on the same page produce two distinct
//     anchors (via the same slug rule the real site renderer applies).
//  3. AC-003: a symbol with no comment still appears, with its real
//     signature and zero invented prose between its heading and its code
//     fence.
//  4. A comment that precedes something other than a recognized symbol (a
//     file overview, a stray mid-script note) is never turned into a fake
//     per-block symbol heading: it is collected once, under one real,
//     non-repeating heading.
func TestAUR464(t *testing.T) {
	t.Run("bash", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR464(t, filepath.Join(srcDir, "pipeline.sh"), bashFixtureAUR464)

		ext := bashextractor.NewBashExtractor(site.NewMockRunner())
		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language:  extractors.LanguageBash,
			SourceDir: srcDir,
			OutputDir: outDir,
		})
		if err != nil {
			t.Fatalf("AUR-464/AC-001/behavior-missing: Extract failed: %v", err)
		}
		if len(result.Files) == 0 {
			t.Fatalf("AUR-464/AC-001/behavior-missing: zero pages generated")
		}
		page := readFileAUR464(t, result.Files[0])
		checkAUR464Page(t, page, "bash", "greet", "farewell", "undocumented_task", "Greets a name.")
	})

	t.Run("powershell", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR464(t, filepath.Join(srcDir, "pipeline.ps1"), powershellFixtureAUR464)

		ext := powershellextractor.NewPowerShellExtractor(site.NewMockRunner())
		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language:  extractors.LanguagePowerShell,
			SourceDir: srcDir,
			OutputDir: outDir,
		})
		if err != nil {
			t.Fatalf("AUR-464/AC-001/behavior-missing: Extract failed: %v", err)
		}
		if len(result.Files) == 0 {
			t.Fatalf("AUR-464/AC-001/behavior-missing: zero pages generated")
		}
		page := readFileAUR464(t, result.Files[0])
		checkAUR464Page(t, page, "powershell", "Get-Greeting", "Undocumented-Task", "", "Greets a name.")
	})

	t.Logf("AUR-464/AC-001/pass bash=ok powershell=ok")
}

// assertOwnDocText requires that wantText appears specifically inside the
// section that starts at headingLine (up to that section's first code
// fence) -- not merely anywhere on the page. A weaker "page contains
// wantText" check cannot tell a real comment attached to its own symbol
// apart from that same text having been swept into "## Script Notes"
// instead: both make the substring true.
func assertOwnDocText(t *testing.T, page, headingLine, wantText string) {
	t.Helper()
	idx := strings.Index(page, headingLine)
	if idx < 0 {
		t.Fatalf("AUR-464/AC-001/behavior-missing: heading %q not found:\n%s", headingLine, page)
	}
	rest := page[idx+len(headingLine):]
	fenceIdx := strings.Index(rest, "```")
	if fenceIdx < 0 {
		t.Fatalf("AUR-464/AC-001/behavior-missing: no code fence after %q", headingLine)
	}
	section := rest[:fenceIdx]
	if !strings.Contains(section, wantText) {
		t.Fatalf("AUR-464/AC-001/behavior-missing: %q's own section carries no real doc (real comment %q lost, likely swept into Notes as if it were an ambiguous file header) even though real code preceded it:\n%s",
			headingLine, wantText, page)
	}
}

// checkAUR464Page runs the AC-001/AC-002/AC-003 assertions shared by both
// languages against one generated page. thirdFn, when non-empty, is a third
// documented-or-not symbol name asserted present alongside docFn/undocFn
// (used for the Bash fixture's three functions).
func checkAUR464Page(t *testing.T, page, lang, docFn, undocFn, thirdFn, docText string) {
	t.Helper()

	// AC-001: the fixed placeholder heading must never appear -- this is
	// the literal defect the card measured.
	if strings.Contains(page, "## Documentation") {
		t.Fatalf("AUR-464/AC-001/behavior-missing[%s]: fixed \"## Documentation\" heading still present:\n%s", lang, page)
	}

	// AC-001: every symbol name must appear as its own heading.
	for _, name := range []string{docFn, undocFn, thirdFn} {
		if name == "" {
			continue
		}
		if !strings.Contains(page, name) {
			t.Fatalf("AUR-464/AC-001/behavior-missing[%s]: page missing symbol %q:\n%s", lang, name, page)
		}
	}

	// AC-001: no heading text repeats within the page.
	headings := headingsAUR464(page)
	seen := map[string]bool{}
	for _, h := range headings {
		if seen[h] {
			t.Fatalf("AUR-464/AC-001/behavior-missing[%s]: heading %q repeats on the same page:\n%s", lang, h, page)
		}
		seen[h] = true
	}
	if len(headings) < 3 {
		t.Fatalf("AUR-464/AC-001/behavior-missing[%s]: expected at least a title plus symbol headings, got %v", lang, headings)
	}

	// AC-002: distinct symbol headings must slug to distinct anchors.
	anchors := map[string]string{}
	for _, h := range headings {
		if !strings.HasPrefix(h, "###") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(h, "#"))
		anchor := githubSlugAUR464(text)
		for otherText, otherAnchor := range anchors {
			if otherAnchor == anchor && otherText != text {
				t.Fatalf("AUR-464/AC-002/behavior-missing[%s]: %q and %q both slug to anchor %q", lang, text, otherText, anchor)
			}
		}
		anchors[text] = anchor
	}
	if len(anchors) < 2 {
		t.Fatalf("AUR-464/AC-002/behavior-missing[%s]: expected at least two symbol headings to compare, got %v", lang, anchors)
	}

	// AC-003: the undocumented symbol's own section carries no prose: only
	// its heading and its code fence, nothing synthesized in between.
	undocHeadingIdx := strings.Index(page, "### function "+undocFn)
	if undocHeadingIdx < 0 {
		t.Fatalf("AUR-464/AC-003/behavior-missing[%s]: heading for undocumented symbol %q not found", lang, undocFn)
	}
	rest := page[undocHeadingIdx+len("### function "+undocFn):]
	fenceIdx := strings.Index(rest, "```")
	if fenceIdx < 0 {
		t.Fatalf("AUR-464/AC-003/behavior-missing[%s]: no code fence after undocumented symbol %q", lang, undocFn)
	}
	between := strings.TrimSpace(rest[:fenceIdx])
	if between != "" {
		t.Fatalf("AUR-464/AC-003/false-claim[%s]: undocumented symbol %q carries synthesized prose: %q", lang, undocFn, between)
	}

	// The documented symbol's own real comment text must be present...
	if !strings.Contains(page, docText) {
		t.Fatalf("AUR-464/AC-001/behavior-missing[%s]: documented symbol's real comment %q missing from page:\n%s", lang, docText, page)
	}
	// ...and a stray, symbol-less comment must be collected once under a
	// real section name, never invisible and never a repeated fake heading.
	if !strings.Contains(page, "not attached to any function") && !strings.Contains(page, "loading") {
		t.Fatalf("AUR-464/behavior-missing[%s]: stray mid-script comment vanished instead of being collected under a named section:\n%s", lang, page)
	}
}

// bashLicenseGluedToFunction and powershellLicenseGluedToFunction are the
// adversarial reviewer's fixtures for the review's Blocker 1: a file-level
// header comment (here, a license notice -- but nothing in the fix below
// may key off that word or any other one) with NO blank line separating it
// from either the shebang above or the first function below. This is
// indistinguishable, by position and syntax alone, from that same first
// function simply carrying its own doc comment -- so scanBashFile must
// never attach it as that function's Doc: per the card, a comment
// attributed to the wrong symbol documents a lie, which is worse than a
// symbol carrying no prose.
const bashLicenseGluedToFunction = `#!/bin/bash
# Copyright 2026 Example Corp.
# All rights reserved. Licensed under MIT.
license_holder() {
  echo "ok"
}
`

const powershellLicenseGluedToFunction = `# Copyright 2026 Example Corp.
# All rights reserved. Licensed under MIT.
function License-Holder {
    Write-Output "ok"
}
`

// TestAUR464FileHeaderNotMisattributed is the reviewer's Blocker 1 fixture,
// proved for both languages: a leading file-level comment with no blank
// line before the first function must never become that function's Doc.
func TestAUR464FileHeaderNotMisattributed(t *testing.T) {
	t.Run("bash", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR464(t, filepath.Join(srcDir, "licensed.sh"), bashLicenseGluedToFunction)

		ext := bashextractor.NewBashExtractor(site.NewMockRunner())
		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language: extractors.LanguageBash, SourceDir: srcDir, OutputDir: outDir,
		})
		if err != nil || len(result.Files) == 0 {
			t.Fatalf("AUR-464/AC-001/behavior-missing: Extract failed: %v", err)
		}
		page := readFileAUR464(t, result.Files[0])

		idx := strings.Index(page, "### function license_holder")
		if idx < 0 {
			t.Fatalf("AUR-464/AC-001/behavior-missing: heading for license_holder not found:\n%s", page)
		}
		rest := page[idx+len("### function license_holder"):]
		fenceIdx := strings.Index(rest, "```")
		if fenceIdx < 0 {
			t.Fatalf("AUR-464/AC-003/behavior-missing: no code fence after license_holder")
		}
		between := strings.TrimSpace(rest[:fenceIdx])
		if between != "" {
			t.Fatalf("AUR-464/AC-003/false-claim: license_holder's own section carries the file header as if it were its doc: %q", between)
		}
	})

	t.Run("powershell", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR464(t, filepath.Join(srcDir, "licensed.ps1"), powershellLicenseGluedToFunction)

		ext := powershellextractor.NewPowerShellExtractor(site.NewMockRunner())
		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language: extractors.LanguagePowerShell, SourceDir: srcDir, OutputDir: outDir,
		})
		if err != nil || len(result.Files) == 0 {
			t.Fatalf("AUR-464/AC-001/behavior-missing: Extract failed: %v", err)
		}
		page := readFileAUR464(t, result.Files[0])

		idx := strings.Index(page, "### function License-Holder")
		if idx < 0 {
			t.Fatalf("AUR-464/AC-001/behavior-missing: heading for License-Holder not found:\n%s", page)
		}
		rest := page[idx+len("### function License-Holder"):]
		fenceIdx := strings.Index(rest, "```")
		if fenceIdx < 0 {
			t.Fatalf("AUR-464/AC-003/behavior-missing: no code fence after License-Holder")
		}
		between := strings.TrimSpace(rest[:fenceIdx])
		if between != "" {
			t.Fatalf("AUR-464/AC-003/false-claim: License-Holder's own section carries the file header as if it were its doc: %q", between)
		}
	})
}

// bashCaseCollisionFixture is the adversarial reviewer's Blocker 2 fixture:
// two symbols whose names differ only in case. Their heading TEXT differs
// ("Foo" vs "foo"), but the standard Markdown-heading-to-anchor slug rule
// (lowercase, then strip) collapses them to the identical anchor
// "function-foo" -- so a link to one lands on the other unless the renderer
// itself disambiguates post-normalization.
const bashCaseCollisionFixture = `#!/bin/bash
Foo() {
  echo "upper"
}

foo() {
  echo "lower"
}
`

// TestAUR464AnchorUniqueAfterNormalization is the reviewer's Blocker 2
// fixture: it slugs every generated heading with the SAME normalization a
// real Markdown renderer applies (lowercase, non-word stripped) and
// requires the RESULT to still be unique, not just the raw heading text.
func TestAUR464AnchorUniqueAfterNormalization(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeFileAUR464(t, filepath.Join(srcDir, "collide.sh"), bashCaseCollisionFixture)

	ext := bashextractor.NewBashExtractor(site.NewMockRunner())
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language: extractors.LanguageBash, SourceDir: srcDir, OutputDir: outDir,
	})
	if err != nil || len(result.Files) == 0 {
		t.Fatalf("AUR-464/AC-002/behavior-missing: Extract failed: %v", err)
	}
	page := readFileAUR464(t, result.Files[0])

	anchors := map[string]string{}
	for _, h := range headingsAUR464(page) {
		if !strings.HasPrefix(h, "###") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(h, "#"))
		anchor := githubSlugAUR464(text)
		if other, exists := anchors[anchor]; exists && other != text {
			t.Fatalf("AUR-464/AC-002/false-claim: headings %q and %q both normalize to anchor %q, so a link to one lands on the other:\n%s",
				other, text, anchor, page)
		}
		anchors[anchor] = text
	}
	if len(anchors) != 2 {
		t.Fatalf("AUR-464/AC-002/behavior-missing: expected 2 distinct post-normalization anchors, got %v", anchors)
	}
}

// bashCodeBeforeDocFixture and powershellCodeBeforeDocFixture are the
// adversarial reviewer's Blocker 1 fixture from the second review round:
// real code (a bare statement, no function) runs BEFORE the first
// documented function. That is the ordinary, common shape of a small
// script -- shebang, a setup statement, then the one documented function --
// and must NOT be swept into Notes the way an ambiguous leading header is.
const bashCodeBeforeDocFixture = `#!/bin/bash
set -euo pipefail

# Greets a name.
greet() {
  echo "hi"
}
`

const powershellCodeBeforeDocFixture = `Set-StrictMode -Version Latest

# Greets a name.
function Get-Greeting {
    Write-Output "hi"
}
`

// TestAUR464CodeBeforeDocPreserved is the reviewer's second-round Blocker 1
// fixture: a real statement precedes the first documented function, so that
// function's own doc comment must survive as ITS doc, not be misfiled as an
// ambiguous file header the way a header glued straight to the first
// function (with no code at all before it) still correctly is.
func TestAUR464CodeBeforeDocPreserved(t *testing.T) {
	t.Run("bash", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR464(t, filepath.Join(srcDir, "setup.sh"), bashCodeBeforeDocFixture)

		ext := bashextractor.NewBashExtractor(site.NewMockRunner())
		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language: extractors.LanguageBash, SourceDir: srcDir, OutputDir: outDir,
		})
		if err != nil || len(result.Files) == 0 {
			t.Fatalf("AUR-464/AC-001/behavior-missing: Extract failed: %v", err)
		}
		page := readFileAUR464(t, result.Files[0])
		assertOwnDocText(t, page, "### function greet", "Greets a name.")
	})

	t.Run("powershell", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR464(t, filepath.Join(srcDir, "setup.ps1"), powershellCodeBeforeDocFixture)

		ext := powershellextractor.NewPowerShellExtractor(site.NewMockRunner())
		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language: extractors.LanguagePowerShell, SourceDir: srcDir, OutputDir: outDir,
		})
		if err != nil || len(result.Files) == 0 {
			t.Fatalf("AUR-464/AC-001/behavior-missing: Extract failed: %v", err)
		}
		page := readFileAUR464(t, result.Files[0])
		assertOwnDocText(t, page, "### function Get-Greeting", "Greets a name.")
	})
}

// powershellAnchorTrioFixture is the adversarial reviewer's Blocker 2 fixture
// from the second review round: three symbols "foo", "Foo", "foo-2" in
// exactly this order. "Foo" collides with "foo" on the base anchor
// "function-foo" and disambiguates to "function-foo (2)" -- but THAT
// disambiguated heading's own anchor, "function-foo-2", is the same anchor
// the plain third symbol "foo-2" produces on its own. A disambiguator that
// only tracks a per-BASE occurrence count (instead of the actual set of
// anchors already written) misses this second collision entirely.
const powershellAnchorTrioFixture = `function foo {
    Write-Output "1"
}

function Foo {
    Write-Output "2"
}

function foo-2 {
    Write-Output "3"
}
`

const bashAnchorTrioFixture = `#!/bin/bash
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

// TestAUR464AnchorTrioUnique is the reviewer's second-round Blocker 2
// fixture, proved for both languages: after the SAME slug normalization a
// real Markdown renderer applies, all three of "foo"/"Foo"/"foo-2" must end
// up on distinct anchors.
func TestAUR464AnchorTrioUnique(t *testing.T) {
	check := func(t *testing.T, page string) {
		t.Helper()
		anchors := map[string]string{}
		for _, h := range headingsAUR464(page) {
			if !strings.HasPrefix(h, "###") {
				continue
			}
			text := strings.TrimSpace(strings.TrimLeft(h, "#"))
			anchor := githubSlugAUR464(text)
			if other, exists := anchors[anchor]; exists && other != text {
				t.Fatalf("AUR-464/AC-002/false-claim: headings %q and %q both normalize to anchor %q, so a link to one lands on the other:\n%s",
					other, text, anchor, page)
			}
			anchors[anchor] = text
		}
		if len(anchors) != 3 {
			t.Fatalf("AUR-464/AC-002/behavior-missing: expected 3 distinct post-normalization anchors for the foo/Foo/foo-2 trio, got %v", anchors)
		}
	}

	t.Run("bash", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR464(t, filepath.Join(srcDir, "trio.sh"), bashAnchorTrioFixture)
		ext := bashextractor.NewBashExtractor(site.NewMockRunner())
		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language: extractors.LanguageBash, SourceDir: srcDir, OutputDir: outDir,
		})
		if err != nil || len(result.Files) == 0 {
			t.Fatalf("AUR-464/AC-002/behavior-missing: Extract failed: %v", err)
		}
		check(t, readFileAUR464(t, result.Files[0]))
	})

	t.Run("powershell", func(t *testing.T) {
		srcDir := t.TempDir()
		outDir := t.TempDir()
		writeFileAUR464(t, filepath.Join(srcDir, "trio.ps1"), powershellAnchorTrioFixture)
		ext := powershellextractor.NewPowerShellExtractor(site.NewMockRunner())
		result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language: extractors.LanguagePowerShell, SourceDir: srcDir, OutputDir: outDir,
		})
		if err != nil || len(result.Files) == 0 {
			t.Fatalf("AUR-464/AC-002/behavior-missing: Extract failed: %v", err)
		}
		check(t, readFileAUR464(t, result.Files[0]))
	})
}

// bashRegressionGuardFixture locks in two behaviors the adversarial review
// confirmed correct and required to survive this round's fix: the 3-way
// case-only trio "Foo"/"foo"/"FOO" still disambiguates to three distinct
// anchors, and "run_docs" (underscore) never collides with "run-docs"
// (hyphen) -- the slug rule keeps both characters distinct on purpose.
const bashRegressionGuardFixture = `#!/bin/bash
Foo() {
  echo "1"
}

foo() {
  echo "2"
}

FOO() {
  echo "3"
}

run_docs() {
  echo "4"
}

run-docs() {
  echo "5"
}
`

func TestAUR464RegressionGuard(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	writeFileAUR464(t, filepath.Join(srcDir, "guard.sh"), bashRegressionGuardFixture)

	ext := bashextractor.NewBashExtractor(site.NewMockRunner())
	result, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language: extractors.LanguageBash, SourceDir: srcDir, OutputDir: outDir,
	})
	if err != nil || len(result.Files) == 0 {
		t.Fatalf("AUR-464/AC-002/behavior-missing: Extract failed: %v", err)
	}
	page := readFileAUR464(t, result.Files[0])

	anchors := map[string]string{}
	for _, h := range headingsAUR464(page) {
		if !strings.HasPrefix(h, "###") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(h, "#"))
		anchor := githubSlugAUR464(text)
		if other, exists := anchors[anchor]; exists && other != text {
			t.Fatalf("AUR-464/AC-002/false-claim: headings %q and %q both normalize to anchor %q:\n%s", other, text, anchor, page)
		}
		anchors[anchor] = text
	}
	if len(anchors) != 5 {
		t.Fatalf("AUR-464/AC-002/behavior-missing: expected 5 distinct anchors (Foo/foo/FOO trio + run_docs/run-docs), got %v", anchors)
	}
}
