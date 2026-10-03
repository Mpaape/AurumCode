// AUR-549: `aurumcode sbom` generates an OWASP CycloneDX 1.6 SBOM with
// Trivy (`trivy fs --format cyclonedx --output <file> <repo>`, and `trivy
// image --format cyclonedx --output <file> <image>` when an image
// reference is given), validates the result (JSON, bomFormat=CycloneDX,
// specVersion equal to the configured one) and writes it only after it
// validates -- internal/sbom.GenerateAndValidate never leaves a partial or
// invalid file where output_file is configured to appear, and this command
// never treats an unvalidated file as the SBOM.
//
// Configuration lives at quality_gates.ssor_dtrack.sbom_generator in the
// SAME .aurumcode/config.yml every other section already uses
// (internal/config.Config.QualityGates, qualitygates.go) -- never a
// separate file. With no section declared, this command is a documented
// no-op (exit 0): "Sem a secao correspondente no yml, nada muda no
// comportamento atual."
//
// Trivy failing, being absent, or producing something that does not
// validate is never silently accepted as an empty SBOM. It is routed
// through the exact same AUR-519 policy gate every other inconclusive
// reason uses (evaluateGate, policygate.go), with its own reason token
// gateReasonSBOMFailure: gate.inconclusive: block (or no gate declared at
// all -- a brand-new command with no prior behavior to preserve) fails
// closed; gate.inconclusive: warn publishes the reason on stderr and exits
// 0. Both the gate AND the sbom_generator section are read through the
// SAME single effective-config resolution (loadEffectiveConfig, below):
// internal/config.Load, and, when a policy directory is given,
// ValidatePolicyOutsideReviewedTree + LoadCentralPolicy + ApplyCentralPolicy
// -- so a central policy governs both exactly as it already governs a
// review's own Gate/Rules/Ignore/Exceptions.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/sbom"
)

// gateReasonSBOMFailure is this card's own token for evaluateGate's
// inconclusiveReason parameter, in the same closed vocabulary as
// gateReasonProviderFailure (aur537.go), "model_parse_failure",
// "degraded_parse" and "partial_coverage" (pr.go): a stable,
// non-model-authored string naming exactly what went wrong, never text an
// LLM or a scanned repository controls.
const gateReasonSBOMFailure = "sbom_generation_failure"

// sbomFlags are the values bound by newSBOMFlagSet.
type sbomFlags struct {
	repo, imagem, imagemAlias, politica, politicaAlias, trivyBin *string
}

// newSBOMFlagSet declares the flags of `sbom`; the help reads the same set.
func newSBOMFlagSet() (*flag.FlagSet, sbomFlags) {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	return fs, sbomFlags{
		repo:          fs.String("repo", ".", "repository root to scan with `trivy fs` (default: current directory)"),
		imagem:        fs.String("imagem", "", "optional container image reference to scan with `trivy image`; alias --image"),
		imagemAlias:   fs.String("image", "", "alias of --imagem"),
		politica:      fs.String("politica", "", "directory containing a central policy's .aurumcode/ (same convention as `review --politica`); default: the AURUMCODE_POLICY environment variable, otherwise no policy"),
		politicaAlias: fs.String("policy", "", "alias of --politica"),
		trivyBin:      fs.String("trivy-bin", "", "override the trivy binary/path (default: \"trivy\", resolved from PATH)"),
	}
}

