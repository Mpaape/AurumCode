package i18n

import (
	"strings"
	"testing"
)

// TestEveryKeyExistsInEveryLocale is AC-002: the embedded catalog loads, and
// every key has a text in pt-BR and en.
func TestEveryKeyExistsInEveryLocale(t *testing.T) {
	cat, err := Parse(catalogYAML)
	if err != nil {
		t.Fatal(err)
	}
	keys := Keys()
	if len(keys) < 40 {
		t.Fatalf("catalog has %d keys; the embedded file is no longer read", len(keys))
	}
	for _, l := range Locales {
		for _, k := range keys {
			if strings.TrimSpace(cat[l][k]) == "" {
				t.Errorf("key %q has no text in %s", k, l)
			}
		}
	}
}

// TestCatalogMissingKeyIsRefused proves the check bites: dropping one key from
// one locale of the real catalog is refused, naming the key.
func TestCatalogMissingKeyIsRefused(t *testing.T) {
	text := string(catalogYAML)
	line := "  review.verdict: \"Veredito\"\n"
	if !strings.Contains(text, line) {
		t.Fatal("anchor line not in the catalog")
	}
	_, err := Parse([]byte(strings.Replace(text, line, "", 1)))
	if err == nil || !strings.Contains(err.Error(), `"review.verdict"`) {
		t.Fatalf("a catalog missing review.verdict in pt-BR was accepted: %v", err)
	}
}

func TestCatalogVerbMismatchIsRefused(t *testing.T) {
	bad := "pt-BR:\n  a: \"%d de %s\"\nen:\n  a: \"%s of %d\"\n"
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("a key whose verbs differ between locales was accepted")
	}
}

func TestLocaleOf(t *testing.T) {
	for in, want := range map[string]Locale{"pt-BR": Portuguese, "pt": Portuguese, " PT-br ": Portuguese, "pt-PT": Portuguese, "en-US": English, "ptx": English, "es-ES": English, "": English} {
		if got := LocaleOf(in); got != want {
			t.Errorf("LocaleOf(%q) = %s, want %s", in, got, want)
		}
	}
}
