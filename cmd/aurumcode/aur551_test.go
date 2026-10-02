package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSupplyChainConfig writes root/.aurumcode/config.yml with
// quality_gates.supply_chain, optionally with an artifacts list -- the
// SAME file every other section (review, rules, gate, quality_gates.*)
// already uses.
func writeSupplyChainConfig(t *testing.T, root, engine string, signSBOM, signArtifacts bool, artifacts []string) {
	t.Helper()
	dir := filepath.Join(root, ".aurumcode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	var b strings.Builder
	b.WriteString("quality_gates:\n  supply_chain:\n")
	b.WriteString("    engine: " + engine + "\n")
	b.WriteString("    sign_sbom: " + boolYAML(signSBOM) + "\n")
	b.WriteString("    sign_artifacts: " + boolYAML(signArtifacts) + "\n")
	if len(artifacts) > 0 {
		b.WriteString("    artifacts:\n")
		for _, a := range artifacts {
			b.WriteString("      - \"" + a + "\"\n")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
}

func boolYAML(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// writeFakeCosignCmd creates an executable "cosign" that logs its argv
// and either succeeds (writing a bundle file right after --bundle, when
// present, before exiting 0) or fails -- the same shape
// internal/supplychain's own test fake uses, duplicated here because
// cmd/aurumcode's own tests must not depend on that package's unexported
// test helpers.
func writeFakeCosignCmd(t *testing.T, fail bool) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(binDir, "cosign.log")

	body := `out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--bundle" ]; then out="$a"; fi
  prev="$a"
done
if [ -n "$out" ]; then
  printf '{"bundle":"fake"}' > "$out"
fi
`
	if fail {
		body += "echo 'fake-cosign: simulated failure' >&2\nexit 1\n"
	} else {
		body += "exit 0\n"
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

// TestAUR551NoSectionIsNoOp is AC-003: with no quality_gates.supply_chain
// declared at all, the command exits 0 and never invokes cosign -- "sem
// a secao supply_chain, nada muda". A fake cosign IS on PATH (so a bug
// that invoked it unconditionally would be caught by the log file
// existing), but it must never run.
func TestAUR551NoSectionIsNoOp(t *testing.T) {
	root := t.TempDir()
	binDir, logPath := writeFakeCosignCmd(t, false)

	var stdout, stderr bytes.Buffer
	code := runSign([]string{"--repo", root, "--cosign-bin", filepath.Join(binDir, "cosign")}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatalf("cosign was invoked despite no supply_chain section")
	}
}

// TestAUR551SignsSBOMAndImage is AC-001: with sign_sbom and sign_artifacts
// both on, the command signs the given SBOM file (writing a verifiable
// bundle next to it) and the configured image, in that order.
func TestAUR551SignsSBOMAndImage(t *testing.T) {
	root := t.TempDir()
	image := "ghcr.io/org/app@sha256:" + strings.Repeat("a", 64)
	writeSupplyChainConfig(t, root, "cosign", true, true, []string{image})
	binDir, logPath := writeFakeCosignCmd(t, false)

	sbomPath := filepath.Join(root, "sbom_app_cyclonedx.json")
	if err := os.WriteFile(sbomPath, []byte(`{"bomFormat":"CycloneDX"}`), 0o644); err != nil {
		t.Fatalf("write sbom fixture: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runSign([]string{"--repo", root, "--cosign-bin", filepath.Join(binDir, "cosign"), "--sbom", sbomPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading cosign log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logged)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 cosign invocations (sign-blob, sign), got %d: %q", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "sign-blob ") || !strings.Contains(lines[0], sbomPath) {
		t.Fatalf("unexpected first cosign argv: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "sign ") || !strings.Contains(lines[1], image) {
		t.Fatalf("unexpected second cosign argv: %q", lines[1])
	}

	if _, err := os.Stat(sbomPath + ".sigstore.json"); err != nil {
		t.Fatalf("sbom bundle not written: %v", err)
	}
}

// TestAUR551SignFailureNamesArtifactAndFailsClosed is AC-002: a signing
// failure fails the command (non-zero exit) and names the artifact that
// was left unsigned on stderr -- unconditionally, with no gate.inconclusive
// mode to soften it (unlike aurumcode sbom's own failure path).
func TestAUR551SignFailureNamesArtifactAndFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeSupplyChainConfig(t, root, "cosign", true, false, nil)
	binDir, _ := writeFakeCosignCmd(t, true)

	sbomPath := filepath.Join(root, "sbom_app_cyclonedx.json")
	if err := os.WriteFile(sbomPath, []byte(`{"bomFormat":"CycloneDX"}`), 0o644); err != nil {
		t.Fatalf("write sbom fixture: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runSign([]string{"--repo", root, "--cosign-bin", filepath.Join(binDir, "cosign"), "--sbom", sbomPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit=0, want non-zero on signing failure; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), sbomPath) {
		t.Fatalf("stderr does not name the unsigned artifact %q: %s", sbomPath, stderr.String())
	}
}

// TestAUR551EngineValidation proves the config-level engine check: a
// declared supply_chain section with an engine other than "cosign" is a
// loud configuration error, before any cosign call.
func TestAUR551EngineValidation(t *testing.T) {
	root := t.TempDir()
	writeSupplyChainConfig(t, root, "notcosign", true, false, nil)
	binDir, logPath := writeFakeCosignCmd(t, false)

	var stdout, stderr bytes.Buffer
	code := runSign([]string{"--repo", root, "--cosign-bin", filepath.Join(binDir, "cosign")}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit=0, want non-zero for engine != cosign; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "cosign") {
		t.Fatalf("stderr does not explain the engine error: %s", stderr.String())
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatalf("cosign was invoked despite an invalid engine")
	}
}

// TestAUR551UnpinnedArtifactRejectedAtConfigLoad proves the digest-pinning
// rule at config load: an artifacts entry without a sha256 digest fails
// before any cosign call, naming the offending reference.
func TestAUR551UnpinnedArtifactRejectedAtConfigLoad(t *testing.T) {
	root := t.TempDir()
	writeSupplyChainConfig(t, root, "cosign", false, true, []string{"ghcr.io/org/app:latest"})
	binDir, logPath := writeFakeCosignCmd(t, false)

	var stdout, stderr bytes.Buffer
	code := runSign([]string{"--repo", root, "--cosign-bin", filepath.Join(binDir, "cosign")}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("exit=0, want non-zero for an unpinned artifact; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "ghcr.io/org/app:latest") {
		t.Fatalf("stderr does not name the unpinned reference: %s", stderr.String())
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatalf("cosign was invoked despite an unpinned artifact")
	}
}

// TestAUR551SBOMFallsBackToSBOMGeneratorOutputFile proves the --sbom
// fallback: with sign_sbom on and no --sbom flag given, the command signs
// quality_gates.ssor_dtrack.sbom_generator.output_file instead of
// requiring the caller to repeat a path `aurumcode sbom` already knows.
func TestAUR551SBOMFallsBackToSBOMGeneratorOutputFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".aurumcode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := `quality_gates:
  ssor_dtrack:
    sbom_generator:
      tool: trivy
      format: cyclonedx
      spec_version: "1.6"
      output_file: sbom_app_cyclonedx.json
  supply_chain:
    engine: cosign
    sign_sbom: true
    sign_artifacts: false
`
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	binDir, logPath := writeFakeCosignCmd(t, false)

	sbomPath := filepath.Join(root, "sbom_app_cyclonedx.json")
	if err := os.WriteFile(sbomPath, []byte(`{"bomFormat":"CycloneDX"}`), 0o644); err != nil {
		t.Fatalf("write sbom fixture: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := runSign([]string{"--repo", root, "--cosign-bin", filepath.Join(binDir, "cosign")}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading cosign log: %v", err)
	}
	if !strings.Contains(string(logged), sbomPath) {
		t.Fatalf("cosign was not invoked against the fallback sbom path %q: %q", sbomPath, string(logged))
	}
}
