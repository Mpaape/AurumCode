#!/usr/bin/env bash
#
# Acceptance program for card AUR-505.
#
# WHAT THIS PROVES
#
#   When the model answers with a response the parser cannot validate
#   (`validation_failed`), `aurumcode review --pr ... --publicar
#   --seguranca --check` must DEGRADE, not crash. Before this card the
#   command printed the diagnosis and exited 1 without publishing a single
#   deterministic finding, so a CI pull request failed even when the
#   deterministic static analysis and security passes were clean.
#
#   AC-001: with an invalid response, the deterministic findings (static
#   analysis + the --seguranca pass) are still published, a declared
#   limitation names the model failure, and the exit code follows the
#   deterministic gate -- --check reports failure (exit 3) when a
#   deterministic error finding exists, and success (exit 0) when it does
#   not. A blanket 1 is gone.
#   AC-002: a valid response still merges normally (no regression).
#   AC-003: the failure is recorded (never silent), and NO finding is
#   fabricated from the invalid response -- the deterministic finding set
#   published for the invalid response equals the set published for a
#   valid empty response over the same diff.
#
#   The behavioral assertions live in tests/unit/AUR-505.go::TestAUR505,
#   run here through the project's generated _test.go bridge pattern. The
#   fake GitHub is a loopback httptest server and the model is an offline
#   fixture, so nothing reaches the network.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutant)
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

readonly card='AUR-505'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|MUT-001|MUT-002) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

[[ -e "$repo_root/cmd/aurumcode/pr.go" ]] || infra missing_input:cmd/aurumcode/pr.go
[[ -e "$repo_root/tests/unit/AUR-505.go" ]] || fail behavior-missing:tests/unit/AUR-505.go
grep -Fq 'modelInvalidOutputNotice' "$repo_root/cmd/aurumcode/pr.go" || fail behavior-missing:degradation-hook

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a505.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir" GOMAXPROCS=1

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_root materializes exactly what `go test ./tests/unit` and the
# binary that test builds on demand need. The whole cmd/aurumcode,
# internal and pkg trees are copied rather than a hand-maintained package
# list, so a new import cannot silently turn this proof into a 79.
stage_root() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum cmd/aurumcode internal pkg
  mkdir -p "$root/tests/unit"
  cp "$repo_root/tests/unit/AUR-505.go" "$root/tests/unit/AUR-505.go"
  cat >"$root/tests/unit/aur505_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR505Bridge(t *testing.T) { TestAUR505(t) }
EOF
  chmod -R u+w -- "$root"
}

# run_unit runs the bridge for one subtest selector and echoes the raw
# `go test` output. Its return value is the raw go test exit code; a
# compile failure or an environment failure must not be conflated with the
# behavioral RED the mutations assert (those checks inspect the output).
run_unit() {
  local root="$1" selector="$2" out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s \
    ./tests/unit -run "^TestAUR505Bridge/${selector}$" -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  return "$rc"
}

shared_root="$run_dir/root-shared"
shared_staged=0
stage_shared() {
  ((shared_staged == 0)) || return 0
  stage_root "$shared_root"
  shared_staged=1
}

# nominal_call runs one AC selector against the pristine staged root.
nominal_call() {
  local selector="$1" out rc
  stage_shared
  set +e
  out="$(run_unit "$shared_root" "$selector")"
  rc=$?
  set -e
  printf '%s\n' "$out" >&2
  ((rc == 0)) || fail "nominal:${selector}:exit:${rc}"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "nominal:${selector}:zero-tests"
}

# mutated_call stages a fresh root, applies the given sed program to
# cmd/aurumcode/pr.go, proves the mutation landed, and runs the selector
# that must now go RED. A failure that is not the expected behavioral
# failure (a build error, a missing bridge) is infrastructure, never the
# RED this program claims.
mutated_call() {
  local name="$1" sed_expr="$2" selector="$3" expect="$4"
  local root="$run_dir/root-$name"
  stage_root "$root"
  local target="$root/cmd/aurumcode/pr.go" before after out rc
  before="$(cksum "$target")" || infra "$name/cksum"
  sed -i "$sed_expr" "$target" || infra "$name/rewrite"
  after="$(cksum "$target")" || infra "$name/cksum"
  [[ "$before" != "$after" ]] || infra "$name/mutation-not-applied"
  grep -Fq "$expect" "$target" || infra "$name/mutation-anchor-missing"

  set +e
  out="$(run_unit "$root" "$selector")"
  rc=$?
  set -e
  printf '%s\n' "$out" >&2
  if ((rc == 0)); then
    fail "$name/not-rejected:${selector}"
  fi
  # A go test failure is exit 1; anything else (2 usage, 79 from the test
  # build) is an environment problem, not the defect being reproduced.
  ((rc == 1)) || infra "$name/unexpected-exit:$rc"
  grep -Fq -- "--- FAIL: TestAUR505Bridge/${selector}" <<<"$out" || infra "$name/wrong-failure-mode"

  # Restoration: the pristine shared root still passes this exact selector.
  local out2 rc2
  stage_shared
  set +e
  out2="$(run_unit "$shared_root" "$selector")"
  rc2=$?
  set -e
  ((rc2 == 0)) || fail "$name/restoration-broken"
  printf '%s/%s/%s/rejected\n' "$card" "$selector" "$name"
}

run_ac001() { nominal_call 'AC-001'; }
run_ac002() { nominal_call 'AC-002'; }
run_ac003() { nominal_call 'AC-003'; }

# MUT-001: drop the deterministic findings on an invalid response -- restore
# the old `return 1` -- and AC-001 must fall (no deterministic finding
# published, exit 1 instead of the deterministic gate).
run_mut001() {
  mutated_call 'MUT-001' \
    's/qualityDegraded = true/return 1 \/\/ AUR-505 MUT-001/' \
    'AC-001' 'AUR-505 MUT-001'
}

# MUT-002: treat the invalid response as valid and fabricate a finding; the
# fabricated finding must surface and AC-003 must fall.
run_mut002() {
  mutated_call 'MUT-002' \
    's/result\.Limitations = append(result\.Limitations, modelInvalidOutputNotice(reviewLanguage, string(parseErr\.Kind)))/result.Issues = append(result.Issues, types.ReviewIssue{File: "app.go", Line: 2, Severity: "error", Message: "AUR505-FABRICATED"})/' \
    'AC-003' 'AUR505-FABRICATED'
}

run_all() {
  run_ac001
  run_ac002
  run_ac003
  run_mut001
  run_mut002
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$selector"
}

case "$selector" in
  all) run_all ;;
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
esac
