package review

import (
	"errors"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/prompt"
)

// A declared model window (review.model_window_tokens,
// AURUMCODE_MODEL_WINDOW_TOKENS) sizes the prompt from the first request
// (split.go). One too small for what goes with every part never fails the
// review: it is set aside with a notice, and the review goes on as if no
// window had been declared, learning the window from the model's refusal.

// SetNotifier installs where the reviewer writes its operator notices (in
// production, the command's stderr); nil keeps them silent.
func (r *Reviewer) SetNotifier(notify func(string)) {
	r.notify = notify
}

// declaredWindowTooSmall reports a review that ended with no room for a
// change under the ceiling of the declared window, before any refusal and
// with no explicit budget of the caller's below it: the declared window is
// the cause.
func (r *Reviewer) declaredWindowTooSmall(err error) bool {
	var noRoom *prompt.NoRoomError
	if err == nil || !errors.As(err, &noRoom) || len(r.refusals) > 0 {
		return false
	}
	ceiling := declaredCeiling(r.cfg.ModelWindowTokens)
	return ceiling > 0 && r.learnedBudget == ceiling && r.promptBudget() == ceiling && r.cfg.MaxTokens == 0
}

// dropDeclaredWindow sets the declared window aside for the rest of the
// review, says so, and resets what was sized under it.
func (r *Reviewer) dropDeclaredWindow(err error) {
	var noRoom *prompt.NoRoomError
	errors.As(err, &noRoom)
	r.droppedWindow = r.cfg.ModelWindowTokens
	r.learnedBudget = 0
	r.contextFits, r.contextReduction = nil, nil
	if r.notify == nil {
		return
	}
	r.notify(fmt.Sprintf("the declared model window (%d tokens, review.model_window_tokens or AURUMCODE_MODEL_WINDOW_TOKENS) is smaller than this review needs: what goes with every part takes about %d tokens of the %d it leaves for a prompt; reviewing as if no window were declared, learning it from the model's refusal",
		r.cfg.ModelWindowTokens, noRoom.FixedTokens, noRoom.MaxTokens))
}
