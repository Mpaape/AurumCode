package grammar

import (
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// commentNodeMarker is the grammar-neutral vocabulary every tree-sitter
// grammar uses to name its comment nodes ("comment", "line_comment",
// "block_comment", ...). It is node vocabulary, not a language name.
const commentNodeMarker = "comment"

// IsComment reports whether line, parsed with the grammar named lang, is
// entirely a comment: the first non-blank byte starts a comment node and that
// node reaches the end of the line. ok is false when the runtime has no
// grammar named lang, so the caller can declare it instead of guessing. The
// same text outside a comment (code, a string) is not a comment.
func (Runtime) IsComment(lang, line string) (isComment bool, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			isComment, ok = false, false
		}
	}()
	entry := grammars.DetectLanguageByName(lang)
	if entry == nil || entry.Language == nil {
		return false, false
	}
	language := entry.Language()
	if language == nil {
		return false, false
	}
	src := []byte(line)
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false, true
	}
	tree, err := gotreesitter.NewParser(language).Parse(src)
	if err != nil || tree == nil {
		return false, true
	}
	start := strings.Index(line, trimmed)
	end := start + len(trimmed)
	return coversWithComment(tree.RootNode(), language, uint32(start), uint32(end)), true
}

// coversWithComment finds a comment node that starts at start and reaches end.
func coversWithComment(n *gotreesitter.Node, lang *gotreesitter.Language, start, end uint32) bool {
	if n == nil {
		return false
	}
	if strings.Contains(n.Type(lang), commentNodeMarker) && n.StartByte() == start && n.EndByte() >= end {
		return true
	}
	for i := 0; i < n.ChildCount(); i++ {
		c := n.Child(i)
		if c != nil && c.StartByte() <= start && c.EndByte() >= start && coversWithComment(c, lang, start, end) {
			return true
		}
	}
	return false
}