// runSBOM is cmd/aurumcode's AUR-549 wiring: flag parsing, configuration
// resolution (repo vs. central policy), the two Trivy calls, and routing a
// failure through the AUR-519 gate. It never touches the review pipeline in
// main.go/pr.go -- this is a new, standalone subcommand, additive to the
// existing `review`/`fix` commands.
func runSBOM(args []string, stdout, stderr io.Writer) int {
	fs, fl := newSBOMFlagSet()
	repoFlag, imagem, imagemAlias, politica, politicaAlias, trivyBin := fl.repo, fl.imagem, fl.imagemAlias, fl.politica, fl.politicaAlias, fl.trivyBin
	if exit, ok := parseSubcommandFlags("sbom", fs, args, stdout, stderr); !ok {
		return exit
	}

	root := strings.TrimSpace(*repoFlag)
	if root == "" {
		root = "."
	}
	imageRef := strings.TrimSpace(*imagem)
	if imageRef == "" {
		imageRef = strings.TrimSpace(*imagemAlias)
	}
	policyDir := strings.TrimSpace(*politica)
	if policyDir == "" {
		policyDir = strings.TrimSpace(*politicaAlias)
	}
	if policyDir == "" {
		policyDir = strings.TrimSpace(os.Getenv("AURUMCODE_POLICY"))
	}

	effective, gateOrigin, warnings, err := loadEffectiveConfig(root, policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode sbom: %v\n", err)
		return 2
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "aurumcode sbom: %s: %s\n", w.Provider, w.Reason)
	}

	var genCfg sbom.GeneratorConfig
	if effective.QualityGates.SsorDtrack != nil {
		genCfg = effective.QualityGates.SsorDtrack.SBOMGenerator
	}
	if err := genCfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "aurumcode sbom: %v\n", err)
		return 2
	}
	if !genCfg.Declared() {
		fmt.Fprintln(stderr, "aurumcode sbom: quality_gates.ssor_dtrack.sbom_generator nao declarado; nada a fazer")
		return 0
	}
	// Trimmed ONCE, here, at the source: Validate() above already checked
	// the TRIMMED value's format (strings.TrimSpace inside
	// SBOMGeneratorConfig.Validate), so every later use of SpecVersion --
	// GenerateAndValidate's comparison against the SBOM's own specVersion
	// included -- must see that exact same trimmed value, never the raw
	// config string a stray surrounding space could otherwise smuggle
	// past Validate's check and into a comparison that then always fails.
	genCfg.SpecVersion = strings.TrimSpace(genCfg.SpecVersion)

	outputPath, err := sbom.ResolveOutputPath(root, genCfg.OutputFile)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode sbom: %v\n", err)
		return 2
	}

	gen := sbom.TrivyGenerator{Binary: strings.TrimSpace(*trivyBin), Format: genCfg.Format}
	ctx := context.Background()

	genErr := sbom.GenerateAndValidate(ctx, gen, sbom.Target{Kind: "fs", Source: root, OutputPath: outputPath}, genCfg.SpecVersion)

	var imageOutputPath string
	if genErr == nil && imageRef != "" {
		imageOutputPath = sbom.ImageOutputPath(outputPath)
		genErr = sbom.GenerateAndValidate(ctx, gen, sbom.Target{Kind: "image", Source: imageRef, OutputPath: imageOutputPath}, genCfg.SpecVersion)
	}

	if genErr != nil {
		return reportSBOMFailure(stderr, genErr, effective.Gate, gateOrigin)
	}

	fmt.Fprintf(stdout, "sbom: %s\n", outputPath)
	if imageOutputPath != "" {
		fmt.Fprintf(stdout, "sbom: %s\n", imageOutputPath)
	}
	return 0
}

// reportSBOMFailure prints genErr and routes it through the AUR-519 gate
// exactly like a provider/transport failure does in runPRReview (pr.go):
// evaluateGate decides Fail/Inconclusive from gate.inconclusive, and every
// line it returns is printed the same way runPRReview prints
// gateResult.Lines. With no gate declared at all (so evaluateGate returns
// an inactive decision), this command fails closed -- unlike --base/--pr,
// `aurumcode sbom` has no pre-AUR-519 behavior to stay byte-compatible
// with, and the card's own Outcome is explicit: "nunca SBOM vazio
// aceito".
func reportSBOMFailure(stderr io.Writer, genErr error, gateCfg config.GateConfig, gateOrigin string) int {
	fmt.Fprintf(stderr, "aurumcode sbom: %v\n", genErr)

	gateResult, gateErr := evaluateGate(gateCfg, gateOrigin, nil, nil, gateReasonSBOMFailure, nil, "", time.Now())
	if gateErr != nil {
		fmt.Fprintf(stderr, "aurumcode sbom: gate: %v\n", gateErr)
		return 2
	}
	mode, modeErr := gateCfg.InconclusiveMode(config.InconclusiveBlock)
	applyInconclusiveModeValue(mode, modeErr, &gateResult)
	for _, line := range gateResult.Lines {
		fmt.Fprintf(stderr, "aurumcode sbom: policy gate: %s\n", line)
	}
	if !gateResult.Active || gateResult.Fail {
		return exitQualityNotReviewed
	}
	// gateResult.Active && !gateResult.Fail: gate.inconclusive: warn (or a
	// declared gate with no inconclusive mode at all) -- published above,
	// never blocking.
	return 0
}

// loadEffectiveConfig reads .aurumcode/config.yml the exact same way
// runReview/runPRReview already do (internal/config.Load, then, when
// policyDir is set, ValidatePolicyOutsideReviewedTree + LoadCentralPolicy
// + ApplyCentralPolicy), so a central policy governs this command's Gate
// AND its quality_gates.ssor_dtrack.sbom_generator section identically to
// how it already governs a review's Gate/Rules/Ignore/Exceptions -- ONE
// load, ONE precedence decision, both sections read from its result. This
// is the only piece of AUR-519/AUR-518 machinery this command reuses from
// internal/config directly (an ordinary import of a package this card now
// also has in its own `paths`, not a reimplementation in internal/sbom).
func loadEffectiveConfig(root, policyDir string) (*config.Config, string, []config.ProviderWarning, error) {
	repoCfg, err := config.Load(root)
	if err != nil {
		return nil, gateOriginRepo, nil, fmt.Errorf("config: %w", err)
	}
	if strings.TrimSpace(policyDir) == "" {
		return repoCfg, gateOriginRepo, nil, nil
	}
	if err := config.ValidatePolicyOutsideReviewedTree(policyDir, root); err != nil {
		return nil, gateOriginPolicy, nil, err
	}
	centralCfg, err := config.LoadCentralPolicy(policyDir)
	if err != nil {
		return nil, gateOriginPolicy, nil, err
	}
	effective, warnings := config.ApplyCentralPolicy(repoCfg, centralCfg)
	return effective, gateOriginPolicy, warnings, nil
}
