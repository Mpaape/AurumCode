package review

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Two batches often say the same thing in slightly different words; the
// consolidated parecer keeps one of each, and keeps what differs.
func TestMergeResultsDropsNearDuplicateProse(t *testing.T) {
	a := &types.ReviewResult{
		Summary:     "A mudanca amplia o produto com novos fluxos.",
		Strengths:   []string{"A mudanca adiciona cobertura de testes para os novos fluxos de changelog."},
		Limitations: []string{"O CI fornecido esta verde; nao ha logs de falha adicionais para diagnosticar."},
		TestPlan:    []string{"Executar os testes focados nos novos fluxos."},
	}
	b := &types.ReviewResult{
		Summary:     "A mudanca amplia o produto com novos fluxos de dependencias.",
		Strengths:   []string{"A mudanca adiciona cobertura de testes para os novos fluxos de dependencias.", "O fluxo novo separa bem as responsabilidades."},
		Limitations: []string{"O CI fornecido esta verde e nao ha logs detalhados de falha.", "Nao houve execucao local dos testes."},
		TestPlan:    []string{"Executar os testes focados nos novos fluxos de dependencias."},
	}
	merged := mergeResults([]*types.ReviewResult{a, b})
	if strings.Count(merged.Summary, "A mudanca amplia") != 1 {
		t.Fatalf("summary repeated: %q", merged.Summary)
	}
	if len(merged.Strengths) != 2 || len(merged.Limitations) != 2 || len(merged.TestPlan) != 1 {
		t.Fatalf("near duplicates kept: strengths=%v limitations=%v tests=%v", merged.Strengths, merged.Limitations, merged.TestPlan)
	}
	if merged.Strengths[1] != "O fluxo novo separa bem as responsabilidades." || merged.Limitations[1] != "Nao houve execucao local dos testes." {
		t.Fatalf("a distinct item was dropped: %v / %v", merged.Strengths, merged.Limitations)
	}
}
