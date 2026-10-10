package i18n

import (
	"strings"
	"testing"
)

// aur607Prefixes are the key families the end-to-end Portuguese output
// added to the catalog.
var aur607Prefixes = []string{"terminal.", "gate.reason", "gate.status.", "gate.inconclusive_line", "status.review.", "scanner.", "mcp."}

// aur607Keys are keys the code reads by name; losing one would show the key
// itself to a reader.
var aur607Keys = []string{
	"terminal.verdict_cache_unavailable", "terminal.quality_skipped", "terminal.quality_required",
	"terminal.deterministic_only", "terminal.no_provider", "terminal.fail_on", "terminal.fail_on_one",
	"terminal.batches", "terminal.model_unparsed", "terminal.model_degraded", "terminal.no_issues",
	"terminal.no_findings_inconclusive", "terminal.notice_binary", "terminal.notice_generated",
	"terminal.formal_review", "terminal.parecer_posted", "terminal.formal_comments", "terminal.formal_comments_one",
	"terminal.formal_comments_none", "gate.inconclusive_line", "gate.reason_separator",
	"gate.reason_list_separator", "gate.reason.unknown", "gate.status.breach", "gate.status.breach_one",
	"gate.status.block", "gate.status.warn", "gate.status.approved", "scanner.secret_finding",
	"scanner.commit_detail", "mcp.next.inconclusive", "mcp.next.configure_provider", "mcp.next.install_scanner",
	"mcp.how_to_fix",
}

// The catalog test of the end-to-end Portuguese output: every new key
// exists, with text, in both languages.
func TestAUR607NewKeysExistInBothLocales(t *testing.T) {
	cat := catalog()
	for _, key := range aur607Keys {
		for _, l := range Locales {
			if strings.TrimSpace(cat[l][key]) == "" {
				t.Errorf("key %q has no text in %s", key, l)
			}
		}
	}
	for key := range cat[English] {
		for _, prefix := range aur607Prefixes {
			if strings.HasPrefix(key, prefix) && strings.TrimSpace(cat[Portuguese][key]) == "" {
				t.Errorf("key %q has no Portuguese text", key)
			}
		}
	}
}

// The singular forms never change English: each English "_one" text is its
// plural text with the first count written as 1, the bytes of earlier
// releases.
func TestAUR607EnglishSingularIsThePluralWithOne(t *testing.T) {
	cat := catalog()
	for key, one := range cat[English] {
		base, ok := strings.CutSuffix(key, "_one")
		if !ok || !strings.HasPrefix(key, "review.") && key != "terminal.fail_on_one" {
			continue
		}
		plural, found := cat[English][base]
		if !found {
			t.Errorf("%s has no plural %s", key, base)
			continue
		}
		if strings.Contains(plural, "(s)") && one != strings.Replace(plural, "%d", "1", 1) {
			t.Errorf("en %s = %q, want the plural with 1: %q", key, one, strings.Replace(plural, "%d", "1", 1))
		}
	}
}

// Portuguese counts without "(s)" in every text the review prints.
func TestAUR607PortugueseHasNoParenthesizedPlural(t *testing.T) {
	for key, text := range catalog()[Portuguese] {
		if strings.Contains(text, "(s)") || strings.Contains(text, "(ns)") {
			t.Errorf("pt-BR %s still counts with a parenthesized plural: %q", key, text)
		}
	}
}

// Lookup reports a missing key instead of echoing it.
func TestAUR607LookupReportsMissingKeys(t *testing.T) {
	if _, ok := Lookup("pt-BR", "no.such.key"); ok {
		t.Error("Lookup found a key the catalog does not have")
	}
	if text, ok := Lookup("pt-BR", "terminal.no_issues"); !ok || text != "Nenhum problema encontrado." {
		t.Errorf("Lookup(terminal.no_issues) = %q, %v", text, ok)
	}
}
