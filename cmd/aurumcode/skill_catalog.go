package main

import (
	"fmt"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// resolveSkillCatalog is the single place the review (--base and --pr) turns
// the skill directories into the catalog sent to the model: the central
// policy's skills layered over the repository's, selected by language and
// path from the changed files.
//
// A policy that cannot be loaded is an error (the policy is the
// organization's, so it fails closed before any model call, naming the
// file). In the repository, a malformed SKILL.md drops only that skill, with
// a warning naming it, and the other skills still apply; a repository skill
// directory that cannot be read at all is declared and the review continues
// without the repository's skills. Warnings reach the model and the review
// text.
// A nil source means that layer is absent.
func resolveSkillCatalog(policy, repo skills.Source) (*skills.Catalog, error) {
	var policySet, repoSet *skills.Set
	var warnings []string
	if policy != nil {
		set, err := skills.LoadSource(policy)
		if err != nil {
			return nil, fmt.Errorf("central policy: %w", err)
		}
		policySet = set
	}
	if repo != nil {
		set, skipped, err := skills.LoadSourceSkippingMalformed(repo)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("repository skills unavailable, review continues without them: %v", err))
		} else {
			repoSet = set
			warnings = append(warnings, skipped...)
		}
	}
	return skills.NewCatalog(policySet, repoSet, nil, warnings), nil
}

// excludeListedSkills drops from catalog every skill directory whose
// SKILL.md its own layer also lists in review.context.skills: the listing
// already sends that text to the model, and it must reach the prompt once.
func excludeListedSkills(catalog *skills.Catalog, policy, repo *config.Config) {
	if catalog == nil {
		return
	}
	if policy != nil {
		catalog.ExcludeListed(skills.LayerPolicy, policy.Review.Context.Skills)
	}
	if repo != nil {
		catalog.ExcludeListed(skills.LayerRepository, repo.Review.Context.Skills)
	}
}

// skillLayers is where the two skill layers of a review live: the
// repository at cwd and, when centralCfg is set, the central policy at
// policyDir.
type skillLayers struct {
	cwd, policyDir  string
	cfg, centralCfg *config.Config
}

// resolveSkillRules is the one computation of the skill catalog and of the
// dynamic skill-section rules a review accepts citations against, for the
// changed paths: the review session and the agent server's rules listing
// both call it, so they can never disagree on which rules apply.
func resolveSkillRules(l skillLayers, changed []string) (*skills.Catalog, map[string]review.Rule, error) {
	var policySource skills.Source
	if l.centralCfg != nil {
		policySource = localSkillSource(l.policyDir, "policy")
	}
	catalog, err := resolveSkillCatalog(policySource, localSkillSource(l.cwd, ""))
	if err != nil {
		return nil, nil, err
	}
	excludeListedSkills(catalog, l.centralCfg, l.cfg)
	catalogPolicy, catalogRepo := dynamicRulesFromCatalog(catalog, changed)
	policySkillRules := map[string]review.Rule{}
	if l.centralCfg != nil {
		policySkillRules = dynamicRulesFromLocalSkills(l.policyDir, l.centralCfg.Review.Context.Skills, gateOriginPolicy)
	}
	repoSkillRules := dynamicRulesFromLocalSkills(l.cwd, l.cfg.Review.Context.Skills, gateOriginRepo)
	return catalog, mergeDynamicRules(unionRules(policySkillRules, catalogPolicy), unionRules(repoSkillRules, catalogRepo)), nil
}

// localSkillSource reads root/.aurumcode/skills from disk, shown with a
// repository-relative label.
func localSkillSource(root, label string) skills.Source {
	return skills.DirSource{
		Dir:    filepath.Join(root, filepath.FromSlash(skills.DefaultDirName)),
		Prefix: filepath.ToSlash(filepath.Join(label, skills.DefaultDirName)),
	}
}

// skillSelectionNotices returns the declared warnings of the selection for
// the changed paths, redacted, for the review text.
func skillSelectionNotices(catalog *skills.Catalog, changed []string, filter *redaction.Filter) []string {
	if catalog == nil {
		return nil
	}
	warnings := catalog.Select(changed).Warnings
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		if filter != nil {
			w = filter.Redact(w)
		}
		out = append(out, w)
	}
	return out
}
