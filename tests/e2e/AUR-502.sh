#!/usr/bin/env bash
#
# E2E program for card AUR-502, selector E2EAUR502.
#
# Builds a standalone Go harness over the real public API against a REAL
# .aurumcode/profiles.yml on a REAL temp filesystem -- no git, no LLM fixture
# -- and asserts the composition a caller like cmd/aurumcode performs: read
# the team file, resolve two profiles, merge their findings with attribution,
# and refuse a hostile team definition that tries to disable a boundary.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 = the promised property holds
#   1 = behavioral RED
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-502'
readonly scenario='E2E'
selector="${1:-E2EAUR502}"
case "$selector" in
  E2EAUR502) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

readonly read_inputs=(go.mod go.sum internal/reviewprofile)
for input in "${read_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e502.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

root="$run_dir/root"
for input in "${read_inputs[@]}"; do
  if [[ -d "$repo_root/$input" ]]; then
    mkdir -p "$root/$input"
    chmod -R u+w -- "$root/$input" >/dev/null 2>&1 || true
    cp -R "$repo_root/$input/." "$root/$input/"
  else
    mkdir -p "$root/$(dirname "$input")"
    cp "$repo_root/$input" "$root/$input"
  fi
done
chmod -R u+w -- "$root"

repo="$run_dir/repo"
mkdir -p "$repo/.aurumcode"
cat >"$repo/.aurumcode/profiles.yml" <<'YAML'
profiles:
  - name: release
    emphasis: upstream compatibility
    families: [quality]
    instructions: Confira o contrato e o custo de rollout.
YAML
mkdir -p "$root/cmd/aur502e2e"
cat >"$root/cmd/aur502e2e/main.go" <<'EOF'
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

func main() {
	root := os.Args[1]
	team, err := reviewprofile.LoadTeamFile(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load-team:", err)
		os.Exit(2)
	}
	if len(team.Names()) != 1 || team.Names()[0] != "release" {
		fmt.Fprintln(os.Stderr, "team-not-read")
		os.Exit(3)
	}
	res, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"solid", "release"}}, team)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve:", err)
		os.Exit(4)
	}
	if !res.Applied || !strings.Contains(res.Declared, "release") || !strings.Contains(res.Declared, "solid") {
		fmt.Fprintln(os.Stderr, "multi-not-declared")
		os.Exit(5)
	}
	if !res.BoundariesFixed() {
		fmt.Fprintln(os.Stderr, "boundary-moved")
		os.Exit(6)
	}
	merged := reviewprofile.MergeFindings([]reviewprofile.Finding{
		{Profile: "solid", RuleID: "q", File: "a.go", Line: 1, Message: "m"},
		{Profile: "release", RuleID: "q", File: "a.go", Line: 1, Message: "m"},
	})
	if len(merged) != 1 || merged[0].Profile != "solid" {
		fmt.Fprintln(os.Stderr, "merge-not-attributed")
		os.Exit(7)
	}
	if _, err := reviewprofile.LoadTeam([]byte("profiles:\n  - name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n    security_pass: false\n")); !errors.Is(err, reviewprofile.ErrRefusedClause) {
		fmt.Fprintln(os.Stderr, "hostile-team-accepted")
		os.Exit(8)
	}
	fmt.Println("e2e-ok")
}
EOF

bin="$run_dir/aur502e2e"
if ! (cd "$root" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go build -o "$bin" ./cmd/aur502e2e) >"$run_dir/build.log" 2>&1; then
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
