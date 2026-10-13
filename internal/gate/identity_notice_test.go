package gate

import "testing"

// The exceptions-disabled notice comes from the catalog in both languages,
// with the bytes of earlier releases (AUR-615).
func TestAUR615RepoIdentityUnavailableNoticeFollowsTheLanguage(t *testing.T) {
	for _, tc := range []struct{ language, want string }{
		{"", "Exceptions disabled: the reviewed repository's identity could not be confirmed from the \"origin\" remote."},
		{"en-US", "Exceptions disabled: the reviewed repository's identity could not be confirmed from the \"origin\" remote."},
		{"pt-BR", "Exceções desativadas: não foi possível confirmar o repositório revisado a partir do remoto \"origin\"."},
		{"pt", "Exceções desativadas: não foi possível confirmar o repositório revisado a partir do remoto \"origin\"."},
	} {
		if got := RepoIdentityUnavailableNotice(tc.language); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.language, got, tc.want)
		}
	}
}
