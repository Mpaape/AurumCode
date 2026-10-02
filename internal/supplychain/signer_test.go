package supplychain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeCosign creates an executable named "cosign" in a fresh temp
// directory that never touches the network and never calls the real
// signer: it logs its full argv to logPath and then either succeeds
// (writing a plausible bundle file right after the --bundle argument, so
// a caller that checks for the bundle's existence is not fooled by a
// "successful" fake that wrote nothing) or fails, depending on behavior.
func writeFakeCosign(t *testing.T, behavior string) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(binDir, "cosign.log")

	var body string
	switch behavior {
	case "ok":
		// Writes a bundle file (when --bundle names one) BEFORE exiting
		// 0, exactly like a real signer would -- so a test asserting the
		// bundle's content, not just the exit code, has something real
		// to read.
		body = `out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--bundle" ]; then out="$a"; fi
  prev="$a"
done
if [ -n "$out" ]; then
  printf '{"bundle":"fake"}' > "$out"
fi
exit 0
`
	case "fail-after-write":
		// AC-002-MUT-001's own trap: a fake that writes a plausible
		// bundle/signature file and THEN fails, so a caller that only
		// checks "did a signature file appear" (rather than cosign's own
		// exit code) would wrongly call this a success.
		body = `out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--bundle" ]; then out="$a"; fi
  prev="$a"
done
if [ -n "$out" ]; then
  printf '{"bundle":"fake"}' > "$out"
fi
echo "fake-cosign: simulated signing failure" >&2
exit 1
`
	case "fail":
		body = `echo "fake-cosign: simulated failure" >&2
exit 1
`
	case "silent-success":
		// B2's own trap: cosign exits 0 but never writes anything, even
		// though --bundle named a destination. A caller trusting the
		// exit code alone would wrongly call this a success.
		body = `exit 0
`
	default:
		t.Fatalf("unknown fake cosign behavior %q", behavior)
	}

	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"printf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		body

	path := filepath.Join(binDir, "cosign")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake cosign: %v", err)
	}
	return binDir, logPath
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return strings.TrimSpace(string(data))
}

func TestSignBlobInvokesCosignWithKeyAndBundle(t *testing.T) {
	binDir, logPath := writeFakeCosign(t, "ok")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}
	bundlePath := filepath.Join(t.TempDir(), "sbom.bundle")

	err := s.SignBlob(context.Background(), Options{KeyPath: "cosign.key", BundlePath: bundlePath, SkipTransparencyLog: true}, "sbom.json")
	if err != nil {
		t.Fatalf("SignBlob: %v", err)
	}
	line := readLog(t, logPath)
	if !strings.HasPrefix(line, "sign-blob ") {
		t.Fatalf("argv does not start with sign-blob: %q", line)
	}
	for _, want := range []string{"--key cosign.key", "--bundle " + bundlePath, "--tlog-upload=false", "-- sbom.json"} {
		if !strings.Contains(line, want) {
			t.Fatalf("argv %q missing %q", line, want)
		}
	}
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("bundle not written: %v", err)
	}
}

func TestSignBlobFailureNamesArtifact(t *testing.T) {
	binDir, _ := writeFakeCosign(t, "fail")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}

	err := s.SignBlob(context.Background(), Options{KeyPath: "cosign.key"}, "sbom_app_cyclonedx.json")
	if err == nil {
		t.Fatalf("SignBlob: want error, got nil")
	}
	if !strings.Contains(err.Error(), "sbom_app_cyclonedx.json") {
		t.Fatalf("error does not name the failed artifact: %v", err)
	}
}

func TestVerifyBlobRoundTrip(t *testing.T) {
	binDir, _ := writeFakeCosign(t, "ok")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}
	bundlePath := filepath.Join(t.TempDir(), "sbom.bundle")
	if err := s.SignBlob(context.Background(), Options{KeyPath: "cosign.key", BundlePath: bundlePath, SkipTransparencyLog: true}, "sbom.json"); err != nil {
		t.Fatalf("SignBlob: %v", err)
	}
	if err := s.VerifyBlob(context.Background(), Options{KeyPath: "cosign.pub", BundlePath: bundlePath, SkipTransparencyLog: true}, "sbom.json"); err != nil {
		t.Fatalf("VerifyBlob: %v", err)
	}
}

func TestSignImageRejectsUnpinnedReference(t *testing.T) {
	binDir, logPath := writeFakeCosign(t, "ok")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}

	err := s.SignImage(context.Background(), Options{KeyPath: "cosign.key"}, "ghcr.io/org/app:latest")
	if err == nil {
		t.Fatalf("SignImage: want error for an unpinned tag, got nil")
	}
	if !strings.Contains(err.Error(), "ghcr.io/org/app:latest") {
		t.Fatalf("error does not name the rejected reference: %v", err)
	}
	if _, statErr := os.Stat(logPath); statErr == nil {
		t.Fatalf("cosign was invoked despite the unpinned reference")
	}
}

func TestSignImageInvokesCosignForDigestPinnedReference(t *testing.T) {
	binDir, logPath := writeFakeCosign(t, "ok")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}
	ref := "ghcr.io/org/app@sha256:" + strings.Repeat("a", 64)

	if err := s.SignImage(context.Background(), Options{KeyPath: "cosign.key"}, ref); err != nil {
		t.Fatalf("SignImage: %v", err)
	}
	line := readLog(t, logPath)
	if !strings.HasPrefix(line, "sign ") || !strings.Contains(line, ref) {
		t.Fatalf("unexpected cosign argv: %q", line)
	}
}

