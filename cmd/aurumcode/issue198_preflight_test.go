package main

// #198 review: the --limite pre-flight prices what the review sends -- the
// assembled prompt with its codebase context, once per analyst pass when
// analysts are selected -- and the printed figure is that count.

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/llm/tokens"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

var issue198PreflightRe = regexp.MustCompile(`estimated cost \$([0-9]+\.[0-9]{4}), pre-flight of the assembled prompt(?: sent once per analyst, ([0-9]+) model passes)? \(about ([0-9]+) tokens`)

// issue198Preflight runs the --pr review of a change whose symbol another
// file uses (the codebase context) under --limite, and returns the
// pre-flight's dollars, passes and tokens with the prompt the model got.
func issue198Preflight(t *testing.T, configYAML string) (usd float64, passes, promptTokens int, prompt string) {
	t.Helper()
	_, head := issue198Repo(t, map[string]string{
		"app.go": "package demo\n\nfunc " + issue198Symbol + "() {}\n",
		"use.go": "package demo\n\nvar _ = " + issue198Symbol + "\n",
	})
	t.Setenv(envCostInputPer1K, "0.003")
	t.Setenv(envCostOutputPer1K, "0.015")
	diff := "diff --git a/app.go b/app.go\n@@ -1,2 +1,3 @@\n package demo\n+func " + issue198Symbol + "() {}\n"
	capture := aur515Env(t, issue198Server(t, head, diff, configYAML).URL)
	var out, errOut strings.Builder
	code := runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, publicationSet: true, publication: "review", limiteSet: true, limite: "5"})
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out.String(), errOut.String())
	}
	m := issue198PreflightRe.FindStringSubmatch(errOut.String())
	if m == nil {
		t.Fatalf("no pre-flight line:\n%s", errOut.String())
	}
	usd, _ = strconv.ParseFloat(m[1], 64)
	passes = 1
	if m[2] != "" {
		passes, _ = strconv.Atoi(m[2])
	}
	promptTokens, _ = strconv.Atoi(m[3])
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "use.go") {
		t.Fatalf("the codebase context is not in the prompt, the test proves nothing:\n%.3000s", data)
	}
	return usd, passes, promptTokens, string(data)
}

func issue198Near(got, want int) bool {
	return math.Abs(float64(got-want)) <= math.Max(3, float64(want)/100)
}

func TestIssue198PreflightCountsTheContextAndEveryAnalystPass(t *testing.T) {
	_, passes, single, prompt := issue198Preflight(t, "")
	body := tokens.Estimate(prompt)
	// The priced tokens are the provider's, at the 3.5 characters per token
	// the declared window is shared by (review.ProviderTokens), not the
	// product's four.
	if atRatio := len(prompt) * 2 / 7; passes != 1 || !issue198Near(single, atRatio) {
		t.Fatalf("single review: %d pass(es), about %d tokens printed; the prompt sent, with its context, is %d characters, %d tokens at 3.5 characters each", passes, single, len(prompt), atRatio)
	}

	usd, passes, both, last := issue198Preflight(t, "review:\n  profiles: [seguranca, solid]\n")
	res, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"seguranca", "solid"}}, reviewprofile.EmptyTeam())
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for _, p := range res.Profiles {
		prefix := profileProvider{profile: p}.prefix()
		if p.Name == res.Profiles[len(res.Profiles)-1].Name && !strings.HasPrefix(last, prefix) {
			t.Fatalf("the last pass did not send %s's prefix, the expected count is wrong:\n%.500s", p.Name, last)
		}
		want += body + tokens.Estimate(prefix)
	}
	want = review.ProviderTokens(want)
	if passes != 2 || !issue198Near(both, want) {
		t.Fatalf("two analysts: %d pass(es), about %d tokens printed; the two passes send about %d", passes, both, want)
	}
	price := cost.PriceMap{InputPer1K: 0.003, OutputPer1K: 0.015}
	if wantUSD := estimateCostUSD(both, 2, price); math.Abs(usd-wantUSD) > 0.00005 {
		t.Fatalf("printed $%.4f for %d tokens and two passes, which price at $%.4f", usd, both, wantUSD)
	}
}

// A pre-flight that cannot count says so, and why, in one line, and prints
// no figure: here batches.max_prompt_tokens admits no batch of a review the
// declared window splits.
func TestIssue198PreflightThatCannotCountPrintsNoFigure(t *testing.T) {
	_, head := issue198Repo(t, map[string]string{"a/a.go": "package a\n", "b/b.go": "package b\n"})
	t.Setenv(envCostInputPer1K, "0.003")
	t.Setenv(envCostOutputPer1K, "0.015")
	t.Setenv("AURUMCODE_MODEL_WINDOW_TOKENS", issue198Window)
	var diff strings.Builder
	for _, dir := range []string{"a", "b"} {
		name := dir + "/" + dir + ".go"
		diff.WriteString("diff --git a/" + name + " b/" + name + "\n@@ -1,1 +1,601 @@\n package " + dir + "\n")
		for i := 0; i < issue198Lines; i++ {
			diff.WriteString("+var value" + dir + strconv.Itoa(i) + " = \"alpha beta gamma delta epsilon zeta eta theta\"\n")
		}
	}
	aur515Env(t, issue198Server(t, head, diff.String(), "batches:\n  max_prompt_tokens: 10\n").URL)
	var out, errOut strings.Builder
	runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, publicationSet: true, publication: "review", limiteSet: true, limite: "5"})
	stderr := errOut.String()
	if strings.Contains(stderr, "estimated cost $") {
		t.Fatalf("a figure was printed for an estimate that did not exist:\n%s", stderr)
	}
	if !strings.Contains(stderr, "cost not estimated before the call: ") || !strings.Contains(stderr, "batches.max_prompt_tokens allows 10") {
		t.Fatalf("the pre-flight does not say it could not estimate, and why:\n%s", stderr)
	}
}
