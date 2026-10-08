package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

var aur572Commands = []string{"review", "fix", "sbom", "sign", "xbom", "dependencies", "mcp", "changelog", "realimentacao"}

// AC-001: the top-level help lists every command of the registry, and each
// `<sub> --help` prints every flag its FlagSet declares plus an example.
func TestAUR572TopLevelHelpListsEverySubcommand(t *testing.T) {
	var out bytes.Buffer
	printTopLevelHelp(&out)
	for _, name := range aur572Commands {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Errorf("top-level help lacks a line for %q:\n%s", name, out.String())
		}
	}
	if got := len(subcommands()); got != len(aur572Commands) {
		t.Errorf("registry has %d commands, want %d", got, len(aur572Commands))
	}
}

func TestAUR572SubcommandHelpListsEveryDeclaredFlag(t *testing.T) {
	for _, sc := range subcommands() {
		var stdout, stderr bytes.Buffer
		if code := sc.run([]string{"--help"}, &stdout, &stderr, redaction.NewFilter()); code != 0 {
			t.Fatalf("%s --help exit %d", sc.name, code)
		}
		if stderr.Len() != 0 {
			t.Errorf("%s --help wrote stderr: %q", sc.name, stderr.String())
		}
		n := 0
		sc.flags().VisitAll(func(f *flag.Flag) {
			n++
			if !strings.Contains(stdout.String(), "\n  -"+f.Name) {
				t.Errorf("%s --help lacks declared flag -%s:\n%s", sc.name, f.Name, stdout.String())
			}
		})
		if n == 0 {
			t.Errorf("%s declares no flags", sc.name)
		}
		for _, want := range []string{"usage: aurumcode " + sc.name, "Example:", sc.example, "Configuração e referência: docs/configuration.md (seção " + sc.docSection + ")"} {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("%s --help lacks %q:\n%s", sc.name, want, stdout.String())
			}
		}
	}
}

// AC-002: no findings and nothing inconclusive keeps "No issues found.";
// any inconclusive source replaces it with a sentence naming it.
func TestAUR572PrintFindingsNeverClaimsCleanWhenInconclusive(t *testing.T) {
	var clean bytes.Buffer
	printFindings(&clean, &types.ReviewResult{}, "", "")
	if clean.String() != "No issues found.\n" {
		t.Errorf("clean review printed %q", clean.String())
	}
	for _, reason := range []string{"sast_execution_error", "partial_coverage", "dtrack_unreachable", "analysis_data_stale", "provider_failure"} {
		var out bytes.Buffer
		printFindings(&out, &types.ReviewResult{}, reason, "")
		if strings.Contains(out.String(), "No issues found.") || !strings.Contains(out.String(), reason) {
			t.Errorf("inconclusive %q printed %q", reason, out.String())
		}
	}
}

func aur572CleanModel(t *testing.T) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
}

// AC-002 end to end: a SAST that did not conclude, over a model that found
// nothing, does not print "No issues found."; a SAST that concluded clean
// does.
func TestAUR572InconclusiveSASTReportNeverSaysNoIssuesFound(t *testing.T) {
	// inconclusive: warn is written so the run completes and the report's
	// closing line is what is under test; without it the run blocks.
	cleanFixture(t, "gate:\n  inconclusive: warn\nquality_gates:\n  sast:\n    enabled: true\n")
	aur572CleanModel(t)
	setSemgrepPATH(t, semgrepFake(t, "boom: semgrep crashed", true, ""))

	var out, errOut strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter()); code != 0 {
		t.Fatalf("exit=%d; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if strings.Contains(out.String(), "No issues found.") {
		t.Fatalf("inconclusive SAST still reported a clean review:\n%s", out.String())
	}
	last := out.String()[strings.LastIndex(strings.TrimRight(out.String(), "\n"), "\n")+1:]
	if !strings.HasPrefix(last, "Sem achados nas fontes concluídas; inconclusivo: sast_") {
		t.Fatalf("closing line does not name the inconclusive source: %q\n%s", last, out.String())
	}
}

func TestAUR572ConclusiveCleanReportKeepsNoIssuesFound(t *testing.T) {
	cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n")
	aur572CleanModel(t)
	setSemgrepPATH(t, semgrepFake(t, semgrepClean, false, ""))

	var out, errOut strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter()); code != 0 {
		t.Fatalf("exit=%d; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.HasSuffix(out.String(), "\nNo issues found.\n") && out.String() != "No issues found.\n" {
		t.Fatalf("clean review does not end with No issues found.:\n%s", out.String())
	}
}
