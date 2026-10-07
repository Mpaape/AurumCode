package config

import (
	"fmt"
	"strings"
)

// ReviewPresentationConfig is review.presentation: how findings are
// published, never whether they count. A blocking finding is always
// published one by one.
type ReviewPresentationConfig struct {
	// Collapse lists severities (info, warning, error) whose non-blocking
	// findings are condensed into one explained line of the review instead
	// of one comment each.
	Collapse []string `yaml:"collapse"`
}

// presentationSeverities are the severities review.presentation.collapse
// accepts.
var presentationSeverities = map[string]bool{"info": true, "warning": true, "error": true}

// ReviewPresentationCollapse returns the validated collapse set. An unknown
// severity is an error naming it, never silently ignored.
func (c *Config) ReviewPresentationCollapse() (map[string]bool, error) {
	if c == nil || len(c.Review.Presentation.Collapse) == 0 {
		return nil, nil
	}
	out := map[string]bool{}
	for _, raw := range c.Review.Presentation.Collapse {
		severity := strings.ToLower(strings.TrimSpace(raw))
		if !presentationSeverities[severity] {
			return nil, fmt.Errorf("unsupported review.presentation.collapse entry %q (use info, warning or error)", raw)
		}
		out[severity] = true
	}
	return out, nil
}
