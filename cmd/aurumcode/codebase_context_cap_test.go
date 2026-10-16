package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func bigPack(t *testing.T, refs int) string {
	t.Helper()
	pack := codebasectx.Pack{}
	for i := 0; i < refs; i++ {
		pack.References = append(pack.References, codebasectx.Reference{File: fmt.Sprintf("pkg/file%04d.py", i), Symbol: "handler", Line: i + 1})
	}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// #198: review.context.max_tokens caps the codebase context and says so.
func TestCapCodebaseContextReducesToTheCeiling(t *testing.T) {
	text := bigPack(t, 2000)
	var stderr strings.Builder
	capped := capCodebaseContext(text, 500, &sync.Once{}, &stderr)
	if len(capped) > 500*charsPerEstimatedToken {
		t.Fatalf("capped pack is %d bytes, over %d", len(capped), 500*charsPerEstimatedToken)
	}
	if !strings.Contains(capped, `"reduced"`) {
		t.Fatalf("the reduction must be declared in the pack: %.200s", capped)
	}
	if !strings.Contains(stderr.String(), "review.context.max_tokens (500)") {
		t.Fatalf("stderr does not name the ceiling: %q", stderr.String())
	}
	if got := capCodebaseContext(text, 0, nil, nil); got != text {
		t.Fatal("zero must leave the pack whole")
	}
}

// A split review asks for the same batches again after a refusal; each file
// set is resolved once.
func TestCappedCodebaseForResolvesEachFileSetOnce(t *testing.T) {
	calls := map[string]int{}
	for_ := cappedCodebaseFor(&config.Config{}, nil, func(files []types.DiffFile) string {
		calls[fileSetKey(files)]++
		return "{}"
	})
	a := []types.DiffFile{{Path: "a.py"}, {Path: "b.py"}}
	b := []types.DiffFile{{Path: "b.py"}, {Path: "a.py"}}
	for i := 0; i < 3; i++ {
		for_(a)
		for_(b)
	}
	if len(calls) != 1 || calls[fileSetKey(a)] != 1 {
		t.Fatalf("each file set must be resolved once, got %v", calls)
	}
}

func TestModelWindowTokensEnvOverridesConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Review.ModelWindowTokens = 128000
	if got := modelWindowTokens(cfg, reviewEnv{}); got != 128000 {
		t.Fatalf("config window: %d", got)
	}
	if got := modelWindowTokens(cfg, reviewEnv{modelWindowTokens: "262144"}); got != 262144 {
		t.Fatalf("env window: %d", got)
	}
	if got := modelWindowTokens(cfg, reviewEnv{modelWindowTokens: "x"}); got != 128000 {
		t.Fatalf("invalid env must fall back to config: %d", got)
	}
	if got := modelWindowTokens(nil, reviewEnv{}); got != 0 {
		t.Fatalf("nothing declared: %d", got)
	}
}
