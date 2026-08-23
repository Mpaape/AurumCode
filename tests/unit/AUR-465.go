// Package unit holds AUR-465's Unit-layer proof: the three deterministic
// guardrails internal/documentation/welcome/sanitize.go adds around the
// generated welcome page each do exactly what AC-001/AC-002/AC-003 require,
// checked function-by-function with no LLM, no filesystem walk of the real
// site, and no other package involved.
//
// AC-001 (SanitizeActionRef): every "Mpaape/AurumCode@<ref>" the generated
// index carries must be the published major tag or a full immutable commit
// SHA. A branch -- @main, @master, or any other name an LLM might echo back
// from a consumer's own README -- is a mutable ref: a later upstream push
// would silently change what a copied workflow executes.
//
// AC-002 (DeclaredAssetPath + AssetExists): a _config.yml "logo:" entry is
// compliant only if it is either absent or resolves to a real file under the
// published site root.
//
// AC-003 (SanitizeInternalLinks): the only internal, relative link the
// welcome content may carry is the getting-started guide; any other
// relative path is an invented link nothing in this package can confirm the
// site scaffold will ever produce, so it is neutralized to plain text.
//
// This file is not named "_test.go" on purpose, mirroring every sibling card
// in this office (AUR-402..AUR-461): tests/acceptance/AUR-465.sh stages a
// private copy of the module and bridges TestAUR465 into a real `go test`
// so these assertions run inside the sandboxed acceptance instead of being
// swept into an unrelated top-level `go test ./...`.
package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/welcome"
)

