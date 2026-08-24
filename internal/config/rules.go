package config

import "github.com/Mpaape/AurumCode/pkg/types"

// ApplyRuleConfig is the ONLY place a rule's enabled/severity state
// changes after the reviewer produced its findings, and it reads ONE
// input for that decision: cfg, the explicit, human-authored
// .aurumcode/config.yml. Its signature has no parameter through which
// ContextProvider text could reach it -- that is the enforcement
// mechanism for the package doc's security boundary, not a convention
// this function has to remember to honor.
//
// An issue whose RuleID cfg explicitly disables (rules.<id>.enabled:
// false) is dropped. An issue whose RuleID carries an explicit severity
// override adopts it, everything else about the issue unchanged. An issue
// whose RuleID has no entry in cfg.Rules -- every issue, whenever cfg is
// the zero-config Config{} Load returns for a missing file -- passes
// through unchanged: same slice contents, same order.
func ApplyRuleConfig(issues []types.ReviewIssue, cfg *Config) []types.ReviewIssue {
	if cfg == nil || len(cfg.Rules) == 0 {
		return issues
	}
	kept := make([]types.ReviewIssue, 0, len(issues))
	for _, issue := range issues {
		rc, ok := cfg.Rules[issue.RuleID]
		if !ok {
			kept = append(kept, issue)
			continue
		}
		if rc.Enabled != nil && !*rc.Enabled {
			continue
		}
		if rc.Severity != "" {
			issue.Severity = rc.Severity
		}
		kept = append(kept, issue)
	}
	return kept
}
