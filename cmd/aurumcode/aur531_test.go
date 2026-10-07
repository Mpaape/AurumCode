package main

// AUR-531 through the session: the reachability explanation is a line of
// the review; the dependency report the gate judges is never touched, even
// when the model says there is no use and asks for a lower severity.

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/llm"
	reviewtools "github.com/Mpaape/AurumCode/internal/review/tools"
	"github.com/Mpaape/AurumCode/pkg/types"
)

type aur531Model struct{}

func (aur531Model) Complete(string, llm.Options) (llm.Response, error) { return llm.Response{}, nil }
func (aur531Model) Tokens(string) (int, error)                         { return 1, nil }
func (aur531Model) Name() string                                       { return "reach" }
func (aur531Model) CompleteWithTools(m []llm.Message, _ []llm.ToolSpec, _ llm.Options) (llm.ToolResponse, error) {
	for _, msg := range m {
		if msg.Role == llm.RoleTool {
			return llm.ToolResponse{Response: llm.Response{Text: `{"uses":"no","locations":[],"explanation":"sem uso","severity":"low","verdict":"approve"}`, TokensIn: 1, TokensOut: 1}}, nil
		}
	}
	return llm.ToolResponse{Response: llm.Response{TokensIn: 1, TokensOut: 1}, ToolCalls: []llm.ToolCall{{ID: "c1", Name: reviewtools.SearchToolName, Arguments: json.RawMessage(`{"query":"Run"}`)}}}, nil
}

// AC-002/AC-003 and MUT-001: "no use" plus a downgrade request leaves the
// report the gate reads byte for byte as it was; the review states the
// explanation and that the finding stands.
func TestAUR531ExplanationNeverChangesTheReport(t *testing.T) {
	aur579Repo(t)
	cfg, err := config.Parse([]byte("deliberation:\n  enabled: true\n  dependency_reachability: true\n"), "t")
	if err != nil {
		t.Fatal(err)
	}
	finding := dependencies.Finding{
		Change: dependencies.Change{Manifest: "go.mod", Ecosystem: "Go", Name: "example.org/lib", Head: "1.0.0"},
		Vuln:   dependencies.Vulnerability{ID: "GHSA-test-0002", Severity: "high", Link: "https://osv.dev/GHSA-test-0002"},
		Status: dependencies.StatusIntroduced,
	}
	report := &dependencies.Report{Manifests: []string{"go.mod"}, Findings: []dependencies.Finding{finding}}
	before, _ := json.Marshal(report)
	var errOut strings.Builder
	s := &reviewState{ctx: context.Background(), stderr: &errOut, stdout: io.Discard, cfg: cfg, depReport: report, result: &types.ReviewResult{}, reviewLanguage: "pt-BR", provider: aur531Model{}, scanRoot: ".", diff: &types.Diff{}}
	s.explainDependencyReach()
	after, _ := json.Marshal(s.depReport)
	if string(before) != string(after) || !reflect.DeepEqual(s.depReport.Findings[0], finding) {
		t.Fatalf("the explanation changed the report the gate judges:\nbefore %s\nafter  %s", before, after)
	}
	if len(s.result.Issues) != 0 || len(s.result.Limitations) != 0 || len(s.reachLines) != 1 {
		t.Fatalf("result = %+v reach = %v: the explanation must be its own section, never a limitation", s.result, s.reachLines)
	}
	line := s.reachLines[0]
	if !strings.Contains(line, "GHSA-test-0002") || !strings.Contains(line, "o modelo não achou uso") || !strings.Contains(line, "não mudam") {
		t.Fatalf("explanation line = %q; stderr=%s", line, errOut.String())
	}
}

// The reachability section survives the review's limitation filters: it is
// appended to the published body under its own localized heading.
func TestAUR531ReachHasItsOwnSection(t *testing.T) {
	body := appendReachSection("## AurumCode\n\nresumo\n", []string{"Alcance de GHSA-1 em lib (go.mod): usado em a.go:3"}, "pt-BR")
	if !strings.Contains(body, "### Alcance das dependências vulneráveis\n\n- Alcance de GHSA-1") || !strings.HasPrefix(body, "## AurumCode") {
		t.Fatalf("body = %q", body)
	}
	if en := appendReachSection("x", []string{"Reach of GHSA-1"}, "en"); !strings.Contains(en, "### Reach of vulnerable dependencies") {
		t.Fatalf("en body = %q", en)
	}
	if same := appendReachSection("x", nil, "pt-BR"); same != "x" {
		t.Fatalf("an empty section changed the body: %q", same)
	}
}
