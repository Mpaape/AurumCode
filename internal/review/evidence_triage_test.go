package review

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// disputingCompleter is a model with a context window that disputes, with a
// justification, every evidence item its prompt shows.
type disputingCompleter struct {
	windowCompleter
}

var shownEvidence = regexp.MustCompile(`\[(E\d+)\] origem=`)

func (d *disputingCompleter) CompleteMessages(ctx context.Context, messages []llm.Message, opts llm.Options) (llm.Response, error) {
	if _, err := d.windowCompleter.CompleteMessages(ctx, messages, opts); err != nil {
		return llm.Response{}, err
	}
	var text strings.Builder
	for _, m := range messages {
		text.WriteString(m.Content)
	}
	var assessments []string
	seen := map[string]bool{}
	for _, m := range shownEvidence.FindAllStringSubmatch(text.String(), -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			assessments = append(assessments, fmt.Sprintf(`{"evidence_id":%q,"status":"disputed","justification":"valor sintetico de fixture"}`, m[1]))
		}
	}
	return llm.Response{Text: `{"verdict":"comment","issues":[],"summary":"ok","evidence_assessments":[` + strings.Join(assessments, ",") + `]}`}, nil
}

// The incident in batches: a review split into batches still has the model
// decide every evidence item -- the one tied to a batch's file, the one tied
// to no file and the one tied to a file outside the reviewed diff (a scanner
// reads the whole checkout) -- and the consolidated result carries every
// dispute.
func TestBatchedReviewCarriesEveryEvidenceAssessment(t *testing.T) {
	model := &disputingCompleter{windowCompleter{limit: 20000}}
	reviewer := NewReviewer(model, DefaultConfig())
	diff := manyFilesDiff(3, 2, 400)
	evidence := []prompt.EvidenceItem{
		{ID: "E1", Origin: "gitleaks", RuleID: "gitleaks:generic-api-key", File: "pkg2/file1.go", Line: 3, Severity: "error"},
		{ID: "E2", Origin: "gitleaks", RuleID: "gitleaks:generic-api-key", File: "demo/fixtures/findings.json", Line: 17, Severity: "error"},
		{ID: "E3", Origin: "analysis", RuleID: "analysis/hardcoded-secret", Severity: "error"},
	}
	result, err := reviewer.GenerateReviewWithContext(context.Background(), diff, ReviewContext{Evidence: evidence})
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewer.Batches()) < 2 {
		t.Fatalf("%d batches: the test does not exercise the split", len(reviewer.Batches()))
	}
	got := map[string]string{}
	for _, a := range result.EvidenceAssessments {
		if _, dup := got[a.EvidenceID]; dup {
			t.Errorf("%s assessed twice in the consolidated result", a.EvidenceID)
		}
		got[a.EvidenceID] = a.Status
	}
	for _, id := range []string{"E1", "E2", "E3"} {
		if got[id] != types.AssessmentDisputed {
			t.Errorf("%s: status %q, want the model's dispute to survive the batches (all: %v)", id, got[id], got)
		}
	}
}

// Of two batches' assessments of the same item, a confirmation outweighs a
// dispute: the consolidated answer never demotes what one batch confirmed.
func TestMergeBatchAssessmentsFailsClosed(t *testing.T) {
	merged := mergeBatchAssessments(
		[]types.EvidenceAssessment{{EvidenceID: "E1", Status: types.AssessmentDisputed, Justification: "x"}},
		[]types.EvidenceAssessment{{EvidenceID: "E1", Status: types.AssessmentConfirmed}, {EvidenceID: "E2", Status: types.AssessmentDisputed}})
	if len(merged) != 2 || merged[0].Status != types.AssessmentConfirmed || merged[1].EvidenceID != "E2" {
		t.Fatalf("merged = %+v", merged)
	}
}

// triageCaller answers the dedicated triage by a function of its prompt.
type triageCaller struct {
	calls   int
	prompts []string
	answer  func(prompt string) (llm.Response, error)
}

