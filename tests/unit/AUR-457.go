// Package unit holds AUR-457's Unit-layer proof.
//
// AUR-457 was opened on the hypothesis that `oci-run --card AUR-002` exits 1
// because AUR-424's gomarkdoc -> go/doc switch drifted the legacy
// characterization. Measurement falsified that. The characterization is
// intact; what fails is compiling the subject inside the sealed profile,
// because AUR-002's `read_paths` enumerates the extractor packages file by
// file and never gained the two `native.go` files AUR-427 (9ef5273) added and
// cmd/regenerate-docs/main.go:665,670 register.
//
// This layer is the pure source-level half of that finding, asserted against
// the tree rather than against the spec: the registration sites still exist,
// and the constructors they name are really DEFINED in the two files the stale
// read_paths omits. It deliberately does not read docs/specs/AUR-457.md -- the
// acceptance script covers the spec, and a layer that only checked prose could
// stay green while the code moved underneath it.
//
// Declared-selector note: the card declares `TestAUR446`, which is a template
// typo -- that symbol already exists at tests/unit/AUR-446.go:138 in this same
// package, so redeclaring it would not compile. Delivered as TestAUR457 and
// recorded as a declared gap in docs/specs/AUR-457.md.
package unit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// aur457NativeCtor matches the definition, not a mention. A doc comment naming
// NewNativeExtractor must not satisfy this test.
var aur457NativeCtor = regexp.MustCompile(`(?m)^func NewNativeExtractor\(\) \*NativeExtractor \{`)

// aur457RepoRoot resolves the tree the assertions read. AURUMCODE_ROOT exists
// because the sealed profile makes only this card's `paths` writable: the
// acceptance stages these packages plus a bridge `_test.go` into a scratch
// module under TMPDIR and points them back at the real, read-only checkout.
// Without the override the walk would stop at the scratch module's go.mod and
// assert against an empty tree.
func aur457RepoRoot(t *testing.T) string {
	t.Helper()
	if override := os.Getenv("AURUMCODE_ROOT"); override != "" {
		if info, err := os.Stat(filepath.Join(override, "go.mod")); err == nil && info.Mode().IsRegular() {
			return override
		}
		t.Fatalf("AUR-457: AURUMCODE_ROOT=%s is not a Go module root", override)
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-457: working directory unresolved: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-457: repository root not found above %s", dir)
		}
		dir = parent
	}
}

func aur457Read(t *testing.T, root, rel string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("AUR-457: required source absent: %s: %v", rel, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("AUR-457: required source is not a regular file: %s", rel)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-457: required source unreadable: %s: %v", rel, err)
	}
	if len(data) == 0 {
		t.Fatalf("AUR-457: required source is empty: %s", rel)
	}
	return string(data)
}

func TestAUR457(t *testing.T) {
	root := aur457RepoRoot(t)

	// The subject still registers both native extractors. If AUR-427 were
	// reverted, this falls -- and with it the whole finding, which is correct:
	// the finding would no longer describe the tree.
	subject := aur457Read(t, root, "cmd/regenerate-docs/main.go")
	for _, ctor := range []string{
		"rustExtractor.NewNativeExtractor(",
		"csharpExtractor.NewNativeExtractor(",
	} {
		if !strings.Contains(subject, ctor) {
			t.Errorf("AUR-457: cmd/regenerate-docs/main.go no longer registers %s", ctor)
		}
	}

	// Both constructors are defined in the two files AUR-002's read_paths
	// omits. These are the exact sources whose absence from the sealed
	// materialization produces `undefined: ...NewNativeExtractor`.
	for _, rel := range []string{
		"internal/documentation/extractors/rust/native.go",
		"internal/documentation/extractors/csharp/native.go",
	} {
		src := aur457Read(t, root, rel)
		if !aur457NativeCtor.MatchString(src) {
			t.Errorf("AUR-457: %s does not define func NewNativeExtractor() *NativeExtractor", rel)
		}
	}

	// The legacy characterization is intact: the three summary lines
	// tests/acceptance/AUR-002.sh greps for are still byte-identical in the
	// baseline replays. This is the assertion that would have been red if the
	// inherited "characterization drift" diagnosis had been correct.
	for replay, want := range map[string]string{
		"complete-success.stderr":  "aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true",
		"missing-extractor.stderr": "aurumcode: result=partial docs=1 skipped=1 failed=0 languages_skipped=java output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true",
		"extractor-error.stderr":   "aurumcode: result=partial docs=1 skipped=0 failed=1 languages_skipped=none output=/tmp/aurum-a002-output index_pages=1 index_pages_excluded=0 config=true",
	} {
		got := aur457Read(t, root, filepath.Join("tests/characterization/legacy-baseline", replay))
		found := false
		for _, line := range strings.Split(got, "\n") {
			if line == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AUR-457: baseline replay %s no longer carries its exact summary line; the characterization drifted", replay)
		}
	}
}
