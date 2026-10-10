package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
	"gopkg.in/yaml.v3"
)

const (
	aur610BotsOff      = aur509Required + "  bots: off\n"
	aur610BotsRequired = aur509Required + "  bots: required\n"
	aur610BotLine      = "changelog: autor é bot (dependabot[bot]); changelog_check.bots: suggest — a PR não é reprovada\n"
)

// aur610Dependabot are the author flags the workflow passes for a
// Dependabot pull request.
var aur610Dependabot = []string{"--autor", "dependabot[bot]", "--tipo-autor", "Bot"}

// aur610Result is everything one check run leaves behind.
type aur610Result struct {
	code                 int
	out, errOut, summary string
}

// aur610Run runs the check, without a model, on a PR whose diff is files
// in a repository whose config.yml says cfg, with the given extra flags.
func aur610Run(t *testing.T, cfg string, files []types.DiffFile, extra ...string) aur610Result {
	t.Helper()
	aur602NoModel(t)
	t.Setenv("AURUMCODE_POLICY", "")
	summary := filepath.Join(t.TempDir(), "step-summary.md")
	t.Setenv(stepSummaryEnv, summary)
	root := aur509Repo(t, cfg)
	deps := changelogDeps{
		differ:   aur509Differ(files, nil, nil),
		commits:  func(string, string, string) ([]changelog.Commit, error) { return aur604Commits, nil },
		provider: changelogProviderFromEnv,
		filter:   redaction.NewFilter(),
	}
	var stdout, stderr bytes.Buffer
	code := runChangelogWith(append([]string{"--base", "base-sha", "--repo", root}, extra...), &stdout, &stderr, deps)
	data, err := os.ReadFile(summary)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return aur610Result{code: code, out: stdout.String(), errOut: stderr.String(), summary: string(data)}
}

// aur610CodeOnly is a PR that changes only a workflow file, like a
// Dependabot bump: no changelog entry.
var aur610CodeOnly = []types.DiffFile{{Path: ".github/workflows/ci.yml"}}

// AC-001: a bot's PR under mode required, with bots at its default, passes
// with the line that says why, and still gets the suggested entry in the
// log and in the job summary. Either event signal alone marks the bot, and
// a central policy's section (bots absent = suggest) decides alone.
func TestAUR610AC001BotPassesWithTheReason(t *testing.T) {
	r := aur610Run(t, aur509Required, aur610CodeOnly, aur610Dependabot...)
	if r.code != 0 {
		t.Fatalf("bot PR failed under the default bots policy: exit %d\nout %q\nstderr %q", r.code, r.out, r.errOut)
	}
	if !strings.HasPrefix(r.out, aur610BotLine) {
		t.Fatalf("the explanation is not the first line:\n%s", r.out)
	}
	for _, want := range []string{"sem entrada útil (entrada_ausente)", "modo suggest, não reprova", "entrada sugerida (fonte: commits)", "- o relatorio aceita filtro por periodo"} {
		if !strings.Contains(r.out, want) {
			t.Fatalf("bot output lacks %q:\n%s", want, r.out)
		}
	}
	if strings.Contains(r.out, "reprovado") {
		t.Fatalf("bot output reports a refusal:\n%s", r.out)
	}
	if !strings.Contains(r.summary, "Sugestão (não reprova a PR)") {
		t.Fatalf("job summary lacks the suggest wording:\n%s", r.summary)
	}
	for name, flags := range map[string][]string{
		"login only":       {"--autor", "renovate[bot]"},
		"type only":        {"--tipo-autor", "Bot"},
		"login typed User": {"--autor", "renovate[bot]", "--tipo-autor", "User"},
	} {
		if r := aur610Run(t, aur509Required, aur610CodeOnly, flags...); r.code != 0 || !strings.Contains(r.out, "changelog_check.bots: suggest") {
			t.Fatalf("%s: exit %d out %q", name, r.code, r.out)
		}
	}
	policy := aur509Repo(t, aur509Required)
	root := aur509Repo(t, aur610BotsRequired)
	var stdout, stderr bytes.Buffer
	code := runChangelog(append([]string{"--base", "base-sha", "--repo", root, "--politica", policy}, aur610Dependabot...), &stdout, &stderr, aur509Differ(aur610CodeOnly, nil, nil))
	if code != 0 || !strings.Contains(stdout.String(), "changelog_check.bots: suggest") {
		t.Fatalf("the central policy's section did not decide alone: exit %d out %q stderr %q", code, stdout.String(), stderr.String())
	}
}

