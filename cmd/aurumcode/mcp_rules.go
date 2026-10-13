package main

import (
	"sort"

	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/mcpserver"
	"github.com/Mpaape/AurumCode/internal/review"
)

// ruleSetOf is the rules listing of one resolution: each selected skill
// with the layer it came from (config.LayerName: "policy", "standards" or
// "repository") and the rules in id order. standardsDir is the engineering
// standards layer's directory, "" without one.
func ruleSetOf(catalog *skills.Catalog, rules map[string]review.Rule, paths []string, policyDir, standardsDir string) mcpserver.RuleSet {
	set := mcpserver.RuleSet{PolicyActive: policyDir, StandardsActive: standardsDir, Skills: []mcpserver.Skill{}, Rules: []mcpserver.Rule{}}
	for _, doc := range catalog.Docs(paths) {
		set.Skills = append(set.Skills, mcpserver.Skill{Path: doc.Path, Layer: string(layerOfDoc(doc))})
	}
	for _, r := range rules {
		set.Rules = append(set.Rules, mcpserver.Rule{ID: r.ID, Title: r.Title, Severity: r.Severity, Origin: r.Origin})
	}
	sort.Slice(set.Rules, func(i, j int) bool { return set.Rules[i].ID < set.Rules[j].ID })
	set.Notices = catalog.Select(paths).Warnings
	return set
}
