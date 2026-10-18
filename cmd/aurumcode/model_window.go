// The declared model window (#198): resolved once per run, its source said
// when the environment overrides the config, and the notice of one set
// aside as too small written once.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review"
)

// modelWindow is the run's declared window and the once-only notices about
// it.
type modelWindow struct {
	resolve sync.Once
	tokens  int
	notice  sync.Once
}

// modelWindowTokens is the declared context window: AURUMCODE_MODEL_WINDOW_TOKENS
// when set to a positive integer, else review.model_window_tokens, else zero.
func modelWindowTokens(cfg *config.Config, env reviewEnv) int {
	if n, ok := envWindowTokens(env); ok {
		return n
	}
	if cfg != nil && cfg.Review.ModelWindowTokens > 0 {
		return cfg.Review.ModelWindowTokens
	}
	return 0
}

// envWindowTokens is AURUMCODE_MODEL_WINDOW_TOKENS when it is a positive
// integer.
func envWindowTokens(env reviewEnv) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(env.modelWindowTokens))
	return n, err == nil && n > 0
}

// modelWindowTokens is this run's declared window, resolved on first use,
// which is sequential (the model phase, or the building of the analyst
// passes). An environment value that overrides a different
// review.model_window_tokens is said once on stderr: the config.yml the team
// reads is not what runs.
func (s *reviewState) modelWindowTokens() int {
	if s.window == nil {
		s.window = &modelWindow{}
	}
	s.window.resolve.Do(func() {
		s.window.tokens = modelWindowTokens(s.cfg, s.env())
		noteWindowOverride(s.stderr, s.cfg, s.env())
	})
	return s.window.tokens
}

// noteWindowOverride writes the override notice when the environment's
// window replaces a different one of the config.
func noteWindowOverride(stderr io.Writer, cfg *config.Config, env reviewEnv) {
	n, ok := envWindowTokens(env)
	if !ok || cfg == nil || cfg.Review.ModelWindowTokens <= 0 || cfg.Review.ModelWindowTokens == n || stderr == nil {
		return
	}
	fmt.Fprintf(stderr, "aurumcode review: AURUMCODE_MODEL_WINDOW_TOKENS (%d) overrides review.model_window_tokens (%d) of the config\n", n, cfg.Review.ModelWindowTokens)
}

// declareModelWindow gives reviewer this run's window and the stderr notice
// of a window set aside as too small, written once whatever the number of
// passes.
func (s *reviewState) declareModelWindow(reviewer *review.Reviewer) {
	reviewer.SetModelWindow(s.modelWindowTokens())
	window := s.window
	reviewer.SetNotifier(func(msg string) {
		window.notice.Do(func() {
			fmt.Fprintf(s.stderr, "aurumcode review: %s\n", msg)
		})
	})
}
