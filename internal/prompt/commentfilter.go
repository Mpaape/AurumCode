package prompt

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// maxNoticeFiles bounds how many file paths the notice names.
const maxNoticeFiles = 5

// isCommentLine asks the grammar provider whether line is a comment in
// language. It carries no language knowledge: a language the provider has no
// grammar for (including the NoStructure marker) is never a comment, i.e. no
// comment filter is applied, and CommentFilterNotice declares that to the
// model. There is no default comment syntax.
func isCommentLine(provider grammar.Provider, line, language string) bool {
	if language == "" || language == grammar.NoStructure {
		return false
	}
	isComment, ok := provider.IsComment(language, line)
	return ok && isComment
}

// CommentFilterNotice returns the paragraph, empty or starting with a blank
// line, that tells the model which changed code files had no comment filter
// because the grammar provider has no grammar for them.
func CommentFilterNotice(diff *types.Diff, provider grammar.Provider) string {
	if diff == nil {
		return ""
	}
	detector := analyzer.NewLanguageDetectorWith(provider)
	var names []string
	for _, file := range diff.Files {
		language, prose := classifyFile(file.Path, detector)
		if prose || detector.IsConfigFile(file.Path) || isNonCodeName(file.Path) {
			continue
		}
		if language == grammar.NoStructure {
			names = append(names, file.Path)
		}
	}
	if len(names) == 0 {
		return ""
	}
	shown := names
	if len(shown) > maxNoticeFiles {
		shown = shown[:maxNoticeFiles]
	}
	more := ""
	if len(names) > len(shown) {
		more = fmt.Sprintf(" and %d more", len(names)-len(shown))
	}
	return fmt.Sprintf("\n\nNotice: no grammar is available for %s%s, so no comment filter was applied to them: comment lines in those files count as changes and were not removed from the diff.",
		strings.Join(shown, ", "), more)
}
