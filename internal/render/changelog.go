package render

import (
	"fmt"
	"strings"
)

// ChangelogSection renders the review's advisory release block from the
// AUR-498 engine output: the suggested next version, the bump name and the
// already-escaped changelog entry. version and bump are engine-generated from
// integers; entry is Markdown the engine already escaped, so no untrusted text
// reaches this function unescaped. language selects Portuguese labels for
// "pt"/"pt-BR"; anything else uses English.
func ChangelogSection(version, bump, entry, language string) string {
	heading := "### Suggested release"
	versionLabel := "Next version"
	bumpLabel := "Bump"
	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "pt" || lang == "pt-br" || strings.HasPrefix(lang, "pt-") {
		heading = "### Versão sugerida"
		versionLabel = "Próxima versão"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", heading)
	fmt.Fprintf(&b, "**%s:** `%s`\n\n", versionLabel, inlineSafe(version))
	fmt.Fprintf(&b, "**%s:** %s\n\n", bumpLabel, inlineSafe(bump))
	b.WriteString(strings.TrimRight(entry, "\n"))
	b.WriteString("\n")
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// inlineSafe strips characters that could break out of an inline code span.
// The engine's version and bump are numeric, so this is defense in depth.
func inlineSafe(s string) string {
	return strings.NewReplacer("`", "", "\n", " ", "\r", " ").Replace(strings.TrimSpace(s))
}
