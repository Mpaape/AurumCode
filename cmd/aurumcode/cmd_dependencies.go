// The scheduled dependency scan: the default branch's whole tree, with no
// pull request, against the advisory source as it is today. Its result is a
// SARIF document of its own category for code scanning, whose alerts are the
// scan's only state: a result absent from the next conclusive upload is
// closed by code scanning. An inconclusive scan writes no document, so the
// open alerts stay as they are.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// dependencyScanCategory is the code scanning category of the scheduled
// scan, distinct from the pull request review's.
const dependencyScanCategory = "aurumcode/dependencies-scheduled"

// dependencyScanFingerprintVersion versions the scan's stable identity.
const dependencyScanFingerprintVersion = "dependency/v1"

type dependencyScanFlags struct {
	repo, politica, sarif, categoria *string
}

func newDependencyScanFlagSet() (*flag.FlagSet, dependencyScanFlags) {
	fs := flag.NewFlagSet("dependencies", flag.ContinueOnError)
	return fs, dependencyScanFlags{
		repo:      fs.String("repo", ".", "checkout of the default branch to scan"),
		politica:  fs.String("politica", "", "central policy directory (AURUMCODE_POLICY when empty)"),
		sarif:     fs.String("sarif", "", "SARIF output path (required); not written when the scan is inconclusive"),
		categoria: fs.String("categoria", dependencyScanCategory, "code scanning category of the results"),
	}
}

// dependencyScanDeps are the scan's collaborators; nil fields take
// production's.
type dependencyScanDeps struct {
	model     func() (dependencies.Completer, error)
	listFiles func(root string) ([]string, error)
	sources   dependencySources
	scanner   func(cfg *config.DependenciesConfig) dependencies.Extractor
	clock     func() time.Time
}

func runDependencyScan(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
	return runDependencyScanWith(args, stdout, stderr, filter, dependencyScanDeps{})
}

func runDependencyScanWith(args []string, stdout, stderr io.Writer, filter *redaction.Filter, deps dependencyScanDeps) int {
	fs, fl := newDependencyScanFlagSet()
	if exit, ok := parseSubcommandFlags("dependencies", fs, args, stdout, stderr); !ok {
		return exit
	}
	deps = deps.withDefaults()
	if filter == nil {
		filter = redaction.NewFilter()
	}
	root, out := strings.TrimSpace(*fl.repo), strings.TrimSpace(*fl.sarif)
	if out == "" {
		fmt.Fprintln(stderr, "aurumcode dependencies: --sarif is required")
		return 2
	}
	policyDir := strings.TrimSpace(*fl.politica)
	if policyDir == "" {
		policyDir = strings.TrimSpace(os.Getenv("AURUMCODE_POLICY"))
	}
	cfg, _, warnings, err := loadEffectiveConfig(root, policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode dependencies: %v\n", err)
		return 2
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "aurumcode dependencies: %s: %s\n", w.Provider, w.Reason)
	}
	report := scanDependencies(root, cfg.Dependencies, deps)
	if report.Inconclusive() {
		fmt.Fprintf(stderr, "aurumcode dependencies: varredura inconclusiva (%s): %s; nenhum SARIF gravado, os alertas abertos ficam como estao\n", report.Reason, filter.Redact(report.Detail))
		return 1
	}
	if err := render.WriteCategorizedSARIF(out, version, strings.TrimSpace(*fl.categoria), dependencySARIFFindings(report), filter); err != nil {
		fmt.Fprintf(stderr, "aurumcode dependencies: writing SARIF: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "aurumcode dependencies: %d manifesto(s), %d pacote(s), %d advisory(s)\n", len(report.Manifests), len(report.Changes), len(report.Findings))
	return 0
}

// scanDependencies lists the tree and runs the whole-tree scan.
func scanDependencies(root string, cfg *config.DependenciesConfig, deps dependencyScanDeps) dependencies.Report {
	paths, err := deps.listFiles(root)
	if err != nil {
		return dependencies.Report{Reason: dependencies.ReasonManifestsOmitted, Detail: "listagem dos arquivos: " + err.Error()}
	}
	model, err := deps.model()
	if err != nil {
		model = nil
	}
	return dependencies.Scan(context.Background(), dependencies.ScanInputs{
		Root:      root,
		Paths:     paths,
		ReadFile:  func(p string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(p))) },
		Model:     model,
		Source:    deps.sources.source(cfg, deps.clock),
		Extractor: deps.scanner(cfg),
	})
}

// dependencySARIFFindings is every advisory as a code scanning result. The
// identity (rule, manifest, ecosystem and package) never carries the
// version, the line, the severity or the time, so the same advisory on the
// same package keeps one alert across runs, and its absence closes it.
func dependencySARIFFindings(report dependencies.Report) []render.SARIFFinding {
	out := make([]render.SARIFFinding, 0, len(report.Findings))
	for _, f := range report.Findings {
		if f.Status == dependencies.StatusFixed {
			continue
		}
		out = append(out, render.SARIFFinding{
			RuleID:    "cve/" + f.Vuln.ID,
			RuleTitle: f.Vuln.Summary,
			Path:      f.Change.Manifest,
			Severity:  sarifSeverityOf(f.Vuln.Severity),
			Message:   dependencyAlertText(f),
			Context:   dependencyScanFingerprintVersion + "|" + f.Change.Ecosystem + "|" + f.Change.Name,
			Origin:    "dependencies",
		})
	}
	return out
}

// sarifSeverityOf maps an advisory severity onto the review's three levels;
// an unknown one is an error.
func sarifSeverityOf(severity string) string {
	switch dependencies.NormalizeSeverity(severity) {
	case "medium":
		return "warning"
	case "low":
		return "info"
	default:
		return "error"
	}
}

func dependencyAlertText(f dependencies.Finding) string {
	fixed := "sem versao corrigida"
	if len(f.Vuln.Fixed) > 0 {
		fixed = "corrigida em " + strings.Join(f.Vuln.Fixed, ", ")
	}
	return fmt.Sprintf("%s: %s %s (%s) em %s, severidade %s, %s, %s",
		strings.Join(f.Vuln.Identifiers(), "/"), f.Change.Name, f.Change.Head, f.Change.Ecosystem, f.Change.Manifest, f.Vuln.Severity, fixed, f.Vuln.Link)
}

// withDefaults fills production's collaborators: the provider from the
// environment, git ls-files, the OSV API and osv-scanner.
func (d dependencyScanDeps) withDefaults() dependencyScanDeps {
	if d.model == nil {
		d.model = func() (dependencies.Completer, error) {
			p, err := selectProvider()
			if err != nil {
				return nil, err
			}
			return llm.NewOrchestrator(p, nil, nil), nil
		}
	}
	if d.listFiles == nil {
		d.listFiles = gitTrackedFiles
	}
	if d.clock == nil {
		d.clock = time.Now
	}
	if d.sources.source == nil {
		d.sources.source = productionDependencySource
	}
	if d.scanner == nil {
		d.scanner = func(cfg *config.DependenciesConfig) dependencies.Extractor {
			return dependencies.OSVScanner{Binary: cfg.EffectiveScanner()}
		}
	}
	return d
}

// gitTrackedFiles lists the files git tracks under root.
func gitTrackedFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	data, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range strings.Split(string(data), "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}
