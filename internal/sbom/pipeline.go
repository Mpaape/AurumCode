package sbom

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Target names one Generator call: Kind is "fs" (GenerateFilesystem) or
// "image" (GenerateImage), Source is the repository root or the image
// reference, and OutputPath is where the validated SBOM must end up.
type Target struct {
	Kind       string
	Source     string
	OutputPath string
}

// GenerateAndValidate runs gen against target and only ever leaves a file
// at target.OutputPath that already passed Validate against wantSpecVersion
// -- the card's own "nunca SBOM vazio aceito": generation writes to a
// temporary file in the same directory first, and that file is renamed
// into place only after it validates. Any failure (the generator erroring,
// producing nothing, or producing something that fails Validate) returns a
// non-nil error and leaves OutputPath completely untouched -- a prior good
// SBOM is never clobbered by a failed run, and a failed first run never
// leaves a partial or invalid file where a downstream consumer (AUR-550)
// might mistake it for real output.
func GenerateAndValidate(ctx context.Context, gen Generator, target Target, wantSpecVersion string) error {
	dir := filepath.Dir(target.OutputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sbom: creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".sbom-*.tmp")
	if err != nil {
		return fmt.Errorf("sbom: creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	switch target.Kind {
	case "fs":
		err = gen.GenerateFilesystem(ctx, target.Source, tmpPath)
	case "image":
		err = gen.GenerateImage(ctx, target.Source, tmpPath)
	default:
		return fmt.Errorf("sbom: unknown target kind %q", target.Kind)
	}
	if err != nil {
		return fmt.Errorf("sbom: generating %s SBOM: %w", target.Kind, err)
	}
	if err := Validate(tmpPath, wantSpecVersion); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, target.OutputPath); err != nil {
		return fmt.Errorf("sbom: writing %s: %w", target.OutputPath, err)
	}
	return nil
}

// ImageOutputPath derives AC-004's separate image-SBOM path from the
// repository SBOM's own configured path: the same directory and extension,
// with "-image" inserted before the extension (sbom_app_cyclonedx.json ->
// sbom_app_cyclonedx-image.json). Only one output_file is configured
// (quality_gates.ssor_dtrack.sbom_generator has no second key for the image
// case); this keeps the image file deterministic and documented
// (docs/configuration.md) rather than inventing a second config key this
// card's Outcome never mentions.
func ImageOutputPath(appOutputPath string) string {
	dir := filepath.Dir(appOutputPath)
	base := filepath.Base(appOutputPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+"-image"+ext)
}
