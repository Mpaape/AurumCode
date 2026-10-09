package analysis

import "testing"

func TestLocalizeCatalogMessages(t *testing.T) {
	cases := []struct{ lang, in, want string }{
		{"pt-BR", msgHardcodedSecret + " (rule analysis/hardcoded-secret)", "Segredo ou credencial atribuído direto no código (rule analysis/hardcoded-secret)"},
		{"pt-BR", "Hardcoded secret matched rule github-pat (Uncovered a token)", "Segredo no código casou a regra github-pat (Uncovered a token)"},
		{"pt-BR", msgSQLInjection, "Consulta SQL montada por concatenação de texto"},
		{"en", msgHardcodedSecret, msgHardcodedSecret},
		{"", msgCommandInjection, msgCommandInjection},
		{"pt-BR", "mensagem livre do modelo", "mensagem livre do modelo"},
	}
	for _, c := range cases {
		if got := Localize(c.lang, c.in); got != c.want {
			t.Fatalf("Localize(%q, %q) = %q, want %q", c.lang, c.in, got, c.want)
		}
	}
}