func (c *triageCaller) Complete(_ context.Context, text string, _ llm.Options) (llm.Response, error) {
	c.calls++
	c.prompts = append(c.prompts, text)
	return c.answer(text)
}

func triageItems(n int) []prompt.EvidenceItem {
	items := make([]prompt.EvidenceItem, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, prompt.EvidenceItem{ID: fmt.Sprintf("E%d", i), Origin: "gitleaks", RuleID: "gitleaks:generic-api-key",
			File: "fixtures/findings.json", Line: i, Severity: "error", Snippet: "token=valor-sintetico-123456"})
	}
	return items
}

// The dedicated triage shows the rule, the place and the code to the model
// and keeps only valid assessments of the ids it offered.
func TestEvidenceTriagerKeepsValidAssessmentsOfOfferedIDs(t *testing.T) {
	caller := &triageCaller{answer: func(string) (llm.Response, error) {
		return llm.Response{Text: `{"evidence_assessments":[
			{"evidence_id":"E1","status":"disputed","justification":"fixture sintetica"},
			{"evidence_id":"E2","status":"confirmed","justification":"chave real"},
			{"evidence_id":"E9","status":"disputed","justification":"inventado"},
			{"evidence_id":"E2","status":"bogus","justification":"x"}]}`}, nil
	}}
	triager := &EvidenceTriager{Caller: caller, Language: "pt-BR"}
	got, err := triager.Assess(context.Background(), triageItems(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].EvidenceID != "E1" || got[0].Status != types.AssessmentDisputed || got[1].Status != types.AssessmentConfirmed {
		t.Fatalf("assessments = %+v", got)
	}
	sent := caller.prompts[0]
	for _, want := range []string{prompt.TriageMarker, "Achado E1", "regra: gitleaks:generic-api-key", "local: fixtures/findings.json:1", "token=valor-sintetico-123456"} {
		if !strings.Contains(sent, want) {
			t.Errorf("the triage prompt lacks %q:\n%s", want, sent)
		}
	}
}

// A triage prompt the provider refuses as too long is split in halves until
// every item is decided; nothing bounds the number of items.
func TestEvidenceTriagerSplitsARefusedPrompt(t *testing.T) {
	caller := &triageCaller{answer: func(text string) (llm.Response, error) {
		ids := regexp.MustCompile(`Achado (E\d+)`).FindAllStringSubmatch(text, -1)
		if len(ids) > 2 {
			return llm.Response{}, &llm.APIError{Provider: "fake", Status: 400, Message: "too long", TooLong: &llm.PromptTooLong{Limit: 10}}
		}
		var parts []string
		for _, id := range ids {
			parts = append(parts, fmt.Sprintf(`{"evidence_id":%q,"status":"disputed","justification":"fixture"}`, id[1]))
		}
		return llm.Response{Text: `{"evidence_assessments":[` + strings.Join(parts, ",") + `]}`}, nil
	}}
	got, err := (&EvidenceTriager{Caller: caller}).Assess(context.Background(), triageItems(7))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 {
		t.Fatalf("%d of 7 items decided after the split (%d calls)", len(got), caller.calls)
	}
}

// A model that does not answer (provider failure, a reply that is not the
// JSON asked for) is an error: the caller leaves the items awaiting it.
func TestEvidenceTriagerFailsWhenTheModelDoesNotAnswer(t *testing.T) {
	for name, answer := range map[string]func(string) (llm.Response, error){
		"provider": func(string) (llm.Response, error) { return llm.Response{}, errors.New("upstream down") },
		"not json": func(string) (llm.Response, error) { return llm.Response{Text: "nao sei"}, nil },
	} {
		if _, err := (&EvidenceTriager{Caller: &triageCaller{answer: answer}}).Assess(context.Background(), triageItems(1)); err == nil {
			t.Errorf("%s: a model that did not answer must be an error", name)
		}
	}
}
