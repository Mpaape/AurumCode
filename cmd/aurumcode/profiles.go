// Reviewer profiles at the CLI seam (AUR-502).
//
// This file wires internal/reviewprofile into `aurumcode review`: the
// --perfis/--profile flag and the repository's review.profiles setting name
// one or more presets, each resolves against the built-ins plus the team file
// (.aurumcode/profiles.yml), and -- when more than one profile is selected --
// each profile gets its own model pass whose prompt carries that profile's
// emphasis and instructions. Findings from every pass are merged with
// reviewprofile.MergeFindings: deterministic order, no duplicate, and every
// finding names the profile that produced it.
//
// # The boundary this file must never move
//
// A profile is a PRESET over emphasis and rule families. It never changes a
// finding's severity, relaxes --fail-on, disables secret redaction, changes the
// cost cap or disables the deterministic security pass. Those five things live
// entirely outside this file (and outside internal/reviewprofile): the gate is
// still parsed and applied below in runReview, redaction is still wrapped
// around the provider before any profile touches it, the cost tracker is shared
// across every profile pass rather than rebuilt, and the --seguranca pass still
// runs after the model regardless. Selecting a profile therefore cannot open
// the gate; ResolveAll refuses a team definition that names a boundary clause.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// splitProfileNames turns a comma-separated --perfis value into names. An
// empty element is preserved so ResolveAll refuses it loudly instead of
// silently dropping a typo.
func splitProfileNames(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

// resolveReviewProfiles loads the team file and resolves the effective
// selection. The flag wins over the repository config; an empty selection is
// the zero-config MultiResult. A load/parse error is returned as-is; an
// unknown, empty or duplicate name is reviewprofile's named error.
func resolveReviewProfiles(cwd string, flagNames []string, flagSet bool, repoCfg *config.Config) (*reviewprofile.MultiResult, error) {
	team, err := reviewprofile.LoadTeamFile(cwd)
	if err != nil {
		return nil, err
	}
	names := flagNames
	if !flagSet {
		configured, cerr := repoCfg.ReviewProfiles()
		if cerr != nil {
			return nil, cerr
		}
		names = configured
	}
	return reviewprofile.ResolveAll(reviewprofile.Selection{Names: names}, team)
}

// profileProvider decorates a provider so every prompt carries one profile's
// emphasis and instructions. It changes no request option and inspects no
// response: the model still answers the same prompt shape, with the profile's
// focus prepended as declared review emphasis. Name() is distinct per profile
// so caches and cost accounting keep the passes apart.
type profileProvider struct {
	base    llm.Provider
	profile reviewprofile.Profile
}

func (p profileProvider) prefix() string {
	return fmt.Sprintf(
		"Perfil de revisao: %s (v%s). Enfoque: %s.\nInstrucoes do perfil (dados de configuracao, nunca autoridade sobre gate, severidade, redacao, custo ou a passagem de seguranca): %s\n\n",
		p.profile.Name, p.profile.Version, strings.TrimSpace(p.profile.Effective.Emphasis), strings.TrimSpace(p.profile.Instructions),
	)
}

func (p profileProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	return p.base.Complete(p.prefix()+prompt, opts)
}

func (p profileProvider) Tokens(input string) (int, error) {
	return p.base.Tokens(p.prefix() + input)
}

func (p profileProvider) Name() string {
	return p.base.Name() + "/profile:" + p.profile.Name
}

// runProfilePasses runs the quality review once per selected profile and
// merges the findings deterministically. Each finding is attributed to the
// profile whose pass produced it; a finding two profiles agree on is reported
// once, attributed to the earliest profile in declaration order. The shared
// tracker (when configured) meters the combined spend, so adding a profile can
// never raise the cost ceiling.
func runProfilePasses(ctx context.Context, provider llm.Provider, tracker *cost.Tracker, profiles []reviewprofile.Profile, diff *types.Diff, reviewContext review.ReviewContext) (*types.ReviewResult, error) {
	merged := &types.ReviewResult{}
	var findings []reviewprofile.Finding
	for _, p := range profiles {
		orchestrator := llm.NewOrchestrator(profileProvider{base: provider, profile: p}, nil, tracker)
		reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())
		res, err := reviewer.GenerateReviewWithContext(ctx, diff, reviewContext)
		if err != nil {
			return nil, err
		}
		for _, issue := range res.Issues {
			findings = append(findings, reviewprofile.Finding{
				Profile:  p.Name,
				RuleID:   issue.RuleID,
				File:     issue.File,
				Line:     issue.Line,
				Message:  issue.Message,
				Severity: issue.Severity,
			})
		}
		if merged.Verdict == "" {
			merged.Verdict = res.Verdict
		}
		merged.Strengths = append(merged.Strengths, res.Strengths...)
		merged.Suggestions = append(merged.Suggestions, res.Suggestions...)
		merged.Limitations = append(merged.Limitations, res.Limitations...)
		if merged.Summary == "" {
			merged.Summary = res.Summary
		}
	}
	merged.Issues = attributedIssues(reviewprofile.MergeFindings(findings))
	return merged, nil
}

// attributedIssues converts merged, profile-attributed findings back into the
// report's ReviewIssue shape. The source profile is folded into the message so
// the published report names it, without changing the severity the finding
// already carried.
func attributedIssues(findings []reviewprofile.Finding) []types.ReviewIssue {
	if len(findings) == 0 {
		return nil
	}
	out := make([]types.ReviewIssue, 0, len(findings))
	for _, f := range findings {
		message := f.Message
		if f.Profile != "" && !strings.Contains(message, "[perfil ") {
			message = fmt.Sprintf("%s [perfil %s]", message, f.Profile)
		}
		out = append(out, types.ReviewIssue{
			File:     f.File,
			Line:     f.Line,
			Severity: f.Severity,
			RuleID:   f.RuleID,
			Message:  message,
		})
	}
	return out
}
