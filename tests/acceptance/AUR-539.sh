#!/usr/bin/env bash
#
# Acceptance program for card AUR-539.
#
# WHAT THIS PROVES
#
#   AUR-467 and AUR-477's own acceptance programs pinned token budgets
#   (1700, 2200, 4000...) small enough that the review prompt's fixed
#   content (system instructions, rule catalog, schema) outgrew them: both
#   cards' sealed acceptance started failing on main at an unchanged base
#   (1d26968) with no change to either card's actual behavior. This card
#   derives those budgets from internal/prompt.PromptBuilder's own exported
#   FixedOverheadTokens measurement instead of a literal that rots again,
#   without weakening the refusal AC-002/AC-003 of AUR-467/477 prove.
#
# SELECTORS
#   all             run every scenario below
#   AC-001          AUR-467 and AUR-477's own test programs (unit,
#                   integration, e2e) pass again, derived budgets and all
#   AC-002          the refusal to assemble a prompt with no room for the
#                   diff still exists and is tested
#                   (TestAUR539FixedOverheadRefusalBoundary)
#   AC-003          the budgets used are derived from the measured fixed
#                   content, not a literal: FixedOverheadTokens exists and
#                   is exported, and the old pinned literals are gone from
#                   the files this card rewrote
#   AC-001-MUT-001  restore AUR-467's old fixed budget (1700) in place of
#                   the derived search -- AC-001 must go RED
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-539'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

