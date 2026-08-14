// Package integration holds AUR-457's Integration-layer proof.
//
// Where the Unit layer reads sources, this layer exercises the delivered
// acceptance script end to end and checks its exit-code contract -- the part
// of the finding that only shows up when the script actually runs. It is the
// layer that would catch an acceptance which "passes" without asserting: a
// script that always exits 0 fails the typed-error cases here.
//
// Declared-selector note: the card declares `IntegrationAUR446`, a template
// typo -- that symbol already exists at tests/integration/AUR-446.go:121 in
// this same package, so redeclaring it would not compile. Delivered as
// IntegrationAUR457 and recorded as a declared gap in docs/specs/AUR-457.md.
package integration

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur457Root(t *testing.T) string {
	t.Helper()
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

// aur457Run executes the delivered acceptance with one selector and returns
// its raw exit code plus stdout and stderr.
func aur457Run(t *testing.T, root, selector string) (int, string, string) {
	t.Helper()
	script := filepath.Join(root, "tests/acceptance/AUR-457.sh")
	if info, err := os.Lstat(script); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("AUR-457: delivered acceptance absent or not a regular file: %v", err)
	}
	cmd := exec.Command("bash", script, selector)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("AUR-457: infrastructure: acceptance could not be executed: %v", err)
		}
		return exitErr.ExitCode(), stdout.String(), stderr.String()
	}
	return 0, stdout.String(), stderr.String()
}

func IntegrationAUR457(t *testing.T) {
	root := aur457Root(t)

	// Nominal: the acceptance runs its six real claims and exits 0 with the
	// bounded JSON receipt. The claim count is asserted, so silently dropping
	// an assertion is caught here rather than passing unnoticed.
	code, stdout, stderr := aur457Run(t, root, "AC-001")
	if code != 0 {
		t.Fatalf("AUR-457: AC-001 raw exit %d, want 0; stderr=%q", code, strings.TrimSpace(stderr))
	}
	want := `{"card":"AUR-457","scenario":"AC-001","claims":6,"result":"pass"}`
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("AUR-457: AC-001 receipt drift:\n got %s\nwant %s", got, want)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("AUR-457: AC-001 wrote to stderr on the nominal path: %q", strings.TrimSpace(stderr))
	}

	// Determinism: the same input produces the same output, as AC-001's
	// "And" clause requires.
	repeatCode, repeatStdout, _ := aur457Run(t, root, "AC-001")
	if repeatCode != code || repeatStdout != stdout {
		t.Errorf("AUR-457: AC-001 is not deterministic: exit %d/%d, stdout %q/%q",
			code, repeatCode, stdout, repeatStdout)
	}

	// Typed errors: an acceptance that exits 0 unconditionally -- the failure
	// mode this project fights -- cannot satisfy these.
	for _, tc := range []struct {
		selector string
		wantExit int
		wantMark string
	}{
		{"invalid-input", 64, "AUR-457/AC-001/invalid-input"},
		{"boundary-overflow", 65, "AUR-457/AC-001/boundary-overflow"},
		{"no-such-selector", 64, "AUR-457/AC-001/unknown-selector"},
	} {
		gotExit, gotStdout, gotStderr := aur457Run(t, root, tc.selector)
		if gotExit != tc.wantExit {
			t.Errorf("AUR-457: selector %s raw exit %d, want %d", tc.selector, gotExit, tc.wantExit)
		}
		if !strings.Contains(gotStderr, tc.wantMark) {
			t.Errorf("AUR-457: selector %s stderr lacks %s: %q", tc.selector, tc.wantMark, strings.TrimSpace(gotStderr))
		}
		if strings.TrimSpace(gotStdout) != "" {
			t.Errorf("AUR-457: selector %s emitted stdout on an error path: %q", tc.selector, gotStdout)
		}
	}
}

func TestIntegrationAUR457(t *testing.T) { IntegrationAUR457(t) }
