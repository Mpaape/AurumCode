// AUR-551: `aurumcode sign` signs the SBOM (AUR-549's own output) and/or
// the artifact image with Sigstore/Cosign (internal/supplychain), calling
// the external `cosign` binary named by --cosign-bin (default: "cosign",
// resolved from PATH -- the same convention `aurumcode sbom`'s
// --trivy-bin already uses for Trivy).
//
// Configuration lives at quality_gates.supply_chain in the SAME
// .aurumcode/config.yml every other section already uses
// (internal/config.QualityGatesConfig, qualitygates.go) -- never a
// separate file, and resolved through the SAME repo/central-policy
// precedence as `aurumcode sbom` (loadEffectiveConfig, aur549.go). With
// no section declared at all, this command is a documented no-op (exit
// 0): "Sem a secao supply_chain, nada muda" (AC-003).
//
// Unlike `aurumcode sbom`'s own inconclusive-SBOM-generation failure,
// this card's own AC-002 is unconditional: "falha de assinatura reprova
// a etapa" names no inconclusive mode to soften it, so a signing failure
// here is ALWAYS a non-zero exit naming the artifact that was left
// unsigned -- it is never routed through gate.inconclusive the way
// reportSBOMFailure routes a Trivy failure. Ignoring that failure (the
// defect MUT-001 exists to catch) would let the gate approve an artifact
// that was never actually signed.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/sbom"
	"github.com/Mpaape/AurumCode/internal/supplychain"
)

// stringListFlag collects repeated occurrences of the same flag
// (--image <ref>, repeated for more than one artifact; --sbom <file>,
// repeated likewise) into an ordered slice -- flag.FlagSet has no
// built-in repeatable string flag.
type stringListFlag struct {
	values []string
}

func (f *stringListFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(f.values, ",")
}

func (f *stringListFlag) Set(v string) error {
	v = strings.TrimSpace(v)
	if v != "" {
		f.values = append(f.values, v)
	}
	return nil
}

// signFlags are the values bound by newSignFlagSet.
type signFlags struct {
	repo, politica, politicaAlias, cosignBin *string
	sboms, images                            *stringListFlag
}

// newSignFlagSet declares the flags of `sign`; the help reads the same set.
func newSignFlagSet() (*flag.FlagSet, signFlags) {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	fl := signFlags{
		repo:          fs.String("repo", ".", "repository root whose configuration governs signing (default: current directory)"),
		politica:      fs.String("politica", "", "directory containing a central policy's .aurumcode/ (same convention as `sbom --politica`); default: the AURUMCODE_POLICY environment variable, otherwise no policy"),
		politicaAlias: fs.String("policy", "", "alias of --politica"),
		cosignBin:     fs.String("cosign-bin", "", "override the cosign binary/path (default: \"cosign\", resolved from PATH)"),
		sboms:         &stringListFlag{},
		images:        &stringListFlag{},
	}
	fs.Var(fl.sboms, "sbom", "SBOM file to sign; repeat for more than one (default: quality_gates.ssor_dtrack.sbom_generator.output_file, when configured)")
	fs.Var(fl.images, "image", "image reference pinned by digest (@sha256:...) to sign; repeat for more than one (default: quality_gates.supply_chain.artifacts)")
	return fs, fl
}

