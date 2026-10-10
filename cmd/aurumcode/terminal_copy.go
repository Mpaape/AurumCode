// Terminal lines of the review command in the review's language: each one
// is a catalog text, so English keeps its earlier bytes and Portuguese never
// falls back to a fixed English sentence.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
)

// printLines prints text one line per line, each behind prefix, so a long
// catalog text reaches the terminal as short lines.
func printLines(w io.Writer, prefix, text string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(w, "%s%s\n", prefix, line)
	}
}

// noProviderText is the teaching text of a run without a model provider:
// in English, errNoProviderConfigured's own text; in Portuguese, short lines.
func noProviderText(language string) string {
	return i18n.Text(language, "terminal.no_provider")
}

// failOnLine is the closing line of a run --fail-on failed: how many
// findings sit at the threshold or above.
func failOnLine(language string, n int, threshold string) string {
	if n == 1 {
		return i18n.Format(language, "terminal.fail_on_one", threshold, threshold)
	}
	return i18n.Format(language, "terminal.fail_on", n, threshold, threshold)
}

// cacheReusedLine counts the files the review cache answered.
func cacheReusedLine(language string, files int) string {
	if files == 1 {
		return i18n.Text(language, "terminal.cache_reused_one")
	}
	return i18n.Format(language, "terminal.cache_reused", files)
}

// formalReviewLine is the stdout line of a formal review published on the
// pull request. English keeps the earlier line byte for byte (it named the
// API event and counted with "(s)"); Portuguese names the decision and
// counts the line comments.
func formalReviewLine(language, event string, prNumber, comments int) string {
	if i18n.LocaleOf(language) == i18n.English {
		return fmt.Sprintf("review formal %q publicado no pull request #%d (%d comentário(s) na linha).", event, prNumber, comments)
	}
	decision, ok := i18n.Lookup(language, "terminal.formal_event."+event)
	if !ok {
		decision = event
	}
	return i18n.Format(language, "terminal.formal_review", prNumber, decision, lineCommentsText(language, comments))
}

// parecerLine is the stdout line of the parecer published (or updated) as a
// comment; how is the publication's own word. English keeps the earlier
// line byte for byte.
func parecerLine(language, how string, prNumber, comments int) string {
	if i18n.LocaleOf(language) == i18n.English {
		return fmt.Sprintf("parecer %s no pull request #%d (%d comentário(s) na linha).", how, prNumber, comments)
	}
	return i18n.Format(language, "terminal.parecer_posted", how, prNumber, lineCommentsText(language, comments))
}

// lineCommentsText counts the comments published on the changed lines.
func lineCommentsText(language string, comments int) string {
	switch comments {
	case 0:
		return i18n.Text(language, "terminal.formal_comments_none")
	case 1:
		return i18n.Text(language, "terminal.formal_comments_one")
	}
	return i18n.Format(language, "terminal.formal_comments", comments)
}
