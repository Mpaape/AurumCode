// AUR-549: `aurumcode sbom` generates an OWASP CycloneDX 1.6 SBOM with
// Trivy (`trivy fs --format cyclonedx --output <file> <repo>`, and `trivy
// image --format cyclonedx --output <file> <image>` when an image
// reference is given), validates the result (JSON, bomFormat=CycloneDX,
// specVersion equal to the configured one) and writes it only after it
// validates -- internal/sbom.GenerateAndValidate never leaves a partial or
// invalid file where output_file is configured to appear, and this command
// never treats an unvalidated file as the SBOM.
//
// Configuration lives at .aurumcode/quality_gates.yml, under
// quality_gates.ssor_dtrack.sbom_generator -- a file of its own, not a new
// key inside internal/config's .aurumcode/config.yml; see
// internal/sbom.ConfigRelPath's doc comment for exactly why, and
// docs/configuration.md for the operator-facing version. With no section
// declared, this command is a documented no-op (exit 0): "Sem a secao
// correspondente no yml, nada muda no comportamento atual."
//
// Trivy failing, being absent, or producing something that does not
// validate is never silently accepted as an empty SBOM. It is routed
// through the exact same AUR-519 policy gate every other inconclusive
// reason uses (evaluateGate, policygate.go), with its own reason token
// gateReasonSBOMFailure: gate.inconclusive: block (or no gate declared at
// all -- a brand-new command with no prior behavior to preserve) fails
// closed; gate.inconclusive: warn publishes the reason on stderr and exits
// 0. The gate's own configuration (quality_gates.yml does not carry a gate
// section) is read from the SAME places --base/--pr already read it from:
// internal/config.Load, and, when a policy directory is given, LoadCentralPolicy
// + ApplyCentralPolicy -- so a central policy's gate.inconclusive setting
// governs this command exactly as it governs a review.
package main

import (
	"context"
	"errors"
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

// runSBOM is cmd/aurumcode's AUR-549 wiring: flag parsing, configuration
// resolution (repo vs. central policy), the two Trivy calls, and routing a
// failure through the AUR-519 gate. It never touches the review pipeline in
// main.go/pr.go -- this is a new, standalone subcommand, additive to the
// existing `review`/`fix` commands.
func runSBOM(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	repoFlag := fs.String("repo", ".", "repository root to scan with `trivy fs` (default: current directory)")
	imagem := fs.String("imagem", "", "optional container image reference to scan with `trivy image`; alias --image")
	imagemAlias := fs.String("image", "", "alias of --imagem")
	politica := fs.String("politica", "", "directory containing a central policy's .aurumcode/ (same convention as `review --politica`); default: the AURUMCODE_POLICY environment variable, otherwise no policy")
	politicaAlias := fs.String("policy", "", "alias of --politica")
	trivyBin := fs.String("trivy-bin", "", "override the trivy binary/path (default: \"trivy\", resolved from PATH)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, "usage: aurumcode sbom [--repo dir] [--imagem referencia] [--politica dir] [--trivy-bin caminho]")
			fmt.Fprintln(stdout, "Gera um SBOM OWASP CycloneDX com Trivy para o repositorio e, com --imagem, para uma imagem de container.")
			fmt.Fprintln(stdout, "Configuracao: quality_gates.ssor_dtrack.sbom_generator em .aurumcode/quality_gates.yml (ver docs/configuration.md).")
			return 0
		}
		return 2
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
	if policyDir != "" {
		// CR-TRUST-001: checked unconditionally, before ResolveConfig's own
		// "nothing declared" early return below -- a policy directory that
		// resolves inside the reviewed repository must never be trusted
		// even for a run that turns out to have nothing to generate.
		if err := config.ValidatePolicyOutsideReviewedTree(policyDir, root); err != nil {
			fmt.Fprintf(stderr, "aurumcode sbom: %v\n", err)
			return 2
		}
	}

	genCfg, warnings, err := sbom.ResolveConfig(root, policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode sbom: %v\n", err)
		return 2
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "aurumcode sbom: %s\n", w)
	}

	if !genCfg.Declared() {
		fmt.Fprintln(stderr, "aurumcode sbom: quality_gates.ssor_dtrack.sbom_generator nao declarado; nada a fazer")
		return 0
	}

	gateCfg, gateOrigin, err := loadSBOMGateConfig(root, policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode sbom: %v\n", err)
		return 2
	}

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
		return reportSBOMFailure(stderr, genErr, gateCfg, gateOrigin)
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
// gateResult.Lines. With no gate declared at all (gateCfg.Declared() ==
// false, so evaluateGate returns an inactive decision), this command fails
// closed -- unlike --base/--pr, `aurumcode sbom` has no pre-AUR-519
// behavior to stay byte-compatible with, and the card's own Outcome is
// explicit: "nunca SBOM vazio aceito".
func reportSBOMFailure(stderr io.Writer, genErr error, gateCfg config.GateConfig, gateOrigin string) int {
	fmt.Fprintf(stderr, "aurumcode sbom: %v\n", genErr)

	gateResult, gateErr := evaluateGate(gateCfg, gateOrigin, nil, nil, gateReasonSBOMFailure, nil, "", time.Now())
	if gateErr != nil {
		fmt.Fprintf(stderr, "aurumcode sbom: gate: %v\n", gateErr)
		return 2
	}
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

// loadSBOMGateConfig reads the AUR-519 gate the exact same way runReview/
// runPRReview already do (internal/config.Load, then, when policyDir is
// set, ValidatePolicyOutsideReviewedTree + LoadCentralPolicy +
// ApplyCentralPolicy), so a central policy's gate.inconclusive setting
// governs `aurumcode sbom` identically to how it governs a review. This is
// the one piece of AUR-519/AUR-518 machinery this command reuses from
// internal/config directly (a read-only import, not an edit to that
// package) rather than reimplementing in internal/sbom.
func loadSBOMGateConfig(root, policyDir string) (config.GateConfig, string, error) {
	repoCfg, err := config.Load(root)
	if err != nil {
		return config.GateConfig{}, gateOriginRepo, fmt.Errorf("config: %w", err)
	}
	if strings.TrimSpace(policyDir) == "" {
		return repoCfg.Gate, gateOriginRepo, nil
	}
	if err := config.ValidatePolicyOutsideReviewedTree(policyDir, root); err != nil {
		return config.GateConfig{}, gateOriginPolicy, err
	}
	centralCfg, err := config.LoadCentralPolicy(policyDir)
	if err != nil {
		return config.GateConfig{}, gateOriginPolicy, err
	}
	effective, _ := config.ApplyCentralPolicy(repoCfg, centralCfg)
	return effective.Gate, gateOriginPolicy, nil
}
