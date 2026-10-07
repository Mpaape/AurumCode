package feedback

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// Alert is the part of a GitHub code scanning alert the loop reads.
type Alert struct {
	Number           int    `json:"number"`
	State            string `json:"state"`
	DismissedReason  string `json:"dismissed_reason"`
	DismissedComment string `json:"dismissed_comment"`
	HTMLURL          string `json:"html_url"`
	Rule             struct {
		ID string `json:"id"`
	} `json:"rule"`
	MostRecentInstance struct {
		CommitSHA string `json:"commit_sha"`
		Location  struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
		} `json:"location"`
	} `json:"most_recent_instance"`
}

// dismissedFalsePositive is the one GitHub dismissal reason that says the
// finding was wrong. "won't fix" and "used in tests" accept a real finding;
// they are neither false nor true positives and produce no signal.
const dismissedFalsePositive = "false positive"

// FalsePositives turns the alerts of repo dismissed as false positives into
// signals with repo, SHA, skill and section of the finding.
func FalsePositives(filter *redaction.Filter, repo string, alerts []Alert) []Signal {
	var out []Signal
	for _, a := range alerts {
		if a.State != "dismissed" {
			continue
		}
		isFalsePositive := strings.EqualFold(strings.TrimSpace(a.DismissedReason), dismissedFalsePositive)
		if !isFalsePositive {
			continue
		}
		out = append(out, newSignal(filter, KindFalsePositive, repo, "alert:"+itoa(a.Number), Signal{
			SHA:         a.MostRecentInstance.CommitSHA,
			Rule:        a.Rule.ID,
			Path:        a.MostRecentInstance.Location.Path,
			Line:        a.MostRecentInstance.Location.StartLine,
			Description: a.DismissedComment,
			Source:      a.HTMLURL,
		}))
	}
	return out
}
