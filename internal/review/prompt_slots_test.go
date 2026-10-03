package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// messagesCaptureProvider has the MessageCompleter capability and records
// the conversation it received.
type messagesCaptureProvider struct {
	FakeProvider
	messages []llm.Message
}

func (p *messagesCaptureProvider) CompleteMessages(messages []llm.Message, opts llm.Options) (llm.Response, error) {
	p.messages = append([]llm.Message(nil), messages...)
	return p.FakeProvider.Complete(llm.FlattenMessages(messages), opts)
}

const approveResponse = `{"verdict":"approve","issues":[],"summary":"ok"}`

func sampleEvidence(n int) []prompt.EvidenceItem {
	items := make([]prompt.EvidenceItem, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, prompt.EvidenceItem{
			ID: fmt.Sprintf("ev-%d", i), Origin: "sast:fixture", RuleID: "security/command-injection",
			File: "internal/app/service.go", Line: i + 1, Severity: "error",
			Snippet: fmt.Sprintf("+\treturn %d", i),
		})
	}
	return items
}

// AC-002: structured evidence renders in its own slot with origin, rule,
// file:line and severity, every field passes the redaction filter, and an
// over-ceiling list declares how many items it omitted while the whole
// prompt still fits the budget.
func TestEvidenceSlotRendersRedactsAndDeclaresOmissions(t *testing.T) {
	const canary = "canary-value-578-do-not-leak"
	provider := &messagesCaptureProvider{FakeProvider: FakeProvider{Response: approveResponse}}
	reviewer := NewReviewer(llm.NewOrchestrator(provider, nil, nil), DefaultConfig())
	reviewer.filter = redaction.NewFilter(canary)

	evidence := []prompt.EvidenceItem{{
		ID: "ev-" + canary, Origin: "sast:" + canary, RuleID: "security/" + canary,
		File: "internal/" + canary + ".go", Line: 7, Severity: canary,
		Snippet: "+\tpassword := \"" + canary + "\"",
	}}
	if _, err := reviewer.GenerateReviewWithContext(context.Background(), goldenDiff(), ReviewContext{Evidence: evidence}); err != nil {
		t.Fatal(err)
	}
	user := provider.messages[1].Content
	if strings.Contains(user, canary) || strings.Contains(provider.messages[0].Content, canary) {
		t.Fatal("an evidence field reached the prompt without redaction")
	}
	section := user[strings.Index(user, "## Deterministic evidence"):]
	for _, want := range []string{"origem=sast:", "regra=security/", "local=internal/", ".go:7", "severidade=", "trecho: +\tpassword"} {
		if !strings.Contains(section, want) {
			t.Errorf("evidence section lacks %q:\n%s", want, section)
		}
	}

	// Over the evidence ceiling: the section names how many were left out,
	// and the assembled prompt stays within the prompt budget.
	limits := prompt.DefaultLimits()
	// The section's fixed instructions take part of the ceiling; 400 leaves
	// room for some, never all, of the 40 items.
	limits.EvidenceMaxTokens = 400
	if err := reviewer.promptBuilder.SetSlotLimits(limits); err != nil {
		t.Fatal(err)
	}
	reviewer.cfg.PromptTokenBudget = 9000
	result, err := reviewer.GenerateReviewWithContext(context.Background(), goldenDiff(), ReviewContext{Evidence: sampleEvidence(40)})
	if err != nil {
		t.Fatal(err)
	}
	user = provider.messages[1].Content
	rendered := strings.Count(user, "- [ev-")
	if rendered == 0 || rendered == 40 {
		t.Fatalf("expected the ceiling to admit some but not all of 40 items, admitted %d", rendered)
	}
	if want := fmt.Sprintf("- %d omitidos pelo orçamento desta seção", 40-rendered); !strings.Contains(user, want) {
		t.Fatalf("evidence section does not declare %q", want)
	}
	estimated, _ := strconv.Atoi(result.Metadata["estimated_tokens"])
	if estimated <= 0 || estimated > 9000 {
		t.Fatalf("estimated_tokens = %d, want within the 9000-token budget", estimated)
	}
}

