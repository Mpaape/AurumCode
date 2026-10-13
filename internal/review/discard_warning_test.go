package review

import "testing"

// The rule gate's discard line keeps its earlier bytes in the default
// language and counts in Portuguese (AUR-615).
func TestAUR615RuleDiscardWarningFollowsTheLanguage(t *testing.T) {
	mixed := discardSummary{Missing: 1, Unknown: 1, UnknownIDs: []string{"security/no-such-rule"}}
	for _, tc := range []struct {
		language string
		d        discardSummary
		want     string
	}{
		{"", mixed, "2 finding(s) discarded: 1 with no rule_id, 1 citing an unknown rule_id (security/no-such-rule)"},
		{"", discardSummary{Missing: 1}, "1 finding(s) discarded: 1 with no rule_id"},
		{"pt-BR", mixed, "2 achados descartados: 1 sem rule_id, 1 citando rule_id desconhecido (security/no-such-rule)"},
		{"pt-BR", discardSummary{Missing: 1}, "1 achado descartado: 1 sem rule_id"},
		{"pt-BR", discardSummary{}, ""},
	} {
		if got := formatDiscardWarning(tc.language, tc.d); got != tc.want {
			t.Errorf("%q %+v: got %q, want %q", tc.language, tc.d, got, tc.want)
		}
	}
}
