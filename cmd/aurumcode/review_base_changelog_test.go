package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/mcpserver"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const (
	wclRequired = "changelog_check:\n  mode: required\n"
	wclSuggest  = "changelog_check:\n  mode: suggest\n"
	wclGate     = "gate:\n  fail_on: [error]\n"
	wclHeading  = "### Required changelog"
	wclBaseLog  = "# Changelog\n\n## Unreleased\n\n- Entrada anterior que ja estava na base.\n"
	wclShort    = "- O retorno de Change passa a ser 42 para quem chama a funcao.\n"
)

// wclLongLine is a 300-character changelog line, over the 240 default.
var wclLongLine = "- " + strings.Repeat("a", 297) + "\n"

// wclRepo builds a real git repository: main holds config (the base's
// .aurumcode/config.yml, empty for none), the changelog and app.go; HEAD is
// a feature branch that changes app.go and adds entry under Unreleased. The
// fixture model answers with no finding, so only the changelog can block.
func wclRepo(t *testing.T, config, entry string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	prGit(t, dir, nil, "init", "-q", "-b", "main")
	write("app.go", "package demo\n\nfunc Change() int { return 1 }\n")
	write("CHANGELOG.md", wclBaseLog)
	if config != "" {
		write(".aurumcode/config.yml", config)
	}
	prGit(t, dir, nil, "add", "-A")
	prGit(t, dir, nil, "commit", "-q", "-m", "base")
	prGit(t, dir, nil, "checkout", "-q", "-b", "feature")
	write("app.go", "package demo\n\nfunc Change() int { return 42 }\n")
	write("CHANGELOG.md", strings.Replace(wclBaseLog, "## Unreleased\n\n", "## Unreleased\n\n"+entry, 1))
	prGit(t, dir, nil, "add", "-A")
	prGit(t, dir, nil, "commit", "-q", "-m", "feat: Change passa a retornar 42")
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_POLICY", "AURUMCODE_STANDARDS"} {
		t.Setenv(key, "")
	}
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"ok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	t.Cleanup(chdir(t, dir))
	return dir
}

// wclReview runs `review --base main` in the current directory.
func wclReview(t *testing.T, extra ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	args := append([]string{"--base", "main"}, extra...)
	code := runReviewWith(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter()}, args)
	return code, out.String(), errOut.String()
}

// A required changelog with a 300-character line closes the local gate,
// with or without a declared gate and without --fail-on: the check itself
// refuses, as the "Changelog obrigatório" workflow would.
func TestLocalGateRefusesTheEntryTheChangelogCheckRefuses(t *testing.T) {
	for name, cfg := range map[string]string{"sem gate": wclRequired, "com gate": wclRequired + wclGate} {
		t.Run(name, func(t *testing.T) {
			wclRepo(t, cfg, wclLongLine)
			code, out, errOut := wclReview(t)
			if code != gateExitFindings {
				t.Fatalf("exit=%d, want %d (refused changelog)\nstdout=%s\nstderr=%s", code, gateExitFindings, out, errOut)
			}
			for _, want := range []string{
				"CHANGELOG.md:3: [error] changelog: reprovado (entrada_longa)",
				wclHeading,
				"aurumcode changelog --base main",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("stdout lacks %q:\n%s", want, out)
				}
			}
			if !strings.Contains(out, "[!CAUTION]") {
				t.Errorf("the headline must block:\n%s", out)
			}
		})
	}
}