// AC-002: bots off skips the check for a bot; bots required refuses the bot
// exactly like a person.
func TestAUR610AC002BotsOffSkipsAndRequiredRefuses(t *testing.T) {
	off := aur610Run(t, aur610BotsOff, aur610CodeOnly, aur610Dependabot...)
	if off.code != 0 || off.out != "changelog: não exigido: autor é bot (dependabot[bot]); changelog_check.bots: off\n" || off.summary != "" {
		t.Fatalf("bots off: exit %d\nout %q\nsummary %q", off.code, off.out, off.summary)
	}
	bot := aur610Run(t, aur610BotsRequired, aur610CodeOnly, aur610Dependabot...)
	person := aur610Run(t, aur610BotsRequired, aur610CodeOnly)
	if bot.code != exitChangelogRefused || !strings.HasPrefix(bot.out, "changelog: reprovado (entrada_ausente)") {
		t.Fatalf("bots required: exit %d out %q", bot.code, bot.out)
	}
	if bot != person {
		t.Fatalf("bots required must treat the bot as a person:\nbot    %+v\nperson %+v", bot, person)
	}
}

// AC-003: a person's run is byte-identical with or without author flags,
// in every mode and verdict, and whatever bots says.
func TestAUR610AC003HumanOutputUnchanged(t *testing.T) {
	valid := []types.DiffFile{analyzer.BuildDiffFile("CHANGELOG.md", aur509Changelog, aur509Entry)}
	cases := []struct {
		name  string
		cfg   string
		files []types.DiffFile
	}{
		{"required missing", aur509Required, aur610CodeOnly},
		{"required missing bots off", aur610BotsOff, aur610CodeOnly},
		{"required valid", aur509Required, valid},
		{"suggest missing", aur604Suggest, aur610CodeOnly},
		{"off", aur604Off, aur610CodeOnly},
		{"absent", "", aur610CodeOnly},
	}
	people := [][]string{
		{"--autor", "paape", "--tipo-autor", "User"},
		{"--autor", "bot-lover"},
		{"--autor", "x[bot]y", "--tipo-autor", "User"},
	}
	for _, c := range cases {
		before := aur610Run(t, c.cfg, c.files)
		for _, flags := range people {
			if after := aur610Run(t, c.cfg, c.files, flags...); after != before {
				t.Fatalf("%s %v: a person's output changed:\nwithout flags %+v\nwith flags    %+v", c.name, flags, before, after)
			}
		}
	}
}

// AC-004: without author flags (or with empty ones) the author is a person:
// mode required refuses as before.
func TestAUR610AC004NoAuthorFlagsIsAPerson(t *testing.T) {
	for name, flags := range map[string][]string{"absent": nil, "empty": {"--autor", "", "--tipo-autor", ""}} {
		r := aur610Run(t, aur509Required, aur610CodeOnly, flags...)
		if r.code != exitChangelogRefused || !strings.HasPrefix(r.out, "changelog: reprovado (entrada_ausente)") || strings.Contains(r.out, "autor é bot") {
			t.Fatalf("%s: exit %d out %q", name, r.code, r.out)
		}
	}
}

// AC-001 in the review body: the suggestion block follows the mode for the
// PR's author, read from the same metadata the review already fetches.
func TestAUR610ReviewBodyUsesTheAuthorsMode(t *testing.T) {
	required := aur509Repo(t, aur509Required)
	_, errOut, body := aur610PR(t, `{"login":"dependabot[bot]","type":"Bot"}`, "--politica", required)
	if !strings.Contains(body, "Suggestion only (the pull request does not fail)") || strings.Contains(body, "requires a changelog entry") {
		t.Fatalf("bot PR under required: review body lacks the suggest wording:\n%s\nstderr: %s", body, errOut)
	}
	_, errOut, body = aur610PR(t, `{"login":"paape","type":"User"}`, "--politica", required)
	if !strings.Contains(body, "requires a changelog entry") {
		t.Fatalf("person PR under required lost the required wording:\n%s\nstderr: %s", body, errOut)
	}
	off := aur509Repo(t, aur610BotsOff)
	_, errOut, body = aur610PR(t, `{"login":"dependabot[bot]","type":"Bot"}`, "--politica", off)
	if strings.Contains(body, "Suggested changelog entry") {
		t.Fatalf("bots off, yet the review body carries a changelog block:\n%s\nstderr: %s", body, errOut)
	}
}

// aur610PR is one --pr review against the AUR-602 fixture whose metadata
// names the author user (a JSON object).
func aur610PR(t *testing.T, user string, extra ...string) (int, string, string) {
	t.Helper()
	var posted githubclient.PullRequestReview
	inner := aur602Fixture(t, "", &posted)
	t.Cleanup(inner.Close)
	target, err := url.Parse(inner.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7" && !strings.Contains(r.Header.Get("Accept"), "diff") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"title":"build(deps): bump the actions group","body":"Bumps the actions group.","user":%s}`, user)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer server.Close()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"Nothing to report."}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "new-head")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	t.Setenv("AURUMCODE_OUTPUT_FILE", "")
	t.Setenv("AURUMCODE_POLICY", "")
	args := append([]string{"--pr", "7", "--repo", "team/project", "--publicar", "--modo-publicacao", "review"}, extra...)
	var stdout, stderr strings.Builder
	code := runReviewWith(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter(), deps: reviewDeps{}}, args)
	return code, stderr.String(), posted.Body
}

