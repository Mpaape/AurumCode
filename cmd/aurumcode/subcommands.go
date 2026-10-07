package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// subcommand is one entry of the single registry of aurumcode's commands
// (AUR-572). The dispatcher in run, the top-level `--help` and every
// `<sub> --help` read this one list, so a command cannot exist without a
// help line and the help of a command cannot list flags its FlagSet does
// not declare: the flags are printed from the very FlagSet constructor the
// command parses with.
type subcommand struct {
	name    string
	summary string
	example string
	// docSection is the heading of docs/configuration.md that documents the
	// command; the help footer is generated from it.
	docSection string
	// flags builds a fresh FlagSet with every flag the command declares.
	flags func() *flag.FlagSet
	run   func(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int
}

// subcommands lists the commands in the order the help prints them. It is a
// function, not a package variable, because the run functions reach back
// into printSubcommandHelp: a variable would be an initialization cycle.
func subcommands() []subcommand {
	return []subcommand{
		{
			name:       "review",
			docSection: "CLI `aurumcode review`",
			summary:    "Review a git diff with the configured model and publish or gate the findings.",
			example:    "aurumcode review --base HEAD~1",
			flags:      newReviewFlagSet,
			run:        runReview,
		},
		{
			name:       "fix",
			docSection: "CLI `aurumcode fix`",
			summary:    "Turn review suggestions into an applyable unified diff (one-click fix).",
			example:    "aurumcode fix --file suggestions.json > fix.patch",
			flags:      func() *flag.FlagSet { fs, _ := newFixFlagSet(); return fs },
			run: func(args []string, stdout, stderr io.Writer, _ *redaction.Filter) int {
				return runFix(args, stdout, stderr)
			},
		},
		{
			name:       "sbom",
			docSection: "SBOM CycloneDX com Trivy (AUR-549)",
			summary:    "Generate an OWASP CycloneDX SBOM with Trivy for the repository and, optionally, an image.",
			example:    "aurumcode sbom --repo . --imagem example.test/app:1.0",
			flags:      func() *flag.FlagSet { fs, _ := newSBOMFlagSet(); return fs },
			run: func(args []string, stdout, stderr io.Writer, _ *redaction.Filter) int {
				return runSBOM(args, stdout, stderr)
			},
		},
		{
			name:       "sign",
			docSection: "CLI `aurumcode sign`",
			summary:    "Sign the SBOM and/or the artifact image with Sigstore/Cosign.",
			example:    "aurumcode sign --repo . --sbom sbom.cdx.json",
			flags:      func() *flag.FlagSet { fs, _ := newSignFlagSet(); return fs },
			run: func(args []string, stdout, stderr io.Writer, _ *redaction.Filter) int {
				return runSign(args, stdout, stderr)
			},
		},
		{
			name:       "xbom",
			docSection: "xBOM além do SBOM: Build BOM e CBOM (AUR-552)",
			summary:    "Generate a CycloneDX 1.6 Build BOM or CBOM whose components cite file and line.",
			example:    "aurumcode xbom --type build --repo . --out build-bom.cdx.json",
			flags:      func() *flag.FlagSet { fs, _ := newXBOMFlagSet(); return fs },
			run: func(args []string, stdout, stderr io.Writer, _ *redaction.Filter) int {
				return runXBOM(args, stdout, stderr)
			},
		},
		{
			name:       "dependencies",
			docSection: "Varredura agendada de dependências (`aurumcode dependencies`)",
			summary:    "Scan the default branch's dependencies against OSV and write a SARIF for code scanning.",
			example:    "aurumcode dependencies --repo . --sarif dependencies.sarif",
			flags:      func() *flag.FlagSet { fs, _ := newDependencyScanFlagSet(); return fs },
			run:        runDependencyScan,
		},
		mcpSubcommand(),
		changelogSubcommand(),
		feedbackSubcommand(),
	}
}

func findSubcommand(name string) (subcommand, bool) {
	for _, sc := range subcommands() {
		if sc.name == name {
			return sc, true
		}
	}
	return subcommand{}, false
}

// printTopLevelHelp is what makes `aurumcode --help` (or `-h`, or `help`)
// answer "what does this command do" without the reader ever opening
// source: every command with one line, and one runnable example each.
// MUT-001 (docs/specs/AUR-443.md, tests/acceptance/AUR-443.sh) removes the
// "--help", "-h", "help" case in run so this function becomes unreachable
// -- `aurumcode --help` then falls through to the `default` arm and reports
// "unknown command", exit 2.
func printTopLevelHelp(stdout io.Writer) {
	fmt.Fprintln(stdout, "usage: aurumcode <command> [flags]")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Commands:")
	for _, sc := range subcommands() {
		fmt.Fprintf(stdout, "  %-8s %s\n", sc.name, sc.summary)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, `Run "aurumcode <command> --help" for that command's own flags.`)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Examples:")
	for _, sc := range subcommands() {
		fmt.Fprintf(stdout, "  %s\n", sc.example)
	}
}

// printSubcommandHelp prints one command's help: usage, summary, every flag
// of its FlagSet and an example.
func printSubcommandHelp(stdout io.Writer, name string) {
	sc, ok := findSubcommand(name)
	if !ok {
		return
	}
	fs := sc.flags()
	fs.SetOutput(stdout)
	fmt.Fprintf(stdout, "usage: aurumcode %s [flags]\n\n%s\n\nFlags:\n", sc.name, sc.summary)
	fs.PrintDefaults()
	fmt.Fprintf(stdout, "\nExample:\n  %s\n", sc.example)
	fmt.Fprintf(stdout, "\nConfiguração e referência: docs/configuration.md (seção %s)\n", sc.docSection)
}

// parseSubcommandFlags parses args with fs. ok is false when the command
// must return exit now: an explicitly requested --help is a fulfilled
// request (stdout, exit 0, AUR-443); a genuine usage error goes to stderr,
// exit 2.
func parseSubcommandFlags(name string, fs *flag.FlagSet, args []string, stdout, stderr io.Writer) (exit int, ok bool) {
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	err := fs.Parse(args)
	switch {
	case err == nil:
		return 0, true
	case errors.Is(err, flag.ErrHelp):
		printSubcommandHelp(stdout, name)
		return 0, false
	}
	io.Copy(stderr, &buf) //nolint:errcheck // best-effort; nothing left to report to on failure
	return 2, false
}
