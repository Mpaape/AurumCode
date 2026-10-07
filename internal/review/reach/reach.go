// Package reach explains, for one advisory on a dependency, whether the
// reviewed code uses the vulnerable part (AUR-531): the model searches the
// repository with the read-only repository tools (AUR-526), in any
// language, and answers where the use appears. The code only orchestrates,
// validates the answer's shape and grounds every cited location in the
// reviewed revision. The explanation travels beside the finding: it never
// lowers or removes the finding, never changes its severity and never
// reaches the verdict ("no use found" is exactly the confidently wrong
// answer a gate cannot accept).
package reach

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/llm"
)

//go:embed prompt.md
var promptText string

var promptTemplate = template.Must(template.New("reach").Parse(promptText))

// The uses an explanation can state.
const (
	UsesYes     = "yes"
	UsesNo      = "no"
	UsesUnknown = "unknown"
)

// Request is one advisory on one package.
type Request struct {
	Package    string
	Ecosystem  string
	Manifest   string
	AdvisoryID string
	Summary    string
}

// Location is a cited use, file and 1-based line.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// answer is the shape read from the model; anything else it writes (a
// severity, a verdict) is never read.
type answer struct {
	Uses        string     `json:"uses"`
	Locations   []Location `json:"locations"`
	Explanation string     `json:"explanation"`
}

// Explanation is the grounded result for one request.
type Explanation struct {
	Request   Request
	Uses      string
	Locations []Location
	Text      string
	// Discarded are locations the model cited that the revision does not
	// hold.
	Discarded []Location
	// Reason says why there is no explanation ("" when there is one).
	Reason string
}

// Explainer runs one bounded deliberation per request.
type Explainer struct {
	Caller deliberation.Caller
	Tools  []deliberation.Tool
	Limits deliberation.Limits
	Redact func(string) string
	// Read reads a file of the reviewed revision (tools.Revision.Read).
	Read func(path string) ([]byte, error)
}

// Explain asks the model and grounds its answer. A failed deliberation or
// an answer of the wrong shape is an Explanation with a Reason, never "no
// use".
func (e Explainer) Explain(ctx context.Context, req Request) Explanation {
	out := Explanation{Request: req, Uses: UsesUnknown}
	var prompt bytes.Buffer
	if err := promptTemplate.Execute(&prompt, req); err != nil {
		out.Reason = "prompt: " + err.Error()
		return out
	}
	text := prompt.String()
	if e.Redact != nil {
		text = e.Redact(text)
	}
	session := deliberation.Session{Caller: e.Caller, Tools: e.Tools, Limits: e.Limits, Redact: e.Redact, Options: llm.Options{JSONMode: true, ResponseSchema: llm.SchemaOf(answer{}), ResponseSchemaName: "aurumcode_reach"}}
	result, err := session.Run(ctx, []llm.Message{{Role: llm.RoleUser, Content: text}})
	if err != nil {
		out.Reason = "deliberação: " + err.Error()
		return out
	}
	var a answer
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Answer.Text)), &a); err != nil {
		out.Reason = "resposta fora do formato"
		return out
	}
	return e.ground(out, a)
}

// ground keeps only locations the revision holds; "yes" without one
// grounded location is "unknown".
func (e Explainer) ground(out Explanation, a answer) Explanation {
	out.Text = strings.TrimSpace(a.Explanation)
	if e.Redact != nil {
		out.Text = e.Redact(out.Text)
	}
	for _, loc := range a.Locations {
		if e.exists(loc) {
			out.Locations = append(out.Locations, loc)
		} else {
			out.Discarded = append(out.Discarded, loc)
		}
	}
	switch {
	case a.Uses == UsesYes && len(out.Locations) > 0:
		out.Uses = UsesYes
	case a.Uses == UsesNo && len(out.Locations) == 0:
		out.Uses = UsesNo
	default:
		out.Uses = UsesUnknown
	}
	return out
}

func (e Explainer) exists(loc Location) bool {
	if e.Read == nil || loc.Line < 1 {
		return false
	}
	data, err := e.Read(loc.File)
	if err != nil {
		return false
	}
	return loc.Line <= bytes.Count(data, []byte("\n"))+1
}

// Line is the explanation as one line of the review: where the use is (or
// that none was found) and, always, that the finding and the verdict stand.
func Line(x Explanation) string {
	head := fmt.Sprintf("Dependencias: alcance de %s em %s (%s)", x.Request.AdvisoryID, x.Request.Package, x.Request.Manifest)
	const stands = "o achado, a severidade e o veredito não mudam"
	switch {
	case x.Reason != "":
		return fmt.Sprintf("%s: sem explicação (%s); %s", head, x.Reason, stands)
	case x.Uses == UsesYes:
		locs := make([]string, 0, len(x.Locations))
		for _, l := range x.Locations {
			locs = append(locs, fmt.Sprintf("%s:%d", l.File, l.Line))
		}
		return fmt.Sprintf("%s: usado em %s. %s; %s", head, strings.Join(locs, ", "), x.Text, stands)
	case x.Uses == UsesNo:
		return fmt.Sprintf("%s: o modelo não achou uso. %s; %s", head, x.Text, stands)
	}
	return fmt.Sprintf("%s: uso indeterminado. %s; %s", head, x.Text, stands)
}
