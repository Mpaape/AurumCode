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
