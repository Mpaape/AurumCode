package sbom

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOutputPathPlainRelative(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveOutputPath(root, "nested/sbom_app_cyclonedx.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rootResolved, err := resolveExistingSymlinks(root)
	if err != nil {
		t.Fatalf("resolving root: %v", err)
	}
	want := filepath.Join(rootResolved, "nested", "sbom_app_cyclonedx.json")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveOutputPathRejectsAbsolute(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveOutputPath(root, "/etc/passwd"); err == nil {
		t.Fatalf("expected an error for an absolute output_file")
	}
}

func TestResolveOutputPathRejectsDotDotEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveOutputPath(root, "../escape.json"); err == nil {
		t.Fatalf("expected an error for a .. escape")
	}
}

// TestResolveOutputPathRejectsSymlinkEscape is the fix this round added:
// a plain, literal "../"-free output_file whose DIRECTORY is a symlink
// pointing outside root must still be refused -- filepath.Clean alone
// cannot see through the symlink, only EvalSymlinks can.
func TestResolveOutputPathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	linkPath := filepath.Join(root, "escape")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlinks not supported on this filesystem: %v", err)
	}
	if _, err := ResolveOutputPath(root, "escape/sbom.json"); err == nil {
		t.Fatalf("expected an error for output_file whose directory symlinks outside root")
	}
}

// TestResolveOutputPathAllowsSymlinkInsideRoot confirms the symlink check
// is about escaping ROOT, not about symlinks in general: a symlinked
// subdirectory that itself still resolves inside root is fine.
func TestResolveOutputPathAllowsSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	linkPath := filepath.Join(root, "link")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Skipf("symlinks not supported on this filesystem: %v", err)
	}
	got, err := ResolveOutputPath(root, "link/sbom.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	realResolved, err := resolveExistingSymlinks(realDir)
	if err != nil {
		t.Fatalf("resolving real dir: %v", err)
	}
	want := filepath.Join(realResolved, "sbom.json")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestResolveOutputPathNotYetExistingDirectory confirms a brand-new,
// not-yet-created output directory (the common first-run case) is still
// accepted -- only the EXISTING prefix needs to resolve through symlinks.
func TestResolveOutputPathNotYetExistingDirectory(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveOutputPath(root, "a/b/c/sbom.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rootResolved, err := resolveExistingSymlinks(root)
	if err != nil {
		t.Fatalf("resolving root: %v", err)
	}
	want := filepath.Join(rootResolved, "a", "b", "c", "sbom.json")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