func TestValidateArtifactRef(t *testing.T) {
	validDigest := strings.Repeat("a", 64)
	cases := []struct {
		name string
		ref  string
		ok   bool
	}{
		{"valid", "ghcr.io/org/app@sha256:" + validDigest, true},
		{"empty", "", false},
		{"tag-only", "ghcr.io/org/app:latest", false},
		{"no-digest-at-all", "ghcr.io/org/app", false},
		{"short-digest", "ghcr.io/org/app@sha256:abc", false},
		{"uppercase-digest", "ghcr.io/org/app@sha256:" + strings.Repeat("A", 64), false},
		{"non-hex-digest", "ghcr.io/org/app@sha256:" + strings.Repeat("g", 64), false},
		{"leading-dash", "-ghcr.io/org/app@sha256:" + validDigest, false},
		{"flag-like", "--certificate-identity=evil@sha256:" + validDigest, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateArtifactRef(tc.ref)
			if tc.ok && err != nil {
				t.Fatalf("ValidateArtifactRef(%q): unexpected error: %v", tc.ref, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("ValidateArtifactRef(%q): want error, got nil", tc.ref)
			}
		})
	}
}

// TestFailAfterWriteFakeIsStillAFailure is this package's own proof that
// the fail-after-write fake behaves as advertised: SignBlob must still
// report an error even though the fake wrote a bundle file first. A
// signing wrapper that mistakes "a bundle file exists" for "cosign
// succeeded" would pass this fake's write but must still fail here on
// cosign's own exit code.
func TestFailAfterWriteFakeIsStillAFailure(t *testing.T) {
	binDir, _ := writeFakeCosign(t, "fail-after-write")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}
	bundlePath := filepath.Join(t.TempDir(), "sbom.bundle")

	err := s.SignBlob(context.Background(), Options{KeyPath: "cosign.key", BundlePath: bundlePath}, "sbom.json")
	if err == nil {
		t.Fatalf("SignBlob: want error even though a bundle file was written, got nil")
	}
	if _, statErr := os.Stat(bundlePath); statErr != nil {
		t.Fatalf("expected the fake to have written a bundle file: %v", statErr)
	}
}

// TestSignBlobFailsWhenCosignSucceedsButWritesNoBundle is B2: an exit 0
// from cosign is not, by itself, enough to call the SBOM signed -- the
// bundle it was asked to write must actually exist, with content.
func TestSignBlobFailsWhenCosignSucceedsButWritesNoBundle(t *testing.T) {
	binDir, _ := writeFakeCosign(t, "silent-success")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}
	bundlePath := filepath.Join(t.TempDir(), "sbom.bundle")

	err := s.SignBlob(context.Background(), Options{KeyPath: "cosign.key", BundlePath: bundlePath}, "sbom_app_cyclonedx.json")
	if err == nil {
		t.Fatalf("SignBlob: want error when cosign exits 0 without writing a bundle, got nil")
	}
	if !strings.Contains(err.Error(), "sbom_app_cyclonedx.json") {
		t.Fatalf("error does not name the artifact: %v", err)
	}
}

// TestSignImageFailsWhenCosignSucceedsButWritesNoBundle is the same B2
// proof for the image path: SignImage always asks cosign for a
// verification bundle (even when the caller names none, using a private
// temp file) specifically to catch this.
func TestSignImageFailsWhenCosignSucceedsButWritesNoBundle(t *testing.T) {
	binDir, _ := writeFakeCosign(t, "silent-success")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}
	ref := "ghcr.io/org/app@sha256:" + strings.Repeat("a", 64)

	err := s.SignImage(context.Background(), Options{KeyPath: "cosign.key"}, ref)
	if err == nil {
		t.Fatalf("SignImage: want error when cosign exits 0 without writing a bundle, got nil")
	}
	if !strings.Contains(err.Error(), ref) {
		t.Fatalf("error does not name the artifact: %v", err)
	}
}

// TestSignBlobRemovesStaleBundleBeforeSigning is B2's other half: a
// bundle already sitting at bundlePath BEFORE this call (left over from
// a previous run, or committed into a PR by accident) must never be
// mistaken for this run's own output. A "silent-success" fake proves it:
// without the pre-removal, the stale file's own non-zero size would let
// the post-sign check wrongly pass.
func TestSignBlobRemovesStaleBundleBeforeSigning(t *testing.T) {
	binDir, _ := writeFakeCosign(t, "silent-success")
	s := Signer{Binary: filepath.Join(binDir, "cosign")}
	bundlePath := filepath.Join(t.TempDir(), "sbom.bundle")
	if err := os.WriteFile(bundlePath, []byte(`{"bundle":"stale-from-a-previous-run"}`), 0o644); err != nil {
		t.Fatalf("seed stale bundle: %v", err)
	}

	err := s.SignBlob(context.Background(), Options{KeyPath: "cosign.key", BundlePath: bundlePath}, "sbom.json")
	if err == nil {
		t.Fatalf("SignBlob: want error -- the stale bundle must not count as this run's own signature, got nil")
	}
}
