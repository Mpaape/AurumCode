// Package consumer is the QA of AurumCode installed in a separate consumer
// repository (AUR-512): the scenarios a real pull request must go through
// (cenarios.json), the evidence one run of run.sh records per scenario, and
// the verifier that compares them. Self-review of this repository is not
// evidence here: every piece of evidence names the consumer repository, the
// SHA and the workflow run it came from.
//
// The verifier never turns missing data into a pass. A scenario whose run
// could not be measured (billing, infrastructure, a fork that needs a second
// account) is "nao_medido" with its limitation; evidence without repository,
// SHA or run is rejected.
package consumer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Expectation is what the product must show for one scenario.
type Expectation struct {
	// Conclusion is the workflow conclusion: success or failure.
	Conclusion string `json:"conclusao"`
	// Statuses maps a commit status or check name to its expected state.
	Statuses map[string]string `json:"status,omitempty"`
	Language string            `json:"idioma,omitempty"`
	Mode     string            `json:"modo,omitempty"`
	// InlineSuggestion requires at least one eligible inline suggestion.
	InlineSuggestion bool `json:"sugestao_inline,omitempty"`
	// MaxComments bounds the product's comments on the PR after every
	// round (a repeated round must not multiply them). Zero: no bound.
	MaxComments int      `json:"comentarios_max,omitempty"`
	Contains    []string `json:"texto_contem,omitempty"`
	NotContains []string `json:"texto_nao_contem,omitempty"`
}

// Scenario is one consumer pull request.
type Scenario struct {
	ID          string `json:"id"`
	AC          string `json:"ac"`
	Description string `json:"descricao"`
	// Fixture is the directory under fixtures/ copied onto the consumer
	// branch; Fix, when set, is a second commit that fixes the problem.
	Fixture string `json:"fixture"`
	Fix     string `json:"correcao,omitempty"`
	// Workflow is the file under workflows/ installed as the consumer's
	// .github/workflows/aurumcode.yml on the branch.
	Workflow string `json:"workflow"`
	// Rounds re-runs the workflow on the same SHA (idempotence).
	Rounds int `json:"rodadas,omitempty"`
	// Manual scenarios need a setup run.sh cannot automate (a fork from a
	// second account); without supplied evidence they are not measured.
	Manual bool        `json:"manual,omitempty"`
	Expect Expectation `json:"espera"`
	// AfterFix is the expectation after the Fix commit.
	AfterFix *Expectation `json:"espera_apos_correcao,omitempty"`
}

// Evidence is what run.sh recorded for one scenario.
type Evidence struct {
	Scenario    string            `json:"cenario"`
	Repo        string            `json:"repo"`
	SHA         string            `json:"sha"`
	RunID       int64             `json:"run_id"`
	RunURL      string            `json:"run_url"`
	Measured    bool              `json:"medido"`
	Limitations []string          `json:"limitacoes"`
	Conclusion  string            `json:"conclusao"`
	Statuses    map[string]string `json:"status"`
	Language    string            `json:"idioma"`
	Mode        string            `json:"modo"`
	Inline      int               `json:"sugestoes_inline"`
	Comments    int               `json:"comentarios_aurum"`
	Text        string            `json:"texto"`
	// AfterFix is the evidence after the fix commit, when the scenario has one.
	AfterFix *Evidence `json:"apos_correcao,omitempty"`
}

// Outcome states of a verified scenario.
const (
	Passed      = "aprovado"
	Failed      = "reprovado"
	NotMeasured = "nao_medido"
)

// Result is the verdict for one scenario.
type Result struct {
	Scenario string   `json:"cenario"`
	State    string   `json:"estado"`
	Reasons  []string `json:"motivos,omitempty"`
}

// LoadScenarios reads cenarios.json and refuses duplicates and scenarios
// without an expectation that could fail.
func LoadScenarios(path string) ([]Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Scenario
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[string]bool{}
	for _, s := range out {
		switch {
		case s.ID == "" || seen[s.ID]:
			return nil, fmt.Errorf("cenário sem id ou repetido: %q", s.ID)
		case s.Expect.Conclusion != "success" && s.Expect.Conclusion != "failure":
			return nil, fmt.Errorf("%s: conclusao deve ser success ou failure", s.ID)
		case s.Fixture == "" || s.Workflow == "":
			return nil, fmt.Errorf("%s: fixture e workflow são obrigatórios", s.ID)
		}
		seen[s.ID] = true
	}
	return out, nil
}

