package i18n

import (
	"strings"
	"testing"
)

// AUR-616 AC-003: the deliberation terminal lines that name the real
// reason exist in both locales, with the provider or analyst as a verb.
func TestAUR616DeliberationKeysExistInBothLocales(t *testing.T) {
	for _, key := range []string{
		"terminal.deliberation_no_tools", "terminal.deliberation_off", "terminal.deliberation_rounds_spent",
		"terminal.deliberation_analyst", "terminal.deliberation_analyst_call",
		"terminal.deliberation_no_tool_round", "terminal.deliberation_analyst_no_tools", "terminal.deliberation_none_offered",
	} {
		for _, language := range []string{"pt-BR", "en"} {
			text, ok := Lookup(language, key)
			if !ok || !strings.HasPrefix(text, "deliberation: ") {
				t.Errorf("%s/%s = %q, %v; want a deliberation line", language, key, text, ok)
			}
		}
	}
	if !strings.Contains(Text("pt-BR", "terminal.deliberation_no_tools"), "%s") || !strings.Contains(Text("pt-BR", "terminal.deliberation_rounds_spent"), "%s") {
		t.Error("the provider and the analyst must be named in the line")
	}
}
