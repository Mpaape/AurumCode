package render

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func sampleResult() *types.ReviewResult {
	return &types.ReviewResult{
		Verdict: "changes_requested",
		Issues: []types.ReviewIssue{
			{File: "internal/render/summary.go", Severity: "error", Message: "a"},
			{File: "internal/render/mermaid.go", Severity: "error", Message: "b"},
			{File: "internal/render/summary.go", Severity: "warning", Message: "c"},
		},
		Suggestions: []types.ReviewSuggestion{
			{File: "internal/render/summary.go", Title: "tidy"},
		},
	}
}

func TestSummaryEnglishShape(t *testing.T) {
	out := Summary(sampleResult(), "en")
	for _, want := range []string{
		"## Code Review Summary",
		"**Verdict:** Changes requested",
		"**Findings by severity:**",
		"- error: 2",
		"- warning: 1",
		"- info: 0",
		"**Files touched:**",
		"- internal/render/mermaid.go",
		"- internal/render/summary.go",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("English summary missing %q:\n%s", want, out)
		}
	}
}

func TestSummaryPortugueseShape(t *testing.T) {
	out := Summary(sampleResult(), "pt-BR")
	for _, want := range []string{
		"## Resumo da Revisão de Código",
		"**Veredito:** Alterações solicitadas",
		"**Resultados por severidade:**",
		"- erro: 2",
		"- aviso: 1",
		"- informação: 0",
		"**Arquivos alterados:**",
		"- internal/render/mermaid.go",
		"- internal/render/summary.go",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Portuguese summary missing %q:\n%s", want, out)
		}
	}

	if out == Summary(sampleResult(), "en") {
		t.Error("pt-BR summary must differ from the English fallback")
	}
}

func TestSummaryPtAlias(t *testing.T) {
	if Summary(sampleResult(), "pt") != Summary(sampleResult(), "pt-BR") {
		t.Error("expected \"pt\" and \"pt-BR\" to produce the same summary")
	}
}

func TestSummaryDeterministic(t *testing.T) {
	if Summary(sampleResult(), "en") != Summary(sampleResult(), "en") {
		t.Error("summary must be deterministic across calls")
	}
}

func TestSummaryNilAndEmpty(t *testing.T) {
	out := Summary(nil, "en")
	if !strings.Contains(out, "**Verdict:** Unknown") {
		t.Errorf("nil result should report an unknown verdict:\n%s", out)
	}
	if !strings.Contains(out, "- None") {
		t.Errorf("nil result should report no touched files:\n%s", out)
	}
}
