package main

// The terminal and the audit say what each analyst pass was actually
// offered: a pass that starts after the review's tool rounds are spent, or
// under a max_rounds that leaves no tool round, lists no tool and says why,
// instead of listing tools it never received.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type offeredAudit struct {
	Deliberation *struct {
		Offered      []string `json:"offered"`
		NotOffered   string   `json:"not_offered"`
		NotRequested []string `json:"not_requested"`
		Analysts     []struct {
			Analyst    string   `json:"analyst"`
			Offered    []string `json:"offered"`
			NotOffered string   `json:"not_offered"`
			Rounds     int      `json:"rounds"`
		} `json:"analysts"`
	} `json:"deliberation"`
}

func offeredRun(t *testing.T, maxRounds string) (string, offeredAudit) {
	t.Helper()
	aur580Setup(t, strings.Replace(aur580Config, "max_rounds: 3", "max_rounds: "+maxRounds, 1), "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, aur616ScannerCall))
	audit := filepath.Join(t.TempDir(), "audit.json")
	_, _, errOut := aur579Review(t, nil, "--auditoria", audit, "--perfis", "seguranca,solid")
	var rec offeredAudit
	data, err := os.ReadFile(audit)
	if err != nil {
		t.Fatalf("no audit: %v; stderr=%s", err, errOut)
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("audit is not JSON: %v\n%s", err, data)
	}
	if rec.Deliberation == nil || len(rec.Deliberation.Analysts) != 2 {
		t.Fatalf("audit deliberation = %+v; stderr=%s", rec.Deliberation, errOut)
	}
	return errOut, rec
}

// With max_rounds 2 the first analyst spends the review's only tool round:
// its line lists the tools it was offered; the second analyst's line and
// audit entry list no tool and name the spent rounds.
func TestAnalystWithoutToolRoundListsNoOfferedTool(t *testing.T) {
	errOut, rec := offeredRun(t, "2")
	if !strings.Contains(errOut, "deliberation: analyst seguranca: offered ["+aur616AllTools+"]; rounds 2; requested [scanner_fakescan]; outcome answered") {
		t.Errorf("stderr lacks what the first analyst was offered:\n%s", errOut)
	}
	want := "deliberation: analyst solid: no tool offered (the review's tool rounds (max_rounds) were already spent by the earlier analysts); rounds 1; outcome answered"
	if !strings.Contains(errOut, want) {
		t.Errorf("stderr lacks %q:\n%s", want, errOut)
	}
	if strings.Contains(errOut, "analyst solid: offered [") {
		t.Errorf("the second analyst was reported offered tools it never received:\n%s", errOut)
	}
	d := rec.Deliberation
	if first := d.Analysts[0]; strings.Join(first.Offered, ", ") != aur616AllTools || first.NotOffered != "" {
		t.Errorf("first analyst = %+v, want every tool offered", first)
	}
	if second := d.Analysts[1]; len(second.Offered) != 0 || second.NotOffered != "tool_rounds_spent" || second.Rounds != 1 {
		t.Errorf("second analyst = %+v, want no tool offered for tool_rounds_spent", second)
	}
	if strings.Join(d.Offered, ", ") != aur616AllTools || d.NotOffered != "" {
		t.Errorf("review offered = %v (%q), want the tools the first analyst received", d.Offered, d.NotOffered)
	}
}

// With max_rounds 1 the review has no tool round at all: neither analyst
// is offered a tool, the review line lists none and says why, nothing is
// counted as declined, and no analyst is told earlier analysts spent the
// rounds.
func TestNoToolRoundReviewListsNoOfferedTool(t *testing.T) {
	errOut, rec := offeredRun(t, "1")
	for _, want := range []string{
		"deliberation: oferecidas []; pedidas []; não pedidas []",
		"deliberation: no tool offered in the review (max_rounds 1 leaves the review no tool round)",
		"deliberation: analyst seguranca without tools: max_rounds 1 leaves the review only the final answer round, no tool round",
		"deliberation: analyst seguranca: no tool offered (max_rounds 1 leaves the review no tool round); rounds 1",
		"deliberation: analyst solid: no tool offered (max_rounds 1 leaves the review no tool round); rounds 1",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "oferecidas [scanner_fakescan]") || strings.Contains(errOut, "spent by the earlier analysts") {
		t.Errorf("stderr claims a tool or a spent round that never existed:\n%s", errOut)
	}
	d := rec.Deliberation
	if len(d.Offered) != 0 || len(d.NotRequested) != 0 || d.NotOffered != "no_tool_round" {
		t.Errorf("review = offered %v, not requested %v, not offered %q; want none offered for no_tool_round", d.Offered, d.NotRequested, d.NotOffered)
	}
	for _, a := range d.Analysts {
		if len(a.Offered) != 0 || a.NotOffered != "no_tool_round" {
			t.Errorf("analyst %+v, want no tool offered for no_tool_round", a)
		}
	}
}
