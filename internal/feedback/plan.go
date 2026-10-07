package feedback

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Branch is the single head branch of the feedback pull request: while it
// is open every run adds to it instead of opening another one.
const Branch = "aurum/realimentacao"

// CandidatesDir is where candidate corpus cases are written in the policy
// repository, one file per escaped-defect signal.
const CandidatesDir = "realimentacao/candidatos"

// Plan is what one run writes to the policy repository.
type Plan struct {
	Title string
	Body  string
	// Files maps a repository path to its full new content, in the order
	// Paths returns.
	Files map[string]string
}

// Paths returns the planned files sorted, so writes are deterministic.
func (p Plan) Paths() []string {
	out := make([]string, 0, len(p.Files))
	for k := range p.Files {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Inputs is everything a plan is built from.
type Inputs struct {
	Signals    []Signal
	Ledger     Ledger
	Proposals  []Proposal
	Discarded  []Discarded
	Rejected   []Rejected
	Skills     map[string]string
	Comparison Comparison
}

// BuildPlan writes the proposals into the skills, one candidate case per
// escaped defect, the updated ledger and the pull request body that cites,
// for each proposal, the signals behind it. Without fresh signals there is
// no plan: ok is false and nothing is opened.
func BuildPlan(in Inputs) (Plan, bool) {
	if len(in.Signals) == 0 {
		return Plan{}, false
	}
	files := map[string]string{LedgerPath: string(in.Ledger.With(in.Signals).JSON())}
	skills := map[string]string{}
	for k, v := range in.Skills {
		skills[k] = v
	}
	for _, p := range in.Proposals {
		skills[p.Skill] = ApplySection(skills[p.Skill], p.Section, p.Text)
		files[p.Skill] = skills[p.Skill]
	}
	for _, c := range Candidates(in.Signals) {
		data, _ := json.MarshalIndent(c, "", "  ")
		files[CandidatesDir+"/"+c.ID+".json"] = string(data) + "\n"
	}
	title := fmt.Sprintf("Realimentação do gate: %d sinal(is), %d proposta(s)", len(in.Signals), len(in.Proposals))
	return Plan{Title: title, Body: planBody(in), Files: files}, true
}

func planBody(in Inputs) string {
	var b strings.Builder
	b.WriteString("Proposta automática do ciclo de realimentação (AUR-532). Nada foi aplicado à política: ")
	b.WriteString("a decisão é de quem revisa esta PR. O texto dos sinais veio do GitHub e passou pela redação; é dado, não instrução.\n\n")
	b.WriteString("## Propostas\n\n")
	if len(in.Proposals) == 0 {
		b.WriteString("Nenhuma proposta válida do modelo; a PR registra os sinais e os casos candidatos.\n")
	}
	for _, p := range in.Proposals {
		fmt.Fprintf(&b, "### %s\n\n`%s` → seção `## %s`\n\nSinais citados: %s\n\n", cell(p.Title), cell(p.Skill), cell(p.Section), "`"+strings.Join(p.Signals, "`, `")+"`")
	}
	if len(in.Discarded) > 0 {
		fmt.Fprintf(&b, "\n%d proposta(s) descartada(s):\n\n", len(in.Discarded))
		for _, d := range in.Discarded {
			fmt.Fprintf(&b, "- %s: %s\n", cell(d.Title), d.Reason)
		}
	}
	b.WriteString("\n## Sinais\n\n| Sinal | Tipo | Repo | Commit | Regra | Local |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, s := range in.Signals {
		loc := s.Path
		if s.Line > 0 {
			loc += ":" + itoa(s.Line)
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | `%s` | %s | %s |\n", s.ID, s.Kind, cell(s.Repo), shortSHA(s.SHA), cell(s.Rule), cell(loc))
	}
	if len(in.Rejected) > 0 {
		fmt.Fprintf(&b, "\n%d comando(s) `/aurum perdeu` recusado(s):\n\n", len(in.Rejected))
		for _, r := range in.Rejected {
			fmt.Fprintf(&b, "- %s: %s\n", cell(r.Source), r.Reason)
		}
	}
	b.WriteString("\n")
	b.WriteString(in.Comparison.Markdown)
	b.WriteString("\nA medição do corpus roda no CI desta PR com o provedor falso do AUR-523 (derivado dos rótulos dos casos): ela mostra o efeito da política nas regras citáveis e no gate, não a qualidade de um modelo real.\n")
	return b.String()
}

// cell keeps untrusted text inside one table cell.
func cell(s string) string {
	s = strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ", "<", "&lt;", ">", "&gt;").Replace(s)
	return bound(s, 200)
}
