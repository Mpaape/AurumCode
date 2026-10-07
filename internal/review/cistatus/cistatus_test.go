package cistatus

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

const ownPrefix = "aurumcode/"

// pr96Context is the shape measured on PR #96: a stale concluded run, the
// current run still in progress, the review job itself running and the
// statuses this product published on the previous round.
const pr96Context = `[
{"name":"Build and test in OCI","state":"SUCCESS","workflow":"CI","link":"https://example.test/1"},
{"name":"Build and test in OCI","state":"IN_PROGRESS","workflow":"CI","link":"https://example.test/2"},
{"name":"Race tests in OCI","state":"IN_PROGRESS","workflow":"CI","link":"https://example.test/3"},
{"name":"Documentation browser checks","state":"IN_PROGRESS","workflow":"CI","link":"https://example.test/4"},
{"name":"review / Review pull request","state":"IN_PROGRESS","workflow":"AurumCode self review","link":"https://example.test/5"},
{"name":"Lint","state":"FAILURE","workflow":"CI","link":"https://example.test/6"},
{"name":"aurumcode/policy-gate","state":"FAILURE","workflow":"","link":""},
{"name":"aurumcode/review","state":"SUCCESS","workflow":"","link":""}
]`

func item(check, status string) types.CIAnalysis {
	return types.CIAnalysis{Check: check, Status: status, Cause: "causa inventada", Fix: "correção inventada"}
}

func checks(items []types.CIAnalysis) string {
	names := make([]string, 0, len(items))
	for _, i := range items {
		names = append(names, i.Check)
	}
	return strings.Join(names, "|")
}

// AC-001: the model never receives this product's previous statuses nor a
// check without a result, and an item about either never survives.
func TestAC001RunningChecksAndOwnStatusesAreNotFacts(t *testing.T) {
	ctx := Parse(pr96Context, ownPrefix)
	text := ctx.ModelText()
	for _, absent := range []string{"aurumcode/policy-gate", "aurumcode/review", "IN_PROGRESS", "Race tests in OCI", "Documentation browser checks", "Review pull request"} {
		if strings.Contains(text, absent) {
			t.Errorf("model CI context carries %q:\n%s", absent, text)
		}
	}
	if !strings.Contains(text, "4 check(s) ainda sem resultado") || !strings.Contains(text, "2 status publicado(s)") {
		t.Errorf("model CI context does not say what was withheld:\n%s", text)
	}
	items := []types.CIAnalysis{
		item("aurumcode/policy-gate", "failure"),
		item("Race tests in OCI", "failure"),
		item("Build and test in OCI", "in_progress"),
		item("Documentation browser checks", "in progress"),
	}
	kept, discarded := Facts{Context: ctx}.Keep(items)
	if len(kept) != 0 || len(discarded) != len(items) {
		t.Fatalf("kept %q, want every item discarded; discarded=%+v", checks(kept), discarded)
	}
	if discarded[0].Reason != ReasonOwnStatus || discarded[1].Reason != ReasonWithoutResult {
		t.Errorf("discard reasons = %+v", discarded)
	}
}

// AC-002: a scanner that did not run in this execution never appears; one
// that ran keeps its real outcome.
func TestAC002ScannerNotRunNeverAppears(t *testing.T) {
	ctx := Parse(pr96Context, ownPrefix)
	items := []types.CIAnalysis{item("scanner_semgrep", "inconclusive")}
	kept, discarded := Facts{Context: ctx}.Keep(items)
	if len(kept) != 0 || len(discarded) != 1 || discarded[0].Reason != ReasonScannerNotRun {
		t.Fatalf("kept %q discarded %+v, want scanner_semgrep discarded as not run", checks(kept), discarded)
	}
	kept, _ = Facts{Context: ctx, Executed: []string{"semgrep"}}.Keep(items)
	if checks(kept) != "scanner_semgrep" {
		t.Fatalf("an executed scanner's outcome was discarded: kept %q", checks(kept))
	}
}

// AC-003: a concluded failure stays in the model's context and its item is
// published with what the context says.
func TestAC003ConcludedFailureStays(t *testing.T) {
	ctx := Parse(pr96Context, ownPrefix)
	text := ctx.ModelText()
	if !strings.Contains(text, `"name":"Lint","state":"FAILURE"`) || !strings.Contains(text, "https://example.test/6") {
		t.Fatalf("the concluded failure left the model context:\n%s", text)
	}
	kept, discarded := Facts{Context: ctx}.Keep([]types.CIAnalysis{item("Lint", "FAILURE"), item("Build and test in OCI", "SUCCESS")})
	if checks(kept) != "Lint|Build and test in OCI" || len(discarded) != 0 {
		t.Fatalf("kept %q discarded %+v, want both concluded checks kept", checks(kept), discarded)
	}
}

// Text that is not a JSON array of checks never reaches the model.
func TestUnreadableContextIsWithheld(t *testing.T) {
	text := Parse("aurumcode/policy-gate pending\n", ownPrefix).ModelText()
	if strings.Contains(text, "pending") || text == "" {
		t.Fatalf("unreadable context reached the model as %q", text)
	}
	if got := Parse("  ", ownPrefix).ModelText(); got != "" {
		t.Fatalf("no context supplied rendered %q, want empty", got)
	}
}
