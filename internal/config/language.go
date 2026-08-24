package config

import (
	"fmt"
	"sort"
	"strings"
)

const DefaultReviewLanguage = "en-US"

var reviewLanguages = map[string]string{
	"de-de": "Deutsch (de-DE)",
	"en":    "English (en)",
	"en-us": "English (en-US)",
	"es-es": "Español (es-ES)",
	"fr-fr": "Français (fr-FR)",
	"it-it": "Italiano (it-IT)",
	"ja-jp": "日本語 (ja-JP)",
	"pt":    "Português (pt)",
	"pt-br": "Português do Brasil (pt-BR)",
}

// ReviewLanguage returns the canonical configured language tag. It is a
// closed, presentation-only choice: arbitrary text must never become a new
// instruction inside the review prompt.
func (c *Config) ReviewLanguage() (string, error) {
	if c == nil {
		return DefaultReviewLanguage, nil
	}
	return NormalizeReviewLanguage(c.Review.Language)
}

// NormalizeReviewLanguage validates and canonicalizes the small supported
// language list. Empty means the stable product default.
func NormalizeReviewLanguage(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return DefaultReviewLanguage, nil
	}
	key := strings.ToLower(value)
	if _, ok := reviewLanguages[key]; !ok {
		allowed := make([]string, 0, len(reviewLanguages))
		for language := range reviewLanguages {
			allowed = append(allowed, language)
		}
		sort.Strings(allowed)
		return "", fmt.Errorf("review.language %q is unsupported; use one of %s", value, strings.Join(allowed, ", "))
	}
	switch key {
	case "en-us":
		return "en-US", nil
	case "es-es":
		return "es-ES", nil
	case "fr-fr":
		return "fr-FR", nil
	case "de-de":
		return "de-DE", nil
	case "it-it":
		return "it-IT", nil
	case "ja-jp":
		return "ja-JP", nil
	case "pt-br":
		return "pt-BR", nil
	default:
		return key, nil
	}
}

// ReviewLanguageLabel returns the human-readable label used in the prompt.
func ReviewLanguageLabel(language string) string {
	if label, ok := reviewLanguages[strings.ToLower(strings.TrimSpace(language))]; ok {
		return label
	}
	return language
}
