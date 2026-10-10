// The aurumcode/review commit status's description, per language. English
// keeps the description of earlier releases byte for byte; Portuguese drops
// the pull request number the page already shows and counts without "(s)".
package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/i18n"
)

// reviewCheckDescription says why the review check failed, or that nothing
// grave was found.
func reviewCheckDescription(language string, prNumber, grave int, qualityIncomplete, providerFailed bool) string {
	if i18n.LocaleOf(language) == i18n.English {
		return legacyReviewCheckDescription(prNumber, grave, qualityIncomplete, providerFailed)
	}
	switch {
	case providerFailed:
		return i18n.Text(language, "status.review.provider_failed")
	case qualityIncomplete:
		return i18n.Text(language, "status.review.quality_incomplete")
	case grave > 0:
		return countText(grave, i18n.Text(language, "status.review.grave_one"), i18n.Text(language, "status.review.grave"))
	}
	return i18n.Text(language, "status.review.none")
}

// legacyReviewCheckDescription is the description of earlier releases.
func legacyReviewCheckDescription(prNumber, grave int, qualityIncomplete, providerFailed bool) string {
	switch {
	case providerFailed:
		return fmt.Sprintf("revisão não executada no pull request #%d: falha do provedor (provider_failure)", prNumber)
	case qualityIncomplete:
		return fmt.Sprintf("revisão por modelo inconclusiva no pull request #%d", prNumber)
	case grave > 0:
		return fmt.Sprintf("%d achado(s) grave(s) no pull request #%d", grave, prNumber)
	}
	return fmt.Sprintf("nenhum achado grave no pull request #%d", prNumber)
}
