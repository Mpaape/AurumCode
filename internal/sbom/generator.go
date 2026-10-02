// Package sbom is AUR-549's SBOM generation: an OWASP CycloneDX document
// produced by running Trivy as an external process (never a network call
// this package makes itself, and never a second, in-process SBOM builder).
//
// Generator is the seam cmd/aurumcode's wiring (aur549.go) calls through, so
// the orchestration (GenerateAndValidate, pipeline.go) and its tests never
// depend on the real trivy binary being installed: TrivyGenerator is the
// only implementation that actually shells out, and the acceptance tests
// substitute a fake trivy executable on PATH instead of hitting a real
// scanner or any network.
package sbom

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Generator produces a CycloneDX SBOM file for either a filesystem tree
// (trivy fs) or a container image (trivy image). Both methods write their
// result to outputPath and return a non-nil error on any failure -- a
// partial or missing write is the caller's (GenerateAndValidate's)
// responsibility to detect, never silently treated as success here.
type Generator interface {
	GenerateFilesystem(ctx context.Context, root, outputPath string) error
	GenerateImage(ctx context.Context, image, outputPath string) error
}

// TrivyGenerator runs the real `trivy` binary. Binary defaults to "trivy"
// (resolved through PATH by os/exec, exactly like any other external tool
// this engine shells out to) and Format defaults to "cyclonedx" -- the only
// format this card ever asks Trivy to produce; a GeneratorConfig that asked
// for anything else is already refused earlier, at config-validation time
// (GeneratorConfig.Validate), so this type never has to re-check it.
type TrivyGenerator struct {
	Binary string
	Format string
}

func (t TrivyGenerator) binary() string {
	if strings.TrimSpace(t.Binary) == "" {
		return "trivy"
	}
	return t.Binary
}

func (t TrivyGenerator) format() string {
	if strings.TrimSpace(t.Format) == "" {
		return "cyclonedx"
	}
	return t.Format
}

// GenerateFilesystem runs `trivy fs --format <format> --output <outputPath>
// <root>` -- the repository scan AC-001/AC-002/AC-003 exercise.
func (t TrivyGenerator) GenerateFilesystem(ctx context.Context, root, outputPath string) error {
	return t.run(ctx, []string{"fs", "--format", t.format(), "--output", outputPath, root})
}

// GenerateImage runs `trivy image --format <format> --output <outputPath>
// <image>` -- AC-004's separate scan for an explicitly given image
// reference. Never invoked unless the caller was actually given an image
// (see aur549.go): this card's Non-goals exclude inventing one.
func (t TrivyGenerator) GenerateImage(ctx context.Context, image, outputPath string) error {
	return t.run(ctx, []string{"image", "--format", t.format(), "--output", outputPath, image})
}

// run executes the trivy binary with args, capturing combined stdout+stderr
// only to attach to the returned error (never printed verbatim elsewhere by
// this package, so a secret Trivy might echo stays bounded to the one
// error path this package's own caller decides how to surface -- today,
// the plain %v the caller prints to stderr, which already goes through the
// process-wide redaction writer in cmd/aurumcode).
func (t TrivyGenerator) run(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, t.binary(), args...)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trivy %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(combined.String()))
	}
	return nil
}