// LoadEvidence reads every <scenario>.json of dir.
func LoadEvidence(dir string) (map[string]Evidence, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]Evidence{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var e Evidence
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out[e.Scenario] = e
	}
	return out, nil
}

// Verify compares the evidence of one scenario with its expectation.
func Verify(s Scenario, e Evidence, found bool) Result {
	r := Result{Scenario: s.ID}
	switch {
	case !found && s.Manual:
		return Result{Scenario: s.ID, State: NotMeasured, Reasons: []string{"cenário manual sem evidência fornecida"}}
	case !found:
		return Result{Scenario: s.ID, State: Failed, Reasons: []string{"sem evidência"}}
	case e.Scenario != s.ID:
		return Result{Scenario: s.ID, State: Failed, Reasons: []string{"evidência de outro cenário"}}
	case !e.Measured:
		if len(e.Limitations) == 0 {
			return Result{Scenario: s.ID, State: Failed, Reasons: []string{"não medido sem limitação registrada"}}
		}
		return Result{Scenario: s.ID, State: NotMeasured, Reasons: e.Limitations}
	}
	r.Reasons = append(r.Reasons, identityProblems(e)...)
	r.Reasons = append(r.Reasons, compare(s.Expect, e)...)
	if s.AfterFix != nil {
		if e.AfterFix == nil {
			r.Reasons = append(r.Reasons, "sem evidência depois da correção")
		} else {
			for _, why := range compare(*s.AfterFix, *e.AfterFix) {
				r.Reasons = append(r.Reasons, "após correção: "+why)
			}
		}
	}
	r.State = Passed
	if len(r.Reasons) > 0 {
		r.State = Failed
	}
	return r
}

func identityProblems(e Evidence) []string {
	var out []string
	if e.Repo == "" || !strings.Contains(e.Repo, "/") {
		out = append(out, "evidência sem repositório consumidor")
	}
	if len(e.SHA) < 7 {
		out = append(out, "evidência sem SHA")
	}
	if e.RunID <= 0 || e.RunURL == "" {
		out = append(out, "evidência sem run")
	}
	return out
}

// compare lists every way the evidence differs from the expectation.
func compare(x Expectation, e Evidence) []string {
	var out []string
	if e.Conclusion != x.Conclusion {
		out = append(out, fmt.Sprintf("conclusão %q, esperada %q", e.Conclusion, x.Conclusion))
	}
	names := make([]string, 0, len(x.Statuses))
	for k := range x.Statuses {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if got := e.Statuses[k]; got != x.Statuses[k] {
			out = append(out, fmt.Sprintf("status %s %q, esperado %q", k, got, x.Statuses[k]))
		}
	}
	if x.Language != "" && e.Language != x.Language {
		out = append(out, fmt.Sprintf("idioma %q, esperado %q", e.Language, x.Language))
	}
	if x.Mode != "" && e.Mode != x.Mode {
		out = append(out, fmt.Sprintf("modo %q, esperado %q", e.Mode, x.Mode))
	}
	if x.InlineSuggestion && e.Inline < 1 {
		out = append(out, "nenhuma sugestão inline")
	}
	if x.MaxComments > 0 && e.Comments > x.MaxComments {
		out = append(out, fmt.Sprintf("%d comentários do produto, máximo %d", e.Comments, x.MaxComments))
	}
	for _, want := range x.Contains {
		if !strings.Contains(e.Text, want) {
			out = append(out, fmt.Sprintf("texto sem %q", want))
		}
	}
	for _, bad := range x.NotContains {
		if strings.Contains(e.Text, bad) {
			out = append(out, fmt.Sprintf("texto contém %q", bad))
		}
	}
	return out
}

// Report verifies every scenario. ok is false when any scenario failed;
// a not-measured scenario never counts as a pass and is listed as such.
func Report(scenarios []Scenario, evidence map[string]Evidence) (results []Result, ok bool) {
	ok = true
	for _, s := range scenarios {
		e, found := evidence[s.ID]
		r := Verify(s, e, found)
		if r.State == Failed {
			ok = false
		}
		results = append(results, r)
	}
	return results, ok
}
