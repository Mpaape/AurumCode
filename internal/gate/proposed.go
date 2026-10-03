package gate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// ProposedOwnerPlaceholder and ProposedExpiresPlaceholder are what a
// proposed exception leaves for the human who adopts it: the model may
// argue that a finding is wrong, it never names who answers for the
// exception nor until when.
const (
	ProposedOwnerPlaceholder   = "<dono a definir>"
	ProposedExpiresPlaceholder = "<AAAA-MM-DD>"
	// ProposedRepoPlaceholder stands in when the run has no verified
	// repository identity (a local --base run): an exception must name it.
	ProposedRepoPlaceholder = "<owner/repo a definir>"
)

// ProposedException is an exception the model's dispute suggests and the
// engine never applies: a human copies it into `exceptions` (with an owner
// and an expiry) or discards it.
type ProposedException struct {
	Repo, Rule, Path, Reason string
	Line                     int
	EvidenceID               string
}

// ProposeExceptions turns every disputed finding that still counts into a
// proposed exception. A demoted finding (gate.triage: model) already stopped
// counting and needs none.
func ProposeExceptions(disputed []types.ReviewIssue, demoted []Demotion, repo string) []ProposedException {
	gone := make(map[string]bool, len(demoted))
	for _, d := range demoted {
		gone[FindingOriginKey(d.Issue.RuleID, d.Issue.File, d.Issue.Line)] = true
	}
	if repo == "" {
		repo = ProposedRepoPlaceholder
	}
	var out []ProposedException
	for _, issue := range disputed {
		if gone[FindingOriginKey(issue.RuleID, issue.File, issue.Line)] || issue.Assessment == nil {
			continue
		}
		out = append(out, ProposedException{
			Repo: repo, Rule: issue.RuleID, Path: issue.File, Line: issue.Line,
			Reason:     strings.Join(strings.Fields(issue.Assessment.Justification), " "),
			EvidenceID: issue.Assessment.EvidenceID,
		})
	}
	return out
}

// RenderProposedExceptions renders proposals as the `exceptions` YAML a
// human can copy, each value quoted so a justification cannot break the
// structure. Nothing here is read back by the engine.
func RenderProposedExceptions(proposals []ProposedException) string {
	if len(proposals) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Excecoes propostas pelo modelo (nao aplicadas; o gate continua contando estes achados).\n")
	b.WriteString("Para aceitar, um humano copia o bloco para `exceptions` com dono e validade:\n")
	b.WriteString("exceptions:\n")
	for _, p := range proposals {
		fmt.Fprintf(&b, "  - repo: %s\n", strconv.Quote(p.Repo))
		fmt.Fprintf(&b, "    rule: %s\n", strconv.Quote(p.Rule))
		fmt.Fprintf(&b, "    path: %s\n", strconv.Quote(p.Path))
		fmt.Fprintf(&b, "    owner: %s\n", strconv.Quote(ProposedOwnerPlaceholder))
		fmt.Fprintf(&b, "    reason: %s\n", strconv.Quote(fmt.Sprintf("contestado pelo modelo (%s, linha %d): %s", p.EvidenceID, p.Line, p.Reason)))
		fmt.Fprintf(&b, "    expires: %s\n", strconv.Quote(ProposedExpiresPlaceholder))
	}
	return b.String()
}
