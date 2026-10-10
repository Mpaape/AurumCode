package main

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur600WarnReply is a model reply with one warning, below a gate whose
// threshold is error.
const aur600WarnReply = `{"summary":"ok","verdict":"comment","issues":[{"file":"app.go","line":2,"severity":"warning","rule_id":"quality/high-complexity","message":"Function is complex","evidence":"func Change() {","impact":"Hard to read","verification":"Split it"}]}`

const aur600Diff = "diff --git a/app.go b/app.go\n@@ -1,1 +1,2 @@\n package demo\n+func Change() {}\n"

// AC-001: with fail_on error and only warnings the gate passes, and the
// published document neither calls anything blocking nor requests changes;
// the warning is listed as non-blocking.
func TestAUR600WarningBelowGateIsNotBlocking(t *testing.T) {
	code, status, posted, errOut := aur567PR(t, aur600Diff, "gate:\n  fail_on: [error]\n", aur600WarnReply, prReviewOptions{})
	if code != 0 || status.State == "failure" {
		t.Fatalf("gate must pass: exit=%d status=%+v stderr=%s", code, status, errOut)
	}
	if !strings.HasPrefix(posted, "COMMENT\n") {
		t.Errorf("formal event must be COMMENT:\n%s", posted)
	}
	for _, banned := range []string{"Changes requested", "blocking finding(s)"} {
		if strings.Contains(posted, banned) {
			t.Errorf("document must not say %q when the gate passed:\n%s", banned, posted)
		}
	}
	for _, want := range []string{"[!NOTE]", "policy gate: passed", "### Observations (non-blocking)", "- `app.go:2` —"} {
		if !strings.Contains(posted, want) {
			t.Errorf("document must contain %q:\n%s", want, posted)
		}
	}
}

// AC-002: one finding the gate fails on is the one blocking finding, and
// the verdict requests changes, coherent with the failing gate; the warning
// below the threshold stays non-blocking.
func TestAUR600GateBreachIsBlocking(t *testing.T) {
	code, status, posted, errOut := aur567PR(t, aur567PRSecretDiff, "gate:\n  fail_on: [error]\n", aur600WarnReply, prReviewOptions{})
	if code != exitFindings || status.State != "failure" {
		t.Fatalf("gate must fail: exit=%d status=%+v stderr=%s", code, status, errOut)
	}
	for _, want := range []string{"REQUEST_CHANGES\n", "[!CAUTION]", "Blocked: 1 problem must be fixed", "### Fix before merge", "### Observations (non-blocking)"} {
		if !strings.Contains(posted, want) {
			t.Errorf("document must contain %q:\n%s", want, posted)
		}
	}
}

// AC-003: without a declared gate the historical text stays: a warning is
// blocking and requests changes, and no finding carries a gate label.
func TestAUR600WithoutGateKeepsHistoricalText(t *testing.T) {
	_, _, posted, _ := aur567PR(t, aur600Diff, "review: {}\n", aur600WarnReply, prReviewOptions{})
	for _, want := range []string{"REQUEST_CHANGES\n", "[!CAUTION]", "Blocked: 1 problem must be fixed", "### Fix before merge"} {
		if !strings.Contains(posted, want) {
			t.Errorf("document must contain %q:\n%s", want, posted)
		}
	}
	if strings.Contains(posted, "non-blocking") {
		t.Errorf("no gate, no gate label:\n%s", posted)
	}
	result := &types.ReviewResult{Issues: []types.ReviewIssue{{File: "a.go", Line: 1, Severity: "warning", Message: "m"}}}
	if formatGatedReviewBody(result, nil, "pt-BR", false, "", blocking.Ungated()) != formatPublishedReviewBody(result, nil, "pt-BR", false, "") {
		t.Error("the ungated rule must render the historical document")
	}
}

// AC-004: a CI status whose every item was discarded says, in one line,
// that nothing failed in this run, in every catalog language.
func TestAUR600CIStatusAllDiscardedShowsOneLine(t *testing.T) {
	result := &types.ReviewResult{Metadata: map[string]string{ciStatusDiscardedKey: "2"}}
	for language, want := range map[string]string{
		"en-US": "### CI status\n\nNothing failed in this run: 2 status item(s)",
		"pt-BR": "### Status do CI\n\nNada falhou nesta execução: 2 itens",
	} {
		body := formatReviewDocument(result, nil, language, blocking.Ungated())
		if !strings.Contains(body, want) {
			t.Errorf("%s: want %q in:\n%s", language, want, body)
		}
	}
	if body := formatReviewDocument(&types.ReviewResult{}, nil, "en-US", blocking.Ungated()); strings.Contains(body, "CI status") {
		t.Errorf("nothing discarded and nothing kept: no CI section:\n%s", body)
	}
}

// The local --base report follows the same gate alignment as the formal
// review event.
func TestAUR600LocalVerdictFollowsGate(t *testing.T) {
	passing := blocking.FromGate(true, gateDecision{Active: true})
	failing := blocking.FromGate(true, gateDecision{Active: true, Fail: true})
	for _, tc := range []struct {
		rule          blocking.Rule
		verdict, want string
	}{
		{passing, "changes_requested", "comment"},
		{passing, "approve", "approve"},
		{failing, "comment", "changes_requested"},
		{blocking.Ungated(), "changes_requested", "changes_requested"},
	} {
		if got := gateRuleVerdict(tc.rule, tc.verdict); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.verdict, got, tc.want)
		}
	}
}
