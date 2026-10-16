// The codebase context's own ceiling and the per-file-set memo (#198).
package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Mpaape/AurumCode/internal/config"
	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// charsPerEstimatedToken is the product's estimate (internal/prompt), the
// unit codebasectx.Fit measures in.
const charsPerEstimatedToken = 4

// capCodebaseContext reduces the pack to review.context.max_tokens when the
// repository set one; the reduction is declared in the pack itself, and
// noted once on stderr. Zero leaves the pack whole.
func capCodebaseContext(text string, maxTokens int, noted *sync.Once, stderr io.Writer) string {
	if maxTokens <= 0 || text == "" || len(text) <= maxTokens*charsPerEstimatedToken {
		return text
	}
	fitted, reduction := codebasectx.Fit(text, maxTokens*charsPerEstimatedToken)
	if noted != nil && stderr != nil {
		noted.Do(func() {
			if reduction.Discarded {
				fmt.Fprintf(stderr, "aurumcode review: codebase context left out: it does not fit review.context.max_tokens (%d)\n", maxTokens)
				return
			}
			fmt.Fprintf(stderr, "aurumcode review: codebase context reduced to review.context.max_tokens (%d): about %d of %d tokens kept\n",
				maxTokens, len(fitted)/charsPerEstimatedToken, len(text)/charsPerEstimatedToken)
		})
	}
	return fitted
}

// cappedCodebaseFor wraps a per-batch resolver: each file set is resolved
// once per review (a split review asks again for the same batches after a
// refusal) and capped like the whole-diff context.
func cappedCodebaseFor(cfg *config.Config, stderr io.Writer, resolve func(files []types.DiffFile) string) func(files []types.DiffFile) string {
	maxTokens := 0
	if cfg != nil {
		maxTokens = cfg.Review.Context.MaxTokens
	}
	var mu sync.Mutex
	memo := map[string]string{}
	noted := &sync.Once{}
	return func(files []types.DiffFile) string {
		key := fileSetKey(files)
		mu.Lock()
		text, ok := memo[key]
		mu.Unlock()
		if ok {
			return text
		}
		text = capCodebaseContext(resolve(files), maxTokens, noted, stderr)
		mu.Lock()
		memo[key] = text
		mu.Unlock()
		return text
	}
}

func fileSetKey(files []types.DiffFile) string {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	sort.Strings(paths)
	return strconv.Itoa(len(paths)) + "\x00" + strings.Join(paths, "\x00")
}

// modelWindowTokens is the declared context window: AURUMCODE_MODEL_WINDOW_TOKENS
// when set to a positive integer, else review.model_window_tokens, else zero.
func modelWindowTokens(cfg *config.Config, env reviewEnv) int {
	if n, err := strconv.Atoi(strings.TrimSpace(env.modelWindowTokens)); err == nil && n > 0 {
		return n
	}
	if cfg != nil && cfg.Review.ModelWindowTokens > 0 {
		return cfg.Review.ModelWindowTokens
	}
	return 0
}
