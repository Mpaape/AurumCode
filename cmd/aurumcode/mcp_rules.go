package main

import (
	"sort"

	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/mcpserver"
	"github.com/Mpaape/AurumCode/internal/review"
)

// Layer names of a skill in the agent server's rules listing.
const (
	layerPolicyName     = "policy"
	layerRepositoryName = "repository"
)

// ruleSetOf is the rules listing of one resolution.
func ruleSetOf(catalog *skills.Catalog, rules map[string]review.Rule, paths []string, policyDir string) mcpserver.RuleSet {
	set := mcpserver.RuleSet{PolicyActive: policyDir, Skills: []mcpserver.Skill{}, Rules: []mcpserver.Rule{}}
	for _, doc := range catalog.Docs(paths) {
		layer := layerRepositoryName
		if doc.Layer == skills.LayerPolicy {
			layer = layerPolicyName
		}
		set.Skills = append(set.Skills, mcpserver.Skill{Path: doc.Path, Layer: layer})
	}
	for _, r := range rules {
		set.Rules = append(set.Rules, mcpserver.Rule{ID: r.ID, Title: r.Title, Severity: r.Severity, Origin: r.Origin})
	}
	sort.Slice(set.Rules, func(i, j int) bool { return set.Rules[i].ID < set.Rules[j].ID })
	set.Notices = catalog.Select(paths).Warnings
	return set
}
