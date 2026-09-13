package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Summary renders a deterministic, concise Markdown summary of a review,
// suitable for prepending to a pull-request comment. It always reports the
// verdict, the counts of findings by severity, and the sorted list of files
// touched. The language hint selects Portuguese ("pt-BR"/"pt") labels;
// anything else falls back to English.
func Summary(result *types.ReviewResult, language string) string {
	if result == nil {
		result = &types.ReviewResult{}
	}
	l := labelsFor(language)

	counts := severityCounts(result)
	files := filesTouched(result)

	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", l.heading)
	fmt.Fprintf(&b, "**%s:** %s\n\n", l.verdictLabel, verdictWord(result.Verdict, l))
	fmt.Fprintf(&b, "**%s:**\n", l.findings)
	fmt.Fprintf(&b, "- %s: %d\n", l.severityError, counts.error)
	fmt.Fprintf(&b, "- %s: %d\n", l.severityWarning, counts.warning)
	fmt.Fprintf(&b, "- %s: %d\n", l.severityInfo, counts.info)
	fmt.Fprintf(&b, "\n**%s:**\n", l.filesTouched)
	if len(files) == 0 {
		fmt.Fprintf(&b, "- %s\n", l.none)
	} else {
		for _, f := range files {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	return b.String()
}

type severityCount struct {
	error   int
	warning int
	info    int
}

func severityCounts(result *types.ReviewResult) severityCount {
	var c severityCount
	for _, issue := range result.Issues {
		switch issue.Severity {
		case "error":
			c.error++
		case "warning":
			c.warning++
		case "info":
			c.info++
		}
	}
	return c
}

// filesTouched collects the unique, non-empty file paths referenced by
// issues, suggestions, and legacy comments, then returns them sorted.
func filesTouched(result *types.ReviewResult) []string {
	seen := make(map[string]struct{})
	add := func(p string) {
		if p = strings.TrimSpace(p); p != "" {
			seen[p] = struct{}{}
		}
	}
	for _, issue := range result.Issues {
		add(issue.File)
	}
	for _, s := range result.Suggestions {
		add(s.File)
	}
	for _, c := range result.LineComments {
		add(c.Path)
	}
	for _, c := range result.FileComments {
		add(c.Path)
	}

	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

type summaryLabels struct {
	heading         string
	verdictLabel    string
	findings        string
	filesTouched    string
	none            string
	unknown         string
	severityError   string
	severityWarning string
	severityInfo    string
	approve         string
	changes         string
	comment         string
}

func labelsFor(language string) summaryLabels {
	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "pt" || lang == "pt-br" || strings.HasPrefix(lang, "pt-") {
		return summaryLabels{
			heading:         "Resumo da Revisão de Código",
			verdictLabel:    "Veredito",
			findings:        "Resultados por severidade",
			filesTouched:    "Arquivos alterados",
			none:            "Nenhum",
			unknown:         "Desconhecido",
			severityError:   "erro",
			severityWarning: "aviso",
			severityInfo:    "informação",
			approve:         "Aprovar",
			changes:         "Alterações solicitadas",
			comment:         "Comentário",
		}
	}
	return summaryLabels{
		heading:         "Code Review Summary",
		verdictLabel:    "Verdict",
		findings:        "Findings by severity",
		filesTouched:    "Files touched",
		none:            "None",
		unknown:         "Unknown",
		severityError:   "error",
		severityWarning: "warning",
		severityInfo:    "info",
		approve:         "Approve",
		changes:         "Changes requested",
		comment:         "Comment",
	}
}

func verdictWord(verdict string, l summaryLabels) string {
	switch verdict {
	case "approve":
		return l.approve
	case "changes_requested":
		return l.changes
	case "comment":
		return l.comment
	case "":
		return l.unknown
	default:
		return verdict
	}
}
