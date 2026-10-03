// Package tokens is the engine's one token heuristic, for every budget
// taken where no provider counts tokens itself.
package tokens

// charsPerToken is the one character-per-token ratio every budget in the
// engine uses when no provider counts tokens itself: unrelated to any
// vendor, close enough for budgeting, the same everywhere so two budgets
// over the same text agree.
const charsPerToken = 4

// Estimate approximates text's token count at charsPerToken
// characters per token, never zero for non-empty text, so a non-empty
// segment can never be budgeted as free.
func Estimate(text string) int {
	if text == "" {
		return 0
	}
	if n := len(text) / charsPerToken; n > 0 {
		return n
	}
	return 1
}
