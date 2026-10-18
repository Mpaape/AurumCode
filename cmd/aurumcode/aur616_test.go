package main

// AUR-616 through the real command: the analysts (--perfis) deliberate with
// the same tools as the single reviewer, from one budget of the review; the
// terminal names the real reason when nothing is offered; without analysts
// nothing changes.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type aur616Audit struct {
	Gate struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	} `json:"gate"`
	Deliberation *struct {
		Limit      string   `json:"limit"`
		Rounds     int      `json:"rounds"`
		CostTokens int      `json:"cost_tokens"`
		Requested  []string `json:"requested"`
		Calls      []struct {
			Tool    string `json:"tool"`
			Analyst string `json:"analyst"`
			Status  string `json:"status"`
		} `json:"calls"`
		Analysts []struct {
			Analyst   string   `json:"analyst"`
			Rounds    int      `json:"rounds"`
			Requested []string `json:"requested"`
			Outcome   string   `json:"outcome"`
			Limit     string   `json:"limit"`
		} `json:"analysts"`
	} `json:"deliberation"`
}

func aur616Run(t *testing.T, args ...string) (int, string, string, aur616Audit) {
	t.Helper()
	audit := filepath.Join(t.TempDir(), "audit.json")
	code, out, errOut := aur579Review(t, nil, append([]string{"--auditoria", audit}, args...)...)
	var rec aur616Audit
	if data, err := os.ReadFile(audit); err == nil {
		if err := json.Unmarshal(data, &rec); err != nil {
			t.Fatalf("audit is not JSON: %v\n%s", err, data)
		}
	}
	return code, out, errOut, rec
}

// aur616ScannerCall is a model that asks for the optional scanner on a
// large diff, once per conversation.
const aur616ScannerCall = `[{"lines_above":1,"tool":"scanner_fakescan"}]`

// aur616AllTools is every tool a pass of this setup is offered, in order.
const aur616AllTools = "scanner_fakescan, codebase_context, read_file, search_text, find_symbol, changed_file_diff"

// AC-001: each analyst's pass is offered the tools and asks for the
// scanner; the scan runs once for the review and answers both; the
// terminal and the audit say which analyst asked for what; the message
// that blames the provider never appears.
func TestAUR616AnalystsDeliberateWithTools(t *testing.T) {
	invoked := aur580Setup(t, aur580Config, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, aur616ScannerCall))
	code, _, errOut, rec := aur616Run(t, "--perfis", "seguranca,solid")
	if code != exitFindings || *invoked != 1 {
		t.Fatalf("exit=%d invoked=%d, want the scan both analysts asked for to run once and fail the gate; stderr=%s", code, *invoked, errOut)
	}
	for _, want := range []string{
		"deliberation: analyst seguranca: offered [" + aur616AllTools + "]; rounds 2; requested [scanner_fakescan]; outcome answered",
		"deliberation: analyst solid: offered [" + aur616AllTools + "]; rounds 2; requested [scanner_fakescan]; outcome answered",
		"deliberation: round 1, analyst seguranca: scanner_fakescan(",
		"deliberation: round 1, analyst solid: scanner_fakescan(",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "does not call tools") || strings.Contains(errOut, "não chama ferramentas") {
		t.Errorf("the provider was blamed although it calls tools:\n%s", errOut)
	}
	d := rec.Deliberation
	if d == nil || d.Rounds != 4 || len(d.Analysts) != 2 || len(d.Calls) != 2 {
		t.Fatalf("audit deliberation = %+v", d)
	}
	for i, name := range []string{"seguranca", "solid"} {
		if a := d.Analysts[i]; a.Analyst != name || a.Rounds != 2 || len(a.Requested) != 1 || a.Requested[0] != "scanner_fakescan" || a.Outcome != "answered" {
			t.Errorf("analyst %d = %+v, want %s with 2 rounds asking the scanner", i, a, name)
		}
		if c := d.Calls[i]; c.Analyst != name || c.Tool != "scanner_fakescan" || c.Status != "executed" {
			t.Errorf("call %d = %+v, want the scanner call of %s", i, c, name)
		}
	}
}