// aur610Edge resolves a file of the repository outside the Go module; a
// module-only acceptance (AURUMCODE_MODULE_ONLY=1) skips.
func aur610Edge(t *testing.T, rel string) string {
	t.Helper()
	root := filepath.Join("..", "..")
	skip, err := manifestCheckSkipped(root, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Skip("AURUMCODE_MODULE_ONLY=1: the acceptance staged only the Go module")
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// AC-001 at the edge: the workflow reads the author from the event into the
// step's environment, hands it to the container, and keeps the job without
// an if: (a skipped job is green in a required check).
func TestAUR610WorkflowHandsTheAuthorToTheCheck(t *testing.T) {
	data, err := os.ReadFile(aur610Edge(t, ".github/workflows/changelog.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				Name string            `yaml:"name"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	job, ok := wf.Jobs["changelog"]
	if !ok || job.If != "" {
		t.Fatalf("changelog job missing or conditional (if: %q)", job.If)
	}
	want := map[string]string{"PR_AUTHOR_LOGIN": "${{ github.event.pull_request.user.login }}", "PR_AUTHOR_TYPE": "${{ github.event.pull_request.user.type }}"}
	found := false
	for _, s := range job.Steps {
		if s.Name != "Check the changelog entry" {
			continue
		}
		found = true
		for k, v := range want {
			if s.Env[k] != v {
				t.Fatalf("step env %s = %q, want %q", k, s.Env[k], v)
			}
		}
	}
	if !found {
		t.Fatal("check step not found")
	}
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".aurumcode-tool", "scripts", "ci"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".aurumcode-tool", "scripts", "ci", "changelog-check.sh"), []byte("exit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	argsFile := filepath.Join(ws, "docker-args")
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+argsFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "step.sh")
	if err := os.WriteFile(script, []byte(aur602WorkflowRun(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-e", script)
	cmd.Dir = ws
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SAME_REPOSITORY=true", "GITHUB_WORKSPACE="+ws, "GITHUB_STEP_SUMMARY="+filepath.Join(ws, "summary.md"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("workflow step: %v\n%s", err, out)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-e\nPR_AUTHOR_LOGIN\n", "-e\nPR_AUTHOR_TYPE\n"} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("docker run lacks %q:\n%s", want, args)
		}
	}
}

// AC-001/AC-004 at the edge: the script turns PR_AUTHOR_LOGIN and
// PR_AUTHOR_TYPE into --autor and --tipo-autor, with or without a policy,
// and passes no author flag when they are absent (a person).
func TestAUR610ScriptPassesTheAuthorFlags(t *testing.T) {
	checker := aur610Edge(t, "scripts/ci/changelog-check.sh")
	bin := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "aurumcode-args")
	fakeGo := "#!/bin/sh\nwhile [ $# -gt 0 ]; do\n  if [ \"$1\" = -o ]; then out=\"$2\"; fi\n  shift\ndone\n" +
		"printf '#!/bin/sh\\nprintf \"%%s\\\\n\" \"$@\" > " + argsFile + "\\n' > \"$out\"\nchmod +x \"$out\"\n"
	for name, body := range map[string]string{"go": fakeGo, "git": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	base := []string{"changelog", "--base", "b", "--head", "h", "--repo", "/w/target"}
	cases := []struct {
		name string
		env  []string
		want []string
	}{
		{"bot", []string{"PR_AUTHOR_LOGIN=dependabot[bot]", "PR_AUTHOR_TYPE=Bot", "POLICY_DIR="}, append(append([]string{}, base...), "--autor", "dependabot[bot]", "--tipo-autor", "Bot")},
		{"bot with policy", []string{"PR_AUTHOR_LOGIN=dependabot[bot]", "PR_AUTHOR_TYPE=Bot", "POLICY_DIR=/w/policy"}, append(append([]string{}, base...), "--autor", "dependabot[bot]", "--tipo-autor", "Bot", "--politica", "/w/policy")},
		{"absent", []string{"PR_AUTHOR_LOGIN=", "PR_AUTHOR_TYPE=", "POLICY_DIR="}, base},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_ = os.Remove(argsFile)
			cmd := exec.Command("sh", checker, "/w/target")
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"HOME="+t.TempDir(), "TMPDIR="+t.TempDir(), "BASE_SHA=b", "HEAD_SHA=h")
			cmd.Env = append(cmd.Env, c.env...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("checker: %v\n%s", err, out)
			}
			got, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatal(err)
			}
			if want := strings.Join(c.want, "\n") + "\n"; string(got) != want {
				t.Fatalf("aurumcode args:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}
