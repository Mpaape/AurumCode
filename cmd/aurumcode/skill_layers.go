// The skill layers of a review in the order config.LayerOrder declares:
// central policy > engineering standards > repository. This file loads
// each layer's skills by the layer's own tolerance, drops a lower layer's
// skill whose directory name a higher layer also declares (the directory
// name is the skill's identity: it prefixes every rule id the skill
// yields), tells which layer a selected skill came from and merges the
// layers' citable rules. Without an engineering standards layer the name
// contests do not run, and the policy and the repository keep the selector
// rule they always had (skills.NewCatalog).
package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review"
)

// layerSets are the loaded skill sets by layer; a missing or nil set is an
// absent layer.
type layerSets map[config.LayerName]*skills.Set

// loadLayerSkills reads one layer's skill directories by the layer's
// tolerance. A central layer fails closed on a malformed SKILL.md, naming
// the layer, because its skills are gate-relevant content the repository
// cannot override. In the repository a malformed SKILL.md drops only that
// skill, with a warning naming it, and a skill directory that cannot be
// read at all is declared and the review continues without the
// repository's skills.
func loadLayerSkills(layer config.Layer, src skills.Source, language string) (*skills.Set, []string, error) {
	if layer.Central {
		set, err := skills.LoadSource(src)
		if err != nil {
			return nil, nil, layerLoadError(layer, language, err)
		}
		return set, nil, nil
	}
	set, skipped, err := skills.LoadSourceSkippingMalformed(src)
	if err != nil {
		return nil, []string{fmt.Sprintf("repository skills unavailable, review continues without them: %v", err)}, nil
	}
	return set, skipped, nil
}

// shadowSameNames applies the layers' precedence by skill directory name,
// in place, and returns the warnings in language, each naming the layer
// that lost and the layer that won. Every pair of layers is judged against
// what the higher layer DECLARED, so a repository skill named like a
// standards skill is dropped even when the policy took that name from the
// standards layer. It runs only when the engineering standards layer is
// declared: without it nothing is dropped, so a review that never declared
// the layer is unchanged.
func shadowSameNames(sets layerSets, language string) []string {
	if sets[config.LayerStandards] == nil {
		return nil
	}
	declared := map[config.LayerName]map[string]bool{}
	for _, name := range config.LayerOrder {
		declared[name] = skillDirNames(sets[name])
	}
	var warnings []string
	for i, upper := range config.LayerOrder {
		for _, lower := range config.LayerOrder[i+1:] {
			var dropped []string
			sets[lower], dropped = withoutNames(sets[lower], declared[upper])
			for _, name := range dropped {
				warnings = append(warnings, i18n.Format(language, "layer.skill_shadowed", name, config.Layers[lower].Label(language), config.Layers[upper].Label(language)))
			}
		}
	}
	return warnings
}

// belowPolicy joins the sets below the policy, in layer order, into the
// catalog's lower layer; nil when none was loaded.
func belowPolicy(sets layerSets) *skills.Set {
	var out *skills.Set
	for _, name := range config.LayerOrder[1:] {
		set := sets[name]
		if set == nil {
			continue
		}
		if out == nil {
			out = &skills.Set{}
		}
		out.Skills = append(out.Skills, set.Skills...)
	}
	return out
}

// skillDirNames is the set of skill directory names of set.
func skillDirNames(set *skills.Set) map[string]bool {
	names := map[string]bool{}
	if set == nil {
		return names
	}
	for _, sk := range set.Skills {
		names[path.Base(sk.Dir)] = true
	}
	return names
}

// withoutNames is set without the skills whose directory name is taken,
// and the names it dropped, in set order. A nil set stays nil.
func withoutNames(set *skills.Set, taken map[string]bool) (*skills.Set, []string) {
	if set == nil {
		return nil, nil
	}
	kept := &skills.Set{}
	var dropped []string
	for _, sk := range set.Skills {
		if name := path.Base(sk.Dir); taken[name] {
			dropped = append(dropped, name)
			continue
		}
		kept.Skills = append(kept.Skills, sk)
	}
	return kept, dropped
}

// layerOfDoc is the layer a selected skill document came from: the catalog
// marks the policy's; a central layer's label starts with the layer's name
// (localSkillSource, centralLayer.skillSource); the rest is the
// repository's, whose labels start with .aurumcode/.
func layerOfDoc(doc skills.Doc) config.LayerName {
	if doc.Layer == skills.LayerPolicy {
		return config.LayerPolicy
	}
	for _, name := range config.LayerOrder {
		if config.Layers[name].Central && strings.HasPrefix(doc.Path, string(name)+"/") {
			return name
		}
	}
	return config.LayerRepository
}

// layeredRules are one run's dynamic skill-section rules by layer.
type layeredRules map[config.LayerName]map[string]review.Rule

// add puts rule into layer name's set.
func (r layeredRules) add(name config.LayerName, rule review.Rule) {
	if r[name] == nil {
		r[name] = map[string]review.Rule{}
	}
	r[name][rule.ID] = rule
}

// merged combines the layers into the one rule set the review accepts
// citations against, lowest layer first so on an id collision the higher
// layer's rule overwrites (CR-TRUST-001: a repository can never shadow a
// central rule id with its own, lower-trust copy).
func (r layeredRules) merged() map[string]review.Rule {
	out := map[string]review.Rule{}
	for i := len(config.LayerOrder) - 1; i >= 0; i-- {
		for id, rule := range r[config.LayerOrder[i]] {
			out[id] = rule
		}
	}
	return out
}

// wonBy names the rule ids layer name won: declared by it and by no layer
// above it, so the review text can attribute a finding that cites one.
func (r layeredRules) wonBy(name config.LayerName) map[string]bool {
	won := map[string]bool{}
	for id := range r[name] {
		if !r.declaredAbove(name, id) {
			won[id] = true
		}
	}
	return won
}

// declaredAbove reports whether a layer above name declares id.
func (r layeredRules) declaredAbove(name config.LayerName, id string) bool {
	for _, upper := range config.LayerOrder {
		if upper == name {
			return false
		}
		if _, ok := r[upper][id]; ok {
			return true
		}
	}
	return false
}
