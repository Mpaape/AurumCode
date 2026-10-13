package main

import "testing"

// The counted terminal lines keep their earlier bytes in the default
// language for any count and agree in number in Portuguese (AUR-615).
func TestAUR615CountedLinesKeepDefaultBytesAndAgreeInPortuguese(t *testing.T) {
	for _, tc := range []struct{ name, got, want string }{
		{"publish default one", publishFailuresText("", 1), "1 comentario(s) falharam ao publicar"},
		{"publish en two", publishFailuresText("en", 2), "2 comentario(s) falharam ao publicar"},
		{"publish pt one", publishFailuresText("pt-BR", 1), "1 comentário falhou ao publicar"},
		{"publish pt three", publishFailuresText("pt-BR", 3), "3 comentários falharam ao publicar"},
		{"deps default", dependenciesSummaryText("", 1, 2, 0), "1 manifesto(s), 2 pacote(s), 0 advisory(s)"},
		{"deps pt", dependenciesSummaryText("pt-BR", 1, 2, 0), "1 manifesto, 2 pacotes, 0 advisories"},
		{"deps pt one each", dependenciesSummaryText("pt", 1, 1, 1), "1 manifesto, 1 pacote, 1 advisory"},
		{"xbom default", xbomSummaryText("", 5, 1), "5 componente(s), 1 descartado(s) sem evidencia"},
		{"xbom pt", xbomSummaryText("pt", 5, 1), "5 componentes, 1 descartado sem evidência"},
		{"xbom pt one", xbomSummaryText("pt-BR", 1, 0), "1 componente, 0 descartados sem evidência"},
		{"feedback", feedbackSummaryText(2, 0), "2 sinal(is), 0 novo(s)"},
		{"feedback one", feedbackSummaryText(1, 1), "1 sinal(is), 1 novo(s)"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}