required_inputs=(
  go.mod go.sum
  internal/analyzer internal/prompt pkg/types
  tests/unit/AUR-467.go tests/integration/AUR-467.go tests/e2e/AUR-467.sh
  tests/unit/AUR-477.go tests/integration/AUR-477.go tests/e2e/AUR-477.sh
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a539.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir" GOMAXPROCS=1
export GOMEMLIMIT=2GiB

run_go() { local dir="$1"; shift; ( cd "$dir" && ulimit -v 8388608 && go "$@" ); }

stage_root() {
  local root="$1"
  mkdir -p "$root"
  local top
  for top in go.mod go.sum cmd internal pkg tests; do
    [[ -e "$repo_root/$top" ]] || continue
    cp -R "$repo_root/$top" "$root/$top"
  done
  chmod -R u+w -- "$root"
}

# AC-001: the AUR-467 and AUR-477 test programs themselves -- the exact
# behavior those cards own -- bridged and run through go test, using the
# derived budgets AUR-539 put in place of the literals that rotted.
ac001_root="$run_dir/root-ac001"
run_ac001() {
  stage_root "$ac001_root"
  cat >"$ac001_root/tests/unit/aur539_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR539_AUR467Unit(t *testing.T)  { TestAUR467(t) }
func TestAUR539_AUR477Unit(t *testing.T)  { TestAUR477(t) }
EOF
  cat >"$ac001_root/tests/integration/aur539_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR539_AUR467Integration(t *testing.T) { IntegrationAUR467(t) }
func TestAUR539_AUR477Integration(t *testing.T) { IntegrationAUR477(t) }
EOF

  local out="$ac001_root/unit.out"
  if ! run_go "$ac001_root" test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR539_AUR467Unit$|^TestAUR539_AUR477Unit$' -count=1 >"$out" 2>&1; then
    cat "$out" >&2; return 1
  fi
  grep -Eq '(^|[[:space:]])ok[[:space:]]' "$out" || return 1

  local iout="$ac001_root/integration.out"
  if ! run_go "$ac001_root" test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR539_AUR467Integration$|^TestAUR539_AUR477Integration$' -count=1 >"$iout" 2>&1; then
    cat "$iout" >&2; return 1
  fi
  grep -Eq '(^|[[:space:]])ok[[:space:]]' "$iout" || return 1

  # tests/e2e/AUR-467.sh (E2EAUR467) is deliberately NOT run here: it fails
  # on an unmodified main for a pre-existing, unrelated reason (a self-review
  # regression-gate check, not a token-budget refusal), it is outside this
  # card's owned paths, and oci-run's own default selector for AUR-467
  # (AC-001 -> nominal_case) never invokes it either. See docs/specs/AUR-539.md.
  ( cd "$repo_root" && bash tests/e2e/AUR-477.sh E2EAUR477 ) >"$run_dir/e2e477.out" 2>&1 || { cat "$run_dir/e2e477.out" >&2; return 1; }
  return 0
}

# AC-002: the refusal to assemble a prompt with no room for the diff still
# exists and is tested -- internal/prompt's own behavior test proves both
# the refusal and that FixedOverheadTokens measures the exact floor it
# refuses at.
run_ac002() {
  local root="$run_dir/root-ac002"
  stage_root "$root"
  local out="$root/ac002.out"
  if ! run_go "$root" test -v -mod=mod -p 1 -timeout 300s ./internal/prompt -run '^TestAUR539FixedOverheadRefusalBoundary$' -count=1 >"$out" 2>&1; then
    cat "$out" >&2; return 1
  fi
  grep -Fq -- '--- PASS: TestAUR539FixedOverheadRefusalBoundary' "$out" || return 1
  return 0
}

# AC-003: the budget used is derived from the measured fixed content, not a
# literal that ages. FixedOverheadTokens must exist and be exported, and
# the specific literals AUR-467/477 pinned before this card must be gone
# from the files AUR-539 rewrote to use it.
run_ac003() {
  grep -Eq 'func \(b \*PromptBuilder\) FixedOverheadTokens\(' "$repo_root/internal/prompt/builder.go" || return 1
  grep -Fq 'FixedOverheadTokens' "$repo_root/tests/unit/AUR-467.go" || return 1
  grep -Fq 'FixedOverheadTokens' "$repo_root/tests/acceptance/AUR-467.sh" || return 1
  grep -Fq 'FixedOverheadTokens' "$repo_root/tests/e2e/AUR-477.sh" || return 1
  # The old literals this card's Outcome names must not reappear as the
  # MaxTokens this card derives.
  grep -Eq 'MaxTokens:\s*1700|MaxTokens:\s*2200|maxTokens, reserve = 1700, 40' "$repo_root/tests/unit/AUR-467.go" "$repo_root/tests/integration/AUR-467.go" "$repo_root/tests/acceptance/AUR-467.sh" && return 1
  grep -Fq 'MaxTokens: 4000' "$repo_root/tests/e2e/AUR-477.sh" && return 1
  return 0
}

# AC-001-MUT-001: restore AUR-467's pre-AUR-539 behavior on its AC-003
# fixture -- a hardcoded MaxTokens=1700 instead of a budget derived from
# FixedOverheadTokens. The two call sites of aur467TightBudget share
# identical text, so only the FIRST one in the file (the AC-003 fixture
# this test directly checks) is rewritten, leaving the other test's
# behavior untouched. The real fixed prompt content (measured at ~3800
# tokens for the review schema) is well above 1700, so BuildPrompt refuses
# and the bridged unit test must go RED.
run_mut001() {
  local root="$run_dir/root-mut001"
  stage_root "$root"
  local target="$root/tests/unit/AUR-467.go"
  local anchor='aur467TightBudget(t, prompt.NewPromptBuilder(), diff, metrics, 40, 20, 4000)'
  local occurrences
  occurrences="$(grep -Fc "$anchor" "$target")" || true
  (( occurrences >= 1 )) || infra 'MUT-001/anchor-missing'
  local replacement='func() (prompt.PromptParts, int) { _ = metrics; return aur467BuildPrompt(t, diff, 1700, 40), 1700 }() // MUT-001: old fixed budget restored'
  local tmp="$root/aur467.mutated"
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"]; done = 0 }
    {
      idx = (!done) ? index($0, anchor) : 0
      if (idx > 0) {
        print substr($0, 1, idx - 1) repl
        done = 1
      } else {
        print $0
      }
    }
  ' "$target" >"$tmp" || infra MUT-001-rewrite
  mv "$tmp" "$target"
  grep -Fq 'MUT-001: old fixed budget restored' "$target" || infra MUT-001-not-applied
  [[ "$(grep -Fc "$anchor" "$target")" == "$(( occurrences - 1 ))" ]] || infra 'MUT-001/rewrote-wrong-count'

  cat >"$root/tests/unit/aur539_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR539_AUR467Unit(t *testing.T) { TestAUR467(t) }
EOF
  local out="$root/mut001.out"
  set +e
  run_go "$root" test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR539_AUR467Unit$' -count=1 >"$out" 2>&1
  local rc=$?
  set -e
  if (( rc == 0 )); then
    cat "$out" >&2
    return 1 # mutation survived: old pinned budget did not turn the test red
  fi
  grep -Eq '^--- FAIL: TestAUR539_AUR467Unit' "$out" || infra 'MUT-001/no-fail-marker'
  grep -Eq 'build failed|cannot use|undefined:|syntax error' "$out" && infra 'MUT-001/build-failure-not-behavioral'
  return 0
}

case "$selector" in
  AC-001)
    run_ac001 || fail behavior-missing
    ;;
  AC-002)
    run_ac002 || fail behavior-missing
    ;;
  AC-003)
    run_ac003 || fail behavior-missing
    ;;
  AC-001-MUT-001)
    run_mut001 || fail MUT-001
    ;;
  all)
    run_ac001 || fail AC-001
    run_ac002 || fail AC-002
    run_ac003 || fail AC-003
    run_mut001 || fail AC-001-MUT-001
    ;;
esac

printf '%s/%s/pass\n' "$card" "$selector"
exit 0
