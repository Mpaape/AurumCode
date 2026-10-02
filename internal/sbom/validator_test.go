package sbom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateEmptyFileRejected is a direct test of Validate's empty-file
// branch, with no trivy (fake or real) involved at all: a zero-byte file
// at path must never be read as a valid SBOM. Asserting the "is empty"
// message (not just "any error") proves THIS branch fired, rather than,
// say, the JSON-unmarshal branch also rejecting an empty byte slice for
// its own, different reason.
func TestValidateEmptyFileRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sbom.json")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	err := Validate(path, "1.6")
	if err == nil {
		t.Fatalf("expected an error for an empty file, got nil")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("error does not name the empty-file branch: %v", err)
	}
}

// TestValidateWhitespaceOnlyFileRejected: a file containing only
// whitespace is, for this purpose, still empty.
func TestValidateWhitespaceOnlyFileRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sbom.json")
	if err := os.WriteFile(path, []byte("   \n\t  "), 0o644); err != nil {
		t.Fatalf("write whitespace file: %v", err)
	}
	if err := Validate(path, "1.6"); err == nil {
		t.Fatalf("expected an error for a whitespace-only file, got nil")
	}
}

// TestValidateMissingFileRejected: Validate must not treat "file does not
// exist" as anything other than an error.
func TestValidateMissingFileRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	if err := Validate(path, "1.6"); err == nil {
		t.Fatalf("expected an error for a missing file, got nil")
	}
}

// TestSpecVersionAtLeast is card v3's own semantics: spec_version in
// config is a MINIMUM, same major, minor at or above the configured one.
func TestSpecVersionAtLeast(t *testing.T) {
	cases := []struct {
		got, want string
		ok        bool
	}{
		{"1.6", "1.6", true},
		{"1.7", "1.6", true},  // the pinned Trivy's real output
		{"1.10", "1.6", true}, // numeric compare, not string compare
		{"1.5", "1.6", false}, // older minor, same major: still rejected
		{"2.0", "1.6", false}, // different major, minor also below floor
		{"2.7", "1.6", false}, // different major, but minor (7) ALONE would
		// pass the minor rule (7 >= 6): a mutation that drops the
		// gotMajor == wantMajor half of specVersionAtLeast survives
		// against "2.0" but must still go red here.
		{"0.9", "0.6", true}, // same major (0), minor at least as new: a
		// low/zero major must not be treated as a special case.
		{"1.6.0", "1.6", false},
		{"1.6", "1.6.0", false},
		{"", "1.6", false},
		{"1.6", "", false},
		{"v1.6", "1.6", false},
		{"1.a", "1.6", false},
		{"1.-1", "1.6", false},
		{"01.6", "1.6", false}, // leading zero: not a strict decimal
		{"1.06", "1.6", false},
	}
	for _, c := range cases {
		got := specVersionAtLeast(c.got, c.want)
		if got != c.ok {
			t.Errorf("specVersionAtLeast(%q, %q) = %v, want %v", c.got, c.want, got, c.ok)
		}
	}
}