// runSign is cmd/aurumcode's AUR-551 wiring: flag parsing, configuration
// resolution (repo vs. central policy, reusing aur549.go's
// loadEffectiveConfig), and the cosign calls themselves. It never touches
// the review pipeline in main.go/pr.go, nor `aurumcode sbom` -- this is a
// new, standalone subcommand, additive to the existing
// `review`/`fix`/`sbom` commands.
func runSign(args []string, stdout, stderr io.Writer) int {
	fs, fl := newSignFlagSet()
	repoFlag, politica, politicaAlias, cosignBin, sboms, images := fl.repo, fl.politica, fl.politicaAlias, fl.cosignBin, fl.sboms, fl.images
	if exit, ok := parseSubcommandFlags("sign", fs, args, stdout, stderr); !ok {
		return exit
	}

	root := strings.TrimSpace(*repoFlag)
	if root == "" {
		root = "."
	}
	policyDir := strings.TrimSpace(*politica)
	if policyDir == "" {
		policyDir = strings.TrimSpace(*politicaAlias)
	}
	if policyDir == "" {
		policyDir = strings.TrimSpace(os.Getenv("AURUMCODE_POLICY"))
	}

	effective, _, warnings, err := loadEffectiveConfig(root, policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode sign: %v\n", err)
		return 2
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "aurumcode sign: %s: %s\n", w.Provider, w.Reason)
	}

	scCfg := effective.QualityGates.SupplyChain
	// AC-003: no section declared at all is a documented no-op, checked
	// BEFORE --cosign-bin is ever resolved or a single file touched --
	// "sem a secao supply_chain, nada muda".
	if !scCfg.Declared() {
		fmt.Fprintln(stderr, "aurumcode sign: quality_gates.supply_chain nao declarado; nada a fazer")
		return 0
	}
	if err := scCfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "aurumcode sign: %v\n", err)
		return 2
	}

	sbomTargets := sboms.values
	if len(sbomTargets) == 0 && scCfg.SignSBOM {
		fallback := ""
		if effective.QualityGates.SsorDtrack != nil {
			fallback = effective.QualityGates.SsorDtrack.SBOMGenerator.OutputFile
		}
		if strings.TrimSpace(fallback) != "" {
			resolved, resolveErr := sbom.ResolveOutputPath(root, fallback)
			if resolveErr != nil {
				fmt.Fprintf(stderr, "aurumcode sign: %v\n", resolveErr)
				return 2
			}
			sbomTargets = []string{resolved}
		}
	}
	imageTargets := images.values
	if len(imageTargets) == 0 && scCfg.SignArtifacts {
		imageTargets = append(imageTargets, scCfg.Artifacts...)
	}

	if scCfg.SignSBOM && len(sbomTargets) == 0 {
		fmt.Fprintln(stderr, "aurumcode sign: sign_sbom esta ligado mas nenhum SBOM foi informado (--sbom, ou quality_gates.ssor_dtrack.sbom_generator.output_file)")
		return exitQualityNotReviewed // AUR-551: sign_sbom declared without an sbom to sign
	}
	if scCfg.SignArtifacts && len(imageTargets) == 0 {
		fmt.Fprintln(stderr, "aurumcode sign: sign_artifacts esta ligado mas nenhuma imagem foi informada (--image, ou quality_gates.supply_chain.artifacts)")
		return exitQualityNotReviewed // AUR-551: sign_artifacts declared without an image to sign
	}

	signer := supplychain.Signer{Binary: strings.TrimSpace(*cosignBin)}
	ctx := context.Background()

	if scCfg.SignSBOM {
		for _, path := range sbomTargets {
			// H3: refused before os.Stat or any cosign call, whether path
			// came from --sbom or from the sbom_generator.output_file
			// fallback -- a value starting with "-" could otherwise be
			// read as a cosign flag depending on argv position (the same
			// reason ValidateArtifactRef refuses a leading "-" for image
			// references below).
			if strings.HasPrefix(path, "-") {
				fmt.Fprintf(stderr, "aurumcode sign: sbom path %q must not start with \"-\"\n", path)
				return exitQualityNotReviewed // AUR-551: sbom path looks like a flag
			}
			if _, statErr := os.Stat(path); statErr != nil {
				fmt.Fprintf(stderr, "aurumcode sign: sbom %s: %v\n", path, statErr)
				return exitQualityNotReviewed // AUR-551: sbom path does not exist
			}
			bundlePath := path + ".sigstore.json"
			if signErr := signer.SignBlob(ctx, supplychain.Options{BundlePath: bundlePath}, path); signErr != nil {
				fmt.Fprintf(stderr, "aurumcode sign: %v\n", signErr)
				return exitQualityNotReviewed // AUR-551 AC-002: sbom signing failure must never be swallowed
			}
			fmt.Fprintf(stdout, "sign: sbom %s -> %s\n", path, bundlePath)
		}
	}

	if scCfg.SignArtifacts {
		for _, image := range imageTargets {
			if validateErr := supplychain.ValidateArtifactRef(image); validateErr != nil {
				fmt.Fprintf(stderr, "aurumcode sign: %v\n", validateErr)
				return exitQualityNotReviewed // AUR-551: image reference is not digest-pinned
			}
			if signErr := signer.SignImage(ctx, supplychain.Options{}, image); signErr != nil {
				fmt.Fprintf(stderr, "aurumcode sign: %v\n", signErr)
				return exitQualityNotReviewed // AUR-551 AC-002: image signing failure must never be swallowed
			}
			fmt.Fprintf(stdout, "sign: image %s\n", image)
		}
	}

	return 0
}