// AC-002: the rounds that ask for a tool are the review's: with max_rounds
// 2 the first analyst spends the one tool round and the second is told,
// and answers without a tool.
func TestAUR616ToolRoundsAreSummedOverTheAnalysts(t *testing.T) {
	invoked := aur580Setup(t, strings.Replace(aur580Config, "max_rounds: 3", "max_rounds: 2", 1), "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, aur616ScannerCall))
	code, _, errOut, rec := aur616Run(t, "--perfis", "seguranca,solid")
	if code != exitFindings || *invoked != 1 {
		t.Fatalf("exit=%d invoked=%d; stderr=%s", code, *invoked, errOut)
	}
	if !strings.Contains(errOut, "deliberation: analyst solid without tools: the review's tool rounds (max_rounds) were already spent by the earlier analysts") {
		t.Errorf("stderr lacks the spent-budget line for solid:\n%s", errOut)
	}
	d := rec.Deliberation
	if d == nil || len(d.Analysts) != 2 {
		t.Fatalf("audit deliberation = %+v", d)
	}
	if a := d.Analysts[0]; a.Rounds != 2 || len(a.Requested) != 1 {
		t.Errorf("first analyst = %+v, want the tool round and the answer", a)
	}
	if a := d.Analysts[1]; a.Rounds != 1 || len(a.Requested) != 0 || a.Outcome != "answered" {
		t.Errorf("second analyst = %+v, want one round, no request, answered", a)
	}
}

// AC-002: the deliberation cost is summed over the analysts against
// max_cost_tokens: a ceiling one analyst fits is exceeded by two, and the
// review is inconclusive with nothing of the model published.
func TestAUR616CostTokensAreSummedOverTheAnalysts(t *testing.T) {
	aur580Setup(t, aur580Config, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, aur616ScannerCall))
	code, _, errOut, one := aur616Run(t, "--perfis", "seguranca")
	if code != exitFindings || one.Deliberation == nil || one.Deliberation.CostTokens < 2 {
		t.Fatalf("one analyst: exit=%d deliberation=%+v; stderr=%s", code, one.Deliberation, errOut)
	}
	ceiling := one.Deliberation.CostTokens + one.Deliberation.CostTokens/2
	writeRepoConfig(t, fmt.Sprintf("deliberation:\n  enabled: true\n  max_rounds: 3\n  max_cost_tokens: %d\nquality_gates:\n  scanners:\n    - engine: fakescan\n", ceiling))
	code, out, errOut, two := aur616Run(t, "--perfis", "seguranca,solid")
	if code == 0 || !strings.Contains(errOut, "deliberation_limit:max_cost_tokens") {
		t.Fatalf("exit=%d, want the summed cost to stop the review; stderr=%s", code, errOut)
	}
	if two.Deliberation == nil || two.Deliberation.Limit != "max_cost_tokens" || two.Deliberation.CostTokens <= ceiling {
		t.Fatalf("audit deliberation=%+v, want the cost over %d recorded with its limit", two.Deliberation, ceiling)
	}
	if a := two.Deliberation.Analysts; len(a) != 2 || a[0].Limit != "" || a[1].Limit != "max_cost_tokens" {
		t.Fatalf("analysts = %+v, want the second stopped by the cost", a)
	}
	if strings.Contains(out, "confirmou fake leak") || strings.Contains(out, "sem varredura opcional") {
		t.Fatalf("a review stopped by its cost published the model's text:\n%s", out)
	}
}

// AC-003: nothing offered says why: a provider without tool calling is
// named; deliberation off in the configuration names the key, never the
// provider.
func TestAUR616MessagesNameTheRealReason(t *testing.T) {
	aur580Setup(t, aur580Config, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur513WriteFixture(t, aur513CleanResponse))
	_, _, errOut, _ := aur616Run(t, "--perfis", "seguranca")
	if !strings.Contains(errOut, "deliberation: provider fixture does not call tools; the optional scanners run without consulting the model") {
		t.Errorf("stderr does not name the provider without tools:\n%s", errOut)
	}
	writeRepoConfig(t, "quality_gates:\n  scanners:\n    - engine: fakescan\n")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, aur616ScannerCall))
	_, _, errOut, _ = aur616Run(t, "--perfis", "seguranca")
	if !strings.Contains(errOut, "deliberation: off in the configuration (deliberation.enabled: false); the analysts review without asking for tools") {
		t.Errorf("stderr does not name the configuration:\n%s", errOut)
	}
	if strings.Contains(errOut, "does not call tools") {
		t.Errorf("the provider was blamed for a configuration choice:\n%s", errOut)
	}
}

// AC-004: without analysts the single reviewer deliberates as before: the
// same report lines, no analyst in the audit, and silence when
// deliberation is off.
func TestAUR616WithoutAnalystsNothingChanges(t *testing.T) {
	aur580Setup(t, aur580Config, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, aur616ScannerCall))
	code, _, errOut, rec := aur616Run(t)
	if code != exitFindings || !strings.Contains(errOut, "pedidas [scanner_fakescan]") {
		t.Fatalf("exit=%d; stderr=%s", code, errOut)
	}
	if strings.Contains(errOut, "analyst") || strings.Contains(errOut, "analista") {
		t.Errorf("an analyst line without analysts:\n%s", errOut)
	}
	d := rec.Deliberation
	if d == nil || d.Rounds != 2 || len(d.Analysts) != 0 || len(d.Calls) != 1 || d.Calls[0].Analyst != "" {
		t.Fatalf("audit deliberation = %+v, want the single reviewer's record", d)
	}
	writeRepoConfig(t, "quality_gates:\n  scanners:\n    - engine: fakescan\n")
	if _, _, errOut, _ = aur616Run(t); strings.Contains(errOut, "deliberation:") {
		t.Errorf("deliberation off without analysts must stay silent:\n%s", errOut)
	}
}
