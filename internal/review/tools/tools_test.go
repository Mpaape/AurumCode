package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/scanner"
)

type namedScanner struct{}

func (namedScanner) Name() string { return "toyscan" }
func (namedScanner) Run(context.Context, scanner.Request) (scanner.Report, error) {
	return scanner.Report{}, nil
}

func TestAUR580ManifestNamesEachToolWithItsCost(t *testing.T) {
	engine := scanner.Engine{Scanner: namedScanner{}, Category: "sast", Origin: "toy"}
	scan := NewScannerTool(engine, func(context.Context) scanner.Outcome {
		return scanner.Outcome{Engine: engine, Findings: []scanner.Finding{{Path: "a.go", Line: 2, RuleID: "r", Severity: "error", Message: "m"}}}
	})
	offers := []Offer{{Tool: scan, Cost: ScannerCost(30)}, {Tool: NewContextTool(".", []string{"a.go"}), Cost: ContextCost}, {Tool: NewSkillTool(map[string]SkillSection{"skill/x": {Title: "X", Text: "faça X"}}), Cost: SkillCost}}
	manifest := Manifest(offers)
	if len(manifest) != 3 || manifest[0].Name != "scanner_toyscan" || !strings.Contains(manifest[0].Cost, "até 30s") || manifest[1].Name != ContextToolName || manifest[2].Name != SkillToolName {
		t.Fatalf("manifest = %+v", manifest)
	}
	res, err := scan.Run(context.Background(), json.RawMessage(`{}`))
	if err != nil || !strings.Contains(res.Content, "r a.go:2 [error] m") || res.Digest == "" {
		t.Fatalf("scanner result = %+v, %v", res, err)
	}
	for _, o := range offers {
		if err := deliberation.ValidateArguments(o.Tool.Spec().Parameters, json.RawMessage(`{"extra":1}`)); err == nil {
			t.Errorf("%s accepted an undeclared argument", o.Tool.Spec().Name)
		}
	}
}

func TestAUR580InconclusiveScanIsAToolError(t *testing.T) {
	engine := scanner.Engine{Scanner: namedScanner{}}
	scan := NewScannerTool(engine, func(context.Context) scanner.Outcome {
		return scanner.Outcome{Engine: engine, Reason: "toyscan_unavailable"}
	})
	if _, err := scan.Run(context.Background(), json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "toyscan_unavailable") {
		t.Fatalf("err = %v", err)
	}
}

func TestAUR580ContextAndSkillToolsAnswerOnlyKnownNames(t *testing.T) {
	if _, err := NewContextTool(".", []string{"a.go"}).Run(context.Background(), json.RawMessage(`{"path":"/etc/passwd"}`)); err == nil {
		t.Fatal("the context tool read a path outside the diff")
	}
	skill := NewSkillTool(map[string]SkillSection{"skill/x": {Title: "X", Text: "faça X"}})
	if _, err := skill.Run(context.Background(), json.RawMessage(`{"rule_id":"skill/y"}`)); err == nil {
		t.Fatal("the skill tool answered an unknown section")
	}
	if res, err := skill.Run(context.Background(), json.RawMessage(`{"rule_id":"skill/x"}`)); err != nil || !strings.Contains(res.Content, "faça X") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
