package feedback

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// AuditFinding is a blocking finding of an AUR-521 audit record.
type AuditFinding struct {
	RuleID string `json:"rule_id"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
}

// AuditRun is one audit record of a pull request review, as uploaded by the
// review workflow (artifact aurumcode-audit-<pr>).
type AuditRun struct {
	PR       int            `json:"-"`
	Created  string         `json:"-"`
	Reviewed string         `json:"reviewed_sha"`
	Blocking []AuditFinding `json:"blocking_findings"`
}

// ChangedLines returns, per path, the old-side lines that from..to removed
// or rewrote.
type ChangedLines func(repo, from, to string) (map[string]map[int]bool, error)

// TruePositives compares consecutive audit records of the same pull request:
// a blocking finding of one run that is gone in the next run, whose diff
// rewrote the flagged line, was fixed -- a true positive. A finding that
// only moved (same rule and path still blocking) or vanished without its
// line changing is not counted.
func TruePositives(filter *redaction.Filter, repo string, runs []AuditRun, changed ChangedLines) ([]Signal, error) {
	byPR := map[int][]AuditRun{}
	for _, r := range runs {
		if strings.TrimSpace(r.Reviewed) != "" {
			byPR[r.PR] = append(byPR[r.PR], r)
		}
	}
	prs := make([]int, 0, len(byPR))
	for pr := range byPR {
		prs = append(prs, pr)
	}
	sort.Ints(prs)
	var out []Signal
	for _, pr := range prs {
		list := byPR[pr]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Created < list[j].Created })
		for i := 0; i+1 < len(list); i++ {
			found, err := fixedBetween(filter, repo, pr, list[i], list[i+1], changed)
			if err != nil {
				return nil, err
			}
			out = append(out, found...)
		}
	}
	return out, nil
}

func fixedBetween(filter *redaction.Filter, repo string, pr int, before, after AuditRun, changed ChangedLines) ([]Signal, error) {
	if before.Reviewed == after.Reviewed || len(before.Blocking) == 0 {
		return nil, nil
	}
	still := map[string]bool{}
	for _, f := range after.Blocking {
		still[f.RuleID+"\x00"+f.Path] = true
	}
	var gone []AuditFinding
	for _, f := range before.Blocking {
		if !still[f.RuleID+"\x00"+f.Path] && f.Line > 0 {
			gone = append(gone, f)
		}
	}
	if len(gone) == 0 {
		return nil, nil
	}
	lines, err := changed(repo, before.Reviewed, after.Reviewed)
	if err != nil {
		return nil, err
	}
	var out []Signal
	for _, f := range gone {
		if !lines[f.Path][f.Line] {
			continue
		}
		key := "audit:" + strconv.Itoa(pr) + ":" + before.Reviewed + ":" + f.RuleID + ":" + f.Path + ":" + itoa(f.Line)
		out = append(out, newSignal(filter, KindTruePositive, repo, key, Signal{
			SHA:    after.Reviewed,
			Rule:   f.RuleID,
			Path:   f.Path,
			Line:   f.Line,
			Source: "pr " + strconv.Itoa(pr) + ": " + shortSHA(before.Reviewed) + ".." + shortSHA(after.Reviewed),
		}))
	}
	return out, nil
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// RemovedOldLines reads a unified diff patch (GitHub compare "patch") and
// returns the old-side line numbers it removed or rewrote.
func RemovedOldLines(patch string) map[int]bool {
	out := map[int]bool{}
	old := 0
	inHunk := false
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			start, ok := hunkOldStart(line)
			old, inHunk = start, ok
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "-"):
			out[old] = true
			old++
		case strings.HasPrefix(line, "+"):
		case strings.HasPrefix(line, `\`):
		default:
			old++
		}
	}
	return out
}

// hunkOldStart parses "@@ -12,3 +12,4 @@".
func hunkOldStart(header string) (int, bool) {
	fields := strings.Fields(header)
	if len(fields) < 3 || !strings.HasPrefix(fields[1], "-") {
		return 0, false
	}
	num := strings.TrimPrefix(fields[1], "-")
	if i := strings.Index(num, ","); i >= 0 {
		num = num[:i]
	}
	n, err := strconv.Atoi(num)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