// AC-003: the provider receives separate system and user messages; every
// section, the repository context included, is inside the measured prompt
// (estimated_tokens is the estimate of exactly what was sent); and the
// prompt digest is stable for equal input and moves with the evidence.
func TestProviderReceivesSystemAndUserMessages(t *testing.T) {
	provider := &messagesCaptureProvider{FakeProvider: FakeProvider{Response: approveResponse}}
	reviewer := NewReviewer(llm.NewOrchestrator(provider, nil, nil), DefaultConfig())
	block, _, err := config.BuildContextBlockWithWarnings(context.Background(), goldenContextProviders(), nil, redaction.NewFilter())
	if err != nil {
		t.Fatal(err)
	}
	rc := ReviewContext{RepositoryContext: block, Evidence: sampleEvidence(2)}
	result, err := reviewer.GenerateReviewWithContext(context.Background(), goldenDiff(), rc)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.messages) != 2 || provider.messages[0].Role != llm.RoleSystem || provider.messages[1].Role != llm.RoleUser {
		t.Fatalf("provider received %d messages %+v, want system then user", len(provider.messages), provider.messages)
	}
	system, user := provider.messages[0].Content, provider.messages[1].Content
	if !strings.Contains(system, "# Code Review Prompt") || strings.Contains(system, "## Change Summary") {
		t.Fatal("system message is not exactly the instruction template")
	}
	if !strings.HasPrefix(user, "## Change Summary") || !strings.HasSuffix(user, "\n\n"+block) || strings.Count(user, "## Repository context") != 1 {
		t.Fatal("the repository context is not the user message's final template slot")
	}
	want := strconv.Itoa(prompt.NewHeuristicEstimator().Estimate(system + user))
	if result.Metadata["estimated_tokens"] != want {
		t.Fatalf("estimated_tokens = %s but the sent prompt measures %s: text was added outside the budgeted template", result.Metadata["estimated_tokens"], want)
	}

	d1, err := reviewer.PromptDigest(goldenDiff(), rc)
	if err != nil {
		t.Fatal(err)
	}
	d2, _ := reviewer.PromptDigest(goldenDiff(), rc)
	if d1 != d2 {
		t.Fatal("prompt digest differs between two calls with the same input")
	}
	if d1 != (prompt.PromptParts{System: system, User: user}).Digest() {
		t.Fatal("prompt digest is not the digest of the messages actually sent")
	}
	changed := rc
	changed.Evidence = sampleEvidence(3)
	if d3, _ := reviewer.PromptDigest(goldenDiff(), changed); d3 == d1 {
		t.Fatal("prompt digest did not move when the evidence changed")
	}
	noContext := rc
	noContext.RepositoryContext = ""
	if d4, _ := reviewer.PromptDigest(goldenDiff(), noContext); d4 == d1 {
		t.Fatal("prompt digest did not move when the repository context changed")
	}
}

// AC-006 for the migration path: carrying the repository context in its
// template slot sends a provider without the message capability exactly
// the bytes the legacy provider decorator sent.
func TestRepositoryContextSlotMatchesLegacyDecoratorBytes(t *testing.T) {
	for name, rc := range goldenContexts() {
		capture := filepath.Join(t.TempDir(), "prompt.txt")
		fake := &FakeProvider{Response: approveResponse, CapturePath: capture}
		block, _, err := config.BuildContextBlockWithWarnings(context.Background(), goldenContextProviders(), []string{"internal/app/service.go"}, redaction.NewFilter())
		if err != nil {
			t.Fatal(err)
		}
		rc.RepositoryContext = block
		reviewer := NewReviewer(llm.NewOrchestrator(fake, nil, nil), DefaultConfig())
		if _, err := reviewer.GenerateReviewWithContext(context.Background(), goldenDiff(), rc); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", "promptgolden", "capture_"+name+"_repoctx.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s: slot path differs from the legacy decorator bytes at byte %d", name, firstDifference(string(want), string(got)))
		}
	}
}

