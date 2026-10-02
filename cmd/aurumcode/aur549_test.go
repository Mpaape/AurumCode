package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes root/.aurumcode/config.yml -- the SAME file every
// other section (review, rules, ignore, gate, exceptions) already uses --
// with this card's quality_gates.ssor_dtrack.sbom_generator section, and,
// when inconclusive is non-blank, a gate.inconclusive section alongside
// it in the SAME file (AUR-549 v2: no separate quality_gates.yml).
func writeConfig(t *testing.T, root, inconclusive string) {
	t.Helper()
	dir := filepath.Join(root, ".aurumcode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	var b strings.Builder
	b.WriteString("quality_gates:\n")
	b.WriteString("  ssor_dtrack:\n")
	b.WriteString("    sbom_generator:\n")
	b.WriteString("      tool: trivy\n")
	b.WriteString("      format: cyclonedx\n")
	b.WriteString("      spec_version: \"1.6\"\n")
	b.WriteString("      output_file: sbom_app_cyclonedx.json\n")
	if strings.TrimSpace(inconclusive) != "" {
		b.WriteString("gate:\n  inconclusive: " + inconclusive + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
}

// writeSBOMConfig writes only the sbom_generator section, no gate.
func writeSBOMConfig(t *testing.T, root string) {
	t.Helper()
	writeConfig(t, root, "")
}

// fakeTrivyBehavior selects what the fake trivy script writes to the path
// given after --output.
type fakeTrivyBehavior string

const (
	behaviorValid       fakeTrivyBehavior = "valid"
	behaviorBadVersion  fakeTrivyBehavior = "badversion"
	behaviorWrongFormat fakeTrivyBehavior = "wrongformat"
	behaviorNotJSON     fakeTrivyBehavior = "notjson"
	behaviorEmpty       fakeTrivyBehavior = "empty"
	behaviorFail        fakeTrivyBehavior = "fail"
)

// writeFakeTrivy creates an executable named "trivy" in a fresh temp
// directory that never touches the network and never calls a real
// scanner: it only reads its own --output argument and writes a fixed
// payload there (or fails), logging its full argv to logPath so a test can
// assert exactly what this package invoked it with.
func writeFakeTrivy(t *testing.T, behavior fakeTrivyBehavior) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(binDir, "trivy.log")

	var write string
	switch behavior {
	case behaviorValid:
		write = `cat > "$out" <<'JSON'
{"bomFormat":"CycloneDX","specVersion":"1.6","components":[]}
JSON
`
	case behaviorBadVersion:
		write = `cat > "$out" <<'JSON'
{"bomFormat":"CycloneDX","specVersion":"1.5","components":[]}
JSON
`
	case behaviorWrongFormat:
		write = `cat > "$out" <<'JSON'
{"bomFormat":"SPDX","specVersion":"1.6","components":[]}
JSON
`
	case behaviorNotJSON:
		write = `printf 'not json\n' > "$out"
`
	case behaviorEmpty:
		write = `: > "$out"
`
	case behaviorFail:
		write = `echo "fake-trivy: simulated failure" >&2
exit 1
`
	default:
		t.Fatalf("unknown fake trivy behavior %q", behavior)
	}

	script := "#!/bin/sh\n" +
		"set -eu\n" +
		"printf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		"out=\"\"\nprev=\"\"\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--output\" ]; then out=\"$a\"; fi\n" +
		"  prev=\"$a\"\n" +
		"done\n" +
		write +
		"exit 0\n"

	path := filepath.Join(binDir, "trivy")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake trivy: %v", err)
	}
	return binDir, logPath
}

// setFakePATH prepends binDir to PATH for the duration of the test, kept
// so the fake trivy's own "#!/bin/sh" is still resolvable. t.Setenv already
// restores PATH when the test ends.
func setFakePATH(t *testing.T, binDir string) {
	t.Helper()
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func readBOM(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return out
}

// TestAUR549TrivyGeneratesValidatedSBOM is AC-001: a fake trivy that
// produces CycloneDX 1.6 output produces the configured, validated file.
func TestAUR549TrivyGeneratesValidatedSBOM(t *testing.T) {
	root := t.TempDir()
	writeSBOMConfig(t, root)
	binDir, logPath := writeFakeTrivy(t, behaviorValid)
	setFakePATH(t, binDir)

	var stdout, stderr bytes.Buffer
	code := runSBOM([]string{"--repo", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	outputPath := filepath.Join(root, "sbom_app_cyclonedx.json")
	bom := readBOM(t, outputPath)
	if bom["bomFormat"] != "CycloneDX" || bom["specVersion"] != "1.6" {
		t.Fatalf("unexpected bom contents: %#v", bom)
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading trivy log: %v", err)
	}
	line := strings.TrimSpace(string(logged))
	if !strings.HasPrefix(line, "fs ") || !strings.Contains(line, "--format cyclonedx") || !strings.Contains(line, "--output") {
		t.Fatalf("unexpected trivy argv: %q", line)
	}
	if !strings.Contains(line, root) {
		t.Fatalf("trivy argv does not name the scanned repo: %q", line)
	}
}

// TestAUR549NonCycloneDXOutputRejected is AC-002: output that is not
// CycloneDX, or whose specVersion differs from the configured one, is
// refused, and no file is left at output_file -- never an empty/invalid
// SBOM silently accepted.
func TestAUR549NonCycloneDXOutputRejected(t *testing.T) {
	cases := []fakeTrivyBehavior{behaviorBadVersion, behaviorWrongFormat, behaviorNotJSON, behaviorEmpty}
	for _, behavior := range cases {
		behavior := behavior
		t.Run(string(behavior), func(t *testing.T) {
			root := t.TempDir()
			writeSBOMConfig(t, root)
			binDir, _ := writeFakeTrivy(t, behavior)
			setFakePATH(t, binDir)

			var stdout, stderr bytes.Buffer
			code := runSBOM([]string{"--repo", root}, &stdout, &stderr)
			if code == 0 {
				t.Fatalf("exit=0, want non-zero (no gate declared: fail closed); stdout=%s stderr=%s", stdout.String(), stderr.String())
			}
			outputPath := filepath.Join(root, "sbom_app_cyclonedx.json")
			if _, err := os.Stat(outputPath); err == nil {
				t.Fatalf("output_file %s was written despite invalid trivy output", outputPath)
			} else if !os.IsNotExist(err) {
				t.Fatalf("stat %s: %v", outputPath, err)
			}
		})
	}
}

// TestAUR549MissingTrivyIsInconclusive is AC-003: trivy absent (or
// erroring) follows the policy's gate.inconclusive -- block (or no gate at
// all) fails closed, warn publishes the reason and exits 0.
func TestAUR549MissingTrivyIsInconclusive(t *testing.T) {
	t.Run("NoGateFailsClosed", func(t *testing.T) {
		root := t.TempDir()
		writeSBOMConfig(t, root)
		emptyDir := t.TempDir()
		// Set PATH to ONLY emptyDir (no prepend): trivy must be genuinely
		// absent, not merely shadowed by a fake one earlier on a PATH that
		// still carries a real trivy later -- exec.Command needs nothing
		// else on PATH to fail with "executable file not found".
		t.Setenv("PATH", emptyDir)

		var stdout, stderr bytes.Buffer
		code := runSBOM([]string{"--repo", root}, &stdout, &stderr)
		if code != exitQualityNotReviewed {
			t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, stdout.String(), stderr.String())
		}
	})

	t.Run("GateBlockFailsClosed", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, "block")
		emptyDir := t.TempDir()
		t.Setenv("PATH", emptyDir)

		var stdout, stderr bytes.Buffer
		code := runSBOM([]string{"--repo", root}, &stdout, &stderr)
		if code != exitQualityNotReviewed {
			t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), gateReasonSBOMFailure) {
			t.Fatalf("stderr does not name the reason %q: %s", gateReasonSBOMFailure, stderr.String())
		}
	})

	t.Run("GateWarnNeverBlocks", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, "warn")
		binDir, _ := writeFakeTrivy(t, behaviorFail)
		setFakePATH(t, binDir)

		var stdout, stderr bytes.Buffer
		code := runSBOM([]string{"--repo", root}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit=%d, want 0 (warn never blocks); stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), gateReasonSBOMFailure) {
			t.Fatalf("stderr does not name the reason %q: %s", gateReasonSBOMFailure, stderr.String())
		}
	})
}

// TestAUR549ImageProducesSeparateFile is AC-004: with an image given, the
// image SBOM is generated into its own file, separate from the
// repository's.
func TestAUR549ImageProducesSeparateFile(t *testing.T) {
	root := t.TempDir()
	writeSBOMConfig(t, root)
	binDir, logPath := writeFakeTrivy(t, behaviorValid)
	setFakePATH(t, binDir)

	var stdout, stderr bytes.Buffer
	code := runSBOM([]string{"--repo", root, "--imagem", "example.test/app:1.0"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	appPath := filepath.Join(root, "sbom_app_cyclonedx.json")
	imagePath := filepath.Join(root, "sbom_app_cyclonedx-image.json")
	if appPath == imagePath {
		t.Fatalf("app and image SBOM paths must differ")
	}
	_ = readBOM(t, appPath)
	_ = readBOM(t, imagePath)

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading trivy log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logged)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want exactly 2 trivy invocations, got %d: %q", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "fs ") {
		t.Fatalf("first invocation should be the filesystem scan: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "image ") || !strings.Contains(lines[1], "example.test/app:1.0") {
		t.Fatalf("second invocation should scan the given image: %q", lines[1])
	}
}
