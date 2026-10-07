package review

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// OutsideDiffFindingsKey is the engine-owned result.Metadata key carrying the
// model findings that point outside the changed lines but bring the full
// proof (evidence, impact, verification) and a resolvable rule citation.
//
// Such a finding has a destination of its own: a general pull request
// comment. It never anchors inline, never enters result.Issues and so never
// reaches the policy gate, the --fail-on threshold, the verdict, SARIF or
// the audit record. A finding outside the diff without that proof is still
// discarded and counted in issues_rejected_by_scope.
//
// The value is a JSON array of types.ReviewIssue. The engine always rewrites
// or removes the key after its gates, so a value a model reply supplied
// under the same name never survives.
const OutsideDiffFindingsKey = "outside_diff_findings"

// hasFullProof reports whether a finding carries the three pieces of proof
// the review prompt requests. Nonempty is the structural bar only; it is
// not proof of correctness.
func hasFullProof(issue types.ReviewIssue) bool {
	return strings.TrimSpace(issue.Evidence) != "" &&
		strings.TrimSpace(issue.Impact) != "" &&
		strings.TrimSpace(issue.Verification) != ""
}

// OutsideDiffFindings decodes the general-comment findings the engine kept
// for result, ordered by file and line. A missing or undecodable value
// yields none: the channel is informative, so failing to read it can only
// withhold a comment, never add a blocking finding.
func OutsideDiffFindings(result *types.ReviewResult) []types.ReviewIssue {
	if result == nil || result.Metadata == nil {
		return nil
	}
	return decodeOutsideDiff(result.Metadata[OutsideDiffFindingsKey])
}

func decodeOutsideDiff(raw string) []types.ReviewIssue {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var issues []types.ReviewIssue
	if err := json.Unmarshal([]byte(raw), &issues); err != nil {
		return nil
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		return issues[i].Line < issues[j].Line
	})
	return issues
}

// setOutsideDiffFindings writes issues under OutsideDiffFindingsKey, or
// removes the key when there are none (including any same-named value a
// model reply carried).
func setOutsideDiffFindings(metadata map[string]string, issues []types.ReviewIssue) {
	if len(issues) == 0 {
		delete(metadata, OutsideDiffFindingsKey)
		return
	}
	encoded, err := json.Marshal(issues)
	if err != nil {
		delete(metadata, OutsideDiffFindingsKey)
		return
	}
	metadata[OutsideDiffFindingsKey] = string(encoded)
}

// mergeOutsideDiff concatenates two batches' general-comment findings.
func mergeOutsideDiff(into map[string]string, from string) {
	merged := append(decodeOutsideDiff(into[OutsideDiffFindingsKey]), decodeOutsideDiff(from)...)
	setOutsideDiffFindings(into, merged)
}
