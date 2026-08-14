// Package integration holds AUR-461's Integration-layer proof, at the seam
// the unit program cannot see: the list internal/prompt renders into the
// review prompt and the set internal/review's AUR-434 rule gate accepts
// are the same set, observed through the real GenerateReview path with a
// deterministic offline provider (review.FakeProvider) standing in for the
// model.
//
// tests/unit/AUR-461.go asserts the rendered list against the loader
// directly. That is a comparison of two data structures. This program
// asserts the composed behaviour instead: a model that obeys the prompt --
// citing an id copied verbatim from the list it was given -- has EVERY
// finding survive the gate and reach the caller, and a model that invents
// a plausible id (security/shell-injection, the exact id the 2026-08-14
// measurement lost a real command injection under) still has it discarded
// and announced. Neither fact is observable from the prompt alone.
package integration

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func aur461Diff() *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path:  "svc.py",
		Lang:  "python",
		Hunks: []types.DiffHunk{{Lines: []string{`+os.system("ls " + user_input)`}}},
	}}}
}

// aur461PromptRuleIDs returns the ids as the MODEL would read them: parsed
// back out of the assembled system prompt, not taken from the exported
// slice.
func aur461PromptRuleIDs(t *testing.T) []string {
	t.Helper()
	diff := aur461Diff()
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, err := prompt.NewPromptBuilder().BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens: 8000, SchemaKind: "review", Role: "reviewer", ReserveReply: 1000,
	})
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}
	seen := map[string]bool{}
	var ids []string
	for _, m := range regexp.MustCompile("`((?:security|quality|performance)/[a-z0-9-]+)`").FindAllStringSubmatch(parts.System, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		t.Fatal("the assembled review prompt offers the model no rule id at all")
	}
	return ids
}

func aur461Review(t *testing.T, response string) *types.ReviewResult {
	t.Helper()
	orchestrator := llm.NewOrchestrator(&review.FakeProvider{Response: response}, nil, nil)
	reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())
	result, err := reviewer.GenerateReview(context.Background(), aur461Diff())
	if err != nil {
		t.Fatalf("GenerateReview failed: %v", err)
	}
	return result
}

// IntegrationAUR461 is this card's integration selector.
func IntegrationAUR461(t *testing.T) {
	t.Run("EveryOfferedRuleSurvivesTheGate", aur461OfferedRulesSurvive)
	t.Run("InventedRuleIsStillDiscardedAndAnnounced", aur461InventedStillDiscarded)
}

// aur461OfferedRulesSurvive: the prompt must never offer the model an id
// the gate rejects. Each offered id is fed back through the full pipeline
// as a finding; a single discard means the prompt is inviting the model to
// produce a finding the user will never see.
func aur461OfferedRulesSurvive(t *testing.T) {
	ids := aur461PromptRuleIDs(t)

	type issue struct {
		File     string `json:"file"`
		Line     int    `json:"line"`
		Severity string `json:"severity"`
		RuleID   string `json:"rule_id"`
		Message  string `json:"message"`
	}
	payload := struct {
		Issues  []issue `json:"issues"`
		Summary string  `json:"summary"`
	}{Summary: "one finding per offered rule"}
	for i, id := range ids {
		payload.Issues = append(payload.Issues, issue{
			File: "svc.py", Line: i + 1, Severity: "warning", RuleID: id,
			Message: "obeying the prompt: id copied verbatim from the list it was given",
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("building the canned response: %v", err)
	}

	result := aur461Review(t, string(raw))
	if len(result.Issues) != len(ids) {
		t.Fatalf("the prompt offered %d rule ids but only %d finding(s) survived the gate: the prompt is teaching ids the gate rejects\ndiscard warning: %q",
			len(ids), len(result.Issues), result.Metadata["discard_warning"])
	}
	if warning := result.Metadata["discard_warning"]; warning != "" {
		t.Errorf("every finding cited an offered id, so nothing should have been discarded, got %q", warning)
	}
	// The catalog citation proves the gate RESOLVED each id rather than
	// merely tolerating it.
	for _, got := range result.Issues {
		if !strings.Contains(got.Message, "(rule ") {
			t.Errorf("a surviving finding must carry its catalog citation, got %q", got.Message)
			break
		}
	}
}

// aur461InventedStillDiscarded is the card's Non-goal, executed: giving
// the model the list must not loosen the gate. security/shell-injection is
// the id the real gateway used for a real command injection on
// 2026-08-14; it is not in the catalog, it is not offered by the prompt,
// and it must still be discarded and named -- never silently rewritten to
// the similar-looking security/command-injection.
func aur461InventedStillDiscarded(t *testing.T) {
	for _, id := range aur461PromptRuleIDs(t) {
		if id == "security/shell-injection" {
			t.Fatal("the prompt offers security/shell-injection, which the catalog does not define")
		}
	}

	result := aur461Review(t, `{"issues":[{"file":"svc.py","line":6,"severity":"error","rule_id":"security/shell-injection","message":"user input reaches os.system"}],"summary":"one finding"}`)

	if len(result.Issues) != 0 {
		t.Fatalf("a finding citing an id outside the catalog reached the user: %+v -- the gate was loosened or the id was matched by similarity", result.Issues)
	}
	warning := result.Metadata["discard_warning"]
	if !strings.Contains(warning, "unknown rule_id") || !strings.Contains(warning, "security/shell-injection") {
		t.Errorf("the discard must still name the offending id, got %q", warning)
	}
}
