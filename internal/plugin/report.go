package plugin

import (
	"errors"
	"fmt"
	"io"

	"github.com/Mpaape/AurumCode/internal/i18n"
)

// Run is the whole `aurumcode plugin ingest`: it ingests and prints ONLY
// counts and stable reason identifiers. The workflow log of a public
// repository is public, so no title, path, description or text of the
// plugin's output ever reaches stdout or stderr. Exit: 0 SARIF written, 1
// inconclusive or refused (no SARIF), 2 usage.
func Run(o Options, language string, stdout, stderr io.Writer) int {
	counts, err := Ingest(o)
	var usage *UsageError
	var inc *InconclusiveError
	var path *PathError
	switch {
	case err == nil:
		fmt.Fprintln(stdout, SummaryText(language, o.Name, counts))
		return 0
	case errors.As(err, &usage):
		fmt.Fprintf(stderr, "aurumcode plugin ingest: %s\n", usage.Msg)
		return 2
	case errors.As(err, &inc):
		fmt.Fprintf(stderr, "aurumcode plugin ingest: %s\n", i18n.Format(language, "plugin.inconclusive", o.Name, inc.Reason))
		return 1
	case errors.As(err, &path):
		fmt.Fprintf(stderr, "aurumcode plugin ingest: %s\n", i18n.Format(language, "plugin.path_refused", o.Name, path.Index+1, path.Reason))
		return 1
	default:
		fmt.Fprintf(stderr, "aurumcode plugin ingest: %s\n", i18n.Format(language, "plugin.write_failed", o.Name))
		return 1
	}
}

// SummaryText is the one line a conclusive ingest prints: counts only.
func SummaryText(language, name string, c Counts) string {
	return i18n.Format(language, "plugin.summary", name,
		i18n.Counted(language, "plugin.alerts", c.Accepted),
		c.FalsePositive, c.OtherChannel, c.NoVerdict)
}
