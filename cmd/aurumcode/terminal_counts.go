// Counted terminal lines outside the review's own copy (terminal_copy.go):
// the comments GitHub refused to publish, the dependencies, xbom and
// realimentacao summaries. Each count is a catalog text with a singular and
// a plural, so English keeps its earlier bytes and Portuguese never prints a
// parenthesized plural.
package main

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
)

// publishFailuresText counts the line comments GitHub refused.
func publishFailuresText(language string, failures int) string {
	return i18n.Counted(language, "terminal.publish_failures", failures)
}

// dependenciesSummaryText counts the manifests, packages and advisories of a
// dependencies run.
func dependenciesSummaryText(language string, manifests, packages, advisories int) string {
	return strings.Join([]string{
		i18n.Counted(language, "terminal.deps.manifests", manifests),
		i18n.Counted(language, "terminal.deps.packages", packages),
		i18n.Counted(language, "terminal.deps.advisories", advisories),
	}, ", ")
}

// xbomSummaryText counts the components an xBOM kept and the ones dropped
// for lack of evidence.
func xbomSummaryText(language string, components, dropped int) string {
	return i18n.Counted(language, "terminal.xbom.components", components) + ", " +
		i18n.Counted(language, "terminal.xbom.dropped", dropped)
}

// feedbackSummaryText counts the signals realimentacao read and the fresh
// ones. The command reads no repository configuration, so it prints the
// catalog's default language.
func feedbackSummaryText(signals, fresh int) string {
	return i18n.Counted("", "terminal.feedback.signals", signals) + ", " +
		i18n.Counted("", "terminal.feedback.fresh", fresh)
}
