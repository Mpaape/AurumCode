// Package i18n holds the interface texts the review publishes, per language,
// as an embedded YAML catalog. Every key exists in every locale with the same
// formatting verbs in the same order; a catalog that breaks that is refused
// when it loads, so a missing translation can never reach a published review.
package i18n

import (
	_ "embed"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Locale is a catalog language.
type Locale string

// The catalog's locales. English is the fallback for any other language.
const (
	Portuguese Locale = "pt-BR"
	English    Locale = "en"
)

// Locales lists every locale the catalog must cover.
var Locales = []Locale{Portuguese, English}

// portugueseTags are the review-language tags (config.NormalizeReviewLanguage
// output, compared case-insensitively) that select Portuguese.
var portugueseTags = map[string]bool{"pt-br": true, "pt": true}

//go:embed catalog.yml
var catalogYAML []byte

// Catalog maps a locale to its key -> text table.
type Catalog map[Locale]map[string]string

// builtin is the embedded catalog, parsed on first use. A catalog that
// fails its own check stops the process at the first text it would print,
// never shows a half-translated review.
var (
	builtinOnce sync.Once
	builtin     Catalog
)

func catalog() Catalog {
	builtinOnce.Do(func() { builtin = mustParse(catalogYAML) })
	return builtin
}

// verbPattern matches a fmt verb (a literal "%%" is not one).
var verbPattern = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)

// LocaleOf maps a review-language tag to its catalog locale.
func LocaleOf(language string) Locale {
	if portugueseTags[strings.ToLower(strings.TrimSpace(language))] {
		return Portuguese
	}
	return English
}

// Text returns the catalog text for key in language's locale. An unknown key
// is a programming error the catalog test catches; it returns the key itself
// so the gap is visible rather than silent.
func Text(language, key string) string {
	if v, ok := catalog()[LocaleOf(language)][key]; ok {
		return v
	}
	return key
}

// Format is fmt.Sprintf over Text.
func Format(language, key string, args ...any) string {
	return fmt.Sprintf(Text(language, key), args...)
}

// Keys returns the embedded catalog's keys, sorted.
func Keys() []string {
	return sortedKeys(catalog()[English])
}

// Parse decodes a catalog and refuses one where a locale is missing, a key
// exists in one locale only, or the formatting verbs of a key differ between
// locales.
func Parse(data []byte) (Catalog, error) {
	var raw map[string]map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("i18n: catalog: %w", err)
	}
	cat := Catalog{}
	for name, table := range raw {
		cat[Locale(name)] = table
	}
	if err := cat.validate(); err != nil {
		return nil, err
	}
	return cat, nil
}

func (c Catalog) validate() error {
	for _, l := range Locales {
		if len(c[l]) == 0 {
			return fmt.Errorf("i18n: catalog: locale %s is missing", l)
		}
	}
	for l := range c {
		if !knownLocale(l) {
			return fmt.Errorf("i18n: catalog: unknown locale %s", l)
		}
	}
	reference := c[English]
	for _, l := range Locales {
		for _, key := range sortedKeys(c[l]) {
			ref, ok := reference[key]
			if !ok {
				return fmt.Errorf("i18n: catalog: key %q exists in %s but not in %s", key, l, English)
			}
			if got, want := verbs(c[l][key]), verbs(ref); got != want {
				return fmt.Errorf("i18n: catalog: key %q has verbs %q in %s but %q in %s", key, got, l, want, English)
			}
		}
		for _, key := range sortedKeys(reference) {
			if _, ok := c[l][key]; !ok {
				return fmt.Errorf("i18n: catalog: key %q exists in %s but not in %s", key, English, l)
			}
		}
	}
	return nil
}

func knownLocale(l Locale) bool {
	for _, k := range Locales {
		if k == l {
			return true
		}
	}
	return false
}

func verbs(s string) string {
	return strings.Join(verbPattern.FindAllString(s, -1), " ")
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mustParse(data []byte) Catalog {
	cat, err := Parse(data)
	if err != nil {
		panic(err)
	}
	return cat
}
