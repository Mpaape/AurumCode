#!/usr/bin/env bash
#
# E2E program for card AUR-479, selector E2EAUR479.
#
# WHAT THIS PROVES, AND WHY IT IS NOT THE ACCEPTANCE AGAIN
#
#   The acceptance program already exercises the composed package API. This
#   program builds a small standalone binary against the staged package and
#   runs it over REAL files on a REAL temp filesystem, pinning the exact
#   end-to-end composition a caller performs: two configured providers read
#   a secret split across two on-disk files, WrapProvider assembles the
#   block through the real redaction filter, and the prompt the base
#   llm.Provider receives must not contain the reconstituted secret.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-479'
readonly scenario='E2E'
selector="${1:-E2EAUR479}"
case "$selector" in
  E2EAUR479) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum internal/config pkg/types internal/llm internal/security/redaction; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e479.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

root="$run_dir/root"
cp "$repo_root/go.mod" "$repo_root/go.sum" "$root/"
mkdir -p "$root/internal/config" "$root/pkg/types" "$root/internal/llm" "$root/internal/security/redaction"
cp -R "$repo_root/internal/config/." "$root/internal/config/"
cp -R "$repo_root/pkg/types/." "$root/pkg/types/"
cp -R "$repo_root/internal/llm/." "$root/internal/llm/"
cp -R "$repo_root/internal/security/redaction/." "$root/internal/security/redaction/"
chmod -R u+w -- "$root"

repo="$run_dir/repo"
mkdir -p "$repo/.aurumcode" "$repo/docs"
printf '%s' 'AURUM-e2e-split' >"$repo/.aurumcode/prompt.md"
printf '%s' '-canary-77aa11' >"$repo/docs/context.md"

mkdir -p "$root/cmd/aur479e2e"
cat >"$root/cmd/aur479e2e/main.go" <<'EOF'
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const secret = "AURUM-e2e-split-canary-77aa11"

type capture struct{ got string }

func (c *capture) Complete(prompt string, _ llm.Options) (llm.Response, error) {
	c.got = prompt
	return llm.Response{Text: `{"issues":[]}`}, nil
}
func (c *capture) Tokens(s string) (int, error) { return len(s), nil }
func (c *capture) Name() string                 { return "e2e-fake" }

func main() {
	root := os.Args[1]
	cfg := &config.Config{Review: config.ReviewConfig{Context: config.ReviewContextConfig{
		Docs: []string{"docs/context.md"},
	}}}
	base := &capture{}
	wrapped, err := config.WrapProvider(context.Background(), base,
		config.ConfiguredProviders(root, cfg), []string{"changed.go"}, redaction.NewFilter(secret))
	if err != nil {
		fmt.Fprintln(os.Stderr, "wrap:", err)
		os.Exit(2)
	}
	if _, err := wrapped.Complete("BASE", llm.Options{}); err != nil {
		fmt.Fprintln(os.Stderr, "complete:", err)
		os.Exit(2)
	}
	if strings.Contains(strings.NewReplacer("\n", "", "\r", "").Replace(base.got), secret) {
		fmt.Fprintln(os.Stderr, "split-secret-reconstituted")
		os.Exit(3)
	}
	if !strings.Contains(base.got, redaction.Marker) {
		fmt.Fprintln(os.Stderr, "redaction-marker-missing")
		os.Exit(4)
	}
	fmt.Println("e2e-ok")
}
EOF

bin="$run_dir/aur479e2e"
if ! (cd "$root" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go build -o "$bin" ./cmd/aur479e2e) >"$run_dir/build.log" 2>&1; then
  cat "$run_dir/build.log" >&2
  infra build_failed
fi

set +e
out="$(ulimit -v 8388608; GOMEMLIMIT=2GiB "$bin" "$repo" 2>"$run_dir/run.err")"
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
  cat "$run_dir/run.err" >&2
  fail "harness-exit:$rc"
fi
[[ "$out" == "e2e-ok" ]] || fail "unexpected-output:$out"

exit 0
