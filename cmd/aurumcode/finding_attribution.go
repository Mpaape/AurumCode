package main

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// attributeLayerFindings names layer on every finding that cites a rule the
// layer won (won), in the review's language, the way a profile pass names
// its analyst: the label ("[engineering standards]") is folded into the
// message, once, so every surface that shows the finding -- the local
// report, the published review, the agent server -- says which layer the
// rule came from. Nothing else of the finding changes; with nothing won the
// issues come back untouched, so a review without the layer is unchanged.
func attributeLayerFindings(issues []types.ReviewIssue, won map[string]bool, layer config.Layer, language string) []types.ReviewIssue {
	if len(won) == 0 {
		return issues
	}
	label := i18n.Format(language, "review.layer_attribution", layer.Label(language))
	for i := range issues {
		if won[issues[i].RuleID] && !strings.Contains(issues[i].Message, label) {
			issues[i].Message += " " + label
		}
	}
	return issues
}
