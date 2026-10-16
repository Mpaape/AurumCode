// The analysts' model passes (review.profiles, --perfis): what every pass
// shares, how each analyst's reviewer is built, and the deliberation the
// passes draw from one budget of the review, so selecting analysts never
// multiplies what the single reviewer may spend on tools.
package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

// analystPasses is one review's analyst passes: the provider each pass
// decorates with its profile, the cost tracker every pass shares, the
// rules and catalog every pass accepts, and the review's deliberation plan
// when tools are offered (nil: no pass is offered a tool).
type analystPasses struct {
	provider       llm.Provider
	tracker        *cost.Tracker
	profiles       []reviewprofile.Profile
	dynamicRules   map[string]review.Rule
	ruleCatalogIDs []string
	plan           *deliberationPlan
	// modelWindow is the declared context window every pass starts under.
	modelWindow int
	// spent is told, before a pass starts, that the review's tool rounds
	// are already gone: the analyst then answers from the evidence the
	// earlier passes gathered.
	spent func(analyst string)
	// refused is told, after a pass, which prompts the model refused as too
	// long during it (review.Reviewer.PromptRefusals).
	refused func(refusals []review.PromptRefusal)
	// reduced is told, after a pass, whether the codebase context was
	// reduced to fit the model's window (review.Reviewer.ContextReduction).
	reduced func(reduction review.ContextReduction, ok bool)
}

// analystsOf is the analysts a resolved selection applies, none when the
// selection is empty.
func analystsOf(res *reviewprofile.MultiResult) []reviewprofile.Profile {
	if res == nil || !res.Applied {
		return nil
	}
	return res.Profiles
}

// analystPasses is this session's passes for profiles. A plan prepared
// without a budget gets one here, so analysts never deliberate unbudgeted.
func (s *reviewState) analystPasses(profiles []reviewprofile.Profile) analystPasses {
	plan := s.deliberation
	if plan != nil && plan.budget == nil {
		plan.budget = deliberation.NewBudget(plan.limits)
	}
	return analystPasses{
		provider: s.provider, tracker: s.tracker, profiles: profiles,
		dynamicRules: s.dynamicRules, ruleCatalogIDs: s.ruleCatalogIDs, plan: plan,
		modelWindow: modelWindowTokens(s.cfg, s.env()),
		spent: func(analyst string) {
			fmt.Fprintf(s.stderr, "aurumcode review: %s\n", i18n.Format(s.reviewLanguage, "terminal.deliberation_rounds_spent", analyst))
		},
		refused: s.noteRefusals,
		reduced: s.noteContextReduction,
	}
}

// reviewer is the analyst's own model pass: its decorated provider behind a
// fresh orchestrator (the shared tracker meters every pass, so adding an
// analyst never raises the cost ceiling), the catalog and dynamic rules
// every pass accepts (AUR-519: selecting analysts must never narrow the
// gate back to the embedded catalog), and the review's deliberation when
// tools are offered, drawing from the budget the passes share.
func (a analystPasses) reviewer(p reviewprofile.Profile) (*review.Reviewer, error) {
	orchestrator := llm.NewOrchestrator(newProfileProvider(a.provider, p), nil, a.tracker)
	reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())
	reviewer.SetModelWindow(a.modelWindow)
	reviewer.SetDynamicRules(a.dynamicRules)
	if err := reviewer.SetRuleCatalog(a.ruleCatalogIDs); err != nil {
		return nil, err
	}
	if a.plan != nil {
		if a.plan.budget.ToolRoundsLeft() == 0 && a.spent != nil {
			a.spent(p.Name)
		}
		reviewer.SetDeliberation(a.plan.forCaller(orchestrator, a.plan.budget))
	}
	return reviewer, nil
}

// transcriptOf is the pass's deliberation attributed to the analyst, nil
// when the pass deliberated nothing.
func transcriptOf(reviewer *review.Reviewer, p reviewprofile.Profile) *deliberation.Transcript {
	t := reviewer.Transcript()
	if t != nil {
		t.AttributeTo(p.Name)
	}
	return t
}
