package feedback

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/i18n"
)

// The pull request title keeps its earlier bytes: the realimentacao command
// has no review language, so the catalog's default applies (AUR-615).
func TestAUR615PlanTitleKeepsTheDefaultBytes(t *testing.T) {
	for _, tc := range []struct {
		signals, proposals int
		want               string
	}{
		{2, 1, "Realimentação do gate: 2 sinal(is), 1 proposta(s)"},
		{1, 0, "Realimentação do gate: 1 sinal(is), 0 proposta(s)"},
	} {
		if got := planTitle(tc.signals, tc.proposals); got != tc.want {
			t.Errorf("planTitle(%d, %d) = %q, want %q", tc.signals, tc.proposals, got, tc.want)
		}
	}
	if got, want := i18n.Counted("", "feedback.plan_discarded", 2), "2 proposta(s) descartada(s):"; got != want {
		t.Errorf("discarded heading = %q, want %q", got, want)
	}
	if got, want := i18n.Counted("pt-BR", "feedback.plan_discarded", 1), "1 proposta descartada:"; got != want {
		t.Errorf("discarded heading pt-BR = %q, want %q", got, want)
	}
}
