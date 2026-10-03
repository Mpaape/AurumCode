package gate

import "testing"

func TestFindingLineSpellsTheRuleCitationWithoutAColon(t *testing.T) {
	got := FindingLine("security/hardcoded-secret", "Hardcoded credentials (rule security/hardcoded-secret: Hardcoded Secrets)", "error", "error", OriginSecurity)
	want := "security/hardcoded-secret - Hardcoded credentials (rule security/hardcoded-secret - Hardcoded Secrets) (severidade error, limiar error, origem security)"
	if got != want {
		t.Fatalf("got %q", got)
	}
	if plain := FindingLine("a/b", "msg (rule a/b)", "error", "error", OriginAnalysis); plain != "a/b - msg (rule a/b) (severidade error, limiar error, origem analysis)" {
		t.Fatalf("plain: %q", plain)
	}
}
