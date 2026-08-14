package unit

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR461 is this card's unit selector. It proves, at the public
// boundary of internal/prompt, that the review prompt now carries the
// closed list of rule ids the AUR-434 gate accepts -- and that the list
// cannot drift from the catalog without failing right here.
//
// The defect (measured 2026-08-14 against the real gateway): six findings
// came back, five were discarded citing ids the model invented
// (security/shell-injection for what the catalog calls
// security/command-injection, quality/naming for quality/poor-naming,
// quality/documentation and quality/readability for nothing at all),
// because templates/review.md showed one example rule_id and never said
// which ids exist.
//
// This file is a plain .go program, not a _test.go file: the acceptance
// script bridges it (see tests/acceptance/AUR-461.sh).
func TestAUR461(t *testing.T) {
	t.Run("RenderedListEqualsEmbeddedCatalog", testAUR461RenderedListEqualsCatalog)
	t.Run("TemplateCitesOnlyRealRules", testAUR461TemplateCitesOnlyRealRules)
	t.Run("OverBudgetCatalogFailsHigh", testAUR461OverBudgetCatalogFailsHigh)
}

// catalogIDs loads the embedded catalog through the same loader the rule
// gate uses. Never a Skip: a catalog this test cannot read is a test that
// proves nothing while reporting success.
func aur461CatalogIDs(t *testing.T) []string {
	t.Helper()
	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("loading the embedded rule catalog: %v", err)
	}
	rules := loader.GetAll()
	if len(rules) == 0 {
		t.Fatal("the embedded catalog reported zero rules")
	}
	ids := make([]string, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	return ids
}

// aur461Diff is a minimal non-degenerate diff: the prompt content under
// test is the instruction half, not the code half.
func aur461Diff() *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{
		Path:  "svc.py",
		Lang:  "python",
		Hunks: []types.DiffHunk{{Lines: []string{`+os.system("ls " + user_input)`}}},
	}}}
}

func aur461SystemPrompt(t *testing.T) string {
	t.Helper()
	builder := prompt.NewPromptBuilder()
	diff := aur461Diff()
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, err := builder.BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens:    8000,
		SchemaKind:   "review",
		Role:         "reviewer",
		ReserveReply: 1000,
	})
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}
	return parts.System
}

// testAUR461RenderedListEqualsCatalog is AC-001: the prompt the model
// actually receives names every id of the embedded catalog, and names
// nothing that is not in it. Set equality in BOTH directions is what makes
// a new rule added to internal/review/rules/*.yml without the matching
// entry in prompt.DefaultRuleCatalog fail this test -- the mirror cannot
// go stale in silence.
func testAUR461RenderedListEqualsCatalog(t *testing.T) {
	want := aur461CatalogIDs(t)
	system := aur461SystemPrompt(t)

	// A template key left unset renders this literal where the rule list
	// belongs, which would ship an obviously broken prompt.
	if strings.Contains(system, "<no value>") {
		t.Fatalf("the rendered prompt contains an unfilled template key:\n%s", system)
	}

	// Read the ids back out of the rendered prompt itself, not out of the
	// exported slice: what the model sees is the only thing that matters.
	found := map[string]bool{}
	for _, m := range regexp.MustCompile("`((?:security|quality|performance)/[a-z0-9-]+)`").FindAllStringSubmatch(system, -1) {
		found[m[1]] = true
	}

	var missing []string
	for _, id := range want {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		t.Errorf("the rendered review prompt never names %d catalog rule(s): %v -- the model can only invent an id for a rule it was not shown", len(missing), missing)
	}

	inCatalog := map[string]bool{}
	for _, id := range want {
		inCatalog[id] = true
	}
	var extra []string
	for id := range found {
		if !inCatalog[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("the rendered review prompt offers %v, which the rule gate does not accept: a finding citing them would be discarded", extra)
	}

	// The exported mirror must match the catalog exactly too, so a drift
	// is reported here rather than only where it happens to be rendered.
	got := prompt.DefaultRuleCatalog
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("prompt.DefaultRuleCatalog drifted from internal/review/rules/*.yml\n got: %v\nwant: %v", got, want)
	}
}

// testAUR461TemplateCitesOnlyRealRules reads the shipped template bytes:
// before this card its own JSON example taught `quality/naming`, an id the
// catalog does not have (it has `quality/poor-naming`), so the prompt was
// literally teaching the model to produce a discarded finding. Every
// rule-shaped literal in the template must resolve against the loader.
func testAUR461TemplateCitesOnlyRealRules(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "prompt", "templates", "review.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("review template not materialized at %s: %v", path, err)
	}

	loader := review.NewRulesLoader()
	if err := loader.Load(); err != nil {
		t.Fatalf("loading the embedded rule catalog: %v", err)
	}

	cited := regexp.MustCompile(`(?:security|quality|performance)/[a-z0-9-]+`).FindAllString(string(raw), -1)
	if len(cited) == 0 {
		t.Fatal("the review template names no rule at all; the model has nothing to copy")
	}
	seen := map[string]bool{}
	for _, id := range cited {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, ok := loader.Get(id); !ok {
			t.Errorf("the review template teaches %q, which the embedded catalog does not define: a finding copying it is discarded by the AUR-434 gate", id)
		}
	}
}

// testAUR461OverBudgetCatalogFailsHigh is AC-002: a catalog too large for
// the prompt budget is an error, never a quietly shortened list. A
// truncated list would make the model cite a rule that exists but was cut
// -- this card's own defect, back by another road.
func testAUR461OverBudgetCatalogFailsHigh(t *testing.T) {
	est := prompt.NewHeuristicEstimator()

	if err := prompt.ValidateRuleCatalog(prompt.DefaultRuleCatalog, est); err != nil {
		t.Fatalf("the shipped catalog must fit the prompt budget: %v", err)
	}

	oversized := append([]string(nil), prompt.DefaultRuleCatalog...)
	for i := 0; i < 200; i++ {
		oversized = append(oversized, "quality/synthetic-overflow-rule-"+strings.Repeat("x", 20)+string(rune('a'+i%26)))
	}
	sort.Strings(oversized)
	err := prompt.ValidateRuleCatalog(oversized, est)
	if err == nil {
		t.Fatal("a catalog over the token budget was accepted: the list would be truncated and the model would cite a rule that was cut")
	}
	if !strings.Contains(err.Error(), "token budget") {
		t.Errorf("the overflow error must name the budget, got %q", err)
	}
	if rendered := prompt.RenderRuleCatalog(oversized); !strings.Contains(rendered, oversized[len(oversized)-1]) {
		t.Error("RenderRuleCatalog dropped entries instead of rendering the whole list; truncation must be impossible, not merely unused")
	}

	// An empty catalog is equally loud: it would tell the model to choose
	// from nothing.
	if err := prompt.ValidateRuleCatalog(nil, est); err == nil {
		t.Error("an empty rule catalog was accepted")
	}

	// And the same failure must stop prompt assembly, not just validation.
	builder := prompt.NewPromptBuilder()
	if err := builder.SetRuleCatalog(oversized); err == nil {
		t.Error("SetRuleCatalog accepted an over-budget catalog")
	}
	if strings.Join(builder.RuleCatalog(), ",") != strings.Join(prompt.DefaultRuleCatalog, ",") {
		t.Error("a rejected catalog must leave the builder on its previous, valid list")
	}
}