// TestAUR465 is AUR-465's Unit-layer selector.
func TestAUR465(t *testing.T) {
	// -- AC-001: SanitizeActionRef -----------------------------------------
	refCases := []struct {
		name       string
		in         string
		wantRef    string
		wantChange bool
	}{
		{"main branch is rewritten", "uses: Mpaape/AurumCode@main", "v1", true},
		{"master branch is rewritten", "uses: Mpaape/AurumCode@master", "v1", true},
		{"arbitrary branch is rewritten", "uses: Mpaape/AurumCode@feature/docs-poc", "v1", true},
		{"published major tag is untouched", "uses: Mpaape/AurumCode@v1", "v1", false},
		{"published patch tag is untouched", "uses: Mpaape/AurumCode@v1.0.1", "v1.0.1", false},
		{
			"full commit sha is untouched (stricter consumer policy)",
			"uses: Mpaape/AurumCode@11bd71901bbe5b1630ceea73d27597364c9af683",
			"11bd71901bbe5b1630ceea73d27597364c9af683",
			false,
		},
	}
	for _, tc := range refCases {
		t.Run("SanitizeActionRef/"+tc.name, func(t *testing.T) {
			out, changed := welcome.SanitizeActionRef(tc.in)
			if changed != tc.wantChange {
				t.Fatalf("AUR-465/AC-001/behavior-missing: changed = %v, want %v (in=%q out=%q)", changed, tc.wantChange, tc.in, out)
			}
			want := "Mpaape/AurumCode@" + tc.wantRef
			if !strings.Contains(out, want) {
				t.Fatalf("AUR-465/AC-001/behavior-missing: output %q does not contain %q", out, want)
			}
			if strings.Contains(out, "@main") || strings.Contains(out, "@master") {
				t.Fatalf("AUR-465/AC-001/behavior-missing: output %q still carries a mutable branch ref", out)
			}
		})
	}

	// -- AC-003: SanitizeInternalLinks --------------------------------------
	linkIn := "See [the guide](docs/getting-started.md), [Section 1](guides/advanced/), " +
		"[GitHub](https://github.com/Mpaape/AurumCode) and [Top](#quick-start)."
	linkOut, linkChanged := welcome.SanitizeInternalLinks(linkIn)
	if !linkChanged {
		t.Fatalf("AUR-465/AC-003/behavior-missing: SanitizeInternalLinks reported no change for input carrying an invented relative link: %q", linkIn)
	}
	if !strings.Contains(linkOut, "[the guide](docs/getting-started.md)") {
		t.Fatalf("AUR-465/AC-003/behavior-missing: the getting-started guide link must survive untouched, got %q", linkOut)
	}
	if strings.Contains(linkOut, "](guides/advanced/)") {
		t.Fatalf("AUR-465/AC-003/behavior-missing: invented relative link 'guides/advanced/' was not neutralized, got %q", linkOut)
	}
	if !strings.Contains(linkOut, "Section 1") {
		t.Fatalf("AUR-465/AC-003/behavior-missing: neutralizing the invented link must keep its label text, got %q", linkOut)
	}
	if !strings.Contains(linkOut, "[GitHub](https://github.com/Mpaape/AurumCode)") {
		t.Fatalf("AUR-465/AC-003/behavior-missing: an external URL must survive untouched, got %q", linkOut)
	}
	if !strings.Contains(linkOut, "[Top](#quick-start)") {
		t.Fatalf("AUR-465/AC-003/behavior-missing: an in-page anchor must survive untouched, got %q", linkOut)
	}

	// AC-003 adversarial cases: SanitizeInternalLinks must compare the
	// normalized target against the guide path by EXACT EQUALITY, never
	// Contains/HasPrefix/HasSuffix. Each of those lets a differently
	// disguised path escape sanitization even though none of them names a
	// file the generator ever produced.
	disguises := []struct {
		name   string
		target string
	}{
		// Reviewer-supplied: a Contains check matches because the guide
		// path substring appears mid-string, even though the real target
		// walks out of the site via "..".
		{"contains: path traversal after the guide substring", "evil/docs/getting-started.md/../../etc/passwd"},
		// Reviewer-supplied: a Contains check matches because the guide
		// path substring appears mid-string, even though the real file is
		// a different, unrelated one.
		{"contains: unrelated file whose name embeds the guide substring", "notes/docs/getting-started.md-old.md"},
		// A HasPrefix check would wrongly accept this: the target starts
		// with the guide path but names a different file (a backup, here).
		{"prefix escape: guide path with a trailing suffix", "docs/getting-started.md.bak"},
		// A HasSuffix check would wrongly accept this: the target ends with
		// the guide path but lives under a different, attacker-chosen
		// directory.
		{"suffix escape: guide path with a foreign leading directory", "internal/attacker/docs/getting-started.md"},
	}
	for _, tc := range disguises {
		t.Run("SanitizeInternalLinks/"+tc.name, func(t *testing.T) {
			in := "[Disguised](" + tc.target + ")"
			out, changed := welcome.SanitizeInternalLinks(in)
			if !changed {
				t.Fatalf("AUR-465/AC-003/behavior-missing: disguised target %q survived sanitization untouched (changed=false); it does not name a file the generator produced", tc.target)
			}
			if strings.Contains(out, "]("+tc.target+")") {
				t.Fatalf("AUR-465/AC-003/behavior-missing: disguised target %q still appears as a live link in output %q", tc.target, out)
			}
			if !strings.Contains(out, "Disguised") {
				t.Fatalf("AUR-465/AC-003/behavior-missing: neutralizing %q must keep its label text, got %q", tc.target, out)
			}
		})
	}

	// The legitimate variants normalizeLinkTarget exists to keep working: a
	// leading "./" and a trailing "#fragment" anchor on the real guide path
	// must NOT cause the link to be neutralized.
	legitVariants := []string{
		"./docs/getting-started.md",
		"docs/getting-started.md#quick-start",
	}
	for _, target := range legitVariants {
		t.Run("SanitizeInternalLinks/legit-variant/"+target, func(t *testing.T) {
			in := "[Guide](" + target + ")"
			out, changed := welcome.SanitizeInternalLinks(in)
			if changed || !strings.Contains(out, "[Guide]("+target+")") {
				t.Fatalf("AUR-465/AC-003/behavior-missing: legitimate guide variant %q was wrongly neutralized, got %q (changed=%v)", target, out, changed)
			}
		})
	}

	// -- AC-002: DeclaredAssetPath + AssetExists ----------------------------
	if path, declared := welcome.DeclaredAssetPath("title: Docs\ndescription: x\n"); declared {
		t.Fatalf("AUR-465/AC-002/behavior-missing: DeclaredAssetPath found a logo in a config that declares none: %q", path)
	}

	declaredNoQuotes := "title: Docs\nlogo: /assets/images/logo.png\n"
	path, declared := welcome.DeclaredAssetPath(declaredNoQuotes)
	if !declared || path != "/assets/images/logo.png" {
		t.Fatalf("AUR-465/AC-002/behavior-missing: DeclaredAssetPath(%q) = (%q, %v), want (/assets/images/logo.png, true)", declaredNoQuotes, path, declared)
	}

	declaredQuoted := "title: Docs\nlogo: \"/assets/images/logo.png\"\n"
	pathQ, declaredQ := welcome.DeclaredAssetPath(declaredQuoted)
	if !declaredQ || pathQ != "/assets/images/logo.png" {
		t.Fatalf("AUR-465/AC-002/behavior-missing: DeclaredAssetPath did not strip quotes: (%q, %v)", pathQ, declaredQ)
	}

	siteRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(siteRoot, "assets", "images"), 0o755); err != nil {
		t.Fatalf("AUR-465/AC-002/infrastructure: mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(siteRoot, "assets", "images", "logo.png"), []byte("fake-png"), 0o644); err != nil {
		t.Fatalf("AUR-465/AC-002/infrastructure: write fixture asset: %v", err)
	}
	if !welcome.AssetExists("/assets/images/logo.png", siteRoot) {
		t.Fatalf("AUR-465/AC-002/behavior-missing: AssetExists is false for an asset that is actually on disk under %s", siteRoot)
	}
	if welcome.AssetExists("/assets/images/missing.png", siteRoot) {
		t.Fatalf("AUR-465/AC-002/behavior-missing: AssetExists is true for an asset that was never written; this is the exact MUT-002 shape (a declared, nonexistent logo)")
	}
}
