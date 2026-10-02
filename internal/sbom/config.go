package sbom

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigRelPath is where this card's own configuration lives, relative to a
// repository (or central policy) root: a SEPARATE file from
// internal/config's .aurumcode/config.yml, never a new top-level key added
// to that file.
//
// This is deliberate, not a shortcut: internal/config.LoadCentralPolicy
// decodes a policy's config.yml with yaml's KnownFields(true) (central.go)
// so a typo in a policy's rule/ignore/gate keys is a loud error. Adding an
// unrecognized "quality_gates" key to that same file would make EVERY
// existing `review --politica`/`--policy` run against a policy that also
// carries this card's section fail with an "unknown field" error --
// breaking AUR-518 for a card that has nothing to do with it. internal/config
// is explicitly NOT in this card's `paths` (AUR-549.md), so teaching it
// about quality_gates is not this card's call to make; AUR-550 (which DOES
// list internal/config in its paths and depends on this card) is the
// natural place to fold this section into the shared Config type if that
// is ever wanted. Until then, quality_gates.yml is read independently, by
// this package alone, and never through internal/config's Parse/Load/
// LoadCentralPolicy at all.
const ConfigRelPath = ".aurumcode/quality_gates.yml"

// GeneratorConfig is quality_gates.ssor_dtrack.sbom_generator, exactly as
// the card's Outcome shows it:
//
//	quality_gates:
//	  ssor_dtrack:
//	    sbom_generator:
//	      tool: trivy
//	      format: cyclonedx
//	      spec_version: "1.6"
//	      output_file: sbom_app_cyclonedx.json
type GeneratorConfig struct {
	Tool        string `yaml:"tool"`
	Format      string `yaml:"format"`
	SpecVersion string `yaml:"spec_version"`
	OutputFile  string `yaml:"output_file"`
}

// Declared reports whether this section was actually written, as opposed to
// being the absent zero value -- the same "zero value means off" contract
// internal/config.GateConfig already uses. Any one of the four fields
// being non-blank counts: a config that names only some of them still has
// to go through Validate so the missing ones are reported, never silently
// treated as "nothing declared at all".
func (g GeneratorConfig) Declared() bool {
	return strings.TrimSpace(g.Tool) != "" ||
		strings.TrimSpace(g.Format) != "" ||
		strings.TrimSpace(g.SpecVersion) != "" ||
		strings.TrimSpace(g.OutputFile) != ""
}

// Validate enforces this card's closed vocabulary. tool/format are not
// free text: AC-001's Outcome names exactly one tool (trivy) and one format
// (cyclonedx), so anything else is a loud configuration error at load time,
// never a silent no-op and never an attempt to shell out to a different,
// unreviewed tool.
func (g GeneratorConfig) Validate() error {
	if !g.Declared() {
		return nil
	}
	if strings.TrimSpace(g.Tool) != "trivy" {
		return fmt.Errorf("sbom: quality_gates.ssor_dtrack.sbom_generator.tool: only %q is supported, got %q", "trivy", g.Tool)
	}
	if strings.TrimSpace(g.Format) != "cyclonedx" {
		return fmt.Errorf("sbom: quality_gates.ssor_dtrack.sbom_generator.format: only %q is supported, got %q", "cyclonedx", g.Format)
	}
	if strings.TrimSpace(g.SpecVersion) == "" {
		return fmt.Errorf("sbom: quality_gates.ssor_dtrack.sbom_generator.spec_version: must not be empty")
	}
	if strings.TrimSpace(g.OutputFile) == "" {
		return fmt.Errorf("sbom: quality_gates.ssor_dtrack.sbom_generator.output_file: must not be empty")
	}
	return nil
}

// configFile is the on-disk shape of quality_gates.yml: just the one
// nested path this card reads. Any other key at any level is ignored by
// yaml.v3's default (non-strict) Unmarshal -- unlike the central-policy
// decode this deliberately avoids (see ConfigRelPath's doc comment), a
// typo in an unrelated, future quality_gates.* section this card does not
// own yet must not make this card's own read fail.
type configFile struct {
	QualityGates struct {
		SSORDTrack struct {
			SBOMGenerator GeneratorConfig `yaml:"sbom_generator"`
		} `yaml:"ssor_dtrack"`
	} `yaml:"quality_gates"`
}

// LoadConfig reads root/.aurumcode/quality_gates.yml. A missing file is the
// zero-config case (GeneratorConfig{}, nil error) -- the card's own
// Documentation note: "Sem a secao correspondente no yml, nada muda no
// comportamento atual." A file that exists but cannot be parsed is a loud
// error, matching internal/config.LoadPath's own contract for the file it
// owns.
func LoadConfig(root string) (GeneratorConfig, error) {
	path := filepath.Join(root, filepath.FromSlash(ConfigRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GeneratorConfig{}, nil
		}
		return GeneratorConfig{}, fmt.Errorf("sbom: reading %s: %w", path, err)
	}
	var parsed configFile
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return GeneratorConfig{}, fmt.Errorf("sbom: parsing %s: %w", path, err)
	}
	return parsed.QualityGates.SSORDTrack.SBOMGenerator, nil
}

// ResolveConfig applies AUR-518's own central-policy precedence to this
// card's section: when policyDir is empty, the repository's own
// quality_gates.yml governs, exactly as read. When policyDir is set, ONLY
// the policy's own section ever applies -- mirroring
// internal/config.ApplyCentralPolicy's treatment of Gate (central.go): a
// repository cannot declare its own sbom_generator section to shadow or
// loosen what a policy decided, even if the policy declares nothing at all
// (which turns the step off for that run). A repo-declared section under
// an active policy is dropped with a named warning, never silently merged.
func ResolveConfig(repoRoot, policyDir string) (GeneratorConfig, []string, error) {
	repoCfg, err := LoadConfig(repoRoot)
	if err != nil {
		return GeneratorConfig{}, nil, err
	}
	if strings.TrimSpace(policyDir) == "" {
		if err := repoCfg.Validate(); err != nil {
			return GeneratorConfig{}, nil, err
		}
		return repoCfg, nil, nil
	}
	policyCfg, err := LoadConfig(policyDir)
	if err != nil {
		return GeneratorConfig{}, nil, err
	}
	if err := policyCfg.Validate(); err != nil {
		return GeneratorConfig{}, nil, err
	}
	var warnings []string
	if repoCfg.Declared() {
		warnings = append(warnings, "quality_gates.ssor_dtrack.sbom_generator do repositório foi ignorado: a política central decide sozinha")
	}
	return policyCfg, warnings, nil
}

// ResolveOutputPath joins root and outputFile, refusing anything that would
// resolve outside root: an absolute path, a "../" escape, or (after
// Abs/Clean) any other way the configured string could name a file outside
// the repository. output_file is repository- or policy-authored text, but
// it is still an externally supplied filename and is treated exactly like
// every other one this engine resolves -- untrusted as a path even when
// its source is trusted as configuration (CR-TRUST-001).
func ResolveOutputPath(root, outputFile string) (string, error) {
	outputFile = strings.TrimSpace(outputFile)
	if outputFile == "" {
		return "", fmt.Errorf("sbom: output_file must not be empty")
	}
	clean := filepath.Clean(filepath.FromSlash(outputFile))
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("sbom: output_file %q must be a relative path", outputFile)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("sbom: %w", err)
	}
	fullAbs, err := filepath.Abs(filepath.Join(rootAbs, clean))
	if err != nil {
		return "", fmt.Errorf("sbom: %w", err)
	}
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("sbom: output_file %q escapes the repository root", outputFile)
	}
	return fullAbs, nil
}