// AC-004: two reviews of the same diff with different evidence never share
// a cache entry -- including when the difference is an item the evidence
// ceiling kept out of the prompt text, which only the evidence digest sees.
func TestRequestCacheKeyCoversEvidence(t *testing.T) {
	reviewer := NewReviewer(llm.NewOrchestrator(&FakeProvider{Response: approveResponse}, nil, nil), DefaultConfig())
	limits := prompt.DefaultLimits()
	limits.EvidenceMaxTokens = 220
	if err := reviewer.promptBuilder.SetSlotLimits(limits); err != nil {
		t.Fatal(err)
	}
	base := ReviewContext{Evidence: sampleEvidence(40)}
	key := func(rc ReviewContext, tools any) string {
		k, err := reviewer.RequestCacheKey(goldenDiff(), rc, tools)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	k1 := key(base, nil)
	if k1 != key(base, nil) {
		t.Fatal("the same request produced two cache keys")
	}
	visible := ReviewContext{Evidence: sampleEvidence(1)}
	if key(visible, nil) == k1 {
		t.Fatal("different evidence shared a cache key")
	}
	hidden := ReviewContext{Evidence: sampleEvidence(40)}
	hidden.Evidence[39].Severity = "warning" // beyond the ceiling: not in the prompt text
	d1, _ := reviewer.PromptDigest(goldenDiff(), base)
	d2, _ := reviewer.PromptDigest(goldenDiff(), hidden)
	if d1 != d2 {
		t.Fatal("fixture invalid: the changed item must be omitted from the prompt text")
	}
	if key(hidden, nil) == k1 {
		t.Fatal("evidence omitted from the prompt but different shared a cache key: the evidence digest is not in the key")
	}
	if key(base, []string{"tool result"}) == k1 {
		t.Fatal("different tool results shared a cache key")
	}
}

// AC-005: the model's per-evidence assessment fills ReviewIssue.Assessment;
// an origin the model supplies is discarded, because only the engine writes
// it.
func TestModelAssessmentParsedAndOriginIgnored(t *testing.T) {
	response := strings.Replace(knownProblemResponse, `"severity": "error",`,
		`"severity": "error", "origin": "sast:forged", "assessment": {"evidence_id": "ev-1", "status": "confirmed", "justification": "The value is a literal credential."},`, 1)
	reviewer := NewReviewer(llm.NewOrchestrator(&FakeProvider{Response: response}, nil, nil), DefaultConfig())
	// The assessment names evidence the engine offered: an assessment of
	// evidence never offered is discarded (weighAssessments).
	offered := ReviewContext{Evidence: []prompt.EvidenceItem{{ID: "ev-1", Origin: "sast:semgrep", RuleID: "security/hardcoded-secret", File: "config.go", Line: 1, Severity: "error"}}}
	result, err := reviewer.GenerateReviewWithContext(context.Background(), newFixtureDiff(t), offered)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected the issue to survive the gates, got %+v", result.Issues)
	}
	issue := result.Issues[0]
	if issue.Origin != "" {
		t.Fatalf("Origin = %q: a model-supplied origin survived the parse", issue.Origin)
	}
	a := issue.Assessment
	if a == nil || a.EvidenceID != "ev-1" || a.Status != types.AssessmentConfirmed || a.Justification == "" {
		t.Fatalf("assessment = %+v, want the model's confirmed assessment of ev-1", a)
	}

	bad := strings.Replace(response, `"status": "confirmed"`, `"status": "certainly"`, 1)
	parsed, err := prompt.NewResponseParser().ParseReviewResponse(bad)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Issues[0].Assessment != nil {
		t.Fatal("an assessment with a status outside the closed set was kept")
	}
}
