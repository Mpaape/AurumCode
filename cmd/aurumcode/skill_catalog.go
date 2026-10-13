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

// catalogResolution is one resolution of the skill catalog.
type catalogResolution struct {
	catalog *skills.Catalog
	// shadowed are the layers' same-name precedence warnings.
	shadowed []string
	// takenAbove are the skill directory names the layers above the
	// repository declare, when the engineering standards layer is declared;
	// nil otherwise, so a review without the layer is unchanged.
	takenAbove map[string]bool
}

// resolveSkillCatalog is the single place the review (--base and --pr) turns
// the skill directories into the catalog sent to the model: every layer
// loaded by its own tolerance (loadLayerSkills), the same-name precedence
// applied in config.LayerOrder (shadowSameNames), the central policy's
// skills layered over the rest by selector (skills.NewCatalog), and the
// skills a layer also lists in review.context.skills sent once
// (excludeListedSkills). Warnings reach the model and the review text;
// shadowed are the precedence warnings alone, for the caller to print on
// the terminal.
func resolveSkillCatalog(src skillSources, policyCfg, repoCfg *config.Config, language string) (catalogResolution, error) {
	sets := layerSets{}
	var warnings []string
	for _, name := range config.LayerOrder {
		if src[name] == nil {
			continue
		}
		set, loadWarnings, err := loadLayerSkills(config.Layers[name], src[name], language)
		if err != nil {
			return catalogResolution{}, err
		}
		sets[name] = set
		warnings = append(warnings, loadWarnings...)
	}
	res := catalogResolution{takenAbove: namesAboveRepository(sets)}
	protected := skillDirNames(sets[config.LayerStandards])
	res.shadowed = shadowSameNames(sets, language)
	warnings = append(warnings, res.shadowed...)
	res.catalog = skills.NewCatalog(sets[config.LayerPolicy], belowPolicy(sets), nil, warnings)
	excludeListedSkills(res.catalog, policyCfg, repoCfg, protected)
	return res, nil
}

// repositoryConfig is cfg, the repository's effective config, without the
// review.context.skills entries that name a skill directory a layer above
// declares (AC-003): a shadowed skill's text must not reach the model
// through the listing either, and its sections must not become repository
// rules. The names are what the layers above DECLARE, so the listing cannot
// resurrect a shadowed skill even when the repository's own directory could
// not be listed. Without a name taken above, cfg comes back as is (the same
// pointer): a review without the standards layer is unchanged.
func (r catalogResolution) repositoryConfig(cfg *config.Config) *config.Config {
	if cfg == nil || len(r.takenAbove) == 0 {
		return cfg
	}
	c := *cfg
	c.Review.Context.Skills = listingWithout(cfg.Review.Context.Skills, r.takenAbove)
	return &c
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

// unprotectedListing is the repository's listing without the entries that
// name a protected skill directory, the standards layer's: the only
// defense between a repository listing and ExcludeListed. With nothing
// protected, listed is returned as is.
func unprotectedListing(listed []string, protected map[string]bool) []string {
	return listingWithout(listed, protected)
}

// listingWithout is listed without the entries that name a skill directory
// in names (.aurumcode/skills/<name>/SKILL.md). With no names, listed is
// returned as is.
func listingWithout(listed []string, names map[string]bool) []string {
	if len(names) == 0 {
		return listed
	}
	out := make([]string, 0, len(listed))
	for _, p := range listed {
		clean := path.Clean(strings.TrimSpace(filepath.ToSlash(p)))
		if dir, ok := strings.CutSuffix(clean, "/"+skills.DocName); ok && path.Dir(dir) == skills.DefaultDirName && names[path.Base(dir)] {
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
	// cfg is the repository's effective config for everything after the
	// resolution: its listing drops the skills a layer above took
	// (catalogResolution.repositoryConfig).
	cfg   *config.Config
	rules map[string]review.Rule
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
	src := centralSkillSources(l.policyDir, l.centralCfg, l.standards)
	src[config.LayerRepository] = localSkillSource(l.cwd, "")
	res, err := resolveSkillCatalog(src, l.centralCfg, l.cfg, l.language)
	if err != nil {
		return skillResolution{}, err
	}
	cfg := res.repositoryConfig(l.cfg)
	layered := dynamicRulesFromCatalog(res.catalog, changed, l.centralCfg != nil)
	if l.centralCfg != nil {
		layered[config.LayerPolicy] = unionRules(dynamicRulesFromLocalSkills(l.policyDir, l.centralCfg.Review.Context.Skills, gateOriginPolicy), layered[config.LayerPolicy])
	}
	layered[config.LayerRepository] = unionRules(dynamicRulesFromLocalSkills(l.cwd, cfg.Review.Context.Skills, gateOriginRepo), layered[config.LayerRepository])
	return skillResolution{catalog: res.catalog, cfg: cfg, rules: layered.merged(), standardsRules: layered.wonBy(config.LayerStandards), shadowed: res.shadowed}, nil
}

// centralSkillSources are the skill sources of the two central layers:
// the policy's when a policy governs the run (centralCfg set), the
// engineering standards layer's when one was declared. The caller adds the
// repository's own source, local or remote.
func centralSkillSources(policyDir string, centralCfg *config.Config, standards *centralLayer) skillSources {
	src := skillSources{}
	if centralCfg != nil {
		src[config.LayerPolicy] = localSkillSource(policyDir, string(config.LayerPolicy))
	}
	if source := standards.skillSource(); source != nil {
		src[config.LayerStandards] = source
	}
	return src
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
