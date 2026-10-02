#!/usr/bin/env bash
# AUR-477 E2E: a large pull request still receives a review (not zero) through
# the real reviewer path -- GenerateReview assembles the prompt with a capped
# token budget and must not refuse. It builds a tiny Go program against the
# fake provider and asserts the review is produced, closing the loop from
# diff -> prompt assembly -> reviewer that the unit and integration selectors
# only touch in parts.
set -Eeuo pipefail
export LC_ALL=C
card='AUR-477'
selector="${1:-E2EAUR477}"
[[ "$selector" == E2EAUR477 ]] || { printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64; }
repo_root="$(CDPATH='' cd -- "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
command -v go >/dev/null 2>&1 || { printf '%s/%s/infrastructure/missing-go\n' "$card" "$selector" >&2; exit 79; }

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a477-e2e.XXXXXX")"
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" HOME="$run_dir/home" GOMAXPROCS=1
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/home"

root="$run_dir/root"; mkdir -p "$root"
for top in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$top" ]] && cp -R "$repo_root/$top" "$root/$top"
done
chmod -R u+w -- "$root"

mkdir -p "$root/cmd/aur477_e2e"
cat >"$root/cmd/aur477_e2e/main.go" <<'EOF'
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const response = `{"issues": [], "summary": "no findings"}`

func main() {
	files := make([]types.DiffFile, 0, 200)
	for i := 0; i < 200; i++ {
		files = append(files, types.DiffFile{
			Path:  fmt.Sprintf("src/features/very/deeply/nested/module%d.go", i),
			Hunks: []types.DiffHunk{{Lines: []string{fmt.Sprintf("+var x%d = 1", i)}}},
		})
	}
	diff := &types.Diff{Files: files}

	// AUR-539: MaxTokens is derived from the review prompt builder's own
	// measured fixed overhead (instructions, rule catalog, schema) instead
	// of the literal 4000 this script hardcoded before, which that fixed
	// content outgrew. The cushion above the floor stays far below the
	// pre-AUR-477 worst case (a bullet per omitted file, ~200 files), so
	// this still proves the bounded coverage-declaration reservation: the
	// review succeeds despite a cushion much smaller than "name every file".
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	fixedOverhead, err := prompt.NewPromptBuilder().FixedOverheadTokens(diff, metrics, prompt.BuildOptions{SchemaKind: "review", Role: "reviewer"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "E2E: could not measure fixed overhead: %v\n", err)
		os.Exit(1)
	}
	const cushion = 1000
	maxTokens := fixedOverhead + cushion

	orch := llm.NewOrchestrator(&review.FakeProvider{Response: response}, nil, nil)
	reviewer := review.NewReviewer(orch, review.Config{MaxTokens: maxTokens})
	if _, err := reviewer.GenerateReview(context.Background(), diff); err != nil {
		fmt.Fprintf(os.Stderr, "E2E: large diff review refused (MaxTokens=%d, fixedOverhead=%d): %v\n", maxTokens, fixedOverhead, err)
		os.Exit(1)
	}
	fmt.Println("E2E: large diff produced a review")
}
EOF

(cd "$root" && go run ./cmd/aur477_e2e)
