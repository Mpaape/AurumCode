#!/usr/bin/env bash
#
# Acceptance program for card AUR-504, scenarios AC-001/AC-002/AC-003 plus the
# two skeptical mutations MUT-001/MUT-002.
#
# WHAT THIS PROVES
#
#   `aurumcode review --pr --check` must publish the `aurumcode/review` commit
#   status on the pull request HEAD returned by the API. Before this card it
#   published on os.Getenv("GITHUB_SHA"), which GitHub Actions reserves to the
#   synthetic merge commit on a pull_request event; a branch protection rule
#   requiring the context then never saw the status on the head and left the PR
#   "expected".
#
#   AC-001: with --pr --check and GITHUB_SHA pointing at the merge commit, the
#           status is published on /statuses/{API-head}.
#   AC-002: the local --base path publishes nothing and never contacts GitHub,
#           keeping its pre-existing output.
#   AC-003: when the head cannot be determined (API error, or a response with
#           no head SHA), the command fails closed naming the reason and
#           publishes no status -- never a fallback to GITHUB_SHA.
#
#   MUT-001 (use GITHUB_SHA/the merge commit for the status again) must make
#   AC-001 RED; MUT-002 (suppress the fail-closed when the head is missing)
#   must make AC-003 RED. Restoring the source reproduces the exact GREEN.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001/MUT-002 mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-504'
readonly scenario='AC-001'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(
  cmd/aurumcode
  internal/git/githubclient
  tests/unit/AUR-504.go
  docs/specs/AUR-504.md
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod
  go.sum
  tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a504.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"
export GOMAXPROCS=1

unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes a build root for cmd/aurumcode plus the fixture
# inputs AC-002 needs, and copies this card's unit program under tests/unit.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum cmd pkg internal
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  mkdir -p "$root/tests/unit"
  cp "$repo_root/tests/unit/AUR-504.go" "$root/tests/unit/AUR-504.go"
  cat >"$root/tests/unit/aur504_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR504UnitBridge(t *testing.T) { TestAUR504(t) }
EOF
  chmod -R u+w -- "$root"
}

# run_selector runs the bridged unit program, optionally filtering to one
# subtest, and returns its exit code. A zero-tests run is never a pass.
run_selector() {
  local root="$1" filter="${2:-}"
  local out rc
  if [[ -n "$filter" ]]; then
    if out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run "TestAUR504UnitBridge/$filter" -count=1 2>&1)"; then
      rc=0
    else
      rc=$?
    fi
  else
    if out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR504UnitBridge$' -count=1 2>&1)"; then
      rc=0
    else
      rc=$?
    fi
  fi
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g' >&2
  if ((rc == 0)); then
    grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || return 99
    grep -Eq -- '--- PASS' <<<"$out" || return 99
  fi
  return "$rc"
}

expect_pass() {
  local rc
  set +e
  run_selector "$1" "${2:-}"
  rc=$?
  set -e
  ((rc == 0)) || fail "selector:${2:-all}:exit:$rc"
}

expect_red() {
  local rc
  set +e
  run_selector "$1" "$2" 2>/dev/null
  rc=$?
  set -e
  case "$rc" in
    0|99) fail "$3" ;;
    1) ;;
    *) fail "$3/unexpected-exit:$rc" ;;
  esac
}

# replace_once rewrites the first occurrence of needle on a single line.
replace_once() {
  local target="$1" needle="$2" repl="$3" label="$4"
  NEEDLE="$needle" REPL="$repl" awk '
    BEGIN { needle = ENVIRON["NEEDLE"]; repl = ENVIRON["REPL"]; done = 0 }
    {
      if (!done && index($0, needle) > 0) {
        i = index($0, needle)
        $0 = substr($0, 1, i - 1) repl substr($0, i + length(needle))
        done = 1
      }
      print
    }
    END { if (!done) exit 1 }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target" || fail "$label/rewrite-failed"
  if grep -Fq -- "$needle" "$target"; then fail "$label/mutation-not-applied"; fi
}

baseline_case() {
  local root="$run_dir/root-baseline"
  stage_source "$root"
  expect_pass "$root"
  cleanup_root "$root"
}

ac001_case() {
  local root="$run_dir/root-ac001"
  stage_source "$root"
  expect_pass "$root" 'AC-001'
  cleanup_root "$root"
}

ac002_case() {
  local root="$run_dir/root-ac002"
  stage_source "$root"
  expect_pass "$root" 'AC-002'
  cleanup_root "$root"
}

ac003_case() {
  local root="$run_dir/root-ac003"
  stage_source "$root"
  expect_pass "$root" 'AC-003'
  cleanup_root "$root"
}

# --- MUT-001: publish on GITHUB_SHA (the merge commit) again --------------
readonly mut1_needle='commitID = headSHA'
readonly mut1_repl='commitID = func() string { _ = headSHA; return os.Getenv("GITHUB_SHA") }()'

mutation_case_1() {
  local root="$run_dir/root-mut1"
  stage_source "$root"
  replace_once "$root/cmd/aurumcode/pr.go" "$mut1_needle" "$mut1_repl" MUT-001
  expect_red "$root" 'AC-001' 'MUT-001/AC-001-survived'
  # Specificity: the mutation is scoped to the status anchor; the --base
  # contract is untouched.
  expect_pass "$root" 'AC-002'
  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# --- MUT-002: suppress the fail-closed when the head is missing -----------
readonly mut2_needle='return "", errors.New("determining the pull request head commit: the API response carried no head SHA")'
readonly mut2_repl='return strings.TrimSpace(os.Getenv("GITHUB_SHA")), nil'

mutation_case_2() {
  local root="$run_dir/root-mut2"
  stage_source "$root"
  replace_once "$root/cmd/aurumcode/pr.go" "$mut2_needle" "$mut2_repl" MUT-002
  expect_red "$root" 'AC-003' 'MUT-002/AC-003-survived'
  # Specificity: AC-001 still anchors on the API head under the mutation.
  expect_pass "$root" 'AC-001'
  cleanup_root "$root"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

run_all() {
  baseline_case
  ac001_case
  ac002_case
  ac003_case
  mutation_case_1
  mutation_case_2
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  all) run_all ;;
  AC-001) ac001_case; printf '%s/%s/AC-001/ok\n' "$card" "$scenario" ;;
  AC-002) ac002_case; printf '%s/%s/AC-002/ok\n' "$card" "$scenario" ;;
  AC-003) ac003_case; printf '%s/%s/AC-003/ok\n' "$card" "$scenario" ;;
  MUT-001) mutation_case_1 ;;
  MUT-002) mutation_case_2 ;;
esac
