package skills

import (
	"strings"
	"testing"
)

// The notice of a repository skill the policy overrides is written in the
// review's language through the catalog; English keeps its text.
func TestOverrideNoticeFollowsTheReviewLanguage(t *testing.T) {
	o := override{winner: "seguranca", winnerDir: "policy/seguranca", loser: "convencoes", loserDir: "repo/convencoes"}
	pt := o.notice("pt-BR")
	if !strings.HasPrefix(pt, `a skill da política "seguranca" (policy/seguranca) prevalece sobre a skill do repositório "convencoes"`) {
		t.Fatalf("pt-BR notice = %q", pt)
	}
	if en := o.notice(""); !strings.HasPrefix(en, `policy skill "seguranca" (policy/seguranca) overrides repository skill "convencoes"`) {
		t.Fatalf("English notice = %q", en)
	}
}