// A short entry passes and the report states the approved verdict.
func TestLocalGatePassesAValidEntryAndSaysSo(t *testing.T) {
	wclRepo(t, wclRequired+wclGate, wclShort)
	code, out, errOut := wclReview(t)
	if code != 0 {
		t.Fatalf("exit=%d, want 0\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, "changelog: aprovado (entrada_valida)") {
		t.Fatalf("the report must state the approved verdict:\n%s", out)
	}
}

// The suggest mode only observes: no finding, no failure, even with
// --fail-on at its lowest level; the report names what is wrong and the
// suggested entry.
func TestLocalGateSuggestModeOnlyObserves(t *testing.T) {
	wclRepo(t, wclSuggest, wclLongLine)
	code, out, errOut := wclReview(t, "--fail-on", "info")
	if code != 0 {
		t.Fatalf("exit=%d, want 0\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, "changelog: sem entrada útil (entrada_longa)") || !strings.Contains(out, "```markdown") {
		t.Fatalf("the report must observe the entry and suggest one:\n%s", out)
	}
	if strings.Contains(out, "[error] changelog") || strings.Contains(out, "[info] changelog") {
		t.Fatalf("the suggest mode must not publish a finding:\n%s", out)
	}
}

// Without a changelog_check section the same change is reviewed exactly as
// before; with the section it is refused (the contrast keeps this test
// meaningful when the check is missing).
func TestLocalGateWithoutTheSectionChangesNothing(t *testing.T) {
	wclRepo(t, "", wclLongLine)
	code, out, errOut := wclReview(t)
	if code != 0 || strings.Contains(out, wclHeading) || strings.Contains(errOut, "changelog:") {
		t.Fatalf("without the section nothing changes: exit=%d\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	wclRepo(t, wclRequired, wclLongLine)
	if code, out, _ := wclReview(t); code == 0 || !strings.Contains(out, "entrada_longa") {
		t.Fatalf("with the section the same change is refused: exit=%d\n%s", code, out)
	}
}

// The agent server's gate is the same session: the refused entry is a
// blocking finding of its answer.
func TestLocalGateAgentServerSeesTheChangelogRefusal(t *testing.T) {
	wclRepo(t, wclRequired, wclLongLine)
	g := &sessionGateway{filter: redaction.NewFilter()}
	o, err := g.Review(context.Background(), mcpserver.ReviewRequest{Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Exit == 0 {
		t.Fatalf("exit=0, want a closed gate\n%s\n%s", o.Report, o.Diagnostics)
	}
	found := false
	for _, f := range o.Blocking {
		found = found || (f.RuleID == "changelog/entrada_longa" && f.File == "CHANGELOG.md")
	}
	if !found {
		t.Fatalf("blocking findings lack the changelog refusal: %+v\n%s", o.Blocking, o.Report)
	}
}

// A check that cannot be established closes the gate without a breach
// (exit 1, as the command): a changelog the diff cannot read (binary) and a
// base configuration that does not parse, whatever mode the proposed change
// declares.
func TestLocalGateIndeterminateChangelogClosesTheGate(t *testing.T) {
	for name, tc := range map[string]struct {
		base, file, body string
	}{
		"changelog ilegível":      {wclRequired, "CHANGELOG.md", "# Changelog\x00\n\n## Unreleased\n\n" + wclShort},
		"config da base inválida": {"changelog_check:\n  mode: talvez\n", ".aurumcode/config.yml", wclRequired},
		// The proposed change cannot lower the check it is subject to.
		"base inválida, PR em suggest": {"changelog_check:\n  mode: talvez\n", ".aurumcode/config.yml", wclSuggest},
		"base inválida, PR sem seção":  {"changelog_check:\n  mode: talvez\n", ".aurumcode/config.yml", "review:\n  language: pt-BR\n"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := wclRepo(t, tc.base, wclShort)
			wclAmend(t, dir, tc.file, tc.body)
			code, out, errOut := wclReview(t)
			if code != gateExitBehavioral {
				t.Fatalf("exit=%d, want %d\nstdout=%s\nstderr=%s", code, gateExitBehavioral, out, errOut)
			}
			if !strings.Contains(out, "indeterminado") || !strings.Contains(out, "[error] changelog") {
				t.Fatalf("the report must name the indeterminate check as blocking:\n%s", out)
			}
		})
	}
}

// wclAmend writes name in dir and amends the current commit with it.
func wclAmend(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	prGit(t, dir, nil, "add", "-A")
	prGit(t, dir, nil, "commit", "-q", "--amend", "--no-edit")
}
