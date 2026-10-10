package main

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// skillSources are the skill directories of one review, by layer; a layer
// without a source (or with a nil one) is absent.
type skillSources map[config.LayerName]skills.Source

// resolveSkillCatalog is the single place the review (--base and --pr) turns
// the skill directories into the catalog sent to the model: every layer
// loaded by its own tolerance (loadLayerSkills), the same-name precedence
// applied in config.LayerOrder (shadowSameNames), the central policy's
// skills layered over the rest by selector (skills.NewCatalog), and the
// skills a layer also lists in review.context.skills sent once
// (excludeListedSkills). Warnings reach the model and the review text;
// shadowed are the precedence warnings alone, for the caller to print on
// the terminal.
func resolveSkillCatalog(src skillSources, policyCfg, repoCfg *config.Config, language string) (catalog *skills.Catalog, shadowed []string, err error) {
	sets := layerSets{}
	var warnings []string
	for _, name := range config.LayerOrder {
		if src[name] == nil {
			continue
		}
		set, loadWarnings, err := loadLayerSkills(config.Layers[name], src[name], language)
		if err != nil {
			return nil, nil, err
		}
		sets[name] = set
		warnings = append(warnings, loadWarnings...)
	}
	protected := skillDirNames(sets[config.LayerStandards])
	shadowed = shadowSameNames(sets, language)
	warnings = append(warnings, shadowed...)
	catalog = skills.NewCatalog(sets[config.LayerPolicy], belowPolicy(sets), nil, warnings)
	excludeListedSkills(catalog, policyCfg, repoCfg, protected)
	return catalog, shadowed, nil
}

// excludeListedSkills drops from catalog every skill directory whose
// SKILL.md its own layer also lists in review.context.skills: the listing
// already sends that text to the model, and it must reach the prompt once.
// The standards layer's skills sit in the catalog's repository layer, so a
// repository listing that names one of their directories (protected) is
// not taken as the standards skill's own listing.
func excludeListedSkills(catalog *skills.Catalog, policy, repo *config.Config, protected map[string]bool) {
	if catalog == nil {
		return
	}
	if policy != nil {
		catalog.ExcludeListed(skills.LayerPolicy, policy.Review.Context.Skills)
	}
	if repo != nil {
		catalog.ExcludeListed(skills.LayerRepository, unprotectedListing(repo.Review.Context.Skills, protected))
	}
}

// unprotectedListing is listed without the entries that name a protected
// skill directory (.aurumcode/skills/<name>/SKILL.md). With nothing
// protected, listed is returned as is.
func unprotectedListing(listed []string, protected map[string]bool) []string {
	if len(protected) == 0 {
		return listed
	}
	out := make([]string, 0, len(listed))
	for _, p := range listed {
		clean := path.Clean(strings.TrimSpace(filepath.ToSlash(p)))
		if dir, ok := strings.CutSuffix(clean, "/"+skills.DocName); ok && path.Dir(dir) == skills.DefaultDirName && protected[path.Base(dir)] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// skillLayers is where the skill layers of a review live: the repository
// at cwd, the central policy at policyDir when centralCfg is set, and the
// engineering standards layer when standards is set. language is the
// review's, for the layering warnings.
type skillLayers struct {
	cwd, policyDir  string
	cfg, centralCfg *config.Config
	standards       *centralLayer
	language        string
}

// skillResolution is one computation of the skill catalog and of the
// dynamic skill-section rules a review accepts citations against.
type skillResolution struct {
	catalog *skills.Catalog
	rules   map[string]review.Rule
	// standardsRules are the rule ids the engineering standards layer won.
	standardsRules map[string]bool
	// shadowed are the layers' same-name precedence warnings.
	shadowed []string
}

// resolveSkillRules is the one computation of the skill catalog and of the
// dynamic skill-section rules a review accepts citations against, for the
// changed paths: the review session and the agent server's rules listing
// both call it, so they can never disagree on which rules apply.
func resolveSkillRules(l skillLayers, changed []string) (skillResolution, error) {
	src := skillSources{config.LayerRepository: localSkillSource(l.cwd, "")}
	if l.centralCfg != nil {
		src[config.LayerPolicy] = localSkillSource(l.policyDir, string(config.LayerPolicy))
	}
	if source := l.standards.skillSource(); source != nil {
		src[config.LayerStandards] = source
	}
	catalog, shadowed, err := resolveSkillCatalog(src, l.centralCfg, l.cfg, l.language)
	if err != nil {
		return skillResolution{}, err
	}
	layered := dynamicRulesFromCatalog(catalog, changed, l.centralCfg != nil)
	if l.centralCfg != nil {
		layered[config.LayerPolicy] = unionRules(dynamicRulesFromLocalSkills(l.policyDir, l.centralCfg.Review.Context.Skills, gateOriginPolicy), layered[config.LayerPolicy])
	}
	layered[config.LayerRepository] = unionRules(dynamicRulesFromLocalSkills(l.cwd, l.cfg.Review.Context.Skills, gateOriginRepo), layered[config.LayerRepository])
	return skillResolution{catalog: catalog, rules: layered.merged(), standardsRules: layered.wonBy(config.LayerStandards), shadowed: shadowed}, nil
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
