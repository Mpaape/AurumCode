package render

import "strings"

// NoIssuesLine is the verdict line of a review that found nothing AND whose
// every source concluded.
const NoIssuesLine = "No issues found."

// NoFindingsLine is the closing line of a review with no findings (AUR-572).
// reason is the gate's comma-joined, machine-readable inconclusive reasons
// ("sast_execution_error", "partial_coverage", ...); empty means every
// source concluded and the line is NoIssuesLine. A source that did not
// conclude cannot vouch that nothing is there, so the line names what stayed
// without a conclusion instead of claiming a clean review.
func NoFindingsLine(reason string) string {
	var names []string
	seen := map[string]bool{}
	for _, r := range strings.Split(reason, ",") {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		names = append(names, r)
	}
	if len(names) == 0 {
		return NoIssuesLine
	}
	return "Sem achados nas fontes concluídas; inconclusivo: " + strings.Join(names, ", ")
}
