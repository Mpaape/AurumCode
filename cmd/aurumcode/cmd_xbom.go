// AUR-552: `aurumcode xbom --type build|cbom` generates a CycloneDX 1.6
// Build BOM or CBOM from the repository. Candidate collection is driven by
// catalogs (internal/xbom/catalog/<type>.yml, overridable by
// .aurumcode/xbom/<type>.yml in the repository or the central policy);
// the LLM, when a provider is configured, only classifies and enriches. A
// component reaches the BOM only if the line it cites contains its evidence
// token (internal/xbom, AC-002). The file is written to a temporary name,
// validated with the AUR-549 validator (specVersion >= the configured
// minimum) and only then renamed: a failure never leaves a partial file.
//
// aibom, saasbom and netbom are not generated: they exit 2 pointing at the
// documentation section that defines their format and delivery (AC-003).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/internal/xbom"
)

// xbomDocumentedOnly maps the xBOM types that are defined but not generated
// to the documentation section that defines them.
var xbomDocumentedOnly = map[string]string{
	"aibom":   "docs/configuration.md#xbom-aibom",
	"saasbom": "docs/configuration.md#xbom-saasbom",
	"netbom":  "docs/configuration.md#xbom-netbom",
}

const exitUsageUnknownType = 64

// xbomFlags are the values bound by newXBOMFlagSet.
type xbomFlags struct {
	typ, repo, politica, policy, out *string
}

// newXBOMFlagSet declares the flags of `xbom`; the help reads the same set.
func newXBOMFlagSet() (*flag.FlagSet, xbomFlags) {
	fs := flag.NewFlagSet("xbom", flag.ContinueOnError)
	return fs, xbomFlags{
		typ:      fs.String("type", "", "xBOM type: build or cbom"),
		repo:     fs.String("repo", ".", "repository root to read"),
		politica: fs.String("politica", "", "central policy directory (default: AURUMCODE_POLICY)"),
		policy:   fs.String("policy", "", "alias of --politica"),
		out:      fs.String("out", "", "output file (default: stdout)"),
	}
}

func runXBOM(args []string, stdout, stderr io.Writer) int {
	fs, fl := newXBOMFlagSet()
	typ, repo, politica, policy, out := fl.typ, fl.repo, fl.politica, fl.policy, fl.out
	if exit, ok := parseSubcommandFlags("xbom", fs, args, stdout, stderr); !ok {
		return exit
	}
	t := strings.TrimSpace(*typ)
	if doc, ok := xbomDocumentedOnly[t]; ok {
		fmt.Fprintf(stderr, "aurumcode xbom: o tipo %q nao e gerado; formato e envio estao definidos em %s\n", t, doc)
		return 2
	}
	known := false
	for _, k := range xbom.Types() {
		if k == t {
			known = true
		}
	}
	if !known {
		fmt.Fprintf(stderr, "aurumcode xbom: --type %q desconhecido (geraveis: %s; documentados: aibom, saasbom, netbom)\n", t, strings.Join(xbom.Types(), ", "))
		return exitUsageUnknownType
	}

	root := strings.TrimSpace(*repo)
	if root == "" {
		root = "."
	}
	policyDir := strings.TrimSpace(*politica)
	if policyDir == "" {
		policyDir = strings.TrimSpace(*policy)
	}
	if policyDir == "" {
		policyDir = strings.TrimSpace(os.Getenv("AURUMCODE_POLICY"))
	}

	effective, _, warnings, err := loadEffectiveConfig(root, policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode xbom: %v\n", err)
		return 2
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "aurumcode xbom: %s: %s\n", w.Provider, w.Reason)
	}
	minSpec := xbom.SpecVersion
	if effective.QualityGates.SsorDtrack != nil {
		g := effective.QualityGates.SsorDtrack.SBOMGenerator
		if err := g.Validate(); err != nil {
			fmt.Fprintf(stderr, "aurumcode xbom: %v\n", err)
			return 2
		}
		if v := strings.TrimSpace(g.SpecVersion); v != "" {
			minSpec = v
		}
	}

	res, err := xbom.LoadCatalog(t, root, policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode xbom: %v\n", err)
		return 2
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "aurumcode xbom: %s\n", w)
	}
	prompt, err := xbom.LoadPrompt(t, policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode xbom: %v\n", err)
		return 2
	}

	opts := xbom.Options{Type: t, Root: root, Catalog: res.Catalog, Prompt: prompt}
	provider, perr := selectProvider()
	switch {
	case perr == nil:
		opts.Provider = provider
		opts.Redact = redaction.FromEnv().Redact
	case errors.Is(perr, errNoProviderConfigured):
		// No provider: deterministic evidence only (llm=absent).
	default:
		fmt.Fprintf(stderr, "aurumcode xbom: %v\n", perr)
		return 2
	}

	gen, err := xbom.Generate(opts)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode xbom: %v\n", err)
		return 1
	}
	if gen.LLM.Err != nil {
		fmt.Fprintf(stderr, "aurumcode xbom: classificacao pelo modelo falhou, BOM so com evidencia deterministica: %v\n", gen.LLM.Err)
	}

	if err := writeXBOM(gen.JSON, *out, minSpec, stdout); err != nil {
		fmt.Fprintf(stderr, "aurumcode xbom: %v\n", err)
		return 1
	}
	fmt.Fprintf(stderr, "aurumcode xbom: %s: %d componente(s), %d descartado(s) sem evidencia\n", t, gen.Components, gen.Dropped.DroppedComponents)
	return 0
}

// writeXBOM validates data in a temporary file next to the destination (or
// in the temp dir for stdout) and publishes it only if it validates.
func writeXBOM(data []byte, outPath, minSpec string, stdout io.Writer) (err error) {
	dir := os.TempDir()
	if outPath != "" {
		dir = filepath.Dir(outPath)
	}
	tmp, err := os.CreateTemp(dir, ".xbom-*.tmp")
	if err != nil {
		return fmt.Errorf("escrevendo BOM: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil || outPath == "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("escrevendo BOM: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("escrevendo BOM: %w", err)
	}
	if err = xbom.ValidateBOM(tmpName, minSpec); err != nil {
		return err
	}
	if outPath == "" {
		_, err = stdout.Write(append(data, '\n'))
		return err
	}
	if err = os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, outPath)
}
