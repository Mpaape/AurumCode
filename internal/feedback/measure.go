package feedback

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CorpusTotals is the total line of an AUR-523 corpus report
// (tests/benchmark multilang-report.json).
type CorpusTotals struct {
	Recall             float64 `json:"recall"`
	Precision          float64 `json:"precision"`
	ApprovedWithDefect int     `json:"approved_with_defect"`
	Defects            int     `json:"defects"`
	Cases              int     `json:"cases"`
}

// CorpusReport is the part of the AUR-523 report the comparison reads.
type CorpusReport struct {
	Schema       string       `json:"schema"`
	CorpusSHA256 string       `json:"corpus_sha256"`
	PolicySHA256 string       `json:"policy_sha256"`
	Total        CorpusTotals `json:"total"`
}

// ParseCorpusReport decodes one report. A report without cases measured
// nothing and is refused.
func ParseCorpusReport(data []byte) (*CorpusReport, error) {
	var r CorpusReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("relatório do corpus: %w", err)
	}
	if r.Total.Cases <= 0 {
		return nil, fmt.Errorf("relatório do corpus sem casos medidos")
	}
	return &r, nil
}

// Comparison is the before/after measurement of a feedback pull request.
type Comparison struct {
	Markdown   string
	Regression bool
	Measured   bool
}

// Compare renders the before/after table. A missing side is stated as not
// measured, never omitted; an increase of "aprovado com defeito", or a drop
// of recall, is a regression and is highlighted.
func Compare(before, after *CorpusReport) Comparison {
	var b strings.Builder
	b.WriteString("## Medição do corpus (AUR-523)\n\n")
	if before == nil || after == nil {
		b.WriteString("**Não medido.** ")
		if before == nil {
			b.WriteString("Falta o relatório de antes. ")
		}
		if after == nil {
			b.WriteString("Falta o relatório de depois. ")
		}
		b.WriteString("Sem as duas medições esta PR não mostra efeito e não deve ser aprovada.\n")
		return Comparison{Markdown: b.String()}
	}
	regressed := after.Total.ApprovedWithDefect > before.Total.ApprovedWithDefect
	recallDrop := after.Total.Recall < before.Total.Recall
	b.WriteString("| Métrica | Antes | Depois |\n| --- | --- | --- |\n")
	fmt.Fprintf(&b, "| Recall | %.4f | %.4f%s |\n", before.Total.Recall, after.Total.Recall, mark(recallDrop))
	fmt.Fprintf(&b, "| Precisão | %.4f | %.4f |\n", before.Total.Precision, after.Total.Precision)
	fmt.Fprintf(&b, "| Aprovado com defeito | %d | %d%s |\n", before.Total.ApprovedWithDefect, after.Total.ApprovedWithDefect, mark(regressed))
	fmt.Fprintf(&b, "| Casos | %d | %d |\n", before.Total.Cases, after.Total.Cases)
	b.WriteString("\nMedição com o provedor falso do AUR-523, derivado dos rótulos dos casos: mede o efeito da política nas regras citáveis e no gate, não a qualidade de um modelo real.\n")
	fmt.Fprintf(&b, "\nCorpus `%s` → `%s`; política `%s` → `%s`.\n", shortSHA(before.CorpusSHA256), shortSHA(after.CorpusSHA256), shortSHA(before.PolicySHA256), shortSHA(after.PolicySHA256))
	if regressed {
		fmt.Fprintf(&b, "\n> **REGRESSÃO:** \"aprovado com defeito\" subiu de %d para %d. A política proposta deixa passar defeitos que a atual reprova.\n", before.Total.ApprovedWithDefect, after.Total.ApprovedWithDefect)
	}
	if recallDrop {
		fmt.Fprintf(&b, "\n> **REGRESSÃO:** o recall caiu de %.4f para %.4f.\n", before.Total.Recall, after.Total.Recall)
	}
	return Comparison{Markdown: b.String(), Regression: regressed || recallDrop, Measured: true}
}

func mark(on bool) string {
	if on {
		return " ⚠ regressão"
	}
	return ""
}
