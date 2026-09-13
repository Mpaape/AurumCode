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

	"github.com/Mpaape/AurumCode/internal/llm"
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
	orch := llm.NewOrchestrator(&review.FakeProvider{Response: response}, nil, nil)
	reviewer := review.NewReviewer(orch, review.Config{MaxTokens: 4000})
	if _, err := reviewer.GenerateReview(context.Background(), &types.Diff{Files: files}); err != nil {
		fmt.Fprintf(os.Stderr, "E2E: large diff review refused: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("E2E: large diff produced a review")
}
EOF

(cd "$root" && go run ./cmd/aur477_e2e)
