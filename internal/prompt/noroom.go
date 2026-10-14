package prompt

import "fmt"

// NoRoomError is the refusal to assemble a prompt whose fixed content
// (instructions, rule catalog, context) already fills the budget, leaving
// no room for a single code change. With the default of no ceiling it only
// happens once the reviewer has lowered the budget to what a provider
// accepts (internal/review/split.go), or when a caller sets one.
type NoRoomError struct {
	FixedTokens  int
	ReserveReply int
	MaxTokens    int
	SchemaKind   string
}

func (e *NoRoomError) Error() string {
	return fmt.Sprintf(
		"prompt instructions and fixed content need %d tokens and the reply reserves %d, which leaves no room for a single code change in the %d-token budget: refusing to assemble a %s prompt with no diff in it",
		e.FixedTokens, e.ReserveReply, e.MaxTokens, e.SchemaKind)
}
